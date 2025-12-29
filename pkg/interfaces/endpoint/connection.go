package endpoint

import (
	"context"
	"time"
)

// ============================================================================
//                              Connection 接口
// ============================================================================

// Connection 表示与远程节点的连接
//
// Connection 是两个节点之间的安全通信链路，基于 QUIC 协议。
// 它支持在单个连接上创建多个独立的流（多路复用）。
//
// 使用示例:
//
//	conn, _ := endpoint.Connect(ctx, remoteNodeID)
//	defer conn.Close()
//
//	// 打开流
//	stream, _ := conn.OpenStream(ctx, "/chat/1.0")
//	stream.Write([]byte("Hello"))
type Connection interface {
	// ==================== 对端信息 ====================

	// RemoteID 返回远程节点 ID
	RemoteID() NodeID

	// RemotePublicKey 返回远程节点公钥
	RemotePublicKey() PublicKey

	// RemoteAddrs 返回远程节点地址
	// 可能包含多个地址（公网、私网、中继等）
	RemoteAddrs() []Address

	// LocalID 返回本地节点 ID
	LocalID() NodeID

	// LocalAddrs 返回本地地址
	LocalAddrs() []Address

	// ==================== 流管理 ====================

	// OpenStream 打开一个新流
	//
	// 创建一个到远程节点的新流，用于指定协议的通信。
	// 流是双向的，可以同时读写。
	//
	// 示例:
	//   stream, _ := conn.OpenStream(ctx, "/file-transfer/1.0")
	//   stream.Write(fileData)
	OpenStream(ctx context.Context, protocolID ProtocolID) (Stream, error)

	// OpenStreamWithPriority 打开指定优先级的流
	OpenStreamWithPriority(ctx context.Context, protocolID ProtocolID, priority Priority) (Stream, error)

	// AcceptStream 接受一个新流
	//
	// 阻塞等待直到远程节点打开新流或连接关闭。
	// 通常在独立的 goroutine 中循环调用。
	AcceptStream(ctx context.Context) (Stream, error)

	// Streams 返回所有活跃流
	Streams() []Stream

	// StreamCount 返回当前流数量
	StreamCount() int

	// ==================== 连接信息 ====================

	// Stats 返回连接统计
	Stats() ConnectionStats

	// Direction 返回连接方向
	// DirInbound 表示入站连接（被动接受）
	// DirOutbound 表示出站连接（主动发起）
	Direction() Direction

	// Transport 返回底层传输协议名称
	// 如 "quic", "tcp" 等
	Transport() string

	// ==================== 中继信息 ====================

	// IsRelayed 返回连接是否通过中继建立
	//
	// 如果连接是通过 Relay Transport 建立的，返回 true。
	// 应用可以使用此方法判断连接类型并做出相应处理。
	//
	// 示例:
	//   conn, _ := endpoint.Connect(ctx, remoteNodeID)
	//   if conn.IsRelayed() {
	//       fmt.Println("连接通过中继建立")
	//   }
	IsRelayed() bool

	// RelayID 返回中继节点 ID
	//
	// 如果连接是通过中继建立的，返回中继节点的 NodeID。
	// 如果是直连，返回空 NodeID。
	//
	// 示例:
	//   if conn.IsRelayed() {
	//       fmt.Printf("中继节点: %s\n", conn.RelayID().ShortString())
	//   }
	RelayID() NodeID

	// ==================== 生命周期 ====================

	// Close 关闭连接
	//
	// 优雅关闭连接，等待数据发送完成。
	// 所有关联的流也会被关闭。
	Close() error

	// CloseWithError 带错误码关闭连接
	//
	// 立即关闭连接并向对端发送错误码。
	// 用于异常情况下的快速关闭。
	CloseWithError(code uint32, reason string) error

	// IsClosed 检查连接是否已关闭
	IsClosed() bool

	// Done 返回连接关闭的通道
	//
	// 当连接关闭时，此通道会被关闭。
	// 可用于监听连接状态。
	//
	// 示例:
	//   select {
	//   case <-conn.Done():
	//       fmt.Println("connection closed")
	//   case <-ctx.Done():
	//       fmt.Println("context canceled")
	//   }
	Done() <-chan struct{}

	// Context 返回连接的上下文
	//
	// 当连接关闭时，此上下文会被取消。
	Context() context.Context

	// ==================== 扩展功能 ====================

	// SetStreamHandler 设置连接级别的流处理器
	//
	// 为此连接设置特定协议的处理器，优先级高于 Endpoint 级别的处理器。
	SetStreamHandler(protocolID ProtocolID, handler ProtocolHandler)

	// RemoveStreamHandler 移除连接级别的流处理器
	RemoveStreamHandler(protocolID ProtocolID)

	// ==================== Realm 上下文 (v1.1 新增) ====================

	// RealmContext 返回连接级 Realm 上下文
	//
	// 返回通过 RealmAuth 协议验证后设置的 Realm 上下文。
	// 如果连接尚未进行 Realm 验证，返回 nil。
	RealmContext() *RealmContext

	// SetRealmContext 设置连接级 Realm 上下文
	//
	// 由 RealmAuth 协议处理器在验证成功后调用。
	// 设置后，该连接上的非系统协议流才能被 Router 接受。
	SetRealmContext(ctx *RealmContext)
}

// ============================================================================
//                              Realm 上下文 (v1.1 新增)
// ============================================================================

// RealmContext 连接级 Realm 上下文
//
// 存储通过 RealmAuth 协议验证后的 Realm 信息。
// 用于 Protocol Router 判断是否允许非系统协议流。
type RealmContext struct {
	// RealmID 验证通过的 Realm 标识
	RealmID string

	// Verified 是否已通过 RealmAuth 验证
	Verified bool

	// ExpiresAt 验证过期时间
	// 超过此时间后需要重新验证
	ExpiresAt time.Time
}

// IsValid 检查 RealmContext 是否有效
func (rc *RealmContext) IsValid() bool {
	if rc == nil {
		return false
	}
	if !rc.Verified {
		return false
	}
	if !rc.ExpiresAt.IsZero() && time.Now().After(rc.ExpiresAt) {
		return false
	}
	return true
}

// ============================================================================
//                              连接状态
// ============================================================================

// ConnectionState 连接状态
type ConnectionState int

const (
	// ConnectionStateConnecting 正在连接
	ConnectionStateConnecting ConnectionState = iota
	// ConnectionStateSecuring 正在进行安全握手
	ConnectionStateSecuring
	// ConnectionStateConnected 已连接
	ConnectionStateConnected
	// ConnectionStateClosing 正在关闭
	ConnectionStateClosing
	// ConnectionStateClosed 已关闭
	ConnectionStateClosed
)

// String 返回状态的字符串表示
func (s ConnectionState) String() string {
	switch s {
	case ConnectionStateConnecting:
		return "connecting"
	case ConnectionStateSecuring:
		return "securing"
	case ConnectionStateConnected:
		return "connected"
	case ConnectionStateClosing:
		return "closing"
	case ConnectionStateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              连接事件
// ============================================================================

// ConnectionEvent 连接事件类型
const (
	// EventConnectionOpened 连接已建立
	EventConnectionOpened = "connection.opened"
	// EventConnectionClosed 连接已关闭
	EventConnectionClosed = "connection.closed"
	// EventConnectionFailed 连接失败
	EventConnectionFailed = "connection.failed"
)

// ConnectionOpenedEvent 连接建立事件
type ConnectionOpenedEvent struct {
	Connection Connection
	Direction  Direction
}

// Type 返回事件类型
func (e ConnectionOpenedEvent) Type() string {
	return EventConnectionOpened
}

// ConnectionClosedEvent 连接关闭事件
type ConnectionClosedEvent struct {
	// Connection 已关闭的连接
	Connection Connection
	// Reason 关闭原因
	Reason error
	// IsRelayConn 是否为 relay 中继连接
	// 当此字段为 true 时，表示这是一个通过 relay 中继建立的连接
	// 应用层可以根据此字段决定是否触发重新预留等操作
	IsRelayConn bool
	// RelayID relay 节点 ID（如果 IsRelayConn 为 true）
	// 可用于识别具体哪个 relay 节点的连接断开
	RelayID NodeID
}

// Type 返回事件类型
func (e ConnectionClosedEvent) Type() string {
	return EventConnectionClosed
}

// ConnectionFailedEvent 连接失败事件
type ConnectionFailedEvent struct {
	NodeID NodeID
	Addrs  []Address
	Error  error
}

// Type 返回事件类型
func (e ConnectionFailedEvent) Type() string {
	return EventConnectionFailed
}
