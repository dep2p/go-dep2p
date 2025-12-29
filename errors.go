package dep2p

import (
	"context"
	"errors"
	"fmt"
)

// ============================================================================
//                              错误定义
// ============================================================================

// 预定义错误（Sentinel Errors）
//
// REQ-CONN-003: 提供可判定的错误类型，用户可使用 errors.Is() 进行精确匹配
var (
	// 生命周期错误
	ErrClosed     = errors.New("endpoint is closed")
	ErrNotStarted = errors.New("endpoint not started")

	// 连接错误
	ErrConnectionFailed  = errors.New("connection failed")
	ErrConnectionClosed  = errors.New("connection closed")
	ErrConnectionTimeout = errors.New("connection timeout")
	ErrConnectionRefused = errors.New("connection refused")

	// 地址错误
	ErrNoAddresses    = errors.New("no addresses available")
	ErrInvalidAddress = errors.New("invalid address")

	// 发现错误
	ErrPeerNotFound    = errors.New("peer not found")
	ErrDiscoveryFailed = errors.New("discovery failed")

	// 协议错误
	ErrProtocolNotSupported = errors.New("protocol not supported")

	// 流错误
	ErrStreamClosed = errors.New("stream closed")
	ErrStreamReset  = errors.New("stream reset")

	// 配置错误
	ErrInvalidConfig = errors.New("invalid configuration")

	// 身份错误
	ErrIdentityMismatch = errors.New("identity mismatch")

	// NAT/中继错误
	ErrNATTraversalFailed = errors.New("NAT traversal failed")
	ErrRelayFailed        = errors.New("relay failed")

	// 资源错误
	ErrResourceLimitExceeded = errors.New("resource limit exceeded")

	// Realm 错误（REQ-REALM-004）
	ErrNotMember       = errors.New("not a member of any realm")
	ErrAlreadyJoined   = errors.New("already joined a realm")
	ErrRealmMismatch   = errors.New("realm mismatch")
	ErrRealmAuthFailed = errors.New("realm authentication failed")
)

// ============================================================================
//                              错误类型
// ============================================================================

// Error dep2p 错误类型
type Error struct {
	// Code 错误码
	Code ErrorCode

	// Message 错误消息
	Message string

	// Cause 原始错误
	Cause error

	// Temporary 是否是临时错误（可重试）
	Temporary bool
}

// Error 实现 error 接口
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap 实现 errors.Unwrap
func (e *Error) Unwrap() error {
	return e.Cause
}

// Is 实现 errors.Is
func (e *Error) Is(target error) bool {
	if t, ok := target.(*Error); ok {
		return e.Code == t.Code
	}
	return false
}

// IsTemporary 是否是临时错误
func (e *Error) IsTemporary() bool {
	return e.Temporary
}

// ============================================================================
//                              错误码
// ============================================================================

// ErrorCode 错误码类型
type ErrorCode string

// 预定义错误码
const (
	// 通用错误
	ErrCodeUnknown  ErrorCode = "UNKNOWN"
	ErrCodeInternal ErrorCode = "INTERNAL"
	ErrCodeTimeout  ErrorCode = "TIMEOUT"
	ErrCodeCanceled ErrorCode = "CANCELED"

	// 连接错误
	ErrCodeConnectionFailed  ErrorCode = "CONNECTION_FAILED"
	ErrCodeConnectionClosed  ErrorCode = "CONNECTION_CLOSED"
	ErrCodeConnectionRefused ErrorCode = "CONNECTION_REFUSED"
	ErrCodeConnectionTimeout ErrorCode = "CONNECTION_TIMEOUT"
	ErrCodeConnectionTrimmed ErrorCode = "CONNECTION_TRIMMED"
	ErrCodeConnectionLimit   ErrorCode = "CONNECTION_LIMIT"

	// 流错误
	ErrCodeStreamClosed ErrorCode = "STREAM_CLOSED"
	ErrCodeStreamReset  ErrorCode = "STREAM_RESET"

	// 协议错误
	ErrCodeProtocolNotSupported ErrorCode = "PROTOCOL_NOT_SUPPORTED"
	ErrCodeProtocolNegotiation  ErrorCode = "PROTOCOL_NEGOTIATION"

	// 身份错误
	ErrCodeIdentityMismatch ErrorCode = "IDENTITY_MISMATCH"
	ErrCodeIdentityInvalid  ErrorCode = "IDENTITY_INVALID"

	// 地址错误
	ErrCodeAddressInvalid ErrorCode = "ADDRESS_INVALID"
	ErrCodeNoAddresses    ErrorCode = "NO_ADDRESSES"

	// 发现错误
	ErrCodePeerNotFound   ErrorCode = "PEER_NOT_FOUND"
	ErrCodeDiscoveryError ErrorCode = "DISCOVERY_ERROR"

	// NAT/中继错误
	ErrCodeNATTraversal ErrorCode = "NAT_TRAVERSAL"
	ErrCodeRelayError   ErrorCode = "RELAY_ERROR"

	// 资源错误
	ErrCodeResourceLimit ErrorCode = "RESOURCE_LIMIT"

	// Realm 错误（REQ-REALM-004）
	ErrCodeNotMember       ErrorCode = "NOT_MEMBER"
	ErrCodeAlreadyJoined   ErrorCode = "ALREADY_JOINED"
	ErrCodeRealmMismatch   ErrorCode = "REALM_MISMATCH"
	ErrCodeRealmAuthFailed ErrorCode = "REALM_AUTH_FAILED"
)

// ============================================================================
//                              错误构造器
// ============================================================================

// NewError 创建新错误
func NewError(code ErrorCode, message string) *Error {
	return &Error{
		Code:    code,
		Message: message,
	}
}

// WrapError 包装错误
func WrapError(code ErrorCode, message string, cause error) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

// NewTemporaryError 创建临时错误
func NewTemporaryError(code ErrorCode, message string, cause error) *Error {
	return &Error{
		Code:      code,
		Message:   message,
		Cause:     cause,
		Temporary: true,
	}
}

// ============================================================================
//                              错误检查（REQ-CONN-003 统一错误分类）
// ============================================================================

// IsTemporaryError 检查是否是临时错误
//
// 临时错误表示操作可以重试，例如：
// - 网络瞬时故障
// - 资源暂时不可用
// - 超时（某些情况下）
func IsTemporaryError(err error) bool {
	var e *Error
	if errors.As(err, &e) {
		return e.Temporary
	}
	return false
}

// GetErrorCode 获取错误码
//
// 如果错误不是 dep2p.Error 类型，返回 ErrCodeUnknown
func GetErrorCode(err error) ErrorCode {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ErrCodeUnknown
}

// GetErrorClass 获取错误分类
//
// REQ-CONN-003: 提供高层错误分类，便于用户判断错误类型
// 返回值：connection, stream, protocol, identity, address, discovery, nat, relay, realm, resource, unknown
func GetErrorClass(err error) string {
	code := GetErrorCode(err)

	switch code {
	case ErrCodeConnectionFailed, ErrCodeConnectionClosed, ErrCodeConnectionRefused,
		ErrCodeConnectionTimeout, ErrCodeConnectionTrimmed, ErrCodeConnectionLimit:
		return "connection"

	case ErrCodeStreamClosed, ErrCodeStreamReset:
		return "stream"

	case ErrCodeProtocolNotSupported, ErrCodeProtocolNegotiation:
		return "protocol"

	case ErrCodeIdentityMismatch, ErrCodeIdentityInvalid:
		return "identity"

	case ErrCodeAddressInvalid, ErrCodeNoAddresses:
		return "address"

	case ErrCodePeerNotFound, ErrCodeDiscoveryError:
		return "discovery"

	case ErrCodeNATTraversal:
		return "nat"

	case ErrCodeRelayError:
		return "relay"

	case ErrCodeNotMember, ErrCodeAlreadyJoined, ErrCodeRealmMismatch, ErrCodeRealmAuthFailed:
		return "realm"

	case ErrCodeResourceLimit:
		return "resource"

	case ErrCodeTimeout, ErrCodeCanceled:
		return "context"

	default:
		// 尝试通过 sentinel errors 判断
		if errors.Is(err, ErrConnectionFailed) || errors.Is(err, ErrConnectionClosed) ||
			errors.Is(err, ErrConnectionTimeout) || errors.Is(err, ErrConnectionRefused) {
			return "connection"
		}
		if errors.Is(err, ErrStreamClosed) || errors.Is(err, ErrStreamReset) {
			return "stream"
		}
		if errors.Is(err, ErrProtocolNotSupported) {
			return "protocol"
		}
		if errors.Is(err, ErrIdentityMismatch) {
			return "identity"
		}
		if errors.Is(err, ErrInvalidAddress) || errors.Is(err, ErrNoAddresses) {
			return "address"
		}
		if errors.Is(err, ErrPeerNotFound) || errors.Is(err, ErrDiscoveryFailed) {
			return "discovery"
		}
		if errors.Is(err, ErrNATTraversalFailed) {
			return "nat"
		}
		if errors.Is(err, ErrRelayFailed) {
			return "relay"
		}
		if errors.Is(err, ErrNotMember) || errors.Is(err, ErrRealmMismatch) ||
			errors.Is(err, ErrAlreadyJoined) || errors.Is(err, ErrRealmAuthFailed) {
			return "realm"
		}
		if errors.Is(err, ErrResourceLimitExceeded) {
			return "resource"
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return "context"
		}
		return "unknown"
	}
}

// IsConnectionError 检查是否是连接错误
func IsConnectionError(err error) bool {
	return GetErrorClass(err) == "connection"
}

// IsStreamError 检查是否是流错误
func IsStreamError(err error) bool {
	return GetErrorClass(err) == "stream"
}

// IsProtocolError 检查是否是协议错误
func IsProtocolError(err error) bool {
	return GetErrorClass(err) == "protocol"
}

// IsIdentityError 检查是否是身份错误
func IsIdentityError(err error) bool {
	return GetErrorClass(err) == "identity"
}

// IsAddressError 检查是否是地址错误
func IsAddressError(err error) bool {
	return GetErrorClass(err) == "address"
}

// IsDiscoveryError 检查是否是发现错误
func IsDiscoveryError(err error) bool {
	return GetErrorClass(err) == "discovery"
}

// IsRealmError 检查是否是 Realm 错误
func IsRealmError(err error) bool {
	return GetErrorClass(err) == "realm"
}

// IsResourceError 检查是否是资源错误
func IsResourceError(err error) bool {
	return GetErrorClass(err) == "resource"
}

// IsRetryable 检查错误是否可重试
//
// 可重试的错误包括：
// - 临时错误
// - 超时错误
// - 部分连接错误（如连接失败、超时）
// - 部分发现错误（如节点未找到）
func IsRetryable(err error) bool {
	if IsTemporaryError(err) {
		return true
	}

	code := GetErrorCode(err)
	switch code {
	case ErrCodeTimeout, ErrCodeConnectionTimeout, ErrCodeConnectionFailed,
		ErrCodePeerNotFound, ErrCodeDiscoveryError:
		return true
	}

	// 也检查 sentinel errors
	if errors.Is(err, ErrConnectionFailed) || errors.Is(err, ErrConnectionTimeout) ||
		errors.Is(err, ErrPeerNotFound) || errors.Is(err, ErrDiscoveryFailed) ||
		errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return false
}

// ============================================================================
//                              错误转换（REQ-CONN-003）
// ============================================================================

// ClassifyError 将任意错误转换为带分类信息的 dep2p.Error
//
// 这个函数用于将内部错误或底层错误转换为用户可理解的分类错误。
// 如果错误已经是 dep2p.Error，直接返回；否则尝试推断错误类型并包装。
func ClassifyError(err error) *Error {
	if err == nil {
		return nil
	}

	// 如果已经是 dep2p.Error，直接返回
	var e *Error
	if errors.As(err, &e) {
		return e
	}

	// 根据错误类型推断分类
	code, msg, temp := inferErrorClassification(err)

	return &Error{
		Code:      code,
		Message:   msg,
		Cause:     err,
		Temporary: temp,
	}
}

// inferErrorClassification 推断错误的分类
func inferErrorClassification(err error) (ErrorCode, string, bool) {
	// 连接相关错误
	if errors.Is(err, ErrConnectionFailed) {
		return ErrCodeConnectionFailed, "connection failed", true
	}
	if errors.Is(err, ErrConnectionClosed) {
		return ErrCodeConnectionClosed, "connection closed", false
	}
	if errors.Is(err, ErrConnectionTimeout) {
		return ErrCodeConnectionTimeout, "connection timeout", true
	}
	if errors.Is(err, ErrConnectionRefused) {
		return ErrCodeConnectionRefused, "connection refused", false
	}

	// 地址相关错误
	if errors.Is(err, ErrNoAddresses) {
		return ErrCodeNoAddresses, "no addresses available", false
	}
	if errors.Is(err, ErrInvalidAddress) {
		return ErrCodeAddressInvalid, "invalid address", false
	}

	// 发现相关错误
	if errors.Is(err, ErrPeerNotFound) {
		return ErrCodePeerNotFound, "peer not found", true
	}
	if errors.Is(err, ErrDiscoveryFailed) {
		return ErrCodeDiscoveryError, "discovery failed", true
	}

	// 协议相关错误
	if errors.Is(err, ErrProtocolNotSupported) {
		return ErrCodeProtocolNotSupported, "protocol not supported", false
	}

	// 流相关错误
	if errors.Is(err, ErrStreamClosed) {
		return ErrCodeStreamClosed, "stream closed", false
	}
	if errors.Is(err, ErrStreamReset) {
		return ErrCodeStreamReset, "stream reset", false
	}

	// 身份相关错误
	if errors.Is(err, ErrIdentityMismatch) {
		return ErrCodeIdentityMismatch, "identity mismatch", false
	}

	// NAT/中继相关错误
	if errors.Is(err, ErrNATTraversalFailed) {
		return ErrCodeNATTraversal, "NAT traversal failed", true
	}
	if errors.Is(err, ErrRelayFailed) {
		return ErrCodeRelayError, "relay failed", true
	}

	// Realm 相关错误
	if errors.Is(err, ErrNotMember) {
		return ErrCodeNotMember, "not a member of any realm", false
	}
	if errors.Is(err, ErrAlreadyJoined) {
		return ErrCodeAlreadyJoined, "already joined a realm", false
	}
	if errors.Is(err, ErrRealmMismatch) {
		return ErrCodeRealmMismatch, "realm mismatch", false
	}
	if errors.Is(err, ErrRealmAuthFailed) {
		return ErrCodeRealmAuthFailed, "realm authentication failed", false
	}

	// 资源相关错误
	if errors.Is(err, ErrResourceLimitExceeded) {
		return ErrCodeResourceLimit, "resource limit exceeded", true
	}

	// 上下文相关错误
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrCodeTimeout, "operation timeout", true
	}
	if errors.Is(err, context.Canceled) {
		return ErrCodeCanceled, "operation canceled", false
	}

	// 未知错误
	return ErrCodeUnknown, err.Error(), false
}

