package identify

import (
	"github.com/dep2p/go-dep2p/core/event"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/p2p/metricshelper"

	"github.com/prometheus/client_golang/prometheus"
)

// metricNamespace 是 identify 指标的命名空间
const metricNamespace = "dep2p_identify"

var (
	// pushesTriggered 统计由事件触发的 identify push 次数
	pushesTriggered = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "identify_pushes_triggered_total",
			Help:      "触发的 Push 总数",
		},
		[]string{"trigger"},
	)

	// identify 统计 identify 请求和响应的次数
	identify = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "identify_total",
			Help:      "Identify 总数",
		},
		[]string{"dir"},
	)

	// identifyPush 统计 identify push 请求和响应的次数
	identifyPush = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "identify_push_total",
			Help:      "Identify Push 总数",
		},
		[]string{"dir"},
	)

	// connPushSupportTotal 统计支持 push 的连接数
	connPushSupportTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "conn_push_support_total",
			Help:      "支持 Identify Push 的连接总数",
		},
		[]string{"support"},
	)

	// protocolsCount 记录当前支持的协议数量
	protocolsCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "protocols_count",
			Help:      "协议数量",
		},
	)

	// addrsCount 记录当前地址数量
	addrsCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "addrs_count",
			Help:      "地址数量",
		},
	)

	// numProtocolsReceived 统计收到的协议数量分布
	numProtocolsReceived = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: metricNamespace,
			Name:      "protocols_received",
			Help:      "收到的协议数量",
			Buckets:   buckets,
		},
	)

	// numAddrsReceived 统计收到的地址数量分布
	numAddrsReceived = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: metricNamespace,
			Name:      "addrs_received",
			Help:      "收到的地址数量",
			Buckets:   buckets,
		},
	)

	// collectors 包含所有需要注册的指标收集器
	collectors = []prometheus.Collector{
		pushesTriggered,
		identify,
		identifyPush,
		connPushSupportTotal,
		protocolsCount,
		addrsCount,
		numProtocolsReceived,
		numAddrsReceived,
	}

	// buckets 定义直方图的桶,1-20 每个数字一个桶,然后 25-100 每 5 个数字一个桶
	buckets = append(
		prometheus.LinearBuckets(1, 1, 20),
		prometheus.LinearBuckets(25, 5, 16)...,
	)
)

// MetricsTracer 定义了指标跟踪器的接口
type MetricsTracer interface {
	// TriggeredPushes 统计由事件触发的 IdentifyPush 次数
	TriggeredPushes(event any)

	// ConnPushSupport 按 Push 支持情况统计对等节点
	ConnPushSupport(identifyPushSupport)

	// IdentifyReceived 跟踪接收 identify 响应的指标
	IdentifyReceived(isPush bool, numProtocols int, numAddrs int)

	// IdentifySent 跟踪发送 identify 响应的指标
	IdentifySent(isPush bool, numProtocols int, numAddrs int)
}

// metricsTracer 实现了 MetricsTracer 接口
type metricsTracer struct{}

// 确保 metricsTracer 实现了 MetricsTracer 接口
var _ MetricsTracer = &metricsTracer{}

// metricsTracerSetting 包含指标跟踪器的配置
type metricsTracerSetting struct {
	reg prometheus.Registerer
}

// MetricsTracerOption 定义了配置指标跟踪器的函数类型
type MetricsTracerOption func(*metricsTracerSetting)

// WithRegisterer 设置指标注册器
// 参数:
//   - reg: prometheus.Registerer 指标注册器
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

// NewMetricsTracer 创建新的指标跟踪器
// 参数:
//   - opts: ...MetricsTracerOption 配置选项
//
// 返回值:
//   - MetricsTracer 指标跟踪器实例
func NewMetricsTracer(opts ...MetricsTracerOption) MetricsTracer {
	// 创建默认配置
	setting := &metricsTracerSetting{reg: prometheus.DefaultRegisterer}
	// 应用配置选项
	for _, opt := range opts {
		opt(setting)
	}
	// 注册所有收集器
	metricshelper.RegisterCollectors(setting.reg, collectors...)
	return &metricsTracer{}
}

// TriggeredPushes 实现 MetricsTracer 接口
// 参数:
//   - ev: any 触发 push 的事件
func (t *metricsTracer) TriggeredPushes(ev any) {
	// 获取标签切片
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)

	// 根据事件类型设置标签
	typ := "unknown"
	switch ev.(type) {
	case event.EvtLocalProtocolsUpdated:
		typ = "protocols_updated"
	case event.EvtLocalAddressesUpdated:
		typ = "addresses_updated"
	}
	*tags = append(*tags, typ)
	// 增加计数
	pushesTriggered.WithLabelValues(*tags...).Inc()
}

// IncrementPushSupport 增加 push 支持计数
// 参数:
//   - s: identifyPushSupport push 支持类型
func (t *metricsTracer) IncrementPushSupport(s identifyPushSupport) {
	// 获取标签切片
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)

	// 添加支持类型标签
	*tags = append(*tags, getPushSupport(s))
	// 增加计数
	connPushSupportTotal.WithLabelValues(*tags...).Inc()
}

// IdentifySent 实现 MetricsTracer 接口
// 参数:
//   - isPush: bool 是否为 push 类型
//   - numProtocols: int 协议数量
//   - numAddrs: int 地址数量
func (t *metricsTracer) IdentifySent(isPush bool, numProtocols int, numAddrs int) {
	// 获取标签切片
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)

	// 根据类型增加相应计数
	if isPush {
		*tags = append(*tags, metricshelper.GetDirection(network.DirOutbound))
		identifyPush.WithLabelValues(*tags...).Inc()
	} else {
		*tags = append(*tags, metricshelper.GetDirection(network.DirInbound))
		identify.WithLabelValues(*tags...).Inc()
	}

	// 更新协议和地址数量
	protocolsCount.Set(float64(numProtocols))
	addrsCount.Set(float64(numAddrs))
}

// IdentifyReceived 实现 MetricsTracer 接口
// 参数:
//   - isPush: bool 是否为 push 类型
//   - numProtocols: int 协议数量
//   - numAddrs: int 地址数量
func (t *metricsTracer) IdentifyReceived(isPush bool, numProtocols int, numAddrs int) {
	// 获取标签切片
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)

	// 根据类型增加相应计数
	if isPush {
		*tags = append(*tags, metricshelper.GetDirection(network.DirInbound))
		identifyPush.WithLabelValues(*tags...).Inc()
	} else {
		*tags = append(*tags, metricshelper.GetDirection(network.DirOutbound))
		identify.WithLabelValues(*tags...).Inc()
	}

	// 记录收到的协议和地址数量
	numProtocolsReceived.Observe(float64(numProtocols))
	numAddrsReceived.Observe(float64(numAddrs))
}

// ConnPushSupport 实现 MetricsTracer 接口
// 参数:
//   - support: identifyPushSupport push 支持类型
func (t *metricsTracer) ConnPushSupport(support identifyPushSupport) {
	// 获取标签切片
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)

	// 添加支持类型标签并增加计数
	*tags = append(*tags, getPushSupport(support))
	connPushSupportTotal.WithLabelValues(*tags...).Inc()
}

// getPushSupport 获取 push 支持类型的字符串表示
// 参数:
//   - s: identifyPushSupport push 支持类型
//
// 返回值:
//   - string 支持类型的字符串表示
func getPushSupport(s identifyPushSupport) string {
	switch s {
	case identifyPushSupported:
		return "supported"
	case identifyPushUnsupported:
		return "not supported"
	default:
		return "unknown"
	}
}
