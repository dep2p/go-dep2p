// Package client 中继客户端
//
// 本文件实现 RelayCircuit：支持多路复用的中继电路。
//
// v2.0 重构：
// - 原 RelayedConn 仅支持单流，每次 NewStream() 返回同一个流
// - 新 RelayCircuit 在 STOP 流上叠加 yamux muxer，支持真正的多路复用
// - 单个流关闭不影响整个电路
// - 实现 pkgif.Connection 接口
package client

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/muxer"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/multiaddr"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              错误定义
// ============================================================================

var (
	// ErrCircuitClosed 电路已关闭
	ErrCircuitClosed = errors.New("relay circuit closed")

	// ErrCircuitNotActive 电路非活跃状态
	ErrCircuitNotActive = errors.New("relay circuit not active")

	// ErrMuxerCreationFailed muxer 创建失败
	ErrMuxerCreationFailed = errors.New("failed to create muxer on relay stream")
)

// ============================================================================
//                              电路状态
// ============================================================================

// CircuitState 电路状态
type CircuitState int32

const (
	// CircuitStateCreating 正在创建
	CircuitStateCreating CircuitState = iota
	// CircuitStateActive 活跃状态
	CircuitStateActive
	// CircuitStateStale 失效状态（心跳超时）
	CircuitStateStale
	// CircuitStateClosed 已关闭
	CircuitStateClosed
)

// String 返回状态字符串
func (s CircuitState) String() string {
	switch s {
	case CircuitStateCreating:
		return "creating"
	case CircuitStateActive:
		return "active"
	case CircuitStateStale:
		return "stale"
	case CircuitStateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              常量定义
// ============================================================================

const (
	// CircuitKeepAliveInterval 电路心跳间隔
	CircuitKeepAliveInterval = 30 * time.Second

	// CircuitIdleTimeout 电路空闲超时
	CircuitIdleTimeout = 2 * time.Minute

	// CircuitPongTimeout 心跳响应超时
	CircuitPongTimeout = 2 * CircuitKeepAliveInterval
)

const (
	controlMsgPing = byte(1)
	controlMsgPong = byte(2)
)

// ============================================================================
//                              RelayCircuit 结构
// ============================================================================

// RelayCircuit 中继电路
//
// v2.0 重构：在 STOP 流上叠加 yamux muxer，支持真正的多路复用。
// 实现 pkgif.Connection 接口。
//
// 关键改进：
// - 原来：单流包装，NewStream() 每次返回同一个流，流关闭后连接失效
// - 现在：yamux 多路复用，可以创建多个独立流，单流关闭不影响电路
type RelayCircuit struct {
	// 底层 STOP 流（作为 muxer 的传输层）
	baseStream pkgif.Stream

	// yamux muxer（关键！）
	muxer pkgif.MuxedConn

	// 身份信息
	localPeer  types.PeerID
	remotePeer types.PeerID
	relayPeer  types.PeerID

	// 地址信息
	localAddr  types.Multiaddr
	remoteAddr types.Multiaddr

	// 连接方向
	direction pkgif.Direction

	// 控制通道
	controlEnabled bool
	initiator      bool
	controlMu      sync.Mutex
	controlStream  pkgif.MuxedStream
	controlReady   chan struct{}
	lastPong       atomic.Int64

	// 状态机
	state atomic.Int32

	// 活跃流跟踪
	streams   []pkgif.Stream
	streamsMu sync.RWMutex

	// 活动时间
	lastActivity atomic.Int64
	createdAt    time.Time

	// 配额管理
	bytesUsed atomic.Int64
	maxBytes  int64
	deadline  time.Time

	// 生命周期
	ctx    context.Context
	cancel context.CancelFunc

	// 关闭同步
	closeOnce sync.Once

	// KeepAlive
	keepAliveStarted atomic.Bool

	// 流接受循环（防止重复启动）
	acceptLoopOnce sync.Once

	// 优雅关闭
	draining atomic.Bool

	// EventBus（用于发射状态变更事件）
	eventbus pkgif.EventBus

	// 生命周期协调器回调（用于通知可达性变化）
	onStateChanged func(oldState, newState CircuitState)
}

// 确保实现接口
var _ pkgif.Connection = (*RelayCircuit)(nil)

// ============================================================================
//                              构造函数
// ============================================================================

// CircuitOptions 电路配置选项
type CircuitOptions struct {
	// Direction 连接方向（入站/出站）
	Direction pkgif.Direction
	// MaxBytes 最大字节数配额（0 表示无限制）
	MaxBytes int64
	// Deadline 电路截止时间（零值表示无限制）
	Deadline time.Time
	// ControlEnabled 是否启用控制通道（心跳）
	ControlEnabled bool
	// Initiator 是否为发起方（用于控制通道创建）
	Initiator bool
	// EventBus 事件总线（可选，用于发射状态变更事件）
	EventBus pkgif.EventBus
	// OnStateChanged 状态变更回调（可选，用于通知生命周期协调器）
	OnStateChanged func(oldState, newState CircuitState)
}

// DefaultCircuitOptions 默认电路选项
func DefaultCircuitOptions() CircuitOptions {
	return CircuitOptions{
		Direction:      pkgif.DirOutbound,
		MaxBytes:       0,           // 无限制
		Deadline:       time.Time{}, // 默认不设置截止时间
		ControlEnabled: true,
		Initiator:      true,
	}
}

// NewRelayCircuit 创建新的中继电路
//
// 参数：
//   - baseStream: STOP 握手完成后的流（作为 muxer 的传输层）
//   - muxer: 在 baseStream 上创建的 yamux muxer
//   - localPeer: 本地节点 ID
//   - remotePeer: 远端节点 ID（实际通信目标）
//   - relayPeer: 中继节点 ID
//   - relayTransportAddr: Relay 的传输层地址（如 /ip4/.../udp/.../quic-v1）
func NewRelayCircuit(
	baseStream pkgif.Stream,
	muxer pkgif.MuxedConn,
	localPeer, remotePeer, relayPeer types.PeerID,
	relayTransportAddr types.Multiaddr,
) *RelayCircuit {
	c := NewRelayCircuitWithOptions(baseStream, muxer, localPeer, remotePeer, relayPeer, relayTransportAddr, DefaultCircuitOptions())
	if err := c.initControlAsInitiator(); err != nil {
		clientLogger.Warn("初始化控制通道失败，降级为无控制通道",
			"remotePeer", safePeerIDPrefix(remotePeer),
			"error", err)
		c.controlEnabled = false
		select {
		case <-c.controlReady:
		default:
			close(c.controlReady)
		}
	}
	return c
}

// NewRelayCircuitWithOptions 使用选项创建中继电路
//
// 完整格式：/ip4/.../udp/.../quic-v1/p2p/<relayPeer>/p2p-circuit/p2p/<remotePeer>
func NewRelayCircuitWithOptions(
	baseStream pkgif.Stream,
	muxer pkgif.MuxedConn,
	localPeer, remotePeer, relayPeer types.PeerID,
	relayTransportAddr types.Multiaddr,
	opts CircuitOptions,
) *RelayCircuit {
	ctx, cancel := context.WithCancel(context.Background())

	// 构建地址
	localAddr, _ := multiaddr.NewMultiaddr("/p2p/" + string(localPeer))

	//
	// 完整格式：<relayTransportAddr>/p2p/<relayPeer>/p2p-circuit/p2p/<remotePeer>
	var remoteAddr types.Multiaddr
	circuitSuffix := "/p2p/" + string(relayPeer) + "/p2p-circuit/p2p/" + string(remotePeer)
	if relayTransportAddr != nil {
		// 使用完整的传输地址
		remoteAddr, _ = multiaddr.NewMultiaddr(relayTransportAddr.String() + circuitSuffix)
	} else {
		// 仅使用 PeerID（传输地址不可用时）
		remoteAddr, _ = multiaddr.NewMultiaddr(circuitSuffix)
	}
	clientLogger.Debug("构建 RemoteMultiaddr",
		"relayTransportAddr", relayTransportAddr,
		"remoteAddr", remoteAddr)

	c := &RelayCircuit{
		baseStream:     baseStream,
		muxer:          muxer,
		localPeer:      localPeer,
		remotePeer:     remotePeer,
		relayPeer:      relayPeer,
		localAddr:      localAddr,
		remoteAddr:     remoteAddr,
		direction:      opts.Direction,
		controlEnabled: opts.ControlEnabled,
		initiator:      opts.Initiator,
		controlReady:   make(chan struct{}),
		streams:        make([]pkgif.Stream, 0),
		createdAt:      time.Now(),
		maxBytes:       opts.MaxBytes,
		deadline:       opts.Deadline,
		ctx:            ctx,
		cancel:         cancel,
		eventbus:       opts.EventBus,
		onStateChanged: opts.OnStateChanged,
	}

	// 设置初始状态为活跃
	c.state.Store(int32(CircuitStateActive))
	c.updateActivity()

	clientLogger.Info("中继电路已创建",
		"localPeer", safePeerIDPrefix(localPeer),
		"remotePeer", safePeerIDPrefix(remotePeer),
		"relayPeer", safePeerIDPrefix(relayPeer),
		"direction", opts.Direction)

	// 初始化心跳时间
	c.lastPong.Store(time.Now().UnixNano())

	// 启动心跳（幂等）
	c.StartKeepAlive()

	// 如果不启用控制通道，直接标记就绪
	if !c.controlEnabled {
		close(c.controlReady)
	}

	return c
}

// ============================================================================
//                              Connection 接口实现
// ============================================================================

// LocalPeer 返回本地节点 ID
func (c *RelayCircuit) LocalPeer() types.PeerID {
	return c.localPeer
}

// LocalMultiaddr 返回本地多地址
func (c *RelayCircuit) LocalMultiaddr() types.Multiaddr {
	return c.localAddr
}

// RemotePeer 返回远端节点 ID
func (c *RelayCircuit) RemotePeer() types.PeerID {
	return c.remotePeer
}

// RemoteMultiaddr 返回远端多地址（通过中继）
func (c *RelayCircuit) RemoteMultiaddr() types.Multiaddr {
	return c.remoteAddr
}

// NewStream 在此电路上创建新流
//
// v2.0 重构：通过 yamux muxer 创建新的逻辑流。
// 单个流的关闭不会影响电路和其他流。
//
// 只有 Closed 和 Draining 状态才拒绝操作。
func (c *RelayCircuit) NewStream(ctx context.Context) (pkgif.Stream, error) {
	//
	state := c.State()
	if state == CircuitStateClosed {
		return nil, ErrCircuitNotActive
	}
	if c.draining.Load() {
		return nil, ErrCircuitNotActive
	}

	// 等待控制通道就绪
	if c.controlEnabled {
		select {
		case <-c.controlReady:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	// 通过 muxer 创建新流
	muxedStream, err := c.muxer.OpenStream(ctx)
	if err != nil {
		clientLogger.Warn("创建流失败", "error", err)
		return nil, err
	}

	// 更新活动时间
	c.updateActivity()

	//
	if state == CircuitStateStale {
		clientLogger.Info("电路恢复活跃（NewStream 活动）",
			"remotePeer", safePeerIDPrefix(c.remotePeer))
		c.setStateWithReason(CircuitStateActive, "activity_resumed")
	}

	// 包装为 pkgif.Stream
	wrapped := newCircuitStream(muxedStream, c)

	// 跟踪流
	c.trackStream(wrapped)

	clientLogger.Debug("创建新流成功",
		"remotePeer", safePeerIDPrefix(c.remotePeer))

	return wrapped, nil
}

// AcceptStream 接受对方创建的流
//
// 只有 Closed 状态才拒绝操作。这样可以打破"Stale → 无法接受流 →
// 无法更新活动时间 → 状态无法恢复"的死锁循环。
func (c *RelayCircuit) AcceptStream() (pkgif.Stream, error) {
	//
	state := c.State()
	if state == CircuitStateClosed {
		return nil, ErrCircuitNotActive
	}

	for {
		// 通过 muxer 接受流
		muxedStream, err := c.muxer.AcceptStream()
		if err != nil {
			return nil, err
		}

		// 控制通道优先绑定（接收方）
		if c.controlEnabled && !c.initiator {
			select {
			case <-c.controlReady:
			default:
				c.attachControlStream(muxedStream)
				continue
			}
		}

		// 更新活动时间（关键：即使 Stale 状态也要更新）
		c.updateActivity()

		//
		// 重新获取状态，因为可能在 muxer.AcceptStream() 期间状态已变化
		currentState := c.State()
		if currentState == CircuitStateStale {
			clientLogger.Info("电路恢复活跃（AcceptStream 活动）",
				"remotePeer", safePeerIDPrefix(c.remotePeer))
			c.setStateWithReason(CircuitStateActive, "activity_resumed")
		}

		// 包装为 pkgif.Stream
		wrapped := newCircuitStream(muxedStream, c)

		// 跟踪流
		c.trackStream(wrapped)

		clientLogger.Debug("接受新流",
			"remotePeer", safePeerIDPrefix(c.remotePeer))

		return wrapped, nil
	}
}

// GetStreams 获取此电路上的所有流
func (c *RelayCircuit) GetStreams() []pkgif.Stream {
	c.streamsMu.RLock()
	defer c.streamsMu.RUnlock()

	// 过滤已关闭的流
	active := make([]pkgif.Stream, 0, len(c.streams))
	for _, s := range c.streams {
		if !s.IsClosed() {
			active = append(active, s)
		}
	}
	return active
}

// Stat 返回连接统计
func (c *RelayCircuit) Stat() pkgif.ConnectionStat {
	return pkgif.ConnectionStat{
		Direction:  c.direction,
		Transient:  true, // 中继连接是临时的
		Opened:     c.createdAt.Unix(),
		NumStreams: len(c.GetStreams()),
	}
}

// ConnType 返回连接类型（v2.0 新增）
func (c *RelayCircuit) ConnType() pkgif.ConnectionType {
	return pkgif.ConnectionTypeRelay
}

// Close 关闭电路
func (c *RelayCircuit) Close() error {
	var err error
	c.closeOnce.Do(func() {
		clientLogger.Info("关闭中继电路",
			"remotePeer", safePeerIDPrefix(c.remotePeer))

		// 更新状态
		c.state.Store(int32(CircuitStateClosed))

		// 取消 context
		c.cancel()

		// 获取所有流的副本（避免在关闭时持有锁）
		c.streamsMu.Lock()
		streams := make([]pkgif.Stream, len(c.streams))
		copy(streams, c.streams)
		c.streams = nil
		c.streamsMu.Unlock()

		// 关闭所有流（不持有锁，避免死锁）
		for _, s := range streams {
			// 直接关闭底层 muxed stream，不调用 circuitStream.Close()
			// 因为 circuitStream.Close() 会尝试调用 removeStream
			if cs, ok := s.(*circuitStream); ok {
				cs.closed.Store(true)
				_ = cs.muxedStream.Close()
			} else {
				_ = s.Close()
			}
		}

		// 关闭控制通道
		c.controlMu.Lock()
		if c.controlStream != nil {
			_ = c.controlStream.Close()
			c.controlStream = nil
		}
		c.controlMu.Unlock()

		// 关闭 muxer
		if c.muxer != nil {
			if muxErr := c.muxer.Close(); muxErr != nil {
				err = muxErr
			}
		}

		// 关闭底层流
		if c.baseStream != nil {
			_ = c.baseStream.Close()
		}
	})
	return err
}

// GracefulClose 优雅关闭电路
//
// 1. 停止创建新流
// 2. 关闭控制通道，提示对端停止新建流
// 3. 等待现有流完成（最多 timeout）
// 4. 最终关闭电路
func (c *RelayCircuit) GracefulClose(timeout time.Duration) error {
	if c.IsClosed() {
		return nil
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	c.draining.Store(true)

	// 停止控制通道，阻止新流建立
	c.controlMu.Lock()
	if c.controlStream != nil {
		_ = c.controlStream.Close()
		c.controlStream = nil
	}
	c.controlMu.Unlock()

	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		if len(c.GetStreams()) == 0 {
			return c.Close()
		}
		select {
		case <-deadline.C:
			return c.Close()
		case <-ticker.C:
		}
	}
}

// IsClosed 检查电路是否已关闭
func (c *RelayCircuit) IsClosed() bool {
	return c.State() == CircuitStateClosed
}

// Done 返回电路的完成信号（用于外部监听关闭）
func (c *RelayCircuit) Done() <-chan struct{} {
	return c.ctx.Done()
}

// ============================================================================
//                              状态管理
// ============================================================================

// State 返回当前状态
func (c *RelayCircuit) State() CircuitState {
	return CircuitState(c.state.Load())
}

// SetState 设置状态
func (c *RelayCircuit) SetState(state CircuitState) {
	c.setStateWithReason(state, "")
}

// setStateWithReason 设置状态（带原因）
func (c *RelayCircuit) setStateWithReason(state CircuitState, reason string) {
	old := CircuitState(c.state.Swap(int32(state)))
	if old != state {
		clientLogger.Debug("电路状态变更",
			"remotePeer", safePeerIDPrefix(c.remotePeer),
			"from", old.String(),
			"to", state.String(),
			"reason", reason)

		// 调用状态变更回调（同步）
		if c.onStateChanged != nil {
			c.onStateChanged(old, state)
		}

		// 发射事件到 EventBus（异步）
		c.emitStateChangedEvent(old, state, reason)
	}
}

// emitStateChangedEvent 发射状态变更事件
func (c *RelayCircuit) emitStateChangedEvent(oldState, newState CircuitState, reason string) {
	if c.eventbus == nil {
		return
	}

	emitter, err := c.eventbus.Emitter(&types.EvtRelayCircuitStateChanged{})
	if err != nil {
		clientLogger.Warn("无法创建状态变更事件发射器", "error", err)
		return
	}
	defer emitter.Close()

	evt := &types.EvtRelayCircuitStateChanged{
		BaseEvent:  types.NewBaseEvent(types.EventTypeRelayCircuitStateChanged),
		RelayPeer:  c.relayPeer,
		RemotePeer: c.remotePeer,
		OldState:   oldState.String(),
		NewState:   newState.String(),
		Reason:     reason,
	}

	if err := emitter.Emit(evt); err != nil {
		clientLogger.Warn("发射状态变更事件失败", "error", err)
	}
}

// updateActivity 更新最后活动时间
func (c *RelayCircuit) updateActivity() {
	c.lastActivity.Store(time.Now().UnixNano())
}

// LastActivity 返回最后活动时间
func (c *RelayCircuit) LastActivity() time.Time {
	return time.Unix(0, c.lastActivity.Load())
}

// trackStream 跟踪流
func (c *RelayCircuit) trackStream(s pkgif.Stream) {
	c.streamsMu.Lock()
	defer c.streamsMu.Unlock()
	c.streams = append(c.streams, s)
}

// removeStream 移除流跟踪
func (c *RelayCircuit) removeStream(s pkgif.Stream) {
	c.streamsMu.Lock()
	defer c.streamsMu.Unlock()
	for i, stream := range c.streams {
		if stream == s {
			c.streams = append(c.streams[:i], c.streams[i+1:]...)
			break
		}
	}
}

// ============================================================================
//                              心跳循环
// ============================================================================

// StartKeepAlive 启动心跳循环
//
// 定期检查电路健康状态，维护电路活跃。
// 应在电路创建后调用一次。
func (c *RelayCircuit) StartKeepAlive() {
	if !c.keepAliveStarted.CompareAndSwap(false, true) {
		return // 已经启动
	}
	go c.keepAliveLoop()
}

// keepAliveLoop 心跳循环
//
// 定期检查：
// 1. 最后活动时间是否超时
// 2. 配额是否耗尽
// 3. 是否超过截止时间
func (c *RelayCircuit) keepAliveLoop() {
	ticker := time.NewTicker(CircuitKeepAliveInterval)
	defer ticker.Stop()

	clientLogger.Debug("心跳循环已启动",
		"remotePeer", safePeerIDPrefix(c.remotePeer))

	for {
		select {
		case <-c.ctx.Done():
			clientLogger.Debug("心跳循环退出（context 取消）",
				"remotePeer", safePeerIDPrefix(c.remotePeer))
			return
		case <-ticker.C:
			c.checkHealth()
			c.sendPing()
		}
	}
}

// checkHealth 检查电路健康状态
func (c *RelayCircuit) checkHealth() {
	if c.IsClosed() {
		return
	}

	// 1. 检查配额（如果设置了限制）
	if c.maxBytes > 0 && c.bytesUsed.Load() >= c.maxBytes {
		clientLogger.Info("电路配额耗尽，关闭电路",
			"remotePeer", safePeerIDPrefix(c.remotePeer),
			"bytesUsed", c.bytesUsed.Load(),
			"maxBytes", c.maxBytes)
		c.setStateWithReason(CircuitStateClosed, "quota_exhausted")
		_ = c.Close()
		return
	}

	// 2. 检查截止时间（如果设置了）
	if !c.deadline.IsZero() && time.Now().After(c.deadline) {
		clientLogger.Info("电路已过期，关闭电路",
			"remotePeer", safePeerIDPrefix(c.remotePeer),
			"deadline", c.deadline)
		c.setStateWithReason(CircuitStateClosed, "deadline_exceeded")
		_ = c.Close()
		return
	}

	// 3. 检查空闲超时
	lastActive := c.LastActivity()
	idleTime := time.Since(lastActive)

	// 控制通道心跳超时
	if c.controlEnabled {
		select {
		case <-c.controlReady:
			lastPong := time.Unix(0, c.lastPong.Load())
			if time.Since(lastPong) > CircuitPongTimeout {
				clientLogger.Warn("控制通道心跳超时，关闭电路",
					"remotePeer", safePeerIDPrefix(c.remotePeer),
					"lastPong", lastPong)
				c.setStateWithReason(CircuitStateClosed, "heartbeat_timeout")
				_ = c.Close()
				return
			}
		default:
		}
	}

	if idleTime > CircuitIdleTimeout {
		// 超过空闲超时，转为 Stale 状态
		if c.State() == CircuitStateActive {
			clientLogger.Warn("电路空闲超时，标记为 Stale",
				"remotePeer", safePeerIDPrefix(c.remotePeer),
				"idleTime", idleTime)
			c.setStateWithReason(CircuitStateStale, "idle_timeout")
		} else if c.State() == CircuitStateStale {
			// 已经是 Stale 状态，再次超时则关闭
			clientLogger.Warn("Stale 电路继续空闲，关闭电路",
				"remotePeer", safePeerIDPrefix(c.remotePeer))
			c.setStateWithReason(CircuitStateClosed, "stale_timeout")
			_ = c.Close()
		}
	} else if c.State() == CircuitStateStale {
		// 有活动，恢复为 Active
		clientLogger.Info("电路恢复活跃",
			"remotePeer", safePeerIDPrefix(c.remotePeer))
		c.setStateWithReason(CircuitStateActive, "activity_resumed")
	}
}

// AddBytesUsed 增加已使用字节数（用于配额管理）
func (c *RelayCircuit) AddBytesUsed(n int64) {
	c.bytesUsed.Add(n)
}

// BytesUsed 返回已使用字节数
func (c *RelayCircuit) BytesUsed() int64 {
	return c.bytesUsed.Load()
}

// initControlAsInitiator 初始化控制通道（发起方）
func (c *RelayCircuit) initControlAsInitiator() error {
	if !c.controlEnabled {
		select {
		case <-c.controlReady:
		default:
			close(c.controlReady)
		}
		return nil
	}
	if !c.initiator {
		return nil
	}

	// 创建控制流
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := c.muxer.OpenStream(ctx)
	if err != nil {
		return err
	}

	c.attachControlStream(stream)
	return nil
}

// attachControlStream 附加控制流（接收方）
func (c *RelayCircuit) attachControlStream(stream pkgif.MuxedStream) {
	c.controlMu.Lock()
	if c.controlStream == nil {
		c.controlStream = stream
		close(c.controlReady)
		c.lastPong.Store(time.Now().UnixNano())
		go c.controlReadLoop()
	} else {
		_ = stream.Close()
	}
	c.controlMu.Unlock()
}

// controlReadLoop 控制通道读循环
func (c *RelayCircuit) controlReadLoop() {
	buf := make([]byte, 1)
	for {
		if c.IsClosed() {
			return
		}
		n, err := c.readControl(buf)
		if err != nil {
			_ = c.Close()
			return
		}
		if n != 1 {
			continue
		}
		switch buf[0] {
		case controlMsgPing:
			_ = c.writeControl([]byte{controlMsgPong})
		case controlMsgPong:
			c.lastPong.Store(time.Now().UnixNano())
		}
	}
}

func (c *RelayCircuit) readControl(buf []byte) (int, error) {
	c.controlMu.Lock()
	stream := c.controlStream
	c.controlMu.Unlock()
	if stream == nil {
		return 0, io.EOF
	}
	return io.ReadFull(stream, buf)
}

func (c *RelayCircuit) writeControl(buf []byte) error {
	c.controlMu.Lock()
	stream := c.controlStream
	c.controlMu.Unlock()
	if stream == nil {
		return io.EOF
	}
	_, err := stream.Write(buf)
	return err
}

// sendPing 发送心跳 Ping
func (c *RelayCircuit) sendPing() {
	if !c.controlEnabled {
		return
	}
	select {
	case <-c.controlReady:
		_ = c.writeControl([]byte{controlMsgPing})
	default:
	}
}

// ============================================================================
//                              流接受循环
// ============================================================================

// AcceptStreamLoop 启动流接受循环
//
// 在后台持续接受入站流，并路由到 Host 处理器。
// 用于中继连接的接收方。
// 同时启动心跳循环。
func (c *RelayCircuit) AcceptStreamLoop(host pkgif.Host) {
	c.AcceptStreamLoopWithHandler(func(stream pkgif.Stream) {
		host.HandleInboundStream(stream)
	})
}

// AcceptStreamLoopWithHandler 启动流接受循环（使用自定义处理器）
//
// 通用版本的流接受循环，允许传入自定义的流处理函数。
// 这使得 Swarm 可以为出站 RelayCircuit 启动流接受循环，
// 而不需要对 Host 的引用。
//
// 使用 sync.Once 确保流接受循环只启动一次，防止 manager.go 和 addConn
// 同时调用导致的重复启动问题。方法立即返回，实际循环在后台 goroutine 运行。
//
// 参数：
//   - handler: 流处理函数，接收入站流并进行协议协商和路由
func (c *RelayCircuit) AcceptStreamLoopWithHandler(handler func(pkgif.Stream)) {
	c.acceptLoopOnce.Do(func() {
		// 在后台 goroutine 启动实际循环，使 Do() 立即返回
		// 这样重复调用不会阻塞
		go c.runAcceptStreamLoop(handler)
	})
}

// runAcceptStreamLoop 实际的流接受循环实现
//
// 由 AcceptStreamLoopWithHandler 通过 sync.Once 在后台 goroutine 调用，确保只执行一次。
func (c *RelayCircuit) runAcceptStreamLoop(handler func(pkgif.Stream)) {
	// 启动心跳
	c.StartKeepAlive()

	clientLogger.Debug("启动流接受循环",
		"remotePeer", safePeerIDPrefix(c.remotePeer))

	for {
		select {
		case <-c.ctx.Done():
			clientLogger.Debug("流接受循环退出（context 取消）",
				"remotePeer", safePeerIDPrefix(c.remotePeer))
			return
		default:
		}

		stream, err := c.AcceptStream()
		if err != nil {
			if c.IsClosed() {
				return
			}
			clientLogger.Warn("接受流失败，关闭电路",
				"remotePeer", safePeerIDPrefix(c.remotePeer),
				"error", err)
			_ = c.Close()
			return
		}

		// 控制通道优先绑定（接收方）
		if c.controlEnabled {
			select {
			case <-c.controlReady:
			default:
				if cs, ok := stream.(*circuitStream); ok {
					c.attachControlStream(cs.muxedStream)
					continue
				}
			}
		}

		// 路由到处理器
		handler(stream)
	}
}

// ============================================================================
//                              streamNetConn - 流到 net.Conn 适配器
// ============================================================================

// streamNetConn 将 pkgif.Stream 适配为 net.Conn
//
// yamux 需要 net.Conn 接口，此适配器将流包装为 net.Conn。
type streamNetConn struct {
	stream pkgif.Stream
}

// newStreamNetConn 创建流适配器
func newStreamNetConn(stream pkgif.Stream) net.Conn {
	return &streamNetConn{stream: stream}
}

func (c *streamNetConn) Read(b []byte) (n int, err error) {
	return c.stream.Read(b)
}

func (c *streamNetConn) Write(b []byte) (n int, err error) {
	return c.stream.Write(b)
}

func (c *streamNetConn) Close() error {
	return c.stream.Close()
}

func (c *streamNetConn) LocalAddr() net.Addr {
	return &relayNetAddr{addr: "local"}
}

func (c *streamNetConn) RemoteAddr() net.Addr {
	return &relayNetAddr{addr: "remote"}
}

func (c *streamNetConn) SetDeadline(t time.Time) error {
	return c.stream.SetDeadline(t)
}

func (c *streamNetConn) SetReadDeadline(t time.Time) error {
	return c.stream.SetReadDeadline(t)
}

func (c *streamNetConn) SetWriteDeadline(t time.Time) error {
	return c.stream.SetWriteDeadline(t)
}

// relayNetAddr 实现 net.Addr
type relayNetAddr struct {
	addr string
}

func (a *relayNetAddr) Network() string {
	return "p2p-circuit"
}

func (a *relayNetAddr) String() string {
	return a.addr
}

// ============================================================================
//                              circuitStream - 电路流包装器
// ============================================================================

// circuitStream 包装 muxer 流为 pkgif.Stream
type circuitStream struct {
	muxedStream pkgif.MuxedStream
	circuit     *RelayCircuit
	protocol    string
	openedAt    time.Time
	closed      atomic.Bool
}

// newCircuitStream 创建电路流
func newCircuitStream(muxedStream pkgif.MuxedStream, circuit *RelayCircuit) *circuitStream {
	return &circuitStream{
		muxedStream: muxedStream,
		circuit:     circuit,
		openedAt:    time.Now(),
	}
}

func (s *circuitStream) Read(p []byte) (n int, err error) {
	n, err = s.muxedStream.Read(p)
	if n > 0 {
		s.circuit.updateActivity()
		s.circuit.AddBytesUsed(int64(n))
	}
	return n, err
}

func (s *circuitStream) Write(p []byte) (n int, err error) {
	n, err = s.muxedStream.Write(p)
	if n > 0 {
		s.circuit.updateActivity()
		s.circuit.AddBytesUsed(int64(n))
	}
	return n, err
}

func (s *circuitStream) Close() error {
	if s.closed.CompareAndSwap(false, true) {
		s.circuit.removeStream(s)
		return s.muxedStream.Close()
	}
	return nil
}

func (s *circuitStream) CloseWrite() error {
	return s.muxedStream.CloseWrite()
}

func (s *circuitStream) CloseRead() error {
	return s.muxedStream.CloseRead()
}

func (s *circuitStream) Reset() error {
	s.closed.Store(true)
	s.circuit.removeStream(s)
	return s.muxedStream.Reset()
}

func (s *circuitStream) SetDeadline(t time.Time) error {
	return s.muxedStream.SetDeadline(t)
}

func (s *circuitStream) SetReadDeadline(t time.Time) error {
	return s.muxedStream.SetReadDeadline(t)
}

func (s *circuitStream) SetWriteDeadline(t time.Time) error {
	return s.muxedStream.SetWriteDeadline(t)
}

func (s *circuitStream) Protocol() string {
	return s.protocol
}

func (s *circuitStream) SetProtocol(protocol string) {
	s.protocol = protocol
}

func (s *circuitStream) Conn() pkgif.Connection {
	return s.circuit
}

func (s *circuitStream) IsClosed() bool {
	return s.closed.Load()
}

func (s *circuitStream) Stat() types.StreamStat {
	direction := types.DirOutbound
	if s.circuit.Stat().Direction == pkgif.DirInbound {
		direction = types.DirInbound
	}
	return types.StreamStat{
		Direction: direction,
		Opened:    s.openedAt,
		Protocol:  types.ProtocolID(s.protocol),
	}
}

func (s *circuitStream) State() types.StreamState {
	if s.closed.Load() {
		return types.StreamStateClosed
	}
	return types.StreamStateOpen
}

// ============================================================================
//                              工厂函数
// ============================================================================

// CreateRelayCircuitFromStream 从流创建中继电路
//
// 这是一个便捷方法，自动在流上创建 yamux muxer。
// 用于 STOP 处理器创建入站中继电路。
//
// #
//
// 参数：
//   - stream: STOP 握手完成后的流
//   - isServer: 是否为服务端（接收方）
//   - localPeer: 本地节点 ID
//   - remotePeer: 远端节点 ID
//   - relayPeer: 中继节点 ID
//   - relayTransportAddr: Relay 的传输层地址
func CreateRelayCircuitFromStream(
	stream pkgif.Stream,
	isServer bool,
	localPeer, remotePeer, relayPeer types.PeerID,
	relayTransportAddr types.Multiaddr,
) (*RelayCircuit, error) {
	return CreateRelayCircuitFromStreamWithOptions(stream, isServer, localPeer, remotePeer, relayPeer, relayTransportAddr, nil, nil)
}

// CreateRelayCircuitFromStreamWithOptions 从流创建中继电路（带选项）
//
// #
//
// 参数：
//   - stream: STOP 握手完成后的流
//   - isServer: 是否为服务端（接收方）
//   - localPeer: 本地节点 ID
//   - remotePeer: 远端节点 ID
//   - relayPeer: 中继节点 ID
//   - relayTransportAddr: Relay 的传输层地址（如 /ip4/.../udp/.../quic-v1）
//   - eventbus: 事件总线（可选，用于发射状态变更事件）
//   - onStateChanged: 状态变更回调（可选）
func CreateRelayCircuitFromStreamWithOptions(
	stream pkgif.Stream,
	isServer bool,
	localPeer, remotePeer, relayPeer types.PeerID,
	relayTransportAddr types.Multiaddr,
	eventbus pkgif.EventBus,
	onStateChanged func(oldState, newState CircuitState),
) (*RelayCircuit, error) {
	// 创建流到 net.Conn 的适配器
	netConn := newStreamNetConn(stream)

	// 创建 yamux muxer
	transport := muxer.NewTransport()
	muxedConn, err := transport.NewConn(netConn, isServer, nil)
	if err != nil {
		clientLogger.Error("创建 muxer 失败", "error", err)
		return nil, ErrMuxerCreationFailed
	}

	// 根据 isServer 设置方向
	opts := DefaultCircuitOptions()
	if isServer {
		opts.Direction = pkgif.DirInbound
		opts.Initiator = false
	} else {
		opts.Direction = pkgif.DirOutbound
		opts.Initiator = true
	}
	opts.EventBus = eventbus
	opts.OnStateChanged = onStateChanged

	circuit := NewRelayCircuitWithOptions(stream, muxedConn, localPeer, remotePeer, relayPeer, relayTransportAddr, opts)
	if err := circuit.initControlAsInitiator(); err != nil {
		_ = circuit.Close()
		return nil, err
	}

	return circuit, nil
}
