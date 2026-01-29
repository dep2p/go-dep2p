package dht

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Key 哈希
// ============================================================================

// HashKey 计算 Key 的 SHA256 哈希
func HashKey(key string) []byte {
	h := sha256.Sum256([]byte(key))
	return h[:]
}

// HashNodeID 计算 NodeID 的 SHA256 哈希
func HashNodeID(nodeID types.NodeID) []byte {
	h := sha256.Sum256([]byte(nodeID))
	return h[:]
}

// HashRealmID 计算 RealmID 的 SHA256 哈希
//
// 用于 DHT Key 构建，确保 Realm 隔离和 Key 均匀分布。
func HashRealmID(realmID types.RealmID) []byte {
	h := sha256.Sum256([]byte(realmID))
	return h[:]
}

// ============================================================================
//                              Key 构建
// ============================================================================

const (
	// KeyPrefix Key 前缀
	KeyPrefix = "/dep2p"

	// KeyVersion Key 版本（v2.0 重构）
	KeyVersion = "v2"

	// ScopeSys 系统域
	ScopeSys = "sys"

	// ScopeRealm 业务域
	ScopeRealm = "realm"

	// ScopeNode 全局节点域（Phase A 使用）
	ScopeNode = "node"

	// KeyTypePeer 节点记录类型
	KeyTypePeer = "peer"

	// KeyTypeValue 值存储类型
	KeyTypeValue = "value"

	// KeyTypeProvider Provider 类型
	KeyTypeProvider = "provider"

	// KeyTypeMembers 成员列表类型（v2.0 新增，用于 Provider Record）
	KeyTypeMembers = "members"
)

// SystemKey 构造系统域 Key
//
// 格式: /dep2p/v1/sys/{keyType}/{payload}
func SystemKey(keyType string, payload []byte) string {
	return fmt.Sprintf("%s/%s/%s/%s/%s", KeyPrefix, KeyVersion, ScopeSys, keyType, string(payload))
}

// GlobalPeerKey 构造全局节点记录 Key
//
// 格式: /dep2p/v2/node/<NodeID>
//
// 用于 Phase A 阶段（冷启动、未加入 Realm）的全局节点发布。
// 与 RealmPeerKey 的区别：
//   - GlobalPeerKey: 不依赖 RealmID，用于全局 DHT 发现
//   - RealmPeerKey: 依赖 RealmID，用于 Realm 内部发现
//
// 设计说明：
//   - v2.0 重构：使用 /dep2p/v2/node/<NodeID> 格式
//   - NodeID 保持原始格式，便于调试和日志追踪
func GlobalPeerKey(nodeID types.NodeID) string {
	return fmt.Sprintf("%s/%s/%s/%s", KeyPrefix, KeyVersion, ScopeNode, string(nodeID))
}

// ParseGlobalPeerKey 解析全局节点 Key
//
// 返回 NodeID，如果格式不正确返回错误
func ParseGlobalPeerKey(key string) (types.NodeID, error) {
	// 期望格式: /dep2p/v2/node/{NodeID}
	parts := strings.Split(key, "/")

	// 需要 5 部分: ["", "dep2p", "v2", "node", NodeID]
	if len(parts) < 5 {
		return "", errors.New("invalid global peer key: too few parts")
	}

	if parts[1] != "dep2p" {
		return "", errors.New("invalid global peer key: wrong prefix")
	}

	if parts[2] != KeyVersion {
		return "", errors.New("invalid global peer key: wrong version")
	}

	if parts[3] != ScopeNode {
		return "", errors.New("invalid global peer key: not a node key")
	}

	// NodeID 可能包含特殊字符，重新组合
	nodeID := strings.Join(parts[4:], "/")
	if nodeID == "" {
		return "", errors.New("invalid global peer key: empty NodeID")
	}

	return types.NodeID(nodeID), nil
}

// IsGlobalPeerKey 检查是否为全局节点 Key
func IsGlobalPeerKey(key string) bool {
	return strings.HasPrefix(key, KeyPrefix+"/"+KeyVersion+"/"+ScopeNode+"/")
}

// RealmPeerKey 构造 Realm 节点记录 Key
//
// 格式: /dep2p/v2/realm/{H(RealmID)}/peer/{NodeID}
// 其中 H() = SHA-256 哈希的十六进制编码
//
// 设计说明：
//   - v2.0 重构：使用 /dep2p/v2/realm/... 格式
//   - RealmID 使用哈希：确保 Key 空间均匀分布，避免热点
//   - NodeID 保持原始：便于调试和日志追踪
//   - 这是 DHT 权威目录的 Key 格式
func RealmPeerKey(realmID types.RealmID, nodeID types.NodeID) string {
	realmHash := HashRealmID(realmID)
	return fmt.Sprintf("%s/%s/%s/%s/%s/%s",
		KeyPrefix,
		KeyVersion,
		ScopeRealm,
		hex.EncodeToString(realmHash),
		KeyTypePeer,
		string(nodeID))
}

// RealmMembersKey 构造 Realm 成员列表 Key（v2.0 新增）
//
// 格式: /dep2p/v2/realm/{H(RealmID)}/members
// 用于 Provider Record 机制，实现"先发布后发现"模式。
//
// 设计说明：
//   - 每个 Realm 成员调用 Provide(RealmMembersKey) 声明自己是成员
//   - 其他节点调用 FindProviders(RealmMembersKey) 发现 Realm 成员
//   - 无需入口节点，打破 Realm 冷启动死锁
func RealmMembersKey(realmID types.RealmID) string {
	realmHash := HashRealmID(realmID)
	return fmt.Sprintf("%s/%s/%s/%s/%s",
		KeyPrefix,
		KeyVersion,
		ScopeRealm,
		hex.EncodeToString(realmHash),
		KeyTypeMembers)
}

// RealmValueKey 构造 Realm 值存储 Key
//
// 格式: /dep2p/v2/realm/{H(RealmID)}/value/{key}
func RealmValueKey(realmID types.RealmID, key string) string {
	realmHash := HashRealmID(realmID)
	return fmt.Sprintf("%s/%s/%s/%s/%s/%s",
		KeyPrefix,
		KeyVersion,
		ScopeRealm,
		hex.EncodeToString(realmHash),
		KeyTypeValue,
		key)
}

// RealmProviderKey 构造 Realm Provider Key
//
// 格式: /dep2p/v2/realm/{H(RealmID)}/provider/{key}
func RealmProviderKey(realmID types.RealmID, key string) string {
	realmHash := HashRealmID(realmID)
	return fmt.Sprintf("%s/%s/%s/%s/%s/%s",
		KeyPrefix,
		KeyVersion,
		ScopeRealm,
		hex.EncodeToString(realmHash),
		KeyTypeProvider,
		key)
}

// ============================================================================
//                              Key 解析
// ============================================================================

// ParsedRealmKey 解析后的 Realm Key
type ParsedRealmKey struct {
	// RealmHash RealmID 的 SHA256 哈希（十六进制）
	RealmHash string

	// KeyType Key 类型（peer/value/provider）
	KeyType string

	// Payload Key 载荷（NodeID/key）
	Payload string
}

// ParseRealmKey 解析 Realm Key
//
// 支持格式: /dep2p/v2/realm/{H(RealmID)}/{keyType}/{payload}
// 对于 members 类型，payload 为空
func ParseRealmKey(key string) (*ParsedRealmKey, error) {
	// 期望格式: /dep2p/v2/realm/{hash}/{type}/{payload}
	parts := strings.Split(key, "/")

	// 至少需要 6 部分: ["", "dep2p", "v2", "realm", hash, type...]
	if len(parts) < 6 {
		return nil, errors.New("invalid realm key: too few parts")
	}

	if parts[1] != "dep2p" {
		return nil, errors.New("invalid realm key: wrong prefix")
	}

	if parts[2] != KeyVersion {
		return nil, errors.New("invalid realm key: wrong version")
	}

	if parts[3] != ScopeRealm {
		return nil, errors.New("invalid realm key: not a realm key")
	}

	realmHash := parts[4]
	if len(realmHash) != 64 { // SHA256 hex = 64 chars
		return nil, errors.New("invalid realm key: invalid realm hash length")
	}

	keyType := parts[5]
	if keyType != KeyTypePeer && keyType != KeyTypeValue && keyType != KeyTypeProvider && keyType != KeyTypeMembers {
		return nil, fmt.Errorf("invalid realm key: unknown key type %s", keyType)
	}

	// payload 可能包含 "/"，所以需要重新组合
	// 对于 members 类型，payload 为空
	var payload string
	if len(parts) > 6 {
		payload = strings.Join(parts[6:], "/")
	}

	return &ParsedRealmKey{
		RealmHash: realmHash,
		KeyType:   keyType,
		Payload:   payload,
	}, nil
}

// ValidateRealmPeerKey 验证 Realm Peer Key 格式
//
// 检查：
//  1. Key 格式正确
//  2. RealmID 哈希匹配
//  3. NodeID 匹配
func ValidateRealmPeerKey(key string, realmID types.RealmID, nodeID types.NodeID) error {
	parsed, err := ParseRealmKey(key)
	if err != nil {
		return err
	}

	if parsed.KeyType != KeyTypePeer {
		return errors.New("invalid key type: expected peer")
	}

	// 验证 RealmID 哈希
	expectedHash := hex.EncodeToString(HashRealmID(realmID))
	if parsed.RealmHash != expectedHash {
		return ErrRealmIDMismatch
	}

	// 验证 NodeID
	if parsed.Payload != string(nodeID) {
		return ErrNodeIDMismatch
	}

	return nil
}

// ============================================================================
//                              兼容性函数（已废弃）
// ============================================================================

// RealmKey 构造业务域 Key（已废弃，保留向后兼容）
//
// Deprecated: 使用 RealmPeerKey、RealmValueKey 或 RealmProviderKey
func RealmKey(realmID types.RealmID, _ string, payload []byte) string {
	return RealmValueKey(realmID, string(payload))
}
