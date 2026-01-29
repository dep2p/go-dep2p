# dep2p 命令行工具

dep2p CLI 是 DeP2P 网络的统一命令行入口，支持启动普通节点和基础设施节点。

## 快速开始

```bash
# 普通节点（默认配置）
go run ./cmd/dep2p/

# 普通节点（指定端口）
go run ./cmd/dep2p/ --port 9000

# 普通节点（使用配置文件）
go run ./cmd/dep2p/ --config config.json

# 基础设施节点（Bootstrap + Relay）
go run ./cmd/dep2p/ --enable-infra --port 4001 --public-addr /ip4/YOUR_IP/udp/4001/quic-v1
```

## 配置边界

dep2p 遵循清晰的配置边界原则：

| 类型 | 职责 | 示例 |
|------|------|------|
| **命令行参数** | 运行时覆盖 / 快速测试 | `--port`, `--preset`, `--enable-*` |
| **配置文件** | 持久化配置 / 长期运行 | 中继、NAT、连接限制、引导节点 |
| **环境变量** | 部署时覆盖 | `DEP2P_PRESET`, `DEP2P_ENABLE_*` |

**优先级**: 命令行参数 > 环境变量 > 配置文件 > 预设默认值

## 命令行参数

### 运行时参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--port` | 监听端口（0 = 随机） | `0` |
| `--config` | 配置文件路径 | - |
| `--preset` | 预设配置 | `desktop` |
| `--identity` | 身份密钥文件 | - |
| `--public-addr` | 公网地址（基础设施节点必需） | - |

### 能力开关

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--enable-bootstrap` | 启用 Bootstrap 服务 | `false` |
| `--enable-relay` | 启用 Relay 服务 | `false` |
| `--enable-infra` | 启用全部基础设施 | `false` |

### 日志参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `--log` | 日志文件路径 | - |
| `--log-dir` | 日志目录 | `logs` |
| `--auto-log` | 自动生成日志文件 | `true` |

### 信息显示

| 参数 | 说明 |
|------|------|
| `--version` | 显示版本 |
| `--help` | 显示帮助 |

## 预设配置

| 预设 | 说明 | 适用场景 |
|------|------|----------|
| `mobile` | 低资源占用 | 移动设备 |
| `desktop` | 均衡配置 | 桌面应用 |
| `server` | 高性能 | 服务器部署 |
| `minimal` | 最小配置 | 测试开发 |

## 配置文件

配置文件使用 JSON 格式，用于持久化节点配置：

```json
{
  "identity": {
    "key_file": "~/.dep2p/identity.key"
  },
  "relay": {
    "enable_client": true,
    "relay_addr": "/ip4/relay.example.com/udp/4001/quic-v1/p2p/12D3KooW..."
  },
  "nat": {
    "enable_auto_nat": true,
    "enable_upnp": true,
    "enable_nat_pmp": true,
    "enable_hole_punch": true
  },
  "conn_mgr": {
    "low_water": 50,
    "high_water": 100
  },
  "discovery": {
    "bootstrap": {
      "peers": [
        "/ip4/bootstrap.example.com/udp/4001/quic-v1/p2p/12D3KooW..."
      ]
    }
  }
}
```

### 配置项说明

| 配置路径 | 说明 | 类型 |
|----------|------|------|
| `identity.key_file` | 身份密钥文件 | string |
| `relay.enable_client` | 启用中继客户端 | bool |
| `relay.relay_addr` | Relay 地址 | string |
| `nat.enable_auto_nat` | 启用 AutoNAT | bool |
| `nat.enable_upnp` | 启用 UPnP | bool |
| `nat.enable_nat_pmp` | 启用 NAT-PMP | bool |
| `nat.enable_hole_punch` | 启用 NAT 打洞 | bool |
| `conn_mgr.low_water` | 连接低水位 | int |
| `conn_mgr.high_water` | 连接高水位 | int |
| `discovery.bootstrap.peers` | 引导节点列表 | []string |

## 环境变量

所有环境变量使用 `DEP2P_` 前缀：

### 运行时环境变量

```bash
DEP2P_PRESET=server           # 预设配置
DEP2P_LISTEN_PORT=9000        # 监听端口
DEP2P_IDENTITY_KEY_FILE=...   # 身份文件
DEP2P_ENABLE_BOOTSTRAP=true   # 启用 Bootstrap
DEP2P_ENABLE_RELAY=true       # 启用 Relay
DEP2P_PUBLIC_ADDR=/ip4/.../p2p/... # 公网地址
DEP2P_LOG_FILE=dep2p.log      # 日志文件
```

### 配置覆盖环境变量

```bash
DEP2P_BOOTSTRAP_PEERS=/ip4/.../p2p/...,/ip4/.../p2p/...  # 覆盖引导节点
DEP2P_RELAY_ADDR=/ip4/.../p2p/...  # 覆盖 Relay 地址
DEP2P_ENABLE_RELAY=true       # 覆盖中继配置
DEP2P_ENABLE_NAT=true         # 覆盖 NAT 配置
```

## 使用示例

### 普通节点

```bash
# 开发测试（最小配置）
go run ./cmd/dep2p/ --preset=minimal --port=9000 --auto-log=false

# 生产环境（使用配置文件）
dep2p --config=config.json

# 服务器模式
dep2p --preset=server --port=4001
```

### 基础设施节点

```bash
# 同时启用 Bootstrap + Relay
dep2p --enable-infra --port=4001 --public-addr=/ip4/1.2.3.4/udp/4001/quic-v1

# 仅 Bootstrap
dep2p --enable-bootstrap --port=4001 --public-addr=/ip4/1.2.3.4/udp/4001/quic-v1

# 仅 Relay
dep2p --enable-relay --port=4001 --public-addr=/ip4/1.2.3.4/udp/4001/quic-v1

# 使用环境变量（Docker 部署）
DEP2P_ENABLE_BOOTSTRAP=true \
DEP2P_PUBLIC_ADDR=/ip4/1.2.3.4/udp/4001/quic-v1 \
dep2p --port=4001
```

## 地址格式

dep2p 使用 [multiaddr](https://multiformats.io/multiaddr/) 格式：

```
/ip4/<IP>/udp/<PORT>/quic-v1/p2p/<NodeID>   # QUIC（推荐）
/ip4/<IP>/tcp/<PORT>/p2p/<NodeID>           # TCP
/ip6/<IPv6>/udp/<PORT>/quic-v1/p2p/<NodeID> # IPv6 QUIC
/dnsaddr/<DOMAIN>/p2p/<NodeID>              # DNS
```

## 为什么 cmd 没有 doc.go？

Go 的 `doc.go` 文件用于为**库包**提供文档，当使用 `go doc` 命令查看包文档时会显示。

`cmd/dep2p` 是一个 `main` 包（可执行程序），不会被其他包导入，因此：

1. `go doc` 对 main 包的支持有限
2. 用户更常阅读 README.md
3. README.md 可以包含丰富的格式、示例和图表

**结论**: 命令行工具使用 README.md 而非 doc.go。

## 云服务器部署

如需在云服务器部署基础设施节点（Bootstrap + Relay + Gateway），请参阅：

**[部署指南](./deploy/README.md)**

部署目录包含：
- `README.md` - 详细部署文档
- `infra.config.json` - 配置模板
- `dep2p.service` - systemd 服务文件
- `data/` - 运行时数据目录（存放身份密钥）

## 相关文档

- [配置边界设计](../../design/_discussions/20260116-config-boundary.md)
- [Bootstrap 节点设计](../../design/_discussions/20260116-bootstrap-node-detailed.md)
- [统一 Relay 设计](../../design/_discussions/20260123-nat-relay-concept-clarification.md)
- [ADR-0009: Bootstrap 能力开关](../../design/03_architecture/L3_decision/adr-0009-bootstrap-capability.md)
- [ADR-0010: Relay 能力开关](../../design/03_architecture/L3_decision/adr-0010-relay-capability.md)
