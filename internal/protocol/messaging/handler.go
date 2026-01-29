// Package messaging 实现点对点消息传递协议
package messaging

import (
	"sync"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// HandlerRegistry 处理器注册表
type HandlerRegistry struct {
	mu       sync.RWMutex
	handlers map[string]interfaces.MessageHandler
}

// NewHandlerRegistry 创建处理器注册表
func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{
		handlers: make(map[string]interfaces.MessageHandler),
	}
}

// Register 注册处理器
func (r *HandlerRegistry) Register(protocol string, handler interfaces.MessageHandler) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.handlers[protocol]; exists {
		return ErrHandlerAlreadyRegistered
	}

	r.handlers[protocol] = handler
	return nil
}

// Unregister 注销处理器
func (r *HandlerRegistry) Unregister(protocol string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.handlers[protocol]; !exists {
		return ErrHandlerNotFound
	}

	delete(r.handlers, protocol)
	return nil
}

// Get 获取处理器
func (r *HandlerRegistry) Get(protocol string) (interfaces.MessageHandler, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	handler, exists := r.handlers[protocol]
	return handler, exists
}

// List 列出所有已注册的协议
func (r *HandlerRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	protocols := make([]string, 0, len(r.handlers))
	for protocol := range r.handlers {
		protocols = append(protocols, protocol)
	}
	return protocols
}

// Clear 清空所有处理器
func (r *HandlerRegistry) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.handlers = make(map[string]interfaces.MessageHandler)
}
