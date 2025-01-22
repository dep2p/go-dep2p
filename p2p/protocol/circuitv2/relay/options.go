package relay

// Option 是一个函数类型,用于配置中继服务
type Option func(*Relay) error

// WithResources 设置中继服务的具体资源配置
// 参数:
//   - rc: Resources 资源配置对象
//
// 返回值:
//   - Option 返回一个配置函数
func WithResources(rc Resources) Option {
	return func(r *Relay) error {
		// 设置资源配置
		r.rc = rc
		return nil
	}
}

// WithLimit 仅设置中继连接的限制
// 参数:
//   - limit: *RelayLimit 连接限制配置
//
// 返回值:
//   - Option 返回一个配置函数
func WithLimit(limit *RelayLimit) Option {
	return func(r *Relay) error {
		// 设置连接限制
		r.rc.Limit = limit
		return nil
	}
}

// WithInfiniteLimits 禁用所有限制
// 返回值:
//   - Option 返回一个配置函数
func WithInfiniteLimits() Option {
	return func(r *Relay) error {
		// 将限制设为nil表示无限制
		r.rc.Limit = nil
		return nil
	}
}

// WithACL 设置访问控制过滤器
// 参数:
//   - acl: ACLFilter 访问控制过滤器
//
// 返回值:
//   - Option 返回一个配置函数
func WithACL(acl ACLFilter) Option {
	return func(r *Relay) error {
		// 设置访问控制过滤器
		r.acl = acl
		return nil
	}
}

// WithMetricsTracer 设置指标追踪器
// 参数:
//   - mt: MetricsTracer 指标追踪器
//
// 返回值:
//   - Option 返回一个配置函数
func WithMetricsTracer(mt MetricsTracer) Option {
	return func(r *Relay) error {
		// 设置指标追踪器
		r.metricsTracer = mt
		return nil
	}
}
