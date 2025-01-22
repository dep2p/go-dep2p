package dep2p

// 此文件包含所有默认配置选项

import (
	"crypto/rand"

	"github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/p2p/host/peerstore/pstoremem"
	rcmgr "github.com/dep2p/go-dep2p/p2p/host/resource-manager"
	"github.com/dep2p/go-dep2p/p2p/muxer/yamux"
	"github.com/dep2p/go-dep2p/p2p/net/connmgr"
	"github.com/dep2p/go-dep2p/p2p/net/swarm"
	"github.com/dep2p/go-dep2p/p2p/security/noise"
	tls "github.com/dep2p/go-dep2p/p2p/security/tls"
	quic "github.com/dep2p/go-dep2p/p2p/transport/quic"
	"github.com/dep2p/go-dep2p/p2p/transport/tcp"
	dep2pwebrtc "github.com/dep2p/go-dep2p/p2p/transport/webrtc"
	ws "github.com/dep2p/go-dep2p/p2p/transport/websocket"
	webtransport "github.com/dep2p/go-dep2p/p2p/transport/webtransport"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// DefaultSecurity 是默认的安全选项配置
// 当需要扩展而不是替换支持的传输安全协议时很有用
var DefaultSecurity = ChainOptions(
	// 配置 TLS 安全传输
	Security(tls.ID, tls.New),
	// 配置 Noise 安全传输
	Security(noise.ID, noise.New),
)

// DefaultMuxers 配置 dep2p 使用流连接多路复用器
// 当需要扩展而不是替换 dep2p 使用的多路复用器时使用此选项
var DefaultMuxers = Muxer(yamux.ID, yamux.DefaultTransport)

// DefaultTransports 是默认的 dep2p 传输配置
// 当需要扩展而不是替换 dep2p 使用的传输时使用此选项
var DefaultTransports = ChainOptions(
	// 配置 TCP 传输
	Transport(tcp.NewTCPTransport),
	// 配置 QUIC 传输
	Transport(quic.NewTransport),
	// 配置 WebSocket 传输
	Transport(ws.New),
	// 配置 WebTransport 传输
	Transport(webtransport.New),
	// 配置 WebRTC 传输
	Transport(dep2pwebrtc.New),
)

// DefaultPrivateTransports 是提供 PSK 时默认的 dep2p 传输配置
// 当需要扩展而不是替换 dep2p 使用的传输时使用此选项
var DefaultPrivateTransports = ChainOptions(
	// 配置 TCP 传输
	Transport(tcp.NewTCPTransport),
	// 配置 WebSocket 传输
	Transport(ws.New),
)

// DefaultPeerstore 配置 dep2p 使用默认的对等点存储
// 参数:
//   - cfg: *Config 配置对象
//
// 返回:
//   - error: 如果发生错误则返回错误信息
var DefaultPeerstore Option = func(cfg *Config) error {
	// 创建新的内存对等点存储
	ps, err := pstoremem.NewPeerstore()
	if err != nil {
		log.Debugf("创建内存对等点存储失败: %s", err)
		return err
	}
	return cfg.Apply(Peerstore(ps))
}

// RandomIdentity 生成随机身份(默认行为)
// 参数:
//   - cfg: *Config 配置对象
//
// 返回:
//   - error: 如果发生错误则返回错误信息
var RandomIdentity = func(cfg *Config) error {
	// 生成 Ed25519 密钥对
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return err
	}
	return cfg.Apply(Identity(priv))
}

// DefaultListenAddrs 配置 dep2p 使用默认的监听地址
// 参数:
//   - cfg: *Config 配置对象
//
// 返回:
//   - error: 如果发生错误则返回错误信息
var DefaultListenAddrs = func(cfg *Config) error {
	// 定义默认监听地址列表
	addrs := []string{
		"/ip4/0.0.0.0/tcp/0",
		"/ip4/0.0.0.0/udp/0/quic-v1",
		"/ip4/0.0.0.0/udp/0/quic-v1/webtransport",
		"/ip4/0.0.0.0/udp/0/webrtc-direct",
		"/ip6/::/tcp/0",
		"/ip6/::/udp/0/quic-v1",
		"/ip6/::/udp/0/quic-v1/webtransport",
		"/ip6/::/udp/0/webrtc-direct",
	}
	// 创建多地址切片
	listenAddrs := make([]multiaddr.Multiaddr, 0, len(addrs))
	// 将地址字符串转换为多地址格式
	for _, s := range addrs {
		addr, err := multiaddr.NewMultiaddr(s)
		if err != nil {
			return err
		}
		listenAddrs = append(listenAddrs, addr)
	}
	return cfg.Apply(ListenAddrs(listenAddrs...))
}

// DefaultEnableRelay 默认启用中继拨号和监听
// 参数:
//   - cfg: *Config 配置对象
//
// 返回:
//   - error: 如果发生错误则返回错误信息
var DefaultEnableRelay = func(cfg *Config) error {
	return cfg.Apply(EnableRelay())
}

// DefaultResourceManager 配置默认的资源管理器
// 参数:
//   - cfg: *Config 配置对象
//
// 返回:
//   - error: 如果发生错误则返回错误信息
var DefaultResourceManager = func(cfg *Config) error {
	// 默认内存限制：总内存的 1/8，最小 128MB，最大 1GB
	limits := rcmgr.DefaultLimits
	SetDefaultServiceLimits(&limits)
	// 创建新的资源管理器
	mgr, err := rcmgr.NewResourceManager(rcmgr.NewFixedLimiter(limits.AutoScale()))
	if err != nil {
		return err
	}

	return cfg.Apply(ResourceManager(mgr))
}

// DefaultConnectionManager 创建默认的连接管理器
// 参数:
//   - cfg: *Config 配置对象
//
// 返回:
//   - error: 如果发生错误则返回错误信息
var DefaultConnectionManager = func(cfg *Config) error {
	// 创建一个新的连接管理器,最小连接数为160,最大连接数为192
	mgr, err := connmgr.NewConnManager(160, 192)
	if err != nil {
		return err
	}

	// 将连接管理器应用到配置中
	return cfg.Apply(ConnectionManager(mgr))
}

// DefaultPrometheusRegisterer 配置dep2p使用默认的Prometheus注册器
// 参数:
//   - cfg: *Config 配置对象
//
// 返回:
//   - error: 如果发生错误则返回错误信息
var DefaultPrometheusRegisterer = func(cfg *Config) error {
	// 应用默认的Prometheus注册器
	return cfg.Apply(PrometheusRegisterer(prometheus.DefaultRegisterer))
}

// defaultUDPBlackHoleDetector 配置UDP黑洞检测器
// 参数:
//   - cfg: *Config 配置对象
//
// 返回:
//   - error: 如果发生错误则返回错误信息
var defaultUDPBlackHoleDetector = func(cfg *Config) error {
	// 配置UDP黑洞检测器,在100次尝试中至少需要5次成功
	// 黑洞是一个二元属性,如果UDP拨号被阻止,所有拨号都会失败
	return cfg.Apply(UDPBlackHoleSuccessCounter(&swarm.BlackHoleSuccessCounter{N: 100, MinSuccesses: 5, Name: "UDP"}))
}

// defaultIPv6BlackHoleDetector 配置IPv6黑洞检测器
// 参数:
//   - cfg: *Config 配置对象
//
// 返回:
//   - error: 如果发生错误则返回错误信息
var defaultIPv6BlackHoleDetector = func(cfg *Config) error {
	// 配置IPv6黑洞检测器,在100次尝试中至少需要5次成功
	// 黑洞是一个二元属性,如果没有IPv6连接,所有拨号都会失败
	return cfg.Apply(IPv6BlackHoleSuccessCounter(&swarm.BlackHoleSuccessCounter{N: 100, MinSuccesses: 5, Name: "IPv6"}))
}

// defaults 定义了默认选项的完整列表以及何时使用它们
// 请不要以其他方式指定默认选项
// 将所有默认选项放在这里可以更容易地跟踪默认值
var defaults = []struct {
	fallback func(cfg *Config) bool // 回退条件函数
	opt      Option                 // 选项
}{
	{
		// 当未配置传输和监听地址时使用默认监听地址
		fallback: func(cfg *Config) bool { return cfg.Transports == nil && cfg.ListenAddrs == nil },
		opt:      DefaultListenAddrs,
	},
	{
		// 当未配置传输和PSK时使用默认传输
		fallback: func(cfg *Config) bool { return cfg.Transports == nil && cfg.PSK == nil },
		opt:      DefaultTransports,
	},
	{
		// 当未配置传输但配置了PSK时使用默认私有传输
		fallback: func(cfg *Config) bool { return cfg.Transports == nil && cfg.PSK != nil },
		opt:      DefaultPrivateTransports,
	},
	{
		// 当未配置多路复用器时使用默认多路复用器
		fallback: func(cfg *Config) bool { return cfg.Muxers == nil },
		opt:      DefaultMuxers,
	},
	{
		// 当未禁用安全且未配置安全传输时使用默认安全选项
		fallback: func(cfg *Config) bool { return !cfg.Insecure && cfg.SecurityTransports == nil },
		opt:      DefaultSecurity,
	},
	{
		// 当未配置对等密钥时使用随机身份
		fallback: func(cfg *Config) bool { return cfg.PeerKey == nil },
		opt:      RandomIdentity,
	},
	{
		// 当未配置对等存储时使用默认对等存储
		fallback: func(cfg *Config) bool { return cfg.Peerstore == nil },
		opt:      DefaultPeerstore,
	},
	{
		// 当未自定义中继配置时使用默认中继配置
		fallback: func(cfg *Config) bool { return !cfg.RelayCustom },
		opt:      DefaultEnableRelay,
	},
	{
		// 当未配置资源管理器时使用默认资源管理器
		fallback: func(cfg *Config) bool { return cfg.ResourceManager == nil },
		opt:      DefaultResourceManager,
	},
	{
		// 当未配置连接管理器时使用默认连接管理器
		fallback: func(cfg *Config) bool { return cfg.ConnManager == nil },
		opt:      DefaultConnectionManager,
	},
	{
		// 当未禁用指标且未配置Prometheus注册器时使用默认Prometheus注册器
		fallback: func(cfg *Config) bool { return !cfg.DisableMetrics && cfg.PrometheusRegisterer == nil },
		opt:      DefaultPrometheusRegisterer,
	},
	{
		// 当未自定义UDP黑洞成功计数器且未配置时使用默认UDP黑洞检测器
		fallback: func(cfg *Config) bool {
			return !cfg.CustomUDPBlackHoleSuccessCounter && cfg.UDPBlackHoleSuccessCounter == nil
		},
		opt: defaultUDPBlackHoleDetector,
	},
	{
		// 当未自定义IPv6黑洞成功计数器且未配置时使用默认IPv6黑洞检测器
		fallback: func(cfg *Config) bool {
			return !cfg.CustomIPv6BlackHoleSuccessCounter && cfg.IPv6BlackHoleSuccessCounter == nil
		},
		opt: defaultIPv6BlackHoleDetector,
	},
}

// Defaults 配置dep2p使用默认选项
// 可以与其他选项组合以扩展默认选项
// 参数:
//   - cfg: *Config 配置对象
//
// 返回:
//   - error: 如果发生错误则返回错误信息
var Defaults Option = func(cfg *Config) error {
	// 遍历所有默认选项并应用
	for _, def := range defaults {
		if err := cfg.Apply(def.opt); err != nil {
			return err
		}
	}
	return nil
}

// FallbackDefaults 仅在未应用其他相关选项时才应用默认选项
// 将被附加到传递给New的选项中
// 参数:
//   - cfg: *Config 配置对象
//
// 返回:
//   - error: 如果发生错误则返回错误信息
var FallbackDefaults Option = func(cfg *Config) error {
	// 遍历所有默认选项
	for _, def := range defaults {
		// 检查是否需要应用回退选项
		if !def.fallback(cfg) {
			continue
		}
		// 应用回退选项
		if err := cfg.Apply(def.opt); err != nil {
			return err
		}
	}
	return nil
}
