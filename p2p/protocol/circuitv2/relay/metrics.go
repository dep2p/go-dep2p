package relay

import (
	"time"

	"github.com/dep2p/go-dep2p/p2p/metricshelper"
	pbv2 "github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/pb"
	"github.com/prometheus/client_golang/prometheus"
)

// 中继服务的指标命名空间
const metricNamespace = "dep2p_relaysvc"

var (
	// 中继状态指标
	status = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "status",
			Help:      "中继状态",
		},
	)

	// 预约请求总数指标
	reservationsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "reservations_total",
			Help:      "中继预约请求总数",
		},
		[]string{"type"},
	)
	// 预约请求响应状态总数指标
	reservationRequestResponseStatusTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "reservation_request_response_status_total",
			Help:      "中继预约请求响应状态总数",
		},
		[]string{"status"},
	)
	// 预约拒绝总数指标
	reservationRejectionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "reservation_rejections_total",
			Help:      "中继预约被拒绝原因总数",
		},
		[]string{"reason"},
	)

	// 连接总数指标
	connectionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "connections_total",
			Help:      "中继连接总数",
		},
		[]string{"type"},
	)
	// 连接请求响应状态总数指标
	connectionRequestResponseStatusTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "connection_request_response_status_total",
			Help:      "中继连接请求状态总数",
		},
		[]string{"status"},
	)
	// 连接拒绝总数指标
	connectionRejectionsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "connection_rejections_total",
			Help:      "中继连接被拒绝原因总数",
		},
		[]string{"reason"},
	)
	// 连接持续时间指标
	connectionDurationSeconds = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: metricNamespace,
			Name:      "connection_duration_seconds",
			Help:      "中继连接持续时间",
		},
	)

	// 数据传输字节总数指标
	dataTransferredBytesTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "data_transferred_bytes_total",
			Help:      "传输字节总数",
		},
	)

	// 所有指标收集器
	collectors = []prometheus.Collector{
		status,
		reservationsTotal,
		reservationRequestResponseStatusTotal,
		reservationRejectionsTotal,
		connectionsTotal,
		connectionRequestResponseStatusTotal,
		connectionRejectionsTotal,
		connectionDurationSeconds,
		dataTransferredBytesTotal,
	}
)

// 请求状态常量
const (
	requestStatusOK       = "ok"       // 请求成功
	requestStatusRejected = "rejected" // 请求被拒绝
	requestStatusError    = "error"    // 请求错误
)

// MetricsTracer 是中继服务的指标跟踪接口
type MetricsTracer interface {
	// RelayStatus 跟踪服务当前是否处于活动状态
	// 参数:
	//   - enabled: bool 服务是否启用
	RelayStatus(enabled bool)

	// ConnectionOpened 跟踪中继连接打开的指标
	ConnectionOpened()

	// ConnectionClosed 跟踪中继连接关闭的指标
	// 参数:
	//   - d: time.Duration 连接持续时间
	ConnectionClosed(d time.Duration)

	// ConnectionRequestHandled 跟踪处理中继连接请求的指标
	// 参数:
	//   - status: pbv2.Status 请求状态
	ConnectionRequestHandled(status pbv2.Status)

	// ReservationAllowed 跟踪打开或续订中继预约的指标
	// 参数:
	//   - isRenewal: bool 是否为续订
	ReservationAllowed(isRenewal bool)

	// ReservationClosed 跟踪关闭中继预约的指标
	// 参数:
	//   - cnt: int 关闭的预约数量
	ReservationClosed(cnt int)

	// ReservationRequestHandled 跟踪处理中继预约请求的指标
	// 参数:
	//   - status: pbv2.Status 请求状态
	ReservationRequestHandled(status pbv2.Status)

	// BytesTransferred 跟踪中继服务传输的总字节数
	// 参数:
	//   - cnt: int 传输的字节数
	BytesTransferred(cnt int)
}

// metricsTracer 实现了 MetricsTracer 接口
type metricsTracer struct{}

var _ MetricsTracer = &metricsTracer{}

// metricsTracerSetting 包含指标跟踪器的配置
type metricsTracerSetting struct {
	reg prometheus.Registerer
}

// MetricsTracerOption 定义指标跟踪器的配置选项函数类型
type MetricsTracerOption func(*metricsTracerSetting)

// WithRegisterer 设置指标注册器
// 参数:
//   - reg: prometheus.Registerer 指标注册器
//
// 返回值:
//   - MetricsTracerOption 配置选项函数
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
	setting := &metricsTracerSetting{reg: prometheus.DefaultRegisterer}
	for _, opt := range opts {
		opt(setting)
	}
	metricshelper.RegisterCollectors(setting.reg, collectors...)
	return &metricsTracer{}
}

// RelayStatus 实现 MetricsTracer 接口
// 参数:
//   - enabled: bool 服务是否启用
func (mt *metricsTracer) RelayStatus(enabled bool) {
	if enabled {
		status.Set(1)
	} else {
		status.Set(0)
	}
}

// ConnectionOpened 实现 MetricsTracer 接口
func (mt *metricsTracer) ConnectionOpened() {
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)
	*tags = append(*tags, "opened")

	connectionsTotal.WithLabelValues(*tags...).Add(1)
}

// ConnectionClosed 实现 MetricsTracer 接口
// 参数:
//   - d: time.Duration 连接持续时间
func (mt *metricsTracer) ConnectionClosed(d time.Duration) {
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)
	*tags = append(*tags, "closed")

	connectionsTotal.WithLabelValues(*tags...).Add(1)
	connectionDurationSeconds.Observe(d.Seconds())
}

// ConnectionRequestHandled 实现 MetricsTracer 接口
// 参数:
//   - status: pbv2.Status 请求状态
func (mt *metricsTracer) ConnectionRequestHandled(status pbv2.Status) {
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)

	respStatus := getResponseStatus(status)

	*tags = append(*tags, respStatus)
	connectionRequestResponseStatusTotal.WithLabelValues(*tags...).Add(1)
	if respStatus == requestStatusRejected {
		*tags = (*tags)[:0]
		*tags = append(*tags, getRejectionReason(status))
		connectionRejectionsTotal.WithLabelValues(*tags...).Add(1)
	}
}

// ReservationAllowed 实现 MetricsTracer 接口
// 参数:
//   - isRenewal: bool 是否为续订
func (mt *metricsTracer) ReservationAllowed(isRenewal bool) {
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)
	if isRenewal {
		*tags = append(*tags, "renewed")
	} else {
		*tags = append(*tags, "opened")
	}

	reservationsTotal.WithLabelValues(*tags...).Add(1)
}

// ReservationClosed 实现 MetricsTracer 接口
// 参数:
//   - cnt: int 关闭的预约数量
func (mt *metricsTracer) ReservationClosed(cnt int) {
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)
	*tags = append(*tags, "closed")

	reservationsTotal.WithLabelValues(*tags...).Add(float64(cnt))
}

// ReservationRequestHandled 实现 MetricsTracer 接口
// 参数:
//   - status: pbv2.Status 请求状态
func (mt *metricsTracer) ReservationRequestHandled(status pbv2.Status) {
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)

	respStatus := getResponseStatus(status)

	*tags = append(*tags, respStatus)
	reservationRequestResponseStatusTotal.WithLabelValues(*tags...).Add(1)
	if respStatus == requestStatusRejected {
		*tags = (*tags)[:0]
		*tags = append(*tags, getRejectionReason(status))
		reservationRejectionsTotal.WithLabelValues(*tags...).Add(1)
	}
}

// BytesTransferred 实现 MetricsTracer 接口
// 参数:
//   - cnt: int 传输的字节数
func (mt *metricsTracer) BytesTransferred(cnt int) {
	dataTransferredBytesTotal.Add(float64(cnt))
}

// getResponseStatus 获取响应状态字符串
// 参数:
//   - status: pbv2.Status 状态码
//
// 返回值:
//   - string 状态字符串
func getResponseStatus(status pbv2.Status) string {
	responseStatus := "unknown"
	switch status {
	case pbv2.Status_RESERVATION_REFUSED,
		pbv2.Status_RESOURCE_LIMIT_EXCEEDED,
		pbv2.Status_PERMISSION_DENIED,
		pbv2.Status_NO_RESERVATION,
		pbv2.Status_MALFORMED_MESSAGE:

		responseStatus = requestStatusRejected
	case pbv2.Status_UNEXPECTED_MESSAGE, pbv2.Status_CONNECTION_FAILED:
		responseStatus = requestStatusError
	case pbv2.Status_OK:
		responseStatus = requestStatusOK
	}
	return responseStatus
}

// getRejectionReason 获取拒绝原因字符串
// 参数:
//   - status: pbv2.Status 状态码
//
// 返回值:
//   - string 拒绝原因字符串
func getRejectionReason(status pbv2.Status) string {
	reason := "unknown"
	switch status {
	case pbv2.Status_RESERVATION_REFUSED:
		reason = "ip constraint violation"
	case pbv2.Status_RESOURCE_LIMIT_EXCEEDED:
		reason = "resource limit exceeded"
	case pbv2.Status_PERMISSION_DENIED:
		reason = "permission denied"
	case pbv2.Status_NO_RESERVATION:
		reason = "no reservation"
	case pbv2.Status_MALFORMED_MESSAGE:
		reason = "malformed message"
	}
	return reason
}
