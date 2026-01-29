// Package netreport 提供网络诊断功能
//
// IMPL-NETWORK-RESILIENCE Phase 6.3: 诊断客户端
package netreport

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ============================================================================
//                              诊断客户端
// ============================================================================

// Client 网络诊断客户端
type Client struct {
	mu     sync.RWMutex
	config Config

	// 缓存
	lastReport     *Report
	lastReportTime time.Time

	// STUN 客户端
	stunClient *STUNClient
}

// NewClient 创建诊断客户端
func NewClient(config Config) *Client {
	return &Client{
		config:     config,
		stunClient: NewSTUNClient(config.STUNServers, config.ProbeTimeout),
	}
}

// GetReport 生成网络诊断报告
func (c *Client) GetReport(ctx context.Context) (*Report, error) {
	return c.GetReportWithOptions(ctx, ProbeOptions{Full: true})
}

// GetReportWithOptions 使用选项生成报告
func (c *Client) GetReportWithOptions(ctx context.Context, opts ProbeOptions) (*Report, error) {
	c.mu.Lock()
	config := c.config
	c.mu.Unlock()

	start := time.Now()
	builder := NewReportBuilder()

	// 如果是增量探测，复用上一份报告的静态字段
	if !opts.Full && opts.PreviousReport != nil {
		for url, latency := range opts.PreviousReport.RelayLatencies {
			builder.AddRelayLatency(url, latency)
		}
		builder.SetPortMapAvailability(
			opts.PreviousReport.UPnPAvailable,
			opts.PreviousReport.NATPMPAvailable,
			opts.PreviousReport.PCPAvailable,
		)
	}

	// 创建带超时的上下文
	probeCtx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()

	// 使用信号量控制并发
	sem := make(chan struct{}, config.MaxConcurrentProbes)
	var wg sync.WaitGroup

	// IPv4 探测
	if config.EnableIPv4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			c.runIPv4Probes(probeCtx, builder)
		}()
	}

	// IPv6 探测
	if config.EnableIPv6 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			c.runIPv6Probes(probeCtx, builder)
		}()
	}

	// 中继延迟探测（仅完整探测时）
	if opts.Full && config.EnableRelayProbe && len(config.RelayServers) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			c.runRelayProbes(probeCtx, builder)
		}()
	}

	// 端口映射探测（仅完整探测时）
	if opts.Full && config.EnablePortMapProbe {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			c.runPortMapProbe(probeCtx, builder)
		}()
	}

	// 强制门户检测
	if config.EnableCaptivePortalProbe {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			c.runCaptivePortalProbe(probeCtx, builder)
		}()
	}

	// 等待所有探测完成
	wg.Wait()

	// 设置报告生成耗时
	builder.SetDuration(time.Since(start))

	report := builder.Build()

	// 缓存报告
	c.mu.Lock()
	c.lastReport = report
	c.lastReportTime = time.Now()
	c.mu.Unlock()

	return report, nil
}

// GetReportAsync 异步生成报告
func (c *Client) GetReportAsync(ctx context.Context) <-chan *Report {
	ch := make(chan *Report, 1)
	go func() {
		report, _ := c.GetReport(ctx)
		ch <- report
		close(ch)
	}()
	return ch
}

// LastReport 返回最后一次诊断报告
func (c *Client) LastReport() *Report {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastReport
}

// SetSTUNServers 设置 STUN 服务器列表
func (c *Client) SetSTUNServers(servers []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config.STUNServers = servers
	c.stunClient = NewSTUNClient(servers, c.config.ProbeTimeout)
}

// SetRelayServers 设置中继服务器列表
func (c *Client) SetRelayServers(relays []string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.config.RelayServers = relays
}

// ForceFullReport 强制下次生成完整报告
func (c *Client) ForceFullReport() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastReport = nil
	c.lastReportTime = time.Time{}
}

// ============================================================================
//                              探测方法
// ============================================================================

// runIPv4Probes 运行 IPv4 探测
func (c *Client) runIPv4Probes(ctx context.Context, builder *ReportBuilder) {
	logger.Debug("开始 IPv4 探测")

	c.mu.RLock()
	serverCount := len(c.config.STUNServers)
	probeTimeout := c.config.ProbeTimeout
	c.mu.RUnlock()

	probeCount := 5
	if serverCount > 0 && serverCount < probeCount {
		probeCount = serverCount
	}

	results, err := c.stunClient.ProbeMultiple(ctx, probeCount)
	if err != nil {
		logger.Warn("IPv4 探测失败", "err", err)
		builder.MarkSTUNFailure(false, err)
	}

	for _, r := range results {
		if r.GlobalIP.To4() != nil {
			builder.AddIPv4Mapping(r.Server, r.GlobalIP, r.Port)
		}
	}

	if len(results) > 0 {
		r := results[0]
		builder.SetUDPv4(true, r.GlobalIP, r.Port)
		return
	}

	// 兜底：若无结果，使用更长超时再次探测
	fallbackTimeout := 8 * time.Second
	if probeTimeout > fallbackTimeout {
		fallbackTimeout = probeTimeout
	}
	fallbackCount := 2
	if serverCount > 0 && serverCount < fallbackCount {
		fallbackCount = serverCount
	}
	if fallbackCount > 0 {
		builder.MarkSTUNFallbackUsed()
		logger.Warn("IPv4 探测无结果，启动兜底探测",
			"servers", serverCount,
			"fallbackTimeout", fallbackTimeout,
			"fallbackCount", fallbackCount)
		fallbackClient := NewSTUNClient(c.config.STUNServers, fallbackTimeout)
		fallbackResults, fallbackErr := fallbackClient.ProbeMultiple(ctx, fallbackCount)
		if fallbackErr != nil {
			logger.Warn("IPv4 兜底探测失败", "err", fallbackErr)
			builder.MarkSTUNFailure(false, fallbackErr)
		}
		for _, r := range fallbackResults {
			if r.GlobalIP.To4() != nil {
				builder.AddIPv4Mapping(r.Server, r.GlobalIP, r.Port)
			}
		}
		if len(fallbackResults) > 0 {
			r := fallbackResults[0]
			builder.SetUDPv4(true, r.GlobalIP, r.Port)
			return
		}
	}

	// 明确标记 IPv4 UDP 不可用
	builder.MarkSTUNFailure(false, fmt.Errorf("no IPv4 STUN results"))
	builder.SetUDPv4(false, nil, 0)
}

// runIPv6Probes 运行 IPv6 探测
func (c *Client) runIPv6Probes(ctx context.Context, builder *ReportBuilder) {
	logger.Debug("开始 IPv6 探测")

	// 检查系统是否支持 IPv6
	if !hasIPv6Support() {
		logger.Debug("系统不支持 IPv6")
		return
	}

	// 使用 IPv6 STUN 服务器
	ipv6Servers := []string{
		"stun.l.google.com:19302",
	}

	client := NewSTUNClient(ipv6Servers, c.config.ProbeTimeout)
	results, err := client.ProbeMultiple(ctx, len(ipv6Servers))
	if err != nil {
		logger.Debug("IPv6 探测失败", "err", err)
		builder.MarkSTUNFailure(true, err)
	}

	for _, r := range results {
		if r.GlobalIP.To4() == nil {
			builder.AddIPv6Mapping(r.Server, r.GlobalIP, r.Port)
		}
	}

	if len(results) > 0 {
		r := results[0]
		builder.SetUDPv6(true, r.GlobalIP, r.Port)
		return
	}

	// 明确标记 IPv6 UDP 不可用
	builder.MarkSTUNFailure(true, fmt.Errorf("no IPv6 STUN results"))
	builder.SetUDPv6(false, nil, 0)
}

// runRelayProbes 运行中继延迟探测
func (c *Client) runRelayProbes(ctx context.Context, builder *ReportBuilder) {
	logger.Debug("开始中继延迟探测")

	c.mu.RLock()
	relays := c.config.RelayServers
	c.mu.RUnlock()

	for _, relay := range relays {
		select {
		case <-ctx.Done():
			return
		default:
		}

		latency := c.measureRelayLatency(ctx, relay)
		if latency > 0 {
			builder.AddRelayLatency(relay, latency)
		}
	}
}

// measureRelayLatency 测量中继延迟
func (c *Client) measureRelayLatency(_ context.Context, relay string) time.Duration {
	start := time.Now()

	// 简单的 TCP 连接测试
	conn, err := net.DialTimeout("tcp", relay, c.config.ProbeTimeout)
	if err != nil {
		return 0
	}
	conn.Close()

	return time.Since(start)
}

// runPortMapProbe 运行端口映射协议探测
func (c *Client) runPortMapProbe(ctx context.Context, builder *ReportBuilder) {
	logger.Debug("开始端口映射协议探测")

	upnp := c.detectUPnP(ctx)
	natpmp := c.detectNATPMP(ctx)
	pcp := c.detectPCP(ctx)

	builder.SetPortMapAvailability(upnp, natpmp, pcp)
}

// detectUPnP 检测 UPnP 可用性
func (c *Client) detectUPnP(ctx context.Context) bool {
	// 发送 SSDP M-SEARCH 请求
	ssdpAddr := "239.255.255.250:1900"
	udpAddr, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		return false
	}

	conn, err := net.DialUDP("udp4", nil, udpAddr)
	if err != nil {
		return false
	}
	defer conn.Close()

	deadline := time.Now().Add(2 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn.SetDeadline(deadline)

	search := "M-SEARCH * HTTP/1.1\r\n" +
		"HOST: 239.255.255.250:1900\r\n" +
		"MAN: \"ssdp:discover\"\r\n" +
		"MX: 2\r\n" +
		"ST: urn:schemas-upnp-org:device:InternetGatewayDevice:1\r\n" +
		"\r\n"

	_, err = conn.Write([]byte(search))
	if err != nil {
		return false
	}

	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		return false
	}

	response := string(buf[:n])
	return strings.Contains(response, "InternetGatewayDevice") ||
		strings.Contains(response, "WANIPConnection") ||
		strings.Contains(response, "WANPPPConnection")
}

// detectNATPMP 检测 NAT-PMP 可用性
func (c *Client) detectNATPMP(ctx context.Context) bool {
	gateway := c.getDefaultGateway()
	if gateway == nil {
		return false
	}

	addr := &net.UDPAddr{IP: gateway, Port: 5351}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return false
	}
	defer conn.Close()

	deadline := time.Now().Add(2 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn.SetDeadline(deadline)

	// 发送外部地址请求 (opcode 0)
	request := []byte{0x00, 0x00}
	_, err = conn.Write(request)
	if err != nil {
		return false
	}

	buf := make([]byte, 12)
	n, err := conn.Read(buf)
	if err != nil {
		return false
	}

	// 验证响应格式
	if n >= 12 && buf[0] == 0 && buf[1] == 128 {
		resultCode := uint16(buf[2])<<8 | uint16(buf[3])
		return resultCode == 0
	}

	return false
}

// detectPCP 检测 PCP 可用性
func (c *Client) detectPCP(_ context.Context) bool {
	// PCP 检测比较复杂，暂时返回 false
	return false
}

// getDefaultGateway 获取默认网关地址
func (c *Client) getDefaultGateway() net.IP {
	// 尝试常见的网关地址
	commonGateways := []string{
		"192.168.1.1",
		"192.168.0.1",
		"10.0.0.1",
		"172.16.0.1",
	}

	for _, gw := range commonGateways {
		ip := net.ParseIP(gw)
		if ip != nil {
			conn, err := net.DialTimeout("udp", gw+":1", 100*time.Millisecond)
			if err == nil {
				conn.Close()
				return ip
			}
		}
	}

	return nil
}

// runCaptivePortalProbe 运行强制门户检测
func (c *Client) runCaptivePortalProbe(ctx context.Context, builder *ReportBuilder) {
	logger.Debug("开始强制门户检测")

	detected := c.detectCaptivePortal(ctx)
	builder.SetCaptivePortal(detected)
}

// detectCaptivePortal 检测强制门户
func (c *Client) detectCaptivePortal(ctx context.Context) bool {
	endpoints := []struct {
		url      string
		expected string
	}{
		{"http://captive.apple.com/hotspot-detect.html", "Success"},
		{"http://www.msftconnecttest.com/connecttest.txt", "Microsoft Connect Test"},
		{"http://connectivitycheck.gstatic.com/generate_204", ""},
	}

	for _, ep := range endpoints {
		detected := c.checkCaptivePortalEndpoint(ctx, ep.url, ep.expected)
		if detected {
			return true
		}
	}

	return false
}

// checkCaptivePortalEndpoint 检查单个强制门户检测端点
func (c *Client) checkCaptivePortalEndpoint(ctx context.Context, url, expected string) bool {
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	// 如果是重定向，可能存在强制门户
	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		return true
	}

	// 对于 generate_204 端点，应该返回 204
	if strings.Contains(url, "generate_204") {
		return resp.StatusCode != 204
	}

	// 检查响应内容
	if expected != "" {
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1024))
		if err != nil {
			return false
		}
		return !strings.Contains(string(body), expected)
	}

	return false
}

// ============================================================================
//                              辅助函数
// ============================================================================

// hasIPv6Support 检查系统是否支持 IPv6
func hasIPv6Support() bool {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return false
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ipnet.IP.To4() == nil && !ipnet.IP.IsLoopback() {
				return true
			}
		}
	}

	return false
}

// ProbeOptions 探测选项
type ProbeOptions struct {
	// Full 是否运行完整探测
	Full bool

	// PreviousReport 上一份报告（用于增量探测时复用字段）
	PreviousReport *Report
}
