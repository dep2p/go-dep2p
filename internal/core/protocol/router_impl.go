// Package protocol 提供协议管理模块的实现
package protocol

import (
	"strings"
	"sync"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	protocolif "github.com/dep2p/go-dep2p/pkg/interfaces/protocol"
	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Handler 条目
// ============================================================================

// handlerEntry 处理器条目
type handlerEntry struct {
	handler endpoint.ProtocolHandler
	match   protocolif.MatchFunc
}

// ============================================================================
//                              Router 实现
// ============================================================================

// Router 协议路由器实现
type Router struct {
	handlers map[types.ProtocolID]*handlerEntry
	matchers []matcherEntry // 带匹配函数的处理器
	mu       sync.RWMutex
}

// matcherEntry 匹配器条目
type matcherEntry struct {
	baseProtocol types.ProtocolID
	match        protocolif.MatchFunc
	handler      endpoint.ProtocolHandler
}

// NewRouter 创建路由器
func NewRouter() *Router {
	return &Router{
		handlers: make(map[types.ProtocolID]*handlerEntry),
		matchers: make([]matcherEntry, 0),
	}
}

// 确保实现接口
var _ protocolif.Router = (*Router)(nil)

// AddHandler 添加处理器
func (r *Router) AddHandler(protocol types.ProtocolID, handler endpoint.ProtocolHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.handlers[protocol] = &handlerEntry{
		handler: handler,
		match:   nil,
	}

	log.Debug("注册协议处理器",
		"protocol", string(protocol))
}

// AddHandlerWithMatch 添加带匹配函数的处理器
func (r *Router) AddHandlerWithMatch(protocol types.ProtocolID, match protocolif.MatchFunc, handler endpoint.ProtocolHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 存储到精确匹配映射
	r.handlers[protocol] = &handlerEntry{
		handler: handler,
		match:   match,
	}

	// 同时存储到匹配器列表，用于模糊匹配
	r.matchers = append(r.matchers, matcherEntry{
		baseProtocol: protocol,
		match:        match,
		handler:      handler,
	})

	log.Debug("注册带匹配函数的协议处理器",
		"protocol", string(protocol))
}

// RemoveHandler 移除处理器
func (r *Router) RemoveHandler(protocol types.ProtocolID) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.handlers, protocol)

	// 从匹配器列表中移除
	newMatchers := make([]matcherEntry, 0, len(r.matchers))
	for _, m := range r.matchers {
		if m.baseProtocol != protocol {
			newMatchers = append(newMatchers, m)
		}
	}
	r.matchers = newMatchers

	log.Debug("移除协议处理器",
		"protocol", string(protocol))
}

// Handle 处理流
//
// v1.1 变更: 强制隔离检查点 #2
//   - 系统协议（/dep2p/sys/...）无需 Realm 验证
//   - 非系统协议需要连接已通过 RealmAuth 验证
func (r *Router) Handle(stream endpoint.Stream) error {
	if stream == nil {
		return ErrStreamClosed
	}

	if stream.IsClosed() {
		return ErrStreamClosed
	}

	protocolID := stream.ProtocolID()
	if protocolID == "" {
		return ErrProtocolInvalid
	}

	// 🔒 强制隔离检查点 #2: Protocol Router
	// 非系统协议需要验证 RealmContext
	if !isSystemProtocol(protocolID) {
		conn := stream.Connection()
		if conn == nil {
			log.Warn("流无连接信息",
				"protocol", string(protocolID))
			_ = stream.Close()
			return ErrRealmAuthRequired
		}

		realmCtx := conn.RealmContext()
		if realmCtx == nil || !realmCtx.IsValid() {
			log.Warn("非成员尝试访问业务协议",
				"protocol", string(protocolID),
				"remote", conn.RemoteID().ShortString())
			_ = stream.Close()
			return ErrRealmAuthRequired
		}
	}

	// 查找处理器
	handler, err := r.findHandler(protocolID)
	if err != nil {
		log.Warn("未找到协议处理器",
			"protocol", string(protocolID))
		return err
	}

	log.Debug("分发流到处理器",
		"protocol", string(protocolID),
		"streamID", uint64(stream.ID()))

	// 使用 recover 保护处理器执行，panic 后返回错误
	var handlerErr error
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error("处理器 panic",
					"protocol", string(protocolID),
					"panic", rec)
				handlerErr = ErrHandlerPanic
			}
		}()
		handler(stream)
	}()

	return handlerErr
}

// isSystemProtocol 判断是否为系统协议
//
// v1.1 新增: 系统协议以 /dep2p/sys/ 开头，无需 Realm 验证
// 引用 pkg/protocolids.SysPrefix 唯一真源
func isSystemProtocol(protocolID types.ProtocolID) bool {
	return strings.HasPrefix(string(protocolID), protocolids.SysPrefix)
}

// findHandler 查找处理器
func (r *Router) findHandler(protocolID types.ProtocolID) (endpoint.ProtocolHandler, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 1. 精确匹配
	if entry, ok := r.handlers[protocolID]; ok {
		return entry.handler, nil
	}

	// 2. 使用匹配函数匹配
	for _, m := range r.matchers {
		if m.match != nil && m.match(protocolID) {
			return m.handler, nil
		}
	}

	// 3. 语义版本匹配（例如 /echo/1.0.0 匹配 /echo/1.x）
	handler := r.findSemanticMatch(protocolID)
	if handler != nil {
		return handler, nil
	}

	return nil, ErrNoHandler
}

// findSemanticMatch 语义版本匹配
func (r *Router) findSemanticMatch(protocolID types.ProtocolID) endpoint.ProtocolHandler {
	// 提取协议基础名称（去掉版本号）
	protoStr := string(protocolID)
	parts := strings.Split(protoStr, "/")
	if len(parts) < 2 {
		return nil
	}

	// 获取协议名称部分（不含版本）
	// 例如 /echo/1.0.0 -> /echo
	baseParts := parts[:len(parts)-1]
	baseName := strings.Join(baseParts, "/")

	// 提取请求的主版本号
	requestedVersion := parts[len(parts)-1]
	requestedMajor := extractMajorVersion(requestedVersion)

	// 查找兼容的处理器
	for registeredProto, entry := range r.handlers {
		regStr := string(registeredProto)
		regParts := strings.Split(regStr, "/")
		if len(regParts) < 2 {
			continue
		}

		// 检查基础名称是否匹配
		regBaseParts := regParts[:len(regParts)-1]
		regBaseName := strings.Join(regBaseParts, "/")
		if regBaseName != baseName {
			continue
		}

		// 检查主版本号是否兼容
		regVersion := regParts[len(regParts)-1]
		regMajor := extractMajorVersion(regVersion)
		if regMajor == requestedMajor {
			return entry.handler
		}
	}

	return nil
}

// extractMajorVersion 提取主版本号
func extractMajorVersion(version string) string {
	// 处理如 "1.0.0", "1.0", "1" 等格式
	parts := strings.Split(version, ".")
	if len(parts) > 0 {
		return parts[0]
	}
	return version
}

// Protocols 返回支持的协议
func (r *Router) Protocols() []types.ProtocolID {
	r.mu.RLock()
	defer r.mu.RUnlock()

	protocols := make([]types.ProtocolID, 0, len(r.handlers))
	for p := range r.handlers {
		protocols = append(protocols, p)
	}
	return protocols
}

// HasProtocol 检查是否支持指定协议
func (r *Router) HasProtocol(protocolID types.ProtocolID) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 精确匹配
	if _, ok := r.handlers[protocolID]; ok {
		return true
	}

	// 匹配函数检查
	for _, m := range r.matchers {
		if m.match != nil && m.match(protocolID) {
			return true
		}
	}

	return false
}

// GetHandler 获取指定协议的处理器
func (r *Router) GetHandler(protocolID types.ProtocolID) (endpoint.ProtocolHandler, bool) {
	handler, err := r.findHandler(protocolID)
	if err != nil {
		return nil, false
	}
	return handler, true
}

// Clear 清空所有处理器
func (r *Router) Clear() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.handlers = make(map[types.ProtocolID]*handlerEntry)
	r.matchers = make([]matcherEntry, 0)

	log.Debug("清空所有协议处理器")
}

// Count 返回注册的协议数量
func (r *Router) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.handlers)
}

