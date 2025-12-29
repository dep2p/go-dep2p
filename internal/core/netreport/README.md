# 网络诊断模块 (NetReport)

## 概述

网络诊断模块提供全面的网络环境检测能力，帮助 P2P 节点了解自身的网络状况并选择最佳的连接策略。

## 架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                         NetReport 模块                               │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐        │
│  │    Client    │────►│    Prober    │────►│ReportBuilder │        │
│  └──────────────┘     └──────────────┘     └──────────────┘        │
│        │                    │                     │                 │
│        │                    ▼                     │                 │
│        │         ┌──────────────────┐             │                 │
│        │         │     探测器池      │             │                 │
│        │         ├──────────────────┤             │                 │
│        │         │ • IPv4Prober    │             │                 │
│        │         │ • IPv6Prober    │             │                 │
│        │         │ • NATProber     │             │                 │
│        │         │ • RelayProber   │             │                 │
│        │         └──────────────────┘             │                 │
│        │                    │                     │                 │
│        │                    ▼                     ▼                 │
│        │              ┌──────────────────────────────┐              │
│        └─────────────►│          Report              │              │
│                       └──────────────────────────────┘              │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

## 文件结构

```
internal/core/netreport/
├── module.go           # fx 模块定义
├── client.go           # 诊断客户端
├── prober.go           # 探测器协调器
├── report.go           # 报告构建器
├── probes/
│   ├── ipv4.go         # IPv4 连通性探测
│   ├── ipv6.go         # IPv6 连通性探测
│   ├── nat.go          # NAT 类型探测
│   └── relay.go        # 中继延迟探测
├── README.md           # 本文档
└── netreport_test.go   # 单元测试
```

## 核心功能

### 1. IPv4/IPv6 连通性检测

- 使用 STUN 协议检测公网地址
- 支持多服务器冗余
- 自动检测 IPv6 可用性

### 2. NAT 类型检测

| NAT 类型 | 说明 |
|----------|------|
| Full Cone | 映射不随目标变化，任何外部主机可访问 |
| Restricted Cone | 映射不变，但限制源 IP |
| Port Restricted | 映射不变，限制源 IP 和端口 |
| Symmetric | 映射随目标变化（最难穿透） |

### 3. 中继延迟测量

- 并发测量多个中继服务器
- 自动选择最低延迟中继
- 支持 HTTP/HTTPS/QUIC 中继

### 4. 端口映射协议检测

| 能力 | 状态 | 说明 |
|------|------|------|
| UPnP 检测 | ✅ 已实现 | 发送 SSDP M-SEARCH 检测 IGD |
| NAT-PMP 检测 | ✅ 已实现 | 向网关发送 NAT-PMP 请求 |
| PCP 检测 | ✅ 已实现 | RFC 6887 协议，支持 CGN/IPv6 |
| 网关发现 | ✅ 已实现 | 使用 jackpal/gateway 获取真实网关 |

### 5. 其他检测

| 能力 | 状态 | 说明 |
|------|------|------|
| 强制门户检测 | ✅ 已实现 | 使用 Apple/Microsoft/Google 端点 |

## 使用示例

### 基本使用

```go
import (
    "context"
    "github.com/dep2p/go-dep2p/internal/core/netreport"
    netreportif "github.com/dep2p/go-dep2p/pkg/interfaces/netreport"
)

// 使用默认配置创建客户端
config := netreportif.DefaultConfig()
client := netreport.NewClient(config, logger)

// 生成诊断报告
ctx := context.Background()
report, err := client.GetReport(ctx)
if err != nil {
    log.Fatal(err)
}

// 查看结果
fmt.Printf("IPv4 UDP: %v\n", report.UDPv4)
fmt.Printf("IPv6 UDP: %v\n", report.UDPv6)
fmt.Printf("NAT 类型: %s\n", report.NATType)
fmt.Printf("首选中继: %s\n", report.PreferredRelay)
```

### 自定义配置

```go
config := netreportif.Config{
    STUNServers: []string{
        "stun.example.com:3478",
        "stun2.example.com:3478",
    },
    RelayServers: []string{
        "https://relay.example.com",
    },
    Timeout:      30 * time.Second,
    ProbeTimeout: 5 * time.Second,
    EnableIPv6:   true,
}

client := netreport.NewClient(config, logger)
```

### 异步获取报告

```go
ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

reportChan := client.GetReportAsync(ctx)

select {
case report := <-reportChan:
    if report != nil {
        // 处理报告
    }
case <-ctx.Done():
    // 超时
}
```

## 配置参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `STUNServers` | Google STUN | STUN 服务器列表 |
| `RelayServers` | 空 | 中继服务器列表 |
| `Timeout` | 30s | 整体诊断超时 |
| `ProbeTimeout` | 5s | 单个探测超时 |
| `EnableIPv4` | true | 启用 IPv4 探测 |
| `EnableIPv6` | true | 启用 IPv6 探测 |
| `MaxConcurrentProbes` | 10 | 最大并发探测数 |
| `FullReportInterval` | 5m | 完整报告间隔 |

## 诊断报告字段

```go
type Report struct {
    // 连通性
    UDPv4         bool      // IPv4 UDP 可用
    UDPv6         bool      // IPv6 UDP 可用
    GlobalV4      net.IP    // 公网 IPv4
    GlobalV6      net.IP    // 公网 IPv6

    // NAT
    NATType               types.NATType
    MappingVariesByDestIPv4 *bool
    MappingVariesByDestIPv6 *bool

    // 中继
    RelayLatencies map[string]time.Duration
    PreferredRelay string

    // 端口映射
    UPnPAvailable   bool
    NATPMPAvailable bool

    // 其他
    CaptivePortal *bool
    Timestamp     time.Time
    Duration      time.Duration
}
```

## 相关文档

- [NAT 模块](../nat/README.md)
- [中继模块](../relay/README.md)
- [设计文档](../../../docs/01-design/protocols/transport/02-security.md)

