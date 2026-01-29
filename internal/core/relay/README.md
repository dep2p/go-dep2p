# Relay 模块

**版本**: v2.0.0（统一 Relay 架构）  
**最后更新**: 2026-01-24

## 概述

本模块实现 DeP2P 的中继服务，为 NAT 后的节点提供连接保底能力。

## v2.0 重大变更

⚠️ **v2.0 统一 Relay 架构**

原双层中继架构（System Relay / Realm Relay）已被废弃，统一为单一 Relay 服务。

### 变更原因

- 双层架构增加了理解和维护成本
- 实际部署中业务方通常只需要一个 Relay
- DHT 作为权威目录后，Relay 只需要提供数据转发和打洞协调

### 三大职责

1. **缓存加速**: 维护地址簿，作为 DHT 本地缓存（非权威目录）
2. **打洞协调**: 提供信令通道，协助 NAT 穿透
3. **数据保底**: 直连/打洞失败时转发数据

## 文件结构

```
internal/core/relay/
├── manager.go      # 中继管理器
├── service.go      # 统一 RelayService
├── config.go       # 配置定义
├── defaults.go     # 内置默认值
├── module.go       # Fx 依赖注入
├── selector.go     # 中继选择器
├── transport.go    # 中继传输层
├── discovery.go    # 中继发现
├── errors.go       # 错误定义
├── addressbook/    # 地址簿服务
├── client/         # 中继客户端
├── server/         # 中继服务端
└── geoip/          # 地理位置解析
```

## 快速开始

### 作为客户端使用 Relay

```go
import "github.com/dep2p/go-dep2p"

// 创建节点并配置 Relay
node, err := dep2p.New(ctx,
    dep2p.WithRelayAddr("/ip4/relay.example.com/tcp/4001/p2p/QmRelay..."),
)
```

### 作为 Relay 服务端

```go
import "github.com/dep2p/go-dep2p"

// 创建 Relay 服务器
node, err := dep2p.New(ctx,
    dep2p.EnableRelayServer(true),
    dep2p.WithPublicAddr("/ip4/1.2.3.4/tcp/4001"),
)
```

### 程序化控制

```go
// 启用/禁用 Relay 能力
node.EnableRelay(ctx)
node.DisableRelay(ctx)

// 检查状态
enabled := node.IsRelayEnabled()
stats := node.RelayStats()

// 设置 Relay 地址
node.SetRelayAddr(addr)
node.RemoveRelayAddr()
```

## 配置参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| EnableClient | true | 启用中继客户端 |
| EnableServer | false | 启用中继服务端 |
| MaxReservations | 1024 | 最大预约数 |
| MaxCircuits | 128 | 最大活跃电路 |
| ReservationTTL | 2h | 预约有效期 |
| BufferSize | 4096 | 中继缓冲区大小 |

## 设计文档

详见: `design/_discussions/20260123-nat-relay-concept-clarification.md` §9.0 统一 Relay 架构
