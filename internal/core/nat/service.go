package nat

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/nat/holepunch"
	"github.com/dep2p/go-dep2p/internal/core/nat/natpmp"
	"github.com/dep2p/go-dep2p/internal/core/nat/stun"
	"github.com/dep2p/go-dep2p/internal/core/nat/upnp"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 使用 doc.go 中定义的 logger

// Reachability 可达性状态
type Reachability int

const (
	// ReachabilityUnknown 未知
	ReachabilityUnknown Reachability = iota
	// ReachabilityPublic 公网可达
	ReachabilityPublic
	// ReachabilityPrivate NAT 后
	ReachabilityPrivate
)

func (r Reachability) String() string {
	switch r {
	case ReachabilityPublic:
		return "Public"
	case ReachabilityPrivate:
		return "Private"
	default:
		return "Unknown"
	}
}

// ============================================================================
// 事件定义
// ============================================================================

// ReachabilityChangedEvent 可达性变更事件
//
// 当 NAT 服务检测到可达性状态变化时发布此事件。
// 上层组件（如 AutoNAT、Relay）可以订阅此事件并调整行为。
type ReachabilityChangedEvent struct {
	// Old 旧可达性状态
	Old Reachability
	// New 新可达性状态
	New Reachability
}

// Service NAT 服务
type Service struct {
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	config   *Config
	swarm    pkgif.Swarm
	eventbus pkgif.EventBus

	// 子组件
	autonat       *AutoNAT
	autonatServer *AutoNATServer
	stunClient    *stun.STUNClient
	upnp          *upnp.UPnPMapper
	natpmp        *natpmp.NATPMPMapper
	puncher       *holepunch.HolePuncher
	natDetector   *stun.NATTypeDetector

	reachability       atomic.Value // Reachability
	reachabilityLocked atomic.Bool  // 可达性是否锁定为 Public
	natType            atomic.Value // types.NATType
	started            atomic.Bool
	closed             atomic.Bool

	mu            sync.RWMutex
	externalAddrs []string

	// 地址发现集成
	coordinator   pkgif.ReachabilityCoordinator // 可达性协调器（用于上报候选地址）
	coordinatorMu sync.RWMutex
	listenPorts   []int // 监听端口（用于与外部 IP 组合）
	listenPortsMu sync.RWMutex

	// v2.0 新增：自动 Relay 支持
	relayManager     RelayManagerInterface // Relay 管理器（用于自动启用 Relay）
	relayManagerMu   sync.RWMutex
	autoRelayEnabled atomic.Bool // 是否已自动启用 Relay

	// 生命周期协调器（用于通知 NAT 类型检测完成）
	lifecycleCoordinator LifecycleCoordinatorInterface
	lifecycleCoordMu     sync.RWMutex

	// STUN 信任模式
	// 启用后，STUN 发现的地址将直接标记为已验证，无需 dial-back/witness 验证
	trustSTUNAddresses atomic.Bool
}

// LifecycleCoordinatorInterface 生命周期协调器接口
//
// 用于 NAT Service 通知 A3 阶段完成
type LifecycleCoordinatorInterface interface {
	// SetNATTypeReady 标记 NAT 类型检测完成
	SetNATTypeReady()
}

// RelayManagerInterface Relay 管理器接口
//
// v2.0 新增：用于 Private 节点自动启用 Relay 客户端
type RelayManagerInterface interface {
	// EnableClient 启用 Relay 客户端
	EnableClient(ctx context.Context) error
	// HasRelay 是否已有 Relay 配置
	HasRelay() bool
	// RegisterToRelay 注册到 Relay 地址簿（如果配置了 Relay 地址）
	RegisterToRelay(ctx context.Context) error
}

// NewService 创建 NAT 服务
func NewService(config *Config, swarm pkgif.Swarm, eventbus pkgif.EventBus) (*Service, error) {
	if config == nil {
		return nil, ErrInvalidConfig
	}

	if err := config.Validate(); err != nil {
		return nil, err
	}

	s := &Service{
		config:   config,
		swarm:    swarm,
		eventbus: eventbus,
	}

	// 初始化可达性状态和 NAT 类型
	s.reachability.Store(ReachabilityUnknown)
	s.natType.Store(types.NATTypeUnknown)

	// 检查可达性锁定配置
	// 基础设施节点（引导节点/中继节点）配置了公网地址时，锁定可达性为 Public
	// 同时设置 NAT 类型为 None，避免被误判为 Symmetric NAT
	if config.LockReachabilityPublic {
		s.reachabilityLocked.Store(true)
		s.reachability.Store(ReachabilityPublic)
		s.natType.Store(types.NATTypeNone) // 公网节点无 NAT
		logger.Info("可达性锁定为 Public",
			"reason", "基础设施节点配置了公网地址",
			"natType", "None")
	}

	// 创建子组件
	if config.EnableAutoNAT {
		s.autonat = newAutoNAT(config)
		s.autonat.service = s
	}

	// 创建 STUN 客户端
	if len(config.STUNServers) > 0 {
		s.stunClient = stun.NewSTUNClient(config.STUNServers)
	}

	// 创建 NAT 类型检测器
	if config.NATTypeDetectionEnabled && len(config.STUNServers) > 0 {
		primaryServer := config.STUNServers[0]
		alternateServer := config.AlternateSTUNServer
		if alternateServer == "" && len(config.STUNServers) > 1 {
			alternateServer = config.STUNServers[1]
		}
		s.natDetector = stun.NewNATTypeDetector(primaryServer, alternateServer)
		if config.NATTypeDetectionTimeout > 0 {
			s.natDetector.SetTimeout(config.NATTypeDetectionTimeout)
		}
	}

	// NAT-001 优化：并行创建 UPnP 和 NAT-PMP 映射器
	// 这两个操作都可能因为超时而阻塞，并行执行可以将总时间从 ~13秒 降低到 ~2秒
	var wg sync.WaitGroup
	var upnpErr, natpmpErr error
	var upnpMapper *upnp.UPnPMapper
	var natpmpMapper *natpmp.NATPMPMapper

	// 并行创建 UPnP 映射器
	if config.EnableUPnP {
		wg.Add(1)
		go func() {
			defer wg.Done()
			upnpTimeout := config.UPnPTimeout
			if upnpTimeout <= 0 {
				upnpTimeout = upnp.DefaultUPnPTimeout
			}
			upnpMapper, upnpErr = upnp.NewUPnPMapperWithTimeout(upnpTimeout)
		}()
	}

	// 并行创建 NAT-PMP 映射器
	if config.EnableNATPMP {
		wg.Add(1)
		go func() {
			defer wg.Done()
			timeout := config.NATPMPTimeout
			if timeout <= 0 {
				timeout = natpmp.DefaultNATPMPTimeout
			}
			natpmpMapper, natpmpErr = natpmp.NewNATPMPMapperWithTimeout(timeout)
		}()
	}

	// 等待并行探测完成
	wg.Wait()

	// 处理 UPnP 结果
	if config.EnableUPnP {
		if upnpErr != nil {
			logger.Debug("UPnP 不可用（这在很多环境中是正常的）", "error", upnpErr)
		} else {
			s.upnp = upnpMapper
			logger.Debug("UPnP 映射器已创建")
		}
	}

	// 处理 NAT-PMP 结果
	if config.EnableNATPMP {
		if natpmpErr != nil {
			logger.Debug("NAT-PMP 不可用（这在很多环境中是正常的）",
				"error", natpmpErr,
				"timeout", config.NATPMPTimeout)
		} else {
			s.natpmp = natpmpMapper
			logger.Debug("NAT-PMP 映射器已创建", "timeout", config.NATPMPTimeout)
		}
	}

	// 创建 Hole Puncher
	if config.EnableHolePunch {
		// TD-001 已完成：完整的 DCUtR 打洞协议实现
		// 注意：host 需要在 Start() 时传入以注册协议处理器
		s.puncher = holepunch.NewHolePuncher(swarm, nil)
	}

	return s, nil
}

// Start 启动 NAT 服务
func (s *Service) Start(_ context.Context, host pkgif.Host) error {
	if s.closed.Load() {
		return ErrServiceClosed
	}

	if !s.started.CompareAndSwap(false, true) {
		return ErrAlreadyStarted
	}

	logger.Info("正在启动 NAT 服务",
		"enableAutoNAT", s.config.EnableAutoNAT,
		"enableHolePunch", s.config.EnableHolePunch,
		"stunServers", len(s.config.STUNServers))

	// 使用 context.Background() 而不是传入的 ctx
	// 因为 Fx OnStart 的 ctx 在返回后会被取消，导致后台循环提前退出
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// 订阅地址更新事件，动态获取监听端口
	// 这解决了 wireAddressDiscovery 在 Swarm.Listen 之前执行的时序问题
	s.subscribeAddressUpdates()

	// 设置 AutoNAT 的 host 和 swarm（用于发送请求）
	if s.autonat != nil && host != nil {
		s.autonat.host = host
		s.autonat.swarm = s.swarm
	}

	// 注册 AutoNAT 服务端协议（如果启用）
	if s.config.EnableAutoNAT && host != nil {
		s.autonatServer = NewAutoNATServer(host, s.swarm, nil)
		if err := s.autonatServer.RegisterProtocol(); err != nil {
			logger.Warn("注册 AutoNAT 协议失败", "error", err)
		} else {
			logger.Debug("AutoNAT 协议已注册")
		}
	}

	// 设置 HolePuncher 的 Host（如果启用）
	if s.puncher != nil && host != nil {
		s.puncher.Host = host

		// 注册 Hole Punch 协议处理器
		handler := holepunch.NewHandler(s.puncher, s.swarm, host)
		if err := handler.RegisterProtocol(); err != nil {
			logger.Warn("注册 Hole Punch 协议失败", "error", err)
		} else {
			logger.Debug("Hole Punch 协议已注册")
		}
	}

	// 启动 AutoNAT 探测循环
	if s.config.EnableAutoNAT && s.autonat != nil {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.autonat.runProbeLoop(s.ctx)
		}()
		logger.Debug("AutoNAT 探测循环已启动")
	}

	// 启动 STUN 地址刷新
	if s.stunClient != nil {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.refreshExternalAddrLoop(s.ctx)
		}()
		logger.Debug("STUN 地址刷新循环已启动")
	}

	// 启动 UPnP 映射续期
	if s.upnp != nil {
		s.upnp.Start(s.ctx)
		logger.Debug("UPnP 映射续期已启动")
	}

	// 启动 NAT-PMP 映射续期
	if s.natpmp != nil {
		s.natpmp.Start(s.ctx)
		logger.Debug("NAT-PMP 映射续期已启动")
	}

	// 启动时自动执行 NAT 类型检测（异步，不阻塞启动）
	// NAT 类型信息用于打洞决策：
	// - Full Cone / Restricted Cone: 可直接打洞
	// - Symmetric: 需要 Relay 中继
	//
	// 时序对齐（Phase A3）：移除延迟，直接执行检测，完成后通知 LifecycleCoordinator
	if s.natDetector != nil && s.config.NATTypeDetectionEnabled {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer s.notifyNATTypeReady() // 无论成功失败都通知 A3 gate 解除

			detectCtx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
			defer cancel()

			natType, err := s.DetectNATType(detectCtx)
			if err != nil {
				logger.Warn("启动时 NAT 类型检测失败，使用 Unknown 继续", "error", err)
			} else {
				logger.Info("启动时 NAT 类型检测完成", "type", natType.String())
			}
		}()
		logger.Debug("NAT 类型检测任务已启动（无延迟）")
	} else {
		// 未启用 NAT 类型检测，直接设置 A3 gate 解除
		s.notifyNATTypeReady()
		logger.Debug("NAT 类型检测未启用，直接解除 A3 gate")
	}

	logger.Info("NAT 服务启动成功")
	return nil
}

// Stop 停止 NAT 服务
func (s *Service) Stop() error {
	if s.closed.Load() {
		return nil
	}

	logger.Info("正在停止 NAT 服务")
	s.closed.Store(true)

	// 停止 UPnP 和 NAT-PMP 续期循环
	if s.upnp != nil {
		s.upnp.Stop()
		logger.Debug("UPnP 已停止")
	}
	if s.natpmp != nil {
		s.natpmp.Stop()
		logger.Debug("NAT-PMP 已停止")
	}

	if s.cancel != nil {
		s.cancel()
	}

	s.wg.Wait()

	logger.Info("NAT 服务已停止")
	return nil
}

// Reachability 返回当前可达性状态
func (s *Service) Reachability() Reachability {
	if v := s.reachability.Load(); v != nil {
		return v.(Reachability)
	}
	return ReachabilityUnknown
}

// GetReachability 获取当前可达性状态（字符串形式）
//
// 实现 interfaces.NATService 接口。
func (s *Service) GetReachability() string {
	return s.Reachability().String()
}

// SetReachability 设置可达性状态（内部使用）
//
// 当可达性状态发生变化时：
//  1. 更新内部状态
//  2. 发布 ReachabilityChangedEvent 到 EventBus
//  3. v2.0 新增：Private 节点自动启用 Relay 客户端
//
// 如果可达性被锁定为 Public，则不允许降级为 Private
func (s *Service) SetReachability(r Reachability) {
	old := s.Reachability()
	if old == r {
		return
	}

	// 检查可达性锁定：如果锁定为 Public，则拒绝降级为 Private/Unknown
	if s.reachabilityLocked.Load() && r != ReachabilityPublic {
		logger.Debug("可达性锁定：拒绝降级",
			"locked", "Public",
			"requested", r.String(),
			"reason", "基础设施节点配置了公网地址，可达性锁定为 Public")
		return
	}

	s.reachability.Store(r)

	// 记录可达性变化（这是重要的网络状态变化）
	logger.Info("可达性状态变化",
		"old", old.String(),
		"new", r.String(),
		"implication", getReachabilityImplication(r))

	// 发布可达性变更事件
	if s.eventbus != nil {
		// 获取事件发射器
		emitter, err := s.eventbus.Emitter(&ReachabilityChangedEvent{})
		if err == nil {
			defer emitter.Close()
			emitter.Emit(&ReachabilityChangedEvent{
				Old: old,
				New: r,
			})
			logger.Debug("可达性变更事件已发布")
		} else {
			logger.Warn("无法发布可达性变更事件", "error", err)
		}
	}

	// v2.0 新增：Private 节点自动启用 Relay 客户端
	if r == ReachabilityPrivate && s.config.AutoEnableRelay {
		s.tryAutoEnableRelay()
	}
}

// tryAutoEnableRelay 尝试自动启用 Relay 客户端
//
// v2.0 新增：当节点被检测为 Private 时，自动启用 Relay 客户端
func (s *Service) tryAutoEnableRelay() {
	// 检查是否已启用
	if s.autoRelayEnabled.Load() {
		return
	}

	// 获取 RelayManager
	s.relayManagerMu.RLock()
	rm := s.relayManager
	s.relayManagerMu.RUnlock()

	if rm == nil {
		logger.Debug("RelayManager 未设置，跳过自动 Relay 启用")
		return
	}

	// 检查是否已有 Relay 配置
	if rm.HasRelay() {
		logger.Debug("已有 Relay 配置，跳过自动启用")
		return
	}

	// 启用 Relay 客户端
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	if err := rm.EnableClient(ctx); err != nil {
		logger.Warn("自动启用 Relay 客户端失败", "error", err)
		return
	}

	// 注册到 Relay 地址簿
	if err := rm.RegisterToRelay(ctx); err != nil {
		logger.Warn("自动注册到 Relay 失败", "error", err)
		// 注册失败不影响启用状态
	}

	s.autoRelayEnabled.Store(true)
	logger.Info("Private 节点自动启用 Relay 客户端成功")
}

// SetRelayManager 设置 Relay 管理器
//
// v2.0 新增：用于 Private 节点自动启用 Relay 功能
func (s *Service) SetRelayManager(rm RelayManagerInterface) {
	s.relayManagerMu.Lock()
	defer s.relayManagerMu.Unlock()
	s.relayManager = rm
}

// SetLifecycleCoordinator 设置生命周期协调器
//
// 用于 NAT 类型检测完成后通知 A3 gate 解除
func (s *Service) SetLifecycleCoordinator(lc LifecycleCoordinatorInterface) {
	s.lifecycleCoordMu.Lock()
	defer s.lifecycleCoordMu.Unlock()
	s.lifecycleCoordinator = lc
}

// notifyNATTypeReady 通知 NAT 类型检测完成
func (s *Service) notifyNATTypeReady() {
	s.lifecycleCoordMu.RLock()
	lc := s.lifecycleCoordinator
	s.lifecycleCoordMu.RUnlock()

	if lc != nil {
		lc.SetNATTypeReady()
	}
}

// getReachabilityImplication 获取可达性状态的含义说明
func getReachabilityImplication(r Reachability) string {
	switch r {
	case ReachabilityPublic:
		return "公网可达，可以接受入站连接"
	case ReachabilityPrivate:
		return "NAT 后，需要中继或打洞才能被连接"
	default:
		return "网络状态未知，等待检测"
	}
}

// ExternalAddrs 返回外部地址列表
func (s *Service) ExternalAddrs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 返回拷贝
	addrs := make([]string, len(s.externalAddrs))
	copy(addrs, s.externalAddrs)
	return addrs
}

// setExternalAddrs 设置外部地址（内部使用）
func (s *Service) setExternalAddrs(addrs []string) {
	s.mu.Lock()
	s.externalAddrs = addrs
	s.mu.Unlock()
}

// MapPort 映射端口（公开方法）
func (s *Service) MapPort(ctx context.Context, proto string, port int) (int, error) {
	if s.closed.Load() {
		return 0, ErrServiceClosed
	}

	// 优先尝试 UPnP
	if s.upnp != nil {
		if extPort, err := s.upnp.MapPort(ctx, proto, port); err == nil {
			return extPort, nil
		}
	}

	// 回退到 NAT-PMP
	if s.natpmp != nil {
		if extPort, err := s.natpmp.MapPort(ctx, proto, port); err == nil {
			return extPort, nil
		}
	}

	return 0, ErrMappingFailed
}

// UnmapPort 取消端口映射
func (s *Service) UnmapPort(proto string, port int) error {
	if s.closed.Load() {
		return ErrServiceClosed
	}

	var lastErr error

	if s.upnp != nil {
		if err := s.upnp.UnmapPort(proto, port); err != nil {
			lastErr = err
		}
	}

	if s.natpmp != nil {
		if err := s.natpmp.UnmapPort(proto, port); err != nil {
			lastErr = err
		}
	}

	if lastErr != nil {
		return lastErr
	}

	return nil
}

// refreshExternalAddrLoop 刷新外部地址循环
func (s *Service) refreshExternalAddrLoop(ctx context.Context) {
	// 首次立即查询
	s.refreshExternalAddr(ctx)

	ticker := time.NewTicker(s.config.STUNCacheDuration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.refreshExternalAddr(ctx)
		}
	}
}

// refreshExternalAddr 刷新外部地址
func (s *Service) refreshExternalAddr(ctx context.Context) {
	if s.stunClient == nil {
		return
	}

	logger.Debug("开始 STUN 外部地址查询")
	startTime := time.Now()

	addr, err := s.stunClient.GetExternalAddr(ctx)
	if err != nil {
		// 记录 STUN 查询失败，便于排查问题
		logger.Warn("STUN 查询外部地址失败",
			"error", err,
			"duration", time.Since(startTime),
			"servers", s.config.STUNServers)
		return
	}

	logger.Info("STUN 查询成功",
		"externalIP", addr.IP.String(),
		"externalPort", addr.Port,
		"duration", time.Since(startTime))

	s.updateExternalAddr(addr)

	// 上报到 Reachability Coordinator
	s.reportSTUNAddressToCoordinator(addr)
}

// reportSTUNAddressToCoordinator 将 STUN 发现的地址上报到 Coordinator
//
// STUN 发现的公网地址作为【候选地址】上报，需要经过 Reachability 验证后才能发布：
// 1. STUN 服务器确认了外部可见 IP，但不保证可达性
// 2. 候选地址需要通过 dial-back/AutoNAT 验证后才能发布到 DHT
// 3. 直接发布未验证地址会导致"不可达地址泛滥"
func (s *Service) reportSTUNAddressToCoordinator(addr *net.UDPAddr) {
	if addr == nil {
		logger.Debug("STUN 地址为空，跳过上报")
		return
	}

	logger.Debug("处理 STUN 发现地址",
		"stunIP", addr.IP.String(),
		"stunPort", addr.Port)

	// 检查是否是公网 IP
	if !isPublicIP(addr.IP) {
		logger.Debug("STUN 返回非公网 IP，跳过", "ip", addr.IP)
		return
	}

	s.coordinatorMu.RLock()
	coordinator := s.coordinator
	s.coordinatorMu.RUnlock()

	if coordinator == nil {
		logger.Warn("Coordinator 未设置，无法上报 STUN 地址")
		return
	}

	// 获取监听端口
	s.listenPortsMu.RLock()
	ports := make([]int, len(s.listenPorts))
	copy(ports, s.listenPorts)
	s.listenPortsMu.RUnlock()

	logger.Debug("组合 STUN 地址",
		"stunIP", addr.IP.String(),
		"stunPort", addr.Port,
		"listenPorts", ports)

	if len(ports) == 0 {
		// 没有监听端口时，不能使用 STUN 返回的临时端口
		// 因为 STUN 查询使用系统分配的临时端口发送请求，返回的端口是这个临时端口，
		// 而不是节点实际监听的端口，对于连接来说是无效的
		logger.Debug("监听端口未设置，跳过 STUN 地址上报",
			"stunIP", addr.IP.String(),
			"stunPort", addr.Port)
		return
	}

	// 检查是否启用 STUN 信任模式
	trustSTUN := s.trustSTUNAddresses.Load()

	// 为每个监听端口生成地址
	for _, port := range ports {
		var maddr string
		if addr.IP.To4() != nil {
			maddr = fmt.Sprintf("/ip4/%s/udp/%d/quic-v1", addr.IP.String(), port)
		} else {
			maddr = fmt.Sprintf("/ip6/%s/udp/%d/quic-v1", addr.IP.String(), port)
		}

		// STUN 协议本身验证了外部地址的存在，直接标记为已验证
		// trustSTUN 模式下使用更高优先级 PriorityVerifiedDirect
		priority := pkgif.PrioritySTUNDiscovered
		source := "stun"
		if trustSTUN {
			priority = pkgif.PriorityVerifiedDirect
			source = "stun-trusted"
		}
		coordinator.OnDirectAddressVerified(maddr, source, priority)
		logger.Info("STUN 公网地址已验证",
			"addr", maddr,
			"stunOriginalPort", addr.Port,
			"usedPort", port,
			"priority", priority.String())
	}
}

// isPublicIP 判断是否是公网 IP
func isPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	// 必须是全局单播地址
	if !ip.IsGlobalUnicast() {
		return false
	}
	// 排除私网地址
	if ip.IsPrivate() {
		return false
	}
	// 排除回环地址
	if ip.IsLoopback() {
		return false
	}
	// 排除链路本地地址
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	return true
}

// updateExternalAddr 更新外部地址
//
// 注意：此方法已废弃，仅作为兼容性保留。
// 正确的地址应从 Reachability Coordinator 获取，它会使用监听端口而不是 STUN 返回的临时端口。
//
// 问题说明：
// STUN 查询使用系统分配的临时端口发送请求，返回的端口是这个临时端口，
// 而不是节点实际监听的端口。因此 STUN 返回的端口对于连接来说是无效的。
// reportSTUNAddressToCoordinator 方法会正确地使用监听端口组合地址。
func (s *Service) updateExternalAddr(addr *net.UDPAddr) {
	if addr == nil {
		return
	}

	// 检查是否是公网 IP
	if !isPublicIP(addr.IP) {
		return
	}

	// 获取监听端口
	s.listenPortsMu.RLock()
	ports := make([]int, len(s.listenPorts))
	copy(ports, s.listenPorts)
	s.listenPortsMu.RUnlock()

	// 如果没有监听端口，不存储地址
	// 因为 STUN 返回的端口是临时端口，对连接没有意义
	if len(ports) == 0 {
		logger.Debug("监听端口未设置，跳过存储 STUN 外部地址",
			"stunIP", addr.IP.String(),
			"stunPort", addr.Port)
		return
	}

	// 使用监听端口（而不是 STUN 返回的临时端口）生成地址
	addrs := make([]string, 0, len(ports))
	for _, port := range ports {
		var maddr string
		if addr.IP.To4() != nil {
			maddr = fmt.Sprintf("/ip4/%s/udp/%d/quic-v1", addr.IP.String(), port)
		} else {
			maddr = fmt.Sprintf("/ip6/%s/udp/%d/quic-v1", addr.IP.String(), port)
		}
		addrs = append(addrs, maddr)
	}

	s.setExternalAddrs(addrs)
	logger.Debug("外部地址已更新", "addrs", addrs)
}

// ForceSTUN 强制重新执行 STUN 探测
//
// 当网络变化时（如 4G→WiFi），外部 IP 地址可能已经改变，
// 需要立即重新执行 STUN 探测来获取新的外部地址。
func (s *Service) ForceSTUN(ctx context.Context) error {
	if s.stunClient == nil {
		logger.Warn("STUN 客户端未配置，跳过 STUN 探测")
		return nil
	}

	// 获取旧地址用于对比
	oldAddrs := s.ExternalAddrs()

	logger.Info("强制执行 STUN 探测",
		"reason", "network change detected",
		"oldAddrs", oldAddrs)

	startTime := time.Now()
	addr, err := s.stunClient.GetExternalAddr(ctx)
	if err != nil {
		logger.Warn("强制 STUN 探测失败",
			"error", err,
			"duration", time.Since(startTime))
		return err
	}

	s.updateExternalAddr(addr)

	// 检测地址是否变化
	newAddrs := s.ExternalAddrs()
	addressChanged := !stringSlicesEqual(oldAddrs, newAddrs)

	logger.Info("强制 STUN 探测完成",
		"externalIP", addr.IP.String(),
		"externalPort", addr.Port,
		"duration", time.Since(startTime),
		"addressChanged", addressChanged,
		"newAddrs", newAddrs)

	// 上报到 Reachability Coordinator
	s.reportSTUNAddressToCoordinator(addr)

	return nil
}

// stringSlicesEqual 比较两个字符串切片是否相等
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// GetNATType 返回当前检测到的 NAT 类型
//
// 实现 interfaces.NATService 接口。
// 如果尚未执行检测，返回 NATTypeUnknown。
func (s *Service) GetNATType() types.NATType {
	if v := s.natType.Load(); v != nil {
		return v.(types.NATType)
	}
	return types.NATTypeUnknown
}

// HolePuncher 返回 NAT 打洞器
//
// 返回内部的 HolePuncher 实例，用于 Realm Connector 进行 NAT 穿透。
// 如果 NAT 打洞未启用，返回 nil。
func (s *Service) HolePuncher() *holepunch.HolePuncher {
	return s.puncher
}

// SetReachabilityCoordinator 设置可达性协调器
//
// 用于将 STUN/UPnP/NAT-PMP 发现的地址上报到 Coordinator。
// 应在 NAT 服务启动前或启动后立即调用。
// 该方法会将 coordinator 传递给 UPnP 和 NAT-PMP Mapper。
func (s *Service) SetReachabilityCoordinator(coordinator pkgif.ReachabilityCoordinator) {
	s.coordinatorMu.Lock()
	s.coordinator = coordinator
	s.coordinatorMu.Unlock()

	// 传递给 UPnP 和 NAT-PMP Mapper
	if s.upnp != nil {
		s.upnp.SetReachabilityCoordinator(coordinator)
	}
	if s.natpmp != nil {
		s.natpmp.SetReachabilityCoordinator(coordinator)
	}

	logger.Debug("Reachability Coordinator 已设置并传递给 UPnP/NATPMP")
}

// SetTrustSTUNAddresses 设置 STUN 信任模式
//
// 启用后，STUN 发现的地址将直接标记为已验证（verified），
// 无需通过 dial-back 或 witness 验证。
//
// 适用场景：
//   - 云服务器部署（VPC 环境，公网 IP 由 NAT Gateway 提供）
//   - 已知公网可达的环境
//
// 风险提示：
//   - 仅在受控环境中启用
//   - 如果 STUN 服务器被劫持，可能导致地址欺骗
func (s *Service) SetTrustSTUNAddresses(trust bool) {
	s.trustSTUNAddresses.Store(trust)
	if trust {
		logger.Info("STUN 信任模式已启用，STUN 地址将直接标记为已验证")
	}
}

// SetListenPorts 设置监听端口
//
// 用于与 STUN 发现的外部 IP 组合生成完整的 multiaddr。
// 应在 Swarm 监听成功后调用。
func (s *Service) SetListenPorts(ports []int) {
	s.listenPortsMu.Lock()
	oldPorts := s.listenPorts
	s.listenPorts = make([]int, len(ports))
	copy(s.listenPorts, ports)
	s.listenPortsMu.Unlock()

	logger.Info("NAT 服务监听端口已设置",
		"oldPorts", oldPorts,
		"newPorts", ports)
}

// GetListenPorts 获取监听端口
func (s *Service) GetListenPorts() []int {
	s.listenPortsMu.RLock()
	defer s.listenPortsMu.RUnlock()

	ports := make([]int, len(s.listenPorts))
	copy(ports, s.listenPorts)
	return ports
}

// DetectNATType 执行 NAT 类型检测
//
// 实现 interfaces.NATService 接口。
// 基于 RFC 3489 算法检测 NAT 类型。
func (s *Service) DetectNATType(ctx context.Context) (types.NATType, error) {
	if s.closed.Load() {
		return types.NATTypeUnknown, ErrServiceClosed
	}

	if s.natDetector == nil {
		// 如果没有配置 NAT 检测器，尝试基于可达性状态推断
		reachability := s.Reachability()
		switch reachability {
		case ReachabilityPublic:
			return types.NATTypeNone, nil
		case ReachabilityPrivate:
			// 无法确定具体的 NAT 类型，返回 Unknown
			return types.NATTypeUnknown, nil
		default:
			return types.NATTypeUnknown, nil
		}
	}

	logger.Info("开始 NAT 类型检测")

	result, err := s.natDetector.DetectNATType(ctx)
	if err != nil {
		logger.Warn("NAT 类型检测失败", "err", err)
		return types.NATTypeUnknown, err
	}

	// 更新缓存的 NAT 类型
	s.natType.Store(result.Type)

	// 同时更新可达性状态
	if result.Type == types.NATTypeNone {
		s.SetReachability(ReachabilityPublic)
	} else if result.Type != types.NATTypeUnknown {
		s.SetReachability(ReachabilityPrivate)
	}

	logger.Info("NAT 类型检测完成",
		"type", result.Type.String(),
		"externalIP", result.ExternalIP,
		"mappedPort", result.MappedPort)

	return result.Type, nil
}

// subscribeAddressUpdates 订阅地址更新事件
//
// 当 Host 监听成功后会发布 EvtLocalAddrsUpdated 事件，
// NAT Service 订阅此事件来获取实际的监听端口。
// 这解决了 wireAddressDiscovery 在 Swarm.Listen 之前执行的时序问题。
func (s *Service) subscribeAddressUpdates() {
	if s.eventbus == nil {
		logger.Debug("EventBus 未设置，无法订阅地址更新事件")
		return
	}

	// 订阅地址更新事件
	sub, err := s.eventbus.Subscribe(&types.EvtLocalAddrsUpdated{})
	if err != nil {
		logger.Warn("订阅地址更新事件失败", "error", err)
		return
	}

	logger.Debug("NAT Service 已订阅地址更新事件")

	// 启动事件处理协程
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		defer sub.Close()

		for {
			select {
			case <-s.ctx.Done():
				return
			case evt, ok := <-sub.Out():
				if !ok {
					return
				}

				// 处理地址更新事件
				if addrEvt, ok := evt.(*types.EvtLocalAddrsUpdated); ok {
					s.handleAddressUpdate(addrEvt)
				}
			}
		}
	}()
}

// handleAddressUpdate 处理地址更新事件
func (s *Service) handleAddressUpdate(evt *types.EvtLocalAddrsUpdated) {
	if evt == nil || len(evt.Current) == 0 {
		return
	}

	// 从地址中提取端口
	ports := extractPortsFromAddrs(evt.Current)
	if len(ports) == 0 {
		logger.Debug("地址更新事件中无有效端口")
		return
	}

	// 检查端口是否已设置（避免重复设置）
	s.listenPortsMu.RLock()
	existingPorts := s.listenPorts
	s.listenPortsMu.RUnlock()

	if len(existingPorts) > 0 {
		// 已有端口，检查是否相同
		if portsEqual(existingPorts, ports) {
			return // 无变化
		}
	}

	// 设置新端口
	s.SetListenPorts(ports)
	logger.Info("从地址更新事件中获取监听端口", "ports", ports)
}

// extractPortsFromAddrs 从地址列表中提取端口
func extractPortsFromAddrs(addrs []string) []int {
	portSet := make(map[int]struct{})

	for _, addr := range addrs {
		port := extractPortFromAddr(addr)
		if port > 0 {
			portSet[port] = struct{}{}
		}
	}

	ports := make([]int, 0, len(portSet))
	for port := range portSet {
		ports = append(ports, port)
	}
	return ports
}

// extractPortFromAddr 从单个地址中提取端口
func extractPortFromAddr(addr string) int {
	// 解析 multiaddr 格式：/ip4/x.x.x.x/udp/4003/quic-v1
	parts := strings.Split(addr, "/")
	for i, part := range parts {
		if (part == "udp" || part == "tcp") && i+1 < len(parts) {
			port, err := strconv.Atoi(parts[i+1])
			if err == nil && port > 0 && port < 65536 {
				return port
			}
		}
	}
	return 0
}

// portsEqual 检查两个端口列表是否相等
func portsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	aSet := make(map[int]struct{}, len(a))
	for _, p := range a {
		aSet[p] = struct{}{}
	}
	for _, p := range b {
		if _, ok := aSet[p]; !ok {
			return false
		}
	}
	return true
}
