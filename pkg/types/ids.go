// Package types 定义 DeP2P 的基础类型
//
// 本文件定义所有 ID 类型，是整个系统的核心标识类型。
// 这些类型是纯值类型，不依赖任何其他 dep2p 内部包。
package types

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

// ============================================================================
//                              PeerID / NodeID - 节点标识
// ============================================================================

// PeerID 节点唯一标识符
//
// PeerID 由公钥派生，确保全网唯一性和可验证性。
// 外部表示格式为 Base58 编码（用户可读、可分享）。
//
// 示例：
//
//	id, err := types.ParsePeerID("12D3KooWLYGJ...")
//	fmt.Println(id.ShortString()) // "12D3KooW"
type PeerID string

// NodeID 是 PeerID 的别名，用于 DHT 和路由场景
//
// 在 Kademlia DHT 中，节点使用 256 位 ID 空间进行路由。
// NodeID 表示 DHT 路由表中的节点标识。
type NodeID = PeerID

// EmptyPeerID 空节点ID
const EmptyPeerID PeerID = ""

// String 返回 PeerID 的字符串表示
func (id PeerID) String() string {
	return string(id)
}

// ShortString 返回 PeerID 的短字符串表示
//
// 格式：前 8 字符 + "..." + 后 3 字符，用于日志中的简短标识。
// 符合 NodeID 规范 (L1_identity/nodeid.md)。
func (id PeerID) ShortString() string {
	s := string(id)
	if len(s) <= 14 {
		return s
	}
	return s[:8] + "..." + s[len(s)-3:]
}

// Bytes 返回 PeerID 的字节切片
func (id PeerID) Bytes() []byte {
	return []byte(id)
}

// IsEmpty 检查 PeerID 是否为空
func (id PeerID) IsEmpty() bool {
	return id == EmptyPeerID
}

// Validate 验证 PeerID 格式
//
// 验证流程：
//  1. 检查是否为空
//  2. Base58 解码验证
//  3. 长度验证（支持 DeP2P 格式和 Multihash 格式）
//
// 支持的格式：
//   - DeP2P 格式: Base58(SHA256(pubKey)) - 32 字节
//   - Multihash 格式: [类型码][长度][数据] - 用于 libp2p 兼容
func (id PeerID) Validate() error {
	if id.IsEmpty() {
		return ErrEmptyPeerID
	}
	
	// Base58 解码验证
	decoded, err := Base58Decode(string(id))
	if err != nil {
		return fmt.Errorf("invalid base58: %w", err)
	}
	
	// 检查解码后的长度
	// DeP2P 使用 Base58(SHA256(pubKey))，SHA256 输出是 32 字节
	if len(decoded) == 32 {
		// DeP2P 原生格式：32 字节 SHA256 哈希
		return nil
	}
	
	// 尝试 Multihash 格式验证（用于 libp2p 兼容）
	// Multihash 格式: [类型码(1字节)][长度(1字节)][数据]
	if len(decoded) >= 2 {
		hashLen := int(decoded[1])
		if len(decoded) == 2+hashLen {
			// 有效的 Multihash 格式
			return nil
		}
	}
	
	// 既不是 DeP2P 格式，也不是有效的 Multihash 格式
	return fmt.Errorf("invalid peer id: length %d (expected 32 for SHA256 or valid multihash)", len(decoded))
}

// Equal 比较两个 PeerID 是否相等
func (id PeerID) Equal(other PeerID) bool {
	return id == other
}

// Hash 返回 PeerID 的 SHA256 哈希值（32字节）
//
// 用于 DHT 路由中的 XOR 距离计算。
func (id PeerID) Hash() [32]byte {
	return sha256.Sum256([]byte(id))
}

// XOR 计算两个 PeerID 的 XOR 距离
//
// 返回 32 字节的距离值，用于 Kademlia DHT 路由。
// 距离越小，两个节点在 DHT 空间中越接近。
func (id PeerID) XOR(other PeerID) [32]byte {
	h1 := id.Hash()
	h2 := other.Hash()
	var result [32]byte
	for i := 0; i < 32; i++ {
		result[i] = h1[i] ^ h2[i]
	}
	return result
}

// DistanceCmp 比较 id 到 a 和 b 的距离
//
// 返回值：
//   - -1: id 距离 a 更近
//   - 0: 距离相等
//   - 1: id 距离 b 更近
//
// 用于 DHT 路由表排序。
func (id PeerID) DistanceCmp(a, b PeerID) int {
	da := id.XOR(a)
	db := id.XOR(b)
	for i := 0; i < 32; i++ {
		if da[i] < db[i] {
			return -1
		}
		if da[i] > db[i] {
			return 1
		}
	}
	return 0
}

// CommonPrefixLen 计算两个 PeerID 的公共前缀位数
//
// 用于 Kademlia DHT 的 k-bucket 索引。
func (id PeerID) CommonPrefixLen(other PeerID) int {
	xorDist := id.XOR(other)
	for i := 0; i < 32; i++ {
		for j := 7; j >= 0; j-- {
			if (xorDist[i]>>j)&1 != 0 {
				return i*8 + (7 - j)
			}
		}
	}
	return 256 // 完全相同
}

// ErrPeerIDNoEmbeddedKey PeerID 不包含内嵌公钥
var ErrPeerIDNoEmbeddedKey = errors.New("peer ID does not contain embedded public key")

// ExtractPublicKey 从 PeerID 中提取内嵌的公钥
//
// 仅适用于 identity multihash 格式的 PeerID（内嵌完整公钥）。
// 对于 DeP2P 原生格式和 SHA256 派生的 PeerID，返回 ErrPeerIDNoEmbeddedKey。
//
// 支持的格式：
//   - DeP2P 原生格式: Base58(SHA256(pubKey)) - 32 字节，不含公钥
//   - Multihash identity (0x00): 内嵌完整公钥
//   - Multihash SHA256 (0x12): 仅包含哈希，不含公钥
func (id PeerID) ExtractPublicKey() ([]byte, error) {
	if id.IsEmpty() {
		return nil, ErrEmptyPeerID
	}
	
	// Base58 解码
	decoded, err := Base58Decode(string(id))
	if err != nil {
		return nil, fmt.Errorf("invalid base58: %w", err)
	}
	
	// DeP2P 原生格式：32 字节 SHA256 哈希
	// 不包含公钥，无法提取
	if len(decoded) == 32 {
		return nil, ErrPeerIDNoEmbeddedKey
	}
	
	// Multihash 格式需要至少 2 字节（类型码 + 长度）
	if len(decoded) < 2 {
		return nil, ErrInvalidPeerID
	}
	
	// 检查 multihash 类型码
	hashType := decoded[0]
	hashLen := int(decoded[1])
	
	// 仅 identity hash (0x00) 包含内嵌公钥
	if hashType == 0x00 {
		if len(decoded) < 2+hashLen {
			return nil, fmt.Errorf("invalid multihash: length mismatch")
		}
		pubKey := make([]byte, hashLen)
		copy(pubKey, decoded[2:2+hashLen])
		return pubKey, nil
	}
	
	// 其他类型（如 SHA256 0x12）不包含公钥
	return nil, ErrPeerIDNoEmbeddedKey
}

// MatchesPublicKey 验证 PeerID 是否与给定公钥匹配
//
// 对于 identity multihash，直接比较内嵌公钥。
// 对于 SHA256 multihash，重新计算 PeerID 并比较。
func (id PeerID) MatchesPublicKey(pubKey []byte) bool {
	if id.IsEmpty() || len(pubKey) == 0 {
		return false
	}
	
	// 尝试从 PeerID 提取公钥
	extractedPubKey, err := id.ExtractPublicKey()
	if err == nil {
		// identity multihash: 直接比较公钥
		if len(extractedPubKey) != len(pubKey) {
			return false
		}
		for i := 0; i < len(pubKey); i++ {
			if extractedPubKey[i] != pubKey[i] {
				return false
			}
		}
		return true
	}
	
	// SHA256 multihash: 重新计算 PeerID
	derivedID, err := PeerIDFromPublicKey(pubKey)
	if err != nil {
		return false
	}
	
	return id == derivedID
}

// ParsePeerID 从字符串解析 PeerID
//
// 支持 Base58 编码格式（用于用户输入和配置）。
func ParsePeerID(s string) (PeerID, error) {
	if s == "" {
		return EmptyPeerID, ErrEmptyPeerID
	}
	id := PeerID(s)
	if err := id.Validate(); err != nil {
		return EmptyPeerID, err
	}
	return id, nil
}

// PeerIDFromBytes 从字节切片创建 PeerID
func PeerIDFromBytes(b []byte) (PeerID, error) {
	if len(b) == 0 {
		return EmptyPeerID, ErrEmptyPeerID
	}
	return PeerID(b), nil
}

// PeerIDFromPublicKey 从公钥派生 PeerID
//
// DeP2P 派生算法：Base58(SHA256(pubKey))
// 生成 32 字节 SHA256 哈希的 Base58 编码。
// 注意：这不是 Multihash 格式，不包含内嵌公钥。
func PeerIDFromPublicKey(pubKey []byte) (PeerID, error) {
	if len(pubKey) == 0 {
		return EmptyPeerID, errors.New("empty public key")
	}
	// SHA256 哈希
	hash := sha256.Sum256(pubKey)
	// Base58 编码
	encoded := Base58Encode(hash[:])
	return PeerID(encoded), nil
}

// ============================================================================
//                              RealmID - 业务域标识
// ============================================================================

// RealmID 业务域标识符
//
// RealmID 从 PSK 派生，用于在 P2P 网络中实现多租户隔离。
//
// 派生算法（HKDF-SHA256）：
//
//	info = SHA256(PSK)
//	RealmID = hex(HKDF(sha256, PSK, salt="dep2p-realm-id-v1", info))
//
// 输出格式：64 字符十六进制字符串（32 字节 = 256 位）
type RealmID string

// EmptyRealmID 空 RealmID
const EmptyRealmID RealmID = ""

// DefaultRealmID 默认业务域（全局）
const DefaultRealmID RealmID = ""

// String 返回 RealmID 字符串
func (r RealmID) String() string {
	return string(r)
}

// IsEmpty 检查 RealmID 是否为空
func (r RealmID) IsEmpty() bool {
	return r == EmptyRealmID
}

// Bytes 返回 RealmID 的字节切片
func (r RealmID) Bytes() []byte {
	return []byte(r)
}

// realmIDSalt RealmID 派生 salt（与 internal/realm/auth/psk.go 保持一致）
const realmIDSalt = "dep2p-realm-id-v1"

// realmIDKeyLength RealmID 派生密钥长度（32 字节 = 256 位）
const realmIDKeyLength = 32

// RealmIDFromPSK 从 PSK 派生 RealmID
//
// 使用 HKDF-SHA256 派生确定性的 RealmID。
// 相同的 PSK 总是派生出相同的 RealmID。
//
// 算法：
//
//	info = SHA256(PSK)
//	RealmID = hex(HKDF(sha256, PSK, salt="dep2p-realm-id-v1", info))
//
// 返回：64 字符十六进制字符串
func RealmIDFromPSK(psk PSK) RealmID {
	if psk.IsEmpty() {
		return EmptyRealmID
	}

	// 计算 PSK 的 SHA256 作为 info（与 auth.DeriveRealmID 保持一致）
	hash := sha256.Sum256(psk)

	// 使用 HKDF 派生
	kdf := hkdf.New(sha256.New, psk, []byte(realmIDSalt), hash[:])

	// 读取 32 字节
	realmID := make([]byte, realmIDKeyLength)
	if _, err := io.ReadFull(kdf, realmID); err != nil {
		return EmptyRealmID
	}

	// 返回十六进制编码
	return RealmID(hex.EncodeToString(realmID))
}

// ============================================================================
//                              PSK - 预共享密钥
// ============================================================================

// PSK 预共享密钥
//
// PSK 是 Realm 的认证凭证，持有密钥即为成员。
// 标准长度为 32 字节（256 位）。
//
// 安全注意：PSK 不实现 String() 方法以避免意外泄露。
type PSK []byte

// PSKLength PSK 标准长度（32字节）
const PSKLength = 32

// IsEmpty 检查 PSK 是否为空
func (p PSK) IsEmpty() bool {
	return len(p) == 0
}

// Len 返回 PSK 长度
func (p PSK) Len() int {
	return len(p)
}

// Equal 比较两个 PSK 是否相等（常量时间比较）
func (p PSK) Equal(other PSK) bool {
	if len(p) != len(other) {
		return false
	}
	var result byte
	for i := 0; i < len(p); i++ {
		result |= p[i] ^ other[i]
	}
	return result == 0
}

// GeneratePSK 生成高熵 PSK
//
// 返回 32 字节密码学安全的随机数。
// 用于创建新 Realm 时生成密钥。
func GeneratePSK() PSK {
	psk := make([]byte, PSKLength)
	if _, err := rand.Read(psk); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return PSK(psk)
}

// PSKFromBytes 从字节切片创建 PSK
func PSKFromBytes(b []byte) (PSK, error) {
	if len(b) == 0 {
		return nil, ErrEmptyPSK
	}
	if len(b) != PSKLength {
		return nil, ErrInvalidPSKLength
	}
	psk := make([]byte, PSKLength)
	copy(psk, b)
	return PSK(psk), nil
}

// PSKFromHex 从十六进制字符串解析 PSK
func PSKFromHex(s string) (PSK, error) {
	if len(s) != PSKLength*2 {
		return nil, ErrInvalidPSKLength
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, errors.New("invalid PSK: invalid hex")
	}
	return PSKFromBytes(b)
}

// DeriveRealmID 从 PSK 派生 RealmID
func (p PSK) DeriveRealmID() RealmID {
	return RealmIDFromPSK(p)
}

// ============================================================================
//                              RealmKey - Realm 密钥（别名）
// ============================================================================

// RealmKey Realm 密钥（32字节高熵随机数）
//
// RealmKey 是 PSK 的别名，保持向后兼容。
// 推荐使用 PSK 类型。
type RealmKey [32]byte

// EmptyRealmKey 空 RealmKey
var EmptyRealmKey RealmKey

// IsEmpty 检查 RealmKey 是否为空
func (k RealmKey) IsEmpty() bool {
	return k == EmptyRealmKey
}

// Bytes 返回 RealmKey 的字节切片
func (k RealmKey) Bytes() []byte {
	return k[:]
}

// ToPSK 转换为 PSK 类型
func (k RealmKey) ToPSK() PSK {
	return PSK(k[:])
}

// GenerateRealmKey 生成高熵 Realm 密钥
func GenerateRealmKey() RealmKey {
	var key RealmKey
	if _, err := rand.Read(key[:]); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return key
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
func DeriveRealmKeyFromName(name string) RealmKey {
	h := sha256.New()
	h.Write([]byte("dep2p-demo-realm-key-v1"))
	h.Write([]byte(name))
	hash := h.Sum(nil)
	var key RealmKey
	copy(key[:], hash)
	return key
}

// 注：ProtocolID 类型定义在 protocol.go 中

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
//                              辅助类型
// ============================================================================

// PeerIDSlice 用于排序的 PeerID 切片
type PeerIDSlice []PeerID

func (s PeerIDSlice) Len() int           { return len(s) }
func (s PeerIDSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s PeerIDSlice) Less(i, j int) bool { return string(s[i]) < string(s[j]) }
