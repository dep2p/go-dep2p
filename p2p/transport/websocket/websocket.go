// websocket 包实现了基于 websocket 的 go-dep2p 传输层
package websocket

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/transport"
	"github.com/dep2p/go-dep2p/p2p/transport/tcpreuse"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	mafmt "github.com/dep2p/go-dep2p/multiformats/multiaddr/fmt"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"

	ws "github.com/gorilla/websocket"
)

// WsFmt 是 WsProtocol 的多地址格式化器
var WsFmt = mafmt.And(mafmt.TCP, mafmt.Base(ma.P_WS))

// dialMatcher 是用于匹配可拨号地址的格式化器
var dialMatcher = mafmt.And(
	mafmt.Or(mafmt.IP, mafmt.DNS),
	mafmt.Base(ma.P_TCP),
	mafmt.Or(
		mafmt.Base(ma.P_WS),
		mafmt.And(
			mafmt.Or(
				mafmt.And(
					mafmt.Base(ma.P_TLS),
					mafmt.Base(ma.P_SNI)),
				mafmt.Base(ma.P_TLS),
			),
			mafmt.Base(ma.P_WS)),
		mafmt.Base(ma.P_WSS)))

var (
	// wssComponent 是 WSS 协议的多地址组件
	wssComponent = ma.StringCast("/wss")
	// tlsWsComponent 是 TLS+WS 协议的多地址组件
	tlsWsComponent = ma.StringCast("/tls/ws")
	// tlsComponent 是 TLS 协议的多地址组件
	tlsComponent = ma.StringCast("/tls")
	// wsComponent 是 WS 协议的多地址组件
	wsComponent = ma.StringCast("/ws")
)

func init() {
	// 注册网络地址解析器
	manet.RegisterFromNetAddr(ParseWebsocketNetAddr, "websocket")
	manet.RegisterToNetAddr(ConvertWebsocketMultiaddrToNetAddr, "ws")
	manet.RegisterToNetAddr(ConvertWebsocketMultiaddrToNetAddr, "wss")
}

// 默认的 gorilla websocket 升级器
var upgrader = ws.Upgrader{
	// 允许所有来源的请求
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Option 是 WebsocketTransport 的配置函数类型
type Option func(*WebsocketTransport) error

// WithTLSClientConfig 为 WebSocket 拨号器设置 TLS 客户端配置
// 仅适用于非浏览器场景
//
// 参数:
//   - c: *tls.Config TLS 配置对象
//
// 返回值:
//   - Option 配置函数
//
// 常见用例包括设置 InsecureSkipVerify 为 true，或设置用户自定义的受信任 CA 证书
func WithTLSClientConfig(c *tls.Config) Option {
	return func(t *WebsocketTransport) error {
		t.tlsClientConf = c
		return nil
	}
}

// WithTLSConfig 为 WebSocket 监听器设置 TLS 配置
//
// 参数:
//   - conf: *tls.Config TLS 配置对象
//
// 返回值:
//   - Option 配置函数
func WithTLSConfig(conf *tls.Config) Option {
	return func(t *WebsocketTransport) error {
		t.tlsConf = conf
		return nil
	}
}

// WebsocketTransport 是实际的 go-dep2p 传输层实现
type WebsocketTransport struct {
	// upgrader 用于升级连接
	upgrader transport.Upgrader
	// rcmgr 是资源管理器
	rcmgr network.ResourceManager
	// tlsClientConf 是 TLS 客户端配置
	tlsClientConf *tls.Config
	// tlsConf 是 TLS 服务端配置
	tlsConf *tls.Config
	// sharedTcp 是共享的 TCP 连接管理器
	sharedTcp *tcpreuse.ConnMgr
}

var _ transport.Transport = (*WebsocketTransport)(nil)

// New 创建一个新的 WebsocketTransport 实例
//
// 参数:
//   - u: transport.Upgrader 连接升级器
//   - rcmgr: network.ResourceManager 资源管理器
//   - sharedTCP: *tcpreuse.ConnMgr 共享的 TCP 连接管理器
//   - opts: ...Option 配置选项
//
// 返回值:
//   - *WebsocketTransport 新创建的传输层实例
//   - error 可能的错误
func New(u transport.Upgrader, rcmgr network.ResourceManager, sharedTCP *tcpreuse.ConnMgr, opts ...Option) (*WebsocketTransport, error) {
	if rcmgr == nil {
		rcmgr = &network.NullResourceManager{}
	}
	t := &WebsocketTransport{
		upgrader:      u,
		rcmgr:         rcmgr,
		tlsClientConf: &tls.Config{},
		sharedTcp:     sharedTCP,
	}
	for _, opt := range opts {
		if err := opt(t); err != nil {
			log.Debugf("配置 WebsocketTransport 失败: %s", err)
			return nil, err
		}
	}
	return t, nil
}

// CanDial 检查是否可以拨号给定的多地址
//
// 参数:
//   - a: ma.Multiaddr 待检查的多地址
//
// 返回值:
//   - bool 是否可以拨号
func (t *WebsocketTransport) CanDial(a ma.Multiaddr) bool {
	return dialMatcher.Matches(a)
}

// Protocols 返回支持的协议列表
//
// 返回值:
//   - []int 支持的协议代码列表
func (t *WebsocketTransport) Protocols() []int {
	return []int{ma.P_WS, ma.P_WSS}
}

// Proxy 返回是否支持代理
//
// 返回值:
//   - bool 始终返回 false
func (t *WebsocketTransport) Proxy() bool {
	return false
}

// Resolve 解析多地址
//
// 参数:
//   - ctx: context.Context 上下文
//   - maddr: ma.Multiaddr 待解析的多地址
//
// 返回值:
//   - []ma.Multiaddr 解析后的多地址列表
//   - error 可能的错误
func (t *WebsocketTransport) Resolve(_ context.Context, maddr ma.Multiaddr) ([]ma.Multiaddr, error) {
	parsed, err := parseWebsocketMultiaddr(maddr)
	if err != nil {
		log.Debugf("解析 WebSocket 多地址失败: %s", err)
		return nil, err
	}

	if !parsed.isWSS {
		// 非安全 websocket 多地址，直接返回
		return []ma.Multiaddr{maddr}, nil
	}

	if parsed.sni == nil {
		var err error
		// 没有 SNI 组件，使用 DNS
		ma.ForEach(parsed.restMultiaddr, func(c ma.Component) bool {
			switch c.Protocol().Code {
			case ma.P_DNS, ma.P_DNS4, ma.P_DNS6:
				parsed.sni, err = ma.NewComponent("sni", c.Value())
				return false
			}
			return true
		})
		if err != nil {
			log.Debugf("解析 WebSocket 多地址失败: %s", err)
			return nil, err
		}
	}

	if parsed.sni == nil {
		// 未找到可用于设置 SNI 的内容，返回原多地址
		return []ma.Multiaddr{maddr}, nil
	}

	return []ma.Multiaddr{parsed.toMultiaddr()}, nil
}

// Dial 拨号连接到远程节点
//
// 参数:
//   - ctx: context.Context 上下文
//   - raddr: ma.Multiaddr 远程地址
//   - p: peer.ID 对端节点 ID
//
// 返回值:
//   - transport.CapableConn 建立的连接
//   - error 可能的错误
func (t *WebsocketTransport) Dial(ctx context.Context, raddr ma.Multiaddr, p peer.ID) (transport.CapableConn, error) {
	connScope, err := t.rcmgr.OpenConnection(network.DirOutbound, true, raddr)
	if err != nil {
		log.Debugf("打开连接失败: %s", err)
		return nil, err
	}
	c, err := t.dialWithScope(ctx, raddr, p, connScope)
	if err != nil {
		connScope.Done()
		log.Debugf("拨号失败: %s", err)
		return nil, err
	}
	return c, nil
}

// dialWithScope 在给定作用域内拨号连接
//
// 参数:
//   - ctx: context.Context 上下文
//   - raddr: ma.Multiaddr 远程地址
//   - p: peer.ID 对端节点 ID
//   - connScope: network.ConnManagementScope 连接管理作用域
//
// 返回值:
//   - transport.CapableConn 建立的连接
//   - error 可能的错误
func (t *WebsocketTransport) dialWithScope(ctx context.Context, raddr ma.Multiaddr, p peer.ID, connScope network.ConnManagementScope) (transport.CapableConn, error) {
	macon, err := t.maDial(ctx, raddr)
	if err != nil {
		log.Debugf("拨号失败: %s", err)
		return nil, err
	}
	conn, err := t.upgrader.Upgrade(ctx, t, macon, network.DirOutbound, p, connScope)
	if err != nil {
		log.Debugf("升级连接失败: %s", err)
		return nil, err
	}
	return &capableConn{CapableConn: conn}, nil
}

// maDial 拨号到多地址
//
// 参数:
//   - ctx: context.Context 上下文
//   - raddr: ma.Multiaddr 远程地址
//
// 返回值:
//   - manet.Conn 建立的连接
//   - error 可能的错误
func (t *WebsocketTransport) maDial(ctx context.Context, raddr ma.Multiaddr) (manet.Conn, error) {
	wsurl, err := parseMultiaddr(raddr)
	if err != nil {
		log.Debugf("解析 WebSocket 多地址失败: %s", err)
		return nil, err
	}
	isWss := wsurl.Scheme == "wss"
	dialer := ws.Dialer{HandshakeTimeout: 30 * time.Second}
	if isWss {
		sni := ""
		sni, err = raddr.ValueForProtocol(ma.P_SNI)
		if err != nil {
			sni = ""
		}

		if sni != "" {
			copytlsClientConf := t.tlsClientConf.Clone()
			copytlsClientConf.ServerName = sni
			dialer.TLSClientConfig = copytlsClientConf
			ipAddr := wsurl.Host
			// 设置 NetDial 因为我们已经有了解析后的 IP 地址，不需要再次解析
			// 将 .Host 设置为 SNI 字段以正确设置 host header
			dialer.NetDial = func(network, address string) (net.Conn, error) {
				tcpAddr, err := net.ResolveTCPAddr(network, ipAddr)
				if err != nil {
					log.Debugf("解析 TCP 地址失败: %s", err)
					return nil, err
				}
				return net.DialTCP("tcp", nil, tcpAddr)
			}
			wsurl.Host = sni + ":" + wsurl.Port()
		} else {
			dialer.TLSClientConfig = t.tlsClientConf
		}
	}

	wscon, _, err := dialer.DialContext(ctx, wsurl.String(), nil)
	if err != nil {
		log.Debugf("拨号失败: %s", err)
		return nil, err
	}

	mnc, err := manet.WrapNetConn(NewConn(wscon, isWss))
	if err != nil {
		wscon.Close()
		log.Debugf("包装网络连接失败: %s", err)
		return nil, err
	}
	return mnc, nil
}

// maListen 监听多地址
//
// 参数:
//   - a: ma.Multiaddr 监听地址
//
// 返回值:
//   - manet.Listener 创建的监听器
//   - error 可能的错误
func (t *WebsocketTransport) maListen(a ma.Multiaddr) (manet.Listener, error) {
	var tlsConf *tls.Config
	if t.tlsConf != nil {
		tlsConf = t.tlsConf.Clone()
	}
	l, err := newListener(a, tlsConf, t.sharedTcp)
	if err != nil {
		log.Debugf("创建监听器失败: %s", err)
		return nil, err
	}
	go l.serve()
	return l, nil
}

// Listen 在给定地址上创建监听器
//
// 参数:
//   - a: ma.Multiaddr 监听地址
//
// 返回值:
//   - transport.Listener 创建的监听器
//   - error 可能的错误
func (t *WebsocketTransport) Listen(a ma.Multiaddr) (transport.Listener, error) {
	malist, err := t.maListen(a)
	if err != nil {
		log.Debugf("创建监听器失败: %s", err)
		return nil, err
	}
	return &transportListener{Listener: t.upgrader.UpgradeListener(t, malist)}, nil
}
