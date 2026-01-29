// Package watcher 提供网络监控和变化处理功能
package watcher

import (
	"context"
	"net"
	"sync"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// Monitor 网络监控器实现
type Monitor struct {
	subscribers   []chan pkgif.NetworkChangeEvent
	subscribersMu sync.RWMutex

	currentState   pkgif.NetworkState
	currentStateMu sync.RWMutex

	watcher InterfaceWatcher

	stopCh chan struct{}
	once   sync.Once
}

// InterfaceWatcher 网络接口监控接口
type InterfaceWatcher interface {
	// Watch 开始监控网络接口变化
	// 返回事件通道，发生变化时会发送事件
	Watch(ctx context.Context) (<-chan InterfaceEvent, error)

	// CurrentState 获取当前网络接口状态
	CurrentState() (pkgif.NetworkState, error)

	// Stop 停止监控
	Stop() error
}

// InterfaceEvent 网络接口事件
type InterfaceEvent struct {
	Type      InterfaceEventType
	Interface pkgif.NetworkInterface
	Timestamp time.Time
}

// InterfaceEventType 接口事件类型
type InterfaceEventType int

const (
	InterfaceAdded InterfaceEventType = iota
	InterfaceRemoved
	InterfaceChanged
)

// NewMonitor 创建网络监控器
func NewMonitor() (*Monitor, error) {
	watcher, err := newInterfaceWatcher()
	if err != nil {
		return nil, err
	}

	// 获取初始状态
	state, err := watcher.CurrentState()
	if err != nil {
		// 降级：使用空状态
		state = pkgif.NetworkState{
			Interfaces:         []pkgif.NetworkInterface{},
			PreferredInterface: "",
			IsOnline:           false,
		}
	}

	return &Monitor{
		subscribers:  make([]chan pkgif.NetworkChangeEvent, 0),
		currentState: state,
		watcher:      watcher,
		stopCh:       make(chan struct{}),
	}, nil
}

// Start 启动网络监控
func (m *Monitor) Start(ctx context.Context) error {
	// 启动接口监控
	events, err := m.watcher.Watch(ctx)
	if err != nil {
		return err
	}

	// 启动事件处理循环
	go m.eventLoop(ctx, events)

	logger.Info("网络监控已启动")
	return nil
}

// Stop 停止网络监控
func (m *Monitor) Stop() error {
	m.once.Do(func() {
		close(m.stopCh)
		if m.watcher != nil {
			m.watcher.Stop()
		}
	})
	logger.Info("网络监控已停止")
	return nil
}

// Subscribe 订阅网络变化事件
func (m *Monitor) Subscribe() <-chan pkgif.NetworkChangeEvent {
	m.subscribersMu.Lock()
	defer m.subscribersMu.Unlock()

	ch := make(chan pkgif.NetworkChangeEvent, 10)
	m.subscribers = append(m.subscribers, ch)
	return ch
}

// NotifyChange 外部通知网络变化
//
// 用于无法自动检测网络变化的平台（如 Android）
func (m *Monitor) NotifyChange() {
	logger.Debug("收到外部网络变化通知")
	
	// 获取新状态
	newState, err := m.watcher.CurrentState()
	if err != nil {
		logger.Warn("获取网络状态失败", "err", err)
		return
	}

	// 检测变化
	m.detectAndNotifyChange(newState)
}

// CurrentState 获取当前网络状态
func (m *Monitor) CurrentState() pkgif.NetworkState {
	m.currentStateMu.RLock()
	defer m.currentStateMu.RUnlock()
	return m.currentState
}

// eventLoop 事件处理循环
func (m *Monitor) eventLoop(ctx context.Context, events <-chan InterfaceEvent) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case event := <-events:
			m.handleInterfaceEvent(event)
		}
	}
}

// handleInterfaceEvent 处理接口事件
func (m *Monitor) handleInterfaceEvent(event InterfaceEvent) {
	logger.Debug("接收到网络接口事件",
		"type", event.Type,
		"interface", event.Interface.Name)

	// 获取新状态
	newState, err := m.watcher.CurrentState()
	if err != nil {
		logger.Warn("获取网络状态失败", "err", err)
		return
	}

	// 检测变化
	m.detectAndNotifyChange(newState)
}

// detectAndNotifyChange 检测并通知网络变化
func (m *Monitor) detectAndNotifyChange(newState pkgif.NetworkState) {
	m.currentStateMu.Lock()
	oldState := m.currentState
	m.currentState = newState
	m.currentStateMu.Unlock()

	// 分类变化类型
	changeType := m.classifyChange(oldState, newState)

	// 提取地址列表
	oldAddrs := extractAddrs(oldState.Interfaces)
	newAddrs := extractAddrs(newState.Interfaces)

	// 创建事件
	event := pkgif.NetworkChangeEvent{
		Type:      changeType,
		OldAddrs:  oldAddrs,
		NewAddrs:  newAddrs,
		Timestamp: time.Now(),
	}

	logger.Info("检测到网络变化",
		"type", changeType,
		"oldInterfaces", len(oldState.Interfaces),
		"newInterfaces", len(newState.Interfaces))

	// 通知所有订阅者
	m.notifySubscribers(event)
}

// classifyChange 分类网络变化类型
func (m *Monitor) classifyChange(oldState, newState pkgif.NetworkState) pkgif.NetworkChangeType {
	// 接口数量变化 = Major
	if len(oldState.Interfaces) != len(newState.Interfaces) {
		return pkgif.NetworkChangeMajor
	}

	// 检查接口名称变化
	oldNames := make(map[string]bool)
	for _, iface := range oldState.Interfaces {
		oldNames[iface.Name] = true
	}

	newNames := make(map[string]bool)
	for _, iface := range newState.Interfaces {
		newNames[iface.Name] = true
		
		// 新接口出现 = Major
		if !oldNames[iface.Name] {
			return pkgif.NetworkChangeMajor
		}
	}

	// 旧接口消失 = Major
	for name := range oldNames {
		if !newNames[name] {
			return pkgif.NetworkChangeMajor
		}
	}

	// 接口列表相同，检查地址变化
	oldAddrs := extractAddrs(oldState.Interfaces)
	newAddrs := extractAddrs(newState.Interfaces)

	if !equalAddrs(oldAddrs, newAddrs) {
		// 仅地址变化 = Minor
		return pkgif.NetworkChangeMinor
	}

	// 无变化
	return pkgif.NetworkChangeMinor
}

// notifySubscribers 通知所有订阅者
func (m *Monitor) notifySubscribers(event pkgif.NetworkChangeEvent) {
	m.subscribersMu.RLock()
	defer m.subscribersMu.RUnlock()

	for _, ch := range m.subscribers {
		select {
		case ch <- event:
		default:
			logger.Warn("订阅者通道已满，丢弃事件")
		}
	}
}

// extractAddrs 提取所有接口的地址
func extractAddrs(interfaces []pkgif.NetworkInterface) []string {
	addrs := make([]string, 0)
	for _, iface := range interfaces {
		addrs = append(addrs, iface.Addrs...)
	}
	return addrs
}

// equalAddrs 比较两个地址列表是否相等
func equalAddrs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	aMap := make(map[string]bool)
	for _, addr := range a {
		aMap[addr] = true
	}

	for _, addr := range b {
		if !aMap[addr] {
			return false
		}
	}

	return true
}

// getCurrentNetworkState 获取当前网络状态
func getCurrentNetworkState() (pkgif.NetworkState, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return pkgif.NetworkState{}, err
	}

	interfaces := make([]pkgif.NetworkInterface, 0)
	preferredInterface := ""
	isOnline := false

	for _, iface := range ifaces {
		// 跳过未启用的接口
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		// 获取地址
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		addrStrs := make([]string, 0)
		for _, addr := range addrs {
			addrStrs = append(addrStrs, addr.String())
		}

		if len(addrStrs) == 0 {
			continue
		}

		netIface := pkgif.NetworkInterface{
			Name:       iface.Name,
			Addrs:      addrStrs,
			IsUp:       iface.Flags&net.FlagUp != 0,
			IsLoopback: iface.Flags&net.FlagLoopback != 0,
		}

		interfaces = append(interfaces, netIface)

		// 选择首选接口（非回环的第一个）
		if !netIface.IsLoopback && preferredInterface == "" {
			preferredInterface = iface.Name
			isOnline = true
		}
	}

	return pkgif.NetworkState{
		Interfaces:         interfaces,
		PreferredInterface: preferredInterface,
		IsOnline:           isOnline,
	}, nil
}
