package rendezvous

import (
	"context"
	"encoding/binary"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	pb "github.com/dep2p/go-dep2p/pkg/proto/rendezvous"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 包级别日志实例
var log = logger.Logger("discovery.rendezvous.store")

// ============================================================================
//                              配置
// ============================================================================

// StoreConfig 存储配置
type StoreConfig struct {
	// MaxRegistrations 最大注册总数
	MaxRegistrations int

	// MaxNamespaces 最大命名空间数
	MaxNamespaces int

	// MaxRegistrationsPerNamespace 每个命名空间最大注册数
	MaxRegistrationsPerNamespace int

	// MaxRegistrationsPerPeer 每个节点最大注册数
	MaxRegistrationsPerPeer int

	// MaxTTL 最大 TTL
	MaxTTL time.Duration

	// DefaultTTL 默认 TTL
	DefaultTTL time.Duration

	// CleanupInterval 清理间隔
	CleanupInterval time.Duration
}

// DefaultStoreConfig 默认存储配置
func DefaultStoreConfig() StoreConfig {
	return StoreConfig{
		MaxRegistrations:             10000,
		MaxNamespaces:                1000,
		MaxRegistrationsPerNamespace: 1000,
		MaxRegistrationsPerPeer:      100,
		MaxTTL:                       72 * time.Hour,
		DefaultTTL:                   2 * time.Hour,
		CleanupInterval:              5 * time.Minute,
	}
}

// ============================================================================
//                              注册记录
// ============================================================================

// Registration 注册记录
type Registration struct {
	// Namespace 命名空间
	Namespace string

	// PeerInfo 节点信息
	PeerInfo discoveryif.PeerInfo

	// TTL 有效期
	TTL time.Duration

	// RegisteredAt 注册时间
	RegisteredAt time.Time

	// ExpiresAt 过期时间
	ExpiresAt time.Time

	// SignedRecord 可选的签名记录
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

// ToProto 转换为 protobuf Registration
func (r *Registration) ToProto() *pb.Registration {
	return PeerInfoToRegistration(r.PeerInfo, r.Namespace, r.RemainingTTL(), r.RegisteredAt)
}

// ============================================================================
//                              存储实现
// ============================================================================

// Store 注册信息存储
type Store struct {
	config StoreConfig

	// registrations: namespace -> peerID -> Registration
	registrations map[string]map[types.NodeID]*Registration

	// peerNamespaces: peerID -> set of namespaces
	peerNamespaces map[types.NodeID]map[string]struct{}

	mu sync.RWMutex

	// 统计
	totalRegistrations   int
	registrationsExpired uint64

	// 生命周期
	ctx       context.Context
	cancel    context.CancelFunc
	closeOnce sync.Once
}

// NewStore 创建存储
func NewStore(config StoreConfig) *Store {
	ctx, cancel := context.WithCancel(context.Background())

	s := &Store{
		config:         config,
		registrations:  make(map[string]map[types.NodeID]*Registration),
		peerNamespaces: make(map[types.NodeID]map[string]struct{}),
		ctx:            ctx,
		cancel:         cancel,
	}

	// 启动清理循环
	go s.cleanupLoop()

	return s
}

// Close 关闭存储（幂等）
func (s *Store) Close() {
	s.closeOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
	})
}

// ============================================================================
//                              注册操作
// ============================================================================

// Register 添加或更新注册
func (s *Store) Register(reg *Registration) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 验证 TTL
	ttl := reg.TTL
	if ttl <= 0 {
		ttl = s.config.DefaultTTL
	}
	if ttl > s.config.MaxTTL {
		ttl = s.config.MaxTTL
	}
	reg.TTL = ttl
	reg.ExpiresAt = reg.RegisteredAt.Add(ttl)

	// 检查命名空间数量限制
	if _, exists := s.registrations[reg.Namespace]; !exists {
		if len(s.registrations) >= s.config.MaxNamespaces {
			return ErrTooManyNamespaces
		}
		s.registrations[reg.Namespace] = make(map[types.NodeID]*Registration)
	}

	nsRegs := s.registrations[reg.Namespace]

	// 检查是否是更新
	_, isUpdate := nsRegs[reg.PeerInfo.ID]

	// 检查命名空间内注册数限制
	if !isUpdate && len(nsRegs) >= s.config.MaxRegistrationsPerNamespace {
		return ErrTooManyRegistrationsInNamespace
	}

	// 检查总注册数限制
	if !isUpdate && s.totalRegistrations >= s.config.MaxRegistrations {
		return ErrTooManyRegistrations
	}

	// 检查单个节点注册数限制
	if !isUpdate {
		if namespaces, exists := s.peerNamespaces[reg.PeerInfo.ID]; exists {
			if len(namespaces) >= s.config.MaxRegistrationsPerPeer {
				return ErrTooManyRegistrationsPerPeer
			}
		}
	}

	// 存储注册
	nsRegs[reg.PeerInfo.ID] = reg

	// 更新 peer -> namespaces 索引
	if _, exists := s.peerNamespaces[reg.PeerInfo.ID]; !exists {
		s.peerNamespaces[reg.PeerInfo.ID] = make(map[string]struct{})
	}
	s.peerNamespaces[reg.PeerInfo.ID][reg.Namespace] = struct{}{}

	// 更新统计
	if !isUpdate {
		s.totalRegistrations++
	}

	log.Debug("registered peer",
		"namespace", reg.Namespace,
		"peer", reg.PeerInfo.ID.String(),
		"ttl", reg.TTL,
	)

	return nil
}

// Unregister 取消注册
func (s *Store) Unregister(namespace string, peerID types.NodeID) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	nsRegs, exists := s.registrations[namespace]
	if !exists {
		return nil // 不存在视为成功
	}

	if _, exists := nsRegs[peerID]; !exists {
		return nil // 不存在视为成功
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

	log.Debug("unregistered peer",
		"namespace", namespace,
		"peer", peerID.String(),
	)

	return nil
}

// ============================================================================
//                              查询操作
// ============================================================================

// Discover 发现命名空间中的节点
func (s *Store) Discover(namespace string, limit int, cookie []byte) ([]*Registration, []byte, error) {
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

// GetRegistration 获取单个注册
func (s *Store) GetRegistration(namespace string, peerID types.NodeID) *Registration {
	s.mu.RLock()
	defer s.mu.RUnlock()

	nsRegs, exists := s.registrations[namespace]
	if !exists {
		return nil
	}

	reg, exists := nsRegs[peerID]
	if !exists || reg.IsExpired() {
		return nil
	}

	return reg
}

// GetPeerNamespaces 获取节点注册的所有命名空间
func (s *Store) GetPeerNamespaces(peerID types.NodeID) []string {
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

// Namespaces 返回所有命名空间
func (s *Store) Namespaces() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]string, 0, len(s.registrations))
	for ns := range s.registrations {
		result = append(result, ns)
	}
	return result
}

// PeersInNamespace 返回命名空间中的节点数
func (s *Store) PeersInNamespace(namespace string) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	nsRegs, exists := s.registrations[namespace]
	if !exists {
		return 0
	}

	count := 0
	for _, reg := range nsRegs {
		if !reg.IsExpired() {
			count++
		}
	}
	return count
}

// ============================================================================
//                              统计
// ============================================================================

// Stats 返回统计信息
func (s *Store) Stats() StoreStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return StoreStats{
		TotalRegistrations:   s.totalRegistrations,
		TotalNamespaces:      len(s.registrations),
		RegistrationsExpired: atomic.LoadUint64(&s.registrationsExpired),
	}
}

// StoreStats 存储统计
type StoreStats struct {
	TotalRegistrations   int
	TotalNamespaces      int
	RegistrationsExpired uint64
}

// ============================================================================
//                              清理
// ============================================================================

// cleanupLoop 定期清理过期注册
func (s *Store) cleanupLoop() {
	ticker := time.NewTicker(s.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cleanup()
		}
	}
}

// cleanup 清理过期注册
func (s *Store) cleanup() {
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
		log.Debug("cleaned up expired registrations",
			"count", expired,
		)
	}
}

// ============================================================================
//                              错误定义
// ============================================================================

var (
	// ErrTooManyNamespaces 命名空间过多
	ErrTooManyNamespaces = &StoreError{Code: "TOO_MANY_NAMESPACES", Message: "too many namespaces"}

	// ErrTooManyRegistrations 注册过多
	ErrTooManyRegistrations = &StoreError{Code: "TOO_MANY_REGISTRATIONS", Message: "too many registrations"}

	// ErrTooManyRegistrationsInNamespace 命名空间内注册过多
	ErrTooManyRegistrationsInNamespace = &StoreError{Code: "TOO_MANY_REGISTRATIONS_IN_NAMESPACE", Message: "too many registrations in namespace"}

	// ErrTooManyRegistrationsPerPeer 单个节点注册过多
	ErrTooManyRegistrationsPerPeer = &StoreError{Code: "TOO_MANY_REGISTRATIONS_PER_PEER", Message: "too many registrations per peer"}
)

// StoreError 存储错误
type StoreError struct {
	Code    string
	Message string
}

func (e *StoreError) Error() string {
	return e.Message
}
