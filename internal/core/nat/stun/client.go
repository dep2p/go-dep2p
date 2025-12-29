// Package stun 提供 STUN 协议客户端实现
//
// STUN (Session Traversal Utilities for NAT) 用于：
// - 发现节点的外部 IP 和端口
// - 检测 NAT 类型 (RFC 5780)
package stun

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	natif "github.com/dep2p/go-dep2p/pkg/interfaces/nat"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 包级别日志实例
var log = logger.Logger("nat.stun")

// ============================================================================
//                              常量定义
// ============================================================================

const (
	// STUN 消息类型
	bindingRequest       uint16 = 0x0001
	bindingResponse      uint16 = 0x0101
	bindingErrorResponse uint16 = 0x0111

	// STUN 属性类型 (RFC 5389)
	attrMappedAddress    uint16 = 0x0001
	attrChangeRequest    uint16 = 0x0003 // RFC 5780
	attrSourceAddress    uint16 = 0x0004 // Deprecated, use RESPONSE-ORIGIN
	attrChangedAddress   uint16 = 0x0005 // Deprecated, use OTHER-ADDRESS
	attrXORMappedAddress uint16 = 0x0020
	attrResponseOrigin   uint16 = 0x802b // RFC 5780
	attrOtherAddress     uint16 = 0x802c // RFC 5780
	attrSoftware         uint16 = 0x8022
	attrFingerprint      uint16 = 0x8028

	// CHANGE-REQUEST 标志
	changeIPFlag   uint32 = 0x04
	changePortFlag uint32 = 0x02

	// Magic Cookie (RFC 5389)
	magicCookie uint32 = 0x2112A442

	// 默认超时
	defaultTimeout = 5 * time.Second

	// Transaction ID 长度
	transactionIDLen = 12
)

// ============================================================================
//                              错误定义
// ============================================================================

// STUN 相关错误
var (
	// ErrNoResponse STUN 服务器无响应
	ErrNoResponse = errors.New("no response from STUN server")
	ErrInvalidResponse  = errors.New("invalid STUN response")
	ErrAllServersFailed = errors.New("all STUN servers failed")
	ErrNoOtherAddress   = errors.New("server does not support CHANGE-REQUEST (no OTHER-ADDRESS)")
)

// ============================================================================
//                              Client 结构
// ============================================================================

// Client STUN 客户端实现
type Client struct {
	servers   []string
	timeout   time.Duration
	timeoutMu sync.RWMutex // 保护 timeout 的并发访问

	// 缓存
	cachedAddr    endpoint.Address
	cachedNATType types.NATType
	cachedTime    time.Time
	cacheDuration time.Duration
	cacheMu       sync.RWMutex

	// 关闭状态
	closeOnce sync.Once
}

// 确保实现接口
var _ natif.STUNClient = (*Client)(nil)

// NewClient 创建 STUN 客户端
func NewClient(servers []string) *Client {
	if len(servers) == 0 {
		servers = DefaultServers()
	}

	return &Client{
		servers:       normalizeServers(servers),
		timeout:       defaultTimeout,
		cacheDuration: 5 * time.Minute,
		cachedNATType: types.NATTypeUnknown,
	}
}

// normalizeServers 将多种常见写法归一化为 net.Dial/net.ResolveUDPAddr 可识别的 "host:port" 形式。
//
// 兼容配置里常见的：
// - "stun:stun.l.google.com:19302"
// - "stun://stun.l.google.com:19302"
// - "stuns:stun.l.google.com:5349"（当前实现仍按 host:port 处理；是否使用 TLS 由上层决定）
func normalizeServers(in []string) []string {
	out := make([]string, 0, len(in))
	for _, raw := range in {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}

		// 去掉 scheme（兼容 "stun:" / "stuns:" / "stun://" / "stuns://"）
		if i := strings.Index(s, "://"); i >= 0 {
			s = s[i+3:]
		} else {
			s = strings.TrimPrefix(s, "stun:")
			s = strings.TrimPrefix(s, "stuns:")
		}

		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, s)
	}
	return out
}

// DefaultServers 返回默认 STUN 服务器列表
func DefaultServers() []string {
	return []string{
		"stun.l.google.com:19302",
		"stun1.l.google.com:19302",
		"stun2.l.google.com:19302",
		"stun.cloudflare.com:3478",
		"stun.stunprotocol.org:3478",
	}
}

// ============================================================================
//                              公共方法
// ============================================================================

// GetMappedAddress 获取映射地址
func (c *Client) GetMappedAddress(ctx context.Context) (endpoint.Address, error) {
	// 检查缓存
	c.cacheMu.RLock()
	if c.cachedAddr != nil && time.Since(c.cachedTime) < c.cacheDuration {
		addr := c.cachedAddr
		c.cacheMu.RUnlock()
		return addr, nil
	}
	c.cacheMu.RUnlock()

	// 尝试每个服务器
	var lastErr error
	for _, server := range c.servers {
		result, err := c.queryServerFull(ctx, server, false, false)
		if err != nil {
			log.Debug("STUN 服务器查询失败",
				"server", server,
				"err", err)
			lastErr = err
			continue
		}

		// 更新缓存
		c.cacheMu.Lock()
		c.cachedAddr = result.MappedAddress
		c.cachedTime = time.Now()
		c.cacheMu.Unlock()

		log.Info("获取到外部地址",
			"server", server,
			"addr", result.MappedAddress.String())

		return result.MappedAddress, nil
	}

	return nil, fmt.Errorf("%w: %v", ErrAllServersFailed, lastErr)
}

// GetNATType 检测 NAT 类型 (RFC 5780)
func (c *Client) GetNATType(ctx context.Context) (types.NATType, error) {
	// 检查缓存
	c.cacheMu.RLock()
	if c.cachedNATType != types.NATTypeUnknown && time.Since(c.cachedTime) < c.cacheDuration {
		natType := c.cachedNATType
		c.cacheMu.RUnlock()
		return natType, nil
	}
	c.cacheMu.RUnlock()

	natType, err := c.detectNATType(ctx)
	if err != nil {
		return types.NATTypeUnknown, err
	}

	// 更新缓存
	c.cacheMu.Lock()
	c.cachedNATType = natType
	c.cachedTime = time.Now()
	c.cacheMu.Unlock()

	log.Info("检测到 NAT 类型", "type", natType.String())
	return natType, nil
}

// Close 关闭客户端
func (c *Client) Close() error {
	c.closeOnce.Do(func() {
		c.cacheMu.Lock()
		c.cachedAddr = nil
		c.cachedNATType = types.NATTypeUnknown
		c.cacheMu.Unlock()
	})
	return nil
}

// SetTimeout 设置超时时间
func (c *Client) SetTimeout(timeout time.Duration) {
	c.timeoutMu.Lock()
	c.timeout = timeout
	c.timeoutMu.Unlock()
}

// getTimeout 获取超时时间（线程安全）
func (c *Client) getTimeout() time.Duration {
	c.timeoutMu.RLock()
	defer c.timeoutMu.RUnlock()
	return c.timeout
}

// SetCacheDuration 设置缓存时间
func (c *Client) SetCacheDuration(duration time.Duration) {
	c.cacheMu.Lock()
	c.cacheDuration = duration
	c.cacheMu.Unlock()
}

// ============================================================================
//                              NAT 类型检测 (RFC 5780)
// ============================================================================

// detectNATType 实现完整的 NAT 类型检测算法
func (c *Client) detectNATType(ctx context.Context) (types.NATType, error) {
	if len(c.servers) == 0 {
		return types.NATTypeUnknown, errors.New("no STUN servers configured")
	}

	server := c.servers[0]

	// Step 1: 基本测试 - 获取映射地址
	log.Debug("NAT 检测 Step 1: 基本测试")
	result1, err := c.queryServerFull(ctx, server, false, false)
	if err != nil {
		return types.NATTypeUnknown, fmt.Errorf("step 1 failed: %w", err)
	}

	// 获取本地地址
	localAddr, err := c.getLocalAddress()
	if err != nil {
		return types.NATTypeUnknown, fmt.Errorf("get local address failed: %w", err)
	}

	// 如果映射地址等于本地地址，则没有 NAT
	if c.addressEqual(localAddr, result1.MappedAddress) {
		log.Debug("检测到无 NAT")
		return types.NATTypeNone, nil
	}

	// Step 2: 测试 Full Cone - 请求服务器从不同 IP 和端口回复
	log.Debug("NAT 检测 Step 2: Full Cone 测试")
	if result1.OtherAddress != nil {
		result2, err := c.queryServerFull(ctx, server, true, true) // change IP and port
		if err == nil && result2.MappedAddress != nil {
			// 收到了从不同 IP/端口发来的响应，说明是 Full Cone
			log.Debug("检测到 Full Cone NAT")
			return types.NATTypeFull, nil
		}
	}

	// Step 3: 测试 Symmetric NAT - 向不同服务器发送请求
	log.Debug("NAT 检测 Step 3: Symmetric NAT 测试")
	if len(c.servers) >= 2 {
		result3, err := c.queryServerFull(ctx, c.servers[1], false, false)
		if err == nil && result3.MappedAddress != nil {
			// 如果两个服务器返回的端口不同，则是 Symmetric NAT
			if !c.samePort(result1.MappedAddress, result3.MappedAddress) {
				log.Debug("检测到 Symmetric NAT (不同端口映射)")
				return types.NATTypeSymmetric, nil
			}
		}
	} else if result1.OtherAddress != nil {
		// 使用 OTHER-ADDRESS 作为第二个服务器
		otherServer := result1.OtherAddress.String()
		result3, err := c.queryServerFull(ctx, otherServer, false, false)
		if err == nil && result3.MappedAddress != nil {
			if !c.samePort(result1.MappedAddress, result3.MappedAddress) {
				log.Debug("检测到 Symmetric NAT (不同端口映射)")
				return types.NATTypeSymmetric, nil
			}
		}
	}

	// Step 4: 区分 Restricted Cone 和 Port Restricted Cone
	log.Debug("NAT 检测 Step 4: Restricted/Port Restricted 测试")
	if result1.OtherAddress != nil {
		// 请求服务器从相同 IP 但不同端口回复
		result4, err := c.queryServerFull(ctx, server, false, true) // change port only
		if err == nil && result4.MappedAddress != nil {
			// 收到了从不同端口发来的响应，说明是 Restricted Cone
			log.Debug("检测到 Restricted Cone NAT")
			return types.NATTypeRestricted, nil
		}
	}

	// 默认为 Port Restricted Cone
	log.Debug("检测到 Port Restricted Cone NAT (默认)")
	return types.NATTypePortRestricted, nil
}

// ============================================================================
//                              STUN 消息构建与解析
// ============================================================================

// stunResult STUN 查询结果
type stunResult struct {
	MappedAddress  endpoint.Address
	ResponseOrigin endpoint.Address
	OtherAddress   endpoint.Address
	TransactionID  []byte
}

// queryServerFull 查询 STUN 服务器（完整版，支持 CHANGE-REQUEST）
func (c *Client) queryServerFull(ctx context.Context, server string, changeIP, changePort bool) (*stunResult, error) {
	// 解析服务器地址
	serverAddr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		return nil, fmt.Errorf("解析服务器地址失败: %w", err)
	}

	// 创建 UDP 连接
	conn, err := net.DialUDP("udp", nil, serverAddr)
	if err != nil {
		return nil, fmt.Errorf("连接 STUN 服务器失败: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// 设置超时
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(c.getTimeout())
	}
	conn.SetDeadline(deadline)

	// 构建请求
	transactionID := make([]byte, transactionIDLen)
	if _, err := rand.Read(transactionID); err != nil {
		return nil, fmt.Errorf("生成 transaction ID 失败: %w", err)
	}

	request := c.buildBindingRequestFull(transactionID, changeIP, changePort)

	// 发送请求
	if _, err := conn.Write(request); err != nil {
		return nil, fmt.Errorf("发送请求失败: %w", err)
	}

	// 读取响应
	buf := make([]byte, 1024)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	// 解析响应
	return c.parseBindingResponseFull(buf[:n], transactionID)
}

// buildBindingRequestFull 构建 STUN Binding Request（支持 CHANGE-REQUEST）
func (c *Client) buildBindingRequestFull(transactionID []byte, changeIP, changePort bool) []byte {
	// 计算消息长度
	msgLen := 0
	if changeIP || changePort {
		msgLen = 8 // CHANGE-REQUEST 属性: 4 字节头 + 4 字节值
	}

	msg := make([]byte, 20+msgLen)

	// 消息类型
	binary.BigEndian.PutUint16(msg[0:2], bindingRequest)

	// 消息长度
	binary.BigEndian.PutUint16(msg[2:4], uint16(msgLen))

	// Magic Cookie
	binary.BigEndian.PutUint32(msg[4:8], magicCookie)

	// Transaction ID
	copy(msg[8:20], transactionID)

	// CHANGE-REQUEST 属性
	if changeIP || changePort {
		offset := 20
		binary.BigEndian.PutUint16(msg[offset:offset+2], attrChangeRequest)
		binary.BigEndian.PutUint16(msg[offset+2:offset+4], 4) // 值长度

		var flags uint32
		if changeIP {
			flags |= changeIPFlag
		}
		if changePort {
			flags |= changePortFlag
		}
		binary.BigEndian.PutUint32(msg[offset+4:offset+8], flags)
	}

	return msg
}

// parseBindingResponseFull 解析 STUN Binding Response（完整版）
func (c *Client) parseBindingResponseFull(data []byte, expectedTxID []byte) (*stunResult, error) {
	if len(data) < 20 {
		return nil, ErrInvalidResponse
	}

	// 检查消息类型
	msgType := binary.BigEndian.Uint16(data[0:2])
	if msgType != bindingResponse {
		if msgType == bindingErrorResponse {
			return nil, fmt.Errorf("binding error response")
		}
		return nil, fmt.Errorf("unexpected message type: 0x%04x", msgType)
	}

	// 验证 Magic Cookie
	cookie := binary.BigEndian.Uint32(data[4:8])
	if cookie != magicCookie {
		return nil, fmt.Errorf("invalid magic cookie: 0x%08x", cookie)
	}

	// 验证 Transaction ID
	txID := data[8:20]
	if len(expectedTxID) > 0 {
		for i := 0; i < transactionIDLen; i++ {
			if txID[i] != expectedTxID[i] {
				return nil, fmt.Errorf("transaction ID mismatch")
			}
		}
	}

	// 获取消息长度
	msgLen := binary.BigEndian.Uint16(data[2:4])
	if len(data) < 20+int(msgLen) {
		return nil, ErrInvalidResponse
	}

	result := &stunResult{
		TransactionID: txID,
	}

	// 解析属性
	offset := 20
	for offset < 20+int(msgLen) {
		if offset+4 > len(data) {
			break
		}

		attrType := binary.BigEndian.Uint16(data[offset : offset+2])
		attrLen := binary.BigEndian.Uint16(data[offset+2 : offset+4])
		offset += 4

		if offset+int(attrLen) > len(data) {
			break
		}

		attrValue := data[offset : offset+int(attrLen)]

		switch attrType {
		case attrXORMappedAddress:
			addr, err := c.parseXORMappedAddressFull(attrValue, data[4:8], txID)
			if err == nil {
				result.MappedAddress = addr
			}
		case attrMappedAddress:
			addr, err := c.parseMappedAddress(attrValue)
			if err == nil && result.MappedAddress == nil {
				result.MappedAddress = addr
			}
		case attrResponseOrigin, attrSourceAddress:
			addr, err := c.parseMappedAddress(attrValue)
			if err == nil {
				result.ResponseOrigin = addr
			}
		case attrOtherAddress, attrChangedAddress:
			addr, err := c.parseMappedAddress(attrValue)
			if err == nil {
				result.OtherAddress = addr
			}
		}

		// 对齐到 4 字节边界
		offset += int(attrLen)
		if attrLen%4 != 0 {
			offset += int(4 - attrLen%4)
		}
	}

	if result.MappedAddress == nil {
		return nil, fmt.Errorf("no mapped address in response")
	}

	return result, nil
}

// parseXORMappedAddressFull 解析 XOR-MAPPED-ADDRESS（支持 IPv6）
func (c *Client) parseXORMappedAddressFull(value, magicCookieBytes, transactionID []byte) (endpoint.Address, error) {
	if len(value) < 4 {
		return nil, ErrInvalidResponse
	}

	// 地址族
	family := value[1]

	// XOR 端口
	xorPort := binary.BigEndian.Uint16(value[2:4])
	port := xorPort ^ uint16(magicCookie>>16)

	var ip net.IP
	if family == 0x01 { // IPv4
		if len(value) < 8 {
			return nil, ErrInvalidResponse
		}
		ip = make(net.IP, 4)
		for i := 0; i < 4; i++ {
			ip[i] = value[4+i] ^ magicCookieBytes[i]
		}
	} else if family == 0x02 { // IPv6
		if len(value) < 20 {
			return nil, ErrInvalidResponse
		}
		// IPv6 XOR 使用 Magic Cookie + Transaction ID
		xorBytes := make([]byte, 16)
		copy(xorBytes[0:4], magicCookieBytes)
		copy(xorBytes[4:16], transactionID)

		ip = make(net.IP, 16)
		for i := 0; i < 16; i++ {
			ip[i] = value[4+i] ^ xorBytes[i]
		}
	} else {
		return nil, fmt.Errorf("unknown address family: %d", family)
	}

	return &stunAddress{
		ip:   ip,
		port: int(port),
	}, nil
}

// parseMappedAddress 解析 MAPPED-ADDRESS 属性
func (c *Client) parseMappedAddress(value []byte) (endpoint.Address, error) {
	if len(value) < 4 {
		return nil, ErrInvalidResponse
	}

	// 地址族
	family := value[1]

	// 端口
	port := binary.BigEndian.Uint16(value[2:4])

	var ip net.IP
	if family == 0x01 { // IPv4
		if len(value) < 8 {
			return nil, ErrInvalidResponse
		}
		// 复制 IP 字节，避免共享底层 slice
		ip = make(net.IP, 4)
		copy(ip, value[4:8])
	} else if family == 0x02 { // IPv6
		if len(value) < 20 {
			return nil, ErrInvalidResponse
		}
		// 复制 IP 字节，避免共享底层 slice
		ip = make(net.IP, 16)
		copy(ip, value[4:20])
	} else {
		return nil, fmt.Errorf("unknown address family: %d", family)
	}

	return &stunAddress{
		ip:   ip,
		port: int(port),
	}, nil
}

// ============================================================================
//                              辅助方法
// ============================================================================

// getLocalAddress 获取本地地址
func (c *Client) getLocalAddress() (endpoint.Address, error) {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()

	localAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return nil, errors.New("unexpected local address type")
	}
	return &stunAddress{
		ip:   localAddr.IP,
		port: localAddr.Port,
	}, nil
}

// addressEqual 比较两个地址是否相等
func (c *Client) addressEqual(a, b endpoint.Address) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.String() == b.String()
}

// samePort 检查两个地址是否有相同的端口
func (c *Client) samePort(addr1, addr2 endpoint.Address) bool {
	a1, ok1 := addr1.(*stunAddress)
	a2, ok2 := addr2.(*stunAddress)
	if !ok1 || !ok2 {
		return addr1.String() == addr2.String()
	}
	return a1.port == a2.port
}

// ============================================================================
//                              stunAddress 实现
// ============================================================================

// stunAddress STUN 地址实现
type stunAddress struct {
	ip   net.IP
	port int
}

func (a *stunAddress) Network() string {
	if a.ip.To4() != nil {
		return "ip4"
	}
	return "ip6"
}

func (a *stunAddress) String() string {
	if a.ip.To4() != nil {
		return fmt.Sprintf("%s:%d", a.ip.String(), a.port)
	}
	return fmt.Sprintf("[%s]:%d", a.ip.String(), a.port)
}

func (a *stunAddress) Bytes() []byte {
	return []byte(a.String())
}

func (a *stunAddress) Equal(other endpoint.Address) bool {
	if other == nil {
		return false
	}
	return a.String() == other.String()
}

func (a *stunAddress) IsPublic() bool {
	// 0.0.0.0 和 :: 是未指定地址，不是公网地址
	return !a.IsPrivate() && !a.IsLoopback() && !a.ip.IsUnspecified()
}

func (a *stunAddress) IsPrivate() bool {
	return a.ip.IsPrivate()
}

func (a *stunAddress) IsLoopback() bool {
	return a.ip.IsLoopback()
}

// IP 返回 IP 地址
func (a *stunAddress) IP() net.IP {
	return a.ip
}

// Port 返回端口
func (a *stunAddress) Port() int {
	return a.port
}

// ToUDPAddr 转换为 net.UDPAddr
func (a *stunAddress) ToUDPAddr() (*net.UDPAddr, error) {
	return &net.UDPAddr{
		IP:   a.ip,
		Port: a.port,
	}, nil
}

// Multiaddr 返回 multiaddr 格式
func (a *stunAddress) Multiaddr() string {
	if a.ip.To4() != nil {
		return fmt.Sprintf("/ip4/%s/udp/%d", a.ip.String(), a.port)
	}
	return fmt.Sprintf("/ip6/%s/udp/%d", a.ip.String(), a.port)
}
