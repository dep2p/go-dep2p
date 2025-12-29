// Package realm 提供 Realm 管理实现
package realm

import (
	"context"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              PSKAuthenticator 测试（IMPL-1227）
// ============================================================================

// TestPSKAuthenticator_GenerateVerify 测试证明生成和验证
func TestPSKAuthenticator_GenerateVerify(t *testing.T) {
	// 设置
	nodeA := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodeB := types.NodeID{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	realmKey := types.GenerateRealmKey()

	// 创建认证器（A 节点）
	authA := NewPSKAuthenticator(nodeA, realmKey)

	// A 生成证明（目标是 B）
	ctx := context.Background()
	proof, err := authA.Generate(ctx, nodeB)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// 验证证明字段
	if proof.NodeID != nodeA {
		t.Error("proof.NodeID should be nodeA")
	}
	if proof.PeerID != nodeB {
		t.Error("proof.PeerID should be nodeB")
	}
	if proof.RealmID != authA.RealmID() {
		t.Error("proof.RealmID should match authenticator RealmID")
	}

	// B 验证 A 的证明（使用相同的 realmKey）
	authB := NewPSKAuthenticator(nodeB, realmKey)
	err = authB.Verify(proof, nodeB) // expectedPeerID = 自己
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	t.Log("PSK 证明生成和验证测试通过")
}

// TestPSKAuthenticator_VerifyPeerIDMismatch 测试 peerID 不匹配场景
func TestPSKAuthenticator_VerifyPeerIDMismatch(t *testing.T) {
	nodeA := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodeB := types.NodeID{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	nodeC := types.NodeID{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
	realmKey := types.GenerateRealmKey()

	authA := NewPSKAuthenticator(nodeA, realmKey)
	authC := NewPSKAuthenticator(nodeC, realmKey)

	// A 生成证明（目标是 B）
	ctx := context.Background()
	proof, err := authA.Generate(ctx, nodeB)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// C 尝试验证（expectedPeerID = C，但证明的 peerID = B）
	err = authC.Verify(proof, nodeC)
	if err != types.ErrPeerIDMismatch {
		t.Errorf("expected ErrPeerIDMismatch, got: %v", err)
	}

	t.Log("PeerID 不匹配测试通过")
}

// TestPSKAuthenticator_VerifyRealmMismatch 测试 Realm 不匹配场景
func TestPSKAuthenticator_VerifyRealmMismatch(t *testing.T) {
	nodeA := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodeB := types.NodeID{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	realmKey1 := types.GenerateRealmKey()
	realmKey2 := types.GenerateRealmKey()

	authA := NewPSKAuthenticator(nodeA, realmKey1)
	authB := NewPSKAuthenticator(nodeB, realmKey2) // 不同的 realmKey

	// A 生成证明
	ctx := context.Background()
	proof, err := authA.Generate(ctx, nodeB)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// B 验证（不同 Realm）
	err = authB.Verify(proof, nodeB)
	if err != ErrRealmMismatch {
		t.Errorf("expected ErrRealmMismatch, got: %v", err)
	}

	t.Log("Realm 不匹配测试通过")
}

// TestPSKAuthenticator_VerifyInvalidMAC 测试 MAC 篡改检测
func TestPSKAuthenticator_VerifyInvalidMAC(t *testing.T) {
	nodeA := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodeB := types.NodeID{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	realmKey := types.GenerateRealmKey()

	authA := NewPSKAuthenticator(nodeA, realmKey)
	authB := NewPSKAuthenticator(nodeB, realmKey)

	// A 生成证明
	ctx := context.Background()
	proof, err := authA.Generate(ctx, nodeB)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// 篡改 MAC
	proof.MAC[0] ^= 0xFF

	// B 验证（MAC 被篡改）
	err = authB.Verify(proof, nodeB)
	if err != types.ErrInvalidProof {
		t.Errorf("expected ErrInvalidProof, got: %v", err)
	}

	t.Log("MAC 篡改检测测试通过")
}

// TestPSKAuthenticator_VerifyExpired 测试过期证明
func TestPSKAuthenticator_VerifyExpired(t *testing.T) {
	nodeA := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodeB := types.NodeID{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	realmKey := types.GenerateRealmKey()

	authA := NewPSKAuthenticator(nodeA, realmKey)
	authB := NewPSKAuthenticator(nodeB, realmKey)

	// A 生成证明
	ctx := context.Background()
	proof, err := authA.Generate(ctx, nodeB)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	// 模拟过期（设置时间戳为 6 分钟前）
	proof.Timestamp = time.Now().Add(-6 * time.Minute).Unix()

	// B 验证（证明已过期）
	err = authB.Verify(proof, nodeB)
	if err != types.ErrProofExpired {
		t.Errorf("expected ErrProofExpired, got: %v", err)
	}

	t.Log("过期证明检测测试通过")
}

// TestPSKAuthenticator_RealmID 测试 RealmID 派生一致性
func TestPSKAuthenticator_RealmID(t *testing.T) {
	nodeA := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodeB := types.NodeID{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	realmKey := types.GenerateRealmKey()

	authA := NewPSKAuthenticator(nodeA, realmKey)
	authB := NewPSKAuthenticator(nodeB, realmKey)

	// 同一个 realmKey 应该派生出相同的 RealmID
	if authA.RealmID() != authB.RealmID() {
		t.Error("same realmKey should derive same RealmID")
	}

	// 不同 realmKey 应该派生出不同的 RealmID
	realmKey2 := types.GenerateRealmKey()
	authC := NewPSKAuthenticator(nodeA, realmKey2)
	if authA.RealmID() == authC.RealmID() {
		t.Error("different realmKey should derive different RealmID")
	}

	t.Logf("RealmID 派生一致性测试通过，RealmID: %s", authA.RealmID())
}

// TestPSKAuthenticator_NonceUniqueness 测试 Nonce 唯一性
func TestPSKAuthenticator_NonceUniqueness(t *testing.T) {
	nodeA := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodeB := types.NodeID{32, 31, 30, 29, 28, 27, 26, 25, 24, 23, 22, 21, 20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	realmKey := types.GenerateRealmKey()

	auth := NewPSKAuthenticator(nodeA, realmKey)
	ctx := context.Background()

	// 生成多个证明，检查 Nonce 唯一性
	nonces := make(map[[16]byte]bool)
	for i := 0; i < 100; i++ {
		proof, err := auth.Generate(ctx, nodeB)
		if err != nil {
			t.Fatalf("Generate failed: %v", err)
		}
		if nonces[proof.Nonce] {
			t.Error("nonce collision detected")
		}
		nonces[proof.Nonce] = true
	}

	t.Log("Nonce 唯一性测试通过")
}

// TestPSKAuthenticatorFactory 测试工厂模式
func TestPSKAuthenticatorFactory(t *testing.T) {
	factory := NewPSKAuthenticatorFactory()

	nodeID := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	realmKey := types.GenerateRealmKey()

	auth, err := factory.Create(nodeID, realmKey)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if auth.RealmID() != types.DeriveRealmID(realmKey) {
		t.Error("factory should create authenticator with correct RealmID")
	}

	t.Log("PSK 认证器工厂测试通过")
}

