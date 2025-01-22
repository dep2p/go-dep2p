package autonatv2

import "time"

// autoNATSettings 用于配置 AutoNAT
type autoNATSettings struct {
	allowPrivateAddrs                    bool                  // 是否允许私有地址
	serverRPM                            int                   // 服务器每分钟请求数限制
	serverPerPeerRPM                     int                   // 每个对等节点每分钟请求数限制
	serverDialDataRPM                    int                   // 服务器每分钟拨号数据请求限制
	dataRequestPolicy                    dataRequestPolicyFunc // 数据请求策略函数
	now                                  func() time.Time      // 获取当前时间的函数
	amplificatonAttackPreventionDialWait time.Duration         // 防止放大攻击的拨号等待时间
	metricsTracer                        MetricsTracer         // 指标追踪器
}

// defaultSettings 返回默认的 AutoNAT 设置
//
// 返回值:
//   - *autoNATSettings 默认的 AutoNAT 设置对象
func defaultSettings() *autoNATSettings {
	return &autoNATSettings{
		allowPrivateAddrs:                    false,
		serverRPM:                            60, // 每秒1次
		serverPerPeerRPM:                     12, // 每5秒1次
		serverDialDataRPM:                    12, // 每5秒1次
		dataRequestPolicy:                    amplificationAttackPrevention,
		amplificatonAttackPreventionDialWait: 3 * time.Second,
		now:                                  time.Now,
	}
}

// AutoNATOption 定义了修改 AutoNAT 设置的函数类型
type AutoNATOption func(s *autoNATSettings) error

// WithServerRateLimit 设置服务器的速率限制
//
// 参数:
//   - rpm: int 服务器每分钟请求数限制
//   - perPeerRPM: int 每个对等节点每分钟请求数限制
//   - dialDataRPM: int 服务器每分钟拨号数据请求限制
//
// 返回值:
//   - AutoNATOption 用于修改设置的函数
func WithServerRateLimit(rpm, perPeerRPM, dialDataRPM int) AutoNATOption {
	return func(s *autoNATSettings) error {
		s.serverRPM = rpm
		s.serverPerPeerRPM = perPeerRPM
		s.serverDialDataRPM = dialDataRPM
		return nil
	}
}

// WithMetricsTracer 设置指标追踪器
//
// 参数:
//   - m: MetricsTracer 指标追踪器接口
//
// 返回值:
//   - AutoNATOption 用于修改设置的函数
func WithMetricsTracer(m MetricsTracer) AutoNATOption {
	return func(s *autoNATSettings) error {
		s.metricsTracer = m
		return nil
	}
}

// withDataRequestPolicy 设置数据请求策略
//
// 参数:
//   - drp: dataRequestPolicyFunc 数据请求策略函数
//
// 返回值:
//   - AutoNATOption 用于修改设置的函数
func withDataRequestPolicy(drp dataRequestPolicyFunc) AutoNATOption {
	return func(s *autoNATSettings) error {
		s.dataRequestPolicy = drp
		return nil
	}
}

// allowPrivateAddrs 允许私有地址
//
// 参数:
//   - s: *autoNATSettings AutoNAT 设置对象
//
// 返回值:
//   - error 错误信息，总是返回 nil
func allowPrivateAddrs(s *autoNATSettings) error {
	s.allowPrivateAddrs = true
	return nil
}

// withAmplificationAttackPreventionDialWait 设置防止放大攻击的拨号等待时间
//
// 参数:
//   - d: time.Duration 等待时间
//
// 返回值:
//   - AutoNATOption 用于修改设置的函数
func withAmplificationAttackPreventionDialWait(d time.Duration) AutoNATOption {
	return func(s *autoNATSettings) error {
		s.amplificatonAttackPreventionDialWait = d
		return nil
	}
}
