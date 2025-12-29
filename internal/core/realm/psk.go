// Package realm 提供 Realm 管理实现
package realm

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"time"

	realmif "github.com/dep2p/go-dep2p/pkg/interfaces/realm"
	"github.com/dep2p/go-dep2p/pkg/types"
	"golang.org/x/crypto/hkdf"
)

// ============================================================================
//                              PSK 认证器实现（IMPL-1227）
// ============================================================================

// PSKAuthSalt PSK 密钥派生盐值
const PSKAuthSalt = "dep2p-realm-membership-v1"

// PSKAuthenticator PSK 成员验证器实现
//
// 实现 realmif.PSKAuthenticator 接口，用于生成和验证成员证明。
//
// 证明公式:
//
//	MAC = HMAC-SHA256(
//	    key  = HKDF(realmKey, "dep2p-realm-membership-v1"),
//	    data = nodeID || realmID || peerID || nonce || timestamp
//	)
type PSKAuthenticator struct {
	// nodeID 本地节点 ID（证明发起者）
	nodeID types.NodeID

	// realmKey Realm 密钥
	realmKey types.RealmKey

	// realmID Realm ID（从 realmKey 派生）
	realmID types.RealmID

	// derivedKey HKDF 派生的密钥（缓存）
	derivedKey []byte
}

// NewPSKAuthenticator 创建 PSK 认证器
//
// 参数:
//   - nodeID: 本地节点 ID
//   - realmKey: Realm 密钥
//
// 返回:
//   - PSKAuthenticator 实例
func NewPSKAuthenticator(nodeID types.NodeID, realmKey types.RealmKey) *PSKAuthenticator {
	realmID := types.DeriveRealmID(realmKey)

	auth := &PSKAuthenticator{
		nodeID:   nodeID,
		realmKey: realmKey,
		realmID:  realmID,
	}

	// 派生密钥并缓存
	auth.derivedKey = auth.deriveKey()

	return auth
}

// ============================================================================
//                              realmif.ProofGenerator 实现
// ============================================================================

// Generate 生成成员证明
//
// peerID 是目标节点（通信对方），用于绑定证明到特定目标。
// 这防止了证明被重用于其他节点。
//
// 参数:
//   - ctx: 上下文（保留，暂未使用）
//   - peerID: 目标节点的 NodeID
//
// 返回:
//   - *types.MembershipProof: 成员证明
//   - error: 错误（如随机数生成失败）
func (a *PSKAuthenticator) Generate(ctx context.Context, peerID types.NodeID) (*types.MembershipProof, error) {
	proof := &types.MembershipProof{
		NodeID:    a.nodeID,
		RealmID:   a.realmID,
		PeerID:    peerID,
		Timestamp: time.Now().Unix(),
	}

	// 生成随机 nonce（16 字节）
	if _, err := rand.Read(proof.Nonce[:]); err != nil {
		return nil, err
	}

	// 计算 MAC
	a.computeMAC(proof)

	return proof, nil
}

// ============================================================================
//                              realmif.ProofVerifier 实现
// ============================================================================

// Verify 验证成员证明
//
// 验证规则:
//  1. 检查时间戳（5分钟窗口）
//  2. 检查 peerID 匹配（证明是否绑定到预期目标）
//  3. 检查 realmID 匹配（证明是否属于同一 Realm）
//  4. 重新计算 MAC 并比较
//
// 参数:
//   - proof: 待验证的成员证明
//   - expectedPeerID: 预期的目标节点
//     -- 中继场景: R 验证时，expectedPeerID = 请求中的 targetNodeID
//     -- 直连场景: B 验证时，expectedPeerID = 自己的 NodeID
//
// 返回:
//   - error: 验证失败原因（nil 表示验证通过）
//     -- types.ErrProofExpired: 证明已过期
//     -- types.ErrPeerIDMismatch: 目标节点不匹配
//     -- types.ErrInvalidProof: MAC 验证失败
//     -- ErrRealmMismatch: Realm 不匹配
func (a *PSKAuthenticator) Verify(proof *types.MembershipProof, expectedPeerID types.NodeID) error {
	// 1. 检查时间戳（5分钟窗口）
	if proof.IsExpired() {
		return types.ErrProofExpired
	}

	// 2. 检查 peerID 匹配（证明是否绑定到预期目标）
	if proof.PeerID != expectedPeerID {
		return types.ErrPeerIDMismatch
	}

	// 3. 检查 realmID 匹配
	if proof.RealmID != a.realmID {
		return ErrRealmMismatch
	}

	// 4. 重新计算 MAC 并比较
	expectedMAC := a.computeMACValue(proof)
	if !hmac.Equal(expectedMAC, proof.MAC[:]) {
		return types.ErrInvalidProof
	}

	return nil
}

// ============================================================================
//                              realmif.PSKAuthenticator 实现
// ============================================================================

// RealmID 返回此认证器对应的 RealmID
func (a *PSKAuthenticator) RealmID() types.RealmID {
	return a.realmID
}

// ============================================================================
//                              内部方法
// ============================================================================

// deriveKey 派生 HMAC 密钥
//
// 使用 HKDF 从 realmKey 派生密钥:
//
//	derivedKey = HKDF-Extract(SHA256, realmKey, "dep2p-realm-membership-v1")
func (a *PSKAuthenticator) deriveKey() []byte {
	// HKDF-Extract
	reader := hkdf.New(sha256.New, a.realmKey[:], []byte(PSKAuthSalt), nil)
	key := make([]byte, 32) // SHA256 输出 32 字节
	if _, err := io.ReadFull(reader, key); err != nil {
		// HKDF 不应该失败，除非参数有问题
		panic("hkdf read failed: " + err.Error())
	}
	return key
}

// computeMAC 计算并设置证明的 MAC 字段
func (a *PSKAuthenticator) computeMAC(proof *types.MembershipProof) {
	mac := a.computeMACValue(proof)
	copy(proof.MAC[:], mac)
}

// computeMACValue 计算证明的 MAC 值（不修改证明）
func (a *PSKAuthenticator) computeMACValue(proof *types.MembershipProof) []byte {
	h := hmac.New(sha256.New, a.derivedKey)
	h.Write(proof.DataForMAC())
	return h.Sum(nil)
}

// ============================================================================
//                              PSK 认证器工厂实现
// ============================================================================

// PSKAuthenticatorFactory PSK 认证器工厂
type PSKAuthenticatorFactory struct{}

// NewPSKAuthenticatorFactory 创建 PSK 认证器工厂
func NewPSKAuthenticatorFactory() *PSKAuthenticatorFactory {
	return &PSKAuthenticatorFactory{}
}

// Create 创建 PSK 认证器
func (f *PSKAuthenticatorFactory) Create(nodeID types.NodeID, realmKey types.RealmKey) (realmif.PSKAuthenticator, error) {
	return NewPSKAuthenticator(nodeID, realmKey), nil
}

// ============================================================================
//                              PSK 相关错误（补充）
// ============================================================================

// 注意：主要的 PSK 错误定义在 pkg/types/psk.go 中
// 这里只定义 internal 包特有的错误

// ErrPSKRequired PSK 认证必须
var ErrPSKRequired = ErrRealmKeyRequired

