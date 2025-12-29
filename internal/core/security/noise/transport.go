package noise

import (
	"context"
	"fmt"
	"time"

	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	securityif "github.com/dep2p/go-dep2p/pkg/interfaces/security"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// DefaultHandshakeTimeout 默认握手超时
const DefaultHandshakeTimeout = 10 * time.Second

// Transport Noise 安全传输
//
// 将普通连接升级为 Noise 加密连接。
// 实现 securityif.SecureTransport 接口。
//
// 通过 identity 绑定机制，确保 Noise 的 RemoteIdentity()
// 返回的是真正的 dep2p identity NodeID，而非 Noise DH 公钥的哈希。
type Transport struct {
	identity   identityif.Identity
	config     *securityif.NoiseConfig
	handshaker *Handshaker
}

// 确保实现 securityif.SecureTransport 接口
var _ securityif.SecureTransport = (*Transport)(nil)

// NewTransport 创建 Noise 传输
//
// identity 参数是必需的，用于在握手过程中进行 identity 绑定，
// 确保 RemoteIdentity() 返回正确的 dep2p NodeID。
// keyFactory 参数用于验证远程 identity 绑定。
func NewTransport(identity identityif.Identity, config *securityif.NoiseConfig, keyFactory identityif.KeyFactory) (*Transport, error) {
	if identity == nil {
		return nil, fmt.Errorf("identity 不能为空")
	}

	if config == nil {
		config = securityif.DefaultNoiseConfig()
	}

	// 创建带完整依赖的握手处理器
	handshaker, err := NewHandshakerWithDeps(config, identity, keyFactory)
	if err != nil {
		return nil, fmt.Errorf("创建握手处理器失败: %w", err)
	}

	log.Info("Noise 传输已创建",
		"pattern", config.HandshakePattern,
		"cipher", config.CipherSuite,
		"identityBinding", true)

	return &Transport{
		identity:   identity,
		config:     config,
		handshaker: handshaker,
	}, nil
}

// SecureInbound 对入站连接进行安全握手
//
// 服务端角色，被动接受连接并执行 Noise 握手。
// 握手过程中会验证对方的 identity 绑定（如果提供）。
func (t *Transport) SecureInbound(ctx context.Context, conn transportif.Conn) (securityif.SecureConn, error) {
	log.Debug("开始入站 Noise 握手",
		"remoteAddr", conn.RemoteAddr().String())

	// 设置握手超时
	deadline := t.getDeadline(ctx)
	if err := conn.SetDeadline(deadline); err != nil {
		log.Debug("设置握手超时失败", "err", err)
	}

	// 获取底层 io.ReadWriter
	rw := t.getReadWriter(conn)

	// 执行握手
	result, err := t.handshaker.HandshakeAsResponder(rw, deadline)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("Noise 握手失败: %w", err)
	}

	// 清除超时
	if err := conn.SetDeadline(time.Time{}); err != nil {
		log.Debug("清除握手超时失败", "err", err)
	}

	log.Debug("入站 Noise 握手成功",
		"remoteID", result.RemoteID.String(),
		"remoteAddr", conn.RemoteAddr().String(),
		"hasIdentityBinding", result.RemoteIdentityPubKey != nil)

	// 创建安全连接
	return NewSecureConnWithIdentity(
		conn,
		result,
		t.identity.ID(),
		t.identity.PublicKey(),
	), nil
}

// SecureOutbound 对出站连接进行安全握手
//
// 客户端角色，主动发起连接并执行 Noise 握手。
// remotePeer 是期望的远程节点 ID，用于身份验证。
// 如果对方提供了 identity 绑定，会验证其 NodeID 与 remotePeer 匹配。
func (t *Transport) SecureOutbound(ctx context.Context, conn transportif.Conn, remotePeer types.NodeID) (securityif.SecureConn, error) {
	log.Debug("开始出站 Noise 握手",
		"remoteAddr", conn.RemoteAddr().String(),
		"expectedPeerID", remotePeer.String())

	// 设置握手超时
	deadline := t.getDeadline(ctx)
	if err := conn.SetDeadline(deadline); err != nil {
		log.Debug("设置握手超时失败", "err", err)
	}

	// 获取底层 io.ReadWriter
	rw := t.getReadWriter(conn)

	// 执行握手
	result, err := t.handshaker.HandshakeAsInitiator(rw, remotePeer, deadline)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("Noise 握手失败: %w", err)
	}

	// 清除超时
	if err := conn.SetDeadline(time.Time{}); err != nil {
		log.Debug("清除握手超时失败", "err", err)
	}

	log.Debug("出站 Noise 握手成功",
		"remoteID", result.RemoteID.String(),
		"remoteAddr", conn.RemoteAddr().String(),
		"hasIdentityBinding", result.RemoteIdentityPubKey != nil)

	// 创建安全连接
	return NewSecureConnWithIdentity(
		conn,
		result,
		t.identity.ID(),
		t.identity.PublicKey(),
	), nil
}

// Protocol 返回安全协议名称
func (t *Transport) Protocol() string {
	return "noise"
}

// getDeadline 获取握手截止时间
func (t *Transport) getDeadline(ctx context.Context) time.Time {
	if deadline, ok := ctx.Deadline(); ok {
		return deadline
	}

	timeout := DefaultHandshakeTimeout
	if t.config != nil && t.config.HandshakeTimeout > 0 {
		timeout = time.Duration(t.config.HandshakeTimeout) * time.Second
	}

	return time.Now().Add(timeout)
}

// getReadWriter 获取连接的 io.ReadWriter 接口
func (t *Transport) getReadWriter(conn transportif.Conn) connReadWriter {
	return connReadWriter{conn: conn}
}

// connReadWriter 适配 transportif.Conn 为 io.ReadWriter
type connReadWriter struct {
	conn transportif.Conn
}

// Read 读取数据
func (rw connReadWriter) Read(b []byte) (int, error) {
	return rw.conn.Read(b)
}

// Write 写入数据
func (rw connReadWriter) Write(b []byte) (int, error) {
	return rw.conn.Write(b)
}
