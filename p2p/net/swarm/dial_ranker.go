package swarm

import (
	"sort"
	"strconv"
	"time"

	"github.com/dep2p/go-dep2p/core/network"
	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

// 250ms 的值来自 Happy Eyeballs RFC 8305。这是一个 RTT 的粗略估计
const (
	// TCP 拨号相对于最后一次 QUIC 拨号延迟的时间
	PublicTCPDelay  = 250 * time.Millisecond
	PrivateTCPDelay = 30 * time.Millisecond

	// QUIC 拨号相对于前一次 QUIC 拨号延迟的时间
	PublicQUICDelay  = 250 * time.Millisecond
	PrivateQUICDelay = 30 * time.Millisecond

	// RelayDelay 是中继拨号相对于直连地址延迟的时间
	RelayDelay = 500 * time.Millisecond

	// 其他传输地址的延迟。这将应用于 /webrtc-direct
	PublicOtherDelay  = 1 * time.Second
	PrivateOtherDelay = 100 * time.Millisecond
)

// NoDelayDialRanker 对地址进行无延迟排序。这对于同时连接请求很有用。
// 参数:
//   - addrs: []ma.Multiaddr 待排序的多地址列表
//
// 返回值:
//   - []network.AddrDelay 排序后的地址延迟列表
func NoDelayDialRanker(addrs []ma.Multiaddr) []network.AddrDelay {
	return getAddrDelay(addrs, 0, 0, 0, 0)
}

// DefaultDialRanker 确定出站连接尝试的排序
// 参数:
//   - addrs: []ma.Multiaddr 待排序的多地址列表
//
// 返回值:
//   - []network.AddrDelay 排序后的地址延迟列表
//
// 地址分为三个不同的组:
//   - 私有地址(本地主机和本地网络(RFC 1918))
//   - 公共地址
//   - 中继地址
//
// 在每个组内,地址按照下面描述的排序逻辑进行排序。
// 然后我们按照这个排序拨号地址,在拨号尝试之间应用短暂的超时。
// 这种排序逻辑大大减少了同时拨号尝试的数量,同时在绝大多数情况下不会引入额外的延迟。
//
// 私有地址和公共地址组并行拨号。
// 如果我们有任何非中继选项,中继地址的拨号会延迟 500ms。
//
// 在每个组(私有、公共、中继地址)内,我们应用以下排序逻辑:
//
//  1. 如果同时存在 IPv6 QUIC 和 IPv4 QUIC 地址,我们采用类似 Happy Eyeballs RFC 8305 的排序方式。
//     首先拨号端口号最小的 IPv6 QUIC 地址。
//     之后我们拨号端口号最小的 IPv4 QUIC 地址,对于公共地址延迟 250ms(PublicQUICDelay),对于本地地址延迟 30ms(PrivateQUICDelay)。
//     之后我们拨号所有剩余的地址,对于公共地址延迟 250ms(PublicQUICDelay),对于本地地址延迟 30ms(PrivateQUICDelay)。
//  2. 如果只存在 QUIC IPv6 或 QUIC IPv4 地址中的一种,首先拨号端口号最小的 QUIC 地址。之后我们拨号剩余的 QUIC 地址,对于公共地址延迟 250ms(PublicQUICDelay),对于本地地址延迟 30ms(PrivateQUICDelay)。
//  3. 如果存在 QUIC 或 WebTransport 地址,TCP 地址的拨号相对于最后一次 QUIC 拨号会延迟:
//     我们倾向于最终建立 QUIC 连接。对于公共地址,引入的延迟是 250ms(PublicTCPDelay),对于私有地址是 30ms(PrivateTCPDelay)。
//  4. 对于 TCP 地址,我们采用类似 QUIC 的策略,并针对第 6 点中描述的 TCP 握手时间较长进行了优化。
//     如果同时存在 IPv6 TCP 和 IPv4 TCP 地址,我们采用 Happy Eyeballs 风格的排序。
//     首先拨号端口号最小的 IPv6 TCP 地址。之后,拨号端口号最小的 IPv4 TCP 地址,对于公共地址延迟 250ms(PublicTCPDelay),对于本地地址延迟 30ms(PrivateTCPDelay)。
//     之后我们拨号所有剩余的地址,对于公共地址延迟 250ms(PublicTCPDelay),对于本地地址延迟 30ms(PrivateTCPDelay)。
//  5. 如果只存在 TCP IPv6 或 TCP IPv4 地址中的一种,首先拨号端口号最小的 TCP 地址。
//     之后我们拨号剩余的 TCP 地址,对于公共地址延迟 250ms(PublicTCPDelay),对于本地地址延迟 30ms(PrivateTCPDelay)。
//  6. 当 TCP 套接字已连接并等待安全和多路复用升级时,我们停止新的拨号 2*PublicTCPDelay 以允许升级完成。
//  7. WebRTC Direct 和其他 IP 传输地址在最后一次 QUIC 或 TCP 拨号后 1 秒拨号。
//     只有当对等节点没有任何其他可用的传输时,我们才需要拨号这些地址,在这种情况下,这些地址会立即拨号。
//
// 我们首先拨号端口号最小的地址,因为它们更可能是监听端口。
func DefaultDialRanker(addrs []ma.Multiaddr) []network.AddrDelay {
	// 过滤出中继地址
	relay, addrs := filterAddrs(addrs, isRelayAddr)
	// 过滤出私有地址
	pvt, addrs := filterAddrs(addrs, manet.IsPrivateAddr)
	// 过滤出公共地址(IPv4和IPv6)
	public, addrs := filterAddrs(addrs, func(a ma.Multiaddr) bool { return isProtocolAddr(a, ma.P_IP4) || isProtocolAddr(a, ma.P_IP6) })

	// 如果有公共直连地址,延迟中继拨号
	var relayOffset time.Duration
	if len(public) > 0 {
		// 如果有公共直连地址可用,延迟中继拨号
		relayOffset = RelayDelay
	}

	// 构建结果切片
	res := make([]network.AddrDelay, 0, len(addrs))
	// 按顺序添加私有地址、公共地址和中继地址
	res = append(res, getAddrDelay(pvt, PrivateTCPDelay, PrivateQUICDelay, PrivateOtherDelay, 0)...)
	res = append(res, getAddrDelay(public, PublicTCPDelay, PublicQUICDelay, PublicOtherDelay, 0)...)
	res = append(res, getAddrDelay(relay, PublicTCPDelay, PublicQUICDelay, PublicOtherDelay, relayOffset)...)

	// 获取最大延迟时间
	var maxDelay time.Duration
	if len(res) > 0 {
		maxDelay = res[len(res)-1].Delay
	}

	// 添加剩余地址,延迟设为最大延迟加上公共其他延迟
	for i := 0; i < len(addrs); i++ {
		res = append(res, network.AddrDelay{Addr: addrs[i], Delay: maxDelay + PublicOtherDelay})
	}

	return res
}

// getAddrDelay 根据 defaultDialRanker 文档中解释的排序逻辑对一组地址进行排序
// offset 用于将所有地址延迟固定时间。这对于相对于直连地址延迟所有中继地址很有用。
// 参数:
//   - addrs: []ma.Multiaddr 待排序的多地址列表
//   - tcpDelay: time.Duration TCP 拨号延迟
//   - quicDelay: time.Duration QUIC 拨号延迟
//   - otherDelay: time.Duration 其他协议拨号延迟
//   - offset: time.Duration 基础延迟偏移量
//
// 返回值:
//   - []network.AddrDelay 排序后的地址延迟列表
func getAddrDelay(addrs []ma.Multiaddr, tcpDelay time.Duration, quicDelay time.Duration,
	otherDelay time.Duration, offset time.Duration) []network.AddrDelay {
	// 空地址列表直接返回nil
	if len(addrs) == 0 {
		return nil
	}

	// 按分数排序地址
	sort.Slice(addrs, func(i, j int) bool { return score(addrs[i]) < score(addrs[j]) })

	// 标记是否需要对QUIC和TCP使用happy eyeballs
	// addrs 现在按(传输,IP版本)排序。重新排序 addrs 以进行 happy eyeballs 拨号。
	// 对于 QUIC 和 TCP,如果我们同时有 IPv6 和 IPv4 地址,将优先级最高的 IPv4 地址移到第二个位置。
	happyEyeballsQUIC := false
	happyEyeballsTCP := false
	// tcpStartIdx 是第一个 TCP 地址的索引
	var tcpStartIdx int

	{
		i := 0
		// 如果第一个QUIC地址是IPv6,将第一个IPv4 QUIC地址移到第二位
		if isQUICAddr(addrs[0]) && isProtocolAddr(addrs[0], ma.P_IP6) {
			for j := 1; j < len(addrs); j++ {
				if isQUICAddr(addrs[j]) && isProtocolAddr(addrs[j], ma.P_IP4) {
					// 第一个 IPv4 地址在位置 j
					// 将第 j 个元素移到位置 1,移动受影响的元素
					if j > 1 {
						a := addrs[j]
						copy(addrs[2:], addrs[1:j])
						addrs[1] = a
					}
					happyEyeballsQUIC = true
					i = j + 1
					break
				}
			}
		}

		// 找到第一个TCP地址的位置
		for tcpStartIdx = i; tcpStartIdx < len(addrs); tcpStartIdx++ {
			if isProtocolAddr(addrs[tcpStartIdx], ma.P_TCP) {
				break
			}
		}

		// 如果第一个 TCP 地址是 IPv6,将第一个 TCP IPv4 地址移到第二个位置
		if tcpStartIdx < len(addrs) && isProtocolAddr(addrs[tcpStartIdx], ma.P_IP6) {
			for j := tcpStartIdx + 1; j < len(addrs); j++ {
				if isProtocolAddr(addrs[j], ma.P_TCP) && isProtocolAddr(addrs[j], ma.P_IP4) {
					// 第一个 TCP IPv4 地址在位置 j,将其移到位置 tcpStartIdx+1,这是第二优先级的 TCP 地址
					if j > tcpStartIdx+1 {
						a := addrs[j]
						copy(addrs[tcpStartIdx+2:], addrs[tcpStartIdx+1:j])
						addrs[tcpStartIdx+1] = a
					}
					happyEyeballsTCP = true
					break
				}
			}
		}
	}

	// 构建结果切片
	res := make([]network.AddrDelay, 0, len(addrs))
	var tcpFirstDialDelay time.Duration
	var lastQUICOrTCPDelay time.Duration

	// 遍历地址计算延迟
	for i, addr := range addrs {
		var delay time.Duration
		switch {
		case isQUICAddr(addr):
			// QUIC地址延迟计算
			// 我们先拨号 IPv6 地址,然后在 quicDelay 后拨号 IPv4 地址,然后在另一个 quicDelay 后拨号剩余的地址。
			if i == 1 {
				delay = quicDelay
			}
			if i > 1 {
				// 如果我们对 QUIC 使用 happy eyeballs,第二个位置之后的拨号将延迟 2*quicDelay
				if happyEyeballsQUIC {
					delay = 2 * quicDelay
				} else {
					delay = quicDelay
				}
			}
			lastQUICOrTCPDelay = delay
			tcpFirstDialDelay = delay + tcpDelay

		case isProtocolAddr(addr, ma.P_TCP):
			// TCP地址延迟计算
			// 我们先拨号 IPv6 地址,然后在 tcpDelay 后拨号 IPv4 地址,然后在另一个 tcpDelay 后拨号剩余的地址。
			if i == tcpStartIdx+1 {
				delay = tcpDelay
			}
			if i > tcpStartIdx+1 {
				// 如果我们对 TCP 使用 happy eyeballs,第二个位置之后的拨号将延迟 2*tcpDelay
				if happyEyeballsTCP {
					delay = 2 * tcpDelay
				} else {
					delay = tcpDelay
				}
			}
			delay += tcpFirstDialDelay
			lastQUICOrTCPDelay = delay
		// 如果既不是 quic、webtransport、tcp 也不是 websocket 地址
		default:
			// 其他协议地址延迟计算
			delay = lastQUICOrTCPDelay + otherDelay
		}
		res = append(res, network.AddrDelay{Addr: addr, Delay: offset + delay})
	}
	return res
}

// score 对多地址进行拨号延迟评分
// 分数越低越好。
// 结果的低 16 位是端口号。
// 低端口排名更高,因为它们更可能是监听地址。
// 参数:
//   - a: ma.Multiaddr 待评分的多地址
//
// 返回值:
//   - int 评分结果,分数越低优先级越高
//
// 地址排名为:
// QUICv1 IPv6 > QUICdraft29 IPv6 > QUICv1 IPv4 > QUICdraft29 IPv4 > WebTransport IPv6 > WebTransport IPv4 > TCP IPv6 > TCP IPv4
func score(a ma.Multiaddr) int {
	// IPv4地址权重
	ip4Weight := 0
	if isProtocolAddr(a, ma.P_IP4) {
		ip4Weight = 1 << 18
	}

	// WebTransport地址评分
	if _, err := a.ValueForProtocol(ma.P_WEBTRANSPORT); err == nil {
		p, _ := a.ValueForProtocol(ma.P_UDP)
		pi, _ := strconv.Atoi(p)
		return ip4Weight + (1 << 19) + pi
	}

	// QUIC地址评分
	if _, err := a.ValueForProtocol(ma.P_QUIC); err == nil {
		p, _ := a.ValueForProtocol(ma.P_UDP)
		pi, _ := strconv.Atoi(p)
		return ip4Weight + pi + (1 << 17)
	}

	// QUICv1地址评分
	if _, err := a.ValueForProtocol(ma.P_QUIC_V1); err == nil {
		p, _ := a.ValueForProtocol(ma.P_UDP)
		pi, _ := strconv.Atoi(p)
		return ip4Weight + pi
	}

	// TCP地址评分
	if p, err := a.ValueForProtocol(ma.P_TCP); err == nil {
		pi, _ := strconv.Atoi(p)
		return ip4Weight + pi + (1 << 20)
	}

	// WebRTC Direct地址评分
	if _, err := a.ValueForProtocol(ma.P_WEBRTC_DIRECT); err == nil {
		return 1 << 21
	}

	// 其他地址评分
	return (1 << 30)
}

// isProtocolAddr 检查多地址是否包含指定协议
// 参数:
//   - a: ma.Multiaddr 待检查的多地址
//   - p: int 协议代码
//
// 返回值:
//   - bool 如果包含指定协议返回true,否则返回false
func isProtocolAddr(a ma.Multiaddr, p int) bool {
	found := false
	ma.ForEach(a, func(c ma.Component) bool {
		if c.Protocol().Code == p {
			found = true
			return false
		}
		return true
	})
	return found
}

// isQUICAddr 检查多地址是否为QUIC地址
// 参数:
//   - a: ma.Multiaddr 待检查的多地址
//
// 返回值:
//   - bool 如果是QUIC地址返回true,否则返回false
func isQUICAddr(a ma.Multiaddr) bool {
	return isProtocolAddr(a, ma.P_QUIC) || isProtocolAddr(a, ma.P_QUIC_V1)
}

// filterAddrs 就地过滤地址切片
// 参数:
//   - addrs: []ma.Multiaddr 待过滤的地址切片
//   - f: func(a ma.Multiaddr) bool 过滤函数
//
// 返回值:
//   - filtered: []ma.Multiaddr 满足过滤条件的地址切片
//   - rest: []ma.Multiaddr 不满足过滤条件的地址切片
func filterAddrs(addrs []ma.Multiaddr, f func(a ma.Multiaddr) bool) (filtered, rest []ma.Multiaddr) {
	j := 0
	for i := 0; i < len(addrs); i++ {
		if f(addrs[i]) {
			addrs[i], addrs[j] = addrs[j], addrs[i]
			j++
		}
	}
	return addrs[:j], addrs[j:]
}
