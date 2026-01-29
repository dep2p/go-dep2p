package dht

import (
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/lib/crypto"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              创建测试记录的辅助函数
// ============================================================================

func createTestSignedRecord(t *testing.T, seq uint64, ttlMs int64) (*SignedRealmPeerRecord, crypto.PrivateKey, string) {
	t.Helper()

	privKey, _, err := crypto.GenerateKeyPair(crypto.KeyTypeEd25519)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// 
	peerID, err := crypto.PeerIDFromPublicKey(privKey.GetPublic())
	if err != nil {
		t.Fatalf("Failed to derive PeerID: %v", err)
	}
	realmID := types.RealmID("test-realm")

	record := &RealmPeerRecord{
		NodeID:       types.NodeID(peerID),
		RealmID:      realmID,
		DirectAddrs:  []string{"/ip4/1.2.3.4/tcp/4001"},
		RelayAddrs:   []string{},
		NATType:      types.NATTypeFullCone,
		Reachability: types.ReachabilityPublic,
		Capabilities: []string{},
		Seq:          seq,
		Timestamp:    time.Now().UnixNano(),
		TTL:          ttlMs,
	}

	signed, err := SignRealmPeerRecord(privKey, record)
	if err != nil {
		t.Fatalf("Failed to sign record: %v", err)
	}

	key := RealmPeerKey(realmID, types.NodeID(peerID))
	return signed, privKey, key
}

// ============================================================================
//                              Validator 测试
// ============================================================================

func TestDefaultValidator_Validate_Valid(t *testing.T) {
	signed, _, key := createTestSignedRecord(t, 1, int64(DefaultPeerRecordTTL/time.Millisecond))

	validator := NewDefaultPeerRecordValidator()
	err := validator.Validate(key, signed)
	if err != nil {
		t.Errorf("Validation should pass: %v", err)
	}
}

func TestDefaultValidator_Validate_InvalidSignature(t *testing.T) {
	signed, _, key := createTestSignedRecord(t, 1, int64(DefaultPeerRecordTTL/time.Millisecond))

	// 篡改签名
	signed.Signature[0] ^= 0xFF

	validator := NewDefaultPeerRecordValidator()
	err := validator.Validate(key, signed)
	if err == nil {
		t.Error("Validation should fail with invalid signature")
	}
}

func TestDefaultValidator_Validate_RealmMismatch(t *testing.T) {
	signed, _, _ := createTestSignedRecord(t, 1, int64(DefaultPeerRecordTTL/time.Millisecond))

	// 使用错误的 key（不同的 RealmID）
	wrongKey := RealmPeerKey(types.RealmID("wrong-realm"), signed.Record.NodeID)

	validator := NewDefaultPeerRecordValidator()
	err := validator.Validate(wrongKey, signed)
	if err == nil {
		t.Error("Validation should fail with realm mismatch")
	}
}

func TestDefaultValidator_Validate_NodeMismatch(t *testing.T) {
	signed, _, _ := createTestSignedRecord(t, 1, int64(DefaultPeerRecordTTL/time.Millisecond))

	// 使用错误的 key（不同的 NodeID）
	wrongKey := RealmPeerKey(signed.Record.RealmID, types.NodeID("wrong-node"))

	validator := NewDefaultPeerRecordValidator()
	err := validator.Validate(wrongKey, signed)
	if err == nil {
		t.Error("Validation should fail with node mismatch")
	}
}

func TestDefaultValidator_Validate_Expired(t *testing.T) {
	privKey, _, _ := crypto.GenerateKeyPair(crypto.KeyTypeEd25519)
	pubKeyBytes, _ := privKey.GetPublic().Raw()
	peerID, _ := types.PeerIDFromPublicKey(pubKeyBytes)
	realmID := types.RealmID("test-realm")

	// 创建过期的记录
	record := &RealmPeerRecord{
		NodeID:    types.NodeID(peerID),
		RealmID:   realmID,
		Seq:       1,
		Timestamp: time.Now().Add(-2 * time.Hour).UnixNano(), // 2 小时前
		TTL:       int64(1 * time.Hour / time.Millisecond),   // TTL 1 小时
	}

	signed, _ := SignRealmPeerRecord(privKey, record)
	key := RealmPeerKey(realmID, types.NodeID(peerID))

	validator := NewDefaultPeerRecordValidator()
	err := validator.Validate(key, signed)
	if err == nil {
		t.Error("Validation should fail with expired record")
	}
}

func TestDefaultValidator_Validate_InvalidSeq(t *testing.T) {
	privKey, _, _ := crypto.GenerateKeyPair(crypto.KeyTypeEd25519)
	pubKeyBytes, _ := privKey.GetPublic().Raw()
	peerID, _ := types.PeerIDFromPublicKey(pubKeyBytes)
	realmID := types.RealmID("test-realm")

	// 手动创建 seq=0 的记录（绕过 SignRealmPeerRecord 的验证）
	record := &RealmPeerRecord{
		NodeID:    types.NodeID(peerID),
		RealmID:   realmID,
		Seq:       0, // 无效
		Timestamp: time.Now().UnixNano(),
		TTL:       int64(DefaultPeerRecordTTL / time.Millisecond),
	}

	rawRecord, _ := record.Marshal()
	toSign := append(PeerRecordPayloadType, rawRecord...)
	sig, _ := privKey.Sign(toSign)

	signed := &SignedRealmPeerRecord{
		Record:    record,
		RawRecord: rawRecord,
		PublicKey: privKey.GetPublic(),
		Signature: sig,
	}

	key := RealmPeerKey(realmID, types.NodeID(peerID))

	validator := NewDefaultPeerRecordValidator()
	err := validator.Validate(key, signed)
	if err == nil {
		t.Error("Validation should fail with seq=0")
	}
}

func TestDefaultValidator_Validate_TTLTooShort(t *testing.T) {
	privKey, _, _ := crypto.GenerateKeyPair(crypto.KeyTypeEd25519)
	pubKeyBytes, _ := privKey.GetPublic().Raw()
	peerID, _ := types.PeerIDFromPublicKey(pubKeyBytes)
	realmID := types.RealmID("test-realm")

	record := &RealmPeerRecord{
		NodeID:    types.NodeID(peerID),
		RealmID:   realmID,
		Seq:       1,
		Timestamp: time.Now().UnixNano(),
		TTL:       int64(1 * time.Minute / time.Millisecond), // TTL 太短
	}

	signed, _ := SignRealmPeerRecord(privKey, record)
	key := RealmPeerKey(realmID, types.NodeID(peerID))

	validator := NewDefaultPeerRecordValidator()
	err := validator.Validate(key, signed)
	if err == nil {
		t.Error("Validation should fail with TTL too short")
	}
}

// ============================================================================
//                              SelectBest 测试
// ============================================================================

func TestDefaultValidator_SelectBest_HigherSeq(t *testing.T) {
	signed1, _, _ := createTestSignedRecord(t, 1, int64(DefaultPeerRecordTTL/time.Millisecond))
	signed2, _, _ := createTestSignedRecord(t, 2, int64(DefaultPeerRecordTTL/time.Millisecond))
	signed3, _, _ := createTestSignedRecord(t, 3, int64(DefaultPeerRecordTTL/time.Millisecond))

	validator := NewDefaultPeerRecordValidator()
	best := validator.SelectBest([]*SignedRealmPeerRecord{signed1, signed3, signed2})

	if best.Record.Seq != 3 {
		t.Errorf("Expected seq 3, got %d", best.Record.Seq)
	}
}

func TestDefaultValidator_SelectBest_PreferValid(t *testing.T) {
	// 有效记录 seq=1
	validSigned, _, _ := createTestSignedRecord(t, 1, int64(DefaultPeerRecordTTL/time.Millisecond))

	// 过期记录 seq=2（更高的 seq，但已过期）
	privKey, _, _ := crypto.GenerateKeyPair(crypto.KeyTypeEd25519)
	pubKeyBytes, _ := privKey.GetPublic().Raw()
	peerID, _ := types.PeerIDFromPublicKey(pubKeyBytes)
	realmID := types.RealmID("test-realm")

	expiredRecord := &RealmPeerRecord{
		NodeID:    types.NodeID(peerID),
		RealmID:   realmID,
		Seq:       2,
		Timestamp: time.Now().Add(-2 * time.Hour).UnixNano(),
		TTL:       int64(1 * time.Hour / time.Millisecond),
	}
	expiredSigned, _ := SignRealmPeerRecord(privKey, expiredRecord)

	validator := NewDefaultPeerRecordValidator()
	best := validator.SelectBest([]*SignedRealmPeerRecord{expiredSigned, validSigned})

	// 应该选择有效的记录，即使 seq 较小
	if best.Record.Seq != 1 {
		t.Errorf("Expected to prefer valid record with seq 1, got %d", best.Record.Seq)
	}
}

func TestDefaultValidator_SelectBest_Empty(t *testing.T) {
	validator := NewDefaultPeerRecordValidator()
	best := validator.SelectBest([]*SignedRealmPeerRecord{})

	if best != nil {
		t.Error("Expected nil for empty input")
	}
}

// ============================================================================
//                              ShouldReplace 测试
// ============================================================================

func TestShouldReplace_NewIsHigherSeq(t *testing.T) {
	// 创建正确匹配的记录
	signed1, privKey1, key1 := createTestSignedRecord(t, 1, int64(DefaultPeerRecordTTL/time.Millisecond))

	// 创建更高 seq 的记录（同一个私钥）
	record2 := &RealmPeerRecord{
		NodeID:      signed1.Record.NodeID,
		RealmID:     signed1.Record.RealmID,
		DirectAddrs: signed1.Record.DirectAddrs,
		Seq:         2,
		Timestamp:   time.Now().UnixNano(),
		TTL:         int64(DefaultPeerRecordTTL / time.Millisecond),
	}
	signed2, _ := SignRealmPeerRecord(privKey1, record2)

	replace, err := ShouldReplace(key1, signed1, signed2)
	if err != nil {
		t.Errorf("ShouldReplace failed: %v", err)
	}
	if !replace {
		t.Error("Should replace with higher seq")
	}
}

func TestShouldReplace_NewIsLowerSeq(t *testing.T) {
	signed1, privKey1, key1 := createTestSignedRecord(t, 2, int64(DefaultPeerRecordTTL/time.Millisecond))

	// 创建更低 seq 的记录
	record2 := &RealmPeerRecord{
		NodeID:    signed1.Record.NodeID,
		RealmID:   signed1.Record.RealmID,
		Seq:       1,
		Timestamp: time.Now().UnixNano(),
		TTL:       int64(DefaultPeerRecordTTL / time.Millisecond),
	}
	signed2, _ := SignRealmPeerRecord(privKey1, record2)

	replace, err := ShouldReplace(key1, signed1, signed2)
	if replace {
		t.Error("Should not replace with lower seq")
	}
	if err != ErrSeqTooOld {
		t.Errorf("Expected ErrSeqTooOld, got %v", err)
	}
}

func TestShouldReplace_OldIsNil(t *testing.T) {
	signed, _, key := createTestSignedRecord(t, 1, int64(DefaultPeerRecordTTL/time.Millisecond))

	replace, err := ShouldReplace(key, nil, signed)
	if err != nil {
		t.Errorf("ShouldReplace failed: %v", err)
	}
	if !replace {
		t.Error("Should replace when old is nil")
	}
}
