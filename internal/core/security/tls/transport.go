// Package tls 实现 TLS 1.3 安全传输
package tls

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"

	"github.com/dep2p/go-dep2p/internal/core/identity"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/types"
)

var logger = log.Logger("core/security/tls")

// 确保实现了接口
var _ pkgif.SecureTransport = (*Transport)(nil)

// Transport TLS 1.3 安全传输
//
// Transport 提供基于 TLS 1.3 的连接加密和身份验证。
type Transport struct {
	identity      pkgif.Identity
	cert          *tls.Certificate
	accessControl *AccessControl // 可选的访问控制
}

// New 创建 TLS 传输
//
// 参数：
//   - identity: 节点身份（提供私钥和公钥）
//
// 返回：
//   - *Transport: TLS 传输实例
//   - error: 创建失败时的错误
func New(identity pkgif.Identity) (*Transport, error) {
	if identity == nil {
		return nil, fmt.Errorf("identity is nil")
	}

	// 生成自签名证书
	cert, err := GenerateCert(identity)
	if err != nil {
		return nil, fmt.Errorf("generate certificate: %w", err)
	}

	return &Transport{
		identity: identity,
		cert:     cert,
	}, nil
}

// SetAccessControl 设置访问控制
//
// 设置后，握手前会检查远程节点是否被允许连接
func (t *Transport) SetAccessControl(ac *AccessControl) {
	t.accessControl = ac
}

// GetAccessControl 获取访问控制
func (t *Transport) GetAccessControl() *AccessControl {
	return t.accessControl
}

// ID 返回协议标识
func (t *Transport) ID() types.ProtocolID {
	return types.ProtocolID("/tls/1.0.0")
}

// SecureInbound 保护入站连接（服务器端握手）
//
// TLS 服务器握手流程：
//  1. 配置 TLS 服务器
//  2. 要求客户端证书
//  3. 自定义验证 VerifyPeerCertificate (INV-001)
//  4. 执行 TLS 握手
//  5. 提取远程公钥
//  6. 返回安全连接
//
// 参数：
//   - ctx: 上下文
//   - conn: 未加密的网络连接
//   - remotePeer: 期望的远程 PeerID
//
// 返回：
//   - pkgif.SecureConn: 安全连接
//   - error: 握手失败时的错误
func (t *Transport) SecureInbound(ctx context.Context, conn net.Conn, remotePeer types.PeerID) (pkgif.SecureConn, error) {
	// 访问控制检查（在握手前）
	if t.accessControl != nil && remotePeer != "" {
		if err := t.accessControl.Check(remotePeer); err != nil {
			remotePeerLabel := string(remotePeer)
			if len(remotePeerLabel) > 8 {
				remotePeerLabel = remotePeerLabel[:8]
			}
			logger.Warn("访问控制拒绝连接", "remotePeer", remotePeerLabel, "error", err)
			conn.Close()
			return nil, fmt.Errorf("access denied for peer %s: %w", remotePeerLabel, err)
		}
	}

	// TLS 服务器配置
	config := &tls.Config{
		Certificates:       []tls.Certificate{*t.cert}, // 服务器证书
		ClientAuth:         tls.RequireAnyClientCert,   // 要求客户端证书
		MinVersion:         tls.VersionTLS13,           // 强制 TLS 1.3
		InsecureSkipVerify: true,                       //nolint:gosec // G402: P2P 使用自定义 PeerID 验证替代 CA 验证

		// ⭐ INV-001: 自定义验证函数
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			return VerifyPeerCertificate(rawCerts, remotePeer)
		},

		NextProtos: []string{"dep2p"}, // ALPN
	}

	remotePeerLabel := string(remotePeer)
	if len(remotePeerLabel) > 8 {
		remotePeerLabel = remotePeerLabel[:8]
	}
	logger.Debug("TLS 入站握手", "remotePeer", remotePeerLabel)
	
	// TLS 服务器握手
	tlsConn := tls.Server(conn, config)

	// 执行握手
	if err := tlsConn.HandshakeContext(ctx); err != nil {
	logger.Warn("TLS 握手失败", "remotePeer", remotePeerLabel, "error", err)
		conn.Close()
		return nil, fmt.Errorf("TLS handshake failed: %w", err)
	}

	// 提取远程公钥
	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		tlsConn.Close()
		return nil, ErrNoCertificate
	}
	// 将证书原始字节包装成 [][]byte
	rawCerts := make([][]byte, len(state.PeerCertificates))
	for i, cert := range state.PeerCertificates {
		rawCerts[i] = cert.Raw
	}
	remotePubKey, err := extractRemotePublicKeyFromConn(rawCerts)
	if err != nil {
		tlsConn.Close()
		return nil, fmt.Errorf("extract remote public key: %w", err)
	}

	// 获取本地公钥字节
	localPubKeyBytes, err := t.identity.PublicKey().Raw()
	if err != nil {
		tlsConn.Close()
		return nil, fmt.Errorf("get local public key: %w", err)
	}

	// 获取远程公钥字节
	remotePubKeyBytes, err := remotePubKey.Raw()
	if err != nil {
		tlsConn.Close()
		return nil, fmt.Errorf("get remote public key: %w", err)
	}

	// 如果未提供远程 PeerID，使用证书派生的实际 PeerID
	if remotePeer == "" {
		derived, err := identity.PeerIDFromPublicKey(remotePubKey)
		if err != nil {
			tlsConn.Close()
			return nil, fmt.Errorf("derive remote peer ID: %w", err)
		}
		remotePeer = types.PeerID(derived)
	}

	// 创建安全连接
	localPeer := types.PeerID(t.identity.PeerID())
	remotePeerLabel = string(remotePeer)
	if len(remotePeerLabel) > 8 {
		remotePeerLabel = remotePeerLabel[:8]
	}
	logger.Debug("TLS 握手成功", "remotePeer", remotePeerLabel)
	return newSecureConn(tlsConn, localPeer, remotePeer, localPubKeyBytes, remotePubKeyBytes), nil
}

// SecureOutbound 保护出站连接（客户端握手）
//
// TLS 客户端握手流程：
//  1. 配置 TLS 客户端
//  2. 提供客户端证书
//  3. 自定义验证 VerifyPeerCertificate (INV-001)
//  4. 执行 TLS 握手
//  5. 提取远程公钥
//  6. 返回安全连接
//
// 参数：
//   - ctx: 上下文
//   - conn: 未加密的网络连接
//   - remotePeer: 期望的远程 PeerID
//
// 返回：
//   - pkgif.SecureConn: 安全连接
//   - error: 握手失败时的错误
func (t *Transport) SecureOutbound(ctx context.Context, conn net.Conn, remotePeer types.PeerID) (pkgif.SecureConn, error) {
	// 访问控制检查（在握手前）
	if t.accessControl != nil && remotePeer != "" {
		if err := t.accessControl.Check(remotePeer); err != nil {
			remotePeerLabel := string(remotePeer)
			if len(remotePeerLabel) > 8 {
				remotePeerLabel = remotePeerLabel[:8]
			}
			logger.Warn("访问控制拒绝连接", "remotePeer", remotePeerLabel, "error", err)
			conn.Close()
			return nil, fmt.Errorf("access denied for peer %s: %w", remotePeerLabel, err)
		}
	}

	// TLS 客户端配置
	config := &tls.Config{
		Certificates:       []tls.Certificate{*t.cert}, // 客户端证书
		MinVersion:         tls.VersionTLS13,           // 强制 TLS 1.3
		InsecureSkipVerify: true,                       //nolint:gosec // G402: P2P 使用自定义 PeerID 验证替代 CA 验证
		ServerName:         "dep2p",                    // SNI

		// ⭐ INV-001: 自定义验证函数
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			return VerifyPeerCertificate(rawCerts, remotePeer)
		},

		NextProtos: []string{"dep2p"}, // ALPN
	}

	remotePeerLabel := string(remotePeer)
	if len(remotePeerLabel) > 8 {
		remotePeerLabel = remotePeerLabel[:8]
	}
	logger.Debug("TLS 出站握手", "remotePeer", remotePeerLabel)
	
	// TLS 客户端握手
	tlsConn := tls.Client(conn, config)

	// 执行握手
	if err := tlsConn.HandshakeContext(ctx); err != nil {
	logger.Warn("TLS 握手失败", "remotePeer", remotePeerLabel, "error", err)
		conn.Close()
		return nil, fmt.Errorf("TLS handshake failed: %w", err)
	}

	// 提取远程公钥
	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		tlsConn.Close()
		return nil, ErrNoCertificate
	}
	// 将证书原始字节包装成 [][]byte
	rawCerts := make([][]byte, len(state.PeerCertificates))
	for i, cert := range state.PeerCertificates {
		rawCerts[i] = cert.Raw
	}
	remotePubKey, err := extractRemotePublicKeyFromConn(rawCerts)
	if err != nil {
		tlsConn.Close()
		return nil, fmt.Errorf("extract remote public key: %w", err)
	}

	// 获取本地公钥字节
	localPubKeyBytes, err := t.identity.PublicKey().Raw()
	if err != nil {
		tlsConn.Close()
		return nil, fmt.Errorf("get local public key: %w", err)
	}

	// 获取远程公钥字节
	remotePubKeyBytes, err := remotePubKey.Raw()
	if err != nil {
		tlsConn.Close()
		return nil, fmt.Errorf("get remote public key: %w", err)
	}

	// 创建安全连接
	localPeer := types.PeerID(t.identity.PeerID())
	remotePeerLabel = string(remotePeer)
	if len(remotePeerLabel) > 8 {
		remotePeerLabel = remotePeerLabel[:8]
	}
	logger.Debug("TLS 握手成功", "remotePeer", remotePeerLabel)
	return newSecureConn(tlsConn, localPeer, remotePeer, localPubKeyBytes, remotePubKeyBytes), nil
}
