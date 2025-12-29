package rendezvous

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	pb "github.com/dep2p/go-dep2p/pkg/proto/rendezvous"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              配置
// ============================================================================

// DiscovererConfig Rendezvous 发现器配置
type DiscovererConfig struct {
	// Points 已知的 Rendezvous 点
	Points []types.NodeID

	// DefaultTTL 默认注册 TTL
	DefaultTTL time.Duration

	// RenewalInterval 续约间隔（通常是 TTL/2）
	RenewalInterval time.Duration

	// DiscoverTimeout 发现超时
	DiscoverTimeout time.Duration

	// RegisterTimeout 注册超时
	RegisterTimeout time.Duration

	// MaxRetries 最大重试次数
	MaxRetries int

	// RetryInterval 重试间隔
	RetryInterval time.Duration
}

// DefaultDiscovererConfig 默认配置
func DefaultDiscovererConfig() DiscovererConfig {
	return DiscovererConfig{
		Points:          nil,
		DefaultTTL:      2 * time.Hour,
		RenewalInterval: 1 * time.Hour,
		DiscoverTimeout: 30 * time.Second,
		RegisterTimeout: 30 * time.Second,
		MaxRetries:      3,
		RetryInterval:   5 * time.Second,
	}
}

// ============================================================================
//                              本地注册记录
// ============================================================================

// localRegistration 本地注册记录
type localRegistration struct {
	Namespace    string
	TTL          time.Duration
	RegisteredAt time.Time
	Point        types.NodeID
	ExpiresAt    time.Time
}

// needsRenewal 是否需要续约
func (r *localRegistration) needsRenewal(renewalInterval time.Duration) bool {
	return time.Now().Add(renewalInterval).After(r.ExpiresAt)
}

// ============================================================================
//                              健康状态
// ============================================================================

// 负载均衡常量
const (
	maxFailCount = 3               // 最大连续失败次数，超过后跳过该节点
	failCooldown = 5 * time.Minute // 故障冷却时间，过后重新尝试
)

// pointHealthState Rendezvous 点健康状态
type pointHealthState struct {
	failCount    int       // 连续失败次数
	lastFailTime time.Time // 上次失败时间
}

// ============================================================================
//                              Discoverer 实现
// ============================================================================

// Discoverer Rendezvous 发现器
type Discoverer struct {
	config   DiscovererConfig
	endpoint endpoint.Endpoint

	// 本地 ID
	localID types.NodeID

	// 已知的 Rendezvous 点
	points   []types.NodeID
	pointsMu sync.RWMutex

	// 负载均衡
	roundRobinIndex uint64                             // 轮询索引
	pointHealth     map[types.NodeID]*pointHealthState // 健康状态
	healthMu        sync.RWMutex                       // 健康状态锁

	// 本地注册状态
	registrations map[string]*localRegistration
	regMu         sync.RWMutex

	// 生命周期
	running int32
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewDiscoverer 创建 Rendezvous 发现器
func NewDiscoverer(endpoint endpoint.Endpoint, localID types.NodeID, config DiscovererConfig) *Discoverer {
	return &Discoverer{
		config:        config,
		endpoint:      endpoint,
		localID:       localID,
		points:        config.Points,
		pointHealth:   make(map[types.NodeID]*pointHealthState),
		registrations: make(map[string]*localRegistration),
	}
}

// 确保实现接口
var _ discoveryif.Rendezvous = (*Discoverer)(nil)
var _ discoveryif.NamespaceDiscoverer = (*Discoverer)(nil)

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动发现器
func (d *Discoverer) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&d.running, 0, 1) {
		return errors.New("rendezvous discoverer already running")
	}

	d.ctx, d.cancel = context.WithCancel(ctx)

	// 启动续约循环
	d.wg.Add(1)
	go d.renewalLoop()

	log.Info("rendezvous discoverer started",
		"points", len(d.points),
	)
	return nil
}

// Stop 停止发现器
func (d *Discoverer) Stop() error {
	if !atomic.CompareAndSwapInt32(&d.running, 1, 0) {
		return nil
	}

	if d.cancel != nil {
		d.cancel()
	}
	d.wg.Wait()

	log.Info("rendezvous discoverer stopped")
	return nil
}

// ============================================================================
//                              Rendezvous 接口实现
// ============================================================================

// Register 注册到命名空间
func (d *Discoverer) Register(ctx context.Context, namespace string, ttl time.Duration) error {
	// 验证命名空间
	if err := ValidateNamespace(namespace); err != nil {
		return err
	}

	// 使用默认 TTL
	if ttl <= 0 {
		ttl = d.config.DefaultTTL
	}

	// 获取本地地址
	addrs := d.getLocalAddresses()
	if len(addrs) == 0 {
		return errors.New("no local addresses available")
	}

	// 获取 Rendezvous 点
	point, err := d.selectPoint(ctx)
	if err != nil {
		return err
	}

	// 发送注册请求
	actualTTL, err := d.sendRegister(ctx, point, namespace, addrs, ttl)
	if err != nil {
		return err
	}

	// 记录本地注册
	d.regMu.Lock()
	d.registrations[namespace] = &localRegistration{
		Namespace:    namespace,
		TTL:          actualTTL,
		RegisteredAt: time.Now(),
		Point:        point,
		ExpiresAt:    time.Now().Add(actualTTL),
	}
	d.regMu.Unlock()

	log.Debug("registered to namespace",
		"namespace", namespace,
		"point", point.String(),
		"ttl", actualTTL,
	)

	return nil
}

// Unregister 取消注册
func (d *Discoverer) Unregister(ctx context.Context, namespace string) error {
	// 验证命名空间
	if err := ValidateNamespace(namespace); err != nil {
		return err
	}

	// 获取本地注册
	d.regMu.RLock()
	reg, exists := d.registrations[namespace]
	d.regMu.RUnlock()

	if !exists {
		return nil // 不存在视为成功
	}

	// 发送取消注册请求
		if err := d.sendUnregister(ctx, reg.Point, namespace); err != nil {
			log.Debug("failed to unregister",
				"namespace", namespace,
				"err", err,
			)
			// 继续删除本地记录
		}

	// 删除本地注册
	d.regMu.Lock()
	delete(d.registrations, namespace)
	d.regMu.Unlock()

	log.Debug("unregistered from namespace",
		"namespace", namespace,
	)

	return nil
}

// Discover 发现命名空间中的节点
func (d *Discoverer) Discover(ctx context.Context, namespace string, limit int) ([]discoveryif.PeerInfo, error) {
	// 验证命名空间
	if err := ValidateNamespace(namespace); err != nil {
		return nil, err
	}

	// 获取 Rendezvous 点
	point, err := d.selectPoint(ctx)
	if err != nil {
		return nil, err
	}

	// 发送发现请求
	peers, _, err := d.sendDiscover(ctx, point, namespace, limit, nil)
	if err != nil {
		return nil, err
	}

	return peers, nil
}

// DiscoverAsync 异步发现节点
func (d *Discoverer) DiscoverAsync(ctx context.Context, namespace string) (<-chan discoveryif.PeerInfo, error) {
	// 验证命名空间
	if err := ValidateNamespace(namespace); err != nil {
		return nil, err
	}

	peerCh := make(chan discoveryif.PeerInfo, 100)

	go func() {
		defer close(peerCh)

		var cookie []byte
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			// 获取 Rendezvous 点
			point, err := d.selectPoint(ctx)
			if err != nil {
				log.Debug("failed to select point",
					"err", err,
				)
				return
			}

			// 发送发现请求
			peers, nextCookie, err := d.sendDiscover(ctx, point, namespace, DefaultLimit, cookie)
			if err != nil {
				log.Debug("discover failed",
					"err", err,
				)
				return
			}

			// 发送发现的节点
			for _, peer := range peers {
				select {
				case peerCh <- peer:
				case <-ctx.Done():
					return
				}
			}

			// 检查是否还有更多
			if len(nextCookie) == 0 {
				return
			}
			cookie = nextCookie
		}
	}()

	return peerCh, nil
}

// ============================================================================
//                              NamespaceDiscoverer 接口实现
// ============================================================================

// DiscoverPeers 发现新节点（实现 NamespaceDiscoverer 接口）
//
// 使用 Rendezvous 命名空间发现节点
func (d *Discoverer) DiscoverPeers(ctx context.Context, namespace string) (<-chan discoveryif.PeerInfo, error) {
	return d.DiscoverAsync(ctx, namespace)
}

// ============================================================================
//                              Rendezvous 点管理
// ============================================================================

// AddPoint 添加 Rendezvous 点
func (d *Discoverer) AddPoint(point types.NodeID) {
	d.pointsMu.Lock()
	defer d.pointsMu.Unlock()

	// 检查是否已存在
	for _, p := range d.points {
		if p == point {
			return
		}
	}

	d.points = append(d.points, point)
	log.Debug("added rendezvous point",
		"point", point.String(),
	)
}

// RemovePoint 移除 Rendezvous 点
func (d *Discoverer) RemovePoint(point types.NodeID) {
	d.pointsMu.Lock()
	defer d.pointsMu.Unlock()

	for i, p := range d.points {
		if p == point {
			d.points = append(d.points[:i], d.points[i+1:]...)
			log.Debug("removed rendezvous point",
				"point", point.String(),
			)
			return
		}
	}
}

// Points 返回已知的 Rendezvous 点
func (d *Discoverer) Points() []types.NodeID {
	d.pointsMu.RLock()
	defer d.pointsMu.RUnlock()

	result := make([]types.NodeID, len(d.points))
	copy(result, d.points)
	return result
}

// selectPoint 选择一个 Rendezvous 点
// 使用轮询策略，并跳过故障节点
func (d *Discoverer) selectPoint(_ context.Context) (types.NodeID, error) {
	d.pointsMu.RLock()
	points := d.points
	d.pointsMu.RUnlock()

	if len(points) == 0 {
		return types.NodeID{}, errors.New("no rendezvous points available")
	}

	// 轮询选择，跳过故障节点
	attempts := len(points)
	for i := 0; i < attempts; i++ {
		// 原子递增轮询索引
		idx := atomic.AddUint64(&d.roundRobinIndex, 1) % uint64(len(points))
		point := points[idx]

		// 检查节点是否健康
		if d.isPointHealthy(point) {
			return point, nil
		}

		log.Debug("跳过故障节点",
			"point", point.String())
	}

	// 所有节点都故障，返回第一个（可能已恢复）
	log.Warn("所有 Rendezvous 点都不健康，使用第一个")
	return points[0], nil
}

// isPointHealthy 检查节点是否健康
func (d *Discoverer) isPointHealthy(point types.NodeID) bool {
	d.healthMu.RLock()
	state, exists := d.pointHealth[point]
	d.healthMu.RUnlock()

	if !exists {
		return true // 没有记录，认为健康
	}

	// 检查是否超过冷却时间（已恢复）
	if time.Since(state.lastFailTime) > failCooldown {
		return true
	}

	// 检查失败次数
	return state.failCount < maxFailCount
}

// markPointFailed 标记节点失败
func (d *Discoverer) markPointFailed(point types.NodeID) {
	d.healthMu.Lock()
	defer d.healthMu.Unlock()

	state, exists := d.pointHealth[point]
	if !exists {
		state = &pointHealthState{}
		d.pointHealth[point] = state
	}

	state.failCount++
	state.lastFailTime = time.Now()

	log.Debug("标记节点失败",
		"point", point.String(),
		"failCount", state.failCount)
}

// markPointSuccess 标记节点成功
func (d *Discoverer) markPointSuccess(point types.NodeID) {
	d.healthMu.Lock()
	defer d.healthMu.Unlock()

	// 成功后重置失败计数
	if state, exists := d.pointHealth[point]; exists {
		if state.failCount > 0 {
			log.Debug("节点恢复健康",
				"point", point.String())
		}
		state.failCount = 0
	}
}

// ============================================================================
//                              协议通信
// ============================================================================

// sendRegister 发送注册请求
func (d *Discoverer) sendRegister(ctx context.Context, point types.NodeID, namespace string, addrs []string, ttl time.Duration) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, d.config.RegisterTimeout)
	defer cancel()

	// 连接到 Rendezvous 点
	conn, err := d.endpoint.Connect(ctx, point)
	if err != nil {
		d.markPointFailed(point)
		return 0, err
	}

	// 打开流
	stream, err := conn.OpenStream(ctx, ProtocolID)
	if err != nil {
		d.markPointFailed(point)
		return 0, err
	}
	defer func() { _ = stream.Close() }()

	// 发送请求
	req := NewRegisterRequest(namespace, d.localID, addrs, ttl)
	if err := WriteMessage(stream, req); err != nil {
		d.markPointFailed(point)
		return 0, err
	}

	// 读取响应
	resp, err := ReadMessage(stream)
	if err != nil {
		d.markPointFailed(point)
		return 0, err
	}

	if resp.Type != pb.MessageType_MESSAGE_TYPE_REGISTER_RESPONSE {
		d.markPointFailed(point)
		return 0, errors.New("unexpected response type")
	}

	regResp := resp.RegisterResponse
	if err := StatusToError(regResp.Status, regResp.StatusText); err != nil {
		d.markPointFailed(point)
		return 0, err
	}

	// 成功，标记节点健康
	d.markPointSuccess(point)
	return time.Duration(regResp.Ttl) * time.Second, nil
}

// sendUnregister 发送取消注册请求
func (d *Discoverer) sendUnregister(ctx context.Context, point types.NodeID, namespace string) error {
	ctx, cancel := context.WithTimeout(ctx, d.config.RegisterTimeout)
	defer cancel()

	// 连接到 Rendezvous 点
	conn, err := d.endpoint.Connect(ctx, point)
	if err != nil {
		d.markPointFailed(point)
		return err
	}

	// 打开流
	stream, err := conn.OpenStream(ctx, ProtocolID)
	if err != nil {
		d.markPointFailed(point)
		return err
	}
	defer func() { _ = stream.Close() }()

	// 发送请求
	req := NewUnregisterRequest(namespace, d.localID)
	if err := WriteMessage(stream, req); err != nil {
		d.markPointFailed(point)
		return err
	}

	// 读取响应
	resp, err := ReadMessage(stream)
	if err != nil {
		d.markPointFailed(point)
		return err
	}

	if resp.Type != pb.MessageType_MESSAGE_TYPE_REGISTER_RESPONSE {
		d.markPointFailed(point)
		return errors.New("unexpected response type")
	}

	regResp := resp.RegisterResponse
	if err := StatusToError(regResp.Status, regResp.StatusText); err != nil {
		d.markPointFailed(point)
		return err
	}

	d.markPointSuccess(point)
	return nil
}

// sendDiscover 发送发现请求
func (d *Discoverer) sendDiscover(ctx context.Context, point types.NodeID, namespace string, limit int, cookie []byte) ([]discoveryif.PeerInfo, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, d.config.DiscoverTimeout)
	defer cancel()

	// 连接到 Rendezvous 点
	conn, err := d.endpoint.Connect(ctx, point)
	if err != nil {
		d.markPointFailed(point)
		return nil, nil, err
	}

	// 打开流
	stream, err := conn.OpenStream(ctx, ProtocolID)
	if err != nil {
		d.markPointFailed(point)
		return nil, nil, err
	}
	defer func() { _ = stream.Close() }()

	// 发送请求
	req := NewDiscoverRequest(namespace, limit, cookie)
	if err := WriteMessage(stream, req); err != nil {
		d.markPointFailed(point)
		return nil, nil, err
	}

	// 读取响应
	resp, err := ReadMessage(stream)
	if err != nil {
		d.markPointFailed(point)
		return nil, nil, err
	}

	if resp.Type != pb.MessageType_MESSAGE_TYPE_DISCOVER_RESPONSE {
		d.markPointFailed(point)
		return nil, nil, errors.New("unexpected response type")
	}

	discResp := resp.DiscoverResponse
	if err := StatusToError(discResp.Status, discResp.StatusText); err != nil {
		d.markPointFailed(point)
		return nil, nil, err
	}

	// 成功，标记节点健康
	d.markPointSuccess(point)

	// 转换结果
	peers := make([]discoveryif.PeerInfo, 0, len(discResp.Registrations))
	for _, reg := range discResp.Registrations {
		peer, err := RegistrationToPeerInfo(reg)
		if err != nil {
			log.Debug("invalid registration",
				"err", err,
			)
			continue
		}
		peers = append(peers, peer)
	}

	return peers, discResp.Cookie, nil
}

// ============================================================================
//                              续约
// ============================================================================

// renewalLoop 续约循环
func (d *Discoverer) renewalLoop() {
	defer d.wg.Done()

	// 检查 ctx 是否为 nil（防止 Start() 未调用）
	if d.ctx == nil {
		return
	}

	ticker := time.NewTicker(d.config.RenewalInterval / 2)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.renewRegistrations()
		}
	}
}

// renewRegistrations 续约注册
func (d *Discoverer) renewRegistrations() {
	d.regMu.RLock()
	toRenew := make([]*localRegistration, 0)
	for _, reg := range d.registrations {
		if reg.needsRenewal(d.config.RenewalInterval) {
			toRenew = append(toRenew, reg)
		}
	}
	d.regMu.RUnlock()

	for _, reg := range toRenew {
		ctx, cancel := context.WithTimeout(d.ctx, d.config.RegisterTimeout)

		addrs := d.getLocalAddresses()
		if len(addrs) == 0 {
			cancel()
			continue
		}

		actualTTL, err := d.sendRegister(ctx, reg.Point, reg.Namespace, addrs, reg.TTL)
		cancel()

		if err != nil {
			log.Debug("failed to renew registration",
				"namespace", reg.Namespace,
				"err", err,
			)
			continue
		}

		// 更新本地记录
		d.regMu.Lock()
		if r, exists := d.registrations[reg.Namespace]; exists {
			r.TTL = actualTTL
			r.RegisteredAt = time.Now()
			r.ExpiresAt = time.Now().Add(actualTTL)
		}
		d.regMu.Unlock()

		log.Debug("renewed registration",
			"namespace", reg.Namespace,
			"ttl", actualTTL,
		)
	}
}

// ============================================================================
//                              辅助方法
// ============================================================================

// getLocalAddresses 获取本地地址
func (d *Discoverer) getLocalAddresses() []string {
	if d.endpoint == nil {
		return nil
	}

	addrs := d.endpoint.ListenAddrs()
	result := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		result = append(result, addr.String())
	}
	return result
}

// ============================================================================
//                              Announcer 接口实现
// ============================================================================

// Announce 通告本节点（实现 Announcer 接口）
//
// 将节点注册到指定命名空间，使其他节点可以发现。
// 使用默认 TTL（2 小时）。
func (d *Discoverer) Announce(ctx context.Context, namespace string) error {
	return d.Register(ctx, namespace, d.config.DefaultTTL)
}

// AnnounceWithTTL 带 TTL 的通告（实现 Announcer 接口）
//
// 将节点注册到指定命名空间，使用指定的 TTL。
func (d *Discoverer) AnnounceWithTTL(ctx context.Context, namespace string, ttl time.Duration) error {
	return d.Register(ctx, namespace, ttl)
}

// StopAnnounce 停止通告（实现 Announcer 接口）
//
// 从指定命名空间取消注册。
// 如果 namespace 为空，取消所有注册。
func (d *Discoverer) StopAnnounce(namespace string) error {
	if namespace == "" {
		// 取消所有注册
		d.regMu.RLock()
		namespaces := make([]string, 0, len(d.registrations))
		for ns := range d.registrations {
			namespaces = append(namespaces, ns)
		}
		d.regMu.RUnlock()

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		var lastErr error
		for _, ns := range namespaces {
			if err := d.Unregister(ctx, ns); err != nil {
				lastErr = err
			}
		}
		return lastErr
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return d.Unregister(ctx, namespace)
}

// 确保 Discoverer 也实现 Announcer 接口
var _ discoveryif.Announcer = (*Discoverer)(nil)
