package backoff

import (
	"context"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/peer"

	lru "github.com/hashicorp/golang-lru/v2"
)

// BackoffConnector 是一个工具,用于连接对等节点,但仅在最近没有尝试连接过的情况下才会连接
type BackoffConnector struct {
	cache      *lru.TwoQueueCache[peer.ID, *connCacheData] // 缓存
	host       host.Host                                   // 主机
	connTryDur time.Duration                               // 连接尝试持续时间
	backoff    BackoffFactory                              // 退避策略工厂
	mux        sync.Mutex                                  // 互斥锁
}

// NewBackoffConnector 创建一个工具用于连接对等节点,但仅在最近没有尝试连接过的情况下才会连接
// 参数:
//   - h: host.Host 主机实例
//   - cacheSize: int 缓存大小
//   - connectionTryDuration: time.Duration 连接尝试超时时间
//   - backoff: BackoffFactory 退避策略工厂函数
//
// 返回值:
//   - *BackoffConnector: 返回创建的BackoffConnector实例
//   - error: 如果发生错误则返回错误信息
func NewBackoffConnector(h host.Host, cacheSize int, connectionTryDuration time.Duration, backoff BackoffFactory) (*BackoffConnector, error) {
	// 创建一个新的TwoQueueCache缓存
	cache, err := lru.New2Q[peer.ID, *connCacheData](cacheSize)
	if err != nil {
		return nil, err
	}

	// 返回新创建的BackoffConnector实例
	return &BackoffConnector{
		cache:      cache,
		host:       h,
		connTryDur: connectionTryDuration,
		backoff:    backoff,
	}, nil
}

// connCacheData 保存连接缓存相关的数据
type connCacheData struct {
	nextTry time.Time       // 下次尝试连接的时间
	strat   BackoffStrategy // 退避策略
}

// Connect 尝试连接从peerCh传入的对等节点
// 如果对等节点处于退避期内则不会连接。
// 由于 Connect 会在获知对等节点后立即尝试拨号,调用者应该尽量控制入站对等节点的数量和速率。
// 参数:
//   - ctx: context.Context 上下文
//   - peerCh: <-chan peer.AddrInfo 对等节点信息通道
func (c *BackoffConnector) Connect(ctx context.Context, peerCh <-chan peer.AddrInfo) {
	for {
		select {
		case pi, ok := <-peerCh:
			// 通道关闭则退出
			if !ok {
				return
			}

			// 跳过无效的节点ID和自身ID
			if pi.ID == c.host.ID() || pi.ID == "" {
				continue
			}

			c.mux.Lock()
			var cachedPeer *connCacheData
			// 检查节点是否在缓存中
			if tv, ok := c.cache.Get(pi.ID); ok {
				now := time.Now()
				// 如果还在退避期内则跳过
				if now.Before(tv.nextTry) {
					c.mux.Unlock()
					continue
				}

				// 更新下次尝试时间
				tv.nextTry = now.Add(tv.strat.Delay())
			} else {
				// 创建新的缓存数据
				cachedPeer = &connCacheData{strat: c.backoff()}
				cachedPeer.nextTry = time.Now().Add(cachedPeer.strat.Delay())
				c.cache.Add(pi.ID, cachedPeer)
			}
			c.mux.Unlock()

			// 异步尝试连接节点
			go func(pi peer.AddrInfo) {
				ctx, cancel := context.WithTimeout(ctx, c.connTryDur)
				defer cancel()

				// 尝试连接节点
				err := c.host.Connect(ctx, pi)
				if err != nil {
					log.Debugf("连接到pubsub对等节点 %s 时出错: %s", pi.ID, err.Error())
					return
				}
			}(pi)

		case <-ctx.Done():
			// log.Infof("发现: 退避连接器上下文错误 %v", ctx.Err())
			return
		}
	}
}
