# DNS 发现模块

## 概述

DNS 发现模块通过解析 DNS TXT 记录发现节点，遵循 libp2p dnsaddr 规范。这是一种简单可靠的引导发现机制，适合作为 Bootstrap 节点发现的补充。

## 架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                         DNS 发现流程                                 │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐        │
│  │  Discoverer  │────►│   Resolver   │────►│  DNS Server  │        │
│  └──────────────┘     └──────────────┘     └──────────────┘        │
│        │                    │                     │                 │
│        │                    │                     │                 │
│        ▼                    ▼                     ▼                 │
│  ┌──────────────┐     ┌──────────────┐     ┌──────────────┐        │
│  │    Cache     │     │  TXT Parser  │     │ _dnsaddr.xxx │        │
│  └──────────────┘     └──────────────┘     └──────────────┘        │
│                                                                      │
│  数据流：                                                            │
│  1. Discoverer 发起发现请求                                          │
│  2. Resolver 查询 DNS TXT 记录                                       │
│  3. 解析 dnsaddr= 格式获取节点信息                                    │
│  4. 支持嵌套 dnsaddr 递归解析                                         │
│  5. 结果缓存避免重复查询                                              │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

## 文件结构

```
internal/core/discovery/dns/
├── README.md           # 本文档
├── resolver.go         # DNS TXT 记录解析器
├── dns.go              # DNS 发现器实现
└── dns_test.go         # 单元测试
```

## DNS TXT 记录格式

遵循 libp2p dnsaddr 规范：

```
# DNS 域名格式
_dnsaddr.<domain>

# TXT 记录格式
dnsaddr=/ip4/<ip>/tcp/<port>/p2p/<nodeID>
dnsaddr=/ip6/<ip>/tcp/<port>/p2p/<nodeID>

# 嵌套 dnsaddr（指向另一个域名）
dnsaddr=/dnsaddr/<nested-domain>
```

### 示例

```
_dnsaddr.bootstrap.dep2p.network TXT "dnsaddr=/ip4/1.2.3.4/tcp/4001/p2p/5Q2STWvB..."
                                 TXT "dnsaddr=/ip6/::1/tcp/4001/p2p/5Q2STWvB..."
                                 TXT "dnsaddr=/dnsaddr/us-east.dep2p.network"
```

## 核心组件

### Resolver

DNS TXT 记录解析器，负责：

1. 查询 DNS TXT 记录
2. 解析 dnsaddr 格式
3. 递归解析嵌套域名
4. 结果缓存

```go
// 创建解析器
config := dns.DefaultResolverConfig()
config.CustomResolver = "8.8.8.8:53"  // 可选：自定义 DNS 服务器
resolver := dns.NewResolver(config, logger)

// 解析域名
peers, err := resolver.Resolve(ctx, "_dnsaddr.bootstrap.dep2p.network")

// 带深度限制的递归解析
peers, err = resolver.ResolveWithDepth(ctx, "_dnsaddr.bootstrap.dep2p.network", 3)
```

### Discoverer

DNS 发现器，实现 `NamespaceDiscoverer` 接口：

1. 管理多个域名
2. 后台自动刷新
3. 实现发现接口

```go
// 创建发现器
config := dns.DefaultDiscovererConfig()
config.Domains = []string{
    "_dnsaddr.bootstrap.dep2p.network",
    "_dnsaddr.mainnet.dep2p.io",
}
discoverer := dns.NewDiscoverer(config, logger)

// 启动发现器
err := discoverer.Start(ctx)

// 发现节点
peerCh, err := discoverer.DiscoverPeers(ctx, "dns")
for peer := range peerCh {
    fmt.Printf("发现节点: %s\n", peer.ID)
}
```

## 配置参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `Timeout` | 10s | DNS 查询超时 |
| `MaxDepth` | 3 | 最大递归深度 |
| `CacheTTL` | 5m | 缓存有效期 |
| `CustomResolver` | "" | 自定义 DNS 服务器 |
| `RefreshInterval` | 5m | 后台刷新间隔 |

## 使用示例

### 基本使用

```go
import (
    "context"
    "github.com/dep2p/go-dep2p/internal/core/discovery/dns"
    "go.uber.org/zap"
)

func main() {
    logger, _ := zap.NewDevelopment()
    
    config := dns.DefaultDiscovererConfig()
    config.Domains = []string{
        "bootstrap.dep2p.network",  // 自动添加 _dnsaddr. 前缀
    }
    
    discoverer := dns.NewDiscoverer(config, logger)
    
    ctx := context.Background()
    if err := discoverer.Start(ctx); err != nil {
        panic(err)
    }
    defer discoverer.Stop()
    
    // 获取所有已发现的节点
    peers := discoverer.AllPeers()
    for _, peer := range peers {
        fmt.Printf("节点: %s, 地址: %v\n", peer.ID, peer.Addrs)
    }
}
```

### 与 DiscoveryService 集成

```go
// 在 discovery module 中注册
if config.EnableDNS && len(config.DNSDomains) > 0 {
    dnsConfig := dns.DiscovererConfig{
        Domains:    config.DNSDomains,
        Timeout:    config.DNSTimeout,
        MaxDepth:   config.DNSMaxDepth,
        CacheTTL:   config.DNSCacheTTL,
    }
    dnsDiscoverer := dns.NewDiscoverer(dnsConfig, logger)
    service.RegisterNamespaceDiscoverer("dns", dnsDiscoverer)
}
```

## 与其他发现机制的关系

DNS 发现作为 Bootstrap 的补充，优先级低于直接配置的引导节点：

```
发现优先级:
1. 直接配置的 BootstrapPeers (最高)
2. DNS 发现的节点
3. DHT 发现
4. mDNS 发现 (仅局域网)
5. Rendezvous 发现
```

## 安全考虑

1. **DNS 欺骗**：建议使用 DNSSEC 或可信 DNS 服务器
2. **缓存投毒**：缓存有过期时间限制
3. **递归深度**：限制最大递归深度防止无限循环

## 相关文档

- [发现协议设计](../../../docs/01-design/protocols/network/01-discovery.md)
- [Rendezvous 发现](../rendezvous/README.md)
- [Bootstrap 发现](../bootstrap/bootstrap.go)

