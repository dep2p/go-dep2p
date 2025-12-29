# DeP2P Core 模块

## 概述

`internal/core/` 包含 dep2p 的核心组件实现，按层级组织，实现 `docs/01-design/` 中定义的协议规范。

## 设计引用

> **所有实现必须遵循 `docs/01-design/` 中的协议规范**

| 文档目录 | 说明 |
|----------|------|
| `docs/01-design/protocols/foundation/` | 基础协议（身份、地址、设备身份） |
| `docs/01-design/protocols/transport/` | 传输协议（QUIC、TLS、多路复用） |
| `docs/01-design/protocols/network/` | 网络协议（发现、NAT、中继、地址管理） |
| `docs/01-design/protocols/application/` | 应用协议（存活、连接管理、消息、Realm） |

## 模块层级

```
┌─────────────────────────────────────────────────────────────────────┐
│  Tier 4: 聚合层                                                      │
│  endpoint/ → 统一入口，聚合所有子模块                                 │
├─────────────────────────────────────────────────────────────────────┤
│  Tier 3: 网络服务层                                                  │
│  discovery/ → DHT、mDNS、Bootstrap                                   │
│  nat/       → STUN、UPnP、打洞                                       │
│  relay/     → 中继客户端、服务器                                     │
│  connmgr/   → 连接管理、节点存活                                     │
│  messaging/ → Request-Response、Pub-Sub                             │
├─────────────────────────────────────────────────────────────────────┤
│  Tier 2: 传输层                                                      │
│  transport/ → QUIC 传输                                              │
│  security/  → TLS 1.3 安全                                           │
│  muxer/     → 多路复用（Yamux 备用）                                 │
│  protocol/  → 协议协商与路由                                         │
├─────────────────────────────────────────────────────────────────────┤
│  Tier 1: 基础层                                                      │
│  identity/  → Ed25519 身份管理                                       │
│  address/   → 地址簿、地址格式                                       │
│  config/    → 配置管理                                               │
└─────────────────────────────────────────────────────────────────────┘
```

## 组件能力状态总览

### Tier 1: 基础层

| 组件 | 核心能力 | 状态 | 设计文档 |
|------|----------|------|----------|
| identity | Ed25519、NodeID、签名验证 | ✅ 已实现 | [身份协议](../../docs/01-design/protocols/foundation/01-identity.md) |
| identity | 设备身份、多密钥类型 | ✅ 已实现 | [设备身份协议](../../docs/01-design/protocols/foundation/03-device-id.md) |
| address | 地址簿、地址格式、签名记录 | ✅ 已实现 | [地址协议](../../docs/01-design/protocols/foundation/02-address.md) |
| config | 配置管理、预设 | ✅ 已实现 | - |

### Tier 2: 传输层

| 组件 | 核心能力 | 状态 | 设计文档 |
|------|----------|------|----------|
| transport | QUIC 连接、1-RTT | ✅ 已实现 | [传输协议](../../docs/01-design/protocols/transport/01-transport.md) |
| transport | 0-RTT 重连、连接迁移 | ✅ 已实现 | [传输协议](../../docs/01-design/protocols/transport/01-transport.md) |
| security | TLS 1.3、身份验证 | ✅ 已实现 | [安全协议](../../docs/01-design/protocols/transport/02-security.md) |
| security | 0-RTT 重连、Session 缓存 | ✅ 已实现 | [安全协议](../../docs/01-design/protocols/transport/02-security.md) |
| muxer | 流多路复用（QUIC/Yamux）、心跳保活 | ✅ 已实现 | [多路复用协议](../../docs/01-design/protocols/transport/03-muxer.md) |
| protocol | 协议路由、协商 | ✅ 已实现 | - |

### Tier 3: 网络服务层

| 组件 | 核心能力 | 状态 | 设计文档 |
|------|----------|------|----------|
| discovery | DHT、mDNS、Bootstrap、Rendezvous、DNS | ✅ 已实现 | [发现协议](../../docs/01-design/protocols/network/01-discovery.md) |
| discovery | 动态发现间隔、Private Realm | ✅ 已实现 | [节点存活协议](../../docs/01-design/protocols/application/01-node-liveness.md) |
| nat | STUN、UPnP、NAT-PMP、UDP/TCP 打洞 | ✅ 已实现 | [NAT 穿透协议](../../docs/01-design/protocols/network/02-nat.md) |
| relay | 中继客户端、服务器、AutoRelay | ✅ 已实现 | [中继协议](../../docs/01-design/protocols/network/03-relay.md) |
| connmgr | 水位线、保护、裁剪、黑名单、抖动容错 | ✅ 已实现 | [连接管理协议](../../docs/01-design/protocols/application/02-connection-manager.md) |
| liveness | 节点存活、Goodbye、心跳、健康评分 | ✅ 已实现 | [节点存活协议](../../docs/01-design/protocols/application/01-node-liveness.md) |
| messaging | Request-Response、Pub-Sub (GossipSub) | ✅ 已实现 | [消息传递协议](../../docs/01-design/protocols/application/03-messaging.md) |
| messaging | Realm 感知、Query | ✅ 已实现 | [Realm 协议](../../docs/01-design/protocols/application/04-realm.md) |

### Tier 4: 聚合层

| 组件 | 核心能力 | 状态 | 设计文档 |
|------|----------|------|----------|
| endpoint | 统一 API 入口 | ✅ 已实现 | - |
| endpoint | Realm 支持 | ✅ 已实现 | [Realm 协议](../../docs/01-design/protocols/application/04-realm.md) |
| realm | Realm 管理、RealmAuth、强制隔离 | ✅ 已实现 | [Realm 协议](../../docs/01-design/protocols/application/04-realm.md) |

## fx 模块集成

每个组件提供 `Module()` 函数用于 fx 依赖注入：

```go
// internal/app/bootstrap.go
func (b *Bootstrap) setupModules() ([]fx.Option, error) {
    return []fx.Option{
        // Tier 1: 基础层
        config.Module(),
        identity.Module(),
        address.Module(),
        
        // Tier 2: 传输层
        transport.Module(),
        security.Module(),
        muxer.Module(),
        protocol.Module(),
        
        // Tier 3: 网络服务层
        nat.Module(),
        discovery.Module(),
        relay.Module(),
        connmgr.Module(),
        messaging.Module(),
        
        // Tier 4: 聚合层
        endpoint.Module(),
    }, nil
}
```

## 迭代跟踪

实施进度和迭代记录见: [docs/05-iterations/](../../docs/05-iterations/)

## 相关文档

- [设计文档](../../docs/01-design/)
- [实施映射](../../design/implementation/)
- [公共接口](../../pkg/interfaces/)
