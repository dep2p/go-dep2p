# Protocol Streams 实现细节

> **组件**: P6-03 protocol_streams

---

## 核心实现

### 1. streamWrapper

**职责**: 将 Host 层流适配为应用层流

```go
type streamWrapper struct {
    stream   interfaces.Stream  // Host层流
    protocol string             // 协议ID
    opened   int64              // 打开时间
    mu       sync.RWMutex       // 并发保护
    closed   bool               // 关闭状态
}
```

**关键方法**:
- `Read/Write`: 委托给底层流
- `Protocol`: 返回协议ID
- `RemotePeer`: 从连接获取远端ID
- `Stat`: 返回流统计信息

---

### 2. Service

**职责**: 管理流服务和处理器

```go
type Service struct {
    host     interfaces.Host
    realmMgr interfaces.RealmManager
    handlers map[string]interfaces.BiStreamHandler
    mu       sync.RWMutex
    started  bool
    ctx      context.Context
    cancel   context.CancelFunc
    config   *Config
}
```

**关键方法**:
- `Open`: 打开到指定节点的流
- `RegisterHandler`: 注册协议处理器
- `UnregisterHandler`: 注销协议处理器

---

### 3. 协议ID管理

**构建协议ID**:
```go
func buildProtocolID(realmID, protocol string) string {
    return fmt.Sprintf("/dep2p/app/%s/streams/%s/1.0.0",
        realmID, protocol)
}
```

**解析协议ID**:
```go
func parseProtocolID(fullProtocol string) (realmID, protocol, version string, error)
```

---

### 4. 并发安全

- 使用 `sync.RWMutex` 保护共享状态
- 读操作使用 `RLock`
- 写操作使用 `Lock`
- 流独立的并发保护

---

### 5. 生命周期

**启动**:
- 初始化context
- 设置started标志

**停止**:
- 取消context
- 清理started标志

**关闭**:
- 移除所有处理器
- 清理资源

---

**最后更新**: 2026-01-14
