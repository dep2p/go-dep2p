// Package liveness 提供节点存活检测服务
package liveness

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/config"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	livenessif "github.com/dep2p/go-dep2p/pkg/interfaces/liveness"
	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              协议 ID
// ============================================================================

// 引用 pkg/protocolids 唯一真源
var (
	// ProtocolPing Ping 协议 (v1.1 scope: sys)
	ProtocolPing = protocolids.SysPing

	// ProtocolGoodbye Goodbye 协议 (v1.1 scope: sys)
	ProtocolGoodbye = protocolids.SysGoodbye
)

const (
	// PingPayloadSize Ping 消息大小
	PingPayloadSize = 32
)

// ============================================================================
//                              错误定义
// ============================================================================

// 存活检测服务相关错误
var (
	// ErrServiceClosed 服务已关闭
	ErrServiceClosed = errors.New("liveness service closed")
	ErrPingTimeout   = errors.New("ping timeout")
	ErrPingFailed    = errors.New("ping failed")
	ErrNoConnection  = errors.New("no connection to peer")
)

// ============================================================================
//                              peerState 节点状态
// ============================================================================

// peerState 节点状态
type peerState struct {
	status          types.PeerStatus
	lastSeen        time.Time
	lastPing        time.Time
	lastPingRTT     time.Duration
	avgRTT          time.Duration
	failedPings     int
	healthScore     int
	heartbeatCtx    context.Context
	heartbeatCancel context.CancelFunc
}

// ============================================================================
//                              Service 实现
// ============================================================================

// Service LivenessService 实现
type Service struct {
	config   config.LivenessConfig
	endpoint endpoint.Endpoint

	peers     map[string]*peerState
	callbacks []livenessif.StatusChangeCallback
	decay     livenessif.HealthScoreDecay
	mu        sync.RWMutex
	cbMu      sync.RWMutex // 回调专用锁，避免与 mu 嵌套导致死锁

	// 运行状态
	running int32
	closed  int32
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewService 创建 Liveness 服务
func NewService(cfg config.LivenessConfig, endpoint endpoint.Endpoint) *Service {
	return &Service{
		config:   cfg,
		endpoint: endpoint,
		peers:    make(map[string]*peerState),
		decay:    livenessif.DefaultHealthScoreDecay(),
	}
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动存活检测服务
func (s *Service) Start(_ context.Context) error {
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		return nil
	}

	// 使用 context.Background() 而非 ctx，因为 Fx OnStart 的 ctx 在 OnStart 返回后会被取消
	// 这会导致后台循环 (expiryLoop) 提前退出
	s.ctx, s.cancel = context.WithCancel(context.Background())

	log.Info("存活检测服务启动中")

	// 注册协议处理器
	if s.endpoint != nil {
		s.endpoint.SetProtocolHandler(ProtocolPing, s.handlePingStream)
		s.endpoint.SetProtocolHandler(ProtocolGoodbye, s.handleGoodbyeStream)
	}

	// 启动状态过期清理
	go s.expiryLoop()

	log.Info("存活检测服务已启动")
	return nil
}

// Stop 停止存活检测服务
func (s *Service) Stop() error {
	if !atomic.CompareAndSwapInt32(&s.closed, 0, 1) {
		return nil
	}

	log.Info("存活检测服务停止中")

	// 停止所有心跳
	s.mu.Lock()
	for _, state := range s.peers {
		if state.heartbeatCancel != nil {
			state.heartbeatCancel()
		}
	}
	s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}

	// 移除协议处理器
	if s.endpoint != nil {
		s.endpoint.RemoveProtocolHandler(ProtocolPing)
		s.endpoint.RemoveProtocolHandler(ProtocolGoodbye)
	}

	atomic.StoreInt32(&s.running, 0)
	log.Info("存活检测服务已停止")
	return nil
}

// ============================================================================
//                              Ping 功能
// ============================================================================

// Ping 对指定节点进行 Ping 检测
func (s *Service) Ping(ctx context.Context, nodeID types.NodeID) (time.Duration, error) {
	if atomic.LoadInt32(&s.closed) == 1 {
		return 0, ErrServiceClosed
	}

	if s.endpoint == nil {
		return 0, ErrNoConnection
	}

	// 获取连接
	conn, ok := s.endpoint.Connection(nodeID)
	if !ok {
		var err error
		conn, err = s.endpoint.Connect(ctx, nodeID)
		if err != nil {
			return 0, ErrNoConnection
		}
	}

	// 打开 Ping 流
	stream, err := conn.OpenStream(ctx, ProtocolPing)
	if err != nil {
		s.handlePingFailure(nodeID)
		return 0, ErrPingFailed
	}
	defer func() { _ = stream.Close() }()

	// 生成随机 payload（使用加密安全随机数）
	payload := make([]byte, PingPayloadSize)
	if _, err := crand.Read(payload); err != nil {
		s.handlePingFailure(nodeID)
		return 0, ErrPingFailed
	}

	// 记录开始时间
	start := time.Now()

	// 发送 ping
	if _, err := stream.Write(payload); err != nil {
		s.handlePingFailure(nodeID)
		return 0, ErrPingFailed
	}

	// 读取 pong
	response := make([]byte, PingPayloadSize)
	if _, err := io.ReadFull(stream, response); err != nil {
		s.handlePingFailure(nodeID)
		return 0, ErrPingFailed
	}

	// 计算 RTT
	rtt := time.Since(start)

	// 更新状态
	s.updatePingSuccess(nodeID, rtt)

	log.Debug("Ping 成功",
		"peer", nodeID.ShortString(),
		"rtt", rtt)

	return rtt, nil
}

// handlePingStream 处理 Ping 流
func (s *Service) handlePingStream(stream endpoint.Stream) {
	defer func() { _ = stream.Close() }()

	// 读取 ping payload
	payload := make([]byte, PingPayloadSize)
	if _, err := io.ReadFull(stream, payload); err != nil {
		log.Debug("读取 Ping payload 失败", "err", err)
		return
	}

	// 回复 pong（echo back）
	if _, err := stream.Write(payload); err != nil {
		log.Debug("发送 Pong 响应失败", "err", err)
	}
}

// updatePingSuccess 更新 Ping 成功
func (s *Service) updatePingSuccess(nodeID types.NodeID, rtt time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idStr := nodeID.String()
	state, exists := s.peers[idStr]
	if !exists {
		state = &peerState{
			status:      types.PeerStatusUnknown,
			healthScore: 50,
		}
		s.peers[idStr] = state
	}

	now := time.Now()
	state.lastPing = now
	state.lastPingRTT = rtt
	state.lastSeen = now
	state.failedPings = 0

	// 更新平均 RTT
	if state.avgRTT == 0 {
		state.avgRTT = rtt
	} else {
		state.avgRTT = (state.avgRTT*7 + rtt) / 8 // 指数移动平均
	}

	// 根据 RTT 更新状态
	oldStatus := state.status
	if rtt < s.config.DegradedRTTThreshold {
		state.status = types.PeerStatusOnline
	} else {
		state.status = types.PeerStatusDegraded
	}

	// 恢复健康分
	state.healthScore += s.decay.RecoveryOnPing
	if state.healthScore > 100 {
		state.healthScore = 100
	}

	// 触发状态变更回调
	if oldStatus != state.status {
		s.notifyStatusChange(nodeID, oldStatus, state.status, "ping_success")
	}
}

// handlePingFailure 处理 Ping 失败
func (s *Service) handlePingFailure(nodeID types.NodeID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idStr := nodeID.String()
	state, exists := s.peers[idStr]
	if !exists {
		state = &peerState{
			status:      types.PeerStatusUnknown,
			healthScore: 50,
		}
		s.peers[idStr] = state
	}

	state.failedPings++
	oldStatus := state.status

	// 连续 3 次失败判定为离线
	if state.failedPings >= 3 {
		state.status = types.PeerStatusOffline
	} else if state.failedPings >= 1 {
		state.status = types.PeerStatusDegraded
	}

	// 衰减健康分
	state.healthScore -= s.decay.DecayAmount
	if state.healthScore < s.decay.MinScore {
		state.healthScore = s.decay.MinScore
	}

	if oldStatus != state.status {
		s.notifyStatusChange(nodeID, oldStatus, state.status, "ping_failure")
	}
}

// ============================================================================
//                              Goodbye 功能
// ============================================================================

// SendGoodbye 发送 Goodbye 消息
func (s *Service) SendGoodbye(ctx context.Context, reason types.GoodbyeReason) error {
	if atomic.LoadInt32(&s.closed) == 1 {
		return ErrServiceClosed
	}

	if s.endpoint == nil {
		return ErrNoConnection
	}

	log.Info("发送 Goodbye 到所有节点",
		"reason", reason.String())

	// 向所有已连接的节点发送 Goodbye
	for _, conn := range s.endpoint.Connections() {
		go func(c endpoint.Connection) {
			_ = s.SendGoodbyeTo(ctx, c.RemoteID(), reason) // 关闭时发送失败可忽略
		}(conn)
	}

	return nil
}

// SendGoodbyeTo 向指定节点发送 Goodbye 消息
func (s *Service) SendGoodbyeTo(ctx context.Context, nodeID types.NodeID, reason types.GoodbyeReason) error {
	if atomic.LoadInt32(&s.closed) == 1 {
		return ErrServiceClosed
	}

	if s.endpoint == nil {
		return ErrNoConnection
	}

	// 获取连接
	conn, ok := s.endpoint.Connection(nodeID)
	if !ok {
		return ErrNoConnection
	}

	// 打开 Goodbye 流
	stream, err := conn.OpenStream(ctx, ProtocolGoodbye)
	if err != nil {
		return err
	}
	defer func() { _ = stream.Close() }()

	// 发送 reason (长度 + 内容)
	reasonBytes := []byte(string(reason))
	if err := binary.Write(stream, binary.BigEndian, uint16(len(reasonBytes))); err != nil {
		return err
	}
	if _, err := stream.Write(reasonBytes); err != nil {
		return err
	}

	log.Debug("发送 Goodbye",
		"peer", nodeID.ShortString(),
		"reason", reason.String())

	// 更新状态
	s.mu.Lock()
	if state, ok := s.peers[nodeID.String()]; ok {
		oldStatus := state.status
		state.status = types.PeerStatusOffline
		if oldStatus != state.status {
			s.notifyStatusChange(nodeID, oldStatus, state.status, "goodbye_sent")
		}
	}
	s.mu.Unlock()

	return nil
}

// handleGoodbyeStream 处理 Goodbye 流
func (s *Service) handleGoodbyeStream(stream endpoint.Stream) {
	defer func() { _ = stream.Close() }()

	conn := stream.Connection()
	if conn == nil {
		return
	}

	nodeID := conn.RemoteID()

	// 读取 reason（格式：长度 + 内容，与 SendGoodbyeTo 一致）
	var reasonLen uint16
	if err := binary.Read(stream, binary.BigEndian, &reasonLen); err != nil {
		log.Debug("读取 Goodbye reason 长度失败", "err", err)
		return
	}

	// 防止过大的 reason 导致内存问题
	if reasonLen > 256 {
		log.Warn("Goodbye reason 长度过大", "len", reasonLen)
		return
	}

	reasonBytes := make([]byte, reasonLen)
	if _, err := io.ReadFull(stream, reasonBytes); err != nil {
		log.Debug("读取 Goodbye reason 内容失败", "err", err)
		return
	}

	reason := types.GoodbyeReason(reasonBytes)
	log.Info("收到 Goodbye",
		"peer", nodeID.ShortString(),
		"reason", reason.String())

	// 更新状态
	s.mu.Lock()
	idStr := nodeID.String()
	state, exists := s.peers[idStr]
	if !exists {
		state = &peerState{
			status:      types.PeerStatusOffline,
			healthScore: 0,
		}
		s.peers[idStr] = state
	}
	oldStatus := state.status
	state.status = types.PeerStatusOffline
	state.lastSeen = time.Now()
	s.mu.Unlock()

	if oldStatus != types.PeerStatusOffline {
		s.notifyStatusChange(nodeID, oldStatus, types.PeerStatusOffline, "goodbye_received")
	}
}

// ============================================================================
//                              心跳功能
// ============================================================================

// StartHeartbeat 开始对指定节点的心跳检测
func (s *Service) StartHeartbeat(nodeID types.NodeID) {
	// 检查服务是否已启动
	if atomic.LoadInt32(&s.running) == 0 {
		log.Warn("服务未启动，无法启动心跳",
			"peer", nodeID.ShortString())
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// 再次检查 s.ctx（防止竞态）
	if s.ctx == nil {
		log.Warn("服务上下文为空，无法启动心跳",
			"peer", nodeID.ShortString())
		return
	}

	idStr := nodeID.String()
	state, exists := s.peers[idStr]
	if !exists {
		state = &peerState{
			status:      types.PeerStatusUnknown,
			healthScore: 50,
		}
		s.peers[idStr] = state
	}

	// 如果已有心跳，先停止
	if state.heartbeatCancel != nil {
		state.heartbeatCancel()
	}

	// 启动新的心跳
	state.heartbeatCtx, state.heartbeatCancel = context.WithCancel(s.ctx)
	go s.heartbeatLoop(nodeID, state)

	log.Debug("启动心跳",
		"peer", nodeID.ShortString())
}

// StopHeartbeat 停止对指定节点的心跳检测
func (s *Service) StopHeartbeat(nodeID types.NodeID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idStr := nodeID.String()
	state, exists := s.peers[idStr]
	if !exists {
		return
	}

	if state.heartbeatCancel != nil {
		state.heartbeatCancel()
		state.heartbeatCancel = nil
	}

	log.Debug("停止心跳",
		"peer", nodeID.ShortString())
}

// heartbeatLoop 心跳循环
func (s *Service) heartbeatLoop(nodeID types.NodeID, state *peerState) {
	ticker := time.NewTicker(s.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-state.heartbeatCtx.Done():
			return
		case <-ticker.C:
			// 检查服务是否仍在运行
			if atomic.LoadInt32(&s.closed) == 1 {
				return
			}

			// 再次检查上下文是否已取消（避免竞态）
			select {
			case <-state.heartbeatCtx.Done():
				return
			default:
			}

			ctx, cancel := context.WithTimeout(state.heartbeatCtx, s.config.HeartbeatTimeout)
			_, err := s.Ping(ctx, nodeID)
			cancel()

			if err != nil {
				log.Debug("心跳 Ping 失败",
					"peer", nodeID.ShortString(),
					"err", err)
			}
		}
	}
}

// ============================================================================
//                              状态查询
// ============================================================================

// PeerStatus 获取节点当前状态
func (s *Service) PeerStatus(nodeID types.NodeID) types.PeerStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, exists := s.peers[nodeID.String()]
	if !exists {
		return types.PeerStatusUnknown
	}
	return state.status
}

// PeerHealth 获取节点健康信息
func (s *Service) PeerHealth(nodeID types.NodeID) *types.PeerHealth {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, exists := s.peers[nodeID.String()]
	if !exists {
		return nil
	}

	return &types.PeerHealth{
		NodeID:      nodeID,
		Status:      state.status,
		LastSeen:    state.lastSeen,
		LastPing:    state.lastPing,
		LastPingRTT: state.lastPingRTT,
		AvgRTT:      state.avgRTT,
		FailedPings: state.failedPings,
		HealthScore: state.healthScore,
	}
}

// AllPeerStatuses 获取所有已知节点的状态
func (s *Service) AllPeerStatuses() map[types.NodeID]types.PeerStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	statuses := make(map[types.NodeID]types.PeerStatus, len(s.peers))
	for idStr, state := range s.peers {
		nodeID, err := types.ParseNodeID(idStr)
		if err != nil {
			continue
		}
		statuses[nodeID] = state.status
	}
	return statuses
}

// OnlinePeers 获取所有在线节点
func (s *Service) OnlinePeers() []types.NodeID {
	s.mu.RLock()
	defer s.mu.RUnlock()

	peers := make([]types.NodeID, 0)
	for idStr, state := range s.peers {
		if state.status == types.PeerStatusOnline {
			nodeID, err := types.ParseNodeID(idStr)
			if err != nil {
				continue
			}
			peers = append(peers, nodeID)
		}
	}
	return peers
}

// HealthScore 获取节点健康评分
func (s *Service) HealthScore(nodeID types.NodeID) int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, exists := s.peers[nodeID.String()]
	if !exists {
		return 0
	}
	return state.healthScore
}

// ============================================================================
//                              回调管理
// ============================================================================

// OnStatusChange 注册状态变更回调
func (s *Service) OnStatusChange(callback livenessif.StatusChangeCallback) {
	s.cbMu.Lock()
	defer s.cbMu.Unlock()
	s.callbacks = append(s.callbacks, callback)
}

// RemoveStatusChangeCallback 移除状态变更回调（通过索引移除）
func (s *Service) RemoveStatusChangeCallback(_ livenessif.StatusChangeCallback) {
	// Go 中无法直接比较函数，需要通过其他机制（如返回取消函数）来实现
	// 当前保留为空实现，建议使用 OnStatusChange 返回取消函数的方式
}

// notifyStatusChange 通知状态变更
func (s *Service) notifyStatusChange(nodeID types.NodeID, oldStatus, newStatus types.PeerStatus, reason string) {
	event := types.PeerStatusChangeEvent{
		NodeID:    nodeID,
		OldStatus: oldStatus,
		NewStatus: newStatus,
		Reason:    reason,
		Timestamp: time.Now(),
	}

	// 复制回调列表，避免在回调执行期间持有锁
	s.cbMu.RLock()
	callbacks := make([]livenessif.StatusChangeCallback, len(s.callbacks))
	copy(callbacks, s.callbacks)
	s.cbMu.RUnlock()

	for _, cb := range callbacks {
		go cb(event)
	}
}

// ============================================================================
//                              配置
// ============================================================================

// SetHealthScoreDecay 设置健康评分衰减规则
func (s *Service) SetHealthScoreDecay(decay livenessif.HealthScoreDecay) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.decay = decay
}

// SetThresholds 设置检测阈值
func (s *Service) SetThresholds(thresholds types.LivenessThresholds) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config.HeartbeatInterval = thresholds.HeartbeatInterval
	s.config.HeartbeatTimeout = thresholds.HeartbeatTimeout
	s.config.DegradedRTTThreshold = thresholds.DegradedRTT
	s.config.StatusExpiry = thresholds.StatusExpiry
}

// Thresholds 获取当前阈值
func (s *Service) Thresholds() types.LivenessThresholds {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return types.LivenessThresholds{
		DegradedRTT:       s.config.DegradedRTTThreshold,
		HeartbeatInterval: s.config.HeartbeatInterval,
		HeartbeatTimeout:  s.config.HeartbeatTimeout,
		StatusExpiry:      s.config.StatusExpiry,
	}
}

// ============================================================================
//                              清理
// ============================================================================

// expiryLoop 状态过期清理循环
func (s *Service) expiryLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cleanupExpiredStates()
		}
	}
}

// cleanupExpiredStates 清理过期状态
func (s *Service) cleanupExpiredStates() {
	// 收集待通知的过期节点
	type expiredPeer struct {
		nodeID    types.NodeID
		oldStatus types.PeerStatus
	}
	var expiredPeers []expiredPeer

	s.mu.Lock()
	now := time.Now()
	for idStr, state := range s.peers {
		// 离线状态超过过期时间，清理
		if state.status == types.PeerStatusOffline {
			if now.Sub(state.lastSeen) > s.config.StatusExpiry {
				nodeID, err := types.ParseNodeID(idStr)
				if err == nil {
					expiredPeers = append(expiredPeers, expiredPeer{
						nodeID:    nodeID,
						oldStatus: state.status,
					})
				}
				delete(s.peers, idStr)
			}
		}
	}
	s.mu.Unlock()

	// 释放锁后发送通知
	for _, ep := range expiredPeers {
		s.notifyStatusChange(ep.nodeID, ep.oldStatus, types.PeerStatusUnknown, "expired")
	}
}
