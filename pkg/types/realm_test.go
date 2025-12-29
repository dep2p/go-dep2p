package types

import (
	"testing"
)

// ============================================================================
//                              RealmKey 测试（IMPL-1227 Phase 7）
// ============================================================================

// TestGenerateRealmKey 测试 RealmKey 生成
func TestGenerateRealmKey(t *testing.T) {
	key := GenerateRealmKey()

	// 应该不为空
	if key.IsEmpty() {
		t.Error("generated key should not be empty")
	}

	// 应该是 32 字节
	if len(key.Bytes()) != 32 {
		t.Errorf("key should be 32 bytes, got %d", len(key.Bytes()))
	}

	// 每次生成应该不同
	key2 := GenerateRealmKey()
	if key == key2 {
		t.Error("generated keys should be unique")
	}
}

// TestRealmKey_IsEmpty 测试 RealmKey 空值检查
func TestRealmKey_IsEmpty(t *testing.T) {
	var emptyKey RealmKey
	if !emptyKey.IsEmpty() {
		t.Error("zero key should be empty")
	}

	if !EmptyRealmKey.IsEmpty() {
		t.Error("EmptyRealmKey should be empty")
	}

	key := GenerateRealmKey()
	if key.IsEmpty() {
		t.Error("generated key should not be empty")
	}
}

// TestRealmKey_Bytes 测试 RealmKey 字节转换
func TestRealmKey_Bytes(t *testing.T) {
	key := GenerateRealmKey()
	bytes := key.Bytes()

	if len(bytes) != 32 {
		t.Errorf("bytes should be 32, got %d", len(bytes))
	}

	// 字节应该与原始 key 一致
	for i := 0; i < 32; i++ {
		if bytes[i] != key[i] {
			t.Errorf("byte %d mismatch", i)
		}
	}
}

// TestRealmKey_String 测试 RealmKey 字符串表示
func TestRealmKey_String(t *testing.T) {
	key := GenerateRealmKey()
	str := key.String()

	// 应该是 64 字符的十六进制
	if len(str) != 64 {
		t.Errorf("hex string should be 64 chars, got %d", len(str))
	}
}

// TestRealmKey_FromBytes 测试从字节解析 RealmKey
func TestRealmKey_FromBytes(t *testing.T) {
	original := GenerateRealmKey()
	bytes := original.Bytes()

	parsed, err := RealmKeyFromBytes(bytes)
	if err != nil {
		t.Fatalf("RealmKeyFromBytes failed: %v", err)
	}

	if parsed != original {
		t.Error("parsed key should equal original")
	}

	// 测试错误的字节长度
	_, err = RealmKeyFromBytes([]byte{1, 2, 3})
	if err == nil {
		t.Error("should fail for short bytes")
	}
}

// TestRealmKey_FromHex 测试从十六进制解析 RealmKey
func TestRealmKey_FromHex(t *testing.T) {
	original := GenerateRealmKey()
	hexStr := original.String()

	parsed, err := RealmKeyFromHex(hexStr)
	if err != nil {
		t.Fatalf("RealmKeyFromHex failed: %v", err)
	}

	if parsed != original {
		t.Error("parsed key should equal original")
	}

	// 测试无效十六进制
	_, err = RealmKeyFromHex("not-hex")
	if err == nil {
		t.Error("should fail for invalid hex")
	}

	// 测试错误长度
	_, err = RealmKeyFromHex("abcd")
	if err == nil {
		t.Error("should fail for short hex")
	}
}

// TestDeriveRealmKeyFromName 测试从名称派生 RealmKey
func TestDeriveRealmKeyFromName(t *testing.T) {
	// 相同名称应该生成相同的 RealmKey
	key1 := DeriveRealmKeyFromName("test-realm")
	key2 := DeriveRealmKeyFromName("test-realm")
	if key1 != key2 {
		t.Error("same name should derive same RealmKey")
	}

	// 不同名称应该生成不同的 RealmKey
	key3 := DeriveRealmKeyFromName("other-realm")
	if key1 == key3 {
		t.Error("different names should derive different RealmKeys")
	}

	// 应该不为空
	if key1.IsEmpty() {
		t.Error("derived key should not be empty")
	}

	// 应该是 32 字节
	if len(key1.Bytes()) != 32 {
		t.Errorf("derived key should be 32 bytes, got %d", len(key1.Bytes()))
	}
}

// TestDeriveRealmKeyFromName_Deterministic 测试派生的确定性
func TestDeriveRealmKeyFromName_Deterministic(t *testing.T) {
	name := "lan-chat"

	// 多次派生应该得到相同结果
	results := make([]RealmKey, 10)
	for i := 0; i < 10; i++ {
		results[i] = DeriveRealmKeyFromName(name)
	}

	for i := 1; i < 10; i++ {
		if results[i] != results[0] {
			t.Errorf("derivation %d differs from first", i)
		}
	}
}

// TestDeriveRealmKeyFromName_SameRealmID 测试相同名称派生相同 RealmID
func TestDeriveRealmKeyFromName_SameRealmID(t *testing.T) {
	// 两个节点使用相同名称应该得到相同的 RealmID
	key1 := DeriveRealmKeyFromName("chat-room")
	key2 := DeriveRealmKeyFromName("chat-room")

	realmID1 := DeriveRealmID(key1)
	realmID2 := DeriveRealmID(key2)

	if realmID1 != realmID2 {
		t.Error("same name should derive same RealmID")
	}
}

// ============================================================================
//                              DeriveRealmID 测试
// ============================================================================

// TestDeriveRealmID 测试 RealmID 派生
func TestDeriveRealmID(t *testing.T) {
	key := GenerateRealmKey()
	realmID := DeriveRealmID(key)

	// RealmID 应该是 64 字符（完整 SHA256 十六进制）
	if len(string(realmID)) != 64 {
		t.Errorf("RealmID should be 64 chars, got %d", len(string(realmID)))
	}

	// 相同 key 应该生成相同的 RealmID
	realmID2 := DeriveRealmID(key)
	if realmID != realmID2 {
		t.Error("same key should derive same RealmID")
	}

	// 不同 key 应该生成不同的 RealmID
	key2 := GenerateRealmKey()
	realmID3 := DeriveRealmID(key2)
	if realmID == realmID3 {
		t.Error("different keys should derive different RealmIDs")
	}
}

// TestDeriveRealmID_Deterministic 测试 RealmID 派生的确定性
func TestDeriveRealmID_Deterministic(t *testing.T) {
	// 创建固定的 RealmKey
	var fixedKey RealmKey
	for i := 0; i < 32; i++ {
		fixedKey[i] = byte(i)
	}

	// 多次派生应该得到相同结果
	results := make([]RealmID, 10)
	for i := 0; i < 10; i++ {
		results[i] = DeriveRealmID(fixedKey)
	}

	for i := 1; i < 10; i++ {
		if results[i] != results[0] {
			t.Errorf("derivation %d differs from first", i)
		}
	}
}

// TestDeriveRealmID_NonReversible 测试无法从 RealmID 反推 RealmKey
func TestDeriveRealmID_NonReversible(t *testing.T) {
	// 这是一个概念测试：验证 RealmID 不包含可识别的 key 信息
	key := GenerateRealmKey()
	realmID := DeriveRealmID(key)

	// RealmID 不应该包含 key 的直接十六进制表示
	keyHex := key.String()
	realmIDStr := string(realmID)

	// 因为使用了双重哈希，realmID 不应该包含 keyHex 的任何子串（除非极度巧合）
	// 这只是一个基本检查
	if realmIDStr == keyHex {
		t.Error("RealmID should not equal key hex (unlikely collision)")
	}

	t.Logf("RealmID derived: %s", realmID)
}

// ============================================================================
//                              RealmDHTKey 测试
// ============================================================================

// TestRealmDHTKey 测试 Realm DHT Key 生成
func TestRealmDHTKey(t *testing.T) {
	nodeID := NodeID{1, 2, 3, 4, 5, 6, 7, 8}
	realmKey := GenerateRealmKey()
	realmID := DeriveRealmID(realmKey)

	// 有 Realm 的 DHT Key
	dhtKey := RealmDHTKey(realmID, nodeID)
	if len(dhtKey) != 32 {
		t.Errorf("DHT key should be 32 bytes, got %d", len(dhtKey))
	}

	// 无 Realm 的 DHT Key
	dhtKeyNoRealm := RealmDHTKey(DefaultRealmID, nodeID)
	if len(dhtKeyNoRealm) != 32 {
		t.Errorf("DHT key (no realm) should be 32 bytes, got %d", len(dhtKeyNoRealm))
	}

	// 有/无 Realm 的 DHT Key 应该不同（隔离）
	if string(dhtKey) == string(dhtKeyNoRealm) {
		t.Error("DHT keys for different realms should differ")
	}
}

// TestRealmDHTKey_Isolation 测试不同 Realm 的 DHT Key 隔离
func TestRealmDHTKey_Isolation(t *testing.T) {
	nodeID := NodeID{1, 2, 3, 4, 5, 6, 7, 8}
	realmKey1 := GenerateRealmKey()
	realmKey2 := GenerateRealmKey()
	realmID1 := DeriveRealmID(realmKey1)
	realmID2 := DeriveRealmID(realmKey2)

	dhtKey1 := RealmDHTKey(realmID1, nodeID)
	dhtKey2 := RealmDHTKey(realmID2, nodeID)

	// 不同 Realm 的同一节点应该有不同的 DHT Key
	if string(dhtKey1) == string(dhtKey2) {
		t.Error("same node in different realms should have different DHT keys")
	}
}

// ============================================================================
//                              AccessLevel 测试
// ============================================================================

// TestAccessLevel_String 测试访问级别字符串表示
func TestAccessLevel_String(t *testing.T) {
	testCases := []struct {
		level    AccessLevel
		expected string
	}{
		{AccessLevelPublic, "public"},
		{AccessLevelProtected, "protected"},
		{AccessLevelPrivate, "private"},
		{AccessLevel(99), "unknown"},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			result := tc.level.String()
			if result != tc.expected {
				t.Errorf("AccessLevel(%d).String() = %q, want %q", tc.level, result, tc.expected)
			}
		})
	}
}

// ============================================================================
//                              RealmMetadata 测试
// ============================================================================

// TestRealmMetadata 测试 Realm 元数据结构
func TestRealmMetadata(t *testing.T) {
	realmKey := GenerateRealmKey()
	realmID := DeriveRealmID(realmKey)
	nodeID := NodeID{1, 2, 3}

	metadata := RealmMetadata{
		ID:          realmID,
		Name:        "test-realm",
		CreatorID:   nodeID,
		AccessLevel: AccessLevelProtected,
		Description: "A test realm",
	}

	if metadata.ID != realmID {
		t.Error("metadata ID mismatch")
	}
	if metadata.Name != "test-realm" {
		t.Error("metadata Name mismatch")
	}
	if metadata.AccessLevel != AccessLevelProtected {
		t.Error("metadata AccessLevel mismatch")
	}
}

