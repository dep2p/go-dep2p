package probes

import (
	"context"
	"fmt"
	"net"
	"time"

	netreportif "github.com/dep2p/go-dep2p/pkg/interfaces/netreport"
)

// IPv6Prober IPv6 连通性探测器
type IPv6Prober struct {
	stunServers []string
	timeout     time.Duration
}

// NewIPv6Prober 创建 IPv6 探测器
func NewIPv6Prober(stunServers []string, timeout time.Duration) *IPv6Prober {
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	// 过滤出可能支持 IPv6 的服务器
	var ipv6Servers []string
	for _, s := range stunServers {
		// 尝试解析为 IPv6
		host, port, err := net.SplitHostPort(s)
		if err != nil {
			continue
		}
		// 检查是否能解析到 IPv6 地址
		ips, err := net.LookupIP(host)
		if err != nil {
			continue
		}
		for _, ip := range ips {
			if ip.To4() == nil && ip.To16() != nil {
				ipv6Servers = append(ipv6Servers, net.JoinHostPort(ip.String(), port))
				break
			}
		}
	}

	return &IPv6Prober{
		stunServers: ipv6Servers,
		timeout:     timeout,
	}
}

// Probe 执行 IPv6 连通性探测
func (p *IPv6Prober) Probe(ctx context.Context) (*netreportif.ProbeResult, error) {
	log.Debug("开始 IPv6 连通性探测")

	result := &netreportif.ProbeResult{
		Type:    netreportif.ProbeTypeIPv6,
		Success: false,
	}

	start := time.Now()

	// 检查是否有可用的 IPv6 服务器
	if len(p.stunServers) == 0 {
		result.Error = fmt.Errorf("无可用的 IPv6 STUN 服务器")
		result.Latency = time.Since(start)
		return result, result.Error
	}

	// 尝试每个 STUN 服务器
	for _, server := range p.stunServers {
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			return result, ctx.Err()
		default:
		}

		probeResult, err := p.probeServer(ctx, server)
		if err != nil {
			log.Debug("IPv6 探测服务器失败",
				"server", server,
				"err", err)
			continue
		}

		result.Success = true
		result.Latency = time.Since(start)
		result.Data = probeResult

		log.Debug("IPv6 探测成功",
			"server", server,
			"globalIP", probeResult.GlobalIP.String(),
			"globalPort", probeResult.GlobalPort)

		return result, nil
	}

	result.Latency = time.Since(start)
	result.Error = fmt.Errorf("所有 IPv6 STUN 服务器均不可达")

	log.Warn("IPv6 探测失败: 无可用服务器")
	return result, result.Error
}

// ProbeMultiple 对多个服务器进行探测（用于 NAT 类型检测）
func (p *IPv6Prober) ProbeMultiple(ctx context.Context, count int) ([]*netreportif.IPv6ProbeData, error) {
	var results []*netreportif.IPv6ProbeData

	for i, server := range p.stunServers {
		if i >= count {
			break
		}

		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}

		probeResult, err := p.probeServer(ctx, server)
		if err != nil {
			log.Debug("IPv6 多服务器探测失败",
				"server", server,
				"err", err)
			continue
		}

		results = append(results, probeResult)
	}

	return results, nil
}

// probeServer 探测单个 STUN 服务器
func (p *IPv6Prober) probeServer(ctx context.Context, server string) (*netreportif.IPv6ProbeData, error) {
	// 解析服务器地址
	addr, err := net.ResolveUDPAddr("udp6", server)
	if err != nil {
		return nil, fmt.Errorf("解析 STUN 服务器地址失败: %w", err)
	}

	// 创建本地 UDP 连接
	conn, err := net.ListenUDP("udp6", nil)
	if err != nil {
		return nil, fmt.Errorf("创建 UDP 连接失败: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// 设置超时
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(p.timeout)
	}
	conn.SetDeadline(deadline)

	// 发送 STUN Binding Request
	bindingRequest := buildSTUNBindingRequest()
	_, err = conn.WriteToUDP(bindingRequest, addr)
	if err != nil {
		return nil, fmt.Errorf("发送 STUN 请求失败: %w", err)
	}

	// 接收响应
	buf := make([]byte, 1024)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		return nil, fmt.Errorf("接收 STUN 响应失败: %w", err)
	}

	// 解析响应
	ip, port, err := parseSTUNBindingResponse(buf[:n])
	if err != nil {
		return nil, fmt.Errorf("解析 STUN 响应失败: %w", err)
	}

	// 验证是 IPv6
	if ip.To4() != nil {
		return nil, fmt.Errorf("收到 IPv4 地址: %s", ip.String())
	}

	return &netreportif.IPv6ProbeData{
		GlobalIP:   ip,
		GlobalPort: port,
		Server:     server,
	}, nil
}

// HasIPv6Support 检查本地是否有 IPv6 支持
func HasIPv6Support() bool {
	interfaces, err := net.Interfaces()
	if err != nil {
		return false
	}

	for _, iface := range interfaces {
		// 跳过回环和未启用的接口
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// 检查是否是全局单播 IPv6 地址
			if ip != nil && ip.To4() == nil && ip.To16() != nil && ip.IsGlobalUnicast() {
				return true
			}
		}
	}

	return false
}

