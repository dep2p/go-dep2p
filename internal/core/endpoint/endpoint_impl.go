// Package endpoint 提供 Endpoint 聚合模块的实现
package endpoint

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/address"
	"github.com/dep2p/go-dep2p/internal/debuglog"
	"github.com/dep2p/go-dep2p/internal/util/logger"
	addressif "github.com/dep2p/go-dep2p/pkg/interfaces/address"
	connmgrif "github.com/dep2p/go-dep2p/pkg/interfaces/connmgr"
	coreif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	muxerif "github.com/dep2p/go-dep2p/pkg/interfaces/muxer"
	natif "github.com/dep2p/go-dep2p/pkg/interfaces/nat"
	protocolif "github.com/dep2p/go-dep2p/pkg/interfaces/protocol"
	reachabilityif "github.com/dep2p/go-dep2p/pkg/interfaces/reachability"
	securityif "github.com/dep2p/go-dep2p/pkg/interfaces/security"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 包级别日志实例
var log = logger.Logger("endpoint")

// ============================================================================
//                              Endpoint 实现
// ============================================================================

// Endpoint Endpoint 实现
type Endpoint struct {
	// 核心依赖
	identity     identityif.Identity
	transport    transportif.Transport
	security     securityif.SecureTransport
	muxerFactory muxerif.MuxerFactory

	// 传输注册表 - 管理多种传输（QUIC, TCP, Relay）
	// 当设置后，Connect() 会使用 Registry 选择合适的传输
	transportRegistry *TransportRegistry
	addressRanker     transportif.AddressRanker

	// 可选依赖
	discovery coreif.DiscoveryService
	nat       natif.NATService
	// 注意：不再存储 RelayClient 引用
	// Relay 通过 TransportRegistry + RelayTransport 透明工作

	// 可达性协调器（可选）：作为通告地址的唯一真源
	reachabilityCoordinator reachabilityif.Coordinator
	reachabilityMu          sync.RWMutex

	// 协议路由器（可选）- 用于统一管理协议处理器
	protocolRouter protocolif.Router

	// 连接管理（可选）- 水位线控制和连接保护
	connManager connmgrif.ConnectionManager

	// 连接门控（可选）- 黑名单和连接拦截
	connGater connmgrif.ConnectionGater

	// 地址簿
	addressBook *AddressBook

	// 协议处理器（内部 fallback，如果没有注入 protocolRouter）
	handlers   map[coreif.ProtocolID]coreif.ProtocolHandler
	handlersMu sync.RWMutex

	// 连接管理
	conns   map[coreif.NodeID]*Connection
	connsMu sync.RWMutex

	// 并发连接去重（REQ-CONN-005）
	// 追踪正在进行的拨号操作，避免对同一节点并发发起多个拨号
	dialInflight sync.Map // map[types.NodeID]*dialFuture

	// 监听器
	listeners   []transportif.Listener
	listenersMu sync.RWMutex

	// 地址管理
	listenAddrs     []coreif.Address
	advertisedAddrs []coreif.Address
	addrsMu         sync.RWMutex

	// 接受队列
	acceptCh chan *Connection

	// 状态
	mu        sync.RWMutex
	started   bool
	closed    bool
	closeCh   chan struct{}
	closeOnce sync.Once

	// 入站连接速率限制
	rateLimiter *tokenBucketLimiter

	// 配置
	config *Config

	// 连接回调列表（用于通知 messaging 等模块）
	connCallbacks   []func(nodeID coreif.NodeID, outbound bool)
	connCallbacksMu sync.RWMutex

	// 事件回调列表（用于连接生命周期事件通知）
	eventCallbacks   []func(event interface{})
	eventCallbacksMu sync.RWMutex
}

// tokenBucketLimiter 令牌桶速率限制器
type tokenBucketLimiter struct {
	tokens     int64 // 当前令牌数
	maxTokens  int64 // 最大令牌数（突发容量）
	refillRate int64 // 每秒补充的令牌数
	lastRefill int64 // 上次补充时间（Unix 纳秒）
	mu         sync.Mutex
}

// newTokenBucketLimiter 创建令牌桶限制器
func newTokenBucketLimiter(rate, burst int) *tokenBucketLimiter {
	if rate <= 0 {
		return nil // 不限制
	}
	return &tokenBucketLimiter{
		tokens:     int64(burst),
		maxTokens:  int64(burst),
		refillRate: int64(rate),
		lastRefill: time.Now().UnixNano(),
	}
}

// Allow 尝试获取一个令牌，返回是否允许
func (l *tokenBucketLimiter) Allow() bool {
	if l == nil {
		return true // 未配置限制器时允许所有
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now().UnixNano()
	elapsed := now - l.lastRefill

	// 计算应该补充的令牌数
	tokensToAdd := elapsed * l.refillRate / int64(time.Second)
	if tokensToAdd > 0 {
		l.tokens = min64(l.tokens+tokensToAdd, l.maxTokens)
		l.lastRefill = now
	}

	// 尝试消费一个令牌
	if l.tokens > 0 {
		l.tokens--
		return true
	}
	return false
}

// min64 返回两个 int64 中的较小值
func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

// RejectedCount 返回被拒绝的请求计数（用于监控）
func (l *tokenBucketLimiter) RejectedCount() int64 {
	if l == nil {
		return 0
	}
	return atomic.LoadInt64(&l.tokens) // 这里可以扩展实际计数
}

// ============================================================================
//                              并发连接去重（REQ-CONN-005）
// ============================================================================

// dialFuture 表示一个正在进行的拨号操作
// 多个并发的 Connect 调用可以等待同一个 dialFuture 完成
type dialFuture struct {
	done chan struct{}      // 拨号完成信号
	conn coreif.Connection  // 成功时的连接（使用接口类型以兼容不同返回路径）
	err  error              // 失败时的错误
	once sync.Once          // 确保只完成一次
}

// newDialFuture 创建新的拨号 future
func newDialFuture() *dialFuture {
	return &dialFuture{
		done: make(chan struct{}),
	}
}

// complete 完成拨号操作（成功或失败）
func (f *dialFuture) complete(conn coreif.Connection, err error) {
	f.once.Do(func() {
		f.conn = conn
		f.err = err
		close(f.done)
	})
}

// wait 等待拨号完成并返回结果
func (f *dialFuture) wait(ctx context.Context) (coreif.Connection, error) {
	select {
	case <-f.done:
		return f.conn, f.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// NewEndpoint 创建 Endpoint（兼容旧接口）
func NewEndpoint(
	identity identityif.Identity,
	transport transportif.Transport,
	discovery coreif.DiscoveryService,
	nat natif.NATService,
) *Endpoint {
	return NewEndpointWithConfig(
		identity,
		transport,
		nil, // security
		nil, // muxerFactory
		discovery,
		nat,
		nil, // protocolRouter
		nil, // connManager
		nil, // connGater
		DefaultConfig(),
	)
}

// NewEndpointWithConfig 创建带配置的 Endpoint
//
// 注意：Relay 不作为参数传入，通过 TransportRegistry + RelayTransport 透明工作
func NewEndpointWithConfig(
	identity identityif.Identity,
	transport transportif.Transport,
	security securityif.SecureTransport,
	muxerFactory muxerif.MuxerFactory,
	discovery coreif.DiscoveryService,
	nat natif.NATService,
	protocolRouter protocolif.Router,
	connManager connmgrif.ConnectionManager,
	connGater connmgrif.ConnectionGater,
	config *Config,
) *Endpoint {
	if config == nil {
		config = DefaultConfig()
	}

	// 创建入站连接速率限制器
	var rateLimiter *tokenBucketLimiter
	if config.InboundRateLimit > 0 {
		rateLimiter = newTokenBucketLimiter(config.InboundRateLimit, config.InboundRateBurst)
		log.Debug("启用入站连接速率限制",
			"rate", config.InboundRateLimit,
			"burst", config.InboundRateBurst)
	}

	ep := &Endpoint{
		identity:       identity,
		transport:      transport,
		security:       security,
		muxerFactory:   muxerFactory,
		discovery:      discovery,
		nat:            nat,
		protocolRouter: protocolRouter,
		connManager:    connManager,
		connGater:      connGater,
		addressBook:    NewAddressBook(),
		handlers:       make(map[coreif.ProtocolID]coreif.ProtocolHandler),
		conns:          make(map[coreif.NodeID]*Connection),
		listeners:      make([]transportif.Listener, 0),
		acceptCh:       make(chan *Connection, 16),
		closeCh:        make(chan struct{}),
		rateLimiter:    rateLimiter,
		config:         config,
		addressRanker:  &DefaultAddressRanker{},
	}

	// 初始化传输注册表并注册主传输
	ep.transportRegistry = NewTransportRegistry()
	if transport != nil {
		if err := ep.transportRegistry.AddTransport(transport); err != nil {
			log.Warn("注册主传输失败", "err", err)
		}
	}

	return ep
}

// 注意：不再需要 SetRelayClient 方法
// Relay 通过 TransportRegistry + RelayTransport 透明工作

// 确保实现接口
var _ coreif.Endpoint = (*Endpoint)(nil)

// ==================== 身份信息 ====================

// ID 返回节点 ID
func (e *Endpoint) ID() coreif.NodeID {
	if e.identity == nil {
		return coreif.EmptyNodeID
	}
	return e.identity.ID()
}

// PublicKey 返回公钥
func (e *Endpoint) PublicKey() coreif.PublicKey {
	if e.identity == nil {
		return nil
	}
	return e.identity.PublicKey()
}

// ==================== 连接管理 ====================

// Connect 连接到指定节点
//
// REQ-CONN-005: 并发连接去重
// - 多个并发的 Connect 调用对同一节点只会发起一个实际拨号
// - 后续调用会等待第一个拨号完成并复用其结果
// - 不会产生 goroutine/连接泄漏
func (e *Endpoint) Connect(ctx context.Context, nodeID coreif.NodeID) (coreif.Connection, error) {
	// 检查是否连接自己
	if nodeID.Equal(e.ID()) {
		return nil, coreif.ErrSelfConnect
	}

	// 检查已有连接（快速路径）
	if conn, ok := e.Connection(nodeID); ok {
		return conn, nil
	}

	// 尝试加入或创建拨号 future（并发去重核心逻辑）
	future := newDialFuture()
	if existing, loaded := e.dialInflight.LoadOrStore(nodeID, future); loaded {
		// 已有进行中的拨号，等待其完成
		existingFuture := existing.(*dialFuture)
		log.Debug("复用进行中的拨号",
			"nodeID", nodeID.ShortString())
		conn, err := existingFuture.wait(ctx)
		if err != nil {
			return nil, err
		}
		return conn, nil
	}

	// 我们是第一个发起拨号的，负责实际拨号
	// 确保完成后清理 dialInflight
	defer e.dialInflight.Delete(nodeID)

	// 再次检查已有连接（可能在等待 LoadOrStore 期间建立）
	if conn, ok := e.Connection(nodeID); ok {
		future.complete(conn, nil)
		return conn, nil
	}

	// 获取所有可用地址（直连 + 中继）
	addrs, err := e.getAllAddresses(ctx, nodeID)
		if err != nil {
				future.complete(nil, err)
				return nil, err
	}

	if len(addrs) == 0 {
		future.complete(nil, coreif.ErrNoAddresses)
		return nil, coreif.ErrNoAddresses
	}

	// 实际执行连接
	conn, err := e.connectWithAddrsInternal(ctx, nodeID, addrs)
	future.complete(conn, err)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// ConnectWithAddrs 使用指定地址连接
//
// REQ-CONN-005: 并发连接去重
// - 多个并发的 ConnectWithAddrs 调用对同一节点只会发起一个实际拨号
func (e *Endpoint) ConnectWithAddrs(ctx context.Context, nodeID coreif.NodeID, addrs []coreif.Address) (coreif.Connection, error) {
	// 检查是否连接自己
	if nodeID.Equal(e.ID()) {
		return nil, coreif.ErrSelfConnect
	}

	// 检查已有连接（快速路径）
	if conn, ok := e.Connection(nodeID); ok {
		return conn, nil
	}

	if len(addrs) == 0 {
		return nil, coreif.ErrNoAddresses
	}

	// 尝试加入或创建拨号 future（并发去重核心逻辑）
	future := newDialFuture()
	if existing, loaded := e.dialInflight.LoadOrStore(nodeID, future); loaded {
		// 已有进行中的拨号，等待其完成
		existingFuture := existing.(*dialFuture)
		log.Debug("复用进行中的拨号（ConnectWithAddrs）",
			"nodeID", nodeID.ShortString())
		conn, err := existingFuture.wait(ctx)
		if err != nil {
			return nil, err
		}
		return conn, nil
	}

	// 我们是第一个发起拨号的，负责实际拨号
	defer e.dialInflight.Delete(nodeID)

	// 再次检查已有连接
	if conn, ok := e.Connection(nodeID); ok {
		future.complete(conn, nil)
		return conn, nil
	}

	// 实际执行连接
	conn, err := e.connectWithAddrsInternal(ctx, nodeID, addrs)
	future.complete(conn, err)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// connectWithAddrsInternal 实际执行连接（内部方法，不含去重逻辑）
//
// 使用 TransportRegistry 选择合适的传输，支持多传输回退：
//   - 直连地址优先尝试
//   - 直连全部失败后尝试中继地址
func (e *Endpoint) connectWithAddrsInternal(ctx context.Context, nodeID coreif.NodeID, addrs []coreif.Address) (*Connection, error) {
	// 对地址进行排序（直连优先）
	rankedAddrs := e.rankAddresses(addrs)

	log.Debug("尝试连接到节点",
		"nodeID", nodeID.ShortString(),
		"addrCount", len(rankedAddrs),
		"directCount", countDirectAddrs(rankedAddrs),
		"relayCount", countRelayAddrs(rankedAddrs))

	var lastErr error

	// 尝试每个地址
	for _, addr := range rankedAddrs {
		conn, err := e.dialAddrWithRegistry(ctx, nodeID, addr)
		if err != nil {
			log.Debug("连接地址失败",
				"addr", addr.String(),
				"isRelay", types.IsRelayAddr(addr.String()),
				"err", err)
			lastErr = err
			continue
		}

		log.Info("成功连接到节点",
			"nodeID", nodeID.ShortString(),
			"addr", addr.String(),
			"viaRelay", types.IsRelayAddr(addr.String()))

		return conn, nil
	}

	return nil, fmt.Errorf("%w: %v", coreif.ErrAllDialsFailed, lastErr)
}

// rankAddresses 对地址进行优先级排序
func (e *Endpoint) rankAddresses(addrs []coreif.Address) []coreif.Address {
	if e.addressRanker != nil {
		return e.addressRanker.RankAddresses(addrs)
	}
	// 默认：直连优先
	return (&DefaultAddressRanker{}).RankAddresses(addrs)
}

// countDirectAddrs 统计直连地址数量
func countDirectAddrs(addrs []coreif.Address) int {
	count := 0
	for _, addr := range addrs {
		if !types.IsRelayAddr(addr.String()) {
			count++
		}
	}
	return count
}

// countRelayAddrs 统计中继地址数量
func countRelayAddrs(addrs []coreif.Address) int {
	count := 0
	for _, addr := range addrs {
		if types.IsRelayAddr(addr.String()) {
			count++
		}
	}
	return count
}

// getAllAddresses 获取节点的所有可用地址（直连 + 中继）
//
// 地址来源（全部合并，去重）：
//  1. 地址簿（已知直连地址）
//  2. 发现服务（DHT 查询）- 始终查询，可能返回新的中继地址
//  3. 中继地址（通过已知中继节点构建）
func (e *Endpoint) getAllAddresses(ctx context.Context, nodeID coreif.NodeID) ([]coreif.Address, error) {
	var allAddrs []coreif.Address
	seen := make(map[string]bool)

	// 辅助函数：添加地址（去重）
	addAddr := func(addr coreif.Address) {
		key := addr.String()
		if !seen[key] {
			seen[key] = true
			allAddrs = append(allAddrs, addr)
		}
	}

	// 1. 从地址簿获取直连地址
	directAddrs := e.addressBook.Get(nodeID)
	for _, addr := range directAddrs {
		addAddr(addr)
	}

	// 2. 始终尝试发现服务（DHT 可能有更新的地址，包括中继地址）
	// 这是 Relay Transport Integration 的关键：其他节点通过 DHT 发布的中继地址需要被发现
	if e.discovery != nil {
		discoveredAddrs, err := e.discovery.FindPeer(ctx, nodeID)
		if err != nil {
			// 如果是 context 超时或取消错误，直接传播
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
				return nil, err
			}
			log.Debug("发现服务查找失败", "err", err)
		} else {
			for _, addr := range discoveredAddrs {
				addAddr(addr)
			}
			// 保存新发现的直连地址到地址簿（中继地址不需要保存）
			var newDirectAddrs []coreif.Address
			for _, addr := range discoveredAddrs {
				if !types.IsRelayAddr(addr.String()) {
					newDirectAddrs = append(newDirectAddrs, addr)
				}
			}
			if len(newDirectAddrs) > 0 {
				e.addressBook.Add(nodeID, newDirectAddrs...)
			}
		}
	}

	// 3. 构建中继地址（作为回退）
	relayAddrs := e.buildRelayAddresses(ctx, nodeID)
	for _, addr := range relayAddrs {
		addAddr(addr)
	}

	return allAddrs, nil
}

// buildRelayAddresses 为目标节点构建中继地址
//
// 注意：中继地址现在通过 TransportRegistry + RelayTransport 透明处理，
// 此方法作为兼容保留，当前返回空列表。
// 如果需要主动构建中继地址，应从 AutoRelay 获取已预留的中继节点。
func (e *Endpoint) buildRelayAddresses(ctx context.Context, destID coreif.NodeID) []coreif.Address {
	// 中继地址由 AutoRelay / RelayTransport 自动管理
	// Endpoint 不再主动持有 RelayClient 引用
	return nil
}

// newSimpleAddr 创建简单地址（用于兼容旧代码）
func newSimpleAddr(addrStr string) coreif.Address {
	// 尝试解析为 multiaddr，如果失败则创建包装器
	if strings.HasPrefix(addrStr, "/") {
		if addr, err := address.Parse(addrStr); err == nil {
			return addr
		}
	}
	// 对于非 multiaddr 格式，使用类型转换创建
	return address.NewAddr(types.Multiaddr(addrStr))
}

// dialAddrWithRegistry 使用 TransportRegistry 拨号到单个地址
func (e *Endpoint) dialAddrWithRegistry(ctx context.Context, nodeID coreif.NodeID, addr coreif.Address) (*Connection, error) {
	// 选择合适的传输
	var selectedTransport transportif.Transport
	if e.transportRegistry != nil {
		selectedTransport = e.transportRegistry.TransportForDialing(addr)
	}
	if selectedTransport == nil {
		// 回退到主传输
		selectedTransport = e.transport
	}

	if selectedTransport == nil {
		return nil, fmt.Errorf("无可用传输: %w", coreif.ErrConnectionClosed)
	}

	log.Debug("选择传输拨号",
		"addr", addr.String(),
		"protocols", selectedTransport.Protocols(),
		"proxy", selectedTransport.Proxy())

	// 使用选中的传输进行拨号
	return e.dialAddrWithTransport(ctx, nodeID, addr, selectedTransport)
}

// dialAddrWithTransport 使用指定传输拨号到单个地址
func (e *Endpoint) dialAddrWithTransport(ctx context.Context, nodeID coreif.NodeID, addr coreif.Address, transport transportif.Transport) (*Connection, error) {
	// 使用 ConnGater 检查是否允许拨号到该节点
	if e.connGater != nil && !e.connGater.InterceptPeerDial(nodeID) {
		log.Debug("连接被门控拦截",
			"nodeID", nodeID.ShortString(),
			"addr", addr.String())
		return nil, fmt.Errorf("连接被门控拦截: %w", coreif.ErrConnectionClosed)
	}

	// 使用 ConnManager 检查是否允许新连接
	if e.connManager != nil && !e.connManager.AllowConnection(nodeID, types.DirOutbound) {
		log.Debug("连接被管理器拒绝（连接数限制）",
			"nodeID", nodeID.ShortString())
		return nil, fmt.Errorf("连接被管理器拒绝: %w", coreif.ErrConnectionClosed)
	}

	// 1. 建立传输层连接
	rawConn, err := transport.Dial(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("传输层连接失败: %w", err)
	}

	// 2. 检查是否已是安全连接（如 QUIC 内置 TLS）
	var secureConn securityif.SecureConn
	if sc, ok := rawConn.(securityif.SecureConn); ok {
		// 连接已经是安全的（如 QUIC），直接使用
		log.Debug("传输层连接已安全，跳过额外安全握手",
			"transport", rawConn.Transport())
		secureConn = sc
	} else if transport.Proxy() {
		// 代理传输（如 Relay）：需要额外的安全协商
		// 注意：Relay 连接是通过中继转发的，需要端到端安全
		if e.security == nil {
			_ = rawConn.Close()
			return nil, fmt.Errorf("代理连接需要安全层: %w", coreif.ErrConnectionClosed)
		}
		secureConn, err = e.security.SecureOutbound(ctx, rawConn, nodeID)
		if err != nil {
			_ = rawConn.Close()
			return nil, fmt.Errorf("代理连接安全握手失败: %w", err)
		}
	} else {
		// 普通连接（如 TCP），需要安全握手
		if e.security == nil {
			_ = rawConn.Close()
			return nil, fmt.Errorf("出站连接需要安全层: %w", coreif.ErrConnectionClosed)
		}
		secureConn, err = e.security.SecureOutbound(ctx, rawConn, nodeID)
		if err != nil {
			_ = rawConn.Close()
			return nil, fmt.Errorf("安全握手失败: %w", err)
		}
	}

	// 验证节点 ID
	if !secureConn.RemoteIdentity().Equal(nodeID) {
		_ = secureConn.Close()
		return nil, coreif.ErrIdentityMismatch
	}

	// 3. 创建多路复用器
	var muxer muxerif.Muxer
	if e.muxerFactory != nil {
		// 对于 QUIC 连接，muxerFactory 会使用 QUIC 内置的多路复用
		muxer, err = e.muxerFactory.NewMuxer(secureConn, false) // 客户端
		if err != nil {
			_ = secureConn.Close()
			return nil, fmt.Errorf("创建多路复用器失败: %w", err)
		}
	}

	// 4. 创建连接包装
	conn := NewConnection(
		secureConn,
		muxer,
		types.DirOutbound,
		e,
	)

	// 5. 注册连接
	e.addConnection(nodeID, conn)

	// 5.1 出站连接成功事件：触发 witness 报告（无外部依赖升级路径）
	e.reachabilityMu.RLock()
	rc := e.reachabilityCoordinator
	e.reachabilityMu.RUnlock()
	if rc != nil {
		rc.OnOutboundConnected(conn, addr.String())
	}

	// 6. 启动流接受循环
	// 使用独立的 context，因为传入的 ctx 可能是带超时的连接 context
	conn.StartStreamAcceptLoop(context.Background())

	// 7. 通知连接管理器
	if e.connManager != nil {
		e.connManager.NotifyConnected(conn)
	}

	return conn, nil
}

// dialAddr 拨号到单个地址（兼容旧代码，使用主传输）
//
// Deprecated: 请使用 dialAddrWithRegistry，它支持多传输选择
func (e *Endpoint) dialAddr(ctx context.Context, nodeID coreif.NodeID, addr coreif.Address) (*Connection, error) {
	// 使用 TransportRegistry 进行拨号
	return e.dialAddrWithRegistry(ctx, nodeID, addr)
}

// Ping 测试连接延迟（内部方法：建立传输+安全连接后立即关闭）
// 这里保留原始代码以供 Ping 使用
func (e *Endpoint) pingInternal(ctx context.Context, nodeID coreif.NodeID, addr coreif.Address) (*Connection, error) {
	// 检查必要的依赖
	if e.transport == nil {
		return nil, fmt.Errorf("传输层未配置: %w", coreif.ErrConnectionClosed)
	}

	// 1. 建立传输层连接
	rawConn, err := e.transport.Dial(ctx, addr)
	if err != nil {
		return nil, fmt.Errorf("传输层连接失败: %w", err)
	}

	// 2. 检查是否已是安全连接（如 QUIC 内置 TLS）
	var secureConn securityif.SecureConn
	if sc, ok := rawConn.(securityif.SecureConn); ok {
		// 连接已经是安全的（如 QUIC），直接使用
		secureConn = sc
	} else {
		// 普通连接（如 TCP），需要安全握手
		if e.security == nil {
			_ = rawConn.Close()
			return nil, fmt.Errorf("出站连接需要安全层: %w", coreif.ErrConnectionClosed)
		}
		secureConn, err = e.security.SecureOutbound(ctx, rawConn, nodeID)
		if err != nil {
			_ = rawConn.Close()
			return nil, fmt.Errorf("安全握手失败: %w", err)
		}
	}

	// 验证节点 ID
	if !secureConn.RemoteIdentity().Equal(nodeID) {
		_ = secureConn.Close()
		return nil, coreif.ErrIdentityMismatch
	}

	// 3. 创建多路复用器
	var muxer muxerif.Muxer
	if e.muxerFactory != nil {
		muxer, err = e.muxerFactory.NewMuxer(secureConn, false)
		if err != nil {
			_ = secureConn.Close()
			return nil, fmt.Errorf("创建多路复用器失败: %w", err)
		}
	}

	// 4. 创建连接包装
	conn := NewConnection(
		secureConn,
		muxer,
		types.DirOutbound,
		e,
	)

	// 注意：Ping 不注册连接，不启动流接受循环
	return conn, nil
}

// VerifyOutboundHandshake 仅执行出站 Dial + 安全握手校验（不注册连接，不复用已有连接）
//
// 用途：Phase 3 dial-back 可达性验证（/dep2p/sys/reachability/1.0.0）。
// 该验证必须绕开 Connect/ConnectWithAddrs 的“已有连接直接返回”逻辑，
// 否则在 dial-back 场景中（双方已连通）会产生“假成功”，无法验证候选地址本身是否可达。
func (e *Endpoint) VerifyOutboundHandshake(ctx context.Context, nodeID coreif.NodeID, addr coreif.Address) (time.Duration, error) {
	start := time.Now()

	if e.transport == nil {
		return 0, fmt.Errorf("传输层未配置: %w", coreif.ErrConnectionClosed)
	}

	// 1) 传输层 Dial
	rawConn, err := e.transport.Dial(ctx, addr)
	if err != nil {
		return 0, fmt.Errorf("传输层连接失败: %w", err)
	}

	// 2) 安全握手（或复用 QUIC 内置安全连接）
	var secureConn securityif.SecureConn
	if sc, ok := rawConn.(securityif.SecureConn); ok {
		secureConn = sc
	} else {
		if e.security == nil {
			_ = rawConn.Close()
			return 0, fmt.Errorf("出站连接需要安全层: %w", coreif.ErrConnectionClosed)
		}
		secureConn, err = e.security.SecureOutbound(ctx, rawConn, nodeID)
		if err != nil {
			_ = rawConn.Close()
			return 0, fmt.Errorf("安全握手失败: %w", err)
		}
	}

	// 3) 校验对端身份
	if !secureConn.RemoteIdentity().Equal(nodeID) {
		_ = secureConn.Close()
		return 0, coreif.ErrIdentityMismatch
	}

	// 4) 关闭连接（不进入 muxer，不写入连接表）
	_ = secureConn.Close()

	return time.Since(start), nil
}

// Disconnect 断开连接
func (e *Endpoint) Disconnect(nodeID coreif.NodeID) error {
	e.connsMu.Lock()
	conn, ok := e.conns[nodeID]
	if ok {
		delete(e.conns, nodeID)
	}
	e.connsMu.Unlock()

	if !ok {
		return nil
	}

	return conn.Close()
}

// Connections 返回所有连接
func (e *Endpoint) Connections() []coreif.Connection {
	e.connsMu.RLock()
	defer e.connsMu.RUnlock()

	conns := make([]coreif.Connection, 0, len(e.conns))
	for _, conn := range e.conns {
		conns = append(conns, conn)
	}
	return conns
}

// Connection 获取指定连接
func (e *Endpoint) Connection(nodeID coreif.NodeID) (coreif.Connection, bool) {
	e.connsMu.RLock()
	defer e.connsMu.RUnlock()

	conn, ok := e.conns[nodeID]
	if ok && !conn.IsClosed() {
		return conn, true
	}
	return nil, false
}

// ConnectionCount 返回连接数
func (e *Endpoint) ConnectionCount() int {
	e.connsMu.RLock()
	defer e.connsMu.RUnlock()
	return len(e.conns)
}

// ==================== 传输注册表 ====================

// TransportRegistry 返回传输注册表
func (e *Endpoint) TransportRegistry() *TransportRegistry {
	return e.transportRegistry
}

// AddTransport 添加传输到注册表
//
// 用于注册额外的传输（如 RelayTransport）
func (e *Endpoint) AddTransport(t transportif.Transport) error {
	if e.transportRegistry == nil {
		return fmt.Errorf("transport registry not initialized")
	}
	return e.transportRegistry.AddTransport(t)
}

// ==================== 监听与接受 ====================

// Listen 开始监听
func (e *Endpoint) Listen(ctx context.Context) error {
	e.mu.Lock()
	if e.started {
		e.mu.Unlock()
		return coreif.ErrAlreadyStarted
	}
	if e.closed {
		e.mu.Unlock()
		return coreif.ErrAlreadyClosed
	}
	e.started = true
	// REQ-OPS-001: 记录启动时间用于诊断报告
	if e.config != nil {
		e.config.StartedAt = time.Now()
	}
	e.mu.Unlock()

	log.Info("开始监听", "addrs", e.config.ListenAddrs)

	// 解析并监听每个地址
	for _, addrStr := range e.config.ListenAddrs {
		addr, err := parseAddress(addrStr)
		if err != nil {
			log.Warn("解析监听地址失败", "addr", addrStr, "err", err)
			continue
		}

		// 选择合适的传输层（与 Dial 路径对齐）
		// 关键：对于 /p2p-circuit 地址，必须选择 RelayTransport 而不是默认 QUIC
		selectedTransport := e.transport
		if e.transportRegistry != nil {
			if t := e.transportRegistry.TransportForListening(addr); t != nil {
				selectedTransport = t
			}
		}
		if selectedTransport == nil {
			log.Warn("无合适传输监听地址", "addr", addrStr)
			continue
		}

		listener, err := selectedTransport.Listen(addr)
		if err != nil {
			log.Warn("监听地址失败", "addr", addrStr, "err", err)
			continue
		}

		e.listenersMu.Lock()
		e.listeners = append(e.listeners, listener)
		e.listenersMu.Unlock()

		// 保存监听地址
		e.addrsMu.Lock()
		e.listenAddrs = append(e.listenAddrs, listener.Addr())
		e.addrsMu.Unlock()

		log.Info("监听器已启动", "addr", listener.Addr().String())

		// 启动接受循环
		go e.acceptLoop(ctx, listener)
	}

	// 启动外部地址发现（异步，不阻塞启动）
	// 注意：使用独立的 context，不依赖传入的 ctx（因为 fx ctx 在启动完成后会被取消）
	if e.nat != nil {
		discoverCtx := context.Background()
		go e.discoverExternalAddresses(discoverCtx)
	}

	// 关键：监听启动后立刻刷新一次发现层的本地地址
	// 这样 mDNS/DHT 等实现了 AddressUpdater 的发现器会收到 UpdateLocalAddrs，
	// 从而在 TXT 记录中携带真实可拨号地址（而不是默认的 Port=0）。
	e.notifyAddressChanged()

	return nil
}

// discoverExternalAddresses 发现外部地址并添加到通告地址
func (e *Endpoint) discoverExternalAddresses(ctx context.Context) {
	// 候选地址来源（INV-005 兼容）：
	// 1. 用户显式配置的 ExternalAddrs（ConfiguredAdvertised）
	// 2. 本机公网接口 IP + 监听端口（LocalInterfacePublic）
	// 3. STUN 公网 IP + 监听端口（STUNPublicIP）
	// 4. 端口映射成功获取的外部地址（PortMapping）
	// 所有候选地址都需要经过 dial-back 验证后才能成为 VerifiedDirect
	log.Debug("开始发现候选地址（需 dial-back 验证后才发布）...")

	// 复制监听地址（避免持锁过久）
	e.addrsMu.Lock()
	listenAddrs := make([]coreif.Address, len(e.listenAddrs))
	copy(listenAddrs, e.listenAddrs)
	e.addrsMu.Unlock()

	// 提取监听端口（区分 IPv4/IPv6）
	ipv4Ports := make(map[int]struct{})
	allPorts := make(map[int]struct{})
	for _, la := range listenAddrs {
		port := 0
		isIPv4Listener := false

		// 尝试从 address.Addr 获取端口和网络类型
		if addr, ok := la.(*address.Addr); ok {
			ma := addr.MA()
			port = ma.Port()
			ip := ma.IP()
			isIPv4Listener = ip != nil && ip.To4() != nil
		} else {
			// 退化解析：host:port 或 multiaddr 字符串
			addrStr := la.String()
			if strings.HasPrefix(addrStr, "/") {
				// multiaddr 格式
				ma := types.Multiaddr(addrStr)
				port = ma.Port()
				ip := ma.IP()
				isIPv4Listener = ip != nil && ip.To4() != nil
			} else {
				// host:port 格式
				host, portStr, err := net.SplitHostPort(addrStr)
				if err == nil {
					p, _ := strconv.Atoi(portStr)
					port = p
					ip := net.ParseIP(host)
					isIPv4Listener = ip != nil && ip.To4() != nil
				}
			}
		}

		if port <= 0 {
			continue
		}
		allPorts[port] = struct{}{}
		if isIPv4Listener {
			ipv4Ports[port] = struct{}{}
		}
	}

	if len(allPorts) == 0 {
		log.Debug("未找到监听端口，跳过候选地址发现")
		return
	}

	// 获取 ReachabilityCoordinator（用于上报候选地址）
	e.reachabilityMu.RLock()
	rc := e.reachabilityCoordinator
	e.reachabilityMu.RUnlock()

	// 辅助函数：上报候选地址
	reportCandidate := func(addr coreif.Address, source string) {
		if rc != nil {
			rc.OnDirectAddressCandidate(addr, source, addressif.PriorityUnverified)
		} else {
			// 兼容：未注入 Coordinator 时，保持旧行为（直接发布）
			e.AddAdvertisedAddr(addr)
		}
	}

	candidateCount := 0

	// ========== 来源 1: 用户显式配置的 ExternalAddrs ==========
	if e.config != nil && len(e.config.ExternalAddrs) > 0 {
		for _, extAddrStr := range e.config.ExternalAddrs {
			addr, err := e.parseConfiguredAddress(extAddrStr)
			if err != nil {
				log.Warn("解析配置的公网地址失败", "addr", extAddrStr, "err", err)
				continue
			}
			reportCandidate(addr, "configured-external")
			log.Info("上报候选地址（用户配置）", "addr", addr.String())
			candidateCount++
		}
	}

	// ========== 来源 2: 本机公网接口 IP + 监听端口 ==========
	publicIPs := e.discoverPublicInterfaceIPs()
	for _, ip := range publicIPs {
		// 为每个公网 IP 和每个监听端口组合生成候选地址
		for port := range allPorts {
			network := "ip4"
			if ip.To4() == nil {
				network = "ip6"
			}
			addrStr := fmt.Sprintf("/%s/%s/udp/%d/quic-v1", network, ip.String(), port)
			addr := address.NewAddr(types.Multiaddr(addrStr))
			reportCandidate(addr, "local-interface-public")
			log.Info("上报候选地址（本机公网接口）", "addr", addr.String())
			candidateCount++
		}
	}

	// ========== 来源 3: STUN 公网 IP + 监听端口 ==========
	// 适用于云服务器（ECS/AWS 等）：公网 IP 不绑在网卡上，但 STUN 能探测到公网出口 IP
	// 注意：STUN 返回的端口是随机 socket 端口，不是监听端口，需要用监听端口替换
	if e.nat != nil && len(allPorts) > 0 {
		e.discoverSTUNPublicIPAddresses(ctx, allPorts, reportCandidate, &candidateCount)
	}

	// ========== 来源 4: 端口映射（NAT 场景） ==========
	// 端口映射可能较慢（整体 15s 超时），因此放在 STUN 之后，避免阻塞云服务器候选的及时产出。
	if e.nat != nil && len(ipv4Ports) > 0 {
		e.discoverPortMappedAddresses(ctx, ipv4Ports, reportCandidate, &candidateCount)
	}

	if candidateCount == 0 {
		log.Debug("未发现任何候选地址（可能需要配置 ExternalAddrs 或检查网络环境）")
	} else {
		log.Info("候选地址发现完成", "count", candidateCount)
	}
}

// parseConfiguredAddress 解析用户配置的公网地址
func (e *Endpoint) parseConfiguredAddress(addrStr string) (coreif.Address, error) {
	// 支持 multiaddr 格式和简化格式
	if !strings.HasPrefix(addrStr, "/") {
		// 简化格式: "1.2.3.4:4001" -> "/ip4/1.2.3.4/udp/4001/quic-v1"
		host, portStr, err := net.SplitHostPort(addrStr)
		if err != nil {
			return nil, fmt.Errorf("无效的地址格式: %w", err)
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("无效的端口: %w", err)
		}
		ip := net.ParseIP(host)
		if ip == nil {
			return nil, fmt.Errorf("无效的 IP 地址: %s", host)
		}
		network := "ip4"
		if ip.To4() == nil {
			network = "ip6"
		}
		addrStr := fmt.Sprintf("/%s/%s/udp/%d/quic-v1", network, ip.String(), port)
		return address.NewAddr(types.Multiaddr(addrStr)), nil
	}

	// multiaddr 格式
	return parseMultiaddr(addrStr)
}

// discoverPublicInterfaceIPs 发现本机公网接口 IP
func (e *Endpoint) discoverPublicInterfaceIPs() []net.IP {
	var publicIPs []net.IP

	ifaces, err := net.Interfaces()
	if err != nil {
		log.Debug("获取网络接口失败", "err", err)
		return publicIPs
	}

	for _, iface := range ifaces {
		// 跳过回环和未启用的接口
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil {
				continue
			}

			// 检查是否是公网 IP
			if isPublicIP(ip) {
				publicIPs = append(publicIPs, ip)
				log.Debug("发现本机公网 IP", "ip", ip.String(), "iface", iface.Name)
			}
		}
	}

	return publicIPs
}

// isPublicIP 判断 IP 是否是公网地址
func isPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	// 仅允许全局单播（排除多播/未指定等）
	if !ip.IsGlobalUnicast() {
		return false
	}

	// 排除回环地址
	if ip.IsLoopback() {
		return false
	}

	// 排除链路本地地址
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}

	// 排除私有地址
	if ip4 := ip.To4(); ip4 != nil {
		// 0.0.0.0/8
		if ip4[0] == 0 {
			return false
		}
		// 127.0.0.0/8 (loopback)
		if ip4[0] == 127 {
			return false
		}
		// 10.0.0.0/8
		if ip4[0] == 10 {
			return false
		}
		// 172.16.0.0/12
		if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
			return false
		}
		// 192.168.0.0/16
		if ip4[0] == 192 && ip4[1] == 168 {
			return false
		}
		// 100.64.0.0/10 (CGN)
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return false
		}
		// 169.254.0.0/16 (link-local)
		if ip4[0] == 169 && ip4[1] == 254 {
			return false
		}
		// 198.18.0.0/15 (benchmarking, RFC 2544)
		if ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19) {
			return false
		}
		// 192.0.2.0/24, 198.51.100.0/24, 203.0.113.0/24 (TEST-NET-1/2/3)
		if ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 2 {
			return false
		}
		if ip4[0] == 198 && ip4[1] == 51 && ip4[2] == 100 {
			return false
		}
		if ip4[0] == 203 && ip4[1] == 0 && ip4[2] == 113 {
			return false
		}
		// 224.0.0.0/4 (multicast) / 240.0.0.0/4 (reserved)
		if ip4[0] >= 224 {
			return false
		}
		return true
	}

	// IPv6 排除
	if ip6 := ip.To16(); ip6 != nil {
		// 排除 ULA (fc00::/7)
		if ip6[0] == 0xfc || ip6[0] == 0xfd {
			return false
		}
		// 排除 link-local (fe80::/10)
		if ip6[0] == 0xfe && (ip6[1]&0xc0) == 0x80 {
			return false
		}
		return true
	}

	return false
}

// discoverPortMappedAddresses 通过端口映射发现外部地址
func (e *Endpoint) discoverPortMappedAddresses(ctx context.Context, ipv4Ports map[int]struct{}, reportCandidate func(coreif.Address, string), candidateCount *int) {
	// 超时控制
	overallCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// 本轮创建的映射（用于失败回滚）
	mappedPorts := make(map[int]int)
	shouldCleanup := true
	defer func() {
		if !shouldCleanup {
			return
		}
		seen := make(map[int]struct{})
		for _, extPort := range mappedPorts {
			if extPort <= 0 {
				continue
			}
			if _, ok := seen[extPort]; ok {
				continue
			}
			seen[extPort] = struct{}{}
			_ = e.nat.UnmapPort("udp", extPort)
		}
	}()

	// 尝试端口映射
	const discoveryLease = 90 * time.Second
	for internalPort := range ipv4Ports {
		select {
		case <-overallCtx.Done():
			log.Debug("端口映射发现被取消/超时", "err", overallCtx.Err())
			return
		default:
		}

		if mapErr := e.nat.MapPort("udp", internalPort, 0, discoveryLease); mapErr != nil {
			log.Debug("端口映射失败", "internalPort", internalPort, "err", mapErr)
			continue
		}

		extPort, mpErr := e.nat.GetMappedPort("udp", internalPort)
		if mpErr != nil || extPort <= 0 {
			log.Debug("获取映射端口失败", "internalPort", internalPort, "err", mpErr)
			continue
		}

		mappedPorts[internalPort] = extPort
	}

	if len(mappedPorts) == 0 {
		log.Debug("无可用端口映射")
		return
	}

	// 从端口映射器获取外部 IP
	extAddr, extErr := e.nat.GetExternalAddressFromPortMapperWithContext(overallCtx)
	if extErr != nil {
		log.Debug("端口映射器获取外部地址失败", "err", extErr)
		return
	}
	if extAddr == nil {
		log.Debug("端口映射器外部地址为空")
		return
	}

	externalIP := extAddr.String()
	if host, _, splitErr := net.SplitHostPort(externalIP); splitErr == nil {
		externalIP = host
	}

	ip := net.ParseIP(externalIP)
	if ip == nil || ip.To4() == nil {
		log.Debug("外部地址不是 IPv4，跳过", "externalIP", externalIP)
		return
	}

	// 续租映射
	const stableLease = 30 * time.Minute
	for internalPort := range mappedPorts {
		_ = e.nat.MapPort("udp", internalPort, 0, stableLease)
		if extPort, mpErr := e.nat.GetMappedPort("udp", internalPort); mpErr == nil && extPort > 0 {
			mappedPorts[internalPort] = extPort
		}
	}

	// 上报候选地址
	for internalPort, externalPort := range mappedPorts {
		addrStr := fmt.Sprintf("/ip4/%s/udp/%d/quic-v1", externalIP, externalPort)
		publicAddr := address.NewAddr(types.Multiaddr(addrStr))
		reportCandidate(publicAddr, "port-mapping")
		log.Info("上报候选地址（端口映射）",
			"addr", publicAddr.String(),
			"internalPort", internalPort,
			"externalPort", externalPort)
		*candidateCount++
	}

	shouldCleanup = false
}

// discoverSTUNPublicIPAddresses 通过 STUN 发现公网 IP 并与监听端口组合生成候选地址
//
// 适用于云服务器场景（ECS/AWS/GCP 等）：
// - 公网 IP 不绑定在本机网卡上（1:1 NAT）
// - UPnP/NAT-PMP 不可用
// - 但 STUN 能探测到公网出口 IP
//
// 注意：STUN 返回的端口是探测时使用的随机 socket 端口，不是监听端口
// 因此只使用 STUN 返回的 IP，端口用本机监听端口替换
func (e *Endpoint) discoverSTUNPublicIPAddresses(
	ctx context.Context,
	listenPorts map[int]struct{},
	reportCandidate func(coreif.Address, string),
	candidateCount *int,
) {
	// 超时控制（STUN 探测可能需要时间）
	stunCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// 从 NAT 服务获取 STUN 发现的外部地址
	extAddr, err := e.nat.GetExternalAddressWithContext(stunCtx)
	if err != nil {
		log.Debug("STUN 获取外部地址失败", "err", err)
		return
	}
	if extAddr == nil {
		log.Debug("STUN 外部地址为空")
		return
	}

	// 提取 IP（忽略 STUN 返回的端口）
	addrStr := extAddr.String()
	externalIP := addrStr

	// 尝试解析为 host:port 格式
	if host, _, splitErr := net.SplitHostPort(addrStr); splitErr == nil {
		externalIP = host
	}

	ip := net.ParseIP(externalIP)
	if ip == nil {
		log.Debug("STUN 返回的地址无法解析为 IP", "addr", addrStr)
		return
	}

	// 确定网络类型
	network := "ip4"
	if ip.To4() == nil {
		network = "ip6"
	}

	// 检查是否是公网 IP
	if !isPublicIP(ip) {
		log.Debug("STUN 返回的不是公网 IP，跳过", "ip", externalIP)
		return
	}

	// 用监听端口替换 STUN 端口，生成候选地址
	for listenPort := range listenPorts {
		addrStr := fmt.Sprintf("/%s/%s/udp/%d/quic-v1", network, ip.String(), listenPort)
		addr := address.NewAddr(types.Multiaddr(addrStr))
		reportCandidate(addr, "stun-public-ip")
		log.Info("上报候选地址（STUN 公网 IP）",
			"addr", addr.String(),
			"stunIP", externalIP,
			"listenPort", listenPort)
		*candidateCount++
	}
}

// acceptLoop 接受连接循环
// 注意：不使用传入的 ctx 控制退出，因为 Fx OnStart 的 ctx 会在启动完成后取消
// 使用 e.closeCh 和 listener 自身的关闭来控制退出
func (e *Endpoint) acceptLoop(_ context.Context, listener transportif.Listener) {
	log.Debug("开始监听", "addr", listener.Addr().String())
	for {
		select {
		case <-e.closeCh:
			return
		default:
		}

		rawConn, err := listener.Accept()
		if err != nil {
			if e.isClosed() {
				return
			}
			log.Debug("接受连接失败", "err", err)
			continue
		}

		// 使用独立的 context 处理入站连接（继承取消信号但不受 OnStart ctx 影响）
		go e.handleInbound(context.Background(), rawConn)
	}
}

// handleInbound 处理入站连接
func (e *Endpoint) handleInbound(ctx context.Context, rawConn transportif.Conn) {
	log.Debug("收到入站连接", "remoteAddr", rawConn.RemoteAddr().String())

	// 速率限制检查（防止 DoS 攻击）
	if e.rateLimiter != nil && !e.rateLimiter.Allow() {
		log.Warn("入站连接被速率限制拒绝",
			"remoteAddr", rawConn.RemoteAddr().String())
		_ = rawConn.Close()
		return
	}

	// 使用 ConnGater 检查是否允许来自该地址的入站连接
	if e.connGater != nil && !e.connGater.InterceptAccept(rawConn.RemoteAddr().String()) {
		log.Debug("入站连接被门控拦截（地址）",
			"remoteAddr", rawConn.RemoteAddr().String())
		_ = rawConn.Close()
		return
	}

	// 1. 检查是否已是安全连接（如 QUIC 内置 TLS）或需要安全升级
	var secureConn securityif.SecureConn
	var err error

	if sc, ok := rawConn.(securityif.SecureConn); ok {
		// 连接已经是安全的（如 QUIC），直接使用
		log.Debug("入站连接已安全（如 QUIC），跳过额外安全握手",
			"transport", rawConn.Transport())
		secureConn = sc
	} else if e.security != nil {
		// 普通连接（如 TCP），需要安全握手
		secureConn, err = e.security.SecureInbound(ctx, rawConn)
		if err != nil {
			log.Debug("入站安全握手失败", "err", err)
			_ = rawConn.Close()
			return
		}
	} else {
		// 无安全层时，无法验证对端身份
		// 记录警告并拒绝连接（生产环境应始终使用安全层）
		log.Warn("入站连接无安全层，无法验证对端身份",
			"remoteAddr", rawConn.RemoteAddr().String())
		_ = rawConn.Close()
		return
	}

	// 2. 获取远程身份
	remoteID := secureConn.RemoteIdentity()

	// 验证远程身份不为空
	if remoteID == (types.NodeID{}) {
		log.Warn("入站连接远程身份为空",
			"remoteAddr", rawConn.RemoteAddr().String())
		_ = secureConn.Close()
		return
	}

	// 使用 ConnGater 检查是否允许来自该节点的入站连接（身份已验证）
	if e.connGater != nil && !e.connGater.InterceptSecured(types.DirInbound, remoteID) {
		log.Debug("入站连接被门控拦截（节点）",
			"remoteID", remoteID.ShortString())
		_ = secureConn.Close()
		return
	}

	// 使用 ConnManager 检查是否允许新连接
	if e.connManager != nil && !e.connManager.AllowConnection(remoteID, types.DirInbound) {
		log.Debug("入站连接被管理器拒绝（连接数限制）",
			"remoteID", remoteID.ShortString())
		_ = secureConn.Close()
		return
	}

	// 检查连接限制（endpoint 本身的限制，与 ConnManager 独立）
	if e.config.MaxConnections > 0 && e.ConnectionCount() >= e.config.MaxConnections {
		log.Warn("连接数已达上限", "max", e.config.MaxConnections)
		_ = secureConn.Close()
		return
	}

	// 3. 创建多路复用器
	var muxer muxerif.Muxer
	if e.muxerFactory != nil {
		// 对于 QUIC 连接，muxerFactory 会使用 QUIC 内置的多路复用
		muxer, err = e.muxerFactory.NewMuxer(secureConn, true) // 服务端
		if err != nil {
			log.Debug("创建多路复用器失败", "err", err)
			_ = secureConn.Close()
			return
		}
	}

	// 4. 创建连接包装
	conn := NewConnection(
		secureConn,
		muxer,
		types.DirInbound,
		e,
	)

	// 5. 注册连接
	e.addConnection(remoteID, conn)

	log.Info("入站连接建立",
		"remoteID", remoteID.ShortString(),
		"remoteAddr", rawConn.RemoteAddr().String())

	// 6. 启动流接受循环
	conn.StartStreamAcceptLoop(ctx)

	// 7. 放入接受队列（用于 Accept() 方法）
	// 注意：即使队列满，连接仍然有效并已注册，只是 Accept() 调用者无法获取此通知
	select {
	case e.acceptCh <- conn:
	default:
		log.Warn("接受队列已满，连接仍然有效但无法通过 Accept() 获取",
			"remoteID", remoteID.ShortString(),
			"queueSize", len(e.acceptCh))
	}
}

// Accept 接受连接
func (e *Endpoint) Accept(ctx context.Context) (coreif.Connection, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-e.closeCh:
		return nil, coreif.ErrAlreadyClosed
	case conn := <-e.acceptCh:
		return conn, nil
	}
}

// ==================== 协议处理 ====================

// SetProtocolHandler 注册协议处理器
func (e *Endpoint) SetProtocolHandler(protocolID coreif.ProtocolID, handler coreif.ProtocolHandler) {
	// 优先使用注入的 protocolRouter
	if e.protocolRouter != nil {
		e.protocolRouter.AddHandler(protocolID, handler)
		log.Debug("注册协议处理器（通过 ProtocolRouter）", "protocolID", string(protocolID))
		return
	}

	// Fallback 到内部的 handlers map
	e.handlersMu.Lock()
	defer e.handlersMu.Unlock()
	e.handlers[protocolID] = handler

	log.Debug("注册协议处理器（内部）", "protocolID", string(protocolID))
}

// RemoveProtocolHandler 移除协议处理器
func (e *Endpoint) RemoveProtocolHandler(protocolID coreif.ProtocolID) {
	// 优先使用注入的 protocolRouter
	if e.protocolRouter != nil {
		e.protocolRouter.RemoveHandler(protocolID)
		return
	}

	// Fallback 到内部的 handlers map
	e.handlersMu.Lock()
	defer e.handlersMu.Unlock()
	delete(e.handlers, protocolID)
}

// SetStreamHandler 设置流处理器 (兼容接口)
func (e *Endpoint) SetStreamHandler(protocolID coreif.ProtocolID, handler coreif.ProtocolHandler) {
	e.SetProtocolHandler(protocolID, handler)
}

// RemoveStreamHandler 移除流处理器 (兼容接口)
func (e *Endpoint) RemoveStreamHandler(protocolID coreif.ProtocolID) {
	e.RemoveProtocolHandler(protocolID)
}

// RegisterConnectionCallback 注册连接建立回调
//
// 当新连接建立时，回调会被调用。
// 这用于通知 GossipSub 等模块新 peer 的连接。
//
// 参数:
//   - callback: 回调函数，接收 nodeID 和 outbound 标志
//
// 示例:
//
//	endpoint.RegisterConnectionCallback(func(nodeID NodeID, outbound bool) {
//	    gossipRouter.AddPeer(nodeID, outbound)
//	})
func (e *Endpoint) RegisterConnectionCallback(callback func(nodeID coreif.NodeID, outbound bool)) {
	e.connCallbacksMu.Lock()
	defer e.connCallbacksMu.Unlock()
	e.connCallbacks = append(e.connCallbacks, callback)
}

// RegisterConnectionEventCallback 注册连接事件回调
//
// 当连接状态变化时（建立、关闭、失败），回调会被调用。
// 事件类型包括：ConnectionOpenedEvent、ConnectionClosedEvent、ConnectionFailedEvent
//
// 参数:
//   - callback: 回调函数，接收事件对象
//
// 示例:
//
//	endpoint.RegisterConnectionEventCallback(func(event interface{}) {
//	    switch e := event.(type) {
//	    case coreif.ConnectionClosedEvent:
//	        log.Info("连接关闭", "node", e.Connection.RemoteID())
//	        if e.IsRelayConn {
//	            // 处理 relay 连接断开
//	        }
//	    }
//	})
func (e *Endpoint) RegisterConnectionEventCallback(callback func(event interface{})) {
	e.eventCallbacksMu.Lock()
	defer e.eventCallbacksMu.Unlock()
	e.eventCallbacks = append(e.eventCallbacks, callback)
}

// dispatchEvent 分发连接事件
//
// 异步调用所有注册的事件回调，避免阻塞调用方
func (e *Endpoint) dispatchEvent(event interface{}) {
	e.eventCallbacksMu.RLock()
	callbacks := make([]func(interface{}), len(e.eventCallbacks))
	copy(callbacks, e.eventCallbacks)
	e.eventCallbacksMu.RUnlock()

	for _, cb := range callbacks {
		go cb(event) // 异步调用，避免阻塞
	}
}

// ConnectWithRetry 带自动重试的连接方法
//
// 当连接失败时，使用指数退避策略自动重试。
// 适用于需要高可靠性连接的场景。
//
// 参数:
//   - ctx: 上下文，用于取消重试
//   - nodeID: 目标节点 ID
//   - config: 重试配置，nil 时使用默认配置
//
// 返回:
//   - Connection: 成功建立的连接
//   - error: 所有重试都失败时返回错误
func (e *Endpoint) ConnectWithRetry(ctx context.Context, nodeID coreif.NodeID, config *coreif.RetryConfig) (coreif.Connection, error) {
	if config == nil {
		config = coreif.DefaultRetryConfig()
	}

	var lastErr error
	backoff := config.InitialBackoff

	for attempt := 0; config.MaxRetries == 0 || attempt < config.MaxRetries; attempt++ {
		// 检查 context 是否已取消
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		conn, err := e.Connect(ctx, nodeID)
		if err == nil {
			if attempt > 0 {
				log.Debug("连接重试成功",
					"nodeID", nodeID.ShortString(),
					"attempt", attempt+1)
			}
			return conn, nil
		}

		lastErr = err
		log.Debug("连接失败，准备重试",
			"nodeID", nodeID.ShortString(),
			"attempt", attempt+1,
			"backoff", backoff,
			"err", err)

		// 等待退避时间
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(backoff):
		}

		// 指数退避
		backoff *= 2
		if backoff > config.MaxBackoff {
			backoff = config.MaxBackoff
		}
	}

	return nil, fmt.Errorf("连接重试次数已达上限 (%d): %w", config.MaxRetries, lastErr)
}

// Protocols 返回协议列表
func (e *Endpoint) Protocols() []coreif.ProtocolID {
	// 优先使用注入的 protocolRouter
	if e.protocolRouter != nil {
		return e.protocolRouter.Protocols()
	}

	// Fallback 到内部的 handlers map
	e.handlersMu.RLock()
	defer e.handlersMu.RUnlock()

	protocols := make([]coreif.ProtocolID, 0, len(e.handlers))
	for p := range e.handlers {
		protocols = append(protocols, p)
	}
	return protocols
}

// getHandler 获取协议处理器
func (e *Endpoint) getHandler(protocolID coreif.ProtocolID) (coreif.ProtocolHandler, bool) {
	// 优先使用注入的 protocolRouter
	// 注意：protocolRouter 不直接返回 handler，而是通过 Handle 方法路由
	// 这里返回一个代理 handler
	if e.protocolRouter != nil && e.protocolRouter.HasProtocol(protocolID) {
		return func(stream coreif.Stream) {
			if err := e.protocolRouter.Handle(stream); err != nil {
				log.Debug("协议路由处理失败", "err", err)
			}
		}, true
	}

	// Fallback 到内部的 handlers map
	e.handlersMu.RLock()
	defer e.handlersMu.RUnlock()
	handler, ok := e.handlers[protocolID]
	return handler, ok
}

// ==================== 地址管理 ====================

// ListenAddrs 返回监听地址
func (e *Endpoint) ListenAddrs() []coreif.Address {
	e.addrsMu.RLock()
	defer e.addrsMu.RUnlock()

	addrs := make([]coreif.Address, len(e.listenAddrs))
	copy(addrs, e.listenAddrs)
	return addrs
}

// AdvertisedAddrs 返回通告地址
//
// 可达性优先策略：统一聚合来自不同来源的地址
// 优先级：已验证直连地址 > Relay 地址 > 监听地址
func (e *Endpoint) AdvertisedAddrs() []coreif.Address {
	// 统一真源：如果已注入可达性协调器，则以其输出为准
	e.reachabilityMu.RLock()
	rc := e.reachabilityCoordinator
	e.reachabilityMu.RUnlock()
	if rc != nil {
		out := rc.AdvertisedAddrs()
		// #region agent log
		debuglog.Log(
			"pre-fix",
			"H1",
			"internal/core/endpoint/endpoint_impl.go:AdvertisedAddrs",
			"reachabilityCoordinator_present",
			map[string]any{"outLen": len(out)},
		)
		// #endregion agent log
		return out
	}

	var result []coreif.Address

	// 1. 已验证直连地址（优先级最高）
	e.addrsMu.RLock()
	if len(e.advertisedAddrs) > 0 {
		result = append(result, e.advertisedAddrs...)
	}
	e.addrsMu.RUnlock()

	// 2. Relay 地址（由 AutoRelay / ReachabilityCoordinator 管理）
	// 注意：Endpoint 不再直接持有 RelayClient，Relay 地址通过 ReachabilityCoordinator 获取

	// 3. 如果没有任何地址，回退到监听地址
	if len(result) == 0 {
		e.addrsMu.RLock()
		// 仅作为最后回退：过滤掉 0.0.0.0/:: 这类不可拨号地址，避免对外传播污染。
		for _, a := range e.listenAddrs {
			if a == nil || isUnspecifiedAddrString(a.String()) {
				continue
			}
			result = append(result, a)
		}
		e.addrsMu.RUnlock()
	}

	// #region agent log
	debuglog.Log(
		"pre-fix",
		"H2",
		"internal/core/endpoint/endpoint_impl.go:AdvertisedAddrs",
		"reachabilityCoordinator_absent",
		map[string]any{"resultLen": len(result)},
	)
	// #endregion agent log

	return result
}

// VerifiedDirectAddrs 返回已验证的直连地址列表（REQ-ADDR-002 真源）
//
// 这是 ShareableAddrs 的真实数据源：
// - 仅返回通过 dial-back 验证的公网直连地址
// - 不包含 Relay 地址
// - 不包含 ListenAddrs 回退
// - 无验证地址时返回 nil（而非空切片）
//
// 用于构建可分享的 Full Address（INV-005）。
func (e *Endpoint) VerifiedDirectAddrs() []coreif.Address {
	// 统一真源：如果已注入可达性协调器，则从协调器获取
	e.reachabilityMu.RLock()
	rc := e.reachabilityCoordinator
	e.reachabilityMu.RUnlock()

	if rc != nil {
		return rc.VerifiedDirectAddresses()
	}

	// 无协调器时回退到 advertisedAddrs（但过滤 Relay 地址）
	e.addrsMu.RLock()
	defer e.addrsMu.RUnlock()

	if len(e.advertisedAddrs) == 0 {
		return nil
	}

	// 过滤掉 Relay 地址，只返回直连地址
	result := make([]coreif.Address, 0, len(e.advertisedAddrs))
	for _, addr := range e.advertisedAddrs {
		if addr == nil {
			continue
		}
		// 检查是否是 Relay 地址（包含 /p2p-circuit/）
		addrStr := addr.String()
		if strings.Contains(addrStr, "/p2p-circuit/") {
			continue
		}
		result = append(result, addr)
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func isUnspecifiedAddrString(s string) bool {
	// multiaddr: /ip4/0.0.0.0/... 或 /ip6/::/...
	if strings.Contains(s, "/ip4/0.0.0.0/") || strings.Contains(s, "/ip6/::/") {
		return true
	}
	host, _, err := net.SplitHostPort(s)
	if err != nil {
		return false
	}
	return host == "0.0.0.0" || host == "::" || host == "0:0:0:0:0:0:0:0"
}

// SetReachabilityCoordinator 设置可达性协调器
//
// 可达性优先策略：Endpoint 的通告地址将以协调器输出为准，并由协调器驱动发现层刷新。
func (e *Endpoint) SetReachabilityCoordinator(c reachabilityif.Coordinator) {
	e.reachabilityMu.Lock()
	e.reachabilityCoordinator = c
	e.reachabilityMu.Unlock()

	// 当协调器地址变更时，刷新发现层的通告地址
	if e.discovery != nil && c != nil {
		c.SetOnAddressChanged(func(addrs []coreif.Address) {
			go e.discovery.RefreshAnnounce(addrs)
		})
	}
}

// AddAdvertisedAddr 添加通告地址
func (e *Endpoint) AddAdvertisedAddr(addr coreif.Address) {
	e.addrsMu.Lock()
	// 可达性优先：对外通告地址应是“可拨号/可用”的地址集合。
	// 监听地址（尤其 0.0.0.0/::/私网监听）不应被隐式混入 AdvertisedAddrs，避免污染发现层与对端拨号。
	for _, existing := range e.advertisedAddrs {
		if existing != nil && existing.Equal(addr) {
			e.addrsMu.Unlock()
			return
		}
	}
	e.advertisedAddrs = append(e.advertisedAddrs, addr)
	e.addrsMu.Unlock()

	// 触发地址变化通知，让发现服务重新发布
	e.notifyAddressChanged()
}

// notifyAddressChanged 通知地址变化
// 当 AdvertisedAddrs 变化时调用，触发发现服务重新发布地址到网络
func (e *Endpoint) notifyAddressChanged() {
	if e.discovery != nil {
		// 获取当前通告地址
		addrs := e.AdvertisedAddrs()
		// 异步刷新，避免阻塞
		go e.discovery.RefreshAnnounce(addrs)
	}
}

// ==================== 子系统访问 ====================

// Discovery 返回发现服务
func (e *Endpoint) Discovery() coreif.DiscoveryService {
	return e.discovery
}

// NAT 返回 NAT 服务
// 返回适配后的 NATService 以满足 endpoint.Endpoint 接口
func (e *Endpoint) NAT() coreif.NATService {
	if e.nat == nil {
		return nil
	}
	return &natServiceAdapter{nat: e.nat}
}

// NATFull 返回完整的 NAT 服务（包含所有 natif.NATService 方法）
func (e *Endpoint) NATFull() natif.NATService {
	return e.nat
}

// natServiceAdapter 适配 natif.NATService 到 coreif.NATService
type natServiceAdapter struct {
	nat natif.NATService
}

func (a *natServiceAdapter) GetExternalAddress() (coreif.Address, error) {
	return a.nat.GetExternalAddress()
}

func (a *natServiceAdapter) NATType() coreif.NATType {
	return coreif.NATType(a.nat.NATType())
}

func (a *natServiceAdapter) MapPort(protocol string, internalPort, externalPort int) error {
	// 使用默认的映射时间（1小时）
	return a.nat.MapPort(protocol, internalPort, externalPort, time.Hour)
}

// Relay 返回中继客户端
//
// 注意：Endpoint 不再直接持有 RelayClient 引用。
// Relay 功能通过 TransportRegistry + RelayTransport 透明工作。
// 此方法保留以满足接口兼容，始终返回 nil。
func (e *Endpoint) Relay() coreif.RelayClient {
	return nil
}

// AddressBook 返回地址簿
func (e *Endpoint) AddressBook() coreif.AddressBook {
	return e.addressBook
}

// ==================== 生命周期 ====================

// Close 关闭 Endpoint
func (e *Endpoint) Close() error {
	e.mu.Lock()
	if e.closed {
		e.mu.Unlock()
		return nil
	}
	e.closed = true
	e.mu.Unlock()

	log.Info("关闭 Endpoint")

	e.closeOnce.Do(func() {
		close(e.closeCh)

		// 关闭所有监听器
		e.listenersMu.Lock()
		listeners := e.listeners
		e.listeners = nil
		e.listenersMu.Unlock()

		for _, l := range listeners {
			_ = l.Close()
		}

		// 关闭所有连接
		// 注意：先复制并清空 map，然后释放锁，再关闭连接
		// 这避免了死锁（conn.Close() 可能调用 removeConnection）
		e.connsMu.Lock()
		connsToClose := make([]*Connection, 0, len(e.conns))
		for _, conn := range e.conns {
			connsToClose = append(connsToClose, conn)
		}
		e.conns = make(map[coreif.NodeID]*Connection)
		e.connsMu.Unlock()

		for _, conn := range connsToClose {
			_ = conn.Close()
		}

		// 关闭传输层
		if e.transport != nil {
			_ = e.transport.Close()
		}
	})

	return nil
}

// isClosed 检查是否已关闭
func (e *Endpoint) isClosed() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.closed
}

// ==================== 内部方法 ====================

// addConnection 添加连接
func (e *Endpoint) addConnection(nodeID coreif.NodeID, conn *Connection) {
	var oldConn *Connection
	var shouldCloseNew bool

	e.connsMu.Lock()
	// 如果已有连接：
	// - 绝大多数场景（尤其是 reachability dial-back 产生的"探测连接"）不应踢掉现有业务连接，
	//   否则会导致应用层 stream 突然断开（muxer closed）。
	// - 这里采取保守策略：若已存在且未关闭，则保留旧连接并关闭新连接。
	//  （后续如需支持 relay->direct upgrade，可引入"连接质量/标签"再做替换决策）
	if existing, exists := e.conns[nodeID]; exists && existing != nil && !existing.IsClosed() {
		oldConn = existing
		shouldCloseNew = true
	} else {
	e.conns[nodeID] = conn
	}
	e.connsMu.Unlock()

	// 在锁外关闭连接，避免死锁
	if shouldCloseNew {
		_ = conn.Close()
		return
	}
	if oldConn != nil {
		_ = oldConn.Close()
	}

	// 通知连接管理器新连接建立
	if e.connManager != nil {
		e.connManager.NotifyConnected(conn)
	}

	// 通知发现服务新连接建立（用于 DHT 引导重试）
	// 获取连接的远端地址
	if e.discovery != nil {
		var addrs []string
		for _, addr := range conn.RemoteAddrs() {
			addrs = append(addrs, addr.String())
		}
		e.discovery.NotifyPeerConnected(nodeID, addrs)
	}

	// 调用连接回调（通知 GossipSub 等模块）
	e.connCallbacksMu.RLock()
	callbacks := make([]func(coreif.NodeID, bool), len(e.connCallbacks))
	copy(callbacks, e.connCallbacks)
	e.connCallbacksMu.RUnlock()

	outbound := conn.Direction() == coreif.DirOutbound
	for _, cb := range callbacks {
		cb(nodeID, outbound)
	}
}

// removeConnection 移除连接
func (e *Endpoint) removeConnection(nodeID coreif.NodeID) {
	e.removeConnectionWithReason(nodeID, nil, nil)
}

// removeConnectionWithReason 带原因移除连接并触发事件
func (e *Endpoint) removeConnectionWithReason(nodeID coreif.NodeID, conn coreif.Connection, reason error) {
	// 如果没有传入连接，尝试从 map 获取（用于事件通知）
	e.connsMu.Lock()
	if conn == nil {
		conn = e.conns[nodeID]
	}
	delete(e.conns, nodeID)
	e.connsMu.Unlock()

	// 通知连接管理器连接断开
	if e.connManager != nil {
		e.connManager.NotifyDisconnected(nodeID)
	}

	// 通知发现服务连接断开
	if e.discovery != nil {
		e.discovery.NotifyPeerDisconnected(nodeID)
	}

	// 分发连接关闭事件
	if conn != nil {
		isRelay, relayID := e.isRelayConnection(conn)
		e.dispatchEvent(coreif.ConnectionClosedEvent{
			Connection:  conn,
			Reason:      reason,
			IsRelayConn: isRelay,
			RelayID:     relayID,
		})
	}
}

// isRelayConnection 检测连接是否为 relay 中继连接
//
// 通过检查连接的远程地址是否包含 /p2p-circuit 来判断
func (e *Endpoint) isRelayConnection(conn coreif.Connection) (isRelay bool, relayID coreif.NodeID) {
	if conn == nil {
		return false, coreif.NodeID{}
	}

	// 安全获取地址列表，避免关闭期间的竞态条件
	addrs := conn.RemoteAddrs()
	if addrs == nil {
		return false, coreif.NodeID{}
	}

	for _, addr := range addrs {
		if addr == nil {
			continue
		}
		addrStr := addr.String()
		// 检查是否为 relay 地址（包含 /p2p-circuit）
		if strings.Contains(addrStr, "/p2p-circuit") {
			// 尝试提取 relay 节点 ID
			// 格式: /ip4/.../p2p/<relayID>/p2p-circuit/p2p/<targetID>
			relayIDStr := extractRelayIDFromAddr(addrStr)
			if relayIDStr != "" {
				relayID, _ = types.ParseNodeID(relayIDStr)
			}
			return true, relayID
		}
	}
	return false, coreif.NodeID{}
}

// extractRelayIDFromAddr 从 relay 地址中提取 relay 节点 ID
//
// 格式: /ip4/.../p2p/<relayID>/p2p-circuit/p2p/<targetID>
func extractRelayIDFromAddr(addr string) string {
	parts := strings.Split(addr, "/")
	for i := 0; i < len(parts)-2; i++ {
		if parts[i] == "p2p" && i+2 < len(parts) && parts[i+2] == "p2p-circuit" {
			return parts[i+1]
		}
	}
	return ""
}

// ============================================================================
//                              诊断报告 (REQ-OPS-001)
// ============================================================================

// DiagnosticReport 生成诊断报告
//
// REQ-OPS-001: 关键状态可观测且有统一诊断入口
func (e *Endpoint) DiagnosticReport() coreif.DiagnosticReport {
	report := coreif.DiagnosticReport{
		Timestamp: time.Now(),
	}

	// 节点信息
	report.Node = coreif.NodeDiagnostics{
		ID:      e.ID().String(),
		IDShort: e.ID().ShortString(),
	}
	if pk := e.PublicKey(); pk != nil {
		report.Node.PublicKeyType = types.KeyType(pk.Type()).String()
	}
	if e.config != nil && !e.config.StartedAt.IsZero() {
		report.Node.StartedAt = e.config.StartedAt
		report.Node.Uptime = time.Since(e.config.StartedAt)
	}

	// 连接信息
	e.connsMu.RLock()
	report.Connections.Total = len(e.conns)
	report.Connections.Peers = make([]string, 0, len(e.conns))
	for nodeID, conn := range e.conns {
		report.Connections.Peers = append(report.Connections.Peers, nodeID.ShortString())
		if conn.Direction() == types.DirInbound {
			report.Connections.Inbound++
		} else {
			report.Connections.Outbound++
		}
		// 统计连接路径类型
		if conn.IsRelayed() {
			report.Connections.PathStats.Relayed++
		} else {
			// TODO: 区分直连和打洞成功需要更多上下文
			// 目前简化为：非中继即直连
			report.Connections.PathStats.Direct++
		}
	}
	e.connsMu.RUnlock()

	// 地址信息
	listenAddrs := e.ListenAddrs()
	report.Addresses.ListenAddrs = make([]string, len(listenAddrs))
	for i, addr := range listenAddrs {
		report.Addresses.ListenAddrs[i] = addr.String()
	}

	advertisedAddrs := e.AdvertisedAddrs()
	report.Addresses.AdvertisedAddrs = make([]string, len(advertisedAddrs))
	for i, addr := range advertisedAddrs {
		report.Addresses.AdvertisedAddrs[i] = addr.String()
	}

	verifiedAddrs := e.VerifiedDirectAddrs()
	report.Addresses.VerifiedDirectAddrs = make([]string, len(verifiedAddrs))
	for i, addr := range verifiedAddrs {
		report.Addresses.VerifiedDirectAddrs[i] = addr.String()
	}

	// 发现服务信息
	if e.discovery != nil {
		// DiscoveryService 接口本身包含 State()，无需类型断言
		state := e.discovery.State()
			report.Discovery.State = state.String()
			report.Discovery.StateReady = state.IsReady()

		// 获取引导节点数量
		if bootstrap, ok := e.discovery.(interface {
			GetBootstrapPeers(context.Context) ([]coreif.PeerInfo, error)
		}); ok {
			ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
			if peers, err := bootstrap.GetBootstrapPeers(ctx); err == nil {
				report.Discovery.BootstrapPeers = len(peers)
			}
			cancel()
		}

		// 获取已知节点数量
		if knownPeers, ok := e.discovery.(interface{ KnownPeerCount() int }); ok {
			report.Discovery.KnownPeers = knownPeers.KnownPeerCount()
		}
	}

	// NAT 信息
	if e.nat != nil {
		report.NAT.Type = e.nat.NATType().String()
		if extAddr, err := e.nat.GetExternalAddress(); err == nil && extAddr != nil {
			report.NAT.ExternalAddr = extAddr.String()
		}
		// 检查是否支持端口映射
		report.NAT.PortMappingAvailable = e.nat.NATType() != coreif.NATTypeSymmetric
	}

	// 中继信息
	// 注意：Endpoint 不再持有 RelayClient 引用
	// 中继信息由 AutoRelay / ReachabilityCoordinator 管理
	// 此处仅检测 RelayTransport 是否注册
	if e.transportRegistry != nil {
		for _, t := range e.transportRegistry.Transports() {
			if t.CanDial(newSimpleAddr("/p2p-circuit")) {
				report.Relay.Enabled = true
				break
			}
		}
	}

	return report
}

// ============================================================================
//                              地址解析
// ============================================================================

// parseAddress 解析地址字符串
func parseAddress(addrStr string) (coreif.Address, error) {
	if addrStr == "" {
		return nil, errors.New("empty address")
	}

	// 检测地址类型
	if strings.HasPrefix(addrStr, "/") {
		// Multiaddr 格式
		return parseMultiaddr(addrStr)
	}

	// IP:Port 或 Domain:Port 格式
	return parseHostPort(addrStr)
}

// parseMultiaddr 解析 Multiaddr 格式地址
func parseMultiaddr(addrStr string) (coreif.Address, error) {
	if addrStr == "" {
		return nil, errors.New("empty address")
	}
	// 尝试用 address.Parse 解析
	addr, err := address.Parse(addrStr)
	if err != nil {
		// 如果解析失败，创建一个简单的包装
		return address.NewAddr(types.Multiaddr(addrStr)), nil
	}
	return addr, nil
}

// parseHostPort 解析 Host:Port 格式地址
func parseHostPort(addrStr string) (coreif.Address, error) {
	host, portStr, err := net.SplitHostPort(addrStr)
	if err != nil {
		// 可能没有端口，创建简单包装
		return address.NewAddr(types.Multiaddr(addrStr)), nil
	}

	port, _ := strconv.Atoi(portStr)

	// 将 host:port 转换为 multiaddr 格式
	ma, err := types.FromHostPort(host, port, "udp/quic-v1")
	if err != nil {
		// 如果转换失败，保留原始字符串
		return address.NewAddr(types.Multiaddr(addrStr)), nil
	}
	return address.NewAddr(ma), nil
}

// parsedAddress 已删除，统一使用 address.Addr
