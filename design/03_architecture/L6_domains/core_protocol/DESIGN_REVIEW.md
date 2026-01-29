# Core Protocol 设计审查

> **审查日期**: 2026-01-13  
> **审查人**: AI Agent  
> **版本**: v1.0.0

---

## 一、设计文档审查

### 1.1 模块定位

**架构层**: Core Layer  
**职责**: 协议注册、协议协商、协议路由、系统协议实现

**核心目标**:
- 管理协议 ID 与处理器的映射（Registry）
- 根据协议 ID 路由入站流（Router）
- 使用 multistream-select 协商协议（Negotiator）
- 实现系统协议（Ping、Identify 等）

### 1.2 协议分类

**系统协议** (`/dep2p/sys/*`):
- `/dep2p/sys/ping/1.0.0` - 存活检测
- `/dep2p/sys/identify/1.0.0` - 身份识别
- `/dep2p/sys/autonat/1.0.0` - NAT 检测（v1.1+）
- `/dep2p/sys/holepunch/1.0.0` - NAT 打洞（v1.1+）
- `/dep2p/relay/1.0.0/{hop,stop}` - 中继服务（v1.1+）

**Realm 协议** (`/dep2p/realm/<id>/*`):
- `/dep2p/realm/<id>/join/1.0.0` - 加入域
- `/dep2p/realm/<id>/auth/1.0.0` - 域认证

**应用协议** (`/dep2p/app/<id>/*`):
- 由应用层定义

---

## 二、go-libp2p protocol.Switch 分析

### 2.1 核心设计

**protocol.Switch 接口**:
```go
type Switch interface {
    Router
    Negotiator
}
```

**特点**:
- 组合 Router 和 Negotiator 接口
- 使用 `go-multistream` 进行协议协商
- 支持精确匹配和模式匹配

### 2.2 Router 接口

```go
type Router interface {
    // AddHandler 注册精确匹配的协议
    AddHandler(protocol ID, handler HandlerFunc)
    
    // AddHandlerWithFunc 注册带匹配函数的协议
    AddHandlerWithFunc(protocol ID, match func(ID) bool, handler HandlerFunc)
    
    // RemoveHandler 移除协议处理器
    RemoveHandler(protocol ID)
    
    // Protocols 返回所有注册的协议
    Protocols() []ID
}
```

### 2.3 Negotiator 接口

```go
type Negotiator interface {
    // Negotiate 协商协议（客户端）
    Negotiate(rwc io.ReadWriteCloser) (ID, HandlerFunc, error)
    
    // Handle 处理协议协商并调用处理器（服务器端）
    Handle(rwc io.ReadWriteCloser) error
}
```

### 2.4 协商流程

**multistream-select 协议**:
```
服务器端（Negotiate）:
1. 接收客户端请求的协议列表
2. 选择第一个支持的协议
3. 返回选中的协议 ID

客户端（SelectOneOf）:
1. 发送支持的协议列表
2. 接收服务器选择的协议
3. 验证并返回
```

---

## 三、go-libp2p Ping 协议分析

### 3.1 协议规范

**协议 ID**: `/ipfs/ping/1.0.0`  
**消息大小**: 32 字节  
**超时时间**: 10 秒

### 3.2 实现要点

```go
type PingService struct {
    Host host.Host
}

// PingHandler 处理 Ping 请求（服务器端）
func (p *PingService) PingHandler(s network.Stream) {
    // 1. 读取 32 字节
    buf := make([]byte, 32)
    io.ReadFull(s, buf)
    
    // 2. 回显
    s.Write(buf)
    
    // 3. 循环处理（支持连续 ping）
}

// Ping 主动 Ping 节点（客户端）
func (p *PingService) Ping(ctx context.Context, peer peer.ID) (time.Duration, error) {
    // 1. 创建流
    s, err := p.Host.NewStream(ctx, peer, ID)
    
    // 2. 生成随机数据
    buf := make([]byte, 32)
    rand.Read(buf)
    
    // 3. 测量 RTT
    start := time.Now()
    s.Write(buf)
    io.ReadFull(s, buf)
    rtt := time.Since(start)
    
    return rtt, nil
}
```

### 3.3 关键特性

- ✅ 简单回显协议
- ✅ 支持 RTT 测量
- ✅ 支持连续 Ping
- ✅ 超时保护

---

## 四、go-libp2p Identify 协议分析

### 4.1 协议规范

**协议 ID**: 
- `/ipfs/id/1.0.0` - Identify
- `/ipfs/id/push/1.0.0` - Identify Push

**功能**: 交换节点身份信息

### 4.2 交换信息

**Identify 消息内容**:
```protobuf
message Identify {
    string protocolVersion = 5;       // e.g. "ipfs/0.1.0"
    string agentVersion = 6;          // e.g. "go-libp2p/0.1.0"
    bytes publicKey = 1;              // 公钥
    repeated bytes listenAddrs = 2;  // 监听地址
    bytes observedAddr = 4;           // 观测到的地址
    repeated string protocols = 3;   // 支持的协议列表
    SignedPeerRecord signedPeerRecord = 8; // 签名的节点记录
}
```

### 4.3 实现要点

```go
type idService struct {
    Host host.Host
}

// IdentifyHandler 处理 Identify 请求（服务器端）
func (ids *idService) IdentifyHandler(s network.Stream) {
    // 1. 构造 Identify 消息
    msg := &pb.Identify{
        ProtocolVersion: "dep2p/1.0.0",
        AgentVersion:    "go-dep2p/1.0.0",
        PublicKey:       ids.Host.ID().PublicKey(),
        ListenAddrs:     ids.Host.Addrs(),
        ObservedAddr:    s.Conn().RemoteMultiaddr(),
        Protocols:       ids.Host.Mux().Protocols(),
    }
    
    // 2. 发送 Protobuf 消息
    writer := pbio.NewDelimitedWriter(s)
    writer.WriteMsg(msg)
}

// Identify 主动识别节点（客户端）
func (ids *idService) Identify(ctx context.Context, peer peer.ID) (*Identify, error) {
    // 1. 创建流
    s, err := ids.Host.NewStream(ctx, peer, ID)
    
    // 2. 接收 Identify 消息
    reader := pbio.NewDelimitedReader(s, maxMsgSize)
    msg := &pb.Identify{}
    reader.ReadMsg(msg)
    
    return msg, nil
}
```

### 4.4 关键特性

- ✅ Protobuf 编码
- ✅ 交换完整节点信息
- ✅ 观测地址（NAT 穿透用）
- ✅ 支持 Push 模式（v1.1+）

---

## 五、DeP2P 设计方案

### 5.1 简化设计

**相比 go-libp2p 的简化**:
- ✅ 保留 Registry、Router、Negotiator 三层
- ✅ 保留 multistream-select 协商
- ✅ v1.0 仅实现 Ping 和 Identify
- ❌ 不实现 Push 模式（v1.1+）
- ❌ 不实现 AutoNAT、HolePunch、Relay（v1.1+）
- ❌ 不实现速率限制（初期不需要）

**理由**:
- Ping 和 Identify 是最核心的系统协议
- 其他协议依赖更多模块（NAT、Relay 等）
- 降低初期复杂度

### 5.2 核心数据结构

```go
// DeP2P Registry 设计
type Registry struct {
    mu       sync.RWMutex
    handlers map[pkgif.ProtocolID]pkgif.StreamHandler
    matchers []matcher  // 模式匹配器
}

type matcher struct {
    protocol pkgif.ProtocolID
    match    func(pkgif.ProtocolID) bool
    handler  pkgif.StreamHandler
}

// DeP2P Router 设计
type Router struct {
    registry   *Registry
    negotiator *Negotiator
}

// DeP2P Negotiator 设计
type Negotiator struct {
    registry *Registry
    timeout  time.Duration
}
```

### 5.3 协议注册流程

```go
// 1. 创建 Registry
registry := NewRegistry()

// 2. 注册 Ping 协议
pingService := ping.NewService(host)
registry.Register(ping.ProtocolID, pingService.Handler)

// 3. 注册 Identify 协议
idService := identify.NewService(host)
registry.Register(identify.ProtocolID, idService.Handler)

// 4. 创建 Router
router := NewRouter(registry, negotiator)

// 5. 处理入站流
router.Route(stream)
```

### 5.4 协议协商流程（DeP2P）

```
入站流处理:

1. Stream 到达
        ↓
2. Router.Route(stream)
        ↓
3. Negotiator.Handle(stream)
        ↓ (multistream-select)
4. 协商出 ProtocolID
        ↓
5. Registry.GetHandler(protocolID)
        ↓
6. handler(stream)
        ↓
7. 协议逻辑执行
```

---

## 六、Ping 协议设计（DeP2P）

### 6.1 协议规范

**协议 ID**: `/dep2p/sys/ping/1.0.0`  
**消息格式**: 32 字节随机数据  
**超时**: 10 秒

### 6.2 实现要点

```go
type PingService struct {
    // 不需要持有 Host，通过流获取信息
}

// Handler 处理 Ping 请求（服务器端）
func (ps *PingService) Handler(stream pkgif.Stream) {
    defer stream.Close()
    
    buf := make([]byte, PingSize)
    for {
        // 读取
        _, err := io.ReadFull(stream, buf)
        if err != nil {
            return
        }
        
        // 回显
        _, err = stream.Write(buf)
        if err != nil {
            return
        }
    }
}

// Ping 主动 Ping（客户端，待实现）
func Ping(ctx context.Context, host Host, peer string) (time.Duration, error) {
    // 1. 创建流
    stream, err := host.NewStream(ctx, peer, ProtocolPing)
    
    // 2. 生成随机数据
    buf := make([]byte, PingSize)
    rand.Read(buf)
    
    // 3. 测量 RTT
    start := time.Now()
    stream.Write(buf)
    io.ReadFull(stream, buf)
    rtt := time.Since(start)
    
    return rtt, nil
}
```

---

## 七、Identify 协议设计（DeP2P）

### 7.1 协议规范

**协议 ID**: `/dep2p/sys/identify/1.0.0`

**功能**: 交换节点身份信息

### 7.2 消息格式（简化版）

```go
type IdentifyInfo struct {
    PeerID          string
    PublicKey       []byte
    ListenAddrs     []string
    ObservedAddr    string
    Protocols       []string
    AgentVersion    string
    ProtocolVersion string
}
```

**v1.0 简化**:
- 使用 JSON 编码（而非 Protobuf）- 简化实现
- 不实现签名节点记录（v1.1+）
- 不实现 Push 模式（v1.1+）

### 7.3 实现要点

```go
type IdentifyService struct {
    host Host
}

// Handler 处理 Identify 请求（服务器端）
func (ids *IdentifyService) Handler(stream pkgif.Stream) {
    defer stream.Close()
    
    // 构造 Identify 消息
    info := &IdentifyInfo{
        PeerID:          ids.host.ID(),
        ListenAddrs:     ids.host.Addrs(),
        Protocols:       registry.Protocols(),
        AgentVersion:    "go-dep2p/1.0.0",
        ProtocolVersion: "dep2p/1.0.0",
    }
    
    // 发送 JSON
    json.NewEncoder(stream).Encode(info)
}

// Identify 主动识别节点（客户端）
func Identify(ctx context.Context, host Host, peer string) (*IdentifyInfo, error) {
    // 1. 创建流
    stream, err := host.NewStream(ctx, peer, ProtocolIdentify)
    
    // 2. 接收 Identify 消息
    info := &IdentifyInfo{}
    json.NewDecoder(stream).Decode(info)
    
    return info, nil
}
```

---

## 八、multistream-select 协商

### 8.1 协商流程

**服务器端**（Negotiate）:
```
1. 创建 multistream.Transport
2. transport.Negotiate(stream)
3. 返回客户端选择的协议
```

**客户端**（SelectOneOf）:
```
1. 创建 multistream.Transport
2. transport.SelectOneOf(protocols, stream)
3. 返回服务器支持的第一个协议
```

### 8.2 参考 core_upgrader

DeP2P 已在 `core_upgrader` 中实现了 multistream-select：

```go
// core_upgrader/multistream.go
func negotiateSecurity(...) (pkgif.SecureTransport, error) {
    mss := multistream.NewMultistreamMuxer[pkgif.SecureTransport]()
    
    // 添加支持的协议
    for _, st := range securityTransports {
        mss.AddHandler(st.ID(), st)
    }
    
    // 协商
    if isServer {
        selected, err := mss.Negotiate(rwc)
        return selected, err
    } else {
        ids := make([]string, len(securityTransports))
        for i, st := range securityTransports {
            ids[i] = st.ID()
        }
        selected, err := mss.SelectOneOf(ids, rwc)
        return selected, err
    }
}
```

**可复用设计**:
- ✅ 使用 `go-multistream` 库
- ✅ `Negotiate()` 用于服务器端
- ✅ `SelectOneOf()` 用于客户端

---

## 九、DeP2P 实现方案

### 9.1 组件关系

```
Registry (注册表)
  ├── handlers: map[ProtocolID]StreamHandler
  └── matchers: []matcher (模式匹配)

Negotiator (协商器)
  ├── registry: *Registry
  └── multistream.Transport

Router (路由器)
  ├── registry: *Registry
  └── negotiator: *Negotiator
  
  Route(stream Stream):
    1. negotiator.Handle(stream) → protocolID
    2. registry.GetHandler(protocolID) → handler
    3. handler(stream)
```

### 9.2 系统协议注册

```go
// module.go
func Module() fx.Option {
    return fx.Module("protocol",
        // 核心组件
        fx.Provide(
            NewRegistry,
            NewRouter,
            NewNegotiator,
        ),
        // 系统协议
        fx.Invoke(registerSystemProtocols),
    )
}

func registerSystemProtocols(registry *Registry, host Host) {
    // Ping
    pingService := ping.NewService()
    registry.Register(ping.ProtocolID, pingService.Handler)
    
    // Identify
    idService := identify.NewService(host)
    registry.Register(identify.ProtocolID, idService.Handler)
}
```

---

## 十、实现计划

### 10.1 核心文件

**协议框架**（6 文件）:
1. `registry.go` - 协议注册表（~150 行）
2. `router.go` - 协议路由器（~100 行）
3. `negotiator.go` - 协议协商器（~120 行）
4. `config.go` - 配置（~40 行）
5. `errors.go` - 错误定义（~30 行）
6. `module.go` - Fx 模块（~50 行）
7. `doc.go` - 包文档（~80 行）

**系统协议**（2 文件，v1.0）:
1. `system/ping/ping.go` - Ping 协议（~150 行）
2. `system/identify/identify.go` - Identify 协议（~150 行，简化版 JSON）

**总代码量**: 约 870 行

### 10.2 测试覆盖

**目标覆盖率**: 70%+

**关键测试**:
- Registry 操作测试
- Router 路由测试
- Negotiator 协商测试
- Ping 回显测试
- Identify 信息交换测试
- 并发安全测试

---

## 十一、风险与挑战

### 11.1 技术风险

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| Host 循环依赖 | protocol 依赖 host，host 依赖 protocol | 使用最小接口解耦 |
| 协议协商失败 | 流无法路由 | 充分测试 + 错误处理 |
| Identify 消息格式 | 与 libp2p 不兼容 | 使用 JSON（v1.0），后续可迁移 Protobuf |

### 11.2 设计挑战

| 挑战 | 说明 | 解决方案 |
|------|------|---------|
| Host 接口缺失 | core_host 未实现 | 定义最小接口 |
| Stream 接口 | 需要 Protocol() 方法 | 已在 interfaces 中定义 |
| 并发安全 | 多个流同时路由 | RWMutex 保护 Registry |

---

## 十二、总结

### 12.1 设计优点

✅ **清晰分层** - Registry、Router、Negotiator 职责分离  
✅ **可扩展** - 易于添加新协议  
✅ **标准协商** - 使用 multistream-select  
✅ **简化务实** - v1.0 仅实现核心功能

### 12.2 实施策略

**v1.0** (本次):
- 协议框架（Registry、Router、Negotiator）
- Ping 协议
- Identify 协议（JSON 编码）

**v1.1+** (后续):
- Identify Push
- Protobuf 编码
- AutoNAT、HolePunch、Relay

### 12.3 下一步

1. ✅ 设计审查完成
2. ⏭️ 接口验证
3. ⏭️ 测试先行
4. ⏭️ 核心实现

---

**审查结论**: ✅ **设计合理，可以开始实施**

**关键要点**:
- 参考 core_upgrader 的 multistream-select 实现
- 参考 go-libp2p 的 Ping 和 Identify 协议
- v1.0 专注核心功能，保持简洁

**审查完成时间**: 2026-01-13
