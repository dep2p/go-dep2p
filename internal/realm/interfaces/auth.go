// Package interfaces 定义 realm 模块内部接口
package interfaces

import (
	"context"
	"time"
)

// ============================================================================
//                              认证器接口
// ============================================================================

// Authenticator 认证器接口
//
// Authenticator 负责验证节点是否有权访问特定 Realm。
type Authenticator interface {
	// Authenticate 验证对方身份
	//
	// 参数：
	//   - ctx: 上下文
	//   - peerID: 对方节点 ID
	//   - proof: 认证证明（格式由认证模式决定）
	//
	// 返回：
	//   - bool: 认证是否成功
	//   - error: 认证过程中的错误
	Authenticate(ctx context.Context, peerID string, proof []byte) (bool, error)

	// GenerateProof 生成认证证明
	//
	// 生成用于证明自己身份的认证数据。
	GenerateProof(ctx context.Context) ([]byte, error)

	// Mode 返回认证模式
	Mode() AuthMode

	// RealmID 返回关联的 Realm ID
	RealmID() string

	// Close 关闭认证器，释放资源
	Close() error
}

// AuthMode 认证模式
type AuthMode int

const (
	// AuthModePSK 预共享密钥认证
	AuthModePSK AuthMode = iota

	// AuthModeCert 证书认证
	AuthModeCert

	// AuthModeCustom 自定义认证
	AuthModeCustom
)

// String 返回认证模式的字符串表示
func (m AuthMode) String() string {
	switch m {
	case AuthModePSK:
		return "PSK"
	case AuthModeCert:
		return "Cert"
	case AuthModeCustom:
		return "Custom"
	default:
		return "Unknown"
	}
}

// ============================================================================
//                              认证管理器接口
// ============================================================================

// AuthManager 认证管理器接口
//
// AuthManager 负责管理多个 Realm 的认证器，处理认证协议。
type AuthManager interface {
	// CreateAuthenticator 创建认证器
	CreateAuthenticator(realmID string, mode AuthMode, config AuthConfig) (Authenticator, error)

	// GetAuthenticator 获取认证器
	GetAuthenticator(realmID string) (Authenticator, bool)

	// RemoveAuthenticator 移除认证器
	RemoveAuthenticator(realmID string) error

	// PerformChallenge 执行挑战-响应认证
	PerformChallenge(ctx context.Context, peerID string, authenticator Authenticator) error

	// HandleChallenge 处理认证挑战请求
	HandleChallenge(ctx context.Context, peerID string, request []byte, authenticator Authenticator) ([]byte, error)

	// Close 关闭管理器
	Close() error
}

// ============================================================================
//                              认证配置
// ============================================================================

// AuthConfig 认证配置
type AuthConfig struct {
	// PSK 预共享密钥（用于 PSK 模式）
	PSK []byte

	// PeerID 本地节点 ID
	PeerID string

	// CertPath 证书路径（用于 Cert 模式）
	CertPath string

	// KeyPath 私钥路径（用于 Cert 模式）
	KeyPath string

	// CustomValidator 自定义验证器（用于 Custom 模式）
	CustomValidator func(ctx context.Context, peerID string, proof []byte) (bool, error)

	// Timeout 认证超时时间
	Timeout time.Duration

	// MaxRetries 最大重试次数
	MaxRetries int

	// ReplayWindow 重放攻击防护时间窗口
	ReplayWindow time.Duration
}

// ============================================================================
//                              挑战-响应协议
// ============================================================================

// ChallengeRequest 挑战请求
type ChallengeRequest struct {
	PeerID    string
	RealmID   string
	Timestamp int64
}

// ChallengeResponse 挑战响应
type ChallengeResponse struct {
	Nonce     []byte
	Timestamp int64
}

// ProofResponse 证明响应
type ProofResponse struct {
	Proof     []byte
	Timestamp int64
}

// AuthResult 认证结果
type AuthResult struct {
	Success bool
	Error   string
}
