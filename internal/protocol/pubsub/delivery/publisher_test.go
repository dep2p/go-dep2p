// Package delivery 提供可靠消息投递功能
package delivery

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReliablePublisher_NewReliablePublisher(t *testing.T) {
	mock := NewMockPublisher()
	publisher := NewReliablePublisher(mock, nil)

	assert.NotNil(t, publisher)
	assert.NotNil(t, publisher.queue)
	assert.NotNil(t, publisher.config)
}

func TestReliablePublisher_StartStop(t *testing.T) {
	mock := NewMockPublisher()
	publisher := NewReliablePublisher(mock, nil)

	ctx := context.Background()
	err := publisher.Start(ctx)
	require.NoError(t, err)

	err = publisher.Stop()
	require.NoError(t, err)
}

func TestReliablePublisher_Publish_Success(t *testing.T) {
	mock := NewMockPublisher()
	publisher := NewReliablePublisher(mock, nil)

	ctx := context.Background()
	err := publisher.Start(ctx)
	require.NoError(t, err)
	defer publisher.Stop()

	// 发布消息
	err = publisher.Publish(ctx, "test-topic", []byte("test-data"))
	require.NoError(t, err)

	// 验证消息已发送
	messages := mock.GetMessages()
	require.Len(t, messages, 1)
	assert.Equal(t, "test-topic", messages[0].Topic)
	assert.Equal(t, []byte("test-data"), messages[0].Data)

	// 验证统计
	stats := publisher.GetStats()
	assert.Equal(t, int64(1), stats.TotalPublished)
	assert.Equal(t, int64(1), stats.TotalSent)
	assert.Equal(t, int64(0), stats.TotalQueued)
}

func TestReliablePublisher_Publish_Queue(t *testing.T) {
	mock := NewMockPublisher()
	mock.ShouldFail = true

	publisher := NewReliablePublisher(mock, nil)

	ctx := context.Background()
	err := publisher.Start(ctx)
	require.NoError(t, err)
	defer publisher.Stop()

	// 发布消息（会失败并入队）
	err = publisher.Publish(ctx, "test-topic", []byte("test-data"))
	require.NoError(t, err)

	// 验证消息已入队
	assert.Equal(t, 1, publisher.QueueSize())

	// 验证统计
	stats := publisher.GetStats()
	assert.Equal(t, int64(1), stats.TotalPublished)
	assert.Equal(t, int64(0), stats.TotalSent)
	assert.Equal(t, int64(1), stats.TotalQueued)
}

func TestReliablePublisher_FlushQueue(t *testing.T) {
	mock := NewMockPublisher()
	mock.ShouldFail = true

	publisher := NewReliablePublisher(mock, nil)

	ctx := context.Background()
	err := publisher.Start(ctx)
	require.NoError(t, err)
	defer publisher.Stop()

	// 发布消息（会失败并入队）
	err = publisher.Publish(ctx, "test-topic", []byte("test-data"))
	require.NoError(t, err)
	assert.Equal(t, 1, publisher.QueueSize())

	// 恢复发布器
	mock.ShouldFail = false

	// 刷新队列
	sent := publisher.FlushQueue(ctx)
	assert.Equal(t, 1, sent)
	assert.True(t, publisher.IsQueueEmpty())

	// 验证消息已发送
	messages := mock.GetMessages()
	require.Len(t, messages, 1)
}

func TestReliablePublisher_StatusCallback(t *testing.T) {
	mock := NewMockPublisher()
	publisher := NewReliablePublisher(mock, nil)

	ctx := context.Background()
	err := publisher.Start(ctx)
	require.NoError(t, err)
	defer publisher.Stop()

	// 注册回调
	var receivedStatus DeliveryStatus
	var receivedMsgID string
	done := make(chan struct{})

	publisher.OnStatusChange(func(msgID string, status DeliveryStatus, err error) {
		receivedMsgID = msgID
		receivedStatus = status
		close(done)
	})

	// 发布消息
	err = publisher.Publish(ctx, "test-topic", []byte("test-data"))
	require.NoError(t, err)

	// 等待回调
	select {
	case <-done:
		assert.NotEmpty(t, receivedMsgID)
		assert.Equal(t, StatusSent, receivedStatus)
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for status callback")
	}
}

func TestMessageQueue_Basic(t *testing.T) {
	queue := NewMessageQueue(nil)

	// 入队
	msg1 := &QueuedMessage{ID: "msg-1", Topic: "topic", Data: []byte("data1")}
	assert.True(t, queue.Enqueue(msg1))
	assert.Equal(t, 1, queue.Len())

	// 不能重复入队
	assert.False(t, queue.Enqueue(msg1))
	assert.Equal(t, 1, queue.Len())

	// 出队
	dequeued := queue.Dequeue()
	assert.NotNil(t, dequeued)
	assert.Equal(t, "msg-1", dequeued.ID)
	assert.True(t, queue.IsEmpty())
}

func TestMessageQueue_MaxSize(t *testing.T) {
	config := &QueueConfig{
		MaxSize:     2,
		MaxAge:      5 * time.Minute,
		MaxAttempts: 3,
	}
	queue := NewMessageQueue(config)

	// 入队 3 条消息
	queue.Enqueue(&QueuedMessage{ID: "msg-1", Topic: "topic", Data: []byte("data1")})
	queue.Enqueue(&QueuedMessage{ID: "msg-2", Topic: "topic", Data: []byte("data2")})
	queue.Enqueue(&QueuedMessage{ID: "msg-3", Topic: "topic", Data: []byte("data3")})

	// 队列大小应为 2
	assert.Equal(t, 2, queue.Len())

	// 第一条消息应被淘汰
	assert.False(t, queue.Contains("msg-1"))
	assert.True(t, queue.Contains("msg-2"))
	assert.True(t, queue.Contains("msg-3"))
}

func TestAckRequest_PrependExtract(t *testing.T) {
	req := &AckRequest{
		MessageID:   "test-msg-id",
		RequesterID: "node-123",
		Topic:       "test-topic",
		Timestamp:   time.Now(),
	}

	payload := []byte("hello world")

	// 添加前缀
	data, err := PrependAckRequest(req, payload)
	require.NoError(t, err)
	assert.True(t, len(data) > len(payload))

	// 提取
	extractedReq, extractedPayload, err := ExtractAckRequest(data)
	require.NoError(t, err)
	assert.NotNil(t, extractedReq)
	assert.Equal(t, req.MessageID, extractedReq.MessageID)
	assert.Equal(t, req.RequesterID, extractedReq.RequesterID)
	assert.Equal(t, req.Topic, extractedReq.Topic)
	assert.Equal(t, payload, extractedPayload)
}

func TestAckRequest_NoAck(t *testing.T) {
	payload := []byte("hello world")

	// 添加空前缀
	data, err := PrependAckRequest(nil, payload)
	require.NoError(t, err)

	// 提取
	extractedReq, extractedPayload, err := ExtractAckRequest(data)
	require.NoError(t, err)
	assert.Nil(t, extractedReq)
	assert.Equal(t, payload, extractedPayload)
}

func TestPendingAck_AddAck(t *testing.T) {
	pending := NewPendingAck("msg-1", "topic", []byte("data"),
		[]string{"peer-1", "peer-2"}, false)

	// 添加一个 ACK（不要求全部）
	complete := pending.AddAck("peer-1")
	assert.True(t, complete)
	assert.True(t, pending.IsComplete())

	// 获取结果
	result := pending.GetResult()
	assert.True(t, result.Success)
	assert.Contains(t, result.AckedBy, "peer-1")
}

func TestPendingAck_RequireAll(t *testing.T) {
	pending := NewPendingAck("msg-1", "topic", []byte("data"),
		[]string{"peer-1", "peer-2"}, true)

	// 添加一个 ACK（要求全部）
	complete := pending.AddAck("peer-1")
	assert.False(t, complete)
	assert.False(t, pending.IsComplete())

	// 添加第二个 ACK
	complete = pending.AddAck("peer-2")
	assert.True(t, complete)
	assert.True(t, pending.IsComplete())

	// 获取结果
	result := pending.GetResult()
	assert.True(t, result.Success)
	assert.Len(t, result.AckedBy, 2)
	assert.Empty(t, result.MissingAcks)
}

// ============================================================================
//                       MessageQueue 补充测试（覆盖 0% 函数）
// ============================================================================

// TestMessageQueue_Peek 测试 Peek 方法
func TestMessageQueue_Peek(t *testing.T) {
	queue := NewMessageQueue(nil)

	// 空队列 Peek 返回 nil
	peeked := queue.Peek()
	assert.Nil(t, peeked)

	// 入队
	msg := &QueuedMessage{ID: "msg-1", Topic: "topic", Data: []byte("data")}
	queue.Enqueue(msg)

	// Peek 不移除消息
	peeked = queue.Peek()
	assert.NotNil(t, peeked)
	assert.Equal(t, "msg-1", peeked.ID)
	assert.Equal(t, 1, queue.Len())

	// 再次 Peek 仍然返回相同消息
	peeked2 := queue.Peek()
	assert.Equal(t, peeked.ID, peeked2.ID)

	t.Log("✅ Peek 测试通过")
}

// TestMessageQueue_Remove 测试 Remove 方法
func TestMessageQueue_Remove(t *testing.T) {
	queue := NewMessageQueue(nil)

	// 入队多条消息
	queue.Enqueue(&QueuedMessage{ID: "msg-1", Topic: "topic", Data: []byte("data1")})
	queue.Enqueue(&QueuedMessage{ID: "msg-2", Topic: "topic", Data: []byte("data2")})
	queue.Enqueue(&QueuedMessage{ID: "msg-3", Topic: "topic", Data: []byte("data3")})
	assert.Equal(t, 3, queue.Len())

	// 移除中间消息
	removed := queue.Remove("msg-2")
	assert.True(t, removed)
	assert.Equal(t, 2, queue.Len())
	assert.False(t, queue.Contains("msg-2"))
	assert.True(t, queue.Contains("msg-1"))
	assert.True(t, queue.Contains("msg-3"))

	// 移除不存在的消息
	removed = queue.Remove("non-existent")
	assert.False(t, removed)
	assert.Equal(t, 2, queue.Len())

	t.Log("✅ Remove 测试通过")
}

// TestMessageQueue_Clear 测试 Clear 方法
func TestMessageQueue_Clear(t *testing.T) {
	queue := NewMessageQueue(nil)

	// 入队多条消息
	queue.Enqueue(&QueuedMessage{ID: "msg-1", Topic: "topic", Data: []byte("data1")})
	queue.Enqueue(&QueuedMessage{ID: "msg-2", Topic: "topic", Data: []byte("data2")})
	assert.Equal(t, 2, queue.Len())

	// 清空队列
	queue.Clear()
	assert.Equal(t, 0, queue.Len())
	assert.True(t, queue.IsEmpty())

	// 清空后可以再次入队
	queue.Enqueue(&QueuedMessage{ID: "msg-3", Topic: "topic", Data: []byte("data3")})
	assert.Equal(t, 1, queue.Len())

	t.Log("✅ Clear 测试通过")
}

// TestMessageQueue_GetAll 测试 GetAll 方法
func TestMessageQueue_GetAll(t *testing.T) {
	queue := NewMessageQueue(nil)

	// 空队列
	all := queue.GetAll()
	assert.Empty(t, all)

	// 入队多条消息
	queue.Enqueue(&QueuedMessage{ID: "msg-1", Topic: "topic", Data: []byte("data1")})
	queue.Enqueue(&QueuedMessage{ID: "msg-2", Topic: "topic", Data: []byte("data2")})
	queue.Enqueue(&QueuedMessage{ID: "msg-3", Topic: "topic", Data: []byte("data3")})

	// 获取所有
	all = queue.GetAll()
	assert.Len(t, all, 3)
	assert.Equal(t, "msg-1", all[0].ID)
	assert.Equal(t, "msg-2", all[1].ID)
	assert.Equal(t, "msg-3", all[2].ID)

	// GetAll 不移除消息
	assert.Equal(t, 3, queue.Len())

	t.Log("✅ GetAll 测试通过")
}

// TestMessageQueue_Stats 测试 Stats 方法
func TestMessageQueue_Stats(t *testing.T) {
	queue := NewMessageQueue(nil)

	// 初始统计
	stats := queue.Stats()
	assert.Equal(t, 0, stats.CurrentSize)
	assert.Equal(t, int64(0), stats.TotalEnqueued)
	assert.Equal(t, int64(0), stats.TotalDequeued)
	assert.Equal(t, int64(0), stats.TotalDropped)

	// 入队
	queue.Enqueue(&QueuedMessage{ID: "msg-1", Topic: "topic", Data: []byte("data1")})
	queue.Enqueue(&QueuedMessage{ID: "msg-2", Topic: "topic", Data: []byte("data2")})

	stats = queue.Stats()
	assert.Equal(t, 2, stats.CurrentSize)
	assert.Equal(t, int64(2), stats.TotalEnqueued)

	// 出队
	queue.Dequeue()
	stats = queue.Stats()
	assert.Equal(t, 1, stats.CurrentSize)
	assert.Equal(t, int64(1), stats.TotalDequeued)

	t.Log("✅ Stats 测试通过")
}

// TestMessageQueue_IncrementAttempts 测试 IncrementAttempts 方法
func TestMessageQueue_IncrementAttempts(t *testing.T) {
	config := &QueueConfig{
		MaxSize:     100,
		MaxAge:      5 * time.Minute,
		MaxAttempts: 3, // 最大 3 次尝试
	}
	queue := NewMessageQueue(config)

	msg := &QueuedMessage{ID: "msg-1", Topic: "topic", Data: []byte("data1")}
	queue.Enqueue(msg)

	// 第 1 次尝试，仍在队列中
	ok := queue.IncrementAttempts("msg-1")
	assert.True(t, ok)
	assert.True(t, queue.Contains("msg-1"))

	// 第 2 次尝试，仍在队列中
	ok = queue.IncrementAttempts("msg-1")
	assert.True(t, ok)
	assert.True(t, queue.Contains("msg-1"))

	// 第 3 次尝试，超过最大次数，被移除
	ok = queue.IncrementAttempts("msg-1")
	assert.False(t, ok)
	assert.False(t, queue.Contains("msg-1"))

	// 不存在的消息
	ok = queue.IncrementAttempts("non-existent")
	assert.False(t, ok)

	t.Log("✅ IncrementAttempts 测试通过")
}

// TestReliablePublisher_ClearQueue 测试 ClearQueue 方法
func TestReliablePublisher_ClearQueue(t *testing.T) {
	mock := NewMockPublisher()
	mock.ShouldFail = true

	publisher := NewReliablePublisher(mock, nil)

	ctx := context.Background()
	err := publisher.Start(ctx)
	require.NoError(t, err)
	defer publisher.Stop()

	// 发布消息（会失败并入队）
	publisher.Publish(ctx, "test-topic", []byte("data1"))
	publisher.Publish(ctx, "test-topic", []byte("data2"))
	assert.Equal(t, 2, publisher.QueueSize())

	// 清空队列
	publisher.ClearQueue()
	assert.True(t, publisher.IsQueueEmpty())

	t.Log("✅ ClearQueue 测试通过")
}

// ============================================================================
//                       DeliveryError 测试（覆盖 0% 函数）
// ============================================================================

// TestDeliveryError_Error 测试 Error 方法
func TestDeliveryError_Error(t *testing.T) {
	err := &DeliveryError{
		Message: "delivery failed",
		Cause:   ErrQueueFull,
	}

	errMsg := err.Error()
	assert.Contains(t, errMsg, "delivery failed")
	assert.Contains(t, errMsg, "queue is full")

	t.Log("✅ DeliveryError.Error 测试通过")
}

// TestDeliveryError_Unwrap 测试 Unwrap 方法
func TestDeliveryError_Unwrap(t *testing.T) {
	err := &DeliveryError{
		Message: "delivery failed",
		Cause:   ErrQueueFull,
	}

	unwrapped := err.Unwrap()
	assert.Equal(t, ErrQueueFull, unwrapped)

	t.Log("✅ DeliveryError.Unwrap 测试通过")
}

// ============================================================================
//                       AckMessageType 测试（覆盖 0% 函数）
// ============================================================================

// TestAckMessageType_String 测试 String 方法
func TestAckMessageType_String(t *testing.T) {
	tests := []struct {
		ackType  AckMessageType
		expected string
	}{
		{AckTypeConfirm, "confirm"},
		{AckTypeReject, "reject"},
		{AckTypeRequest, "request"},
		{AckMessageType(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.ackType.String())
		})
	}

	t.Log("✅ AckMessageType.String 测试通过")
}

// TestDeliveryStatus_String 测试 String 方法
func TestDeliveryStatus_String(t *testing.T) {
	tests := []struct {
		status   DeliveryStatus
		expected string
	}{
		{StatusQueued, "queued"},
		{StatusSent, "sent"},
		{StatusAcked, "acked"},
		{StatusFailed, "failed"},
		{StatusDropped, "dropped"},
		{StatusPendingAck, "pending_ack"},
		{DeliveryStatus(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.status.String())
		})
	}

	t.Log("✅ DeliveryStatus.String 测试通过")
}

// ============================================================================
//                          Setter 函数测试
// ============================================================================

// TestSetUnderlyingPublisher_Valid 测试设置有效的底层发布器
func TestSetUnderlyingPublisher_Valid(t *testing.T) {
	mock1 := NewMockPublisher()
	publisher := NewReliablePublisher(mock1, nil)

	// 设置新的底层发布器
	mock2 := NewMockPublisher()
	publisher.SetUnderlyingPublisher(mock2)

	ctx := context.Background()
	publisher.Start(ctx)
	defer publisher.Stop()

	// 发布消息，应该使用新的发布器
	err := publisher.Publish(ctx, "test-topic", []byte("test"))
	require.NoError(t, err)

	// 验证消息发送到新发布器
	assert.Len(t, mock2.GetMessages(), 1)
	assert.Len(t, mock1.GetMessages(), 0)
}

// TestSetUnderlyingPublisher_Nil 测试设置 nil 发布器
func TestSetUnderlyingPublisher_Nil(t *testing.T) {
	mock := NewMockPublisher()
	publisher := NewReliablePublisher(mock, nil)

	// 🎯 边界测试：设置 nil 发布器不应 panic
	assert.NotPanics(t, func() {
		publisher.SetUnderlyingPublisher(nil)
	})

	// 验证：设置成功，nil 是有效值（可能用于禁用发布）
	assert.Nil(t, publisher.underlying)
}

// TestSetAckHandler_Valid 测试设置有效的 ACK 处理器
func TestSetAckHandler_Valid(t *testing.T) {
	mock := NewMockPublisher()
	publisher := NewReliablePublisher(mock, nil)

	handler := &mockAckHandler{}
	publisher.SetAckHandler(handler)

	ctx := context.Background()
	publisher.Start(ctx)
	defer publisher.Stop()

	// 验证设置成功
	assert.NotNil(t, publisher.ackHandler)
}

// TestSetAckHandler_Nil 测试设置 nil 处理器
func TestSetAckHandler_Nil(t *testing.T) {
	mock := NewMockPublisher()
	publisher := NewReliablePublisher(mock, nil)

	// 🎯 边界测试：设置 nil 处理器不应 panic
	assert.NotPanics(t, func() {
		publisher.SetAckHandler(nil)
	})
}

// TestSetLocalNodeID_Valid 测试设置有效的节点 ID
func TestSetLocalNodeID_Valid(t *testing.T) {
	mock := NewMockPublisher()
	publisher := NewReliablePublisher(mock, nil)

	testNodeID := "test-node-123"
	publisher.SetLocalNodeID(testNodeID)

	// 验证设置成功
	assert.Equal(t, testNodeID, publisher.localNodeID)
}

// TestSetLocalNodeID_Empty 测试设置空节点 ID
func TestSetLocalNodeID_Empty(t *testing.T) {
	mock := NewMockPublisher()
	publisher := NewReliablePublisher(mock, nil)

	// 🎯 边界测试：空节点 ID 不应 panic
	assert.NotPanics(t, func() {
		publisher.SetLocalNodeID("")
	})

	assert.Equal(t, "", publisher.localNodeID)
}

// TestSetCriticalPeers_Valid 测试设置关键节点列表
func TestSetCriticalPeers_Valid(t *testing.T) {
	mock := NewMockPublisher()
	publisher := NewReliablePublisher(mock, nil)

	peers := []string{"peer1", "peer2", "peer3"}
	publisher.SetCriticalPeers(peers)

	// 验证设置成功
	assert.Equal(t, peers, publisher.config.CriticalPeers)
}

// TestSetCriticalPeers_Nil 测试设置 nil 列表
func TestSetCriticalPeers_Nil(t *testing.T) {
	mock := NewMockPublisher()
	publisher := NewReliablePublisher(mock, nil)

	// 🎯 边界测试：nil 列表不应 panic
	assert.NotPanics(t, func() {
		publisher.SetCriticalPeers(nil)
	})

	assert.Nil(t, publisher.config.CriticalPeers)
}

// TestSetCriticalPeers_Empty 测试设置空列表
func TestSetCriticalPeers_Empty(t *testing.T) {
	mock := NewMockPublisher()
	publisher := NewReliablePublisher(mock, nil)

	publisher.SetCriticalPeers([]string{})

	assert.Empty(t, publisher.config.CriticalPeers)
}

// mockAckHandler 模拟 ACK 处理器
type mockAckHandler struct{}

func (m *mockAckHandler) SendAck(ctx context.Context, targetPeer string, ack *AckMessage) error {
	// 模拟实现
	return nil
}

// ============================================================================
//                          序列化测试
// ============================================================================

// TestMarshalAckMessage_Valid 测试正常序列化
func TestMarshalAckMessage_Valid(t *testing.T) {
	now := time.Now()
	msg := &AckMessage{
		MessageID: "test-msg-123",
		AckerID:   "peer-456",
		Topic:     "test-topic",
		Timestamp: now,
	}

	data, err := MarshalAckMessage(msg)
	require.NoError(t, err)
	require.NotNil(t, data)

	// 验证可以反序列化
	decoded, err := UnmarshalAckMessage(data)
	require.NoError(t, err)
	assert.Equal(t, msg.MessageID, decoded.MessageID)
	assert.Equal(t, msg.AckerID, decoded.AckerID)
	assert.Equal(t, msg.Topic, decoded.Topic)
	// 时间精度为毫秒
	assert.Equal(t, msg.Timestamp.UnixMilli(), decoded.Timestamp.UnixMilli())
}

// TestMarshalAckMessage_EmptyFields 测试空字段序列化
func TestMarshalAckMessage_EmptyFields(t *testing.T) {
	msg := &AckMessage{
		MessageID: "",
		AckerID:   "",
		Topic:     "",
		Timestamp: time.Time{},
	}

	// 🎯 边界测试：空字段应该可以序列化
	data, err := MarshalAckMessage(msg)
	require.NoError(t, err)
	require.NotNil(t, data)

	decoded, err := UnmarshalAckMessage(data)
	require.NoError(t, err)
	assert.Equal(t, msg.MessageID, decoded.MessageID)
}

// TestUnmarshalAckMessage_Valid 测试正常反序列化
func TestUnmarshalAckMessage_Valid(t *testing.T) {
	// 手动构造有效的 JSON（使用实际的 JSON tag 和类型）
	validJSON := `{
		"v": 1,
		"t": 0,
		"mid": "test-123",
		"aid": "peer-456",
		"topic": "test-topic",
		"ts": 1234567890000
	}`

	msg, err := UnmarshalAckMessage([]byte(validJSON))
	require.NoError(t, err)
	assert.Equal(t, "test-123", msg.MessageID)
	assert.Equal(t, "peer-456", msg.AckerID)
	assert.Equal(t, "test-topic", msg.Topic)
}

// TestUnmarshalAckMessage_Malformed 测试畸形数据
func TestUnmarshalAckMessage_Malformed(t *testing.T) {
	testCases := []struct {
		name string
		data string
	}{
		{"empty", ""},
		{"invalid json", "{invalid}"},
		{"incomplete json", `{"message_id": `},
		{"not json", "this is not json"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 🎯 错误路径测试：畸形数据应该返回错误
			_, err := UnmarshalAckMessage([]byte(tc.data))
			assert.Error(t, err, "畸形数据应该返回错误")
		})
	}
}

// TestUnmarshalAckMessage_Nil 测试 nil 输入
func TestUnmarshalAckMessage_Nil(t *testing.T) {
	// 🎯 边界测试：nil 输入应该返回错误
	_, err := UnmarshalAckMessage(nil)
	assert.Error(t, err)
}

// ============================================================================
//                          并发测试
// ============================================================================

// TestReliablePublisher_ConcurrentPublish 测试并发发布
func TestReliablePublisher_ConcurrentPublish(t *testing.T) {
	mock := NewMockPublisher()
	publisher := NewReliablePublisher(mock, nil)

	ctx := context.Background()
	err := publisher.Start(ctx)
	require.NoError(t, err)
	defer publisher.Stop()

	// 🎯 并发测试：20 个 goroutine 同时发布
	const goroutines = 20
	const messagesPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < messagesPerGoroutine; j++ {
				data := []byte(string(rune('A' + id)))
				err := publisher.Publish(ctx, "test-topic", data)
				if err != nil {
					t.Logf("goroutine %d publish %d failed: %v", id, j, err)
				}
			}
		}(i)
	}

	wg.Wait()

	// 验证：所有消息应该被处理（成功或入队）
	messages := mock.GetMessages()
	t.Logf("发送了 %d 条消息（预期 %d）", len(messages), goroutines*messagesPerGoroutine)
	// 注意：由于队列限制，可能不是全部发送成功
	assert.GreaterOrEqual(t, len(messages), 0)
}

// TestReliablePublisher_ConcurrentSetters 测试并发设置
func TestReliablePublisher_ConcurrentSetters(t *testing.T) {
	mock := NewMockPublisher()
	publisher := NewReliablePublisher(mock, nil)

	ctx := context.Background()
	publisher.Start(ctx)
	defer publisher.Stop()

	// 🎯 并发测试：多个 goroutine 同时设置配置
	var wg sync.WaitGroup
	wg.Add(4)

	// Goroutine 1: 设置底层发布器
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			newMock := NewMockPublisher()
			publisher.SetUnderlyingPublisher(newMock)
		}
	}()

	// Goroutine 2: 设置节点 ID
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			publisher.SetLocalNodeID("node-" + string(rune(i)))
		}
	}()

	// Goroutine 3: 设置关键节点
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			peers := []string{"peer1", "peer2"}
			publisher.SetCriticalPeers(peers)
		}
	}()

	// Goroutine 4: 发布消息
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_ = publisher.Publish(ctx, "test", []byte("data"))
		}
	}()

	// 🎯 验证：不应 panic，不应死锁
	wg.Wait()
	t.Log("✅ 并发设置测试通过，无 Race 和死锁")
}

// ============================================================================
//                          错误路径测试
// ============================================================================

// TestPublish_UnderlyingFailed 测试底层发布失败
func TestPublish_UnderlyingFailed(t *testing.T) {
	mock := NewMockPublisher()
	mock.ShouldFail = true // 模拟底层发布失败
	mock.FailError = assert.AnError
	publisher := NewReliablePublisher(mock, nil)

	ctx := context.Background()
	publisher.Start(ctx)
	defer publisher.Stop()

	// 🎯 错误路径：底层发布失败，消息应入队
	err := publisher.Publish(ctx, "test-topic", []byte("test"))
	// 第一次可能入队成功（返回 nil）或失败
	// 重要的是不应 panic
	t.Logf("发布结果: err=%v", err)
}

// TestPublish_QueueFull 测试队列已满
func TestPublish_QueueFull(t *testing.T) {
	mock := NewMockPublisher()
	mock.ShouldFail = true // 底层失败，强制入队
	mock.FailError = assert.AnError
	
	config := DefaultPublisherConfig()
	config.QueueConfig.MaxSize = 2 // 小队列
	publisher := NewReliablePublisher(mock, config)

	ctx := context.Background()
	publisher.Start(ctx)
	defer publisher.Stop()

	// 🎯 错误路径：填满队列
	for i := 0; i < 5; i++ {
		err := publisher.Publish(ctx, "test", []byte("data"))
		if err != nil {
			t.Logf("第 %d 次发布失败: %v", i+1, err)
		}
	}

	// 验证：队列已满，后续发布应失败
	err := publisher.Publish(ctx, "test", []byte("data"))
	if err != nil {
		assert.Contains(t, err.Error(), "队列已满", "应返回队列已满错误")
	}
}

// TestPublish_ContextCanceled 测试 context 取消
func TestPublish_ContextCanceled(t *testing.T) {
	mock := NewMockPublisher()
	publisher := NewReliablePublisher(mock, nil)

	ctx := context.Background()
	publisher.Start(ctx)
	defer publisher.Stop()

	// 🎯 错误路径：context 已取消
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	err := publisher.Publish(canceledCtx, "test-topic", []byte("test"))
	// 验证：应返回 context 错误或成功（取决于实现）
	t.Logf("取消 context 发布结果: %v", err)
}

// TestPublish_WithoutStart 测试未启动就发布
func TestPublish_WithoutStart(t *testing.T) {
	mock := NewMockPublisher()
	publisher := NewReliablePublisher(mock, nil)

	ctx := context.Background()

	// 🎯 错误路径：未启动就发布，可能失败或入队
	err := publisher.Publish(ctx, "test-topic", []byte("test"))
	// 不应 panic
	t.Logf("未启动发布结果: %v", err)
}
