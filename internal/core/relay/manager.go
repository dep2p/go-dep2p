package relay

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/nat"
	"github.com/dep2p/go-dep2p/internal/core/nat/holepunch"
	"github.com/dep2p/go-dep2p/internal/core/relay/addressbook"
	"github.com/dep2p/go-dep2p/internal/core/relay/client"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/multiaddr"
	"github.com/dep2p/go-dep2p/pkg/protocol"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ════════════════════════════════════════════════════════════════════════════
// Manager - 统一中继管理器（v2.0）
// ════════════════════════════════════════════════════════════════════════════
//
// v2.0 统一 Relay 架构：
// - 单一 Relay 服务（不再区分 System/Realm）
// - 三大职责：缓存加速 + 打洞协调 + 数据保底
// - 详见 design/_discussions/20260123-nat-relay-concept-clarification.md §9.0
// ════════════════════════════════════════════════════════════════════════════

// Manager 中继管理器
type Manager struct {
	config    *Config
	swarm     pkgif.Swarm
	host      pkgif.Host
	eventbus  pkgif.EventBus
	peerstore pkgif.Peerstore

	// 统一中继服务（v2.0）
	relay *RelayService

	// v2.0 新增：AutoRelay 自动中继管理
	autoRelay   pkgif.AutoRelay
	autoRelayMu sync.RWMutex

	// v2.0 新增：可达性协调器（用于地址回调）
	coordinator   pkgif.ReachabilityCoordinator
	coordinatorMu sync.RWMutex

	// v2.0 新增：地址簿服务（用于注册到 Relay）
	addressBookService   *addressbook.AddressBookService
	addressBookServiceMu sync.RWMutex

	// 生命周期协调器（用于通知 Relay 连接完成）
	lifecycleCoordinator LifecycleCoordinatorInterface
	lifecycleCoordMu     sync.RWMutex

	// Hole Puncher
	holePuncher *holepunch.HolePuncher

	// v2.0 新增：备份 Relay 连接（打洞成功后保留）
	// key: target peer ID, value: relay connection
	backupRelayConns   map[string]pkgif.Connection
	backupRelayConnsMu sync.RWMutex

	mu     sync.RWMutex
	closed atomic.Bool

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// LifecycleCoordinatorInterface 生命周期协调器接口
//
// 用于 Relay Manager 通知连接完成和电路状态变化
type LifecycleCoordinatorInterface interface {
	// SetRelayConnected 标记 Relay 连接完成
	SetRelayConnected()
	// SetRelayConfigured 标记已配置 Relay
	SetRelayConfigured()
	// OnRelayCircuitStateChanged 电路状态变更回调（可选实现）
	// 当电路从 Active→Stale 时表示可达性下降
	// 当电路从 Stale→Active 时表示可达性恢复
	OnRelayCircuitStateChanged(remotePeer string, oldState, newState string)
}

// NewManager 创建中继管理器
func NewManager(config *Config, swarm pkgif.Swarm, eventbus pkgif.EventBus, peerstore pkgif.Peerstore, host pkgif.Host) *Manager {
	if config == nil {
		config = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	m := &Manager{
		config:           config,
		swarm:            swarm,
		host:             host,
		eventbus:         eventbus,
		peerstore:        peerstore,
		backupRelayConns: make(map[string]pkgif.Connection),
		ctx:              ctx,
		cancel:           cancel,
	}

	// 初始化 Hole Puncher
	if swarm != nil && host != nil {
		m.holePuncher = holepunch.NewHolePuncher(swarm, host)
	}

	return m
}

// SetAutoRelay 设置 AutoRelay
//
// v2.0 新增：用于自动中继管理
func (m *Manager) SetAutoRelay(ar pkgif.AutoRelay) {
	m.autoRelayMu.Lock()
	m.autoRelay = ar
	m.autoRelayMu.Unlock()

	// 如果 Coordinator 已设置，立即绑定回调
	m.bindAutoRelayCallback()
}

// SetCoordinator 设置可达性协调器
//
// v2.0 新增：用于将 AutoRelay 地址回调到广告地址
func (m *Manager) SetCoordinator(c pkgif.ReachabilityCoordinator) {
	m.coordinatorMu.Lock()
	m.coordinator = c
	m.coordinatorMu.Unlock()

	// 如果 AutoRelay 已设置，立即绑定回调
	m.bindAutoRelayCallback()
}

// SetLifecycleCoordinator 设置生命周期协调器
//
// 用于 Relay 连接完成后通知 A5 gate 解除
func (m *Manager) SetLifecycleCoordinator(lc LifecycleCoordinatorInterface) {
	m.lifecycleCoordMu.Lock()
	m.lifecycleCoordinator = lc
	m.lifecycleCoordMu.Unlock()

	// 重新绑定 AutoRelay 回调以包含 lifecycle 通知
	m.bindAutoRelayCallback()
}

// HasConfiguredRelay 检查是否配置了 Relay
//
// 用于判断是否需要等待 Relay 连接完成
func (m *Manager) HasConfiguredRelay() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.relay != nil && m.relay.addr != nil
}

// bindAutoRelayCallback 绑定 AutoRelay 地址变更回调
func (m *Manager) bindAutoRelayCallback() {
	m.autoRelayMu.RLock()
	ar := m.autoRelay
	m.autoRelayMu.RUnlock()

	m.coordinatorMu.RLock()
	coord := m.coordinator
	m.coordinatorMu.RUnlock()

	m.lifecycleCoordMu.RLock()
	lc := m.lifecycleCoordinator
	m.lifecycleCoordMu.RUnlock()

	if ar != nil && coord != nil {
		// 创建包装回调，同时通知 Lifecycle Coordinator 和 Reachability Coordinator
		ar.SetOnAddrsChanged(func(addrs []string) {
			// 先通知 Lifecycle Coordinator（用于 A5 gate 解除）
			// 必须在 coord.OnRelayReserved 之前调用，因为 OnRelayReserved 会触发地址变更回调
			if lc != nil && len(addrs) > 0 {
				lc.SetRelayConnected()
			}

			// 再通知 Reachability Coordinator（用于地址通告，会触发地址变更回调）
			coord.OnRelayReserved(addrs)
		})
		logger.Info("AutoRelay 地址变更回调已绑定到 Coordinator")
	}
}

// GetAutoRelay 获取 AutoRelay
func (m *Manager) GetAutoRelay() pkgif.AutoRelay {
	m.autoRelayMu.RLock()
	defer m.autoRelayMu.RUnlock()
	return m.autoRelay
}

// Start 启动管理器
func (m *Manager) Start(ctx context.Context) error {
	if m.closed.Load() {
		return ErrServiceClosed
	}

	logger.Info("正在启动 Relay 管理器",
		"enableClient", m.config.EnableClient,
		"enableServer", m.config.EnableServer)

	// 初始化统一 Relay 服务
	if m.config.EnableClient || m.config.EnableServer {
		relayService, err := NewRelayService(m.swarm, m.host)
		if err != nil {
			logger.Error("创建 Relay 服务失败", "error", err)
			return err
		}
		relayService.SetServerConfig(m.config)
		m.relay = relayService

		// 启动 Relay 服务端
		if m.config.EnableServer {
			if err := m.relay.Enable(ctx); err != nil {
				logger.Error("启动 Relay 服务端失败", "error", err)
				return err
			}
			logger.Info("Relay 服务端已启动")
		}

		// 客户端注册 STOP 处理器
		if m.config.EnableClient && !m.config.EnableServer && m.host != nil {
			m.registerClientStopHandler()
			logger.Debug("已注册客户端 STOP 处理器")
		}
	}

	// v2.0 新增：绑定 AutoRelay 回调（如果已设置）
	m.bindAutoRelayCallback()

	// 订阅 Reachability 变化事件
	if m.eventbus != nil {
		m.wg.Add(1)
		go m.subscribeReachabilityEvents()
		logger.Debug("已订阅可达性变化事件")
	}

	// 将 Manager 注入到 Swarm 作为 RelayDialer
	if m.swarm != nil {
		if setter, ok := m.swarm.(interface {
			SetRelayDialer(pkgif.RelayDialer)
		}); ok {
			setter.SetRelayDialer(m)
			logger.Debug("已将 RelayManager 注入到 Swarm 作为 RelayDialer")
		}
	}

	logger.Info("Relay 管理器启动成功")
	return nil
}

// Stop 停止管理器
func (m *Manager) Stop() error {
	if !m.closed.CompareAndSwap(false, true) {
		return nil
	}

	logger.Info("正在停止 Relay 管理器")

	if m.cancel != nil {
		m.cancel()
	}

	// 停止 Relay 服务
	if m.relay != nil {
		m.relay.Disconnect()
		logger.Debug("Relay 已断开")
	}

	m.wg.Wait()

	logger.Info("Relay 管理器已停止")
	return nil
}

// Relay 返回统一 Relay 服务
func (m *Manager) Relay() *RelayService {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.relay
}

// ════════════════════════════════════════════════════════════════════════════
// RelayManager 接口实现（v2.0 统一接口）
// ════════════════════════════════════════════════════════════════════════════

// EnableRelay 启用 Relay 能力
func (m *Manager) EnableRelay(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.relay == nil {
		relayService, err := NewRelayService(m.swarm, m.host)
		if err != nil {
			return err
		}
		m.relay = relayService
	}

	return m.relay.Enable(ctx)
}

// DisableRelay 禁用 Relay 能力
func (m *Manager) DisableRelay(ctx context.Context) error {
	m.mu.RLock()
	relay := m.relay
	m.mu.RUnlock()

	if relay == nil {
		return nil
	}

	return relay.Disable(ctx)
}

// IsRelayEnabled 检查 Relay 能力是否已启用
func (m *Manager) IsRelayEnabled() bool {
	m.mu.RLock()
	relay := m.relay
	m.mu.RUnlock()

	if relay == nil {
		return false
	}

	return relay.IsEnabled()
}

// RelayStats 返回 Relay 统计信息
func (m *Manager) RelayStats() pkgif.RelayStats {
	m.mu.RLock()
	relay := m.relay
	m.mu.RUnlock()

	if relay == nil {
		return pkgif.RelayStats{}
	}

	return relay.Stats()
}

// SetRelayAddr 设置要使用的 Relay 地址
//
// 时序对齐（Phase A5）：设置 Relay 后通知 lifecycle coordinator
func (m *Manager) SetRelayAddr(addr types.Multiaddr) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.relay == nil {
		relayService, err := NewRelayService(m.swarm, m.host)
		if err != nil {
			return err
		}
		m.relay = relayService
	}

	if err := m.relay.SetRelay(addr); err != nil {
		return err
	}

	// 通知 lifecycle coordinator 已配置 Relay
	m.lifecycleCoordMu.RLock()
	lc := m.lifecycleCoordinator
	m.lifecycleCoordMu.RUnlock()
	if lc != nil {
		lc.SetRelayConfigured()
	}

	return nil
}

// RemoveRelayAddr 移除 Relay 地址配置
func (m *Manager) RemoveRelayAddr() error {
	m.mu.RLock()
	relay := m.relay
	m.mu.RUnlock()

	if relay == nil {
		return nil
	}

	return relay.RemoveRelay()
}

// RelayAddr 获取当前配置的 Relay 地址
func (m *Manager) RelayAddr() (types.Multiaddr, bool) {
	m.mu.RLock()
	relay := m.relay
	m.mu.RUnlock()

	if relay == nil {
		return nil, false
	}

	return relay.Relay()
}

// ════════════════════════════════════════════════════════════════════════════
// RelayDialer 接口实现
// ════════════════════════════════════════════════════════════════════════════

// DialViaRelay 通过 Relay 连接目标节点
func (m *Manager) DialViaRelay(ctx context.Context, target string) (pkgif.Connection, error) {
	if m.closed.Load() {
		return nil, ErrServiceClosed
	}

	m.mu.RLock()
	relay := m.relay
	m.mu.RUnlock()

	if relay != nil {
		conn, err := relay.DialViaRelay(ctx, target)
		if err == nil {
			return conn, nil
		}
		logger.Debug("Relay 连接失败", "target", target, "error", err)
		return nil, err
	}

	return nil, ErrNoRelayAvailable
}

// HasRelay 检查是否配置了 Relay
func (m *Manager) HasRelay() bool {
	if m.closed.Load() {
		return false
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.relay != nil {
		if addr, ok := m.relay.Relay(); ok && addr != nil {
			return true
		}
	}

	// 检查 AutoRelay 是否有活跃的中继
	m.autoRelayMu.RLock()
	ar := m.autoRelay
	m.autoRelayMu.RUnlock()

	if ar != nil && len(ar.Relays()) > 0 {
		return true
	}

	return false
}

// ════════════════════════════════════════════════════════════════════════════
// RelayManagerInterface 实现（v2.0 NAT Service 所需）
// ════════════════════════════════════════════════════════════════════════════

// EnableClient 启用 Relay 客户端
//
// v2.0 实现：启动 AutoRelay 自动发现并获取中继预留
// 实现 nat.RelayManagerInterface
func (m *Manager) EnableClient(ctx context.Context) error {
	if m.closed.Load() {
		return ErrServiceClosed
	}

	m.autoRelayMu.Lock()
	ar := m.autoRelay
	m.autoRelayMu.Unlock()

	if ar == nil {
		logger.Warn("AutoRelay 未设置，无法启用 Relay 客户端")
		return ErrNoRelayAvailable
	}

	// 启用 AutoRelay
	ar.Enable()

	// 如果 AutoRelay 尚未运行，启动它
	if err := ar.Start(ctx); err != nil {
		// 如果已经在运行，忽略错误
		logger.Debug("启动 AutoRelay", "result", err)
	}

	logger.Info("Relay 客户端已启用")
	return nil
}

// ConnectAndRegister 连接 Relay 并注册地址簿（A5 时序对齐）
//
// 时序对齐（Phase A5）：
//  1. 连接配置的 Relay
//  2. 注册到 Relay 地址簿
//  3. 通知 lifecycle coordinator
//
// 如果没有配置 Relay，直接通知 lifecycle coordinator。
func (m *Manager) ConnectAndRegister(ctx context.Context) error {
	if m.closed.Load() {
		return ErrServiceClosed
	}

	// 检查是否配置了 Relay
	m.mu.RLock()
	relay := m.relay
	hasRelay := relay != nil && relay.addr != nil
	m.mu.RUnlock()

	if !hasRelay {
		// 没有配置 Relay，直接通知 lifecycle
		m.notifyRelayConnected()
		logger.Debug("未配置 Relay，跳过连接")
		return nil
	}

	// 连接 Relay
	if err := relay.Connect(ctx); err != nil {
		logger.Warn("A5 Relay 连接失败", "error", err)
		// 连接失败仍然通知 lifecycle，使用 Unknown NAT 继续
		m.notifyRelayConnected()
		return err
	}

	// 注册到地址簿（A5）
	if err := m.RegisterToRelay(ctx); err != nil {
		logger.Warn("A5 地址簿注册失败", "error", err)
		// 注册失败不影响连接状态
	}

	// 通知 lifecycle coordinator
	m.notifyRelayConnected()

	logger.Info("A5 Relay 连接并注册完成")
	return nil
}

// notifyRelayConnected 通知 Relay 连接完成
func (m *Manager) notifyRelayConnected() {
	m.lifecycleCoordMu.RLock()
	lc := m.lifecycleCoordinator
	m.lifecycleCoordMu.RUnlock()

	if lc != nil {
		lc.SetRelayConnected()
	}
}

// RegisterToRelay 注册到 Relay 地址簿
//
// v2.0 实现：使用 AddressBookService 向 Relay 注册本节点地址
// 实现 nat.RelayManagerInterface
func (m *Manager) RegisterToRelay(ctx context.Context) error {
	if m.closed.Load() {
		return ErrServiceClosed
	}

	m.addressBookServiceMu.RLock()
	abs := m.addressBookService
	m.addressBookServiceMu.RUnlock()

	if abs == nil {
		logger.Debug("AddressBookService 未设置，跳过 Relay 注册")
		return nil
	}

	// 获取当前配置的 Relay 地址
	relayPeerID := ""

	// 1. 优先使用静态配置的 Relay
	m.mu.RLock()
	if m.relay != nil {
		if addr, ok := m.relay.Relay(); ok && addr != nil {
			relayPeerID = extractNodeID(addr)
		}
	}
	m.mu.RUnlock()

	// 2. 如果没有静态配置，使用 AutoRelay 的活跃中继
	if relayPeerID == "" {
		m.autoRelayMu.RLock()
		ar := m.autoRelay
		m.autoRelayMu.RUnlock()

		if ar != nil {
			relays := ar.Relays()
			if len(relays) > 0 {
				relayPeerID = relays[0]
			}
		}
	}

	if relayPeerID == "" {
		logger.Debug("没有可用的 Relay，跳过注册")
		return nil
	}

	// 使用带超时的 context
	regCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := abs.RegisterSelf(regCtx, relayPeerID); err != nil {
		logger.Warn("注册到 Relay 地址簿失败", "relay", relayPeerID, "error", err)
		return err
	}

	logger.Info("已注册到 Relay 地址簿", "relay", relayPeerID)
	return nil
}

// SetAddressBookService 设置地址簿服务
//
// v2.0 新增：用于向 Relay 注册地址
func (m *Manager) SetAddressBookService(abs *addressbook.AddressBookService) {
	m.addressBookServiceMu.Lock()
	m.addressBookService = abs
	m.addressBookServiceMu.Unlock()
}

// GetAutoRelayAddrs 获取 AutoRelay 的所有中继地址
//
// v2.0 新增：用于传输层获取多候选中继
func (m *Manager) GetAutoRelayAddrs() []string {
	m.autoRelayMu.RLock()
	ar := m.autoRelay
	m.autoRelayMu.RUnlock()

	if ar == nil {
		return nil
	}

	return ar.RelayAddrs()
}

// ════════════════════════════════════════════════════════════════════════════
// 优先级拨号
// ════════════════════════════════════════════════════════════════════════════

// DialWithPriority 按优先级拨号（连接优先级：直连 > 打洞 > 中继）
func (m *Manager) DialWithPriority(ctx context.Context, target string, _ string) (pkgif.Connection, error) {
	if m.closed.Load() {
		return nil, ErrServiceClosed
	}

	// 1. 尝试已有连接
	conn := m.getExistingConnection(target)
	if conn != nil {
		return conn, nil
	}

	// 2. 尝试直连
	conn, err := m.dialDirect(ctx, target)
	if err == nil {
		return conn, nil
	}

	// 3. 尝试打洞
	conn, err = m.dialHolePunch(ctx, target)
	if err == nil {
		return conn, nil
	}

	// 4. 尝试中继
	if m.relay != nil {
		return m.relay.DialViaRelay(ctx, target)
	}

	return nil, ErrNoRelayAvailable
}

// getExistingConnection 获取已有连接
func (m *Manager) getExistingConnection(target string) pkgif.Connection {
	if m.swarm == nil {
		return nil
	}

	conns := m.swarm.ConnsToPeer(target)
	if len(conns) > 0 {
		return conns[0]
	}

	return nil
}

// dialDirect 尝试直连
func (m *Manager) dialDirect(ctx context.Context, target string) (pkgif.Connection, error) {
	if m.swarm == nil {
		return nil, ErrNoRelayAvailable
	}

	return m.swarm.DialPeer(ctx, target)
}

// dialHolePunch 尝试打洞
//
// v2.0 改进：打洞成功后保留 Relay 连接作为备份
// - 如果打洞前有 Relay 连接，打洞成功后保留为 backup
// - backup 连接可用于后续打洞信令或快速恢复
func (m *Manager) dialHolePunch(ctx context.Context, target string) (pkgif.Connection, error) {
	if m.holePuncher == nil {
		return nil, ErrNoRelayAvailable
	}

	// 获取目标地址
	addrs := m.getAddrsForPeer(target)
	if len(addrs) == 0 {
		return nil, ErrNoRelayAvailable
	}

	// v2.0 新增：记录打洞前的 Relay 连接
	var relayConn pkgif.Connection
	if m.swarm != nil {
		conns := m.swarm.ConnsToPeer(target)
		for _, conn := range conns {
			if m.isRelayConnection(conn) {
				relayConn = conn
				break
			}
		}
	}

	// 尝试打洞
	if err := m.holePuncher.DirectConnect(ctx, target, addrs); err != nil {
		return nil, err
	}

	// 打洞成功后获取新建立的连接
	if m.swarm != nil {
		conns := m.swarm.ConnsToPeer(target)
		for _, conn := range conns {
			// 返回直连连接（非 Relay）
			if !m.isRelayConnection(conn) {
				// v2.0 新增：保留 Relay 连接作为备份
				if relayConn != nil && m.config.KeepRelayAfterHolePunch {
					m.saveBackupRelayConn(target, relayConn)
					logger.Debug("打洞成功，保留 Relay 连接作为备份",
						"target", target[:8],
						"directConn", conn.RemoteMultiaddr().String())
				}
				return conn, nil
			}
		}
		// 如果只有 Relay 连接，也返回
		if len(conns) > 0 {
			return conns[0], nil
		}
	}

	return nil, ErrNoRelayAvailable
}

// isRelayConnection 判断是否是 Relay 连接
func (m *Manager) isRelayConnection(conn pkgif.Connection) bool {
	if conn == nil {
		return false
	}
	addr := conn.RemoteMultiaddr()
	if addr == nil {
		return false
	}
	// Relay 连接的地址包含 /p2p-circuit/
	return strings.Contains(addr.String(), "/p2p-circuit/")
}

// saveBackupRelayConn 保存备份 Relay 连接
func (m *Manager) saveBackupRelayConn(target string, conn pkgif.Connection) {
	m.backupRelayConnsMu.Lock()
	defer m.backupRelayConnsMu.Unlock()

	// 如果已有旧的备份，不替换（保持连接稳定性）
	if _, exists := m.backupRelayConns[target]; exists {
		return
	}

	m.backupRelayConns[target] = conn
}

// GetBackupRelayConn 获取备份 Relay 连接
//
// v2.0 新增：用于在直连断开时快速恢复。
func (m *Manager) GetBackupRelayConn(target string) pkgif.Connection {
	m.backupRelayConnsMu.RLock()
	defer m.backupRelayConnsMu.RUnlock()
	return m.backupRelayConns[target]
}

// RemoveBackupRelayConn 移除备份 Relay 连接
func (m *Manager) RemoveBackupRelayConn(target string) {
	m.backupRelayConnsMu.Lock()
	defer m.backupRelayConnsMu.Unlock()
	delete(m.backupRelayConns, target)
}

// getAddrsForPeer 获取节点地址
func (m *Manager) getAddrsForPeer(target string) []string {
	if m.peerstore == nil {
		return []string{}
	}

	peerID := types.PeerID(target)
	addrs := m.peerstore.Addrs(peerID)

	if len(addrs) == 0 {
		return []string{}
	}

	result := make([]string, len(addrs))
	for i, addr := range addrs {
		result[i] = addr.String()
	}

	return result
}

// ════════════════════════════════════════════════════════════════════════════
// 事件处理
// ════════════════════════════════════════════════════════════════════════════

// subscribeReachabilityEvents 订阅可达性变化事件
func (m *Manager) subscribeReachabilityEvents() {
	defer m.wg.Done()

	if m.eventbus == nil {
		return
	}

	localSub, err := m.eventbus.Subscribe(&nat.ReachabilityChangedEvent{})
	if err != nil {
		return
	}
	defer localSub.Close()

	for {
		select {
		case <-m.ctx.Done():
			return
		case event := <-localSub.Out():
			if e, ok := event.(*nat.ReachabilityChangedEvent); ok {
				m.handleLocalReachabilityChanged(e)
			}
		}
	}
}

// handleLocalReachabilityChanged 处理本地可达性变化事件
func (m *Manager) handleLocalReachabilityChanged(_ *nat.ReachabilityChangedEvent) {
	// 本地节点可达性变化：不需要加入候选池
	// 本地节点不应该成为自己的中继
}

// ════════════════════════════════════════════════════════════════════════════
// 连接管理
// ════════════════════════════════════════════════════════════════════════════

// CloseStaleConnections 关闭失效的中继连接
func (m *Manager) CloseStaleConnections(_ context.Context) error {
	m.mu.RLock()
	relay := m.relay
	m.mu.RUnlock()

	logger.Info("检查并关闭失效的中继连接")

	// 获取当前本地有效地址
	localAddrs := m.getLocalAddrs()
	localAddrMap := make(map[string]bool)
	for _, addr := range localAddrs {
		localAddrMap[addr] = true
	}

	closedCount := 0

	// 检查 Relay 连接
	if relay != nil && relay.IsConnected() {
		relayAddr, hasRelay := relay.Relay()
		if hasRelay && relayAddr != nil {
			relayID := extractNodeID(relayAddr)
			if relayID != "" && m.swarm != nil {
				conns := m.swarm.ConnsToPeer(relayID)
				for _, conn := range conns {
					if conn == nil {
						continue
					}

					localMA := conn.LocalMultiaddr()
					if localMA != nil {
						sourceAddr := localMA.String()
						if !m.isAddrStillValid(sourceAddr, localAddrMap) {
							logger.Info("Relay 连接源地址已失效，关闭连接",
								"sourceAddr", sourceAddr,
								"relayID", relayID)
							relay.Disconnect()
							closedCount++
							break
						}
					}
				}
			}
		}
	}

	if closedCount > 0 {
		logger.Info("关闭失效中继连接完成", "count", closedCount)
	} else {
		logger.Debug("没有需要关闭的失效中继连接")
	}

	return nil
}

// isAddrStillValid 检查地址是否仍然有效
func (m *Manager) isAddrStillValid(sourceAddr string, localAddrMap map[string]bool) bool {
	if len(localAddrMap) == 0 {
		return true
	}

	if localAddrMap[sourceAddr] {
		return true
	}

	sourceIP := extractIPFromMultiaddr(sourceAddr)
	if sourceIP == "" {
		return true
	}

	for localAddr := range localAddrMap {
		localIP := extractIPFromMultiaddr(localAddr)
		if localIP == sourceIP {
			return true
		}
	}

	return false
}

// extractIPFromMultiaddr 从 multiaddr 中提取 IP 地址
func extractIPFromMultiaddr(addr string) string {
	if strings.HasPrefix(addr, "/ip4/") {
		parts := strings.Split(addr[5:], "/")
		if len(parts) > 0 {
			return parts[0]
		}
	} else if strings.HasPrefix(addr, "/ip6/") {
		parts := strings.Split(addr[5:], "/")
		if len(parts) > 0 {
			return parts[0]
		}
	}
	return ""
}

// getLocalAddrs 获取当前本地有效地址
func (m *Manager) getLocalAddrs() []string {
	if m.swarm == nil {
		return []string{}
	}

	addrSet := make(map[string]bool)

	conns := m.swarm.Conns()
	for _, conn := range conns {
		if conn == nil {
			continue
		}

		localMA := conn.LocalMultiaddr()
		if localMA != nil {
			addrStr := localMA.String()
			if addrStr != "" {
				addrSet[addrStr] = true
			}
		}
	}

	addrs := make([]string, 0, len(addrSet))
	for addr := range addrSet {
		addrs = append(addrs, addr)
	}

	return addrs
}

// registerClientStopHandler 为客户端注册 STOP 协议处理器
//
// v2.0 重构：使用 RelayCircuit 支持多路复用
// 在 STOP 握手完成后，创建 yamux muxer，启动流接受循环。
func (m *Manager) registerClientStopHandler() {
	if m.host == nil {
		return
	}

	stopProtocolID := string(protocol.RelayStop)

	m.host.SetStreamHandler(stopProtocolID, func(stream pkgif.Stream) {
		header := make([]byte, 5)
		if _, err := stream.Read(header); err != nil {
			logger.Debug("读取 STOP 消息头失败", "error", err)
			stream.Close()
			return
		}

		msgType := header[0]
		if msgType != 1 { // MsgTypeConnect = 1
			logger.Debug("收到非 CONNECT 类型的 STOP 消息", "type", msgType)
			stream.Close()
			return
		}

		length := uint32(header[1])<<24 | uint32(header[2])<<16 | uint32(header[3])<<8 | uint32(header[4])
		var srcPeerID types.PeerID
		srcShort := "unknown"
		if length > 0 && length < 1024 {
			srcPeer := make([]byte, length)
			if _, err := stream.Read(srcPeer); err == nil {
				srcPeerID = types.PeerID(srcPeer)
				srcShort = string(srcPeer)
				if len(srcShort) > 8 {
					srcShort = srcShort[:8]
				}
			}
		}
		logger.Debug("收到中继连接请求", "from", srcShort)

		response := []byte{2, 0, 0, 0, 1, 0}
		if _, err := stream.Write(response); err != nil {
			logger.Debug("发送 STOP 响应失败", "error", err)
			stream.Close()
			return
		}

		// v2.0 重构：创建 RelayCircuit（支持多路复用）
		//
		// 在 STOP 握手完成后的流上叠加 yamux muxer：
		// - 原来：stream 直接传递给 HandleInboundStream，只能处理一次协议协商
		// - 现在：创建 RelayCircuit，启动流接受循环，支持多个并发流
		if srcPeerID == "" {
			// 无法识别来源节点，回退到旧逻辑
			logger.Warn("无法识别来源节点，使用旧处理逻辑")
			m.host.HandleInboundStream(stream)
			return
		}

		// 获取中继节点 ID（STOP 流的直接对端是 Relay）
		relayPeerID := stream.Conn().RemotePeer()

		//
		relayTransportAddr := extractTransportAddrFromConn(stream.Conn())

		// 创建状态变更回调（用于通知生命周期协调器）
		var onStateChanged func(oldState, newState client.CircuitState)
		m.lifecycleCoordMu.RLock()
		lc := m.lifecycleCoordinator
		m.lifecycleCoordMu.RUnlock()
		if lc != nil {
			remotePeerStr := string(srcPeerID)
			onStateChanged = func(oldState, newState client.CircuitState) {
				lc.OnRelayCircuitStateChanged(remotePeerStr, oldState.String(), newState.String())
			}
		}

		// 创建 RelayCircuit（带 eventbus 和状态变更回调）
		//
		circuit, err := client.CreateRelayCircuitFromStreamWithOptions(
			stream,
			true, // isServer - 接收方
			types.PeerID(m.swarm.LocalPeer()),
			srcPeerID,
			relayPeerID,
			relayTransportAddr,
			m.eventbus,
			onStateChanged,
		)
		if err != nil {
			logger.Error("创建中继电路失败", "error", err, "from", srcShort)
			stream.Close()
			return
		}

		logger.Info("中继电路已建立（多路复用）",
			"from", srcShort,
			"relay", safePeerIDPrefix(relayPeerID),
			"relayTransportAddr", relayTransportAddr)

		// v2.0 关键修复：将电路注册到 Swarm 连接池
		//
		// 这样其他代码可以通过 Swarm.ConnsToPeer() 找到这个电路，
		// 并通过 Swarm.NewStream() 使用它发送消息。
		m.swarm.AddInboundConnection(circuit)

		// 启动流接受循环（后台 goroutine）
		go circuit.AcceptStreamLoop(m.host)
	})
}

// Close 关闭管理器
func (m *Manager) Close() error {
	return m.Stop()
}

// extractTransportAddrFromConn 从连接中提取传输层地址（不含 /p2p/<ID>）
//
// 输入: /ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelay...
// 输出: /ip4/1.2.3.4/udp/4001/quic-v1
func extractTransportAddrFromConn(conn pkgif.Connection) types.Multiaddr {
	if conn == nil {
		return nil
	}
	addr := conn.RemoteMultiaddr()
	if addr == nil {
		return nil
	}
	addrStr := addr.String()
	// 找到 /p2p/ 的位置，截取之前的部分
	idx := strings.LastIndex(addrStr, "/p2p/")
	if idx == -1 {
		return addr // 没有 /p2p/ 后缀，直接返回
	}
	transportStr := addrStr[:idx]
	if transportStr == "" {
		return nil
	}
	transportAddr, err := multiaddr.NewMultiaddr(transportStr)
	if err != nil {
		logger.Warn("提取传输地址失败", "addr", addrStr, "error", err)
		return nil
	}
	return transportAddr
}
