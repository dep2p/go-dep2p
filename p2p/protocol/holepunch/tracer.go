package holepunch

import (
	"context"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// 垃圾回收间隔时间
const (
	tracerGCInterval    = 2 * time.Minute
	tracerCacheDuration = 5 * time.Minute
)

// WithTracer 使用 EventTracer 启用打洞追踪
// 参数:
//   - et: EventTracer 事件追踪器
//
// 返回值:
//   - Option 配置选项函数
func WithTracer(et EventTracer) Option {
	return func(hps *Service) error {
		hps.tracer = &tracer{
			et:    et,
			mt:    nil,
			self:  hps.host.ID(),
			peers: make(map[peer.ID]peerInfo),
		}
		return nil
	}
}

// WithMetricsTracer 使用 MetricsTracer 启用打洞追踪
// 参数:
//   - mt: MetricsTracer 指标追踪器
//
// 返回值:
//   - Option 配置选项函数
func WithMetricsTracer(mt MetricsTracer) Option {
	return func(hps *Service) error {
		hps.tracer = &tracer{
			et:    nil,
			mt:    mt,
			self:  hps.host.ID(),
			peers: make(map[peer.ID]peerInfo),
		}
		return nil
	}
}

// WithMetricsAndEventTracer 同时使用 MetricsTracer 和 EventTracer 启用打洞追踪
// 参数:
//   - mt: MetricsTracer 指标追踪器
//   - et: EventTracer 事件追踪器
//
// 返回值:
//   - Option 配置选项函数
func WithMetricsAndEventTracer(mt MetricsTracer, et EventTracer) Option {
	return func(hps *Service) error {
		hps.tracer = &tracer{
			et:    et,
			mt:    mt,
			self:  hps.host.ID(),
			peers: make(map[peer.ID]peerInfo),
		}
		return nil
	}
}

// tracer 追踪器结构体
type tracer struct {
	et   EventTracer   // 事件追踪器
	mt   MetricsTracer // 指标追踪器
	self peer.ID       // 本地节点 ID

	refCount  sync.WaitGroup     // 引用计数
	ctx       context.Context    // 上下文
	ctxCancel context.CancelFunc // 上下文取消函数

	mutex sync.Mutex           // 互斥锁
	peers map[peer.ID]peerInfo // 节点信息映射
}

// peerInfo 节点信息结构体
type peerInfo struct {
	counter int       // 计数器
	last    time.Time // 最后更新时间
}

// EventTracer 事件追踪器接口
type EventTracer interface {
	Trace(evt *Event)
}

// Event 事件结构体
type Event struct {
	Timestamp int64       // UNIX 纳秒时间戳
	Peer      peer.ID     // 本地节点 ID
	Remote    peer.ID     // 远程节点 ID
	Type      string      // 事件类型
	Evt       interface{} // 具体事件
}

// 事件类型常量
const (
	DirectDialEvtT       = "DirectDial"       // 直接拨号
	ProtocolErrorEvtT    = "ProtocolError"    // 协议错误
	StartHolePunchEvtT   = "StartHolePunch"   // 开始打洞
	EndHolePunchEvtT     = "EndHolePunch"     // 结束打洞
	HolePunchAttemptEvtT = "HolePunchAttempt" // 打洞尝试
)

// DirectDialEvt 直接拨号事件
type DirectDialEvt struct {
	Success      bool          // 是否成功
	EllapsedTime time.Duration // 耗时
	Error        string        `json:",omitempty"` // 错误信息
}

// ProtocolErrorEvt 协议错误事件
type ProtocolErrorEvt struct {
	Error string // 错误信息
}

// StartHolePunchEvt 开始打洞事件
type StartHolePunchEvt struct {
	RemoteAddrs []string      // 远程地址列表
	RTT         time.Duration // 往返时延
}

// EndHolePunchEvt 结束打洞事件
type EndHolePunchEvt struct {
	Success      bool          // 是否成功
	EllapsedTime time.Duration // 耗时
	Error        string        `json:",omitempty"` // 错误信息
}

// HolePunchAttemptEvt 打洞尝试事件
type HolePunchAttemptEvt struct {
	Attempt int // 尝试次数
}

// DirectDialSuccessful 记录直接拨号成功事件
// 参数:
//   - p: peer.ID 目标节点 ID
//   - dt: time.Duration 耗时
func (t *tracer) DirectDialSuccessful(p peer.ID, dt time.Duration) {
	if t == nil {
		return
	}

	if t.et != nil {
		t.et.Trace(&Event{
			Timestamp: time.Now().UnixNano(),
			Peer:      t.self,
			Remote:    p,
			Type:      DirectDialEvtT,
			Evt: &DirectDialEvt{
				Success:      true,
				EllapsedTime: dt,
			},
		})
	}

	if t.mt != nil {
		t.mt.DirectDialFinished(true)
	}
}

// DirectDialFailed 记录直接拨号失败事件
// 参数:
//   - p: peer.ID 目标节点 ID
//   - dt: time.Duration 耗时
//   - err: error 错误信息
func (t *tracer) DirectDialFailed(p peer.ID, dt time.Duration, err error) {
	if t == nil {
		return
	}

	if t.et != nil {
		t.et.Trace(&Event{
			Timestamp: time.Now().UnixNano(),
			Peer:      t.self,
			Remote:    p,
			Type:      DirectDialEvtT,
			Evt: &DirectDialEvt{
				Success:      false,
				EllapsedTime: dt,
				Error:        err.Error(),
			},
		})
	}

	if t.mt != nil {
		t.mt.DirectDialFinished(false)
	}
}

// ProtocolError 记录协议错误事件
// 参数:
//   - p: peer.ID 目标节点 ID
//   - err: error 错误信息
func (t *tracer) ProtocolError(p peer.ID, err error) {
	if t != nil && t.et != nil {
		t.et.Trace(&Event{
			Timestamp: time.Now().UnixNano(),
			Peer:      t.self,
			Remote:    p,
			Type:      ProtocolErrorEvtT,
			Evt: &ProtocolErrorEvt{
				Error: err.Error(),
			},
		})
	}
}

// StartHolePunch 记录开始打洞事件
// 参数:
//   - p: peer.ID 目标节点 ID
//   - obsAddrs: []ma.Multiaddr 观察到的地址列表
//   - rtt: time.Duration 往返时延
func (t *tracer) StartHolePunch(p peer.ID, obsAddrs []ma.Multiaddr, rtt time.Duration) {
	if t != nil && t.et != nil {
		addrs := make([]string, 0, len(obsAddrs))
		for _, a := range obsAddrs {
			addrs = append(addrs, a.String())
		}

		t.et.Trace(&Event{
			Timestamp: time.Now().UnixNano(),
			Peer:      t.self,
			Remote:    p,
			Type:      StartHolePunchEvtT,
			Evt: &StartHolePunchEvt{
				RemoteAddrs: addrs,
				RTT:         rtt,
			},
		})
	}
}

// EndHolePunch 记录结束打洞事件
// 参数:
//   - p: peer.ID 目标节点 ID
//   - dt: time.Duration 打洞耗时
//   - err: error 错误信息
//
// 注意:
//   - 当 err 为 nil 时表示打洞成功
func (t *tracer) EndHolePunch(p peer.ID, dt time.Duration, err error) {
	// 检查 tracer 和事件追踪器是否有效
	if t != nil && t.et != nil {
		// 构造结束打洞事件
		evt := &EndHolePunchEvt{
			Success:      err == nil, // 根据错误判断是否成功
			EllapsedTime: dt,         // 记录耗时
		}
		// 如果有错误则记录错误信息
		if err != nil {
			evt.Error = err.Error()
		}

		// 追踪事件
		t.et.Trace(&Event{
			Timestamp: time.Now().UnixNano(), // 当前时间戳
			Peer:      t.self,                // 本节点 ID
			Remote:    p,                     // 远程节点 ID
			Type:      EndHolePunchEvtT,      // 事件类型
			Evt:       evt,                   // 事件详情
		})
	}
}

// HolePunchFinished 记录打洞完成事件
// 参数:
//   - side: string 打洞发起方标识
//   - numAttempts: int 尝试次数
//   - theirAddrs: []ma.Multiaddr 对方地址列表
//   - ourAddrs: []ma.Multiaddr 本地地址列表
//   - directConn: network.Conn 直连连接
func (t *tracer) HolePunchFinished(side string, numAttempts int, theirAddrs []ma.Multiaddr, ourAddrs []ma.Multiaddr, directConn network.Conn) {
	// 检查 tracer 和指标追踪器是否有效
	if t != nil && t.mt != nil {
		t.mt.HolePunchFinished(side, numAttempts, theirAddrs, ourAddrs, directConn)
	}
}

// HolePunchAttempt 记录打洞尝试事件
// 参数:
//   - p: peer.ID 目标节点 ID
func (t *tracer) HolePunchAttempt(p peer.ID) {
	// 检查 tracer 和事件追踪器是否有效
	if t != nil && t.et != nil {
		now := time.Now()
		// 更新打洞尝试计数
		t.mutex.Lock()
		attempt := t.peers[p]
		attempt.counter++
		counter := attempt.counter
		attempt.last = now
		t.peers[p] = attempt
		t.mutex.Unlock()

		// 追踪事件
		t.et.Trace(&Event{
			Timestamp: now.UnixNano(),                         // 当前时间戳
			Peer:      t.self,                                 // 本节点 ID
			Remote:    p,                                      // 远程节点 ID
			Type:      HolePunchAttemptEvtT,                   // 事件类型
			Evt:       &HolePunchAttemptEvt{Attempt: counter}, // 尝试次数
		})
	}
}

// gc 清理过期的节点记录
// 注意:
//   - 仅当 tracer 初始化时提供了有效的 EventTracer 才会运行
func (t *tracer) gc() {
	defer t.refCount.Done()
	// 创建定时器
	timer := time.NewTicker(tracerGCInterval)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			// 清理过期记录
			now := time.Now()
			t.mutex.Lock()
			for id, entry := range t.peers {
				if entry.last.Before(now.Add(-tracerCacheDuration)) {
					delete(t.peers, id)
				}
			}
			t.mutex.Unlock()
		case <-t.ctx.Done():
			return
		}
	}
}

// Start 启动 tracer
func (t *tracer) Start() {
	// 检查 tracer 和事件追踪器是否有效
	if t != nil && t.et != nil {
		t.ctx, t.ctxCancel = context.WithCancel(context.Background())
		t.refCount.Add(1)
		go t.gc() // 启动垃圾回收协程
	}
}

// Close 关闭 tracer
// 返回值:
//   - error 关闭过程中的错误
func (t *tracer) Close() error {
	// 检查 tracer 和事件追踪器是否有效
	if t != nil && t.et != nil {
		t.ctxCancel()     // 取消上下文
		t.refCount.Wait() // 等待所有协程结束
	}
	return nil
}
