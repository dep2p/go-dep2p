package websocket

import (
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"

	"go.uber.org/zap"

	logging "github.com/dep2p/log"

	"github.com/dep2p/go-dep2p/core/transport"
	"github.com/dep2p/go-dep2p/p2p/transport/tcpreuse"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

// 使用 zap 日志记录器创建 websocket 传输层的日志记录器
var log = logging.Logger("p2p-transport-websocket")
var stdLog = zap.NewStdLog(log.Desugar())

// listener 实现 WebSocket 监听器
type listener struct {
	nl     net.Listener // 底层网络监听器
	server http.Server  // HTTP 服务器
	// Go 标准库会设置 http.Server.TLSConfig,无论是 WS 还是 WSS,所以不能依赖检查 server.TLSConfig 来判断
	isWss bool // 是否为 WSS 连接

	laddr ma.Multiaddr // 本地多地址

	incoming chan *Conn // 传入连接的通道

	closeOnce sync.Once     // 确保只关闭一次
	closeErr  error         // 关闭时的错误
	closed    chan struct{} // 关闭信号通道
}

// toMultiaddr 将解析后的 WebSocket 多地址转换为标准多地址
// 返回值:
//   - ma.Multiaddr: 转换后的多地址
func (pwma *parsedWebsocketMultiaddr) toMultiaddr() ma.Multiaddr {
	if !pwma.isWSS {
		return pwma.restMultiaddr.Encapsulate(wsComponent)
	}

	if pwma.sni == nil {
		return pwma.restMultiaddr.Encapsulate(tlsComponent).Encapsulate(wsComponent)
	}

	return pwma.restMultiaddr.Encapsulate(tlsComponent).Encapsulate(pwma.sni).Encapsulate(wsComponent)
}

// newListener 从原始 net.Listener 创建新的监听器
// 参数:
//   - a: ma.Multiaddr 监听地址
//   - tlsConf: *tls.Config TLS 配置,可为 nil(用于未加密的 websocket)
//   - sharedTcp: *tcpreuse.ConnMgr TCP 连接管理器
//
// 返回值:
//   - *listener: 创建的监听器
//   - error: 创建过程中的错误
func newListener(a ma.Multiaddr, tlsConf *tls.Config, sharedTcp *tcpreuse.ConnMgr) (*listener, error) {
	// 解析 WebSocket 多地址
	parsed, err := parseWebsocketMultiaddr(a)
	if err != nil {
		log.Debugf("解析 WebSocket 多地址失败: %s", err)
		return nil, err
	}

	// 检查 WSS 地址是否提供了 TLS 配置
	if parsed.isWSS && tlsConf == nil {
		log.Debugf("无法在没有 tls.Config 的情况下监听 wss 地址 %s", a)
		return nil, fmt.Errorf("无法在没有 tls.Config 的情况下监听 wss 地址 %s", a)
	}

	var nl net.Listener

	if sharedTcp == nil {
		// 获取网络类型和地址
		lnet, lnaddr, err := manet.DialArgs(parsed.restMultiaddr)
		if err != nil {
			log.Debugf("获取网络类型和地址失败: %s", err)
			return nil, err
		}
		// 创建监听器
		nl, err = net.Listen(lnet, lnaddr)
		if err != nil {
			log.Debugf("创建监听器失败: %s", err)
			return nil, err
		}
	} else {
		// 根据是否为 WSS 确定连接类型
		var connType tcpreuse.DemultiplexedConnType
		if parsed.isWSS {
			connType = tcpreuse.DemultiplexedConnType_TLS
		} else {
			connType = tcpreuse.DemultiplexedConnType_HTTP
		}
		// 创建多路复用监听器
		mal, err := sharedTcp.DemultiplexedListen(parsed.restMultiaddr, connType)
		if err != nil {
			log.Debugf("创建多路复用监听器失败: %s", err)
			return nil, err
		}
		nl = manet.NetListener(mal)
	}

	// 获取本地地址
	laddr, err := manet.FromNetAddr(nl.Addr())
	if err != nil {
		log.Debugf("获取本地地址失败: %s", err)
		return nil, err
	}

	// 处理 DNS 地址
	first, _ := ma.SplitFirst(a)
	// 不解析 DNS 地址
	// 我们希望能够通告域名,以便对等方可以验证 TLS 证书
	if c := first.Protocol().Code; c == ma.P_DNS || c == ma.P_DNS4 || c == ma.P_DNS6 || c == ma.P_DNSADDR {
		_, last := ma.SplitFirst(laddr)
		laddr = first.Encapsulate(last)
	}
	parsed.restMultiaddr = laddr

	// 创建监听器对象
	ln := &listener{
		nl:       nl,
		laddr:    parsed.toMultiaddr(),
		incoming: make(chan *Conn),
		closed:   make(chan struct{}),
	}
	ln.server = http.Server{Handler: ln, ErrorLog: stdLog}
	if parsed.isWSS {
		ln.isWss = true
		ln.server.TLSConfig = tlsConf
	}
	return ln, nil
}

// serve 启动监听服务
func (l *listener) serve() {
	defer close(l.closed)
	if !l.isWss {
		l.server.Serve(l.nl)
	} else {
		l.server.ServeTLS(l.nl, "", "")
	}
}

// ServeHTTP 实现 http.Handler 接口
// 参数:
//   - w: http.ResponseWriter 响应写入器
//   - r: *http.Request HTTP 请求
func (l *listener) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// 升级 HTTP 连接为 WebSocket 连接
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		// upgrader 会为我们写入响应
		return
	}
	// 创建新的 WebSocket 连接
	nc := NewConn(c, l.isWss)
	if nc == nil {
		c.Close()
		w.WriteHeader(500)
		return
	}
	// 将连接发送到 incoming 通道
	select {
	case l.incoming <- NewConn(c, l.isWss):
	case <-l.closed:
		c.Close()
	}
	// 连接已被劫持,可以安全返回
}

// Accept 接受新的连接
// 返回值:
//   - manet.Conn: 新接受的连接
//   - error: 接受过程中的错误
func (l *listener) Accept() (manet.Conn, error) {
	select {
	case c, ok := <-l.incoming:
		if !ok {
			log.Debugf("监听器已关闭")
			return nil, transport.ErrListenerClosed
		}
		return c, nil
	case <-l.closed:
		log.Debugf("监听器已关闭")
		return nil, transport.ErrListenerClosed
	}
}

// Addr 返回监听器的网络地址
// 返回值:
//   - net.Addr: 网络地址
func (l *listener) Addr() net.Addr {
	return l.nl.Addr()
}

// Close 关闭监听器
// 返回值:
//   - error: 关闭过程中的错误
func (l *listener) Close() error {
	l.closeOnce.Do(func() {
		err1 := l.nl.Close()
		err2 := l.server.Close()
		<-l.closed
		l.closeErr = errors.Join(err1, err2)
	})
	return l.closeErr
}

// Multiaddr 返回监听器的多地址
// 返回值:
//   - ma.Multiaddr: 多地址
func (l *listener) Multiaddr() ma.Multiaddr {
	return l.laddr
}

// transportListener 包装了 transport.Listener
type transportListener struct {
	transport.Listener
}

// Accept 接受新的传输层连接
// 返回值:
//   - transport.CapableConn: 具有能力的连接
//   - error: 接受过程中的错误
func (l *transportListener) Accept() (transport.CapableConn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		log.Debugf("接受传输层连接失败: %s", err)
		return nil, err
	}
	return &capableConn{CapableConn: conn}, nil
}
