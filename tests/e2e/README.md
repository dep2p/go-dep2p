# 端到端测试

本目录存放 DEP2P 的端到端测试，模拟完整的用户场景，对标 `examples/chat-local`。

## 测试范围

### 用户场景 (`scenario/`)

- **chat_test.go**: 完整聊天场景
  - Phase 1: 启动 3 个节点
  - Phase 2: 加入 Realm + 自动连接
  - Phase 3: 群聊测试 (PubSub)
  - Phase 4: 私聊测试 (Streams)
  - Phase 5: 成员变化测试 (节点退出)

- **discovery_test.go**: 发现场景
  - mDNS 自动发现
  - 手动连接

### 韧性测试 (`resilience/`)

- **partition_test.go**: 网络分区恢复 (占位)
- **recovery_test.go**: 节点故障恢复 (占位)

## 运行测试

```bash
# 运行所有 E2E 测试
go test ./tests/e2e/... -v -timeout 10m

# 运行场景测试
go test ./tests/e2e/scenario/... -v -tags=e2e

# 运行特定测试
go test ./tests/e2e/scenario/chat_test.go -v -tags=e2e -run ChatScenario_Full

# 使用构建标签
go test -tags=e2e ./tests/e2e/... -timeout 10m
```

## 测试特点

1. **完整性**: 覆盖 chat-local 全部功能
2. **真实性**: 多节点真实网络交互
3. **自动化**: 无需手动操作
4. **独立性**: 不依赖 examples 进行功能验证

## 示例

```go
func TestE2E_ChatScenario_Full(t *testing.T) {
    if testing.Short() {
        t.Skip("跳过 E2E 测试")
    }

    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
    defer cancel()

    psk := "chat-local-e2e-test"

    // 启动节点
    alice := testutil.NewTestNode(t).Start()
    bob := testutil.NewTestNode(t).Start()

    // 加入 Realm
    realmAlice := testutil.NewTestRealm(t, alice).WithPSK(psk).Join()
    realmBob := testutil.NewTestRealm(t, bob).WithPSK(psk).Join()

    // 连接和认证
    bob.Host().Connect(ctx, alice.ID(), alice.ListenAddrs())
    testutil.WaitForMembers(t, realmAlice, 2, 30*time.Second)

    // 群聊测试
    topicAlice, _ := realmAlice.PubSub().Join("chat/general")
    topicBob, _ := realmBob.PubSub().Join("chat/general")
    subBob, _ := topicBob.Subscribe()
    
    topicAlice.Publish(ctx, []byte("Hello"))
    msg := testutil.WaitForMessage(t, subBob, 10*time.Second)
    assert.Contains(t, string(msg.Data), "Hello")
}
```

## 注意事项

- E2E 测试需要较长时间（5-10 分钟）
- 某些测试可能需要特殊环境（如 mDNS）
- 使用 `testing.Short()` 跳过长时间测试
