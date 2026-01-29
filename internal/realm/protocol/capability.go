package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
	
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/protocol"
	"github.com/dep2p/go-dep2p/pkg/types"
)

var logger = log.Logger("realm/protocol/capability")

// ============================================================================
//                          节点能力公告协议
// ============================================================================

// protocolCapabilityAnnounce 生成节点能力公告协议 ID
// 协议 ID 嵌入 RealmID，实现协议级别的显式隔离
func protocolCapabilityAnnounce(realmID types.RealmID) string {
	return string(protocol.NewRealmBuilder(string(realmID)).Announce())
}

// CapabilityAnnounce 节点能力公告消息
//
// Realm 成员广播自己的网络能力（可达性、中继能力等）。
type CapabilityAnnounce struct {
	// NodeID 节点 ID
	NodeID string `json:"node_id"`
	
	// Reachability 可达性状态
	Reachability string `json:"reachability"` // "Public" / "Private" / "Unknown"
	
	// Addrs 地址列表（公网地址）
	Addrs []string `json:"addrs,omitempty"`
	
	// RelayCapable 是否具备中继能力
	RelayCapable bool `json:"relay_capable"`
	
	// Timestamp 时间戳
	Timestamp int64 `json:"timestamp"`
}

// CapabilityManager 能力管理器
//
// 负责：
//  1. 监听本地可达性变化
//  2. 广播给 Realm 成员
//  3. 接收远程成员的能力公告
//  4. 通知中继服务更新候选池
type CapabilityManager struct {
	realmID    types.RealmID
	protocolID string        // 协议 ID（嵌入 realmID）
	host       pkgif.Host
	eventbus   pkgif.EventBus
	nat        pkgif.NATService

	// 连接函数（用于能力公告前确保连接）
	connectFunc func(ctx context.Context, peerID string) error
	
	// 成员提供者：返回当前 Realm 成员列表
	memberProvider func() []string
	
	// 回调函数：接收到远程成员能力公告时调用
	onMemberCapability func(nodeID string, reachability string, addrs []string)
	
	// 成员能力缓存
	mu           sync.RWMutex
	capabilities map[string]*CapabilityAnnounce // nodeID -> capability
	
	// 生命周期
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewCapabilityManager 创建能力管理器
func NewCapabilityManager(
	realmID types.RealmID,
	host pkgif.Host,
	eventbus pkgif.EventBus,
	nat pkgif.NATService,
) *CapabilityManager {
	ctx, cancel := context.WithCancel(context.Background())
	
	return &CapabilityManager{
		realmID:      realmID,
		protocolID:   protocolCapabilityAnnounce(realmID),
		host:         host,
		eventbus:     eventbus,
		nat:          nat,
		capabilities: make(map[string]*CapabilityAnnounce),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// SetConnectFunc 设置连接函数（可选）
//
// 用于在发送能力公告前确保与目标节点建立连接。
func (m *CapabilityManager) SetConnectFunc(fn func(ctx context.Context, peerID string) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connectFunc = fn
}

// SetMemberCapabilityHandler 设置成员能力回调
func (m *CapabilityManager) SetMemberCapabilityHandler(handler func(string, string, []string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onMemberCapability = handler
}

// Start 启动能力管理器
func (m *CapabilityManager) Start() error {
	// 注册协议处理器
	if m.host != nil {
		m.host.SetStreamHandler(m.protocolID, m.handleCapabilityStream)
	}
	
	// 启动本地能力广播
	m.wg.Add(1)
	go m.broadcastLoop()
	
	return nil
}

// Stop 停止能力管理器
func (m *CapabilityManager) Stop() error {
	if m.cancel != nil {
		m.cancel()
	}
	
	// 移除协议处理器
	if m.host != nil {
		m.host.RemoveStreamHandler(m.protocolID)
	}
	
	m.wg.Wait()
	return nil
}

// broadcastLoop 定期广播本地能力
func (m *CapabilityManager) broadcastLoop() {
	defer m.wg.Done()
	
	// 首次立即广播
	m.broadcastCapability()
	
	ticker := time.NewTicker(60 * time.Second) // 每 60 秒广播一次
	defer ticker.Stop()
	
	for {
		select {
		case <-m.ctx.Done():
			return
			
		case <-ticker.C:
			m.broadcastCapability()
		}
	}
}

// broadcastCapability 广播本地能力
func (m *CapabilityManager) broadcastCapability() {
	// 构造能力公告
	announce := &CapabilityAnnounce{
		NodeID:    m.host.ID(),
		Timestamp: time.Now().Unix(),
	}
	
	// 获取可达性状态
	if m.nat != nil {
		announce.Reachability = m.nat.GetReachability()
		
		// 如果是公网可达，包含外部地址
		if announce.Reachability == "Public" {
			announce.Addrs = m.nat.ExternalAddrs()
			announce.RelayCapable = true
		}
	}
	
	// 发布事件（通过 EventBus）
	// 注意：EventBus.Publish 方法需要在接口中定义
	// 当前简化：通过触发回调通知
	m.mu.RLock()
	handler := m.onMemberCapability
	m.mu.RUnlock()
	
	if handler != nil {
		handler(announce.NodeID, announce.Reachability, announce.Addrs)
	}

	// 向 Realm 成员广播（单播到每个成员）
	m.sendToMembers(announce)
}

// handleCapabilityStream 处理能力公告流
func (m *CapabilityManager) handleCapabilityStream(stream pkgif.Stream) {
	defer stream.Close()
	
	// 解码能力公告
	var announce CapabilityAnnounce
	decoder := json.NewDecoder(stream)
	if err := decoder.Decode(&announce); err != nil {
		return
	}
	
	// 验证消息
	if announce.NodeID == "" {
		return
	}
	
	// 更新缓存
	m.mu.Lock()
	m.capabilities[announce.NodeID] = &announce
	m.mu.Unlock()
	
	// 触发回调
	m.mu.RLock()
	handler := m.onMemberCapability
	m.mu.RUnlock()
	
	if handler != nil {
		handler(announce.NodeID, announce.Reachability, announce.Addrs)
	}
}

// GetMemberCapability 获取成员能力
func (m *CapabilityManager) GetMemberCapability(nodeID string) *CapabilityAnnounce {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	if cap, exists := m.capabilities[nodeID]; exists {
		// 返回拷贝
		return &CapabilityAnnounce{
			NodeID:       cap.NodeID,
			Reachability: cap.Reachability,
			Addrs:        append([]string{}, cap.Addrs...),
			RelayCapable: cap.RelayCapable,
			Timestamp:    cap.Timestamp,
		}
	}
	
	return nil
}

// CapabilityAnnounceEvent 能力公告事件
type CapabilityAnnounceEvent struct {
	RealmID  types.RealmID
	Announce *CapabilityAnnounce
}

// Topic 返回事件主题
func (e *CapabilityAnnounceEvent) Topic() string {
	return fmt.Sprintf("realm.capability.%s", e.RealmID)
}

// ReBroadcast 重新广播本节点的能力
//
// 当网络变化时（如 4G→WiFi），本节点的网络能力可能发生改变：
// - 可达性变化（Private → Public 或反向）
// - 地址列表更新
//
// 需要重新广播让其他成员感知。
func (m *CapabilityManager) ReBroadcast(_ context.Context, newAddrs []string) error {
	logger.Info("网络变化，重新广播能力",
		"newAddrs", len(newAddrs))
	
	// 获取本地节点 ID
	localNodeID := m.host.ID()
	
	// 重新评估可达性
	m.mu.RLock()
	oldCap := m.capabilities[localNodeID]
	m.mu.RUnlock()
	
	reachability := "Unknown"
	relayCapable := false
	
	if oldCap != nil {
		reachability = oldCap.Reachability
		relayCapable = oldCap.RelayCapable
	}
	
	// 如果有新地址，更新可达性判断
	if len(newAddrs) > 0 {
		// 简化判断：有公网地址则为 Public
		for _, addr := range newAddrs {
			if !isPrivateAddr(addr) {
				reachability = "Public"
				relayCapable = true
				break
			}
		}
	}
	
	// 构建新的能力公告
	announce := CapabilityAnnounce{
		NodeID:       localNodeID,
		Reachability: reachability,
		Addrs:        newAddrs,
		RelayCapable: relayCapable,
		Timestamp:    time.Now().Unix(),
	}
	
	// 更新本地缓存
	m.mu.Lock()
	m.capabilities[localNodeID] = &announce
	m.mu.Unlock()
	
	// 使用现有的 broadcastCapability 方法
	// 因为它会自动从缓存中读取最新的能力信息
	m.broadcastCapability()
	
	return nil
}

// SetMemberProvider 设置成员提供者
//
// 用于广播时获取当前 Realm 成员列表。
func (m *CapabilityManager) SetMemberProvider(provider func() []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.memberProvider = provider
}

// isPrivateAddr 判断是否为私网地址
//
// 根据 RFC 1918 和 RFC 4193 判断地址是否为私网地址：
// - IPv4 私网范围：10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16
// - IPv4 链路本地：169.254.0.0/16
// - IPv4 环回：127.0.0.0/8
// - IPv6 私网：fc00::/7 (ULA), fe80::/10 (链路本地)
// - IPv6 环回：::1
func isPrivateAddr(addr string) bool {
	// 解析 multiaddr 格式的地址
	ip := extractIPFromMultiaddr(addr)
	if ip == nil {
		return true // 无法解析，保守认为是私网
	}

	return isPrivateIP(ip)
}

// extractIPFromMultiaddr 从 multiaddr 格式中提取 IP 地址
func extractIPFromMultiaddr(addr string) net.IP {
	// 格式示例: /ip4/192.168.1.1/tcp/4001 或 /ip6/::1/tcp/4001
	parts := strings.Split(addr, "/")
	
	for i, part := range parts {
		if (part == "ip4" || part == "ip6") && i+1 < len(parts) {
			return net.ParseIP(parts[i+1])
		}
	}
	
	// 尝试直接解析为 IP
	return net.ParseIP(addr)
}

// isPrivateIP 判断 IP 是否为私网地址
func isPrivateIP(ip net.IP) bool {
	if ip == nil {
		return true
	}

	// 检查环回地址
	if ip.IsLoopback() {
		return true
	}

	// 检查链路本地地址
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}

	// IPv4 私网范围检查
	if ip4 := ip.To4(); ip4 != nil {
		// 10.0.0.0/8
		if ip4[0] == 10 {
			return true
		}
		// 172.16.0.0/12 (172.16.0.0 - 172.31.255.255)
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return true
		}
		// 192.168.0.0/16
		if ip4[0] == 192 && ip4[1] == 168 {
			return true
		}
		// 169.254.0.0/16 (链路本地)
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
		// 100.64.0.0/10 (CGN/Carrier-Grade NAT)
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return true
		}
		return false
	}

	// IPv6 私网范围检查
	if ip6 := ip.To16(); ip6 != nil {
		// fc00::/7 (Unique Local Address)
		if ip6[0] == 0xfc || ip6[0] == 0xfd {
			return true
		}
		// fe80::/10 (Link-Local)
		if ip6[0] == 0xfe && (ip6[1]&0xc0) == 0x80 {
			return true
		}
	}

	return false
}

// IsPublicAddr 判断是否为公网地址
func IsPublicAddr(addr string) bool {
	return !isPrivateAddr(addr)
}

// FilterPublicAddrs 过滤出公网地址
func FilterPublicAddrs(addrs []string) []string {
	public := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		if !isPrivateAddr(addr) {
			public = append(public, addr)
		}
	}
	return public
}

// FilterPrivateAddrs 过滤出私网地址
func FilterPrivateAddrs(addrs []string) []string {
	private := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		if isPrivateAddr(addr) {
			private = append(private, addr)
		}
	}
	return private
}

// SendToPeer 向指定节点发送能力公告（单播）
//
// P0 修复：当新成员加入时，主动向其发送本节点的能力信息。
// 这解决了新成员无法通过广播获取已有成员能力的问题。
func (m *CapabilityManager) SendToPeer(ctx context.Context, peerID string) error {
	if m.host == nil {
		return fmt.Errorf("host not available")
	}

	// 构造能力公告
	announce := &CapabilityAnnounce{
		NodeID:    m.host.ID(),
		Timestamp: time.Now().Unix(),
	}

	// 获取可达性状态
	if m.nat != nil {
		announce.Reachability = m.nat.GetReachability()

		// 如果是公网可达，包含外部地址
		if announce.Reachability == "Public" {
			announce.Addrs = m.nat.ExternalAddrs()
			announce.RelayCapable = true
		}
	} else {
		// NAT 服务不可用，使用 Host 的可分享地址
		announce.Reachability = "Unknown"
		announce.Addrs = m.host.ShareableAddrs()
	}

	return m.sendAnnounceToPeer(ctx, peerID, announce)
}

// sendToMembers 向所有成员发送能力公告（单播）
func (m *CapabilityManager) sendToMembers(announce *CapabilityAnnounce) {
	m.mu.RLock()
	provider := m.memberProvider
	m.mu.RUnlock()

	if provider == nil || m.host == nil || announce == nil {
		return
	}

	members := provider()
	if len(members) == 0 {
		return
	}

	localID := m.host.ID()
	for _, peerID := range members {
		if peerID == "" || peerID == localID {
			continue
		}

		ctx, cancel := context.WithTimeout(m.ctx, 3*time.Second)
		if err := m.sendAnnounceToPeer(ctx, peerID, announce); err != nil {
			logger.Debug("能力公告发送失败",
				"target", peerID[:8],
				"err", err)
		}
		cancel()
	}
}

// sendAnnounceToPeer 发送指定公告到目标节点
func (m *CapabilityManager) sendAnnounceToPeer(ctx context.Context, peerID string, announce *CapabilityAnnounce) error {
	if m.host == nil {
		return fmt.Errorf("host not available")
	}
	if announce == nil {
		return fmt.Errorf("announce is nil")
	}

	// 确保与目标节点已连接
	if err := m.ensureConnected(ctx, peerID); err != nil {
		return fmt.Errorf("ensure connection to %s failed: %w", peerID[:8], err)
	}

	// 打开流到目标节点
	stream, err := m.host.NewStream(ctx, peerID, m.protocolID)
	if err != nil {
		return fmt.Errorf("failed to open capability stream to %s: %w", peerID[:8], err)
	}
	defer stream.Close()

	// 编码并发送
	encoder := json.NewEncoder(stream)
	if err := encoder.Encode(announce); err != nil {
		return fmt.Errorf("failed to send capability announce to %s: %w", peerID[:8], err)
	}

	logger.Debug("已发送能力公告（单播）",
		"target", peerID[:8],
		"reachability", announce.Reachability,
		"addrs", len(announce.Addrs))

	return nil
}

// ensureConnected 确保与目标节点建立连接
func (m *CapabilityManager) ensureConnected(ctx context.Context, peerID string) error {
	network := m.host.Network()
	if network != nil && network.Connectedness(peerID) == pkgif.Connected {
		return nil
	}

	m.mu.RLock()
	connectFn := m.connectFunc
	m.mu.RUnlock()

	if connectFn != nil {
		return connectFn(ctx, peerID)
	}

	// fallback：使用 Host.Connect + Peerstore 地址
	ps := m.host.Peerstore()
	if ps == nil {
		return fmt.Errorf("peerstore not available")
	}
	addrs := ps.Addrs(types.PeerID(peerID))
	if len(addrs) == 0 {
		return fmt.Errorf("no addrs for peer")
	}

	addrStrs := make([]string, len(addrs))
	for i, addr := range addrs {
		addrStrs[i] = addr.String()
	}
	return m.host.Connect(ctx, peerID, addrStrs)
}
