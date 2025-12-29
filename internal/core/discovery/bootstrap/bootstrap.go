// Package bootstrap 提供引导节点发现功能
//
// Bootstrap 发现器用于在启动时连接到已知的引导节点，
// 从而获取网络中其他节点的信息。
package bootstrap

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"errors"
	"math/rand" //nolint:gosec // G404: 使用 crypto/rand 初始化种子，用于非安全的随机打乱
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/address"
	"github.com/dep2p/go-dep2p/internal/util/logger"
	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 包级别日志实例
var log = logger.Logger("discovery.bootstrap")

func init() {
	// 使用 crypto/rand 初始化 math/rand 种子
	var seed int64
	if err := binary.Read(crand.Reader, binary.LittleEndian, &seed); err != nil {
		seed = time.Now().UnixNano()
	}
	rand.Seed(seed)
}

// ============================================================================
//                              错误定义
// ============================================================================

var (
	// ErrNoBootstrapPeers 没有引导节点
	ErrNoBootstrapPeers = errors.New("no bootstrap peers available")

	// ErrBootstrapFailed 引导失败
	ErrBootstrapFailed = errors.New("bootstrap failed")
)

// ============================================================================
//                              配置
// ============================================================================

// Config 引导发现器配置
type Config struct {
	// Peers 引导节点列表
	Peers []discoveryif.PeerInfo

	// ConnectTimeout 连接超时
	ConnectTimeout time.Duration

	// MaxConcurrent 最大并发连接数
	MaxConcurrent int

	// RetryInterval 重试基础间隔（指数退避的基础值）
	RetryInterval time.Duration

	// MaxRetries 最大重试次数
	MaxRetries int

	// ShuffleOrder 是否随机顺序连接
	ShuffleOrder bool

	// ============================================================================
	//                              指数退避配置（REQ-BOOT-004）
	// ============================================================================

	// EnableBackoff 启用指数退避
	EnableBackoff bool

	// BackoffMultiplier 退避倍数（每次失败后间隔乘以此值）
	// 例如 2.0 表示 30s -> 60s -> 120s -> 240s
	BackoffMultiplier float64

	// MaxBackoffInterval 最大退避间隔
	MaxBackoffInterval time.Duration

	// BackoffJitter 退避抖动因子（0-1），用于防止惊群效应
	// 例如 0.2 表示在计算出的间隔基础上 ±20% 抖动
	BackoffJitter float64

	// HealthCheckInterval 健康检查间隔（定期检查引导节点状态）
	HealthCheckInterval time.Duration

	// UnhealthyThreshold 不健康阈值（连续失败次数超过此值标记为不健康）
	UnhealthyThreshold int

	// RecoveryBackoff 恢复检测退避（不健康节点的重试间隔）
	RecoveryBackoff time.Duration
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		Peers:          nil,
		ConnectTimeout: 30 * time.Second,
		MaxConcurrent:  5,
		RetryInterval:  30 * time.Second,
		MaxRetries:     3,
		ShuffleOrder:   true,

		// 指数退避默认配置
		EnableBackoff:       true,
		BackoffMultiplier:   2.0,
		MaxBackoffInterval:  5 * time.Minute,
		BackoffJitter:       0.2,
		HealthCheckInterval: 1 * time.Minute,
		UnhealthyThreshold:  3,
		RecoveryBackoff:     2 * time.Minute,
	}
}

// ============================================================================
//                              Bootstrap 实现
// ============================================================================

// ============================================================================
//                              节点健康状态（REQ-BOOT-004）
// ============================================================================

// PeerHealth 节点健康状态
type PeerHealth struct {
	// NodeID 节点 ID
	NodeID types.NodeID

	// FailCount 连续失败次数
	FailCount int

	// LastAttempt 上次尝试时间
	LastAttempt time.Time

	// LastSuccess 上次成功时间
	LastSuccess time.Time

	// LastError 上次错误
	LastError error

	// CurrentBackoff 当前退避间隔
	CurrentBackoff time.Duration

	// IsHealthy 是否健康
	IsHealthy bool
}

// Discoverer 引导发现器
type Discoverer struct {
	config Config

	// 连接器（用于连接到引导节点）
	connector Connector

	// 引导节点管理
	peers   []discoveryif.PeerInfo
	peersMu sync.RWMutex

	// 已连接的节点
	connected   map[types.NodeID]bool
	connectedMu sync.RWMutex

	// 已发现的节点（从引导节点获取）
	discovered   map[types.NodeID]discoveryif.PeerInfo
	discoveredMu sync.RWMutex

	// 节点健康状态（REQ-BOOT-004）
	peerHealth   map[types.NodeID]*PeerHealth
	peerHealthMu sync.RWMutex

	// 运行状态
	running int32
	ctx     context.Context
	cancel  context.CancelFunc
}

// Connector 连接器接口
//
// Layer1 身份第一性原则：所有连接必须绑定 expected NodeID
type Connector interface {
	// Connect 连接到指定节点
	//
	// 参数:
	//   - nodeID: expected NodeID（身份第一性：必须绑定）
	//   - addrs: 拨号地址列表（Dial Addresses）
	//
	// 语义：DialByNodeIDWithDialAddrs
	// 连接成功后验证 RemoteIdentity == nodeID
	Connect(ctx context.Context, nodeID types.NodeID, addrs []string) error

	// GetPeers 从连接的节点获取其已知节点
	GetPeers(ctx context.Context, nodeID types.NodeID) ([]discoveryif.PeerInfo, error)

	// LocalID 返回本地节点 ID
	LocalID() types.NodeID
}

// NewDiscoverer 创建引导发现器
func NewDiscoverer(config Config, connector Connector) *Discoverer {
	d := &Discoverer{
		config:     config,
		connector:  connector,
		peers:      make([]discoveryif.PeerInfo, 0),
		connected:  make(map[types.NodeID]bool),
		discovered: make(map[types.NodeID]discoveryif.PeerInfo),
		peerHealth: make(map[types.NodeID]*PeerHealth),
	}

	// 复制配置的引导节点
	if len(config.Peers) > 0 {
		d.peers = append(d.peers, config.Peers...)
	}

	return d
}

// ============================================================================
//                              指数退避逻辑（REQ-BOOT-004）
// ============================================================================

// calculateBackoff 计算下一次退避间隔
//
// 使用指数退避算法：interval = baseInterval * (multiplier ^ failCount)
// 并添加抖动以防止惊群效应
func (d *Discoverer) calculateBackoff(failCount int) time.Duration {
	if !d.config.EnableBackoff || failCount == 0 {
		return d.config.RetryInterval
	}

	// 计算指数退避：baseInterval * (multiplier ^ failCount)
	backoff := float64(d.config.RetryInterval)
	for i := 0; i < failCount && i < d.config.MaxRetries; i++ {
		backoff *= d.config.BackoffMultiplier
	}

	// 限制最大退避间隔
	if time.Duration(backoff) > d.config.MaxBackoffInterval {
		backoff = float64(d.config.MaxBackoffInterval)
	}

	// 添加抖动（±jitter%）
	if d.config.BackoffJitter > 0 {
		jitterRange := backoff * d.config.BackoffJitter
		jitter := (rand.Float64()*2 - 1) * jitterRange // -jitterRange to +jitterRange
		backoff += jitter
	}

	return time.Duration(backoff)
}

// shouldAttemptConnect 检查是否应该尝试连接（基于退避策略）
func (d *Discoverer) shouldAttemptConnect(nodeID types.NodeID) bool {
	d.peerHealthMu.RLock()
	health, exists := d.peerHealth[nodeID]
	d.peerHealthMu.RUnlock()

	if !exists {
		return true // 首次尝试
	}

	// 如果节点健康或已连接，允许连接
	if health.IsHealthy {
		return true
	}

	// 检查是否已过退避时间
	timeSinceLastAttempt := time.Since(health.LastAttempt)
	return timeSinceLastAttempt >= health.CurrentBackoff
}

// recordConnectSuccess 记录连接成功
func (d *Discoverer) recordConnectSuccess(nodeID types.NodeID) {
	d.peerHealthMu.Lock()
	defer d.peerHealthMu.Unlock()

	now := time.Now()
	if health, exists := d.peerHealth[nodeID]; exists {
		health.FailCount = 0
		health.LastSuccess = now
		health.LastAttempt = now
		health.IsHealthy = true
		health.CurrentBackoff = d.config.RetryInterval
		health.LastError = nil
	} else {
		d.peerHealth[nodeID] = &PeerHealth{
			NodeID:         nodeID,
			FailCount:      0,
			LastAttempt:    now,
			LastSuccess:    now,
			IsHealthy:      true,
			CurrentBackoff: d.config.RetryInterval,
		}
	}

	log.Debug("记录连接成功",
		"nodeID", nodeID.ShortString())
}

// recordConnectFailure 记录连接失败
func (d *Discoverer) recordConnectFailure(nodeID types.NodeID, err error) {
	d.peerHealthMu.Lock()
	defer d.peerHealthMu.Unlock()

	now := time.Now()
	if health, exists := d.peerHealth[nodeID]; exists {
		health.FailCount++
		health.LastAttempt = now
		health.LastError = err
		health.CurrentBackoff = d.calculateBackoff(health.FailCount)

		// 超过阈值标记为不健康
		if health.FailCount >= d.config.UnhealthyThreshold {
			health.IsHealthy = false
			health.CurrentBackoff = d.config.RecoveryBackoff
		}
	} else {
		backoff := d.calculateBackoff(1)
		d.peerHealth[nodeID] = &PeerHealth{
			NodeID:         nodeID,
			FailCount:      1,
			LastAttempt:    now,
			LastError:      err,
			IsHealthy:      true,
			CurrentBackoff: backoff,
		}
	}

	log.Debug("记录连接失败",
		"nodeID", nodeID.ShortString(),
		"err", err,
		"failCount", d.peerHealth[nodeID].FailCount,
		"nextBackoff", d.peerHealth[nodeID].CurrentBackoff)
}

// GetPeerHealth 获取节点健康状态（用于诊断）
func (d *Discoverer) GetPeerHealth(nodeID types.NodeID) *PeerHealth {
	d.peerHealthMu.RLock()
	defer d.peerHealthMu.RUnlock()

	if health, exists := d.peerHealth[nodeID]; exists {
		// 返回副本以避免并发问题
		healthCopy := *health
		return &healthCopy
	}
	return nil
}

// GetAllPeerHealth 获取所有节点健康状态（用于诊断）
func (d *Discoverer) GetAllPeerHealth() map[types.NodeID]*PeerHealth {
	d.peerHealthMu.RLock()
	defer d.peerHealthMu.RUnlock()

	result := make(map[types.NodeID]*PeerHealth, len(d.peerHealth))
	for id, health := range d.peerHealth {
		healthCopy := *health
		result[id] = &healthCopy
	}
	return result
}

// HealthyPeerCount 返回健康的引导节点数量
func (d *Discoverer) HealthyPeerCount() int {
	d.peerHealthMu.RLock()
	defer d.peerHealthMu.RUnlock()

	count := 0
	for _, health := range d.peerHealth {
		if health.IsHealthy {
			count++
		}
	}
	return count
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动引导发现
func (d *Discoverer) Start(ctx context.Context) error {
	// 检查是否已经启动
	if !atomic.CompareAndSwapInt32(&d.running, 0, 1) {
		return nil // 已经启动
	}

	d.ctx, d.cancel = context.WithCancel(ctx)

	log.Info("引导发现器启动",
		"bootstrap_peers", len(d.peers),
		"backoff_enabled", d.config.EnableBackoff)

	// 立即执行一次引导
	go d.bootstrap()

	// REQ-BOOT-004: 启动健康检查循环
	if d.config.HealthCheckInterval > 0 {
		go d.healthCheckLoop()
	}

	return nil
}

// Stop 停止引导发现
func (d *Discoverer) Stop() error {
	// 检查是否正在运行
	if !atomic.CompareAndSwapInt32(&d.running, 1, 0) {
		return nil // 未启动或已停止
	}

	if d.cancel != nil {
		d.cancel()
	}
	log.Info("引导发现器已停止")
	return nil
}

// Close 关闭发现器
func (d *Discoverer) Close() error {
	return d.Stop()
}

// ============================================================================
//                              引导流程
// ============================================================================

// bootstrap 执行引导流程
func (d *Discoverer) bootstrap() {
	d.peersMu.RLock()
	peers := make([]discoveryif.PeerInfo, len(d.peers))
	copy(peers, d.peers)
	d.peersMu.RUnlock()

	if len(peers) == 0 {
		log.Warn("没有引导节点")
		return
	}

	// 随机打乱顺序
	if d.config.ShuffleOrder {
		rand.Shuffle(len(peers), func(i, j int) {
			peers[i], peers[j] = peers[j], peers[i]
		})
	}

	// 并发连接
	sem := make(chan struct{}, d.config.MaxConcurrent)
	var wg sync.WaitGroup

	successCount := 0
	var successMu sync.Mutex

	for _, peer := range peers {
		if d.ctx.Err() != nil {
			break
		}

		wg.Add(1)
		go func(p discoveryif.PeerInfo) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			if d.connectToPeer(p) {
				successMu.Lock()
				successCount++
				successMu.Unlock()
			}
		}(peer)
	}

	wg.Wait()

	log.Info("引导完成",
		"connected", successCount,
		"total", len(peers))
}

// healthCheckLoop 健康检查循环（REQ-BOOT-004）
//
// 定期检查不健康的引导节点，尝试重新连接
func (d *Discoverer) healthCheckLoop() {
	ticker := time.NewTicker(d.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.performHealthCheck()
		}
	}
}

// performHealthCheck 执行健康检查
func (d *Discoverer) performHealthCheck() {
	d.peersMu.RLock()
	peers := make([]discoveryif.PeerInfo, len(d.peers))
	copy(peers, d.peers)
	d.peersMu.RUnlock()

	unhealthyCount := 0
	retriedCount := 0

	for _, peer := range peers {
		// 检查是否已连接
		d.connectedMu.RLock()
		isConnected := d.connected[peer.ID]
		d.connectedMu.RUnlock()

		if isConnected {
			continue // 已连接，跳过
		}

		// 获取健康状态
		health := d.GetPeerHealth(peer.ID)
		if health == nil {
			continue // 首次，bootstrap 会处理
		}

		if !health.IsHealthy {
			unhealthyCount++

			// 检查是否可以重试
			if d.shouldAttemptConnect(peer.ID) {
				retriedCount++
				go d.connectToPeer(peer)
			}
		}
	}

	if unhealthyCount > 0 {
		log.Debug("健康检查完成",
			"unhealthy", unhealthyCount,
			"retried", retriedCount)
	}
}

// connectToPeer 连接到单个引导节点
//
// Layer1 身份第一性：使用 DialByNodeIDWithDialAddrs 语义
// - peer.ID: expected NodeID
// - peer.Addrs: 拨号地址列表
//
// REQ-BOOT-004: 使用指数退避策略
// - 检查是否应该尝试连接（基于退避状态）
// - 记录成功/失败以更新健康状态
func (d *Discoverer) connectToPeer(peer discoveryif.PeerInfo) bool {
	// 检查 ctx 是否已初始化（Start() 是否已调用）
	if d.ctx == nil {
		return false
	}

	// 检查是否是自己
	if d.connector != nil && peer.ID == d.connector.LocalID() {
		return false
	}

	// 检查是否已连接
	d.connectedMu.RLock()
	if d.connected[peer.ID] {
		d.connectedMu.RUnlock()
		return true
	}
	d.connectedMu.RUnlock()

	// REQ-BOOT-004: 检查退避状态
	if !d.shouldAttemptConnect(peer.ID) {
		log.Debug("跳过连接（退避中）",
			"peer", peer.ID.ShortString())
		return false
	}

	// 检查地址列表
	if len(peer.Addrs) == 0 {
		log.Debug("引导节点无地址",
			"peer", peer.ID.ShortString())
		return false
	}

	ctx, cancel := context.WithTimeout(d.ctx, d.config.ConnectTimeout)
	defer cancel()

	// Layer1 身份第一性：使用 DialByNodeIDWithDialAddrs 语义
	// 传入 expected NodeID 和地址列表，由连接器验证 RemoteIdentity == expected
	if d.connector != nil {
		addrStrs := types.MultiaddrsToStrings(peer.Addrs)
		if err := d.connector.Connect(ctx, peer.ID, addrStrs); err != nil {
			// REQ-BOOT-004: 记录连接失败
			d.recordConnectFailure(peer.ID, err)
			log.Debug("连接引导节点失败",
				"peer", peer.ID.ShortString(),
				"addrs", addrStrs,
				"err", err)
			return false
		}
	}

	// REQ-BOOT-004: 记录连接成功
	d.recordConnectSuccess(peer.ID)

	d.connectedMu.Lock()
	d.connected[peer.ID] = true
	d.connectedMu.Unlock()

	log.Info("已连接引导节点",
		"peer", peer.ID.ShortString())

	// 从引导节点获取其他节点
	go d.fetchPeersFrom(peer.ID)

	return true
}

// fetchPeersFrom 从引导节点获取其已知节点
func (d *Discoverer) fetchPeersFrom(nodeID types.NodeID) {
	if d.connector == nil || d.ctx == nil {
		return
	}

	ctx, cancel := context.WithTimeout(d.ctx, 30*time.Second)
	defer cancel()

	peers, err := d.connector.GetPeers(ctx, nodeID)
	if err != nil {
		log.Debug("获取节点列表失败",
			"from", nodeID.ShortString(),
			"err", err)
		return
	}

	d.discoveredMu.Lock()
	for _, peer := range peers {
		if peer.ID != d.connector.LocalID() {
			d.discovered[peer.ID] = peer
		}
	}
	d.discoveredMu.Unlock()

	log.Debug("从引导节点获取节点列表",
		"from", nodeID.ShortString(),
		"count", len(peers))
}

// ============================================================================
//                              Discoverer 接口实现
// ============================================================================

// FindPeer 查找指定节点
func (d *Discoverer) FindPeer(_ context.Context, id types.NodeID) ([]endpoint.Address, error) {
	d.discoveredMu.RLock()
	if peer, ok := d.discovered[id]; ok {
		d.discoveredMu.RUnlock()
		addrs := make([]endpoint.Address, len(peer.Addrs))
		for i, addr := range peer.Addrs {
			addrs[i] = address.NewAddr(addr)
		}
		return addrs, nil
	}
	d.discoveredMu.RUnlock()

	// 检查引导节点列表
	d.peersMu.RLock()
	for _, peer := range d.peers {
		if peer.ID == id {
			d.peersMu.RUnlock()
			addrs := make([]endpoint.Address, len(peer.Addrs))
			for i, addr := range peer.Addrs {
				addrs[i] = address.NewAddr(addr)
			}
			return addrs, nil
		}
	}
	d.peersMu.RUnlock()

	return nil, nil
}

// FindPeers 批量查找节点
func (d *Discoverer) FindPeers(ctx context.Context, ids []types.NodeID) (map[types.NodeID][]endpoint.Address, error) {
	result := make(map[types.NodeID][]endpoint.Address)

	for _, id := range ids {
		addrs, _ := d.FindPeer(ctx, id)
		if len(addrs) > 0 {
			result[id] = addrs
		}
	}

	return result, nil
}

// FindClosestPeers 查找最近的节点（引导发现器不支持）
func (d *Discoverer) FindClosestPeers(_ context.Context, _ []byte, count int) ([]types.NodeID, error) {
	// 引导发现器不支持按距离查找，返回所有已发现节点
	d.discoveredMu.RLock()
	defer d.discoveredMu.RUnlock()

	result := make([]types.NodeID, 0, count)
	for id := range d.discovered {
		result = append(result, id)
		if len(result) >= count {
			break
		}
	}

	return result, nil
}

// DiscoverPeers 发现节点
func (d *Discoverer) DiscoverPeers(ctx context.Context, _ string) (<-chan discoveryif.PeerInfo, error) {
	ch := make(chan discoveryif.PeerInfo, 10)

	go func() {
		defer close(ch)

		// 返回所有已发现的节点
		d.discoveredMu.RLock()
		peers := make([]discoveryif.PeerInfo, 0, len(d.discovered))
		for _, peer := range d.discovered {
			peers = append(peers, peer)
		}
		d.discoveredMu.RUnlock()

		for _, peer := range peers {
			select {
			case <-ctx.Done():
				return
			case ch <- peer:
			}
		}

		// 也返回引导节点
		d.peersMu.RLock()
		bootstrapPeers := make([]discoveryif.PeerInfo, len(d.peers))
		copy(bootstrapPeers, d.peers)
		d.peersMu.RUnlock()

		for _, peer := range bootstrapPeers {
			select {
			case <-ctx.Done():
				return
			case ch <- peer:
			}
		}
	}()

	return ch, nil
}

// ============================================================================
//                              引导节点管理
// ============================================================================

// GetBootstrapPeers 获取引导节点列表
func (d *Discoverer) GetBootstrapPeers(_ context.Context) ([]discoveryif.PeerInfo, error) {
	d.peersMu.RLock()
	defer d.peersMu.RUnlock()

	result := make([]discoveryif.PeerInfo, len(d.peers))
	copy(result, d.peers)
	return result, nil
}

// AddBootstrapPeer 添加引导节点
func (d *Discoverer) AddBootstrapPeer(peer discoveryif.PeerInfo) {
	d.peersMu.Lock()
	defer d.peersMu.Unlock()

	// 检查是否已存在
	for _, existing := range d.peers {
		if existing.ID == peer.ID {
			return
		}
	}

	d.peers = append(d.peers, peer)
	log.Info("添加引导节点",
		"peer", peer.ID.ShortString(),
		"total", len(d.peers))
}

// RemoveBootstrapPeer 移除引导节点
func (d *Discoverer) RemoveBootstrapPeer(id types.NodeID) {
	d.peersMu.Lock()
	defer d.peersMu.Unlock()

	for i, peer := range d.peers {
		if peer.ID == id {
			d.peers = append(d.peers[:i], d.peers[i+1:]...)
			log.Info("移除引导节点",
				"peer", id.ShortString(),
				"remaining", len(d.peers))
			return
		}
	}
}

// SetBootstrapPeers 设置引导节点列表
func (d *Discoverer) SetBootstrapPeers(peers []discoveryif.PeerInfo) {
	d.peersMu.Lock()
	defer d.peersMu.Unlock()

	d.peers = make([]discoveryif.PeerInfo, len(peers))
	copy(d.peers, peers)

	log.Info("设置引导节点列表", "count", len(peers))
}

// ============================================================================
//                              状态查询
// ============================================================================

// ConnectedCount 返回已连接的引导节点数量
func (d *Discoverer) ConnectedCount() int {
	d.connectedMu.RLock()
	defer d.connectedMu.RUnlock()
	return len(d.connected)
}

// DiscoveredCount 返回已发现的节点数量
func (d *Discoverer) DiscoveredCount() int {
	d.discoveredMu.RLock()
	defer d.discoveredMu.RUnlock()
	return len(d.discovered)
}

// IsConnected 检查是否已连接到指定引导节点
func (d *Discoverer) IsConnected(id types.NodeID) bool {
	d.connectedMu.RLock()
	defer d.connectedMu.RUnlock()
	return d.connected[id]
}

// ============================================================================
//                              接口断言
// ============================================================================

var _ discoveryif.Discoverer = (*Discoverer)(nil)
var _ discoveryif.Bootstrap = (*Discoverer)(nil)

