// Package server 提供中继服务器实现
//
// 中继服务器帮助 NAT 后的节点互相连接，主要功能：
// - 资源预留管理
// - 连接请求处理
// - 流量转发
// - 资源限制和配额
package server

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	realmif "github.com/dep2p/go-dep2p/pkg/interfaces/realm"
	relayif "github.com/dep2p/go-dep2p/pkg/interfaces/relay"
	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

var log = logger.Logger("relay.server")

// ============================================================================
//                              常量
// ============================================================================

// 引用 pkg/protocolids 唯一真源
var (
	// ProtocolRelay 中继协议 (v1.1 scope: sys)
	ProtocolRelay = protocolids.SysRelay
	// ProtocolRelayHop 中继跳转协议 (v1.1 scope: sys)
	ProtocolRelayHop = protocolids.SysRelayHop
)

const (
	// MsgTypeReserve 预留请求消息类型
	MsgTypeReserve uint8 = 1
	// MsgTypeReserveOK 预留成功响应
	MsgTypeReserveOK uint8 = 2
	// MsgTypeReserveError 预留失败响应
	MsgTypeReserveError uint8 = 3
	// MsgTypeConnect 连接请求消息类型
	MsgTypeConnect uint8 = 4
	// MsgTypeConnectOK 连接成功响应
	MsgTypeConnectOK uint8 = 5
	// MsgTypeConnectError 连接失败响应
	MsgTypeConnectError uint8 = 6
)

// ============================================================================
//                              Server 实现
// ============================================================================

// Server 中继服务器
//
// IMPL-1227: 支持两种模式：
//   - System Relay: realmID 为空，只允许系统协议，无成员验证
//   - Realm Relay: realmID 非空，只允许本 Realm 协议，需要 PSK 成员验证
type Server struct {
	config   relayif.ServerConfig
	localID  types.NodeID
	endpoint endpoint.Endpoint

	// IMPL-1227: Realm 验证（新增）
	realmID types.RealmID              // Realm ID（空表示 System Relay）
	pskAuth realmif.PSKAuthenticator   // PSK 认证器（Realm Relay 模式）

	// 预留管理
	reservations   map[types.NodeID]*Reservation
	reservationsMu sync.RWMutex

	// 活跃电路
	circuits   map[string]*Circuit
	circuitsMu sync.RWMutex

	// 统计
	stats   Stats
	statsMu sync.RWMutex

	// 资源限制
	limiter *RateLimiter

	// 状态
	running int32
	closed  int32
	ctx     context.Context
	cancel  context.CancelFunc
}

// Reservation 预留信息
type Reservation struct {
	// PeerID 预留节点
	PeerID types.NodeID

	// Expiry 过期时间
	Expiry time.Time

	// Slots 分配的槽位
	Slots int

	// UsedSlots 已使用槽位
	UsedSlots int

	// CreatedAt 创建时间
	CreatedAt time.Time

	// Addrs 中继地址
	Addrs []string
}

// IsExpired 检查是否过期
func (r *Reservation) IsExpired() bool {
	return time.Now().After(r.Expiry)
}

// Circuit 活跃电路
type Circuit struct {
	// ID 电路 ID
	ID string

	// Src 源节点
	Src types.NodeID

	// Dest 目标节点
	Dest types.NodeID

	// SrcStream 源流
	SrcStream endpoint.Stream

	// DestStream 目标流
	DestStream endpoint.Stream

	// CreatedAt 创建时间
	CreatedAt time.Time

	// BytesTransferred 传输字节数
	BytesTransferred int64

	// Deadline 截止时间
	Deadline time.Time

	// closed 是否关闭
	closed int32
}

// Stats 统计信息
type Stats struct {
	// TotalReservations 总预留数
	TotalReservations int64

	// ActiveReservations 活跃预留数
	ActiveReservations int64

	// TotalCircuits 总电路数
	TotalCircuits int64

	// ActiveCircuits 活跃电路数
	ActiveCircuits int64

	// BytesRelayed 中继字节数
	BytesRelayed int64

	// ConnectionsAccepted 接受的连接数
	ConnectionsAccepted int64

	// ConnectionsRejected 拒绝的连接数
	ConnectionsRejected int64
}

// NewServer 创建中继服务器
func NewServer(config relayif.ServerConfig, localID types.NodeID, endpoint endpoint.Endpoint) *Server {
	return &Server{
		config:       config,
		localID:      localID,
		endpoint:     endpoint,
		reservations: make(map[types.NodeID]*Reservation),
		circuits:     make(map[string]*Circuit),
		limiter:      NewRateLimiter(config.MaxDataRate),
	}
}

// SetRealmID 设置 Realm ID（IMPL-1227）
//
// 设置后，Server 进入 Realm Relay 模式：
//   - 只允许本 Realm 的应用协议和控制协议
//   - 需要配合 SetPSKAuthenticator 使用以启用成员验证
func (s *Server) SetRealmID(realmID types.RealmID) {
	s.realmID = realmID
}

// SetPSKAuthenticator 设置 PSK 认证器（IMPL-1227）
//
// 设置后，连接请求需要通过 PSK 成员验证。
// 只有持有相同 RealmKey 的节点才能使用此 Relay。
func (s *Server) SetPSKAuthenticator(auth realmif.PSKAuthenticator) {
	s.pskAuth = auth
}

// RealmID 返回当前 Realm ID
func (s *Server) RealmID() types.RealmID {
	return s.realmID
}

// IsRealmRelay 检查是否为 Realm Relay 模式
func (s *Server) IsRealmRelay() bool {
	return s.realmID != ""
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动服务器
func (s *Server) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		return nil
	}

	// 注意：这里不能直接使用外部传入的 ctx（例如 fx 的 OnStart ctx）。
	// 该类 ctx 往往在 OnStart 返回后就会被取消，导致后续 connectToDest()/OpenStream()
	// 立刻失败并出现 "context canceled"（典型表现：Relay CONNECT 返回 ErrCodeConnectFailed=300）。
	//
	// RelayServer 需要一个“随进程存活”的内部 ctx，并在 Stop() 时显式 cancel。
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// 注册协议处理器
	// 使用 Protocol 服务注册处理器（如果可用）
	log.Debug("注册中继协议处理器")

	// 启动清理循环
	go s.cleanupLoop()

	log.Info("中继服务器已启动",
		"max_reservations", s.config.MaxReservations,
		"max_circuits", s.config.MaxCircuits)

	return nil
}

// Stop 停止服务器
func (s *Server) Stop() error {
	if !atomic.CompareAndSwapInt32(&s.closed, 0, 1) {
		return nil
	}

	// 关闭所有电路
	s.circuitsMu.Lock()
	for _, circuit := range s.circuits {
		s.closeCircuit(circuit)
	}
	s.circuits = make(map[string]*Circuit)
	s.circuitsMu.Unlock()

	// 清理预留
	s.reservationsMu.Lock()
	s.reservations = make(map[types.NodeID]*Reservation)
	s.reservationsMu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}

	atomic.StoreInt32(&s.running, 0)
	log.Info("中继服务器已停止")
	return nil
}

// ============================================================================
//                              预留处理
// ============================================================================

// HandleReserve 处理预留请求（公开方法，用于协议处理器注册）
//
// Layer1 修复：将原来的私有方法改为公开方法，以便在 Fx 模块中注册为协议处理器
// 协议 ID: /dep2p/sys/relay/1.0.0
func (s *Server) HandleReserve(stream endpoint.Stream) {
	s.handleReserve(stream)
}

// handleReserve 处理预留请求（内部实现）
func (s *Server) handleReserve(stream endpoint.Stream) {
	defer func() { _ = stream.Close() }()

	// 从流获取远程节点 ID
	conn := stream.Connection()
	if conn == nil {
		log.Debug("收到无连接的流，拒绝预留请求")
		return
	}
	peerID := conn.RemoteID()

	log.Debug("收到预留请求",
		"peer", peerID.ShortString(),
		"isRealmRelay", s.IsRealmRelay())

	// IMPL-1227: Realm Relay PSK 验证
	// 在预留阶段验证成员资格，验证通过才能获得预留
	if s.IsRealmRelay() && s.pskAuth != nil {
		if err := s.verifyPSKMembership(stream, peerID); err != nil {
			_ = s.sendReserveError(stream, ErrCodePermission)
			log.Debug("预留请求被拒绝：PSK 验证失败",
				"peer", peerID.ShortString(),
				"err", err)
			return
		}
	}

	// 检查是否达到限制
	s.reservationsMu.RLock()
	activeCount := len(s.reservations)
	s.reservationsMu.RUnlock()

	if activeCount >= s.config.MaxReservations {
		_ = s.sendReserveError(stream, ErrCodeResourceLimit)
		log.Debug("预留请求被拒绝：达到限制",
			"peer", peerID.ShortString())
		return
	}

	// 读取请求
	_, err := s.readReserveRequest(stream)
	if err != nil {
		_ = s.sendReserveError(stream, ErrCodeMalformed)
		return
	}

	// 创建预留
	reservation := &Reservation{
		PeerID:    peerID,
		Expiry:    time.Now().Add(s.config.ReservationTTL),
		Slots:     s.config.MaxCircuitsPerPeer,
		UsedSlots: 0,
		CreatedAt: time.Now(),
		Addrs:     s.getRelayAddrs(peerID),
	}

	// 保存预留
	s.reservationsMu.Lock()
	s.reservations[peerID] = reservation
	s.reservationsMu.Unlock()

	// 更新统计
	s.statsMu.Lock()
	s.stats.TotalReservations++
	s.stats.ActiveReservations = int64(activeCount + 1)
	s.statsMu.Unlock()

	// 发送成功响应
	_ = s.sendReserveOK(stream, reservation)

	log.Info("预留成功",
		"peer", peerID.ShortString(),
		"expiry", reservation.Expiry)
}

// readReserveRequest 读取预留请求
// 格式: type(1) + version(1) + TTL(4) = 6 字节
func (s *Server) readReserveRequest(r io.Reader) (uint32, error) {
	buf := make([]byte, 6)
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, err
	}

	// 验证消息类型
	if buf[0] != MsgTypeReserve {
		return 0, fmt.Errorf("unexpected message type: %d, expected: %d", buf[0], MsgTypeReserve)
	}

	// buf[0] = msg type
	// buf[1] = version
	// buf[2:6] = TTL (uint32)
	ttl := uint32(buf[2])<<24 | uint32(buf[3])<<16 | uint32(buf[4])<<8 | uint32(buf[5])
	return ttl, nil
}

// sendReserveOK 发送预留成功响应
func (s *Server) sendReserveOK(w io.Writer, res *Reservation) error {
	// 消息类型
	if _, err := w.Write([]byte{MsgTypeReserveOK, 1}); err != nil {
		return fmt.Errorf("write message header: %w", err)
	}

	// TTL（秒）
	ttlSec := uint32(time.Until(res.Expiry).Seconds())
	ttlBytes := []byte{byte(ttlSec >> 24), byte(ttlSec >> 16), byte(ttlSec >> 8), byte(ttlSec)}
	if _, err := w.Write(ttlBytes); err != nil {
		return fmt.Errorf("write TTL: %w", err)
	}

	// Slots
	if _, err := w.Write([]byte{byte(res.Slots >> 8), byte(res.Slots)}); err != nil {
		return fmt.Errorf("write slots: %w", err)
	}

	// 地址数量
	addrCount := byte(len(res.Addrs))
	if _, err := w.Write([]byte{addrCount}); err != nil {
		return fmt.Errorf("write address count: %w", err)
	}

	// 地址列表
	for _, addr := range res.Addrs {
		addrLen := uint16(len(addr)) // #nosec G115 -- address length bounded by protocol
		if _, err := w.Write([]byte{byte(addrLen >> 8), byte(addrLen)}); err != nil {
			return fmt.Errorf("write address length: %w", err)
		}
		if _, err := w.Write([]byte(addr)); err != nil {
			return fmt.Errorf("write address: %w", err)
		}
	}

	return nil
}

// sendReserveError 发送预留错误响应
func (s *Server) sendReserveError(w io.Writer, code ErrorCode) error {
	if _, err := w.Write([]byte{MsgTypeReserveError, 1}); err != nil {
		return fmt.Errorf("write error header: %w", err)
	}
	if _, err := w.Write([]byte{byte(code >> 8), byte(code)}); err != nil {
		return fmt.Errorf("write error code: %w", err)
	}
	return nil
}

// getRelayAddrs 获取中继地址
func (s *Server) getRelayAddrs(targetPeer types.NodeID) []string {
	// 生成中继地址格式: /p2p/<relay-id>/p2p-circuit/p2p/<target-id>
	// Layer1 修复：使用完整 NodeID，避免截断导致的碰撞和不可拨号问题
	return []string{string(types.BuildSimpleRelayCircuit(s.localID, targetPeer))}
}

// ============================================================================
//                              连接处理
// ============================================================================

// HandleConnect 处理连接请求（公开方法，用于协议处理器注册）
//
// Layer1 修复：将原来的私有方法改为公开方法，以便在 Fx 模块中注册为协议处理器
// 协议 ID: /dep2p/sys/relay-hop/1.0.0
func (s *Server) HandleConnect(srcStream endpoint.Stream) {
	s.handleConnect(srcStream)
}

// handleConnect 处理连接请求（内部实现）
func (s *Server) handleConnect(srcStream endpoint.Stream) {
	// 检查连接有效性
	conn := srcStream.Connection()
	if conn == nil {
		log.Debug("收到无连接的流，拒绝连接请求")
		_ = srcStream.Close()
		return
	}
	srcPeer := conn.RemoteID()

	log.Debug("收到连接请求",
		"src", srcPeer.ShortString())

	// 读取 CONNECT 请求（包含目标节点和协议）
	req, err := s.readConnectRequest(srcStream)
	if err != nil {
		_ = s.sendConnectError(srcStream, ErrCodeMalformed)
		_ = srcStream.Close()
		return
	}
	destPeer := req.DestPeer

	log.Debug("解析连接请求",
		"src", srcPeer.ShortString(),
		"dest", destPeer.ShortString(),
		"protocol", req.Protocol)

	// IMPL-1227: 协议白名单检查
	// 如果请求中包含协议，检查是否允许
	if req.Protocol != "" && !s.isProtocolAllowed(req.Protocol) {
		_ = s.sendConnectError(srcStream, ErrCodeProtocolNotAllowed)
		_ = srcStream.Close()
		log.Debug("连接请求被拒绝：协议不允许",
			"src", srcPeer.ShortString(),
			"dest", destPeer.ShortString(),
			"protocol", req.Protocol)
		return
	}

	// 检查目标节点是否有预留
	s.reservationsMu.RLock()
	reservation, hasReservation := s.reservations[destPeer]
	s.reservationsMu.RUnlock()

	if !hasReservation || reservation.IsExpired() {
		_ = s.sendConnectError(srcStream, ErrCodeNoReservation)
		_ = srcStream.Close()
		log.Debug("连接请求被拒绝：目标无预留",
			"dest", destPeer.ShortString())
		return
	}

	// 检查槽位
	if reservation.UsedSlots >= reservation.Slots {
		_ = s.sendConnectError(srcStream, ErrCodeResourceLimit)
		_ = srcStream.Close()
		log.Debug("连接请求被拒绝：槽位已满",
			"dest", destPeer.ShortString())
		return
	}

	// 检查总电路数
	s.circuitsMu.RLock()
	circuitCount := len(s.circuits)
	s.circuitsMu.RUnlock()

	if circuitCount >= s.config.MaxCircuits {
		_ = s.sendConnectError(srcStream, ErrCodeResourceLimit)
		_ = srcStream.Close()
		log.Debug("连接请求被拒绝：达到电路限制")
		return
	}

	// 连接到目标节点
	destStream, err := s.connectToDest(srcPeer, destPeer)
	if err != nil {
		_ = s.sendConnectError(srcStream, ErrCodeConnectFailed)
		_ = srcStream.Close()
		log.Debug("连接到目标失败",
			"dest", destPeer.ShortString(),
			"err", err)
		return
	}

	// 更新预留槽位
	s.reservationsMu.Lock()
	if res, ok := s.reservations[destPeer]; ok {
		res.UsedSlots++
	}
	s.reservationsMu.Unlock()

	// 发送成功响应给源节点
	if err := s.sendConnectOK(srcStream); err != nil {
		// 回滚槽位：sendConnectOK 失败时需要释放已占用的槽位
		s.reservationsMu.Lock()
		if res, ok := s.reservations[destPeer]; ok {
			res.UsedSlots--
		}
		s.reservationsMu.Unlock()

		_ = srcStream.Close()
		_ = destStream.Close()
		return
	}

	// 创建电路
	circuit := &Circuit{
		ID:         fmt.Sprintf("%s-%s-%d", srcPeer.ShortString(), destPeer.ShortString(), time.Now().UnixNano()),
		Src:        srcPeer,
		Dest:       destPeer,
		SrcStream:  srcStream,
		DestStream: destStream,
		CreatedAt:  time.Now(),
		Deadline:   time.Now().Add(s.config.MaxDuration),
	}

	// 添加电路并获取实时计数
	s.circuitsMu.Lock()
	s.circuits[circuit.ID] = circuit
	activeCircuitCount := len(s.circuits)
	s.circuitsMu.Unlock()

	// 更新统计：使用实时电路数量
	s.statsMu.Lock()
	s.stats.TotalCircuits++
	s.stats.ActiveCircuits = int64(activeCircuitCount)
	s.stats.ConnectionsAccepted++
	s.statsMu.Unlock()

	log.Info("电路已建立",
		"id", circuit.ID,
		"src", srcPeer.ShortString(),
		"dest", destPeer.ShortString())

	// 启动数据转发
	go s.relayData(circuit)
}

// readConnectRequest 读取连接请求
// 格式: type(1) + version(1) + destPeerID(32) = 34 字节
// ConnectRequest CONNECT 请求结构（IMPL-1227 扩展）
type ConnectRequest struct {
	DestPeer types.NodeID     // 目标节点
	Protocol types.ProtocolID // 目标协议（可选，空表示无协议检查）
}

// readConnectRequest 读取 CONNECT 请求
//
// 格式（IMPL-1227 扩展版）：
//   - MsgType:    1 byte
//   - Version:    1 byte
//   - DestPeerID: 32 bytes
//   - ProtoLen:   2 bytes (big-endian, 0 表示无协议)
//   - Protocol:   变长
func (s *Server) readConnectRequest(r io.Reader) (*ConnectRequest, error) {
	// 读取固定头部
	header := make([]byte, 2+32+2) // type + version + destPeer + protoLen
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}

	// 验证消息类型
	if header[0] != MsgTypeConnect {
		return nil, fmt.Errorf("unexpected message type: %d, expected: %d", header[0], MsgTypeConnect)
	}

	// 解析 DestPeerID
	var destPeer types.NodeID
	copy(destPeer[:], header[2:34])

	// 验证 NodeID 不为零
	var zeroID types.NodeID
	if destPeer == zeroID {
		return nil, errors.New("destination peer ID is zero")
	}

	// 解析协议长度
	protoLen := int(binary.BigEndian.Uint16(header[34:36]))

	// 读取协议（如果有）
	var protocol types.ProtocolID
	if protoLen > 0 {
		if protoLen > 1024 { // 安全限制
			return nil, fmt.Errorf("protocol too long: %d", protoLen)
		}
		protoBuf := make([]byte, protoLen)
		if _, err := io.ReadFull(r, protoBuf); err != nil {
			return nil, fmt.Errorf("read protocol: %w", err)
		}
		protocol = types.ProtocolID(protoBuf)
	}

	return &ConnectRequest{
		DestPeer: destPeer,
		Protocol: protocol,
	}, nil
}

// sendConnectOK 发送连接成功响应
func (s *Server) sendConnectOK(w io.Writer) error {
	_, err := w.Write([]byte{MsgTypeConnectOK, 1})
	return err
}

// sendConnectError 发送连接错误响应
func (s *Server) sendConnectError(w io.Writer, code ErrorCode) error {
	if _, err := w.Write([]byte{MsgTypeConnectError, 1}); err != nil {
		return fmt.Errorf("write error header: %w", err)
	}
	if _, err := w.Write([]byte{byte(code >> 8), byte(code)}); err != nil {
		return fmt.Errorf("write error code: %w", err)
	}
	return nil
}

// connectToDest 连接到目标节点
func (s *Server) connectToDest(srcPeer, destPeer types.NodeID) (endpoint.Stream, error) {
	// 检查运行状态和上下文
	if atomic.LoadInt32(&s.running) == 0 || s.ctx == nil {
		return nil, errors.New("server not running")
	}

	if s.endpoint == nil {
		return nil, errors.New("no endpoint")
	}

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	// 连接到目标节点
	conn, err := s.endpoint.Connect(ctx, destPeer)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to dest: %w", err)
	}

	// 打开 STOP 流
	stream, err := conn.OpenStream(ctx, ProtocolRelayHop)
	if err != nil {
		return nil, fmt.Errorf("failed to open stream: %w", err)
	}

	// 发送 STOP 请求
	buf := make([]byte, 2+32*2) // NodeID is 32 bytes
	buf[0] = 10                 // MsgStopConnect
	buf[1] = 1                  // version
	copy(buf[2:], s.localID[:])
	copy(buf[2+32:], srcPeer[:])

	if _, err := stream.Write(buf); err != nil {
		_ = stream.Close()
		return nil, fmt.Errorf("failed to send stop request: %w", err)
	}

	// 读取响应
	respBuf := make([]byte, 4)
	if _, err := io.ReadFull(stream, respBuf); err != nil {
		_ = stream.Close()
		return nil, fmt.Errorf("failed to read stop response: %w", err)
	}

	if respBuf[0] != 11 { // MsgStopConnectOK
		_ = stream.Close()
		return nil, errors.New("dest rejected connection")
	}

	return stream, nil
}

// ============================================================================
//                              数据转发
// ============================================================================

// relayData 转发数据
func (s *Server) relayData(circuit *Circuit) {
	defer s.cleanupCircuit(circuit)

	// 设置截止时间
	_ = circuit.SrcStream.SetDeadline(circuit.Deadline)
	_ = circuit.DestStream.SetDeadline(circuit.Deadline)

	var wg sync.WaitGroup
	wg.Add(2)

	// 源 -> 目标
	go func() {
		defer wg.Done()
		n := s.copyWithLimit(circuit.DestStream, circuit.SrcStream, circuit)
		atomic.AddInt64(&circuit.BytesTransferred, n)
	}()

	// 目标 -> 源
	go func() {
		defer wg.Done()
		n := s.copyWithLimit(circuit.SrcStream, circuit.DestStream, circuit)
		atomic.AddInt64(&circuit.BytesTransferred, n)
	}()

	wg.Wait()

	log.Debug("电路关闭",
		"id", circuit.ID,
		"bytes", circuit.BytesTransferred)
}

// copyWithLimit 带限制的数据复制
func (s *Server) copyWithLimit(dst io.Writer, src io.Reader, circuit *Circuit) int64 {
	buf := make([]byte, 32*1024) // 32KB buffer
	var total int64

	for atomic.LoadInt32(&circuit.closed) == 0 && !time.Now().After(circuit.Deadline) {
		n, err := src.Read(buf)
		if n > 0 {
			// 限流：使用实际读取的字节数，而不是缓冲区大小
			if s.limiter != nil {
				s.limiter.Wait(n)
			}

			written, werr := dst.Write(buf[:n])
			if written > 0 {
				total += int64(written)

				// 更新全局统计
				s.statsMu.Lock()
				s.stats.BytesRelayed += int64(written)
				s.statsMu.Unlock()
			}
			if werr != nil {
				break
			}
		}
		if err != nil {
			break
		}
	}

	return total
}

// cleanupCircuit 清理电路
func (s *Server) cleanupCircuit(circuit *Circuit) {
	s.closeCircuit(circuit)

	// 从电路列表移除
	s.circuitsMu.Lock()
	delete(s.circuits, circuit.ID)
	activeCount := len(s.circuits)
	s.circuitsMu.Unlock()

	// 更新预留槽位
	s.reservationsMu.Lock()
	if res, ok := s.reservations[circuit.Dest]; ok {
		res.UsedSlots--
	}
	s.reservationsMu.Unlock()

	// 更新统计
	s.statsMu.Lock()
	s.stats.ActiveCircuits = int64(activeCount)
	s.statsMu.Unlock()
}

// closeCircuit 关闭电路
func (s *Server) closeCircuit(circuit *Circuit) {
	if !atomic.CompareAndSwapInt32(&circuit.closed, 0, 1) {
		return
	}

	if circuit.SrcStream != nil {
		_ = circuit.SrcStream.Close()
	}
	if circuit.DestStream != nil {
		_ = circuit.DestStream.Close()
	}
}

// ============================================================================
//                              清理
// ============================================================================

// cleanupLoop 清理循环
func (s *Server) cleanupLoop() {
	// 检查上下文是否有效
	if s.ctx == nil {
		return
	}

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if atomic.LoadInt32(&s.running) == 0 {
				return
			}
			s.cleanupExpired()
		}
	}
}

// cleanupExpired 清理过期资源
func (s *Server) cleanupExpired() {
	// 清理过期预留
	s.reservationsMu.Lock()
	for id, res := range s.reservations {
		if res.IsExpired() {
			delete(s.reservations, id)
			log.Debug("移除过期预留",
				"peer", id.ShortString())
		}
	}
	activeReservations := len(s.reservations)
	s.reservationsMu.Unlock()

	// 清理过期电路
	s.circuitsMu.Lock()
	for id, circuit := range s.circuits {
		if time.Now().After(circuit.Deadline) {
			s.closeCircuit(circuit)
			delete(s.circuits, id)
			log.Debug("移除过期电路",
				"id", id)
		}
	}
	activeCircuits := len(s.circuits)
	s.circuitsMu.Unlock()

	// 更新统计
	s.statsMu.Lock()
	s.stats.ActiveReservations = int64(activeReservations)
	s.stats.ActiveCircuits = int64(activeCircuits)
	s.statsMu.Unlock()
}

// ============================================================================
//                              查询接口
// ============================================================================

// Stats 返回统计信息
func (s *Server) Stats() relayif.RelayStats {
	s.statsMu.RLock()
	defer s.statsMu.RUnlock()

	// 安全转换：统计值始终为非负数
	bytesRelayed := s.stats.BytesRelayed
	if bytesRelayed < 0 {
		bytesRelayed = 0
	}
	totalConns := s.stats.ConnectionsAccepted + s.stats.ConnectionsRejected
	if totalConns < 0 {
		totalConns = 0
	}

	return relayif.RelayStats{
		ActiveReservations: int(s.stats.ActiveReservations),
		ActiveConnections:  int(s.stats.ActiveCircuits),
		TotalBytesRelayed:  uint64(bytesRelayed), // #nosec G115 -- checked non-negative above
		TotalConnections:   uint64(totalConns),   // #nosec G115 -- checked non-negative above
	}
}

// Reservations 返回所有预留
func (s *Server) Reservations() []relayif.ReservationInfo {
	s.reservationsMu.RLock()
	defer s.reservationsMu.RUnlock()

	result := make([]relayif.ReservationInfo, 0, len(s.reservations))
	for _, res := range s.reservations {
		result = append(result, relayif.ReservationInfo{
			PeerID:    res.PeerID,
			Expiry:    res.Expiry,
			Addrs:     res.Addrs,
			UsedSlots: res.UsedSlots,
			MaxSlots:  res.Slots,
		})
	}
	return result
}

// Config 返回配置
func (s *Server) Config() relayif.ServerConfig {
	return s.config
}

// ============================================================================
//                              错误码
// ============================================================================

// ErrorCode 错误码
type ErrorCode uint16

const (
	// ErrCodeNone 无错误
	ErrCodeNone ErrorCode = 0
	// ErrCodeMalformed 消息格式错误
	ErrCodeMalformed ErrorCode = 100
	// ErrCodeResourceLimit 资源限制
	ErrCodeResourceLimit ErrorCode = 200
	// ErrCodeNoReservation 无预留
	ErrCodeNoReservation ErrorCode = 201
	// ErrCodeConnectFailed 连接失败
	ErrCodeConnectFailed ErrorCode = 300
	// ErrCodePermission 权限错误（IMPL-1227: PSK 验证失败）
	ErrCodePermission ErrorCode = 401
	// ErrCodeProtocolNotAllowed 协议不允许（IMPL-1227: 协议白名单）
	ErrCodeProtocolNotAllowed ErrorCode = 402
)

// ============================================================================
//                              限流器
// ============================================================================

// RateLimiter 限流器
type RateLimiter struct {
	rate     int64 // 字节/秒
	tokens   int64
	lastTime time.Time
	mu       sync.Mutex
}

// NewRateLimiter 创建限流器
func NewRateLimiter(rate int64) *RateLimiter {
	if rate <= 0 {
		return nil
	}
	return &RateLimiter{
		rate:     rate,
		tokens:   rate,
		lastTime: time.Now(),
	}
}

// Wait 等待可用令牌
func (r *RateLimiter) Wait(n int) {
	if r == nil || r.rate <= 0 {
		return
	}

	r.mu.Lock()

	// 补充令牌
	now := time.Now()
	elapsed := now.Sub(r.lastTime)
	r.tokens += int64(elapsed.Seconds() * float64(r.rate))
	if r.tokens > r.rate {
		r.tokens = r.rate
	}
	r.lastTime = now

	// 消费令牌
	needed := int64(n)
	if r.tokens >= needed {
		r.tokens -= needed
		r.mu.Unlock()
		return
	}

	// 计算需要等待的时间
	deficit := needed - r.tokens
	waitTime := time.Duration(float64(deficit) / float64(r.rate) * float64(time.Second))
	r.tokens = 0 // 先消费所有可用令牌
	r.mu.Unlock()

	// 等待足够的时间让令牌补充
	time.Sleep(waitTime)
}

// ============================================================================
//                              IMPL-1227: Realm Relay 验证
// ============================================================================

// isProtocolAllowed 检查协议是否允许（IMPL-1227）
//
// Realm Relay 模式：只允许本 Realm 的应用协议和控制协议
// System Relay 模式：只允许系统协议
func (s *Server) isProtocolAllowed(proto types.ProtocolID) bool {
	protoStr := string(proto)

	// Realm Relay 模式
	if s.realmID != "" {
		// 本 Realm 应用协议：允许
		expectedAppPrefix := fmt.Sprintf(protocolids.AppProtocolTemplate, s.realmID)
		if strings.HasPrefix(protoStr, expectedAppPrefix) {
			return true
		}
		// 本 Realm 控制协议：允许
		expectedRealmPrefix := fmt.Sprintf(protocolids.RealmProtocolTemplate, s.realmID)
		if strings.HasPrefix(protoStr, expectedRealmPrefix) {
			return true
		}
		// 其他协议：拒绝
		return false
	}

	// System Relay 模式：只允许系统协议
	return strings.HasPrefix(protoStr, protocolids.SysPrefix)
}

// verifyPSKMembership 验证 PSK 成员资格（IMPL-1227）
//
// Realm Relay 模式下：
//   - 验证请求者持有 realmKey（通过 MAC 校验）
//   - 验证证明的 NodeID == 请求者的 ID
//   - 验证证明的 RealmID == 本 Relay 的 RealmID
//   - 不验证 PeerID（Relay 场景下 PeerID 是最终通信目标，由目标节点验证）
//
// 安全性说明（见 DISC-1227-relay-isolation.md）：
//   - proof.PeerID = 目标节点（A 想与 B 通信，则 PeerID = B）
//   - Relay 无法验证 PeerID（R != B），但验证 MAC 证明 A 持有 realmKey
//   - B 收到后验证 proof.PeerID == 自己（防止 Relay 中转到错误目标）
//
// 返回 nil 表示验证通过，返回错误表示验证失败。
func (s *Server) verifyPSKMembership(stream endpoint.Stream, peerID types.NodeID) error {
	// System Relay 模式：无需验证
	if s.realmID == "" {
		return nil
	}

	// Realm Relay 模式但未配置 PSK 认证器：安全策略 - 必须拒绝
	// 这是安全不变量：Realm Relay 必须有 PSK 认证器
	if s.pskAuth == nil {
		log.Error("Realm Relay 未配置 PSK 认证器，拒绝连接",
			"realmID", s.realmID,
			"peer", peerID.ShortString())
		return errors.New("realm relay requires PSK authenticator")
	}

	// 发送请求成员证明的信号
	if err := s.writeMembershipProofRequest(stream); err != nil {
		log.Debug("发送成员证明请求失败",
			"peer", peerID.ShortString(),
			"err", err)
		return fmt.Errorf("write proof request: %w", err)
	}

	// 读取成员证明
	proof, err := s.readMembershipProof(stream)
	if err != nil {
		log.Debug("读取成员证明失败",
			"peer", peerID.ShortString(),
			"err", err)
		return fmt.Errorf("read membership proof: %w", err)
	}

	// 验证证明中的 NodeID 与连接发起者匹配（基本身份验证）
	if proof.NodeID != peerID {
		log.Debug("成员证明 NodeID 不匹配",
			"expected", peerID.ShortString(),
			"got", proof.NodeID.ShortString())
		return errors.New("membership proof NodeID mismatch")
	}

	// 验证证明中的 RealmID 与当前 Relay 匹配（Realm 隔离）
	if proof.RealmID != s.realmID {
		log.Debug("成员证明 RealmID 不匹配",
			"expected", s.realmID,
			"got", proof.RealmID)
		return errors.New("membership proof RealmID mismatch")
	}

	// 验证 MAC（证明持有 realmKey）
	// 注意：这里不传 expectedPeerID，只验证 MAC 的正确性
	// Relay 验证时，我们验证 proof 中的 PeerID 是任意有效目标即可
	// 因为 Relay 场景下：
	//   - proof.PeerID = 目标节点 B（不是 Relay R）
	//   - 只有 B 能验证 PeerID == 自己
	//   - Relay 只需验证 MAC 正确（证明 A 持有 realmKey）
	if err := s.pskAuth.Verify(proof, proof.PeerID); err != nil {
		log.Debug("成员证明 MAC 验证失败",
			"peer", peerID.ShortString(),
			"err", err)
		return fmt.Errorf("verify membership proof: %w", err)
	}

	log.Debug("成员证明验证通过",
		"peer", peerID.ShortString(),
		"realmID", s.realmID,
		"targetPeer", proof.PeerID.ShortString())
	return nil
}

// readMembershipProof 从流中读取成员证明
//
// 协议格式（变长）:
//   - 长度前缀: 2 bytes (big-endian)
//   - 证明数据: 变长
//
// 证明数据格式（见 types.MembershipProof.Serialize）:
//   - NodeID: 32 bytes
//   - RealmIDLen: 2 bytes (big-endian)
//   - RealmID: 变长
//   - PeerID: 32 bytes
//   - Nonce: 16 bytes
//   - Timestamp: 8 bytes
//   - MAC: 32 bytes
func (s *Server) readMembershipProof(r io.Reader) (*types.MembershipProof, error) {
	// 读取长度前缀
	lenBuf := make([]byte, 2)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return nil, fmt.Errorf("read proof length: %w", err)
	}
	proofLen := int(binary.BigEndian.Uint16(lenBuf))

	// 安全检查
	if proofLen < types.MembershipProofMinSize || proofLen > 1024 {
		return nil, fmt.Errorf("invalid proof length: %d", proofLen)
	}

	// 读取证明数据
	buf := make([]byte, proofLen)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("read proof data: %w", err)
	}

	return types.DeserializeMembershipProof(buf)
}

// writeMembershipProofRequest 请求成员证明（发送到连接发起者）
func (s *Server) writeMembershipProofRequest(w io.Writer) error {
	// 发送请求标识
	_, err := w.Write([]byte{0x01}) // 0x01 = 请求成员证明
	return err
}

// ============================================================================
//                              接口断言
// ============================================================================

var _ relayif.RelayServer = (*Server)(nil)
