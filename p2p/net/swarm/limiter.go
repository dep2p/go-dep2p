package swarm

import (
	"context"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/transport"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// dialJob 表示一个拨号任务
type dialJob struct {
	// addr 是要拨号的多地址
	addr ma.Multiaddr
	// peer 是要拨号的对等节点ID
	peer peer.ID
	// ctx 是拨号的上下文
	ctx context.Context
	// resp 是用于发送拨号更新的通道
	resp chan transport.DialUpdate
	// timeout 是拨号超时时间
	timeout time.Duration
}

// cancelled 检查拨号任务是否已取消
// 返回值:
//   - bool 如果任务已取消返回 true,否则返回 false
func (dj *dialJob) cancelled() bool {
	return dj.ctx.Err() != nil
}

// dialLimiter 用于限制并发拨号数量
type dialLimiter struct {
	// lk 用于保护并发访问的互斥锁
	lk sync.Mutex

	// fdConsuming 当前正在消耗的文件描述符数量
	fdConsuming int
	// fdLimit 文件描述符的最大限制
	fdLimit int
	// waitingOnFd 等待文件描述符的拨号任务队列
	waitingOnFd []*dialJob

	// dialFunc 执行实际拨号的函数
	dialFunc dialfunc

	// activePerPeer 每个对等节点当前活跃的拨号数
	activePerPeer map[peer.ID]int
	// perPeerLimit 每个对等节点的最大并发拨号数限制
	perPeerLimit int
	// waitingOnPeerLimit 每个对等节点等待拨号的任务队列
	waitingOnPeerLimit map[peer.ID][]*dialJob
}

// dialfunc 定义拨号函数的类型
type dialfunc func(context.Context, peer.ID, ma.Multiaddr, chan<- transport.DialUpdate) (transport.CapableConn, error)

// newDialLimiter 创建一个新的拨号限制器
// 参数:
//   - df: dialfunc 拨号函数
//
// 返回值:
//   - *dialLimiter 新创建的拨号限制器
func newDialLimiter(df dialfunc) *dialLimiter {
	fd := ConcurrentFdDials
	if env := os.Getenv("LIBP2P_SWARM_FD_LIMIT"); env != "" {
		if n, err := strconv.ParseInt(env, 10, 32); err == nil {
			fd = int(n)
		}
	}
	return newDialLimiterWithParams(df, fd, DefaultPerPeerRateLimit)
}

// newDialLimiterWithParams 使用指定参数创建拨号限制器
// 参数:
//   - df: dialfunc 拨号函数
//   - fdLimit: int 文件描述符限制
//   - perPeerLimit: int 每个对等节点的限制
//
// 返回值:
//   - *dialLimiter 新创建的拨号限制器
func newDialLimiterWithParams(df dialfunc, fdLimit, perPeerLimit int) *dialLimiter {
	return &dialLimiter{
		fdLimit:            fdLimit,
		perPeerLimit:       perPeerLimit,
		waitingOnPeerLimit: make(map[peer.ID][]*dialJob),
		activePerPeer:      make(map[peer.ID]int),
		dialFunc:           df,
	}
}

// freeFDToken 释放一个文件描述符令牌,如果有等待的拨号任务则调度下一个
func (dl *dialLimiter) freeFDToken() {
	log.Debugf("[限制器] 释放文件描述符令牌; 等待数: %d; 使用中: %d", len(dl.waitingOnFd), dl.fdConsuming)
	dl.fdConsuming--

	for len(dl.waitingOnFd) > 0 {
		next := dl.waitingOnFd[0]
		dl.waitingOnFd[0] = nil // 清理内存
		dl.waitingOnFd = dl.waitingOnFd[1:]

		if len(dl.waitingOnFd) == 0 {
			// 清理内存
			dl.waitingOnFd = nil
		}

		// 跳过已取消的拨号而不是启动新的 goroutine
		if next.cancelled() {
			dl.freePeerToken(next)
			continue
		}
		dl.fdConsuming++

		// 此时我们已经有了 activePerPeer 令牌,可以直接拨号
		go dl.executeDial(next)
		return
	}
}

// freePeerToken 释放对等节点令牌
// 参数:
//   - dj: *dialJob 要释放令牌的拨号任务
func (dl *dialLimiter) freePeerToken(dj *dialJob) {
	log.Debugf("[限制器] 释放对等节点令牌; 对等节点 %s; 地址: %s; 该对等节点活跃数: %d; 等待对等节点限制数: %d",
		dj.peer, dj.addr, dl.activePerPeer[dj.peer], len(dl.waitingOnPeerLimit[dj.peer]))
	// 按获取令牌的相反顺序释放令牌
	dl.activePerPeer[dj.peer]--
	if dl.activePerPeer[dj.peer] == 0 {
		delete(dl.activePerPeer, dj.peer)
	}

	waitlist := dl.waitingOnPeerLimit[dj.peer]
	for len(waitlist) > 0 {
		next := waitlist[0]
		waitlist[0] = nil // 清理内存
		waitlist = waitlist[1:]

		if len(waitlist) == 0 {
			delete(dl.waitingOnPeerLimit, next.peer)
		} else {
			dl.waitingOnPeerLimit[next.peer] = waitlist
		}

		if next.cancelled() {
			continue
		}

		dl.activePerPeer[next.peer]++ // 我们仍然需要这个令牌

		dl.addCheckFdLimit(next)
		return
	}
}

// finishedDial 完成拨号任务时调用
// 参数:
//   - dj: *dialJob 已完成的拨号任务
func (dl *dialLimiter) finishedDial(dj *dialJob) {
	dl.lk.Lock()
	defer dl.lk.Unlock()
	if dl.shouldConsumeFd(dj.addr) {
		dl.freeFDToken()
	}

	dl.freePeerToken(dj)
}

// shouldConsumeFd 检查给定地址是否需要消耗文件描述符
// 参数:
//   - addr: ma.Multiaddr 要检查的地址
//
// 返回值:
//   - bool 如果需要消耗文件描述符返回 true
func (dl *dialLimiter) shouldConsumeFd(addr ma.Multiaddr) bool {
	// 对于中继地址,我们暂时不消耗文件描述符,因为它们会在中继传输实际拨号中继服务器时消耗
	// 该拨号调用也会通过限制器,使用中继服务器的非中继地址
	_, err := addr.ValueForProtocol(ma.P_CIRCUIT)

	isRelay := err == nil

	return !isRelay && isFdConsumingAddr(addr)
}

// addCheckFdLimit 检查文件描述符限制并添加拨号任务
// 参数:
//   - dj: *dialJob 要添加的拨号任务
func (dl *dialLimiter) addCheckFdLimit(dj *dialJob) {
	if dl.shouldConsumeFd(dj.addr) {
		if dl.fdConsuming >= dl.fdLimit {
			log.Debugf("[限制器] 拨号因等待文件描述符令牌而阻塞; 对等节点: %s; 地址: %s; 使用中: %d; "+
				"限制: %d; 等待数: %d", dj.peer, dj.addr, dl.fdConsuming, dl.fdLimit, len(dl.waitingOnFd))
			dl.waitingOnFd = append(dl.waitingOnFd, dj)
			return
		}

		log.Debugf("[限制器] 获取文件描述符令牌: 对等节点: %s; 地址: %s; 之前使用数: %d",
			dj.peer, dj.addr, dl.fdConsuming)
		// 获取令牌
		dl.fdConsuming++
	}

	log.Debugf("[限制器] 执行拨号; 对等节点: %s; 地址: %s; 文件描述符使用数: %d; 等待数: %d",
		dj.peer, dj.addr, dl.fdConsuming, len(dl.waitingOnFd))
	go dl.executeDial(dj)
}

// addCheckPeerLimit 检查对等节点限制并添加拨号任务
// 参数:
//   - dj: *dialJob 要添加的拨号任务
func (dl *dialLimiter) addCheckPeerLimit(dj *dialJob) {
	if dl.activePerPeer[dj.peer] >= dl.perPeerLimit {
		log.Debugf("[限制器] 拨号因等待对等节点限制而阻塞; 对等节点: %s; 地址: %s; 活跃数: %d; "+
			"对等节点限制: %d; 等待数: %d", dj.peer, dj.addr, dl.activePerPeer[dj.peer], dl.perPeerLimit,
			len(dl.waitingOnPeerLimit[dj.peer]))
		wlist := dl.waitingOnPeerLimit[dj.peer]
		dl.waitingOnPeerLimit[dj.peer] = append(wlist, dj)
		return
	}
	dl.activePerPeer[dj.peer]++

	dl.addCheckFdLimit(dj)
}

// AddDialJob 尝试获取启动给定拨号任务所需的令牌
// 如果获取到所有需要的令牌,则立即开始拨号,否则将其放入请求令牌的等待列表
// 参数:
//   - dj: *dialJob 要添加的拨号任务
func (dl *dialLimiter) AddDialJob(dj *dialJob) {
	dl.lk.Lock()
	defer dl.lk.Unlock()

	log.Debugf("[限制器] 通过限制器添加拨号任务: %v", dj.addr)
	dl.addCheckPeerLimit(dj)
}

// clearAllPeerDials 清除指定对等节点的所有拨号任务
// 参数:
//   - p: peer.ID 要清除的对等节点ID
func (dl *dialLimiter) clearAllPeerDials(p peer.ID) {
	dl.lk.Lock()
	defer dl.lk.Unlock()
	delete(dl.waitingOnPeerLimit, p)
	log.Debugf("[限制器] 清除所有对等节点拨号: %v", p)
	// 注意: waitingOnFd 列表不需要在此清理,因为我们会在遇到它们时移除它们,因为此时它们已被"取消"
}

// executeDial 调用拨号函数,并在完成时通过响应通道报告结果
// 一旦发送响应,它还会释放在拨号期间持有的所有令牌
// 参数:
//   - j: *dialJob 要执行的拨号任务
func (dl *dialLimiter) executeDial(j *dialJob) {
	defer dl.finishedDial(j)
	if j.cancelled() {
		return
	}

	dctx, cancel := context.WithTimeout(j.ctx, j.timeout)
	defer cancel()

	con, err := dl.dialFunc(dctx, j.peer, j.addr, j.resp)
	kind := transport.UpdateKindDialSuccessful
	if err != nil {
		kind = transport.UpdateKindDialFailed
	}
	select {
	case j.resp <- transport.DialUpdate{Kind: kind, Conn: con, Addr: j.addr, Err: err}:
	case <-j.ctx.Done():
		if con != nil {
			con.Close()
		}
	}
}
