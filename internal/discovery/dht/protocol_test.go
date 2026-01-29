package dht

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// MessageType.String() 测试
// ============================================================================

// TestMessageType_String 测试所有消息类型的字符串表示
func TestMessageType_String(t *testing.T) {
	tests := []struct {
		msgType  MessageType
		expected string
	}{
		{MessageTypeFindNode, "FIND_NODE"},
		{MessageTypeFindNodeResponse, "FIND_NODE_RESPONSE"},
		{MessageTypeFindValue, "FIND_VALUE"},
		{MessageTypeFindValueResponse, "FIND_VALUE_RESPONSE"},
		{MessageTypeStore, "STORE"},
		{MessageTypeStoreResponse, "STORE_RESPONSE"},
		{MessageTypePing, "PING"},
		{MessageTypePingResponse, "PING_RESPONSE"},
		{MessageTypeAddProvider, "ADD_PROVIDER"},
		{MessageTypeAddProviderResponse, "ADD_PROVIDER_RESPONSE"},
		{MessageTypeGetProviders, "GET_PROVIDERS"},
		{MessageTypeGetProvidersResponse, "GET_PROVIDERS_RESPONSE"},
		{MessageTypeRemoveProvider, "REMOVE_PROVIDER"},
		{MessageTypeRemoveProviderResponse, "REMOVE_PROVIDER_RESPONSE"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.msgType.String())
		})
	}

	t.Log("✅ 所有消息类型字符串表示正确")
}

// TestMessageType_String_Unknown 测试未知消息类型
func TestMessageType_String_Unknown(t *testing.T) {
	unknown := MessageType(255)
	assert.Equal(t, "UNKNOWN", unknown.String())

	t.Log("✅ 未知消息类型正确返回 UNKNOWN")
}

// ============================================================================
// 编解码测试 - 重点：发现数据丢失或损坏BUG
// ============================================================================

// TestEncodeDecode_FindNodeRequest 测试FindNode请求编解码
func TestEncodeDecode_FindNodeRequest(t *testing.T) {
	original := NewFindNodeRequest(
		12345,
		types.NodeID("sender-peer"),
		[]string{"/ip4/1.2.3.4/tcp/4001"},
		types.NodeID("target-peer"),
	)

	// 编码
	data, err := original.Encode()
	require.NoError(t, err)
	assert.NotEmpty(t, data)

	// 解码
	decoded, err := DecodeMessage(data)
	require.NoError(t, err)

	// 验证所有字段
	assert.Equal(t, MessageTypeFindNode, decoded.Type)
	assert.Equal(t, uint64(12345), decoded.RequestID)
	assert.Equal(t, types.NodeID("sender-peer"), decoded.Sender)
	assert.Equal(t, []string{"/ip4/1.2.3.4/tcp/4001"}, decoded.SenderAddrs)
	assert.Equal(t, types.NodeID("target-peer"), decoded.Target)

	t.Log("✅ FindNode请求编解码正确")
}

// TestEncodeDecode_FindNodeResponse 测试FindNode响应编解码
func TestEncodeDecode_FindNodeResponse(t *testing.T) {
	closerPeers := []PeerRecord{
		{ID: "peer-1", Addrs: []string{"/ip4/1.1.1.1/tcp/4001"}},
		{ID: "peer-2", Addrs: []string{"/ip4/2.2.2.2/tcp/4001"}},
	}

	original := NewFindNodeResponse(12345, types.NodeID("sender"), closerPeers)

	data, err := original.Encode()
	require.NoError(t, err)

	decoded, err := DecodeMessage(data)
	require.NoError(t, err)

	assert.Equal(t, MessageTypeFindNodeResponse, decoded.Type)
	assert.Equal(t, uint64(12345), decoded.RequestID)
	assert.True(t, decoded.Success)
	assert.Equal(t, 2, len(decoded.CloserPeers))
	assert.Equal(t, types.NodeID("peer-1"), decoded.CloserPeers[0].ID)

	t.Log("✅ FindNode响应编解码正确")
}

// TestEncodeDecode_FindValueRequest 测试FindValue请求编解码
func TestEncodeDecode_FindValueRequest(t *testing.T) {
	original := NewFindValueRequest(
		99999,
		types.NodeID("sender"),
		[]string{"/ip4/127.0.0.1/tcp/4001"},
		"my-key",
	)

	data, err := original.Encode()
	require.NoError(t, err)

	decoded, err := DecodeMessage(data)
	require.NoError(t, err)

	assert.Equal(t, MessageTypeFindValue, decoded.Type)
	assert.Equal(t, "my-key", decoded.Key)

	t.Log("✅ FindValue请求编解码正确")
}

// TestEncodeDecode_FindValueResponse_WithValue 测试FindValue响应（找到值）
func TestEncodeDecode_FindValueResponse_WithValue(t *testing.T) {
	value := []byte("this is the stored value")
	original := NewFindValueResponse(12345, types.NodeID("sender"), value)

	data, err := original.Encode()
	require.NoError(t, err)

	decoded, err := DecodeMessage(data)
	require.NoError(t, err)

	assert.Equal(t, MessageTypeFindValueResponse, decoded.Type)
	assert.Equal(t, value, decoded.Value)
	assert.True(t, decoded.Success)

	t.Log("✅ FindValue响应（有值）编解码正确")
}

// TestEncodeDecode_FindValueResponse_WithPeers 测试FindValue响应（返回更近节点）
func TestEncodeDecode_FindValueResponse_WithPeers(t *testing.T) {
	closerPeers := []PeerRecord{
		{ID: "closer-1", Addrs: []string{"/ip4/10.0.0.1/tcp/4001"}},
	}
	original := NewFindValueResponseWithPeers(12345, types.NodeID("sender"), closerPeers)

	data, err := original.Encode()
	require.NoError(t, err)

	decoded, err := DecodeMessage(data)
	require.NoError(t, err)

	assert.Equal(t, MessageTypeFindValueResponse, decoded.Type)
	assert.Empty(t, decoded.Value) // 没有值
	assert.Equal(t, 1, len(decoded.CloserPeers))

	t.Log("✅ FindValue响应（返回节点）编解码正确")
}

// TestEncodeDecode_StoreRequest 测试Store请求编解码
func TestEncodeDecode_StoreRequest(t *testing.T) {
	value := []byte("value to store")
	original := NewStoreRequest(
		11111,
		types.NodeID("sender"),
		[]string{"/ip4/1.2.3.4/tcp/4001"},
		"store-key",
		value,
		3600, // 1小时TTL
	)

	data, err := original.Encode()
	require.NoError(t, err)

	decoded, err := DecodeMessage(data)
	require.NoError(t, err)

	assert.Equal(t, MessageTypeStore, decoded.Type)
	assert.Equal(t, "store-key", decoded.Key)
	assert.Equal(t, value, decoded.Value)
	assert.Equal(t, uint32(3600), decoded.TTL)

	t.Log("✅ Store请求编解码正确")
}

// TestEncodeDecode_StoreResponse 测试Store响应编解码
func TestEncodeDecode_StoreResponse(t *testing.T) {
	// 成功响应
	successResp := NewStoreResponse(11111, types.NodeID("sender"), true, "")
	data, err := successResp.Encode()
	require.NoError(t, err)

	decoded, err := DecodeMessage(data)
	require.NoError(t, err)

	assert.Equal(t, MessageTypeStoreResponse, decoded.Type)
	assert.True(t, decoded.Success)
	assert.Empty(t, decoded.Error)

	// 失败响应
	failResp := NewStoreResponse(11111, types.NodeID("sender"), false, "storage full")
	data, err = failResp.Encode()
	require.NoError(t, err)

	decoded, err = DecodeMessage(data)
	require.NoError(t, err)

	assert.False(t, decoded.Success)
	assert.Equal(t, "storage full", decoded.Error)

	t.Log("✅ Store响应编解码正确")
}

// TestEncodeDecode_PingRequest 测试Ping请求编解码
func TestEncodeDecode_PingRequest(t *testing.T) {
	original := NewPingRequest(
		22222,
		types.NodeID("pinger"),
		[]string{"/ip4/192.168.1.1/tcp/4001"},
	)

	data, err := original.Encode()
	require.NoError(t, err)

	decoded, err := DecodeMessage(data)
	require.NoError(t, err)

	assert.Equal(t, MessageTypePing, decoded.Type)
	assert.Equal(t, types.NodeID("pinger"), decoded.Sender)

	t.Log("✅ Ping请求编解码正确")
}

// TestEncodeDecode_PingResponse 测试Ping响应编解码
func TestEncodeDecode_PingResponse(t *testing.T) {
	original := NewPingResponse(
		22222,
		types.NodeID("ponger"),
		[]string{"/ip4/192.168.1.2/tcp/4001"},
	)

	data, err := original.Encode()
	require.NoError(t, err)

	decoded, err := DecodeMessage(data)
	require.NoError(t, err)

	assert.Equal(t, MessageTypePingResponse, decoded.Type)
	assert.True(t, decoded.Success)

	t.Log("✅ Ping响应编解码正确")
}

// TestEncodeDecode_AddProviderRequest 测试AddProvider请求编解码
func TestEncodeDecode_AddProviderRequest(t *testing.T) {
	original := NewAddProviderRequest(
		33333,
		types.NodeID("provider"),
		[]string{"/ip4/10.0.0.1/tcp/4001"},
		"content-key",
		7200, // 2小时TTL
	)

	data, err := original.Encode()
	require.NoError(t, err)

	decoded, err := DecodeMessage(data)
	require.NoError(t, err)

	assert.Equal(t, MessageTypeAddProvider, decoded.Type)
	assert.Equal(t, "content-key", decoded.Key)
	assert.Equal(t, uint32(7200), decoded.TTL)

	t.Log("✅ AddProvider请求编解码正确")
}

// TestEncodeDecode_GetProvidersRequest 测试GetProviders请求编解码
func TestEncodeDecode_GetProvidersRequest(t *testing.T) {
	original := NewGetProvidersRequest(
		44444,
		types.NodeID("requester"),
		[]string{"/ip4/172.16.0.1/tcp/4001"},
		"wanted-content",
	)

	data, err := original.Encode()
	require.NoError(t, err)

	decoded, err := DecodeMessage(data)
	require.NoError(t, err)

	assert.Equal(t, MessageTypeGetProviders, decoded.Type)
	assert.Equal(t, "wanted-content", decoded.Key)

	t.Log("✅ GetProviders请求编解码正确")
}

// TestEncodeDecode_GetProvidersResponse 测试GetProviders响应编解码
func TestEncodeDecode_GetProvidersResponse(t *testing.T) {
	providers := []PeerRecord{
		{ID: "provider-1", Addrs: []string{"/ip4/1.1.1.1/tcp/4001"}, TTL: 3600},
		{ID: "provider-2", Addrs: []string{"/ip4/2.2.2.2/tcp/4001"}, TTL: 7200},
	}
	closerPeers := []PeerRecord{
		{ID: "closer-1", Addrs: []string{"/ip4/3.3.3.3/tcp/4001"}},
	}

	original := NewGetProvidersResponse(44444, types.NodeID("responder"), providers, closerPeers)

	data, err := original.Encode()
	require.NoError(t, err)

	decoded, err := DecodeMessage(data)
	require.NoError(t, err)

	assert.Equal(t, MessageTypeGetProvidersResponse, decoded.Type)
	assert.Equal(t, 2, len(decoded.Providers))
	assert.Equal(t, 1, len(decoded.CloserPeers))
	assert.Equal(t, uint32(3600), decoded.Providers[0].TTL)

	t.Log("✅ GetProviders响应编解码正确")
}

// TestEncodeDecode_RemoveProviderRequest 测试RemoveProvider请求编解码
func TestEncodeDecode_RemoveProviderRequest(t *testing.T) {
	original := NewRemoveProviderRequest(
		55555,
		types.NodeID("remover"),
		[]string{"/ip4/192.168.1.100/tcp/4001"},
		"remove-key",
	)

	data, err := original.Encode()
	require.NoError(t, err)

	decoded, err := DecodeMessage(data)
	require.NoError(t, err)

	assert.Equal(t, MessageTypeRemoveProvider, decoded.Type)
	assert.Equal(t, "remove-key", decoded.Key)

	t.Log("✅ RemoveProvider请求编解码正确")
}

// TestEncodeDecode_ErrorResponse 测试错误响应编解码
func TestEncodeDecode_ErrorResponse(t *testing.T) {
	// 针对 FIND_NODE 请求的错误响应
	original := NewErrorResponse(12345, types.NodeID("sender"), MessageTypeFindNode, "node not found")

	data, err := original.Encode()
	require.NoError(t, err)

	decoded, err := DecodeMessage(data)
	require.NoError(t, err)

	// 验证响应类型 = 请求类型 + 1
	assert.Equal(t, MessageTypeFindNodeResponse, decoded.Type)
	assert.False(t, decoded.Success)
	assert.Equal(t, "node not found", decoded.Error)

	t.Log("✅ 错误响应编解码正确")
}

// ============================================================================
// 边界条件和异常测试 - 重点发现BUG
// ============================================================================

// TestDecodeMessage_InvalidJSON 测试解码无效JSON
func TestDecodeMessage_InvalidJSON(t *testing.T) {
	invalidData := []byte("not valid json{{{")

	msg, err := DecodeMessage(invalidData)

	assert.Error(t, err)
	assert.Nil(t, msg)

	t.Log("✅ 无效JSON正确返回错误")
}

// TestDecodeMessage_EmptyData 测试解码空数据
func TestDecodeMessage_EmptyData(t *testing.T) {
	msg, err := DecodeMessage([]byte{})

	assert.Error(t, err)
	assert.Nil(t, msg)

	t.Log("✅ 空数据正确返回错误")
}

// TestDecodeMessage_NullJSON 测试解码null JSON
func TestDecodeMessage_NullJSON(t *testing.T) {
	msg, err := DecodeMessage([]byte("null"))

	// JSON null 解码到结构体会创建零值结构体（Go json.Unmarshal 行为）
	assert.NoError(t, err) // json.Unmarshal 对 null 不返回错误
	// 注意：json.Unmarshal(&msg, []byte("null")) 会创建零值 Message
	assert.NotNil(t, msg)
	assert.Equal(t, MessageType(0), msg.Type) // 零值类型

	t.Log("✅ null JSON正确处理（返回零值消息）")
}

// TestEncode_LargeValue 测试编码大数据
func TestEncode_LargeValue(t *testing.T) {
	// 创建1MB的值
	largeValue := make([]byte, 1024*1024)
	for i := range largeValue {
		largeValue[i] = byte(i % 256)
	}

	original := NewStoreRequest(12345, types.NodeID("sender"), nil, "large-key", largeValue, 3600)

	data, err := original.Encode()
	require.NoError(t, err)

	decoded, err := DecodeMessage(data)
	require.NoError(t, err)

	assert.Equal(t, len(largeValue), len(decoded.Value))
	assert.Equal(t, largeValue, decoded.Value)

	t.Log("✅ 大数据编解码正确")
}

// TestEncode_UnicodeStrings 测试编码Unicode字符串
func TestEncode_UnicodeStrings(t *testing.T) {
	original := NewFindNodeRequest(
		12345,
		types.NodeID("发送者-节点-🚀"),
		[]string{"/ip4/1.2.3.4/tcp/4001"},
		types.NodeID("目标-节点-🎯"),
	)

	data, err := original.Encode()
	require.NoError(t, err)

	decoded, err := DecodeMessage(data)
	require.NoError(t, err)

	assert.Equal(t, types.NodeID("发送者-节点-🚀"), decoded.Sender)
	assert.Equal(t, types.NodeID("目标-节点-🎯"), decoded.Target)

	t.Log("✅ Unicode字符串编解码正确")
}

// TestEncode_EmptyFields 测试空字段
func TestEncode_EmptyFields(t *testing.T) {
	original := &Message{
		Type:      MessageTypePing,
		RequestID: 0,
		Sender:    "",
	}

	data, err := original.Encode()
	require.NoError(t, err)

	decoded, err := DecodeMessage(data)
	require.NoError(t, err)

	assert.Equal(t, MessageTypePing, decoded.Type)
	assert.Equal(t, uint64(0), decoded.RequestID)
	assert.Equal(t, types.NodeID(""), decoded.Sender)

	t.Log("✅ 空字段编解码正确")
}

// TestEncode_SpecialCharacters 测试特殊字符
func TestEncode_SpecialCharacters(t *testing.T) {
	specialKey := "key/with\\special\"chars\n\t"
	original := NewFindValueRequest(12345, types.NodeID("sender"), nil, specialKey)

	data, err := original.Encode()
	require.NoError(t, err)

	decoded, err := DecodeMessage(data)
	require.NoError(t, err)

	assert.Equal(t, specialKey, decoded.Key)

	t.Log("✅ 特殊字符编解码正确")
}

// TestEncode_BinaryValue 测试二进制值
func TestEncode_BinaryValue(t *testing.T) {
	// 包含所有可能的字节值
	binaryValue := make([]byte, 256)
	for i := 0; i < 256; i++ {
		binaryValue[i] = byte(i)
	}

	original := NewStoreRequest(12345, types.NodeID("sender"), nil, "binary-key", binaryValue, 3600)

	data, err := original.Encode()
	require.NoError(t, err)

	decoded, err := DecodeMessage(data)
	require.NoError(t, err)

	assert.Equal(t, binaryValue, decoded.Value)

	t.Log("✅ 二进制值编解码正确")
}

// ============================================================================
// 消息构造器一致性测试 - 发现字段遗漏BUG
// ============================================================================

// TestNewErrorResponse_AllMessageTypes 测试所有消息类型的错误响应
func TestNewErrorResponse_AllMessageTypes(t *testing.T) {
	requestTypes := []MessageType{
		MessageTypeFindNode,
		MessageTypeFindValue,
		MessageTypeStore,
		MessageTypePing,
		MessageTypeAddProvider,
		MessageTypeGetProviders,
		MessageTypeRemoveProvider,
	}

	for _, reqType := range requestTypes {
		errResp := NewErrorResponse(12345, types.NodeID("sender"), reqType, "error")

		// 验证响应类型 = 请求类型 + 1
		expectedRespType := reqType + 1
		assert.Equal(t, expectedRespType, errResp.Type,
			"错误响应类型应该是请求类型+1: %s", reqType.String())
		assert.False(t, errResp.Success)
		assert.Equal(t, "error", errResp.Error)
	}

	t.Log("✅ 所有消息类型的错误响应正确")
}

// TestPeerRecord_WithTimestamp 测试PeerRecord时间戳
func TestPeerRecord_WithTimestamp(t *testing.T) {
	record := PeerRecord{
		ID:        "peer-1",
		Addrs:     []string{"/ip4/1.2.3.4/tcp/4001"},
		Timestamp: 1234567890000000000, // Unix 纳秒
		TTL:       3600,
	}

	original := &Message{
		Type:      MessageTypeGetProvidersResponse,
		RequestID: 12345,
		Providers: []PeerRecord{record},
	}

	data, err := original.Encode()
	require.NoError(t, err)

	decoded, err := DecodeMessage(data)
	require.NoError(t, err)

	assert.Equal(t, int64(1234567890000000000), decoded.Providers[0].Timestamp)
	assert.Equal(t, uint32(3600), decoded.Providers[0].TTL)

	t.Log("✅ PeerRecord时间戳编解码正确")
}

// TestMessage_RoundTrip_AllTypes 测试所有消息类型的完整往返
func TestMessage_RoundTrip_AllTypes(t *testing.T) {
	messages := []*Message{
		NewFindNodeRequest(1, "sender", []string{"/ip4/1.1.1.1/tcp/4001"}, "target"),
		NewFindNodeResponse(2, "sender", []PeerRecord{{ID: "peer", Addrs: []string{"/ip4/2.2.2.2/tcp/4001"}}}),
		NewFindValueRequest(3, "sender", []string{"/ip4/3.3.3.3/tcp/4001"}, "key"),
		NewFindValueResponse(4, "sender", []byte("value")),
		NewFindValueResponseWithPeers(5, "sender", []PeerRecord{{ID: "peer", Addrs: []string{"/ip4/4.4.4.4/tcp/4001"}}}),
		NewStoreRequest(6, "sender", []string{"/ip4/5.5.5.5/tcp/4001"}, "key", []byte("value"), 3600),
		NewStoreResponse(7, "sender", true, ""),
		NewStoreResponse(8, "sender", false, "error"),
		NewPingRequest(9, "sender", []string{"/ip4/6.6.6.6/tcp/4001"}),
		NewPingResponse(10, "sender", []string{"/ip4/7.7.7.7/tcp/4001"}),
		NewAddProviderRequest(11, "sender", []string{"/ip4/8.8.8.8/tcp/4001"}, "key", 7200),
		NewAddProviderResponse(12, "sender", true, ""),
		NewGetProvidersRequest(13, "sender", []string{"/ip4/9.9.9.9/tcp/4001"}, "key"),
		NewGetProvidersResponse(14, "sender", []PeerRecord{{ID: "prov"}}, []PeerRecord{{ID: "closer"}}),
		NewRemoveProviderRequest(15, "sender", []string{"/ip4/10.10.10.10/tcp/4001"}, "key"),
		NewRemoveProviderResponse(16, "sender", true, ""),
		NewErrorResponse(17, "sender", MessageTypeFindNode, "error"),
	}

	for i, original := range messages {
		data, err := original.Encode()
		require.NoError(t, err, "消息 %d 编码失败", i)

		decoded, err := DecodeMessage(data)
		require.NoError(t, err, "消息 %d 解码失败", i)

		// 验证关键字段
		assert.Equal(t, original.Type, decoded.Type, "消息 %d 类型不匹配", i)
		assert.Equal(t, original.RequestID, decoded.RequestID, "消息 %d RequestID 不匹配", i)
		assert.Equal(t, original.Sender, decoded.Sender, "消息 %d Sender 不匹配", i)
	}

	t.Logf("✅ 所有 %d 种消息类型往返测试通过", len(messages))
}
