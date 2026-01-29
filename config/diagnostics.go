package config

// DiagnosticsConfig 诊断服务配置
type DiagnosticsConfig struct {
	// EnableIntrospect 启用自省服务
	EnableIntrospect bool `json:"enable_introspect" yaml:"enable_introspect"`

	// IntrospectAddr 自省服务监听地址
	// 默认 "127.0.0.1:6060"
	IntrospectAddr string `json:"introspect_addr" yaml:"introspect_addr"`
}

// DefaultDiagnosticsConfig 返回默认诊断配置
func DefaultDiagnosticsConfig() DiagnosticsConfig {
	return DiagnosticsConfig{
		EnableIntrospect: false, // 默认禁用
		IntrospectAddr:   "127.0.0.1:6060",
	}
}
