# Protocol Streams - 流协议

> **版本**: v1.0.0  
> **状态**: ✅ 已完成  
> **架构层**: Protocol Layer

---

## 概述

`streams` 实现双向流协议，提供长连接的双向数据流通信能力。

**协议标识**: `/dep2p/app/<realmID>/streams/1.0.0`

**核心功能**:
- 🔄 双向流 - 建立持久双向数据流
- 🎯 处理器注册 - 注册流处理器
- 🏠 Realm 集成 - 支持 Realm 绑定模式
- ⚡ 高性能 - 低延迟流式传输

---

## 快速开始

### 创建流

```go
import "github.com/dep2p/go-dep2p/internal/protocol/streams"

// 全局模式
svc, err := streams.New(host, realmMgr)
if err != nil {
    log.Fatal(err)
}

// 或 Realm 绑定模式
svc, err := streams.NewForRealm(host, realm)

// 启动服务
if err := svc.Start(ctx); err != nil {
    log.Fatal(err)
}
defer svc.Stop(ctx)

// 打开双向流
stream, err := svc.OpenBiStream(ctx, peerID, "myprotocol")
if err != nil {
    log.Fatal(err)
}
defer stream.Close()

// 发送数据
_, err = stream.Write([]byte("hello"))

// 接收数据
buf := make([]byte, 1024)
n, err := stream.Read(buf)
fmt.Printf("Received: %s\n", buf[:n])
```

### 注册处理器

```go
// 注册双向流处理器
err = svc.RegisterBiStreamHandler("myprotocol", func(stream interfaces.BiStream) {
    defer stream.Close()
    
    buf := make([]byte, 1024)
    for {
        n, err := stream.Read(buf)
        if err != nil {
            return
        }
        
        // Echo back
        stream.Write(buf[:n])
    }
})
```

---

## 工作模式

### 全局模式

```go
svc, err := streams.New(host, realmMgr, opts...)
```

- 可与任意节点建立流
- 协议 ID 不含 RealmID

### Realm 绑定模式

```go
svc, err := streams.NewForRealm(host, realm, opts...)
```

- 只与该 Realm 成员建立流
- 协议 ID: `/dep2p/app/<realmID>/streams/1.0.0`
- 自动验证成员资格

---

## 配置

```go
svc, err := streams.New(
    host,
    realmMgr,
    streams.WithReadTimeout(30*time.Second),   // 读超时
    streams.WithWriteTimeout(30*time.Second),  // 写超时
    streams.WithBufferSize(4096),              // 缓冲区大小
)
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `ReadTimeout` | `30s` | 读超时 |
| `WriteTimeout` | `30s` | 写超时 |
| `BufferSize` | `4KB` | 缓冲区大小 |

---

## 流接口

```go
type BiStream interface {
    // 读写操作
    Read(p []byte) (n int, err error)
    Write(p []byte) (n int, err error)
    
    // 关闭操作
    Close() error
    CloseRead() error
    CloseWrite() error
    
    // 元信息
    Protocol() string
    RemotePeer() types.PeerID
    
    // 超时控制
    SetDeadline(t time.Time) error
    SetReadDeadline(t time.Time) error
    SetWriteDeadline(t time.Time) error
}
```

---

## 错误处理

| 错误 | 说明 |
|------|------|
| `ErrNotStarted` | 服务未启动 |
| `ErrAlreadyStarted` | 服务已启动 |
| `ErrNilHost` | Host 为 nil |
| `ErrStreamClosed` | 流已关闭 |
| `ErrHandlerExists` | 处理器已存在 |
| `ErrHandlerNotFound` | 处理器未找到 |

---

## 使用场景

- 文件传输 - 大文件流式传输
- 实时通信 - 视频/音频流
- 游戏同步 - 低延迟状态同步
- 数据管道 - 持续数据流处理

---

## 测试

```bash
go test -v ./internal/protocol/streams/...
go test -cover ./internal/protocol/streams/...
go test -bench=. ./internal/protocol/streams/...
```

---

## 相关文档

- [doc.go](doc.go) - 包文档
- [pkg/interfaces/streams.go](../../../pkg/interfaces/streams.go) - 公共接口

---

**最后更新**: 2026-01-20
