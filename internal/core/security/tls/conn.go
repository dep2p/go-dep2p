// Package tls 提供基于 TLS 的安全传输实现
package tls

import (
	"bytes"
	stdcrypto "crypto"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	securityif "github.com/dep2p/go-dep2p/pkg/interfaces/security"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// SecureConn 安全连接封装
// 将 tls.Conn 包装为 securityif.SecureConn
type SecureConn struct {
	tlsConn      *tls.Conn
	rawConn      transportif.Conn
	localID      types.NodeID
	localPubKey  identity.PublicKey
	remoteID     types.NodeID
	remotePubKey identity.PublicKey
	state        types.ConnectionState

	closed   bool
	closedMu sync.RWMutex
}

// 确保 SecureConn 实现 securityif.SecureConn 接口
var _ securityif.SecureConn = (*SecureConn)(nil)

// NewSecureConn 创建安全连接
func NewSecureConn(
	tlsConn *tls.Conn,
	rawConn transportif.Conn,
	localID types.NodeID,
	localPubKey identity.PublicKey,
) (*SecureConn, error) {
	// 获取 TLS 连接状态
	tlsState := tlsConn.ConnectionState()

	// 提取远程 NodeID
	remoteID, err := ExtractNodeIDFromTLSState(tlsState)
	if err != nil {
		return nil, fmt.Errorf("提取远程 NodeID 失败: %w", err)
	}

	// 提取远程公钥
	var remotePubKey identity.PublicKey
	if len(tlsState.PeerCertificates) > 0 {
		remotePubKey = &tlsPublicKey{
			cert: tlsState.PeerCertificates[0],
		}
	}

	// 构建连接状态
	state := buildConnectionState(tlsState)

	return &SecureConn{
		tlsConn:      tlsConn,
		rawConn:      rawConn,
		localID:      localID,
		localPubKey:  localPubKey,
		remoteID:     remoteID,
		remotePubKey: remotePubKey,
		state:        state,
	}, nil
}

// buildConnectionState 从 TLS 状态构建连接状态
func buildConnectionState(tlsState tls.ConnectionState) types.ConnectionState {
	// 获取 TLS 版本字符串
	var version string
	switch tlsState.Version {
	case tls.VersionTLS10:
		version = "1.0"
	case tls.VersionTLS11:
		version = "1.1"
	case tls.VersionTLS12:
		version = "1.2"
	case tls.VersionTLS13:
		version = "1.3"
	default:
		version = fmt.Sprintf("0x%04x", tlsState.Version)
	}

	// 收集对端证书
	var peerCerts [][]byte
	for _, cert := range tlsState.PeerCertificates {
		peerCerts = append(peerCerts, cert.Raw)
	}

	return types.ConnectionState{
		Protocol:         "tls",
		Version:          version,
		CipherSuite:      tls.CipherSuiteName(tlsState.CipherSuite),
		PeerCertificates: peerCerts,
		DidResume:        tlsState.DidResume,
	}
}

// ============================================================================
//                              io.ReadWriteCloser 实现
// ============================================================================

// Read 从连接读取数据
func (c *SecureConn) Read(p []byte) (int, error) {
	return c.tlsConn.Read(p)
}

// Write 向连接写入数据
func (c *SecureConn) Write(p []byte) (int, error) {
	return c.tlsConn.Write(p)
}

// Close 关闭连接
func (c *SecureConn) Close() error {
	c.closedMu.Lock()
	defer c.closedMu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	// 先关闭 TLS 连接，记录错误但不提前返回
	tlsErr := c.tlsConn.Close()

	// 始终尝试关闭底层连接，防止资源泄漏
	// 注意：TLS conn 关闭时可能已经关闭了底层连接，
	// 所以这里可能会返回 "use of closed network connection" 错误
	rawErr := c.rawConn.Close()

	// 返回第一个遇到的错误
	// 如果 TLS 关闭成功，忽略 raw conn 的 "already closed" 错误
	if tlsErr != nil {
		return tlsErr
	}
	// 如果 raw conn 返回 "use of closed" 错误，这是预期的（TLS 已经关闭了它）
	if rawErr != nil && isClosedConnError(rawErr) {
		return nil
	}
	return rawErr
}

// isClosedConnError 检查是否是连接已关闭的错误
func isClosedConnError(err error) bool {
	if err == nil {
		return false
	}
	// 检查常见的连接已关闭错误消息
	errStr := err.Error()
	return strings.Contains(errStr, "use of closed network connection") ||
		strings.Contains(errStr, "connection reset by peer") ||
		strings.Contains(errStr, "broken pipe")
}

// ============================================================================
//                              transportif.Conn 实现
// ============================================================================

// LocalAddr 返回本地地址
func (c *SecureConn) LocalAddr() endpoint.Address {
	return c.rawConn.LocalAddr()
}

// RemoteAddr 返回远程地址
func (c *SecureConn) RemoteAddr() endpoint.Address {
	return c.rawConn.RemoteAddr()
}

// LocalNetAddr 返回底层本地 net.Addr
func (c *SecureConn) LocalNetAddr() net.Addr {
	return c.tlsConn.LocalAddr()
}

// RemoteNetAddr 返回底层远程 net.Addr
func (c *SecureConn) RemoteNetAddr() net.Addr {
	return c.tlsConn.RemoteAddr()
}

// SetDeadline 设置读写超时
func (c *SecureConn) SetDeadline(t time.Time) error {
	return c.tlsConn.SetDeadline(t)
}

// SetReadDeadline 设置读超时
func (c *SecureConn) SetReadDeadline(t time.Time) error {
	return c.tlsConn.SetReadDeadline(t)
}

// SetWriteDeadline 设置写超时
func (c *SecureConn) SetWriteDeadline(t time.Time) error {
	return c.tlsConn.SetWriteDeadline(t)
}

// IsClosed 检查连接是否已关闭
func (c *SecureConn) IsClosed() bool {
	c.closedMu.RLock()
	defer c.closedMu.RUnlock()
	return c.closed
}

// Transport 返回传输协议名称
func (c *SecureConn) Transport() string {
	return c.rawConn.Transport()
}

// ============================================================================
//                              securityif.SecureConn 实现
// ============================================================================

// LocalIdentity 返回本地节点 ID
func (c *SecureConn) LocalIdentity() types.NodeID {
	return c.localID
}

// LocalPublicKey 返回本地公钥
func (c *SecureConn) LocalPublicKey() identity.PublicKey {
	return c.localPubKey
}

// RemoteIdentity 返回远程节点 ID
func (c *SecureConn) RemoteIdentity() types.NodeID {
	return c.remoteID
}

// RemotePublicKey 返回远程公钥
func (c *SecureConn) RemotePublicKey() identity.PublicKey {
	return c.remotePubKey
}

// ConnectionState 返回连接状态
func (c *SecureConn) ConnectionState() types.ConnectionState {
	return c.state
}

// TLSConnectionState 返回原始 TLS 连接状态
func (c *SecureConn) TLSConnectionState() tls.ConnectionState {
	return c.tlsConn.ConnectionState()
}

// ============================================================================
//                              tlsPublicKey 适配器
// ============================================================================

// tlsPublicKey 从 TLS 证书提取的公钥
type tlsPublicKey struct {
	cert *x509.Certificate
}

// 确保实现 identity.PublicKey 接口
var _ identity.PublicKey = (*tlsPublicKey)(nil)

// Bytes 返回公钥的字节表示
func (k *tlsPublicKey) Bytes() []byte {
	if k.cert == nil {
		return nil
	}
	der, err := x509.MarshalPKIXPublicKey(k.cert.PublicKey)
	if err != nil {
		return nil
	}
	return der
}

// Equal 比较两个公钥是否相等
func (k *tlsPublicKey) Equal(other identity.PublicKey) bool {
	if other == nil {
		return k == nil || k.cert == nil
	}
	// 使用 Bytes() 比较，两个 nil cert 都返回 nil，bytes.Equal(nil, nil) == true
	return bytes.Equal(k.Bytes(), other.Bytes())
}

// Verify 使用公钥验证签名
// 返回值语义：
// - (true, nil): 签名验证成功
// - (false, nil): 签名验证失败（签名无效）
// - (false, error): 无法执行验证（操作不支持或发生错误）
func (k *tlsPublicKey) Verify(_, _ []byte) (bool, error) {
	if k.cert == nil {
		return false, fmt.Errorf("证书为空，无法验证签名")
	}

	// TLS 证书的公钥通常用于握手加密，不直接用于消息签名验证
	// 如果需要签名验证，应该使用 identity 模块的公钥
	// 这里返回操作不支持的错误
	return false, fmt.Errorf("TLS 证书公钥不支持直接签名验证，请使用 identity 模块的公钥")
}

// Type 返回密钥类型
func (k *tlsPublicKey) Type() types.KeyType {
	if k.cert == nil {
		return types.KeyTypeUnknown
	}
	switch k.cert.PublicKeyAlgorithm {
	case x509.RSA:
		return types.KeyTypeRSA
	case x509.ECDSA:
		return types.KeyTypeECDSA
	case x509.Ed25519:
		return types.KeyTypeEd25519
	default:
		return types.KeyTypeUnknown
	}
}

// Raw 返回底层公钥
func (k *tlsPublicKey) Raw() stdcrypto.PublicKey {
	if k.cert == nil {
		return nil
	}
	return k.cert.PublicKey
}


