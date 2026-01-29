# Core Protocol - 协议注册与路由

> **版本**: v1.0.0  
> **状态**: ✅ 已完成  
> **覆盖率**: 60.7%

---

## 概述

`protocol` 模块实现协议注册与路由，负责协议处理器管理、协议协商和流路由。

**核心功能**:
- 📋 协议注册表 - 管理协议 ID 与处理器映射
- 🔀 协议路由器 - 根据协议 ID 路由入站流
- 🤝 协议协商器 - multistream-select 协商
- ⚡ 系统协议 - Ping 和 Identify

---

## 快速开始

### 创建协议注册表

```go
import "github.com/dep2p/go-dep2p/internal/core/protocol"

registry := protocol.NewRegistry()
```

### 注册协议

```go
handler := func(stream pkgif.Stream) {
    defer stream.Close()
    // 处理协议逻辑
}

err := registry.Register("/my/protocol/1.0.0", handler)
if err != nil {
    log.Fatal(err)
}
```

### 创建路由器

```go
negotiator := protocol.NewNegotiator(registry)
router := protocol.NewRouter(registry, negotiator)
```

### 路由入站流

```go
err := router.Route(stream)
if err != nil {
    log.Error("路由失败", err)
}
```

---

## 系统协议

### Ping 协议

**协议 ID**: `/dep2p/sys/ping/1.0.0`  
**功能**: 存活检测和 RTT 测量

```go
import "github.com/dep2p/go-dep2p/internal/core/protocol/system/ping"

// 注册 Ping 协议
pingService := ping.NewService()
registry.Register(ping.ProtocolID, pingService.Handler)

// 主动 Ping 节点
rtt, err := ping.Ping(ctx, host, "peer-id")
fmt.Printf("RTT: %v\n", rtt)
```

**特性**:
- 32 字节随机数据
- 服务器回显
- RTT 测量
- 支持连续 Ping

### Identify 协议

**协议 ID**: `/dep2p/sys/identify/1.0.0`  
**功能**: 节点身份信息交换

```go
import "github.com/dep2p/go-dep2p/internal/core/protocol/system/identify"

// 注册 Identify 协议
idService := identify.NewService(host, registry)
registry.Register(identify.ProtocolID, idService.Handler)

// 主动识别节点
info, err := identify.Identify(ctx, host, "peer-id")
fmt.Printf("Peer: %s\n", info.PeerID)
fmt.Printf("Protocols: %v\n", info.Protocols)
```

**交换信息**:
- PeerID（节点 ID）
- PublicKey（公钥）
- ListenAddrs（监听地址）
- ObservedAddr（观测地址）
- Protocols（支持的协议）
- AgentVersion（代理版本）

---

## API 文档

### Registry 接口

```go
type Registry struct { ... }

// NewRegistry 创建协议注册表
func NewRegistry() *Registry

// Register 注册协议处理器
func (r *Registry) Register(protocolID ProtocolID, handler StreamHandler) error

// Unregister 注销协议处理器
func (r *Registry) Unregister(protocolID ProtocolID) error

// GetHandler 获取协议处理器
func (r *Registry) GetHandler(protocolID ProtocolID) (StreamHandler, bool)

// Protocols 返回所有已注册的协议
func (r *Registry) Protocols() []ProtocolID

// AddMatcher 添加模式匹配器
func (r *Registry) AddMatcher(protocol ProtocolID, match func(ProtocolID) bool, handler StreamHandler)
```

### Router 接口

```go
type Router struct { ... }

// NewRouter 创建协议路由器
func NewRouter(registry *Registry, negotiator *Negotiator) *Router

// Route 路由流到对应的协议处理器
func (r *Router) Route(stream Stream) error

// AddRoute 添加路由规则（支持通配符）
func (r *Router) AddRoute(pattern string, handler StreamHandler) error

// RemoveRoute 移除路由规则
func (r *Router) RemoveRoute(pattern string) error
```

### Negotiator 接口

```go
type Negotiator struct { ... }

// NewNegotiator 创建协议协商器
func NewNegotiator(registry *Registry) *Negotiator

// Negotiate 协商协议（客户端模式）
func (n *Negotiator) Negotiate(ctx context.Context, conn Connection, protocols []ProtocolID) (ProtocolID, error)

// Handle 处理入站协议协商（服务器模式）
func (n *Negotiator) Handle(ctx context.Context, conn Connection) (ProtocolID, error)
```

---

## 测试结果

### 主包测试

✅ **20/20 通过** (10 个跳过)

**Registry 测试**:
- ✅ TestRegistry_Register - 注册协议
- ✅ TestRegistry_Unregister - 注销协议
- ✅ TestRegistry_GetHandler - 获取处理器
- ✅ TestRegistry_DuplicateRegister - 重复注册拒绝
- ✅ TestRegistry_Concurrent - 并发安全
- ✅ TestRegistry_AddMatcher - 模式匹配
- ✅ TestRegistry_Clear - 清空

**Router 测试**:
- ✅ TestRouter_New - 创建路由器
- ✅ TestRouter_AddRoute - 添加路由
- ✅ TestRouter_Route - 路由流

**覆盖率**: **60.7%**

### Ping 协议测试

✅ **4/4 通过**

- ✅ TestPing_Handler_Echo - 回显测试
- ✅ TestPingService_New - 创建服务
- ✅ TestPing_ProtocolID - 常量验证
- ✅ TestPing_DataIntegrity - 数据完整性

**覆盖率**: **29.0%**

### Identify 协议测试

✅ **1/1 通过** (3 个跳过)

- ✅ TestIdentify_Constants - 常量验证

**覆盖率**: **0%** (handler 需要完整 Host)

---

## 协议分类

### 系统协议 (/dep2p/sys/*)

| 协议 | 状态 | 说明 |
|------|------|------|
| `/dep2p/sys/ping/1.0.0` | ✅ v1.0 | 存活检测和 RTT 测量 |
| `/dep2p/sys/identify/1.0.0` | ✅ v1.0 | 节点身份信息交换 |
| `/dep2p/sys/autonat/1.0.0` | ⬜ v1.1+ | NAT 类型检测 |
| `/dep2p/sys/holepunch/1.0.0` | ⬜ v1.1+ | NAT 打洞协调 |
| `/dep2p/relay/1.0.0/hop` | ⬜ v1.1+ | 中继服务（HOP） |
| `/dep2p/relay/1.0.0/stop` | ⬜ v1.1+ | 中继服务（STOP） |

### Realm 协议 (/dep2p/realm/*)

由 Realm 层定义

### 应用协议 (/dep2p/app/*)

由应用层定义

---

## 架构

### 组件关系

```
┌─────────────────────────────────────────┐
│         Router (协议路由器)              │
├─────────────────────────────────────────┤
│  - Route(stream)                        │
│  - AddRoute(pattern, handler)           │
└─────────────┬───────────────────────────┘
              │
      ┌───────┴────────┐
      │                │
┌─────▼─────┐    ┌────▼──────┐
│ Registry  │    │Negotiator │
│(注册表)   │    │(协商器)   │
└───────────┘    └───────────┘

系统协议:
  ├── Ping (/dep2p/sys/ping/1.0.0)
  └── Identify (/dep2p/sys/identify/1.0.0)
```

---

## 性能

- **注册操作**: O(1) 时间复杂度
- **获取处理器**: O(1) 精确匹配，O(n) 模式匹配
- **协商延迟**: ~1-2 RTT（multistream-select）
- **并发安全**: 所有方法都是线程安全的

---

## 注意事项

1. ⚠️ **处理器调用**: handler(stream) 由调用方在新的 goroutine 中调用
2. ⚠️ **流关闭**: 处理器负责关闭流
3. ⚠️ **错误处理**: 协商失败返回 ErrNegotiationFailed
4. ⚠️ **模式匹配**: 使用简单的通配符匹配（"/test/*"）

---

## 未来扩展

- [ ] Protobuf 编码 - Identify 使用 Protobuf（v1.1+）
- [ ] Identify Push - 主动推送节点信息变更（v1.1+）
- [ ] AutoNAT v2 - NAT 类型自动检测（v1.1+）
- [ ] HolePunch - NAT 打洞协调（v1.1+）
- [ ] Circuit Relay v2 - 中继服务（v1.1+）

---

**最后更新**: 2026-01-13
