// Package addrmgmt 提供地址管理协议的实现
//
// 协议 ID: /dep2p/sys/addr-mgmt/1.0.0 (v1.1 scope: sys)
//
// 本包实现以下协议消息：
// - AddressRefreshNotify: 地址刷新通知
// - AddressQueryRequest/Response: 地址查询请求/响应
package addrmgmt

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/address"
	endpointif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              常量定义
// ============================================================================

// 引用 pkg/protocolids 唯一真源
var (
	// ProtocolID 协议标识符 (v1.1 scope: sys)
	ProtocolID = protocolids.SysAddrMgmt
)

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
//                              Handler 实现
// ============================================================================

// Handler 地址管理协议处理器
type Handler struct {
	// 本地身份
	localID     types.NodeID
	addressBook *address.AddressBook

	// 密钥工厂（用于解析公钥，通过 Fx 注入）
	keyFactory identityif.KeyFactory

	// 地址记录管理
	records   map[types.NodeID]*address.AddressRecord
	recordsMu sync.RWMutex

	// 邻居管理
	neighbors map[types.NodeID]neighborInfo
}

// neighborInfo 邻居信息
type neighborInfo struct {
	ID       types.NodeID
	LastSeen time.Time
	Addrs    []endpointif.Address
}

// NewHandler 创建处理器
//
// keyFactory: 密钥工厂（用于解析公钥，通过 Fx 注入）
func NewHandler(localID types.NodeID, addressBook *address.AddressBook, keyFactory identityif.KeyFactory) *Handler {
	return &Handler{
		localID:     localID,
		addressBook: addressBook,
		keyFactory:  keyFactory,
		records:     make(map[types.NodeID]*address.AddressRecord),
		neighbors:   make(map[types.NodeID]neighborInfo),
	}
}

// HandleStream 处理传入的流
func (h *Handler) HandleStream(stream endpointif.Stream) {
	defer func() { _ = stream.Close() }()

	// 读取消息头
	header := make([]byte, 5) // 1 byte type + 4 bytes length
	if _, err := io.ReadFull(stream, header); err != nil {
		log.Debug("读取消息头失败", "err", err)
		return
	}

	msgType := header[0]
	msgLen := binary.BigEndian.Uint32(header[1:5])

	if msgLen > MaxMessageSize {
		log.Warn("消息过大", "size", msgLen)
		return
	}

	// 读取消息体
	body := make([]byte, msgLen)
	if _, err := io.ReadFull(stream, body); err != nil {
		log.Debug("读取消息体失败", "err", err)
		return
	}

	// 处理消息
	switch msgType {
	case MsgTypeRefreshNotify:
		h.handleRefreshNotify(stream, body)
	case MsgTypeQueryRequest:
		h.handleQueryRequest(stream, body)
	default:
		log.Debug("未知消息类型", "type", msgType)
	}
}

// handleRefreshNotify 处理地址刷新通知
//
// AddressRefreshNotify 消息格式:
// - NodeID: 32 bytes
// - RealmID length: 2 bytes
// - RealmID: variable
// - Sequence: 8 bytes
// - Timestamp: 8 bytes
// - AddressCount: 2 bytes
// - Addresses: variable (length-prefixed strings)
// - KeyType: 1 byte
// - PublicKey length: 2 bytes
// - PublicKey: variable
// - Signature: variable
func (h *Handler) handleRefreshNotify(_ endpointif.Stream, body []byte) {
	if len(body) < 50 { // 最小长度检查
		log.Debug("刷新通知消息过短")
		return
	}

	record, pubKey, err := h.decodeRefreshNotifyWithKey(body)
	if err != nil {
		log.Debug("解码刷新通知失败", "err", err)
		return
	}

	// 验证签名
	if pubKey != nil && len(record.Signature) > 0 {
		// 1. 验证公钥与 NodeID 匹配（使用接口层公共函数）
		expectedNodeID := identityif.NodeIDFromPublicKey(pubKey)
		if !expectedNodeID.Equal(record.NodeID) {
			log.Warn("公钥与 NodeID 不匹配",
				"expected", expectedNodeID.ShortString(),
				"actual", record.NodeID.ShortString())
			return
		}

		// 2. 验证签名
		if !record.Verify(pubKey) {
			log.Warn("地址记录签名验证失败",
				"nodeID", record.NodeID.ShortString())
			return
		}

		log.Debug("签名验证成功",
			"nodeID", record.NodeID.ShortString())
	} else {
		// 无签名或无公钥：仅在调试/测试模式下接受
		log.Debug("收到未签名的地址记录",
			"nodeID", record.NodeID.ShortString(),
			"hasKey", pubKey != nil,
			"sigLen", len(record.Signature))
	}

	// 检查序列号
	h.recordsMu.RLock()
	existing := h.records[record.NodeID]
	h.recordsMu.RUnlock()

	if existing != nil && !record.IsNewerThan(existing) {
		log.Debug("收到过时的地址记录",
			"nodeID", record.NodeID.ShortString(),
			"existing", existing.Sequence,
			"received", record.Sequence)
		return
	}

	// 更新记录
	h.recordsMu.Lock()
	h.records[record.NodeID] = record
	h.recordsMu.Unlock()

	// 更新地址簿
	if h.addressBook != nil {
		h.addressBook.AddAddrs(record.NodeID, record.Addresses, record.TTL)
	}

	log.Debug("收到地址刷新通知",
		"nodeID", record.NodeID.ShortString(),
		"addrs", len(record.Addresses),
		"seq", record.Sequence)
}

// handleQueryRequest 处理地址查询请求
func (h *Handler) handleQueryRequest(stream endpointif.Stream, body []byte) {
	if len(body) < 32 {
		return
	}

	var targetID types.NodeID
	copy(targetID[:], body[:32])

	// 查找地址
	h.recordsMu.RLock()
	record := h.records[targetID]
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

// decodeRefreshNotifyWithKey 解码刷新通知（含公钥）
//
// 消息格式:
// - NodeID: 32 bytes
// - RealmID length: 2 bytes
// - RealmID: variable
// - Sequence: 8 bytes
// - Timestamp: 8 bytes
// - AddressCount: 2 bytes
// - Addresses: variable (length-prefixed strings)
// - KeyType: 1 byte (0=无公钥, 1=Ed25519, 2=ECDSA-P256, 3=ECDSA-P384)
// - PublicKey length: 2 bytes (仅当 KeyType > 0)
// - PublicKey: variable (仅当 KeyType > 0)
// - Signature: variable
func (h *Handler) decodeRefreshNotifyWithKey(data []byte) (*address.AddressRecord, identityif.PublicKey, error) {
	if len(data) < 50 {
		return nil, nil, ErrInvalidMessage
	}

	record := &address.AddressRecord{}
	offset := 0

	// NodeID
	copy(record.NodeID[:], data[offset:offset+32])
	offset += 32

	// RealmID length
	realmLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2

	if offset+realmLen > len(data) {
		return nil, nil, ErrInvalidMessage
	}

	// RealmID
	record.RealmID = types.RealmID(data[offset : offset+realmLen])
	offset += realmLen

	// Sequence
	if offset+8 > len(data) {
		return nil, nil, ErrInvalidMessage
	}
	record.Sequence = binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8

	// Timestamp
	if offset+8 > len(data) {
		return nil, nil, ErrInvalidMessage
	}
	ts := binary.BigEndian.Uint64(data[offset : offset+8])
	record.Timestamp = time.Unix(0, int64(ts)) // #nosec G115 -- timestamp conversion is safe
	offset += 8

	// Address count
	if offset+2 > len(data) {
		return nil, nil, ErrInvalidMessage
	}
	addrCount := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2

	if addrCount > MaxAddresses {
		return nil, nil, ErrMessageTooLarge
	}

	// Addresses
	record.Addresses = make([]endpointif.Address, 0, addrCount)
	for i := 0; i < addrCount; i++ {
		if offset+2 > len(data) {
			return nil, nil, ErrInvalidMessage
		}
		addrLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2

		if offset+addrLen > len(data) {
			return nil, nil, ErrInvalidMessage
		}
		addrStr := string(data[offset : offset+addrLen])
		offset += addrLen

		record.Addresses = append(record.Addresses, address.NewAddr(types.Multiaddr(addrStr)))
	}

	// TTL (默认 1 小时)
	record.TTL = time.Hour

	// 解析公钥（如果存在）
	var pubKey identityif.PublicKey
	if offset < len(data) {
		// KeyType
		keyType := types.KeyType(data[offset])
		offset++

		if keyType != types.KeyTypeUnknown && offset+2 <= len(data) {
			// PublicKey length
			keyLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
			offset += 2

			if offset+keyLen > len(data) {
				return nil, nil, ErrInvalidMessage
			}

			// PublicKey
			keyBytes := data[offset : offset+keyLen]
			offset += keyLen

			// 根据类型解析公钥（使用 KeyFactory 通过 Fx 注入）
			var err error
			if h.keyFactory != nil {
				pubKey, err = h.keyFactory.PublicKeyFromBytes(keyBytes, keyType)
				if err != nil {
					log.Debug("解析公钥失败",
						"keyType", keyType.String(),
						"err", err)
					// 继续处理，但不进行签名验证
				}
			}
		}
	}

	// 签名 (剩余数据)
	if offset < len(data) {
		record.Signature = make([]byte, len(data)-offset)
		copy(record.Signature, data[offset:])
	}

	return record, pubKey, nil
}

// encodeQueryResponse 编码查询响应
func (h *Handler) encodeQueryResponse(record *address.AddressRecord) []byte {
	// 估算大小
	size := 5 + 32 + 8 + 2 // header + nodeID + seq + addrCount
	for _, addr := range record.Addresses {
		size += 2 + len(addr.String())
	}

	buf := make([]byte, size)
	offset := 0

	// 消息类型
	buf[offset] = MsgTypeQueryResponse
	offset++

	// 长度（稍后填充）
	offset += 4

	// NodeID
	copy(buf[offset:], record.NodeID[:])
	offset += 32

	// Sequence
	binary.BigEndian.PutUint64(buf[offset:], record.Sequence)
	offset += 8

	// Address count (bounded by MaxAddresses)
	binary.BigEndian.PutUint16(buf[offset:], uint16(len(record.Addresses))) // #nosec G115 -- bounded by MaxAddresses
	offset += 2

	// Addresses
	for _, addr := range record.Addresses {
		addrStr := addr.String()
		binary.BigEndian.PutUint16(buf[offset:], uint16(len(addrStr))) // #nosec G115 -- bounded by protocol
		offset += 2
		copy(buf[offset:], addrStr)
		offset += len(addrStr)
	}

	// 填充长度
	binary.BigEndian.PutUint32(buf[1:5], uint32(offset-5)) // #nosec G115 -- bounded by MaxMessageSize

	return buf[:offset]
}

// ============================================================================
//                              发送方法
// ============================================================================

// SendRefreshNotify 发送地址刷新通知
func (h *Handler) SendRefreshNotify(_ context.Context, stream endpointif.Stream, record *address.AddressRecord) error {
	// 编码消息
	data := h.encodeRefreshNotify(record)

	_, err := stream.Write(data)
	return err
}

// encodeRefreshNotify 编码刷新通知（无公钥，向后兼容）
func (h *Handler) encodeRefreshNotify(record *address.AddressRecord) []byte {
	return h.encodeRefreshNotifyWithKey(record, nil)
}

// encodeRefreshNotifyWithKey 编码刷新通知（含公钥）
//
// 消息格式:
// - 消息类型: 1 byte
// - 消息长度: 4 bytes
// - NodeID: 32 bytes
// - RealmID length: 2 bytes
// - RealmID: variable
// - Sequence: 8 bytes
// - Timestamp: 8 bytes
// - AddressCount: 2 bytes
// - Addresses: variable
// - KeyType: 1 byte
// - PublicKey length: 2 bytes (仅当有公钥)
// - PublicKey: variable (仅当有公钥)
// - Signature: variable
func (h *Handler) encodeRefreshNotifyWithKey(record *address.AddressRecord, pubKey identityif.PublicKey) []byte {
	// 估算大小
	size := 5 + 32 + 2 + len(record.RealmID) + 8 + 8 + 2
	for _, addr := range record.Addresses {
		size += 2 + len(addr.String())
	}
	// KeyType
	size++
	// PublicKey (如果有)
	var keyBytes []byte
	var keyType types.KeyType
	if pubKey != nil {
		keyBytes = pubKey.Bytes()
		keyType = pubKey.Type()
		size += 2 + len(keyBytes)
	}
	// Signature
	size += len(record.Signature)

	buf := make([]byte, size)
	offset := 0

	// 消息类型
	buf[offset] = MsgTypeRefreshNotify
	offset++

	// 长度占位
	offset += 4

	// NodeID
	copy(buf[offset:], record.NodeID[:])
	offset += 32

	// RealmID length
	binary.BigEndian.PutUint16(buf[offset:], uint16(len(record.RealmID))) // #nosec G115 -- bounded by protocol
	offset += 2

	// RealmID
	copy(buf[offset:], record.RealmID)
	offset += len(record.RealmID)

	// Sequence
	binary.BigEndian.PutUint64(buf[offset:], record.Sequence)
	offset += 8

	// Timestamp
	binary.BigEndian.PutUint64(buf[offset:], uint64(record.Timestamp.UnixNano())) // #nosec G115 -- timestamp is always positive
	offset += 8

	// Address count
	binary.BigEndian.PutUint16(buf[offset:], uint16(len(record.Addresses))) // #nosec G115 -- bounded by MaxAddresses
	offset += 2

	// Addresses
	for _, addr := range record.Addresses {
		addrStr := addr.String()
		binary.BigEndian.PutUint16(buf[offset:], uint16(len(addrStr))) // #nosec G115 -- bounded by protocol
		offset += 2
		copy(buf[offset:], addrStr)
		offset += len(addrStr)
	}

	// KeyType
	buf[offset] = byte(keyType)
	offset++

	// PublicKey (如果有)
	if len(keyBytes) > 0 {
		binary.BigEndian.PutUint16(buf[offset:], uint16(len(keyBytes))) // #nosec G115 -- bounded by key size
		offset += 2
		copy(buf[offset:], keyBytes)
		offset += len(keyBytes)
	}

	// Signature
	copy(buf[offset:], record.Signature)
	offset += len(record.Signature)

	// 填充长度
	binary.BigEndian.PutUint32(buf[1:5], uint32(offset-5)) // #nosec G115 -- bounded by MaxMessageSize

	return buf[:offset]
}

// SendRefreshNotifyWithKey 发送地址刷新通知（含公钥）
func (h *Handler) SendRefreshNotifyWithKey(_ context.Context, stream endpointif.Stream, record *address.AddressRecord, pubKey identityif.PublicKey) error {
	data := h.encodeRefreshNotifyWithKey(record, pubKey)
	_, err := stream.Write(data)
	return err
}

// QueryPeer 查询节点地址
func (h *Handler) QueryPeer(_ context.Context, stream endpointif.Stream, targetID types.NodeID) (*address.AddressRecord, error) {
	// 发送查询请求
	request := make([]byte, 37)
	request[0] = MsgTypeQueryRequest
	binary.BigEndian.PutUint32(request[1:5], 32)
	copy(request[5:], targetID[:])

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
func (h *Handler) decodeQueryResponse(data []byte) (*address.AddressRecord, error) {
	if len(data) < 42 { // 32 + 8 + 2
		return nil, ErrInvalidMessage
	}

	record := &address.AddressRecord{}
	offset := 0

	// NodeID
	copy(record.NodeID[:], data[offset:offset+32])
	offset += 32

	// Sequence
	record.Sequence = binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8

	// Address count
	addrCount := int(binary.BigEndian.Uint16(data[offset : offset+2]))
	offset += 2

	if addrCount > MaxAddresses {
		return nil, ErrMessageTooLarge
	}

	// Addresses
	record.Addresses = make([]endpointif.Address, 0, addrCount)
	for i := 0; i < addrCount; i++ {
		if offset+2 > len(data) {
			return nil, ErrInvalidMessage
		}
		addrLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
		offset += 2

		if offset+addrLen > len(data) {
			return nil, ErrInvalidMessage
		}
		addrStr := string(data[offset : offset+addrLen])
		offset += addrLen

		record.Addresses = append(record.Addresses, address.NewAddr(types.Multiaddr(addrStr)))
	}

	record.TTL = time.Hour
	record.Timestamp = time.Now()

	return record, nil
}

// stringAddress 已删除，统一使用 address.Addr
