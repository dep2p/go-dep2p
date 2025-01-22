package pstoremem

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/peerstore"
	"github.com/dep2p/go-dep2p/core/record"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	logging "github.com/dep2p/log"
)

// 日志记录器
var log = logging.Logger("host-peerstore-pstoremem")

// expiringAddr 表示一个带有过期时间的地址
type expiringAddr struct {
	Addr      ma.Multiaddr  // 多地址
	TTL       time.Duration // 生存时间
	Expiry    time.Time     // 过期时间
	Peer      peer.ID       // 对等节点ID
	heapIndex int           // 用于在堆中排序,值为-1表示不在堆中
}

// ExpiredBy 检查地址是否在给定时间已过期
// 参数:
//   - t: 检查时间
//
// 返回:
//   - bool: 是否已过期
func (e *expiringAddr) ExpiredBy(t time.Time) bool {
	return !t.Before(e.Expiry)
}

// IsConnected 检查地址是否处于已连接状态
// 返回:
//   - bool: 是否已连接
func (e *expiringAddr) IsConnected() bool {
	return ttlIsConnected(e.TTL)
}

// ttlIsConnected 检查TTL是否大于等于已连接地址的TTL
// 参数:
//   - ttl: 生存时间
//
// 返回:
//   - bool: 是否已连接
func ttlIsConnected(ttl time.Duration) bool {
	return ttl >= peerstore.ConnectedAddrTTL
}

// peerRecordState 表示对等节点记录状态
type peerRecordState struct {
	Envelope *record.Envelope // 记录信封
	Seq      uint64           // 序列号
}

// 确保peerAddrs实现了heap.Interface接口
var _ heap.Interface = &peerAddrs{}

// peerAddrs 管理对等节点的地址
type peerAddrs struct {
	Addrs        map[peer.ID]map[string]*expiringAddr // 对等节点ID -> 地址字节串 -> 过期地址
	expiringHeap []*expiringAddr                      // 仅存储未连接的地址,因为已连接地址有无限TTL
}

// newPeerAddrs 创建新的peerAddrs实例
// 返回:
//   - peerAddrs: 新创建的peerAddrs实例
func newPeerAddrs() peerAddrs {
	return peerAddrs{
		Addrs: make(map[peer.ID]map[string]*expiringAddr),
	}
}

// 以下是实现heap.Interface所需的方法

func (pa *peerAddrs) Len() int { return len(pa.expiringHeap) }

func (pa *peerAddrs) Less(i, j int) bool {
	return pa.expiringHeap[i].Expiry.Before(pa.expiringHeap[j].Expiry)
}

func (pa *peerAddrs) Swap(i, j int) {
	pa.expiringHeap[i], pa.expiringHeap[j] = pa.expiringHeap[j], pa.expiringHeap[i]
	pa.expiringHeap[i].heapIndex = i
	pa.expiringHeap[j].heapIndex = j
}

func (pa *peerAddrs) Push(x any) {
	a := x.(*expiringAddr)
	a.heapIndex = len(pa.expiringHeap)
	pa.expiringHeap = append(pa.expiringHeap, a)
}

func (pa *peerAddrs) Pop() any {
	a := pa.expiringHeap[len(pa.expiringHeap)-1]
	a.heapIndex = -1
	pa.expiringHeap = pa.expiringHeap[0 : len(pa.expiringHeap)-1]
	return a
}

// Delete 从peerAddrs中删除指定地址
// 参数:
//   - a: 要删除的地址
func (pa *peerAddrs) Delete(a *expiringAddr) {
	if ea, ok := pa.Addrs[a.Peer][string(a.Addr.Bytes())]; ok {
		if ea.heapIndex != -1 {
			heap.Remove(pa, a.heapIndex)
		}
		delete(pa.Addrs[a.Peer], string(a.Addr.Bytes()))
		if len(pa.Addrs[a.Peer]) == 0 {
			delete(pa.Addrs, a.Peer)
		}
	}
}

// FindAddr 查找指定对等节点的地址
// 参数:
//   - p: 对等节点ID
//   - addr: 要查找的地址
//
// 返回:
//   - *expiringAddr: 找到的地址
//   - bool: 是否找到
func (pa *peerAddrs) FindAddr(p peer.ID, addr ma.Multiaddr) (*expiringAddr, bool) {
	if m, ok := pa.Addrs[p]; ok {
		v, ok := m[string(addr.Bytes())]
		return v, ok
	}
	return nil, false
}

// NextExpiry 获取下一个过期时间
// 返回:
//   - time.Time: 下一个过期时间
func (pa *peerAddrs) NextExpiry() time.Time {
	if len(pa.expiringHeap) == 0 {
		return time.Time{}
	}
	return pa.expiringHeap[0].Expiry
}

// PopIfExpired 如果有过期地址则弹出
// 参数:
//   - now: 当前时间
//
// 返回:
//   - *expiringAddr: 过期的地址
//   - bool: 是否有过期地址
func (pa *peerAddrs) PopIfExpired(now time.Time) (*expiringAddr, bool) {
	if len(pa.expiringHeap) > 0 && !now.Before(pa.NextExpiry()) {
		ea := heap.Pop(pa).(*expiringAddr)
		delete(pa.Addrs[ea.Peer], string(ea.Addr.Bytes()))
		if len(pa.Addrs[ea.Peer]) == 0 {
			delete(pa.Addrs, ea.Peer)
		}
		return ea, true
	}
	return nil, false
}

// Update 更新地址状态
// 参数:
//   - a: 要更新的地址
func (pa *peerAddrs) Update(a *expiringAddr) {
	if a.heapIndex == -1 {
		return
	}
	if a.IsConnected() {
		heap.Remove(pa, a.heapIndex)
	} else {
		heap.Fix(pa, a.heapIndex)
	}
}

// Insert 插入新地址
// 参数:
//   - a: 要插入的地址
func (pa *peerAddrs) Insert(a *expiringAddr) {
	a.heapIndex = -1
	if _, ok := pa.Addrs[a.Peer]; !ok {
		pa.Addrs[a.Peer] = make(map[string]*expiringAddr)
	}
	pa.Addrs[a.Peer][string(a.Addr.Bytes())] = a
	if a.IsConnected() {
		return
	}
	heap.Push(pa, a)
}

// NumUnconnectedAddrs 获取未连接地址数量
// 返回:
//   - int: 未连接地址数量
func (pa *peerAddrs) NumUnconnectedAddrs() int {
	return len(pa.expiringHeap)
}

// clock 时钟接口
type clock interface {
	Now() time.Time
}

// realclock 实际时钟实现
type realclock struct{}

func (rc realclock) Now() time.Time {
	return time.Now()
}

// 常量定义
const (
	defaultMaxSignedPeerRecords = 100_000   // 默认最大签名对等记录数
	defaultMaxUnconnectedAddrs  = 1_000_000 // 默认最大未连接地址数
)

// memoryAddrBook 内存地址簿
type memoryAddrBook struct {
	mu                   sync.RWMutex                 // 读写锁
	addrs                peerAddrs                    // 地址存储
	signedPeerRecords    map[peer.ID]*peerRecordState // 签名对等记录
	maxUnconnectedAddrs  int                          // 最大未连接地址数
	maxSignedPeerRecords int                          // 最大签名对等记录数

	refCount sync.WaitGroup // 引用计数
	cancel   func()         // 取消函数

	subManager *AddrSubManager // 地址订阅管理器
	clock      clock           // 时钟接口
}

// 确保memoryAddrBook实现了相关接口
var _ peerstore.AddrBook = (*memoryAddrBook)(nil)
var _ peerstore.CertifiedAddrBook = (*memoryAddrBook)(nil)

// NewAddrBook 创建新的地址簿
// 参数:
//   - opts: 地址簿选项
//
// 返回:
//   - *memoryAddrBook: 新创建的地址簿
func NewAddrBook(opts ...AddrBookOption) *memoryAddrBook {
	ctx, cancel := context.WithCancel(context.Background())

	ab := &memoryAddrBook{
		addrs:                newPeerAddrs(),
		signedPeerRecords:    make(map[peer.ID]*peerRecordState),
		subManager:           NewAddrSubManager(),
		cancel:               cancel,
		clock:                realclock{},
		maxUnconnectedAddrs:  defaultMaxUnconnectedAddrs,
		maxSignedPeerRecords: defaultMaxSignedPeerRecords,
	}
	for _, opt := range opts {
		opt(ab)
	}

	ab.refCount.Add(1)
	go ab.background(ctx)
	return ab
}

// AddrBookOption 地址簿选项函数类型
type AddrBookOption func(book *memoryAddrBook) error

// WithClock 设置时钟选项
// 参数:
//   - clock: 时钟接口
//
// 返回:
//   - AddrBookOption: 地址簿选项函数
func WithClock(clock clock) AddrBookOption {
	return func(book *memoryAddrBook) error {
		book.clock = clock // 设置时钟
		return nil
	}
}

// WithMaxAddresses 设置最大地址数选项
// 参数:
//   - n: 最大地址数
//
// 返回:
//   - AddrBookOption: 地址簿选项函数
func WithMaxAddresses(n int) AddrBookOption {
	return func(b *memoryAddrBook) error {
		b.maxUnconnectedAddrs = n // 设置最大未连接地址数
		return nil
	}
}

// WithMaxSignedPeerRecords 设置最大签名对等记录数选项
// 参数:
//   - n: 最大签名对等记录数
//
// 返回:
//   - AddrBookOption: 地址簿选项函数
func WithMaxSignedPeerRecords(n int) AddrBookOption {
	return func(b *memoryAddrBook) error {
		b.maxSignedPeerRecords = n // 设置最大签名对等记录数
		return nil
	}
}

// background 周期性执行垃圾回收
// 参数:
//   - ctx: 上下文
func (mab *memoryAddrBook) background(ctx context.Context) {
	defer mab.refCount.Done()                 // 减少引用计数
	ticker := time.NewTicker(1 * time.Minute) // 创建定时器
	defer ticker.Stop()                       // 停止定时器

	for {
		select {
		case <-ticker.C: // 定时器触发
			mab.gc() // 执行垃圾回收
		case <-ctx.Done(): // 上下文取消
			return
		}
	}
}

// Close 关闭地址簿
// 返回:
//   - error: 错误信息
func (mab *memoryAddrBook) Close() error {
	mab.cancel()        // 取消上下文
	mab.refCount.Wait() // 等待引用计数归零
	return nil
}

// gc 执行内存地址簿的垃圾回收
func (mab *memoryAddrBook) gc() {
	now := mab.clock.Now() // 获取当前时间
	mab.mu.Lock()          // 加写锁
	defer mab.mu.Unlock()  // 解锁
	for {
		ea, ok := mab.addrs.PopIfExpired(now) // 弹出过期地址
		if !ok {
			return
		}
		mab.maybeDeleteSignedPeerRecordUnlocked(ea.Peer) // 尝试删除签名对等记录
	}
}

// PeersWithAddrs 获取所有有地址的对等节点
// 返回:
//   - peer.IDSlice: 对等节点ID列表
func (mab *memoryAddrBook) PeersWithAddrs() peer.IDSlice {
	mab.mu.RLock()                                       // 加读锁
	defer mab.mu.RUnlock()                               // 解锁
	peers := make(peer.IDSlice, 0, len(mab.addrs.Addrs)) // 创建切片
	for pid := range mab.addrs.Addrs {                   // 遍历所有地址
		peers = append(peers, pid) // 添加对等节点ID
	}
	return peers
}

// AddAddr 为单个地址调用AddAddrs
// 参数:
//   - p: 对等节点ID
//   - addr: 多地址
//   - ttl: 生存时间
func (mab *memoryAddrBook) AddAddr(p peer.ID, addr ma.Multiaddr, ttl time.Duration) {
	mab.AddAddrs(p, []ma.Multiaddr{addr}, ttl) // 调用AddAddrs
}

// AddAddrs 为对等节点添加地址列表
// 参数:
//   - p: 对等节点ID
//   - addrs: 多地址列表
//   - ttl: 生存时间
func (mab *memoryAddrBook) AddAddrs(p peer.ID, addrs []ma.Multiaddr, ttl time.Duration) {
	mab.addAddrs(p, addrs, ttl) // 调用内部方法
}

// ConsumePeerRecord 添加已签名的对等记录
// 参数:
//   - recordEnvelope: 记录信封
//   - ttl: 生存时间
//
// 返回:
//   - bool: 是否成功
//   - error: 错误信息
func (mab *memoryAddrBook) ConsumePeerRecord(recordEnvelope *record.Envelope, ttl time.Duration) (bool, error) {
	r, err := recordEnvelope.Record() // 获取记录
	if err != nil {
		log.Debugf("获取记录失败: %v", err)
		return false, err
	}
	rec, ok := r.(*peer.PeerRecord) // 类型断言
	if !ok {
		log.Debugf("无法处理信封:不是PeerRecord")
		return false, fmt.Errorf("无法处理信封:不是PeerRecord")
	}
	if !rec.PeerID.MatchesPublicKey(recordEnvelope.PublicKey) { // 验证公钥
		log.Debugf("签名密钥与PeerRecord中的PeerID不匹配")
		return false, fmt.Errorf("签名密钥与PeerRecord中的PeerID不匹配")
	}

	mab.mu.Lock()         // 加写锁
	defer mab.mu.Unlock() // 解锁

	lastState, found := mab.signedPeerRecords[rec.PeerID] // 获取上一个状态
	if found && lastState.Seq > rec.Seq {                 // 检查序列号
		return false, nil
	}
	if !found && len(mab.signedPeerRecords) >= mab.maxSignedPeerRecords { // 检查记录数量
		log.Debugf("签名对等记录数量过多")
		return false, errors.New("签名对等记录数量过多")
	}
	mab.signedPeerRecords[rec.PeerID] = &peerRecordState{ // 保存记录状态
		Envelope: recordEnvelope,
		Seq:      rec.Seq,
	}
	mab.addAddrsUnlocked(rec.PeerID, rec.Addrs, ttl) // 添加地址
	return true, nil
}

func (mab *memoryAddrBook) maybeDeleteSignedPeerRecordUnlocked(p peer.ID) {
	if len(mab.addrs.Addrs[p]) == 0 { // 如果没有地址
		delete(mab.signedPeerRecords, p) // 删除签名记录
	}
}

func (mab *memoryAddrBook) addAddrs(p peer.ID, addrs []ma.Multiaddr, ttl time.Duration) {
	mab.mu.Lock()         // 加写锁
	defer mab.mu.Unlock() // 解锁

	mab.addAddrsUnlocked(p, addrs, ttl) // 调用内部方法
}

func (mab *memoryAddrBook) addAddrsUnlocked(p peer.ID, addrs []ma.Multiaddr, ttl time.Duration) {
	defer mab.maybeDeleteSignedPeerRecordUnlocked(p) // 尝试删除签名记录

	// 如果ttl为0，直接返回
	if ttl <= 0 {
		return
	}

	// 如果超过未连接地址数限制，丢弃这些地址
	if !ttlIsConnected(ttl) && mab.addrs.NumUnconnectedAddrs() >= mab.maxUnconnectedAddrs {
		return
	}

	exp := mab.clock.Now().Add(ttl) // 计算过期时间
	for _, addr := range addrs {    // 遍历地址
		// 从地址中移除/p2p/peer-id后缀
		addr, addrPid := peer.SplitAddr(addr)
		if addr == nil {
			log.Warnw("传入了空的多地址", "peer", p)
			continue
		}
		if addrPid != "" && addrPid != p {
			log.Warnf("传入了带有不同对等节点ID的p2p地址,发现: %s 期望: %s", addrPid, p)
			continue
		}
		a, found := mab.addrs.FindAddr(p, addr) // 查找地址
		if !found {                             // 如果未找到
			// 创建新条目并插入
			entry := &expiringAddr{Addr: addr, Expiry: exp, TTL: ttl, Peer: p}
			mab.addrs.Insert(entry)
			mab.subManager.BroadcastAddr(p, addr) // 广播地址
		} else { // 如果找到
			// 更新TTL和过期时间为较大值
			var changed bool
			if ttl > a.TTL {
				changed = true
				a.TTL = ttl
			}
			if exp.After(a.Expiry) {
				changed = true
				a.Expiry = exp
			}
			if changed {
				mab.addrs.Update(a) // 更新地址
			}
		}
	}
}

// SetAddr 为单个地址调用SetAddrs
// 参数:
//   - p: 对等节点ID
//   - addr: 多地址
//   - ttl: 生存时间
func (mab *memoryAddrBook) SetAddr(p peer.ID, addr ma.Multiaddr, ttl time.Duration) {
	mab.SetAddrs(p, []ma.Multiaddr{addr}, ttl) // 调用SetAddrs
}

// SetAddrs 设置地址的TTL。这会清除之前的任何TTL。
// 当我们收到地址有效性的最佳估计时使用此方法。
// 参数:
//   - p: 对等节点ID
//   - addrs: 要设置的地址列表
//   - ttl: 生存时间
func (mab *memoryAddrBook) SetAddrs(p peer.ID, addrs []ma.Multiaddr, ttl time.Duration) {
	mab.mu.Lock()         // 加写锁
	defer mab.mu.Unlock() // 解锁

	defer mab.maybeDeleteSignedPeerRecordUnlocked(p) // 尝试删除签名记录

	exp := mab.clock.Now().Add(ttl) // 计算过期时间
	for _, addr := range addrs {    // 遍历地址列表
		// 从地址中移除/p2p/peer-id后缀
		addr, addrPid := peer.SplitAddr(addr)
		if addr == nil { // 如果地址为空
			log.Warnw("传入了空的多地址", "peer", p)
			continue
		}
		if addrPid != "" && addrPid != p { // 如果地址中的对等节点ID不匹配
			log.Warnf("传入了带有不同对等节点ID的p2p地址,发现: %s 期望: %s", addrPid, p)
			continue
		}

		a, found := mab.addrs.FindAddr(p, addr) // 查找地址
		if found {                              // 如果找到地址
			if ttl > 0 { // 如果TTL大于0
				// 如果地址已连接且新TTL不是连接状态,且未连接地址数超过限制
				if a.IsConnected() && !ttlIsConnected(ttl) && mab.addrs.NumUnconnectedAddrs() >= mab.maxUnconnectedAddrs {
					mab.addrs.Delete(a) // 删除地址
				} else {
					a.Addr = addr                         // 更新地址
					a.Expiry = exp                        // 更新过期时间
					a.TTL = ttl                           // 更新TTL
					mab.addrs.Update(a)                   // 更新地址状态
					mab.subManager.BroadcastAddr(p, addr) // 广播地址更新
				}
			} else { // 如果TTL为0
				mab.addrs.Delete(a) // 删除地址
			}
		} else { // 如果未找到地址
			if ttl > 0 { // 如果TTL大于0
				// 如果不是连接状态且未连接地址数超过限制
				if !ttlIsConnected(ttl) && mab.addrs.NumUnconnectedAddrs() >= mab.maxUnconnectedAddrs {
					continue
				}
				// 创建新的过期地址
				entry := &expiringAddr{Addr: addr, Expiry: exp, TTL: ttl, Peer: p}
				mab.addrs.Insert(entry)               // 插入地址
				mab.subManager.BroadcastAddr(p, addr) // 广播新地址
			}
		}
	}
}

// UpdateAddrs 将具有给定oldTTL的对等节点的地址更新为新的newTTL
// 参数:
//   - p: 对等节点ID
//   - oldTTL: 原TTL
//   - newTTL: 新TTL
func (mab *memoryAddrBook) UpdateAddrs(p peer.ID, oldTTL time.Duration, newTTL time.Duration) {
	mab.mu.Lock()         // 加写锁
	defer mab.mu.Unlock() // 解锁

	defer mab.maybeDeleteSignedPeerRecordUnlocked(p) // 尝试删除签名记录

	exp := mab.clock.Now().Add(newTTL)     // 计算新的过期时间
	for _, a := range mab.addrs.Addrs[p] { // 遍历对等节点的所有地址
		if oldTTL == a.TTL { // 如果TTL匹配
			if newTTL == 0 { // 如果新TTL为0
				mab.addrs.Delete(a) // 删除地址
			} else {
				// 如果原地址已连接且新TTL不是连接状态,且未连接地址数超过限制
				if ttlIsConnected(oldTTL) && !ttlIsConnected(newTTL) && mab.addrs.NumUnconnectedAddrs() >= mab.maxUnconnectedAddrs {
					mab.addrs.Delete(a) // 删除地址
				} else {
					a.TTL = newTTL      // 更新TTL
					a.Expiry = exp      // 更新过期时间
					mab.addrs.Update(a) // 更新地址状态
				}
			}
		}
	}
}

// Addrs 返回给定对等节点的所有已知(且有效)地址
// 参数:
//   - p: 对等节点ID
//
// 返回:
//   - []ma.Multiaddr: 有效地址列表
func (mab *memoryAddrBook) Addrs(p peer.ID) []ma.Multiaddr {
	mab.mu.RLock()                        // 加读锁
	defer mab.mu.RUnlock()                // 解锁
	if _, ok := mab.addrs.Addrs[p]; !ok { // 如果对等节点不存在
		log.Debugf("对等节点不存在: %v", p)
		return nil
	}
	return validAddrs(mab.clock.Now(), mab.addrs.Addrs[p]) // 返回有效地址
}

// validAddrs 返回当前时间点有效的地址列表
// 参数:
//   - now: 当前时间
//   - amap: 地址映射
//
// 返回:
//   - []ma.Multiaddr: 有效地址列表
func validAddrs(now time.Time, amap map[string]*expiringAddr) []ma.Multiaddr {
	good := make([]ma.Multiaddr, 0, len(amap)) // 创建结果切片
	if amap == nil {                           // 如果地址映射为空
		return good
	}
	for _, m := range amap { // 遍历所有地址
		if !m.ExpiredBy(now) { // 如果地址未过期
			good = append(good, m.Addr) // 添加到结果中
		}
	}
	return good
}

// GetPeerRecord 返回包含给定对等节点ID的PeerRecord的信封(如果存在)。
// 如果不存在已签名的PeerRecord,则返回nil。
// 参数:
//   - p: 对等节点ID
//
// 返回:
//   - *record.Envelope: 记录信封,不存在则返回nil
func (mab *memoryAddrBook) GetPeerRecord(p peer.ID) *record.Envelope {
	mab.mu.RLock()         // 加读锁
	defer mab.mu.RUnlock() // 解锁

	if _, ok := mab.addrs.Addrs[p]; !ok { // 如果对等节点不存在
		log.Debugf("尝试访问不存在的对等节点: %s", p)
		return nil
	}
	// 记录可能已过期,但尚未被垃圾回收
	if len(validAddrs(mab.clock.Now(), mab.addrs.Addrs[p])) == 0 { // 如果没有有效地址
		log.Debugf("没有有效地址")
		return nil
	}

	state := mab.signedPeerRecords[p] // 获取签名记录状态
	if state == nil {                 // 如果状态不存在
		log.Debugf("签名记录状态不存在")
		return nil
	}
	return state.Envelope // 返回信封
}

// ClearAddrs 移除所有先前存储的地址
// 参数:
//   - p: 对等节点ID
func (mab *memoryAddrBook) ClearAddrs(p peer.ID) {
	mab.mu.Lock()         // 加写锁
	defer mab.mu.Unlock() // 解锁

	delete(mab.signedPeerRecords, p)       // 删除签名记录
	for _, a := range mab.addrs.Addrs[p] { // 遍历所有地址
		mab.addrs.Delete(a) // 删除地址
	}
}

// AddrStream 返回一个通道,所有为给定对等节点ID发现的新地址都将在该通道上发布
// 参数:
//   - ctx: 上下文
//   - p: 对等节点ID
//
// 返回:
//   - <-chan ma.Multiaddr: 地址通道
func (mab *memoryAddrBook) AddrStream(ctx context.Context, p peer.ID) <-chan ma.Multiaddr {
	var initial []ma.Multiaddr // 初始地址列表

	mab.mu.RLock()                       // 加读锁
	if m, ok := mab.addrs.Addrs[p]; ok { // 如果对等节点存在
		initial = make([]ma.Multiaddr, 0, len(m)) // 创建初始地址切片
		for _, a := range m {                     // 遍历所有地址
			initial = append(initial, a.Addr) // 添加地址
		}
	}
	mab.mu.RUnlock() // 解锁

	return mab.subManager.AddrStream(ctx, p, initial) // 返回地址流
}

// addrSub 表示地址订阅
type addrSub struct {
	pubch chan ma.Multiaddr // 发布通道
	ctx   context.Context   // 上下文
}

// pubAddr 发布地址到通道
// 参数:
//   - a: 要发布的地址
func (s *addrSub) pubAddr(a ma.Multiaddr) {
	select {
	case s.pubch <- a: // 发送地址
	case <-s.ctx.Done(): // 上下文取消
	}
}

// AddrSubManager 是地址流的抽象发布-订阅管理器。从memoryAddrBook中提取出来以支持额外的实现。
type AddrSubManager struct {
	mu   sync.RWMutex           // 读写锁
	subs map[peer.ID][]*addrSub // 订阅映射
}

// NewAddrSubManager 初始化一个AddrSubManager
// 返回:
//   - *AddrSubManager: 新创建的管理器
func NewAddrSubManager() *AddrSubManager {
	return &AddrSubManager{
		subs: make(map[peer.ID][]*addrSub), // 初始化订阅映射
	}
}

// removeSub 由地址流协程内部使用,用于从管理器中移除订阅
// 参数:
//   - p: 对等节点ID
//   - s: 要移除的订阅
func (mgr *AddrSubManager) removeSub(p peer.ID, s *addrSub) {
	mgr.mu.Lock()         // 加写锁
	defer mgr.mu.Unlock() // 解锁

	subs := mgr.subs[p] // 获取对等节点的订阅列表
	if len(subs) == 1 { // 如果只有一个订阅
		if subs[0] != s { // 如果不是要移除的订阅
			return
		}
		delete(mgr.subs, p) // 删除整个订阅列表
		return
	}

	for i, v := range subs { // 遍历订阅列表
		if v == s { // 找到要移除的订阅
			subs[i] = subs[len(subs)-1]      // 将最后一个元素移到当前位置
			subs[len(subs)-1] = nil          // 清空最后一个元素
			mgr.subs[p] = subs[:len(subs)-1] // 截断切片
			return
		}
	}
}

// BroadcastAddr 向所有订阅的流广播新地址
// 参数:
//   - p: 对等节点ID
//   - addr: 要广播的地址
func (mgr *AddrSubManager) BroadcastAddr(p peer.ID, addr ma.Multiaddr) {
	mgr.mu.RLock()         // 加读锁
	defer mgr.mu.RUnlock() // 解锁

	if subs, ok := mgr.subs[p]; ok { // 如果存在订阅
		for _, sub := range subs { // 遍历所有订阅
			sub.pubAddr(addr) // 发布地址
		}
	}
}

// AddrStream 为给定的对等节点ID创建新的订阅,并用我们可能已经存储的任何地址预填充通道
// 参数:
//   - ctx: 上下文
//   - p: 对等节点ID
//   - initial: 初始地址列表
//
// 返回:
//   - <-chan ma.Multiaddr: 地址通道
func (mgr *AddrSubManager) AddrStream(ctx context.Context, p peer.ID, initial []ma.Multiaddr) <-chan ma.Multiaddr {
	sub := &addrSub{pubch: make(chan ma.Multiaddr), ctx: ctx} // 创建新订阅
	out := make(chan ma.Multiaddr)                            // 创建输出通道

	mgr.mu.Lock()                          // 加写锁
	mgr.subs[p] = append(mgr.subs[p], sub) // 添加订阅
	mgr.mu.Unlock()                        // 解锁

	sort.Sort(addrList(initial)) // 对初始地址列表排序

	go func(buffer []ma.Multiaddr) {
		defer close(out) // 关闭输出通道

		sent := make(map[string]struct{}, len(buffer)) // 已发送地址集合
		for _, a := range buffer {                     // 遍历初始地址
			sent[string(a.Bytes())] = struct{}{} // 标记为已发送
		}

		var outch chan ma.Multiaddr // 输出通道
		var next ma.Multiaddr       // 下一个要发送的地址
		if len(buffer) > 0 {        // 如果缓冲区不为空
			next = buffer[0]    // 设置下一个地址
			buffer = buffer[1:] // 更新缓冲区
			outch = out         // 设置输出通道
		}

		for {
			select {
			case outch <- next: // 发送地址
				if len(buffer) > 0 { // 如果缓冲区不为空
					next = buffer[0]    // 设置下一个地址
					buffer = buffer[1:] // 更新缓冲区
				} else {
					outch = nil // 清空输出通道
					next = nil  // 清空下一个地址
				}
			case naddr := <-sub.pubch: // 接收新地址
				if _, ok := sent[string(naddr.Bytes())]; ok { // 如果地址已发送
					continue
				}
				sent[string(naddr.Bytes())] = struct{}{} // 标记为已发送

				if next == nil { // 如果没有下一个地址
					next = naddr // 设置为下一个地址
					outch = out  // 设置输出通道
				} else {
					buffer = append(buffer, naddr) // 添加到缓冲区
				}
			case <-ctx.Done(): // 上下文取消
				mgr.removeSub(p, sub) // 移除订阅
				return
			}
		}
	}(initial)

	return out // 返回输出通道
}
