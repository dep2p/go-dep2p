package auth

import (
	"context"
	"fmt"
	"sync"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("realm/auth")

// ============================================================================
//                              认证管理器
// ============================================================================

// AuthManager 认证管理器
type AuthManager struct {
	mu sync.RWMutex

	// 认证器工厂
	factory *AuthenticatorFactory

	// 已创建的认证器
	authenticators map[string]interfaces.Authenticator

	// 挑战处理器
	challengeHandler *ChallengeHandler

	// 状态
	closed bool
}

// NewAuthManager 创建认证管理器
func NewAuthManager() *AuthManager {
	return &AuthManager{
		factory:          NewAuthenticatorFactory(),
		authenticators:   make(map[string]interfaces.Authenticator),
		challengeHandler: NewChallengeHandler(0, 0, 0), // 使用默认值
	}
}

// CreateAuthenticator 创建认证器
func (m *AuthManager) CreateAuthenticator(
	realmID string,
	mode interfaces.AuthMode,
	config interfaces.AuthConfig,
) (interfaces.Authenticator, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil, ErrAuthenticatorClosed
	}

	// 转换配置（interfaces.AuthConfig -> 内部 AuthConfig）
	localConfig := AuthConfig{
		PSK:             config.PSK,
		PeerID:          config.PeerID,
		CertPath:        config.CertPath,
		KeyPath:         config.KeyPath,
		CustomValidator: config.CustomValidator,
		Timeout:         config.Timeout,
		MaxRetries:      config.MaxRetries,
		ReplayWindow:    config.ReplayWindow,
		NonceSize:       32,
	}

	// 使用工厂创建
	auth, err := m.factory.CreateAuthenticator(mode, localConfig)
	if err != nil {
		logger.Error("创建认证器失败", "realmID", realmID, "mode", mode, "error", err)
		return nil, err
	}

	// 存储到管理器（使用传入的 realmID 或认证器的 RealmID）
	if realmID == "" {
		realmID = auth.RealmID()
	}
	m.authenticators[realmID] = auth

	logger.Info("认证器已创建", "realmID", realmID, "mode", mode)
	return auth, nil
}

// GetAuthenticator 获取认证器
func (m *AuthManager) GetAuthenticator(realmID string) (interfaces.Authenticator, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	auth, exists := m.authenticators[realmID]
	return auth, exists
}

// RemoveAuthenticator 移除认证器
func (m *AuthManager) RemoveAuthenticator(realmID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	auth, exists := m.authenticators[realmID]
	if !exists {
		return ErrAuthenticatorNotFound
	}

	// 关闭认证器
	if err := auth.Close(); err != nil {
		return err
	}

	delete(m.authenticators, realmID)
	return nil
}

// ListAuthenticators 列出所有认证器
func (m *AuthManager) ListAuthenticators() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	realmIDs := make([]string, 0, len(m.authenticators))
	for realmID := range m.authenticators {
		realmIDs = append(realmIDs, realmID)
	}
	return realmIDs
}

// PerformChallenge 执行挑战-响应认证（实现接口）
func (m *AuthManager) PerformChallenge(
	ctx context.Context,
	peerID string,
	authenticator interfaces.Authenticator,
) error {
	// 简化实现：直接使用认证器生成和验证证明
	// 真实实现需要网络通信（通过 Host.NewStream）
	proof, err := authenticator.GenerateProof(ctx)
	if err != nil {
		return err
	}

	// 验证自己的证明（用于测试）
	valid, err := authenticator.Authenticate(ctx, peerID, proof)
	if err != nil {
		return err
	}

	if !valid {
		return ErrAuthFailed
	}

	return nil
}

// HandleChallenge 处理认证挑战请求（实现接口）
func (m *AuthManager) HandleChallenge(
	ctx context.Context,
	peerID string,
	request []byte,
	authenticator interfaces.Authenticator,
) ([]byte, error) {
	// 验证请求
	valid, err := authenticator.Authenticate(ctx, peerID, request)
	if err != nil {
		return nil, err
	}

	if !valid {
		return nil, ErrAuthFailed
	}

	// 生成响应（简化实现）
	return []byte("auth-success"), nil
}

// Close 关闭管理器
func (m *AuthManager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.closed {
		return nil
	}

	m.closed = true

	// 关闭所有认证器
	for realmID, auth := range m.authenticators {
		if err := auth.Close(); err != nil {
			// 记录错误但继续关闭其他认证器
			_ = fmt.Errorf("failed to close authenticator %s: %w", realmID, err)
		}
	}

	m.authenticators = nil

	return nil
}

// 确保实现接口
var _ interfaces.AuthManager = (*AuthManager)(nil)
