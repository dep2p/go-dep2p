package dht

import (
	"bytes"
	"crypto/sha256"
	"strings"
	"testing"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                    DHT Keyspace v1.1 测试
// ============================================================================

// TestBuildKeyString_SystemScope 验证系统层 Key 字符串格式
func TestBuildKeyString_SystemScope(t *testing.T) {
	tests := []struct {
		keyType  string
		payload  string
		expected string
	}{
		{TypeBootstrap, "QmXxx", "dep2p/v1/sys/bootstrap/QmXxx"},
		{TypeRelay, "QmYyy", "dep2p/v1/sys/relay/QmYyy"},
		{TypeNAT, "QmZzz", "dep2p/v1/sys/nat/QmZzz"},
		{TypePeer, "QmAaa", "dep2p/v1/sys/peer/QmAaa"},
	}

	for _, tt := range tests {
		got := BuildKeyString(ScopeSys, tt.keyType, "", tt.payload)
		if got != tt.expected {
			t.Errorf("BuildKeyString(sys, %s, \"\", %s) = %q, want %q",
				tt.keyType, tt.payload, got, tt.expected)
		}
	}
}

// TestBuildKeyString_RealmScope 验证业务层 Key 字符串格式
func TestBuildKeyString_RealmScope(t *testing.T) {
	realmID := types.RealmID("my-blockchain")

	tests := []struct {
		keyType  string
		payload  string
		expected string
	}{
		{TypePeer, "QmXxx", "dep2p/v1/realm/my-blockchain/peer/QmXxx"},
		{TypeService, "chat", "dep2p/v1/realm/my-blockchain/service/chat"},
	}

	for _, tt := range tests {
		got := BuildKeyString(ScopeRealm, tt.keyType, realmID, tt.payload)
		if got != tt.expected {
			t.Errorf("BuildKeyString(realm, %s, %s, %s) = %q, want %q",
				tt.keyType, realmID, tt.payload, got, tt.expected)
		}
	}
}

// TestSystemKey 验证系统层 Key 计算
func TestSystemKey(t *testing.T) {
	payload := "QmTestNode"
	key := SystemKey(TypeBootstrap, payload)

	// 手动计算期望值
	keyString := "dep2p/v1/sys/bootstrap/QmTestNode"
	expected := sha256.Sum256([]byte(keyString))

	if !bytes.Equal(key, expected[:]) {
		t.Errorf("SystemKey mismatch")
		t.Errorf("  KeyString: %s", keyString)
		t.Errorf("  Got:       %x", key[:8])
		t.Errorf("  Expected:  %x", expected[:8])
	}
}

// TestRealmKey 验证业务层 Key 计算
func TestRealmKey(t *testing.T) {
	realmID := types.RealmID("test-realm")
	payload := "QmTestNode"
	key := RealmKey(TypePeer, realmID, payload)

	// 手动计算期望值
	keyString := "dep2p/v1/realm/test-realm/peer/QmTestNode"
	expected := sha256.Sum256([]byte(keyString))

	if !bytes.Equal(key, expected[:]) {
		t.Errorf("RealmKey mismatch")
		t.Errorf("  KeyString: %s", keyString)
		t.Errorf("  Got:       %x", key[:8])
		t.Errorf("  Expected:  %x", expected[:8])
	}
}

// TestRealmAwareDHTKey_NewFormat 验证新格式的 Realm 感知 DHT Key
func TestRealmAwareDHTKey_NewFormat(t *testing.T) {
	nodeID := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	nodeIDStr := nodeID.String()

	t.Run("WithRealm", func(t *testing.T) {
		realmID := types.RealmID("my-blockchain")
		key := RealmAwareDHTKey(realmID, nodeID)

		// 新格式: SHA256("dep2p/v1/realm/{realmID}/peer/{nodeID}")
		keyString := "dep2p/v1/realm/my-blockchain/peer/" + nodeIDStr
		expected := sha256.Sum256([]byte(keyString))

		if !bytes.Equal(key, expected[:]) {
			t.Errorf("RealmAwareDHTKey new format mismatch")
			t.Errorf("  KeyString: %s", keyString)
			t.Errorf("  Got:       %x", key[:8])
			t.Errorf("  Expected:  %x", expected[:8])
		}

		// 验证 Key 字符串包含正确的路径组件
		if !strings.Contains(keyString, "dep2p/v1/realm/") {
			t.Error("Key string should contain 'dep2p/v1/realm/'")
		}
	})

	t.Run("WithoutRealm", func(t *testing.T) {
		key := RealmAwareDHTKey(types.DefaultRealmID, nodeID)

		// 新格式: SHA256("dep2p/v1/sys/peer/{nodeID}")
		keyString := "dep2p/v1/sys/peer/" + nodeIDStr
		expected := sha256.Sum256([]byte(keyString))

		if !bytes.Equal(key, expected[:]) {
			t.Errorf("RealmAwareDHTKey without realm mismatch")
			t.Errorf("  KeyString: %s", keyString)
			t.Errorf("  Got:       %x", key[:8])
			t.Errorf("  Expected:  %x", expected[:8])
		}
	})

	t.Run("EmptyRealm", func(t *testing.T) {
		key := RealmAwareDHTKey("", nodeID)

		// 空 Realm 应该与 DefaultRealmID 相同
		keyDefault := RealmAwareDHTKey(types.DefaultRealmID, nodeID)
		if !bytes.Equal(key, keyDefault) {
			t.Error("Empty realm should produce same key as DefaultRealmID")
		}
	})
}

// TestRealmAwareDHTKey_Isolation 验证不同 Realm 产生不同 Key
func TestRealmAwareDHTKey_Isolation(t *testing.T) {
	nodeID := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16,
		17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}

	realm1 := types.RealmID("realm-a")
	realm2 := types.RealmID("realm-b")

	key1 := RealmAwareDHTKey(realm1, nodeID)
	key2 := RealmAwareDHTKey(realm2, nodeID)

	if bytes.Equal(key1, key2) {
		t.Error("Different realms should produce different keys")
	}

	// 系统层与业务层也应该不同
	keySys := RealmAwareDHTKey("", nodeID)
	if bytes.Equal(keySys, key1) {
		t.Error("System key should differ from realm key")
	}
}

// TestRealmAwareValueKey_NewFormat 验证新格式的值存储 Key
func TestRealmAwareValueKey_NewFormat(t *testing.T) {
	t.Run("WithRealm", func(t *testing.T) {
		realmID := types.RealmID("my-realm")
		keyType := TypeService
		keyData := []byte("chat")

		key := RealmAwareValueKey(realmID, keyType, keyData)

		// 新格式: SHA256("dep2p/v1/realm/{realmID}/{keyType}/{keyData}")
		keyString := "dep2p/v1/realm/my-realm/service/chat"
		expected := sha256.Sum256([]byte(keyString))

		if !bytes.Equal(key, expected[:]) {
			t.Errorf("RealmAwareValueKey new format mismatch")
			t.Errorf("  KeyString: %s", keyString)
			t.Errorf("  Got:       %x", key[:8])
			t.Errorf("  Expected:  %x", expected[:8])
		}
	})

	t.Run("WithoutRealm", func(t *testing.T) {
		keyType := TypeBootstrap
		keyData := []byte("node1")

		key := RealmAwareValueKey("", keyType, keyData)

		// 新格式: SHA256("dep2p/v1/sys/{keyType}/{keyData}")
		keyString := "dep2p/v1/sys/bootstrap/node1"
		expected := sha256.Sum256([]byte(keyString))

		if !bytes.Equal(key, expected[:]) {
			t.Errorf("RealmAwareValueKey without realm mismatch")
			t.Errorf("  KeyString: %s", keyString)
			t.Errorf("  Got:       %x", key[:8])
			t.Errorf("  Expected:  %x", expected[:8])
		}
	})
}

// ============================================================================
//                    Kademlia 距离计算测试
// ============================================================================

// TestXORDistance 验证 XOR 距离计算
func TestXORDistance(t *testing.T) {
	a := []byte{0xFF, 0x00, 0xAA}
	b := []byte{0x00, 0xFF, 0x55}

	dist := XORDistance(a, b)

	expected := []byte{0xFF, 0xFF, 0xFF}
	if !bytes.Equal(dist, expected) {
		t.Errorf("XORDistance mismatch: got %x, want %x", dist, expected)
	}
}

// TestCommonPrefixLength 验证公共前缀长度计算
func TestCommonPrefixLength(t *testing.T) {
	tests := []struct {
		a, b     []byte
		expected int
	}{
		{[]byte{0x80}, []byte{0x00}, 0},            // 第一位不同
		{[]byte{0x40}, []byte{0x00}, 1},            // 1 位相同
		{[]byte{0x00, 0x80}, []byte{0x00, 0x00}, 8}, // 8 位相同
		{[]byte{0x00, 0x00}, []byte{0x00, 0x00}, 16}, // 完全相同
	}

	for _, tt := range tests {
		got := CommonPrefixLength(tt.a, tt.b)
		if got != tt.expected {
			t.Errorf("CommonPrefixLength(%x, %x) = %d, want %d",
				tt.a, tt.b, got, tt.expected)
		}
	}
}

// ============================================================================
//                    Realm 隔离检查测试
// ============================================================================

// TestSameRealm 验证 Realm 比较
func TestSameRealm(t *testing.T) {
	tests := []struct {
		a, b     types.RealmID
		expected bool
	}{
		{"", "", true},
		{types.DefaultRealmID, "", true},
		{"realm-a", "realm-a", true},
		{"realm-a", "realm-b", false},
		{"realm-a", "", false},
		{"", "realm-b", false},
	}

	for _, tt := range tests {
		got := SameRealm(tt.a, tt.b)
		if got != tt.expected {
			t.Errorf("SameRealm(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.expected)
		}
	}
}

// TestIsCloser 验证距离比较
func TestIsCloser(t *testing.T) {
	target := []byte{0x00, 0x00}
	closer := []byte{0x01, 0x00}
	farther := []byte{0x02, 0x00}

	if !IsCloser(closer, farther, target) {
		t.Error("IsCloser should return true when first is closer")
	}

	if IsCloser(farther, closer, target) {
		t.Error("IsCloser should return false when first is farther")
	}
}

// ============================================================================
//                    Key 常量测试
// ============================================================================

// TestKeyConstants 验证 Key 常量定义正确
func TestKeyConstants(t *testing.T) {
	// 验证前缀
	if KeyPrefix != "dep2p/v1" {
		t.Errorf("KeyPrefix should be 'dep2p/v1', got %q", KeyPrefix)
	}

	// 验证 Scope
	if ScopeSys != "sys" {
		t.Errorf("ScopeSys should be 'sys', got %q", ScopeSys)
	}
	if ScopeRealm != "realm" {
		t.Errorf("ScopeRealm should be 'realm', got %q", ScopeRealm)
	}

	// 验证类型
	expectedTypes := map[string]string{
		"TypePeer":       TypePeer,
		"TypeService":    TypeService,
		"TypeBootstrap":  TypeBootstrap,
		"TypeRelay":      TypeRelay,
		"TypeNAT":        TypeNAT,
		"TypeRendezvous": TypeRendezvous,
	}

	for name, value := range expectedTypes {
		if value == "" {
			t.Errorf("%s should not be empty", name)
		}
	}
}

// ============================================================================
//                    Namespace 归一化测试 (Layer1 修复)
// ============================================================================

// TestNormalizeNamespace 验证 namespace 归一化
func TestNormalizeNamespace(t *testing.T) {
	tests := []struct {
		name              string
		input             string
		expectedScope     KeyScope
		expectedNamespace string
	}{
		// 正常用法
		{"simple namespace", "relay", KeyScopeSys, "relay"},
		{"namespace with path", "service/chat", KeyScopeSys, "service/chat"},

		// 显式 sys: 前缀
		{"sys: prefix", "sys:relay", KeyScopeSys, "relay"},
		{"sys: prefix with path", "sys:service/chat", KeyScopeSys, "service/chat"},

		// 错误用法（应该被归一化）
		{"wrong sys/ prefix", "sys/relay", KeyScopeSys, "relay"},
		{"wrong sys/ prefix with path", "sys/service/chat", KeyScopeSys, "service/chat"},

		// 边界情况
		{"empty namespace", "", KeyScopeSys, ""},
		{"whitespace", "  relay  ", KeyScopeSys, "relay"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := NormalizeNamespace(tt.input)
			if parsed.Scope != tt.expectedScope {
				t.Errorf("NormalizeNamespace(%q).Scope = %v, want %v",
					tt.input, parsed.Scope, tt.expectedScope)
			}
			if parsed.Namespace != tt.expectedNamespace {
				t.Errorf("NormalizeNamespace(%q).Namespace = %q, want %q",
					tt.input, parsed.Namespace, tt.expectedNamespace)
			}
		})
	}
}

// TestBuildProviderKey 验证 provider key 构建
func TestBuildProviderKey(t *testing.T) {
	tests := []struct {
		name      string
		scope     KeyScope
		realmID   string
		namespace string
		expected  string
	}{
		// sys 域
		{"sys simple", KeyScopeSys, "", "relay", "dep2p/v1/sys/relay"},
		{"sys with path", KeyScopeSys, "", "service/chat", "dep2p/v1/sys/service/chat"},
		{"sys ignores realmID", KeyScopeSys, "my-realm", "relay", "dep2p/v1/sys/relay"},

		// realm 域
		{"realm simple", KeyScopeRealm, "my-realm", "relay", "dep2p/v1/realm/my-realm/relay"},
		{"realm with path", KeyScopeRealm, "my-realm", "service/chat", "dep2p/v1/realm/my-realm/service/chat"},

		// realm 域但 realmID 为空（回退到 sys）
		{"realm empty realmID fallback", KeyScopeRealm, "", "relay", "dep2p/v1/sys/relay"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildProviderKey(tt.scope, tt.realmID, tt.namespace)
			if got != tt.expected {
				t.Errorf("BuildProviderKey(%v, %q, %q) = %q, want %q",
					tt.scope, tt.realmID, tt.namespace, got, tt.expected)
			}
		})
	}
}

// TestBuildProviderKeyWithParsed 验证解析后构建 key 的完整流程
func TestBuildProviderKeyWithParsed(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		realmID   string
		expected  string
	}{
		// 正常用法 - 不会产生双前缀
		{"simple", "relay", "", "dep2p/v1/sys/relay"},

		// 防止双前缀
		{"prevent double prefix sys:", "sys:relay", "", "dep2p/v1/sys/relay"},
		{"prevent double prefix sys/", "sys/relay", "", "dep2p/v1/sys/relay"},

		// 验证不会产生 dep2p/v1/sys/sys/relay
		{"no sys/sys", "sys/relay", "", "dep2p/v1/sys/relay"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed := NormalizeNamespace(tt.namespace)
			got := BuildProviderKeyWithParsed(parsed, tt.realmID)
			if got != tt.expected {
				t.Errorf("BuildProviderKeyWithParsed(NormalizeNamespace(%q), %q) = %q, want %q",
					tt.namespace, tt.realmID, got, tt.expected)
			}

			// 确保不会产生双 sys 前缀
			if strings.Contains(got, "sys/sys") {
				t.Errorf("Key should not contain 'sys/sys': %q", got)
			}
		})
	}
}
