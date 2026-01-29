package pubsub

import (
	"context"
	"testing"

	pb "github.com/dep2p/go-dep2p/pkg/lib/proto/gossipsub"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidator_Validate_ValidMessage(t *testing.T) {
	realmMgr := newMockRealmManager()
	realm := newMockRealm("realm-1", "Test Realm")
	realm.AddMember("peer-1")
	realmMgr.AddRealm(realm)

	validator := newMessageValidator(realmMgr, 1024*1024)

	msg := &pb.Message{
		From:  []byte("peer-1"),
		Data:  []byte("test data"),
		Topic: "test-topic",
		Seqno: []byte{1, 2, 3},
	}

	ctx := context.Background()
	err := validator.Validate(ctx, "peer-1", msg)
	require.NoError(t, err)
}

func TestValidator_Validate_MessageTooLarge(t *testing.T) {
	realmMgr := newMockRealmManager()
	validator := newMessageValidator(realmMgr, 100) // 小上限

	msg := &pb.Message{
		From:  []byte("peer-1"),
		Data:  make([]byte, 200), // 超过上限
		Topic: "test-topic",
		Seqno: []byte{1, 2, 3},
	}

	ctx := context.Background()
	err := validator.Validate(ctx, "peer-1", msg)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrMessageTooLarge)
}

func TestValidator_Validate_MissingFields(t *testing.T) {
	realmMgr := newMockRealmManager()
	validator := newMessageValidator(realmMgr, 1024*1024)

	ctx := context.Background()

	// 缺少 From
	msg1 := &pb.Message{
		Data:  []byte("test"),
		Topic: "topic",
		Seqno: []byte{1},
	}
	err := validator.Validate(ctx, "peer-1", msg1)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidMessage)

	// 缺少 Data
	msg2 := &pb.Message{
		From:  []byte("peer-1"),
		Topic: "topic",
		Seqno: []byte{1},
	}
	err = validator.Validate(ctx, "peer-1", msg2)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidMessage)

	// 缺少 Topic
	msg3 := &pb.Message{
		From:  []byte("peer-1"),
		Data:  []byte("test"),
		Seqno: []byte{1},
	}
	err = validator.Validate(ctx, "peer-1", msg3)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidMessage)
}

func TestValidator_Validate_NotRealmMember(t *testing.T) {
	realmMgr := newMockRealmManager()
	validator := newMessageValidator(realmMgr, 1024*1024)

	msg := &pb.Message{
		From:  []byte("peer-unknown"),
		Data:  []byte("test"),
		Topic: "topic",
		Seqno: []byte{1},
	}

	ctx := context.Background()
	err := validator.Validate(ctx, "peer-1", msg)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrNotRealmMember)
}

func TestValidator_RegisterValidator(t *testing.T) {
	realmMgr := newMockRealmManager()
	realm := newMockRealm("realm-1", "Test Realm")
	realm.AddMember("peer-1")
	realmMgr.AddRealm(realm)

	validator := newMessageValidator(realmMgr, 1024*1024)

	// 注册自定义验证器
	customValidatorCalled := false
	validator.RegisterValidator("test-topic", func(ctx context.Context, peerID string, msg *pb.Message) bool {
		customValidatorCalled = true
		return msg.Data[0] == 'A' // 只接受以 'A' 开头的消息
	})

	ctx := context.Background()

	// 有效消息
	msg1 := &pb.Message{
		From:  []byte("peer-1"),
		Data:  []byte("A test"),
		Topic: "test-topic",
		Seqno: []byte{1},
	}
	err := validator.Validate(ctx, "peer-1", msg1)
	require.NoError(t, err)
	assert.True(t, customValidatorCalled)

	// 无效消息
	customValidatorCalled = false
	msg2 := &pb.Message{
		From:  []byte("peer-1"),
		Data:  []byte("B test"),
		Topic: "test-topic",
		Seqno: []byte{2},
	}
	err = validator.Validate(ctx, "peer-1", msg2)
	assert.Error(t, err)
	assert.True(t, customValidatorCalled)
}

func TestValidator_UnregisterValidator(t *testing.T) {
	realmMgr := newMockRealmManager()
	realm := newMockRealm("realm-1", "Test Realm")
	realm.AddMember("peer-1")
	realmMgr.AddRealm(realm)

	validator := newMessageValidator(realmMgr, 1024*1024)

	// 注册验证器
	validator.RegisterValidator("test-topic", func(ctx context.Context, peerID string, msg *pb.Message) bool {
		return false // 总是拒绝
	})

	ctx := context.Background()

	msg := &pb.Message{
		From:  []byte("peer-1"),
		Data:  []byte("test"),
		Topic: "test-topic",
		Seqno: []byte{1},
	}

	// 验证应失败
	err := validator.Validate(ctx, "peer-1", msg)
	assert.Error(t, err)

	// 注销验证器
	validator.UnregisterValidator("test-topic")

	// 现在应该通过
	err = validator.Validate(ctx, "peer-1", msg)
	require.NoError(t, err)
}

// TestValidator_SystemTopic_SkipSenderValidation 测试系统 topic 跳过发送者验证
//
// 系统 topic 用于成员同步，必须允许：
//   - 转发者是成员时，接受来自任何发送者的消息
//   - 这解决了成员同步的鸡生蛋问题
//
// 系统 topic 格式：
//   - 旧格式：__sys__/members（向后兼容）
//   - 新格式：/dep2p/realm/<realmID>/members（Step B2 对齐）
func TestValidator_SystemTopic_SkipSenderValidation(t *testing.T) {
	realmMgr := newMockRealmManager()
	realm := newMockRealm("realm-1", "Test Realm")
	realm.AddMember("trusted-forwarder") // 转发者是成员
	// 注意：unknown-sender 不是成员
	realmMgr.AddRealm(realm)

	validator := newMessageValidator(realmMgr, 1024*1024)
	ctx := context.Background()

	// 场景 1：旧格式系统 topic 消息，发送者不是成员，但转发者是成员
	// 应该通过验证
	sysMsg := &pb.Message{
		From:  []byte("unknown-sender"), // 不是成员
		Data:  []byte("join:new-peer"),
		Topic: "__sys__/members", // 旧格式系统 topic
		Seqno: []byte{1},
	}
	err := validator.Validate(ctx, "trusted-forwarder", sysMsg) // 转发者是成员
	assert.NoError(t, err, "系统 topic 应跳过发送者验证，只验证转发者")

	// 场景 1b：新格式 Realm 作用域 topic（Step B2 对齐）
	realmScopedMsg := &pb.Message{
		From:  []byte("unknown-sender"),
		Data:  []byte("join2:{\"peer_id\":\"new-peer\"}"),
		Topic: "/dep2p/realm/realm-1/members", // 新格式
		Seqno: []byte{1},
	}
	err = validator.Validate(ctx, "trusted-forwarder", realmScopedMsg)
	assert.NoError(t, err, "Realm 作用域 topic 应跳过发送者验证")

	// 场景 2：系统 topic 消息，转发者也不是成员
	// 应该失败
	err = validator.Validate(ctx, "untrusted-forwarder", sysMsg) // 转发者不是成员
	assert.Error(t, err, "系统 topic 仍需验证转发者是成员")
	assert.ErrorIs(t, err, ErrNotRealmMember)

	// 场景 3：普通 topic 消息，发送者不是成员
	// 应该失败
	normalMsg := &pb.Message{
		From:  []byte("unknown-sender"), // 不是成员
		Data:  []byte("hello"),
		Topic: "chat/general", // 普通 topic
		Seqno: []byte{2},
	}
	err = validator.Validate(ctx, "trusted-forwarder", normalMsg)
	assert.Error(t, err, "普通 topic 应验证发送者是成员")
	assert.ErrorIs(t, err, ErrNotRealmMember)
}

// TestIsSystemTopic 测试系统 topic 判断
func TestIsSystemTopic(t *testing.T) {
	tests := []struct {
		topic    string
		expected bool
	}{
		// 旧格式（向后兼容）
		{"__sys__/members", true},
		{"__sys__/config", true},
		{"__sys__/heartbeat", true},

		// Step B2 对齐：新的 Realm 作用域成员同步 Topic
		{"/dep2p/realm/abc123/members", true},
		{"/dep2p/realm/test-realm-id/members", true},
		{"/dep2p/realm/64char-hex-realm-id-here-0123456789abcdef/members", true},

		// 非系统 topic
		{"chat/general", false},
		{"room/123", false},
		{"__sys__", false},           // 没有后缀
		{"__sys_/test", false},       // 前缀不完整
		{"", false},
		{"/dep2p/realm//members", false},  // 空 realmID
		{"/dep2p/realm/abc", false},       // 没有 /members 后缀
		{"/dep2p/realm/members", false},   // 缺少 realmID
	}

	for _, tt := range tests {
		t.Run(tt.topic, func(t *testing.T) {
			result := isSystemTopic(tt.topic)
			assert.Equal(t, tt.expected, result, "topic: %s", tt.topic)
		})
	}
}
