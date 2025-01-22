package swarm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"slices"

	"github.com/dep2p/go-dep2p/core/connmgr"
	"github.com/dep2p/go-dep2p/core/event"
	"github.com/dep2p/go-dep2p/core/metrics"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/peerstore"
	"github.com/dep2p/go-dep2p/core/transport"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	madns "github.com/dep2p/go-dep2p/multiformats/multiaddr/dns"
	logging "github.com/dep2p/log"
)

// 默认拨号超时时间
const (
	// 全局拨号超时时间为 15 秒
	defaultDialTimeout = 15 * time.Second

	// 本地网络拨号超时时间为 5 秒，是允许拨号到本地网络地址所需的最大时间。
	// 包括建立原始网络连接、协议选择和握手(如果需要)的时间
	defaultDialTimeoutLocal = 5 * time.Second

	// 默认新建流的超时时间为 15 秒
	defaultNewStreamTimeout = 15 * time.Second
)

// 日志记录器
var log = logging.Logger("net-swarm")

// ErrSwarmClosed 在尝试操作已关闭的 swarm 时返回。
var ErrSwarmClosed = errors.New("swarm 已关闭")

// ErrAddrFiltered 在尝试注册到被过滤地址的连接时返回。
// 除非底层传输出现异常,否则不应该看到此错误
var ErrAddrFiltered = errors.New("地址被过滤")

// ErrDialTimeout 表示由于全局超时导致拨号失败
var ErrDialTimeout = errors.New("拨号超时")

// Option 定义 Swarm 的配置选项函数类型
type Option func(*Swarm) error

// WithConnectionGater 设置连接过滤器
// 参数:
//   - gater: connmgr.ConnectionGater 连接过滤器对象
//
// 返回值:
//   - Option 配置选项函数
func WithConnectionGater(gater connmgr.ConnectionGater) Option {
	return func(s *Swarm) error {
		s.gater = gater
		return nil
	}
}

// WithMultiaddrResolver 设置多地址解析器
// 参数:
//   - resolver: network.MultiaddrDNSResolver 多地址解析器对象
//
// 返回值:
//   - Option 配置选项函数
func WithMultiaddrResolver(resolver network.MultiaddrDNSResolver) Option {
	return func(s *Swarm) error {
		s.multiaddrResolver = resolver
		return nil
	}
}

// WithMetrics 设置指标报告器
// 参数:
//   - reporter: metrics.Reporter 指标报告器对象
//
// 返回值:
//   - Option 配置选项函数
func WithMetrics(reporter metrics.Reporter) Option {
	return func(s *Swarm) error {
		s.bwc = reporter
		return nil
	}
}

// WithMetricsTracer 设置指标追踪器
// 参数:
//   - t: MetricsTracer 指标追踪器对象
//
// 返回值:
//   - Option 配置选项函数
func WithMetricsTracer(t MetricsTracer) Option {
	return func(s *Swarm) error {
		s.metricsTracer = t
		return nil
	}
}

// WithDialTimeout 设置拨号超时时间
// 参数:
//   - t: time.Duration 超时时间
//
// 返回值:
//   - Option 配置选项函数
func WithDialTimeout(t time.Duration) Option {
	return func(s *Swarm) error {
		s.dialTimeout = t
		return nil
	}
}

// WithDialTimeoutLocal 设置本地拨号超时时间
// 参数:
//   - t: time.Duration 超时时间
//
// 返回值:
//   - Option 配置选项函数
func WithDialTimeoutLocal(t time.Duration) Option {
	return func(s *Swarm) error {
		s.dialTimeoutLocal = t
		return nil
	}
}

// WithResourceManager 设置资源管理器
// 参数:
//   - m: network.ResourceManager 资源管理器对象
//
// 返回值:
//   - Option 配置选项函数
func WithResourceManager(m network.ResourceManager) Option {
	return func(s *Swarm) error {
		s.rcmgr = m
		return nil
	}
}

// WithDialRanker 设置拨号排序器
// 参数:
//   - d: network.DialRanker 拨号排序器对象
//
// 返回值:
//   - Option 配置选项函数
//
// 注意:
//   - 拨号排序器不能为空
func WithDialRanker(d network.DialRanker) Option {
	return func(s *Swarm) error {
		if d == nil {
			return errors.New("swarm: 拨号排序器不能为空")
		}
		s.dialRanker = d
		return nil
	}
}

// WithUDPBlackHoleSuccessCounter 配置 UDP 黑洞检测
// 参数:
//   - f: *BlackHoleSuccessCounter 黑洞检测计数器
//
// 返回值:
//   - Option 配置选项函数
//
// 注意:
//   - n 是评估黑洞状态的滑动窗口大小
//   - min 是不阻塞请求所需的最小成功数
func WithUDPBlackHoleSuccessCounter(f *BlackHoleSuccessCounter) Option {
	return func(s *Swarm) error {
		s.udpBHF = f
		return nil
	}
}

// WithIPv6BlackHoleSuccessCounter 配置 IPv6 黑洞检测
// 参数:
//   - f: *BlackHoleSuccessCounter 黑洞检测计数器
//
// 返回值:
//   - Option 配置选项函数
//
// 注意:
//   - n 是评估黑洞状态的滑动窗口大小
//   - min 是不阻塞请求所需的最小成功数
func WithIPv6BlackHoleSuccessCounter(f *BlackHoleSuccessCounter) Option {
	return func(s *Swarm) error {
		s.ipv6BHF = f
		return nil
	}
}

// WithReadOnlyBlackHoleDetector 配置只读黑洞检测
// 返回值:
//   - Option 配置选项函数
//
// 注意:
//   - 只读模式下未知状态的拨号请求会被拒绝
//   - 不会更新检测器状态
//   - 适用于需要准确提供可达性信息的服务(如 AutoNAT)
func WithReadOnlyBlackHoleDetector() Option {
	return func(s *Swarm) error {
		s.readOnlyBHD = true
		return nil
	}
}

// Swarm 是一个连接多路复用器
// 允许打开和关闭与其他对等点的连接,同时使用相同的通道进行所有通信
// 通道发送/接收消息时会注明目标或源对等点
type Swarm struct {
	nextConnID   atomic.Uint64 // 连接 ID 计数器
	nextStreamID atomic.Uint64 // 流 ID 计数器

	refs sync.WaitGroup // 引用计数,等待 swarm 完全拆除

	emitter event.Emitter // 事件发射器

	rcmgr network.ResourceManager // 资源管理器

	local peer.ID             // 本地对等点 ID
	peers peerstore.Peerstore // 对等点存储

	dialTimeout      time.Duration // 拨号超时时间
	dialTimeoutLocal time.Duration // 本地拨号超时时间

	conns struct { // 连接管理
		sync.RWMutex
		m map[peer.ID][]*Conn
	}

	listeners struct { // 监听器管理
		sync.RWMutex

		ifaceListenAddres []ma.Multiaddr
		cacheEOL          time.Time

		m map[transport.Listener]struct{}
	}

	notifs struct { // 通知管理
		sync.RWMutex
		m map[network.Notifiee]struct{}
	}

	directConnNotifs struct { // 直接连接通知管理
		sync.Mutex
		m map[peer.ID][]chan struct{}
	}

	transports struct { // 传输层管理
		sync.RWMutex
		m map[int]transport.Transport
	}

	multiaddrResolver network.MultiaddrDNSResolver // 多地址解析器

	streamh atomic.Pointer[network.StreamHandler] // 流处理器

	dsync   *dialSync               // 拨号同步器
	backf   DialBackoff             // 拨号退避
	limiter *dialLimiter            // 拨号限制器
	gater   connmgr.ConnectionGater // 连接过滤器

	closeOnce sync.Once          // 确保只关闭一次
	ctx       context.Context    // 关闭时取消的上下文
	ctxCancel context.CancelFunc // 上下文取消函数

	bwc           metrics.Reporter // 带宽报告器
	metricsTracer MetricsTracer    // 指标追踪器

	dialRanker network.DialRanker // 拨号排序器

	connectednessEventEmitter *connectednessEventEmitter
	udpBHF                    *BlackHoleSuccessCounter
	ipv6BHF                   *BlackHoleSuccessCounter
	bhd                       *blackHoleDetector
	readOnlyBHD               bool
}

// NewSwarm 构造一个 Swarm
// 参数:
//   - local: peer.ID 本地节点 ID
//   - peers: peerstore.Peerstore 对等点存储
//   - eventBus: event.Bus 事件总线
//   - opts: ...Option 可选配置项
//
// 返回值:
//   - *Swarm swarm 对象
//   - error 错误信息
//
// 注意:
//   - 如果创建事件发射器失败会返回错误
func NewSwarm(local peer.ID, peers peerstore.Peerstore, eventBus event.Bus, opts ...Option) (*Swarm, error) {
	// 创建连接性变更事件发射器
	emitter, err := eventBus.Emitter(new(event.EvtPeerConnectednessChanged))
	if err != nil {
		log.Debugf("创建连接性变更事件发射器失败: %v", err)
		return nil, err
	}

	// 创建可取消的上下文
	ctx, cancel := context.WithCancel(context.Background())

	// 初始化 swarm 对象
	s := &Swarm{
		local:             local,
		peers:             peers,
		emitter:           emitter,
		ctx:               ctx,
		ctxCancel:         cancel,
		dialTimeout:       defaultDialTimeout,
		dialTimeoutLocal:  defaultDialTimeoutLocal,
		multiaddrResolver: ResolverFromMaDNS{madns.DefaultResolver},
		dialRanker:        DefaultDialRanker,

		// 黑洞检测器配置
		// 在网络上,如果 UDP 拨号被阻塞或没有 IPv6 连接,所有拨号都会失败
		// 所以 100 次拨号中有 5 次成功的低成功率就足够了
		udpBHF:  &BlackHoleSuccessCounter{N: 100, MinSuccesses: 5, Name: "UDP"},
		ipv6BHF: &BlackHoleSuccessCounter{N: 100, MinSuccesses: 5, Name: "IPv6"},
	}

	// 初始化各种映射
	s.conns.m = make(map[peer.ID][]*Conn)
	s.listeners.m = make(map[transport.Listener]struct{})
	s.transports.m = make(map[int]transport.Transport)
	s.notifs.m = make(map[network.Notifiee]struct{})
	s.directConnNotifs.m = make(map[peer.ID][]chan struct{})
	s.connectednessEventEmitter = newConnectednessEventEmitter(s.Connectedness, emitter)

	// 应用可选配置
	for _, opt := range opts {
		if err := opt(s); err != nil {
			log.Debugf("应用配置选项失败: %v", err)
			return nil, err
		}
	}

	// 如果未配置资源管理器,使用空实现
	if s.rcmgr == nil {
		s.rcmgr = &network.NullResourceManager{}
	}

	// 初始化拨号同步器
	s.dsync = newDialSync(s.dialWorkerLoop)

	// 初始化拨号限制器和退避机制
	s.limiter = newDialLimiter(s.dialAddr)
	s.backf.init(s.ctx)

	// 初始化黑洞检测器
	s.bhd = &blackHoleDetector{
		udp:      s.udpBHF,
		ipv6:     s.ipv6BHF,
		mt:       s.metricsTracer,
		readOnly: s.readOnlyBHD,
	}

	return s, nil
}

// Close 关闭 swarm
// 返回值:
//   - error 错误信息
func (s *Swarm) Close() error {
	s.closeOnce.Do(s.close)
	return nil
}

// Done 返回一个在 swarm 关闭时关闭的通道
// 返回值:
//   - <-chan struct{} 关闭通道
func (s *Swarm) Done() <-chan struct{} {
	return s.ctx.Done()
}

// close 执行实际的关闭操作
func (s *Swarm) close() {
	// 取消上下文
	s.ctxCancel()

	// 防止添加新的连接和监听器
	s.listeners.Lock()
	listeners := s.listeners.m
	s.listeners.m = nil
	s.listeners.Unlock()

	s.conns.Lock()
	conns := s.conns.m
	s.conns.m = nil
	s.conns.Unlock()

	// 并行关闭所有监听器
	s.refs.Add(len(listeners))
	for l := range listeners {
		go func(l transport.Listener) {
			defer s.refs.Done()
			if err := l.Close(); err != nil && err != transport.ErrListenerClosed {
				log.Errorf("关闭监听器时出错: %s", err)
			}
		}(l)
	}

	// 关闭所有连接
	for _, cs := range conns {
		for _, c := range cs {
			go func(c *Conn) {
				if err := c.Close(); err != nil {
					log.Errorf("关闭连接时出错: %s", err)
				}
			}(c)
		}
	}

	// 等待所有操作完成
	s.refs.Wait()
	s.connectednessEventEmitter.Close()
	s.emitter.Close()

	// 关闭所有传输层
	s.transports.Lock()
	transports := s.transports.m
	s.transports.m = nil
	s.transports.Unlock()

	// 去重可能在多个协议上监听的传输层
	transportsToClose := make(map[transport.Transport]struct{}, len(transports))
	for _, t := range transports {
		transportsToClose[t] = struct{}{}
	}

	// 并行关闭所有传输层
	var wg sync.WaitGroup
	for t := range transportsToClose {
		if closer, ok := t.(io.Closer); ok {
			wg.Add(1)
			go func(c io.Closer) {
				defer wg.Done()
				if err := c.Close(); err != nil {
					log.Errorf("关闭传输 %T 时出错: %s", c, err)
				}
			}(closer)
		}
	}
	wg.Wait()
}

// addConn 添加一个新的传输连接
// 参数:
//   - tc: transport.CapableConn 传输连接对象
//   - dir: network.Direction 连接方向
//
// 返回值:
//   - *Conn 连接对象
//   - error 错误信息
func (s *Swarm) addConn(tc transport.CapableConn, dir network.Direction) (*Conn, error) {
	var (
		p    = tc.RemotePeer()      // 获取远程对等点 ID
		addr = tc.RemoteMultiaddr() // 获取远程多地址
	)

	// 创建连接状态对象
	var stat network.ConnStats
	if cs, ok := tc.(network.ConnStat); ok {
		stat = cs.Stat()
	}
	stat.Direction = dir
	stat.Opened = time.Now()
	isLimited := stat.Limited

	// 创建并初始化连接对象
	c := &Conn{
		conn:  tc,
		swarm: s,
		stat:  stat,
		id:    s.nextConnID.Add(1),
	}

	// 检查连接过滤器
	if s.gater != nil {
		if allow, _ := s.gater.InterceptUpgraded(c); !allow {
			err := tc.Close()
			if err != nil {
				log.Warnf("关闭与对等点 %s 和地址 %s 的连接失败; err: %s", p, addr, err)
			}
			return nil, ErrGaterDisallowedConnection
		}
	}

	// 添加公钥
	if pk := tc.RemotePublicKey(); pk != nil {
		s.peers.AddPubKey(p, pk)
	}

	// 清除退避状态
	s.backf.Clear(p)

	// 添加连接到 swarm
	s.conns.Lock()
	if s.conns.m == nil {
		s.conns.Unlock()
		tc.Close()
		log.Debugf("swarm 已关闭")
		return nil, ErrSwarmClosed
	}

	// 初始化流映射
	c.streams.m = make(map[*Stream]struct{})
	s.conns.m[p] = append(s.conns.m[p], c)

	// 添加两个 swarm 引用:
	// 1. 在 Conn.doClose 中的关闭通知触发后递减
	// 2. 在 Conn.start 退出时递减
	s.refs.Add(2)

	// 获取通知锁以确保连接通知完成
	c.notifyLk.Lock()
	s.conns.Unlock()

	// 更新连接性状态
	s.connectednessEventEmitter.AddConn(p)

	// 处理非受限连接的通知
	if !isLimited {
		s.directConnNotifs.Lock()
		for _, ch := range s.directConnNotifs.m[p] {
			close(ch)
		}
		delete(s.directConnNotifs.m, p)
		s.directConnNotifs.Unlock()
	}

	// 通知所有观察者
	s.notifyAll(func(f network.Notifiee) {
		f.Connected(s, c)
	})
	c.notifyLk.Unlock()

	// 启动连接
	c.start()
	return c, nil
}

// Peerstore 返回此 swarm 的对等点存储
// 返回值:
//   - peerstore.Peerstore 对等点存储对象
func (s *Swarm) Peerstore() peerstore.Peerstore {
	return s.peers
}

// SetStreamHandler 设置新流的处理程序
// 参数:
//   - handler: network.StreamHandler 流处理程序
func (s *Swarm) SetStreamHandler(handler network.StreamHandler) {
	s.streamh.Store(&handler)
}

// StreamHandler 获取新流的处理程序
// 返回值:
//   - network.StreamHandler 流处理程序
func (s *Swarm) StreamHandler() network.StreamHandler {
	handler := s.streamh.Load()
	if handler == nil {
		return nil
	}
	return *handler
}

// NewStream 在与对等点的任何可用连接上创建新流,必要时进行拨号
// 参数:
//   - ctx: context.Context 上下文对象
//   - p: peer.ID 对等点ID
//
// 返回值:
//   - network.Stream 新建的流对象
//   - error 错误信息
//
// 注意:
//   - 使用 network.WithAllowLimitedConn 在有限(中继)连接上打开流
func (s *Swarm) NewStream(ctx context.Context, p peer.ID) (network.Stream, error) {
	log.Debugf("[%s] 正在打开到对等点 [%s] 的流", s.local, p)

	// 算法:
	// 1. 找到最佳连接,否则,拨号
	// 2. 如果最佳连接受限,则通过连接反转或打洞等待直接连接
	// 3. 尝试打开流
	// 4. 如果底层连接实际上已关闭,则关闭外部连接并重试
	//    如果我们有一个已关闭的连接但直到实际尝试打开流时才注意到,我们会这样做
	//
	// TODO: 即使在非关闭连接上打开流时出错,也要尝试所有连接
	numDials := 0
	for {
		// 获取最佳连接
		c := s.bestConnToPeer(p)
		if c == nil {
			// 如果没有连接且允许拨号
			if nodial, _ := network.GetNoDial(ctx); !nodial {
				numDials++
				if numDials > DialAttempts {
					log.Debugf("超出最大拨号尝试次数")
					return nil, errors.New("超出最大拨号尝试次数")
				}
				var err error
				c, err = s.dialPeer(ctx, p)
				if err != nil {
					log.Debugf("拨号失败: %v", err)
					return nil, err
				}
			} else {
				return nil, network.ErrNoConn
			}
		}

		// 检查是否允许有限连接
		limitedAllowed, _ := network.GetAllowLimitedConn(ctx)
		if !limitedAllowed && c.Stat().Limited {
			var err error
			c, err = s.waitForDirectConn(ctx, p)
			if err != nil {
				log.Debugf("等待直接连接失败: %v", err)
				return nil, err
			}
		}

		// 创建新流
		str, err := c.NewStream(ctx)
		if err != nil {
			if c.conn.IsClosed() {
				continue
			}
			log.Debugf("创建新流失败: %v", err)
			return nil, err
		}
		return str, nil
	}
}

// waitForDirectConn 等待通过打洞或连接反转建立的直接连接
// 参数:
//   - ctx: context.Context 上下文对象
//   - p: peer.ID 对等点ID
//
// 返回值:
//   - *Conn 连接对象
//   - error 错误信息
func (s *Swarm) waitForDirectConn(ctx context.Context, p peer.ID) (*Conn, error) {
	// 加锁
	s.directConnNotifs.Lock()

	// 获取最佳连接
	c := s.bestConnToPeer(p)
	if c == nil {
		s.directConnNotifs.Unlock()
		log.Debugf("没有找到最佳连接")
		return nil, network.ErrNoConn
	} else if !c.Stat().Limited {
		s.directConnNotifs.Unlock()
		return c, nil
	}

	// 创建通知通道
	ch := make(chan struct{})
	s.directConnNotifs.m[p] = append(s.directConnNotifs.m[p], ch)
	s.directConnNotifs.Unlock()

	// 应用拨号超时
	ctx, cancel := context.WithTimeout(ctx, network.GetDialPeerTimeout(ctx))
	defer cancel()

	// 等待通知
	select {
	case <-ctx.Done():
		// 从通知列表中删除自己
		s.directConnNotifs.Lock()
		defer s.directConnNotifs.Unlock()

		s.directConnNotifs.m[p] = slices.DeleteFunc(
			s.directConnNotifs.m[p],
			func(c chan struct{}) bool { return c == ch },
		)
		if len(s.directConnNotifs.m[p]) == 0 {
			delete(s.directConnNotifs.m, p)
		}
		return nil, ctx.Err()
	case <-ch:
		// 获取最佳连接
		c := s.bestConnToPeer(p)
		if c == nil {
			log.Debugf("没有找到最佳连接")
			return nil, network.ErrNoConn
		}
		if c.Stat().Limited {
			return nil, network.ErrLimitedConn
		}
		return c, nil
	}
}

// ConnsToPeer 返回与对等点的所有活动连接
// 参数:
//   - p: peer.ID 对等点ID
//
// 返回值:
//   - []network.Conn 连接列表
//
// 注意:
//   - TODO: 考虑将连接列表从最佳到最差排序。目前,它按从最旧到最新排序
func (s *Swarm) ConnsToPeer(p peer.ID) []network.Conn {
	s.conns.RLock()
	defer s.conns.RUnlock()
	conns := s.conns.m[p]
	output := make([]network.Conn, len(conns))
	for i, c := range conns {
		output[i] = c
	}
	return output
}

// isBetterConn 比较两个连接的优先级
// 参数:
//   - a: *Conn 连接A
//   - b: *Conn 连接B
//
// 返回值:
//   - bool 如果连接A优于连接B则返回true
func isBetterConn(a, b *Conn) bool {
	// 如果一个受限而另一个不受限,则首选无限制连接
	aLimited := a.Stat().Limited
	bLimited := b.Stat().Limited
	if aLimited != bLimited {
		return !aLimited
	}

	// 如果一个是直接的而另一个不是,则首选直接连接
	aDirect := isDirectConn(a)
	bDirect := isDirectConn(b)
	if aDirect != bDirect {
		return aDirect
	}

	// 否则,首选具有更多打开流的连接
	a.streams.Lock()
	aLen := len(a.streams.m)
	a.streams.Unlock()

	b.streams.Lock()
	bLen := len(b.streams.m)
	b.streams.Unlock()

	if aLen != bLen {
		return aLen > bLen
	}

	// 最后,选择最后一个连接
	return true
}

// bestConnToPeer 返回到对等点的最佳连接
// 参数:
//   - p: peer.ID 对等点ID
//
// 返回值:
//   - *Conn 最佳连接对象
//
// 注意:
//   - TODO: 优先选择某些传输而不是其他传输
//   - 目前,优先选择直接连接而不是中继连接
//   - 对于平局,选择具有最多流的最新非关闭连接
func (s *Swarm) bestConnToPeer(p peer.ID) *Conn {
	s.conns.RLock()
	defer s.conns.RUnlock()

	var best *Conn
	for _, c := range s.conns.m[p] {
		if c.conn.IsClosed() {
			// 我们无论如何很快就会进行垃圾收集
			continue
		}
		if best == nil || isBetterConn(c, best) {
			best = c
		}
	}
	return best
}

// bestAcceptableConnToPeer 返回最佳可接受的连接,考虑传入的ctx
// 参数:
//   - ctx: context.Context 上下文对象
//   - p: peer.ID 对等点ID
//
// 返回值:
//   - *Conn 最佳可接受的连接对象
//
// 注意:
//   - 如果使用 network.WithForceDirectDial,它只返回直接连接,忽略与对等点的任何有限(中继)连接
func (s *Swarm) bestAcceptableConnToPeer(ctx context.Context, p peer.ID) *Conn {
	conn := s.bestConnToPeer(p)

	forceDirect, _ := network.GetForceDirectDial(ctx)
	if forceDirect && !isDirectConn(conn) {
		return nil
	}
	return conn
}

// isDirectConn 检查连接是否为直接连接
// 参数:
//   - c: *Conn 连接对象
//
// 返回值:
//   - bool 如果是直接连接则返回true
func isDirectConn(c *Conn) bool {
	return c != nil && !c.conn.Transport().Proxy()
}

// Connectedness 返回我们与给定对等点的连接性状态
// 参数:
//   - p: peer.ID 对等点ID
//
// 返回值:
//   - network.Connectedness 连接性状态
//
// 注意:
//   - 要检查我们是否有打开的连接,使用 `s.Connectedness(p) == network.Connected`
func (s *Swarm) Connectedness(p peer.ID) network.Connectedness {
	s.conns.RLock()
	defer s.conns.RUnlock()

	return s.connectednessUnlocked(p)
}

// connectednessUnlocked 返回对等点的连接性状态(无锁)
// 参数:
//   - p: peer.ID 对等点ID
//
// 返回值:
//   - network.Connectedness 连接性状态
func (s *Swarm) connectednessUnlocked(p peer.ID) network.Connectedness {
	var haveLimited bool
	for _, c := range s.conns.m[p] {
		if c.IsClosed() {
			// 这些很快就会被垃圾收集
			continue
		}
		if c.Stat().Limited {
			haveLimited = true
		} else {
			return network.Connected
		}
	}
	if haveLimited {
		return network.Limited
	}
	return network.NotConnected
}

// Conns 返回所有连接的切片
// 返回值:
//   - []network.Conn 所有连接的列表
func (s *Swarm) Conns() []network.Conn {
	s.conns.RLock()
	defer s.conns.RUnlock()

	conns := make([]network.Conn, 0, len(s.conns.m))
	for _, cs := range s.conns.m {
		for _, c := range cs {
			conns = append(conns, c)
		}
	}
	return conns
}

// ClosePeer 关闭与给定对等点的所有连接
// 参数:
//   - p: peer.ID 对等点ID
//
// 返回值:
//   - error 错误信息
func (s *Swarm) ClosePeer(p peer.ID) error {
	conns := s.ConnsToPeer(p)
	switch len(conns) {
	case 0:
		return nil
	case 1:
		return conns[0].Close()
	default:
		errCh := make(chan error)
		for _, c := range conns {
			go func(c network.Conn) {
				errCh <- c.Close()
			}(c)
		}

		var errs []string
		for range conns {
			err := <-errCh
			if err != nil {
				errs = append(errs, err.Error())
			}
		}
		if len(errs) > 0 {
			log.Debugf("断开与对等点 %s 的连接时: %s", p, strings.Join(errs, ", "))
			return fmt.Errorf("断开与对等点 %s 的连接时: %s", p, strings.Join(errs, ", "))
		}
		return nil
	}
}

// Peers 返回swarm连接到的对等点集合的副本
// 返回值:
//   - []peer.ID 对等点ID列表
func (s *Swarm) Peers() []peer.ID {
	s.conns.RLock()
	defer s.conns.RUnlock()
	peers := make([]peer.ID, 0, len(s.conns.m))
	for p := range s.conns.m {
		peers = append(peers, p)
	}

	return peers
}

// LocalPeer 返回与swarm关联的本地对等点ID
// 返回值:
//   - peer.ID 本地对等点ID
func (s *Swarm) LocalPeer() peer.ID {
	return s.local
}

// Backoff 返回此swarm的DialBackoff对象
// 返回值:
//   - *DialBackoff 拨号退避对象
func (s *Swarm) Backoff() *DialBackoff {
	return &s.backf
}

// notifyAll 向所有Notifiee发送信号
// 参数:
//   - notify: func(network.Notifiee) 通知函数
func (s *Swarm) notifyAll(notify func(network.Notifiee)) {
	s.notifs.RLock()
	for f := range s.notifs.m {
		notify(f)
	}
	s.notifs.RUnlock()
}

// Notify 注册Notifiee以在事件发生时接收信号
// 参数:
//   - f: network.Notifiee 通知接收者
func (s *Swarm) Notify(f network.Notifiee) {
	s.notifs.Lock()
	s.notifs.m[f] = struct{}{}
	s.notifs.Unlock()
}

// StopNotify 取消注册Notifiee以接收信号
// 参数:
//   - f: network.Notifiee 通知接收者
func (s *Swarm) StopNotify(f network.Notifiee) {
	s.notifs.Lock()
	delete(s.notifs.m, f)
	s.notifs.Unlock()
}

// removeConn 从swarm中移除连接
// 参数:
//   - c: *Conn 要移除的连接
func (s *Swarm) removeConn(c *Conn) {
	p := c.RemotePeer()

	s.conns.Lock()
	cs := s.conns.m[p]
	for i, ci := range cs {
		if ci == c {
			// 注意:我们故意保留顺序
			// 这样,与对等点的连接总是按从最旧到最新排序
			copy(cs[i:], cs[i+1:])
			cs[len(cs)-1] = nil
			s.conns.m[p] = cs[:len(cs)-1]
			break
		}
	}
	if len(s.conns.m[p]) == 0 {
		delete(s.conns.m, p)
	}
	s.conns.Unlock()
}

// String 返回Network的字符串表示形式
// 返回值:
//   - string 字符串表示
func (s *Swarm) String() string {
	return fmt.Sprintf("<Swarm %s>", s.LocalPeer())
}

// ResourceManager 返回资源管理器
// 返回值:
//   - network.ResourceManager 资源管理器对象
func (s *Swarm) ResourceManager() network.ResourceManager {
	return s.rcmgr
}

// Swarm 实现了 Network 接口
var (
	_ network.Network            = (*Swarm)(nil)
	_ transport.TransportNetwork = (*Swarm)(nil)
)

// connWithMetrics 包含了带有指标追踪的连接
type connWithMetrics struct {
	transport.CapableConn                   // 底层的可扩展连接
	opened                time.Time         // 连接打开时间
	dir                   network.Direction // 连接方向
	metricsTracer         MetricsTracer     // 指标追踪器
	once                  sync.Once         // 确保关闭操作只执行一次
	closeErr              error             // 关闭时的错误
}

// wrapWithMetrics 使用指标追踪器包装一个可扩展连接
// 参数:
//   - capableConn: transport.CapableConn 可扩展连接
//   - metricsTracer: MetricsTracer 指标追踪器
//   - opened: time.Time 连接打开时间
//   - dir: network.Direction 连接方向
//
// 返回值:
//   - *connWithMetrics 带有指标追踪的连接
func wrapWithMetrics(capableConn transport.CapableConn, metricsTracer MetricsTracer, opened time.Time, dir network.Direction) *connWithMetrics {
	// 创建带指标的连接对象
	c := &connWithMetrics{CapableConn: capableConn, opened: opened, dir: dir, metricsTracer: metricsTracer}
	// 记录连接打开事件
	c.metricsTracer.OpenedConnection(c.dir, capableConn.RemotePublicKey(), capableConn.ConnState(), capableConn.LocalMultiaddr())
	return c
}

// completedHandshake 记录握手完成事件
func (c *connWithMetrics) completedHandshake() {
	// 记录握手完成的指标
	c.metricsTracer.CompletedHandshake(time.Since(c.opened), c.ConnState(), c.LocalMultiaddr())
}

// Close 关闭连接并记录指标
// 返回值:
//   - error 关闭时的错误
func (c *connWithMetrics) Close() error {
	// 确保关闭操作只执行一次
	c.once.Do(func() {
		// 记录连接关闭事件
		c.metricsTracer.ClosedConnection(c.dir, time.Since(c.opened), c.ConnState(), c.LocalMultiaddr())
		// 关闭底层连接
		c.closeErr = c.CapableConn.Close()
	})
	return c.closeErr
}

// Stat 返回连接统计信息
// 返回值:
//   - network.ConnStats 连接统计信息
func (c *connWithMetrics) Stat() network.ConnStats {
	// 如果底层连接支持统计功能，则返回其统计信息
	if cs, ok := c.CapableConn.(network.ConnStat); ok {
		return cs.Stat()
	}
	// 否则返回空统计信息
	return network.ConnStats{}
}

// 确保 connWithMetrics 实现了 network.ConnStat 接口
var _ network.ConnStat = &connWithMetrics{}

// ResolverFromMaDNS 包装了 madns.Resolver
type ResolverFromMaDNS struct {
	*madns.Resolver
}

// 确保 ResolverFromMaDNS 实现了 network.MultiaddrDNSResolver 接口
var _ network.MultiaddrDNSResolver = ResolverFromMaDNS{}

// startsWithDNSADDR 检查多地址是否以 DNSADDR 开头
// 参数:
//   - m: ma.Multiaddr 要检查的多地址
//
// 返回值:
//   - bool 是否以 DNSADDR 开头
func startsWithDNSADDR(m ma.Multiaddr) bool {
	if m == nil {
		return false
	}

	startsWithDNSADDR := false
	// 使用 ForEach 避免内存分配
	ma.ForEach(m, func(c ma.Component) bool {
		startsWithDNSADDR = c.Protocol().Code == ma.P_DNSADDR
		return false
	})
	return startsWithDNSADDR
}

// ResolveDNSAddr 实现 MultiaddrDNSResolver 接口，解析 DNS 地址
// 参数:
//   - ctx: context.Context 上下文
//   - expectedPeerID: peer.ID 期望的对等点ID
//   - maddr: ma.Multiaddr 要解析的多地址
//   - recursionLimit: int 递归限制
//   - outputLimit: int 输出限制
//
// 返回值:
//   - []ma.Multiaddr 解析后的多地址列表
//   - error 解析过程中的错误
func (r ResolverFromMaDNS) ResolveDNSAddr(ctx context.Context, expectedPeerID peer.ID, maddr ma.Multiaddr, recursionLimit int, outputLimit int) ([]ma.Multiaddr, error) {
	// 检查输出限制
	if outputLimit <= 0 {
		return nil, nil
	}
	// 检查递归限制
	if recursionLimit <= 0 {
		return []ma.Multiaddr{maddr}, nil
	}
	var resolved, toResolve []ma.Multiaddr
	// 解析地址
	addrs, err := r.Resolve(ctx, maddr)
	if err != nil {
		log.Debugf("解析DNS地址失败: %v", err)
		return nil, err
	}
	// 应用输出限制
	if len(addrs) > outputLimit {
		addrs = addrs[:outputLimit]
	}

	// 分类地址
	for _, addr := range addrs {
		if startsWithDNSADDR(addr) {
			toResolve = append(toResolve, addr)
		} else {
			resolved = append(resolved, addr)
		}
	}

	// 递归解析 DNSADDR
	for i, addr := range toResolve {
		// 设置下一个输出限制为:
		//   outputLimit
		//   - len(resolved)          // 已解析的数量
		//   - (len(toResolve) - i)   // 剩余待解析的地址数量
		//   + 1                      // 当前正在解析的地址
		// 这假设每个 DNSADDR 地址至少会解析为一个多地址。
		// 这个假设让我们可以限制解析所需的空间。
		nextOutputLimit := outputLimit - len(resolved) - (len(toResolve) - i) + 1
		resolvedAddrs, err := r.ResolveDNSAddr(ctx, expectedPeerID, addr, recursionLimit-1, nextOutputLimit)
		if err != nil {
			// 丢弃此地址
			log.Warnf("解析 dnsaddr 失败 %v %s: ", addr, err)
			continue
		}
		resolved = append(resolved, resolvedAddrs...)
	}

	// 应用输出限制
	if len(resolved) > outputLimit {
		resolved = resolved[:outputLimit]
	}

	// 验证对等点ID
	if expectedPeerID != "" {
		removeMismatchPeerID := func(a ma.Multiaddr) bool {
			id, err := peer.IDFromP2PAddr(a)
			if err == peer.ErrInvalidAddr {
				// 此多地址不包含对等点ID，假设它是为此对等点准备的。
				// 如果不是，握手时会失败。
				return false
			} else if err != nil {
				// 此多地址无效，丢弃它。
				return true
			}
			return id != expectedPeerID
		}
		resolved = slices.DeleteFunc(resolved, removeMismatchPeerID)
	}

	return resolved, nil
}

// ResolveDNSComponent 实现 MultiaddrDNSResolver 接口，解析 DNS 组件
// 参数:
//   - ctx: context.Context 上下文
//   - maddr: ma.Multiaddr 要解析的多地址
//   - outputLimit: int 输出限制
//
// 返回值:
//   - []ma.Multiaddr 解析后的多地址列表
//   - error 解析过程中的错误
func (r ResolverFromMaDNS) ResolveDNSComponent(ctx context.Context, maddr ma.Multiaddr, outputLimit int) ([]ma.Multiaddr, error) {
	// 解析地址
	addrs, err := r.Resolve(ctx, maddr)
	if err != nil {
		log.Debugf("解析DNS地址失败: %v", err)
		return nil, err
	}
	// 应用输出限制
	if len(addrs) > outputLimit {
		addrs = addrs[:outputLimit]
	}
	return addrs, nil
}
