# core_muxer 编码指南

> 代码规范与最佳实践

---

## 代码组织

```
internal/core/muxer/
├── module.go           # Fx 模块
├── muxer.go            # 多路复用器
├── conn.go             # 连接
├── stream.go           # 流
├── errors.go           # 错误
└── *_test.go           # 测试
```

---

## 流使用规范

### 正确关闭

```
// 优雅关闭
func handleStream(s Stream) error {
    defer s.Close()  // 始终关闭
    
    // 业务逻辑
    return nil
}

// 错误时重置
func handleStream(s Stream) error {
    if err := process(s); err != nil {
        s.Reset()  // 强制关闭
        return err
    }
    return s.Close()  // 优雅关闭
}
```

### 超时设置

```
func readWithTimeout(s Stream, timeout time.Duration) ([]byte, error) {
    s.SetReadDeadline(time.Now().Add(timeout))
    defer s.SetReadDeadline(time.Time{})  // 清除
    
    return io.ReadAll(s)
}
```

---

## 并发模式

### 并发接受流

```
func (c *muxedConn) acceptLoop(handler func(Stream)) {
    for {
        stream, err := c.AcceptStream()
        if err != nil {
            if errors.Is(err, ErrConnClosed) {
                return
            }
            continue
        }
        go handler(stream)
    }
}
```

### 流池（可选）

```
// 对于高频场景，可以考虑流池
type StreamPool struct {
    streams chan Stream
    conn    MuxedConn
}
```

---

## 资源管理

### 流限制

```
// 限制并发流数量
type RateLimitedConn struct {
    MuxedConn
    sem chan struct{}
}

func (c *RateLimitedConn) OpenStream(ctx context.Context) (Stream, error) {
    select {
    case c.sem <- struct{}{}:
        // 获取许可
    case <-ctx.Done():
        return nil, ctx.Err()
    }
    
    stream, err := c.MuxedConn.OpenStream(ctx)
    if err != nil {
        <-c.sem  // 释放许可
        return nil, err
    }
    
    return &limitedStream{Stream: stream, sem: c.sem}, nil
}
```

---

**最后更新**：2026-01-11
