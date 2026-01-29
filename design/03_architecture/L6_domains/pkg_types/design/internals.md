# pkg_types 内部设计

> 内部结构与实现细节

---

## 内部结构

```
pkg/types/
├── nodeid.go           # NodeID 类型
├── realmid.go          # RealmID 类型
├── psk.go              # PSK 类型
├── peerinfo.go         # PeerInfo 类型
├── encoding.go         # 编码工具
└── types_test.go       # 测试
```

---

## NodeID 实现

```
package types

import (
    "bytes"
    "encoding/base58"
)

type NodeID [32]byte

func (n NodeID) String() string {
    return base58.Encode(n[:])
}

func (n NodeID) Pretty() string {
    s := n.String()
    if len(s) <= 12 {
        return s
    }
    return s[:6] + "..." + s[len(s)-6:]
}

func (n NodeID) Bytes() []byte {
    return n[:]
}

func (n NodeID) Equals(other NodeID) bool {
    return bytes.Equal(n[:], other[:])
}

func (n NodeID) MarshalText() ([]byte, error) {
    return []byte(n.String()), nil
}

func (n *NodeID) UnmarshalText(data []byte) error {
    decoded, err := base58.Decode(string(data))
    if err != nil {
        return err
    }
    if len(decoded) != 32 {
        return ErrInvalidNodeID
    }
    copy(n[:], decoded)
    return nil
}

// ParseNodeID 从字符串解析
func ParseNodeID(s string) (NodeID, error) {
    var n NodeID
    return n, n.UnmarshalText([]byte(s))
}
```

---

## RealmID 实现

```
type RealmID [32]byte

func (r RealmID) String() string {
    return base58.Encode(r[:])
}

func (r RealmID) Bytes() []byte {
    return r[:]
}

func (r RealmID) Equals(other RealmID) bool {
    return bytes.Equal(r[:], other[:])
}
```

---

## PSK 实现

```
type PSK [32]byte

func (p PSK) Bytes() []byte {
    return p[:]
}

// 故意不实现 String()，避免意外打印

// GeneratePSK 生成随机 PSK
func GeneratePSK() (PSK, error) {
    var psk PSK
    _, err := rand.Read(psk[:])
    return psk, err
}
```

---

## PeerInfo 实现

```
type PeerInfo struct {
    ID    NodeID
    Addrs []multiaddr.Multiaddr
}

func (p PeerInfo) HasAddr() bool {
    return len(p.Addrs) > 0
}

func (p PeerInfo) String() string {
    return fmt.Sprintf("{ID: %s, Addrs: %v}", p.ID.Pretty(), p.Addrs)
}
```

---

## 错误定义

```
var (
    ErrInvalidNodeID  = errors.New("types: invalid NodeID")
    ErrInvalidRealmID = errors.New("types: invalid RealmID")
    ErrInvalidPSK     = errors.New("types: invalid PSK")
)
```

---

**最后更新**：2026-01-11
