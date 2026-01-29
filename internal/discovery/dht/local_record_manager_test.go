package dht

import (
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/lib/crypto"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              LocalPeerRecordManager 测试
// ============================================================================

func TestLocalPeerRecordManager_Initialize(t *testing.T) {
	manager := NewLocalPeerRecordManager()

	// 未初始化
	if manager.IsInitialized() {
		t.Error("expected IsInitialized() to return false before initialization")
	}

	// 生成私钥
	privKey, _, err := crypto.GenerateKeyPair(crypto.KeyTypeEd25519)
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	// 初始化
	nodeID := types.NodeID("test-node-id")
	realmID := types.RealmID("test-realm")
	manager.Initialize(privKey, nodeID, realmID)

	// 已初始化
	if !manager.IsInitialized() {
		t.Error("expected IsInitialized() to return true after initialization")
	}
}

func TestLocalPeerRecordManager_SeqIncrement(t *testing.T) {
	manager := NewLocalPeerRecordManager()

	// 初始 seq 应为 0
	if seq := manager.CurrentSeq(); seq != 0 {
		t.Errorf("expected initial seq to be 0, got %d", seq)
	}

	// 递增
	seq1 := manager.NextSeq()
	if seq1 != 1 {
		t.Errorf("expected seq to be 1, got %d", seq1)
	}

	seq2 := manager.NextSeq()
	if seq2 != 2 {
		t.Errorf("expected seq to be 2, got %d", seq2)
	}

	// 设置
	manager.SetSeq(100)
	if seq := manager.CurrentSeq(); seq != 100 {
		t.Errorf("expected seq to be 100, got %d", seq)
	}
}

func TestLocalPeerRecordManager_CreateSignedRecord(t *testing.T) {
	manager := NewLocalPeerRecordManager()

	// 生成私钥
	privKey, _, err := crypto.GenerateKeyPair(crypto.KeyTypeEd25519)
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	nodeID := types.NodeID("test-node-id")
	realmID := types.RealmID("test-realm")
	manager.Initialize(privKey, nodeID, realmID)

	// 创建签名记录
	directAddrs := []string{"/ip4/192.168.1.1/tcp/4001"}
	relayAddrs := []string{"/ip4/10.0.0.1/tcp/4001/p2p-circuit/p2p/relay-id"}
	natType := types.NATTypeFullCone
	reachability := types.ReachabilityPublic
	capabilities := []string{"dht-server", "relay"}
	ttl := 1 * time.Hour

	signed, err := manager.CreateSignedRecord(
		directAddrs,
		relayAddrs,
		natType,
		reachability,
		capabilities,
		ttl,
	)

	if err != nil {
		t.Fatalf("CreateSignedRecord failed: %v", err)
	}

	// 验证记录
	if signed.Record.NodeID != nodeID {
		t.Errorf("expected NodeID %s, got %s", nodeID, signed.Record.NodeID)
	}

	if signed.Record.RealmID != realmID {
		t.Errorf("expected RealmID %s, got %s", realmID, signed.Record.RealmID)
	}

	if signed.Record.Seq != 1 {
		t.Errorf("expected Seq 1, got %d", signed.Record.Seq)
	}

	if len(signed.Record.DirectAddrs) != 1 {
		t.Errorf("expected 1 direct addr, got %d", len(signed.Record.DirectAddrs))
	}

	if len(signed.Record.RelayAddrs) != 1 {
		t.Errorf("expected 1 relay addr, got %d", len(signed.Record.RelayAddrs))
	}

	if len(signed.Record.Capabilities) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(signed.Record.Capabilities))
	}

	// 验证签名
	if len(signed.Signature) == 0 {
		t.Error("expected non-empty signature")
	}

	// 验证签名有效性
	if err := VerifySignedRealmPeerRecord(signed); err != nil {
		t.Errorf("signature verification failed: %v", err)
	}
}

func TestLocalPeerRecordManager_NeedsRepublish(t *testing.T) {
	manager := NewLocalPeerRecordManager()

	privKey, _, _ := crypto.GenerateKeyPair(crypto.KeyTypeEd25519)
	manager.Initialize(privKey, "node-id", "realm-id")

	directAddrs := []string{"/ip4/192.168.1.1/tcp/4001"}
	relayAddrs := []string{}
	interval := 20 * time.Minute

	// 从未发布过，应该需要发布
	needsRepublish, reason := manager.NeedsRepublish(interval, directAddrs, relayAddrs)
	if !needsRepublish {
		t.Error("expected NeedsRepublish to return true when never published")
	}
	if reason != "never published" {
		t.Errorf("expected reason 'never published', got '%s'", reason)
	}

	// 创建一个记录（模拟发布）
	_, err := manager.CreateSignedRecord(directAddrs, relayAddrs, types.NATTypeUnknown, types.ReachabilityUnknown, nil, time.Hour)
	if err != nil {
		t.Fatalf("CreateSignedRecord failed: %v", err)
	}

	// 刚刚发布，不需要重新发布
	needsRepublish, _ = manager.NeedsRepublish(interval, directAddrs, relayAddrs)
	if needsRepublish {
		t.Error("expected NeedsRepublish to return false right after publishing")
	}

	// 地址变化，应该需要发布
	newDirectAddrs := []string{"/ip4/192.168.1.2/tcp/4001"}
	needsRepublish, reason = manager.NeedsRepublish(interval, newDirectAddrs, relayAddrs)
	if !needsRepublish {
		t.Error("expected NeedsRepublish to return true when addresses changed")
	}
	if reason != "direct addrs changed" {
		t.Errorf("expected reason 'direct addrs changed', got '%s'", reason)
	}
}

func TestLocalPeerRecordManager_CreateWithoutInit(t *testing.T) {
	manager := NewLocalPeerRecordManager()

	// 未初始化时创建记录应该失败
	_, err := manager.CreateSignedRecord(nil, nil, types.NATTypeUnknown, types.ReachabilityUnknown, nil, time.Hour)
	if err == nil {
		t.Error("expected error when creating record without initialization")
	}
}

// TestLocalPeerRecordManager_SetRealmID 测试 SetRealmID 方法
//
// Step B3 对齐：验证 Realm Join 后可以切换发布目标
func TestLocalPeerRecordManager_SetRealmID(t *testing.T) {
	manager := NewLocalPeerRecordManager()

	// 生成私钥
	privKey, _, err := crypto.GenerateKeyPair(crypto.KeyTypeEd25519)
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	nodeID := types.NodeID("test-node-id")
	initialRealmID := types.RealmID("initial-realm")

	// 初始化（全局模式，空 RealmID）
	manager.Initialize(privKey, nodeID, "")

	// 验证初始状态（无 RealmID）
	realmID, hasRealm := manager.RealmID()
	if hasRealm {
		t.Error("expected hasRealm to be false before SetRealmID")
	}
	if realmID != "" {
		t.Errorf("expected empty realmID, got %s", realmID)
	}

	// 设置 RealmID（模拟 Realm Join）
	manager.SetRealmID(initialRealmID)

	// 验证 RealmID 已设置
	realmID, hasRealm = manager.RealmID()
	if !hasRealm {
		t.Error("expected hasRealm to be true after SetRealmID")
	}
	if realmID != initialRealmID {
		t.Errorf("expected realmID %s, got %s", initialRealmID, realmID)
	}

	// 创建记录，验证包含新的 RealmID
	signed, err := manager.CreateSignedRecord(
		[]string{"/ip4/192.168.1.1/tcp/4001"},
		nil,
		types.NATTypeUnknown,
		types.ReachabilityUnknown,
		nil,
		time.Hour,
	)
	if err != nil {
		t.Fatalf("CreateSignedRecord failed: %v", err)
	}
	if signed.Record.RealmID != initialRealmID {
		t.Errorf("expected record RealmID %s, got %s", initialRealmID, signed.Record.RealmID)
	}

	// 切换到新的 RealmID（模拟切换 Realm）
	newRealmID := types.RealmID("new-realm")
	manager.SetRealmID(newRealmID)

	realmID, _ = manager.RealmID()
	if realmID != newRealmID {
		t.Errorf("expected realmID %s after switch, got %s", newRealmID, realmID)
	}

	// 创建新记录，验证使用新的 RealmID
	signed2, err := manager.CreateSignedRecord(
		[]string{"/ip4/192.168.1.2/tcp/4001"},
		nil,
		types.NATTypeUnknown,
		types.ReachabilityUnknown,
		nil,
		time.Hour,
	)
	if err != nil {
		t.Fatalf("CreateSignedRecord failed: %v", err)
	}
	if signed2.Record.RealmID != newRealmID {
		t.Errorf("expected record RealmID %s, got %s", newRealmID, signed2.Record.RealmID)
	}
}

// ============================================================================
//                              AddressChangeDetector 测试
// ============================================================================

func TestAddressChangeDetector_DetectsChange(t *testing.T) {
	changeCount := 0
	var lastOldAddrs, lastNewAddrs []string

	detector := NewAddressChangeDetector(func(oldAddrs, newAddrs []string) {
		changeCount++
		lastOldAddrs = oldAddrs
		lastNewAddrs = newAddrs
	})

	// 初始地址
	initialAddrs := []string{"/ip4/192.168.1.1/tcp/4001"}
	changed := detector.Check(initialAddrs)
	if !changed {
		t.Error("expected first check to detect change (from empty)")
	}
	if changeCount != 1 {
		t.Errorf("expected changeCount 1, got %d", changeCount)
	}

	// 相同地址，不应该检测到变化
	changed = detector.Check(initialAddrs)
	if changed {
		t.Error("expected no change with same addresses")
	}
	if changeCount != 1 {
		t.Errorf("expected changeCount still 1, got %d", changeCount)
	}

	// 新地址
	newAddrs := []string{"/ip4/192.168.1.2/tcp/4001"}
	changed = detector.Check(newAddrs)
	if !changed {
		t.Error("expected change with new addresses")
	}
	if changeCount != 2 {
		t.Errorf("expected changeCount 2, got %d", changeCount)
	}
	if len(lastOldAddrs) != 1 || lastOldAddrs[0] != "/ip4/192.168.1.1/tcp/4001" {
		t.Errorf("unexpected old addrs: %v", lastOldAddrs)
	}
	if len(lastNewAddrs) != 1 || lastNewAddrs[0] != "/ip4/192.168.1.2/tcp/4001" {
		t.Errorf("unexpected new addrs: %v", lastNewAddrs)
	}
}

func TestAddressChangeDetector_OrderIndependent(t *testing.T) {
	detector := NewAddressChangeDetector(nil)

	// 设置初始地址
	addrs1 := []string{"/ip4/1.1.1.1/tcp/4001", "/ip4/2.2.2.2/tcp/4001"}
	detector.Check(addrs1)

	// 相同地址，不同顺序
	addrs2 := []string{"/ip4/2.2.2.2/tcp/4001", "/ip4/1.1.1.1/tcp/4001"}
	changed := detector.Check(addrs2)
	if changed {
		t.Error("expected no change with same addresses in different order")
	}
}

func TestAddressChangeDetector_LastAddrs(t *testing.T) {
	detector := NewAddressChangeDetector(nil)

	addrs := []string{"/ip4/192.168.1.1/tcp/4001"}
	detector.Check(addrs)

	lastAddrs := detector.LastAddrs()
	if len(lastAddrs) != 1 || lastAddrs[0] != "/ip4/192.168.1.1/tcp/4001" {
		t.Errorf("unexpected last addrs: %v", lastAddrs)
	}
}

// ============================================================================
//                              stringSliceEqual 测试
// ============================================================================

func TestStringSliceEqual(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []string
		expected bool
	}{
		{
			name:     "both empty",
			a:        []string{},
			b:        []string{},
			expected: true,
		},
		{
			name:     "both nil",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "same elements same order",
			a:        []string{"a", "b", "c"},
			b:        []string{"a", "b", "c"},
			expected: true,
		},
		{
			name:     "same elements different order",
			a:        []string{"a", "b", "c"},
			b:        []string{"c", "a", "b"},
			expected: true,
		},
		{
			name:     "different lengths",
			a:        []string{"a", "b"},
			b:        []string{"a", "b", "c"},
			expected: false,
		},
		{
			name:     "different elements",
			a:        []string{"a", "b", "c"},
			b:        []string{"a", "b", "d"},
			expected: false,
		},
		{
			name:     "duplicates handled",
			a:        []string{"a", "a", "b"},
			b:        []string{"a", "b", "a"},
			expected: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := stringSliceEqual(tc.a, tc.b)
			if result != tc.expected {
				t.Errorf("stringSliceEqual(%v, %v) = %v, expected %v", tc.a, tc.b, result, tc.expected)
			}
		})
	}
}

// TestLocalPeerRecordManager_NodeID 测试 NodeID 方法
func TestLocalPeerRecordManager_NodeID(t *testing.T) {
	manager := NewLocalPeerRecordManager()

	// 未初始化时应返回空字符串
	nodeID := manager.NodeID()
	if nodeID != "" {
		t.Errorf("expected empty nodeID before initialization, got %s", nodeID)
	}

	// 生成私钥并初始化
	privKey, _, err := crypto.GenerateKeyPair(crypto.KeyTypeEd25519)
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	expectedNodeID := types.NodeID("test-node-id")
	manager.Initialize(privKey, expectedNodeID, "test-realm")

	// 初始化后应返回正确的 NodeID
	nodeID = manager.NodeID()
	if nodeID != expectedNodeID {
		t.Errorf("expected nodeID %s, got %s", expectedNodeID, nodeID)
	}
}

// TestLocalPeerRecordManager_Clear 测试 Clear 方法
//
// Phase D 对齐：验证优雅关闭时可以清理管理器状态
func TestLocalPeerRecordManager_Clear(t *testing.T) {
	manager := NewLocalPeerRecordManager()

	// 生成私钥并初始化
	privKey, _, err := crypto.GenerateKeyPair(crypto.KeyTypeEd25519)
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	nodeID := types.NodeID("test-node-id")
	realmID := types.RealmID("test-realm")
	manager.Initialize(privKey, nodeID, realmID)

	// 创建一个记录
	_, err = manager.CreateSignedRecord(
		[]string{"/ip4/192.168.1.1/tcp/4001"},
		nil,
		types.NATTypeUnknown,
		types.ReachabilityUnknown,
		nil,
		time.Hour,
	)
	if err != nil {
		t.Fatalf("CreateSignedRecord failed: %v", err)
	}

	// 验证初始状态
	if !manager.IsInitialized() {
		t.Error("expected IsInitialized() to return true before Clear")
	}
	if manager.LastRecord() == nil {
		t.Error("expected LastRecord() to return non-nil before Clear")
	}

	// 清理
	manager.Clear()

	// 验证清理后的状态
	if manager.IsInitialized() {
		t.Error("expected IsInitialized() to return false after Clear")
	}
	if manager.LastRecord() != nil {
		t.Error("expected LastRecord() to return nil after Clear")
	}
	if !manager.LastPublishTime().IsZero() {
		t.Error("expected LastPublishTime() to return zero time after Clear")
	}

	// 清理后 NodeID 和 RealmID 应该保留（用于 UnpublishPeerRecord）
	if manager.NodeID() != nodeID {
		t.Errorf("expected nodeID %s after Clear, got %s", nodeID, manager.NodeID())
	}
	storedRealmID, hasRealm := manager.RealmID()
	if !hasRealm || storedRealmID != realmID {
		t.Errorf("expected realmID %s after Clear, got %s (hasRealm=%v)", realmID, storedRealmID, hasRealm)
	}

	// 清理后创建记录应该失败（因为 privKey 已清空）
	_, err = manager.CreateSignedRecord(nil, nil, types.NATTypeUnknown, types.ReachabilityUnknown, nil, time.Hour)
	if err == nil {
		t.Error("expected error when creating record after Clear")
	}
}
