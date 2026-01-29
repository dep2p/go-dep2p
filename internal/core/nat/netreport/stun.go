// Package netreport 提供网络诊断功能
//
// STUN 协议实现
package netreport

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"sync"
	"time"
)

// ============================================================================
//                              STUN 客户端
// ============================================================================

// STUNClient STUN 客户端
type STUNClient struct {
	servers []string
	timeout time.Duration
}

// NewSTUNClient 创建 STUN 客户端
func NewSTUNClient(servers []string, timeout time.Duration) *STUNClient {
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	return &STUNClient{
		servers: servers,
		timeout: timeout,
	}
}

// STUNResult STUN 探测结果
type STUNResult struct {
	Server   string
	GlobalIP net.IP
	Port     uint16
	Latency  time.Duration
}

// Probe 探测单个服务器
func (c *STUNClient) Probe(ctx context.Context, server string) (*STUNResult, error) {
	addr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		return nil, fmt.Errorf("解析 STUN 服务器地址失败: %w", err)
	}

	// 确定网络类型
	network := "udp4"
	if addr.IP.To4() == nil {
		network = "udp6"
	}

	// 创建 UDP 连接
	conn, err := net.ListenPacket(network, "")
	if err != nil {
		return nil, fmt.Errorf("创建 UDP 连接失败: %w", err)
	}
	defer conn.Close()

	// 设置超时
	deadline := time.Now().Add(c.timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn.SetDeadline(deadline)

	start := time.Now()

	// 发送 STUN Binding Request
	request := buildSTUNBindingRequest()
	_, err = conn.WriteTo(request, addr)
	if err != nil {
		return nil, fmt.Errorf("发送 STUN 请求失败: %w", err)
	}

	// 接收响应
	buf := make([]byte, 1024)
	n, _, err := conn.ReadFrom(buf)
	if err != nil {
		return nil, fmt.Errorf("接收 STUN 响应失败: %w", err)
	}

	latency := time.Since(start)

	// 解析响应
	ip, port, err := parseSTUNBindingResponse(buf[:n])
	if err != nil {
		return nil, fmt.Errorf("解析 STUN 响应失败: %w", err)
	}

	return &STUNResult{
		Server:   server,
		GlobalIP: ip,
		Port:     port,
		Latency:  latency,
	}, nil
}

// ProbeMultiple 探测多个服务器
func (c *STUNClient) ProbeMultiple(ctx context.Context, count int) ([]*STUNResult, error) {
	var results []*STUNResult

	if count <= 0 || len(c.servers) == 0 {
		return results, nil
	}

	if count > len(c.servers) {
		count = len(c.servers)
	}

	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		sem    = make(chan struct{}, count)
		ctxErr error
	)

loop:
	for i := 0; i < count; i++ {
		select {
		case <-ctx.Done():
			ctxErr = ctx.Err()
			break loop
		default:
		}

		server := c.servers[i]
		wg.Add(1)
		sem <- struct{}{}

		go func(srv string) {
			defer wg.Done()
			defer func() { <-sem }()

			result, err := c.Probe(ctx, srv)
			if err != nil {
				logger.Debug("STUN 探测失败", "server", srv, "err", err)
				return
			}

			mu.Lock()
			results = append(results, result)
			mu.Unlock()
		}(server)
	}

	wg.Wait()

	if ctxErr != nil {
		return results, ctxErr
	}

	return results, nil
}

// ============================================================================
//                              STUN 协议
// ============================================================================

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
	if _, err := rand.Read(msg[8:20]); err != nil {
		// 使用时间戳作为备用随机源
		copy(msg[8:20], []byte(fmt.Sprintf("%012d", time.Now().UnixNano()%1e12)))
	}

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

	// 提取 Transaction ID（用于 XOR 解码）
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
	switch family {
	case 0x01: // IPv4
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
	case 0x02: // IPv6
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
	default:
		return nil, 0, fmt.Errorf("未知地址族: %d", family)
	}

	return ip, port, nil
}
