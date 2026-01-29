package pubsub

import (
	"context"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscription_Next(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	realm := newMockRealm("realm-1", "Test Realm")
	realmMgr.AddRealm(realm)

	svc, err := New(host, realmMgr, WithDisableHeartbeat(true))
	require.NoError(t, err)

	ctx := context.Background()
	err = svc.Start(ctx)
	require.NoError(t, err)
	defer svc.Close()

	topic, err := svc.Join("test-topic")
	require.NoError(t, err)

	sub, err := topic.Subscribe()
	require.NoError(t, err)

	// 超时 context
	ctxTimeout, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()

	// 应该超时
	_, err = sub.Next(ctxTimeout)
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestSubscription_Cancel(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()

	realm := newMockRealm("realm-1", "Test Realm")
	realmMgr.AddRealm(realm)

	svc, err := New(host, realmMgr, WithDisableHeartbeat(true))
	require.NoError(t, err)

	ctx := context.Background()
	err = svc.Start(ctx)
	require.NoError(t, err)
	defer svc.Close()

	topic, err := svc.Join("test-topic")
	require.NoError(t, err)

	sub, err := topic.Subscribe()
	require.NoError(t, err)

	// 取消订阅
	sub.Cancel()

	// 下次调用应失败
	_, err = sub.Next(ctx)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrSubscriptionCancelled)
}

func TestSubscription_PushMessage(t *testing.T) {
	topicObj := &topic{
		name: "test",
	}

	sub := newTopicSubscription(topicObj, 10)

	msg := &interfaces.Message{
		From:  "peer-1",
		Data:  []byte("test"),
		Topic: "test",
	}

	// 推送消息
	ok := sub.pushMessage(msg)
	assert.True(t, ok)

	// 接收消息
	ctx := context.Background()
	received, err := sub.Next(ctx)
	require.NoError(t, err)
	assert.Equal(t, msg.From, received.From)
	assert.Equal(t, msg.Data, received.Data)
}

func TestSubscription_BufferFull(t *testing.T) {
	topicObj := &topic{
		name: "test",
	}

	// 小缓冲区
	sub := newTopicSubscription(topicObj, 2)

	msg := &interfaces.Message{
		From:  "peer-1",
		Data:  []byte("test"),
		Topic: "test",
	}

	// 填满缓冲区
	assert.True(t, sub.pushMessage(msg))
	assert.True(t, sub.pushMessage(msg))

	// 缓冲区满,应该丢弃
	assert.False(t, sub.pushMessage(msg))
}

// ============================================================================
//                       isCancelled 测试（补充 0% 覆盖）
// ============================================================================

// TestSubscription_isCancelled_NotCancelled 测试未取消状态
func TestSubscription_isCancelled_NotCancelled(t *testing.T) {
	topicObj := &topic{
		name: "test",
	}

	sub := newTopicSubscription(topicObj, 10)

	// 未取消时应返回 false
	assert.False(t, sub.isCancelled())
	t.Log("✅ isCancelled 未取消状态测试通过")
}

// TestSubscription_isCancelled_Cancelled 测试已取消状态
func TestSubscription_isCancelled_Cancelled(t *testing.T) {
	topicObj := &topic{
		name: "test",
	}

	sub := newTopicSubscription(topicObj, 10)

	// 取消订阅
	sub.Cancel()

	// 取消后应返回 true
	assert.True(t, sub.isCancelled())
	t.Log("✅ isCancelled 已取消状态测试通过")
}

// TestSubscription_pushMessage_AfterCancel 测试取消后推送消息
func TestSubscription_pushMessage_AfterCancel(t *testing.T) {
	topicObj := &topic{
		name: "test",
	}

	sub := newTopicSubscription(topicObj, 10)

	// 先取消
	sub.Cancel()

	msg := &interfaces.Message{
		From:  "peer-1",
		Data:  []byte("test"),
		Topic: "test",
	}

	// 取消后推送应失败
	ok := sub.pushMessage(msg)
	assert.False(t, ok)
	t.Log("✅ pushMessage 取消后测试通过")
}

// TestSubscription_Cancel_Idempotent 测试 Cancel 幂等性
func TestSubscription_Cancel_Idempotent(t *testing.T) {
	topicObj := &topic{
		name: "test",
	}

	sub := newTopicSubscription(topicObj, 10)

	// 多次取消不应 panic
	sub.Cancel()
	sub.Cancel()
	sub.Cancel()

	assert.True(t, sub.isCancelled())
	t.Log("✅ Cancel 幂等性测试通过")
}

// TestSubscription_DefaultBufferSize 测试默认缓冲区大小
func TestSubscription_DefaultBufferSize(t *testing.T) {
	topicObj := &topic{
		name: "test",
	}

	// 使用 0 作为 bufferSize，应该使用默认值 32
	sub := newTopicSubscription(topicObj, 0)

	// 应该能推送多条消息
	msg := &interfaces.Message{
		From:  "peer-1",
		Data:  []byte("test"),
		Topic: "test",
	}

	for i := 0; i < 32; i++ {
		ok := sub.pushMessage(msg)
		assert.True(t, ok, "第 %d 条消息推送失败", i+1)
	}

	// 第 33 条应该失败（缓冲区满）
	ok := sub.pushMessage(msg)
	assert.False(t, ok)

	t.Log("✅ 默认缓冲区大小测试通过")
}
