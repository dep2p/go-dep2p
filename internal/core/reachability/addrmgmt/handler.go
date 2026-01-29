// Package addrmgmt 提供地址管理协议的实现
package addrmgmt

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/protocol"
)

var logger = log.Logger("core/reachability/addrmgmt")

// ProtocolID 协议标识符（使用统一定义）
var ProtocolID = string(protocol.AddrMgmt)

// ============================================================================
//                              常量定义
// ============================================================================

const (

	// MsgTypeRefreshNotify 地址刷新通知消息类型
	MsgTypeRefreshNotify = 0x01
	// MsgTypeQueryRequest 地址查询请求消息类型
	MsgTypeQueryRequest = 0x02
	// MsgTypeQueryResponse 地址查询响应消息类型
	MsgTypeQueryResponse = 0x03

	// MaxMessageSize 最大消息大小
	MaxMessageSize = 64 * 1024 // 64KB

	// MaxAddresses 最大地址数量
	MaxAddresses = 100
)

// ============================================================================
//                              错误定义
// ============================================================================

var (
	// ErrInvalidMessage 无效消息
	ErrInvalidMessage = errors.New("invalid address management message")

	// ErrMessageTooLarge 消息过大
	ErrMessageTooLarge = errors.New("message too large")

	// ErrUnknownMessageType 未知消息类型
	ErrUnknownMessageType = errors.New("unknown message type")

	// ErrInvalidSignature 签名验证失败
	ErrInvalidSignature = errors.New("invalid address record signature")

	// ErrNodeIDMismatch 公钥与 NodeID 不匹配
	ErrNodeIDMismatch = errors.New("public key does not match node ID")

	// ErrUnsupportedKeyType 不支持的密钥类型
	ErrUnsupportedKeyType = errors.New("unsupported key type")
)

// ============================================================================
//                              AddressRecord 简化版
// ============================================================================

// AddressRecord 地址记录
type AddressRecord struct {
	// NodeID 节点 ID
	NodeID string

	// RealmID Realm ID（可选）
	RealmID string

	// Sequence 序列号（用于判断新旧）
	Sequence uint64

	// Timestamp 时间戳
	Timestamp time.Time

	// Addresses 地址列表
	Addresses []string

	// TTL 记录有效期
	TTL time.Duration

	// Signature 签名
	Signature []byte
}

// NewAddressRecord 创建新的地址记录
func NewAddressRecord(nodeID string, addrs []string, ttl time.Duration) *AddressRecord {
	return &AddressRecord{
		NodeID:    nodeID,
		Sequence:  1,
		Timestamp: time.Now(),
		Addresses: addrs,
		TTL:       ttl,
	}
}

// IsExpired 检查记录是否过期
func (r *AddressRecord) IsExpired() bool {
	return time.Since(r.Timestamp) > r.TTL
}

// IsNewerThan 检查是否比另一个记录更新
func (r *AddressRecord) IsNewerThan(other *AddressRecord) bool {
	if other == nil {
		return true
	}
	return r.Sequence > other.Sequence
}

// UpdateAddresses 更新地址并递增序列号
func (r *AddressRecord) UpdateAddresses(addrs []string) {
	// BUG FIX #B31: 防止序列号溢出
	// 使用饱和算法：到达最大值后保持不变
	if r.Sequence == ^uint64(0) {
		logger.Warn("地址记录序列号已达最大值，无法继续递增",
			"nodeID", r.NodeID,
			"sequence", r.Sequence)
		// 仍然更新地址和时间戳，但序列号保持最大值
		r.Addresses = addrs
		r.Timestamp = time.Now()
		return
	}
	
	r.Addresses = addrs
	r.Sequence++
	r.Timestamp = time.Now()
}

// Clone 克隆记录
func (r *AddressRecord) Clone() *AddressRecord {
	addrs := make([]string, len(r.Addresses))
	copy(addrs, r.Addresses)

	sig := make([]byte, len(r.Signature))
	copy(sig, r.Signature)

	return &AddressRecord{
		NodeID:    r.NodeID,
		RealmID:   r.RealmID,
		Sequence:  r.Sequence,
		Timestamp: r.Timestamp,
		Addresses: addrs,
		TTL:       r.TTL,
		Signature: sig,
	}
}

// ============================================================================
//                              Handler 实现
// ============================================================================

// Handler 地址管理协议处理器
type Handler struct {
	// 本地身份
	localID string

	// 地址记录管理
	records   map[string]*AddressRecord
	recordsMu sync.RWMutex

	// 邻居管理
	neighbors map[string]*neighborInfo

	// 签名验证函数（可选，由外部注入）
	verifySignature func(nodeID string, data, signature []byte) bool
}

// neighborInfo 邻居信息
type neighborInfo struct {
	ID       string
	LastSeen time.Time
	Addrs    []string
}

// NewHandler 创建处理器
func NewHandler(localID string) *Handler {
	return &Handler{
		localID:   localID,
		records:   make(map[string]*AddressRecord),
		neighbors: make(map[string]*neighborInfo),
	}
}

// SetSignatureVerifier 设置签名验证函数
func (h *Handler) SetSignatureVerifier(verify func(nodeID string, data, signature []byte) bool) {
	h.verifySignature = verify
}

// HandleStream 处理传入的流
func (h *Handler) HandleStream(stream interfaces.Stream) {
	defer func() { _ = stream.Close() }()

	// 读取消息头
	header := make([]byte, 5) // 1 byte type + 4 bytes length
	if _, err := io.ReadFull(stream, header); err != nil {
		logger.Debug("读取消息头失败", "err", err)
		return
	}

	msgType := header[0]
	msgLen := binary.BigEndian.Uint32(header[1:5])

	if msgLen > MaxMessageSize {
		logger.Warn("消息过大", "size", msgLen)
		return
	}

	// 读取消息体
	body := make([]byte, msgLen)
	if _, err := io.ReadFull(stream, body); err != nil {
		logger.Debug("读取消息体失败", "err", err)
		return
	}

	// 处理消息
	switch msgType {
	case MsgTypeRefreshNotify:
		h.handleRefreshNotify(stream, body)
	case MsgTypeQueryRequest:
		h.handleQueryRequest(stream, body)
	default:
		logger.Debug("未知消息类型", "type", msgType)
	}
}

// handleRefreshNotify 处理地址刷新通知
func (h *Handler) handleRefreshNotify(_ interfaces.Stream, body []byte) {
	if len(body) < 50 {
		logger.Debug("刷新通知消息过短")
		return
	}

	record, err := h.decodeRefreshNotify(body)
	if err != nil {
		logger.Debug("解码刷新通知失败", "err", err)
		return
	}

	// 验证签名（如果配置了验证函数）
	if h.verifySignature != nil && len(record.Signature) > 0 {
		// 构建签名数据（不包含签名本身）
		dataLen := len(body) - len(record.Signature)
		if dataLen > 0 && !h.verifySignature(record.NodeID, body[:dataLen], record.Signature) {
			logger.Warn("地址记录签名验证失败",
				"nodeID", record.NodeID)
			return
		}
		logger.Debug("签名验证成功",
			"nodeID", record.NodeID)
	}

	// 检查序列号
	h.recordsMu.RLock()
	existing := h.records[record.NodeID]
	h.recordsMu.RUnlock()

	if existing != nil && !record.IsNewerThan(existing) {
		logger.Debug("收到过时的地址记录",
			"nodeID", record.NodeID,
			"existing", existing.Sequence,
			"received", record.Sequence)
		return
	}

	// 更新记录
	h.recordsMu.Lock()
	h.records[record.NodeID] = record
	h.recordsMu.Unlock()

	logger.Debug("收到地址刷新通知",
		"nodeID", record.NodeID,
		"addrs", len(record.Addresses),
		"seq", record.Sequence)
}

// handleQueryRequest 处理地址查询请求
func (h *Handler) handleQueryRequest(stream interfaces.Stream, body []byte) {
	if len(body) < 1 {
		return
	}

	// 读取目标 ID 长度
	idLen := int(body[0])
	if len(body) < 1+idLen {
		return
	}

	// 查找地址（直接使用 string 转换以提高效率）
	h.recordsMu.RLock()
	record := h.records[string(body[1:1+idLen])]
	h.recordsMu.RUnlock()

	// 发送响应
	var response []byte
	if record != nil {
		response = h.encodeQueryResponse(record)
	} else {
		// 空响应
		response = make([]byte, 5)
		response[0] = MsgTypeQueryResponse
		// 长度为 0
	}

	_, _ = stream.Write(response)
}

// ============================================================================
//                              消息编解码
// ============================================================================

// decodeRefreshNotify 解码刷新通知
func (h *Handler) decodeRefreshNotify(data []byte) (*AddressRecord, error) {
	if len(data) < 20 {
		return nil, ErrInvalidMessage
	}

	record := &AddressRecord{}
	offset := 0

	// NodeID length + NodeID
	if offset >= len(data) {
		return nil, ErrInvalidMessage
	}
	idLen := int(data[offset])
	offset++

	if offset+idLen > len(data) {
		return nil, ErrInvalidMessage
	}
	record.NodeID = string(data[offset : offset+idLen])
	offset += idLen

	// RealmID length + RealmID
	if offset+2 > len(data) {
		return nil, ErrInvalidMessage
	}
	realmLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2

	if offset+realmLen > len(data) {
		return nil, ErrInvalidMessage
	}
	record.RealmID = string(data[offset : offset+realmLen])
	offset += realmLen

	// Sequence
	if offset+8 > len(data) {
		return nil, ErrInvalidMessage
	}
	record.Sequence = binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8

	// Timestamp
	if offset+8 > len(data) {
		return nil, ErrInvalidMessage
	}
	ts := binary.BigEndian.Uint64(data[offset : offset+8])
	record.Timestamp = time.Unix(0, int64(ts))
	offset += 8

	// Address count
	if offset+2 > len(data) {
		return nil, ErrInvalidMessage
	}
	addrCount := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2

	if addrCount > MaxAddresses {
		return nil, ErrMessageTooLarge
	}

	// Addresses
	record.Addresses = make([]string, 0, addrCount)
	for i := 0; i < addrCount; i++ {
		if offset+2 > len(data) {
			return nil, ErrInvalidMessage
		}
		addrLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2

		if offset+addrLen > len(data) {
			return nil, ErrInvalidMessage
		}
		record.Addresses = append(record.Addresses, string(data[offset:offset+addrLen]))
		offset += addrLen
	}

	// TTL (默认 1 小时)
	record.TTL = time.Hour

	// Signature (剩余数据)
	if offset < len(data) {
		record.Signature = make([]byte, len(data)-offset)
		copy(record.Signature, data[offset:])
	}

	return record, nil
}

// encodeRefreshNotify 编码刷新通知
func (h *Handler) encodeRefreshNotify(record *AddressRecord) []byte {
	// 估算大小
	size := 5 + 1 + len(record.NodeID) + 2 + len(record.RealmID) + 8 + 8 + 2
	for _, addr := range record.Addresses {
		size += 2 + len(addr)
	}
	size += len(record.Signature)

	buf := make([]byte, size)
	offset := 0

	// 消息类型
	buf[offset] = MsgTypeRefreshNotify
	offset++

	// 长度占位
	offset += 4

	// NodeID length + NodeID
	buf[offset] = byte(len(record.NodeID))
	offset++
	copy(buf[offset:], record.NodeID)
	offset += len(record.NodeID)

	// RealmID length + RealmID
	binary.BigEndian.PutUint16(buf[offset:], uint16(len(record.RealmID)))
	offset += 2
	copy(buf[offset:], record.RealmID)
	offset += len(record.RealmID)

	// Sequence
	binary.BigEndian.PutUint64(buf[offset:], record.Sequence)
	offset += 8

	// Timestamp
	binary.BigEndian.PutUint64(buf[offset:], uint64(record.Timestamp.UnixNano()))
	offset += 8

	// Address count
	binary.BigEndian.PutUint16(buf[offset:], uint16(len(record.Addresses)))
	offset += 2

	// Addresses
	for _, addr := range record.Addresses {
		binary.BigEndian.PutUint16(buf[offset:], uint16(len(addr)))
		offset += 2
		copy(buf[offset:], addr)
		offset += len(addr)
	}

	// Signature
	copy(buf[offset:], record.Signature)
	offset += len(record.Signature)

	// 填充长度
	binary.BigEndian.PutUint32(buf[1:5], uint32(offset-5))

	return buf[:offset]
}

// encodeQueryResponse 编码查询响应
func (h *Handler) encodeQueryResponse(record *AddressRecord) []byte {
	// 估算大小
	size := 5 + 1 + len(record.NodeID) + 8 + 2
	for _, addr := range record.Addresses {
		size += 2 + len(addr)
	}

	buf := make([]byte, size)
	offset := 0

	// 消息类型
	buf[offset] = MsgTypeQueryResponse
	offset++

	// 长度占位
	offset += 4

	// NodeID length + NodeID
	buf[offset] = byte(len(record.NodeID))
	offset++
	copy(buf[offset:], record.NodeID)
	offset += len(record.NodeID)

	// Sequence
	binary.BigEndian.PutUint64(buf[offset:], record.Sequence)
	offset += 8

	// Address count
	binary.BigEndian.PutUint16(buf[offset:], uint16(len(record.Addresses)))
	offset += 2

	// Addresses
	for _, addr := range record.Addresses {
		binary.BigEndian.PutUint16(buf[offset:], uint16(len(addr)))
		offset += 2
		copy(buf[offset:], addr)
		offset += len(addr)
	}

	// 填充长度
	binary.BigEndian.PutUint32(buf[1:5], uint32(offset-5))

	return buf[:offset]
}

// ============================================================================
//                              发送方法
// ============================================================================

// SendRefreshNotify 发送地址刷新通知
func (h *Handler) SendRefreshNotify(_ context.Context, stream interfaces.Stream, record *AddressRecord) error {
	data := h.encodeRefreshNotify(record)
	_, err := stream.Write(data)
	return err
}

// QueryPeer 查询节点地址
func (h *Handler) QueryPeer(_ context.Context, stream interfaces.Stream, targetID string) (*AddressRecord, error) {
	// 发送查询请求
	request := make([]byte, 6+len(targetID))
	request[0] = MsgTypeQueryRequest
	binary.BigEndian.PutUint32(request[1:5], uint32(1+len(targetID)))
	request[5] = byte(len(targetID))
	copy(request[6:], targetID)

	if _, err := stream.Write(request); err != nil {
		return nil, err
	}

	// 读取响应
	header := make([]byte, 5)
	if _, err := io.ReadFull(stream, header); err != nil {
		return nil, err
	}

	if header[0] != MsgTypeQueryResponse {
		return nil, ErrUnknownMessageType
	}

	msgLen := binary.BigEndian.Uint32(header[1:5])
	if msgLen == 0 {
		return nil, nil // 未找到
	}

	if msgLen > MaxMessageSize {
		return nil, ErrMessageTooLarge
	}

	body := make([]byte, msgLen)
	if _, err := io.ReadFull(stream, body); err != nil {
		return nil, err
	}

	return h.decodeQueryResponse(body)
}

// decodeQueryResponse 解码查询响应
func (h *Handler) decodeQueryResponse(data []byte) (*AddressRecord, error) {
	if len(data) < 11 {
		return nil, ErrInvalidMessage
	}

	record := &AddressRecord{}
	offset := 0

	// NodeID length + NodeID
	idLen := int(data[offset])
	offset++

	if offset+idLen > len(data) {
		return nil, ErrInvalidMessage
	}
	record.NodeID = string(data[offset : offset+idLen])
	offset += idLen

	// Sequence
	if offset+8 > len(data) {
		return nil, ErrInvalidMessage
	}
	record.Sequence = binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8

	// Address count
	if offset+2 > len(data) {
		return nil, ErrInvalidMessage
	}
	addrCount := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2

	if addrCount > MaxAddresses {
		return nil, ErrMessageTooLarge
	}

	// Addresses
	record.Addresses = make([]string, 0, addrCount)
	for i := 0; i < addrCount; i++ {
		if offset+2 > len(data) {
			return nil, ErrInvalidMessage
		}
		addrLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2

		if offset+addrLen > len(data) {
			return nil, ErrInvalidMessage
		}
		record.Addresses = append(record.Addresses, string(data[offset:offset+addrLen]))
		offset += addrLen
	}

	record.TTL = time.Hour
	record.Timestamp = time.Now()

	return record, nil
}

// ============================================================================
//                              辅助方法
// ============================================================================

// GetRecord 获取地址记录
func (h *Handler) GetRecord(nodeID string) *AddressRecord {
	h.recordsMu.RLock()
	defer h.recordsMu.RUnlock()
	return h.records[nodeID]
}

// GetAllRecords 获取所有地址记录
func (h *Handler) GetAllRecords() map[string]*AddressRecord {
	h.recordsMu.RLock()
	defer h.recordsMu.RUnlock()

	result := make(map[string]*AddressRecord, len(h.records))
	for k, v := range h.records {
		result[k] = v.Clone()
	}
	return result
}

// RemoveRecord 移除地址记录
func (h *Handler) RemoveRecord(nodeID string) {
	h.recordsMu.Lock()
	defer h.recordsMu.Unlock()
	delete(h.records, nodeID)
}

// CleanExpired 清理过期记录
func (h *Handler) CleanExpired() int {
	h.recordsMu.Lock()
	defer h.recordsMu.Unlock()

	count := 0
	for nodeID, record := range h.records {
		if record.IsExpired() {
			delete(h.records, nodeID)
			count++
		}
	}
	return count
}
