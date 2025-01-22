package swarm

import (
	"fmt"
	"sync"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

// BlackHoleState 表示黑洞检测器的状态
type BlackHoleState int

const (
	// blackHoleStateProbing 表示正在探测状态
	blackHoleStateProbing BlackHoleState = iota
	// blackHoleStateAllowed 表示允许状态
	blackHoleStateAllowed
	// blackHoleStateBlocked 表示被阻塞状态
	blackHoleStateBlocked
)

// String 返回状态的字符串表示
// 返回值:
//   - string 状态的字符串描述
func (st BlackHoleState) String() string {
	switch st {
	case blackHoleStateProbing:
		return "Probing"
	case blackHoleStateAllowed:
		return "Allowed"
	case blackHoleStateBlocked:
		return "Blocked"
	default:
		return fmt.Sprintf("Unknown %d", st)
	}
}

// BlackHoleSuccessCounter 为拨号提供黑洞过滤功能
// 此过滤器应与 UDP 或 IPv6 地址过滤器一起使用,以检测 UDP 或 IPv6 黑洞
// 在黑洞环境中,如果最近 N 次拨号中成功次数少于 MinSuccesses,则拨号请求会被拒绝
// 如果在阻塞状态下请求成功,过滤器状态将重置,并在重新评估黑洞状态之前允许 N 个后续请求
// 当其他并发拨号成功时取消的拨号将被计为失败
// 足够大的 N 可以防止这种情况下的误报
type BlackHoleSuccessCounter struct {
	// N 表示:
	// 1. 评估黑洞状态前所需的最小完成拨号次数
	// 2. 在阻塞状态下探测黑洞状态的最小请求次数
	N int
	// MinSuccesses 是最近 n 次拨号中需要的最小成功次数,用于判断是否未被阻塞
	MinSuccesses int
	// Name 是检测器的名称
	Name string

	mu sync.Mutex
	// requests 记录对对等点的拨号请求数
	// 我们在对等点级别处理请求,并在单个地址拨号级别记录结果
	requests int
	// dialResults 记录最近 `n` 次拨号的结果,成功拨号为 true
	dialResults []bool
	// successes 是 outcomes 中成功拨号的计数
	successes int
	// state 是检测器的当前状态
	state BlackHoleState
}

// RecordResult 记录拨号的结果
// 参数:
//   - success: bool 拨号是否成功
//
// 说明:
//   - 在阻塞状态下成功拨号会将过滤器状态更改为探测状态
//   - 仅当最近 n 次结果中的成功比例小于过滤器的最小成功比例时,失败的拨号才会阻塞后续请求
func (b *BlackHoleSuccessCounter) RecordResult(success bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.state == blackHoleStateBlocked && success {
		// 如果在阻塞状态下调用成功,我们重置为允许状态
		// 这比慢慢累积值直到我们超过最小成功比例阈值要好,因为黑洞是一个二元属性
		b.reset()
		return
	}

	if success {
		b.successes++
	}
	b.dialResults = append(b.dialResults, success)

	if len(b.dialResults) > b.N {
		if b.dialResults[0] {
			b.successes--
		}
		b.dialResults = b.dialResults[1:]
	}

	b.updateState()
}

// HandleRequest 返回请求的黑洞过滤结果
// 返回值:
//   - BlackHoleState 黑洞检测器的状态
func (b *BlackHoleSuccessCounter) HandleRequest() BlackHoleState {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.requests++

	if b.state == blackHoleStateAllowed {
		return blackHoleStateAllowed
	} else if b.state == blackHoleStateProbing || b.requests%b.N == 0 {
		return blackHoleStateProbing
	} else {
		return blackHoleStateBlocked
	}
}

// reset 重置计数器的状态
func (b *BlackHoleSuccessCounter) reset() {
	b.successes = 0
	b.dialResults = b.dialResults[:0]
	b.requests = 0
	b.updateState()
}

// updateState 更新检测器的状态
func (b *BlackHoleSuccessCounter) updateState() {
	st := b.state

	if len(b.dialResults) < b.N {
		b.state = blackHoleStateProbing
	} else if b.successes >= b.MinSuccesses {
		b.state = blackHoleStateAllowed
	} else {
		b.state = blackHoleStateBlocked
	}

	if st != b.state {
		log.Debugf("%s 黑洞检测器状态从 %s 变更为 %s", b.Name, st, b.state)
	}
}

// State 返回当前状态
// 返回值:
//   - BlackHoleState 黑洞检测器的当前状态
func (b *BlackHoleSuccessCounter) State() BlackHoleState {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.state
}

// blackHoleInfo 存储黑洞检测器的信息
type blackHoleInfo struct {
	// name 是检测器的名称
	name string
	// state 是当前状态
	state BlackHoleState
	// nextProbeAfter 是下次探测前需要等待的请求数
	nextProbeAfter int
	// successFraction 是成功比例
	successFraction float64
}

// info 返回检测器的当前信息
// 返回值:
//   - blackHoleInfo 包含检测器当前状态的信息
func (b *BlackHoleSuccessCounter) info() blackHoleInfo {
	b.mu.Lock()
	defer b.mu.Unlock()

	nextProbeAfter := 0
	if b.state == blackHoleStateBlocked {
		nextProbeAfter = b.N - (b.requests % b.N)
	}

	successFraction := 0.0
	if len(b.dialResults) > 0 {
		successFraction = float64(b.successes) / float64(len(b.dialResults))
	}

	return blackHoleInfo{
		name:            b.Name,
		state:           b.state,
		nextProbeAfter:  nextProbeAfter,
		successFraction: successFraction,
	}
}

// blackHoleDetector 使用 `BlackHoleSuccessCounter` 为每个协议提供 UDP 和 IPv6 黑洞检测
// 有关黑洞检测逻辑的详细信息,请参见 `BlackHoleSuccessCounter`
// 在只读模式下,检测器不会更新底层过滤器的状态,并在黑洞状态未知时拒绝请求
// 这对于专门用于 AutoNAT 等服务的 Swarm 很有用,在这些服务中我们关心准确报告对等点的可达性
//
// 黑洞过滤在对等点拨号级别完成,以确保用于检测黑洞状态变化的定期探测实际被拨号,而不会因为拨号优先级逻辑而被跳过
type blackHoleDetector struct {
	// udp 是 UDP 黑洞检测器
	udp *BlackHoleSuccessCounter
	// ipv6 是 IPv6 黑洞检测器
	ipv6 *BlackHoleSuccessCounter
	// mt 是指标追踪器
	mt MetricsTracer
	// readOnly 表示是否为只读模式
	readOnly bool
}

// FilterAddrs 过滤对等点的地址,移除黑洞地址
// 参数:
//   - addrs: []ma.Multiaddr 要过滤的地址列表
//
// 返回值:
//   - []ma.Multiaddr 有效的地址列表
//   - []ma.Multiaddr 被黑洞的地址列表
func (d *blackHoleDetector) FilterAddrs(addrs []ma.Multiaddr) (valid []ma.Multiaddr, blackHoled []ma.Multiaddr) {
	hasUDP, hasIPv6 := false, false
	for _, a := range addrs {
		if !manet.IsPublicAddr(a) {
			continue
		}
		if isProtocolAddr(a, ma.P_UDP) {
			hasUDP = true
		}
		if isProtocolAddr(a, ma.P_IP6) {
			hasIPv6 = true
		}
	}

	udpRes := blackHoleStateAllowed
	if d.udp != nil && hasUDP {
		udpRes = d.getFilterState(d.udp)
		d.trackMetrics(d.udp)
	}

	ipv6Res := blackHoleStateAllowed
	if d.ipv6 != nil && hasIPv6 {
		ipv6Res = d.getFilterState(d.ipv6)
		d.trackMetrics(d.ipv6)
	}

	blackHoled = make([]ma.Multiaddr, 0, len(addrs))
	return ma.FilterAddrs(
		addrs,
		func(a ma.Multiaddr) bool {
			if !manet.IsPublicAddr(a) {
				return true
			}
			// 在探测时允许所有 UDP 地址,不考虑 IPv6 黑洞状态
			if udpRes == blackHoleStateProbing && isProtocolAddr(a, ma.P_UDP) {
				return true
			}
			// 在探测时允许所有 IPv6 地址,不考虑 UDP 黑洞状态
			if ipv6Res == blackHoleStateProbing && isProtocolAddr(a, ma.P_IP6) {
				return true
			}

			if udpRes == blackHoleStateBlocked && isProtocolAddr(a, ma.P_UDP) {
				blackHoled = append(blackHoled, a)
				return false
			}
			if ipv6Res == blackHoleStateBlocked && isProtocolAddr(a, ma.P_IP6) {
				blackHoled = append(blackHoled, a)
				return false
			}
			return true
		},
	), blackHoled
}

// RecordResult 更新地址相关的 BlackHoleSuccessCounter 状态
// 参数:
//   - addr: ma.Multiaddr 要记录结果的地址
//   - success: bool 是否成功
func (d *blackHoleDetector) RecordResult(addr ma.Multiaddr, success bool) {
	if d.readOnly || !manet.IsPublicAddr(addr) {
		return
	}
	if d.udp != nil && isProtocolAddr(addr, ma.P_UDP) {
		d.udp.RecordResult(success)
		d.trackMetrics(d.udp)
	}
	if d.ipv6 != nil && isProtocolAddr(addr, ma.P_IP6) {
		d.ipv6.RecordResult(success)
		d.trackMetrics(d.ipv6)
	}
}

// getFilterState 获取过滤器的状态
// 参数:
//   - f: *BlackHoleSuccessCounter 要获取状态的过滤器
//
// 返回值:
//   - BlackHoleState 过滤器的状态
func (d *blackHoleDetector) getFilterState(f *BlackHoleSuccessCounter) BlackHoleState {
	if d.readOnly {
		if f.State() != blackHoleStateAllowed {
			return blackHoleStateBlocked
		}
		return blackHoleStateAllowed
	}
	return f.HandleRequest()
}

// trackMetrics 追踪过滤器的指标
// 参数:
//   - f: *BlackHoleSuccessCounter 要追踪指标的过滤器
func (d *blackHoleDetector) trackMetrics(f *BlackHoleSuccessCounter) {
	if d.readOnly || d.mt == nil {
		return
	}
	// 仅在非只读状态下追踪指标
	info := f.info()
	d.mt.UpdatedBlackHoleSuccessCounter(info.name, info.state, info.nextProbeAfter, info.successFraction)
}
