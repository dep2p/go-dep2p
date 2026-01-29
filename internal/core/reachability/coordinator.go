// Package reachability 提供可达性协调模块的实现
package reachability

import (
	"context"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("core/reachability")

// ============================================================================
//                              Coordinator 结构
// ============================================================================

// Coordinator 可达性协调器
type Coordinator struct {
	// 配置
	config *interfaces.ReachabilityConfig

	// 已验证的直连地址
	verifiedAddrs   map[string]*AddressEntry
	verifiedAddrsMu sync.RWMutex

	// 直连地址候选（尚未通过验证）
	candidateAddrs   map[string]*AddressEntry
	candidateAddrsMu sync.RWMutex

	// Relay 地址
	relayAddrs   []string
	relayAddrsMu sync.RWMutex

	// Witness 证据台账
	witnessLedger   map[string]map[string]*WitnessRecord
	witnessLedgerMu sync.RWMutex

	// 地址变更回调
	onChange   func([]string)
	onChangeMu sync.RWMutex

	// 按来源索引
	sourceIndex   map[string]map[string]struct{}
	sourceIndexMu sync.RWMutex

	// dial-back 服务
	dialBackService *DialBackService
	enableDialBack  bool

	// witness 服务
	witnessService *WitnessService

	// 网卡扫描器
	interfaceScanner *InterfaceScanner

	// 监听端口（用于与发现的 IP 组合）
	listenPorts   []int
	listenPortsMu sync.RWMutex

	// 持久化存储
	store *DirectAddrStore

	// 直接地址更新状态机（可选，用于协调地址发现→验证→发布流程）
	stateMachine   *DirectAddrUpdateStateMachine
	stateMachineMu sync.RWMutex

	// 运行状态
	ctx       context.Context
	cancel    context.CancelFunc
	running   bool
	runningMu sync.Mutex
}

// AddressEntry 地址条目
type AddressEntry struct {
	Addr       string
	Priority   interfaces.AddressPriority
	Source     string
	Verified   bool
	VerifiedAt time.Time
	LastSeen   time.Time
}

// WitnessRecord 入站见证记录
type WitnessRecord struct {
	PeerID         string
	RemoteIPPrefix string
	Timestamp      time.Time
}

// NewCoordinator 创建可达性协调器
func NewCoordinator(config *interfaces.ReachabilityConfig) *Coordinator {
	if config == nil {
		config = interfaces.DefaultReachabilityConfig()
	}

	c := &Coordinator{
		config:           config,
		verifiedAddrs:    make(map[string]*AddressEntry),
		candidateAddrs:   make(map[string]*AddressEntry),
		witnessLedger:    make(map[string]map[string]*WitnessRecord),
		sourceIndex:      make(map[string]map[string]struct{}),
		interfaceScanner: NewInterfaceScanner(),
	}

	// 初始化存储
	if config.DirectAddrStorePath != "" || config.EnableDialBack {
		c.store = NewDirectAddrStore(config.DirectAddrStorePath)
		c.store.SetConfig(1000, config.CandidateTTL, config.VerifiedTTL, time.Second)
	}

	return c
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动协调器
func (c *Coordinator) Start(ctx context.Context) error {
	c.runningMu.Lock()
	defer c.runningMu.Unlock()

	if c.running {
		return nil
	}

	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.running = true

	// 启动 dial-back 服务
	if c.config.EnableDialBack && c.dialBackService != nil {
		_ = c.dialBackService.Start(ctx)
	}
	c.enableDialBack = c.config.EnableDialBack

	// 启动 witness 服务
	if c.witnessService != nil {
		_ = c.witnessService.Start(c.ctx)
	}

	// 加载存储
	if c.store != nil {
		if err := c.loadStoreAndSeed(); err != nil {
			logger.Warn("加载直连地址存储失败", "err", err)
		} else {
			c.notifyAddressChanged()
		}

		// 启动定期清理
		go c.cleanupExpiredLoop()
	}

	// 启动定期重验
	if c.enableDialBack {
		go c.reVerificationLoop()
	}

	logger.Info("可达性协调器已启动")
	return nil
}

// Stop 停止协调器
func (c *Coordinator) Stop() error {
	c.runningMu.Lock()
	defer c.runningMu.Unlock()

	if !c.running {
		return nil
	}

	if c.cancel != nil {
		c.cancel()
	}
	c.running = false

	if c.dialBackService != nil {
		_ = c.dialBackService.Stop()
	}

	if c.witnessService != nil {
		_ = c.witnessService.Stop()
	}

	if c.store != nil {
		_ = c.store.Close()
	}

	logger.Info("可达性协调器已停止")
	return nil
}

// ============================================================================
//                              状态机集成
// ============================================================================

// SetStateMachine 设置直接地址更新状态机
//
// 状态机用于协调地址更新流程：发现 → 验证 → 发布
// 设置后，Coordinator 会在地址变更时自动触发状态机
func (c *Coordinator) SetStateMachine(sm *DirectAddrUpdateStateMachine) {
	c.stateMachineMu.Lock()
	defer c.stateMachineMu.Unlock()
	c.stateMachine = sm

	if sm == nil {
		return
	}

	// 配置状态机回调
	sm.SetDiscoverCallback(c.discoverExternalAddresses)
	sm.SetValidateCallback(c.validateDiscoveredAddresses)
	sm.SetPublishCallback(c.publishValidatedAddresses)

	logger.Debug("DirectAddrUpdateStateMachine 已设置")
}

// GetStateMachine 获取直接地址更新状态机
func (c *Coordinator) GetStateMachine() *DirectAddrUpdateStateMachine {
	c.stateMachineMu.RLock()
	defer c.stateMachineMu.RUnlock()
	return c.stateMachine
}

// discoverExternalAddresses 发现外部地址（状态机回调）
func (c *Coordinator) discoverExternalAddresses(_ context.Context) ([]string, error) {
	// 使用网卡扫描器发现本地地址
	if c.interfaceScanner != nil {
		ips := c.interfaceScanner.DiscoverAllUsableIPs()
		if len(ips) > 0 {
			// 将 IP 转换为字符串切片
			addrs := make([]string, 0, len(ips))
			for _, ip := range ips {
				addrs = append(addrs, ip.String())
			}
			return addrs, nil
		}
	}

	// 如果没有网卡扫描器结果，返回当前候选地址
	c.candidateAddrsMu.RLock()
	defer c.candidateAddrsMu.RUnlock()

	result := make([]string, 0, len(c.candidateAddrs))
	for addr := range c.candidateAddrs {
		result = append(result, addr)
	}
	return result, nil
}

// validateDiscoveredAddresses 验证发现的地址（状态机回调）
func (c *Coordinator) validateDiscoveredAddresses(ctx context.Context, addrs []string) ([]string, error) {
	if !c.enableDialBack || c.dialBackService == nil {
		// 没有 dial-back 服务，直接返回所有地址作为有效
		return addrs, nil
	}

	// 使用 dial-back 服务批量验证地址
	// VerifyAddresses 签名：(ctx context.Context, helperID string, candidateAddrs []string)
	// helperID 为空时使用默认验证
	validated, err := c.dialBackService.VerifyAddresses(ctx, "", addrs)
	if err != nil {
		// 验证失败，返回空列表
		logger.Warn("dial-back 验证失败", "error", err)
		return nil, nil
	}
	return validated, nil
}

// publishValidatedAddresses 发布验证后的地址（状态机回调）
func (c *Coordinator) publishValidatedAddresses(_ context.Context, addrs []string) error {
	for _, addr := range addrs {
		// 将地址添加为已验证（使用现有的 OnDirectAddressVerified 方法）
		c.OnDirectAddressVerified(addr, "state_machine", interfaces.PriorityVerifiedDirect)
	}
	return nil
}

// TriggerAddressUpdate 手动触发地址更新流程
//
// 如果设置了状态机，会启动完整的发现→验证→发布流程
func (c *Coordinator) TriggerAddressUpdate(ctx context.Context) error {
	c.stateMachineMu.RLock()
	sm := c.stateMachine
	c.stateMachineMu.RUnlock()

	if sm == nil {
		// 没有状态机，直接刷新地址
		c.notifyAddressChanged()
		return nil
	}

	return sm.StartUpdate(ctx)
}

// ============================================================================
//                              地址管理
// ============================================================================

// AdvertisedAddrs 返回按优先级排序的通告地址
func (c *Coordinator) AdvertisedAddrs() []string {
	now := time.Now()

	// 快照已验证直连地址
	verified := c.filteredVerifiedEntries()

	// 快照 Relay 地址
	c.relayAddrsMu.RLock()
	relayAddrs := make([]string, len(c.relayAddrs))
	copy(relayAddrs, c.relayAddrs)
	c.relayAddrsMu.RUnlock()

	// 聚合地址
	entries := make([]*AddressEntry, 0, len(verified)+len(relayAddrs))

	// 1. 已验证直连地址（优先级最高）
	entries = append(entries, verified...)

	// 2. Relay 地址（始终保留，保证可达性兜底）
	for _, addr := range relayAddrs {
		if addr == "" {
			continue
		}
		entries = append(entries, &AddressEntry{
			Addr:     addr,
			Priority: interfaces.PriorityRelayGuarantee,
			Source:   "relay",
			Verified: true,
			LastSeen: now,
		})
	}

	// 按优先级排序
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Priority > entries[j].Priority
	})

	// 去重
	seen := make(map[string]struct{}, len(entries))
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || entry.Addr == "" {
			continue
		}
		if _, ok := seen[entry.Addr]; ok {
			continue
		}
		seen[entry.Addr] = struct{}{}
		result = append(result, entry.Addr)
	}

	return result
}

// VerifiedDirectAddresses 返回已验证的直连地址
func (c *Coordinator) VerifiedDirectAddresses() []string {
	entries := c.filteredVerifiedEntries()
	result := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || entry.Addr == "" {
			continue
		}
		result = append(result, entry.Addr)
	}
	return result
}

// CandidateDirectAddresses 返回候选直连地址列表（未验证的 STUN/UPnP/NAT-PMP 地址）
//
// 这些地址是 NAT 网关分配的公网端点，虽未通过 dial-back 验证，
// 但对于打洞协商是必需的——打洞的本质就是双方同时向对方的外部地址发包。
func (c *Coordinator) CandidateDirectAddresses() []string {
	c.candidateAddrsMu.RLock()
	defer c.candidateAddrsMu.RUnlock()

	result := make([]string, 0, len(c.candidateAddrs))
	for _, entry := range c.candidateAddrs {
		if entry == nil || entry.Addr == "" {
			continue
		}
		result = append(result, entry.Addr)
	}

	logger.Debug("获取候选地址",
		"count", len(result),
		"addrs", result)

	return result
}

// RelayAddresses 返回所有 Relay 地址
func (c *Coordinator) RelayAddresses() []string {
	c.relayAddrsMu.RLock()
	defer c.relayAddrsMu.RUnlock()

	result := make([]string, len(c.relayAddrs))
	copy(result, c.relayAddrs)
	return result
}

// BootstrapCandidates 返回可用于冷启动的候选地址
func (c *Coordinator) BootstrapCandidates(nodeID string) []interfaces.BootstrapCandidate {
	var result []interfaces.BootstrapCandidate
	seen := make(map[string]bool)

	// 1. 已验证的直连地址
	for _, entry := range c.filteredVerifiedEntries() {
		if entry.Addr == "" {
			continue
		}
		fullAddr := entry.Addr + "/p2p/" + nodeID
		if seen[fullAddr] {
			continue
		}
		seen[fullAddr] = true

		result = append(result, interfaces.BootstrapCandidate{
			FullAddr:   fullAddr,
			Kind:       interfaces.CandidateKindDirect,
			Source:     entry.Source,
			Confidence: interfaces.ConfidenceHigh,
			Verified:   true,
			Notes:      "已验证直连地址",
		})
	}

	// 2. 未验证的直连候选
	c.candidateAddrsMu.RLock()
	for _, entry := range c.candidateAddrs {
		if entry.Addr == "" {
			continue
		}
		fullAddr := entry.Addr + "/p2p/" + nodeID
		if seen[fullAddr] {
			continue
		}
		seen[fullAddr] = true

		confidence := interfaces.ConfidenceLow
		switch entry.Source {
		case "listen-bound-public", "cloud-metadata", "stun":
			confidence = interfaces.ConfidenceMedium
		}

		result = append(result, interfaces.BootstrapCandidate{
			FullAddr:   fullAddr,
			Kind:       interfaces.CandidateKindDirect,
			Source:     entry.Source,
			Confidence: confidence,
			Verified:   false,
			Notes:      "待验证候选",
		})
	}
	c.candidateAddrsMu.RUnlock()

	// 3. Relay 地址
	c.relayAddrsMu.RLock()
	for _, addr := range c.relayAddrs {
		if addr == "" {
			continue
		}
		fullAddr := addr
		if !strings.Contains(addr, "/p2p/"+nodeID) {
			fullAddr = addr + "/p2p/" + nodeID
		}
		if seen[fullAddr] {
			continue
		}
		seen[fullAddr] = true

		result = append(result, interfaces.BootstrapCandidate{
			FullAddr:   fullAddr,
			Kind:       interfaces.CandidateKindRelay,
			Source:     "relay",
			Confidence: interfaces.ConfidenceMedium,
			Verified:   false,
			Notes:      "中继候选",
		})
	}
	c.relayAddrsMu.RUnlock()

	return result
}

// OnRelayReserved Relay 预留成功回调
func (c *Coordinator) OnRelayReserved(addrs []string) {
	// BUG FIX #B27: 过滤空地址
	filtered := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		if addr != "" {
			filtered = append(filtered, addr)
		}
	}

	c.relayAddrsMu.Lock()
	c.relayAddrs = filtered
	c.relayAddrsMu.Unlock()

	if len(filtered) != len(addrs) {
		logger.Warn("过滤了空 Relay 地址", "原始数量", len(addrs), "过滤后", len(filtered))
	}

	logger.Info("Relay 地址已更新", "count", len(filtered))
	c.notifyAddressChanged()
}

// OnDirectAddressVerified 直连地址验证成功回调
func (c *Coordinator) OnDirectAddressVerified(addr string, source string, priority interfaces.AddressPriority) {
	// BUG FIX #B27: 拒绝空地址
	if addr == "" {
		logger.Warn("拒绝空地址验证", "source", source)
		return
	}

	now := time.Now()

	c.verifiedAddrsMu.Lock()
	wasEmpty := len(c.verifiedAddrs) == 0
	c.verifiedAddrs[addr] = &AddressEntry{
		Addr:       addr,
		Priority:   priority,
		Source:     source,
		Verified:   true,
		VerifiedAt: now,
		LastSeen:   now,
	}
	c.verifiedAddrsMu.Unlock()

	// 从候选地址中移除
	c.candidateAddrsMu.Lock()
	delete(c.candidateAddrs, addr)
	c.candidateAddrsMu.Unlock()

	// 更新来源索引
	c.updateSourceIndex(source, addr)

	// 更新存储
	if c.store != nil {
		c.store.UpdateVerified(addr, source, priority)
	}

	logger.Info("直连地址已验证并发布",
		"addr", addr,
		"source", source,
		"priority", priority.String())

	c.notifyAddressChanged()

	// 如果是第一个验证的直连地址，可以触发额外操作
	if wasEmpty {
		logger.Debug("首个验证直连地址已添加")
	}
}

// OnDirectAddressCandidate 直连地址候选回调
func (c *Coordinator) OnDirectAddressCandidate(addr string, source string, priority interfaces.AddressPriority) {
	if addr == "" {
		return
	}
	now := time.Now()

	c.candidateAddrsMu.Lock()
	c.candidateAddrs[addr] = &AddressEntry{
		Addr:     addr,
		Priority: priority,
		Source:   source,
		Verified: false,
		LastSeen: now,
	}
	c.candidateAddrsMu.Unlock()

	// 更新来源索引
	c.updateSourceIndex(source, addr)

	// 更新存储
	if c.store != nil {
		c.store.UpdateCandidate(addr, source, priority)
	}

	logger.Debug("直连地址候选已上报",
		"addr", addr,
		"source", source,
		"priority", priority.String())

	// 启用 dial-back 时：异步触发验证
	if c.enableDialBack && c.dialBackService != nil {
		go func(a string) {
			verifyCtx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
			defer cancel()
			c.verifyAddressWithDialBack(verifyCtx, []string{a})
		}(addr)
	}
}

// UpdateDirectCandidates 批量更新直连候选地址
func (c *Coordinator) UpdateDirectCandidates(source string, candidates []interfaces.CandidateUpdate) {
	if len(candidates) == 0 {
		c.replaceAddressesFromSource(source, make(map[string]struct{}))
		return
	}

	newAddrKeys := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate.Addr == "" {
			continue
		}
		newAddrKeys[candidate.Addr] = struct{}{}
		c.OnDirectAddressCandidate(candidate.Addr, source, candidate.Priority)
	}

	c.replaceAddressesFromSource(source, newAddrKeys)

	logger.Debug("批量更新直连候选地址完成",
		"source", source,
		"count", len(candidates))
}

// OnDirectAddressExpired 直连地址过期
func (c *Coordinator) OnDirectAddressExpired(addr string) {
	// BUG FIX #B27: 忽略空地址
	if addr == "" {
		return
	}

	c.verifiedAddrsMu.Lock()
	entry, exists := c.verifiedAddrs[addr]
	delete(c.verifiedAddrs, addr)
	nowEmpty := len(c.verifiedAddrs) == 0
	c.verifiedAddrsMu.Unlock()

	if exists && entry != nil {
		c.removeFromSourceIndex(entry.Source, addr)
	}

	if c.store != nil && exists {
		c.store.RemoveVerified(addr)
	}

	logger.Info("直连地址已过期", "addr", addr)
	c.notifyAddressChanged()

	if nowEmpty {
		logger.Debug("所有验证直连地址已移除")
	}
}

// ============================================================================
//                              回调注册
// ============================================================================

// SetOnAddressChanged 设置地址变更回调
func (c *Coordinator) SetOnAddressChanged(callback func([]string)) {
	c.onChangeMu.Lock()
	c.onChange = callback
	c.onChangeMu.Unlock()
}

// notifyAddressChanged 通知地址变更
func (c *Coordinator) notifyAddressChanged() {
	c.onChangeMu.RLock()
	callback := c.onChange
	c.onChangeMu.RUnlock()

	addrs := c.AdvertisedAddrs()

	// 统计地址来源
	c.verifiedAddrsMu.RLock()
	verifiedCount := len(c.verifiedAddrs)
	c.verifiedAddrsMu.RUnlock()

	c.relayAddrsMu.RLock()
	relayCount := len(c.relayAddrs)
	c.relayAddrsMu.RUnlock()

	logger.Info("对外通告地址已更新",
		"totalAddrs", len(addrs),
		"verifiedDirect", verifiedCount,
		"relayAddrs", relayCount,
		"addrs", addrs)

	if callback != nil {
		go callback(addrs)
	}
}

// ============================================================================
//                              辅助方法
// ============================================================================

// HasRelayAddress 是否有 Relay 地址
func (c *Coordinator) HasRelayAddress() bool {
	c.relayAddrsMu.RLock()
	defer c.relayAddrsMu.RUnlock()
	return len(c.relayAddrs) > 0
}

// HasVerifiedDirectAddress 是否有已验证的直连地址
func (c *Coordinator) HasVerifiedDirectAddress() bool {
	c.verifiedAddrsMu.RLock()
	defer c.verifiedAddrsMu.RUnlock()
	return len(c.verifiedAddrs) > 0
}

// SetDialBackService 设置 dial-back 服务
func (c *Coordinator) SetDialBackService(service *DialBackService) {
	c.dialBackService = service
}

// SetWitnessService 设置 witness 服务
func (c *Coordinator) SetWitnessService(service *WitnessService) {
	c.witnessService = service
}

// SetListenPorts 设置监听端口
//
// 用于与发现的外部 IP 组合生成完整的 multiaddr。
// 应在 Swarm 监听成功后调用。
func (c *Coordinator) SetListenPorts(ports []int) {
	c.listenPortsMu.Lock()
	c.listenPorts = make([]int, len(ports))
	copy(c.listenPorts, ports)
	c.listenPortsMu.Unlock()

	logger.Debug("监听端口已更新", "ports", ports)

	// 端口一旦确定，清理与当前监听端口不匹配的历史地址
	c.pruneVerifiedByListenPorts()

	// 同步执行网卡发现（确保启动时地址立即可用）
	c.DiscoverFromInterfaces()
}

// SetConfiguredAddresses 设置用户配置的公网地址
//
// 用户通过 WithPublicAddr() 配置的地址，作为最高优先级的已验证地址。
// 这些地址是用户明确声明可用的，不需要 dial-back 验证。
//
// 典型场景：
//   - 云服务器：用户知道弹性 IP，但网卡上看不到
//   - 端口映射：用户已配置 NAT 端口转发
//   - Bootstrap 节点：需要固定的公网地址
func (c *Coordinator) SetConfiguredAddresses(addrs []string) {
	if len(addrs) == 0 {
		return
	}

	for _, addr := range addrs {
		if addr == "" {
			continue
		}
		// 用户配置的地址，最高优先级，直接标记为已验证
		c.OnDirectAddressVerified(addr, "configured", interfaces.PriorityConfiguredAdvertise)
	}

	logger.Info("用户配置地址已添加", "count", len(addrs), "addrs", addrs)
}

// GetListenPorts 获取监听端口
func (c *Coordinator) GetListenPorts() []int {
	c.listenPortsMu.RLock()
	defer c.listenPortsMu.RUnlock()

	ports := make([]int, len(c.listenPorts))
	copy(ports, c.listenPorts)
	return ports
}

// DiscoverFromInterfaces 从本地网卡发现公网地址
//
// 扫描本机所有网络接口，找出公网 IP 并上报为候选地址。
// 适用于云服务器直接绑定公网 IP 的场景。
func (c *Coordinator) DiscoverFromInterfaces() {
	if c.interfaceScanner == nil {
		logger.Debug("网卡扫描器未初始化")
		return
	}

	ports := c.GetListenPorts()
	if len(ports) == 0 {
		logger.Debug("未配置监听端口，跳过网卡发现", "reason", "no_ports")
		return
	}

	logger.Debug("开始网卡扫描", "listenPorts", ports)

	// 发现所有可用 IP
	allIPs := c.interfaceScanner.DiscoverAllUsableIPs()
	publicIPs := c.interfaceScanner.DiscoverPublicIPs()

	logger.Info("网卡扫描结果",
		"allUsableIPs", len(allIPs),
		"publicIPs", len(publicIPs),
		"listenPorts", ports)

	// 记录发现的 IP
	for _, ip := range allIPs {
		isPublic := false
		for _, pub := range publicIPs {
			if ip.Equal(pub) {
				isPublic = true
				break
			}
		}
		logger.Debug("发现网卡 IP",
			"ip", ip.String(),
			"isPublic", isPublic)
	}

	if len(publicIPs) == 0 {
		logger.Debug("未发现本机公网接口 IP",
			"totalIPs", len(allIPs),
			"hint", "可能处于 NAT 后，需要依赖 STUN 发现外部地址")
		return
	}

	// 为每个公网 IP 和端口组合生成地址
	var addedAddrs []string
	for _, ip := range publicIPs {
		for _, port := range ports {
			maddr := buildMultiaddr(ip, port)
			// 本机公网 IP 通常是可信的，直接标记为已验证
			c.OnDirectAddressVerified(maddr, "local-interface-public", interfaces.PriorityVerifiedDirect)
			addedAddrs = append(addedAddrs, maddr)
		}
	}

	logger.Info("从网卡发现并验证公网地址",
		"count", len(addedAddrs),
		"addrs", addedAddrs)
}

// buildMultiaddr 构建 multiaddr 格式地址
func buildMultiaddr(ip net.IP, port int) string {
	if ip4 := ip.To4(); ip4 != nil {
		return "/ip4/" + ip4.String() + "/udp/" + itoa(port) + "/quic-v1"
	}
	return "/ip6/" + ip.String() + "/udp/" + itoa(port) + "/quic-v1"
}

// ============================================================================
//                              Witness 验证
// ============================================================================

// OnInboundWitness 上报入站见证
func (c *Coordinator) OnInboundWitness(dialedAddr string, remotePeerID string, remoteIP string) {
	if dialedAddr == "" || remotePeerID == "" || remoteIP == "" {
		return
	}

	// 检查是否是候选地址
	c.candidateAddrsMu.RLock()
	entry, isCandidate := c.candidateAddrs[dialedAddr]
	c.candidateAddrsMu.RUnlock()

	if !isCandidate {
		logger.Debug("witness: 地址不在候选列表中",
			"dialedAddr", dialedAddr,
			"remotePeer", remotePeerID)
		return
	}

	// 计算 IP 前缀
	ipPrefix := c.getIPPrefix(remoteIP)
	if ipPrefix == "" {
		logger.Debug("witness: 无法解析 IP 前缀", "remoteIP", remoteIP)
		return
	}

	// 生成去重键
	witnessKey := remotePeerID + "|" + ipPrefix

	c.witnessLedgerMu.Lock()
	defer c.witnessLedgerMu.Unlock()

	// 初始化
	if c.witnessLedger[dialedAddr] == nil {
		c.witnessLedger[dialedAddr] = make(map[string]*WitnessRecord)
	}

	// 检查是否重复
	if _, exists := c.witnessLedger[dialedAddr][witnessKey]; exists {
		logger.Debug("witness: 重复见证",
			"dialedAddr", dialedAddr,
			"remotePeer", remotePeerID)
		return
	}

	// 记录见证
	c.witnessLedger[dialedAddr][witnessKey] = &WitnessRecord{
		PeerID:         remotePeerID,
		RemoteIPPrefix: ipPrefix,
		Timestamp:      time.Now(),
	}

	// 统计不同 IP 前缀
	uniquePrefixes := make(map[string]bool)
	for _, record := range c.witnessLedger[dialedAddr] {
		uniquePrefixes[record.RemoteIPPrefix] = true
	}
	witnessCount := len(uniquePrefixes)

	logger.Info("witness: 记录入站见证",
		"dialedAddr", dialedAddr,
		"remotePeer", remotePeerID,
		"witnessCount", witnessCount,
		"minWitnesses", c.config.MinWitnesses)

	// 检查是否达到阈值
	if witnessCount >= c.config.MinWitnesses {
		logger.Info("witness: 达到阈值，升级为 VerifiedDirect",
			"dialedAddr", dialedAddr,
			"witnessCount", witnessCount)

		c.OnDirectAddressVerified(entry.Addr, "witness-threshold", interfaces.PriorityVerifiedDirect)
	}
}

// getIPPrefix 计算 IP 前缀
func (c *Coordinator) getIPPrefix(ipStr string) string {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return ""
	}

	if ip4 := ip.To4(); ip4 != nil {
		// IPv4
		mask := net.CIDRMask(c.config.WitnessIPv4Prefix, 32)
		network := ip4.Mask(mask)
		return network.String() + "/" + itoa(c.config.WitnessIPv4Prefix)
	}

	// IPv6
	mask := net.CIDRMask(c.config.WitnessIPv6Prefix, 128)
	network := ip.Mask(mask)
	return network.String() + "/" + itoa(c.config.WitnessIPv6Prefix)
}

// itoa int 转 string
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}

// ============================================================================
//                              Dial-Back 验证
// ============================================================================

// verifyAddressWithDialBack 使用 dial-back 验证地址
func (c *Coordinator) verifyAddressWithDialBack(ctx context.Context, candidateAddrs []string) ([]string, error) {
	if !c.enableDialBack || c.dialBackService == nil {
		return nil, nil
	}

	// 使用配置的 trusted helpers
	reachable, err := c.dialBackService.VerifyAddresses(ctx, "", candidateAddrs)
	if err != nil {
		return nil, err
	}

	// 将验证成功的地址添加到已验证列表
	for _, addr := range reachable {
		c.OnDirectAddressVerified(addr, "dial-back", interfaces.PriorityVerifiedDirect)
	}

	return reachable, nil
}

// reVerificationLoop 定期重验已发布地址
func (c *Coordinator) reVerificationLoop() {
	interval := c.config.VerificationInterval
	if interval == 0 {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.reVerifyPublishedAddresses()
		}
	}
}

// reVerifyPublishedAddresses 重新验证所有已发布的直连地址
func (c *Coordinator) reVerifyPublishedAddresses() {
	if !c.enableDialBack || c.dialBackService == nil {
		return
	}

	// 获取当前已验证地址
	c.verifiedAddrsMu.RLock()
	addrs := make([]string, 0, len(c.verifiedAddrs))
	for _, entry := range c.verifiedAddrs {
		addrs = append(addrs, entry.Addr)
	}
	c.verifiedAddrsMu.RUnlock()

	if len(addrs) == 0 {
		return
	}

	logger.Debug("开始定期重验已发布地址", "count", len(addrs))

	ctx, cancel := context.WithTimeout(c.ctx, 60*time.Second)
	defer cancel()

	reachable, err := c.dialBackService.VerifyAddresses(ctx, "", addrs)
	if err != nil {
		logger.Debug("定期重验失败", "err", err)
		return
	}

	// 构建可达地址集合
	reachableSet := make(map[string]struct{}, len(reachable))
	for _, addr := range reachable {
		reachableSet[addr] = struct{}{}
	}

	// 移除不可达的地址
	var removed []string
	c.verifiedAddrsMu.Lock()
	for key, entry := range c.verifiedAddrs {
		if _, ok := reachableSet[key]; !ok {
			delete(c.verifiedAddrs, key)
			removed = append(removed, entry.Addr)
			logger.Info("定期重验：地址不可达，已移除", "addr", entry.Addr)
		} else {
			entry.LastSeen = time.Now()
		}
	}
	c.verifiedAddrsMu.Unlock()

	if c.store != nil && len(removed) > 0 {
		for _, addr := range removed {
			c.store.RemoveVerified(addr)
		}
	}

	if len(removed) > 0 {
		c.notifyAddressChanged()
	}

	logger.Debug("定期重验完成",
		"verified", len(addrs),
		"still_reachable", len(reachable))
}

// ============================================================================
//                              存储相关
// ============================================================================

// loadStoreAndSeed 加载存储并 seed 内存
func (c *Coordinator) loadStoreAndSeed() error {
	if c.store == nil {
		return nil
	}

	if err := c.store.Load(); err != nil {
		return err
	}

	// seed 候选地址
	storedCandidates := c.store.GetCandidates()
	c.candidateAddrsMu.Lock()
	for key, stored := range storedCandidates {
		entry := &AddressEntry{
			Addr:     stored.AddrString,
			Priority: stored.Priority,
			Source:   stored.Source,
			Verified: false,
			LastSeen: time.Unix(stored.LastSeen, 0),
		}
		c.candidateAddrs[key] = entry
		c.updateSourceIndex(stored.Source, key)
	}
	c.candidateAddrsMu.Unlock()

	// seed 已验证地址
	storedVerified := c.store.GetVerified()
	c.verifiedAddrsMu.Lock()
	for key, stored := range storedVerified {
		entry := &AddressEntry{
			Addr:       stored.AddrString,
			Priority:   stored.Priority,
			Source:     stored.Source,
			Verified:   true,
			VerifiedAt: time.Unix(stored.VerifiedAt, 0),
			LastSeen:   time.Unix(stored.LastSeen, 0),
		}
		c.verifiedAddrs[key] = entry
		c.updateSourceIndex(stored.Source, key)
	}
	c.verifiedAddrsMu.Unlock()

	logger.Info("已从存储加载地址",
		"candidates", len(storedCandidates),
		"verified", len(storedVerified))

	return nil
}

// cleanupExpiredLoop 定期清理过期地址
func (c *Coordinator) cleanupExpiredLoop() {
	if c.store == nil {
		return
	}

	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			candidatesRemoved, verifiedRemoved := c.store.CleanExpired()

			if candidatesRemoved > 0 {
				c.candidateAddrsMu.Lock()
				now := time.Now()
				for key, entry := range c.candidateAddrs {
					if now.Sub(entry.LastSeen) > c.config.CandidateTTL {
						delete(c.candidateAddrs, key)
						c.removeFromSourceIndex(entry.Source, key)
					}
				}
				c.candidateAddrsMu.Unlock()
			}

			if verifiedRemoved > 0 {
				c.verifiedAddrsMu.Lock()
				now := time.Now()
				for key, entry := range c.verifiedAddrs {
					if now.Sub(entry.LastSeen) > c.config.VerifiedTTL {
						delete(c.verifiedAddrs, key)
						c.removeFromSourceIndex(entry.Source, key)
					}
				}
				c.verifiedAddrsMu.Unlock()
				c.notifyAddressChanged()
			}
		}
	}
}

// updateSourceIndex 更新来源索引
func (c *Coordinator) updateSourceIndex(source, addrKey string) {
	c.sourceIndexMu.Lock()
	defer c.sourceIndexMu.Unlock()

	if c.sourceIndex[source] == nil {
		c.sourceIndex[source] = make(map[string]struct{})
	}
	c.sourceIndex[source][addrKey] = struct{}{}
}

// removeFromSourceIndex 从来源索引中移除
func (c *Coordinator) removeFromSourceIndex(source, addrKey string) {
	c.sourceIndexMu.Lock()
	defer c.sourceIndexMu.Unlock()

	if c.sourceIndex[source] != nil {
		delete(c.sourceIndex[source], addrKey)
		if len(c.sourceIndex[source]) == 0 {
			delete(c.sourceIndex, source)
		}
	}
}

// filteredVerifiedEntries 返回过滤后的已验证直连地址
//
// 规则：
// - 优先按最近活跃（LastSeen）排序
// - 同一 IP 只保留最新的一条
// - 限制最大数量（防止对外通告过多历史端口）
func (c *Coordinator) filteredVerifiedEntries() []*AddressEntry {
	c.verifiedAddrsMu.RLock()
	entries := make([]*AddressEntry, 0, len(c.verifiedAddrs))
	for _, entry := range c.verifiedAddrs {
		entries = append(entries, entry)
	}
	c.verifiedAddrsMu.RUnlock()

	if len(entries) == 0 {
		return nil
	}

	listenPorts := c.GetListenPorts()
	// 未获取监听端口前，允许已验证的直连地址和用户配置的公网地址
	if len(listenPorts) == 0 {
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Priority != entries[j].Priority {
				return entries[i].Priority > entries[j].Priority
			}
			return entries[i].LastSeen.After(entries[j].LastSeen)
		})

		limit := c.config.MaxVerifiedDirectAddrs
		if limit <= 0 {
			limit = 3
		}

		filtered := make([]*AddressEntry, 0, limit)
		for _, entry := range entries {
			if entry == nil || entry.Addr == "" {
				continue
			}
			// 允许已验证的直连地址（PriorityVerifiedDirect）及更高优先级
			if entry.Priority < interfaces.PriorityVerifiedDirect {
				continue
			}
			filtered = append(filtered, entry)
			if len(filtered) >= limit {
				break
			}
		}
		if len(filtered) == 0 {
			return nil
		}
		return filtered
	}

	portSet := make(map[int]struct{}, len(listenPorts))
	for _, port := range listenPorts {
		if port > 0 {
			portSet[port] = struct{}{}
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Priority != entries[j].Priority {
			return entries[i].Priority > entries[j].Priority
		}
		return entries[i].LastSeen.After(entries[j].LastSeen)
	})

	limit := c.config.MaxVerifiedDirectAddrs
	if limit <= 0 {
		limit = 3
	}

	seenIP := make(map[string]struct{})
	filtered := make([]*AddressEntry, 0, limit)
	for _, entry := range entries {
		if entry == nil || entry.Addr == "" {
			continue
		}

		// 如果已知监听端口，仅保留端口匹配的直连地址
		if len(portSet) > 0 {
			port := extractPortFromMultiaddr(entry.Addr)
			if port == 0 {
				continue
			}
			if _, ok := portSet[port]; !ok {
				continue
			}
		}

		ip := extractIPFromMultiaddr(entry.Addr)
		if ip != "" {
			if _, ok := seenIP[ip]; ok {
				continue
			}
			seenIP[ip] = struct{}{}
		}

		filtered = append(filtered, entry)
		if len(filtered) >= limit {
			break
		}
	}

	return filtered
}

// pruneVerifiedByListenPorts 移除与当前监听端口不匹配的已验证地址
//
// 目的：避免历史端口污染对外通告，确保展示地址真实可达。
func (c *Coordinator) pruneVerifiedByListenPorts() {
	listenPorts := c.GetListenPorts()
	if len(listenPorts) == 0 {
		return
	}

	portSet := make(map[int]struct{}, len(listenPorts))
	for _, port := range listenPorts {
		if port > 0 {
			portSet[port] = struct{}{}
		}
	}

	var removed []string
	c.verifiedAddrsMu.Lock()
	for key, entry := range c.verifiedAddrs {
		if entry == nil || entry.Addr == "" {
			delete(c.verifiedAddrs, key)
			continue
		}
		port := extractPortFromMultiaddr(entry.Addr)
		if port == 0 {
			continue
		}
		if _, ok := portSet[port]; !ok {
			delete(c.verifiedAddrs, key)
			removed = append(removed, entry.Addr)
		}
	}
	c.verifiedAddrsMu.Unlock()

	if len(removed) > 0 {
		if c.store != nil {
			for _, addr := range removed {
				c.store.RemoveVerified(addr)
			}
		}
		logger.Info("已清理历史端口直连地址", "removed", len(removed))
		c.notifyAddressChanged()
	}
}

// extractIPFromMultiaddr 从 multiaddr 提取 IP（ip4/ip6）
func extractIPFromMultiaddr(addr string) string {
	parts := strings.Split(addr, "/")
	for i, part := range parts {
		if (part == "ip4" || part == "ip6") && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// extractPortFromMultiaddr 从 multiaddr 提取端口（udp/tcp）
func extractPortFromMultiaddr(addr string) int {
	parts := strings.Split(addr, "/")
	for i, part := range parts {
		if (part == "udp" || part == "tcp") && i+1 < len(parts) {
			return atoi(parts[i+1])
		}
	}
	return 0
}

// atoi 将字符串端口解析为 int，失败返回 0
func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// replaceAddressesFromSource 按来源替换地址
func (c *Coordinator) replaceAddressesFromSource(source string, newAddrKeys map[string]struct{}) {
	c.sourceIndexMu.RLock()
	oldAddrKeys := make(map[string]struct{})
	if c.sourceIndex[source] != nil {
		for k := range c.sourceIndex[source] {
			oldAddrKeys[k] = struct{}{}
		}
	}
	c.sourceIndexMu.RUnlock()

	for oldKey := range oldAddrKeys {
		if _, exists := newAddrKeys[oldKey]; !exists {
			c.candidateAddrsMu.Lock()
			if entry, ok := c.candidateAddrs[oldKey]; ok {
				delete(c.candidateAddrs, oldKey)
				c.candidateAddrsMu.Unlock()

				if c.store != nil {
					c.store.RemoveCandidate(entry.Addr)
				}
				c.removeFromSourceIndex(source, oldKey)
			} else {
				c.candidateAddrsMu.Unlock()
			}
		}
	}
}
