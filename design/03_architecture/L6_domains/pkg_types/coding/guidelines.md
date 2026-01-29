# pkg_types 编码指南

> 代码规范与最佳实践

---

## 代码组织

```
pkg/types/
├── ids.go              # PeerID, RealmID, PSK, RealmKey, StreamID
├── enums.go            # Direction, Connectedness, NATType...
├── protocol.go         # ProtocolID, 协议常量
├── base58.go           # Base58 编解码
├── multiaddr.go        # Multiaddr
├── errors.go           # 公共错误
├── discovery.go        # PeerInfo, AddrInfo
├── connection.go       # ConnInfo, ConnStat
├── stream.go           # StreamInfo, StreamStat
├── realm.go            # RealmInfo, RealmConfig
├── events.go           # 事件类型
└── *_test.go           # 测试
```

---

## 类型使用规范

### PeerID 使用

```go
import "github.com/dep2p/dep2p/pkg/types"

// 解析
id, err := types.ParsePeerID("12D3KooW...")
if err != nil {
    return err
}

// 比较
if id.Equal(other) {
    // ...
}

// 作为 map key
peers := make(map[types.PeerID]types.PeerInfo)

// 日志打印（使用短格式）
slog.Info("peer connected", "id", id.ShortString())

// 验证
if err := id.Validate(); err != nil {
    return err
}
```

### PSK 使用

```go
// 生成 PSK
psk := types.GeneratePSK()

// 从 Hex 解析
psk, err := types.PSKFromHex("0123456789abcdef...")

// 派生 RealmID
realmID := psk.DeriveRealmID()

// 安全比较（常量时间）
if psk.Equal(other) {
    // ...
}

// ⚠️ 不要打印！
// BAD: fmt.Println(psk)
// BAD: slog.Info("psk", "value", psk)
```

### RealmKey 使用

```go
// 生成
key := types.GenerateRealmKey()

// 从演示名称派生（仅用于测试）
demoKey := types.DeriveRealmKeyFromName("demo-realm")

// 转换为 PSK
psk := key.ToPSK()
realmID := types.RealmIDFromPSK(psk)
```

### ProtocolID 使用

```go
// 使用系统协议常量
proto := types.ProtocolPing  // "/dep2p/sys/ping/1.0.0"

// 构建 Realm 协议
realmProto := types.BuildRealmProtocolID("realm123", "chat", "1.0.0")
// -> "/dep2p/realm/realm123/chat/1.0.0"

// 构建应用协议
appProto := types.BuildAppProtocolID("realm123", "custom", "1.0.0")
// -> "/dep2p/app/realm123/custom/1.0.0"

// 检查协议类型
if proto.IsSystem() {
    // 系统协议
}
if proto.IsRealm() {
    // Realm 协议
    realmID := proto.RealmID()
}
```

### Multiaddr 使用

```go
// 解析
ma, err := types.ParseMultiaddr("/ip4/127.0.0.1/tcp/4001")

// 提取值
ip, err := ma.ValueForProtocolName("ip4")
port, err := ma.ValueForProtocolName("tcp")

// 封装/解封装
full, _ := ma.Encapsulate(types.P2PMultiaddr(peerID))
transport := full.Decapsulate(types.P2PMultiaddr(peerID))

// 分离 P2P 组件
transport, peerID := types.SplitP2P(ma)
```

### PeerInfo/AddrInfo 使用

```go
// 创建 PeerInfo
info := types.NewPeerInfo(peerID, addrs)
info := types.NewPeerInfoWithSource(peerID, addrs, types.SourceDHT)

// 检查
if info.HasAddrs() {
    // ...
}

// 检查过期
if info.IsExpired(time.Hour) {
    // 需要刷新
}

// 转换
ai := types.NewAddrInfo(peerID, addrs)
pi := ai.ToPeerInfo()
```

---

## 类型扩展规范

### 添加新方法

```go
// 添加新方法时保持不可变性
func (id PeerID) XOR(other PeerID) (PeerID, error) {
    // 返回新值，不修改原值
    result := ...
    return result, nil
}
```

### 添加新类型

```go
// 新类型应遵循相同模式
type NewID string  // 或 [32]byte

func (t NewID) String() string { ... }
func (t NewID) IsEmpty() bool { ... }
func (t NewID) Equal(other NewID) bool { ... }

// 如果是敏感类型，不实现 String()
type SensitiveType []byte
func (t SensitiveType) IsEmpty() bool { ... }
// 不实现 String()
```

---

## 序列化规范

### JSON 序列化

```go
type Config struct {
    PeerID types.PeerID `json:"peer_id"`
    Addrs  []string     `json:"addrs"`
}

// PeerID 自动序列化为字符串
```

### 二进制序列化

```go
// 使用 Bytes() 进行二进制操作
buf := make([]byte, len(peerID.Bytes()))
copy(buf, peerID.Bytes())
```

---

## 错误处理规范

```go
// 使用预定义错误
if peerID.IsEmpty() {
    return types.ErrEmptyPeerID
}

// 检查错误类型
if errors.Is(err, types.ErrInvalidPeerID) {
    // ...
}

// 常用错误
// types.ErrEmptyPeerID
// types.ErrInvalidPeerID
// types.ErrEmptyPSK
// types.ErrInvalidPSKLength
// types.ErrNotConnected
// types.ErrPeerNotFound
```

---

## 枚举使用规范

```go
// 使用枚举常量
dir := types.DirInbound
if dir == types.DirOutbound {
    // ...
}

// 使用 String() 方法获取可读字符串
fmt.Println(dir.String())  // "inbound"

// 所有枚举都有 String() 方法
types.Connected.String()         // "connected"
types.NATTypeSymmetric.String()  // "symmetric"
types.KeyTypeEd25519.String()    // "Ed25519"
```

---

## 测试规范

```go
func TestPeerID_Equal(t *testing.T) {
    id1, _ := types.ParsePeerID("12D3KooWTest1")
    id2, _ := types.ParsePeerID("12D3KooWTest1")
    id3, _ := types.ParsePeerID("12D3KooWTest2")
    
    if !id1.Equal(id2) {
        t.Error("Equal() = false for same IDs")
    }
    if id1.Equal(id3) {
        t.Error("Equal() = true for different IDs")
    }
}

func TestPSK_Security(t *testing.T) {
    psk := types.GeneratePSK()
    
    // 验证长度
    if psk.Len() != types.PSKLength {
        t.Errorf("Len() = %d, want %d", psk.Len(), types.PSKLength)
    }
    
    // 验证 String() 未实现（通过编译时检查）
}
```

---

**最后更新**：2026-01-13
