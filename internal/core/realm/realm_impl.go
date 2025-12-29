// Package realm 提供 Realm 管理实现
package realm

import (
	"context"
	"sync"

	messagingif "github.com/dep2p/go-dep2p/pkg/interfaces/messaging"
	realmif "github.com/dep2p/go-dep2p/pkg/interfaces/realm"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Realm 实现（IMPL-1227）
// ============================================================================

// realmImpl Realm 接口实现
//
// realmImpl 是 realmif.Realm 接口的具体实现，作为 Layer 2 产物。
// 用户通过 RealmManager.JoinRealm() 获得此对象，然后通过它访问 Layer 3 服务。
//
// 示例:
//
//	realm, err := node.JoinRealm(ctx, "my-business", dep2p.WithRealmKey(key))
//	messaging := realm.Messaging()
//	pubsub := realm.PubSub()
type realmImpl struct {
	// manager 所属的 Realm 管理器
	manager *Manager

	// name Realm 的显示名称（用户提供）
	name string

	// id Realm 的唯一标识（从 realmKey 派生）
	id types.RealmID

	// key Realm 密钥（用于 PSK 成员证明）
	key types.RealmKey

	// state Realm 内部状态（由 Manager 管理）
	state *realmState

	// psk PSK 认证器
	psk *PSKAuthenticator

	// 上下文和取消函数
	ctx    context.Context
	cancel context.CancelFunc

	// ============================
	// 底层服务依赖（IMPL-1227 Phase 4）
	// ============================

	// messagingSvc 底层消息服务（由 Manager 注入）
	messagingSvc messagingif.MessagingService

	// ============================
	// 服务实例（懒加载）
	// ============================

	messaging realmif.Messaging
	pubsub    realmif.PubSub
	discovery realmif.RealmDiscoveryService
	streams   realmif.StreamManager
	relay     realmif.RealmRelayService

	// 服务初始化锁
	mu sync.RWMutex
}

// newRealmImpl 创建 Realm 实现
func newRealmImpl(
	manager *Manager,
	name string,
	key types.RealmKey,
	state *realmState,
) *realmImpl {
	ctx, cancel := context.WithCancel(manager.ctx)

	realmID := types.DeriveRealmID(key)

	r := &realmImpl{
		manager: manager,
		name:    name,
		id:      realmID,
		key:     key,
		state:   state,
		ctx:     ctx,
		cancel:  cancel,
		// IMPL-1227 Phase 4: 从 Manager 获取底层服务
		messagingSvc: manager.messaging,
	}

	// 创建 PSK 认证器（endpoint 可能为 nil，在测试环境中）
	if manager.endpoint != nil {
		r.psk = NewPSKAuthenticator(manager.endpoint.ID(), key)
	}

	return r
}

// ============================================================================
//                              基本信息
// ============================================================================

// Name 返回 Realm 的显示名称
func (r *realmImpl) Name() string {
	return r.name
}

// ID 返回 Realm 的唯一标识
func (r *realmImpl) ID() types.RealmID {
	return r.id
}

// Key 返回 Realm 密钥
func (r *realmImpl) Key() types.RealmKey {
	return r.key
}

// ============================================================================
//                              成员管理
// ============================================================================

// Members 返回 Realm 内的所有成员节点
func (r *realmImpl) Members() []types.NodeID {
	r.manager.mu.RLock()
	defer r.manager.mu.RUnlock()

	if r.state == nil || r.state.peers == nil {
		return nil
	}

	members := make([]types.NodeID, 0, len(r.state.peers))
	for nodeID := range r.state.peers {
		members = append(members, nodeID)
	}
	return members
}

// MemberCount 返回 Realm 内的成员数量
func (r *realmImpl) MemberCount() int {
	r.manager.mu.RLock()
	defer r.manager.mu.RUnlock()

	if r.state == nil || r.state.peers == nil {
		return 0
	}

	return len(r.state.peers)
}

// IsMember 检查指定节点是否是 Realm 成员
func (r *realmImpl) IsMember(peer types.NodeID) bool {
	r.manager.mu.RLock()
	defer r.manager.mu.RUnlock()

	if r.state == nil || r.state.peers == nil {
		return false
	}

	_, ok := r.state.peers[peer]
	return ok
}

// ============================================================================
//                              Layer 3 服务入口
// ============================================================================

// Messaging 获取消息服务
func (r *realmImpl) Messaging() realmif.Messaging {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.messaging == nil {
		r.messaging = newRealmMessaging(r)
	}
	return r.messaging
}

// PubSub 获取发布订阅服务
func (r *realmImpl) PubSub() realmif.PubSub {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.pubsub == nil {
		r.pubsub = newRealmPubSub(r)
	}
	return r.pubsub
}

// Discovery 获取 Realm 内发现服务
func (r *realmImpl) Discovery() realmif.RealmDiscoveryService {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.discovery == nil {
		r.discovery = newRealmDiscovery(r)
	}
	return r.discovery
}

// Streams 获取流管理服务
func (r *realmImpl) Streams() realmif.StreamManager {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.streams == nil {
		r.streams = newRealmStreams(r)
	}
	return r.streams
}

// Relay 获取 Realm 中继服务
func (r *realmImpl) Relay() realmif.RealmRelayService {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.relay == nil {
		r.relay = newRealmRelay(r)
	}
	return r.relay
}

// ============================================================================
//                              生命周期
// ============================================================================

// Leave 离开此 Realm
func (r *realmImpl) Leave() error {
	// 取消上下文
	r.cancel()

	// 委托给 Manager 处理
	return r.manager.LeaveRealm()
}

// Context 返回 Realm 的上下文
func (r *realmImpl) Context() context.Context {
	return r.ctx
}

// ============================================================================
//                              PSK 认证（扩展）
// ============================================================================

// PSKAuth 获取 PSK 认证器
//
// 实现 realmif.AuthenticatedRealm 接口
func (r *realmImpl) PSKAuth() realmif.PSKAuthenticator {
	return r.psk
}

// ============================================================================
//                              内部方法
// ============================================================================

// close 关闭 Realm（内部使用）
func (r *realmImpl) close() {
	r.cancel()
}

