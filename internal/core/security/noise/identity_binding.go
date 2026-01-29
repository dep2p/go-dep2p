// Package noise 实现 Noise 协议安全传输
//
// IdentityBinding 提供身份绑定验证功能：
//   - 验证 PeerID 与公钥的绑定关系
//   - 创建和验证身份证明
//   - 防止身份伪造攻击
package noise

import (
	"bytes"
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/dep2p/go-dep2p/pkg/lib/crypto"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              错误定义
// ============================================================================

var (
	// ErrIdentityMismatch 身份不匹配错误
	ErrIdentityMismatch = errors.New("identity mismatch: peer ID does not match public key")

	// ErrInvalidProof 无效的身份证明
	ErrInvalidProof = errors.New("invalid identity proof")

	// ErrProofExpired 身份证明已过期
	ErrProofExpired = errors.New("identity proof expired")

	// ErrInvalidSignature 无效的签名
	ErrInvalidSignature = errors.New("invalid signature")

	// ErrInvalidPublicKey 无效的公钥
	ErrInvalidPublicKey = errors.New("invalid public key")
)

// ============================================================================
//                              常量定义
// ============================================================================

const (
	// ProofVersion 当前身份证明版本
	ProofVersion = 1

	// ProofValidityDuration 身份证明有效期（24小时）
	ProofValidityDuration = 24 * time.Hour

	// SignaturePrefix 签名前缀，防止跨协议重放攻击
	SignaturePrefix = "dep2p-noise-identity-proof-v1:"

	// MinProofSize 最小证明大小（版本 + 时间戳 + 公钥 + 签名）
	MinProofSize = 1 + 8 + 32 + 64
)

// ============================================================================
//                              IdentityBinding 结构
// ============================================================================

// IdentityBinding 身份绑定验证器
//
// 用于在 Noise 握手中验证 PeerID 与公钥的绑定关系。
// 提供身份证明的创建和验证功能，确保节点身份的真实性。
type IdentityBinding struct {
	// localID 本地节点 ID
	localID types.PeerID

	// privateKey 本地私钥
	privateKey pkgif.PrivateKey

	// publicKey 本地公钥
	publicKey pkgif.PublicKey

	// allowExpiredProofs 是否允许过期证明（仅用于测试）
	allowExpiredProofs bool
}

// IdentityProof 身份证明
//
// 包含节点身份信息和签名，用于验证节点声称的身份。
type IdentityProof struct {
	// Version 证明版本
	Version uint8

	// Timestamp 证明创建时间戳（Unix 秒）
	Timestamp int64

	// PublicKey 节点公钥
	PublicKey []byte

	// PeerID 节点 ID
	PeerID types.PeerID

	// Signature 签名
	Signature []byte

	// Extensions 扩展数据（预留）
	Extensions []byte
}

// ============================================================================
//                              构造函数
// ============================================================================

// NewIdentityBinding 创建身份绑定验证器
//
// 参数：
//   - privateKey: 本地私钥，用于创建身份证明
//
// 返回：
//   - *IdentityBinding: 身份绑定验证器
//   - error: 创建失败时的错误
func NewIdentityBinding(privateKey pkgif.PrivateKey) (*IdentityBinding, error) {
	if privateKey == nil {
		return nil, fmt.Errorf("private key is nil")
	}

	publicKey := privateKey.PublicKey()
	if publicKey == nil {
		return nil, fmt.Errorf("failed to get public key")
	}

	// 派生 PeerID
	pubKeyBytes, err := publicKey.Raw()
	if err != nil {
		return nil, fmt.Errorf("get public key bytes: %w", err)
	}

	localID, err := derivePeerIDFromEd25519(pubKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("derive peer id: %w", err)
	}

	return &IdentityBinding{
		localID:    localID,
		privateKey: privateKey,
		publicKey:  publicKey,
	}, nil
}

// NewIdentityBindingFromIdentity 从 Identity 接口创建身份绑定验证器
func NewIdentityBindingFromIdentity(identity pkgif.Identity) (*IdentityBinding, error) {
	if identity == nil {
		return nil, fmt.Errorf("identity is nil")
	}

	return NewIdentityBinding(identity.PrivateKey())
}

// ============================================================================
//                              身份验证方法
// ============================================================================

// VerifyBinding 验证远端身份绑定
//
// 验证远端节点声称的 PeerID 是否与其公钥匹配。
// 这是防止身份伪造的核心验证。
//
// 参数：
//   - remotePubKey: 远端公钥
//   - claimedPeerID: 远端声称的 PeerID
//
// 返回：
//   - error: 验证失败时返回错误
func (ib *IdentityBinding) VerifyBinding(remotePubKey pkgif.PublicKey, claimedPeerID types.PeerID) error {
	if remotePubKey == nil {
		return ErrInvalidPublicKey
	}

	// 从公钥派生 PeerID
	pubKeyBytes, err := remotePubKey.Raw()
	if err != nil {
		return fmt.Errorf("get public key bytes: %w", err)
	}

	derivedPeerID, err := derivePeerIDFromEd25519(pubKeyBytes)
	if err != nil {
		return fmt.Errorf("derive peer id: %w", err)
	}

	// 验证 PeerID 匹配
	if derivedPeerID != claimedPeerID {
		logger.Warn("身份绑定验证失败",
			"claimed", string(claimedPeerID)[:8],
			"derived", string(derivedPeerID)[:8])
		return ErrIdentityMismatch
	}

	return nil
}

// VerifyBindingFromBytes 从公钥字节验证身份绑定
//
// 参数：
//   - remotePubKeyBytes: 远端公钥字节（Ed25519 格式）
//   - claimedPeerID: 远端声称的 PeerID
//
// 返回：
//   - error: 验证失败时返回错误
func (ib *IdentityBinding) VerifyBindingFromBytes(remotePubKeyBytes []byte, claimedPeerID types.PeerID) error {
	if len(remotePubKeyBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: invalid key length %d", ErrInvalidPublicKey, len(remotePubKeyBytes))
	}

	// 派生 PeerID
	derivedPeerID, err := derivePeerIDFromEd25519(remotePubKeyBytes)
	if err != nil {
		return fmt.Errorf("derive peer id: %w", err)
	}

	// 验证 PeerID 匹配
	if derivedPeerID != claimedPeerID {
		return ErrIdentityMismatch
	}

	return nil
}

// ============================================================================
//                              身份证明方法
// ============================================================================

// CreateProof 创建身份证明
//
// 创建一个包含签名的身份证明，用于向其他节点证明自己的身份。
//
// 返回：
//   - *IdentityProof: 身份证明
//   - error: 创建失败时的错误
func (ib *IdentityBinding) CreateProof() (*IdentityProof, error) {
	pubKeyBytes, err := ib.publicKey.Raw()
	if err != nil {
		return nil, fmt.Errorf("get public key bytes: %w", err)
	}

	now := time.Now().Unix()

	// 创建证明数据
	proof := &IdentityProof{
		Version:   ProofVersion,
		Timestamp: now,
		PublicKey: pubKeyBytes,
		PeerID:    ib.localID,
	}

	// 计算签名数据
	sigData := proof.signatureData()

	// 签名
	privKeyBytes, err := ib.privateKey.Raw()
	if err != nil {
		return nil, fmt.Errorf("get private key bytes: %w", err)
	}

	// Ed25519 签名
	signature := ed25519.Sign(privKeyBytes, sigData)
	proof.Signature = signature

	return proof, nil
}

// CreateProofBytes 创建身份证明的字节表示
//
// 返回：
//   - []byte: 序列化的身份证明
//   - error: 创建失败时的错误
func (ib *IdentityBinding) CreateProofBytes() ([]byte, error) {
	proof, err := ib.CreateProof()
	if err != nil {
		return nil, err
	}

	return proof.Marshal()
}

// VerifyProof 验证身份证明
//
// 验证身份证明的签名和有效期。
//
// 参数：
//   - proof: 身份证明
//
// 返回：
//   - error: 验证失败时返回错误
func (ib *IdentityBinding) VerifyProof(proof *IdentityProof) error {
	if proof == nil {
		return ErrInvalidProof
	}

	// 检查版本
	if proof.Version != ProofVersion {
		return fmt.Errorf("%w: unsupported version %d", ErrInvalidProof, proof.Version)
	}

	// 检查有效期
	if !ib.allowExpiredProofs {
		proofTime := time.Unix(proof.Timestamp, 0)
		if time.Since(proofTime) > ProofValidityDuration {
			return ErrProofExpired
		}
		// 也检查未来时间（防止时钟偏移攻击）
		if proofTime.After(time.Now().Add(5 * time.Minute)) {
			return fmt.Errorf("%w: proof from future", ErrInvalidProof)
		}
	}

	// 验证公钥长度
	if len(proof.PublicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: invalid public key length", ErrInvalidPublicKey)
	}

	// 验证签名长度
	if len(proof.Signature) != ed25519.SignatureSize {
		return fmt.Errorf("%w: invalid signature length", ErrInvalidSignature)
	}

	// 验证 PeerID 与公钥绑定
	derivedPeerID, err := derivePeerIDFromEd25519(proof.PublicKey)
	if err != nil {
		return fmt.Errorf("derive peer id: %w", err)
	}
	if derivedPeerID != proof.PeerID {
		return ErrIdentityMismatch
	}

	// 验证签名
	sigData := proof.signatureData()
	if !ed25519.Verify(proof.PublicKey, sigData, proof.Signature) {
		return ErrInvalidSignature
	}

	return nil
}

// VerifyProofBytes 验证序列化的身份证明
//
// 参数：
//   - data: 序列化的身份证明
//
// 返回：
//   - *IdentityProof: 解析后的身份证明
//   - error: 验证失败时返回错误
func (ib *IdentityBinding) VerifyProofBytes(data []byte) (*IdentityProof, error) {
	proof, err := UnmarshalIdentityProof(data)
	if err != nil {
		return nil, fmt.Errorf("unmarshal proof: %w", err)
	}

	if err := ib.VerifyProof(proof); err != nil {
		return nil, err
	}

	return proof, nil
}

// ============================================================================
//                              辅助方法
// ============================================================================

// LocalID 返回本地节点 ID
func (ib *IdentityBinding) LocalID() types.PeerID {
	return ib.localID
}

// PublicKey 返回本地公钥
func (ib *IdentityBinding) PublicKey() pkgif.PublicKey {
	return ib.publicKey
}

// SetAllowExpiredProofs 设置是否允许过期证明（仅用于测试）
func (ib *IdentityBinding) SetAllowExpiredProofs(allow bool) {
	ib.allowExpiredProofs = allow
}

// ============================================================================
//                              IdentityProof 方法
// ============================================================================

// signatureData 生成签名数据
func (p *IdentityProof) signatureData() []byte {
	var buf bytes.Buffer

	// 写入签名前缀
	buf.WriteString(SignaturePrefix)

	// 写入版本
	buf.WriteByte(p.Version)

	// 写入时间戳
	ts := make([]byte, 8)
	binary.BigEndian.PutUint64(ts, uint64(p.Timestamp))
	buf.Write(ts)

	// 写入公钥
	buf.Write(p.PublicKey)

	// 写入 PeerID
	buf.WriteString(string(p.PeerID))

	return buf.Bytes()
}

// Marshal 序列化身份证明
func (p *IdentityProof) Marshal() ([]byte, error) {
	var buf bytes.Buffer

	// 版本 (1 字节)
	buf.WriteByte(p.Version)

	// 时间戳 (8 字节)
	ts := make([]byte, 8)
	binary.BigEndian.PutUint64(ts, uint64(p.Timestamp))
	buf.Write(ts)

	// 公钥长度 + 公钥
	pubKeyLen := make([]byte, 2)
	binary.BigEndian.PutUint16(pubKeyLen, uint16(len(p.PublicKey)))
	buf.Write(pubKeyLen)
	buf.Write(p.PublicKey)

	// PeerID 长度 + PeerID
	peerIDBytes := []byte(p.PeerID)
	peerIDLen := make([]byte, 2)
	binary.BigEndian.PutUint16(peerIDLen, uint16(len(peerIDBytes)))
	buf.Write(peerIDLen)
	buf.Write(peerIDBytes)

	// 签名长度 + 签名
	sigLen := make([]byte, 2)
	binary.BigEndian.PutUint16(sigLen, uint16(len(p.Signature)))
	buf.Write(sigLen)
	buf.Write(p.Signature)

	// 扩展数据长度 + 扩展数据
	extLen := make([]byte, 2)
	binary.BigEndian.PutUint16(extLen, uint16(len(p.Extensions)))
	buf.Write(extLen)
	buf.Write(p.Extensions)

	return buf.Bytes(), nil
}

// UnmarshalIdentityProof 反序列化身份证明
func UnmarshalIdentityProof(data []byte) (*IdentityProof, error) {
	if len(data) < MinProofSize {
		return nil, fmt.Errorf("%w: data too short", ErrInvalidProof)
	}

	p := &IdentityProof{}
	offset := 0

	// 版本
	p.Version = data[offset]
	offset++

	// 时间戳
	p.Timestamp = int64(binary.BigEndian.Uint64(data[offset : offset+8]))
	offset += 8

	// 公钥
	if offset+2 > len(data) {
		return nil, fmt.Errorf("%w: truncated public key length", ErrInvalidProof)
	}
	pubKeyLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2
	if offset+pubKeyLen > len(data) {
		return nil, fmt.Errorf("%w: truncated public key", ErrInvalidProof)
	}
	p.PublicKey = make([]byte, pubKeyLen)
	copy(p.PublicKey, data[offset:offset+pubKeyLen])
	offset += pubKeyLen

	// PeerID
	if offset+2 > len(data) {
		return nil, fmt.Errorf("%w: truncated peer id length", ErrInvalidProof)
	}
	peerIDLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2
	if offset+peerIDLen > len(data) {
		return nil, fmt.Errorf("%w: truncated peer id", ErrInvalidProof)
	}
	p.PeerID = types.PeerID(data[offset : offset+peerIDLen])
	offset += peerIDLen

	// 签名
	if offset+2 > len(data) {
		return nil, fmt.Errorf("%w: truncated signature length", ErrInvalidProof)
	}
	sigLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2
	if offset+sigLen > len(data) {
		return nil, fmt.Errorf("%w: truncated signature", ErrInvalidProof)
	}
	p.Signature = make([]byte, sigLen)
	copy(p.Signature, data[offset:offset+sigLen])
	offset += sigLen

	// 扩展数据（可选）
	if offset+2 <= len(data) {
		extLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2
		if offset+extLen <= len(data) {
			p.Extensions = make([]byte, extLen)
			copy(p.Extensions, data[offset:offset+extLen])
		}
	}

	return p, nil
}

// ============================================================================
//                              便捷函数
// ============================================================================

// VerifyPeerIDBinding 验证 PeerID 与公钥的绑定关系（静态函数）
//
// 这是一个便捷函数，无需创建 IdentityBinding 实例即可验证绑定。
//
// 参数：
//   - pubKeyBytes: 公钥字节（Ed25519 格式）
//   - claimedPeerID: 声称的 PeerID
//
// 返回：
//   - error: 验证失败时返回错误
func VerifyPeerIDBinding(pubKeyBytes []byte, claimedPeerID types.PeerID) error {
	if len(pubKeyBytes) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: invalid key length %d", ErrInvalidPublicKey, len(pubKeyBytes))
	}

	// 派生 PeerID
	derivedPeerID, err := derivePeerIDFromEd25519(pubKeyBytes)
	if err != nil {
		return fmt.Errorf("derive peer id: %w", err)
	}

	// 验证 PeerID 匹配
	if derivedPeerID != claimedPeerID {
		return ErrIdentityMismatch
	}

	return nil
}

// VerifyPublicKeyMatchesPeerID 验证公钥是否与 PeerID 匹配
//
// 参数：
//   - pubKeyBytes: 公钥字节（Ed25519 格式）
//   - peerID: 节点 ID
//
// 返回：
//   - bool: 是否匹配
//   - error: 验证失败时返回错误
func VerifyPublicKeyMatchesPeerID(pubKeyBytes []byte, peerID types.PeerID) (bool, error) {
	// 从公钥派生 PeerID
	pubKey, err := crypto.UnmarshalEd25519PublicKey(pubKeyBytes)
	if err != nil {
		return false, fmt.Errorf("unmarshal public key: %w", err)
	}

	return crypto.VerifyPeerID(pubKey, peerID)
}
