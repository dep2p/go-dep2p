package member

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/storage/kv"
	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
)

// ============================================================================
//                              持久化成员信息
// ============================================================================

// persistedMemberInfo 持久化的成员信息格式
type persistedMemberInfo struct {
	PeerID   string `json:"peer_id"`
	RealmID  string `json:"realm_id"`
	Role     int    `json:"role"`
	Online   bool   `json:"online"`
	JoinedAt int64  `json:"joined_at"`
	LastSeen int64  `json:"last_seen"`
}

// ============================================================================
//                              BadgerStore 实现
// ============================================================================

// BadgerStore BadgerDB 存储实现
//
// 键格式: m/{peerID}
// 值格式: JSON 序列化的 persistedMemberInfo
type BadgerStore struct {
	store *kv.Store

	// 内存缓存
	cache  map[string]*interfaces.MemberInfo
	mu     sync.RWMutex
	closed atomic.Bool
}

// NewBadgerStore 创建 BadgerDB 存储
//
// 参数:
//   - store: KV 存储实例（已带前缀 m/）
func NewBadgerStore(store *kv.Store) (*BadgerStore, error) {
	s := &BadgerStore{
		store: store,
		cache: make(map[string]*interfaces.MemberInfo),
	}

	// 从存储加载已有数据
	if err := s.loadFromStore(); err != nil {
		return nil, err
	}

	return s, nil
}

// loadFromStore 从存储加载数据到内存缓存
func (s *BadgerStore) loadFromStore() error {
	return s.store.PrefixScan(nil, func(_ []byte, value []byte) bool {
		var persisted persistedMemberInfo
		if err := json.Unmarshal(value, &persisted); err != nil {
			// 跳过损坏的数据
			return true
		}

		// 转换为 MemberInfo
		member := s.fromPersisted(&persisted)
		s.cache[member.PeerID] = member

		return true
	})
}

// toPersisted 转换为持久化格式
func (s *BadgerStore) toPersisted(member *interfaces.MemberInfo) *persistedMemberInfo {
	return &persistedMemberInfo{
		PeerID:   member.PeerID,
		RealmID:  member.RealmID,
		Role:     int(member.Role),
		Online:   member.Online,
		JoinedAt: member.JoinedAt.UnixNano(),
		LastSeen: member.LastSeen.UnixNano(),
	}
}

// fromPersisted 从持久化格式转换
func (s *BadgerStore) fromPersisted(p *persistedMemberInfo) *interfaces.MemberInfo {
	return &interfaces.MemberInfo{
		PeerID:   p.PeerID,
		RealmID:  p.RealmID,
		Role:     interfaces.Role(p.Role),
		Online:   p.Online,
		JoinedAt: time.Unix(0, p.JoinedAt),
		LastSeen: time.Unix(0, p.LastSeen),
	}
}

// ============================================================================
//                              MemberStore 接口实现
// ============================================================================

// Save 保存成员
func (s *BadgerStore) Save(member *interfaces.MemberInfo) error {
	if s.closed.Load() {
		return ErrStoreClosed
	}

	if member == nil {
		return ErrInvalidMember
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 持久化
	persisted := s.toPersisted(member)
	if err := s.store.PutJSON([]byte(member.PeerID), persisted); err != nil {
		return err
	}

	// 更新缓存
	s.cache[member.PeerID] = member

	return nil
}

// Load 加载成员
func (s *BadgerStore) Load(peerID string) (*interfaces.MemberInfo, error) {
	if s.closed.Load() {
		return nil, ErrStoreClosed
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	member, ok := s.cache[peerID]
	if !ok {
		return nil, ErrMemberNotFound
	}

	return member, nil
}

// Delete 删除成员
func (s *BadgerStore) Delete(peerID string) error {
	if s.closed.Load() {
		return ErrStoreClosed
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 从存储删除
	if err := s.store.Delete([]byte(peerID)); err != nil {
		return err
	}

	// 从缓存删除
	delete(s.cache, peerID)

	return nil
}

// LoadAll 加载所有成员
func (s *BadgerStore) LoadAll() ([]*interfaces.MemberInfo, error) {
	if s.closed.Load() {
		return nil, ErrStoreClosed
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	members := make([]*interfaces.MemberInfo, 0, len(s.cache))
	for _, member := range s.cache {
		members = append(members, member)
	}

	return members, nil
}

// Compact 压缩存储
//
// BadgerDB 自动进行 GC，此方法为空操作以满足接口要求
func (s *BadgerStore) Compact() error {
	if s.closed.Load() {
		return ErrStoreClosed
	}

	// BadgerDB 自动 GC，无需手动压缩
	return nil
}

// Close 关闭存储
func (s *BadgerStore) Close() error {
	if s.closed.Swap(true) {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.cache = nil

	return nil
}

// Len 返回成员数量（用于测试）
func (s *BadgerStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.cache)
}

// 确保实现接口
var _ interfaces.MemberStore = (*BadgerStore)(nil)
