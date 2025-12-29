// Package types 定义 DeP2P 的基础类型
//
// 这是整个系统的最底层包，不依赖任何其他 dep2p 内部包。
// 所有类型都是纯值类型，用于在各模块间传递数据。
package types

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// ============================================================================
//                              NodeID - 节点标识
// ============================================================================

// NodeID 节点唯一标识符
// 由公钥派生（通常是公钥的 SHA256 哈希）
//
// 外部表示格式：
//   - String(): Base58 编码（用户可读、可分享）
//   - ShortString(): Base58 前缀（日志简短标识）
type NodeID [32]byte

// EmptyNodeID 空节点ID
var EmptyNodeID NodeID

// ErrInvalidNodeID 无效的节点ID错误
var ErrInvalidNodeID = errors.New("invalid node ID: must be Base58")

// String 返回 NodeID 的 Base58 字符串表示
//
// 这是 NodeID 的规范外部表示，用于：
//   - Bootstrap 地址中的 /p2p/<NodeID>
//   - 用户间分享节点身份
//   - 配置文件
func (id NodeID) String() string {
	if id.IsEmpty() {
		return ""
	}
	return Base58Encode(id[:])
}

// ShortString 返回 NodeID 的短字符串表示
//
// 格式：Base58 前 8 个字符，用于日志中的简短标识。
func (id NodeID) ShortString() string {
	s := id.String()
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// Bytes 返回 NodeID 的字节切片
func (id NodeID) Bytes() []byte {
	return id[:]
}

// Equal 比较两个 NodeID 是否相等
func (id NodeID) Equal(other NodeID) bool {
	return id == other
}

// IsEmpty 检查 NodeID 是否为空
func (id NodeID) IsEmpty() bool {
	return id == EmptyNodeID
}

// NodeIDFromBytes 从字节切片创建 NodeID
func NodeIDFromBytes(b []byte) (NodeID, error) {
	if len(b) != 32 {
		return EmptyNodeID, ErrInvalidNodeID
	}
	var id NodeID
	copy(id[:], b)
	return id, nil
}

// ParseNodeID 从字符串解析 NodeID
//
// 仅支持 Base58 编码（用于用户输入和配置）。
//
// 示例：
//
//	// Base58 格式
//	id, err := ParseNodeID("5Q2STWvBFn...")
func ParseNodeID(s string) (NodeID, error) {
	if s == "" {
		return EmptyNodeID, ErrInvalidNodeID
	}

	// 尝试 Base58 解码
	b, err := Base58Decode(s)
	if err != nil {
		return EmptyNodeID, ErrInvalidNodeID
	}
	if len(b) != 32 {
		return EmptyNodeID, ErrInvalidNodeID
	}

	var id NodeID
	copy(id[:], b)
	return id, nil
}

// ============================================================================
//                              StreamID - 流标识
// ============================================================================

// StreamID 流唯一标识符
type StreamID uint64

// String 返回 StreamID 的字符串表示
func (id StreamID) String() string {
	return hex.EncodeToString([]byte{
		byte(id >> 56), byte(id >> 48), byte(id >> 40), byte(id >> 32),
		byte(id >> 24), byte(id >> 16), byte(id >> 8), byte(id),
	})
}

// ============================================================================
//                              ProtocolID - 协议标识
// ============================================================================

// ProtocolID 协议标识符
// 格式: /name/version，如 /echo/1.0
type ProtocolID string

// String 返回协议ID字符串
func (p ProtocolID) String() string {
	return string(p)
}

// ============================================================================
//                              RealmID - 业务域标识
// ============================================================================

// RealmID 业务域标识符
// 用于在 P2P 网络中实现多租户隔离
type RealmID string

// String 返回 RealmID 字符串
func (r RealmID) String() string {
	return string(r)
}

// IsEmpty 检查 RealmID 是否为空
func (r RealmID) IsEmpty() bool {
	return r == ""
}

// DefaultRealmID 默认业务域（全局）
const DefaultRealmID RealmID = ""

// ============================================================================
//                              RealmKey - 业务域密钥
// ============================================================================

// RealmKey Realm 密钥（32字节高熵随机数）
//
// 用于：
//   - PSK 成员认证（持有密钥即为成员）
//   - RealmID 派生（RealmID = H(realmKey)，不可枚举）
//   - 通过带外渠道分发（邀请链接/二维码/私信等）
type RealmKey [32]byte

// EmptyRealmKey 空 RealmKey
var EmptyRealmKey RealmKey

// GenerateRealmKey 生成高熵 Realm 密钥
//
// 返回 32 字节密码学安全的随机数。
// 用于创建新 Realm 时生成密钥。
func GenerateRealmKey() RealmKey {
	var key RealmKey
	if _, err := rand.Read(key[:]); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return key
}

// IsEmpty 检查 RealmKey 是否为空
func (k RealmKey) IsEmpty() bool {
	return k == EmptyRealmKey
}

// Bytes 返回 RealmKey 的字节切片
func (k RealmKey) Bytes() []byte {
	return k[:]
}

// String 返回 RealmKey 的十六进制字符串表示（仅用于调试）
//
// 注意：生产环境中不应打印完整密钥
func (k RealmKey) String() string {
	if k.IsEmpty() {
		return ""
	}
	return hex.EncodeToString(k[:])
}

// RealmKeyFromBytes 从字节切片创建 RealmKey
func RealmKeyFromBytes(b []byte) (RealmKey, error) {
	if len(b) != 32 {
		return EmptyRealmKey, errors.New("invalid realm key: must be 32 bytes")
	}
	var key RealmKey
	copy(key[:], b)
	return key, nil
}

// RealmKeyFromHex 从十六进制字符串解析 RealmKey
func RealmKeyFromHex(s string) (RealmKey, error) {
	if len(s) != 64 {
		return EmptyRealmKey, errors.New("invalid realm key: must be 64 hex characters")
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return EmptyRealmKey, errors.New("invalid realm key: invalid hex")
	}
	return RealmKeyFromBytes(b)
}

// DeriveRealmKeyFromName 从 realm 名称派生 RealmKey（仅用于演示/测试）
//
// 注意：此函数仅适用于演示和测试场景。生产环境应使用 GenerateRealmKey()
// 生成高熵密钥，并通过安全渠道分发给成员。
//
// 公式: RealmKey = SHA256("dep2p-demo-realm-key-v1" || name)
func DeriveRealmKeyFromName(name string) RealmKey {
	h := sha256.New()
	h.Write([]byte("dep2p-demo-realm-key-v1"))
	h.Write([]byte(name))
	hash := h.Sum(nil)
	var key RealmKey
	copy(key[:], hash)
	return key
}
