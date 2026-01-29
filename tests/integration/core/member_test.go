//go:build integration

package core_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/dep2p/go-dep2p/tests/testutil"
)

// TestMember_List 测试成员列表查询
//
// 验证 Members() 返回正确的成员列表。
func TestMember_List(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 启动节点并加入 Realm
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()
	realmB := testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()

	// 2. 建立连接并等待认证
	nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	testutil.WaitForMembers(t, realmA, 2, 30*time.Second)

	// 3. 验证成员列表
	membersA := realmA.Members()
	membersB := realmB.Members()

	assert.Equal(t, 2, len(membersA), "Realm A 应有 2 个成员")
	assert.Equal(t, 2, len(membersB), "Realm B 应有 2 个成员")

	// 4. 验证包含自己和对方
	assert.Contains(t, membersA, nodeA.ID(), "A 应包含自己")
	assert.Contains(t, membersA, nodeB.ID(), "A 应包含 B")
	assert.Contains(t, membersB, nodeA.ID(), "B 应包含 A")
	assert.Contains(t, membersB, nodeB.ID(), "B 应包含自己")

	t.Log("✅ 成员列表测试通过")
}

// TestMember_IsMember 测试成员检查
//
// 验证 IsMember() 方法正确判断成员身份。
func TestMember_IsMember(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 启动节点并加入 Realm
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()
	_ = testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()

	// 2. 建立连接并等待认证
	nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	testutil.WaitForMembers(t, realmA, 2, 30*time.Second)

	// 3. 验证 IsMember (通过 Members() 间接验证)
	membersA := realmA.Members()
	assert.Contains(t, membersA, nodeA.ID(), "A 应该是成员")
	assert.Contains(t, membersA, nodeB.ID(), "B 应该是成员")

	t.Log("✅ 成员检查测试通过")
}

// TestMember_Leave 测试成员离开
//
// 验证节点离开 Realm 后，成员列表更新。
func TestMember_Leave(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK

	// 1. 启动 3 个节点
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()
	nodeB := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()
	nodeC := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()
	_ = testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()
	_ = testutil.NewTestRealm(t, nodeC).WithPSK(psk).Join()

	// 2. 建立连接
	nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	nodeC.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())

	// 3. 等待所有成员发现
	testutil.WaitForMembers(t, realmA, 3, 45*time.Second)

	initialCount := len(realmA.Members())
	assert.Equal(t, 3, initialCount, "初始应有 3 个成员")

	// 4. 关闭节点 C
	nodeC.Close()
	time.Sleep(3 * time.Second)

	// 5. 验证成员数减少
	testutil.Eventually(t, 10*time.Second, func() bool {
		return len(realmA.Members()) < initialCount
	}, "成员数应该减少")

	finalCount := len(realmA.Members())
	assert.Less(t, finalCount, initialCount, "成员数应该减少")
	assert.GreaterOrEqual(t, finalCount, 2, "至少应有 2 个成员 (A 和 B)")

	t.Logf("✅ 成员离开测试通过: 初始=%d, 最终=%d", initialCount, finalCount)
}
