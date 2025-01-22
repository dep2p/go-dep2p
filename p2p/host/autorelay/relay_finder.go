package autorelay

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/dep2p/go-dep2p/core/event"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	basic "github.com/dep2p/go-dep2p/p2p/host/basic"
	"github.com/dep2p/go-dep2p/p2p/host/eventbus"
	circuitv2 "github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/client"
	circuitv2_proto "github.com/dep2p/go-dep2p/p2p/protocol/circuitv2/proto"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

// 中继协议ID
const protoIDv2 = circuitv2_proto.ProtoIDv2Hop

// 术语说明:
// Candidate(候选节点): 当我们连接到一个支持中继协议的节点时,我们称其为候选节点,并考虑将其用作中继。
// Relay(中继节点): 从候选节点列表中,我们选择一个节点作为中继。
// 目前,我们只是随机选择一个候选节点,但我们可以在这里采用更复杂的选择策略(例如考虑RTT)。

const (
	rsvpRefreshInterval = time.Minute     // 预约刷新间隔
	rsvpExpirationSlack = 2 * time.Minute // 预约过期宽限期
	autorelayTag        = "autorelay"     // 自动中继标签
)

// candidate 候选节点结构体
type candidate struct {
	added           time.Time     // 添加时间
	supportsRelayV2 bool          // 是否支持中继v2协议
	ai              peer.AddrInfo // 节点地址信息
}

// relayFinder 当检测到NAT时使用中继进行连接的主机
type relayFinder struct {
	bootTime time.Time        // 启动时间
	host     *basic.BasicHost // 基础主机实例

	conf *config // 配置对象

	refCount sync.WaitGroup // 引用计数器

	ctxCancel   context.CancelFunc // 上下文取消函数
	ctxCancelMx sync.Mutex         // 上下文取消锁

	peerSource PeerSource // 节点源函数

	candidateFound             chan struct{}          // 每当找到新的中继候选节点时接收信号
	candidateMx                sync.Mutex             // 候选节点锁
	candidates                 map[peer.ID]*candidate // 候选节点映射
	backoff                    map[peer.ID]time.Time  // 退避时间映射
	maybeConnectToRelayTrigger chan struct{}          // 容量为1的通道,可能需要连接到中继时触发
	// 任何可能导致我们需要新候选节点的事件发生时触发。
	// 这可能是:
	// * 与中继断开连接
	// * 尝试获取当前候选节点的预约失败
	// * 由于候选节点年龄而被删除
	maybeRequestNewCandidates chan struct{} // 容量为1的通道

	relayUpdated chan struct{} // 中继更新通道

	relayMx sync.Mutex                         // 中继锁
	relays  map[peer.ID]*circuitv2.Reservation // 中继预约映射

	cachedAddrs       []ma.Multiaddr // 缓存的地址列表
	cachedAddrsExpiry time.Time      // 缓存地址过期时间

	// 触发运行 `runScheduledWork` 的通道
	triggerRunScheduledWork chan struct{} // 触发运行计划任务的通道
	metricsTracer           MetricsTracer // 指标追踪器
}

// 已运行错误
var errAlreadyRunning = errors.New("中继查找器已在运行")

// newRelayFinder 创建新的中继查找器
// 参数:
//   - host: *basic.BasicHost 基础主机实例
//   - peerSource: PeerSource 节点源函数
//   - conf: *config 配置对象
//
// 返回值:
//   - *relayFinder: 返回中继查找器实例
func newRelayFinder(host *basic.BasicHost, peerSource PeerSource, conf *config) *relayFinder {
	// 检查节点源函数是否为空
	if peerSource == nil {
		panic("无法创建新的中继查找器。需要一个节点源函数或静态中继列表。请参考 `dep2p.EnableAutoRelay` 的文档")
	}

	// 创建并返回中继查找器实例
	return &relayFinder{
		bootTime:                   conf.clock.Now(),                          // 设置启动时间
		host:                       host,                                      // 设置主机
		conf:                       conf,                                      // 设置配置
		peerSource:                 peerSource,                                // 设置节点源
		candidates:                 make(map[peer.ID]*candidate),              // 初始化候选节点映射
		backoff:                    make(map[peer.ID]time.Time),               // 初始化退避时间映射
		candidateFound:             make(chan struct{}, 1),                    // 初始化候选节点发现通道
		maybeConnectToRelayTrigger: make(chan struct{}, 1),                    // 初始化中继连接触发通道
		maybeRequestNewCandidates:  make(chan struct{}, 1),                    // 初始化新候选请求通道
		triggerRunScheduledWork:    make(chan struct{}, 1),                    // 初始化计划任务触发通道
		relays:                     make(map[peer.ID]*circuitv2.Reservation),  // 初始化中继预约映射
		relayUpdated:               make(chan struct{}, 1),                    // 初始化中继更新通道
		metricsTracer:              &wrappedMetricsTracer{conf.metricsTracer}, // 设置指标追踪器
	}
}

// scheduledWorkTimes 定义计划任务时间结构体
type scheduledWorkTimes struct {
	leastFrequentInterval       time.Duration // 最小频率间隔
	nextRefresh                 time.Time     // 下次刷新时间
	nextBackoff                 time.Time     // 下次退避时间
	nextOldCandidateCheck       time.Time     // 下次旧候选检查时间
	nextAllowedCallToPeerSource time.Time     // 下次允许调用节点源时间
}

// cleanupDisconnectedPeers 清理断开连接的节点
// 参数:
//   - ctx: context.Context 上下文对象
func (rf *relayFinder) cleanupDisconnectedPeers(ctx context.Context) {
	// 订阅节点连接状态变更事件
	subConnectedness, err := rf.host.EventBus().Subscribe(new(event.EvtPeerConnectednessChanged), eventbus.Name("autorelay (relay finder)"), eventbus.BufSize(32))
	if err != nil {
		return
	}
	defer subConnectedness.Close() // 延迟关闭订阅

	for {
		select {
		case <-ctx.Done(): // 上下文取消
			return
		case ev, ok := <-subConnectedness.Out(): // 接收事件
			if !ok {
				return
			}
			evt := ev.(event.EvtPeerConnectednessChanged)  // 类型断言
			if evt.Connectedness != network.NotConnected { // 如果不是断开连接状态
				continue
			}
			push := false

			rf.relayMx.Lock()            // 加锁
			if rf.usingRelay(evt.Peer) { // 如果正在使用该中继
				log.Debugw("与中继断开连接", "id", evt.Peer) // 记录日志
				delete(rf.relays, evt.Peer)           // 删除中继
				rf.notifyMaybeConnectToRelay()        // 通知可能需要连接中继
				rf.notifyMaybeNeedNewCandidates()     // 通知可能需要新候选
				push = true
			}
			rf.relayMx.Unlock() // 解锁

			if push { // 如果需要推送
				rf.clearCachedAddrsAndSignalAddressChange() // 清除缓存地址并发送地址变更信号
				rf.metricsTracer.ReservationEnded(1)        // 记录预约结束指标
			}
		}
	}
}

// background 后台任务处理
// 参数:
//   - ctx: context.Context 上下文对象
func (rf *relayFinder) background(ctx context.Context) {
	peerSourceRateLimiter := make(chan struct{}, 1) // 创建节点源限流器

	// 启动查找节点协程
	rf.refCount.Add(1)
	go func() {
		defer rf.refCount.Done()
		rf.findNodes(ctx, peerSourceRateLimiter)
	}()

	// 启动处理新候选节点协程
	rf.refCount.Add(1)
	go func() {
		defer rf.refCount.Done()
		rf.handleNewCandidates(ctx)
	}()

	now := rf.conf.clock.Now()                                               // 获取当前时间
	bootDelayTimer := rf.conf.clock.InstantTimer(now.Add(rf.conf.bootDelay)) // 创建启动延迟定时器
	defer bootDelayTimer.Stop()                                              // 延迟停止定时器

	// 计算最小频率间隔
	leastFrequentInterval := rf.conf.minInterval
	if rf.conf.backoff > leastFrequentInterval || leastFrequentInterval == 0 {
		leastFrequentInterval = rf.conf.backoff
	}
	if rf.conf.maxCandidateAge > leastFrequentInterval || leastFrequentInterval == 0 {
		leastFrequentInterval = rf.conf.maxCandidateAge
	}
	if rsvpRefreshInterval > leastFrequentInterval || leastFrequentInterval == 0 {
		leastFrequentInterval = rsvpRefreshInterval
	}

	// 初始化计划任务时间
	scheduledWork := &scheduledWorkTimes{
		leastFrequentInterval:       leastFrequentInterval,
		nextRefresh:                 now.Add(rsvpRefreshInterval),
		nextBackoff:                 now.Add(rf.conf.backoff),
		nextOldCandidateCheck:       now.Add(rf.conf.maxCandidateAge),
		nextAllowedCallToPeerSource: now.Add(-time.Second), // 立即允许
	}

	// 创建工作定时器
	workTimer := rf.conf.clock.InstantTimer(rf.runScheduledWork(ctx, now, scheduledWork, peerSourceRateLimiter))
	defer workTimer.Stop()

	// 启动清理断开连接节点的协程
	go rf.cleanupDisconnectedPeers(ctx)

	// 主循环
	for {
		select {
		case <-rf.candidateFound: // 发现候选节点
			rf.notifyMaybeConnectToRelay()
		case <-bootDelayTimer.Ch(): // 启动延迟到期
			rf.notifyMaybeConnectToRelay()
		case <-rf.relayUpdated: // 中继更新
			rf.clearCachedAddrsAndSignalAddressChange()
		case now := <-workTimer.Ch(): // 工作定时器触发
			nextTime := rf.runScheduledWork(ctx, now, scheduledWork, peerSourceRateLimiter)
			workTimer.Reset(nextTime)
		case <-rf.triggerRunScheduledWork: // 触发运行计划任务
			_ = rf.runScheduledWork(ctx, rf.conf.clock.Now(), scheduledWork, peerSourceRateLimiter)
		case <-ctx.Done(): // 上下文取消
			return
		}
	}
}

// clearCachedAddrsAndSignalAddressChange 清除缓存的地址并发送地址变更信号
func (rf *relayFinder) clearCachedAddrsAndSignalAddressChange() {
	rf.relayMx.Lock()             // 加锁保护缓存地址
	rf.cachedAddrs = nil          // 清空缓存地址
	rf.relayMx.Unlock()           // 解锁
	rf.host.SignalAddressChange() // 通知主机地址变更

	rf.metricsTracer.RelayAddressUpdated() // 更新指标
}

// runScheduledWork 运行计划任务
// 参数:
//   - ctx: context.Context 上下文对象
//   - now: time.Time 当前时间
//   - scheduledWork: *scheduledWorkTimes 计划任务时间
//   - peerSourceRateLimiter: chan<- struct{} 节点源速率限制器
//
// 返回值:
//   - time.Time: 下次运行时间
func (rf *relayFinder) runScheduledWork(ctx context.Context, now time.Time, scheduledWork *scheduledWorkTimes, peerSourceRateLimiter chan<- struct{}) time.Time {
	nextTime := now.Add(scheduledWork.leastFrequentInterval) // 计算下次运行时间

	// 检查是否需要刷新预约
	if now.After(scheduledWork.nextRefresh) {
		scheduledWork.nextRefresh = now.Add(rsvpRefreshInterval) // 更新下次刷新时间
		if rf.refreshReservations(ctx, now) {                    // 刷新预约
			rf.clearCachedAddrsAndSignalAddressChange() // 清除缓存并发送变更信号
		}
	}

	// 检查是否需要清理退避状态
	if now.After(scheduledWork.nextBackoff) {
		scheduledWork.nextBackoff = rf.clearBackoff(now) // 清理退避状态
	}

	// 检查是否需要清理旧候选节点
	if now.After(scheduledWork.nextOldCandidateCheck) {
		scheduledWork.nextOldCandidateCheck = rf.clearOldCandidates(now) // 清理旧候选节点
	}

	// 检查是否可以调用节点源
	if now.After(scheduledWork.nextAllowedCallToPeerSource) {
		select {
		case peerSourceRateLimiter <- struct{}{}: // 尝试获取速率限制令牌
			scheduledWork.nextAllowedCallToPeerSource = now.Add(rf.conf.minInterval) // 更新下次允许调用时间
			if scheduledWork.nextAllowedCallToPeerSource.Before(nextTime) {
				nextTime = scheduledWork.nextAllowedCallToPeerSource // 更新下次运行时间
			}
		default:
		}
	} else {
		// 如果下次允许调用时间早于当前计划时间,更新下次运行时间
		if scheduledWork.nextAllowedCallToPeerSource.Before(nextTime) {
			nextTime = scheduledWork.nextAllowedCallToPeerSource
		}
	}

	// 找出最早需要运行的任务时间
	if scheduledWork.nextRefresh.Before(nextTime) {
		nextTime = scheduledWork.nextRefresh
	}
	if scheduledWork.nextBackoff.Before(nextTime) {
		nextTime = scheduledWork.nextBackoff
	}
	if scheduledWork.nextOldCandidateCheck.Before(nextTime) {
		nextTime = scheduledWork.nextOldCandidateCheck
	}
	if nextTime.Equal(now) {
		// 仅在CI中使用模拟时钟时发生
		nextTime = nextTime.Add(1) // 避免无限循环
	}

	rf.metricsTracer.ScheduledWorkUpdated(scheduledWork) // 更新指标

	return nextTime
}

// clearOldCandidates 清理过期的候选节点
// 参数:
//   - now: time.Time 当前时间
//
// 返回值:
//   - time.Time: 下次运行时间
func (rf *relayFinder) clearOldCandidates(now time.Time) time.Time {
	// 如果没有候选节点,在 rf.conf.maxCandidateAge 后再次运行
	nextTime := now.Add(rf.conf.maxCandidateAge)

	var deleted bool              // 是否有节点被删除的标志
	rf.candidateMx.Lock()         // 加锁保护候选节点映射
	defer rf.candidateMx.Unlock() // 延迟解锁
	for id, cand := range rf.candidates {
		expiry := cand.added.Add(rf.conf.maxCandidateAge) // 计算过期时间
		if expiry.After(now) {                            // 如果未过期
			if expiry.Before(nextTime) {
				nextTime = expiry // 更新下次运行时间
			}
		} else { // 如果已过期
			log.Debugw("deleting candidate due to age", "id", id)
			deleted = true
			rf.removeCandidate(id) // 移除候选节点
		}
	}
	if deleted {
		rf.notifyMaybeNeedNewCandidates() // 如果有节点被删除,通知可能需要新候选节点
	}

	return nextTime
}

// clearBackoff 清理过期的退避状态
// 参数:
//   - now: time.Time 当前时间
//
// 返回值:
//   - time.Time: 下次运行时间
func (rf *relayFinder) clearBackoff(now time.Time) time.Time {
	nextTime := now.Add(rf.conf.backoff) // 计算下次运行时间

	rf.candidateMx.Lock()         // 加锁保护候选节点映射
	defer rf.candidateMx.Unlock() // 延迟解锁
	for id, t := range rf.backoff {
		expiry := t.Add(rf.conf.backoff) // 计算退避过期时间
		if expiry.After(now) {           // 如果未过期
			if expiry.Before(nextTime) {
				nextTime = expiry // 更新下次运行时间
			}
		} else { // 如果已过期
			log.Debugw("removing backoff for node", "id", id)
			delete(rf.backoff, id) // 删除退避状态
		}
	}

	return nextTime
}

// findNodes 从通道接收节点并测试它们是否支持中继功能
// 该方法在公共和私有节点上都会运行
// 它会对旧条目进行垃圾回收,以防止节点溢出
// 这确保了当我们需要查找中继候选节点时,它们是可用的
// 参数:
//   - ctx: context.Context 上下文对象,用于取消操作
//   - peerSourceRateLimiter: <-chan struct{} 节点源速率限制器
func (rf *relayFinder) findNodes(ctx context.Context, peerSourceRateLimiter <-chan struct{}) {
	var peerChan <-chan peer.AddrInfo // 节点信息通道
	var wg sync.WaitGroup             // 等待组,用于等待所有goroutine完成

	for {
		rf.candidateMx.Lock()
		numCandidates := len(rf.candidates) // 获取当前候选节点数量
		rf.candidateMx.Unlock()

		// 如果没有节点通道且候选节点数量小于最小要求
		if peerChan == nil && numCandidates < rf.conf.minCandidates {
			rf.metricsTracer.CandidateLoopState(peerSourceRateLimited) // 更新指标状态

			select {
			case <-peerSourceRateLimiter: // 等待速率限制器
				peerChan = rf.peerSource(ctx, rf.conf.maxCandidates) // 获取新的节点通道
				select {
				case rf.triggerRunScheduledWork <- struct{}{}: // 触发计划任务
				default:
				}
			case <-ctx.Done(): // 上下文取消
				return
			}
		}

		// 更新指标状态
		if peerChan == nil {
			rf.metricsTracer.CandidateLoopState(waitingForTrigger)
		} else {
			rf.metricsTracer.CandidateLoopState(waitingOnPeerChan)
		}

		select {
		case <-rf.maybeRequestNewCandidates: // 收到请求新候选节点的信号
			continue
		case pi, ok := <-peerChan: // 从节点通道接收节点
			if !ok { // 通道已关闭
				wg.Wait()      // 等待所有处理完成
				peerChan = nil // 重置通道
				continue
			}
			log.Debugw("找到节点", "id", pi.ID)
			rf.candidateMx.Lock()
			numCandidates := len(rf.candidates)            // 获取当前候选数量
			backoffStart, isOnBackoff := rf.backoff[pi.ID] // 检查是否在退避状态
			rf.candidateMx.Unlock()
			if isOnBackoff { // 如果节点在退避状态
				log.Debugw("跳过最近获取预约失败的节点", "id", pi.ID, "上次尝试", rf.conf.clock.Since(backoffStart))
				continue
			}
			if numCandidates >= rf.conf.maxCandidates { // 如果候选节点数量已达上限
				log.Debugw("跳过节点。已有足够的候选节点", "id", pi.ID, "当前数量", numCandidates, "最大数量", rf.conf.maxCandidates)
				continue
			}
			rf.refCount.Add(1)
			wg.Add(1)
			go func() { // 启动goroutine处理新节点
				defer rf.refCount.Done()
				defer wg.Done()
				if added := rf.handleNewNode(ctx, pi); added {
					rf.notifyNewCandidate() // 通知发现新候选节点
				}
			}()
		case <-ctx.Done(): // 上下文取消
			rf.metricsTracer.CandidateLoopState(stopped)
			return
		}
	}
}

// notifyMaybeConnectToRelay 通知可能需要连接到中继
func (rf *relayFinder) notifyMaybeConnectToRelay() {
	select {
	case rf.maybeConnectToRelayTrigger <- struct{}{}:
	default:
	}
}

// notifyMaybeNeedNewCandidates 通知可能需要新的候选节点
func (rf *relayFinder) notifyMaybeNeedNewCandidates() {
	select {
	case rf.maybeRequestNewCandidates <- struct{}{}:
	default:
	}
}

// notifyNewCandidate 通知发现新的候选节点
func (rf *relayFinder) notifyNewCandidate() {
	select {
	case rf.candidateFound <- struct{}{}:
	default:
	}
}

// handleNewNode 测试节点是否支持电路v2协议
// 此方法仅在私有节点上运行
// 如果节点支持,则将其添加到候选节点映射中
// 注意:仅支持协议并不能保证我们也能获得预约
// 参数:
//   - ctx: context.Context 上下文对象
//   - pi: peer.AddrInfo 节点地址信息
//
// 返回值:
//   - bool: 是否添加成功
func (rf *relayFinder) handleNewNode(ctx context.Context, pi peer.AddrInfo) (added bool) {
	rf.relayMx.Lock()                  // 加锁保护中继映射
	relayInUse := rf.usingRelay(pi.ID) // 检查节点是否已作为中继使用
	rf.relayMx.Unlock()                // 解锁
	if relayInUse {                    // 如果节点已作为中继使用
		return false // 返回false
	}

	ctx, cancel := context.WithTimeout(ctx, 20*time.Second) // 创建超时上下文
	defer cancel()                                          // 延迟取消上下文
	supportsV2, err := rf.tryNode(ctx, pi)                  // 尝试连接节点并检查协议支持
	if err != nil {                                         // 如果发生错误
		log.Debugf("节点 %s 未被接受为候选: %s", pi.ID, err)
		if err == errProtocolNotSupported { // 如果是协议不支持错误
			rf.metricsTracer.CandidateChecked(false) // 更新指标
		}
		return false // 返回false
	}
	rf.metricsTracer.CandidateChecked(true) // 更新指标

	rf.candidateMx.Lock()                           // 加锁保护候选节点映射
	if len(rf.candidates) > rf.conf.maxCandidates { // 如果候选节点数量超过上限
		rf.candidateMx.Unlock() // 解锁
		return false            // 返回false
	}
	log.Debugw("节点支持中继协议", "peer", pi.ID, "支持电路v2", supportsV2)
	rf.addCandidate(&candidate{ // 添加候选节点
		added:           rf.conf.clock.Now(), // 设置添加时间
		ai:              pi,                  // 设置节点信息
		supportsRelayV2: supportsV2,          // 设置是否支持v2
	})
	rf.candidateMx.Unlock() // 解锁
	return true             // 返回true
}

var errProtocolNotSupported = errors.New("不支持电路v2协议")

// tryNode 检查节点是否支持电路v2协议
// 此方法不修改任何内部状态
// 参数:
//   - ctx: context.Context 上下文对象
//   - pi: peer.AddrInfo 节点地址信息
//
// 返回值:
//   - supportsRelayV2: bool 是否支持中继v2协议
//   - err: error 错误信息
func (rf *relayFinder) tryNode(ctx context.Context, pi peer.AddrInfo) (supportsRelayV2 bool, err error) {
	if err := rf.host.Connect(ctx, pi); err != nil { // 连接到节点
		log.Debugf("连接中继 %s 时出错: %v", pi.ID, err)
		return false, fmt.Errorf("连接中继 %s 时出错: %w", pi.ID, err) // 返回连接错误
	}

	conns := rf.host.Network().ConnsToPeer(pi.ID) // 获取到节点的连接
	for _, conn := range conns {                  // 遍历所有连接
		if isRelayAddr(conn.RemoteMultiaddr()) { // 如果是中继地址
			log.Debugf("不是公共节点")
			return false, errors.New("不是公共节点") // 返回错误
		}
	}

	// 等待至少一个连接完成身份识别,以便检查支持的协议
	ready := make(chan struct{}, 1) // 创建就绪通道
	for _, conn := range conns {    // 遍历所有连接
		go func(conn network.Conn) { // 启动goroutine等待身份识别
			select {
			case <-rf.host.IDService().IdentifyWait(conn): // 等待身份识别完成
				select {
				case ready <- struct{}{}: // 发送就绪信号
				default:
				}
			case <-ctx.Done(): // 上下文取消
			}
		}(conn)
	}

	select {
	case <-ready: // 等待就绪信号
	case <-ctx.Done(): // 上下文取消
		log.Debugf("检查节点 %s 的中继协议支持时出错: %v", pi.ID, ctx.Err())
		return false, ctx.Err() // 返回上下文错误
	}

	protos, err := rf.host.Peerstore().SupportsProtocols(pi.ID, protoIDv2) // 检查协议支持
	if err != nil {                                                        // 如果发生错误
		log.Debugf("检查节点 %s 的中继协议支持时出错: %v", pi.ID, err)
		return false, fmt.Errorf("检查节点 %s 的中继协议支持时出错: %w", pi.ID, err) // 返回错误
	}
	if len(protos) == 0 { // 如果不支持任何协议
		log.Debugf("节点 %s 不支持中继协议", pi.ID)
		return false, errProtocolNotSupported // 返回协议不支持错误
	}
	return true, nil // 返回支持v2协议
}

// handleNewCandidates 处理新的候选节点通知
// 当发现新的可能作为中继的节点时,我们会在 maybeConnectToRelayTrigger 通道上收到通知
// 此函数确保一次只运行一个 maybeConnectToRelay 实例,并缓冲一个触发事件以运行 maybeConnectToRelay
// 参数:
//   - ctx: context.Context 上下文对象
func (rf *relayFinder) handleNewCandidates(ctx context.Context) {
	for { // 无限循环
		select {
		case <-ctx.Done(): // 上下文取消
			return
		case <-rf.maybeConnectToRelayTrigger: // 收到可能连接中继的触发
			rf.maybeConnectToRelay(ctx) // 尝试连接中继
		}
	}
}

// maybeConnectToRelay 尝试连接到中继节点
// 参数:
//   - ctx: context.Context 上下文对象
func (rf *relayFinder) maybeConnectToRelay(ctx context.Context) {
	rf.relayMx.Lock()           // 加锁保护中继映射
	numRelays := len(rf.relays) // 获取当前中继数量
	rf.relayMx.Unlock()         // 解锁
	// 如果已连接到期望数量的中继,则无需操作
	if numRelays == rf.conf.desiredRelays { // 如果达到期望中继数量
		return // 直接返回
	}

	rf.candidateMx.Lock() // 加锁保护候选节点映射
	if len(rf.relays) == 0 && len(rf.candidates) < rf.conf.minCandidates && rf.conf.clock.Since(rf.bootTime) < rf.conf.bootDelay {
		// 在启动阶段,我们不想连接到找到的第一个候选节点
		// 相反,我们等待直到找到至少 minCandidates 个候选节点,然后选择其中最好的
		// 但是,如果等待时间太长(超过 bootDelay),我们仍会继续
		rf.candidateMx.Unlock() // 解锁
		return                  // 返回
	}
	if len(rf.candidates) == 0 { // 如果没有候选节点
		rf.candidateMx.Unlock() // 解锁
		return                  // 返回
	}
	candidates := rf.selectCandidates() // 选择候选节点
	rf.candidateMx.Unlock()             // 解锁

	// 现在遍历候选节点,依次尝试获取预约,直到达到期望的中继数量
	for _, cand := range candidates { // 遍历候选节点
		id := cand.ai.ID                // 获取节点ID
		rf.relayMx.Lock()               // 加锁保护中继映射
		usingRelay := rf.usingRelay(id) // 检查是否已使用该中继
		rf.relayMx.Unlock()             // 解锁
		if usingRelay {                 // 如果已使用该中继
			rf.candidateMx.Lock()             // 加锁
			rf.removeCandidate(id)            // 移除候选节点
			rf.candidateMx.Unlock()           // 解锁
			rf.notifyMaybeNeedNewCandidates() // 通知可能需要新候选节点
			continue                          // 继续下一个
		}
		rsvp, err := rf.connectToRelay(ctx, cand) // 连接到中继
		if err != nil {                           // 如果发生错误
			log.Debugf("连接中继失败: %v", err)
			rf.notifyMaybeNeedNewCandidates()                       // 通知可能需要新候选节点
			rf.metricsTracer.ReservationRequestFinished(false, err) // 更新指标
			continue                                                // 继续下一个
		}
		log.Debugf("添加新中继: %s", id)
		rf.relayMx.Lock()                 // 加锁
		rf.relays[id] = rsvp              // 添加中继预约
		numRelays := len(rf.relays)       // 获取中继数量
		rf.relayMx.Unlock()               // 解锁
		rf.notifyMaybeNeedNewCandidates() // 通知可能需要新候选节点

		rf.host.ConnManager().Protect(id, autorelayTag) // 保护连接

		select {
		case rf.relayUpdated <- struct{}{}: // 发送中继更新信号
		default:
		}

		rf.metricsTracer.ReservationRequestFinished(false, nil) // 更新指标

		if numRelays >= rf.conf.desiredRelays { // 如果达到期望中继数量
			break // 跳出循环
		}
	}
}

// connectToRelay 连接到中继节点并获取预约
// 参数:
//   - ctx: context.Context 上下文对象
//   - cand: *candidate 候选节点
//
// 返回值:
//   - *circuitv2.Reservation: 中继预约对象
//   - error: 错误信息
func (rf *relayFinder) connectToRelay(ctx context.Context, cand *candidate) (*circuitv2.Reservation, error) {
	id := cand.ai.ID // 获取节点ID

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second) // 创建超时上下文
	defer cancel()                                          // 延迟取消上下文

	var rsvp *circuitv2.Reservation // 声明预约变量

	// 确保仍然保持连接
	if rf.host.Network().Connectedness(id) != network.Connected { // 如果未连接
		if err := rf.host.Connect(ctx, cand.ai); err != nil { // 尝试连接
			rf.candidateMx.Lock()          // 加锁
			rf.removeCandidate(cand.ai.ID) // 移除候选节点
			rf.candidateMx.Unlock()        // 解锁
			log.Debugf("连接中继 %s 失败: %v", id, err)
			return nil, fmt.Errorf("连接失败: %w", err) // 返回错误
		}
	}

	rf.candidateMx.Lock()                // 加锁
	rf.backoff[id] = rf.conf.clock.Now() // 设置退避时间
	rf.candidateMx.Unlock()              // 解锁
	var err error                        // 声明错误变量
	if cand.supportsRelayV2 {            // 如果支持中继v2协议
		rsvp, err = circuitv2.Reserve(ctx, rf.host, cand.ai) // 获取预约
		if err != nil {                                      // 如果发生错误
			err = fmt.Errorf("预约失败: %w", err) // 包装错误
		}
	}
	rf.candidateMx.Lock()   // 加锁
	rf.removeCandidate(id)  // 移除候选节点
	rf.candidateMx.Unlock() // 解锁
	return rsvp, err        // 返回预约和错误
}

// refreshReservations 刷新即将过期的预约
// 参数:
//   - ctx: context.Context 上下文对象
//   - now: time.Time 当前时间
//
// 返回值:
//   - bool: 是否有预约刷新失败
func (rf *relayFinder) refreshReservations(ctx context.Context, now time.Time) bool {
	rf.relayMx.Lock() // 加锁

	// 并行刷新即将过期的预约
	g := new(errgroup.Group)         // 创建错误组
	for p, rsvp := range rf.relays { // 遍历所有中继
		if now.Add(rsvpExpirationSlack).Before(rsvp.Expiration) { // 如果预约未接近过期
			continue // 继续下一个
		}

		p := p              // 捕获变量
		g.Go(func() error { // 启动协程
			err := rf.refreshRelayReservation(ctx, p)              // 刷新预约
			rf.metricsTracer.ReservationRequestFinished(true, err) // 更新指标
			if err != nil {
				log.Debugf("刷新中继预约失败: %v", err)
			}
			return err // 返回错误
		})
	}
	rf.relayMx.Unlock() // 解锁

	err := g.Wait()   // 等待所有刷新完成
	return err != nil // 返回是否有错误
}

// refreshRelayReservation 刷新单个中继预约
// 参数:
//   - ctx: context.Context 上下文对象
//   - p: peer.ID 节点ID
//
// 返回值:
//   - error: 错误信息
func (rf *relayFinder) refreshRelayReservation(ctx context.Context, p peer.ID) error {
	rsvp, err := circuitv2.Reserve(ctx, rf.host, peer.AddrInfo{ID: p}) // 获取新预约

	rf.relayMx.Lock() // 加锁
	if err != nil {   // 如果发生错误
		log.Debugw("刷新中继预约失败", "relay", p, "error", err) // 记录日志
		_, exists := rf.relays[p]                        // 检查是否存在
		delete(rf.relays, p)                             // 删除中继
		rf.host.ConnManager().Unprotect(p, autorelayTag) // 取消保护连接
		rf.relayMx.Unlock()                              // 解锁
		if exists {                                      // 如果存在
			rf.metricsTracer.ReservationEnded(1) // 更新指标
		}
		log.Debugf("刷新中继预约失败: %v", err)
		return err // 返回错误
	}

	log.Debugw("刷新中继预约成功", "relay", p) // 记录日志
	rf.relays[p] = rsvp                // 更新预约
	rf.relayMx.Unlock()                // 解锁
	return nil                         // 返回nil
}

// usingRelay 检查是否正在使用指定中继
// 参数:
//   - p: peer.ID 节点ID
//
// 返回值:
//   - bool: 是否正在使用
func (rf *relayFinder) usingRelay(p peer.ID) bool {
	_, ok := rf.relays[p] // 检查中继映射
	return ok             // 返回结果
}

// addCandidate 添加候选节点
// 假设调用者持有candidateMx锁
// 参数:
//   - cand: *candidate 候选节点
func (rf *relayFinder) addCandidate(cand *candidate) {
	_, exists := rf.candidates[cand.ai.ID] // 检查是否存在
	rf.candidates[cand.ai.ID] = cand       // 添加候选节点
	if !exists {                           // 如果不存在
		rf.metricsTracer.CandidateAdded(1) // 更新指标
	}
}

// removeCandidate 移除候选节点
// 参数:
//   - id: peer.ID 节点ID
func (rf *relayFinder) removeCandidate(id peer.ID) {
	_, exists := rf.candidates[id] // 检查是否存在
	if exists {                    // 如果存在
		delete(rf.candidates, id)            // 删除候选节点
		rf.metricsTracer.CandidateRemoved(1) // 更新指标
	}
}

// selectCandidates 选择候选节点
// 返回值:
//   - []*candidate: 候选节点列表
func (rf *relayFinder) selectCandidates() []*candidate {
	now := rf.conf.clock.Now()                              // 获取当前时间
	candidates := make([]*candidate, 0, len(rf.candidates)) // 创建候选节点切片
	for _, cand := range rf.candidates {                    // 遍历所有候选节点
		if cand.added.Add(rf.conf.maxCandidateAge).After(now) { // 如果未过期
			candidates = append(candidates, cand) // 添加到切片
		}
	}

	// TODO: 实现更好的中继选择策略;目前只是随机选择,
	// 但我们应该使用ping延迟作为选择指标
	rand.Shuffle(len(candidates), func(i, j int) { // 随机打乱顺序
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})
	return candidates // 返回候选节点列表
}

// relayAddrs 计算NAT后的中继地址
// 当我们的状态为私有时:
//   - 从地址集中移除公共地址
//   - 保留非公共地址,以便同一NAT/防火墙后的节点可以直接连接
//   - 为已连接的中继添加中继特定地址。对于每个非私有中继地址,
//     我们封装可以拨号的p2p-circuit地址
//
// 参数:
//   - addrs: []ma.Multiaddr 地址列表
//
// 返回值:
//   - []ma.Multiaddr: 处理后的地址列表
func (rf *relayFinder) relayAddrs(addrs []ma.Multiaddr) []ma.Multiaddr {
	rf.relayMx.Lock()         // 加锁
	defer rf.relayMx.Unlock() // 延迟解锁

	if rf.cachedAddrs != nil && rf.conf.clock.Now().Before(rf.cachedAddrsExpiry) { // 如果缓存未过期
		return rf.cachedAddrs // 返回缓存地址
	}

	raddrs := make([]ma.Multiaddr, 0, 4*len(rf.relays)+4) // 创建地址切片

	// 只保留原地址集中的私有地址
	for _, addr := range addrs { // 遍历地址
		if manet.IsPrivateAddr(addr) { // 如果是私有地址
			raddrs = append(raddrs, addr) // 添加到切片
		}
	}

	// 添加中继特定地址
	relayAddrCnt := 0          // 中继地址计数
	for p := range rf.relays { // 遍历中继
		addrs := cleanupAddressSet(rf.host.Peerstore().Addrs(p))        // 获取清理后的地址
		relayAddrCnt += len(addrs)                                      // 更新计数
		circuit := ma.StringCast(fmt.Sprintf("/p2p/%s/p2p-circuit", p)) // 创建电路地址
		for _, addr := range addrs {                                    // 遍历地址
			pub := addr.Encapsulate(circuit) // 封装地址
			raddrs = append(raddrs, pub)     // 添加到切片
		}
	}

	rf.cachedAddrs = raddrs                                          // 缓存地址
	rf.cachedAddrsExpiry = rf.conf.clock.Now().Add(30 * time.Second) // 设置过期时间

	rf.metricsTracer.RelayAddressCount(relayAddrCnt) // 更新指标
	return raddrs                                    // 返回地址列表
}

// Start 启动中继查找器
// 返回值:
//   - error: 错误信息
func (rf *relayFinder) Start() error {
	rf.ctxCancelMx.Lock()         // 加锁
	defer rf.ctxCancelMx.Unlock() // 延迟解锁
	if rf.ctxCancel != nil {      // 如果已启动
		return errAlreadyRunning // 返回错误
	}
	log.Debug("启动中继查找器") // 记录日志

	rf.initMetrics() // 初始化指标

	ctx, cancel := context.WithCancel(context.Background()) // 创建上下文
	rf.ctxCancel = cancel                                   // 保存取消函数
	rf.refCount.Add(1)                                      // 增加引用计数
	go func() {                                             // 启动后台任务
		defer rf.refCount.Done() // 减少引用计数
		rf.background(ctx)       // 执行后台任务
	}()
	return nil // 返回nil
}

// Stop 停止中继查找器
// 返回值:
//   - error: 错误信息
func (rf *relayFinder) Stop() error {
	rf.ctxCancelMx.Lock()         // 加锁
	defer rf.ctxCancelMx.Unlock() // 延迟解锁
	log.Debug("停止中继查找器")          // 记录日志
	if rf.ctxCancel != nil {      // 如果已启动
		rf.ctxCancel() // 取消上下文
	}
	rf.refCount.Wait() // 等待引用计数归零
	rf.ctxCancel = nil // 清空取消函数

	rf.resetMetrics() // 重置指标
	return nil        // 返回nil
}

// initMetrics 初始化指标
func (rf *relayFinder) initMetrics() {
	rf.metricsTracer.DesiredReservations(rf.conf.desiredRelays) // 设置期望预约数

	rf.relayMx.Lock()                                  // 加锁
	rf.metricsTracer.ReservationOpened(len(rf.relays)) // 更新预约指标
	rf.relayMx.Unlock()                                // 解锁

	rf.candidateMx.Lock()                               // 加锁
	rf.metricsTracer.CandidateAdded(len(rf.candidates)) // 更新候选指标
	rf.candidateMx.Unlock()                             // 解锁
}

// resetMetrics 重置指标
func (rf *relayFinder) resetMetrics() {
	rf.relayMx.Lock()                                 // 加锁
	rf.metricsTracer.ReservationEnded(len(rf.relays)) // 更新预约指标
	rf.relayMx.Unlock()                               // 解锁

	rf.candidateMx.Lock()                                 // 加锁
	rf.metricsTracer.CandidateRemoved(len(rf.candidates)) // 更新候选指标
	rf.candidateMx.Unlock()                               // 解锁

	rf.metricsTracer.RelayAddressCount(0)                        // 重置地址计数
	rf.metricsTracer.ScheduledWorkUpdated(&scheduledWorkTimes{}) // 更新计划任务时间
}
