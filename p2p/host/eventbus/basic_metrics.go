package eventbus

import (
	"reflect"
	"strings"

	"github.com/dep2p/go-dep2p/p2p/metricshelper"

	"github.com/prometheus/client_golang/prometheus"
)

// 定义指标命名空间
const metricNamespace = "dep2p_eventbus"

var (
	// 已发送事件计数器
	eventsEmitted = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "events_emitted_total",
			Help:      "已发送的事件总数",
		},
		[]string{"event"},
	)

	// 订阅者总数指标
	totalSubscribers = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "subscribers_total",
			Help:      "每种事件类型的订阅者数量",
		},
		[]string{"event"},
	)

	// 订阅者队列长度指标
	subscriberQueueLength = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "subscriber_queue_length",
			Help:      "订阅者队列长度",
		},
		[]string{"subscriber_name"},
	)

	// 订阅者队列已满指标
	subscriberQueueFull = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "subscriber_queue_full",
			Help:      "订阅者队列是否已满",
		},
		[]string{"subscriber_name"},
	)

	// 订阅者事件入队计数器
	subscriberEventQueued = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "subscriber_event_queued",
			Help:      "订阅者事件入队总数",
		},
		[]string{"subscriber_name"},
	)

	// 收集器列表
	collectors = []prometheus.Collector{
		eventsEmitted,
		totalSubscribers,
		subscriberQueueLength,
		subscriberQueueFull,
		subscriberEventQueued,
	}
)

// MetricsTracer 定义事件总线子系统的指标跟踪接口
type MetricsTracer interface {
	// EventEmitted 统计按事件类型分组的事件总数
	// 参数:
	//   - typ: reflect.Type 事件类型
	EventEmitted(typ reflect.Type)

	// AddSubscriber 为事件类型添加订阅者
	// 参数:
	//   - typ: reflect.Type 事件类型
	AddSubscriber(typ reflect.Type)

	// RemoveSubscriber 移除事件类型的订阅者
	// 参数:
	//   - typ: reflect.Type 事件类型
	RemoveSubscriber(typ reflect.Type)

	// SubscriberQueueLength 设置订阅者通道长度
	// 参数:
	//   - name: string 订阅者名称
	//   - n: int 队列长度
	SubscriberQueueLength(name string, n int)

	// SubscriberQueueFull 跟踪订阅者通道是否已满
	// 参数:
	//   - name: string 订阅者名称
	//   - isFull: bool 是否已满
	SubscriberQueueFull(name string, isFull bool)

	// SubscriberEventQueued 统计按订阅者分组的事件总数
	// 参数:
	//   - name: string 订阅者名称
	SubscriberEventQueued(name string)
}

// metricsTracer 实现 MetricsTracer 接口
type metricsTracer struct{}

// 确保 metricsTracer 实现了 MetricsTracer 接口
var _ MetricsTracer = &metricsTracer{}

// metricsTracerSetting 定义指标跟踪器配置
type metricsTracerSetting struct {
	reg prometheus.Registerer // 指标注册器
}

// MetricsTracerOption 定义配置选项函数类型
type MetricsTracerOption func(*metricsTracerSetting)

// WithRegisterer 设置指标注册器选项
// 参数:
//   - reg: prometheus.Registerer 指标注册器
//
// 返回:
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
// 返回:
//   - MetricsTracer 指标跟踪器实例
func NewMetricsTracer(opts ...MetricsTracerOption) MetricsTracer {
	// 初始化默认配置
	setting := &metricsTracerSetting{reg: prometheus.DefaultRegisterer}
	// 应用配置选项
	for _, opt := range opts {
		opt(setting)
	}
	// 注册收集器
	metricshelper.RegisterCollectors(setting.reg, collectors...)
	return &metricsTracer{}
}

// EventEmitted 实现 MetricsTracer 接口
// 参数:
//   - typ: reflect.Type 事件类型
func (m *metricsTracer) EventEmitted(typ reflect.Type) {
	// 获取标签切片
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)

	// 添加事件类型标签
	*tags = append(*tags, strings.TrimPrefix(typ.String(), "event."))
	// 增加计数
	eventsEmitted.WithLabelValues(*tags...).Inc()
}

// AddSubscriber 实现 MetricsTracer 接口
// 参数:
//   - typ: reflect.Type 订阅的事件类型
func (m *metricsTracer) AddSubscriber(typ reflect.Type) {
	// 获取标签切片
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)

	// 添加事件类型标签
	*tags = append(*tags, strings.TrimPrefix(typ.String(), "event."))
	// 增加订阅者计数
	totalSubscribers.WithLabelValues(*tags...).Inc()
}

// RemoveSubscriber 实现 MetricsTracer 接口
// 参数:
//   - typ: reflect.Type 取消订阅的事件类型
func (m *metricsTracer) RemoveSubscriber(typ reflect.Type) {
	// 获取标签切片
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)

	// 添加事件类型标签
	*tags = append(*tags, strings.TrimPrefix(typ.String(), "event."))
	// 减少订阅者计数
	totalSubscribers.WithLabelValues(*tags...).Dec()
}

// SubscriberQueueLength 实现 MetricsTracer 接口
// 参数:
//   - name: string 订阅者名称
//   - n: int 队列长度
func (m *metricsTracer) SubscriberQueueLength(name string, n int) {
	// 获取标签切片
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)

	// 添加订阅者名称标签
	*tags = append(*tags, name)
	// 设置队列长度
	subscriberQueueLength.WithLabelValues(*tags...).Set(float64(n))
}

// SubscriberQueueFull 实现 MetricsTracer 接口
// 参数:
//   - name: string 订阅者名称
//   - isFull: bool 队列是否已满
func (m *metricsTracer) SubscriberQueueFull(name string, isFull bool) {
	// 获取标签切片
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)

	// 添加订阅者名称标签
	*tags = append(*tags, name)
	// 获取观察器
	observer := subscriberQueueFull.WithLabelValues(*tags...)
	// 设置队列状态
	if isFull {
		observer.Set(1)
	} else {
		observer.Set(0)
	}
}

// SubscriberEventQueued 实现 MetricsTracer 接口
// 参数:
//   - name: string 订阅者名称
func (m *metricsTracer) SubscriberEventQueued(name string) {
	// 获取标签切片
	tags := metricshelper.GetStringSlice()
	defer metricshelper.PutStringSlice(tags)

	// 添加订阅者名称标签
	*tags = append(*tags, name)
	// 增加入队事件计数
	subscriberEventQueued.WithLabelValues(*tags...).Inc()
}
