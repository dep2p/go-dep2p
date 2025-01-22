package pstoreds

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/p2p/host/peerstore/pstoreds/pb"
	"google.golang.org/protobuf/proto"

	ds "github.com/dep2p/go-dep2p/datastore"
	"github.com/dep2p/go-dep2p/datastore/query"
	b32 "github.com/dep2p/go-dep2p/multiformats/base32"
)

var (
	// GC 预查条目存储在以下键模式中:
	// /peers/gc/addrs/<下次访问的unix时间戳>/<peer ID b32> => nil
	// 在具有字典序键顺序的数据库中,这种时间索引允许我们只访问感兴趣的时间片
	gcLookaheadBase = ds.NewKey("/peers/gc/addrs")

	// 查询
	// 清除预查查询
	purgeLookaheadQuery = query.Query{
		Prefix:   gcLookaheadBase.String(),
		Orders:   []query.Order{query.OrderByFunction(orderByTimestampInKey)},
		KeysOnly: true,
	}

	// 清除存储查询
	purgeStoreQuery = query.Query{
		Prefix:   addrBookBase.String(),
		Orders:   []query.Order{query.OrderByKey{}},
		KeysOnly: false,
	}

	// 填充预查查询
	populateLookaheadQuery = query.Query{
		Prefix:   addrBookBase.String(),
		Orders:   []query.Order{query.OrderByKey{}},
		KeysOnly: true,
	}
)

// dsAddrBookGc 负责基于数据存储的地址簿的垃圾回收
type dsAddrBookGc struct {
	ctx              context.Context // 上下文
	ab               *dsAddrBook     // 地址簿
	running          chan struct{}   // 运行状态通道
	lookaheadEnabled bool            // 是否启用预查
	purgeFunc        func()          // 清除函数
	currWindowEnd    int64           // 当前窗口结束时间
}

// newAddressBookGc 创建新的地址簿垃圾回收器
// 参数:
//   - ctx: context.Context 上下文
//   - ab: *dsAddrBook 地址簿实例
//
// 返回:
//   - *dsAddrBookGc 垃圾回收器实例
//   - error 错误信息
func newAddressBookGc(ctx context.Context, ab *dsAddrBook) (*dsAddrBookGc, error) {
	// 检查清除间隔是否为负
	if ab.opts.GCPurgeInterval < 0 {
		log.Debugf("提供了负的GC清除间隔: %s", ab.opts.GCPurgeInterval)
		return nil, fmt.Errorf("提供了负的GC清除间隔: %s", ab.opts.GCPurgeInterval)
	}
	// 检查预查间隔是否为负
	if ab.opts.GCLookaheadInterval < 0 {
		log.Debugf("提供了负的GC预查间隔: %s", ab.opts.GCLookaheadInterval)
		return nil, fmt.Errorf("提供了负的GC预查间隔: %s", ab.opts.GCLookaheadInterval)
	}
	// 检查初始延迟是否为负
	if ab.opts.GCInitialDelay < 0 {
		log.Debugf("提供了负的GC初始延迟: %s", ab.opts.GCInitialDelay)
		return nil, fmt.Errorf("提供了负的GC初始延迟: %s", ab.opts.GCInitialDelay)
	}
	// 检查预查间隔是否大于清除间隔
	if ab.opts.GCLookaheadInterval > 0 && ab.opts.GCLookaheadInterval < ab.opts.GCPurgeInterval {
		log.Debugf("预查间隔必须大于清除间隔,分别为: %s, %s",
			ab.opts.GCLookaheadInterval, ab.opts.GCPurgeInterval)
		return nil, fmt.Errorf("预查间隔必须大于清除间隔,分别为: %s, %s",
			ab.opts.GCLookaheadInterval, ab.opts.GCPurgeInterval)
	}

	// 是否启用预查
	lookaheadEnabled := ab.opts.GCLookaheadInterval > 0
	gc := &dsAddrBookGc{
		ctx:              ctx,
		ab:               ab,
		running:          make(chan struct{}, 1),
		lookaheadEnabled: lookaheadEnabled,
	}

	// 根据是否启用预查设置清除函数
	if lookaheadEnabled {
		gc.purgeFunc = gc.purgeLookahead
	} else {
		gc.purgeFunc = gc.purgeStore
	}

	// 如果清除间隔大于0,启动GC定时器
	if ab.opts.GCPurgeInterval > 0 {
		gc.ab.childrenDone.Add(1)
		go gc.background()
	}

	return gc, nil
}

// background 在后台定期运行垃圾回收
// 该方法应作为goroutine运行
func (gc *dsAddrBookGc) background() {
	// 完成时减少计数
	defer gc.ab.childrenDone.Done()

	// 等待初始延迟或上下文取消
	select {
	case <-gc.ab.clock.After(gc.ab.opts.GCInitialDelay):
	case <-gc.ab.ctx.Done():
		// 如果在延迟结束前被取消/关闭则退出
		return
	}

	// 创建清除定时器
	purgeTimer := time.NewTicker(gc.ab.opts.GCPurgeInterval)
	defer purgeTimer.Stop()

	var lookaheadCh <-chan time.Time
	// 如果启用预查,创建预查定时器
	if gc.lookaheadEnabled {
		lookaheadTimer := time.NewTicker(gc.ab.opts.GCLookaheadInterval)
		lookaheadCh = lookaheadTimer.C
		gc.populateLookahead() // 立即执行一次预查
		defer lookaheadTimer.Stop()
	}

	// 主循环
	for {
		select {
		case <-purgeTimer.C:
			// 执行清除
			gc.purgeFunc()

		case <-lookaheadCh:
			// 如果预查被禁用(nil Duration)则永远不会触发
			gc.populateLookahead()

		case <-gc.ctx.Done():
			return
		}
	}
}

// purgeLookahead 执行一次带预查的GC清除循环
// 如果启用预查,则在预查窗口内操作;否则访问数据存储中的所有条目,删除已过期的地址
func (gc *dsAddrBookGc) purgeLookahead() {
	// 检查是否有其他清除正在运行
	select {
	case gc.running <- struct{}{}:
		defer func() { <-gc.running }()
	default:
		// 如果预查正在运行则退出
		return
	}

	var id peer.ID
	// 创建空记录以重用并避免分配
	record := &addrsRecord{AddrBookRecord: &pb.AddrBookRecord{}}
	batch, err := newCyclicBatch(gc.ab.ds, defaultOpsPerCyclicBatch)
	if err != nil {
		log.Warnf("创建批处理以清除GC条目时失败: %v", err)
	}

	// dropInError 删除无法解析的GC条目
	// 这是为了安全。如果我们修改了键的格式,这是一个避免在旧数据库上运行新版本时累积垃圾的方法
	dropInError := func(key ds.Key, err error, msg string) {
		if err != nil {
			log.Warnf("在%s记录时失败,GC键: %v, 错误: %v; 正在删除", msg, key, err)
		}
		if err = batch.Delete(context.TODO(), key); err != nil {
			log.Warnf("删除损坏的GC预查条目失败: %v, 错误: %v", key, err)
		}
	}

	// dropOrReschedule 在条目被正确清理后删除GC键
	// 如果下一个最早过期时间仍在当前窗口内,可能会重新安排另一次访问
	dropOrReschedule := func(key ds.Key, ar *addrsRecord) {
		if err := batch.Delete(context.TODO(), key); err != nil {
			log.Warnf("删除预查条目失败: %v, 错误: %v", key, err)
		}

		// 如果记录需要在此窗口内再次访问,则重新添加记录
		if len(ar.Addrs) != 0 && ar.Addrs[0].Expiry <= gc.currWindowEnd {
			gcKey := gcLookaheadBase.ChildString(fmt.Sprintf("%d/%s", ar.Addrs[0].Expiry, key.Name()))
			if err := batch.Put(context.TODO(), gcKey, []byte{}); err != nil {
				log.Warnf("添加新GC键失败: %v, 错误: %v", gcKey, err)
			}
		}
	}

	// 查询需要清除的条目
	results, err := gc.ab.ds.Query(context.TODO(), purgeLookaheadQuery)
	if err != nil {
		log.Warnf("获取要清除的条目时失败: %v", err)
		return
	}
	defer results.Close()

	now := gc.ab.clock.Now().Unix()

	// 键: /peers/gc/addrs/<下次访问的unix时间戳>/<peer ID b32>
	// 值: nil
	for result := range results.Next() {
		gcKey := ds.RawKey(result.Key)
		// 解析时间戳
		ts, err := strconv.ParseInt(gcKey.Parent().Name(), 10, 64)
		if err != nil {
			dropInError(gcKey, err, "解析时间戳")
			log.Warnf("从键解析时间戳失败: %v, 错误: %v", result.Key, err)
			continue
		} else if ts > now {
			// 这是一个有序游标;当我们遇到时间戳超过now的条目时可以中断
			break
		}

		// 解析peer ID
		idb32, err := b32.RawStdEncoding.DecodeString(gcKey.Name())
		if err != nil {
			dropInError(gcKey, err, "解析peer ID")
			log.Warnf("从键解析b32 peer ID失败: %v, 错误: %v", result.Key, err)
			continue
		}

		id, err = peer.IDFromBytes(idb32)
		if err != nil {
			dropInError(gcKey, err, "解码peer ID")
			log.Warnf("从键解码peer ID失败: %v, 错误: %v", result.Key, err)
			continue
		}

		// 如果记录在缓存中,清理并在必要时刷新
		if cached, ok := gc.ab.cache.Peek(id); ok {
			cached.Lock()
			if cached.clean(gc.ab.clock.Now()) {
				if err = cached.flush(batch); err != nil {
					log.Warnf("刷新peer的GC修改条目失败: %s, 错误: %v", id, err)
				}
			}
			dropOrReschedule(gcKey, cached)
			cached.Unlock()
			continue
		}

		record.Reset()

		// 否则,从存储中获取记录,清理并刷新
		entryKey := addrBookBase.ChildString(gcKey.Name())
		val, err := gc.ab.ds.Get(context.TODO(), entryKey)
		if err != nil {
			// 捕获所有错误,包括ErrNotFound
			dropInError(gcKey, err, "获取条目")
			continue
		}
		err = proto.Unmarshal(val, record)
		if err != nil {
			dropInError(gcKey, err, "反序列化条目")
			continue
		}
		if record.clean(gc.ab.clock.Now()) {
			err = record.flush(batch)
			if err != nil {
				log.Warnf("刷新peer的GC修改条目失败: %s, 错误: %v", id, err)
			}
		}
		dropOrReschedule(gcKey, record)
	}

	if err = batch.Commit(context.TODO()); err != nil {
		log.Warnf("提交GC清除批处理失败: %v", err)
	}
}

// purgeStore 执行一次不带预查的GC清除循环
func (gc *dsAddrBookGc) purgeStore() {
	// 检查是否有其他清除正在运行
	select {
	case gc.running <- struct{}{}:
		defer func() { <-gc.running }()
	default:
		// 如果预查正在运行则退出
		return
	}

	// 创建空记录以重用并避免分配
	record := &addrsRecord{AddrBookRecord: &pb.AddrBookRecord{}}
	batch, err := newCyclicBatch(gc.ab.ds, defaultOpsPerCyclicBatch)
	if err != nil {
		log.Warnf("创建批处理以清除GC条目时失败: %v", err)
	}

	results, err := gc.ab.ds.Query(context.TODO(), purgeStoreQuery)
	if err != nil {
		log.Warnf("打开迭代器时失败: %v", err)
		return
	}
	defer results.Close()

	// 键: /peers/addrs/<peer ID b32>
	for result := range results.Next() {
		record.Reset()
		if err = proto.Unmarshal(result.Value, record); err != nil {
			// TODO 记录日志
			continue
		}

		id := record.Id
		if !record.clean(gc.ab.clock.Now()) {
			continue
		}

		if err := record.flush(batch); err != nil {
			log.Warnf("刷新peer的GC修改条目失败: &v, 错误: %v", id, err)
		}
		gc.ab.cache.Remove(peer.ID(id))
	}

	if err = batch.Commit(context.TODO()); err != nil {
		log.Warnf("提交GC清除批处理失败: %v", err)
	}
}

// populateLookahead 通过扫描整个存储并选择最早过期时间在窗口期内的条目来填充预查窗口
//
// 这些条目存储在存储的预查区域中,按需要访问的时间戳索引,以便于时间范围扫描
func (gc *dsAddrBookGc) populateLookahead() {
	if gc.ab.opts.GCLookaheadInterval == 0 {
		return
	}

	select {
	case gc.running <- struct{}{}:
		defer func() { <-gc.running }()
	default:
		// 如果有其他操作正在运行则退出
		return
	}

	until := gc.ab.clock.Now().Add(gc.ab.opts.GCLookaheadInterval).Unix()

	var id peer.ID
	record := &addrsRecord{AddrBookRecord: &pb.AddrBookRecord{}}
	results, err := gc.ab.ds.Query(context.TODO(), populateLookaheadQuery)
	if err != nil {
		log.Warnf("查询以填充预查GC窗口时失败: %v", err)
		return
	}
	defer results.Close()

	batch, err := newCyclicBatch(gc.ab.ds, defaultOpsPerCyclicBatch)
	if err != nil {
		log.Warnf("创建批处理以填充预查GC窗口时失败: %v", err)
		return
	}

	for result := range results.Next() {
		idb32 := ds.RawKey(result.Key).Name()
		k, err := b32.RawStdEncoding.DecodeString(idb32)
		if err != nil {
			log.Warnf("从键解码peer ID失败: %v, 错误: %v", result.Key, err)
			continue
		}
		if id, err = peer.IDFromBytes(k); err != nil {
			log.Warnf("从键解码peer ID失败: %v, 错误: %v", result.Key, err)
		}

		// 如果记录在缓存中,使用缓存版本
		if cached, ok := gc.ab.cache.Peek(id); ok {
			cached.RLock()
			if len(cached.Addrs) == 0 || cached.Addrs[0].Expiry > until {
				cached.RUnlock()
				continue
			}
			gcKey := gcLookaheadBase.ChildString(fmt.Sprintf("%d/%s", cached.Addrs[0].Expiry, idb32))
			if err = batch.Put(context.TODO(), gcKey, []byte{}); err != nil {
				log.Warnf("插入peer的GC条目时失败: %s, 错误: %v", id, err)
			}
			cached.RUnlock()
			continue
		}

		record.Reset()

		val, err := gc.ab.ds.Get(context.TODO(), ds.RawKey(result.Key))
		if err != nil {
			log.Warnf("从存储获取peer记录时失败: %s, 错误: %v", id, err)
			continue
		}
		if err := proto.Unmarshal(val, record); err != nil {
			log.Warnf("从存储反序列化peer记录时失败: %s, 错误: %v", id, err)
			continue
		}
		if len(record.Addrs) > 0 && record.Addrs[0].Expiry <= until {
			gcKey := gcLookaheadBase.ChildString(fmt.Sprintf("%d/%s", record.Addrs[0].Expiry, idb32))
			if err = batch.Put(context.TODO(), gcKey, []byte{}); err != nil {
				log.Warnf("插入peer的GC条目时失败: %s, 错误: %v", id, err)
			}
		}
	}

	if err = batch.Commit(context.TODO()); err != nil {
		log.Warnf("提交GC预查批处理失败: %v", err)
	}

	gc.currWindowEnd = until
}

// orderByTimestampInKey 通过比较键中的时间戳对结果进行排序
// 仅按字典序排序是错误的,因为"10"小于"2",但作为整数2显然小于10
// 参数:
//   - a: query.Entry 第一个条目
//   - b: query.Entry 第二个条目
//
// 返回:
//   - int 比较结果(-1: a<b, 0: a=b, 1: a>b)
func orderByTimestampInKey(a, b query.Entry) int {
	aKey := ds.RawKey(a.Key)
	aInt, err := strconv.ParseInt(aKey.Parent().Name(), 10, 64)
	if err != nil {
		return -1
	}
	bKey := ds.RawKey(b.Key)
	bInt, err := strconv.ParseInt(bKey.Parent().Name(), 10, 64)
	if err != nil {
		return -1
	}
	if aInt < bInt {
		return -1
	} else if aInt == bInt {
		return 0
	}
	return 1
}
