package muxer

import (
	"context"
	"io"
	"testing"
	"time"
)

// ============================================================================
// MuxedStream 测试
// ============================================================================

// TestStream_ReadWrite 测试流读写
func TestStream_ReadWrite(t *testing.T) {
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

	// 客户端打开流并写入数据
	clientStream, err := client.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream() failed: %v", err)
	}
	defer clientStream.Close()

	testData := []byte("hello world")
	n, err := clientStream.Write(testData)
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Write() wrote %d bytes, want %d", n, len(testData))
	}

	// 服务端接受流并读取数据
	serverStream, err := server.AcceptStream()
	if err != nil {
		t.Fatalf("AcceptStream() failed: %v", err)
	}
	defer serverStream.Close()

	buf := make([]byte, len(testData))
	n, err = io.ReadFull(serverStream, buf)
	if err != nil {
		t.Fatalf("Read() failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Read() read %d bytes, want %d", n, len(testData))
	}
	if string(buf) != string(testData) {
		t.Errorf("Read() = %s, want %s", string(buf), string(testData))
	}
}

// TestStream_Close 测试关闭流
func TestStream_Close(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	client, err := transport.NewConn(clientConn, false, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}
	defer client.Close()

	stream, err := client.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream() failed: %v", err)
	}

	err = stream.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	// 关闭后再次关闭应该成功（幂等）
	err = stream.Close()
	if err != nil {
		t.Errorf("Close() second time failed: %v", err)
	}
}

// TestStream_Reset 测试重置流
func TestStream_Reset(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	client, err := transport.NewConn(clientConn, false, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}
	defer client.Close()

	stream, err := client.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream() failed: %v", err)
	}

	err = stream.Reset()
	if err != nil {
		t.Errorf("Reset() failed: %v", err)
	}
}

// TestStream_CloseRead 测试关闭读端
func TestStream_CloseRead(t *testing.T) {
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

	// 关闭读端
	err = serverStream.CloseRead()
	if err != nil {
		t.Errorf("CloseRead() failed: %v", err)
	}

	// 读取应该返回 EOF
	buf := make([]byte, 10)
	_, err = serverStream.Read(buf)
	if err == nil {
		t.Error("Read() after CloseRead() should fail")
	}
}

// TestStream_CloseWrite 测试关闭写端
func TestStream_CloseWrite(t *testing.T) {
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

	// 客户端打开流并关闭写端
	clientStream, err := client.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream() failed: %v", err)
	}
	defer clientStream.Close()

	// 写入数据
	testData := []byte("test")
	_, err = clientStream.Write(testData)
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}

	// 关闭写端
	err = clientStream.CloseWrite()
	if err != nil {
		t.Errorf("CloseWrite() failed: %v", err)
	}

	// 服务端应该能读到 EOF
	serverStream, err := server.AcceptStream()
	if err != nil {
		t.Fatalf("AcceptStream() failed: %v", err)
	}
	defer serverStream.Close()

	buf := make([]byte, len(testData)+10)
	n, err := serverStream.Read(buf)
	if err != nil && err != io.EOF {
		t.Errorf("Read() failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Read() = %d bytes, want %d", n, len(testData))
	}
}

// TestStream_SetDeadline 测试设置超时
func TestStream_SetDeadline(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	client, err := transport.NewConn(clientConn, false, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}
	defer client.Close()

	stream, err := client.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream() failed: %v", err)
	}
	defer stream.Close()

	// 设置超时
	deadline := time.Now().Add(1 * time.Second)
	err = stream.SetDeadline(deadline)
	if err != nil {
		t.Errorf("SetDeadline() failed: %v", err)
	}
}

// TestStream_SetReadDeadline 测试设置读超时
func TestStream_SetReadDeadline(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	client, err := transport.NewConn(clientConn, false, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}
	defer client.Close()

	stream, err := client.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream() failed: %v", err)
	}
	defer stream.Close()

	// 设置读超时
	deadline := time.Now().Add(100 * time.Millisecond)
	err = stream.SetReadDeadline(deadline)
	if err != nil {
		t.Errorf("SetReadDeadline() failed: %v", err)
	}

	// 读取应该超时
	buf := make([]byte, 10)
	_, err = stream.Read(buf)
	if err == nil {
		t.Error("Read() should timeout")
	}
}

// TestStream_SetWriteDeadline 测试设置写超时
func TestStream_SetWriteDeadline(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	client, err := transport.NewConn(clientConn, false, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}
	defer client.Close()

	stream, err := client.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream() failed: %v", err)
	}
	defer stream.Close()

	// 设置写超时
	deadline := time.Now().Add(1 * time.Second)
	err = stream.SetWriteDeadline(deadline)
	if err != nil {
		t.Errorf("SetWriteDeadline() failed: %v", err)
	}
}
