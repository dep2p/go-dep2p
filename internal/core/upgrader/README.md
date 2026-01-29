# upgrader - 连接升级器

> **版本**: v1.0.0  
> **状态**: ✅ 核心实现完成  
> **覆盖率**: 待测试  
> **最后更新**: 2026-01-13

---

## 模块概述

upgrader 模块负责将原始网络连接升级为安全、多路复用的 P2P 连接。

| 属性 | 值 |
|------|-----|
| **架构层** | Core Layer (Level 3) |
| **代码位置** | `internal/core/upgrader/` |
| **Fx 模块** | `fx.Module("upgrader")` |
| **依赖** | security, muxer, identity |

---

## 核心功能

### 升级流程

```
Raw Connection
     ↓
Security Negotiation (multistream-select)
     ↓
Security Handshake (TLS/Noise)
     ↓
Muxer Negotiation (multistream-select)
     ↓
Muxer Setup (yamux)
     ↓
Upgraded Connection
```

### 支持的协议

**安全协议**:
- ✅ TLS 1.3 (`/tls/1.0.0`)
- ⚠️ Noise (`/noise`) - 基础实现

**多路复用器**:
- ✅ yamux (`/yamux/1.0.0`)

---

## 快速开始

### 基本使用

```go
import (
    "github.com/dep2p/go-dep2p/internal/core/upgrader"
    "github.com/dep2p/go-dep2p/internal/core/identity"
    "github.com/dep2p/go-dep2p/internal/core/security/tls"
    "github.com/dep2p/go-dep2p/internal/core/muxer"
)

// 创建身份
id, _ := identity.Generate()

// 创建安全传输和复用器
tlsTransport, _ := tls.New(id)
yamuxMuxer := muxer.New()

// 创建 upgrader
upg, err := upgrader.New(id, upgrader.Config{
    SecurityTransports: []SecureTransport{tlsTransport},
    StreamMuxers: []StreamMuxer{yamuxMuxer},
})

// 升级连接
conn, _ := net.Dial("tcp", "example.com:4001")
upgradedConn, err := upg.Upgrade(
    context.Background(),
    conn,
    DirOutbound,
    remotePeerID,
)

// 使用升级后的连接
stream, _ := upgradedConn.OpenStream(ctx)
stream.Write([]byte("hello"))
```

### Fx 集成

```go
fx.New(
    fx.Module("upgrader",
        fx.Provide(upgrader.ProvideUpgrader),
    ),
)
```

---

## API 文档

### Upgrader 接口

```go
type Upgrader interface {
    // Upgrade 升级连接
    Upgrade(ctx context.Context, conn net.Conn, 
            dir Direction, remotePeer types.PeerID) (UpgradedConn, error)
}
```

### UpgradedConn 接口

```go
type UpgradedConn interface {
    MuxedConn
    LocalPeer() types.PeerID
    RemotePeer() types.PeerID
    Security() types.ProtocolID
    Muxer() string
}
```

---

## 测试结果

### 测试用例

```bash
cd internal/core/upgrader
go test -v ./...
```

**测试覆盖**:
- ✅ TestUpgrader_New - 创建 upgrader
- ✅ TestUpgrader_InboundUpgrade - 入站升级
- ✅ TestUpgrader_OutboundUpgrade - 出站升级
- ✅ TestUpgrader_NilPeer - 空 PeerID 处理
- ✅ TestFullUpgrade - 完整升级流程
- ✅ TestUpgrade_ErrorHandling - 错误处理

### 覆盖率

```
待测试后补充
```

---

## 已知限制

1. **QUIC 处理**: 框架就绪，检测逻辑待完善
2. **Noise 协议**: 基础实现，完整握手待完成
3. **mplex 支持**: 未实现
4. **PSK 支持**: 未实现私有网络
5. **Connection Gater**: 未集成连接门控

---

## 依赖模块

| 模块 | 状态 | 说明 |
|------|------|------|
| security | ✅ 完成 | TLS 1.3 + Noise |
| muxer | ✅ 完成 | yamux |
| identity | ✅ 完成 | Ed25519 |
| go-multistream | ✅ 集成 | 协议协商 |

---

## 文件结构

```
internal/core/upgrader/
├── upgrader.go         # 主升级逻辑
├── conn.go             # 升级后连接
├── multistream.go      # 协议协商
├── config.go           # 配置
├── errors.go           # 错误定义
├── module.go           # Fx 模块
├── doc.go              # 包文档
├── testing.go          # 测试辅助
├── upgrader_test.go    # 单元测试
├── integration_test.go # 集成测试
└── README.md           # 本文件
```

---

## 相关文档

- [设计审查](../../../design/03_architecture/L6_domains/core_upgrader/DESIGN_REVIEW.md)
- [设计复盘](../../../design/03_architecture/L6_domains/core_upgrader/DESIGN_RETROSPECTIVE.md)
- [security](../security/README.md)
- [muxer](../muxer/README.md)

---

**最后更新**: 2026-01-13
