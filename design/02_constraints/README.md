# 约束与规范 (Constraints)

> 定义 DeP2P 必须遵守的协议规范和工程标准

---

## 概述

本目录定义的协议规范（L0-L4）与软件架构层次是**不同维度**的分层：

- **协议规范层次 (L0-L4)**：描述网络协议栈和消息格式
- **软件架构层次**：描述代码组织和模块依赖

---

## 目录结构

```
02_constraints/
├── README.md                    # 本文件
├── protocol/                    # 协议规范
│   ├── README.md
│   ├── SPEC_INDEX.md            # 规范索引
│   ├── L0_encoding/             # 编码规范
│   ├── L1_identity/             # 身份与密钥
│   ├── L2_transport/            # 传输层协议
│   ├── L3_network/              # 网络层协议
│   └── L4_application/          # 应用层协议
├── engineering/                 # 工程标准
│   ├── README.md
│   ├── coding_specs/            # 编码规范
│   ├── isolation/               # 隔离约束
│   └── standards/               # 工程标准
└── scenarios/                   # 场景说明
```

---

## 子目录说明

### protocol/ - 协议规范

定义 DeP2P 网络协议栈的规范，采用分层结构：

| 层级 | 目录 | 内容 |
|------|------|------|
| L0 | `L0_encoding/` | 序列化、版本号、字节序 |
| L1 | `L1_identity/` | 密钥格式、NodeID 计算、签名算法 |
| L2 | `L2_transport/` | QUIC、Relay、安全握手 |
| L3 | `L3_network/` | 发现协议、NAT 穿透、路由 |
| L4 | `L4_application/` | Realm、消息、PubSub、Streams、Liveness |

### engineering/ - 工程标准

定义开发过程必须遵守的标准：

| 目录 | 内容 |
|------|------|
| `coding_specs/` | 代码风格、错误处理、日志规范 |
| `isolation/` | 测试隔离、网络边界约束 |
| `standards/` | 命名规范、API 设计、文档要求 |

---

## 协议命名规范

DeP2P 使用统一的协议命名空间，Realm 协议和应用协议**嵌入 RealmID** 实现协议级别的显式隔离。

```
系统协议格式：/dep2p/sys/<protocol>/<version>
Realm 协议格式：/dep2p/realm/<realmID>/<protocol>/<version>
应用协议格式：/dep2p/app/<realmID>/<protocol>/<version>
```

> **关键设计**：RealmID 嵌入协议路径，提供协议级别的隔离边界，无法伪造。

### 协议分类示例

| 类别 | 协议 ID 格式 | 说明 |
|------|-------------|------|
| **系统协议** | `/dep2p/sys/identify/1.0.0` | 身份识别 |
| | `/dep2p/sys/ping/1.0.0` | 存活检测 |
| | `/dep2p/relay/1.0.0/hop` | 中继 HOP |
| | `/dep2p/relay/1.0.0/stop` | 中继 STOP |
| | `/dep2p/sys/dht/1.0.0` | DHT 发现 |
| **Realm 协议** | `/dep2p/realm/<realmID>/join/1.0.0` | 加入 Realm |
| | `/dep2p/realm/<realmID>/auth/1.0.0` | Realm 认证 |
| | `/dep2p/realm/<realmID>/sync/1.0.0` | 成员同步 |
| **应用协议** | `/dep2p/app/<realmID>/messaging/1.0.0` | 消息传递 |
| | `/dep2p/app/<realmID>/pubsub/1.0.0` | 发布订阅 |
| | `/dep2p/app/<realmID>/streams/1.0.0` | 双向流 |

> **说明**：Relay 的 HOP/STOP 流协议采用 `/dep2p/relay/1.0.0/*` 前缀，作为系统协议特例使用。

### RealmID 格式

| 属性 | 值 |
|------|-----|
| 长度 | 32 字节 |
| 编码 | Base58（约 44 字符） |
| 派生 | HKDF(PSK, salt="dep2p-realm-id-v1") |

### 访问控制规则

| 协议前缀 | Relay | 需要 Realm |
|---------|-------|-----------|
| `/dep2p/sys/*` | ✅ 允许 | 否 |
| `/dep2p/realm/<realmID>/*` | ✅ 仅匹配的 ID + 成员验证 | 是 |
| `/dep2p/app/<realmID>/*` | ✅ 仅匹配的 ID + 成员验证 | 是 |

---

## 核心设计原则

| 原则 | 说明 | 不变量 |
|------|------|--------|
| **身份第一** | 所有连接必须验证身份 | INV-001 |
| **Realm 隔离** | 业务 API 需要 Realm 成员资格 | INV-002 |
| **统一 Relay** | Relay 统一承载，通过协议与成员认证隔离 | ADR-0010 |
| **"仅 ID 连接"边界** | Realm 内允许纯 NodeID 连接，跨 Realm 必须提供地址 | INV-004 |
| **Relay 统一作用** | 地址发现辅助 + 数据通信保底 | - |

详见：[系统不变量](../03_architecture/L1_overview/invariants.md)

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [协议规范](protocol/) | 协议规范 (L0-L4) |
| [命名空间规范](protocol/namespace.md) | DHT Key / PubSub Topic / Discovery namespace |
| [工程标准](engineering/) | 编码规范和工程标准 |
| [pkg/ 层设计原则](engineering/standards/pkg_design.md) | 工具包 vs 接口包 |
| [架构决策](../01_context/decisions/) | ADR 记录 |

---

**最后更新**：2026-01-27
