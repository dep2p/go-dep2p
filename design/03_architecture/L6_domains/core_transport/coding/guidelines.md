# core_transport 编码指南

> 代码规范与最佳实践

---

## 代码组织

```
internal/core/transport/
├── module.go           # Fx 模块
├── transport.go        # 主接口
├── listener.go         # 监听器
├── connection.go       # 连接
├── stream.go           # 流
├── addr.go             # 地址工具
├── errors.go           # 错误定义
└── *_test.go           # 测试
```

---

## 命名规范

### 类型命名

| 类型 | 命名 | 示例 |
|------|------|------|
| 传输接口 | Transport | `transport.Transport` |
| 连接 | Connection | `Connection` |
| 监听器 | Listener | `Listener` |
| 流 | Stream | `Stream` |

### 方法命名

| 方法类型 | 规则 | 示例 |
|----------|------|------|
| 连接 | Dial | `Dial(ctx, addr, peer)` |
| 监听 | Listen | `Listen(addr)` |
| 接受 | Accept | `Accept()` |
| 打开 | Open | `OpenStream(ctx)` |

---

## 错误处理

### Context 传播

```
func (t *QUICTransport) Dial(ctx context.Context, addr multiaddr.Multiaddr, peer types.NodeID) (Connection, error) {
    // 始终检查 context
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }
    
    // 传递 context 到底层
    quicConn, err := quic.DialAddr(ctx, udpAddr, tlsConf, config)
    // ...
}
```

### 错误包装

```
func (l *Listener) Accept() (Connection, error) {
    conn, err := l.quicListener.Accept(context.Background())
    if err != nil {
        return nil, fmt.Errorf("accept: %w", err)
    }
    return newConnection(conn), nil
}
```

---

## 并发模式

### 监听器并发

```
// 多个 goroutine 可以并发调用 Accept
func (l *Listener) acceptLoop(handler func(Connection)) {
    for {
        conn, err := l.Accept()
        if err != nil {
            if errors.Is(err, ErrListenerClosed) {
                return
            }
            continue
        }
        go handler(conn)
    }
}
```

### 连接并发安全

```
// Connection 的 OpenStream 可并发调用
type Connection struct {
    quicConn quic.Connection  // quic.Connection 本身是并发安全的
}
```

---

## 资源管理

### 连接关闭

```
func (c *Connection) Close() error {
    return c.quicConn.CloseWithError(0, "normal close")
}

// 带错误码关闭
func (c *Connection) CloseWithError(code int, msg string) error {
    return c.quicConn.CloseWithError(quic.ApplicationErrorCode(code), msg)
}
```

### 流关闭

```
func (s *stream) Close() error {
    // 优雅关闭：发送 FIN
    return s.quicStream.Close()
}

func (s *stream) Reset() error {
    // 强制关闭：发送 RST
    s.quicStream.CancelRead(0)
    s.quicStream.CancelWrite(0)
    return nil
}
```

---

## 日志规范

```
slog.Debug("dial started",
    "addr", addr.String(),
    "peer", peer.Pretty(),
)

slog.Info("connection established",
    "remote", conn.RemotePeer().Pretty(),
    "addr", conn.RemoteAddr().String(),
)
```

---

**最后更新**：2026-01-11
