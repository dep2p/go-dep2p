// Package recovery 提供网络恢复功能
package recovery

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("core/recovery")

// ============================================================================
//                              错误定义
// ============================================================================

var (
	// ErrRecoveryInProgress 恢复已在进行中
	ErrRecoveryInProgress = errors.New("recovery already in progress")

	// ErrRecoveryFailed 恢复失败
	ErrRecoveryFailed = errors.New("recovery failed")

	// ErrRebindFailed rebind 失败
	ErrRebindFailed = errors.New("rebind failed")
)

// ============================================================================
//                              RecoveryManager
// ============================================================================

// Manager 恢复管理器
type Manager struct {
	mu sync.Mutex

	// 配置
	config *Config

	// 依赖组件
	rebinder          interfaces.Rebinder
	addressDiscoverer interfaces.AddressDiscoverer
	connector         interfaces.RecoveryConnector

	// 状态
	recovering     atomic.Bool
	currentAttempt int
	lastRecovery   time.Time

	// 恢复结果回调
	callbacks   []func(interfaces.RecoveryResult)
	callbacksMu sync.RWMutex

	// 运行状态
	ctx    context.Context
	cancel context.CancelFunc
}

// 确保实现接口
var _ interfaces.RecoveryManager = (*Manager)(nil)

// NewManager 创建恢复管理器
func NewManager(config *Config) *Manager {
	if config == nil {
		config = DefaultConfig()
	}
	// Validate 修正无效值为默认值（不会返回错误）
	config.Validate()

	return &Manager{
		config:    config,
		callbacks: make([]func(interfaces.RecoveryResult), 0),
	}
}

// ============================================================================
//                              配置设置
// ============================================================================

// SetRebinder 设置重绑定器
func (m *Manager) SetRebinder(r interfaces.Rebinder) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rebinder = r
}

// SetAddressDiscoverer 设置地址发现器
func (m *Manager) SetAddressDiscoverer(d interfaces.AddressDiscoverer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.addressDiscoverer = d
}

// SetConnector 设置连接器
func (m *Manager) SetConnector(c interfaces.RecoveryConnector) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connector = c
}

// SetCriticalPeers 设置关键节点列表
func (m *Manager) SetCriticalPeers(peers []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config.CriticalPeers = peers
}

// SetCriticalPeersWithAddrs 设置关键节点列表（带地址）
func (m *Manager) SetCriticalPeersWithAddrs(peers []string, addrs []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.config.CriticalPeers = peers
	m.config.CriticalPeersAddrs = addrs
}

// OnRecoveryComplete 注册恢复完成回调
func (m *Manager) OnRecoveryComplete(callback func(interfaces.RecoveryResult)) {
	m.callbacksMu.Lock()
	defer m.callbacksMu.Unlock()
	m.callbacks = append(m.callbacks, callback)
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动恢复管理器
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.ctx != nil {
		m.mu.Unlock()
		return nil // 已启动
	}
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.mu.Unlock()

	logger.Info("恢复管理器已启动")
	return nil
}

// Stop 停止恢复管理器
func (m *Manager) Stop() error {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Unlock()

	logger.Info("恢复管理器已停止")
	return nil
}

// ============================================================================
//                              恢复触发
// ============================================================================

// TriggerRecovery 触发恢复流程
func (m *Manager) TriggerRecovery(ctx context.Context, reason interfaces.RecoveryReason) *interfaces.RecoveryResult {
	startTime := time.Now()

	// 检查是否已在恢复中
	if !m.recovering.CompareAndSwap(false, true) {
		logger.Debug("恢复已在进行中，跳过")
		return &interfaces.RecoveryResult{
			Success:  false,
			Reason:   reason,
			Duration: time.Since(startTime),
			Error:    ErrRecoveryInProgress,
		}
	}
	defer m.recovering.Store(false)

	logger.Info("开始网络恢复", "reason", reason.String())

	m.mu.Lock()
	m.currentAttempt++
	attempt := m.currentAttempt
	m.mu.Unlock()

	result := &interfaces.RecoveryResult{
		Reason:   reason,
		Attempts: attempt,
	}

	// 创建超时上下文
	recoveryCtx, cancel := context.WithTimeout(ctx, m.config.RecoveryTimeout)
	defer cancel()

	// 步骤 1: Rebind（如果需要）
	if reason.NeedsRebind() && m.config.RebindOnCriticalError {
		if err := m.performRebind(recoveryCtx); err != nil {
			logger.Warn("Rebind 失败", "error", err)
			// 继续尝试其他恢复步骤
		} else {
			result.RebindPerformed = true
			logger.Info("Rebind 成功")
		}
	}

	// 步骤 2: 重新发现地址
	if m.config.RediscoverAddresses {
		if err := m.performAddressDiscovery(recoveryCtx); err != nil {
			logger.Warn("地址发现失败", "error", err)
		} else {
			result.AddressesDiscovered = 1
			logger.Info("地址发现完成")
		}
	}

	// 步骤 3: 重建关键连接
	restoredCount := m.reconnectCriticalPeers(recoveryCtx)
	result.ConnectionsRestored = restoredCount

	// 判断恢复是否成功
	m.mu.Lock()
	connector := m.connector
	m.mu.Unlock()

	if connector != nil && connector.ConnectionCount() > 0 {
		result.Success = true
		m.mu.Lock()
		m.lastRecovery = time.Now()
		m.currentAttempt = 0 // 重置尝试计数
		m.mu.Unlock()
		logger.Info("网络恢复成功", "connections_restored", restoredCount)
	} else {
		result.Success = false
		result.Error = ErrRecoveryFailed
		logger.Warn("网络恢复失败", "attempts", attempt)
	}

	result.Duration = time.Since(startTime)

	// 通知回调
	m.notifyCallbacks(*result)

	return result
}

// IsRecovering 检查是否正在恢复
func (m *Manager) IsRecovering() bool {
	return m.recovering.Load()
}

// GetAttemptCount 获取当前尝试次数
func (m *Manager) GetAttemptCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.currentAttempt
}

// ResetAttempts 重置尝试次数
func (m *Manager) ResetAttempts() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.currentAttempt = 0
}

// ============================================================================
//                              内部方法
// ============================================================================

// performRebind 执行 rebind
func (m *Manager) performRebind(ctx context.Context) error {
	m.mu.Lock()
	rebinder := m.rebinder
	m.mu.Unlock()

	if rebinder == nil {
		logger.Debug("未设置 Rebinder，跳过 rebind")
		return nil
	}

	if !rebinder.IsRebindNeeded() {
		logger.Debug("不需要 rebind")
		return nil
	}

	return rebinder.Rebind(ctx)
}

// performAddressDiscovery 执行地址发现
func (m *Manager) performAddressDiscovery(ctx context.Context) error {
	m.mu.Lock()
	discoverer := m.addressDiscoverer
	m.mu.Unlock()

	if discoverer == nil {
		logger.Debug("未设置 AddressDiscoverer，跳过地址发现")
		return nil
	}

	return discoverer.DiscoverAddresses(ctx)
}

// reconnectCriticalPeers 重连关键节点
func (m *Manager) reconnectCriticalPeers(ctx context.Context) int {
	m.mu.Lock()
	connector := m.connector
	criticalPeers := m.config.CriticalPeers
	criticalPeersAddrs := m.config.CriticalPeersAddrs
	m.mu.Unlock()

	if connector == nil {
		logger.Debug("未设置 Connector，跳过重连")
		return 0
	}

	if len(criticalPeers) == 0 {
		logger.Debug("无关键节点配置，跳过重连")
		return 0
	}

	var restoredCount int
	for i, peer := range criticalPeers {
		select {
		case <-ctx.Done():
			logger.Debug("恢复超时，停止重连")
			return restoredCount
		default:
		}

		peerLabel := peer
		if len(peerLabel) > 8 {
			peerLabel = peerLabel[:8]
		}

		// 优先使用地址直拨
		var err error
		if i < len(criticalPeersAddrs) && criticalPeersAddrs[i] != "" {
			// 尝试使用地址连接
			err = connector.ConnectWithAddrs(ctx, peer, []string{criticalPeersAddrs[i]})
			if err != nil {
				logger.Debug("使用地址重连关键节点失败，回退到 PeerID",
					"peer", peerLabel,
					"addr", criticalPeersAddrs[i],
					"error", err)
				// 回退到使用 PeerID 连接
				err = connector.Connect(ctx, peer)
			}
		} else {
			// 没有地址，直接使用 PeerID 连接
			err = connector.Connect(ctx, peer)
		}

		if err != nil {
			logger.Debug("重连关键节点失败",
				"peer", peerLabel,
				"error", err)
		} else {
			restoredCount++
			logger.Info("重连关键节点成功", "peer", peerLabel)
		}
	}

	return restoredCount
}

// notifyCallbacks 通知所有回调
func (m *Manager) notifyCallbacks(result interfaces.RecoveryResult) {
	m.callbacksMu.RLock()
	callbacks := make([]func(interfaces.RecoveryResult), len(m.callbacks))
	copy(callbacks, m.callbacks)
	m.callbacksMu.RUnlock()

	for _, cb := range callbacks {
		go func(callback func(interfaces.RecoveryResult)) {
			defer func() {
				if r := recover(); r != nil {
					logger.Warn("恢复回调 panic", "error", r)
				}
			}()
			callback(result)
		}(cb)
	}
}
