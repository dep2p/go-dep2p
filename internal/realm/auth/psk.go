package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/crypto/hkdf"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

// ============================================================================
//                              HKDF 密钥派生
// ============================================================================

const (
	// RealmID 派生 salt
	realmIDSalt = "dep2p-realm-id-v1"

	// 认证密钥派生 salt
	authKeySalt = "dep2p-auth-key-v1"

	// 密钥长度（32 字节 = 256 位）
	keyLength = 32
)

// DeriveRealmID 从 PSK 派生 RealmID
//
// 使用 HKDF-SHA256 派生确定性的 RealmID。
// 相同的 PSK 总是派生出相同的 RealmID。
func DeriveRealmID(psk []byte) string {
	if len(psk) == 0 {
		return ""
	}

	// 计算 PSK 的 SHA256 作为 info
	hash := sha256.Sum256(psk)

	// 使用 HKDF 派生
	kdf := hkdf.New(sha256.New, psk, []byte(realmIDSalt), hash[:])

	// 读取 32 字节
	realmID := make([]byte, keyLength)
	if _, err := io.ReadFull(kdf, realmID); err != nil {
		return ""
	}

	// 返回十六进制编码
	return hex.EncodeToString(realmID)
}

// DeriveAuthKey 从 PSK 派生认证密钥
//
// 使用 HKDF-SHA256 派生用于 HMAC 的认证密钥。
// 认证密钥与 RealmID 绑定。
func DeriveAuthKey(psk []byte, realmID string) []byte {
	if len(psk) == 0 || realmID == "" {
		return nil
	}

	// 使用 RealmID 作为 info
	kdf := hkdf.New(sha256.New, psk, []byte(authKeySalt), []byte(realmID))

	// 读取 32 字节
	authKey := make([]byte, keyLength)
	if _, err := io.ReadFull(kdf, authKey); err != nil {
		return nil
	}

	return authKey
}

// ============================================================================
//                              PSK 认证器
// ============================================================================

// PSKAuthenticator PSK 认证器
type PSKAuthenticator struct {
	// 配置
	psk     []byte
	peerID  string
	realmID string
	authKey []byte

	// 重放攻击防护
	mu             sync.RWMutex
	replayWindow   time.Duration
	lastTimestamps map[string]int64 // peerID -> last timestamp
	cleanupTicker  *time.Ticker
	cleanupStop    chan struct{}

	// 状态
	closed bool
}

// NewPSKAuthenticator 创建 PSK 认证器
func NewPSKAuthenticator(psk []byte, peerID string) (*PSKAuthenticator, error) {
	if len(psk) == 0 {
		return nil, ErrInvalidPSK
	}

	if len(psk) < 16 {
		return nil, fmt.Errorf("%w: PSK too short (minimum 16 bytes)", ErrInvalidPSK)
	}

	if peerID == "" {
		return nil, fmt.Errorf("%w: peerID cannot be empty", ErrInvalidConfig)
	}

	// 派生 RealmID
	realmID := DeriveRealmID(psk)
	if realmID == "" {
		return nil, fmt.Errorf("%w: failed to derive RealmID", ErrInvalidPSK)
	}

	// 派生认证密钥
	authKey := DeriveAuthKey(psk, realmID)
	if authKey == nil {
		return nil, fmt.Errorf("%w: failed to derive auth key", ErrInvalidPSK)
	}

	auth := &PSKAuthenticator{
		psk:            psk,
		peerID:         peerID,
		realmID:        realmID,
		authKey:        authKey,
		replayWindow:   5 * time.Minute,
		lastTimestamps: make(map[string]int64),
		cleanupStop:    make(chan struct{}),
	}

	// 启动清理协程
	auth.cleanupTicker = time.NewTicker(time.Minute)
	go auth.cleanupLoop()

	return auth, nil
}

// Mode 返回认证模式
func (a *PSKAuthenticator) Mode() interfaces.AuthMode {
	return interfaces.AuthModePSK
}

// RealmID 返回 Realm ID
func (a *PSKAuthenticator) RealmID() string {
	return a.realmID
}

// GenerateProof 生成认证证明
func (a *PSKAuthenticator) GenerateProof(_ context.Context) ([]byte, error) {
	a.mu.RLock()
	closed := a.closed
	a.mu.RUnlock()

	if closed {
		return nil, ErrAuthenticatorClosed
	}

	// 生成随机 nonce
	nonce, err := GenerateNonce()
	if err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// 当前时间戳
	timestamp := time.Now().Unix()

	// 计算证明：HMAC-SHA256(AuthKey, nonce||peerID||timestamp)
	proof := ComputeProof(a.authKey, nonce, a.peerID, timestamp)

	// 组合：nonce + timestamp + proof
	result := make([]byte, 0, len(nonce)+8+len(proof))
	result = append(result, nonce...)
	result = appendInt64(result, timestamp)
	result = append(result, proof...)

	return result, nil
}

// Authenticate 验证认证证明
func (a *PSKAuthenticator) Authenticate(_ context.Context, peerID string, proof []byte) (bool, error) {
	a.mu.RLock()
	closed := a.closed
	a.mu.RUnlock()

	if closed {
		return false, ErrAuthenticatorClosed
	}

	logger.Debug("验证认证证明", "peerID", log.TruncateID(peerID, 8))

	// 解析证明：nonce + timestamp + proof
	if len(proof) < 32+8+32 {
		logger.Warn("认证证明格式无效", "peerID", log.TruncateID(peerID, 8), "proofLen", len(proof))
		return false, ErrInvalidProof
	}

	nonce := proof[:32]
	timestamp := parseInt64(proof[32:40])
	proofData := proof[40:]

	// 验证时间戳（防重放攻击）
	if !VerifyTimestamp(timestamp, a.replayWindow) {
		logger.Warn("认证时间戳过期", "peerID", log.TruncateID(peerID, 8))
		return false, ErrTimestampExpired
	}

	// 检查是否是重放攻击
	a.mu.Lock()
	lastTimestamp, exists := a.lastTimestamps[peerID]
	if exists && timestamp <= lastTimestamp {
		a.mu.Unlock()
		logger.Warn("检测到重放攻击", "peerID", log.TruncateID(peerID, 8))
		return false, ErrReplayAttack
	}
	a.lastTimestamps[peerID] = timestamp
	a.mu.Unlock()

	// 计算期望的证明
	expected := ComputeProof(a.authKey, nonce, peerID, timestamp)

	// 使用 hmac.Equal 防时间攻击
	if !hmac.Equal(proofData, expected) {
		logger.Warn("认证证明验证失败", "peerID", log.TruncateID(peerID, 8))
		return false, nil
	}

	logger.Debug("认证成功", "peerID", log.TruncateID(peerID, 8))
	return true, nil
}

// Close 关闭认证器
func (a *PSKAuthenticator) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.closed {
		return nil
	}

	a.closed = true

	// 停止清理协程
	close(a.cleanupStop)
	if a.cleanupTicker != nil {
		a.cleanupTicker.Stop()
	}

	// 清除敏感数据
	if len(a.psk) > 0 {
		for i := range a.psk {
			a.psk[i] = 0
		}
		a.psk = nil
	}

	if len(a.authKey) > 0 {
		for i := range a.authKey {
			a.authKey[i] = 0
		}
		a.authKey = nil
	}

	a.lastTimestamps = nil

	return nil
}

// cleanupLoop 清理过期的时间戳记录
func (a *PSKAuthenticator) cleanupLoop() {
	for {
		select {
		case <-a.cleanupTicker.C:
			a.cleanupExpiredTimestamps()
		case <-a.cleanupStop:
			return
		}
	}
}

// cleanupExpiredTimestamps 清理过期的时间戳
func (a *PSKAuthenticator) cleanupExpiredTimestamps() {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now().Unix()
	windowSeconds := int64(a.replayWindow.Seconds())

	for peerID, timestamp := range a.lastTimestamps {
		if now-timestamp > windowSeconds*2 {
			delete(a.lastTimestamps, peerID)
		}
	}
}

// ============================================================================
//                              辅助函数
// ============================================================================

// appendInt64 将 int64 追加到字节切片
func appendInt64(b []byte, v int64) []byte {
	return append(b,
		byte(v>>56), byte(v>>48), byte(v>>40), byte(v>>32),
		byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

// parseInt64 从字节切片解析 int64
func parseInt64(b []byte) int64 {
	if len(b) < 8 {
		return 0
	}
	return int64(b[0])<<56 | int64(b[1])<<48 | int64(b[2])<<40 | int64(b[3])<<32 |
		int64(b[4])<<24 | int64(b[5])<<16 | int64(b[6])<<8 | int64(b[7])
}

// 确保实现接口
var _ interfaces.Authenticator = (*PSKAuthenticator)(nil)
