package rcmgr

import (
	"strings"

	"github.com/dep2p/go-dep2p/p2p/metricshelper"
	"github.com/prometheus/client_golang/prometheus"
)

// dep2p资源管理器的指标命名空间
const metricNamespace = "dep2p_rcmgr"

var (
	// 连接相关指标
	conns = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricNamespace,
		Name:      "connections", // 连接数量
		Help:      "连接数量",        // 帮助信息:连接数量
	}, []string{"dir", "scope"}) // 标签:方向和作用域

	// 入站系统连接指标
	connsInboundSystem = conns.With(prometheus.Labels{"dir": "inbound", "scope": "system"})
	// 入站临时连接指标
	connsInboundTransient = conns.With(prometheus.Labels{"dir": "inbound", "scope": "transient"})
	// 出站系统连接指标
	connsOutboundSystem = conns.With(prometheus.Labels{"dir": "outbound", "scope": "system"})
	// 出站临时连接指标
	connsOutboundTransient = conns.With(prometheus.Labels{"dir": "outbound", "scope": "transient"})

	// 1-10线性分布后指数分布的桶
	oneTenThenExpDistributionBuckets = []float64{
		1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 16, 32, 64, 128, 256,
	}

	// 每个节点的连接数指标
	peerConns = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: metricNamespace,
		Name:      "peer_connections", // 节点连接数
		Buckets:   oneTenThenExpDistributionBuckets,
		Help:      "此节点拥有的连接数",
	}, []string{"dir"}) // 标签:方向
	// 入站节点连接指标
	peerConnsInbound = peerConns.With(prometheus.Labels{"dir": "inbound"})
	// 出站节点连接指标
	peerConnsOutbound = peerConns.With(prometheus.Labels{"dir": "outbound"})

	// 用于构建当前状态的直方图。更多信息参见 https://github.com/dep2p/go-dep2p-resource-manager/pull/54#discussion_r911244757
	previousPeerConns = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: metricNamespace,
		Name:      "previous_peer_connections", // 节点之前的连接数
		Buckets:   oneTenThenExpDistributionBuckets,
		Help:      "此节点之前拥有的连接数。用于通过从peer_connections直方图中减去此值来获取每个节点的当前连接数直方图",
	}, []string{"dir"}) // 标签:方向
	// 入站节点之前的连接指标
	previousPeerConnsInbound = previousPeerConns.With(prometheus.Labels{"dir": "inbound"})
	// 出站节点之前的连接指标
	previousPeerConnsOutbound = previousPeerConns.With(prometheus.Labels{"dir": "outbound"})

	// 流相关指标
	streams = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricNamespace,
		Name:      "streams", // 流数量
		Help:      "流数量",
	}, []string{"dir", "scope", "protocol"}) // 标签:方向、作用域和协议

	// 每个节点的流数指标
	peerStreams = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: metricNamespace,
		Name:      "peer_streams", // 节点流数
		Buckets:   oneTenThenExpDistributionBuckets,
		Help:      "此节点拥有的流数",
	}, []string{"dir"}) // 标签:方向
	// 入站节点流指标
	peerStreamsInbound = peerStreams.With(prometheus.Labels{"dir": "inbound"})
	// 出站节点流指标
	peerStreamsOutbound = peerStreams.With(prometheus.Labels{"dir": "outbound"})

	// 节点之前的流数指标
	previousPeerStreams = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: metricNamespace,
		Name:      "previous_peer_streams", // 节点之前的流数
		Buckets:   oneTenThenExpDistributionBuckets,
		Help:      "此节点拥有的流数",
	}, []string{"dir"}) // 标签:方向
	// 入站节点之前的流指标
	previousPeerStreamsInbound = previousPeerStreams.With(prometheus.Labels{"dir": "inbound"})
	// 出站节点之前的流指标
	previousPeerStreamsOutbound = previousPeerStreams.With(prometheus.Labels{"dir": "outbound"})

	// 内存相关指标
	memoryTotal = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricNamespace,
		Name:      "memory", // 内存使用量
		Help:      "向资源管理器报告的预留内存量",
	}, []string{"scope", "protocol"}) // 标签:作用域和协议

	// 节点内存指标
	peerMemory = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: metricNamespace,
		Name:      "peer_memory", // 节点内存
		Buckets:   memDistribution,
		Help:      "向资源管理器报告的预留此内存桶的节点数量",
	})
	// 节点之前的内存指标
	previousPeerMemory = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: metricNamespace,
		Name:      "previous_peer_memory", // 节点之前的内存
		Buckets:   memDistribution,
		Help:      "向资源管理器报告的之前预留此内存桶的节点数量",
	})

	// 连接内存指标
	connMemory = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: metricNamespace,
		Name:      "conn_memory", // 连接内存
		Buckets:   memDistribution,
		Help:      "向资源管理器报告的预留此内存桶的连接数量",
	})
	// 连接之前的内存指标
	previousConnMemory = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace: metricNamespace,
		Name:      "previous_conn_memory", // 连接之前的内存
		Buckets:   memDistribution,
		Help:      "向资源管理器报告的之前预留此内存桶的连接数量",
	})

	// 文件描述符相关指标
	fds = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricNamespace,
		Name:      "fds", // 文件描述符数量
		Help:      "向资源管理器报告的预留文件描述符数量",
	}, []string{"scope"}) // 标签:作用域

	// 系统文件描述符指标
	fdsSystem = fds.With(prometheus.Labels{"scope": "system"})
	// 临时文件描述符指标
	fdsTransient = fds.With(prometheus.Labels{"scope": "transient"})

	// 被阻塞的资源指标
	blockedResources = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: metricNamespace,
		Name:      "blocked_resources", // 被阻塞的资源数量
		Help:      "被阻塞的资源数量",
	}, []string{"dir", "scope", "resource"}) // 标签:方向、作用域和资源类型
)

// 内存分布区间定义
var (
	memDistribution = []float64{
		1 << 10,   // 1KB - 1KB大小的内存区间
		4 << 10,   // 4KB - 4KB大小的内存区间
		32 << 10,  // 32KB - 32KB大小的内存区间
		1 << 20,   // 1MB - 1MB大小的内存区间
		32 << 20,  // 32MB - 32MB大小的内存区间
		256 << 20, // 256MB - 256MB大小的内存区间
		512 << 20, // 512MB - 512MB大小的内存区间
		1 << 30,   // 1GB - 1GB大小的内存区间
		2 << 30,   // 2GB - 2GB大小的内存区间
		4 << 30,   // 4GB - 4GB大小的内存区间
	}
)

// MustRegisterWith 注册所有指标收集器到Prometheus
// @param reg - Prometheus注册器实例
func MustRegisterWith(reg prometheus.Registerer) {
	// 注册所有指标收集器
	metricshelper.RegisterCollectors(reg,
		conns,             // 连接数指标
		peerConns,         // 节点连接数指标
		previousPeerConns, // 节点之前的连接数指标
		streams,           // 流数指标
		peerStreams,       // 节点流数指标

		previousPeerStreams, // 节点之前的流数指标

		memoryTotal,        // 总内存指标
		peerMemory,         // 节点内存指标
		previousPeerMemory, // 节点之前的内存指标
		connMemory,         // 连接内存指标
		previousConnMemory, // 连接之前的内存指标
		fds,                // 文件描述符指标
		blockedResources,   // 被阻塞的资源指标
	)
}

// WithMetricsDisabled 返回一个禁用指标收集的选项函数
// @return Option - 资源管理器配置选项
func WithMetricsDisabled() Option {
	return func(r *resourceManager) error {
		r.disableMetrics = true // 设置禁用指标标志为true
		return nil
	}
}

// StatsTraceReporter 使用跟踪信息报告资源管理器的统计数据
type StatsTraceReporter struct{}

// NewStatsTraceReporter 创建一个新的统计跟踪报告器
// 返回值:
//   - StatsTraceReporter: 统计跟踪报告器实例
//   - error: 错误信息
func NewStatsTraceReporter() (StatsTraceReporter, error) {
	// TODO: 告知 prometheus 系统限制
	return StatsTraceReporter{}, nil
}

// ConsumeEvent 消费跟踪事件
// 参数:
//   - evt: TraceEvt - 跟踪事件
func (r StatsTraceReporter) ConsumeEvent(evt TraceEvt) {
	// 从对象池获取字符串切片
	tags := metricshelper.GetStringSlice()
	// 使用完后归还到对象池
	defer metricshelper.PutStringSlice(tags)

	// 使用标签切片消费事件
	r.consumeEventWithLabelSlice(evt, tags)
}

// consumeEventWithLabelSlice 使用标签切片消费事件(单独的函数以便测试该函数是否分配内存,对象池可能会分配)
// 参数:
//   - evt: TraceEvt - 跟踪事件
//   - tags: *[]string - 标签切片指针
func (r StatsTraceReporter) consumeEventWithLabelSlice(evt TraceEvt, tags *[]string) {
	// 根据事件类型进行处理
	switch evt.Type {
	case TraceAddStreamEvt, TraceRemoveStreamEvt: // 处理添加/移除流事件
		if p := PeerStrInScopeName(evt.Name); p != "" { // 如果是节点作用域
			// 聚合的节点统计。统计有N个流打开的节点数量。
			// 使用两个桶聚合。一个用于计算节点当前的流数量。
			// 另一个用于计算负值,即节点之前有多少流。
			// 查看数据时取两者的差值。

			// 计算出站流的变化
			oldStreamsOut := int64(evt.StreamsOut - evt.DeltaOut) // 计算旧的出站流数量
			peerStreamsOut := int64(evt.StreamsOut)               // 当前出站流数量
			if oldStreamsOut != peerStreamsOut {                  // 如果数量发生变化
				if oldStreamsOut != 0 { // 如果旧数量不为0
					previousPeerStreamsOutbound.Observe(float64(oldStreamsOut)) // 记录旧的出站流数量
				}
				if peerStreamsOut != 0 { // 如果当前数量不为0
					peerStreamsOutbound.Observe(float64(peerStreamsOut)) // 记录当前出站流数量
				}
			}

			// 计算入站流的变化
			oldStreamsIn := int64(evt.StreamsIn - evt.DeltaIn) // 计算旧的入站流数量
			peerStreamsIn := int64(evt.StreamsIn)              // 当前入站流数量
			if oldStreamsIn != peerStreamsIn {                 // 如果数量发生变化
				if oldStreamsIn != 0 { // 如果旧数量不为0
					previousPeerStreamsInbound.Observe(float64(oldStreamsIn)) // 记录旧的入站流数量
				}
				if peerStreamsIn != 0 { // 如果当前数量不为0
					peerStreamsInbound.Observe(float64(peerStreamsIn)) // 记录当前入站流数量
				}
			}
		} else { // 如果不是节点作用域
			if evt.DeltaOut != 0 { // 如果出站流变化不为0
				if IsSystemScope(evt.Name) || IsTransientScope(evt.Name) { // 如果是系统作用域或临时作用域
					// 重置标签切片
					*tags = (*tags)[:0]                                            // 清空标签切片
					*tags = append(*tags, "outbound", evt.Name, "")                // 添加标签
					streams.WithLabelValues(*tags...).Set(float64(evt.StreamsOut)) // 设置出站流数量
				} else if proto := ParseProtocolScopeName(evt.Name); proto != "" { // 如果是协议作用域
					// 重置标签切片
					*tags = (*tags)[:0]                                            // 清空标签切片
					*tags = append(*tags, "outbound", "protocol", proto)           // 添加标签
					streams.WithLabelValues(*tags...).Set(float64(evt.StreamsOut)) // 设置出站流数量
				} else {
					// 不测量服务作用域、连接作用域、服务节点和协议节点。数据量太大,
					// 可以使用聚合的节点统计+服务统计来推断这些数据。
					break
				}
			}

			if evt.DeltaIn != 0 { // 如果入站流变化不为0
				if IsSystemScope(evt.Name) || IsTransientScope(evt.Name) { // 如果是系统作用域或临时作用域
					// 重置标签切片
					*tags = (*tags)[:0]                                           // 清空标签切片
					*tags = append(*tags, "inbound", evt.Name, "")                // 添加标签
					streams.WithLabelValues(*tags...).Set(float64(evt.StreamsIn)) // 设置入站流数量
				} else if proto := ParseProtocolScopeName(evt.Name); proto != "" { // 如果是协议作用域
					// 重置标签切片
					*tags = (*tags)[:0]                                           // 清空标签切片
					*tags = append(*tags, "inbound", "protocol", proto)           // 添加标签
					streams.WithLabelValues(*tags...).Set(float64(evt.StreamsIn)) // 设置入站流数量
				} else {
					// 不测量服务作用域、连接作用域、服务节点和协议节点。数据量太大,
					// 可以使用聚合的节点统计+服务统计来推断这些数据。
					break
				}
			}
		}

	case TraceAddConnEvt, TraceRemoveConnEvt: // 处理添加/移除连接事件
		if p := PeerStrInScopeName(evt.Name); p != "" { // 如果是节点作用域
			// 聚合的节点统计。统计有N个连接的节点数量。
			// 使用两个桶聚合。一个用于计算节点当前的连接数量。
			// 另一个用于计算负值,即节点之前有多少连接。
			// 查看数据时取两者的差值。

			// 计算出站连接的变化
			oldConnsOut := int64(evt.ConnsOut - evt.DeltaOut) // 计算旧的出站连接数量
			connsOut := int64(evt.ConnsOut)                   // 当前出站连接数量
			if oldConnsOut != connsOut {                      // 如果数量发生变化
				if oldConnsOut != 0 { // 如果旧数量不为0
					previousPeerConnsOutbound.Observe(float64(oldConnsOut)) // 记录旧的出站连接数量
				}
				if connsOut != 0 { // 如果当前数量不为0
					peerConnsOutbound.Observe(float64(connsOut)) // 记录当前出站连接数量
				}
			}

			// 计算入站连接的变化
			oldConnsIn := int64(evt.ConnsIn - evt.DeltaIn) // 计算旧的入站连接数量
			connsIn := int64(evt.ConnsIn)                  // 当前入站连接数量
			if oldConnsIn != connsIn {                     // 如果数量发生变化
				if oldConnsIn != 0 { // 如果旧数量不为0
					previousPeerConnsInbound.Observe(float64(oldConnsIn)) // 记录旧的入站连接数量
				}
				if connsIn != 0 { // 如果当前数量不为0
					peerConnsInbound.Observe(float64(connsIn)) // 记录当前入站连接数量
				}
			}
		} else { // 如果不是节点作用域
			if IsConnScope(evt.Name) { // 如果是连接作用域
				// 不测量这个。认为它没有用处。
				break
			}

			// 更新系统和临时作用域的连接计数
			if IsSystemScope(evt.Name) { // 如果是系统作用域
				connsInboundSystem.Set(float64(evt.ConnsIn))   // 设置入站连接数量
				connsOutboundSystem.Set(float64(evt.ConnsOut)) // 设置出站连接数量
			} else if IsTransientScope(evt.Name) { // 如果是临时作用域
				connsInboundTransient.Set(float64(evt.ConnsIn))   // 设置入站连接数量
				connsOutboundTransient.Set(float64(evt.ConnsOut)) // 设置出站连接数量
			}

			// 表示文件描述符的变化
			if evt.Delta != 0 { // 如果文件描述符数量发生变化
				if IsSystemScope(evt.Name) { // 如果是系统作用域
					fdsSystem.Set(float64(evt.FD)) // 设置系统文件描述符数量
				} else if IsTransientScope(evt.Name) { // 如果是临时作用域
					fdsTransient.Set(float64(evt.FD)) // 设置临时文件描述符数量
				}
			}
		}

	case TraceReserveMemoryEvt, TraceReleaseMemoryEvt: // 处理预留/释放内存事件
		if p := PeerStrInScopeName(evt.Name); p != "" { // 如果是节点作用域
			// 计算节点内存使用的变化
			oldMem := evt.Memory - evt.Delta // 计算旧的内存使用量
			if oldMem != evt.Memory {        // 如果内存使用量发生变化
				if oldMem != 0 { // 如果旧使用量不为0
					previousPeerMemory.Observe(float64(oldMem)) // 记录旧的内存使用量
				}
				if evt.Memory != 0 { // 如果当前使用量不为0
					peerMemory.Observe(float64(evt.Memory)) // 记录当前内存使用量
				}
			}
		} else if IsConnScope(evt.Name) { // 如果是连接作用域
			// 计算连接内存使用的变化
			oldMem := evt.Memory - evt.Delta // 计算旧的内存使用量
			if oldMem != evt.Memory {        // 如果内存使用量发生变化
				if oldMem != 0 { // 如果旧使用量不为0
					previousConnMemory.Observe(float64(oldMem)) // 记录旧的内存使用量
				}
				if evt.Memory != 0 { // 如果当前使用量不为0
					connMemory.Observe(float64(evt.Memory)) // 记录当前内存使用量
				}
			}
		} else { // 如果是其他作用域
			if IsSystemScope(evt.Name) || IsTransientScope(evt.Name) { // 如果是系统作用域或临时作用域
				// 重置标签切片
				*tags = (*tags)[:0]                                            // 清空标签切片
				*tags = append(*tags, evt.Name, "")                            // 添加标签
				memoryTotal.WithLabelValues(*tags...).Set(float64(evt.Memory)) // 设置总内存使用量
			} else if proto := ParseProtocolScopeName(evt.Name); proto != "" { // 如果是协议作用域
				// 重置标签切片
				*tags = (*tags)[:0]                                            // 清空标签切片
				*tags = append(*tags, "protocol", proto)                       // 添加标签
				memoryTotal.WithLabelValues(*tags...).Set(float64(evt.Memory)) // 设置总内存使用量
			} else {
				// 不测量连接作用域、服务节点和协议节点。数据量太大,
				// 可以使用聚合的节点统计+服务统计来推断这些数据。
				break
			}
		}

	case TraceBlockAddConnEvt, TraceBlockAddStreamEvt, TraceBlockReserveMemoryEvt: // 处理阻塞事件
		// 确定资源类型
		var resource string                   // 资源类型字符串
		if evt.Type == TraceBlockAddConnEvt { // 如果是阻塞添加连接事件
			resource = "connection" // 设置资源类型为连接
		} else if evt.Type == TraceBlockAddStreamEvt { // 如果是阻塞添加流事件
			resource = "stream" // 设置资源类型为流
		} else { // 如果是阻塞预留内存事件
			resource = "memory" // 设置资源类型为内存
		}

		// 获取作用域名称
		scopeName := evt.Name // 获取事件名称作为作用域名称
		// 只获取顶层作用域名称。这里不需要获取节点ID。
		// 使用索引和切片以避免分配。
		scopeSplitIdx := strings.IndexByte(scopeName, ':') // 查找冒号分隔符
		if scopeSplitIdx != -1 {                           // 如果找到分隔符
			scopeName = evt.Name[0:scopeSplitIdx] // 截取顶层作用域名称
		}
		// 删除连接或流ID
		idSplitIdx := strings.IndexByte(scopeName, '-') // 查找连字符分隔符
		if idSplitIdx != -1 {                           // 如果找到分隔符
			scopeName = scopeName[0:idSplitIdx] // 截取不包含ID的作用域名称
		}

		// 更新入站被阻塞的资源计数
		if evt.DeltaIn != 0 { // 如果入站变化不为0
			*tags = (*tags)[:0]                                                  // 清空标签切片
			*tags = append(*tags, "inbound", scopeName, resource)                // 添加标签
			blockedResources.WithLabelValues(*tags...).Add(float64(evt.DeltaIn)) // 增加入站被阻塞资源计数
		}

		// 更新出站被阻塞的资源计数
		if evt.DeltaOut != 0 { // 如果出站变化不为0
			*tags = (*tags)[:0]                                                   // 清空标签切片
			*tags = append(*tags, "outbound", scopeName, resource)                // 添加标签
			blockedResources.WithLabelValues(*tags...).Add(float64(evt.DeltaOut)) // 增加出站被阻塞资源计数
		}

		// 更新被阻塞的文件描述符或其他资源计数
		if evt.Delta != 0 && resource == "connection" { // 如果是连接资源且变化不为0
			// 这表示被阻塞的文件描述符
			*tags = (*tags)[:0]                                                // 清空标签切片
			*tags = append(*tags, "", scopeName, "fd")                         // 添加标签
			blockedResources.WithLabelValues(*tags...).Add(float64(evt.Delta)) // 增加被阻塞文件描述符计数
		} else if evt.Delta != 0 { // 如果是其他资源且变化不为0
			*tags = (*tags)[:0]                                                // 清空标签切片
			*tags = append(*tags, "", scopeName, resource)                     // 添加标签
			blockedResources.WithLabelValues(*tags...).Add(float64(evt.Delta)) // 增加被阻塞资源计数
		}
	}
}
