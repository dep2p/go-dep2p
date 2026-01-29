// Package server 实现中继服务端
package server

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/protocol"
	"github.com/dep2p/go-dep2p/pkg/types"
	mss "github.com/multiformats/go-multistream"
)

var serverLogger = log.Logger("relay/server")

var (
	// ErrMessageTooLarge 消息过大
	ErrMessageTooLarge = errors.New("relay: message too large")
	// ErrEmptyStatusData 空状态数据
	ErrEmptyStatusData = errors.New("relay: empty status data")
)

// HOP 协议 ID（使用统一定义）
var (
	HopProtocolID  = string(protocol.RelayHop)
	StopProtocolID = string(protocol.RelayStop)
)

// 消息类型
const (
	MsgTypeReserve = 0
	MsgTypeConnect = 1
	MsgTypeStatus  = 2
)

// 状态码
const (
	StatusOK                    = 0
	StatusPermissionDenied      = 1
	StatusNoReservation         = 2
	StatusResourceLimitExceeded = 3
	StatusMalformedMessage      = 4
	StatusUnexpectedMessage     = 5
	// 细分状态码
	StatusTargetUnreachable = 6 // 目标节点不可达
	StatusProtocolError     = 7 // 协议协商失败
	StatusInternalError     = 8 // 内部错误
)

const (
	// MaxMessageSize 最大消息大小 (64KB)，防止内存攻击
	MaxMessageSize = 64 * 1024
)

// Server 中继服务端
type Server struct {
	swarm   pkgif.Swarm
	host    pkgif.Host
	limiter Limiter

	// 预约管理
	reservations map[types.PeerID]time.Time
	circuits     map[types.PeerID]int

	// ACL
	acl ACL

	// 事件订阅
	disconnSub pkgif.Subscription

	mu     sync.RWMutex
	closed bool

	ctx    context.Context
	cancel context.CancelFunc
}

// Limiter 资源限制器接口
type Limiter interface {
	// CanReserve 检查是否可以预约
	CanReserve(peer types.PeerID) bool
	// CanConnect 检查是否可以连接
	CanConnect(src, dst types.PeerID) bool
	// ReserveFor 预约时长
	ReserveFor() time.Duration
	// MaxCircuitsPerPeer 每节点最大电路数
	MaxCircuitsPerPeer() int
}

// ACL 访问控制列表
type ACL interface {
	// AllowReserve 是否允许预约
	AllowReserve(peer types.PeerID) bool
	// AllowConnect 是否允许连接
	AllowConnect(src, dst types.PeerID) bool
}

// NewServer 创建中继服务端
func NewServer(swarm pkgif.Swarm, limiter Limiter) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		swarm:        swarm,
		limiter:      limiter,
		reservations: make(map[types.PeerID]time.Time),
		circuits:     make(map[types.PeerID]int),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// SetHost 设置 Host（用于注册协议处理器）
//
// 必须在 Start() 之前调用。
func (s *Server) SetHost(host pkgif.Host) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.host = host
}

// Start 启动服务端
func (s *Server) Start() error {
	s.mu.RLock()
	host := s.host
	s.mu.RUnlock()

	// 注册 HOP 协议处理器
	if host != nil {
		host.SetStreamHandler(HopProtocolID, s.HandleHop)
		host.SetStreamHandler(StopProtocolID, s.HandleStop)
		serverLogger.Info("Relay 协议处理器已注册",
			"hopProtocol", HopProtocolID,
			"stopProtocol", StopProtocolID)

		// 订阅连接断开事件，用于清除断开节点的预约
		if eventBus := host.EventBus(); eventBus != nil {
			sub, err := eventBus.Subscribe(new(types.EvtPeerDisconnected))
			if err != nil {
				serverLogger.Warn("订阅断开事件失败", "error", err)
			} else {
				s.mu.Lock()
				s.disconnSub = sub
				s.mu.Unlock()
				go s.handlePeerDisconnected(sub)
				serverLogger.Debug("已订阅 EvtPeerDisconnected 事件")
			}
		}
	} else {
		serverLogger.Error("host 为 nil，Relay 协议处理器未注册，客户端预留将失败")
	}

	// 启动后台 GC
	go s.gc()

	return nil
}

// HandleStop 处理 STOP 协议流
//
// STOP 协议用于目标节点接受中继连接。
// 当中继服务器转发连接请求时，目标节点通过此处理器接收。
//
// Phase 11 修复：实现完整的 STOP 协议验证逻辑
func (s *Server) HandleStop(stream pkgif.Stream) {
	defer stream.Close()

	// 获取本地节点 ID（目标节点）
	conn := stream.Conn()
	if conn == nil {
		s.writeStatus(stream, StatusMalformedMessage)
		return
	}
	localPeer := types.PeerID(conn.LocalPeer())

	// 读取 CONNECT 消息（包含源节点 ID）
	msgType, data, err := s.readMessage(stream)
	if err != nil || msgType != MsgTypeConnect {
		s.writeStatus(stream, StatusMalformedMessage)
		return
	}

	srcPeer := types.PeerID(data)

	// 1. 验证源节点 ID 有效性
	if srcPeer == "" {
		s.writeStatus(stream, StatusMalformedMessage)
		return
	}

	// 2. ACL 检查：本节点是否允许来自 srcPeer 的连接
	if s.acl != nil && !s.acl.AllowConnect(srcPeer, localPeer) {
		s.writeStatus(stream, StatusPermissionDenied)
		return
	}

	// 3. Limiter 检查：资源限制
	if s.limiter != nil && !s.limiter.CanConnect(srcPeer, localPeer) {
		s.writeStatus(stream, StatusResourceLimitExceeded)
		return
	}

	// 4. 增加电路计数（目标侧，maxCircuitsPerPeer <= 0 表示不限制）
	s.mu.Lock()
	maxCircuitsPerPeer := 16
	if s.limiter != nil {
		maxCircuitsPerPeer = s.limiter.MaxCircuitsPerPeer()
	}
	if maxCircuitsPerPeer > 0 && s.circuits[localPeer] >= maxCircuitsPerPeer {
		s.mu.Unlock()
		s.writeStatus(stream, StatusResourceLimitExceeded)
		return
	}
	s.circuits[localPeer]++
	s.mu.Unlock()

	// 5. 返回成功，准备接收中继数据
	s.writeStatus(stream, StatusOK)

	// 注意：此时流保持打开，后续的数据由中继服务器双向转发
	// 流的关闭由 defer 处理，或者由中继服务器在 relay() 结束后关闭
}

// Stop 停止服务端
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	// 关闭事件订阅
	if s.disconnSub != nil {
		s.disconnSub.Close()
	}

	s.cancel()
	return nil
}

// handlePeerDisconnected 处理节点断开事件
//
// 当与已预约节点的连接断开时，清除该节点的预约。
// 这确保了预约状态与连接状态的一致性，避免出现"预约有效但无法连接"的问题。
func (s *Server) handlePeerDisconnected(sub pkgif.Subscription) {
	defer sub.Close()
	for {
		select {
		case <-s.ctx.Done():
			return
		case evt := <-sub.Out():
			disconnected, ok := evt.(*types.EvtPeerDisconnected)
			if !ok {
				continue
			}

			peer := disconnected.PeerID
			peerShort := string(peer)
			if len(peerShort) > 8 {
				peerShort = peerShort[:8]
			}

			s.mu.Lock()
			if _, exists := s.reservations[peer]; exists {
				delete(s.reservations, peer)
				serverLogger.Info("连接断开，清除预约",
					"peer", peerShort,
					"reason", disconnected.Reason)
			}
			// 同时清理该节点的电路计数
			delete(s.circuits, peer)
			s.mu.Unlock()
		}
	}
}

// ServerStats 服务端统计信息
//
// Phase 9 修复：添加统计信息结构
type ServerStats struct {
	// ActiveReservations 当前活跃预约数
	ActiveReservations int
	// TotalCircuits 当前活跃电路总数
	TotalCircuits int
	// UniqueRelayedPeers 正在中继的唯一节点数
	UniqueRelayedPeers int
}

// Stats 获取服务端统计信息
//
// Phase 9 修复：实现真实统计数据获取
func (s *Server) Stats() ServerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := ServerStats{
		ActiveReservations: len(s.reservations),
		UniqueRelayedPeers: len(s.circuits),
	}

	// 计算总电路数
	for _, count := range s.circuits {
		stats.TotalCircuits += count
	}

	return stats
}

// HandleHop 处理 HOP 协议流
func (s *Server) HandleHop(stream pkgif.Stream) {
	defer stream.Close()

	// 获取远程节点
	conn := stream.Conn()
	if conn == nil {
		serverLogger.Warn("HandleHop 收到无效流", "reason", "conn=nil")
		return
	}
	peer := conn.RemotePeer()
	peerShort := string(peer)
	if len(peerShort) > 8 {
		peerShort = peerShort[:8]
	}

	serverLogger.Info("收到 HOP 协议请求", "peer", peerShort)

	// 读取消息
	msgType, data, err := s.readMessage(stream)
	if err != nil {
		serverLogger.Warn("HOP 协议读取消息失败", "peer", peerShort, "error", err)
		s.writeStatus(stream, StatusMalformedMessage)
		return
	}

	switch msgType {
	case MsgTypeReserve:
		serverLogger.Info("处理 RESERVE 请求", "peer", peerShort)
		s.handleReserve(stream, peer)
	case MsgTypeConnect:
		targetShort := string(data)
		if len(targetShort) > 8 {
			targetShort = targetShort[:8]
		}
		serverLogger.Info("HandleHop: 收到 CONNECT 请求", "peer", peerShort, "target", targetShort)
		s.handleConnect(stream, peer, data)
	default:
		serverLogger.Warn("HOP 协议收到未知消息类型", "peer", peerShort, "msgType", msgType)
		s.writeStatus(stream, StatusUnexpectedMessage)
	}
}

// handleReserve 处理预约请求
func (s *Server) handleReserve(stream pkgif.Stream, peer types.PeerID) {
	s.mu.Lock()
	defer s.mu.Unlock()

	peerShort := string(peer)
	if len(peerShort) > 8 {
		peerShort = peerShort[:8]
	}

	// 1. ACL 检查
	if s.acl != nil && !s.acl.AllowReserve(peer) {
		serverLogger.Debug("RESERVE 被 ACL 拒绝", "peer", peerShort)
		s.writeStatus(stream, StatusPermissionDenied)
		return
	}

	// 2. 资源限制检查
	if s.limiter != nil && !s.limiter.CanReserve(peer) {
		serverLogger.Debug("RESERVE 被 Limiter 拒绝（资源限制）", "peer", peerShort)
		s.writeStatus(stream, StatusResourceLimitExceeded)
		return
	}

	// 3. 记录预约
	duration := time.Hour // 默认 1 小时
	if s.limiter != nil {
		duration = s.limiter.ReserveFor()
	}
	s.reservations[peer] = time.Now().Add(duration)

	// 4. 返回成功
	serverLogger.Info("RESERVE 成功", "peer", peerShort, "duration", duration, "totalReservations", len(s.reservations))
	s.writeStatus(stream, StatusOK)
}

// handleConnect 处理连接请求
func (s *Server) handleConnect(stream pkgif.Stream, src types.PeerID, data []byte) {
	srcShort := string(src)
	if len(srcShort) > 8 {
		srcShort = srcShort[:8]
	}

	// 解析目标节点
	target := types.PeerID(data)
	if target == "" {
		serverLogger.Warn("CONNECT 失败: 目标为空", "src", srcShort)
		s.writeStatus(stream, StatusMalformedMessage)
		return
	}

	targetShort := string(target)
	if len(targetShort) > 8 {
		targetShort = targetShort[:8]
	}

	serverLogger.Info("处理 CONNECT 请求", "src", srcShort, "target", targetShort)

	s.mu.RLock()
	// ★
	//
	// 根据 Circuit Relay v2 协议：
	// - 被动方（被连接的节点）需要在 Relay 服务器上预约（RESERVE）
	// - 主动方（发起连接的节点）只需要连接到 Relay 服务器并发送 CONNECT 请求
	// - Relay 服务器只需检查目标节点是否有预约
	//
	// 之前错误地检查了源节点是否有预约，导致主动方无法连接到被动方。
	dstExpire, dstOK := s.reservations[target]
	s.mu.RUnlock()

	if !dstOK || time.Now().After(dstExpire) {
		serverLogger.Warn("CONNECT 失败: 目标节点无预约或已过期",
			"src", srcShort, "target", targetShort,
			"dstOK", dstOK, "dstExpired", dstOK && time.Now().After(dstExpire))
		s.writeStatus(stream, StatusNoReservation)
		return
	}

	serverLogger.Debug("目标节点预约有效，继续处理 CONNECT",
		"src", srcShort, "target", targetShort)

	// 3. ACL 检查
	if s.acl != nil && !s.acl.AllowConnect(src, target) {
		serverLogger.Warn("CONNECT 失败: ACL 拒绝", "src", srcShort, "target", targetShort)
		s.writeStatus(stream, StatusPermissionDenied)
		return
	}

	// 4. 并发数检查（maxCircuitsPerPeer <= 0 表示不限制）
	s.mu.Lock()
	maxCircuitsPerPeer := 16
	if s.limiter != nil {
		maxCircuitsPerPeer = s.limiter.MaxCircuitsPerPeer()
	}
	srcCircuits := s.circuits[src]
	targetCircuits := s.circuits[target]
	if maxCircuitsPerPeer > 0 && (srcCircuits >= maxCircuitsPerPeer || targetCircuits >= maxCircuitsPerPeer) {
		s.mu.Unlock()
		serverLogger.Warn("CONNECT 失败: 电路数超限",
			"src", srcShort, "target", targetShort,
			"srcCircuits", srcCircuits, "targetCircuits", targetCircuits,
			"maxCircuitsPerPeer", maxCircuitsPerPeer)
		s.writeStatus(stream, StatusResourceLimitExceeded)
		return
	}
	s.circuits[src]++
	s.circuits[target]++
	s.mu.Unlock()

	// 5. 打开到目标的 STOP 流
	// 优先使用已有连接：目标节点（NAT 后）在 RESERVE 时已建立连接
	// 如果尝试 DialPeer 新建连接，会因为 NAT 导致连接失败
	var dstConn pkgif.Connection
	existingConns := s.swarm.ConnsToPeer(string(target))
	if len(existingConns) > 0 {
		dstConn = existingConns[0]
		serverLogger.Debug("使用已有连接到目标节点",
			"target", string(target)[:8],
			"existingConns", len(existingConns))
	} else {
		// 回退到 DialPeer（对公网节点可能成功）
		serverLogger.Debug("无已有连接，尝试拨号到目标节点",
			"target", string(target)[:8])
		var err error
		dstConn, err = s.swarm.DialPeer(s.ctx, string(target))
		if err != nil {
			serverLogger.Warn("无法连接到目标节点",
				"target", string(target)[:8],
				"error", err)
			s.decrementCircuits(src, target)
			s.writeStatus(stream, StatusTargetUnreachable)
			return
		}
	}

	dstStream, err := dstConn.NewStream(s.ctx)
	if err != nil {
		serverLogger.Warn("无法打开到目标的新流",
			"target", string(target)[:8],
			"error", err)
		s.decrementCircuits(src, target)
		s.writeStatus(stream, StatusInternalError)
		return
	}

	// 进行 STOP 协议协商
	selectedProto, err := mss.SelectOneOf([]string{StopProtocolID}, dstStream)
	if err != nil {
		serverLogger.Warn("STOP 协议协商失败",
			"target", string(target)[:8],
			"error", err)
		dstStream.Close()
		s.decrementCircuits(src, target)
		s.writeStatus(stream, StatusProtocolError)
		return
	}
	dstStream.SetProtocol(selectedProto)

	// 6. 发送 STOP CONNECT 消息到目标
	if err := s.writeStopConnect(dstStream, src); err != nil {
		serverLogger.Warn("发送 STOP CONNECT 失败",
			"target", string(target)[:8],
			"error", err)
		dstStream.Close()
		s.decrementCircuits(src, target)
		s.writeStatus(stream, StatusInternalError)
		return
	}

	// 7. 读取目标响应
	msgType, statusData, err := s.readMessage(dstStream)
	if err != nil {
		serverLogger.Warn("读取目标响应失败", "target", string(target)[:8], "error", err)
		dstStream.Close()
		s.decrementCircuits(src, target)
		s.writeStatus(stream, StatusInternalError)
		return
	}
	if msgType != MsgTypeStatus {
		serverLogger.Warn("目标响应消息类型异常", "target", string(target)[:8], "msgType", msgType)
		dstStream.Close()
		s.decrementCircuits(src, target)
		s.writeStatus(stream, StatusProtocolError)
		return
	}
	targetStatus := decodeStatus(statusData)
	if targetStatus != StatusOK {
		serverLogger.Warn("目标拒绝连接", "target", string(target)[:8], "status", targetStatus)
		dstStream.Close()
		s.decrementCircuits(src, target)
		// 透传目标的状态码
		s.writeStatus(stream, targetStatus)
		return
	}

	// 8. 返回成功给源节点
	serverLogger.Info("CONNECT 成功", "src", srcShort, "target", targetShort)
	s.writeStatus(stream, StatusOK)

	// 9. 双向转发
	s.relay(stream, dstStream, src, target)
}

// relay 双向转发数据
func (s *Server) relay(src, dst pkgif.Stream, srcPeer, dstPeer types.PeerID) {
	defer src.Close()
	defer dst.Close()
	defer s.decrementCircuits(srcPeer, dstPeer)

	done := make(chan struct{}, 2)

	// src -> dst
	go func() {
		io.Copy(dst, src)
		done <- struct{}{}
	}()

	// dst -> src
	go func() {
		io.Copy(src, dst)
		done <- struct{}{}
	}()

	// 等待一方完成
	<-done
}

// decrementCircuits 减少电路计数
func (s *Server) decrementCircuits(src, dst types.PeerID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.circuits[src] > 0 {
		s.circuits[src]--
	}
	if s.circuits[dst] > 0 {
		s.circuits[dst]--
	}
}

// gc 后台清理过期预约
func (s *Server) gc() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.cleanExpiredReservations()
		}
	}
}

// cleanExpiredReservations 清理过期预约
func (s *Server) cleanExpiredReservations() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	for peer, expire := range s.reservations {
		if now.After(expire) {
			delete(s.reservations, peer)
		}
	}
}

// writeStatus 写入状态响应
func (s *Server) writeStatus(w io.Writer, status int) error {
	return s.writeMessage(w, MsgTypeStatus, []byte{byte(status)})
}

// writeStopConnect 写入 STOP CONNECT 消息
func (s *Server) writeStopConnect(w io.Writer, src types.PeerID) error {
	return s.writeMessage(w, MsgTypeConnect, []byte(src))
}

// writeMessage 写入消息
func (s *Server) writeMessage(w io.Writer, msgType byte, data []byte) error {
	header := make([]byte, 5)
	header[0] = msgType
	binary.BigEndian.PutUint32(header[1:], uint32(len(data)))

	if _, err := w.Write(header); err != nil {
		return err
	}

	if len(data) > 0 {
		if _, err := w.Write(data); err != nil {
			return err
		}
	}

	return nil
}

// readMessage 读取消息
func (s *Server) readMessage(r io.Reader) (byte, []byte, error) {
	header := make([]byte, 5)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, nil, err
	}

	msgType := header[0]
	length := binary.BigEndian.Uint32(header[1:])

	// 检查消息大小，防止内存攻击
	if length > MaxMessageSize {
		return 0, nil, fmt.Errorf("%w: %d > %d", ErrMessageTooLarge, length, MaxMessageSize)
	}

	if length > 0 {
		data := make([]byte, length)
		if _, err := io.ReadFull(r, data); err != nil {
			return 0, nil, err
		}
		return msgType, data, nil
	}

	return msgType, nil, nil
}

// decodeStatus 解码状态
// 空数据返回错误状态而不是 OK
func decodeStatus(data []byte) int {
	if len(data) == 0 {
		// 空数据应返回错误状态而不是 OK
		return StatusMalformedMessage
	}
	return int(data[0])
}
