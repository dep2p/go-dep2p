// Package upnp 提供 UPnP IGD 端口映射实现
//
// UPnP IGD (Internet Gateway Device) 允许应用程序：
// - 在 NAT 路由器上创建端口映射
// - 获取外部 IP 地址
package upnp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/huin/goupnp"
	"github.com/huin/goupnp/dcps/internetgateway1"
	"github.com/huin/goupnp/dcps/internetgateway2"
	"github.com/huin/goupnp/httpu"
	"github.com/huin/goupnp/ssdp"
	"github.com/jackpal/gateway"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	natif "github.com/dep2p/go-dep2p/pkg/interfaces/nat"
)

// 包级别日志实例
var log = logger.Logger("nat.upnp")

// ============================================================================
//                              错误定义
// ============================================================================

// UPnP 相关错误
var (
	// ErrNoGateway 未找到 UPnP 网关
	ErrNoGateway = errors.New("no UPnP gateway found")
	ErrMappingFailed    = errors.New("port mapping failed")
	ErrUPnPNotSupported = errors.New("UPnP not supported")
	ErrGatewayNotReady  = errors.New("UPnP gateway not ready")
)

// ============================================================================
//                              地址过滤（SSDP 候选地址选择）
// ============================================================================

// virtualIfacePrefixes 虚拟网卡名称前缀黑名单
var virtualIfacePrefixes = []string{
	"utun",        // macOS/iOS VPN tunnel
	"bridge",      // Linux bridge
	"awdl",        // Apple Wireless Direct Link
	"llw",         // Low Latency WLAN
	"lo",          // Loopback
	"loopback",    // Windows loopback pseudo interface
	"gif",         // Generic tunnel interface
	"stf",         // 6to4 tunnel
	"tun",         // TUN device
	"tap",         // TAP device
	"tap-windows", // Windows TAP adapter (OpenVPN/WireGuard)
	"wintun",      // WireGuard Wintun
	"vethernet",   // Hyper-V vEthernet
	"hyper-v",     // Hyper-V
	"docker",      // Docker bridge
	"vboxnet",     // VirtualBox
	"virtualbox",  // VirtualBox on Windows
	"vmnet",       // VMware
	"vmware",      // VMware on Windows
	"veth",        // Virtual Ethernet
	"virbr",       // libvirt bridge
	"br-",         // Docker custom bridge
	"cni",         // Kubernetes CNI
	"flannel",     // Flannel overlay
	"calico",      // Calico overlay
	"npf",         // Npcap loopback / capture adapters
}

// blockedCIDRs 不可用于 SSDP 的地址段
var blockedCIDRs = []*net.IPNet{
	mustParseCIDR("127.0.0.0/8"),    // Loopback
	mustParseCIDR("169.254.0.0/16"), // Link-local
	mustParseCIDR("198.18.0.0/15"),  // Benchmark testing (常见 VPN 隧道地址)
	mustParseCIDR("100.64.0.0/10"),  // CGNAT (Carrier-grade NAT)
	mustParseCIDR("224.0.0.0/4"),    // Multicast
	mustParseCIDR("240.0.0.0/4"),    // Reserved
	mustParseCIDR("::1/128"),        // IPv6 loopback
	mustParseCIDR("fe80::/10"),      // IPv6 link-local
	mustParseCIDR("fc00::/7"),       // IPv6 ULA
}

// rfc1918CIDRs RFC1918 私有地址段（优先使用）
var rfc1918CIDRs = []*net.IPNet{
	mustParseCIDR("10.0.0.0/8"),
	mustParseCIDR("172.16.0.0/12"),
	mustParseCIDR("192.168.0.0/16"),
}

func mustParseCIDR(s string) *net.IPNet {
	_, ipnet, err := net.ParseCIDR(s)
	if err != nil {
		panic(fmt.Sprintf("invalid CIDR: %s", s))
	}
	return ipnet
}

// isVirtualInterface 判断是否为虚拟网卡
func isVirtualInterface(name string) bool {
	nameLower := strings.ToLower(name)
	for _, prefix := range virtualIfacePrefixes {
		if strings.HasPrefix(nameLower, prefix) {
			return true
		}
	}
	return false
}

// isBlockedIP 判断 IP 是否在黑名单地址段
func isBlockedIP(ip net.IP) bool {
	for _, cidr := range blockedCIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// isRFC1918 判断是否为 RFC1918 私有地址
func isRFC1918(ip net.IP) bool {
	for _, cidr := range rfc1918CIDRs {
		if cidr.Contains(ip) {
			return true
		}
	}
	return false
}

// getCandidateLANAddresses 获取适合发送 SSDP 的候选 LAN 地址
//
// 过滤规则：
// - 接口必须 UP 且支持 Multicast
// - 排除 Loopback
// - 排除虚拟网卡（utun/bridge/docker 等）
// - 排除黑名单地址段（127/169.254/198.18 等）
// - 优先返回 RFC1918 地址（10/172.16/192.168）
func getCandidateLANAddresses() []net.IP {
	type candidateIP struct {
		ip    net.IP
		ipnet *net.IPNet // 可能为空（例如 *net.IPAddr）
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		log.Debug("获取网络接口失败", "err", err)
		return nil
	}

	var candidates []candidateIP
	var nonRFC1918 []candidateIP

	for _, iface := range ifaces {
		// 1) 检查接口状态
		if iface.Flags&net.FlagUp == 0 {
			continue // 接口未启用
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue // 跳过 loopback
		}
		if iface.Flags&net.FlagMulticast == 0 {
			continue // 不支持组播
		}

		// 2) 检查接口名称（排除虚拟网卡）
		if isVirtualInterface(iface.Name) {
			log.Debug("跳过虚拟网卡", "iface", iface.Name)
			continue
		}

		// 3) 获取接口地址
		addrs, err := iface.Addrs()
		if err != nil {
			log.Debug("获取接口地址失败", "iface", iface.Name, "err", err)
			continue
		}

		for _, addr := range addrs {
			var (
				ip    net.IP
				ipnet *net.IPNet
			)
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
				ipnet = v
			case *net.IPAddr:
				ip = v.IP
			default:
				continue
			}

			// 只处理 IPv4（SSDP 主要基于 IPv4 组播）
			if ip4 := ip.To4(); ip4 == nil {
				continue
			}

			// 4) 检查地址是否在黑名单
			if isBlockedIP(ip) {
				log.Debug("跳过黑名单地址", "iface", iface.Name, "ip", ip.String())
				continue
			}

			// 5) 分类存储（RFC1918 优先）
			if isRFC1918(ip) {
				candidates = append(candidates, candidateIP{ip: ip, ipnet: ipnet})
				log.Debug("发现 RFC1918 候选地址", "iface", iface.Name, "ip", ip.String())
			} else {
				nonRFC1918 = append(nonRFC1918, candidateIP{ip: ip, ipnet: ipnet})
				log.Debug("发现非 RFC1918 候选地址", "iface", iface.Name, "ip", ip.String())
			}
		}
	}

	// RFC1918 地址优先，且在多网卡/多私网地址时，优先选择与默认网关同网段的地址（best-effort）
	if len(candidates) > 1 {
		if gw, gwErr := gateway.DiscoverGateway(); gwErr == nil && gw != nil && gw.To4() != nil {
			var sameSubnet []candidateIP
			var other []candidateIP
			for _, c := range candidates {
				if c.ipnet != nil && c.ipnet.Contains(gw) {
					sameSubnet = append(sameSubnet, c)
				} else {
					other = append(other, c)
				}
			}
			if len(sameSubnet) > 0 {
				candidates = append(sameSubnet, other...)
			}
		}
	}

	// 输出：RFC1918 在前，其他地址作为备选
	out := make([]net.IP, 0, len(candidates)+len(nonRFC1918))
	for _, c := range candidates {
		out = append(out, c.ip)
	}
	for _, c := range nonRFC1918 {
		out = append(out, c.ip)
	}
	return out
}

// ============================================================================
//                              IGD 客户端接口
// ============================================================================

// igdClient 抽象 IGD 客户端接口
type igdClient interface {
	GetExternalIPAddress() (string, error)
	AddPortMapping(
		newRemoteHost string,
		newExternalPort uint16,
		newProtocol string,
		newInternalPort uint16,
		newInternalClient string,
		newEnabled bool,
		newPortMappingDescription string,
		newLeaseDuration uint32,
	) error
	DeletePortMapping(
		newRemoteHost string,
		newExternalPort uint16,
		newProtocol string,
	) error
	GetServiceClient() *goupnp.ServiceClient

	// GetGenericPortMappingEntry 枚举端口映射条目
	// 用于启动时清理历史映射，避免映射表膨胀
	// index: 从 0 开始的索引，直到返回错误表示枚举结束
	GetGenericPortMappingEntry(index uint16) (
		remoteHost string, externalPort uint16, protocol string,
		internalPort uint16, internalClient string, enabled bool,
		description string, leaseDuration uint32, err error,
	)
}

// ============================================================================
//                              IGDv1 包装器
// ============================================================================

type igdv1Wrapper struct {
	client *internetgateway1.WANIPConnection1
}

func (w *igdv1Wrapper) GetExternalIPAddress() (string, error) {
	return w.client.GetExternalIPAddress()
}

func (w *igdv1Wrapper) AddPortMapping(
	remoteHost string, externalPort uint16, protocol string,
	internalPort uint16, internalClient string, enabled bool,
	description string, leaseDuration uint32,
) error {
	return w.client.AddPortMapping(
		remoteHost, externalPort, protocol, internalPort,
		internalClient, enabled, description, leaseDuration,
	)
}

func (w *igdv1Wrapper) DeletePortMapping(remoteHost string, externalPort uint16, protocol string) error {
	return w.client.DeletePortMapping(remoteHost, externalPort, protocol)
}

func (w *igdv1Wrapper) GetServiceClient() *goupnp.ServiceClient {
	return &w.client.ServiceClient
}

func (w *igdv1Wrapper) GetGenericPortMappingEntry(index uint16) (
	remoteHost string, externalPort uint16, protocol string,
	internalPort uint16, internalClient string, enabled bool,
	description string, leaseDuration uint32, err error,
) {
	return w.client.GetGenericPortMappingEntry(index)
}

// ============================================================================
//                              IGDv2 包装器
// ============================================================================

type igdv2Wrapper struct {
	client *internetgateway2.WANIPConnection2
}

func (w *igdv2Wrapper) GetExternalIPAddress() (string, error) {
	return w.client.GetExternalIPAddress()
}

func (w *igdv2Wrapper) AddPortMapping(
	remoteHost string, externalPort uint16, protocol string,
	internalPort uint16, internalClient string, enabled bool,
	description string, leaseDuration uint32,
) error {
	return w.client.AddPortMapping(
		remoteHost, externalPort, protocol, internalPort,
		internalClient, enabled, description, leaseDuration,
	)
}

func (w *igdv2Wrapper) DeletePortMapping(remoteHost string, externalPort uint16, protocol string) error {
	return w.client.DeletePortMapping(remoteHost, externalPort, protocol)
}

func (w *igdv2Wrapper) GetServiceClient() *goupnp.ServiceClient {
	return &w.client.ServiceClient
}

func (w *igdv2Wrapper) GetGenericPortMappingEntry(index uint16) (
	remoteHost string, externalPort uint16, protocol string,
	internalPort uint16, internalClient string, enabled bool,
	description string, leaseDuration uint32, err error,
) {
	return w.client.GetGenericPortMappingEntry(index)
}

// ============================================================================
//                              IGDv2 PPP 包装器
// ============================================================================

type igdv2PPPWrapper struct {
	client *internetgateway2.WANPPPConnection1
}

func (w *igdv2PPPWrapper) GetExternalIPAddress() (string, error) {
	return w.client.GetExternalIPAddress()
}

func (w *igdv2PPPWrapper) AddPortMapping(
	remoteHost string, externalPort uint16, protocol string,
	internalPort uint16, internalClient string, enabled bool,
	description string, leaseDuration uint32,
) error {
	return w.client.AddPortMapping(
		remoteHost, externalPort, protocol, internalPort,
		internalClient, enabled, description, leaseDuration,
	)
}

func (w *igdv2PPPWrapper) DeletePortMapping(remoteHost string, externalPort uint16, protocol string) error {
	return w.client.DeletePortMapping(remoteHost, externalPort, protocol)
}

func (w *igdv2PPPWrapper) GetServiceClient() *goupnp.ServiceClient {
	return &w.client.ServiceClient
}

func (w *igdv2PPPWrapper) GetGenericPortMappingEntry(index uint16) (
	remoteHost string, externalPort uint16, protocol string,
	internalPort uint16, internalClient string, enabled bool,
	description string, leaseDuration uint32, err error,
) {
	return w.client.GetGenericPortMappingEntry(index)
}

// ============================================================================
//                              IGDv1 PPP 包装器
// ============================================================================

type igdv1PPPWrapper struct {
	client *internetgateway1.WANPPPConnection1
}

func (w *igdv1PPPWrapper) GetExternalIPAddress() (string, error) {
	return w.client.GetExternalIPAddress()
}

func (w *igdv1PPPWrapper) AddPortMapping(
	remoteHost string, externalPort uint16, protocol string,
	internalPort uint16, internalClient string, enabled bool,
	description string, leaseDuration uint32,
) error {
	return w.client.AddPortMapping(
		remoteHost, externalPort, protocol, internalPort,
		internalClient, enabled, description, leaseDuration,
	)
}

func (w *igdv1PPPWrapper) DeletePortMapping(remoteHost string, externalPort uint16, protocol string) error {
	return w.client.DeletePortMapping(remoteHost, externalPort, protocol)
}

func (w *igdv1PPPWrapper) GetServiceClient() *goupnp.ServiceClient {
	return &w.client.ServiceClient
}

func (w *igdv1PPPWrapper) GetGenericPortMappingEntry(index uint16) (
	remoteHost string, externalPort uint16, protocol string,
	internalPort uint16, internalClient string, enabled bool,
	description string, leaseDuration uint32, err error,
) {
	return w.client.GetGenericPortMappingEntry(index)
}

// ============================================================================
//                              Mapper 结构
// ============================================================================

// Mapper UPnP 端口映射器实现
type Mapper struct {
	// IGD 客户端
	client     igdClient
	clientMu   sync.RWMutex
	discovered bool

	// 网关信息
	externalIP string
	localIP    string

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

// NewMapper 创建 UPnP 映射器
func NewMapper() *Mapper {
	localIP := getLocalIP()

	return &Mapper{
		mappings:      make(map[string]*natif.Mapping),
		timeout:       10 * time.Second,
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

// staleMappingEntry 待清理的映射条目
type staleMappingEntry struct {
	externalPort uint16
	protocol     string
	description  string
}

// cleanupStaleMappings 清理 dep2p 历史端口映射
//
// 在网关发现成功后调用，枚举路由器上的现有映射，
// 清理描述符包含 "dep2p" 的旧条目，避免映射表膨胀。
//
// 清理策略（保守模式）：
// 1. 先完整枚举所有映射，收集需要删除的条目
// 2. 枚举完成后，再逐个删除
//
// 这种两阶段策略不依赖路由器的索引实现细节：
// - 有些路由器删除条目后索引会"滑动"（后续条目向前移动）
// - 有些路由器删除条目后索引不变（留下空位）
// 采用先收集后删除的方式，避免索引变化导致的遗漏或重复处理。
//
// 注意：调用者应持有 clientMu 锁
func (m *Mapper) cleanupStaleMappings() {
	if m.client == nil {
		return
	}

	// 阶段 1：枚举并收集需要清理的映射
	var toDelete []staleMappingEntry
	for i := uint16(0); i < 1000; i++ { // 限制枚举上限，防止死循环
		_, extPort, proto, _, _, _, desc, _, err := m.client.GetGenericPortMappingEntry(i)
		if err != nil {
			// 枚举结束或出错
			break
		}

		// 检查是否为 dep2p 创建的映射
		if strings.Contains(strings.ToLower(desc), "dep2p") {
			toDelete = append(toDelete, staleMappingEntry{
				externalPort: extPort,
				protocol:     proto,
				description:  desc,
			})
		}
	}

	if len(toDelete) == 0 {
		return
	}

	log.Info("发现历史 dep2p 端口映射，准备清理", "count", len(toDelete))

	// 阶段 2：逐个删除收集到的映射
	cleanedCount := 0
	for _, entry := range toDelete {
		if delErr := m.client.DeletePortMapping("", entry.externalPort, entry.protocol); delErr != nil {
			log.Debug("清理历史映射失败",
				"port", entry.externalPort,
				"protocol", entry.protocol,
				"description", entry.description,
				"err", delErr)
		} else {
			log.Info("清理历史映射",
				"port", entry.externalPort,
				"protocol", entry.protocol,
				"description", entry.description)
			cleanedCount++
		}
	}

	if cleanedCount > 0 {
		log.Info("启动时清理历史 dep2p 端口映射完成",
			"cleaned", cleanedCount,
			"total", len(toDelete))
	}
}

// ============================================================================
//                              PortMapper 接口实现
// ============================================================================

// Name 返回映射器名称
func (m *Mapper) Name() string {
	return "upnp"
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
	localIP := m.localIP
	externalIP := m.externalIP // 在持锁时复制 externalIP
	m.clientMu.RUnlock()

	if client == nil {
		// 尝试发现
		ctx, cancel := context.WithTimeout(context.Background(), m.getTimeout())
		defer cancel()
		if err := m.discoverGateway(ctx); err != nil {
			return 0, fmt.Errorf("%w: %v", ErrUPnPNotSupported, err)
		}
		m.clientMu.RLock()
		client = m.client
		localIP = m.localIP
		externalIP = m.externalIP // 再次在持锁时复制
		m.clientMu.RUnlock()
	}

	if client == nil {
		return 0, ErrGatewayNotReady
	}

	// 使用相同的内外端口
	externalPort := uint16(internalPort)

	// 计算租约时间（秒）
	leaseDuration := uint32(duration.Seconds())
	if leaseDuration == 0 {
		leaseDuration = 3600 // 默认 1 小时
	}

	// 转换协议名称为大写
	proto := "UDP"
	if protocol == "tcp" || protocol == "TCP" {
		proto = "TCP"
	}

	// 调用 UPnP AddPortMapping
	err := client.AddPortMapping(
		"",                   // remoteHost - 空表示任意
		externalPort,         // externalPort
		proto,                // protocol
		uint16(internalPort), // internalPort
		localIP,              // internalClient
		true,                 // enabled
		description,          // description
		leaseDuration,        // leaseDuration
	)
	if err != nil {
		log.Warn("UPnP 端口映射失败",
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
		ExternalPort: int(externalPort),
		ExternalAddr: externalIP, // 使用本地复制的值
		Description:  description,
		Expiry:       time.Now().Add(duration),
	}
	m.mappingsMu.Unlock()

	log.Info("UPnP 端口映射成功",
		"protocol", protocol,
		"internalPort", internalPort,
		"externalPort", int(externalPort),
		"localIP", localIP)

	return int(externalPort), nil
}

// DeleteMapping 删除端口映射
func (m *Mapper) DeleteMapping(protocol string, externalPort int) error {
	m.clientMu.RLock()
	client := m.client
	m.clientMu.RUnlock()

	if client == nil {
		return nil // 没有网关，无需删除
	}

	// 转换协议名称为大写
	proto := "UDP"
	if protocol == "tcp" || protocol == "TCP" {
		proto = "TCP"
	}

	// 调用 UPnP DeletePortMapping
	err := client.DeletePortMapping("", uint16(externalPort), proto)
	if err != nil {
		log.Debug("删除 UPnP 端口映射失败",
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

	log.Info("UPnP 端口映射已删除",
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
			log.Warn("刷新 UPnP 映射失败",
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

		log.Info("关闭 UPnP 映射器")

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
				proto := "UDP"
				if mapping.Protocol == "tcp" || mapping.Protocol == "TCP" {
					proto = "TCP"
				}
				_ = client.DeletePortMapping("", uint16(mapping.ExternalPort), proto)
				log.Debug("清理 UPnP 端口映射",
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

// ssdpSearchTargets SSDP 搜索目标（按优先级排序）
// 注意：WANPPPConnection:1 在 IGDv1 和 IGDv2 中 URN 相同，需要通过不同工厂尝试创建
var ssdpSearchTargets = []struct {
	target      string
	description string
	igdVersion  int // 1 或 2
	isPPP       bool
}{
	{internetgateway2.URN_WANIPConnection_2, "IGDv2-WANIPConnection2", 2, false},
	{internetgateway2.URN_WANPPPConnection_1, "IGDv2-WANPPPConnection1", 2, true},
	{internetgateway1.URN_WANIPConnection_1, "IGDv1-WANIPConnection1", 1, false},
	{internetgateway1.URN_WANPPPConnection_1, "IGDv1-WANPPPConnection1", 1, true},
}

// ssdpSearchFromAddr 从指定本地 IP 地址发起 SSDP 搜索
//
// 返回值：
// - 找到的设备 Location URL 列表
// - 错误（如果绑定本地地址失败）
func ssdpSearchFromAddr(ctx context.Context, localIP net.IP, searchTarget string) ([]*url.URL, error) {
	// 创建绑定到指定 IP 的 HTTPU 客户端
	client, err := httpu.NewHTTPUClientAddr(localIP.String())
	if err != nil {
		return nil, fmt.Errorf("绑定本地地址 %s 失败: %w", localIP.String(), err)
	}
	defer func() { _ = client.Close() }()

	// 设置搜索超时（从 ctx 继承，或默认 3 秒）
	searchCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	// 执行 SSDP 搜索
	responses, err := ssdp.RawSearch(searchCtx, client, searchTarget, 3)
	if err != nil {
		return nil, fmt.Errorf("SSDP 搜索失败: %w", err)
	}

	// 提取有效的 Location URL
	var locations []*url.URL
	for _, resp := range responses {
		loc, err := resp.Location()
		if err != nil {
			continue
		}
		locations = append(locations, loc)
	}

	return locations, nil
}

// tryCreateClientFromLocation 尝试从 Location URL 创建 IGD 客户端
func (m *Mapper) tryCreateClientFromLocation(ctx context.Context, loc *url.URL, igdVersion int, isPPP bool, _ string) bool {
	if igdVersion == 2 && !isPPP {
		// IGDv2: WANIPConnection2
		if clients, err := internetgateway2.NewWANIPConnection2ClientsByURLCtx(ctx, loc); err == nil && len(clients) > 0 {
			m.client = &igdv2Wrapper{client: clients[0]}
			m.discovered = true
			if extIP, err := m.client.GetExternalIPAddress(); err == nil {
				m.externalIP = extIP
			}
			if sc := m.client.GetServiceClient(); sc != nil && sc.RootDevice != nil {
				log.Info("发现 UPnP IGDv2 网关（WANIPConnection2）",
					"device", sc.RootDevice.Device.FriendlyName,
					"externalIP", m.externalIP,
					"location", loc.String())
			}
			return true
		}
	} else if igdVersion == 2 && isPPP {
		// IGDv2: WANPPPConnection1
		if clients, err := internetgateway2.NewWANPPPConnection1ClientsByURLCtx(ctx, loc); err == nil && len(clients) > 0 {
			m.client = &igdv2PPPWrapper{client: clients[0]}
			m.discovered = true
			if extIP, err := m.client.GetExternalIPAddress(); err == nil {
				m.externalIP = extIP
			}
			if sc := m.client.GetServiceClient(); sc != nil && sc.RootDevice != nil {
				log.Info("发现 UPnP IGDv2 网关（WANPPPConnection1）",
					"device", sc.RootDevice.Device.FriendlyName,
					"externalIP", m.externalIP,
					"location", loc.String())
			}
			return true
		}
	} else if igdVersion == 1 && !isPPP {
		// IGDv1: WANIPConnection1
		if clients, err := internetgateway1.NewWANIPConnection1ClientsByURLCtx(ctx, loc); err == nil && len(clients) > 0 {
			m.client = &igdv1Wrapper{client: clients[0]}
			m.discovered = true
			if extIP, err := m.client.GetExternalIPAddress(); err == nil {
				m.externalIP = extIP
			}
			if sc := m.client.GetServiceClient(); sc != nil && sc.RootDevice != nil {
				log.Info("发现 UPnP IGDv1 网关（WANIPConnection1）",
					"device", sc.RootDevice.Device.FriendlyName,
					"externalIP", m.externalIP,
					"location", loc.String())
			}
			return true
		}
	} else if igdVersion == 1 && isPPP {
		// IGDv1: WANPPPConnection1
		if clients, err := internetgateway1.NewWANPPPConnection1ClientsByURLCtx(ctx, loc); err == nil && len(clients) > 0 {
			m.client = &igdv1PPPWrapper{client: clients[0]}
			m.discovered = true
			if extIP, err := m.client.GetExternalIPAddress(); err == nil {
				m.externalIP = extIP
			}
			if sc := m.client.GetServiceClient(); sc != nil && sc.RootDevice != nil {
				log.Info("发现 UPnP IGDv1 网关（WANPPPConnection1）",
					"device", sc.RootDevice.Device.FriendlyName,
					"externalIP", m.externalIP,
					"location", loc.String())
			}
			return true
		}
	}

	return false
}

// discoverGateway 发现 UPnP 网关
//
// 改进：使用过滤后的 LAN 候选地址逐个尝试 SSDP 搜索，
// 避免从虚拟网卡（utun/bridge/198.18 等）发送 SSDP 导致 "no route to host"。
func (m *Mapper) discoverGateway(ctx context.Context) error {
	m.clientMu.Lock()
	defer m.clientMu.Unlock()

	if m.discovered && m.client != nil {
		return nil
	}

	log.Debug("开始发现 UPnP 网关（使用过滤后的 LAN 地址）...")

	// 1) 获取候选 LAN 地址
	candidates := getCandidateLANAddresses()
	if len(candidates) == 0 {
		log.Debug("未找到可用的 LAN 候选地址，回退到默认 SSDP 发现")
		return m.discoverGatewayFallback(ctx)
	}

	log.Debug("找到候选 LAN 地址", "count", len(candidates), "addrs", formatIPs(candidates))

	// 2) 对每个候选地址尝试 SSDP 搜索
	for _, localIP := range candidates {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		log.Debug("尝试从本地地址发起 SSDP 搜索", "localIP", localIP.String())

		// 3) 按优先级尝试各种服务类型
		for _, st := range ssdpSearchTargets {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			locations, err := ssdpSearchFromAddr(ctx, localIP, st.target)
			if err != nil {
				log.Debug("SSDP 搜索失败",
					"localIP", localIP.String(),
					"target", st.description,
					"err", err)
				continue
			}

			if len(locations) == 0 {
				log.Debug("SSDP 未发现设备",
					"localIP", localIP.String(),
					"target", st.description)
				continue
			}

			log.Debug("SSDP 发现设备",
				"localIP", localIP.String(),
				"target", st.description,
				"locations", len(locations))

			// 4) 尝试从 Location 创建客户端
			for _, loc := range locations {
				if m.tryCreateClientFromLocation(ctx, loc, st.igdVersion, st.isPPP, st.description) {
					// 关键：将 internalClient 固定为本次成功的 LAN 源地址
					// 避免 getLocalIP() 走 VPN/隧道地址，导致 AddPortMapping internalClient 不可达。
					m.localIP = localIP.String()
					// 启动时清理历史 dep2p 映射，避免映射表膨胀
					m.cleanupStaleMappings()
					return nil
				}
			}
		}
	}

	// 5) 所有候选地址都失败，回退到默认方式
	log.Debug("所有候选地址的 SSDP 搜索均失败，回退到默认发现")
	return m.discoverGatewayFallback(ctx)
}

// discoverGatewayFallback 回退到 goupnp 默认的 SSDP 发现方式
//
// 当过滤后的候选地址都无法发现网关时，作为最后的尝试。
func (m *Mapper) discoverGatewayFallback(ctx context.Context) error {
	log.Debug("使用 goupnp 默认 SSDP 发现（回退）...")

	// 按顺序尝试各种 IGD 服务

	// 1) IGDv2: WANIPConnection2
	if clients2, _, err := internetgateway2.NewWANIPConnection2ClientsCtx(ctx); err == nil && len(clients2) > 0 {
		m.client = &igdv2Wrapper{client: clients2[0]}
		m.discovered = true
		if extIP, err := m.client.GetExternalIPAddress(); err == nil {
			m.externalIP = extIP
		}
		if sc := m.client.GetServiceClient(); sc != nil && sc.RootDevice != nil {
			log.Info("发现 UPnP IGDv2 网关（WANIPConnection2，回退模式）",
				"device", sc.RootDevice.Device.FriendlyName,
				"externalIP", m.externalIP)
		}
		// 启动时清理历史 dep2p 映射
		m.cleanupStaleMappings()
		return nil
	}

	// 2) IGDv2: WANPPPConnection1
	if clients2ppp, _, err := internetgateway2.NewWANPPPConnection1ClientsCtx(ctx); err == nil && len(clients2ppp) > 0 {
		m.client = &igdv2PPPWrapper{client: clients2ppp[0]}
		m.discovered = true
		if extIP, err := m.client.GetExternalIPAddress(); err == nil {
			m.externalIP = extIP
		}
		if sc := m.client.GetServiceClient(); sc != nil && sc.RootDevice != nil {
			log.Info("发现 UPnP IGDv2 网关（WANPPPConnection1，回退模式）",
				"device", sc.RootDevice.Device.FriendlyName,
				"externalIP", m.externalIP)
		}
		// 启动时清理历史 dep2p 映射
		m.cleanupStaleMappings()
		return nil
	}

	// 3) IGDv1: WANIPConnection1
	if clients1, _, err := internetgateway1.NewWANIPConnection1ClientsCtx(ctx); err == nil && len(clients1) > 0 {
		m.client = &igdv1Wrapper{client: clients1[0]}
		m.discovered = true
		if extIP, err := m.client.GetExternalIPAddress(); err == nil {
			m.externalIP = extIP
		}
		if sc := m.client.GetServiceClient(); sc != nil && sc.RootDevice != nil {
			log.Info("发现 UPnP IGDv1 网关（WANIPConnection1，回退模式）",
				"device", sc.RootDevice.Device.FriendlyName,
				"externalIP", m.externalIP)
		}
		// 启动时清理历史 dep2p 映射
		m.cleanupStaleMappings()
		return nil
	}

	// 4) IGDv1: WANPPPConnection1
	if clients1ppp, _, err := internetgateway1.NewWANPPPConnection1ClientsCtx(ctx); err == nil && len(clients1ppp) > 0 {
		m.client = &igdv1PPPWrapper{client: clients1ppp[0]}
		m.discovered = true
		if extIP, err := m.client.GetExternalIPAddress(); err == nil {
			m.externalIP = extIP
		}
		if sc := m.client.GetServiceClient(); sc != nil && sc.RootDevice != nil {
			log.Info("发现 UPnP IGDv1 网关（WANPPPConnection1，回退模式）",
				"device", sc.RootDevice.Device.FriendlyName,
				"externalIP", m.externalIP)
		}
		// 启动时清理历史 dep2p 映射
		m.cleanupStaleMappings()
		return nil
	}

	return ErrNoGateway
}

// formatIPs 格式化 IP 列表用于日志输出
func formatIPs(ips []net.IP) string {
	if len(ips) == 0 {
		return "[]"
	}
	strs := make([]string, len(ips))
	for i, ip := range ips {
		strs[i] = ip.String()
	}
	return "[" + strings.Join(strs, ", ") + "]"
}

// ============================================================================
//                              辅助函数
// ============================================================================

// getLocalIP 获取本地 IP 地址
func getLocalIP() string {
	// 与 SSDP 候选过滤逻辑保持一致：优先选择过滤后的 LAN 地址
	candidates := getCandidateLANAddresses()
	if len(candidates) > 0 {
		return candidates[0].String()
	}
	return "127.0.0.1"
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
