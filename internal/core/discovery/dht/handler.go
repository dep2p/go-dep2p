// Package dht 提供分布式哈希表实现
package dht

import (
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Layer1 验证错误
// ============================================================================

var (
	// errNodeIDMismatch NodeID 不匹配（发送者尝试发布他人的记录）
	errNodeIDMismatch = errors.New("node ID mismatch: sender cannot publish records for other nodes")

	// errSenderMismatch 发送者身份不匹配（消息体声称的 Sender 与连接真实远端不一致）
	// Layer1 安全：防止伪造 sender 绕过限流/投毒路由表
	errSenderMismatch = errors.New("sender identity mismatch: claimed sender does not match connection remote peer")

	// errRecordExpired 记录已过期
	errRecordExpired = errors.New("record has expired")

	// errSeqnoRollback seqno 回滚（新记录的 seqno 不大于已存在的记录）
	errSeqnoRollback = errors.New("seqno rollback: new record is not newer than existing")

	// errRateLimitExceeded 速率限制超限
	errRateLimitExceeded = errors.New("rate limit exceeded")

	// errInvalidAddress 无效地址
	errInvalidAddress = errors.New("invalid address format")

	// errUnroutableAddress 不可路由地址（私网/回环/未指定）
	errUnroutableAddress = errors.New("unroutable address (private/loopback/unspecified)")

	// errInvalidPort 无效端口
	errInvalidPort = errors.New("invalid port number")

	// errNotMultiaddr 非 multiaddr 格式
	// Layer1 要求：DHT 只接受规范 multiaddr 格式，拒绝 host:port 等非标准格式
	errNotMultiaddr = errors.New("address must be multiaddr format (e.g. /ip4/.../tcp/..., /dns4/.../tcp/...)")

	// errMissingTransport 缺少传输协议
	errMissingTransport = errors.New("multiaddr must include transport protocol (tcp/udp/quic/quic-v1)")
)

// ============================================================================
//                              速率限制器
// ============================================================================

// Layer1 设计要求（文档第 579-583 行）：
// - 单节点每分钟最多发布 10 条 PeerRecord 更新
// - 单节点每分钟最多发布 50 条 ProviderRecord
const (
	// PeerRecordRateLimit PeerRecord 速率限制（每分钟）
	PeerRecordRateLimit = 10

	// ProviderRecordRateLimit ProviderRecord 速率限制（每分钟）
	ProviderRecordRateLimit = 50

	// RateLimitWindow 速率限制窗口
	RateLimitWindow = time.Minute

	// RateLimitCleanupInterval 清理间隔
	RateLimitCleanupInterval = 5 * time.Minute
)

// rateLimiter 速率限制器
//
// 使用滑动窗口计数器实现 per-sender 速率限制
type rateLimiter struct {
	mu sync.RWMutex

	// peerRecordCounts PeerRecord 请求计数（per sender）
	peerRecordCounts map[types.NodeID]*requestCounter

	// providerCounts Provider 请求计数（per sender）
	providerCounts map[types.NodeID]*requestCounter
}

// requestCounter 请求计数器
type requestCounter struct {
	count     int
	windowEnd time.Time
}

// newRateLimiter 创建速率限制器
func newRateLimiter() *rateLimiter {
	rl := &rateLimiter{
		peerRecordCounts: make(map[types.NodeID]*requestCounter),
		providerCounts:   make(map[types.NodeID]*requestCounter),
	}
	go rl.cleanupLoop()
	return rl
}

// allowPeerRecord 检查是否允许 PeerRecord 请求
func (rl *rateLimiter) allowPeerRecord(sender types.NodeID) bool {
	return rl.allow(sender, rl.peerRecordCounts, PeerRecordRateLimit)
}

// allowProvider 检查是否允许 Provider 请求
func (rl *rateLimiter) allowProvider(sender types.NodeID) bool {
	return rl.allow(sender, rl.providerCounts, ProviderRecordRateLimit)
}

// allow 通用速率检查
func (rl *rateLimiter) allow(sender types.NodeID, counts map[types.NodeID]*requestCounter, limit int) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	counter, exists := counts[sender]

	if !exists || now.After(counter.windowEnd) {
		// 新窗口或旧窗口已过期
		counts[sender] = &requestCounter{
			count:     1,
			windowEnd: now.Add(RateLimitWindow),
		}
		return true
	}

	// 检查是否超限
	if counter.count >= limit {
		return false
	}

	counter.count++
	return true
}

// cleanupLoop 定期清理过期的计数器
func (rl *rateLimiter) cleanupLoop() {
	ticker := time.NewTicker(RateLimitCleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		rl.cleanup()
	}
}

// cleanup 清理过期计数器
func (rl *rateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	for sender, counter := range rl.peerRecordCounts {
		if now.After(counter.windowEnd) {
			delete(rl.peerRecordCounts, sender)
		}
	}

	for sender, counter := range rl.providerCounts {
		if now.After(counter.windowEnd) {
			delete(rl.providerCounts, sender)
		}
	}
}

// ============================================================================
//                              Handler
// ============================================================================

// Handler DHT 协议处理器
//
// 处理入站 DHT 请求，调用 DHT 本地逻辑后响应
type Handler struct {
	// dht DHT 实例
	dht *DHT

	// rateLimiter 速率限制器（Layer1 防投毒/资源耗尽）
	rateLimiter *rateLimiter
}

// NewHandler 创建 DHT 协议处理器
func NewHandler(dht *DHT) *Handler {
	return &Handler{
		dht:         dht,
		rateLimiter: newRateLimiter(),
	}
}

// Handle 处理入站流
//
// 实现 endpoint.ProtocolHandler
//
// Layer1 安全设计：
// - 使用 stream.Connection().RemoteID() 获取真实远端身份
// - 拒绝 req.Sender 与真实远端不一致的请求（防止伪造 sender 绕过限流/投毒路由表）
// - 若 req.Sender 为空，自动用真实远端覆盖
func (h *Handler) Handle(stream endpoint.Stream) {
	defer func() { _ = stream.Close() }()

	// Layer1 安全：获取真实的远端节点 ID（来自 QUIC 连接的加密握手）
	conn := stream.Connection()
	if conn == nil {
		log.Warn("DHT 请求无法获取连接信息")
		return
	}
	trustedRemoteID := conn.RemoteID()

	// 读取请求
	reqData, err := readFrame(stream)
	if err != nil {
		log.Debug("读取 DHT 请求失败", "err", err)
		return
	}

	// 解码请求
	req, err := DecodeMessage(reqData)
	if err != nil {
		log.Debug("解码 DHT 请求失败", "err", err)
		return
	}

	// Layer1 安全：验证/绑定 sender 身份
	// - 若 req.Sender 为空：用真实远端覆盖
	// - 若 req.Sender 与真实远端不一致：拒绝请求
	if req.Sender.IsEmpty() {
		// 空 sender，用真实远端覆盖
		req.Sender = trustedRemoteID
		log.Debug("DHT 请求 Sender 为空，已用真实远端覆盖",
			"trustedRemoteID", trustedRemoteID.ShortString())
	} else if !req.Sender.Equal(trustedRemoteID) {
		// sender 与真实远端不一致，拒绝请求
		log.Warn("DHT 请求 Sender 身份伪造，拒绝处理",
			"claimedSender", req.Sender.ShortString(),
			"trustedRemoteID", trustedRemoteID.ShortString(),
			"requestType", req.Type.String())
		// 发送错误响应
		resp := NewErrorResponse(req.RequestID, h.dht.localID, req.Type, errSenderMismatch.Error())
		if respData, err := resp.Encode(); err == nil {
			_ = writeFrame(stream, respData)
		}
		return
	}

	log.Debug("收到 DHT 请求",
		"type", req.Type.String(),
		"requestID", req.RequestID,
		"sender", req.Sender.ShortString(),
		"verified", "true")

	// 更新路由表（发送者信息）- 已通过身份验证
	if !req.Sender.IsEmpty() && len(req.SenderAddrs) > 0 {
		node := &RoutingNode{
			ID:       req.Sender,
			Addrs:    req.SenderAddrs,
			LastSeen: time.Now(),
			RealmID:  h.dht.realmID,
		}
		_ = h.dht.routingTable.Update(node) // 忽略 bucket full 错误
	}

	// 处理请求
	var resp *Message
	switch req.Type {
	case MessageTypeFindNode:
		resp = h.handleFindNode(req)
	case MessageTypeFindValue:
		resp = h.handleFindValue(req)
	case MessageTypeStore:
		resp = h.handleStore(req)
	case MessageTypePing:
		resp = h.handlePing(req)
	case MessageTypeAddProvider:
		resp = h.handleAddProvider(req)
	case MessageTypeGetProviders:
		resp = h.handleGetProviders(req)
	case MessageTypeRemoveProvider:
		resp = h.handleRemoveProvider(req)
	default:
		log.Warn("未知的 DHT 消息类型", "type", req.Type)
		resp = NewErrorResponse(req.RequestID, h.dht.localID, req.Type, "unknown message type")
	}

	// 编码响应
	respData, err := resp.Encode()
	if err != nil {
		log.Debug("编码 DHT 响应失败", "err", err)
		return
	}

	// 发送响应
	if err := writeFrame(stream, respData); err != nil {
		log.Debug("发送 DHT 响应失败", "err", err)
		return
	}

	log.Debug("发送 DHT 响应",
		"type", resp.Type.String(),
		"requestID", resp.RequestID,
		"success", resp.Success)
}

// handleFindNode 处理 FIND_NODE 请求
func (h *Handler) handleFindNode(req *Message) *Message {
	// 查找最近的节点
	closestPeers := h.dht.routingTable.NearestPeers(req.Target[:], h.dht.config.BucketSize)

	// 转换为 PeerRecord
	records := make([]PeerRecord, 0, len(closestPeers))
	for _, id := range closestPeers {
		node := h.dht.routingTable.Find(id)
		if node != nil {
			records = append(records, PeerRecord{
				ID:    node.ID,
				Addrs: node.Addrs,
			})
		}
	}

	return NewFindNodeResponse(req.RequestID, h.dht.localID, records)
}

// handleFindValue 处理 FIND_VALUE 请求
//
// T2 修复：存储和距离计算使用 SHA256(key)
func (h *Handler) handleFindValue(req *Message) *Message {
	// T2: 使用哈希后的 key 进行本地存储查找
	hashedKey := HashKeyString(req.Key)

	// 先检查本地是否有值
	h.dht.storeMu.RLock()
	stored, ok := h.dht.store[hashedKey]
	h.dht.storeMu.RUnlock()

	if ok && time.Since(stored.Timestamp) < stored.TTL {
		// 找到值，直接返回
		return NewFindValueResponse(req.RequestID, h.dht.localID, stored.Value)
	}

	// T2: 距离计算使用哈希后的 key bytes
	keyBytes := HashKey(req.Key)
	closestPeers := h.dht.routingTable.NearestPeers(keyBytes, h.dht.config.BucketSize)

	records := make([]PeerRecord, 0, len(closestPeers))
	for _, id := range closestPeers {
		node := h.dht.routingTable.Find(id)
		if node != nil {
			records = append(records, PeerRecord{
				ID:    node.ID,
				Addrs: node.Addrs,
			})
		}
	}

	return NewFindValueResponseWithPeers(req.RequestID, h.dht.localID, records)
}

// handleStore 处理 STORE 请求
//
// T2 修复：存储使用 SHA256(key)
// Layer1 修复：
// - TTL 上限约束（≤24h）
// - sys/peer/* 记录需验证签名/NodeID/过期/seqno
// - 速率限制（PeerRecord 10/min）
func (h *Handler) handleStore(req *Message) *Message {
	// 验证参数
	if req.Key == "" {
		return NewStoreResponse(req.RequestID, h.dht.localID, false, "empty key")
	}

	// Layer1 速率限制：对 sys/peer/* 记录检查速率
	if strings.HasPrefix(req.Key, PeerRecordKeyPrefix) {
		if !h.rateLimiter.allowPeerRecord(req.Sender) {
			log.Warn("PeerRecord 速率限制超限",
				"sender", req.Sender.ShortString(),
				"limit", PeerRecordRateLimit)
			return NewStoreResponse(req.RequestID, h.dht.localID, false, errRateLimitExceeded.Error())
		}
	}

	// 计算 TTL（强制上限约束）
	ttl := time.Duration(req.TTL) * time.Second
	if ttl <= 0 {
		ttl = h.dht.config.MaxRecordAge
	}
	// Layer1: TTL 上限约束，防止过长缓存导致撤销不及时
	if ttl > h.dht.config.MaxRecordAge {
		ttl = h.dht.config.MaxRecordAge
	}

	// T2: 使用哈希后的 key 进行存储
	hashedKey := HashKeyString(req.Key)

	// Layer1: 对 sys/peer/* 记录进行特殊验证（防投毒/防回滚）
	if strings.HasPrefix(req.Key, PeerRecordKeyPrefix) {
		if err := h.validatePeerRecordStore(req); err != nil {
			log.Warn("拒绝无效的 PeerRecord STORE",
				"key", req.Key,
				"sender", req.Sender.ShortString(),
				"err", err)
			return NewStoreResponse(req.RequestID, h.dht.localID, false, "peer record validation failed: "+err.Error())
		}
	}

	// 存储值
	h.dht.storeMu.Lock()
	h.dht.store[hashedKey] = storedValue{
		Value:     req.Value,
		Provider:  req.Sender,
		Timestamp: time.Now(),
		TTL:       ttl,
	}
	h.dht.storeMu.Unlock()

	log.Debug("存储值",
		"key", req.Key,
		"hashedKey", hashedKey[:16]+"...",
		"size", len(req.Value),
		"ttl", ttl,
		"from", req.Sender.ShortString())

	return NewStoreResponse(req.RequestID, h.dht.localID, true, "")
}

// ============================================================================
//                              地址校验
// ============================================================================

// validateAddresses 验证地址列表
//
// Layer1 设计要求（文档第 572-576 行）：
// - 必须是有效的 multiaddr 格式
// - 拒绝：0.0.0.0, ::, 127.0.0.1, 私网地址
// - 拒绝：无效端口（0, >65535）
func validateAddresses(addrs []string) error {
	if len(addrs) == 0 {
		return errInvalidAddress
	}

	for _, addr := range addrs {
		if err := validateAddress(addr); err != nil {
			return err
		}
	}
	return nil
}

// validateAddress 验证单个地址
//
// Layer1 严格策略（multiaddr-only）：
// - 必须是 multiaddr 格式，以 / 开头
// - 允许的协议前缀：/ip4/, /ip6/, /dns4/, /dns6/, /dnsaddr/
// - 必须包含传输协议：tcp/udp/quic/quic-v1
// - Relay circuit 地址特殊处理：/p2p/<relay>/p2p-circuit/p2p/<target>
// - 拒绝：host:port 文本格式、纯主机名、随机字符串
func validateAddress(addr string) error {
	if addr == "" {
		return errInvalidAddress
	}

	// Layer1 严格策略：必须是 multiaddr 格式（以 / 开头）
	if !strings.HasPrefix(addr, "/") {
		return errNotMultiaddr
	}

	// Relay circuit 地址特殊处理：优先检查 /p2p-circuit/
	// 无论以什么协议开头，只要包含 /p2p-circuit/ 就是有效的 relay 地址
	if strings.Contains(addr, "/p2p-circuit/") {
		return nil
	}

	var host string
	var portStr string
	var hasTransport bool

	// 解析 multiaddr 格式
	switch {
	case strings.HasPrefix(addr, "/ip4/") || strings.HasPrefix(addr, "/ip6/"):
		// IP 地址格式: /ip4/x.x.x.x/tcp/port 或 /ip6/.../tcp/port
		host, portStr, hasTransport = parseMultiaddrStrict(addr)
	case strings.HasPrefix(addr, "/dns4/") || strings.HasPrefix(addr, "/dns6/") || strings.HasPrefix(addr, "/dnsaddr/"):
		// DNS 地址格式: /dns4/example.com/tcp/port
		_, portStr, hasTransport = parseMultiaddrStrict(addr)
		// DNS 地址不验证 IP（由 DNS 解析器处理）
	case strings.HasPrefix(addr, "/p2p/"):
		// 纯 /p2p/<nodeID> 不是有效的可拨号地址（没有传输协议）
		// 注意：带有 /p2p-circuit/ 的已在上面处理
		return errMissingTransport
	default:
		// 未知的 multiaddr 协议前缀
		return errNotMultiaddr
	}

	// Layer1 严格策略：必须包含传输协议
	if !hasTransport {
		return errMissingTransport
	}

	// 验证 IP 地址（仅对 /ip4/, /ip6/）
	if host != "" {
		ip := net.ParseIP(host)
		if ip != nil {
			// 拒绝不可路由地址
			if ip.IsUnspecified() { // 0.0.0.0 or ::
				return errUnroutableAddress
			}
			if ip.IsLoopback() { // 127.0.0.1 or ::1
				return errUnroutableAddress
			}
			if ip.IsPrivate() { // 私网地址
				return errUnroutableAddress
			}
		}
	}

	// 验证端口
	if portStr != "" {
		port, err := strconv.Atoi(portStr)
		if err != nil || port <= 0 || port > 65535 {
			return errInvalidPort
		}
	}

	return nil
}

// parseMultiaddrStrict 解析 multiaddr 格式，提取 host、port 和传输协议标志
//
// 支持格式：
// - /ip4/x.x.x.x/tcp/port, /ip6/.../tcp/port
// - /dns4/example.com/tcp/port, /dns6/example.com/tcp/port
// - /dnsaddr/example.com
//
// 返回：
// - host: IP 地址或域名
// - port: 端口号
// - hasTransport: 是否包含传输协议（tcp/udp/quic/quic-v1）
func parseMultiaddrStrict(addr string) (host, port string, hasTransport bool) {
	parts := strings.Split(addr, "/")
	// /ip4/127.0.0.1/tcp/4001 -> ["", "ip4", "127.0.0.1", "tcp", "4001"]
	// /ip6/::1/tcp/4001 -> ["", "ip6", "::1", "tcp", "4001"]
	// /dns4/example.com/tcp/4001 -> ["", "dns4", "example.com", "tcp", "4001"]

	for i := 0; i < len(parts)-1; i++ {
		switch parts[i] {
		case "ip4", "ip6":
			if i+1 < len(parts) {
				host = parts[i+1]
			}
		case "dns4", "dns6", "dnsaddr":
			if i+1 < len(parts) {
				host = parts[i+1]
			}
		case "tcp", "udp", "quic", "quic-v1":
			hasTransport = true
			if i+1 < len(parts) {
				port = parts[i+1]
			}
		}
	}
	return
}

// validatePeerRecordStore 验证 sys/peer/* 记录的 STORE 请求
//
// Layer1 安全约束：
// 1. 解码为 SignedPeerRecord
// 2. 验证签名（VerifySelf）
// 3. 验证 NodeID 与发送者匹配（只能发布自己的记录）
// 4. 验证未过期
// 5. 验证地址格式（拒绝私网/回环/未指定/无效端口）
// 6. 验证 seqno 单调递增（防回滚）
func (h *Handler) validatePeerRecordStore(req *Message) error {
	// Layer1 安全约束：sys/peer 必须使用 SignedPeerRecord，不允许无签名降级
	// 这确保了地址索引的抗投毒能力

	// 1. 解码为 SignedPeerRecord（不降级到 ProviderRecord）
	record, err := DecodeSignedPeerRecord(req.Value)
	if err != nil {
		return errors.New("sys/peer requires SignedPeerRecord: " + err.Error())
	}

	// 2. 验证签名
	if err := record.VerifySelf(); err != nil {
		return err
	}

	// 3. 验证 NodeID 与发送者匹配（只能发布自己的记录）
	if record.NodeID != req.Sender {
		return errNodeIDMismatch
	}

	// 4. 验证未过期
	if record.IsExpired() {
		return errRecordExpired
	}

	// 5. 验证地址格式（Layer1: 拒绝不可路由地址）
	if err := validateAddresses(record.Addrs); err != nil {
		return err
	}

	// 5. 验证 seqno 单调递增（防回滚攻击）
	hashedKey := HashKeyString(req.Key)
	h.dht.storeMu.RLock()
	existing, exists := h.dht.store[hashedKey]
	h.dht.storeMu.RUnlock()

	if exists {
		existingRecord, err := DecodeSignedPeerRecord(existing.Value)
		if err == nil && existingRecord != nil {
			if !record.IsNewerThan(existingRecord) {
				return errSeqnoRollback
			}
		}
	}

	return nil
}

// handlePing 处理 PING 请求
func (h *Handler) handlePing(req *Message) *Message {
	return NewPingResponse(req.RequestID, h.dht.localID, h.dht.localAddrs)
}

// handleAddProvider 处理 ADD_PROVIDER 请求
//
// T2 修复：存储使用 SHA256(key)
// Layer1 修复：
// - TTL 上限约束（≤24h）
// - 速率限制（Provider 50/min）
func (h *Handler) handleAddProvider(req *Message) *Message {
	if req.Key == "" {
		return NewAddProviderResponse(req.RequestID, h.dht.localID, false, "empty key")
	}

	// Layer1 速率限制：检查 Provider 请求速率
	if !h.rateLimiter.allowProvider(req.Sender) {
		log.Warn("Provider 速率限制超限",
			"sender", req.Sender.ShortString(),
			"limit", ProviderRecordRateLimit)
		return NewAddProviderResponse(req.RequestID, h.dht.localID, false, errRateLimitExceeded.Error())
	}

	// T2: 使用哈希后的 key 进行存储
	hashedKey := HashKeyString(req.Key)

	ttl := time.Duration(req.TTL) * time.Second
	if ttl <= 0 {
		ttl = DefaultProviderTTL
	}
	// Layer1: TTL 上限约束，防止过长缓存导致撤销不及时
	if ttl > DefaultProviderTTL {
		ttl = DefaultProviderTTL
	}

	// 添加 provider
	h.dht.addProviderLocal(hashedKey, req.Sender, req.SenderAddrs, ttl)

	log.Debug("添加 provider",
		"key", req.Key,
		"hashedKey", hashedKey[:16]+"...",
		"ttl", ttl,
		"provider", req.Sender.ShortString())

	return NewAddProviderResponse(req.RequestID, h.dht.localID, true, "")
}

// handleGetProviders 处理 GET_PROVIDERS 请求
//
// T2 修复：存储和距离计算使用 SHA256(key)
func (h *Handler) handleGetProviders(req *Message) *Message {
	if req.Key == "" {
		return NewErrorResponse(req.RequestID, h.dht.localID, MessageTypeGetProviders, "empty key")
	}

	// T2: 使用哈希后的 key 进行存储查找
	hashedKey := HashKeyString(req.Key)

	// 获取本地存储的 providers
	providers := h.dht.getProvidersLocal(hashedKey)

	// 转换为 PeerRecord
	providerRecords := make([]PeerRecord, len(providers))
	for i, p := range providers {
		providerRecords[i] = PeerRecord{
			ID:        p.ID,
			Addrs:     p.Addrs,
			Timestamp: p.Timestamp.UnixNano(),
			TTL:       uint32(p.TTL.Seconds()),
		}
	}

	// T2: 距离计算使用哈希后的 key bytes
	keyBytes := HashKey(req.Key)
	closestPeers := h.dht.routingTable.NearestPeers(keyBytes, h.dht.config.BucketSize)

	closerRecords := make([]PeerRecord, 0, len(closestPeers))
	for _, id := range closestPeers {
		node := h.dht.routingTable.Find(id)
		if node != nil {
			closerRecords = append(closerRecords, PeerRecord{
				ID:    node.ID,
				Addrs: node.Addrs,
			})
		}
	}

	return NewGetProvidersResponse(req.RequestID, h.dht.localID, providerRecords, closerRecords)
}

// handleRemoveProvider 处理 REMOVE_PROVIDER 请求
//
// best-effort 撤销：仅允许撤销发送者自身的 provider 记录。
func (h *Handler) handleRemoveProvider(req *Message) *Message {
	if req.Key == "" {
		return NewRemoveProviderResponse(req.RequestID, h.dht.localID, false, "empty key")
	}

	// T2: 使用哈希后的 key 进行存储
	hashedKey := HashKeyString(req.Key)
	h.dht.removeProviderLocal(hashedKey, req.Sender)

	log.Debug("移除 provider",
		"key", req.Key,
		"hashedKey", hashedKey[:16]+"...",
		"provider", req.Sender.ShortString())

	return NewRemoveProviderResponse(req.RequestID, h.dht.localID, true, "")
}

// HandlerFunc 返回 endpoint.ProtocolHandler 函数
func (h *Handler) HandlerFunc() endpoint.ProtocolHandler {
	return func(stream endpoint.Stream) {
		h.Handle(stream)
	}
}

