package dht

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dep2p/go-dep2p/internal/core/storage/engine/badger"
	"github.com/dep2p/go-dep2p/internal/core/storage/kv"
)

func newTestPersistentValueStore(t *testing.T) (*PersistentValueStore, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := engine.DefaultConfig(dbPath)
	eng, err := badger.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	store := kv.New(eng, []byte("d/v/"))
	vs, err := NewPersistentValueStore(store)
	if err != nil {
		eng.Close()
		t.Fatalf("failed to create persistent value store: %v", err)
	}

	cleanup := func() {
		eng.Close()
	}

	return vs, cleanup
}

func TestPersistentValueStore_PutAndGet(t *testing.T) {
	vs, cleanup := newTestPersistentValueStore(t)
	defer cleanup()

	key := "test-key"
	value := []byte("test-value")

	// 存储
	vs.Put(key, value, time.Hour)

	// 获取
	got, ok := vs.Get(key)
	if !ok {
		t.Fatal("expected value to exist")
	}

	if !bytes.Equal(got, value) {
		t.Errorf("expected %s, got %s", value, got)
	}
}

func TestPersistentValueStore_Delete(t *testing.T) {
	vs, cleanup := newTestPersistentValueStore(t)
	defer cleanup()

	key := "test-key"
	value := []byte("test-value")

	// 存储
	vs.Put(key, value, time.Hour)

	// 删除
	vs.Delete(key)

	// 验证已删除
	_, ok := vs.Get(key)
	if ok {
		t.Fatal("expected value to be deleted")
	}
}

func TestPersistentValueStore_TTLExpiry(t *testing.T) {
	vs, cleanup := newTestPersistentValueStore(t)
	defer cleanup()

	key := "test-key"
	value := []byte("test-value")

	// 存储短 TTL
	vs.Put(key, value, 10*time.Millisecond)

	// 等待过期
	time.Sleep(50 * time.Millisecond)

	// 验证已过期
	_, ok := vs.Get(key)
	if ok {
		t.Fatal("expected value to be expired")
	}
}

func TestPersistentValueStore_CleanupExpired(t *testing.T) {
	vs, cleanup := newTestPersistentValueStore(t)
	defer cleanup()

	// 存储多个值
	vs.Put("key1", []byte("value1"), 10*time.Millisecond)
	vs.Put("key2", []byte("value2"), time.Hour)

	// 等待部分过期
	time.Sleep(50 * time.Millisecond)

	// 清理
	count := vs.CleanupExpired()
	if count != 1 {
		t.Errorf("expected 1 expired, got %d", count)
	}

	// 验证只剩未过期的
	if vs.Size() != 1 {
		t.Errorf("expected size 1, got %d", vs.Size())
	}

	_, ok := vs.Get("key2")
	if !ok {
		t.Fatal("expected key2 to still exist")
	}
}

func TestPersistentValueStore_Size(t *testing.T) {
	vs, cleanup := newTestPersistentValueStore(t)
	defer cleanup()

	if vs.Size() != 0 {
		t.Errorf("expected size 0, got %d", vs.Size())
	}

	vs.Put("key1", []byte("value1"), time.Hour)
	vs.Put("key2", []byte("value2"), time.Hour)

	if vs.Size() != 2 {
		t.Errorf("expected size 2, got %d", vs.Size())
	}
}

func TestPersistentValueStore_Clear(t *testing.T) {
	vs, cleanup := newTestPersistentValueStore(t)
	defer cleanup()

	vs.Put("key1", []byte("value1"), time.Hour)
	vs.Put("key2", []byte("value2"), time.Hour)

	vs.Clear()

	if vs.Size() != 0 {
		t.Errorf("expected size 0 after clear, got %d", vs.Size())
	}
}

func TestPersistentValueStore_Persistence(t *testing.T) {
	// 创建引擎
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "persist.db")
	cfg := engine.DefaultConfig(dbPath)
	eng, err := badger.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer eng.Close()

	store := kv.New(eng, []byte("d/v/"))
	key := "test-key"
	value := []byte("test-value")

	// 创建第一个 ValueStore 并写入数据
	vs1, err := NewPersistentValueStore(store)
	if err != nil {
		t.Fatalf("failed to create persistent value store: %v", err)
	}

	vs1.Put(key, value, time.Hour)

	// 创建第二个 ValueStore，验证数据持久化
	vs2, err := NewPersistentValueStore(store)
	if err != nil {
		t.Fatalf("failed to create persistent value store: %v", err)
	}

	got, ok := vs2.Get(key)
	if !ok {
		t.Fatal("expected value to persist")
	}

	if !bytes.Equal(got, value) {
		t.Errorf("expected %s, got %s", value, got)
	}
}

func TestPersistentValueStore_BinaryValue(t *testing.T) {
	vs, cleanup := newTestPersistentValueStore(t)
	defer cleanup()

	key := "binary-key"
	// 包含各种字节的二进制值
	value := []byte{0x00, 0x01, 0xFF, 0xFE, 0x80, 0x7F}

	vs.Put(key, value, time.Hour)

	got, ok := vs.Get(key)
	if !ok {
		t.Fatal("expected value to exist")
	}

	if !bytes.Equal(got, value) {
		t.Errorf("binary value mismatch")
	}
}
