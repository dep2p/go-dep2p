package muxer

import (
	"context"
	"io"
	"testing"
)

// ============================================================================
// 集成测试
// ============================================================================

// TestIntegration_ClientServerStreams 测试客户端-服务端流通信
func TestIntegration_ClientServerStreams(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	client, err := transport.NewConn(clientConn, false, nil)
	if err != nil {
		t.Fatalf("NewConn() client failed: %v", err)
	}
	defer client.Close()

	server, err := transport.NewConn(serverConn, true, nil)
	if err != nil {
		t.Fatalf("NewConn() server failed: %v", err)
	}
	defer server.Close()

	// 客户端打开流并发送消息
	clientStream, err := client.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream() failed: %v", err)
	}
	defer clientStream.Close()

	msg := []byte("hello from client")
	n, err := clientStream.Write(msg)
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}
	if n != len(msg) {
		t.Errorf("Write() wrote %d bytes, want %d", n, len(msg))
	}

	// 服务端接受流并读取消息
	serverStream, err := server.AcceptStream()
	if err != nil {
		t.Fatalf("AcceptStream() failed: %v", err)
	}
	defer serverStream.Close()

	buf := make([]byte, 1024)
	n, err = serverStream.Read(buf)
	if err != nil {
		t.Fatalf("Read() failed: %v", err)
	}

	if string(buf[:n]) != string(msg) {
		t.Errorf("Read() = %s, want %s", string(buf[:n]), string(msg))
	}

	// 服务端回复
	reply := []byte("hello from server")
	n, err = serverStream.Write(reply)
	if err != nil {
		t.Fatalf("Write() reply failed: %v", err)
	}

	// 客户端读取回复
	n, err = clientStream.Read(buf)
	if err != nil {
		t.Fatalf("Read() reply failed: %v", err)
	}

	if string(buf[:n]) != string(reply) {
		t.Errorf("Read() reply = %s, want %s", string(buf[:n]), string(reply))
	}
}

// TestIntegration_BidirectionalData 测试双向数据传输
func TestIntegration_BidirectionalData(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	client, err := transport.NewConn(clientConn, false, nil)
	if err != nil {
		t.Fatalf("NewConn() client failed: %v", err)
	}
	defer client.Close()

	server, err := transport.NewConn(serverConn, true, nil)
	if err != nil {
		t.Fatalf("NewConn() server failed: %v", err)
	}
	defer server.Close()

	// 客户端打开流
	clientStream, err := client.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream() failed: %v", err)
	}
	defer clientStream.Close()

	// 服务端接受流
	serverStream, err := server.AcceptStream()
	if err != nil {
		t.Fatalf("AcceptStream() failed: %v", err)
	}
	defer serverStream.Close()

	// 双向传输大数据
	dataSize := 1024 * 1024 // 1MB
	testData := make([]byte, dataSize)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	done := make(chan bool, 2)

	// 客户端写入
	go func() {
		n, err := clientStream.Write(testData)
		if err != nil {
			t.Errorf("Write() failed: %v", err)
		}
		if n != dataSize {
			t.Errorf("Write() = %d bytes, want %d", n, dataSize)
		}
		done <- true
	}()

	// 服务端读取
	go func() {
		buf := make([]byte, dataSize)
		n, err := io.ReadFull(serverStream, buf)
		if err != nil {
			t.Errorf("Read() failed: %v", err)
		}
		if n != dataSize {
			t.Errorf("Read() = %d bytes, want %d", n, dataSize)
		}
		done <- true
	}()

	// 等待完成
	<-done
	<-done
}

// TestIntegration_MultipleConnections 测试多个连接
func TestIntegration_MultipleConnections(t *testing.T) {
	transport := NewTransport()

	numConns := 5

	for i := 0; i < numConns; i++ {
		clientConn, serverConn := testConnPair(t)
		defer clientConn.Close()
		defer serverConn.Close()

		client, err := transport.NewConn(clientConn, false, nil)
		if err != nil {
			t.Fatalf("NewConn() client %d failed: %v", i, err)
		}
		defer client.Close()

		server, err := transport.NewConn(serverConn, true, nil)
		if err != nil {
			t.Fatalf("NewConn() server %d failed: %v", i, err)
		}
		defer server.Close()

		// 每个连接打开一个流
		stream, err := client.OpenStream(context.Background())
		if err != nil {
			t.Fatalf("OpenStream() on conn %d failed: %v", i, err)
		}
		stream.Close()
	}
}

// TestIntegration_ResourceManagerIntegration 测试资源管理器集成
func TestIntegration_ResourceManagerIntegration(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	// 使用 mock PeerScope
	scope := &mockPeerScope{}

	client, err := transport.NewConn(clientConn, false, scope)
	if err != nil {
		t.Fatalf("NewConn() with scope failed: %v", err)
	}
	defer client.Close()

	server, err := transport.NewConn(serverConn, true, scope)
	if err != nil {
		t.Fatalf("NewConn() server failed: %v", err)
	}
	defer server.Close()

	// 打开流（应该调用 scope.BeginSpan()）
	stream, err := client.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream() failed: %v", err)
	}
	defer stream.Close()

	// 写入数据测试内存管理
	data := make([]byte, 1024)
	_, err = stream.Write(data)
	if err != nil {
		t.Errorf("Write() failed: %v", err)
	}
}
