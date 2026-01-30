package dep2p

import (
	"context"
	"fmt"
	"sync"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ════════════════════════════════════════════════════════════════════════════
//                              连接事件回调
// ════════════════════════════════════════════════════════════════════════════

// peerEventCallbacks 存储连接事件回调
// 这些字段通过 initPeerEventCallbacks 在 Node 构造时初始化
type peerEventCallbacks struct {
	// 连接事件回调
	peerConnectedCallbacks   []func(PeerConnectedEvent)
	peerConnectedCallbacksMu sync.RWMutex

	// 断开事件回调
	peerDisconnectedCallbacks   []func(PeerDisconnectedEvent)
	peerDisconnectedCallbacksMu sync.RWMutex
}

// 全局回调存储（与 Node 实例关联）
// 使用 sync.Map 避免并发问题
var nodeCallbacks sync.Map // map[*Node]*peerEventCallbacks

// getCallbacks 获取或创建 Node 的回调存储
func (n *Node) getCallbacks() *peerEventCallbacks {
	if cb, ok := nodeCallbacks.Load(n); ok {
		return cb.(*peerEventCallbacks)
	}
	cb := &peerEventCallbacks{}
	nodeCallbacks.Store(n, cb)
	return cb
}

// OnPeerConnected 注册节点连接回调
//
// 当有新节点连接时调用回调函数。
// 回调在独立 goroutine 中执行，不阻塞网络层。
//
// 示例：
//
//	node.OnPeerConnected(func(event dep2p.PeerConnectedEvent) {
//	    fmt.Printf("节点连接: %s, 方向: %s\n", event.PeerID[:8], event.Direction)
//	})
func (n *Node) OnPeerConnected(handler func(event PeerConnectedEvent)) {
	cb := n.getCallbacks()
	cb.peerConnectedCallbacksMu.Lock()
	defer cb.peerConnectedCallbacksMu.Unlock()
	cb.peerConnectedCallbacks = append(cb.peerConnectedCallbacks, handler)
}

// OnPeerDisconnected 注册节点断开回调
//
// 当节点断开连接时调用回调函数。
// 回调在独立 goroutine 中执行，不阻塞网络层。
//
// 示例：
//
//	node.OnPeerDisconnected(func(event dep2p.PeerDisconnectedEvent) {
//	    fmt.Printf("节点断开: %s, 原因: %s\n", event.PeerID[:8], event.Reason)
//	})
func (n *Node) OnPeerDisconnected(handler func(event PeerDisconnectedEvent)) {
	cb := n.getCallbacks()
	cb.peerDisconnectedCallbacksMu.Lock()
	defer cb.peerDisconnectedCallbacksMu.Unlock()
	cb.peerDisconnectedCallbacks = append(cb.peerDisconnectedCallbacks, handler)
}

// ════════════════════════════════════════════════════════════════════════════
//                              SwarmNotifier 实现
// ════════════════════════════════════════════════════════════════════════════

// nodeSwarmNotifier 实现 pkgif.SwarmNotifier 接口
// 将 Swarm 层的连接事件转发给用户注册的回调
type nodeSwarmNotifier struct {
	node *Node
}

// Connected 当建立新连接时调用
func (n *nodeSwarmNotifier) Connected(conn pkgif.Connection) {
	if n.node == nil || n.node.host == nil {
		return
	}

	// 获取远端节点 ID
	peerID := string(conn.RemotePeer())
	if peerID == "" {
		return
	}

	// 构建用户友好事件
	event := PeerConnectedEvent{
		PeerID:    peerID,
		Direction: directionToString(conn.Stat().Direction),
		Timestamp: time.Now(),
	}

	// 获取远端地址
	if remoteAddr := conn.RemoteMultiaddr(); remoteAddr != nil {
		event.Addrs = []string{remoteAddr.String()}
	}

	// 获取连接数
	if swarm := n.node.host.Network(); swarm != nil {
		conns := swarm.ConnsToPeer(peerID)
		event.NumConns = len(conns)
	}

	// 从 Peerstore 获取更多地址
	if ps := n.node.host.Peerstore(); ps != nil {
		addrs := ps.Addrs(conn.RemotePeer())
		for _, addr := range addrs {
			addrStr := addr.String()
			// 避免重复
			found := false
			for _, existing := range event.Addrs {
				if existing == addrStr {
					found = true
					break
				}
			}
			if !found {
				event.Addrs = append(event.Addrs, addrStr)
			}
		}
	}

	// 调用回调（异步，不阻塞网络层）
	cb := n.node.getCallbacks()
	cb.peerConnectedCallbacksMu.RLock()
	callbacks := make([]func(PeerConnectedEvent), len(cb.peerConnectedCallbacks))
	copy(callbacks, cb.peerConnectedCallbacks)
	cb.peerConnectedCallbacksMu.RUnlock()

	for _, handler := range callbacks {
		go handler(event)
	}

	// 日志记录
	if len(callbacks) > 0 {
		logger.Debug("节点连接事件已触发",
			"peer", peerID[:8],
			"direction", event.Direction,
			"numConns", event.NumConns,
			"callbacks", len(callbacks))
	}
}

// Disconnected 当连接断开时调用
func (n *nodeSwarmNotifier) Disconnected(conn pkgif.Connection) {
	if n.node == nil || n.node.host == nil {
		return
	}

	// 获取远端节点 ID
	peerID := string(conn.RemotePeer())
	if peerID == "" {
		return
	}

	// 构建用户友好事件
	event := PeerDisconnectedEvent{
		PeerID:    peerID,
		Reason:    "unknown", // 默认原因
		Timestamp: time.Now(),
	}

	// 获取剩余连接数
	if swarm := n.node.host.Network(); swarm != nil {
		conns := swarm.ConnsToPeer(peerID)
		event.NumConns = len(conns)
	}

	// 调用回调（异步，不阻塞网络层）
	cb := n.node.getCallbacks()
	cb.peerDisconnectedCallbacksMu.RLock()
	callbacks := make([]func(PeerDisconnectedEvent), len(cb.peerDisconnectedCallbacks))
	copy(callbacks, cb.peerDisconnectedCallbacks)
	cb.peerDisconnectedCallbacksMu.RUnlock()

	for _, handler := range callbacks {
		go handler(event)
	}

	// 日志记录
	if len(callbacks) > 0 {
		logger.Debug("节点断开事件已触发",
			"peer", peerID[:8],
			"reason", event.Reason,
			"numConns", event.NumConns,
			"callbacks", len(callbacks))
	}
}

// registerSwarmNotifier 注册 Swarm 事件通知
//
// 在 Node.Start() 中调用，将 Swarm 事件转发给用户回调。
func (n *Node) registerSwarmNotifier() {
	if n.host == nil {
		return
	}

	swarm := n.host.Network()
	if swarm == nil {
		return
	}

	// 注册通知器
	notifier := &nodeSwarmNotifier{node: n}
	swarm.Notify(notifier)

	logger.Debug("Swarm 事件通知器已注册")
}

// cleanupPeerEventCallbacks 清理节点的回调存储
//
// 在 Node.Close() 中调用，避免内存泄漏。
func (n *Node) cleanupPeerEventCallbacks() {
	nodeCallbacks.Delete(n)
}

// ════════════════════════════════════════════════════════════════════════════
//                              带宽统计
// ════════════════════════════════════════════════════════════════════════════

// BandwidthStats 返回总体带宽统计
//
// 返回节点的总体带宽使用情况。
// 如果带宽统计未启用（配置 bandwidth.enabled=false），返回空值。
//
// 示例：
//
//	stats := node.BandwidthStats()
//	fmt.Printf("总入站: %d bytes, 入站速率: %.2f bytes/sec\n",
//	    stats.TotalIn, stats.RateIn)
func (n *Node) BandwidthStats() BandwidthSnapshot {
	counter := n.getBandwidthCounter()
	if counter == nil {
		return BandwidthSnapshot{}
	}

	internal := counter.GetTotals()
	return BandwidthSnapshot{
		TotalIn:  internal.TotalIn,
		TotalOut: internal.TotalOut,
		RateIn:   internal.RateIn,
		RateOut:  internal.RateOut,
	}
}

// BandwidthForPeer 返回指定节点的带宽统计
//
// 返回与指定节点之间的带宽使用情况。
// 如果带宽统计未启用或节点未连接，返回空值。
//
// 示例：
//
//	stats := node.BandwidthForPeer("12D3KooW...")
//	fmt.Printf("节点 %s: 入站 %d, 出站 %d\n",
//	    peerID[:8], stats.TotalIn, stats.TotalOut)
func (n *Node) BandwidthForPeer(peerID string) BandwidthSnapshot {
	counter := n.getBandwidthCounter()
	if counter == nil {
		return BandwidthSnapshot{}
	}

	internal := counter.GetForPeer(peerID)
	return BandwidthSnapshot{
		TotalIn:  internal.TotalIn,
		TotalOut: internal.TotalOut,
		RateIn:   internal.RateIn,
		RateOut:  internal.RateOut,
	}
}

// BandwidthForProtocol 返回指定协议的带宽统计
//
// 返回指定协议的带宽使用情况。
// 如果带宽统计未启用，返回空值。
//
// 示例：
//
//	stats := node.BandwidthForProtocol("/dep2p/messaging/1.0.0")
//	fmt.Printf("协议带宽: 入站 %d, 出站 %d\n",
//	    stats.TotalIn, stats.TotalOut)
func (n *Node) BandwidthForProtocol(protocol string) BandwidthSnapshot {
	counter := n.getBandwidthCounter()
	if counter == nil {
		return BandwidthSnapshot{}
	}

	internal := counter.GetForProtocol(protocol)
	return BandwidthSnapshot{
		TotalIn:  internal.TotalIn,
		TotalOut: internal.TotalOut,
		RateIn:   internal.RateIn,
		RateOut:  internal.RateOut,
	}
}

// BandwidthByPeer 返回所有节点的带宽统计
//
// 返回与所有已连接节点的带宽使用情况映射。
// 如果带宽统计未启用，返回 nil。
//
// 示例：
//
//	statsByPeer := node.BandwidthByPeer()
//	for peerID, stats := range statsByPeer {
//	    fmt.Printf("%s: in=%d, out=%d\n",
//	        peerID[:8], stats.TotalIn, stats.TotalOut)
//	}
func (n *Node) BandwidthByPeer() map[string]BandwidthSnapshot {
	counter := n.getBandwidthCounter()
	if counter == nil {
		return nil
	}

	internal := counter.GetByPeer()
	if len(internal) == 0 {
		return nil
	}

	result := make(map[string]BandwidthSnapshot, len(internal))
	for peer, stats := range internal {
		result[peer] = BandwidthSnapshot{
			TotalIn:  stats.TotalIn,
			TotalOut: stats.TotalOut,
			RateIn:   stats.RateIn,
			RateOut:  stats.RateOut,
		}
	}
	return result
}

// BandwidthByProtocol 返回所有协议的带宽统计
//
// 返回所有协议的带宽使用情况映射。
// 如果带宽统计未启用，返回 nil。
//
// 示例：
//
//	statsByProto := node.BandwidthByProtocol()
//	for proto, stats := range statsByProto {
//	    fmt.Printf("%s: in=%d, out=%d\n",
//	        proto, stats.TotalIn, stats.TotalOut)
//	}
func (n *Node) BandwidthByProtocol() map[string]BandwidthSnapshot {
	counter := n.getBandwidthCounter()
	if counter == nil {
		return nil
	}

	internal := counter.GetByProtocol()
	if len(internal) == 0 {
		return nil
	}

	result := make(map[string]BandwidthSnapshot, len(internal))
	for proto, stats := range internal {
		result[proto] = BandwidthSnapshot{
			TotalIn:  stats.TotalIn,
			TotalOut: stats.TotalOut,
			RateIn:   stats.RateIn,
			RateOut:  stats.RateOut,
		}
	}
	return result
}

// IsBandwidthStatsEnabled 检查带宽统计是否已启用
//
// 返回 true 表示带宽统计已启用，可以使用 BandwidthStats 等方法。
func (n *Node) IsBandwidthStatsEnabled() bool {
	return n.getBandwidthCounter() != nil
}

// getBandwidthCounter 获取带宽计数器
//
// 通过 Swarm 类型断言获取内部的 BandwidthCounter。
// 如果带宽统计未启用或 Swarm 不支持，返回 nil。
func (n *Node) getBandwidthCounter() pkgif.BandwidthCounter {
	if n.host == nil {
		return nil
	}

	// 获取 Swarm
	swarm := n.host.Network()
	if swarm == nil {
		return nil
	}

	// 类型断言：检查 Swarm 是否实现了 BandwidthCounterProvider 接口
	type bandwidthProvider interface {
		BandwidthCounter() pkgif.BandwidthCounter
	}

	if provider, ok := swarm.(bandwidthProvider); ok {
		return provider.BandwidthCounter()
	}

	return nil
}

// ════════════════════════════════════════════════════════════════════════════
//                              网络变化处理
// ════════════════════════════════════════════════════════════════════════════

// NetworkChange 通知节点网络可能已变化
//
// 在某些平台（如 Android）上，系统无法自动检测网络变化，
// 应用需要在收到系统网络变化回调时调用此方法。
//
// 即使网络实际未变化，调用此方法也不会有副作用。
//
// 使用示例：
//
//	// Android 中接收系统网络变化通知
//	func onNetworkChanged() {
//	    node.NetworkChange()
//	}
func (n *Node) NetworkChange() {
	if n.networkMonitor == nil {
		return
	}

	n.networkMonitor.NotifyChange()
}

// OnNetworkChange 注册网络变化回调
//
// 当检测到网络变化时，会调用注册的回调函数。
// 可以用于应用层做相应处理，如重新获取配置等。
//
// 使用示例：
//
//	node.OnNetworkChange(func(event pkgif.NetworkChangeEvent) {
//	    log.Printf("网络变化: %s", event.Type)
//	    // 重新获取配置、重连重要节点等
//	})
func (n *Node) OnNetworkChange(callback func(event pkgif.NetworkChangeEvent)) {
	n.networkCallbacksMu.Lock()
	defer n.networkCallbacksMu.Unlock()

	n.networkChangeCallbacks = append(n.networkChangeCallbacks, callback)
}

// notifyNetworkChange 通知所有网络变化回调
func (n *Node) notifyNetworkChange(event pkgif.NetworkChangeEvent) {
	n.networkCallbacksMu.RLock()
	callbacks := make([]func(event pkgif.NetworkChangeEvent), len(n.networkChangeCallbacks))
	copy(callbacks, n.networkChangeCallbacks)
	n.networkCallbacksMu.RUnlock()

	for _, callback := range callbacks {
		go callback(event)
	}

	// 网络变化时触发地址变化协调
	go n.triggerAddressChangeCoordination(event)
}

// subscribeNetworkChanges 订阅网络变化并转发给用户回调
func (n *Node) subscribeNetworkChanges(ctx context.Context) {
	events := n.networkMonitor.Subscribe()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			n.notifyNetworkChange(event)
		}
	}
}

// triggerAddressChangeCoordination 网络变化时原子性协调地址更新
//
// 对齐设计文档 Section 7.3 AddressChangeCoordinator:
// 网络变化时原子性触发：
//  1. 更新 Peerstore（本地缓存）
//  2. 更新 DHT (seq+1)
//  3. 更新所有已加入的 Realm 的 DHT 记录 (seq+1)
//  4. 通知 Relay 地址簿
//  5. 广播给 MemberList（通过 Realm）
func (n *Node) triggerAddressChangeCoordination(event pkgif.NetworkChangeEvent) {
	// 只处理重大网络变化（如网络接口变化，4G→WiFi）
	if event.Type != pkgif.NetworkChangeMajor {
		return
	}

	logger.Info("AddressChangeCoordinator: 检测到重大网络变化",
		"type", event.Type,
		"oldAddrs", len(event.OldAddrs),
		"newAddrs", len(event.NewAddrs))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 使用 sync.WaitGroup 确保所有更新尽可能同时进行
	var wg sync.WaitGroup
	var updateErrors []string
	var errorsMu sync.Mutex

	recordError := func(component string, err error) {
		if err != nil {
			errorsMu.Lock()
			updateErrors = append(updateErrors, fmt.Sprintf("%s: %v", component, err))
			errorsMu.Unlock()
		}
	}

	// Step 1: 更新 Peerstore（本地地址缓存）
	if n.host != nil {
		logger.Debug("AddressChangeCoordinator: Step 1 - Peerstore 地址已由 Host 自动更新")
	}

	// Step 2: 全局 DHT 更新
	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Debug("AddressChangeCoordinator: Step 2 - DHT 更新由组件自动处理")
	}()

	// Step 3: 更新所有已加入的 Realm 的 DHT + Relay 地址簿 + MemberList
	wg.Add(1)
	go func() {
		defer wg.Done()
		if n.realmManager != nil {
			if err := n.realmManager.NotifyNetworkChange(ctx, event); err != nil {
				recordError("Realm", err)
				logger.Debug("AddressChangeCoordinator: Step 3 - Realm 更新失败", "err", err)
			} else {
				logger.Debug("AddressChangeCoordinator: Step 3 - Realm 更新成功")
			}
		} else {
			logger.Debug("AddressChangeCoordinator: Step 3 - RealmManager 不可用，跳过")
		}
	}()

	// 等待所有更新完成
	wg.Wait()

	if len(updateErrors) > 0 {
		logger.Warn("AddressChangeCoordinator: 部分更新失败",
			"errors", updateErrors)
	} else {
		logger.Info("AddressChangeCoordinator: 地址变化协调完成")
	}
}

// ════════════════════════════════════════════════════════════════════════════
//                              健康检查
// ════════════════════════════════════════════════════════════════════════════

// Health 获取节点健康状态
//
// 返回节点各组件的健康状态映射。
// 可用于监控、诊断和运维。
//
// 检查项：
//   - node: 节点整体状态
//   - host: 网络主机状态
//   - connections: 连接状态
//   - discovery: 发现服务状态（如启用）
//   - realm: Realm 状态（如已加入）
func (n *Node) Health(ctx context.Context) map[string]pkgif.HealthStatus {
	n.mu.RLock()
	defer n.mu.RUnlock()

	results := make(map[string]pkgif.HealthStatus)

	// 1. 节点整体状态
	results["node"] = n.nodeHealthStatus()

	// 2. Host 状态
	results["host"] = n.hostHealthStatus()

	// 3. 连接状态
	results["connections"] = n.connectionsHealthStatus()

	// 4. Discovery 状态
	if n.discovery != nil {
		results["discovery"] = n.discoveryHealthStatus(ctx)
	}

	// 5. Realm 状态
	if n.currentRealm != nil {
		results["realm"] = n.realmHealthStatus()
	}

	return results
}

// HealthSummary 返回健康检查摘要
//
// 返回整体健康状态和不健康组件列表。
func (n *Node) HealthSummary(ctx context.Context) (overall pkgif.HealthState, unhealthy []string) {
	health := n.Health(ctx)
	overall = pkgif.HealthStateHealthy

	for name, status := range health {
		if status.Status == pkgif.HealthStateUnhealthy {
			overall = pkgif.HealthStateUnhealthy
			unhealthy = append(unhealthy, name)
		} else if status.Status == pkgif.HealthStateDegraded && overall != pkgif.HealthStateUnhealthy {
			overall = pkgif.HealthStateDegraded
		}
	}

	return
}

// nodeHealthStatus 返回节点整体健康状态
func (n *Node) nodeHealthStatus() pkgif.HealthStatus {
	switch n.state {
	case StateRunning:
		return pkgif.NewHealthStatusWithDetails(
			pkgif.HealthStateHealthy,
			"node is running",
			map[string]interface{}{"state": n.state.String()},
		)
	case StateInitializing, StateStarting:
		return pkgif.NewHealthStatusWithDetails(
			pkgif.HealthStateDegraded,
			"node is starting up",
			map[string]interface{}{"state": n.state.String()},
		)
	case StateStopping:
		return pkgif.NewHealthStatusWithDetails(
			pkgif.HealthStateDegraded,
			"node is shutting down",
			map[string]interface{}{"state": n.state.String()},
		)
	default:
		return pkgif.NewHealthStatusWithDetails(
			pkgif.HealthStateUnhealthy,
			"node is not running",
			map[string]interface{}{"state": n.state.String()},
		)
	}
}

// hostHealthStatus 返回 Host 健康状态
func (n *Node) hostHealthStatus() pkgif.HealthStatus {
	if n.host == nil {
		return pkgif.UnhealthyStatus("host not initialized")
	}

	addrs := n.host.Addrs()
	if len(addrs) == 0 {
		return pkgif.UnhealthyStatus("no listen addresses")
	}

	return pkgif.NewHealthStatusWithDetails(
		pkgif.HealthStateHealthy,
		fmt.Sprintf("listening on %d addresses", len(addrs)),
		map[string]interface{}{
			"id":        n.host.ID(),
			"addresses": addrs,
		},
	)
}

// connectionsHealthStatus 返回连接健康状态
func (n *Node) connectionsHealthStatus() pkgif.HealthStatus {
	connCount := 0
	if n.host != nil {
		if hostImpl, ok := n.host.(interface{ Network() pkgif.Swarm }); ok {
			if swarm := hostImpl.Network(); swarm != nil {
				connCount = len(swarm.Conns())
			}
		}
	}

	// 连接数为 0 不一定是不健康的（可能刚启动）
	state := pkgif.HealthStateHealthy
	message := fmt.Sprintf("%d active connections", connCount)

	return pkgif.NewHealthStatusWithDetails(
		state,
		message,
		map[string]interface{}{"count": connCount},
	)
}

// discoveryHealthStatus 返回 Discovery 健康状态
func (n *Node) discoveryHealthStatus(ctx context.Context) pkgif.HealthStatus {
	// 如果 Discovery 实现了 HealthChecker，使用其实现
	if hc, ok := n.discovery.(pkgif.HealthChecker); ok {
		return hc.Check(ctx)
	}

	// 默认：有 discovery 实例就认为是健康的
	return pkgif.HealthyStatus("discovery service available")
}

// realmHealthStatus 返回 Realm 健康状态
func (n *Node) realmHealthStatus() pkgif.HealthStatus {
	if n.currentRealm == nil {
		return pkgif.UnhealthyStatus("not in any realm")
	}

	return pkgif.NewHealthStatusWithDetails(
		pkgif.HealthStateHealthy,
		fmt.Sprintf("joined realm: %s", n.currentRealm.ID()),
		map[string]interface{}{
			"realm_id": n.currentRealm.ID(),
		},
	)
}
