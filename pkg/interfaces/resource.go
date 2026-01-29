// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 ResourceManager 接口，管理系统资源。
package interfaces

import "github.com/dep2p/go-dep2p/pkg/types"

// ResourceManager 定义资源管理器接口
//
// ResourceManager 控制内存、连接数、流数和带宽等资源限制。
type ResourceManager interface {
	// ViewSystem 查看系统级资源使用
	ViewSystem(func(ResourceScope) error) error

	// ViewTransient 查看临时资源使用
	ViewTransient(func(ResourceScope) error) error

	// ViewService 查看服务级资源使用
	ViewService(service string, fn func(ServiceScope) error) error

	// ViewProtocol 查看协议级资源使用
	ViewProtocol(proto types.ProtocolID, fn func(ProtocolScope) error) error

	// ViewPeer 查看节点级资源使用
	ViewPeer(peer types.PeerID, fn func(PeerScope) error) error

	// OpenConnection 打开连接，申请资源
	OpenConnection(dir Direction, usefd bool, endpoint types.Multiaddr) (ConnManagementScope, error)

	// OpenStream 打开流，申请资源
	OpenStream(peer types.PeerID, dir Direction) (StreamManagementScope, error)

	// Close 关闭资源管理器
	Close() error
}

// ResourceScope 定义资源范围接口
type ResourceScope interface {
	// Stat 返回当前资源统计
	Stat() ScopeStat

	// BeginSpan 开始资源追踪跨度
	BeginSpan() (ResourceScopeSpan, error)
}

// ResourceScopeSpan 定义资源跨度接口
type ResourceScopeSpan interface {
	ResourceScope

	// Done 结束跨度，释放资源
	Done()

	// ReserveMemory 预留内存
	ReserveMemory(size int, prio uint8) error

	// ReleaseMemory 释放内存
	ReleaseMemory(size int)
}

// ServiceScope 定义服务资源范围
type ServiceScope interface {
	ResourceScope

	// Name 返回服务名称
	Name() string
}

// PeerScope 定义节点资源范围
type PeerScope interface {
	ResourceScope

	// Peer 返回节点 ID
	Peer() types.PeerID
}

// ProtocolScope 定义协议资源范围
type ProtocolScope interface {
	ResourceScope

	// Protocol 返回协议 ID
	Protocol() types.ProtocolID
}

// ConnManagementScope 定义连接管理资源范围
type ConnManagementScope interface {
	ResourceScopeSpan

	// PeerScope 返回关联的节点资源范围
	PeerScope() PeerScope

	// SetPeer 设置连接的节点
	SetPeer(peer types.PeerID) error

	// ProtectPeer 保护节点
	ProtectPeer(peer types.PeerID)
}

// ConnectionScope 定义连接资源范围（用户视图）
type ConnectionScope interface {
	ResourceScope
}

// StreamManagementScope 定义流管理资源范围
type StreamManagementScope interface {
	ResourceScopeSpan

	// ProtocolScope 返回关联的协议资源范围
	ProtocolScope() ProtocolScope

	// PeerScope 返回关联的节点资源范围
	PeerScope() PeerScope

	// ServiceScope 返回关联的服务资源范围
	ServiceScope() ServiceScope

	// SetProtocol 设置流协议
	SetProtocol(proto types.ProtocolID) error

	// SetService 设置流服务
	SetService(service string) error
}

// StreamScope 定义流资源范围（用户视图）
type StreamScope interface {
	ResourceScope
}

// ScopeStat 资源统计
type ScopeStat struct {
	NumStreamsInbound  int
	NumStreamsOutbound int
	NumConnsInbound    int
	NumConnsOutbound   int
	NumFD              int
	Memory             int64
}

// Direction 连接/流方向
type Direction int

const (
	// DirUnknown 未知方向
	DirUnknown Direction = iota
	// DirInbound 入站
	DirInbound
	// DirOutbound 出站
	DirOutbound
)

// Limit 资源限制
type Limit struct {
	Streams         int   // 最大流数
	StreamsInbound  int   // 最大入站流数
	StreamsOutbound int   // 最大出站流数
	Conns           int   // 最大连接数
	ConnsInbound    int   // 最大入站连接数
	ConnsOutbound   int   // 最大出站连接数
	FD              int   // 最大文件描述符数
	Memory          int64 // 最大内存（字节）
}

// LimitConfig 限制配置
type LimitConfig struct {
	System              Limit // 系统级限制
	Transient           Limit // 临时资源限制
	ServiceDefault      Limit // 服务默认限制
	ServicePeerDefault  Limit // 服务-节点默认限制
	ProtocolDefault     Limit // 协议默认限制
	ProtocolPeerDefault Limit // 协议-节点默认限制
	PeerDefault         Limit // 节点默认限制
	Conn                Limit // 连接限制
	Stream              Limit // 流限制
}

// 预留优先级常量
const (
	// ReservationPriorityLow 低优先级预留（可用内存 <= 40% 时失败）
	ReservationPriorityLow uint8 = 101

	// ReservationPriorityMedium 中优先级预留（可用内存 <= 60% 时失败）
	ReservationPriorityMedium uint8 = 152

	// ReservationPriorityHigh 高优先级预留（可用内存 <= 80% 时失败）
	ReservationPriorityHigh uint8 = 203

	// ReservationPriorityAlways 始终预留（只要有资源就成功）
	ReservationPriorityAlways uint8 = 255
)
