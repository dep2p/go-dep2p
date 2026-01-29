package dht

import (
	"context"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              速率限制
// ============================================================================

const (
	// PeerRecordRateLimit PeerRecord 速率限制（每分钟）
	PeerRecordRateLimit = 10

	// ProviderRateLimit Provider 速率限制（每分钟）
	ProviderRateLimit = 50
)

// rateLimiter 速率限制器
type rateLimiter struct {
	// 每个 sender 的最后请求时间戳列表
	records map[types.NodeID][]time.Time

	// 限速阈值
	limit int

	// 时间窗口
	window time.Duration

	mu sync.Mutex
}

// newRateLimiter 创建速率限制器
func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		records: make(map[types.NodeID][]time.Time),
		limit:   limit,
		window:  window,
	}
}

// Allow 检查是否允许请求
func (rl *rateLimiter) Allow(sender types.NodeID) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// 获取该 sender 的历史记录
	records, exists := rl.records[sender]
	if !exists {
		rl.records[sender] = []time.Time{now}
		return true
	}

	// 清理过期记录
	cutoff := now.Add(-rl.window)
	var validRecords []time.Time
	for _, t := range records {
		if t.After(cutoff) {
			validRecords = append(validRecords, t)
		}
	}

	// 检查是否超限
	if len(validRecords) >= rl.limit {
		return false
	}

	// 添加新记录
	validRecords = append(validRecords, now)
	rl.records[sender] = validRecords

	return true
}

// ============================================================================
//                              地址验证
// ============================================================================

// isRoutableAddr 检查地址是否可路由
// allowPrivate: 是否允许私网地址
func isRoutableAddr(addr string, allowPrivate bool) bool {
	// 提取 IP 地址
	ip := extractIP(addr)
	if ip == nil {
		return false
	}

	// 始终拒绝回环地址
	if ip.IsLoopback() {
		return false
	}

	// 始终拒绝链路本地地址
	if ip.IsLinkLocalUnicast() {
		return false
	}

	// 根据配置决定是否接受私网地址
	if !allowPrivate && isPrivateIP(ip) {
		return false
	}

	return true
}

// extractIP 从 multiaddr 格式提取 IP
func extractIP(addr string) net.IP {
	// multiaddr 格式: /ip4/1.2.3.4/tcp/4001 或 /ip6/::1/tcp/4001
	parts := strings.Split(addr, "/")
	if len(parts) < 3 {
		return nil
	}

	ipProto := parts[1]
	ipStr := parts[2]

	if ipProto == "ip4" || ipProto == "ip6" {
		return net.ParseIP(ipStr)
	}

	return nil
}

// isPrivateIP 检查是否私网地址
func isPrivateIP(ip net.IP) bool {
	// 10.0.0.0/8
	if ip4 := ip.To4(); ip4 != nil {
		return ip4[0] == 10 ||
			// 172.16.0.0/12
			(ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31) ||
			// 192.168.0.0/16
			(ip4[0] == 192 && ip4[1] == 168)
	}

	// IPv6 私网: fc00::/7
	if len(ip) == 16 {
		return ip[0]&0xfe == 0xfc
	}

	return false
}

// ============================================================================
//                              Handler
// ============================================================================

// Handler DHT 协议处理器
//
// 实现 Layer1 安全机制：
//  1. 速率限制（PeerRecord: 10/min, Provider: 50/min）
//  2. 地址验证（拒绝私网、回环、链路本地地址）
//  3. Sender 身份验证
type Handler struct {
	// dht DHT 实例
	dht *DHT

	// 速率限制器
	peerRecordLimiter *rateLimiter
	providerLimiter   *rateLimiter

	mu sync.RWMutex
}

// NewHandler 创建协议处理器
func NewHandler(dht *DHT) *Handler {
	return &Handler{
		dht:               dht,
		peerRecordLimiter: newRateLimiter(PeerRecordRateLimit, 1*time.Minute),
		providerLimiter:   newRateLimiter(ProviderRateLimit, 1*time.Minute),
	}
}

// filterValidAddrs 过滤有效地址
func (h *Handler) filterValidAddrs(addrs []string, allowPrivate bool) []string {
	if len(addrs) == 0 {
		return nil
	}

	var valid []string
	for _, addr := range addrs {
		if isRoutableAddr(addr, allowPrivate) {
			valid = append(valid, addr)
		}
	}
	return valid
}

// HandleStream 处理协议流
func (h *Handler) HandleStream(ctx context.Context, stream io.ReadWriteCloser) {
	defer stream.Close()

	// 读取请求
	req, err := h.dht.network.readMessage(stream)
	if err != nil {
		h.sendErrorResponse(stream, 0, types.NodeID(h.dht.host.ID()), MessageTypeFindNode, fmt.Sprintf("read message failed: %v", err))
		return
	}

	// 验证 Sender
	if req.Sender == "" {
		h.sendErrorResponse(stream, req.RequestID, types.NodeID(h.dht.host.ID()), req.Type, "sender is empty")
		return
	}

	// 路由到对应处理函数
	var resp *Message
	switch req.Type {
	case MessageTypeFindNode:
		resp = h.handleFindNode(ctx, req)
	case MessageTypeFindValue:
		resp = h.handleFindValue(ctx, req)
	case MessageTypeStore:
		resp = h.handleStore(ctx, req)
	case MessageTypePing:
		resp = h.handlePing(ctx, req)
	case MessageTypeAddProvider:
		resp = h.handleAddProvider(ctx, req)
	case MessageTypeGetProviders:
		resp = h.handleGetProviders(ctx, req)
	case MessageTypeRemoveProvider:
		resp = h.handleRemoveProvider(ctx, req)
	case MessageTypePutPeerRecord:
		resp = h.handlePutPeerRecord(ctx, req)
	case MessageTypeGetPeerRecord:
		resp = h.handleGetPeerRecord(ctx, req)
	default:
		resp = NewErrorResponse(req.RequestID, types.NodeID(h.dht.host.ID()), req.Type, "unknown message type")
	}

	// 发送响应
	if err := h.dht.network.writeMessage(stream, resp); err != nil {
		// 日志记录错误即可
		return
	}
}

// handleFindNode 处理 FIND_NODE 请求
func (h *Handler) handleFindNode(_ context.Context, req *Message) *Message {
	// 验证地址
	allowPrivate := h.dht.config != nil && h.dht.config.AllowPrivateAddrs
	validAddrs := h.filterValidAddrs(req.SenderAddrs, allowPrivate)

	if len(validAddrs) == 0 && len(req.SenderAddrs) > 0 {
		return NewErrorResponse(req.RequestID, types.NodeID(h.dht.host.ID()), req.Type, "no routable addresses")
	}

	// 更新路由表（使用过滤后的地址，或如果全部有效则使用原始地址）
	addrsToUse := validAddrs
	if len(addrsToUse) == 0 {
		addrsToUse = req.SenderAddrs
	}

	h.dht.routingTable.Add(&RoutingNode{
		ID:       req.Sender,
		Addrs:    addrsToUse,
		LastSeen: time.Now(),
	})

	// 查找最近的节点
	closerPeers := h.dht.routingTable.NearestPeers(req.Target, BucketSize)

	// 转换为 PeerRecord
	var peerRecords []PeerRecord
	for _, peer := range closerPeers {
		peerRecords = append(peerRecords, PeerRecord{
			ID:    peer.ID,
			Addrs: peer.Addrs,
		})
	}

	return NewFindNodeResponse(req.RequestID, types.NodeID(h.dht.host.ID()), peerRecords)
}

// handleFindValue 处理 FIND_VALUE 请求
func (h *Handler) handleFindValue(_ context.Context, req *Message) *Message {
	// 验证地址
	allowPrivate := h.dht.config != nil && h.dht.config.AllowPrivateAddrs
	validAddrs := h.filterValidAddrs(req.SenderAddrs, allowPrivate)

	if len(validAddrs) == 0 && len(req.SenderAddrs) > 0 {
		return NewErrorResponse(req.RequestID, types.NodeID(h.dht.host.ID()), req.Type, "no routable addresses")
	}

	// 更新路由表
	addrsToUse := validAddrs
	if len(addrsToUse) == 0 {
		addrsToUse = req.SenderAddrs
	}

	h.dht.routingTable.Add(&RoutingNode{
		ID:       req.Sender,
		Addrs:    addrsToUse,
		LastSeen: time.Now(),
	})

	// 查找值
	value, exists := h.dht.valueStore.Get(req.Key)
	if exists {
		return NewFindValueResponse(req.RequestID, types.NodeID(h.dht.host.ID()), value)
	}

	// 未找到，返回更近的节点
	keyHash := HashKey(req.Key)
	targetID := types.NodeID(string(keyHash))
	closerPeers := h.dht.routingTable.NearestPeers(targetID, BucketSize)

	var peerRecords []PeerRecord
	for _, peer := range closerPeers {
		peerRecords = append(peerRecords, PeerRecord{
			ID:    peer.ID,
			Addrs: peer.Addrs,
		})
	}

	return NewFindValueResponseWithPeers(req.RequestID, types.NodeID(h.dht.host.ID()), peerRecords)
}

// handleStore 处理 STORE 请求
func (h *Handler) handleStore(_ context.Context, req *Message) *Message {
	// 验证地址
	allowPrivate := h.dht.config != nil && h.dht.config.AllowPrivateAddrs
	validAddrs := h.filterValidAddrs(req.SenderAddrs, allowPrivate)

	if len(validAddrs) == 0 && len(req.SenderAddrs) > 0 {
		return NewErrorResponse(req.RequestID, types.NodeID(h.dht.host.ID()), req.Type, "no routable addresses")
	}

	// 更新路由表
	addrsToUse := validAddrs
	if len(addrsToUse) == 0 {
		addrsToUse = req.SenderAddrs
	}

	h.dht.routingTable.Add(&RoutingNode{
		ID:       req.Sender,
		Addrs:    addrsToUse,
		LastSeen: time.Now(),
	})

	// 存储值
	ttl := time.Duration(req.TTL) * time.Second
	h.dht.valueStore.Put(req.Key, req.Value, ttl)

	return NewStoreResponse(req.RequestID, types.NodeID(h.dht.host.ID()), true, "")
}

// handlePing 处理 PING 请求
func (h *Handler) handlePing(_ context.Context, req *Message) *Message {
	// 验证地址
	allowPrivate := h.dht.config != nil && h.dht.config.AllowPrivateAddrs
	validAddrs := h.filterValidAddrs(req.SenderAddrs, allowPrivate)

	if len(validAddrs) == 0 && len(req.SenderAddrs) > 0 {
		return NewErrorResponse(req.RequestID, types.NodeID(h.dht.host.ID()), req.Type, "no routable addresses")
	}

	// 更新路由表
	addrsToUse := validAddrs
	if len(addrsToUse) == 0 {
		addrsToUse = req.SenderAddrs
	}

	h.dht.routingTable.Add(&RoutingNode{
		ID:       req.Sender,
		Addrs:    addrsToUse,
		LastSeen: time.Now(),
	})

	//使用 AdvertisedAddrs 确保包含 Relay 地址
	localAddrs := h.dht.host.AdvertisedAddrs()
	return NewPingResponse(req.RequestID, types.NodeID(h.dht.host.ID()), localAddrs)
}

// handleAddProvider 处理 ADD_PROVIDER 请求
func (h *Handler) handleAddProvider(_ context.Context, req *Message) *Message {
	// 速率限制
	if !h.providerLimiter.Allow(req.Sender) {
		return NewErrorResponse(req.RequestID, types.NodeID(h.dht.host.ID()), req.Type, "rate limit exceeded")
	}

	// 验证地址
	allowPrivate := h.dht.config != nil && h.dht.config.AllowPrivateAddrs
	validAddrs := h.filterValidAddrs(req.SenderAddrs, allowPrivate)

	if len(validAddrs) == 0 && len(req.SenderAddrs) > 0 {
		return NewErrorResponse(req.RequestID, types.NodeID(h.dht.host.ID()), req.Type, "no routable addresses")
	}

	// 更新路由表
	addrsToUse := validAddrs
	if len(addrsToUse) == 0 {
		addrsToUse = req.SenderAddrs
	}

	h.dht.routingTable.Add(&RoutingNode{
		ID:       req.Sender,
		Addrs:    addrsToUse,
		LastSeen: time.Now(),
	})

	// 添加 Provider
	ttl := time.Duration(req.TTL) * time.Second
	h.dht.providerStore.AddProvider(req.Key, req.Sender, req.SenderAddrs, ttl)

	return NewAddProviderResponse(req.RequestID, types.NodeID(h.dht.host.ID()), true, "")
}

// handleGetProviders 处理 GET_PROVIDERS 请求
func (h *Handler) handleGetProviders(_ context.Context, req *Message) *Message {
	// 验证地址
	allowPrivate := h.dht.config != nil && h.dht.config.AllowPrivateAddrs
	validAddrs := h.filterValidAddrs(req.SenderAddrs, allowPrivate)

	if len(validAddrs) == 0 && len(req.SenderAddrs) > 0 {
		return NewErrorResponse(req.RequestID, types.NodeID(h.dht.host.ID()), req.Type, "no routable addresses")
	}

	// 更新路由表
	addrsToUse := validAddrs
	if len(addrsToUse) == 0 {
		addrsToUse = req.SenderAddrs
	}

	h.dht.routingTable.Add(&RoutingNode{
		ID:       req.Sender,
		Addrs:    addrsToUse,
		LastSeen: time.Now(),
	})

	// 获取 Providers
	providers := h.dht.providerStore.GetProviders(req.Key)

	// 转换为 PeerRecord
	var providerRecords []PeerRecord
	for _, p := range providers {
		providerRecords = append(providerRecords, PeerRecord{
			ID:    p.PeerID,
			Addrs: p.Addrs,
		})
	}

	// 查找更近的节点
	keyHash := HashKey(req.Key)
	targetID := types.NodeID(string(keyHash))
	closerPeers := h.dht.routingTable.NearestPeers(targetID, BucketSize)

	var closerRecords []PeerRecord
	for _, peer := range closerPeers {
		closerRecords = append(closerRecords, PeerRecord{
			ID:    peer.ID,
			Addrs: peer.Addrs,
		})
	}

	return NewGetProvidersResponse(req.RequestID, types.NodeID(h.dht.host.ID()), providerRecords, closerRecords)
}

// handleRemoveProvider 处理 REMOVE_PROVIDER 请求
func (h *Handler) handleRemoveProvider(_ context.Context, req *Message) *Message {
	// 验证地址
	allowPrivate := h.dht.config != nil && h.dht.config.AllowPrivateAddrs
	validAddrs := h.filterValidAddrs(req.SenderAddrs, allowPrivate)

	if len(validAddrs) == 0 && len(req.SenderAddrs) > 0 {
		return NewErrorResponse(req.RequestID, types.NodeID(h.dht.host.ID()), req.Type, "no routable addresses")
	}

	// 只允许移除自己的 Provider 记录
	if req.Sender != types.NodeID(h.dht.host.ID()) {
		h.dht.providerStore.RemoveProvider(req.Key, req.Sender)
	}

	return NewRemoveProviderResponse(req.RequestID, types.NodeID(h.dht.host.ID()), true, "")
}

// sendErrorResponse 发送错误响应
func (h *Handler) sendErrorResponse(w io.Writer, requestID uint64, sender types.NodeID, msgType MessageType, errMsg string) {
	resp := NewErrorResponse(requestID, sender, msgType, errMsg)
	_ = h.dht.network.writeMessage(w, resp)
}

// ============================================================================
//                              PeerRecord 处理器（v2.0 新增）
// ============================================================================

// handlePutPeerRecord 处理 PUT_PEER_RECORD 请求
//
// 验证流程：
//  1. 速率限制
//  2. 反序列化 SignedRealmPeerRecord
//  3. 验证签名
//  4. 验证 Key 匹配
//  5. 验证 TTL
//  6. 冲突解决（seq 更大的获胜）
//  7. 存储
func (h *Handler) handlePutPeerRecord(_ context.Context, req *Message) *Message {
	// 速率限制
	if !h.peerRecordLimiter.Allow(req.Sender) {
		return NewPutPeerRecordResponse(req.RequestID, types.NodeID(h.dht.host.ID()), false, "rate limit exceeded")
	}

	// 反序列化 SignedRealmPeerRecord
	if len(req.SignedRecord) == 0 {
		return NewPutPeerRecordResponse(req.RequestID, types.NodeID(h.dht.host.ID()), false, "empty signed record")
	}

	signed, err := UnmarshalSignedRealmPeerRecord(req.SignedRecord)
	if err != nil {
		return NewPutPeerRecordResponse(req.RequestID, types.NodeID(h.dht.host.ID()), false, fmt.Sprintf("unmarshal failed: %v", err))
	}

	// 存储（Put 方法会执行完整验证）
	replaced, err := h.dht.peerRecordStore.Put(req.Key, signed)
	if err != nil {
		return NewPutPeerRecordResponse(req.RequestID, types.NodeID(h.dht.host.ID()), false, fmt.Sprintf("store failed: %v", err))
	}

	// 更新路由表（使用 PeerRecord 中的地址）
	allowPrivate := h.dht.config != nil && h.dht.config.AllowPrivateAddrs
	allAddrs := signed.Record.AllAddrs()
	validAddrs := h.filterValidAddrs(allAddrs, allowPrivate)

	if len(validAddrs) > 0 {
		h.dht.routingTable.Add(&RoutingNode{
			ID:       signed.Record.NodeID,
			Addrs:    validAddrs,
			LastSeen: time.Now(),
		})
	}

	logger.Debug("PeerRecord 存储成功",
		"nodeID", signed.Record.NodeID,
		"seq", signed.Record.Seq,
		"replaced", replaced)

	return NewPutPeerRecordResponse(req.RequestID, types.NodeID(h.dht.host.ID()), true, "")
}

// handleGetPeerRecord 处理 GET_PEER_RECORD 请求
//
// 返回：
//   - 如果找到：返回 SignedRealmPeerRecord
//   - 如果未找到：返回更近的节点列表
func (h *Handler) handleGetPeerRecord(_ context.Context, req *Message) *Message {
	// 验证地址
	allowPrivate := h.dht.config != nil && h.dht.config.AllowPrivateAddrs
	validAddrs := h.filterValidAddrs(req.SenderAddrs, allowPrivate)

	if len(validAddrs) == 0 && len(req.SenderAddrs) > 0 {
		return NewErrorResponse(req.RequestID, types.NodeID(h.dht.host.ID()), req.Type, "no routable addresses")
	}

	// 更新路由表
	addrsToUse := validAddrs
	if len(addrsToUse) == 0 {
		addrsToUse = req.SenderAddrs
	}

	h.dht.routingTable.Add(&RoutingNode{
		ID:       req.Sender,
		Addrs:    addrsToUse,
		LastSeen: time.Now(),
	})

	// 查找 PeerRecord
	record, exists := h.dht.peerRecordStore.Get(req.Key)
	if exists {
		// 序列化
		signedBytes, err := record.Marshal()
		if err != nil {
			return NewErrorResponse(req.RequestID, types.NodeID(h.dht.host.ID()), req.Type, fmt.Sprintf("marshal failed: %v", err))
		}
		return NewGetPeerRecordResponse(req.RequestID, types.NodeID(h.dht.host.ID()), signedBytes)
	}

	// 未找到，返回更近的节点
	keyHash := HashKey(req.Key)
	targetID := types.NodeID(string(keyHash))
	closerPeers := h.dht.routingTable.NearestPeers(targetID, BucketSize)

	var peerRecords []PeerRecord
	for _, peer := range closerPeers {
		peerRecords = append(peerRecords, PeerRecord{
			ID:    peer.ID,
			Addrs: peer.Addrs,
		})
	}

	return NewGetPeerRecordResponseWithPeers(req.RequestID, types.NodeID(h.dht.host.ID()), peerRecords)
}
