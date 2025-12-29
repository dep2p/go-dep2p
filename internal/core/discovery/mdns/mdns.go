// Package mdns 提供基于 mDNS 的本地网络节点发现
//
// mDNS (Multicast DNS) 用于在局域网内自动发现其他节点，
// 无需任何中央服务器或引导节点。
package mdns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/mdns"

	"github.com/dep2p/go-dep2p/internal/core/address"
	"github.com/dep2p/go-dep2p/internal/util/logger"
	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 包级别日志实例
var log = logger.Logger("discovery.mdns")

// ErrPortUnknown 表示 mDNS 服务端口尚不可确定（通常是因为还未收到 UpdateLocalAddrs）。
// 这不是致命错误：Discoverer 仍可作为客户端运行，待端口确定后再启动 server。
var ErrPortUnknown = errors.New("mdns port unknown")

// ============================================================================
//                              配置
// ============================================================================

// Config mDNS 发现器配置
type Config struct {
	// ServiceTag 服务标签（用于区分不同的 P2P 网络）
	ServiceTag string

	// Domain 域名
	Domain string

	// Port 本地监听端口
	Port int

	// TTL 服务记录 TTL
	TTL time.Duration

	// QueryInterval 查询间隔
	QueryInterval time.Duration

	// Interface 指定网络接口（空表示所有接口）
	Interface string

	// DisableIPv4 禁用 IPv4
	DisableIPv4 bool

	// DisableIPv6 禁用 IPv6
	DisableIPv6 bool
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		ServiceTag:    "_dep2p._udp",
		Domain:        "local.",
		Port:          0,
		TTL:           10 * time.Minute,
		QueryInterval: 1 * time.Minute,
		Interface:     "",
		DisableIPv4:   false,
		DisableIPv6:   true, // 默认禁用 IPv6 以避免问题
	}
}

// ============================================================================
//                              mDNS 发现器
// ============================================================================

// Discoverer mDNS 发现器
type Discoverer struct {
	config Config

	// 本地节点信息
	localID    types.NodeID
	localAddrs []string

	// mDNS 服务
	server *mdns.Server

	// 已发现的节点
	peers   map[types.NodeID]peerEntry
	peersMu sync.RWMutex

	// 节点发现回调
	onPeerDiscovered func(discoveryif.PeerInfo)
	callbackMu       sync.RWMutex

	// 运行状态
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.RWMutex
}

// peerEntry 节点条目
// 注意：内部使用 []string 存储（mDNS 解析的原始格式）
// 在返回 PeerInfo 时转换为 []Multiaddr
type peerEntry struct {
	ID       types.NodeID
	Addrs    []string
	LastSeen time.Time
}

// NewDiscoverer 创建 mDNS 发现器
func NewDiscoverer(config Config, localID types.NodeID, localAddrs []string) *Discoverer {
	return &Discoverer{
		config:     config,
		localID:    localID,
		localAddrs: localAddrs,
		peers:      make(map[types.NodeID]peerEntry),
	}
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动 mDNS 服务
func (d *Discoverer) Start(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.running {
		return nil
	}

	d.ctx, d.cancel = context.WithCancel(ctx)

	// 创建 mDNS 服务
	serverStarted := false
	if err := d.startServer(); err != nil {
		// 端口尚未可确定是预期状态：通常会在 endpoint.Listen() 后触发 UpdateLocalAddrs，
		// 继而重启并以 server_mode=true 运行。
		if errors.Is(err, ErrPortUnknown) {
			log.Debug("mDNS 服务端口尚不可确定（将仅作为客户端运行，等待 UpdateLocalAddrs）",
				"config_port", d.config.Port,
				"local_addrs", d.localAddrs)
		} else {
			log.Warn("启动 mDNS 服务失败（将仅作为客户端运行）",
				"err", err,
				"config_port", d.config.Port,
				"local_addrs", d.localAddrs,
				"hint", "如果端口为 0，请确保已调用 UpdateLocalAddrs 或设置 config.Port")
		}
	} else {
		serverStarted = true
	}

	// 启动查询循环
	go d.queryLoop()

	// 启动清理循环
	go d.cleanupLoop()

	d.running = true
	log.Info("mDNS 发现器已启动",
		"service", d.config.ServiceTag,
		"server_mode", serverStarted,
		"client_mode", true)

	return nil
}

// Stop 停止 mDNS 服务
func (d *Discoverer) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.running {
		return nil
	}

	if d.cancel != nil {
		d.cancel()
	}

	if d.server != nil {
		_ = d.server.Shutdown() // mDNS 服务器关闭错误可忽略
		d.server = nil
	}

	d.running = false
	log.Info("mDNS 发现器已停止")
	return nil
}

// Close 关闭发现器
func (d *Discoverer) Close() error {
	return d.Stop()
}

// ============================================================================
//                              mDNS 服务
// ============================================================================

// startServer 启动 mDNS 服务器
func (d *Discoverer) startServer() error {
	// 获取本地 IP
	ips, err := d.getLocalIPs()
	if err != nil {
		return fmt.Errorf("获取本地 IP 失败: %w", err)
	}

	if len(ips) == 0 {
		return fmt.Errorf("未找到本地 IP 地址")
	}

	// 确定服务端口
	port := d.config.Port
	if port == 0 {
		// 尝试从 localAddrs 推断端口
		if inferredPort := inferPort(d.localAddrs); inferredPort > 0 {
			port = inferredPort
			log.Info("从地址推断 mDNS 服务端口",
				"port", port,
				"addrs", d.localAddrs)
		} else {
			return ErrPortUnknown
		}
	}

	// 创建服务信息
	// TXT 记录格式: nodeID=xxx,addrs=addr1,addr2,...
	txt := buildTXTRecords(d.localID.String(), filterDialableAddrs(d.localAddrs))

	// 服务实例名使用 NodeID 前缀（注意：这里是 instance name，不应包含 service tag/domain）
	// hashicorp/mdns 会自行拼接为: <instance>.<service>.<domain>
	serviceName := fmt.Sprintf("dep2p-%s", d.localID.ShortString())

	ipStrings := make([]string, len(ips))
	for i, ip := range ips {
		ipStrings[i] = ip.String()
	}
	log.Info("启动 mDNS 服务器",
		"instance", serviceName,
		"service", d.config.ServiceTag,
		"domain", d.config.Domain,
		"port", port,
		"ips", ipStrings,
		"txt", txt)

	// 创建服务配置
	service, err := mdns.NewMDNSService(
		serviceName,
		d.config.ServiceTag,
		d.config.Domain,
		"",
		port,
		ips,
		txt,
	)
	if err != nil {
		return fmt.Errorf("创建 mDNS 服务失败: %w", err)
	}

	// 创建服务器配置
	serverConfig := &mdns.Config{
		Zone: service,
	}

	// 指定网络接口
	if d.config.Interface != "" {
		iface, err := net.InterfaceByName(d.config.Interface)
		if err != nil {
			log.Warn("找不到指定接口",
				"interface", d.config.Interface,
				"err", err)
		} else {
			serverConfig.Iface = iface
			log.Debug("使用指定网络接口", "interface", d.config.Interface)
		}
	}

	// 创建服务器
	server, err := mdns.NewServer(serverConfig)
	if err != nil {
		return fmt.Errorf("创建 mDNS 服务器失败: %w", err)
	}

	d.server = server
	ipStrings2 := make([]string, len(ips))
	for i, ip := range ips {
		ipStrings2[i] = ip.String()
	}
	log.Info("mDNS 服务器已成功启动",
		"instance", serviceName,
		"port", port,
		"ips", ipStrings2)

	return nil
}

// buildTXTRecords 构建满足 DNS TXT 单条 <=255 字节限制的记录。
//
// 约束：
// - 单条 TXT record 最大 255 字节（RFC 1035 / miekg/dns 实现约束）
// - hashicorp/mdns 使用 []string 表示多条 TXT
//
// 语义：
// - 始终包含 "id=<nodeID>"
// - 地址使用多个 "addrs=" 分片发布（同 key 可重复），消费端会聚合
func buildTXTRecords(nodeID string, addrs []string) []string {
	const maxLen = 255

	txt := []string{fmt.Sprintf("id=%s", nodeID)}
	if len(addrs) == 0 {
		return txt
	}

	const prefix = "addrs="
	cur := prefix
	flush := func() {
		if cur != prefix {
			txt = append(txt, cur)
		}
		cur = prefix
	}

	for _, a := range addrs {
		if a == "" {
			continue
		}
		// 计算加入该 addr 后的长度（含逗号）
		next := a
		if cur != prefix {
			next = "," + a
		}
		if len(cur)+len(next) > maxLen {
			flush()
		}
		// 极端情况下单个地址本身就超过 255，直接丢弃（避免 mdns 报错）
		if len(prefix)+len(a) > maxLen {
			continue
		}
		if cur != prefix {
			cur += ","
		}
		cur += a
	}
	flush()
	return txt
}

// getLocalIPs 获取本地 IP 地址
// 此函数会：
// 1. 过滤虚拟网卡（VPN、容器、虚拟机等）
// 2. 只选择有效的局域网 IP（RFC1918 私网地址）
// 3. 按优先级排序（192.168.x > 10.x > 172.16-31.x）
func (d *Discoverer) getLocalIPs() ([]net.IP, error) {
	type scoredIP struct {
		ip    net.IP
		score int
	}
	var scored []scoredIP

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		// 跳过回环和非活动接口
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		// 如果指定了接口，只使用该接口
		if d.config.Interface != "" && iface.Name != d.config.Interface {
			continue
		}

		// 跳过虚拟网卡（VPN、容器、虚拟机等）
		// 虚拟网卡的 IP 地址跨机通常不可达
		if isVirtualInterface(iface.Name) {
			log.Debug("跳过虚拟网卡",
				"interface", iface.Name)
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

			// 跳过回环和链路本地地址
			if ip.IsLoopback() || ip.IsLinkLocalUnicast() {
				continue
			}

			// 根据配置过滤 IPv4/IPv6
			isIPv4 := ip.To4() != nil
			if isIPv4 && d.config.DisableIPv4 {
				continue
			}
			if !isIPv4 && d.config.DisableIPv6 {
				continue
			}

			// 计算优先级分数（只接受局域网 IP）
			score := scoreLANIP(ip)
			if score == 0 {
				log.Debug("跳过非局域网 IP",
					"interface", iface.Name,
					"ip", ip.String())
				continue
			}

			scored = append(scored, scoredIP{ip: ip, score: score})
		}
	}

	// 按分数降序排序
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	// 提取排序后的 IP 列表
	ips := make([]net.IP, len(scored))
	for i, s := range scored {
		ips[i] = s.ip
	}

	return ips, nil
}

// ============================================================================
//                              查询循环
// ============================================================================

// queryLoop 查询循环
func (d *Discoverer) queryLoop() {
	// 检查 ctx 是否为 nil（防止 Start() 未调用）
	if d.ctx == nil {
		return
	}

	// 首次立即查询
	d.runQuery()

	ticker := time.NewTicker(d.config.QueryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.runQuery()
		}
	}
}

// runQuery 执行一次 mDNS 查询
func (d *Discoverer) runQuery() {
	// 创建查询参数
	params := &mdns.QueryParam{
		Service:             d.config.ServiceTag,
		Domain:              d.config.Domain,
		Timeout:             10 * time.Second,
		DisableIPv4:         d.config.DisableIPv4,
		DisableIPv6:         d.config.DisableIPv6,
		WantUnicastResponse: true,
	}

	// 指定接口
	if d.config.Interface != "" {
		iface, err := net.InterfaceByName(d.config.Interface)
		if err == nil {
			params.Interface = iface
		}
	}

	// 创建结果通道
	entries := make(chan *mdns.ServiceEntry, 10)
	// 关键：把结果通道传给 mdns.Query，否则我们永远收不到发现结果
	params.Entries = entries

	// 处理结果
	go func() {
		for entry := range entries {
			d.handleEntry(entry)
		}
	}()

	// 执行查询
	if err := mdns.Query(params); err != nil {
		log.Debug("mDNS 查询失败", "err", err)
	}

	close(entries)
}

// handleEntry 处理发现的服务条目
func (d *Discoverer) handleEntry(entry *mdns.ServiceEntry) {
	if entry == nil {
		return
	}

	// 解析 TXT 记录
	var nodeID types.NodeID
	var addrs []string

	for _, txt := range entry.InfoFields {
		if strings.HasPrefix(txt, "id=") {
			idStr := strings.TrimPrefix(txt, "id=")
			// NodeID 的规范外部表示是 Base58，因此这里必须用 ParseNodeID。
			id, err := types.ParseNodeID(idStr)
			if err != nil {
				log.Debug("解析 NodeID 失败",
					"id", idStr,
					"err", err)
				continue
			}
			nodeID = id
		} else if strings.HasPrefix(txt, "addrs=") {
			addrsStr := strings.TrimPrefix(txt, "addrs=")
			if addrsStr != "" {
				part := strings.Split(addrsStr, ",")
				// 丢弃不可拨号的 TXT 地址，确保必要时能回退到 A/AAAA + entry.Port
				part = filterDialableAddrs(part)
				addrs = append(addrs, part...)
			}
		}
	}
	addrs = dedupeStrings(addrs)

	// 如果没有从 TXT 解析到地址，使用 A/AAAA 记录
	// 注意：需要检查地址是否为有效的局域网 IP，避免使用 VPN/隧道地址
	if len(addrs) == 0 {
		if entry.AddrV4 != nil && isLANIP(entry.AddrV4) {
			addrs = append(addrs, fmt.Sprintf("%s:%d", entry.AddrV4, entry.Port))
		} else if entry.AddrV4 != nil {
			log.Debug("跳过非局域网 AddrV4",
				"ip", entry.AddrV4.String(),
				"isNonRoutable", isNonRoutableIP(entry.AddrV4))
		}
		if entry.AddrV6 != nil && isLANIP(entry.AddrV6) {
			addrs = append(addrs, fmt.Sprintf("[%s]:%d", entry.AddrV6, entry.Port))
		} else if entry.AddrV6 != nil {
			log.Debug("跳过非局域网 AddrV6",
				"ip", entry.AddrV6.String(),
				"isNonRoutable", isNonRoutableIP(entry.AddrV6))
		}
	}

	// 对地址按可达性排序（局域网地址优先）
	addrs = sortAddrsByReachability(addrs)

	// 跳过自己
	if nodeID == d.localID {
		return
	}

	// 跳过无效条目
	if nodeID.IsEmpty() || len(addrs) == 0 {
		return
	}

	// 检查是否是新发现的节点
	d.peersMu.RLock()
	_, exists := d.peers[nodeID]
	d.peersMu.RUnlock()

	// 存储发现的节点
	d.peersMu.Lock()
	d.peers[nodeID] = peerEntry{
		ID:       nodeID,
		Addrs:    addrs,
		LastSeen: time.Now(),
	}
	d.peersMu.Unlock()

	log.Debug("mDNS 发现节点",
		"peer", nodeID.ShortString(),
		"addrs", addrs,
		"new", !exists)

	// 如果是新节点，触发回调
	if !exists {
		d.callbackMu.RLock()
		callback := d.onPeerDiscovered
		d.callbackMu.RUnlock()

		if callback != nil {
			peerInfo := discoveryif.PeerInfo{
				ID:    nodeID,
				Addrs: types.StringsToMultiaddrs(addrs),
			}
			// 在独立的 goroutine 中调用回调，避免阻塞 mDNS 处理
			go func() {
				defer func() {
					if r := recover(); r != nil {
						log.Error("mDNS 节点发现回调 panic", "recover", r)
					}
				}()
				callback(peerInfo)
			}()
		}
	}
}

func dedupeStrings(in []string) []string {
	if len(in) <= 1 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// cleanupLoop 清理循环
func (d *Discoverer) cleanupLoop() {
	// 检查 ctx 是否为 nil（防止 Start() 未调用）
	if d.ctx == nil {
		return
	}

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.cleanup()
		}
	}
}

// cleanup 清理过期节点
func (d *Discoverer) cleanup() {
	d.peersMu.Lock()
	defer d.peersMu.Unlock()

	cutoff := time.Now().Add(-d.config.TTL)
	for id, entry := range d.peers {
		if entry.LastSeen.Before(cutoff) {
			delete(d.peers, id)
			log.Debug("移除过期 mDNS 节点",
				"peer", id.ShortString())
		}
	}
}

// ============================================================================
//                              Discoverer 接口实现
// ============================================================================

// FindPeer 查找指定节点
func (d *Discoverer) FindPeer(_ context.Context, id types.NodeID) ([]endpoint.Address, error) {
	d.peersMu.RLock()
	entry, ok := d.peers[id]
	d.peersMu.RUnlock()

	if !ok {
		return nil, nil
	}

	addrs := make([]endpoint.Address, len(entry.Addrs))
	for i, addr := range entry.Addrs {
		addrs[i] = address.NewAddr(types.Multiaddr(addr))
	}

	return addrs, nil
}

// FindPeers 批量查找节点
func (d *Discoverer) FindPeers(ctx context.Context, ids []types.NodeID) (map[types.NodeID][]endpoint.Address, error) {
	result := make(map[types.NodeID][]endpoint.Address)

	for _, id := range ids {
		addrs, _ := d.FindPeer(ctx, id)
		if len(addrs) > 0 {
			result[id] = addrs
		}
	}

	return result, nil
}

// FindClosestPeers 查找最近的节点（mDNS 不支持按距离查找）
func (d *Discoverer) FindClosestPeers(_ context.Context, _ []byte, count int) ([]types.NodeID, error) {
	d.peersMu.RLock()
	defer d.peersMu.RUnlock()

	result := make([]types.NodeID, 0, count)
	for id := range d.peers {
		result = append(result, id)
		if len(result) >= count {
			break
		}
	}

	return result, nil
}

// DiscoverPeers 发现节点
func (d *Discoverer) DiscoverPeers(ctx context.Context, _ string) (<-chan discoveryif.PeerInfo, error) {
	ch := make(chan discoveryif.PeerInfo, 10)

	go func() {
		defer close(ch)

		d.peersMu.RLock()
		peers := make([]peerEntry, 0, len(d.peers))
		for _, entry := range d.peers {
			peers = append(peers, entry)
		}
		d.peersMu.RUnlock()

		for _, entry := range peers {
			select {
			case <-ctx.Done():
				return
			case ch <- discoveryif.PeerInfo{
				ID:    entry.ID,
				Addrs: types.StringsToMultiaddrs(entry.Addrs),
			}:
			}
		}
	}()

	return ch, nil
}

// ============================================================================
//                              MDNS 接口实现
// ============================================================================

// Peers 返回发现的本地节点
func (d *Discoverer) Peers() []discoveryif.PeerInfo {
	d.peersMu.RLock()
	defer d.peersMu.RUnlock()

	result := make([]discoveryif.PeerInfo, 0, len(d.peers))
	for _, entry := range d.peers {
		result = append(result, discoveryif.PeerInfo{
			ID:    entry.ID,
			Addrs: types.StringsToMultiaddrs(entry.Addrs),
		})
	}

	return result
}

// ============================================================================
//                              状态查询
// ============================================================================

// PeerCount 返回发现的节点数量
func (d *Discoverer) PeerCount() int {
	d.peersMu.RLock()
	defer d.peersMu.RUnlock()
	return len(d.peers)
}

// IsRunning 返回是否正在运行
func (d *Discoverer) IsRunning() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.running
}

// ============================================================================
//                              地址更新
// ============================================================================

// UpdateLocalAddrs 更新本地地址
func (d *Discoverer) UpdateLocalAddrs(addrs []string) {
	d.mu.Lock()

	// 推断服务端口（先从原始地址推断，包括 0.0.0.0:port）
	oldPort := d.config.Port
	if d.config.Port == 0 {
		if p := inferPort(addrs); p > 0 {
			d.config.Port = p
			log.Info("从地址推断 mDNS 服务端口",
				"port", p,
				"addrs", addrs)
		}
	}

	// 过滤不可拨号地址
	filtered := filterDialableAddrs(addrs)

	// 如果过滤后为空（通常是因为传入的是 0.0.0.0:port），
	// 则用 getLocalIPs() 获取的真实局域网 IP 来构造可拨号地址
	if len(filtered) == 0 && d.config.Port > 0 {
		localIPs, err := d.getLocalIPs()
		if err == nil && len(localIPs) > 0 {
			for _, ip := range localIPs {
				if ip.To4() != nil {
					filtered = append(filtered, fmt.Sprintf("%s:%d", ip.String(), d.config.Port))
				} else {
					filtered = append(filtered, fmt.Sprintf("[%s]:%d", ip.String(), d.config.Port))
				}
			}
			log.Info("使用本地 IP 替换未指定地址",
				"original", addrs,
				"replaced", filtered)
		}
	}

	d.localAddrs = filtered
	wasRunning := d.running
	d.mu.Unlock()

	log.Info("更新 mDNS 本地地址",
		"input_count", len(addrs),
		"filtered_count", len(filtered),
		"port", d.config.Port,
		"port_changed", oldPort != d.config.Port,
		"filtered_addrs", filtered)

	// 重启服务以应用新地址（如果正在运行）
	if wasRunning {
		log.Debug("重启 mDNS 服务以应用新地址")
		_ = d.Stop()                   // 停止时错误可忽略
		_ = d.Start(context.Background()) // 重启失败在下次刷新时会重试
	}
}

// SetOnPeerDiscovered 设置节点发现回调
//
// 当发现新节点时会调用此回调。
func (d *Discoverer) SetOnPeerDiscovered(callback func(discoveryif.PeerInfo)) {
	d.callbackMu.Lock()
	defer d.callbackMu.Unlock()
	d.onPeerDiscovered = callback
}

// filterDialableAddrs 过滤不可拨号地址（如 0.0.0.0/::/127.0.0.1，以及端口为 0 的地址）。
// 目前支持:
// - host:port (如 "192.168.1.1:4001")
// - [ip6]:port (如 "[2001::1]:4001")
// - multiaddr (如 "/ip4/192.168.1.1/udp/4001/quic-v1")
func filterDialableAddrs(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		
		// multiaddr 格式: /ip4/... 或 /ip6/...
		if strings.HasPrefix(s, "/ip4/") || strings.HasPrefix(s, "/ip6/") {
			// 过滤掉 0.0.0.0 和 ::
			if strings.Contains(s, "/ip4/0.0.0.0") || strings.Contains(s, "/ip6/::") {
				continue
			}
			// mDNS 仅用于局域网发现：丢弃公网 IP（避免 VPN/出口 IP 被广播后拨号超时）
			if ip := extractIPFromMultiaddr(s); ip != nil && !isLANIP(ip) {
				continue
			}
			// 检查是否有有效端口
			port := inferPort([]string{s})
			if port <= 0 {
				continue
			}
			out = append(out, s)
			continue
		}
		
		// host:port 格式
		host, portStr, err := net.SplitHostPort(s)
		if err == nil {
			p, _ := strconv.Atoi(portStr)
			if p <= 0 {
				continue
			}
			h := strings.Trim(host, "[]")
			if h == "0.0.0.0" || h == "::" || h == "127.0.0.1" || h == "localhost" {
				continue
			}
			// 如果是 IP，则只接受局域网地址
			if ip := net.ParseIP(h); ip != nil && !isLANIP(ip) {
				continue
			}
			out = append(out, s)
			continue
		}
		
		// 未知格式 - 尝试推断端口，如果有端口则保留
		if inferPort([]string{s}) > 0 {
			out = append(out, s)
		}
	}
	return out
}

// extractIPFromMultiaddr 从形如 /ip4/x.x.x.x/... 或 /ip6/xxxx/... 的字符串中提取 IP
func extractIPFromMultiaddr(s string) net.IP {
	if strings.HasPrefix(s, "/ip4/") {
		rest := strings.TrimPrefix(s, "/ip4/")
		ipStr := rest
		if idx := strings.IndexByte(rest, '/'); idx >= 0 {
			ipStr = rest[:idx]
		}
		return net.ParseIP(ipStr)
	}
	if strings.HasPrefix(s, "/ip6/") {
		rest := strings.TrimPrefix(s, "/ip6/")
		ipStr := rest
		if idx := strings.IndexByte(rest, '/'); idx >= 0 {
			ipStr = rest[:idx]
		}
		return net.ParseIP(ipStr)
	}
	return nil
}

// isLANIP 判断是否为局域网可达 IP（RFC1918/ULA/link-local），用于 mDNS 场景过滤公网地址。
// 注意：此函数会排除已知的 VPN/隧道地址段，即使它们在技术上是"私网"地址。
func isLANIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() || ip.IsUnspecified() {
		return false
	}
	// 排除已知的 VPN/隧道/CGNAT 地址段（这些地址跨机不可达）
	if isNonRoutableIP(ip) {
		return false
	}
	// Go: IsPrivate 覆盖 RFC1918 + IPv6 ULA（fc00::/7）
	if ip.IsPrivate() {
		return true
	}
	// 允许链路本地（某些局域网/Bonjour 场景）
	if ip.IsLinkLocalUnicast() {
		return true
	}
	return false
}

// ============================================================================
//                              网卡和地址过滤
// ============================================================================

// virtualInterfacePrefixes 虚拟网卡前缀列表
// 这些网卡通常用于 VPN、容器、虚拟机等，其 IP 地址跨机不可达
var virtualInterfacePrefixes = []string{
	// macOS/iOS VPN 隧道
	"utun",   // macOS TUN 接口（常用于 VPN）
	"ipsec",  // IPSec 隧道
	// macOS 特殊接口
	"awdl",   // Apple Wireless Direct Link (AirDrop)
	"llw",    // Low Latency WLAN（AirDrop 相关）
	"ap",     // Access Point (热点)
	"bridge", // macOS 桥接接口
	// Linux/Docker/Kubernetes
	"docker", // Docker 默认桥接
	"br-",    // Docker 自定义桥接
	"veth",   // 虚拟以太网（容器）
	"virbr",  // libvirt 虚拟桥接
	"vboxnet", // VirtualBox
	"vmnet",  // VMware
	// 通用虚拟接口
	"tun",    // TUN 设备
	"tap",    // TAP 设备
	"vlan",   // VLAN 接口
	"bond",   // 网卡绑定
	"dummy",  // 虚拟测试接口
	// Tailscale/WireGuard
	"tailscale", // Tailscale VPN
	"wg",     // WireGuard
}

// isVirtualInterface 判断网卡是否为虚拟网卡（VPN、容器、虚拟机等）
// 虚拟网卡的 IP 地址通常在局域网跨机不可达，不适合用于 mDNS 广播
func isVirtualInterface(name string) bool {
	nameLower := strings.ToLower(name)
	for _, prefix := range virtualInterfacePrefixes {
		if strings.HasPrefix(nameLower, prefix) {
			return true
		}
	}
	return false
}

// nonRoutableCIDRs 不可路由的地址段（VPN、CGNAT、测试网段等）
// 这些地址段在局域网跨机通常不可达
var nonRoutableCIDRs = []string{
	// VPN/隧道专用地址段
	"198.18.0.0/15",   // RFC 2544 基准测试网段，常被 VPN 使用（如 Surge/Clash）
	"198.51.100.0/24", // RFC 5737 文档示例
	"203.0.113.0/24",  // RFC 5737 文档示例
	// CGNAT（运营商级 NAT）
	"100.64.0.0/10",   // RFC 6598 CGNAT 共享地址空间
	// Tailscale/CGNAT 100.x.x.x
	// 注：100.64.0.0/10 已覆盖 100.64-127.x.x，Tailscale 用 100.x.x.x 全段
}

// parsedNonRoutableCIDRs 预解析的不可路由网段
var parsedNonRoutableCIDRs []*net.IPNet

func init() {
	for _, cidr := range nonRoutableCIDRs {
		_, ipNet, err := net.ParseCIDR(cidr)
		if err == nil {
			parsedNonRoutableCIDRs = append(parsedNonRoutableCIDRs, ipNet)
		}
	}
}

// isNonRoutableIP 判断 IP 是否属于不可路由的 VPN/隧道/测试网段
func isNonRoutableIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, ipNet := range parsedNonRoutableCIDRs {
		if ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

// scoreLANIP 对局域网 IP 评分，用于优先级排序
// 分数越高优先级越高
// 返回 0 表示不是有效的局域网 IP
func scoreLANIP(ip net.IP) int {
	if ip == nil {
		return 0
	}
	if ip.IsLoopback() || ip.IsUnspecified() {
		return 0
	}
	if isNonRoutableIP(ip) {
		return 0
	}

	// IPv4 优先于 IPv6（因为 IPv6 在很多局域网环境兼容性差）
	isIPv4 := ip.To4() != nil
	baseScore := 0
	if isIPv4 {
		baseScore = 1000
	} else {
		baseScore = 100
	}

	// RFC1918 私网地址优先级：192.168.x.x > 10.x.x.x > 172.16-31.x.x
	if ip.IsPrivate() {
		if isIPv4 {
			ip4 := ip.To4()
			if ip4[0] == 192 && ip4[1] == 168 {
				return baseScore + 300 // 192.168.x.x 最常见的家庭/小型办公网络
			}
			if ip4[0] == 10 {
				return baseScore + 200 // 10.x.x.x 企业/大型网络
			}
			if ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31 {
				return baseScore + 100 // 172.16-31.x.x 较少使用
			}
		}
		return baseScore + 50 // 其他私网（如 IPv6 ULA）
	}

	// 链路本地地址优先级最低（169.254.x.x / fe80::）
	if ip.IsLinkLocalUnicast() {
		return baseScore + 10
	}

	return 0 // 公网地址不适合 mDNS
}

// sortAddrsByReachability 对地址列表按可达性排序
// 局域网地址优先，公网地址最后
// 支持 host:port 和 multiaddr 格式
func sortAddrsByReachability(addrs []string) []string {
	if len(addrs) <= 1 {
		return addrs
	}

	type scoredAddr struct {
		addr  string
		score int
	}

	scored := make([]scoredAddr, 0, len(addrs))
	for _, addr := range addrs {
		score := scoreAddrString(addr)
		scored = append(scored, scoredAddr{addr: addr, score: score})
	}

	// 按分数降序排序
	for i := 0; i < len(scored)-1; i++ {
		for j := i + 1; j < len(scored); j++ {
			if scored[j].score > scored[i].score {
				scored[i], scored[j] = scored[j], scored[i]
			}
		}
	}

	result := make([]string, len(scored))
	for i, s := range scored {
		result[i] = s.addr
	}
	return result
}

// scoreAddrString 对地址字符串评分
// 支持 host:port 和 multiaddr 格式
func scoreAddrString(addr string) int {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return 0
	}

	var ip net.IP

	// multiaddr 格式: /ip4/... 或 /ip6/...
	if strings.HasPrefix(addr, "/ip4/") || strings.HasPrefix(addr, "/ip6/") {
		ip = extractIPFromMultiaddr(addr)
	} else {
		// host:port 格式
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return 0
		}
		host = strings.Trim(host, "[]")
		ip = net.ParseIP(host)
	}

	if ip == nil {
		return 0
	}

	return scoreLANIP(ip)
}

// inferPort 从地址列表中推断一个服务端口（优先解析 host:port，其次尝试解析 multiaddr 的 /tcp/<p> 或 /udp/<p>）。
func inferPort(addrs []string) int {
	for _, s := range addrs {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, portStr, err := net.SplitHostPort(s); err == nil {
			if p, err := strconv.Atoi(portStr); err == nil && p > 0 {
				return p
			}
		}
		// multiaddr: .../tcp/<p>/... or .../udp/<p>/...
		if idx := strings.Index(s, "/tcp/"); idx >= 0 {
			rest := s[idx+len("/tcp/"):]
			if p := readLeadingInt(rest); p > 0 {
				return p
			}
		}
		if idx := strings.Index(s, "/udp/"); idx >= 0 {
			rest := s[idx+len("/udp/"):]
			if p := readLeadingInt(rest); p > 0 {
				return p
			}
		}
	}
	return 0
}

func readLeadingInt(s string) int {
	n := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// ============================================================================
//                              接口断言
// ============================================================================

var _ discoveryif.Discoverer = (*Discoverer)(nil)
var _ discoveryif.MDNS = (*Discoverer)(nil)
var _ discoveryif.AddressUpdater = (*Discoverer)(nil)

