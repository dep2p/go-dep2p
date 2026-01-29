package connmgr

import "sync"

// logger 在 manager.go 中定义

// protectStore 存储节点保护信息
type protectStore struct {
	mu       sync.RWMutex
	protects map[string]map[string]struct{} // peerID -> tags
}

// newProtectStore 创建保护存储
func newProtectStore() *protectStore {
	return &protectStore{
		protects: make(map[string]map[string]struct{}),
	}
}

// Protect 保护节点（添加保护标签）
func (s *protectStore) Protect(peer, tag string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.protects[peer] == nil {
		s.protects[peer] = make(map[string]struct{})
	}
	s.protects[peer][tag] = struct{}{}
	logger.Debug("节点保护", "peerID", truncateID(peer, 8), "tag", tag)
}

// Unprotect 取消保护（移除保护标签）
// 返回 true 表示还有其他保护标签，false 表示没有了
func (s *protectStore) Unprotect(peer, tag string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.protects[peer] == nil {
		return false
	}

	delete(s.protects[peer], tag)

	// 如果没有保护标签了，删除整个节点
	if len(s.protects[peer]) == 0 {
		delete(s.protects, peer)
		logger.Debug("节点取消保护（无剩余标签）", "peerID", truncateID(peer, 8), "tag", tag)
		return false
	}

	logger.Debug("节点取消保护（仍有其他标签）", "peerID", truncateID(peer, 8), "tag", tag)
	return true
}

// IsProtected 检查节点的特定标签是否受保护
func (s *protectStore) IsProtected(peer, tag string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.protects[peer] == nil {
		return false
	}

	// 如果 tag 为空，检查是否有任何保护
	if tag == "" {
		return len(s.protects[peer]) > 0
	}

	_, protected := s.protects[peer][tag]
	return protected
}

// HasAnyProtection 检查节点是否有任何保护
func (s *protectStore) HasAnyProtection(peer string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.protects[peer]) > 0
}

// GetProtections 获取节点的所有保护标签
func (s *protectStore) GetProtections(peer string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.protects[peer] == nil {
		return nil
	}

	tags := make([]string, 0, len(s.protects[peer]))
	for tag := range s.protects[peer] {
		tags = append(tags, tag)
	}
	return tags
}

// Clear 清空所有保护（用于测试）
func (s *protectStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.protects = make(map[string]map[string]struct{})
}
