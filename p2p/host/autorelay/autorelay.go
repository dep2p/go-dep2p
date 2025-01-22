package autorelay

import (
	"context"
	"errors"
	"sync"

	"github.com/dep2p/go-dep2p/core/event"
	"github.com/dep2p/go-dep2p/core/host"
	"github.com/dep2p/go-dep2p/core/network"
	basic "github.com/dep2p/go-dep2p/p2p/host/basic"
	"github.com/dep2p/go-dep2p/p2p/host/eventbus"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	logging "github.com/dep2p/log"
)

// 定义日志对象
var log = logging.Logger("host-autorelay")

// AutoRelay 自动中继服务结构体
type AutoRelay struct {
	refCount  sync.WaitGroup     // 引用计数器,用于等待所有goroutine完成
	ctx       context.Context    // 上下文对象
	ctxCancel context.CancelFunc // 取消函数

	conf *config // 配置对象

	mx     sync.Mutex           // 互斥锁,用于保护status字段
	status network.Reachability // 网络可达性状态

	relayFinder *relayFinder // 中继节点查找器

	host host.Host // dep2p主机实例

	metricsTracer MetricsTracer // 指标追踪器
}

// NewAutoRelay 创建一个新的自动中继服务实例
// 参数:
//   - bhost: *basic.BasicHost 基础主机实例
//   - opts: ...Option 可选配置项
//
// 返回值:
//   - *AutoRelay: 返回创建的自动中继服务实例
//   - error: 如果发生错误则返回错误信息
func NewAutoRelay(bhost *basic.BasicHost, opts ...Option) (*AutoRelay, error) {
	r := &AutoRelay{ // 创建AutoRelay实例
		host:   bhost,                       // 设置主机
		status: network.ReachabilityUnknown, // 初始化可达性状态为未知
	}
	conf := defaultConfig      // 使用默认配置
	for _, opt := range opts { // 应用所有配置选项
		if err := opt(&conf); err != nil {
			log.Debugf("应用配置选项失败: %v", err)
			return nil, err
		}
	}
	r.ctx, r.ctxCancel = context.WithCancel(context.Background()) // 创建上下文
	r.conf = &conf                                                // 保存配置
	r.relayFinder = newRelayFinder(bhost, conf.peerSource, &conf) // 创建中继查找器
	r.metricsTracer = &wrappedMetricsTracer{conf.metricsTracer}   // 设置指标追踪器

	// 更新主机地址工厂,在私有网络时使用自动中继地址
	addrF := bhost.AddrsFactory // 保存原地址工厂
	bhost.AddrsFactory = func(addrs []ma.Multiaddr) []ma.Multiaddr {
		addrs = addrF(addrs) // 先使用原地址工厂处理
		r.mx.Lock()
		defer r.mx.Unlock()

		if r.status != network.ReachabilityPrivate { // 如果不是私有网络
			return addrs // 直接返回地址
		}
		return r.relayFinder.relayAddrs(addrs) // 返回中继地址
	}

	return r, nil
}

// Start 启动自动中继服务
func (r *AutoRelay) Start() {
	r.refCount.Add(1) // 增加引用计数
	go func() {
		defer r.refCount.Done() // 完成时减少引用计数
		r.background()          // 运行后台任务
	}()
}

// background 后台任务,监听网络可达性变化
func (r *AutoRelay) background() {
	// 订阅本地可达性变化事件
	subReachability, err := r.host.EventBus().Subscribe(new(event.EvtLocalReachabilityChanged), eventbus.Name("autorelay (background)"))
	if err != nil {
		log.Debug("订阅EvtLocalReachabilityChanged失败")
		return
	}
	defer subReachability.Close() // 退出时关闭订阅

	for {
		select {
		case <-r.ctx.Done(): // 上下文取消时退出
			return
		case ev, ok := <-subReachability.Out():
			if !ok { // 通道关闭时退出
				return
			}
			evt := ev.(event.EvtLocalReachabilityChanged)
			switch evt.Reachability {
			case network.ReachabilityPrivate, network.ReachabilityUnknown: // 私有网络或未知状态
				err := r.relayFinder.Start() // 启动中继查找器
				if errors.Is(err, errAlreadyRunning) {
					log.Debug("尝试启动已运行的中继查找器")
				} else if err != nil {
					log.Debugf("启动中继查找器失败: %v", err)
				} else {
					r.metricsTracer.RelayFinderStatus(true)
				}
			case network.ReachabilityPublic: // 公共网络
				r.relayFinder.Stop() // 停止中继查找器
				r.metricsTracer.RelayFinderStatus(false)
			}
			r.mx.Lock()
			r.status = evt.Reachability // 更新可达性状态
			r.mx.Unlock()
		}
	}
}

// Close 关闭自动中继服务
// 返回值:
//   - error: 如果发生错误则返回错误信息
func (r *AutoRelay) Close() error {
	r.ctxCancel()               // 取消上下文
	err := r.relayFinder.Stop() // 停止中继查找器
	r.refCount.Wait()           // 等待所有goroutine完成
	if err != nil {
		log.Debugf("停止中继查找器失败: %v", err)
	}
	return err
}
