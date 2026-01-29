// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 Protocol 相关接口，管理协议注册和路由。
package interfaces

import (
	"context"
)

// ProtocolID 协议标识符类型
//
// 协议常量定义在 pkg/types/protocol.go 中，避免重复定义。
type ProtocolID string

// ProtocolRegistry 定义协议注册表接口
type ProtocolRegistry interface {
	// Register 注册协议处理器
	Register(protocolID ProtocolID, handler StreamHandler) error

	// Unregister 注销协议处理器
	Unregister(protocolID ProtocolID) error

	// GetHandler 获取协议处理器
	GetHandler(protocolID ProtocolID) (StreamHandler, bool)

	// Protocols 返回所有已注册的协议
	Protocols() []ProtocolID
}

// ProtocolRouter 定义协议路由器接口
type ProtocolRouter interface {
	// Route 路由到合适的协议处理器
	Route(stream Stream) error

	// AddRoute 添加路由规则
	AddRoute(pattern string, handler StreamHandler) error

	// RemoveRoute 移除路由规则
	RemoveRoute(pattern string) error
}

// ProtocolNegotiator 定义协议协商器接口
type ProtocolNegotiator interface {
	// Negotiate 协商协议
	Negotiate(ctx context.Context, conn Connection, protocols []ProtocolID) (ProtocolID, error)

	// Handle 处理入站协议协商
	Handle(ctx context.Context, conn Connection) (ProtocolID, error)
}

// RealmProtocolID 生成 Realm 协议 ID
func RealmProtocolID(realmID, protocol, version string) ProtocolID {
	return ProtocolID("/dep2p/realm/" + realmID + "/" + protocol + "/" + version)
}

// AppProtocolID 生成应用协议 ID
func AppProtocolID(realmID, protocol, version string) ProtocolID {
	return ProtocolID("/dep2p/app/" + realmID + "/" + protocol + "/" + version)
}
