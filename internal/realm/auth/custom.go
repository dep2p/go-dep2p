package auth

import (
	"context"
	"fmt"
	"sync"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
)

// ============================================================================
//                              自定义认证器
// ============================================================================

// CustomValidator 自定义验证器函数类型
type CustomValidator func(ctx context.Context, peerID string, proof []byte) (bool, error)

// CustomProofGenerator 自定义证明生成器函数类型
type CustomProofGenerator func(ctx context.Context) ([]byte, error)

// CustomAuthenticator 自定义认证器
type CustomAuthenticator struct {
	mu sync.RWMutex

	// 配置
	realmID   string
	peerID    string
	validator CustomValidator
	generator CustomProofGenerator

	// 状态
	closed bool
}

// NewCustomAuthenticator 创建自定义认证器
func NewCustomAuthenticator(realmID, peerID string, validator CustomValidator) *CustomAuthenticator {
	return &CustomAuthenticator{
		realmID:   realmID,
		peerID:    peerID,
		validator: validator,
		generator: defaultProofGenerator,
	}
}

// NewCustomAuthenticatorWithGenerator 创建带自定义证明生成器的认证器
func NewCustomAuthenticatorWithGenerator(
	realmID, peerID string,
	validator CustomValidator,
	generator CustomProofGenerator,
) *CustomAuthenticator {
	return &CustomAuthenticator{
		realmID:   realmID,
		peerID:    peerID,
		validator: validator,
		generator: generator,
	}
}

// Mode 返回认证模式
func (a *CustomAuthenticator) Mode() interfaces.AuthMode {
	return interfaces.AuthModeCustom
}

// RealmID 返回 Realm ID
func (a *CustomAuthenticator) RealmID() string {
	return a.realmID
}

// GenerateProof 生成认证证明
func (a *CustomAuthenticator) GenerateProof(ctx context.Context) ([]byte, error) {
	a.mu.RLock()
	closed := a.closed
	generator := a.generator
	a.mu.RUnlock()

	if closed {
		return nil, ErrAuthenticatorClosed
	}

	if generator == nil {
		return nil, fmt.Errorf("%w: proof generator not set", ErrInvalidConfig)
	}

	return generator(ctx)
}

// Authenticate 验证认证证明
func (a *CustomAuthenticator) Authenticate(ctx context.Context, peerID string, proof []byte) (bool, error) {
	a.mu.RLock()
	closed := a.closed
	validator := a.validator
	a.mu.RUnlock()

	if closed {
		return false, ErrAuthenticatorClosed
	}

	if validator == nil {
		return false, fmt.Errorf("%w: validator not set", ErrInvalidConfig)
	}

	return validator(ctx, peerID, proof)
}

// SetValidator 设置验证器
func (a *CustomAuthenticator) SetValidator(validator CustomValidator) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.validator = validator
}

// SetProofGenerator 设置证明生成器
func (a *CustomAuthenticator) SetProofGenerator(generator CustomProofGenerator) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.generator = generator
}

// Close 关闭认证器
func (a *CustomAuthenticator) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return nil
	}

	a.closed = true
	a.validator = nil
	a.generator = nil

	return nil
}

// defaultProofGenerator 默认证明生成器
func defaultProofGenerator(_ context.Context) ([]byte, error) {
	// 生成随机 nonce 作为证明
	return GenerateNonce()
}

// 确保实现接口
var _ interfaces.Authenticator = (*CustomAuthenticator)(nil)
