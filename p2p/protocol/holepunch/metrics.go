package holepunch

import (
	"github.com/dep2p/go-dep2p/core/network"
	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	"github.com/dep2p/go-dep2p/p2p/metricshelper"
	"github.com/prometheus/client_golang/prometheus"
)

// 用于 dep2p 打洞指标的命名空间
const metricNamespace = "dep2p_holepunch"

var (
	// 直接拨号总数指标
	directDialsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "direct_dials_total",
			Help:      "直接拨号总数",
		},
		[]string{"outcome"},
	)

	// 按传输协议统计的打洞结果指标
	hpAddressOutcomesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "address_outcomes_total",
			Help:      "按传输协议统计的打洞结果",
		},
		[]string{"side", "num_attempts", "ipv", "transport", "outcome"},
	)

	// 整体打洞结果指标
	hpOutcomesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "outcomes_total",
			Help:      "整体打洞结果",
		},
		[]string{"side", "num_attempts", "outcome"},
	)

	// 收集器列表
	collectors = []prometheus.Collector{
		directDialsTotal,
		hpAddressOutcomesTotal,
		hpOutcomesTotal,
	}
)

// MetricsTracer 定义了打洞指标追踪器的接口
type MetricsTracer interface {
	// HolePunchFinished 记录打洞完成的指标
	HolePunchFinished(side string, attemptNum int, theirAddrs []ma.Multiaddr, ourAddr []ma.Multiaddr, directConn network.ConnMultiaddrs)
	// DirectDialFinished 记录直接拨号完成的指标
	DirectDialFinished(success bool)
}

// metricsTracer 实现了 MetricsTracer 接口
type metricsTracer struct{}

var _ MetricsTracer = &metricsTracer{}

// metricsTracerSetting 包含指标追踪器的配置
type metricsTracerSetting struct {
	reg prometheus.Registerer
}

// MetricsTracerOption 定义了配置指标追踪器的函数类型
type MetricsTracerOption func(*metricsTracerSetting)

// WithRegisterer 返回一个设置注册器的选项函数
// 参数:
//   - reg: prometheus.Registerer Prometheus 注册器
//
// 返回值:
//   - MetricsTracerOption 配置函数
func WithRegisterer(reg prometheus.Registerer) MetricsTracerOption {
	return func(s *metricsTracerSetting) {
		if reg != nil {
			s.reg = reg
		}
	}
}

// NewMetricsTracer 创建一个新的指标追踪器
// 参数:
//   - opts: ...MetricsTracerOption 配置选项
//
// 返回值:
//   - MetricsTracer 指标追踪器实例
func NewMetricsTracer(opts ...MetricsTracerOption) MetricsTracer {
	setting := &metricsTracerSetting{reg: prometheus.DefaultRegisterer}
	for _, opt := range opts {
		opt(setting)
	}
	metricshelper.RegisterCollectors(setting.reg, collectors...)
	// 初始化指标标签,以确保第一个数据点被正确处理
	for _, side := range []string{"initiator", "receiver"} {
		for _, numAttempts := range []string{"1", "2", "3", "4"} {
			for _, outcome := range []string{"success", "failed", "cancelled", "no_suitable_address"} {
				for _, ipv := range []string{"ip4", "ip6"} {
					for _, transport := range []string{"quic", "quic-v1", "tcp", "webtransport"} {
						hpAddressOutcomesTotal.WithLabelValues(side, numAttempts, ipv, transport, outcome)
					}
				}
				if outcome == "cancelled" {
					// 对整体打洞指标来说不是有效的结果
					continue
				}
				hpOutcomesTotal.WithLabelValues(side, numAttempts, outcome)
			}
		}
	}
	return &metricsTracer{}
}

// HolePunchFinished 追踪打洞完成的指标。指标在打洞尝试级别和参与打洞的各个地址级别进行追踪。
//
// 地址的结果计算如下:
//
//   - success (成功):
//     使用此地址与对等节点建立了直接连接
//   - cancelled (取消):
//     与对等节点建立了直接连接,但不是使用此地址
//   - failed (失败):
//     未能与对等节点建立直接连接,且对等节点报告了具有相同传输协议的地址
//   - no_suitable_address (无合适地址):
//     对等节点未报告具有相同传输协议的地址
func (mt *metricsTracer) HolePunchFinished(side string, numAttempts int,
	remoteAddrs []ma.Multiaddr, localAddrs []ma.Multiaddr, directConn network.ConnMultiaddrs) {
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)

	*tags = append(*tags, side, getNumAttemptString(numAttempts))
	var dipv, dtransport string
	if directConn != nil {
		dipv = metricshelper.GetIPVersion(directConn.LocalMultiaddr())
		dtransport = metricshelper.GetTransport(directConn.LocalMultiaddr())
	}

	matchingAddressCount := 0
	// 计算所有参与地址的打洞结果
	for _, la := range localAddrs {
		lipv := metricshelper.GetIPVersion(la)
		ltransport := metricshelper.GetTransport(la)

		matchingAddress := false
		for _, ra := range remoteAddrs {
			ripv := metricshelper.GetIPVersion(ra)
			rtransport := metricshelper.GetTransport(ra)
			if ripv == lipv && rtransport == ltransport {
				// 对等节点报告了具有相同传输协议的地址
				matchingAddress = true
				matchingAddressCount++

				*tags = append(*tags, ripv, rtransport)
				if directConn != nil && dipv == ripv && dtransport == rtransport {
					// 使用此地址建立了连接
					*tags = append(*tags, "success")
				} else if directConn != nil {
					// 建立了连接但不是使用此地址
					*tags = append(*tags, "cancelled")
				} else {
					// 未建立连接
					*tags = append(*tags, "failed")
				}
				hpAddressOutcomesTotal.WithLabelValues(*tags...).Inc()
				*tags = (*tags)[:2] // 保留 (side, numAttempts)
				break
			}
		}
		if !matchingAddress {
			*tags = append(*tags, lipv, ltransport, "no_suitable_address")
			hpAddressOutcomesTotal.WithLabelValues(*tags...).Inc()
			*tags = (*tags)[:2] // 保留 (side, numAttempts)
		}
	}

	outcome := "failed"
	if directConn != nil {
		outcome = "success"
	} else if matchingAddressCount == 0 {
		// 没有匹配的地址,此次尝试必然失败
		outcome = "no_suitable_address"
	}

	*tags = append(*tags, outcome)
	hpOutcomesTotal.WithLabelValues(*tags...).Inc()
}

// getNumAttemptString 将尝试次数转换为字符串表示
// 参数:
//   - numAttempt: int 尝试次数
//
// 返回值:
//   - string 尝试次数的字符串表示
func getNumAttemptString(numAttempt int) string {
	var attemptStr = [...]string{"0", "1", "2", "3", "4", "5"}
	if numAttempt > 5 {
		return "> 5"
	}
	return attemptStr[numAttempt]
}

// DirectDialFinished 记录直接拨号完成的指标
// 参数:
//   - success: bool 拨号是否成功
func (mt *metricsTracer) DirectDialFinished(success bool) {
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)
	if success {
		*tags = append(*tags, "success")
	} else {
		*tags = append(*tags, "failed")
	}
	directDialsTotal.WithLabelValues(*tags...).Inc()
}
