// Package streams 实现流协议
package streams

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("protocol/streams")

// Service 实现 Streams 接口
type Service struct {
	host     interfaces.Host
	realmMgr interfaces.RealmManager // 可选，用于全局模式
	realm    interfaces.Realm        // 可选，用于 Realm 绑定模式
	realmID  string                  // 绑定的 RealmID（如果有）

	mu       sync.RWMutex
	handlers map[string]interfaces.BiStreamHandler
	started  bool
	ctx      context.Context
	cancel   context.CancelFunc

	// 配置
	config *Config
}

// 确保 Service 实现了 interfaces.Streams 接口
var _ interfaces.Streams = (*Service)(nil)

// New 创建 Streams 服务（全局模式）
func New(host interfaces.Host, realmMgr interfaces.RealmManager, opts ...Option) (*Service, error) {
	if host == nil {
		return nil, ErrNilHost
	}
	// RealmManager 现在是可选的
	// 如果 realmMgr == nil，某些 Realm 相关功能将被禁用

	// 应用配置
	config := DefaultConfig()
	for _, opt := range opts {
		opt(config)
	}

	s := &Service{
		host:     host,
		realmMgr: realmMgr,
		handlers: make(map[string]interfaces.BiStreamHandler),
		config:   config,
	}

	return s, nil
}

// NewForRealm 创建绑定到特定 Realm 的 Streams 服务
//
// 与全局模式不同，此构造函数将服务绑定到特定的 Realm：
// - 协议 ID 包含 RealmID: /dep2p/app/<realmID>/streams/1.0.0
// - 只处理该 Realm 的流
// - 成员验证基于绑定的 Realm
func NewForRealm(host interfaces.Host, realm interfaces.Realm, opts ...Option) (*Service, error) {
	if host == nil {
		return nil, ErrNilHost
	}
	if realm == nil {
		return nil, fmt.Errorf("realm is required for NewForRealm")
	}

	// 应用配置
	config := DefaultConfig()
	for _, opt := range opts {
		opt(config)
	}

	s := &Service{
		host:     host,
		realm:    realm,
		realmID:  realm.ID(),
		handlers: make(map[string]interfaces.BiStreamHandler),
		config:   config,
	}

	return s, nil
}

// Start 启动服务
func (s *Service) Start(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return ErrAlreadyStarted
	}

	logger.Info("正在启动 Streams 服务")

	// 使用 context.Background() 而不是传入的 ctx
	// 因为 Fx OnStart 的 ctx 在返回后会被取消，导致后台循环提前退出
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.started = true

	logger.Info("Streams 服务启动成功")
	return nil
}

// Stop 停止服务
func (s *Service) Stop(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return ErrNotStarted
	}

	logger.Info("正在停止 Streams 服务")

	if s.cancel != nil {
		s.cancel()
	}

	s.started = false
	logger.Info("Streams 服务已停止")
	return nil
}

// Open 打开到指定节点的流
func (s *Service) Open(ctx context.Context, peerID string, protocol string) (interfaces.BiStream, error) {
	s.mu.RLock()
	if !s.started {
		s.mu.RUnlock()
		return nil, ErrNotStarted
	}
	
	logger.Debug("打开流", "peerID", log.TruncateID(peerID, 8), "protocol", protocol)
	s.mu.RUnlock()

	// 验证协议
	if err := validateProtocol(protocol); err != nil {
		return nil, err
	}

	// 验证节点ID
	if peerID == "" {
		return nil, ErrInvalidPeerID
	}

	// 确定使用的协议
	var fullProtocol string
	var found bool

	if s.realm != nil && s.realmID != "" {
		// Realm-bound 模式：直接使用绑定的 Realm
		if s.realm.IsMember(peerID) {
			fullProtocol = buildProtocolID(s.realmID, protocol)
			found = true
		}
	} else if s.realmMgr != nil {
		// 全局模式：查找包含该节点的 Realm
		realms := s.realmMgr.ListRealms()
		for _, realm := range realms {
			// 检查是否为Realm成员
			if realm.IsMember(peerID) {
				// 构建协议ID
				fullProtocol = buildProtocolID(realm.ID(), protocol)
				found = true
				break
			}
		}

		// 如果有默认Realm，也尝试使用它
		if !found && s.config.DefaultRealmID != "" {
			realm, ok := s.realmMgr.GetRealm(s.config.DefaultRealmID)
			if ok {
				fullProtocol = buildProtocolID(realm.ID(), protocol)
				found = true
			}
		}
	}

	if !found {
		return nil, ErrNoRealm
	}

	// 打开流
	stream, err := s.host.NewStream(ctx, peerID, fullProtocol)
	if err != nil {
		return nil, err
	}

	// 包装为 BiStream
	wrapper := newStreamWrapper(stream, fullProtocol)

	// 应用配置的超时
	s.applyStreamTimeouts(wrapper)

	return wrapper, nil
}

// applyStreamTimeouts 应用配置的流超时
//
// 如果配置了 ReadTimeout 或 WriteTimeout，自动设置到流上。
// 用户可以之后调用 SetDeadline/SetReadDeadline/SetWriteDeadline 覆盖。
func (s *Service) applyStreamTimeouts(wrapper *streamWrapper) {
	// 如果同时配置了读写超时，使用 SetDeadline 一次性设置
	// 否则分别设置
	if s.config.ReadTimeout > 0 && s.config.WriteTimeout > 0 {
		// 使用较短的超时作为统一超时
		timeout := s.config.ReadTimeout
		if s.config.WriteTimeout < timeout {
			timeout = s.config.WriteTimeout
		}
		wrapper.SetDeadline(time.Now().Add(timeout))
	} else {
		if s.config.ReadTimeout > 0 {
			wrapper.SetReadDeadline(time.Now().Add(s.config.ReadTimeout))
		}
		if s.config.WriteTimeout > 0 {
			wrapper.SetWriteDeadline(time.Now().Add(s.config.WriteTimeout))
		}
	}
}

// RegisterHandler 注册流处理器
func (s *Service) RegisterHandler(protocol string, handler interfaces.BiStreamHandler) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return ErrNotStarted
	}

	// 验证协议
	if err := validateProtocol(protocol); err != nil {
		return err
	}

	// 检查处理器是否已存在
	if _, exists := s.handlers[protocol]; exists {
		return ErrHandlerExists
	}

	// 保存处理器
	s.handlers[protocol] = handler

	// 根据模式注册处理器
	if s.realm != nil && s.realmID != "" {
		// Realm-bound 模式：只为绑定的 Realm 注册
		fullProtocol := buildProtocolID(s.realmID, protocol)
		s.registerHostHandler(fullProtocol, handler)
	} else if s.realmMgr != nil {
		// 全局模式：为所有 Realm 注册处理器
		realms := s.realmMgr.ListRealms()
		for _, realm := range realms {
			fullProtocol := buildProtocolID(realm.ID(), protocol)
			s.registerHostHandler(fullProtocol, handler)
		}

		// 如果有默认Realm，也注册
		if s.config.DefaultRealmID != "" {
			realm, ok := s.realmMgr.GetRealm(s.config.DefaultRealmID)
			if ok {
				fullProtocol := buildProtocolID(realm.ID(), protocol)
				s.registerHostHandler(fullProtocol, handler)
			}
		}
	}

	return nil
}

// registerHostHandler 在 Host 层注册处理器
func (s *Service) registerHostHandler(fullProtocol string, handler interfaces.BiStreamHandler) {
	// 将 BiStreamHandler 适配为 StreamHandler
	s.host.SetStreamHandler(fullProtocol, func(stream interfaces.Stream) {
		// 包装为 BiStream
		wrapper := newStreamWrapper(stream, fullProtocol)
		// 应用配置的超时（入站流也需要超时保护）
		s.applyStreamTimeouts(wrapper)
		// 调用用户处理器
		handler(wrapper)
	})
}

// UnregisterHandler 注销流处理器
func (s *Service) UnregisterHandler(protocol string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return ErrNotStarted
	}

	// 验证协议
	if err := validateProtocol(protocol); err != nil {
		return err
	}

	// 检查处理器是否存在
	if _, exists := s.handlers[protocol]; !exists {
		return ErrHandlerNotFound
	}

	// 移除处理器
	delete(s.handlers, protocol)

	// 根据模式移除处理器
	if s.realm != nil && s.realmID != "" {
		// Realm-bound 模式：只移除绑定的 Realm 的处理器
		fullProtocol := buildProtocolID(s.realmID, protocol)
		s.host.RemoveStreamHandler(fullProtocol)
	} else if s.realmMgr != nil {
		// 全局模式：从所有 Realm 移除处理器
		realms := s.realmMgr.ListRealms()
		for _, realm := range realms {
			fullProtocol := buildProtocolID(realm.ID(), protocol)
			s.host.RemoveStreamHandler(fullProtocol)
		}

		// 如果有默认Realm，也移除
		if s.config.DefaultRealmID != "" {
			realm, ok := s.realmMgr.GetRealm(s.config.DefaultRealmID)
			if ok {
				fullProtocol := buildProtocolID(realm.ID(), protocol)
				s.host.RemoveStreamHandler(fullProtocol)
			}
		}
	}

	return nil
}

// Close 关闭服务
func (s *Service) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}

	// 根据模式移除所有处理器
	for protocol := range s.handlers {
		if s.realm != nil && s.realmID != "" {
			// Realm-bound 模式：只移除绑定的 Realm 的处理器
			fullProtocol := buildProtocolID(s.realmID, protocol)
			s.host.RemoveStreamHandler(fullProtocol)
		} else if s.realmMgr != nil {
			// 全局模式：从所有 Realm 移除处理器
			realms := s.realmMgr.ListRealms()
			for _, realm := range realms {
				fullProtocol := buildProtocolID(realm.ID(), protocol)
				s.host.RemoveStreamHandler(fullProtocol)
			}
		}
	}

	s.handlers = make(map[string]interfaces.BiStreamHandler)
	s.started = false

	return nil
}
