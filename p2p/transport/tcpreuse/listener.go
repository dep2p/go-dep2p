package tcpreuse

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/connmgr"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/transport"
	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	"github.com/dep2p/go-dep2p/p2p/net/reuseport"
	logging "github.com/dep2p/log"
)

// 并行读取64个连接的3个字节是合理的
const acceptQueueSize = 64

// 等待连接被接受的超时时间，超过此时间将丢弃连接
const acceptTimeout = 30 * time.Second

var log = logging.Logger("p2p-transport-tcpreuse")

// ConnMgr 允许在TCP和WebSocket传输之间共享相同的监听地址
type ConnMgr struct {
	enableReuseport bool                    // 是否启用端口重用
	reuse           reuseport.Transport     // 端口重用传输实例
	connGater       connmgr.ConnectionGater // 连接过滤器
	rcmgr           network.ResourceManager // 资源管理器

	mx        sync.Mutex                      // 互斥锁
	listeners map[string]*multiplexedListener // 多路复用监听器映射
}

// NewConnMgr 创建一个新的连接管理器
// 参数:
//   - enableReuseport: bool 是否启用端口重用
//   - gater: connmgr.ConnectionGater 连接过滤器
//   - rcmgr: network.ResourceManager 资源管理器
//
// 返回值:
//   - *ConnMgr 新创建的连接管理器
func NewConnMgr(enableReuseport bool, gater connmgr.ConnectionGater, rcmgr network.ResourceManager) *ConnMgr {
	// 如果未提供资源管理器，使用空实现
	if rcmgr == nil {
		rcmgr = &network.NullResourceManager{}
	}
	return &ConnMgr{
		enableReuseport: enableReuseport,
		reuse:           reuseport.Transport{},
		connGater:       gater,
		rcmgr:           rcmgr,
		listeners:       make(map[string]*multiplexedListener),
	}
}

// maListen 根据是否启用端口重用创建监听器
// 参数:
//   - listenAddr: ma.Multiaddr 监听地址
//
// 返回值:
//   - manet.Listener 创建的监听器
//   - error 可能的错误
func (t *ConnMgr) maListen(listenAddr ma.Multiaddr) (manet.Listener, error) {
	if t.useReuseport() {
		return t.reuse.Listen(listenAddr)
	} else {
		return manet.Listen(listenAddr)
	}
}

// useReuseport 检查是否应该使用端口重用
// 返回值:
//   - bool 是否使用端口重用
func (t *ConnMgr) useReuseport() bool {
	return t.enableReuseport && ReuseportIsAvailable()
}

// getTCPAddr 从多地址中提取TCP地址部分
// 参数:
//   - listenAddr: ma.Multiaddr 输入的多地址
//
// 返回值:
//   - ma.Multiaddr TCP地址部分
//   - error 可能的错误
func getTCPAddr(listenAddr ma.Multiaddr) (ma.Multiaddr, error) {
	haveTCP := false
	addr, _ := ma.SplitFunc(listenAddr, func(c ma.Component) bool {
		if haveTCP {
			return true
		}
		if c.Protocol().Code == ma.P_TCP {
			haveTCP = true
		}
		return false
	})
	if !haveTCP {
		log.Debugf("无效的监听地址 %s，需要TCP地址", listenAddr)
		return nil, fmt.Errorf("无效的监听地址 %s，需要TCP地址", listenAddr)
	}
	return addr, nil
}

// DemultiplexedListen 为指定地址和连接类型返回一个监听器
// 从返回的监听器接受的连接需要使用 transport.Upgrader 进行升级
// 注意：所有使用端口0的监听器共享相同的底层socket，因此它们具有相同的具体端口
// 参数:
//   - laddr: ma.Multiaddr 监听地址
//   - connType: DemultiplexedConnType 连接类型
//
// 返回值:
//   - manet.Listener 创建的监听器
//   - error 可能的错误
func (t *ConnMgr) DemultiplexedListen(laddr ma.Multiaddr, connType DemultiplexedConnType) (manet.Listener, error) {
	if !connType.IsKnown() {
		log.Debugf("未知的连接类型: %s", connType)
		return nil, fmt.Errorf("未知的连接类型: %s", connType)
	}
	laddr, err := getTCPAddr(laddr)
	if err != nil {
		log.Debugf("获取TCP地址时出错: %s", err)
		return nil, err
	}

	t.mx.Lock()
	defer t.mx.Unlock()
	ml, ok := t.listeners[laddr.String()]
	if ok {
		dl, err := ml.DemultiplexedListen(connType)
		if err != nil {
			log.Debugf("获取连接类型时出错: %s", err)
			return nil, err
		}
		return dl, nil
	}

	l, err := t.maListen(laddr)
	if err != nil {
		log.Debugf("创建监听器时出错: %s", err)
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancelFunc := func() error {
		cancel()
		t.mx.Lock()
		defer t.mx.Unlock()
		delete(t.listeners, laddr.String())
		delete(t.listeners, l.Multiaddr().String())
		return l.Close()
	}
	ml = &multiplexedListener{
		Listener:  l,
		listeners: make(map[DemultiplexedConnType]*demultiplexedListener),
		ctx:       ctx,
		closeFn:   cancelFunc,
		connGater: t.connGater,
		rcmgr:     t.rcmgr,
	}
	t.listeners[laddr.String()] = ml
	t.listeners[l.Multiaddr().String()] = ml

	dl, err := ml.DemultiplexedListen(connType)
	if err != nil {
		cerr := ml.Close()
		log.Debugf("获取连接类型时出错: %s", err)
		return nil, errors.Join(err, cerr)
	}

	ml.wg.Add(1)
	go ml.run()

	return dl, nil
}

var _ manet.Listener = &demultiplexedListener{}

// multiplexedListener 实现了多路复用监听器
type multiplexedListener struct {
	manet.Listener                                                  // 底层监听器
	listeners      map[DemultiplexedConnType]*demultiplexedListener // 按连接类型分类的监听器映射
	mx             sync.RWMutex                                     // 读写锁

	connGater connmgr.ConnectionGater // 连接过滤器
	rcmgr     network.ResourceManager // 资源管理器
	ctx       context.Context         // 上下文
	closeFn   func() error            // 关闭函数
	wg        sync.WaitGroup          // 等待组
}

// ErrListenerExists 表示在此地址上已存在此连接类型的监听器
var ErrListenerExists = errors.New("此地址上已存在此连接类型的监听器")

// DemultiplexedListen 为指定的连接类型创建一个新的多路复用监听器
// 参数:
//   - connType: DemultiplexedConnType 连接类型
//
// 返回值:
//   - manet.Listener 创建的监听器
//   - error 可能的错误
func (m *multiplexedListener) DemultiplexedListen(connType DemultiplexedConnType) (manet.Listener, error) {
	if !connType.IsKnown() {
		log.Debugf("未知的连接类型: %s", connType)
		return nil, fmt.Errorf("未知的连接类型: %s", connType)
	}

	m.mx.Lock()
	defer m.mx.Unlock()
	if _, ok := m.listeners[connType]; ok {
		log.Debugf("此地址上已存在此连接类型的监听器: %s", connType)
		return nil, ErrListenerExists
	}

	ctx, cancel := context.WithCancel(m.ctx)
	l := &demultiplexedListener{
		buffer:     make(chan manet.Conn),
		inner:      m.Listener,
		ctx:        ctx,
		cancelFunc: cancel,
		closeFn:    func() error { m.removeDemultiplexedListener(connType); return nil },
	}

	m.listeners[connType] = l

	return l, nil
}

// run 运行多路复用监听器的主循环
// 返回值:
//   - error 可能的错误
func (m *multiplexedListener) run() error {
	defer m.Close()
	defer m.wg.Done()
	acceptQueue := make(chan struct{}, acceptQueueSize)
	for {
		c, err := m.Listener.Accept()
		if err != nil {
			log.Debugf("接受新连接时出错: %s", err)
			return err
		}

		// 在此处进行连接的过滤和资源限制
		// 如果在采样连接后再做这些，我们将容易受到单个对等点的DOS攻击，这可能会堵塞整个连接队列
		// 这在此处和upgrader之间重复了过滤和资源限制的责任
		// 不重复的替代方案需要将升级连接的过程移到这里，这迫使我们在这里建立websocket连接。这会导致更多的重复，或者是一个重大的破坏性变更
		//
		// 通过传输集成测试可以防止多次调用OpenConnection或InterceptAccept的错误
		if m.connGater != nil && !m.connGater.InterceptAccept(c) {
			log.Debugf("过滤器阻止了来自 %s 到本地地址 %s 的传入连接",
				c.RemoteMultiaddr(), c.LocalMultiaddr())
			if err := c.Close(); err != nil {
				log.Warnf("关闭被过滤器拒绝的传入连接失败: %s", err)
			}
			continue
		}
		connScope, err := m.rcmgr.OpenConnection(network.DirInbound, true, c.RemoteMultiaddr())
		if err != nil {
			log.Debugf("资源管理器阻止接受新连接: %s", err)
			if err := c.Close(); err != nil {
				log.Warnf("关闭传入连接失败。被资源管理器拒绝: %s", err)
			}
			continue
		}

		select {
		case acceptQueue <- struct{}{}:
		// 注意：我们可以丢弃连接，但这与upgrader中的行为类似
		case <-m.ctx.Done():
			c.Close()
			log.Debugf("接受队列已满，丢弃连接: %s", c.RemoteMultiaddr())
		}

		m.wg.Add(1)
		go func() {
			defer func() { <-acceptQueue }()
			defer m.wg.Done()
			ctx, cancelCtx := context.WithTimeout(m.ctx, acceptTimeout)
			defer cancelCtx()
			t, c, err := identifyConnType(c)
			if err != nil {
				connScope.Done()
				log.Debugf("多路复用连接时出错: %s", err.Error())
				return
			}

			connWithScope, err := manetConnWithScope(c, connScope)
			if err != nil {
				connScope.Done()
				closeErr := c.Close()
				err = errors.Join(err, closeErr)
				log.Debugf("用作用域包装连接时出错: %s", err.Error())
				return
			}

			m.mx.RLock()
			demux, ok := m.listeners[t]
			m.mx.RUnlock()
			if !ok {
				closeErr := connWithScope.Close()
				if closeErr != nil {
					log.Debugf("没有为多路复用连接 %s 注册的监听器。关闭连接时出错 %s", t, closeErr.Error())
				} else {
					log.Debugf("没有为多路复用连接 %s 注册的监听器", t)
				}
				return
			}

			select {
			case demux.buffer <- connWithScope:
			case <-ctx.Done():
				connWithScope.Close()
			}
		}()
	}
}

// Close 关闭多路复用监听器
// 返回值:
//   - error 可能的错误
func (m *multiplexedListener) Close() error {
	m.mx.Lock()
	for _, l := range m.listeners {
		l.cancelFunc()
	}
	err := m.closeListener()
	m.mx.Unlock()
	m.wg.Wait()
	return err
}

// closeListener 关闭底层监听器
// 返回值:
//   - error 可能的错误
func (m *multiplexedListener) closeListener() error {
	lerr := m.Listener.Close()
	cerr := m.closeFn()
	return errors.Join(lerr, cerr)
}

// removeDemultiplexedListener 移除指定连接类型的多路复用监听器
// 参数:
//   - c: DemultiplexedConnType 要移除的连接类型
func (m *multiplexedListener) removeDemultiplexedListener(c DemultiplexedConnType) {
	m.mx.Lock()
	defer m.mx.Unlock()

	delete(m.listeners, c)
	if len(m.listeners) == 0 {
		m.closeListener()
		m.mx.Unlock()
		m.wg.Wait()
		m.mx.Lock()
	}
}

// demultiplexedListener 实现了单个连接类型的监听器
type demultiplexedListener struct {
	buffer     chan manet.Conn    // 连接缓冲通道
	inner      manet.Listener     // 内部监听器
	ctx        context.Context    // 上下文
	cancelFunc context.CancelFunc // 取消函数
	closeFn    func() error       // 关闭函数
}

// Accept 接受一个新的连接
// 返回值:
//   - manet.Conn 接受的连接
//   - error 可能的错误
func (m *demultiplexedListener) Accept() (manet.Conn, error) {
	select {
	case c := <-m.buffer:
		return c, nil
	case <-m.ctx.Done():
		return nil, transport.ErrListenerClosed
	}
}

// Close 关闭监听器
// 返回值:
//   - error 可能的错误
func (m *demultiplexedListener) Close() error {
	m.cancelFunc()
	return m.closeFn()
}

// Multiaddr 返回监听器的多地址
// 返回值:
//   - ma.Multiaddr 监听器的多地址
func (m *demultiplexedListener) Multiaddr() ma.Multiaddr {
	return m.inner.Multiaddr()
}

// Addr 返回监听器的网络地址
// 返回值:
//   - net.Addr 监听器的网络地址
func (m *demultiplexedListener) Addr() net.Addr {
	return m.inner.Addr()
}
