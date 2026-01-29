# DeP2P Chat - 统一 P2P 聊天示例

## 概述

这是一个统一的 P2P 聊天示例，整合了局域网和跨网络场景，演示 DeP2P 的核心功能：

- **mDNS 自动发现** - 同一局域网节点自动连接
- **Bootstrap 发现** - 通过引导节点发现其他节点
- **Relay** - NAT 后节点通过中继通信（统一 Relay v2.0）
- **Gateway** - Realm 内部网关，运行时可配置（Realm 层）
- **PubSub 群聊** - 基于 GossipSub 的发布订阅
- **Streams 私聊** - 基于双向流的点对点消息

---

## 架构说明

### Node 层 vs Realm 层

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                              架构分层                                         │
├──────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │                         Node 层（启动前配置）                            │ │
│  │                                                                        │ │
│  │  配置项：                                                               │ │
│  │    --bootstrap <addr>     引导节点地址                                  │ │
│  │    --relay <addr>         Relay 地址                                   │ │
│  │                                                                        │ │
│  │  服务能力：                                                             │ │
│  │    --serve                启用 Bootstrap + Relay 服务                  │ │
│  │    --public-addr <addr>   公网地址（服务模式必需）                       │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                    │                                         │
│                                    │ JoinRealm()                             │
│                                    ▼                                         │
│  ┌────────────────────────────────────────────────────────────────────────┐ │
│  │                        Realm 层（运行时配置）                            │ │
│  │                                                                        │ │
│  │  配置项：                                                               │ │
│  │    /gateway set <addr>    设置 Gateway（运行时）                        │ │
│  │    /gateway remove        移除 Gateway                                 │ │
│  │                                                                        │ │
│  │  服务能力：                                                             │ │
│  │    /gateway enable        启用 Gateway 服务                            │ │
│  │    /gateway disable       禁用 Gateway 服务                            │ │
│  └────────────────────────────────────────────────────────────────────────┘ │
│                                                                              │
└──────────────────────────────────────────────────────────────────────────────┘
```

**关键区别**：
- **Node 层配置**（Bootstrap、Relay）必须在**启动前**指定
- **Realm 层配置**（Gateway）可以在**运行时**动态配置

---

## 快速开始

### 场景 1：零配置启动（局域网聊天）

最简单的使用方式，同一局域网内的节点自动通过 mDNS 发现并连接。

```bash
# 终端 1
go run ./examples/chat

# 终端 2（同一局域网）
go run ./examples/chat

# 终端 3（可选）
go run ./examples/chat
```

节点会自动发现并连接，直接开始聊天。

---

### 场景 2：直接连接（知道对方地址）

如果知道对方的完整地址，可以直接连接。

```bash
# 节点 A 启动
go run ./examples/chat

# 节点 A 会显示连接地址：
# 🔗 连接地址（分享给其他人）:
#    /ip4/192.168.1.100/udp/9000/quic-v1/p2p/12D3KooWxxxxxxxx

# 节点 B 启动后，使用 /connect 命令连接
go run ./examples/chat
[me] /connect /ip4/192.168.1.100/udp/9000/quic-v1/p2p/12D3KooWxxxxxxxx
# ✅ 已连接 12D3KooW
```

---

### 场景 3：跨网络聊天（引导 + 中继）

在不同网络（如家里 WiFi、公司网络、4G 移动网络）之间聊天。

#### 步骤 1：在云服务器上启动基础设施节点

```bash
go run ./examples/chat \
    --serve \
    --port 4001 \
    --public-addr "/ip4/YOUR_PUBLIC_IP/udp/4001/quic-v1"
```

输出示例：
```
╔════════════════════════════════════════════════════════════╗
║           DeP2P Chat - Server Mode                         ║
╚════════════════════════════════════════════════════════════╝

服务模式: Bootstrap + Relay

🔗 连接地址（分享给其他人）:
   /ip4/1.2.3.4/udp/4001/quic-v1/p2p/12D3KooWxxxxxxxx
```

#### 步骤 2：在 WiFi 网络启动客户端

```bash
go run ./examples/chat \
    --bootstrap "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/12D3KooWxxxxxxxx" \
    --relay "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/12D3KooWxxxxxxxx"
```

#### 步骤 3：在 4G 网络启动客户端

```bash
go run ./examples/chat \
    --bootstrap "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/12D3KooWxxxxxxxx" \
    --relay "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/12D3KooWxxxxxxxx"
```

现在 WiFi 和 4G 上的节点可以通过引导节点发现彼此，通过中继进行通信。

---

## 命令参考

### 启动参数

```bash
go run ./examples/chat [选项]

Node 层配置（启动前）：
  --bootstrap <addr>      引导节点地址（逗号分隔多个）
  --relay <addr>          Relay 地址

服务能力开关：
  --serve                 服务模式：启用 Bootstrap + Relay 能力
  --public-addr <addr>    公网地址（--serve 时必需）

其他：
  --port <port>           监听端口（0 = 随机）
  --nick <name>           昵称
  --realm-key <key>       Realm PSK（相同密钥的节点加入同一 Realm）
```

### 运行时命令

| 命令 | 说明 |
|------|------|
| `<消息>` | 发送群聊到默认主题 |
| `/msg <ID> <消息>` | 私聊（ID 可只输入前几位） |
| `/connect <地址>` | 直接连接节点 |
| `/peers` | 查看在线成员 |
| `/status` | 查看网络状态 |
| `/sub <主题>` | 订阅新主题 |
| `/unsub <主题>` | 取消订阅主题 |
| `/topics` | 列出已订阅主题 |
| `/info` | 显示节点信息 |
| `/help` | 显示帮助 |
| `/quit` | 退出程序 |

### Relay 命令（Realm 层）

| 命令 | 说明 |
|------|------|
| `/gateway set <地址>` | 设置 Gateway |
| `/gateway remove` | 移除 Gateway |
| `/gateway enable` | 启用 Gateway 服务（需公网可达） |
| `/gateway disable` | 禁用 Gateway 服务 |
| `/relay status` | 查看 Relay 状态 |

---

## 详细场景说明

### 场景 A：两人在同一咖啡厅（局域网）

```
Alice:  go run ./examples/chat
Bob:    go run ./examples/chat

# 5秒后自动互相发现
# [系统背后：mDNS 在工作]

Alice: 你好！
Bob 看到: [chat/general][12D3KooW] Alice: 你好！
```

### 场景 B：朋友发来地址让我连接

```
Me:     go run ./examples/chat
[me] /connect /ip4/1.2.3.4/udp/4001/quic-v1/p2p/12D3KooWxxxxxxxx
# ✅ 已连接

# [系统背后：直接建立 QUIC 连接]
```

### 场景 C：4G 和 WiFi（都在 NAT 后）

```
# 云服务器（公网可达）
Server: go run ./examples/chat --serve --port 4001 --public-addr /ip4/公网IP/...

# 4G 用户
Me:     go run ./examples/chat \
            --bootstrap /ip4/公网IP/udp/4001/quic-v1/p2p/12D3KooW... \
            --relay /ip4/公网IP/udp/4001/quic-v1/p2p/12D3KooW...

# WiFi 用户
Friend: go run ./examples/chat \
            --bootstrap /ip4/公网IP/udp/4001/quic-v1/p2p/12D3KooW... \
            --relay /ip4/公网IP/udp/4001/quic-v1/p2p/12D3KooW...

# [系统背后：
#   1. 两人都连接 Server
#   2. 通过 Bootstrap 发现对方
#   3. 尝试直连（打洞）
#   4. 直连失败则通过 Relay 中继
# ]
```

### 场景 D：业务方自建 Gateway

```
# 业务方服务器启动
BizServer: go run ./examples/chat --serve --public-addr /ip4/...

# 加入 Realm 后启用 Relay 服务
[BizServer] /relay enable
# Relay 服务已启用

# 其他成员运行时设置 Relay
[Member] /relay set /ip4/业务服务器IP/udp/4001/quic-v1/p2p/12D3KooW...
# Relay 已设置

# [统一 Relay v2.0：
#   - 单一 Relay 服务，同时支持节点发现和业务消息转发
#   - 详见 design/_discussions/20260123-nat-relay-concept-clarification.md
# ]
```

---

## 网络状态说明

使用 `/status` 查看详细的网络状态：

```
╭────────────────────────────────────────────────────────────────╮
│ 📊 网络状态                                                     │
├────────────────────────────────────────────────────────────────┤
│ ─── Node 层 ───                                                │
│ mDNS:           ✅ 已启用（局域网发现）                          │
│ Bootstrap:      ✅ 已配置                                       │
│                    /ip4/1.2.3.4/udp/4001/quic-v1/p2p/12D3K...  │
│ Relay:          已配置                                           │
│                                                                │
│ ─── Realm 层 ───                                               │
│ Realm ID:       a1b2c3d4e5f6...                                │
│ Gateway:        未配置 (可用 /gateway set 设置)                 │
│ Relay 服务:     ❌ 未启用                                       │
│                                                                │
│ ─── 连接统计 ───                                                │
│ 已连接节点:     5                                               │
╰────────────────────────────────────────────────────────────────╯
```

---

## 架构图

```
                     ┌────────────────────────┐
                     │   云服务器（公网）      │
                     │   --serve 模式         │
                     │                        │
                     │   • Bootstrap 服务     │
                     │   • Relay 服务         │
                     │   • 可选参与聊天        │
                     └───────────┬────────────┘
                                 │
         ┌───────────────────────┼───────────────────────┐
         │                       │                       │
         ▼                       ▼                       ▼
┌─────────────────┐     ┌─────────────────┐     ┌─────────────────┐
│  WiFi 节点       │     │   4G 节点       │     │  办公网节点      │
│  (NAT 后)        │     │   (NAT 后)      │     │  (NAT 后)       │
│                 │     │                 │     │                 │
│ --bootstrap ... │     │ --bootstrap ... │     │ --bootstrap ... │
│ --relay ...     │     │ --relay ...     │     │ --relay ...     │
└────────┬────────┘     └────────┬────────┘     └────────┬────────┘
         │                       │                       │
         └───────────────────────┴───────────────────────┘
                         同一 Realm (PSK)
                         通过引导发现
                         通过中继通信
```

---

## 通信流程

```
Client A (WiFi)                    Server (公网)                    Client B (4G)
      │                                │                                │
      │──────── 连接 Server ──────────>│                                │
      │                                │<─────── 连接 Server ───────────│
      │                                │                                │
      │<──── Bootstrap: 发现 B ────────│                                │
      │                                │────── Bootstrap: 发现 A ──────>│
      │                                │                                │
      │═══════ 尝试直连 (打洞) ════════════════════════════════════════>│
      │                          [如果失败]                             │
      │═══════ 通过 Relay 中继 ═══════>│═══════ 转发消息 ══════════════>│
      │                                │                                │
```

---

## 常见问题

### Q: 节点启动后无法自动发现其他局域网节点？

检查：
1. 确认两台机器在同一局域网
2. 检查防火墙是否允许 UDP 5353（mDNS 端口）
3. 检查是否使用相同的 `--realm-key`

### Q: 跨网络无法连接？

检查：
1. 确认服务器有公网 IP 且端口已开放
2. 确认 `--bootstrap` 和 `--relay` 地址正确
3. 检查云服务商安全组/防火墙规则

### Q: 什么时候需要配置 Gateway？

- **Relay**（`--relay`）：统一 Relay v2.0，用于 NAT 穿透和消息转发
- **Gateway**（`/gateway set`）：Realm 内部网关，用于 Realm 成员间通信

大多数情况下，配置 `--relay` 即可。当业务方需要自建 Realm 网关时，使用 Gateway。

### Q: 没有公网服务器怎么测试跨网络？

方法 1：使用 ngrok/frp 等内网穿透工具
```bash
# 本地启动服务模式
go run ./examples/chat --serve --port 4001

# 使用 ngrok 暴露
ngrok tcp 4001
# 得到 tcp://0.tcp.ngrok.io:xxxxx

# 其他客户端使用 ngrok 地址
go run ./examples/chat \
    --bootstrap "/dns4/0.tcp.ngrok.io/tcp/xxxxx/p2p/NodeID"
```

方法 2：在同一局域网测试（验证功能，非跨网络）

---

## 运行时文件

```
examples/chat/data/
├── node-{pid}/         # 节点实例数据（按 PID 隔离）
│   └── dep2p.db/       # BadgerDB 数据库
└── logs/               # 日志文件
    └── chat-*.log
```

清理：
```bash
rm -rf examples/chat/data/
```

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [ADR-0009: Bootstrap 极简配置](../../design/01_context/decisions/ADR-0009-bootstrap-simplified.md) | Bootstrap 设计决策 |
| [ADR-0010: Relay 明确配置](../../design/01_context/decisions/ADR-0010-relay-explicit-config.md) | Relay 设计决策 |
| [pkg/interfaces/node.go](../../pkg/interfaces/node.go) | Node 接口定义 |
| [pkg/interfaces/realm.go](../../pkg/interfaces/realm.go) | Realm 接口定义 |
