package addressbook

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dep2p/go-dep2p/internal/core/storage/engine/badger"
	"github.com/dep2p/go-dep2p/internal/core/storage/kv"
	realmif "github.com/dep2p/go-dep2p/internal/realm/interfaces"
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

	kvStore := kv.New(eng, []byte("a/"))
	store, err := NewBadgerStore(kvStore)
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

func newTestEntry(nodeID string) realmif.MemberEntry {
	addr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	return realmif.MemberEntry{
		NodeID:       types.NodeID(nodeID),
		DirectAddrs:  []types.Multiaddr{addr},
		NATType:      types.NATTypeNone,
		Capabilities: []string{"relay"},
		Online:       true,
		LastSeen:     time.Now(),
		LastUpdate:   time.Now(),
	}
}

func TestBadgerStore_PutAndGet(t *testing.T) {
	store, cleanup := newTestBadgerStore(t)
	defer cleanup()

	ctx := context.Background()
	entry := newTestEntry("node-1")

	// 存储
	if err := store.Put(ctx, entry); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// 获取
	got, found, err := store.Get(ctx, entry.NodeID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !found {
		t.Fatal("expected entry to be found")
	}

	if got.NodeID != entry.NodeID {
		t.Errorf("NodeID mismatch: got %s, want %s", got.NodeID, entry.NodeID)
	}

	if len(got.DirectAddrs) != 1 {
		t.Errorf("expected 1 address, got %d", len(got.DirectAddrs))
	}
}

func TestBadgerStore_Delete(t *testing.T) {
	store, cleanup := newTestBadgerStore(t)
	defer cleanup()

	ctx := context.Background()
	entry := newTestEntry("node-1")

	// 存储
	store.Put(ctx, entry)

	// 删除
	if err := store.Delete(ctx, entry.NodeID); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// 验证已删除
	_, found, _ := store.Get(ctx, entry.NodeID)
	if found {
		t.Fatal("expected entry to be deleted")
	}
}

func TestBadgerStore_List(t *testing.T) {
	store, cleanup := newTestBadgerStore(t)
	defer cleanup()

	ctx := context.Background()

	// 存储多个条目
	store.Put(ctx, newTestEntry("node-1"))
	store.Put(ctx, newTestEntry("node-2"))
	store.Put(ctx, newTestEntry("node-3"))

	// 列出
	entries, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
}

func TestBadgerStore_SetTTL(t *testing.T) {
	store, cleanup := newTestBadgerStore(t)
	defer cleanup()

	ctx := context.Background()
	entry := newTestEntry("node-1")

	// 存储
	store.Put(ctx, entry)

	// 设置短 TTL
	if err := store.SetTTL(ctx, entry.NodeID, 10*time.Millisecond); err != nil {
		t.Fatalf("SetTTL failed: %v", err)
	}

	// 等待过期
	time.Sleep(50 * time.Millisecond)

	// 验证已过期
	_, found, _ := store.Get(ctx, entry.NodeID)
	if found {
		t.Fatal("expected entry to be expired")
	}
}

func TestBadgerStore_CleanExpired(t *testing.T) {
	store, cleanup := newTestBadgerStore(t)
	defer cleanup()

	ctx := context.Background()

	// 设置短默认 TTL
	store.SetDefaultTTL(10 * time.Millisecond)

	// 存储
	store.Put(ctx, newTestEntry("node-1"))

	// 重置 TTL 为长时间
	store.SetDefaultTTL(24 * time.Hour)
	store.Put(ctx, newTestEntry("node-2"))

	// 等待部分过期
	time.Sleep(50 * time.Millisecond)

	// 清理
	if err := store.CleanExpired(ctx); err != nil {
		t.Fatalf("CleanExpired failed: %v", err)
	}

	// 验证
	entries, _ := store.List(ctx)
	if len(entries) != 1 {
		t.Errorf("expected 1 entry after cleanup, got %d", len(entries))
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

	kvStore := kv.New(eng, []byte("a/"))
	ctx := context.Background()
	entry := newTestEntry("node-1")

	// 创建第一个存储并写入数据
	store1, err := NewBadgerStore(kvStore)
	if err != nil {
		t.Fatalf("failed to create badger store: %v", err)
	}

	if err := store1.Put(ctx, entry); err != nil {
		t.Fatalf("Put failed: %v", err)
	}
	store1.Close()

	// 创建第二个存储，验证数据持久化
	store2, err := NewBadgerStore(kvStore)
	if err != nil {
		t.Fatalf("failed to create badger store: %v", err)
	}
	defer store2.Close()

	got, found, err := store2.Get(ctx, entry.NodeID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !found {
		t.Fatal("expected entry to persist")
	}

	if got.NodeID != entry.NodeID {
		t.Errorf("NodeID mismatch: got %s, want %s", got.NodeID, entry.NodeID)
	}
}

func TestBadgerStore_ContextCancellation(t *testing.T) {
	store, cleanup := newTestBadgerStore(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	entry := newTestEntry("node-1")

	// 所有操作应该返回上下文错误
	if err := store.Put(ctx, entry); err != context.Canceled {
		t.Errorf("Put should return context.Canceled, got %v", err)
	}

	if _, _, err := store.Get(ctx, entry.NodeID); err != context.Canceled {
		t.Errorf("Get should return context.Canceled, got %v", err)
	}

	if err := store.Delete(ctx, entry.NodeID); err != context.Canceled {
		t.Errorf("Delete should return context.Canceled, got %v", err)
	}

	if _, err := store.List(ctx); err != context.Canceled {
		t.Errorf("List should return context.Canceled, got %v", err)
	}
}

func TestBadgerStore_Closed(t *testing.T) {
	store, cleanup := newTestBadgerStore(t)
	cleanup() // 立即关闭

	ctx := context.Background()
	entry := newTestEntry("node-1")

	// 所有操作应该返回 ErrStoreClosed
	if err := store.Put(ctx, entry); err != ErrStoreClosed {
		t.Errorf("Put should return ErrStoreClosed, got %v", err)
	}

	if _, _, err := store.Get(ctx, entry.NodeID); err != ErrStoreClosed {
		t.Errorf("Get should return ErrStoreClosed, got %v", err)
	}

	if err := store.Delete(ctx, entry.NodeID); err != ErrStoreClosed {
		t.Errorf("Delete should return ErrStoreClosed, got %v", err)
	}

	if _, err := store.List(ctx); err != ErrStoreClosed {
		t.Errorf("List should return ErrStoreClosed, got %v", err)
	}
}

func TestBadgerStore_InvalidNodeID(t *testing.T) {
	store, cleanup := newTestBadgerStore(t)
	defer cleanup()

	ctx := context.Background()
	emptyNodeID := types.NodeID("")

	// 空 NodeID 应该返回错误
	entry := realmif.MemberEntry{NodeID: emptyNodeID}
	if err := store.Put(ctx, entry); err != ErrInvalidNodeID {
		t.Errorf("Put with empty NodeID should return ErrInvalidNodeID, got %v", err)
	}

	if _, _, err := store.Get(ctx, emptyNodeID); err != ErrInvalidNodeID {
		t.Errorf("Get with empty NodeID should return ErrInvalidNodeID, got %v", err)
	}

	if err := store.Delete(ctx, emptyNodeID); err != ErrInvalidNodeID {
		t.Errorf("Delete with empty NodeID should return ErrInvalidNodeID, got %v", err)
	}
}

func TestNewStore(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "store.db")
	cfg := engine.DefaultConfig(dbPath)
	eng, err := badger.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer eng.Close()

	config := StoreConfig{
		Engine:     eng,
		DefaultTTL: time.Hour,
	}

	store, err := NewStore(config)
	if err != nil {
		t.Fatalf("NewStore failed: %v", err)
	}
	defer store.Close()

	// 验证是 BadgerStore
	_, ok := store.(*BadgerStore)
	if !ok {
		t.Error("expected BadgerStore")
	}
}

func TestNewStore_WithoutEngine(t *testing.T) {
	config := StoreConfig{
		DefaultTTL: time.Hour,
		Engine:     nil,
	}

	_, err := NewStore(config)
	if err != ErrEngineRequired {
		t.Errorf("expected ErrEngineRequired, got %v", err)
	}
}
