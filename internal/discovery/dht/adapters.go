// Package dht 提供分布式哈希表实现
//
// 本文件实现适配器，将现有的 Relay 地址簿和可达性协调器
// 连接到 DHT 定义的接口，实现真实的集成而非伪实现。
package dht

import (
	"context"
	"strings"
	"time"

	realmif "github.com/dep2p/go-dep2p/internal/realm/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              RelayAddressBookAdapter
// ============================================================================

// RelayAddressBookAdapter 将 Realm 地址簿适配到 AddressBookProvider 接口
//
// 这个适配器连接 DHT 与 Relay 地址簿，实现：
// - DHT 查询失败时回退到 Relay 地址簿
// - DHT 查询成功时更新 Relay 地址簿缓存
// - 检测到不可达地址时使缓存失效
type RelayAddressBookAdapter struct {
	// addressBook Realm 地址簿（实现 realmif.AddressBook 接口）
	addressBook realmif.AddressBook
}

// NewRelayAddressBookAdapter 创建 Relay 地址簿适配器
func NewRelayAddressBookAdapter(addressBook realmif.AddressBook) *RelayAddressBookAdapter {
	return &RelayAddressBookAdapter{
		addressBook: addressBook,
	}
}

// GetPeerAddresses 从地址簿获取节点地址
//
// 实现 AddressBookProvider.GetPeerAddresses
func (a *RelayAddressBookAdapter) GetPeerAddresses(ctx context.Context, nodeID types.NodeID) (directAddrs, relayAddrs []string, found bool) {
	if a.addressBook == nil {
		return nil, nil, false
	}

	entry, err := a.addressBook.Query(ctx, nodeID)
	if err != nil {
		return nil, nil, false
	}

	if entry.IsEmpty() || !entry.HasAddrs() {
		return nil, nil, false
	}

	// 分类地址：直连地址 vs Relay 地址
	for _, addr := range entry.DirectAddrs {
		addrStr := addr.String()
		if isRelayAddr(addrStr) {
			relayAddrs = append(relayAddrs, addrStr)
		} else {
			directAddrs = append(directAddrs, addrStr)
		}
	}

	return directAddrs, relayAddrs, true
}

// UpdateFromDHT 用 DHT 权威记录更新地址簿
//
// 实现 AddressBookProvider.UpdateFromDHT
// 当 DHT 查询返回权威记录时，更新 Relay 地址簿缓存
func (a *RelayAddressBookAdapter) UpdateFromDHT(ctx context.Context, record *SignedRealmPeerRecord) error {
	if a.addressBook == nil || record == nil || record.Record == nil {
		return nil
	}

	r := record.Record

	// 收集所有地址
	var addrs []types.Multiaddr
	for _, addr := range r.DirectAddrs {
		if ma, err := types.NewMultiaddr(addr); err == nil {
			addrs = append(addrs, ma)
		}
	}
	for _, addr := range r.RelayAddrs {
		if ma, err := types.NewMultiaddr(addr); err == nil {
			addrs = append(addrs, ma)
		}
	}

	// 构建 MemberEntry
	entry := realmif.MemberEntry{
		NodeID:       r.NodeID,
		DirectAddrs:  addrs,
		NATType:      r.NATType,
		Capabilities: r.Capabilities,
		Online:       true,
		LastSeen:     time.Now(),
		LastUpdate:   time.Now(),
	}

	// 尝试更新，如果不存在则注册
	if err := a.addressBook.Update(ctx, entry); err != nil {
		// 可能是成员不存在，尝试注册
		return a.addressBook.Register(ctx, entry)
	}

	return nil
}

// InvalidateCache 使缓存失效
//
// 实现 AddressBookProvider.InvalidateCache
// 当检测到地址不可达时调用，标记节点为离线
func (a *RelayAddressBookAdapter) InvalidateCache(ctx context.Context, nodeID types.NodeID) error {
	if a.addressBook == nil {
		return nil
	}

	// 设置节点为离线状态
	return a.addressBook.SetOnline(ctx, nodeID, false)
}

// 确保实现接口
var _ AddressBookProvider = (*RelayAddressBookAdapter)(nil)

// ============================================================================
//                              CoordinatorReachabilityAdapter
// ============================================================================

// ReachabilityCoordinator 可达性协调器接口
//
// 这是对 internal/core/reachability.Coordinator 的抽象，
// 避免直接依赖具体实现。
type ReachabilityCoordinator interface {
	// VerifiedDirectAddresses 返回已验证的直连地址
	VerifiedDirectAddresses() []string

	// RelayAddresses 返回所有 Relay 地址
	RelayAddresses() []string

	// AdvertisedAddrs 返回当前可对外通告的地址集合
	AdvertisedAddrs() []string

	// HasVerifiedDirectAddress 是否有已验证的直连地址
	HasVerifiedDirectAddress() bool

	// HasRelayAddress 是否有 Relay 地址
	HasRelayAddress() bool
}

// CoordinatorReachabilityAdapter 将可达性协调器适配到 ReachabilityChecker 接口
//
// 这个适配器连接 DHT 与可达性验证系统，实现：
// - 真实的地址可达性验证（通过 DialBack）
// - 获取已验证的直连地址列表
// - 判断节点是否直连可达
type CoordinatorReachabilityAdapter struct {
	// coordinator 可达性协调器
	coordinator ReachabilityCoordinator

	// natTypeProvider NAT 类型提供器（可选）
	natTypeProvider func() types.NATType
}

// NewCoordinatorReachabilityAdapter 创建可达性协调器适配器
func NewCoordinatorReachabilityAdapter(coordinator ReachabilityCoordinator) *CoordinatorReachabilityAdapter {
	return &CoordinatorReachabilityAdapter{
		coordinator: coordinator,
	}
}

// SetNATTypeProvider 设置 NAT 类型提供器
func (a *CoordinatorReachabilityAdapter) SetNATTypeProvider(provider func() types.NATType) {
	a.natTypeProvider = provider
}

// CheckReachability 检查当前节点的可达性状态
//
// 实现 ReachabilityChecker.CheckReachability
func (a *CoordinatorReachabilityAdapter) CheckReachability(_ context.Context) (types.Reachability, types.NATType) {
	if a.coordinator == nil {
		return types.ReachabilityUnknown, types.NATTypeUnknown
	}

	// 判断可达性状态
	var reachability types.Reachability
	if a.coordinator.HasVerifiedDirectAddress() {
		reachability = types.ReachabilityPublic
	} else if a.coordinator.HasRelayAddress() {
		reachability = types.ReachabilityPrivate
	} else {
		reachability = types.ReachabilityUnknown
	}

	// 获取 NAT 类型
	var natType types.NATType
	if a.natTypeProvider != nil {
		natType = a.natTypeProvider()
	} else {
		// 根据可达性推断 NAT 类型
		switch reachability {
		case types.ReachabilityPublic:
			natType = types.NATTypeNone
		case types.ReachabilityPrivate:
			natType = types.NATTypeSymmetric
		default:
			natType = types.NATTypeUnknown
		}
	}

	return reachability, natType
}

// VerifyAddresses 验证地址列表的可达性
//
// 实现 ReachabilityChecker.VerifyAddresses
// 通过检查地址是否在已验证列表中来判断可达性
func (a *CoordinatorReachabilityAdapter) VerifyAddresses(_ context.Context, addrs []string) (verified, unverified []string) {
	if a.coordinator == nil {
		// 无协调器，所有地址视为未验证
		return nil, addrs
	}

	// 获取已验证的直连地址集合
	verifiedSet := make(map[string]bool)
	for _, addr := range a.coordinator.VerifiedDirectAddresses() {
		verifiedSet[addr] = true
	}

	// 获取 Relay 地址集合（Relay 地址始终视为可用）
	relaySet := make(map[string]bool)
	for _, addr := range a.coordinator.RelayAddresses() {
		relaySet[addr] = true
	}

	for _, addr := range addrs {
		// Relay 地址始终视为已验证
		if isRelayAddr(addr) || relaySet[addr] {
			verified = append(verified, addr)
		} else if verifiedSet[addr] {
			// 直连地址需要在已验证列表中
			verified = append(verified, addr)
		} else {
			// 检查是否是已验证地址的前缀匹配（处理端口不同的情况）
			found := false
			for vAddr := range verifiedSet {
				if strings.HasPrefix(addr, strings.Split(vAddr, "/tcp")[0]) ||
					strings.HasPrefix(vAddr, strings.Split(addr, "/tcp")[0]) {
					verified = append(verified, addr)
					found = true
					break
				}
			}
			if !found {
				unverified = append(unverified, addr)
			}
		}
	}

	return verified, unverified
}

// GetVerifiedDirectAddresses 获取已验证的直连地址
//
// 实现 ReachabilityChecker.GetVerifiedDirectAddresses
func (a *CoordinatorReachabilityAdapter) GetVerifiedDirectAddresses(_ context.Context) []string {
	if a.coordinator == nil {
		return nil
	}

	return a.coordinator.VerifiedDirectAddresses()
}

// IsDirectlyReachable 检查是否直连可达
//
// 实现 ReachabilityChecker.IsDirectlyReachable
func (a *CoordinatorReachabilityAdapter) IsDirectlyReachable(_ context.Context) bool {
	if a.coordinator == nil {
		return false
	}

	return a.coordinator.HasVerifiedDirectAddress()
}

// 确保实现接口
var _ ReachabilityChecker = (*CoordinatorReachabilityAdapter)(nil)

// ============================================================================
//                              工厂函数
// ============================================================================

// CreateAddressBookAdapter 创建地址簿适配器的工厂函数
//
// 参数接受 interface{} 以支持依赖注入，内部进行类型断言
func CreateAddressBookAdapter(addressBook interface{}) AddressBookProvider {
	if addressBook == nil {
		return nil
	}

	if ab, ok := addressBook.(realmif.AddressBook); ok {
		return NewRelayAddressBookAdapter(ab)
	}

	return nil
}

// CreateReachabilityAdapter 创建可达性适配器的工厂函数
//
// 参数接受 interface{} 以支持依赖注入，内部进行类型断言
func CreateReachabilityAdapter(coordinator interface{}) ReachabilityChecker {
	if coordinator == nil {
		return nil
	}

	if c, ok := coordinator.(ReachabilityCoordinator); ok {
		return NewCoordinatorReachabilityAdapter(c)
	}

	return nil
}
