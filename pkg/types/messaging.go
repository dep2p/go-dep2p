package types

import "time"

// ============================================================================
//                              Request - 请求消息
// ============================================================================

// Request 请求消息
// 用于请求-响应模式
type Request struct {
	// ID 请求 ID（用于关联响应）
	ID uint64

	// Protocol 协议
	Protocol ProtocolID

	// Data 请求数据
	Data []byte

	// From 发送者节点 ID
	From NodeID
}

// ============================================================================
//                              Response - 响应消息
// ============================================================================

// Response 响应消息
type Response struct {
	// Status 状态码（0 表示成功）
	Status uint16

	// Data 响应数据
	Data []byte

	// Error 错误信息（如果失败）
	Error string
}

// IsSuccess 检查响应是否成功
func (r Response) IsSuccess() bool {
	return r.Status == 0 && r.Error == ""
}

// ============================================================================
//                              QueryResponse - 查询响应
// ============================================================================

// QueryResponse 查询响应
// 用于 Query/QueryAll 模式
type QueryResponse struct {
	// From 响应者节点 ID
	From NodeID

	// Data 响应数据
	Data []byte

	// Latency 响应延迟
	Latency time.Duration
}

// ============================================================================
//                              Message - 发布订阅消息
// ============================================================================

// Message 发布订阅消息
type Message struct {
	// ID 消息 ID（用于去重）
	ID []byte

	// Topic 主题
	Topic string

	// From 发送者节点 ID
	From NodeID

	// Data 消息内容
	Data []byte

	// Timestamp 时间戳
	Timestamp time.Time

	// Sequence 序列号
	Sequence uint64

	// Signature 可选签名（用于消息验证）
	Signature []byte

	// Key 签名者公钥（可选，用于验证签名）
	Key []byte

	// KeyType 签名者公钥类型（Ed25519/ECDSA-P256/ECDSA-P384）
	// 默认 KeyTypeUnknown，通过 Key 字段长度自动推断（向后兼容）
	KeyType KeyType

	// ReceivedFrom 接收来源节点（非消息原始发送者）
	ReceivedFrom NodeID

	// ==================== Query 相关字段 ====================

	// IsQuery 标识是否为查询消息
	// 查询消息会被 QueryHandler 处理，响应通过点对点发送
	IsQuery bool

	// QueryID 查询唯一标识符
	// 用于关联查询和响应
	QueryID string

	// ReplyTo 响应目标节点 ID
	// 订阅者处理查询后，将响应发送到此节点
	ReplyTo NodeID
}

// Age 返回消息年龄
func (m Message) Age() time.Duration {
	return time.Since(m.Timestamp)
}

// IsSigned 检查消息是否已签名
func (m Message) IsSigned() bool {
	return len(m.Signature) > 0
}

// IsQueryMessage 检查是否为查询消息
func (m Message) IsQueryMessage() bool {
	return m.IsQuery && m.QueryID != ""
}

// NeedsReply 检查消息是否需要回复
func (m Message) NeedsReply() bool {
	return m.IsQuery && m.ReplyTo != EmptyNodeID
}

