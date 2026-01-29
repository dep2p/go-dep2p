package auth

import "errors"

var (
	// ErrInvalidPSK PSK 无效
	ErrInvalidPSK = errors.New("auth: invalid PSK")

	// ErrInvalidProof 证明无效
	ErrInvalidProof = errors.New("auth: invalid proof")

	// ErrAuthFailed 认证失败
	ErrAuthFailed = errors.New("auth: authentication failed")

	// ErrReplayAttack 重放攻击
	ErrReplayAttack = errors.New("auth: replay attack detected")

	// ErrTimestampExpired 时间戳过期
	ErrTimestampExpired = errors.New("auth: timestamp expired")

	// ErrInvalidTimestamp 时间戳无效
	ErrInvalidTimestamp = errors.New("auth: invalid timestamp")

	// ErrInvalidNonce nonce 无效
	ErrInvalidNonce = errors.New("auth: invalid nonce")

	// ErrInvalidCert 证书无效
	ErrInvalidCert = errors.New("auth: invalid certificate")

	// ErrCertExpired 证书过期
	ErrCertExpired = errors.New("auth: certificate expired")

	// ErrCertRevoked 证书已吊销
	ErrCertRevoked = errors.New("auth: certificate revoked")

	// ErrInvalidConfig 配置无效
	ErrInvalidConfig = errors.New("auth: invalid config")

	// ErrAuthenticatorNotFound 认证器不存在
	ErrAuthenticatorNotFound = errors.New("auth: authenticator not found")

	// ErrAuthenticatorClosed 认证器已关闭
	ErrAuthenticatorClosed = errors.New("auth: authenticator closed")

	// ErrTimeout 认证超时
	ErrTimeout = errors.New("auth: authentication timeout")

	// ErrContextCanceled 上下文已取消
	ErrContextCanceled = errors.New("auth: context canceled")
)
