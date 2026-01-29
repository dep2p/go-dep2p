// Package netmon 提供网络状态监控功能
//
//go:build darwin
// +build darwin

package netmon

import (
	"context"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// Phase 9 修复：实现 macOS 原生网络变化监听
// 使用 BSD routing socket (AF_ROUTE) 监听网络变化事件

// darwinSystemWatcher macOS 原生网络变化监听器
//
// 使用 BSD routing socket 监听网络配置变化，包括：
// - 接口 up/down
// - IP 地址变化
// - 路由表变化
type darwinSystemWatcher struct {
	config *WatcherConfig

	// routing socket
	routingSocket int

	// 事件通道
	events chan NetworkEvent

	// 状态
	running atomic.Bool

	// 回调（兼容旧接口）
	callbacks   []func(NetworkEvent)
	callbacksMu sync.RWMutex

	// 上次已知状态（用于检测变化）
	lastAddrs   map[string]struct{}
	lastAddrsMu sync.RWMutex

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// routing message 类型常量
const (
	rtmAdd     = 0x1  // 添加路由
	rtmDelete  = 0x2  // 删除路由
	rtmChange  = 0x3  // 路由变化
	rtmNewAddr = 0xc  // 新地址
	rtmDelAddr = 0xd  // 删除地址
	rtmIfInfo  = 0xe  // 接口信息变化
)

// rt_msghdr routing message header
type rtMsghdr struct {
	MsgLen  uint16
	Version uint8
	Type    uint8
	// ... 其他字段省略
}

// newNativeSystemWatcher 创建平台原生监听器（macOS 实现）
//
// Phase 9 修复：使用 BSD routing socket 实现原生网络变化监听
func newNativeSystemWatcher(config *WatcherConfig) SystemWatcher {
	ctx, cancel := context.WithCancel(context.Background())

	watcher := &darwinSystemWatcher{
		config:    config,
		events:    make(chan NetworkEvent, 16),
		lastAddrs: make(map[string]struct{}),
		ctx:       ctx,
		cancel:    cancel,
	}

	return watcher
}

// Start 启动监听
func (w *darwinSystemWatcher) Start(ctx context.Context) error {
	if !w.running.CompareAndSwap(false, true) {
		return nil // 已经在运行
	}

	// 创建 routing socket
	fd, err := syscall.Socket(syscall.AF_ROUTE, syscall.SOCK_RAW, 0)
	if err != nil {
		w.running.Store(false)
		// 回退到轮询模式
		return w.startPollingFallback(ctx)
	}
	w.routingSocket = fd

	// 记录初始地址状态
	w.updateAddressSnapshot()

	// 启动监听循环
	w.wg.Add(1)
	go w.watchLoop()

	return nil
}

// Stop 停止监听
func (w *darwinSystemWatcher) Stop() error {
	if !w.running.CompareAndSwap(true, false) {
		return nil
	}

	w.cancel()

	// 关闭 socket 以中断阻塞的 read
	if w.routingSocket > 0 {
		syscall.Close(w.routingSocket)
	}

	w.wg.Wait()

	// 关闭事件通道
	close(w.events)

	return nil
}

// Events 返回事件通道
func (w *darwinSystemWatcher) Events() <-chan NetworkEvent {
	return w.events
}

// IsRunning 检查是否正在运行
func (w *darwinSystemWatcher) IsRunning() bool {
	return w.running.Load()
}

// OnNetworkChange 注册网络变化回调（兼容旧接口）
func (w *darwinSystemWatcher) OnNetworkChange(callback func(NetworkEvent)) {
	w.callbacksMu.Lock()
	w.callbacks = append(w.callbacks, callback)
	w.callbacksMu.Unlock()
}

// watchLoop 监听循环
func (w *darwinSystemWatcher) watchLoop() {
	defer w.wg.Done()

	buf := make([]byte, 4096)

	for w.running.Load() {
		// 设置读取超时（允许定期检查 running 状态）
		syscall.SetsockoptTimeval(w.routingSocket, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &syscall.Timeval{
			Sec:  1,
			Usec: 0,
		})

		n, err := syscall.Read(w.routingSocket, buf)
		if err != nil {
			// 超时或中断，继续循环
			if err == syscall.EAGAIN || err == syscall.EINTR {
				continue
			}
			// 其他错误，停止
			if !w.running.Load() {
				return
			}
			continue
		}

		if n < int(unsafe.Sizeof(rtMsghdr{})) {
			continue
		}

		// 解析 routing message
		hdr := (*rtMsghdr)(unsafe.Pointer(&buf[0]))

		// 检查感兴趣的事件类型
		var eventType NetworkEventType
		switch hdr.Type {
		case rtmNewAddr, rtmDelAddr:
			eventType = EventAddressAdded
		case rtmIfInfo:
			eventType = EventInterfaceUp
		case rtmAdd, rtmDelete, rtmChange:
			eventType = EventRouteChanged
		default:
			continue
		}

		// 更新地址快照并检测实际变化
		if w.hasAddressChanged() {
			event := NetworkEvent{
				Type:      eventType,
				Timestamp: time.Now(),
			}

			// 发送到事件通道（非阻塞）
			select {
			case w.events <- event:
			default:
				// 通道满，丢弃事件
			}

			// 通知回调
			w.notifyCallbacks(event)
		}
	}
}

// hasAddressChanged 检查地址是否发生变化
func (w *darwinSystemWatcher) hasAddressChanged() bool {
	currentAddrs := w.getCurrentAddresses()

	w.lastAddrsMu.Lock()
	defer w.lastAddrsMu.Unlock()

	// 检查是否有变化
	if len(currentAddrs) != len(w.lastAddrs) {
		w.lastAddrs = currentAddrs
		return true
	}

	for addr := range currentAddrs {
		if _, ok := w.lastAddrs[addr]; !ok {
			w.lastAddrs = currentAddrs
			return true
		}
	}

	return false
}

// getCurrentAddresses 获取当前所有网络地址
func (w *darwinSystemWatcher) getCurrentAddresses() map[string]struct{} {
	addrs := make(map[string]struct{})

	ifaces, err := net.Interfaces()
	if err != nil {
		return addrs
	}

	for _, iface := range ifaces {
		// 跳过 loopback 和 down 的接口
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		ifaceAddrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range ifaceAddrs {
			addrs[addr.String()] = struct{}{}
		}
	}

	return addrs
}

// updateAddressSnapshot 更新地址快照
func (w *darwinSystemWatcher) updateAddressSnapshot() {
	addrs := w.getCurrentAddresses()
	w.lastAddrsMu.Lock()
	w.lastAddrs = addrs
	w.lastAddrsMu.Unlock()
}

// notifyCallbacks 通知所有回调
func (w *darwinSystemWatcher) notifyCallbacks(event NetworkEvent) {
	w.callbacksMu.RLock()
	callbacks := make([]func(NetworkEvent), len(w.callbacks))
	copy(callbacks, w.callbacks)
	w.callbacksMu.RUnlock()

	for _, cb := range callbacks {
		go cb(event)
	}
}

// startPollingFallback 启动轮询回退模式
func (w *darwinSystemWatcher) startPollingFallback(ctx context.Context) error {
	w.updateAddressSnapshot()

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()

		pollInterval := w.config.PollInterval
		if pollInterval <= 0 {
			pollInterval = 5 * time.Second
		}

		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-w.ctx.Done():
				return
			case <-ticker.C:
				if !w.running.Load() {
					return
				}
				if w.hasAddressChanged() {
					event := NetworkEvent{
						Type:      EventAddressAdded,
						Timestamp: time.Now(),
					}

					// 发送到事件通道（非阻塞）
					select {
					case w.events <- event:
					default:
					}

					w.notifyCallbacks(event)
				}
			}
		}
	}()

	return nil
}
