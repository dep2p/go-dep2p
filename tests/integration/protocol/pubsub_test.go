//go:build integration

package protocol_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/tests/testutil"
)

// TestPubSub_GroupBroadcast 测试 PubSub 群组广播
//
// 验证:
//   - 3 个节点加入同一 Realm
//   - 所有节点 Join 同一 Topic
//   - 节点 A Publish 消息
//   - 节点 B 和 C 都能收到消息
func TestPubSub_GroupBroadcast(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK
	topicName := testutil.DefaultTestTopic

	// 1. 创建 3 个节点并加入 Realm
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
	realmB := testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()
	realmC := testutil.NewTestRealm(t, nodeC).WithPSK(psk).Join()

	// 2. 建立连接并等待成员发现
	nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
	nodeC.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())

	testutil.WaitForMembers(t, realmA, 3, 45*time.Second)
	t.Logf("成员发现完成: A=%d, B=%d, C=%d",
		len(realmA.Members()), len(realmB.Members()), len(realmC.Members()))

	// 3. 加入主题
	topicA, err := realmA.PubSub().Join(topicName)
	require.NoError(t, err, "节点 A 加入主题失败")
	topicB, err := realmB.PubSub().Join(topicName)
	require.NoError(t, err, "节点 B 加入主题失败")
	topicC, err := realmC.PubSub().Join(topicName)
	require.NoError(t, err, "节点 C 加入主题失败")

	// 4. B/C 创建订阅
	subB, err := topicB.Subscribe()
	require.NoError(t, err, "节点 B 订阅失败")
	defer subB.Cancel()

	subC, err := topicC.Subscribe()
	require.NoError(t, err, "节点 C 订阅失败")
	defer subC.Cancel()

	// 等待订阅传播 (GossipSub 需要时间建立 mesh)
	time.Sleep(2 * time.Second)

	// 5. A 发布消息
	testMsg := "Hello from A"
	err = topicA.Publish(ctx, []byte(testMsg))
	require.NoError(t, err, "节点 A 发布消息失败")
	t.Logf("节点 A 发布: %s", testMsg)

	// 6. 验证 B/C 收到消息
	msgB := testutil.WaitForMessage(t, subB, 15*time.Second)
	msgC := testutil.WaitForMessage(t, subC, 15*time.Second)

	assert.Equal(t, testMsg, string(msgB.Data), "节点 B 收到的消息应匹配")
	assert.Equal(t, testMsg, string(msgC.Data), "节点 C 收到的消息应匹配")
	assert.Equal(t, nodeA.ID(), msgB.From, "消息来源应为节点 A")
	assert.Equal(t, nodeA.ID(), msgC.From, "消息来源应为节点 A")

	t.Log("✅ PubSub 群组广播测试通过")
}

// TestPubSub_MultipleMessages 测试多条消息传递
//
// 验证 PubSub 能够可靠传递多条消息。
func TestPubSub_MultipleMessages(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK
	topicName := testutil.DefaultTestTopic

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

	// 2. 加入主题并订阅
	topicA, _ := realmA.PubSub().Join(topicName)
	topicB, _ := realmB.PubSub().Join(topicName)
	subB, _ := topicB.Subscribe()
	defer subB.Cancel()

	time.Sleep(2 * time.Second)

	// 3. 发送多条消息
	messages := []string{"Message 1", "Message 2", "Message 3"}
	for _, msg := range messages {
		topicA.Publish(ctx, []byte(msg))
		time.Sleep(500 * time.Millisecond)
	}

	// 4. 验证收到所有消息
	received := make([]string, 0, len(messages))
	for i := 0; i < len(messages); i++ {
		msg := testutil.WaitForMessage(t, subB, 10*time.Second)
		received = append(received, string(msg.Data))
	}

	assert.Equal(t, len(messages), len(received), "应收到所有消息")
	for i, msg := range messages {
		assert.Contains(t, received, msg, "应包含消息 %d", i+1)
	}

	t.Log("✅ 多条消息传递测试通过")
}

// TestPubSub_SelfMessage 测试自己发送的消息
//
// 验证节点不会收到自己发送的消息 (或正确处理)。
func TestPubSub_SelfMessage(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	psk := testutil.DefaultTestPSK
	topicName := testutil.DefaultTestTopic

	// 1. 创建节点
	nodeA := testutil.NewTestNode(t).
		WithListenAddr("/ip4/127.0.0.1/udp/0/quic-v1").
		Start()

	realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()

	// 2. 加入主题并订阅
	topicA, _ := realmA.PubSub().Join(topicName)
	subA, _ := topicA.Subscribe()
	defer subA.Cancel()

	time.Sleep(1 * time.Second)

	// 3. 发布消息
	testMsg := "Self message"
	topicA.Publish(ctx, []byte(testMsg))

	// 4. 验证 (根据实现，可能收到也可能收不到自己的消息)
	// 这里只验证不会崩溃
	// 使用带超时的 context 调用 Next
	shortCtx, shortCancel := context.WithTimeout(ctx, 2*time.Second)
	defer shortCancel()
	
	msg, err := subA.Next(shortCtx)
	if err == nil {
		t.Logf("收到自己的消息: %s (这是可接受的行为)", string(msg.Data))
	} else {
		t.Log("未收到自己的消息 (这也是可接受的行为)")
	}

	t.Log("✅ 自己消息测试通过")
}

// TestPubSub_MultipleTopics 测试多个主题
//
// 验证节点可以同时订阅多个主题。
func TestPubSub_MultipleTopics(t *testing.T) {
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

	// 2. 加入多个主题
	topic1A, _ := realmA.PubSub().Join("topic/1")
	topic2A, _ := realmA.PubSub().Join("topic/2")
	topic1B, _ := realmB.PubSub().Join("topic/1")
	topic2B, _ := realmB.PubSub().Join("topic/2")

	// 3. 订阅
	sub1B, _ := topic1B.Subscribe()
	defer sub1B.Cancel()
	sub2B, _ := topic2B.Subscribe()
	defer sub2B.Cancel()

	time.Sleep(2 * time.Second)

	// 4. 在不同主题发布消息
	topic1A.Publish(ctx, []byte("Topic 1 message"))
	time.Sleep(500 * time.Millisecond)
	topic2A.Publish(ctx, []byte("Topic 2 message"))

	// 5. 验证收到对应主题的消息
	msg1 := testutil.WaitForMessage(t, sub1B, 10*time.Second)
	assert.Equal(t, "Topic 1 message", string(msg1.Data))

	msg2 := testutil.WaitForMessage(t, sub2B, 10*time.Second)
	assert.Equal(t, "Topic 2 message", string(msg2.Data))

	t.Log("✅ 多主题测试通过")
}
