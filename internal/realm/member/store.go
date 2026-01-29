package member

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
)

// ============================================================================
//                              存储接口实现
// ============================================================================

// Store 成员持久化存储
type Store struct {
	mu        sync.RWMutex
	path      string
	storeType string
	file      *os.File
	memory    map[string]*interfaces.MemberInfo
	closed    bool
}

// NewStore 创建存储
func NewStore(path string, storeType string) (*Store, error) {
	store := &Store{
		path:      path,
		storeType: storeType,
		memory:    make(map[string]*interfaces.MemberInfo),
	}

	if storeType == "file" && path != "" {
		// 确保目录存在
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create directory: %w", err)
		}

		// 打开文件（如果存在则加载）
		if err := store.loadFromFile(); err != nil {
			return nil, err
		}
	}

	return store, nil
}

// Save 保存成员
func (s *Store) Save(member *interfaces.MemberInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	if member == nil {
		return ErrInvalidMember
	}

	if s.storeType == "memory" {
		s.memory[member.PeerID] = member
		return nil
	}

	// 文件存储：追加写入
	if s.file == nil {
		var err error
		s.file, err = os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			return fmt.Errorf("failed to open file: %w", err)
		}
	}

	// 序列化为 JSON
	data, err := json.Marshal(member)
	if err != nil {
		return fmt.Errorf("failed to marshal member: %w", err)
	}

	// 写入一行
	if _, err := s.file.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write member: %w", err)
	}

	// 同时更新内存索引
	s.memory[member.PeerID] = member

	return nil
}

// Load 加载成员
func (s *Store) Load(peerID string) (*interfaces.MemberInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	member, ok := s.memory[peerID]
	if !ok {
		return nil, ErrMemberNotFound
	}

	return member, nil
}

// Delete 删除成员
func (s *Store) Delete(peerID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	delete(s.memory, peerID)

	// 文件存储需要重写（在 Compact 时处理）
	return nil
}

// LoadAll 加载所有成员
func (s *Store) LoadAll() ([]*interfaces.MemberInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed {
		return nil, ErrStoreClosed
	}

	members := make([]*interfaces.MemberInfo, 0, len(s.memory))
	for _, member := range s.memory {
		members = append(members, member)
	}

	return members, nil
}

// Compact 压缩存储（重写文件，移除已删除的成员）
func (s *Store) Compact() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return ErrStoreClosed
	}

	if s.storeType != "file" {
		return nil // 内存存储不需要压缩
	}

	// 关闭当前文件
	if s.file != nil {
		s.file.Close()
		s.file = nil
	}

	// 创建临时文件
	tmpPath := s.path + ".tmp"
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}

	// 写入所有成员
	for _, member := range s.memory {
		data, err := json.Marshal(member)
		if err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("failed to marshal member: %w", err)
		}

		if _, err := tmpFile.Write(append(data, '\n')); err != nil {
			tmpFile.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("failed to write member: %w", err)
		}
	}

	tmpFile.Close()

	// 替换原文件
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("failed to rename file: %w", err)
	}

	return nil
}

// Close 关闭存储
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	if s.file != nil {
		s.file.Close()
		s.file = nil
	}

	s.memory = nil

	return nil
}

// loadFromFile 从文件加载所有成员
func (s *Store) loadFromFile() error {
	// 检查文件是否存在
	if _, err := os.Stat(s.path); os.IsNotExist(err) {
		return nil // 文件不存在，正常
	}

	file, err := os.Open(s.path)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var member interfaces.MemberInfo
		if err := json.Unmarshal(line, &member); err != nil {
			// 跳过无效行
			continue
		}

		s.memory[member.PeerID] = &member
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("failed to scan file: %w", err)
	}

	return nil
}

// 确保实现接口
var _ interfaces.MemberStore = (*Store)(nil)
