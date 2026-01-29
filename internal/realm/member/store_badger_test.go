package member

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dep2p/go-dep2p/internal/core/storage/engine/badger"
	"github.com/dep2p/go-dep2p/internal/core/storage/kv"
	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
)

func newTestMemberBadgerStore(t *testing.T) (*BadgerStore, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := engine.DefaultConfig(dbPath)
	eng, err := badger.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	kvStore := kv.New(eng, []byte("m/"))
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

func newTestMemberInfo(peerID string) *interfaces.MemberInfo {
	return &interfaces.MemberInfo{
		PeerID:   peerID,
		RealmID:  "test-realm",
		Role:     interfaces.RoleMember,
		Online:   true,
		JoinedAt: time.Now(),
		LastSeen: time.Now(),
	}
}

func TestMemberBadgerStore_SaveAndLoad(t *testing.T) {
	store, cleanup := newTestMemberBadgerStore(t)
	defer cleanup()

	member := newTestMemberInfo("peer-1")

	// 保存
	if err := store.Save(member); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// 加载
	loaded, err := store.Load("peer-1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.PeerID != member.PeerID {
		t.Errorf("PeerID mismatch: got %s, want %s", loaded.PeerID, member.PeerID)
	}

	if loaded.RealmID != member.RealmID {
		t.Errorf("RealmID mismatch: got %s, want %s", loaded.RealmID, member.RealmID)
	}
}

func TestMemberBadgerStore_Delete(t *testing.T) {
	store, cleanup := newTestMemberBadgerStore(t)
	defer cleanup()

	member := newTestMemberInfo("peer-1")

	// 保存
	store.Save(member)

	// 删除
	if err := store.Delete("peer-1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// 验证已删除
	_, err := store.Load("peer-1")
	if err != ErrMemberNotFound {
		t.Errorf("expected ErrMemberNotFound, got %v", err)
	}
}

func TestMemberBadgerStore_LoadAll(t *testing.T) {
	store, cleanup := newTestMemberBadgerStore(t)
	defer cleanup()

	// 保存多个成员
	store.Save(newTestMemberInfo("peer-1"))
	store.Save(newTestMemberInfo("peer-2"))
	store.Save(newTestMemberInfo("peer-3"))

	// 加载全部
	members, err := store.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	if len(members) != 3 {
		t.Errorf("expected 3 members, got %d", len(members))
	}
}

func TestMemberBadgerStore_Compact(t *testing.T) {
	store, cleanup := newTestMemberBadgerStore(t)
	defer cleanup()

	// Compact 应该是空操作（BadgerDB 自动 GC）
	if err := store.Compact(); err != nil {
		t.Errorf("Compact failed: %v", err)
	}
}

func TestMemberBadgerStore_Persistence(t *testing.T) {
	// 创建引擎
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "persist.db")
	cfg := engine.DefaultConfig(dbPath)
	eng, err := badger.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer eng.Close()

	kvStore := kv.New(eng, []byte("m/"))
	member := newTestMemberInfo("peer-1")

	// 创建第一个存储并写入数据
	store1, err := NewBadgerStore(kvStore)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	if err := store1.Save(member); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	store1.Close()

	// 创建第二个存储，验证数据持久化
	store2, err := NewBadgerStore(kvStore)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store2.Close()

	loaded, err := store2.Load("peer-1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.PeerID != member.PeerID {
		t.Error("PeerID mismatch after persistence")
	}
}

func TestMemberBadgerStore_Update(t *testing.T) {
	store, cleanup := newTestMemberBadgerStore(t)
	defer cleanup()

	member := newTestMemberInfo("peer-1")
	member.Online = true

	// 保存
	store.Save(member)

	// 更新
	member.Online = false
	member.LastSeen = time.Now()
	store.Save(member)

	// 验证更新
	loaded, _ := store.Load("peer-1")
	if loaded.Online != false {
		t.Error("Online status should be false after update")
	}
}

func TestMemberBadgerStore_Closed(t *testing.T) {
	store, cleanup := newTestMemberBadgerStore(t)
	cleanup() // 立即关闭

	member := newTestMemberInfo("peer-1")

	// 所有操作应该返回 ErrStoreClosed
	if err := store.Save(member); err != ErrStoreClosed {
		t.Errorf("Save should return ErrStoreClosed, got %v", err)
	}

	if _, err := store.Load("peer-1"); err != ErrStoreClosed {
		t.Errorf("Load should return ErrStoreClosed, got %v", err)
	}

	if err := store.Delete("peer-1"); err != ErrStoreClosed {
		t.Errorf("Delete should return ErrStoreClosed, got %v", err)
	}

	if _, err := store.LoadAll(); err != ErrStoreClosed {
		t.Errorf("LoadAll should return ErrStoreClosed, got %v", err)
	}

	if err := store.Compact(); err != ErrStoreClosed {
		t.Errorf("Compact should return ErrStoreClosed, got %v", err)
	}
}

func TestMemberBadgerStore_InvalidMember(t *testing.T) {
	store, cleanup := newTestMemberBadgerStore(t)
	defer cleanup()

	// nil 成员应该返回错误
	if err := store.Save(nil); err != ErrInvalidMember {
		t.Errorf("Save(nil) should return ErrInvalidMember, got %v", err)
	}
}

func TestMemberBadgerStore_Len(t *testing.T) {
	store, cleanup := newTestMemberBadgerStore(t)
	defer cleanup()

	// 初始为空
	if store.Len() != 0 {
		t.Error("expected 0 members initially")
	}

	// 添加成员
	store.Save(newTestMemberInfo("peer-1"))
	store.Save(newTestMemberInfo("peer-2"))

	if store.Len() != 2 {
		t.Errorf("expected 2 members, got %d", store.Len())
	}

	// 删除一个
	store.Delete("peer-1")

	if store.Len() != 1 {
		t.Errorf("expected 1 member after delete, got %d", store.Len())
	}
}
