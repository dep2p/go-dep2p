# Protocol Liveness 模块

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13  
> **定位**: 存活检测（Protocol Layer）

---

## 模块概述

protocol_liveness 提供节点存活检测能力，监控连接状态和节点健康。

| 属性 | 值 |
|------|-----|
| **架构层** | Protocol Layer |
| **代码位置** | `internal/protocol/liveness/` |
| **协议前缀** | `/dep2p/app/<realmID>/liveness/*` |
| **Fx 模块** | `fx.Module("protocol/liveness")` |
| **状态** | ✅ 已实现 |
| **依赖** | core_host, realm |

---

## 核心职责

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    protocol_liveness 职责                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 存活检测                                                                 │
│     • Ping/Pong 机制                                                        │
│     • RTT 测量                                                              │
│     • 连接健康检查                                                          │
│                                                                             │
│  2. 状态监控                                                                 │
│     • 节点在线状态                                                          │
│     • 连接质量评估                                                          │
│     • 超时检测                                                              │
│                                                                             │
│  3. Realm 成员监控                                                           │
│     • Realm 内成员存活检测                                                  │
│     • 成员离线通知                                                          │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 与系统 Ping 的区别

| 特性 | 系统 Ping (`/dep2p/sys/ping`) | Liveness (`/dep2p/app/<realmID>/liveness`) |
|------|-------------------------------|---------------------------------------------|
| 层级 | Core Layer | Protocol Layer |
| 作用域 | 全网任意节点 | Realm 内成员 |
| 认证 | 无需认证 | 需要 Realm 成员资格 |
| 用途 | 基础存活检测 | 业务级健康监控 |

---

## 公共接口

```go
// pkg/interfaces/liveness.go

// LivenessService 存活检测服务接口
type LivenessService interface {
    // Ping 发送 Ping 请求
    Ping(ctx context.Context, peer types.PeerID) (time.Duration, error)
    
    // IsAlive 检查节点是否存活
    IsAlive(peer types.PeerID) bool
    
    // Subscribe 订阅存活事件
    Subscribe() <-chan LivenessEvent
    
    // SetCheckInterval 设置检查间隔
    SetCheckInterval(interval time.Duration)
}

// LivenessEvent 存活事件
type LivenessEvent struct {
    Peer      types.PeerID
    Alive     bool
    RTT       time.Duration
    Timestamp time.Time
}
```

---

## 配置参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `CheckInterval` | 30s | 检查间隔 |
| `Timeout` | 10s | 单次检测超时 |
| `FailureThreshold` | 3 | 失败阈值（连续失败次数） |

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [core_protocol](../core_protocol/) | 系统 Ping 协议 |
| [realm](../realm/) | Realm 成员管理 |
| [L2: 五层架构](../../L2_structural/layer_model.md) | 架构定义 |

---

**最后更新**：2026-01-13
