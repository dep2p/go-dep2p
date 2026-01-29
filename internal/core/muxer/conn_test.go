package muxer

import (
	"context"
	"testing"
	"time"
)

// ============================================================================
// MuxedConn 测试
// ============================================================================

// TestConn_OpenStream 测试打开流
func TestConn_OpenStream(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	client, err := transport.NewConn(clientConn, false, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}
	defer client.Close()

	server, err := transport.NewConn(serverConn, true, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}
	defer server.Close()

	// 客户端打开流
	stream, err := client.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream() failed: %v", err)
	}
	defer stream.Close()

	if stream == nil {
		t.Fatal("OpenStream() returned nil")
	}
}

// TestConn_AcceptStream 测试接受流
func TestConn_AcceptStream(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	client, err := transport.NewConn(clientConn, false, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}
	defer client.Close()

	server, err := transport.NewConn(serverConn, true, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}
	defer server.Close()

	// 客户端打开流
	go func() {
		stream, _ := client.OpenStream(context.Background())
		if stream != nil {
			defer stream.Close()
		}
	}()

	// 服务端接受流
	stream, err := server.AcceptStream()
	if err != nil {
		t.Fatalf("AcceptStream() failed: %v", err)
	}
	defer stream.Close()

	if stream == nil {
		t.Fatal("AcceptStream() returned nil")
	}
}

// TestConn_Close 测试关闭连接
func TestConn_Close(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	muxed, err := transport.NewConn(clientConn, false, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}

	err = muxed.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	// 关闭后再次关闭应该成功（幂等）
	err = muxed.Close()
	if err != nil {
		t.Errorf("Close() second time failed: %v", err)
	}
}

// TestConn_IsClosed 测试检查关闭状态
func TestConn_IsClosed(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	muxed, err := transport.NewConn(clientConn, false, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}

	// 初始应该未关闭
	if muxed.IsClosed() {
		t.Error("IsClosed() = true, want false")
	}

	// 关闭后应该为 true
	muxed.Close()

	if !muxed.IsClosed() {
		t.Error("IsClosed() = false, want true after Close()")
	}
}

// TestConn_OpenStreamWithContext 测试带上下文打开流
func TestConn_OpenStreamWithContext(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	client, err := transport.NewConn(clientConn, false, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}
	defer client.Close()

	server, err := transport.NewConn(serverConn, true, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}
	defer server.Close()

	// 使用超时上下文
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := client.OpenStream(ctx)
	if err != nil {
		t.Fatalf("OpenStream() with context failed: %v", err)
	}
	defer stream.Close()
}

// TestConn_AcceptStreamAfterClose 测试关闭后接受流
func TestConn_AcceptStreamAfterClose(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	server, err := transport.NewConn(serverConn, true, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}

	// 关闭连接
	server.Close()

	// 关闭后接受流应该失败
	_, err = server.AcceptStream()
	if err == nil {
		t.Error("AcceptStream() after Close() should fail")
	}
}
