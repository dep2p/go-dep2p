package stun

import (
	"context"
	"net"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/pion/stun"
)

// NATTypeResult NAT 类型检测结果
type NATTypeResult struct {
	// Type 检测到的 NAT 类型
	Type types.NATType

	// ExternalIP 外部 IP 地址
	ExternalIP net.IP

	// MappedPort 映射的端口
	MappedPort int

	// LocalAddr 本地地址
	LocalAddr *net.UDPAddr
}

// NATTypeDetector NAT 类型检测器
//
// 基于 RFC 3489 实现 NAT 类型检测算法。
// 检测流程：
//  1. Test 1: 发送到主服务器，获取映射地址
//  2. Test 2: 请求服务器从不同 IP 和端口响应（检测 Full Cone）
//  3. Test 3: 发送到备用 IP，检查映射是否变化（检测 Symmetric）
//  4. Test 4: 请求服务器从不同端口响应（区分 Restricted 和 Port Restricted）
type NATTypeDetector struct {
	// primaryServer 主 STUN 服务器地址
	primaryServer string

	// alternateServer 备用 STUN 服务器地址（不同 IP）
	alternateServer string

	// timeout 单次请求超时
	timeout time.Duration

	// retries 重试次数
	retries int

	// conn 复用的 UDP 连接
	conn *net.UDPConn

	// 用于测试的钩子函数
	testFunc func(testNum int) (interface{}, error)
}

// NewNATTypeDetector 创建 NAT 类型检测器
func NewNATTypeDetector(primaryServer, alternateServer string) *NATTypeDetector {
	return &NATTypeDetector{
		primaryServer:   primaryServer,
		alternateServer: alternateServer,
		timeout:         3 * time.Second,
		retries:         2,
	}
}

// DetectNATType 检测 NAT 类型
//
// 实现 RFC 3489 NAT 类型检测算法：
//   - 无响应 → UDP Blocked
//   - 映射地址 == 本地地址 → No NAT
//   - Test 2 有响应 → Full Cone NAT
//   - Test 3 映射变化 → Symmetric NAT
//   - Test 4 有响应 → Restricted Cone NAT
//   - Test 4 无响应 → Port Restricted Cone NAT
func (d *NATTypeDetector) DetectNATType(ctx context.Context) (*NATTypeResult, error) {
	// 创建 UDP 连接（复用同一端口进行所有测试）
	conn, err := net.ListenUDP("udp4", nil)
	if err != nil {
		return nil, &STUNError{Message: "create UDP socket", Cause: err}
	}
	defer conn.Close()
	d.conn = conn

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	// Test 1: 发送到主服务器
	mappedAddr1, err := d.test1(ctx)
	if err != nil {
		// 无响应，UDP 可能被阻塞
		return &NATTypeResult{
			Type:      types.NATTypeUnknown,
			LocalAddr: localAddr,
		}, nil
	}

	result := &NATTypeResult{
		ExternalIP: mappedAddr1.IP,
		MappedPort: mappedAddr1.Port,
		LocalAddr:  localAddr,
	}

	// 检查是否在公网（映射地址等于本地地址）
	if mappedAddr1.IP.Equal(getLocalIP()) && mappedAddr1.Port == localAddr.Port {
		result.Type = types.NATTypeNone
		return result, nil
	}

	// Test 2: 请求服务器从不同 IP 和端口响应
	if d.test2(ctx) {
		result.Type = types.NATTypeFullCone
		return result, nil
	}

	// Test 3: 发送到备用服务器，检查映射是否变化
	if d.alternateServer != "" {
		mappedAddr3, err := d.test3(ctx)
		if err == nil {
			// 如果映射地址不同，是 Symmetric NAT
			if !mappedAddr3.IP.Equal(mappedAddr1.IP) || mappedAddr3.Port != mappedAddr1.Port {
				result.Type = types.NATTypeSymmetric
				return result, nil
			}
		}
	}

	// Test 4: 请求服务器从不同端口响应（同 IP）
	if d.test4(ctx) {
		result.Type = types.NATTypeRestrictedCone
	} else {
		result.Type = types.NATTypePortRestricted
	}

	return result, nil
}

// test1 执行 Test 1：发送 Binding Request 到主服务器
//
// 返回映射地址（XOR-MAPPED-ADDRESS 或 MAPPED-ADDRESS）
func (d *NATTypeDetector) test1(ctx context.Context) (*net.UDPAddr, error) {
	if d.testFunc != nil {
		result, err := d.testFunc(1)
		if err != nil {
			return nil, err
		}
		return result.(*net.UDPAddr), nil
	}

	return d.sendBindingRequest(ctx, d.primaryServer, false, false)
}

// test2 执行 Test 2：请求服务器从不同 IP 和端口响应
//
// 发送带 CHANGE-REQUEST 属性的请求（change-ip=true, change-port=true）
// 如果收到响应，说明是 Full Cone NAT
func (d *NATTypeDetector) test2(ctx context.Context) bool {
	if d.testFunc != nil {
		result, _ := d.testFunc(2)
		if b, ok := result.(bool); ok {
			return b
		}
		return false
	}

	_, err := d.sendBindingRequest(ctx, d.primaryServer, true, true)
	return err == nil
}

// test3 执行 Test 3：发送到备用服务器
//
// 使用同一本地端口发送请求到备用服务器（不同 IP）
// 检查映射地址是否与 Test 1 相同
func (d *NATTypeDetector) test3(ctx context.Context) (*net.UDPAddr, error) {
	if d.testFunc != nil {
		result, err := d.testFunc(3)
		if err != nil {
			return nil, err
		}
		return result.(*net.UDPAddr), nil
	}

	if d.alternateServer == "" {
		return nil, &STUNError{Message: "no alternate server configured"}
	}

	return d.sendBindingRequest(ctx, d.alternateServer, false, false)
}

// test4 执行 Test 4：请求服务器从不同端口响应
//
// 发送带 CHANGE-REQUEST 属性的请求（change-ip=false, change-port=true）
// 如果收到响应，说明是 Restricted Cone NAT
// 如果未收到响应，说明是 Port Restricted Cone NAT
func (d *NATTypeDetector) test4(ctx context.Context) bool {
	if d.testFunc != nil {
		result, _ := d.testFunc(4)
		if b, ok := result.(bool); ok {
			return b
		}
		return false
	}

	_, err := d.sendBindingRequest(ctx, d.primaryServer, false, true)
	return err == nil
}

// sendBindingRequest 发送 STUN Binding Request
func (d *NATTypeDetector) sendBindingRequest(ctx context.Context, server string, changeIP, changePort bool) (*net.UDPAddr, error) {
	// 解析服务器地址
	serverAddr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		return nil, &STUNError{Message: "resolve server address", Cause: err}
	}

	// 构造 STUN 消息
	var msgOpts []stun.Setter
	msgOpts = append(msgOpts, stun.TransactionID, stun.BindingRequest)

	// 添加 CHANGE-REQUEST 属性（如果需要）
	if changeIP || changePort {
		changeReq := buildChangeRequest(changeIP, changePort)
		msgOpts = append(msgOpts, changeReq)
	}

	msg, err := stun.Build(msgOpts...)
	if err != nil {
		return nil, &STUNError{Message: "build request", Cause: err}
	}

	// 设置超时
	deadline := time.Now().Add(d.timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	d.conn.SetDeadline(deadline)

	// 发送请求（支持重试）
	var lastErr error
	for retry := 0; retry <= d.retries; retry++ {
		// 发送
		if _, err := d.conn.WriteToUDP(msg.Raw, serverAddr); err != nil {
			lastErr = &STUNError{Message: "send request", Cause: err}
			continue
		}

		// 接收响应
		buf := make([]byte, 1500)
		n, _, err := d.conn.ReadFromUDP(buf)
		if err != nil {
			lastErr = &STUNError{Message: "read response", Cause: err}
			continue
		}

		// 解析响应
		res := new(stun.Message)
		res.Raw = buf[:n]
		if err := res.Decode(); err != nil {
			lastErr = &STUNError{Message: "decode response", Cause: err}
			continue
		}

		// 提取映射地址
		addr, err := extractMappedAddress(res)
		if err != nil {
			lastErr = err
			continue
		}

		return addr, nil
	}

	return nil, lastErr
}

// extractMappedAddress 从 STUN 响应中提取映射地址
func extractMappedAddress(msg *stun.Message) (*net.UDPAddr, error) {
	// 优先使用 XOR-MAPPED-ADDRESS
	var xorAddr stun.XORMappedAddress
	if err := xorAddr.GetFrom(msg); err == nil {
		return &net.UDPAddr{
			IP:   xorAddr.IP,
			Port: xorAddr.Port,
		}, nil
	}

	// 回退到 MAPPED-ADDRESS
	var mappedAddr stun.MappedAddress
	if err := mappedAddr.GetFrom(msg); err == nil {
		return &net.UDPAddr{
			IP:   mappedAddr.IP,
			Port: mappedAddr.Port,
		}, nil
	}

	return nil, &STUNError{Message: "no mapped address in response"}
}

// ChangeRequest STUN CHANGE-REQUEST 属性
type ChangeRequest struct {
	ChangeIP   bool
	ChangePort bool
}

// buildChangeRequest 构造 CHANGE-REQUEST 属性
func buildChangeRequest(changeIP, changePort bool) stun.Setter {
	return &ChangeRequest{
		ChangeIP:   changeIP,
		ChangePort: changePort,
	}
}

// AddTo 实现 stun.Setter 接口
func (c *ChangeRequest) AddTo(m *stun.Message) error {
	var flags byte
	if c.ChangeIP {
		flags |= 0x04
	}
	if c.ChangePort {
		flags |= 0x02
	}
	// CHANGE-REQUEST 属性类型 = 0x0003
	// 值为 4 字节，最后一个字节包含标志
	value := []byte{0, 0, 0, flags}
	m.Add(stun.AttrType(0x0003), value)
	return nil
}

// getLocalIP 获取本机的外部 IP（用于判断是否在公网）
func getLocalIP() net.IP {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP
}

// SetTestFunc 设置测试钩子函数（用于单元测试）
func (d *NATTypeDetector) SetTestFunc(f func(testNum int) (interface{}, error)) {
	d.testFunc = f
}

// SetTimeout 设置超时时间
func (d *NATTypeDetector) SetTimeout(timeout time.Duration) {
	d.timeout = timeout
}

// SetRetries 设置重试次数
func (d *NATTypeDetector) SetRetries(retries int) {
	d.retries = retries
}
