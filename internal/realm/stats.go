package realm

// ============================================================================
//                              统计信息
// ============================================================================

// Stats Manager 统计信息
type Stats struct {
	// Manager 状态
	CurrentRealm  string
	TotalRealms   int
	ActiveMembers int
	TotalMembers  int

	// 子模块统计（简化实现：使用接口避免循环依赖）
	// AuthStats    interface{}
	// MemberStats  interface{}
	// RoutingStats interface{}
	// GatewayStats interface{}
}

// GetStats 获取统计信息
func (m *Manager) GetStats() *Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := &Stats{
		TotalRealms: len(m.realms),
	}

	// 当前 Realm
	if m.current != nil {
		stats.CurrentRealm = m.current.ID()
		stats.TotalMembers = m.current.MemberCount()

		// 子模块统计（简化实现）
		// 实际需要调用各子模块的 GetStats() 方法
	}

	return stats
}

// GetRealmStats 获取 Realm 统计
func (r *realmImpl) GetStats() *RealmStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := &RealmStats{
		ID:          r.id,
		Name:        r.name,
		MemberCount: r.MemberCount(),
		IsActive:    r.active.Load(),
	}

	return stats
}

// RealmStats Realm 统计信息
type RealmStats struct {
	ID          string
	Name        string
	MemberCount int
	IsActive    bool
}
