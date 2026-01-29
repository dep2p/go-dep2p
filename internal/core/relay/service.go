package relay

import (
	"context"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/relay/client"
	"github.com/dep2p/go-dep2p/internal/core/relay/server"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ════════════════════════════════════════════════════════════════════════════
// RelayService - 统一中继服务（v2.0）
// ════════════════════════════════════════════════════════════════════════════
//
// v2.0 统一 Relay 架构：
// - 三大职责：缓存加速 + 打洞协调 + 数据保底
// - Relay 只负责消息转发，不依赖 Realm 实现
// - 遵循高内聚低耦合和依赖倒置原则
//
// 设计文档：design/_discussions/20260123-nat-relay-concept-clarification.md §9.0
// ════════════════════════════════════════════════════════════════════════════

// RelayService 统一中继服务
type RelayService struct {
	addr  types.Multiaddr // 配置的中继地址
	state RelayState      // 连接状态

	swarm pkgif.Swarm
	host  pkgif.Host // 用于注册协议处理器

	client  *client.Client // 中继客户端
	server  *server.Server // 中继服务端
	limiter *RelayLimiter  // 统一限流器

	// v2.0 新增：外部配置支持
	serverConfig *Config // 外部服务端配置（如果设置则覆盖默认值）

	// 能力状态
	serverEnabled atomic.Bool // 是否启用为 Relay 服务端

	mu sync.RWMutex
}

// NewRelayService 创建统一 Relay 服务
//
// 参数:
//   - swarm: 网络 Swarm
//   - host: 网络 Host
func NewRelayService(swarm pkgif.Swarm, host pkgif.Host) (*RelayService, error) {
	return &RelayService{
		state: RelayStateNone,
		swarm: swarm,
		host:  host,
	}, nil
}

// SetHost 设置 Host（用于注册协议处理器）
func (s *RelayService) SetHost(host pkgif.Host) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.host = host
}

// SetServerConfig 设置服务端配置
//
// v2.0 新增：允许注入外部配置，覆盖内置默认值
// 应在调用 Enable() 之前设置
func (s *RelayService) SetServerConfig(config *Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.serverConfig = config
}

// ════════════════════════════════════════════════════════════════════════════
// 中继配置 API
// ════════════════════════════════════════════════════════════════════════════

// SetRelay 设置中继地址（配置，不连接）
func (s *RelayService) SetRelay(addr types.Multiaddr) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	logger.Debug("配置 Relay 地址", "addr", addr)

	// 验证地址
	if addr == nil || addr.String() == "" {
		logger.Warn("Relay 地址无效", "reason", "nil or empty")
		return ErrInvalidRelayAddress
	}

	// 提取节点 ID
	relayID := extractNodeID(addr)
	if relayID == "" {
		logger.Warn("Relay 地址无效", "reason", "no peer ID in addr", "addr", addr)
		return ErrInvalidRelayAddress
	}

	// 自身过滤（不能中继到自己）
	if s.swarm != nil && relayID == s.swarm.LocalPeer() {
		logger.Warn("Relay 配置失败", "reason", "cannot relay to self", "relayID", relayID)
		return ErrCannotRelayToSelf
	}

	// 如果已连接，先断开（替换旧配置）
	if s.state == RelayStateConnected {
		logger.Debug("断开旧 Relay 连接")
		s.disconnect()
	}

	// 保存配置（不连接）
	s.addr = addr
	s.state = RelayStateConfigured

	relayIDShort := relayID
	if len(relayIDShort) > 8 {
		relayIDShort = relayIDShort[:8]
	}
	logger.Info("Relay 已配置", "relayID", relayIDShort, "addr", addr)
	return nil
}

// RemoveRelay 移除中继配置
func (s *RelayService) RemoveRelay() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 如果已连接，先断开
	if s.state == RelayStateConnected {
		s.disconnect()
	}

	s.addr = nil
	s.state = RelayStateNone

	return nil
}

// Relay 获取中继配置
func (s *RelayService) Relay() (types.Multiaddr, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.addr == nil {
		return nil, false
	}

	return s.addr, true
}

// State 获取状态
func (s *RelayService) State() RelayState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// ════════════════════════════════════════════════════════════════════════════
// 连接管理
// ════════════════════════════════════════════════════════════════════════════

// Connect 连接中继（按需连接）
func (s *RelayService) Connect(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查配置
	if s.addr == nil {
		logger.Debug("Relay 连接失败", "reason", "no relay configured")
		return ErrNoRelayAvailable
	}

	// 已连接
	if s.state == RelayStateConnected {
		logger.Debug("Relay 已连接，跳过")
		return nil
	}

	// 正在连接
	if s.state == RelayStateConnecting {
		logger.Debug("Relay 正在连接中，跳过")
		return nil
	}

	// 开始连接
	s.state = RelayStateConnecting

	// 提取中继节点 ID
	relayID := extractNodeID(s.addr)
	if relayID == "" {
		s.state = RelayStateFailed
		logger.Error("Relay 连接失败", "reason", "invalid relay address", "addr", s.addr)
		return ErrInvalidRelayAddress
	}

	relayIDShort := relayID
	if len(relayIDShort) > 8 {
		relayIDShort = relayIDShort[:8]
	}
	logger.Info("开始连接 Relay", "relayID", relayIDShort, "addr", s.addr)

	// 创建中继客户端
	s.client = client.NewClient(s.swarm, types.PeerID(relayID), s.addr)

	// 发送 RESERVE 消息并等待响应
	startTime := time.Now()
	_, err := s.client.Reserve(ctx)
	if err != nil {
		s.state = RelayStateFailed
		s.client = nil
		// 对于资源限制错误，使用 DEBUG 级别，避免日志文件过大
		// 资源限制是正常的业务逻辑，不应该记录为 ERROR
		if strings.Contains(err.Error(), "resource limit exceeded") {
			logger.Debug("Relay 预约失败（资源限制）",
				"relayID", relayIDShort,
				"error", err,
				"duration", time.Since(startTime))
		} else {
			logger.Error("Relay 预约失败",
				"relayID", relayIDShort,
				"error", err,
				"duration", time.Since(startTime))
		}
		return err
	}

	s.state = RelayStateConnected

	logger.Info("Relay 连接成功",
		"relayID", relayIDShort,
		"duration", time.Since(startTime))
	return nil
}

// Disconnect 断开中继连接
func (s *RelayService) Disconnect() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.disconnect()
}

// disconnect 内部断开连接（不加锁）
func (s *RelayService) disconnect() error {
	if s.state != RelayStateConnected {
		return nil
	}

	// 关闭中继客户端
	if s.client != nil {
		s.client.Close()
		s.client = nil
	}

	s.state = RelayStateConfigured // 保留配置

	return nil
}

// IsConnected 检查是否已连接
func (s *RelayService) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state == RelayStateConnected
}

// DialViaRelay 通过中继拨号
func (s *RelayService) DialViaRelay(ctx context.Context, target string) (pkgif.Connection, error) {
	targetShort := target
	if len(targetShort) > 8 {
		targetShort = targetShort[:8]
	}

	s.mu.RLock()
	hasRelay := s.addr != nil
	relayAddr := s.addr
	s.mu.RUnlock()

	if !hasRelay {
		logger.Warn("Relay 未配置，无法中继", "target", targetShort)
		return nil, ErrNoRelayAvailable
	}

	// 按需连接
	if !s.IsConnected() {
		if err := s.Connect(ctx); err != nil {
			logger.Warn("Relay 连接失败，无法中继", "target", targetShort, "error", err)
			return nil, err
		}
	}

	s.mu.RLock()
	relayClient := s.client
	s.mu.RUnlock()

	if relayClient == nil {
		logger.Warn("Relay 客户端不可用", "target", targetShort)
		return nil, ErrNoRelayAvailable
	}

	// 通过中继客户端拨号
	startTime := time.Now()
	conn, err := relayClient.Connect(ctx, types.PeerID(target))
	if err != nil {
		logger.Error("Relay 中继拨号失败",
			"target", targetShort,
			"relay", relayAddr,
			"error", err,
			"duration", time.Since(startTime))
		return nil, err
	}

	logger.Info("Relay 中继拨号成功",
		"target", targetShort,
		"relay", relayAddr,
		"duration", time.Since(startTime))
	return conn, nil
}

// ════════════════════════════════════════════════════════════════════════════
// 能力开关 API
// ════════════════════════════════════════════════════════════════════════════

// Enable 启用 Relay 能力
//
// 将当前节点设置为中继服务器，为 NAT 后的节点提供中继服务。
//
// 前置条件：
//   - 节点必须有公网可达地址（非 NAT 后）
//
// v2.0 改进：优先使用外部配置，回退到内置默认值
func (s *RelayService) Enable(_ context.Context) error {
	// 幂等检查
	if s.serverEnabled.Load() {
		return nil
	}

	// 前置条件检查：公网可达性
	if !s.isPubliclyReachable() {
		return ErrNotPubliclyReachable
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 如果已经有服务器，直接返回
	if s.server != nil {
		s.serverEnabled.Store(true)
		return nil
	}

	// v2.0 改进：优先使用外部配置，回退到内置默认值
	cfg := s.serverConfig
	if cfg == nil {
		cfg = DefaultConfig()
	}

	// 创建统一限流器（使用配置值）
	s.limiter = NewRelayLimiter(RelayLimiterConfig{
		MaxBandwidth:          cfg.MaxBandwidth,
		MaxConnections:        cfg.MaxCircuits,
		MaxConnectionsPerPeer: cfg.MaxCircuitsPerPeer,
		IdleTimeout:           GetRelayDefaults().IdleTimeout, // IdleTimeout 仍使用内置默认值
	})

	// 创建 RelayServer（使用配置适配器）
	serverLimiter := newServerLimiterFromConfig(cfg)
	s.server = server.NewServer(s.swarm, serverLimiter)

	// 设置 Host 用于注册协议处理器
	if s.host == nil {
		logger.Error("RelayService.host 为 nil，协议处理器将不会被注册！")
	} else {
		logger.Info("正在设置 Relay Server Host，将注册协议处理器")
	}
	s.server.SetHost(s.host)

	// 启动服务端
	if err := s.server.Start(); err != nil {
		s.server = nil
		return err
	}

	// 计算实际生效的 maxCircuitsPerPeer（用于日志，0 表示不限制）
	effectiveMaxCircuitsPerPeer := cfg.MaxCircuitsPerPeer
	effectiveMaxCircuitsPerPeerUnlimited := effectiveMaxCircuitsPerPeer == 0
	effectiveMaxCircuitsTotalUnlimited := cfg.MaxCircuits == 0

	logger.Info("Relay Server 已启动",
		"maxReservations", cfg.MaxReservations,
		"maxCircuitsTotal", cfg.MaxCircuits,
		"maxCircuitsPerPeer", effectiveMaxCircuitsPerPeer,
		"maxCircuitsPerPeerUnlimited", effectiveMaxCircuitsPerPeerUnlimited,
		"maxCircuitsTotalUnlimited", effectiveMaxCircuitsTotalUnlimited,
		"reservationTTL", cfg.ReservationTTL)

	s.serverEnabled.Store(true)
	return nil
}

// newServerLimiterFromConfig 从配置创建服务端限流器
//
// 语义处理：外部配置中 0 表示"不限制"
func newServerLimiterFromConfig(cfg *Config) server.Limiter {
	defaults := RelayDefaults{
		MaxBandwidth:       cfg.MaxBandwidth,
		MaxDuration:        cfg.MaxDuration,
		MaxDataPerConn:     0, // 暂不支持
		MaxReservations:    cfg.MaxReservations,
		MaxCircuitsPerPeer: cfg.MaxCircuitsPerPeer, // 0 表示不限制
		MaxCircuitsTotal:   cfg.MaxCircuits,
		ReservationTTL:     cfg.ReservationTTL,
		BufferSize:         cfg.BufferSize,
		ConnectTimeout:     GetRelayDefaults().ConnectTimeout,
		IdleTimeout:        GetRelayDefaults().IdleTimeout,
	}
	return &serverLimiterAdapter{defaults: defaults}
}

// Disable 禁用 Relay 能力
//
// 停止作为中继服务。已建立的中继电路会被优雅关闭。
func (s *RelayService) Disable(_ context.Context) error {
	if !s.serverEnabled.Load() {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 停止服务端
	if s.server != nil {
		s.server.Stop()
		s.server = nil
	}

	s.serverEnabled.Store(false)
	return nil
}

// IsEnabled 检查 Relay 能力是否已启用
func (s *RelayService) IsEnabled() bool {
	return s.serverEnabled.Load()
}

// Stats 返回统计信息
func (s *RelayService) Stats() pkgif.RelayStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := pkgif.RelayStats{
		Enabled: s.serverEnabled.Load(),
	}

	if s.server != nil {
		serverStats := s.server.Stats()
		stats.ActiveCircuits = serverStats.TotalCircuits
		stats.ReservationCount = serverStats.ActiveReservations
		stats.TotalRelayed = uint64(serverStats.UniqueRelayedPeers)
		stats.PeakCircuits = serverStats.TotalCircuits
	}

	return stats
}

// ════════════════════════════════════════════════════════════════════════════
// 辅助函数
// ════════════════════════════════════════════════════════════════════════════

// isPubliclyReachable 检查节点是否公网可达
//
// v2.0 修复：只有经过验证的可达地址才认为可达
// 只允许 ShareableAddrs（已验证公网地址）触发 Relay 服务端启用
func (s *RelayService) isPubliclyReachable() bool {
	if s.host == nil {
		logger.Warn("Relay 可达性检查失败", "reason", "host is nil")
		return false
	}

	// 1. 检查已验证的可达地址（ShareableAddrs）
	shareableAddrs := s.host.ShareableAddrs()
	for _, addr := range shareableAddrs {
		if isPublicAddr(addr) {
			logger.Debug("Relay 可达性检查通过",
				"reason", "has verified shareable addr",
				"addr", addr)
			return true
		}
	}

	// v2.0 修复：未验证的地址不算可达，避免 NAT/防火墙节点错误开启 Relay 服务
	if len(shareableAddrs) > 0 {
		logger.Debug("Relay 可达性检查失败",
			"reason", "shareable addrs not public",
			"shareableAddrs", shareableAddrs)
	} else {
		logger.Warn("Relay 可达性检查失败",
			"reason", "no shareable addrs")
	}
	return false
}

// isPublicAddr 检查地址是否为公网地址
func isPublicAddr(addr string) bool {
	ip := extractIP(addr)
	if ip == nil {
		return false
	}

	// 检查是否为私有/回环地址
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() {
		return false
	}

	return true
}

// extractIP 从 multiaddr 字符串提取 IP
func extractIP(addr string) net.IP {
	patterns := []string{"/ip4/", "/ip6/"}
	for _, pattern := range patterns {
		idx := -1
		for i := 0; i <= len(addr)-len(pattern); i++ {
			if addr[i:i+len(pattern)] == pattern {
				idx = i + len(pattern)
				break
			}
		}
		if idx >= 0 && idx < len(addr) {
			end := idx
			for end < len(addr) && addr[end] != '/' {
				end++
			}
			ipStr := addr[idx:end]
			return net.ParseIP(ipStr)
		}
	}
	return nil
}

// extractNodeID 从 multiaddr 提取节点 ID
//
// 从 multiaddr 中提取 /p2p/<peerID> 组件。
// 例如：/ip4/1.2.3.4/tcp/4001/p2p/QmXXX -> QmXXX
func extractNodeID(addr types.Multiaddr) string {
	if addr == nil {
		return ""
	}

	// 尝试从地址中提取 p2p 协议的值
	peerID, err := addr.ValueForProtocol(types.ProtocolP2P)
	if err != nil {
		// 如果没有 p2p 组件，返回空
		return ""
	}

	return peerID
}

// ════════════════════════════════════════════════════════════════════════════
// Server Limiter 适配器
// ════════════════════════════════════════════════════════════════════════════

// serverLimiterAdapter 实现 server.Limiter 接口
type serverLimiterAdapter struct {
	defaults RelayDefaults

	// 状态跟踪
	reservations sync.Map // peerID -> time.Time (预约时间)
	circuits     sync.Map // peerID -> int (当前电路数)

	// 统计
	totalCircuits int32
}

// newServerLimiterAdapter 创建服务端限流器适配器
func newServerLimiterAdapter(defaults RelayDefaults) server.Limiter {
	return &serverLimiterAdapter{defaults: defaults}
}

// CanReserve 检查是否可以预约
//
// 预约记录会过期清理：过期则删除旧记录允许重新预约。
// 允许同一节点重复预约（续期）：未过期则刷新预约时间并允许通过（幂等）。
func (l *serverLimiterAdapter) CanReserve(peer types.PeerID) bool {
	peerKey := string(peer)

	// 检查是否已有预约
	if reserveTime, exists := l.reservations.Load(peerKey); exists {
		// 检查预约是否过期
		if time.Since(reserveTime.(time.Time)) < l.defaults.ReservationTTL {
			// 未过期，刷新预约时间并允许重复预约
			l.reservations.Store(peerKey, time.Now())
			return true
		}
		// 过期了，删除旧记录，允许重新预约
		l.reservations.Delete(peerKey)
	}

	// 检查总预约数是否超限（同时清理过期记录）
	count := 0
	now := time.Now()
	var expiredPeers []string
	l.reservations.Range(func(key, value interface{}) bool {
		if time.Since(value.(time.Time)) >= l.defaults.ReservationTTL {
			// 记录过期的 peer，稍后删除
			expiredPeers = append(expiredPeers, key.(string))
		} else {
			count++
		}
		return true // 继续遍历
	})

	// 清理过期记录
	for _, expiredPeer := range expiredPeers {
		l.reservations.Delete(expiredPeer)
	}

	// 使用清理后的计数检查
	if count >= l.defaults.MaxReservations {
		return false
	}

	// 记录预约
	l.reservations.Store(peerKey, now)
	return true
}

// CanConnect 检查是否可以连接
func (l *serverLimiterAdapter) CanConnect(src, _ types.PeerID) bool {
	// 检查总电路数（0 表示不限制）
	if l.defaults.MaxCircuitsTotal > 0 && int(atomic.LoadInt32(&l.totalCircuits)) >= l.defaults.MaxCircuitsTotal {
		return false
	}

	// 检查单节点电路数（0 表示不限制）
	srcKey := string(src)
	if count, ok := l.circuits.Load(srcKey); ok {
		if l.defaults.MaxCircuitsPerPeer > 0 && count.(int) >= l.defaults.MaxCircuitsPerPeer {
			return false
		}
	}

	// 增加计数
	if existing, loaded := l.circuits.LoadOrStore(srcKey, 1); loaded {
		l.circuits.Store(srcKey, existing.(int)+1)
	}
	atomic.AddInt32(&l.totalCircuits, 1)

	return true
}

// ReserveFor 预约时长
func (l *serverLimiterAdapter) ReserveFor() time.Duration {
	return l.defaults.ReservationTTL
}

// MaxCircuitsPerPeer 每节点最大电路数
func (l *serverLimiterAdapter) MaxCircuitsPerPeer() int {
	return l.defaults.MaxCircuitsPerPeer
}

// ReleaseCircuit 释放电路
func (l *serverLimiterAdapter) ReleaseCircuit(peer types.PeerID) {
	key := string(peer)
	if count, ok := l.circuits.Load(key); ok {
		newCount := count.(int) - 1
		if newCount <= 0 {
			l.circuits.Delete(key)
		} else {
			l.circuits.Store(key, newCount)
		}
		atomic.AddInt32(&l.totalCircuits, -1)
	}
}

// ReleaseReservation 释放预约
func (l *serverLimiterAdapter) ReleaseReservation(peer types.PeerID) {
	l.reservations.Delete(string(peer))
}
