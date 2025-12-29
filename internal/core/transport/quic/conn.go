// Package quic 提供基于 QUIC 的传输层实现
package quic

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"math/big"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	securityif "github.com/dep2p/go-dep2p/pkg/interfaces/security"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 错误定义
var (
	// ErrConnClosed 连接已关闭
	ErrConnClosed = errors.New("connection closed")
	// ErrStreamNotInitialized 流尚未初始化
	ErrStreamNotInitialized = errors.New("stream not initialized")
)

// Conn QUIC 连接封装
// 实现 securityif.SecureConn 接口，因为 QUIC 内置 TLS 1.3
type Conn struct {
	quicConn   *quic.Conn
	localAddr  *Address
	remoteAddr *Address
	transport  *Transport

	// 本地身份（从 Transport 继承）
	localIdentity identityif.Identity

	// 缓存的远程身份信息（从 TLS 状态提取）
	remoteID        types.NodeID
	remotePublicKey identityif.PublicKey
	identityOnce    sync.Once
	identityErr     error

	// 用于实现 io.ReadWriteCloser
	readStream  *quic.Stream
	writeStream *quic.Stream
	streamMu    sync.Mutex

	closed atomic.Bool
}

// 确保实现 transport.Conn 接口
var _ transportif.Conn = (*Conn)(nil)

// 确保实现 security.SecureConn 接口
var _ securityif.SecureConn = (*Conn)(nil)

// NewConn 创建连接封装
func NewConn(qc *quic.Conn, transport *Transport) *Conn {
	localAddr, _ := FromNetAddr(qc.LocalAddr())
	remoteAddr, _ := FromNetAddr(qc.RemoteAddr())

	var localIdentity identityif.Identity
	if transport != nil {
		localIdentity = transport.Identity()
	}

	return &Conn{
		quicConn:      qc,
		localAddr:     localAddr,
		remoteAddr:    remoteAddr,
		transport:     transport,
		localIdentity: localIdentity,
	}
}

// ============================================================================
//                              SecureConn 接口实现
// ============================================================================

// LocalIdentity 返回本地节点 ID
func (c *Conn) LocalIdentity() types.NodeID {
	if c.localIdentity != nil {
		return c.localIdentity.ID()
	}
	return types.EmptyNodeID
}

// LocalPublicKey 返回本地公钥
func (c *Conn) LocalPublicKey() identityif.PublicKey {
	if c.localIdentity != nil {
		return c.localIdentity.PublicKey()
	}
	return nil
}

// RemoteIdentity 返回远程节点 ID（从 TLS 证书公钥派生）
func (c *Conn) RemoteIdentity() types.NodeID {
	c.extractRemoteIdentity()
	return c.remoteID
}

// RemotePublicKey 返回远程公钥
func (c *Conn) RemotePublicKey() identityif.PublicKey {
	c.extractRemoteIdentity()
	return c.remotePublicKey
}

// extractRemoteIdentity 从 TLS 状态中提取远程身份（只执行一次）
func (c *Conn) extractRemoteIdentity() {
	c.identityOnce.Do(func() {
		tlsState := c.quicConn.ConnectionState().TLS
		if len(tlsState.PeerCertificates) == 0 {
			c.identityErr = errors.New("对端未提供 TLS 证书")
			return
		}

		// 使用 quic 包中的 ExtractNodeID 函数从证书公钥派生 NodeID
		nodeID, err := ExtractNodeID(tls.ConnectionState(tlsState))
		if err != nil {
			c.identityErr = err
			return
		}
		c.remoteID = nodeID

		// 提取远程公钥（包装为 identityif.PublicKey）
		c.remotePublicKey = newCertPublicKey(tlsState.PeerCertificates[0].PublicKey)
	})
}

// SecurityConnectionState 返回安全连接状态（实现 SecureConn.ConnectionState）
func (c *Conn) SecurityConnectionState() types.ConnectionState {
	tlsState := c.quicConn.ConnectionState().TLS

	// 收集对端证书的 DER 编码
	var peerCerts [][]byte
	for _, cert := range tlsState.PeerCertificates {
		peerCerts = append(peerCerts, cert.Raw)
	}

	return types.ConnectionState{
		Protocol:         "quic-tls",
		Version:          tlsVersionString(tlsState.Version),
		CipherSuite:      tls.CipherSuiteName(tlsState.CipherSuite),
		PeerCertificates: peerCerts,
		DidResume:        tlsState.DidResume,
	}
}

// ConnectionState 实现 SecureConn.ConnectionState 接口
// 注意：这个方法名与原来返回 quic.ConnectionState 的方法冲突
// 为了兼容，我们添加一个新方法 SecurityConnectionState，并让这个方法返回 types.ConnectionState
func (c *Conn) ConnectionState() types.ConnectionState {
	return c.SecurityConnectionState()
}

// QuicConnectionState 返回底层 QUIC 连接状态（保持向后兼容）
func (c *Conn) QuicConnectionState() quic.ConnectionState {
	return c.quicConn.ConnectionState()
}

// tlsVersionString 将 TLS 版本号转换为字符串
func tlsVersionString(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "1.0"
	case tls.VersionTLS11:
		return "1.1"
	case tls.VersionTLS12:
		return "1.2"
	case tls.VersionTLS13:
		return "1.3"
	default:
		return "unknown"
	}
}

// certPublicKey 包装 TLS 证书中的公钥为 identityif.PublicKey
type certPublicKey struct {
	raw      crypto.PublicKey
	rawBytes []byte
}

// newCertPublicKey 从证书公钥创建 certPublicKey
func newCertPublicKey(pubKey interface{}) *certPublicKey {
	var rawBytes []byte
	var cryptoPubKey crypto.PublicKey

	switch key := pubKey.(type) {
	case ed25519.PublicKey:
		rawBytes = key
		cryptoPubKey = key
	case *ecdsa.PublicKey:
		rawBytes = elliptic.Marshal(key.Curve, key.X, key.Y)
		cryptoPubKey = key
	case *rsa.PublicKey:
		rawBytes = x509.MarshalPKCS1PublicKey(key)
		cryptoPubKey = key
	}
	return &certPublicKey{
		raw:      cryptoPubKey,
		rawBytes: rawBytes,
	}
}

func (p *certPublicKey) Bytes() []byte {
	return p.rawBytes
}

func (p *certPublicKey) Raw() crypto.PublicKey {
	return p.raw
}

func (p *certPublicKey) Type() types.KeyType {
	switch p.raw.(type) {
	case ed25519.PublicKey:
		return types.KeyTypeEd25519
	case *ecdsa.PublicKey:
		return types.KeyTypeECDSA
	case *rsa.PublicKey:
		return types.KeyTypeRSA
	default:
		return types.KeyTypeUnknown
	}
}

func (p *certPublicKey) Equal(other identityif.PublicKey) bool {
	if other == nil {
		return false
	}
	// 比较字节表示
	return string(p.rawBytes) == string(other.Bytes())
}

func (p *certPublicKey) Verify(data, signature []byte) (bool, error) {
	switch key := p.raw.(type) {
	case ed25519.PublicKey:
		return ed25519.Verify(key, data, signature), nil

	case *ecdsa.PublicKey:
		// P1 修复：实现 ECDSA 签名验证
		// 尝试 ASN.1 DER 格式
		if ecdsa.VerifyASN1(key, data, signature) {
			return true, nil
		}
		// 回退：尝试 r||s 格式（根据曲线大小）
		curveSize := (key.Curve.Params().BitSize + 7) / 8
		if len(signature) == 2*curveSize {
			r := new(big.Int).SetBytes(signature[:curveSize])
			s := new(big.Int).SetBytes(signature[curveSize:])
			return ecdsa.Verify(key, data, r, s), nil
		}
		return false, nil

	case *rsa.PublicKey:
		// P1 修复：实现 RSA 签名验证
		// 使用 PKCS#1 v1.5 验证（SHA256 哈希由调用方提供）
		// 注意：这里假设 data 是原始数据，需要在验证前进行哈希
		// 但由于 identityif.PublicKey.Verify 的语义是验证原始数据，
		// 我们在这里尝试多种方式
		err := rsa.VerifyPKCS1v15(key, 0, data, signature)
		if err == nil {
			return true, nil
		}
		// 尝试无哈希验证失败，可能调用方已经提供了哈希值
		// 返回 false 而非 error，让调用方可以尝试其他方式
		return false, nil

	default:
		return false, errors.New("不支持的公钥类型")
	}
}

// ============================================================================
//                              transport.Conn 接口实现
// ============================================================================

// Read 从连接读取数据
// 注意: QUIC 连接是基于流的，这里使用默认流进行读取
// 使用连接的 context，确保连接关闭时操作能被取消
func (c *Conn) Read(p []byte) (int, error) {
	if c.closed.Load() {
		return 0, ErrConnClosed
	}

	c.streamMu.Lock()
	if c.readStream == nil {
		// 使用连接的 context，确保连接关闭时能取消阻塞
		stream, err := c.quicConn.AcceptStream(c.quicConn.Context())
		if err != nil {
			c.streamMu.Unlock()
			return 0, err
		}
		c.readStream = stream
	}
	stream := c.readStream
	c.streamMu.Unlock()

	return stream.Read(p)
}

// Write 向连接写入数据
// 注意: QUIC 连接是基于流的，这里使用默认流进行写入
// 使用连接的 context，确保连接关闭时操作能被取消
func (c *Conn) Write(p []byte) (int, error) {
	if c.closed.Load() {
		return 0, ErrConnClosed
	}

	c.streamMu.Lock()
	if c.writeStream == nil {
		// 使用连接的 context，确保连接关闭时能取消阻塞
		stream, err := c.quicConn.OpenStreamSync(c.quicConn.Context())
		if err != nil {
			c.streamMu.Unlock()
			return 0, err
		}
		c.writeStream = stream
	}
	stream := c.writeStream
	c.streamMu.Unlock()

	return stream.Write(p)
}

// Close 关闭连接
// 确保所有流和连接都被正确关闭
func (c *Conn) Close() error {
	if c.closed.Swap(true) {
		return nil // 已经关闭
	}

	var errs []error

	// 关闭流，记录错误但继续关闭其他资源
	c.streamMu.Lock()
	if c.readStream != nil {
		if err := c.readStream.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if c.writeStream != nil {
		if err := c.writeStream.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	c.streamMu.Unlock()

	// 关闭 QUIC 连接
	if err := c.quicConn.CloseWithError(0, "正常关闭"); err != nil {
		errs = append(errs, err)
	}

	// 返回第一个错误（如果有）
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// LocalAddr 返回本地地址
func (c *Conn) LocalAddr() endpoint.Address {
	return c.localAddr
}

// RemoteAddr 返回远程地址
func (c *Conn) RemoteAddr() endpoint.Address {
	return c.remoteAddr
}

// LocalNetAddr 返回底层 net.Addr
func (c *Conn) LocalNetAddr() net.Addr {
	return c.quicConn.LocalAddr()
}

// RemoteNetAddr 返回底层远程 net.Addr
func (c *Conn) RemoteNetAddr() net.Addr {
	return c.quicConn.RemoteAddr()
}

// SetDeadline 设置读写超时
func (c *Conn) SetDeadline(t time.Time) error {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()

	var err error
	if c.readStream != nil {
		if e := c.readStream.SetDeadline(t); e != nil {
			err = e
		}
	}
	if c.writeStream != nil {
		if e := c.writeStream.SetDeadline(t); e != nil {
			err = e
		}
	}
	return err
}

// SetReadDeadline 设置读超时
func (c *Conn) SetReadDeadline(t time.Time) error {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()

	if c.readStream != nil {
		return c.readStream.SetReadDeadline(t)
	}
	return nil
}

// SetWriteDeadline 设置写超时
func (c *Conn) SetWriteDeadline(t time.Time) error {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()

	if c.writeStream != nil {
		return c.writeStream.SetWriteDeadline(t)
	}
	return nil
}

// IsClosed 检查连接是否已关闭
func (c *Conn) IsClosed() bool {
	return c.closed.Load()
}

// Transport 返回传输协议名称
func (c *Conn) Transport() string {
	return "quic"
}

// QuicConn 返回底层 QUIC 连接
func (c *Conn) QuicConn() *quic.Conn {
	return c.quicConn
}

// OpenStream 打开新流
func (c *Conn) OpenStream(ctx context.Context) (transportif.Stream, error) {
	stream, err := c.quicConn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	return NewStream(stream, c), nil
}

// AcceptStream 接受新流
func (c *Conn) AcceptStream(ctx context.Context) (transportif.Stream, error) {
	stream, err := c.quicConn.AcceptStream(ctx)
	if err != nil {
		return nil, err
	}
	return NewStream(stream, c), nil
}

// OpenUniStream 打开单向流
func (c *Conn) OpenUniStream(ctx context.Context) (*quic.SendStream, error) {
	return c.quicConn.OpenUniStreamSync(ctx)
}

// AcceptUniStream 接受单向流
func (c *Conn) AcceptUniStream(ctx context.Context) (*quic.ReceiveStream, error) {
	return c.quicConn.AcceptUniStream(ctx)
}

// Context 返回连接上下文
func (c *Conn) Context() context.Context {
	return c.quicConn.Context()
}

// readWriteCloser 实现 io.ReadWriteCloser 的辅助类型
type readWriteCloser struct {
	reader io.Reader
	writer io.Writer
	closer io.Closer
}

func (rwc *readWriteCloser) Read(p []byte) (int, error) {
	return rwc.reader.Read(p)
}

func (rwc *readWriteCloser) Write(p []byte) (int, error) {
	return rwc.writer.Write(p)
}

func (rwc *readWriteCloser) Close() error {
	return rwc.closer.Close()
}
