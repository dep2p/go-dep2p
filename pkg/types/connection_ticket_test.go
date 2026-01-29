package types

import (
	"strings"
	"testing"
)

// TestDecodeConnectionTicket_ValidTicket 测试有效票据解码
func TestDecodeConnectionTicket_ValidTicket(t *testing.T) {
	// 创建一个有效的票据
	nodeID := Base58Encode(make([]byte, 32)) // 32 字节的 Base58 编码
	ticket := NewConnectionTicket(nodeID, []string{"/ip4/127.0.0.1/udp/4001/quic-v1/p2p/" + nodeID})

	encoded, err := ticket.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decoded, err := DecodeConnectionTicket(encoded)
	if err != nil {
		t.Fatalf("DecodeConnectionTicket failed: %v", err)
	}

	if decoded.NodeID != nodeID {
		t.Errorf("NodeID mismatch: got %s, want %s", decoded.NodeID, nodeID)
	}

	if len(decoded.AddressHints) != 1 {
		t.Errorf("AddressHints count mismatch: got %d, want 1", len(decoded.AddressHints))
	}
}

// TestDecodeConnectionTicket_EmptyString 测试空字符串
func TestDecodeConnectionTicket_EmptyString(t *testing.T) {
	_, err := DecodeConnectionTicket("")
	if err == nil {
		t.Error("expected error for empty string")
	}
	if !strings.Contains(err.Error(), "empty string") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestDecodeConnectionTicket_Whitespace 测试纯空格
func TestDecodeConnectionTicket_Whitespace(t *testing.T) {
	_, err := DecodeConnectionTicket("   ")
	if err == nil {
		t.Error("expected error for whitespace-only string")
	}
}

// TestDecodeConnectionTicket_MissingPrefix 测试缺少前缀
func TestDecodeConnectionTicket_MissingPrefix(t *testing.T) {
	_, err := DecodeConnectionTicket("eyJub2RlX2lkIjoiYWJjIn0")
	if err == nil {
		t.Error("expected error for missing prefix")
	}
	if !strings.Contains(err.Error(), "missing dep2p:// prefix") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestDecodeConnectionTicket_EmptyPayload 测试空载荷
func TestDecodeConnectionTicket_EmptyPayload(t *testing.T) {
	_, err := DecodeConnectionTicket("dep2p://")
	if err == nil {
		t.Error("expected error for empty payload")
	}
	if !strings.Contains(err.Error(), "empty payload") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestDecodeConnectionTicket_TooLongPayload 测试超长载荷
func TestDecodeConnectionTicket_TooLongPayload(t *testing.T) {
	// 创建超长字符串
	longPayload := strings.Repeat("a", 3000)
	_, err := DecodeConnectionTicket("dep2p://" + longPayload)
	if err == nil {
		t.Error("expected error for too long payload")
	}
	if !strings.Contains(err.Error(), "payload too long") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestDecodeConnectionTicket_InvalidBase64 测试无效 Base64
func TestDecodeConnectionTicket_InvalidBase64(t *testing.T) {
	_, err := DecodeConnectionTicket("dep2p://not-valid-base64!!!")
	if err == nil {
		t.Error("expected error for invalid base64")
	}
	if !strings.Contains(err.Error(), "decode ticket") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestDecodeConnectionTicket_InvalidJSON 测试无效 JSON
func TestDecodeConnectionTicket_InvalidJSON(t *testing.T) {
	// "not json" 的 Base64 编码
	_, err := DecodeConnectionTicket("dep2p://bm90IGpzb24")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "unmarshal ticket") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestDecodeConnectionTicket_MissingNodeID 测试缺少 NodeID
func TestDecodeConnectionTicket_MissingNodeID(t *testing.T) {
	// {"address_hints":["addr"]} 的 Base64 编码
	_, err := DecodeConnectionTicket("dep2p://eyJhZGRyZXNzX2hpbnRzIjpbImFkZHIiXX0")
	if err == nil {
		t.Error("expected error for missing node_id")
	}
	if !strings.Contains(err.Error(), "missing node_id") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestDecodeConnectionTicket_ShortNodeID 测试过短的 NodeID
func TestDecodeConnectionTicket_ShortNodeID(t *testing.T) {
	// {"node_id":"abc"} 的 Base64 编码
	_, err := DecodeConnectionTicket("dep2p://eyJub2RlX2lkIjoiYWJjIn0")
	if err == nil {
		t.Error("expected error for short node_id")
	}
	if !strings.Contains(err.Error(), "node_id length invalid") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestDecodeConnectionTicket_InvalidNodeIDFormat 测试无效的 NodeID 格式
func TestDecodeConnectionTicket_InvalidNodeIDFormat(t *testing.T) {
	// {"node_id":"0OIl..."} - 包含 Base58 不允许的字符
	// 使用足够长但格式无效的 NodeID
	invalidNodeID := strings.Repeat("0", 44) // '0' 不在 Base58 字母表中
	ticket := &ConnectionTicket{NodeID: invalidNodeID}
	encoded, _ := ticket.Encode()

	_, err := DecodeConnectionTicket(encoded)
	if err == nil {
		t.Error("expected error for invalid node_id format")
	}
	if !strings.Contains(err.Error(), "node_id format invalid") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestDecodeConnectionTicket_FilterInvalidAddresses 测试过滤无效地址
func TestDecodeConnectionTicket_FilterInvalidAddresses(t *testing.T) {
	nodeID := Base58Encode(make([]byte, 32))
	ticket := &ConnectionTicket{
		NodeID: nodeID,
		AddressHints: []string{
			"/ip4/127.0.0.1/tcp/4001/p2p/" + nodeID, // 有效
			"",                                      // 空 - 应被过滤
			"   ",                                   // 纯空格 - 应被过滤
			"not-a-multiaddr",                       // 无效格式 - 应被过滤
			"/ip4/1.2.3.4/tcp/4001;ls",              // 危险字符 - 应被过滤
			strings.Repeat("a", 600),                // 过长 - 应被过滤
			"/ip4/192.168.1.1/udp/4001/quic-v1/p2p/" + nodeID, // 有效
		},
	}

	encoded, err := ticket.Encode()
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	decoded, err := DecodeConnectionTicket(encoded)
	if err != nil {
		t.Fatalf("DecodeConnectionTicket failed: %v", err)
	}

	// 应该只剩下 2 个有效地址
	if len(decoded.AddressHints) != 2 {
		t.Errorf("expected 2 valid addresses, got %d: %v", len(decoded.AddressHints), decoded.AddressHints)
	}
}

// TestDecodeConnectionTicket_DangerousCharacters 测试危险字符过滤
func TestDecodeConnectionTicket_DangerousCharacters(t *testing.T) {
	nodeID := Base58Encode(make([]byte, 32))

	dangerousAddrs := []string{
		"/ip4/1.2.3.4;rm -rf /",
		"/ip4/1.2.3.4|cat /etc/passwd",
		"/ip4/1.2.3.4&echo pwned",
		"/ip4/1.2.3.4$HOME",
		"/ip4/1.2.3.4`id`",
		"/ip4/1.2.3.4\ninjected",
		"/ip4/1.2.3.4\rinjected",
		"/ip4/1.2.3.4\\injected",
	}

	for _, addr := range dangerousAddrs {
		ticket := &ConnectionTicket{
			NodeID:       nodeID,
			AddressHints: []string{addr},
		}

		encoded, err := ticket.Encode()
		if err != nil {
			t.Fatalf("Encode failed: %v", err)
		}

		decoded, err := DecodeConnectionTicket(encoded)
		if err != nil {
			t.Fatalf("DecodeConnectionTicket failed: %v", err)
		}

		// 危险地址应该被过滤掉
		if len(decoded.AddressHints) != 0 {
			t.Errorf("dangerous address not filtered: %q", addr)
		}
	}
}

// TestConnectionTicket_IsExpired 测试过期检查
func TestConnectionTicket_IsExpired(t *testing.T) {
	nodeID := Base58Encode(make([]byte, 32))

	// 新创建的票据不应过期
	ticket := NewConnectionTicket(nodeID, nil)
	if ticket.IsExpired(24 * 60 * 60 * 1e9) { // 24小时
		t.Error("new ticket should not be expired")
	}

	// 无时间戳的票据不应过期
	ticket2 := &ConnectionTicket{NodeID: nodeID, Timestamp: 0}
	if ticket2.IsExpired(1) {
		t.Error("ticket without timestamp should not be expired")
	}

	// 过期的票据
	ticket3 := &ConnectionTicket{NodeID: nodeID, Timestamp: 1} // 1970年
	if !ticket3.IsExpired(24 * 60 * 60 * 1e9) {
		t.Error("old ticket should be expired")
	}
}

// BenchmarkDecodeConnectionTicket 性能测试
func BenchmarkDecodeConnectionTicket(b *testing.B) {
	nodeID := Base58Encode(make([]byte, 32))
	ticket := NewConnectionTicket(nodeID, []string{"/ip4/127.0.0.1/udp/4001/quic-v1/p2p/" + nodeID})
	encoded, _ := ticket.Encode()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = DecodeConnectionTicket(encoded)
	}
}
