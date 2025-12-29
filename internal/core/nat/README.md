# NAT 穿透模块

## 概述

**层级**: Tier 3  
**职责**: 提供 NAT 类型检测、外部地址发现、端口映射和打洞能力。

## 设计引用

> **重要**: 实现前请详细阅读以下设计规范

| 设计文档 | 说明 |
|----------|------|
| [NAT 穿透协议](../../../docs/01-design/protocols/network/02-nat.md) | STUN、UPnP、打洞机制 |
| [中继协议](../../../docs/01-design/protocols/network/03-relay.md) | 保底中继方案 |

## 能力清单

### NAT 检测能力 (必须实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| STUN 外部地址发现 | ✅ 已实现 | 获取公网 IP:Port |
| NAT 类型检测 | ✅ 已实现 | Full Cone/Restricted/Symmetric |
| 多 STUN 服务器 | ✅ 已实现 | 容错机制 |

### 端口映射能力 (必须实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| UPnP 映射 | ✅ 已实现 | 自动端口映射 |
| NAT-PMP 映射 | ✅ 已实现 | Apple 设备支持 |
| 映射刷新 | ✅ 已实现 | 定期刷新映射 |

### 打洞能力 (必须实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| UDP 打洞 | ✅ 已实现 | 同时发送打洞包 |
| 打洞协调 | ✅ 已实现 | 通过中继交换地址（信令/协调） |
| TCP 打洞 | ✅ 已实现 | TCP 同时连接 (tcp_puncher.go) |

### 内置协议 (必须实现)

| 协议 | 状态 | 说明 |
|------|------|------|
| `/dep2p/sys/holepunch/1.0.0` | ✅ 已实现 | 打洞协调协议（交换候选地址/同步发送） |

## 依赖关系

### 接口依赖

```
pkg/types/           → NATType, Address
pkg/interfaces/core/ → Address 接口
pkg/interfaces/nat/  → NATService, HolePuncher, PortMapper 接口
```

### 模块依赖

```
transport → 底层网络连接
relay     → 打洞协调（交换地址信息）
```

### 第三方依赖

```
github.com/pion/stun      → STUN 协议实现
github.com/huin/goupnp    → UPnP 实现
github.com/jackpal/gateway → 网关发现
```

## 目录结构

```
nat/
├── README.md            # 本文件
├── module.go            # fx 模块定义
├── stun/                # STUN 实现
│   ├── README.md        # STUN 子模块说明
│   └── client.go        # STUN 客户端
├── upnp/                # UPnP 实现
│   ├── README.md        # UPnP 子模块说明
│   └── mapper.go        # UPnP 端口映射
└── holepunch/           # 打洞实现
    ├── README.md        # 打洞子模块说明
    └── puncher.go       # 打洞协调
```

## 公共接口

实现 `pkg/interfaces/nat/` 中的接口：

```go
// NATService NAT 服务接口
type NATService interface {
    // GetExternalAddress 获取外部地址
    GetExternalAddress() (core.Address, error)
    GetExternalAddressWithContext(ctx context.Context) (core.Address, error)
    
    // NATType 返回 NAT 类型
    NATType() types.NATType
    
    // DetectNATType 检测 NAT 类型
    DetectNATType(ctx context.Context) (types.NATType, error)
    
    // MapPort 映射端口
    MapPort(protocol string, internalPort, externalPort int, duration time.Duration) error
    
    // UnmapPort 取消端口映射
    UnmapPort(protocol string, externalPort int) error
    
    // GetMappedPort 获取映射的端口
    GetMappedPort(protocol string, internalPort int) (int, error)
    
    // Refresh 刷新所有映射
    Refresh(ctx context.Context) error
    
    // Close 关闭服务
    Close() error
}

// HolePuncher 打洞器接口
type HolePuncher interface {
    // Punch 尝试打洞连接
    Punch(ctx context.Context, remoteID types.NodeID, remoteAddrs []core.Address) (core.Address, error)
}
```

## 关键算法

### NAT 类型 (来自设计文档)

```go
const (
    NATTypeUnknown         NATType = iota
    NATTypeNone                    // 公网 IP，无 NAT
    NATTypeFullCone               // 完全锥形 - 最开放
    NATTypeRestrictedCone         // 受限锥形
    NATTypePortRestricted         // 端口受限锥形
    NATTypeSymmetric              // 对称型 - 最难穿透
)
```

### NAT 类型检测算法 (来自设计文档)

```
Step 1: 检测是否有 NAT
─────────────────────
发送 STUN 请求
如果返回地址 == 本地地址: 无 NAT，结束

Step 2: 检测是否是 Symmetric NAT
────────────────────────────────
向两个不同 STUN 服务器发送请求
如果返回端口不同: Symmetric NAT

Step 3: 检测 Cone 类型
─────────────────────
请求 STUN 服务器从不同 IP 回复
如果收到: Full Cone

请求 STUN 服务器从相同 IP 不同端口回复
如果收到: Restricted Cone
否则: Port Restricted Cone
```

### 打洞流程 (来自设计文档)

```
节点 A (NAT 后)                  中继服务器               节点 B (NAT 后)
      │                             │                          │
      │  1. 预留请求                │                          │
      │───────────────────────────►│                          │
      │                             │                          │
      │  2. 地址交换请求            │                          │
      │───────────────────────────►│  3. 通知 B               │
      │                             │─────────────────────────►│
      │                             │                          │
      │  4. 开始打洞                │       4. 开始打洞         │
      │◄─────────────────────────────────────────────────────►│
      │         同时发送 UDP 包                                 │
      │                                                        │
      │  5. 连接建立                                           │
      │◄══════════════════════════════════════════════════════►│
```

### STUN 外部地址发现

```go
func (c *stunClient) GetMappedAddress(ctx context.Context) (core.Address, error) {
    for _, server := range c.servers {
        conn, err := net.DialTimeout("udp", server, 5*time.Second)
        if err != nil {
            continue
        }
        defer conn.Close()
        
        // 发送 STUN Binding Request
        msg := stun.MustBuild(stun.TransactionID, stun.BindingRequest)
        conn.Write(msg.Raw)
        
        // 读取响应
        buf := make([]byte, 1024)
        conn.SetReadDeadline(time.Now().Add(5 * time.Second))
        n, err := conn.Read(buf)
        if err != nil {
            continue
        }
        
        // 解析 MAPPED-ADDRESS
        var resp stun.Message
        resp.Raw = buf[:n]
        var xorAddr stun.XORMappedAddress
        if err := xorAddr.GetFrom(&resp); err == nil {
            return parseAddress(xorAddr.IP, xorAddr.Port), nil
        }
    }
    return nil, ErrNoSTUNResponse
}
```

## fx 模块

```go
type ModuleInput struct {
    fx.In
    Transport transportif.Transport `name:"transport"`
    Relay     relayif.RelayClient   `name:"relay" optional:"true"`
    Config    *natif.Config         `optional:"true"`
}

type ModuleOutput struct {
    fx.Out
    NATService  natif.NATService  `name:"nat"`
    HolePuncher natif.HolePuncher `name:"hole_puncher"`
}

func Module() fx.Option {
    return fx.Module("nat",
        fx.Provide(ProvideServices),
        fx.Invoke(registerLifecycle),
    )
}
```

## 默认 STUN 服务器

```go
var defaultSTUNServers = []string{
    "stun.l.google.com:19302",
    "stun1.l.google.com:19302",
    "stun2.l.google.com:19302",
    "stun.cloudflare.com:3478",
    "stun.stunprotocol.org:3478",
}
```

## 配置参数

```go
type Config struct {
    // STUN 配置
    STUNServers      []string
    STUNTimeout      time.Duration  // 默认 5s
    
    // UPnP 配置
    EnableUPnP       bool
    UPnPTimeout      time.Duration  // 默认 3s
    
    // 打洞配置
    EnableHolePunch  bool
    PunchTimeout     time.Duration  // 默认 30s
    PunchAttempts    int            // 默认 5
    
    // 端口映射刷新
    MappingRefresh   time.Duration  // 默认 5min
}
```

## 相关文档

- [NAT 穿透协议](../../../docs/01-design/protocols/network/02-nat.md)
- [中继协议](../../../docs/01-design/protocols/network/03-relay.md)
- [pkg/interfaces/nat](../../../pkg/interfaces/nat/)
