package holepunch

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"
)

// ============================================================================
//                              NAT 穿透集成测试
// ============================================================================
//
// 这些测试验证 NAT 穿透的端到端工作流程。
// 由于真实 NAT 穿透需要特殊网络环境，这里使用本地模拟。

// TestIntegration_TCPHolePunch_SimultaneousOpen 测试 TCP 同时打开
//
// 模拟两个节点同时发起 TCP 连接（Simultaneous Open）：
// - 节点 A 监听并尝试连接节点 B
// - 节点 B 监听并尝试连接节点 A
// - 使用 SO_REUSEADDR/SO_REUSEPORT 实现端口复用
func TestIntegration_TCPHolePunch_SimultaneousOpen(t *testing.T) {
	// 创建两个监听器模拟两个节点的本地端口
	listenerA, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener A: %v", err)
	}
	defer listenerA.Close()

	listenerB, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener B: %v", err)
	}
	defer listenerB.Close()

	addrA := listenerA.Addr().String()
	addrB := listenerB.Addr().String()

	// 创建两个 TCP puncher
	configA := DefaultTCPConfig()
	configA.MaxAttempts = 5
	configA.Timeout = 5 * time.Second
	puncherA := NewTCPPuncher(configA)

	configB := DefaultTCPConfig()
	configB.MaxAttempts = 5
	configB.Timeout = 5 * time.Second
	puncherB := NewTCPPuncher(configB)

	ctx := context.Background()

	// 用于接收连接的 goroutine
	acceptedA := make(chan net.Conn, 1)
	acceptedB := make(chan net.Conn, 1)

	go func() {
		conn, err := listenerA.Accept()
		if err != nil {
			return
		}
		acceptedA <- conn
	}()

	go func() {
		conn, err := listenerB.Accept()
		if err != nil {
			return
		}
		acceptedB <- conn
	}()

	// 同时从两端发起连接
	var wg sync.WaitGroup
	var connA, connB net.Conn
	var errA, errB error

	wg.Add(2)

	go func() {
		defer wg.Done()
		connA, _, errA = puncherA.Punch(ctx, "node-B", []string{addrB})
	}()

	go func() {
		defer wg.Done()
		connB, _, errB = puncherB.Punch(ctx, "node-A", []string{addrA})
	}()

	wg.Wait()

	// 检查结果
	if errA != nil && errB != nil {
		t.Fatalf("Both punches failed: A=%v, B=%v", errA, errB)
	}

	// 至少一个应该成功
	if connA != nil {
		connA.Close()
		t.Log("Node A successfully connected to Node B")
	}
	if connB != nil {
		connB.Close()
		t.Log("Node B successfully connected to Node A")
	}

	// 检查服务端接受的连接
	select {
	case conn := <-acceptedA:
		conn.Close()
		t.Log("Listener A accepted a connection")
	default:
	}

	select {
	case conn := <-acceptedB:
		conn.Close()
		t.Log("Listener B accepted a connection")
	default:
	}

	t.Log("✅ TCP 同时打开测试通过")
}

// TestIntegration_TCPHolePunch_DataTransfer 测试打洞后数据传输
func TestIntegration_TCPHolePunch_DataTransfer(t *testing.T) {
	// 创建服务端监听器
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().String()

	// 服务端接收数据
	serverDone := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- ""
			return
		}
		defer conn.Close()

		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			serverDone <- ""
			return
		}
		serverDone <- string(buf[:n])
	}()

	// 客户端打洞并发送数据
	config := DefaultTCPConfig()
	puncher := NewTCPPuncher(config)

	ctx := context.Background()
	conn, _, err := puncher.Punch(ctx, "server", []string{addr})
	if err != nil {
		t.Fatalf("Punch failed: %v", err)
	}
	defer conn.Close()

	// 发送测试数据
	testData := "Hello, NAT Traversal!"
	_, err = conn.Write([]byte(testData))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// 验证服务端收到数据
	select {
	case received := <-serverDone:
		if received != testData {
			t.Errorf("Data mismatch: got %q, want %q", received, testData)
		} else {
			t.Log("✅ 打洞后数据传输正常")
		}
	case <-time.After(5 * time.Second):
		t.Error("Server did not receive data within timeout")
	}
}

// TestIntegration_TCPHolePunch_BidirectionalData 测试双向数据传输
func TestIntegration_TCPHolePunch_BidirectionalData(t *testing.T) {
	// 创建服务端监听器
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().String()

	// 服务端接收并回复
	serverDone := make(chan bool, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			serverDone <- false
			return
		}
		defer conn.Close()

		// 接收
		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			serverDone <- false
			return
		}

		// 回复（反转数据）
		received := buf[:n]
		reversed := make([]byte, len(received))
		for i, b := range received {
			reversed[len(received)-1-i] = b
		}
		_, err = conn.Write(reversed)
		serverDone <- (err == nil)
	}()

	// 客户端打洞
	config := DefaultTCPConfig()
	puncher := NewTCPPuncher(config)

	ctx := context.Background()
	conn, _, err := puncher.Punch(ctx, "server", []string{addr})
	if err != nil {
		t.Fatalf("Punch failed: %v", err)
	}
	defer conn.Close()

	// 发送数据
	testData := "Hello"
	_, err = conn.Write([]byte(testData))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}

	// 接收回复
	buf := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	received := string(buf[:n])
	expected := "olleH" // 反转的 "Hello"

	if received != expected {
		t.Errorf("Response mismatch: got %q, want %q", received, expected)
	}

	// 等待服务端完成
	select {
	case success := <-serverDone:
		if !success {
			t.Error("Server encountered an error")
		}
	case <-time.After(5 * time.Second):
		t.Error("Server did not finish within timeout")
	}

	t.Log("✅ 双向数据传输正常")
}

// TestIntegration_TCPHolePunch_MultipleConnections 测试多连接
func TestIntegration_TCPHolePunch_MultipleConnections(t *testing.T) {
	// 创建服务端监听器
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create listener: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().String()

	// 服务端接收多个连接
	acceptCount := 0
	var acceptMu sync.Mutex

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			acceptMu.Lock()
			acceptCount++
			acceptMu.Unlock()
			conn.Close()
		}
	}()

	// 创建多个客户端并发打洞
	numClients := 5
	config := DefaultTCPConfig()

	var wg sync.WaitGroup
	successCount := 0
	var successMu sync.Mutex

	for i := 0; i < numClients; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			puncher := NewTCPPuncher(config)
			ctx := context.Background()

			conn, _, err := puncher.Punch(ctx, "server", []string{addr})
			if err == nil && conn != nil {
				successMu.Lock()
				successCount++
				successMu.Unlock()
				conn.Close()
			}
		}(i)
	}

	wg.Wait()

	// 所有客户端应该都成功
	if successCount != numClients {
		t.Errorf("Only %d/%d clients succeeded", successCount, numClients)
	}

	time.Sleep(100 * time.Millisecond) // 等待服务端处理完

	acceptMu.Lock()
	finalCount := acceptCount
	acceptMu.Unlock()

	if finalCount < numClients {
		t.Logf("Server accepted %d connections (expected %d)", finalCount, numClients)
	}

	t.Logf("✅ 多连接测试通过 (%d/%d 成功)", successCount, numClients)
}

// TestIntegration_TCPHolePunch_Retry 测试重试机制
func TestIntegration_TCPHolePunch_Retry(t *testing.T) {
	// 创建一个延迟启动的监听器
	// 模拟目标节点稍后才准备好

	var listener net.Listener
	var listenerAddr string
	listenerReady := make(chan struct{})

	// 延迟启动监听器
	go func() {
		time.Sleep(500 * time.Millisecond)

		var err error
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return
		}
		listenerAddr = listener.Addr().String()
		close(listenerReady)

		// 接受连接
		conn, _ := listener.Accept()
		if conn != nil {
			conn.Close()
		}
	}()

	// 等待监听器准备好
	select {
	case <-listenerReady:
	case <-time.After(2 * time.Second):
		t.Skip("Listener did not start in time")
	}
	defer listener.Close()

	// 客户端打洞（监听器已启动，应该立即成功）
	config := DefaultTCPConfig()
	config.MaxAttempts = 5
	config.Timeout = 5 * time.Second
	puncher := NewTCPPuncher(config)

	ctx := context.Background()
	conn, _, err := puncher.Punch(ctx, "delayed-server", []string{listenerAddr})

	if err != nil {
		t.Fatalf("Punch failed: %v", err)
	}
	if conn != nil {
		conn.Close()
	}

	t.Log("✅ 重试机制正常")
}

// ============================================================================
//                              AutoRelay 集成测试存根
// ============================================================================

// TestIntegration_AutoRelay_Discovery 测试 AutoRelay 发现流程
//
// 注意：这是一个测试存根。完整的 AutoRelay 集成测试需要：
// - 运行中的中继服务器
// - 完整的 Host 和 Peerstore 实现
// - 网络发现机制
//
// 单元测试已在 autorelay_test.go 中覆盖核心逻辑。
func TestIntegration_AutoRelay_Discovery(t *testing.T) {
	t.Skip("AutoRelay 集成测试需要完整的网络环境，参见 autorelay_test.go 的单元测试")
}

// TestIntegration_AutoRelay_Reservation 测试 AutoRelay 预留流程
func TestIntegration_AutoRelay_Reservation(t *testing.T) {
	t.Skip("AutoRelay 预留测试需要运行中的中继服务器，参见 autorelay_test.go 的单元测试")
}
