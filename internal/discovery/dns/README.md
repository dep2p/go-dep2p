# Discovery DNS - DNS 节点发现

> **版本**: v1.0.0  
> **状态**: ✅ 已完成  
> **架构层**: Discovery Layer

---

## 概述

`dns` 通过 DNS TXT 记录发现节点，支持 dnsaddr 格式解析、递归嵌套解析、结果缓存和后台刷新。

**核心功能**:
- 🌐 dnsaddr 解析 - 解析 `/dnsaddr/example.com` 格式
- 🔄 递归解析 - 支持嵌套 dnsaddr 引用
- 💾 结果缓存 - 缓存 DNS 查询结果
- ⏰ 后台刷新 - 定期刷新配置的域名

---

## 快速开始

```go
import "github.com/dep2p/go-dep2p/internal/discovery/dns"

config := dns.DefaultConfig()
config.Domains = []string{"bootstrap.dep2p.io"}

discoverer := dns.NewDiscoverer(config)
if err := discoverer.Start(ctx); err != nil {
    log.Fatal(err)
}
defer discoverer.Stop(ctx)

// 发现节点
peerCh, err := discoverer.FindPeers(ctx, "dns")
for peer := range peerCh {
    log.Info("found peer:", peer.ID)
}

// 直接解析域名
peers, err := discoverer.Resolve(ctx, "bootstrap.dep2p.io")
```

---

## DNS 记录格式

### dnsaddr TXT 记录

```
_dnsaddr.bootstrap.dep2p.io.  300  IN  TXT  "dnsaddr=/ip4/1.2.3.4/tcp/4001/p2p/QmYwAPJzv..."
_dnsaddr.bootstrap.dep2p.io.  300  IN  TXT  "dnsaddr=/ip6/::1/tcp/4001/p2p/QmYwAPJzv..."
_dnsaddr.bootstrap.dep2p.io.  300  IN  TXT  "dnsaddr=/dnsaddr/us-east.dep2p.io"
```

### 直接地址

```
dnsaddr=/ip4/1.2.3.4/tcp/4001/p2p/QmYwAPJzv...
dnsaddr=/ip6/2001:db8::/tcp/4001/p2p/QmYwAPJzv...
```

### 嵌套引用

```
dnsaddr=/dnsaddr/us-east.bootstrap.dep2p.io
```

---

## 架构

```
┌─────────────────────────────────────────┐
│         Discovery 接口层                 │
│  FindPeers, Advertise, Start, Stop      │
├─────────────────────────────────────────┤
│        Discoverer 发现器                 │
│  域名管理, 后台刷新                      │
├─────────────────────────────────────────┤
│        Resolver 解析器                   │
│  DNS 查询, 缓存, 递归解析               │
└─────────────────────────────────────────┘
              ↓
┌─────────────────────────────────────────┐
│       net.Resolver (标准库)              │
└─────────────────────────────────────────┘
```

---

## 配置

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `Domains` | `[]` | 要查询的域名列表 |
| `Timeout` | `10s` | DNS 查询超时 |
| `MaxDepth` | `3` | 最大递归深度 |
| `CacheTTL` | `5min` | 缓存 TTL |
| `RefreshInterval` | `5min` | 刷新间隔 |

---

## 使用场景

- 引导节点配置（使用 dnsaddr 而非硬编码 IP）
- 动态节点发现
- 多地域负载均衡

---

## 测试

```bash
go test -v ./internal/discovery/dns/...
go test -cover ./internal/discovery/dns/...
```

---

**最后更新**: 2026-01-20
