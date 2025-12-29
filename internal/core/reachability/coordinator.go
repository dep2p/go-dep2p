// Package reachability 提供可达性协调模块的实现
//
// 可达性协调器统一管理地址发布，实现"可达性优先"策略：
// - 先保证连得上（Relay 兜底）
// - 再争取直连更优路径
package reachability

import (
	"context"
	"errors"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/debuglog"
	"github.com/dep2p/go-dep2p/internal/util/logger"
	addressif "github.com/dep2p/go-dep2p/pkg/interfaces/address"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	natif "github.com/dep2p/go-dep2p/pkg/interfaces/nat"
	reachabilityif "github.com/dep2p/go-dep2p/pkg/interfaces/reachability"
	relayif "github.com/dep2p/go-dep2p/pkg/interfaces/relay"
)

// 包级别日志实例
var log = logger.Logger("reachability")

// ============================================================================
//                              Coordinator 结构
// ============================================================================

// NATServicePolicyUpdater NAT 服务策略更新接口
//
// Layer1 修复：用于通知 NAT 服务更新自适应策略状态
type NATServicePolicyUpdater interface {
	UpdateReachabilityState(hasVerifiedDirect bool)
}

// Coordinator 可达性协调器
//
// 统一管理 NAT、Relay、AddressManager 的地址发布，实现"可达性优先"策略。
// 核心职责：
// - 聚合来自不同来源的地址
// - 按优先级排序
// - 在地址变更时通知订阅者
// - 通过 dial-back 验证地址的真实可达性（可选）
// - Layer1: 通知 NAT 服务更新自适应策略状态
type Coordinator struct {
	// 依赖组件
	nat          natif.NATService
	autoRelay    relayif.AutoRelay
	addrManager  addressif.AddressManager

	// dial-back 验证服务（可选）
	dialBackService *DialBackService

	// witness 服务（用于 witness-threshold 验证）
	witnessService *WitnessService

	// endpoint 引用，用于获取连接的节点列表以选择 dial-back 协助节点
	endpoint endpoint.Endpoint

	// Layer1: NAT 策略更新器（可选，用于自适应策略）
	natPolicyUpdater NATServicePolicyUpdater

	// 已验证的直连地址
	verifiedAddrs   map[string]*AddressEntry
	verifiedAddrsMu sync.RWMutex

	// 直连地址候选（尚未通过 dial-back 验证，不对外发布）
	candidateAddrs   map[string]*AddressEntry
	candidateAddrsMu sync.RWMutex

	// Relay 地址
	relayAddrs   []endpoint.Address
	relayAddrsMu sync.RWMutex

	// Witness 证据台账（用于 witness-threshold 验证）
	// key: dialedAddrKey, value: map[witnessKey]*WitnessRecord
	witnessLedger   map[string]map[string]*WitnessRecord
	witnessLedgerMu sync.RWMutex

	// 地址变更回调
	onChange   func([]endpoint.Address)
	onChangeMu sync.RWMutex

	// 运行状态
	ctx       context.Context
	cancel    context.CancelFunc
	running   bool
	runningMu sync.Mutex

	// dial-back 验证配置
	enableDialBack bool // 是否启用 dial-back 验证

	// witness-threshold 配置
	minWitnesses      int // 最小见证数（默认 2）
	witnessIPv4Prefix int // IPv4 去重前缀长度（默认 /24）
	witnessIPv6Prefix int // IPv6 去重前缀长度（默认 /48）

	// 配置的可信 helpers（优先使用）
	trustedHelpers []endpoint.NodeID

	// 验证间隔（定期重验已发布地址）
	verificationInterval time.Duration
}

// WitnessRecord 入站见证记录
type WitnessRecord struct {
	// PeerID 见证者 peer ID
	PeerID endpoint.NodeID

	// RemoteIPPrefix 见证者 IP 前缀（用于去重）
	RemoteIPPrefix string

	// Timestamp 见证时间
	Timestamp time.Time
}

// AddressEntry 地址条目
type AddressEntry struct {
	// Addr 地址
	Addr endpoint.Address

	// Priority 优先级
	Priority addressif.AddressPriority

	// Source 来源（如 "upnp", "stun", "relay"）
	Source string

	// Verified 是否已验证可达
	Verified bool

	// VerifiedAt 验证时间
	VerifiedAt time.Time

	// LastSeen 最后一次确认时间
	LastSeen time.Time
}

// ============================================================================
//                              构造函数
// ============================================================================

// NewCoordinator 创建可达性协调器
func NewCoordinator(
	nat natif.NATService,
	autoRelay relayif.AutoRelay,
	addrManager addressif.AddressManager,
) *Coordinator {
	return &Coordinator{
		nat:               nat,
		autoRelay:         autoRelay,
		addrManager:       addrManager,
		verifiedAddrs:     make(map[string]*AddressEntry),
		candidateAddrs:    make(map[string]*AddressEntry),
		witnessLedger:     make(map[string]map[string]*WitnessRecord),
		minWitnesses:      reachabilityif.DefaultMinWitnesses,
		witnessIPv4Prefix: reachabilityif.DefaultWitnessIPv4Prefix,
		witnessIPv6Prefix: reachabilityif.DefaultWitnessIPv6Prefix,
	}
}

// SetAutoRelay 设置自动中继实例
//
// 设计目的：避免在 fx Provide 阶段注入 auto_relay 造成依赖环。
// 允许在 Coordinator 创建/启动之后再回注入 AutoRelay，并立即绑定回调同步一次 Relay 地址。
func (c *Coordinator) SetAutoRelay(ar relayif.AutoRelay) {
	c.runningMu.Lock()
	c.autoRelay = ar
	c.runningMu.Unlock()

	// 绑定 AutoRelay 的地址变更回调（如果实现了扩展接口）
	type relayAddrsNotifier interface {
		SetOnAddrsChanged(func([]endpoint.Address))
		RelayAddrs() []endpoint.Address
	}
	if n, ok := ar.(relayAddrsNotifier); ok {
		n.SetOnAddrsChanged(c.OnRelayReserved)
		// 同步一次当前 Relay 地址（如果已有）
		c.OnRelayReserved(n.RelayAddrs())
	}
}

// SetEndpoint 设置 endpoint 引用
//
// 用于获取连接的节点列表，以选择 dial-back 协助节点。
// 如果同时设置了 enableDialBack，则会自动创建 DialBackService。
// 同时初始化 WitnessService。
func (c *Coordinator) SetEndpoint(ep endpoint.Endpoint) {
	c.endpoint = ep

	// 初始化 witness 服务
	if c.witnessService == nil && ep != nil {
		c.witnessService = NewWitnessService(c, ep)
	}
}

// SetNATPolicyUpdater 设置 NAT 策略更新器
//
// Layer1 修复：用于在 VerifiedDirect 状态变化时通知 NAT 服务更新自适应策略
func (c *Coordinator) SetNATPolicyUpdater(updater NATServicePolicyUpdater) {
	c.natPolicyUpdater = updater
	log.Debug("NAT 策略更新器已设置")
}

// notifyNATPolicy 通知 NAT 服务更新策略
//
// Layer1 修复：当 VerifiedDirect 状态变化时调用
func (c *Coordinator) notifyNATPolicy() {
	if c.natPolicyUpdater == nil {
		return
	}

	hasVerified := c.HasVerifiedDirectAddress()
	c.natPolicyUpdater.UpdateReachabilityState(hasVerified)

	log.Debug("已通知 NAT 服务更新策略",
		"hasVerifiedDirect", hasVerified)
}

// EnableDialBack 启用 dial-back 验证
//
// 启用后，新发现的直连地址会通过 dial-back 验证其真实可达性。
// 需要先调用 SetEndpoint 设置 endpoint 引用。
func (c *Coordinator) EnableDialBack(enable bool, cfg *reachabilityif.Config) {
	c.enableDialBack = enable

	if cfg != nil {
		c.trustedHelpers = cfg.TrustedHelpers
		c.verificationInterval = cfg.VerificationInterval
	}
	// 使用默认间隔如果未配置
	if c.verificationInterval == 0 {
		c.verificationInterval = 5 * time.Minute
	}

	if enable && c.dialBackService == nil && c.endpoint != nil {
		c.dialBackService = NewDialBackService(c.endpoint, cfg)
	}
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

	// 可达性协调器是常驻组件：不要绑定到 fx OnStart 的短生命周期 ctx。
	// 由 Stop() 负责关闭。
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.running = true

	// 绑定 AutoRelay 的地址变更回调（如果实现了扩展接口）
	type relayAddrsNotifier interface {
		SetOnAddrsChanged(func([]endpoint.Address))
		RelayAddrs() []endpoint.Address
	}
	if ar, ok := c.autoRelay.(relayAddrsNotifier); ok {
		ar.SetOnAddrsChanged(c.OnRelayReserved)
		// 启动时同步一次当前 Relay 地址（如果已有）
		c.OnRelayReserved(ar.RelayAddrs())
	}

	// 启动 dial-back 服务（helper 侧协议响应 + requester 侧发起验证）
	if c.enableDialBack && c.dialBackService != nil {
		_ = c.dialBackService.Start(ctx)
	}

	// 启动 witness 服务（无外部依赖升级路径）
	if c.witnessService != nil {
		_ = c.witnessService.Start(c.ctx)
	}

	// 启动定期重验循环（Phase4: 持续运行、定期重新验证）
	if c.enableDialBack {
		go c.reVerificationLoop()
	}

	log.Info("可达性协调器已启动")
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

	log.Info("可达性协调器已停止")
	return nil
}

// ============================================================================
//                              地址管理
// ============================================================================

// AdvertisedAddrs 返回按优先级排序的通告地址
//
// 可达性优先策略：
// 1. 已验证直连地址（优先级最高）
// 2. Relay 地址（始终保留，保证可达）
// 3. 监听地址（回退）
//
// 并发一致性设计（快照模式）：
//
// 本方法采用"先复制快照、再聚合排序"的策略，而非使用单一大锁覆盖整个操作。
// 设计权衡：
//   - 优点：减少锁持有时间，提高并发吞吐量
//   - 缺点：在两次 RLock 之间，底层数据可能已变化，导致返回的地址列表
//     在逻辑上不是严格的"某一时刻快照"
//
// 为什么可接受：
//   - P2P 场景下，地址列表的轻微不一致（例如 Relay 地址刚被移除但直连地址
//     刚被添加）不会导致功能错误，仅影响连接优化路径的选择
//   - 调用者通常会重试或周期性刷新，最终会收敛到一致状态
//   - 避免了长时间持锁导致的写入饥饿问题
//
// 如需严格一致性，可改用单一 RWMutex 保护所有地址状态，但会降低并发性能。
func (c *Coordinator) AdvertisedAddrs() []endpoint.Address {
	// ======== 快照阶段 ========
	// 分别复制各来源的地址数据，每次持锁时间最短化
	now := time.Now()

	// 快照 1: 已验证直连地址
	c.verifiedAddrsMu.RLock()
	verified := make([]*AddressEntry, 0, len(c.verifiedAddrs))
	for _, entry := range c.verifiedAddrs {
		verified = append(verified, entry)
	}
	c.verifiedAddrsMu.RUnlock()

	// 快照 2: Relay 地址
	c.relayAddrsMu.RLock()
	relayAddrs := make([]endpoint.Address, len(c.relayAddrs))
	copy(relayAddrs, c.relayAddrs)
	c.relayAddrsMu.RUnlock()

	// 快照 3: 监听地址（来自 AddressManager，作为回退）
	var listenAddrs []endpoint.Address
	if c.addrManager != nil {
		listenAddrs = c.addrManager.ListenAddrs()
	}

	// #region agent log
	debuglog.Log(
		"pre-fix",
		"H1",
		"internal/core/reachability/coordinator.go:AdvertisedAddrs",
		"snapshot_counts",
		map[string]any{
			"verifiedLen": len(verified),
			"relayLen":    len(relayAddrs),
			"listenLen":   len(listenAddrs),
		},
	)
	// #endregion agent log

	// ======== 聚合阶段 ========
	// 以下操作无需持锁，因为操作的是本地快照副本
	entries := make([]*AddressEntry, 0, len(verified)+len(relayAddrs)+len(listenAddrs))

	// 1. 已验证直连地址（优先级最高）
	entries = append(entries, verified...)

	// 2. Relay 地址（始终保留，保证可达性兜底）
	for _, addr := range relayAddrs {
		if addr == nil {
			continue
		}
		entries = append(entries, &AddressEntry{
			Addr:     addr,
			Priority: addressif.PriorityRelayGuarantee,
			Source:   "relay",
			Verified: true,
			LastSeen: now,
		})
	}

	// 3. 监听地址仅在无其它地址时作为回退，并过滤 0.0.0.0/:: 等不可拨号地址
	if len(entries) == 0 {
		for _, addr := range listenAddrs {
			if addr == nil || isUnspecifiedAddr(addr) {
				continue
			}
			entries = append(entries, &AddressEntry{
				Addr:     addr,
				Priority: addressif.PriorityLocalListen,
				Source:   "listen",
				Verified: false,
				LastSeen: now,
			})
		}
	}

	// ======== 排序阶段 ========
	// 按优先级排序（高优先级在前），确保直连地址优先于 Relay
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Priority > entries[j].Priority
	})

	// ======== 去重阶段 ========
	// 同一地址可能来自多个来源（例如端口映射和 STUN 都发现了相同的外部地址）
	// 保留优先级最高的那个（因为已排序，第一次出现的优先级最高）
	seen := make(map[string]struct{}, len(entries))
	result := make([]endpoint.Address, 0, len(entries))
	for _, entry := range entries {
		if entry == nil || entry.Addr == nil {
			continue
		}
		key := entry.Addr.String()
		if _, ok := seen[key]; ok {
			continue // 已存在，跳过（保留优先级更高的版本）
		}
		seen[key] = struct{}{}
		result = append(result, entry.Addr)
	}

	// #region agent log
	debuglog.Log(
		"pre-fix",
		"H2",
		"internal/core/reachability/coordinator.go:AdvertisedAddrs",
		"result_count",
		map[string]any{"resultLen": len(result)},
	)
	// #endregion agent log

	return result
}

func isUnspecifiedAddr(addr endpoint.Address) bool {
	if addr == nil {
		return true
	}
	s := addr.String()
	// multiaddr: /ip4/0.0.0.0/... 或 /ip6/::/...
	if containsAny(s, "/ip4/0.0.0.0/", "/ip6/::/") {
		return true
	}
	// host:port
	host, _, err := net.SplitHostPort(s)
	if err != nil {
		return false
	}
	return host == "0.0.0.0" || host == "::" || host == "0:0:0:0:0:0:0:0"
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if len(sub) == 0 {
			continue
		}
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// OnRelayReserved Relay 预留成功回调
//
// 当 AutoRelay 成功预留中继时调用，将 Relay 地址加入发布列表
func (c *Coordinator) OnRelayReserved(addrs []endpoint.Address) {
	c.relayAddrsMu.Lock()
	c.relayAddrs = addrs
	c.relayAddrsMu.Unlock()

	log.Info("Relay 地址已更新",
		"count", len(addrs))

	// 触发地址变更通知
	c.notifyAddressChanged()
}

// OnDirectAddressVerified 直连地址验证成功回调
//
// 当一个候选直连地址通过可达性验证后调用
func (c *Coordinator) OnDirectAddressVerified(addr endpoint.Address, source string, priority addressif.AddressPriority) {
	key := addr.String()
	now := time.Now()

	// 检查是否是第一个验证的直连地址（用于 NAT 策略通知）
	c.verifiedAddrsMu.Lock()
	wasEmpty := len(c.verifiedAddrs) == 0
	c.verifiedAddrs[key] = &AddressEntry{
		Addr:       addr,
		Priority:   priority,
		Source:     source,
		Verified:   true,
		VerifiedAt: now,
		LastSeen:   now,
	}
	c.verifiedAddrsMu.Unlock()

	log.Info("直连地址已验证并发布",
		"addr", addr.String(),
		"source", source,
		"priority", priority.String())

	// 触发地址变更通知
	c.notifyAddressChanged()

	// Layer1: 如果是第一个验证的直连地址，通知 NAT 服务更新策略
	if wasEmpty {
		c.notifyNATPolicy()
	}
}

// OnDirectAddressCandidate 直连地址候选回调（未验证，不对外发布）
//
// 端口映射成功/观测地址等只作为候选输入，真正的发布门槛由 dial-back 验证决定。
func (c *Coordinator) OnDirectAddressCandidate(addr endpoint.Address, source string, priority addressif.AddressPriority) {
	if addr == nil {
		return
	}
	key := addr.String()
	now := time.Now()

	c.candidateAddrsMu.Lock()
	c.candidateAddrs[key] = &AddressEntry{
		Addr:     addr,
		Priority: priority,
		Source:   source,
		Verified: false,
		LastSeen: now,
	}
	c.candidateAddrsMu.Unlock()

	log.Debug("直连地址候选已上报（未验证）",
		"addr", addr.String(),
		"source", source,
		"priority", priority.String())

	// 启用 dial-back 时：异步触发验证，通过后才进入 verifiedAddrs 并对外发布
	if c.enableDialBack && c.dialBackService != nil && c.endpoint != nil {
		go func(a endpoint.Address) {
			verifyCtx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
			defer cancel()
			_, _ = c.VerifyAddressWithDialBack(verifyCtx, []endpoint.Address{a})
		}(addr)
	}
}

// OnDirectAddressExpired 直连地址过期/失效
//
// 当一个直连地址不再可达时调用
func (c *Coordinator) OnDirectAddressExpired(addr endpoint.Address) {
	key := addr.String()

	c.verifiedAddrsMu.Lock()
	delete(c.verifiedAddrs, key)
	nowEmpty := len(c.verifiedAddrs) == 0
	c.verifiedAddrsMu.Unlock()

	log.Info("直连地址已过期",
		"addr", addr.String())

	// 触发地址变更通知
	c.notifyAddressChanged()

	// Layer1: 如果没有验证的直连地址了，通知 NAT 服务恢复探测
	if nowEmpty {
		c.notifyNATPolicy()
	}
}

// ============================================================================
//                              回调注册
// ============================================================================

// SetOnAddressChanged 设置地址变更回调
//
// 当地址列表发生变化时（新增/移除/优先级变化），会调用此回调
func (c *Coordinator) SetOnAddressChanged(callback func([]endpoint.Address)) {
	c.onChangeMu.Lock()
	c.onChange = callback
	c.onChangeMu.Unlock()
}

// notifyAddressChanged 通知地址变更
func (c *Coordinator) notifyAddressChanged() {
	c.onChangeMu.RLock()
	callback := c.onChange
	c.onChangeMu.RUnlock()

	if callback != nil {
		addrs := c.AdvertisedAddrs()
		// 异步调用，避免阻塞
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

// VerifiedDirectAddresses 返回所有已验证的直连地址
func (c *Coordinator) VerifiedDirectAddresses() []endpoint.Address {
	c.verifiedAddrsMu.RLock()
	defer c.verifiedAddrsMu.RUnlock()

	result := make([]endpoint.Address, 0, len(c.verifiedAddrs))
	for _, entry := range c.verifiedAddrs {
		result = append(result, entry.Addr)
	}
	return result
}

// RelayAddresses 返回所有 Relay 地址
func (c *Coordinator) RelayAddresses() []endpoint.Address {
	c.relayAddrsMu.RLock()
	defer c.relayAddrsMu.RUnlock()

	result := make([]endpoint.Address, len(c.relayAddrs))
	copy(result, c.relayAddrs)
	return result
}

// BootstrapCandidates 返回可用于冷启动尝试的候选地址列表（旁路/非严格）
//
// 包含直连候选 + relay 候选，不保证可达，仅供跨设备试连。
// MUST NOT 用于 DHT 发布，不等同于 ShareableAddrs。
func (c *Coordinator) BootstrapCandidates(nodeID endpoint.NodeID) []reachabilityif.BootstrapCandidate {
	var result []reachabilityif.BootstrapCandidate
	seen := make(map[string]bool)

	// 1. 已验证的直连地址（标记 Verified=true）
	c.verifiedAddrsMu.RLock()
	for _, entry := range c.verifiedAddrs {
		if entry.Addr == nil {
			continue
		}
		fullAddr := entry.Addr.String() + "/p2p/" + nodeID.String()
		if seen[fullAddr] {
			continue
		}
		seen[fullAddr] = true

		result = append(result, reachabilityif.BootstrapCandidate{
			FullAddr:   fullAddr,
			Kind:       reachabilityif.CandidateKindDirect,
			Source:     entry.Source,
			Confidence: reachabilityif.ConfidenceHigh,
			Verified:   true,
			Notes:      "已验证直连地址",
		})
	}
	c.verifiedAddrsMu.RUnlock()

	// 2. 未验证的直连候选（标记 Verified=false）
	c.candidateAddrsMu.RLock()
	for _, entry := range c.candidateAddrs {
		if entry.Addr == nil {
			continue
		}
		fullAddr := entry.Addr.String() + "/p2p/" + nodeID.String()
		if seen[fullAddr] {
			continue
		}
		seen[fullAddr] = true

		// 根据来源确定置信度
		confidence := reachabilityif.ConfidenceLow
		switch entry.Source {
		case "listen-bound-public", "cloud-metadata":
			confidence = reachabilityif.ConfidenceMedium
		case "user-config", "configured-external":
			confidence = reachabilityif.ConfidenceMedium
		}

		result = append(result, reachabilityif.BootstrapCandidate{
			FullAddr:   fullAddr,
			Kind:       reachabilityif.CandidateKindDirect,
			Source:     entry.Source,
			Confidence: confidence,
			Verified:   false,
			Notes:      "待验证候选",
		})
	}
	c.candidateAddrsMu.RUnlock()

	// 3. Relay 地址（标记 Kind=relay）
	c.relayAddrsMu.RLock()
	for _, addr := range c.relayAddrs {
		if addr == nil {
			continue
		}
		addrStr := addr.String()
		// Relay 地址可能已包含 /p2p/<selfID>，检查一下
		fullAddr := addrStr
		if !strings.Contains(addrStr, "/p2p/"+nodeID.String()) {
			fullAddr = addrStr + "/p2p/" + nodeID.String()
		}
		if seen[fullAddr] {
			continue
		}
		seen[fullAddr] = true

		result = append(result, reachabilityif.BootstrapCandidate{
			FullAddr:   fullAddr,
			Kind:       reachabilityif.CandidateKindRelay,
			Source:     "relay",
			Confidence: reachabilityif.ConfidenceMedium,
			Verified:   false,
			Notes:      "中继候选（不入 DHT）",
		})
	}
	c.relayAddrsMu.RUnlock()

	return result
}

// ============================================================================
//                              Witness 验证（无外部依赖）
// ============================================================================

// OnInboundWitness 上报入站见证（无外部依赖升级路径）
//
// 当 peer 使用候选地址成功连入后，调用此方法记录见证。
// 达到阈值后自动升级为 VerifiedDirect。
func (c *Coordinator) OnInboundWitness(dialedAddr string, remotePeerID endpoint.NodeID, remoteIP string) {
	if dialedAddr == "" || remotePeerID.IsEmpty() || remoteIP == "" {
		return
	}

	dialedAddrKey := dialedAddr

	// 检查是否是我们的候选地址
	c.candidateAddrsMu.RLock()
	entry, isCandidate := c.candidateAddrs[dialedAddrKey]
	c.candidateAddrsMu.RUnlock()

	if !isCandidate {
		log.Debug("witness: 地址不在候选列表中，忽略",
			"dialedAddr", dialedAddrKey,
			"remotePeer", remotePeerID.ShortString())
		return
	}

	// 计算 IP 前缀
	ipPrefix := c.getIPPrefix(remoteIP)
	if ipPrefix == "" {
		log.Debug("witness: 无法解析 IP 前缀",
			"remoteIP", remoteIP)
		return
	}

	// 生成去重键：peerID + IP前缀
	witnessKey := remotePeerID.String() + "|" + ipPrefix

	c.witnessLedgerMu.Lock()
	defer c.witnessLedgerMu.Unlock()

	// 初始化该地址的见证 map
	if c.witnessLedger[dialedAddrKey] == nil {
		c.witnessLedger[dialedAddrKey] = make(map[string]*WitnessRecord)
	}

	// 检查是否已记录过（同一 peer + 同一 IP 前缀）
	if _, exists := c.witnessLedger[dialedAddrKey][witnessKey]; exists {
		log.Debug("witness: 重复见证，忽略",
			"dialedAddr", dialedAddrKey,
			"remotePeer", remotePeerID.ShortString(),
			"ipPrefix", ipPrefix)
		return
	}

	// 记录见证
	c.witnessLedger[dialedAddrKey][witnessKey] = &WitnessRecord{
		PeerID:         remotePeerID,
		RemoteIPPrefix: ipPrefix,
		Timestamp:      time.Now(),
	}

	// 统计不同 IP 前缀的见证数
	uniquePrefixes := make(map[string]bool)
	for _, record := range c.witnessLedger[dialedAddrKey] {
		uniquePrefixes[record.RemoteIPPrefix] = true
	}
	witnessCount := len(uniquePrefixes)

	log.Info("witness: 记录入站见证",
		"dialedAddr", dialedAddrKey,
		"remotePeer", remotePeerID.ShortString(),
		"ipPrefix", ipPrefix,
		"witnessCount", witnessCount,
		"minWitnesses", c.minWitnesses)

	// 检查是否达到阈值
	if witnessCount >= c.minWitnesses {
		log.Info("witness: 达到阈值，升级为 VerifiedDirect",
			"dialedAddr", dialedAddrKey,
			"witnessCount", witnessCount)

		// 升级为 VerifiedDirect
		c.OnDirectAddressVerified(entry.Addr, "witness-threshold", addressif.PriorityVerifiedDirect)
	}
}

// getIPPrefix 计算 IP 前缀
func (c *Coordinator) getIPPrefix(ipStr string) string {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return ""
	}

	if ip4 := ip.To4(); ip4 != nil {
		// IPv4: 使用配置的前缀长度（默认 /24）
		mask := net.CIDRMask(c.witnessIPv4Prefix, 32)
		network := ip4.Mask(mask)
		_, ipNet, _ := net.ParseCIDR(network.String() + "/" + itoa(c.witnessIPv4Prefix))
		if ipNet != nil {
			return ipNet.String()
		}
		return network.String()
	}

	// IPv6: 使用配置的前缀长度（默认 /48）
	mask := net.CIDRMask(c.witnessIPv6Prefix, 128)
	network := ip.Mask(mask)
	_, ipNet, _ := net.ParseCIDR(network.String() + "/" + itoa(c.witnessIPv6Prefix))
	if ipNet != nil {
		return ipNet.String()
	}
	return network.String()
}

// itoa 简单的 int 转 string
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

// OnOutboundConnected 上报出站连接成功事件
//
// 用于触发 witness 报告发送（无外部依赖升级路径）。
func (c *Coordinator) OnOutboundConnected(conn endpoint.Connection, dialedAddr string) {
	if conn == nil || dialedAddr == "" {
		return
	}
	if c.witnessService == nil {
		return
	}
	// 异步发送，避免影响主拨号路径
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = c.witnessService.SendWitnessReport(ctx, conn, dialedAddr)
	}()
}

// ============================================================================
//                              Dial-Back 验证
// ============================================================================

// VerifyAddressWithDialBack 使用 dial-back 验证地址的可达性
//
// 选择一个已连接的节点作为协助方，请求其回拨验证候选地址。
// 如果验证成功，地址会被添加到已验证直连地址列表。
//
// 参数：
//   - ctx: 上下文
//   - candidateAddrs: 待验证的候选地址
//
// 返回：
//   - reachable: 验证可达的地址列表
//   - err: 错误信息
func (c *Coordinator) VerifyAddressWithDialBack(ctx context.Context, candidateAddrs []endpoint.Address) ([]endpoint.Address, error) {
	if !c.enableDialBack || c.dialBackService == nil {
		return nil, ErrDialBackDisabled
	}

	if c.endpoint == nil {
		return nil, ErrNoEndpoint
	}

	// 执行 dial-back 验证：混合 helper（配置优先 + connected peers 退化）
	reachable, err := c.dialBackService.VerifyAddressesWithHelperPool(
		ctx,
		c.trustedHelpers,
		c.endpoint.Connections(),
		candidateAddrs,
	)
	if err != nil {
		return nil, err
	}

	// 将验证成功的地址添加到已验证列表
	for _, addr := range reachable {
		c.OnDirectAddressVerified(addr, "dial-back", addressif.PriorityVerifiedDirect)
	}

	return reachable, nil
}

// GetDialBackService 获取 dial-back 服务实例
//
// 返回 nil 如果 dial-back 未启用或未初始化
func (c *Coordinator) GetDialBackService() *DialBackService {
	return c.dialBackService
}

// DialBackEnabled 返回 dial-back 验证是否启用
func (c *Coordinator) DialBackEnabled() bool {
	return c.enableDialBack
}

// reVerificationLoop 定期重验已发布地址的可达性
//
// Phase4 要求：持续运行、定期重新验证已发布地址，
// 及时发现地址失效并从发布列表中移除。
func (c *Coordinator) reVerificationLoop() {
	interval := c.verificationInterval
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
	if !c.enableDialBack || c.dialBackService == nil || c.endpoint == nil {
		return
	}

	// 获取当前已验证地址的快照
	c.verifiedAddrsMu.RLock()
	addrs := make([]endpoint.Address, 0, len(c.verifiedAddrs))
	for _, entry := range c.verifiedAddrs {
		addrs = append(addrs, entry.Addr)
	}
	c.verifiedAddrsMu.RUnlock()

	if len(addrs) == 0 {
		return
	}

	log.Debug("开始定期重验已发布地址", "count", len(addrs))

	// 使用超时上下文进行重验
	ctx, cancel := context.WithTimeout(c.ctx, 60*time.Second)
	defer cancel()

	// 重新验证
	reachable, err := c.dialBackService.VerifyAddressesWithHelperPool(
		ctx,
		c.trustedHelpers,
		c.endpoint.Connections(),
		addrs,
	)
	if err != nil {
		log.Debug("定期重验失败", "err", err)
		return
	}

	// 构建可达地址集合
	reachableSet := make(map[string]struct{}, len(reachable))
	for _, addr := range reachable {
		reachableSet[addr.String()] = struct{}{}
	}

	// 移除不可达的地址
	c.verifiedAddrsMu.Lock()
	for key, entry := range c.verifiedAddrs {
		if _, ok := reachableSet[key]; !ok {
			// 地址不再可达，移除
			delete(c.verifiedAddrs, key)
			log.Info("定期重验：地址不可达，已移除",
				"addr", entry.Addr.String(),
				"source", entry.Source)
		} else {
			// 更新 LastSeen
			entry.LastSeen = time.Now()
		}
	}
	c.verifiedAddrsMu.Unlock()

	// 如果有地址被移除，触发变更通知
	if len(reachable) < len(addrs) {
		c.notifyAddressChanged()
	}

	log.Debug("定期重验完成",
		"verified", len(addrs),
		"still_reachable", len(reachable))
}

// ErrDialBackDisabled dial-back 验证未启用
var ErrDialBackDisabled = errors.New("dial-back verification is disabled")

// ErrNoEndpoint 未设置 endpoint
var ErrNoEndpoint = errors.New("endpoint not set")

