package dep2pwebtransport

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/core/connmgr"
	ic "github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/pnet"
	tpt "github.com/dep2p/go-dep2p/core/transport"
	"github.com/dep2p/go-dep2p/p2p/security/noise"
	"github.com/dep2p/go-dep2p/p2p/security/noise/pb"
	"github.com/dep2p/go-dep2p/p2p/transport/quicreuse"

	"github.com/benbjohnson/clock"
	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	"github.com/dep2p/go-dep2p/multiformats/multihash"
	logging "github.com/dep2p/log"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/quic-go/webtransport-go"
)

// log 是 webtransport 包的日志记录器
var log = logging.Logger("p2p-transport/webtransport")

// webtransportHTTPEndpoint 是 WebTransport 的 HTTP 端点路径
const webtransportHTTPEndpoint = "/.well-known/dep2p-webtransport"

// errorCodeConnectionGating 是连接网关错误码,ASCII 码为 "GATE"
const errorCodeConnectionGating = 0x47415445

// certValidity 是证书的有效期限
const certValidity = 14 * 24 * time.Hour

// Option 是配置 transport 的函数类型
type Option func(*transport) error

// WithClock 设置自定义时钟
// 参数:
//   - cl: clock.Clock 自定义时钟对象
//
// 返回值:
//   - Option 返回一个配置函数
func WithClock(cl clock.Clock) Option {
	return func(t *transport) error {
		t.clock = cl
		return nil
	}
}

// WithTLSClientConfig 设置自定义 TLS 客户端配置
// 参数:
//   - c: *tls.Config TLS 配置对象
//
// 返回值:
//   - Option 返回一个配置函数
//
// 注意:
//   - 此选项最常用于设置自定义的 RootCAs 证书池
//   - 当拨号包含 /certhash 组件的多地址时,会设置 InsecureSkipVerify 并覆盖 VerifyPeerCertificate 回调
func WithTLSClientConfig(c *tls.Config) Option {
	return func(t *transport) error {
		t.tlsClientConf = c
		return nil
	}
}

// WithHandshakeTimeout 设置握手超时时间
// 参数:
//   - d: time.Duration 超时时间
//
// 返回值:
//   - Option 返回一个配置函数
func WithHandshakeTimeout(d time.Duration) Option {
	return func(t *transport) error {
		t.handshakeTimeout = d
		return nil
	}
}

// transport 实现了 WebTransport 传输层
type transport struct {
	privKey ic.PrivKey  // 私钥
	pid     peer.ID     // 节点 ID
	clock   clock.Clock // 时钟

	connManager *quicreuse.ConnManager  // QUIC 连接管理器
	rcmgr       network.ResourceManager // 资源管理器
	gater       connmgr.ConnectionGater // 连接网关

	listenOnce     sync.Once    // 确保只监听一次
	listenOnceErr  error        // 监听时的错误
	certManager    *certManager // 证书管理器
	hasCertManager atomic.Bool  // 标记证书管理器是否已初始化
	staticTLSConf  *tls.Config  // 静态 TLS 配置
	tlsClientConf  *tls.Config  // 客户端 TLS 配置

	noise *noise.Transport // Noise 协议传输层

	connMx           sync.Mutex                         // 连接映射锁
	conns            map[quic.ConnectionTracingID]*conn // 使用 quic-go 的 ConnectionTracingKey 作为映射键
	handshakeTimeout time.Duration                      // 握手超时时间
}

// 确保 transport 实现了这些接口
var _ tpt.Transport = &transport{}
var _ tpt.Resolver = &transport{}
var _ io.Closer = &transport{}

// New 创建新的 WebTransport 传输层
// 参数:
//   - key: ic.PrivKey 私钥
//   - psk: pnet.PSK 预共享密钥
//   - connManager: *quicreuse.ConnManager QUIC 连接管理器
//   - gater: connmgr.ConnectionGater 连接网关
//   - rcmgr: network.ResourceManager 资源管理器
//   - opts: ...Option 配置选项
//
// 返回值:
//   - tpt.Transport WebTransport 传输层
//   - error 错误信息
func New(key ic.PrivKey, psk pnet.PSK, connManager *quicreuse.ConnManager, gater connmgr.ConnectionGater, rcmgr network.ResourceManager, opts ...Option) (tpt.Transport, error) {
	if len(psk) > 0 {
		log.Debugf("WebTransport 暂不支持私有网络")
		return nil, errors.New("WebTransport 暂不支持私有网络")
	}
	if rcmgr == nil {
		rcmgr = &network.NullResourceManager{}
	}
	id, err := peer.IDFromPrivateKey(key)
	if err != nil {
		log.Debugf("从私钥创建 peer.ID 失败: %s", err)
		return nil, err
	}
	t := &transport{
		pid:              id,
		privKey:          key,
		rcmgr:            rcmgr,
		gater:            gater,
		clock:            clock.New(),
		connManager:      connManager,
		conns:            map[quic.ConnectionTracingID]*conn{},
		handshakeTimeout: handshakeTimeout,
	}
	for _, opt := range opts {
		if err := opt(t); err != nil {
			log.Debugf("配置选项失败: %s", err)
			return nil, err
		}
	}
	n, err := noise.New(noise.ID, key, nil)
	if err != nil {
		log.Debugf("创建 Noise 传输层失败: %s", err)
		return nil, err
	}
	t.noise = n
	return t, nil
}

// Dial 建立到远程节点的连接
// 参数:
//   - ctx: context.Context 上下文
//   - raddr: ma.Multiaddr 远程地址
//   - p: peer.ID 远程节点 ID
//
// 返回值:
//   - tpt.CapableConn 建立的连接
//   - error 错误信息
func (t *transport) Dial(ctx context.Context, raddr ma.Multiaddr, p peer.ID) (tpt.CapableConn, error) {
	// 打开出站连接资源
	scope, err := t.rcmgr.OpenConnection(network.DirOutbound, false, raddr)
	if err != nil {
		log.Debugf("资源管理器阻止了出站连接: %s", err)
		return nil, err
	}

	// 使用资源范围建立连接
	c, err := t.dialWithScope(ctx, raddr, p, scope)
	if err != nil {
		scope.Done()
		log.Debugf("建立连接失败: %s", err)
		return nil, err
	}

	return c, nil
}

// dialWithScope 在指定资源范围内建立连接
// 参数:
//   - ctx: context.Context 上下文
//   - raddr: ma.Multiaddr 远程地址
//   - p: peer.ID 远程节点 ID
//   - scope: network.ConnManagementScope 连接管理范围
//
// 返回值:
//   - tpt.CapableConn 建立的连接
//   - error 错误信息
func (t *transport) dialWithScope(ctx context.Context, raddr ma.Multiaddr, p peer.ID, scope network.ConnManagementScope) (tpt.CapableConn, error) {
	// 解析拨号参数
	_, addr, err := manet.DialArgs(raddr)
	if err != nil {
		log.Debugf("解析拨号参数失败: %s", err)
		return nil, err
	}
	// 构造 WebTransport URL
	url := fmt.Sprintf("https://%s%s?type=noise", addr, webtransportHTTPEndpoint)
	// 提取证书哈希
	certHashes, err := extractCertHashes(raddr)
	if err != nil {
		log.Debugf("提取证书哈希失败: %s", err)
		return nil, err
	}

	// 检查证书哈希是否存在
	if len(certHashes) == 0 {
		log.Debugf("无法在没有证书哈希的情况下建立 WebTransport 连接")
		return nil, errors.New("无法在没有证书哈希的情况下建立 WebTransport 连接")
	}

	// 提取 SNI
	sni, _ := extractSNI(raddr)

	// 设置对等节点
	if err := scope.SetPeer(p); err != nil {
		log.Debugf("资源管理器阻止了对等节点的出站连接: %s", err)
		return nil, err
	}

	// 分割多地址
	maddr, _ := ma.SplitFunc(raddr, func(c ma.Component) bool { return c.Protocol().Code == ma.P_WEBTRANSPORT })
	// 建立 WebTransport 会话
	sess, qconn, err := t.dial(ctx, maddr, url, sni, certHashes)
	if err != nil {
		log.Debugf("建立 WebTransport 会话失败: %s", err)
		return nil, err
	}
	// 升级连接
	sconn, err := t.upgrade(ctx, sess, p, certHashes)
	if err != nil {
		log.Debugf("升级连接失败: %s", err)
		sess.CloseWithError(1, "")
		qconn.CloseWithError(1, "")
		return nil, err
	}
	// 检查连接网关
	if t.gater != nil && !t.gater.InterceptSecured(network.DirOutbound, p, sconn) {
		log.Debugf("安全连接被网关拦截")
		sess.CloseWithError(errorCodeConnectionGating, "")
		qconn.CloseWithError(errorCodeConnectionGating, "")
		return nil, fmt.Errorf("安全连接被网关拦截")
	}
	// 创建新连接
	conn := newConn(t, sess, sconn, scope, qconn)
	t.addConn(sess, conn)
	return conn, nil
}

// dial 建立 WebTransport 会话
// 参数:
//   - ctx: context.Context 上下文
//   - addr: ma.Multiaddr 地址
//   - url: string WebTransport URL
//   - sni: string SNI 服务器名称
//   - certHashes: []multihash.DecodedMultihash 证书哈希列表
//
// 返回值:
//   - *webtransport.Session WebTransport 会话
//   - quic.Connection QUIC 连接
//   - error 错误信息
func (t *transport) dial(ctx context.Context, addr ma.Multiaddr, url, sni string, certHashes []multihash.DecodedMultihash) (*webtransport.Session, quic.Connection, error) {
	// 克隆或创建 TLS 配置
	var tlsConf *tls.Config
	if t.tlsClientConf != nil {
		tlsConf = t.tlsClientConf.Clone()
	} else {
		tlsConf = &tls.Config{}
	}
	tlsConf.NextProtos = append(tlsConf.NextProtos, http3.NextProtoH3)

	// 设置 SNI
	if sni != "" {
		tlsConf.ServerName = sni
	}

	// 配置证书验证
	if len(certHashes) > 0 {
		// 这不是不安全的。我们自己验证证书。
		// 参见 https://www.w3.org/TR/webtransport/#certificate-hashes
		tlsConf.InsecureSkipVerify = true
		tlsConf.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			return verifyRawCerts(rawCerts, certHashes)
		}
	}
	// 设置关联
	ctx = quicreuse.WithAssociation(ctx, t)
	// 建立 QUIC 连接
	conn, err := t.connManager.DialQUIC(ctx, addr, tlsConf, t.allowWindowIncrease)
	if err != nil {
		log.Debugf("建立 QUIC 连接失败: %s", err)
		return nil, nil, err
	}
	// 配置 WebTransport 拨号器
	dialer := webtransport.Dialer{
		DialAddr: func(ctx context.Context, addr string, tlsCfg *tls.Config, cfg *quic.Config) (quic.EarlyConnection, error) {
			return conn.(quic.EarlyConnection), nil
		},
		QUICConfig: t.connManager.ClientConfig().Clone(),
	}
	// 建立 WebTransport 会话
	rsp, sess, err := dialer.Dial(ctx, url, nil)
	if err != nil {
		conn.CloseWithError(1, "")
		log.Debugf("建立 WebTransport 会话失败: %s", err)
		return nil, nil, err
	}
	// 检查响应状态码
	if rsp.StatusCode < 200 || rsp.StatusCode > 299 {
		conn.CloseWithError(1, "")
		log.Debugf("无效的响应状态码: %d", rsp.StatusCode)
		return nil, nil, fmt.Errorf("无效的响应状态码: %d", rsp.StatusCode)
	}
	return sess, conn, err
}

// upgrade 升级 WebTransport 会话为安全连接
// 参数:
//   - ctx: context.Context 上下文
//   - sess: *webtransport.Session WebTransport 会话
//   - p: peer.ID 对等节点 ID
//   - certHashes: []multihash.DecodedMultihash 证书哈希列表
//
// 返回值:
//   - *connSecurityMultiaddrs 安全连接
//   - error 错误信息
func (t *transport) upgrade(ctx context.Context, sess *webtransport.Session, p peer.ID, certHashes []multihash.DecodedMultihash) (*connSecurityMultiaddrs, error) {
	// 获取本地地址
	local, err := toWebtransportMultiaddr(sess.LocalAddr())
	if err != nil {
		log.Debugf("确定本地地址时出错: %s", err)
		return nil, err
	}
	// 获取远程地址
	remote, err := toWebtransportMultiaddr(sess.RemoteAddr())
	if err != nil {
		log.Debugf("确定远程地址时出错: %s", err)
		return nil, err
	}

	// 打开流
	str, err := sess.OpenStreamSync(ctx)
	if err != nil {
		log.Debugf("打开流失败: %s", err)
		return nil, err
	}
	defer str.Close()

	// 运行 Noise 握手(使用早期数据)并从服务器获取所有证书哈希
	// 我们将验证用于拨号的证书哈希是从服务器接收到的证书哈希的子集
	var verified bool
	n, err := t.noise.WithSessionOptions(noise.EarlyData(newEarlyDataReceiver(func(b *pb.NoiseExtensions) error {
		decodedCertHashes, err := decodeCertHashesFromProtobuf(b.WebtransportCerthashes)
		if err != nil {
			log.Debugf("解码证书哈希失败: %s", err)
			return err
		}
		for _, sent := range certHashes {
			var found bool
			for _, rcvd := range decodedCertHashes {
				if sent.Code == rcvd.Code && bytes.Equal(sent.Digest, rcvd.Digest) {
					found = true
					break
				}
			}
			if !found {
				log.Debugf("缺少证书哈希: %v", sent)
				return fmt.Errorf("缺少证书哈希: %v", sent)
			}
		}
		verified = true
		return nil
	}), nil))
	if err != nil {
		log.Debugf("创建 Noise 传输时失败: %s", err)
		return nil, err
	}
	// 建立安全出站连接
	c, err := n.SecureOutbound(ctx, &webtransportStream{Stream: str, wsess: sess}, p)
	if err != nil {
		log.Debugf("创建安全出站连接失败: %s", err)
		return nil, err
	}
	defer c.Close()
	// Noise 握手应该保证我们的验证回调被调用
	// 以防万一再次检查
	if !verified {
		log.Debugf("未验证")
		return nil, errors.New("未验证")
	}
	return &connSecurityMultiaddrs{
		ConnSecurity:   c,
		ConnMultiaddrs: &connMultiaddrs{local: local, remote: remote},
	}, nil
}

// decodeCertHashesFromProtobuf 从 protobuf 解码证书哈希
// 参数:
//   - b: [][]byte 要解码的字节数组
//
// 返回值:
//   - []multihash.DecodedMultihash 解码后的证书哈希列表
//   - error 错误信息
func decodeCertHashesFromProtobuf(b [][]byte) ([]multihash.DecodedMultihash, error) {
	hashes := make([]multihash.DecodedMultihash, 0, len(b))
	for _, h := range b {
		dh, err := multihash.Decode(h)
		if err != nil {
			log.Debugf("解码哈希失败: %s", err)
			return nil, err
		}
		hashes = append(hashes, *dh)
	}
	return hashes, nil
}

// CanDial 检查是否可以拨号到指定地址
// 参数:
//   - addr: ma.Multiaddr 要检查的地址
//
// 返回值:
//   - bool 是否可以拨号
func (t *transport) CanDial(addr ma.Multiaddr) bool {
	ok, _ := IsWebtransportMultiaddr(addr)
	return ok
}

// Listen 在指定地址上监听连接
// 参数:
//   - laddr: ma.Multiaddr 要监听的地址
//
// 返回值:
//   - tpt.Listener 监听器
//   - error 错误信息
func (t *transport) Listen(laddr ma.Multiaddr) (tpt.Listener, error) {
	isWebTransport, certhashCount := IsWebtransportMultiaddr(laddr)
	if !isWebTransport {
		log.Debugf("无法在非 WebTransport 地址上监听: %s", laddr)
		return nil, fmt.Errorf("无法在非 WebTransport 地址上监听: %s", laddr)
	}
	if certhashCount > 0 {
		log.Debugf("无法在指定证书哈希的非 WebTransport 地址上监听: %s", laddr)
		return nil, fmt.Errorf("无法在指定证书哈希的非 WebTransport 地址上监听: %s", laddr)
	}
	if t.staticTLSConf == nil {
		t.listenOnce.Do(func() {
			t.certManager, t.listenOnceErr = newCertManager(t.privKey, t.clock)
			t.hasCertManager.Store(true)
		})
		if t.listenOnceErr != nil {
			log.Debugf("监听失败: %s", t.listenOnceErr)
			return nil, t.listenOnceErr
		}
	} else {
		log.Debugf("WebTransport 不支持静态 TLS 配置")
		return nil, errors.New("WebTransport 不支持静态 TLS 配置")
	}
	tlsConf := t.staticTLSConf.Clone()
	if tlsConf == nil {
		tlsConf = &tls.Config{GetConfigForClient: func(*tls.ClientHelloInfo) (*tls.Config, error) {
			return t.certManager.GetConfig(), nil
		}}
	}
	tlsConf.NextProtos = append(tlsConf.NextProtos, http3.NextProtoH3)

	ln, err := t.connManager.ListenQUICAndAssociate(t, laddr, tlsConf, t.allowWindowIncrease)
	if err != nil {
		log.Debugf("监听失败: %s", err)
		return nil, err
	}
	return newListener(ln, t, t.staticTLSConf != nil)
}

// Protocols 返回支持的协议列表
// 返回值:
//   - []int 协议代码列表
func (t *transport) Protocols() []int {
	return []int{ma.P_WEBTRANSPORT}
}

// Proxy 返回是否为代理传输
// 返回值:
//   - bool 是否为代理
func (t *transport) Proxy() bool {
	return false
}

// Close 关闭传输层
// 返回值:
//   - error 错误信息
func (t *transport) Close() error {
	t.listenOnce.Do(func() {})
	if t.certManager != nil {
		log.Debugf("关闭证书管理器: %s", t.certManager.Close())
		return t.certManager.Close()
	}
	return nil
}

// allowWindowIncrease 检查是否允许增加窗口大小
// 参数:
//   - conn: quic.Connection QUIC 连接
//   - size: uint64 窗口大小
//
// 返回值:
//   - bool 是否允许增加
func (t *transport) allowWindowIncrease(conn quic.Connection, size uint64) bool {
	t.connMx.Lock()
	defer t.connMx.Unlock()

	c, ok := t.conns[conn.Context().Value(quic.ConnectionTracingKey).(quic.ConnectionTracingID)]
	if !ok {
		return false
	}
	return c.allowWindowIncrease(size)
}

// addConn 添加连接
// 参数:
//   - sess: *webtransport.Session WebTransport 会话
//   - c: *conn 连接对象
func (t *transport) addConn(sess *webtransport.Session, c *conn) {
	t.connMx.Lock()
	t.conns[sess.Context().Value(quic.ConnectionTracingKey).(quic.ConnectionTracingID)] = c
	t.connMx.Unlock()
}

// removeConn 移除连接
// 参数:
//   - sess: *webtransport.Session WebTransport 会话
func (t *transport) removeConn(sess *webtransport.Session) {
	t.connMx.Lock()
	delete(t.conns, sess.Context().Value(quic.ConnectionTracingKey).(quic.ConnectionTracingID))
	t.connMx.Unlock()
}

// extractSNI 从多地址中提取 SNI
// 如果多地址中有 SNI 组件,则返回该值且 foundSniComponent 为 true
// 如果没有 SNI 组件但有类 DNS 组件,则返回该值且 foundSniComponent 为 false(因为没有找到实际的 SNI 组件)
// 参数:
//   - maddr: ma.Multiaddr 多地址
//
// 返回值:
//   - sni: string SNI 值
//   - foundSniComponent: bool 是否找到 SNI 组件
func extractSNI(maddr ma.Multiaddr) (sni string, foundSniComponent bool) {
	ma.ForEach(maddr, func(c ma.Component) bool {
		switch c.Protocol().Code {
		case ma.P_SNI:
			sni = c.Value()
			foundSniComponent = true
			return false
		case ma.P_DNS, ma.P_DNS4, ma.P_DNS6, ma.P_DNSADDR:
			sni = c.Value()
			// 继续查找是否有 sni 组件
			return true
		}
		return true
	})
	return sni, foundSniComponent
}

// Resolve 实现 transport.Resolver 接口
// 参数:
//   - _: context.Context 上下文(未使用)
//   - maddr: ma.Multiaddr 要解析的多地址
//
// 返回值:
//   - []ma.Multiaddr 解析后的多地址列表
//   - error 错误信息
func (t *transport) Resolve(_ context.Context, maddr ma.Multiaddr) ([]ma.Multiaddr, error) {
	sni, foundSniComponent := extractSNI(maddr)

	if foundSniComponent || sni == "" {
		// 多地址已经有 SNI 字段,可以继续使用。或者没有任何 SNI 相关内容
		return []ma.Multiaddr{maddr}, nil
	}

	beforeQuicMA, afterIncludingQuicMA := ma.SplitFunc(maddr, func(c ma.Component) bool {
		return c.Protocol().Code == ma.P_QUIC_V1
	})
	quicComponent, afterQuicMA := ma.SplitFirst(afterIncludingQuicMA)
	sniComponent, err := ma.NewComponent(ma.ProtocolWithCode(ma.P_SNI).Name, sni)
	if err != nil {
		log.Debugf("创建 SNI 组件失败: %s", err)
		return nil, err
	}
	return []ma.Multiaddr{beforeQuicMA.Encapsulate(quicComponent).Encapsulate(sniComponent).Encapsulate(afterQuicMA)}, nil
}

// AddCertHashes 向多地址添加当前证书哈希
// 如果在 Listen 之前调用,则为空操作
// 参数:
//   - m: ma.Multiaddr 要添加证书哈希的多地址
//
// 返回值:
//   - ma.Multiaddr 添加了证书哈希的多地址
//   - bool 是否成功添加
func (t *transport) AddCertHashes(m ma.Multiaddr) (ma.Multiaddr, bool) {
	if !t.hasCertManager.Load() {
		return m, false
	}
	return m.Encapsulate(t.certManager.AddrComponent()), true
}
