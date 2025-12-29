// Package protocol 定义协议管理相关接口
//
// 协议模块负责协议的注册、协商和路由，包括：
// - 协议注册
// - 协议协商
// - 流路由
package protocol

import (
	"context"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/interfaces/transport"
	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Protocol 接口
// ============================================================================

// Protocol 协议接口
//
// 定义一个 P2P 协议的行为。
type Protocol interface {
	// ID 返回协议 ID
	ID() types.ProtocolID

	// Handle 处理流
	//
	// 当收到此协议的入站流时被调用。
	Handle(stream endpoint.Stream) error
}

// ============================================================================
//                              Router 接口
// ============================================================================

// Router 协议路由器接口
//
// 管理协议处理器的注册和流的路由。
type Router interface {
	// AddHandler 添加协议处理器
	AddHandler(protocolID types.ProtocolID, handler endpoint.ProtocolHandler)

	// AddHandlerWithMatch 添加带匹配函数的处理器
	//
	// match 函数用于协议版本兼容等场景。
	AddHandlerWithMatch(protocolID types.ProtocolID, match MatchFunc, handler endpoint.ProtocolHandler)

	// RemoveHandler 移除协议处理器
	RemoveHandler(protocolID types.ProtocolID)

	// Handle 处理流
	//
	// 根据流的协议 ID 路由到对应的处理器。
	Handle(stream endpoint.Stream) error

	// Protocols 返回支持的协议列表
	Protocols() []types.ProtocolID

	// HasProtocol 检查是否支持指定协议
	HasProtocol(protocolID types.ProtocolID) bool
}

// MatchFunc 协议匹配函数
// 用于协议版本兼容等场景
type MatchFunc func(types.ProtocolID) bool

// ============================================================================
//                              Negotiator 接口
// ============================================================================

// Negotiator 协议协商器接口
//
// 负责在连接建立后协商使用的协议。
type Negotiator interface {
	// Negotiate 协商协议
	//
	// 在连接上协商协议，返回双方都支持的协议。
	Negotiate(conn transport.Conn, protocols []types.ProtocolID) (types.ProtocolID, error)

	// NegotiateWithPeer 与指定节点协商
	//
	// 连接到节点并协商协议。
	NegotiateWithPeer(ctx context.Context, peer types.NodeID, protocols []types.ProtocolID) (types.ProtocolID, error)

	// SelectProtocol 选择协议
	//
	// 从本地和远程支持的协议中选择最优协议。
	SelectProtocol(local, remote []types.ProtocolID) (types.ProtocolID, error)
}

// ============================================================================
//                              Multiplexer 接口
// ============================================================================

// Multiplexer 协议多路复用器接口
//
// 在单个流上支持多个协议。
type Multiplexer interface {
	// AddProtocol 添加协议
	AddProtocol(protocol Protocol)

	// RemoveProtocol 移除协议
	RemoveProtocol(protocolID types.ProtocolID)

	// HandleStream 处理流
	HandleStream(stream endpoint.Stream) error
}

// ============================================================================
//                              Registry 接口
// ============================================================================

// Registry 协议注册表接口
type Registry interface {
	// Register 注册协议
	Register(protocol Protocol) error

	// Unregister 注销协议
	Unregister(protocolID types.ProtocolID) error

	// Get 获取协议
	Get(protocolID types.ProtocolID) (Protocol, bool)

	// List 列出所有协议
	List() []Protocol

	// Match 匹配协议
	//
	// 返回匹配指定协议 ID 的协议列表。
	Match(protocolID types.ProtocolID) []Protocol
}

// ============================================================================
//                              内置协议
// ============================================================================

// 引用 pkg/protocolids 唯一真源
var (
	// ProtocolPing Ping 协议
	ProtocolPing = protocolids.SysPing

	// ProtocolIdentify 身份识别协议
	ProtocolIdentify = protocolids.SysIdentify

	// ProtocolRelay 中继协议
	ProtocolRelay = protocolids.SysRelay

	// ProtocolHolePunch 打洞协议
	ProtocolHolePunch = protocolids.SysHolepunch
)

// ============================================================================
//                              消息类型
// ============================================================================

// Message 协议消息接口
type Message interface {
	// Type 返回消息类型
	Type() string

	// Marshal 序列化消息
	Marshal() ([]byte, error)

	// Unmarshal 反序列化消息
	Unmarshal(data []byte) error
}

// ============================================================================
//                              配置
// ============================================================================

// Config 协议模块配置
type Config struct {
	// NegotiationTimeout 协商超时
	NegotiationTimeout int

	// MaxProtocols 最大协议数
	MaxProtocols int

	// EnableMultiplexing 启用协议多路复用
	EnableMultiplexing bool
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		NegotiationTimeout: 10,
		MaxProtocols:       100,
		EnableMultiplexing: true,
	}
}
