# TLS 安全实现

## 概述

基于 Go 标准库 `crypto/tls` 的 TLS 1.3 安全实现。

## 文件结构

```
tls/
├── README.md        # 本文件
├── transport.go     # SecureTransport 实现
└── config.go        # TLS 配置生成
```

## 核心实现

### transport.go

```go
// Transport TLS 安全传输
type Transport struct {
    identity    identityif.Identity
    serverConfig *tls.Config
    clientConfig *tls.Config
}

// NewTransport 创建 TLS 传输
func NewTransport(identity identityif.Identity, config Config) *Transport

// SecureInbound 安全入站连接
func (t *Transport) SecureInbound(ctx context.Context, conn transportif.Conn) (securityif.SecureConn, error)

// SecureOutbound 安全出站连接
func (t *Transport) SecureOutbound(ctx context.Context, conn transportif.Conn, remotePeer types.NodeID) (securityif.SecureConn, error)
```

### config.go

```go
// GenerateCertificate 生成自签名证书
func GenerateCertificate(identity identityif.Identity) (*tls.Certificate, error)

// ServerConfig 生成服务端 TLS 配置
func ServerConfig(cert *tls.Certificate) *tls.Config

// ClientConfig 生成客户端 TLS 配置
func ClientConfig(cert *tls.Certificate) *tls.Config

// ExtractNodeID 从证书提取 NodeID
func ExtractNodeID(cert *x509.Certificate) (types.NodeID, error)
```

## TLS 配置详情

### 服务端配置

```go
&tls.Config{
    MinVersion:               tls.VersionTLS13,
    Certificates:             []tls.Certificate{cert},
    ClientAuth:               tls.RequireAnyClientCert,
    NextProtos:               []string{"dep2p/1.0.0"},
    VerifyPeerCertificate:    verifyCallback,
    SessionTicketsDisabled:   false,
    PreferServerCipherSuites: true,
}
```

### 客户端配置

```go
&tls.Config{
    MinVersion:            tls.VersionTLS13,
    Certificates:          []tls.Certificate{cert},
    InsecureSkipVerify:    true,  // 我们自己验证
    VerifyPeerCertificate: verifyCallback,
    NextProtos:            []string{"dep2p/1.0.0"},
}
```

## 证书格式

### X.509 扩展

NodeID 编码到证书的 Subject Alternative Name (SAN) 扩展：

```
OID: 1.3.6.1.4.1.99999.1  (dep2p NodeID)
Value: <32 bytes NodeID>
```

### 证书有效期

- 默认有效期：1 年
- 自动续期机制

## 握手流程

```
Client                              Server
   |                                   |
   |-------- ClientHello ------------>|
   |                                   |
   |<------- ServerHello -------------|
   |<------- Certificate -------------|
   |<------- CertificateRequest ------|
   |<------- ServerHelloDone ---------|
   |                                   |
   |-------- Certificate ------------>|
   |-------- ClientKeyExchange ------>|
   |-------- CertificateVerify ------>|
   |-------- Finished --------------->|
   |                                   |
   |<------- Finished ----------------|
   |                                   |
   |======= Encrypted Data ==========>|
```

## 依赖

- Go 标准库 `crypto/tls`
- Go 标准库 `crypto/x509`

