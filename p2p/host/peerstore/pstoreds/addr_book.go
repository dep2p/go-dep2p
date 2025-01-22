package pstoreds

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/peer"
	pstore "github.com/dep2p/go-dep2p/core/peerstore"
	"github.com/dep2p/go-dep2p/core/record"
	"github.com/dep2p/go-dep2p/p2p/host/peerstore/pstoreds/pb"
	"github.com/dep2p/go-dep2p/p2p/host/peerstore/pstoremem"

	ds "github.com/dep2p/go-dep2p/datastore"
	"github.com/dep2p/go-dep2p/datastore/query"
	b32 "github.com/dep2p/go-dep2p/multiformats/base32"
	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	logging "github.com/dep2p/log"
	"github.com/hashicorp/golang-lru/arc/v2"
	"google.golang.org/protobuf/proto"
)

// ttlWriteMode 定义TTL写入模式
type ttlWriteMode int

const (
	ttlOverride ttlWriteMode = iota // 覆盖模式
	ttlExtend                       // 延长模式
)

var (
	// log 日志记录器
	log = logging.Logger("host-peerstore-pstoreds")

	// addrBookBase 地址簿基础路径
	// peer地址存储在数据库中的键模式:
	// /peers/addrs/<b32 peer id 无填充>
	addrBookBase = ds.NewKey("/peers/addrs")
)

// addrsRecord 为AddrBookRecord添加锁和元数据
type addrsRecord struct {
	sync.RWMutex            // 读写锁
	*pb.AddrBookRecord      // 地址簿记录
	dirty              bool // 脏标记,表示记录是否被修改
}

// flush 将记录写入数据存储
// 如果记录标记为删除则调用ds.Delete,否则调用ds.Put
// 在锁内调用
//
// 参数:
//   - write: ds.Write 数据存储写入接口
//
// 返回:
//   - error 写入错误
func (r *addrsRecord) flush(write ds.Write) (err error) {
	// 构造键
	key := addrBookBase.ChildString(b32.RawStdEncoding.EncodeToString(r.Id))

	// 如果地址为空,删除记录
	if len(r.Addrs) == 0 {
		if err = write.Delete(context.TODO(), key); err == nil {
			r.dirty = false
		}
		log.Debugf("删除地址簿记录: %v", key)
		return err
	}

	// 序列化记录
	data, err := proto.Marshal(r)
	if err != nil {
		log.Debugf("序列化地址簿记录失败: %v", err)
		return err
	}
	// 写入数据存储
	if err = write.Put(context.TODO(), key, data); err != nil {
		log.Debugf("写入地址簿记录失败: %v", err)
		return err
	}
	// 写入成功,清除脏标记
	r.dirty = false
	return nil
}

// clean 对记录进行清理
// 返回值表示记录是否被修改
//
// clean执行以下操作:
// * 按过期时间排序地址(最早过期的在前)
// * 移除过期地址
//
// 在以下情况会调用clean:
// * 访问条目时
// * 执行周期性GC时
// * 条目被修改后(如添加或删除地址、更新TTL等)
//
// 参数:
//   - now: time.Time 当前时间
//
// 返回:
//   - bool 是否发生变更
func (r *addrsRecord) clean(now time.Time) (chgd bool) {
	nowUnix := now.Unix()
	addrsLen := len(r.Addrs)

	// 如果记录未修改且没有过期地址,直接返回
	if !r.dirty && !r.hasExpiredAddrs(nowUnix) {
		return false
	}

	// 如果是空记录,标记需要写入
	if addrsLen == 0 {
		return true
	}

	// 如果记录被修改且有多个地址,按过期时间排序
	if r.dirty && addrsLen > 1 {
		sort.Slice(r.Addrs, func(i, j int) bool {
			return r.Addrs[i].Expiry < r.Addrs[j].Expiry
		})
	}

	// 移除过期地址
	r.Addrs = removeExpired(r.Addrs, nowUnix)

	return r.dirty || len(r.Addrs) != addrsLen
}

// hasExpiredAddrs 检查是否有过期地址
//
// 参数:
//   - now: int64 当前时间戳
//
// 返回:
//   - bool 是否存在过期地址
func (r *addrsRecord) hasExpiredAddrs(now int64) bool {
	if len(r.Addrs) > 0 && r.Addrs[0].Expiry <= now {
		return true
	}
	return false
}

// removeExpired 移除过期地址
//
// 参数:
//   - entries: []*pb.AddrBookRecord_AddrEntry 地址条目列表
//   - now: int64 当前时间戳
//
// 返回:
//   - []*pb.AddrBookRecord_AddrEntry 移除过期地址后的列表
func removeExpired(entries []*pb.AddrBookRecord_AddrEntry, now int64) []*pb.AddrBookRecord_AddrEntry {
	// 由于地址按过期时间排序,找到第一个未过期的地址
	pivot := -1
	for i, addr := range entries {
		if addr.Expiry > now {
			break
		}
		pivot = i
	}

	return entries[pivot+1:]
}

// dsAddrBook 是一个基于数据存储的地址簿,带有GC过程来清除过期条目
// 使用内存中的地址流管理器
type dsAddrBook struct {
	ctx  context.Context // 上下文
	opts Options         // 选项

	cache       cache[peer.ID, *addrsRecord] // 缓存
	ds          ds.Batching                  // 数据存储
	gc          *dsAddrBookGc                // GC管理器
	subsManager *pstoremem.AddrSubManager    // 订阅管理器

	// 控制子goroutine生命周期
	childrenDone sync.WaitGroup // 等待组
	cancelFn     func()         // 取消函数

	clock clock // 时钟接口
}

// clock 定义时钟接口
type clock interface {
	Now() time.Time                         // 获取当前时间
	After(d time.Duration) <-chan time.Time // 定时器
}

// realclock 实现真实时钟
type realclock struct{}

// Now 获取当前时间
//
// 返回:
//   - time.Time 当前时间
func (rc realclock) Now() time.Time {
	return time.Now()
}

// After 创建定时器
//
// 参数:
//   - d: time.Duration 延迟时间
//
// 返回:
//   - <-chan time.Time 定时通道
func (rc realclock) After(d time.Duration) <-chan time.Time {
	return time.After(d)
}

// 确保 dsAddrBook 实现了 pstore.AddrBook 接口
var _ pstore.AddrBook = (*dsAddrBook)(nil)

// 确保 dsAddrBook 实现了 pstore.CertifiedAddrBook 接口
var _ pstore.CertifiedAddrBook = (*dsAddrBook)(nil)

// NewAddrBook 初始化一个基于数据存储的地址簿
//
// 参数:
//   - ctx: context.Context 上下文
//   - store: ds.Batching 数据存储接口
//   - opts: Options 配置选项
//
// 返回:
//   - *dsAddrBook 地址簿实例
//   - error 错误信息
//
// 说明:
// 它可以作为 pstoremem(内存存储)的替代品,可以与任何实现 ds.Batching 接口的数据存储一起工作。
//
// 地址和对等点记录被序列化为 protobuf,每个对等点存储一个数据条目,同时包含控制地址过期的元数据。
// 为了减轻磁盘访问和序列化开销,我们在内部使用读/写 ARC 缓存,其大小可通过 Options.CacheSize 调整。
//
// 用户可以选择两种 GC 算法:
//
//  1. 前瞻 GC:通过维护一个时间索引列表来最小化完整存储遍历,该列表包含在 Options.GCLookaheadInterval
//     指定的时间段内需要访问的条目。这在 TTL 差异较大且数据存储的原生迭代器按字典序返回条目的场景中很有用。
//     通过设置 Options.GCLookaheadInterval > 0 启用此模式。前瞻窗口是跳跃式的,而不是滑动式的。
//     清除操作仅在前瞻窗口内以 Options.GCPurgeInterval 的周期执行。
//
//  2. 完全清除 GC(默认):以 Options.GCPurgeInterval 的周期对存储进行完整访问。当可能的 TTL 值范围较小
//     且值本身也很极端时很有用,例如在其他 dep2p 模块中常用的 10 分钟或永久值。在这种情况下,使用前瞻窗口
//     优化意义不大。
func NewAddrBook(ctx context.Context, store ds.Batching, opts Options) (ab *dsAddrBook, err error) {
	// 创建带取消的上下文
	ctx, cancelFn := context.WithCancel(ctx)

	// 初始化地址簿结构
	ab = &dsAddrBook{
		ctx:         ctx,
		ds:          store,
		opts:        opts,
		cancelFn:    cancelFn,
		subsManager: pstoremem.NewAddrSubManager(),
		clock:       realclock{},
	}

	// 如果提供了自定义时钟,则使用自定义时钟
	if opts.Clock != nil {
		ab.clock = opts.Clock
	}

	// 根据缓存大小配置初始化缓存
	if opts.CacheSize > 0 {
		if ab.cache, err = arc.NewARC[peer.ID, *addrsRecord](int(opts.CacheSize)); err != nil {
			log.Debugf("初始化ARC缓存失败: %v", err)
			return nil, err
		}
	} else {
		ab.cache = new(noopCache[peer.ID, *addrsRecord])
	}

	// 初始化垃圾回收器
	if ab.gc, err = newAddressBookGc(ctx, ab); err != nil {
		log.Debugf("初始化垃圾回收器失败: %v", err)
		return nil, err
	}

	return ab, nil
}

// Close 关闭地址簿
//
// 返回:
//   - error 错误信息
func (ab *dsAddrBook) Close() error {
	// 调用取消函数
	ab.cancelFn()
	// 等待所有子goroutine完成
	ab.childrenDone.Wait()
	return nil
}

// loadRecord 从缓存或数据存储中加载记录
// 如果缓存未命中则从数据存储中获取,如果peer不存在则返回新初始化的记录
// 在返回记录前会调用clean()方法。如果记录发生变化且update为true,则将结果状态保存到数据存储中
// 如果cache为true,则从数据存储加载时将记录插入缓存
//
// 参数:
//   - id: peer.ID peer标识
//   - cache: bool 是否缓存
//   - update: bool 是否更新
//
// 返回:
//   - *addrsRecord 地址记录
//   - error 错误信息
func (ab *dsAddrBook) loadRecord(id peer.ID, cache bool, update bool) (pr *addrsRecord, err error) {
	// 尝试从缓存获取
	if pr, ok := ab.cache.Get(id); ok {
		pr.Lock()
		defer pr.Unlock()

		// 清理记录并根据需要更新
		if pr.clean(ab.clock.Now()) && update {
			err = pr.flush(ab.ds)
		}
		log.Debugf("从缓存加载地址簿记录: %v", id)
		return pr, err
	}

	// 初始化新记录
	pr = &addrsRecord{AddrBookRecord: &pb.AddrBookRecord{}}
	// 构造存储键
	key := addrBookBase.ChildString(b32.RawStdEncoding.EncodeToString([]byte(id)))
	// 从数据存储获取
	data, err := ab.ds.Get(context.TODO(), key)

	switch err {
	case ds.ErrNotFound:
		// 记录不存在,创建新记录
		err = nil
		pr.Id = []byte(id)
	case nil:
		// 反序列化记录
		if err := proto.Unmarshal(data, pr); err != nil {
			log.Debugf("反序列化地址簿记录失败: %v", err)
			return nil, err
		}
		// 清理记录并根据需要更新
		if pr.clean(ab.clock.Now()) && update {
			err = pr.flush(ab.ds)
		}
	default:
		log.Debugf("加载地址簿记录失败: %v", err)
		return nil, err
	}

	// 根据需要缓存记录
	if cache {
		ab.cache.Add(id, pr)
	}
	return pr, err
}

// AddAddr 添加一个新地址(如果不在地址簿中)
//
// 参数:
//   - p: peer.ID peer标识
//   - addr: ma.Multiaddr 多地址
//   - ttl: time.Duration 过期时间
func (ab *dsAddrBook) AddAddr(p peer.ID, addr ma.Multiaddr, ttl time.Duration) {
	// 调用AddAddrs添加单个地址
	ab.AddAddrs(p, []ma.Multiaddr{addr}, ttl)
}

// AddAddrs 添加多个新地址(如果不在地址簿中)
//
// 参数:
//   - p: peer.ID peer标识
//   - addrs: []ma.Multiaddr 多地址列表
//   - ttl: time.Duration 过期时间
func (ab *dsAddrBook) AddAddrs(p peer.ID, addrs []ma.Multiaddr, ttl time.Duration) {
	// 检查ttl是否有效
	if ttl <= 0 {
		return
	}
	// 清理地址
	addrs = cleanAddrs(addrs, p)
	// 设置地址,使用ttlExtend模式,不更新序列号
	ab.setAddrs(p, addrs, ttl, ttlExtend, false)
}

// ConsumePeerRecord 添加来自签名的peer.PeerRecord(包含在record.Envelope中)的地址,
// 这些地址将在给定的TTL后过期。
// 更多详情参见 https://godoc.org/github.com/dep2p/core/peerstore#CertifiedAddrBook
//
// 参数:
//   - recordEnvelope: *record.Envelope 包含PeerRecord的信封
//   - ttl: time.Duration 过期时间
//
// 返回:
//   - bool 是否成功消费记录
//   - error 错误信息
func (ab *dsAddrBook) ConsumePeerRecord(recordEnvelope *record.Envelope, ttl time.Duration) (bool, error) {
	// 从信封中获取记录
	r, err := recordEnvelope.Record()
	if err != nil {
		log.Debugf("从信封中获取记录失败: %v", err)
		return false, err
	}
	// 类型断言为PeerRecord
	rec, ok := r.(*peer.PeerRecord)
	if !ok {
		log.Debugf("信封未包含PeerRecord")
		return false, fmt.Errorf("信封未包含PeerRecord")
	}
	// 验证签名密钥是否匹配PeerRecord中的PeerID
	if !rec.PeerID.MatchesPublicKey(recordEnvelope.PublicKey) {
		log.Debugf("签名密钥与PeerRecord中的PeerID不匹配")
		return false, fmt.Errorf("签名密钥与PeerRecord中的PeerID不匹配")
	}

	// 确保信封中的序列号大于等于之前接收到的序列号
	// 当相等时更新以延长TTL
	if ab.latestPeerRecordSeq(rec.PeerID) > rec.Seq {
		return false, nil
	}

	// 清理地址
	addrs := cleanAddrs(rec.Addrs, rec.PeerID)
	// 设置地址
	err = ab.setAddrs(rec.PeerID, addrs, ttl, ttlExtend, true)
	if err != nil {
		log.Debugf("设置地址失败: %v", err)
		return false, err
	}

	// 存储签名的PeerRecord
	err = ab.storeSignedPeerRecord(rec.PeerID, recordEnvelope, rec)
	if err != nil {
		log.Debugf("存储签名的PeerRecord失败: %v", err)
		return false, err
	}
	return true, nil
}

// latestPeerRecordSeq 获取最新的PeerRecord序列号
//
// 参数:
//   - p: peer.ID peer标识
//
// 返回:
//   - uint64 序列号
func (ab *dsAddrBook) latestPeerRecordSeq(p peer.ID) uint64 {
	// 加载记录
	pr, err := ab.loadRecord(p, true, false)
	if err != nil {
		// 忽略错误,因为我们不希望在这种情况下存储新记录失败
		log.Debugf("无法加载记录: %v", err)
		return 0
	}
	pr.RLock()
	defer pr.RUnlock()

	// 如果没有地址或认证记录,返回0
	if len(pr.Addrs) == 0 || pr.CertifiedRecord == nil || len(pr.CertifiedRecord.Raw) == 0 {
		return 0
	}
	return pr.CertifiedRecord.Seq
}

// storeSignedPeerRecord 存储签名的PeerRecord
//
// 参数:
//   - p: peer.ID peer标识
//   - envelope: *record.Envelope 信封
//   - rec: *peer.PeerRecord peer记录
//
// 返回:
//   - error 错误信息
func (ab *dsAddrBook) storeSignedPeerRecord(p peer.ID, envelope *record.Envelope, rec *peer.PeerRecord) error {
	// 序列化信封
	envelopeBytes, err := envelope.Marshal()
	if err != nil {
		log.Debugf("序列化信封失败: %v", err)
		return err
	}
	// 重新加载记录并添加路由状态
	// 这必须在添加地址之后完成,因为如果尝试刷新没有地址的数据存储记录,
	// 它将被删除
	pr, err := ab.loadRecord(p, true, false)
	if err != nil {
		log.Debugf("加载记录失败: %v", err)
		return err
	}
	pr.Lock()
	defer pr.Unlock()
	// 设置认证记录
	pr.CertifiedRecord = &pb.AddrBookRecord_CertifiedRecord{
		Seq: rec.Seq,
		Raw: envelopeBytes,
	}
	pr.dirty = true
	err = pr.flush(ab.ds)
	return err
}

// GetPeerRecord 返回包含给定peer id的peer.PeerRecord的record.Envelope
// 如果不存在签名的PeerRecord则返回nil
//
// 参数:
//   - p: peer.ID peer标识
//
// 返回:
//   - *record.Envelope 包含PeerRecord的信封
func (ab *dsAddrBook) GetPeerRecord(p peer.ID) *record.Envelope {
	// 加载记录
	pr, err := ab.loadRecord(p, true, false)
	if err != nil {
		log.Errorf("无法为peer %s加载记录: %v", p, err)
		return nil
	}
	pr.RLock()
	defer pr.RUnlock()
	// 检查记录是否有效
	if pr.CertifiedRecord == nil || len(pr.CertifiedRecord.Raw) == 0 || len(pr.Addrs) == 0 {
		log.Debugf("记录无效: %v", pr)
		return nil
	}
	// 解析信封
	state, _, err := record.ConsumeEnvelope(pr.CertifiedRecord.Raw, peer.PeerRecordEnvelopeDomain)
	if err != nil {
		log.Errorf("解析peer %s的存储签名peer记录时出错: %v", p, err)
		return nil
	}
	return state
}

// SetAddr 在地址簿中添加或更新地址的TTL
//
// 参数:
//   - p: peer.ID peer标识
//   - addr: ma.Multiaddr 多地址
//   - ttl: time.Duration 过期时间
func (ab *dsAddrBook) SetAddr(p peer.ID, addr ma.Multiaddr, ttl time.Duration) {
	ab.SetAddrs(p, []ma.Multiaddr{addr}, ttl)
}

// SetAddrs 在地址簿中添加或更新多个地址的TTL
//
// 参数:
//   - p: peer.ID peer标识
//   - addrs: []ma.Multiaddr 多地址列表
//   - ttl: time.Duration 过期时间
func (ab *dsAddrBook) SetAddrs(p peer.ID, addrs []ma.Multiaddr, ttl time.Duration) {
	// 清理地址
	addrs = cleanAddrs(addrs, p)
	// 如果ttl小于等于0,删除地址
	if ttl <= 0 {
		ab.deleteAddrs(p, addrs)
		return
	}
	// 设置地址
	ab.setAddrs(p, addrs, ttl, ttlOverride, false)
}

// UpdateAddrs 更新给定peer和TTL组合的地址的TTL
//
// 参数:
//   - p: peer.ID peer标识
//   - oldTTL: time.Duration 旧的过期时间
//   - newTTL: time.Duration 新的过期时间
func (ab *dsAddrBook) UpdateAddrs(p peer.ID, oldTTL time.Duration, newTTL time.Duration) {
	// 加载记录
	pr, err := ab.loadRecord(p, true, false)
	if err != nil {
		log.Errorf("更新peer %s的ttl失败: %s\n", p, err)
		return
	}

	pr.Lock()
	defer pr.Unlock()

	// 计算新的过期时间
	newExp := ab.clock.Now().Add(newTTL).Unix()
	// 更新匹配的TTL
	for _, entry := range pr.Addrs {
		if entry.Ttl != int64(oldTTL) {
			continue
		}
		entry.Ttl, entry.Expiry = int64(newTTL), newExp
		pr.dirty = true
	}

	// 清理并刷新记录
	if pr.clean(ab.clock.Now()) {
		pr.flush(ab.ds)
	}
}

// Addrs 返回给定peer的所有未过期地址
//
// 参数:
//   - p: peer.ID peer标识
//
// 返回:
//   - []ma.Multiaddr 多地址列表
func (ab *dsAddrBook) Addrs(p peer.ID) []ma.Multiaddr {
	// 加载记录
	pr, err := ab.loadRecord(p, true, true)
	if err != nil {
		log.Warnf("查询地址时加载peer %s的peerstore条目失败, 错误: %v", p, err)
		return nil
	}

	pr.RLock()
	defer pr.RUnlock()

	// 转换地址
	addrs := make([]ma.Multiaddr, len(pr.Addrs))
	for i, a := range pr.Addrs {
		var err error
		addrs[i], err = ma.NewMultiaddrBytes(a.Addr)
		if err != nil {
			log.Warnf("查询地址时解析peer %v的peerstore条目失败, 错误: %v", p, err)
			return nil
		}
	}
	return addrs
}

// PeersWithAddrs 返回地址簿中所有有地址的peer ID列表
//
// 返回:
//   - peer.IDSlice peer ID切片
func (ab *dsAddrBook) PeersWithAddrs() peer.IDSlice {
	// 从数据存储中获取唯一的peer ID
	ids, err := uniquePeerIds(ab.ds, addrBookBase, func(result query.Result) string {
		return ds.RawKey(result.Key).Name()
	})
	if err != nil {
		log.Errorf("获取有地址的peer时出错: %v", err)
	}
	return ids
}

// AddrStream 返回一个通道,用于发布给定peer ID的所有新发现的地址
//
// 参数:
//   - ctx: context.Context 上下文
//   - p: peer.ID peer标识
//
// 返回:
//   - <-chan ma.Multiaddr 地址通道
func (ab *dsAddrBook) AddrStream(ctx context.Context, p peer.ID) <-chan ma.Multiaddr {
	// 获取初始地址列表
	initial := ab.Addrs(p)
	// 返回订阅管理器的地址流
	return ab.subsManager.AddrStream(ctx, p, initial)
}

// ClearAddrs 删除peer ID的所有已知地址
//
// 参数:
//   - p: peer.ID peer标识
func (ab *dsAddrBook) ClearAddrs(p peer.ID) {
	// 从缓存中移除
	ab.cache.Remove(p)

	// 构造存储键
	key := addrBookBase.ChildString(b32.RawStdEncoding.EncodeToString([]byte(p)))
	// 从数据存储中删除
	if err := ab.ds.Delete(context.TODO(), key); err != nil {
		log.Errorf("清除peer %s的地址失败: %v", p, err)
	}
}

// setAddrs 设置peer的地址列表
//
// 参数:
//   - p: peer.ID peer标识
//   - addrs: []ma.Multiaddr 地址列表
//   - ttl: time.Duration 过期时间
//   - mode: ttlWriteMode TTL写入模式
//   - signed: bool 是否签名
//
// 返回:
//   - error 错误信息
func (ab *dsAddrBook) setAddrs(p peer.ID, addrs []ma.Multiaddr, ttl time.Duration, mode ttlWriteMode, signed bool) (err error) {
	// 如果地址列表为空则返回
	if len(addrs) == 0 {
		return nil
	}

	// 加载记录
	pr, err := ab.loadRecord(p, true, false)
	if err != nil {
		log.Debugf("设置地址时加载peer %s的peerstore条目失败, 错误: %v", p, err)
		return fmt.Errorf("设置地址时加载peer %s的peerstore条目失败, 错误: %v", p, err)
	}

	pr.Lock()
	defer pr.Unlock()

	// 计算新的过期时间
	newExp := ab.clock.Now().Add(ttl).Unix()
	// 构建地址映射
	addrsMap := make(map[string]*pb.AddrBookRecord_AddrEntry, len(pr.Addrs))
	for _, addr := range pr.Addrs {
		addrsMap[string(addr.Addr)] = addr
	}

	// 更新已存在地址的函数
	updateExisting := func(incoming ma.Multiaddr) *pb.AddrBookRecord_AddrEntry {
		existingEntry := addrsMap[string(incoming.Bytes())]
		if existingEntry == nil {
			return nil
		}

		switch mode {
		case ttlOverride:
			// 覆盖模式:直接使用新的TTL和过期时间
			existingEntry.Ttl = int64(ttl)
			existingEntry.Expiry = newExp
		case ttlExtend:
			// 延长模式:使用较大的TTL和过期时间
			if int64(ttl) > existingEntry.Ttl {
				existingEntry.Ttl = int64(ttl)
			}
			if newExp > existingEntry.Expiry {
				existingEntry.Expiry = newExp
			}
		default:
			panic("BUG: 未实现的TTL模式")
		}
		return existingEntry
	}

	// 处理地址列表
	var entries []*pb.AddrBookRecord_AddrEntry
	for _, incoming := range addrs {
		existingEntry := updateExisting(incoming)

		if existingEntry == nil {
			// 新地址,添加并广播
			entry := &pb.AddrBookRecord_AddrEntry{
				Addr:   incoming.Bytes(),
				Ttl:    int64(ttl),
				Expiry: newExp,
			}
			entries = append(entries, entry)

			// 广播新地址
			ab.subsManager.BroadcastAddr(p, incoming)
		}
	}

	// 添加新地址到记录
	pr.Addrs = append(pr.Addrs, entries...)

	// 标记为脏并清理
	pr.dirty = true
	pr.clean(ab.clock.Now())
	return pr.flush(ab.ds)
}

// deleteInPlace 原地删除地址,避免复制直到遇到第一个需要删除的地址
// 不保持顺序,但在写入磁盘前会重新排序
//
// 参数:
//   - s: []*pb.AddrBookRecord_AddrEntry 地址条目切片
//   - addrs: []ma.Multiaddr 要删除的地址列表
//
// 返回:
//   - []*pb.AddrBookRecord_AddrEntry 处理后的地址条目切片
func deleteInPlace(s []*pb.AddrBookRecord_AddrEntry, addrs []ma.Multiaddr) []*pb.AddrBookRecord_AddrEntry {
	if s == nil || len(addrs) == 0 {
		return s
	}
	survived := len(s)
Outer:
	for i, addr := range s {
		for _, del := range addrs {
			if !bytes.Equal(del.Bytes(), addr.Addr) {
				continue
			}
			survived--
			// 如果没有存活的地址,退出
			if survived == 0 {
				break Outer
			}
			s[i] = s[survived]
			// 已处理s[i],继续下一个
			continue Outer
		}
	}
	return s[:survived]
}

// deleteAddrs 删除peer的指定地址
//
// 参数:
//   - p: peer.ID peer标识
//   - addrs: []ma.Multiaddr 要删除的地址列表
//
// 返回:
//   - error 错误信息
func (ab *dsAddrBook) deleteAddrs(p peer.ID, addrs []ma.Multiaddr) (err error) {
	// 加载记录
	pr, err := ab.loadRecord(p, false, false)
	if err != nil {
		log.Debugf("删除地址时加载peer %v的peerstore条目失败, 错误: %v", p, err)
		return fmt.Errorf("删除地址时加载peer %v的peerstore条目失败, 错误: %v", p, err)
	}

	pr.Lock()
	defer pr.Unlock()

	if pr.Addrs == nil {
		return nil
	}

	// 删除地址
	pr.Addrs = deleteInPlace(pr.Addrs, addrs)

	// 标记为脏并清理
	pr.dirty = true
	pr.clean(ab.clock.Now())
	return pr.flush(ab.ds)
}

// cleanAddrs 清理地址列表,移除/p2p/peer-id后缀
//
// 参数:
//   - addrs: []ma.Multiaddr 要清理的地址列表
//   - pid: peer.ID peer标识
//
// 返回:
//   - []ma.Multiaddr 清理后的地址列表
func cleanAddrs(addrs []ma.Multiaddr, pid peer.ID) []ma.Multiaddr {
	clean := make([]ma.Multiaddr, 0, len(addrs))
	for _, addr := range addrs {
		// 从地址中移除/p2p/peer-id后缀
		addr, addrPid := peer.SplitAddr(addr)
		if addr == nil {
			log.Warnw("传入了空地址", "peer", pid)
			continue
		}
		if addrPid != "" && addrPid != pid {
			log.Warnf("传入的p2p地址包含不同的peerID. 发现: %s, 期望: %s", addrPid, pid)
			continue
		}
		clean = append(clean, addr)
	}
	return clean
}
