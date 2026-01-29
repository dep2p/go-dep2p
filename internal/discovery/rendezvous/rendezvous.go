package rendezvous

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	pb "github.com/dep2p/go-dep2p/pkg/lib/proto/rendezvous"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("discovery/rendezvous")

// ============================================================================
//                              本地注册记录
// ============================================================================

// localRegistration 本地注册记录
type localRegistration struct {
	Namespace    string
	TTL          time.Duration
	RegisteredAt time.Time
	Point        types.PeerID
	ExpiresAt    time.Time
}

// needsRenewal 是否需要续约
func (r *localRegistration) needsRenewal(renewalInterval time.Duration) bool {
	return time.Now().Add(renewalInterval).After(r.ExpiresAt)
}

// ============================================================================
//                              健康状态
// ============================================================================

const (
	maxFailCount = 3               // 最大连续失败次数
	failCooldown = 5 * time.Minute // 故障冷却时间
)

// pointHealthState Rendezvous 点健康状态
type pointHealthState struct {
	failCount    int
	lastFailTime time.Time
}

// ============================================================================
//                              Discoverer 实现
// ============================================================================

// Discoverer Rendezvous 发现器（客户端）
type Discoverer struct {
	config DiscovererConfig
	host   pkgif.Host

	// 本地 ID
	localID types.PeerID

	// 已知的 Rendezvous 点
	points   []types.PeerID
	pointsMu sync.RWMutex

	// 负载均衡
	roundRobinIndex uint64
	pointHealth     map[types.PeerID]*pointHealthState
	healthMu        sync.RWMutex

	// 本地注册状态
	registrations map[string]*localRegistration
	regMu         sync.RWMutex

	// 生命周期
	ctx       context.Context
	ctxCancel context.CancelFunc
	started   atomic.Bool
	wg        sync.WaitGroup

	mu sync.RWMutex
}

// NewDiscoverer 创建 Rendezvous 发现器
func NewDiscoverer(host pkgif.Host, config DiscovererConfig) *Discoverer {
	ctx, cancel := context.WithCancel(context.Background())

	return &Discoverer{
		config:        config,
		host:          host,
		localID:       types.PeerID(host.ID()),
		points:        config.Points,
		pointHealth:   make(map[types.PeerID]*pointHealthState),
		registrations: make(map[string]*localRegistration),
		ctx:           ctx,
		ctxCancel:     cancel,
	}
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动发现器
func (d *Discoverer) Start(_ context.Context) error {
	if d.started.Load() {
		return ErrAlreadyStarted
	}

	logger.Info("正在启动 Rendezvous 发现器", "points", len(d.points))

	d.started.Store(true)

	// 启动续约循环
	d.wg.Add(1)
	go d.renewalLoop()

	logger.Info("Rendezvous 发现器启动成功")
	return nil
}

// Stop 停止发现器
func (d *Discoverer) Stop(_ context.Context) error {
	if !d.started.Load() {
		return ErrNotStarted
	}

	logger.Info("正在停止 Rendezvous 发现器")

	d.started.Store(false)
	d.ctxCancel()

	// 等待后台循环结束
	d.wg.Wait()

	logger.Info("Rendezvous 发现器已停止")
	return nil
}

// ============================================================================
//                              Discovery 接口实现
// ============================================================================

// FindPeers 发现节点（实现 Discovery 接口）
func (d *Discoverer) FindPeers(ctx context.Context, ns string, opts ...pkgif.DiscoveryOption) (<-chan types.PeerInfo, error) {
	if !d.started.Load() {
		return nil, ErrNotStarted
	}

	ns = pkgif.NormalizeNamespace(ns)

	logger.Debug("Rendezvous 查找节点", "namespace", ns)

	// 解析选项
	options := &pkgif.DiscoveryOptions{Limit: 100}
	for _, opt := range opts {
		opt(options)
	}

	ch := make(chan types.PeerInfo, options.Limit)

	go func() {
		defer close(ch)

		peers, err := d.Discover(ctx, ns, options.Limit)
		if err != nil {
			return
		}

		for _, peer := range peers {
			select {
			case ch <- peer:
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}

// Advertise 广播（实现 Discovery 接口）
func (d *Discoverer) Advertise(ctx context.Context, ns string, opts ...pkgif.DiscoveryOption) (time.Duration, error) {
	if !d.started.Load() {
		return 0, ErrNotStarted
	}

	ns = pkgif.NormalizeNamespace(ns)

	// 解析选项
	options := &pkgif.DiscoveryOptions{TTL: d.config.DefaultTTL}
	for _, opt := range opts {
		opt(options)
	}

	ttl := options.TTL
	if ttl <= 0 {
		ttl = d.config.DefaultTTL
	}

	// 注册到命名空间
	if err := d.Register(ctx, ns, ttl); err != nil {
		return 0, err
	}

	return ttl, nil
}

// ============================================================================
//                              Rendezvous 方法
// ============================================================================

// Register 注册到命名空间
func (d *Discoverer) Register(ctx context.Context, namespace string, ttl time.Duration) error {
	if !d.started.Load() {
		return ErrNotStarted
	}

	namespace = pkgif.NormalizeNamespace(namespace)

	// 验证命名空间
	if err := ValidateNamespace(namespace); err != nil {
		return err
	}

	// 选择 Point
	point, err := d.selectPoint()
	if err != nil {
		return err
	}

	// 构造请求（简化：使用 PeerID 作为 SignedPeerRecord）
	signedPeerRecord := []byte(d.localID)
	req := NewRegisterRequest(namespace, signedPeerRecord, ttl)

	// 发送请求
	resp, err := d.sendRequest(ctx, point, req)
	if err != nil {
		d.recordFailure(point)
		return fmt.Errorf("send request failed: %w", err)
	}

	// 处理响应
	if resp.RegisterResponse == nil {
		return errors.New("missing register response")
	}

	if err := StatusToError(resp.RegisterResponse.Status, resp.RegisterResponse.StatusText); err != nil {
		return err
	}

	// 记录本地注册状态
	actualTTL := time.Duration(resp.RegisterResponse.Ttl) * time.Second
	d.recordRegistration(namespace, point, actualTTL)

	return nil
}

// Unregister 取消注册
func (d *Discoverer) Unregister(namespace string) error {
	if !d.started.Load() {
		return ErrNotStarted
	}

	// 获取注册状态
	d.regMu.RLock()
	reg, exists := d.registrations[namespace]
	d.regMu.RUnlock()

	if !exists {
		return nil // 未注册视为成功
	}

	// 构造请求
	req := NewUnregisterRequest(namespace, []byte(d.localID))

	// 发送请求
	ctx, cancel := context.WithTimeout(d.ctx, d.config.RegisterTimeout)
	defer cancel()

	_, err := d.sendRequest(ctx, reg.Point, req)
	if err != nil {
		return fmt.Errorf("send request failed: %w", err)
	}

	// 移除本地记录
	d.regMu.Lock()
	delete(d.registrations, namespace)
	d.regMu.Unlock()

	return nil
}

// Discover 发现命名空间中的节点
func (d *Discoverer) Discover(ctx context.Context, namespace string, limit int) ([]types.PeerInfo, error) {
	if !d.started.Load() {
		return nil, ErrNotStarted
	}

	namespace = pkgif.NormalizeNamespace(namespace)

	// 验证命名空间
	if err := ValidateNamespace(namespace); err != nil {
		return nil, err
	}

	// 选择 Point
	point, err := d.selectPoint()
	if err != nil {
		return nil, err
	}

	// 构造请求
	req := NewDiscoverRequest(namespace, limit, nil)

	// 发送请求
	resp, err := d.sendRequest(ctx, point, req)
	if err != nil {
		d.recordFailure(point)
		return nil, fmt.Errorf("send request failed: %w", err)
	}

	// 处理响应
	if resp.DiscoverResponse == nil {
		return nil, errors.New("missing discover response")
	}

	if err := StatusToError(resp.DiscoverResponse.Status, resp.DiscoverResponse.StatusText); err != nil {
		return nil, err
	}

	// 转换为 PeerInfo
	var peers []types.PeerInfo
	for _, pbReg := range resp.DiscoverResponse.Registrations {
		peerInfo, err := RegistrationToPeerInfo(pbReg)
		if err == nil {
			peers = append(peers, peerInfo)
		}
	}

	return peers, nil
}

// ============================================================================
//                              Point 选择
// ============================================================================

// selectPoint 选择一个健康的 Point（轮询 + 健康检查）
func (d *Discoverer) selectPoint() (types.PeerID, error) {
	d.pointsMu.RLock()
	points := d.points
	d.pointsMu.RUnlock()

	if len(points) == 0 {
		return "", ErrNoPoints
	}

	// 轮询选择
	startIdx := int(atomic.AddUint64(&d.roundRobinIndex, 1) - 1)

	// 尝试所有 Points
	for i := 0; i < len(points); i++ {
		idx := (startIdx + i) % len(points)
		point := points[idx]

		// 检查健康状态
		if d.isPointHealthy(point) {
			return point, nil
		}
	}

	return "", ErrAllPointsFailed
}

// isPointHealthy 检查 Point 是否健康
func (d *Discoverer) isPointHealthy(point types.PeerID) bool {
	d.healthMu.RLock()
	health, exists := d.pointHealth[point]
	d.healthMu.RUnlock()

	if !exists {
		return true
	}

	// 检查是否在冷却期
	if health.failCount >= maxFailCount {
		if time.Since(health.lastFailTime) < failCooldown {
			return false
		}
		// 冷却期结束，重置计数
		d.healthMu.Lock()
		health.failCount = 0
		d.healthMu.Unlock()
	}

	return true
}

// recordFailure 记录 Point 失败
func (d *Discoverer) recordFailure(point types.PeerID) {
	d.healthMu.Lock()
	defer d.healthMu.Unlock()

	health, exists := d.pointHealth[point]
	if !exists {
		health = &pointHealthState{}
		d.pointHealth[point] = health
	}

	health.failCount++
	health.lastFailTime = time.Now()
}

// ============================================================================
//                              网络通信
// ============================================================================

// sendRequest 发送请求到 Point
func (d *Discoverer) sendRequest(ctx context.Context, point types.PeerID, req *pb.Message) (*pb.Message, error) {
	// 连接到 Point
	if err := d.host.Connect(ctx, string(point), nil); err != nil {
		return nil, fmt.Errorf("connect failed: %w", err)
	}

	// 创建流
	stream, err := d.host.NewStream(ctx, string(point), ProtocolID)
	if err != nil {
		return nil, fmt.Errorf("create stream failed: %w", err)
	}
	if stream == nil {
		return nil, errors.New("stream is nil")
	}
	defer stream.Close()

	// 发送请求
	if err := WriteMessage(stream, req); err != nil {
		return nil, fmt.Errorf("write message failed: %w", err)
	}

	// 读取响应
	resp, err := ReadMessage(stream)
	if err != nil {
		return nil, fmt.Errorf("read message failed: %w", err)
	}

	return resp, nil
}

// ============================================================================
//                              本地状态管理
// ============================================================================

// recordRegistration 记录本地注册状态
func (d *Discoverer) recordRegistration(namespace string, point types.PeerID, ttl time.Duration) {
	d.regMu.Lock()
	defer d.regMu.Unlock()

	d.registrations[namespace] = &localRegistration{
		Namespace:    namespace,
		TTL:          ttl,
		RegisteredAt: time.Now(),
		Point:        point,
		ExpiresAt:    time.Now().Add(ttl),
	}
}

// ============================================================================
//                              后台循环
// ============================================================================

// renewalLoop 自动续约循环
func (d *Discoverer) renewalLoop() {
	defer d.wg.Done()

	ticker := time.NewTicker(d.config.RenewalInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.renewRegistrations()

		case <-d.ctx.Done():
			return
		}
	}
}

// renewRegistrations 续约所有注册
func (d *Discoverer) renewRegistrations() {
	d.regMu.RLock()
	regsToRenew := make(map[string]*localRegistration)
	for ns, reg := range d.registrations {
		if reg.needsRenewal(d.config.RenewalInterval) {
			regsToRenew[ns] = reg
		}
	}
	d.regMu.RUnlock()

	// 续约
	for ns, reg := range regsToRenew {
		ctx, cancel := context.WithTimeout(d.ctx, d.config.RegisterTimeout)
		_ = d.Register(ctx, ns, reg.TTL)
		cancel()
	}
}
