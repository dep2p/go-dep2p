// Package realm 定义 Realm 相关接口
//
// v1.1 新增: RealmAuth 协议类型定义
package realm

import (
	"time"

	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              RealmAuth 协议类型 (v1.1 新增)
// ============================================================================

// ConnRealmContext 连接级 Realm 上下文
//
// 存储通过 RealmAuth 协议验证后的 Realm 信息。
// 用于 Protocol Router 判断是否允许非系统协议流。
type ConnRealmContext struct {
	// RealmID 验证通过的 Realm 标识
	RealmID types.RealmID

	// Verified 是否已通过 RealmAuth 验证
	Verified bool

	// ExpiresAt 验证过期时间
	// 超过此时间后需要重新验证
	ExpiresAt time.Time

	// Role 在 Realm 中的角色（可选）
	Role string
}

// IsValid 检查 ConnRealmContext 是否有效
func (c *ConnRealmContext) IsValid() bool {
	if c == nil {
		return false
	}
	if !c.Verified {
		return false
	}
	if !c.ExpiresAt.IsZero() && time.Now().After(c.ExpiresAt) {
		return false
	}
	return true
}

// ============================================================================
//                              RealmAuth 请求/响应消息
// ============================================================================

// RealmAuthRequest RealmAuth 协议请求消息
//
// 由发起方发送，请求验证 Realm 成员身份。
type RealmAuthRequest struct {
	// SelectedRealm 请求验证的 Realm ID
	SelectedRealm types.RealmID

	// JoinProof 加入证明（用于 Protected Realm）
	// 对于 Public Realm 可为空
	JoinProof []byte

	// InviteProof 邀请证明（用于 Private Realm）
	// 由已有成员签名的邀请凭证
	InviteProof []byte

	// Timestamp 请求时间戳（Unix 秒）
	// 用于防止重放攻击
	Timestamp int64

	// Signature 请求签名
	// 使用发起方私钥对请求内容签名
	Signature []byte
}

// RealmAuthResponse RealmAuth 协议响应消息
//
// 由响应方返回，确认或拒绝 Realm 成员身份验证。
type RealmAuthResponse struct {
	// SelectedRealm 验证的 Realm ID
	SelectedRealm types.RealmID

	// Verified 是否验证通过
	Verified bool

	// ExpiresAt 验证过期时间（Unix 秒）
	// 为 0 表示不过期
	ExpiresAt int64

	// ErrorCode 错误码（验证失败时）
	// 0 表示成功
	ErrorCode uint32

	// ErrorMessage 错误消息（验证失败时）
	ErrorMessage string

	// Signature 响应签名
	// 使用响应方私钥对响应内容签名
	Signature []byte
}

// ============================================================================
//                              RealmAuth 错误码
// ============================================================================

const (
	// RealmAuthErrNone 无错误
	RealmAuthErrNone uint32 = 0

	// RealmAuthErrNotMember 不是 Realm 成员
	RealmAuthErrNotMember uint32 = 1

	// RealmAuthErrRealmNotFound Realm 不存在
	RealmAuthErrRealmNotFound uint32 = 2

	// RealmAuthErrInvalidProof 无效的加入/邀请证明
	RealmAuthErrInvalidProof uint32 = 3

	// RealmAuthErrExpired 请求已过期
	RealmAuthErrExpired uint32 = 4

	// RealmAuthErrInvalidSignature 无效签名
	RealmAuthErrInvalidSignature uint32 = 5

	// RealmAuthErrRealmMismatch Realm 不匹配
	RealmAuthErrRealmMismatch uint32 = 6

	// RealmAuthErrInternal 内部错误
	RealmAuthErrInternal uint32 = 100
)

// ============================================================================
//                              RealmAuth 协议常量
// ============================================================================

// 引用 pkg/protocolids 唯一真源
var (
	// RealmAuthProtocolID RealmAuth 协议标识
	RealmAuthProtocolID = protocolids.SysRealmAuth
)

const (

	// RealmAuthVersion RealmAuth 协议版本
	RealmAuthVersion = "1.0.0"

	// DefaultRealmAuthTimeout 默认 RealmAuth 超时时间
	DefaultRealmAuthTimeout = 10 * time.Second

	// DefaultRealmAuthExpiry 默认 RealmAuth 验证有效期
	// 24 小时后需要重新验证
	DefaultRealmAuthExpiry = 24 * time.Hour

	// MaxRealmAuthRequestAge 最大请求年龄（防重放）
	// 超过此时间的请求将被拒绝
	MaxRealmAuthRequestAge = 5 * time.Minute
)


