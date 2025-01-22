package dep2pquic

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/connmgr"
	ic "github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/pnet"
	tpt "github.com/dep2p/go-dep2p/core/transport"
	p2ptls "github.com/dep2p/go-dep2p/p2p/security/tls"
	"github.com/dep2p/go-dep2p/p2p/transport/quicreuse"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	mafmt "github.com/dep2p/go-dep2p/multiformats/multiaddr/fmt"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	logging "github.com/dep2p/log"
	"github.com/quic-go/quic-go"
)

// ListenOrder 定义了 QUIC 传输的监听顺序
const ListenOrder = 1

// log 是 QUIC 传输的日志记录器
var log = logging.Logger("p2p-transport-quic")

// ErrHolePunching 表示尝试进行打洞但没有活跃的拨号
var ErrHolePunching = errors.New("尝试进行打洞;没有活跃的拨号")

// HolePunchTimeout 定义了打洞超时时间
var HolePunchTimeout = 5 * time.Second

// errorCodeConnectionGating 是连接门控的错误码(ASCII 码中的 GATE)
const errorCodeConnectionGating = 0x47415445 // GATE in ASCII

// transport 实现了 QUIC 连接的 tpt.Transport 接口
// 参数:
//   - privKey: ic.PrivKey 私钥
//   - localPeer: peer.ID 本地节点 ID
//   - identity: *p2ptls.Identity TLS 身份信息
//   - connManager: *quicreuse.ConnManager 连接管理器
//   - gater: connmgr.ConnectionGater 连接门控器
//   - rcmgr: network.ResourceManager 资源管理器
//   - holePunchingMx: sync.Mutex 打洞锁
//   - holePunching: map[holePunchKey]*activeHolePunch 活跃的打洞映射
//   - rndMx: sync.Mutex 随机数锁
//   - rnd: rand.Rand 随机数生成器
//   - connMx: sync.Mutex 连接锁
//   - conns: map[quic.Connection]*conn 连接映射
//   - listenersMu: sync.Mutex 监听器锁
//   - listeners: map[string][]*virtualListener 虚拟监听器映射
type transport struct {
	privKey     ic.PrivKey
	localPeer   peer.ID
	identity    *p2ptls.Identity
	connManager *quicreuse.ConnManager
	gater       connmgr.ConnectionGater
	rcmgr       network.ResourceManager

	holePunchingMx sync.Mutex
	holePunching   map[holePunchKey]*activeHolePunch

	rndMx sync.Mutex
	rnd   rand.Rand

	connMx sync.Mutex
	conns  map[quic.Connection]*conn

	listenersMu sync.Mutex
	// UDPAddr 字符串到虚拟监听器的映射
	listeners map[string][]*virtualListener
}

var _ tpt.Transport = &transport{}

// holePunchKey 定义了打洞的键
// 参数:
//   - addr: string 地址
//   - peer: peer.ID 对等节点 ID
type holePunchKey struct {
	addr string
	peer peer.ID
}

// activeHolePunch 表示活跃的打洞状态
// 参数:
//   - connCh: chan tpt.CapableConn 连接通道
//   - fulfilled: bool 是否已完成
type activeHolePunch struct {
	connCh    chan tpt.CapableConn
	fulfilled bool
}

// NewTransport 创建一个新的 QUIC 传输
// 参数:
//   - key: ic.PrivKey 私钥
//   - connManager: *quicreuse.ConnManager 连接管理器
//   - psk: pnet.PSK 预共享密钥
//   - gater: connmgr.ConnectionGater 连接门控器
//   - rcmgr: network.ResourceManager 资源管理器
//
// 返回值:
//   - tpt.Transport QUIC 传输实例
//   - error 错误信息
func NewTransport(key ic.PrivKey, connManager *quicreuse.ConnManager, psk pnet.PSK, gater connmgr.ConnectionGater, rcmgr network.ResourceManager) (tpt.Transport, error) {
	if len(psk) > 0 {
		log.Debugf("QUIC 目前不支持私有网络")
		return nil, errors.New("QUIC 目前不支持私有网络")
	}
	localPeer, err := peer.IDFromPrivateKey(key)
	if err != nil {
		return nil, err
	}
	identity, err := p2ptls.NewIdentity(key)
	if err != nil {
		return nil, err
	}

	if rcmgr == nil {
		rcmgr = &network.NullResourceManager{}
	}

	return &transport{
		privKey:      key,
		localPeer:    localPeer,
		identity:     identity,
		connManager:  connManager,
		gater:        gater,
		rcmgr:        rcmgr,
		conns:        make(map[quic.Connection]*conn),
		holePunching: make(map[holePunchKey]*activeHolePunch),
		rnd:          *rand.New(rand.NewSource(time.Now().UnixNano())),

		listeners: make(map[string][]*virtualListener),
	}, nil
}

// ListenOrder 返回监听顺序
// 返回值:
//   - int 监听顺序值
func (t *transport) ListenOrder() int {
	return ListenOrder
}

// Dial 拨号建立新的 QUIC 连接
// 参数:
//   - ctx: context.Context 上下文
//   - raddr: ma.Multiaddr 远程地址
//   - p: peer.ID 对等节点 ID
//
// 返回值:
//   - tpt.CapableConn 连接对象
//   - error 错误信息
func (t *transport) Dial(ctx context.Context, raddr ma.Multiaddr, p peer.ID) (_c tpt.CapableConn, _err error) {
	if ok, isClient, _ := network.GetSimultaneousConnect(ctx); ok && !isClient {
		return t.holePunch(ctx, raddr, p)
	}

	scope, err := t.rcmgr.OpenConnection(network.DirOutbound, false, raddr)
	if err != nil {
		log.Debugw("资源管理器阻止了出站连接", "peer", p, "addr", raddr, "error", err)
		return nil, err
	}

	c, err := t.dialWithScope(ctx, raddr, p, scope)
	if err != nil {
		scope.Done()
		return nil, err
	}
	return c, nil
}

// dialWithScope 在指定作用域内建立 QUIC 连接
// 参数:
//   - ctx: context.Context 上下文
//   - raddr: ma.Multiaddr 远程地址
//   - p: peer.ID 对等节点 ID
//   - scope: network.ConnManagementScope 连接管理作用域
//
// 返回值:
//   - tpt.CapableConn 连接对象
//   - error 错误信息
func (t *transport) dialWithScope(ctx context.Context, raddr ma.Multiaddr, p peer.ID, scope network.ConnManagementScope) (tpt.CapableConn, error) {
	if err := scope.SetPeer(p); err != nil {
		log.Debugw("资源管理器阻止了对指定节点的出站连接", "peer", p, "addr", raddr, "error", err)
		return nil, err
	}

	tlsConf, keyCh := t.identity.ConfigForPeer(p)
	ctx = quicreuse.WithAssociation(ctx, t)
	pconn, err := t.connManager.DialQUIC(ctx, raddr, tlsConf, t.allowWindowIncrease)
	if err != nil {
		return nil, err
	}

	// 此时应该已准备就绪,不要阻塞
	var remotePubKey ic.PubKey
	select {
	case remotePubKey = <-keyCh:
	default:
	}
	if remotePubKey == nil {
		pconn.CloseWithError(1, "")
		return nil, errors.New("p2p/transport/quic 错误: 预期的远程公钥未设置")
	}

	localMultiaddr, err := quicreuse.ToQuicMultiaddr(pconn.LocalAddr(), pconn.ConnectionState().Version)
	if err != nil {
		pconn.CloseWithError(1, "")
		return nil, err
	}
	c := &conn{
		quicConn:        pconn,
		transport:       t,
		scope:           scope,
		localPeer:       t.localPeer,
		localMultiaddr:  localMultiaddr,
		remotePubKey:    remotePubKey,
		remotePeerID:    p,
		remoteMultiaddr: raddr,
	}
	if t.gater != nil && !t.gater.InterceptSecured(network.DirOutbound, p, c) {
		pconn.CloseWithError(errorCodeConnectionGating, "连接被门控")
		return nil, fmt.Errorf("安全连接被门控")
	}
	t.addConn(pconn, c)
	return c, nil
}

// addConn 添加连接到连接映射
// 参数:
//   - conn: quic.Connection QUIC 连接
//   - c: *conn 连接对象
func (t *transport) addConn(conn quic.Connection, c *conn) {
	t.connMx.Lock()
	t.conns[conn] = c
	t.connMx.Unlock()
}

// removeConn 从连接映射中移除连接
// 参数:
//   - conn: quic.Connection 要移除的 QUIC 连接
func (t *transport) removeConn(conn quic.Connection) {
	t.connMx.Lock()
	delete(t.conns, conn)
	t.connMx.Unlock()
}

// holePunch 执行 NAT 打洞
// 参数:
//   - ctx: context.Context 上下文
//   - raddr: ma.Multiaddr 远程地址
//   - p: peer.ID 对等节点 ID
//
// 返回值:
//   - tpt.CapableConn 连接对象
//   - error 错误信息
func (t *transport) holePunch(ctx context.Context, raddr ma.Multiaddr, p peer.ID) (tpt.CapableConn, error) {
	network, saddr, err := manet.DialArgs(raddr)
	if err != nil {
		return nil, err
	}
	addr, err := net.ResolveUDPAddr(network, saddr)
	if err != nil {
		return nil, err
	}
	tr, err := t.connManager.TransportWithAssociationForDial(t, network, addr)
	if err != nil {
		return nil, err
	}
	defer tr.DecreaseCount()

	ctx, cancel := context.WithTimeout(ctx, HolePunchTimeout)
	defer cancel()

	key := holePunchKey{addr: addr.String(), peer: p}
	t.holePunchingMx.Lock()
	if _, ok := t.holePunching[key]; ok {
		t.holePunchingMx.Unlock()
		return nil, fmt.Errorf("已在为 %s 进行打洞", addr)
	}
	connCh := make(chan tpt.CapableConn, 1)
	t.holePunching[key] = &activeHolePunch{connCh: connCh}
	t.holePunchingMx.Unlock()

	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()

	payload := make([]byte, 64)
	var punchErr error
loop:
	for i := 0; ; i++ {
		t.rndMx.Lock()
		_, err := t.rnd.Read(payload)
		t.rndMx.Unlock()
		if err != nil {
			punchErr = err
			break
		}
		if _, err := tr.WriteTo(payload, addr); err != nil {
			punchErr = err
			break
		}

		maxSleep := 10 * (i + 1) * (i + 1) // 单位为毫秒
		if maxSleep > 200 {
			maxSleep = 200
		}
		d := 10*time.Millisecond + time.Duration(rand.Intn(maxSleep))*time.Millisecond
		if timer == nil {
			timer = time.NewTimer(d)
		} else {
			timer.Reset(d)
		}
		select {
		case c := <-connCh:
			t.holePunchingMx.Lock()
			delete(t.holePunching, key)
			t.holePunchingMx.Unlock()
			return c, nil
		case <-timer.C:
		case <-ctx.Done():
			punchErr = ErrHolePunching
			break loop
		}
	}
	// 仅在 punchErr != nil 时到达此处
	t.holePunchingMx.Lock()
	defer func() {
		delete(t.holePunching, key)
		t.holePunchingMx.Unlock()
	}()
	select {
	case c := <-t.holePunching[key].connCh:
		return c, nil
	default:
		return nil, punchErr
	}
}

// 不使用 mafmt.QUIC,因为我们不想拨号 DNS 地址。仅使用 /ip{4,6}/udp/quic-v1
var dialMatcher = mafmt.And(mafmt.IP, mafmt.Base(ma.P_UDP), mafmt.Base(ma.P_QUIC_V1))

// CanDial 判断是否可以拨号到指定地址
// 参数:
//   - addr: ma.Multiaddr 目标地址
//
// 返回值:
//   - bool 是否可以拨号
func (t *transport) CanDial(addr ma.Multiaddr) bool {
	return dialMatcher.Matches(addr)
}

// Listen 在指定多地址上监听新的 QUIC 连接
// 参数:
//   - addr: ma.Multiaddr 监听地址
//
// 返回值:
//   - tpt.Listener 监听器
//   - error 错误信息
func (t *transport) Listen(addr ma.Multiaddr) (tpt.Listener, error) {
	var tlsConf tls.Config
	tlsConf.GetConfigForClient = func(_ *tls.ClientHelloInfo) (*tls.Config, error) {
		// 返回用于验证对等节点证书链的 tls.Config
		// 注意:由于我们无法将传入的 QUIC 连接与此处计算的对等节点 ID 关联,我们实际上不会从密钥通道接收对等节点的公钥
		conf, _ := t.identity.ConfigForPeer("")
		return conf, nil
	}
	tlsConf.NextProtos = []string{"dep2p"}
	udpAddr, version, err := quicreuse.FromQuicMultiaddr(addr)
	if err != nil {
		return nil, err
	}

	t.listenersMu.Lock()
	defer t.listenersMu.Unlock()
	listeners := t.listeners[udpAddr.String()]
	var underlyingListener *listener
	var acceptRunner *acceptLoopRunner
	if len(listeners) != 0 {
		// 已有底层监听器,使用它
		underlyingListener = listeners[0].listener
		acceptRunner = listeners[0].acceptRunnner
		// 确保底层监听器支持指定的 QUIC 版本
		if _, ok := underlyingListener.localMultiaddrs[version]; !ok {
			return nil, fmt.Errorf("无法监听 QUIC 版本 %v,底层监听器不支持", version)
		}
	} else {
		ln, err := t.connManager.ListenQUICAndAssociate(t, addr, &tlsConf, t.allowWindowIncrease)
		if err != nil {
			return nil, err
		}
		l, err := newListener(ln, t, t.localPeer, t.privKey, t.rcmgr)
		if err != nil {
			_ = ln.Close()
			return nil, err
		}
		underlyingListener = &l

		acceptRunner = &acceptLoopRunner{
			acceptSem: make(chan struct{}, 1),
			muxer:     make(map[quic.Version]chan acceptVal),
		}
	}

	l := &virtualListener{
		listener:      underlyingListener,
		version:       version,
		udpAddr:       udpAddr.String(),
		t:             t,
		acceptRunnner: acceptRunner,
		acceptChan:    acceptRunner.AcceptForVersion(version),
	}

	listeners = append(listeners, l)
	t.listeners[udpAddr.String()] = listeners

	return l, nil
}

// allowWindowIncrease 判断是否允许增加窗口大小
// 参数:
//   - conn: quic.Connection QUIC 连接
//   - size: uint64 窗口大小
//
// 返回值:
//   - bool 是否允许增加
func (t *transport) allowWindowIncrease(conn quic.Connection, size uint64) bool {
	// 如果 QUIC 连接在我们将其插入连接映射之前尝试增加窗口(这是在拨号/接受之后立即进行的),我们就无法计算该内存。这种情况应该非常罕见。
	// 阻止此尝试。连接可以稍后请求更多内存。
	t.connMx.Lock()
	c, ok := t.conns[conn]
	t.connMx.Unlock()
	if !ok {
		return false
	}
	return c.allowWindowIncrease(size)
}

// Proxy 返回此传输是否为代理
// 返回值:
//   - bool 是否为代理
func (t *transport) Proxy() bool {
	return false
}

// Protocols 返回此传输处理的协议集
// 返回值:
//   - []int 协议 ID 列表
func (t *transport) Protocols() []int {
	return t.connManager.Protocols()
}

// String 返回传输的字符串表示
// 返回值:
//   - string 传输名称
func (t *transport) String() string {
	return "QUIC"
}

// Close 关闭传输
// 返回值:
//   - error 错误信息
func (t *transport) Close() error {
	return nil
}

// CloseVirtualListener 关闭虚拟监听器
// 参数:
//   - l: *virtualListener 要关闭的虚拟监听器
//
// 返回值:
//   - error 错误信息
func (t *transport) CloseVirtualListener(l *virtualListener) error {
	t.listenersMu.Lock()
	defer t.listenersMu.Unlock()

	var err error
	listeners := t.listeners[l.udpAddr]
	if len(listeners) == 1 {
		// 这是此处的最后一个虚拟监听器,可以关闭底层监听器
		err = l.listener.Close()
		delete(t.listeners, l.udpAddr)
		return err
	}

	for i := 0; i < len(listeners); i++ {
		// 交换移除
		if l == listeners[i] {
			listeners[i] = listeners[len(listeners)-1]
			listeners = listeners[:len(listeners)-1]
			t.listeners[l.udpAddr] = listeners
			break
		}
	}

	return nil
}
