package tcp

import (
	"context"
	"testing"
	"time"

	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
)

func TestNewTransport(t *testing.T) {
	config := transportif.DefaultConfig()
	transport := NewTransport(config)
	defer transport.Close()

	if transport == nil {
		t.Fatal("Transport should not be nil")
	}

	if transport.IsClosed() {
		t.Error("New transport should not be closed")
	}
}

func TestTransport_Protocols(t *testing.T) {
	transport := NewTransport(transportif.DefaultConfig())
	defer transport.Close()

	protocols := transport.Protocols()
	if len(protocols) != 3 {
		t.Errorf("Expected 3 protocols, got %d", len(protocols))
	}

	expected := map[string]bool{
		"tcp":  true,
		"tcp4": true,
		"tcp6": true,
	}

	for _, p := range protocols {
		if !expected[p] {
			t.Errorf("Unexpected protocol: %s", p)
		}
	}
}

func TestTransport_CanDial(t *testing.T) {
	transport := NewTransport(transportif.DefaultConfig())
	defer transport.Close()

	tests := []struct {
		addr     string
		expected bool
	}{
		{"/ip4/127.0.0.1/tcp/4001", true},
		{"/ip6/::1/tcp/4001", true},
		{"/ip4/0.0.0.0/quic-v1/4001", false},
	}

	for _, tt := range tests {
		addr, err := ParseAddress(tt.addr)
		if err != nil {
			if tt.expected {
				t.Errorf("Failed to parse address %s: %v", tt.addr, err)
			}
			continue
		}

		result := transport.CanDial(addr)
		if result != tt.expected {
			t.Errorf("CanDial(%s) = %v, expected %v", tt.addr, result, tt.expected)
		}
	}
}

func TestTransport_ListenAndDial(t *testing.T) {
	transport := NewTransport(transportif.DefaultConfig())
	defer transport.Close()

	// 监听
	listenAddr := MustParseAddress("/ip4/127.0.0.1/tcp/0")
	listener, err := transport.Listen(listenAddr)
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}
	defer func() { _ = listener.Close() }()

	// 获取实际监听地址
	actualAddr := listener.Addr()
	t.Logf("Listening on %s", actualAddr.String())

	// 服务端接受连接
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := listener.Accept()
		if err != nil {
			t.Errorf("Accept failed: %v", err)
			return
		}
		defer func() { _ = conn.Close() }()

		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			t.Errorf("Read failed: %v", err)
			return
		}

		if string(buf[:n]) != "Hello" {
			t.Errorf("Expected 'Hello', got '%s'", string(buf[:n]))
		}

		_, err = conn.Write([]byte("World"))
		if err != nil {
			t.Errorf("Write failed: %v", err)
		}
	}()

	// 客户端连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := transport.Dial(ctx, actualAddr)
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// 发送数据
	_, err = conn.Write([]byte("Hello"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// 接收响应
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	if string(buf[:n]) != "World" {
		t.Errorf("Expected 'World', got '%s'", string(buf[:n]))
	}

	// 等待服务端完成
	<-serverDone
}

func TestTransport_Close(t *testing.T) {
	transport := NewTransport(transportif.DefaultConfig())

	// 监听
	addr := MustParseAddress("/ip4/127.0.0.1/tcp/0")
	listener, err := transport.Listen(addr)
	if err != nil {
		t.Fatalf("Listen failed: %v", err)
	}

	if transport.ListenerCount() != 1 {
		t.Errorf("Expected 1 listener, got %d", transport.ListenerCount())
	}

	// 关闭传输层
	err = transport.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}

	if !transport.IsClosed() {
		t.Error("Transport should be closed")
	}

	// 再次关闭应该无害
	err = transport.Close()
	if err != nil {
		t.Errorf("Second close should not fail: %v", err)
	}

	// 监听器应该已关闭
	if !listener.(*Listener).IsClosed() {
		t.Error("Listener should be closed after transport close")
	}
}

// TestFactory 已删除：TransportFactory 接口已删除（v1.1 清理）

