// Package relay 中继模块测试
package relay

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	relayif "github.com/dep2p/go-dep2p/pkg/interfaces/relay"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              协议消息测试
// ============================================================================

func TestEncodeDecodeReserveRequest(t *testing.T) {
	req := &ReserveRequest{
		TTL: 3600,
	}

	encoded := EncodeReserveRequest(req)
	decoded, err := DecodeReserveRequest(encoded)

	if err != nil {
		t.Fatalf("DecodeReserveRequest error: %v", err)
	}

	if decoded.TTL != req.TTL {
		t.Errorf("TTL mismatch: got %d, want %d", decoded.TTL, req.TTL)
	}
}

func TestEncodeDecodeReserveResponse_OK(t *testing.T) {
	resp := &ReserveResponse{
		Status: MsgHopReserveOK,
		TTL:    3600,
		Slots:  5,
		Addrs:  []string{"/ip4/192.168.1.1/tcp/8000", "/ip4/10.0.0.1/tcp/8000"},
	}

	encoded := EncodeReserveResponse(resp)
	decoded, err := DecodeReserveResponse(encoded)

	if err != nil {
		t.Fatalf("DecodeReserveResponse error: %v", err)
	}

	if decoded.Status != resp.Status {
		t.Errorf("Status mismatch: got %d, want %d", decoded.Status, resp.Status)
	}

	if decoded.TTL != resp.TTL {
		t.Errorf("TTL mismatch: got %d, want %d", decoded.TTL, resp.TTL)
	}

	if decoded.Slots != resp.Slots {
		t.Errorf("Slots mismatch: got %d, want %d", decoded.Slots, resp.Slots)
	}

	if len(decoded.Addrs) != len(resp.Addrs) {
		t.Errorf("Addrs length mismatch: got %d, want %d", len(decoded.Addrs), len(resp.Addrs))
	}
}

func TestEncodeDecodeReserveResponse_Error(t *testing.T) {
	resp := &ReserveResponse{
		Status:    MsgHopReserveError,
		ErrorCode: ErrCodeResourceLimitHit,
	}

	encoded := EncodeReserveResponse(resp)
	decoded, err := DecodeReserveResponse(encoded)

	if err != nil {
		t.Fatalf("DecodeReserveResponse error: %v", err)
	}

	if decoded.Status != resp.Status {
		t.Errorf("Status mismatch: got %d, want %d", decoded.Status, resp.Status)
	}

	if decoded.ErrorCode != resp.ErrorCode {
		t.Errorf("ErrorCode mismatch: got %d, want %d", decoded.ErrorCode, resp.ErrorCode)
	}
}

func TestEncodeDecodeConnectRequest(t *testing.T) {
	req := &ConnectRequest{
		DestPeer: types.NodeID{1, 2, 3, 4, 5, 6, 7, 8},
	}

	encoded := EncodeConnectRequest(req)
	decoded, err := DecodeConnectRequest(encoded)

	if err != nil {
		t.Fatalf("DecodeConnectRequest error: %v", err)
	}

	if decoded.DestPeer != req.DestPeer {
		t.Errorf("DestPeer mismatch")
	}
}

func TestEncodeDecodeConnectResponse(t *testing.T) {
	resp := &ConnectResponse{
		Status:    MsgHopConnectOK,
		ErrorCode: ErrCodeNone,
	}

	encoded := EncodeConnectResponse(resp)
	decoded, err := DecodeConnectResponse(encoded)

	if err != nil {
		t.Fatalf("DecodeConnectResponse error: %v", err)
	}

	if decoded.Status != resp.Status {
		t.Errorf("Status mismatch: got %d, want %d", decoded.Status, resp.Status)
	}
}

func TestEncodeDecodeStopConnectRequest(t *testing.T) {
	req := &StopConnectRequest{
		Relay: types.NodeID{1, 2, 3},
		Src:   types.NodeID{4, 5, 6},
	}

	encoded := EncodeStopConnectRequest(req)
	decoded, err := DecodeStopConnectRequest(encoded)

	if err != nil {
		t.Fatalf("DecodeStopConnectRequest error: %v", err)
	}

	if decoded.Relay != req.Relay {
		t.Errorf("Relay mismatch")
	}

	if decoded.Src != req.Src {
		t.Errorf("Src mismatch")
	}
}

func TestEncodeDecodeStatusMessage(t *testing.T) {
	msg := &StatusMessage{
		Reservations: 10,
		Circuits:     5,
		DataRate:     1024 * 1024,
	}

	encoded := EncodeStatusMessage(msg)
	decoded, err := DecodeStatusMessage(encoded)

	if err != nil {
		t.Fatalf("DecodeStatusMessage error: %v", err)
	}

	if decoded.Reservations != msg.Reservations {
		t.Errorf("Reservations mismatch: got %d, want %d", decoded.Reservations, msg.Reservations)
	}

	if decoded.Circuits != msg.Circuits {
		t.Errorf("Circuits mismatch: got %d, want %d", decoded.Circuits, msg.Circuits)
	}

	if decoded.DataRate != msg.DataRate {
		t.Errorf("DataRate mismatch: got %d, want %d", decoded.DataRate, msg.DataRate)
	}
}

func TestEncodeDecodeDataFrame(t *testing.T) {
	frame := &DataFrame{
		StreamID: 12345,
		Flags:    FlagFin,
		Data:     []byte("hello world"),
	}

	encoded := EncodeDataFrame(frame)
	decoded, err := DecodeDataFrame(encoded)

	if err != nil {
		t.Fatalf("DecodeDataFrame error: %v", err)
	}

	if decoded.StreamID != frame.StreamID {
		t.Errorf("StreamID mismatch: got %d, want %d", decoded.StreamID, frame.StreamID)
	}

	if decoded.Flags != frame.Flags {
		t.Errorf("Flags mismatch: got %d, want %d", decoded.Flags, frame.Flags)
	}

	if !bytes.Equal(decoded.Data, frame.Data) {
		t.Errorf("Data mismatch")
	}
}

// ============================================================================
//                              Voucher 测试
// ============================================================================

func TestEncodeDecodeVoucher(t *testing.T) {
	v := &Voucher{
		Relay:      types.NodeID{1, 2, 3},
		Peer:       types.NodeID{4, 5, 6},
		Expiration: time.Now().Add(time.Hour),
		Limit: Limit{
			Duration: 2 * time.Minute,
			Data:     1024 * 1024,
		},
	}

	encoded := EncodeVoucher(v)
	decoded, err := DecodeVoucher(encoded)

	if err != nil {
		t.Fatalf("DecodeVoucher error: %v", err)
	}

	if decoded.Relay != v.Relay {
		t.Errorf("Relay mismatch")
	}

	if decoded.Peer != v.Peer {
		t.Errorf("Peer mismatch")
	}

	if decoded.Limit.Duration != v.Limit.Duration {
		t.Errorf("Limit.Duration mismatch: got %v, want %v", decoded.Limit.Duration, v.Limit.Duration)
	}

	if decoded.Limit.Data != v.Limit.Data {
		t.Errorf("Limit.Data mismatch: got %d, want %d", decoded.Limit.Data, v.Limit.Data)
	}
}

func TestVoucher_IsExpired(t *testing.T) {
	// 未过期
	v1 := &Voucher{
		Expiration: time.Now().Add(time.Hour),
	}
	if v1.IsExpired() {
		t.Error("voucher should not be expired")
	}

	// 已过期
	v2 := &Voucher{
		Expiration: time.Now().Add(-time.Hour),
	}
	if !v2.IsExpired() {
		t.Error("voucher should be expired")
	}
}

// ============================================================================
//                              错误码测试
// ============================================================================

func TestErrorCode_String(t *testing.T) {
	tests := []struct {
		code ErrorCode
		want string
	}{
		{ErrCodeNone, "no error"},
		{ErrCodeMalformedMessage, "malformed message"},
		{ErrCodeResourceLimitHit, "resource limit reached"},
		{ErrCodeNoReservation, "no reservation"},
		{ErrCodePeerNotFound, "peer not found"},
		{ErrCodeRelayBusy, "relay busy"},
		{ErrorCode(9999), "unknown error"},
	}

	for _, tt := range tests {
		if got := tt.code.String(); got != tt.want {
			t.Errorf("ErrorCode(%d).String() = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestErrorCode_ToError(t *testing.T) {
	if ErrCodeNone.ToError() != nil {
		t.Error("ErrCodeNone.ToError() should return nil")
	}

	if ErrCodeMalformedMessage.ToError() == nil {
		t.Error("ErrCodeMalformedMessage.ToError() should not return nil")
	}
}

// ============================================================================
//                              消息读写测试
// ============================================================================

func TestWriteReadMessage(t *testing.T) {
	var buf bytes.Buffer

	payload := []byte("test payload")
	err := WriteMessage(&buf, MsgHopConnect, payload)
	if err != nil {
		t.Fatalf("WriteMessage error: %v", err)
	}

	msgType, readPayload, err := ReadMessage(&buf)
	if err != nil {
		t.Fatalf("ReadMessage error: %v", err)
	}

	if msgType != MsgHopConnect {
		t.Errorf("msgType mismatch: got %d, want %d", msgType, MsgHopConnect)
	}

	if !bytes.Equal(readPayload, payload) {
		t.Errorf("payload mismatch")
	}
}

func TestReadMessage_TooLarge(t *testing.T) {
	var buf bytes.Buffer

	// 写入过大的长度
	buf.WriteByte(0xFF)
	buf.WriteByte(0xFF)
	buf.WriteByte(0xFF)
	buf.WriteByte(0xFF)

	_, _, err := ReadMessage(&buf)
	if err == nil {
		t.Error("expected error for message too large")
	}
}

// ============================================================================
//                              Limit 测试
// ============================================================================

func TestDefaultLimit(t *testing.T) {
	limit := DefaultLimit()

	if limit.Duration != 2*time.Minute {
		t.Errorf("Duration mismatch: got %v, want %v", limit.Duration, 2*time.Minute)
	}

	if limit.Data != 1024*1024*128 {
		t.Errorf("Data mismatch: got %d, want %d", limit.Data, 1024*1024*128)
	}
}

// ============================================================================
//                              RelayClient 测试
// ============================================================================

func TestRelayClient_NewAndClose(t *testing.T) {
	config := relayif.DefaultConfig()
	client := NewRelayClient(nil, nil, config)

	if client == nil {
		t.Fatal("expected non-nil client")
	}

	if err := client.Close(); err != nil {
		t.Errorf("Close error: %v", err)
	}
}

func TestRelayClient_Relays(t *testing.T) {
	config := relayif.DefaultConfig()
	client := NewRelayClient(nil, nil, config)

	// 初始没有中继
	if len(client.Relays()) != 0 {
		t.Errorf("expected 0 relays, got %d", len(client.Relays()))
	}

	// 添加中继
	relayID := types.NodeID{1, 2, 3}
	client.AddRelay(relayID, nil)

	if len(client.Relays()) != 1 {
		t.Errorf("expected 1 relay, got %d", len(client.Relays()))
	}

	// 移除中继
	client.RemoveRelay(relayID)

	if len(client.Relays()) != 0 {
		t.Errorf("expected 0 relays after remove, got %d", len(client.Relays()))
	}
}

func TestRelayClient_AddMultipleRelays(t *testing.T) {
	config := relayif.DefaultConfig()
	client := NewRelayClient(nil, nil, config)

	// 添加多个中继
	for i := 0; i < 5; i++ {
		relayID := types.NodeID{byte(i)}
		client.AddRelay(relayID, nil)
	}

	if len(client.Relays()) != 5 {
		t.Errorf("expected 5 relays, got %d", len(client.Relays()))
	}
}

func TestRelayClient_RemoveNonExistent(t *testing.T) {
	config := relayif.DefaultConfig()
	client := NewRelayClient(nil, nil, config)

	// 移除不存在的中继不应报错
	relayID := types.NodeID{1, 2, 3}
	client.RemoveRelay(relayID)

	if len(client.Relays()) != 0 {
		t.Errorf("expected 0 relays, got %d", len(client.Relays()))
	}
}

func TestRelayClient_DuplicateAdd(t *testing.T) {
	config := relayif.DefaultConfig()
	client := NewRelayClient(nil, nil, config)

	relayID := types.NodeID{1, 2, 3}

	// 添加同一个中继两次
	client.AddRelay(relayID, nil)
	client.AddRelay(relayID, nil)

	// 应该只有一个（或者根据实现可能是两个）
	// 这里测试实际行为
	count := len(client.Relays())
	t.Logf("Duplicate add resulted in %d relays", count)
}

func TestRelayClient_DoubleClose(t *testing.T) {
	config := relayif.DefaultConfig()
	client := NewRelayClient(nil, nil, config)

	// 第一次关闭
	if err := client.Close(); err != nil {
		t.Errorf("First close error: %v", err)
	}

	// 第二次关闭应该是幂等的
	if err := client.Close(); err != nil {
		t.Errorf("Second close error: %v", err)
	}
}

// ============================================================================
//                              边界条件测试
// ============================================================================

func TestDecodeReserveRequest_Invalid(t *testing.T) {
	// 空数据
	_, err := DecodeReserveRequest(nil)
	if err == nil {
		t.Error("expected error for nil data")
	}

	// 损坏的数据
	_, err = DecodeReserveRequest([]byte{0x00})
	if err == nil {
		t.Error("expected error for corrupted data")
	}
}

func TestDecodeConnectRequest_Invalid(t *testing.T) {
	_, err := DecodeConnectRequest(nil)
	if err == nil {
		t.Error("expected error for nil data")
	}

	_, err = DecodeConnectRequest([]byte{0x00, 0x01})
	if err == nil {
		t.Error("expected error for incomplete data")
	}
}

func TestDecodeDataFrame_Invalid(t *testing.T) {
	_, err := DecodeDataFrame(nil)
	if err == nil {
		t.Error("expected error for nil data")
	}

	_, err = DecodeDataFrame([]byte{0x00})
	if err == nil {
		t.Error("expected error for too short data")
	}
}

func TestLimit_IsValid(t *testing.T) {
	limit := DefaultLimit()

	// 默认限制应该有效
	if limit.Duration == 0 {
		t.Error("Duration should not be 0")
	}

	if limit.Data == 0 {
		t.Error("Data should not be 0")
	}
}

// ============================================================================
//                              协议一致性测试 (Bug 修复验证)
// ============================================================================

// TestClientServerProtocolConsistency_Reserve 验证客户端-服务器预留请求协议一致性
// 这是对 BUG-001 (客户端-服务器预留请求协议不匹配) 的修复验证
func TestClientServerProtocolConsistency_Reserve(t *testing.T) {
	config := relayif.DefaultConfig()
	client := NewRelayClient(nil, nil, config)

	// 测试客户端写入预留请求
	var buf bytes.Buffer
	err := client.writeReserveRequest(&buf)
	if err != nil {
		t.Fatalf("writeReserveRequest error: %v", err)
	}

	// 验证格式: type(1) + version(1) + TTL(4) = 6 字节
	if buf.Len() != 6 {
		t.Errorf("reserve request length = %d, want 6 (type + version + TTL)", buf.Len())
	}

	data := buf.Bytes()
	// 验证消息类型
	if data[0] != MsgTypeReserve {
		t.Errorf("message type = %d, want %d (MsgTypeReserve)", data[0], MsgTypeReserve)
	}

	// 验证版本号
	if data[1] != 1 {
		t.Errorf("version = %d, want 1", data[1])
	}

	// 验证 TTL (默认 1 小时 = 3600 秒)
	ttl := uint32(data[2])<<24 | uint32(data[3])<<16 | uint32(data[4])<<8 | uint32(data[5])
	if ttl != 3600 {
		t.Errorf("TTL = %d, want 3600", ttl)
	}
}

// TestClientServerProtocolConsistency_Connect 验证客户端-服务器连接请求协议一致性
// 这是对 BUG-002 (客户端-服务器连接请求协议不匹配) 的修复验证
//
// 背景（IMPL-1227 设计断层修复）：
// - Server 端 readConnectRequest 已扩展为支持 ProtoLen + Protocol 字段
// - Client 端 writeConnectRequest 必须同步写入这些字段
// - 格式定义见 design/protocols/transport/relay.md#connect-请求格式impl-1227-v2
func TestClientServerProtocolConsistency_Connect(t *testing.T) {
	config := relayif.DefaultConfig()
	client := NewRelayClient(nil, nil, config)

	t.Run("无协议字段", func(t *testing.T) {
		destPeer := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8}

		// 测试客户端写入连接请求（无协议）
		var buf bytes.Buffer
		err := client.writeConnectRequest(&buf, destPeer, "")
		if err != nil {
			t.Fatalf("writeConnectRequest error: %v", err)
		}

		// 验证格式: type(1) + version(1) + destPeerID(32) + protoLen(2) = 36 字节
		if buf.Len() != 36 {
			t.Errorf("connect request length = %d, want 36 (type + version + nodeID + protoLen)", buf.Len())
		}

		data := buf.Bytes()
		// 验证消息类型
		if data[0] != MsgTypeConnect {
			t.Errorf("message type = %d, want %d (MsgTypeConnect)", data[0], MsgTypeConnect)
		}

		// 验证版本号
		if data[1] != 1 {
			t.Errorf("version = %d, want 1", data[1])
		}

		// 验证目标节点 ID
		var readPeer types.NodeID
		copy(readPeer[:], data[2:34])
		if readPeer != destPeer {
			t.Errorf("destPeer mismatch")
		}

		// 验证 protoLen=0
		protoLen := binary.BigEndian.Uint16(data[34:36])
		if protoLen != 0 {
			t.Errorf("protoLen = %d, want 0", protoLen)
		}
	})

	t.Run("携带协议字段", func(t *testing.T) {
		destPeer := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8}
		protocol := "/dep2p/app/test-realm/chat/1.0.0"

		// 测试客户端写入连接请求（携带协议）
		var buf bytes.Buffer
		err := client.writeConnectRequest(&buf, destPeer, types.ProtocolID(protocol))
		if err != nil {
			t.Fatalf("writeConnectRequest error: %v", err)
		}

		// 验证格式: type(1) + version(1) + destPeerID(32) + protoLen(2) + protocol(N)
		expectedLen := 36 + len(protocol)
		if buf.Len() != expectedLen {
			t.Errorf("connect request length = %d, want %d", buf.Len(), expectedLen)
		}

		data := buf.Bytes()

		// 验证消息类型
		if data[0] != MsgTypeConnect {
			t.Errorf("message type = %d, want %d", data[0], MsgTypeConnect)
		}

		// 验证 protoLen
		protoLen := binary.BigEndian.Uint16(data[34:36])
		if int(protoLen) != len(protocol) {
			t.Errorf("protoLen = %d, want %d", protoLen, len(protocol))
		}

		// 验证协议内容
		readProto := string(data[36 : 36+protoLen])
		if readProto != protocol {
			t.Errorf("protocol = %q, want %q", readProto, protocol)
		}
	})
}

// TestClientServerProtocolConsistency_ReserveResponse 验证预留响应解析一致性
// 这是对 BUG-003 (响应解析协议不匹配) 的修复验证
func TestClientServerProtocolConsistency_ReserveResponse(t *testing.T) {
	config := relayif.DefaultConfig()
	client := NewRelayClient(nil, nil, config)

	t.Run("正确解析成功响应", func(t *testing.T) {
		// 构造服务器响应: type(1) + version(1) + TTL(4) + slots(2)
		var buf bytes.Buffer
		buf.WriteByte(MsgTypeReserveOK)  // type
		buf.WriteByte(1)                 // version
		buf.Write([]byte{0, 0, 14, 16})  // TTL = 3600
		buf.Write([]byte{0, 5})          // slots = 5

		res, err := client.readReserveResponse(&buf)
		if err != nil {
			t.Fatalf("readReserveResponse error: %v", err)
		}

		// 验证 slots
		if res.slots != 5 {
			t.Errorf("slots = %d, want 5", res.slots)
		}

		// 验证 TTL 大约为 3600 秒 (允许 1 秒误差)
		expectedExpiry := time.Now().Add(3600 * time.Second)
		diff := res.expires.Sub(expectedExpiry)
		if diff < -time.Second || diff > time.Second {
			t.Errorf("expires diff = %v, want within 1s", diff)
		}
	})

	t.Run("正确解析错误响应", func(t *testing.T) {
		// 构造服务器错误响应: type(1) + version(1) + errCode(2)
		var buf bytes.Buffer
		buf.WriteByte(MsgTypeReserveError) // type
		buf.WriteByte(1)                   // version
		buf.Write([]byte{0, 200})          // error code = 200

		_, err := client.readReserveResponse(&buf)
		if err == nil {
			t.Error("expected error for error response")
		}
	})
}

// TestClientServerProtocolConsistency_ConnectResponse 验证连接响应解析一致性
func TestClientServerProtocolConsistency_ConnectResponse(t *testing.T) {
	config := relayif.DefaultConfig()
	client := NewRelayClient(nil, nil, config)

	t.Run("正确解析成功响应", func(t *testing.T) {
		// 构造服务器响应: type(1) + version(1) = 2 字节
		var buf bytes.Buffer
		buf.WriteByte(MsgTypeConnectOK) // type
		buf.WriteByte(1)                // version

		err := client.readConnectResponse(&buf)
		if err != nil {
			t.Fatalf("readConnectResponse error: %v", err)
		}
	})

	t.Run("正确解析错误响应", func(t *testing.T) {
		// 构造服务器错误响应: type(1) + version(1) + errCode(2)
		// 使用服务器定义的错误码：ErrCodeNoReservation = 201
		var buf bytes.Buffer
		buf.WriteByte(MsgTypeConnectError) // type
		buf.WriteByte(1)                   // version
		binary.Write(&buf, binary.BigEndian, uint16(ErrCodeNoReservation)) // error code = 201

		err := client.readConnectResponse(&buf)
		if err == nil {
			t.Fatalf("expected error, got nil")
		}
		if !errors.Is(err, ErrReservationFailed) {
			t.Errorf("expected ErrReservationFailed, got: %v", err)
		}
	})
}


