// Package lifecycle 提供节点生命周期协调器
//
// 对齐设计文档 20260125-node-lifecycle-cross-cutting.md 中的阶段定义：
//   - Phase A (冷启动): A1→A6
//   - Phase B (Realm Join): B1→B4
//   - Phase C (稳态运行)
//   - Phase D (优雅关闭)
//
// 本模块的核心职责：
//   1. 定义生命周期阶段 gate
//   2. 提供基于信号的显式依赖机制
//   3. 确保各阶段按序推进，避免"缝缝补补"式重试
package lifecycle

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("core/lifecycle")

// ============================================================================
//                              阶段定义
// ============================================================================

// Phase 生命周期阶段
type Phase int

const (
	// PhaseCreated 节点已创建，未启动
	PhaseCreated Phase = iota

	// ═══════════════════════════ Phase A: 冷启动 ═══════════════════════════

	// PhaseA1IdentityInit A1: 身份初始化
	// 生成/加载 PrivateKey，派生 NodeID
	PhaseA1IdentityInit

	// PhaseA2TransportStart A2: 传输层启动
	// 绑定端口、监听地址
	PhaseA2TransportStart

	// PhaseA3AddressDiscovery A3: 地址发现
	// STUN/UPnP/NAT-PMP/网卡扫描
	PhaseA3AddressDiscovery

	// PhaseA4NetworkJoin A4: 网络接入
	// Bootstrap DHT、连接引导节点
	PhaseA4NetworkJoin

	// PhaseA5AddressPublish A5: 地址发布
	// 发布全局 PeerRecord 到 DHT
	// 关键 Gate: 必须在地址（含 Relay 地址）就绪后才能发布
	PhaseA5AddressPublish

	// PhaseA6Ready A6: 冷启动完成
	// Node.ReadyLevel = Reachable
	PhaseA6Ready

	// ═══════════════════════════ Phase B: Realm Join ═══════════════════════

	// PhaseB1PSKAuth B1: PSK 认证
	// 加入 Realm 时的预共享密钥认证
	PhaseB1PSKAuth

	// PhaseB2MemberDiscovery B2: 成员发现
	// 通过 Discovery/Rendezvous 或 Join 协议获取成员列表
	PhaseB2MemberDiscovery

	// PhaseB3DHTPublish B3: DHT 发布
	// 发布 Provider Record 和 PeerRecord 到 DHT
	// 关键 Gate: 必须在 A5 完成后执行
	PhaseB3DHTPublish

	// 注意：B4 阶段已移除
	// 地址簿注册已移至 A5（Node 级别），由 Relay Manager 在连接时自动执行
	// 参见 internal/core/relay/manager.go ConnectAndRegister()

	// ═══════════════════════════ Phase C: 稳态运行 ═══════════════════════════

	// PhaseCRunning 稳态运行
	PhaseCRunning

	// ═══════════════════════════ Phase D: 优雅关闭 ═══════════════════════════

	// PhaseD1NotifyLeave D1: 通知离开
	// 广播离开消息给成员
	PhaseD1NotifyLeave

	// PhaseD2DrainConnections D2: 排空连接
	// 等待进行中的请求完成
	PhaseD2DrainConnections

	// PhaseD3UnpublishDHT D3: 取消发布 DHT
	// 从 DHT 移除 PeerRecord
	PhaseD3UnpublishDHT

	// PhaseD4Shutdown D4: 关闭完成
	PhaseD4Shutdown
)

// String 返回阶段字符串表示
func (p Phase) String() string {
	switch p {
	case PhaseCreated:
		return "created"
	case PhaseA1IdentityInit:
		return "A1:identity_init"
	case PhaseA2TransportStart:
		return "A2:transport_start"
	case PhaseA3AddressDiscovery:
		return "A3:address_discovery"
	case PhaseA4NetworkJoin:
		return "A4:network_join"
	case PhaseA5AddressPublish:
		return "A5:address_publish"
	case PhaseA6Ready:
		return "A6:ready"
	case PhaseB1PSKAuth:
		return "B1:psk_auth"
	case PhaseB2MemberDiscovery:
		return "B2:member_discovery"
	case PhaseB3DHTPublish:
		return "B3:dht_publish"
	case PhaseCRunning:
		return "C:running"
	case PhaseD1NotifyLeave:
		return "D1:notify_leave"
	case PhaseD2DrainConnections:
		return "D2:drain_connections"
	case PhaseD3UnpublishDHT:
		return "D3:unpublish_dht"
	case PhaseD4Shutdown:
		return "D4:shutdown"
	default:
		return fmt.Sprintf("unknown(%d)", p)
	}
}

// ============================================================================
//                              生命周期协调器
// ============================================================================

// Coordinator 生命周期协调器
//
// 核心职责：
//   1. 追踪当前生命周期阶段
//   2. 提供阶段 gate（等待特定阶段完成）
//   3. 通知阶段变更
//   4. 确保阶段按序推进
type Coordinator struct {
	mu sync.RWMutex

	// 当前阶段
	phase Phase

	// 阶段完成信号 map
	// key: 阶段, value: 已关闭的 channel（表示该阶段已完成）
	phaseSignals map[Phase]chan struct{}

	// 地址就绪信号（A5 关键 gate）
	// 关闭后表示 Relay 地址已就绪，可以进行 DHT 发布
	addressReadyChan chan struct{}
	addressReady     bool

	// NAT 类型就绪信号（A3 关键 gate）
	// 关闭后表示 NAT 类型检测完成，可以进行后续决策
	natTypeReadyChan chan struct{}
	natTypeReady     bool

	// Relay 连接完成信号（A5 前置 gate）
	// 关闭后表示配置的 Relay 已连接，可以获取 Relay 地址
	relayConnectedChan chan struct{}
	relayConnected     bool
	relayConfigured    bool // 是否配置了 Relay

	// 阶段变更回调
	onPhaseChange []func(old, new Phase)

	// 上下文
	ctx    context.Context
	cancel context.CancelFunc
}

// NewCoordinator 创建生命周期协调器
func NewCoordinator() *Coordinator {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Coordinator{
		phase:              PhaseCreated,
		phaseSignals:       make(map[Phase]chan struct{}),
		addressReadyChan:   make(chan struct{}),
		natTypeReadyChan:   make(chan struct{}),
		relayConnectedChan: make(chan struct{}),
		ctx:                ctx,
		cancel:             cancel,
	}

	// 初始化所有阶段信号
	for p := PhaseCreated; p <= PhaseD4Shutdown; p++ {
		c.phaseSignals[p] = make(chan struct{})
	}

	return c
}

// ============================================================================
//                              阶段管理
// ============================================================================

// Phase 返回当前阶段
func (c *Coordinator) Phase() Phase {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.phase
}

// AdvanceTo 推进到指定阶段
//
// 规则：
//   - 只能向前推进，不能后退
//   - 会自动完成中间所有阶段的信号
//
// 返回:
//   - error: 如果尝试后退或阶段无效
func (c *Coordinator) AdvanceTo(target Phase) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if target < c.phase {
		return fmt.Errorf("cannot advance backwards: current=%s target=%s", c.phase, target)
	}

	if target == c.phase {
		return nil // 已在目标阶段
	}

	old := c.phase

	// 完成从当前到目标的所有中间阶段信号
	for p := c.phase; p <= target; p++ {
		if ch, ok := c.phaseSignals[p]; ok {
			select {
			case <-ch:
				// 已关闭
			default:
				close(ch)
			}
		}
	}

	c.phase = target

	logger.Info("生命周期阶段推进",
		"from", old.String(),
		"to", target.String())

	// 通知回调（在释放锁后调用）
	callbacks := make([]func(old, new Phase), len(c.onPhaseChange))
	copy(callbacks, c.onPhaseChange)

	// 异步通知，避免回调阻塞
	go func() {
		for _, cb := range callbacks {
			cb(old, target)
		}
	}()

	return nil
}

// Complete 标记指定阶段完成
//
// 与 AdvanceTo 类似，但只完成单个阶段的信号，不改变当前阶段。
// 用于标记某个子任务完成（如地址发现完成）。
func (c *Coordinator) Complete(phase Phase) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ch, ok := c.phaseSignals[phase]; ok {
		select {
		case <-ch:
			// 已完成
		default:
			close(ch)
			logger.Debug("阶段信号已完成", "phase", phase.String())
		}
	}
}

// WaitFor 等待指定阶段完成
//
// 阻塞直到目标阶段完成或上下文取消。
//
// 参数:
//   - ctx: 上下文
//   - phase: 目标阶段
//
// 返回:
//   - error: 上下文取消时返回 ctx.Err()
func (c *Coordinator) WaitFor(ctx context.Context, phase Phase) error {
	c.mu.RLock()
	ch := c.phaseSignals[phase]
	c.mu.RUnlock()

	if ch == nil {
		return fmt.Errorf("invalid phase: %d", phase)
	}

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.ctx.Done():
		return c.ctx.Err()
	}
}

// WaitForWithTimeout 带超时等待指定阶段完成
func (c *Coordinator) WaitForWithTimeout(phase Phase, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(c.ctx, timeout)
	defer cancel()
	return c.WaitFor(ctx, phase)
}

// IsCompleted 检查指定阶段是否已完成
func (c *Coordinator) IsCompleted(phase Phase) bool {
	c.mu.RLock()
	ch := c.phaseSignals[phase]
	c.mu.RUnlock()

	if ch == nil {
		return false
	}

	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// ============================================================================
//                              地址就绪 Gate（A5 关键依赖）
// ============================================================================

// SetAddressReady 标记地址已就绪
//
// 调用时机：Relay 地址获取成功后
// 效果：解除 WaitAddressReady 的阻塞，允许 DHT 发布
func (c *Coordinator) SetAddressReady() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.addressReady {
		return // 已就绪
	}

	c.addressReady = true
	close(c.addressReadyChan)

	logger.Info("地址就绪信号已设置（A5 gate 解除）")
}

// WaitAddressReady 等待地址就绪
//
// 阻塞直到地址（包括 Relay 地址）就绪或上下文取消。
// DHT 发布必须在此信号后执行。
//
// 参数:
//   - ctx: 上下文
//
// 返回:
//   - error: 上下文取消时返回 ctx.Err()
func (c *Coordinator) WaitAddressReady(ctx context.Context) error {
	c.mu.RLock()
	ch := c.addressReadyChan
	ready := c.addressReady
	c.mu.RUnlock()

	if ready {
		return nil
	}

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.ctx.Done():
		return c.ctx.Err()
	}
}

// IsAddressReady 检查地址是否已就绪
func (c *Coordinator) IsAddressReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.addressReady
}

// AddressReadyChan 返回地址就绪信号 channel
//
// 用于 select 语句中监听地址就绪事件。
func (c *Coordinator) AddressReadyChan() <-chan struct{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.addressReadyChan
}

// ============================================================================
//                              NAT 类型就绪 Gate（A3 关键依赖）
// ============================================================================

// SetNATTypeReady 标记 NAT 类型检测已完成
//
// 调用时机：NAT Service 完成 NAT 类型检测后（无论成功或失败）
// 效果：解除 WaitNATTypeReady 的阻塞，允许后续阶段使用 NAT 类型信息
func (c *Coordinator) SetNATTypeReady() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.natTypeReady {
		return // 已就绪
	}

	c.natTypeReady = true
	close(c.natTypeReadyChan)

	logger.Info("NAT 类型就绪信号已设置（A3 gate 解除）")
}

// WaitNATTypeReady 等待 NAT 类型检测完成
//
// 阻塞直到 NAT 类型检测完成或上下文取消。
// A4/A5 阶段的决策依赖 NAT 类型信息。
func (c *Coordinator) WaitNATTypeReady(ctx context.Context) error {
	c.mu.RLock()
	ch := c.natTypeReadyChan
	ready := c.natTypeReady
	c.mu.RUnlock()

	if ready {
		return nil
	}

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.ctx.Done():
		return c.ctx.Err()
	}
}

// IsNATTypeReady 检查 NAT 类型是否已就绪
func (c *Coordinator) IsNATTypeReady() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.natTypeReady
}

// ============================================================================
//                              Relay 连接 Gate（A5 前置依赖）
// ============================================================================

// SetRelayConfigured 标记已配置 Relay
//
// 调用时机：解析配置发现有 Relay 地址时
// 效果：标记需要等待 Relay 连接完成
func (c *Coordinator) SetRelayConfigured() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.relayConfigured = true
	logger.Debug("已标记 Relay 配置")
}

// IsRelayConfigured 检查是否配置了 Relay
func (c *Coordinator) IsRelayConfigured() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.relayConfigured
}

// SetRelayConnected 标记 Relay 连接已完成
//
// 调用时机：Relay Manager 成功连接到配置的 Relay 后
// 效果：解除 WaitRelayConnected 的阻塞，允许 A5 地址发布
func (c *Coordinator) SetRelayConnected() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.relayConnected {
		return // 已连接
	}

	c.relayConnected = true
	close(c.relayConnectedChan)

	logger.Info("Relay 连接就绪信号已设置（A5 前置 gate 解除）")
}

// OnRelayCircuitStateChanged 电路状态变更回调
//
// 当中继电路状态变化时被调用。用于监控电路健康状态。
// - Active→Stale: 可达性下降警告
// - Stale→Active: 可达性恢复
// - Active/Stale→Closed: 电路关闭
func (c *Coordinator) OnRelayCircuitStateChanged(remotePeer string, oldState, newState string) {
	short := remotePeer
	if len(short) > 8 {
		short = short[:8]
	}

	switch {
	case oldState == "active" && newState == "stale":
		logger.Warn("中继电路可达性下降",
			"remotePeer", short,
			"oldState", oldState,
			"newState", newState)
	case oldState == "stale" && newState == "active":
		logger.Info("中继电路可达性恢复",
			"remotePeer", short,
			"oldState", oldState,
			"newState", newState)
	case newState == "closed":
		logger.Info("中继电路已关闭",
			"remotePeer", short,
			"oldState", oldState)
	default:
		logger.Debug("中继电路状态变更",
			"remotePeer", short,
			"oldState", oldState,
			"newState", newState)
	}
}

// WaitRelayConnected 等待 Relay 连接完成
//
// 阻塞直到 Relay 连接完成或上下文取消。
// 仅在配置了 Relay 时才需要等待。
func (c *Coordinator) WaitRelayConnected(ctx context.Context) error {
	c.mu.RLock()
	ch := c.relayConnectedChan
	connected := c.relayConnected
	configured := c.relayConfigured
	c.mu.RUnlock()

	// 如果没有配置 Relay，直接返回
	if !configured {
		return nil
	}

	if connected {
		return nil
	}

	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-c.ctx.Done():
		return c.ctx.Err()
	}
}

// IsRelayConnected 检查 Relay 是否已连接
func (c *Coordinator) IsRelayConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.relayConnected
}

// ============================================================================
//                              回调管理
// ============================================================================

// OnPhaseChange 注册阶段变更回调
func (c *Coordinator) OnPhaseChange(callback func(old, new Phase)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onPhaseChange = append(c.onPhaseChange, callback)
}

// ============================================================================
//                              生命周期控制
// ============================================================================

// Stop 停止协调器
func (c *Coordinator) Stop() {
	c.cancel()
}

// Context 返回协调器上下文
func (c *Coordinator) Context() context.Context {
	return c.ctx
}
