//go:build !windows && !riscv64 && !loong64

package tcp

import (
	"strings"
	"sync"
	"time"

	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	"github.com/marten-seemann/tcp"
	"github.com/mikioh/tcpinfo"
	"github.com/prometheus/client_golang/prometheus"
)

// 定义 Prometheus 指标变量
var (
	// newConns 新建连接计数器
	newConns *prometheus.CounterVec
	// closedConns 关闭连接计数器
	closedConns *prometheus.CounterVec
	// segsSentDesc 发送段数指标描述
	segsSentDesc *prometheus.Desc
	// segsRcvdDesc 接收段数指标描述
	segsRcvdDesc *prometheus.Desc
	// bytesSentDesc 发送字节数指标描述
	bytesSentDesc *prometheus.Desc
	// bytesRcvdDesc 接收字节数指标描述
	bytesRcvdDesc *prometheus.Desc
)

// collectFrequency 指标收集频率
const collectFrequency = 10 * time.Second

// defaultCollector 默认指标收集器
var defaultCollector *aggregatingCollector

// initMetricsOnce 确保指标只初始化一次
var initMetricsOnce sync.Once

// initMetrics 初始化 Prometheus 指标
func initMetrics() {
	// 初始化指标描述
	segsSentDesc = prometheus.NewDesc("tcp_sent_segments_total", "已发送的 TCP 段数", nil, nil)
	segsRcvdDesc = prometheus.NewDesc("tcp_rcvd_segments_total", "已接收的 TCP 段数", nil, nil)
	bytesSentDesc = prometheus.NewDesc("tcp_sent_bytes", "已发送的 TCP 字节数", nil, nil)
	bytesRcvdDesc = prometheus.NewDesc("tcp_rcvd_bytes", "已接收的 TCP 字节数", nil, nil)

	// 创建并注册默认收集器
	defaultCollector = newAggregatingCollector()
	prometheus.MustRegister(defaultCollector)

	// 定义标签
	const direction = "direction"

	// 创建并注册新连接计数器
	newConns = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tcp_connections_new_total",
			Help: "新建 TCP 连接数",
		},
		[]string{direction},
	)
	prometheus.MustRegister(newConns)

	// 创建并注册关闭连接计数器
	closedConns = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tcp_connections_closed_total",
			Help: "已关闭的 TCP 连接数",
		},
		[]string{direction},
	)
	prometheus.MustRegister(closedConns)
}

// aggregatingCollector 聚合指标收集器结构体
type aggregatingCollector struct {
	cronOnce sync.Once // 确保定时任务只启动一次

	mutex         sync.Mutex              // 互斥锁
	highestID     uint64                  // 最大连接 ID
	conns         map[uint64]*tracingConn // 连接映射表
	rtts          prometheus.Histogram    // RTT 直方图
	connDurations prometheus.Histogram    // 连接持续时间直方图
	segsSent      uint64                  // 已发送段数
	segsRcvd      uint64                  // 已接收段数
	bytesSent     uint64                  // 已发送字节数
	bytesRcvd     uint64                  // 已接收字节数
}

// 确保 aggregatingCollector 实现了 prometheus.Collector 接口
var _ prometheus.Collector = &aggregatingCollector{}

// newAggregatingCollector 创建新的聚合指标收集器
// 返回值:
//   - *aggregatingCollector 新创建的收集器对象
func newAggregatingCollector() *aggregatingCollector {
	c := &aggregatingCollector{
		conns: make(map[uint64]*tracingConn),
		rtts: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "tcp_rtt",
			Help:    "TCP 往返时间",
			Buckets: prometheus.ExponentialBuckets(0.001, 1.25, 40), // 1ms 到 ~6000ms
		}),
		connDurations: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "tcp_connection_duration",
			Help:    "TCP 连接持续时间",
			Buckets: prometheus.ExponentialBuckets(1, 1.5, 40), // 1s 到 ~12 周
		}),
	}
	return c
}

// AddConn 添加一个新的跟踪连接
// 参数:
//   - t: *tracingConn 要添加的跟踪连接
//
// 返回值:
//   - uint64 分配的连接 ID
func (c *aggregatingCollector) AddConn(t *tracingConn) uint64 {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.highestID++
	c.conns[c.highestID] = t
	return c.highestID
}

// removeConn 移除指定 ID 的连接
// 参数:
//   - id: uint64 要移除的连接 ID
func (c *aggregatingCollector) removeConn(id uint64) {
	delete(c.conns, id)
}

// Describe 实现 prometheus.Collector 接口
// 参数:
//   - descs: chan<- *prometheus.Desc 指标描述通道
func (c *aggregatingCollector) Describe(descs chan<- *prometheus.Desc) {
	descs <- c.rtts.Desc()
	descs <- c.connDurations.Desc()
	if hasSegmentCounter {
		descs <- segsSentDesc
		descs <- segsRcvdDesc
	}
	if hasByteCounter {
		descs <- bytesSentDesc
		descs <- bytesRcvdDesc
	}
}

// cron 定期收集指标的后台任务
func (c *aggregatingCollector) cron() {
	ticker := time.NewTicker(collectFrequency)
	defer ticker.Stop()

	for now := range ticker.C {
		c.gatherMetrics(now)
	}
}

// gatherMetrics 收集所有连接的指标
// 参数:
//   - now: time.Time 当前时间
func (c *aggregatingCollector) gatherMetrics(now time.Time) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	// 重置计数器
	c.segsSent = 0
	c.segsRcvd = 0
	c.bytesSent = 0
	c.bytesRcvd = 0

	// 遍历所有连接收集指标
	for _, conn := range c.conns {
		info, err := conn.getTCPInfo()
		if err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				continue
			}
			log.Errorf("获取 TCP 信息失败: %s", err)
			continue
		}
		if hasSegmentCounter {
			c.segsSent += getSegmentsSent(info)
			c.segsRcvd += getSegmentsRcvd(info)
		}
		if hasByteCounter {
			c.bytesSent += getBytesSent(info)
			c.bytesRcvd += getBytesRcvd(info)
		}
		c.rtts.Observe(info.RTT.Seconds())
		c.connDurations.Observe(now.Sub(conn.startTime).Seconds())
	}
}

// Collect 实现 prometheus.Collector 接口
// 参数:
//   - metrics: chan<- prometheus.Metric 指标通道
func (c *aggregatingCollector) Collect(metrics chan<- prometheus.Metric) {
	// 首次调用时启动指标收集
	c.cronOnce.Do(func() {
		c.gatherMetrics(time.Now())
		go c.cron()
	})

	c.mutex.Lock()
	defer c.mutex.Unlock()

	// 发送指标
	metrics <- c.rtts
	metrics <- c.connDurations
	if hasSegmentCounter {
		segsSentMetric, err := prometheus.NewConstMetric(segsSentDesc, prometheus.CounterValue, float64(c.segsSent))
		if err != nil {
			log.Errorf("创建 tcp_sent_segments_total 指标失败: %v", err)
			return
		}
		segsRcvdMetric, err := prometheus.NewConstMetric(segsRcvdDesc, prometheus.CounterValue, float64(c.segsRcvd))
		if err != nil {
			log.Errorf("创建 tcp_rcvd_segments_total 指标失败: %v", err)
			return
		}
		metrics <- segsSentMetric
		metrics <- segsRcvdMetric
	}
	if hasByteCounter {
		bytesSentMetric, err := prometheus.NewConstMetric(bytesSentDesc, prometheus.CounterValue, float64(c.bytesSent))
		if err != nil {
			log.Errorf("创建 tcp_sent_bytes 指标失败: %v", err)
			return
		}
		bytesRcvdMetric, err := prometheus.NewConstMetric(bytesRcvdDesc, prometheus.CounterValue, float64(c.bytesRcvd))
		if err != nil {
			log.Errorf("创建 tcp_rcvd_bytes 指标失败: %v", err)
			return
		}
		metrics <- bytesSentMetric
		metrics <- bytesRcvdMetric
	}
}

// ClosedConn 处理连接关闭事件
// 参数:
//   - conn: *tracingConn 已关闭的连接
//   - direction: string 连接方向
func (c *aggregatingCollector) ClosedConn(conn *tracingConn, direction string) {
	c.mutex.Lock()
	c.removeConn(conn.id)
	c.mutex.Unlock()
	closedConns.WithLabelValues(direction).Inc()
}

// tracingConn 带指标跟踪功能的连接结构体
type tracingConn struct {
	id        uint64                // 连接 ID
	collector *aggregatingCollector // 指标收集器
	startTime time.Time             // 连接开始时间
	isClient  bool                  // 是否为客户端连接

	manet.Conn           // 底层网络连接
	tcpConn    *tcp.Conn // TCP 连接
	closeOnce  sync.Once // 确保只关闭一次
	closeErr   error     // 关闭时的错误
}

// newTracingConn 创建新的跟踪连接
// 参数:
//   - c: manet.Conn 原始连接
//   - collector: *aggregatingCollector 指标收集器(可为空)
//   - isClient: bool 是否为客户端连接
//
// 返回值:
//   - *tracingConn 新创建的跟踪连接
//   - error 创建过程中的错误
func newTracingConn(c manet.Conn, collector *aggregatingCollector, isClient bool) (*tracingConn, error) {
	initMetricsOnce.Do(func() { initMetrics() })
	conn, err := tcp.NewConn(c)
	if err != nil {
		log.Debugf("创建TCP连接时出错: %s", err)
		return nil, err
	}
	tc := &tracingConn{
		startTime: time.Now(),
		isClient:  isClient,
		Conn:      c,
		tcpConn:   conn,
		collector: collector,
	}
	if tc.collector == nil {
		tc.collector = defaultCollector
	}
	tc.id = tc.collector.AddConn(tc)
	newConns.WithLabelValues(tc.getDirection()).Inc()
	return tc, nil
}

// getDirection 获取连接方向
// 返回值:
//   - string 连接方向("outgoing"或"incoming")
func (c *tracingConn) getDirection() string {
	if c.isClient {
		return "outgoing"
	}
	return "incoming"
}

// Close 关闭连接
// 返回值:
//   - error 关闭过程中的错误
func (c *tracingConn) Close() error {
	c.closeOnce.Do(func() {
		c.collector.ClosedConn(c, c.getDirection())
		c.closeErr = c.Conn.Close()
	})
	return c.closeErr
}

// getTCPInfo 获取 TCP 连接信息
// 返回值:
//   - *tcpinfo.Info TCP 连接信息
//   - error 获取过程中的错误
func (c *tracingConn) getTCPInfo() (*tcpinfo.Info, error) {
	var o tcpinfo.Info
	var b [256]byte
	i, err := c.tcpConn.Option(o.Level(), o.Name(), b[:])
	if err != nil {
		log.Debugf("获取TCP连接信息时出错: %s", err)
		return nil, err
	}
	info := i.(*tcpinfo.Info)
	return info, nil
}

// tracingListener 带指标跟踪功能的监听器结构体
type tracingListener struct {
	manet.Listener                       // 底层网络监听器
	collector      *aggregatingCollector // 指标收集器
}

// newTracingListener 创建新的跟踪监听器
// 参数:
//   - l: manet.Listener 原始监听器
//   - collector: *aggregatingCollector 指标收集器(可为空)
//
// 返回值:
//   - *tracingListener 新创建的跟踪监听器
func newTracingListener(l manet.Listener, collector *aggregatingCollector) *tracingListener {
	return &tracingListener{Listener: l, collector: collector}
}

// Accept 接受新的连接
// 返回值:
//   - manet.Conn 新接受的连接
//   - error 接受过程中的错误
func (l *tracingListener) Accept() (manet.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		log.Debugf("接受新连接时出错: %s", err)
		return nil, err
	}
	return newTracingConn(conn, l.collector, false)
}
