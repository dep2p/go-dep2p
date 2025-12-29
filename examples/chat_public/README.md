# DeP2P 公网 Chat 示例 (v3)

这是一个**公网 P2P 聊天示例**，演示 dep2p 在真实互联网环境中的使用。

## v3 核心特性

| 特性 | 实现 | 说明 |
|------|------|------|
| **群聊** | GossipSub (PubSub) | 消息自动广播给所有订阅者 |
| **私聊** | 点对点 Stream | 消息仅对方可见 |
| **NAT 穿透** | Relay Transport | 直连失败时透明回退 |
| **成员发现** | 混合模式 | Seed Bootstrap + PubSub 自动发现 |

## 架构图

```
┌─────────────────────────────────────────────────────────────────────┐
│                      chat_public v3 架构                             │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌─────────────┐       ┌─────────────┐       ┌─────────────┐        │
│  │   Alice     │       │    Seed     │       │    Bob      │        │
│  │   (NAT)     │       │   (公网)    │       │   (NAT)     │        │
│  └──────┬──────┘       └──────┬──────┘       └──────┬──────┘        │
│         │                     │                     │               │
│         │   1. Bootstrap      │   1. Bootstrap      │               │
│         ├────────────────────►│◄────────────────────┤               │
│         │                     │                     │               │
│         │   2. Relay 预留     │   2. Relay 预留     │               │
│         ├────────────────────►│◄────────────────────┤               │
│         │                     │                     │               │
│         │◄═══════════════ GossipSub Mesh ═══════════════►│          │
│         │        （群聊：自动广播，透明使用 Relay）        │          │
│         │                                                 │          │
│         │◄─────────────── Private Stream ────────────────►│          │
│         │        （私聊：点对点，透明使用 Relay）          │          │
│                                                                      │
│  通信模式：                                                          │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │ 群聊: node.Publish("chat-room:xxx", msg)                     │   │
│  │       → GossipSub 自动广播给所有订阅者                       │   │
│  │       → 透明使用 Relay Transport（若直连失败）               │   │
│  ├──────────────────────────────────────────────────────────────┤   │
│  │ 私聊: conn.OpenStream(privateProtocol)                       │   │
│  │       → 点对点 Stream，仅对方可见                            │   │
│  │       → 自动通过 Relay 建立连接（若直连失败）                │   │
│  └──────────────────────────────────────────────────────────────┘   │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

## 快速开始（三节点）

### 1. 启动 Seed 节点（VPS 上）

```bash
# 在公网 VPS 上运行（确保端口 4001/UDP 对外开放）
go run main.go -mode seed -port 4001
```

输出示例：
```
╔══════════════════════════════════════════════════════╗
║   DeP2P Chat Public v3                               ║
║   PubSub 群聊 + Stream 私聊 + Relay 透明回退          ║
╚══════════════════════════════════════════════════════╝

════════════════════════════════════════════════════════
        🌐 SEED 模式（Bootstrap + Relay Server）
════════════════════════════════════════════════════════

✅ Seed 可分享地址:
   /ip4/101.37.245.124/udp/4001/quic-v1/p2p/5Q2STWvBFnXyz...

✅ 已订阅群聊 Topic

Seed 已就绪，等待 Peer 连接...
```

### 2. 启动 Alice（本地机器 A）

```bash
# 复制 seed 输出的 Full Address
go run main.go -mode peer \
  -seed "/ip4/101.37.245.124/udp/4001/quic-v1/p2p/5Q2STWvB..." \
  -name alice
```

### 3. 启动 Bob（本地机器 B）

```bash
# 使用相同的 seed Full Address
go run main.go -mode peer \
  -seed "/ip4/101.37.245.124/udp/4001/quic-v1/p2p/5Q2STWvB..." \
  -name bob
```

### 4. 开始聊天

**群聊**（所有人可见）：
```
[alice] 大家好！
```

**私聊**（仅对方可见）：
```
[alice] /msg bob 这是一条私密消息
[私聊 → bob] 这是一条私密消息
```

## 命令参考

### 启动参数

| 参数 | 说明 | 示例 |
|------|------|------|
| `-mode` | 运行模式 | `seed` 或 `peer` |
| `-port` | 监听端口 | `4001`（seed 建议固定） |
| `-seed` | Seed 的 Full Address | peer 模式必填 |
| `-name` | 昵称 | `alice` |
| `-realm` | 聊天室名称 | 默认 `public-chat` |

### 交互命令

| 命令 | 说明 | 示例 |
|------|------|------|
| `<直接输入>` | 发送群聊消息 | `大家好！` |
| `/msg <昵称> <消息>` | 发送私聊消息 | `/msg bob 私密消息` |
| `/peers` | 列出在线成员 | |
| `/info` | 显示本节点信息 | |
| `/help` | 显示帮助 | |
| `/quit` | 退出程序 | |

## 技术细节

### 群聊机制（GossipSub）

```go
// 发送群聊消息
node.Publish(ctx, "chat-room:public-chat", messageData)

// 订阅群聊
sub, _ := node.Subscribe(ctx, "chat-room:public-chat")
for msg := range sub.Messages() {
    // 自动收到所有群聊消息
}
```

**特点**：
- 使用 GossipSub v1.1 协议
- 自动维护 mesh 网络
- O(log N) 轮传播，带宽效率高
- 透明支持 Relay Transport（NAT 节点间可通过 Relay 传递消息）

### 私聊机制（Stream）

```go
// 建立连接（自动 Relay 回退）
conn, _ := node.Connect(ctx, targetNodeID)

// 打开私聊流
stream, _ := conn.OpenStream(ctx, "/dep2p/chat/private/1.0.0")

// 发送私聊消息
stream.Write(messageData)
```

**特点**：
- 点对点直接通信
- 消息仅对方可见
- 直连失败时自动通过 Relay Transport

### 成员发现（混合模式）

1. **Seed Bootstrap**：Peer 连接 Seed 获取 Relay 预留
2. **Join 广播**：新成员通过 PubSub 广播 join 消息
3. **昵称学习**：通过消息自动学习其他成员的昵称

### NAT 穿透（Relay Transport）

```
┌───────────────────────────────────────────────────────────────┐
│                    Relay Transport 工作原理                   │
├───────────────────────────────────────────────────────────────┤
│                                                               │
│  Alice (NAT)              Seed (公网)              Bob (NAT)  │
│      │                        │                        │      │
│      │──── Relay 预留 ───────►│◄────── Relay 预留 ────│      │
│      │                        │                        │      │
│      │◄═══════════════ Relay 连接 ═══════════════════►│      │
│      │                        │                        │      │
│      │        GossipSub / Private Stream               │      │
│      │◄═══════════════════════════════════════════════►│      │
│                                                               │
│  关键点：                                                     │
│  • GossipSub 使用 endpoint.Connection(peer) 获取连接         │
│  • 连接类型（直连/Relay）对 GossipSub 透明                    │
│  • 只要有连接，PubSub 消息就能传递                           │
│                                                               │
└───────────────────────────────────────────────────────────────┘
```

## 版本演进

| 版本 | 群聊实现 | 成员发现 | 私聊 | NAT 支持 |
|------|----------|----------|------|----------|
| v1 | Seed 转发 | Seed 通知 | 无 | 部分 |
| v2 | Stream 逐个写 | Seed 通知 | 无 | Relay Transport |
| **v3** | **GossipSub** | **混合模式** | **Stream** | **Relay Transport** |

### v3 相比 v2 的改进

1. **群聊效率**：从 O(N) 逐个写变为 GossipSub O(log N) 传播
2. **私聊支持**：新增点对点私聊能力
3. **去中心化**：Seed 仅做 Bootstrap，不参与消息转发
4. **自动发现**：通过 PubSub join 消息自动学习成员

## 故障诊断

### Seed: 无法获取可分享地址

**检查步骤**：
1. 确认在公网 VPS 上运行
2. 检查端口是否开放：`sudo ufw allow 4001/udp`
3. 检查云厂商安全组配置

### Peer: 连接 Seed 失败

**检查步骤**：
1. Full Address 是否完整复制（很长，注意不要截断）
2. Seed 节点是否仍在运行
3. 网络是否能到达 Seed

### 私聊: 未找到用户

```
❌ 私聊失败: 未找到用户 'xxx'
```

**原因**：对方尚未发送过消息，昵称未被学习。

**解决**：等待对方发送一条群聊消息，或使用完整 NodeID。

## 相关文档

- [Relay 穿透示例](../relay/)
- [GossipSub 实现](../../internal/core/messaging/gossipsub/README.md)
- [Relay Transport 设计](../../docs/05-iterations/2025-12-26-relay-transport-integration.md)
- [设计文档](../../docs/01-design/)
