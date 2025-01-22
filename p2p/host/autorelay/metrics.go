package autorelay

import (
	"errors"

	"github.com/dep2p/go-dep2p/p2p/metricshelper"
	"github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/client"
	pbv2 "github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/pb"
	"github.com/prometheus/client_golang/prometheus"
)

// 指标命名空间
const metricNamespace = "dep2p_autorelay"

var (
	// 中继查找器状态指标
	status = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: metricNamespace,
		Name:      "status",
		Help:      "中继查找器是否活跃", // 1表示活跃,0表示不活跃
	})

	// 已打开的预约总数计数器
	reservationsOpenedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "reservations_opened_total",
			Help:      "已打开的预约总数",
		},
	)

	// 已关闭的预约总数计数器
	reservationsClosedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "reservations_closed_total",
			Help:      "已关闭的预约总数",
		},
	)

	// 预约请求结果计数器向量
	reservationRequestsOutcomeTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "reservation_requests_outcome_total",
			Help:      "预约请求结果统计",
		},
		[]string{"request_type", "outcome"}, // 标签:请求类型和结果
	)

	// 中继地址更新总数计数器
	relayAddressesUpdatedTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "relay_addresses_updated_total",
			Help:      "中继地址更新次数",
		},
	)

	// 中继地址数量指标
	relayAddressesCount = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "relay_addresses_count",
			Help:      "中继地址数量",
		},
	)

	// 候选节点对电路v2支持情况计数器向量
	candidatesCircuitV2SupportTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "candidates_circuit_v2_support_total",
			Help:      "支持电路v2的候选节点统计",
		},
		[]string{"support"}, // 标签:是否支持
	)

	// 候选节点总数计数器向量
	candidatesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: metricNamespace,
			Name:      "candidates_total",
			Help:      "候选节点总数",
		},
		[]string{"type"}, // 标签:类型
	)

	// 候选循环状态指标
	candLoopState = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "candidate_loop_state",
			Help:      "候选循环状态",
		},
	)

	// 计划工作时间指标向量
	scheduledWorkTime = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "scheduled_work_time",
			Help:      "计划工作时间",
		},
		[]string{"work_type"}, // 标签:工作类型
	)

	// 期望预约数量指标
	desiredReservations = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Namespace: metricNamespace,
			Name:      "desired_reservations",
			Help:      "期望的预约数量",
		},
	)

	// 收集器列表
	collectors = []prometheus.Collector{
		status,
		reservationsOpenedTotal,
		reservationsClosedTotal,
		reservationRequestsOutcomeTotal,
		relayAddressesUpdatedTotal,
		relayAddressesCount,
		candidatesCircuitV2SupportTotal,
		candidatesTotal,
		candLoopState,
		scheduledWorkTime,
		desiredReservations,
	}
)

// 候选循环状态类型
type candidateLoopState int

const (
	peerSourceRateLimited candidateLoopState = iota // 对等节点源速率受限
	waitingOnPeerChan                               // 等待对等节点通道
	waitingForTrigger                               // 等待触发
	stopped                                         // 已停止
)

// MetricsTracer 定义了自动中继的指标跟踪接口
type MetricsTracer interface {
	// RelayFinderStatus 更新中继查找器状态
	RelayFinderStatus(isActive bool)

	// ReservationEnded 记录已结束的预约数量
	ReservationEnded(cnt int)
	// ReservationOpened 记录已打开的预约数量
	ReservationOpened(cnt int)
	// ReservationRequestFinished 记录预约请求完成情况
	ReservationRequestFinished(isRefresh bool, err error)

	// RelayAddressCount 更新中继地址数量
	RelayAddressCount(int)
	// RelayAddressUpdated 记录中继地址更新
	RelayAddressUpdated()

	// CandidateChecked 记录候选节点检查结果
	CandidateChecked(supportsCircuitV2 bool)
	// CandidateAdded 记录添加的候选节点数量
	CandidateAdded(cnt int)
	// CandidateRemoved 记录移除的候选节点数量
	CandidateRemoved(cnt int)
	// CandidateLoopState 更新候选循环状态
	CandidateLoopState(state candidateLoopState)

	// ScheduledWorkUpdated 更新计划工作时间
	ScheduledWorkUpdated(scheduledWork *scheduledWorkTimes)

	// DesiredReservations 更新期望的预约数量
	DesiredReservations(int)
}

// metricsTracer 实现了 MetricsTracer 接口
type metricsTracer struct{}

var _ MetricsTracer = &metricsTracer{}

// metricsTracerSetting 定义指标跟踪器设置
type metricsTracerSetting struct {
	reg prometheus.Registerer // 指标注册器
}

// MetricsTracerOption 定义指标跟踪器选项函数类型
type MetricsTracerOption func(*metricsTracerSetting)

// WithRegisterer 设置指标注册器的选项函数
func WithRegisterer(reg prometheus.Registerer) MetricsTracerOption {
	return func(s *metricsTracerSetting) {
		if reg != nil {
			s.reg = reg
		}
	}
}

// NewMetricsTracer 创建新的指标跟踪器
// @param opts 可选的配置选项
// @return MetricsTracer 返回指标跟踪器实例
func NewMetricsTracer(opts ...MetricsTracerOption) MetricsTracer {
	// 创建默认设置
	setting := &metricsTracerSetting{reg: prometheus.DefaultRegisterer}
	// 应用选项
	for _, opt := range opts {
		opt(setting)
	}
	// 注册所有收集器
	metricshelper.RegisterCollectors(setting.reg, collectors...)

	// 初始化计数器为0,以便正确处理首次预约请求
	reservationRequestsOutcomeTotal.WithLabelValues("refresh", "success")
	reservationRequestsOutcomeTotal.WithLabelValues("new", "success")
	candidatesCircuitV2SupportTotal.WithLabelValues("yes")
	candidatesCircuitV2SupportTotal.WithLabelValues("no")
	return &metricsTracer{}
}

// RelayFinderStatus 更新中继查找器状态
// @param isActive 是否活跃
func (mt *metricsTracer) RelayFinderStatus(isActive bool) {
	if isActive {
		status.Set(1) // 设置为活跃状态
	} else {
		status.Set(0) // 设置为非活跃状态
	}
}

// ReservationEnded 记录预约结束的数量
// 参数:
//   - cnt: int 结束的预约数量
func (mt *metricsTracer) ReservationEnded(cnt int) {
	reservationsClosedTotal.Add(float64(cnt)) // 增加已关闭预约计数器
}

// ReservationOpened 记录新开启的预约数量
// 参数:
//   - cnt: int 开启的预约数量
func (mt *metricsTracer) ReservationOpened(cnt int) {
	reservationsOpenedTotal.Add(float64(cnt)) // 增加已开启预约计数器
}

// ReservationRequestFinished 记录预约请求完成的结果
// 参数:
//   - isRefresh: bool 是否为刷新请求
//   - err: error 请求的错误信息
func (mt *metricsTracer) ReservationRequestFinished(isRefresh bool, err error) {
	tags := metricshelper.GetStringSlice()   // 获取标签切片
	defer metricshelper.PutStringSlice(tags) // 延迟归还标签切片

	if isRefresh { // 如果是刷新请求
		*tags = append(*tags, "refresh") // 添加刷新标签
	} else {
		*tags = append(*tags, "new") // 添加新请求标签
	}
	*tags = append(*tags, getReservationRequestStatus(err))         // 添加请求状态标签
	reservationRequestsOutcomeTotal.WithLabelValues(*tags...).Inc() // 增加请求结果计数器

	if !isRefresh && err == nil { // 如果是成功的新请求
		reservationsOpenedTotal.Inc() // 增加已开启预约计数器
	}
}

// RelayAddressUpdated 记录中继地址更新事件
func (mt *metricsTracer) RelayAddressUpdated() {
	relayAddressesUpdatedTotal.Inc() // 增加地址更新计数器
}

// RelayAddressCount 记录当前中继地址数量
// 参数:
//   - cnt: int 中继地址数量
func (mt *metricsTracer) RelayAddressCount(cnt int) {
	relayAddressesCount.Set(float64(cnt)) // 设置地址数量指标
}

// CandidateChecked 记录候选节点检查结果
// 参数:
//   - supportsCircuitV2: bool 是否支持电路v2协议
func (mt *metricsTracer) CandidateChecked(supportsCircuitV2 bool) {
	tags := metricshelper.GetStringSlice()   // 获取标签切片
	defer metricshelper.PutStringSlice(tags) // 延迟归还标签切片
	if supportsCircuitV2 {                   // 如果支持电路v2
		*tags = append(*tags, "yes") // 添加支持标签
	} else {
		*tags = append(*tags, "no") // 添加不支持标签
	}
	candidatesCircuitV2SupportTotal.WithLabelValues(*tags...).Inc() // 增加支持情况计数器
}

// CandidateAdded 记录添加的候选节点数量
// 参数:
//   - cnt: int 添加的节点数量
func (mt *metricsTracer) CandidateAdded(cnt int) {
	tags := metricshelper.GetStringSlice()                      // 获取标签切片
	defer metricshelper.PutStringSlice(tags)                    // 延迟归还标签切片
	*tags = append(*tags, "added")                              // 添加增加标签
	candidatesTotal.WithLabelValues(*tags...).Add(float64(cnt)) // 增加候选节点计数器
}

// CandidateRemoved 记录移除的候选节点数量
// 参数:
//   - cnt: int 移除的节点数量
func (mt *metricsTracer) CandidateRemoved(cnt int) {
	tags := metricshelper.GetStringSlice()                      // 获取标签切片
	defer metricshelper.PutStringSlice(tags)                    // 延迟归还标签切片
	*tags = append(*tags, "removed")                            // 添加移除标签
	candidatesTotal.WithLabelValues(*tags...).Add(float64(cnt)) // 增加候选节点计数器
}

// CandidateLoopState 记录候选循环状态
// 参数:
//   - state: candidateLoopState 循环状态值
func (mt *metricsTracer) CandidateLoopState(state candidateLoopState) {
	candLoopState.Set(float64(state)) // 设置循环状态指标
}

// ScheduledWorkUpdated 更新计划任务时间
// 参数:
//   - scheduledWork: *scheduledWorkTimes 计划任务时间信息
func (mt *metricsTracer) ScheduledWorkUpdated(scheduledWork *scheduledWorkTimes) {
	tags := metricshelper.GetStringSlice()   // 获取标签切片
	defer metricshelper.PutStringSlice(tags) // 延迟归还标签切片

	*tags = append(*tags, "allowed peer source call")                                                          // 添加允许对等源调用标签
	scheduledWorkTime.WithLabelValues(*tags...).Set(float64(scheduledWork.nextAllowedCallToPeerSource.Unix())) // 设置下次允许调用时间
	*tags = (*tags)[:0]                                                                                        // 清空标签

	*tags = append(*tags, "reservation refresh")                                               // 添加预约刷新标签
	scheduledWorkTime.WithLabelValues(*tags...).Set(float64(scheduledWork.nextRefresh.Unix())) // 设置下次刷新时间
	*tags = (*tags)[:0]                                                                        // 清空标签

	*tags = append(*tags, "clear backoff")                                                     // 添加清除退避标签
	scheduledWorkTime.WithLabelValues(*tags...).Set(float64(scheduledWork.nextBackoff.Unix())) // 设置下次退避时间
	*tags = (*tags)[:0]                                                                        // 清空标签

	*tags = append(*tags, "old candidate check")                                                         // 添加旧候选检查标签
	scheduledWorkTime.WithLabelValues(*tags...).Set(float64(scheduledWork.nextOldCandidateCheck.Unix())) // 设置下次检查时间
}

// DesiredReservations 记录期望的预约数量
// 参数:
//   - cnt: int 期望的预约数量
func (mt *metricsTracer) DesiredReservations(cnt int) {
	desiredReservations.Set(float64(cnt)) // 设置期望预约数量指标
}

// getReservationRequestStatus 获取预约请求状态字符串
// 参数:
//   - err: error 错误信息
//
// 返回值:
//   - string: 状态描述字符串
func getReservationRequestStatus(err error) string {
	if err == nil { // 如果没有错误
		return "success" // 返回成功状态
	}

	status := "err other"          // 默认为其他错误
	var re client.ReservationError // 定义预约错误变量
	if errors.As(err, &re) {       // 如果是预约错误类型
		switch re.Status { // 根据状态码判断
		case pbv2.Status_CONNECTION_FAILED:
			return "connection failed" // 连接失败
		case pbv2.Status_MALFORMED_MESSAGE:
			return "malformed message" // 消息格式错误
		case pbv2.Status_RESERVATION_REFUSED:
			return "reservation refused" // 预约被拒绝
		case pbv2.Status_PERMISSION_DENIED:
			return "permission denied" // 权限被拒绝
		case pbv2.Status_RESOURCE_LIMIT_EXCEEDED:
			return "resource limit exceeded" // 资源超限
		}
	}
	return status // 返回状态字符串
}

// wrappedMetricsTracer 包装指标跟踪器,当mt为nil时忽略所有调用
type wrappedMetricsTracer struct {
	mt MetricsTracer // 内部指标跟踪器
}

var _ MetricsTracer = &wrappedMetricsTracer{} // 确保实现了MetricsTracer接口

// RelayFinderStatus 更新中继查找器状态
// 参数:
//   - isActive: bool 是否活跃
func (mt *wrappedMetricsTracer) RelayFinderStatus(isActive bool) {
	if mt.mt != nil { // 如果内部跟踪器不为空
		mt.mt.RelayFinderStatus(isActive) // 调用内部方法
	}
}

// ReservationEnded 记录预约结束
// 参数:
//   - cnt: int 结束的预约数量
func (mt *wrappedMetricsTracer) ReservationEnded(cnt int) {
	if mt.mt != nil { // 如果内部跟踪器不为空
		mt.mt.ReservationEnded(cnt) // 调用内部方法
	}
}

// ReservationOpened 记录预约开启
// 参数:
//   - cnt: int 开启的预约数量
func (mt *wrappedMetricsTracer) ReservationOpened(cnt int) {
	if mt.mt != nil { // 如果内部跟踪器不为空
		mt.mt.ReservationOpened(cnt) // 调用内部方法
	}
}

// ReservationRequestFinished 记录预约请求完成
// 参数:
//   - isRefresh: bool 是否为刷新请求
//   - err: error 错误信息
func (mt *wrappedMetricsTracer) ReservationRequestFinished(isRefresh bool, err error) {
	if mt.mt != nil { // 如果内部跟踪器不为空
		mt.mt.ReservationRequestFinished(isRefresh, err) // 调用内部方法
	}
}

// RelayAddressUpdated 记录中继地址更新
func (mt *wrappedMetricsTracer) RelayAddressUpdated() {
	if mt.mt != nil { // 如果内部跟踪器不为空
		mt.mt.RelayAddressUpdated() // 调用内部方法
	}
}

// RelayAddressCount 记录中继地址数量
// 参数:
//   - cnt: int 地址数量
func (mt *wrappedMetricsTracer) RelayAddressCount(cnt int) {
	if mt.mt != nil { // 如果内部跟踪器不为空
		mt.mt.RelayAddressCount(cnt) // 调用内部方法
	}
}

// CandidateChecked 记录候选节点检查
// 参数:
//   - supportsCircuitV2: bool 是否支持电路v2
func (mt *wrappedMetricsTracer) CandidateChecked(supportsCircuitV2 bool) {
	if mt.mt != nil { // 如果内部跟踪器不为空
		mt.mt.CandidateChecked(supportsCircuitV2) // 调用内部方法
	}
}

// CandidateAdded 记录候选节点添加
// 参数:
//   - cnt: int 添加的节点数量
func (mt *wrappedMetricsTracer) CandidateAdded(cnt int) {
	if mt.mt != nil { // 如果内部跟踪器不为空
		mt.mt.CandidateAdded(cnt) // 调用内部方法
	}
}

// CandidateRemoved 记录候选节点移除
// 参数:
//   - cnt: int 移除的节点数量
func (mt *wrappedMetricsTracer) CandidateRemoved(cnt int) {
	if mt.mt != nil { // 如果内部跟踪器不为空
		mt.mt.CandidateRemoved(cnt) // 调用内部方法
	}
}

// ScheduledWorkUpdated 更新计划任务时间
// 参数:
//   - scheduledWork: *scheduledWorkTimes 计划任务时间信息
func (mt *wrappedMetricsTracer) ScheduledWorkUpdated(scheduledWork *scheduledWorkTimes) {
	if mt.mt != nil { // 如果内部跟踪器不为空
		mt.mt.ScheduledWorkUpdated(scheduledWork) // 调用内部方法
	}
}

// DesiredReservations 记录期望的预约数量
// 参数:
//   - cnt: int 期望的预约数量
func (mt *wrappedMetricsTracer) DesiredReservations(cnt int) {
	if mt.mt != nil { // 如果内部跟踪器不为空
		mt.mt.DesiredReservations(cnt) // 调用内部方法
	}
}

// CandidateLoopState 记录候选循环状态
// 参数:
//   - state: candidateLoopState 循环状态值
func (mt *wrappedMetricsTracer) CandidateLoopState(state candidateLoopState) {
	if mt.mt != nil { // 如果内部跟踪器不为空
		mt.mt.CandidateLoopState(state) // 调用内部方法
	}
}
