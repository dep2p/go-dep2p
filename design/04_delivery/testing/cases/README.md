# 测试用例 (Cases)

> 按模块组织的测试用例索引

---

## 目录结构

```
cases/
├── README.md              # 本文件
├── identity/              # 身份测试用例
│   └── README.md
├── transport/             # 传输测试用例
│   └── README.md
├── relay/                 # 中继测试用例
│   └── README.md
├── realm/                 # Realm 测试用例
│   └── README.md
├── protocol/              # Protocol 层测试用例
│   └── README.md
├── discovery/             # Discovery 层测试用例
│   └── README.md
└── e2e/                   # 端到端测试用例
    └── README.md
```

---

## 测试用例编号规范

### 编号格式

```
TST-{模块}-{序号}

Core 层模块代码：
- IDENTITY: 身份模块
- TRANSPORT: 传输模块
- SECURITY: 安全模块
- RELAY: 中继模块
- CONNMGR: 连接管理模块

Realm 层模块代码：
- REALM: Realm 核心
- REALM-AUTH: Realm 认证
- REALM-MEMBER: 成员管理
- REALM-ROUTING: Realm 路由
- REALM-GATEWAY: Realm 网关

Protocol 层模块代码：
- MESSAGING: 消息传递
- PUBSUB: 发布订阅
- STREAMS: 双向流
- LIVENESS: 活性检测

Discovery 层模块代码：
- DHT: DHT 发现
- MDNS: mDNS 发现
- BOOTSTRAP: 引导节点
- DNS: DNS 发现
- RENDEZVOUS: 汇合点

端到端：
- E2E: 端到端测试
```

### 序号分配

| 编号范围 | 模块 |
|----------|------|
| 0001-0099 | 每模块预留 99 个 |

---

## 测试用例索引

### Core 层

#### 身份模块 (Identity)

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| [TST-IDENTITY-0001](identity/README.md) | 密钥生成 | 单元 | P0 | ✅ |
| [TST-IDENTITY-0002](identity/README.md) | PeerID 派生 | 单元 | P0 | ✅ |
| [TST-IDENTITY-0003](identity/README.md) | 签名验证 | 单元 | P0 | ✅ |

#### 传输模块 (Transport)

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| [TST-TRANSPORT-0001](transport/README.md) | TCP 连接建立 | 集成 | P0 | ✅ |
| [TST-TRANSPORT-0002](transport/README.md) | QUIC 连接建立 | 集成 | P0 | ✅ |
| [TST-TRANSPORT-0003](transport/README.md) | 数据传输 | 集成 | P0 | ✅ |

#### 中继模块 (Relay)

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| [TST-RELAY-0001](relay/README.md) | Relay 注册 | 集成 | P0 | ✅ |
| [TST-RELAY-0002](relay/README.md) | Relay 注册（Realm 场景） | 集成 | P0 | ✅ |
| [TST-RELAY-0003](relay/README.md) | 中继转发 | 集成 | P0 | ✅ |

---

### Realm 层

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| [TST-REALM-0001](realm/README.md) | Realm 创建 | 单元 | P0 | ✅ |
| [TST-REALM-0002](realm/README.md) | RealmID 生成 | 单元 | P0 | ✅ |
| [TST-REALM-0003](realm/README.md) | Manager 生命周期 | 单元 | P0 | ✅ |
| [TST-REALM-AUTH-0001](realm/README.md) | PSK 认证成功 | 集成 | P0 | ✅ |
| [TST-REALM-AUTH-0002](realm/README.md) | PSK 认证失败 | 集成 | P0 | ✅ |
| [TST-REALM-MEMBER-0001](realm/README.md) | 成员加入 | 集成 | P0 | ✅ |
| [TST-REALM-MEMBER-0002](realm/README.md) | 成员离开 | 集成 | P0 | ✅ |
| [TST-REALM-ROUTING-0001](realm/README.md) | 路由表更新 | 单元 | P1 | ✅ |
| [TST-REALM-GATEWAY-0001](realm/README.md) | 网关中继 | 集成 | P1 | ✅ |

详见 [realm/README.md](realm/README.md)

---

### Protocol 层

#### Messaging 模块

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| [TST-MESSAGING-0001](protocol/README.md) | Request/Response | 单元 | P0 | ✅ |
| [TST-MESSAGING-0002](protocol/README.md) | 超时处理 | 单元 | P0 | ✅ |
| [TST-MESSAGING-0003](protocol/README.md) | 并发请求 | 单元 | P0 | ✅ |
| [TST-MESSAGING-0004](protocol/README.md) | Handler 注册 | 单元 | P0 | ✅ |
| [TST-MESSAGING-0005](protocol/README.md) | 编解码 | 单元 | P1 | ✅ |

#### PubSub 模块

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| [TST-PUBSUB-0001](protocol/README.md) | Topic 订阅 | 单元 | P0 | ✅ |
| [TST-PUBSUB-0002](protocol/README.md) | 消息发布 | 单元 | P0 | ✅ |
| [TST-PUBSUB-0003](protocol/README.md) | 取消订阅 | 单元 | P0 | ✅ |
| [TST-PUBSUB-0004](protocol/README.md) | 消息验证 | 单元 | P1 | ✅ |
| [TST-PUBSUB-0005](protocol/README.md) | Mesh 管理 | 单元 | P1 | ✅ |

#### Streams 模块

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| [TST-STREAMS-0001](protocol/README.md) | 打开双向流 | 单元 | P0 | ✅ |
| [TST-STREAMS-0002](protocol/README.md) | 流读写 | 单元 | P0 | ✅ |
| [TST-STREAMS-0003](protocol/README.md) | 流关闭 | 单元 | P0 | ✅ |
| [TST-STREAMS-0004](protocol/README.md) | 并发流 | 单元 | P1 | ✅ |

#### Liveness 模块

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| [TST-LIVENESS-0001](protocol/README.md) | Ping/Pong | 单元 | P0 | ✅ |
| [TST-LIVENESS-0002](protocol/README.md) | 健康状态 | 单元 | P0 | ✅ |
| [TST-LIVENESS-0003](protocol/README.md) | Watch 订阅 | 单元 | P1 | ✅ |

详见 [protocol/README.md](protocol/README.md)

---

### Discovery 层

| 用例 ID | 标题 | 类型 | 优先级 | 状态 |
|---------|------|------|:------:|:----:|
| [TST-DHT-0001](discovery/README.md) | DHT 发现 | 集成 | P0 | ✅ |
| [TST-DHT-0002](discovery/README.md) | DHT 查询 | 单元 | P0 | ✅ |
| [TST-MDNS-0001](discovery/README.md) | mDNS 发现 | 集成 | P0 | ✅ |
| [TST-MDNS-0002](discovery/README.md) | mDNS 广播 | 单元 | P0 | ✅ |
| [TST-BOOTSTRAP-0001](discovery/README.md) | 引导连接 | 集成 | P0 | ✅ |
| [TST-DNS-0001](discovery/README.md) | DNS 解析 | 单元 | P1 | ✅ |
| [TST-RENDEZVOUS-0001](discovery/README.md) | 汇合点注册 | 集成 | P1 | ✅ |

详见 [discovery/README.md](discovery/README.md)

---

### 端到端测试 (E2E)

| 用例 ID | 标题 | 类型 | 优先级 | 状态 | 代码位置 |
|---------|------|------|:------:|:----:|----------|
| [TST-E2E-0001](e2e/README.md) | 完整聊天场景 | E2E | P0 | ✅ | `tests/e2e/scenario/chat_test.go` |
| [TST-E2E-0002](e2e/README.md) | 私聊场景 | E2E | P0 | ✅ | `tests/e2e/scenario/chat_test.go` |
| [TST-E2E-0003](e2e/README.md) | mDNS 发现 | E2E | P0 | ✅ | `tests/e2e/scenario/discovery_test.go` |
| [TST-E2E-0004](e2e/README.md) | 发现后加入 | E2E | P0 | ✅ | `tests/e2e/scenario/discovery_test.go` |
| [TST-E2E-0005](e2e/README.md) | 网络分区 | E2E | P1 | ⏳ | `tests/e2e/resilience/` |
| [TST-E2E-0006](e2e/README.md) | 故障恢复 | E2E | P1 | ⏳ | `tests/e2e/resilience/` |

详见 [e2e/README.md](e2e/README.md)

---

## 用例状态说明

| 状态 | 说明 |
|------|------|
| ✅ 已实现 | 代码已完成 |
| 🚧 进行中 | 正在实现 |
| ⏳ 计划中 | 待实现 |
| 📝 草稿 | 用例设计中 |
| ❌ 已废弃 | 不再使用 |

---

## 快速链接

| 目录 | 说明 |
|------|------|
| [identity/](identity/) | 身份测试用例 |
| [transport/](transport/) | 传输测试用例 |
| [relay/](relay/) | 中继测试用例 |
| [realm/](realm/) | Realm 测试用例 |
| [protocol/](protocol/) | Protocol 层测试用例 |
| [discovery/](discovery/) | Discovery 层测试用例 |
| [e2e/](e2e/) | 端到端测试用例 |

---

**最后更新**：2026-01-15
