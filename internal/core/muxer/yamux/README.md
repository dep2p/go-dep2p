# yamux 多路复用实现

## 概述

基于 `hashicorp/yamux` 库的多路复用实现，用于 TCP 传输降级场景。

## 文件结构

```
yamux/
├── README.md        # 本文件
├── muxer.go         # Muxer 实现
├── stream.go        # Stream 实现
├── heartbeat.go     # 心跳监控实现
└── heartbeat_test.go # 心跳监控测试
```

## 能力清单

| 能力 | 状态 | 说明 |
|------|------|------|
| 多路复用 | ✅ 已实现 | 基于 yamux 库 |
| 流管理 | ✅ 已实现 | 创建/接受/关闭流 |
| KeepAlive | ✅ 已实现 | yamux 内置 KeepAlive |
| 心跳监控 | ✅ 已实现 | 独立心跳监控器 |
| 超时检测 | ✅ 已实现 | 连接超时自动处理 |
| 延迟统计 | ✅ 已实现 | 心跳往返延迟统计 |

## 核心实现

### muxer.go

```go
// Muxer yamux 多路复用器
type Muxer struct {
    session  *yamux.Session
    isServer bool
    closed   atomic.Bool
}

// NewMuxer 创建 yamux 多路复用器
func NewMuxer(conn io.ReadWriteCloser, isServer bool, config *yamux.Config) (*Muxer, error)

// NewStream 创建新流
func (m *Muxer) NewStream(ctx context.Context) (muxerif.Stream, error)

// AcceptStream 接受新流
func (m *Muxer) AcceptStream() (muxerif.Stream, error)
```

### stream.go

```go
// Stream yamux 流包装
type Stream struct {
    *yamux.Stream
}

// ID 返回流 ID
func (s *Stream) ID() uint32

// CloseRead 关闭读端
func (s *Stream) CloseRead() error

// CloseWrite 关闭写端
func (s *Stream) CloseWrite() error
```

### heartbeat.go

```go
// HeartbeatMonitor 心跳监控器
type HeartbeatMonitor struct {
    config     HeartbeatConfig
    conns      map[string]*connState
    onTimeout  func(connID string)
    onMissed   func(connID string, missed int)
}

// HeartbeatConfig 心跳配置
type HeartbeatConfig struct {
    Interval      time.Duration  // 心跳间隔 (默认 30s)
    Timeout       time.Duration  // 超时时间 (默认 90s)
    MaxMissed     int            // 最大丢失次数 (默认 3)
}

// AddConnection 添加连接监控
func (m *HeartbeatMonitor) AddConnection(connID string) error

// RemoveConnection 移除连接监控
func (m *HeartbeatMonitor) RemoveConnection(connID string) error

// RecordHeartbeat 记录收到心跳
func (m *HeartbeatMonitor) RecordHeartbeat(connID string) error

// GetStats 获取连接心跳统计
func (m *HeartbeatMonitor) GetStats(connID string) (*HeartbeatStats, error)
```

## yamux 配置

```go
var defaultConfig = &yamux.Config{
    AcceptBacklog:          256,
    EnableKeepAlive:        true,
    KeepAliveInterval:      30 * time.Second,
    ConnectionWriteTimeout: 10 * time.Second,
    MaxStreamWindowSize:    256 * 1024,
    StreamOpenTimeout:      75 * time.Second,
    StreamCloseTimeout:     time.Minute,
}
```

## 心跳监控配置

```go
var defaultHeartbeatConfig = HeartbeatConfig{
    Interval:  30 * time.Second,   // 心跳间隔
    Timeout:   90 * time.Second,   // 超时时间 (3 * Interval)
    MaxMissed: 3,                  // 最大丢失次数
}
```

## 使用示例

### 基本多路复用

```go
// 创建 Muxer
muxer, err := yamux.NewMuxer(conn, isServer, nil)
if err != nil {
    return err
}

// 创建流
stream, err := muxer.NewStream(ctx)
if err != nil {
    return err
}
defer stream.Close()

// 读写数据
_, err = stream.Write([]byte("hello"))
```

### 心跳监控

```go
// 创建心跳监控器
monitor := yamux.NewHeartbeatMonitor(config, logger)

// 设置回调
monitor.OnTimeout(func(connID string) {
    log.Printf("Connection %s timed out", connID)
    // 关闭连接
})

monitor.OnMissed(func(connID string, missed int) {
    log.Printf("Connection %s missed %d heartbeats", connID, missed)
})

// 启动监控
monitor.Start(ctx)

// 添加连接
monitor.AddConnection("conn-1")

// 记录收到心跳
monitor.RecordHeartbeat("conn-1")

// 获取统计
stats, _ := monitor.GetStats("conn-1")
fmt.Printf("Latency: %v, Missed: %d\n", stats.AvgLatency, stats.MissedCount)
```

## 使用场景

1. **TCP 传输降级**：当 QUIC 不可用时
2. **测试环境**：简化测试配置
3. **兼容性**：与不支持 QUIC 的网络环境兼容
4. **心跳监控**：检测连接活性，及时发现断连

## 依赖

- `github.com/hashicorp/yamux` - yamux 多路复用库
