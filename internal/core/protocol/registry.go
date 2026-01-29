package protocol

import (
	"sync"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
)

// Registry 协议注册表
type Registry struct {
	mu       sync.RWMutex
	handlers map[pkgif.ProtocolID]pkgif.StreamHandler
	matchers []matcher // 模式匹配器
}

// matcher 模式匹配器
type matcher struct {
	protocol pkgif.ProtocolID
	match    func(pkgif.ProtocolID) bool
	handler  pkgif.StreamHandler
}

var _ pkgif.ProtocolRegistry = (*Registry)(nil)

// NewRegistry 创建协议注册表
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[pkgif.ProtocolID]pkgif.StreamHandler),
		matchers: make([]matcher, 0),
	}
}

// Register 注册协议处理器
func (r *Registry) Register(protocolID pkgif.ProtocolID, handler pkgif.StreamHandler) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 检查是否已注册
	if _, exists := r.handlers[protocolID]; exists {
		return ErrDuplicateProtocol
	}

	r.handlers[protocolID] = handler
	return nil
}

// Unregister 注销协议处理器
func (r *Registry) Unregister(protocolID pkgif.ProtocolID) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 检查是否存在
	if _, exists := r.handlers[protocolID]; !exists {
		return ErrProtocolNotRegistered
	}

	delete(r.handlers, protocolID)
	return nil
}

// GetHandler 获取协议处理器
func (r *Registry) GetHandler(protocolID pkgif.ProtocolID) (pkgif.StreamHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 精确匹配
	if handler, ok := r.handlers[protocolID]; ok {
		return handler, true
	}

	// 模式匹配
	for _, m := range r.matchers {
		if m.match(protocolID) {
			return m.handler, true
		}
	}

	return nil, false
}

// Protocols 返回所有已注册的协议
func (r *Registry) Protocols() []pkgif.ProtocolID {
	r.mu.RLock()
	defer r.mu.RUnlock()

	protocols := make([]pkgif.ProtocolID, 0, len(r.handlers))
	for id := range r.handlers {
		protocols = append(protocols, id)
	}

	return protocols
}

// AddMatcher 添加模式匹配器
func (r *Registry) AddMatcher(protocol pkgif.ProtocolID, match func(pkgif.ProtocolID) bool, handler pkgif.StreamHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.matchers = append(r.matchers, matcher{
		protocol: protocol,
		match:    match,
		handler:  handler,
	})
}

// RemoveMatcher 移除模式匹配器
func (r *Registry) RemoveMatcher(protocol pkgif.ProtocolID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for i, m := range r.matchers {
		if m.protocol == protocol {
			r.matchers = append(r.matchers[:i], r.matchers[i+1:]...)
			return
		}
	}
}

// Clear 清空所有注册（用于测试）
func (r *Registry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.handlers = make(map[pkgif.ProtocolID]pkgif.StreamHandler)
	r.matchers = make([]matcher, 0)
}
