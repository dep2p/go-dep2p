package probes

import (
	"context"
	"fmt"
	"net"
	"time"

	netreportif "github.com/dep2p/go-dep2p/pkg/interfaces/netreport"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// NATProber NAT 类型探测器
type NATProber struct {
	stunServers []string
	timeout     time.Duration
}

// NewNATProber 创建 NAT 类型探测器
func NewNATProber(stunServers []string, timeout time.Duration) *NATProber {
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	return &NATProber{
		stunServers: stunServers,
		timeout:     timeout,
	}
}

// Probe 执行 NAT 类型探测
func (p *NATProber) Probe(ctx context.Context) (*netreportif.ProbeResult, error) {
	log.Debug("开始 NAT 类型探测")

	result := &netreportif.ProbeResult{
		Type:    netreportif.ProbeTypeNAT,
		Success: false,
	}

	start := time.Now()

	// 需要至少 2 个服务器来检测对称 NAT
	if len(p.stunServers) < 2 {
		result.Error = fmt.Errorf("需要至少 2 个 STUN 服务器来检测 NAT 类型")
		result.Latency = time.Since(start)
		return result, result.Error
	}

	// 对多个服务器进行探测
	var mappings []mappingInfo
	for i := 0; i < minInt(3, len(p.stunServers)); i++ {
		select {
		case <-ctx.Done():
			result.Error = ctx.Err()
			return result, ctx.Err()
		default:
		}

		mapping, err := p.probeServer(ctx, p.stunServers[i])
		if err != nil {
			log.Debug("NAT 探测服务器失败",
				"server", p.stunServers[i],
				"err", err)
			continue
		}
		mappings = append(mappings, *mapping)
	}

	if len(mappings) < 2 {
		result.Error = fmt.Errorf("无法从足够多的服务器获取映射")
		result.Latency = time.Since(start)
		return result, result.Error
	}

	// 分析 NAT 类型
	natType, mappingVaries := p.analyzeNATType(mappings)

	result.Success = true
	result.Latency = time.Since(start)
	result.Data = &netreportif.NATProbeData{
		NATType:       natType,
		MappingVaries: mappingVaries,
	}

	log.Debug("NAT 类型探测完成",
		"natType", natType.String(),
		"mappingVaries", mappingVaries)

	return result, nil
}

// mappingInfo 映射信息
type mappingInfo struct {
	server string
	ip     net.IP
	port   uint16
}

// probeServer 探测单个 STUN 服务器获取映射
func (p *NATProber) probeServer(ctx context.Context, server string) (*mappingInfo, error) {
	// 解析服务器地址
	addr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		return nil, fmt.Errorf("解析 STUN 服务器地址失败: %w", err)
	}

	// 确定网络类型
	network := "udp4"
	if addr.IP.To4() == nil {
		network = "udp6"
	}

	// 创建本地 UDP 连接
	conn, err := net.ListenUDP(network, nil)
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

	return &mappingInfo{
		server: server,
		ip:     ip,
		port:   port,
	}, nil
}

// analyzeNATType 分析 NAT 类型
func (p *NATProber) analyzeNATType(mappings []mappingInfo) (types.NATType, bool) {
	if len(mappings) < 2 {
		return types.NATTypeUnknown, false
	}

	// 比较映射是否一致
	first := mappings[0]
	mappingVaries := false

	for _, m := range mappings[1:] {
		if !first.ip.Equal(m.ip) || first.port != m.port {
			mappingVaries = true
			break
		}
	}

	if mappingVaries {
		// 映射随目标变化 = 对称 NAT
		return types.NATTypeSymmetric, true
	}

	// 映射不变 (Endpoint-Independent Mapping)
	// 要区分 Full Cone、Restricted Cone、Port Restricted Cone 需要:
	// - Full Cone: 任何外部主机都可以向映射地址发送数据
	// - Restricted Cone: 只有节点主动联系过的 IP 可以发送数据
	// - Port Restricted Cone: 只有节点主动联系过的 IP:Port 可以发送数据
	//
	// 需要 STUN 服务器支持 CHANGE-REQUEST (RFC 5780) 来区分
	// 由于大多数公共 STUN 服务器不支持此功能，我们基于映射行为推断:
	// - 如果映射是 EIM（已验证），那么可能是 Full Cone 或 Restricted Cone
	// - 报告为 Full Cone，但实际可能是 Restricted
	return types.NATTypeFull, false
}

// minInt 返回两个整数中的较小值
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// DetectNATTypeAdvanced 高级 NAT 类型检测
//
// 使用多服务器探测方法检测 NAT 类型
// RFC 5780 定义了完整的检测流程，但需要服务器支持 CHANGE-REQUEST
// 本实现使用多个独立服务器来模拟部分功能
func (p *NATProber) DetectNATTypeAdvanced(ctx context.Context) (types.NATType, error) {
	log.Debug("开始高级 NAT 类型检测")

	if len(p.stunServers) == 0 {
		return types.NATTypeUnknown, fmt.Errorf("无可用的 STUN 服务器")
	}

	// 第一步：基本连通性测试
	server := p.stunServers[0]
	mapping1, err := p.probeServer(ctx, server)
	if err != nil {
		return types.NATTypeUnknown, fmt.Errorf("基本连通性测试失败: %w", err)
	}
	log.Debug("第一次映射结果",
		"server", server,
		"mappedIP", mapping1.ip.String(),
		"mappedPort", mapping1.port)

	// 第二步：检测 Endpoint-Independent Mapping (EIM)
	// 向不同服务器发送请求，检查映射是否变化
	if len(p.stunServers) >= 2 {
		mapping2, err := p.probeServer(ctx, p.stunServers[1])
		if err == nil {
			log.Debug("第二次映射结果",
				"server", p.stunServers[1],
				"mappedIP", mapping2.ip.String(),
				"mappedPort", mapping2.port)

			if !mapping1.ip.Equal(mapping2.ip) || mapping1.port != mapping2.port {
				// 映射随目标变化 = Endpoint-Dependent Mapping = Symmetric NAT
				log.Info("检测到对称型 NAT (映射随目标变化)")
				return types.NATTypeSymmetric, nil
			}
		}
	}

	// 第三步：如果有第三个服务器，进一步验证
	if len(p.stunServers) >= 3 {
		mapping3, err := p.probeServer(ctx, p.stunServers[2])
		if err == nil && (!mapping1.ip.Equal(mapping3.ip) || mapping1.port != mapping3.port) {
			log.Info("检测到对称型 NAT (第三服务器验证)")
			return types.NATTypeSymmetric, nil
		}
	}

	// 映射是 Endpoint-Independent
	// 过滤行为需要 CHANGE-REQUEST 来检测，目前无法区分
	// Full Cone / Restricted Cone / Port Restricted Cone
	log.Info("检测到 Endpoint-Independent Mapping (可能为 Full Cone 或 Restricted Cone)")
	return types.NATTypeFull, nil
}

// ProbeWithLocalAddr 使用指定本地地址探测
//
// 用于检测 Endpoint-Independent Mapping (EIM)
func (p *NATProber) ProbeWithLocalAddr(ctx context.Context, localAddr *net.UDPAddr) (*mappingInfo, error) {
	if len(p.stunServers) == 0 {
		return nil, fmt.Errorf("无可用的 STUN 服务器")
	}

	server := p.stunServers[0]
	addr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		return nil, fmt.Errorf("解析 STUN 服务器地址失败: %w", err)
	}

	// 创建绑定到指定地址的连接
	conn, err := net.DialUDP("udp", localAddr, addr)
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

	// 发送 STUN 请求
	bindingRequest := buildSTUNBindingRequest()
	_, err = conn.Write(bindingRequest)
	if err != nil {
		return nil, fmt.Errorf("发送 STUN 请求失败: %w", err)
	}

	// 接收响应
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("接收 STUN 响应失败: %w", err)
	}

	// 解析响应
	ip, port, err := parseSTUNBindingResponse(buf[:n])
	if err != nil {
		return nil, fmt.Errorf("解析 STUN 响应失败: %w", err)
	}

	return &mappingInfo{
		server: server,
		ip:     ip,
		port:   port,
	}, nil
}

