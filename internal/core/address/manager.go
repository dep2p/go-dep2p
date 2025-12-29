// Package address 提供地址管理模块的实现
package address

import (
	"context"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	natif "github.com/dep2p/go-dep2p/pkg/interfaces/nat"
)

// 包级别日志实例
var log = logger.Logger("address")

// ============================================================================
//                              AddressManager 实现
// ============================================================================

// AddressManager 地址管理器实现
//
// 管理节点的监听地址和通告地址：
// - 监听地址：节点实际绑定的本地地址
// - 通告地址：向网络通告的公网可达地址
type AddressManager struct {
	// 监听地址
	listenAddrs   []endpoint.Address
	listenAddrsMu sync.RWMutex

	// 通告地址
	advertisedAddrs   []endpoint.Address
	advertisedAddrsMu sync.RWMutex

	// NAT 服务（用于发现外部地址）
	natService   natif.NATService
	natServiceMu sync.RWMutex

	// 地址优先级配置
	config AddressManagerConfig

	// 运行状态
	ctx     context.Context
	cancel  context.CancelFunc
	running bool
	mu      sync.Mutex
}

// AddressManagerConfig 地址管理器配置
type AddressManagerConfig struct {
	// RefreshInterval 地址刷新间隔
	RefreshInterval time.Duration

	// PreferPublic 优先使用公网地址
	PreferPublic bool

	// PreferIPv6 优先使用 IPv6
	PreferIPv6 bool

	// FilterLocalAddrs 过滤本地地址（如 127.0.0.1）
	FilterLocalAddrs bool

	// FilterPrivateAddrs 过滤私有地址（对外通告时）
	FilterPrivateAddrs bool
}

// DefaultAddressManagerConfig 返回默认配置
func DefaultAddressManagerConfig() AddressManagerConfig {
	return AddressManagerConfig{
		RefreshInterval:    5 * time.Minute,
		PreferPublic:       true,
		PreferIPv6:         false,
		FilterLocalAddrs:   true,
		FilterPrivateAddrs: true,
	}
}

// NewAddressManager 创建地址管理器
func NewAddressManager(config AddressManagerConfig) *AddressManager {
	return &AddressManager{
		listenAddrs:     make([]endpoint.Address, 0),
		advertisedAddrs: make([]endpoint.Address, 0),
		config:          config,
	}
}

// SetNATService 设置 NAT 服务
func (m *AddressManager) SetNATService(nat natif.NATService) {
	m.natServiceMu.Lock()
	m.natService = nat
	m.natServiceMu.Unlock()
}

// getNATService 获取 NAT 服务（线程安全）
func (m *AddressManager) getNATService() natif.NATService {
	m.natServiceMu.RLock()
	defer m.natServiceMu.RUnlock()
	return m.natService
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动地址管理器
func (m *AddressManager) Start(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.running {
		return nil
	}

	// 使用 context.Background() 而非 ctx，因为 Fx OnStart 的 ctx 在 OnStart 返回后会被取消
	// 这会导致后台循环 (refreshLoop, discoverExternalAddresses) 提前退出
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.running = true

	// 初始发现外部地址
	go m.discoverExternalAddresses()

	// 定期刷新
	go m.refreshLoop()

	log.Info("地址管理器已启动")
	return nil
}

// Stop 停止地址管理器
func (m *AddressManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.running {
		return nil
	}

	if m.cancel != nil {
		m.cancel()
	}

	m.running = false
	log.Info("地址管理器已停止")
	return nil
}

// ============================================================================
//                              监听地址管理
// ============================================================================

// ListenAddrs 返回监听地址列表
func (m *AddressManager) ListenAddrs() []endpoint.Address {
	m.listenAddrsMu.RLock()
	defer m.listenAddrsMu.RUnlock()

	result := make([]endpoint.Address, len(m.listenAddrs))
	copy(result, m.listenAddrs)
	return result
}

// AddListenAddr 添加监听地址
func (m *AddressManager) AddListenAddr(addr endpoint.Address) {
	m.listenAddrsMu.Lock()

	// 检查是否已存在
	for _, existing := range m.listenAddrs {
		if existing.Equal(addr) {
			m.listenAddrsMu.Unlock()
			return
		}
	}

	m.listenAddrs = append(m.listenAddrs, addr)
	isPublic := addr.IsPublic()
	m.listenAddrsMu.Unlock()

	log.Debug("添加监听地址", "addr", addr.String())

	// 同时更新通告地址（如果是公网地址）
	// 注意：使用 AddAdvertisedAddr 获取正确的锁，避免跨锁调用
	if isPublic {
		m.AddAdvertisedAddr(addr)
	}
}

// RemoveListenAddr 移除监听地址
func (m *AddressManager) RemoveListenAddr(addr endpoint.Address) {
	m.listenAddrsMu.Lock()
	defer m.listenAddrsMu.Unlock()

	for i, existing := range m.listenAddrs {
		if existing.Equal(addr) {
			m.listenAddrs = append(m.listenAddrs[:i], m.listenAddrs[i+1:]...)
			log.Debug("移除监听地址", "addr", addr.String())
			return
		}
	}
}

// ============================================================================
//                              通告地址管理
// ============================================================================

// AdvertisedAddrs 返回通告地址列表
func (m *AddressManager) AdvertisedAddrs() []endpoint.Address {
	m.advertisedAddrsMu.RLock()
	defer m.advertisedAddrsMu.RUnlock()

	result := make([]endpoint.Address, len(m.advertisedAddrs))
	copy(result, m.advertisedAddrs)
	return result
}

// AddAdvertisedAddr 添加通告地址
func (m *AddressManager) AddAdvertisedAddr(addr endpoint.Address) {
	m.advertisedAddrsMu.Lock()
	defer m.advertisedAddrsMu.Unlock()
	m.addAdvertisedAddrLocked(addr)
}

// addAdvertisedAddrLocked 添加通告地址（需持有锁）
func (m *AddressManager) addAdvertisedAddrLocked(addr endpoint.Address) {
	// 检查是否已存在
	for _, existing := range m.advertisedAddrs {
		if existing.Equal(addr) {
			return
		}
	}

	m.advertisedAddrs = append(m.advertisedAddrs, addr)
	log.Debug("添加通告地址", "addr", addr.String())
}

// RemoveAdvertisedAddr 移除通告地址
func (m *AddressManager) RemoveAdvertisedAddr(addr endpoint.Address) {
	m.advertisedAddrsMu.Lock()
	defer m.advertisedAddrsMu.Unlock()

	for i, existing := range m.advertisedAddrs {
		if existing.Equal(addr) {
			m.advertisedAddrs = append(m.advertisedAddrs[:i], m.advertisedAddrs[i+1:]...)
			log.Debug("移除通告地址", "addr", addr.String())
			return
		}
	}
}

// SetAdvertisedAddrs 设置通告地址（替换现有）
func (m *AddressManager) SetAdvertisedAddrs(addrs []endpoint.Address) {
	m.advertisedAddrsMu.Lock()
	defer m.advertisedAddrsMu.Unlock()

	m.advertisedAddrs = make([]endpoint.Address, len(addrs))
	copy(m.advertisedAddrs, addrs)
}

// ============================================================================
//                              最佳地址选择
// ============================================================================

// BestAddr 返回最佳地址
//
// 根据以下优先级选择：
// 1. 公网 IPv4/IPv6 地址
// 2. NAT 映射地址
// 3. 中继地址
func (m *AddressManager) BestAddr() endpoint.Address {
	m.advertisedAddrsMu.RLock()
	defer m.advertisedAddrsMu.RUnlock()

	if len(m.advertisedAddrs) == 0 {
		return nil
	}

	// 按优先级排序
	sorted := m.sortAddresses(m.advertisedAddrs)
	if len(sorted) > 0 {
		return sorted[0]
	}
	return nil
}

// sortAddresses 按优先级排序地址
func (m *AddressManager) sortAddresses(addrs []endpoint.Address) []endpoint.Address {
	result := make([]endpoint.Address, len(addrs))
	copy(result, addrs)

	sort.Slice(result, func(i, j int) bool {
		scoreI := m.addressScore(result[i])
		scoreJ := m.addressScore(result[j])
		return scoreI > scoreJ
	})

	return result
}

// addressScore 计算地址分数
func (m *AddressManager) addressScore(addr endpoint.Address) int {
	score := 0

	// 公网地址加分
	if addr.IsPublic() {
		score += 100
	}

	// 私有地址减分
	if addr.IsPrivate() {
		score -= 20
	}

	// 回环地址大减分
	if addr.IsLoopback() {
		score -= 100
	}

	// IPv6 偏好
	if m.config.PreferIPv6 && addr.Network() == "ip6" {
		score += 10
	}

	// IPv4 偏好（如果不偏好 IPv6）
	if !m.config.PreferIPv6 && addr.Network() == "ip4" {
		score += 10
	}

	// 检测地址类型
	addrType := DetectAddressType(addr)
	score += addrType.BasePriority()

	return score
}

// ============================================================================
//                              外部地址发现
// ============================================================================

// discoverExternalAddresses 发现外部地址
func (m *AddressManager) discoverExternalAddresses() {
	natService := m.getNATService()
	if natService == nil {
		return
	}

	// 获取 ctx（线程安全）
	m.mu.Lock()
	ctx := m.ctx
	m.mu.Unlock()

	if ctx == nil {
		return
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 获取外部地址
	addr, err := natService.GetExternalAddressWithContext(timeoutCtx)
	if err != nil {
		log.Debug("发现外部地址失败", "err", err)
		return
	}

	if addr != nil {
		// 可达性优先策略：不把"仅 IP（无端口）"的结果加入 AdvertisedAddrs
		// 真实可拨号的公网 multiaddr 由 Endpoint 在端口映射成功后负责发布。
		addrStr := addr.String()
		if strings.HasPrefix(addrStr, "/") {
			// multiaddr 形式（含协议/端口），允许加入
			m.AddAdvertisedAddr(addr)
			log.Info("发现外部地址", "addr", addrStr)
			return
		}

		// host:port 形式允许加入，否则跳过
		if _, _, splitErr := net.SplitHostPort(addrStr); splitErr == nil {
			m.AddAdvertisedAddr(addr)
			log.Info("发现外部地址", "addr", addrStr)
			return
		}

		log.Debug("跳过外部地址（无端口，只发布可验证可达地址）", "addr", addrStr)
	}
}

// refreshLoop 定期刷新地址
func (m *AddressManager) refreshLoop() {
	// 获取 ctx（线程安全）
	m.mu.Lock()
	ctx := m.ctx
	m.mu.Unlock()

	if ctx == nil {
		return
	}

	ticker := time.NewTicker(m.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.discoverExternalAddresses()
		}
	}
}

// ============================================================================
//                              地址过滤
// ============================================================================

// FilteredAdvertisedAddrs 返回过滤后的通告地址
//
// 根据配置过滤本地和私有地址。
func (m *AddressManager) FilteredAdvertisedAddrs() []endpoint.Address {
	m.advertisedAddrsMu.RLock()
	defer m.advertisedAddrsMu.RUnlock()

	var result []endpoint.Address
	for _, addr := range m.advertisedAddrs {
		// 过滤本地地址
		if m.config.FilterLocalAddrs && addr.IsLoopback() {
			continue
		}

		// 过滤私有地址
		if m.config.FilterPrivateAddrs && addr.IsPrivate() {
			continue
		}

		result = append(result, addr)
	}

	return result
}

// ============================================================================
//                              本地接口地址
// ============================================================================

// LocalInterfaceAddrs 获取本地网络接口地址
func (m *AddressManager) LocalInterfaceAddrs() []endpoint.Address {
	interfaces, err := net.Interfaces()
	if err != nil {
		log.Debug("获取网络接口失败", "err", err)
		return nil
	}

	var result []endpoint.Address
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

			ip := ipNet.IP
			if ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}

			result = append(result, &simpleAddr{
				network: getIPNetwork(ip),
				addr:    ip.String(),
			})
		}
	}

	return result
}

// getIPNetwork 获取 IP 网络类型
func getIPNetwork(ip net.IP) string {
	if ip.To4() != nil {
		return "ip4"
	}
	return "ip6"
}

// ============================================================================
//                              simpleAddr 实现
// ============================================================================

// simpleAddr 简单地址实现
type simpleAddr struct {
	network string
	addr    string
}

func (a *simpleAddr) Network() string {
	return a.network
}

func (a *simpleAddr) String() string {
	return a.addr
}

func (a *simpleAddr) Bytes() []byte {
	return []byte(a.addr)
}

func (a *simpleAddr) Equal(other endpoint.Address) bool {
	if other == nil {
		return false
	}
	return a.addr == other.String()
}

func (a *simpleAddr) IsPublic() bool {
	ip := net.ParseIP(a.addr)
	if ip == nil {
		return false
	}
	// 0.0.0.0 和 :: 是未指定地址，不是公网地址
	return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsUnspecified() && !ip.IsLinkLocalUnicast()
}

func (a *simpleAddr) IsPrivate() bool {
	ip := net.ParseIP(a.addr)
	if ip == nil {
		return false
	}
	return ip.IsPrivate()
}

func (a *simpleAddr) IsLoopback() bool {
	ip := net.ParseIP(a.addr)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

func (a *simpleAddr) Multiaddr() string {
	// simpleAddr 通常只存储 IP，需要根据 network 转换
	if strings.HasPrefix(a.addr, "/") {
		return a.addr
	}
	// 如果是纯 IP，构建 multiaddr
	ipType := "ip4"
	if ip := net.ParseIP(a.addr); ip != nil && ip.To4() == nil {
		ipType = "ip6"
	}
	return fmt.Sprintf("/%s/%s", ipType, a.addr)
}

