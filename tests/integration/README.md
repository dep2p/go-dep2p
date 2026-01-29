# 组件集成测试

本目录存放 DEP2P 组件间的集成测试，使用**真实组件**（非 Mock）验证组件协同工作。

## 测试范围

### Core 层集成 (`core/`)

- **connection_test.go**: 连接建立、断开、重连、多节点连接
- **realm_auth_test.go**: Realm 加入、PSK 认证、成员发现
- **member_test.go**: 成员列表、成员检查、成员离开
- **eventbus_test.go**: 连接/断开事件、成员变化事件

### Protocol 层集成 (`protocol/`)

- **pubsub_test.go**: PubSub 群组广播、多条消息、多主题
- **streams_test.go**: Streams 双向流、多流、Handler 注销、大数据传输
- **liveness_test.go**: Liveness Ping/Pong、多次 Ping、不可达节点

## 运行测试

```bash
# 运行所有集成测试
go test ./tests/integration/... -v -timeout 5m

# 运行 Core 层测试
go test ./tests/integration/core/... -v

# 运行 Protocol 层测试
go test ./tests/integration/protocol/... -v

# 使用构建标签
go test -tags=integration ./tests/integration/...
```

## 测试特点

1. **真实性**: 使用真实节点和网络连接，非 Mock
2. **单进程**: 所有节点在同一进程内运行
3. **自动化**: 所有测试可通过 `go test` 运行
4. **独立性**: 每个测试使用独立的数据目录

## 示例

```go
func TestPubSub_GroupBroadcast(t *testing.T) {
    ctx := context.Background()
    psk := testutil.DefaultTestPSK

    // 创建节点
    nodeA := testutil.NewTestNode(t).Start()
    nodeB := testutil.NewTestNode(t).Start()

    // 加入 Realm
    realmA := testutil.NewTestRealm(t, nodeA).WithPSK(psk).Join()
    realmB := testutil.NewTestRealm(t, nodeB).WithPSK(psk).Join()

    // 建立连接并等待认证
    nodeB.Host().Connect(ctx, nodeA.ID(), nodeA.ListenAddrs())
    testutil.WaitForMembers(t, realmA, 2, 30*time.Second)

    // 测试 PubSub
    topicA, _ := realmA.PubSub().Join("test/chat")
    topicB, _ := realmB.PubSub().Join("test/chat")
    subB, _ := topicB.Subscribe()
    
    topicA.Publish(ctx, []byte("Hello"))
    msg := testutil.WaitForMessage(t, subB, 10*time.Second)
    assert.Equal(t, "Hello", string(msg.Data))
}
```
