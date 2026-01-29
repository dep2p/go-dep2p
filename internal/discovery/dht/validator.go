// Package dht 提供分布式哈希表实现
//
// 本文件实现 PeerRecord 验证器，用于 DHT 写入和冲突解决。
package dht

import (
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/dep2p/go-dep2p/pkg/lib/crypto"
)

// ============================================================================
//                              验证器接口
// ============================================================================

// PeerRecordValidator PeerRecord 验证器接口
//
// 实现依赖倒置原则：DHT 核心逻辑依赖此接口，不依赖具体实现。
type PeerRecordValidator interface {
	// Validate 完整验证 PeerRecord
	//
	// 验证内容：
	//   - 签名有效性
	//   - Key 与 RealmID/NodeID 匹配
	//   - TTL 有效性
	//   - seq > 0
	Validate(key string, signed *SignedRealmPeerRecord) error

	// ValidateSignature 仅验证签名
	ValidateSignature(signed *SignedRealmPeerRecord) error

	// ValidateKeyMatch 验证 Key 与 PeerRecord 内容匹配
	ValidateKeyMatch(key string, record *RealmPeerRecord) error

	// ValidateTTL 验证 TTL 有效性（未过期）
	ValidateTTL(record *RealmPeerRecord) error

	// SelectBest 从多个记录中选择最优（seq 最大的有效记录）
	SelectBest(records []*SignedRealmPeerRecord) *SignedRealmPeerRecord
}

// ============================================================================
//                              默认验证器实现
// ============================================================================

// DefaultPeerRecordValidator 默认 PeerRecord 验证器
type DefaultPeerRecordValidator struct {
	// clockSkewTolerance 时钟偏差容忍（用于 TTL 验证）
	clockSkewTolerance time.Duration
}

// NewDefaultPeerRecordValidator 创建默认验证器
func NewDefaultPeerRecordValidator() *DefaultPeerRecordValidator {
	return &DefaultPeerRecordValidator{
		clockSkewTolerance: 5 * time.Minute, // 允许 5 分钟时钟偏差
	}
}

// Validate 完整验证 PeerRecord
func (v *DefaultPeerRecordValidator) Validate(key string, signed *SignedRealmPeerRecord) error {
	if signed == nil {
		return errors.New("nil signed record")
	}
	if signed.Record == nil {
		return ErrNilPeerRecord
	}

	// 1. 验证 seq > 0
	if signed.Record.Seq == 0 {
		return ErrInvalidSeq
	}

	// 2. 验证签名
	if err := v.ValidateSignature(signed); err != nil {
		return fmt.Errorf("signature validation failed: %w", err)
	}

	// 3. 验证 Key 匹配
	if err := v.ValidateKeyMatch(key, signed.Record); err != nil {
		return fmt.Errorf("key validation failed: %w", err)
	}

	// 4. 验证 TTL
	if err := v.ValidateTTL(signed.Record); err != nil {
		return fmt.Errorf("TTL validation failed: %w", err)
	}

	// 5. 验证 NodeID 与公钥匹配
	if err := v.validateNodeIDMatchesPublicKey(signed); err != nil {
		return fmt.Errorf("NodeID/PublicKey mismatch: %w", err)
	}

	return nil
}

// ValidateSignature 验证签名
func (v *DefaultPeerRecordValidator) ValidateSignature(signed *SignedRealmPeerRecord) error {
	return VerifySignedRealmPeerRecord(signed)
}

// ValidateKeyMatch 验证 Key 与 PeerRecord 内容匹配
func (v *DefaultPeerRecordValidator) ValidateKeyMatch(key string, record *RealmPeerRecord) error {
	if record == nil {
		return ErrNilPeerRecord
	}

	// 解析 Key
	parsed, err := ParseRealmKey(key)
	if err != nil {
		return fmt.Errorf("invalid key format: %w", err)
	}

	// 验证 Key 类型
	if parsed.KeyType != KeyTypePeer {
		return fmt.Errorf("invalid key type: expected %s, got %s", KeyTypePeer, parsed.KeyType)
	}

	// 验证 RealmID 哈希匹配
	expectedRealmHash := hex.EncodeToString(HashRealmID(record.RealmID))
	if parsed.RealmHash != expectedRealmHash {
		return ErrRealmIDMismatch
	}

	// 验证 NodeID 匹配
	if parsed.Payload != string(record.NodeID) {
		return ErrNodeIDMismatch
	}

	return nil
}

// ValidateTTL 验证 TTL 有效性
func (v *DefaultPeerRecordValidator) ValidateTTL(record *RealmPeerRecord) error {
	if record == nil {
		return ErrNilPeerRecord
	}

	// 检查 TTL 范围
	ttlDuration := time.Duration(record.TTL) * time.Millisecond
	if ttlDuration < MinPeerRecordTTL {
		return fmt.Errorf("TTL too short: %v < %v", ttlDuration, MinPeerRecordTTL)
	}
	if ttlDuration > MaxPeerRecordTTL {
		return fmt.Errorf("TTL too long: %v > %v", ttlDuration, MaxPeerRecordTTL)
	}

	// 检查是否过期（考虑时钟偏差）
	expiryTime := time.Unix(0, record.Timestamp).Add(ttlDuration)
	now := time.Now()

	// 允许一定的时钟偏差
	if now.After(expiryTime.Add(v.clockSkewTolerance)) {
		return ErrRecordExpired
	}

	return nil
}

// SelectBest 从多个记录中选择最优
//
// 选择规则：
//  1. 过滤掉无效记录（签名无效或已过期）
//  2. 选择 seq 最大的记录
//  3. 如果 seq 相同，选择 timestamp 更新的
func (v *DefaultPeerRecordValidator) SelectBest(records []*SignedRealmPeerRecord) *SignedRealmPeerRecord {
	if len(records) == 0 {
		return nil
	}

	var best *SignedRealmPeerRecord

	for _, r := range records {
		// 跳过无效记录
		if r == nil || r.Record == nil {
			continue
		}

		// 验证签名
		if err := v.ValidateSignature(r); err != nil {
			continue
		}

		// 验证 TTL（允许过期记录参与选择，但优先选择未过期的）
		isExpired := r.Record.IsExpired()

		if best == nil {
			best = r
			continue
		}

		bestExpired := best.Record.IsExpired()

		// 优先选择未过期的记录
		if bestExpired && !isExpired {
			best = r
			continue
		}
		if !bestExpired && isExpired {
			continue
		}

		// 选择 seq 更大的
		if r.Record.Seq > best.Record.Seq {
			best = r
			continue
		}

		// seq 相同时，选择 timestamp 更新的
		if r.Record.Seq == best.Record.Seq && r.Record.Timestamp > best.Record.Timestamp {
			best = r
		}
	}

	return best
}

// validateNodeIDMatchesPublicKey 验证 NodeID 与公钥匹配
//
// 使用 crypto.PeerIDFromPublicKey 保持与 Identity 层一致的派生算法：
// - Identity 层：MarshalPublicKey（带类型前缀）→ SHA256 → Base58
// - 旧实现错误：PublicKey.Raw()（无类型前缀）→ SHA256 → Base58
//
//统一使用 crypto.PeerIDFromPublicKey
func (v *DefaultPeerRecordValidator) validateNodeIDMatchesPublicKey(signed *SignedRealmPeerRecord) error {
	if signed == nil || signed.PublicKey == nil || signed.Record == nil {
		return errors.New("invalid signed record")
	}

	// 使用 crypto.PeerIDFromPublicKey（与 Identity 层一致）
	derivedPeerID, err := crypto.PeerIDFromPublicKey(signed.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to derive peer ID: %w", err)
	}

	// 比较
	if string(derivedPeerID) != string(signed.Record.NodeID) {
		return errors.New("NodeID does not match public key")
	}

	return nil
}

// ============================================================================
//                              验证结果
// ============================================================================

// ValidationResult 验证结果
type ValidationResult struct {
	// Valid 是否有效
	Valid bool

	// Error 错误信息（如果无效）
	Error error

	// Reason 失败原因代码
	Reason ValidationFailureReason
}

// ValidationFailureReason 验证失败原因
type ValidationFailureReason int

const (
	// ValidationOK 验证通过
	ValidationOK ValidationFailureReason = iota

	// ValidationFailedSignature 签名验证失败
	ValidationFailedSignature

	// ValidationFailedRealmMismatch RealmID 不匹配
	ValidationFailedRealmMismatch

	// ValidationFailedNodeMismatch NodeID 不匹配
	ValidationFailedNodeMismatch

	// ValidationFailedExpired 记录已过期
	ValidationFailedExpired

	// ValidationFailedInvalidSeq 无效的 seq
	ValidationFailedInvalidSeq

	// ValidationFailedKeyFormat Key 格式错误
	ValidationFailedKeyFormat

	// ValidationFailedPublicKeyMismatch 公钥与 NodeID 不匹配
	ValidationFailedPublicKeyMismatch
)

// String 返回失败原因的字符串表示
func (r ValidationFailureReason) String() string {
	switch r {
	case ValidationOK:
		return "ok"
	case ValidationFailedSignature:
		return "invalid_signature"
	case ValidationFailedRealmMismatch:
		return "realm_mismatch"
	case ValidationFailedNodeMismatch:
		return "node_mismatch"
	case ValidationFailedExpired:
		return "expired"
	case ValidationFailedInvalidSeq:
		return "invalid_seq"
	case ValidationFailedKeyFormat:
		return "invalid_key_format"
	case ValidationFailedPublicKeyMismatch:
		return "public_key_mismatch"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              批量验证
// ============================================================================

// ValidateAndSelectBest 验证并选择最优记录
//
// 一站式函数：验证所有记录，返回最优的有效记录。
func ValidateAndSelectBest(key string, records []*SignedRealmPeerRecord) (*SignedRealmPeerRecord, error) {
	if len(records) == 0 {
		return nil, errors.New("no records to validate")
	}

	validator := NewDefaultPeerRecordValidator()

	// 过滤有效记录
	validRecords := make([]*SignedRealmPeerRecord, 0, len(records))
	for _, r := range records {
		if err := validator.Validate(key, r); err != nil {
			// 记录验证失败，跳过
			logger.Debug("记录验证失败", "nodeID", r.Record.NodeID, "error", err)
			continue
		}
		validRecords = append(validRecords, r)
	}

	if len(validRecords) == 0 {
		return nil, errors.New("no valid records")
	}

	// 选择最优
	best := validator.SelectBest(validRecords)
	if best == nil {
		return nil, errors.New("failed to select best record")
	}

	return best, nil
}

// ============================================================================
//                              冲突解决
// ============================================================================

// ShouldReplace 判断是否应该用新记录替换旧记录
//
// 替换规则：
//  1. 如果新记录无效，不替换
//  2. 如果旧记录为空，替换
//  3. 如果新记录 seq 更大，替换
//  4. 如果 seq 相同但新记录 timestamp 更新，替换
func ShouldReplace(key string, oldRecord, newRecord *SignedRealmPeerRecord) (bool, error) {
	validator := NewDefaultPeerRecordValidator()

	// 验证新记录
	if err := validator.Validate(key, newRecord); err != nil {
		return false, fmt.Errorf("new record validation failed: %w", err)
	}

	// 如果没有旧记录，直接替换
	if oldRecord == nil || oldRecord.Record == nil {
		return true, nil
	}

	// 比较 seq
	if newRecord.Record.Seq > oldRecord.Record.Seq {
		return true, nil
	}

	// seq 相同时，比较 timestamp
	if newRecord.Record.Seq == oldRecord.Record.Seq {
		if newRecord.Record.Timestamp > oldRecord.Record.Timestamp {
			return true, nil
		}
	}

	// 新记录 seq 更小，不替换
	return false, ErrSeqTooOld
}
