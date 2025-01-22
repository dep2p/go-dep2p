package backoff

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/discovery"
	"github.com/dep2p/go-dep2p/core/peer"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// BackoffDiscovery 实现了一个带缓存和退避机制的发现服务
type BackoffDiscovery struct {
	disc         discovery.Discovery      // 底层的发现服务
	stratFactory BackoffFactory           // 退避策略工厂函数
	peerCache    map[string]*backoffCache // 节点缓存,key为命名空间
	peerCacheMux sync.RWMutex             // 保护peerCache的读写锁

	parallelBufSz int // 并行查询时的缓冲区大小
	returnedBufSz int // 返回结果时的缓冲区大小

	clock clock // 时钟接口,用于获取当前时间
}

// BackoffDiscoveryOption 定义了BackoffDiscovery的配置选项函数类型
type BackoffDiscoveryOption func(*BackoffDiscovery) error

// NewBackoffDiscovery 创建一个新的BackoffDiscovery实例
// 参数:
//   - disc: 底层的发现服务
//   - stratFactory: 退避策略工厂函数
//   - opts: 可选的配置选项
//
// 返回值:
//   - discovery.Discovery: 返回创建的发现服务实例
//   - error: 如果发生错误则返回错误信息
func NewBackoffDiscovery(disc discovery.Discovery, stratFactory BackoffFactory, opts ...BackoffDiscoveryOption) (discovery.Discovery, error) {
	// 创建基础实例
	b := &BackoffDiscovery{
		disc:         disc,
		stratFactory: stratFactory,
		peerCache:    make(map[string]*backoffCache),

		parallelBufSz: 32, // 默认并行缓冲区大小
		returnedBufSz: 32, // 默认返回缓冲区大小

		clock: realClock{}, // 使用真实时钟
	}

	// 应用配置选项
	for _, opt := range opts {
		if err := opt(b); err != nil {
			log.Debugf("应用配置选项失败: %v", err)
			return nil, err
		}
	}

	return b, nil
}

// WithBackoffDiscoverySimultaneousQueryBufferSize 设置同时查询时的缓冲区大小
// 参数:
//   - size: 缓冲区大小
//
// 返回值:
//   - BackoffDiscoveryOption: 返回配置选项函数
func WithBackoffDiscoverySimultaneousQueryBufferSize(size int) BackoffDiscoveryOption {
	return func(b *BackoffDiscovery) error {
		if size < 0 {
			log.Debugf("不能设置小于0的缓冲区大小")
			return fmt.Errorf("不能设置小于0的缓冲区大小")
		}
		b.parallelBufSz = size
		return nil
	}
}

// WithBackoffDiscoveryReturnedChannelSize 设置返回结果通道的缓冲区大小
// 注意:此设置在退避时间内的查询中不生效
// 参数:
//   - size: 缓冲区大小
//
// 返回值:
//   - BackoffDiscoveryOption: 返回配置选项函数
func WithBackoffDiscoveryReturnedChannelSize(size int) BackoffDiscoveryOption {
	return func(b *BackoffDiscovery) error {
		if size < 0 {
			log.Debugf("不能设置小于0的缓冲区大小")
			return fmt.Errorf("不能设置小于0的缓冲区大小")
		}
		b.returnedBufSz = size
		return nil
	}
}

// clock 定义了获取当前时间的接口
type clock interface {
	Now() time.Time
}

// realClock 实现了真实时钟
type realClock struct{}

// Now 返回当前时间
func (c realClock) Now() time.Time {
	return time.Now()
}

// backoffCache 缓存了每个命名空间的节点信息和退避状态
type backoffCache struct {
	strat BackoffStrategy // 退避策略,创建时分配且不再修改

	mux          sync.Mutex                 // 保护以下字段的互斥锁
	nextDiscover time.Time                  // 下次允许发现的时间
	prevPeers    map[peer.ID]peer.AddrInfo  // 上一次发现的节点
	peers        map[peer.ID]peer.AddrInfo  // 当前发现的节点
	sendingChs   map[chan peer.AddrInfo]int // 发送通道及其剩余配额
	ongoing      bool                       // 是否有正在进行的发现请求

	clock clock // 时钟接口
}

// Advertise 实现了discovery.Discovery接口的Advertise方法
// 参数:
//   - ctx: 上下文
//   - ns: 命名空间
//   - opts: 发现选项
//
// 返回值:
//   - time.Duration: 广播持续时间
//   - error: 如果发生错误则返回错误信息
func (d *BackoffDiscovery) Advertise(ctx context.Context, ns string, opts ...discovery.Option) (time.Duration, error) {
	return d.disc.Advertise(ctx, ns, opts...)
}

// FindPeers 实现了discovery.Discovery接口的FindPeers方法
// 参数:
//   - ctx: 上下文
//   - ns: 命名空间
//   - opts: 发现选项
//
// 返回值:
//   - <-chan peer.AddrInfo: 返回发现的节点信息通道
//   - error: 如果发生错误则返回错误信息
func (d *BackoffDiscovery) FindPeers(ctx context.Context, ns string, opts ...discovery.Option) (<-chan peer.AddrInfo, error) {
	// 解析选项
	var options discovery.Options
	err := options.Apply(opts...)
	if err != nil {
		log.Debugf("解析选项失败: %v", err)
		return nil, err
	}

	// 获取缓存的节点信息
	d.peerCacheMux.RLock()
	c, ok := d.peerCache[ns]
	d.peerCacheMux.RUnlock()

	/*
		整体策略:
		1. 如果到了查找节点的时间,则查找节点并返回
		2. 如果还未到时间则返回缓存
		3. 如果到了查找时间但已经有查找在进行,则加入到当前查找中
	*/

	// 如果缓存不存在则创建
	if !ok {
		pc := &backoffCache{
			nextDiscover: time.Time{},
			prevPeers:    make(map[peer.ID]peer.AddrInfo),
			peers:        make(map[peer.ID]peer.AddrInfo),
			sendingChs:   make(map[chan peer.AddrInfo]int),
			strat:        d.stratFactory(),
			clock:        d.clock,
		}

		d.peerCacheMux.Lock()
		c, ok = d.peerCache[ns]

		if !ok {
			d.peerCache[ns] = pc
			c = pc
		}

		d.peerCacheMux.Unlock()
	}

	c.mux.Lock()
	defer c.mux.Unlock()

	timeExpired := d.clock.Now().After(c.nextDiscover)

	// 如果未到查找时间且没有正在进行的查找,返回缓存的节点
	if !(timeExpired || c.ongoing) {
		chLen := options.Limit

		if chLen == 0 {
			chLen = len(c.prevPeers)
		} else if chLen > len(c.prevPeers) {
			chLen = len(c.prevPeers)
		}
		pch := make(chan peer.AddrInfo, chLen)
		for _, ai := range c.prevPeers {
			select {
			case pch <- ai:
			default:
				// 如果请求的限制小于已知节点数,跳过多余的节点
			}
		}
		close(pch)
		return pch, nil
	}

	// 如果没有正在进行的查找,启动一个新的查找
	if !c.ongoing {
		pch, err := d.disc.FindPeers(ctx, ns, opts...)
		if err != nil {
			log.Debugf("查找节点失败: %v", err)
			return nil, err
		}

		c.ongoing = true
		go findPeerDispatcher(ctx, c, pch)
	}

	// 设置接收通道
	evtCh := make(chan peer.AddrInfo, d.parallelBufSz)
	pch := make(chan peer.AddrInfo, d.returnedBufSz)
	rcvPeers := make([]peer.AddrInfo, 0, 32)
	for _, ai := range c.peers {
		rcvPeers = append(rcvPeers, ai)
	}
	c.sendingChs[evtCh] = options.Limit

	go findPeerReceiver(ctx, pch, evtCh, rcvPeers)

	return pch, nil
}

// findPeerDispatcher 处理发现的节点并分发给所有接收者
// 参数:
//   - ctx: 上下文
//   - c: 节点缓存
//   - pch: 接收发现节点的通道
func findPeerDispatcher(ctx context.Context, c *backoffCache, pch <-chan peer.AddrInfo) {
	defer func() {
		c.mux.Lock()

		// 如果节点地址有变化则重置退避
		if checkUpdates(c.prevPeers, c.peers) {
			c.strat.Reset()
			c.prevPeers = c.peers
		}
		c.nextDiscover = c.clock.Now().Add(c.strat.Delay())

		c.ongoing = false
		c.peers = make(map[peer.ID]peer.AddrInfo)

		for ch := range c.sendingChs {
			close(ch)
		}
		c.sendingChs = make(map[chan peer.AddrInfo]int)
		c.mux.Unlock()
	}()

	for {
		select {
		case ai, ok := <-pch:
			if !ok {
				return
			}
			c.mux.Lock()

			// 如果收到相同节点多次,合并其地址
			var sendAi peer.AddrInfo
			if prevAi, ok := c.peers[ai.ID]; ok {
				if combinedAi := mergeAddrInfos(prevAi, ai); combinedAi != nil {
					sendAi = *combinedAi
				} else {
					c.mux.Unlock()
					continue
				}
			} else {
				sendAi = ai
			}

			c.peers[ai.ID] = sendAi

			for ch, rem := range c.sendingChs {
				if rem > 0 {
					ch <- sendAi
					c.sendingChs[ch] = rem - 1
				}
			}

			c.mux.Unlock()
		case <-ctx.Done():
			return
		}
	}
}

// findPeerReceiver 接收并转发发现的节点
// 参数:
//   - ctx: 上下文
//   - pch: 发送节点的通道
//   - evtCh: 接收节点的通道
//   - rcvPeers: 已接收的节点列表
func findPeerReceiver(ctx context.Context, pch, evtCh chan peer.AddrInfo, rcvPeers []peer.AddrInfo) {
	defer close(pch)

	for {
		select {
		case ai, ok := <-evtCh:
			if ok {
				rcvPeers = append(rcvPeers, ai)

				sentAll := true
			sendPeers:
				for i, p := range rcvPeers {
					select {
					case pch <- p:
					default:
						rcvPeers = rcvPeers[i:]
						sentAll = false
						break sendPeers
					}
				}
				if sentAll {
					rcvPeers = []peer.AddrInfo{}
				}
			} else {
				for _, p := range rcvPeers {
					select {
					case pch <- p:
					case <-ctx.Done():
						return
					}
				}
				return
			}
		case <-ctx.Done():
			return
		}
	}
}

// mergeAddrInfos 合并两个节点的地址信息
// 参数:
//   - prevAi: 原有节点信息
//   - newAi: 新的节点信息
//
// 返回值:
//   - *peer.AddrInfo: 如果地址有更新返回合并后的信息,否则返回nil
func mergeAddrInfos(prevAi, newAi peer.AddrInfo) *peer.AddrInfo {
	seen := make(map[string]struct{}, len(prevAi.Addrs))
	combinedAddrs := make([]ma.Multiaddr, 0, len(prevAi.Addrs))
	addAddrs := func(addrs []ma.Multiaddr) {
		for _, addr := range addrs {
			if _, ok := seen[addr.String()]; ok {
				continue
			}
			seen[addr.String()] = struct{}{}
			combinedAddrs = append(combinedAddrs, addr)
		}
	}
	addAddrs(prevAi.Addrs)
	addAddrs(newAi.Addrs)

	if len(combinedAddrs) > len(prevAi.Addrs) {
		combinedAi := &peer.AddrInfo{ID: prevAi.ID, Addrs: combinedAddrs}
		return combinedAi
	}
	return nil
}

// checkUpdates 检查节点信息是否有更新
// 参数:
//   - orig: 原有节点信息映射
//   - update: 更新的节点信息映射
//
// 返回值:
//   - bool: 如果有更新返回true,否则返回false
func checkUpdates(orig, update map[peer.ID]peer.AddrInfo) bool {
	if len(orig) != len(update) {
		return true
	}
	for p, ai := range update {
		if prevAi, ok := orig[p]; ok {
			if combinedAi := mergeAddrInfos(prevAi, ai); combinedAi != nil {
				return true
			}
		} else {
			return true
		}
	}
	return false
}
