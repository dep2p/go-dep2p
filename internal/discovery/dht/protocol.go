// Package dht 提供分布式哈希表实现
package dht

import (
	"encoding/json"

	"github.com/dep2p/go-dep2p/pkg/protocol"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              协议定义
// ============================================================================

// ProtocolID DHT 协议 ID（使用统一定义）
//
// 使用 /dep2p/sys/ 前缀，表示系统协议无需 Realm 验证
var ProtocolID = string(protocol.DHT)

// ============================================================================
//                              消息类型
// ============================================================================

// MessageType 消息类型
type MessageType uint8

const (
	// MessageTypeFindNode FIND_NODE 请求
	MessageTypeFindNode MessageType = iota + 1
	// MessageTypeFindNodeResponse FIND_NODE 响应
	MessageTypeFindNodeResponse

	// MessageTypeFindValue FIND_VALUE 请求
	MessageTypeFindValue
	// MessageTypeFindValueResponse FIND_VALUE 响应
	MessageTypeFindValueResponse

	// MessageTypeStore STORE 请求
	MessageTypeStore
	// MessageTypeStoreResponse STORE 响应
	MessageTypeStoreResponse

	// MessageTypePing PING 请求
	MessageTypePing
	// MessageTypePingResponse PING 响应
	MessageTypePingResponse

	// MessageTypeAddProvider ADD_PROVIDER 请求
	MessageTypeAddProvider
	// MessageTypeAddProviderResponse ADD_PROVIDER 响应
	MessageTypeAddProviderResponse

	// MessageTypeGetProviders GET_PROVIDERS 请求
	MessageTypeGetProviders
	// MessageTypeGetProvidersResponse GET_PROVIDERS 响应
	MessageTypeGetProvidersResponse

	// MessageTypeRemoveProvider REMOVE_PROVIDER 请求
	MessageTypeRemoveProvider
	// MessageTypeRemoveProviderResponse REMOVE_PROVIDER 响应
	MessageTypeRemoveProviderResponse

	// MessageTypePutPeerRecord PUT_PEER_RECORD 请求（v2.0 新增）
	MessageTypePutPeerRecord
	// MessageTypePutPeerRecordResponse PUT_PEER_RECORD 响应
	MessageTypePutPeerRecordResponse

	// MessageTypeGetPeerRecord GET_PEER_RECORD 请求（v2.0 新增）
	MessageTypeGetPeerRecord
	// MessageTypeGetPeerRecordResponse GET_PEER_RECORD 响应
	MessageTypeGetPeerRecordResponse
)

// String 返回消息类型的字符串表示
func (m MessageType) String() string {
	switch m {
	case MessageTypeFindNode:
		return "FIND_NODE"
	case MessageTypeFindNodeResponse:
		return "FIND_NODE_RESPONSE"
	case MessageTypeFindValue:
		return "FIND_VALUE"
	case MessageTypeFindValueResponse:
		return "FIND_VALUE_RESPONSE"
	case MessageTypeStore:
		return "STORE"
	case MessageTypeStoreResponse:
		return "STORE_RESPONSE"
	case MessageTypePing:
		return "PING"
	case MessageTypePingResponse:
		return "PING_RESPONSE"
	case MessageTypeAddProvider:
		return "ADD_PROVIDER"
	case MessageTypeAddProviderResponse:
		return "ADD_PROVIDER_RESPONSE"
	case MessageTypeGetProviders:
		return "GET_PROVIDERS"
	case MessageTypeGetProvidersResponse:
		return "GET_PROVIDERS_RESPONSE"
	case MessageTypeRemoveProvider:
		return "REMOVE_PROVIDER"
	case MessageTypeRemoveProviderResponse:
		return "REMOVE_PROVIDER_RESPONSE"
	case MessageTypePutPeerRecord:
		return "PUT_PEER_RECORD"
	case MessageTypePutPeerRecordResponse:
		return "PUT_PEER_RECORD_RESPONSE"
	case MessageTypeGetPeerRecord:
		return "GET_PEER_RECORD"
	case MessageTypeGetPeerRecordResponse:
		return "GET_PEER_RECORD_RESPONSE"
	default:
		return "UNKNOWN"
	}
}

// ============================================================================
//                              消息结构
// ============================================================================

// Message DHT 消息
type Message struct {
	// Type 消息类型
	Type MessageType `json:"type"`

	// RequestID 请求 ID（用于匹配请求和响应）
	RequestID uint64 `json:"request_id"`

	// Sender 发送者节点 ID
	Sender types.NodeID `json:"sender"`

	// SenderAddrs 发送者地址列表
	SenderAddrs []string `json:"sender_addrs,omitempty"`

	// Target 目标节点 ID（用于 FIND_NODE）
	Target types.NodeID `json:"target,omitempty"`

	// Key 键（用于 FIND_VALUE/STORE/ADD_PROVIDER/GET_PROVIDERS/REMOVE_PROVIDER/PUT_PEER_RECORD/GET_PEER_RECORD）
	Key string `json:"key,omitempty"`

	// Value 值（用于 STORE/FIND_VALUE 响应）
	Value []byte `json:"value,omitempty"`

	// TTL 生存时间（秒，用于 STORE/ADD_PROVIDER/GET_PROVIDERS(ProviderRecord)）
	TTL uint32 `json:"ttl,omitempty"`

	// CloserPeers 更近的节点列表（用于响应）
	CloserPeers []PeerRecord `json:"closer_peers,omitempty"`

	// Providers Provider 列表（用于 GET_PROVIDERS 响应）
	Providers []PeerRecord `json:"providers,omitempty"`

	// SignedRecord 签名的节点记录（v2.0 新增，用于 PUT_PEER_RECORD/GET_PEER_RECORD）
	// 使用 base64 编码的序列化 SignedRealmPeerRecord
	SignedRecord []byte `json:"signed_record,omitempty"`

	// Success 操作是否成功
	Success bool `json:"success,omitempty"`

	// Error 错误信息
	Error string `json:"error,omitempty"`
}

// PeerRecord 节点记录（用于消息传输）
type PeerRecord struct {
	// ID 节点 ID
	ID types.NodeID `json:"id"`

	// Addrs 地址列表
	Addrs []string `json:"addrs"`

	// Timestamp 创建时间戳（Unix 纳秒）
	// 仅在 Providers 列表中使用；用于对端计算过期。
	Timestamp int64 `json:"timestamp,omitempty"`

	// TTL 生存时间（秒）
	// 仅在 Providers 列表中使用；与 Timestamp 配合得到过期时间。
	TTL uint32 `json:"ttl,omitempty"`
}

// ============================================================================
//                              消息编解码
// ============================================================================

// Encode 编码消息为字节数组
func (m *Message) Encode() ([]byte, error) {
	return json.Marshal(m)
}

// DecodeMessage 从字节数组解码消息
func DecodeMessage(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// ============================================================================
//                              请求构造器
// ============================================================================

// NewFindNodeRequest 创建 FIND_NODE 请求
func NewFindNodeRequest(requestID uint64, sender types.NodeID, senderAddrs []string, target types.NodeID) *Message {
	return &Message{
		Type:        MessageTypeFindNode,
		RequestID:   requestID,
		Sender:      sender,
		SenderAddrs: senderAddrs,
		Target:      target,
	}
}

// NewFindNodeResponse 创建 FIND_NODE 响应
func NewFindNodeResponse(requestID uint64, sender types.NodeID, closerPeers []PeerRecord) *Message {
	return &Message{
		Type:        MessageTypeFindNodeResponse,
		RequestID:   requestID,
		Sender:      sender,
		CloserPeers: closerPeers,
		Success:     true,
	}
}

// NewFindValueRequest 创建 FIND_VALUE 请求
func NewFindValueRequest(requestID uint64, sender types.NodeID, senderAddrs []string, key string) *Message {
	return &Message{
		Type:        MessageTypeFindValue,
		RequestID:   requestID,
		Sender:      sender,
		SenderAddrs: senderAddrs,
		Key:         key,
	}
}

// NewFindValueResponse 创建 FIND_VALUE 响应（找到值）
func NewFindValueResponse(requestID uint64, sender types.NodeID, value []byte) *Message {
	return &Message{
		Type:      MessageTypeFindValueResponse,
		RequestID: requestID,
		Sender:    sender,
		Value:     value,
		Success:   true,
	}
}

// NewFindValueResponseWithPeers 创建 FIND_VALUE 响应（返回更近节点）
func NewFindValueResponseWithPeers(requestID uint64, sender types.NodeID, closerPeers []PeerRecord) *Message {
	return &Message{
		Type:        MessageTypeFindValueResponse,
		RequestID:   requestID,
		Sender:      sender,
		CloserPeers: closerPeers,
		Success:     true,
	}
}

// NewStoreRequest 创建 STORE 请求
func NewStoreRequest(requestID uint64, sender types.NodeID, senderAddrs []string, key string, value []byte, ttlSeconds uint32) *Message {
	return &Message{
		Type:        MessageTypeStore,
		RequestID:   requestID,
		Sender:      sender,
		SenderAddrs: senderAddrs,
		Key:         key,
		Value:       value,
		TTL:         ttlSeconds,
	}
}

// NewStoreResponse 创建 STORE 响应
func NewStoreResponse(requestID uint64, sender types.NodeID, success bool, errMsg string) *Message {
	return &Message{
		Type:      MessageTypeStoreResponse,
		RequestID: requestID,
		Sender:    sender,
		Success:   success,
		Error:     errMsg,
	}
}

// NewPingRequest 创建 PING 请求
func NewPingRequest(requestID uint64, sender types.NodeID, senderAddrs []string) *Message {
	return &Message{
		Type:        MessageTypePing,
		RequestID:   requestID,
		Sender:      sender,
		SenderAddrs: senderAddrs,
	}
}

// NewPingResponse 创建 PING 响应
func NewPingResponse(requestID uint64, sender types.NodeID, senderAddrs []string) *Message {
	return &Message{
		Type:        MessageTypePingResponse,
		RequestID:   requestID,
		Sender:      sender,
		SenderAddrs: senderAddrs,
		Success:     true,
	}
}

// NewAddProviderRequest 创建 ADD_PROVIDER 请求
func NewAddProviderRequest(requestID uint64, sender types.NodeID, senderAddrs []string, key string, ttlSeconds uint32) *Message {
	return &Message{
		Type:        MessageTypeAddProvider,
		RequestID:   requestID,
		Sender:      sender,
		SenderAddrs: senderAddrs,
		Key:         key,
		TTL:         ttlSeconds,
	}
}

// NewAddProviderResponse 创建 ADD_PROVIDER 响应
func NewAddProviderResponse(requestID uint64, sender types.NodeID, success bool, errMsg string) *Message {
	return &Message{
		Type:      MessageTypeAddProviderResponse,
		RequestID: requestID,
		Sender:    sender,
		Success:   success,
		Error:     errMsg,
	}
}

// NewGetProvidersRequest 创建 GET_PROVIDERS 请求
func NewGetProvidersRequest(requestID uint64, sender types.NodeID, senderAddrs []string, key string) *Message {
	return &Message{
		Type:        MessageTypeGetProviders,
		RequestID:   requestID,
		Sender:      sender,
		SenderAddrs: senderAddrs,
		Key:         key,
	}
}

// NewGetProvidersResponse 创建 GET_PROVIDERS 响应
func NewGetProvidersResponse(requestID uint64, sender types.NodeID, providers []PeerRecord, closerPeers []PeerRecord) *Message {
	return &Message{
		Type:        MessageTypeGetProvidersResponse,
		RequestID:   requestID,
		Sender:      sender,
		Providers:   providers,
		CloserPeers: closerPeers,
		Success:     true,
	}
}

// NewRemoveProviderRequest 创建 REMOVE_PROVIDER 请求
func NewRemoveProviderRequest(requestID uint64, sender types.NodeID, senderAddrs []string, key string) *Message {
	return &Message{
		Type:        MessageTypeRemoveProvider,
		RequestID:   requestID,
		Sender:      sender,
		SenderAddrs: senderAddrs,
		Key:         key,
	}
}

// NewRemoveProviderResponse 创建 REMOVE_PROVIDER 响应
func NewRemoveProviderResponse(requestID uint64, sender types.NodeID, success bool, errMsg string) *Message {
	return &Message{
		Type:      MessageTypeRemoveProviderResponse,
		RequestID: requestID,
		Sender:    sender,
		Success:   success,
		Error:     errMsg,
	}
}

// NewErrorResponse 创建错误响应
func NewErrorResponse(requestID uint64, sender types.NodeID, msgType MessageType, errMsg string) *Message {
	// 响应类型 = 请求类型 + 1
	responseType := msgType + 1
	return &Message{
		Type:      responseType,
		RequestID: requestID,
		Sender:    sender,
		Success:   false,
		Error:     errMsg,
	}
}

// ============================================================================
//                              PeerRecord 请求构造器（v2.0 新增）
// ============================================================================

// NewPutPeerRecordRequest 创建 PUT_PEER_RECORD 请求
//
// 参数:
//   - requestID: 请求 ID
//   - sender: 发送者节点 ID
//   - senderAddrs: 发送者地址列表
//   - key: DHT Key（格式: /dep2p/v2/realm/{H(RealmID)}/peer/{NodeID}）
//   - signedRecord: 序列化的 SignedRealmPeerRecord
func NewPutPeerRecordRequest(requestID uint64, sender types.NodeID, senderAddrs []string, key string, signedRecord []byte) *Message {
	return &Message{
		Type:         MessageTypePutPeerRecord,
		RequestID:    requestID,
		Sender:       sender,
		SenderAddrs:  senderAddrs,
		Key:          key,
		SignedRecord: signedRecord,
	}
}

// NewPutPeerRecordResponse 创建 PUT_PEER_RECORD 响应
func NewPutPeerRecordResponse(requestID uint64, sender types.NodeID, success bool, errMsg string) *Message {
	return &Message{
		Type:      MessageTypePutPeerRecordResponse,
		RequestID: requestID,
		Sender:    sender,
		Success:   success,
		Error:     errMsg,
	}
}

// NewGetPeerRecordRequest 创建 GET_PEER_RECORD 请求
//
// 参数:
//   - requestID: 请求 ID
//   - sender: 发送者节点 ID
//   - senderAddrs: 发送者地址列表
//   - key: DHT Key（格式: /dep2p/v2/realm/{H(RealmID)}/peer/{NodeID}）
func NewGetPeerRecordRequest(requestID uint64, sender types.NodeID, senderAddrs []string, key string) *Message {
	return &Message{
		Type:        MessageTypeGetPeerRecord,
		RequestID:   requestID,
		Sender:      sender,
		SenderAddrs: senderAddrs,
		Key:         key,
	}
}

// NewGetPeerRecordResponse 创建 GET_PEER_RECORD 响应（找到记录）
func NewGetPeerRecordResponse(requestID uint64, sender types.NodeID, signedRecord []byte) *Message {
	return &Message{
		Type:         MessageTypeGetPeerRecordResponse,
		RequestID:    requestID,
		Sender:       sender,
		SignedRecord: signedRecord,
		Success:      true,
	}
}

// NewGetPeerRecordResponseWithPeers 创建 GET_PEER_RECORD 响应（返回更近节点）
func NewGetPeerRecordResponseWithPeers(requestID uint64, sender types.NodeID, closerPeers []PeerRecord) *Message {
	return &Message{
		Type:        MessageTypeGetPeerRecordResponse,
		RequestID:   requestID,
		Sender:      sender,
		CloserPeers: closerPeers,
		Success:     true,
	}
}
