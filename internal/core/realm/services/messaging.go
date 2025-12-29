// Package services 提供 Realm Layer 3 服务适配器
package services

import (
	"context"
	"fmt"

	realmif "github.com/dep2p/go-dep2p/pkg/interfaces/realm"
	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Messaging 服务适配器（IMPL-1227）
// ============================================================================

// RealmProvider 提供 Realm 信息的接口
type RealmProvider interface {
	ID() types.RealmID
	Key() types.RealmKey
}

// MessagingUpstream 底层消息服务接口
//
// 定义 Messaging 适配器需要的底层服务接口
type MessagingUpstream interface {
	// Send 发送消息
	Send(ctx context.Context, to types.NodeID, protocol types.ProtocolID, data []byte) error

	// Request 发送请求并等待响应
	Request(ctx context.Context, to types.NodeID, protocol types.ProtocolID, data []byte) ([]byte, error)

	// RegisterHandler 注册协议处理器
	RegisterHandler(protocol types.ProtocolID, handler func(from types.NodeID, data []byte) ([]byte, error))

	// UnregisterHandler 注销协议处理器
	UnregisterHandler(protocol types.ProtocolID)
}

// RealmMessaging Realm 消息服务适配器
//
// 实现 realmif.Messaging 接口，封装底层消息服务并自动添加 Realm 协议前缀。
type RealmMessaging struct {
	realm    RealmProvider
	upstream MessagingUpstream

	// 默认消息协议（相对名称）
	defaultProtocol string

	// 已注册的处理器（用于清理）
	registeredHandlers map[string]types.ProtocolID
}

// NewRealmMessaging 创建 Realm 消息服务适配器
func NewRealmMessaging(realm RealmProvider, upstream MessagingUpstream) *RealmMessaging {
	return &RealmMessaging{
		realm:              realm,
		upstream:           upstream,
		defaultProtocol:    "messaging/1.0.0",
		registeredHandlers: make(map[string]types.ProtocolID),
	}
}

// ============================================================================
//                              realmif.Messaging 实现
// ============================================================================

// Send 发送消息（使用默认协议）
func (m *RealmMessaging) Send(ctx context.Context, to types.NodeID, data []byte) error {
	return m.SendWithProtocol(ctx, to, m.defaultProtocol, data)
}

// SendWithProtocol 发送消息（指定协议）
//
// 用户只需写相对协议名（如 "chat/1.0.0"），
// 框架自动转换为 "/dep2p/app/<realmID>/chat/1.0.0"
func (m *RealmMessaging) SendWithProtocol(ctx context.Context, to types.NodeID, protocol string, data []byte) error {
	// 验证用户协议
	if err := protocolids.ValidateUserProtocol(protocol, m.realm.ID()); err != nil {
		return fmt.Errorf("invalid protocol: %w", err)
	}

	// 自动添加 Realm 前缀
	fullProto := protocolids.FullAppProtocol(m.realm.ID(), protocol)

	return m.upstream.Send(ctx, to, fullProto, data)
}

// Request 发送请求并等待响应（使用默认协议）
func (m *RealmMessaging) Request(ctx context.Context, to types.NodeID, data []byte) ([]byte, error) {
	return m.RequestWithProtocol(ctx, to, m.defaultProtocol, data)
}

// RequestWithProtocol 发送请求并等待响应（指定协议）
func (m *RealmMessaging) RequestWithProtocol(ctx context.Context, to types.NodeID, protocol string, data []byte) ([]byte, error) {
	// 验证用户协议
	if err := protocolids.ValidateUserProtocol(protocol, m.realm.ID()); err != nil {
		return nil, fmt.Errorf("invalid protocol: %w", err)
	}

	// 自动添加 Realm 前缀
	fullProto := protocolids.FullAppProtocol(m.realm.ID(), protocol)

	return m.upstream.Request(ctx, to, fullProto, data)
}

// OnMessage 注册默认消息处理器
func (m *RealmMessaging) OnMessage(handler realmif.MessageHandler) {
	m.OnProtocol(m.defaultProtocol, func(from types.NodeID, protocol string, data []byte) ([]byte, error) {
		handler(from, data)
		return nil, nil
	})
}

// OnRequest 注册默认请求处理器
func (m *RealmMessaging) OnRequest(handler realmif.RequestHandler) {
	m.OnProtocol(m.defaultProtocol, func(from types.NodeID, protocol string, data []byte) ([]byte, error) {
		return handler(from, data)
	})
}

// OnProtocol 注册自定义协议处理器
func (m *RealmMessaging) OnProtocol(protocol string, handler realmif.ProtocolHandler) {
	// 验证用户协议
	if err := protocolids.ValidateUserProtocol(protocol, m.realm.ID()); err != nil {
		return // 静默忽略无效协议
	}

	// 自动添加 Realm 前缀
	fullProto := protocolids.FullAppProtocol(m.realm.ID(), protocol)

	// 记录已注册的处理器
	m.registeredHandlers[protocol] = fullProto

	// 注册到底层服务
	m.upstream.RegisterHandler(fullProto, func(from types.NodeID, data []byte) ([]byte, error) {
		return handler(from, protocol, data)
	})
}

// Close 清理资源
func (m *RealmMessaging) Close() {
	for _, fullProto := range m.registeredHandlers {
		m.upstream.UnregisterHandler(fullProto)
	}
	m.registeredHandlers = make(map[string]types.ProtocolID)
}

