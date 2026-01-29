package rendezvous

import (
	"encoding/binary"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              注册记录
// ============================================================================

// Registration 注册记录
type Registration struct {
	// Namespace 命名空间
	Namespace string

	// PeerInfo 节点信息
	PeerInfo types.PeerInfo

	// TTL 有效期
	TTL time.Duration

	// RegisteredAt 注册时间
	RegisteredAt time.Time

	// ExpiresAt 过期时间
	ExpiresAt time.Time

	// SignedRecord 签名记录
	SignedRecord []byte
}

// IsExpired 检查是否过期
func (r *Registration) IsExpired() bool {
	return time.Now().After(r.ExpiresAt)
}

// RemainingTTL 返回剩余 TTL
func (r *Registration) RemainingTTL() time.Duration {
	remaining := time.Until(r.ExpiresAt)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// ============================================================================
//                              Store 存储
// ============================================================================

// Store 注册信息存储
type Store struct {
	config StoreConfig

	// registrations: namespace -> peerID -> Registration
	registrations map[string]map[types.PeerID]*Registration

	// peerNamespaces: peerID -> set of namespaces
	peerNamespaces map[types.PeerID]map[string]struct{}

	mu sync.RWMutex

	// 统计
	totalRegistrations   int
	registrationsExpired uint64
}

// NewStore 创建存储
func NewStore(config StoreConfig) *Store {
	return &Store{
		config:         config,
		registrations:  make(map[string]map[types.PeerID]*Registration),
		peerNamespaces: make(map[types.PeerID]map[string]struct{}),
	}
}

// ============================================================================
//                              注册操作
// ============================================================================

// Add 添加或更新注册
func (s *Store) Add(namespace string, peerInfo types.PeerInfo, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 验证 TTL
	if ttl <= 0 {
		ttl = s.config.DefaultTTL
	}
	if ttl > s.config.MaxTTL {
		ttl = s.config.MaxTTL
	}

	// 检查命名空间数量限制
	if _, exists := s.registrations[namespace]; !exists {
		if len(s.registrations) >= s.config.MaxNamespaces {
			return ErrMaxNamespacesExceeded
		}
		s.registrations[namespace] = make(map[types.PeerID]*Registration)
	}

	nsRegs := s.registrations[namespace]

	// 检查是否是更新
	_, isUpdate := nsRegs[peerInfo.ID]

	// 检查命名空间内注册数限制
	if !isUpdate && len(nsRegs) >= s.config.MaxRegistrationsPerNamespace {
		return ErrMaxRegistrationsPerNamespaceExceeded
	}

	// 检查总注册数限制
	if !isUpdate && s.totalRegistrations >= s.config.MaxRegistrations {
		return ErrMaxRegistrationsExceeded
	}

	// 检查单个节点注册数限制
	if !isUpdate {
		if namespaces, exists := s.peerNamespaces[peerInfo.ID]; exists {
			if len(namespaces) >= s.config.MaxRegistrationsPerPeer {
				return ErrMaxRegistrationsPerPeerExceeded
			}
		}
	}

	now := time.Now()

	// 存储注册
	nsRegs[peerInfo.ID] = &Registration{
		Namespace:    namespace,
		PeerInfo:     peerInfo,
		TTL:          ttl,
		RegisteredAt: now,
		ExpiresAt:    now.Add(ttl),
	}

	// 更新 peer -> namespaces 索引
	if _, exists := s.peerNamespaces[peerInfo.ID]; !exists {
		s.peerNamespaces[peerInfo.ID] = make(map[string]struct{})
	}
	s.peerNamespaces[peerInfo.ID][namespace] = struct{}{}

	// 更新统计
	if !isUpdate {
		s.totalRegistrations++
	}

	return nil
}

// Remove 移除注册
func (s *Store) Remove(namespace string, peerID types.PeerID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	nsRegs, exists := s.registrations[namespace]
	if !exists {
		return
	}

	if _, exists := nsRegs[peerID]; !exists {
		return
	}

	delete(nsRegs, peerID)
	s.totalRegistrations--

	// 清理空的命名空间
	if len(nsRegs) == 0 {
		delete(s.registrations, namespace)
	}

	// 更新 peer -> namespaces 索引
	if namespaces, exists := s.peerNamespaces[peerID]; exists {
		delete(namespaces, namespace)
		if len(namespaces) == 0 {
			delete(s.peerNamespaces, peerID)
		}
	}
}

// ============================================================================
//                              查询操作
// ============================================================================

// Get 查询命名空间中的注册（支持分页）
func (s *Store) Get(namespace string, limit int, cookie []byte) ([]*Registration, []byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	nsRegs, exists := s.registrations[namespace]
	if !exists {
		return nil, nil, nil
	}

	// 解析 cookie（偏移量）
	offset := 0
	if len(cookie) >= 4 {
		offset = int(binary.BigEndian.Uint32(cookie))
	}

	// 收集有效注册
	var results []*Registration
	count := 0
	skipped := 0

	for _, reg := range nsRegs {
		if reg.IsExpired() {
			continue
		}

		// 跳过 offset 个
		if skipped < offset {
			skipped++
			continue
		}

		results = append(results, reg)
		count++

		// 检查限制
		if limit > 0 && count >= limit {
			break
		}
	}

	// 生成下一页 cookie
	var nextCookie []byte
	if limit > 0 && count >= limit && offset+count < len(nsRegs) {
		nextCookie = make([]byte, 4)
		binary.BigEndian.PutUint32(nextCookie, uint32(offset+count))
	}

	return results, nextCookie, nil
}

// GetPeerNamespaces 获取节点注册的所有命名空间
func (s *Store) GetPeerNamespaces(peerID types.PeerID) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	namespaces, exists := s.peerNamespaces[peerID]
	if !exists {
		return nil
	}

	result := make([]string, 0, len(namespaces))
	for ns := range namespaces {
		result = append(result, ns)
	}
	return result
}

// ============================================================================
//                              清理
// ============================================================================

// CleanupExpired 清理过期注册
func (s *Store) CleanupExpired() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	expired := 0

	for namespace, nsRegs := range s.registrations {
		for peerID, reg := range nsRegs {
			if now.After(reg.ExpiresAt) {
				delete(nsRegs, peerID)
				s.totalRegistrations--
				expired++

				// 更新 peer -> namespaces 索引
				if namespaces, exists := s.peerNamespaces[peerID]; exists {
					delete(namespaces, namespace)
					if len(namespaces) == 0 {
						delete(s.peerNamespaces, peerID)
					}
				}
			}
		}

		// 清理空的命名空间
		if len(nsRegs) == 0 {
			delete(s.registrations, namespace)
		}
	}

	if expired > 0 {
		atomic.AddUint64(&s.registrationsExpired, uint64(expired))
	}

	return expired
}

// ============================================================================
//                              统计
// ============================================================================

// Stats 统计信息
type Stats struct {
	TotalRegistrations   int
	TotalNamespaces      int
	RegistrationsExpired uint64
}

// Stats 返回统计信息
func (s *Store) Stats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return Stats{
		TotalRegistrations:   s.totalRegistrations,
		TotalNamespaces:      len(s.registrations),
		RegistrationsExpired: atomic.LoadUint64(&s.registrationsExpired),
	}
}
