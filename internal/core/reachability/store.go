// Package reachability 提供可达性协调模块的实现
package reachability

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
//                              DirectAddrStore
// ============================================================================

// DirectAddrStore 直连地址存储（落盘）
type DirectAddrStore struct {
	// 存储路径
	storePath string

	// 内存缓存
	candidates map[string]*storedAddressEntry
	verified   map[string]*storedAddressEntry
	mu         sync.RWMutex

	// flush 控制
	flushMu       sync.Mutex
	flushPending  bool
	flushTimer    *time.Timer
	flushDebounce time.Duration

	// 配置
	maxEntries   int
	candidateTTL time.Duration
	verifiedTTL  time.Duration

	// 运行状态
	ctx    context.Context
	cancel context.CancelFunc
}

// storedAddressEntry 存储格式的地址条目
type storedAddressEntry struct {
	AddrString string                     `json:"addr"`
	Priority   interfaces.AddressPriority `json:"priority"`
	Source     string                     `json:"source"`
	Sources    []string                   `json:"sources,omitempty"`
	Verified   bool                       `json:"verified"`
	VerifiedAt int64                      `json:"verified_at,omitempty"`
	LastSeen   int64                      `json:"last_seen"`
}

// storeSchema 存储文件 schema
type storeSchema struct {
	Version    int                            `json:"version"`
	UpdatedAt  int64                          `json:"updated_at"`
	Candidates map[string]*storedAddressEntry `json:"candidates"`
	Verified   map[string]*storedAddressEntry `json:"verified"`
}

const (
	schemaVersion        = 1
	defaultFlushDebounce = 1 * time.Second
	defaultMaxEntries    = 1000
	defaultCandidateTTL  = 2 * time.Hour
	defaultVerifiedTTL   = 24 * time.Hour
)

// NewDirectAddrStore 创建直连地址存储
func NewDirectAddrStore(storePath string) *DirectAddrStore {
	if storePath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			homeDir = "."
		}
		storePath = filepath.Join(homeDir, ".dep2p", "direct_addrs.json")
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &DirectAddrStore{
		storePath:     storePath,
		candidates:    make(map[string]*storedAddressEntry),
		verified:      make(map[string]*storedAddressEntry),
		flushDebounce: defaultFlushDebounce,
		maxEntries:    defaultMaxEntries,
		candidateTTL:  defaultCandidateTTL,
		verifiedTTL:   defaultVerifiedTTL,
		ctx:           ctx,
		cancel:        cancel,
	}
}

// SetConfig 设置配置
func (s *DirectAddrStore) SetConfig(maxEntries int, candidateTTL, verifiedTTL, flushDebounce time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if maxEntries > 0 {
		s.maxEntries = maxEntries
	}
	if candidateTTL > 0 {
		s.candidateTTL = candidateTTL
	}
	if verifiedTTL > 0 {
		s.verifiedTTL = verifiedTTL
	}
	if flushDebounce > 0 {
		s.flushDebounce = flushDebounce
	}
}

// Load 加载存储文件
func (s *DirectAddrStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 检查文件是否存在
	if _, err := os.Stat(s.storePath); os.IsNotExist(err) {
		logger.Debug("存储文件不存在，使用空状态", "path", s.storePath)
		return nil
	}

	// 读取文件
	data, err := os.ReadFile(s.storePath)
	if err != nil {
		return fmt.Errorf("读取存储文件失败: %w", err)
	}

	// 解析 JSON
	var schema storeSchema
	if err := json.Unmarshal(data, &schema); err != nil {
		return fmt.Errorf("解析存储文件失败: %w", err)
	}

	// 检查版本
	if schema.Version != schemaVersion {
		logger.Warn("存储文件版本不匹配",
			"fileVersion", schema.Version,
			"expectedVersion", schemaVersion)
		return nil
	}

	// 加载数据
	if schema.Candidates != nil {
		s.candidates = schema.Candidates
	} else {
		s.candidates = make(map[string]*storedAddressEntry)
	}

	if schema.Verified != nil {
		s.verified = schema.Verified
	} else {
		s.verified = make(map[string]*storedAddressEntry)
	}

	logger.Info("已加载直连地址存储",
		"candidates", len(s.candidates),
		"verified", len(s.verified),
		"path", s.storePath)

	return nil
}

// Save 保存到存储文件（原子写）
func (s *DirectAddrStore) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	schema := storeSchema{
		Version:    schemaVersion,
		UpdatedAt:  time.Now().Unix(),
		Candidates: s.candidates,
		Verified:   s.verified,
	}

	data, err := json.MarshalIndent(schema, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化存储数据失败: %w", err)
	}

	// 确保目录存在
	dir := filepath.Dir(s.storePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("创建存储目录失败: %w", err)
	}

	// 原子写（使用 0600 权限保护敏感数据）
	tmpPath := s.storePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("写入临时文件失败: %w", err)
	}

	if err := os.Rename(tmpPath, s.storePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("原子替换存储文件失败: %w", err)
	}

	logger.Debug("已保存直连地址存储",
		"candidates", len(s.candidates),
		"verified", len(s.verified),
		"path", s.storePath)

	return nil
}

// ScheduleFlush 调度 flush（debounce）
func (s *DirectAddrStore) ScheduleFlush() {
	s.flushMu.Lock()
	defer s.flushMu.Unlock()

	if s.flushPending {
		s.flushTimer.Stop()
	}

	s.flushPending = true
	s.flushTimer = time.AfterFunc(s.flushDebounce, func() {
		s.flushMu.Lock()
		s.flushPending = false
		s.flushMu.Unlock()

		if err := s.Save(); err != nil {
			logger.Warn("保存存储文件失败", "err", err)
		}
	})
}

// UpdateCandidate 更新候选地址
func (s *DirectAddrStore) UpdateCandidate(addr string, source string, priority interfaces.AddressPriority) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()

	entry, exists := s.candidates[addr]
	if !exists {
		entry = &storedAddressEntry{
			AddrString: addr,
			Priority:   priority,
			Source:     source,
			Sources:    []string{source},
			Verified:   false,
			LastSeen:   now,
		}
	} else {
		entry.LastSeen = now

		// 添加新来源
		found := false
		for _, src := range entry.Sources {
			if src == source {
				found = true
				break
			}
		}
		if !found {
			entry.Sources = append(entry.Sources, source)
		}

		if priority > entry.Priority {
			entry.Priority = priority
		}
	}

	// 检查容量
	if len(s.candidates) >= s.maxEntries && !exists {
		s.evictOldestCandidate()
	}

	s.candidates[addr] = entry
	s.ScheduleFlush()
}

// UpdateVerified 更新已验证地址
func (s *DirectAddrStore) UpdateVerified(addr string, source string, priority interfaces.AddressPriority) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().Unix()

	// 从候选地址中移除
	delete(s.candidates, addr)

	entry, exists := s.verified[addr]
	if !exists {
		entry = &storedAddressEntry{
			AddrString: addr,
			Priority:   priority,
			Source:     source,
			Sources:    []string{source},
			Verified:   true,
			VerifiedAt: now,
			LastSeen:   now,
		}
	} else {
		entry.LastSeen = now
		if !entry.Verified {
			entry.Verified = true
			entry.VerifiedAt = now
		}

		found := false
		for _, src := range entry.Sources {
			if src == source {
				found = true
				break
			}
		}
		if !found {
			entry.Sources = append(entry.Sources, source)
		}

		if priority > entry.Priority {
			entry.Priority = priority
		}
	}

	// 检查容量
	if len(s.verified) >= s.maxEntries && !exists {
		s.evictOldestVerified()
	}

	s.verified[addr] = entry
	s.ScheduleFlush()
}

// RemoveCandidate 移除候选地址
func (s *DirectAddrStore) RemoveCandidate(addr string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.candidates, addr)
	s.ScheduleFlush()
}

// RemoveVerified 移除已验证地址
func (s *DirectAddrStore) RemoveVerified(addr string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.verified, addr)
	s.ScheduleFlush()
}

// evictOldestCandidate 淘汰最旧的候选地址
func (s *DirectAddrStore) evictOldestCandidate() {
	if len(s.candidates) == 0 {
		return
	}

	var oldestKey string
	oldestTime := time.Now().Unix()

	for key, entry := range s.candidates {
		if entry.LastSeen < oldestTime {
			oldestTime = entry.LastSeen
			oldestKey = key
		}
	}

	if oldestKey != "" {
		delete(s.candidates, oldestKey)
		logger.Debug("已淘汰最旧候选地址", "addr", oldestKey)
	}
}

// evictOldestVerified 淘汰最旧的已验证地址
func (s *DirectAddrStore) evictOldestVerified() {
	if len(s.verified) == 0 {
		return
	}

	var oldestKey string
	oldestTime := time.Now().Unix()

	for key, entry := range s.verified {
		if entry.LastSeen < oldestTime {
			oldestTime = entry.LastSeen
			oldestKey = key
		}
	}

	if oldestKey != "" {
		delete(s.verified, oldestKey)
		logger.Debug("已淘汰最旧已验证地址", "addr", oldestKey)
	}
}

// CleanExpired 清理过期地址
func (s *DirectAddrStore) CleanExpired() (candidatesRemoved, verifiedRemoved int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// 清理过期候选地址
	for key, entry := range s.candidates {
		lastSeen := time.Unix(entry.LastSeen, 0)
		if now.Sub(lastSeen) > s.candidateTTL {
			delete(s.candidates, key)
			candidatesRemoved++
		}
	}

	// 清理过期已验证地址
	for key, entry := range s.verified {
		lastSeen := time.Unix(entry.LastSeen, 0)
		if now.Sub(lastSeen) > s.verifiedTTL {
			delete(s.verified, key)
			verifiedRemoved++
		}
	}

	if candidatesRemoved > 0 || verifiedRemoved > 0 {
		s.ScheduleFlush()
		logger.Debug("已清理过期地址",
			"candidatesRemoved", candidatesRemoved,
			"verifiedRemoved", verifiedRemoved)
	}

	return candidatesRemoved, verifiedRemoved
}

// GetCandidates 获取所有候选地址
func (s *DirectAddrStore) GetCandidates() map[string]*storedAddressEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*storedAddressEntry, len(s.candidates))
	for k, v := range s.candidates {
		result[k] = v
	}
	return result
}

// GetVerified 获取所有已验证地址
func (s *DirectAddrStore) GetVerified() map[string]*storedAddressEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[string]*storedAddressEntry, len(s.verified))
	for k, v := range s.verified {
		result[k] = v
	}
	return result
}

// Close 关闭存储
func (s *DirectAddrStore) Close() error {
	s.cancel()

	s.flushMu.Lock()
	if s.flushTimer != nil {
		s.flushTimer.Stop()
	}
	s.flushMu.Unlock()

	return s.Save()
}
