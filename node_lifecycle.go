package dep2p

import (
	"context"
	"fmt"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/lifecycle"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ════════════════════════════════════════════════════════════════════════════
//                              生命周期常量
// ════════════════════════════════════════════════════════════════════════════

const (
	// initializeTimeout 初始化超时（Fx App Start）
	initializeTimeout = 30 * time.Second

	// readyCheckTimeout 就绪检查超时
	readyCheckTimeout = 10 * time.Second

	// readyCheckInterval 就绪检查间隔
	readyCheckInterval = 100 * time.Millisecond
)

// ════════════════════════════════════════════════════════════════════════════
//                              生命周期管理
// ════════════════════════════════════════════════════════════════════════════

// Start 启动节点
//
// 采用阶段化启动策略：
//  1. Initialize: 启动 Fx App，初始化所有组件
//  2. Ready Check: 等待关键组件就绪（如监听地址）
//  3. Running: 进入运行状态，接受用户请求
//
// 启动所有内部组件：
//   - Host（网络监听）
//   - Discovery（节点发现）
//   - RealmManager（Realm 管理）
//   - 协议层服务
//
// 可以多次调用 Start/Stop，但必须先调用 Start 才能使用节点。
func (n *Node) Start(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return ErrNodeClosed
	}

	if n.started {
		return ErrAlreadyStarted
	}

	// ════════════════════════════════════════════════════════════════════════
	// Phase 1: Initialize - 启动 Fx App
	// ════════════════════════════════════════════════════════════════════════
	n.state = StateInitializing
	logger.Info("正在初始化节点")

	// 使用超时上下文
	initCtx, initCancel := context.WithTimeout(ctx, initializeTimeout)
	defer initCancel()

	// 启动 Fx 应用（调用所有模块的 OnStart）
	// 此时 lifecycle.Module 的 OnStart 会推进到 A1
	if err := n.app.Start(initCtx); err != nil {
		n.state = StateIdle
		logger.Error("节点初始化失败", "error", err)
		return fmt.Errorf("initialize failed: %w", err)
	}
	logger.Debug("Fx 应用启动成功")

	// 生命周期阶段 A1 (IdentityInit) 已在 lifecycle.Module OnStart 中完成
	// 此时 Identity 已加载，NodeID 已可用

	// ════════════════════════════════════════════════════════════════════════
	// Phase 1.5: Listen - 启动监听地址
	// ════════════════════════════════════════════════════════════════════════
	if n.host != nil && len(n.config.listenAddrs) > 0 {
		logger.Debug("开始监听地址", "count", len(n.config.listenAddrs))
		if err := n.host.Listen(n.config.listenAddrs...); err != nil {
			n.state = StateStopping
			logger.Error("监听地址失败", "error", err)
			stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer stopCancel()
			_ = n.app.Stop(stopCtx)
			n.state = StateStopped
			return fmt.Errorf("listen failed: %w", err)
		}
		logger.Info("监听地址成功", "addrs", n.host.Addrs())
	}

	// 生命周期阶段 A2: 传输层启动完成
	if n.lifecycleCoordinator != nil {
		_ = n.lifecycleCoordinator.AdvanceTo(lifecycle.PhaseA2TransportStart)
	}

	// ReadyLevel: Network - 传输层就绪
	n.setReadyLevel(pkgif.ReadyLevelNetwork)

	// ════════════════════════════════════════════════════════════════════════
	// Phase 2: Ready Check - 等待关键组件就绪
	// ════════════════════════════════════════════════════════════════════════
	n.state = StateStarting
	logger.Debug("等待关键组件就绪")

	if err := n.waitForReady(ctx); err != nil {
		// 启动失败，停止 Fx App
		n.state = StateStopping
		logger.Error("就绪检查失败", "error", err)
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer stopCancel()
		_ = n.app.Stop(stopCtx) // 忽略停止错误
		n.state = StateStopped
		return fmt.Errorf("ready check failed: %w", err)
	}

	// 生命周期阶段 A3: 地址发现（事件驱动）
	// 时序对齐：等待 NAT 类型检测完成再推进阶段
	if n.lifecycleCoordinator != nil {
		natCtx, natCancel := context.WithTimeout(ctx, 30*time.Second)
		if err := n.lifecycleCoordinator.WaitNATTypeReady(natCtx); err != nil {
			logger.Warn("等待 NAT 类型检测超时，使用 Unknown 继续", "error", err)
		}
		natCancel()
		_ = n.lifecycleCoordinator.AdvanceTo(lifecycle.PhaseA3AddressDiscovery)
	}

	// 生命周期阶段 A4: 网络接入
	// waitForReady 完成意味着 DHT Bootstrap 已执行
	if n.lifecycleCoordinator != nil {
		_ = n.lifecycleCoordinator.AdvanceTo(lifecycle.PhaseA4NetworkJoin)
	}

	// ReadyLevel: Discovered - DHT 入网成功（waitForReady 完成表示 DHT Bootstrap 已执行）
	n.setReadyLevel(pkgif.ReadyLevelDiscovered)

	// ════════════════════════════════════════════════════════════════════════
	// Phase 3: 自动启用基础设施服务
	// ════════════════════════════════════════════════════════════════════════
	//
	// 根据配置自动启用 Bootstrap 和 Relay 服务能力。
	// 这些服务需要在节点完全启动后才能启用，因为它们依赖于：
	//   - Host 已完成初始化并监听地址
	//   - Reachability Coordinator 已设置好对外通告地址
	//
	// 注意：这里的启用是"软失败"的，错误只会记录日志而不会导致节点启动失败。
	// 这是因为基础设施能力是可选的，不应该影响节点的基本功能。
	n.enableInfrastructureServices(ctx)

	// ════════════════════════════════════════════════════════════════════════
	// Phase 4: Running - 进入运行状态
	// ════════════════════════════════════════════════════════════════════════
	n.state = StateRunning
	n.started = true
	logger.Info("节点启动成功", "nodeID", n.host.ID()[:8])

	// 生命周期阶段 A5: 地址发布（事件驱动）
	// 时序对齐：等待地址就绪（包括 NAT 类型检测 + Relay 连接）再推进阶段
	// A5 的实际发布由 fx.go wireAddressDiscovery 中的 SetAddressReady 触发
	if n.lifecycleCoordinator != nil {
		addrCtx, addrCancel := context.WithTimeout(ctx, 60*time.Second)
		if err := n.lifecycleCoordinator.WaitAddressReady(addrCtx); err != nil {
			logger.Warn("等待地址就绪超时，继续启动", "error", err)
		}
		addrCancel()
		_ = n.lifecycleCoordinator.AdvanceTo(lifecycle.PhaseA5AddressPublish)
		_ = n.lifecycleCoordinator.AdvanceTo(lifecycle.PhaseA6Ready)
	}

	// ReadyLevel: Reachable - 可达性验证完成
	// 注意：此时 PeerRecord 应该已经发布到 DHT（由 dht/module.go 的 Bootstrap 完成后触发）
	n.setReadyLevel(pkgif.ReadyLevelReachable)

	// ════════════════════════════════════════════════════════════════════════
	// Phase 5: 订阅网络变化事件
	// ════════════════════════════════════════════════════════════════════════
	if n.networkMonitor != nil {
		go n.subscribeNetworkChanges(context.Background())
	}

	// ════════════════════════════════════════════════════════════════════════
	// Phase 6: 注册连接事件通知
	// ════════════════════════════════════════════════════════════════════════
	n.registerSwarmNotifier()

	return nil
}

// Stop 停止节点
//
// 停止所有内部组件，但保留状态。
// 调用 Stop 后可以再次调用 Start 重启节点。
//
// 停止顺序（反向启动顺序）：
//  1. Protocol Layer
//  2. Realm Layer
//  3. Discovery Layer
//  4. Core Layer
func (n *Node) Stop(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return ErrNodeClosed
	}

	if !n.started {
		return ErrNotStarted
	}

	// 状态转换
	n.state = StateStopping
	logger.Info("正在停止节点")

	// 停止 Fx 应用（自动按反向顺序调用 OnStop）
	if err := n.app.Stop(ctx); err != nil {
		// 即使停止出错，也标记为已停止
		n.state = StateStopped
		n.started = false
		logger.Error("停止节点失败", "error", err)
		return fmt.Errorf("stop fx app: %w", err)
	}

	n.state = StateStopped
	n.started = false
	logger.Info("节点已停止")
	return nil
}

// Close 关闭节点并释放所有资源
//
// 对齐设计文档 Phase D: 优雅关闭
// 执行流程：
//  1. D1: 通知离开 - 离开所有 Realm，广播离开消息
//  2. D2: 排空连接 - 等待进行中的请求完成
//  3. D3: 取消发布 DHT - 从 DHT 移除 PeerRecord
//  4. D4: 释放资源 - 停止 Fx 应用，释放所有资源
//
// 与 Stop 的区别：
//   - Stop: 可以重新 Start
//   - Close: 完全关闭，不可重新启动
func (n *Node) Close() error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.closed {
		return nil // 已经关闭
	}

	logger.Info("正在关闭节点（Phase D 优雅关闭）")

	// ════════════════════════════════════════════════════════════════════════
	// Phase D Step D1: 通知离开 - 离开所有 Realm
	// ════════════════════════════════════════════════════════════════════════
	if n.lifecycleCoordinator != nil {
		_ = n.lifecycleCoordinator.AdvanceTo(lifecycle.PhaseD1NotifyLeave)
	}

	if n.currentRealm != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := n.currentRealm.internal.Leave(ctx); err != nil {
			logger.Warn("离开 Realm 失败", "error", err)
		}
		cancel()
		n.currentRealm = nil
	}

	// ════════════════════════════════════════════════════════════════════════
	// Phase D Step D2: 排空连接 - 等待进行中的请求完成
	// 注：由 Fx OnStop 钩子自动处理连接关闭
	// ════════════════════════════════════════════════════════════════════════
	if n.lifecycleCoordinator != nil {
		_ = n.lifecycleCoordinator.AdvanceTo(lifecycle.PhaseD2DrainConnections)
	}

	// ════════════════════════════════════════════════════════════════════════
	// Phase D Step D3: 取消发布 DHT PeerRecord
	// ════════════════════════════════════════════════════════════════════════
	if n.lifecycleCoordinator != nil {
		_ = n.lifecycleCoordinator.AdvanceTo(lifecycle.PhaseD3UnpublishDHT)
	}

	if n.discovery != nil {
		// 尝试类型断言为 pkgif.DHT 以调用 UnpublishPeerRecord
		if dht, ok := n.discovery.(pkgif.DHT); ok {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := dht.UnpublishPeerRecord(ctx); err != nil {
				logger.Warn("取消发布 DHT PeerRecord 失败", "error", err)
			} else {
				logger.Info("DHT PeerRecord 已取消发布")
			}
			cancel()
		}
	}

	// ════════════════════════════════════════════════════════════════════════
	// Phase D Step D4: 释放资源 - 停止 Fx 应用
	// ════════════════════════════════════════════════════════════════════════
	if n.lifecycleCoordinator != nil {
		_ = n.lifecycleCoordinator.AdvanceTo(lifecycle.PhaseD4Shutdown)
	}

	if n.started {
		n.state = StateStopping
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := n.app.Stop(ctx); err != nil {
			logger.Warn("停止 Fx 应用失败", "error", err)
		}
	}

	n.state = StateStopped
	n.started = false
	n.closed = true

	// 清理连接事件回调
	n.cleanupPeerEventCallbacks()

	logger.Info("节点已关闭（Phase D 完成）")
	return nil
}

// ════════════════════════════════════════════════════════════════════════════
//                              ReadyLevel API
// ════════════════════════════════════════════════════════════════════════════

// ReadyLevel 返回当前就绪级别
//
// 就绪级别表示节点启动过程中的当前阶段。
func (n *Node) ReadyLevel() pkgif.ReadyLevel {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.readyLevel
}

// WaitReady 等待到达指定就绪级别
//
// 阻塞直到节点达到指定的就绪级别或 context 被取消。
func (n *Node) WaitReady(ctx context.Context, level pkgif.ReadyLevel) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	// 如果已经达到目标级别，立即返回
	if n.readyLevel >= level {
		return nil
	}

	// 创建一个 channel 用于检测 context 取消
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		n.mu.Lock()
		n.readyLevelCond.Broadcast() // 唤醒所有等待者
		n.mu.Unlock()
		close(done)
	}()

	// 等待条件满足
	for n.readyLevel < level {
		// 检查 context 是否已取消
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// 等待条件变量
		n.readyLevelCond.Wait()

		// 再次检查 context
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	return nil
}

// OnReadyLevelChange 注册就绪级别变化回调
//
// 当就绪级别发生变化时，会调用注册的回调函数。
func (n *Node) OnReadyLevelChange(callback func(level pkgif.ReadyLevel)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.readyLevelCallbacks = append(n.readyLevelCallbacks, callback)
}

// ════════════════════════════════════════════════════════════════════════════
//                              内部方法
// ════════════════════════════════════════════════════════════════════════════

// setReadyLevel 设置就绪级别并通知回调
//
// 内部方法，用于在启动流程中更新就绪级别。
// 必须在持有 mu 锁或启动流程中调用。
func (n *Node) setReadyLevel(level pkgif.ReadyLevel) {
	if n.readyLevel >= level {
		return // 不允许降级
	}

	oldLevel := n.readyLevel
	n.readyLevel = level

	logger.Info("就绪级别变更",
		"from", oldLevel.String(),
		"to", level.String(),
	)

	// 通知等待者
	if n.readyLevelCond != nil {
		n.readyLevelCond.Broadcast()
	}

	// 调用回调（复制切片以避免持锁时调用）
	callbacks := make([]func(level pkgif.ReadyLevel), len(n.readyLevelCallbacks))
	copy(callbacks, n.readyLevelCallbacks)

	// 在新 goroutine 中调用回调，避免阻塞
	go func() {
		for _, cb := range callbacks {
			cb(level)
		}
	}()
}

// waitForReady 等待关键组件就绪
//
// 检查以下条件：
//   - Host 已初始化
//   - 至少有一个监听地址
func (n *Node) waitForReady(ctx context.Context) error {
	// 检查 Host 是否已注入
	if n.host == nil {
		return fmt.Errorf("host not initialized")
	}

	// 等待至少一个监听地址
	deadline := time.After(readyCheckTimeout)
	ticker := time.NewTicker(readyCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			return fmt.Errorf("timeout waiting for listen addresses (got %d)", len(n.host.Addrs()))
		case <-ticker.C:
			if len(n.host.Addrs()) > 0 {
				// 就绪
				return nil
			}
		}
	}
}

// enableInfrastructureServices 自动启用基础设施服务
//
// 根据配置自动启用 Bootstrap 和 Relay 服务能力。
// 错误只会记录日志，不会导致启动失败。
func (n *Node) enableInfrastructureServices(ctx context.Context) {
	if n.config == nil || n.config.config == nil {
		return
	}
	cfg := n.config.config

	// 自动启用 Bootstrap 服务（如果配置了）
	if cfg.Discovery.Bootstrap.EnableService {
		logger.Debug("正在自动启用 Bootstrap 服务")
		// 注意：这里不能持有 n.mu 锁，因为 Start() 已经持有了
		// EnableBootstrap 方法不需要 n.mu 锁
		if n.bootstrapService != nil {
			if err := n.bootstrapService.Enable(ctx); err != nil {
				logger.Warn("自动启用 Bootstrap 服务失败", "error", err)
			} else {
				logger.Info("Bootstrap 服务已自动启用")
			}
		} else {
			logger.Warn("Bootstrap 服务不可用，无法自动启用",
				"hint", "确保 discovery.enable_bootstrap=true 以加载 Bootstrap 模块")
		}
	}

	// 注意：Relay 服务已经在 Relay Manager 的 Start() 中根据 EnableServer 配置自动启用
	// 这里不需要额外处理，只记录状态
	if cfg.Relay.EnableServer && n.relayManager != nil {
		if n.relayManager.IsRelayEnabled() {
			logger.Debug("Relay 服务已启用（由 Relay Manager 自动处理）")
		}
	}
}
