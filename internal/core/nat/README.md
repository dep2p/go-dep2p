# Core NAT - NAT 穿透

> **版本**: v1.0.0  
> **状态**: ✅ 已完成  
> **架构层**: Core Layer

---

## 概述

`nat` 模块实现 NAT 穿透功能，帮助处于 NAT 后的节点建立直接连接。

**核心功能**:
- 🔍 AutoNAT - NAT 类型检测和可达性判断
- 🌐 STUN - 外部地址获取
- 🔌 UPnP/NAT-PMP - 自动端口映射
- 🕳️ Hole Punching - UDP 打洞

---

## 快速开始

```go
import "github.com/dep2p/go-dep2p/internal/core/nat"

// 创建 NAT 服务
config := nat.DefaultConfig()
service, err := nat.NewService(config, swarm, eventbus)
if err != nil {
    log.Fatal(err)
}

// 启动服务
ctx := context.Background()
if err := service.Start(ctx); err != nil {
    log.Fatal(err)
}
defer service.Stop()

// 查询可达性
reachability := service.Reachability()
fmt.Println("Reachability:", reachability)

// 获取外部地址
addrs := service.ExternalAddrs()
```

---

## 子模块

| 子目录 | 功能 | 说明 |
|--------|------|------|
| `stun/` | STUN 客户端 | 获取外部 IP 和端口 |
| `upnp/` | UPnP 映射 | IGD 端口映射 |
| `natpmp/` | NAT-PMP 映射 | Apple 路由器端口映射 |
| `holepunch/` | 打洞协议 | UDP/TCP 打洞 |
| `netreport/` | 网络诊断 | NAT 类型检测报告 |

---

## 可达性状态

```
┌─────────────────────────────────────────┐
│  ReachabilityUnknown (初始状态)          │
└─────────────────┬───────────────────────┘
                  │ 探测
         ┌────────┴────────┐
         ▼                 ▼
┌─────────────────┐ ┌─────────────────┐
│ ReachabilityPublic │ │ReachabilityPrivate│
│  (公网可达)        │ │  (NAT 后)         │
└─────────────────┘ └─────────────────┘
```

---

## 配置

```go
config := &nat.Config{
    EnableAutoNAT:       true,               // 启用 AutoNAT 检测
    EnableUPnP:          true,               // 启用 UPnP 映射
    EnableNATPMP:        true,               // 启用 NAT-PMP 映射
    EnableHolePunch:     true,               // 启用打洞
    STUNServers:         []string{...},      // STUN 服务器列表
    ProbeInterval:       15 * time.Second,   // 探测间隔
    ConfidenceThreshold: 3,                  // 置信度阈值
}
```

---

## 测试

```bash
# 单元测试
go test -v ./internal/core/nat/...

# 集成测试
go test -v -tags=integration ./internal/core/nat/...
```

---

## 相关文档

- [doc.go](doc.go) - 包文档
- [DESIGN_REVIEW.md](DESIGN_REVIEW.md) - 设计评审

---

**最后更新**: 2026-01-20
