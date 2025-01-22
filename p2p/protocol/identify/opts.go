package identify

// config 结构体定义了标识协议的配置选项
type config struct {
	// protocolVersion 协议版本字符串
	protocolVersion string
	// userAgent 用户代理标识字符串
	userAgent string
	// disableSignedPeerRecord 是否禁用签名的对等节点记录
	disableSignedPeerRecord bool
	// metricsTracer 指标追踪器
	metricsTracer MetricsTracer
	// disableObservedAddrManager 是否禁用观察地址管理器
	disableObservedAddrManager bool
}

// Option 是用于标识协议的选项函数类型
// 参数:
//   - *config: 配置对象指针
type Option func(*config)

// ProtocolVersion 设置用于标识对等节点所使用的协议族的协议版本字符串
// 参数:
//   - s: string 协议版本字符串
//
// 返回值:
//   - Option 配置选项函数
func ProtocolVersion(s string) Option {
	return func(cfg *config) {
		cfg.protocolVersion = s
	}
}

// UserAgent 设置此节点用于向对等节点标识自身的用户代理
// 参数:
//   - ua: string 用户代理字符串
//
// 返回值:
//   - Option 配置选项函数
func UserAgent(ua string) Option {
	return func(cfg *config) {
		cfg.userAgent = ua
	}
}

// DisableSignedPeerRecord 禁用在传出标识响应中填充签名的对等节点记录，仅发送未签名的地址
// 返回值:
//   - Option 配置选项函数
func DisableSignedPeerRecord() Option {
	return func(cfg *config) {
		cfg.disableSignedPeerRecord = true
	}
}

// WithMetricsTracer 设置指标追踪器
// 参数:
//   - tr: MetricsTracer 指标追踪器接口
//
// 返回值:
//   - Option 配置选项函数
func WithMetricsTracer(tr MetricsTracer) Option {
	return func(cfg *config) {
		cfg.metricsTracer = tr
	}
}

// DisableObservedAddrManager 禁用观察地址管理器
// 这也会有效地禁用 NAT 发射器和 EvtNATDeviceTypeChanged 事件
// 返回值:
//   - Option 配置选项函数
func DisableObservedAddrManager() Option {
	return func(cfg *config) {
		cfg.disableObservedAddrManager = true
	}
}
