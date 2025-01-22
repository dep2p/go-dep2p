package autonatv2

import (
	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	"github.com/dep2p/go-dep2p/p2p/metricshelper"
	"github.com/dep2p/go-dep2p/p2p/protocol/autonatv2/pb"
	"github.com/prometheus/client_golang/prometheus"
)

// MetricsTracer 定义了指标追踪器的接口
type MetricsTracer interface {
	// CompletedRequest 记录已完成的请求事件
	// 参数:
	//   - e: EventDialRequestCompleted 拨号请求完成事件
	CompletedRequest(EventDialRequestCompleted)
}

// metricNamespace 定义了指标的命名空间
const metricNamespace = "dep2p_autonatv2"

var (
	// requestsCompleted 统计已完成请求的计数器
	requestsCompleted = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "requests_completed_total",
			Help:      "已完成的请求总数",
		},
		[]string{"server_error", "response_status", "dial_status", "dial_data_required", "ip_or_dns_version", "transport"},
	)
)

// metricsTracer 实现了 MetricsTracer 接口
type metricsTracer struct {
}

// NewMetricsTracer 创建一个新的指标追踪器
// 参数:
//   - reg: prometheus.Registerer Prometheus 注册器
//
// 返回值:
//   - MetricsTracer 指标追踪器实例
func NewMetricsTracer(reg prometheus.Registerer) MetricsTracer {
	metricshelper.RegisterCollectors(reg, requestsCompleted)
	return &metricsTracer{}
}

// CompletedRequest 实现了 MetricsTracer 接口中的方法，用于记录已完成的请求
// 参数:
//   - e: EventDialRequestCompleted 拨号请求完成事件
func (m *metricsTracer) CompletedRequest(e EventDialRequestCompleted) {
	labels := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(labels)

	errStr := getErrString(e.Error)

	dialData := "false"
	if e.DialDataRequired {
		dialData = "true"
	}

	var ip, transport string
	if e.DialedAddr != nil {
		ip = getIPOrDNSVersion(e.DialedAddr)
		transport = metricshelper.GetTransport(e.DialedAddr)
	}

	*labels = append(*labels,
		errStr,
		pb.DialResponse_ResponseStatus_name[int32(e.ResponseStatus)],
		pb.DialStatus_name[int32(e.DialStatus)],
		dialData,
		ip,
		transport,
	)
	requestsCompleted.WithLabelValues(*labels...).Inc()
}

// getIPOrDNSVersion 获取多地址中的 IP 或 DNS 版本
// 参数:
//   - a: ma.Multiaddr 多地址对象
//
// 返回值:
//   - string IP 或 DNS 版本字符串
func getIPOrDNSVersion(a ma.Multiaddr) string {
	if a == nil {
		return ""
	}
	res := "unknown"
	ma.ForEach(a, func(c ma.Component) bool {
		switch c.Protocol().Code {
		case ma.P_IP4:
			res = "ip4"
		case ma.P_IP6:
			res = "ip6"
		case ma.P_DNS, ma.P_DNSADDR:
			res = "dns"
		case ma.P_DNS4:
			res = "dns4"
		case ma.P_DNS6:
			res = "dns6"
		}
		return false
	})
	return res
}

// getErrString 将错误转换为字符串表示
// 参数:
//   - e: error 错误对象
//
// 返回值:
//   - string 错误的字符串表示
func getErrString(e error) string {
	var errStr string
	switch e {
	case nil:
		errStr = "nil"
	case errBadRequest, errDialDataRefused, errResourceLimitExceeded:
		errStr = e.Error()
	default:
		errStr = "other"
	}
	return errStr
}
