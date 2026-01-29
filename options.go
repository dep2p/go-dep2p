package dep2p

import (
	"fmt"
	"time"

	"go.uber.org/fx"

	"github.com/dep2p/go-dep2p/config"
)

// Option 配置选项函数
//
// 使用函数式选项模式配置节点。
// 每个 Option 修改内部配置状态。
//
// 示例：
//
//	node, err := dep2p.New(ctx,
//	    dep2p.WithPreset("server"),
//	    dep2p.WithListenPort(4001),
//	    dep2p.WithRelay(true),
//	)
type Option func(*nodeConfig) error

// nodeConfig 内部配置
//
// 由 Option 函数填充。
// 包含统一的 config.Config 和额外的运行时配置。
type nodeConfig struct {
	// config 统一配置（所有配置的唯一来源）
	config *config.Config

	// preset 预设名称（记录使用的预设）
	preset string

	// listenAddrs 监听地址列表（覆盖预设）
	listenAddrs []string

	// advertiseAddrs 通告地址列表（不绑定，仅对外公告）
	// 用于云服务器场景：实际监听 0.0.0.0，但对外公告公网 IP
	advertiseAddrs []string

	// logFile 日志文件路径
	logFile string

	// dataDir 数据目录路径（覆盖 config.Storage.DataDir）
	dataDir string

	// trustSTUNAddresses STUN 信任模式
	// 启用后，STUN 发现的地址将直接标记为已验证
	trustSTUNAddresses bool

	// userFxOptions 用户自定义 Fx 选项
	// 允许用户注入自定义模块到依赖注入容器
	userFxOptions []fx.Option
}

// newNodeConfig 创建默认的 nodeConfig
func newNodeConfig() *nodeConfig {
	return &nodeConfig{
		config: config.NewConfig(),
	}
}

// ════════════════════════════════════════════════════════════════════════════
//
//	预设选项
//
// ════════════════════════════════════════════════════════════════════════════

// WithPreset 使用预设配置
//
// 预设提供开箱即用的配置组合。
// 可选预设："mobile", "desktop", "server", "minimal"
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithPreset("server"))
func WithPreset(presetName string) Option {
	return func(cfg *nodeConfig) error {
		if err := config.ApplyPreset(cfg.config, presetName); err != nil {
			return err
		}
		cfg.preset = presetName
		return nil
	}
}

// ════════════════════════════════════════════════════════════════════════════
//
//	网络选项
//
// ════════════════════════════════════════════════════════════════════════════

// WithListenPort 设置监听端口
//
// 指定 UDP 端口用于 QUIC 传输。
// 设为 0 时系统自动分配随机可用端口。
//
// 注意：在 Linux 系统上，绑定 `0.0.0.0` 时会自动同时监听 IPv4 和 IPv6
// （当 net.ipv6.bindv6only=0 时），因此无需显式添加 IPv6 地址。
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithListenPort(4001))
func WithListenPort(port int) Option {
	return func(cfg *nodeConfig) error {
		if port < 0 || port > 65535 {
			return fmt.Errorf("invalid port: %d (must be 0-65535)", port)
		}
		// 生成默认监听地址（QUIC）
		// Linux 双栈机制：绑定 0.0.0.0 会自动包含 IPv6，无需显式添加 [::]
		cfg.listenAddrs = []string{
			fmt.Sprintf("/ip4/0.0.0.0/udp/%d/quic-v1", port),
		}
		return nil
	}
}

// WithListenAddrs 设置监听地址
//
// 自定义监听地址列表，覆盖预设的默认地址。
// 地址格式为 multiaddr，例如："/ip4/0.0.0.0/udp/4001/quic-v1"
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithListenAddrs(
//	    "/ip4/0.0.0.0/udp/4001/quic-v1",
//	    "/ip6/::/udp/4001/quic-v1",
//	))
func WithListenAddrs(addrs ...string) Option {
	return func(cfg *nodeConfig) error {
		if len(addrs) == 0 {
			return fmt.Errorf("at least one address is required")
		}
		cfg.listenAddrs = addrs
		return nil
	}
}

// WithDialTimeout 设置拨号超时
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithDialTimeout(30*time.Second))
func WithDialTimeout(timeout time.Duration) Option {
	return func(cfg *nodeConfig) error {
		if timeout <= 0 {
			return fmt.Errorf("dial timeout must be positive")
		}
		cfg.config.Transport.DialTimeout = config.Duration(timeout)
		return nil
	}
}

// ════════════════════════════════════════════════════════════════════════════
//
//	传输选项
//
// ════════════════════════════════════════════════════════════════════════════

// WithQUIC 启用或禁用 QUIC 传输
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithQUIC(true))
func WithQUIC(enable bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.config.Transport.EnableQUIC = enable
		return nil
	}
}

// WithTCP 启用或禁用 TCP 传输
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithTCP(true))
func WithTCP(enable bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.config.Transport.EnableTCP = enable
		return nil
	}
}

// WithWebSocket 启用或禁用 WebSocket 传输
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithWebSocket(true))
func WithWebSocket(enable bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.config.Transport.EnableWebSocket = enable
		return nil
	}
}

// ════════════════════════════════════════════════════════════════════════════
//
//	安全选项
//
// ════════════════════════════════════════════════════════════════════════════

// WithTLS 启用或禁用 TLS
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithTLS(true))
func WithTLS(enable bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.config.Security.EnableTLS = enable
		return nil
	}
}

// WithNoise 启用或禁用 Noise
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithNoise(true))
func WithNoise(enable bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.config.Security.EnableNoise = enable
		return nil
	}
}

// ════════════════════════════════════════════════════════════════════════════
//
//	身份选项
//
// ════════════════════════════════════════════════════════════════════════════

// WithIdentityFromFile 从文件加载身份密钥
//
// 指定密钥文件路径。
// 如果文件不存在，将自动生成新密钥并保存。
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithIdentityFromFile("~/.dep2p/identity.key"))
func WithIdentityFromFile(path string) Option {
	return func(cfg *nodeConfig) error {
		if path == "" {
			return fmt.Errorf("identity key file path cannot be empty")
		}
		cfg.config.Identity.KeyFile = path
		return nil
	}
}

// WithIdentityKeyType 设置密钥类型
//
// 支持的类型: "Ed25519", "RSA", "ECDSA", "Secp256k1"
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithIdentityKeyType("Ed25519"))
func WithIdentityKeyType(keyType string) Option {
	return func(cfg *nodeConfig) error {
		switch keyType {
		case "Ed25519", "RSA", "ECDSA", "Secp256k1":
			cfg.config.Identity.KeyType = keyType
		default:
			return fmt.Errorf("invalid key type: %s", keyType)
		}
		return nil
	}
}

// ════════════════════════════════════════════════════════════════════════════
//
//	发现选项
//
// ════════════════════════════════════════════════════════════════════════════

// WithDHT 启用或禁用 DHT
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithDHT(true))
func WithDHT(enable bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.config.Discovery.EnableDHT = enable
		return nil
	}
}

// WithMDNS 启用或禁用 mDNS
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithMDNS(true))
func WithMDNS(enable bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.config.Discovery.EnableMDNS = enable
		return nil
	}
}

// WithBootstrapPeers 设置引导节点
//
// 引导节点用于初始连接和 DHT 引导。
// 调用此方法会自动启用 Bootstrap 和 DHT 发现功能。
// 地址格式为完整的 multiaddr，例如：
//
//	"/ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ"
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithBootstrapPeers(
//	    "/ip4/104.131.131.82/tcp/4001/p2p/QmaCpDMGvV2BGHeYERUEnRQAwe3N8SzbUtfsmvsqQLuvuJ",
//	))
func WithBootstrapPeers(peers ...string) Option {
	return func(cfg *nodeConfig) error {
		if len(peers) == 0 {
			return fmt.Errorf("at least one bootstrap peer is required")
		}
		cfg.config.Discovery.Bootstrap.Peers = peers
		// 自动启用 Bootstrap 和 DHT，因为用户明确想要连接引导节点
		cfg.config.Discovery.EnableBootstrap = true
		cfg.config.Discovery.EnableDHT = true
		return nil
	}
}

// WithKnownPeers 设置已知节点列表
//
// 启动时将直接连接这些节点，不依赖引导节点或 DHT 发现。
// 适用于云服务器部署、私有网络等已知节点地址的场景。
//
// 与 WithBootstrapPeers 的区别：
//   - WithBootstrapPeers: 用于 DHT 引导，需要引导节点提供服务
//   - WithKnownPeers: 直接连接已知节点，不依赖任何发现机制
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithKnownPeers(
//	    config.KnownPeer{
//	        PeerID: "QmPeer1...",
//	        Addrs:  []string{"/ip4/1.2.3.4/udp/4001/quic-v1"},
//	    },
//	    config.KnownPeer{
//	        PeerID: "QmPeer2...",
//	        Addrs:  []string{"/ip4/5.6.7.8/udp/4001/quic-v1"},
//	    },
//	))
func WithKnownPeers(peers ...config.KnownPeer) Option {
	return func(cfg *nodeConfig) error {
		if len(peers) == 0 {
			return fmt.Errorf("at least one known peer is required")
		}
		for _, peer := range peers {
			if peer.PeerID == "" {
				return fmt.Errorf("known peer must have a valid PeerID")
			}
			if len(peer.Addrs) == 0 {
				return fmt.Errorf("known peer %s must have at least one address", peer.PeerID)
			}
		}
		cfg.config.KnownPeers = append(cfg.config.KnownPeers, peers...)
		return nil
	}
}

// EnableBootstrap 启用 Bootstrap 能力（ADR-0009）
//
// 将当前节点设置为引导节点，为网络中的新节点提供初始对等方发现服务。
// 启用后，节点将：
//   - 维护扩展的节点存储（最多 50,000 个节点）
//   - 定期探测存储节点的存活状态
//   - 主动通过 Random Walk 发现新节点
//   - 响应 FIND_NODE 请求，返回最近 K 个节点
//
// 前置条件：
//   - 节点必须有公网可达地址（非 NAT 后）
//
// 所有运营参数使用内置默认值，用户无需也无法配置。
//
// 示例：
//
//	// 项目方部署引导节点
//	dep2p.New(ctx, dep2p.EnableBootstrap(true))
func EnableBootstrap(enable bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.config.Discovery.Bootstrap.EnableService = enable
		if enable {
			// 
			cfg.config.NAT.LockReachabilityPublic = true
		}
		return nil
	}
}

// EnableInfrastructure 快捷方式：同时启用 Bootstrap 和 Relay
//
// 用于项目方快速部署基础设施节点。
// 等价于同时调用 EnableBootstrap(true) 和 EnableRelayServer(true)。
//
// 前置条件：
//   - 节点必须有公网可达地址（非 NAT 后）
//
// 示例：
//
//	// 项目方部署基础设施节点
//	dep2p.New(ctx, dep2p.EnableInfrastructure(true))
func EnableInfrastructure(enable bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.config.Discovery.Bootstrap.EnableService = enable
		cfg.config.Relay.EnableServer = enable
		if enable {
			cfg.config.Discovery.EnableMDNS = false // 基础设施节点禁用 mDNS（云服务器无局域网邻居）
			// 
			// 基础设施节点配置了公网地址，不应被 AutoNAT 降级为 Private
			cfg.config.NAT.LockReachabilityPublic = true
		}
		return nil
	}
}

// ════════════════════════════════════════════════════════════════════════════
//
//	NAT 选项
//
// ════════════════════════════════════════════════════════════════════════════

// WithAutoNAT 启用或禁用 AutoNAT
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithAutoNAT(true))
func WithAutoNAT(enable bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.config.NAT.EnableAutoNAT = enable
		return nil
	}
}

// WithUPnP 启用或禁用 UPnP
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithUPnP(true))
func WithUPnP(enable bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.config.NAT.EnableUPnP = enable
		return nil
	}
}

// WithNATPMP 启用或禁用 NAT-PMP
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithNATPMP(true))
func WithNATPMP(enable bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.config.NAT.EnableNATPMP = enable
		return nil
	}
}

// WithHolePunch 启用或禁用 Hole Punching
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithHolePunch(true))
func WithHolePunch(enable bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.config.NAT.EnableHolePunch = enable
		return nil
	}
}

// WithNAT 启用或禁用所有 NAT 穿透技术
//
// 启用后会自动尝试 UPnP、AutoNAT、打洞等技术。
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithNAT(true))
func WithNAT(enable bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.config.NAT.EnableAutoNAT = enable
		cfg.config.NAT.EnableUPnP = enable
		cfg.config.NAT.EnableNATPMP = enable
		cfg.config.NAT.EnableHolePunch = enable
		return nil
	}
}

// WithNATPMPTimeout 设置 NAT-PMP 操作超时时间
//
// 21: 添加可配置超时，避免 NAT-PMP 探测长时间阻塞
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithNATPMPTimeout(5*time.Second))
func WithNATPMPTimeout(timeout time.Duration) Option {
	return func(cfg *nodeConfig) error {
		if timeout <= 0 {
			return fmt.Errorf("NAT-PMP timeout must be positive")
		}
		cfg.config.NAT.NATPMP.Timeout = timeout
		return nil
	}
}

// WithUPnPTimeout 设置 UPnP 操作超时时间
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithUPnPTimeout(5*time.Second))
func WithUPnPTimeout(timeout time.Duration) Option {
	return func(cfg *nodeConfig) error {
		if timeout <= 0 {
			return fmt.Errorf("UPnP timeout must be positive")
		}
		cfg.config.NAT.UPnP.Timeout = timeout
		return nil
	}
}

// WithTrustSTUNAddresses 启用 STUN 信任模式
//
// 启用后，STUN 发现的地址将直接标记为已验证（verified），
// 无需通过 dial-back 或 witness 验证。
//
// 适用场景：
//   - 云服务器部署（VPC 环境，公网 IP 由 NAT Gateway 提供）
//   - 已知公网可达的环境
//   - 私有网络部署
//
// 风险提示：
//   - 仅在受控环境中启用
//   - 如果 STUN 服务器被劫持，可能导致地址欺骗
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithTrustSTUNAddresses(true))
func WithTrustSTUNAddresses(trust bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.trustSTUNAddresses = trust
		return nil
	}
}

// ════════════════════════════════════════════════════════════════════════════
//
//	中继选项
//
// ════════════════════════════════════════════════════════════════════════════

// WithRelay 启用或禁用中继客户端
//
// 启用后节点可通过中继节点与 NAT 后的节点通信。
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithRelay(true))
func WithRelay(enable bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.config.Relay.EnableClient = enable
		return nil
	}
}

// WithRelayServer 启用或禁用中继服务端
//
// 启用后节点可以作为中继服务器，帮助其他节点建立连接。
// 通常用于公网可达的服务器节点。
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithRelayServer(true))
func WithRelayServer(enable bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.config.Relay.EnableServer = enable
		return nil
	}
}

// EnableRelayServer 启用 Relay 服务能力
//
// 将当前节点设置为中继服务器，为 NAT 后的节点提供中继服务。
//
// 前置条件：
//   - 节点必须有公网可达地址（非 NAT 后）
//
// 所有资源限制参数使用内置默认值，用户无需也无法配置。
//
// 示例：
//
//	// 部署 Relay 服务器
//	dep2p.New(ctx, dep2p.EnableRelayServer(true))
func EnableRelayServer(enable bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.config.Relay.EnableServer = enable
		if enable {
			// 
			cfg.config.NAT.LockReachabilityPublic = true
		}
		return nil
	}
}

// WithRelayAddr 设置要使用的 Relay 地址（客户端配置）
//
// 指定节点应使用的 Relay 地址。
//
// 参数：
//   - addr: Relay 的完整 multiaddr 地址
//
// 示例：
//
//	// 普通节点使用 Relay
//	dep2p.New(ctx,
//	    dep2p.WithRelayAddr("/ip4/relay.dep2p.io/tcp/4001/p2p/QmRelay..."),
//	)
func WithRelayAddr(addr string) Option {
	return func(cfg *nodeConfig) error {
		if addr == "" {
			return fmt.Errorf("relay address cannot be empty")
		}
		cfg.config.Relay.RelayAddr = addr
		return nil
	}
}

// WithPublicAddr 设置公网可达地址（通告地址）
//
// 显式声明节点的公网可达地址。此地址仅用于对外通告，不会尝试绑定。
// 这对于云服务器（如阿里云、AWS）场景非常重要：
//   - 云服务器内网 IP 与公网 IP 不同
//   - 节点实际监听 0.0.0.0:port（使用 WithListenPort）
//   - 但对外公告公网 IP（使用 WithPublicAddr）
//
// 典型场景：
//   - 部署 Bootstrap 节点时指定对外公开的地址
//   - 部署 Relay 节点时指定对外公开的地址
//   - 节点位于 NAT 后，但已通过端口映射配置公网可达
//
// 示例：
//
//	// 基础设施节点：监听 0.0.0.0:4001，对外公告公网 IP
//	dep2p.New(ctx,
//	    dep2p.EnableInfrastructure(true),
//	    dep2p.WithListenPort(4001),  // 监听所有接口
//	    dep2p.WithPublicAddr("/ip4/1.2.3.4/udp/4001/quic-v1"),  // 公告公网地址
//	)
func WithPublicAddr(addr string) Option {
	return func(cfg *nodeConfig) error {
		if addr == "" {
			return fmt.Errorf("public address cannot be empty")
		}
		cfg.advertiseAddrs = append(cfg.advertiseAddrs, addr)
		return nil
	}
}

// ════════════════════════════════════════════════════════════════════════════
//
//	连接管理选项
//
// ════════════════════════════════════════════════════════════════════════════

// WithConnectionLimits 设置连接限制
//
// low: 低水位线，当连接数低于此值时主动发现并连接新节点
// high: 高水位线，当连接数超过此值时开始清理低优先级连接
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithConnectionLimits(50, 100))
func WithConnectionLimits(low, high int) Option {
	return func(cfg *nodeConfig) error {
		if low < 0 {
			return fmt.Errorf("low water mark must be >= 0, got %d", low)
		}
		if high < low {
			return fmt.Errorf("high water mark (%d) must be >= low water mark (%d)", high, low)
		}
		cfg.config.ConnMgr.LowWater = low
		cfg.config.ConnMgr.HighWater = high
		return nil
	}
}

// WithGracePeriod 设置新连接保护期
//
// 新连接在此期间内不会被清理
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithGracePeriod(30*time.Second))
func WithGracePeriod(period time.Duration) Option {
	return func(cfg *nodeConfig) error {
		if period < 0 {
			return fmt.Errorf("grace period must be non-negative")
		}
		cfg.config.ConnMgr.GracePeriod = config.Duration(period)
		return nil
	}
}

// ════════════════════════════════════════════════════════════════════════════
//
//	资源管理选项
//
// ════════════════════════════════════════════════════════════════════════════

// WithResourceManager 启用或禁用资源管理
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithResourceManager(true))
func WithResourceManager(enable bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.config.Resource.EnableResourceManager = enable
		return nil
	}
}

// WithMaxConnections 设置系统最大连接数
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithMaxConnections(1000))
func WithMaxConnections(max int) Option {
	return func(cfg *nodeConfig) error {
		if max <= 0 {
			return fmt.Errorf("max connections must be positive")
		}
		cfg.config.Resource.System.MaxConnections = max
		return nil
	}
}

// WithMaxStreams 设置系统最大流数
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithMaxStreams(10000))
func WithMaxStreams(max int) Option {
	return func(cfg *nodeConfig) error {
		if max <= 0 {
			return fmt.Errorf("max streams must be positive")
		}
		cfg.config.Resource.System.MaxStreams = max
		return nil
	}
}

// WithMaxMemory 设置系统最大内存
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithMaxMemory(1<<30)) // 1 GB
func WithMaxMemory(max int64) Option {
	return func(cfg *nodeConfig) error {
		if max < 0 {
			return fmt.Errorf("max memory must be non-negative")
		}
		cfg.config.Resource.System.MaxMemory = max
		return nil
	}
}

// ════════════════════════════════════════════════════════════════════════════
//
//	消息传递选项
//
// ════════════════════════════════════════════════════════════════════════════

// WithPubSub 启用或禁用 PubSub
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithPubSub(true))
func WithPubSub(enable bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.config.Messaging.EnablePubSub = enable
		return nil
	}
}

// WithStreams 启用或禁用 Streams
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithStreams(true))
func WithStreams(enable bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.config.Messaging.EnableStreams = enable
		return nil
	}
}

// WithLiveness 启用或禁用 Liveness
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithLiveness(true))
func WithLiveness(enable bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.config.Messaging.EnableLiveness = enable
		return nil
	}
}

// ════════════════════════════════════════════════════════════════════════════
//
//	Realm 选项
//
// ════════════════════════════════════════════════════════════════════════════

// WithRealm 启用或禁用所有 Realm 组件
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithRealm(true))
func WithRealm(enable bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.config.Realm.EnableGateway = enable
		cfg.config.Realm.EnableRouting = enable
		cfg.config.Realm.EnableMember = enable
		cfg.config.Realm.EnableAuth = enable
		return nil
	}
}

// WithRealmGateway 启用或禁用 Realm Gateway
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithRealmGateway(true))
func WithRealmGateway(enable bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.config.Realm.EnableGateway = enable
		return nil
	}
}

// WithRealmAuth 启用或禁用 Realm Auth
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithRealmAuth(true))
func WithRealmAuth(enable bool) Option {
	return func(cfg *nodeConfig) error {
		cfg.config.Realm.EnableAuth = enable
		return nil
	}
}

// ════════════════════════════════════════════════════════════════════════════
//
//	日志选项
//
// ════════════════════════════════════════════════════════════════════════════

// WithLogFile 设置日志文件路径
//
// 如果不设置，日志输出到 stderr。
// 设置后日志将写入指定文件。
//
// 示例：
//
//	dep2p.New(ctx, dep2p.WithLogFile("/var/log/dep2p.log"))
func WithLogFile(path string) Option {
	return func(cfg *nodeConfig) error {
		if path == "" {
			return fmt.Errorf("log file path cannot be empty")
		}
		cfg.logFile = path
		return nil
	}
}

// WithDataDir 设置数据目录路径
//
// 数据目录用于存放 BadgerDB 数据库和其他持久化数据。
// 所有组件统一使用此目录，通过 Key 前缀隔离数据。
//
// 目录结构：
//
//	${DataDir}/
//	├── dep2p.db/           # BadgerDB 主数据库
//	└── logs/               # 日志目录（可选）
//
// 示例：
//
//	dep2p.Start(ctx, dep2p.WithDataDir("./myapp/data"))
func WithDataDir(path string) Option {
	return func(cfg *nodeConfig) error {
		if path == "" {
			return fmt.Errorf("data directory path cannot be empty")
		}
		cfg.dataDir = path
		// 同时更新 config 中的 Storage.DataDir
		if cfg.config == nil {
			cfg.config = config.NewConfig()
		}
		cfg.config.Storage.DataDir = path
		return nil
	}
}

// ════════════════════════════════════════════════════════════════════════════
//
//	配置文件选项
//
// ════════════════════════════════════════════════════════════════════════════

// WithConfig 直接使用完整配置
//
// 使用已有的 config.Config 结构体。
// 通常用于从配置文件加载后应用。
//
// 示例：
//
//	cfg := config.NewServerConfig()
//	dep2p.New(ctx, dep2p.WithConfig(cfg))
func WithConfig(c *config.Config) Option {
	return func(cfg *nodeConfig) error {
		if c == nil {
			return fmt.Errorf("config cannot be nil")
		}
		cfg.config = c
		return nil
	}
}

// ════════════════════════════════════════════════════════════════════════════
//
//	Fx 扩展选项
//
// ════════════════════════════════════════════════════════════════════════════

// WithFxOption 添加自定义 Fx 模块
//
// 允许用户注入自定义 Fx 模块到 DEP2P 的依赖注入容器中。
// 可用于添加自定义服务、替换内置组件或扩展功能。
//
// 示例：
//
//	// 添加自定义服务
//	node, err := dep2p.New(ctx,
//	    dep2p.WithFxOption(fx.Provide(NewCustomService)),
//	)
//
//	// 添加启动钩子
//	node, err := dep2p.New(ctx,
//	    dep2p.WithFxOption(fx.Invoke(func(host pkgif.Host) {
//	        fmt.Println("Node started:", host.ID())
//	    })),
//	)
func WithFxOption(opt fx.Option) Option {
	return func(cfg *nodeConfig) error {
		if opt == nil {
			return fmt.Errorf("fx option cannot be nil")
		}
		cfg.userFxOptions = append(cfg.userFxOptions, opt)
		return nil
	}
}

// WithFxOptions 添加多个自定义 Fx 模块
//
// 批量添加多个 Fx 选项。
//
// 示例：
//
//	node, err := dep2p.New(ctx,
//	    dep2p.WithFxOptions(
//	        fx.Provide(NewServiceA),
//	        fx.Provide(NewServiceB),
//	        fx.Invoke(initServices),
//	    ),
//	)
func WithFxOptions(opts ...fx.Option) Option {
	return func(cfg *nodeConfig) error {
		for _, opt := range opts {
			if opt == nil {
				return fmt.Errorf("fx option cannot be nil")
			}
		}
		cfg.userFxOptions = append(cfg.userFxOptions, opts...)
		return nil
	}
}
