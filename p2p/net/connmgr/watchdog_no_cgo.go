//go:build !cgo || nowatchdog

package connmgr

// registerWatchdog 注册一个看门狗函数
// 参数:
//   - f func(): 要注册的看门狗函数
//
// 返回值:
//   - unregister func(): 注销看门狗函数的方法
func registerWatchdog(func()) (unregister func()) {
	// 由于平台不支持看门狗,直接返回nil
	return nil
}

// WithEmergencyTrim 是一个选项,用于启用内存紧急情况下的连接修剪功能
// 参数:
//   - enable bool: 是否启用紧急修剪
//
// 返回值:
//   - Option: 配置选项函数
func WithEmergencyTrim(enable bool) Option {
	return func(cfg *config) error {
		// 记录警告日志,提示平台不支持看门狗功能
		log.Warn("平台不支持 go-watchdog")
		// 由于不支持,直接返回nil
		return nil
	}
}
