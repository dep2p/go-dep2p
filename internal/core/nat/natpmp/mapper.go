// Package natpmp 提供 NAT-PMP 端口映射实现
//
// NAT-PMP (NAT Port Mapping Protocol) 是 Apple 提出的轻量级协议：
// - 基于 UDP
// - 比 UPnP 简单
// - Apple 设备原生支持
package natpmp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	natpmp "github.com/jackpal/go-nat-pmp"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	natif "github.com/dep2p/go-dep2p/pkg/interfaces/nat"
)

// 包级别日志实例
var log = logger.Logger("nat.natpmp")

// ============================================================================
//                              错误定义
// ============================================================================

// NAT-PMP 相关错误
var (
	// ErrNoGateway 未找到 NAT-PMP 网关
	ErrNoGateway = errors.New("no NAT-PMP gateway found")
	ErrMappingFailed      = errors.New("port mapping failed")
	ErrNATPMPNotSupported = errors.New("NAT-PMP not supported")
	ErrGatewayNotReady    = errors.New("NAT-PMP gateway not ready")
)

// ============================================================================
//                              Mapper 结构
// ============================================================================

// Mapper NAT-PMP 端口映射器实现
type Mapper struct {
	// NAT-PMP 客户端
	client   *natpmp.Client
	clientMu sync.RWMutex
	gateway  net.IP

	// 网关信息
	externalIP string
	localIP    string
	discovered bool

	// 活跃映射
	mappings   map[string]*natif.Mapping // key: "protocol:internalPort"
	mappingsMu sync.RWMutex

	// 配置
	timeout       time.Duration
	refreshPeriod time.Duration
	configMu      sync.RWMutex // 保护 timeout 和 refreshPeriod 的并发访问

	// 状态
	closed    bool
	closeOnce sync.Once
}

// 确保实现接口
var _ natif.PortMapper = (*Mapper)(nil)

// NewMapper 创建 NAT-PMP 映射器
func NewMapper() *Mapper {
	localIP := getLocalIP()

	return &Mapper{
		mappings:      make(map[string]*natif.Mapping),
		timeout:       5 * time.Second,
		refreshPeriod: 20 * time.Minute,
		localIP:       localIP,
	}
}

// getTimeout 获取超时时间（线程安全）
func (m *Mapper) getTimeout() time.Duration {
	m.configMu.RLock()
	defer m.configMu.RUnlock()
	return m.timeout
}

// SetTimeout 设置超时时间
func (m *Mapper) SetTimeout(timeout time.Duration) {
	m.configMu.Lock()
	m.timeout = timeout
	m.configMu.Unlock()
}

// getRefreshPeriod 获取刷新周期（线程安全）
func (m *Mapper) getRefreshPeriod() time.Duration {
	m.configMu.RLock()
	defer m.configMu.RUnlock()
	return m.refreshPeriod
}

// SetRefreshPeriod 设置刷新周期
func (m *Mapper) SetRefreshPeriod(period time.Duration) {
	m.configMu.Lock()
	m.refreshPeriod = period
	m.configMu.Unlock()
}

// ============================================================================
//                              PortMapper 接口实现
// ============================================================================

// Name 返回映射器名称
func (m *Mapper) Name() string {
	return "nat-pmp"
}

// Available 检查映射器是否可用
func (m *Mapper) Available() bool {
	m.clientMu.RLock()
	if m.discovered && m.client != nil {
		m.clientMu.RUnlock()
		return true
	}
	m.clientMu.RUnlock()

	// 尝试发现网关
	ctx, cancel := context.WithTimeout(context.Background(), m.getTimeout())
	defer cancel()

	return m.discoverGateway(ctx) == nil
}

// GetExternalAddress 获取外部地址
func (m *Mapper) GetExternalAddress() (endpoint.Address, error) {
	m.clientMu.RLock()
	if m.externalIP != "" {
		ip := m.externalIP
		m.clientMu.RUnlock()
		return newIPAddr(net.ParseIP(ip), 0), nil
	}
	m.clientMu.RUnlock()

	// 尝试发现并获取外部 IP
	ctx, cancel := context.WithTimeout(context.Background(), m.getTimeout())
	defer cancel()

	if err := m.discoverGateway(ctx); err != nil {
		return nil, err
	}

	m.clientMu.RLock()
	defer m.clientMu.RUnlock()

	if m.externalIP == "" {
		return nil, ErrNoGateway
	}

	return newIPAddr(net.ParseIP(m.externalIP), 0), nil
}

// AddMapping 添加端口映射
func (m *Mapper) AddMapping(protocol string, internalPort int, description string, duration time.Duration) (int, error) {
	m.clientMu.RLock()
	client := m.client
	externalIP := m.externalIP // 在持锁时复制 externalIP
	m.clientMu.RUnlock()

	if client == nil {
		// 尝试发现
		ctx, cancel := context.WithTimeout(context.Background(), m.getTimeout())
		defer cancel()
		if err := m.discoverGateway(ctx); err != nil {
			return 0, fmt.Errorf("%w: %v", ErrNATPMPNotSupported, err)
		}
		m.clientMu.RLock()
		client = m.client
		externalIP = m.externalIP // 再次在持锁时复制
		m.clientMu.RUnlock()
	}

	if client == nil {
		return 0, ErrGatewayNotReady
	}

	// 计算租约时间（秒）
	lifetime := int(duration.Seconds())
	if lifetime == 0 {
		lifetime = 3600 // 默认 1 小时
	}

	// 使用相同的内外端口
	externalPort := internalPort

	// 调用 NAT-PMP AddPortMapping
	var resp *natpmp.AddPortMappingResult
	var err error

	if protocol == "tcp" || protocol == "TCP" {
		resp, err = client.AddPortMapping("tcp", internalPort, externalPort, lifetime)
	} else {
		resp, err = client.AddPortMapping("udp", internalPort, externalPort, lifetime)
	}

	if err != nil {
		log.Warn("NAT-PMP 端口映射失败",
			"protocol", protocol,
			"port", internalPort,
			"err", err)
		return 0, fmt.Errorf("%w: %v", ErrMappingFailed, err)
	}

	// 保存映射记录
	key := fmt.Sprintf("%s:%d", protocol, internalPort)
	m.mappingsMu.Lock()
	m.mappings[key] = &natif.Mapping{
		Protocol:     protocol,
		InternalPort: internalPort,
		ExternalPort: int(resp.MappedExternalPort),
		ExternalAddr: externalIP, // 使用本地复制的值
		Description:  description,
		Expiry:       time.Now().Add(time.Duration(resp.PortMappingLifetimeInSeconds) * time.Second),
	}
	m.mappingsMu.Unlock()

	log.Info("NAT-PMP 端口映射成功",
		"protocol", protocol,
		"internalPort", internalPort,
		"externalPort", int(resp.MappedExternalPort),
		"lifetime", int(resp.PortMappingLifetimeInSeconds))

	return int(resp.MappedExternalPort), nil
}

// DeleteMapping 删除端口映射
func (m *Mapper) DeleteMapping(protocol string, externalPort int) error {
	m.clientMu.RLock()
	client := m.client
	m.clientMu.RUnlock()

	if client == nil {
		return nil // 没有网关，无需删除
	}

	// 删除映射（通过设置 lifetime 为 0）
	var err error
	if protocol == "tcp" || protocol == "TCP" {
		_, err = client.AddPortMapping("tcp", externalPort, externalPort, 0)
	} else {
		_, err = client.AddPortMapping("udp", externalPort, externalPort, 0)
	}

	if err != nil {
		log.Debug("删除 NAT-PMP 端口映射失败",
			"protocol", protocol,
			"externalPort", externalPort,
			"err", err)
	}

	// 移除本地记录
	m.mappingsMu.Lock()
	for key, mapping := range m.mappings {
		if mapping.Protocol == protocol && mapping.ExternalPort == externalPort {
			delete(m.mappings, key)
			break
		}
	}
	m.mappingsMu.Unlock()

	log.Info("NAT-PMP 端口映射已删除",
		"protocol", protocol,
		"externalPort", externalPort)

	return nil
}

// GetMapping 获取端口映射
func (m *Mapper) GetMapping(protocol string, externalPort int) (*natif.Mapping, error) {
	m.mappingsMu.RLock()
	defer m.mappingsMu.RUnlock()

	for _, mapping := range m.mappings {
		if mapping.Protocol == protocol && mapping.ExternalPort == externalPort {
			return mapping, nil
		}
	}

	return nil, fmt.Errorf("mapping not found")
}

// ============================================================================
//                              其他方法
// ============================================================================

// RefreshMappings 刷新所有映射
func (m *Mapper) RefreshMappings(ctx context.Context) error {
	m.mappingsMu.RLock()
	mappings := make([]*natif.Mapping, 0, len(m.mappings))
	for _, mp := range m.mappings {
		mappings = append(mappings, mp)
	}
	m.mappingsMu.RUnlock()

	var errs []error
	for _, mapping := range mappings {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		ttl := mapping.TTL()
		if ttl <= 0 {
			ttl = 30 * time.Minute
		}

		_, err := m.AddMapping(mapping.Protocol, mapping.InternalPort, mapping.Description, ttl)
		if err != nil {
			log.Warn("刷新 NAT-PMP 映射失败",
				"protocol", mapping.Protocol,
				"port", mapping.InternalPort,
				"err", err)
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("refresh failed for %d mappings", len(errs))
	}
	return nil
}

// Close 关闭映射器
func (m *Mapper) Close() error {
	m.closeOnce.Do(func() {
		m.closed = true

		log.Info("关闭 NAT-PMP 映射器")

		// 删除所有映射
		m.mappingsMu.Lock()
		mappings := make([]*natif.Mapping, 0, len(m.mappings))
		for _, mp := range m.mappings {
			mappings = append(mappings, mp)
		}
		m.mappings = make(map[string]*natif.Mapping)
		m.mappingsMu.Unlock()

		m.clientMu.RLock()
		client := m.client
		m.clientMu.RUnlock()

		if client != nil {
			for _, mapping := range mappings {
				// 删除映射（通过设置 lifetime 为 0）
				proto := "udp"
				if mapping.Protocol == "tcp" || mapping.Protocol == "TCP" {
					proto = "tcp"
				}
				_, _ = client.AddPortMapping(proto, mapping.ExternalPort, mapping.ExternalPort, 0)
				log.Debug("清理 NAT-PMP 端口映射",
					"protocol", mapping.Protocol,
					"port", mapping.ExternalPort)
			}
		}
	})

	return nil
}

// ============================================================================
//                              网关发现
// ============================================================================

// discoverGateway 发现 NAT-PMP 网关
//
// 注意：底层 go-nat-pmp 库的网络调用不支持 context 取消，
// 因此这里用 goroutine + select 包装实现超时控制。
func (m *Mapper) discoverGateway(ctx context.Context) error {
	m.clientMu.Lock()
	defer m.clientMu.Unlock()

	if m.discovered && m.client != nil {
		return nil
	}

	log.Debug("开始发现 NAT-PMP 网关...")

	// 获取默认网关
	gateway, err := getDefaultGateway()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNoGateway, err)
	}

	m.gateway = gateway

	// 创建 NAT-PMP 客户端
	client := natpmp.NewClient(gateway)
	m.client = client

	// 用 goroutine + select 包装阻塞调用，实现 ctx 超时控制
	type result struct {
		resp *natpmp.GetExternalAddressResult
		err  error
	}
	ch := make(chan result, 1)

	go func() {
		resp, err := client.GetExternalAddress()
		ch <- result{resp, err}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			log.Debug("NAT-PMP 获取外部地址失败", "err", r.err)
			m.client = nil
			return fmt.Errorf("%w: %v", ErrNATPMPNotSupported, r.err)
		}
		m.externalIP = net.IP(r.resp.ExternalIPAddress[:]).String()
		m.discovered = true
		log.Info("发现 NAT-PMP 网关",
			"gateway", gateway.String(),
			"externalIP", m.externalIP)
		return nil

	case <-ctx.Done():
		log.Debug("NAT-PMP 发现超时", "err", ctx.Err())
		m.client = nil
		return fmt.Errorf("%w: %v", ErrNATPMPNotSupported, ctx.Err())
	}
}

// ============================================================================
//                              辅助函数
// ============================================================================

// getDefaultGateway 获取默认网关
//
// 这个函数通过多种方式尝试获取真正的默认网关:
// 1. 首先尝试使用 gateway 库从路由表获取
// 2. 如果失败，尝试解析网络接口信息
// 3. 最后回退到基于子网的推测方法
func getDefaultGateway() (net.IP, error) {
	// 方法 1: 尝试从路由表推断（通过解析接口）
	gateway, err := discoverGatewayFromInterfaces()
	if err == nil && gateway != nil {
		return gateway, nil
	}

	// 方法 2: 使用出站连接获取本地 IP，然后推测网关
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()

	localAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return nil, errors.New("unexpected local address type")
	}
	ip := localAddr.IP.To4()
	if ip == nil {
		return nil, errors.New("no IPv4 address")
	}

	// 尝试常见的网关地址模式
	gatewayIPs := []net.IP{
		net.IPv4(ip[0], ip[1], ip[2], 1),   // x.x.x.1 (最常见)
		net.IPv4(ip[0], ip[1], ip[2], 254), // x.x.x.254 (某些路由器)
	}

	// 测试哪个网关地址是可达的
	for _, gw := range gatewayIPs {
		if isGatewayReachable(gw) {
			return gw, nil
		}
	}

	// 回退到 .1 地址
	return gatewayIPs[0], nil
}

// discoverGatewayFromInterfaces 从网络接口推断网关
func discoverGatewayFromInterfaces() (net.IP, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range interfaces {
		// 跳过回环和非活动接口
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			ip := ipNet.IP.To4()
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}

			// 找到一个有效的 IPv4 地址，尝试推断网关
			// 对于 /24 子网，网关通常是 x.x.x.1
			// 对于 /16 子网，网关通常是 x.x.0.1
			if ones, _ := ipNet.Mask.Size(); ones >= 24 {
				gw := net.IPv4(ip[0], ip[1], ip[2], 1)
				if isGatewayReachable(gw) {
					return gw, nil
				}
			} else if ones >= 16 {
				gw := net.IPv4(ip[0], ip[1], 0, 1)
				if isGatewayReachable(gw) {
					return gw, nil
				}
			}
		}
	}

	return nil, errors.New("no gateway found from interfaces")
}

// isGatewayReachable 检查网关是否支持 NAT-PMP
// 通过发送真实的 NAT-PMP 请求来验证网关可用性
func isGatewayReachable(gateway net.IP) bool {
	// 创建临时客户端，使用短超时
	client := natpmp.NewClientWithTimeout(gateway, 500*time.Millisecond)

	// 发送 GetExternalAddress 请求验证网关响应
	_, err := client.GetExternalAddress()
	return err == nil
}

// getLocalIP 获取本地 IP 地址
func getLocalIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer func() { _ = conn.Close() }()

	localAddr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return "127.0.0.1"
	}
	return localAddr.IP.String()
}

// ============================================================================
//                              ipAddr 实现（统一地址类型）
// ============================================================================

// ipAddr 统一的 IP 地址实现
// 此类型实现 endpoint.Address 接口，替代所有散落的 Address 实现
type ipAddr struct {
	ip   net.IP
	port int
}

// newIPAddr 创建新的 IP 地址
func newIPAddr(ip net.IP, port int) *ipAddr {
	return &ipAddr{ip: ip, port: port}
}

func (a *ipAddr) Network() string {
	if a.ip.To4() != nil {
		return "ip4"
	}
	return "ip6"
}

func (a *ipAddr) String() string {
	if a.port == 0 {
		return a.ip.String()
	}
	if a.ip.To4() != nil {
		return fmt.Sprintf("%s:%d", a.ip.String(), a.port)
	}
	return fmt.Sprintf("[%s]:%d", a.ip.String(), a.port)
}

func (a *ipAddr) Bytes() []byte {
	return []byte(a.String())
}

func (a *ipAddr) Equal(other endpoint.Address) bool {
	if other == nil {
		return false
	}
	return a.String() == other.String()
}

func (a *ipAddr) IsPublic() bool {
	// 0.0.0.0 和 :: 是未指定地址，不是公网地址
	return !a.IsPrivate() && !a.IsLoopback() && !a.ip.IsUnspecified()
}

func (a *ipAddr) IsPrivate() bool {
	return a.ip.IsPrivate()
}

func (a *ipAddr) IsLoopback() bool {
	return a.ip.IsLoopback()
}

func (a *ipAddr) Multiaddr() string {
	ipType := "ip4"
	if a.ip.To4() == nil {
		ipType = "ip6"
	}
	return fmt.Sprintf("/%s/%s/udp/%d/quic-v1", ipType, a.ip.String(), a.port)
}
