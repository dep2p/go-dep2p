// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 Host 接口，提供核心主机功能。
package interfaces

import (
	"context"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// Host 定义 P2P 主机接口
//
// Host 是 P2P 网络的核心抽象，负责协议注册和流处理。
//
// v2.0 API 透明性保证：
// - Connect, NewStream 等方法对 Relay 实现透明
// - 用户只需提供 peerID，底层自动选择最佳路径（直连 > 打洞 > 中继）
// - 如需获取连接类型，可通过 Connection.ConnType() 查询（仅供调试）
type Host interface {
	// ID 返回主机的 PeerID
	ID() string

	// Addrs 返回主机监听的地址列表
	Addrs() []string

	// AdvertisedAddrs 返回对外公告地址
	//
	// 整合多个地址来源，按优先级排序：
	//   1. 已验证的直连地址（来自 Reachability Coordinator）
	//   2. Relay 地址（来自 Reachability Coordinator）
	//   3. 监听地址（作为兜底）
	//
	// 返回的地址格式包含 /p2p/<peerID> 后缀。
	AdvertisedAddrs() []string

	// ShareableAddrs 返回可分享的公网地址
	//
	// 只返回已验证的公网地址（不包含私网地址和 0.0.0.0）。
	// 适用于需要分享给其他节点连接时使用。
	ShareableAddrs() []string

	// HolePunchAddrs 返回用于打洞协商的地址列表
	//
	// 
	// 而不仅仅是已验证的地址。对于 NAT 节点，dial-back 验证无法成功，
	// 但 STUN 候选地址是真实的外部地址，是打洞必需的。
	//
	// 地址优先级：
	//   1. STUN/UPnP/NAT-PMP 发现的候选地址（★ 打洞核心）
	//   2. 已验证的直连地址
	//   3. Relay 地址（仅用于信令，不应作为打洞目标）
	//
	// 注意：返回的地址不包含 Relay 地址。打洞的目标是建立直连，
	// 如果只有 Relay 地址，说明 STUN 发现失败，应该使用 Relay 兜底。
	HolePunchAddrs() []string

	// Listen 监听指定地址
	//
	// addrs 是 multiaddr 格式的地址列表，例如：
	//   - "/ip4/0.0.0.0/udp/4001/quic-v1"
	//   - "/ip4/0.0.0.0/tcp/4001"
	Listen(addrs ...string) error

	// Connect 连接到指定节点
	Connect(ctx context.Context, peerID string, addrs []string) error

	// SetStreamHandler 为指定协议设置流处理器
	SetStreamHandler(protocolID string, handler StreamHandler)

	// RemoveStreamHandler 移除指定协议的流处理器
	RemoveStreamHandler(protocolID string)

	// NewStream 创建到指定节点的新流
	NewStream(ctx context.Context, peerID string, protocolIDs ...string) (Stream, error)

	// Peerstore 返回节点存储
	Peerstore() Peerstore

	// EventBus 返回事件总线
	EventBus() EventBus

	// Network 返回底层 Swarm（网络层）
	//
	// 用于访问底层连接管理功能，如检查连接状态。
	// P2 修复：GossipSub 需要检查节点连接状态以清理断开的 Mesh 成员。
	Network() Swarm

	// SetReachabilityCoordinator 设置可达性协调器
	//
	// 用于获取已验证的外部地址和 Relay 地址。
	SetReachabilityCoordinator(coordinator ReachabilityCoordinator)

	// Close 关闭主机
	Close() error

	// HandleInboundStream 处理入站流
	//
	// 对流进行协议协商并路由到相应处理器。
	// 用于处理非标准来源的入站流（如中继连接的 STOP 流）。
	//
	// 
	// 后续的协议协商请求需要通过此方法路由到正确的处理器。
	HandleInboundStream(stream Stream)
}

// StreamHandler 定义流处理函数类型
type StreamHandler func(Stream)

// Stream 定义双向流接口
type Stream interface {
	// Read 从流中读取数据
	Read(p []byte) (n int, err error)

	// Write 向流中写入数据
	Write(p []byte) (n int, err error)

	// Close 关闭流
	Close() error

	// CloseWrite 关闭写端（半关闭）
	//
	// 关闭后无法继续写入，但仍可读取。
	// 发送 FIN 信号告知对方"我已发送完毕"。
	// 对于 QUIC，这是发送 STREAM 帧的 FIN 位。
	CloseWrite() error

	// CloseRead 关闭读端（半关闭）
	//
	// 关闭后无法继续读取，但仍可写入。
	// 告知传输层不再需要接收数据。
	CloseRead() error

	// Reset 重置流（异常关闭）
	Reset() error

	// SetDeadline 设置读写超时
	//
	// 设置读和写操作的截止时间。
	// 超时后，Read 和 Write 会返回错误。
	// 传入零值 time.Time{} 表示不超时。
	SetDeadline(t time.Time) error

	// SetReadDeadline 设置读超时
	SetReadDeadline(t time.Time) error

	// SetWriteDeadline 设置写超时
	SetWriteDeadline(t time.Time) error

	// Protocol 返回流使用的协议 ID
	Protocol() string

	// SetProtocol 设置流使用的协议 ID（协议协商时使用）
	SetProtocol(protocol string)

	// Conn 返回底层连接
	Conn() Connection

	// IsClosed 检查流是否已关闭
	//
	// 返回 true 表示流已关闭（通过 Close、Reset 或底层传输关闭）。
	// 可用于在操作前检查流状态，避免对已关闭流的操作。
	IsClosed() bool

	// Stat 返回流统计信息
	//
	// 返回流的统计信息，包括方向、打开时间、协议、字节数等。
	Stat() types.StreamStat

	// State 返回流当前状态
	//
	// 返回流的当前状态，用于跟踪流的半关闭状态。
	// 状态包括：Open、ReadClosed、WriteClosed、Closed、Reset。
	State() types.StreamState
}
