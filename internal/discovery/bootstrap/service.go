// Package bootstrap 提供引导节点发现服务
//
// 本文件实现 Bootstrap 能力服务，提供 EnableBootstrap/DisableBootstrap API。
package bootstrap

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/multiaddr"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 使用 bootstrap.go 中定义的 logger

// ════════════════════════════════════════════════════════════════════════════
// BootstrapService 引导节点能力服务
// ════════════════════════════════════════════════════════════════════════════

// BootstrapService 引导节点能力服务
// 管理 Bootstrap 能力的启用/禁用和相关组件
type BootstrapService struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	// 依赖
	host     pkgif.Host
	dht      pkgif.DHT
	liveness pkgif.Liveness

	// 组件
	store   *ExtendedNodeStore
	probe   *ProbeService
	walker  *DiscoveryWalker

	// 状态
	enabled atomic.Bool
	mu      sync.RWMutex

	// 配置
	defaults BootstrapDefaults
	dataDir  string
}

// ServiceOption 服务选项
type ServiceOption func(*BootstrapService)

// WithDataDir 设置数据目录
func WithDataDir(dir string) ServiceOption {
	return func(s *BootstrapService) {
		s.dataDir = dir
	}
}

// WithDHT 设置 DHT 服务
func WithDHT(dht pkgif.DHT) ServiceOption {
	return func(s *BootstrapService) {
		s.dht = dht
	}
}

// WithLiveness 设置 Liveness 服务
func WithLiveness(liveness pkgif.Liveness) ServiceOption {
	return func(s *BootstrapService) {
		s.liveness = liveness
	}
}

// NewBootstrapService 创建 Bootstrap 服务
func NewBootstrapService(host pkgif.Host, opts ...ServiceOption) *BootstrapService {
	ctx, cancel := context.WithCancel(context.Background())

	s := &BootstrapService{
		ctx:       ctx,
		ctxCancel: cancel,
		host:      host,
		defaults:  GetDefaults(),
		dataDir:   ".",
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// ════════════════════════════════════════════════════════════════════════════
// 能力开关 API
// ════════════════════════════════════════════════════════════════════════════

// Enable 启用 Bootstrap 能力
//
// 将当前节点设置为引导节点，为网络中的新节点提供初始对等方发现服务。
// 启用后，节点将：
//   - 维护扩展的节点存储（最多 50,000 个节点）
//   - 定期探测存储节点的存活状态
//   - 主动通过 Random Walk 发现新节点
//   - 响应 FIND_NODE 请求，返回最近 K 个节点
//
// 前置条件：
//   - 节点必须有公网可达地址（非 NAT 后）
//
// 所有运营参数使用内置默认值，用户无需也无法配置。
func (s *BootstrapService) Enable(_ context.Context) error {
	// 幂等检查
	if s.enabled.Load() {
		return nil
	}

	// 前置条件检查：公网可达性
	if !s.isPubliclyReachable() {
		return ErrNotPubliclyReachable
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 初始化存储
	s.store = NewExtendedNodeStore(
		WithMaxNodes(s.defaults.MaxNodes),
		WithCacheSize(s.defaults.CacheSize),
		WithExpireTime(s.defaults.NodeExpireTime),
		WithOfflineThreshold(s.defaults.OfflineThreshold),
	)

	// 尝试从持久化加载
	if err := s.store.LoadFromPersister(); err != nil {
		// 加载失败不影响服务启动，使用空存储
		logger.Debug("从持久化加载节点失败（将使用空存储）", "error", err)
	}

	// 初始化探测服务
	s.probe = NewProbeService(
		s.host,
		s.store,
		s.liveness,
		WithProbeInterval(s.defaults.ProbeInterval),
		WithProbeBatchSize(s.defaults.ProbeBatchSize),
		WithProbeTimeout(s.defaults.ProbeTimeout),
		WithProbeMaxConcurrent(s.defaults.ProbeMaxConcurrent),
	)

	// 初始化发现服务
	s.walker = NewDiscoveryWalker(
		s.host,
		s.store,
		s.dht,
		WithWalkerInterval(s.defaults.DiscoveryInterval),
		WithWalkLen(s.defaults.DiscoveryWalkLen),
	)

	// 启动后台服务
	if err := s.probe.Start(); err != nil {
		return fmt.Errorf("start probe service: %w", err)
	}

	if err := s.walker.Start(); err != nil {
		s.probe.Stop()
		return fmt.Errorf("start walker service: %w", err)
	}

	// 启动清理任务
	go s.runCleanup()

	s.enabled.Store(true)
	return nil
}

// Disable 禁用 Bootstrap 能力
//
// 停止作为引导节点服务，但保留已存储的节点信息（下次启用时可快速恢复）。
func (s *BootstrapService) Disable(_ context.Context) error {
	if !s.enabled.Load() {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 停止后台服务
	if s.probe != nil {
		s.probe.Stop()
	}
	if s.walker != nil {
		s.walker.Stop()
	}

	// 保留存储数据，不关闭（允许下次快速恢复）

	s.enabled.Store(false)
	return nil
}

// IsEnabled 检查是否已启用
func (s *BootstrapService) IsEnabled() bool {
	if s == nil {
		return false
	}
	return s.enabled.Load()
}

// ════════════════════════════════════════════════════════════════════════════
// 统计信息
// ════════════════════════════════════════════════════════════════════════════

// Stats 返回统计信息
func (s *BootstrapService) Stats() pkgif.BootstrapStats {
	if s == nil {
		return pkgif.BootstrapStats{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := pkgif.BootstrapStats{
		Enabled: s.enabled.Load(),
	}

	if s.store != nil {
		storeStats := s.store.Stats()
		stats.TotalNodes = storeStats.TotalNodes
		stats.OnlineNodes = storeStats.OnlineNodes
	}

	if s.probe != nil {
		probeStats := s.probe.Stats()
		stats.LastProbe = probeStats.LastProbeTime
		if probeStats.TotalProbes > 0 {
			stats.ProbeSuccessRate = float64(probeStats.SuccessProbes) / float64(probeStats.TotalProbes)
		}
	}

	if s.walker != nil {
		stats.LastDiscovery = s.walker.LastRun()
	}

	return stats
}

// ════════════════════════════════════════════════════════════════════════════
// FIND_NODE 响应
// ════════════════════════════════════════════════════════════════════════════

// FindClosest 查找 XOR 距离最近的 K 个节点
// 用于响应 FIND_NODE 请求
func (s *BootstrapService) FindClosest(target types.NodeID) []*NodeEntry {
	if !s.enabled.Load() {
		return nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.store == nil {
		return nil
	}

	return s.store.FindClosest(target, s.defaults.ResponseK)
}

// AddNode 添加节点到存储
// 用于从 DHT 回调中添加发现的节点
func (s *BootstrapService) AddNode(peer types.PeerInfo) error {
	if !s.enabled.Load() {
		return ErrNotEnabled
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.store == nil {
		return ErrNotEnabled
	}

	// 转换地址格式
	addrs := make([]string, len(peer.Addrs))
	for i, addr := range peer.Addrs {
		addrs[i] = addr.String()
	}

	entry := &NodeEntry{
		ID:        types.NodeID(peer.ID),
		Addrs:     addrs,
		LastSeen:  time.Now(),
		Status:    NodeStatusUnknown,
		CreatedAt: time.Now(),
	}

	return s.store.Put(entry)
}

// ════════════════════════════════════════════════════════════════════════════
// 内部方法
// ════════════════════════════════════════════════════════════════════════════

// isPubliclyReachable 检查节点是否公网可达
//
// 检查优先级：
//  1. 先检查对外通告的地址（AdvertisedAddrs），包含用户配置的 --public-addr
//  2. 再检查监听地址（Addrs）
//
// 注意：如果用户通过 --public-addr 明确配置了地址，即使是私网 IP 也认为可达，
// 因为这通常用于测试场景或内网部署。
func (s *BootstrapService) isPubliclyReachable() bool {
	if s.host == nil {
		logger.Warn("Bootstrap 可达性检查失败", "reason", "host is nil")
		return false
	}

	// 1. 优先检查对外通告的地址（包含用户配置的 --public-addr）
	advertisedAddrs := s.host.AdvertisedAddrs()
	if len(advertisedAddrs) > 0 {
		// 检查是否有公网地址
		for _, addr := range advertisedAddrs {
			if isPublicAddr(addr) {
				logger.Debug("Bootstrap 可达性检查通过", 
					"reason", "has public advertised addr",
					"addr", addr)
				return true
			}
		}
		// 如果有用户配置的地址（即使是私网），也认为可达
		logger.Info("Bootstrap 可达性检查通过（用户配置地址）", 
			"reason", "user configured address",
			"advertisedAddrs", advertisedAddrs,
			"note", "private IP allowed when explicitly configured")
		return true
	}

	// 2. 检查监听地址
	addrs := s.host.Addrs()
	if len(addrs) == 0 {
		logger.Warn("Bootstrap 可达性检查失败", 
			"reason", "no listen addresses",
			"advertisedAddrs", advertisedAddrs)
		return false
	}

	// 检查是否有公网地址
	for _, addr := range addrs {
		if isPublicAddr(addr) {
			logger.Debug("Bootstrap 可达性检查通过", 
				"reason", "has public listen addr",
				"addr", addr)
			return true
		}
	}

	logger.Warn("Bootstrap 可达性检查失败", 
		"reason", "no public address found",
		"listenAddrs", addrs,
		"advertisedAddrs", advertisedAddrs)
	return false
}

// isPublicAddr 检查地址是否为公网地址
//
// Phase 11 修复：使用 multiaddr 库正确解析地址
func isPublicAddr(addr string) bool {
	ip := extractIP(addr)
	if ip == nil {
		return false
	}

	// 检查是否为私有/回环/链路本地地址
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}

	// 检查是否为未指定地址 (0.0.0.0 或 ::)
	if ip.IsUnspecified() {
		return false
	}

	return true
}

// extractIP 从 multiaddr 字符串提取 IP
//
// Phase 11 修复：使用 multiaddr 库正确解析
func extractIP(addr string) net.IP {
	// 使用 multiaddr 库解析
	ma, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		return nil
	}

	// 尝试获取 IPv4 地址
	ipStr, err := ma.ValueForProtocol(multiaddr.P_IP4)
	if err == nil && ipStr != "" {
		return net.ParseIP(ipStr)
	}

	// 尝试获取 IPv6 地址
	ipStr, err = ma.ValueForProtocol(multiaddr.P_IP6)
	if err == nil && ipStr != "" {
		return net.ParseIP(ipStr)
	}

	return nil
}

// runCleanup 运行清理任务
func (s *BootstrapService) runCleanup() {
	ticker := time.NewTicker(s.defaults.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if s.enabled.Load() && s.store != nil {
				s.store.Cleanup()
			}
		}
	}
}

// Close 关闭服务
func (s *BootstrapService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.ctxCancel != nil {
		s.ctxCancel()
	}

	if s.probe != nil {
		s.probe.Stop()
	}
	if s.walker != nil {
		s.walker.Stop()
	}
	if s.store != nil {
		s.store.Close()
	}

	return nil
}
