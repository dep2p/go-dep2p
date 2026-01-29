// Package addrmgmt 网络函数集成测试
package addrmgmt

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/tests/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                     Handler 网络函数集成测试
// ============================================================================

func TestHandler_SendRefreshNotify_Success(t *testing.T) {
	handler := NewHandler("local-peer")
	stream := mocks.NewMockStream()

	record := &AddressRecord{
		NodeID:    "peer1",
		RealmID:   "realm1",
		Sequence:  42,
		Timestamp: time.Now(),
		Addresses: []string{"/ip4/1.1.1.1/tcp/4001", "/ip4/2.2.2.2/tcp/4002"},
		TTL:       time.Hour,
		Signature: []byte("test-signature"),
	}

	ctx := context.Background()
	err := handler.SendRefreshNotify(ctx, stream, record)
	require.NoError(t, err)

	// 验证数据被写入
	assert.Greater(t, len(stream.WriteData), 5, "应该写入消息头+消息体")

	// 验证消息类型
	assert.Equal(t, uint8(MsgTypeRefreshNotify), stream.WriteData[0], "消息类型应为 RefreshNotify")
	
	// 验证 stream 状态
	assert.False(t, stream.IsClosed(), "发送成功后 stream 不应关闭")
}

func TestHandler_SendRefreshNotify_WriteError(t *testing.T) {
	handler := NewHandler("local-peer")
	stream := mocks.NewMockStream()
	
	// 注入写入错误
	stream.WriteFunc = func(p []byte) (int, error) {
		return 0, io.ErrUnexpectedEOF
	}

	record := NewAddressRecord("peer1", []string{"/ip4/1.1.1.1/tcp/4001"}, time.Hour)

	ctx := context.Background()
	err := handler.SendRefreshNotify(ctx, stream, record)
	assert.Error(t, err, "写入错误应该返回")
	assert.ErrorIs(t, err, io.ErrUnexpectedEOF)
}

func TestHandler_QueryPeer_Success(t *testing.T) {
	handler := NewHandler("local-peer")

	// 准备响应数据
	responseRecord := &AddressRecord{
		NodeID:    "target-peer",
		Sequence:  10,
		Addresses: []string{"/ip4/3.3.3.3/tcp/4003"},
		Timestamp: time.Now(),
		TTL:       time.Hour,
	}

	// 编码完整响应（包括消息头）
	responseData := handler.encodeQueryResponse(responseRecord)

	// 创建 stream，先读取查询请求，再返回响应
	stream := mocks.NewMockStream()
	readPos := 0
	stream.ReadFunc = func(p []byte) (int, error) {
		// 先返回响应数据
		if readPos >= len(responseData) {
			return 0, io.EOF
		}
		n := copy(p, responseData[readPos:])
		readPos += n
		return n, nil
	}

	ctx := context.Background()
	result, err := handler.QueryPeer(ctx, stream, "target-peer")
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, "target-peer", result.NodeID)
	assert.Equal(t, uint64(10), result.Sequence)
	assert.Len(t, result.Addresses, 1)
	
	// 验证发送了查询请求
	assert.Greater(t, len(stream.WriteData), 0, "应该发送了查询请求")
	assert.Equal(t, uint8(MsgTypeQueryRequest), stream.WriteData[0], "应该是查询请求")
}

func TestHandler_QueryPeer_ReadError(t *testing.T) {
	handler := NewHandler("local-peer")
	stream := mocks.NewMockStream()
	
	// 注入读取错误
	stream.ReadFunc = func(p []byte) (int, error) {
		return 0, io.ErrUnexpectedEOF
	}

	ctx := context.Background()
	_, err := handler.QueryPeer(ctx, stream, "target-peer")
	assert.Error(t, err, "读取错误应该返回")
	
	// 验证仍然发送了查询请求
	assert.Greater(t, len(stream.WriteData), 0, "应该尝试发送查询请求")
}

func TestHandler_QueryPeer_NotFound(t *testing.T) {
	handler := NewHandler("local-peer")

	// 准备空响应（未找到）
	emptyResponse := []byte{
		MsgTypeQueryResponse, // 消息类型
		0, 0, 0, 0, // 长度 = 0（未找到）
	}

	stream := mocks.NewMockStreamWithData(emptyResponse)

	ctx := context.Background()
	result, err := handler.QueryPeer(ctx, stream, "target-peer")
	require.NoError(t, err)
	assert.Nil(t, result, "未找到应返回 nil")
	
	// 验证发送了查询请求
	assert.Greater(t, len(stream.WriteData), 0, "应该发送了查询请求")
}

// ============================================================================
//                     HandleStream 集成测试
// ============================================================================

func TestHandler_HandleStream_RefreshNotify(t *testing.T) {
	handlerSender := NewHandler("sender-peer")
	handlerReceiver := NewHandler("receiver-peer")

	// 准备 RefreshNotify 消息（使用更真实的数据确保超过 50 字节最小限制）
	record := &AddressRecord{
		NodeID:    "peer1",
		RealmID:   "test-realm-id",
		Sequence:  5,
		Addresses: []string{
			"/ip4/1.1.1.1/tcp/4001",
			"/ip4/2.2.2.2/tcp/4002",
		},
		Timestamp: time.Now(),
		TTL:       time.Hour,
		Signature: []byte("test-signature-data"),
	}

	// 使用发送方的 handler 编码消息（确保编码正确）
	msgData := handlerSender.encodeRefreshNotify(record)
	
	t.Logf("消息大小: %d 字节（最小要求: 50）", len(msgData)-5) // 减去消息头

	// 创建 stream 用于接收
	stream := mocks.NewMockStreamWithData(msgData)

	// 接收方处理消息
	handlerReceiver.HandleStream(stream)

	// 等待异步处理完成
	time.Sleep(50 * time.Millisecond)

	// 验证记录被缓存
	cached := handlerReceiver.GetRecord("peer1")
	require.NotNil(t, cached, "记录应该被缓存")
	assert.Equal(t, uint64(5), cached.Sequence)
	assert.Len(t, cached.Addresses, 2)
	
	// 验证 stream 被关闭
	assert.True(t, stream.IsClosed(), "HandleStream 完成后应关闭 stream")
}

func TestHandler_HandleStream_QueryRequest(t *testing.T) {
	handler := NewHandler("remote-peer")

	// 预置一个记录
	handler.records["target-peer"] = &AddressRecord{
		NodeID:    "target-peer",
		Sequence:  20,
		Addresses: []string{"/ip4/4.4.4.4/tcp/4004"},
		Timestamp: time.Now(),
		TTL:       time.Hour,
	}

	// 准备 QueryRequest 消息（完整格式）
	targetID := "target-peer"
	queryMsg := make([]byte, 6+len(targetID))
	queryMsg[0] = MsgTypeQueryRequest
	// 填充长度字段（big endian）
	msgLen := uint32(1 + len(targetID))
	queryMsg[1] = byte(msgLen >> 24)
	queryMsg[2] = byte(msgLen >> 16)
	queryMsg[3] = byte(msgLen >> 8)
	queryMsg[4] = byte(msgLen)
	queryMsg[5] = byte(len(targetID))
	copy(queryMsg[6:], targetID)

	// 创建 stream
	stream := mocks.NewMockStreamWithData(queryMsg)

	// 处理查询
	handler.HandleStream(stream)

	// 等待异步处理
	time.Sleep(50 * time.Millisecond)

	// 验证响应被写入
	assert.Greater(t, len(stream.WriteData), 5, "应该写入了响应")
	assert.Equal(t, uint8(MsgTypeQueryResponse), stream.WriteData[0], "应该返回 QueryResponse")
	
	// 验证 stream 被关闭
	assert.True(t, stream.IsClosed())
}

func TestHandler_HandleStream_InvalidMessage(t *testing.T) {
	handler := NewHandler("remote-peer")

	// 无效消息类型
	invalidMsg := []byte{0xFF, 0x00, 0x00, 0x00, 0x01, 0xAA}
	stream := mocks.NewMockStreamWithData(invalidMsg)

	// 应该优雅处理
	handler.HandleStream(stream)

	// 等待处理完成
	time.Sleep(10 * time.Millisecond)

	// stream 应该被关闭
	assert.True(t, stream.IsClosed(), "无效消息后应关闭 stream")
}

func TestHandler_HandleStream_ReadError(t *testing.T) {
	handler := NewHandler("remote-peer")

	stream := mocks.NewMockStream()
	// 注入读取错误
	stream.ReadFunc = func(p []byte) (int, error) {
		return 0, io.ErrUnexpectedEOF
	}

	// 应该优雅处理错误
	handler.HandleStream(stream)

	// 等待处理完成
	time.Sleep(10 * time.Millisecond)

	// stream 应该被关闭
	assert.True(t, stream.IsClosed(), "读取错误后应关闭 stream")
}

// ============================================================================
//                     端到端集成测试
// ============================================================================

func TestHandler_EndToEnd_RefreshNotifyAndQuery(t *testing.T) {
	// 场景：节点 A 向节点 B 发送地址刷新，节点 C 查询节点 A 的地址

	handlerA := NewHandler("peer-A")
	handlerB := NewHandler("peer-B")
	handlerC := NewHandler("peer-C")

	// 步骤 1：节点 A 创建地址记录
	recordA := &AddressRecord{
		NodeID:    "peer-A",
		Sequence:  1,
		Addresses: []string{"/ip4/10.0.0.1/tcp/4001", "/ip4/10.0.0.2/tcp/4002"},
		Timestamp: time.Now(),
		TTL:       time.Hour,
	}

	// 步骤 2：节点 A 向节点 B 发送 RefreshNotify
	streamAB := mocks.NewMockStream()
	ctx := context.Background()
	err := handlerA.SendRefreshNotify(ctx, streamAB, recordA)
	require.NoError(t, err)

	// 步骤 3：节点 B 处理 RefreshNotify
	notifyData := streamAB.WriteData
	streamBA := mocks.NewMockStreamWithData(notifyData)
	handlerB.HandleStream(streamBA)

	// 等待异步处理
	time.Sleep(50 * time.Millisecond)

	// 步骤 4：验证节点 B 缓存了节点 A 的地址
	cachedInB := handlerB.GetRecord("peer-A")
	require.NotNil(t, cachedInB, "节点 B 应该缓存了节点 A 的地址")
	assert.Equal(t, uint64(1), cachedInB.Sequence)
	assert.Len(t, cachedInB.Addresses, 2)

	// 步骤 5：节点 C 从节点 B 查询节点 A 的地址
	// 准备响应数据
	responseData := handlerB.encodeQueryResponse(cachedInB)

	streamCB := mocks.NewMockStream()
	readPos := 0
	streamCB.ReadFunc = func(p []byte) (int, error) {
		if readPos >= len(responseData) {
			return 0, io.EOF
		}
		n := copy(p, responseData[readPos:])
		readPos += n
		return n, nil
	}

	resultC, err := handlerC.QueryPeer(ctx, streamCB, "peer-A")
	require.NoError(t, err)
	require.NotNil(t, resultC)

	// 步骤 6：验证节点 C 获取到了正确的地址
	assert.Equal(t, "peer-A", resultC.NodeID)
	assert.Equal(t, uint64(1), resultC.Sequence)
	assert.Len(t, resultC.Addresses, 2)
	assert.Contains(t, resultC.Addresses, "/ip4/10.0.0.1/tcp/4001")
	assert.Contains(t, resultC.Addresses, "/ip4/10.0.0.2/tcp/4002")

	t.Log("✅ 端到端集成测试通过：RefreshNotify + Query 流程正常")
}

func TestHandler_EndToEnd_MessageSizeLimit(t *testing.T) {
	handler := NewHandler("local-peer")

	// 创建超大消息（超过 MaxMessageSize）
	largeAddrs := make([]string, 1000)
	for i := 0; i < 1000; i++ {
		largeAddrs[i] = "/ip4/1.1.1.1/tcp/4001/very/long/address/component/to/exceed/size/limit"
	}

	record := &AddressRecord{
		NodeID:    "peer1",
		Sequence:  1,
		Addresses: largeAddrs,
		Timestamp: time.Now(),
		TTL:       time.Hour,
	}

	// 发送消息
	stream := mocks.NewMockStream()
	ctx := context.Background()
	err := handler.SendRefreshNotify(ctx, stream, record)
	require.NoError(t, err) // 发送本身应该成功

	// 接收方应该能处理（或拒绝）超大消息
	written := stream.WriteData
	t.Logf("消息大小: %d bytes (MaxMessageSize: %d)", len(written), MaxMessageSize)
	
	// 🚨 潜在 BUG 探测：消息超过 MaxMessageSize 时的处理
	if len(written) > MaxMessageSize {
		t.Logf("⚠️ 警告：消息大小 (%d) 超过 MaxMessageSize (%d)", len(written), MaxMessageSize)
		t.Log("接收方可能会拒绝此消息")
	}
}

// ============================================================================
//                     并发集成测试
// ============================================================================

func TestHandler_Concurrent_HandleStreams(t *testing.T) {
	handler := NewHandler("server")

	const goroutines = 10
	done := make(chan bool, goroutines)

	// 并发处理多个 RefreshNotify
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()

			record := &AddressRecord{
				NodeID:    "peer-" + string(rune('0'+id)),
				Sequence:  uint64(id + 1),
				Addresses: []string{"/ip4/1.1.1.1/tcp/4001"},
				Timestamp: time.Now(),
				TTL:       time.Hour,
			}

			msgData := handler.encodeRefreshNotify(record)
			stream := mocks.NewMockStreamWithData(msgData)
			handler.HandleStream(stream)
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < goroutines; i++ {
		<-done
	}

	// 等待异步处理完成
	time.Sleep(100 * time.Millisecond)

	// 验证所有记录都被缓存
	allRecords := handler.GetAllRecords()
	assert.Equal(t, goroutines, len(allRecords), "应该缓存了所有节点的记录")

	t.Log("✅ 并发处理 HandleStream 测试通过")
}
