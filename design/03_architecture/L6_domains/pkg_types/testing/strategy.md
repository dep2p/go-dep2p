# pkg_types 测试策略

> 测试范围与方法

---

## 测试覆盖目标

| 文件 | 覆盖要求 | 当前状态 | 说明 |
|------|----------|----------|------|
| ids.go | > 80% | ✅ | 核心 ID 类型 |
| enums.go | > 80% | ✅ | 枚举类型 |
| base58.go | > 90% | ✅ | 编解码（核心） |
| protocol.go | > 80% | ✅ | 协议类型 |
| multiaddr.go | > 80% | ✅ | 地址类型 |
| discovery.go | > 80% | ✅ | 发现类型 |
| realm.go | > 80% | ✅ | Realm 类型 |
| events.go | > 70% | ✅ | 事件类型 |
| connection.go | > 70% | ✅ | 连接类型 |
| stream.go | > 70% | ✅ | 流类型 |

**整体覆盖率目标**: > 80%  
**当前覆盖率**: 89.4% ✅

---

## 测试类型

### PeerID 测试

```go
func TestPeerID_Parse(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {"valid", "12D3KooWTest", false},
        {"empty", "", true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            _, err := types.ParsePeerID(tt.input)
            if (err != nil) != tt.wantErr {
                t.Errorf("ParsePeerID() error = %v", err)
            }
        })
    }
}

func TestPeerID_Equal(t *testing.T) {
    id1 := types.PeerID("test")
    id2 := types.PeerID("test")
    id3 := types.PeerID("other")
    
    if !id1.Equal(id2) {
        t.Error("Equal() = false for same")
    }
    if id1.Equal(id3) {
        t.Error("Equal() = true for different")
    }
}

func TestPeerID_ShortString(t *testing.T) {
    id := types.PeerID("12D3KooWLongPeerIDString")
    short := id.ShortString()
    
    if len(short) > 11 { // 8 + "..."
        t.Errorf("ShortString() too long: %q", short)
    }
}
```

### PSK 测试

```go
func TestGeneratePSK(t *testing.T) {
    psk1 := types.GeneratePSK()
    psk2 := types.GeneratePSK()
    
    // 长度检查
    if psk1.Len() != types.PSKLength {
        t.Errorf("Len() = %d", psk1.Len())
    }
    
    // 唯一性检查
    if psk1.Equal(psk2) {
        t.Error("Two generated PSKs are equal")
    }
}

func TestPSK_FromBytes(t *testing.T) {
    // 有效
    data := make([]byte, types.PSKLength)
    psk, err := types.PSKFromBytes(data)
    if err != nil {
        t.Errorf("PSKFromBytes() error = %v", err)
    }
    
    // 无效长度
    _, err = types.PSKFromBytes([]byte{1, 2, 3})
    if err == nil {
        t.Error("PSKFromBytes() should fail for invalid length")
    }
}

func TestPSK_DeriveRealmID(t *testing.T) {
    psk := types.GeneratePSK()
    
    // 派生应确定性
    r1 := psk.DeriveRealmID()
    r2 := psk.DeriveRealmID()
    
    if r1 != r2 {
        t.Error("DeriveRealmID() not deterministic")
    }
    
    // 不同 PSK 应产生不同 RealmID
    psk2 := types.GeneratePSK()
    r3 := psk2.DeriveRealmID()
    
    if r1 == r3 {
        t.Error("Different PSKs produced same RealmID")
    }
}
```

### Base58 测试

```go
func TestBase58_RoundTrip(t *testing.T) {
    tests := [][]byte{
        {},
        {0},
        {0, 0, 0},
        {1, 2, 3, 4, 5},
        []byte("Hello, World!"),
    }
    
    for i, original := range tests {
        encoded := types.Base58Encode(original)
        decoded, err := types.Base58Decode(encoded)
        
        if err != nil {
            t.Errorf("Test %d: decode error = %v", i, err)
            continue
        }
        
        if !bytes.Equal(decoded, original) {
            t.Errorf("Test %d: roundtrip failed", i)
        }
    }
}

func TestBase58_InvalidChars(t *testing.T) {
    // 0, O, I, l 是无效字符
    invalid := []string{"0abc", "abcO", "abcI", "abcl"}
    
    for _, s := range invalid {
        _, err := types.Base58Decode(s)
        if err == nil {
            t.Errorf("Base58Decode(%q) should fail", s)
        }
    }
}
```

### ProtocolID 测试

```go
func TestProtocolID_Categories(t *testing.T) {
    tests := []struct {
        proto    types.ProtocolID
        isSystem bool
        isRealm  bool
        isApp    bool
    }{
        {"/dep2p/sys/ping/1.0.0", true, false, false},
        {"/dep2p/realm/xxx/chat/1.0.0", false, true, false},
        {"/dep2p/app/xxx/custom/1.0.0", false, false, true},
        {"/other/proto", false, false, false},
    }
    
    for _, tt := range tests {
        if tt.proto.IsSystem() != tt.isSystem {
            t.Errorf("%q.IsSystem() = %v", tt.proto, !tt.isSystem)
        }
        if tt.proto.IsRealm() != tt.isRealm {
            t.Errorf("%q.IsRealm() = %v", tt.proto, !tt.isRealm)
        }
        if tt.proto.IsApp() != tt.isApp {
            t.Errorf("%q.IsApp() = %v", tt.proto, !tt.isApp)
        }
    }
}
```

### Events 测试

```go
func TestBaseEvent(t *testing.T) {
    evt := types.NewBaseEvent("test_event")
    
    if evt.Type() != "test_event" {
        t.Errorf("Type() = %q", evt.Type())
    }
    
    if time.Since(evt.Timestamp()) > time.Second {
        t.Error("Timestamp() is not recent")
    }
}

func TestEventTypes(t *testing.T) {
    // 验证所有事件类型常量唯一
    seen := make(map[string]bool)
    types := []string{
        types.EventTypePeerConnected,
        types.EventTypePeerDisconnected,
        // ...
    }
    
    for _, typ := range types {
        if seen[typ] {
            t.Errorf("Duplicate event type: %q", typ)
        }
        seen[typ] = true
    }
}
```

---

## 基准测试

```go
func BenchmarkPeerIDFromPublicKey(b *testing.B) {
    pubKey := make([]byte, 32)
    for i := range pubKey {
        pubKey[i] = byte(i)
    }
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        types.PeerIDFromPublicKey(pubKey)
    }
}

func BenchmarkBase58Encode(b *testing.B) {
    data := make([]byte, 32)
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        types.Base58Encode(data)
    }
}

func BenchmarkGeneratePSK(b *testing.B) {
    for i := 0; i < b.N; i++ {
        types.GeneratePSK()
    }
}
```

---

## 测试运行

```bash
# 运行所有测试
go test ./pkg/types/...

# 带覆盖率
go test ./pkg/types/... -cover

# 详细输出
go test ./pkg/types/... -v

# 运行基准测试
go test ./pkg/types/... -bench=.

# 生成覆盖率报告
go test ./pkg/types/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

---

## 测试文件清单

| 测试文件 | 覆盖源文件 |
|----------|------------|
| ids_test.go | ids.go |
| enums_test.go | enums.go |
| base58_test.go | base58.go |
| protocol_test.go | protocol.go |
| multiaddr_test.go | multiaddr.go |
| discovery_test.go | discovery.go |
| realm_test.go | realm.go |
| events_test.go | events.go |
| connection_test.go | connection.go |
| stream_test.go | stream.go |

---

**最后更新**：2026-01-13
