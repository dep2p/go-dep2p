// Package dht 提供 DHT（分布式哈希表）的核心实现
package dht

import (
	"crypto/sha256"
	"fmt"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              DHT Keyspace 常量
// ============================================================================

// Key 前缀与 Scope 常量
//
// 设计规范: dep2p/v1/{scope}/{type}/[{realmID}/]{payload}
// 参考: docs/01-design/protocols/network/01-discovery.md
const (
	// KeyPrefix 版本前缀
	KeyPrefix = "dep2p/v1"

	// ScopeSys 系统层 Scope（全局共享）
	ScopeSys = "sys"

	// ScopeRealm 业务层 Scope（Realm 隔离）
	ScopeRealm = "realm"
)

// Key 类型常量
const (
	// TypePeer 节点地址记录
	TypePeer = "peer"

	// TypeService 服务发现记录
	TypeService = "service"

	// TypeBootstrap 引导节点记录
	TypeBootstrap = "bootstrap"

	// TypeRelay 中继节点记录
	TypeRelay = "relay"

	// TypeNAT NAT 映射记录
	TypeNAT = "nat"

	// TypeRendezvous Rendezvous 记录
	TypeRendezvous = "rendezvous"
)

// ============================================================================
//                              Key 构造函数
// ============================================================================

// BuildKeyString 构造符合设计规范的 Key 字符串
//
// 格式:
//   - 系统层: dep2p/v1/sys/{type}/{payload}
//   - 业务层: dep2p/v1/realm/{realmID}/{type}/{payload}
//
// 示例:
//
//	BuildKeyString(ScopeSys, TypeBootstrap, "", "QmXxx...")
//	  => "dep2p/v1/sys/bootstrap/QmXxx..."
//
//	BuildKeyString(ScopeRealm, TypePeer, "my-blockchain", "QmYyy...")
//	  => "dep2p/v1/realm/my-blockchain/peer/QmYyy..."
func BuildKeyString(scope, keyType string, realmID types.RealmID, payload string) string {
	if scope == ScopeSys {
		return fmt.Sprintf("%s/%s/%s/%s", KeyPrefix, ScopeSys, keyType, payload)
	}
	return fmt.Sprintf("%s/%s/%s/%s/%s", KeyPrefix, ScopeRealm, string(realmID), keyType, payload)
}

// BuildKey 构造 DHT Key（SHA256 哈希）
//
// Key = SHA256(KeyString)
func BuildKey(scope, keyType string, realmID types.RealmID, payload string) []byte {
	keyString := BuildKeyString(scope, keyType, realmID, payload)
	hash := sha256.Sum256([]byte(keyString))
	return hash[:]
}

// SystemKey 构造系统层 Key
//
// 示例:
//
//	SystemKey(TypeBootstrap, nodeID.String())
//	  => SHA256("dep2p/v1/sys/bootstrap/{nodeID}")
func SystemKey(keyType string, payload string) []byte {
	return BuildKey(ScopeSys, keyType, "", payload)
}

// RealmKey 构造业务层 Key
//
// 示例:
//
//	RealmKey(TypePeer, "my-blockchain", nodeID.String())
//	  => SHA256("dep2p/v1/realm/my-blockchain/peer/{nodeID}")
func RealmKey(keyType string, realmID types.RealmID, payload string) []byte {
	return BuildKey(ScopeRealm, keyType, realmID, payload)
}

// ============================================================================
//                              Realm 感知 DHT Key（兼容旧 API）
// ============================================================================

// RealmAwareDHTKey 计算 Realm 感知的 DHT Key
//
// v1.1 设计规范:
//   - 无 Realm: Key = SHA256("dep2p/v1/sys/peer/{nodeID}")
//   - 有 Realm: Key = SHA256("dep2p/v1/realm/{realmID}/peer/{nodeID}")
//
// 这确保不同 Realm 的节点在 DHT 中被隔离，
// 同一节点在不同 Realm 中有不同的 Key。
//
// 示例：
//
//	// 无 Realm（系统层）
//	key := RealmAwareDHTKey(types.DefaultRealmID, nodeID)
//
//	// 有 Realm（业务层）
//	key := RealmAwareDHTKey("my-blockchain", nodeID)
func RealmAwareDHTKey(realmID types.RealmID, nodeID types.NodeID) []byte {
	nodeIDStr := nodeID.String()

	// 无 Realm: 系统层 Key
	if realmID == types.DefaultRealmID || realmID == "" {
		return SystemKey(TypePeer, nodeIDStr)
	}

	// 有 Realm: 业务层 Key
	return RealmKey(TypePeer, realmID, nodeIDStr)
}

// RealmAwareValueKey 计算 Realm 感知的值存储 Key
//
// v1.1 设计规范:
//   - 无 Realm: Key = SHA256("dep2p/v1/sys/{keyType}/{keyData}")
//   - 有 Realm: Key = SHA256("dep2p/v1/realm/{realmID}/{keyType}/{keyData}")
//
// 用于在 DHT 中存储 Realm 相关的值，如服务发现记录。
//
// 示例：
//
//	// 服务发现
//	key := RealmAwareValueKey("my-blockchain", "service", []byte("chat"))
func RealmAwareValueKey(realmID types.RealmID, keyType string, keyData []byte) []byte {
	payload := string(keyData)

	// 无 Realm: 系统层 Key
	if realmID == types.DefaultRealmID || realmID == "" {
		return SystemKey(keyType, payload)
	}

	// 有 Realm: 业务层 Key
	return RealmKey(keyType, realmID, payload)
}

// ============================================================================
//                              Key 哈希工具
// ============================================================================

// HashKey 对原始 key 字符串进行 SHA256 哈希
//
// T2 修复：所有 DHT 存储和距离计算都应使用哈希后的 key。
// 网络消息仍传输原始 key（对端自行哈希，保持协议一致性）。
//
// 返回 32 字节的 SHA256 哈希值。
func HashKey(key string) []byte {
	hash := sha256.Sum256([]byte(key))
	return hash[:]
}

// HashKeyString 对原始 key 字符串进行 SHA256 哈希，返回 hex 字符串
//
// 用于 map[string] 存储时作为 key。
func HashKeyString(key string) string {
	hash := sha256.Sum256([]byte(key))
	return fmt.Sprintf("%x", hash)
}

// ============================================================================
//                              Kademlia 距离计算
// ============================================================================

// XORDistance 计算两个 Key 的 XOR 距离
//
// Kademlia 使用 XOR 距离度量来确定节点之间的"距离"。
// 返回值是一个 32 字节的数组，表示 XOR 结果。
func XORDistance(a, b []byte) []byte {
	// 确保长度相同
	length := len(a)
	if len(b) < length {
		length = len(b)
	}

	result := make([]byte, length)
	for i := 0; i < length; i++ {
		result[i] = a[i] ^ b[i]
	}

	return result
}

// CommonPrefixLength 计算两个 Key 的公共前缀位数
//
// 这用于确定节点应该放入哪个 K-Bucket。
// 公共前缀越长，节点越"近"。
func CommonPrefixLength(a, b []byte) int {
	distance := XORDistance(a, b)
	return LeadingZeros(distance)
}

// LeadingZeros 计算字节数组的前导零位数
func LeadingZeros(data []byte) int {
	zeros := 0
	for _, b := range data {
		if b == 0 {
			zeros += 8
		} else {
			// 计算这个字节的前导零
			for i := 7; i >= 0; i-- {
				if (b>>i)&1 == 0 {
					zeros++
				} else {
					return zeros
				}
			}
		}
	}
	return zeros
}

// BucketIndex 计算节点应该放入的 K-Bucket 索引
//
// 索引 = 256 - CommonPrefixLength(self, target)
// 索引越小，节点越近。
func BucketIndex(selfKey, targetKey []byte) int {
	cpl := CommonPrefixLength(selfKey, targetKey)
	// 假设使用 256 位 Key
	return 256 - cpl
}

// ============================================================================
//                              Realm 隔离检查
// ============================================================================

// SameRealm 检查两个 Realm 是否相同
//
// 考虑空 Realm（全局 Realm）的情况。
func SameRealm(a, b types.RealmID) bool {
	// 空 Realm 被视为全局 Realm
	aIsGlobal := a == types.DefaultRealmID || a == ""
	bIsGlobal := b == types.DefaultRealmID || b == ""

	// 两个都是全局 Realm
	if aIsGlobal && bIsGlobal {
		return true
	}

	// 一个是全局，一个不是
	if aIsGlobal != bIsGlobal {
		return false
	}

	// 两个都不是全局，比较 ID
	return a == b
}

// RealmFilter 创建一个 Realm 过滤器
//
// 返回一个函数，用于检查节点是否属于指定 Realm。
type RealmFilter func(nodeRealmID types.RealmID) bool

// NewRealmFilter 创建 Realm 过滤器
//
// 如果 targetRealm 是全局 Realm，则接受所有节点。
// 否则只接受同 Realm 的节点。
func NewRealmFilter(targetRealm types.RealmID) RealmFilter {
	isGlobal := targetRealm == types.DefaultRealmID || targetRealm == ""

	return func(nodeRealmID types.RealmID) bool {
		if isGlobal {
			return true // 全局 Realm 接受所有节点
		}
		return SameRealm(targetRealm, nodeRealmID)
	}
}

// ============================================================================
//                              DHT Key 工具
// ============================================================================

// NodeIDToKey 将 NodeID 转换为 DHT Key（32 字节）
func NodeIDToKey(nodeID types.NodeID) []byte {
	return nodeID[:]
}

// KeyToNodeID 将 DHT Key 转换为 NodeID
//
// 注意：这假设 Key 是直接从 NodeID 派生的，
// 对于 Realm 感知的 Key，无法还原 NodeID。
func KeyToNodeID(key []byte) (types.NodeID, bool) {
	if len(key) != 32 {
		return types.EmptyNodeID, false
	}
	var nodeID types.NodeID
	copy(nodeID[:], key)
	return nodeID, true
}

// IsCloser 判断 a 是否比 b 更接近 target
func IsCloser(a, b, target []byte) bool {
	distA := XORDistance(a, target)
	distB := XORDistance(b, target)

	// 按字节比较距离
	for i := 0; i < len(distA) && i < len(distB); i++ {
		if distA[i] < distB[i] {
			return true
		}
		if distA[i] > distB[i] {
			return false
		}
	}
	return false
}
