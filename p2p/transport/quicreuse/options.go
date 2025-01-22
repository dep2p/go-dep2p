package quicreuse

import "github.com/prometheus/client_golang/prometheus"

// Option 定义了一个函数类型,用于配置 ConnManager
// 参数:
//   - *ConnManager: 连接管理器指针
//
// 返回值:
//   - error: 配置过程中的错误信息
type Option func(*ConnManager) error

// DisableReuseport 禁用端口重用功能
// 返回值:
//   - Option: 返回一个配置函数,用于禁用端口重用
func DisableReuseport() Option {
	return func(m *ConnManager) error {
		// 设置端口重用标志为 false
		m.enableReuseport = false
		return nil
	}
}

// EnableMetrics 启用 Prometheus 指标收集
// 参数:
//   - reg: prometheus.Registerer Prometheus 注册器
//
// 返回值:
//   - Option: 返回一个配置函数,用于启用指标收集
//
// 注意:
//   - 如果 reg 为 nil,将使用 prometheus.DefaultRegisterer 作为注册器
func EnableMetrics(reg prometheus.Registerer) Option {
	return func(m *ConnManager) error {
		// 启用指标收集
		m.enableMetrics = true
		// 如果提供了注册器,则使用提供的注册器
		if reg != nil {
			m.registerer = reg
		}
		return nil
	}
}
