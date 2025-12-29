package quic

import (
	"context"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/identity"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
)

// createTestTransport 创建测试用的传输层
func createTestTransport(t *testing.T) *Transport {
	t.Helper()

	// 创建测试身份
	mgr := identity.NewManager(identityif.DefaultConfig())
	id, err := mgr.Create()
	if err != nil {
		t.Fatalf("创建身份失败: %v", err)
	}

	// 创建传输层
	config := transportif.DefaultConfig()
	transport, err := NewTransport(config, id)
	if err != nil {
		t.Fatalf("创建传输层失败: %v", err)
	}

	return transport
}

// TestNewTransport 测试传输层创建
func TestNewTransport(t *testing.T) {
	transport := createTestTransport(t)
	defer transport.Close()

	if transport == nil {
		t.Fatal("传输层不应为 nil")
	}

	if transport.IsClosed() {
		t.Error("新创建的传输层不应处于关闭状态")
	}

	protocols := transport.Protocols()
	if len(protocols) == 0 {
		t.Error("协议列表不应为空")
	}

	// 验证协议
	hasQuic := false
	for _, p := range protocols {
		if p == "quic" || p == "quic-v1" {
			hasQuic = true
			break
		}
	}
	if !hasQuic {
		t.Error("协议列表应包含 quic")
	}
}

// TestTransportListenAndAccept 测试监听和接受连接
func TestTransportListenAndAccept(t *testing.T) {
	transport := createTestTransport(t)
	defer transport.Close()

	// 监听
	addr := NewAddress("127.0.0.1", 0)
	listener, err := transport.Listen(addr)
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	defer func() { _ = listener.Close() }()

	// 验证监听器
	listenAddr := listener.Addr()
	if listenAddr == nil {
		t.Fatal("监听地址不应为 nil")
	}

	t.Logf("监听地址: %s", listenAddr.String())
	t.Logf("多地址: %s", listener.Multiaddr())

	// 验证监听器数量
	if transport.ListenersCount() != 1 {
		t.Errorf("监听器数量应为 1，实际为 %d", transport.ListenersCount())
	}
}

// TestTransportDialAndConnect 测试拨号和连接
func TestTransportDialAndConnect(t *testing.T) {
	// 创建服务端传输层
	serverTransport := createTestTransport(t)
	defer serverTransport.Close()

	// 服务端监听
	serverAddr := NewAddress("127.0.0.1", 0)
	listener, err := serverTransport.Listen(serverAddr)
	if err != nil {
		t.Fatalf("服务端监听失败: %v", err)
	}
	defer func() { _ = listener.Close() }()

	// 获取实际监听地址
	actualAddr := listener.Addr().(*Address)

	// 创建客户端传输层
	clientTransport := createTestTransport(t)
	defer clientTransport.Close()

	// 接受连接的 goroutine
	var serverConn transportif.Conn
	var acceptErr error
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		serverConn, acceptErr = listener.Accept()
	}()

	// 客户端拨号
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	clientConn, err := clientTransport.Dial(ctx, actualAddr)
	if err != nil {
		t.Fatalf("客户端拨号失败: %v", err)
	}
	defer clientConn.Close()

	// 等待服务端接受连接
	select {
	case <-acceptDone:
		if acceptErr != nil {
			t.Fatalf("服务端接受连接失败: %v", acceptErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("服务端接受连接超时")
	}
	defer serverConn.Close()

	// 验证连接
	if clientConn.IsClosed() {
		t.Error("客户端连接不应处于关闭状态")
	}
	if serverConn.IsClosed() {
		t.Error("服务端连接不应处于关闭状态")
	}

	// 验证传输协议名称
	if clientConn.Transport() != "quic" {
		t.Errorf("传输协议应为 quic，实际为 %s", clientConn.Transport())
	}
}

// TestTransportDataTransfer 测试数据传输
func TestTransportDataTransfer(t *testing.T) {
	// 创建服务端
	serverTransport := createTestTransport(t)
	defer serverTransport.Close()

	listener, err := serverTransport.Listen(NewAddress("127.0.0.1", 0))
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	defer func() { _ = listener.Close() }()

	actualAddr := listener.Addr().(*Address)

	// 创建客户端
	clientTransport := createTestTransport(t)
	defer clientTransport.Close()

	// 数据传输测试
	testData := []byte("Hello, QUIC Transport!")
	receivedChan := make(chan []byte, 1)
	errChan := make(chan error, 2)

	// 服务端接受连接并读取数据
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			errChan <- err
			return
		}

		// 打开流接收数据
		quicConn := conn.(*Conn)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		stream, err := quicConn.AcceptStream(ctx)
		if err != nil {
			errChan <- err
			conn.Close()
			return
		}

		data, err := io.ReadAll(stream)
		stream.Close()
		conn.Close()

		if err != nil {
			errChan <- err
			return
		}
		receivedChan <- data
	}()

	// 稍微延迟确保服务端准备好
	time.Sleep(50 * time.Millisecond)

	// 客户端发送数据
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := clientTransport.Dial(ctx, actualAddr)
	if err != nil {
		t.Fatalf("拨号失败: %v", err)
	}

	// 打开流发送数据
	quicConn := conn.(*Conn)
	stream, err := quicConn.OpenStream(context.Background())
	if err != nil {
		conn.Close()
		t.Fatalf("打开流失败: %v", err)
	}

	_, err = stream.Write(testData)
	if err != nil {
		stream.Close()
		conn.Close()
		t.Fatalf("写入数据失败: %v", err)
	}
	stream.Close()
	conn.Close()

	// 等待服务端接收数据
	select {
	case receivedData := <-receivedChan:
		if string(receivedData) != string(testData) {
			t.Errorf("数据不匹配: 期望 %s，实际 %s", testData, receivedData)
		}
	case err := <-errChan:
		// "正常关闭" 错误是预期的
		if err != nil && !isNormalCloseError(err) {
			t.Fatalf("服务端错误: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("等待数据超时")
	}
}

// isNormalCloseError 检查是否是正常关闭错误
func isNormalCloseError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "正常关闭") || 
		strings.Contains(errStr, "Application error 0x0") ||
		strings.Contains(errStr, "closed")
}

// TestTransportClose 测试关闭传输层
func TestTransportClose(t *testing.T) {
	transport := createTestTransport(t)

	// 创建监听器
	listener, err := transport.Listen(NewAddress("127.0.0.1", 0))
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}

	// 验证监听器存在
	if transport.ListenersCount() != 1 {
		t.Error("应有一个监听器")
	}

	// 关闭传输层
	if err := transport.Close(); err != nil {
		t.Fatalf("关闭传输层失败: %v", err)
	}

	// 验证已关闭
	if !transport.IsClosed() {
		t.Error("传输层应处于关闭状态")
	}

	// 验证监听器已清理
	if transport.ListenersCount() != 0 {
		t.Error("监听器应已清理")
	}

	// 验证监听器已关闭
	if !listener.(*Listener).IsClosed() {
		t.Error("监听器应处于关闭状态")
	}

	// 重复关闭不应报错
	if err := transport.Close(); err != nil {
		t.Errorf("重复关闭不应报错: %v", err)
	}
}

// TestTransportCanDial 测试 CanDial
func TestTransportCanDial(t *testing.T) {
	transport := createTestTransport(t)
	defer transport.Close()

	tests := []struct {
		name     string
		addr     *Address
		expected bool
	}{
		{
			name:     "IPv4 地址",
			addr:     NewAddress("127.0.0.1", 8080),
			expected: true,
		},
		{
			name:     "IPv6 地址",
			addr:     NewAddress("::1", 8080),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := transport.CanDial(tt.addr)
			if result != tt.expected {
				t.Errorf("CanDial(%s) = %v，期望 %v", tt.addr.String(), result, tt.expected)
			}
		})
	}

	// 测试 nil 地址
	if transport.CanDial(nil) {
		t.Error("CanDial(nil) 应返回 false")
	}
}

// TestTransportDialTimeout 测试拨号超时
func TestTransportDialTimeout(t *testing.T) {
	transport := createTestTransport(t)
	defer transport.Close()

	// 尝试连接一个不存在的地址
	addr := NewAddress("192.0.2.1", 12345) // TEST-NET-1，不可路由

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := transport.Dial(ctx, addr)
	if err == nil {
		t.Error("应返回超时错误")
	}
}

// TestTransportDialClosedTransport 测试在关闭的传输层上拨号
func TestTransportDialClosedTransport(t *testing.T) {
	transport := createTestTransport(t)
	transport.Close()

	addr := NewAddress("127.0.0.1", 8080)
	ctx := context.Background()

	_, err := transport.Dial(ctx, addr)
	if err == nil {
		t.Error("在关闭的传输层上拨号应返回错误")
	}
}

// TestTransportListenClosedTransport 测试在关闭的传输层上监听
func TestTransportListenClosedTransport(t *testing.T) {
	transport := createTestTransport(t)
	transport.Close()

	addr := NewAddress("127.0.0.1", 0)
	_, err := transport.Listen(addr)
	if err == nil {
		t.Error("在关闭的传输层上监听应返回错误")
	}
}

// TestTransportConcurrentOperations 测试并发操作
func TestTransportConcurrentOperations(t *testing.T) {
	serverTransport := createTestTransport(t)
	defer serverTransport.Close()

	listener, err := serverTransport.Listen(NewAddress("127.0.0.1", 0))
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	defer func() { _ = listener.Close() }()

	actualAddr := listener.Addr().(*Address)

	// 启动服务端接受连接
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	// 并发拨号
	var wg sync.WaitGroup
	numDialers := 5

	for i := 0; i < numDialers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			clientTransport := createTestTransport(t)
			defer clientTransport.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			conn, err := clientTransport.Dial(ctx, actualAddr)
			if err != nil {
				t.Logf("拨号错误: %v", err)
				return
			}
			conn.Close()
		}()
	}

	wg.Wait()
}

// TestNewTransportWithTLS 测试使用自定义 TLS 配置创建传输层
func TestNewTransportWithTLS(t *testing.T) {
	// 创建身份
	mgr := identity.NewManager(identityif.DefaultConfig())
	id, err := mgr.Create()
	if err != nil {
		t.Fatalf("创建身份失败: %v", err)
	}

	// 生成 TLS 配置
	tlsConfigGen := NewTLSConfig(id)
	tlsConfig, err := tlsConfigGen.GenerateConfig()
	if err != nil {
		t.Fatalf("生成 TLS 配置失败: %v", err)
	}

	// 使用自定义 TLS 配置创建传输层
	config := transportif.DefaultConfig()
	transport := NewTransportWithTLS(config, tlsConfig)
	defer transport.Close()

	if transport == nil {
		t.Fatal("传输层不应为 nil")
	}

	// 验证配置
	if transport.Config().IdleTimeout != config.IdleTimeout {
		t.Error("配置不匹配")
	}
}

// TestTransportGetConnAndListener 测试获取连接和监听器
func TestTransportGetConnAndListener(t *testing.T) {
	transport := createTestTransport(t)
	defer transport.Close()

	// 创建监听器
	listener, err := transport.Listen(NewAddress("127.0.0.1", 0))
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}
	defer func() { _ = listener.Close() }()

	// 获取监听器
	addrStr := listener.Addr().String()
	gotListener := transport.GetListener(addrStr)
	if gotListener == nil {
		t.Error("应能获取监听器")
	}

	// 获取不存在的监听器
	if transport.GetListener("nonexistent:1234") != nil {
		t.Error("不存在的监听器应返回 nil")
	}

	// 获取不存在的连接
	if transport.GetConn("nonexistent:1234") != nil {
		t.Error("不存在的连接应返回 nil")
	}
}

// TestListenerAcceptClosed 测试在关闭的监听器上接受连接
func TestListenerAcceptClosed(t *testing.T) {
	transport := createTestTransport(t)
	defer transport.Close()

	listener, err := transport.Listen(NewAddress("127.0.0.1", 0))
	if err != nil {
		t.Fatalf("监听失败: %v", err)
	}

	// 关闭监听器
	listener.Close()

	// 尝试接受连接
	_, err = listener.Accept()
	if err == nil {
		t.Error("在关闭的监听器上接受连接应返回错误")
	}
}

// BenchmarkTransportDial 基准测试拨号性能
func BenchmarkTransportDial(b *testing.B) {
	// 创建服务端
	mgr := identity.NewManager(identityif.DefaultConfig())
	serverID, _ := mgr.Create()
	serverTransport, _ := NewTransport(transportif.DefaultConfig(), serverID)
	defer serverTransport.Close()

	listener, _ := serverTransport.Listen(NewAddress("127.0.0.1", 0))
	defer func() { _ = listener.Close() }()

	actualAddr := listener.Addr().(*Address)

	// 启动服务端
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clientID, _ := mgr.Create()
		clientTransport, _ := NewTransport(transportif.DefaultConfig(), clientID)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		conn, err := clientTransport.Dial(ctx, actualAddr)
		cancel()

		if err == nil {
			conn.Close()
		}
		clientTransport.Close()
	}
}

// getFreePort 获取可用端口
func getFreePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

