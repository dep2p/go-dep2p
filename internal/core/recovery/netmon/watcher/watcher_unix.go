//go:build darwin || linux || freebsd || openbsd || netbsd
// +build darwin linux freebsd openbsd netbsd

package watcher

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// 轮询间隔配置
const (
	// normalPollInterval 正常轮询间隔
	normalPollInterval = 2 * time.Second
	
	// fastPollInterval 检测到变化后的快速轮询间隔
	fastPollInterval = 500 * time.Millisecond
	
	// fastPollDuration 快速轮询持续时间
	fastPollDuration = 10 * time.Second
)

// unixInterfaceWatcher Unix 平台的网络接口监控器
//
// 使用优化的轮询策略：
// - 正常情况下每 2 秒检查一次
// - 检测到变化后切换到 500ms 快速轮询，持续 10 秒
// - 支持外部通知触发立即检查
type unixInterfaceWatcher struct {
	events chan InterfaceEvent
	stopCh chan struct{}
	
	// 强制检查信号
	forceCheck chan struct{}
	
	// 上次状态（用于变化检测）
	lastState   pkgif.NetworkState
	lastStateMu sync.RWMutex
	
	// 是否处于快速轮询模式
	fastMode atomic.Bool
	
	// 已停止标志
	stopped atomic.Bool
}

// newInterfaceWatcher 创建平台特定的接口监控器
func newInterfaceWatcher() (InterfaceWatcher, error) {
	w := &unixInterfaceWatcher{
		events:     make(chan InterfaceEvent, 10),
		stopCh:     make(chan struct{}),
		forceCheck: make(chan struct{}, 1),
	}
	
	// 获取初始状态
	state, _ := getCurrentNetworkState()
	w.lastState = state
	
	return w, nil
}

// Watch 开始监控网络接口变化
func (w *unixInterfaceWatcher) Watch(ctx context.Context) (<-chan InterfaceEvent, error) {
	// 启动监控循环
	go w.watchLoop(ctx)
	
	return w.events, nil
}

// CurrentState 获取当前网络接口状态
func (w *unixInterfaceWatcher) CurrentState() (pkgif.NetworkState, error) {
	return getCurrentNetworkState()
}

// Stop 停止监控
func (w *unixInterfaceWatcher) Stop() error {
	if w.stopped.CompareAndSwap(false, true) {
		close(w.stopCh)
	}
	return nil
}

// ForceCheck 强制立即检查（用于外部通知）
func (w *unixInterfaceWatcher) ForceCheck() {
	select {
	case w.forceCheck <- struct{}{}:
	default:
		// 已经有待处理的检查
	}
}

// watchLoop 监控主循环
func (w *unixInterfaceWatcher) watchLoop(ctx context.Context) {
	ticker := time.NewTicker(normalPollInterval)
	defer ticker.Stop()
	
	var fastModeTimer *time.Timer
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
			
		case <-w.forceCheck:
			// 外部触发立即检查
			w.checkAndNotify()
			// 进入快速轮询模式
			w.enterFastMode(&ticker, &fastModeTimer)
			
		case <-ticker.C:
			// 定期检查
			if w.checkAndNotify() {
				// 检测到变化，进入快速轮询模式
				w.enterFastMode(&ticker, &fastModeTimer)
			}
		}
	}
}

// enterFastMode 进入快速轮询模式
func (w *unixInterfaceWatcher) enterFastMode(ticker **time.Ticker, fastModeTimer **time.Timer) {
	if !w.fastMode.CompareAndSwap(false, true) {
		return // 已在快速模式
	}
	
	logger.Debug("进入快速轮询模式")
	
	// 重置 ticker 为快速间隔
	(*ticker).Stop()
	*ticker = time.NewTicker(fastPollInterval)
	
	// 设置定时器退出快速模式
	if *fastModeTimer != nil {
		(*fastModeTimer).Stop()
	}
	
	*fastModeTimer = time.AfterFunc(fastPollDuration, func() {
		w.exitFastMode(ticker)
	})
}

// exitFastMode 退出快速轮询模式
func (w *unixInterfaceWatcher) exitFastMode(ticker **time.Ticker) {
	if !w.fastMode.CompareAndSwap(true, false) {
		return // 已不在快速模式
	}
	
	logger.Debug("退出快速轮询模式")
	
	// 重置 ticker 为正常间隔
	(*ticker).Stop()
	*ticker = time.NewTicker(normalPollInterval)
}

// checkAndNotify 检查状态变化并通知
//
// 返回 true 表示检测到变化
func (w *unixInterfaceWatcher) checkAndNotify() bool {
	currentState, err := w.CurrentState()
	if err != nil {
		logger.Warn("获取网络状态失败", "err", err)
		return false
	}
	
	w.lastStateMu.RLock()
	lastState := w.lastState
	w.lastStateMu.RUnlock()
	
	// 检测变化
	if !w.hasChanged(lastState, currentState) {
		return false
	}
	
	// 更新最后状态
	w.lastStateMu.Lock()
	w.lastState = currentState
	w.lastStateMu.Unlock()
	
	// 确定变化类型
	eventType := w.determineEventType(lastState, currentState)
	
	// 发送变化事件
	event := InterfaceEvent{
		Type:      eventType,
		Timestamp: time.Now(),
	}
	
	select {
	case w.events <- event:
		logger.Info("检测到网络接口变化", "type", eventType)
	default:
		logger.Warn("事件通道已满，丢弃事件")
	}
	
	return true
}

// determineEventType 确定事件类型
func (w *unixInterfaceWatcher) determineEventType(old, new pkgif.NetworkState) InterfaceEventType {
	oldNames := make(map[string]bool)
	for _, iface := range old.Interfaces {
		oldNames[iface.Name] = true
	}
	
	newNames := make(map[string]bool)
	for _, iface := range new.Interfaces {
		newNames[iface.Name] = true
	}
	
	// 检查新增接口
	for name := range newNames {
		if !oldNames[name] {
			return InterfaceAdded
		}
	}
	
	// 检查移除接口
	for name := range oldNames {
		if !newNames[name] {
			return InterfaceRemoved
		}
	}
	
	// 接口列表相同，但有其他变化
	return InterfaceChanged
}

// hasChanged 检查状态是否发生变化
func (w *unixInterfaceWatcher) hasChanged(old, new pkgif.NetworkState) bool {
	// 接口数量变化
	if len(old.Interfaces) != len(new.Interfaces) {
		return true
	}

	// 检查接口名称
	oldNames := make(map[string]bool)
	for _, iface := range old.Interfaces {
		oldNames[iface.Name] = true
	}

	for _, iface := range new.Interfaces {
		if !oldNames[iface.Name] {
			return true
		}
	}

	// 检查地址
	oldAddrs := extractAddrs(old.Interfaces)
	newAddrs := extractAddrs(new.Interfaces)

	return !equalAddrs(oldAddrs, newAddrs)
}
