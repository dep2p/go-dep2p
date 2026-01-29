package addrbook

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dep2p/go-dep2p/internal/core/storage/engine/badger"
	"github.com/dep2p/go-dep2p/internal/core/storage/kv"
	"github.com/dep2p/go-dep2p/pkg/types"
)

func newTestPersistentAddrBook(t *testing.T) (*PersistentAddrBook, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := engine.DefaultConfig(dbPath)
	eng, err := badger.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	store := kv.New(eng, []byte("p/a/"))
	ab, err := NewPersistent(store)
	if err != nil {
		eng.Close()
		t.Fatalf("failed to create persistent addrbook: %v", err)
	}

	cleanup := func() {
		ab.Close()
		eng.Close()
	}

	return ab, cleanup
}

func TestPersistentAddrBook_AddAndGet(t *testing.T) {
	ab, cleanup := newTestPersistentAddrBook(t)
	defer cleanup()

	peerID := types.PeerID("test-peer-1")
	addr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")

	// 添加地址
	ab.AddAddr(peerID, addr, time.Hour)

	// 获取地址
	addrs := ab.Addrs(peerID)
	if len(addrs) != 1 {
		t.Fatalf("expected 1 address, got %d", len(addrs))
	}

	if addrs[0].String() != addr.String() {
		t.Errorf("expected %s, got %s", addr.String(), addrs[0].String())
	}
}

func TestPersistentAddrBook_SetAddrs(t *testing.T) {
	ab, cleanup := newTestPersistentAddrBook(t)
	defer cleanup()

	peerID := types.PeerID("test-peer-1")
	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	addr2, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4002")

	// 添加地址
	ab.AddAddrs(peerID, []types.Multiaddr{addr1}, time.Hour)

	// 设置新地址（覆盖）
	ab.SetAddrs(peerID, []types.Multiaddr{addr2}, time.Hour)

	// 获取地址
	addrs := ab.Addrs(peerID)
	if len(addrs) != 1 {
		t.Fatalf("expected 1 address, got %d", len(addrs))
	}

	if addrs[0].String() != addr2.String() {
		t.Errorf("expected %s, got %s", addr2.String(), addrs[0].String())
	}
}

func TestPersistentAddrBook_ClearAddrs(t *testing.T) {
	ab, cleanup := newTestPersistentAddrBook(t)
	defer cleanup()

	peerID := types.PeerID("test-peer-1")
	addr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")

	// 添加地址
	ab.AddAddr(peerID, addr, time.Hour)

	// 清除地址
	ab.ClearAddrs(peerID)

	// 验证地址已清除
	addrs := ab.Addrs(peerID)
	if len(addrs) != 0 {
		t.Fatalf("expected 0 addresses, got %d", len(addrs))
	}
}

func TestPersistentAddrBook_TTLExpiry(t *testing.T) {
	ab, cleanup := newTestPersistentAddrBook(t)
	defer cleanup()

	peerID := types.PeerID("test-peer-1")
	addr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")

	// 添加短 TTL 地址
	ab.AddAddr(peerID, addr, 10*time.Millisecond)

	// 等待过期
	time.Sleep(50 * time.Millisecond)

	// 验证地址已过期
	addrs := ab.Addrs(peerID)
	if len(addrs) != 0 {
		t.Fatalf("expected 0 addresses (expired), got %d", len(addrs))
	}
}

func TestPersistentAddrBook_AddrsWithSource(t *testing.T) {
	ab, cleanup := newTestPersistentAddrBook(t)
	defer cleanup()

	peerID := types.PeerID("test-peer-1")
	addr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")

	// 添加带来源的地址
	ab.AddAddrsWithSource(peerID, []AddressWithSource{
		{Addr: addr, Source: SourceDHT, TTL: time.Hour},
	})

	// 获取地址来源
	source := ab.GetAddrSource(peerID, addr)
	if source != SourceDHT {
		t.Errorf("expected source %v, got %v", SourceDHT, source)
	}

	// 获取带来源的地址
	addrsWithSource := ab.AddrsWithSource(peerID)
	if len(addrsWithSource) != 1 {
		t.Fatalf("expected 1 address, got %d", len(addrsWithSource))
	}

	if addrsWithSource[0].Source != SourceDHT {
		t.Errorf("expected source %v, got %v", SourceDHT, addrsWithSource[0].Source)
	}
}

func TestPersistentAddrBook_PeersWithAddrs(t *testing.T) {
	ab, cleanup := newTestPersistentAddrBook(t)
	defer cleanup()

	peer1 := types.PeerID("test-peer-1")
	peer2 := types.PeerID("test-peer-2")
	addr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")

	// 添加地址
	ab.AddAddr(peer1, addr, time.Hour)
	ab.AddAddr(peer2, addr, time.Hour)

	// 获取有地址的节点
	peers := ab.PeersWithAddrs()
	if len(peers) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(peers))
	}
}

func TestPersistentAddrBook_Persistence(t *testing.T) {
	// 创建引擎
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "persist.db")
	cfg := engine.DefaultConfig(dbPath)
	eng, err := badger.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer eng.Close()

	store := kv.New(eng, []byte("p/a/"))
	peerID := types.PeerID("test-peer-1")
	addr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")

	// 创建第一个 AddrBook 并写入数据
	ab1, err := NewPersistent(store)
	if err != nil {
		t.Fatalf("failed to create persistent addrbook: %v", err)
	}

	ab1.AddAddr(peerID, addr, time.Hour)
	ab1.Close()

	// 创建第二个 AddrBook，验证数据持久化
	ab2, err := NewPersistent(store)
	if err != nil {
		t.Fatalf("failed to create persistent addrbook: %v", err)
	}
	defer ab2.Close()

	addrs := ab2.Addrs(peerID)
	if len(addrs) != 1 {
		t.Fatalf("expected 1 address after reload, got %d", len(addrs))
	}

	if addrs[0].String() != addr.String() {
		t.Errorf("expected %s, got %s", addr.String(), addrs[0].String())
	}
}

func TestPersistentAddrBook_ResetTemporaryAddrs(t *testing.T) {
	ab, cleanup := newTestPersistentAddrBook(t)
	defer cleanup()

	peerID := types.PeerID("test-peer-1")
	shortAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	longAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4002")

	// 添加短期和长期地址
	ab.AddAddr(peerID, shortAddr, 5*time.Minute)  // 临时地址
	ab.AddAddr(peerID, longAddr, 24*time.Hour)    // 永久地址

	// 重置临时地址
	removed := ab.ResetTemporaryAddrs()

	if removed != 1 {
		t.Errorf("expected 1 address removed, got %d", removed)
	}

	// 验证只剩长期地址
	addrs := ab.Addrs(peerID)
	if len(addrs) != 1 {
		t.Fatalf("expected 1 address, got %d", len(addrs))
	}

	if addrs[0].String() != longAddr.String() {
		t.Errorf("expected long-term address %s, got %s", longAddr.String(), addrs[0].String())
	}
}
