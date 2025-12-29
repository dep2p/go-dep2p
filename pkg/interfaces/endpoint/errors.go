// Package endpoint 提供端点相关的接口定义和错误类型
package endpoint

import (
	"errors"
	"fmt"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              预定义错误
// ============================================================================

var (
	// ErrInvalidNodeID 无效节点 ID 错误
	ErrInvalidNodeID = types.ErrInvalidNodeID // 从 types 包导入
	// ErrNodeNotFound 节点未找到错误
	ErrNodeNotFound = errors.New("node not found")
	// ErrSelfConnect 自连接错误
	ErrSelfConnect = errors.New("cannot connect to self")

	// ErrConnectionRefused 连接被拒绝错误
	ErrConnectionRefused = errors.New("connection refused")
	// ErrConnectionClosed 连接已关闭错误
	ErrConnectionClosed = errors.New("connection closed")
	// ErrConnectionTimeout 连接超时错误
	ErrConnectionTimeout = errors.New("connection timeout")
	// ErrConnectionFailed 连接失败错误
	ErrConnectionFailed = errors.New("connection failed")
	// ErrAlreadyConnected 已连接错误
	ErrAlreadyConnected = errors.New("already connected")
	// ErrNoAddresses 无可用地址错误
	ErrNoAddresses = errors.New("no addresses available")
	// ErrAllDialsFailed 所有拨号尝试失败错误
	ErrAllDialsFailed = errors.New("all dial attempts failed")

	// ErrStreamClosed 流已关闭错误
	ErrStreamClosed = errors.New("stream closed")
	// ErrStreamReset 流重置错误
	ErrStreamReset = errors.New("stream reset")
	// ErrStreamLimit 流限制错误
	ErrStreamLimit = errors.New("stream limit reached")

	// ErrProtocolNotSupported 协议不支持错误
	ErrProtocolNotSupported = errors.New("protocol not supported")
	// ErrProtocolNegotiation 协议协商失败错误
	ErrProtocolNegotiation = errors.New("protocol negotiation failed")

	// ErrIdentityMismatch 身份不匹配错误
	ErrIdentityMismatch = errors.New("identity mismatch")
	// ErrInvalidSignature 无效签名错误
	ErrInvalidSignature = errors.New("invalid signature")
	// ErrInvalidPublicKey 无效公钥错误
	ErrInvalidPublicKey = errors.New("invalid public key")

	// ErrAddressInvalid 无效地址错误
	ErrAddressInvalid = errors.New("invalid address")
	// ErrAddressParsing 地址解析失败错误
	ErrAddressParsing = errors.New("address parsing failed")

	// ErrNATTraversalFailed NAT 穿透失败错误
	ErrNATTraversalFailed = errors.New("NAT traversal failed")
	// ErrRelayFailed 中继连接失败错误
	ErrRelayFailed = errors.New("relay connection failed")
	// ErrNoRelayAvailable 无可用中继错误
	ErrNoRelayAvailable = errors.New("no relay available")

	// ErrDiscoveryFailed 发现失败错误
	ErrDiscoveryFailed = errors.New("discovery failed")
	// ErrDiscoveryTimeout 发现超时错误
	ErrDiscoveryTimeout = errors.New("discovery timeout")

	// ErrResourceExhausted 资源耗尽错误
	ErrResourceExhausted = errors.New("resource exhausted")
	// ErrRateLimited 速率限制错误
	ErrRateLimited = errors.New("rate limited")

	// ErrNotStarted 未启动错误
	ErrNotStarted = errors.New("not started")
	// ErrAlreadyStarted 已启动错误
	ErrAlreadyStarted = errors.New("already started")
	// ErrAlreadyClosed 已关闭错误
	ErrAlreadyClosed = errors.New("already closed")
	// ErrShuttingDown 正在关闭错误
	ErrShuttingDown = errors.New("shutting down")

	// ErrNotMember 非 Realm 成员错误
	ErrNotMember = errors.New("not a member of any realm")
	// ErrAlreadyJoined 已加入 Realm 错误
	ErrAlreadyJoined = errors.New("already joined a realm")
	// ErrRealmMismatch Realm 不匹配错误
	ErrRealmMismatch = errors.New("realm mismatch")
	// ErrRealmAuthFailed Realm 认证失败错误
	ErrRealmAuthFailed = errors.New("realm authentication failed")
)

// ============================================================================
//                              错误码
// ============================================================================

// ErrorCode 错误码
type ErrorCode int

const (
	// ErrCodeUnknown 未知错误码
	ErrCodeUnknown ErrorCode = iota
	// ErrCodeInternal 内部错误码
	ErrCodeInternal
	// ErrCodeInvalidArgument 无效参数错误码
	ErrCodeInvalidArgument
	// ErrCodeTimeout 超时错误码
	ErrCodeTimeout
	// ErrCodeCanceled 取消错误码
	ErrCodeCanceled
)

const (
	// ErrCodeConnectionRefused 连接被拒绝错误码
	ErrCodeConnectionRefused ErrorCode = 100 + iota
	// ErrCodeConnectionClosed 连接已关闭错误码
	ErrCodeConnectionClosed
	// ErrCodeConnectionTimeout 连接超时错误码
	ErrCodeConnectionTimeout
	// ErrCodeConnectionFailed 连接失败错误码
	ErrCodeConnectionFailed
	// ErrCodeAlreadyConnected 已连接错误码
	ErrCodeAlreadyConnected
	// ErrCodeNoAddresses 无地址错误码
	ErrCodeNoAddresses
)

const (
	// ErrCodeStreamClosed 流关闭错误码
	ErrCodeStreamClosed ErrorCode = 200 + iota
	// ErrCodeStreamReset 流重置错误码
	ErrCodeStreamReset
	// ErrCodeStreamLimit 流限制错误码
	ErrCodeStreamLimit
)

const (
	// ErrCodeProtocolNotSupported 协议不支持错误码
	ErrCodeProtocolNotSupported ErrorCode = 300 + iota
	// ErrCodeProtocolNegotiation 协议协商失败错误码
	ErrCodeProtocolNegotiation
)

const (
	// ErrCodeIdentityMismatch 身份不匹配错误码
	ErrCodeIdentityMismatch ErrorCode = 400 + iota
	// ErrCodeInvalidSignature 无效签名错误码
	ErrCodeInvalidSignature
	// ErrCodeInvalidPublicKey 无效公钥错误码
	ErrCodeInvalidPublicKey
)

const (
	// ErrCodeNATTraversalFailed NAT 穿透失败错误码
	ErrCodeNATTraversalFailed ErrorCode = 500 + iota
	// ErrCodeRelayFailed 中继失败错误码
	ErrCodeRelayFailed
	// ErrCodeNoRelayAvailable 无可用中继错误码
	ErrCodeNoRelayAvailable
)

const (
	// ErrCodeDiscoveryFailed 发现失败错误码
	ErrCodeDiscoveryFailed ErrorCode = 600 + iota
	// ErrCodeDiscoveryTimeout 发现超时错误码
	ErrCodeDiscoveryTimeout
)

const (
	// ErrCodeNotMember 非成员错误码
	ErrCodeNotMember ErrorCode = 700 + iota
	// ErrCodeAlreadyJoined 已加入错误码
	ErrCodeAlreadyJoined
	// ErrCodeRealmMismatch Realm 不匹配错误码
	ErrCodeRealmMismatch
	// ErrCodeRealmAuthFailed Realm 认证失败错误码
	ErrCodeRealmAuthFailed
)

// ============================================================================
//                              错误接口（v1.1 已删除）
// ============================================================================

// 注意：Error 接口已删除（v1.1 清理）。
// 原因：无外部使用，公共 API 使用 dep2p.Error 结构体。
// 内部错误处理使用 dep2pError 私有类型。

// ============================================================================
//                              错误实现
// ============================================================================

// dep2pError 错误实现
type dep2pError struct {
	code      ErrorCode
	message   string
	cause     error
	temporary bool
}

// NewError 创建新错误
func NewError(code ErrorCode, message string) error {
	return &dep2pError{
		code:    code,
		message: message,
	}
}

// NewErrorWithCause 创建带原因的错误
func NewErrorWithCause(code ErrorCode, message string, cause error) error {
	return &dep2pError{
		code:    code,
		message: message,
		cause:   cause,
	}
}

// NewTemporaryError 创建临时错误
func NewTemporaryError(code ErrorCode, message string, cause error) error {
	return &dep2pError{
		code:      code,
		message:   message,
		cause:     cause,
		temporary: true,
	}
}

// Error 实现 error 接口
func (e *dep2pError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s: %v", e.message, e.cause)
	}
	return e.message
}

// Code 返回错误码
func (e *dep2pError) Code() ErrorCode {
	return e.code
}

// IsTemporary 是否是临时错误
func (e *dep2pError) IsTemporary() bool {
	return e.temporary
}

// Unwrap 返回包装的错误
func (e *dep2pError) Unwrap() error {
	return e.cause
}

// Is 实现 errors.Is 支持
func (e *dep2pError) Is(target error) bool {
	t, ok := target.(*dep2pError)
	if !ok {
		return false
	}
	return e.code == t.code
}

// ============================================================================
//                              错误辅助函数
// ============================================================================

// WrapError 包装错误
func WrapError(err error, message string) error {
	if err == nil {
		return nil
	}
	return &dep2pError{
		code:    ErrCodeUnknown,
		message: message,
		cause:   err,
	}
}

// ============================================================================
//                              错误检查函数（v1.1 已移动）
// ============================================================================

// 注意：以下函数已移动到主包 errors.go：
// - IsTemporaryError(err error) bool
// - GetErrorCode(err error) ErrorCode
// - IsConnectionError(err error) bool
// - IsStreamError(err error) bool
// - IsProtocolError(err error) bool
// - IsRealmError(err error) bool
//
// 请使用 dep2p.IsTemporaryError() 等函数代替。
