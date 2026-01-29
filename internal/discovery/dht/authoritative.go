// Package dht 提供分布式哈希表实现
//
// 本文件实现 DHT 权威目录能力：
// - DHT 作为跨 Relay 的唯一权威地址来源
// - 定义权威性层级（DHT > Relay > Local）
// - 提供权威查询接口
package dht

import (
	"context"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              权威性层级定义
// ============================================================================
//删除重复定义，使用 pkgif 中的统一类型

// 类型别名，指向 pkgif 中的统一定义
type AuthoritativeSource = pkgif.AuthoritativeSource

// 常量别名，指向 pkgif 中的统一定义
const (
	SourceUnknown = pkgif.SourceUnknown
	SourceLocal   = pkgif.SourceLocal
	SourceRelay   = pkgif.SourceRelay
	SourceDHT     = pkgif.SourceDHT
)

// ============================================================================
//                              权威记录定义
// ============================================================================

// AuthoritativeRecord 权威记录
//
// 包含 PeerRecord 及其权威性元数据
type AuthoritativeRecord struct {
	// Record 签名的 PeerRecord
	Record *SignedRealmPeerRecord

	// Source 记录来源
	Source AuthoritativeSource

	// FetchedAt 获取时间
	FetchedAt time.Time

	// ExpiresAt 过期时间（基于 TTL 计算）
	ExpiresAt time.Time

	// Verified 是否经过完整验证
	Verified bool

	// VerificationError 验证错误（如果有）
	VerificationError error
}

// IsExpired 检查记录是否已过期
func (r *AuthoritativeRecord) IsExpired() bool {
	return time.Now().After(r.ExpiresAt)
}

// IsValid 检查记录是否有效（未过期且已验证）
func (r *AuthoritativeRecord) IsValid() bool {
	return r.Verified && !r.IsExpired() && r.VerificationError == nil
}

// TTLRemaining 返回剩余 TTL
func (r *AuthoritativeRecord) TTLRemaining() time.Duration {
	remaining := time.Until(r.ExpiresAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// ============================================================================
//                              地址簿集成接口（依赖倒置）
// ============================================================================

// AddressBookProvider 地址簿提供者接口
//
// 用于与 Relay 地址簿集成，实现"DHT 权威 / Relay 缓存"的协作关系。
// 这是依赖倒置的核心接口，DHT 不直接依赖 Relay 实现。
type AddressBookProvider interface {
	// GetPeerAddresses 从地址簿获取节点地址
	//
	// 返回:
	//   - directAddrs: 直连地址列表
	//   - relayAddrs: 中继地址列表
	//   - found: 是否找到记录
	GetPeerAddresses(ctx context.Context, nodeID types.NodeID) (directAddrs, relayAddrs []string, found bool)

	// UpdateFromDHT 用 DHT 权威记录更新地址簿
	//
	// 当 DHT 查询返回权威记录时，应调用此方法更新 Relay 地址簿缓存
	UpdateFromDHT(ctx context.Context, record *SignedRealmPeerRecord) error

	// InvalidateCache 使缓存失效
	//
	// 当检测到地址不可达时调用
	InvalidateCache(ctx context.Context, nodeID types.NodeID) error
}

// ============================================================================
//                              查询结果
// ============================================================================
//使用 pkgif.AuthoritativeQueryResult 作为统一返回类型

// AuthoritativeQueryResult 类型别名，指向 pkgif 中的统一定义
type AuthoritativeQueryResult = pkgif.AuthoritativeQueryResult

// ============================================================================
//                              DHT 权威查询方法
// ============================================================================

// GetAuthoritativePeerRecord 获取权威 PeerRecord
//
// 实现地址发现优先级：
//  1. DHT（权威）→ 2. Relay 地址簿（缓存）→ 3. Peerstore（本地）
//
//方法签名与 pkgif.DHT 接口保持一致（移除变参选项）
//
// 参数:
//   - ctx: 上下文
//   - realmID: Realm ID
//   - nodeID: 目标节点 ID
//
// 返回:
//   - *AuthoritativeQueryResult: 查询结果
//   - error: 查询失败时返回错误
func (d *DHT) GetAuthoritativePeerRecord(
	ctx context.Context,
	realmID types.RealmID,
	nodeID types.NodeID,
) (*AuthoritativeQueryResult, error) {
	if !d.started.Load() {
		return nil, ErrNotStarted
	}

	startTime := time.Now()
	//使用默认选项（查询所有来源）
	options := defaultAuthoritativeQueryOptions()

	result := &AuthoritativeQueryResult{
		Source: SourceUnknown,
	}

	// 1. 尝试从 DHT 获取（最高权威）
	if options.QueryDHT {
		//使用内部方法获取结构化记录
		dhtRecord, err := d.getPeerRecordInternal(ctx, realmID, nodeID)
		if err == nil && dhtRecord != nil {
			record := d.buildAuthoritativeRecord(dhtRecord, SourceDHT)
			if record.IsValid() {
				result.Source = SourceDHT
				result.DirectAddrs = dhtRecord.Record.DirectAddrs
				result.RelayAddrs = dhtRecord.Record.RelayAddrs
				result.Addresses = append(result.DirectAddrs, result.RelayAddrs...)
				result.QueryDuration = time.Since(startTime)
				result.ExpiresAt = record.ExpiresAt // 设置过期时间

				// 更新 Relay 地址簿缓存（如果配置了）
				if d.addressBookProvider != nil {
					_ = d.addressBookProvider.UpdateFromDHT(ctx, dhtRecord)
				}

				logger.Debug("权威查询：DHT 命中",
					"nodeID", nodeID,
					"seq", dhtRecord.Record.Seq,
					"duration", result.QueryDuration)
				return result, nil
			}
		}
	}

	// 2. 尝试从 Relay 地址簿获取（缓存层）
	if options.QueryRelay && d.addressBookProvider != nil {
		directAddrs, relayAddrs, found := d.addressBookProvider.GetPeerAddresses(ctx, nodeID)
		if found && (len(directAddrs) > 0 || len(relayAddrs) > 0) {
			result.Source = SourceRelay
			result.DirectAddrs = directAddrs
			result.RelayAddrs = relayAddrs
			result.Addresses = append(directAddrs, relayAddrs...)
			result.FallbackUsed = true
			result.FallbackReason = "DHT miss, using Relay cache"
			result.QueryDuration = time.Since(startTime)

			logger.Debug("权威查询：Relay 缓存命中",
				"nodeID", nodeID,
				"directCount", len(directAddrs),
				"relayCount", len(relayAddrs))
			return result, nil
		}
	}

	// 3. 尝试从 Peerstore 获取（本地缓存）
	if options.QueryPeerstore && d.peerstore != nil {
		peerID := types.PeerID(nodeID)
		addrs := d.peerstore.Addrs(peerID)
		if len(addrs) > 0 {
			result.Source = SourceLocal
			for _, addr := range addrs {
				addrStr := addr.String()
				result.Addresses = append(result.Addresses, addrStr)
				if isRelayAddr(addrStr) {
					result.RelayAddrs = append(result.RelayAddrs, addrStr)
				} else {
					result.DirectAddrs = append(result.DirectAddrs, addrStr)
				}
			}
			result.FallbackUsed = true
			result.FallbackReason = "DHT and Relay miss, using local cache"
			result.QueryDuration = time.Since(startTime)

			logger.Debug("权威查询：Peerstore 缓存命中",
				"nodeID", nodeID,
				"addrCount", len(addrs))
			return result, nil
		}
	}

	result.QueryDuration = time.Since(startTime)
	return result, ErrPeerNotFound
}

// GetAuthoritativePeerRecordWithOptions 获取权威 PeerRecord（带选项）
//
// 内部使用的方法，支持查询选项配置。
// 外部应使用 GetAuthoritativePeerRecord 方法。
func (d *DHT) GetAuthoritativePeerRecordWithOptions(
	ctx context.Context,
	realmID types.RealmID,
	nodeID types.NodeID,
	opts ...AuthoritativeQueryOption,
) (*AuthoritativeQueryResult, error) {
	if !d.started.Load() {
		return nil, ErrNotStarted
	}

	startTime := time.Now()
	options := defaultAuthoritativeQueryOptions()
	for _, opt := range opts {
		opt(options)
	}

	result := &AuthoritativeQueryResult{
		Source: SourceUnknown,
	}

	// 1. 尝试从 DHT 获取（最高权威）
	if options.QueryDHT {
		//使用内部方法获取结构化记录
		dhtRecord, err := d.getPeerRecordInternal(ctx, realmID, nodeID)
		if err == nil && dhtRecord != nil {
			record := d.buildAuthoritativeRecord(dhtRecord, SourceDHT)
			if record.IsValid() {
				result.Source = SourceDHT
				result.DirectAddrs = dhtRecord.Record.DirectAddrs
				result.RelayAddrs = dhtRecord.Record.RelayAddrs
				result.Addresses = append(result.DirectAddrs, result.RelayAddrs...)
				result.QueryDuration = time.Since(startTime)
				result.ExpiresAt = record.ExpiresAt

				if d.addressBookProvider != nil {
					_ = d.addressBookProvider.UpdateFromDHT(ctx, dhtRecord)
				}
				return result, nil
			}
		}
	}

	// 2. 尝试从 Relay 地址簿获取
	if options.QueryRelay && d.addressBookProvider != nil {
		directAddrs, relayAddrs, found := d.addressBookProvider.GetPeerAddresses(ctx, nodeID)
		if found && (len(directAddrs) > 0 || len(relayAddrs) > 0) {
			result.Source = SourceRelay
			result.DirectAddrs = directAddrs
			result.RelayAddrs = relayAddrs
			result.Addresses = append(directAddrs, relayAddrs...)
			result.FallbackUsed = true
			result.FallbackReason = "DHT miss, using Relay cache"
			result.QueryDuration = time.Since(startTime)
			return result, nil
		}
	}

	// 3. 尝试从 Peerstore 获取
	if options.QueryPeerstore && d.peerstore != nil {
		peerID := types.PeerID(nodeID)
		addrs := d.peerstore.Addrs(peerID)
		if len(addrs) > 0 {
			result.Source = SourceLocal
			for _, addr := range addrs {
				addrStr := addr.String()
				result.Addresses = append(result.Addresses, addrStr)
				if isRelayAddr(addrStr) {
					result.RelayAddrs = append(result.RelayAddrs, addrStr)
				} else {
					result.DirectAddrs = append(result.DirectAddrs, addrStr)
				}
			}
			result.FallbackUsed = true
			result.FallbackReason = "DHT and Relay miss, using local cache"
			result.QueryDuration = time.Since(startTime)
			return result, nil
		}
	}

	result.QueryDuration = time.Since(startTime)
	return result, ErrPeerNotFound
}

// buildAuthoritativeRecord 构建权威记录
func (d *DHT) buildAuthoritativeRecord(signed *SignedRealmPeerRecord, source AuthoritativeSource) *AuthoritativeRecord {
	record := &AuthoritativeRecord{
		Record:    signed,
		Source:    source,
		FetchedAt: time.Now(),
	}

	if signed != nil && signed.Record != nil {
		// 计算过期时间
		ttl := time.Duration(signed.Record.TTL) * time.Millisecond
		if ttl == 0 {
			ttl = d.config.PeerRecordTTL
		}
		record.ExpiresAt = time.Now().Add(ttl)

		// 验证记录
		validator := NewDefaultPeerRecordValidator()
		key := RealmPeerKey(signed.Record.RealmID, signed.Record.NodeID)
		if err := validator.Validate(key, signed); err != nil {
			record.VerificationError = err
			record.Verified = false
		} else {
			record.Verified = true
		}
	}

	return record
}

// ============================================================================
//                              查询选项
// ============================================================================

// AuthoritativeQueryOptions 权威查询选项
type AuthoritativeQueryOptions struct {
	// QueryDHT 是否查询 DHT（默认 true）
	QueryDHT bool

	// QueryRelay 是否查询 Relay 地址簿（默认 true）
	QueryRelay bool

	// QueryPeerstore 是否查询 Peerstore（默认 true）
	QueryPeerstore bool

	// RequireAuthoritative 是否要求权威结果（仅 DHT）
	RequireAuthoritative bool
}

// AuthoritativeQueryOption 查询选项函数
type AuthoritativeQueryOption func(*AuthoritativeQueryOptions)

// defaultAuthoritativeQueryOptions 返回默认查询选项
func defaultAuthoritativeQueryOptions() *AuthoritativeQueryOptions {
	return &AuthoritativeQueryOptions{
		QueryDHT:             true,
		QueryRelay:           true,
		QueryPeerstore:       true,
		RequireAuthoritative: false,
	}
}

// WithDHTOnly 仅查询 DHT
func WithDHTOnly() AuthoritativeQueryOption {
	return func(o *AuthoritativeQueryOptions) {
		o.QueryDHT = true
		o.QueryRelay = false
		o.QueryPeerstore = false
		o.RequireAuthoritative = true
	}
}

// WithFallback 启用回退查询
func WithFallback(relay, peerstore bool) AuthoritativeQueryOption {
	return func(o *AuthoritativeQueryOptions) {
		o.QueryRelay = relay
		o.QueryPeerstore = peerstore
	}
}

// ============================================================================
//                              DHT 集成
// ============================================================================

// addressBookProvider 地址簿提供者（可选）
var _ AddressBookProvider = (*noopAddressBookProvider)(nil)

// noopAddressBookProvider 空操作地址簿提供者
type noopAddressBookProvider struct{}

func (n *noopAddressBookProvider) GetPeerAddresses(_ context.Context, _ types.NodeID) ([]string, []string, bool) {
	return nil, nil, false
}

func (n *noopAddressBookProvider) UpdateFromDHT(_ context.Context, _ *SignedRealmPeerRecord) error {
	return nil
}

func (n *noopAddressBookProvider) InvalidateCache(_ context.Context, _ types.NodeID) error {
	return nil
}
