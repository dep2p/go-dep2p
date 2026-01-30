package tests

// ════════════════════════════════════════════════════════════════════════════
//                      连接事件 API 测试 (v1.1)
// 来源: design/_discussions/20260129-wes-gap-enhancement.md TASK-001
// ════════════════════════════════════════════════════════════════════════════

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p"
)

// TestOnPeerConnected 测试节点连接回调
func TestOnPeerConnected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 创建节点 A
	nodeA, err := dep2p.New(ctx,
		dep2p.WithListenPort(0), // 随机端口
	)
	if err != nil {
		t.Fatalf("创建节点 A 失败: %v", err)
	}
	defer nodeA.Close()

	// 启动节点 A
	if err := nodeA.Start(ctx); err != nil {
		t.Fatalf("启动节点 A 失败: %v", err)
	}

	// 创建节点 B
	nodeB, err := dep2p.New(ctx,
		dep2p.WithListenPort(0),
	)
	if err != nil {
		t.Fatalf("创建节点 B 失败: %v", err)
	}
	defer nodeB.Close()

	// 启动节点 B
	if err := nodeB.Start(ctx); err != nil {
		t.Fatalf("启动节点 B 失败: %v", err)
	}

	// 注册连接回调
	var wg sync.WaitGroup
	var connectedEvent dep2p.PeerConnectedEvent
	var eventReceived bool

	wg.Add(1)
	nodeA.OnPeerConnected(func(event dep2p.PeerConnectedEvent) {
		connectedEvent = event
		eventReceived = true
		wg.Done()
	})

	// 节点 B 连接到节点 A
	addrs := nodeA.ListenAddrs()
	if len(addrs) == 0 {
		t.Fatal("节点 A 没有监听地址")
	}

	// 构建完整地址
	fullAddr := addrs[0] + "/p2p/" + nodeA.ID()
	if err := nodeB.Connect(ctx, fullAddr); err != nil {
		t.Fatalf("连接失败: %v", err)
	}

	// 等待回调被触发（最多 5 秒）
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 回调已触发
	case <-time.After(5 * time.Second):
		t.Fatal("连接回调未在 5 秒内触发")
	}

	// 验证事件内容
	if !eventReceived {
		t.Fatal("未收到连接事件")
	}

	if connectedEvent.PeerID != nodeB.ID() {
		t.Errorf("PeerID 不匹配: 期望 %s, 实际 %s", nodeB.ID(), connectedEvent.PeerID)
	}

	if connectedEvent.Direction != "inbound" {
		t.Errorf("Direction 不匹配: 期望 inbound, 实际 %s", connectedEvent.Direction)
	}

	if connectedEvent.Timestamp.IsZero() {
		t.Error("Timestamp 不应为零值")
	}

	t.Logf("连接事件: PeerID=%s, Direction=%s, NumConns=%d, Addrs=%v",
		connectedEvent.PeerID[:8], connectedEvent.Direction,
		connectedEvent.NumConns, connectedEvent.Addrs)
}

// TestOnPeerDisconnected 测试节点断开回调
func TestOnPeerDisconnected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 创建并启动节点 A
	nodeA, err := dep2p.New(ctx,
		dep2p.WithListenPort(0),
	)
	if err != nil {
		t.Fatalf("创建节点 A 失败: %v", err)
	}
	defer nodeA.Close()

	if err := nodeA.Start(ctx); err != nil {
		t.Fatalf("启动节点 A 失败: %v", err)
	}

	// 创建并启动节点 B
	nodeB, err := dep2p.New(ctx,
		dep2p.WithListenPort(0),
	)
	if err != nil {
		t.Fatalf("创建节点 B 失败: %v", err)
	}

	if err := nodeB.Start(ctx); err != nil {
		t.Fatalf("启动节点 B 失败: %v", err)
	}

	// 注册断开回调
	var wg sync.WaitGroup
	var disconnectedEvent dep2p.PeerDisconnectedEvent
	var eventReceived bool

	wg.Add(1)
	nodeA.OnPeerDisconnected(func(event dep2p.PeerDisconnectedEvent) {
		disconnectedEvent = event
		eventReceived = true
		wg.Done()
	})

	// 连接节点
	addrs := nodeA.ListenAddrs()
	if len(addrs) == 0 {
		t.Fatal("节点 A 没有监听地址")
	}

	fullAddr := addrs[0] + "/p2p/" + nodeA.ID()
	if err := nodeB.Connect(ctx, fullAddr); err != nil {
		t.Fatalf("连接失败: %v", err)
	}

	// 等待连接建立
	time.Sleep(500 * time.Millisecond)

	// 关闭节点 B 触发断开
	nodeBID := nodeB.ID()
	nodeB.Close()

	// 等待回调被触发（最多 10 秒，断开检测可能需要更长时间）
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 回调已触发
	case <-time.After(10 * time.Second):
		t.Fatal("断开回调未在 10 秒内触发")
	}

	// 验证事件内容
	if !eventReceived {
		t.Fatal("未收到断开事件")
	}

	if disconnectedEvent.PeerID != nodeBID {
		t.Errorf("PeerID 不匹配: 期望 %s, 实际 %s", nodeBID, disconnectedEvent.PeerID)
	}

	if disconnectedEvent.Timestamp.IsZero() {
		t.Error("Timestamp 不应为零值")
	}

	t.Logf("断开事件: PeerID=%s, Reason=%s, NumConns=%d",
		disconnectedEvent.PeerID[:8], disconnectedEvent.Reason,
		disconnectedEvent.NumConns)
}

// TestMultipleCallbacks 测试多个回调
func TestMultipleCallbacks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 创建并启动节点
	node, err := dep2p.New(ctx,
		dep2p.WithListenPort(0),
	)
	if err != nil {
		t.Fatalf("创建节点失败: %v", err)
	}
	defer node.Close()

	if err := node.Start(ctx); err != nil {
		t.Fatalf("启动节点失败: %v", err)
	}

	// 注册多个回调
	var callCount int
	var mu sync.Mutex

	for i := 0; i < 3; i++ {
		node.OnPeerConnected(func(event dep2p.PeerConnectedEvent) {
			mu.Lock()
			callCount++
			mu.Unlock()
		})
	}

	// 创建并连接节点 B
	nodeB, err := dep2p.New(ctx,
		dep2p.WithListenPort(0),
	)
	if err != nil {
		t.Fatalf("创建节点 B 失败: %v", err)
	}
	defer nodeB.Close()

	if err := nodeB.Start(ctx); err != nil {
		t.Fatalf("启动节点 B 失败: %v", err)
	}

	addrs := node.ListenAddrs()
	if len(addrs) == 0 {
		t.Fatal("节点没有监听地址")
	}

	fullAddr := addrs[0] + "/p2p/" + node.ID()
	if err := nodeB.Connect(ctx, fullAddr); err != nil {
		t.Fatalf("连接失败: %v", err)
	}

	// 等待回调执行
	time.Sleep(2 * time.Second)

	// 验证所有回调都被调用
	mu.Lock()
	if callCount != 3 {
		t.Errorf("回调调用次数不匹配: 期望 3, 实际 %d", callCount)
	}
	mu.Unlock()

	t.Logf("多个回调测试通过，调用次数: %d", callCount)
}

// TestCallbackNonBlocking 测试回调不阻塞网络层
func TestCallbackNonBlocking(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 创建并启动节点
	node, err := dep2p.New(ctx,
		dep2p.WithListenPort(0),
	)
	if err != nil {
		t.Fatalf("创建节点失败: %v", err)
	}
	defer node.Close()

	if err := node.Start(ctx); err != nil {
		t.Fatalf("启动节点失败: %v", err)
	}

	// 注册一个慢回调（模拟阻塞）
	node.OnPeerConnected(func(event dep2p.PeerConnectedEvent) {
		time.Sleep(5 * time.Second) // 模拟慢处理
	})

	// 创建并连接节点 B
	nodeB, err := dep2p.New(ctx,
		dep2p.WithListenPort(0),
	)
	if err != nil {
		t.Fatalf("创建节点 B 失败: %v", err)
	}
	defer nodeB.Close()

	if err := nodeB.Start(ctx); err != nil {
		t.Fatalf("启动节点 B 失败: %v", err)
	}

	addrs := node.ListenAddrs()
	if len(addrs) == 0 {
		t.Fatal("节点没有监听地址")
	}

	// 连接应该快速完成，不被回调阻塞
	start := time.Now()
	fullAddr := addrs[0] + "/p2p/" + node.ID()
	if err := nodeB.Connect(ctx, fullAddr); err != nil {
		t.Fatalf("连接失败: %v", err)
	}
	elapsed := time.Since(start)

	// 连接应该在 2 秒内完成（远小于回调的 5 秒延迟）
	if elapsed > 2*time.Second {
		t.Errorf("连接被回调阻塞: 耗时 %v", elapsed)
	}

	t.Logf("非阻塞测试通过，连接耗时: %v", elapsed)
}
