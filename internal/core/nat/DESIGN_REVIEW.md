# Core NAT 设计审查

> **日期**: 2026-01-13  
> **版本**: v1.0.0  
> **审查人**: AI Assistant

---

## 一、go-libp2p 参考实现分析

### 1.1 AutoNAT 检测机制

**文件**: `p2p/host/autonat/autonat.go`, `client.go`, `svc.go`

#### 核心设计

```go
// 客户端：请求远程节点拨回我们的地址
type client struct {
    h        host.Host
    addrFunc AddrFunc  // 返回需要测试的地址
}

// DialBack 请求对方拨回我们的地址
func (c *client) DialBack(ctx context.Context, p peer.ID) error {
    // 1. 建立流到 AutoNAT 服务节点
    // 2. 发送 DIAL 请求（包含我们的地址）
    // 3. 等待 DIAL_RESPONSE
    // 4. 根据响应判断可达性
}
```

**服务端逻辑**（`svc.go:handleDial`）：
1. 接收客户端的拨号请求
2. 验证请求（peer ID 匹配、地址合法性）
3. 尝试拨回客户端提供的地址
4. 返回结果：OK（成功）/ E_DIAL_ERROR（失败）/ E_DIAL_REFUSED（拒绝）

#### 状态机（`autonat.go:backgroundLoop`）

```
Unknown → (探测成功3次) → Public
Unknown → (探测失败3次) → Private
Public  → (连续失败)     → Private
Private → (连续成功)     → Public
```

**关键参数**：
- `maxConfidence = 3` - 需要3次一致的结果才改变状态
- `recentProbes` - 避免重复探测同一节点
- `ProbeInterval` - 探测间隔（默认15秒）

#### 限流机制（`svc.go`）

```go
type autoNATService struct {
    reqs       map[peer.ID]int  // 每个节点的请求计数
    globalReqs int              // 全局请求计数
}

// 限流规则：
// - 每个节点每分钟最多3次请求
// - 全局每分钟最多30次请求
// - 拒绝拨号到私有地址
```

---

### 1.2 Hole Punching 协议

**文件**: `p2p/protocol/holepunch/holepuncher.go`, `svc.go`

#### 协议流程

```
节点 A (NAT后)                Relay                节点 B (NAT后)
     │                         │                         │
     ├─[通过 Relay 连接]────────┤─[通过 Relay 连接]──────┤
     │                         │                         │
     ├──[CONNECT 消息]────────>│                         │
     │  (角色: Initiator)      │                         │
     │                         ├─[转发 CONNECT]────────>│
     │                         │  (角色: Receiver)       │
     │                         │                         │
     │                         │<──[CONNECT 响应]───────┤
     │<──[转发响应]─────────────┤  (包含观察地址)        │
     │  (包含观察地址)         │                         │
     │                         │                         │
     ├═══[同时发送 UDP 包]═══════════════════════════>│
     │<═══════════════════════[同时发送 UDP 包]══════┤
     │                         │                         │
     ├─[尝试直连]──────────────────────────────────────>│
     │<─[建立直连]───────────────────────────────────────┤
```

#### 核心实现（`holepuncher.go:DirectConnect`）

```go
func (hp *holePuncher) DirectConnect(p peer.ID) error {
    // 1. 去重：检查是否已有活跃的打洞
    if _, ok := hp.active[p]; ok {
        return ErrHolePunchActive
    }
    hp.active[p] = struct{}{}
    defer delete(hp.active, p)
    
    // 2. 尝试直连（如果有公网地址）
    if hasPublicAddr(peer) {
        if tryDirectDial(peer) == nil {
            return nil
        }
    }
    
    // 3. 通过中继协商打洞
    stream := openRelayStream(peer, Protocol)
    
    // 4. 交换观察地址
    sendAddrs(stream, localAddrs)
    remoteAddrs := recvAddrs(stream)
    
    // 5. 发起 SYNC 消息
    sendSync(stream)
    recvSync(stream)
    
    // 6. 同时进行打洞（UDP）
    holePunch(remoteAddrs)
    
    // 7. 等待连接建立
    return waitForConnection(peer)
}
```

#### 关键设计点

1. **角色协商**：
   - Initiator：主动发起打洞的一方
   - Receiver：被动接受打洞的一方
   - 通过比较 Peer ID 大小决定角色

2. **地址过滤**（`filter.go`）：
   - 仅尝试打洞公网地址
   - 跳过中继地址
   - 过滤本地地址

3. **超时控制**：
   - `StreamTimeout = 1分钟` - 协商流超时
   - `directDialTimeout = 10秒` - 直连尝试超时

4. **去重机制**：
   - `active map[peer.ID]struct{}` - 防止对同一节点重复打洞

---

### 1.3 协议消息格式

#### AutoNAT 协议（protobuf）

```protobuf
message Message {
    enum MessageType {
        DIAL = 0;
        DIAL_RESPONSE = 1;
    }
    
    enum ResponseStatus {
        OK = 0;
        E_DIAL_ERROR = 100;
        E_DIAL_REFUSED = 101;
        E_BAD_REQUEST = 200;
        E_INTERNAL_ERROR = 300;
    }
    
    optional MessageType type = 1;
    optional Dial dial = 2;
    optional DialResponse dialResponse = 3;
}

message Dial {
    optional PeerInfo peer = 1;
}

message DialResponse {
    optional ResponseStatus status = 1;
    optional string statusText = 2;
    optional bytes addr = 3;
}
```

#### Hole Punch 协议（protobuf）

```protobuf
message HolePunch {
    enum Type {
        CONNECT = 100;
        SYNC = 300;
    }
    
    optional Type type = 1;
    repeated bytes ObsAddrs = 2;
}
```

---

## 二、STUN 协议要点

### 2.1 STUN Binding Request/Response

**库选择**: `github.com/pion/stun` (成熟的 Go STUN 实现)

#### 基本流程

```go
import "github.com/pion/stun"

// 1. 连接 STUN 服务器
conn, _ := net.DialUDP("udp", nil, stunAddr)
defer conn.Close()

// 2. 构造 Binding Request
msg := stun.MustBuild(stun.TransactionID, stun.BindingRequest)

// 3. 发送请求
msg.WriteTo(conn)

// 4. 接收响应
buf := make([]byte, 1500)
n, _ := conn.Read(buf)

// 5. 解析 MAPPED-ADDRESS
res := new(stun.Message)
res.Raw = buf[:n]
res.Decode()

var xorAddr stun.XORMappedAddress
xorAddr.GetFrom(res)

externalAddr := &net.UDPAddr{
    IP:   xorAddr.IP,
    Port: xorAddr.Port,
}
```

#### STUN 服务器列表

公共 STUN 服务器：
- `stun.l.google.com:19302` (Google)
- `stun1.l.google.com:19302`
- `stun2.l.google.com:19302`
- `stun.stunprotocol.org:3478`

**建议**：配置多个服务器，轮询/故障转移

---

### 2.2 超时和重试策略

```go
type STUNClient struct {
    servers []string
    timeout time.Duration  // 默认 5 秒
    retries int           // 默认 3 次
}

func (s *STUNClient) GetExternalAddr(ctx context.Context) (*net.UDPAddr, error) {
    for _, server := range s.servers {
        for retry := 0; retry < s.retries; retry++ {
            addr, err := s.queryServer(ctx, server)
            if err == nil {
                return addr, nil
            }
            // 指数退避
            time.Sleep(time.Duration(1<<retry) * time.Second)
        }
    }
    return nil, ErrSTUNTimeout
}
```

---

## 三、UPnP 和 NAT-PMP

### 3.1 UPnP 实现

**库选择**: `github.com/huin/goupnp` (标准 UPnP 库)

```go
import (
    "github.com/huin/goupnp"
    "github.com/huin/goupnp/dcps/internetgateway2"
)

// 1. 发现 IGD (Internet Gateway Device)
clients, _ := internetgateway2.NewWANIPConnection1Clients()
if len(clients) == 0 {
    return ErrNoUPnPDevice
}

// 2. 添加端口映射
client := clients[0]
err := client.AddPortMapping(
    "",           // NewRemoteHost (空表示任意)
    externalPort, // NewExternalPort
    "UDP",        // NewProtocol
    internalPort, // NewInternalPort
    localIP,      // NewInternalClient
    true,         // NewEnabled
    "dep2p",      // NewPortMappingDescription
    3600,         // NewLeaseDuration (秒)
)
```

### 3.2 NAT-PMP 实现

**库选择**: `github.com/jackpal/gateway` (检测网关) + 自实现

```go
import "github.com/jackpal/gateway"

// 1. 获取默认网关
gatewayIP, _ := gateway.DiscoverGateway()

// 2. NAT-PMP 协议（简单 UDP）
conn, _ := net.DialUDP("udp", nil, &net.UDPAddr{
    IP:   gatewayIP,
    Port: 5351, // NAT-PMP 标准端口
})

// 3. 发送映射请求
req := natpmpRequest{
    Version:  0,
    Opcode:   1, // MAP UDP
    Internal: uint16(internalPort),
    External: uint16(externalPort),
    Lifetime: 3600,
}
conn.Write(req.Encode())

// 4. 读取响应
buf := make([]byte, 16)
conn.Read(buf)
resp := parseNATPMPResponse(buf)
```

---

## 四、DeP2P v1.0 简化策略

### 4.1 简化内容

| 特性 | go-libp2p | DeP2P v1.0 | 原因 |
|------|-----------|------------|------|
| AutoNAT 服务端 | ✅ 完整实现 | ⬜ 延后到 v1.1 | v1.0 仅作为客户端 |
| Hole Punch 完整协商 | ✅ CONNECT+SYNC | ⚠️ 简化版 | 无中继时仅记录框架 |
| 黑洞检测 | ✅ | ⬜ v1.1+ | 复杂度高 |
| Backoff 策略 | ✅ | ⬜ v1.1+ | 暂不需要 |
| 复杂 NAT 穿透 | ✅ 对称 NAT | ⬜ v1.1+ | 成功率低 |

### 4.2 v1.0 实现范围

**核心功能**：
1. ✅ AutoNAT 客户端 - 检测自己的可达性
2. ✅ STUN 客户端 - 获取外部地址
3. ✅ UPnP 映射 - 自动端口映射
4. ✅ NAT-PMP 映射 - 备选映射方案
5. ⚠️ Hole Punch 框架 - 基础结构（完整实现需要 core_relay）

**状态管理**：
```go
type Reachability int

const (
    ReachabilityUnknown  Reachability = iota  // 初始状态
    ReachabilityPublic                        // 公网可达
    ReachabilityPrivate                       // NAT 后
)
```

### 4.3 依赖关系

```
core_nat 依赖：
├── core_swarm    (必需：获取连接、地址信息)
├── core_eventbus (可选：发布可达性变化事件)
└── 外部库
    ├── github.com/pion/stun
    ├── github.com/huin/goupnp
    └── github.com/jackpal/gateway
```

---

## 五、架构设计

### 5.1 模块结构

```
internal/core/nat/
├── service.go              # NAT 服务主入口
│   ├── Start()            - 启动所有子服务
│   ├── Stop()             - 停止服务
│   ├── Reachability()     - 返回当前可达性
│   └── ExternalAddrs()    - 返回外部地址列表
│
├── autonat.go              # AutoNAT 检测器
│   ├── runProbeLoop()     - 定期探测
│   ├── probeRandomPeer()  - 选择节点探测
│   └── updateStatus()     - 更新可达性状态
│
├── stun/
│   └── stun.go             # STUN 客户端
│       ├── GetExternalAddr()
│       └── queryServer()
│
├── upnp/
│   └── upnp.go             # UPnP 映射器
│       ├── MapPort()
│       ├── UnmapPort()
│       └── renewMappings() - 定期续期
│
├── natpmp/
│   └── natpmp.go           # NAT-PMP 映射器
│       ├── MapPort()
│       └── UnmapPort()
│
├── holepunch/
│   ├── puncher.go          # 打洞协调器
│   │   └── DirectConnect()
│   └── protocol.go         # 打洞协议定义
│
├── config.go               # 配置
├── errors.go               # 错误定义
├── module.go               # Fx 模块
└── doc.go                  # 包文档
```

### 5.2 并发模型

```go
type Service struct {
    ctx    context.Context
    cancel context.CancelFunc
    wg     sync.WaitGroup
    
    // 子组件各自管理自己的 goroutine
    autonat *AutoNAT  // 1 个 probeLoop goroutine
    upnp    *UPnP     // 1 个 renewal goroutine
    natpmp  *NATPMP   // 1 个 renewal goroutine
}

// 生命周期管理
func (s *Service) Start(ctx context.Context) error {
    s.ctx, s.cancel = context.WithCancel(ctx)
    
    // 启动 AutoNAT 探测循环
    if s.config.EnableAutoNAT {
        s.wg.Add(1)
        go s.autonat.runProbeLoop(s.ctx)
    }
    
    // 启动 UPnP 映射续期
    if s.config.EnableUPnP {
        s.wg.Add(1)
        go s.upnp.renewLoop(s.ctx)
    }
    
    return nil
}

func (s *Service) Stop() error {
    s.cancel()
    s.wg.Wait()
    return nil
}
```

---

## 六、实现要点

### 6.1 AutoNAT 探测逻辑

```go
func (a *AutoNAT) runProbeLoop(ctx context.Context) {
    ticker := time.NewTicker(a.config.ProbeInterval)
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            a.probe(ctx)
        }
    }
}

func (a *AutoNAT) probe(ctx context.Context) {
    // 1. 选择支持 AutoNAT 的随机节点
    peers := a.selectProbePeers()
    if len(peers) == 0 {
        return
    }
    
    // 2. 请求拨回
    for _, p := range peers {
        err := a.client.DialBack(ctx, p)
        
        // 3. 更新状态
        if err == nil {
            a.recordSuccess()
        } else {
            a.recordFailure()
        }
        
        // 4. 检查是否达到置信度
        if a.shouldUpdateStatus() {
            a.updateReachability()
        }
    }
}
```

### 6.2 STUN 地址缓存

```go
type STUNClient struct {
    servers []string
    
    // 缓存外部地址（避免频繁查询）
    cachedAddr   *net.UDPAddr
    cachedTime   time.Time
    cacheDuration time.Duration // 默认 5 分钟
}

func (s *STUNClient) GetExternalAddr(ctx context.Context) (*net.UDPAddr, error) {
    // 使用缓存
    if time.Since(s.cachedTime) < s.cacheDuration && s.cachedAddr != nil {
        return s.cachedAddr, nil
    }
    
    // 查询新地址
    addr, err := s.queryServers(ctx)
    if err != nil {
        return nil, err
    }
    
    // 更新缓存
    s.cachedAddr = addr
    s.cachedTime = time.Now()
    
    return addr, nil
}
```

### 6.3 端口映射续期

```go
type Mapping struct {
    Protocol     string
    InternalPort int
    ExternalPort int
    Duration     time.Duration
    CreatedAt    time.Time
}

func (u *UPnPMapper) renewLoop(ctx context.Context) {
    ticker := time.NewTicker(30 * time.Minute) // 每30分钟检查一次
    defer ticker.Stop()
    
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            u.renewMappings(ctx)
        }
    }
}

func (u *UPnPMapper) renewMappings(ctx context.Context) {
    for _, m := range u.mappings {
        // 在租期到期前续期
        if time.Since(m.CreatedAt) > m.Duration*2/3 {
            u.MapPort(ctx, m.Protocol, m.InternalPort)
        }
    }
}
```

---

## 七、测试策略

### 7.1 单元测试

```go
// AutoNAT 状态转换测试
func TestAutoNAT_StatusTransition(t *testing.T) {
    an := NewAutoNAT(...)
    
    // 模拟3次成功探测
    for i := 0; i < 3; i++ {
        an.recordSuccess()
    }
    
    if an.Reachability() != ReachabilityPublic {
        t.Error("Expected Public after 3 successes")
    }
}

// STUN 超时测试
func TestSTUN_Timeout(t *testing.T) {
    client := NewSTUNClient([]string{"invalid:9999"})
    
    ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
    defer cancel()
    
    _, err := client.GetExternalAddr(ctx)
    if err != ErrSTUNTimeout {
        t.Error("Expected timeout error")
    }
}
```

### 7.2 集成测试

需要模拟环境：
- Mock Swarm（提供连接信息）
- Mock AutoNAT 服务器（响应探测请求）
- 本地 STUN 服务器

---

## 八、风险和限制

### 8.1 已知限制

1. **AutoNAT 依赖**：需要网络中有提供 AutoNAT 服务的节点
2. **STUN 可靠性**：依赖公共 STUN 服务器的可用性
3. **UPnP 覆盖率**：不是所有路由器都支持 UPnP
4. **Hole Punching**：v1.0 无中继支持，打洞成功率低

### 8.2 安全考虑

1. **限流**：防止被滥用探测（v1.1 实现服务端时）
2. **地址验证**：拒绝拨号到私有地址
3. **超时控制**：防止资源耗尽

---

## 九、总结

### 9.1 设计决策

✅ **采用 go-libp2p 成熟设计** - AutoNAT 协议、STUN、UPnP  
✅ **v1.0 简化范围** - 仅客户端功能，完整打洞延后  
✅ **模块化架构** - service/autonat/stun/upnp/natpmp 分离  
✅ **使用成熟库** - pion/stun, huin/goupnp  

### 9.2 实施优先级

**P0（必须）**：
- AutoNAT 客户端检测
- STUN 外部地址获取
- Service 主入口和状态管理

**P1（重要）**：
- UPnP 端口映射
- NAT-PMP 端口映射

**P2（可选）**：
- Hole Punching 框架（完整实现需 core_relay）

---

**审查完成日期**: 2026-01-13  
**下一步**: Step 2 接口验证（跳过）→ Step 3 测试先行
