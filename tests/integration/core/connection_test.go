//go:build integration

package core_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/dep2p/go-dep2p/tests/testutil"
)

// TestConnection_DirectConnect 测试两节点直连
//
// 验证:
//   - 节点能够启动并监听
//   - 节点 B 能连接到节点 A
//   - ConnectionCount() 正确反映连接数
//   - 双方都能看到连接
func TestConnection_DirectConnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. 启动两个节点
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	t.Logf("节点 A: %s, 监听: %v", nodeA.ID()[:8], nodeA.ListenAddrs())
	t.Logf("节点 B: %s, 监听: %v", nodeB.ID()[:8], nodeB.ListenAddrs())

	// 记录连接前的连接数
	countABefore := nodeA.ConnectionCount()
	countBBefore := nodeB.ConnectionCount()

	// 2. B 连接到 A
	err := nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err, "节点 B 连接到节点 A 失败")

	// 3. 验证连接建立
	// 方法1: 检查 Peerstore 中对方地址已添加（表示连接成功）
	testutil.Eventually(t, 5*time.Second, func() bool {
		// A 的 Peerstore 应该有 B 的地址
		addrsOfBinA := nodeA.Host().Peerstore().Addrs(types.PeerID(nodeB.ID()))
		// B 的 Peerstore 应该有 A 的地址
		addrsOfAinB := nodeB.Host().Peerstore().Addrs(types.PeerID(nodeA.ID()))
		return len(addrsOfBinA) > 0 && len(addrsOfAinB) > 0
	}, "A 和 B 应该互相知道对方地址")

	// 方法2: 验证连接数增加
	countAAfter := nodeA.ConnectionCount()
	countBAfter := nodeB.ConnectionCount()
	assert.Greater(t, countAAfter, countABefore, "节点 A 连接数应增加")
	assert.Greater(t, countBAfter, countBBefore, "节点 B 连接数应增加")

	t.Logf("✅ 连接测试通过: A 连接数 %d→%d, B 连接数 %d→%d",
		countABefore, countAAfter, countBBefore, countBAfter)
}

// TestConnection_Disconnect 测试连接断开
//
// 验证:
//   - 节点关闭后连接数减少
//   - 对方节点能感知到断开
func TestConnection_Disconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. 启动并连接
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	err := nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err)

	// 等待连接建立
	testutil.Eventually(t, 5*time.Second, func() bool {
		addrsOfBinA := nodeA.Host().Peerstore().Addrs(types.PeerID(nodeB.ID()))
		return len(addrsOfBinA) > 0
	}, "A 应该知道 B 的地址")

	// 记录 A 的连接数
	initialCountA := nodeA.ConnectionCount()
	assert.Greater(t, initialCountA, 0, "节点 A 应有连接")

	// 2. 关闭节点 B
	nodeBID := nodeB.ID() // 保存 ID 用于后续验证
	nodeB.Close()
	time.Sleep(2 * time.Second)

	// 3. 验证节点 A 的连接数减少
	testutil.Eventually(t, 10*time.Second, func() bool {
		return nodeA.ConnectionCount() < initialCountA
	}, "节点 A 的连接数应该减少")

	t.Logf("✅ 断开测试通过: 节点 B (%s) 已断开", nodeBID[:8])

	t.Log("✅ 断开测试通过")
}

// TestConnection_Reconnect 测试重连
//
// 验证节点断开后能够重新连接。
func TestConnection_Reconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. 启动节点 A
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	// 2. 启动节点 B 并连接
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	err := nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err)

	testutil.Eventually(t, 5*time.Second, func() bool {
		addrsOfBinA := nodeA.Host().Peerstore().Addrs(types.PeerID(nodeB.ID()))
		return len(addrsOfBinA) > 0
	}, "A 应该知道 B 的地址")

	initialCountA := nodeA.ConnectionCount()

	// 3. 关闭节点 B
	nodeB.Close()
	time.Sleep(2 * time.Second)

	// 验证连接数减少
	testutil.Eventually(t, 5*time.Second, func() bool {
		return nodeA.ConnectionCount() < initialCountA
	}, "节点 A 连接数应减少")

	// 4. 重新启动节点 B（新身份）
	nodeB2 := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	// 5. 重新连接
	err = nodeB2.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	require.NoError(t, err, "重连失败")

	testutil.Eventually(t, 5*time.Second, func() bool {
		addrsOfB2inA := nodeA.Host().Peerstore().Addrs(types.PeerID(nodeB2.ID()))
		addrsOfAinB2 := nodeB2.Host().Peerstore().Addrs(types.PeerID(nodeA.ID()))
		return len(addrsOfB2inA) > 0 && len(addrsOfAinB2) > 0
	}, "A 和 B2 应该互相知道对方地址")

	t.Log("✅ 重连测试通过")
}

// TestConnection_MultiNode 测试多节点连接
//
// 验证一个节点可以同时连接多个节点。
func TestConnection_MultiNode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// 1. 启动中心节点
	center := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	// 2. 启动多个节点并连接到中心节点
	nodeCount := 3
	nodes := make([]*dep2p.Node, nodeCount)
	for i := range nodes {
		nodes[i] = testutil.NewTestNode(t).
			WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
			Start()

		err := nodes[i].Host().Connect(ctx, center.ID(), center.ListenAddrs())
		require.NoError(t, err, "节点 %d 连接失败", i)
	}

	// 3. 等待所有连接建立（验证 Peerstore 中有对方地址）
	testutil.Eventually(t, 10*time.Second, func() bool {
		knownCount := 0
		for _, node := range nodes {
			addrs := center.Host().Peerstore().Addrs(types.PeerID(node.ID()))
			if len(addrs) > 0 {
				knownCount++
			}
		}
		return knownCount >= nodeCount
	}, "中心节点应该知道所有节点的地址")

	// 验证双向地址发现
	for i, node := range nodes {
		addrsOfCenterInNode := node.Host().Peerstore().Addrs(types.PeerID(center.ID()))
		addrsOfNodeInCenter := center.Host().Peerstore().Addrs(types.PeerID(node.ID()))
		t.Logf("节点 %d (%s): 知道中心地址=%d, 中心知道节点地址=%d",
			i, node.ID()[:8], len(addrsOfCenterInNode), len(addrsOfNodeInCenter))
		assert.Greater(t, len(addrsOfCenterInNode), 0, "节点 %d 应知道中心地址", i)
		assert.Greater(t, len(addrsOfNodeInCenter), 0, "中心应知道节点 %d 地址", i)
	}

	t.Logf("✅ 多节点连接测试通过: 中心节点总连接数=%d, 预期节点数=%d",
		center.ConnectionCount(), nodeCount)
}
