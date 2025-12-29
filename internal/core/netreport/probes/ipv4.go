// Package probes 提供网络诊断探测器实现
package probes

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"time"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	netreportif "github.com/dep2p/go-dep2p/pkg/interfaces/netreport"
)

// 包级别日志实例
var log = logger.Logger("netreport.probes")

// IPv4Prober IPv4 连通性探测器
type IPv4Prober struct {
	stunServers []string
	timeout     time.Duration
}

// NewIPv4Prober 创建 IPv4 探测器
func NewIPv4Prober(stunServers []string, timeout time.Duration) *IPv4Prober {
	if timeout == 0 {
		timeout = 5 * time.Second
	}

	return &IPv4Prober{
		stunServers: stunServers,
		timeout:     timeout,
	}
}

// Probe 执行 IPv4 连通性探测
func (p *IPv4Prober) Probe(ctx context.Context) (*netreportif.ProbeResult, error) {
	log.Debug("开始 IPv4 连通性探测")

	result := &netreportif.ProbeResult{
		Type:    netreportif.ProbeTypeIPv4,
		Success: false,
	}

	start := time.Now()

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
			log.Debug("IPv4 探测服务器失败",
				"server", server,
				"err", err)
			continue
		}

		result.Success = true
		result.Latency = time.Since(start)
		result.Data = probeResult

		log.Debug("IPv4 探测成功",
			"server", server,
			"globalIP", probeResult.GlobalIP.String(),
			"globalPort", probeResult.GlobalPort)

		return result, nil
	}

	result.Latency = time.Since(start)
	result.Error = fmt.Errorf("所有 STUN 服务器均不可达")

	log.Warn("IPv4 探测失败: 无可用服务器")
	return result, result.Error
}

// ProbeMultiple 对多个服务器进行探测（用于 NAT 类型检测）
func (p *IPv4Prober) ProbeMultiple(ctx context.Context, count int) ([]*netreportif.IPv4ProbeData, error) {
	var results []*netreportif.IPv4ProbeData

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
			log.Debug("IPv4 多服务器探测失败",
				"server", server,
				"err", err)
			continue
		}

		results = append(results, probeResult)
	}

	return results, nil
}

// probeServer 探测单个 STUN 服务器
func (p *IPv4Prober) probeServer(ctx context.Context, server string) (*netreportif.IPv4ProbeData, error) {
	// 解析服务器地址
	addr, err := net.ResolveUDPAddr("udp4", server)
	if err != nil {
		return nil, fmt.Errorf("解析 STUN 服务器地址失败: %w", err)
	}

	// 创建本地 UDP 连接
	conn, err := net.ListenUDP("udp4", nil)
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

	// 验证是 IPv4
	if ip.To4() == nil {
		return nil, fmt.Errorf("收到非 IPv4 地址: %s", ip.String())
	}

	return &netreportif.IPv4ProbeData{
		GlobalIP:   ip,
		GlobalPort: port,
		Server:     server,
	}, nil
}

// buildSTUNBindingRequest 构建 STUN Binding Request
func buildSTUNBindingRequest() []byte {
	// STUN Message:
	// - 2 bytes: Message Type (0x0001 = Binding Request)
	// - 2 bytes: Message Length (0x0000)
	// - 4 bytes: Magic Cookie (0x2112A442)
	// - 12 bytes: Transaction ID

	msg := make([]byte, 20)

	// Message Type: Binding Request
	msg[0] = 0x00
	msg[1] = 0x01

	// Message Length: 0
	msg[2] = 0x00
	msg[3] = 0x00

	// Magic Cookie: 0x2112A442
	msg[4] = 0x21
	msg[5] = 0x12
	msg[6] = 0xA4
	msg[7] = 0x42

	// Transaction ID: random 12 bytes
	// 忽略 rand.Read 的错误，因为 crypto/rand.Read 在主流平台上几乎不会失败
	// 即使失败，零值的 Transaction ID 仍然是有效的 STUN 请求
	_, _ = rand.Read(msg[8:20])

	return msg
}

// parseSTUNBindingResponse 解析 STUN Binding Response
func parseSTUNBindingResponse(data []byte) (net.IP, uint16, error) {
	if len(data) < 20 {
		return nil, 0, fmt.Errorf("响应太短")
	}

	// 检查 Message Type (0x0101 = Binding Response)
	if data[0] != 0x01 || data[1] != 0x01 {
		return nil, 0, fmt.Errorf("不是 Binding Response")
	}

	// 提取 Transaction ID（用于 IPv6 XOR 解码）
	transactionID := data[8:20]

	// 跳过 Message Type, Length, Magic Cookie, Transaction ID
	offset := 20

	// 解析属性
	for offset < len(data) {
		if offset+4 > len(data) {
			break
		}

		attrType := uint16(data[offset])<<8 | uint16(data[offset+1])
		attrLen := uint16(data[offset+2])<<8 | uint16(data[offset+3])
		offset += 4

		if offset+int(attrLen) > len(data) {
			break
		}

		// XOR-MAPPED-ADDRESS (0x0020) 或 MAPPED-ADDRESS (0x0001)
		if attrType == 0x0020 || attrType == 0x0001 {
			return parseAddressAttribute(data[offset:offset+int(attrLen)], attrType == 0x0020, transactionID)
		}

		// 填充到 4 字节边界
		attrLen = (attrLen + 3) & ^uint16(3)
		offset += int(attrLen)
	}

	return nil, 0, fmt.Errorf("未找到地址属性")
}

// parseAddressAttribute 解析地址属性
func parseAddressAttribute(data []byte, xor bool, transactionID []byte) (net.IP, uint16, error) {
	if len(data) < 8 {
		return nil, 0, fmt.Errorf("地址属性太短")
	}

	// Family
	family := data[1]

	// Port
	port := uint16(data[2])<<8 | uint16(data[3])
	if xor {
		port ^= 0x2112 // Magic Cookie 高 16 位
	}

	// Address
	var ip net.IP
	if family == 0x01 { // IPv4
		if len(data) < 8 {
			return nil, 0, fmt.Errorf("IPv4 地址属性太短")
		}
		ip = make(net.IP, 4)
		copy(ip, data[4:8])
		if xor {
			// XOR with Magic Cookie
			ip[0] ^= 0x21
			ip[1] ^= 0x12
			ip[2] ^= 0xA4
			ip[3] ^= 0x42
		}
	} else if family == 0x02 { // IPv6
		if len(data) < 20 {
			return nil, 0, fmt.Errorf("IPv6 地址属性太短")
		}
		ip = make(net.IP, 16)
		copy(ip, data[4:20])
		if xor {
			// XOR with Magic Cookie (前 4 字节)
			magicCookie := []byte{0x21, 0x12, 0xA4, 0x42}
			for i := 0; i < 4; i++ {
				ip[i] ^= magicCookie[i]
			}
			// XOR with Transaction ID (后 12 字节)
			if len(transactionID) >= 12 {
				for i := 0; i < 12; i++ {
					ip[4+i] ^= transactionID[i]
				}
			}
		}
	} else {
		return nil, 0, fmt.Errorf("未知地址族: %d", family)
	}

	return ip, port, nil
}

