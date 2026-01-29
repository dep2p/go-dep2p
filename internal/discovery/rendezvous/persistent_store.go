package rendezvous

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              持久化存储配置
// ============================================================================

// PersistentStoreConfig 持久化存储配置
type PersistentStoreConfig struct {
	// DataDir 数据目录
	DataDir string

	// SaveInterval 自动保存间隔
	SaveInterval time.Duration

	// MaxRegistrations 最大注册数
	MaxRegistrations int

	// MaxNamespaces 最大命名空间数
	MaxNamespaces int

	// MaxRegistrationsPerNamespace 每命名空间最大注册数
	MaxRegistrationsPerNamespace int

	// MaxRegistrationsPerPeer 每节点最大注册数
	MaxRegistrationsPerPeer int

	// MaxTTL 最大 TTL
	MaxTTL time.Duration

	// DefaultTTL 默认 TTL
	DefaultTTL time.Duration
}

// DefaultPersistentStoreConfig 默认配置
func DefaultPersistentStoreConfig() PersistentStoreConfig {
	return PersistentStoreConfig{
		DataDir:                      "",
		SaveInterval:                 5 * time.Minute,
		MaxRegistrations:             10000,
		MaxNamespaces:                1000,
		MaxRegistrationsPerNamespace: 1000,
		MaxRegistrationsPerPeer:      100,
		MaxTTL:                       72 * time.Hour,
		DefaultTTL:                   2 * time.Hour,
	}
}

// ============================================================================
//                              持久化注册记录
// ============================================================================

// persistedRegistration 持久化的注册记录
type persistedRegistration struct {
	Namespace    string   `json:"namespace"`
	PeerID       string   `json:"peer_id"`
	Addrs        []string `json:"addrs"`
	TTLSeconds   int64    `json:"ttl_seconds"`
	RegisteredAt int64    `json:"registered_at"`
	ExpiresAt    int64    `json:"expires_at"`
	SignedRecord []byte   `json:"signed_record,omitempty"`
}

// persistedData 持久化数据结构
type persistedData struct {
	Version       int                      `json:"version"`
	LastSaved     int64                    `json:"last_saved"`
	Registrations []*persistedRegistration `json:"registrations"`
}

// ============================================================================
//                              PersistentStore 实现
// ============================================================================

// PersistentStore 持久化存储
type PersistentStore struct {
	config PersistentStoreConfig
	store  *Store // 内嵌内存存储

	dataFile string
	dirty    bool

	stopCh   chan struct{}
	stoppedW sync.WaitGroup
	mu       sync.RWMutex
}

// NewPersistentStore 创建持久化存储
func NewPersistentStore(config PersistentStoreConfig) (*PersistentStore, error) {
	if config.DataDir == "" {
		return nil, errors.New("data directory is required")
	}

	// 创建数据目录
	if err := os.MkdirAll(config.DataDir, 0755); err != nil {
		return nil, err
	}

	// 创建内存存储
	storeConfig := StoreConfig{
		MaxRegistrations:             config.MaxRegistrations,
		MaxNamespaces:                config.MaxNamespaces,
		MaxRegistrationsPerNamespace: config.MaxRegistrationsPerNamespace,
		MaxRegistrationsPerPeer:      config.MaxRegistrationsPerPeer,
		MaxTTL:                       config.MaxTTL,
		DefaultTTL:                   config.DefaultTTL,
		CleanupInterval:              5 * time.Minute,
	}

	ps := &PersistentStore{
		config:   config,
		store:    NewStore(storeConfig),
		dataFile: filepath.Join(config.DataDir, "rendezvous_registrations.json"),
		stopCh:   make(chan struct{}),
	}

	// 加载已有数据
	if err := ps.load(); err != nil {
		// 忽略文件不存在的错误
		if !os.IsNotExist(err) {
			return nil, err
		}
	}

	// 启动自动保存
	if config.SaveInterval > 0 {
		ps.stoppedW.Add(1)
		go ps.saveLoop()
	}

	return ps, nil
}

// ============================================================================
//                              Store 接口代理
// ============================================================================

// Add 添加注册
func (ps *PersistentStore) Add(namespace string, peerInfo types.PeerInfo, ttl time.Duration) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if err := ps.store.Add(namespace, peerInfo, ttl); err != nil {
		return err
	}

	ps.dirty = true
	return nil
}

// AddWithSignedRecord 添加带签名记录的注册
func (ps *PersistentStore) AddWithSignedRecord(namespace string, peerInfo types.PeerInfo, ttl time.Duration, _ []byte) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	if err := ps.store.Add(namespace, peerInfo, ttl); err != nil {
		return err
	}

	// 存储签名记录（如果需要）
	// 可以扩展 store 来保存这个

	ps.dirty = true
	return nil
}

// Remove 移除注册
func (ps *PersistentStore) Remove(namespace string, peerID types.PeerID) {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	ps.store.Remove(namespace, peerID)
	ps.dirty = true
}

// Get 查询注册
func (ps *PersistentStore) Get(namespace string, limit int, cookie []byte) ([]*Registration, []byte, error) {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	return ps.store.Get(namespace, limit, cookie)
}

// CleanupExpired 清理过期注册
func (ps *PersistentStore) CleanupExpired() int {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	count := ps.store.CleanupExpired()
	if count > 0 {
		ps.dirty = true
	}
	return count
}

// Stats 统计信息
func (ps *PersistentStore) Stats() Stats {
	ps.mu.RLock()
	defer ps.mu.RUnlock()

	return ps.store.Stats()
}

// ============================================================================
//                              持久化操作
// ============================================================================

// load 从文件加载数据
func (ps *PersistentStore) load() error {
	data, err := os.ReadFile(ps.dataFile)
	if err != nil {
		return err
	}

	var persisted persistedData
	if err := json.Unmarshal(data, &persisted); err != nil {
		return err
	}

	// 恢复注册
	now := time.Now()
	for _, pr := range persisted.Registrations {
		expiresAt := time.Unix(0, pr.ExpiresAt)

		// 跳过已过期的
		if expiresAt.Before(now) {
			continue
		}

		// 转换地址
		addrs := make([]types.Multiaddr, 0, len(pr.Addrs))
		for _, addrStr := range pr.Addrs {
			ma, err := types.NewMultiaddr(addrStr)
			if err == nil {
				addrs = append(addrs, ma)
			}
		}

		peerInfo := types.PeerInfo{
			ID:    types.PeerID(pr.PeerID),
			Addrs: addrs,
		}

		// 计算剩余 TTL
		remainingTTL := expiresAt.Sub(now)
		if remainingTTL > 0 {
			_ = ps.store.Add(pr.Namespace, peerInfo, remainingTTL)
		}
	}

	return nil
}

// Save 保存到文件
func (ps *PersistentStore) Save() error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	return ps.saveUnlocked()
}

// saveUnlocked 无锁保存（调用者需持有锁）
func (ps *PersistentStore) saveUnlocked() error {
	if !ps.dirty {
		return nil
	}

	// 收集所有注册
	var registrations []*persistedRegistration

	// 遍历内存存储（需要访问内部结构）
	ps.store.mu.RLock()
	for namespace, nsRegs := range ps.store.registrations {
		for _, reg := range nsRegs {
			if reg.IsExpired() {
				continue
			}

			// 转换地址
			addrs := make([]string, len(reg.PeerInfo.Addrs))
			for i, addr := range reg.PeerInfo.Addrs {
				addrs[i] = addr.String()
			}

			pr := &persistedRegistration{
				Namespace:    namespace,
				PeerID:       string(reg.PeerInfo.ID),
				Addrs:        addrs,
				TTLSeconds:   int64(reg.TTL.Seconds()),
				RegisteredAt: reg.RegisteredAt.UnixNano(),
				ExpiresAt:    reg.ExpiresAt.UnixNano(),
				SignedRecord: reg.SignedRecord,
			}
			registrations = append(registrations, pr)
		}
	}
	ps.store.mu.RUnlock()

	// 序列化
	persisted := persistedData{
		Version:       1,
		LastSaved:     time.Now().UnixNano(),
		Registrations: registrations,
	}

	data, err := json.MarshalIndent(persisted, "", "  ")
	if err != nil {
		return err
	}

	// 写入临时文件（使用 0600 权限保护数据）
	tmpFile := ps.dataFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0600); err != nil {
		return err
	}

	// 原子重命名
	if err := os.Rename(tmpFile, ps.dataFile); err != nil {
		return err
	}

	ps.dirty = false
	return nil
}

// saveLoop 自动保存循环
func (ps *PersistentStore) saveLoop() {
	defer ps.stoppedW.Done()

	ticker := time.NewTicker(ps.config.SaveInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ps.mu.Lock()
			_ = ps.saveUnlocked()
			ps.mu.Unlock()

		case <-ps.stopCh:
			// 最后保存一次
			ps.mu.Lock()
			_ = ps.saveUnlocked()
			ps.mu.Unlock()
			return
		}
	}
}

// Close 关闭存储
func (ps *PersistentStore) Close() error {
	close(ps.stopCh)
	ps.stoppedW.Wait()
	return nil
}
