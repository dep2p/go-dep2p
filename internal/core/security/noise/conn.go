package noise

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	noiselib "github.com/flynn/noise"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	cryptoif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	securityif "github.com/dep2p/go-dep2p/pkg/interfaces/security"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 常量定义
const (
	// MaxPlaintextSize 最大明文大小
	// Noise 协议限制为 65535 - 16 (AEAD tag)
	MaxPlaintextSize = 65535 - 16

	// LengthPrefixSize 长度前缀大小
	LengthPrefixSize = 2

	// ReadBufferSize 读缓冲区大小
	ReadBufferSize = 65535
)

// 错误定义
var (
	// ErrClosed 连接已关闭
	ErrClosed = errors.New("connection closed")

	// ErrPlaintextTooLarge 明文过大
	ErrPlaintextTooLarge = errors.New("plaintext too large")

	// ErrDecryptionFailed 解密失败
	ErrDecryptionFailed = errors.New("decryption failed")
)

// SecureConn Noise 加密连接
//
// 实现 securityif.SecureConn 接口，提供加密通信能力。
//
// Identity 绑定：
//   - LocalIdentity()/RemoteIdentity() 返回 dep2p identity 派生的 NodeID
//   - LocalPublicKey()/RemotePublicKey() 返回 dep2p identity 公钥（如果有绑定）
//   - Noise DH 公钥作为内部实现细节，不对外暴露
type SecureConn struct {
	// 底层连接
	rawConn transportif.Conn

	// 加密状态
	sendCipher *noiselib.CipherState
	recvCipher *noiselib.CipherState

	// 身份信息（identity 层面）
	localID       types.NodeID
	localPubKey   cryptoif.PublicKey // identity 公钥
	remoteID      types.NodeID
	remotePubKey  cryptoif.PublicKey // identity 公钥（如果有绑定）

	// Noise DH 密钥（内部使用）
	localNoiseKey  []byte
	remoteNoiseKey []byte

	// 读缓冲区
	readBuf   []byte
	readStart int
	readEnd   int

	// 同步
	readMu  sync.Mutex
	writeMu sync.Mutex

	// 状态
	closed bool
}

// 确保实现 securityif.SecureConn 接口
var _ securityif.SecureConn = (*SecureConn)(nil)

// NewSecureConn 创建加密连接（向后兼容，不推荐使用）
//
// Deprecated: 使用 NewSecureConnWithIdentity 替代
func NewSecureConn(
	rawConn transportif.Conn,
	result *HandshakeResult,
	localID types.NodeID,
	localPubKey []byte,
) *SecureConn {
	remoteNoiseKey := result.NoiseRemotePubKey
	if remoteNoiseKey == nil {
		remoteNoiseKey = result.RemotePubKey // 兼容旧字段
	}

	return &SecureConn{
		rawConn:        rawConn,
		sendCipher:     result.SendCipher,
		recvCipher:     result.RecvCipher,
		localID:        localID,
		localPubKey:    nil, // 无 identity 公钥
		remoteID:       result.RemoteID,
		remotePubKey:   result.RemoteIdentityPubKey, // 可能为 nil
		localNoiseKey:  localPubKey,
		remoteNoiseKey: remoteNoiseKey,
		readBuf:        make([]byte, ReadBufferSize),
	}
}

// NewSecureConnWithIdentity 创建带 identity 绑定的加密连接
func NewSecureConnWithIdentity(
	rawConn transportif.Conn,
	result *HandshakeResult,
	localID types.NodeID,
	localPubKey cryptoif.PublicKey,
) *SecureConn {
	remoteNoiseKey := result.NoiseRemotePubKey
	if remoteNoiseKey == nil {
		remoteNoiseKey = result.RemotePubKey // 兼容旧字段
	}

	return &SecureConn{
		rawConn:        rawConn,
		sendCipher:     result.SendCipher,
		recvCipher:     result.RecvCipher,
		localID:        localID,
		localPubKey:    localPubKey,
		remoteID:       result.RemoteID,
		remotePubKey:   result.RemoteIdentityPubKey, // 可能为 nil（对方无绑定）
		localNoiseKey:  nil,                         // 不再暴露
		remoteNoiseKey: remoteNoiseKey,
		readBuf:        make([]byte, ReadBufferSize),
	}
}

// Read 读取解密数据
func (c *SecureConn) Read(b []byte) (int, error) {
	c.readMu.Lock()
	defer c.readMu.Unlock()

	if c.closed {
		return 0, ErrClosed
	}

	// 如果缓冲区有数据，先返回缓冲区数据
	if c.readStart < c.readEnd {
		n := copy(b, c.readBuf[c.readStart:c.readEnd])
		c.readStart += n
		return n, nil
	}

	// 读取并解密新数据
	plaintext, err := c.readAndDecrypt()
	if err != nil {
		return 0, err
	}

	// 直接返回给调用者
	n := copy(b, plaintext)
	if n < len(plaintext) {
		// 剩余数据放入缓冲区
		copy(c.readBuf, plaintext[n:])
		c.readStart = 0
		c.readEnd = len(plaintext) - n
	}

	return n, nil
}

// Write 加密并写入数据
func (c *SecureConn) Write(b []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if c.closed {
		return 0, ErrClosed
	}

	totalWritten := 0
	remaining := b

	// 分片写入，确保不超过最大明文大小
	for len(remaining) > 0 {
		chunkSize := len(remaining)
		if chunkSize > MaxPlaintextSize {
			chunkSize = MaxPlaintextSize
		}

		chunk := remaining[:chunkSize]
		if err := c.encryptAndWrite(chunk); err != nil {
			return totalWritten, err
		}

		totalWritten += chunkSize
		remaining = remaining[chunkSize:]
	}

	return totalWritten, nil
}

// MinCiphertextSize 最小密文大小（AEAD tag 16 字节）
const MinCiphertextSize = 16

// readAndDecrypt 读取并解密一个消息
func (c *SecureConn) readAndDecrypt() ([]byte, error) {
	// 读取长度前缀
	var lenBuf [LengthPrefixSize]byte
	if _, err := io.ReadFull(c.rawConn, lenBuf[:]); err != nil {
		return nil, err
	}

	msgLen := binary.BigEndian.Uint16(lenBuf[:])

	// 验证消息长度：
	// - 零长度可能是恶意构造或连接关闭信号
	// - 有效的加密消息至少包含 AEAD tag (16 字节)
	if msgLen == 0 {
		return nil, io.EOF
	}
	if msgLen < MinCiphertextSize {
		return nil, fmt.Errorf("%w: 密文长度 %d 小于最小值 %d", ErrDecryptionFailed, msgLen, MinCiphertextSize)
	}

	// 读取密文
	ciphertext := make([]byte, msgLen)
	if _, err := io.ReadFull(c.rawConn, ciphertext); err != nil {
		return nil, err
	}

	// 解密
	plaintext, err := c.recvCipher.Decrypt(nil, nil, ciphertext)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

// encryptAndWrite 加密并写入一个消息
func (c *SecureConn) encryptAndWrite(plaintext []byte) error {
	if len(plaintext) > MaxPlaintextSize {
		return ErrPlaintextTooLarge
	}

	// 加密
	ciphertext, err := c.sendCipher.Encrypt(nil, nil, plaintext)
	if err != nil {
		return err
	}

	// 写入长度前缀
	var lenBuf [LengthPrefixSize]byte
	binary.BigEndian.PutUint16(lenBuf[:], uint16(len(ciphertext))) //nolint:gosec // G115: 密文长度由协议限制

	if _, err := c.rawConn.Write(lenBuf[:]); err != nil {
		return err
	}

	// 写入密文
	if _, err := c.rawConn.Write(ciphertext); err != nil {
		return err
	}

	return nil
}

// Close 关闭连接
func (c *SecureConn) Close() error {
	c.readMu.Lock()
	c.writeMu.Lock()
	defer c.readMu.Unlock()
	defer c.writeMu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	return c.rawConn.Close()
}

// LocalAddr 返回本地地址
func (c *SecureConn) LocalAddr() endpoint.Address {
	return c.rawConn.LocalAddr()
}

// RemoteAddr 返回远程地址
func (c *SecureConn) RemoteAddr() endpoint.Address {
	return c.rawConn.RemoteAddr()
}

// LocalNetAddr 返回本地网络地址
func (c *SecureConn) LocalNetAddr() net.Addr {
	return c.rawConn.LocalNetAddr()
}

// RemoteNetAddr 返回远程网络地址
func (c *SecureConn) RemoteNetAddr() net.Addr {
	return c.rawConn.RemoteNetAddr()
}

// SetDeadline 设置超时
func (c *SecureConn) SetDeadline(t time.Time) error {
	return c.rawConn.SetDeadline(t)
}

// SetReadDeadline 设置读超时
func (c *SecureConn) SetReadDeadline(t time.Time) error {
	return c.rawConn.SetReadDeadline(t)
}

// SetWriteDeadline 设置写超时
func (c *SecureConn) SetWriteDeadline(t time.Time) error {
	return c.rawConn.SetWriteDeadline(t)
}

// IsClosed 检查连接是否已关闭
func (c *SecureConn) IsClosed() bool {
	c.readMu.Lock()
	defer c.readMu.Unlock()
	return c.closed
}

// LocalIdentity 返回本地节点 ID
//
// 返回 dep2p identity 派生的 NodeID，与 TLS 模式下的 NodeID 一致。
func (c *SecureConn) LocalIdentity() types.NodeID {
	return c.localID
}

// LocalPublicKey 返回本地 identity 公钥
//
// 返回 dep2p identity 公钥，可用于签名验证。
// 如果未设置 identity，返回 nil。
func (c *SecureConn) LocalPublicKey() cryptoif.PublicKey {
	return c.localPubKey
}

// RemoteIdentity 返回远程节点 ID
//
// 返回远程 dep2p identity 派生的 NodeID。
// 如果对方提供了 identity 绑定，此值与 TLS 模式下的 NodeID 一致。
// 如果对方未提供 identity 绑定，此值为 Noise DH 公钥的哈希（不推荐）。
func (c *SecureConn) RemoteIdentity() types.NodeID {
	return c.remoteID
}

// RemotePublicKey 返回远程 identity 公钥
//
// 返回远程 dep2p identity 公钥，可用于签名验证。
// 如果对方未提供 identity 绑定，返回 nil。
func (c *SecureConn) RemotePublicKey() cryptoif.PublicKey {
	return c.remotePubKey
}

// HasRemoteIdentityBinding 检查对方是否提供了 identity 绑定
func (c *SecureConn) HasRemoteIdentityBinding() bool {
	return c.remotePubKey != nil
}

// NoiseRemotePublicKey 返回远程 Noise 静态公钥（内部使用）
//
// 这是 Curve25519 DH 密钥，不是 identity 公钥。
// 仅用于调试或特殊场景，一般不需要使用。
func (c *SecureConn) NoiseRemotePublicKey() []byte {
	if c.remoteNoiseKey == nil {
		return nil
	}
	result := make([]byte, len(c.remoteNoiseKey))
	copy(result, c.remoteNoiseKey)
	return result
}

// ConnectionState 返回连接状态
func (c *SecureConn) ConnectionState() types.ConnectionState {
	return types.ConnectionState{
		Protocol:    "noise",
		Version:     "1.0",
		CipherSuite: "Noise_XX_25519_ChaChaPoly_SHA256",
	}
}

// Transport 返回传输协议名称
func (c *SecureConn) Transport() string {
	return "noise"
}
