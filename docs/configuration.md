# DeP2P 配置指南

本文档介绍 DeP2P 的配置系统，包括配置优先级、配置文件格式、环境变量和预设配置。

## 设计原则

DeP2P 遵循"库不做 I/O"的原则：

- **库（dep2p 包）**：只提供 Option/Config 结构体，不负责读文件/读环境变量
- **应用（cmd/dep2p）**：负责配置文件读取、环境变量解析、命令行参数处理

这意味着：
- 使用 dep2p 作为库时，你需要自己读取配置文件
- 使用 dep2p CLI 时，它会自动处理配置文件和环境变量

## 配置优先级

CLI 的配置优先级从高到低为：

```
1. 命令行参数          - 最高优先级
2. 环境变量（DEP2P_*）
3. 配置文件（JSON）
4. Preset 预设
5. 内部默认值          - 最低优先级
```

## 作为库使用

当把 dep2p 作为库使用时，配置读取由调用方负责：

```go
package main

import (
    "context"
    "encoding/json"
    "os"
    
    "github.com/dep2p/go-dep2p"
)

func main() {
    ctx := context.Background()
    
    // 方式1：直接使用 Option
    node, err := dep2p.New(ctx,
        dep2p.WithPreset(dep2p.PresetDesktop),
        dep2p.WithListenPort(4001),
    )
    if err != nil {
        panic(err)
    }
    if err := node.Start(ctx); err != nil {
        panic(err)
    }
    defer node.Close()
    
    // 方式2：从配置文件加载
    data, _ := os.ReadFile("config.json")
    var cfg dep2p.UserConfig
    json.Unmarshal(data, &cfg)
    node2, _ := dep2p.New(ctx, cfg.ToOptions()...)
    if err := node2.Start(ctx); err != nil {
        panic(err)
    }
}
```

## 使用 CLI

### 命令行参数

```bash
# 使用默认配置启动
dep2p

# 指定端口
dep2p -port 4001

# 使用配置文件
dep2p -config config.json

# 服务器模式
dep2p -preset server -port 4001

# 指定引导节点（逗号分隔）
dep2p -bootstrap "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/12D3KooW..."
```

### 环境变量

所有环境变量使用 `DEP2P_` 前缀：

| 环境变量 | 说明 | 示例 |
|----------|------|------|
| `DEP2P_PRESET` | 预设名称 | `server` |
| `DEP2P_LISTEN_PORT` | 监听端口 | `4001` |
| `DEP2P_BOOTSTRAP_PEERS` | 引导节点（逗号分隔） | `/ip4/.../p2p/...` |
| `DEP2P_IDENTITY_KEY_FILE` | 身份密钥文件 | `/etc/dep2p/identity.key` |
| `DEP2P_ENABLE_RELAY` | 启用中继 | `true` |
| `DEP2P_ENABLE_NAT` | 启用 NAT 穿透 | `true` |
| `DEP2P_LOG_FILE` | 日志文件路径 | `/var/log/dep2p.log` |

### 容器化部署

```dockerfile
# Dockerfile
ENV DEP2P_PRESET=server
ENV DEP2P_LISTEN_PORT=4001
ENV DEP2P_ENABLE_RELAY=true
```

```yaml
# docker-compose.yml
services:
  dep2p-node:
    image: dep2p/node
    environment:
      - DEP2P_PRESET=server
      - DEP2P_LISTEN_PORT=4001
```

## 配置文件格式

配置文件使用 JSON 格式，参考 `cmd/dep2p/example.config.json`：

```json
{
  "preset": "desktop",
  "listen_port": 4001,
  
  "identity": {
    "key_file": "~/.dep2p/identity.key"
  },
  
  "connection_limits": {
    "low": 50,
    "high": 100,
    "grace_period": "30s",
    "idle_timeout": "5m"
  },
  
  "discovery": {
    "bootstrap_peers": [
      "/dns4/bootstrap-ap.dep2p.io/udp/4001/quic-v1/p2p/12D3KooW..."
    ],
    "refresh_interval": "3m"
  },
  
  "relay": {
    "enable": true,
    "enable_server": false
  },
  
  "nat": {
    "enable": true,
    "enable_upnp": true,
    "stun_servers": [
      "stun:stun.l.google.com:19302"
    ]
  },
  
  "transport": {
    "max_connections": 100,
    "max_streams_per_conn": 256,
    "idle_timeout": "5m"
  }
}
```

### 配置项说明

| 配置项 | 类型 | 说明 |
|--------|------|------|
| `preset` | string | 预设：mobile, desktop, server, minimal, test |
| `listen_port` | int | 监听端口，0 表示随机 |
| `identity.key_file` | string | 身份密钥文件路径 |
| `connection_limits.low` | int | 连接数低水位线 |
| `connection_limits.high` | int | 连接数高水位线 |
| `discovery.bootstrap_peers` | []string | 引导节点列表 |
| `relay.enable` | bool | 启用中继客户端 |
| `relay.enable_server` | bool | 启用中继服务器 |
| `relay.addrs` | []string | Relay 服务器地址列表 |
| `nat.enable` | bool | 启用 NAT 穿透 |

### Relay 配置（v2.0 三大职责）

DeP2P v2.0 中 Relay 承担三大职责：

```
┌─────────────────────────────────────────────────────────────────────┐
│                    Relay 三大职责                                    │
├─────────────────────────────────────────────────────────────────────┤
│  1. 缓存加速层 - 地址簿作为 DHT 的本地缓存                            │
│  2. 打洞协调信令 - 提供 NAT 打洞的信令通道                            │
│  3. 数据通信保底 - 直连/打洞失败时转发数据                            │
└─────────────────────────────────────────────────────────────────────┘
```

```json
{
  "relay": {
    "enable": true,
    "enable_server": false,
    "addrs": [
      "/ip4/relay1.example.com/udp/4005/quic-v1/p2p/12D3KooW...",
      "/ip4/relay2.example.com/udp/4005/quic-v1/p2p/12D3KooW..."
    ]
  }
}
```

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `relay.enable` | bool | `true` | 启用 Relay 客户端功能 |
| `relay.enable_server` | bool | `false` | 启用 Relay 服务器功能（为其他节点转发） |
| `relay.addrs` | []string | `[]` | 明确配置的 Relay 地址列表 |

> **注意**：DeP2P v2.0 采用"明确配置"原则，不自动发现 Relay。部署方需要显式配置 Relay 地址。

### DHT 配置（权威目录模型）

DeP2P v2.0 采用 DHT 权威模型：

```json
{
  "discovery": {
    "dht": {
      "enable": true,
      "mode": "auto",
      "peer_record_ttl": "4h",
      "refresh_interval": "30m"
    },
    "bootstrap_peers": [
      "/dns4/bootstrap.dep2p.io/udp/4001/quic-v1/p2p/12D3KooW..."
    ]
  }
}
```

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `discovery.dht.enable` | bool | `true` | 启用 DHT |
| `discovery.dht.mode` | string | `auto` | DHT 模式：`client`/`server`/`auto` |
| `discovery.dht.peer_record_ttl` | duration | `4h` | PeerRecord TTL（NAT 后可能更短） |
| `discovery.dht.refresh_interval` | duration | `30m` | DHT 路由表刷新间隔 |

**地址查询优先级**：
1. KnownPeers（启动配置）
2. Peerstore（本地缓存）
3. MemberList（Gossip 同步）
4. DHT 查询（权威来源）
5. Relay 地址簿（缓存回退）

### 云服务器与连接性配置

以下配置项用于优化云服务器场景下的 P2P 连接性：

```json
{
  "known_peers": [
    {
      "peer_id": "12D3KooWxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
      "addrs": ["/ip4/1.2.3.4/udp/4001/quic-v1"]
    }
  ],
  
  "reachability": {
    "trust_stun_addresses": true,
    "stun_servers": [
      "stun:stun.l.google.com:19302"
    ]
  }
}
```

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `known_peers` | []object | `[]` | 已知节点列表，启动时直接连接 |
| `known_peers[].peer_id` | string | - | 节点的 Peer ID |
| `known_peers[].addrs` | []string | - | 节点的多地址列表 |
| `reachability.trust_stun_addresses` | bool | `false` | 信任 STUN 探测地址，跳过入站验证 |
| `reachability.stun_servers` | []string | Google STUN | STUN 服务器列表 |

**`known_peers` 使用场景**：
- 私有集群中的节点互联
- 无公共 Bootstrap 节点的环境
- 确保特定节点始终保持连接

**`trust_stun_addresses` 使用场景**：
- 云服务器有真实公网 IP
- 网络配置确保入站流量可达
- 避免等待入站连接验证导致的延迟

### 断开检测配置

DeP2P 采用多层次断开检测机制，可通过以下配置调优：

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

| 配置项 | 类型 | 默认值 | 说明 |
|--------|------|--------|------|
| `disconnect_detection.quic.keep_alive_period` | duration | `3s` | QUIC Keep-Alive 探测间隔 |
| `disconnect_detection.quic.max_idle_timeout` | duration | `6s` | QUIC 最大空闲超时 |
| `disconnect_detection.reconnect_grace_period` | duration | `15s` | 重连宽限期，期间不触发离线事件 |
| `disconnect_detection.disconnect_protection` | duration | `30s` | 断开保护期，防止重复添加刚断开的成员 |
| `disconnect_detection.witness.enabled` | bool | `true` | 启用见证人机制 |
| `disconnect_detection.witness.count` | int | `3` | 发送见证报告的节点数 |
| `disconnect_detection.witness.quorum` | int | `2` | 确认所需的最小见证人数 |
| `disconnect_detection.witness.timeout` | duration | `5s` | 见证报告超时时间 |
| `disconnect_detection.flapping.enabled` | bool | `true` | 启用震荡检测 |
| `disconnect_detection.flapping.window` | duration | `60s` | 震荡检测窗口 |
| `disconnect_detection.flapping.threshold` | int | `3` | 触发震荡判定的断线次数 |
| `disconnect_detection.flapping.cooldown` | duration | `120s` | 震荡后的冷却时间 |

**配置建议**：

| 场景 | `keep_alive_period` | `max_idle_timeout` | `reconnect_grace_period` |
|------|---------------------|-------------------|-------------------------|
| 稳定网络 | 3s | 6s | 10s |
| 移动网络 | 5s | 10s | 20s |
| 跨区域网络 | 3s | 6s | 15s（默认） |

### 完整配置示例（云服务器场景）

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
    "high": 500,
    "grace_period": "30s",
    "idle_timeout": "5m"
  },
  
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
      "quorum": 2
    }
  },
  
  "relay": {
    "enable": true,
    "enable_server": true
  },
  
  "nat": {
    "enable": true,
    "enable_upnp": false
  },
  
  "transport": {
    "max_connections": 500,
    "max_streams_per_conn": 256,
    "idle_timeout": "5m"
  }
}
```

## 预设配置

DeP2P 提供了针对不同场景优化的预设配置：

### mobile（移动端）

针对手机、平板等资源受限设备：
- 低连接数限制 (20/50)
- 低频率发现 (5分钟)
- 启用中继
- 较短空闲超时

### desktop（桌面端）

默认配置，适用于 PC、笔记本：
- 中等连接数限制 (50/100)
- 标准发现间隔 (3分钟)
- 启用中继
- 标准空闲超时

### server（服务器）

针对服务器、高性能节点：
- 高连接数限制 (200/500)
- 高频率发现 (1分钟)
- 可作为中继服务器
- 较长空闲超时

### minimal（最小）

仅用于测试和特殊场景：
- 极低连接数限制 (10/20)
- 禁用中继和 NAT
- 无 bootstrap 节点

### test（测试）

用于单元测试和集成测试：
- 低连接数限制
- 短发现间隔
- 无 bootstrap 节点（测试隔离）

## Bootstrap 节点

### 官方节点

官方 bootstrap 节点定义在 `internal/config/defaults.go`。部署公共节点后，使用 PresetDesktop/Server/Mobile 的节点会自动连接。

### 自定义节点

```go
ctx := context.Background()

// 使用自定义 bootstrap
node, _ := dep2p.New(ctx,
    dep2p.WithPreset(dep2p.PresetDesktop),
    dep2p.WithBootstrapPeers(
        "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/12D3KooW...",
    ),
)
node.Start(ctx)

// 使用 known_peers（推荐用于私有网络）
node2, _ := dep2p.New(ctx,
    dep2p.WithPreset(dep2p.PresetDesktop),
    dep2p.WithKnownPeers([]dep2p.KnownPeer{
        {
            PeerID: "12D3KooW...",
            Addrs:  []string{"/ip4/1.2.3.4/udp/4001/quic-v1"},
        },
    }),
)
node2.Start()

// 创世节点（无 bootstrap）
node3, _ := dep2p.New(ctx,
    dep2p.WithPreset(dep2p.PresetMinimal),
    dep2p.WithBootstrapPeers(nil),
)
node3.Start()
```

## 架构说明

```
cmd/dep2p/              CLI 入口（配置 I/O）
    ├── main.go         命令行参数、启动逻辑
    ├── config.go       配置文件/环境变量读取
    └── example.config.json
        ↓
dep2p (根包)            纯 API
    ├── config.go       UserConfig 结构体 + ToOptions()
    ├── options.go      WithXxx Option 函数
    └── presets.go      预设配置
        ↓
internal/config/        唯一配置真源
    ├── config.go       内部配置结构
    └── defaults.go     所有默认值
```

这种设计确保：
- 库使用者可以完全控制配置来源
- CLI 用户获得开箱即用的体验
- 默认值只有一个真源，不会混乱
