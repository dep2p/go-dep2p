package realm

import (
	"testing"

	"github.com/dep2p/go-dep2p/internal/config"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              RealmMessagingWrapper 测试
// ============================================================================

func TestRealmMessagingWrapper_NewWrapper(t *testing.T) {
	cfg := config.RealmConfig{
		RealmAuthEnabled: false,
	}
	manager := NewManager(cfg, nil)

	wrapper := NewRealmMessagingWrapper(manager, nil)
	if wrapper == nil {
		t.Fatal("wrapper should not be nil")
	}
}

func TestRealmMessagingWrapper_NilManager(t *testing.T) {
	wrapper := NewRealmMessagingWrapper(nil, nil)
	if wrapper == nil {
		t.Fatal("wrapper should handle nil manager")
	}
}

func TestRealmMessagingWrapper_BuildRealmTopic(t *testing.T) {
	cfg := config.RealmConfig{
		RealmAuthEnabled: false,
	}
	manager := NewManager(cfg, nil)
	wrapper := NewRealmMessagingWrapper(manager, nil)

	tests := []struct {
		realmID  types.RealmID
		topic    string
		expected string
	}{
		{"realm-1", "topic-a", "realm/realm-1/topic-a"},
		{"my-realm", "events", "realm/my-realm/events"},
		{"", "empty-realm", "realm//empty-realm"},
	}

	for _, tt := range tests {
		result := wrapper.buildRealmTopic(tt.realmID, tt.topic)
		if result != tt.expected {
			t.Errorf("buildRealmTopic(%q, %q) = %q, want %q",
				tt.realmID, tt.topic, result, tt.expected)
		}
	}
}

func TestRealmMessagingWrapper_Close(t *testing.T) {
	wrapper := NewRealmMessagingWrapper(nil, nil)

	// 第一次关闭应该成功
	err := wrapper.Close()
	if err != nil {
		t.Errorf("first close failed: %v", err)
	}

	// 重复关闭应该安全（幂等）
	err = wrapper.Close()
	if err != nil {
		t.Errorf("second close failed: %v", err)
	}
}

// ============================================================================
//                              RealmNamespace 辅助函数测试
// ============================================================================

func TestRealmTopicNamespace_Messaging(t *testing.T) {
	tests := []struct {
		realmID  types.RealmID
		topic    string
		expected string
	}{
		{"test", "my-topic", "realm/test/topic/my-topic"},
		{"realm-1", "events", "realm/realm-1/topic/events"},
		{"", "empty", "realm//topic/empty"},
	}

	for _, tt := range tests {
		result := RealmTopicNamespace(tt.realmID, tt.topic)
		if result != tt.expected {
			t.Errorf("RealmTopicNamespace(%q, %q) = %q, want %q",
				tt.realmID, tt.topic, result, tt.expected)
		}
	}
}

// ============================================================================
//                              Manager 与 Messaging 集成测试
// ============================================================================

func TestManager_SetDiscoveryAndLiveness(t *testing.T) {
	cfg := config.RealmConfig{
		RealmAuthEnabled: false,
	}
	manager := NewManager(cfg, nil)

	// 测试 SetDiscovery 不 panic
	manager.SetDiscovery(nil)

	// 测试 SetLiveness 不 panic
	manager.SetLiveness(nil)
}

func TestManager_RealmDHTKey(t *testing.T) {
	cfg := config.RealmConfig{
		RealmAuthEnabled: false,
	}
	manager := NewManager(cfg, nil)

	nodeID := types.NodeID{1, 2, 3, 4, 5}
	realmID := types.RealmID("test-realm")

	key := manager.RealmDHTKey(nodeID, realmID)
	if len(key) != 32 {
		t.Errorf("expected 32 byte key, got %d", len(key))
	}

	// 相同输入应该产生相同输出
	key2 := manager.RealmDHTKey(nodeID, realmID)
	for i := range key {
		if key[i] != key2[i] {
			t.Error("same input should produce same key")
			break
		}
	}
}

func TestManager_RealmMetadata_NotMember(t *testing.T) {
	cfg := config.RealmConfig{
		RealmAuthEnabled: false,
	}
	manager := NewManager(cfg, nil)

	_, err := manager.RealmMetadata(types.RealmID("unknown"))
	if err != ErrNotMember {
		t.Errorf("expected ErrNotMember, got %v", err)
	}
}
