package dht

import (
	"strings"
	"testing"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              RealmPeerKey 测试
// ============================================================================

func TestRealmPeerKey_Format(t *testing.T) {
	realmID := types.RealmID("test-realm")
	nodeID := types.NodeID("test-node-12345")

	key := RealmPeerKey(realmID, nodeID)

	// 验证 v2 格式
	if !strings.HasPrefix(key, "/dep2p/v2/realm/") {
		t.Errorf("Key should start with /dep2p/v2/realm/, got %s", key)
	}

	if !strings.Contains(key, "/peer/") {
		t.Errorf("Key should contain /peer/, got %s", key)
	}

	if !strings.HasSuffix(key, string(nodeID)) {
		t.Errorf("Key should end with nodeID, got %s", key)
	}

	// 验证 RealmID 被哈希
	if strings.Contains(key, string(realmID)) {
		t.Error("Key should contain hashed RealmID, not plain text")
	}
}

func TestRealmPeerKey_DifferentRealms(t *testing.T) {
	nodeID := types.NodeID("test-node")
	realm1 := types.RealmID("realm-1")
	realm2 := types.RealmID("realm-2")

	key1 := RealmPeerKey(realm1, nodeID)
	key2 := RealmPeerKey(realm2, nodeID)

	// 不同 Realm 应该产生不同的 Key
	if key1 == key2 {
		t.Error("Different realms should produce different keys")
	}
}

func TestRealmPeerKey_SameInput(t *testing.T) {
	realmID := types.RealmID("test-realm")
	nodeID := types.NodeID("test-node")

	key1 := RealmPeerKey(realmID, nodeID)
	key2 := RealmPeerKey(realmID, nodeID)

	// 相同输入应该产生相同的 Key
	if key1 != key2 {
		t.Error("Same input should produce same key")
	}
}

// ============================================================================
//                              ParseRealmKey 测试
// ============================================================================

func TestParseRealmKey_Valid(t *testing.T) {
	realmID := types.RealmID("test-realm")
	nodeID := types.NodeID("test-node-12345")

	key := RealmPeerKey(realmID, nodeID)

	parsed, err := ParseRealmKey(key)
	if err != nil {
		t.Fatalf("ParseRealmKey failed: %v", err)
	}

	if parsed.KeyType != KeyTypePeer {
		t.Errorf("Expected key type %s, got %s", KeyTypePeer, parsed.KeyType)
	}

	if parsed.Payload != string(nodeID) {
		t.Errorf("Expected payload %s, got %s", nodeID, parsed.Payload)
	}

	// RealmHash 应该是 64 字符的十六进制
	if len(parsed.RealmHash) != 64 {
		t.Errorf("Expected realm hash length 64, got %d", len(parsed.RealmHash))
	}
}

func TestParseRealmKey_Invalid(t *testing.T) {
	testCases := []struct {
		name string
		key  string
	}{
		{"empty", ""},
		{"too short", "/dep2p/v2/realm"},
		{"wrong prefix", "/libp2p/v2/realm/abc/peer/node"},
		{"wrong version", "/dep2p/v1/realm/" + strings.Repeat("a", 64) + "/peer/node"},
		{"invalid realm hash", "/dep2p/v2/realm/short/peer/node"},
		{"unknown key type", "/dep2p/v2/realm/" + strings.Repeat("a", 64) + "/unknown/node"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseRealmKey(tc.key)
			if err == nil {
				t.Errorf("Expected error for key: %s", tc.key)
			}
		})
	}
}

// ============================================================================
//                              ValidateRealmPeerKey 测试
// ============================================================================

func TestValidateRealmPeerKey_Valid(t *testing.T) {
	realmID := types.RealmID("test-realm")
	nodeID := types.NodeID("test-node")

	key := RealmPeerKey(realmID, nodeID)

	err := ValidateRealmPeerKey(key, realmID, nodeID)
	if err != nil {
		t.Errorf("Validation should pass: %v", err)
	}
}

func TestValidateRealmPeerKey_RealmMismatch(t *testing.T) {
	realmID1 := types.RealmID("realm-1")
	realmID2 := types.RealmID("realm-2")
	nodeID := types.NodeID("test-node")

	key := RealmPeerKey(realmID1, nodeID)

	err := ValidateRealmPeerKey(key, realmID2, nodeID)
	if err != ErrRealmIDMismatch {
		t.Errorf("Expected ErrRealmIDMismatch, got %v", err)
	}
}

func TestValidateRealmPeerKey_NodeMismatch(t *testing.T) {
	realmID := types.RealmID("test-realm")
	nodeID1 := types.NodeID("node-1")
	nodeID2 := types.NodeID("node-2")

	key := RealmPeerKey(realmID, nodeID1)

	err := ValidateRealmPeerKey(key, realmID, nodeID2)
	if err != ErrNodeIDMismatch {
		t.Errorf("Expected ErrNodeIDMismatch, got %v", err)
	}
}

// ============================================================================
//                              RealmMembersKey 测试（v2.0 新增）
// ============================================================================

func TestRealmMembersKey_Format(t *testing.T) {
	realmID := types.RealmID("test-realm")
	key := RealmMembersKey(realmID)

	// 验证 v2 格式
	if !strings.HasPrefix(key, "/dep2p/v2/realm/") {
		t.Errorf("Key should start with /dep2p/v2/realm/, got %s", key)
	}

	if !strings.HasSuffix(key, "/members") {
		t.Errorf("Key should end with /members, got %s", key)
	}

	// 验证 RealmID 被哈希
	if strings.Contains(key, string(realmID)) {
		t.Error("Key should contain hashed RealmID, not plain text")
	}
}

func TestRealmMembersKey_DifferentRealms(t *testing.T) {
	realm1 := types.RealmID("realm-1")
	realm2 := types.RealmID("realm-2")

	key1 := RealmMembersKey(realm1)
	key2 := RealmMembersKey(realm2)

	// 不同 Realm 应该产生不同的 Key
	if key1 == key2 {
		t.Error("Different realms should produce different keys")
	}
}

// ============================================================================
//                              RealmValueKey 和 RealmProviderKey 测试
// ============================================================================

func TestRealmValueKey_Format(t *testing.T) {
	realmID := types.RealmID("test-realm")
	key := RealmValueKey(realmID, "my-key")

	// 验证 v2 格式
	if !strings.HasPrefix(key, "/dep2p/v2/realm/") {
		t.Errorf("Key should start with /dep2p/v2/realm/, got %s", key)
	}

	if !strings.Contains(key, "/value/") {
		t.Errorf("Key should contain /value/, got %s", key)
	}

	if !strings.HasSuffix(key, "my-key") {
		t.Errorf("Key should end with my-key, got %s", key)
	}
}

func TestRealmProviderKey_Format(t *testing.T) {
	realmID := types.RealmID("test-realm")
	key := RealmProviderKey(realmID, "my-content")

	// 验证 v2 格式
	if !strings.HasPrefix(key, "/dep2p/v2/realm/") {
		t.Errorf("Key should start with /dep2p/v2/realm/, got %s", key)
	}

	if !strings.Contains(key, "/provider/") {
		t.Errorf("Key should contain /provider/, got %s", key)
	}

	if !strings.HasSuffix(key, "my-content") {
		t.Errorf("Key should end with my-content, got %s", key)
	}
}

// ============================================================================
//                              Hash 函数测试
// ============================================================================

func TestHashRealmID_Deterministic(t *testing.T) {
	realmID := types.RealmID("test-realm")

	hash1 := HashRealmID(realmID)
	hash2 := HashRealmID(realmID)

	if string(hash1) != string(hash2) {
		t.Error("Hash should be deterministic")
	}
}

func TestHashRealmID_Length(t *testing.T) {
	realmID := types.RealmID("test-realm")

	hash := HashRealmID(realmID)

	if len(hash) != 32 {
		t.Errorf("SHA256 hash should be 32 bytes, got %d", len(hash))
	}
}
