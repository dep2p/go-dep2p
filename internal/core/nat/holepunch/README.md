# 打洞实现

## 概述

NAT 打洞实现，支持 UDP 和 TCP 两种打洞方式，用于穿透 NAT 建立直接连接。

## 文件结构

```
holepunch/
├── README.md        # 本文件
├── puncher.go       # UDP/QUIC 打洞器
├── tcp_puncher.go   # TCP 打洞器 (Simultaneous Open)
├── protocol.go      # 打洞协议消息定义
└── holepunch_test.go # 单元测试
```

## 核心实现

### puncher.go - UDP 打洞器

```go
// Puncher 打洞器实现（实现 natif.HolePuncher 接口）
type Puncher struct {
    config Config
    logger *zap.Logger
}

// NewPuncher 创建打洞器
func NewPuncher(config Config, logger *zap.Logger) *Puncher

// Punch 尝试 UDP 打洞连接
func (p *Puncher) Punch(ctx context.Context, remoteID types.NodeID, remoteAddrs []endpoint.Address) (endpoint.Address, error)

// StartRendezvous 启动打洞协调
func (p *Puncher) StartRendezvous(ctx context.Context, remoteID types.NodeID) error
```

### tcp_puncher.go - TCP 打洞器

```go
// TCPPuncher TCP 打洞器实现（实现 natif.TCPHolePuncher 接口）
type TCPPuncher struct {
    config TCPConfig
    logger *zap.Logger
}

// NewTCPPuncher 创建 TCP 打洞器
func NewTCPPuncher(config TCPConfig, logger *zap.Logger) *TCPPuncher

// PunchTCP 尝试 TCP 打洞连接
func (p *TCPPuncher) PunchTCP(ctx context.Context, remoteID types.NodeID, remoteAddrs []endpoint.Address) (conn interface{}, addr endpoint.Address, err error)

// PunchTCPWithLocalPort 使用指定本地端口尝试 TCP 打洞
func (p *TCPPuncher) PunchTCPWithLocalPort(ctx context.Context, remoteID types.NodeID, remoteAddrs []endpoint.Address, localPort int) (conn interface{}, addr endpoint.Address, err error)
```

### protocol.go - 协议消息

```go
// HolePunchRequest 打洞请求消息
type HolePunchRequest struct {
    InitiatorID    types.NodeID
    InitiatorAddrs []endpoint.Address
    ResponderID    types.NodeID
}

// HolePunchConnect 打洞连接消息
type HolePunchConnect struct {
    InitiatorAddrs []endpoint.Address
    ResponderAddrs []endpoint.Address
    Nonce          []byte
}

// HolePunchSync 打洞同步消息
type HolePunchSync struct {
    Nonce []byte
}

// HolePunchResponse 打洞响应消息
type HolePunchResponse struct {
    Success bool
    Nonce   []byte
    Error   string
}
```

## 打洞流程

```
Node A                    Relay                    Node B
   |                        |                        |
   |-- Reserve ------------>|                        |
   |<-- Relay Addr ---------|                        |
   |                        |                        |
   |                        |<-- Reserve ------------|
   |                        |-- Relay Addr --------->|
   |                        |                        |
   |== Connect via Relay ==>|<== Connect via Relay ==|
   |                        |                        |
   |-- HolePunch Request -->|-- Forward ------------>|
   |<-- HolePunch Response -|<-- Forward ------------|
   |                        |                        |
   |.......... Simultaneous UDP/QUIC packets .......|
   |<================== Direct Connection =========>|
```

## 打洞策略

### UDP 打洞

1. **同时发送**: 双方同时向对方发送数据包
2. **多地址尝试**: 并行尝试所有已知地址
3. **Nonce 验证**: 使用 16 字节随机 nonce 验证响应
4. **超时重试**: 多轮尝试直到成功或超时

### TCP 打洞 (Simultaneous Open)

1. **SO_REUSEADDR/SO_REUSEPORT**: 允许端口复用
2. **同时连接**: 双方同时发起 TCP 连接
3. **多轮重试**: 由于时序要求严格，需要多轮尝试

## 适用场景

| NAT A 类型 | NAT B 类型 | UDP 打洞 | TCP 打洞 |
|-----------|-----------|---------|---------|
| Full Cone | Any | 高 | 高 |
| Restricted | Full/Restricted | 高 | 中 |
| Port Restricted | Full/Restricted | 中 | 低 |
| Symmetric | Full Cone | 低 | 极低 |
| Symmetric | Symmetric | 极低 | 极低 |

## 配置选项

### UDP 打洞配置

```go
Config{
    MaxAttempts:     5,                   // 最大尝试次数
    AttemptInterval: 200 * time.Millisecond, // 尝试间隔
    Timeout:         10 * time.Second,    // 总超时时间
    PacketSize:      64,                  // 打洞包大小
}
```

### TCP 打洞配置

```go
TCPConfig{
    MaxAttempts:     10,                  // 最大尝试次数
    AttemptInterval: 100 * time.Millisecond, // 尝试间隔
    Timeout:         15 * time.Second,    // 总超时时间
    ConnectTimeout:  2 * time.Second,     // 单次连接超时
    LocalPort:       0,                   // 本地端口（0 自动分配）
    EnableReusePort: true,                // 启用端口复用
}
```

## 实现状态

| 功能 | 状态 |
|------|------|
| UDP 打洞 | ✅ 已实现 |
| TCP 打洞 (Simultaneous Open) | ✅ 已实现 |
| 多地址并行尝试 | ✅ 已实现 |
| Nonce 验证 | ✅ 已实现 |
| 协议消息编解码 | ✅ 已实现 |
| 会话管理 | ✅ 已实现 |
| HolePuncher 接口 | ✅ 已实现 |
| TCPHolePuncher 接口 | ✅ 已实现 |

## 安全考虑

- **Nonce 验证**: 使用 `crypto/rand` 生成 16 字节随机 nonce，防止重放攻击
- **地址数量限制**: 最多处理 16 个地址，防止资源耗尽
- **消息长度验证**: 所有解码操作都有长度检查
