// Package dht 提供分布式哈希表实现
//
// 本文件实现 DHT 双向操作特性中的可达性验证：
// - 定义 ReachabilityChecker 接口（依赖倒置）
// - 实现发布前可达性验证
// - 不可达时自动降级到 relay_addrs
// - 实现动态 TTL 调整（基于 NAT 类型）
package dht

import (
	"context"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              可达性检测接口（依赖倒置）
// ============================================================================

// ReachabilityChecker 可达性检测器接口
//
// 用于在发布 PeerRecord 前验证地址可达性。
// 这是依赖倒置的核心接口，DHT 不直接依赖 AutoNAT 或 DialBack 实现。
type ReachabilityChecker interface {
	// CheckReachability 检查当前节点的可达性状态
	//
	// 返回:
	//   - reachability: 可达性状态
	//   - natType: NAT 类型
	CheckReachability(ctx context.Context) (reachability types.Reachability, natType types.NATType)

	// VerifyAddresses 验证地址列表的可达性
	//
	// 参数:
	//   - ctx: 上下文
	//   - addrs: 待验证的地址列表
	//
	// 返回:
	//   - verified: 已验证可达的地址列表
	//   - unverified: 未验证或不可达的地址列表
	VerifyAddresses(ctx context.Context, addrs []string) (verified, unverified []string)

	// GetVerifiedDirectAddresses 获取已验证的直连地址
	//
	// 返回经过 AutoNAT/DialBack 验证的公网可达地址
	GetVerifiedDirectAddresses(ctx context.Context) []string

	// IsDirectlyReachable 检查是否直连可达
	//
	// 返回 true 表示至少有一个直连地址经过验证
	IsDirectlyReachable(ctx context.Context) bool
}

// ============================================================================
//                              默认实现（基于配置）
// ============================================================================

// DefaultReachabilityChecker 默认可达性检测器
//
// 基于配置的 ReachabilityProvider 实现基本的可达性检测。
// 生产环境建议替换为 AutoNAT/DialBack 集成实现。
type DefaultReachabilityChecker struct {
	// reachabilityProvider 可达性提供器（来自配置）
	reachabilityProvider func() types.NATType

	// verifiedAddrs 已验证的地址列表（缓存）
	verifiedAddrs []string

	// lastCheck 最后检测时间
	lastCheck time.Time

	// cacheDuration 缓存有效期
	cacheDuration time.Duration
}

// NewDefaultReachabilityChecker 创建默认可达性检测器
func NewDefaultReachabilityChecker(provider func() types.NATType) *DefaultReachabilityChecker {
	return &DefaultReachabilityChecker{
		reachabilityProvider: provider,
		cacheDuration:        5 * time.Minute,
	}
}

// CheckReachability 检查可达性状态
func (c *DefaultReachabilityChecker) CheckReachability(_ context.Context) (types.Reachability, types.NATType) {
	if c.reachabilityProvider == nil {
		return types.ReachabilityUnknown, types.NATTypeUnknown
	}

	natType := c.reachabilityProvider()

	// 根据 NAT 类型推断可达性
	switch natType {
	case types.NATTypeNone, types.NATTypeFullCone:
		return types.ReachabilityPublic, natType
	case types.NATTypeRestrictedCone, types.NATTypePortRestricted:
		// 受限锥形 NAT 可能可以穿透，但不确定
		return types.ReachabilityUnknown, natType
	case types.NATTypeSymmetric:
		// 对称型 NAT 通常不可直连
		return types.ReachabilityPrivate, natType
	default:
		return types.ReachabilityUnknown, natType
	}
}

// VerifyAddresses 验证地址可达性
func (c *DefaultReachabilityChecker) VerifyAddresses(ctx context.Context, addrs []string) ([]string, []string) {
	reachability, _ := c.CheckReachability(ctx)

	// 如果是 Public，所有非 Relay 地址都视为已验证
	if reachability == types.ReachabilityPublic {
		var verified, unverified []string
		for _, addr := range addrs {
			if isRelayAddr(addr) {
				// Relay 地址始终可用
				verified = append(verified, addr)
			} else {
				// 直连地址视为已验证
				verified = append(verified, addr)
			}
		}
		return verified, unverified
	}

	// 如果是 Private，只有 Relay 地址视为可达
	if reachability == types.ReachabilityPrivate {
		var verified, unverified []string
		for _, addr := range addrs {
			if isRelayAddr(addr) {
				verified = append(verified, addr)
			} else {
				unverified = append(unverified, addr)
			}
		}
		return verified, unverified
	}

	// Unknown 状态，保守处理
	var verified, unverified []string
	for _, addr := range addrs {
		if isRelayAddr(addr) {
			verified = append(verified, addr)
		} else {
			// 直连地址标记为未验证，但仍然尝试发布
			unverified = append(unverified, addr)
		}
	}
	return verified, unverified
}

// GetVerifiedDirectAddresses 获取已验证的直连地址
func (c *DefaultReachabilityChecker) GetVerifiedDirectAddresses(ctx context.Context) []string {
	reachability, _ := c.CheckReachability(ctx)
	if reachability == types.ReachabilityPublic {
		return c.verifiedAddrs
	}
	return nil
}

// IsDirectlyReachable 检查是否直连可达
func (c *DefaultReachabilityChecker) IsDirectlyReachable(ctx context.Context) bool {
	reachability, _ := c.CheckReachability(ctx)
	return reachability == types.ReachabilityPublic
}

// ============================================================================
//                              动态 TTL 计算
// ============================================================================

// DynamicTTLCalculator 动态 TTL 计算器
//
// 根据 NAT 类型和网络状况动态调整 PeerRecord TTL
type DynamicTTLCalculator struct {
	// baseTTL 基础 TTL（默认 1 小时）
	baseTTL time.Duration

	// minTTL 最小 TTL（默认 15 分钟）
	minTTL time.Duration

	// maxTTL 最大 TTL（默认 24 小时）
	maxTTL time.Duration
}

// NewDynamicTTLCalculator 创建动态 TTL 计算器
func NewDynamicTTLCalculator() *DynamicTTLCalculator {
	return &DynamicTTLCalculator{
		baseTTL: DefaultPeerRecordTTL,
		minTTL:  MinPeerRecordTTL,
		maxTTL:  MaxPeerRecordTTL,
	}
}

// CalculateTTL 根据 NAT 类型计算 TTL
//
// 策略：
//   - Public/None/FullCone: 使用较长 TTL（地址稳定）
//   - RestrictedCone/PortRestricted: 使用中等 TTL
//   - Symmetric/Unknown: 使用较短 TTL（地址可能频繁变化）
func (c *DynamicTTLCalculator) CalculateTTL(natType types.NATType, addressChangeFrequency float64) time.Duration {
	var ttl time.Duration

	switch natType {
	case types.NATTypeNone:
		// 公网节点，地址最稳定
		ttl = c.maxTTL
	case types.NATTypeFullCone:
		// 完全锥形 NAT，地址比较稳定
		ttl = c.baseTTL * 2
	case types.NATTypeRestrictedCone, types.NATTypePortRestricted:
		// 受限锥形 NAT，使用基础 TTL
		ttl = c.baseTTL
	case types.NATTypeSymmetric:
		// 对称型 NAT，地址可能频繁变化
		ttl = c.baseTTL / 2
	default:
		// 未知类型，使用较短 TTL
		ttl = c.baseTTL / 2
	}

	// 根据地址变化频率调整
	// addressChangeFrequency: 每小时地址变化次数
	if addressChangeFrequency > 2 {
		// 频繁变化，缩短 TTL
		ttl = ttl / 2
	} else if addressChangeFrequency > 0.5 {
		// 偶尔变化，略微缩短
		ttl = ttl * 3 / 4
	}

	// 确保在有效范围内
	if ttl < c.minTTL {
		ttl = c.minTTL
	}
	if ttl > c.maxTTL {
		ttl = c.maxTTL
	}

	return ttl
}

// ============================================================================
//                              发布策略
// ============================================================================

// PublishDecision 发布决策
type PublishDecision struct {
	// ShouldPublish 是否应该发布
	ShouldPublish bool

	// DirectAddrs 应发布的直连地址
	DirectAddrs []string

	// RelayAddrs 应发布的中继地址
	RelayAddrs []string

	// TTL 建议的 TTL
	TTL time.Duration

	// Reason 决策原因
	Reason string

	// Warnings 警告信息
	Warnings []string
}

// MakePublishDecision 做出发布决策
//
// 根据可达性状态和配置决定应该发布哪些地址
func (d *DHT) MakePublishDecision(ctx context.Context, allAddrs []string) *PublishDecision {
	decision := &PublishDecision{
		ShouldPublish: true,
		TTL:           d.config.PeerRecordTTL,
	}

	// 分类地址
	directAddrs := filterDirectAddrs(allAddrs)
	relayAddrs := filterRelayAddrs(allAddrs)

	// 获取可达性状态
	var reachability types.Reachability
	var natType types.NATType

	if d.reachabilityChecker != nil {
		reachability, natType = d.reachabilityChecker.CheckReachability(ctx)
	} else if d.config.ReachabilityProvider != nil {
		natType = d.config.ReachabilityProvider()
		if isPrivateNAT(natType) {
			reachability = types.ReachabilityPrivate
		} else {
			reachability = types.ReachabilityPublic
		}
	} else {
		reachability = types.ReachabilityUnknown
		natType = types.NATTypeUnknown
	}

	// 根据策略和可达性决定发布内容
	switch d.config.AddressPublishStrategy {
	case PublishAll:
		decision.DirectAddrs = directAddrs
		decision.RelayAddrs = relayAddrs
		decision.Reason = "strategy: publish all"

	case PublishDirectOnly:
		if reachability == types.ReachabilityPrivate {
			decision.Warnings = append(decision.Warnings,
				"direct-only strategy but node is private, addresses may be unreachable")
		}
		decision.DirectAddrs = directAddrs
		decision.Reason = "strategy: direct only"

	case PublishRelayOnly:
		decision.RelayAddrs = relayAddrs
		decision.Reason = "strategy: relay only"

	case PublishAuto:
		fallthrough
	default:
		// 自动策略：根据可达性决定
		switch reachability {
		case types.ReachabilityPublic:
			// 公网可达，发布所有地址
			decision.DirectAddrs = directAddrs
			decision.RelayAddrs = relayAddrs
			decision.Reason = "auto: public reachability, publishing all"

		case types.ReachabilityPrivate:
			// 私网，仅发布 Relay 地址
			decision.RelayAddrs = relayAddrs
			if len(relayAddrs) == 0 {
				decision.Warnings = append(decision.Warnings,
					"private node without relay addresses, may be unreachable")
			}
			decision.Reason = "auto: private reachability, relay only"

		default:
			// 未知状态，保守发布 Relay + 部分 Direct
			decision.RelayAddrs = relayAddrs
			// 如果有可达性检测器，使用已验证的直连地址
			if d.reachabilityChecker != nil {
				verifiedDirect := d.reachabilityChecker.GetVerifiedDirectAddresses(ctx)
				decision.DirectAddrs = verifiedDirect
			}
			decision.Reason = "auto: unknown reachability, conservative publish"
		}
	}

	// 计算动态 TTL
	if d.config.PeerRecordTTL == 0 || d.config.PeerRecordTTL == DefaultPeerRecordTTL {
		calculator := NewDynamicTTLCalculator()
		decision.TTL = calculator.CalculateTTL(natType, 0)
	}

	// 检查是否有可发布的地址
	if len(decision.DirectAddrs) == 0 && len(decision.RelayAddrs) == 0 {
		decision.ShouldPublish = false
		decision.Reason = "no publishable addresses"
	}

	return decision
}

// ============================================================================
//                              DHT 集成方法
// ============================================================================

// SetReachabilityChecker 设置可达性检测器
func (d *DHT) SetReachabilityChecker(checker ReachabilityChecker) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.reachabilityChecker = checker
}

// SetAddressBookProvider 设置地址簿提供者
func (d *DHT) SetAddressBookProvider(provider AddressBookProvider) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.addressBookProvider = provider
}

// PublishLocalPeerRecordWithVerification 带可达性验证的本地 PeerRecord 发布
//
// v2.0 新增：发布前验证地址可达性
//
// 流程：
//  1. 收集当前地址
//  2. 根据可达性状态过滤地址
//  3. 计算动态 TTL
//  4. 创建签名记录并发布
func (d *DHT) PublishLocalPeerRecordWithVerification(ctx context.Context) (*PublishDecision, error) {
	if !d.started.Load() {
		return nil, ErrNotStarted
	}

	if d.localRecordManager == nil || !d.localRecordManager.IsInitialized() {
		return nil, ErrNilPeerRecord
	}

	// 1. 获取当前地址（使用 AdvertisedAddrs 包含 Relay 地址）
	//之前使用 Addrs() 只返回监听地址，不包含 Relay 地址
	allAddrs := d.host.AdvertisedAddrs()

	// 2. 做出发布决策
	decision := d.MakePublishDecision(ctx, allAddrs)
	if !decision.ShouldPublish {
		// 暂无可发布地址是启动阶段的正常状态，降级到 DEBUG
		logger.Debug("跳过 PeerRecord 发布（等待地址就绪）", "reason", decision.Reason)
		return decision, nil
	}

	// 3. 获取可达性信息
	var natType types.NATType
	var reachability types.Reachability

	if d.reachabilityChecker != nil {
		reachability, natType = d.reachabilityChecker.CheckReachability(ctx)
	} else if d.config.ReachabilityProvider != nil {
		natType = d.config.ReachabilityProvider()
		if isPrivateNAT(natType) {
			reachability = types.ReachabilityPrivate
		} else {
			reachability = types.ReachabilityPublic
		}
	}

	// 4. 构建能力列表
	capabilities := d.buildCapabilities()

	// 5. 创建签名记录
	signed, err := d.localRecordManager.CreateSignedRecord(
		decision.DirectAddrs,
		decision.RelayAddrs,
		natType,
		reachability,
		capabilities,
		decision.TTL,
	)
	if err != nil {
		return decision, err
	}

	// 6. 发布到 DHT
	//使用内部方法
	if err := d.publishPeerRecordInternal(ctx, signed); err != nil {
		return decision, err
	}

	logger.Info("PeerRecord 发布成功（带验证）",
		"directCount", len(decision.DirectAddrs),
		"relayCount", len(decision.RelayAddrs),
		"ttl", decision.TTL,
		"reason", decision.Reason)

	return decision, nil
}
