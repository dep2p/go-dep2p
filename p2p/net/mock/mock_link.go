package mocknet

import (
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
)

// link 实现了 mocknet.Link 接口,为了简单起见也实现了 network.Conn 接口
type link struct {
	// mock 是所属的模拟网络实例
	mock *mocknet
	// nets 是链路连接的两个网络
	nets []*peernet
	// opts 是链路配置选项
	opts LinkOptions
	// ratelimiter 是速率限制器
	ratelimiter *RateLimiter
	// 这里可以在两端都有地址

	// 读写锁
	sync.RWMutex
}

// newLink 创建一个新的链路
// 参数:
//   - mn: 模拟网络实例
//   - opts: 链路配置选项
//
// 返回值:
//   - *link: 新创建的链路实例
func newLink(mn *mocknet, opts LinkOptions) *link {
	// 创建并返回链路实例,设置模拟网络、选项和速率限制器
	l := &link{mock: mn,
		opts:        opts,
		ratelimiter: NewRateLimiter(opts.Bandwidth)}
	return l
}

// newConnPair 为拨号方创建一对连接
// 参数:
//   - dialer: 发起连接的网络
//
// 返回值:
//   - *conn: 拨号方的连接
//   - *conn: 目标方的连接
func (l *link) newConnPair(dialer *peernet) (*conn, *conn) {
	l.RLock()
	defer l.RUnlock()

	// 获取目标网络
	target := l.nets[0]
	if target == dialer {
		target = l.nets[1]
	}
	// 创建拨号方和目标方的连接
	dc := newConn(dialer, target, l, network.DirOutbound)
	tc := newConn(target, dialer, l, network.DirInbound)
	// 设置连接的对端
	dc.rconn = tc
	tc.rconn = dc
	return dc, tc
}

// Networks 返回链路连接的网络列表
// 返回值:
//   - []network.Network: 网络列表副本
func (l *link) Networks() []network.Network {
	l.RLock()
	defer l.RUnlock()

	// 创建并返回网络列表的副本
	cp := make([]network.Network, len(l.nets))
	for i, n := range l.nets {
		cp[i] = n
	}
	return cp
}

// Peers 返回链路连接的对等点ID列表
// 返回值:
//   - []peer.ID: 对等点ID列表副本
func (l *link) Peers() []peer.ID {
	l.RLock()
	defer l.RUnlock()

	// 创建并返回对等点ID列表的副本
	cp := make([]peer.ID, len(l.nets))
	for i, n := range l.nets {
		cp[i] = n.peer
	}
	return cp
}

// SetOptions 设置链路选项
// 参数:
//   - o: 新的链路选项
func (l *link) SetOptions(o LinkOptions) {
	l.Lock()
	defer l.Unlock()
	// 更新选项和带宽限制
	l.opts = o
	l.ratelimiter.UpdateBandwidth(l.opts.Bandwidth)
}

// Options 获取链路选项
// 返回值:
//   - LinkOptions: 当前的链路选项
func (l *link) Options() LinkOptions {
	l.RLock()
	defer l.RUnlock()
	return l.opts
}

// GetLatency 获取链路延迟
// 返回值:
//   - time.Duration: 链路延迟时间
func (l *link) GetLatency() time.Duration {
	l.RLock()
	defer l.RUnlock()
	return l.opts.Latency
}

// RateLimit 根据数据大小计算速率限制延迟
// 参数:
//   - dataSize: 数据大小
//
// 返回值:
//   - time.Duration: 需要等待的延迟时间
func (l *link) RateLimit(dataSize int) time.Duration {
	return l.ratelimiter.Limit(dataSize)
}
