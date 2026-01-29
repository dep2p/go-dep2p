# pkg_multiaddr 内部实现细节

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13

---

## Varint 编码实现

### 什么是 Varint

Varint（Variable-length Integer）是一种变长整数编码，小数字使用更少的字节。

### 编码规则

```
每个字节：
  - 最高位（MSB）：继续位（1=有后续字节，0=最后一个字节）
  - 低 7 位：数据位

示例：
  300 = 0b100101100
  
  编码为：10101100 00000010
         ^^^^^^^^  ^^^^^^^^
         低 7 位    高 2 位
         (续)      (终)
```

### 实现

```go
func uvarintEncode(x uint64) []byte {
    buf := make([]byte, binary.MaxVarintLen64)
    n := binary.PutUvarint(buf, x)
    return buf[:n]
}

func uvarintDecode(buf []byte) (uint64, int, error) {
    x, n := binary.Uvarint(buf)
    if n == 0 {
        return 0, 0, ErrVarintTooShort
    }
    if n < 0 {
        return 0, 0, ErrVarintTooBig
    }
    return x, n, nil
}
```

**优化**：使用标准库 `encoding/binary`，性能最优。

---

## 二进制编码格式

### 格式规范

```
┌──────────────────────────────────────────────────────┐
│  Protocol 1:                                          │
│    [varint: protocol_code]                            │
│    [varint: data_length]? (仅变长协议)                 │
│    [data_bytes]                                       │
│                                                       │
│  Protocol 2:                                          │
│    [varint: protocol_code]                            │
│    [varint: data_length]?                             │
│    [data_bytes]                                       │
│  ...                                                  │
└──────────────────────────────────────────────────────┘
```

### 固定长度 vs 变长

**固定长度**（Size > 0）：
- IP4: 32 位 = 4 字节
- IP6: 128 位 = 16 字节
- TCP/UDP: 16 位 = 2 字节

**变长**（Size = -1）：
- DNS: 长度前缀 + 域名字节
- P2P: 长度前缀 + PeerID 字节
- Unix: 长度前缀 + 路径字节

**无数据**（Size = 0）：
- QUIC, WS, WSS, TLS 等

---

## 编码示例

### 示例 1: /ip4/127.0.0.1/tcp/4001

```
字符串：/ip4/127.0.0.1/tcp/4001

二进制（十六进制）：
  04 7f 00 00 01 06 0f a1

详细解释：
  04       - IP4 协议代码（varint: 4）
  7f000001 - 127.0.0.1（4 字节）
  06       - TCP 协议代码（varint: 6）
  0fa1     - 端口 4001（2 字节，大端序）
```

### 示例 2: /ip4/1.2.3.4/udp/5000/quic-v1

```
字符串：/ip4/1.2.3.4/udp/5000/quic-v1

二进制（十六进制）：
  04 01 02 03 04 91 02 13 88 cd 01

详细解释：
  04       - IP4 协议代码
  01020304 - 1.2.3.4（4 字节）
  91 02    - UDP 协议代码（varint: 273 = 0x111）
  1388     - 端口 5000（2 字节，大端序）
  cd 01    - QUIC-V1 协议代码（varint: 461 = 0x1CD）
```

### 示例 3: /dns/example.com/tcp/80

```
字符串：/dns/example.com/tcp/80

二进制（十六进制）：
  35 0b 65 78 61 6d 70 6c 65 2e 63 6f 6d 06 00 50

详细解释：
  35       - DNS 协议代码（varint: 53）
  0b       - 长度：11 字节（varint）
  65...6d  - "example.com"（11 字节）
  06       - TCP 协议代码
  0050     - 端口 80（2 字节）
```

---

## Transcoder 实现细节

### IP4 Transcoder

```go
func ip4StringToBytes(s string) ([]byte, error) {
    ip := net.ParseIP(s).To4()
    if ip == nil {
        return nil, fmt.Errorf("failed to parse ip4: %s", s)
    }
    return ip, nil  // 4 字节
}

func ip4BytesToString(b []byte) (string, error) {
    if len(b) != 4 {
        return "", fmt.Errorf("invalid ip4 length: %d", len(b))
    }
    return net.IP(b).String(), nil
}
```

### Port Transcoder（TCP/UDP）

```go
func portStringToBytes(s string) ([]byte, error) {
    port, err := strconv.ParseUint(s, 10, 16)
    if err != nil {
        return nil, err
    }
    if port > 65535 {
        return nil, errors.New("port out of range")
    }
    
    b := make([]byte, 2)
    binary.BigEndian.PutUint16(b, uint16(port))
    return b, nil  // 2 字节，大端序
}
```

### DNS Transcoder

```go
func dnsStringToBytes(s string) ([]byte, error) {
    if len(s) == 0 {
        return nil, errors.New("empty DNS name")
    }
    // 验证不包含 '/'（会破坏 multiaddr 解析）
    if strings.Contains(s, "/") {
        return nil, fmt.Errorf("DNS name contains '/': %s", s)
    }
    return []byte(s), nil  // 直接 UTF-8 字节
}
```

### P2P Transcoder

```go
func p2pStringToBytes(s string) ([]byte, error) {
    if len(s) == 0 {
        return nil, errors.New("empty peer ID")
    }
    // 简化实现：直接存储字符串
    // 完整实现应该 base58 解码
    return []byte(s), nil
}
```

---

## 协议大小计算

### sizeForAddr 函数

```go
func sizeForAddr(proto Protocol, b []byte) (int, int, error) {
    if proto.Size == 0 {
        // 无数据协议
        return 0, 0, nil
    }
    
    if proto.Size == LengthPrefixedVarSize {
        // 变长：读取长度前缀
        length, n, err := uvarintDecode(b)
        if err != nil {
            return 0, 0, err
        }
        return n, int(length), nil
    }
    
    // 固定长度（位转字节）
    return 0, proto.Size / 8, nil
}
```

**返回值**：
- prefixBytes：长度前缀占用的字节数
- dataBytes：协议数据占用的字节数
- error：读取错误

---

## 地址操作实现

### Encapsulate（封装）

```go
func (m *multiaddr) Encapsulate(other Multiaddr) Multiaddr {
    if other == nil {
        return m
    }
    
    mb := m.bytes
    ob := other.Bytes()
    
    // 简单拼接
    result := make([]byte, len(mb)+len(ob))
    copy(result, mb)
    copy(result[len(mb):], ob)
    
    return &multiaddr{bytes: result}
}
```

### Decapsulate（解封装）

```go
func (m *multiaddr) Decapsulate(other Multiaddr) Multiaddr {
    if other == nil {
        return m
    }
    
    mb := m.bytes
    ob := other.Bytes()
    
    // 检查后缀匹配
    if len(ob) > len(mb) {
        return m
    }
    
    if bytes.Equal(mb[len(mb)-len(ob):], ob) {
        // 移除后缀
        return &multiaddr{bytes: mb[:len(mb)-len(ob)]}
    }
    
    return m  // 不匹配，返回自身
}
```

---

## 网络地址转换

### ToTCPAddr 实现

```
算法：
  1. 提取 IP（尝试 IPv4，再尝试 IPv6）
  2. 提取 TCP 端口
  3. 解析为 net.TCPAddr
```

```go
func (m *multiaddr) ToTCPAddr() (*net.TCPAddr, error) {
    // 获取 IP
    var ipStr string
    var err error
    
    ipStr, err = m.ValueForProtocol(P_IP4)
    if err != nil {
        ipStr, err = m.ValueForProtocol(P_IP6)
        if err != nil {
            return nil, fmt.Errorf("no IP in multiaddr")
        }
    }
    
    // 获取端口
    port, err := m.ValueForProtocol(P_TCP)
    if err != nil {
        return nil, fmt.Errorf("no TCP port")
    }
    
    // 组装
    return &net.TCPAddr{
        IP:   net.ParseIP(ipStr),
        Port: atoi(port),
    }, nil
}
```

---

## P2P 地址处理

### Split 实现

```go
func Split(m Multiaddr) (transport Multiaddr, peerID string) {
    s := m.String()
    
    // 查找 /p2p/ 分隔符
    idx := strings.Index(s, "/p2p/")
    if idx < 0 {
        return m, ""  // 无 P2P 组件
    }
    
    // 分离
    transportStr := s[:idx]
    peerID = s[idx+5:]  // 跳过 "/p2p/"
    
    if transportStr != "" {
        transport, _ = NewMultiaddr(transportStr)
    }
    
    return
}
```

### Join 实现

```go
func Join(transport Multiaddr, peerID string) Multiaddr {
    if peerID == "" {
        return transport
    }
    
    p2pAddr, _ := NewMultiaddr("/p2p/" + peerID)
    
    if transport == nil {
        return p2pAddr
    }
    
    return transport.Encapsulate(p2pAddr)
}
```

---

## 相关文档

- [overview.md](overview.md) - 设计概述
- [../coding/guidelines.md](../coding/guidelines.md) - 编码指南

---

**最后更新**：2026-01-13
