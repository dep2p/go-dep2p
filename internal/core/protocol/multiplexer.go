// Package protocol 提供协议多路复用器实现
package protocol

import (
	"errors"
	"sync"
	"sync/atomic"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	protocolif "github.com/dep2p/go-dep2p/pkg/interfaces/protocol"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              错误定义
// ============================================================================

// 协议多路复用器相关错误
var (
	// ErrMultiplexerClosed 多路复用器已关闭
	ErrMultiplexerClosed = errors.New("multiplexer is closed")
	ErrProtocolNotAdded   = errors.New("protocol not added to multiplexer")
	ErrNegotiationFailed  = errors.New("protocol negotiation failed")
)

// ============================================================================
//                              Multiplexer 实现
// ============================================================================

// Multiplexer 协议多路复用器
//
// 在单个流/连接上支持多个协议
type Multiplexer struct {
	protocols  map[types.ProtocolID]protocolif.Protocol
	negotiator *Negotiator
	registry   *Registry

	mu     sync.RWMutex
	closed int32
}

// NewMultiplexer 创建多路复用器
func NewMultiplexer(negotiator *Negotiator, registry *Registry) *Multiplexer {
	return &Multiplexer{
		protocols:  make(map[types.ProtocolID]protocolif.Protocol),
		negotiator: negotiator,
		registry:   registry,
	}
}

// 确保实现接口
var _ protocolif.Multiplexer = (*Multiplexer)(nil)

// AddProtocol 添加协议
func (m *Multiplexer) AddProtocol(protocol protocolif.Protocol) {
	if atomic.LoadInt32(&m.closed) == 1 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.protocols[protocol.ID()] = protocol

	log.Debug("添加协议到多路复用器",
		"protocol", string(protocol.ID()))
}

// RemoveProtocol 移除协议
func (m *Multiplexer) RemoveProtocol(protocolID types.ProtocolID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.protocols, protocolID)

	log.Debug("从多路复用器移除协议",
		"protocol", string(protocolID))
}

// HandleStream 处理流
func (m *Multiplexer) HandleStream(stream endpoint.Stream) error {
	if atomic.LoadInt32(&m.closed) == 1 {
		return ErrMultiplexerClosed
	}

	// 获取流的协议 ID
	protocolID := stream.ProtocolID()

	// 如果已经有协议 ID，直接路由
	if protocolID != "" {
		return m.routeToProtocol(stream, protocolID)
	}

	// 否则进行协商
	if m.negotiator != nil {
		negotiatedProto, err := m.negotiator.HandleIncoming(stream)
		if err != nil {
			return err
		}

		return m.routeToProtocol(stream, negotiatedProto)
	}

	return ErrNegotiationFailed
}

// routeToProtocol 路由到协议处理器
func (m *Multiplexer) routeToProtocol(stream endpoint.Stream, protocolID types.ProtocolID) error {
	m.mu.RLock()
	protocol, ok := m.protocols[protocolID]
	m.mu.RUnlock()

	if !ok {
		// 尝试从注册表查找
		if m.registry != nil {
			handler, found := m.registry.MatchHandler(protocolID)
			if found {
				handler(stream)
				return nil
			}
		}
		return ErrProtocolNotAdded
	}

	return protocol.Handle(stream)
}

// Protocols 返回协议列表
func (m *Multiplexer) Protocols() []types.ProtocolID {
	m.mu.RLock()
	defer m.mu.RUnlock()

	protocols := make([]types.ProtocolID, 0, len(m.protocols))
	for id := range m.protocols {
		protocols = append(protocols, id)
	}
	return protocols
}

// HasProtocol 检查是否支持协议
func (m *Multiplexer) HasProtocol(protocolID types.ProtocolID) bool {
	m.mu.RLock()
	_, ok := m.protocols[protocolID]
	m.mu.RUnlock()

	if ok {
		return true
	}

	// 检查注册表
	if m.registry != nil {
		_, found := m.registry.MatchHandler(protocolID)
		return found
	}

	return false
}

// Close 关闭多路复用器
func (m *Multiplexer) Close() error {
	if !atomic.CompareAndSwapInt32(&m.closed, 0, 1) {
		return nil
	}

	m.mu.Lock()
	m.protocols = make(map[types.ProtocolID]protocolif.Protocol)
	m.mu.Unlock()

	log.Debug("关闭协议多路复用器")
	return nil
}

// ============================================================================
//                              StreamMux 流级多路复用
// ============================================================================

// StreamMux 流级协议多路复用
//
// 支持在单个流上复用多个子协议
type StreamMux struct {
	base       endpoint.Stream
	protocols  map[types.ProtocolID]*subStream
	nextID     uint32
	negotiator *Negotiator

	mu     sync.RWMutex
	closed int32
}

// subStream 子流
type subStream struct {
	id       uint32
	protocol types.ProtocolID
	buffer   []byte
	closed   bool
}

// NewStreamMux 创建流级多路复用器
func NewStreamMux(base endpoint.Stream, negotiator *Negotiator) *StreamMux {
	return &StreamMux{
		base:       base,
		protocols:  make(map[types.ProtocolID]*subStream),
		negotiator: negotiator,
	}
}

// OpenSubStream 打开子流
func (sm *StreamMux) OpenSubStream(protocolID types.ProtocolID) (*subStream, error) {
	if atomic.LoadInt32(&sm.closed) == 1 {
		return nil, ErrMultiplexerClosed
	}

	sm.mu.Lock()
	defer sm.mu.Unlock()

	// 检查是否已存在
	if ss, ok := sm.protocols[protocolID]; ok {
		return ss, nil
	}

	// 创建新子流
	sm.nextID++
	ss := &subStream{
		id:       sm.nextID,
		protocol: protocolID,
		buffer:   make([]byte, 0),
	}

	sm.protocols[protocolID] = ss

	log.Debug("打开子流",
		"protocol", string(protocolID),
		"id", ss.id)

	return ss, nil
}

// CloseSubStream 关闭子流
func (sm *StreamMux) CloseSubStream(protocolID types.ProtocolID) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if ss, ok := sm.protocols[protocolID]; ok {
		ss.closed = true
		delete(sm.protocols, protocolID)

		log.Debug("关闭子流",
			"protocol", string(protocolID))
	}
}

// Close 关闭流多路复用器
func (sm *StreamMux) Close() error {
	if !atomic.CompareAndSwapInt32(&sm.closed, 0, 1) {
		return nil
	}

	sm.mu.Lock()
	for _, ss := range sm.protocols {
		ss.closed = true
	}
	sm.protocols = make(map[types.ProtocolID]*subStream)
	sm.mu.Unlock()

	// 检查 base 是否为 nil
	if sm.base == nil {
		return nil
	}
	return sm.base.Close()
}

// ============================================================================
//                              Protocol Switch
// ============================================================================

// ProtocolSwitch 协议切换器
//
// 支持在连接生命周期内动态切换协议
type ProtocolSwitch struct {
	router     *Router
	negotiator *Negotiator
	current    types.ProtocolID
	mu         sync.RWMutex
}

// NewProtocolSwitch 创建协议切换器
func NewProtocolSwitch(router *Router, negotiator *Negotiator) *ProtocolSwitch {
	return &ProtocolSwitch{
		router:     router,
		negotiator: negotiator,
	}
}

// Switch 切换协议
func (ps *ProtocolSwitch) Switch(stream endpoint.Stream, newProtocol types.ProtocolID) error {
	ps.mu.Lock()
	defer ps.mu.Unlock()

	// 检查是否支持新协议
	if !ps.router.HasProtocol(newProtocol) {
		return ErrProtocolNotAdded
	}

	oldProtocol := ps.current
	ps.current = newProtocol

	log.Debug("切换协议",
		"from", string(oldProtocol),
		"to", string(newProtocol))

	// 路由到新协议处理器
	return ps.router.Handle(stream)
}

// Current 返回当前协议
func (ps *ProtocolSwitch) Current() types.ProtocolID {
	ps.mu.RLock()
	defer ps.mu.RUnlock()
	return ps.current
}

// ============================================================================
//                              Protocol Wrapper
// ============================================================================

// ProtocolWrapper 协议包装器
//
// 包装一个处理函数为 Protocol 接口
type ProtocolWrapper struct {
	id      types.ProtocolID
	handler endpoint.ProtocolHandler
}

// NewProtocolWrapper 创建协议包装器
func NewProtocolWrapper(id types.ProtocolID, handler endpoint.ProtocolHandler) *ProtocolWrapper {
	return &ProtocolWrapper{
		id:      id,
		handler: handler,
	}
}

// ID 返回协议 ID
func (pw *ProtocolWrapper) ID() types.ProtocolID {
	return pw.id
}

// Handle 处理流
func (pw *ProtocolWrapper) Handle(stream endpoint.Stream) error {
	if pw.handler != nil {
		pw.handler(stream)
	}
	return nil
}

// 确保实现 Protocol 接口
var _ protocolif.Protocol = (*ProtocolWrapper)(nil)

