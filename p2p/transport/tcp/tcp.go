package tcp

import (
	"context"
	"errors"
	"net"
	"os"
	"runtime"
	"syscall"
	"time"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/transport"
	"github.com/dep2p/go-dep2p/p2p/net/reuseport"
	"github.com/dep2p/go-dep2p/p2p/transport/tcpreuse"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	mafmt "github.com/dep2p/go-dep2p/multiformats/multiaddr/fmt"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	logging "github.com/dep2p/log"
)

// 默认连接超时时间
const defaultConnectTimeout = 5 * time.Second

// 日志记录器
var log = logging.Logger("p2p-transport-tcp")

// TCP keepalive 探测周期
const keepAlivePeriod = 30 * time.Second

// canKeepAlive 定义了支持 keepalive 的接口
// 方法:
//   - SetKeepAlive: 设置是否启用 keepalive
//   - SetKeepAlivePeriod: 设置 keepalive 探测周期
type canKeepAlive interface {
	SetKeepAlive(bool) error
	SetKeepAlivePeriod(time.Duration) error
}

// 确保 net.TCPConn 实现了 canKeepAlive 接口
var _ canKeepAlive = &net.TCPConn{}

// 已弃用: 请使用 tcpreuse.ReuseportIsAvailable
var ReuseportIsAvailable = tcpreuse.ReuseportIsAvailable

// tryKeepAlive 尝试为连接设置 TCP keepalive
// 参数:
//   - conn: net.Conn 网络连接对象
//   - keepAlive: bool 是否启用 keepalive
func tryKeepAlive(conn net.Conn, keepAlive bool) {
	// 尝试类型转换为支持 keepalive 的连接
	keepAliveConn, ok := conn.(canKeepAlive)
	if !ok {
		log.Errorf("无法设置 TCP keepalive")
		return
	}

	// 设置 keepalive
	if err := keepAliveConn.SetKeepAlive(keepAlive); err != nil {
		// 在 Darwin 系统上有时会收到"无效参数"错误
		// 这可能是由于连接已关闭导致,但在 Linux 上无法重现
		//
		// 对于无效参数错误,我们只记录 debug 日志
		if errors.Is(err, os.ErrInvalid) || errors.Is(err, syscall.EINVAL) {
			log.Debugf("启用 TCP keepalive 失败: %s", err)
		} else {
			log.Debugf("启用 TCP keepalive 失败: %s", err)
		}
		return
	}

	// 在非 OpenBSD 系统上设置 keepalive 周期
	if runtime.GOOS != "openbsd" {
		if err := keepAliveConn.SetKeepAlivePeriod(keepAlivePeriod); err != nil {
			log.Debugf("设置 keepalive 周期失败: %s", err)
		}
	}
}

// tryLinger 尝试为连接设置 linger 选项
// 参数:
//   - conn: net.Conn 网络连接对象
//   - sec: int linger 超时时间(秒)
func tryLinger(conn net.Conn, sec int) {
	// 定义支持 linger 的接口
	type canLinger interface {
		SetLinger(int) error
	}

	// 尝试设置 linger
	if lingerConn, ok := conn.(canLinger); ok {
		_ = lingerConn.SetLinger(sec)
	}
}

// tcpListener TCP 监听器结构体
type tcpListener struct {
	manet.Listener
	sec int // linger 超时时间
}

// Accept 接受新的连接
// 返回值:
//   - manet.Conn 新接受的连接
//   - error 错误信息
func (ll *tcpListener) Accept() (manet.Conn, error) {
	// 接受新连接
	c, err := ll.Listener.Accept()
	if err != nil {
		log.Debugf("接受新连接时出错: %s", err)
		return nil, err
	}

	// 设置 linger 和 keepalive
	tryLinger(c, ll.sec)
	tryKeepAlive(c, true)
	// 这里不调用资源管理器的 OpenConnection,因为 manet.Conn 不允许我们保存 scope
	// 由调用者(通常是 p2p/net/upgrader)负责调用资源管理器
	return c, nil
}

// Option 定义 TcpTransport 的配置选项函数类型
type Option func(*TcpTransport) error

// DisableReuseport 禁用端口重用选项
// 返回值:
//   - Option 配置选项函数
func DisableReuseport() Option {
	return func(tr *TcpTransport) error {
		tr.disableReuseport = true
		return nil
	}
}

// WithConnectionTimeout 设置连接超时选项
// 参数:
//   - d: time.Duration 超时时间
//
// 返回值:
//   - Option 配置选项函数
func WithConnectionTimeout(d time.Duration) Option {
	return func(tr *TcpTransport) error {
		tr.connectTimeout = d
		return nil
	}
}

// WithMetrics 启用指标收集选项
// 返回值:
//   - Option 配置选项函数
func WithMetrics() Option {
	return func(tr *TcpTransport) error {
		tr.enableMetrics = true
		return nil
	}
}

// TcpTransport TCP 传输层结构体
type TcpTransport struct {
	// 用于将不安全流连接升级为安全多路复用连接的升级器
	upgrader transport.Upgrader

	disableReuseport bool // 是否禁用端口重用
	enableMetrics    bool // 是否启用指标收集

	// 在多个传输层之间共享和多路复用 TCP 监听器
	sharedTcp *tcpreuse.ConnMgr

	// TCP 连接超时时间
	connectTimeout time.Duration

	rcmgr network.ResourceManager // 资源管理器

	reuse reuseport.Transport // 端口重用传输层

	metricsCollector *aggregatingCollector // 指标收集器
}

// 确保 TcpTransport 实现了相关接口
var _ transport.Transport = &TcpTransport{}
var _ transport.DialUpdater = &TcpTransport{}

// NewTCPTransport 创建一个新的 TCP 传输层对象
// 参数:
//   - upgrader: transport.Upgrader 连接升级器
//   - rcmgr: network.ResourceManager 资源管理器
//   - sharedTCP: *tcpreuse.ConnMgr TCP 连接管理器
//   - opts: ...Option 配置选项
//
// 返回值:
//   - *TcpTransport TCP 传输层对象
//   - error 错误信息
func NewTCPTransport(upgrader transport.Upgrader, rcmgr network.ResourceManager, sharedTCP *tcpreuse.ConnMgr, opts ...Option) (*TcpTransport, error) {
	// 如果未提供资源管理器,使用空实现
	if rcmgr == nil {
		rcmgr = &network.NullResourceManager{}
	}

	// 创建传输层对象
	tr := &TcpTransport{
		upgrader:       upgrader,
		connectTimeout: defaultConnectTimeout, // 可通过 WithConnectionTimeout 选项修改
		rcmgr:          rcmgr,
		sharedTcp:      sharedTCP,
	}

	// 应用配置选项
	for _, o := range opts {
		if err := o(tr); err != nil {
			log.Debugf("应用配置选项时出错: %s", err)
			return nil, err
		}
	}
	return tr, nil
}

// 用于匹配可拨号地址的格式
var dialMatcher = mafmt.And(mafmt.IP, mafmt.Base(ma.P_TCP))

// CanDial 判断是否可以拨号给定的多地址
// 参数:
//   - addr: ma.Multiaddr 多地址
//
// 返回值:
//   - bool 是否可以拨号
func (t *TcpTransport) CanDial(addr ma.Multiaddr) bool {
	return dialMatcher.Matches(addr)
}

// maDial 拨号到指定的多地址
// 参数:
//   - ctx: context.Context 上下文
//   - raddr: ma.Multiaddr 目标多地址
//
// 返回值:
//   - manet.Conn 建立的连接
//   - error 错误信息
func (t *TcpTransport) maDial(ctx context.Context, raddr ma.Multiaddr) (manet.Conn, error) {
	// 如果设置了超时时间则应用超时
	if t.connectTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, t.connectTimeout)
		defer cancel()
	}

	// 使用共享 TCP 管理器拨号
	if t.sharedTcp != nil {
		return t.sharedTcp.DialContext(ctx, raddr)
	}

	// 使用端口重用拨号
	if t.UseReuseport() {
		return t.reuse.DialContext(ctx, raddr)
	}

	// 使用普通拨号器
	var d manet.Dialer
	return d.DialContext(ctx, raddr)
}

// Dial 拨号到指定的对等节点
// 参数:
//   - ctx: context.Context 上下文
//   - raddr: ma.Multiaddr 目标多地址
//   - p: peer.ID 对等节点 ID
//
// 返回值:
//   - transport.CapableConn 建立的连接
//   - error 错误信息
func (t *TcpTransport) Dial(ctx context.Context, raddr ma.Multiaddr, p peer.ID) (transport.CapableConn, error) {
	return t.DialWithUpdates(ctx, raddr, p, nil)
}

// DialWithUpdates 拨号到指定的对等节点,并提供拨号进度更新
// 参数:
//   - ctx: context.Context 上下文
//   - raddr: ma.Multiaddr 目标多地址
//   - p: peer.ID 对等节点 ID
//   - updateChan: chan<- transport.DialUpdate 拨号进度更新通道
//
// 返回值:
//   - transport.CapableConn 建立的连接
//   - error 错误信息
func (t *TcpTransport) DialWithUpdates(ctx context.Context, raddr ma.Multiaddr, p peer.ID, updateChan chan<- transport.DialUpdate) (transport.CapableConn, error) {
	// 打开出站连接
	connScope, err := t.rcmgr.OpenConnection(network.DirOutbound, true, raddr)
	if err != nil {
		log.Debugf("资源管理器阻止了出站连接: %s", err)
		return nil, err
	}

	// 使用连接作用域拨号
	c, err := t.dialWithScope(ctx, raddr, p, connScope, updateChan)
	if err != nil {
		connScope.Done()
		log.Debugf("拨号时出错: %s", err)
		return nil, err
	}
	return c, nil
}

// dialWithScope 在给定作用域内拨号到指定的对等节点
// 参数:
//   - ctx: context.Context 上下文
//   - raddr: ma.Multiaddr 目标多地址
//   - p: peer.ID 对等节点 ID
//   - connScope: network.ConnManagementScope 连接管理作用域
//   - updateChan: chan<- transport.DialUpdate 拨号进度更新通道
//
// 返回值:
//   - transport.CapableConn 建立的连接
//   - error 错误信息
func (t *TcpTransport) dialWithScope(ctx context.Context, raddr ma.Multiaddr, p peer.ID, connScope network.ConnManagementScope, updateChan chan<- transport.DialUpdate) (transport.CapableConn, error) {
	// 设置对等节点
	if err := connScope.SetPeer(p); err != nil {
		log.Debugf("资源管理器阻止了与对等节点的连接: %s", err)
		return nil, err
	}

	// 建立连接
	conn, err := t.maDial(ctx, raddr)
	if err != nil {
		log.Debugf("拨号时出错: %s", err)
		return nil, err
	}

	// 设置 linger 为 0,避免卡在 TIME-WAIT 状态
	// 当 linger 为 0 时,连接会被重置而不是通过 FIN 关闭
	// 这意味着我们可以立即重用 5 元组并重新连接
	tryLinger(conn, 0)
	tryKeepAlive(conn, true)

	c := conn
	// 启用指标收集
	if t.enableMetrics {
		var err error
		c, err = newTracingConn(conn, t.metricsCollector, true)
		if err != nil {
			log.Debugf("启用指标收集时出错: %s", err)
			return nil, err
		}
	}

	// 发送拨号进度更新
	if updateChan != nil {
		select {
		case updateChan <- transport.DialUpdate{Kind: transport.UpdateKindHandshakeProgressed, Addr: raddr}:
		default:
			// 跳过更新比延迟升级连接更好
		}
	}

	// 确定连接方向
	direction := network.DirOutbound
	if ok, isClient, _ := network.GetSimultaneousConnect(ctx); ok && !isClient {
		direction = network.DirInbound
	}

	// 升级连接
	return t.upgrader.Upgrade(ctx, t, c, direction, p, connScope)
}

// UseReuseport 判断是否启用端口重用
// 返回值:
//   - bool 是否启用端口重用
func (t *TcpTransport) UseReuseport() bool {
	return !t.disableReuseport && tcpreuse.ReuseportIsAvailable()
}

// unsharedMAListen 在指定地址创建非共享监听器
// 参数:
//   - laddr: ma.Multiaddr 监听地址
//
// 返回值:
//   - manet.Listener 创建的监听器
//   - error 错误信息
func (t *TcpTransport) unsharedMAListen(laddr ma.Multiaddr) (manet.Listener, error) {
	if t.UseReuseport() {
		return t.reuse.Listen(laddr)
	}
	return manet.Listen(laddr)
}

// Listen 在指定地址监听连接
// 参数:
//   - laddr: ma.Multiaddr 监听地址
//
// 返回值:
//   - transport.Listener 创建的监听器
//   - error 错误信息
func (t *TcpTransport) Listen(laddr ma.Multiaddr) (transport.Listener, error) {
	var list manet.Listener
	var err error

	// 创建监听器
	if t.sharedTcp == nil {
		list, err = t.unsharedMAListen(laddr)
	} else {
		list, err = t.sharedTcp.DemultiplexedListen(laddr, tcpreuse.DemultiplexedConnType_MultistreamSelect)
	}
	if err != nil {
		log.Debugf("创建监听器时出错: %s", err)
		return nil, err
	}

	// 启用指标收集
	if t.enableMetrics {
		list = newTracingListener(&tcpListener{list, 0}, t.metricsCollector)
	}

	// 升级监听器
	return t.upgrader.UpgradeListener(t, list), nil
}

// Protocols 返回此传输层可以拨号的终端协议列表
// 返回值:
//   - []int 协议 ID 列表
func (t *TcpTransport) Protocols() []int {
	return []int{ma.P_TCP}
}

// Proxy 返回此传输层是否为代理传输层
// 返回值:
//   - bool 始终返回 false
func (t *TcpTransport) Proxy() bool {
	return false
}

// String 返回传输层的字符串表示
// 返回值:
//   - string 传输层名称
func (t *TcpTransport) String() string {
	return "TCP"
}
