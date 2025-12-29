package quic

import (
	"context"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/identity"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
)

// TestConnBasicProperties 测试连接基本属性
func TestConnBasicProperties(t *testing.T) {
	clientConn, serverConn, cleanup := setupTestConnectionForConn(t)
	defer cleanup()

	// 测试本地地址
	if clientConn.LocalAddr() == nil {
		t.Error("客户端本地地址不应为 nil")
	}

	// 测试远程地址
	if clientConn.RemoteAddr() == nil {
		t.Error("客户端远程地址不应为 nil")
	}

	// 测试 net.Addr
	if clientConn.LocalNetAddr() == nil {
		t.Error("LocalNetAddr 不应为 nil")
	}
	if clientConn.RemoteNetAddr() == nil {
		t.Error("RemoteNetAddr 不应为 nil")
	}

	// 测试传输协议名称
	if clientConn.Transport() != "quic" {
		t.Errorf("传输协议应为 quic，实际为 %s", clientConn.Transport())
	}

	// 测试关闭状态
	if clientConn.IsClosed() {
		t.Error("新连接不应处于关闭状态")
	}

	// 测试 Context
	ctx := clientConn.Context()
	if ctx == nil {
		t.Error("Context 不应为 nil")
	}

	// 测试 ConnectionState
	state := clientConn.ConnectionState()
	t.Logf("连接状态: Protocol=%s, Version=%s, CipherSuite=%s", state.Protocol, state.Version, state.CipherSuite)

	// 测试 QuicConn
	if clientConn.QuicConn() == nil {
		t.Error("QuicConn 不应为 nil")
	}

	_ = serverConn // 使用变量
}

// TestConnClose 测试连接关闭
func TestConnClose(t *testing.T) {
	clientConn, _, cleanup := setupTestConnectionForConn(t)
	defer cleanup()

	// 关闭连接
	if err := clientConn.Close(); err != nil {
		t.Errorf("关闭连接失败: %v", err)
	}

	// 验证已关闭
	if !clientConn.IsClosed() {
		t.Error("连接应处于关闭状态")
	}

	// 重复关闭不应报错
	if err := clientConn.Close(); err != nil {
		t.Errorf("重复关闭不应报错: %v", err)
	}
}

// TestConnReadWrite 测试连接读写
func TestConnReadWrite(t *testing.T) {
	clientConn, serverConn, cleanup := setupTestConnectionForConn(t)
	defer cleanup()

	testData := []byte("Hello, Connection!")
	done := make(chan struct{})

	// 服务端读取
	go func() {
		defer close(done)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		stream, err := serverConn.AcceptStream(ctx)
		if err != nil {
			t.Logf("接受流失败: %v", err)
			return
		}
		defer func() { _ = stream.Close() }()

		buf := make([]byte, len(testData))
		n, err := stream.Read(buf)
		if err != nil {
			t.Logf("读取失败: %v", err)
			return
		}

		if string(buf[:n]) != string(testData) {
			t.Errorf("数据不匹配: 期望 %s，实际 %s", testData, buf[:n])
		}
	}()

	// 客户端写入
	time.Sleep(50 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := clientConn.OpenStream(ctx)
	if err != nil {
		t.Fatalf("打开流失败: %v", err)
	}

	_, err = stream.Write(testData)
	if err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	stream.Close()

	<-done
}

// TestConnDeadline 测试连接超时设置
func TestConnDeadline(t *testing.T) {
	clientConn, _, cleanup := setupTestConnectionForConn(t)
	defer cleanup()

	deadline := time.Now().Add(time.Second)

	// 测试设置统一超时
	err := clientConn.SetDeadline(deadline)
	if err != nil {
		t.Logf("设置超时: %v", err)
	}

	// 测试设置读超时
	err = clientConn.SetReadDeadline(deadline)
	if err != nil {
		t.Logf("设置读超时: %v", err)
	}

	// 测试设置写超时
	err = clientConn.SetWriteDeadline(deadline)
	if err != nil {
		t.Logf("设置写超时: %v", err)
	}
}

// TestConnOpenAcceptStream 测试流操作
func TestConnOpenAcceptStream(t *testing.T) {
	clientConn, serverConn, cleanup := setupTestConnectionForConn(t)
	defer cleanup()

	done := make(chan struct{})

	// 服务端接受流
	go func() {
		defer close(done)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		stream, err := serverConn.AcceptStream(ctx)
		if err != nil {
			t.Logf("接受流失败: %v", err)
			return
		}
		stream.Close()
	}()

	// 客户端打开流
	time.Sleep(50 * time.Millisecond)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := clientConn.OpenStream(ctx)
	if err != nil {
		t.Fatalf("打开流失败: %v", err)
	}
	stream.Write([]byte("test"))
	stream.Close()

	<-done
}

// setupTestConnectionForConn 创建测试连接
func setupTestConnectionForConn(t *testing.T) (*Conn, *Conn, func()) {
	t.Helper()

	// 创建服务端
	mgr := identity.NewManager(identityif.DefaultConfig())
	serverID, _ := mgr.Create()
	serverTransport, _ := NewTransport(transportif.DefaultConfig(), serverID)

	listener, err := serverTransport.Listen(NewAddress("127.0.0.1", 0))
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}

	actualAddr := listener.Addr().(*Address)

	// 创建客户端
	clientID, _ := mgr.Create()
	clientTransport, _ := NewTransport(transportif.DefaultConfig(), clientID)

	// 接受连接
	var serverConn transportif.Conn
	var acceptErr error
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		serverConn, acceptErr = listener.Accept()
	}()

	// 客户端连接
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientConn, err := clientTransport.Dial(ctx, actualAddr)
	if err != nil {
		t.Fatalf("拨号失败: %v", err)
	}

	// 等待服务端
	select {
	case <-acceptDone:
		if acceptErr != nil {
			t.Fatalf("接受连接失败: %v", acceptErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("接受连接超时")
	}

	cleanup := func() {
		clientConn.Close()
		if serverConn != nil {
			serverConn.Close()
		}
		listener.Close()
		clientTransport.Close()
		serverTransport.Close()
	}

	return clientConn.(*Conn), serverConn.(*Conn), cleanup
}

