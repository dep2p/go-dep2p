# QUIC Transport 实现

## 概述

基于 `quic-go` 库的 QUIC 传输实现。

## 文件结构

```
quic/
├── README.md        # 本文件
├── transport.go     # Transport 实现
├── listener.go      # Listener 实现
└── conn.go          # Connection 包装
```

## 核心实现

### transport.go

```go
// Transport QUIC 传输实现
type Transport struct {
    config    *quic.Config
    tlsConfig *tls.Config
    identity  identityif.Identity
}

// NewTransport 创建 QUIC 传输
func NewTransport(identity identityif.Identity, config Config) *Transport

// Dial 建立连接
func (t *Transport) Dial(ctx context.Context, addr interface{}) (transportif.Conn, error)

// Listen 监听连接
func (t *Transport) Listen(addr interface{}) (transportif.Listener, error)
```

### listener.go

```go
// Listener QUIC 监听器
type Listener struct {
    quicListener *quic.Listener
    addr         core.Address
}

// Accept 接受连接
func (l *Listener) Accept() (transportif.Conn, error)
```

### conn.go

```go
// Conn QUIC 连接包装
type Conn struct {
    quic.Connection
    localAddr  core.Address
    remoteAddr core.Address
    remotePub  crypto.PublicKey
}
```

## QUIC 配置

```go
var defaultQuicConfig = &quic.Config{
    MaxIdleTimeout:        30 * time.Second,
    MaxIncomingStreams:    256,
    MaxIncomingUniStreams: 256,
    KeepAlivePeriod:       15 * time.Second,
    EnableDatagrams:       true,
}
```

## TLS 配置

使用节点身份生成 TLS 证书，支持双向认证：

```go
func generateTLSConfig(identity identityif.Identity) *tls.Config {
    // 生成自签名证书
    // 设置客户端认证
    // 配置 ALPN
}
```

## 依赖

- `github.com/quic-go/quic-go` - QUIC 协议实现

