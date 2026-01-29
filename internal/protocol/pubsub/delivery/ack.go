// Package delivery 提供可靠消息投递功能
//
// IMPL-NETWORK-RESILIENCE Phase 4: ACK 确认协议
package delivery

import (
	"encoding/json"
	"time"

	"github.com/dep2p/go-dep2p/pkg/protocol"
)

// ============================================================================
//                              ACK 协议定义
// ============================================================================

// AckProtocolID ACK 协议标识（使用统一定义）
// 用于消息可靠投递的确认协议
var AckProtocolID = string(protocol.DeliveryAck)

// AckMessageType ACK 消息类型
type AckMessageType int

const (
	// AckTypeConfirm 确认消息已收到
	AckTypeConfirm AckMessageType = iota

	// AckTypeReject 拒绝/无法处理
	AckTypeReject

	// AckTypeRequest 请求重发
	AckTypeRequest
)

// String 返回类型字符串
func (t AckMessageType) String() string {
	switch t {
	case AckTypeConfirm:
		return "confirm"
	case AckTypeReject:
		return "reject"
	case AckTypeRequest:
		return "request"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              ACK 消息
// ============================================================================

// AckMessage ACK 消息结构
type AckMessage struct {
	// MessageID 原消息 ID
	MessageID string

	// AckerID 确认者节点 ID
	AckerID string

	// Topic 消息主题
	Topic string

	// Timestamp 确认时间
	Timestamp time.Time
}

// AckMessageWire ACK 消息线上格式
type AckMessageWire struct {
	// Version 协议版本
	Version uint8 `json:"v"`

	// Type 消息类型
	Type AckMessageType `json:"t"`

	// MessageID 原消息 ID
	MessageID string `json:"mid"`

	// AckerID 确认者节点 ID
	AckerID string `json:"aid"`

	// Topic 消息主题
	Topic string `json:"topic,omitempty"`

	// Timestamp 确认时间戳（Unix 毫秒）
	Timestamp int64 `json:"ts"`

	// Extra 扩展数据
	Extra map[string]string `json:"extra,omitempty"`
}

// MarshalAckMessage 序列化 ACK 消息
func MarshalAckMessage(msg *AckMessage) ([]byte, error) {
	wire := &AckMessageWire{
		Version:   1,
		Type:      AckTypeConfirm,
		MessageID: msg.MessageID,
		AckerID:   msg.AckerID,
		Topic:     msg.Topic,
		Timestamp: msg.Timestamp.UnixMilli(),
	}
	return json.Marshal(wire)
}

// UnmarshalAckMessage 反序列化 ACK 消息
func UnmarshalAckMessage(data []byte) (*AckMessage, error) {
	var wire AckMessageWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, err
	}

	return &AckMessage{
		MessageID: wire.MessageID,
		AckerID:   wire.AckerID,
		Topic:     wire.Topic,
		Timestamp: time.UnixMilli(wire.Timestamp),
	}, nil
}

// ============================================================================
//                              ACK 请求消息
// ============================================================================

// AckRequest ACK 请求（发送方请求接收方 ACK）
type AckRequest struct {
	// MessageID 消息 ID
	MessageID string

	// RequesterID 请求者节点 ID
	RequesterID string

	// Topic 消息主题
	Topic string

	// Timestamp 请求时间
	Timestamp time.Time
}

// AckRequestWire ACK 请求线上格式
type AckRequestWire struct {
	Version     uint8  `json:"v"`
	MessageID   string `json:"mid"`
	RequesterID string `json:"rid"`
	Topic       string `json:"topic"`
	Timestamp   int64  `json:"ts"`
}

// MarshalAckRequest 序列化 ACK 请求
func MarshalAckRequest(req *AckRequest) ([]byte, error) {
	wire := &AckRequestWire{
		Version:     1,
		MessageID:   req.MessageID,
		RequesterID: req.RequesterID,
		Topic:       req.Topic,
		Timestamp:   req.Timestamp.UnixMilli(),
	}
	return json.Marshal(wire)
}

// UnmarshalAckRequest 反序列化 ACK 请求
func UnmarshalAckRequest(data []byte) (*AckRequest, error) {
	var wire AckRequestWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, err
	}

	return &AckRequest{
		MessageID:   wire.MessageID,
		RequesterID: wire.RequesterID,
		Topic:       wire.Topic,
		Timestamp:   time.UnixMilli(wire.Timestamp),
	}, nil
}

// ============================================================================
//                              ACK 元数据
// ============================================================================

// AckMetadataKey GossipSub 消息中的 ACK 元数据键
const AckMetadataKey = "ack_request"

// ExtractAckRequest 从消息数据中提取 ACK 请求
//
// 消息格式: [ack_request_len(2bytes)][ack_request][payload]
func ExtractAckRequest(data []byte) (*AckRequest, []byte, error) {
	if len(data) < 2 {
		// 没有 ACK 请求前缀，返回原数据
		return nil, data, nil
	}

	// 读取 ACK 请求长度（大端序）
	ackLen := int(data[0])<<8 | int(data[1])

	if ackLen == 0 {
		// 长度为 0，表示没有 ACK 请求
		return nil, data[2:], nil
	}

	if len(data) < 2+ackLen {
		// 数据不完整
		return nil, data, nil
	}

	// 解析 ACK 请求
	req, err := UnmarshalAckRequest(data[2 : 2+ackLen])
	if err != nil {
		return nil, data, err
	}

	// 返回 ACK 请求和实际 payload
	return req, data[2+ackLen:], nil
}

// PrependAckRequest 在消息前添加 ACK 请求
//
// 消息格式: [ack_request_len(2bytes)][ack_request][payload]
func PrependAckRequest(req *AckRequest, payload []byte) ([]byte, error) {
	if req == nil {
		// 无 ACK 请求，添加长度 0 前缀
		result := make([]byte, 2+len(payload))
		result[0] = 0
		result[1] = 0
		copy(result[2:], payload)
		return result, nil
	}

	// 序列化 ACK 请求
	ackData, err := MarshalAckRequest(req)
	if err != nil {
		return nil, err
	}

	if len(ackData) > 65535 {
		return nil, &DeliveryError{Message: "ack request too large"}
	}

	// 构建结果
	result := make([]byte, 2+len(ackData)+len(payload))
	result[0] = byte(len(ackData) >> 8)
	result[1] = byte(len(ackData))
	copy(result[2:], ackData)
	copy(result[2+len(ackData):], payload)

	return result, nil
}

// ============================================================================
//                              PendingAck
// ============================================================================

// PendingAck 等待 ACK 的消息
type PendingAck struct {
	// MessageID 消息 ID
	MessageID string

	// Topic 消息主题
	Topic string

	// Data 消息内容（用于重发）
	Data []byte

	// RequiredAcks 需要 ACK 的节点列表
	RequiredAcks []string

	// ReceivedAcks 已收到的 ACK
	// nodeID -> 收到时间
	ReceivedAcks map[string]time.Time

	// CreatedAt 创建时间
	CreatedAt time.Time

	// Attempts 重试次数
	Attempts int

	// Done 完成信号
	Done chan struct{}

	// RequireAll 是否要求所有节点确认
	RequireAll bool
}

// NewPendingAck 创建等待 ACK 的消息
func NewPendingAck(msgID, topic string, data []byte, requiredAcks []string, requireAll bool) *PendingAck {
	return &PendingAck{
		MessageID:    msgID,
		Topic:        topic,
		Data:         data,
		RequiredAcks: requiredAcks,
		ReceivedAcks: make(map[string]time.Time),
		CreatedAt:    time.Now(),
		Done:         make(chan struct{}),
		RequireAll:   requireAll,
	}
}

// AddAck 添加收到的 ACK
// 返回是否已满足 ACK 要求
func (p *PendingAck) AddAck(nodeID string) bool {
	// 检查是否是期望的节点
	isExpected := false
	for _, n := range p.RequiredAcks {
		if n == nodeID {
			isExpected = true
			break
		}
	}

	if !isExpected {
		return false
	}

	// 记录 ACK
	if _, exists := p.ReceivedAcks[nodeID]; !exists {
		p.ReceivedAcks[nodeID] = time.Now()
	}

	return p.IsComplete()
}

// IsComplete 检查是否已完成
func (p *PendingAck) IsComplete() bool {
	if p.RequireAll {
		// 要求所有节点确认
		return len(p.ReceivedAcks) >= len(p.RequiredAcks)
	}
	// 任意一个节点确认即可
	return len(p.ReceivedAcks) > 0
}

// GetResult 获取 ACK 结果
func (p *PendingAck) GetResult() *AckResult {
	result := &AckResult{
		AckedBy:     make([]string, 0, len(p.ReceivedAcks)),
		MissingAcks: make([]string, 0),
	}

	for nodeID := range p.ReceivedAcks {
		result.AckedBy = append(result.AckedBy, nodeID)
	}

	for _, nodeID := range p.RequiredAcks {
		if _, acked := p.ReceivedAcks[nodeID]; !acked {
			result.MissingAcks = append(result.MissingAcks, nodeID)
		}
	}

	result.Success = p.IsComplete()
	if !result.Success {
		result.Error = ErrAckTimeout
	}

	return result
}

// AckResult ACK 结果
type AckResult struct {
	// Success 是否成功
	Success bool

	// AckedBy 确认的节点列表
	AckedBy []string

	// MissingAcks 未确认的节点列表
	MissingAcks []string

	// Error 错误信息
	Error error
}
