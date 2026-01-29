# 配置项参考

本文档详细说明 DeP2P 的所有配置选项。

---

## 概述

DeP2P 使用函数式选项模式进行配置：

```go
node, err := dep2p.New(ctx,
    dep2p.WithPreset(dep2p.PresetDesktop),
    dep2p.WithListenPort(4001),
    dep2p.WithKnownPeers(config.KnownPeer{...}),
)
```

---

## 配置分类

```
┌─────────────────────────────────────────────────────────────────────────┐
│                        配置分类                                          │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                         │
│  基础配置           连接配置           发现配置           NAT 配置       │
│  ├─ WithPreset     ├─ ConnectionLimits ├─ KnownPeers     ├─ WithNAT    │
│  ├─ WithIdentity   ├─ ConnectionTimeout├─ BootstrapPeers ├─ AutoNAT    │
│  └─ WithListenPort └─ IdleTimeout      ├─ WithMDNS       ├─ STUNServers│
│                                        └─ WithDHT        └─ TrustSTUN  │
│                                                                         │
│  中继配置           Realm 配置         断开检测配置                      │
│  ├─ WithRelay      ├─ RealmAuth        ├─ QUIC Keep-Alive              │
│  ├─ RelayServer    └─ AuthTimeout      ├─ ReconnectGrace               │
│  └─ RelayMap                           ├─ Witness                      │
│                                        └─ Flapping                     │
│                                                                         │
└─────────────────────────────────────────────────────────────────────────┘
```

---

## 基础配置

### WithPreset

使用预设配置。

```go
func WithPreset(preset Preset) Option
```

| 预设 | 描述 | 连接数 | mDNS |
|------|------|--------|------|
| `PresetMinimal` | 最小配置，测试用 | 10/20 | ❌ |
| `PresetDesktop` | 桌面应用（默认） | 50/100 | ✅ |
| `PresetServer` | 服务器 | 200/500 | ✅ |
| `PresetMobile` | 移动端 | 20/50 | ✅ |

**示例**：

```go
node, _ := dep2p.New(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
```

---

### WithIdentity

设置节点身份。

```go
func WithIdentity(identity crypto.PrivKey) Option
```

**说明**：
- 如果不设置，自动生成新身份
- 相同私钥产生相同 NodeID

**示例**：

```go
// 从文件加载
node, _ := dep2p.New(ctx, dep2p.WithIdentityFile("./node.key"))

// 使用现有私钥
key, _ := dep2p.LoadIdentity("node.key")
node, _ := dep2p.New(ctx, dep2p.WithIdentity(key))
```

---

### WithListenPort

设置监听端口。

```go
func WithListenPort(port int) Option
```

**说明**：
- 默认使用随机端口
- 端口 0 表示随机分配

**示例**：

```go
node, _ := dep2p.New(ctx, dep2p.WithListenPort(4001))
```

---

## 发现配置

### WithKnownPeers ⭐

设置已知节点列表。

```go
func WithKnownPeers(peers ...config.KnownPeer) Option
```

**说明**：
- 启动时直接连接这些节点
- 不依赖 Bootstrap 或 DHT 发现
- 适用于私有网络、云服务器部署

**KnownPeer 结构**：

```go
type KnownPeer struct {
    PeerID string   `json:"peer_id"` // 节点 Peer ID
    Addrs  []string `json:"addrs"`   // 地址列表
}
```

**示例**：

```go
import "github.com/dep2p/go-dep2p/config"

node, _ := dep2p.New(ctx,
    dep2p.WithPreset(dep2p.PresetDesktop),
    dep2p.WithKnownPeers(
        config.KnownPeer{
            PeerID: "12D3KooWxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
            Addrs:  []string{"/ip4/1.2.3.4/udp/4001/quic-v1"},
        },
        config.KnownPeer{
            PeerID: "12D3KooWyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy",
            Addrs:  []string{"/ip4/5.6.7.8/udp/4001/quic-v1"},
        },
    ),
)
```

**JSON 配置**：

```json
{
  "known_peers": [
    {
      "peer_id": "12D3KooWxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
      "addrs": ["/ip4/1.2.3.4/udp/4001/quic-v1"]
    }
  ]
}
```

**与 Bootstrap 的区别**：

| 特性 | known_peers | Bootstrap |
|------|-------------|-----------|
| 用途 | 直接连接 | DHT 引导 |
| 连接时机 | 启动即连接 | DHT 初始化后 |
| 依赖 | 仅目标节点在线 | Bootstrap 服务 |
| 适用场景 | 私有网络、固定节点 | 公共网络 |

---

### WithBootstrapPeers

设置引导节点。

```go
func WithBootstrapPeers(addrs ...string) Option
```

**说明**：
- 使用完整地址格式（含 /p2p/<NodeID>）
- 用于 DHT 引导和节点发现

**示例**：

```go
bootstrapAddrs := []string{
    "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/12D3KooWxxxxx...",
    "/dns4/bootstrap.example.com/udp/4001/quic-v1/p2p/12D3KooWxxxxx...",
}
node, _ := dep2p.New(ctx, dep2p.WithBootstrapPeers(bootstrapAddrs...))
```

---

### WithMDNS

启用 mDNS 本地发现。

```go
func WithMDNS(enabled bool) Option
```

**说明**：
- 仅在局域网内有效
- PresetDesktop/Server/Mobile 默认启用

---

### WithDHT

配置 DHT 模式。

```go
func WithDHT(mode DHTMode) Option
```

| 模式 | 描述 |
|------|------|
| `DHTClient` | 仅客户端模式 |
| `DHTServer` | 服务器模式（参与路由） |
| `DHTAuto` | 自动模式 |

---

## NAT 配置

### WithTrustSTUNAddresses ⭐

信任 STUN 探测发现的公网地址。

```go
func WithTrustSTUNAddresses(enabled bool) Option
```

**说明**：
- 适用于云服务器场景
- 跳过入站连接验证
- 加速地址发布

**适用场景**：
- 云服务器有真实公网 IP
- 网络配置确保入站流量可达

**示例**：

```go
// 云服务器场景
node, _ := dep2p.New(ctx,
    dep2p.WithPreset(dep2p.PresetServer),
    dep2p.WithTrustSTUNAddresses(true),
)
```

**JSON 配置**：

```json
{
  "reachability": {
    "trust_stun_addresses": true
  }
}
```

---

### WithSTUNServers

设置 STUN 服务器。

```go
func WithSTUNServers(servers ...string) Option
```

**示例**：

```go
node, _ := dep2p.New(ctx,
    dep2p.WithSTUNServers(
        "stun.l.google.com:19302",
        "stun.cloudflare.com:3478",
    ),
)
```

---

### WithNAT

启用 NAT 穿透。

```go
func WithNAT(enabled bool) Option
```

---

### WithHolePunching

启用打洞。

```go
func WithHolePunching(enabled bool) Option
```

---

## 断开检测配置 ⭐

### DisconnectDetection

配置断开检测参数。

**JSON 配置**：

```json
{
  "disconnect_detection": {
    "quic": {
      "keep_alive_period": "3s",
      "max_idle_timeout": "6s"
    },
    "reconnect_grace_period": "15s",
    "disconnect_protection": "30s",
    "witness": {
      "enabled": true,
      "count": 3,
      "quorum": 2,
      "timeout": "5s"
    },
    "flapping": {
      "enabled": true,
      "window": "60s",
      "threshold": 3,
      "cooldown": "120s"
    }
  }
}
```

### 参数说明

#### QUIC 配置

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `keep_alive_period` | duration | `3s` | Keep-Alive 探测间隔 |
| `max_idle_timeout` | duration | `6s` | 最大空闲超时 |

#### 重连配置

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `reconnect_grace_period` | duration | `15s` | 重连宽限期，期间不触发 MemberLeft |
| `disconnect_protection` | duration | `30s` | 断开保护期 |

#### 见证人配置

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `witness.enabled` | bool | `true` | 启用见证人机制 |
| `witness.count` | int | `3` | 见证报告数量 |
| `witness.quorum` | int | `2` | 确认所需最小见证人数 |
| `witness.timeout` | duration | `5s` | 见证报告超时 |

#### 震荡检测配置

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `flapping.enabled` | bool | `true` | 启用震荡检测 |
| `flapping.window` | duration | `60s` | 检测窗口 |
| `flapping.threshold` | int | `3` | 触发震荡判定的断线次数 |
| `flapping.cooldown` | duration | `120s` | 震荡后冷却时间 |

### 场景推荐配置

| 场景 | keep_alive | max_idle | grace_period |
|------|------------|----------|--------------|
| 稳定网络 | 3s | 6s | 10s |
| 移动网络 | 5s | 10s | 30s |
| 实时游戏 | 1s | 3s | 5s |
| 后台同步 | 10s | 30s | 60s |

---

## 中继配置

### WithRelay

启用中继客户端。

```go
func WithRelay(enabled bool) Option
```

---

### WithRelayServer

启用中继服务器。

```go
func WithRelayServer(enabled bool) Option
```

**说明**：
- 作为中继节点为其他节点提供服务
- 需要公网 IP

---

## 连接配置

### WithConnectionLimits

设置连接数限制。

```go
func WithConnectionLimits(low, high int) Option
```

| 参数 | 描述 |
|------|------|
| `low` | 低水位（不主动裁剪） |
| `high` | 高水位（开始裁剪） |

---

### WithConnectionTimeout

设置连接超时。

```go
func WithConnectionTimeout(d time.Duration) Option
```

---

### WithIdleTimeout

设置空闲连接超时。

```go
func WithIdleTimeout(d time.Duration) Option
```

---

## 完整配置示例

### 云服务器场景

```json
{
  "preset": "server",
  "listen_port": 4001,
  
  "identity": {
    "key_file": "/etc/dep2p/identity.key"
  },
  
  "known_peers": [
    {
      "peer_id": "12D3KooWxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
      "addrs": ["/ip4/peer1.example.com/udp/4001/quic-v1"]
    }
  ],
  
  "reachability": {
    "trust_stun_addresses": true
  },
  
  "connection_limits": {
    "low": 100,
    "high": 500
  },
  
  "disconnect_detection": {
    "quic": {
      "keep_alive_period": "3s",
      "max_idle_timeout": "6s"
    },
    "reconnect_grace_period": "15s",
    "witness": {
      "enabled": true,
      "count": 3,
      "quorum": 2
    }
  },
  
  "relay": {
    "enable": true,
    "enable_server": true
  }
}
```

### 私有集群场景

```go
node, _ := dep2p.New(ctx,
    dep2p.WithPreset(dep2p.PresetDesktop),
    dep2p.WithListenPort(4001),
    dep2p.WithIdentityFile("./node.key"),
    
    // 配置已知节点（不依赖 Bootstrap）
    dep2p.WithKnownPeers(
        config.KnownPeer{
            PeerID: "12D3KooWxxxxx...",
            Addrs:  []string{"/ip4/192.168.1.10/udp/4001/quic-v1"},
        },
        config.KnownPeer{
            PeerID: "12D3KooWyyyyy...",
            Addrs:  []string{"/ip4/192.168.1.11/udp/4001/quic-v1"},
        },
    ),
    
    // 禁用公共发现
    dep2p.WithBootstrapPeers(nil),
)
```

### 测试场景

```go
node, _ := dep2p.New(ctx,
    dep2p.WithPreset(dep2p.PresetMinimal),  // 最小配置
    dep2p.WithListenPort(0),                 // 随机端口
)
```

---

## 配置参数总表

### 基础配置

| 选项 | 类型 | 默认值 | 描述 |
|------|------|--------|------|
| `WithPreset` | `Preset` | - | 使用预设配置 |
| `WithIdentity` | `crypto.PrivKey` | 自动生成 | 节点身份 |
| `WithIdentityFile` | `string` | - | 身份文件路径 |
| `WithListenPort` | `int` | 随机 | 监听端口 |

### 发现配置

| 选项 | 类型 | 默认值 | 描述 |
|------|------|--------|------|
| `WithKnownPeers` | `[]KnownPeer` | `[]` | 已知节点列表 |
| `WithBootstrapPeers` | `[]string` | 公共节点 | 引导节点 |
| `WithMDNS` | `bool` | 见预设 | mDNS 发现 |
| `WithDHT` | `DHTMode` | `DHTClient` | DHT 模式 |

### NAT 配置

| 选项 | 类型 | 默认值 | 描述 |
|------|------|--------|------|
| `WithNAT` | `bool` | `true` | 启用 NAT |
| `WithTrustSTUNAddresses` | `bool` | `false` | 信任 STUN 地址 |
| `WithSTUNServers` | `[]string` | Google STUN | STUN 服务器 |
| `WithHolePunching` | `bool` | `true` | 启用打洞 |

### 连接配置

| 选项 | 类型 | 默认值 | 描述 |
|------|------|--------|------|
| `WithConnectionLimits` | `int, int` | 见预设 | 连接数限制 |
| `WithConnectionTimeout` | `Duration` | `30s` | 连接超时 |
| `WithIdleTimeout` | `Duration` | `5m` | 空闲超时 |

### 中继配置

| 选项 | 类型 | 默认值 | 描述 |
|------|------|--------|------|
| `WithRelay` | `bool` | `true` | 启用中继客户端 |
| `WithRelayServer` | `bool` | `false` | 启用中继服务器 |

---

## 相关文档

- [预设配置](presets.md) - 预设详细说明
- [Node API](api/node.md) - Node 接口参考
- [快速开始](../getting-started/quickstart.md) - 快速入门
- [云服务器部署](../tutorials/03-cloud-deploy.md) - 云部署教程
