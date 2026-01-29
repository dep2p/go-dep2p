# Protocol Liveness - 存活检测

> **版本**: v1.0.0  
> **状态**: ✅ 已完成  
> **架构层**: Protocol Layer

---

## 概述

`liveness` 实现存活检测服务，通过周期性 Ping 监控节点的在线状态。

**协议标识**: `/dep2p/app/<realmID>/liveness/1.0.0`

**核心功能**:
- 🔍 存活探测 - Ping 节点检测在线状态
- 📊 状态监控 - 跟踪节点存活状态变化
- 👁️ Watch 机制 - 实时订阅状态事件
- 🏠 Realm 集成 - 支持 Realm 绑定模式

---

## 快速开始

```go
import "github.com/dep2p/go-dep2p/internal/protocol/liveness"

// 全局模式
svc, err := liveness.New(host, realmMgr)
if err != nil {
    log.Fatal(err)
}

// 或 Realm 绑定模式
svc, err := liveness.NewForRealm(host, realm)

// 启动服务
if err := svc.Start(ctx); err != nil {
    log.Fatal(err)
}
defer svc.Stop(ctx)

// Ping 节点
latency, err := svc.Ping(ctx, peerID)
if err != nil {
    log.Printf("peer offline: %v", err)
}
fmt.Printf("latency: %v\n", latency)

// 获取状态
status := svc.GetStatus(peerID)
fmt.Printf("peer %s is %s\n", peerID, status.State)

// Watch 节点状态
eventCh, err := svc.Watch(ctx, peerID)
for event := range eventCh {
    fmt.Printf("peer %s state changed: %s\n", event.PeerID, event.State)
}
```

---

## 工作模式

### 全局模式

```go
svc, err := liveness.New(host, realmMgr, opts...)
```

- 可监控任意节点
- 协议 ID 不含 RealmID

### Realm 绑定模式

```go
svc, err := liveness.NewForRealm(host, realm, opts...)
```

- 只监控该 Realm 成员
- 协议 ID: `/dep2p/app/<realmID>/liveness/1.0.0`
- 自动验证成员资格

---

## 配置

```go
svc, err := liveness.New(
    host,
    realmMgr,
    liveness.WithPingInterval(5*time.Second),  // Ping 间隔
    liveness.WithPingTimeout(3*time.Second),   // Ping 超时
    liveness.WithFailureThreshold(3),          // 失败阈值
)
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `PingInterval` | `5s` | Ping 间隔 |
| `PingTimeout` | `3s` | Ping 超时 |
| `FailureThreshold` | `3` | 连续失败阈值 |

---

## 状态模型

| 状态 | 说明 |
|------|------|
| `Unknown` | 初始状态，未探测 |
| `Alive` | 在线，Ping 成功 |
| `Dead` | 离线，连续失败达阈值 |
| `Suspect` | 可疑，部分失败 |

---

## 测试

```bash
go test -v ./internal/protocol/liveness/...
go test -cover ./internal/protocol/liveness/...
go test -bench=. ./internal/protocol/liveness/...
```

---

## 相关文档

- [doc.go](doc.go) - 包文档
- [pkg/interfaces/liveness.go](../../../pkg/interfaces/liveness.go) - 公共接口

---

**最后更新**: 2026-01-20
