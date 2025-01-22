package config

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"slices"
	"time"

	"github.com/dep2p/go-dep2p/core/connmgr"
	"github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/event"
	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/metrics"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/peerstore"
	"github.com/dep2p/go-dep2p/core/pnet"
	"github.com/dep2p/go-dep2p/core/protocol"
	"github.com/dep2p/go-dep2p/core/routing"
	"github.com/dep2p/go-dep2p/core/sec"
	"github.com/dep2p/go-dep2p/core/sec/insecure"
	"github.com/dep2p/go-dep2p/core/transport"
	"github.com/dep2p/go-dep2p/p2p/host/autonat"
	"github.com/dep2p/go-dep2p/p2p/host/autorelay"
	bhost "github.com/dep2p/go-dep2p/p2p/host/basic"
	blankhost "github.com/dep2p/go-dep2p/p2p/host/blank"
	"github.com/dep2p/go-dep2p/p2p/host/eventbus"
	"github.com/dep2p/go-dep2p/p2p/host/peerstore/pstoremem"
	rcmgr "github.com/dep2p/go-dep2p/p2p/host/resource-manager"
	routed "github.com/dep2p/go-dep2p/p2p/host/routed"
	"github.com/dep2p/go-dep2p/p2p/net/swarm"
	tptu "github.com/dep2p/go-dep2p/p2p/net/upgrader"
	circuitv2 "github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/client"
	relayv2 "github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/relay"
	"github.com/dep2p/go-dep2p/p2p/protocol/holepunch"
	"github.com/dep2p/go-dep2p/p2p/protocol/identify"
	"github.com/dep2p/go-dep2p/p2p/transport/quicreuse"
	"github.com/dep2p/go-dep2p/p2p/transport/tcpreuse"
	dep2pwebrtc "github.com/dep2p/go-dep2p/p2p/transport/webrtc"
	"github.com/prometheus/client_golang/prometheus"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	"github.com/quic-go/quic-go"
	"go.uber.org/fx"
	"go.uber.org/fx/fxevent"
)

// AddrsFactory 是一个函数类型，用于处理节点的地址工厂
// 参数：
//   - 接收一组正在监听的多地址
//
// 返回值：
//   - 返回一组应该向网络广播的多地址
type AddrsFactory = bhost.AddrsFactory

// NATManagerC 是一个函数类型，用于构造 NAT 管理器
// 参数：
//   - network.Network: 网络对象，用于管理 NAT 穿透
//
// 返回值：
//   - bhost.NATManager: 返回一个 NAT 管理器实例
type NATManagerC func(network.Network) bhost.NATManager

// RoutingC 是一个函数类型，用于构造对等路由
// 参数：
//   - host.Host: 主机对象，用于路由功能
//
// 返回值：
//   - routing.PeerRouting: 对等路由实例
//   - error: 如果发生错误，返回错误信息
type RoutingC func(host.Host) (routing.PeerRouting, error)

// AutoNATConfig 定义了 dep2p host 的 AutoNAT 配置
type AutoNATConfig struct {
	ForceReachability   *network.Reachability // 强制设置节点的可达性状态
	EnableService       bool                  // 是否启用 AutoNAT 服务
	ThrottleGlobalLimit int                   // 全局限流阈值
	ThrottlePeerLimit   int                   // 每个对等点的限流阈值
	ThrottleInterval    time.Duration         // 限流检查的时间间隔
}

// Security 定义了安全传输的配置
type Security struct {
	ID          protocol.ID // 安全传输协议的标识符
	Constructor interface{} // 用于构造安全传输的构造器
}

// Config 描述了 dep2p 节点的完整配置
type Config struct {
	UserAgent       string // 节点的用户代理标识符
	ProtocolVersion string // 节点使用的协议版本

	PeerKey crypto.PrivKey // 节点的私钥

	// 传输层配置
	QUICReuse          []fx.Option        // QUIC 连接复用的选项
	Transports         []fx.Option        // 传输层的配置选项
	Muxers             []tptu.StreamMuxer // 流多路复用器列表
	SecurityTransports []Security         // 安全传输配置列表
	Insecure           bool               // 是否允许不安全连接
	PSK                pnet.PSK           // 预共享密钥

	// 连接相关配置
	DialTimeout time.Duration // 连接超时时间

	// 中继相关配置
	RelayCustom        bool             // 是否使用自定义中继
	Relay              bool             // 是否启用中继功能
	EnableRelayService bool             // 是否启用中继服务
	RelayServiceOpts   []relayv2.Option // 中继服务的配置选项

	// 网络地址配置
	ListenAddrs     []ma.Multiaddr          // 监听地址列表
	AddrsFactory    bhost.AddrsFactory      // 地址工厂
	ConnectionGater connmgr.ConnectionGater // 连接过滤器

	// 资源管理配置
	ConnManager     connmgr.ConnManager     // 连接管理器
	ResourceManager network.ResourceManager // 资源管理器

	// NAT 和对等存储配置
	NATManager NATManagerC         // NAT 管理器构造函数
	Peerstore  peerstore.Peerstore // 对等点存储
	Reporter   metrics.Reporter    // 指标报告器

	// 网络功能配置
	MultiaddrResolver network.MultiaddrDNSResolver // 多地址 DNS 解析器
	DisablePing       bool                         // 是否禁用 ping
	Routing           RoutingC                     // 路由构造函数

	// 自动中继和 NAT 配置
	EnableAutoRelay bool               // 是否启用自动中继
	AutoRelayOpts   []autorelay.Option // 自动中继选项
	AutoNATConfig                      // AutoNAT 配置

	// 打洞功能配置
	EnableHolePunching  bool               // 是否启用打洞功能
	HolePunchingOptions []holepunch.Option // 打洞选项

	// 监控和指标配置
	DisableMetrics       bool                  // 是否禁用指标收集
	PrometheusRegisterer prometheus.Registerer // Prometheus 注册器

	// 网络优化配置
	DialRanker network.DialRanker // 拨号优先级排序器
	SwarmOpts  []swarm.Option     // Swarm 配置选项

	// 功能开关配置
	DisableIdentifyAddressDiscovery bool // 是否禁用地址发现
	EnableAutoNATv2                 bool // 是否启用 AutoNAT v2

	// 黑洞检测配置
	UDPBlackHoleSuccessCounter        *swarm.BlackHoleSuccessCounter // UDP 黑洞检测成功计数器
	CustomUDPBlackHoleSuccessCounter  bool                           // 是否使用自定义 UDP 黑洞检测计数器
	IPv6BlackHoleSuccessCounter       *swarm.BlackHoleSuccessCounter // IPv6 黑洞检测成功计数器
	CustomIPv6BlackHoleSuccessCounter bool                           // 是否使用自定义 IPv6 黑洞检测计数器

	// 用户自定义配置
	UserFxOptions []fx.Option // 用户自定义的 fx 选项

	// TCP 监听器共享配置
	ShareTCPListener bool // 是否共享 TCP 监听器
}

// makeSwarm 创建并返回一个新的 Swarm 实例
// 参数:
//   - eventBus: 事件总线,用于处理事件
//   - enableMetrics: 是否启用指标收集
//
// 返回:
//   - *swarm.Swarm: 创建的 Swarm 实例
//   - error: 错误信息
func (cfg *Config) makeSwarm(eventBus event.Bus, enableMetrics bool) (*swarm.Swarm, error) {
	// 检查是否配置了对等点存储
	if cfg.Peerstore == nil {
		log.Debugf("未指定 peerstore")
		return nil, fmt.Errorf("未指定 peerstore")
	}

	// 尽早检查这一点。防止我们在没有验证这一点的情况下就开始。
	// 检查是否配置了私有网络保护器
	if pnet.ForcePrivateNetwork && len(cfg.PSK) == 0 {
		log.Debugf("试图创建一个没有私有网络保护器的 dep2p 节点,但环境强制使用私有网络")
		// 注意:这也在升级器本身中检查,所以即使你不使用 dep2p 构造函数,它也会被强制执行。
		return nil, pnet.ErrNotInPrivateNetwork
	}

	// 检查是否配置了对等密钥
	if cfg.PeerKey == nil {
		log.Debugf("未指定对等密钥")
		return nil, fmt.Errorf("未指定对等密钥")
	}

	// 从公钥生成对等 ID
	pid, err := peer.IDFromPublicKey(cfg.PeerKey.GetPublic())
	if err != nil {
		log.Debugf("从公钥生成对等 ID失败: %v", err)
		return nil, err
	}

	// 将私钥添加到对等点存储
	if err := cfg.Peerstore.AddPrivKey(pid, cfg.PeerKey); err != nil {
		log.Debugf("将私钥添加到对等点存储失败: %v", err)
		return nil, err
	}
	// 将公钥添加到对等点存储
	if err := cfg.Peerstore.AddPubKey(pid, cfg.PeerKey.GetPublic()); err != nil {
		log.Debugf("将公钥添加到对等点存储失败: %v", err)
		return nil, err
	}

	// 配置 Swarm 选项
	opts := append(cfg.SwarmOpts,
		swarm.WithUDPBlackHoleSuccessCounter(cfg.UDPBlackHoleSuccessCounter),
		swarm.WithIPv6BlackHoleSuccessCounter(cfg.IPv6BlackHoleSuccessCounter),
	)
	// 添加指标报告器
	if cfg.Reporter != nil {
		opts = append(opts, swarm.WithMetrics(cfg.Reporter))
	}
	// 添加连接过滤器
	if cfg.ConnectionGater != nil {
		opts = append(opts, swarm.WithConnectionGater(cfg.ConnectionGater))
	}
	// 设置拨号超时
	if cfg.DialTimeout != 0 {
		opts = append(opts, swarm.WithDialTimeout(cfg.DialTimeout))
	}
	// 添加资源管理器
	if cfg.ResourceManager != nil {
		opts = append(opts, swarm.WithResourceManager(cfg.ResourceManager))
	}
	// 添加多地址解析器
	if cfg.MultiaddrResolver != nil {
		opts = append(opts, swarm.WithMultiaddrResolver(cfg.MultiaddrResolver))
	}
	// 添加拨号优先级排序器
	if cfg.DialRanker != nil {
		opts = append(opts, swarm.WithDialRanker(cfg.DialRanker))
	}

	// 启用指标收集
	if enableMetrics {
		opts = append(opts,
			swarm.WithMetricsTracer(swarm.NewMetricsTracer(swarm.WithRegisterer(cfg.PrometheusRegisterer))))
	}
	// TODO: 使 swarm 实现可配置。
	// 创建并返回新的 Swarm 实例
	return swarm.NewSwarm(pid, cfg.Peerstore, eventBus, opts...)
}

// makeAutoNATV2Host 创建并返回一个用于 AutoNAT v2 的 dep2p 主机
// 返回:
//   - host.Host: 创建的主机实例
//   - error: 创建过程中的错误
func (cfg *Config) makeAutoNATV2Host() (host.Host, error) {
	// 生成新的 Ed25519 密钥对用于 AutoNAT
	autonatPrivKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		log.Debugf("生成 Ed25519 密钥对失败: %v", err)
		return nil, err
	}

	// 创建内存中的对等点存储
	ps, err := pstoremem.NewPeerstore()
	if err != nil {
		log.Debugf("创建内存中的对等点存储失败: %v", err)
		return nil, err
	}

	// 配置 AutoNAT 的配置对象
	autoNatCfg := Config{
		Transports:                  cfg.Transports,                  // 传输层配置
		Muxers:                      cfg.Muxers,                      // 多路复用器配置
		SecurityTransports:          cfg.SecurityTransports,          // 安全传输配置
		Insecure:                    cfg.Insecure,                    // 是否允许不安全连接
		PSK:                         cfg.PSK,                         // 预共享密钥
		ConnectionGater:             cfg.ConnectionGater,             // 连接过滤器
		Reporter:                    cfg.Reporter,                    // 指标报告器
		PeerKey:                     autonatPrivKey,                  // AutoNAT 专用密钥
		Peerstore:                   ps,                              // 对等点存储
		DialRanker:                  swarm.NoDelayDialRanker,         // 拨号优先级排序器
		UDPBlackHoleSuccessCounter:  cfg.UDPBlackHoleSuccessCounter,  // UDP 黑洞检测计数器
		IPv6BlackHoleSuccessCounter: cfg.IPv6BlackHoleSuccessCounter, // IPv6 黑洞检测计数器
		ResourceManager:             cfg.ResourceManager,             // 资源管理器
		SwarmOpts: []swarm.Option{
			// 不要为失败的 autonat 拨号更新黑洞状态
			// 配置只读黑洞检测器
			swarm.WithReadOnlyBlackHoleDetector(),
		},
	}

	// 添加传输层配置
	fxopts, err := autoNatCfg.addTransports()
	if err != nil {
		log.Debugf("添加传输层配置失败: %v", err)
		return nil, err
	}

	// 声明拨号主机变量
	var dialerHost host.Host

	// 添加依赖注入选项
	fxopts = append(fxopts,
		fx.Provide(eventbus.NewBus), // 提供事件总线
		// 提供 Swarm 实例
		fx.Provide(func(lifecycle fx.Lifecycle, b event.Bus) (*swarm.Swarm, error) {
			// 添加生命周期钩子用于清理
			lifecycle.Append(fx.Hook{
				OnStop: func(context.Context) error {
					return ps.Close()
				}})
			sw, err := autoNatCfg.makeSwarm(b, false)
			if err != nil {
				log.Debugf("创建 Swarm 实例失败: %v", err)
				return nil, err
			}
			return sw, nil
		}),
		// 提供空白主机实例
		fx.Provide(func(sw *swarm.Swarm) *blankhost.BlankHost {
			return blankhost.NewBlankHost(sw)
		}),
		// 将空白主机转换为通用主机接口
		fx.Provide(func(bh *blankhost.BlankHost) host.Host {
			return bh
		}),
		// 提供私钥
		fx.Provide(func() crypto.PrivKey { return autonatPrivKey }),
		// 提供对等点 ID
		fx.Provide(func(bh host.Host) peer.ID { return bh.ID() }),
		// 设置拨号主机
		fx.Invoke(func(bh *blankhost.BlankHost) {
			dialerHost = bh
		}),
	)

	// 创建并启动应用
	app := fx.New(fxopts...)
	if err := app.Err(); err != nil {
		log.Debugf("创建应用失败: %v", err)
		return nil, err
	}
	err = app.Start(context.Background())
	if err != nil {
		log.Debugf("启动应用失败: %v", err)
		return nil, err
	}

	// 监听 Swarm 关闭事件并停止应用
	go func() {
		<-dialerHost.Network().(*swarm.Swarm).Done()
		app.Stop(context.Background())
	}()

	return dialerHost, nil
}

// addTransports 添加传输层相关的依赖注入选项
// 参数:
//   - cfg: 配置对象
//
// 返回值:
//   - []fx.Option: 依赖注入选项列表
//   - error: 错误信息
func (cfg *Config) addTransports() ([]fx.Option, error) {
	// 初始化基础的依赖注入选项
	fxopts := []fx.Option{
		// 设置 fx 日志记录器
		fx.WithLogger(func() fxevent.Logger { return getFXLogger() }),
		// 提供传输层升级器,并标记为 security 参数
		fx.Provide(fx.Annotate(tptu.New, fx.ParamTags(`name:"security"`))),
		// 提供多路复用器配置
		fx.Supply(cfg.Muxers),
		// 提供连接网关
		fx.Provide(func() connmgr.ConnectionGater { return cfg.ConnectionGater }),
		// 提供预共享密钥
		fx.Provide(func() pnet.PSK { return cfg.PSK }),
		// 提供资源管理器
		fx.Provide(func() network.ResourceManager { return cfg.ResourceManager }),
		// 提供 TCP 连接管理器
		fx.Provide(func(gater connmgr.ConnectionGater, rcmgr network.ResourceManager) *tcpreuse.ConnMgr {
			if !cfg.ShareTCPListener {
				return nil
			}
			return tcpreuse.NewConnMgr(tcpreuse.EnvReuseportVal, gater, rcmgr)
		}),
		// 提供 WebRTC UDP 监听函数
		fx.Provide(func(cm *quicreuse.ConnManager, sw *swarm.Swarm) dep2pwebrtc.ListenUDPFn {
			// 检查是否存在指定网络和地址的 QUIC 端口
			hasQuicAddrPortFor := func(network string, laddr *net.UDPAddr) bool {
				quicAddrPorts := map[string]struct{}{}
				for _, addr := range sw.ListenAddresses() {
					if _, err := addr.ValueForProtocol(ma.P_QUIC_V1); err == nil {
						netw, addr, err := manet.DialArgs(addr)
						if err != nil {
							log.Debugf("获取网络和地址失败: %v", err)
							return false
						}
						quicAddrPorts[netw+"_"+addr] = struct{}{}
					}
				}
				_, ok := quicAddrPorts[network+"_"+laddr.String()]
				return ok
			}

			// 返回 UDP 监听函数
			return func(network string, laddr *net.UDPAddr) (net.PacketConn, error) {
				if hasQuicAddrPortFor(network, laddr) {
					return cm.SharedNonQUICPacketConn(network, laddr)
				}
				return net.ListenUDP(network, laddr)
			}
		}),
	}

	// 添加用户配置的传输选项
	fxopts = append(fxopts, cfg.Transports...)

	// 根据是否启用不安全模式添加相应的安全传输选项
	if cfg.Insecure {
		// 添加不安全传输选项
		fxopts = append(fxopts,
			fx.Provide(
				fx.Annotate(
					func(id peer.ID, priv crypto.PrivKey) []sec.SecureTransport {
						return []sec.SecureTransport{insecure.NewWithIdentity(insecure.ID, id, priv)}
					},
					fx.ResultTags(`name:"security"`),
				),
			),
		)
	} else {
		// 添加安全传输选项
		// fx 组是无序的,但我们需要保持安全传输的顺序。
		// 首先,我们构造需要的安全传输,并将它们保存到一个名为 security_unordered 的组中。
		for _, s := range cfg.SecurityTransports {
			fxName := fmt.Sprintf(`name:"security_%s"`, s.ID)
			fxopts = append(fxopts, fx.Supply(fx.Annotate(s.ID, fx.ResultTags(fxName))))
			fxopts = append(fxopts,
				fx.Provide(fx.Annotate(
					s.Constructor,
					fx.ParamTags(fxName),
					fx.As(new(sec.SecureTransport)),
					fx.ResultTags(`group:"security_unordered"`),
				)),
			)
		}
		// 然后按用户配置的顺序排序安全传输
		fxopts = append(fxopts, fx.Provide(
			fx.Annotate(
				func(secs []sec.SecureTransport) ([]sec.SecureTransport, error) {
					if len(secs) != len(cfg.SecurityTransports) {
						log.Debugf("安全传输长度不一致")
						return nil, errors.New("安全传输长度不一致")
					}
					t := make([]sec.SecureTransport, 0, len(secs))
					for _, s := range cfg.SecurityTransports {
						for _, st := range secs {
							if s.ID != st.ID() {
								continue
							}
							t = append(t, st)
						}
					}
					return t, nil
				},
				fx.ParamTags(`group:"security_unordered"`),
				fx.ResultTags(`name:"security"`),
			)))
	}

	// 添加 QUIC 相关选项
	fxopts = append(fxopts, fx.Provide(PrivKeyToStatelessResetKey))
	fxopts = append(fxopts, fx.Provide(PrivKeyToTokenGeneratorKey))
	if cfg.QUICReuse != nil {
		fxopts = append(fxopts, cfg.QUICReuse...)
	} else {
		// 提供默认的 QUIC 连接管理器
		fxopts = append(fxopts,
			fx.Provide(func(key quic.StatelessResetKey, tokenGenerator quic.TokenGeneratorKey, lifecycle fx.Lifecycle) (*quicreuse.ConnManager, error) {
				var opts []quicreuse.Option
				if !cfg.DisableMetrics {
					opts = append(opts, quicreuse.EnableMetrics(cfg.PrometheusRegisterer))
				}
				cm, err := quicreuse.NewConnManager(key, tokenGenerator, opts...)
				if err != nil {
					log.Debugf("创建 QUIC 连接管理器失败: %v", err)
					return nil, err
				}
				lifecycle.Append(fx.StopHook(cm.Close))
				return cm, nil
			}),
		)
	}

	// 添加传输层到 Swarm
	fxopts = append(fxopts, fx.Invoke(
		fx.Annotate(
			func(swrm *swarm.Swarm, tpts []transport.Transport) error {
				for _, t := range tpts {
					if err := swrm.AddTransport(t); err != nil {
						log.Debugf("添加传输层到 Swarm 失败: %v", err)
						return err
					}
				}
				return nil
			},
			fx.ParamTags("", `group:"transport"`),
		)),
	)

	// 如果启用中继,添加中继传输
	if cfg.Relay {
		fxopts = append(fxopts, fx.Invoke(circuitv2.AddTransport))
	}

	return fxopts, nil
}

// newBasicHost 创建一个新的基础 dep2p 主机
// 参数:
//   - swrm: swarm 网络层实例
//   - eventBus: 事件总线实例
//
// 返回:
//   - *bhost.BasicHost: 创建的基础主机实例
//   - error: 创建过程中的错误
func (cfg *Config) newBasicHost(swrm *swarm.Swarm, eventBus event.Bus) (*bhost.BasicHost, error) {
	// 声明 AutoNAT v2 拨号器变量
	var autonatv2Dialer host.Host

	// 如果启用了 AutoNAT v2,创建相应的主机
	if cfg.EnableAutoNATv2 {
		ah, err := cfg.makeAutoNATV2Host()
		if err != nil {
			log.Debugf("创建 AutoNAT v2 主机失败: %v", err)
			return nil, err
		}
		autonatv2Dialer = ah
	}

	// 使用提供的选项创建新的基础主机
	h, err := bhost.NewHost(swrm, &bhost.HostOpts{
		EventBus:                        eventBus,                            // 事件总线
		ConnManager:                     cfg.ConnManager,                     // 连接管理器
		AddrsFactory:                    cfg.AddrsFactory,                    // 地址工厂
		NATManager:                      cfg.NATManager,                      // NAT 管理器
		EnablePing:                      !cfg.DisablePing,                    // 是否启用 ping
		UserAgent:                       cfg.UserAgent,                       // 用户代理
		ProtocolVersion:                 cfg.ProtocolVersion,                 // 协议版本
		EnableHolePunching:              cfg.EnableHolePunching,              // 是否启用打洞
		HolePunchingOptions:             cfg.HolePunchingOptions,             // 打洞选项
		EnableRelayService:              cfg.EnableRelayService,              // 是否启用中继服务
		RelayServiceOpts:                cfg.RelayServiceOpts,                // 中继服务选项
		EnableMetrics:                   !cfg.DisableMetrics,                 // 是否启用指标
		PrometheusRegisterer:            cfg.PrometheusRegisterer,            // Prometheus 注册器
		DisableIdentifyAddressDiscovery: cfg.DisableIdentifyAddressDiscovery, // 是否禁用地址发现
		EnableAutoNATv2:                 cfg.EnableAutoNATv2,                 // 是否启用 AutoNAT v2
		AutoNATv2Dialer:                 autonatv2Dialer,                     // AutoNAT v2 拨号器
	})

	// 检查创建过程中是否有错误
	if err != nil {
		log.Debugf("创建基础主机失败: %v", err)
		return nil, err
	}

	// 返回创建的主机实例
	return h, nil
}

// NewNode 从配置构造一个新的 dep2p Host
//
// 参数:
//   - 无
//
// 返回:
//   - host.Host: 创建的 dep2p 主机实例
//   - error: 创建过程中的错误
//
// 注意: 此函数会消耗配置,不要重复使用它
func (cfg *Config) NewNode() (host.Host, error) {
	// 检查自动中继配置是否有效
	if cfg.EnableAutoRelay && !cfg.Relay {
		log.Debugf("无法启用自动中继;中继未启用")
		return nil, fmt.Errorf("无法启用自动中继;中继未启用")
	}
	// 如果可能,检查资源管理器连接限制是否高于连接管理器中设置的限制。
	// 检查资源管理器连接限制
	if l, ok := cfg.ResourceManager.(connmgr.GetConnLimiter); ok {
		err := cfg.ConnManager.CheckLimit(l)
		if err != nil {
			log.Warnf("rcmgr 限制与 connmgr 限制冲突: %v", err)
		}
	}

	// 注册指标
	if !cfg.DisableMetrics {
		rcmgr.MustRegisterWith(cfg.PrometheusRegisterer)
	}

	// 构建 fx 选项列表
	fxopts := []fx.Option{
		// 提供事件总线
		fx.Provide(func() event.Bus {
			return eventbus.NewBus(eventbus.WithMetricsTracer(eventbus.NewMetricsTracer(eventbus.WithRegisterer(cfg.PrometheusRegisterer))))
		}),
		// 提供私钥
		fx.Provide(func() crypto.PrivKey {
			return cfg.PeerKey
		}),
		// 提供 swarm 网络层
		// 确保 swarm 构造函数依赖于 quicreuse.ConnManager。
		// 这样,ConnManager 将在 swarm 之前启动,更重要的是,swarm 将在 ConnManager 之前停止。
		fx.Provide(func(eventBus event.Bus, _ *quicreuse.ConnManager, lifecycle fx.Lifecycle) (*swarm.Swarm, error) {
			sw, err := cfg.makeSwarm(eventBus, !cfg.DisableMetrics)
			if err != nil {
				log.Debugf("创建 Swarm 实例失败: %v", err)
				return nil, err
			}
			lifecycle.Append(fx.Hook{
				OnStart: func(context.Context) error {
					// TODO: 此方法在监听一个地址成功时就成功。
					// 如果监听*任何*地址失败,我们可能应该失败。
					return sw.Listen(cfg.ListenAddrs...)
				},
				OnStop: func(context.Context) error {
					return sw.Close()
				},
			})
			return sw, nil
		}),
		// 提供基础主机
		fx.Provide(cfg.newBasicHost),
		// 提供身份服务
		fx.Provide(func(bh *bhost.BasicHost) identify.IDService {
			return bh.IDService()
		}),
		// 提供主机接口
		fx.Provide(func(bh *bhost.BasicHost) host.Host {
			return bh
		}),
		// 提供节点 ID
		fx.Provide(func(h *swarm.Swarm) peer.ID { return h.LocalPeer() }),
	}

	// 添加传输层选项
	transportOpts, err := cfg.addTransports()
	if err != nil {
		log.Debugf("添加传输层选项失败: %v", err)
		return nil, err
	}
	fxopts = append(fxopts, transportOpts...)

	// 配置路由和自动中继
	if cfg.Routing != nil {
		fxopts = append(fxopts,
			fx.Provide(cfg.Routing),
			fx.Provide(func(h host.Host, router routing.PeerRouting) *routed.RoutedHost {
				return routed.Wrap(h, router)
			}),
		)
	}

	// 配置自动中继
	fxopts = append(fxopts,
		fx.Invoke(func(h *bhost.BasicHost, lifecycle fx.Lifecycle) error {
			if cfg.EnableAutoRelay {
				if !cfg.DisableMetrics {
					mt := autorelay.WithMetricsTracer(
						autorelay.NewMetricsTracer(autorelay.WithRegisterer(cfg.PrometheusRegisterer)))
					mtOpts := []autorelay.Option{mt}
					cfg.AutoRelayOpts = append(mtOpts, cfg.AutoRelayOpts...)
				}

				ar, err := autorelay.NewAutoRelay(h, cfg.AutoRelayOpts...)
				if err != nil {
					log.Debugf("创建自动中继失败: %v", err)
					return err
				}
				lifecycle.Append(fx.StartStopHook(ar.Start, ar.Close))
				return nil
			}
			return nil
		}),
	)

	// 保存基础主机引用
	var bh *bhost.BasicHost
	fxopts = append(fxopts, fx.Invoke(func(bho *bhost.BasicHost) { bh = bho }))
	fxopts = append(fxopts, fx.Invoke(func(h *bhost.BasicHost, lifecycle fx.Lifecycle) {
		lifecycle.Append(fx.StartHook(h.Start))
	}))

	// 保存路由主机引用
	var rh *routed.RoutedHost
	if cfg.Routing != nil {
		fxopts = append(fxopts, fx.Invoke(func(bho *routed.RoutedHost) { rh = bho }))
	}

	// 添加用户自定义选项
	fxopts = append(fxopts, cfg.UserFxOptions...)

	// 创建并启动应用
	app := fx.New(fxopts...)
	if err := app.Start(context.Background()); err != nil {
		log.Debugf("启动应用失败: %v", err)
		return nil, err
	}

	// 添加 AutoNAT 支持
	if err := cfg.addAutoNAT(bh); err != nil {
		log.Debugf("添加 AutoNAT 支持失败: %v", err)
		app.Stop(context.Background())
		if cfg.Routing != nil {
			rh.Close()
		} else {
			bh.Close()
		}
		return nil, err
	}

	// 返回适当类型的主机
	if cfg.Routing != nil {
		return &closableRoutedHost{App: app, RoutedHost: rh}, nil
	}
	return &closableBasicHost{App: app, BasicHost: bh}, nil
}

// addAutoNAT 为主机添加 AutoNAT 支持
// 参数:
//   - h: 基础主机实例
//
// 返回:
//   - error: 如果出现错误则返回,否则返回 nil
func (cfg *Config) addAutoNAT(h *bhost.BasicHost) error {
	// 定义地址过滤函数,仅返回公共地址
	addrFunc := func() []ma.Multiaddr {
		return slices.DeleteFunc(h.AllAddrs(), func(a ma.Multiaddr) bool { return !manet.IsPublicAddr(a) })
	}

	// 如果配置了地址工厂,使用工厂生成的地址进行过滤
	if cfg.AddrsFactory != nil {
		addrFunc = func() []ma.Multiaddr {
			return slices.DeleteFunc(
				slices.Clone(cfg.AddrsFactory(h.AllAddrs())),
				func(a ma.Multiaddr) bool { return !manet.IsPublicAddr(a) })
		}
	}

	// 初始化 AutoNAT 选项
	autonatOpts := []autonat.Option{
		autonat.UsingAddresses(addrFunc),
	}

	// 如果启用了指标收集,添加指标追踪器
	if !cfg.DisableMetrics {
		autonatOpts = append(autonatOpts, autonat.WithMetricsTracer(
			autonat.NewMetricsTracer(autonat.WithRegisterer(cfg.PrometheusRegisterer)),
		))
	}

	// 配置节流参数
	if cfg.AutoNATConfig.ThrottleInterval != 0 {
		autonatOpts = append(autonatOpts,
			autonat.WithThrottling(cfg.AutoNATConfig.ThrottleGlobalLimit, cfg.AutoNATConfig.ThrottleInterval),
			autonat.WithPeerThrottling(cfg.AutoNATConfig.ThrottlePeerLimit))
	}

	// 如果启用了 AutoNAT 服务
	if cfg.AutoNATConfig.EnableService {
		// 生成服务专用的 Ed25519 密钥对
		autonatPrivKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			log.Debugf("生成服务专用的 Ed25519 密钥对失败: %v", err)
			return err
		}

		// 创建内存对等存储
		ps, err := pstoremem.NewPeerstore()
		if err != nil {
			log.Debugf("创建内存对等存储失败: %v", err)
			return err
		}

		// 创建 AutoNAT 服务配置
		// 提取我们*实际*关心的配置部分。
		// 具体来说,不要设置诸如监听器、标识等内容。
		autoNatCfg := Config{
			Transports:         cfg.Transports,
			Muxers:             cfg.Muxers,
			SecurityTransports: cfg.SecurityTransports,
			Insecure:           cfg.Insecure,
			PSK:                cfg.PSK,
			ConnectionGater:    cfg.ConnectionGater,
			Reporter:           cfg.Reporter,
			PeerKey:            autonatPrivKey,
			Peerstore:          ps,
			DialRanker:         swarm.NoDelayDialRanker,
			ResourceManager:    cfg.ResourceManager,
			SwarmOpts: []swarm.Option{
				swarm.WithUDPBlackHoleSuccessCounter(nil),
				swarm.WithIPv6BlackHoleSuccessCounter(nil),
			},
		}

		// 添加传输层配置
		fxopts, err := autoNatCfg.addTransports()
		if err != nil {
			log.Debugf("添加传输层配置失败: %v", err)
			return err
		}
		var dialer *swarm.Swarm

		// 添加依赖注入配置
		fxopts = append(fxopts,
			fx.Provide(eventbus.NewBus),
			fx.Provide(func(lifecycle fx.Lifecycle, b event.Bus) (*swarm.Swarm, error) {
				// 添加清理钩子
				lifecycle.Append(fx.Hook{
					OnStop: func(context.Context) error {
						return ps.Close()
					}})
				var err error
				dialer, err = autoNatCfg.makeSwarm(b, false)
				return dialer, err

			}),
			fx.Provide(func(s *swarm.Swarm) peer.ID { return s.LocalPeer() }),
			fx.Provide(func() crypto.PrivKey { return autonatPrivKey }),
		)

		// 创建并启动应用
		app := fx.New(fxopts...)
		if err := app.Err(); err != nil {
			log.Debugf("创建应用失败: %v", err)
			return err
		}
		err = app.Start(context.Background())
		if err != nil {
			log.Debugf("启动应用失败: %v", err)
			return err
		}

		// 监听 swarm 关闭事件
		go func() {
			<-dialer.Done() // 等待 AutoNAT swarm 关闭,然后我们现在可以清理了
			app.Stop(context.Background())
		}()
		autonatOpts = append(autonatOpts, autonat.EnableService(dialer))
	}

	// 设置强制可达性(如果配置)
	if cfg.AutoNATConfig.ForceReachability != nil {
		autonatOpts = append(autonatOpts, autonat.WithReachability(*cfg.AutoNATConfig.ForceReachability))
	}

	// 创建 AutoNAT 实例
	autonat, err := autonat.New(h, autonatOpts...)
	if err != nil {
		log.Debugf("autonat 初始化失败: %v", err)
		return err
	}

	// 将 AutoNAT 实例设置到主机
	h.SetAutoNat(autonat)
	return nil
}

// Option 是一个函数类型,用于配置 dep2p
// 参数:
//   - cfg: 配置对象指针
//
// 返回:
//   - error: 配置错误
type Option func(cfg *Config) error

// Apply 应用一组配置选项
// 参数:
//   - opts: 配置选项列表
//
// 返回:
//   - error: 应用配置时的错误
func (cfg *Config) Apply(opts ...Option) error {
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		if err := opt(cfg); err != nil {
			log.Debugf("应用配置选项失败: %v", err)
			return err
		}
	}
	return nil
}
