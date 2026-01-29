# F2: 传输层需求

> 定义 DeP2P 的传输协议、连接管理和流多路复用能力

---

## 需求列表

| ID | 标题 | 优先级 | 状态 | 进展 |
|----|------|--------|------|------|
| [REQ-TRANS-001](REQ-TRANS-001.md) | QUIC 传输 | P0 | implementing | QUIC KeepAlive 已修复 |
| [REQ-TRANS-002](REQ-TRANS-002.md) | 连接管理 | P0 | implementing | 多层保活策略已设计 |
| [REQ-TRANS-003](REQ-TRANS-003.md) | 流多路复用 | P1 | implementing | Stream 半关闭已修复 |

---

## 核心设计

- **QUIC 优先**：以 QUIC 为核心传输协议
- **MagicSock**：借鉴 iroh 的多路径抽象
- **自动升级**：从 Relay 升级到直连
- **★ 多层保活**：QUIC KeepAlive + Liveness 心跳 + GossipSub 心跳
- **★ Stream 可靠性**：半关闭、超时控制、状态管理

---

## 2026-01-18 重要修复

### QUIC 连接保活问题 (✅ 已修复)

**问题**：QUIC 连接在 30 秒无通信后自动断开
**原因**：缺少 `KeepAlivePeriod` 配置
**修复**：添加 `KeepAlivePeriod = IdleTimeout / 2`

### Stream 半关闭能力 (✅ 已修复)

**问题**：跨机器私聊失败，接收方报 EOF
**原因**：`Stream` 接口缺少 `CloseWrite()` 方法
**修复**：扩展 Stream 接口，添加半关闭方法

---

## 关键竞品参考

| 竞品 | 特点 | DeP2P 借鉴 |
|------|------|------------|
| iroh | MagicSock、QUIC 优先 | 传输抽象、路径选择 |
| libp2p | 多传输、可插拔 | 连接管理模式 |
| **旧 dep2p** | **多层保活、Stream 设计** | **KeepAlive、半关闭** |

---

**最后更新**：2026-01-18
