// Package netmon 提供网络状态监控功能
//
//go:build !darwin
// +build !darwin

package netmon

// newNativeSystemWatcher 创建平台原生监听器（stub 实现）
//
// 在非 macOS 平台上，返回 nil 表示不支持原生监听。
// 将回退到使用 PollingWatcher。
func newNativeSystemWatcher(_ *WatcherConfig) SystemWatcher {
	return nil
}
