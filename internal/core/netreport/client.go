package netreport

import (
	"context"
	"sync"
	"time"

	netreportif "github.com/dep2p/go-dep2p/pkg/interfaces/netreport"
)

// Client 网络诊断客户端
//
// 实现 netreportif.Client 接口
type Client struct {
	config netreportif.Config
	prober *Prober

	// 报告缓存
	lastReport *netreportif.Report
	lastFull   time.Time
	mu         sync.RWMutex

	// 历史报告
	history     []*netreportif.Report
	historySize int
}

// 确保实现 netreportif.Client 接口
var _ netreportif.Client = (*Client)(nil)

// NewClient 创建网络诊断客户端
func NewClient(config netreportif.Config) *Client {
	return &Client{
		config:      config,
		prober:      NewProber(config),
		historySize: 10, // 保留最近 10 个报告
		history:     make([]*netreportif.Report, 0, 10),
	}
}

// GetReport 生成网络诊断报告
func (c *Client) GetReport(ctx context.Context) (*netreportif.Report, error) {
	log.Debug("开始生成网络诊断报告")

	// 检查是否需要完整报告
	c.mu.RLock()
	needFull := c.shouldDoFullReport()
	c.mu.RUnlock()

	// 运行探测
	report := c.prober.RunProbes(ctx)

	// 保存报告
	c.mu.Lock()
	c.saveReport(report, needFull)
	c.mu.Unlock()

	log.Info("网络诊断报告生成完成",
		"udpv4", report.UDPv4,
		"udpv6", report.UDPv6,
		"natType", report.NATType.String(),
		"duration", report.Duration)

	return report, nil
}

// GetReportAsync 异步生成报告
func (c *Client) GetReportAsync(ctx context.Context) <-chan *netreportif.Report {
	ch := make(chan *netreportif.Report, 1)

	go func() {
		defer close(ch)

		report, err := c.GetReport(ctx)
		if err != nil {
			log.Error("异步生成报告失败", "err", err)
			return
		}

		select {
		case ch <- report:
		case <-ctx.Done():
		}
	}()

	return ch
}

// LastReport 返回最后一次诊断报告
func (c *Client) LastReport() *netreportif.Report {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.lastReport
}

// SetSTUNServers 设置 STUN 服务器列表
func (c *Client) SetSTUNServers(servers []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.config.STUNServers = servers
	c.prober.UpdateConfig(c.config)
}

// SetRelayServers 设置中继服务器列表
func (c *Client) SetRelayServers(relays []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.config.RelayServers = relays
	c.prober.UpdateConfig(c.config)
}

// shouldDoFullReport 判断是否需要完整报告
func (c *Client) shouldDoFullReport() bool {
	// 如果没有历史报告，需要完整报告
	if c.lastReport == nil {
		return true
	}

	// 如果距离上次完整报告超过间隔，需要完整报告
	if time.Since(c.lastFull) > c.config.FullReportInterval {
		return true
	}

	// 如果上次报告没有 UDP 连通性且检测到强制门户，需要完整报告
	if !c.lastReport.HasUDP() && c.lastReport.CaptivePortal != nil && *c.lastReport.CaptivePortal {
		return true
	}

	return false
}

// saveReport 保存报告
func (c *Client) saveReport(report *netreportif.Report, isFull bool) {
	c.lastReport = report

	if isFull {
		c.lastFull = time.Now()
	}

	// 添加到历史
	c.history = append(c.history, report)

	// 保持历史大小
	if len(c.history) > c.historySize {
		c.history = c.history[len(c.history)-c.historySize:]
	}
}

// History 返回历史报告
func (c *Client) History() []*netreportif.Report {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]*netreportif.Report, len(c.history))
	copy(result, c.history)
	return result
}

// Config 返回当前配置
func (c *Client) Config() netreportif.Config {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.config
}

// UpdateConfig 更新配置
func (c *Client) UpdateConfig(config netreportif.Config) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.config = config
	c.prober.UpdateConfig(config)
}

// ForceFullReport 强制下次生成完整报告
func (c *Client) ForceFullReport() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.lastFull = time.Time{} // 重置上次完整报告时间
}

// ReportStats 报告统计
type ReportStats struct {
	// TotalReports 总报告数
	TotalReports int

	// SuccessfulIPv4 成功的 IPv4 探测次数
	SuccessfulIPv4 int

	// SuccessfulIPv6 成功的 IPv6 探测次数
	SuccessfulIPv6 int

	// AverageLatency 平均报告生成耗时
	AverageLatency time.Duration

	// LastReportTime 最后报告时间
	LastReportTime time.Time
}

// Stats 返回统计信息
func (c *Client) Stats() ReportStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := ReportStats{
		TotalReports: len(c.history),
	}

	if len(c.history) == 0 {
		return stats
	}

	var totalLatency time.Duration
	for _, r := range c.history {
		if r.UDPv4 {
			stats.SuccessfulIPv4++
		}
		if r.UDPv6 {
			stats.SuccessfulIPv6++
		}
		totalLatency += r.Duration
	}

	stats.AverageLatency = totalLatency / time.Duration(len(c.history))
	stats.LastReportTime = c.history[len(c.history)-1].Timestamp

	return stats
}

// BestRelays 返回按延迟排序的最佳中继列表
func (c *Client) BestRelays(count int) []string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.lastReport == nil || len(c.lastReport.RelayLatencies) == 0 {
		return nil
	}

	// 创建延迟-URL 对列表
	type relayLatency struct {
		url     string
		latency time.Duration
	}
	var relays []relayLatency
	for url, latency := range c.lastReport.RelayLatencies {
		relays = append(relays, relayLatency{url, latency})
	}

	// 按延迟排序
	for i := 0; i < len(relays)-1; i++ {
		for j := i + 1; j < len(relays); j++ {
			if relays[j].latency < relays[i].latency {
				relays[i], relays[j] = relays[j], relays[i]
			}
		}
	}

	// 返回前 count 个
	var result []string
	for i := 0; i < len(relays) && i < count; i++ {
		result = append(result, relays[i].url)
	}

	return result
}

