package endpoint

import (
	"context"
	"testing"
	"time"

	coreif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
)

// TestRetryConfig_Default 测试默认配置
func TestRetryConfig_Default(t *testing.T) {
	config := coreif.DefaultRetryConfig()

	if config.MaxRetries != 5 {
		t.Errorf("expected MaxRetries=5, got %d", config.MaxRetries)
	}
	if config.InitialBackoff != 100*time.Millisecond {
		t.Errorf("expected InitialBackoff=100ms, got %v", config.InitialBackoff)
	}
	if config.MaxBackoff != 30*time.Second {
		t.Errorf("expected MaxBackoff=30s, got %v", config.MaxBackoff)
	}
}

// TestRetryConfig_Custom 测试自定义配置
func TestRetryConfig_Custom(t *testing.T) {
	config := &coreif.RetryConfig{
		MaxRetries:     3,
		InitialBackoff: 50 * time.Millisecond,
		MaxBackoff:     5 * time.Second,
	}

	if config.MaxRetries != 3 {
		t.Errorf("expected MaxRetries=3, got %d", config.MaxRetries)
	}
	if config.InitialBackoff != 50*time.Millisecond {
		t.Errorf("expected InitialBackoff=50ms, got %v", config.InitialBackoff)
	}
	if config.MaxBackoff != 5*time.Second {
		t.Errorf("expected MaxBackoff=5s, got %v", config.MaxBackoff)
	}
}

// TestConnectWithRetry_ContextCancelBeforeStart 测试启动前上下文取消
func TestConnectWithRetry_ContextCancelBeforeStart(t *testing.T) {
	e := &Endpoint{
		conns:   make(map[coreif.NodeID]*Connection),
		closeCh: make(chan struct{}),
	}

	// 创建一个已取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	nodeID := coreif.NodeID{}
	config := &coreif.RetryConfig{
		MaxRetries:     5,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     1 * time.Second,
	}

	conn, err := e.ConnectWithRetry(ctx, nodeID, config)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
	if conn != nil {
		t.Fatal("expected nil connection")
	}
}

// TestConnectWithRetry_NilConfig 测试 nil 配置使用默认值
func TestConnectWithRetry_NilConfig(t *testing.T) {
	e := &Endpoint{
		conns:   make(map[coreif.NodeID]*Connection),
		closeCh: make(chan struct{}),
	}

	// 创建一个短超时的上下文
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	nodeID := coreif.NodeID{}

	// 使用 nil 配置应该使用默认配置
	conn, err := e.ConnectWithRetry(ctx, nodeID, nil)
	// 由于没有设置真正的连接逻辑，会超时
	if err == nil {
		t.Error("expected error due to context timeout or connection failure")
	}
	if conn != nil {
		t.Fatal("expected nil connection")
	}
}

// TestExponentialBackoff_Calculation 测试指数退避计算逻辑
func TestExponentialBackoff_Calculation(t *testing.T) {
	initialBackoff := 100 * time.Millisecond
	maxBackoff := 1 * time.Second

	backoff := initialBackoff
	expectedSequence := []time.Duration{
		100 * time.Millisecond, // 初始
		200 * time.Millisecond, // *2
		400 * time.Millisecond, // *2
		800 * time.Millisecond, // *2
		1 * time.Second,        // capped at maxBackoff
		1 * time.Second,        // stays at maxBackoff
	}

	for i, expected := range expectedSequence {
		if backoff != expected {
			t.Errorf("iteration %d: expected %v, got %v", i, expected, backoff)
		}

		// 模拟退避时间翻倍
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// TestExtractRelayIDFromAddr 测试从 relay 地址提取 relay ID
func TestExtractRelayIDFromAddr(t *testing.T) {
	tests := []struct {
		name     string
		addr     string
		expected string
	}{
		{
			name:     "standard relay address",
			addr:     "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelayID123/p2p-circuit/p2p/QmTargetID456",
			expected: "QmRelayID123",
		},
		{
			name:     "relay address with tcp",
			addr:     "/ip4/192.168.1.1/tcp/4001/p2p/QmRelay/p2p-circuit/p2p/QmTarget",
			expected: "QmRelay",
		},
		{
			name:     "no relay ID",
			addr:     "/ip4/1.2.3.4/tcp/4001/p2p/QmDirect",
			expected: "",
		},
		{
			name:     "empty address",
			addr:     "",
			expected: "",
		},
		{
			name:     "p2p-circuit at start",
			addr:     "/p2p/QmRelay/p2p-circuit",
			expected: "QmRelay",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRelayIDFromAddr(tt.addr)
			if result != tt.expected {
				t.Errorf("extractRelayIDFromAddr(%q) = %q, want %q", tt.addr, result, tt.expected)
			}
		})
	}
}
