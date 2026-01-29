# Core Protocol 模块

> **版本**: v1.2.0  
> **更新日期**: 2026-01-23  
> **定位**: 协议注册与路由（Core Layer）

---

## 模块概述

core_protocol 负责协议的注册、协商和路由。它管理所有协议处理器，并包含系统协议的实现。

| 属性 | 值 |
|------|-----|
| **架构层** | Core Layer |
| **代码位置** | `internal/core/protocol/` |
| **Fx 模块** | `fx.Module("protocol")` |
| **状态** | ✅ 已实现 |
| **依赖** | host, peerstore, swarm |

---

## 核心职责

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       core_protocol 职责                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 协议注册 (Registry)                                                      │
│     • 注册协议 ID 与处理器映射                                              │
│     • 协议版本管理                                                          │
│     • 协议启用/禁用                                                         │
│                                                                             │
│  2. 协议路由 (Router)                                                        │
│     • 根据协议 ID 路由入站流                                                │
│     • 协议优先级排序                                                        │
│     • 通配符匹配                                                            │
│                                                                             │
│  3. 协议协商 (Negotiator)                                                    │
│     • multistream-select 协商                                               │
│     • 协议能力交换                                                          │
│                                                                             │
│  4. 系统协议实现                                                             │
│     • Identify (/dep2p/sys/identify/1.0.0)                                 │
│     • Ping (/dep2p/sys/ping/1.0.0)                                         │
│     • AutoNAT (/dep2p/sys/autonat/1.0.0)                                   │
│     • HolePunch (/dep2p/sys/holepunch/1.0.0)                               │
│     • Relay (/dep2p/relay/1.0.0/{hop,stop})                                │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 协议分类

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         DeP2P 协议分类                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ 系统协议 (System Protocols)                                          │   │
│  │ 前缀: /dep2p/sys/*                                                   │   │
│  │ 位置: internal/core/protocol/system/                                 │   │
│  ├─────────────────────────────────────────────────────────────────────┤   │
│  │ • /dep2p/sys/identify/1.0.0      │ 身份识别                         │   │
│  │ • /dep2p/sys/identify/push/1.0.0 │ 身份推送                         │   │
│  │ • /dep2p/sys/ping/1.0.0          │ 存活检测                         │   │
│  │ • /dep2p/sys/autonat/1.0.0       │ NAT 检测                         │   │
│  │ • /dep2p/sys/holepunch/1.0.0     │ NAT 打洞                         │   │
│  │ • /dep2p/relay/1.0.0/hop         │ Relay HOP                        │   │
│  │ • /dep2p/relay/1.0.0/stop        │ Relay STOP                       │   │
│  │ • /dep2p/sys/dht/1.0.0           │ DHT                              │   │
│  │ • /dep2p/sys/rendezvous/1.0.0    │ Rendezvous                       │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ Realm 协议 (Realm Protocols)                                         │   │
│  │ 前缀: /dep2p/realm/<realmID>/*                                       │   │
│  │ 位置: internal/realm/protocol/                                       │   │
│  ├─────────────────────────────────────────────────────────────────────┤   │
│  │ • /dep2p/realm/<id>/join/1.0.0   │ 加入域                           │   │
│  │ • /dep2p/realm/<id>/auth/1.0.0   │ 域认证                           │   │
│  │ • /dep2p/realm/<id>/sync/1.0.0   │ 成员同步                         │   │
│  │ • /dep2p/realm/<id>/route/1.0.0  │ 域内路由                         │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
│  ┌─────────────────────────────────────────────────────────────────────┐   │
│  │ 应用协议 (Application Protocols)                                     │   │
│  │ 前缀: /dep2p/app/<realmID>/*                                         │   │
│  │ 位置: internal/protocol/                                             │   │
│  ├─────────────────────────────────────────────────────────────────────┤   │
│  │ • /dep2p/app/<id>/messaging/1.0.0│ 请求/响应                        │   │
│  │ • /dep2p/app/<id>/pubsub/1.0.0   │ 发布/订阅                        │   │
│  │ • /dep2p/app/<id>/streams/1.0.0  │ 双向流                           │   │
│  │ • /dep2p/app/<id>/liveness/1.0.0 │ 存活检测                         │   │
│  └─────────────────────────────────────────────────────────────────────┘   │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 系统协议实现

### 目录结构

```
internal/core/protocol/
├── doc.go
├── module.go           # Fx 模块入口
├── registry.go         # 协议注册表
├── router.go           # 协议路由器
├── negotiator.go       # 协议协商
│
└── system/             # 系统协议实现
    ├── identify/       # 身份识别
    │   ├── identify.go
    │   └── push.go
    ├── ping/           # Ping
    │   └── ping.go
    ├── autonat/        # AutoNAT
    │   ├── client.go
    │   └── server.go
    ├── holepunch/      # 打洞
    │   ├── holepunch.go
    │   └── coordinator.go
    └── relay/          # 中继 (Circuit v2)
        ├── protocol.go
        ├── client/
        └── server/
```

---

## 公共接口

```go
// pkg/interfaces/protocol.go

// ProtocolRouter 协议路由器接口
type ProtocolRouter interface {
    // SetStreamHandler 设置协议处理器
    SetStreamHandler(proto types.ProtocolID, handler StreamHandler)
    
    // SetStreamHandlerMatch 设置模式匹配处理器
    SetStreamHandlerMatch(proto types.ProtocolID, match func(types.ProtocolID) bool, handler StreamHandler)
    
    // RemoveStreamHandler 移除协议处理器
    RemoveStreamHandler(proto types.ProtocolID)
    
    // Protocols 返回支持的协议列表
    Protocols() []types.ProtocolID
}

// StreamHandler 流处理函数
type StreamHandler func(Stream)

// ProtocolNegotiator 协议协商器接口
type ProtocolNegotiator interface {
    // Negotiate 协商协议
    Negotiate(ctx context.Context, conn Connection, protos ...types.ProtocolID) (types.ProtocolID, error)
}
```

---

## 系统协议说明

### Identify

```
功能: 交换节点身份信息
协议: /dep2p/sys/identify/1.0.0

交换内容:
• PeerID
• 公钥
• 监听地址
• 观测地址 (ObservedAddr)
• 支持的协议列表
• Agent 版本
```

### ★ ObservedAddr 可信性说明（关键）

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    ObservedAddr 可信性分析（★ 必读）                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  什么是 ObservedAddr？                                                       │
│  ═══════════════════                                                        │
│  Identify 协议中，对端告知的"我看到的你的地址"                               │
│  例如：Alice 连接 Bob 时，Bob 告诉 Alice "你的外部地址是 1.2.3.4:5678"      │
│                                                                             │
│  ⚠️ 可信性警告：ObservedAddr 仅为候选地址！                                 │
│  ══════════════════════════════════════════                                 │
│                                                                             │
│  风险：                                                                      │
│  • 恶意节点：可能故意告知错误地址                                           │
│  • 错误配置：对端可能在代理或负载均衡后                                     │
│  • NAT 映射过期：告知的地址可能已失效                                       │
│  • 私网地址：对端与本节点在同一 NAT 后时，报告的是私网地址                  │
│                                                                             │
│  正确使用方式：                                                              │
│  ═════════════                                                              │
│                                                                             │
│  1. 标记为候选地址（Candidate）                                             │
│     ─────────────────────────────                                           │
│     存入 Peerstore 时，设置 TTL = CandidateAddrTTL（3 分钟）                │
│     不能直接作为可达地址使用                                                │
│                                                                             │
│  2. 多源验证                                                                 │
│     ──────────                                                              │
│     至少 2 个不同节点报告相同地址时，提升可信度                              │
│     不同子网的节点报告相同地址 = 更可信                                     │
│                                                                             │
│  3. Reachability 验证                                                       │
│     ─────────────────                                                       │
│     最终必须通过 AutoNAT/dialback 验证后才能升级为 Verified                 │
│     验证通过后才能发布到 DHT/Relay 地址簿                                   │
│                                                                             │
│  ❌ 禁止行为：                                                               │
│  • 直接将 ObservedAddr 发布到 DHT                                           │
│  • 将单一来源的 ObservedAddr 当作可达地址                                   │
│  • 不验证直接广播给其他节点                                                 │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### Ping

```
功能: 存活检测和 RTT 测量
协议: /dep2p/sys/ping/1.0.0

消息格式:
• 32 字节随机数据
• 服务端回显
• 测量往返时间
```

### AutoNAT

```
功能: NAT 类型自动检测
协议: /dep2p/sys/autonat/1.0.0

工作流程:
1. 客户端请求 Dial 回调
2. 服务端尝试连接客户端声明的地址
3. 返回连接结果
4. 客户端判断 NAT 类型
```

### HolePunch

```
功能: NAT 打洞协调
协议: /dep2p/sys/holepunch/1.0.0

工作流程:
1. 通过中继建立协调通道
2. 交换各自的外部地址
3. 同时拨号 (Simultaneous Open)
4. 建立直连
```

### Relay (Circuit v2)

```
功能: 中继转发服务
协议: /dep2p/relay/1.0.0/{hop,stop}

特性:
• 预约制资源管理
• 流量和时间限制
• 凭证验证
```

---

## 参考实现

### go-libp2p 系统协议

```
github.com/libp2p/go-libp2p/p2p/protocol/
├── identify/         # 身份识别
├── ping/            # Ping
├── autonatv2/       # AutoNAT v2
├── holepunch/       # 打洞
└── circuitv2/       # 中继 v2
```

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [core_host](../core_host/) | 主机服务 |
| [core_swarm](../core_swarm/) | 连接群管理 |
| [core_reachability](../core_reachability/) | 地址可达性验证 |
| [core_peerstore](../core_peerstore/) | 节点地址存储 |
| [protocol_messaging](../protocol_messaging/) | 应用消息协议 |
| [L2: 五层架构](../../L2_structural/layer_model.md) | 架构定义 |
| [02_constraints/protocol](../../../02_constraints/protocol/) | 协议规范 |

---

**最后更新**：2026-01-23
