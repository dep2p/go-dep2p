// Package connmgr 提供连接管理模块的实现
package connmgr

import (
	"github.com/dep2p/go-dep2p/pkg/interfaces/connmgr"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              过滤
// ============================================================================

// AllowConnection 检查是否允许连接
func (m *ConnectionManager) AllowConnection(nodeID types.NodeID, direction types.Direction) bool {
	m.mu.RLock()
	connCount := len(m.peers)
	m.mu.RUnlock()

	// 检查紧急水位线
	if connCount >= m.config.EmergencyWater {
		log.Warn("达到紧急水位线，拒绝连接",
			"peer", nodeID.ShortString(),
			"current", connCount,
			"limit", m.config.EmergencyWater)
		return false
	}

	// 检查连接门控（黑名单）
	m.gaterMu.RLock()
	gater := m.gater
	m.gaterMu.RUnlock()

	if gater != nil {
		// 出站连接检查
		if direction == types.DirOutbound && !gater.InterceptPeerDial(nodeID) {
			log.Debug("连接被门控拒绝（节点在黑名单）",
				"peer", nodeID.ShortString())
			return false
		}
		// 入站连接在 InterceptSecured 中检查
		if direction == types.DirInbound && !gater.InterceptSecured(direction, nodeID) {
			log.Debug("连接被门控拒绝（节点在黑名单）",
				"peer", nodeID.ShortString())
			return false
		}
	}

	// 检查自定义过滤器
	m.filterMu.RLock()
	filter := m.filter
	m.filterMu.RUnlock()

	if filter != nil && !filter.Allow(nodeID, direction) {
		log.Debug("连接被过滤器拒绝",
			"peer", nodeID.ShortString())
		return false
	}

	return true
}

// SetConnectionFilter 设置连接过滤器
func (m *ConnectionManager) SetConnectionFilter(filter connmgr.ConnectionFilter) {
	m.filterMu.Lock()
	m.filter = filter
	m.filterMu.Unlock()

	log.Debug("设置连接过滤器")
}

// SetConnectionGater 设置连接门控
func (m *ConnectionManager) SetConnectionGater(gater connmgr.ConnectionGater) {
	m.gaterMu.Lock()
	m.gater = gater
	m.gaterMu.Unlock()

	log.Debug("设置连接门控")
}

// GetConnectionGater 获取连接门控
func (m *ConnectionManager) GetConnectionGater() connmgr.ConnectionGater {
	m.gaterMu.RLock()
	defer m.gaterMu.RUnlock()
	return m.gater
}

// ============================================================================
//                              配置
// ============================================================================

// SetLimits 设置水位线
func (m *ConnectionManager) SetLimits(low, high int) {
	m.config.LowWater = low
	m.config.HighWater = high

	log.Info("更新水位线",
		"lowWater", low,
		"highWater", high)
}

// GetLimits 获取水位线
func (m *ConnectionManager) GetLimits() (low, high int) {
	return m.config.LowWater, m.config.HighWater
}

