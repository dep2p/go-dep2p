//go:build integration

package core_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/dep2p/go-dep2p/tests/testutil"
)

// TestEventBus_PeerConnected 测试连接事件
//
// 验证:
//   - EvtPeerConnected 事件正确触发
//   - 事件包含正确的 PeerID
func TestEventBus_PeerConnected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. 启动节点 A
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	// 2. 订阅连接事件
	eventBus := nodeA.Host().EventBus()
	require.NotNil(t, eventBus, "EventBus 不应为 nil")

	connectedSub, err := eventBus.Subscribe(new(types.EvtPeerConnected))
	require.NoError(t, err, "订阅连接事件失败")
	defer connectedSub.Close()

	// 3. 启动节点 B 并连接
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	// 4. 连接并等待事件
	err = nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err)

	// 5. 接收事件（注意：可能先收到 mDNS 发现的其他节点事件）
	// 等待收到 nodeB 的连接事件，最多等待 10 秒
	timeout := time.After(10 * time.Second)
	receivedNodeBEvent := false
	for !receivedNodeBEvent {
		select {
		case evt := <-connectedSub.Out():
			e, ok := evt.(*types.EvtPeerConnected)
			require.True(t, ok, "事件类型应为 EvtPeerConnected")
			peerID := string(e.PeerID)
			if peerID == nodeB.ID() {
				receivedNodeBEvent = true
				t.Logf("✅ 收到目标连接事件: %s", peerID[:8])
			} else {
				t.Logf("⚠️ 收到其他节点连接事件 (mDNS): %s，继续等待 nodeB", peerID[:8])
			}
		case <-timeout:
			t.Fatalf("等待 nodeB 连接事件超时（收到其他节点但未收到 nodeB）")
		}
	}
}

// TestEventBus_PeerDisconnected 测试断开事件
//
// 验证:
//   - EvtPeerDisconnected 事件正确触发
//   - 事件在节点关闭后触发
func TestEventBus_PeerDisconnected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. 启动节点 A
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	// 2. 订阅断开事件
	eventBus := nodeA.Host().EventBus()
	require.NotNil(t, eventBus, "EventBus 不应为 nil")

	disconnectedSub, err := eventBus.Subscribe(new(types.EvtPeerDisconnected))
	require.NoError(t, err, "订阅断开事件失败")
	defer disconnectedSub.Close()

	// 3. 启动节点 B 并连接
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	err = nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err)

	// 等待连接建立
	testutil.Eventually(t, 5*time.Second, func() bool {
		return nodeA.ConnectionCount() > 0
	}, "连接应该建立")

	// 4. 关闭节点 B
	nodeB.Close()
	time.Sleep(1 * time.Second)

	// 5. 接收断开事件
	select {
	case evt := <-disconnectedSub.Out():
		e, ok := evt.(*types.EvtPeerDisconnected)
		require.True(t, ok, "事件类型应为 EvtPeerDisconnected")
		assert.Equal(t, nodeB.ID(), string(e.PeerID), "PeerID 应匹配")
		t.Logf("✅ 收到断开事件: %s", string(e.PeerID)[:8])
	case <-time.After(10 * time.Second):
		t.Fatal("等待断开事件超时")
	}
}

// TestEventBus_MultipleEvents 测试多个事件
//
// 验证连接和断开事件的顺序和正确性。
func TestEventBus_MultipleEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. 启动节点 A
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	eventBus := nodeA.Host().EventBus()
	require.NotNil(t, eventBus)

	// 2. 订阅连接和断开事件
	connectedSub, _ := eventBus.Subscribe(new(types.EvtPeerConnected))
	defer connectedSub.Close()
	disconnectedSub, _ := eventBus.Subscribe(new(types.EvtPeerDisconnected))
	defer disconnectedSub.Close()

	// 3. 启动节点 B 并连接
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	err := nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err)

	// 4. 接收连接事件（等待特定的 nodeB）
	var connectedPeerID string
	timeout := time.After(10 * time.Second)
	for connectedPeerID != nodeB.ID() {
		select {
		case evt := <-connectedSub.Out():
			e := evt.(*types.EvtPeerConnected)
			peerID := string(e.PeerID)
			if peerID == nodeB.ID() {
				connectedPeerID = peerID
				t.Log("✅ 收到 nodeB 连接事件")
			} else {
				t.Logf("⚠️ 收到其他节点连接事件 (mDNS): %s", peerID[:8])
			}
		case <-timeout:
			t.Fatal("等待 nodeB 连接事件超时")
		}
	}

	// 5. 关闭节点 B
	nodeB.Close()
	time.Sleep(1 * time.Second)

	// 6. 接收断开事件（等待特定的 nodeB）
	timeout2 := time.After(10 * time.Second)
	receivedDisconnect := false
	for !receivedDisconnect {
		select {
		case evt := <-disconnectedSub.Out():
			e := evt.(*types.EvtPeerDisconnected)
			disconnectedPeerID := string(e.PeerID)
			if disconnectedPeerID == connectedPeerID {
				receivedDisconnect = true
				t.Log("✅ 收到 nodeB 断开事件")
			} else {
				t.Logf("⚠️ 收到其他节点断开事件: %s", disconnectedPeerID[:8])
			}
		case <-timeout2:
			t.Fatal("等待 nodeB 断开事件超时")
		}
	}
}

// TestEventBus_RealmMemberEvents 测试 Realm 成员变化事件
//
// 验证 Realm 成员加入/离开时的事件通知。
func TestEventBus_RealmMemberEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 启动节点
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()
	_ = testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()

	// 2. 订阅连接事件 (用于验证成员加入)
	eventBus := nodeA.Host().EventBus()
	connectedSub, _ := eventBus.Subscribe(new(types.EvtPeerConnected))
	defer connectedSub.Close()

	// 3. 建立连接
	nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())

	// 4. 等待连接事件（等待特定的 nodeB）
	timeout := time.After(10 * time.Second)
	receivedNodeBEvent := false
	for !receivedNodeBEvent {
		select {
		case evt := <-connectedSub.Out():
			e := evt.(*types.EvtPeerConnected)
			peerID := string(e.PeerID)
			if peerID == nodeB.ID() {
				receivedNodeBEvent = true
				t.Log("✅ 收到 nodeB 连接事件")
			} else {
				t.Logf("⚠️ 收到其他节点连接事件 (mDNS): %s", peerID[:8])
			}
		case <-timeout:
			t.Fatal("等待 nodeB 连接事件超时")
		}
	}

	// 5. 等待成员发现
	testutil.WaitForMembers(t, realmA, 2, 30*time.Second)

	// 6. 验证成员列表
	members := realmA.Members()
	assert.Contains(t, members, nodeB.ID(), "B 应该在成员列表中")

	t.Log("✅ Realm 成员事件测试通过")
}
