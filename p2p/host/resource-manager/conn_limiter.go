package rcmgr

import (
	"math"
	"net/netip"
	"slices"
	"sync"
)

// ConnLimitPerSubnet 定义了每个子网的连接限制
type ConnLimitPerSubnet struct {
	// PrefixLength 定义了子网的大小。例如，/24 子网的 PrefixLength 为 24。
	// 所有共享相同 24 位前缀的 IP 都在同一个子网中,并受相同的限制约束。
	PrefixLength int
	// ConnCount 每个子网允许的最大连接数。
	ConnCount int
}

// NetworkPrefixLimit 定义了特定网络前缀的连接限制
type NetworkPrefixLimit struct {
	// Network 此限制适用的网络前缀。
	Network netip.Prefix
	// ConnCount 此子网允许的最大连接数。
	ConnCount int
}

// defaultMaxConcurrentConns 目前为 8,与 swarm_dial.go 中可能进行的并发拨号数量相匹配。
// 随着未来智能拨号工作的进行,我们应该降低这个值
var defaultMaxConcurrentConns = 8

// defaultIP4Limit 定义了 IPv4 的默认连接限制
var defaultIP4Limit = ConnLimitPerSubnet{
	ConnCount:    defaultMaxConcurrentConns,
	PrefixLength: 32,
}

// defaultIP6Limits 定义了 IPv6 的默认连接限制
var defaultIP6Limits = []ConnLimitPerSubnet{
	{
		ConnCount:    defaultMaxConcurrentConns,
		PrefixLength: 56,
	},
	{
		ConnCount:    8 * defaultMaxConcurrentConns,
		PrefixLength: 48,
	},
}

// DefaultNetworkPrefixLimitV4 定义了 IPv4 的默认网络前缀限制
var DefaultNetworkPrefixLimitV4 = sortNetworkPrefixes([]NetworkPrefixLimit{
	{
		// v4 的回环地址 https://datatracker.ietf.org/doc/html/rfc6890#section-2.2.2
		Network:   netip.MustParsePrefix("127.0.0.0/8"),
		ConnCount: math.MaxInt, // 无限制
	},
})

// DefaultNetworkPrefixLimitV6 定义了 IPv6 的默认网络前缀限制
var DefaultNetworkPrefixLimitV6 = sortNetworkPrefixes([]NetworkPrefixLimit{
	{
		// v6 的回环地址 https://datatracker.ietf.org/doc/html/rfc6890#section-2.2.3
		Network:   netip.MustParsePrefix("::1/128"),
		ConnCount: math.MaxInt, // 无限制
	},
})

// sortNetworkPrefixes 对网络前缀限制进行排序。
// 网络前缀限制必须按照从最具体到最不具体的顺序排序。
// 这让我们能够实际使用更具体的限制,否则只会匹配到不太具体的限制。例如 1.2.3.0/24 必须在 1.2.0.0/16 之前。
//
// 参数:
//   - limits: []NetworkPrefixLimit - 需要排序的网络前缀限制列表
//
// 返回值:
//   - []NetworkPrefixLimit - 排序后的网络前缀限制列表
func sortNetworkPrefixes(limits []NetworkPrefixLimit) []NetworkPrefixLimit {
	// 使用 slices.SortStableFunc 进行稳定排序,按照前缀位数从大到小排序
	slices.SortStableFunc(limits, func(a, b NetworkPrefixLimit) int {
		return b.Network.Bits() - a.Network.Bits()
	})
	return limits
}

// WithNetworkPrefixLimit 设置特定网络前缀允许的连接数限制。
// 当你想为特定子网设置比默认子网限制更高的限制时使用此选项。
//
// 参数:
//   - ipv4: []NetworkPrefixLimit - IPv4 的网络前缀限制列表
//   - ipv6: []NetworkPrefixLimit - IPv6 的网络前缀限制列表
//
// 返回值:
//   - Option - 返回一个资源管理器选项函数
func WithNetworkPrefixLimit(ipv4 []NetworkPrefixLimit, ipv6 []NetworkPrefixLimit) Option {
	return func(rm *resourceManager) error {
		// 如果提供了 IPv4 限制,则更新
		if ipv4 != nil {
			rm.connLimiter.networkPrefixLimitV4 = sortNetworkPrefixes(ipv4)
		}
		// 如果提供了 IPv6 限制,则更新
		if ipv6 != nil {
			rm.connLimiter.networkPrefixLimitV6 = sortNetworkPrefixes(ipv6)
		}
		return nil
	}
}

// WithLimitPerSubnet 设置每个子网允许的连接数限制。
// 如果该子网未在 NetworkPrefixLimit 选项中定义,这将限制每个子网的连接数。
// 可以将其视为任何给定子网的默认限制。
//
// 参数:
//   - ipv4: []ConnLimitPerSubnet - IPv4 的子网限制列表
//   - ipv6: []ConnLimitPerSubnet - IPv6 的子网限制列表
//
// 返回值:
//   - Option - 返回一个资源管理器选项函数
func WithLimitPerSubnet(ipv4 []ConnLimitPerSubnet, ipv6 []ConnLimitPerSubnet) Option {
	return func(rm *resourceManager) error {
		// 如果提供了 IPv4 限制,则更新
		if ipv4 != nil {
			rm.connLimiter.connLimitPerSubnetV4 = ipv4
		}
		// 如果提供了 IPv6 限制,则更新
		if ipv6 != nil {
			rm.connLimiter.connLimitPerSubnetV6 = ipv6
		}
		return nil
	}
}

// connLimiter 连接限制器结构体
type connLimiter struct {
	mu sync.Mutex // 互斥锁,用于并发控制

	// 特定网络前缀限制。如果设置了这些限制,它们优先于子网限制。
	// 这些必须按照从最具体到最不具体的顺序排序。
	networkPrefixLimitV4    []NetworkPrefixLimit // IPv4 网络前缀限制
	networkPrefixLimitV6    []NetworkPrefixLimit // IPv6 网络前缀限制
	connsPerNetworkPrefixV4 []int                // 每个 IPv4 网络前缀的当前连接数
	connsPerNetworkPrefixV6 []int                // 每个 IPv6 网络前缀的当前连接数

	// 子网限制
	connLimitPerSubnetV4 []ConnLimitPerSubnet // IPv4 子网限制
	connLimitPerSubnetV6 []ConnLimitPerSubnet // IPv6 子网限制
	ip4connsPerLimit     []map[string]int     // 每个 IPv4 子网的当前连接数
	ip6connsPerLimit     []map[string]int     // 每个 IPv6 子网的当前连接数
}

// newConnLimiter 创建一个新的连接限制器
//
// 返回值:
//   - *connLimiter - 返回一个初始化的连接限制器指针
func newConnLimiter() *connLimiter {
	return &connLimiter{
		networkPrefixLimitV4: DefaultNetworkPrefixLimitV4,
		networkPrefixLimitV6: DefaultNetworkPrefixLimitV6,
		connLimitPerSubnetV4: []ConnLimitPerSubnet{defaultIP4Limit},
		connLimitPerSubnetV6: defaultIP6Limits,
	}
}

// addNetworkPrefixLimit 添加一个网络前缀限制
//
// 参数:
//   - isIP6: bool - 是否为 IPv6
//   - npLimit: NetworkPrefixLimit - 要添加的网络前缀限制
func (cl *connLimiter) addNetworkPrefixLimit(isIP6 bool, npLimit NetworkPrefixLimit) {
	cl.mu.Lock()
	defer cl.mu.Unlock()
	// 根据 IP 版本添加限制并重新排序
	if isIP6 {
		cl.networkPrefixLimitV6 = append(cl.networkPrefixLimitV6, npLimit)
		cl.networkPrefixLimitV6 = sortNetworkPrefixes(cl.networkPrefixLimitV6)
	} else {
		cl.networkPrefixLimitV4 = append(cl.networkPrefixLimitV4, npLimit)
		cl.networkPrefixLimitV4 = sortNetworkPrefixes(cl.networkPrefixLimitV4)
	}
}

// addConn 为给定的 IP 地址添加一个连接
//
// 参数:
//   - ip: netip.Addr - IP 地址
//
// 返回值:
//   - bool - 如果允许连接返回 true,否则返回 false
func (cl *connLimiter) addConn(ip netip.Addr) bool {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	// 根据 IP 版本选择相应的限制和计数器
	networkPrefixLimits := cl.networkPrefixLimitV4
	connsPerNetworkPrefix := cl.connsPerNetworkPrefixV4
	limits := cl.connLimitPerSubnetV4
	connsPerLimit := cl.ip4connsPerLimit
	isIP6 := ip.Is6()
	if isIP6 {
		networkPrefixLimits = cl.networkPrefixLimitV6
		connsPerNetworkPrefix = cl.connsPerNetworkPrefixV6
		limits = cl.connLimitPerSubnetV6
		connsPerLimit = cl.ip6connsPerLimit
	}

	// 首先检查网络前缀限制
	if len(connsPerNetworkPrefix) == 0 && len(networkPrefixLimits) > 0 {
		// 初始化计数
		connsPerNetworkPrefix = make([]int, len(networkPrefixLimits))
		if isIP6 {
			cl.connsPerNetworkPrefixV6 = connsPerNetworkPrefix
		} else {
			cl.connsPerNetworkPrefixV4 = connsPerNetworkPrefix
		}
	}

	// 检查是否匹配任何网络前缀限制
	for i, limit := range networkPrefixLimits {
		if limit.Network.Contains(ip) {
			if connsPerNetworkPrefix[i]+1 > limit.ConnCount {
				return false
			}
			connsPerNetworkPrefix[i]++
			return true
		}
	}

	// 初始化子网连接计数映射
	if len(connsPerLimit) == 0 && len(limits) > 0 {
		connsPerLimit = make([]map[string]int, len(limits))
		if isIP6 {
			cl.ip6connsPerLimit = connsPerLimit
		} else {
			cl.ip4connsPerLimit = connsPerLimit
		}
	}

	// 检查所有子网限制
	for i, limit := range limits {
		prefix, err := ip.Prefix(limit.PrefixLength)
		if err != nil {
			return false
		}
		masked := prefix.String()
		counts, ok := connsPerLimit[i][masked]
		if !ok {
			if connsPerLimit[i] == nil {
				connsPerLimit[i] = make(map[string]int)
			}
			connsPerLimit[i][masked] = 0
		}
		if counts+1 > limit.ConnCount {
			return false
		}
	}

	// 所有限制检查通过,更新计数
	for i, limit := range limits {
		prefix, _ := ip.Prefix(limit.PrefixLength)
		masked := prefix.String()
		connsPerLimit[i][masked]++
	}

	return true
}

// rmConn 移除一个 IP 地址的连接
//
// 参数:
//   - ip: netip.Addr - 要移除连接的 IP 地址
func (cl *connLimiter) rmConn(ip netip.Addr) {
	cl.mu.Lock()
	defer cl.mu.Unlock()

	// 根据 IP 版本选择相应的限制和计数器
	networkPrefixLimits := cl.networkPrefixLimitV4
	connsPerNetworkPrefix := cl.connsPerNetworkPrefixV4
	limits := cl.connLimitPerSubnetV4
	connsPerLimit := cl.ip4connsPerLimit
	isIP6 := ip.Is6()
	if isIP6 {
		networkPrefixLimits = cl.networkPrefixLimitV6
		connsPerNetworkPrefix = cl.connsPerNetworkPrefixV6
		limits = cl.connLimitPerSubnetV6
		connsPerLimit = cl.ip6connsPerLimit
	}

	// 首先检查网络前缀限制
	if len(connsPerNetworkPrefix) == 0 && len(networkPrefixLimits) > 0 {
		// 以防万一进行初始化
		connsPerNetworkPrefix = make([]int, len(networkPrefixLimits))
		if isIP6 {
			cl.connsPerNetworkPrefixV6 = connsPerNetworkPrefix
		} else {
			cl.connsPerNetworkPrefixV4 = connsPerNetworkPrefix
		}
	}

	// 检查是否匹配任何网络前缀限制
	for i, limit := range networkPrefixLimits {
		if limit.Network.Contains(ip) {
			count := connsPerNetworkPrefix[i]
			if count <= 0 {
				log.Errorf("IP %s 的连接计数异常。是否没有先通过 addConn 添加?", ip)
				return
			}
			connsPerNetworkPrefix[i]--
			return
		}
	}

	// 初始化子网连接计数映射
	if len(connsPerLimit) == 0 && len(limits) > 0 {
		connsPerLimit = make([]map[string]int, len(limits))
		if isIP6 {
			cl.ip6connsPerLimit = connsPerLimit
		} else {
			cl.ip4connsPerLimit = connsPerLimit
		}
	}

	// 更新所有子网限制的计数
	for i, limit := range limits {
		prefix, err := ip.Prefix(limit.PrefixLength)
		if err != nil {
			log.Errorf("获取前缀时出现意外错误: %v", err)
			continue
		}
		masked := prefix.String()
		counts, ok := connsPerLimit[i][masked]
		if !ok || counts == 0 {
			log.Errorf("%s 的连接计数异常 ok=%v count=%v", masked, ok, counts)
			continue
		}
		connsPerLimit[i][masked]--
		if connsPerLimit[i][masked] <= 0 {
			delete(connsPerLimit[i], masked)
		}
	}
}
