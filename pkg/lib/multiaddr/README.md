# pkg/multiaddr - 多地址工具包

> **版本**: v1.1.0  
> **覆盖率**: 82.2%  
> **协议对齐**: multiformats/multicodec

---

## 概述

`pkg/multiaddr` 提供多地址（Multiaddr）的完整实现，支持自描述的网络地址格式，兼容 libp2p 生态系统。

多地址是一种灵活的网络地址格式，可以描述任意协议组合：

```
/ip4/127.0.0.1/tcp/4001
/ip6/::1/tcp/8080
/ip4/192.168.1.1/udp/4001/quic-v1
/ip4/1.2.3.4/tcp/4001/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N
/dns/example.com/tcp/443/wss
```

---

## 快速开始

### 创建多地址

```go
import "github.com/dep2p/go-dep2p/pkg/multiaddr"

// 从字符串创建
ma, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
if err != nil {
    log.Fatal(err)
}

// 从字节创建
bytes := ma.Bytes()
ma2, err := multiaddr.NewMultiaddrBytes(bytes)
```

### 地址操作

```go
// 封装（添加组件）
p2p, _ := multiaddr.NewMultiaddr("/p2p/QmYyQ...")
full := ma.Encapsulate(p2p)
// 结果：/ip4/127.0.0.1/tcp/4001/p2p/QmYyQ...

// 解封装（移除组件）
transport := full.Decapsulate(p2p)
// 结果：/ip4/127.0.0.1/tcp/4001

// 获取协议值
ip, _ := ma.ValueForProtocol(multiaddr.P_IP4)
port, _ := ma.ValueForProtocol(multiaddr.P_TCP)
```

### 协议检查

```go
// 检查是否包含协议
hasTCP := multiaddr.HasProtocol(ma, multiaddr.P_TCP)
isIP4 := multiaddr.IsIP4Multiaddr(ma)
isIP6 := multiaddr.IsIP6Multiaddr(ma)
```

### 与标准网络类型转换

```go
// 从 net.TCPAddr 创建
tcpAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 4001}
ma, _ := multiaddr.FromTCPAddr(tcpAddr)

// 转换为 net.TCPAddr
tcpAddr, _ := ma.ToTCPAddr()

// 从 net.UDPAddr 创建
udpAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 5000}
ma, _ := multiaddr.FromUDPAddr(udpAddr)

// 转换为 net.UDPAddr
udpAddr, _ := ma.ToUDPAddr()
```

### P2P 地址处理

```go
// 分离传输地址和 PeerID
transport, peerID := multiaddr.Split(ma)

// 合并传输地址和 PeerID
full := multiaddr.Join(transport, peerID)

// 提取 PeerID
peerID, _ := multiaddr.GetPeerID(ma)

// 添加/替换 PeerID
ma, _ := multiaddr.WithPeerID(transport, "QmNewPeerID")

// 移除 PeerID
transport := multiaddr.WithoutPeerID(ma)
```

### 地址过滤和去重

```go
// 过滤 TCP 地址
tcpAddrs := multiaddr.FilterAddrs(addrs, func(ma multiaddr.Multiaddr) bool {
    return multiaddr.HasProtocol(ma, multiaddr.P_TCP)
})

// 去重
unique := multiaddr.UniqueAddrs(addrs)
```

---

## 支持的协议

### 核心协议（Phase 0）

| 协议 | 代码 | 说明 | 状态 |
|------|------|------|------|
| **ip4** | 0x0004 | IPv4 地址 | ✅ |
| **ip6** | 0x0029 | IPv6 地址 | ✅ |
| **tcp** | 0x0006 | TCP 端口 | ✅ |
| **udp** | 0x0111 | UDP 端口 | ✅ |
| **quic** | 0x01CC | QUIC (旧) | ✅ |
| **quic-v1** | 0x01CD | QUIC v1 | ✅ |
| **p2p** | 0x01A5 | Peer ID | ✅ |
| **ws** | 0x01DD | WebSocket | ✅ |
| **wss** | 0x01DE | WebSocket Secure | ✅ |
| **dns** | 0x0035 | DNS 名称 | ✅ |
| **dns4** | 0x0036 | DNS IPv4 | ✅ |
| **dns6** | 0x0037 | DNS IPv6 | ✅ |
| **dnsaddr** | 0x0038 | DNS 地址 | ✅ |

### 扩展协议

| 协议 | 代码 | 说明 | 状态 |
|------|------|------|------|
| **tls** | 0x01C0 | TLS | ✅ |
| **noise** | 0x01C6 | Noise | ✅ |
| **onion** | 0x01BC | Tor v2 | ✅ |
| **onion3** | 0x01BD | Tor v3 | ✅ |
| **garlic32** | 0x01BF | I2P 短地址 | ✅ |
| **garlic64** | 0x01BE | I2P 长地址 | ✅ |
| **p2p-circuit** | 0x0122 | 中继电路 | ✅ |
| **unix** | 0x0190 | Unix socket | ✅ |

---

## 二进制编码格式

多地址使用紧凑的二进制编码：

```
┌──────────────────────────────────────────────────────┐
│  Protocol 1:                                          │
│    [varint: code]                                     │
│    [varint: length]? (仅变长协议)                      │
│    [data bytes]                                       │
│                                                       │
│  Protocol 2:                                          │
│    [varint: code]                                     │
│    [varint: length]?                                  │
│    [data bytes]                                       │
│  ...                                                  │
└──────────────────────────────────────────────────────┘
```

**示例**：`/ip4/127.0.0.1/tcp/4001`

```
十六进制：04 7f 00 00 01 06 0f a1
解释：
  04       - IP4 协议代码（varint）
  7f000001 - 127.0.0.1（4 字节）
  06       - TCP 协议代码（varint）
  0fa1     - 端口 4001（2 字节，大端序）
```

---

## API 参考

### 核心接口

```go
type Multiaddr interface {
    // 基本方法
    Bytes() []byte
    String() string
    Equal(Multiaddr) bool
    
    // 协议操作
    Protocols() []Protocol
    ValueForProtocol(code int) (string, error)
    
    // 地址操作
    Encapsulate(Multiaddr) Multiaddr
    Decapsulate(Multiaddr) Multiaddr
    
    // 网络转换
    ToTCPAddr() (*net.TCPAddr, error)
    ToUDPAddr() (*net.UDPAddr, error)
}
```

### Protocol 结构

```go
type Protocol struct {
    Name       string      // 协议名称（如 "ip4"）
    Code       int         // 协议代码（如 0x0004）
    VCode      []byte      // 预计算的 varint 编码
    Size       int         // 数据大小（位，-1 表示变长）
    Path       bool        // 是否为路径协议
    Transcoder Transcoder  // 编解码器
}
```

### 构造函数

```go
// 从字符串创建（验证）
func NewMultiaddr(s string) (Multiaddr, error)

// 从字节创建（验证）
func NewMultiaddrBytes(b []byte) (Multiaddr, error)

// 强制转换（不验证，仅用于已知有效的数据）
func Cast(b []byte) Multiaddr
```

### 转换函数

```go
// 从标准网络类型创建
func FromTCPAddr(addr *net.TCPAddr) (Multiaddr, error)
func FromUDPAddr(addr *net.UDPAddr) (Multiaddr, error)
func FromNetAddr(addr net.Addr) (Multiaddr, error)
```

### 工具函数

```go
// P2P 地址处理
func Split(m Multiaddr) (transport Multiaddr, peerID string)
func Join(transport Multiaddr, peerID string) Multiaddr
func GetPeerID(m Multiaddr) (string, error)
func WithPeerID(m Multiaddr, peerID string) (Multiaddr, error)
func WithoutPeerID(m Multiaddr) Multiaddr

// 地址过滤
func FilterAddrs(addrs []Multiaddr, filter func(Multiaddr) bool) []Multiaddr
func UniqueAddrs(addrs []Multiaddr) []Multiaddr

// 协议检查
func HasProtocol(m Multiaddr, code int) bool
func IsTCPMultiaddr(m Multiaddr) bool
func IsUDPMultiaddr(m Multiaddr) bool
func IsIP4Multiaddr(m Multiaddr) bool
func IsIP6Multiaddr(m Multiaddr) bool
func IsIPMultiaddr(m Multiaddr) bool
```

---

## 文件结构

```
pkg/multiaddr/
├── doc.go              # 包文档
├── README.md           # 本文件
├── errors.go           # 错误定义
├── multiaddr.go        # 核心接口和实现
├── protocols.go        # 协议定义和注册表
├── codec.go            # 二进制编解码
├── transcoder.go       # 协议 transcoder
├── varint.go           # Varint 编解码
├── convert.go          # net.Addr 转换
├── util.go             # 工具函数
├── *_test.go           # 单元测试
```

---

## 性能基准

| 操作 | 耗时 | 内存分配 |
|------|------|----------|
| NewMultiaddr | ~496 ns | 266 B |
| String() | ~329 ns | 208 B |
| Bytes() | ~2.5 ns | 0 B |
| 协议查找（名称） | ~19 ns | 0 B |
| 协议查找（代码） | ~12 ns | 0 B |
| Split | ~888 ns | 392 B |
| Join | ~471 ns | 352 B |

---

## 与 multiformats 对齐

本实现完全遵循 [multiformats/multicodec](https://github.com/multiformats/multicodec) 规范，所有协议代码与标准一致。

**兼容性**：
- ✅ 二进制格式兼容 go-multiaddr
- ✅ 协议代码完全对齐
- ✅ 支持标准 Transcoder 接口

---

## 相关文档

- [设计文档](../../design/03_architecture/L6_domains/pkg_multiaddr/)
- [pkg_design.md](../../design/02_constraints/engineering/standards/pkg_design.md) - pkg 层设计原则
- [multiformats/multicodec](https://github.com/multiformats/multicodec) - 协议代码规范

---

## 示例代码

### 完整示例

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/dep2p/go-dep2p/pkg/multiaddr"
)

func main() {
    // 创建基础地址
    ma, err := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
    if err != nil {
        log.Fatal(err)
    }
    
    // 添加 P2P 组件
    p2p, _ := multiaddr.NewMultiaddr("/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
    full := ma.Encapsulate(p2p)
    
    fmt.Println("完整地址:", full.String())
    // 输出：/ip4/127.0.0.1/tcp/4001/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N
    
    // 分离组件
    transport, peerID := multiaddr.Split(full)
    fmt.Println("传输地址:", transport.String())
    fmt.Println("PeerID:", peerID)
    
    // 转换为 TCP 地址
    tcpAddr, _ := transport.ToTCPAddr()
    fmt.Printf("TCP 地址: %s:%d\n", tcpAddr.IP, tcpAddr.Port)
}
```

---

**最后更新**：2026-01-13
