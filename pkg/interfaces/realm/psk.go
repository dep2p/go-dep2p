// Package realm 定义 Realm 相关接口
package realm

import (
	"context"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              PSK 成员认证接口（IMPL-1227 新增）
// ============================================================================

// ProofGenerator 成员证明生成器接口
//
// 用于生成 PSK 成员证明，证明自己是 Realm 的合法成员。
//
// 证明公式:
//
//	MAC = HMAC-SHA256(
//	    key  = HKDF(realmKey, "dep2p-realm-membership-v1"),
//	    data = nodeID || realmID || peerID || nonce || timestamp
//	)
//
// 使用场景:
//   - 连接到其他 Realm 成员时
//   - 请求 Realm Relay 服务时
//   - 验证成员身份时
type ProofGenerator interface {
	// Generate 生成成员证明
	//
	// peerID 是目标节点（通信对方），用于绑定证明到特定目标。
	// 这防止了证明被重用于其他节点。
	//
	// 示例:
	//
	//	proof, err := generator.Generate(ctx, targetPeerID)
	//	if err != nil { ... }
	//	// 将 proof 发送给目标或中继
	Generate(ctx context.Context, peerID types.NodeID) (*types.MembershipProof, error)
}

// ProofVerifier 成员证明验证器接口
//
// 用于验证 PSK 成员证明，确认对方是 Realm 的合法成员。
//
// 验证规则:
//   - MAC 正确（对方持有相同的 realmKey）
//   - peerID 匹配预期（中继场景下是 targetNodeID，直连场景下是自己的 NodeID）
//   - timestamp 有效（在 5 分钟窗口内）
type ProofVerifier interface {
	// Verify 验证成员证明
	//
	// expectedPeerID 是预期的目标节点:
	//   - 中继场景: R 验证时，expectedPeerID = 请求中的 targetNodeID
	//   - 直连场景: B 验证时，expectedPeerID = 自己的 NodeID
	//
	// 错误:
	//   - types.ErrProofExpired: 证明已过期
	//   - types.ErrInvalidProof: MAC 验证失败
	//   - types.ErrPeerIDMismatch: 目标节点不匹配
	Verify(proof *types.MembershipProof, expectedPeerID types.NodeID) error
}

// PSKAuthenticator 组合接口（生成 + 验证）
//
// 同时具备生成和验证成员证明的能力。
// 每个 Realm 成员都应该持有一个 PSKAuthenticator 实例。
//
// 示例:
//
//	auth := realm.PSKAuth()
//
//	// 生成证明
//	proof, _ := auth.Generate(ctx, peerID)
//
//	// 验证证明
//	err := auth.Verify(receivedProof, myNodeID)
type PSKAuthenticator interface {
	ProofGenerator
	ProofVerifier

	// RealmID 返回此认证器对应的 RealmID
	RealmID() types.RealmID
}

// ============================================================================
//                              PSK 认证器工厂
// ============================================================================

// PSKAuthenticatorFactory PSK 认证器工厂接口
//
// 用于创建 PSKAuthenticator 实例。
type PSKAuthenticatorFactory interface {
	// Create 创建 PSK 认证器
	//
	// nodeID: 本地节点 ID
	// realmKey: Realm 密钥
	Create(nodeID types.NodeID, realmKey types.RealmKey) (PSKAuthenticator, error)
}

// ============================================================================
//                              Realm 认证接口扩展
// ============================================================================

// AuthenticatedRealm 带认证能力的 Realm 接口
//
// 扩展 Realm 接口，添加 PSK 认证相关方法。
// 这是 Realm 接口的可选扩展，用于需要显式访问认证能力的场景。
type AuthenticatedRealm interface {
	Realm

	// PSKAuth 获取 PSK 认证器
	//
	// 用于生成和验证成员证明。
	PSKAuth() PSKAuthenticator
}

