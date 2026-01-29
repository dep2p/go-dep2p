# pkg_multiaddr 编码指南

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13

---

## 代码组织

### 文件命名规范

| 文件类型 | 命名规则 | 示例 |
|---------|---------|------|
| 核心实现 | `<功能>.go` | `multiaddr.go` |
| 测试文件 | `<功能>_test.go` | `multiaddr_test.go` |
| 协议定义 | `protocols.go` | `protocols.go` |
| 编解码 | `codec.go`, `varint.go` | - |

### 导入顺序

```go
import (
    // 1. 标准库
    "bytes"
    "encoding/binary"
    "fmt"
    "net"
    "strings"
    
    // 2. 无外部依赖
)
```

---

## 接口实现规范

### Multiaddr 接口实现

```go
// ✅ 好：返回共享字节，避免拷贝
func (m *multiaddr) Bytes() []byte {
    return m.bytes
}

// ❌ 坏：不必要的拷贝
func (m *multiaddr) Bytes() []byte {
    b := make([]byte, len(m.bytes))
    copy(b, m.bytes)
    return b
}
```

### Equal 实现

```go
// ✅ 好：使用字节比较
func (m *multiaddr) Equal(other Multiaddr) bool {
    if other == nil {
        return false
    }
    return bytes.Equal(m.bytes, other.Bytes())
}

// ❌ 坏：字符串比较（低效）
func (m *multiaddr) Equal(other Multiaddr) bool {
    return m.String() == other.String()
}
```

---

## 错误处理规范

### 预定义错误

```go
// ✅ 好：使用预定义错误
var ErrInvalidMultiaddr = errors.New("invalid multiaddr")

func NewMultiaddr(s string) (Multiaddr, error) {
    if !strings.HasPrefix(s, "/") {
        return nil, fmt.Errorf("%w: must begin with /", ErrInvalidMultiaddr)
    }
    // ...
}
```

### 错误包装

```go
// ✅ 好：提供上下文
func (m *multiaddr) ValueForProtocol(code int) (string, error) {
    proto := ProtocolWithCode(code)
    if proto.Code == 0 {
        return "", fmt.Errorf("unknown protocol code: %d", code)
    }
    // ...
}

// ❌ 坏：错误消息不清晰
func (m *multiaddr) ValueForProtocol(code int) (string, error) {
    // ...
    return "", errors.New("error")
}
```

---

## 性能优化规范

### 1. 避免字符串拼接

```go
// ✅ 好：使用 strings.Builder
func bytesToString(b []byte) (string, error) {
    var sb strings.Builder
    // ...
    sb.WriteString("/")
    sb.WriteString(proto.Name)
    // ...
    return sb.String(), nil
}

// ❌ 坏：频繁字符串拼接
func bytesToString(b []byte) (string, error) {
    s := ""
    s += "/" + proto.Name
    s += "/" + value
    return s, nil
}
```

### 2. 预分配切片

```go
// ✅ 好：预分配容量
result := make([]Multiaddr, 0, len(addrs))

// ❌ 坏：频繁扩容
var result []Multiaddr
```

### 3. 协议查找优化

```go
// ✅ 好：使用 map 查找（O(1)）
var protocols = map[int]Protocol{...}
func ProtocolWithCode(code int) Protocol {
    if p, ok := protocols[code]; ok {
        return p
    }
    return Protocol{}
}

// ❌ 坏：线性查找（O(n)）
func ProtocolWithCode(code int) Protocol {
    for _, p := range allProtocols {
        if p.Code == code {
            return p
        }
    }
    return Protocol{}
}
```

---

## 注释规范

### GoDoc 注释

```go
// ✅ 好：完整的 GoDoc
// NewMultiaddr 从字符串创建多地址
//
// 输入的字符串必须以 / 开头，并且包含有效的协议/值对。
//
// 参数：
//   - s: 多地址字符串
//
// 返回：
//   - Multiaddr: 多地址对象
//   - error: 解析错误
//
// 示例：
//   ma, err := NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
func NewMultiaddr(s string) (Multiaddr, error) {
    // ...
}

// ❌ 坏：注释不完整
// 创建多地址
func NewMultiaddr(s string) (Multiaddr, error) {
    // ...
}
```

---

## 测试编码规范

### 测试命名

```go
// ✅ 好：清晰的测试名称
func TestNewMultiaddr_IPv4_TCP(t *testing.T) { ... }
func TestCodec_RoundTrip(t *testing.T) { ... }
func TestTranscoderIP4_InvalidIP(t *testing.T) { ... }

// ❌ 坏：模糊的名称
func TestMultiaddr(t *testing.T) { ... }
func Test1(t *testing.T) { ... }
```

### 表驱动测试

```go
// ✅ 好：使用表驱动测试
func TestNewMultiaddr(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {"IPv4 + TCP", "/ip4/127.0.0.1/tcp/4001", false},
        {"Empty", "", true},
        // ...
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            _, err := NewMultiaddr(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

---

## 代码审查清单

### 功能性

- [ ] 协议代码与 multiformats 对齐
- [ ] 二进制格式兼容 go-multiaddr
- [ ] 所有 Transcoder 实现正确
- [ ] 边界条件处理完整

### 性能

- [ ] 避免不必要的内存分配
- [ ] 使用 strings.Builder 构建字符串
- [ ] 协议查找使用 map
- [ ] Bytes() 避免拷贝

### 可维护性

- [ ] GoDoc 注释完整
- [ ] 代码结构清晰
- [ ] 错误处理完善
- [ ] 测试覆盖充分

---

## 相关文档

- [../design/overview.md](../design/overview.md) - 设计概述
- [../testing/strategy.md](../testing/strategy.md) - 测试策略

---

**最后更新**：2026-01-13
