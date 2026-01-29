// Package delivery 提供可靠消息投递功能
//
// 测试辅助实现
package delivery

import (
	"context"
	"sync"
	"time"
)

// ============================================================================
//                              Mock Publisher
// ============================================================================

// MockPublisher 模拟发布器（用于测试）
type MockPublisher struct {
	mu sync.Mutex

	// PublishedMessages 已发布的消息记录
	PublishedMessages []*PublishedMessage

	// ShouldFail 是否返回错误
	ShouldFail bool

	// FailError 返回的错误
	FailError error

	// OnPublish 发布时的回调
	OnPublish func(topic string, data []byte)
}

// PublishedMessage 已发布的消息
type PublishedMessage struct {
	Topic     string
	Data      []byte
	Timestamp time.Time
}

// NewMockPublisher 创建模拟发布器
func NewMockPublisher() *MockPublisher {
	return &MockPublisher{
		PublishedMessages: make([]*PublishedMessage, 0),
	}
}

// Publish 记录发布请求
func (p *MockPublisher) Publish(_ context.Context, topic string, data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.ShouldFail {
		if p.FailError != nil {
			return p.FailError
		}
		return &DeliveryError{Message: "mock publish failed"}
	}

	msg := &PublishedMessage{
		Topic:     topic,
		Data:      data,
		Timestamp: time.Now(),
	}
	p.PublishedMessages = append(p.PublishedMessages, msg)

	if p.OnPublish != nil {
		p.OnPublish(topic, data)
	}

	return nil
}

// GetMessages 获取已发布的消息
func (p *MockPublisher) GetMessages() []*PublishedMessage {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := make([]*PublishedMessage, len(p.PublishedMessages))
	copy(result, p.PublishedMessages)
	return result
}

// Reset 重置状态
func (p *MockPublisher) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.PublishedMessages = make([]*PublishedMessage, 0)
	p.ShouldFail = false
	p.FailError = nil
}

// ============================================================================
//                              Mock ACK Handler
// ============================================================================

// MockAckHandler 模拟 ACK 处理器（用于测试）
type MockAckHandler struct {
	mu sync.Mutex

	// SentAcks 记录发送的 ACK
	SentAcks []*SentAckRecord

	// SendError 如果设置，SendAck 会返回此错误
	SendError error

	// OnSend 发送时的回调
	OnSend func(target string, ack *AckMessage)
}

// SentAckRecord 记录发送的 ACK
type SentAckRecord struct {
	Target    string
	Ack       *AckMessage
	Timestamp time.Time
}

// NewMockAckHandler 创建模拟 ACK 处理器
func NewMockAckHandler() *MockAckHandler {
	return &MockAckHandler{
		SentAcks: make([]*SentAckRecord, 0),
	}
}

// SendAck 记录发送请求
func (h *MockAckHandler) SendAck(_ context.Context, target string, ack *AckMessage) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.SendError != nil {
		return h.SendError
	}

	record := &SentAckRecord{
		Target:    target,
		Ack:       ack,
		Timestamp: time.Now(),
	}
	h.SentAcks = append(h.SentAcks, record)

	if h.OnSend != nil {
		h.OnSend(target, ack)
	}

	return nil
}

// GetSentAcks 获取发送记录
func (h *MockAckHandler) GetSentAcks() []*SentAckRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	result := make([]*SentAckRecord, len(h.SentAcks))
	copy(result, h.SentAcks)
	return result
}

// Reset 重置记录
func (h *MockAckHandler) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.SentAcks = make([]*SentAckRecord, 0)
	h.SendError = nil
}

// ============================================================================
//                              NoOp ACK Handler
// ============================================================================

// NoOpAckHandler 空操作 ACK 处理器
type NoOpAckHandler struct{}

// NewNoOpAckHandler 创建空操作 ACK 处理器
func NewNoOpAckHandler() *NoOpAckHandler {
	return &NoOpAckHandler{}
}

// SendAck 不执行任何操作
func (h *NoOpAckHandler) SendAck(_ context.Context, _ string, _ *AckMessage) error {
	return nil
}
