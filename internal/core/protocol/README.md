# Protocol 协议管理模块

## 概述

**层级**: Tier 2  
**职责**: 提供协议注册、协商和分发能力，管理节点支持的协议列表。

## 设计引用

> **重要**: 实现前请详细阅读以下设计规范

| 设计文档 | 说明 |
|----------|------|
| [传输协议规范](../../../docs/01-design/protocols/transport/01-transport.md) | 协议 ID 命名规范 |
| [消息传递协议](../../../docs/01-design/protocols/application/03-messaging.md) | 应用层协议 |

## 能力清单

### 协议注册能力 (必须实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| 处理器注册 | ✅ 已实现 | AddHandler() |
| 处理器移除 | ✅ 已实现 | RemoveHandler() |
| 协议列表 | ✅ 已实现 | Protocols() |
| 协议检查 | ✅ 已实现 | HasProtocol() |

### 协议协商能力 (必须实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| 多选一协商 | ✅ 已实现 | Multistream-select 协商 |
| 版本协商 | ✅ 已实现 | Semver 兼容版本选择 |
| 协商超时 | ✅ 已实现 | 协商超时处理 |

### 流分发能力 (必须实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| 流路由 | ✅ 已实现 | Router 根据协议分发流 |
| 匹配函数 | ✅ 已实现 | 自定义协议匹配 |

## 依赖关系

### 接口依赖

```
pkg/types/              → ProtocolID
pkg/interfaces/core/    → Stream, ProtocolHandler
pkg/interfaces/protocol/ → Router, Negotiator
```

### 模块依赖

```
无（Tier 2 基础模块）
```

## 目录结构

```
protocol/
├── README.md            # 本文件
├── module.go            # fx 模块定义
├── router.go            # 协议路由器
└── negotiator.go        # 协议协商器
```

## 公共接口

实现 `pkg/interfaces/protocol/` 中的接口：

```go
// Router 协议路由器接口
type Router interface {
    // AddHandler 添加协议处理器
    AddHandler(protocol types.ProtocolID, handler core.ProtocolHandler)
    
    // AddHandlerWithMatch 添加带匹配函数的处理器
    AddHandlerWithMatch(protocol types.ProtocolID, match MatchFunc, handler core.ProtocolHandler)
    
    // RemoveHandler 移除协议处理器
    RemoveHandler(protocol types.ProtocolID)
    
    // Handle 处理入站流
    Handle(stream core.Stream) error
    
    // Protocols 返回支持的协议列表
    Protocols() []types.ProtocolID
    
    // HasProtocol 检查是否支持协议
    HasProtocol(protocol types.ProtocolID) bool
}

// Negotiator 协议协商器接口
type Negotiator interface {
    // Negotiate 协商协议
    Negotiate(ctx context.Context, conn net.Conn, protocols []types.ProtocolID) (types.ProtocolID, error)
    
    // Handle 处理协商请求
    Handle(ctx context.Context, conn net.Conn) (types.ProtocolID, error)
}

// MatchFunc 协议匹配函数
type MatchFunc func(protocol types.ProtocolID) bool

// ProtocolHandler 协议处理器
type ProtocolHandler func(stream core.Stream)
```

## 协议 ID 命名规范

### 基本格式

```
/<namespace>/<protocol>/<version>

示例:
├── /dep2p/identity/1.0     → 身份协议
├── /dep2p/reqres/1.0       → 请求响应协议
├── /dep2p/pubsub/1.0       → 发布订阅协议
├── /dep2p/goodbye/1.0      → 优雅下线协议
├── /dep2p/sys/relay/1.0.0  → 中继协议
└── /myapp/transfer/1.0     → 自定义应用协议
```

### 版本兼容

```
版本匹配规则:
├── 精确匹配: /dep2p/foo/1.0 只匹配 /dep2p/foo/1.0
├── 前缀匹配: /dep2p/foo/1 匹配 /dep2p/foo/1.0, /dep2p/foo/1.1
└── 通配匹配: /dep2p/foo 匹配所有版本
```

## 协议协商流程

### Multistream-Select 协议

```
客户端                              服务端
   │                                  │
   │  1. "/multistream/1.0.0\n"       │
   │─────────────────────────────────►│
   │                                  │
   │  2. "/multistream/1.0.0\n"       │
   │◄─────────────────────────────────│
   │                                  │
   │  3. "/dep2p/reqres/1.0\n"        │
   │─────────────────────────────────►│
   │                                  │
   │  4a. "/dep2p/reqres/1.0\n" (支持) │
   │◄─────────────────────────────────│
   │                                  │
   │  或                              │
   │  4b. "na\n" (不支持)             │
   │◄─────────────────────────────────│
```

## 关键实现

### 协议路由器

```go
type router struct {
    handlers map[types.ProtocolID]handlerEntry
    mu       sync.RWMutex
}

type handlerEntry struct {
    handler core.ProtocolHandler
    match   MatchFunc
}

func (r *router) Handle(stream core.Stream) error {
    protocol := stream.Protocol()
    
    r.mu.RLock()
    entry, ok := r.handlers[protocol]
    r.mu.RUnlock()
    
    if !ok {
        // 尝试匹配函数
        for pid, e := range r.handlers {
            if e.match != nil && e.match(protocol) {
                entry = e
                ok = true
                break
            }
        }
    }
    
    if !ok {
        return ErrProtocolNotSupported
    }
    
    // 调用处理器
    entry.handler(stream)
    return nil
}
```

### 协议协商器

```go
type negotiator struct {
    supported []types.ProtocolID
}

func (n *negotiator) Negotiate(ctx context.Context, conn net.Conn, protocols []types.ProtocolID) (types.ProtocolID, error) {
    // 发送 multistream header
    writeString(conn, "/multistream/1.0.0\n")
    
    // 读取确认
    header, _ := readString(conn)
    if header != "/multistream/1.0.0" {
        return "", ErrProtocolNegotiation
    }
    
    // 尝试每个协议
    for _, proto := range protocols {
        writeString(conn, string(proto)+"\n")
        
        resp, _ := readString(conn)
        if resp == string(proto) {
            return proto, nil  // 协商成功
        }
        // "na" 表示不支持，继续尝试
    }
    
    return "", ErrNoCommonProtocol
}
```

## fx 模块

```go
type ModuleInput struct {
    fx.In
    Handlers []ProtocolHandlerEntry `group:"protocol_handlers" optional:"true"`
}

type ProtocolHandlerEntry struct {
    Protocol types.ProtocolID
    Handler  core.ProtocolHandler
}

type ModuleOutput struct {
    fx.Out
    Router protocolif.Router `name:"protocol_router"`
}

func Module() fx.Option {
    return fx.Module("protocol",
        fx.Provide(ProvideServices),
        fx.Invoke(registerLifecycle),
    )
}
```

## 内置协议

| 协议 ID | 说明 |
|---------|------|
| `/dep2p/identity/1.0` | 身份交换 |
| `/dep2p/reqres/1.0` | 请求响应 |
| `/dep2p/pubsub/1.0` | 发布订阅 |
| `/dep2p/goodbye/1.0` | 优雅下线 |
| `/dep2p/ping/1.0` | Ping/Pong 心跳 |
| `/dep2p/sys/relay/1.0.0` | 中继协议 |
| `/dep2p/sys/dht/1.0.0` | DHT 协议（系统层，走标准协议协商） |
| `/dep2p/sys/holepunch/1.0.0` | 打洞协议 |
| `/dep2p/addr-mgmt/1.0` | 地址刷新/查询协议 |
| `/dep2p/realm/1.0` | Realm 管理协议 |
| `/dep2p/cross-realm/1.0` | 跨 Realm 路由协议 |
| `/dep2p/device-id/1.0` | 设备身份协议（可选） |

## DHT 协议说明

> **T6 修复**：DHT 协议已改为通过 Endpoint 注册协议处理器，走标准协议协商。

DHT 协议 ID 为 `/dep2p/sys/dht/1.0.0`，实现位于 `internal/core/discovery/dht/`：

1. **协议处理器**：`Handler` 实现 `endpoint.ProtocolHandler`，通过 `Endpoint.SetProtocolHandler` 注册
2. **网络适配器**：`NetworkAdapter` 实现 `Network` 接口，封装 DHT 请求为 stream 消息
3. **消息格式**：使用自定义二进制帧格式（4字节长度头 + JSON 载荷）

### Provider 扩展（sys/relay 等）

- **ADD_PROVIDER**：通告/续租本节点为 `namespace` 的 provider（请求携带 `ttl` 秒；ttl 会被裁剪到 24h 上限）。  
- **GET_PROVIDERS**：获取 providers（响应包含 `providers[].ttl` 与 `providers[].timestamp`，用于对端缓存与过期判断）。  
- **REMOVE_PROVIDER**：best-effort 撤销本节点 provider 注册（仅允许撤销发送者自身记录）。  

```
DHT 消息流:
Node A --[OpenStream(/dep2p/sys/dht/1.0.0)]--> Node B
         │
         │ [FIND_NODE Request]
         ├──────────────────────────────────►
         │
         │ [FIND_NODE Response]
         ◄──────────────────────────────────┤
         │
         │ [CloseStream]
         ├──────────────────────────────────►
```

### namespace 命名规范

DHT 使用的 namespace 参数（如 `relay`）会在内部拼接为完整 key：

```
ProviderKeyPrefix + namespace = "dep2p/v1/sys/" + "relay" = "dep2p/v1/sys/relay"
```

调用示例：
```go
// 正确：使用简短 namespace
discovery.Announce(ctx, "relay")      // → key: "dep2p/v1/sys/relay"
discovery.DiscoverPeers(ctx, "relay") // → key: "dep2p/v1/sys/relay"

// 错误：不要重复前缀
discovery.Announce(ctx, "sys/relay")  // → key: "dep2p/v1/sys/sys/relay" (错误！)
```

## 相关文档

- [传输协议规范](../../../docs/01-design/protocols/transport/01-transport.md)
- [消息传递协议](../../../docs/01-design/protocols/application/03-messaging.md)
- [DHT 实现](../discovery/dht/README.md)
- [pkg/interfaces/protocol](../../../pkg/interfaces/protocol/)
