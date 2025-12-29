package netreport

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackpal/gateway"

	"github.com/dep2p/go-dep2p/internal/core/netreport/probes"
	netreportif "github.com/dep2p/go-dep2p/pkg/interfaces/netreport"
)

// Prober 网络探测器
//
// 协调并发执行多种网络探测
type Prober struct {
	config   netreportif.Config
	configMu sync.RWMutex // 保护 config 的并发访问

	// 探测器
	ipv4Prober  *probes.IPv4Prober
	ipv6Prober  *probes.IPv6Prober
	natProber   *probes.NATProber
	relayProber *probes.RelayProber
}

// NewProber 创建探测器
func NewProber(config netreportif.Config) *Prober {
	// 创建各个子探测器
	ipv4Prober := probes.NewIPv4Prober(config.STUNServers, config.ProbeTimeout)
	ipv6Prober := probes.NewIPv6Prober(config.STUNServers, config.ProbeTimeout)
	natProber := probes.NewNATProber(config.STUNServers, config.ProbeTimeout)
	relayProber := probes.NewRelayProber(config.RelayServers, config.ProbeTimeout)

	return &Prober{
		config:      config,
		ipv4Prober:  ipv4Prober,
		ipv6Prober:  ipv6Prober,
		natProber:   natProber,
		relayProber: relayProber,
	}
}

// RunProbes 运行所有探测
func (p *Prober) RunProbes(ctx context.Context) *netreportif.Report {
	start := time.Now()
	builder := NewReportBuilder()

	// 获取配置快照
	p.configMu.RLock()
	config := p.config
	p.configMu.RUnlock()

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

			p.runIPv4Probes(probeCtx, builder)
		}()
	}

	// IPv6 探测
	if config.EnableIPv6 && probes.HasIPv6Support() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			p.runIPv6Probes(probeCtx, builder)
		}()
	}

	// 中继延迟探测
	if config.EnableRelayProbe && len(config.RelayServers) > 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			p.runRelayProbes(probeCtx, builder)
		}()
	}

	// 端口映射探测
	if config.EnablePortMapProbe {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			p.runPortMapProbe(probeCtx, builder)
		}()
	}

	// 强制门户检测
	if config.EnableCaptivePortalProbe {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			p.runCaptivePortalProbe(probeCtx, builder)
		}()
	}

	// 等待所有探测完成
	wg.Wait()

	// 设置报告生成耗时
	builder.SetDuration(time.Since(start))

	return builder.Build()
}

// runIPv4Probes 运行 IPv4 探测
func (p *Prober) runIPv4Probes(ctx context.Context, builder *ReportBuilder) {
	log.Debug("开始 IPv4 探测")

	// 多服务器探测（用于 NAT 类型检测）
	results, err := p.ipv4Prober.ProbeMultiple(ctx, 3)
	if err != nil {
		log.Warn("IPv4 多服务器探测失败", "err", err)
		return
	}

	for _, r := range results {
		builder.AddIPv4Mapping(r.Server, r.GlobalIP, r.GlobalPort)
	}

	if len(results) > 0 {
		builder.SetUDPv4(true, results[0].GlobalIP, results[0].GlobalPort)
	}
}

// runIPv6Probes 运行 IPv6 探测
func (p *Prober) runIPv6Probes(ctx context.Context, builder *ReportBuilder) {
	log.Debug("开始 IPv6 探测")

	// 多服务器探测（用于 NAT 类型检测）
	results, err := p.ipv6Prober.ProbeMultiple(ctx, 3)
	if err != nil {
		log.Warn("IPv6 多服务器探测失败", "err", err)
		return
	}

	for _, r := range results {
		builder.AddIPv6Mapping(r.Server, r.GlobalIP, r.GlobalPort)
	}

	if len(results) > 0 {
		builder.SetUDPv6(true, results[0].GlobalIP, results[0].GlobalPort)
	}
}

// runRelayProbes 运行中继延迟探测
func (p *Prober) runRelayProbes(ctx context.Context, builder *ReportBuilder) {
	log.Debug("开始中继延迟探测")

	results, err := p.relayProber.ProbeAll(ctx)
	if err != nil {
		log.Warn("中继延迟探测失败", "err", err)
		return
	}

	for _, r := range results {
		builder.AddRelayLatency(r.URL, r.Latency)
	}
}

// runPortMapProbe 运行端口映射协议探测
func (p *Prober) runPortMapProbe(ctx context.Context, builder *ReportBuilder) {
	log.Debug("开始端口映射协议探测")

	// 使用真实协议探测 UPnP、NAT-PMP 和 PCP 可用性
	upnp := p.detectUPnP(ctx)
	natpmp := p.detectNATPMP(ctx)
	pcp := p.detectPCP(ctx)

	builder.SetPortMapAvailability(upnp, natpmp, pcp)
}

// detectUPnP 检测 UPnP 可用性
func (p *Prober) detectUPnP(ctx context.Context) bool {
	// 发送 SSDP M-SEARCH 请求检测 UPnP IGD
	ssdpAddr := "239.255.255.250:1900"
	udpAddr, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		return false
	}

	conn, err := net.DialUDP("udp4", nil, udpAddr)
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()

	// 设置超时
	deadline := time.Now().Add(2 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn.SetDeadline(deadline)

	// 构造 M-SEARCH 请求
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

	// 等待响应
	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		return false
	}

	// 检查响应是否包含 IGD
	response := string(buf[:n])
	return strings.Contains(response, "InternetGatewayDevice") ||
		strings.Contains(response, "WANIPConnection") ||
		strings.Contains(response, "WANPPPConnection")
}

// detectNATPMP 检测 NAT-PMP 可用性
func (p *Prober) detectNATPMP(ctx context.Context) bool {
	// NAT-PMP 服务运行在网关的 5351 端口
	// 获取默认网关地址
	gateway := p.getDefaultGateway()
	if gateway == nil {
		return false
	}

	addr := &net.UDPAddr{IP: gateway, Port: 5351}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()

	// 设置超时
	deadline := time.Now().Add(2 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn.SetDeadline(deadline)

	// 发送外部地址请求 (opcode 0)
	// 格式: [version(1)][opcode(1)]
	request := []byte{0x00, 0x00}
	_, err = conn.Write(request)
	if err != nil {
		return false
	}

	// 读取响应
	buf := make([]byte, 12)
	n, err := conn.Read(buf)
	if err != nil {
		return false
	}

	// 验证响应格式
	// [version(1)][opcode(1)][result(2)][epoch(4)][external_ip(4)]
	if n >= 12 && buf[0] == 0 && buf[1] == 128 {
		// opcode 128 = 0 + 128，表示响应
		resultCode := uint16(buf[2])<<8 | uint16(buf[3])
		return resultCode == 0 // 0 = success
	}

	return false
}

// detectPCP 检测 PCP (Port Control Protocol) 可用性
// PCP 是 NAT-PMP 的继任者 (RFC 6887)，支持 IPv6 和 CGN 场景
func (p *Prober) detectPCP(ctx context.Context) bool {
	// PCP 使用与 NAT-PMP 相同的端口 5351，但协议版本为 2
	gateway := p.getDefaultGateway()
	if gateway == nil {
		return false
	}

	addr := &net.UDPAddr{IP: gateway, Port: 5351}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return false
	}
	defer func() { _ = conn.Close() }()

	// 设置超时
	deadline := time.Now().Add(2 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn.SetDeadline(deadline)

	// 构造 PCP MAP 请求 (RFC 6887)
	// 请求格式:
	// [Version(1)][OpCode(1)][Reserved(2)][Lifetime(4)][ClientIP(16)]
	// [Nonce(12)][Protocol(1)][Reserved(3)][InternalPort(2)][ExternalPort(2)][ExternalIP(16)]
	request := make([]byte, 60)

	// Header
	request[0] = 2 // Version = 2 (PCP)
	request[1] = 1 // OpCode = 1 (MAP)
	request[2] = 0 // Reserved
	request[3] = 0 // Reserved
	request[4] = 0 // Lifetime (4 bytes) = 0 for probe
	request[5] = 0
	request[6] = 0
	request[7] = 0

	// Client IP (16 bytes) - IPv4-mapped IPv6 地址
	// 获取本地 IP
	localAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok || localAddr == nil {
		return false
	}
	if localAddr.IP.To4() != nil {
		// IPv4-mapped IPv6: ::ffff:x.x.x.x
		copy(request[8:20], []byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0xff, 0xff})
		copy(request[20:24], localAddr.IP.To4())
	} else {
		copy(request[8:24], localAddr.IP.To16())
	}

	// MAP 请求特定字段 (从偏移 24 开始)
	// Nonce (12 bytes) - 随机数
	for i := 24; i < 36; i++ {
		request[i] = byte(i) // 简单的非零值用于探测
	}

	request[36] = 17 // Protocol = 17 (UDP)
	request[37] = 0  // Reserved
	request[38] = 0  // Reserved
	request[39] = 0  // Reserved
	request[40] = 0  // Internal Port (2 bytes) = 0 (探测)
	request[41] = 0
	request[42] = 0 // Suggested External Port (2 bytes) = 0
	request[43] = 0
	// External IP (16 bytes) = all zeros (让服务器选择)
	// request[44:60] 已经是 0

	_, err = conn.Write(request)
	if err != nil {
		return false
	}

	// 读取响应
	buf := make([]byte, 60)
	n, err := conn.Read(buf)
	if err != nil {
		return false
	}

	// 验证 PCP 响应格式
	// [Version(1)][R+OpCode(1)][Reserved(1)][ResultCode(1)][Lifetime(4)][Epoch(4)][Reserved(12)]
	// 响应的 OpCode 最高位为 1
	if n >= 24 && buf[0] == 2 && (buf[1]&0x80) != 0 {
		// Version = 2, R bit set (response)
		resultCode := buf[3]
		// ResultCode 0 = SUCCESS, 1 = UNSUPP_VERSION, 2 = NOT_AUTHORIZED, etc.
		// 即使返回错误码，只要能响应就说明 PCP 服务存在
		log.Debug("PCP 响应",
			"resultCode", resultCode,
			"success", resultCode == 0)
		return resultCode == 0 || resultCode == 1 || resultCode == 2
	}

	return false
}

// getDefaultGateway 获取默认网关地址
func (p *Prober) getDefaultGateway() net.IP {
	// 使用 jackpal/gateway 库获取真实的默认网关
	gatewayIP, err := gateway.DiscoverGateway()
	if err == nil && gatewayIP != nil {
		log.Debug("发现默认网关", "gateway", gatewayIP.String())
		return gatewayIP
	}

	log.Debug("无法通过系统路由表获取网关，尝试常见地址", "err", err)

	// 回退：尝试常见的网关地址
	commonGateways := []string{
		"192.168.1.1",
		"192.168.0.1",
		"10.0.0.1",
		"172.16.0.1",
	}

	for _, gw := range commonGateways {
		ip := net.ParseIP(gw)
		if ip != nil {
			// 尝试连接测试
			conn, err := net.DialTimeout("udp", gw+":1", 100*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				log.Debug("使用常见网关地址", "gateway", gw)
				return ip
			}
		}
	}

	return nil
}

// runCaptivePortalProbe 运行强制门户检测
func (p *Prober) runCaptivePortalProbe(ctx context.Context, builder *ReportBuilder) {
	log.Debug("开始强制门户检测")

	detected := p.detectCaptivePortal(ctx)
	builder.SetCaptivePortal(detected)
}

// detectCaptivePortal 检测强制门户
func (p *Prober) detectCaptivePortal(ctx context.Context) bool {
	// 使用常见的强制门户检测端点
	// 如果返回的内容与预期不符，则认为存在强制门户

	endpoints := []struct {
		url      string
		expected string
	}{
		{"http://captive.apple.com/hotspot-detect.html", "Success"},
		{"http://www.msftconnecttest.com/connecttest.txt", "Microsoft Connect Test"},
		{"http://connectivitycheck.gstatic.com/generate_204", ""},
	}

	for _, ep := range endpoints {
		detected := p.checkCaptivePortalEndpoint(ctx, ep.url, ep.expected)
		if detected {
			return true
		}
	}

	return false
}

// checkCaptivePortalEndpoint 检查单个强制门户检测端点
func (p *Prober) checkCaptivePortalEndpoint(ctx context.Context, url, expected string) bool {
	// 创建 HTTP 客户端，禁用重定向跟随
	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			// 禁止重定向跟随，强制门户通常会重定向
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
	defer func() { _ = resp.Body.Close() }()

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
		// 如果内容不匹配预期，可能存在强制门户
		return !strings.Contains(string(body), expected)
	}

	return false
}

// UpdateConfig 更新配置
func (p *Prober) UpdateConfig(config netreportif.Config) {
	p.configMu.Lock()
	p.config = config
	p.configMu.Unlock()

	// 重新创建探测器
	p.ipv4Prober = probes.NewIPv4Prober(config.STUNServers, config.ProbeTimeout)
	p.ipv6Prober = probes.NewIPv6Prober(config.STUNServers, config.ProbeTimeout)
	p.natProber = probes.NewNATProber(config.STUNServers, config.ProbeTimeout)
	p.relayProber = probes.NewRelayProber(config.RelayServers, config.ProbeTimeout)
}
