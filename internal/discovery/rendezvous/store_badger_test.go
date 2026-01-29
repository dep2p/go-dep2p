package rendezvous

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dep2p/go-dep2p/internal/core/storage/engine/badger"
	"github.com/dep2p/go-dep2p/internal/core/storage/kv"
	"github.com/dep2p/go-dep2p/pkg/types"
)

func newTestBadgerStore(t *testing.T) (*BadgerStore, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := engine.DefaultConfig(dbPath)
	eng, err := badger.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	kvStore := kv.New(eng, []byte("r/"))
	store, err := NewBadgerStore(kvStore, DefaultStoreConfig())
	if err != nil {
		eng.Close()
		t.Fatalf("failed to create badger store: %v", err)
	}

	cleanup := func() {
		store.Close()
		eng.Close()
	}

	return store, cleanup
}

func newTestPeerInfo(id string) types.PeerInfo {
	addr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	return types.PeerInfo{
		ID:    types.PeerID(id),
		Addrs: []types.Multiaddr{addr},
	}
}

func TestBadgerStore_AddAndGet(t *testing.T) {
	store, cleanup := newTestBadgerStore(t)
	defer cleanup()

	// 添加注册
	peerInfo := newTestPeerInfo("peer-1")
	err := store.Add("test-ns", peerInfo, time.Hour)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// 查询
	regs, _, err := store.Get("test-ns", 10, nil)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if len(regs) != 1 {
		t.Fatalf("expected 1 registration, got %d", len(regs))
	}

	if regs[0].PeerInfo.ID != peerInfo.ID {
		t.Errorf("PeerID mismatch: got %s, want %s", regs[0].PeerInfo.ID, peerInfo.ID)
	}
}

func TestBadgerStore_Remove(t *testing.T) {
	store, cleanup := newTestBadgerStore(t)
	defer cleanup()

	// 添加
	peerInfo := newTestPeerInfo("peer-1")
	store.Add("test-ns", peerInfo, time.Hour)

	// 移除
	store.Remove("test-ns", peerInfo.ID)

	// 验证已移除
	regs, _, _ := store.Get("test-ns", 10, nil)
	if len(regs) != 0 {
		t.Error("expected 0 registrations after remove")
	}
}

func TestBadgerStore_TTLExpiry(t *testing.T) {
	store, cleanup := newTestBadgerStore(t)
	defer cleanup()

	// 添加短 TTL 的注册
	peerInfo := newTestPeerInfo("peer-1")
	err := store.Add("test-ns", peerInfo, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// 等待过期
	time.Sleep(50 * time.Millisecond)

	// 查询应返回空
	regs, _, _ := store.Get("test-ns", 10, nil)
	if len(regs) != 0 {
		t.Error("expected 0 registrations after expiry")
	}
}

func TestBadgerStore_CleanupExpired(t *testing.T) {
	store, cleanup := newTestBadgerStore(t)
	defer cleanup()

	// 添加短 TTL 的注册
	store.Add("test-ns", newTestPeerInfo("peer-1"), 10*time.Millisecond)
	store.Add("test-ns", newTestPeerInfo("peer-2"), time.Hour)

	// 等待部分过期
	time.Sleep(50 * time.Millisecond)

	// 清理
	expired := store.CleanupExpired()
	if expired != 1 {
		t.Errorf("expected 1 expired, got %d", expired)
	}

	// 验证剩余
	regs, _, _ := store.Get("test-ns", 10, nil)
	if len(regs) != 1 {
		t.Errorf("expected 1 registration, got %d", len(regs))
	}
}

func TestBadgerStore_Pagination(t *testing.T) {
	store, cleanup := newTestBadgerStore(t)
	defer cleanup()

	// 添加多个注册
	for i := 0; i < 5; i++ {
		peerInfo := newTestPeerInfo("peer-" + string(rune('a'+i)))
		store.Add("test-ns", peerInfo, time.Hour)
	}

	// 分页查询
	regs1, cookie, _ := store.Get("test-ns", 2, nil)
	if len(regs1) != 2 {
		t.Errorf("expected 2 registrations in first page, got %d", len(regs1))
	}

	if cookie == nil {
		t.Error("expected cookie for next page")
	}

	regs2, _, _ := store.Get("test-ns", 2, cookie)
	if len(regs2) != 2 {
		t.Errorf("expected 2 registrations in second page, got %d", len(regs2))
	}
}

func TestBadgerStore_GetPeerNamespaces(t *testing.T) {
	store, cleanup := newTestBadgerStore(t)
	defer cleanup()

	peerInfo := newTestPeerInfo("peer-1")

	// 在多个命名空间注册
	store.Add("ns-1", peerInfo, time.Hour)
	store.Add("ns-2", peerInfo, time.Hour)

	// 查询命名空间
	namespaces := store.GetPeerNamespaces(peerInfo.ID)
	if len(namespaces) != 2 {
		t.Errorf("expected 2 namespaces, got %d", len(namespaces))
	}
}

func TestBadgerStore_Persistence(t *testing.T) {
	// 创建引擎
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "persist.db")
	cfg := engine.DefaultConfig(dbPath)
	eng, err := badger.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer eng.Close()

	kvStore := kv.New(eng, []byte("r/"))
	peerInfo := newTestPeerInfo("peer-1")

	// 创建第一个存储并写入数据
	store1, err := NewBadgerStore(kvStore, DefaultStoreConfig())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	store1.Add("test-ns", peerInfo, time.Hour)
	store1.Close()

	// 创建第二个存储，验证数据持久化
	store2, err := NewBadgerStore(kvStore, DefaultStoreConfig())
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store2.Close()

	regs, _, err := store2.Get("test-ns", 10, nil)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if len(regs) != 1 {
		t.Fatal("expected 1 registration after persistence")
	}

	if regs[0].PeerInfo.ID != peerInfo.ID {
		t.Error("PeerID mismatch after persistence")
	}
}

func TestBadgerStore_Stats(t *testing.T) {
	store, cleanup := newTestBadgerStore(t)
	defer cleanup()

	// 添加注册
	store.Add("ns-1", newTestPeerInfo("peer-1"), time.Hour)
	store.Add("ns-1", newTestPeerInfo("peer-2"), time.Hour)
	store.Add("ns-2", newTestPeerInfo("peer-3"), time.Hour)

	stats := store.Stats()

	if stats.TotalRegistrations != 3 {
		t.Errorf("expected 3 registrations, got %d", stats.TotalRegistrations)
	}

	if stats.TotalNamespaces != 2 {
		t.Errorf("expected 2 namespaces, got %d", stats.TotalNamespaces)
	}
}

func TestBadgerStore_Limits(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "limits.db")
	cfg := engine.DefaultConfig(dbPath)
	eng, err := badger.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer eng.Close()

	// 设置严格限制
	config := StoreConfig{
		MaxRegistrations:             2,
		MaxNamespaces:                1,
		MaxRegistrationsPerNamespace: 2,
		MaxRegistrationsPerPeer:      1,
		MaxTTL:                       time.Hour,
		DefaultTTL:                   time.Hour,
		CleanupInterval:              time.Minute,
	}

	kvStore := kv.New(eng, []byte("r/"))
	store, err := NewBadgerStore(kvStore, config)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	// 测试命名空间限制
	store.Add("ns-1", newTestPeerInfo("peer-1"), time.Hour)
	err = store.Add("ns-2", newTestPeerInfo("peer-2"), time.Hour)
	if err != ErrMaxNamespacesExceeded {
		t.Errorf("expected ErrMaxNamespacesExceeded, got %v", err)
	}

	// 测试每节点限制
	err = store.Add("ns-1", newTestPeerInfo("peer-1"), time.Hour) // 更新已有，应该成功
	if err != nil {
		t.Errorf("update should succeed: %v", err)
	}
}

func TestBadgerStore_Closed(t *testing.T) {
	store, cleanup := newTestBadgerStore(t)
	cleanup() // 立即关闭

	// 所有操作应该返回错误
	if err := store.Add("test", newTestPeerInfo("peer"), time.Hour); err != ErrStoreClosed {
		t.Errorf("Add should return ErrStoreClosed, got %v", err)
	}

	if _, _, err := store.Get("test", 10, nil); err != ErrStoreClosed {
		t.Errorf("Get should return ErrStoreClosed, got %v", err)
	}
}
