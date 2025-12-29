package dep2p

import (
	"fmt"
	"time"

	"github.com/dep2p/go-dep2p/internal/config"
	"github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	realmif "github.com/dep2p/go-dep2p/pkg/interfaces/realm"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// Option 用户配置选项函数
type Option func(*options) error

// options 内部选项结构
type options struct {
	// 预设配置
	preset *Preset

	// 身份配置
	identityKeyFile string
	privateKey      identity.PrivateKey

	// Realm 业务域
	realmID string

	// 监听地址
	listenAddrs []string

	// 连接配置
	connectionLimits struct {
		low  int
		high int
	}

	// 发现配置
	discovery struct {
		bootstrapPeers    []string
		bootstrapPeersSet bool // 是否显式设置了 bootstrapPeers（区分空和未设置）
	}

	// 中继配置
	relay struct {
		enable       *bool
		enableServer *bool
	}

	// NAT 配置
	nat struct {
		enable        *bool
		externalAddrs []string // 用户显式声明的公网地址（作为候选地址）
	}

	// Realm 高级配置
	realm struct {
		isolateDiscovery *bool
		isolatePubSub    *bool
	}

	// Liveness 配置
	liveness struct {
		enable            *bool
		heartbeatInterval *time.Duration
		heartbeatTimeout  *time.Duration
		enableGoodbye     *bool
	}

	// 关闭配置
	shutdown struct {
		goodbyeWait time.Duration // Goodbye 消息传播等待时间
	}

	// 日志配置
	logFile string

	// 自省服务配置
	introspect struct {
		enable *bool
		addr   string
	}

	// 用户自定义配置（JSON/文件加载）
	userConfig *UserConfig
}

// newOptions 创建默认选项
func newOptions() *options {
	return &options{
		listenAddrs: []string{},
	}
}

// toInternalConfig 转换为内部配置
func (o *options) toInternalConfig() *config.Config {
	cfg := config.NewConfig()

	// 日志文件配置（必须在最早期应用）
	cfg.LogFile = o.logFile

	// 应用预设
	if o.preset != nil {
		o.preset.Apply(cfg)
	}

	// 覆盖: Realm
	if o.realmID != "" {
		cfg.RealmID = o.realmID
	}

	// 覆盖: 身份配置
	if o.identityKeyFile != "" {
		cfg.Identity.KeyFile = o.identityKeyFile
	}
	// 应用直接注入的私钥（WithIdentity 场景）
	if o.privateKey != nil {
		cfg.Identity.PrivateKey = o.privateKey
	}

	// 覆盖: 监听地址
	if len(o.listenAddrs) > 0 {
		cfg.ListenAddrs = o.listenAddrs
	}

	// 覆盖: 连接限制
	if o.connectionLimits.low > 0 || o.connectionLimits.high > 0 {
		cfg.ConnectionManager.LowWater = o.connectionLimits.low
		cfg.ConnectionManager.HighWater = o.connectionLimits.high
	}

	// 覆盖: Bootstrap peers
	// 如果显式设置了 bootstrapPeers（包括显式设置为空，用于创世节点）
	if o.discovery.bootstrapPeersSet {
		cfg.Discovery.BootstrapPeers = o.discovery.bootstrapPeers
	}

	// 覆盖: 中继配置
	if o.relay.enable != nil {
		cfg.Relay.Enable = *o.relay.enable
	}
	if o.relay.enableServer != nil {
		cfg.Relay.EnableServer = *o.relay.enableServer
	}

	// 覆盖: NAT 配置
	if o.nat.enable != nil {
		cfg.NAT.Enable = *o.nat.enable
	}
	if len(o.nat.externalAddrs) > 0 {
		cfg.NAT.ExternalAddrs = o.nat.externalAddrs
	}

	// 覆盖: Realm 高级配置
	if o.realm.isolateDiscovery != nil {
		cfg.Realm.IsolateDiscovery = *o.realm.isolateDiscovery
	}
	if o.realm.isolatePubSub != nil {
		cfg.Realm.IsolatePubSub = *o.realm.isolatePubSub
	}

	// 覆盖: Liveness 配置
	if o.liveness.enable != nil {
		cfg.Liveness.Enable = *o.liveness.enable
	}
	if o.liveness.heartbeatInterval != nil {
		cfg.Liveness.HeartbeatInterval = *o.liveness.heartbeatInterval
	}
	if o.liveness.heartbeatTimeout != nil {
		cfg.Liveness.HeartbeatTimeout = *o.liveness.heartbeatTimeout
	}
	if o.liveness.enableGoodbye != nil {
		cfg.Liveness.EnableGoodbye = *o.liveness.enableGoodbye
	}

	// 覆盖: 自省服务配置
	if o.introspect.enable != nil {
		cfg.Introspect.Enable = *o.introspect.enable
	}
	if o.introspect.addr != "" {
		cfg.Introspect.Addr = o.introspect.addr
	}

	return cfg
}

// ============================================================================
//                              预设选项
// ============================================================================

// WithPreset 使用预设配置
//
// 预设提供针对不同场景优化的默认配置：
//   - PresetMobile: 移动端优化，低资源占用
//   - PresetDesktop: 桌面端默认配置
//   - PresetServer: 服务器优化，高性能
//   - PresetMinimal: 最小配置，仅用于测试
func WithPreset(preset *Preset) Option {
	return func(o *options) error {
		if preset == nil {
			return fmt.Errorf("预设不能为空")
		}
		o.preset = preset
		return nil
	}
}

// ============================================================================
//                              身份选项
// ============================================================================

// WithIdentityFromFile 从文件加载身份密钥
//
// 如果文件不存在，将自动创建新的身份密钥并保存。
//
//	dep2p.New(dep2p.WithIdentityFromFile("~/.dep2p/identity.key"))
func WithIdentityFromFile(path string) Option {
	return func(o *options) error {
		if path == "" {
			return fmt.Errorf("身份密钥文件路径不能为空")
		}
		o.identityKeyFile = path
		return nil
	}
}

// WithPrivateKey 使用指定的私钥作为身份
//
// 适用于程序化生成或外部管理密钥的场景。
func WithPrivateKey(key identity.PrivateKey) Option {
	return func(o *options) error {
		if key == nil {
			return fmt.Errorf("私钥不能为空")
		}
		o.privateKey = key
		return nil
	}
}

// WithIdentity 使用指定的私钥作为身份
//
// 这是 WithPrivateKey 的别名，提供更直观的命名。
//
//	key, _ := dep2p.GenerateKey()
//	dep2p.NewNode(dep2p.WithIdentity(key))
func WithIdentity(key identity.PrivateKey) Option {
	return WithPrivateKey(key)
}

// ============================================================================
//                              Realm 选项
// ============================================================================

// WithRealm 设置业务域（Realm）
//
// Realm 用于隔离不同业务的 P2P 网络：
//   - 同一 Realm 的节点互相发现
//   - 不同 Realm 的节点互不可见
//
// 示例:
//
//	dep2p.NewNode(dep2p.WithRealm("my-blockchain-mainnet"))
func WithRealm(realmID string) Option {
	return func(o *options) error {
		if realmID == "" {
			return fmt.Errorf("realm ID 不能为空")
		}
		o.realmID = realmID
		return nil
	}
}

// WithRealmIsolation 设置 Realm 隔离策略
//
// 参数:
//   - isolateDiscovery: 是否隔离节点发现（只发现同 Realm 节点）
//   - isolatePubSub: 是否隔离 Pub-Sub（只订阅同 Realm 主题）
//
// 示例:
//
//	dep2p.NewNode(
//	    dep2p.WithRealm("my-realm"),
//	    dep2p.WithRealmIsolation(true, true),
//	)
func WithRealmIsolation(isolateDiscovery, isolatePubSub bool) Option {
	return func(o *options) error {
		o.realm.isolateDiscovery = &isolateDiscovery
		o.realm.isolatePubSub = &isolatePubSub
		return nil
	}
}

// ============================================================================
//                              Liveness 选项
// ============================================================================

// WithLiveness 启用/禁用存活检测
//
// 启用后，节点将定期检测邻居的存活状态。
func WithLiveness(enable bool) Option {
	return func(o *options) error {
		o.liveness.enable = &enable
		return nil
	}
}

// WithHeartbeat 设置心跳参数
//
// 参数:
//   - interval: 心跳发送间隔
//   - timeout: 心跳超时时间（超过此时间无响应判定为离线）
//
// 示例:
//
//	dep2p.NewNode(dep2p.WithHeartbeat(15*time.Second, 30*time.Second))
func WithHeartbeat(interval, timeout time.Duration) Option {
	return func(o *options) error {
		if interval <= 0 {
			return fmt.Errorf("心跳间隔必须大于 0")
		}
		if timeout <= 0 {
			return fmt.Errorf("心跳超时必须大于 0")
		}
		if timeout < interval {
			return fmt.Errorf("心跳超时不能小于心跳间隔")
		}
		o.liveness.heartbeatInterval = &interval
		o.liveness.heartbeatTimeout = &timeout
		return nil
	}
}

// WithGoodbye 启用/禁用 Goodbye 协议
//
// 启用后，节点关闭前会向邻居发送 Goodbye 消息。
func WithGoodbye(enable bool) Option {
	return func(o *options) error {
		o.liveness.enableGoodbye = &enable
		return nil
	}
}

// ============================================================================
//                              关闭选项
// ============================================================================

// WithGoodbyeWait 设置 Goodbye 消息传播等待时间
//
// 节点关闭时会先发送 Goodbye 消息，然后等待指定时间让消息传播到邻居节点，
// 最后再断开连接和释放资源。
//
// 默认值: 0（不等待）
// 推荐值: 500ms - 2s
//
// 示例:
//
//	dep2p.NewNode(dep2p.WithGoodbyeWait(time.Second))
func WithGoodbyeWait(wait time.Duration) Option {
	return func(o *options) error {
		if wait < 0 {
			return fmt.Errorf("goodbye 等待时间不能为负数")
		}
		o.shutdown.goodbyeWait = wait
		return nil
	}
}

// ============================================================================
//                              日志选项
// ============================================================================

// WithLogFile 将日志输出重定向到指定文件
//
// 默认情况下，dep2p 的结构化日志会输出到 stderr。
// 使用此选项可以将所有日志写入文件，避免干扰交互式程序的输出。
//
// 文件会以追加模式打开（os.O_APPEND），多次运行会累积日志。
//
// 示例:
//
//	dep2p.NewNode(dep2p.WithLogFile("dep2p.log"))
func WithLogFile(path string) Option {
	return func(o *options) error {
		if path == "" {
			return fmt.Errorf("日志文件路径不能为空")
		}
		o.logFile = path
		return nil
	}
}

// ============================================================================
//                              监听选项
// ============================================================================

// WithListenPort 使用指定端口监听所有接口（QUIC）
//
// 在 IPv4 和 IPv6 上同时监听指定端口，使用 QUIC 协议。
// port=0 表示使用系统分配的随机端口。
//
// 示例:
//
//	dep2p.NewNode(dep2p.WithListenPort(4001))
func WithListenPort(port int) Option {
	return func(o *options) error {
		if port < 0 || port > 65535 {
			return fmt.Errorf("无效的端口号: %d", port)
		}
		// port=0 表示由系统分配随机端口。
		// 注意：如果同时配置 ip4+ip6 且 port=0，会导致两套监听器拿到不同的随机端口，
		// 进而让“把 ListenAddrs 作为可拨号地址”的场景（尤其是测试）变得不稳定。
		// 因此这里在 port=0 时只启用 IPv4 随机端口监听（0.0.0.0:0）。
		if port == 0 {
			o.listenAddrs = []string{"/ip4/0.0.0.0/udp/0/quic-v1"}
			return nil
		}

		o.listenAddrs = []string{
			fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", port),
			fmt.Sprintf("/ip6/::/udp/%d/quic-v1", port),
		}
		return nil
	}
}

// WithExtraListenAddrs 追加监听地址（不覆盖已有 WithListenPort/用户配置的 ListenAddrs）。
//
// 典型用途：
// - 为 RelayTransport 增加 /p2p-circuit 监听地址，使节点能够接收中继入站连接。
//
// 注意：
// - 该选项只追加到 listenAddrs 列表；最终由 Endpoint.Listen() 统一启动监听器。
func WithExtraListenAddrs(addrs ...string) Option {
	return func(o *options) error {
		for _, a := range addrs {
			if a == "" {
				continue
			}
			o.listenAddrs = append(o.listenAddrs, a)
		}
		return nil
	}
}

// ============================================================================
//                              连接选项
// ============================================================================

// WithConnectionLimits 设置连接数限制
//
// 参数:
//   - low: 低水位线，连接数低于此值时停止裁剪
//   - high: 高水位线，连接数超过此值时触发裁剪
//
// 示例:
//
//	dep2p.New(dep2p.WithConnectionLimits(50, 100))
func WithConnectionLimits(low, high int) Option {
	return func(o *options) error {
		if low < 0 || high < 0 {
			return fmt.Errorf("连接限制不能为负数")
		}
		if low > high {
			return fmt.Errorf("低水位线不能大于高水位线")
		}
		o.connectionLimits.low = low
		o.connectionLimits.high = high
		return nil
	}
}

// ============================================================================
//                              发现选项
// ============================================================================

// WithBootstrapPeers 设置引导节点
//
// 引导节点用于首次加入网络时发现其他节点。
// 地址格式（必须是完整地址）: /ip4/x.x.x.x/udp/4001/quic-v1/p2p/12D3KooW...
//
// REQ-BOOT-001 强制约束：
//   - Bootstrap seed 必须是 Full Address（含 /p2p/<NodeID>）
//   - 不允许使用 DialAddr（无 /p2p/ 后缀）
//   - 不允许使用 RelayCircuitAddress（含 /p2p-circuit/）
//
// 特殊用法：
//   - WithBootstrapPeers(nil) 或 WithBootstrapPeers() 表示创世节点（无 bootstrap）
//   - 这会覆盖 Preset 中的默认 bootstrap 节点
//
// 示例：
//
//	// 创世节点（无 bootstrap）
//	dep2p.NewNode(dep2p.WithBootstrapPeers(nil))
//
//	// 使用自定义 bootstrap
//	dep2p.NewNode(dep2p.WithBootstrapPeers(
//	    "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/12D3KooW...",
//	))
func WithBootstrapPeers(peers ...string) Option {
	return func(o *options) error {
		// REQ-BOOT-001: 解析期强校验
		for _, peer := range peers {
			if peer == "" {
				continue // 允许空字符串（会被后续过滤）
			}

			ma := types.Multiaddr(peer)

			// 强制 Full Address 校验：必须包含 /p2p/<NodeID>
			if ma.PeerID().IsEmpty() {
				return fmt.Errorf("invalid bootstrap peer %q: must be Full Address (with /p2p/<NodeID>)", peer)
			}

			// 禁止 RelayCircuit 地址作为 seed
			if ma.IsRelay() {
				return fmt.Errorf("invalid bootstrap peer %q: RelayCircuitAddress not allowed as bootstrap seed", peer)
			}
		}

		o.discovery.bootstrapPeers = peers
		o.discovery.bootstrapPeersSet = true // 标记为显式设置
		return nil
	}
}

// ============================================================================
//                              中继选项
// ============================================================================

// WithRelay 启用/禁用中继客户端
//
// 启用后，节点可以通过中继服务器与 NAT 后的节点通信。
func WithRelay(enable bool) Option {
	return func(o *options) error {
		o.relay.enable = &enable
		return nil
	}
}

// WithRelayServer 启用/禁用中继服务器功能
//
// 启用后，本节点可以作为中继服务器为其他节点提供中继服务。
func WithRelayServer(enable bool) Option {
	return func(o *options) error {
		o.relay.enableServer = &enable
		return nil
	}
}

// ============================================================================
//                              NAT 选项
// ============================================================================

// WithNAT 启用/禁用 NAT 穿透
//
// 启用后，节点将尝试检测 NAT 类型并进行端口映射。
func WithNAT(enable bool) Option {
	return func(o *options) error {
		o.nat.enable = &enable
		return nil
	}
}

// WithExternalAddrs 显式声明公网地址（作为候选地址）
//
// 用于公网服务器场景：当节点有独立公网IP但无法通过UPnP/STUN自动发现时，
// 用户可以显式声明其公网地址。
//
// 重要语义（INV-005 兼容）：
//   - 这些地址将作为 Candidate 输入到 dial-back 验证流程
//   - 验证成功后才会成为 VerifiedDirect 并发布到 DHT
//   - 验证需要至少一个已连接的 helper 节点
//
// 根据 IMPL-ADDRESS-UNIFICATION.md 规范，仅支持 multiaddr 格式：
//   - "/ip4/203.0.113.5/udp/4001/quic-v1"
//   - "/ip6/::1/udp/4001/quic-v1"
//   - "/dns4/example.com/udp/4001/quic-v1"
//
// 如需从 host:port 格式转换，请使用 types.FromHostPort：
//
//	ma, _ := types.FromHostPort("47.95.1.2", 4001, "udp/quic-v1")
//	dep2p.WithExternalAddrs(ma.String())
//
// 示例:
//
//	// 阿里云 ECS 等无 UPnP 的公网服务器
//	dep2p.NewNode(
//	    dep2p.WithListenPort(4001),
//	    dep2p.WithExternalAddrs("/ip4/47.95.1.2/udp/4001/quic-v1"),
//	)
func WithExternalAddrs(addrs ...string) Option {
	return func(o *options) error {
		for _, addr := range addrs {
			if addr == "" {
				continue
			}
			// 根据 IMPL-ADDRESS-UNIFICATION.md：仅接受 multiaddr 格式
			ma, err := types.ParseMultiaddr(addr)
			if err != nil {
				return fmt.Errorf("无效的公网地址 %q（仅支持 multiaddr 格式，如需 host:port 请使用 types.FromHostPort 转换）: %w", addr, err)
			}
			o.nat.externalAddrs = append(o.nat.externalAddrs, ma.String())
		}
		return nil
	}
}

// ============================================================================
//                              配置文件选项
// ============================================================================

// WithConfig 使用 UserConfig 结构体配置
//
// 适用于从 JSON/YAML 文件加载配置的场景。
func WithConfig(cfg *UserConfig) Option {
	return func(o *options) error {
		if cfg == nil {
			return fmt.Errorf("配置不能为空")
		}
		o.userConfig = cfg
		return nil
	}
}

// ============================================================================
//                              自省服务选项
// ============================================================================

// WithIntrospect 启用或禁用本地自省 HTTP 服务
//
// 自省服务提供 JSON 格式的诊断信息，用于调试和监控：
//   - GET /debug/introspect      - 完整诊断报告
//   - GET /debug/introspect/node - 节点信息
//   - GET /debug/introspect/connections - 连接信息
//   - GET /debug/introspect/realm - Realm 信息
//   - GET /debug/introspect/relay - Relay 信息
//   - GET /debug/pprof/*         - Go pprof 端点
//
// 默认监听地址: 127.0.0.1:6060（仅本地可访问）
//
// 示例:
//
//	node, _ := dep2p.StartNode(ctx,
//	    dep2p.WithIntrospect(true),
//	)
//	// 访问 http://127.0.0.1:6060/debug/introspect
func WithIntrospect(enable bool) Option {
	return func(o *options) error {
		o.introspect.enable = &enable
		return nil
	}
}

// WithIntrospectAddr 设置自省服务监听地址
//
// 默认值: "127.0.0.1:6060"
//
// 安全警告: 不建议将自省服务暴露到公网，pprof 端点可能泄露敏感信息。
//
// 示例:
//
//	node, _ := dep2p.StartNode(ctx,
//	    dep2p.WithIntrospect(true),
//	    dep2p.WithIntrospectAddr("127.0.0.1:9090"),
//	)
func WithIntrospectAddr(addr string) Option {
	return func(o *options) error {
		o.introspect.addr = addr
		return nil
	}
}

// ============================================================================
//                              Realm 加入选项（IMPL-1227）
// ============================================================================

// WithRealmKey 设置 Realm 密钥（加入 Realm 时使用）
//
// 这是 realmif.WithRealmKey 的便捷别名，允许用户直接使用 dep2p.WithRealmKey。
//
// 示例:
//
//	realmKey := types.GenerateRealmKey()
//	realm, err := node.JoinRealm(ctx, "my-realm", dep2p.WithRealmKey(realmKey))
func WithRealmKey(key types.RealmKey) realmif.RealmOption {
	return realmif.WithRealmKey(key)
}

// ============================================================================
//                              辅助函数
// ============================================================================
