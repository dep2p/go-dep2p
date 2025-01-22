package dep2pwebtransport

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/dep2p/go-dep2p/core/network"
	tpt "github.com/dep2p/go-dep2p/core/transport"
	"github.com/dep2p/go-dep2p/p2p/security/noise"
	"github.com/dep2p/go-dep2p/p2p/security/noise/pb"
	"github.com/dep2p/go-dep2p/p2p/transport/quicreuse"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
)

// 连接队列长度
const queueLen = 16

// 握手超时时间
const handshakeTimeout = 10 * time.Second

// connKey 用于在 context 中存储连接的 key
type connKey struct{}

// negotiatingConn 是对 quic.Connection 的包装
// 在升级过程中使用自定义的 context 来包装连接
// 用于将 quic 连接升级为 h3 连接再升级为 webtransport 会话
type negotiatingConn struct {
	quic.Connection                    // QUIC 连接
	ctx             context.Context    // 上下文
	cancel          context.CancelFunc // 取消函数
	// stopClose 是一个函数,用于在 context 完成时阻止连接关闭
	// 如果连接关闭函数未被调用则返回 true
	stopClose func() bool
	err       error // 错误信息
}

// Unwrap 解包装连接
// 返回值:
//   - quic.Connection QUIC 连接
//   - error 错误信息
func (c *negotiatingConn) Unwrap() (quic.Connection, error) {
	defer c.cancel()
	if c.stopClose != nil {
		// 第一次解包装
		if !c.stopClose() {
			c.err = errTimeout
		}
		c.stopClose = nil
	}
	if c.err != nil {
		log.Debugf("解包装失败: %s", c.err)
		return nil, c.err
	}
	return c.Connection, nil
}

// wrapConn 包装 QUIC 连接
// 参数:
//   - ctx: context.Context 上下文
//   - c: quic.Connection QUIC 连接
//   - handshakeTimeout: time.Duration 握手超时时间
//
// 返回值:
//   - *negotiatingConn 包装后的连接
func wrapConn(ctx context.Context, c quic.Connection, handshakeTimeout time.Duration) *negotiatingConn {
	ctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	stopClose := context.AfterFunc(ctx, func() {
		log.Debugf("连接握手失败: %s", c.RemoteAddr())
		c.CloseWithError(1, "")
	})
	return &negotiatingConn{
		Connection: c,
		ctx:        ctx,
		cancel:     cancel,
		stopClose:  stopClose,
	}
}

// 超时错误
var errTimeout = errors.New("超时")

// listener WebTransport 监听器
type listener struct {
	transport       *transport         // WebTransport 传输层
	isStaticTLSConf bool               // 是否使用静态 TLS 配置
	reuseListener   quicreuse.Listener // QUIC 复用监听器

	server webtransport.Server // WebTransport 服务器

	ctx       context.Context    // 上下文
	ctxCancel context.CancelFunc // 取消函数

	serverClosed chan struct{} // 当 server.Serve 返回时关闭

	addr      net.Addr     // 监听地址
	multiaddr ma.Multiaddr // 多地址

	queue chan tpt.CapableConn // 连接队列
}

// 确保实现了 tpt.Listener 接口
var _ tpt.Listener = &listener{}

// newListener 创建新的监听器
// 参数:
//   - reuseListener: quicreuse.Listener QUIC 复用监听器
//   - t: *transport WebTransport 传输层
//   - isStaticTLSConf: bool 是否使用静态 TLS 配置
//
// 返回值:
//   - tpt.Listener 监听器接口
//   - error 错误信息
func newListener(reuseListener quicreuse.Listener, t *transport, isStaticTLSConf bool) (tpt.Listener, error) {
	localMultiaddr, err := toWebtransportMultiaddr(reuseListener.Addr())
	if err != nil {
		log.Debugf("转换本地地址失败: %s", err)
		return nil, err
	}

	ln := &listener{
		reuseListener:   reuseListener,
		transport:       t,
		isStaticTLSConf: isStaticTLSConf,
		queue:           make(chan tpt.CapableConn, queueLen),
		serverClosed:    make(chan struct{}),
		addr:            reuseListener.Addr(),
		multiaddr:       localMultiaddr,
		server: webtransport.Server{
			H3: http3.Server{
				ConnContext: func(ctx context.Context, c quic.Connection) context.Context {
					return context.WithValue(ctx, connKey{}, c)
				},
			},
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
	ln.ctx, ln.ctxCancel = context.WithCancel(context.Background())
	mux := http.NewServeMux()
	mux.HandleFunc(webtransportHTTPEndpoint, ln.httpHandler)
	ln.server.H3.Handler = mux
	go func() {
		defer close(ln.serverClosed)
		for {
			conn, err := ln.reuseListener.Accept(context.Background())
			if err != nil {
				log.Debugw("服务失败", "addr", ln.Addr(), "error", err)
				return
			}
			wrapped := wrapConn(ln.ctx, conn, t.handshakeTimeout)
			go ln.server.ServeQUICConn(wrapped)
		}
	}()
	return ln, nil
}

// httpHandler 处理 HTTP 请求
// 参数:
//   - w: http.ResponseWriter 响应写入器
//   - r: *http.Request HTTP 请求
func (l *listener) httpHandler(w http.ResponseWriter, r *http.Request) {
	typ, ok := r.URL.Query()["type"]
	if !ok || len(typ) != 1 || typ[0] != "noise" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	remoteMultiaddr, err := stringToWebtransportMultiaddr(r.RemoteAddr)
	if err != nil {
		// 这种情况不应该发生
		log.Debugf("转换远程地址失败", "remote", r.RemoteAddr, "error", err)
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if l.transport.gater != nil && !l.transport.gater.InterceptAccept(&connMultiaddrs{local: l.multiaddr, remote: remoteMultiaddr}) {
		w.WriteHeader(http.StatusForbidden)
		return
	}

	connScope, err := l.transport.rcmgr.OpenConnection(network.DirInbound, false, remoteMultiaddr)
	if err != nil {
		log.Debugw("资源管理器阻止了入站连接", "addr", r.RemoteAddr, "error", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	err = l.httpHandlerWithConnScope(w, r, connScope)
	if err != nil {
		connScope.Done()
	}
}

// httpHandlerWithConnScope 使用连接作用域处理 HTTP 请求
// 参数:
//   - w: http.ResponseWriter 响应写入器
//   - r: *http.Request HTTP 请求
//   - connScope: network.ConnManagementScope 连接管理作用域
//
// 返回值:
//   - error 错误信息
func (l *listener) httpHandlerWithConnScope(w http.ResponseWriter, r *http.Request, connScope network.ConnManagementScope) error {
	sess, err := l.server.Upgrade(w, r)
	if err != nil {
		log.Debugf("升级失败: %s", err)
		// TODO: 考虑使用合适的状态码
		w.WriteHeader(500)
		return err
	}
	ctx, cancel := context.WithTimeout(l.ctx, handshakeTimeout)
	sconn, err := l.handshake(ctx, sess)
	if err != nil {
		cancel()
		log.Debugf("握手失败: %s", err)
		sess.CloseWithError(1, "")
		return err
	}
	cancel()

	if l.transport.gater != nil && !l.transport.gater.InterceptSecured(network.DirInbound, sconn.RemotePeer(), sconn) {
		log.Debugf("gater 阻止了连接")
		// TODO: 是否可以使用特定错误码关闭
		sess.CloseWithError(errorCodeConnectionGating, "")
		return errors.New("gater 阻止了连接")
	}

	if err := connScope.SetPeer(sconn.RemotePeer()); err != nil {
		log.Debugf("资源管理器阻止了对等点的入站连接: %s", err)
		sess.CloseWithError(1, "")
		return err
	}

	connVal := r.Context().Value(connKey{})
	if connVal == nil {
		log.Errorf("上下文中缺少连接")
		sess.CloseWithError(1, "")
		return errors.New("无效的上下文")
	}
	nconn, ok := connVal.(*negotiatingConn)
	if !ok {
		log.Errorf("上下文中的连接类型错误: %T", nconn)
		sess.CloseWithError(1, "")
		return errors.New("无效的上下文")
	}
	qconn, err := nconn.Unwrap()
	if err != nil {
		log.Debugf("握手超时: %s", r.RemoteAddr)
		sess.CloseWithError(1, "")
		return err
	}

	conn := newConn(l.transport, sess, sconn, connScope, qconn)
	l.transport.addConn(sess, conn)
	select {
	case l.queue <- conn:
	default:
		log.Debugf("接受队列已满,丢弃入站连接: %s", err)
		conn.Close()
		return errors.New("接受队列已满")
	}

	return nil
}

// Accept 接受新的连接
// 返回值:
//   - tpt.CapableConn 可用的连接
//   - error 错误信息
func (l *listener) Accept() (tpt.CapableConn, error) {
	select {
	case <-l.ctx.Done():
		return nil, tpt.ErrListenerClosed
	case c := <-l.queue:
		return c, nil
	}
}

// handshake 执行握手过程
// 参数:
//   - ctx: context.Context 上下文
//   - sess: *webtransport.Session WebTransport 会话
//
// 返回值:
//   - *connSecurityMultiaddrs 连接安全和多地址组合
//   - error 错误信息
func (l *listener) handshake(ctx context.Context, sess *webtransport.Session) (*connSecurityMultiaddrs, error) {
	local, err := toWebtransportMultiaddr(sess.LocalAddr())
	if err != nil {
		log.Debugf("确定本地地址时出错: %s", err)
		return nil, err
	}
	remote, err := toWebtransportMultiaddr(sess.RemoteAddr())
	if err != nil {
		log.Debugf("确定远程地址时出错: %s", err)
		return nil, err
	}

	str, err := sess.AcceptStream(ctx)
	if err != nil {
		log.Debugf("接受流失败: %s", err)
		return nil, err
	}
	var earlyData [][]byte
	if !l.isStaticTLSConf {
		earlyData = l.transport.certManager.SerializedCertHashes()
	}

	n, err := l.transport.noise.WithSessionOptions(noise.EarlyData(
		nil,
		newEarlyDataSender(&pb.NoiseExtensions{WebtransportCerthashes: earlyData}),
	))
	if err != nil {
		log.Debugf("初始化 Noise 会话失败: %s", err)
		return nil, err
	}
	c, err := n.SecureInbound(ctx, &webtransportStream{Stream: str, wsess: sess}, "")
	if err != nil {
		log.Debugf("安全入站失败: %s", err)
		return nil, err
	}

	return &connSecurityMultiaddrs{
		ConnSecurity:   c,
		ConnMultiaddrs: &connMultiaddrs{local: local, remote: remote},
	}, nil
}

// Addr 获取监听地址
// 返回值:
//   - net.Addr 网络地址
func (l *listener) Addr() net.Addr {
	return l.addr
}

// Multiaddr 获取多地址
// 返回值:
//   - ma.Multiaddr 多地址
func (l *listener) Multiaddr() ma.Multiaddr {
	if l.transport.certManager == nil {
		return l.multiaddr
	}
	return l.multiaddr.Encapsulate(l.transport.certManager.AddrComponent())
}

// Close 关闭监听器
// 返回值:
//   - error 错误信息
func (l *listener) Close() error {
	l.ctxCancel()
	l.reuseListener.Close()
	err := l.server.Close()
	<-l.serverClosed
loop:
	for {
		select {
		case conn := <-l.queue:
			conn.Close()
		default:
			break loop
		}
	}
	return err
}
