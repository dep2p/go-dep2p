package quicreuse

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"

	"github.com/dep2p/go-dep2p/core/transport"
	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	"github.com/quic-go/quic-go"
)

// Listener 定义了 QUIC 监听器接口
// 参数:
//   - Accept: 接受新的连接
//   - Addr: 返回监听地址
//   - Multiaddrs: 返回多地址列表
//   - io.Closer: 关闭监听器
type Listener interface {
	Accept(context.Context) (quic.Connection, error)
	Addr() net.Addr
	Multiaddrs() []ma.Multiaddr
	io.Closer
}

// protoConf 定义了协议配置
// 参数:
//   - ln: *listener 监听器实例
//   - tlsConf: *tls.Config TLS 配置
//   - allowWindowIncrease: 允许增加窗口大小的回调函数
type protoConf struct {
	ln                  *listener
	tlsConf             *tls.Config
	allowWindowIncrease func(conn quic.Connection, delta uint64) bool
}

// quicListener 实现了 QUIC 监听器
// 参数:
//   - l: *quic.Listener QUIC 监听器实例
//   - transport: refCountedQuicTransport 引用计数的传输层
//   - running: chan struct{} 运行状态通道
//   - addrs: []ma.Multiaddr 多地址列表
//   - protocolsMu: sync.Mutex 协议映射锁
//   - protocols: map[string]protoConf 协议配置映射
type quicListener struct {
	l         *quic.Listener
	transport refCountedQuicTransport
	running   chan struct{}
	addrs     []ma.Multiaddr

	protocolsMu sync.Mutex
	protocols   map[string]protoConf
}

// newQuicListener 创建新的 QUIC 监听器
// 参数:
//   - tr: refCountedQuicTransport 引用计数的传输层
//   - quicConfig: *quic.Config QUIC 配置
//
// 返回值:
//   - *quicListener 监听器实例
//   - error 错误信息
func newQuicListener(tr refCountedQuicTransport, quicConfig *quic.Config) (*quicListener, error) {
	// 创建本地多地址列表
	localMultiaddrs := make([]ma.Multiaddr, 0, 2)
	a, err := ToQuicMultiaddr(tr.LocalAddr(), quic.Version1)
	if err != nil {
		log.Debugf("将网络地址转换为QUIC多地址时出错: %s", err)
		return nil, err
	}
	localMultiaddrs = append(localMultiaddrs, a)

	// 创建监听器实例
	cl := &quicListener{
		protocols: map[string]protoConf{},
		running:   make(chan struct{}),
		transport: tr,
		addrs:     localMultiaddrs,
	}

	// 配置 TLS
	tlsConf := &tls.Config{
		SessionTicketsDisabled: true, // 禁用会话票据
		GetConfigForClient: func(info *tls.ClientHelloInfo) (*tls.Config, error) {
			cl.protocolsMu.Lock()
			defer cl.protocolsMu.Unlock()
			for _, proto := range info.SupportedProtos {
				if entry, ok := cl.protocols[proto]; ok {
					conf := entry.tlsConf
					if conf.GetConfigForClient != nil {
						return conf.GetConfigForClient(info)
					}
					return conf, nil
				}
			}
			return nil, fmt.Errorf("未找到支持的协议。提供的协议: %+v", info.SupportedProtos)
		},
	}

	// 克隆并配置 QUIC 配置
	quicConf := quicConfig.Clone()
	quicConf.AllowConnectionWindowIncrease = cl.allowWindowIncrease
	ln, err := tr.Listen(tlsConf, quicConf)
	if err != nil {
		log.Debugf("创建QUIC监听器时出错: %s", err)
		return nil, err
	}
	cl.l = ln
	go cl.Run() // 启动监听循环
	return cl, nil
}

// allowWindowIncrease 检查是否允许增加连接窗口大小
// 参数:
//   - conn: quic.Connection QUIC 连接
//   - delta: uint64 增加的窗口大小
//
// 返回值:
//   - bool 是否允许增加
func (l *quicListener) allowWindowIncrease(conn quic.Connection, delta uint64) bool {
	l.protocolsMu.Lock()
	defer l.protocolsMu.Unlock()

	conf, ok := l.protocols[conn.ConnectionState().TLS.NegotiatedProtocol]
	if !ok {
		return false
	}
	return conf.allowWindowIncrease(conn, delta)
}

// Add 添加新的协议监听
// 参数:
//   - tlsConf: *tls.Config TLS 配置
//   - allowWindowIncrease: 允许增加窗口大小的回调函数
//   - onRemove: 移除时的回调函数
//
// 返回值:
//   - Listener 监听器接口
//   - error 错误信息
func (l *quicListener) Add(tlsConf *tls.Config, allowWindowIncrease func(conn quic.Connection, delta uint64) bool, onRemove func()) (Listener, error) {
	l.protocolsMu.Lock()
	defer l.protocolsMu.Unlock()

	if len(tlsConf.NextProtos) == 0 {
		log.Debugf("tls.Config 中未找到 ALPN")
		return nil, fmt.Errorf("tls.Config 中未找到 ALPN")
	}

	for _, proto := range tlsConf.NextProtos {
		if _, ok := l.protocols[proto]; ok {
			log.Debugf("已在监听协议 %s", proto)
			return nil, fmt.Errorf("已在监听协议 %s", proto)
		}
	}

	ln := newSingleListener(l.l.Addr(), l.addrs, func() {
		l.protocolsMu.Lock()
		for _, proto := range tlsConf.NextProtos {
			delete(l.protocols, proto)
		}
		l.protocolsMu.Unlock()
		onRemove()
	}, l.running)
	for _, proto := range tlsConf.NextProtos {
		l.protocols[proto] = protoConf{
			ln:                  ln,
			tlsConf:             tlsConf,
			allowWindowIncrease: allowWindowIncrease,
		}
	}
	return ln, nil
}

// Run 运行监听循环
// 返回值:
//   - error 错误信息
func (l *quicListener) Run() error {
	defer close(l.running)
	defer l.transport.DecreaseCount()
	for {
		conn, err := l.l.Accept(context.Background())
		if err != nil {
			if errors.Is(err, quic.ErrServerClosed) || strings.Contains(err.Error(), "use of closed network connection") {
				log.Debugf("QUIC监听器已关闭")
				return transport.ErrListenerClosed
			}
			log.Debugf("接受新连接时出错: %s", err)
			return err
		}
		proto := conn.ConnectionState().TLS.NegotiatedProtocol

		l.protocolsMu.Lock()
		ln, ok := l.protocols[proto]
		if !ok {
			l.protocolsMu.Unlock()
			log.Debugf("协商了未知协议: %s", proto)
			return fmt.Errorf("协商了未知协议: %s", proto)
		}
		ln.ln.add(conn)
		l.protocolsMu.Unlock()
	}
}

// Close 关闭监听器
// 返回值:
//   - error 错误信息
func (l *quicListener) Close() error {
	err := l.l.Close()
	<-l.running // 等待 Run 返回
	return err
}

// 接受队列长度
const queueLen = 16

// listener 实现了单个 ALPN 协议集的监听器
// 参数:
//   - queue: chan quic.Connection 连接队列
//   - acceptLoopRunning: chan struct{} 接受循环运行状态
//   - addr: net.Addr 监听地址
//   - addrs: []ma.Multiaddr 多地址列表
//   - remove: func() 移除回调
//   - closeOnce: sync.Once 确保只关闭一次
type listener struct {
	queue             chan quic.Connection
	acceptLoopRunning chan struct{}
	addr              net.Addr
	addrs             []ma.Multiaddr
	remove            func()
	closeOnce         sync.Once
}

// 确保 listener 实现了 Listener 接口
var _ Listener = &listener{}

// newSingleListener 创建新的单协议监听器
// 参数:
//   - addr: net.Addr 监听地址
//   - addrs: []ma.Multiaddr 多地址列表
//   - remove: func() 移除回调
//   - running: chan struct{} 运行状态通道
//
// 返回值:
//   - *listener 监听器实例
func newSingleListener(addr net.Addr, addrs []ma.Multiaddr, remove func(), running chan struct{}) *listener {
	return &listener{
		queue:             make(chan quic.Connection, queueLen),
		acceptLoopRunning: running,
		remove:            remove,
		addr:              addr,
		addrs:             addrs,
	}
}

// add 添加新连接到队列
// 参数:
//   - c: quic.Connection QUIC 连接
func (l *listener) add(c quic.Connection) {
	select {
	case l.queue <- c:
	default:
		c.CloseWithError(1, "队列已满")
	}
}

// Accept 接受新连接
// 参数:
//   - ctx: context.Context 上下文
//
// 返回值:
//   - quic.Connection QUIC 连接
//   - error 错误信息
func (l *listener) Accept(ctx context.Context) (quic.Connection, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-l.acceptLoopRunning:
		return nil, transport.ErrListenerClosed
	case c, ok := <-l.queue:
		if !ok {
			return nil, transport.ErrListenerClosed
		}
		return c, nil
	}
}

// Addr 返回监听地址
// 返回值:
//   - net.Addr 网络地址
func (l *listener) Addr() net.Addr {
	return l.addr
}

// Multiaddrs 返回多地址列表
// 返回值:
//   - []ma.Multiaddr 多地址列表
func (l *listener) Multiaddrs() []ma.Multiaddr {
	return l.addrs
}

// Close 关闭监听器
// 返回值:
//   - error 错误信息
func (l *listener) Close() error {
	l.closeOnce.Do(func() {
		l.remove()
		close(l.queue)
		// 清空队列
		for conn := range l.queue {
			conn.CloseWithError(1, "正在关闭")
		}
	})
	return nil
}
