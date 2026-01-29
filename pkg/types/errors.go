// Package types 定义 DeP2P 的基础类型
//
// 本文件定义所有公共错误类型。
package types

import "errors"

// ============================================================================
//                              ID 相关错误
// ============================================================================

var (
	// ErrEmptyPeerID 空节点 ID
	ErrEmptyPeerID = errors.New("empty peer ID")

	// ErrInvalidPeerID 无效的节点 ID
	ErrInvalidPeerID = errors.New("invalid peer ID")

	// ErrEmptyRealmID 空 Realm ID
	ErrEmptyRealmID = errors.New("empty realm ID")

	// ErrInvalidRealmID 无效的 Realm ID
	ErrInvalidRealmID = errors.New("invalid realm ID")

	// ErrEmptyProtocolID 空协议 ID
	ErrEmptyProtocolID = errors.New("empty protocol ID")
)

// ============================================================================
//                              PSK 相关错误
// ============================================================================

var (
	// ErrEmptyPSK 空 PSK
	ErrEmptyPSK = errors.New("empty PSK")

	// ErrInvalidPSKLength PSK 长度无效
	ErrInvalidPSKLength = errors.New("invalid PSK length: must be 32 bytes")

	// ErrEmptyRealmKey 空 Realm 密钥
	ErrEmptyRealmKey = errors.New("empty realm key")

	// ErrInvalidRealmKey 无效的 Realm 密钥
	ErrInvalidRealmKey = errors.New("invalid realm key")
)

// ============================================================================
//                              连接相关错误
// ============================================================================

var (
	// ErrNotConnected 未连接
	ErrNotConnected = errors.New("not connected")

	// ErrConnectionClosed 连接已关闭
	ErrConnectionClosed = errors.New("connection closed")

	// ErrConnectionRefused 连接被拒绝
	ErrConnectionRefused = errors.New("connection refused")

	// ErrConnectionTimeout 连接超时
	ErrConnectionTimeout = errors.New("connection timeout")

	// ErrMaxConnectionsReached 达到最大连接数
	ErrMaxConnectionsReached = errors.New("max connections reached")
)

// ============================================================================
//                              流相关错误
// ============================================================================

var (
	// ErrStreamClosed 流已关闭
	ErrStreamClosed = errors.New("stream closed")

	// ErrStreamReset 流已重置
	ErrStreamReset = errors.New("stream reset")

	// ErrMaxStreamsReached 达到最大流数
	ErrMaxStreamsReached = errors.New("max streams reached")
)

// ============================================================================
//                              协议相关错误
// ============================================================================

var (
	// ErrProtocolNotSupported 协议不支持
	ErrProtocolNotSupported = errors.New("protocol not supported")

	// ErrProtocolNegotiationFailed 协议协商失败
	ErrProtocolNegotiationFailed = errors.New("protocol negotiation failed")

	// ErrNoProtocolHandler 无协议处理器
	ErrNoProtocolHandler = errors.New("no protocol handler")
)

// ============================================================================
//                              Realm 相关错误
// ============================================================================

var (
	// ErrRealmNotFound Realm 不存在
	ErrRealmNotFound = errors.New("realm not found")

	// ErrRealmExists Realm 已存在
	ErrRealmExists = errors.New("realm already exists")

	// ErrNotRealmMember 非 Realm 成员
	ErrNotRealmMember = errors.New("not a realm member")

	// ErrRealmAuthFailed Realm 认证失败
	ErrRealmAuthFailed = errors.New("realm authentication failed")

	// ErrRealmFull Realm 已满
	ErrRealmFull = errors.New("realm is full")
)

// ============================================================================
//                              发现相关错误
// ============================================================================

var (
	// ErrPeerNotFound 节点未找到
	ErrPeerNotFound = errors.New("peer not found")

	// ErrNoAddresses 无可用地址
	ErrNoAddresses = errors.New("no addresses available")

	// ErrDiscoveryTimeout 发现超时
	ErrDiscoveryTimeout = errors.New("discovery timeout")
)

// ============================================================================
//                              资源相关错误
// ============================================================================

var (
	// ErrResourceLimitExceeded 资源限制超出
	ErrResourceLimitExceeded = errors.New("resource limit exceeded")

	// ErrMemoryLimitExceeded 内存限制超出
	ErrMemoryLimitExceeded = errors.New("memory limit exceeded")

	// ErrBandwidthLimitExceeded 带宽限制超出
	ErrBandwidthLimitExceeded = errors.New("bandwidth limit exceeded")
)

// ============================================================================
//                              通用错误
// ============================================================================

var (
	// ErrNotReady 服务未就绪
	ErrNotReady = errors.New("service not ready")

	// ErrClosed 服务已关闭
	ErrClosed = errors.New("service closed")

	// ErrInvalidArgument 参数无效
	ErrInvalidArgument = errors.New("invalid argument")

	// ErrOperationCanceled 操作已取消
	ErrOperationCanceled = errors.New("operation canceled")

	// ErrTimeout 操作超时
	ErrTimeout = errors.New("operation timeout")
)
