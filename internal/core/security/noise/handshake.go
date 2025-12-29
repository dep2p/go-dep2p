// Package noise 提供基于 Noise Protocol 的安全传输实现
//
// Noise Protocol 是一种用于建立安全通信通道的协议框架，
// 相比 TLS 更加轻量，特别适合 P2P 场景。
//
// 支持的握手模式：
//   - XX: 双向认证，双方都不知道对方身份
//   - IK: 发起者已知响应者身份
//   - NK: 仅验证响应者身份
//
// Identity 绑定：
//   通过在握手 payload 中携带 identity 公钥和签名，实现 Noise
//   静态密钥与 dep2p identity 的强绑定，确保 RemoteIdentity() 返回
//   的是真正的 identity NodeID，而非 Noise DH 公钥的哈希。
//
// 参考：https://noiseprotocol.org/noise.html
package noise

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/flynn/noise"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	securityif "github.com/dep2p/go-dep2p/pkg/interfaces/security"
	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

var log = logger.Logger("security.noise")

// 引用 pkg/protocolids 唯一真源
var (
	// NoiseProtocolID 协议标识 (v1.1 scope: sys)
	NoiseProtocolID = protocolids.SysNoise
)

// 常量定义
const (

	// MaxHandshakeMessageSize 最大握手消息大小
	MaxHandshakeMessageSize = 65535

	// HandshakeMessagePrefix 握手消息前缀长度 (2 bytes for length)
	HandshakeMessagePrefix = 2

	// NoiseKeySize Curve25519 密钥大小
	NoiseKeySize = 32
)

// 错误定义
var (
	// ErrInvalidHandshakePattern 无效的握手模式
	ErrInvalidHandshakePattern = errors.New("invalid handshake pattern")

	// ErrHandshakeTimeout 握手超时
	ErrHandshakeTimeout = errors.New("handshake timeout")

	// ErrHandshakeFailed 握手失败
	ErrHandshakeFailed = errors.New("handshake failed")

	// ErrInvalidPublicKey 无效的公钥
	ErrInvalidPublicKey = errors.New("invalid public key")

	// ErrMessageTooLarge 消息过大
	ErrMessageTooLarge = errors.New("message too large")

	// ErrRemoteIdentityMismatch 远程身份不匹配
	ErrRemoteIdentityMismatch = errors.New("remote identity mismatch")

	// ErrIdentityBindingRequired identity 绑定必需
	ErrIdentityBindingRequired = errors.New("identity binding required for dep2p noise")
)

// Handshaker 处理 Noise 握手
type Handshaker struct {
	config      *securityif.NoiseConfig
	localKeys   noise.DHKey
	cipherSuite noise.CipherSuite
	identity    identityif.Identity   // dep2p identity（用于 identity 绑定）
	keyFactory  identityif.KeyFactory // 密钥工厂（用于验证远程 identity）
}

// NewHandshaker 创建握手处理器
func NewHandshaker(config *securityif.NoiseConfig) (*Handshaker, error) {
	return NewHandshakerWithDeps(config, nil, nil)
}

// NewHandshakerWithIdentity 创建带 identity 绑定的握手处理器
//
// Deprecated: 使用 NewHandshakerWithDeps 代替，以支持 KeyFactory 注入。
func NewHandshakerWithIdentity(config *securityif.NoiseConfig, identity identityif.Identity) (*Handshaker, error) {
	return NewHandshakerWithDeps(config, identity, nil)
}

// NewHandshakerWithDeps 创建带完整依赖的握手处理器
//
// identity: dep2p identity（用于 identity 绑定，可选）
// keyFactory: 密钥工厂（用于验证远程 identity，可选）
func NewHandshakerWithDeps(config *securityif.NoiseConfig, identity identityif.Identity, keyFactory identityif.KeyFactory) (*Handshaker, error) {
	if config == nil {
		config = securityif.DefaultNoiseConfig()
	}

	// 选择加密套件
	cipherSuite, err := selectCipherSuite(config)
	if err != nil {
		return nil, err
	}

	// 生成或使用提供的密钥对
	var localKeys noise.DHKey
	if config.StaticKeypair != nil {
		localKeys = noise.DHKey{
			Public:  config.StaticKeypair.PublicKey,
			Private: config.StaticKeypair.PrivateKey,
		}
	} else {
		localKeys, err = cipherSuite.GenerateKeypair(nil)
		if err != nil {
			return nil, fmt.Errorf("生成密钥对失败: %w", err)
		}
	}

	return &Handshaker{
		config:      config,
		localKeys:   localKeys,
		cipherSuite: cipherSuite,
		identity:    identity,
		keyFactory:  keyFactory,
	}, nil
}

// HandshakeAsInitiator 作为发起者执行握手
//
// 如果设置了 identity，会在握手消息中携带 identity 绑定信息，
// 并验证对方的 identity 绑定。
func (h *Handshaker) HandshakeAsInitiator(
	rw io.ReadWriter,
	expectedRemote types.NodeID,
	deadline time.Time,
) (*HandshakeResult, error) {
	log.Debug("开始 Noise 握手 (发起者)",
		"hasIdentity", h.identity != nil)

	// 获取握手模式
	pattern, err := h.getHandshakePattern()
	if err != nil {
		return nil, err
	}

	// 创建握手状态
	hs, err := noise.NewHandshakeState(noise.Config{
		CipherSuite:   h.cipherSuite,
		Pattern:       pattern,
		Initiator:     true,
		StaticKeypair: h.localKeys,
		Prologue:      h.config.Prologue,
	})
	if err != nil {
		return nil, fmt.Errorf("创建握手状态失败: %w", err)
	}

	// 准备本地 identity 绑定 payload（如果有 identity）
	var localBindingPayload []byte
	if h.identity != nil {
		localBindingPayload, err = EncodeIdentityBindingPayload(h.identity, h.localKeys.Public)
		if err != nil {
			return nil, fmt.Errorf("编码 identity 绑定失败: %w", err)
		}
	}

	// XX 模式握手流程:
	// -> e
	// <- e, ee, s, es [+ identity binding payload]
	// -> s, se [+ identity binding payload]

	// 消息 1: -> e
	msg1, _, _, err := hs.WriteMessage(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("生成握手消息1失败: %w", err)
	}

	if err := h.writeMessage(rw, msg1, deadline); err != nil {
		return nil, fmt.Errorf("发送握手消息1失败: %w", err)
	}

	// 消息 2: <- e, ee, s, es [+ identity binding payload]
	msg2, err := h.readMessage(rw, deadline)
	if err != nil {
		return nil, fmt.Errorf("读取握手消息2失败: %w", err)
	}

	remotePayload2, _, _, err := hs.ReadMessage(nil, msg2)
	if err != nil {
		return nil, fmt.Errorf("处理握手消息2失败: %w", err)
	}

	// 消息 3: -> s, se [+ identity binding payload]
	msg3, cs1, cs2, err := hs.WriteMessage(nil, localBindingPayload)
	if err != nil {
		return nil, fmt.Errorf("生成握手消息3失败: %w", err)
	}

	if err := h.writeMessage(rw, msg3, deadline); err != nil {
		return nil, fmt.Errorf("发送握手消息3失败: %w", err)
	}

	// 获取远程 Noise 公钥
	remotePubKey := hs.PeerStatic()
	if len(remotePubKey) != NoiseKeySize {
		return nil, ErrInvalidPublicKey
	}

	// 处理 identity 绑定
	result := &HandshakeResult{
		NoiseRemotePubKey: remotePubKey,
		SendCipher:        cs1,
		RecvCipher:        cs2,
	}

	// 如果对方发送了 identity 绑定 payload，验证它
	if len(remotePayload2) > 0 {
		binding, err := DecodeAndVerifyIdentityBindingPayload(remotePayload2, remotePubKey, h.keyFactory)
		if err != nil {
			return nil, fmt.Errorf("验证远程 identity 绑定失败: %w", err)
		}
		result.RemoteID = binding.NodeID
		result.RemoteIdentityPubKey = binding.PublicKey
		log.Debug("远程 identity 绑定验证成功",
			"remoteID", binding.NodeID.String())
	} else {
		// 无 identity 绑定，回退到 Noise 公钥派生（不推荐）
		result.RemoteID = NodeIDFromNoiseKey(remotePubKey)
		log.Warn("远程节点未提供 identity 绑定，使用 Noise 公钥派生 NodeID")
	}

	// 验证远程身份
	if !expectedRemote.IsEmpty() && !result.RemoteID.Equal(expectedRemote) {
		return nil, fmt.Errorf("%w: 期望 %s, 得到 %s",
			ErrRemoteIdentityMismatch, expectedRemote.String(), result.RemoteID.String())
	}

	log.Debug("Noise 握手完成 (发起者)",
		"remoteID", result.RemoteID.String())

	return result, nil
}

// HandshakeAsResponder 作为响应者执行握手
//
// 如果设置了 identity，会在握手消息中携带 identity 绑定信息，
// 并验证对方的 identity 绑定。
func (h *Handshaker) HandshakeAsResponder(
	rw io.ReadWriter,
	deadline time.Time,
) (*HandshakeResult, error) {
	log.Debug("开始 Noise 握手 (响应者)",
		"hasIdentity", h.identity != nil)

	// 获取握手模式
	pattern, err := h.getHandshakePattern()
	if err != nil {
		return nil, err
	}

	// 创建握手状态
	hs, err := noise.NewHandshakeState(noise.Config{
		CipherSuite:   h.cipherSuite,
		Pattern:       pattern,
		Initiator:     false,
		StaticKeypair: h.localKeys,
		Prologue:      h.config.Prologue,
	})
	if err != nil {
		return nil, fmt.Errorf("创建握手状态失败: %w", err)
	}

	// 准备本地 identity 绑定 payload（如果有 identity）
	var localBindingPayload []byte
	if h.identity != nil {
		localBindingPayload, err = EncodeIdentityBindingPayload(h.identity, h.localKeys.Public)
		if err != nil {
			return nil, fmt.Errorf("编码 identity 绑定失败: %w", err)
		}
	}

	// XX 模式握手流程:
	// -> e
	// <- e, ee, s, es [+ identity binding payload]
	// -> s, se [+ identity binding payload]

	// 消息 1: -> e
	msg1, err := h.readMessage(rw, deadline)
	if err != nil {
		return nil, fmt.Errorf("读取握手消息1失败: %w", err)
	}

	_, _, _, err = hs.ReadMessage(nil, msg1)
	if err != nil {
		return nil, fmt.Errorf("处理握手消息1失败: %w", err)
	}

	// 消息 2: <- e, ee, s, es [+ identity binding payload]
	msg2, _, _, err := hs.WriteMessage(nil, localBindingPayload)
	if err != nil {
		return nil, fmt.Errorf("生成握手消息2失败: %w", err)
	}

	if err := h.writeMessage(rw, msg2, deadline); err != nil {
		return nil, fmt.Errorf("发送握手消息2失败: %w", err)
	}

	// 消息 3: -> s, se [+ identity binding payload]
	msg3, err := h.readMessage(rw, deadline)
	if err != nil {
		return nil, fmt.Errorf("读取握手消息3失败: %w", err)
	}

	remotePayload3, cs2, cs1, err := hs.ReadMessage(nil, msg3)
	if err != nil {
		return nil, fmt.Errorf("处理握手消息3失败: %w", err)
	}

	// 获取远程 Noise 公钥
	remotePubKey := hs.PeerStatic()
	if len(remotePubKey) != NoiseKeySize {
		return nil, ErrInvalidPublicKey
	}

	// 处理 identity 绑定
	result := &HandshakeResult{
		NoiseRemotePubKey: remotePubKey,
		SendCipher:        cs1,
		RecvCipher:        cs2,
	}

	// 如果对方发送了 identity 绑定 payload，验证它
	if len(remotePayload3) > 0 {
		binding, err := DecodeAndVerifyIdentityBindingPayload(remotePayload3, remotePubKey, h.keyFactory)
		if err != nil {
			return nil, fmt.Errorf("验证远程 identity 绑定失败: %w", err)
		}
		result.RemoteID = binding.NodeID
		result.RemoteIdentityPubKey = binding.PublicKey
		log.Debug("远程 identity 绑定验证成功",
			"remoteID", binding.NodeID.String())
	} else {
		// 无 identity 绑定，回退到 Noise 公钥派生（不推荐）
		result.RemoteID = NodeIDFromNoiseKey(remotePubKey)
		log.Warn("远程节点未提供 identity 绑定，使用 Noise 公钥派生 NodeID")
	}

	log.Debug("Noise 握手完成 (响应者)",
		"remoteID", result.RemoteID.String())

	return result, nil
}

// getHandshakePattern 获取握手模式
func (h *Handshaker) getHandshakePattern() (noise.HandshakePattern, error) {
	switch h.config.HandshakePattern {
	case "XX", "":
		return noise.HandshakeXX, nil
	case "IK":
		return noise.HandshakeIK, nil
	case "NK":
		return noise.HandshakeNK, nil
	default:
		return noise.HandshakePattern{}, fmt.Errorf("%w: %s", ErrInvalidHandshakePattern, h.config.HandshakePattern)
	}
}

// writeMessage 写入带长度前缀的消息
// 将长度前缀和消息合并为单次写入，避免部分写入导致的协议不一致
func (h *Handshaker) writeMessage(w io.Writer, msg []byte, deadline time.Time) error {
	if len(msg) > MaxHandshakeMessageSize {
		return ErrMessageTooLarge
	}

	// 检查超时
	if !deadline.IsZero() && time.Now().After(deadline) {
		return ErrHandshakeTimeout
	}

	// 合并长度前缀和消息，确保原子写入
	buf := make([]byte, HandshakeMessagePrefix+len(msg))
	binary.BigEndian.PutUint16(buf[:HandshakeMessagePrefix], uint16(len(msg))) //nolint:gosec // G115: 消息大小由协议限制
	copy(buf[HandshakeMessagePrefix:], msg)

	// 单次写入
	_, err := w.Write(buf)
	return err
}

// readMessage 读取带长度前缀的消息
func (h *Handshaker) readMessage(r io.Reader, deadline time.Time) ([]byte, error) {
	// 检查超时
	if !deadline.IsZero() && time.Now().After(deadline) {
		return nil, ErrHandshakeTimeout
	}

	// 读取长度前缀
	var lenBuf [HandshakeMessagePrefix]byte
	if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
		return nil, err
	}

	msgLen := binary.BigEndian.Uint16(lenBuf[:])

	// 验证消息长度：Noise 握手消息不能为空
	if msgLen == 0 {
		return nil, fmt.Errorf("%w: 收到零长度握手消息", ErrHandshakeFailed)
	}

	if msgLen > MaxHandshakeMessageSize {
		return nil, ErrMessageTooLarge
	}

	// 读取消息
	msg := make([]byte, msgLen)
	if _, err := io.ReadFull(r, msg); err != nil {
		return nil, err
	}

	return msg, nil
}

// LocalPublicKey 返回本地 Noise 公钥
func (h *Handshaker) LocalPublicKey() []byte {
	return h.localKeys.Public
}

// LocalPrivateKey 返回本地 Noise 私钥
func (h *Handshaker) LocalPrivateKey() []byte {
	return h.localKeys.Private
}

// Identity 返回关联的 identity
func (h *Handshaker) Identity() identityif.Identity {
	return h.identity
}

// HandshakeResult 握手结果
type HandshakeResult struct {
	// RemoteID 远程节点 ID（从 identity 绑定派生，或回退到 Noise 公钥派生）
	RemoteID types.NodeID

	// RemoteIdentityPubKey 远程 identity 公钥（如果有 identity 绑定）
	RemoteIdentityPubKey identityif.PublicKey

	// NoiseRemotePubKey 远程 Noise 静态公钥（Curve25519）
	NoiseRemotePubKey []byte

	// SendCipher 发送加密器
	SendCipher *noise.CipherState

	// RecvCipher 接收加密器
	RecvCipher *noise.CipherState

	// RemotePubKey 已弃用，使用 NoiseRemotePubKey
	// Deprecated: 使用 NoiseRemotePubKey 替代
	RemotePubKey []byte
}

// selectCipherSuite 选择加密套件
func selectCipherSuite(config *securityif.NoiseConfig) (noise.CipherSuite, error) {
	// 选择密钥交换算法
	var dh noise.DHFunc
	switch config.DHCurve {
	case "25519", "":
		dh = noise.DH25519
	default:
		return nil, fmt.Errorf("不支持的 DH 曲线: %s", config.DHCurve)
	}

	// 选择加密算法
	var cipher noise.CipherFunc
	switch config.CipherSuite {
	case "ChaChaPoly", "":
		cipher = noise.CipherChaChaPoly
	case "AESGCM":
		cipher = noise.CipherAESGCM
	default:
		return nil, fmt.Errorf("不支持的加密套件: %s", config.CipherSuite)
	}

	// 选择哈希函数
	var hash noise.HashFunc
	switch config.HashFunction {
	case "SHA256", "":
		hash = noise.HashSHA256
	case "BLAKE2b":
		hash = noise.HashBLAKE2b
	case "BLAKE2s":
		hash = noise.HashBLAKE2s
	default:
		return nil, fmt.Errorf("不支持的哈希函数: %s", config.HashFunction)
	}

	return noise.NewCipherSuite(dh, cipher, hash), nil
}

// NodeIDFromNoiseKey 从 Noise 公钥派生 NodeID
//
// 使用公钥的 SHA-256 哈希作为 NodeID。
// 注意：这与 identity.NodeIDFromPublicKey 使用不同的输入格式，
// 因此结果不同。推荐使用 identity 绑定来获取一致的 NodeID。
func NodeIDFromNoiseKey(pubKey []byte) types.NodeID {
	if len(pubKey) != NoiseKeySize {
		return types.EmptyNodeID
	}

	// 使用 SHA-256 哈希计算 NodeID
	hash := sha256.Sum256(pubKey)
	return types.NodeID(hash)
}

// NoiseKeyFromNodeID 已废弃
//
// 由于 NodeID 现在是公钥的 SHA-256 哈希，无法从 NodeID 恢复公钥。
// 如果需要公钥，应该从其他来源（如证书、握手消息）获取。
//
// Deprecated: NodeID 是单向哈希，无法逆向恢复公钥
func NoiseKeyFromNodeID(_ types.NodeID) []byte {
	// 返回 nil 表示无法恢复
	return nil
}
