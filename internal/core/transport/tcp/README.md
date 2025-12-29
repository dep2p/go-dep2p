# TCP 传输模块

## 概述

TCP 传输模块提供基于 TCP 协议的传输层实现，作为 QUIC 的备选方案。

## 使用场景

- **UDP 被阻止**：当网络环境阻止 UDP 流量时，可以使用 TCP 作为备选
- **兼容性**：与只支持 TCP 的节点通信
- **调试**：TCP 流量更容易被网络工具捕获和分析

## 与 QUIC 的对比

| 特性 | QUIC | TCP |
|------|------|-----|
| 连接延迟 | 0-1 RTT | 1-3 RTT |
| 多路复用 | 原生支持 | 需要 Muxer |
| 内置加密 | TLS 1.3 | 需要单独配置 |
| NAT 穿透 | 较易 | 困难 |
| 头部阻塞 | 流级别 | 连接级别 |

## 地址格式

支持以下多地址格式：

```
/ip4/127.0.0.1/tcp/4001
/ip6/::1/tcp/4001
/dns4/example.com/tcp/4001
/dns6/example.com/tcp/4001
```

## 使用示例

### 创建传输

```go
import (
    "github.com/dep2p/go-dep2p/internal/core/transport/tcp"
    transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
)

config := transportif.DefaultConfig()
transport := tcp.NewTransport(config)
defer transport.Close()
```

### 监听连接

```go
addr := tcp.MustParseAddress("/ip4/0.0.0.0/tcp/4001")
listener, err := transport.Listen(addr)
if err != nil {
    log.Fatal(err)
}
defer listener.Close()

for {
    conn, err := listener.Accept()
    if err != nil {
        log.Println(err)
        continue
    }
    go handleConn(conn)
}
```

### 建立连接

```go
addr := tcp.MustParseAddress("/ip4/192.168.1.100/tcp/4001")
conn, err := transport.Dial(context.Background(), addr)
if err != nil {
    log.Fatal(err)
}
defer conn.Close()

// 读写数据
conn.Write([]byte("Hello"))
buf := make([]byte, 1024)
n, _ := conn.Read(buf)
```

## 配置选项

### DialOptions

| 选项 | 默认值 | 说明 |
|------|--------|------|
| Timeout | 30s | 连接超时 |
| KeepAlive | 15s | 保活间隔 |
| NoDelay | true | 禁用 Nagle 算法 |

### ListenOptions

| 选项 | 默认值 | 说明 |
|------|--------|------|
| Backlog | 128 | 连接队列大小 |
| ReuseAddr | true | 允许地址重用 |
| ReusePort | false | 允许端口重用 |

## 注意事项

1. **安全性**：TCP 传输不提供内置加密，需要配合 TLS 或 Noise 安全层使用
2. **多路复用**：TCP 不支持原生多路复用，需要配合 Muxer（如 Yamux）使用
3. **性能**：相比 QUIC，TCP 有更高的连接建立延迟和头部阻塞问题

## 代码结构

```
tcp/
├── address.go      # 地址解析和表示
├── conn.go         # 连接实现
├── listener.go     # 监听器实现
├── transport.go    # Transport 接口实现
└── README.md       # 本文档
```

