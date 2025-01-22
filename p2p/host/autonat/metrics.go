package autonat

import (
	"time"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/p2p/host/autonat/pb"
	"github.com/dep2p/go-dep2p/p2p/metricshelper"
	"github.com/prometheus/client_golang/prometheus"
)

// 指标命名空间
const metricNamespace = "libp2p_autonat"

var (
	// 节点可达性状态指标
	reachabilityStatus = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "reachability_status",
			Help:      "当前节点的可达性状态",
		},
	)
	// 节点可达性状态置信度指标
	reachabilityStatusConfidence = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "reachability_status_confidence",
			Help:      "节点可达性状态的置信度",
		},
	)
	// 客户端收到的回拨响应计数器
	receivedDialResponseTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "received_dial_response_total",
			Help:      "客户端收到的回拨响应总数",
		},
		[]string{"response_status"}, // 响应状态标签
	)
	// 服务端发出的回拨响应计数器
	outgoingDialResponseTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "outgoing_dial_response_total",
			Help:      "服务端发出的回拨响应总数",
		},
		[]string{"response_status"}, // 响应状态标签
	)
	// 服务端拒绝回拨请求计数器
	outgoingDialRefusedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "outgoing_dial_refused_total",
			Help:      "服务端拒绝的回拨请求总数",
		},
		[]string{"refusal_reason"}, // 拒绝原因标签
	)
	// 下次探测时间戳指标
	nextProbeTimestamp = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "next_probe_timestamp",
			Help:      "下次探测的时间戳",
		},
	)
	// 所有指标收集器列表
	collectors = []prometheus.Collector{
		reachabilityStatus,
		reachabilityStatusConfidence,
		receivedDialResponseTotal,
		outgoingDialResponseTotal,
		outgoingDialRefusedTotal,
		nextProbeTimestamp,
	}
)

// MetricsTracer 定义了指标追踪器接口
type MetricsTracer interface {
	// ReachabilityStatus 记录可达性状态
	// 参数:
	//   - status: network.Reachability 可达性状态
	ReachabilityStatus(status network.Reachability)

	// ReachabilityStatusConfidence 记录可达性状态置信度
	// 参数:
	//   - confidence: int 置信度值
	ReachabilityStatusConfidence(confidence int)

	// ReceivedDialResponse 记录收到的回拨响应
	// 参数:
	//   - status: pb.Message_ResponseStatus 响应状态
	ReceivedDialResponse(status pb.Message_ResponseStatus)

	// OutgoingDialResponse 记录发出的回拨响应
	// 参数:
	//   - status: pb.Message_ResponseStatus 响应状态
	OutgoingDialResponse(status pb.Message_ResponseStatus)

	// OutgoingDialRefused 记录拒绝的回拨请求
	// 参数:
	//   - reason: string 拒绝原因
	OutgoingDialRefused(reason string)

	// NextProbeTime 记录下次探测时间
	// 参数:
	//   - t: time.Time 探测时间
	NextProbeTime(t time.Time)
}

// getResponseStatus 将响应状态转换为字符串
// 参数:
//   - status: pb.Message_ResponseStatus 响应状态
//
// 返回值:
//   - string: 状态对应的字符串描述
func getResponseStatus(status pb.Message_ResponseStatus) string {
	var s string
	switch status {
	case pb.Message_OK:
		s = "ok" // 成功
	case pb.Message_E_DIAL_ERROR:
		s = "dial error" // 回拨错误
	case pb.Message_E_DIAL_REFUSED:
		s = "dial refused" // 拒绝回拨
	case pb.Message_E_BAD_REQUEST:
		s = "bad request" // 错误请求
	case pb.Message_E_INTERNAL_ERROR:
		s = "internal error" // 内部错误
	default:
		s = "unknown" // 未知状态
	}
	return s
}

// 拒绝原因常量定义
const (
	rate_limited     = "rate limited"     // 速率限制
	dial_blocked     = "dial blocked"     // 回拨被阻止
	no_valid_address = "no valid address" // 无有效地址
)

// metricsTracer 实现了MetricsTracer接口
type metricsTracer struct{}

// 确保metricsTracer实现了MetricsTracer接口
var _ MetricsTracer = &metricsTracer{}

// metricsTracerSetting 定义了指标追踪器的配置
type metricsTracerSetting struct {
	reg prometheus.Registerer // 指标注册器
}

// MetricsTracerOption 定义了指标追踪器的配置选项函数类型
type MetricsTracerOption func(*metricsTracerSetting)

// WithRegisterer 设置指标注册器
// 参数:
//   - reg: prometheus.Registerer 指标注册器
//
// 返回值:
//   - MetricsTracerOption: 返回配置选项函数
func WithRegisterer(reg prometheus.Registerer) MetricsTracerOption {
	return func(s *metricsTracerSetting) {
		if reg != nil {
			s.reg = reg
		}
	}
}

// NewMetricsTracer 创建新的指标追踪器
// 参数:
//   - opts: ...MetricsTracerOption 配置选项
//
// 返回值:
//   - MetricsTracer: 返回指标追踪器实例
func NewMetricsTracer(opts ...MetricsTracerOption) MetricsTracer {
	// 创建默认配置
	setting := &metricsTracerSetting{reg: prometheus.DefaultRegisterer}
	// 应用配置选项
	for _, opt := range opts {
		opt(setting)
	}
	// 注册所有指标收集器
	metricshelper.RegisterCollectors(setting.reg, collectors...)
	return &metricsTracer{}
}

// ReachabilityStatus 实现MetricsTracer接口,记录可达性状态
func (mt *metricsTracer) ReachabilityStatus(status network.Reachability) {
	reachabilityStatus.Set(float64(status))
}

// ReachabilityStatusConfidence 实现MetricsTracer接口,记录可达性状态置信度
func (mt *metricsTracer) ReachabilityStatusConfidence(confidence int) {
	reachabilityStatusConfidence.Set(float64(confidence))
}

// ReceivedDialResponse 实现MetricsTracer接口,记录收到的回拨响应
func (mt *metricsTracer) ReceivedDialResponse(status pb.Message_ResponseStatus) {
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)
	*tags = append(*tags, getResponseStatus(status))
	receivedDialResponseTotal.WithLabelValues(*tags...).Inc()
}

// OutgoingDialResponse 实现MetricsTracer接口,记录发出的回拨响应
func (mt *metricsTracer) OutgoingDialResponse(status pb.Message_ResponseStatus) {
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)
	*tags = append(*tags, getResponseStatus(status))
	outgoingDialResponseTotal.WithLabelValues(*tags...).Inc()
}

// OutgoingDialRefused 实现MetricsTracer接口,记录拒绝的回拨请求
func (mt *metricsTracer) OutgoingDialRefused(reason string) {
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)
	*tags = append(*tags, reason)
	outgoingDialRefusedTotal.WithLabelValues(*tags...).Inc()
}

// NextProbeTime 实现MetricsTracer接口,记录下次探测时间
func (mt *metricsTracer) NextProbeTime(t time.Time) {
	nextProbeTimestamp.Set(float64(t.Unix()))
}
