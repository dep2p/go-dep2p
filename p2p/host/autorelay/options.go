package autorelay

import (
	"context"
	"errors"
	"time"

	"github.com/dep2p/go-dep2p/core/peer"
)

// PeerSource 定义了一个函数类型,用于获取中继候选节点
// 参数:
//   - ctx: context.Context 上下文对象,用于取消操作
//   - num: int 请求的节点数量
//
// 返回值:
//   - <-chan peer.AddrInfo: 返回一个只读通道,用于接收节点地址信息
//
// AutoRelay 在需要新的候选节点时会调用此函数,原因可能是:
// 1. 未连接到期望数量的中继节点
// 2. 与某个中继节点断开连接
// 实现必须最多发送 numPeers 个节点,并在不打算提供更多节点时关闭通道。
// 在通道关闭之前,AutoRelay 不会再次调用回调。
// 实现应该发送新节点,但也可以发送之前发送过的节点。
// AutoRelay 实现了每个节点的退避机制(参见 WithBackoff)。
// 使用 WithMinInterval 设置回调调用的最小间隔。
// 当 AutoRelay 认为满意时,传入的 context.Context 可能会被取消,
// 当节点关闭时它也会被取消。如果上下文被取消,您必须在某个时候关闭输出通道。
type PeerSource func(ctx context.Context, num int) <-chan peer.AddrInfo

// config 定义了 AutoRelay 的配置选项
type config struct {
	clock            ClockWithInstantTimer // 时钟实例,用于计时和定时器操作
	peerSource       PeerSource            // 节点源函数,用于获取候选节点
	minInterval      time.Duration         // 调用 peerSource 回调的最小间隔
	minCandidates    int                   // 最小候选节点数量
	maxCandidates    int                   // 最大候选节点数量
	bootDelay        time.Duration         // 启动延迟时间
	backoff          time.Duration         // 失败后的退避时间
	desiredRelays    int                   // 期望的中继节点数量
	maxCandidateAge  time.Duration         // 候选节点的最大存活时间
	setMinCandidates bool                  // 是否已设置最小候选节点数量
	metricsTracer    MetricsTracer         // 指标追踪器
}

// defaultConfig 定义了默认配置
var defaultConfig = config{
	clock:           RealClock{},      // 使用真实时钟
	minCandidates:   4,                // 默认最小候选节点数为4
	maxCandidates:   20,               // 默认最大候选节点数为20
	bootDelay:       3 * time.Minute,  // 默认启动延迟3分钟
	backoff:         time.Hour,        // 默认退避时间1小时
	desiredRelays:   2,                // 默认期望2个中继节点
	maxCandidateAge: 30 * time.Minute, // 默认候选节点最大存活30分钟
	minInterval:     30 * time.Second, // 默认最小间隔30秒
}

var (
	errAlreadyHavePeerSource = errors.New("只能使用一个 WithPeerSource 或 WithStaticRelays")
)

// Option 定义配置函数类型
// 参数:
//   - *config: 配置对象指针
//
// 返回值:
//   - error: 返回可能的错误
type Option func(*config) error

// WithStaticRelays 配置使用静态中继节点列表
// 参数:
//   - static: []peer.AddrInfo 静态中继节点列表
//
// 返回值:
//   - Option: 返回配置函数
func WithStaticRelays(static []peer.AddrInfo) Option {
	return func(c *config) error {
		if c.peerSource != nil {
			log.Debugf("已经设置了节点源")
			return errAlreadyHavePeerSource
		}

		WithPeerSource(func(ctx context.Context, numPeers int) <-chan peer.AddrInfo {
			if len(static) < numPeers {
				numPeers = len(static)
			}
			c := make(chan peer.AddrInfo, numPeers)
			defer close(c)

			for i := 0; i < numPeers; i++ {
				c <- static[i]
			}
			return c
		})(c)
		WithMinCandidates(len(static))(c)
		WithMaxCandidates(len(static))(c)
		WithNumRelays(len(static))(c)

		return nil
	}
}

// WithPeerSource 为 AutoRelay 定义一个回调函数,用于查询更多的中继候选节点
// 参数:
//   - f: PeerSource 节点源函数
//
// 返回值:
//   - Option: 返回配置函数
func WithPeerSource(f PeerSource) Option {
	return func(c *config) error {
		if c.peerSource != nil {
			log.Debugf("已经设置了节点源")
			return errAlreadyHavePeerSource
		}
		c.peerSource = f
		return nil
	}
}

// WithNumRelays 设置我们努力获取预约的中继节点数量
// 参数:
//   - n: int 期望的中继节点数量
//
// 返回值:
//   - Option: 返回配置函数
func WithNumRelays(n int) Option {
	return func(c *config) error {
		c.desiredRelays = n
		return nil
	}
}

// WithMaxCandidates 设置我们缓存的中继候选节点数量
// 参数:
//   - n: int 最大候选节点数量
//
// 返回值:
//   - Option: 返回配置函数
func WithMaxCandidates(n int) Option {
	return func(c *config) error {
		c.maxCandidates = n
		if c.minCandidates > n {
			c.minCandidates = n
		}
		return nil
	}
}

// WithMinCandidates 设置在与任何候选节点获取预约之前收集的最小候选节点数量
// 参数:
//   - n: int 最小候选节点数量
//
// 返回值:
//   - Option: 返回配置函数
func WithMinCandidates(n int) Option {
	return func(c *config) error {
		if n > c.maxCandidates {
			n = c.maxCandidates
		}
		c.minCandidates = n
		c.setMinCandidates = true
		return nil
	}
}

// WithBootDelay 设置查找中继的启动延迟
// 参数:
//   - d: time.Duration 延迟时间
//
// 返回值:
//   - Option: 返回配置函数
func WithBootDelay(d time.Duration) Option {
	return func(c *config) error {
		c.bootDelay = d
		return nil
	}
}

// WithBackoff 设置与候选节点获取预约失败后的等待时间
// 参数:
//   - d: time.Duration 退避时间
//
// 返回值:
//   - Option: 返回配置函数
func WithBackoff(d time.Duration) Option {
	return func(c *config) error {
		c.backoff = d
		return nil
	}
}

// WithMaxCandidateAge 设置候选节点的最大年龄
// 参数:
//   - d: time.Duration 最大存活时间
//
// 返回值:
//   - Option: 返回配置函数
func WithMaxCandidateAge(d time.Duration) Option {
	return func(c *config) error {
		c.maxCandidateAge = d
		return nil
	}
}

// InstantTimer 定义了在特定时刻触发的计时器接口
type InstantTimer interface {
	Reset(d time.Time) bool // 重置计时器到指定时间
	Stop() bool             // 停止计时器
	Ch() <-chan time.Time   // 获取计时器通道
}

// ClockWithInstantTimer 定义了带即时计时器的时钟接口
type ClockWithInstantTimer interface {
	Now() time.Time                           // 获取当前时间
	Since(t time.Time) time.Duration          // 计算自指定时间以来的持续时间
	InstantTimer(when time.Time) InstantTimer // 创建即时计时器
}

// RealTimer 实现了 InstantTimer 接口
type RealTimer struct {
	t *time.Timer // 内部计时器
}

var _ InstantTimer = (*RealTimer)(nil)

// Ch 获取计时器通道
// 返回值:
//   - <-chan time.Time: 返回只读的时间通道
func (t RealTimer) Ch() <-chan time.Time {
	return t.t.C
}

// Reset 重置计时器到指定时间
// 参数:
//   - d: time.Time 目标时间
//
// 返回值:
//   - bool: 返回是否成功重置
func (t RealTimer) Reset(d time.Time) bool {
	return t.t.Reset(time.Until(d))
}

// Stop 停止计时器
// 返回值:
//   - bool: 返回是否成功停止
func (t RealTimer) Stop() bool {
	return t.t.Stop()
}

// RealClock 实现了 ClockWithInstantTimer 接口
type RealClock struct{}

var _ ClockWithInstantTimer = RealClock{}

// Now 获取当前时间
// 返回值:
//   - time.Time: 返回当前时间
func (RealClock) Now() time.Time {
	return time.Now()
}

// Since 计算自指定时间以来的持续时间
// 参数:
//   - t: time.Time 起始时间
//
// 返回值:
//   - time.Duration: 返回持续时间
func (RealClock) Since(t time.Time) time.Duration {
	return time.Since(t)
}

// InstantTimer 创建即时计时器
// 参数:
//   - when: time.Time 触发时间
//
// 返回值:
//   - InstantTimer: 返回即时计时器实例
func (RealClock) InstantTimer(when time.Time) InstantTimer {
	t := time.NewTimer(time.Until(when))
	return &RealTimer{t}
}

// WithClock 配置使用指定的时钟实例
// 参数:
//   - cl: ClockWithInstantTimer 时钟实例
//
// 返回值:
//   - Option: 返回配置函数
func WithClock(cl ClockWithInstantTimer) Option {
	return func(c *config) error {
		c.clock = cl
		return nil
	}
}

// WithMinInterval 设置调用 peerSource 回调的最小间隔时间
// 参数:
//   - interval: time.Duration 最小间隔时间
//
// 返回值:
//   - Option: 返回配置函数
func WithMinInterval(interval time.Duration) Option {
	return func(c *config) error {
		c.minInterval = interval
		return nil
	}
}

// WithMetricsTracer 配置 autorelay 使用指定的指标追踪器
// 参数:
//   - mt: MetricsTracer 指标追踪器实例
//
// 返回值:
//   - Option: 返回配置函数
func WithMetricsTracer(mt MetricsTracer) Option {
	return func(c *config) error {
		c.metricsTracer = mt
		return nil
	}
}
