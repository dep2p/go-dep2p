package types

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// ============================================================================
//                              Realm 访问级别
// ============================================================================

// AccessLevel Realm 访问级别
type AccessLevel int

const (
	// AccessLevelPublic 公开 - 任何人可加入，节点可被发现
	AccessLevelPublic AccessLevel = iota
	// AccessLevelProtected 受保护 - 需要 JoinKey 加入，节点可被同 Realm 发现
	AccessLevelProtected
	// AccessLevelPrivate 私有 - 需要 JoinKey 加入，节点不可被外部发现
	AccessLevelPrivate
)

// String 返回访问级别的字符串表示
func (a AccessLevel) String() string {
	switch a {
	case AccessLevelPublic:
		return "public"
	case AccessLevelProtected:
		return "protected"
	case AccessLevelPrivate:
		return "private"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              Realm 元数据
// ============================================================================

// RealmMetadata Realm 元数据
type RealmMetadata struct {
	// ID Realm 唯一标识
	ID RealmID

	// Name Realm 名称（人类可读）
	Name string

	// CreatorID 创建者节点 ID
	CreatorID NodeID

	// AccessLevel 访问级别
	AccessLevel AccessLevel

	// CreatedAt 创建时间
	CreatedAt time.Time

	// Description 描述
	Description string
}

// ============================================================================
//                              Realm 成员信息
// ============================================================================

// RealmMembership 节点的 Realm 成员身份
type RealmMembership struct {
	// RealmID 所属 Realm
	RealmID RealmID

	// NodeID 节点 ID
	NodeID NodeID

	// JoinedAt 加入时间
	JoinedAt time.Time

	// Role 角色（可选，用于权限控制）
	Role string
}

// ============================================================================
//                              Realm DHT Key
// ============================================================================

// RealmDHTKey 计算 Realm 感知的 DHT Key
//
// v1.1（以设计为准）：
//   - 无 Realm: Key = SHA256("dep2p/v1/sys/peer/{nodeID}")
//   - 有 Realm: Key = SHA256("dep2p/v1/realm/{realmID}/peer/{nodeID}")
//
// 这确保不同 Realm 的节点在 DHT 中隔离。
func RealmDHTKey(realmID RealmID, nodeID NodeID) []byte {
	nodeIDStr := nodeID.String()

	// 与 docs/05-iterations/2025-12-22-user-simplicity-gap-analysis.md 第 6 章一致：
	// Key = SHA256("dep2p/v1/{scope}/{type}/[{realmID}/]{payload}")
	var keyString string
	if realmID == DefaultRealmID || realmID == "" {
		keyString = "dep2p/v1/sys/peer/" + nodeIDStr
	} else {
		keyString = "dep2p/v1/realm/" + string(realmID) + "/peer/" + nodeIDStr
	}

	hash := sha256.Sum256([]byte(keyString))
	return hash[:]
}

// ============================================================================
//                              RealmID 派生
// ============================================================================

// DeriveRealmID 从 realmKey 派生 RealmID
//
// 公式: RealmID = SHA256("dep2p-realm-id-v1" || H(realmKey))
//
// 返回：完整 SHA256 哈希的十六进制字符串（64字符）
//
// 设计决策（见 DISC-1227-api-layer-design.md）：
//   - RealmID 与 Name 解耦，绑定 realmKey
//   - 不可枚举：无法从 name 推测 realmID
//   - 可升级：通过修改前缀版本号
//   - 同名不同 key → 不同 realmID（不会冲突）
func DeriveRealmID(realmKey RealmKey) RealmID {
	// 先对 realmKey 做一次哈希，确保即使 realmID 泄露也无法反推 key
	keyHash := sha256.Sum256(realmKey[:])

	// 再加上版本前缀做第二次哈希
	h := sha256.New()
	h.Write([]byte("dep2p-realm-id-v1"))
	h.Write(keyHash[:])
	hash := h.Sum(nil)

	// 返回完整 32 字节 = 64 字符 hex
	return RealmID(hex.EncodeToString(hash))
}

// Deprecated: GenerateRealmID 已废弃，请使用 DeriveRealmID
//
// 此函数基于创建者公钥和名称生成 RealmID，存在以下问题：
//   - 可枚举：name 带业务语义，hash 可被字典枚举
//   - 冲突/混淆：同名但不同 key 会被混为同一 Realm
//
// 新设计使用 DeriveRealmID(realmKey)，基于高熵密钥派生。
func GenerateRealmID(creatorPubKey []byte, realmName string) RealmID {
	h := sha256.New()
	h.Write(creatorPubKey)
	h.Write([]byte(realmName))
	hash := h.Sum(nil)
	// 使用前16字节的十六进制作为 RealmID（32字符）
	return RealmID(string(hash[:16]))
}


