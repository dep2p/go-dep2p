package dep2p

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/internal/core/lifecycle"     // 生命周期协调器
	"github.com/dep2p/go-dep2p/internal/core/nat/netreport" // 合并到 nat 子目录
	"github.com/dep2p/go-dep2p/internal/debug/introspect"   // 移至 debug 层
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/types"
)

var logger = log.Logger("dep2p")

// ════════════════════════════════════════════════════════════════════════════
//                              节点状态
// ════════════════════════════════════════════════════════════════════════════

// NodeState 节点状态
//
// 表示节点在生命周期中的当前阶段。
type NodeState int

const (
	// StateIdle 空闲状态（已创建，未启动）
	StateIdle NodeState = iota

	// StateInitializing 初始化中（Fx App 启动中）
	StateInitializing

	// StateStarting 启动中（等待组件就绪）
	StateStarting

	// StateRunning 运行中（正常工作状态）
	StateRunning

	// StateStopping 停止中（正在关闭组件）
	StateStopping

	// StateStopped 已停止（可重新启动）
	StateStopped
)

// String 返回状态的字符串表示
func (s NodeState) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateInitializing:
		return "initializing"
	case StateStarting:
		return "starting"
	case StateRunning:
		return "running"
	case StateStopping:
		return "stopping"
	case StateStopped:
		return "stopped"
	default:
		return "unknown"
	}
}

// 启动超时配置
const (
	// initializeTimeout 初始化超时（Fx App Start）
	initializeTimeout = 30 * time.Second

	// readyCheckTimeout 就绪检查超时
	readyCheckTimeout = 10 * time.Second

	// readyCheckInterval 就绪检查间隔
	readyCheckInterval = 100 * time.Millisecond
)

// Node DeP2P 节点
//
// Node 是用户与 DeP2P 网络交互的主入口。
// 它是一个门面（Facade），聚合了所有内部组件。
//
// 架构层次：
//   - API Layer: Node (本层，用户直接交互)
//   - Protocol Layer: Messaging, PubSub, Streams, Liveness
//   - Realm Layer: RealmManager, Realm
//   - Core Layer: Host, Transport, Security, NAT, Relay
//   - Discovery Layer: DHT, Bootstrap, mDNS
//
// 使用示例：
//
//	// 创建并启动节点
//	node, err := dep2p.New(ctx,
//	    dep2p.WithPreset(dep2p.PresetDesktop),
//	    dep2p.WithListenPort(4001),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer node.Close()
//
//	// 启动节点
//	if err := node.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//
//	// 加入 Realm
//	realm, err := node.JoinRealm(ctx, realmKey)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 使用协议层服务（多协议支持）
//
//	// Messaging - 请求-响应模式
//	messaging := realm.Messaging()
//	messaging.RegisterHandler("chat", chatHandler)
//	resp, _ := messaging.Send(ctx, peerID, "chat", []byte("hello"))
//
//	// PubSub - 发布订阅模式
//	pubsub := realm.PubSub()
//	topic, _ := pubsub.Join("room/general")
//	topic.Publish(ctx, []byte("hello everyone"))
//
//	// Streams - 双向流模式
//	streams := realm.Streams()
//	stream, _ := streams.Open(ctx, peerID, "file-transfer")
//	stream.Write(fileData)
type Node struct {
	// ────────────────────────────────────────────────────────────────────────
	// 配置和状态
	// ────────────────────────────────────────────────────────────────────────

	// config 节点配置
	config *nodeConfig

	// app Fx 应用
	app *fx.App

	// ────────────────────────────────────────────────────────────────────────
	// 核心组件（由 Fx 注入）
	// ────────────────────────────────────────────────────────────────────────

	// host 网络主机
	host pkgif.Host

	// realmManager Realm 管理器
	realmManager pkgif.RealmManager

	// discovery 节点发现服务
	discovery pkgif.Discovery

	// natService NAT 穿透服务（用于获取外部地址）
	natService pkgif.NATService

	// networkMonitor 网络监控器
	networkMonitor pkgif.NetworkMonitor

	// bootstrapService Bootstrap 能力服务（用于 EnableBootstrap）
	bootstrapService pkgif.BootstrapService

	// relayManager Relay 管理器（用于 EnableRelay）
	relayManager pkgif.RelayManager

	// netReportClient 网络诊断客户端（P2 修复）
	netReportClient *netreport.Client

	// introspectServer 自省服务
	introspectServer *introspect.Server

	// lifecycleCoordinator 生命周期协调器
	// 对齐 20260125-node-lifecycle-cross-cutting.md 时序图
	lifecycleCoordinator *lifecycle.Coordinator

	// ────────────────────────────────────────────────────────────────────────
	// Realm 状态
	// ────────────────────────────────────────────────────────────────────────

	// currentRealm 当前加入的 Realm（用户级类型）
	currentRealm *Realm

	// ────────────────────────────────────────────────────────────────────────
	// 网络变化回调
	// ────────────────────────────────────────────────────────────────────────

	networkChangeCallbacks []func(event pkgif.NetworkChangeEvent)
	networkCallbacksMu     sync.RWMutex

	// ────────────────────────────────────────────────────────────────────────
	// 生命周期状态
	// ────────────────────────────────────────────────────────────────────────

	mu      sync.RWMutex
	state   NodeState // 节点状态
	started bool
	closed  bool

	// ────────────────────────────────────────────────────────────────────────
	// ReadyLevel 状态（讨论稿 Section 7.4 对齐）
	// ────────────────────────────────────────────────────────────────────────

	readyLevel          pkgif.ReadyLevel               // 当前就绪级别
	readyLevelCond      *sync.Cond                     // 就绪级别变化条件变量
	readyLevelCallbacks []func(level pkgif.ReadyLevel) // 就绪级别变化回调
}

// ════════════════════════════════════════════════════════════════════════════
//                              构造函数
// ════════════════════════════════════════════════════════════════════════════

// New 创建新节点
//
// 创建节点但不启动，需要调用 Start() 启动。
// 通过 Option 函数配置节点。
//
// 示例：
//
//	node, err := dep2p.New(ctx,
//	    dep2p.WithPreset(dep2p.PresetDesktop),
//	    dep2p.WithListenPort(4001),
//	    dep2p.WithRelay(true),
//	)
func New(ctx context.Context, opts ...Option) (*Node, error) {
	// 创建配置
	cfg := newNodeConfig()

	// 应用选项
	for _, opt := range opts {
		if err := opt(cfg); err != nil {
			return nil, fmt.Errorf("apply option: %w", err)
		}
	}

	// 创建 Node 实例
	node := &Node{
		config:     cfg,
		readyLevel: pkgif.ReadyLevelCreated,
	}
	// 初始化条件变量（用于 WaitReady）
	node.readyLevelCond = sync.NewCond(&node.mu)

	// 构建 Fx 应用
	var err error
	node.app, err = buildFxApp(cfg, node)
	if err != nil {
		return nil, fmt.Errorf("build fx app: %w", err)
	}

	return node, nil
}

// Start 快捷启动函数
//
// 创建节点并立即启动。
// 等价于 New() + Start()。
//
// 示例：
//
//	node, err := dep2p.Start(ctx,
//	    dep2p.WithPreset(dep2p.PresetServer),
//	)
func Start(ctx context.Context, opts ...Option) (*Node, error) {
	node, err := New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	if err := node.Start(ctx); err != nil {
		return nil, fmt.Errorf("start node: %w", err)
	}

	return node, nil
}

// ════════════════════════════════════════════════════════════════════════════
//                              基本信息
// ════════════════════════════════════════════════════════════════════════════

// ID 返回节点 ID
//
// 节点 ID 由节点的公钥派生而来，全局唯一。
func (n *Node) ID() string {
	if n.host == nil {
		return ""
	}
	return n.host.ID()
}

// ListenAddrs 返回监听地址
//
// 返回节点正在监听的所有本地地址。
// 注意：不包含外部地址和中继地址，如需对外公告请使用 AdvertisedAddrs()。
func (n *Node) ListenAddrs() []string {
	if n.host == nil {
		return nil
	}
	return n.host.Addrs()
}

// AdvertisedAddrs 返回对外公告地址
//
// 返回应该对外公告的地址列表，包括：
//  1. 用户配置的公网地址（WithPublicAddr）
//  2. 已验证的公网直连地址（ShareableAddrs）
//  3. 如果配置了 Relay，包含中继电路地址
//
// 中继地址格式：
//
//	{relay-addr}/p2p-circuit/p2p/{local-id}
//	例如：/ip4/relay.example.com/tcp/4001/p2p/QmRelay/p2p-circuit/p2p/QmLocal
//
// 其他节点可以使用中继地址通过 Relay 连接到本节点。
//
// 示例：
//
//	addrs := node.AdvertisedAddrs()
//	for _, addr := range addrs {
//	    fmt.Println("公告地址:", addr)
//	}
func (n *Node) AdvertisedAddrs() []string {
	nodeID := n.ID()
	if nodeID == "" {
		return nil
	}

	var result []string
	seen := make(map[string]struct{})

	// 1. 添加用户配置的公网地址（WithPublicAddr 设置的地址）
	//    这些地址用于云服务器场景：节点监听 0.0.0.0，但对外公告公网 IP
	//    优先级最高，因为是用户明确配置的
	if n.config != nil && len(n.config.advertiseAddrs) > 0 {
		for _, addr := range n.config.advertiseAddrs {
			// 如果地址不包含 /p2p/ 后缀，添加本节点 ID
			if !strings.Contains(addr, "/p2p/") {
				addr = addr + "/p2p/" + nodeID
			}
			if _, ok := seen[addr]; !ok {
				seen[addr] = struct{}{}
				result = append(result, addr)
			}
		}
	}

	// 2. 从 Host/Reachability Coordinator 获取地址
	//    包括：已验证的直连地址 + Relay 地址 + 监听地址
	if n.host != nil {
		// 优先使用 Host.AdvertisedAddrs()，它整合了 Coordinator 的地址
		if hostAddrs := n.host.AdvertisedAddrs(); len(hostAddrs) > 0 {
			for _, addr := range hostAddrs {
				if _, ok := seen[addr]; !ok {
					seen[addr] = struct{}{}
					result = append(result, addr)
				}
			}
		}
	}

	// 3. 从 NAT Service 获取外部地址（兼容旧逻辑）
	if n.natService != nil {
		for _, addr := range n.natService.ExternalAddrs() {
			fullAddr := addr
			if !strings.Contains(addr, "/p2p/") {
				fullAddr = addr + "/p2p/" + nodeID
			}
			if _, ok := seen[fullAddr]; !ok {
				seen[fullAddr] = struct{}{}
				result = append(result, fullAddr)
			}
		}
	}

	// 4. 如果配置了 Relay，添加中继电路地址
	if n.relayManager != nil {
		relayAddr, hasRelay := n.relayManager.RelayAddr()
		if hasRelay && relayAddr != nil {
			// 生成中继电路地址：{relay-addr}/p2p-circuit/p2p/{local-id}
			circuitAddr := buildCircuitAddr(relayAddr.String(), nodeID)
			if circuitAddr != "" {
				if _, ok := seen[circuitAddr]; !ok {
					seen[circuitAddr] = struct{}{}
					result = append(result, circuitAddr)
				}
			}
		}
	}

	if len(result) == 0 {
		return nil
	}

	return result
}

// buildCircuitAddr 构建中继电路地址
//
// 格式：{relay-addr}/p2p-circuit/p2p/{local-id}
//
// 参数：
//   - relayAddr: Relay 节点的完整地址（含 /p2p/{relay-id}）
//   - localID: 本地节点 ID
//
// 返回：
//   - 完整的电路地址，或空字符串（如果构建失败）
func buildCircuitAddr(relayAddr string, localID string) string {
	if relayAddr == "" || localID == "" {
		return ""
	}

	// 验证 relayAddr 包含 /p2p/ 组件
	if !strings.Contains(relayAddr, "/p2p/") {
		return ""
	}

	// 构建电路地址：{relay-addr}/p2p-circuit/p2p/{local-id}
	return relayAddr + "/p2p-circuit/p2p/" + localID
}

// ShareableAddrs 返回可分享的完整地址
//
// 严格语义（继承自 go-dep2p-main）：
//   - 仅返回已验证的公网直连地址（VerifiedDirect）
//   - 返回 Full Address 格式（包含 /p2p/<NodeID>）
//   - 过滤掉私网/回环/link-local 地址
//   - 无验证地址时返回 nil
//
// 用途：
//   - DHT 发布
//   - 分享给其他用户
//   - 作为引导节点地址
//
// 示例：
//
//	addrs := node.ShareableAddrs()
//	for _, addr := range addrs {
//	    fmt.Println("可分享地址:", addr)
//	}
func (n *Node) ShareableAddrs() []string {
	// 获取本地节点 ID
	nodeID := n.ID()
	if nodeID == "" {
		return nil
	}

	var result []string
	seen := make(map[string]struct{})

	// 1. 优先从 Host/Reachability Coordinator 获取已验证的公网地址
	if n.host != nil {
		if shareableAddrs := n.host.ShareableAddrs(); len(shareableAddrs) > 0 {
			for _, addr := range shareableAddrs {
				if _, ok := seen[addr]; !ok {
					seen[addr] = struct{}{}
					result = append(result, addr)
				}
			}
		}
	}

	// 2. 从 NAT Service 获取外部地址（兼容旧逻辑）
	if n.natService != nil {
		for _, addr := range n.natService.ExternalAddrs() {
			// 过滤非公网地址
			if !isPublicAddr(addr) {
				continue
			}

			// 添加 /p2p/<NodeID> 后缀
			fullAddr := addr
			if !strings.Contains(addr, "/p2p/") {
				fullAddr = addr + "/p2p/" + nodeID
			}

			if _, ok := seen[fullAddr]; !ok {
				seen[fullAddr] = struct{}{}
				result = append(result, fullAddr)
			}
		}
	}

	// 3. 用户配置的公网地址也可作为可分享地址
	if n.config != nil && len(n.config.advertiseAddrs) > 0 {
		for _, addr := range n.config.advertiseAddrs {
			// 过滤非公网地址
			if !isPublicAddr(addr) {
				continue
			}

			fullAddr := addr
			if !strings.Contains(addr, "/p2p/") {
				fullAddr = addr + "/p2p/" + nodeID
			}

			if _, ok := seen[fullAddr]; !ok {
				seen[fullAddr] = struct{}{}
				result = append(result, fullAddr)
			}
		}
	}

	if len(result) == 0 {
		return nil
	}

	return result
}

// WaitShareableAddrs 等待可分享地址就绪
//
// 典型用途：创世节点/引导节点启动后等待地址验证完成。
//
// 示例：
//
//	// 启动引导节点
//	node.Start(ctx)
//
//	// 等待地址就绪
//	addrs, err := node.WaitShareableAddrs(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 将地址添加到配置文件
//	saveBootstrapAddrs(addrs)
func (n *Node) WaitShareableAddrs(ctx context.Context) ([]string, error) {
	// 设置超时
	const (
		maxWait       = 30 * time.Second
		checkInterval = 500 * time.Millisecond
	)

	deadline := time.After(maxWait)
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case <-deadline:
			return nil, fmt.Errorf("timeout waiting for shareable addresses")

		case <-ticker.C:
			addrs := n.ShareableAddrs()
			if len(addrs) > 0 {
				return addrs, nil
			}
		}
	}
}

// BootstrapCandidates 返回候选地址（旁路，不入 DHT）
//
// 与 ShareableAddrs 正交：
//   - ShareableAddrs: 严格验证，可入 DHT
//   - BootstrapCandidates: 旁路，包含所有候选（直连+监听+中继）
//
// 用途：
//   - 人工分享/跨设备冷启动
//   - 提供备选地址（即使未验证）
//
// 返回类型：
//   - BootstrapCandidateDirect: 直连地址
//   - BootstrapCandidateRelay: 中继电路地址
func (n *Node) BootstrapCandidates() []types.BootstrapCandidate {
	var candidates []types.BootstrapCandidate

	nodeID := n.ID()
	if nodeID == "" {
		return nil
	}

	// 1. 添加已验证的公网地址（Direct）
	shareableAddrs := n.ShareableAddrs()
	if len(shareableAddrs) > 0 {
		candidates = append(candidates, types.BootstrapCandidate{
			NodeID: nodeID,
			Addrs:  shareableAddrs,
			Type:   types.BootstrapCandidateDirect,
		})
	}

	// 2. 添加所有监听地址（作为候选）
	listenAddrs := n.ListenAddrs()
	if len(listenAddrs) > 0 {
		// 过滤掉已包含在 shareable 中的地址
		var uniqueAddrs []string
		for _, addr := range listenAddrs {
			if !containsAddr(shareableAddrs, addr) {
				uniqueAddrs = append(uniqueAddrs, addr)
			}
		}

		if len(uniqueAddrs) > 0 {
			candidates = append(candidates, types.BootstrapCandidate{
				NodeID: nodeID,
				Addrs:  uniqueAddrs,
				Type:   types.BootstrapCandidateDirect,
			})
		}
	}

	// 3. 添加中继电路地址（Relay）
	if n.relayManager != nil {
		relayAddr, hasRelay := n.relayManager.RelayAddr()
		if hasRelay && relayAddr != nil {
			circuitAddr := buildCircuitAddr(relayAddr.String(), nodeID)
			if circuitAddr != "" {
				candidates = append(candidates, types.BootstrapCandidate{
					NodeID: nodeID,
					Addrs:  []string{circuitAddr},
					Type:   types.BootstrapCandidateRelay,
				})
			}
		}
	}

	return candidates
}

// ConnectionTicket 返回用户友好的连接票据
//
// 格式：dep2p://base64url(...)
// 便于通过聊天/二维码分享。
//
// 票据包含的地址优先级：
//  1. ShareableAddrs（已验证的外部地址，不包含 0.0.0.0）
//  2. AdvertisedAddrs（直连+中继地址）
//
// 注意：如果没有可分享的地址，票据中将只包含 NodeID，
// 连接时需要依赖 DHT 或其他发现机制。
//
// 示例：
//
//	ticket := node.ConnectionTicket()
//	fmt.Println("分享此票据给其他用户:", ticket)
//
//	// 其他节点使用票据连接
//	err := otherNode.Connect(ctx, ticket)
func (n *Node) ConnectionTicket() string {
	nodeID := n.ID()
	if nodeID == "" {
		return ""
	}

	// 获取地址提示（优先使用 ShareableAddrs，已过滤 0.0.0.0 等不可连接地址）
	var addressHints []string
	if shareableAddrs := n.ShareableAddrs(); len(shareableAddrs) > 0 {
		addressHints = shareableAddrs
	} else if advertisedAddrs := n.AdvertisedAddrs(); len(advertisedAddrs) > 0 {
		// 回退到 AdvertisedAddrs（可能包含 Relay 地址）
		// 但需要过滤掉无效地址
		for _, addr := range advertisedAddrs {
			if !strings.Contains(addr, "/0.0.0.0/") &&
				!strings.Contains(addr, "/::/") &&
				!strings.Contains(addr, "/127.0.0.1/") {
				addressHints = append(addressHints, addr)
			}
		}
	}

	// 如果没有有效地址，返回空字符串
	// 票据没有可连接地址是没有意义的
	if len(addressHints) == 0 {
		return ""
	}

	// 创建票据
	ticket := types.NewConnectionTicket(nodeID, addressHints)

	// 编码
	encoded, err := ticket.Encode()
	if err != nil {
		return "" // 编码失败，返回空字符串
	}

	return encoded
}

// containsAddr 检查地址列表是否包含指定地址
func containsAddr(addrs []string, target string) bool {
	for _, addr := range addrs {
		if addr == target {
			return true
		}
	}
	return false
}

// Host 返回底层 Host
//
// 高级用法：直接访问底层 Host 进行低级操作。
// 一般用户不需要使用此方法。
func (n *Node) Host() pkgif.Host {
	return n.host
}

// ConnectionCount 返回当前连接数
//
// 返回节点当前的活跃连接数量。
func (n *Node) ConnectionCount() int {
	if n.host == nil {
		return 0
	}

	// 通过 host.Network() 获取实际连接数
	// 注意：Host 接口可能没有 Network() 方法，
	// 但可以通过内部的 swarm 字段访问
	// 这里使用类型断言访问内部实现
	if hostImpl, ok := n.host.(interface{ Network() pkgif.Swarm }); ok {
		if swarm := hostImpl.Network(); swarm != nil {
			return len(swarm.Conns())
		}
	}

	return 0
}

// State 返回节点当前状态
//
// 可用于监控节点的生命周期阶段。
func (n *Node) State() NodeState {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.state
}

// IsRunning 检查节点是否正在运行
func (n *Node) IsRunning() bool {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.state == StateRunning
}

// ════════════════════════════════════════════════════════════════════════════
//                              ReadyLevel API（讨论稿 Section 7.4 对齐）
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

// ════════════════════════════════════════════════════════════════════════════
//                              Bootstrap 能力开关（ADR-0009）
// ════════════════════════════════════════════════════════════════════════════

// EnableBootstrap 启用 Bootstrap 能力
//
// 将当前节点设置为引导节点，为网络中的新节点提供初始对等方发现服务。
func (n *Node) EnableBootstrap(ctx context.Context) error {
	if n.bootstrapService == nil {
		return fmt.Errorf("bootstrap service not available")
	}
	return n.bootstrapService.Enable(ctx)
}

// DisableBootstrap 禁用 Bootstrap 能力
//
// 停止作为引导节点服务，但保留已存储的节点信息。
func (n *Node) DisableBootstrap(ctx context.Context) error {
	if n.bootstrapService == nil {
		return nil
	}
	return n.bootstrapService.Disable(ctx)
}

// IsBootstrapEnabled 检查 Bootstrap 能力是否已启用
func (n *Node) IsBootstrapEnabled() bool {
	if n.bootstrapService == nil {
		return false
	}
	return n.bootstrapService.IsEnabled()
}

// BootstrapStats 获取 Bootstrap 统计信息
func (n *Node) BootstrapStats() pkgif.BootstrapStats {
	if n.bootstrapService == nil {
		return pkgif.BootstrapStats{}
	}
	return n.bootstrapService.Stats()
}

// ════════════════════════════════════════════════════════════════════════════
//                              Relay 能力开关（v2.0 统一接口）
// ════════════════════════════════════════════════════════════════════════════

// EnableRelay 启用 Relay 能力
//
// 将当前节点设置为中继服务器，为 NAT 后的节点提供中继服务。
func (n *Node) EnableRelay(ctx context.Context) error {
	if n.relayManager == nil {
		return fmt.Errorf("relay manager not available")
	}
	return n.relayManager.EnableRelay(ctx)
}

// DisableRelay 禁用 Relay 能力
func (n *Node) DisableRelay(ctx context.Context) error {
	if n.relayManager == nil {
		return nil
	}
	return n.relayManager.DisableRelay(ctx)
}

// IsRelayEnabled 检查 Relay 能力是否已启用
func (n *Node) IsRelayEnabled() bool {
	if n.relayManager == nil {
		return false
	}
	return n.relayManager.IsRelayEnabled()
}

// SetRelayAddr 设置要使用的 Relay 地址（客户端使用）
func (n *Node) SetRelayAddr(addr types.Multiaddr) error {
	if n.relayManager == nil {
		return fmt.Errorf("relay manager not available")
	}
	return n.relayManager.SetRelayAddr(addr)
}

// RemoveRelayAddr 移除 Relay 地址配置
func (n *Node) RemoveRelayAddr() error {
	if n.relayManager == nil {
		return nil
	}
	return n.relayManager.RemoveRelayAddr()
}

// RelayAddr 获取当前配置的 Relay 地址
func (n *Node) RelayAddr() (types.Multiaddr, bool) {
	if n.relayManager == nil {
		return nil, false
	}
	return n.relayManager.RelayAddr()
}

// RelayStats 获取 Relay 统计信息
func (n *Node) RelayStats() pkgif.RelayStats {
	if n.relayManager == nil {
		return pkgif.RelayStats{}
	}
	return n.relayManager.RelayStats()
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

	return nil
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
	logger.Info("节点已关闭（Phase D 完成）")
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

	// ════════════════════════════════════════════════════════════════════════
	// P2 对齐：网络变化时触发 DHT 重新发布
	// ════════════════════════════════════════════════════════════════════════
	go n.triggerDHTRepublish(event)
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
//
// 取代原来的 triggerDHTRepublish，提供更完整的地址变化响应。
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
	// 虽然不是严格的原子性，但可以最小化不一致窗口
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
	// Peerstore 会自动通过 Host 的 Addrs() 获取最新地址
	// 这里只需确保地址已更新到 Host
	if n.host != nil {
		logger.Debug("AddressChangeCoordinator: Step 1 - Peerstore 地址已由 Host 自动更新")
	}

	// Step 2: 全局 DHT 更新
	// v2.0 重构：节点级别的 DHT 发布由 DHT 组件自动管理
	// 此处仅记录事件，不主动触发发布
	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Debug("AddressChangeCoordinator: Step 2 - DHT 更新由组件自动处理")
	}()

	// Step 3: 更新所有已加入的 Realm 的 DHT + Relay 地址簿 + MemberList
	// 通过 RealmManager.NotifyNetworkChange 统一处理
	wg.Add(1)
	go func() {
		defer wg.Done()
		if n.realmManager != nil {
			// NotifyNetworkChange 会遍历所有活跃 Realm，调用其 OnNetworkChange
			// OnNetworkChange 内部处理：DHT 发布、Relay 地址簿更新、MemberList 广播
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

	// Step 4: 通知 Relay 地址簿（由 Realm.OnNetworkChange 内部处理）
	// Step 5: 广播给 MemberList（由 Realm.OnNetworkChange 内部处理）

	// 等待所有更新完成
	wg.Wait()

	if len(updateErrors) > 0 {
		logger.Warn("AddressChangeCoordinator: 部分更新失败",
			"errors", updateErrors)
	} else {
		logger.Info("AddressChangeCoordinator: 地址变化协调完成")
	}
}

// triggerDHTRepublish 在网络变化时触发 DHT 重新发布
// 已废弃：使用 triggerAddressChangeCoordination 替代
// 保留此函数以保持向后兼容
func (n *Node) triggerDHTRepublish(event pkgif.NetworkChangeEvent) {
	n.triggerAddressChangeCoordination(event)
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

// ════════════════════════════════════════════════════════════════════════════
//                              连接管理
// ════════════════════════════════════════════════════════════════════════════

// Connect 连接到目标节点
//
// 支持多种输入格式（自动检测）：
//  1. Full Address: /ip4/1.2.3.4/udp/4001/quic-v1/p2p/12D3KooW...
//  2. ConnectionTicket: dep2p://base58...
//  3. 纯 NodeID: 12D3KooW...（通过 DHT 发现）
//
// 连接策略（自动执行）：
//  1. 检查 Peerstore 缓存
//  2. 解析提供的地址
//  3. 尝试直连
//  4. 尝试 NAT 打洞
//  5. 回退到中继
//
// 身份验证：
//   - TLS/Noise 握手时自动验证目标身份
//   - 身份不匹配返回 ErrIdentityMismatch
//
// 示例：
//
//	// 使用完整地址连接
//	err := node.Connect(ctx, "/ip4/1.2.3.4/tcp/4001/p2p/12D3KooW...")
//
//	// 使用票据连接
//	err := node.Connect(ctx, "dep2p://5Hx3fK...")
//
//	// 仅使用 NodeID 连接（通过 DHT 发现）
//	err := node.Connect(ctx, "12D3KooW...")
func (n *Node) Connect(ctx context.Context, target string) error {
	n.mu.RLock()
	if !n.started {
		n.mu.RUnlock()
		return ErrNotStarted
	}
	n.mu.RUnlock()

	// 检测输入格式并路由到对应的处理方法
	if strings.HasPrefix(target, "dep2p://") {
		// ConnectionTicket 格式
		return n.connectByTicket(ctx, target)
	} else if strings.HasPrefix(target, "/") {
		// Multiaddr 格式
		return n.connectByMultiaddr(ctx, target)
	} else {
		// 纯 NodeID 格式
		return n.connectByNodeID(ctx, target)
	}
}

// ════════════════════════════════════════════════════════════════════════════
//                              Realm 操作
// ════════════════════════════════════════════════════════════════════════════

// JoinRealm 加入业务域
//
// 使用预共享密钥加入 Realm。
// 一个节点同时只能属于一个 Realm。
//
// 加入 Realm 后才能使用协议层服务（Messaging, PubSub, Streams, Liveness）。
//
// 返回用户级 *Realm 对象，只暴露用户需要的方法。
//
// 示例：
//
//	realmKey := []byte("my-secret-realm-key")
//	realm, err := node.JoinRealm(ctx, realmKey)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 使用通信服务
//	messaging := realm.Messaging()
//	pubsub := realm.PubSub()
func (n *Node) JoinRealm(ctx context.Context, realmKey []byte) (*Realm, error) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if !n.started {
		return nil, ErrNotStarted
	}

	if n.currentRealm != nil {
		return nil, ErrAlreadyInRealm
	}

	// 从 realmKey 派生 RealmID
	realmID := deriveRealmID(realmKey)

	// 创建 Realm（系统接口）
	internalRealm, err := n.realmManager.CreateWithOpts(ctx,
		WithRealmID(realmID),
		WithRealmPSK(realmKey),
	)
	if err != nil {
		return nil, fmt.Errorf("create realm: %w", err)
	}

	// 加入 Realm
	if err := internalRealm.Join(ctx); err != nil {
		return nil, fmt.Errorf("join realm: %w", err)
	}

	// 包装为用户级类型
	realm := &Realm{internal: internalRealm}
	n.currentRealm = realm

	// 生命周期对齐：Realm Join 成功后推进到 PhaseCRunning 并设置 ReadyLevelRealmReady
	if n.lifecycleCoordinator != nil {
		_ = n.lifecycleCoordinator.AdvanceTo(lifecycle.PhaseCRunning)
	}
	n.setReadyLevel(pkgif.ReadyLevelRealmReady)

	return realm, nil
}

// Realm 获取当前 Realm
//
// 返回当前加入的 Realm，如果未加入返回 nil。
//
// 示例：
//
//	realm := node.Realm()
//	if realm != nil {
//	    fmt.Println("Current realm:", realm.ID())
//	}
func (n *Node) Realm() *Realm {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.currentRealm
}

// LeaveRealm 离开当前 Realm
//
// 离开 Realm 后无法继续使用协议层服务。
func (n *Node) LeaveRealm(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.currentRealm == nil {
		return ErrNotInRealm
	}

	if err := n.currentRealm.internal.Leave(ctx); err != nil {
		return fmt.Errorf("leave realm: %w", err)
	}

	if err := n.currentRealm.internal.Close(); err != nil {
		return fmt.Errorf("close realm: %w", err)
	}

	n.currentRealm = nil
	return nil
}

// ════════════════════════════════════════════════════════════════════════════
//                              快捷方法
// ════════════════════════════════════════════════════════════════════════════

// Messaging 获取消息服务
//
// 必须先调用 JoinRealm 才能使用。
// 返回当前 Realm 的消息服务。
//
// 示例：
//
//	messaging := node.Messaging()
//	if messaging != nil {
//	    messaging.Send(ctx, peer, "chat", data)
//	}
func (n *Node) Messaging() *Messaging {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.currentRealm == nil {
		return nil
	}

	return n.currentRealm.Messaging()
}

// PubSub 获取发布订阅服务
//
// 必须先调用 JoinRealm 才能使用。
// 返回当前 Realm 的发布订阅服务。
//
// 示例：
//
//	pubsub := node.PubSub()
//	if pubsub != nil {
//	    topic, _ := pubsub.Join("room")
//	    topic.Publish(ctx, data)
//	}
func (n *Node) PubSub() *PubSub {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.currentRealm == nil {
		return nil
	}

	return n.currentRealm.PubSub()
}

// Streams 获取流服务
//
// 必须先调用 JoinRealm 才能使用。
// 返回当前 Realm 的流服务。
//
// 示例：
//
//	streams := node.Streams()
//	if streams != nil {
//	    stream, _ := streams.Open(ctx, peer, "file")
//	    stream.Write(data)
//	}
func (n *Node) Streams() *Streams {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.currentRealm == nil {
		return nil
	}

	return n.currentRealm.Streams()
}

// Liveness 获取存活检测服务
//
// 必须先调用 JoinRealm 才能使用。
// 返回当前 Realm 的存活检测服务。
//
// 示例：
//
//	liveness := node.Liveness()
//	if liveness != nil {
//	    rtt, _ := liveness.Ping(ctx, peer)
//	    fmt.Println("RTT:", rtt)
//	}
func (n *Node) Liveness() *Liveness {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if n.currentRealm == nil {
		return nil
	}

	return n.currentRealm.Liveness()
}

// ════════════════════════════════════════════════════════════════════════════
//                              辅助函数
// ════════════════════════════════════════════════════════════════════════════

// deriveRealmID 从 realmKey 派生 RealmID
//
// 使用标准的 HKDF-SHA256 派生算法（与 internal/realm/auth 保持一致）。
// 格式：64 字符十六进制字符串
func deriveRealmID(realmKey []byte) string {
	if len(realmKey) == 0 {
		return ""
	}

	// 使用 types.RealmIDFromPSK 标准实现（HKDF-SHA256）
	psk := types.PSK(realmKey)
	realmID := psk.DeriveRealmID()
	return string(realmID)
}

// connectByMultiaddr 通过 Multiaddr 连接
//
// 解析完整地址（包含 /p2p/<NodeID>），提取节点 ID 和传输地址。
//
// 支持 Relay 地址（/p2p-circuit/）
// 当地址包含 /p2p-circuit/ 时，需要先将 Relay 服务器地址添加到 Peerstore，
// 否则 Swarm 拨号时找不到 Relay 服务器的地址会失败。
func (n *Node) connectByMultiaddr(ctx context.Context, addr string) error {
	// 解析 Multiaddr
	ma, err := types.NewMultiaddr(addr)
	if err != nil {
		return fmt.Errorf("invalid multiaddr: %w", err)
	}

	// 检测并处理 Relay 地址
	if strings.Contains(addr, "/p2p-circuit/") {
		return n.connectViaRelayAddr(ctx, addr, ma)
	}

	// 提取 NodeID 和传输地址
	addrInfo, err := types.AddrInfoFromP2pAddr(ma)
	if err != nil {
		return fmt.Errorf("extract addrinfo: %w", err)
	}

	// 转换为字符串列表
	nodeID := string(addrInfo.ID)
	addrs := make([]string, len(addrInfo.Addrs))
	for i, a := range addrInfo.Addrs {
		addrs[i] = a.String()
	}

	// 使用 Host.Connect 连接
	if n.host == nil {
		return fmt.Errorf("host not initialized")
	}

	return n.host.Connect(ctx, nodeID, addrs)
}

// connectViaRelayAddr 通过 Relay 地址连接
//
// 处理 /p2p-circuit/ 格式的 Relay 地址
// 地址格式：/ip4/x.x.x.x/udp/port/quic-v1/p2p/<RelayID>/p2p-circuit/p2p/<TargetID>
//
// 简化后的步骤（Swarm 层已正确处理 Relay 地址）：
//  1. 提取目标节点 ID
//  2. 将完整 Relay 地址添加到目标节点的 Peerstore
//  3. 调用 Host.Connect，让 Swarm.dialPeer 自动处理 Relay 地址
func (n *Node) connectViaRelayAddr(ctx context.Context, addr string, ma types.Multiaddr) error {
	if n.host == nil {
		return fmt.Errorf("host not initialized")
	}

	// 1. 提取目标节点 ID
	// 分割 Relay 地址：/relay-addr/p2p-circuit/p2p/target
	parts := strings.Split(addr, "/p2p-circuit/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid relay address format")
	}

	targetPart := parts[1] // p2p/<TargetID>

	// 提取目标节点 ID
	targetID := ""
	if strings.HasPrefix(targetPart, "p2p/") {
		targetID = strings.TrimPrefix(targetPart, "p2p/")
	} else {
		return fmt.Errorf("invalid target in relay address: %s", targetPart)
	}

	// 2. 将完整 Relay 地址添加到目标节点的 Peerstore
	// Swarm.dialPeer 会通过 filterRelayAddrs 提取并使用这个地址
	if n.host.Peerstore() != nil {
		relayMA, err := types.NewMultiaddr(addr)
		if err != nil {
			return fmt.Errorf("invalid relay address: %w", err)
		}
		n.host.Peerstore().AddAddrs(types.PeerID(targetID), []types.Multiaddr{relayMA}, time.Hour)
	}

	// 3. 调用 Host.Connect，让 Swarm.dialPeer 自动处理
	// Swarm.dialPeer 流程：
	//   - 直连失败（无直连地址或失败）
	//   - 通过 Peerstore 中的 Relay 地址连接
	return n.host.Connect(ctx, targetID, nil)
}

// connectByTicket 通过 ConnectionTicket 连接
//
// 解码票据，提取节点 ID 和地址提示。
func (n *Node) connectByTicket(ctx context.Context, ticket string) error {
	// 解码票据
	t, err := types.DecodeConnectionTicket(ticket)
	if err != nil {
		return fmt.Errorf("decode ticket: %w", err)
	}

	// 检查票据是否过期
	if t.IsExpired(24 * time.Hour) {
		return fmt.Errorf("ticket expired")
	}

	// 如果有地址提示，直接使用
	// 根据设计文档 Section 8.3，应该先尝试直连，再回退到 Relay
	if len(t.AddressHints) > 0 {
		if n.host == nil {
			return fmt.Errorf("host not initialized")
		}
		var relayHints []string
		var directHints []string
		for _, hint := range t.AddressHints {
			if strings.Contains(hint, "/p2p-circuit/") {
				relayHints = append(relayHints, hint)
			} else {
				directHints = append(directHints, hint)
			}
		}

		var lastErr error

		// 1. 先尝试直连地址提示（符合设计文档流程）
		if len(directHints) > 0 {
			if err := n.host.Connect(ctx, t.NodeID, directHints); err == nil {
				return nil
			} else {
				lastErr = err
				logger.Debug("票据直连地址连接失败，尝试 Relay 地址",
					"nodeID", t.NodeID[:8],
					"error", err)
			}
		}

		// 2. 直连失败或无直连地址，尝试 Relay 地址提示（32 补漏）
		for _, hint := range relayHints {
			ma, err := types.NewMultiaddr(hint)
			if err != nil {
				lastErr = err
				continue
			}
			if err := n.connectViaRelayAddr(ctx, hint, ma); err == nil {
				return nil
			} else {
				lastErr = err
			}
		}

		if lastErr != nil {
			return lastErr
		}
	}

	// 否则回退到 NodeID 发现
	return n.connectByNodeID(ctx, t.NodeID)
}

// connectByNodeID 通过 NodeID 连接
//
// 使用 DHT 发现节点地址，然后连接。
func (n *Node) connectByNodeID(ctx context.Context, nodeID string) error {
	// 1. 检查 Peerstore 缓存
	if n.host != nil && n.host.Peerstore() != nil {
		peerID := types.PeerID(nodeID)
		addrs := n.host.Peerstore().Addrs(peerID)

		if len(addrs) > 0 {
			// 转换为字符串列表
			addrStrs := make([]string, len(addrs))
			for i, a := range addrs {
				addrStrs[i] = a.String()
			}

			// 尝试使用缓存的地址连接
			err := n.host.Connect(ctx, nodeID, addrStrs)
			if err == nil {
				return nil
			}
			// 缓存地址连接失败，继续尝试 DHT 发现
		}
	}

	// 2. 使用 DHT 发现节点
	if n.discovery == nil {
		return fmt.Errorf("discovery not available, cannot find node by ID")
	}

	// 类型断言为 DHTDiscovery
	dhtDiscovery, ok := n.discovery.(pkgif.DHTDiscovery)
	if !ok {
		return fmt.Errorf("discovery service does not support FindPeer (DHT required)")
	}

	// 通过 DHT 发现节点
	peerID := types.PeerID(nodeID)
	peerInfo, err := dhtDiscovery.FindPeer(ctx, peerID)
	if err != nil {
		return fmt.Errorf("dht find peer: %w", err)
	}

	if len(peerInfo.Addrs) == 0 {
		return fmt.Errorf("no addresses found for peer %s", nodeID)
	}

	// 转换为字符串列表
	addrStrs := peerInfo.AddrsToStrings()

	// 3. 连接到发现的地址
	if n.host == nil {
		return fmt.Errorf("host not initialized")
	}

	return n.host.Connect(ctx, nodeID, addrStrs)
}

// isPublicAddr 检查地址是否为公网地址
//
// 过滤掉私网、回环、link-local 地址。
func isPublicAddr(addr string) bool {
	// 回环地址
	if strings.Contains(addr, "/ip4/127.") || strings.Contains(addr, "/ip6/::1") {
		return false
	}

	// 私网地址（RFC 1918）
	if strings.Contains(addr, "/ip4/10.") ||
		strings.Contains(addr, "/ip4/172.16.") ||
		strings.Contains(addr, "/ip4/172.17.") ||
		strings.Contains(addr, "/ip4/172.18.") ||
		strings.Contains(addr, "/ip4/172.19.") ||
		strings.Contains(addr, "/ip4/172.20.") ||
		strings.Contains(addr, "/ip4/172.21.") ||
		strings.Contains(addr, "/ip4/172.22.") ||
		strings.Contains(addr, "/ip4/172.23.") ||
		strings.Contains(addr, "/ip4/172.24.") ||
		strings.Contains(addr, "/ip4/172.25.") ||
		strings.Contains(addr, "/ip4/172.26.") ||
		strings.Contains(addr, "/ip4/172.27.") ||
		strings.Contains(addr, "/ip4/172.28.") ||
		strings.Contains(addr, "/ip4/172.29.") ||
		strings.Contains(addr, "/ip4/172.30.") ||
		strings.Contains(addr, "/ip4/172.31.") ||
		strings.Contains(addr, "/ip4/192.168.") {
		return false
	}

	// Link-local 地址
	if strings.Contains(addr, "/ip4/169.254.") || strings.Contains(addr, "/ip6/fe80:") {
		return false
	}

	return true
}

// ════════════════════════════════════════════════════════════════════════════
//                              网络诊断 API（P2 修复）
// ════════════════════════════════════════════════════════════════════════════

// NetworkDiagnosticReport 网络诊断报告（用户友好类型）
type NetworkDiagnosticReport struct {
	// IPv4 相关
	IPv4Available bool   `json:"ipv4_available"`
	IPv4GlobalIP  string `json:"ipv4_global_ip,omitempty"`
	IPv4Port      int    `json:"ipv4_port,omitempty"`

	// IPv6 相关
	IPv6Available bool   `json:"ipv6_available"`
	IPv6GlobalIP  string `json:"ipv6_global_ip,omitempty"`

	// NAT 类型
	NATType string `json:"nat_type"`

	// 端口映射可用性
	UPnPAvailable   bool `json:"upnp_available"`
	NATPMPAvailable bool `json:"natpmp_available"`
	PCPAvailable    bool `json:"pcp_available"`

	// 强制门户
	CaptivePortal bool `json:"captive_portal"`

	// 中继延迟（毫秒）
	RelayLatencies map[string]int64 `json:"relay_latencies,omitempty"`

	// 生成耗时（毫秒）
	Duration int64 `json:"duration_ms"`
}

// GetNetworkDiagnostics 获取网络诊断报告
//
// 运行全面的网络诊断，检测：
//   - IPv4/IPv6 可用性和外部地址
//   - NAT 类型
//   - 端口映射协议可用性（UPnP、NAT-PMP、PCP）
//   - 是否存在强制门户
//   - 中继服务器延迟
//
// 示例：
//
//	report, err := node.GetNetworkDiagnostics(ctx)
//	if err != nil {
//	    log.Printf("诊断失败: %v", err)
//	    return
//	}
//	fmt.Printf("IPv4 外部地址: %s:%d\n", report.IPv4GlobalIP, report.IPv4Port)
//	fmt.Printf("NAT 类型: %s\n", report.NATType)
func (n *Node) GetNetworkDiagnostics(ctx context.Context) (*NetworkDiagnosticReport, error) {
	if n.netReportClient == nil {
		return nil, fmt.Errorf("network diagnostics not available (NAT module not loaded)")
	}

	report, err := n.netReportClient.GetReport(ctx)
	if err != nil {
		return nil, err
	}

	// 转换为用户友好格式
	result := &NetworkDiagnosticReport{
		IPv4Available:   report.UDPv4,
		IPv6Available:   report.UDPv6,
		UPnPAvailable:   report.UPnPAvailable,
		NATPMPAvailable: report.NATPMPAvailable,
		PCPAvailable:    report.PCPAvailable,
		Duration:        report.Duration.Milliseconds(),
		RelayLatencies:  make(map[string]int64),
	}

	// CaptivePortal 是指针类型
	if report.CaptivePortal != nil {
		result.CaptivePortal = *report.CaptivePortal
	}

	if report.GlobalV4 != nil {
		result.IPv4GlobalIP = report.GlobalV4.String()
		result.IPv4Port = int(report.GlobalV4Port)
	}

	if report.GlobalV6 != nil {
		result.IPv6GlobalIP = report.GlobalV6.String()
	}

	// NAT 类型
	result.NATType = report.NATType.String()

	// 中继延迟
	for url, latency := range report.RelayLatencies {
		result.RelayLatencies[url] = latency.Milliseconds()
	}

	return result, nil
}

// ════════════════════════════════════════════════════════════════════════════
//                       nodedb 种子节点恢复 API（P3 修复完成）
// ════════════════════════════════════════════════════════════════════════════

// SeedRecord 种子节点记录（用户友好类型）
type SeedRecord struct {
	// ID 节点 ID
	ID string `json:"id"`

	// Addrs 节点地址列表
	Addrs []string `json:"addrs"`

	// LastSeen 最后活跃时间
	LastSeen time.Time `json:"last_seen"`

	// LastPong 最后 Pong 时间
	LastPong time.Time `json:"last_pong"`
}

// RecoverSeeds 从节点缓存恢复种子节点并尝试连接
//
// 用于节点重启后快速恢复网络连接，而无需从头开始发现。
//
// 参数:
//   - ctx: 上下文
//   - count: 最大恢复节点数
//   - maxAge: 节点最大年龄（超过此时间的节点不恢复）
//
// 返回:
//   - 成功连接的节点数
//   - 恢复的种子节点列表
//   - 错误信息
//
// 示例:
//
//	connected, seeds, err := node.RecoverSeeds(ctx, 50, 24*time.Hour)
//	if err != nil {
//	    log.Printf("恢复种子失败: %v", err)
//	}
//	log.Printf("从 %d 个种子中成功连接 %d 个", len(seeds), connected)
func (n *Node) RecoverSeeds(ctx context.Context, count int, maxAge time.Duration) (int, []SeedRecord, error) {
	if n.host == nil {
		return 0, nil, fmt.Errorf("host not available")
	}

	ps := n.host.Peerstore()
	if ps == nil {
		return 0, nil, fmt.Errorf("peerstore not available")
	}

	// 查询种子节点
	seeds := ps.QuerySeeds(count, maxAge)
	if len(seeds) == 0 {
		return 0, nil, nil
	}

	// 转换为用户友好格式
	records := make([]SeedRecord, 0, len(seeds))
	for _, seed := range seeds {
		records = append(records, SeedRecord{
			ID:       seed.ID,
			Addrs:    seed.Addrs,
			LastSeen: seed.LastSeen,
			LastPong: seed.LastPong,
		})
	}

	// 尝试连接种子节点
	connected := 0
	for _, seed := range seeds {
		if len(seed.Addrs) == 0 {
			continue
		}

		// 尝试连接（传入地址列表）
		if err := n.host.Connect(ctx, seed.ID, seed.Addrs); err == nil {
			connected++
			// 更新拨号成功状态
			ps.UpdateDialAttempt(types.PeerID(seed.ID), true)
		} else {
			// 更新拨号失败状态
			ps.UpdateDialAttempt(types.PeerID(seed.ID), false)
		}
	}

	logger.Info("种子节点恢复完成", "total", len(seeds), "connected", connected)
	return connected, records, nil
}

// GetSeedCount 获取节点缓存中的种子节点数量
func (n *Node) GetSeedCount() int {
	if n.host == nil {
		return 0
	}
	ps := n.host.Peerstore()
	if ps == nil {
		return 0
	}
	return ps.NodeDBSize()
}

// ════════════════════════════════════════════════════════════════════════════
//                       introspect 自省服务 API（P3 修复完成）
// ════════════════════════════════════════════════════════════════════════════

// IntrospectInfo 自省信息（用户友好类型）
type IntrospectInfo struct {
	// Enabled 是否启用
	Enabled bool `json:"enabled"`

	// Addr 监听地址
	Addr string `json:"addr,omitempty"`

	// Endpoints 可用端点列表
	Endpoints []string `json:"endpoints,omitempty"`
}

// GetIntrospectInfo 获取自省服务信息
//
// 返回自省服务的状态和可用端点。
//
// 示例:
//
//	info := node.GetIntrospectInfo()
//	if info.Enabled {
//	    fmt.Printf("自省服务地址: %s\n", info.Addr)
//	    fmt.Printf("可用端点: %v\n", info.Endpoints)
//	}
func (n *Node) GetIntrospectInfo() IntrospectInfo {
	if n.introspectServer == nil {
		return IntrospectInfo{Enabled: false}
	}

	return IntrospectInfo{
		Enabled: true,
		Addr:    n.introspectServer.Addr(),
		Endpoints: []string{
			"/debug/introspect",
			"/debug/introspect/node",
			"/debug/introspect/connections",
			"/debug/introspect/peers",
			"/debug/introspect/bandwidth",
			"/debug/introspect/runtime",
			"/debug/pprof/",
			"/health",
		},
	}
}

// GetIntrospectAddr 获取自省服务监听地址
//
// 如果自省服务未启用，返回空字符串。
func (n *Node) GetIntrospectAddr() string {
	if n.introspectServer == nil {
		return ""
	}
	return n.introspectServer.Addr()
}

// IsIntrospectEnabled 检查自省服务是否启用
func (n *Node) IsIntrospectEnabled() bool {
	return n.introspectServer != nil
}

// buildFxApp 在 fx.go 中实现

// Realm 选项辅助函数（临时实现，应该在 pkg/interfaces 中）

// WithRealmID 设置 Realm ID
func WithRealmID(id string) pkgif.RealmOption {
	return func(cfg *pkgif.RealmConfig) {
		cfg.ID = id
	}
}

// WithRealmPSK 设置 Realm PSK
func WithRealmPSK(psk []byte) pkgif.RealmOption {
	return func(cfg *pkgif.RealmConfig) {
		cfg.PSK = psk
	}
}
