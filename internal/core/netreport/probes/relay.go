package probes

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/quic-go/quic-go"

	netreportif "github.com/dep2p/go-dep2p/pkg/interfaces/netreport"
)

// RelayProber 中继延迟探测器
type RelayProber struct {
	relayServers []string
	timeout      time.Duration
	httpClient   *http.Client
}

// NewRelayProber 创建中继探测器
func NewRelayProber(relayServers []string, timeout time.Duration) *RelayProber {
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	// 创建 HTTP 客户端用于 HTTPS 中继
	httpClient := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
			},
			DialContext: (&net.Dialer{
				Timeout:   timeout,
				KeepAlive: 30 * time.Second,
			}).DialContext,
		},
	}

	return &RelayProber{
		relayServers: relayServers,
		timeout:      timeout,
		httpClient:   httpClient,
	}
}

// Probe 执行中继延迟探测（单个最佳中继）
func (p *RelayProber) Probe(ctx context.Context) (*netreportif.ProbeResult, error) {
	log.Debug("开始中继延迟探测")

	result := &netreportif.ProbeResult{
		Type:    netreportif.ProbeTypeRelay,
		Success: false,
	}

	start := time.Now()

	if len(p.relayServers) == 0 {
		result.Error = fmt.Errorf("无配置的中继服务器")
		result.Latency = time.Since(start)
		return result, result.Error
	}

	// 并发探测所有中继
	results := p.probeAll(ctx)
	if len(results) == 0 {
		result.Error = fmt.Errorf("所有中继服务器均不可达")
		result.Latency = time.Since(start)
		return result, result.Error
	}

	// 找到延迟最低的中继
	var best *netreportif.RelayProbeData
	for _, r := range results {
		if best == nil || r.Latency < best.Latency {
			best = r
		}
	}

	result.Success = true
	result.Latency = time.Since(start)
	result.Data = best

	log.Debug("中继延迟探测完成",
		"bestRelay", best.URL,
		"latency", best.Latency)

	return result, nil
}

// ProbeAll 探测所有中继服务器
func (p *RelayProber) ProbeAll(ctx context.Context) ([]*netreportif.RelayProbeData, error) {
	log.Debug("开始探测所有中继服务器")

	if len(p.relayServers) == 0 {
		return nil, fmt.Errorf("无配置的中继服务器")
	}

	results := p.probeAll(ctx)
	if len(results) == 0 {
		return nil, fmt.Errorf("所有中继服务器均不可达")
	}

	return results, nil
}

// probeAll 并发探测所有中继
func (p *RelayProber) probeAll(ctx context.Context) []*netreportif.RelayProbeData {
	var wg sync.WaitGroup
	resultsChan := make(chan *netreportif.RelayProbeData, len(p.relayServers))

	for _, relay := range p.relayServers {
		wg.Add(1)
		go func(relayURL string) {
			defer wg.Done()

			result, err := p.probeRelay(ctx, relayURL)
			if err != nil {
				log.Debug("中继探测失败",
					"relay", relayURL,
					"err", err)
				return
			}
			resultsChan <- result
		}(relay)
	}

	// 等待所有探测完成
	go func() {
		wg.Wait()
		close(resultsChan)
	}()

	// 收集结果
	var results []*netreportif.RelayProbeData
	for result := range resultsChan {
		results = append(results, result)
	}

	return results
}

// probeRelay 探测单个中继服务器
func (p *RelayProber) probeRelay(ctx context.Context, relayURL string) (*netreportif.RelayProbeData, error) {
	// 解析 URL
	u, err := url.Parse(relayURL)
	if err != nil {
		return nil, fmt.Errorf("解析中继 URL 失败: %w", err)
	}

	var latency time.Duration

	switch u.Scheme {
	case "https", "http":
		// HTTP(S) 中继
		latency, err = p.probeHTTPRelay(ctx, relayURL)
	case "quic", "":
		// QUIC 中继
		latency, err = p.probeQUICRelay(ctx, u.Host)
	default:
		// 尝试 TCP 连接
		latency, err = p.probeTCPRelay(ctx, u.Host)
	}

	if err != nil {
		return nil, err
	}

	return &netreportif.RelayProbeData{
		URL:     relayURL,
		Latency: latency,
	}, nil
}

// probeHTTPRelay 探测 HTTP(S) 中继
func (p *RelayProber) probeHTTPRelay(ctx context.Context, relayURL string) (time.Duration, error) {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, "HEAD", relayURL, nil)
	if err != nil {
		return 0, fmt.Errorf("创建请求失败: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	latency := time.Since(start)

	// 任何响应都算成功（只要能连接）
	return latency, nil
}

// probeQUICRelay 探测 QUIC 中继
// 使用 quic-go 库建立真实的 QUIC 连接，测量完整握手延迟
func (p *RelayProber) probeQUICRelay(ctx context.Context, host string) (time.Duration, error) {
	start := time.Now()

	// 创建 TLS 配置
	// InsecureSkipVerify 用于延迟探测，避免证书验证失败
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,                    //nolint:gosec // G402: 这是延迟探测工具，不需要证书验证
		NextProtos:         []string{"h3", "dep2p"}, // HTTP/3 或 dep2p 协议
		MinVersion:         tls.VersionTLS12,        // 设置最低 TLS 版本
	}

	// QUIC 配置
	quicConf := &quic.Config{
		HandshakeIdleTimeout: p.timeout,
		MaxIdleTimeout:       p.timeout,
	}

	// 创建带超时的上下文
	probeCtx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	// 尝试建立 QUIC 连接
	conn, err := quic.DialAddr(probeCtx, host, tlsConf, quicConf)
	if err != nil {
		return 0, fmt.Errorf("QUIC 连接失败: %w", err)
	}

	// 测量到握手完成的延迟
	latency := time.Since(start)

	// 优雅关闭连接
	_ = conn.CloseWithError(0, "probe complete")

	log.Debug("QUIC 探测完成",
		"host", host,
		"latency", latency)

	return latency, nil
}

// probeTCPRelay 探测 TCP 中继
func (p *RelayProber) probeTCPRelay(ctx context.Context, host string) (time.Duration, error) {
	start := time.Now()

	var d net.Dialer
	d.Timeout = p.timeout

	conn, err := d.DialContext(ctx, "tcp", host)
	if err != nil {
		return 0, fmt.Errorf("TCP 连接失败: %w", err)
	}
	defer func() { _ = conn.Close() }()

	latency := time.Since(start)

	return latency, nil
}

// SortByLatency 按延迟排序中继结果
func SortByLatency(results []*netreportif.RelayProbeData) {
	for i := 0; i < len(results)-1; i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Latency < results[i].Latency {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
}

// FilterByMaxLatency 过滤超过最大延迟的中继
func FilterByMaxLatency(results []*netreportif.RelayProbeData, maxLatency time.Duration) []*netreportif.RelayProbeData {
	var filtered []*netreportif.RelayProbeData
	for _, r := range results {
		if r.Latency <= maxLatency {
			filtered = append(filtered, r)
		}
	}
	return filtered
}
