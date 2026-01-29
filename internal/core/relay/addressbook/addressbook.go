package addressbook

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	realmif "github.com/dep2p/go-dep2p/internal/realm/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// MemberAddressBook Realm 成员地址簿（聚合根）
//
// 封装地址簿的业务逻辑，提供成员地址的注册、查询、更新和删除功能。
// 这是"仅 ID 连接"能力的核心数据结构。
//
// 从 v1.1.0 开始，MemberAddressBook 统一使用 BadgerDB 持久化存储，
// 必须通过 NewWithEngine 或 NewWithStore 传入存储实现。
type MemberAddressBook struct {
	realmID types.RealmID
	store   realmif.AddressBookStore
	mu      sync.RWMutex
	closed  atomic.Bool

	// 可选：事件总线，用于发布地址变更事件
	eventbus pkgif.EventBus
}

// Config 地址簿配置
type Config struct {
	// RealmID Realm 标识（必需）
	RealmID types.RealmID

	// Engine 存储引擎（必需，除非提供 Store）
	// 从 v1.1.0 开始，必须使用持久化存储
	Engine engine.InternalEngine

	// Store 存储实现（可选，优先使用）
	// 如果提供则忽略 Engine
	Store realmif.AddressBookStore

	// EventBus 事件总线（可选）
	EventBus pkgif.EventBus

	// DefaultTTL 默认 TTL（可选，默认 24 小时）
	DefaultTTL time.Duration
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c.Store == nil && c.Engine == nil {
		return fmt.Errorf("addressbook: %w", ErrEngineRequired)
	}
	return nil
}

// New 创建地址簿
//
// 从 v1.1.0 开始，必须提供 Engine 或 Store。
func New(config Config) (*MemberAddressBook, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	store := config.Store
	if store == nil {
		// 使用引擎创建存储
		var err error
		store, err = NewBadgerStoreWithEngine(config.Engine)
		if err != nil {
			return nil, fmt.Errorf("addressbook: create store: %w", err)
		}
	}

	return &MemberAddressBook{
		realmID:  config.RealmID,
		store:    store,
		eventbus: config.EventBus,
	}, nil
}

// NewWithEngine 使用存储引擎创建地址簿
//
// 这是推荐的创建方式，确保数据持久化。
func NewWithEngine(realmID types.RealmID, eng engine.InternalEngine) (*MemberAddressBook, error) {
	if eng == nil {
		return nil, fmt.Errorf("addressbook: %w", ErrEngineRequired)
	}

	store, err := NewBadgerStoreWithEngine(eng)
	if err != nil {
		return nil, err
	}

	return &MemberAddressBook{
		realmID: realmID,
		store:   store,
	}, nil
}

// NewWithStore 使用指定存储创建地址簿
//
// 参数:
//   - realmID: Realm ID
//   - store: 存储实现（必需）
func NewWithStore(realmID types.RealmID, store realmif.AddressBookStore) (*MemberAddressBook, error) {
	if store == nil {
		return nil, fmt.Errorf("addressbook: store is required (v1.1.0+ requires persistent storage)")
	}

	return &MemberAddressBook{
		realmID: realmID,
		store:   store,
	}, nil
}

// RealmID 返回 Realm ID
func (b *MemberAddressBook) RealmID() types.RealmID {
	return b.realmID
}

// Register 注册成员地址
//
// 成员加入 Realm 时调用，注册其地址信息。
// 如果成员已存在，则更新其地址。
func (b *MemberAddressBook) Register(ctx context.Context, entry realmif.MemberEntry) error {
	if b.closed.Load() {
		return ErrBookClosed
	}

	if entry.NodeID.IsEmpty() {
		return ErrInvalidNodeID
	}

	// 设置时间戳
	now := time.Now()
	if entry.LastSeen.IsZero() {
		entry.LastSeen = now
	}
	entry.LastUpdate = now
	entry.Online = true

	// 存储
	if err := b.store.Put(ctx, entry); err != nil {
		return err
	}

	// 发布事件（如果配置了事件总线）
	b.publishEvent(MemberRegisteredEvent{
		RealmID: b.realmID,
		NodeID:  entry.NodeID,
		Entry:   entry,
	})

	return nil
}

// Query 查询成员地址
//
// 通过 NodeID 查询成员地址信息。
// 如果成员不存在，返回 ErrMemberNotFound。
func (b *MemberAddressBook) Query(ctx context.Context, nodeID types.NodeID) (realmif.MemberEntry, error) {
	if b.closed.Load() {
		return realmif.MemberEntry{}, ErrBookClosed
	}

	if nodeID.IsEmpty() {
		return realmif.MemberEntry{}, ErrInvalidNodeID
	}

	entry, found, err := b.store.Get(ctx, nodeID)
	if err != nil {
		return realmif.MemberEntry{}, err
	}

	if !found {
		return realmif.MemberEntry{}, ErrMemberNotFound
	}

	return entry, nil
}

// Update 更新成员地址
//
// 更新已存在成员的地址信息。
// 如果成员不存在，返回 ErrMemberNotFound。
func (b *MemberAddressBook) Update(ctx context.Context, entry realmif.MemberEntry) error {
	if b.closed.Load() {
		return ErrBookClosed
	}

	if entry.NodeID.IsEmpty() {
		return ErrInvalidNodeID
	}

	// 检查成员是否存在
	_, found, err := b.store.Get(ctx, entry.NodeID)
	if err != nil {
		return err
	}
	if !found {
		return ErrMemberNotFound
	}

	// 更新时间戳
	entry.LastUpdate = time.Now()

	// 存储更新
	if err := b.store.Put(ctx, entry); err != nil {
		return err
	}

	// 发布事件
	b.publishEvent(MemberUpdatedEvent{
		RealmID: b.realmID,
		NodeID:  entry.NodeID,
		Entry:   entry,
	})

	return nil
}

// Remove 移除成员
//
// 成员离开 Realm 时调用。
func (b *MemberAddressBook) Remove(ctx context.Context, nodeID types.NodeID) error {
	if b.closed.Load() {
		return ErrBookClosed
	}

	if nodeID.IsEmpty() {
		return ErrInvalidNodeID
	}

	// 删除
	if err := b.store.Delete(ctx, nodeID); err != nil {
		return err
	}

	// 发布事件
	b.publishEvent(MemberRemovedEvent{
		RealmID: b.realmID,
		NodeID:  nodeID,
	})

	return nil
}

// Members 获取所有成员
//
// 返回所有已注册的成员列表。
func (b *MemberAddressBook) Members(ctx context.Context) ([]realmif.MemberEntry, error) {
	if b.closed.Load() {
		return nil, ErrBookClosed
	}

	return b.store.List(ctx)
}

// OnlineMembers 获取在线成员
//
// 返回当前在线的成员列表。
func (b *MemberAddressBook) OnlineMembers(ctx context.Context) ([]realmif.MemberEntry, error) {
	if b.closed.Load() {
		return nil, ErrBookClosed
	}

	all, err := b.store.List(ctx)
	if err != nil {
		return nil, err
	}

	online := make([]realmif.MemberEntry, 0, len(all))
	for _, entry := range all {
		if entry.Online {
			online = append(online, entry)
		}
	}

	return online, nil
}

// SetOnline 设置成员在线状态
//
// 心跳检测时调用，更新成员在线状态。
func (b *MemberAddressBook) SetOnline(ctx context.Context, nodeID types.NodeID, online bool) error {
	if b.closed.Load() {
		return ErrBookClosed
	}

	if nodeID.IsEmpty() {
		return ErrInvalidNodeID
	}

	// 获取现有条目
	entry, found, err := b.store.Get(ctx, nodeID)
	if err != nil {
		return err
	}
	if !found {
		return ErrMemberNotFound
	}

	// 更新状态
	entry.Online = online
	if online {
		entry.LastSeen = time.Now()
	}
	entry.LastUpdate = time.Now()

	// 存储更新
	return b.store.Put(ctx, entry)
}

// RefreshTTL 刷新成员 TTL（续期）
func (b *MemberAddressBook) RefreshTTL(ctx context.Context, nodeID types.NodeID, ttl time.Duration) error {
	if b.closed.Load() {
		return ErrBookClosed
	}

	return b.store.SetTTL(ctx, nodeID, ttl)
}

// CleanExpired 清理过期成员
func (b *MemberAddressBook) CleanExpired(ctx context.Context) error {
	if b.closed.Load() {
		return ErrBookClosed
	}

	return b.store.CleanExpired(ctx)
}

// Close 关闭地址簿
func (b *MemberAddressBook) Close() error {
	if b.closed.Swap(true) {
		return nil // 已经关闭
	}

	return b.store.Close()
}

// publishEvent 发布事件（内部方法）
func (b *MemberAddressBook) publishEvent(event interface{}) {
	if b.eventbus == nil {
		return
	}

	// 获取发射器并发布事件
	emitter, err := b.eventbus.Emitter(event)
	if err != nil {
		return
	}
	defer emitter.Close()

	emitter.Emit(event)
}

// ============================================================================
//                              事件定义
// ============================================================================

// MemberRegisteredEvent 成员注册事件
type MemberRegisteredEvent struct {
	RealmID types.RealmID
	NodeID  types.NodeID
	Entry   realmif.MemberEntry
}

// MemberUpdatedEvent 成员更新事件
type MemberUpdatedEvent struct {
	RealmID types.RealmID
	NodeID  types.NodeID
	Entry   realmif.MemberEntry
}

// MemberRemovedEvent 成员移除事件
type MemberRemovedEvent struct {
	RealmID types.RealmID
	NodeID  types.NodeID
}

// 确保实现了接口
var _ realmif.AddressBook = (*MemberAddressBook)(nil)
