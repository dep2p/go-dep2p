package dep2p

// 此文件包含所有dep2p配置选项(除了默认值,默认值在defaults.go中)

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/dep2p/go-dep2p/config"
	"github.com/dep2p/go-dep2p/core/connmgr"
	"github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/metrics"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/peerstore"
	"github.com/dep2p/go-dep2p/core/pnet"
	"github.com/dep2p/go-dep2p/core/protocol"
	"github.com/dep2p/go-dep2p/core/transport"
	"github.com/dep2p/go-dep2p/p2p/host/autorelay"
	bhost "github.com/dep2p/go-dep2p/p2p/host/basic"
	"github.com/dep2p/go-dep2p/p2p/net/swarm"
	tptu "github.com/dep2p/go-dep2p/p2p/net/upgrader"
	relayv2 "github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/relay"
	"github.com/dep2p/go-dep2p/p2p/protocol/holepunch"
	"github.com/dep2p/go-dep2p/p2p/transport/quicreuse"
	"github.com/prometheus/client_golang/prometheus"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	"go.uber.org/fx"
)

// ListenAddrStrings 配置dep2p监听给定的(未解析的)地址
// 参数:
//   - s: 要监听的地址字符串切片
//
// 返回:
//   - Option: 配置函数
func ListenAddrStrings(s ...string) Option {
	return func(cfg *Config) error {
		for _, addrstr := range s {
			// 将地址字符串解析为multiaddr
			a, err := ma.NewMultiaddr(addrstr)
			if err != nil {
				log.Debugf("解析地址失败: %s", err)
				return err
			}
			// 添加到监听地址列表
			cfg.ListenAddrs = append(cfg.ListenAddrs, a)
		}
		return nil
	}
}

// ListenAddrs 配置dep2p监听给定的地址
// 参数:
//   - addrs: 要监听的multiaddr地址切片
//
// 返回:
//   - Option: 配置函数
func ListenAddrs(addrs ...ma.Multiaddr) Option {
	return func(cfg *Config) error {
		// 将地址添加到监听地址列表
		cfg.ListenAddrs = append(cfg.ListenAddrs, addrs...)
		return nil
	}
}

// Security 配置dep2p使用给定的安全传输(或传输构造函数)
// 参数:
//   - name: 协议名称
//   - constructor: 传输构造函数,可以是已构造的security.Transport或一个函数,该函数可以接受以下任意参数:
//   - 公钥
//   - 私钥
//   - Peer ID
//   - Host
//   - Network
//   - Peerstore
//
// 返回:
//   - Option: 配置函数
func Security(name string, constructor interface{}) Option {
	return func(cfg *Config) error {
		// 检查是否启用了不安全模式
		if cfg.Insecure {
			log.Debugf("不能在不安全的dep2p配置中使用安全传输")
			return fmt.Errorf("不能在不安全的dep2p配置中使用安全传输")
		}
		// 添加安全传输配置
		cfg.SecurityTransports = append(cfg.SecurityTransports, config.Security{ID: protocol.ID(name), Constructor: constructor})
		return nil
	}
}

// NoSecurity 是一个完全禁用所有传输安全的选项
// 它与所有其他传输安全协议不兼容
var NoSecurity Option = func(cfg *Config) error {
	if len(cfg.SecurityTransports) > 0 {
		log.Debugf("不能在不安全的dep2p配置中使用安全传输")
		return fmt.Errorf("不能在不安全的dep2p配置中使用安全传输")
	}
	cfg.Insecure = true
	return nil
}

// Muxer 配置dep2p使用给定的流多路复用器
// 参数:
//   - name: 协议名称
//   - muxer: 多路复用器实现
//
// 返回:
//   - Option: 配置函数
func Muxer(name string, muxer network.Multiplexer) Option {
	return func(cfg *Config) error {
		cfg.Muxers = append(cfg.Muxers, tptu.StreamMuxer{Muxer: muxer, ID: protocol.ID(name)})
		return nil
	}
}

// QUICReuse 配置QUIC传输的地址重用
// 参数:
//   - constructor: QUIC构造函数
//   - opts: QUIC选项
//
// 返回:
//   - Option: 配置函数
func QUICReuse(constructor interface{}, opts ...quicreuse.Option) Option {
	return func(cfg *Config) error {
		tag := `group:"quicreuseopts"`
		typ := reflect.ValueOf(constructor).Type()
		numParams := typ.NumIn()
		isVariadic := typ.IsVariadic()

		// 检查构造函数是否支持选项
		if !isVariadic && len(opts) > 0 {
			log.Debugf("QUIC构造函数不接受任何选项")
			return errors.New("QUIC构造函数不接受任何选项")
		}

		var params []string
		if isVariadic && len(opts) > 0 {
			// 如果有选项,应用标签
			// 由于选项是可变参数,它们必须是构造函数的最后一个参数
			params = make([]string, numParams)
			params[len(params)-1] = tag
		}

		// 添加QUIC重用配置
		cfg.QUICReuse = append(cfg.QUICReuse, fx.Provide(fx.Annotate(constructor, fx.ParamTags(params...))))
		for _, opt := range opts {
			cfg.QUICReuse = append(cfg.QUICReuse, fx.Supply(fx.Annotate(opt, fx.ResultTags(tag))))
		}
		return nil
	}
}

// Transport 配置dep2p使用给定的传输(或传输构造函数)
// 参数:
//   - constructor: 传输构造函数,可以是已构造的transport.Transport或一个函数,该函数可以接受以下任意参数:
//   - Transport Upgrader (*tptu.Upgrader)
//   - Host
//   - Stream muxer (muxer.Transport)
//   - Security transport (security.Transport)
//   - Private network protector (pnet.Protector)
//   - Peer ID
//   - Private Key
//   - Public Key
//   - Address filter (filter.Filter)
//   - Peerstore
//   - opts: 传输选项
//
// 返回:
//   - Option: 配置函数
func Transport(constructor interface{}, opts ...interface{}) Option {
	return func(cfg *Config) error {
		// 生成随机标识符,用于fx关联构造函数和选项
		b := make([]byte, 8)
		rand.Read(b)
		id := binary.BigEndian.Uint64(b)

		tag := fmt.Sprintf(`group:"transportopt_%d"`, id)

		typ := reflect.ValueOf(constructor).Type()
		numParams := typ.NumIn()
		isVariadic := typ.IsVariadic()

		// 检查构造函数是否支持选项
		if !isVariadic && len(opts) > 0 {
			log.Debugf("传输构造函数不接受任何选项")
			return errors.New("传输构造函数不接受任何选项")
		}
		if isVariadic && numParams >= 1 {
			paramType := typ.In(numParams - 1).Elem()
			for _, opt := range opts {
				if typ := reflect.TypeOf(opt); !typ.AssignableTo(paramType) {
					log.Debugf("类型为 %s 的传输选项不能赋值给 %s", typ, paramType)
					return fmt.Errorf("类型为 %s 的传输选项不能赋值给 %s", typ, paramType)
				}
			}
		}

		var params []string
		if isVariadic && len(opts) > 0 {
			// 如果有传输选项,应用标签
			// 由于选项是可变参数,它们必须是构造函数的最后一个参数
			params = make([]string, numParams)
			params[len(params)-1] = tag
		}

		// 添加传输配置
		cfg.Transports = append(cfg.Transports, fx.Provide(
			fx.Annotate(
				constructor,
				fx.ParamTags(params...),
				fx.As(new(transport.Transport)),
				fx.ResultTags(`group:"transport"`),
			),
		))
		for _, opt := range opts {
			cfg.Transports = append(cfg.Transports, fx.Supply(
				fx.Annotate(
					opt,
					fx.ResultTags(tag),
				),
			))
		}
		return nil
	}
}

// Peerstore 配置dep2p使用给定的peerstore
// 参数:
//   - ps: peerstore实现
//
// 返回:
//   - Option: 配置函数
func Peerstore(ps peerstore.Peerstore) Option {
	return func(cfg *Config) error {
		if cfg.Peerstore != nil {
			log.Debugf("不能指定多个peerstore选项")
			return fmt.Errorf("不能指定多个peerstore选项")
		}

		cfg.Peerstore = ps
		return nil
	}
}

// PrivateNetwork 配置dep2p使用给定的私有网络保护器
// 参数:
//   - psk: 预共享密钥
//
// 返回:
//   - Option: 配置函数
func PrivateNetwork(psk pnet.PSK) Option {
	return func(cfg *Config) error {
		if cfg.PSK != nil {
			log.Debugf("不能指定多个私有网络选项")
			return fmt.Errorf("不能指定多个私有网络选项")
		}

		cfg.PSK = psk
		return nil
	}
}

// BandwidthReporter 配置dep2p使用给定的带宽报告器
// 参数:
//   - rep: 带宽报告器实现
//
// 返回:
//   - Option: 配置函数
func BandwidthReporter(rep metrics.Reporter) Option {
	return func(cfg *Config) error {
		if cfg.Reporter != nil {
			log.Debugf("不能指定多个带宽报告器选项")
			return fmt.Errorf("不能指定多个带宽报告器选项")
		}

		cfg.Reporter = rep
		return nil
	}
}

// Identity 配置dep2p使用给定的私钥来标识自己
// 参数:
//   - sk: 私钥
//
// 返回:
//   - Option: 配置函数
func Identity(sk crypto.PrivKey) Option {
	return func(cfg *Config) error {
		if cfg.PeerKey != nil {
			log.Debugf("不能指定多个身份")
			return fmt.Errorf("不能指定多个身份")
		}

		cfg.PeerKey = sk
		return nil
	}
}

// ConnectionManager 配置dep2p使用给定的连接管理器
// 当前"标准"连接管理器位于github.com/dep2p/go-dep2p-connmgr
// 参数:
//   - connman: 连接管理器实现
//
// 返回:
//   - Option: 配置函数
func ConnectionManager(connman connmgr.ConnManager) Option {
	return func(cfg *Config) error {
		if cfg.ConnManager != nil {
			log.Debugf("不能指定多个连接管理器")
			return fmt.Errorf("不能指定多个连接管理器")
		}
		cfg.ConnManager = connman
		return nil
	}
}

// AddrsFactory 配置dep2p使用给定的地址工厂
// 参数:
//   - factory: 地址工厂实现
//
// 返回:
//   - Option: 配置函数
func AddrsFactory(factory config.AddrsFactory) Option {
	return func(cfg *Config) error {
		if cfg.AddrsFactory != nil {
			log.Debugf("不能指定多个地址工厂")
			return fmt.Errorf("不能指定多个地址工厂")
		}
		cfg.AddrsFactory = factory
		return nil
	}
}

// EnableRelay 配置dep2p启用中继传输
// 此选项仅配置dep2p接受来自中继的入站连接,并在远程对等方请求时通过中继进行出站连接
// 此选项支持circuit v1和v2连接
// (默认:启用)
// 返回:
//   - Option: 配置函数
func EnableRelay() Option {
	return func(cfg *Config) error {
		cfg.RelayCustom = true
		cfg.Relay = true
		return nil
	}
}

// DisableRelay 配置dep2p禁用中继传输
// 返回:
//   - Option: 配置函数
func DisableRelay() Option {
	return func(cfg *Config) error {
		cfg.RelayCustom = true
		cfg.Relay = false
		return nil
	}
}

// EnableRelayService 配置dep2p运行circuit v2中继,如果我们检测到我们是公开可访问的
// 参数:
//   - opts: 中继选项
//
// 返回:
//   - Option: 配置函数
func EnableRelayService(opts ...relayv2.Option) Option {
	return func(cfg *Config) error {
		cfg.EnableRelayService = true
		cfg.RelayServiceOpts = opts
		return nil
	}
}

// EnableAutoRelay 配置dep2p启用AutoRelay子系统
// 依赖:
//   - 中继(默认启用)
//   - 以下之一:
//     1. 静态中继列表
//     2. 提供中继chan的PeerSource函数。参见`autorelay.WithPeerSource`
//
// 当检测到节点公开不可访问时(例如在NAT后面),此子系统执行自动地址重写以通告中继地址
//
// 已弃用:使用EnableAutoRelayWithStaticRelays或EnableAutoRelayWithPeerSource
// 参数:
//   - opts: AutoRelay选项
//
// 返回:
//   - Option: 配置函数
func EnableAutoRelay(opts ...autorelay.Option) Option {
	return func(cfg *Config) error {
		cfg.EnableAutoRelay = true
		cfg.AutoRelayOpts = opts
		return nil
	}
}

// EnableAutoRelayWithStaticRelays 使用提供的中继作为中继候选者配置dep2p启用AutoRelay子系统
// 当检测到节点公开不可访问时(例如在NAT后面),此子系统执行自动地址重写以通告中继地址
// 参数:
//   - static: 静态中继地址信息列表
//   - opts: AutoRelay选项
//
// 返回:
//   - Option: 配置函数
func EnableAutoRelayWithStaticRelays(static []peer.AddrInfo, opts ...autorelay.Option) Option {
	return func(cfg *Config) error {
		cfg.EnableAutoRelay = true
		cfg.AutoRelayOpts = append([]autorelay.Option{autorelay.WithStaticRelays(static)}, opts...)
		return nil
	}
}

// EnableAutoRelayWithPeerSource 使用提供的PeerSource回调获取更多中继候选者来配置dep2p启用AutoRelay子系统
// 当检测到节点公开不可访问时(例如在NAT后面),此子系统执行自动地址重写以通告中继地址
// 参数:
//   - peerSource: 对等点源回调函数
//   - opts: AutoRelay选项
//
// 返回:
//   - Option: 配置函数
func EnableAutoRelayWithPeerSource(peerSource autorelay.PeerSource, opts ...autorelay.Option) Option {
	return func(cfg *Config) error {
		cfg.EnableAutoRelay = true
		cfg.AutoRelayOpts = append([]autorelay.Option{autorelay.WithPeerSource(peerSource)}, opts...)
		return nil
	}
}

// ForceReachabilityPublic 覆盖AutoNAT子系统中的自动可达性检测,强制本地节点相信它是外部可达的
// 返回:
//   - Option: 配置函数
func ForceReachabilityPublic() Option {
	return func(cfg *Config) error {
		public := network.ReachabilityPublic
		cfg.AutoNATConfig.ForceReachability = &public
		return nil
	}
}

// ForceReachabilityPrivate 覆盖AutoNAT子系统中的自动可达性检测,强制本地节点相信它在NAT后面且外部不可达
// 返回:
//   - Option: 配置函数
func ForceReachabilityPrivate() Option {
	return func(cfg *Config) error {
		private := network.ReachabilityPrivate
		cfg.AutoNATConfig.ForceReachability = &private
		return nil
	}
}

// EnableNATService 配置dep2p为对等点提供确定其可达性状态的服务
// 启用后,主机将尝试回拨对等点,然后告诉它们是否成功建立此类连接
// 返回:
//   - Option: 配置函数
func EnableNATService() Option {
	return func(cfg *Config) error {
		cfg.AutoNATConfig.EnableService = true
		return nil
	}
}

// AutoNATServiceRateLimit 更改帮助其他对等点确定其可达性状态的默认速率限制
// 设置后,主机将限制在每60秒期间响应的请求数量为设定的数字
// 值为'0'禁用限制
// 参数:
//   - global: 全局限制
//   - perPeer: 每个对等点限制
//   - interval: 限制间隔
//
// 返回:
//   - Option: 配置函数
func AutoNATServiceRateLimit(global, perPeer int, interval time.Duration) Option {
	return func(cfg *Config) error {
		cfg.AutoNATConfig.ThrottleGlobalLimit = global
		cfg.AutoNATConfig.ThrottlePeerLimit = perPeer
		cfg.AutoNATConfig.ThrottleInterval = interval
		return nil
	}
}

// ConnectionGater 配置dep2p使用给定的ConnectionGater根据连接的生命周期阶段主动拒绝入站/出站连接
// 参数:
//   - cg: 连接门控器实现
//
// 返回:
//   - Option: 配置函数
func ConnectionGater(cg connmgr.ConnectionGater) Option {
	return func(cfg *Config) error {
		if cfg.ConnectionGater != nil {
			log.Debugf("不能配置多个连接门控器,或者不能同时配置Filters和ConnectionGater")
			return errors.New("不能配置多个连接门控器,或者不能同时配置Filters和ConnectionGater")
		}
		cfg.ConnectionGater = cg
		return nil
	}
}

// ResourceManager 配置dep2p使用给定的ResourceManager
// 当使用p2p/host/resource-manager实现ResourceManager接口时,建议通过调用SetDefaultServiceLimits为dep2p协议设置限制
// 参数:
//   - rcmgr: 资源管理器实现
//
// 返回:
//   - Option: 配置函数
func ResourceManager(rcmgr network.ResourceManager) Option {
	return func(cfg *Config) error {
		if cfg.ResourceManager != nil {
			log.Debugf("不能配置多个资源管理器")
			return errors.New("不能配置多个资源管理器")
		}
		cfg.ResourceManager = rcmgr
		return nil
	}
}

// NATPortMap 配置dep2p使用默认的NATManager
// 默认NATManager将尝试使用UPnP在网络防火墙中打开端口
// 返回:
//   - Option: 配置函数
func NATPortMap() Option {
	return NATManager(bhost.NewNATManager)
}

// NATManager 配置dep2p使用请求的NATManager
// 此函数应传入一个接受dep2p Network的NATManager构造函数
// 参数:
//   - nm: NAT管理器构造函数
//
// 返回:
//   - Option: 配置函数
func NATManager(nm config.NATManagerC) Option {
	return func(cfg *Config) error {
		if cfg.NATManager != nil {
			log.Debugf("不能指定多个NAT管理器")
			return fmt.Errorf("不能指定多个NAT管理器")
		}
		cfg.NATManager = nm
		return nil
	}
}

// Ping 配置dep2p支持ping服务;默认启用
// 参数:
//   - enable: 是否启用ping
//
// 返回:
//   - Option: 配置函数
func Ping(enable bool) Option {
	return func(cfg *Config) error {
		cfg.DisablePing = !enable
		return nil
	}
}

// Routing 配置dep2p使用路由
// 参数:
//   - rt: 路由构造函数
//
// 返回:
//   - Option: 配置函数
func Routing(rt config.RoutingC) Option {
	return func(cfg *Config) error {
		if cfg.Routing != nil {
			log.Debugf("不能指定多个路由选项")
			return fmt.Errorf("不能指定多个路由选项")
		}
		cfg.Routing = rt
		return nil
	}
}

// NoListenAddrs 配置dep2p默认不监听
//
// 这将清除任何已配置的监听地址并防止dep2p应用默认监听地址选项
// 它还禁用中继,除非用户通过选项显式指定,因为传输会创建一个隐式监听地址,使节点可以通过它连接的任何中继进行拨号
var NoListenAddrs = func(cfg *Config) error {
	cfg.ListenAddrs = []ma.Multiaddr{}
	if !cfg.RelayCustom {
		cfg.RelayCustom = true
		cfg.Relay = false
	}
	return nil
}

// NoTransports 配置dep2p不启用任何传输
//
// 这将清除任何已配置的传输(在先前的dep2p选项中指定)并防止dep2p应用默认传输
var NoTransports = func(cfg *Config) error {
	cfg.Transports = []fx.Option{}
	return nil
}

// ProtocolVersion 设置dep2p Identify协议所需的协议版本字符串
// 参数:
//   - s: 协议版本字符串
//
// 返回:
//   - Option: 配置函数
func ProtocolVersion(s string) Option {
	return func(cfg *Config) error {
		cfg.ProtocolVersion = s
		return nil
	}
}

// UserAgent 设置与identify协议一起发送的dep2p用户代理
// 参数:
//   - userAgent: 用户代理字符串
//
// 返回:
//   - Option: 配置函数
func UserAgent(userAgent string) Option {
	return func(cfg *Config) error {
		cfg.UserAgent = userAgent
		return nil
	}
}

// MultiaddrResolver 设置dep2p dns解析器
// 参数:
//   - rslv: multiaddr DNS解析器实现
//
// 返回:
//   - Option: 配置函数
func MultiaddrResolver(rslv network.MultiaddrDNSResolver) Option {
	return func(cfg *Config) error {
		cfg.MultiaddrResolver = rslv
		return nil
	}
}

// Experimental
// EnableHolePunching 通过使NAT后的对等点能够发起和响应打洞尝试来启用NAT穿透,以创建与其他对等点的直接/NAT穿透连接(默认:禁用)
//
// 依赖:
//   - 中继(默认启用)
//
// 此子系统执行两个功能:
//
//  1. 在接收入站中继连接时,它通过在中继连接上发起和协调打洞来尝试与远程对等点创建直接连接
//  2. 如果对等点在出站中继连接上看到协调打洞的请求,它将参与打洞以创建与远程对等点的直接连接
//
// 如果打洞成功,所有新流都将在打洞连接上创建
// 中继连接最终将在宽限期后关闭
//
// 用户必须重新打开中继连接上所有现有的无限期长期流在打洞连接上
// 用户可以使用Network发出的"Connected"/"Disconnected"通知来实现此目的
//
// 启用"AutoRelay"选项(参见"EnableAutoRelay")不是强制性的,但很好,这样如果对等点通过AutoNAT发现它是NAT后的并且通过私有可达性,它就可以发现并连接到中继服务器
// 这将使其能够通告中继地址,这些地址可用于接受入站中继连接以协调打洞
//
// 如果配置了"EnableAutoRelay"并且用户确信对等点具有私有可达性/在NAT后,可以配置"ForceReachabilityPrivate"选项以短路通过AutoNAT的可达性发现,以便对等点可以立即开始连接到中继服务器
//
// 如果配置了"EnableAutoRelay",可以使用"StaticRelays"选项配置一组静态中继服务器供"AutoRelay"连接,这样它就不需要通过路由发现中继服务器
// 参数:
//   - opts: 打洞选项
//
// 返回:
//   - Option: 配置函数
func EnableHolePunching(opts ...holepunch.Option) Option {
	return func(cfg *Config) error {
		cfg.EnableHolePunching = true
		cfg.HolePunchingOptions = opts
		return nil
	}
}

// WithDialTimeout 设置拨号超时时间
// 参数:
//   - t: 超时时间
//
// 返回:
//   - Option: 配置函数
func WithDialTimeout(t time.Duration) Option {
	return func(cfg *Config) error {
		if t <= 0 {
			log.Debugf("拨号超时时间必须为正数")
			return errors.New("拨号超时时间必须为正数")
		}
		cfg.DialTimeout = t
		return nil
	}
}

// DisableMetrics 配置dep2p禁用prometheus指标
// 返回:
//   - Option: 配置函数
func DisableMetrics() Option {
	return func(cfg *Config) error {
		cfg.DisableMetrics = true
		return nil
	}
}

// PrometheusRegisterer 配置dep2p使用reg作为所有指标子系统的注册器
// 参数:
//   - reg: prometheus注册器
//
// 返回:
//   - Option: 配置函数
func PrometheusRegisterer(reg prometheus.Registerer) Option {
	return func(cfg *Config) error {
		if cfg.DisableMetrics {
			log.Debugf("指标被禁用时不能设置注册器")
			return errors.New("指标被禁用时不能设置注册器")
		}
		if cfg.PrometheusRegisterer != nil {
			log.Debugf("注册器已设置")
			return errors.New("注册器已设置")
		}
		if reg == nil {
			log.Debugf("注册器不能为空")
			return errors.New("注册器不能为空")
		}
		cfg.PrometheusRegisterer = reg
		return nil
	}
}

// DialRanker 配置dep2p使用d作为拨号排序器。要启用智能拨号,使用`swarm.DefaultDialRanker`。使用`swarm.NoDelayDialRanker`禁用智能拨号
//
// 已弃用:使用SwarmOpts(swarm.WithDialRanker(d))代替
// 参数:
//   - d: 拨号排序器实现
//
// 返回:
//   - Option: 配置函数
func DialRanker(d network.DialRanker) Option {
	return func(cfg *Config) error {
		if cfg.DialRanker != nil {
			log.Debugf("拨号排序器已配置")
			return errors.New("拨号排序器已配置")
		}
		cfg.DialRanker = d
		return nil
	}
}

// SwarmOpts 配置dep2p使用带选项的swarm
// 参数:
//   - opts: swarm选项
//
// 返回:
//   - Option: 配置函数
func SwarmOpts(opts ...swarm.Option) Option {
	return func(cfg *Config) error {
		cfg.SwarmOpts = opts
		return nil
	}
}

// DisableIdentifyAddressDiscovery 禁用使用对等点在identify中提供的观察地址进行地址发现
// 如果你预先知道你的公共地址,建议使用AddressFactory为主机提供外部地址,并使用此选项禁用从identify进行的地址发现
// 参数:
//   - 无
//
// 返回:
//   - Option: 配置函数
func DisableIdentifyAddressDiscovery() Option {
	return func(cfg *Config) error {
		cfg.DisableIdentifyAddressDiscovery = true
		return nil
	}
}

// EnableAutoNATv2 启用AutoNAT v2功能
// 参数:
//   - 无
//
// 返回:
//   - Option: 配置函数
func EnableAutoNATv2() Option {
	return func(cfg *Config) error {
		cfg.EnableAutoNATv2 = true
		return nil
	}
}

// UDPBlackHoleSuccessCounter 配置dep2p使用f作为UDP地址的黑洞过滤器
// 参数:
//   - f: *swarm.BlackHoleSuccessCounter 黑洞成功计数器
//
// 返回:
//   - Option: 配置函数
func UDPBlackHoleSuccessCounter(f *swarm.BlackHoleSuccessCounter) Option {
	return func(cfg *Config) error {
		cfg.UDPBlackHoleSuccessCounter = f
		cfg.CustomUDPBlackHoleSuccessCounter = true
		return nil
	}
}

// IPv6BlackHoleSuccessCounter 配置dep2p使用f作为IPv6地址的黑洞过滤器
// 参数:
//   - f: *swarm.BlackHoleSuccessCounter 黑洞成功计数器
//
// 返回:
//   - Option: 配置函数
func IPv6BlackHoleSuccessCounter(f *swarm.BlackHoleSuccessCounter) Option {
	return func(cfg *Config) error {
		cfg.IPv6BlackHoleSuccessCounter = f
		cfg.CustomIPv6BlackHoleSuccessCounter = true
		return nil
	}
}

// WithFxOption 添加用户提供的fx.Option到dep2p构造函数中
// 实验性功能:此选项可能会更改或删除
// 参数:
//   - opts: ...fx.Option fx选项列表
//
// 返回:
//   - Option: 配置函数
func WithFxOption(opts ...fx.Option) Option {
	return func(cfg *Config) error {
		cfg.UserFxOptions = append(cfg.UserFxOptions, opts...)
		return nil
	}
}

// ShareTCPListener 在TCP和Websocket传输之间共享相同的监听地址,这允许两种传输使用相同的TCP端口
// 目前此行为是可选的。在未来的版本中,这将成为默认行为,此选项将被移除
// 参数:
//   - 无
//
// 返回:
//   - Option: 配置函数
func ShareTCPListener() Option {
	return func(cfg *Config) error {
		cfg.ShareTCPListener = true
		return nil
	}
}
