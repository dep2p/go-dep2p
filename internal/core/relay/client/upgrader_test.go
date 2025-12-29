package client

import (
	"bytes"
	"context"
	"encoding/binary"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	endpointif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                    GAP-016: 中继连接升级测试
// ============================================================================

// TestUpgraderConfig 验证默认配置
func TestUpgraderConfig(t *testing.T) {
	config := DefaultUpgraderConfig()

	if config.HolePunchTimeout != 10*time.Second {
		t.Errorf("HolePunchTimeout = %v, want 10s", config.HolePunchTimeout)
	}

	if config.AddrExchangeTimeout != 5*time.Second {
		t.Errorf("AddrExchangeTimeout = %v, want 5s", config.AddrExchangeTimeout)
	}

	if config.RetryInterval != 5*time.Minute {
		t.Errorf("RetryInterval = %v, want 5m", config.RetryInterval)
	}

	if config.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", config.MaxRetries)
	}

	if !config.EnableAutoUpgrade {
		t.Error("EnableAutoUpgrade should be true by default")
	}

	t.Logf("Default config verified: HolePunchTimeout=%v, RetryInterval=%v",
		config.HolePunchTimeout, config.RetryInterval)
}

// TestUpgradeStates 验证升级状态常量
func TestUpgradeStates(t *testing.T) {
	states := []struct {
		state int
		name  string
	}{
		{UpgradeStatePending, "Pending"},
		{UpgradeStateExchanged, "Exchanged"},
		{UpgradeStatePunching, "Punching"},
		{UpgradeStateSuccess, "Success"},
		{UpgradeStateFailed, "Failed"},
	}

	// 验证状态值是有序的
	for i := 0; i < len(states)-1; i++ {
		if states[i].state >= states[i+1].state {
			t.Errorf("State order violated: %s(%d) >= %s(%d)",
				states[i].name, states[i].state,
				states[i+1].name, states[i+1].state)
		}
	}

	t.Log("Upgrade states verified: Pending → Exchanged → Punching → Success/Failed")
}

// TestAddressMessage 验证地址消息编码
func TestAddressMessage(t *testing.T) {
	// 模拟地址消息编码
	addrs := []string{"192.168.1.1:8000", "10.0.0.1:9000"}

	// 编码
	var buf bytes.Buffer
	buf.WriteByte(MsgTypeAddrs)

	countBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(countBuf, uint16(len(addrs)))
	buf.Write(countBuf)

	for _, addr := range addrs {
		addrBytes := []byte(addr)
		lenBuf := make([]byte, 2)
		binary.BigEndian.PutUint16(lenBuf, uint16(len(addrBytes)))
		buf.Write(lenBuf)
		buf.Write(addrBytes)
	}

	encoded := buf.Bytes()

	// 验证消息头
	if encoded[0] != MsgTypeAddrs {
		t.Errorf("Message type = %d, want %d", encoded[0], MsgTypeAddrs)
	}

	count := binary.BigEndian.Uint16(encoded[1:3])
	if count != uint16(len(addrs)) {
		t.Errorf("Address count = %d, want %d", count, len(addrs))
	}

	t.Logf("Address message encoding verified: %d bytes, %d addresses", len(encoded), count)
}

// TestStringAddress 验证字符串地址实现
func TestStringAddress(t *testing.T) {
	addr := &stringAddress{s: "192.168.1.1:8000"}

	// 无法识别协议前缀的地址使用默认 "relay" 类型
	if addr.Network() != "relay" {
		t.Errorf("Network() = %s, want relay", addr.Network())
	}

	if addr.String() != "192.168.1.1:8000" {
		t.Errorf("String() = %s, want 192.168.1.1:8000", addr.String())
	}

	if !bytes.Equal(addr.Bytes(), []byte("192.168.1.1:8000")) {
		t.Error("Bytes() mismatch")
	}

	// 测试 Equal
	same := &stringAddress{s: "192.168.1.1:8000"}
	diff := &stringAddress{s: "10.0.0.1:9000"}

	if !addr.Equal(same) {
		t.Error("Equal should return true for same address")
	}

	if addr.Equal(diff) {
		t.Error("Equal should return false for different address")
	}

	if addr.Equal(nil) {
		t.Error("Equal should return false for nil")
	}
}

// TestStringAddress_Network 测试 Network() 方法的协议推断
func TestStringAddress_Network(t *testing.T) {
	tests := []struct {
		addr    string
		network string
	}{
		{"/ip4/127.0.0.1/tcp/8080", "ip4"},
		{"/ip6/::1/tcp/8080", "ip6"},
		{"/tcp/8080", "tcp"},
		{"/udp/8080", "udp"},
		{"/quic/8080", "quic"},
		{"/quic-v1/udp/8080", "quic"},  // quic-v1 格式
		{"192.168.1.1:8000", "relay"},  // 无法识别
		{"", "relay"},                   // 空地址
	}

	for _, tt := range tests {
		addr := &stringAddress{s: tt.addr}
		if addr.Network() != tt.network {
			t.Errorf("Network() for %q = %s, want %s", tt.addr, addr.Network(), tt.network)
		}
	}
}

// TestProtocolID 验证协议 ID 格式
func TestProtocolID(t *testing.T) {
	// 验证三段版本号格式
	// 地址交换属于系统层协议（无需 Realm 验证），使用 /dep2p/sys/ 前缀
	expected := "/dep2p/sys/addr-exchange/1.0.0"
	if string(ProtocolAddrExchange) != expected {
		t.Errorf("ProtocolAddrExchange = %s, want %s", ProtocolAddrExchange, expected)
	}

	t.Log("Protocol ID format verified: semver 3-segment")
}

// TestNewConnectionUpgrader 验证创建升级器
func TestNewConnectionUpgrader(t *testing.T) {
	config := DefaultUpgraderConfig()

	upgrader := NewConnectionUpgrader(config, nil, nil, nil)

	if upgrader == nil {
		t.Fatal("NewConnectionUpgrader returned nil")
	}

	if upgrader.sessions == nil {
		t.Error("sessions map not initialized")
	}

	t.Log("ConnectionUpgrader creation verified")
}

// TestUpgraderConfigCustom 验证自定义配置
func TestUpgraderConfigCustom(t *testing.T) {
	config := UpgraderConfig{
		HolePunchTimeout:    20 * time.Second,
		AddrExchangeTimeout: 10 * time.Second,
		RetryInterval:       10 * time.Minute,
		MaxRetries:          5,
		EnableAutoUpgrade:   false,
	}

	if config.HolePunchTimeout != 20*time.Second {
		t.Error("Custom HolePunchTimeout not applied")
	}

	if config.EnableAutoUpgrade != false {
		t.Error("Custom EnableAutoUpgrade not applied")
	}

	t.Log("Custom config verified")
}

// ============================================================================
//                    生命周期测试
// ============================================================================

// TestConnectionUpgrader_StartStop 测试升级器的启动和停止
func TestConnectionUpgrader_StartStop(t *testing.T) {
	t.Run("正常启动停止", func(t *testing.T) {
		config := DefaultUpgraderConfig()
		upgrader := NewConnectionUpgrader(config, nil, nil, nil)

		ctx := context.Background()
		err := upgrader.Start(ctx)
		require.NoError(t, err)
		assert.Equal(t, int32(1), atomic.LoadInt32(&upgrader.running))

		err = upgrader.Stop()
		require.NoError(t, err)
		assert.Equal(t, int32(0), atomic.LoadInt32(&upgrader.running))
	})

	t.Run("重复启动", func(t *testing.T) {
		config := DefaultUpgraderConfig()
		upgrader := NewConnectionUpgrader(config, nil, nil, nil)

		ctx := context.Background()
		err := upgrader.Start(ctx)
		require.NoError(t, err)

		err = upgrader.Start(ctx) // 重复启动
		require.NoError(t, err)
		assert.Equal(t, int32(1), atomic.LoadInt32(&upgrader.running))

		upgrader.Stop()
	})

	t.Run("重复停止", func(t *testing.T) {
		config := DefaultUpgraderConfig()
		upgrader := NewConnectionUpgrader(config, nil, nil, nil)

		ctx := context.Background()
		upgrader.Start(ctx)
		err := upgrader.Stop()
		require.NoError(t, err)

		err = upgrader.Stop() // 重复停止
		require.NoError(t, err)
	})

	t.Run("未启动时停止", func(t *testing.T) {
		config := DefaultUpgraderConfig()
		upgrader := NewConnectionUpgrader(config, nil, nil, nil)

		// 不应 panic
		err := upgrader.Stop()
		require.NoError(t, err)
	})
}

// TestConnectionUpgrader_StopWithActiveSessions 测试带有活跃会话时的停止
func TestConnectionUpgrader_StopWithActiveSessions(t *testing.T) {
	config := DefaultUpgraderConfig()
	upgrader := NewConnectionUpgrader(config, nil, nil, nil)

	ctx := context.Background()
	upgrader.Start(ctx)

	// 手动添加一个模拟会话
	testNodeID := types.NodeID{1, 2, 3}
	upgrader.sessionsMu.Lock()
	upgrader.sessions[testNodeID] = &upgradeSession{
		done: make(chan struct{}),
	}
	upgrader.sessionsMu.Unlock()

	// 停止应该安全关闭所有会话
	err := upgrader.Stop()
	require.NoError(t, err)

	// 验证会话已清理
	upgrader.sessionsMu.RLock()
	count := len(upgrader.sessions)
	upgrader.sessionsMu.RUnlock()
	assert.Equal(t, 0, count)
}

// TestConnectionUpgrader_RetryLoopWithoutStart 测试未启动时 retryLoop 不会 panic
func TestConnectionUpgrader_RetryLoopWithoutStart(t *testing.T) {
	config := DefaultUpgraderConfig()
	config.RetryInterval = 10 * time.Millisecond
	upgrader := NewConnectionUpgrader(config, nil, nil, nil)

	// 手动调用 retryLoop（模拟 ctx 为 nil 的情况）
	// 应该安全返回而不是 panic
	upgrader.retryLoop()
}

// TestConnectionUpgrader_RetryFailedSessionsWithoutStart 测试未启动时 retryFailedSessions 的安全性
func TestConnectionUpgrader_RetryFailedSessionsWithoutStart(t *testing.T) {
	config := DefaultUpgraderConfig()
	upgrader := NewConnectionUpgrader(config, nil, nil, nil)

	// 不应 panic
	upgrader.retryFailedSessions()
}

// ============================================================================
//                    地址接收限制测试
// ============================================================================

// TestReceiveAddresses_MaxCount 测试 receiveAddresses 的最大地址数限制
func TestReceiveAddresses_MaxCount(t *testing.T) {
	// 构造一个超过限制的地址消息
	var buf bytes.Buffer
	buf.WriteByte(MsgTypeAddrs)

	// 设置地址数量超过 MaxAddressCount
	countBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(countBuf, MaxAddressCount+1)
	buf.Write(countBuf)

	// 创建一个模拟的 stream
	stream := &mockStream{reader: bytes.NewReader(buf.Bytes())}

	config := DefaultUpgraderConfig()
	upgrader := NewConnectionUpgrader(config, nil, nil, nil)

	// 应该返回错误
	_, err := upgrader.receiveAddresses(stream)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too many addresses")
}

// TestMaxAddressCount 验证常量值
func TestMaxAddressCount(t *testing.T) {
	// 验证 MaxAddressCount 是一个合理的值
	assert.Equal(t, uint16(100), uint16(MaxAddressCount))
}

// stringAddress 字符串地址（测试用）
type stringAddress struct {
	s string
}

func (a *stringAddress) Network() string {
	// 根据地址格式推断网络类型
	// 注意：匹配顺序很重要，更具体的协议应该先匹配
	switch {
	case bytes.Contains([]byte(a.s), []byte("/ip4/")):
		return "ip4"
	case bytes.Contains([]byte(a.s), []byte("/ip6/")):
		return "ip6"
	case bytes.Contains([]byte(a.s), []byte("/quic-v1")) || bytes.Contains([]byte(a.s), []byte("/quic/")):
		return "quic"
	case bytes.Contains([]byte(a.s), []byte("/tcp")):
		return "tcp"
	case bytes.Contains([]byte(a.s), []byte("/udp")):
		return "udp"
	default:
		return "relay"
	}
}
func (a *stringAddress) String() string                      { return a.s }
func (a *stringAddress) Bytes() []byte                       { return []byte(a.s) }
func (a *stringAddress) Equal(other endpointif.Address) bool { return other != nil && a.s == other.String() }
func (a *stringAddress) IsPublic() bool                      { return types.Multiaddr(a.s).IsPublic() }
func (a *stringAddress) IsPrivate() bool                     { return types.Multiaddr(a.s).IsPrivate() }
func (a *stringAddress) IsLoopback() bool                    { return types.Multiaddr(a.s).IsLoopback() }
func (a *stringAddress) Multiaddr() string                   { return a.s }

// mockStream 模拟 Stream 接口用于测试
type mockStream struct {
	reader *bytes.Reader
}

func (m *mockStream) Read(p []byte) (n int, err error) {
	return m.reader.Read(p)
}

func (m *mockStream) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (m *mockStream) Close() error {
	return nil
}

func (m *mockStream) CloseRead() error {
	return nil
}

func (m *mockStream) CloseWrite() error {
	return nil
}

func (m *mockStream) Reset() error {
	return nil
}

func (m *mockStream) SetDeadline(t time.Time) error {
	return nil
}

func (m *mockStream) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *mockStream) SetWriteDeadline(t time.Time) error {
	return nil
}

func (m *mockStream) Connection() endpointif.Connection {
	return nil
}

func (m *mockStream) ID() endpointif.StreamID {
	return 0
}

func (m *mockStream) ProtocolID() endpointif.ProtocolID {
	return ""
}

func (m *mockStream) Stats() types.StreamStats {
	return types.StreamStats{}
}

func (m *mockStream) IsClosed() bool {
	return false
}

func (m *mockStream) Priority() types.Priority {
	return 0
}

func (m *mockStream) SetPriority(priority types.Priority) {
}

var _ endpointif.Stream = (*mockStream)(nil)

