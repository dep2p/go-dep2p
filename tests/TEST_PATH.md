# DEP2P 测试路径指南

> 日期: 2026-01-17  
> 状态: 已验证通过

---

## 1. 测试层级架构

```
                          ╭─────────────────────────╮
                         ╱                           ╲
                        ╱     E2E 场景测试 (4 个)     ╲     ← 完整用户流程
                       ╱      tests/e2e/scenario      ╲       chat-local 对标
                      ╱                                 ╲
                     ╱━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╲
                    ╱                                     ╲
                   ╱       Protocol 集成测试 (11 个)        ╲   ← 跨节点通信
                  ╱       tests/integration/protocol        ╲     PubSub/Streams
                 ╱                                           ╲
                ╱━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━╲
               ╱                                               ╲
              ╱            Core 集成测试 (15 个)                 ╲  ← 基础组件
             ╱           tests/integration/core                  ╲    连接/认证
            ╱                                                     ╲
           ╰───────────────────────────────────────────────────────╯
```

---

## 2. 按优先级执行测试

### 2.1 第一优先级：基础连接 (必须首先通过)

```bash
go test -tags=integration ./tests/integration/core/connection_test.go -v
```

| 测试名 | 验证内容 |
|-------|---------|
| `TestConnection_DirectConnect` | 节点直连成功 |
| `TestConnection_Disconnect` | 断开检测正确 |
| `TestConnection_Reconnect` | 重连机制正常 |
| `TestConnection_MultiNode` | 多节点连接 |

### 2.2 第二优先级：点对点流 (依赖连接)

```bash
go test -tags=integration ./tests/integration/core/simple_stream_test.go -v
go test -tags=integration ./tests/integration/protocol/streams_test.go -v
```

| 测试名 | 验证内容 |
|-------|---------|
| `TestSimple_HostStream` | Host 层流基础功能 |
| `TestStreams_BidirectionalChat` | 双向流通信 |
| `TestStreams_MultipleStreams` | 多流并发 |
| `TestStreams_HandlerUnregister` | Handler 注销 |
| `TestStreams_LargeData` | 大数据传输 |

### 2.3 第三优先级：Realm 认证与成员管理 (依赖连接)

```bash
go test -tags=integration ./tests/integration/core/realm_auth_test.go -v
go test -tags=integration ./tests/integration/core/member_test.go -v
```

| 测试名 | 验证内容 |
|-------|---------|
| `TestRealm_AuthAndMemberDiscovery` | PSK 认证 + 成员发现 |
| `TestRealm_MultiNodeAuth` | 多节点认证 |
| `TestRealm_DifferentPSK` | 不同 PSK 隔离 |
| `TestMember_List` | 成员列表查询 |
| `TestMember_IsMember` | 成员检查 |
| `TestMember_Leave` | 成员离开检测 |

### 2.4 第四优先级：PubSub 群组消息 (依赖 Realm)

```bash
go test -tags=integration ./tests/integration/protocol/pubsub_test.go -v
```

| 测试名 | 验证内容 |
|-------|---------|
| `TestPubSub_GroupBroadcast` | 群组广播 |
| `TestPubSub_MultipleMessages` | 多消息传输 |
| `TestPubSub_SelfMessage` | 自发消息过滤 |
| `TestPubSub_MultipleTopics` | 多 Topic 订阅 |

### 2.5 第五优先级：事件总线与 Liveness

```bash
go test -tags=integration ./tests/integration/core/eventbus_test.go -v
go test -tags=integration ./tests/integration/protocol/liveness_test.go -v
```

| 测试名 | 验证内容 |
|-------|---------|
| `TestEventBus_PeerConnected` | 连接事件 |
| `TestEventBus_PeerDisconnected` | 断开事件 |
| `TestEventBus_MultipleEvents` | 多事件并发 |
| `TestEventBus_RealmMemberEvents` | Realm 成员事件 |
| `TestLiveness_Ping` | Ping 功能 |
| `TestLiveness_MultiplePings` | 连续 Ping |
| `TestLiveness_UnreachablePeer` | 不可达节点处理 |

### 2.6 第六优先级：E2E 完整场景

```bash
go test -tags=e2e ./tests/e2e/scenario/... -v
```

| 测试名 | 验证内容 |
|-------|---------|
| `TestE2E_ChatScenario_Full` | **完整聊天场景** (5 阶段) |
| `TestE2E_ChatScenario_GroupChatOnly` | 简化群聊场景 |
| `TestE2E_MDNS_Discovery` | mDNS 发现场景 |
| `TestE2E_ManualConnect` | 手动连接场景 |

---

## 3. 完整测试执行命令

### 3.1 快速验证 (< 2 分钟)

```bash
# 只运行 core 集成测试
go test -tags=integration ./tests/integration/core/... -timeout 3m
```

### 3.2 标准验证 (约 3 分钟)

```bash
# 运行所有集成测试
go test -tags=integration ./tests/integration/... -timeout 5m
```

### 3.3 完整验证 (约 5 分钟)

```bash
# 运行所有测试 (集成 + E2E)
go test -tags=integration ./tests/integration/... -timeout 5m && \
go test -tags=e2e ./tests/e2e/... -timeout 5m
```

---

## 4. 测试覆盖矩阵 (chat-local 对标)

| 功能 | 集成测试 | E2E 测试 | chat-local 对应 |
|-----|---------|---------|----------------|
| **节点启动** | ✅ connection | ✅ chat_full | `dep2p.Start()` |
| **监听地址** | ✅ connection | ✅ chat_full | `WithListenPort()` |
| **直接连接** | ✅ connection | ✅ manual_connect | `Host().Connect()` |
| **mDNS 发现** | - | ✅ mdns_discovery | 自动发现 |
| **Realm 加入** | ✅ realm_auth | ✅ chat_full | `JoinRealm()` |
| **PSK 认证** | ✅ realm_auth | ✅ chat_full | 自动 |
| **成员发现** | ✅ member | ✅ chat_full | `Members()` |
| **成员离开** | ✅ member_leave | ✅ chat_full | `Close()` |
| **PubSub Join** | ✅ pubsub | ✅ chat_full | `PubSub().Join()` |
| **PubSub Subscribe** | ✅ pubsub | ✅ chat_full | `Subscribe()` |
| **PubSub Publish** | ✅ pubsub | ✅ chat_full | `Publish()` |
| **群消息接收** | ✅ pubsub | ✅ chat_full | 广播消息 |
| **Streams Register** | ✅ streams | ✅ chat_full | `RegisterHandler()` |
| **Streams Open** | ✅ streams | ✅ chat_full | `Open()` |
| **私聊 Read/Write** | ✅ streams | ✅ chat_full | `/msg` 命令 |
| **连接事件** | ✅ eventbus | - | `EvtPeerConnected` |
| **断开事件** | ✅ eventbus | - | `EvtPeerDisconnected` |
| **Ping** | ✅ liveness | - | `/ping` 命令 |

---

## 5. 当前测试统计

```
┌─────────────────────────────────────────────────────────────┐
│                    测试通过情况                              │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  集成测试 (integration/)                                     │
│  ├── core/          15 个测试  ✅ 全部通过                    │
│  └── protocol/      11 个测试  ✅ 全部通过                    │
│                                                             │
│  E2E 测试 (e2e/)                                            │
│  ├── scenario/       4 个测试  ✅ 全部通过                    │
│  └── resilience/     2 个测试  ⏳ TODO (网络分区/故障恢复)     │
│                                                             │
│  总计: 30 个测试通过, 2 个待实现                              │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

---

## 6. 问题排查指南

### 6.1 连接测试失败

1. 检查端口是否被占用
2. 检查 QUIC 传输是否正常初始化
3. 确认 `handleInboundStreams` 在 dial 和 listen 都启动

### 6.2 流测试超时

1. 确认 Handler 已注册 (`RegisterHandler`)
2. 确认 `handleInboundStreams` 已启动
3. 检查协议 ID 是否匹配

### 6.3 PubSub 消息未收到

1. 确认 Join 后 Mesh 已建立 (`graftPeers`)
2. 检查订阅是否成功 (`Subscribe`)
3. 确认有连接且成员发现完成

### 6.4 成员离开未检测

1. 确认订阅了 `PeerDisconnected` 事件
2. 检查 `watchDisconnections` 是否启动
3. 确认连接断开事件正确触发

---

## 7. 已修复的关键 BUG

| BUG | 修复位置 | 影响 |
|-----|---------|------|
| QUIC 连接死锁 | `quic/conn.go` | 连接/断开卡死 |
| 出站连接无法接收入站流 | `swarm/dial.go` | 流测试超时 |
| PubSub Mesh 未建立 | `pubsub/gossipsub.go` | 消息不传播 |
| 成员离开未检测 | `member/manager.go` | 成员列表不更新 |
