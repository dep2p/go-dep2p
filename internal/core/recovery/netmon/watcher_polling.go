// Package netmon 提供网络状态监控功能
//
// IMPL-NETWORK-RESILIENCE: 基于轮询的跨平台网络监听
package netmon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
//                              PollingWatcher
// ============================================================================

// PollingWatcher 基于轮询的网络变化监听器
//
// IMPL-NETWORK-RESILIENCE: 跨平台实现，使用标准库 net.Interfaces()
// Phase 5.2: 支持检测系统网络接口变化
type PollingWatcher struct {
	mu sync.RWMutex

	// 配置
	config *WatcherConfig

	// 事件通道
	events chan NetworkEvent

	// 上次网络指纹
	lastFingerprint string

	// 上次接口信息（用于检测具体变化）
	lastInterfaces map[string]interfaceInfo

	// 运行状态
	running atomic.Bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// interfaceInfo 接口信息
type interfaceInfo struct {
	Name      string
	HWAddr    string
	Flags     net.Flags
	Addresses []string
}

// NewPollingWatcher 创建轮询监听器
func NewPollingWatcher(config *WatcherConfig) *PollingWatcher {
	if config == nil {
		config = DefaultWatcherConfig()
	}
	_ = config.Validate()

	return &PollingWatcher{
		config:         config,
		events:         make(chan NetworkEvent, config.EventBufferSize),
		lastInterfaces: make(map[string]interfaceInfo),
	}
}

// Start 启动监听
func (w *PollingWatcher) Start(ctx context.Context) error {
	if !w.running.CompareAndSwap(false, true) {
		return nil // 已在运行
	}

	w.ctx, w.cancel = context.WithCancel(ctx)

	// 初始化指纹
	w.mu.Lock()
	w.lastFingerprint = w.getNetworkFingerprint()
	w.lastInterfaces = w.getInterfacesInfo()
	w.mu.Unlock()

	// 启动轮询循环
	w.wg.Add(1)
	go w.pollLoop()

	logger.Info("网络变化监听器已启动",
		"poll_interval", w.config.PollInterval)

	return nil
}

// Stop 停止监听
func (w *PollingWatcher) Stop() error {
	if !w.running.CompareAndSwap(true, false) {
		return nil // 未运行
	}

	if w.cancel != nil {
		w.cancel()
	}

	w.wg.Wait()

	logger.Info("网络变化监听器已停止")
	return nil
}

// Events 返回事件通道
func (w *PollingWatcher) Events() <-chan NetworkEvent {
	return w.events
}

// IsRunning 检查是否运行
func (w *PollingWatcher) IsRunning() bool {
	return w.running.Load()
}

// ============================================================================
//                              轮询逻辑
// ============================================================================

// pollLoop 轮询循环
func (w *PollingWatcher) pollLoop() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.checkNetworkChange()
		}
	}
}

// checkNetworkChange 检查网络变化
func (w *PollingWatcher) checkNetworkChange() {
	currentFingerprint := w.getNetworkFingerprint()
	currentInterfaces := w.getInterfacesInfo()

	w.mu.Lock()
	lastFingerprint := w.lastFingerprint
	lastInterfaces := w.lastInterfaces
	w.lastFingerprint = currentFingerprint
	w.lastInterfaces = currentInterfaces
	w.mu.Unlock()

	// 如果指纹相同，无变化
	if currentFingerprint == lastFingerprint {
		return
	}

	oldFP := lastFingerprint
	if len(oldFP) > 8 {
		oldFP = oldFP[:8]
	}
	newFP := currentFingerprint
	if len(newFP) > 8 {
		newFP = newFP[:8]
	}

	logger.Debug("检测到网络变化",
		"old_fingerprint", oldFP,
		"new_fingerprint", newFP)

	// 检测具体变化
	events := w.detectChanges(lastInterfaces, currentInterfaces)

	// 发送事件
	for _, event := range events {
		select {
		case w.events <- event:
			logger.Debug("发送网络事件",
				"type", event.Type.String(),
				"interface", event.Interface)
		default:
			logger.Warn("网络事件缓冲区已满，丢弃事件",
				"type", event.Type.String())
		}
	}

	// 如果没有检测到具体变化，发送通用变化事件
	if len(events) == 0 {
		select {
		case w.events <- NetworkEvent{
			Type:      EventNetworkChanged,
			Timestamp: time.Now(),
		}:
		default:
		}
	}
}

// detectChanges 检测具体变化
func (w *PollingWatcher) detectChanges(old, new map[string]interfaceInfo) []NetworkEvent {
	var events []NetworkEvent
	now := time.Now()

	// 检查新增/删除的接口
	for name, newInfo := range new {
		oldInfo, existed := old[name]

		if !existed {
			// 新增接口
			events = append(events, NetworkEvent{
				Type:      EventInterfaceUp,
				Interface: name,
				Timestamp: now,
			})
			continue
		}

		// 检查接口状态变化
		wasUp := oldInfo.Flags&net.FlagUp != 0
		isUp := newInfo.Flags&net.FlagUp != 0

		if !wasUp && isUp {
			events = append(events, NetworkEvent{
				Type:      EventInterfaceUp,
				Interface: name,
				Timestamp: now,
			})
		} else if wasUp && !isUp {
			events = append(events, NetworkEvent{
				Type:      EventInterfaceDown,
				Interface: name,
				Timestamp: now,
			})
		}

		// 检查地址变化
		oldAddrs := make(map[string]bool)
		for _, addr := range oldInfo.Addresses {
			oldAddrs[addr] = true
		}

		newAddrs := make(map[string]bool)
		for _, addr := range newInfo.Addresses {
			newAddrs[addr] = true
		}

		// 新增地址
		for addr := range newAddrs {
			if !oldAddrs[addr] {
				events = append(events, NetworkEvent{
					Type:      EventAddressAdded,
					Interface: name,
					Address:   addr,
					Timestamp: now,
				})
			}
		}

		// 删除地址
		for addr := range oldAddrs {
			if !newAddrs[addr] {
				events = append(events, NetworkEvent{
					Type:      EventAddressRemoved,
					Interface: name,
					Address:   addr,
					Timestamp: now,
				})
			}
		}
	}

	// 检查删除的接口
	for name := range old {
		if _, exists := new[name]; !exists {
			events = append(events, NetworkEvent{
				Type:      EventInterfaceDown,
				Interface: name,
				Timestamp: now,
			})
		}
	}

	return events
}

// ============================================================================
//                              网络指纹
// ============================================================================

// getNetworkFingerprint 获取网络指纹
// 基于所有网络接口和地址计算哈希
func (w *PollingWatcher) getNetworkFingerprint() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	var parts []string

	for _, iface := range ifaces {
		// 跳过 loopback 和未启用的接口
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		part := iface.Name + ":" + iface.HardwareAddr.String() + ":" + iface.Flags.String()

		// 获取地址
		addrs, err := iface.Addrs()
		if err == nil {
			var addrStrs []string
			for _, addr := range addrs {
				addrStrs = append(addrStrs, addr.String())
			}
			sort.Strings(addrStrs)
			part += ":[" + strings.Join(addrStrs, ",") + "]"
		}

		parts = append(parts, part)
	}

	sort.Strings(parts)
	data := strings.Join(parts, "|")

	h := sha256.Sum256([]byte(data))
	return hex.EncodeToString(h[:])
}

// getInterfacesInfo 获取接口信息
func (w *PollingWatcher) getInterfacesInfo() map[string]interfaceInfo {
	result := make(map[string]interfaceInfo)

	ifaces, err := net.Interfaces()
	if err != nil {
		return result
	}

	for _, iface := range ifaces {
		// 跳过 loopback
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		info := interfaceInfo{
			Name:   iface.Name,
			HWAddr: iface.HardwareAddr.String(),
			Flags:  iface.Flags,
		}

		addrs, err := iface.Addrs()
		if err == nil {
			for _, addr := range addrs {
				info.Addresses = append(info.Addresses, addr.String())
			}
		}

		result[iface.Name] = info
	}

	return result
}

// ============================================================================
//                              工厂函数
// ============================================================================

// NewSystemWatcher 创建系统监听器
//
// IMPL-NETWORK-RESILIENCE: 根据配置和平台选择合适的实现
// Phase 5.2: 优先使用平台原生实现，否则回退到轮询实现
func NewSystemWatcher(config *WatcherConfig) SystemWatcher {
	if config == nil {
		config = DefaultWatcherConfig()
	}

	if !config.Enabled {
		return NewNoOpWatcher()
	}

	// 优先使用平台原生（事件驱动）实现；否则回退到轮询实现
	if native := newNativeSystemWatcher(config); native != nil {
		return native
	}

	return NewPollingWatcher(config)
}
