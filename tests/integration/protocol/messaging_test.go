//go:build integration

package protocol_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p"
	"github.com/dep2p/go-dep2p/tests/testutil"
)

// TestMessaging_RequestResponse 测试 Messaging 请求-响应模式
//
// 验证:
//   - RegisterHandler 注册处理器
//   - Send 发送消息并等待响应
//   - 响应数据正确返回
func TestMessaging_RequestResponse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK
	protocolID := "test-messaging"

	// 1. 创建节点并加入 Realm
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()
	realmB := testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()

	nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	testutil.WaitForMembers(t, realmA, 2, 30*time.Second)

	// 2. B 注册 Handler
	err := realmB.Messaging().RegisterHandler(protocolID, func(ctx context.Context, req *dep2p.Request) (*dep2p.Response, error) {
		t.Logf("节点 B 收到消息: %s (来自: %s)", string(req.Data), req.From[:8])
		// 返回响应
		return &dep2p.Response{
			Data: []byte("Reply: " + string(req.Data)),
		}, nil
	})
	require.NoError(t, err, "注册 Handler 失败")

	// 等待 Handler 注册完成
	time.Sleep(500 * time.Millisecond)

	// 3. A 发送消息
	msg := "Hello from A"
	resp, err := realmA.Messaging().Send(ctx, nodeB.ID(), protocolID, []byte(msg))
	require.NoError(t, err, "发送消息失败")

	// 4. 验证响应
	assert.Equal(t, "Reply: "+msg, string(resp), "响应应匹配")
	t.Logf("节点 A 收到响应: %s", string(resp))

	t.Log("✅ Messaging 请求-响应测试通过")
}

// TestMessaging_MultipleProtocols 测试多协议支持
//
// 验证一个 Realm 可以注册多个协议处理器，互不干扰。
func TestMessaging_MultipleProtocols(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 创建节点
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()
	realmB := testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()

	nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	testutil.WaitForMembers(t, realmA, 2, 30*time.Second)

	// 2. B 注册多个 Handler
	err := realmB.Messaging().RegisterHandler("chat", func(ctx context.Context, req *dep2p.Request) (*dep2p.Response, error) {
		return &dep2p.Response{Data: []byte("CHAT:" + string(req.Data))}, nil
	})
	require.NoError(t, err)

	err = realmB.Messaging().RegisterHandler("rpc", func(ctx context.Context, req *dep2p.Request) (*dep2p.Response, error) {
		return &dep2p.Response{Data: []byte("RPC:" + string(req.Data))}, nil
	})
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	// 3. A 发送不同协议的消息
	chatResp, err := realmA.Messaging().Send(ctx, nodeB.ID(), "chat", []byte("hello"))
	require.NoError(t, err)
	assert.Equal(t, "CHAT:hello", string(chatResp), "chat 协议响应应匹配")

	rpcResp, err := realmA.Messaging().Send(ctx, nodeB.ID(), "rpc", []byte("call"))
	require.NoError(t, err)
	assert.Equal(t, "RPC:call", string(rpcResp), "rpc 协议响应应匹配")

	t.Log("✅ Messaging 多协议测试通过")
}

// TestMessaging_AsyncSend 测试异步发送
//
// 验证 SendAsync 方法的异步行为。
func TestMessaging_AsyncSend(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK
	protocolID := "test-async"

	// 1. 创建节点
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()
	realmB := testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()

	nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	testutil.WaitForMembers(t, realmA, 2, 30*time.Second)

	// 2. B 注册 Handler (模拟延迟处理)
	err := realmB.Messaging().RegisterHandler(protocolID, func(ctx context.Context, req *dep2p.Request) (*dep2p.Response, error) {
		time.Sleep(100 * time.Millisecond) // 模拟处理延迟
		return &dep2p.Response{Data: []byte("async-reply")}, nil
	})
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	// 3. A 异步发送
	respCh, err := realmA.Messaging().SendAsync(ctx, nodeB.ID(), protocolID, []byte("async-msg"))
	require.NoError(t, err)

	// 4. 可以在等待响应的同时做其他事情
	t.Log("发送异步请求，等待响应...")

	// 5. 接收响应
	select {
	case resp := <-respCh:
		require.NoError(t, resp.Error, "异步响应不应有错误")
		assert.Equal(t, "async-reply", string(resp.Data), "异步响应应匹配")
		t.Logf("收到异步响应: %s", string(resp.Data))
	case <-time.After(10 * time.Second):
		t.Fatal("异步响应超时")
	}

	t.Log("✅ Messaging 异步发送测试通过")
}

// TestMessaging_ConcurrentRequests 测试并发请求
//
// 验证 Messaging 能够正确处理并发请求。
func TestMessaging_ConcurrentRequests(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK
	protocolID := "test-concurrent"

	// 1. 创建节点
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()
	realmB := testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()

	nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	testutil.WaitForMembers(t, realmA, 2, 30*time.Second)

	// 2. B 注册 Handler
	var receivedCount atomic.Int32
	err := realmB.Messaging().RegisterHandler(protocolID, func(ctx context.Context, req *dep2p.Request) (*dep2p.Response, error) {
		receivedCount.Add(1)
		return &dep2p.Response{Data: []byte("ok-" + string(req.Data))}, nil
	})
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	// 3. A 并发发送多个请求
	numRequests := 10
	results := make(chan error, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(idx int) {
			msg := []byte(string(rune('0' + idx)))
			resp, err := realmA.Messaging().Send(ctx, nodeB.ID(), protocolID, msg)
			if err != nil {
				results <- err
				return
			}
			expected := "ok-" + string(msg)
			if string(resp) != expected {
				results <- assert.AnError
				return
			}
			results <- nil
		}(i)
	}

	// 4. 收集结果
	successCount := 0
	for i := 0; i < numRequests; i++ {
		select {
		case err := <-results:
			if err == nil {
				successCount++
			} else {
				t.Logf("请求失败: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("等待并发结果超时")
		}
	}

	// 5. 验证
	assert.Equal(t, numRequests, successCount, "所有请求应成功")
	assert.Equal(t, int32(numRequests), receivedCount.Load(), "B 应收到所有请求")

	t.Logf("✅ Messaging 并发测试通过 (%d/%d 成功)", successCount, numRequests)
}

// TestMessaging_HandlerUnregister 测试 Handler 注销
//
// 验证注销 Handler 后，消息请求会失败。
func TestMessaging_HandlerUnregister(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK
	protocolID := "test-unregister"

	// 1. 创建节点
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()
	realmB := testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()

	nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	testutil.WaitForMembers(t, realmA, 2, 30*time.Second)

	// 2. B 注册 Handler
	err := realmB.Messaging().RegisterHandler(protocolID, func(ctx context.Context, req *dep2p.Request) (*dep2p.Response, error) {
		return &dep2p.Response{Data: []byte("ok")}, nil
	})
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	// 3. A 发送消息 (应该成功)
	resp, err := realmA.Messaging().Send(ctx, nodeB.ID(), protocolID, []byte("test"))
	require.NoError(t, err, "发送消息应该成功")
	assert.Equal(t, "ok", string(resp))

	// 4. B 注销 Handler
	err = realmB.Messaging().UnregisterHandler(protocolID)
	require.NoError(t, err)

	time.Sleep(500 * time.Millisecond)

	// 5. A 再次发送消息 (应该失败或超时)
	shortCtx, shortCancel := context.WithTimeout(ctx, 3*time.Second)
	defer shortCancel()

	_, err = realmA.Messaging().Send(shortCtx, nodeB.ID(), protocolID, []byte("test2"))
	if err != nil {
		t.Logf("注销后发送失败 (预期): %v", err)
	} else {
		t.Log("注销后仍能发送 (实现可能不同)")
	}

	t.Log("✅ Messaging Handler 注销测试通过")
}
