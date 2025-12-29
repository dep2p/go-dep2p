// Package realm 提供 Realm 管理实现
package realm

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              同步协议消息类型
// ============================================================================

// SyncMsgType 同步消息类型
type SyncMsgType uint8

const (
	// SyncMsgMemberList 成员列表请求/响应
	SyncMsgMemberList SyncMsgType = iota

	// SyncMsgMemberJoin 成员加入通知
	SyncMsgMemberJoin

	// SyncMsgMemberLeave 成员离开通知
	SyncMsgMemberLeave

	// SyncMsgMetadata 元数据广播
	SyncMsgMetadata

	// SyncMsgHeartbeat 心跳
	SyncMsgHeartbeat
)

// 引用 pkg/protocolids 唯一真源
var (
	// ProtocolSync Realm 同步协议标识
	ProtocolSync = protocolids.SysRealmSync
)

const (
	// MaxSyncPayloadSize 最大同步消息 payload 大小 (1MB)
	MaxSyncPayloadSize = 1 << 20
)

// ============================================================================
//                              同步消息
// ============================================================================

// SyncMessage 同步消息
type SyncMessage struct {
	Type      SyncMsgType
	RealmID   types.RealmID
	From      types.NodeID
	Timestamp int64
	Payload   []byte
}

// MemberInfo 成员信息
type MemberInfo struct {
	NodeID   types.NodeID
	JoinedAt int64
	Role     string
	Addrs    []string
}

// ============================================================================
//                              SyncService 同步服务
// ============================================================================

// SyncService Realm 同步服务
type SyncService struct {
	manager  *Manager
	endpoint endpoint.Endpoint

	// 同步状态
	lastSync map[types.RealmID]time.Time
	syncMu   sync.RWMutex

	// 运行状态
	running int32
	closed  int32
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewSyncService 创建同步服务
func NewSyncService(manager *Manager, endpoint endpoint.Endpoint) *SyncService {
	return &SyncService{
		manager:  manager,
		endpoint: endpoint,
		lastSync: make(map[types.RealmID]time.Time),
	}
}

// Start 启动同步服务
func (s *SyncService) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		return nil
	}

	s.ctx, s.cancel = context.WithCancel(ctx)

	log.Info("同步服务启动中")

	// 注册协议处理器
	if s.endpoint != nil {
		s.endpoint.SetProtocolHandler(ProtocolSync, s.handleSyncStream)
	}

	// 启动定期同步
	go s.syncLoop()

	log.Info("同步服务已启动")
	return nil
}

// Stop 停止同步服务
func (s *SyncService) Stop() error {
	if !atomic.CompareAndSwapInt32(&s.closed, 0, 1) {
		return nil
	}

	log.Info("同步服务停止中")

	if s.cancel != nil {
		s.cancel()
	}

	if s.endpoint != nil {
		s.endpoint.RemoveProtocolHandler(ProtocolSync)
	}

	atomic.StoreInt32(&s.running, 0)
	log.Info("同步服务已停止")
	return nil
}

// SyncMembers 同步成员列表
func (s *SyncService) SyncMembers(ctx context.Context, realmID types.RealmID) error {
	if s.manager == nil || s.endpoint == nil {
		return ErrNotMember
	}

	// 获取 Realm 内的节点
	peers := s.manager.RealmPeers(realmID)
	if len(peers) == 0 {
		return nil
	}

	log.Debug("同步成员列表",
		"realm", string(realmID),
		"peers", len(peers))

	// 向多个节点请求成员列表
	var wg sync.WaitGroup
	membersCh := make(chan []MemberInfo, len(peers))

	for _, peer := range peers {
		wg.Add(1)
		go func(nodeID types.NodeID) {
			defer wg.Done()

			members, err := s.requestMemberList(ctx, nodeID, realmID)
			if err != nil {
				log.Debug("请求成员列表失败",
					"peer", nodeID.ShortString(),
					"err", err)
				return
			}

			membersCh <- members
		}(peer)
	}

	go func() {
		wg.Wait()
		close(membersCh)
	}()

	// 合并成员列表
	allMembers := make(map[types.NodeID]MemberInfo)
	for members := range membersCh {
		for _, m := range members {
			if existing, ok := allMembers[m.NodeID]; ok {
				// 保留较新的信息
				if m.JoinedAt > existing.JoinedAt {
					allMembers[m.NodeID] = m
				}
			} else {
				allMembers[m.NodeID] = m
			}
		}
	}

	// 更新本地成员列表
	for nodeID := range allMembers {
		s.manager.addRealmPeer(realmID, nodeID, nil)
	}

	// 更新同步时间
	s.syncMu.Lock()
	s.lastSync[realmID] = time.Now()
	s.syncMu.Unlock()

	return nil
}

// requestMemberList 请求成员列表
func (s *SyncService) requestMemberList(ctx context.Context, nodeID types.NodeID, realmID types.RealmID) ([]MemberInfo, error) {
	conn, ok := s.endpoint.Connection(nodeID)
	if !ok {
		var err error
		conn, err = s.endpoint.Connect(ctx, nodeID)
		if err != nil {
			return nil, err
		}
	}

	stream, err := conn.OpenStream(ctx, ProtocolSync)
	if err != nil {
		return nil, err
	}
	defer func() { _ = stream.Close() }()

	// 发送请求
	msg := &SyncMessage{
		Type:      SyncMsgMemberList,
		RealmID:   realmID,
		Timestamp: time.Now().UnixNano(),
	}

	if err := s.writeMessage(stream, msg); err != nil {
		return nil, err
	}

	// 读取响应
	resp, err := s.readMessage(stream)
	if err != nil {
		return nil, err
	}

	// 解析成员列表
	return s.decodeMemberList(resp.Payload)
}

// BroadcastMetadata 广播元数据
func (s *SyncService) BroadcastMetadata(ctx context.Context, realmID types.RealmID, metadata []byte) error {
	if s.manager == nil || s.endpoint == nil {
		return ErrNotMember
	}

	peers := s.manager.RealmPeers(realmID)
	if len(peers) == 0 {
		return nil
	}

	log.Debug("广播元数据",
		"realm", string(realmID),
		"peers", len(peers),
		"size", len(metadata))

	msg := &SyncMessage{
		Type:      SyncMsgMetadata,
		RealmID:   realmID,
		Timestamp: time.Now().UnixNano(),
		Payload:   metadata,
	}

	var wg sync.WaitGroup
	for _, peer := range peers {
		wg.Add(1)
		go func(nodeID types.NodeID) {
			defer wg.Done()
			_ = s.sendMessage(ctx, nodeID, msg)
		}(peer)
	}

	wg.Wait()
	return nil
}

// BroadcastMemberJoin 广播成员加入
func (s *SyncService) BroadcastMemberJoin(ctx context.Context, realmID types.RealmID, newMember types.NodeID) error {
	peers := s.manager.RealmPeers(realmID)

	msg := &SyncMessage{
		Type:      SyncMsgMemberJoin,
		RealmID:   realmID,
		Timestamp: time.Now().UnixNano(),
		Payload:   newMember[:],
	}

	for _, peer := range peers {
		if peer == newMember {
			continue
		}
		go func(p types.NodeID) { _ = s.sendMessage(ctx, p, msg) }(peer)
	}

	return nil
}

// BroadcastMemberLeave 广播成员离开
func (s *SyncService) BroadcastMemberLeave(ctx context.Context, realmID types.RealmID, leavingMember types.NodeID) error {
	peers := s.manager.RealmPeers(realmID)

	msg := &SyncMessage{
		Type:      SyncMsgMemberLeave,
		RealmID:   realmID,
		Timestamp: time.Now().UnixNano(),
		Payload:   leavingMember[:],
	}

	for _, peer := range peers {
		if peer == leavingMember {
			continue
		}
		go func(p types.NodeID) { _ = s.sendMessage(ctx, p, msg) }(peer)
	}

	return nil
}

// sendMessage 发送消息
func (s *SyncService) sendMessage(ctx context.Context, nodeID types.NodeID, msg *SyncMessage) error {
	conn, ok := s.endpoint.Connection(nodeID)
	if !ok {
		var err error
		conn, err = s.endpoint.Connect(ctx, nodeID)
		if err != nil {
			return err
		}
	}

	stream, err := conn.OpenStream(ctx, ProtocolSync)
	if err != nil {
		return err
	}
	defer func() { _ = stream.Close() }()

	return s.writeMessage(stream, msg)
}

// handleSyncStream 处理同步流
func (s *SyncService) handleSyncStream(stream endpoint.Stream) {
	defer func() { _ = stream.Close() }()

	msg, err := s.readMessage(stream)
	if err != nil {
		log.Debug("读取同步消息失败", "err", err)
		return
	}

	switch msg.Type {
	case SyncMsgMemberList:
		s.handleMemberListRequest(stream, msg)

	case SyncMsgMemberJoin:
		s.handleMemberJoin(msg)

	case SyncMsgMemberLeave:
		s.handleMemberLeave(msg)

	case SyncMsgMetadata:
		s.handleMetadata(msg)

	case SyncMsgHeartbeat:
		// 心跳不需要处理
	}
}

// handleMemberListRequest 处理成员列表请求
func (s *SyncService) handleMemberListRequest(stream endpoint.Stream, msg *SyncMessage) {
	peers := s.manager.RealmPeers(msg.RealmID)

	// 构建成员信息列表
	members := make([]MemberInfo, 0, len(peers))
	for _, peer := range peers {
		members = append(members, MemberInfo{
			NodeID:   peer,
			JoinedAt: time.Now().UnixNano(), // 实际应该从状态获取
		})
	}

	// 编码成员列表
	payload := s.encodeMemberList(members)

	// 发送响应
	resp := &SyncMessage{
		Type:      SyncMsgMemberList,
		RealmID:   msg.RealmID,
		Timestamp: time.Now().UnixNano(),
		Payload:   payload,
	}

	s.writeMessage(stream, resp)
}

// handleMemberJoin 处理成员加入
func (s *SyncService) handleMemberJoin(msg *SyncMessage) {
	if len(msg.Payload) < 32 {
		return
	}

	var nodeID types.NodeID
	copy(nodeID[:], msg.Payload[:32])

	s.manager.addRealmPeer(msg.RealmID, nodeID, nil)

	log.Debug("收到成员加入通知",
		"realm", string(msg.RealmID),
		"node", nodeID.ShortString())
}

// handleMemberLeave 处理成员离开
func (s *SyncService) handleMemberLeave(msg *SyncMessage) {
	if len(msg.Payload) < 32 {
		return
	}

	var nodeID types.NodeID
	copy(nodeID[:], msg.Payload[:32])

	s.manager.removeRealmPeer(msg.RealmID, nodeID)

	log.Debug("收到成员离开通知",
		"realm", string(msg.RealmID),
		"node", nodeID.ShortString())
}

// handleMetadata 处理元数据
func (s *SyncService) handleMetadata(msg *SyncMessage) {
	log.Debug("收到元数据广播",
		"realm", string(msg.RealmID),
		"size", len(msg.Payload))

	// 实际应用中，这里应该解析并更新本地元数据
}

// syncLoop 同步循环
func (s *SyncService) syncLoop() {
	// 检查 ctx 是否为 nil（防止 Start() 未调用）
	if s.ctx == nil {
		return
	}

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.runPeriodicSync()
		}
	}
}

// runPeriodicSync 执行定期同步
//
// v1.1 变更: 严格单 Realm 模型，只同步当前 Realm
func (s *SyncService) runPeriodicSync() {
	if s.manager == nil || s.ctx == nil {
		return
	}

	// IMPL-1227: CurrentRealm() 现在返回 Realm 对象
	realm := s.manager.CurrentRealm()
	if realm == nil {
		return // 未加入任何 Realm
	}
	currentRealmID := realm.ID()

	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	if err := s.SyncMembers(ctx, currentRealmID); err != nil {
		log.Debug("定期同步失败",
			"realm", string(currentRealmID),
			"err", err)
	}
}

// ============================================================================
//                              消息编解码
// ============================================================================

// writeMessage 写入消息
func (s *SyncService) writeMessage(w io.Writer, msg *SyncMessage) error {
	var buf bytes.Buffer

	// Type
	buf.WriteByte(byte(msg.Type))

	// RealmID
	realmBytes := []byte(msg.RealmID)
	_ = binary.Write(&buf, binary.BigEndian, uint16(len(realmBytes))) //nolint:gosec // G115: realm ID 长度由协议限制
	buf.Write(realmBytes)

	// From
	buf.Write(msg.From[:])

	// Timestamp
	_ = binary.Write(&buf, binary.BigEndian, msg.Timestamp)

	// Payload
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(msg.Payload)))
	buf.Write(msg.Payload)

	_, err := w.Write(buf.Bytes())
	return err
}

// readMessage 读取消息
func (s *SyncService) readMessage(r io.Reader) (*SyncMessage, error) {
	msg := &SyncMessage{}

	// Type
	var msgType byte
	if err := binary.Read(r, binary.BigEndian, &msgType); err != nil {
		return nil, err
	}
	msg.Type = SyncMsgType(msgType)

	// RealmID
	var realmLen uint16
	if err := binary.Read(r, binary.BigEndian, &realmLen); err != nil {
		return nil, err
	}
	realmBytes := make([]byte, realmLen)
	if _, err := io.ReadFull(r, realmBytes); err != nil {
		return nil, err
	}
	msg.RealmID = types.RealmID(realmBytes)

	// From
	if _, err := io.ReadFull(r, msg.From[:]); err != nil {
		return nil, err
	}

	// Timestamp
	if err := binary.Read(r, binary.BigEndian, &msg.Timestamp); err != nil {
		return nil, err
	}

	// Payload
	var payloadLen uint32
	if err := binary.Read(r, binary.BigEndian, &payloadLen); err != nil {
		return nil, err
	}

	// 检查 payload 大小限制
	if payloadLen > MaxSyncPayloadSize {
		return nil, ErrMessageTooLarge
	}

	msg.Payload = make([]byte, payloadLen)
	if _, err := io.ReadFull(r, msg.Payload); err != nil {
		return nil, err
	}

	return msg, nil
}

// encodeMemberList 编码成员列表
func (s *SyncService) encodeMemberList(members []MemberInfo) []byte {
	var buf bytes.Buffer

	// 成员数量
	_ = binary.Write(&buf, binary.BigEndian, uint32(len(members)))

	for _, m := range members {
		// NodeID
		buf.Write(m.NodeID[:])

		// JoinedAt
		_ = binary.Write(&buf, binary.BigEndian, m.JoinedAt)

		// Role
		roleBytes := []byte(m.Role)
		_ = binary.Write(&buf, binary.BigEndian, uint16(len(roleBytes))) //nolint:gosec // G115: role 长度由协议限制
		buf.Write(roleBytes)
	}

	return buf.Bytes()
}

// decodeMemberList 解码成员列表
func (s *SyncService) decodeMemberList(data []byte) ([]MemberInfo, error) {
	buf := bytes.NewReader(data)

	// 成员数量
	var count uint32
	if err := binary.Read(buf, binary.BigEndian, &count); err != nil {
		return nil, err
	}

	members := make([]MemberInfo, 0, count)

	for i := uint32(0); i < count; i++ {
		var m MemberInfo

		// NodeID
		if _, err := io.ReadFull(buf, m.NodeID[:]); err != nil {
			return nil, err
		}

		// JoinedAt
		if err := binary.Read(buf, binary.BigEndian, &m.JoinedAt); err != nil {
			return nil, err
		}

		// Role
		var roleLen uint16
		if err := binary.Read(buf, binary.BigEndian, &roleLen); err != nil {
			return nil, err
		}
		roleBytes := make([]byte, roleLen)
		if _, err := io.ReadFull(buf, roleBytes); err != nil {
			return nil, err
		}
		m.Role = string(roleBytes)

		members = append(members, m)
	}

	return members, nil
}

