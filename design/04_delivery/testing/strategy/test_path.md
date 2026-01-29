# 测试路径与优先级

> 测试执行顺序、依赖关系、优先级策略

---

## 元信息

| 字段 | 值 |
|------|-----|
| **状态** | approved |
| **Owner** | DeP2P Team |
| **创建日期** | 2026-01-17 |
| **更新日期** | 2026-01-17 |

---

## 1. 测试依赖关系

### 1.1 架构依赖图

```
┌─────────────────────────────────────────────────────────────────────────────────┐
│                          测试依赖关系 (自底向上)                                  │
├─────────────────────────────────────────────────────────────────────────────────┤
│                                                                                 │
│  第 4 层 - Protocol (依赖 Realm)                                                │
│  ┌─────────┐ ┌─────────┐ ┌───────────┐ ┌──────────┐                            │
│  │ Streams │ │ PubSub  │ │ Messaging │ │ Liveness │                            │
│  └────┬────┘ └────┬────┘ └─────┬─────┘ └────┬─────┘                            │
│       │           │            │            │                                   │
│       └───────────┴────────────┴────────────┘                                   │
│                          │                                                      │
│                          ▼                                                      │
│  第 3 层 - Realm (依赖 Core + Discovery)                                        │
│  ┌──────┐ ┌────────┐ ┌───────────┐ ┌─────────┐ ┌─────────┐                     │
│  │ Auth │ │ Member │ │ Connector │ │ Routing │ │ Gateway │                     │
│  └──┬───┘ └───┬────┘ └─────┬─────┘ └────┬────┘ └────┬────┘                     │
│     │         │            │            │           │                           │
│     └─────────┴────────────┴────────────┴───────────┘                           │
│                          │                                                      │
│          ┌───────────────┴───────────────┐                                      │
│          ▼                               ▼                                      │
│  第 2 层 - Discovery              第 1 层 - Core                                │
│  ┌──────┐ ┌──────┐ ┌─────────┐   ┌───────────┐ ┌───────┐ ┌──────────┐          │
│  │ mDNS │ │ DHT  │ │Bootstrap│   │ Transport │ │ Swarm │ │ Security │          │
│  └──────┘ └──────┘ └─────────┘   └───────────┘ └───────┘ └──────────┘          │
│                                        │                                        │
│                                        ▼                                        │
│                              ┌──────────────────┐                               │
│                              │ 网络/操作系统基础 │                               │
│                              └──────────────────┘                               │
│                                                                                 │
└─────────────────────────────────────────────────────────────────────────────────┘
```

### 1.2 测试依赖规则

| 规则 | 说明 |
|------|------|
| **自底向上** | 先测试底层组件，再测试上层 |
| **隔离优先** | 单组件测试 → 组件间测试 → 完整场景 |
| **阻塞传递** | 底层失败则跳过依赖它的上层测试 |

---

## 2. 测试优先级路径

### 2.1 推荐执行顺序

```
1. 连接测试 ─────► 2. 流测试 ─────► 3. Realm/成员 ─────► 4. PubSub ─────► 5. 事件 ─────► 6. E2E
   (基础)          (依赖连接)        (依赖连接)          (依赖Realm)       (辅助)         (完整场景)
```

### 2.2 分阶段执行

#### 阶段 1: 基础连接 (必须先通过)

```bash
# 优先级: P0
go test ./tests/integration/core/connection_test.go -v
```

| 测试 | 验证内容 | 阻塞影响 |
|------|---------|---------|
| `TestConnection_DirectConnect` | 两节点直连 | 阻塞所有后续测试 |
| `TestConnection_MultiNode` | 多节点连接 | 阻塞 Realm/PubSub 测试 |
| `TestConnection_Disconnect` | 断开检测 | 阻塞成员离开测试 |

#### 阶段 2: 基础流 (依赖阶段 1)

```bash
# 优先级: P0
go test ./tests/integration/core/simple_stream_test.go -v
```

| 测试 | 验证内容 | 阻塞影响 |
|------|---------|---------|
| `TestSimple_HostStream` | Host 层流通信 | 阻塞 Streams/Messaging 测试 |

#### 阶段 3: Realm 认证 (依赖阶段 1)

```bash
# 优先级: P0
go test ./tests/integration/core/realm_auth_test.go -v
```

| 测试 | 验证内容 | 阻塞影响 |
|------|---------|---------|
| `TestRealm_Auth_Success` | PSK 认证成功 | 阻塞成员管理测试 |
| `TestRealm_Auth_WrongPSK` | 认证失败处理 | - |
| `TestRealm_MultiPeer_Auth` | 多节点认证 | 阻塞 PubSub 测试 |

#### 阶段 4: 成员管理 (依赖阶段 3)

```bash
# 优先级: P0
go test ./tests/integration/core/member_test.go -v
```

| 测试 | 验证内容 | 阻塞影响 |
|------|---------|---------|
| `TestMember_Join` | 成员加入 | 阻塞 PubSub/Streams 测试 |
| `TestMember_Leave` | 成员离开 | 阻塞 E2E 退出场景 |
| `TestMember_Discovery` | 成员发现 | 阻塞群聊测试 |

#### 阶段 5: 应用协议 (依赖阶段 4)

```bash
# 优先级: P0
go test ./tests/integration/protocol/... -v
```

| 测试组 | 验证内容 | 阻塞影响 |
|--------|---------|---------|
| `streams_test.go` | 双向流通信 | 阻塞私聊 E2E |
| `pubsub_test.go` | 发布订阅 | 阻塞群聊 E2E |
| `liveness_test.go` | 存活检测 | - |

#### 阶段 6: 事件总线 (并行)

```bash
# 优先级: P1
go test ./tests/integration/core/eventbus_test.go -v
```

#### 阶段 7: E2E 场景 (依赖阶段 5)

```bash
# 优先级: P0
go test -tags=e2e ./tests/e2e/scenario/... -v -timeout 10m
```

---

## 3. 快速验证命令

### 3.1 最小验证 (~30秒)

```bash
# 仅验证连接和流
go test ./tests/integration/core/connection_test.go \
        ./tests/integration/core/simple_stream_test.go -v
```

### 3.2 核心验证 (~2分钟)

```bash
# 验证所有 Core 层集成测试
go test ./tests/integration/core/... -v -timeout 3m
```

### 3.3 完整验证 (~5分钟)

```bash
# 验证所有集成测试 + E2E
go test ./tests/integration/... -v -timeout 5m && \
go test -tags=e2e ./tests/e2e/scenario/... -v -timeout 5m
```

---

## 4. CI/CD 集成

### 4.1 PR 检查流程

```yaml
jobs:
  test:
    steps:
      # 阶段 1: 快速反馈
      - name: Quick Check (Connection + Stream)
        run: |
          go test ./tests/integration/core/connection_test.go \
                  ./tests/integration/core/simple_stream_test.go -v
        timeout-minutes: 2

      # 阶段 2: 完整集成测试
      - name: Integration Tests
        run: go test ./tests/integration/... -v -timeout 5m
        if: success()

      # 阶段 3: E2E 测试 (仅 main 分支)
      - name: E2E Tests
        run: go test -tags=e2e ./tests/e2e/... -v -timeout 10m
        if: success() && github.ref == 'refs/heads/main'
```

### 4.2 失败处理策略

| 阶段失败 | 处理 |
|---------|------|
| 连接测试失败 | 跳过所有后续测试，标记 PR 为阻塞 |
| 流测试失败 | 跳过协议层测试，允许 Core 其他测试继续 |
| Realm 测试失败 | 跳过 Protocol 和 E2E 测试 |
| E2E 失败 | 允许合并但标记警告 |

---

## 5. 问题排查路径

### 5.1 常见问题定位

```
测试超时?
    └─► 检查 connection_test.go 是否通过
        ├─ 通过 → 检查 simple_stream_test.go
        │         ├─ 通过 → 检查 realm_auth_test.go
        │         │         └─ 通过 → 问题在协议层
        │         └─ 失败 → 问题在 Swarm.handleInboundStreams
        └─ 失败 → 问题在 Transport/QUIC 层
```

### 5.2 调试命令

```bash
# 详细日志
DEP2P_LOG_LEVEL=debug go test ./tests/integration/core/connection_test.go -v

# 单个测试
go test ./tests/integration/core/... -run TestConnection_DirectConnect -v

# 超时调试
go test ./tests/integration/core/... -v -timeout 120s
```

---

## 6. 实施过程修复的关键 BUG

> 记录在测试路径实施过程中发现并修复的问题

| BUG | 修复位置 | 影响 | 修复方式 |
|-----|---------|------|---------|
| QUIC 连接死锁 | `internal/core/transport/quic/conn.go` | 连接/断开操作卡死 | 释放锁后再调用阻塞操作 |
| 出站连接无法接收入站流 | `internal/core/swarm/dial.go` | 流测试超时 | dialAddr 中启动 handleInboundStreams |
| 连接断开检测失败 | `internal/core/swarm/listen.go` | 成员列表不更新 | handleInboundStreams 退出时主动关闭连接 |
| PubSub Mesh 未建立 | `internal/protocol/pubsub/gossipsub.go` | 消息不传播 | Join 后立即调用 graftPeers |
| 成员离开未检测 | `internal/realm/member/manager.go` | 成员列表不更新 | 订阅 PeerDisconnected 事件 |

---

## 变更历史

| 版本 | 日期 | 作者 | 变更说明 |
|------|------|------|----------|
| v1.0 | 2026-01-17 | DeP2P Team | 初始版本，基于测试框架实施经验 |
