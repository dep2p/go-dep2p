// Package netmon 提供网络状态监控功能
package netmon

import (
	"context"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("core/netmon")

// ============================================================================
//                              NetworkMonitor
// ============================================================================

// Monitor 网络状态监控器
//
// IMPL-NETWORK-RESILIENCE Phase 2: 网络监控状态机
// Phase 5.2: 支持 Prober 主动探测和 SystemWatcher 系统事件
type Monitor struct {
	mu sync.RWMutex

	// 配置
	config *Config

	// 状态
	state interfaces.ConnectionHealth

	// 错误计数器
	errorCounter *ErrorCounter

	// 状态变更订阅者
	subscribers   []chan interfaces.ConnectionHealthChange
	subscribersMu sync.RWMutex

	// 恢复尝试计数
	recoveryAttempts int

	// 最后状态变更时间
	lastStateChange time.Time

	// 防抖定时器
	debounceTimer *time.Timer
	debounceMu    sync.Mutex

	// Phase 5.2: Prober 主动探测器（可选）
	prober Prober

	// Phase 5.2: SystemWatcher 系统网络监听器（可选）
	watcher SystemWatcher

	// NetworkMonitor 系统网络监控器（可选）
	// 用于订阅系统级网络变化事件
	networkMonitor interfaces.NetworkMonitor

	// 运行状态
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// 确保实现接口
var _ interfaces.ConnectionHealthMonitor = (*Monitor)(nil)

// NewMonitor 创建网络监控器
func NewMonitor(config *Config) *Monitor {
	if config == nil {
		config = DefaultConfig()
	}
	// FIX #B37: Validate() 只会修正无效值，永远返回 nil
	// 直接调用不需要检查错误
	config.Validate()

	return &Monitor{
		config:          config,
		state:           interfaces.ConnectionHealthy,
		errorCounter:    NewErrorCounter(config),
		subscribers:     make([]chan interfaces.ConnectionHealthChange, 0),
		lastStateChange: time.Now(),
	}
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动监控器
//
// Phase 5.2: 同时启动 Prober 和 SystemWatcher（如果已配置）
func (m *Monitor) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.ctx != nil {
		m.mu.Unlock()
		return nil // 已启动
	}
	m.ctx, m.cancel = context.WithCancel(ctx)
	watcher := m.watcher
	prober := m.prober
	networkMonitor := m.networkMonitor
	m.mu.Unlock()

	// Phase 5.2: 启动 SystemWatcher
	if watcher != nil {
		if err := watcher.Start(m.ctx); err != nil {
			logger.Warn("启动 SystemWatcher 失败", "error", err)
		} else {
			// 启动事件监听
			m.wg.Add(1)
			go m.watchSystemEvents()
		}
	}

	// 订阅 NetworkMonitor 事件
	if networkMonitor != nil {
		m.wg.Add(1)
		go m.watchNetworkMonitorEvents()
		logger.Debug("已订阅 NetworkMonitor 事件")
	}

	// Phase 5.2: 启动定期探测（如果配置了 Prober）
	if prober != nil && m.config.ProbeInterval > 0 {
		m.wg.Add(1)
		go m.probeLoop()
	}

	logger.Info("网络监控器已启动")
	return nil
}

// Stop 停止监控器
//
// Phase 5.2: 同时停止 Prober 循环和 SystemWatcher
func (m *Monitor) Stop() error {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
	}
	watcher := m.watcher
	m.mu.Unlock()

	// 等待所有 goroutine 结束
	m.wg.Wait()

	// Phase 5.2: 停止 SystemWatcher
	if watcher != nil {
		if err := watcher.Stop(); err != nil {
			logger.Warn("停止 SystemWatcher 失败", "error", err)
		}
	}

	// 关闭所有订阅通道
	m.subscribersMu.Lock()
	for _, ch := range m.subscribers {
		close(ch)
	}
	m.subscribers = nil
	m.subscribersMu.Unlock()

	logger.Info("网络监控器已停止")
	return nil
}

// ============================================================================
//                              错误上报
// ============================================================================

// OnSendError 处理发送错误
func (m *Monitor) OnSendError(peer string, err error) {
	if err == nil {
		return
	}

	// 记录错误
	reachedThreshold, isCritical := m.errorCounter.RecordError(peer, err)

	// FIX #B35: 安全截断 peer ID 用于日志
	peerLabel := truncatePeerID(peer, 8)

	logger.Debug("收到发送错误",
		"peer", peerLabel,
		"error", err,
		"threshold_reached", reachedThreshold,
		"is_critical", isCritical)

	// 检查是否需要状态变更
	if isCritical {
		// 关键错误立即触发 Down 状态
		m.transitionToWithDebounce(interfaces.ConnectionDown, interfaces.ReasonCriticalError, peer, err)
	} else if reachedThreshold {
		// 达到阈值，检查整体状态
		m.evaluateStateTransition(peer, err)
	}
}

// OnSendSuccess 处理发送成功
func (m *Monitor) OnSendSuccess(peer string) {
	m.errorCounter.RecordSuccess(peer)

	// 检查是否可以恢复到健康状态
	m.mu.RLock()
	currentState := m.state
	m.mu.RUnlock()

	// 当状态不是 Healthy 时，检查是否可以恢复
	if currentState != interfaces.ConnectionHealthy {
		// 检查是否所有节点都恢复
		failingPeers := m.errorCounter.GetFailingPeers()
		if len(failingPeers) == 0 {
			m.transitionTo(interfaces.ConnectionHealthy, interfaces.ReasonConnectionRestored, peer, nil)
		}
	}
}

// ============================================================================
//                              状态管理
// ============================================================================

// GetState 获取当前状态
func (m *Monitor) GetState() interfaces.ConnectionHealth {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state
}

// GetSnapshot 获取状态快照
func (m *Monitor) GetSnapshot() interfaces.ConnectionHealthSnapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, lastErrTime, lastErr := m.errorCounter.GetLastCriticalError()

	return interfaces.ConnectionHealthSnapshot{
		State:            m.state,
		Timestamp:        time.Now(),
		TotalPeers:       m.errorCounter.TotalPeerCount(),
		HealthyPeers:     len(m.errorCounter.GetHealthyPeers()),
		FailingPeers:     len(m.errorCounter.GetFailingPeers()),
		LastError:        lastErr,
		LastErrorTime:    lastErrTime,
		RecoveryAttempts: m.recoveryAttempts,
		LastRecoveryTime: m.lastStateChange,
	}
}

// Subscribe 订阅状态变更
func (m *Monitor) Subscribe() <-chan interfaces.ConnectionHealthChange {
	ch := make(chan interfaces.ConnectionHealthChange, 10)

	m.subscribersMu.Lock()
	m.subscribers = append(m.subscribers, ch)
	m.subscribersMu.Unlock()

	return ch
}

// Unsubscribe 取消订阅
func (m *Monitor) Unsubscribe(ch <-chan interfaces.ConnectionHealthChange) {
	m.subscribersMu.Lock()
	defer m.subscribersMu.Unlock()

	for i, sub := range m.subscribers {
		if sub == ch {
			close(sub)
			// FIX #B36: 安全删除切片元素
			// 使用交换末尾元素后截断的方式，避免在遍历中修改切片
			lastIdx := len(m.subscribers) - 1
			m.subscribers[i] = m.subscribers[lastIdx]
			m.subscribers = m.subscribers[:lastIdx]
			return
		}
	}
}

// TriggerRecoveryState 手动触发恢复状态
func (m *Monitor) TriggerRecoveryState() {
	m.transitionTo(interfaces.ConnectionRecovering, interfaces.ReasonManualTrigger, "", nil)
}

// NotifyRecoverySuccess 通知恢复成功
func (m *Monitor) NotifyRecoverySuccess() {
	m.mu.Lock()
	m.recoveryAttempts = 0
	m.mu.Unlock()

	m.transitionTo(interfaces.ConnectionHealthy, interfaces.ReasonRecoverySucceeded, "", nil)
}

// NotifyRecoveryFailed 通知恢复失败
func (m *Monitor) NotifyRecoveryFailed(err error) {
	m.mu.Lock()
	m.recoveryAttempts++
	attempts := m.recoveryAttempts
	m.mu.Unlock()

	// 如果尝试次数过多，保持 Down 状态
	if attempts >= m.config.MaxRecoveryAttempts {
		m.transitionTo(interfaces.ConnectionDown, interfaces.ReasonRecoveryFailed, "", err)
	}
}

// Reset 重置监控器状态
func (m *Monitor) Reset() {
	m.mu.Lock()
	m.state = interfaces.ConnectionHealthy
	m.recoveryAttempts = 0
	m.lastStateChange = time.Now()
	m.mu.Unlock()

	m.errorCounter.Reset()
}

// ============================================================================
//                              内部方法
// ============================================================================

// evaluateStateTransition 评估是否需要状态转换
func (m *Monitor) evaluateStateTransition(triggerPeer string, triggerErr error) {
	m.mu.RLock()
	currentState := m.state
	m.mu.RUnlock()

	failingPeers := m.errorCounter.GetFailingPeers()
	failingCount := len(failingPeers)

	// 获取健康节点数
	healthyPeers := m.errorCounter.GetHealthyPeers()
	healthyCount := len(healthyPeers)

	var newState interfaces.ConnectionHealth
	var reason interfaces.StateChangeReason

	switch {
	case failingCount == 0:
		newState = interfaces.ConnectionHealthy
		reason = interfaces.ReasonConnectionRestored
	case healthyCount == 0 && failingCount > 0:
		// 所有已知节点都失败
		newState = interfaces.ConnectionDown
		reason = interfaces.ReasonAllConnectionsLost
	case failingCount > 0:
		newState = interfaces.ConnectionDegraded
		reason = interfaces.ReasonErrorThreshold
	default:
		return // 无需变更
	}

	if newState != currentState {
		m.transitionToWithDebounce(newState, reason, triggerPeer, triggerErr)
	}
}

// transitionToWithDebounce 带防抖的状态转换
func (m *Monitor) transitionToWithDebounce(newState interfaces.ConnectionHealth, reason interfaces.StateChangeReason, peer string, err error) {
	m.debounceMu.Lock()
	defer m.debounceMu.Unlock()

	// 取消之前的定时器
	if m.debounceTimer != nil {
		m.debounceTimer.Stop()
	}

	// 关键错误或恢复相关不防抖
	if reason == interfaces.ReasonCriticalError || reason == interfaces.ReasonRecoverySucceeded || reason == interfaces.ReasonManualTrigger {
		m.transitionTo(newState, reason, peer, err)
		return
	}

	// 设置防抖定时器
	m.debounceTimer = time.AfterFunc(m.config.StateChangeDebounce, func() {
		m.transitionTo(newState, reason, peer, err)
	})
}

// transitionTo 执行状态转换
func (m *Monitor) transitionTo(newState interfaces.ConnectionHealth, reason interfaces.StateChangeReason, peer string, err error) {
	m.mu.Lock()
	previousState := m.state
	if previousState == newState {
		m.mu.Unlock()
		return
	}

	m.state = newState
	m.lastStateChange = time.Now()
	m.mu.Unlock()

	// 创建状态变更事件
	change := interfaces.ConnectionHealthChange{
		PreviousState: previousState,
		CurrentState:  newState,
		Reason:        reason,
		Timestamp:     time.Now(),
		TriggerPeer:   peer,
		TriggerError:  err,
	}

	logger.Info("网络状态变更",
		"previous", previousState.String(),
		"current", newState.String(),
		"reason", reason.String())

	// 通知订阅者
	m.notifySubscribers(change)
}

// notifySubscribers 通知所有订阅者
func (m *Monitor) notifySubscribers(change interfaces.ConnectionHealthChange) {
	m.subscribersMu.RLock()
	subscribers := make([]chan interfaces.ConnectionHealthChange, len(m.subscribers))
	copy(subscribers, m.subscribers)
	m.subscribersMu.RUnlock()

	// FIX #B38: 对于关键状态变更，不能静默丢弃
	// 使用超时后强制发送，避免丢失重要通知
	for _, ch := range subscribers {
		select {
		case ch <- change:
			// 成功发送
		default:
			// 通道已满，记录警告并尝试带超时的发送
			logger.Warn("订阅者处理过慢，状态变更可能延迟",
				"state", change.CurrentState.String(),
				"reason", change.Reason.String())

			// 使用 100ms 超时尝试发送
			select {
			case ch <- change:
				logger.Debug("延迟发送成功")
			case <-time.After(100 * time.Millisecond):
				// 超时后仍然失败，记录错误但不阻塞
				logger.Error("订阅者无响应，丢弃状态变更通知",
					"state", change.CurrentState.String())
			}
		}
	}
}

// GetErrorCounter 获取错误计数器（用于测试和诊断）
func (m *Monitor) GetErrorCounter() *ErrorCounter {
	return m.errorCounter
}

// ============================================================================
//                         Phase 5.2: Prober 和 SystemWatcher 支持
// ============================================================================

// SetProber 设置网络探测器
//
// Phase 5.2: 用于主动探测网络健康状态
// 必须在 Start() 之前调用
func (m *Monitor) SetProber(prober Prober) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prober = prober
}

// SetSystemWatcher 设置系统网络监听器
//
// Phase 5.2: 用于监听操作系统网络接口变化
// 必须在 Start() 之前调用
func (m *Monitor) SetSystemWatcher(watcher SystemWatcher) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.watcher = watcher
}

// SetNetworkMonitor 设置系统网络监控器
//
// 用于订阅系统级网络变化事件（如网卡变化、IP 变化）
// 必须在 Start() 之前调用
func (m *Monitor) SetNetworkMonitor(monitor interfaces.NetworkMonitor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.networkMonitor = monitor
}

// probeLoop 定期探测循环
func (m *Monitor) probeLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.ProbeInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.performProbe()
		}
	}
}

// performProbe 执行一次探测
func (m *Monitor) performProbe() {
	m.mu.RLock()
	prober := m.prober
	m.mu.RUnlock()

	if prober == nil {
		return
	}

	result := prober.Probe(m.ctx)

	logger.Debug("探测完成",
		"success", result.Success,
		"reachable", result.ReachablePeers,
		"total", result.TotalPeers)

	// 根据探测结果更新状态
	m.mu.RLock()
	currentState := m.state
	m.mu.RUnlock()

	if result.IsDown() && currentState != interfaces.ConnectionDown {
		// 探测显示网络不可用
		m.transitionToWithDebounce(interfaces.ConnectionDown, interfaces.ReasonProbeFailed, "", result.Error)
	} else if result.IsDegraded() && currentState == interfaces.ConnectionHealthy {
		// 探测显示网络降级
		m.transitionToWithDebounce(interfaces.ConnectionDegraded, interfaces.ReasonProbeFailed, "", result.Error)
	} else if result.IsHealthy() && currentState != interfaces.ConnectionHealthy {
		// 探测显示网络恢复
		m.transitionTo(interfaces.ConnectionHealthy, interfaces.ReasonConnectionRestored, "", nil)
	}
}

// watchSystemEvents 监听系统网络事件
func (m *Monitor) watchSystemEvents() {
	defer m.wg.Done()

	m.mu.RLock()
	watcher := m.watcher
	m.mu.RUnlock()

	if watcher == nil {
		return
	}

	events := watcher.Events()

	for {
		select {
		case <-m.ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			m.handleSystemEvent(event)
		}
	}
}

// handleSystemEvent 处理系统网络事件
func (m *Monitor) handleSystemEvent(event NetworkEvent) {
	logger.Info("收到系统网络事件",
		"type", event.Type.String(),
		"interface", event.Interface,
		"address", event.Address)

	// 重大变化触发状态检查
	if event.Type.IsMajorChange() {
		m.mu.RLock()
		currentState := m.state
		m.mu.RUnlock()

		// 如果是接口下线或网关变化，可能需要触发恢复
		if currentState == interfaces.ConnectionHealthy {
			m.transitionToWithDebounce(interfaces.ConnectionDegraded, interfaces.ReasonNetworkChanged, "", nil)
		}
	}
}

// watchNetworkMonitorEvents 监听 NetworkMonitor 事件
func (m *Monitor) watchNetworkMonitorEvents() {
	defer m.wg.Done()

	m.mu.RLock()
	networkMonitor := m.networkMonitor
	m.mu.RUnlock()

	if networkMonitor == nil {
		return
	}

	events := networkMonitor.Subscribe()

	for {
		select {
		case <-m.ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			m.handleNetworkChangeEvent(event)
		}
	}
}

// handleNetworkChangeEvent 处理 NetworkMonitor 事件
func (m *Monitor) handleNetworkChangeEvent(event interfaces.NetworkChangeEvent) {
	logger.Info("收到 NetworkMonitor 事件",
		"type", event.Type,
		"oldAddrs", len(event.OldAddrs),
		"newAddrs", len(event.NewAddrs))

	m.mu.RLock()
	currentState := m.state
	m.mu.RUnlock()

	// 网络变化时，检查是否需要状态变更
	if event.Type == interfaces.NetworkChangeMajor {
		// 主要变化（网卡切换）可能导致连接中断
		if currentState == interfaces.ConnectionHealthy {
			m.transitionToWithDebounce(interfaces.ConnectionDegraded, interfaces.ReasonNetworkChanged, "", nil)
		}
	}
}

// ============================================================================
//                              辅助函数
// ============================================================================

// truncatePeerID 安全截断 peer ID 用于日志输出
//
// FIX #B35: 避免字符串切片导致的 panic
// 处理多字节 UTF-8 字符和短字符串情况
func truncatePeerID(peerID string, maxLen int) string {
	if len(peerID) == 0 {
		return ""
	}

	// 如果长度小于等于最大长度，直接返回
	if len(peerID) <= maxLen {
		return peerID
	}

	// 转换为 rune 切片以正确处理多字节字符
	runes := []rune(peerID)
	if len(runes) <= maxLen {
		return peerID
	}

	return string(runes[:maxLen])
}
