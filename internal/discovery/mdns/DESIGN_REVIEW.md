# discovery_mdns 设计审查

> **日期**: 2026-01-14  
> **版本**: v1.0.0  
> **参考**: go-libp2p/p2p/discovery/mdns

---

## 一、go-libp2p mDNS 实现分析

### 1.1 核心架构

go-libp2p 的 mDNS 实现使用 **zeroconf 库**进行服务注册和发现：

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    go-libp2p mDNS 架构                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  mdnsService                                                                │
│  ├─ host (libp2p host)                                                     │
│  ├─ serviceName (默认 "_p2p._udp")                                         │
│  ├─ peerName (随机 32-63 字符)                                             │
│  ├─ ctx + ctxCancel (生命周期控制)                                         │
│  ├─ server (*zeroconf.Server) - 服务广播                                   │
│  ├─ resolver (goroutine) - 服务发现                                        │
│  ├─ notifee (Notifee接口) - 节点发现回调                                   │
│  └─ resolverWG (sync.WaitGroup) - Resolver 同步                            │
│                                                                             │
│  工作流程：                                                                  │
│  1. Start() -> startServer() + startResolver()                             │
│  2. startServer():                                                          │
│     • 获取 Host 监听地址                                                    │
│     • 过滤适合 mDNS 的地址 (isSuitableForMDNS)                             │
│     • 构建 TXT 记录 (dnsaddr=<multiaddr>)                                   │
│     • 调用 zeroconf.RegisterProxy()                                         │
│  3. startResolver():                                                        │
│     • 启动 zeroconf.Resolver.Browse()                                       │
│     • 监听 ServiceEntry channel                                             │
│     • 解析 TXT 记录提取 multiaddr                                           │
│     • 构建 peer.AddrInfo                                                    │
│     • 调用 notifee.HandlePeerFound()                                        │
│  4. Close() -> 取消 ctx，关闭 server，等待 resolver                         │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 1.2 关键组件

#### 1. zeroconf 库

**包**: `github.com/libp2p/zeroconf/v2`

**核心 API**:
```go
// 注册服务（Server）
server, err := zeroconf.RegisterProxy(
    instance string,      // 服务实例名（随机字符串）
    service string,       // 服务类型 "_p2p._udp"
    domain string,        // "local"
    port int,             // 4001（占位，实际不用）
    host string,          // 主机名
    ips []string,         // IP 地址列表
    txt []string,         // TXT 记录（存储 multiaddr）
    ifaces []net.Interface, // nil = 所有接口
)

// 发现服务（Resolver）
resolver, err := zeroconf.NewResolver(nil)
entries := make(chan *zeroconf.ServiceEntry)
err = resolver.Browse(ctx, "_p2p._udp", "local", entries)

// ServiceEntry 结构
type ServiceEntry struct {
    ServiceInstanceName string
    Text                []string  // TXT 记录
    // ... 其他字段
}
```

#### 2. Notifee 模式

go-libp2p 使用**回调模式**通知节点发现：

```go
type Notifee interface {
    HandlePeerFound(peer.AddrInfo)
}

// 在 resolver goroutine 中调用
for entry := range entryChan {
    // 解析 entry.Text
    for _, info := range infos {
        if info.ID == s.host.ID() {
            continue  // 跳过自己
        }
        go s.notifee.HandlePeerFound(info)
    }
}
```

#### 3. 地址过滤

**函数**: `isSuitableForMDNS(addr ma.Multiaddr) bool`

**过滤规则**:
```go
// 1. 必须以 IP4/IP6 或 .local DNS 开头
switch first.Protocol().Code {
case ma.P_IP4, ma.P_IP6:
    // ✅ 直接 IP 地址适合 LAN 发现
case ma.P_DNS, ma.P_DNS4, ma.P_DNS6, ma.P_DNSADDR:
    // ✅ 只有 .local TLD 适合（mDNS 域）
    if !strings.HasSuffix(strings.ToLower(first.Value()), ".local") {
        return false
    }
default:
    return false
}

// 2. 不能包含不适合的协议
return !containsUnsuitableProtocol(addr)
```

**不适合协议**:
- `P_CIRCUIT` - Circuit Relay（需要中继，不是直连）
- `P_WEBTRANSPORT`, `P_WEBRTC`, `P_WEBRTC_DIRECT` - 浏览器传输
- `P_WS`, `P_WSS` - WebSocket（浏览器不用 mDNS）

**原因**: 减少 mDNS 数据包大小，保持在 RFC 6762 推荐的 1500 字节限制内。

#### 4. TXT 记录格式

```go
const dnsaddrPrefix = "dnsaddr="

// 构建 TXT 记录
var txts []string
for _, addr := range addrs {
    if isSuitableForMDNS(addr) {
        txts = append(txts, dnsaddrPrefix+addr.String())
    }
}

// 示例 TXT 记录：
// "dnsaddr=/ip4/192.168.1.100/tcp/4001/p2p/12D3KooW..."
```

#### 5. 随机 PeerName

```go
peerName := randomString(32 + rand.Intn(32))  // 32-63 字符

// 目的：避免实例名冲突
```

### 1.3 生命周期管理

```go
// 启动
func (s *mdnsService) Start() error {
    if err := s.startServer(); err != nil {
        return err
    }
    s.startResolver(s.ctx)
    return nil
}

// 关闭
func (s *mdnsService) Close() error {
    s.ctxCancel()           // 取消上下文
    if s.server != nil {
        s.server.Shutdown() // 关闭服务器
    }
    s.resolverWG.Wait()     // 等待 Resolver goroutine
    return nil
}
```

### 1.4 IP 地址提取

```go
func (s *mdnsService) getIPs(addrs []ma.Multiaddr) ([]string, error) {
    var ip4, ip6 string
    for _, addr := range addrs {
        first, _ := ma.SplitFirst(addr)
        if first == nil {
            continue
        }
        if ip4 == "" && first.Protocol().Code == ma.P_IP4 {
            ip4 = first.Value()
        } else if ip6 == "" && first.Protocol().Code == ma.P_IP6 {
            ip6 = first.Value()
        }
    }
    ips := make([]string, 0, 2)
    if ip4 != "" {
        ips = append(ips, ip4)
    }
    if ip6 != "" {
        ips = append(ips, ip6)
    }
    if len(ips) == 0 {
        return nil, errors.New("didn't find any IP addresses")
    }
    return ips, nil
}
```

**说明**: mDNS 规范要求发送 A/AAAA 记录，但 libp2p 实际只用 TXT 记录。

---

## 二、DeP2P 适配策略

### 2.1 类型映射

| go-libp2p | DeP2P | 说明 |
|-----------|-------|------|
| `host.Host` | `pkgif.Host` | 使用 DeP2P Host 接口 |
| `peer.AddrInfo` | `types.PeerInfo` | 节点信息类型 |
| `peer.ID` | `types.PeerID` | 节点 ID 类型 |
| `ma.Multiaddr` | `types.Multiaddr` | 多地址类型 |
| `Notifee` | 内部 channel | 替换为 channel 模式 |

### 2.2 接口实现

DeP2P 实现 `pkg/interfaces/discovery.go` 的 `Discovery` 接口：

```go
type Discovery interface {
    FindPeers(ctx, ns string, opts ...DiscoveryOption) (<-chan types.PeerInfo, error)
    Advertise(ctx, ns string, opts ...DiscoveryOption) (time.Duration, error)
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}
```

**映射关系**:
- `FindPeers` -> 启动 Resolver，返回 peer channel（替代 Notifee）
- `Advertise` -> 启动 Server，返回 TTL
- `Start` -> Server + Resolver 同时启动
- `Stop` -> 关闭 Server + Resolver

### 2.3 ServiceName 配置

```go
const DefaultServiceName = "_dep2p._udp"

// go-libp2p 使用 "_p2p._udp"
// DeP2P 使用独立命名空间避免冲突
```

### 2.4 Notifee -> Channel 模式

**go-libp2p**:
```go
type Notifee interface {
    HandlePeerFound(peer.AddrInfo)
}
```

**DeP2P**:
```go
// FindPeers 返回 channel
func (m *MDNS) FindPeers(ctx, ns string, opts...) (<-chan types.PeerInfo, error) {
    peerCh := make(chan types.PeerInfo, 100)
    
    // 内部 notifee 推送到 channel
    go m.startResolver(ctx, peerCh)
    
    return peerCh, nil
}
```

---

## 三、v1.0 实现范围

### 3.1 核心功能（完整实现）

✅ **服务广播 (Advertise)**
- zeroconf.RegisterProxy() 注册服务
- TXT 记录存储 multiaddr
- 地址过滤（isSuitableForMDNS）
- 随机 PeerName 生成

✅ **服务发现 (FindPeers)**
- zeroconf.Resolver.Browse() 监听服务
- TXT 记录解析
- peer.AddrInfo 构建
- 通过 channel 返回节点

✅ **地址过滤**
- IP4/IP6 地址检查
- 不适合协议排除（circuit, websocket, webrtc）
- .local 域名支持

✅ **Discovery 接口**
- FindPeers 实现
- Advertise 实现
- Start/Stop 生命周期

✅ **并发安全**
- atomic.Bool 状态控制
- sync.RWMutex 保护共享状态
- sync.WaitGroup Resolver 同步

✅ **错误处理**
- 预定义错误类型
- 自定义 MDNSError
- 详细错误信息

### 3.2 v1.1 扩展（技术债）

⬜ **TD-MDNS-001: TTL 控制**
- 自定义广播 TTL
- 当前固定返回 10s

⬜ **TD-MDNS-002: 查询间隔**
- Interval 参数实际使用
- 当前 zeroconf 库控制

⬜ **TD-MDNS-003: 多播分组**
- 支持多个 ServiceTag
- 当前单一 ServiceTag

---

## 四、关键设计决策

### 决策 1: 使用 zeroconf 库

**选择**: `github.com/grandcat/zeroconf`

**原因**:
1. go-libp2p 已验证的实现
2. 纯 Go 实现，跨平台
3. 活跃维护
4. 支持 mDNS/DNS-SD 标准

**替代方案**: `github.com/hashicorp/mdns`（go-libp2p 早期使用，已弃用）

### 决策 2: Channel 替代 Notifee

**选择**: `FindPeers` 返回 `<-chan types.PeerInfo`

**原因**:
1. 符合 Discovery 接口规范
2. 更 Go 惯用（channel 通信）
3. 上下文取消更自然
4. 与 DHT, Bootstrap 一致

**实现**:
```go
// 内部 notifee 推送到 channel
type peerNotifee struct {
    peerCh chan<- types.PeerInfo
}

func (n *peerNotifee) handleEntry(entry *zeroconf.ServiceEntry) {
    // 解析 entry
    n.peerCh <- peerInfo
}
```

### 决策 3: 独立 ServiceName

**选择**: `_dep2p._udp`

**原因**:
1. 避免与 go-libp2p 节点冲突
2. DeP2P 独立命名空间
3. 便于过滤和调试

### 决策 4: 地址过滤策略

**选择**: 完全复制 go-libp2p 的 `isSuitableForMDNS` 逻辑

**原因**:
1. 已经过生产验证
2. 符合 RFC 6762 限制
3. 避免不必要的网络流量

---

## 五、架构对比

| 维度 | go-libp2p | DeP2P mdns v1.0 |
|------|-----------|-----------------|
| **库依赖** | zeroconf/v2 | 相同 |
| **Server** | RegisterProxy | 相同 |
| **Resolver** | Browse | 相同 |
| **Notifee** | 接口回调 | Channel 模式 |
| **Discovery接口** | 无 | 完整实现 |
| **ServiceName** | `_p2p._udp` | `_dep2p._udp` |
| **地址过滤** | isSuitableForMDNS | 相同逻辑 |
| **并发控制** | WaitGroup | WaitGroup + atomic |
| **配置化** | 硬编码 | Config 结构 |
| **Fx集成** | 无 | 完整模块 |

---

## 六、依赖关系

```
discovery_mdns
├─ core_host (pkgif.Host)
│  ├─ ID() string
│  └─ Addrs() []string
├─ pkg/types
│  ├─ PeerInfo
│  ├─ PeerID
│  └─ Multiaddr
├─ pkg/interfaces
│  └─ Discovery
└─ github.com/grandcat/zeroconf
   ├─ RegisterProxy()
   ├─ NewResolver()
   └─ ServiceEntry
```

---

## 七、实施建议

### 7.1 分步实现

1. **Phase 1**: 配置和错误定义
2. **Phase 2**: Notifee + 地址过滤
3. **Phase 3**: MDNS 主逻辑（Server + Resolver）
4. **Phase 4**: Discovery 接口实现
5. **Phase 5**: Fx 模块集成

### 7.2 测试策略

**单元测试**:
- Mock Host 测试配置和逻辑
- 地址过滤函数测试
- 并发安全测试

**集成测试**:
- 两个 Host 互发现（需真实 Host）
- 多节点发现场景
- ServiceName 隔离测试

### 7.3 注意事项

1. **IP 地址提取**: 需要至少一个 IP4 或 IP6
2. **TXT 记录大小**: 保持在 1500 字节内
3. **随机 PeerName**: 避免实例名冲突
4. **Context 取消**: 确保 Resolver goroutine 正确退出
5. **跳过自己**: Resolver 需要过滤自己的 PeerID

---

## 八、代码量估算

| 组件 | 预估行数 | 说明 |
|------|---------|------|
| config.go | 120 | 配置管理 |
| errors.go | 60 | 错误定义 |
| mdns.go | 350 | 主逻辑 |
| notifee.go | 80 | Notifee + 过滤 |
| module.go | 70 | Fx 模块 |
| doc.go | 150 | 文档 |
| **实现小计** | **830** | |
| mdns_test.go | 600 | 单元测试（15个） |
| integration_test.go | 150 | 集成测试（3个） |
| **测试小计** | **750** | |
| **总计** | **~1580** | 实现 + 测试 |

---

**最后更新**: 2026-01-14  
**审查结论**: ✅ 设计清晰，可直接实施
