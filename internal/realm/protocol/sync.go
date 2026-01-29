// Package protocol 实现 Realm 协议
package protocol

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/protocol"
)

// SyncProtocolID 同步协议 ID 模板（使用统一定义）
//
// 格式：/dep2p/realm/<realmID>/sync/1.0.0
var SyncProtocolID = protocol.RealmSyncFormat

// 默认同步配置
const (
	DefaultSyncInterval   = 30 * time.Second // 默认同步间隔
	DefaultSyncPeerCount  = 3                // 默认同步节点数
	DefaultMinSyncMembers = 1                // 最少成员数才启动同步
)

// ============================================================================
//                              消息类型
// ============================================================================

// 消息类型定义
const (
	MsgTypeSyncRequest  byte = 0x01 // 同步请求
	MsgTypeSyncResponse byte = 0x02 // 同步响应
)

// ============================================================================
//                              消息结构
// ============================================================================

// SyncRequest 同步请求消息
type SyncRequest struct {
	RealmID      string // Realm ID
	LocalVersion uint64 // 本地版本号
}

// SyncResponse 同步响应消息
type SyncResponse struct {
	Version uint64                   // 远程版本号
	Added   []*interfaces.MemberInfo // 新增成员
	Removed []string                 // 移除的成员 PeerID 列表
}

// ============================================================================
//                              同步协议处理器
// ============================================================================

// SyncHandler 同步协议处理器
//
// 提供 Realm 成员同步的协议层封装。
type SyncHandler struct {
	mu sync.RWMutex

	// 依赖
	host          pkgif.Host
	realmID       string
	synchronizer  interfaces.MemberSynchronizer // 同步器
	memberManager interfaces.MemberManager      // 成员管理器

	// 版本控制
	version atomic.Uint64

	// 同步配置
	syncInterval   time.Duration
	syncPeerCount  int
	minSyncMembers int

	// 自动同步
	syncTicker *time.Ticker
	syncCtx    context.Context
	syncCancel context.CancelFunc

	// 状态
	started bool
	closed  bool

	// 回调
	onSyncSuccess func(peerID string, added, removed int)
	onSyncFailed  func(peerID string, err error)
}

// NewSyncHandler 创建同步处理器
//
// 参数：
//   - host: P2P 主机
//   - realmID: Realm ID
//   - synchronizer: 同步器（用于应用增量）
//   - memberManager: 成员管理器
func NewSyncHandler(
	host pkgif.Host,
	realmID string,
	synchronizer interfaces.MemberSynchronizer,
	memberManager interfaces.MemberManager,
) *SyncHandler {
	h := &SyncHandler{
		host:           host,
		realmID:        realmID,
		synchronizer:   synchronizer,
		memberManager:  memberManager,
		syncInterval:   DefaultSyncInterval,
		syncPeerCount:  DefaultSyncPeerCount,
		minSyncMembers: DefaultMinSyncMembers,
	}

	// 初始化版本号为 1
	h.version.Store(1)

	return h
}

// ============================================================================
//                              生命周期管理
// ============================================================================

// Start 启动处理器，注册协议到 Host 并启动自动同步
func (h *SyncHandler) Start(_ context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.started {
		return fmt.Errorf("sync handler already started")
	}

	if h.closed {
		return fmt.Errorf("sync handler is closed")
	}

	// 构造协议 ID
	protocolID := fmt.Sprintf(SyncProtocolID, h.realmID)

	// 注册协议处理器
	h.host.SetStreamHandler(protocolID, h.handleIncoming)

	// 启动自动同步循环
	// 使用 context.Background() 保证后台循环不受上层 ctx 取消的影响
	h.syncCtx, h.syncCancel = context.WithCancel(context.Background())
	h.syncTicker = time.NewTicker(h.syncInterval)
	go h.syncLoop()

	h.started = true

	return nil
}

// Stop 停止处理器，注销协议并停止自动同步
func (h *SyncHandler) Stop(_ context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.started {
		return nil
	}

	if h.closed {
		return nil
	}

	// 停止自动同步
	if h.syncCancel != nil {
		h.syncCancel()
	}

	if h.syncTicker != nil {
		h.syncTicker.Stop()
	}

	// 注销协议处理器
	protocolID := fmt.Sprintf(SyncProtocolID, h.realmID)
	h.host.RemoveStreamHandler(protocolID)

	h.started = false

	return nil
}

// Close 关闭处理器
func (h *SyncHandler) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return nil
	}

	h.closed = true

	// 如果已启动，先停止
	if h.started {
		if h.syncCancel != nil {
			h.syncCancel()
		}
		if h.syncTicker != nil {
			h.syncTicker.Stop()
		}

		protocolID := fmt.Sprintf(SyncProtocolID, h.realmID)
		h.host.RemoveStreamHandler(protocolID)
		h.started = false
	}

	return nil
}

// ============================================================================
//                              入站处理（服务端侧）
// ============================================================================

// handleIncoming 处理入站同步请求
//
// 流程：
//  1. 读取 SYNC_REQUEST
//  2. 验证 RealmID
//  3. 比较版本号
//  4. 如果本地版本更高，返回增量数据
//  5. 否则返回空响应
func (h *SyncHandler) handleIncoming(stream pkgif.Stream) {
	defer stream.Close()

	h.mu.RLock()
	if h.closed {
		h.mu.RUnlock()
		return
	}
	realmID := h.realmID
	memberManager := h.memberManager
	localVersion := h.version.Load()
	h.mu.RUnlock()

	// 1. 读取 SYNC_REQUEST
	request, err := h.readSyncRequest(stream)
	if err != nil {
		return
	}

	// 2. 验证 RealmID
	if request.RealmID != realmID {
		return
	}

	// 3. 比较版本号
	response := &SyncResponse{
		Version: localVersion,
		Added:   []*interfaces.MemberInfo{},
		Removed: []string{},
	}

	// 如果本地版本更高，返回全量成员列表（简化实现）
	// 实际生产环境应该维护增量日志
	if localVersion > request.LocalVersion {
		ctx := context.Background()
		members, err := memberManager.List(ctx, interfaces.DefaultListOptions())
		if err == nil {
			response.Added = members
		}
	}

	// 4. 发送响应
	_ = h.sendSyncResponse(stream, response)
}

// ============================================================================
//                              出站请求（客户端侧）
// ============================================================================

// RequestSync 向指定节点发起同步请求
//
// 参数：
//   - ctx: 上下文
//   - peerID: 目标节点 ID
//
// 返回：
//   - error: 错误
func (h *SyncHandler) RequestSync(ctx context.Context, peerID string) error {
	h.mu.RLock()
	if h.closed {
		h.mu.RUnlock()
		return fmt.Errorf("sync handler is closed")
	}
	realmID := h.realmID
	localVersion := h.version.Load()
	synchronizer := h.synchronizer
	onSuccess := h.onSyncSuccess
	onFailed := h.onSyncFailed
	h.mu.RUnlock()

	// 构造协议 ID
	protocolID := fmt.Sprintf(SyncProtocolID, realmID)

	// 打开流到目标节点
	stream, err := h.host.NewStream(ctx, peerID, protocolID)
	if err != nil {
		if onFailed != nil {
			onFailed(peerID, err)
		}
		return fmt.Errorf("failed to open sync stream: %w", err)
	}
	defer stream.Close()

	// 1. 发送 SYNC_REQUEST
	request := &SyncRequest{
		RealmID:      realmID,
		LocalVersion: localVersion,
	}

	if err := h.writeSyncRequest(stream, request); err != nil {
		if onFailed != nil {
			onFailed(peerID, err)
		}
		return fmt.Errorf("failed to send sync request: %w", err)
	}

	// 2. 接收 SYNC_RESPONSE
	response, err := h.readSyncResponse(stream)
	if err != nil {
		if onFailed != nil {
			onFailed(peerID, err)
		}
		return fmt.Errorf("failed to receive sync response: %w", err)
	}

	// 3. 如果远程版本更高，应用增量
	if response.Version > localVersion {
		if synchronizer != nil {
			if err := synchronizer.SyncDelta(ctx, response.Added, convertToMemberInfoList(response.Removed)); err != nil {
				if onFailed != nil {
					onFailed(peerID, err)
				}
				return fmt.Errorf("failed to apply delta: %w", err)
			}

			// 更新本地版本号
			h.version.Store(response.Version)
		}

		// 触发成功回调
		if onSuccess != nil {
			onSuccess(peerID, len(response.Added), len(response.Removed))
		}
	}

	return nil
}

// ============================================================================
//                              自动同步循环
// ============================================================================

// syncLoop 自动同步循环
//
// 每隔 syncInterval 执行一次同步：
//  1. 随机选择 syncPeerCount 个成员
//  2. 向每个成员发起同步请求
func (h *SyncHandler) syncLoop() {
	for {
		select {
		case <-h.syncTicker.C:
			h.performAutoSync()

		case <-h.syncCtx.Done():
			return
		}
	}
}

// performAutoSync 执行自动同步
func (h *SyncHandler) performAutoSync() {
	h.mu.RLock()
	memberManager := h.memberManager
	syncPeerCount := h.syncPeerCount
	minSyncMembers := h.minSyncMembers
	h.mu.RUnlock()

	if memberManager == nil {
		return
	}

	ctx := context.Background()

	// 获取成员列表
	members, err := memberManager.List(ctx, interfaces.DefaultListOptions())
	if err != nil || len(members) < minSyncMembers {
		return
	}

	// 随机选择成员
	selectedPeers := h.selectRandomPeers(members, syncPeerCount)

	// 向每个成员发起同步请求
	for _, peerID := range selectedPeers {
		// 跳过自己
		if peerID == string(h.host.ID()) {
			continue
		}

		// 异步执行同步
		go func(pid string) {
			syncCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			_ = h.RequestSync(syncCtx, pid)
		}(peerID)
	}
}

// selectRandomPeers 随机选择成员
func (h *SyncHandler) selectRandomPeers(members []*interfaces.MemberInfo, count int) []string {
	if len(members) <= count {
		result := make([]string, len(members))
		for i, m := range members {
			result[i] = m.PeerID
		}
		return result
	}

	// 使用 Fisher-Yates 洗牌算法
	indices := make([]int, len(members))
	for i := range indices {
		indices[i] = i
	}

	for i := len(indices) - 1; i > 0; i-- {
		j := rand.Intn(i + 1) //nolint:gosec // G404: 随机打乱顺序不需要加密级随机
		indices[i], indices[j] = indices[j], indices[i]
	}

	result := make([]string, count)
	for i := 0; i < count; i++ {
		result[i] = members[indices[i]].PeerID
	}

	return result
}

// ============================================================================
//                              版本控制
// ============================================================================

// GetVersion 获取当前版本号
func (h *SyncHandler) GetVersion() uint64 {
	return h.version.Load()
}

// IncrementVersion 递增版本号
func (h *SyncHandler) IncrementVersion() uint64 {
	return h.version.Add(1)
}

// SetVersion 设置版本号
func (h *SyncHandler) SetVersion(version uint64) {
	h.version.Store(version)
}

// ============================================================================
//                              配置方法
// ============================================================================

// SetSyncInterval 设置同步间隔
func (h *SyncHandler) SetSyncInterval(interval time.Duration) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.syncInterval = interval

	// 如果已启动，重启 ticker
	if h.started && h.syncTicker != nil {
		h.syncTicker.Stop()
		h.syncTicker = time.NewTicker(interval)
	}
}

// SetSyncPeerCount 设置同步节点数
func (h *SyncHandler) SetSyncPeerCount(count int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.syncPeerCount = count
}

// ============================================================================
//                              回调函数
// ============================================================================

// SetOnSyncSuccess 设置同步成功回调
func (h *SyncHandler) SetOnSyncSuccess(fn func(peerID string, added, removed int)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onSyncSuccess = fn
}

// SetOnSyncFailed 设置同步失败回调
func (h *SyncHandler) SetOnSyncFailed(fn func(peerID string, err error)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.onSyncFailed = fn
}

// ============================================================================
//                              消息编解码
// ============================================================================

// readSyncRequest 读取同步请求
func (h *SyncHandler) readSyncRequest(stream pkgif.Stream) (*SyncRequest, error) {
	// 读取消息类型
	typeBuf := make([]byte, 1)
	if _, err := io.ReadFull(stream, typeBuf); err != nil {
		return nil, err
	}

	if typeBuf[0] != MsgTypeSyncRequest {
		return nil, fmt.Errorf("unexpected message type: %d", typeBuf[0])
	}

	// 读取消息体
	data, err := readMessage(stream)
	if err != nil {
		return nil, err
	}

	return decodeSyncRequest(data)
}

// writeSyncRequest 写入同步请求
func (h *SyncHandler) writeSyncRequest(stream pkgif.Stream, req *SyncRequest) error {
	// 写入消息类型
	if _, err := stream.Write([]byte{MsgTypeSyncRequest}); err != nil {
		return err
	}

	// 编码消息体
	data := encodeSyncRequest(req)

	// 写入消息体
	return writeMessage(stream, data)
}

// readSyncResponse 读取同步响应
func (h *SyncHandler) readSyncResponse(stream pkgif.Stream) (*SyncResponse, error) {
	// 读取消息类型
	typeBuf := make([]byte, 1)
	if _, err := io.ReadFull(stream, typeBuf); err != nil {
		return nil, err
	}

	if typeBuf[0] != MsgTypeSyncResponse {
		return nil, fmt.Errorf("unexpected message type: %d", typeBuf[0])
	}

	// 读取消息体
	data, err := readMessage(stream)
	if err != nil {
		return nil, err
	}

	return decodeSyncResponse(data)
}

// sendSyncResponse 发送同步响应
func (h *SyncHandler) sendSyncResponse(stream pkgif.Stream, resp *SyncResponse) error {
	// 写入消息类型
	if _, err := stream.Write([]byte{MsgTypeSyncResponse}); err != nil {
		return err
	}

	// 编码消息体
	data := encodeSyncResponse(resp)

	// 写入消息体
	return writeMessage(stream, data)
}

// ============================================================================
//                              编解码实现
// ============================================================================

// encodeSyncRequest 编码同步请求
//
// 格式：[realmID长度(2)][realmID][version(8)]
func encodeSyncRequest(req *SyncRequest) []byte {
	buf := make([]byte, 0, 256)

	// RealmID
	buf = appendString(buf, req.RealmID)

	// LocalVersion
	buf = appendUint64(buf, req.LocalVersion)

	return buf
}

// decodeSyncRequest 解码同步请求
func decodeSyncRequest(data []byte) (*SyncRequest, error) {
	offset := 0
	req := &SyncRequest{}

	// RealmID
	realmID, n, err := readString(data, offset)
	if err != nil {
		return nil, err
	}
	req.RealmID = realmID
	offset += n

	// LocalVersion
	if offset+8 > len(data) {
		return nil, fmt.Errorf("invalid data: insufficient length for version")
	}
	req.LocalVersion = binary.BigEndian.Uint64(data[offset : offset+8])
	// offset += 8 // 函数结束，不再需要更新 offset

	return req, nil
}

// encodeSyncResponse 编码同步响应
//
// Phase 11 修复：完整编码地址和元数据
// 格式：[version(8)][新增数量(2)][新增成员...][移除数量(2)][移除PeerID...]
// 成员格式：[peerID][realmID][role(1)][online(1)][addrs数量(2)][addrs...][metadata数量(2)][metadata...]
func encodeSyncResponse(resp *SyncResponse) []byte {
	buf := make([]byte, 0, 1024)

	// Version
	buf = appendUint64(buf, resp.Version)

	// Added 数量
	buf = appendUint16(buf, uint16(len(resp.Added)))

	// Added 列表
	for _, member := range resp.Added {
		buf = appendString(buf, member.PeerID)
		buf = appendString(buf, member.RealmID)
		buf = append(buf, byte(member.Role))
		if member.Online {
			buf = append(buf, 1)
		} else {
			buf = append(buf, 0)
		}

		// Phase 11 修复：编码地址列表
		buf = appendUint16(buf, uint16(len(member.Addrs)))
		for _, addr := range member.Addrs {
			buf = appendString(buf, addr)
		}

		// Phase 11 修复：编码元数据
		buf = appendUint16(buf, uint16(len(member.Metadata)))
		for key, value := range member.Metadata {
			buf = appendString(buf, key)
			buf = appendString(buf, value)
		}
	}

	// Removed 数量
	buf = appendUint16(buf, uint16(len(resp.Removed)))

	// Removed 列表
	for _, peerID := range resp.Removed {
		buf = appendString(buf, peerID)
	}

	return buf
}

// decodeSyncResponse 解码同步响应
func decodeSyncResponse(data []byte) (*SyncResponse, error) {
	if len(data) < 8 {
		return nil, fmt.Errorf("invalid data: too short")
	}

	offset := 0
	resp := &SyncResponse{}

	// Version
	resp.Version = binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8

	// Added 数量
	if offset+2 > len(data) {
		return nil, fmt.Errorf("invalid data: insufficient length for added count")
	}
	addedCount := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	// Added 列表
	resp.Added = make([]*interfaces.MemberInfo, addedCount)
	for i := 0; i < int(addedCount); i++ {
		member := &interfaces.MemberInfo{}

		// PeerID
		peerID, n, err := readString(data, offset)
		if err != nil {
			return nil, err
		}
		member.PeerID = peerID
		offset += n

		// RealmID
		realmID, n, err := readString(data, offset)
		if err != nil {
			return nil, err
		}
		member.RealmID = realmID
		offset += n

		// Role
		if offset >= len(data) {
			return nil, fmt.Errorf("invalid data: insufficient length for role")
		}
		member.Role = interfaces.Role(data[offset])
		offset++

		// Online
		if offset >= len(data) {
			return nil, fmt.Errorf("invalid data: insufficient length for online")
		}
		member.Online = data[offset] == 1
		offset++

		// Phase 11 修复：解码地址列表
		if offset+2 > len(data) {
			return nil, fmt.Errorf("invalid data: insufficient length for addrs count")
		}
		addrsCount := binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2

		member.Addrs = make([]string, addrsCount)
		for j := 0; j < int(addrsCount); j++ {
			addr, n, err := readString(data, offset)
			if err != nil {
				return nil, err
			}
			member.Addrs[j] = addr
			offset += n
		}

		// Phase 11 修复：解码元数据
		if offset+2 > len(data) {
			return nil, fmt.Errorf("invalid data: insufficient length for metadata count")
		}
		metadataCount := binary.BigEndian.Uint16(data[offset : offset+2])
		offset += 2

		member.Metadata = make(map[string]string, metadataCount)
		for j := 0; j < int(metadataCount); j++ {
			key, n, err := readString(data, offset)
			if err != nil {
				return nil, err
			}
			offset += n

			value, n, err := readString(data, offset)
			if err != nil {
				return nil, err
			}
			offset += n

			member.Metadata[key] = value
		}

		resp.Added[i] = member
	}

	// Removed 数量
	if offset+2 > len(data) {
		return nil, fmt.Errorf("invalid data: insufficient length for removed count")
	}
	removedCount := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	// Removed 列表
	resp.Removed = make([]string, removedCount)
	for i := 0; i < int(removedCount); i++ {
		peerID, n, err := readString(data, offset)
		if err != nil {
			return nil, err
		}
		resp.Removed[i] = peerID
		offset += n
	}

	return resp, nil
}

// ============================================================================
//                              辅助函数
// ============================================================================

// appendUint16 追加 uint16
func appendUint16(buf []byte, v uint16) []byte {
	tmp := make([]byte, 2)
	binary.BigEndian.PutUint16(tmp, v)
	return append(buf, tmp...)
}

// appendUint64 追加 uint64
func appendUint64(buf []byte, v uint64) []byte {
	tmp := make([]byte, 8)
	binary.BigEndian.PutUint64(tmp, v)
	return append(buf, tmp...)
}

// appendString 追加字符串（带长度前缀）
//
// 格式：[长度(2)][内容]
func appendString(buf []byte, s string) []byte {
	buf = appendUint16(buf, uint16(len(s)))
	return append(buf, []byte(s)...)
}

// readString 从数据中读取字符串
//
// 返回：字符串内容、消耗的字节数、错误
func readString(data []byte, offset int) (string, int, error) {
	if offset+2 > len(data) {
		return "", 0, fmt.Errorf("invalid data: insufficient length for string length")
	}

	length := binary.BigEndian.Uint16(data[offset : offset+2])
	if offset+2+int(length) > len(data) {
		return "", 0, fmt.Errorf("invalid data: insufficient length for string content")
	}

	return string(data[offset+2 : offset+2+int(length)]), 2 + int(length), nil
}

// convertToMemberInfoList 将 PeerID 列表转换为 MemberInfo 列表
func convertToMemberInfoList(peerIDs []string) []*interfaces.MemberInfo {
	result := make([]*interfaces.MemberInfo, len(peerIDs))
	for i, peerID := range peerIDs {
		result[i] = &interfaces.MemberInfo{
			PeerID: peerID,
		}
	}
	return result
}
