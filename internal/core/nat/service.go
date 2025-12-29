// Package nat 提供 NAT 穿透模块的实现
package nat

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/nat/holepunch"
	"github.com/dep2p/go-dep2p/internal/core/nat/http"
	"github.com/dep2p/go-dep2p/internal/core/nat/natpmp"
	"github.com/dep2p/go-dep2p/internal/core/nat/stun"
	"github.com/dep2p/go-dep2p/internal/core/nat/upnp"
	"github.com/dep2p/go-dep2p/internal/util/logger"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	natif "github.com/dep2p/go-dep2p/pkg/interfaces/nat"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 包级别日志实例
var log = logger.Logger("nat")

// ============================================================================
//                              Service 结构
// ============================================================================

// Service NAT 服务聚合实现
type Service struct {
	config natif.Config

	// IP 发现器（按优先级排序）
	discoverers []natif.IPDiscoverer

	// 端口映射器
	mappers []natif.PortMapper

	// STUN 客户端
	stunClient *stun.Client

	// 打洞器
	holePuncher    *holepunch.Puncher
	tcpHolePuncher *holepunch.TCPPuncher

	// 缓存的 NAT 信息
	natType      types.NATType
	externalAddr endpoint.Address
	cacheMu      sync.RWMutex
	cacheTime    time.Time

	// 活跃映射
	mappings   map[string]*natif.Mapping // key: "protocol:internalPort"
	mappingsMu sync.RWMutex

	// 刷新定时器
	refreshTicker *time.Ticker
	stopCh        chan struct{}

	// Layer1: 自适应策略状态
	verifiedDirect    bool      // 是否已有验证的直连地址
	verifiedDirectMu  sync.RWMutex
	lastPolicyUpdate  time.Time
	policyUpdateCount int64

	// 状态
	closed    bool
	closeOnce sync.Once
	mu        sync.RWMutex
}

// 确保实现接口
var _ natif.NATService = (*Service)(nil)

// ============================================================================
//                              构造函数
// ============================================================================

// NewService 创建 NAT 服务
func NewService(config natif.Config) *Service {
	s := &Service{
		config:   config,
		mappings: make(map[string]*natif.Mapping),
		stopCh:   make(chan struct{}),
	}

	// 初始化发现器
	s.initDiscoverers()

	// 初始化映射器
	s.initMappers()

	// 初始化打洞器
	s.initHolePuncher()

	// 启动刷新循环
	if config.MappingRefreshInterval > 0 {
		s.refreshTicker = time.NewTicker(config.MappingRefreshInterval)
		go s.refreshLoop()
	}

	log.Info("NAT 服务已初始化",
		"policyMode", config.PolicyMode.String(),
		"stunEnabled", config.EnableSTUN,
		"upnpEnabled", config.EnableUPnP,
		"natpmpEnabled", config.EnableNATPMP,
		"httpIPEnabled", config.EnableHTTPIPServices,
		"stunServers", len(config.STUNServers),
		"mappers", len(s.mappers))

	return s
}

// ============================================================================
//                              初始化方法
// ============================================================================

// initDiscoverers 初始化 IP 发现器
//
// Layer1 修复：根据 PolicyMode 和 EnableHTTPIPServices 决定是否添加 HTTP 发现器
func (s *Service) initDiscoverers() {
	s.discoverers = make([]natif.IPDiscoverer, 0)

	// STUN 发现器（优先级最高）
	if s.config.EnableSTUN {
		servers := s.config.STUNServers
		if len(servers) == 0 {
			servers = stun.DefaultServers()
		}
		s.stunClient = stun.NewClient(servers)
		s.discoverers = append(s.discoverers, &stunDiscoverer{client: s.stunClient})
		log.Debug("已添加 STUN 发现器", "servers", len(servers))
	}

	// HTTP 发现器（备用）- Layer1 修复：根据配置决定是否添加
	// 隐私模式或未显式启用时，不添加 HTTP 发现器
	if s.config.PolicyMode != natif.PolicyModePrivacy && s.config.EnableHTTPIPServices {
		if len(s.config.HTTPIPServices) > 0 {
			httpDisc := http.NewDiscoverer(s.config.HTTPIPServices)
			s.discoverers = append(s.discoverers, httpDisc)
			log.Debug("已添加 HTTP IP 发现器", "services", len(s.config.HTTPIPServices))
			log.Warn("HTTP IP 发现器已启用：将向第三方服务发送请求以获取外网 IP",
				"services", s.config.HTTPIPServices)
		}
	} else {
		log.Debug("HTTP IP 发现器未启用",
			"policyMode", s.config.PolicyMode.String(),
			"enableHTTPIPServices", s.config.EnableHTTPIPServices)
	}

	// 按优先级排序
	sort.Slice(s.discoverers, func(i, j int) bool {
		return s.discoverers[i].Priority() < s.discoverers[j].Priority()
	})
}

// initMappers 初始化端口映射器
func (s *Service) initMappers() {
	s.mappers = make([]natif.PortMapper, 0)

	// UPnP 映射器
	if s.config.EnableUPnP {
		upnpMapper := upnp.NewMapper()
		s.mappers = append(s.mappers, upnpMapper)
		log.Debug("已添加 UPnP 映射器")
	}

	// NAT-PMP 映射器
	if s.config.EnableNATPMP {
		natpmpMapper := natpmp.NewMapper()
		s.mappers = append(s.mappers, natpmpMapper)
		log.Debug("已添加 NAT-PMP 映射器")
	}
}

// initHolePuncher 初始化打洞器
func (s *Service) initHolePuncher() {
	// UDP 打洞器
	udpConfig := holepunch.DefaultConfig()
	udpConfig.Timeout = s.config.PunchTimeout
	s.holePuncher = holepunch.NewPuncher(udpConfig)

	// TCP 打洞器
	tcpConfig := holepunch.DefaultTCPConfig()
	tcpConfig.Timeout = s.config.PunchTimeout
	s.tcpHolePuncher = holepunch.NewTCPPuncher(tcpConfig)
}

// ============================================================================
//                              NATService 接口实现
// ============================================================================

// GetExternalAddress 获取外部地址
func (s *Service) GetExternalAddress() (endpoint.Address, error) {
	return s.GetExternalAddressWithContext(context.Background())
}

// GetExternalAddressWithContext 带上下文的外部地址获取
func (s *Service) GetExternalAddressWithContext(ctx context.Context) (endpoint.Address, error) {
	// 检查缓存
	s.cacheMu.RLock()
	if s.externalAddr != nil && time.Since(s.cacheTime) < 5*time.Minute {
		addr := s.externalAddr
		s.cacheMu.RUnlock()
		return addr, nil
	}
	s.cacheMu.RUnlock()

	// 尝试每个发现器
	var lastErr error
	for _, discoverer := range s.discoverers {
		addr, err := discoverer.Discover(ctx)
		if err != nil {
			log.Debug("IP 发现器失败",
				"discoverer", discoverer.Name(),
				"err", err)
			lastErr = err
			continue
		}

		// 更新缓存
		s.cacheMu.Lock()
		s.externalAddr = addr
		s.cacheTime = time.Now()
		s.cacheMu.Unlock()

		log.Info("获取到外部地址",
			"discoverer", discoverer.Name(),
			"addr", addr.String())

		return addr, nil
	}

	return nil, fmt.Errorf("all IP discoverers failed: %v", lastErr)
}

// GetExternalAddressFromPortMapperWithContext 仅从端口映射器获取外部地址（严格模式）
//
// 注意：
// - 此方法不会复用 GetExternalAddressWithContext 的 discoverers（避免 HTTP/VPN 出口 IP 与路由器外部 IP 不一致）；
// - 仅当存在可用 PortMapper 并能返回外部地址时才成功。
// - 每个 mapper 调用有独立的 5s 超时，避免阻塞的底层网络调用（如 NAT-PMP）卡住整个流程。
// - 支持 ctx 取消，即使底层调用阻塞也能及时返回。
func (s *Service) GetExternalAddressFromPortMapperWithContext(ctx context.Context) (endpoint.Address, error) {
	// 单个 mapper 调用的最大超时
	const perMapperTimeout = 5 * time.Second

	// 遍历可用映射器，优先使用可用的映射器返回的外部地址
	var lastErr error
	for _, mapper := range s.mappers {
		if mapper == nil || !mapper.Available() {
			continue
		}

		// 用 goroutine + select 包装 mapper 调用，确保可被 ctx 取消
		type result struct {
			addr endpoint.Address
			err  error
		}
		ch := make(chan result, 1)
		mapperName := mapper.Name() // 捕获 mapper 名称供 goroutine 使用

		go func(m natif.PortMapper) {
			addr, err := m.GetExternalAddress()
			ch <- result{addr, err}
		}(mapper)

		// 为单个 mapper 创建超时 context
		mapperCtx, cancel := context.WithTimeout(ctx, perMapperTimeout)

		select {
		case r := <-ch:
			cancel()
			if r.err != nil {
				lastErr = r.err
				log.Debug("端口映射器获取外部地址失败",
					"mapper", mapperName,
					"err", r.err)
				continue
			}
			if r.addr == nil {
				lastErr = fmt.Errorf("mapper %s returned nil address", mapperName)
				continue
			}

			// 可达性验证：只接受"真实公网"外部地址。
			// 一些双层 NAT / CGNAT / 运营商网关可能会返回私网 external IP（如 192.168.x.x），这不能当作公网通告。
			if !r.addr.IsPublic() {
				lastErr = fmt.Errorf("mapper %s returned non-public external address: %s", mapperName, r.addr.String())
				log.Debug("端口映射器返回的外部地址不是公网，忽略（可能是双层 NAT/CGNAT）",
					"mapper", mapperName,
					"addr", r.addr.String())
				continue
			}

			log.Info("通过端口映射器获取到外部地址",
				"mapper", mapperName,
				"addr", r.addr.String())
			return r.addr, nil

		case <-mapperCtx.Done():
			cancel()
			// 区分是整体 ctx 取消还是单个 mapper 超时
			if ctx.Err() != nil {
				// 整体 ctx 被取消，立即返回
				return nil, ctx.Err()
			}
			// 单个 mapper 超时，继续尝试下一个
			lastErr = fmt.Errorf("mapper %s timed out after %v", mapperName, perMapperTimeout)
			log.Debug("端口映射器调用超时",
				"mapper", mapperName,
				"timeout", perMapperTimeout)
			continue
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("no available port mappers")
	}
	return nil, lastErr
}

// NATType 返回 NAT 类型
func (s *Service) NATType() types.NATType {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	return s.natType
}

// DetectNATType 检测 NAT 类型
func (s *Service) DetectNATType(ctx context.Context) (types.NATType, error) {
	if s.stunClient == nil {
		return types.NATTypeUnknown, fmt.Errorf("STUN client not available")
	}

	natType, err := s.stunClient.GetNATType(ctx)
	if err != nil {
		return types.NATTypeUnknown, err
	}

	// 更新缓存
	s.cacheMu.Lock()
	s.natType = natType
	s.cacheMu.Unlock()

	log.Info("检测到 NAT 类型", "type", natType.String())

	return natType, nil
}

// MapPort 映射端口
func (s *Service) MapPort(protocol string, internalPort, _ int, duration time.Duration) error {
	if duration == 0 {
		duration = s.config.MappingDuration
	}

	// 若该 internalPort 已存在旧映射，先 best-effort 清理，避免路由器映射表堆积。
	key := fmt.Sprintf("%s:%d", protocol, internalPort)
	var existingExternalPort int
	s.mappingsMu.RLock()
	if m, ok := s.mappings[key]; ok && m != nil {
		existingExternalPort = m.ExternalPort
	}
	s.mappingsMu.RUnlock()

	// 尝试每个映射器
	var lastErr error
	for _, mapper := range s.mappers {
		if !mapper.Available() {
			log.Debug("映射器不可用", "mapper", mapper.Name())
			continue
		}

		// 尝试删除旧映射（忽略错误）：旧映射可能来自其它 mapper 或已被路由器清理
		if existingExternalPort > 0 {
			_ = mapper.DeleteMapping(protocol, existingExternalPort)
		}

		mappedPort, err := mapper.AddMapping(protocol, internalPort, "dep2p", duration)
		if err != nil {
			log.Debug("端口映射失败",
				"mapper", mapper.Name(),
				"err", err)
			lastErr = err
			continue
		}

		// 保存映射
		s.mappingsMu.Lock()
		s.mappings[key] = &natif.Mapping{
			Protocol:     protocol,
			InternalPort: internalPort,
			ExternalPort: mappedPort,
			Description:  "dep2p",
			Expiry:       time.Now().Add(duration),
		}
		s.mappingsMu.Unlock()

		log.Info("端口映射成功",
			"mapper", mapper.Name(),
			"protocol", protocol,
			"internalPort", internalPort,
			"externalPort", mappedPort)

		return nil
	}

	if lastErr != nil {
		return fmt.Errorf("all mappers failed: %v", lastErr)
	}
	return fmt.Errorf("no port mapper available")
}

// UnmapPort 取消端口映射
func (s *Service) UnmapPort(protocol string, externalPort int) error {
	for _, mapper := range s.mappers {
		if !mapper.Available() {
			continue
		}

		if err := mapper.DeleteMapping(protocol, externalPort); err != nil {
			log.Debug("删除端口映射失败",
				"mapper", mapper.Name(),
				"err", err)
			continue
		}

		// 移除本地记录
		s.mappingsMu.Lock()
		for key, mapping := range s.mappings {
			if mapping.Protocol == protocol && mapping.ExternalPort == externalPort {
				delete(s.mappings, key)
				break
			}
		}
		s.mappingsMu.Unlock()

		return nil
	}

	return nil
}

// GetMappedPort 获取已映射的端口
func (s *Service) GetMappedPort(protocol string, internalPort int) (int, error) {
	key := fmt.Sprintf("%s:%d", protocol, internalPort)

	s.mappingsMu.RLock()
	defer s.mappingsMu.RUnlock()

	if mapping, ok := s.mappings[key]; ok {
		return mapping.ExternalPort, nil
	}

	return 0, fmt.Errorf("mapping not found")
}

// Refresh 刷新所有映射
func (s *Service) Refresh(ctx context.Context) error {
	s.mappingsMu.RLock()
	mappings := make([]*natif.Mapping, 0, len(s.mappings))
	for _, m := range s.mappings {
		mappings = append(mappings, m)
	}
	s.mappingsMu.RUnlock()

	var errs []error
	for _, mapping := range mappings {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		ttl := mapping.TTL()
		if ttl <= 0 {
			ttl = s.config.MappingDuration
		}

		if err := s.MapPort(mapping.Protocol, mapping.InternalPort, mapping.ExternalPort, ttl); err != nil {
			log.Warn("刷新映射失败",
				"protocol", mapping.Protocol,
				"port", mapping.InternalPort,
				"err", err)
			errs = append(errs, err)
		}
	}

	// 同时刷新外部地址缓存
	s.cacheMu.Lock()
	s.externalAddr = nil
	s.cacheMu.Unlock()

	if len(errs) > 0 {
		return fmt.Errorf("refresh failed for %d mappings", len(errs))
	}
	return nil
}

// Close 关闭服务
func (s *Service) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()

		log.Info("关闭 NAT 服务")

		// 停止刷新循环（安全关闭 channel）
		close(s.stopCh)
		if s.refreshTicker != nil {
			s.refreshTicker.Stop()
		}

		// 关闭 STUN 客户端
		if s.stunClient != nil {
			_ = s.stunClient.Close() // 关闭时忽略错误
		}

		// 删除所有映射
		s.mappingsMu.Lock()
		for key, mapping := range s.mappings {
			for _, mapper := range s.mappers {
				if mapper.Available() {
					_ = mapper.DeleteMapping(mapping.Protocol, mapping.ExternalPort) // 清理时忽略错误
				}
			}
			delete(s.mappings, key)
		}
		s.mappingsMu.Unlock()

		// 关闭映射器
		for _, mapper := range s.mappers {
			switch m := mapper.(type) {
			case *upnp.Mapper:
				m.Close()
			case *natpmp.Mapper:
				m.Close()
			}
		}
	})

	return nil
}

// ============================================================================
//                              扩展方法
// ============================================================================

// HolePuncher 返回 UDP 打洞器
func (s *Service) HolePuncher() natif.HolePuncher {
	return s.holePuncher
}

// TCPHolePuncher 返回 TCP 打洞器
func (s *Service) TCPHolePuncher() *holepunch.TCPPuncher {
	return s.tcpHolePuncher
}

// STUNClient 返回 STUN 客户端
func (s *Service) STUNClient() natif.STUNClient {
	return s.stunClient
}

// Mappers 返回所有映射器
func (s *Service) Mappers() []natif.PortMapper {
	return s.mappers
}

// Discoverers 返回所有发现器
func (s *Service) Discoverers() []natif.IPDiscoverer {
	return s.discoverers
}

// GetMappings 返回所有映射
func (s *Service) GetMappings() []*natif.Mapping {
	s.mappingsMu.RLock()
	defer s.mappingsMu.RUnlock()

	mappings := make([]*natif.Mapping, 0, len(s.mappings))
	for _, m := range s.mappings {
		mappings = append(mappings, m)
	}
	return mappings
}

// ============================================================================
//                              内部方法
// ============================================================================

// refreshLoop 刷新循环
func (s *Service) refreshLoop() {
	// 检查 refreshTicker 是否有效
	if s.refreshTicker == nil {
		return
	}

	for {
		select {
		case <-s.stopCh:
			return
		case <-s.refreshTicker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			if err := s.Refresh(ctx); err != nil {
				log.Warn("刷新映射失败", "err", err)
			}
			cancel()
		}
	}
}

// ============================================================================
//                              Layer1: 自适应策略
// ============================================================================

// UpdateReachabilityState 更新可达性状态
//
// Layer1 修复：当 ReachabilityCoordinator 检测到 VerifiedDirect 状态变化时调用此方法。
// 根据新状态动态调整 NAT 探测/映射行为。
//
// 参数：
//   - hasVerifiedDirect: 是否有已验证的直连地址
func (s *Service) UpdateReachabilityState(hasVerifiedDirect bool) {
	s.verifiedDirectMu.Lock()
	oldState := s.verifiedDirect
	s.verifiedDirect = hasVerifiedDirect
	s.lastPolicyUpdate = time.Now()
	s.policyUpdateCount++
	s.verifiedDirectMu.Unlock()

	if oldState == hasVerifiedDirect {
		return // 状态未变化
	}

	// 仅在自适应模式下生效
	if s.config.PolicyMode != natif.PolicyModeAdaptive {
		return
	}

	if hasVerifiedDirect {
		// 已有验证直连：降频/关闭外部探测
		log.Info("自适应策略：检测到 VerifiedDirect，降低外部探测频率",
			"disableExternal", s.config.AdaptivePolicy.DisableExternalOnVerified)

		// 停止端口映射刷新（如果配置了 DisableExternalOnVerified）
		if s.config.AdaptivePolicy.DisableExternalOnVerified {
			s.pauseExternalProbing()
		}
	} else {
		// 无验证直连：恢复正常探测
		log.Info("自适应策略：VerifiedDirect 失效，恢复正常探测频率")
		s.resumeExternalProbing()
	}
}

// IsVerifiedDirect 返回当前是否有验证的直连地址
func (s *Service) IsVerifiedDirect() bool {
	s.verifiedDirectMu.RLock()
	defer s.verifiedDirectMu.RUnlock()
	return s.verifiedDirect
}

// pauseExternalProbing 暂停外部探测
func (s *Service) pauseExternalProbing() {
	// 暂停刷新定时器
	if s.refreshTicker != nil {
		s.refreshTicker.Stop()
	}

	log.Debug("外部探测已暂停（VerifiedDirect 状态）")
}

// resumeExternalProbing 恢复外部探测
func (s *Service) resumeExternalProbing() {
	// 重启刷新定时器
	if s.config.MappingRefreshInterval > 0 && s.refreshTicker != nil {
		// 重新创建 ticker
		s.refreshTicker.Reset(s.config.MappingRefreshInterval)
	}

	log.Debug("外部探测已恢复")
}

// GetPolicyMode 返回当前策略模式
func (s *Service) GetPolicyMode() natif.PolicyMode {
	return s.config.PolicyMode
}

// GetAdaptivePolicyConfig 返回自适应策略配置
func (s *Service) GetAdaptivePolicyConfig() natif.AdaptivePolicyConfig {
	return s.config.AdaptivePolicy
}

// ============================================================================
//                              stunDiscoverer 包装
// ============================================================================

// stunDiscoverer STUN 发现器包装
type stunDiscoverer struct {
	client *stun.Client
}

func (d *stunDiscoverer) Name() string {
	return "stun"
}

func (d *stunDiscoverer) Discover(ctx context.Context) (endpoint.Address, error) {
	return d.client.GetMappedAddress(ctx)
}

func (d *stunDiscoverer) Priority() int {
	return 10 // 最高优先级
}
