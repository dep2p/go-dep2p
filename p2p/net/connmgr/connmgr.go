package connmgr

import (
	"context"     // 用于上下文控制
	"fmt"         // 用于格式化输出
	"sort"        // 用于排序
	"sync"        // 用于同步原语
	"sync/atomic" // 用于原子操作
	"time"        // 用于时间相关操作

	"github.com/benbjohnson/clock"           // 用于时钟模拟
	"github.com/dep2p/go-dep2p/core/connmgr" // 连接管理器接口
	"github.com/dep2p/go-dep2p/core/network" // 网络接口
	"github.com/dep2p/go-dep2p/core/peer"    // peer相关定义

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr" // 多地址
	logging "github.com/dep2p/log"                        // 日志
)

// 连接管理器的日志记录器
var log = logging.Logger("net-connmgr")

// BasicConnMgr 是一个连接管理器,当连接数超过高水位时会修剪连接。
// 新连接在被修剪之前会有一个宽限期。
// 修剪会在需要时自动运行,只有当距离上次修剪的时间超过10秒时才会执行。
// 此外,可以通过该结构体的公共接口显式请求修剪(参见 TrimOpenConns)。
//
// 配置参数见 NewConnManager。
type BasicConnMgr struct {
	*decayer // 衰减器,用于处理标签值的衰减

	clock clock.Clock // 时钟实例,用于时间相关操作

	cfg      *config  // 配置信息
	segments segments // 分段存储

	plk       sync.RWMutex                    // 保护 protected 字段的互斥锁
	protected map[peer.ID]map[string]struct{} // 受保护的peer ID和标签映射

	trimMutex sync.Mutex   // 基于通道的信号量,确保同一时间只有一个修剪操作在进行
	connCount atomic.Int32 // 连接计数,原子操作
	trimCount uint64       // 修剪计数,原子操作。这模仿了sync.Once的实现。

	lastTrimMu sync.RWMutex // 上次修剪时间的互斥锁
	lastTrim   time.Time    // 上次修剪的时间戳

	refCount                sync.WaitGroup  // 引用计数
	ctx                     context.Context // 上下文
	cancel                  func()          // 取消函数
	unregisterMemoryWatcher func()          // 内存监视器注销函数
}

// 确保 BasicConnMgr 实现了 ConnManager 和 Decayer 接口
var (
	_ connmgr.ConnManager = (*BasicConnMgr)(nil)
	_ connmgr.Decayer     = (*BasicConnMgr)(nil)
)

// segment 表示一个分段,存储peer信息
type segment struct {
	sync.Mutex                       // 内嵌互斥锁用于并发控制
	peers      map[peer.ID]*peerInfo // 存储peer信息的映射
}

// segments 存储所有分段
type segments struct {
	bucketsMu sync.Mutex    // 用于防止并发进程尝试同时获取多个分段锁时发生死锁
	buckets   [256]*segment // 分段数组
}

// get 根据peer ID获取对应的分段
// 参数:
//   - p: peer ID
//
// 返回:
//   - *segment: 对应的分段
func (ss *segments) get(p peer.ID) *segment {
	return ss.buckets[p[len(p)-1]] // 使用peer ID的最后一个字节作为索引
}

// countPeers 统计所有分段中的peer总数
// 返回:
//   - count: peer总数
func (ss *segments) countPeers() (count int) {
	for _, seg := range ss.buckets { // 遍历所有分段
		seg.Lock()              // 加锁
		count += len(seg.peers) // 累加每个分段中的peer数量
		seg.Unlock()            // 解锁
	}
	return count // 返回总数
}

// tagInfoFor 获取或创建peer的标签信息
// 参数:
//   - p: peer ID,要获取标签信息的peer标识符
//   - now: time.Time,当前时间戳,用于设置firstSeen字段
//
// 返回:
//   - *peerInfo: peer的标签信息对象
func (s *segment) tagInfoFor(p peer.ID, now time.Time) *peerInfo {
	// 尝试从现有映射中获取peer信息
	pi, ok := s.peers[p]
	if ok {
		// 如果找到,直接返回
		return pi
	}

	// 如果不存在,创建一个临时peer来缓存早期标签,等待Connected通知到达
	pi = &peerInfo{
		id:        p,                                             // 设置peer ID
		firstSeen: now,                                           // 设置首次见到的时间戳,将在第一个Connected通知到达时更新
		temp:      true,                                          // 标记为临时条目
		tags:      make(map[string]int),                          // 初始化标签映射
		decaying:  make(map[*decayingTag]*connmgr.DecayingValue), // 初始化衰减标签映射
		conns:     make(map[network.Conn]time.Time),              // 初始化连接映射
	}

	// 将新创建的peer信息存入映射
	s.peers[p] = pi

	// 返回peer信息
	return pi
}

// NewConnManager 创建一个新的 BasicConnMgr
// 参数:
//   - low: int,低水位线,当连接数低于此值时不会触发修剪
//   - hi: int,高水位线,当连接数超过此值时触发修剪
//   - opts: ...Option,可选的配置选项
//
// 返回:
//   - *BasicConnMgr: 新创建的连接管理器
//   - error: 如果创建过程中出现错误则返回错误
func NewConnManager(low, hi int, opts ...Option) (*BasicConnMgr, error) {
	// 创建默认配置
	cfg := &config{
		highWater:     hi,               // 设置高水位线
		lowWater:      low,              // 设置低水位线
		gracePeriod:   time.Minute,      // 设置默认宽限期为1分钟
		silencePeriod: 10 * time.Second, // 设置默认静默期为10秒
		clock:         clock.New(),      // 创建新的时钟实例
	}

	// 应用所有配置选项
	for _, o := range opts {
		if err := o(cfg); err != nil {
			log.Debugf("应用配置选项失败: %v", err)
			return nil, err
		}
	}

	// 如果没有设置衰减器,使用默认配置
	if cfg.decayer == nil {
		// 设置默认衰减器配置
		cfg.decayer = (&DecayerCfg{}).WithDefaults()
	}

	// 创建连接管理器实例
	cm := &BasicConnMgr{
		cfg:       cfg,                                       // 设置配置
		clock:     cfg.clock,                                 // 设置时钟
		protected: make(map[peer.ID]map[string]struct{}, 16), // 初始化受保护peer映射
		segments:  segments{},                                // 初始化分段
	}

	// 初始化所有分段
	for i := range cm.segments.buckets {
		cm.segments.buckets[i] = &segment{
			peers: make(map[peer.ID]*peerInfo), // 为每个分段创建peer映射
		}
	}

	// 创建上下文和取消函数
	cm.ctx, cm.cancel = context.WithCancel(context.Background())

	// 如果启用了紧急修剪,注册内存监视器
	if cfg.emergencyTrim {
		// 当内存不足时,立即触发修剪
		cm.unregisterMemoryWatcher = registerWatchdog(cm.memoryEmergency)
	}

	// 创建并设置衰减器
	decay, _ := NewDecayer(cfg.decayer, cm)
	cm.decayer = decay

	// 启动后台任务
	cm.refCount.Add(1)
	go cm.background()
	return cm, nil
}

// memoryEmergency 在内存不足时运行
// 关闭连接直到达到低水位
// 不考虑静默期或宽限期
// 尽量不关闭受保护的连接,但如果必要,没有连接是安全的
func (cm *BasicConnMgr) memoryEmergency() {
	// 获取当前连接数
	connCount := int(cm.connCount.Load())
	// 计算需要关闭的连接数
	target := connCount - cm.cfg.lowWater
	if target < 0 {
		log.Warnw("内存不足,但我们只有少量连接", "数量", connCount, "低水位", cm.cfg.lowWater)
		return
	} else {
		log.Warnf("内存不足。正在关闭 %d 个连接。", target)
	}

	// 获取修剪锁
	cm.trimMutex.Lock()
	// 修剪完成后增加修剪计数
	defer atomic.AddUint64(&cm.trimCount, 1)
	// 释放修剪锁
	defer cm.trimMutex.Unlock()

	// 获取需要关闭的连接并关闭它们
	for _, c := range cm.getConnsToCloseEmergency(target) {
		log.Infow("内存不足,正在关闭连接", "peer", c.RemotePeer())
		c.Close()
	}

	// 更新上次修剪时间
	cm.lastTrimMu.Lock()
	cm.lastTrim = cm.clock.Now()
	cm.lastTrimMu.Unlock()
}

// Close 关闭连接管理器
// 参数: 无
// 返回:
//   - error: 关闭过程中的错误
func (cm *BasicConnMgr) Close() error {
	// 取消上下文
	cm.cancel()
	// 如果注册了内存监视器,注销它
	if cm.unregisterMemoryWatcher != nil {
		cm.unregisterMemoryWatcher()
	}
	// 关闭衰减器
	if err := cm.decayer.Close(); err != nil {
		log.Debugf("关闭衰减器失败: %v", err)
		return err
	}
	// 等待所有引用计数归零
	cm.refCount.Wait()
	return nil
}

// Protect 保护指定peer ID和标签
// 参数:
//   - id: peer ID
//   - tag: 标签名
func (cm *BasicConnMgr) Protect(id peer.ID, tag string) {
	// 获取保护锁
	cm.plk.Lock()
	defer cm.plk.Unlock()

	// 获取或创建标签集合
	tags, ok := cm.protected[id]
	if !ok {
		tags = make(map[string]struct{}, 2)
		cm.protected[id] = tags
	}
	// 添加标签
	tags[tag] = struct{}{}
}

// Unprotect 取消对指定peer ID和标签的保护
// 参数:
//   - id: peer ID
//   - tag: 标签名
//
// 返回:
//   - protected: 是否仍受保护
func (cm *BasicConnMgr) Unprotect(id peer.ID, tag string) (protected bool) {
	// 获取保护锁
	cm.plk.Lock()
	defer cm.plk.Unlock()

	// 获取标签集合
	tags, ok := cm.protected[id]
	if !ok {
		return false
	}
	// 删除标签,如果没有标签了就删除整个peer
	if delete(tags, tag); len(tags) == 0 {
		delete(cm.protected, id)
		return false
	}
	return true
}

// IsProtected 检查指定peer ID和标签是否受保护
// 参数:
//   - id: peer ID
//   - tag: 标签名
//
// 返回:
//   - protected: 是否受保护
func (cm *BasicConnMgr) IsProtected(id peer.ID, tag string) (protected bool) {
	// 获取保护锁
	cm.plk.Lock()
	defer cm.plk.Unlock()

	// 获取标签集合
	tags, ok := cm.protected[id]
	if !ok {
		return false
	}

	// 如果没有指定标签,只要peer受保护就返回true
	if tag == "" {
		return true
	}

	// 检查特定标签是否存在
	_, protected = tags[tag]
	return protected
}

// CheckLimit 检查连接限制是否超过系统限制
// 参数:
//   - systemLimit: 系统连接限制器
//
// 返回:
//   - error: 如果超过限制则返回错误
func (cm *BasicConnMgr) CheckLimit(systemLimit connmgr.GetConnLimiter) error {
	if cm.cfg.highWater > systemLimit.GetConnLimit() {
		log.Debugf(
			"连接管理器高水位限制: %d, 超过了系统连接限制: %d",
			cm.cfg.highWater,
			systemLimit.GetConnLimit(),
		)
		return fmt.Errorf(
			"连接管理器高水位限制: %d, 超过了系统连接限制: %d",
			cm.cfg.highWater,
			systemLimit.GetConnLimit(),
		)
	}
	return nil
}

// peerInfo 存储peer的元数据
type peerInfo struct {
	id       peer.ID
	tags     map[string]int                          // 每个标签的值
	decaying map[*decayingTag]*connmgr.DecayingValue // 衰减标签

	value int  // 所有标签值的缓存和
	temp  bool // 这是一个临时条目,保存早期标签并等待连接

	conns map[network.Conn]time.Time // 每个连接的开始时间

	firstSeen time.Time // 开始跟踪此peer的时间戳
}

type peerInfos []*peerInfo

// SortByValueAndStreams 根据值和流数量对peerInfos进行排序
// 在其他条件相同的情况下,它会将没有流的peer排在有流的peer之前
// 如果 sortByMoreStreams 为true,它会将流更多的peer排在流较少的peer之前
// 这对于优先释放内存很有用
// 参数:
//   - segments: *segments,分段存储
//   - sortByMoreStreams: bool,是否按流数量降序排序
func (p peerInfos) SortByValueAndStreams(segments *segments, sortByMoreStreams bool) {
	// 使用sort.Slice对peerInfos进行排序
	sort.Slice(p, func(i, j int) bool {
		left, right := p[i], p[j] // 获取要比较的两个peer信息

		// 获取bucketsMu锁以防止在获取分段锁时发生死锁
		segments.bucketsMu.Lock()

		// 锁定左侧peer所在分段以防止并发修改
		leftSegment := segments.get(left.id)
		leftSegment.Lock()
		defer leftSegment.Unlock()

		// 获取右侧peer所在分段
		rightSegment := segments.get(right.id)
		if leftSegment != rightSegment {
			// 如果两个peer不在同一分段,锁定右侧分段
			rightSegment.Lock()
			defer rightSegment.Unlock()
		}
		segments.bucketsMu.Unlock() // 释放bucketsMu锁

		// 优先修剪临时peer
		if left.temp != right.temp {
			return left.temp
		}
		// 如果值不同,按值升序排序
		if left.value != right.value {
			return left.value < right.value
		}

		// 辅助函数:统计连接的入站状态和流数量
		incomingAndStreams := func(m map[network.Conn]time.Time) (incoming bool, numStreams int) {
			for c := range m {
				stat := c.Stat()
				if stat.Direction == network.DirInbound {
					incoming = true
				}
				numStreams += stat.NumStreams
			}
			return
		}

		// 获取两个peer的连接状态
		leftIncoming, leftStreams := incomingAndStreams(left.conns)
		rightIncoming, rightStreams := incomingAndStreams(right.conns)

		// 优先关闭没有活动流的连接
		if rightStreams != leftStreams && (leftStreams == 0 || rightStreams == 0) {
			return leftStreams < rightStreams
		}
		// 优先修剪入站连接
		if leftIncoming != rightIncoming {
			return leftIncoming
		}

		if sortByMoreStreams {
			// 如果指定按流数量降序,则流多的排在前面
			return rightStreams < leftStreams
		} else {
			// 否则按流数量升序排序
			return leftStreams < rightStreams
		}
	})
}

// TrimOpenConns 关闭尽可能多的peer连接,使peer数量等于低水位
// peer按总值升序排序,优先修剪分数最低的peer,只要它们不在宽限期内
// 此函数会阻塞直到修剪完成
// 如果修剪正在进行,不会启动新的修剪,而是等待当前修剪完成后返回
// 参数:
//   - _: context.Context,上下文(未使用)
func (cm *BasicConnMgr) TrimOpenConns(_ context.Context) {
	// TODO: 返回错误值,以便我们可以清晰地表明我们正在中止,因为:
	// (a)有另一个修剪正在进行,或(b)静默期正在生效

	cm.doTrim() // 执行修剪操作
}

// background 运行后台任务
func (cm *BasicConnMgr) background() {
	defer cm.refCount.Done()

	interval := cm.cfg.gracePeriod / 2
	if cm.cfg.silencePeriod != 0 {
		interval = cm.cfg.silencePeriod
	}

	ticker := cm.clock.Ticker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if cm.connCount.Load() < int32(cm.cfg.highWater) {
				// 低于高水位,跳过
				continue
			}
		case <-cm.ctx.Done():
			return
		}
		cm.trim()
	}
}

// doTrim 执行修剪操作
func (cm *BasicConnMgr) doTrim() {
	// 此逻辑模仿标准库中sync.Once的实现
	count := atomic.LoadUint64(&cm.trimCount)
	cm.trimMutex.Lock()
	defer cm.trimMutex.Unlock()
	if count == atomic.LoadUint64(&cm.trimCount) {
		cm.trim()
		cm.lastTrimMu.Lock()
		cm.lastTrim = cm.clock.Now()
		cm.lastTrimMu.Unlock()
		atomic.AddUint64(&cm.trimCount, 1)
	}
}

// trim 如果距离上次修剪的时间超过配置的静默期,则开始修剪
func (cm *BasicConnMgr) trim() {
	// 执行实际的修剪
	for _, c := range cm.getConnsToClose() {
		log.Debugw("正在关闭连接", "peer", c.RemotePeer())
		c.Close()
	}
}

// getConnsToCloseEmergency 在紧急情况下获取需要关闭的连接
// 参数:
//   - target: 目标关闭连接数
//
// 返回:
//   - []network.Conn: 需要关闭的连接列表
func (cm *BasicConnMgr) getConnsToCloseEmergency(target int) []network.Conn {
	// 创建候选peer列表
	candidates := make(peerInfos, 0, cm.segments.countPeers())

	// 获取所有非受保护的peer
	cm.plk.RLock()
	for _, s := range cm.segments.buckets {
		s.Lock()
		for id, inf := range s.peers {
			if _, ok := cm.protected[id]; ok {
				// 跳过受保护的peer
				continue
			}
			candidates = append(candidates, inf)
		}
		s.Unlock()
	}
	cm.plk.RUnlock()

	// 根据peer的价值对候选者进行排序
	candidates.SortByValueAndStreams(&cm.segments, true)

	// 创建要关闭的连接列表
	selected := make([]network.Conn, 0, target+10)
	for _, inf := range candidates {
		if target <= 0 {
			break
		}
		s := cm.segments.get(inf.id)
		s.Lock()
		for c := range inf.conns {
			selected = append(selected, c)
		}
		target -= len(inf.conns)
		s.Unlock()
	}
	if len(selected) >= target {
		// 找到足够的非受保护连接
		return selected
	}

	// 未找到足够的非受保护连接
	// 不得不关闭一些受保护的连接
	candidates = candidates[:0]
	cm.plk.RLock()
	for _, s := range cm.segments.buckets {
		s.Lock()
		for _, inf := range s.peers {
			candidates = append(candidates, inf)
		}
		s.Unlock()
	}
	cm.plk.RUnlock()

	candidates.SortByValueAndStreams(&cm.segments, true)
	for _, inf := range candidates {
		if target <= 0 {
			break
		}
		// 加锁以防止连接/断开事件的并发修改
		s := cm.segments.get(inf.id)
		s.Lock()
		for c := range inf.conns {
			selected = append(selected, c)
		}
		target -= len(inf.conns)
		s.Unlock()
	}
	return selected
}

// getConnsToClose 运行 TrimOpenConns 中描述的启发式算法并返回要关闭的连接
// 返回:
//   - []network.Conn: 需要关闭的连接列表
func (cm *BasicConnMgr) getConnsToClose() []network.Conn {
	if cm.cfg.lowWater == 0 || cm.cfg.highWater == 0 {
		// 连接管理已禁用
		return nil
	}

	if int(cm.connCount.Load()) <= cm.cfg.lowWater {
		log.Info("开放连接数低于限制")
		return nil
	}

	// 创建候选peer列表
	candidates := make(peerInfos, 0, cm.segments.countPeers())
	var ncandidates int
	gracePeriodStart := cm.clock.Now().Add(-cm.cfg.gracePeriod)

	// 获取所有非受保护且超过宽限期的peer
	cm.plk.RLock()
	for _, s := range cm.segments.buckets {
		s.Lock()
		for id, inf := range s.peers {
			if _, ok := cm.protected[id]; ok {
				// 跳过受保护的peer
				continue
			}
			if inf.firstSeen.After(gracePeriodStart) {
				// 跳过在宽限期内的peer
				continue
			}
			// 注意这里复制了条目,但由于 inf.conns 是一个map,它仍然指向原始对象
			candidates = append(candidates, inf)
			ncandidates += len(inf.conns)
		}
		s.Unlock()
	}
	cm.plk.RUnlock()

	if ncandidates < cm.cfg.lowWater {
		log.Info("开放连接数超过限制但太多连接处于宽限期")
		// 我们有太多连接,但宽限期外的连接少于低水位
		// 如果现在修剪,我们可能会杀死潜在有用的连接
		return nil
	}

	// 根据peer的价值对候选者进行排序
	candidates.SortByValueAndStreams(&cm.segments, false)

	target := ncandidates - cm.cfg.lowWater

	// 稍微多分配一些空间,因为每个peer可能有多个连接
	selected := make([]network.Conn, 0, target+10)

	for _, inf := range candidates {
		if target <= 0 {
			break
		}

		// 加锁以防止连接/断开事件的并发修改
		s := cm.segments.get(inf.id)
		s.Lock()
		if len(inf.conns) == 0 && inf.temp {
			// 处理早期标签的临时条目 -- 该条目已超过宽限期且仍未持有连接,因此将其删除
			delete(s.peers, inf.id)
		} else {
			for c := range inf.conns {
				selected = append(selected, c)
			}
			target -= len(inf.conns)
		}
		s.Unlock()
	}

	return selected
}

// GetTagInfo 获取与指定peer关联的标签信息,如果p指向未知peer则返回nil
// 参数:
//   - p: peer ID
//
// 返回:
//   - *connmgr.TagInfo: peer的标签信息
func (cm *BasicConnMgr) GetTagInfo(p peer.ID) *connmgr.TagInfo {
	s := cm.segments.get(p)
	s.Lock()
	defer s.Unlock()

	pi, ok := s.peers[p]
	if !ok {
		return nil
	}

	// 创建输出结构
	out := &connmgr.TagInfo{
		FirstSeen: pi.firstSeen,
		Value:     pi.value,
		Tags:      make(map[string]int),
		Conns:     make(map[string]time.Time),
	}

	// 复制标签信息
	for t, v := range pi.tags {
		out.Tags[t] = v
	}
	for t, v := range pi.decaying {
		out.Tags[t.name] = v.Value
	}
	for c, t := range pi.conns {
		out.Conns[c.RemoteMultiaddr().String()] = t
	}

	return out
}

// TagPeer 为指定peer关联一个字符串标签和整数值
// 参数:
//   - p: peer ID
//   - tag: 标签名
//   - val: 标签值
func (cm *BasicConnMgr) TagPeer(p peer.ID, tag string, val int) {
	s := cm.segments.get(p)
	s.Lock()
	defer s.Unlock()

	pi := s.tagInfoFor(p, cm.clock.Now())

	// 更新peer的总价值
	pi.value += val - pi.tags[tag]
	pi.tags[tag] = val
}

// UntagPeer 解除指定peer的标签关联
// 参数:
//   - p: peer ID
//   - tag: 要解除的标签名
func (cm *BasicConnMgr) UntagPeer(p peer.ID, tag string) {
	s := cm.segments.get(p)
	s.Lock()
	defer s.Unlock()

	pi, ok := s.peers[p]
	if !ok {
		log.Debug("尝试从未跟踪的peer移除标签: ", p, tag)
		return
	}

	// 更新peer的总价值
	pi.value -= pi.tags[tag]
	delete(pi.tags, tag)
}

// UpsertTag 插入/更新peer标签
// 参数:
//   - p: peer ID
//   - tag: 标签名
//   - upsert: 更新函数,接收旧值返回新值
func (cm *BasicConnMgr) UpsertTag(p peer.ID, tag string, upsert func(int) int) {
	s := cm.segments.get(p)
	s.Lock()
	defer s.Unlock()

	pi := s.tagInfoFor(p, cm.clock.Now())

	oldval := pi.tags[tag]
	newval := upsert(oldval)
	pi.value += newval - oldval
	pi.tags[tag] = newval
}

// CMInfo 保存 BasicConnMgr 的配置和状态数据
type CMInfo struct {
	// LowWater 低水位线,如 NewConnManager 中所述
	LowWater int

	// HighWater 高水位线,如 NewConnManager 中所述
	HighWater int

	// LastTrim 上次触发修剪的时间戳
	LastTrim time.Time

	// GracePeriod 配置的宽限期,如 NewConnManager 中所述
	GracePeriod time.Duration

	// ConnCount 当前连接数
	ConnCount int
}

// GetInfo 返回此连接管理器的配置和状态数据
// 返回:
//   - CMInfo: 连接管理器的信息
func (cm *BasicConnMgr) GetInfo() CMInfo {
	cm.lastTrimMu.RLock()
	lastTrim := cm.lastTrim
	cm.lastTrimMu.RUnlock()

	return CMInfo{
		HighWater:   cm.cfg.highWater,
		LowWater:    cm.cfg.lowWater,
		LastTrim:    lastTrim,
		GracePeriod: cm.cfg.gracePeriod,
		ConnCount:   int(cm.connCount.Load()),
	}
}

// Notifee 返回一个接收器,通过它通知器可以在事件发生时通知 BasicConnMgr
// 目前,通知器仅对连接事件{Connected, Disconnected}做出反应
// 返回:
//   - network.Notifiee: 通知接收器
func (cm *BasicConnMgr) Notifee() network.Notifiee {
	return (*cmNotifee)(cm)
}

// cmNotifee 是 BasicConnMgr 的通知接收器类型
type cmNotifee BasicConnMgr

// cm 返回通知接收器对应的连接管理器
// 返回:
//   - *BasicConnMgr: 连接管理器实例
func (nn *cmNotifee) cm() *BasicConnMgr {
	return (*BasicConnMgr)(nn)
}

// Connected 由通知器调用,通知新连接已建立
// 通知接收器更新 BasicConnMgr 以开始跟踪连接
// 如果新连接数超过高水位线,可能会触发修剪
// 参数:
//   - n: 网络实例
//   - c: 新建立的连接
func (nn *cmNotifee) Connected(n network.Network, c network.Conn) {
	cm := nn.cm()

	p := c.RemotePeer()
	s := cm.segments.get(p)
	s.Lock()
	defer s.Unlock()

	id := c.RemotePeer()
	pinfo, ok := s.peers[id]
	if !ok {
		pinfo = &peerInfo{
			id:        id,
			firstSeen: cm.clock.Now(),
			tags:      make(map[string]int),
			decaying:  make(map[*decayingTag]*connmgr.DecayingValue),
			conns:     make(map[network.Conn]time.Time),
		}
		s.peers[id] = pinfo
	} else if pinfo.temp {
		// 我们为这个peer创建了一个临时条目来缓存Connected通知到达前的早期标签:
		// 翻转临时标志,并将firstSeen时间戳更新为真实值
		pinfo.temp = false
		pinfo.firstSeen = cm.clock.Now()
	}

	_, ok = pinfo.conns[c]
	if ok {
		log.Debugf("收到已在跟踪的连接的connected通知: ", p)
		return
	}

	pinfo.conns[c] = cm.clock.Now()
	cm.connCount.Add(1)
}

// Disconnected 由通知器调用,通知现有连接已关闭或终止
// 通知接收器相应地更新 BasicConnMgr 以停止跟踪连接,并执行清理工作
// 参数:
//   - n: 网络实例
//   - c: 已关闭的连接
func (nn *cmNotifee) Disconnected(n network.Network, c network.Conn) {
	cm := nn.cm()

	p := c.RemotePeer()
	s := cm.segments.get(p)
	s.Lock()
	defer s.Unlock()

	cinf, ok := s.peers[p]
	if !ok {
		log.Debugf("收到未跟踪peer的disconnected通知: ", p)
		return
	}

	_, ok = cinf.conns[c]
	if !ok {
		log.Debugf("收到未跟踪连接的disconnected通知: ", p)
		return
	}

	delete(cinf.conns, c)
	if len(cinf.conns) == 0 {
		delete(s.peers, p)
	}
	cm.connCount.Add(-1)
}

// Listen 在此实现中为空操作
func (nn *cmNotifee) Listen(n network.Network, addr ma.Multiaddr) {}

// ListenClose 在此实现中为空操作
func (nn *cmNotifee) ListenClose(n network.Network, addr ma.Multiaddr) {}
