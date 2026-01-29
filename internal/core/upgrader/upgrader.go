// Package upgrader 实现连接升级器
package upgrader

import (
	"context"
	"fmt"
	"net"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("core/upgrader")

// truncateID 安全截取 ID 用于日志显示
func truncateID(id string, maxLen int) string {
	if len(id) <= maxLen {
		return id
	}
	return id[:maxLen]
}

// 确保实现了接口
var _ pkgif.Upgrader = (*Upgrader)(nil)

// Upgrader 连接升级器
type Upgrader struct {
	identity pkgif.Identity

	securityTransports []pkgif.SecureTransport
	streamMuxers       []pkgif.StreamMuxer

	resourceMgr pkgif.ResourceManager
}

// New 创建连接升级器
func New(id pkgif.Identity, cfg Config) (*Upgrader, error) {
	if id == nil {
		return nil, ErrNilIdentity
	}

	if len(cfg.SecurityTransports) == 0 {
		return nil, ErrNoSecurityTransport
	}

	if len(cfg.StreamMuxers) == 0 {
		return nil, ErrNoStreamMuxer
	}

	return &Upgrader{
		identity:           id,
		securityTransports: cfg.SecurityTransports,
		streamMuxers:       cfg.StreamMuxers,
		resourceMgr:        cfg.ResourceManager,
	}, nil
}

// Upgrade 升级连接
//
// 升级流程：
//  1. 申请连接资源（ResourceManager）
//  2. 协商安全协议（multistream-select）
//  3. 安全握手（TLS/Noise）
//  4. 设置远程 PeerID（ResourceManager）
//  5. 协商多路复用器（multistream-select）
//  6. 多路复用设置（yamux）
//
// QUIC 连接会跳过升级流程（自带加密和多路复用）
func (u *Upgrader) Upgrade(
	ctx context.Context,
	conn net.Conn,
	dir pkgif.Direction,
	remotePeer types.PeerID,
) (pkgif.UpgradedConn, error) {
	// Outbound 必须提供 remotePeer
	if dir == pkgif.DirOutbound && remotePeer == "" {
		return nil, ErrNoPeerID
	}

	// 检测 QUIC 连接，跳过升级
	// QUIC 自带 TLS 1.3 加密和流多路复用，无需额外升级
	if isQUICConn(conn) {
		return wrapQUICConn(conn, u.resourceMgr, remotePeer)
	}

	isServer := dir == pkgif.DirInbound

	// 1. 申请连接资源（如果有 ResourceManager）
	var connScope pkgif.ConnManagementScope
	if u.resourceMgr != nil {
		// 获取远程地址（用于资源追踪）
		var endpoint types.Multiaddr
		if remoteAddr := conn.RemoteAddr(); remoteAddr != nil {
			// 尝试转换为 Multiaddr（简化实现）
			endpoint, _ = types.NewMultiaddr("/ip4/" + remoteAddr.String())
		}
		
		// 打开连接资源范围
		var err error
		connScope, err = u.resourceMgr.OpenConnection(dir, true, endpoint)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("open connection scope: %w", err)
		}
	}

	// 2. 协商安全协议
	logger.Debug("协商安全协议", "direction", dir, "remotePeer", truncateID(string(remotePeer), 8))
	secTransport, err := u.negotiateSecurity(ctx, conn, isServer)
	if err != nil {
		logger.Warn("安全协议协商失败", "error", err)
		if connScope != nil {
			connScope.Done()
		}
		conn.Close()
		return nil, fmt.Errorf("security negotiation: %w", err)
	}

	// 3. 安全握手
	logger.Debug("执行安全握手", "isServer", isServer)
	var secConn pkgif.SecureConn
	if isServer {
		// 服务器端：入站握手
		// remotePeer 可能为空，由握手后确定
		secConn, err = secTransport.SecureInbound(ctx, conn, remotePeer)
	} else {
		// 客户端：出站握手
		// remotePeer 必须提供
		secConn, err = secTransport.SecureOutbound(ctx, conn, remotePeer)
	}
	if err != nil {
		logger.Warn("安全握手失败", "error", err)
		if connScope != nil {
			connScope.Done()
		}
		conn.Close()
		return nil, fmt.Errorf("security handshake: %w", err)
	}
	logger.Debug("安全握手成功", "remotePeer", truncateID(string(secConn.RemotePeer()), 8))

	// 4. 设置远程 PeerID（用于资源追踪）
	if connScope != nil {
		// 安全握手后获取实际的远程 PeerID
		actualRemotePeer := secConn.RemotePeer()
		if actualRemotePeer != "" {
			if err := connScope.SetPeer(actualRemotePeer); err != nil {
				// SetPeer 失败可能是因为资源限制
				connScope.Done()
				secConn.Close()
				return nil, fmt.Errorf("set peer in resource scope: %w", err)
			}
		}
	}

	// 5. 协商多路复用器
	logger.Debug("协商多路复用器")
	muxer, err := u.negotiateMuxer(ctx, secConn, isServer)
	if err != nil {
		logger.Warn("多路复用器协商失败", "error", err)
		if connScope != nil {
			connScope.Done()
		}
		secConn.Close()
		return nil, fmt.Errorf("muxer negotiation: %w", err)
	}

	// 6. 创建多路复用连接
	// 从 ConnManagementScope 获取 PeerScope
	var peerScope pkgif.PeerScope
	if connScope != nil {
		peerScope = connScope.PeerScope()
	}
	
	muxedConn, err := muxer.NewConn(secConn, isServer, peerScope)
	if err != nil {
		if connScope != nil {
			connScope.Done()
		}
		secConn.Close()
		return nil, fmt.Errorf("muxer setup: %w", err)
	}

	// 7. 封装为 UpgradedConn
	upgradedConn := newUpgradedConnWithScope(
		muxedConn,
		secConn,
		secTransport.ID(),
		muxer.ID(),
		connScope,
	)
	
	logger.Info("连接升级成功", "remotePeer", truncateID(string(secConn.RemotePeer()), 8), "security", secTransport.ID(), "muxer", muxer.ID())

	return upgradedConn, nil
}

// ============================================================================
// QUIC 连接检测和封装
// ============================================================================

// QuicConn QUIC 连接接口（用于类型检测）
//
// QUIC 连接已经内置 TLS 1.3 加密和流多路复用，
// 实现了 pkgif.Connection 接口，无需额外升级。
type QuicConn interface {
	net.Conn
	pkgif.Connection
}

// isQUICConn 检测是否为 QUIC 连接
//
// 检测方法：
//  1. 尝试类型断言为 QuicConn 接口
//  2. 或者检查是否同时实现了 net.Conn 和 pkgif.Connection
func isQUICConn(conn net.Conn) bool {
	// 方法 1: 类型断言为 QuicConn
	if _, ok := conn.(QuicConn); ok {
		return true
	}
	
	// 方法 2: 检查是否同时实现了 pkgif.Connection
	if _, ok := conn.(pkgif.Connection); ok {
		return true
	}
	
	return false
}

// wrapQUICConn 封装 QUIC 连接为 UpgradedConn
//
// QUIC 连接已经提供了：
//  - TLS 1.3 加密（安全传输）
//  - 流多路复用（QUIC 原生支持）
//
// 因此只需简单封装，不需要额外的安全握手和多路复用器设置。
func wrapQUICConn(conn net.Conn, resourceMgr pkgif.ResourceManager, _ types.PeerID) (pkgif.UpgradedConn, error) {
	// 类型断言为 pkgif.Connection
	quicConn, ok := conn.(pkgif.Connection)
	if !ok {
		return nil, fmt.Errorf("conn does not implement pkgif.Connection")
	}
	
	// 注意：ResourceManager 集成暂时简化
	// 完整实现需要在连接建立时就分配资源
	_ = resourceMgr
	
	// 创建 QUIC 专用的 UpgradedConn
	upgradedConn := &quicUpgradedConn{
		Connection: quicConn,
	}
	
	return upgradedConn, nil
}

// quicUpgradedConn QUIC 升级连接（简化版）
//
// QUIC 连接本身已经是"升级后"的连接，
// 因此这个结构只是简单地封装 pkgif.Connection。
type quicUpgradedConn struct {
	pkgif.Connection
}

// 确保实现接口
var _ pkgif.UpgradedConn = (*quicUpgradedConn)(nil)

// Security 返回安全协议（QUIC 使用 TLS 1.3）
func (c *quicUpgradedConn) Security() types.ProtocolID {
	return types.ProtocolID("/quic/tls/1.3")
}

// Muxer 返回多路复用协议（QUIC 原生多路复用）
func (c *quicUpgradedConn) Muxer() string {
	return "/quic/muxer/1.0"
}

// OpenStream 打开新流
func (c *quicUpgradedConn) OpenStream(ctx context.Context) (pkgif.MuxedStream, error) {
	stream, err := c.NewStream(ctx)
	if err != nil {
		return nil, err
	}
	return &quicMuxedStream{Stream: stream}, nil
}

// AcceptStream 接受新流（QUIC 原生支持）
func (c *quicUpgradedConn) AcceptStream() (pkgif.MuxedStream, error) {
	// QUIC Connection 的 AcceptStream 返回 Stream
	// 需要转换为 MuxedStream
	stream, err := c.Connection.AcceptStream()
	if err != nil {
		return nil, err
	}
	
	// 包装为 MuxedStream
	return &quicMuxedStream{Stream: stream}, nil
}

// quicMuxedStream 将 Stream 包装为 MuxedStream
type quicMuxedStream struct {
	pkgif.Stream
}

// 确保实现接口
var _ pkgif.MuxedStream = (*quicMuxedStream)(nil)

// Reset 重置流
func (s *quicMuxedStream) Reset() error {
	// QUIC Stream 支持重置
	return s.Stream.Reset()
}

// CloseWrite 关闭写端
func (s *quicMuxedStream) CloseWrite() error {
	// QUIC Stream 支持半关闭
	// 由于 Stream 接口没有 CloseWrite，使用 Close
	return s.Close()
}

// CloseRead 关闭读端
func (s *quicMuxedStream) CloseRead() error {
	// QUIC Stream 支持半关闭
	// 由于 Stream 接口没有 CloseRead，这里是 no-op
	return nil
}

// SetDeadline 设置读写截止时间
func (s *quicMuxedStream) SetDeadline(t time.Time) error {
	return s.Stream.SetDeadline(t)
}

// SetReadDeadline 设置读截止时间
func (s *quicMuxedStream) SetReadDeadline(t time.Time) error {
	return s.Stream.SetReadDeadline(t)
}

// SetWriteDeadline 设置写截止时间
func (s *quicMuxedStream) SetWriteDeadline(t time.Time) error {
	return s.Stream.SetWriteDeadline(t)
}
