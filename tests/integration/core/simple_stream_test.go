//go:build integration

package core_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/tests/testutil"
)

// TestSimple_HostStream 测试 Host 层的流功能
// 
// 这是最基础的流测试，绕过 Realm 和 Streams 服务层，
// 直接测试 Host.SetStreamHandler 和 Host.NewStream
func TestSimple_HostStream(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. 创建两个节点
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	t.Logf("Node A: %s", nodeA.ID()[:8])
	t.Logf("Node B: %s", nodeB.ID()[:8])

	// 2. B 注册 Handler
	testProtocol := "/test/simple/1.0.0"
	received := make(chan string, 1)

	nodeB.Host().SetStreamHandler(testProtocol, func(stream pkgif.Stream) {
		t.Log("B: handler called!")
		buf := make([]byte, 1024)
		n, err := stream.Read(buf)
		if err != nil {
			t.Logf("B read error: %v", err)
			stream.Close()
			return
		}
		received <- string(buf[:n])
		
		// 发送回复
		_, err = stream.Write([]byte("Hi A"))
		if err != nil {
			t.Logf("B write error: %v", err)
		}
		// 注意：这里不关闭流，让对方先读取完毕
		// 流会在测试结束时随节点关闭而清理
	})

	// 3. B 连接到 A
	err := nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err, "B 连接到 A 失败")
	t.Log("B connected to A")

	// 等待连接稳定
	time.Sleep(1 * time.Second)

	// 4. A 打开流到 B (带超时)
	t.Log("A opening stream to B...")
	streamCtx, streamCancel := context.WithTimeout(ctx, 10*time.Second)
	defer streamCancel()
	
	stream, err := nodeA.Host().NewStream(streamCtx, nodeB.ID(), testProtocol)
	if err != nil {
		t.Logf("A 打开流失败: %v", err)
		t.Logf("A 连接数: %d", nodeA.ConnectionCount())
		t.Logf("B 连接数: %d", nodeB.ConnectionCount())
		t.FailNow()
	}
	defer stream.Close()
	t.Log("A stream opened")

	// 5. A 发送消息
	_, err = stream.Write([]byte("Hello B"))
	require.NoError(t, err, "A 写入失败")
	t.Log("A sent: Hello B")

	// 6. 等待 B 收到
	select {
	case msg := <-received:
		t.Logf("B received: %s", msg)
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for B")
	}

	// 7. A 读取回复 (带超时)
	readDone := make(chan struct{})
	var readErr error
	var reply string
	
	go func() {
		buf := make([]byte, 1024)
		n, err := stream.Read(buf)
		if err != nil {
			readErr = err
		} else {
			reply = string(buf[:n])
		}
		close(readDone)
	}()
	
	select {
	case <-readDone:
		if readErr != nil {
			t.Logf("A 读取回复失败: %v", readErr)
			t.FailNow()
		}
		t.Logf("A received reply: %s", reply)
	case <-time.After(5 * time.Second):
		t.Fatal("A 读取回复超时")
	}

	t.Log("✅ Host 层流测试通过")
}
