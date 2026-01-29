//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !windows
// +build !darwin,!linux,!freebsd,!openbsd,!netbsd,!windows

package watcher

import (
	"context"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// stubInterfaceWatcher Stub 实现，不支持主动检测
type stubInterfaceWatcher struct {
	events chan InterfaceEvent
}

// newInterfaceWatcher 创建平台特定的接口监控器
func newInterfaceWatcher() (InterfaceWatcher, error) {
	logger.Warn("当前平台不支持网络接口主动监控，请使用 NotifyChange() 外部通知")
	
	return &stubInterfaceWatcher{
		events: make(chan InterfaceEvent, 10),
	}, nil
}

// Watch 开始监控网络接口变化（stub 实现不会发送事件）
func (w *stubInterfaceWatcher) Watch(ctx context.Context) (<-chan InterfaceEvent, error) {
	// 返回空通道，不发送任何事件
	// 用户需要通过 NotifyChange() 手动触发
	return w.events, nil
}

// CurrentState 获取当前网络接口状态
func (w *stubInterfaceWatcher) CurrentState() (pkgif.NetworkState, error) {
	return getCurrentNetworkState()
}

// Stop 停止监控
func (w *stubInterfaceWatcher) Stop() error {
	return nil
}
