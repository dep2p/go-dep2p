package mdns

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/lib/zeroconf"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// mDNS 服务状态
const (
	StateWaiting int32 = iota // 等待地址（无有效地址时）
	StateRunning              // 正常运行（广播中）
	StateStopped              // 已停止
)

// MDNS 多播发现服务
type MDNS struct {
	ctx    context.Context
	cancel context.CancelFunc
	host   pkgif.Host
	config *Config

	server   *zeroconf.Server // mDNS 服务器
	peerName string           // 随机服务实例名

	started atomic.Bool
	closed  atomic.Bool
	state   atomic.Int32 // 服务状态: StateWaiting, StateRunning, StateStopped
	mu      sync.RWMutex
	wg      sync.WaitGroup
}

// New 创建 MDNS 服务
func New(host pkgif.Host, config *Config) (*MDNS, error) {
	if host == nil {
		return nil, ErrNilHost
	}

	if config == nil {
		config = DefaultConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	m := &MDNS{
		ctx:      ctx,
		cancel:   cancel,
		host:     host,
		config:   config,
		peerName: randomString(32 + rand.Intn(32)), //nolint:gosec // G404: peer name 不需要加密级随机
	}

	return m, nil
}

// FindPeers 发现节点
func (m *MDNS) FindPeers(ctx context.Context, ns string, _ ...pkgif.DiscoveryOption) (<-chan types.PeerInfo, error) {
	if m.closed.Load() {
		return nil, ErrAlreadyClosed
	}

	ns = pkgif.NormalizeNamespace(ns)

	peerCh := make(chan types.PeerInfo, 100)

	// 启动 Resolver
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		done, notifee, err := m.startResolver(ctx, peerCh)
		if err != nil {
			// Resolver 启动失败，channel 将被关闭
			close(peerCh)
			return
		}

		// 等待 ctx 取消
		<-ctx.Done()

		// 先标记 notifee 为关闭状态，防止新的发送
		if notifee != nil {
			notifee.Close()
		}

		// 等待 startResolver 内部所有 goroutines 完成
		// done channel 在 wg.Wait() 后关闭
		<-done

		// 现在可以安全关闭 peerCh
		close(peerCh)
	}()

	return peerCh, nil
}

// Advertise 广播自身
func (m *MDNS) Advertise(_ context.Context, ns string, opts ...pkgif.DiscoveryOption) (time.Duration, error) {
	if m.closed.Load() {
		return 0, ErrAlreadyClosed
	}

	ns = pkgif.NormalizeNamespace(ns)

	if err := m.startServer(); err != nil {
		return 0, fmt.Errorf("%w: %v", ErrServerStart, err)
	}

	// 返回广播 TTL（使用 Interval 作为 TTL）
	return m.config.Interval, nil
}

// Start 启动服务（Server + Resolver）
func (m *MDNS) Start(_ context.Context) error {
	if m.closed.Load() {
		return ErrAlreadyClosed
	}

	if m.started.Swap(true) {
		return ErrAlreadyStarted
	}

	logger.Info("mDNS 服务启动中",
		"serviceTag", m.config.ServiceTag,
		"interval", m.config.Interval)

	// 启动 Server
	if err := m.startServer(); err != nil {
		m.started.Store(false)
		logger.Warn("mDNS 服务启动失败", "error", err)
		return fmt.Errorf("%w: %v", ErrServerStart, err)
	}

	state := m.state.Load()
	stateStr := "unknown"
	switch state {
	case StateWaiting:
		stateStr = "waiting_for_addr"
	case StateRunning:
		stateStr = "running"
	case StateStopped:
		stateStr = "stopped"
	}

	logger.Info("mDNS 服务启动完成",
		"state", stateStr,
		"peerName", m.peerName[:8])

	return nil
}

// Stop 停止服务
func (m *MDNS) Stop(_ context.Context) error {
	if m.closed.Swap(true) {
		// 已经关闭，幂等操作
		return nil
	}

	// 设置状态为已停止
	m.state.Store(StateStopped)

	// 取消上下文
	m.cancel()

	// 关闭 Server
	m.mu.Lock()
	if m.server != nil {
		m.server.Shutdown()
		m.server = nil
	}
	m.mu.Unlock()

	// 等待所有 goroutine 结束（带超时）
	// zeroconf Browse 在 context 取消后可能需要数秒才能退出
	// 使用 2 秒超时，避免阻塞整个关闭流程
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Debug("mDNS 所有 goroutine 已正常结束")
	case <-time.After(2 * time.Second):
		logger.Debug("mDNS 关闭超时，goroutine 将在后台继续清理")
	}

	return nil
}

// stopServer 停止 mDNS 服务器（当地址丢失时调用）
func (m *MDNS) stopServer() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.server != nil {
		m.server.Shutdown()
		m.server = nil
	}

	// 如果服务没有完全停止，进入等待状态
	if m.state.Load() != StateStopped {
		m.state.Store(StateWaiting)
	}
}

// TryStartServer 尝试启动服务器（当地址变化时调用）
// 如果处于等待状态且有有效地址，则启动服务器
func (m *MDNS) TryStartServer() error {
	if m.state.Load() != StateWaiting {
		return nil // 不在等待状态，无需操作
	}
	return m.startServer()
}

// Started 返回是否已启动
func (m *MDNS) Started() bool {
	return m.started.Load()
}

// Closed 返回是否已关闭
func (m *MDNS) Closed() bool {
	return m.closed.Load()
}

// State 返回当前服务状态
func (m *MDNS) State() int32 {
	return m.state.Load()
}

// IsWaiting 返回是否处于等待地址状态
func (m *MDNS) IsWaiting() bool {
	return m.state.Load() == StateWaiting
}

// IsRunning 返回是否正在运行
func (m *MDNS) IsRunning() bool {
	return m.state.Load() == StateRunning
}

// mDNS 模块 logger
var logger = log.Logger("discovery/mdns")

// startServer 启动 mDNS 服务器（广播）
// 优雅降级：如果没有有效地址，进入 Waiting 状态而非返回错误
func (m *MDNS) startServer() error {
	logger.Debug("startServer 被调用")

	m.mu.Lock()
	defer m.mu.Unlock()

	// 如果已经启动，直接返回
	if m.server != nil {
		logger.Debug("服务器已启动，跳过")
		return nil
	}

	// 如果已停止，不再启动
	if m.state.Load() == StateStopped {
		logger.Debug("已停止状态，返回错误")
		return ErrAlreadyClosed
	}

	// 获取 Host 地址
	hostAddrs := m.host.Addrs()
	logger.Debug("Host 地址", "count", len(hostAddrs), "addrs", hostAddrs)

	if len(hostAddrs) == 0 {
		// 优雅降级：无地址时进入等待状态，而非返回错误
		logger.Debug("无地址，进入等待状态")
		m.state.Store(StateWaiting)
		return nil
	}

	// 展开 0.0.0.0 为实际的网络接口地址
	expandedAddrs := m.expandWildcardAddrs(hostAddrs)
	if len(expandedAddrs) == 0 {
		m.state.Store(StateWaiting)
		return nil
	}

	// 转换为 Multiaddr（预分配）
	multiaddrs := make([]types.Multiaddr, 0, len(expandedAddrs))
	for _, addrStr := range expandedAddrs {
		addr, err := types.NewMultiaddr(addrStr)
		if err != nil {
			continue
		}
		multiaddrs = append(multiaddrs, addr)
	}

	// 构建完整的 p2p multiaddr
	// 注意：Host.Addrs() 返回的地址可能已经包含 /p2p/<peerID>，需要检查
	hostID := m.host.ID()
	var p2pAddrs []types.Multiaddr
	for _, addr := range multiaddrs {
		// 检查是否已经包含 /p2p/ 后缀
		if _, err := addr.ValueForProtocol(types.P_P2P); err == nil {
			// 已包含 /p2p/，直接使用
			p2pAddrs = append(p2pAddrs, addr)
		} else {
			// 添加 /p2p/<peerID>
			p2pAddr, err := types.NewMultiaddr(fmt.Sprintf("%s/p2p/%s", addr.String(), hostID))
			if err != nil {
				continue
			}
			p2pAddrs = append(p2pAddrs, p2pAddr)
		}
	}

	// 过滤适合 mDNS 的地址
	var suitableAddrs []types.Multiaddr
	for _, addr := range p2pAddrs {
		if isSuitableForMDNS(addr) {
			suitableAddrs = append(suitableAddrs, addr)
		}
	}

	if len(suitableAddrs) == 0 {
		// 优雅降级：没有适合的地址时进入等待状态
		m.state.Store(StateWaiting)
		return nil
	}

	// 构建 TXT 记录
	var txts []string
	for _, addr := range suitableAddrs {
		txts = append(txts, DNSAddrPrefix+addr.String())
	}

	// 提取 IP 地址
	ips, err := m.getIPs(multiaddrs)
	if err != nil {
		// 优雅降级：无法提取 IP 时进入等待状态
		m.state.Store(StateWaiting)
		return nil
	}

	// 注册服务
	server, err := zeroconf.RegisterProxy(
		m.peerName,
		m.config.ServiceTag,
		MDNSDomain,
		4001, // 占位端口，实际不用
		m.peerName,
		ips,
		txts,
		nil, // nil = 所有接口
	)
	if err != nil {
		return err
	}

	m.server = server
	m.state.Store(StateRunning)
	return nil
}

// expandWildcardAddrs 将包含 0.0.0.0 的地址展开为实际的网络接口地址
//
// 例如 /ip4/0.0.0.0/udp/1234/quic-v1 会被展开为：
//   - /ip4/192.168.1.100/udp/1234/quic-v1
//   - /ip4/10.0.0.5/udp/1234/quic-v1
func (m *MDNS) expandWildcardAddrs(addrs []string) []string {
	// 获取本机网络接口地址
	localIPs := getLocalInterfaceIPs()
	if len(localIPs) == 0 {
		logger.Warn("未找到本地网络接口地址")
		return nil
	}

	logger.Debug("本地网络接口", "ips", localIPs)

	var result []string
	for _, addr := range addrs {
		if strings.Contains(addr, "/ip4/0.0.0.0/") {
			// 展开为每个本地 IP
			for _, ip := range localIPs {
				if ip.To4() != nil { // 只用 IPv4
					expanded := strings.Replace(addr, "/ip4/0.0.0.0/", "/ip4/"+ip.String()+"/", 1)
					result = append(result, expanded)
					logger.Debug("展开地址", "from", addr, "to", expanded)
				}
			}
		} else if strings.Contains(addr, "/ip6/::/") {
			// 展开 IPv6 通配符
			for _, ip := range localIPs {
				if ip.To4() == nil && ip.To16() != nil { // IPv6
					expanded := strings.Replace(addr, "/ip6/::/", "/ip6/"+ip.String()+"/", 1)
					result = append(result, expanded)
				}
			}
		} else {
			// 非通配符地址直接保留
			result = append(result, addr)
			logger.Debug("保留地址", "addr", addr)
		}
	}

	logger.Debug("展开后地址", "count", len(result))
	return result
}

// getLocalInterfaceIPs 获取本机所有网络接口的 IP 地址
// 只返回适合 mDNS 广播的地址（非回环、非链路本地、非虚拟网桥）
func getLocalInterfaceIPs() []net.IP {
	var ips []net.IP

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	for _, iface := range ifaces {
		// 跳过 down 的接口
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		// 跳过回环接口
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		// 跳过 Docker 和其他容器虚拟网桥接口
		// 这些接口的 IP 地址只对本机容器可达，对外部节点不可达
		if isVirtualBridgeInterface(iface.Name) {
			logger.Debug("跳过虚拟网桥接口", "interface", iface.Name)
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

			if ip == nil {
				continue
			}

			// 跳过回环地址
			if ip.IsLoopback() {
				continue
			}

			// 跳过链路本地地址
			if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				continue
			}

			// 跳过无效地址
			if ip.IsUnspecified() {
				continue
			}

			//
			// 198.18.0.0/15 是 RFC 2544 保留的基准测试地址，
			// 常被 VPN/代理软件（Surge、Clash 等）用作虚拟网络接口，
			// 这些地址只在本机有效，对其他节点不可达
			if !isValidBroadcastIP(ip) {
				continue
			}

			ips = append(ips, ip)
		}
	}

	return ips
}

// isVirtualBridgeInterface 检查接口是否为虚拟网桥接口
//
//   - docker0: Docker 默认网桥
//   - br-*: Docker 自定义网络网桥
//   - veth*: Docker/容器虚拟以太网设备
//   - docker_gwbridge: Docker Swarm 网关网桥
//   - cni*: Kubernetes CNI 网络接口
//   - flannel*: Flannel 网络接口
//   - calico*: Calico 网络接口
//   - weave*: Weave 网络接口
//   - virbr*: libvirt/KVM 虚拟网桥
//   - lxcbr*: LXC 容器网桥
//   - lxdbr*: LXD 容器网桥
//
// 这些接口的 IP 地址通常只对本机容器/虚拟机可达，对外部节点不可达。
func isVirtualBridgeInterface(name string) bool {
	// Docker 相关
	if name == "docker0" || name == "docker_gwbridge" {
		return true
	}
	if strings.HasPrefix(name, "br-") {
		return true
	}
	if strings.HasPrefix(name, "veth") {
		return true
	}

	// Kubernetes/CNI 相关
	if strings.HasPrefix(name, "cni") {
		return true
	}
	if strings.HasPrefix(name, "flannel") {
		return true
	}
	if strings.HasPrefix(name, "calico") {
		return true
	}
	if strings.HasPrefix(name, "weave") {
		return true
	}

	// 虚拟化相关
	if strings.HasPrefix(name, "virbr") {
		return true
	}
	if strings.HasPrefix(name, "lxcbr") {
		return true
	}
	if strings.HasPrefix(name, "lxdbr") {
		return true
	}

	return false
}

// isValidBroadcastIP 检查 IP 是否适合对外广播
//
//   - 198.18.0.0/15: RFC 2544 基准测试地址（VPN/代理常用）
//   - 100.64.0.0/10: RFC 6598 运营商级 NAT 地址（CGNAT）
//   - 169.254.0.0/16: 链路本地地址（已在上面过滤）
func isValidBroadcastIP(ip net.IP) bool {
	ip4 := ip.To4()
	if ip4 == nil {
		// IPv6 暂不过滤额外地址
		return true
	}

	// 198.18.0.0/15 - RFC 2544 基准测试地址
	// VPN/代理软件（Surge、Clash、Shadowrocket 等）常用此地址段
	if ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19) {
		logger.Debug("2544 基准测试地址（VPN/代理虚拟接口）", "ip", ip.String())
		return false
	}

	// 100.64.0.0/10 - RFC 6598 运营商级 NAT (CGNAT)
	// 这些地址在运营商网络内部使用，对外不可达
	if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
		logger.Debug("6598 CGNAT 地址", "ip", ip.String())
		return false
	}

	return true
}

// startResolver 启动 mDNS 解析器（发现）
// 返回 done channel 和 notifee（用于安全关闭）
func (m *MDNS) startResolver(ctx context.Context, peerCh chan<- types.PeerInfo) (<-chan struct{}, *peerNotifee, error) {
	// 创建 ServiceEntry channel
	entryChan := make(chan *zeroconf.ServiceEntry, 1000)

	// 创建 notifee
	notifee := newPeerNotifee(ctx, m.host.ID(), peerCh)

	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	// 启动 entry 处理 goroutine
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer wg.Done()
		for entry := range entryChan {
			if err := notifee.handleEntry(entry); err != nil {
				logger.Debug("处理 entry 失败", "error", err)
				continue
			}
		}
	}()

	// 启动 Browse goroutine
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer wg.Done()

		// 创建 Resolver
		resolver, err := zeroconf.NewResolver(nil)
		if err != nil {
			logger.Error("创建 mDNS 解析器失败", "error", err)
			close(entryChan)
			return
		}

		// 开始浏览（这会阻塞直到 ctx 被取消）
		// zeroconf 会在内部关闭 entryChan，所以我们不需要手动关闭
		err = resolver.Browse(ctx, m.config.ServiceTag, MDNSDomain, entryChan)
		if err != nil {
			logger.Debug("mDNS 浏览结束", "error", err)
		}
	}()

	go func() {
		wg.Wait()
		close(done)
	}()

	return done, notifee, nil
}

// getIPs 从 multiaddr 中提取 IP 地址
func (m *MDNS) getIPs(addrs []types.Multiaddr) ([]string, error) {
	var ip4, ip6 string

	for _, addr := range addrs {
		first, _ := types.SplitFirst(addr)
		if first.Protocol().Code == 0 {
			continue
		}

		if ip4 == "" && first.Protocol().Code == types.P_IP4 {
			ip4 = first.Value()
		} else if ip6 == "" && first.Protocol().Code == types.P_IP6 {
			ip6 = first.Value()
		}
	}

	ips := make([]string, 0, 2)
	if ip4 != "" {
		ips = append(ips, ip4)
	}
	if ip6 != "" {
		ips = append(ips, ip6)
	}

	if len(ips) == 0 {
		return nil, ErrNoValidAddresses
	}

	return ips, nil
}

// randomString 生成随机字符串（用于 peer name，非安全场景）
func randomString(l int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	s := make([]byte, 0, l)
	for i := 0; i < l; i++ {
		s = append(s, alphabet[rand.Intn(len(alphabet))]) //nolint:gosec // G404: 非安全场景
	}
	return string(s)
}
