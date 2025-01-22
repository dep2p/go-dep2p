//go:build cgo && !nowatchdog

package connmgr

import "github.com/raulk/go-watchdog"

// registerWatchdog 注册一个GC后的通知回调函数
// 参数:
//   - cb: func(),GC后要执行的回调函数
//
// 返回值:
//   - unregister: func(),用于注销通知的函数
func registerWatchdog(cb func()) (unregister func()) {
	// 注册GC后的通知回调函数,并返回注销函数
	return watchdog.RegisterPostGCNotifee(cb)
}

// WithEmergencyTrim 用于在内存紧急情况下启用连接修剪的选项
// 参数:
//   - enable: bool,是否启用紧急修剪
//
// 返回值:
//   - Option: 配置选项函数
func WithEmergencyTrim(enable bool) Option {
	return func(cfg *config) error {
		// 设置是否启用紧急修剪
		cfg.emergencyTrim = enable
		return nil
	}
}
