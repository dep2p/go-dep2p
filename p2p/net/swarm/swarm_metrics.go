package swarm

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/p2p/metricshelper"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"

	"github.com/prometheus/client_golang/prometheus"
)

// 定义 dep2p swarm 指标的命名空间
const metricNamespace = "dep2p_swarm"

var (
	// 连接打开计数器
	connsOpened = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "connections_opened_total",
			Help:      "已打开的连接数",
		},
		[]string{"dir", "transport", "security", "muxer", "early_muxer", "ip_version"},
	)

	// 密钥类型计数器
	keyTypes = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "key_types_total",
			Help:      "密钥类型",
		},
		[]string{"dir", "key_type"},
	)

	// 连接关闭计数器
	connsClosed = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "connections_closed_total",
			Help:      "已关闭的连接数",
		},
		[]string{"dir", "transport", "security", "muxer", "early_muxer", "ip_version"},
	)

	// 拨号错误计数器
	dialError = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "dial_errors_total",
			Help:      "拨号错误数",
		},
		[]string{"transport", "error", "ip_version"},
	)

	// 连接持续时间直方图
	connDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricNamespace,
			Name:      "connection_duration_seconds",
			Help:      "连接持续时间",
			Buckets:   prometheus.ExponentialBuckets(1.0/16, 2, 25), // 最长24天
		},
		[]string{"dir", "transport", "security", "muxer", "early_muxer", "ip_version"},
	)

	// 连接握手延迟直方图
	connHandshakeLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricNamespace,
			Name:      "handshake_latency_seconds",
			Help:      "dep2p握手延迟时间",
			Buckets:   prometheus.ExponentialBuckets(0.001, 1.3, 35),
		},
		[]string{"transport", "security", "muxer", "early_muxer", "ip_version"},
	)

	// 每个节点的拨号次数计数器
	dialsPerPeer = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "dials_per_peer_total",
			Help:      "每个节点的拨号地址数",
		},
		[]string{"outcome", "num_dials"},
	)

	// 拨号延迟直方图
	dialLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: metricNamespace,
			Name:      "dial_latency_seconds",
			Help:      "建立节点连接所需时间",
			Buckets:   []float64{0.001, 0.01, 0.05, 0.1, 0.2, 0.3, 0.4, 0.5, 0.75, 1, 2},
		},
		[]string{"outcome", "num_dials"},
	)

	// 拨号排序延迟直方图
	dialRankingDelay = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Namespace: metricNamespace,
			Name:      "dial_ranking_delay_seconds",
			Help:      "拨号排序逻辑引入的延迟",
			Buckets:   []float64{0.001, 0.01, 0.05, 0.1, 0.2, 0.3, 0.4, 0.5, 0.75, 1, 2},
		},
	)

	// 黑洞过滤器状态指标
	blackHoleSuccessCounterState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "black_hole_filter_state",
			Help:      "黑洞过滤器状态",
		},
		[]string{"name"},
	)

	// 黑洞过滤器成功率指标
	blackHoleSuccessCounterSuccessFraction = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "black_hole_filter_success_fraction",
			Help:      "最近n次请求中成功拨号的比例",
		},
		[]string{"name"},
	)

	// 黑洞过滤器下次允许请求时间指标
	blackHoleSuccessCounterNextRequestAllowedAfter = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "black_hole_filter_next_request_allowed_after",
			Help:      "允许下一次请求前需要的请求数",
		},
		[]string{"name"},
	)

	// 所有收集器列表
	// 包含了所有需要注册的 Prometheus 指标收集器
	collectors = []prometheus.Collector{
		connsOpened,                            // 已打开连接数指标
		keyTypes,                               // 密钥类型指标
		connsClosed,                            // 已关闭连接数指标
		dialError,                              // 拨号错误指标
		connDuration,                           // 连接持续时间指标
		connHandshakeLatency,                   // 连接握手延迟指标
		dialsPerPeer,                           // 每个节点的拨号次数指标
		dialRankingDelay,                       // 拨号排序延迟指标
		dialLatency,                            // 拨号延迟指标
		blackHoleSuccessCounterSuccessFraction, // 黑洞过滤器成功率指标
		blackHoleSuccessCounterState,           // 黑洞过滤器状态指标
		blackHoleSuccessCounterNextRequestAllowedAfter, // 黑洞过滤器下次允许请求时间指标
	}
)

// MetricsTracer 定义了指标追踪器接口
// 用于收集和记录网络连接相关的各种指标
type MetricsTracer interface {
	// OpenedConnection 记录新建连接的指标
	// 参数:
	//   - direction: 连接方向(入站/出站)
	//   - pubKey: 对端的公钥
	//   - connState: 连接状态信息
	//   - localAddr: 本地多地址
	OpenedConnection(network.Direction, crypto.PubKey, network.ConnectionState, ma.Multiaddr)

	// ClosedConnection 记录关闭连接的指标
	// 参数:
	//   - direction: 连接方向
	//   - duration: 连接持续时间
	//   - connState: 连接状态信息
	//   - localAddr: 本地多地址
	ClosedConnection(network.Direction, time.Duration, network.ConnectionState, ma.Multiaddr)

	// CompletedHandshake 记录完成握手的指标
	// 参数:
	//   - duration: 握手耗时
	//   - connState: 连接状态信息
	//   - localAddr: 本地多地址
	CompletedHandshake(time.Duration, network.ConnectionState, ma.Multiaddr)

	// FailedDialing 记录拨号失败的指标
	// 参数:
	//   - addr: 目标多地址
	//   - dialErr: 拨号错误
	//   - cause: 错误原因
	FailedDialing(ma.Multiaddr, error, error)

	// DialCompleted 记录拨号完成的指标
	// 参数:
	//   - success: 是否成功
	//   - totalDials: 总拨号次数
	//   - latency: 拨号延迟
	DialCompleted(success bool, totalDials int, latency time.Duration)

	// DialRankingDelay 记录拨号排序延迟的指标
	// 参数:
	//   - d: 延迟时间
	DialRankingDelay(d time.Duration)

	// UpdatedBlackHoleSuccessCounter 更新黑洞过滤器计数器
	// 参数:
	//   - name: 过滤器名称
	//   - state: 过滤器状态
	//   - nextProbeAfter: 下次探测前需等待的请求数
	//   - successFraction: 成功率
	UpdatedBlackHoleSuccessCounter(name string, state BlackHoleState, nextProbeAfter int, successFraction float64)
}

// metricsTracer 实现了 MetricsTracer 接口
type metricsTracer struct{}

// 确保 metricsTracer 实现了 MetricsTracer 接口
var _ MetricsTracer = &metricsTracer{}

// metricsTracerSetting 包含指标追踪器的配置
type metricsTracerSetting struct {
	reg prometheus.Registerer // Prometheus 注册器
}

// MetricsTracerOption 定义了配置指标追踪器的函数类型
type MetricsTracerOption func(*metricsTracerSetting)

// WithRegisterer 返回一个设置 Prometheus 注册器的选项函数
// 参数:
//   - reg: Prometheus 注册器
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
//   - opts: 配置选项列表
//
// 返回值:
//   - MetricsTracer 指标追踪器实例
func NewMetricsTracer(opts ...MetricsTracerOption) MetricsTracer {
	setting := &metricsTracerSetting{reg: prometheus.DefaultRegisterer}
	for _, opt := range opts {
		opt(setting)
	}
	metricshelper.RegisterCollectors(setting.reg, collectors...)
	return &metricsTracer{}
}

// appendConnectionState 添加连接状态相关的标签
// 参数:
//   - tags: 现有标签列表
//   - cs: 连接状态
//
// 返回值:
//   - []string 添加了连接状态标签的列表
func appendConnectionState(tags []string, cs network.ConnectionState) []string {
	if cs.Transport == "" {
		// 如果传输层未正确设置 Transport 字段，这种情况不应该发生
		tags = append(tags, "unknown")
	} else {
		tags = append(tags, cs.Transport)
	}
	// 这些字段可能为空，取决于具体的传输层
	// 例如，QUIC 不设置安全性和多路复用器
	tags = append(tags, string(cs.Security))
	tags = append(tags, string(cs.StreamMultiplexer))

	earlyMuxer := "false"
	if cs.UsedEarlyMuxerNegotiation {
		earlyMuxer = "true"
	}
	tags = append(tags, earlyMuxer)
	return tags
}

// OpenedConnection 实现了 MetricsTracer 接口的方法
// 记录新建连接的指标
func (m *metricsTracer) OpenedConnection(dir network.Direction, p crypto.PubKey, cs network.ConnectionState, laddr ma.Multiaddr) {
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)

	*tags = append(*tags, metricshelper.GetDirection(dir))
	*tags = appendConnectionState(*tags, cs)
	*tags = append(*tags, metricshelper.GetIPVersion(laddr))
	connsOpened.WithLabelValues(*tags...).Inc()

	*tags = (*tags)[:0]
	*tags = append(*tags, metricshelper.GetDirection(dir))
	*tags = append(*tags, p.Type().String())
	keyTypes.WithLabelValues(*tags...).Inc()
}

// ClosedConnection 实现了 MetricsTracer 接口的方法
// 记录关闭连接的指标
func (m *metricsTracer) ClosedConnection(dir network.Direction, duration time.Duration, cs network.ConnectionState, laddr ma.Multiaddr) {
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)

	*tags = append(*tags, metricshelper.GetDirection(dir))
	*tags = appendConnectionState(*tags, cs)
	*tags = append(*tags, metricshelper.GetIPVersion(laddr))
	connsClosed.WithLabelValues(*tags...).Inc()
	connDuration.WithLabelValues(*tags...).Observe(duration.Seconds())
}

// CompletedHandshake 实现了 MetricsTracer 接口的方法
// 记录完成握手的指标
func (m *metricsTracer) CompletedHandshake(t time.Duration, cs network.ConnectionState, laddr ma.Multiaddr) {
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)

	*tags = appendConnectionState(*tags, cs)
	*tags = append(*tags, metricshelper.GetIPVersion(laddr))
	connHandshakeLatency.WithLabelValues(*tags...).Observe(t.Seconds())
}

// FailedDialing 实现了 MetricsTracer 接口的方法
// 记录拨号失败的指标
func (m *metricsTracer) FailedDialing(addr ma.Multiaddr, dialErr error, cause error) {
	transport := metricshelper.GetTransport(addr)
	e := "other"
	// 拨号超时或父上下文超时
	if errors.Is(dialErr, context.DeadlineExceeded) || errors.Is(cause, context.DeadlineExceeded) {
		e = "deadline"
	} else if errors.Is(dialErr, context.Canceled) {
		// 拨号被取消
		if errors.Is(cause, context.Canceled) {
			// 父上下文被取消
			e = "application canceled"
		} else if errors.Is(cause, errConcurrentDialSuccessful) {
			e = "canceled: concurrent dial successful"
		} else {
			// 其他原因
			e = "canceled: other"
		}
	} else {
		nerr, ok := dialErr.(net.Error)
		if ok && nerr.Timeout() {
			e = "timeout"
		} else if strings.Contains(dialErr.Error(), "connect: connection refused") {
			e = "connection refused"
		}
	}

	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)

	*tags = append(*tags, transport, e)
	*tags = append(*tags, metricshelper.GetIPVersion(addr))
	dialError.WithLabelValues(*tags...).Inc()
}

// DialCompleted 实现了 MetricsTracer 接口的方法
// 记录拨号完成的指标
func (m *metricsTracer) DialCompleted(success bool, totalDials int, latency time.Duration) {
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)
	if success {
		*tags = append(*tags, "success")
	} else {
		*tags = append(*tags, "failed")
	}

	numDialLabels := [...]string{"0", "1", "2", "3", "4", "5", ">=6"}
	var numDials string
	if totalDials < len(numDialLabels) {
		numDials = numDialLabels[totalDials]
	} else {
		numDials = numDialLabels[len(numDialLabels)-1]
	}
	*tags = append(*tags, numDials)
	dialsPerPeer.WithLabelValues(*tags...).Inc()
	dialLatency.WithLabelValues(*tags...).Observe(latency.Seconds())
}

// DialRankingDelay 实现了 MetricsTracer 接口的方法
// 记录拨号排序延迟的指标
func (m *metricsTracer) DialRankingDelay(d time.Duration) {
	dialRankingDelay.Observe(d.Seconds())
}

// UpdatedBlackHoleSuccessCounter 实现了 MetricsTracer 接口的方法
// 更新黑洞过滤器计数器
func (m *metricsTracer) UpdatedBlackHoleSuccessCounter(name string, state BlackHoleState,
	nextProbeAfter int, successFraction float64) {
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)

	*tags = append(*tags, name)

	blackHoleSuccessCounterState.WithLabelValues(*tags...).Set(float64(state))
	blackHoleSuccessCounterSuccessFraction.WithLabelValues(*tags...).Set(successFraction)
	blackHoleSuccessCounterNextRequestAllowedAfter.WithLabelValues(*tags...).Set(float64(nextProbeAfter))
}
