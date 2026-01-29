# pkg_multiaddr 测试策略

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13

---

## 测试目标

| 目标 | 指标 | 状态 |
|------|------|------|
| 覆盖率 | > 80% | ✅ 82.2% |
| 边界测试 | 所有边界条件 | ✅ 完成 |
| 性能基准 | 所有核心操作 | ✅ 完成 |
| 往返测试 | 所有协议 | ✅ 完成 |

---

## 测试分类

### 1. 单元测试

**目标**：验证每个组件的正确性

| 组件 | 测试文件 | 覆盖内容 |
|------|---------|---------|
| Multiaddr | `multiaddr_test.go` | 创建、操作、序列化 |
| Codec | `codec_test.go` | 编解码、往返 |
| Protocols | `protocols_test.go` | 协议查找、注册表 |
| Transcoder | `transcoder_test.go` | 各协议编解码 |
| Convert | `convert_test.go` | net.Addr 转换 |
| Util | `util_test.go` | 工具函数 |
| Varint | `varint_test.go` | Varint 编解码 |

### 2. 集成测试

**目标**：验证与其他模块的集成

```go
// 与 pkg/types 集成
func TestIntegration_WithPkgTypes(t *testing.T) {
    // 使用 types.NewMultiaddr
    ma, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
    
    // 使用 types 的工具函数
    transport, peerID := types.SplitMultiaddr(ma)
    
    // 验证结果
    assert.NotNil(t, transport)
    assert.Empty(t, peerID)
}
```

### 3. 基准测试

**目标**：性能验证和回归检测

```go
func BenchmarkNewMultiaddr(b *testing.B) {
    addr := "/ip4/127.0.0.1/tcp/4001/p2p/QmYyQ..."
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = NewMultiaddr(addr)
    }
}
```

---

## 测试用例设计

### 正常路径测试

```go
func TestNewMultiaddr_Valid(t *testing.T) {
    tests := []string{
        "/ip4/127.0.0.1/tcp/4001",
        "/ip6/::1/tcp/8080",
        "/ip4/1.2.3.4/udp/5000/quic-v1",
        "/dns/example.com/tcp/443/wss",
    }
    
    for _, addr := range tests {
        t.Run(addr, func(t *testing.T) {
            ma, err := NewMultiaddr(addr)
            if err != nil {
                t.Fatalf("NewMultiaddr(%q) error = %v", addr, err)
            }
            if ma.String() != addr {
                t.Errorf("Round trip failed")
            }
        })
    }
}
```

### 边界条件测试

```go
func TestNewMultiaddr_EdgeCases(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {"Empty", "", true},
        {"No leading slash", "ip4/127.0.0.1", true},
        {"Unknown protocol", "/unknown/value", true},
        {"Incomplete", "/ip4", true},
        {"Trailing slashes", "/ip4/127.0.0.1/", false},
        {"Zero port", "/ip4/127.0.0.1/tcp/0", false},
        {"Max port", "/ip4/127.0.0.1/tcp/65535", false},
        {"Over max port", "/ip4/127.0.0.1/tcp/65536", true},
    }
    // ...
}
```

### 往返测试

```go
func TestRoundTrip(t *testing.T) {
    tests := []string{
        "/ip4/127.0.0.1/tcp/4001",
        "/ip6/::1/tcp/8080",
        "/dns/example.com/tcp/443",
    }
    
    for _, addr := range tests {
        t.Run(addr, func(t *testing.T) {
            // String -> Bytes
            b, err := stringToBytes(addr)
            if err != nil {
                t.Fatalf("stringToBytes() error = %v", err)
            }
            
            // Bytes -> String
            s, err := bytesToString(b)
            if err != nil {
                t.Fatalf("bytesToString() error = %v", err)
            }
            
            if s != addr {
                t.Errorf("Round trip: got %v, want %v", s, addr)
            }
        })
    }
}
```

### 协议对齐测试

```go
func TestProtocolCodes_AlignWithMultiformats(t *testing.T) {
    // 验证与 multiformats/multicodec 对齐
    tests := []struct {
        name string
        code int
        want int
    }{
        {"IP4", P_IP4, 0x0004},
        {"TCP", P_TCP, 0x0006},
        {"UDP", P_UDP, 0x0111},
        {"P2P", P_P2P, 0x01A5},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if tt.code != tt.want {
                t.Errorf("%s code = 0x%04X, want 0x%04X", tt.name, tt.code, tt.want)
            }
        })
    }
}
```

---

## 覆盖率策略

### 核心功能优先

1. **P0 优先级**：
   - IP4/IP6/TCP/UDP（必须 100%）
   - 字符串/二进制编解码（必须 100%）
   - 地址操作（必须 100%）

2. **P1 优先级**：
   - DNS/P2P/QUIC（目标 > 80%）
   - 网络转换（目标 > 80%）
   - 工具函数（目标 > 70%）

3. **P2 优先级**：
   - Onion/Garlic（目标 > 50%）
   - 特殊协议（基本测试即可）

### 当前覆盖率

```
总体覆盖率：82.2%

详细分组：
  multiaddr.go    100%   (核心实现)
  protocols.go    95%    (协议定义)
  codec.go        92%    (编解码)
  transcoder.go   75%    (部分特殊协议未覆盖)
  varint.go       100%   (Varint)
  convert.go      90%    (网络转换)
  util.go         78%    (工具函数)
```

---

## 测试数据管理

### 已知地址示例

```go
var testAddresses = []string{
    "/ip4/127.0.0.1/tcp/4001",
    "/ip6/::1/tcp/8080",
    "/ip4/192.168.1.1/udp/5000/quic-v1",
    "/dns/example.com/tcp/443/wss",
    "/ip4/1.2.3.4/tcp/4001/p2p/QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N",
}
```

### 无效地址示例

```go
var invalidAddresses = []string{
    "",                          // 空
    "ip4/127.0.0.1",            // 无 /
    "/unknown/value",           // 未知协议
    "/ip4",                     // 不完整
    "/ip4/999.999.999.999",     // 无效 IP
    "/ip4/127.0.0.1/tcp/99999", // 端口越界
}
```

---

## 性能测试

### 关键操作基准

```go
// 地址创建
func BenchmarkNewMultiaddr(b *testing.B) { ... }

// 字符串/字节转换
func BenchmarkMultiaddr_String(b *testing.B) { ... }
func BenchmarkMultiaddr_Bytes(b *testing.B) { ... }

// 协议查找
func BenchmarkProtocolWithName(b *testing.B) { ... }
func BenchmarkProtocolWithCode(b *testing.B) { ... }

// 地址操作
func BenchmarkSplit(b *testing.B) { ... }
func BenchmarkJoin(b *testing.B) { ... }

// 编解码
func BenchmarkStringToBytes(b *testing.B) { ... }
func BenchmarkBytesToString(b *testing.B) { ... }
```

### 性能目标

| 操作 | 目标 | 实际 | 状态 |
|------|------|------|------|
| NewMultiaddr | < 1μs | ~496ns | ✅ |
| String() | < 500ns | ~329ns | ✅ |
| Bytes() | < 10ns | ~2.5ns | ✅ |
| 协议查找 | < 50ns | ~12-19ns | ✅ |

---

## 测试执行策略

### CI/CD 流程

```bash
# 单元测试
go test ./pkg/multiaddr/...

# 带覆盖率
go test -cover ./pkg/multiaddr/...

# 基准测试
go test -bench=. -benchmem ./pkg/multiaddr/...

# 竞态检测
go test -race ./pkg/multiaddr/...
```

### 本地测试

```bash
# 快速测试
make test-multiaddr

# 详细测试
go test -v ./pkg/multiaddr/...

# 覆盖率报告
go test -coverprofile=coverage.out ./pkg/multiaddr/...
go tool cover -html=coverage.out
```

---

## 测试维护

### 定期审查

- [ ] 每月审查测试覆盖率
- [ ] 新增功能必须有测试
- [ ] 修复 bug 必须添加回归测试
- [ ] 基准测试结果趋势分析

### 测试质量指标

| 指标 | 目标 | 当前 |
|------|------|------|
| 覆盖率 | > 80% | 82.2% |
| 测试数量 | > 80 | 95+ |
| 平均测试时间 | < 2s | 0.9s |
| Flaky 测试数 | 0 | 0 |

---

## 相关文档

- [../coding/guidelines.md](../coding/guidelines.md) - 编码指南
- [../design/overview.md](../design/overview.md) - 设计概述

---

**最后更新**：2026-01-13
