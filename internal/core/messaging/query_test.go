package messaging

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Query 消息编解码测试
// ============================================================================

func TestEncodeDecodeQueryMessage(t *testing.T) {
	// 构造测试消息
	queryID := "test-query-123"
	replyTo := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	data := []byte("test query data")

	msg := &types.Message{
		IsQuery: true,
		QueryID: queryID,
		ReplyTo: replyTo,
		Data:    data,
	}

	// 编码
	encoded := encodeQueryMessage(msg)

	// 解码
	decodedQueryID, decodedReplyTo, decodedData, isQuery := decodeQueryMessage(encoded)

	// 验证
	if !isQuery {
		t.Error("应该识别为查询消息")
	}

	if decodedQueryID != queryID {
		t.Errorf("QueryID 不匹配: got %s, want %s", decodedQueryID, queryID)
	}

	if decodedReplyTo != replyTo {
		t.Errorf("ReplyTo 不匹配: got %v, want %v", decodedReplyTo, replyTo)
	}

	if string(decodedData) != string(data) {
		t.Errorf("Data 不匹配: got %s, want %s", string(decodedData), string(data))
	}
}

func TestDecodeNonQueryMessage(t *testing.T) {
	// 普通消息
	normalData := []byte("normal message data")

	queryID, replyTo, data, isQuery := decodeQueryMessage(normalData)

	if isQuery {
		t.Error("普通消息不应该被识别为查询消息")
	}

	if queryID != "" {
		t.Errorf("普通消息的 QueryID 应为空")
	}

	if replyTo != types.EmptyNodeID {
		t.Errorf("普通消息的 ReplyTo 应为空")
	}

	if string(data) != string(normalData) {
		t.Errorf("Data 不匹配: got %s, want %s", string(data), string(normalData))
	}
}

func TestGenerateQueryID(t *testing.T) {
	id1, err1 := generateQueryID()
	require.NoError(t, err1)
	id2, err2 := generateQueryID()
	require.NoError(t, err2)

	if id1 == id2 {
		t.Error("生成的 QueryID 不应该相同")
	}

	if len(id1) != 32 { // 16 bytes hex encoded
		t.Errorf("QueryID 长度不正确: got %d, want 32", len(id1))
	}
}

func TestGenerateMessageID(t *testing.T) {
	id1, err1 := generateMessageID()
	require.NoError(t, err1)
	id2, err2 := generateMessageID()
	require.NoError(t, err2)

	if string(id1) == string(id2) {
		t.Error("生成的 MessageID 不应该相同")
	}

	if len(id1) != 20 {
		t.Errorf("MessageID 长度不正确: got %d, want 20", len(id1))
	}
}

// ============================================================================
//                              QueryResponseCollector 测试
// ============================================================================

func TestQueryResponseCollector(t *testing.T) {
	collector := newQueryResponseCollector(3)

	// 添加响应
	resp1 := types.QueryResponse{
		From: types.NodeID{1},
		Data: []byte("response 1"),
	}
	resp2 := types.QueryResponse{
		From: types.NodeID{2},
		Data: []byte("response 2"),
	}

	if !collector.addResponse(resp1) {
		t.Error("添加第一个响应应该成功")
	}

	if !collector.addResponse(resp2) {
		t.Error("添加第二个响应应该成功")
	}
}

func TestQueryResponseCollectorFull(t *testing.T) {
	collector := newQueryResponseCollector(1)

	// 添加第一个响应（成功）
	resp1 := types.QueryResponse{
		From: types.NodeID{1},
		Data: []byte("response 1"),
	}
	if !collector.addResponse(resp1) {
		t.Error("添加第一个响应应该成功")
	}

	// 添加第二个响应（失败，已满）
	resp2 := types.QueryResponse{
		From: types.NodeID{2},
		Data: []byte("response 2"),
	}
	if collector.addResponse(resp2) {
		t.Error("添加到已满的收集器应该失败")
	}
}

// ============================================================================
//                              Message 类型测试
// ============================================================================

func TestMessageIsQueryMessage(t *testing.T) {
	// 查询消息
	queryMsg := types.Message{
		IsQuery: true,
		QueryID: "test-query",
	}
	if !queryMsg.IsQueryMessage() {
		t.Error("应该是查询消息")
	}

	// 非查询消息
	normalMsg := types.Message{
		IsQuery: false,
	}
	if normalMsg.IsQueryMessage() {
		t.Error("不应该是查询消息")
	}

	// IsQuery=true 但无 QueryID
	invalidMsg := types.Message{
		IsQuery: true,
		QueryID: "",
	}
	if invalidMsg.IsQueryMessage() {
		t.Error("无 QueryID 的消息不应该是有效的查询消息")
	}
}

func TestMessageNeedsReply(t *testing.T) {
	replyTo := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8}

	// 需要回复
	msg1 := types.Message{
		IsQuery: true,
		ReplyTo: replyTo,
	}
	if !msg1.NeedsReply() {
		t.Error("应该需要回复")
	}

	// 不需要回复（非查询）
	msg2 := types.Message{
		IsQuery: false,
		ReplyTo: replyTo,
	}
	if msg2.NeedsReply() {
		t.Error("非查询消息不需要回复")
	}

	// 不需要回复（无 ReplyTo）
	msg3 := types.Message{
		IsQuery: true,
		ReplyTo: types.EmptyNodeID,
	}
	if msg3.NeedsReply() {
		t.Error("无 ReplyTo 的消息不需要回复")
	}
}

