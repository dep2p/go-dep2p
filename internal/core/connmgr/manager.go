// Package connmgr 提供连接管理模块的实现
package connmgr

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	"github.com/dep2p/go-dep2p/pkg/interfaces/connmgr"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 包级别日志实例
var log = logger.Logger("connmgr")

// ============================================================================
//                              ConnectionManager 实现
// ============================================================================

// ConnectionManager 连接管理器实现
type ConnectionManager struct {
	config connmgr.Config

	// 连接信息
	peers map[types.NodeID]*peerInfo
	mu    sync.RWMutex

	// 连接过滤器
	filter   connmgr.ConnectionFilter
	filterMu sync.RWMutex

	// 连接门控（黑名单）
	gater   connmgr.ConnectionGater
	gaterMu sync.RWMutex

	// 抖动容错
	jitter   *JitterTolerance
	jitterMu sync.RWMutex

	// 连接关闭回调（用于实际关闭连接）
	closeCallback func(types.NodeID) error
	// 重连回调
	reconnectCallback func(ctx context.Context, nodeID types.NodeID) error
	callbackMu        sync.RWMutex

	// 状态
	running int32
	closed  int32
	stopCh  chan struct{}

	// 裁剪触发通道
	trimCh chan struct{}
}

type peerInfo struct {
	nodeID     types.NodeID
	tags       map[string]struct{}
	connInfo   connmgr.ConnectionInfo
	protected  bool
	createdAt  time.Time
	lastActive time.Time

	// 用于多维度裁剪评分的统计信息
	bytesSent uint64 // 发送字节数
	bytesRecv uint64 // 接收字节数
	rtt       time.Duration // 往返时延（如果可用）
}

// NewConnectionManager 创建连接管理器
func NewConnectionManager(config connmgr.Config) *ConnectionManager {
	return &ConnectionManager{
		config: config,
		peers:  make(map[types.NodeID]*peerInfo),
		stopCh: make(chan struct{}),
		trimCh: make(chan struct{}, 1),
	}
}

// 确保实现接口
var _ connmgr.ConnectionManager = (*ConnectionManager)(nil)

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动管理器
func (m *ConnectionManager) Start(_ context.Context) error {
	if !atomic.CompareAndSwapInt32(&m.running, 0, 1) {
		return nil // 已经运行
	}

	log.Info("连接管理器启动中",
		"lowWater", m.config.LowWater,
		"highWater", m.config.HighWater,
		"emergencyWater", m.config.EmergencyWater)

	// 启动后台裁剪协程
	go m.trimLoop()

	log.Info("连接管理器已启动")
	return nil
}

// Close 关闭管理器
func (m *ConnectionManager) Close() error {
	if !atomic.CompareAndSwapInt32(&m.closed, 0, 1) {
		return nil // 已经关闭
	}

	log.Info("连接管理器停止中")

	// 发送停止信号
	close(m.stopCh)

	atomic.StoreInt32(&m.running, 0)
	log.Info("连接管理器已停止")
	return nil
}

// ============================================================================
//                              连接通知
// ============================================================================

// NotifyConnected 通知新连接
func (m *ConnectionManager) NotifyConnected(conn endpoint.Connection) {
	m.mu.Lock()

	nodeID := conn.RemoteID()
	now := time.Now()

	m.peers[nodeID] = &peerInfo{
		nodeID: nodeID,
		tags:   make(map[string]struct{}),
		connInfo: connmgr.ConnectionInfo{
			NodeID:    nodeID,
			Direction: conn.Direction(),
			CreatedAt: now,
		},
		createdAt:  now,
		lastActive: now,
	}

	connCount := len(m.peers)
	m.mu.Unlock()

	log.Debug("新连接",
		"peer", nodeID.ShortString(),
		"total", connCount)

	// 检查是否需要裁剪
	if connCount >= m.config.HighWater {
		m.TriggerTrim()
	}
}

// NotifyDisconnected 通知连接断开
func (m *ConnectionManager) NotifyDisconnected(nodeID types.NodeID) {
	m.mu.Lock()
	delete(m.peers, nodeID)
	connCount := len(m.peers)
	m.mu.Unlock()

	log.Debug("连接断开",
		"peer", nodeID.ShortString(),
		"total", connCount)
}

// ============================================================================
//                              查询
// ============================================================================

// ConnCount 返回连接数
func (m *ConnectionManager) ConnCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.peers)
}

// ConnCountByTag 返回指定标签的连接数
func (m *ConnectionManager) ConnCountByTag(tag string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, peer := range m.peers {
		if _, ok := peer.tags[tag]; ok {
			count++
		}
	}
	return count
}

// GetConnInfo 获取连接信息
func (m *ConnectionManager) GetConnInfo(nodeID types.NodeID) (connmgr.ConnectionInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	peer, ok := m.peers[nodeID]
	if !ok {
		return connmgr.ConnectionInfo{}, false
	}
	return peer.connInfo, true
}

// AllConnInfo 返回所有连接信息
func (m *ConnectionManager) AllConnInfo() []connmgr.ConnectionInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	infos := make([]connmgr.ConnectionInfo, 0, len(m.peers))
	for _, peer := range m.peers {
		infos = append(infos, peer.connInfo)
	}
	return infos
}

// UpdateLastActive 更新最后活跃时间
func (m *ConnectionManager) UpdateLastActive(nodeID types.NodeID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if peer, ok := m.peers[nodeID]; ok {
		peer.lastActive = time.Now()
	}
}

// UpdateStats 更新连接统计信息（用于裁剪评分）
func (m *ConnectionManager) UpdateStats(nodeID types.NodeID, bytesSent, bytesRecv uint64, rtt time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if peer, ok := m.peers[nodeID]; ok {
		peer.bytesSent = bytesSent
		peer.bytesRecv = bytesRecv
		if rtt > 0 {
			peer.rtt = rtt
		}
		peer.lastActive = time.Now()
	}
}

// SetCloseCallback 设置连接关闭回调（线程安全）
func (m *ConnectionManager) SetCloseCallback(callback func(types.NodeID) error) {
	m.callbackMu.Lock()
	m.closeCallback = callback
	m.callbackMu.Unlock()
}

// ============================================================================
//                              抖动容错
// ============================================================================

// EnableJitterTolerance 启用抖动容错（线程安全）
func (m *ConnectionManager) EnableJitterTolerance(config JitterConfig) {
	m.jitterMu.Lock()
	m.jitter = NewJitterTolerance(config)

	// 设置重连回调
	m.callbackMu.RLock()
	if m.reconnectCallback != nil {
		m.jitter.SetReconnectCallback(m.reconnectCallback)
	}
	m.callbackMu.RUnlock()
	m.jitterMu.Unlock()
}

// SetReconnectCallback 设置重连回调（线程安全）
func (m *ConnectionManager) SetReconnectCallback(callback func(ctx context.Context, nodeID types.NodeID) error) {
	m.callbackMu.Lock()
	m.reconnectCallback = callback
	m.callbackMu.Unlock()

	m.jitterMu.RLock()
	if m.jitter != nil {
		m.jitter.SetReconnectCallback(callback)
	}
	m.jitterMu.RUnlock()
}

// JitterTolerance 返回抖动容错器（线程安全）
func (m *ConnectionManager) JitterTolerance() *JitterTolerance {
	m.jitterMu.RLock()
	defer m.jitterMu.RUnlock()
	return m.jitter
}

// StartJitterTolerance 启动抖动容错
func (m *ConnectionManager) StartJitterTolerance(ctx context.Context) error {
	m.jitterMu.RLock()
	jitter := m.jitter
	m.jitterMu.RUnlock()

	if jitter == nil {
		return nil
	}
	return jitter.Start(ctx)
}

// GetJitterState 获取节点抖动状态
func (m *ConnectionManager) GetJitterState(nodeID types.NodeID) (PeerJitterState, bool) {
	m.jitterMu.RLock()
	jitter := m.jitter
	m.jitterMu.RUnlock()

	if jitter == nil {
		return StateConnected, false
	}
	return jitter.GetState(nodeID)
}

// JitterStats 返回抖动统计
func (m *ConnectionManager) JitterStats() JitterStats {
	m.jitterMu.RLock()
	jitter := m.jitter
	m.jitterMu.RUnlock()

	if jitter == nil {
		return JitterStats{}
	}
	return jitter.Stats()
}
