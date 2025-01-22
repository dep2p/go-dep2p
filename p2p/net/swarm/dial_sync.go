package swarm

import (
	"context"
	"errors"
	"sync"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
)

// dialWorkerFunc 用于 dialSync 生成新的拨号工作器
type dialWorkerFunc func(peer.ID, <-chan dialRequest)

// errConcurrentDialSuccessful 用于表示并发拨号成功
var errConcurrentDialSuccessful = errors.New("并发拨号成功")

// newDialSync 构造一个新的拨号同步器
// 参数:
//   - worker: dialWorkerFunc 拨号工作器函数
//
// 返回值:
//   - *dialSync 拨号同步器实例
func newDialSync(worker dialWorkerFunc) *dialSync {
	return &dialSync{
		dials:      make(map[peer.ID]*activeDial),
		dialWorker: worker,
	}
}

// dialSync 是一个拨号同步辅助器,确保在任何时候对任何给定对等节点最多只有一个活跃的拨号
type dialSync struct {
	// mutex 用于保护 dials 映射的互斥锁
	mutex sync.Mutex
	// dials 保存每个对等节点的活跃拨号状态
	dials map[peer.ID]*activeDial
	// dialWorker 拨号工作器函数
	dialWorker dialWorkerFunc
}

// activeDial 表示一个活跃的拨号状态
type activeDial struct {
	// refCnt 引用计数
	refCnt int
	// ctx 拨号上下文
	ctx context.Context
	// cancelCause 取消拨号的函数,可以设置取消原因
	cancelCause func(error)
	// reqch 拨号请求通道
	reqch chan dialRequest
}

// dial 执行拨号操作
// 参数:
//   - ctx: context.Context 拨号上下文
//
// 返回值:
//   - *Conn 建立的连接
//   - error 拨号过程中的错误
func (ad *activeDial) dial(ctx context.Context) (*Conn, error) {
	// 使用活跃拨号的上下文
	dialCtx := ad.ctx

	// 检查是否需要强制直连
	if forceDirect, reason := network.GetForceDirectDial(ctx); forceDirect {
		dialCtx = network.WithForceDirectDial(dialCtx, reason)
	}
	// 检查是否需要同时连接
	if simConnect, isClient, reason := network.GetSimultaneousConnect(ctx); simConnect {
		dialCtx = network.WithSimultaneousConnect(dialCtx, isClient, reason)
	}

	// 创建响应通道
	resch := make(chan dialResponse, 1)
	// 发送拨号请求
	select {
	case ad.reqch <- dialRequest{ctx: dialCtx, resch: resch}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// 等待拨号响应
	select {
	case res := <-resch:
		log.Debugf("拨号响应: %v", res)
		return res.conn, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// getActiveDial 获取或创建一个活跃的拨号状态
// 参数:
//   - p: peer.ID 对等节点ID
//
// 返回值:
//   - *activeDial 活跃拨号状态
//   - error 获取过程中的错误
func (ds *dialSync) getActiveDial(p peer.ID) (*activeDial, error) {
	ds.mutex.Lock()
	defer ds.mutex.Unlock()

	// 查找现有的活跃拨号
	actd, ok := ds.dials[p]
	if !ok {
		// 故意使用后台上下文,否则如果第一次拨号被取消,后续的拨号也会被取消
		ctx, cancel := context.WithCancelCause(context.Background())
		actd = &activeDial{
			ctx:         ctx,
			cancelCause: cancel,
			reqch:       make(chan dialRequest),
		}
		// 启动拨号工作器
		go ds.dialWorker(p, actd.reqch)
		ds.dials[p] = actd
	}
	// 在释放互斥锁前增加引用计数
	actd.refCnt++
	return actd, nil
}

// Dial 如果没有正在进行的拨号,则发起对给定对等节点的拨号,然后等待拨号完成
// 参数:
//   - ctx: context.Context 拨号上下文
//   - p: peer.ID 对等节点ID
//
// 返回值:
//   - *Conn 建立的连接
//   - error 拨号过程中的错误
func (ds *dialSync) Dial(ctx context.Context, p peer.ID) (*Conn, error) {
	// 获取活跃拨号状态
	ad, err := ds.getActiveDial(p)
	if err != nil {
		log.Debugf("获取活跃拨号状态失败: %v", err)
		return nil, err
	}

	// 执行拨号
	conn, err := ad.dial(ctx)

	ds.mutex.Lock()
	defer ds.mutex.Unlock()

	// 减少引用计数
	ad.refCnt--
	// 如果引用计数为0,清理拨号状态
	if ad.refCnt == 0 {
		if err == nil {
			ad.cancelCause(errConcurrentDialSuccessful)
		} else {
			ad.cancelCause(err)
		}
		close(ad.reqch)
		delete(ds.dials, p)
	}

	return conn, err
}
