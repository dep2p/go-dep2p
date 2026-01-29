# Protocol Messaging - 点对点消息

> **版本**: v1.0.0  
> **状态**: ✅ 已完成  
> **架构层**: Protocol Layer

---

## 概述

`messaging` 实现点对点消息传递协议，提供请求-响应模式的同步/异步消息通信。

**协议标识**: `/dep2p/app/<realmID>/messaging/1.0.0`

**核心功能**:
- 📤 请求-响应 - 同步发送并等待响应
- ⚡ 异步发送 - 通过 channel 接收响应
- 🎯 处理器注册 - 注册协议处理器
- 🔄 自动重试 - 失败自动重试
- 🏠 Realm 集成 - 自动验证成员资格

---

## 快速开始

### 发送消息

```go
import "github.com/dep2p/go-dep2p/internal/protocol/messaging"

// 创建服务
svc, err := messaging.New(host, realmMgr)
if err != nil {
    log.Fatal(err)
}

if err := svc.Start(ctx); err != nil {
    log.Fatal(err)
}
defer svc.Stop(ctx)

// 同步发送
resp, err := svc.Send(ctx, peerID, "myprotocol", []byte("hello"))
if err != nil {
    log.Fatal(err)
}
fmt.Printf("Response: %s\n", resp)

// 异步发送
respCh, err := svc.SendAsync(ctx, peerID, "myprotocol", []byte("hello"))
resp := <-respCh
if resp.Error != nil {
    log.Fatal(resp.Error)
}
```

### 注册处理器

```go
err = svc.RegisterHandler("myprotocol", func(ctx context.Context, req *interfaces.Request) (*interfaces.Response, error) {
    fmt.Printf("Received: %s from %s\n", req.Data, req.From)
    
    return &interfaces.Response{
        ID:   req.ID,
        From: host.ID(),
        Data: []byte("world"),
    }, nil
})
```

---

## 配置

```go
svc, err := messaging.New(
    host,
    realmMgr,
    messaging.WithTimeout(10*time.Second),  // 请求超时
    messaging.WithMaxRetries(5),            // 最大重试次数
    messaging.WithRetryDelay(time.Second),  // 重试延迟
)
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `Timeout` | `10s` | 请求超时 |
| `MaxRetries` | `3` | 最大重试次数 |
| `RetryDelay` | `1s` | 重试延迟 |

---

## 协议格式

**消息格式** (Protobuf):

```protobuf
message Request {
    string id = 1;
    string from = 2;
    string protocol = 3;
    bytes data = 4;
    int64 timestamp = 5;
    map<string, string> metadata = 6;
}

message Response {
    string id = 1;
    string from = 2;
    bytes data = 3;
    string error = 4;
    int64 timestamp = 5;
    int64 latency = 6;
    map<string, string> metadata = 7;
}
```

---

## 错误处理

| 错误 | 说明 |
|------|------|
| `ErrNotStarted` | 服务未启动 |
| `ErrAlreadyStarted` | 服务已启动 |
| `ErrInvalidProtocol` | 无效协议格式 |
| `ErrNotRealmMember` | 非 Realm 成员 |
| `ErrHandlerNotFound` | 处理器未找到 |
| `ErrTimeout` | 请求超时 |
| `ErrStreamClosed` | 流已关闭 |
| `ErrInvalidMessage` | 无效消息格式 |

---

## 性能特性

- **消息延迟**: < 100ms (局域网)
- **吞吐量**: > 1000 msg/s
- **并发安全**: 所有方法并发安全
- **自动重试**: 网络错误自动重试
- **流复用**: 复用 Host 流多路复用

---

## 测试

```bash
go test -v ./internal/protocol/messaging/...
go test -cover ./internal/protocol/messaging/...
go test -bench=. ./internal/protocol/messaging/...
```

---

## 相关文档

- [doc.go](doc.go) - 包文档
- [pkg/interfaces/messaging.go](../../../pkg/interfaces/messaging.go) - 公共接口
- [pkg/proto/messaging/](../../../pkg/proto/messaging/) - Protobuf 定义

---

**最后更新**: 2026-01-20
