# Discovery mDNS 模块

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13  
> **定位**: 局域网发现（Discovery Layer）

---

## 模块概述

discovery_mdns 使用多播 DNS 协议进行局域网内的节点自动发现，无需中心服务器。

| 属性 | 值 |
|------|-----|
| **架构层** | Discovery Layer |
| **代码位置** | `internal/discovery/mdns/` |
| **Fx 模块** | `fx.Module("discovery/mdns")` |
| **状态** | ✅ 已实现 |
| **依赖** | core_host |

---

## 核心职责

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    discovery_mdns 职责                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 服务广播                                                                 │
│     • 局域网节点广播                                                        │
│     • 服务注册                                                              │
│     • 定期广播刷新                                                          │
│                                                                             │
│  2. 服务发现                                                                 │
│     • 局域网节点发现                                                        │
│     • 服务查询                                                              │
│     • 自动节点检测                                                          │
│                                                                             │
│  3. 地址解析                                                                 │
│     • mDNS 响应解析                                                         │
│     • 地址验证                                                              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 公共接口

```go
// pkg/interfaces/discovery.go (mDNS 部分)

// MDNSDiscovery mDNS 发现接口
type MDNSDiscovery interface {
    Discovery
    
    // RegisterService 注册服务
    RegisterService(serviceTag string) error
    
    // UnregisterService 取消注册
    UnregisterService() error
}
```

---

## 使用场景

- 开发和测试环境
- 无互联网连接的私有网络
- 快速节点发现
- 局域网 P2P 应用

---

## 配置参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `ServiceTag` | `_dep2p._udp` | mDNS 服务标签 |
| `Interval` | 10s | 广播间隔 |
| `Enabled` | true | 是否启用 |

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [discovery_coordinator](../discovery_coordinator/) | 发现协调器 |
| [L2: 五层架构](../../L2_structural/layer_model.md) | 架构定义 |

---

**最后更新**：2026-01-13
