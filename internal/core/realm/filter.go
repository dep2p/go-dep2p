// Package realm 提供 Realm 管理实现
package realm

import (
	"context"
	"sync"

	realmif "github.com/dep2p/go-dep2p/pkg/interfaces/realm"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              RealmFilter 节点过滤器
// ============================================================================

// RealmFilter Realm 感知的节点过滤器
//
// 用于过滤只属于特定 Realm 的节点
type RealmFilter struct {
	manager *Manager

	// 缓存：已验证的 Realm 成员
	memberCache map[types.RealmID]map[types.NodeID]struct{}
	cacheMu     sync.RWMutex
}

// NewRealmFilter 创建 Realm 过滤器
func NewRealmFilter(manager *Manager) *RealmFilter {
	return &RealmFilter{
		manager:     manager,
		memberCache: make(map[types.RealmID]map[types.NodeID]struct{}),
	}
}

// FilterByRealm 按 Realm 过滤节点
//
// 返回属于指定 Realm 的节点
func (f *RealmFilter) FilterByRealm(nodes []types.NodeID, realmID types.RealmID) []types.NodeID {
	if f.manager == nil {
		return nodes
	}

	// 获取 Realm 成员
	members := f.manager.RealmPeers(realmID)
	memberSet := make(map[types.NodeID]struct{}, len(members))
	for _, m := range members {
		memberSet[m] = struct{}{}
	}

	// 过滤
	filtered := make([]types.NodeID, 0, len(nodes))
	for _, node := range nodes {
		if _, ok := memberSet[node]; ok {
			filtered = append(filtered, node)
		}
	}

	log.Debug("按 Realm 过滤节点",
		"realm", string(realmID),
		"input", len(nodes),
		"output", len(filtered))

	return filtered
}

// FilterOutRealm 排除 Realm 节点
//
// 返回不属于指定 Realm 的节点
func (f *RealmFilter) FilterOutRealm(nodes []types.NodeID, realmID types.RealmID) []types.NodeID {
	if f.manager == nil {
		return nodes
	}

	// 获取 Realm 成员
	members := f.manager.RealmPeers(realmID)
	memberSet := make(map[types.NodeID]struct{}, len(members))
	for _, m := range members {
		memberSet[m] = struct{}{}
	}

	// 过滤
	filtered := make([]types.NodeID, 0, len(nodes))
	for _, node := range nodes {
		if _, ok := memberSet[node]; !ok {
			filtered = append(filtered, node)
		}
	}

	return filtered
}

// IsMemberOf 检查节点是否是 Realm 成员
func (f *RealmFilter) IsMemberOf(nodeID types.NodeID, realmID types.RealmID) bool {
	if f.manager == nil {
		return false
	}

	// 先检查缓存
	f.cacheMu.RLock()
	if members, ok := f.memberCache[realmID]; ok {
		_, isMember := members[nodeID]
		f.cacheMu.RUnlock()
		return isMember
	}
	f.cacheMu.RUnlock()

	// 从管理器获取
	members := f.manager.RealmPeers(realmID)
	for _, m := range members {
		if m == nodeID {
			f.updateCache(realmID, nodeID)
			return true
		}
	}

	return false
}

// updateCache 更新缓存
func (f *RealmFilter) updateCache(realmID types.RealmID, nodeID types.NodeID) {
	f.cacheMu.Lock()
	defer f.cacheMu.Unlock()

	if _, ok := f.memberCache[realmID]; !ok {
		f.memberCache[realmID] = make(map[types.NodeID]struct{})
	}
	f.memberCache[realmID][nodeID] = struct{}{}
}

// ClearCache 清除缓存
func (f *RealmFilter) ClearCache() {
	f.cacheMu.Lock()
	f.memberCache = make(map[types.RealmID]map[types.NodeID]struct{})
	f.cacheMu.Unlock()
}

// ============================================================================
//                              RealmDiscovery 实现
// ============================================================================

// RealmDiscoveryWrapper Realm 感知的发现服务包装器
type RealmDiscoveryWrapper struct {
	manager *Manager
	filter  *RealmFilter
}

// NewRealmDiscoveryWrapper 创建 Realm 发现包装器
func NewRealmDiscoveryWrapper(manager *Manager) *RealmDiscoveryWrapper {
	return &RealmDiscoveryWrapper{
		manager: manager,
		filter:  NewRealmFilter(manager),
	}
}

// FindRealmPeers 在指定 Realm 内发现节点
func (d *RealmDiscoveryWrapper) FindRealmPeers(ctx context.Context, realmID types.RealmID, limit int) (<-chan types.NodeID, error) {
	ch := make(chan types.NodeID, limit)

	go func() {
		defer close(ch)

		if d.manager == nil {
			return
		}

		// 获取 Realm 成员
		peers := d.manager.RealmPeers(realmID)

		count := 0
		for _, peer := range peers {
			if count >= limit {
				break
			}

			select {
			case <-ctx.Done():
				return
			case ch <- peer:
				count++
			}
		}

		log.Debug("发现 Realm 节点",
			"realm", string(realmID),
			"count", count)
	}()

	return ch, nil
}

// AnnounceToRealm 在指定 Realm 内通告自己
func (d *RealmDiscoveryWrapper) AnnounceToRealm(ctx context.Context, realmID types.RealmID) error {
	if d.manager == nil {
		return ErrNotMember
	}

	// 检查是否是成员
	// v1.1: 使用 IsMemberOf 检查特定 Realm
	if !d.manager.IsMemberOf(realmID) {
		return ErrNotMember
	}

	// 通过 DHT 通告
	d.manager.registerWithDHT(ctx, realmID)

	log.Debug("通告到 Realm",
		"realm", string(realmID))

	return nil
}

// 确保实现接口
var _ realmif.RealmDiscovery = (*RealmDiscoveryWrapper)(nil)

// ============================================================================
//                              DHT 命名空间隔离
// ============================================================================

// RealmNamespace 生成 Realm 命名空间
//
// 格式: realm/{realmID}
func RealmNamespace(realmID types.RealmID) string {
	return "realm/" + string(realmID)
}

// RealmTopicNamespace 生成 Realm 主题命名空间
//
// 格式: realm/{realmID}/topic/{topic}
func RealmTopicNamespace(realmID types.RealmID, topic string) string {
	return "realm/" + string(realmID) + "/topic/" + topic
}

// ParseRealmFromNamespace 从命名空间解析 RealmID
func ParseRealmFromNamespace(namespace string) (types.RealmID, bool) {
	const prefix = "realm/"
	if len(namespace) <= len(prefix) {
		return "", false
	}

	if namespace[:len(prefix)] != prefix {
		return "", false
	}

	// 查找下一个 '/'
	rest := namespace[len(prefix):]
	for i, c := range rest {
		if c == '/' {
			return types.RealmID(rest[:i]), true
		}
	}

	return types.RealmID(rest), true
}

// ============================================================================
//                              节点发现过滤回调
// ============================================================================

// DiscoveryFilterFunc 发现过滤函数类型
type DiscoveryFilterFunc func(nodeID types.NodeID) bool

// CreateRealmDiscoveryFilter 创建 Realm 发现过滤器
//
// 只允许发现同 Realm 的节点
func CreateRealmDiscoveryFilter(manager *Manager, realmID types.RealmID) DiscoveryFilterFunc {
	return func(nodeID types.NodeID) bool {
		if manager == nil {
			return true
		}

		// 获取 Realm 成员
		members := manager.RealmPeers(realmID)
		for _, m := range members {
			if m == nodeID {
				return true
			}
		}

		return false
	}
}

// CreateMultiRealmDiscoveryFilter 创建多 Realm 发现过滤器
//
// 只允许发现任一指定 Realm 的节点
func CreateMultiRealmDiscoveryFilter(manager *Manager, realmIDs []types.RealmID) DiscoveryFilterFunc {
	return func(nodeID types.NodeID) bool {
		if manager == nil {
			return true
		}

		for _, realmID := range realmIDs {
			members := manager.RealmPeers(realmID)
			for _, m := range members {
				if m == nodeID {
					return true
				}
			}
		}

		return false
	}
}

// CreatePrivateRealmDiscoveryFilter 创建私有 Realm 发现过滤器
//
// 完全阻止外部发现
func CreatePrivateRealmDiscoveryFilter(manager *Manager, realmID types.RealmID, accessController *AccessController) DiscoveryFilterFunc {
	return func(nodeID types.NodeID) bool {
		if manager == nil || accessController == nil {
			return true
		}

		// 检查访问级别
		access := accessController.GetAccess(realmID)
		if access == types.AccessLevelPrivate {
			// 私有 Realm，只允许成员发现
			members := manager.RealmPeers(realmID)
			for _, m := range members {
				if m == nodeID {
					return true
				}
			}
			return false
		}

		// 其他级别按正常逻辑
		return true
	}
}

