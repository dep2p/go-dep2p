package upgrader

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/dep2p/go-dep2p/core/connmgr"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	ipnet "github.com/dep2p/go-dep2p/core/pnet"
	"github.com/dep2p/go-dep2p/core/protocol"
	"github.com/dep2p/go-dep2p/core/sec"
	"github.com/dep2p/go-dep2p/core/transport"
	"github.com/dep2p/go-dep2p/p2p/net/pnet"

	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	mss "github.com/dep2p/go-dep2p/multiformats/multistream"
)

// ErrNilPeer 在尝试升级出站连接但未指定对等点ID时返回
var ErrNilPeer = errors.New("空对等点")

// AcceptQueueLength 是在不接受任何新连接之前完全设置的连接数
var AcceptQueueLength = 16

const (
	// defaultAcceptTimeout 默认接受超时时间
	defaultAcceptTimeout = 15 * time.Second
	// defaultNegotiateTimeout 默认协商超时时间
	defaultNegotiateTimeout = 60 * time.Second
)

// Option 是配置 upgrader 的函数类型
type Option func(*upgrader) error

// WithAcceptTimeout 设置接受超时时间
// 参数:
//   - t: time.Duration 超时时间
//
// 返回值:
//   - Option 配置函数
func WithAcceptTimeout(t time.Duration) Option {
	return func(u *upgrader) error {
		u.acceptTimeout = t
		return nil
	}
}

// StreamMuxer 流多路复用器结构体
type StreamMuxer struct {
	// ID 多路复用器协议ID
	ID protocol.ID
	// Muxer 网络多路复用器实现
	Muxer network.Multiplexer
}

// upgrader 是一个多流升级器,可以将底层连接升级为完整的传输连接(安全且多路复用)
type upgrader struct {
	// psk 私有网络共享密钥
	psk ipnet.PSK
	// connGater 连接过滤器
	connGater connmgr.ConnectionGater
	// rcmgr 资源管理器
	rcmgr network.ResourceManager

	// muxerMuxer 多路复用器的多流选择器
	muxerMuxer *mss.MultistreamMuxer[protocol.ID]
	// muxers 可用的多路复用器列表
	muxers []StreamMuxer
	// muxerIDs 多路复用器协议ID列表
	muxerIDs []protocol.ID

	// security 安全传输列表
	security []sec.SecureTransport
	// securityMuxer 安全传输的多流选择器
	securityMuxer *mss.MultistreamMuxer[protocol.ID]
	// securityIDs 安全传输协议ID列表
	securityIDs []protocol.ID

	// acceptTimeout 是允许 Accept 操作花费的最大时间
	// 包括接受原始网络连接、协议选择以及握手(如果适用)的时间
	//
	// 如果未设置,使用默认值(15s)
	acceptTimeout time.Duration
}

// 确保 upgrader 实现了 transport.Upgrader 接口
var _ transport.Upgrader = &upgrader{}

// New 创建一个新的升级器
// 参数:
//   - security: []sec.SecureTransport 安全传输列表
//   - muxers: []StreamMuxer 多路复用器列表
//   - psk: ipnet.PSK 私有网络共享密钥
//   - rcmgr: network.ResourceManager 资源管理器
//   - connGater: connmgr.ConnectionGater 连接过滤器
//   - opts: ...Option 配置选项
//
// 返回值:
//   - transport.Upgrader 升级器实例
//   - error 错误信息
func New(security []sec.SecureTransport, muxers []StreamMuxer, psk ipnet.PSK, rcmgr network.ResourceManager, connGater connmgr.ConnectionGater, opts ...Option) (transport.Upgrader, error) {
	// 创建升级器实例
	u := &upgrader{
		acceptTimeout: defaultAcceptTimeout,
		rcmgr:         rcmgr,
		connGater:     connGater,
		psk:           psk,
		muxerMuxer:    mss.NewMultistreamMuxer[protocol.ID](),
		muxers:        muxers,
		security:      security,
		securityMuxer: mss.NewMultistreamMuxer[protocol.ID](),
	}

	// 应用配置选项
	for _, opt := range opts {
		if err := opt(u); err != nil {
			log.Debugf("配置升级器失败: %v", err)
			return nil, err
		}
	}

	// 如果未提供资源管理器,使用空实现
	if u.rcmgr == nil {
		u.rcmgr = &network.NullResourceManager{}
	}

	// 初始化多路复用器ID列表
	u.muxerIDs = make([]protocol.ID, 0, len(muxers))
	for _, m := range muxers {
		u.muxerMuxer.AddHandler(m.ID, nil)
		u.muxerIDs = append(u.muxerIDs, m.ID)
	}

	// 初始化安全传输ID列表
	u.securityIDs = make([]protocol.ID, 0, len(security))
	for _, s := range security {
		u.securityMuxer.AddHandler(s.ID(), nil)
		u.securityIDs = append(u.securityIDs, s.ID())
	}
	return u, nil
}

// UpgradeListener 将传入的多地址网络监听器升级为完整的 dep2p 传输监听器
// 参数:
//   - t: transport.Transport 传输层对象
//   - list: manet.Listener 多地址网络监听器
//
// 返回值:
//   - transport.Listener 升级后的传输监听器
func (u *upgrader) UpgradeListener(t transport.Transport, list manet.Listener) transport.Listener {
	// 创建带取消功能的上下文
	ctx, cancel := context.WithCancel(context.Background())
	// 创建监听器实例
	l := &listener{
		Listener:  list,
		upgrader:  u,
		transport: t,
		rcmgr:     u.rcmgr,
		threshold: newThreshold(AcceptQueueLength),
		incoming:  make(chan transport.CapableConn),
		cancel:    cancel,
		ctx:       ctx,
	}
	// 启动处理传入连接的 goroutine
	go l.handleIncoming()
	return l
}

// Upgrade 将多地址/网络连接升级为完整的 dep2p 传输连接
// 参数:
//   - ctx: context.Context 上下文
//   - t: transport.Transport 传输层对象
//   - maconn: manet.Conn 多地址网络连接
//   - dir: network.Direction 连接方向
//   - p: peer.ID 对等点ID
//   - connScope: network.ConnManagementScope 连接管理范围
//
// 返回值:
//   - transport.CapableConn 升级后的传输连接
//   - error 错误信息
func (u *upgrader) Upgrade(ctx context.Context, t transport.Transport, maconn manet.Conn, dir network.Direction, p peer.ID, connScope network.ConnManagementScope) (transport.CapableConn, error) {
	// 升级连接
	c, err := u.upgrade(ctx, t, maconn, dir, p, connScope)
	if err != nil {
		// 如果升级失败,标记连接管理范围完成
		connScope.Done()
		log.Debugf("升级连接失败: %v", err)
		return nil, err
	}
	return c, nil
}

// upgrade 执行实际的连接升级操作
// 参数:
//   - ctx: context.Context 上下文
//   - t: transport.Transport 传输层对象
//   - maconn: manet.Conn 多地址网络连接
//   - dir: network.Direction 连接方向
//   - p: peer.ID 对等点ID
//   - connScope: network.ConnManagementScope 连接管理范围
//
// 返回值:
//   - transport.CapableConn 升级后的传输连接
//   - error 错误信息
func (u *upgrader) upgrade(ctx context.Context, t transport.Transport, maconn manet.Conn, dir network.Direction, p peer.ID, connScope network.ConnManagementScope) (transport.CapableConn, error) {
	// 检查出站连接是否指定了对等点ID
	if dir == network.DirOutbound && p == "" {
		log.Debugf("出站连接未指定对等点ID")
		return nil, ErrNilPeer
	}

	// 获取连接统计信息
	var stat network.ConnStats
	if cs, ok := maconn.(network.ConnStat); ok {
		stat = cs.Stat()
	}

	// 处理私有网络
	var conn net.Conn = maconn
	if u.psk != nil {
		// 设置私有网络保护
		pconn, err := pnet.NewProtectedConn(u.psk, conn)
		if err != nil {
			conn.Close()
			log.Debugf("设置私有网络保护失败: %v", err)
			return nil, err
		}
		conn = pconn
	} else if ipnet.ForcePrivateNetwork {
		// 如果强制使用私有网络但未设置保护器,返回错误
		log.Debugf("尝试在没有私有网络保护器的情况下拨号,但环境强制要求使用私有网络")
		return nil, ipnet.ErrNotInPrivateNetwork
	}

	// 设置安全层
	isServer := dir == network.DirInbound
	sconn, security, err := u.setupSecurity(ctx, conn, p, isServer)
	if err != nil {
		conn.Close()
		log.Debugf("协商安全协议失败: %v", err)
		return nil, err
	}

	// 调用连接过滤器(如果已注册)
	if u.connGater != nil && !u.connGater.InterceptSecured(dir, sconn.RemotePeer(), maconn) {
		if err := maconn.Close(); err != nil {
			log.Debugf("关闭连接失败", "peer", p, "addr", maconn.RemoteMultiaddr(), "error", err)
		}
		return nil, fmt.Errorf("过滤器拒绝了与对等点 %s 和地址 %s 的方向为 %d 的连接",
			sconn.RemotePeer(), maconn.RemoteMultiaddr(), dir)
	}

	// 仅在未设置的情况下设置对等点
	if connScope.PeerScope() == nil {
		if err := connScope.SetPeer(sconn.RemotePeer()); err != nil {
			log.Debugw("资源管理器阻止了对等点连接", "peer", sconn.RemotePeer(), "addr", conn.RemoteAddr(), "error", err)
			if err := maconn.Close(); err != nil {
				log.Debugf("关闭连接失败", "peer", p, "addr", maconn.RemoteMultiaddr(), "error", err)
			}
			return nil, fmt.Errorf("资源管理器阻止了与对等点 %s 和地址 %s 的方向为 %d 的连接",
				sconn.RemotePeer(), maconn.RemoteMultiaddr(), dir)
		}
	}

	// 设置多路复用器
	muxer, smconn, err := u.setupMuxer(ctx, sconn, isServer, connScope.PeerScope())
	if err != nil {
		sconn.Close()
		log.Debugf("协商流多路复用器失败: %v", err)
		return nil, err
	}

	// 创建传输连接
	tc := &transportConn{
		MuxedConn:                 smconn,
		ConnMultiaddrs:            maconn,
		ConnSecurity:              sconn,
		transport:                 t,
		stat:                      stat,
		scope:                     connScope,
		muxer:                     muxer,
		security:                  security,
		usedEarlyMuxerNegotiation: sconn.ConnState().UsedEarlyMuxerNegotiation,
	}
	return tc, nil
}

// setupSecurity 设置连接的安全层
// 参数:
//   - ctx: context.Context 上下文
//   - conn: net.Conn 网络连接
//   - p: peer.ID 对等点ID
//   - isServer: bool 是否为服务端
//
// 返回值:
//   - sec.SecureConn 安全连接
//   - protocol.ID 安全协议ID
//   - error 错误信息
func (u *upgrader) setupSecurity(ctx context.Context, conn net.Conn, p peer.ID, isServer bool) (sec.SecureConn, protocol.ID, error) {
	// 协商安全传输
	st, err := u.negotiateSecurity(ctx, conn, isServer)
	if err != nil {
		log.Debugf("协商安全传输失败: %v", err)
		return nil, "", err
	}
	if isServer {
		// 服务端安全连接
		sconn, err := st.SecureInbound(ctx, conn, p)
		return sconn, st.ID(), err
	}
	// 客户端安全连接
	sconn, err := st.SecureOutbound(ctx, conn, p)
	return sconn, st.ID(), err
}

// negotiateMuxer 协商多路复用器
// 参数:
//   - nc: net.Conn 网络连接
//   - isServer: bool 是否为服务端
//
// 返回值:
//   - *StreamMuxer 流多路复用器
//   - error 错误信息
func (u *upgrader) negotiateMuxer(nc net.Conn, isServer bool) (*StreamMuxer, error) {
	// 设置协商超时
	if err := nc.SetDeadline(time.Now().Add(defaultNegotiateTimeout)); err != nil {
		log.Debugf("设置协商超时失败: %v", err)
		return nil, err
	}

	var proto protocol.ID
	if isServer {
		// 服务端协商
		selected, _, err := u.muxerMuxer.Negotiate(nc)
		if err != nil {
			log.Debugf("服务端协商多路复用器失败: %v", err)
			return nil, err
		}
		proto = selected
	} else {
		// 客户端协商
		selected, err := mss.SelectOneOf(u.muxerIDs, nc)
		if err != nil {
			log.Debugf("客户端协商多路复用器失败: %v", err)
			return nil, err
		}
		proto = selected
	}

	// 清除超时设置
	if err := nc.SetDeadline(time.Time{}); err != nil {
		log.Debugf("清除协商超时失败: %v", err)
		return nil, err
	}

	// 获取选定的多路复用器
	if m := u.getMuxerByID(proto); m != nil {
		return m, nil
	}
	log.Debugf("选择了一个我们没有传输层的协议")
	return nil, fmt.Errorf("选择了一个我们没有传输层的协议")
}

// getMuxerByID 根据ID获取多路复用器
// 参数:
//   - id: protocol.ID 协议ID
//
// 返回值:
//   - *StreamMuxer 流多路复用器
func (u *upgrader) getMuxerByID(id protocol.ID) *StreamMuxer {
	for _, m := range u.muxers {
		if m.ID == id {
			return &m
		}
	}
	return nil
}

// setupMuxer 设置多路复用器
// 参数:
//   - ctx: context.Context 上下文
//   - conn: sec.SecureConn 安全连接
//   - server: bool 是否为服务端
//   - scope: network.PeerScope 对等点范围
//
// 返回值:
//   - protocol.ID 协议ID
//   - network.MuxedConn 多路复用连接
//   - error 错误信息
func (u *upgrader) setupMuxer(ctx context.Context, conn sec.SecureConn, server bool, scope network.PeerScope) (protocol.ID, network.MuxedConn, error) {
	// 从安全握手中获取选定的多路复用器
	muxerSelected := conn.ConnState().StreamMultiplexer
	// 如果可用则使用安全握手选定的多路复用器,否则回退到多流选择
	if len(muxerSelected) > 0 {
		m := u.getMuxerByID(muxerSelected)
		if m == nil {
			log.Debugf("选择了一个未知的多路复用器: %s", muxerSelected)
			return "", nil, fmt.Errorf("选择了一个未知的多路复用器: %s", muxerSelected)
		}
		c, err := m.Muxer.NewConn(conn, server, scope)
		if err != nil {
			log.Debugf("创建多路复用连接失败: %v", err)
			return "", nil, err
		}
		return muxerSelected, c, nil
	}

	// 定义结果类型
	type result struct {
		smconn  network.MuxedConn
		muxerID protocol.ID
		err     error
	}

	// 创建结果通道
	done := make(chan result, 1)
	// TODO: 多路复用器应该接受上下文
	go func() {
		// 协商多路复用器
		m, err := u.negotiateMuxer(conn, server)
		if err != nil {
			done <- result{err: err}
			return
		}
		// 创建新的多路复用连接
		smconn, err := m.Muxer.NewConn(conn, server, scope)
		done <- result{smconn: smconn, muxerID: m.ID, err: err}
	}()

	// 等待结果或上下文取消
	select {
	case r := <-done:
		return r.muxerID, r.smconn, r.err
	case <-ctx.Done():
		// 中断此过程
		conn.Close()
		// 等待完成
		<-done
		return "", nil, ctx.Err()
	}
}

// getSecurityByID 根据ID获取安全传输
// 参数:
//   - id: protocol.ID 协议ID
//
// 返回值:
//   - sec.SecureTransport 安全传输
func (u *upgrader) getSecurityByID(id protocol.ID) sec.SecureTransport {
	for _, s := range u.security {
		if s.ID() == id {
			return s
		}
	}
	return nil
}

// negotiateSecurity 协商安全传输
// 参数:
//   - ctx: context.Context 上下文
//   - insecure: net.Conn 不安全的连接
//   - server: bool 是否为服务端
//
// 返回值:
//   - sec.SecureTransport 安全传输
//   - error 错误信息
func (u *upgrader) negotiateSecurity(ctx context.Context, insecure net.Conn, server bool) (sec.SecureTransport, error) {
	// 定义结果类型
	type result struct {
		proto protocol.ID
		err   error
	}

	// 创建结果通道
	done := make(chan result, 1)
	go func() {
		if server {
			// 服务端协商
			var r result
			r.proto, _, r.err = u.securityMuxer.Negotiate(insecure)
			done <- r
			return
		}
		// 客户端协商
		var r result
		r.proto, r.err = mss.SelectOneOf(u.securityIDs, insecure)
		done <- r
	}()

	// 等待结果或上下文取消
	select {
	case r := <-done:
		if r.err != nil {
			log.Debugf("协商安全传输失败: %v", r.err)
			return nil, r.err
		}
		if s := u.getSecurityByID(r.proto); s != nil {
			return s, nil
		}
		return nil, fmt.Errorf("选择了未知的安全传输: %s", r.proto)
	case <-ctx.Done():
		// 必须执行此操作。我们在连接上有未完成的工作,不再安全使用
		insecure.Close()
		<-done // 等待停止使用连接
		return nil, ctx.Err()
	}
}
