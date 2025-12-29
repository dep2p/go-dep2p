// Package realm 提供 Realm 管理实现
package realm

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              服务适配器测试（IMPL-1227 Phase 7）
// ============================================================================

// TestRealmMessaging_ProtocolPrefix 测试协议前缀自动添加
func TestRealmMessaging_ProtocolPrefix(t *testing.T) {
	// 创建测试用的 realmKey 和 RealmID
	realmKey := types.GenerateRealmKey()
	realmID := types.DeriveRealmID(realmKey)

	// 测试用户协议前缀生成
	userProto := "chat/1.0.0"
	fullProto := protocolids.FullAppProtocol(realmID, userProto)

	// 验证前缀格式
	expectedPrefix := "/dep2p/app/" + string(realmID) + "/"
	if len(string(fullProto)) <= len(expectedPrefix) {
		t.Errorf("fullProto too short: %s", fullProto)
	}

	// 验证完整协议包含用户协议
	expected := expectedPrefix + userProto
	if string(fullProto) != expected {
		t.Errorf("expected %s, got %s", expected, fullProto)
	}

	t.Logf("协议前缀测试通过: %s", fullProto)
}

// TestRealmMessaging_ValidateUserProtocol 测试用户协议验证
func TestRealmMessaging_ValidateUserProtocol(t *testing.T) {
	realmKey := types.GenerateRealmKey()
	realmID := types.DeriveRealmID(realmKey)

	testCases := []struct {
		name     string
		protocol string
		wantErr  bool
	}{
		{"valid simple", "chat/1.0.0", false},
		{"valid nested", "file/transfer/1.0.0", false},
		{"invalid sys prefix", "/dep2p/sys/echo/1.0.0", true},
		{"invalid realm prefix", "/dep2p/realm/xxx/proto", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := protocolids.ValidateUserProtocol(tc.protocol, realmID)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateUserProtocol(%q) error = %v, wantErr %v", tc.protocol, err, tc.wantErr)
			}
		})
	}
}

// TestRealmPubSub_TopicPrefix 测试 Topic 前缀自动添加
func TestRealmPubSub_TopicPrefix(t *testing.T) {
	realmKey := types.GenerateRealmKey()
	realmID := types.DeriveRealmID(realmKey)

	// 测试 Topic 前缀生成（与协议格式相同）
	userTopic := "blocks"
	fullTopic := string(protocolids.FullAppProtocol(realmID, userTopic))

	// 验证前缀格式
	expectedPrefix := "/dep2p/app/" + string(realmID) + "/"
	expected := expectedPrefix + userTopic
	if fullTopic != expected {
		t.Errorf("expected %s, got %s", expected, fullTopic)
	}

	t.Logf("Topic 前缀测试通过: %s", fullTopic)
}

// TestProtocolids_IsSystemProtocol 测试系统协议识别
func TestProtocolids_IsSystemProtocol(t *testing.T) {
	testCases := []struct {
		proto    types.ProtocolID
		expected bool
	}{
		{protocolids.SysEcho, true},
		{"/dep2p/sys/test/1.0.0", true},
		{"/dep2p/app/xxx/chat/1.0.0", false},
		{"/dep2p/realm/xxx/auth/1.0.0", false},
		{"random/proto", false},
	}

	for _, tc := range testCases {
		t.Run(string(tc.proto), func(t *testing.T) {
			result := protocolids.IsSystemProtocol(tc.proto)
			if result != tc.expected {
				t.Errorf("IsSystemProtocol(%q) = %v, expected %v", tc.proto, result, tc.expected)
			}
		})
	}
}

// TestProtocolids_IsAppProtocol 测试应用协议识别
func TestProtocolids_IsAppProtocol(t *testing.T) {
	realmKey := types.GenerateRealmKey()
	realmID := types.DeriveRealmID(realmKey)
	fullProto := protocolids.FullAppProtocol(realmID, "chat/1.0.0")

	testCases := []struct {
		proto    types.ProtocolID
		expected bool
	}{
		{fullProto, true},
		{"/dep2p/app/somerealmid/chat/1.0.0", true},
		{"/dep2p/sys/echo/1.0.0", false},
		{"/dep2p/realm/xxx/auth/1.0.0", false},
	}

	for _, tc := range testCases {
		t.Run(string(tc.proto), func(t *testing.T) {
			result := protocolids.IsAppProtocol(tc.proto)
			if result != tc.expected {
				t.Errorf("IsAppProtocol(%q) = %v, expected %v", tc.proto, result, tc.expected)
			}
		})
	}
}

// TestProtocolids_BelongsToRealm 测试协议所属 Realm 判断
func TestProtocolids_BelongsToRealm(t *testing.T) {
	realmKey1 := types.GenerateRealmKey()
	realmKey2 := types.GenerateRealmKey()
	realmID1 := types.DeriveRealmID(realmKey1)
	realmID2 := types.DeriveRealmID(realmKey2)

	proto1 := protocolids.FullAppProtocol(realmID1, "chat/1.0.0")

	// 应该属于 realmID1
	if !protocolids.BelongsToRealm(proto1, realmID1) {
		t.Error("proto1 should belong to realmID1")
	}

	// 不应该属于 realmID2
	if protocolids.BelongsToRealm(proto1, realmID2) {
		t.Error("proto1 should not belong to realmID2")
	}

	// 系统协议不属于任何 Realm
	if protocolids.BelongsToRealm(protocolids.SysEcho, realmID1) {
		t.Error("system protocol should not belong to any realm")
	}

	t.Log("协议所属 Realm 判断测试通过")
}

// TestProtocolids_ExtractRealmID 测试从协议中提取 RealmID
func TestProtocolids_ExtractRealmID(t *testing.T) {
	realmKey := types.GenerateRealmKey()
	realmID := types.DeriveRealmID(realmKey)
	proto := protocolids.FullAppProtocol(realmID, "chat/1.0.0")

	// 从协议中提取 RealmID
	extracted, err := protocolids.ExtractRealmID(proto)
	if err != nil {
		t.Fatalf("ExtractRealmID failed: %v", err)
	}

	if extracted != realmID {
		t.Errorf("extracted RealmID mismatch: expected %s, got %s", realmID, extracted)
	}

	// 系统协议应该返回错误
	_, err = protocolids.ExtractRealmID(protocolids.SysEcho)
	if err == nil {
		t.Error("ExtractRealmID should fail for system protocol")
	}

	t.Log("RealmID 提取测试通过")
}

// TestRealmStreams_HandlerProtocolPrefix 测试流处理器协议前缀
func TestRealmStreams_HandlerProtocolPrefix(t *testing.T) {
	realmKey := types.GenerateRealmKey()
	realmID := types.DeriveRealmID(realmKey)

	// 用户注册的协议
	userProto := "file-transfer/1.0.0"
	fullProto := protocolids.FullAppProtocol(realmID, userProto)

	// 验证完整协议格式
	if !protocolids.IsAppProtocol(fullProto) {
		t.Error("fullProto should be an app protocol")
	}

	if !protocolids.BelongsToRealm(fullProto, realmID) {
		t.Error("fullProto should belong to the realm")
	}

	t.Logf("流处理器协议前缀测试通过: %s", fullProto)
}

// ============================================================================
//                              realmTopic.Peers() 测试
// ============================================================================

// TestRealmTopic_Peers 测试 Topic.Peers() 方法
func TestRealmTopic_Peers(t *testing.T) {
	t.Run("left topic returns nil", func(t *testing.T) {
		topic := &realmTopic{
			left: true,
		}
		peers := topic.Peers()
		if peers != nil {
			t.Error("left topic should return nil peers")
		}
	})

	t.Run("nil messagingSvc returns nil", func(t *testing.T) {
		realm := &realmImpl{
			messagingSvc: nil,
		}
		topic := &realmTopic{
			realm:    realm,
			name:     "test-topic",
			fullName: "/dep2p/app/realm123/test-topic",
			left:     false,
		}
		peers := topic.Peers()
		if peers != nil {
			t.Error("nil messagingSvc should return nil peers")
		}
	})
}

