package swarm

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	tpt "github.com/dep2p/go-dep2p/core/transport"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

// dialRequest 是用于请求拨号到与工作循环相关联的对等节点的结构
type dialRequest struct {
	// ctx 是可用于请求的上下文。如果有另一个并发请求,任何并发请求的 ctx 都可用于拨号到对等节点的地址。
	// 同时连接请求的 ctx 优先级高于普通请求
	ctx context.Context
	// resch 是用于发送此查询响应的通道
	resch chan dialResponse
}

// dialResponse 是在请求的 resch 通道上发送给 dialRequests 的响应
type dialResponse struct {
	// conn 是成功时与对等节点的连接
	conn *Conn
	// err 是拨号对等节点时的错误,连接成功时为 nil
	err error
}

// pendRequest 用于跟踪 dialRequest 的进度
type pendRequest struct {
	// req 是原始的 dialRequest
	req dialRequest
	// err 包含所有失败拨号的错误
	err *DialError
	// addrs 是我们正在等待挂起拨号的地址。在创建时,addrs 初始化为对等节点的所有地址。
	// 在拨号失败时,该地址从映射中删除并更新 err。
	// 在拨号成功时,dialRequest 完成并发送带有连接的响应
	addrs map[string]struct{}
}

// addrDial 跟踪到特定多地址的拨号
type addrDial struct {
	// addr 是拨号的地址
	addr ma.Multiaddr
	// ctx 是用于拨号地址的上下文
	ctx context.Context
	// conn 是成功时建立的连接
	conn *Conn
	// err 是拨号地址时的错误
	err error
	// dialed 表示我们是否已触发到该地址的拨号
	dialed bool
	// createdAt 是创建此结构的时间
	createdAt time.Time
	// dialRankingDelay 是由排名逻辑引入的拨号此地址的延迟
	dialRankingDelay time.Duration
	// expectedTCPUpgradeTime 是预期安全升级完成的时间
	expectedTCPUpgradeTime time.Time
}

// dialWorker 同步到对等节点的并发拨号。
// 它确保我们最多只对对等节点的地址进行一次拨号
type dialWorker struct {
	s    *Swarm
	peer peer.ID
	// reqch 用于向工作程序发送拨号请求。关闭 reqch 以结束工作程序循环
	reqch <-chan dialRequest
	// pendingRequests 是挂起请求的集合
	pendingRequests map[*pendRequest]struct{}
	// trackedDials 跟踪到对等节点地址的拨号。
	// 此处的条目用于确保我们最多只拨号一次地址
	trackedDials map[string]*addrDial
	// resch 用于接收对等节点地址拨号的响应。
	resch chan tpt.DialUpdate

	connected bool // 当成功建立连接时为 true

	// 用于测试
	wg sync.WaitGroup
	cl Clock
}

// newDialWorker 创建一个新的拨号工作器
// 参数:
//   - s: *Swarm swarm 实例
//   - p: peer.ID 对等节点ID
//   - reqch: <-chan dialRequest 拨号请求通道
//   - cl: Clock 时钟接口
//
// 返回值:
//   - *dialWorker 新创建的拨号工作器
func newDialWorker(s *Swarm, p peer.ID, reqch <-chan dialRequest, cl Clock) *dialWorker {
	// 如果未提供时钟实现,使用真实时钟
	if cl == nil {
		cl = RealClock{}
	}
	// 返回初始化的拨号工作器
	return &dialWorker{
		s:               s,
		peer:            p,
		reqch:           reqch,
		pendingRequests: make(map[*pendRequest]struct{}),
		trackedDials:    make(map[string]*addrDial),
		resch:           make(chan tpt.DialUpdate),
		cl:              cl,
	}
}

// loop 实现核心拨号工作器循环
// 请求在 w.reqch 上接收
// 当 w.reqch 关闭时循环退出
func (w *dialWorker) loop() {
	// 增加等待组计数
	w.wg.Add(1)
	// 函数退出时减少等待组计数
	defer w.wg.Done()
	// 清除所有对等节点的拨号限制
	defer w.s.limiter.clearAllPeerDials(w.peer)

	// dq 用于调节对对等节点不同地址的拨号
	dq := newDialQueue()
	// dialsInFlight 跟踪正在进行的拨号数量
	dialsInFlight := 0

	// 记录开始时间
	startTime := w.cl.Now()
	// 创建拨号定时器
	dialTimer := w.cl.InstantTimer(startTime.Add(math.MaxInt64))
	defer dialTimer.Stop()

	timerRunning := true
	// scheduleNextDial 更新触发下一次拨号的定时器
	scheduleNextDial := func() {
		// 如果定时器正在运行,尝试停止它
		if timerRunning && !dialTimer.Stop() {
			<-dialTimer.Ch()
		}
		timerRunning = false
		if dq.Len() > 0 {
			if dialsInFlight == 0 && !w.connected {
				// 如果没有正在进行的拨号,立即触发下一次拨号
				dialTimer.Reset(startTime)
			} else {
				// 计算下一次拨号的时间
				resetTime := startTime.Add(dq.top().Delay)
				// 检查所有跟踪的拨号,找到最晚的预期 TCP 升级时间
				for _, ad := range w.trackedDials {
					if !ad.expectedTCPUpgradeTime.IsZero() && ad.expectedTCPUpgradeTime.After(resetTime) {
						resetTime = ad.expectedTCPUpgradeTime
					}
				}
				dialTimer.Reset(resetTime)
			}
			timerRunning = true
		}
	}

	// totalDials 用于跟踪此工作器进行的拨号总数,用于指标统计
	totalDials := 0
loop:
	for {
		// 循环有三个部分:
		//  1. 在 w.reqch 上接收输入请求
		//     如果没有合适的连接,创建 pendRequest 对象跟踪 dialRequest 并将地址添加到 dq
		//  2. 根据延迟逻辑在适当的时间间隔拨号 dialQueue 中的地址
		//     在 w.resch 上接收这些拨号完成的通知
		//  3. 在 w.resch 上接收拨号响应
		//     收到响应时,更新对此地址拨号感兴趣的 pendRequests

		select {
		case req, ok := <-w.reqch:
			if !ok {
				// 请求通道已关闭,记录指标并退出
				if w.s.metricsTracer != nil {
					w.s.metricsTracer.DialCompleted(w.connected, totalDials, time.Since(startTime))
				}
				return
			}
			// 收到新请求,如果没有合适的连接,使用 pendRequest 跟踪此 dialRequest
			// 将与此请求相关的对等节点地址排队到 dq 中,并跟踪相关地址的拨号

			// 检查是否已有可用连接
			c := w.s.bestAcceptableConnToPeer(req.ctx, w.peer)
			if c != nil {
				req.resch <- dialResponse{conn: c}
				continue loop
			}

			// 获取要拨号的地址列表
			addrs, addrErrs, err := w.s.addrsForDial(req.ctx, w.peer)
			if err != nil {
				req.resch <- dialResponse{
					err: &DialError{
						Peer:       w.peer,
						DialErrors: addrErrs,
						Cause:      err,
					}}
				continue loop
			}

			// 从 swarm 的 dialRanker 获取拨号这些地址的延迟
			simConnect, _, _ := network.GetSimultaneousConnect(req.ctx)
			addrRanking := w.rankAddrs(addrs, simConnect)
			addrDelay := make(map[string]time.Duration, len(addrRanking))

			// 创建挂起请求对象
			pr := &pendRequest{
				req:   req,
				addrs: make(map[string]struct{}, len(addrRanking)),
				err:   &DialError{Peer: w.peer, DialErrors: addrErrs},
			}
			for _, adelay := range addrRanking {
				pr.addrs[string(adelay.Addr.Bytes())] = struct{}{}
				addrDelay[string(adelay.Addr.Bytes())] = adelay.Delay
			}

			// 检查对任何地址的拨号是否已完成
			// 如果出错,在 pr 中记录错误
			// 如果成功,用连接响应
			// 如果挂起,将它们添加到 tojoin
			// 如果之前没有见过任何地址,将它们添加到 todial
			var todial []ma.Multiaddr
			var tojoin []*addrDial

			for _, adelay := range addrRanking {
				ad, ok := w.trackedDials[string(adelay.Addr.Bytes())]
				if !ok {
					todial = append(todial, adelay.Addr)
					continue
				}

				if ad.conn != nil {
					// 到此地址的拨号成功,完成请求
					req.resch <- dialResponse{conn: ad.conn}
					continue loop
				}

				if ad.err != nil {
					// 到此地址的拨号出错,累积错误
					pr.err.recordErr(ad.addr, ad.err)
					delete(pr.addrs, string(ad.addr.Bytes()))
					continue
				}

				// 拨号仍在挂起,添加到 join 列表
				tojoin = append(tojoin, ad)
			}

			if len(todial) == 0 && len(tojoin) == 0 {
				// 所有请求适用的地址都已拨号,且都出错了
				pr.err.Cause = ErrAllDialsFailed
				req.resch <- dialResponse{err: pr.err}
				continue loop
			}

			// 请求有一些挂起或新的拨号
			w.pendingRequests[pr] = struct{}{}

			for _, ad := range tojoin {
				if !ad.dialed {
					// 还没有拨号此地址,更新 ad.ctx 以正确设置同时连接值
					if simConnect, isClient, reason := network.GetSimultaneousConnect(req.ctx); simConnect {
						if simConnect, _, _ := network.GetSimultaneousConnect(ad.ctx); !simConnect {
							ad.ctx = network.WithSimultaneousConnect(ad.ctx, isClient, reason)
							// 更新 dq 中的元素以使用同时连接延迟
							dq.UpdateOrAdd(network.AddrDelay{
								Addr:  ad.addr,
								Delay: addrDelay[string(ad.addr.Bytes())],
							})
						}
					}
				}
				// 将请求添加到 addrDial
			}

			if len(todial) > 0 {
				now := time.Now()
				// 这些是新地址,跟踪它们并添加到 dq
				for _, a := range todial {
					w.trackedDials[string(a.Bytes())] = &addrDial{
						addr:      a,
						ctx:       req.ctx,
						createdAt: now,
					}
					dq.Add(network.AddrDelay{Addr: a, Delay: addrDelay[string(a.Bytes())]})
				}
			}
			// 为 dq 的更新设置 dialTimer
			scheduleNextDial()

		case <-dialTimer.Ch():
			// 是时候拨号下一批地址了
			// 不检查从队列收到的地址的延迟,因为如果计时器在延迟之前触发,
			// 这意味着所有正在进行的拨号都已出错,应该拨号下一批地址
			now := time.Now()
			for _, adelay := range dq.NextBatch() {
				// 生成拨号
				ad, ok := w.trackedDials[string(adelay.Addr.Bytes())]
				if !ok {
					log.Errorf("SWARM 错误: trackedDials 中没有地址 %s 的条目", adelay.Addr)
					continue
				}
				ad.dialed = true
				ad.dialRankingDelay = now.Sub(ad.createdAt)
				err := w.s.dialNextAddr(ad.ctx, w.peer, ad.addr, w.resch)
				if err != nil {
					// 在尝试拨号之前出错
					// 这在退避或黑洞的情况下发生
					w.dispatchError(ad, err)
				} else {
					// 拨号成功,更新正在进行的拨号数
					dialsInFlight++
					totalDials++
				}
			}
			timerRunning = false
			// 安排更多拨号
			scheduleNextDial()

		case res := <-w.resch:
			// 到地址的拨号已完成
			// 更新等待此地址的所有请求
			// 成功时完成请求
			// 出错时记录错误

			ad, ok := w.trackedDials[string(res.Addr.Bytes())]
			if !ok {
				log.Errorf("SWARM 错误: trackedDials 中没有地址 %s 的条目", res.Addr)
				if res.Conn != nil {
					res.Conn.Close()
				}
				dialsInFlight--
				continue
			}

			// TCP 连接已建立
			// 在此地址上等待连接升级,然后再进行新的拨号
			if res.Kind == tpt.UpdateKindHandshakeProgressed {
				// 只等待公共地址完成拨号,因为私有拨号无论如何都很快
				if manet.IsPublicAddr(res.Addr) {
					ad.expectedTCPUpgradeTime = w.cl.Now().Add(PublicTCPDelay)
				}
				scheduleNextDial()
				continue
			}
			dialsInFlight--
			ad.expectedTCPUpgradeTime = time.Time{}
			if res.Conn != nil {
				// 得到了一个连接,将其添加到 swarm
				conn, err := w.s.addConn(res.Conn, network.DirOutbound)
				if err != nil {
					// 添加到 swarm 失败
					res.Conn.Close()
					w.dispatchError(ad, err)
					continue loop
				}

				for pr := range w.pendingRequests {
					if _, ok := pr.addrs[string(ad.addr.Bytes())]; ok {
						pr.req.resch <- dialResponse{conn: conn}
						delete(w.pendingRequests, pr)
					}
				}

				ad.conn = conn
				if !w.connected {
					w.connected = true
					if w.s.metricsTracer != nil {
						w.s.metricsTracer.DialRankingDelay(ad.dialRankingDelay)
					}
				}

				continue loop
			}

			// 一定是错误 -- 如果适用则添加退避并分发
			// ErrDialRefusedBlackHole 不应该出现在这里,只是一个安全检查
			if res.Err != ErrDialRefusedBlackHole && res.Err != context.Canceled && !w.connected {
				// 为了与旧拨号器行为保持一致,只在没有成功连接时添加退避
				w.s.backf.AddBackoff(w.peer, res.Addr)
			} else if res.Err == ErrDialRefusedBlackHole {
				log.Errorf("SWARM 错误: 在拨号对等节点 %s 到地址 %s 时出现意外的 ErrDialRefusedBlackHole",
					w.peer, res.Addr)
			}

			w.dispatchError(ad, res.Err)
			// 只在出错时安排下一次拨号
			// 如果在成功时 scheduleNextDial,最终会多拨号一次,
			// 因为最后一次成功的拨号会生成一次更多的拨号
			scheduleNextDial()
		}
	}
}

// dispatchError 将错误分发到特定的地址拨号
// 参数:
//   - ad: *addrDial 地址拨号对象
//   - err: error 要分发的错误
func (w *dialWorker) dispatchError(ad *addrDial, err error) {
	ad.err = err
	for pr := range w.pendingRequests {
		// 累积错误
		if _, ok := pr.addrs[string(ad.addr.Bytes())]; ok {
			pr.err.recordErr(ad.addr, err)
			delete(pr.addrs, string(ad.addr.Bytes()))
			if len(pr.addrs) == 0 {
				// 所有地址都出错了,分发拨号错误
				// 但首先做最后一次检查,以防稍后启动的同时拨号添加了新的可接受地址并已建立连接
				c := w.s.bestAcceptableConnToPeer(pr.req.ctx, w.peer)
				if c != nil {
					pr.req.resch <- dialResponse{conn: c}
				} else {
					pr.err.Cause = ErrAllDialsFailed
					pr.req.resch <- dialResponse{err: pr.err}
				}
				delete(w.pendingRequests, pr)
			}
		}
	}

	// 如果是退避,清除地址拨号,这样它就不会抑制新的拨号请求
	// 这对于支持主动监听场景是必要的,在这种场景中,
	// 当另一个拨号正在进行时进来新的拨号,并且需要在不受拨号退避抑制的情况下进行直接连接
	if err == ErrDialBackoff {
		delete(w.trackedDials, string(ad.addr.Bytes()))
	}
}

// rankAddrs 对拨号的地址进行排名
// 参数:
//   - addrs: []ma.Multiaddr 要排名的地址列表
//   - isSimConnect: bool 是否为同时连接请求
//
// 返回值:
//   - []network.AddrDelay 排序后的地址延迟列表
func (w *dialWorker) rankAddrs(addrs []ma.Multiaddr, isSimConnect bool) []network.AddrDelay {
	if isSimConnect {
		return NoDelayDialRanker(addrs)
	}
	return w.s.dialRanker(addrs)
}

// dialQueue 是用于安排拨号的优先级队列
type dialQueue struct {
	// q 包含按延迟排序的拨号
	q []network.AddrDelay
}

// newDialQueue 返回一个新的 dialQueue
// 返回值:
//   - *dialQueue 新创建的拨号队列
func newDialQueue() *dialQueue {
	return &dialQueue{
		q: make([]network.AddrDelay, 0, 16),
	}
}

// Add 向 dialQueue 添加一个新元素
// 要更新元素请使用 UpdateOrAdd
// 参数:
//   - adelay: network.AddrDelay 要添加的地址延迟
func (dq *dialQueue) Add(adelay network.AddrDelay) {
	for i := dq.Len() - 1; i >= 0; i-- {
		if dq.q[i].Delay <= adelay.Delay {
			// 在位置 i+1 插入
			dq.q = append(dq.q, network.AddrDelay{}) // 扩展切片
			copy(dq.q[i+2:], dq.q[i+1:])
			dq.q[i+1] = adelay
			return
		}
	}
	// 在位置 0 插入
	dq.q = append(dq.q, network.AddrDelay{}) // 扩展切片
	copy(dq.q[1:], dq.q[0:])
	dq.q[0] = adelay
}

// UpdateOrAdd 将地址为 adelay.Addr 的元素更新为新的延迟
// 在打洞时很有用
// 参数:
//   - adelay: network.AddrDelay 要更新或添加的地址延迟
func (dq *dialQueue) UpdateOrAdd(adelay network.AddrDelay) {
	for i := 0; i < dq.Len(); i++ {
		if dq.q[i].Addr.Equal(adelay.Addr) {
			if dq.q[i].Delay == adelay.Delay {
				// 现有元素相同,无需操作
				return
			}
			// 删除元素
			copy(dq.q[i:], dq.q[i+1:])
			dq.q = dq.q[:len(dq.q)-1]
		}
	}
	dq.Add(adelay)
}

// NextBatch 返回队列中具有最高优先级的所有元素
// 返回值:
//   - []network.AddrDelay 最高优先级的地址延迟列表
func (dq *dialQueue) NextBatch() []network.AddrDelay {
	if dq.Len() == 0 {
		return nil
	}

	// i 是第二高优先级元素的索引
	var i int
	for i = 0; i < dq.Len(); i++ {
		if dq.q[i].Delay != dq.q[0].Delay {
			break
		}
	}
	res := dq.q[:i]
	dq.q = dq.q[i:]
	return res
}

// top 返回队列的顶部元素
// 返回值:
//   - network.AddrDelay 队列顶部的地址延迟
func (dq *dialQueue) top() network.AddrDelay {
	return dq.q[0]
}

// Len 返回队列中的元素数
// 返回值:
//   - int 队列中的元素数量
func (dq *dialQueue) Len() int {
	return len(dq.q)
}
