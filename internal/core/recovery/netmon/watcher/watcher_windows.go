//go:build windows
// +build windows

package watcher

import (
	"context"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// windowsInterfaceWatcher Windows 平台的网络接口监控器
type windowsInterfaceWatcher struct {
	events chan InterfaceEvent
	stopCh chan struct{}
}

// newInterfaceWatcher 创建平台特定的接口监控器
func newInterfaceWatcher() (InterfaceWatcher, error) {
	return &windowsInterfaceWatcher{
		events: make(chan InterfaceEvent, 10),
		stopCh: make(chan struct{}),
	}, nil
}

// Watch 开始监控网络接口变化
func (w *windowsInterfaceWatcher) Watch(ctx context.Context) (<-chan InterfaceEvent, error) {
	// 启动轮询监控
	// 完整实现应使用 Windows IP Helper API
	go w.pollLoop(ctx)
	
	return w.events, nil
}

// CurrentState 获取当前网络接口状态
func (w *windowsInterfaceWatcher) CurrentState() (pkgif.NetworkState, error) {
	return getCurrentNetworkState()
}

// Stop 停止监控
func (w *windowsInterfaceWatcher) Stop() error {
	close(w.stopCh)
	return nil
}

// pollLoop 轮询监控循环
func (w *windowsInterfaceWatcher) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var lastState pkgif.NetworkState
	lastState, _ = w.CurrentState()

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.stopCh:
			return
		case <-ticker.C:
			currentState, err := w.CurrentState()
			if err != nil {
				logger.Warn("获取网络状态失败", "err", err)
				continue
			}

			// 检测变化
			if w.hasChanged(lastState, currentState) {
				// 发送变化事件
				event := InterfaceEvent{
					Type:      InterfaceChanged,
					Timestamp: time.Now(),
				}
				
				select {
				case w.events <- event:
				default:
					logger.Warn("事件通道已满")
				}
			}

			lastState = currentState
		}
	}
}

// hasChanged 检查状态是否发生变化
func (w *windowsInterfaceWatcher) hasChanged(old, new pkgif.NetworkState) bool {
	if len(old.Interfaces) != len(new.Interfaces) {
		return true
	}

	oldNames := make(map[string]bool)
	for _, iface := range old.Interfaces {
		oldNames[iface.Name] = true
	}

	for _, iface := range new.Interfaces {
		if !oldNames[iface.Name] {
			return true
		}
	}

	oldAddrs := extractAddrs(old.Interfaces)
	newAddrs := extractAddrs(new.Interfaces)

	return !equalAddrs(oldAddrs, newAddrs)
}
