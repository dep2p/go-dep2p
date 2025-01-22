package connmgr

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/core/connmgr"
	"github.com/dep2p/go-dep2p/core/peer"

	"github.com/benbjohnson/clock"
)

// DefaultResolution 是衰减追踪器的默认分辨率
var DefaultResolution = 1 * time.Minute

// bumpCmd 表示一个增加命令
type bumpCmd struct {
	peer  peer.ID      // peer ID
	tag   *decayingTag // 衰减标签
	delta int          // 增加的值
}

// removeCmd 表示一个标签移除命令
type removeCmd struct {
	peer peer.ID      // peer ID
	tag  *decayingTag // 要移除的标签
}

// decayer 追踪和管理所有衰减标签及其值
type decayer struct {
	cfg   *DecayerCfg   // 配置信息
	mgr   *BasicConnMgr // 连接管理器
	clock clock.Clock   // 用于测试的时钟

	tagsMu    sync.Mutex              // 保护 knownTags 的互斥锁
	knownTags map[string]*decayingTag // 已知标签映射

	// lastTick 存储最后一次衰减的时间,由原子操作保护
	lastTick atomic.Pointer[time.Time]

	// bumpTagCh 用于队列化待处理的增加命令
	bumpTagCh   chan bumpCmd
	removeTagCh chan removeCmd    // 用于队列化待处理的移除命令
	closeTagCh  chan *decayingTag // 用于队列化待处理的关闭命令

	// 关闭相关的通道
	closeCh chan struct{} // 关闭信号通道
	doneCh  chan struct{} // 完成信号通道
	err     error         // 错误信息
}

// 确保 decayer 实现了 Decayer 接口
var _ connmgr.Decayer = (*decayer)(nil)

// DecayerCfg 是衰减器的配置对象
type DecayerCfg struct {
	Resolution time.Duration // 分辨率
	Clock      clock.Clock   // 时钟实例
}

// WithDefaults 为 DecayerConfig 实例写入默认值,并返回自身以支持链式调用
//
//	cfg := (&DecayerCfg{}).WithDefaults()
//	cfg.Resolution = 30 * time.Second
//	t := NewDecayer(cfg, cm)
func (cfg *DecayerCfg) WithDefaults() *DecayerCfg {
	cfg.Resolution = DefaultResolution
	return cfg
}

// NewDecayer 创建一个新的衰减标签注册表
// 参数:
//   - cfg: *DecayerCfg,衰减器配置
//   - mgr: *BasicConnMgr,连接管理器
//
// 返回:
//   - *decayer: 新创建的衰减器
//   - error: 如果创建过程中出现错误则返回错误
func NewDecayer(cfg *DecayerCfg, mgr *BasicConnMgr) (*decayer, error) {
	// 如果配置中的时钟为空,使用真实时钟
	if cfg.Clock == nil {
		cfg.Clock = clock.New()
	}

	// 创建衰减器实例
	d := &decayer{
		cfg:         cfg,
		mgr:         mgr,
		clock:       cfg.Clock,
		knownTags:   make(map[string]*decayingTag),
		bumpTagCh:   make(chan bumpCmd, 128),
		removeTagCh: make(chan removeCmd, 128),
		closeTagCh:  make(chan *decayingTag, 128),
		closeCh:     make(chan struct{}),
		doneCh:      make(chan struct{}),
	}

	// 获取当前时间并存储为最后一次衰减时间
	now := d.clock.Now()
	d.lastTick.Store(&now)

	// 启动处理循环
	go d.process()

	return d, nil
}

// RegisterDecayingTag 注册一个新的衰减标签
// 参数:
//   - name: string,标签名称
//   - interval: time.Duration,衰减间隔
//   - decayFn: connmgr.DecayFn,衰减函数
//   - bumpFn: connmgr.BumpFn,增加函数
//
// 返回:
//   - connmgr.DecayingTag: 新创建的衰减标签
//   - error: 如果注册过程中出现错误则返回错误
func (d *decayer) RegisterDecayingTag(name string, interval time.Duration, decayFn connmgr.DecayFn, bumpFn connmgr.BumpFn) (connmgr.DecayingTag, error) {
	d.tagsMu.Lock()
	defer d.tagsMu.Unlock()

	// 检查标签名是否已存在
	if _, ok := d.knownTags[name]; ok {
		log.Debugf("已存在名为 %s 的衰减标签", name)
		return nil, fmt.Errorf("已存在名为 %s 的衰减标签", name)
	}

	// 确保间隔不小于分辨率
	if interval < d.cfg.Resolution {
		log.Warnf("标签 %s 的衰减间隔(%s)小于追踪器的分辨率(%s);已覆盖为分辨率值",
			name, interval, d.cfg.Resolution)
		interval = d.cfg.Resolution
	}

	// 检查间隔是否为分辨率的整数倍
	if interval%d.cfg.Resolution != 0 {
		log.Warnf("标签 %s 的衰减间隔(%s)不是追踪器分辨率(%s)的整数倍;"+
			"可能会损失一些精度", name, interval, d.cfg.Resolution)
	}

	// 获取最后一次衰减时间
	lastTick := d.lastTick.Load()

	// 创建新的衰减标签
	tag := &decayingTag{
		trkr:     d,
		name:     name,
		interval: interval,
		nextTick: lastTick.Add(interval),
		decayFn:  decayFn,
		bumpFn:   bumpFn,
	}

	// 存储标签
	d.knownTags[name] = tag
	return tag, nil
}

// Close 关闭衰减器,此方法是幂等的
// 返回:
//   - error: 关闭过程中的错误
func (d *decayer) Close() error {
	select {
	case <-d.doneCh:
		return d.err
	default:
	}

	close(d.closeCh)
	<-d.doneCh
	return d.err
}

// process 是追踪器的核心,执行以下职责:
//  1. 管理衰减
//  2. 应用分数增加
//  3. 在关闭时退出
func (d *decayer) process() {
	defer close(d.doneCh)

	ticker := d.clock.Ticker(d.cfg.Resolution)
	defer ticker.Stop()

	var (
		bmp   bumpCmd
		visit = make(map[*decayingTag]struct{})
	)

	for {
		select {
		case <-ticker.C:
			now := d.clock.Now()
			d.lastTick.Store(&now)

			// 获取需要更新的标签
			d.tagsMu.Lock()
			for _, tag := range d.knownTags {
				if tag.nextTick.After(now) {
					// 跳过此标签
					continue
				}
				// 标记此标签在本轮需要更新
				visit[tag] = struct{}{}
			}
			d.tagsMu.Unlock()

			// 访问每个peer,衰减需要衰减的标签
			for _, s := range d.mgr.segments.buckets {
				s.Lock()

				// 处理包含peer的分段
				for _, p := range s.peers {
					for tag, v := range p.decaying {
						if _, ok := visit[tag]; !ok {
							// 跳过此标签
							continue
						}

						// 处理需要访问的值
						var delta int
						if after, rm := tag.decayFn(*v); rm {
							// 删除值并继续下一个标签
							delta -= v.Value
							delete(p.decaying, tag)
						} else {
							// 累加delta并应用更改
							delta += after - v.Value
							v.Value, v.LastVisit = after, now
						}
						p.value += delta
					}
				}

				s.Unlock()
			}

			// 重置每个标签的下次访问轮次,并清空已访问集合
			for tag := range visit {
				tag.nextTick = tag.nextTick.Add(tag.interval)
				delete(visit, tag)
			}

		case bmp = <-d.bumpTagCh:
			var (
				now       = d.clock.Now()
				peer, tag = bmp.peer, bmp.tag
			)

			s := d.mgr.segments.get(peer)
			s.Lock()

			p := s.tagInfoFor(peer, d.clock.Now())
			v, ok := p.decaying[tag]
			if !ok {
				v = &connmgr.DecayingValue{
					Tag:       tag,
					Peer:      peer,
					LastVisit: now,
					Added:     now,
					Value:     0,
				}
				p.decaying[tag] = v
			}

			prev := v.Value
			v.Value, v.LastVisit = v.Tag.(*decayingTag).bumpFn(*v, bmp.delta), now
			p.value += v.Value - prev

			s.Unlock()

		case rm := <-d.removeTagCh:
			s := d.mgr.segments.get(rm.peer)
			s.Lock()

			p := s.tagInfoFor(rm.peer, d.clock.Now())
			v, ok := p.decaying[rm.tag]
			if !ok {
				s.Unlock()
				continue
			}
			p.value -= v.Value
			delete(p.decaying, rm.tag)
			s.Unlock()

		case t := <-d.closeTagCh:
			// 停止追踪标签
			d.tagsMu.Lock()
			delete(d.knownTags, t.name)
			d.tagsMu.Unlock()

			// 从所有拥有该标签的peer中移除标签
			for _, s := range d.mgr.segments.buckets {
				s.Lock()
				for _, p := range s.peers {
					if dt, ok := p.decaying[t]; ok {
						// 减少tagInfo的值,并删除标签
						p.value -= dt.Value
						delete(p.decaying, t)
					}
				}
				s.Unlock()
			}

		case <-d.closeCh:
			return
		}
	}
}

// decayingTag 表示一个衰减标签,包含衰减间隔、衰减函数和增加函数
type decayingTag struct {
	trkr     *decayer        // 衰减器引用
	name     string          // 标签名称
	interval time.Duration   // 衰减间隔
	nextTick time.Time       // 下次衰减时间
	decayFn  connmgr.DecayFn // 衰减函数
	bumpFn   connmgr.BumpFn  // 增加函数

	// closed 标记此标签是否已关闭,用于在关闭后增加时返回错误
	closed atomic.Bool
}

// 确保 decayingTag 实现了 DecayingTag 接口
var _ connmgr.DecayingTag = (*decayingTag)(nil)

// Name 返回标签名称
func (t *decayingTag) Name() string {
	return t.name
}

// Interval 返回衰减间隔
func (t *decayingTag) Interval() time.Duration {
	return t.interval
}

// Bump 为指定peer增加标签值
// 参数:
//   - p: peer.ID,peer标识符
//   - delta: int,增加的值
//
// 返回:
//   - error: 如果增加过程中出现错误则返回错误
func (t *decayingTag) Bump(p peer.ID, delta int) error {
	if t.closed.Load() {
		log.Debugf("衰减标签 %s 已关闭;不接受更多增加操作", t.name)
		return fmt.Errorf("衰减标签 %s 已关闭;不接受更多增加操作", t.name)
	}

	bmp := bumpCmd{peer: p, tag: t, delta: delta}

	select {
	case t.trkr.bumpTagCh <- bmp:
		return nil
	default:
		log.Debugf(
			"无法为peer %s 增加衰减标签 %s,delta %d;队列已满(长度=%d)",
			p, t.name, delta, len(t.trkr.bumpTagCh))
		return fmt.Errorf(
			"无法为peer %s 增加衰减标签 %s,delta %d;队列已满(长度=%d)",
			p, t.name, delta, len(t.trkr.bumpTagCh))
	}
}

// Remove 移除指定peer的标签
// 参数:
//   - p: peer.ID,peer标识符
//
// 返回:
//   - error: 如果移除过程中出现错误则返回错误
func (t *decayingTag) Remove(p peer.ID) error {
	if t.closed.Load() {
		log.Debugf("衰减标签 %s 已关闭;不接受更多移除操作", t.name)
		return fmt.Errorf("衰减标签 %s 已关闭;不接受更多移除操作", t.name)
	}

	rm := removeCmd{peer: p, tag: t}

	select {
	case t.trkr.removeTagCh <- rm:
		return nil
	default:
		log.Debugf(
			"无法为peer %s 移除衰减标签 %s;队列已满(长度=%d)",
			p, t.name, len(t.trkr.removeTagCh))
		return fmt.Errorf(
			"无法为peer %s 移除衰减标签 %s;队列已满(长度=%d)",
			p, t.name, len(t.trkr.removeTagCh))
	}
}

// Close 关闭衰减标签
// 返回:
//   - error: 如果关闭过程中出现错误则返回错误
func (t *decayingTag) Close() error {
	if !t.closed.CompareAndSwap(false, true) {
		log.Warnf("重复关闭衰减标签: %s;跳过", t.name)
		return nil
	}

	select {
	case t.trkr.closeTagCh <- t:
		return nil
	default:
		log.Debugf(
			"无法关闭衰减标签 %s;队列已满(长度=%d)",
			t.name, len(t.trkr.closeTagCh))
		return fmt.Errorf(
			"无法关闭衰减标签 %s;队列已满(长度=%d)",
			t.name, len(t.trkr.closeTagCh))
	}
}
