# core_muxer 内部设计

> 内部结构与实现细节

---

## 内部结构

```
internal/core/muxer/
├── module.go           # Fx 模块定义
├── muxer.go            # 多路复用器实现
├── conn.go             # 多路复用连接
├── stream.go           # 流封装
└── muxer_test.go       # 测试
```

---

## QUIC 流实现

### 流创建

```
func (c *muxedConn) OpenStream(ctx context.Context) (Stream, error) {
    quicStream, err := c.quicConn.OpenStreamSync(ctx)
    if err != nil {
        return nil, fmt.Errorf("open stream: %w", err)
    }
    
    return &stream{
        Stream: quicStream,
    }, nil
}
```

### 流接受

```
func (c *muxedConn) AcceptStream() (Stream, error) {
    quicStream, err := c.quicConn.AcceptStream(context.Background())
    if err != nil {
        return nil, fmt.Errorf("accept stream: %w", err)
    }
    
    return &stream{
        Stream: quicStream,
    }, nil
}
```

---

## 流控制

### QUIC 流量控制

```
连接级流控：
• 初始窗口：64KB
• 最大窗口：16MB

流级流控：
• 初始窗口：64KB
• 最大窗口：16MB

窗口更新：
• 消费 50% 后发送 WINDOW_UPDATE
```

### 配置

```
var defaultConfig = &quic.Config{
    MaxIncomingStreams:    1024,
    MaxIncomingUniStreams: 1024,
    InitialStreamReceiveWindow:     64 * 1024,
    MaxStreamReceiveWindow:         16 * 1024 * 1024,
    InitialConnectionReceiveWindow: 64 * 1024,
    MaxConnectionReceiveWindow:     64 * 1024 * 1024,
}
```

---

## 流状态

### Stream 包装

```
type stream struct {
    quic.Stream
    closed atomic.Bool
}

func (s *stream) Close() error {
    if s.closed.Swap(true) {
        return nil // 已关闭
    }
    return s.Stream.Close()
}

func (s *stream) Reset() error {
    s.closed.Store(true)
    s.Stream.CancelRead(0)
    s.Stream.CancelWrite(0)
    return nil
}
```

---

## 错误处理

```
var (
    ErrStreamClosed = errors.New("muxer: stream closed")
    ErrStreamReset  = errors.New("muxer: stream reset")
    ErrConnClosed   = errors.New("muxer: connection closed")
)
```

---

**最后更新**：2026-01-11
