package protocol

import (
	"strings"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("core/protocol")

// Router 协议路由器
type Router struct {
	registry   *Registry
	negotiator *Negotiator
}

var _ pkgif.ProtocolRouter = (*Router)(nil)

// NewRouter 创建协议路由器
func NewRouter(registry *Registry, negotiator *Negotiator) *Router {
	return &Router{
		registry:   registry,
		negotiator: negotiator,
	}
}

// Route 路由流到对应的协议处理器
func (r *Router) Route(stream pkgif.Stream) error {
	// 获取流的协议 ID
	protocolID := pkgif.ProtocolID(stream.Protocol())

	logger.Debug("路由流", "protocolID", string(protocolID))

	// 从注册表获取处理器
	handler, ok := r.registry.GetHandler(protocolID)
	if !ok {
		logger.Warn("未找到协议处理器", "protocolID", string(protocolID))
		return ErrNoHandler
	}

	// 调用处理器
	handler(stream)

	logger.Debug("流路由完成", "protocolID", string(protocolID))
	return nil
}

// AddRoute 添加路由规则
func (r *Router) AddRoute(pattern string, handler pkgif.StreamHandler) error {
	logger.Debug("添加路由", "pattern", pattern)
	
	// 如果不是模式（不含通配符），使用精确匹配
	if !strings.Contains(pattern, "*") {
		if err := r.registry.Register(pkgif.ProtocolID(pattern), handler); err != nil {
			logger.Warn("注册路由失败", "pattern", pattern, "error", err)
			return err
		}
		logger.Debug("路由注册成功", "pattern", pattern)
		return nil
	}

	// 使用模式匹配
	matchFunc := createMatcher(pattern)
	r.registry.AddMatcher(pkgif.ProtocolID(pattern), matchFunc, handler)
	logger.Debug("模式路由添加成功", "pattern", pattern)

	return nil
}

// RemoveRoute 移除路由规则
func (r *Router) RemoveRoute(pattern string) error {
	logger.Debug("移除路由", "pattern", pattern)
	
	// 如果不是模式，使用精确注销
	if !strings.Contains(pattern, "*") {
		if err := r.registry.Unregister(pkgif.ProtocolID(pattern)); err != nil {
			logger.Warn("注销路由失败", "pattern", pattern, "error", err)
			return err
		}
		logger.Debug("路由注销成功", "pattern", pattern)
		return nil
	}

	// 移除模式匹配器
	r.registry.RemoveMatcher(pkgif.ProtocolID(pattern))
	logger.Debug("模式路由移除成功", "pattern", pattern)

	return nil
}

// createMatcher 创建模式匹配函数
func createMatcher(pattern string) func(pkgif.ProtocolID) bool {
	return func(protocolID pkgif.ProtocolID) bool {
		// 简单的通配符匹配
		// "/test/*" 匹配 "/test/v1", "/test/v2" 等
		
		parts := strings.Split(pattern, "*")
		if len(parts) == 0 {
			return false
		}

		str := string(protocolID)
		
		// 检查前缀
		if !strings.HasPrefix(str, parts[0]) {
			return false
		}

		// 如果有后缀，检查后缀
		if len(parts) > 1 && parts[1] != "" {
			if !strings.HasSuffix(str, parts[1]) {
				return false
			}
		}

		return true
	}
}
