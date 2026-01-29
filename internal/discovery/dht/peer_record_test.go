package dht

import (
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/lib/crypto"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              RealmPeerRecord 序列化测试
// ============================================================================

func TestRealmPeerRecord_Marshal(t *testing.T) {
	record := &RealmPeerRecord{
		NodeID:       types.NodeID("test-node-id"),
		RealmID:      types.RealmID("test-realm-id"),
		DirectAddrs:  []string{"/ip4/1.2.3.4/tcp/4001"},
		RelayAddrs:   []string{"/p2p/relay-id/p2p-circuit/p2p/test-node-id"},
		NATType:      types.NATTypeFullCone,
		Reachability: types.ReachabilityPublic,
		Capabilities: []string{"relay", "dht-server"},
		Seq:          1,
		Timestamp:    time.Now().UnixNano(),
		TTL:          int64(DefaultPeerRecordTTL / time.Millisecond),
	}

	// 序列化
	data, err := record.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("Marshal returned empty data")
	}

	// 反序列化
	decoded, err := UnmarshalRealmPeerRecord(data)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	// 验证字段
	if decoded.NodeID != record.NodeID {
		t.Errorf("NodeID mismatch: got %s, want %s", decoded.NodeID, record.NodeID)
	}
	if decoded.RealmID != record.RealmID {
		t.Errorf("RealmID mismatch: got %s, want %s", decoded.RealmID, record.RealmID)
	}
	if len(decoded.DirectAddrs) != len(record.DirectAddrs) {
		t.Errorf("DirectAddrs count mismatch: got %d, want %d", len(decoded.DirectAddrs), len(record.DirectAddrs))
	}
	if len(decoded.RelayAddrs) != len(record.RelayAddrs) {
		t.Errorf("RelayAddrs count mismatch: got %d, want %d", len(decoded.RelayAddrs), len(record.RelayAddrs))
	}
	if decoded.NATType != record.NATType {
		t.Errorf("NATType mismatch: got %v, want %v", decoded.NATType, record.NATType)
	}
	if decoded.Reachability != record.Reachability {
		t.Errorf("Reachability mismatch: got %v, want %v", decoded.Reachability, record.Reachability)
	}
	if len(decoded.Capabilities) != len(record.Capabilities) {
		t.Errorf("Capabilities count mismatch: got %d, want %d", len(decoded.Capabilities), len(record.Capabilities))
	}
	if decoded.Seq != record.Seq {
		t.Errorf("Seq mismatch: got %d, want %d", decoded.Seq, record.Seq)
	}
	if decoded.Timestamp != record.Timestamp {
		t.Errorf("Timestamp mismatch: got %d, want %d", decoded.Timestamp, record.Timestamp)
	}
	if decoded.TTL != record.TTL {
		t.Errorf("TTL mismatch: got %d, want %d", decoded.TTL, record.TTL)
	}
}

func TestRealmPeerRecord_MarshalEmpty(t *testing.T) {
	record := &RealmPeerRecord{
		NodeID:    types.NodeID("test-node-id"),
		RealmID:   types.RealmID("test-realm-id"),
		Seq:       1,
		Timestamp: time.Now().UnixNano(),
		TTL:       int64(DefaultPeerRecordTTL / time.Millisecond),
	}

	data, err := record.Marshal()
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	decoded, err := UnmarshalRealmPeerRecord(data)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.NodeID != record.NodeID {
		t.Errorf("NodeID mismatch")
	}
	if len(decoded.DirectAddrs) != 0 {
		t.Errorf("Expected empty DirectAddrs")
	}
	if len(decoded.RelayAddrs) != 0 {
		t.Errorf("Expected empty RelayAddrs")
	}
}

// ============================================================================
//                              SignedRealmPeerRecord 测试
// ============================================================================

func TestSignedRealmPeerRecord_SignAndVerify(t *testing.T) {
	// 生成密钥对
	privKey, _, err := crypto.GenerateKeyPair(crypto.KeyTypeEd25519)
	if err != nil {
		t.Fatalf("Failed to generate key pair: %v", err)
	}

	// 从公钥派生 PeerID
	pubKeyBytes, _ := privKey.GetPublic().Raw()
	peerID, _ := types.PeerIDFromPublicKey(pubKeyBytes)

	record := &RealmPeerRecord{
		NodeID:       types.NodeID(peerID),
		RealmID:      types.RealmID("test-realm-id"),
		DirectAddrs:  []string{"/ip4/1.2.3.4/tcp/4001"},
		RelayAddrs:   []string{"/p2p/relay-id/p2p-circuit/p2p/" + string(peerID)},
		NATType:      types.NATTypeFullCone,
		Reachability: types.ReachabilityPublic,
		Capabilities: []string{"relay"},
		Seq:          1,
		Timestamp:    time.Now().UnixNano(),
		TTL:          int64(DefaultPeerRecordTTL / time.Millisecond),
	}

	// 签名
	signed, err := SignRealmPeerRecord(privKey, record)
	if err != nil {
		t.Fatalf("Sign failed: %v", err)
	}

	// 验证签名
	if err := VerifySignedRealmPeerRecord(signed); err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	// 序列化和反序列化
	data, err := signed.Marshal()
	if err != nil {
		t.Fatalf("Marshal signed record failed: %v", err)
	}

	decoded, err := UnmarshalSignedRealmPeerRecord(data)
	if err != nil {
		t.Fatalf("Unmarshal signed record failed: %v", err)
	}

	// 验证反序列化后的签名
	if err := VerifySignedRealmPeerRecord(decoded); err != nil {
		t.Fatalf("Verify decoded failed: %v", err)
	}
}

func TestSignedRealmPeerRecord_InvalidSignature(t *testing.T) {
	// 生成两个密钥对
	privKey1, _, _ := crypto.GenerateKeyPair(crypto.KeyTypeEd25519)
	privKey2, _, _ := crypto.GenerateKeyPair(crypto.KeyTypeEd25519)

	pubKeyBytes, _ := privKey1.GetPublic().Raw()
	peerID, _ := types.PeerIDFromPublicKey(pubKeyBytes)

	record := &RealmPeerRecord{
		NodeID:    types.NodeID(peerID),
		RealmID:   types.RealmID("test-realm-id"),
		Seq:       1,
		Timestamp: time.Now().UnixNano(),
		TTL:       int64(DefaultPeerRecordTTL / time.Millisecond),
	}

	// 用 privKey1 签名
	signed, _ := SignRealmPeerRecord(privKey1, record)

	// 替换为 privKey2 的公钥（模拟篡改）
	signed.PublicKey = privKey2.GetPublic()

	// 验证应该失败
	if err := VerifySignedRealmPeerRecord(signed); err == nil {
		t.Error("Expected verification to fail with wrong public key")
	}
}

func TestSignedRealmPeerRecord_SeqZero(t *testing.T) {
	privKey, _, _ := crypto.GenerateKeyPair(crypto.KeyTypeEd25519)

	pubKeyBytes, _ := privKey.GetPublic().Raw()
	peerID, _ := types.PeerIDFromPublicKey(pubKeyBytes)

	record := &RealmPeerRecord{
		NodeID:    types.NodeID(peerID),
		RealmID:   types.RealmID("test-realm-id"),
		Seq:       0, // 无效的 seq
		Timestamp: time.Now().UnixNano(),
		TTL:       int64(DefaultPeerRecordTTL / time.Millisecond),
	}

	// 签名应该失败
	_, err := SignRealmPeerRecord(privKey, record)
	if err != ErrInvalidSeq {
		t.Errorf("Expected ErrInvalidSeq, got %v", err)
	}
}

// ============================================================================
//                              PeerRecord 辅助方法测试
// ============================================================================

func TestRealmPeerRecord_IsExpired(t *testing.T) {
	// 未过期的记录
	record := &RealmPeerRecord{
		NodeID:    types.NodeID("test"),
		Timestamp: time.Now().UnixNano(),
		TTL:       int64(DefaultPeerRecordTTL / time.Millisecond),
	}

	if record.IsExpired() {
		t.Error("Record should not be expired")
	}

	// 过期的记录
	expiredRecord := &RealmPeerRecord{
		NodeID:    types.NodeID("test"),
		Timestamp: time.Now().Add(-2 * time.Hour).UnixNano(),
		TTL:       int64(1 * time.Hour / time.Millisecond),
	}

	if !expiredRecord.IsExpired() {
		t.Error("Record should be expired")
	}
}

func TestRealmPeerRecord_AllAddrs(t *testing.T) {
	record := &RealmPeerRecord{
		DirectAddrs: []string{"/ip4/1.2.3.4/tcp/4001", "/ip4/5.6.7.8/tcp/4001"},
		RelayAddrs:  []string{"/p2p/relay/p2p-circuit/p2p/target"},
	}

	addrs := record.AllAddrs()
	if len(addrs) != 3 {
		t.Errorf("Expected 3 addrs, got %d", len(addrs))
	}
}

func TestRealmPeerRecord_HasCapability(t *testing.T) {
	record := &RealmPeerRecord{
		Capabilities: []string{"relay", "dht-server"},
	}

	if !record.HasCapability("relay") {
		t.Error("Should have relay capability")
	}
	if !record.HasCapability("dht-server") {
		t.Error("Should have dht-server capability")
	}
	if record.HasCapability("unknown") {
		t.Error("Should not have unknown capability")
	}
}
