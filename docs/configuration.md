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
    "encoding/json"
    "os"
    
    "github.com/dep2p/go-dep2p"
)

func main() {
    // 方式1：直接使用 Option
    node, err := dep2p.NewNode(
        dep2p.WithPreset(dep2p.PresetDesktop),
        dep2p.WithListenPort(4001),
    )
    
    // 方式2：从配置文件加载
    data, _ := os.ReadFile("config.json")
    var cfg dep2p.UserConfig
    json.Unmarshal(data, &cfg)
    node, err := dep2p.NewNode(cfg.ToOptions()...)
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
| `nat.enable` | bool | 启用 NAT 穿透 |

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
// 使用自定义 bootstrap
dep2p.NewNode(dep2p.WithBootstrapPeers(
    "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/12D3KooW...",
))

// 创世节点（无 bootstrap）
dep2p.NewNode(dep2p.WithBootstrapPeers(nil))
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
