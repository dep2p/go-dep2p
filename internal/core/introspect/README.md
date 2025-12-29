# Introspect 自省服务

## 概述

提供本地 HTTP 自省接口，用于调试和监控 DeP2P 节点状态。

## 目录结构

```
introspect/
├── README.md       # 本文件
└── server.go       # HTTP 服务实现
```

## 接口定义

```go
// Server 本地自省 HTTP 服务
type Server struct {
    endpoint endpoint.Endpoint
    realm    realmif.RealmManager
    relay    relayif.RelayServer // 可选
    addr     string
}

// 创建和管理
func New(cfg Config) *Server
func (s *Server) Start(ctx context.Context) error
func (s *Server) Stop() error
func (s *Server) Addr() string
```

## HTTP 端点

| 端点 | 方法 | 说明 |
|------|------|------|
| `/debug/introspect` | GET | 完整诊断报告 |
| `/debug/introspect/node` | GET | 节点信息 |
| `/debug/introspect/connections` | GET | 连接信息 |
| `/debug/introspect/realm` | GET | Realm 信息 |
| `/debug/introspect/relay` | GET | Relay 信息 |
| `/debug/pprof/*` | GET | Go pprof 端点 |
| `/health` | GET | 健康检查 |

## 响应格式

### 完整诊断报告 `/debug/introspect`

```json
{
  "timestamp": "2025-12-28T10:00:00Z",
  "node": {
    "id": "12D3KooW...",
    "id_short": "12D3Koo",
    "public_key_type": "Ed25519",
    "uptime": 3600000000000,
    "started_at": "2025-12-28T09:00:00Z"
  },
  "connections": {
    "total": 15,
    "inbound": 8,
    "outbound": 7,
    "peers": ["12D3Koo", "QmPeer1"],
    "path_stats": {
      "direct": 10,
      "hole_punched": 2,
      "relayed": 3
    }
  },
  "addresses": {
    "listen_addrs": ["/ip4/0.0.0.0/udp/4001/quic-v1"],
    "advertised_addrs": ["/ip4/1.2.3.4/udp/4001/quic-v1"],
    "verified_direct_addrs": ["/ip4/1.2.3.4/udp/4001/quic-v1"]
  },
  "discovery": {
    "state": "Ready",
    "state_ready": true,
    "bootstrap_peers": 3,
    "known_peers": 50
  },
  "nat": {
    "type": "Symmetric",
    "port_mapping_available": false
  },
  "relay": {
    "enabled": true,
    "reserved_relays": 2
  },
  "realm": {
    "current_realm": "my-realm",
    "is_member": true,
    "member_count": 50
  },
  "extended": {
    "relay_server": {
      "running": true,
      "active_reservations": 5,
      "active_connections": 3,
      "total_connections": 100,
      "total_bytes_relayed": 1048576
    },
    "realm_details": {
      "realm_id": "my-realm",
      "members": ["12D3Koo", "QmPeer1"],
      "topics": ["chat", "events"]
    }
  }
}
```

## 使用示例

### 启用自省服务

```go
import (
    "github.com/dep2p/go-dep2p"
)

node, _ := dep2p.StartNode(ctx,
    dep2p.WithPreset(dep2p.PresetDesktop),
    dep2p.WithIntrospect(true),                  // 启用自省
    dep2p.WithIntrospectAddr("127.0.0.1:6060"),  // 可选：自定义地址
)
```

### 查询诊断信息

```bash
# 完整诊断报告
curl http://127.0.0.1:6060/debug/introspect

# 节点信息
curl http://127.0.0.1:6060/debug/introspect/node

# 连接信息
curl http://127.0.0.1:6060/debug/introspect/connections

# 健康检查
curl http://127.0.0.1:6060/health

# Go pprof
go tool pprof http://127.0.0.1:6060/debug/pprof/profile
```

## 安全说明

- 默认绑定到 `127.0.0.1`，仅本地可访问
- 不要将自省端口暴露到公网
- pprof 端点可能泄露敏感信息，生产环境谨慎使用

## 依赖

- `net/http` - HTTP 服务
- `net/http/pprof` - Go 性能分析
- `pkg/interfaces/endpoint` - 诊断报告接口

