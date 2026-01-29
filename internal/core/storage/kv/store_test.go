package kv

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dep2p/go-dep2p/internal/core/storage/engine/badger"
)

// testStore 创建测试用 KVStore
// 使用 t.TempDir() 创建临时目录，确保测试与生产一致
func testStore(t *testing.T, prefix string) *Store {
	t.Helper()

	// 使用临时目录，测试结束后自动清理
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := engine.DefaultConfig(dbPath)
	eng, err := badger.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	t.Cleanup(func() {
		if err := eng.Close(); err != nil {
			t.Errorf("failed to close engine: %v", err)
		}
	})

	return New(eng, []byte(prefix))
}

// ============= 基础操作测试 =============

func TestStore_PutGet(t *testing.T) {
	s := testStore(t, "test/")

	key := []byte("key1")
	value := []byte("value1")

	// Put
	if err := s.Put(key, value); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Get
	got, err := s.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if !bytes.Equal(got, value) {
		t.Errorf("Get returned %q, want %q", got, value)
	}
}

func TestStore_Delete(t *testing.T) {
	s := testStore(t, "test/")

	key := []byte("delete-key")
	value := []byte("delete-value")

	// Put then Delete
	if err := s.Put(key, value); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	if err := s.Delete(key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	_, err := s.Get(key)
	if !engine.IsNotFound(err) {
		t.Errorf("Get after Delete returned %v, want ErrNotFound", err)
	}
}

func TestStore_Has(t *testing.T) {
	s := testStore(t, "test/")

	key := []byte("has-key")

	// Has before Put
	exists, err := s.Has(key)
	if err != nil {
		t.Fatalf("Has failed: %v", err)
	}
	if exists {
		t.Error("Has returned true for nonexistent key")
	}

	// Put
	if err := s.Put(key, []byte("value")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Has after Put
	exists, err = s.Has(key)
	if err != nil {
		t.Fatalf("Has failed: %v", err)
	}
	if !exists {
		t.Error("Has returned false for existing key")
	}
}

// ============= 前缀隔离测试 =============

func TestStore_PrefixIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "isolation.db")

	cfg := engine.DefaultConfig(dbPath)
	eng, err := badger.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer eng.Close()

	// 创建两个不同前缀的 Store
	store1 := New(eng, []byte("prefix1/"))
	store2 := New(eng, []byte("prefix2/"))

	key := []byte("shared-key")
	value1 := []byte("value-from-store1")
	value2 := []byte("value-from-store2")

	// 在 store1 中存储
	if err := store1.Put(key, value1); err != nil {
		t.Fatalf("store1.Put failed: %v", err)
	}

	// 在 store2 中存储同名键
	if err := store2.Put(key, value2); err != nil {
		t.Fatalf("store2.Put failed: %v", err)
	}

	// 验证数据隔离
	got1, err := store1.Get(key)
	if err != nil {
		t.Fatalf("store1.Get failed: %v", err)
	}
	if !bytes.Equal(got1, value1) {
		t.Errorf("store1.Get returned %q, want %q", got1, value1)
	}

	got2, err := store2.Get(key)
	if err != nil {
		t.Fatalf("store2.Get failed: %v", err)
	}
	if !bytes.Equal(got2, value2) {
		t.Errorf("store2.Get returned %q, want %q", got2, value2)
	}
}

// ============= JSON 便捷方法测试 =============

type testData struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

func TestStore_JSON(t *testing.T) {
	s := testStore(t, "json/")

	key := []byte("json-key")
	data := testData{Name: "test", Value: 42}

	// PutJSON
	if err := s.PutJSON(key, &data); err != nil {
		t.Fatalf("PutJSON failed: %v", err)
	}

	// GetJSON
	var got testData
	if err := s.GetJSON(key, &got); err != nil {
		t.Fatalf("GetJSON failed: %v", err)
	}

	if got.Name != data.Name || got.Value != data.Value {
		t.Errorf("GetJSON returned %+v, want %+v", got, data)
	}
}

// ============= Uint64 便捷方法测试 =============

func TestStore_Uint64(t *testing.T) {
	s := testStore(t, "uint64/")

	key := []byte("counter")

	// PutUint64
	if err := s.PutUint64(key, 100); err != nil {
		t.Fatalf("PutUint64 failed: %v", err)
	}

	// GetUint64
	got, err := s.GetUint64(key)
	if err != nil {
		t.Fatalf("GetUint64 failed: %v", err)
	}
	if got != 100 {
		t.Errorf("GetUint64 returned %d, want 100", got)
	}
}

func TestStore_IncrUint64(t *testing.T) {
	s := testStore(t, "incr/")

	key := []byte("counter")

	// IncrUint64 on nonexistent key
	val, err := s.IncrUint64(key, 5)
	if err != nil {
		t.Fatalf("IncrUint64 failed: %v", err)
	}
	if val != 5 {
		t.Errorf("IncrUint64 returned %d, want 5", val)
	}

	// IncrUint64 again
	val, err = s.IncrUint64(key, 10)
	if err != nil {
		t.Fatalf("IncrUint64 failed: %v", err)
	}
	if val != 15 {
		t.Errorf("IncrUint64 returned %d, want 15", val)
	}
}

func TestStore_DecrUint64(t *testing.T) {
	s := testStore(t, "decr/")

	key := []byte("counter")

	// Initialize
	if err := s.PutUint64(key, 100); err != nil {
		t.Fatalf("PutUint64 failed: %v", err)
	}

	// DecrUint64
	val, err := s.DecrUint64(key, 30)
	if err != nil {
		t.Fatalf("DecrUint64 failed: %v", err)
	}
	if val != 70 {
		t.Errorf("DecrUint64 returned %d, want 70", val)
	}

	// Decrement below zero
	val, err = s.DecrUint64(key, 100)
	if err != nil {
		t.Fatalf("DecrUint64 failed: %v", err)
	}
	if val != 0 {
		t.Errorf("DecrUint64 returned %d, want 0", val)
	}
}

// ============= String 便捷方法测试 =============

func TestStore_String(t *testing.T) {
	s := testStore(t, "string/")

	key := []byte("string-key")
	value := "hello, world"

	// PutString
	if err := s.PutString(key, value); err != nil {
		t.Fatalf("PutString failed: %v", err)
	}

	// GetString
	got, err := s.GetString(key)
	if err != nil {
		t.Fatalf("GetString failed: %v", err)
	}
	if got != value {
		t.Errorf("GetString returned %q, want %q", got, value)
	}
}

// ============= 前缀扫描测试 =============

func TestStore_PrefixScan(t *testing.T) {
	s := testStore(t, "scan/")

	// Put some keys with different sub-prefixes
	for i := 0; i < 5; i++ {
		key := []byte(fmt.Sprintf("a/%d", i))
		if err := s.Put(key, []byte(fmt.Sprintf("value-a-%d", i))); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	for i := 0; i < 3; i++ {
		key := []byte(fmt.Sprintf("b/%d", i))
		if err := s.Put(key, []byte(fmt.Sprintf("value-b-%d", i))); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	// Scan prefix "a/"
	var count int
	err := s.PrefixScan([]byte("a/"), func(key, value []byte) bool {
		count++
		return true
	})

	if err != nil {
		t.Fatalf("PrefixScan failed: %v", err)
	}

	if count != 5 {
		t.Errorf("PrefixScan found %d keys, want 5", count)
	}
}

func TestStore_Keys(t *testing.T) {
	s := testStore(t, "keys/")

	// Put some keys
	for i := 0; i < 10; i++ {
		key := []byte(fmt.Sprintf("key-%d", i))
		if err := s.Put(key, []byte("value")); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	// Get all keys
	keys, err := s.Keys(nil)
	if err != nil {
		t.Fatalf("Keys failed: %v", err)
	}

	if len(keys) != 10 {
		t.Errorf("Keys returned %d keys, want 10", len(keys))
	}
}

func TestStore_Count(t *testing.T) {
	s := testStore(t, "count/")

	// Put some keys
	for i := 0; i < 15; i++ {
		key := []byte(fmt.Sprintf("key-%d", i))
		if err := s.Put(key, []byte("value")); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	// Count
	count, err := s.Count(nil)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}

	if count != 15 {
		t.Errorf("Count returned %d, want 15", count)
	}
}

func TestStore_DeletePrefix(t *testing.T) {
	s := testStore(t, "delpfx/")

	// Put some keys with different prefixes
	for i := 0; i < 5; i++ {
		if err := s.Put([]byte(fmt.Sprintf("a/%d", i)), []byte("value")); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
		if err := s.Put([]byte(fmt.Sprintf("b/%d", i)), []byte("value")); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	// Delete prefix "a/"
	if err := s.DeletePrefix([]byte("a/")); err != nil {
		t.Fatalf("DeletePrefix failed: %v", err)
	}

	// Verify "a/" keys deleted
	countA, _ := s.Count([]byte("a/"))
	if countA != 0 {
		t.Errorf("Count(a/) = %d, want 0", countA)
	}

	// Verify "b/" keys remain
	countB, _ := s.Count([]byte("b/"))
	if countB != 5 {
		t.Errorf("Count(b/) = %d, want 5", countB)
	}
}

// ============= 批量操作测试 =============

func TestBatch_PutAndWrite(t *testing.T) {
	s := testStore(t, "batch/")

	batch := s.NewBatch()

	// Add operations
	for i := 0; i < 50; i++ {
		batch.Put([]byte(fmt.Sprintf("key-%d", i)), []byte(fmt.Sprintf("value-%d", i)))
	}

	if batch.Size() != 50 {
		t.Errorf("Batch.Size() = %d, want 50", batch.Size())
	}

	// Write
	if err := batch.Write(); err != nil {
		t.Fatalf("Batch.Write failed: %v", err)
	}

	// Verify
	for i := 0; i < 50; i++ {
		key := []byte(fmt.Sprintf("key-%d", i))
		expected := []byte(fmt.Sprintf("value-%d", i))

		got, err := s.Get(key)
		if err != nil {
			t.Errorf("Get(%s) failed: %v", key, err)
			continue
		}
		if !bytes.Equal(got, expected) {
			t.Errorf("Get(%s) = %q, want %q", key, got, expected)
		}
	}
}

func TestBatch_Delete(t *testing.T) {
	s := testStore(t, "batch-del/")

	// Put some keys
	for i := 0; i < 10; i++ {
		if err := s.Put([]byte(fmt.Sprintf("key-%d", i)), []byte("value")); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	// Batch delete
	batch := s.NewBatch()
	for i := 0; i < 10; i++ {
		batch.Delete([]byte(fmt.Sprintf("key-%d", i)))
	}

	if err := batch.Write(); err != nil {
		t.Fatalf("Batch.Write failed: %v", err)
	}

	// Verify deleted
	count, _ := s.Count(nil)
	if count != 0 {
		t.Errorf("Count after batch delete = %d, want 0", count)
	}
}

func TestBatch_PutJSON(t *testing.T) {
	s := testStore(t, "batch-json/")

	batch := s.NewBatch()

	data := testData{Name: "batch-test", Value: 123}
	if err := batch.PutJSON([]byte("json-key"), &data); err != nil {
		t.Fatalf("Batch.PutJSON failed: %v", err)
	}

	if err := batch.Write(); err != nil {
		t.Fatalf("Batch.Write failed: %v", err)
	}

	// Verify
	var got testData
	if err := s.GetJSON([]byte("json-key"), &got); err != nil {
		t.Fatalf("GetJSON failed: %v", err)
	}
	if got.Name != data.Name || got.Value != data.Value {
		t.Errorf("GetJSON returned %+v, want %+v", got, data)
	}
}

// ============= 事务测试 =============

func TestTransaction_ReadWrite(t *testing.T) {
	s := testStore(t, "txn/")

	key := []byte("txn-key")
	value := []byte("txn-value")

	// Start write transaction
	txn := s.NewTransaction(true)
	defer txn.Discard()

	// Set in transaction
	if err := txn.Set(key, value); err != nil {
		t.Fatalf("Transaction.Set failed: %v", err)
	}

	// Get in same transaction
	got, err := txn.Get(key)
	if err != nil {
		t.Fatalf("Transaction.Get failed: %v", err)
	}
	if !bytes.Equal(got, value) {
		t.Errorf("Transaction.Get returned %q, want %q", got, value)
	}

	// Commit
	if err := txn.Commit(); err != nil {
		t.Fatalf("Transaction.Commit failed: %v", err)
	}

	// Verify outside transaction
	got, err = s.Get(key)
	if err != nil {
		t.Fatalf("Get after commit failed: %v", err)
	}
	if !bytes.Equal(got, value) {
		t.Errorf("Get after commit returned %q, want %q", got, value)
	}
}

func TestTransaction_Discard(t *testing.T) {
	s := testStore(t, "txn-discard/")

	key := []byte("discard-key")
	value := []byte("discard-value")

	// Start write transaction
	txn := s.NewTransaction(true)

	// Set in transaction
	if err := txn.Set(key, value); err != nil {
		t.Fatalf("Transaction.Set failed: %v", err)
	}

	// Discard
	txn.Discard()

	// Verify key does not exist
	_, err := s.Get(key)
	if !engine.IsNotFound(err) {
		t.Errorf("Get after discard returned %v, want ErrNotFound", err)
	}
}

func TestTransaction_JSON(t *testing.T) {
	s := testStore(t, "txn-json/")

	key := []byte("json-key")
	data := testData{Name: "txn-json", Value: 456}

	txn := s.NewTransaction(true)
	defer txn.Discard()

	// SetJSON
	if err := txn.SetJSON(key, &data); err != nil {
		t.Fatalf("Transaction.SetJSON failed: %v", err)
	}

	if err := txn.Commit(); err != nil {
		t.Fatalf("Transaction.Commit failed: %v", err)
	}

	// GetJSON outside transaction
	var got testData
	if err := s.GetJSON(key, &got); err != nil {
		t.Fatalf("GetJSON failed: %v", err)
	}
	if got.Name != data.Name || got.Value != data.Value {
		t.Errorf("GetJSON returned %+v, want %+v", got, data)
	}
}

// ============= SubStore 测试 =============

func TestStore_SubStore(t *testing.T) {
	s := testStore(t, "parent/")

	// Create sub-store
	sub := s.SubStore([]byte("child/"))

	// Verify prefix
	expectedPrefix := []byte("parent/child/")
	if !bytes.Equal(sub.Prefix(), expectedPrefix) {
		t.Errorf("SubStore prefix = %q, want %q", sub.Prefix(), expectedPrefix)
	}

	// Put in sub-store
	if err := sub.Put([]byte("key"), []byte("value")); err != nil {
		t.Fatalf("SubStore.Put failed: %v", err)
	}

	// Get from parent with full path
	got, err := s.Get([]byte("child/key"))
	if err != nil {
		t.Fatalf("Parent.Get failed: %v", err)
	}
	if !bytes.Equal(got, []byte("value")) {
		t.Errorf("Parent.Get returned %q, want %q", got, "value")
	}
}

// ============= 并发测试 =============

func TestStore_ConcurrentAccess(t *testing.T) {
	s := testStore(t, "concurrent/")

	var wg sync.WaitGroup
	errCh := make(chan error, 200)

	// Concurrent writes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			key := []byte(fmt.Sprintf("key-%d", idx))
			value := []byte(fmt.Sprintf("value-%d", idx))

			if err := s.Put(key, value); err != nil {
				errCh <- fmt.Errorf("Put(%s) failed: %v", key, err)
			}
		}(i)
	}

	// Concurrent reads (of pre-existing keys)
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			key := []byte(fmt.Sprintf("key-%d", idx))
			// May not exist yet, ignore NotFound
			_, err := s.Get(key)
			if err != nil && !engine.IsNotFound(err) {
				errCh <- fmt.Errorf("Get(%s) failed: %v", key, err)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}

	// Verify all keys exist
	count, err := s.Count(nil)
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if count != 100 {
		t.Errorf("Count = %d, want 100", count)
	}
}
