package dep2p

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/internal/core/lifecycle"     // 生命周期协调器
	"github.com/dep2p/go-dep2p/internal/core/nat/netreport" // 合并到 nat 子目录
	"github.com/dep2p/go-dep2p/internal/debug/introspect"   // 移至 debug 层
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("dep2p")

// ════════════════════════════════════════════════════════════════════════════
//                              Node 结构定义
// ════════════════════════════════════════════════════════════════════════════

// Node DeP2P 节点
//
// Node 是用户与 DeP2P 网络交互的主入口。
// 它是一个门面（Facade），聚合了所有内部组件。
//
// # 架构层次
//
//   - API Layer: Node (本层，用户直接交互)
//   - Protocol Layer: Messaging, PubSub, Streams, Liveness
//   - Realm Layer: RealmManager, Realm
//   - Core Layer: Host, Transport, Security, NAT, Relay
//   - Discovery Layer: DHT, Bootstrap, mDNS
//
// # API 分组
//
// Node 的方法按功能分布在多个文件中：
//
//   - node.go: 结构定义、New()、基本信息
//   - node_lifecycle.go: Start、Stop、Close、ReadyLevel
//   - node_address.go: 地址管理
//   - node_connect.go: 连接管理
//   - node_observe.go: 可观测性（事件、带宽、健康）
//   - node_capabilities.go: Bootstrap、Relay 能力开关
//   - node_diagnostics.go: 诊断工具
//
// # 使用示例
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
//	// 使用协议层服务
//	messaging := realm.Messaging()
//	pubsub := realm.PubSub()
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
//                              快捷方法（协议层服务）
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
//                              Realm 选项辅助函数
// ════════════════════════════════════════════════════════════════════════════

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

// buildFxApp 在 fx.go 中实现
