package dht

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dep2p/go-dep2p/internal/core/storage/engine/badger"
	"github.com/dep2p/go-dep2p/internal/core/storage/kv"
	"github.com/dep2p/go-dep2p/pkg/types"
)

func newTestPersistentProviderStore(t *testing.T) (*PersistentProviderStore, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := engine.DefaultConfig(dbPath)
	eng, err := badger.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	store := kv.New(eng, []byte("d/p/"))
	ps, err := NewPersistentProviderStore(store)
	if err != nil {
		eng.Close()
		t.Fatalf("failed to create persistent provider store: %v", err)
	}

	cleanup := func() {
		eng.Close()
	}

	return ps, cleanup
}

func TestPersistentProviderStore_AddAndGet(t *testing.T) {
	ps, cleanup := newTestPersistentProviderStore(t)
	defer cleanup()

	key := "test-key"
	peerID := types.NodeID("peer-1")
	addrs := []string{"/ip4/127.0.0.1/tcp/4001"}

	// 添加
	ps.AddProvider(key, peerID, addrs, time.Hour)

	// 获取
	providers := ps.GetProviders(key)
	if len(providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(providers))
	}

	if providers[0].PeerID != types.PeerID(peerID) {
		t.Errorf("expected peer %s, got %s", peerID, providers[0].PeerID)
	}

	if len(providers[0].Addrs) != 1 || providers[0].Addrs[0] != addrs[0] {
		t.Errorf("address mismatch")
	}
}

func TestPersistentProviderStore_MultipleProviders(t *testing.T) {
	ps, cleanup := newTestPersistentProviderStore(t)
	defer cleanup()

	key := "test-key"
	peer1 := types.NodeID("peer-1")
	peer2 := types.NodeID("peer-2")
	addrs := []string{"/ip4/127.0.0.1/tcp/4001"}

	// 添加多个 provider
	ps.AddProvider(key, peer1, addrs, time.Hour)
	ps.AddProvider(key, peer2, addrs, time.Hour)

	// 获取
	providers := ps.GetProviders(key)
	if len(providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(providers))
	}
}

func TestPersistentProviderStore_UpdateProvider(t *testing.T) {
	ps, cleanup := newTestPersistentProviderStore(t)
	defer cleanup()

	key := "test-key"
	peerID := types.NodeID("peer-1")
	oldAddrs := []string{"/ip4/127.0.0.1/tcp/4001"}
	newAddrs := []string{"/ip4/127.0.0.1/tcp/4002"}

	// 添加
	ps.AddProvider(key, peerID, oldAddrs, time.Hour)

	// 更新
	ps.AddProvider(key, peerID, newAddrs, time.Hour)

	// 验证
	providers := ps.GetProviders(key)
	if len(providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(providers))
	}

	if providers[0].Addrs[0] != newAddrs[0] {
		t.Errorf("expected address %s, got %s", newAddrs[0], providers[0].Addrs[0])
	}
}

func TestPersistentProviderStore_RemoveProvider(t *testing.T) {
	ps, cleanup := newTestPersistentProviderStore(t)
	defer cleanup()

	key := "test-key"
	peer1 := types.NodeID("peer-1")
	peer2 := types.NodeID("peer-2")
	addrs := []string{"/ip4/127.0.0.1/tcp/4001"}

	// 添加
	ps.AddProvider(key, peer1, addrs, time.Hour)
	ps.AddProvider(key, peer2, addrs, time.Hour)

	// 移除
	ps.RemoveProvider(key, peer1)

	// 验证
	providers := ps.GetProviders(key)
	if len(providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(providers))
	}

	if providers[0].PeerID != types.PeerID(peer2) {
		t.Errorf("expected peer %s, got %s", peer2, providers[0].PeerID)
	}
}

func TestPersistentProviderStore_TTLExpiry(t *testing.T) {
	ps, cleanup := newTestPersistentProviderStore(t)
	defer cleanup()

	key := "test-key"
	peerID := types.NodeID("peer-1")
	addrs := []string{"/ip4/127.0.0.1/tcp/4001"}

	// 添加短 TTL
	ps.AddProvider(key, peerID, addrs, 10*time.Millisecond)

	// 等待过期
	time.Sleep(50 * time.Millisecond)

	// 验证已过期
	providers := ps.GetProviders(key)
	if len(providers) != 0 {
		t.Fatalf("expected 0 providers (expired), got %d", len(providers))
	}
}

func TestPersistentProviderStore_CleanupExpired(t *testing.T) {
	ps, cleanup := newTestPersistentProviderStore(t)
	defer cleanup()

	key := "test-key"
	peer1 := types.NodeID("peer-1")
	peer2 := types.NodeID("peer-2")
	addrs := []string{"/ip4/127.0.0.1/tcp/4001"}

	// 添加
	ps.AddProvider(key, peer1, addrs, 10*time.Millisecond) // 会过期
	ps.AddProvider(key, peer2, addrs, time.Hour)           // 不会过期

	// 等待部分过期
	time.Sleep(50 * time.Millisecond)

	// 清理
	count := ps.CleanupExpired()
	if count != 1 {
		t.Errorf("expected 1 expired, got %d", count)
	}

	// 验证
	providers := ps.GetProviders(key)
	if len(providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(providers))
	}
}

func TestPersistentProviderStore_Size(t *testing.T) {
	ps, cleanup := newTestPersistentProviderStore(t)
	defer cleanup()

	if ps.Size() != 0 {
		t.Errorf("expected size 0, got %d", ps.Size())
	}

	ps.AddProvider("key1", types.NodeID("peer-1"), []string{}, time.Hour)
	ps.AddProvider("key1", types.NodeID("peer-2"), []string{}, time.Hour)
	ps.AddProvider("key2", types.NodeID("peer-1"), []string{}, time.Hour)

	if ps.Size() != 3 {
		t.Errorf("expected size 3, got %d", ps.Size())
	}
}

func TestPersistentProviderStore_Clear(t *testing.T) {
	ps, cleanup := newTestPersistentProviderStore(t)
	defer cleanup()

	ps.AddProvider("key1", types.NodeID("peer-1"), []string{}, time.Hour)
	ps.AddProvider("key2", types.NodeID("peer-2"), []string{}, time.Hour)

	ps.Clear()

	if ps.Size() != 0 {
		t.Errorf("expected size 0 after clear, got %d", ps.Size())
	}
}

func TestPersistentProviderStore_Persistence(t *testing.T) {
	// 创建引擎
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "persist.db")
	cfg := engine.DefaultConfig(dbPath)
	eng, err := badger.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer eng.Close()

	store := kv.New(eng, []byte("d/p/"))
	key := "test-key"
	peerID := types.NodeID("peer-1")
	addrs := []string{"/ip4/127.0.0.1/tcp/4001"}

	// 创建第一个 ProviderStore 并写入数据
	ps1, err := NewPersistentProviderStore(store)
	if err != nil {
		t.Fatalf("failed to create persistent provider store: %v", err)
	}

	ps1.AddProvider(key, peerID, addrs, time.Hour)

	// 创建第二个 ProviderStore，验证数据持久化
	ps2, err := NewPersistentProviderStore(store)
	if err != nil {
		t.Fatalf("failed to create persistent provider store: %v", err)
	}

	providers := ps2.GetProviders(key)
	if len(providers) != 1 {
		t.Fatalf("expected 1 provider after reload, got %d", len(providers))
	}

	if providers[0].PeerID != types.PeerID(peerID) {
		t.Errorf("expected peer %s, got %s", peerID, providers[0].PeerID)
	}
}

func TestPersistentProviderStore_UpdateLocalAddrs(t *testing.T) {
	ps, cleanup := newTestPersistentProviderStore(t)
	defer cleanup()

	localID := types.NodeID("local-peer")
	otherID := types.NodeID("other-peer")
	oldAddrs := []string{"/ip4/127.0.0.1/tcp/4001"}
	newAddrs := []string{"/ip4/192.168.1.1/tcp/4001"}

	// 添加
	ps.AddProvider("key1", localID, oldAddrs, time.Hour)
	ps.AddProvider("key1", otherID, oldAddrs, time.Hour)
	ps.AddProvider("key2", localID, oldAddrs, time.Hour)

	// 更新本地地址
	ps.UpdateLocalAddrs(localID, newAddrs, time.Hour)

	// 验证本地地址已更新
	providers1 := ps.GetProviders("key1")
	for _, p := range providers1 {
		if p.PeerID == types.PeerID(localID) {
			if p.Addrs[0] != newAddrs[0] {
				t.Errorf("expected local address %s, got %s", newAddrs[0], p.Addrs[0])
			}
		} else {
			// 其他节点地址不变
			if p.Addrs[0] != oldAddrs[0] {
				t.Errorf("expected other address %s, got %s", oldAddrs[0], p.Addrs[0])
			}
		}
	}
}
