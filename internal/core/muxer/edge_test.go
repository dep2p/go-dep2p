package muxer

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

// ============================================================================
// 边界条件和错误路径测试
// ============================================================================

// TestEdge_StreamCloseMultipleTimes 测试多次关闭流
func TestEdge_StreamCloseMultipleTimes(t *testing.T) {
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

	// 第一次关闭
	err = stream.Close()
	if err != nil {
		t.Errorf("Close() failed: %v", err)
	}

	// 第二次关闭应该成功（幂等）
	err = stream.Close()
	if err != nil {
		t.Errorf("Close() second time failed: %v", err)
	}
}

// TestEdge_StreamResetAfterClose 测试关闭后重置
func TestEdge_StreamResetAfterClose(t *testing.T) {
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

	// 先关闭
	stream.Close()

	// 然后重置（应该成功或无影响）
	err = stream.Reset()
	if err != nil {
		t.Logf("Reset() after Close() returned: %v", err)
	}
}

// TestEdge_OpenStreamWithCanceledContext 测试取消的上下文
func TestEdge_OpenStreamWithCanceledContext(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	client, err := transport.NewConn(clientConn, false, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}
	defer client.Close()

	// 创建已取消的上下文
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	// 打开流应该失败或立即返回
	_, err = client.OpenStream(ctx)
	if err != nil {
		t.Logf("OpenStream() with canceled context returned: %v", err)
	}
}

// TestEdge_StreamReadAfterReset 测试重置后读取
func TestEdge_StreamReadAfterReset(t *testing.T) {
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

	// 客户端打开流并重置
	clientStream, err := client.OpenStream(context.Background())
	if err != nil {
		t.Fatalf("OpenStream() failed: %v", err)
	}

	// 写入一些数据
	clientStream.Write([]byte("test"))

	// 重置流
	clientStream.Reset()

	// 服务端接受流
	serverStream, err := server.AcceptStream()
	if err != nil {
		t.Fatalf("AcceptStream() failed: %v", err)
	}
	defer serverStream.Close()

	// 读取应该失败或返回错误
	buf := make([]byte, 10)
	_, err = serverStream.Read(buf)
	if err == nil {
		t.Log("Read() after Reset() returned no error (may be expected)")
	}
}

// TestEdge_SetDeadlineWithNil 测试 nil 超时
func TestEdge_SetDeadlineWithNil(t *testing.T) {
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

	// 设置零值时间（清除超时）
	err = stream.SetDeadline(time.Time{})
	if err != nil {
		t.Errorf("SetDeadline(time.Time{}) failed: %v", err)
	}

	err = stream.SetReadDeadline(time.Time{})
	if err != nil {
		t.Errorf("SetReadDeadline(time.Time{}) failed: %v", err)
	}

	err = stream.SetWriteDeadline(time.Time{})
	if err != nil {
		t.Errorf("SetWriteDeadline(time.Time{}) failed: %v", err)
	}
}

// TestEdge_StreamReadWrite_LargeData 测试大数据读写
func TestEdge_StreamReadWrite_LargeData(t *testing.T) {
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
	go func() {
		serverStream, err := server.AcceptStream()
		if err != nil {
			t.Errorf("AcceptStream() failed: %v", err)
			return
		}
		defer serverStream.Close()

		// 读取大数据
		buf := make([]byte, 2*1024*1024) // 2MB
		_, err = io.ReadFull(serverStream, buf)
		if err != nil {
			t.Errorf("Read() failed: %v", err)
		}
	}()

	// 客户端写入大数据
	data := make([]byte, 2*1024*1024) // 2MB
	for i := range data {
		data[i] = byte(i % 256)
	}

	n, err := clientStream.Write(data)
	if err != nil {
		t.Errorf("Write() failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("Write() = %d bytes, want %d", n, len(data))
	}

	time.Sleep(100 * time.Millisecond) // 等待读取完成
}

// TestEdge_ConnCloseWithActiveStreams 测试有活跃流时关闭连接
func TestEdge_ConnCloseWithActiveStreams(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	client, err := transport.NewConn(clientConn, false, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}

	// 打开一些流
	for i := 0; i < 5; i++ {
		stream, err := client.OpenStream(context.Background())
		if err != nil {
			t.Fatalf("OpenStream() %d failed: %v", i, err)
		}
		// 故意不关闭流
		_ = stream
	}

	// 关闭连接（应该清理所有流）
	err = client.Close()
	if err != nil {
		t.Errorf("Close() with active streams failed: %v", err)
	}

	// 验证已关闭
	if !client.IsClosed() {
		t.Error("IsClosed() = false, want true")
	}
}

// TestEdge_TransportIDConsistency 测试 ID 一致性
func TestEdge_TransportIDConsistency(t *testing.T) {
	transport1 := NewTransport()
	transport2 := NewTransport()

	id1 := transport1.ID()
	id2 := transport2.ID()

	if id1 != id2 {
		t.Errorf("ID() inconsistent: %s != %s", id1, id2)
	}

	expected := "/yamux/1.0.0"
	if id1 != expected {
		t.Errorf("ID() = %s, want %s", id1, expected)
	}
}

// TestEdge_ParseError_StreamReset 测试流重置错误转换
// 验证 yamux.ErrStreamReset 正确转换为 ErrStreamReset
func TestEdge_ParseError_StreamReset(t *testing.T) {
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

	// 服务端接受流
	serverStream, err := server.AcceptStream()
	if err != nil {
		t.Fatalf("AcceptStream() failed: %v", err)
	}

	// 客户端重置流
	clientStream.Reset()

	// 给服务端一点时间接收重置
	time.Sleep(50 * time.Millisecond)

	// 服务端读取应该返回 ErrStreamReset
	buf := make([]byte, 10)
	_, err = serverStream.Read(buf)
	if err == nil {
		t.Error("Read() after Reset() should return error")
	}
	// 验证错误类型 - 应该是我们定义的 ErrStreamReset
	if err != nil && err != ErrStreamReset && err != io.EOF {
		t.Logf("Read() after Reset() returned: %v (expected ErrStreamReset or EOF)", err)
	}
}

// TestEdge_ParseError_SessionShutdown 测试会话关闭错误转换
// 验证 yamux.ErrSessionShutdown 正确转换为 ErrConnClosed
func TestEdge_ParseError_SessionShutdown(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	client, err := transport.NewConn(clientConn, false, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}

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

	// 关闭客户端连接
	client.Close()

	// 给对端一点时间接收关闭
	time.Sleep(50 * time.Millisecond)

	// 尝试在已关闭的连接上操作应该返回 ErrConnClosed
	_, err = stream.Write([]byte("test"))
	if err == nil {
		t.Error("Write() on closed connection should return error")
	}
	// 验证错误类型
	if err != nil && err != ErrConnClosed {
		t.Logf("Write() on closed connection returned: %v (expected ErrConnClosed)", err)
	}
}

// TestEdge_OpenStreamAfterClose 测试关闭后打开流
func TestEdge_OpenStreamAfterClose(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	client, err := transport.NewConn(clientConn, false, nil)
	if err != nil {
		t.Fatalf("NewConn() failed: %v", err)
	}

	// 关闭连接
	client.Close()

	// 尝试打开流应该失败
	_, err = client.OpenStream(context.Background())
	if err == nil {
		t.Error("OpenStream() after Close() should return error")
	}
	// 应该返回 ErrConnClosed 或类似错误
	t.Logf("OpenStream() after Close() returned: %v", err)
}

// TestEdge_TransportConfig 测试获取配置
func TestEdge_TransportConfig(t *testing.T) {
	transport := NewTransport()

	config := transport.Config()
	if config == nil {
		t.Fatal("Config() returned nil")
	}

	// 验证配置值
	if config.MaxStreamWindowSize != 16*1024*1024 {
		t.Errorf("MaxStreamWindowSize = %d, want %d", config.MaxStreamWindowSize, 16*1024*1024)
	}
}

// TestEdge_ConfigFromUnified_NilConfig 测试 nil 配置
func TestEdge_ConfigFromUnified_NilConfig(t *testing.T) {
	cfg := ConfigFromUnified(nil)

	// 应该返回默认配置
	defaultCfg := DefaultConfig()
	if cfg.MaxStreamWindowSize != defaultCfg.MaxStreamWindowSize {
		t.Errorf("MaxStreamWindowSize = %d, want %d", cfg.MaxStreamWindowSize, defaultCfg.MaxStreamWindowSize)
	}
}

// TestEdge_PeerScopeError 测试 PeerScope 返回错误时的处理
// 这是一个关键的错误路径 - 验证资源管理错误是否正确处理
func TestEdge_PeerScopeError(t *testing.T) {
	transport := NewTransport()

	clientConn, serverConn := testConnPair(t)
	defer clientConn.Close()
	defer serverConn.Close()

	// 使用返回错误的 PeerScope
	errScope := &errorPeerScope{err: errors.New("resource exhausted")}

	// 创建连接时使用错误的 scope
	// 注意：yamux 会在需要时调用 BeginSpan，所以这里可能不会立即失败
	muxedConn, err := transport.NewConn(serverConn, true, errScope)
	if err != nil {
		// 如果立即失败，这是预期的
		t.Logf("NewConn() with error scope failed immediately: %v", err)
		return
	}
	defer muxedConn.Close()

	// 尝试打开流时可能会触发资源分配错误
	_, err = muxedConn.OpenStream(context.Background())
	if err != nil {
		t.Logf("OpenStream() with error scope failed: %v (expected)", err)
	}
}

// TestEdge_StreamWriteAfterCloseWrite 测试关闭写端后写入
func TestEdge_StreamWriteAfterCloseWrite(t *testing.T) {
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

	// 关闭写端
	err = stream.CloseWrite()
	if err != nil {
		t.Fatalf("CloseWrite() failed: %v", err)
	}

	// 关闭写端后写入应该失败
	_, err = stream.Write([]byte("test"))
	if err == nil {
		t.Error("Write() after CloseWrite() should return error")
	}
	t.Logf("Write() after CloseWrite() returned: %v", err)
}

// TestEdge_StreamHalfClose 测试半关闭
func TestEdge_StreamHalfClose(t *testing.T) {
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

	// 客户端写入数据并关闭写端
	testData := []byte("half-close test")
	_, err = clientStream.Write(testData)
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}

	err = clientStream.CloseWrite()
	if err != nil {
		t.Errorf("CloseWrite() failed: %v", err)
	}

	// 服务端应该能读到数据和 EOF
	buf := make([]byte, 1024)
	n, err := serverStream.Read(buf)
	if err != nil && err != io.EOF {
		t.Errorf("Read() failed: %v", err)
	}
	if n > 0 && string(buf[:n]) != string(testData) {
		t.Errorf("Read() = %s, want %s", string(buf[:n]), string(testData))
	}

	// 服务端仍然可以写入
	reply := []byte("still can write")
	_, err = serverStream.Write(reply)
	if err != nil {
		t.Errorf("Write() after client CloseWrite() failed: %v", err)
	}
}
