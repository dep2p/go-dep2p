package autonat

import (
	"errors"
	"time"

	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
)

// config 保存 autonat 子系统的可配置选项
type config struct {
	host host.Host // dep2p 主机实例,用于网络通信和节点管理

	addressFunc       AddrFunc             // 地址获取函数,用于获取本地地址
	dialPolicy        dialPolicy           // 拨号策略,定义如何处理拨号请求
	dialer            network.Network      // 网络拨号器,用于建立网络连接
	forceReachability bool                 // 是否强制设置可达性,true表示强制使用指定的可达性状态
	reachability      network.Reachability // 可达性状态,表示节点的网络可达性类型
	metricsTracer     MetricsTracer        // 指标追踪器,用于收集和记录指标数据

	// 客户端配置
	bootDelay          time.Duration // 启动延迟时间,系统启动时的等待时间
	retryInterval      time.Duration // 重试间隔时间,失败后的重试等待时间
	refreshInterval    time.Duration // 刷新间隔时间,定期刷新状态的时间间隔
	requestTimeout     time.Duration // 请求超时时间,单个请求的最大等待时间
	throttlePeerPeriod time.Duration // 节点限流周期,对单个节点限流的时间周期

	// 服务端配置
	dialTimeout         time.Duration // 拨号超时时间,建立连接的最大等待时间
	maxPeerAddresses    int           // 每个节点最大地址数,限制每个节点可以注册的地址数量
	throttleGlobalMax   int           // 全局限流最大值,所有节点的总限流阈值
	throttlePeerMax     int           // 每个节点限流最大值,单个节点的限流阈值
	throttleResetPeriod time.Duration // 限流重置周期,重置限流计数器的时间间隔
	throttleResetJitter time.Duration // 限流重置抖动时间,重置时间的随机偏移量
}

// 默认配置
// 参数:
//   - c: *config 配置对象指针,用于设置默认值
//
// 返回值:
//   - error: 如果发生错误则返回错误信息,nil表示设置成功
var defaults = func(c *config) error {
	c.bootDelay = 15 * time.Second          // 设置默认启动延迟为15秒
	c.retryInterval = 90 * time.Second      // 设置默认重试间隔为90秒
	c.refreshInterval = 15 * time.Minute    // 设置默认刷新间隔为15分钟
	c.requestTimeout = 30 * time.Second     // 设置默认请求超时为30秒
	c.throttlePeerPeriod = 90 * time.Second // 设置默认节点限流周期为90秒

	c.dialTimeout = 15 * time.Second         // 设置默认拨号超时为15秒
	c.maxPeerAddresses = 16                  // 设置默认每个节点最大地址数为16
	c.throttleGlobalMax = 30                 // 设置默认全局限流最大值为30
	c.throttlePeerMax = 3                    // 设置默认每个节点限流最大值为3
	c.throttleResetPeriod = 1 * time.Minute  // 设置默认限流重置周期为1分钟
	c.throttleResetJitter = 15 * time.Second // 设置默认限流重置抖动时间为15秒
	return nil
}

// 最大刷新间隔时间常量,定义了刷新间隔的上限为24小时
const maxRefreshInterval = 24 * time.Hour

// EnableService 指定 AutoNAT 可以运行 NAT 服务来帮助其他节点确定自己的 NAT 状态。
// 提供的 Network 不应该是传递给 New 的主机的默认网络/拨号器,因为 NAT 系统需要建立并行连接,
// 因此会修改相关的节点存储并终止此拨号器的连接。但是提供的拨号器应该与 dep2p 网络的传输协议兼容(TCP/UDP)。
// 参数:
//   - dialer: network.Network 用于建立连接的网络拨号器
//
// 返回值:
//   - Option: 返回配置选项函数
func EnableService(dialer network.Network) Option {
	return func(c *config) error {
		if dialer == c.host.Network() || dialer.Peerstore() == c.host.Peerstore() {
			log.Debugf("拨号器不应该是主机的拨号器")
			return errors.New("拨号器不应该是主机的拨号器")
		}
		c.dialer = dialer
		return nil
	}
}

// WithReachability 覆盖 autonat 以简单地报告一个被覆盖的可达性状态
// 参数:
//   - reachability: network.Reachability 要设置的可达性状态
//
// 返回值:
//   - Option: 返回配置选项函数
func WithReachability(reachability network.Reachability) Option {
	return func(c *config) error {
		c.forceReachability = true
		c.reachability = reachability
		return nil
	}
}

// UsingAddresses 允许覆盖 AutoNAT 客户端认为是"自己的"地址。
// 这对测试很有用,或者对于更特殊的端口转发场景,主机可能在不同于它想要对外广播或验证连接性的端口上监听。
// 参数:
//   - addrFunc: AddrFunc 用于获取地址的函数
//
// 返回值:
//   - Option: 返回配置选项函数
func UsingAddresses(addrFunc AddrFunc) Option {
	return func(c *config) error {
		if addrFunc == nil {
			log.Debugf("提供的地址函数无效")
			return errors.New("提供的地址函数无效")
		}
		c.addressFunc = addrFunc
		return nil
	}
}

// WithSchedule 配置探测主机地址的频率。
// retryInterval 表示当主机对其地址缺乏信心时应该多久探测一次,
// refreshInterval 表示当主机认为它知道其稳态可达性时的定期探测计划。
// 参数:
//   - retryInterval: time.Duration 重试间隔时间
//   - refreshInterval: time.Duration 刷新间隔时间
//
// 返回值:
//   - Option: 返回配置选项函数
func WithSchedule(retryInterval, refreshInterval time.Duration) Option {
	return func(c *config) error {
		c.retryInterval = retryInterval
		c.refreshInterval = refreshInterval
		return nil
	}
}

// WithoutStartupDelay 移除 NAT 子系统通常用作缓冲的初始延迟,
// 该延迟用于确保在启动期间连接性和对主机本地接口的猜测已经稳定。
// 返回值:
//   - Option: 返回配置选项函数
func WithoutStartupDelay() Option {
	return func(c *config) error {
		c.bootDelay = 1 // 将启动延迟设置为1纳秒
		return nil
	}
}

// WithoutThrottling 表示此 autonat 服务在作为服务器时不应限制它愿意帮助的节点数量
// 返回值:
//   - Option: 返回配置选项函数
func WithoutThrottling() Option {
	return func(c *config) error {
		c.throttleGlobalMax = 0 // 设置全局限流最大值为0,表示不限制
		return nil
	}
}

// WithThrottling 指定在作为服务器时每个时间间隔(interval)愿意帮助多少个节点(amount)
// 参数:
//   - amount: int 限流阈值
//   - interval: time.Duration 限流时间间隔
//
// 返回值:
//   - Option: 返回配置选项函数
func WithThrottling(amount int, interval time.Duration) Option {
	return func(c *config) error {
		c.throttleGlobalMax = amount         // 设置全局限流最大值
		c.throttleResetPeriod = interval     // 设置限流重置周期
		c.throttleResetJitter = interval / 4 // 设置限流重置抖动时间为间隔的1/4
		return nil
	}
}

// WithPeerThrottling 指定此节点在每个间隔内为单个节点提供的 IP 检查的最大数量限制
// 参数:
//   - amount: int 每个节点的限流阈值
//
// 返回值:
//   - Option: 返回配置选项函数
func WithPeerThrottling(amount int) Option {
	return func(c *config) error {
		c.throttlePeerMax = amount // 设置每个节点的限流最大值
		return nil
	}
}

// WithMetricsTracer 使用 mt 来跟踪 autonat 指标
// 参数:
//   - mt: MetricsTracer 指标追踪器实例
//
// 返回值:
//   - Option: 返回配置选项函数
func WithMetricsTracer(mt MetricsTracer) Option {
	return func(c *config) error {
		c.metricsTracer = mt // 设置指标追踪器
		return nil
	}
}
