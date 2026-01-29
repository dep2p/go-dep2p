package badger

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sync"
	"testing"

	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
)

// testEngine 创建测试用引擎
// 使用 t.TempDir() 创建临时目录，确保测试与生产一致
func testEngine(t *testing.T) *Engine {
	t.Helper()

	// 使用临时目录，测试结束后自动清理
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := engine.DefaultConfig(dbPath)
	e, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	t.Cleanup(func() {
		if err := e.Close(); err != nil {
			t.Errorf("failed to close engine: %v", err)
		}
	})

	return e
}

// ============= 基础 CRUD 测试 =============

func TestEngine_PutGet(t *testing.T) {
	e := testEngine(t)

	key := []byte("test-key")
	value := []byte("test-value")

	// Put
	if err := e.Put(key, value); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Get
	got, err := e.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if !bytes.Equal(got, value) {
		t.Errorf("Get returned %q, want %q", got, value)
	}
}

func TestEngine_GetNotFound(t *testing.T) {
	e := testEngine(t)

	_, err := e.Get([]byte("nonexistent"))
	if err != engine.ErrNotFound {
		t.Errorf("Get returned error %v, want ErrNotFound", err)
	}
}

func TestEngine_Delete(t *testing.T) {
	e := testEngine(t)

	key := []byte("delete-key")
	value := []byte("delete-value")

	// Put then Delete
	if err := e.Put(key, value); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	if err := e.Delete(key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	_, err := e.Get(key)
	if err != engine.ErrNotFound {
		t.Errorf("Get after Delete returned error %v, want ErrNotFound", err)
	}
}

func TestEngine_DeleteNonexistent(t *testing.T) {
	e := testEngine(t)

	// Delete nonexistent key should not error
	if err := e.Delete([]byte("nonexistent")); err != nil {
		t.Errorf("Delete nonexistent key returned error: %v", err)
	}
}

func TestEngine_Has(t *testing.T) {
	e := testEngine(t)

	key := []byte("has-key")

	// Has before Put
	exists, err := e.Has(key)
	if err != nil {
		t.Fatalf("Has failed: %v", err)
	}
	if exists {
		t.Error("Has returned true for nonexistent key")
	}

	// Put
	if err := e.Put(key, []byte("value")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Has after Put
	exists, err = e.Has(key)
	if err != nil {
		t.Fatalf("Has failed: %v", err)
	}
	if !exists {
		t.Error("Has returned false for existing key")
	}
}

func TestEngine_EmptyKey(t *testing.T) {
	e := testEngine(t)

	// Put with empty key
	if err := e.Put([]byte{}, []byte("value")); err != engine.ErrEmptyKey {
		t.Errorf("Put with empty key returned %v, want ErrEmptyKey", err)
	}

	// Get with empty key
	if _, err := e.Get([]byte{}); err != engine.ErrEmptyKey {
		t.Errorf("Get with empty key returned %v, want ErrEmptyKey", err)
	}

	// Delete with empty key
	if err := e.Delete([]byte{}); err != engine.ErrEmptyKey {
		t.Errorf("Delete with empty key returned %v, want ErrEmptyKey", err)
	}

	// Has with empty key
	if _, err := e.Has([]byte{}); err != engine.ErrEmptyKey {
		t.Errorf("Has with empty key returned %v, want ErrEmptyKey", err)
	}
}

func TestEngine_EmptyValue(t *testing.T) {
	e := testEngine(t)

	key := []byte("empty-value-key")

	// Put with empty value should work
	if err := e.Put(key, []byte{}); err != nil {
		t.Fatalf("Put with empty value failed: %v", err)
	}

	// Get should return empty value
	got, err := e.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("Get returned %q, want empty", got)
	}
}

func TestEngine_Overwrite(t *testing.T) {
	e := testEngine(t)

	key := []byte("overwrite-key")
	value1 := []byte("value1")
	value2 := []byte("value2")

	// Put value1
	if err := e.Put(key, value1); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Overwrite with value2
	if err := e.Put(key, value2); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Get should return value2
	got, err := e.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !bytes.Equal(got, value2) {
		t.Errorf("Get returned %q, want %q", got, value2)
	}
}

// ============= Batch 测试 =============

func TestBatch_PutAndWrite(t *testing.T) {
	e := testEngine(t)

	batch := e.NewBatch()
	defer batch.(*WriteBatch).Close()

	// Add multiple operations
	for i := 0; i < 100; i++ {
		key := []byte(fmt.Sprintf("batch-key-%d", i))
		value := []byte(fmt.Sprintf("batch-value-%d", i))
		batch.Put(key, value)
	}

	if batch.Size() != 100 {
		t.Errorf("Batch size is %d, want 100", batch.Size())
	}

	// Write batch
	if err := batch.Write(); err != nil {
		t.Fatalf("Batch.Write failed: %v", err)
	}

	// Verify all keys exist
	for i := 0; i < 100; i++ {
		key := []byte(fmt.Sprintf("batch-key-%d", i))
		expected := []byte(fmt.Sprintf("batch-value-%d", i))

		got, err := e.Get(key)
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
	e := testEngine(t)

	// First put some keys
	for i := 0; i < 10; i++ {
		key := []byte(fmt.Sprintf("del-key-%d", i))
		if err := e.Put(key, []byte("value")); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	// Delete using batch
	batch := e.NewBatch()
	for i := 0; i < 10; i++ {
		batch.Delete([]byte(fmt.Sprintf("del-key-%d", i)))
	}

	if err := batch.Write(); err != nil {
		t.Fatalf("Batch.Write failed: %v", err)
	}

	// Verify all keys deleted
	for i := 0; i < 10; i++ {
		key := []byte(fmt.Sprintf("del-key-%d", i))
		_, err := e.Get(key)
		if err != engine.ErrNotFound {
			t.Errorf("Get(%s) after delete returned %v, want ErrNotFound", key, err)
		}
	}
}

func TestBatch_Reset(t *testing.T) {
	e := testEngine(t)

	batch := e.NewBatch()
	defer batch.(*WriteBatch).Close()

	batch.Put([]byte("key1"), []byte("value1"))
	batch.Put([]byte("key2"), []byte("value2"))

	if batch.Size() != 2 {
		t.Errorf("Batch size is %d, want 2", batch.Size())
	}

	batch.Reset()

	if batch.Size() != 0 {
		t.Errorf("Batch size after reset is %d, want 0", batch.Size())
	}
}

// ============= Iterator 测试 =============

func TestIterator_Basic(t *testing.T) {
	e := testEngine(t)

	// Put some keys
	keys := []string{"a", "b", "c", "d", "e"}
	for _, k := range keys {
		if err := e.Put([]byte(k), []byte("value-"+k)); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	// Iterate all
	iter := e.NewIterator(nil)
	defer iter.Close()

	var gotKeys []string
	for iter.First(); iter.Valid(); iter.Next() {
		gotKeys = append(gotKeys, string(iter.Key()))
	}

	if err := iter.Error(); err != nil {
		t.Fatalf("Iterator error: %v", err)
	}

	if len(gotKeys) != len(keys) {
		t.Errorf("Iterated %d keys, want %d", len(gotKeys), len(keys))
	}
}

func TestIterator_Prefix(t *testing.T) {
	e := testEngine(t)

	// Put keys with different prefixes
	prefixedKeys := []string{"prefix/a", "prefix/b", "prefix/c"}
	otherKeys := []string{"other/x", "other/y"}

	for _, k := range append(prefixedKeys, otherKeys...) {
		if err := e.Put([]byte(k), []byte("value")); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	// Iterate with prefix
	iter := e.NewPrefixIterator([]byte("prefix/"))
	defer iter.Close()

	var gotKeys []string
	for iter.First(); iter.Valid(); iter.Next() {
		gotKeys = append(gotKeys, string(iter.Key()))
	}

	if len(gotKeys) != len(prefixedKeys) {
		t.Errorf("Iterated %d keys with prefix, want %d", len(gotKeys), len(prefixedKeys))
	}
}

func TestIterator_Empty(t *testing.T) {
	e := testEngine(t)

	iter := e.NewIterator(nil)
	defer iter.Close()

	if iter.First() {
		t.Error("First() returned true for empty store")
	}

	if iter.Valid() {
		t.Error("Valid() returned true for empty store")
	}
}

// ============= Transaction 测试 =============

func TestTransaction_ReadWrite(t *testing.T) {
	e := testEngine(t)

	key := []byte("txn-key")
	value := []byte("txn-value")

	// Start write transaction
	txn := e.NewTransaction(true)
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
	got, err = e.Get(key)
	if err != nil {
		t.Fatalf("Get after commit failed: %v", err)
	}
	if !bytes.Equal(got, value) {
		t.Errorf("Get after commit returned %q, want %q", got, value)
	}
}

func TestTransaction_Discard(t *testing.T) {
	e := testEngine(t)

	key := []byte("discard-key")
	value := []byte("discard-value")

	// Start write transaction
	txn := e.NewTransaction(true)

	// Set in transaction
	if err := txn.Set(key, value); err != nil {
		t.Fatalf("Transaction.Set failed: %v", err)
	}

	// Discard
	txn.Discard()

	// Verify key does not exist
	_, err := e.Get(key)
	if err != engine.ErrNotFound {
		t.Errorf("Get after discard returned %v, want ErrNotFound", err)
	}
}

func TestTransaction_ReadOnly(t *testing.T) {
	e := testEngine(t)

	// Put a key first
	key := []byte("readonly-key")
	value := []byte("readonly-value")
	if err := e.Put(key, value); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Start read-only transaction
	txn := e.NewTransaction(false)
	defer txn.Discard()

	// Read should work
	got, err := txn.Get(key)
	if err != nil {
		t.Fatalf("Transaction.Get failed: %v", err)
	}
	if !bytes.Equal(got, value) {
		t.Errorf("Transaction.Get returned %q, want %q", got, value)
	}

	// Write should fail
	if err := txn.Set([]byte("new-key"), []byte("new-value")); err != engine.ErrReadOnly {
		t.Errorf("Transaction.Set in read-only returned %v, want ErrReadOnly", err)
	}
}

func TestTransaction_Delete(t *testing.T) {
	e := testEngine(t)

	key := []byte("txn-del-key")
	value := []byte("txn-del-value")

	// Put a key first
	if err := e.Put(key, value); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Start write transaction
	txn := e.NewTransaction(true)
	defer txn.Discard()

	// Delete in transaction
	if err := txn.Delete(key); err != nil {
		t.Fatalf("Transaction.Delete failed: %v", err)
	}

	// Commit
	if err := txn.Commit(); err != nil {
		t.Fatalf("Transaction.Commit failed: %v", err)
	}

	// Verify deleted
	_, err := e.Get(key)
	if err != engine.ErrNotFound {
		t.Errorf("Get after delete returned %v, want ErrNotFound", err)
	}
}

// ============= 并发测试 =============

func TestEngine_ConcurrentReads(t *testing.T) {
	e := testEngine(t)

	// Put some keys
	for i := 0; i < 100; i++ {
		key := []byte(fmt.Sprintf("concurrent-read-%d", i))
		value := []byte(fmt.Sprintf("value-%d", i))
		if err := e.Put(key, value); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	// Concurrent reads
	var wg sync.WaitGroup
	errCh := make(chan error, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			key := []byte(fmt.Sprintf("concurrent-read-%d", idx))
			expected := []byte(fmt.Sprintf("value-%d", idx))

			got, err := e.Get(key)
			if err != nil {
				errCh <- fmt.Errorf("Get(%s) failed: %v", key, err)
				return
			}
			if !bytes.Equal(got, expected) {
				errCh <- fmt.Errorf("Get(%s) = %q, want %q", key, got, expected)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}
}

func TestEngine_ConcurrentWrites(t *testing.T) {
	e := testEngine(t)

	// Concurrent writes
	var wg sync.WaitGroup
	errCh := make(chan error, 100)

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			key := []byte(fmt.Sprintf("concurrent-write-%d", idx))
			value := []byte(fmt.Sprintf("value-%d", idx))

			if err := e.Put(key, value); err != nil {
				errCh <- fmt.Errorf("Put(%s) failed: %v", key, err)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}

	// Verify all keys exist
	for i := 0; i < 100; i++ {
		key := []byte(fmt.Sprintf("concurrent-write-%d", i))
		if _, err := e.Get(key); err != nil {
			t.Errorf("Get(%s) after concurrent write failed: %v", key, err)
		}
	}
}

func TestEngine_ConcurrentReadWrite(t *testing.T) {
	e := testEngine(t)

	// Pre-populate some keys
	for i := 0; i < 50; i++ {
		key := []byte(fmt.Sprintf("rw-key-%d", i))
		if err := e.Put(key, []byte("initial")); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	var wg sync.WaitGroup
	errCh := make(chan error, 200)

	// Concurrent reads
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			key := []byte(fmt.Sprintf("rw-key-%d", idx))
			_, err := e.Get(key)
			if err != nil && err != engine.ErrNotFound {
				errCh <- fmt.Errorf("Get(%s) failed: %v", key, err)
			}
		}(i)
	}

	// Concurrent writes
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			key := []byte(fmt.Sprintf("rw-key-%d", idx))
			value := []byte(fmt.Sprintf("updated-%d", idx))

			if err := e.Put(key, value); err != nil {
				errCh <- fmt.Errorf("Put(%s) failed: %v", key, err)
			}
		}(i)
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Error(err)
	}
}

// ============= 关闭测试 =============

func TestEngine_CloseAndOperate(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "close-test.db")

	cfg := engine.DefaultConfig(dbPath)
	e, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// Close
	if err := e.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Operations after close should fail
	if err := e.Put([]byte("key"), []byte("value")); err != engine.ErrClosed {
		t.Errorf("Put after close returned %v, want ErrClosed", err)
	}

	if _, err := e.Get([]byte("key")); err != engine.ErrClosed {
		t.Errorf("Get after close returned %v, want ErrClosed", err)
	}

	if err := e.Delete([]byte("key")); err != engine.ErrClosed {
		t.Errorf("Delete after close returned %v, want ErrClosed", err)
	}

	if _, err := e.Has([]byte("key")); err != engine.ErrClosed {
		t.Errorf("Has after close returned %v, want ErrClosed", err)
	}
}

func TestEngine_DoubleClose(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "double-close.db")

	cfg := engine.DefaultConfig(dbPath)
	e, err := New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// First close
	if err := e.Close(); err != nil {
		t.Fatalf("First Close failed: %v", err)
	}

	// Second close should not error
	if err := e.Close(); err != nil {
		t.Errorf("Second Close returned error: %v", err)
	}
}

// ============= Stats 测试 =============

func TestEngine_Stats(t *testing.T) {
	e := testEngine(t)

	// Put some keys
	for i := 0; i < 10; i++ {
		key := []byte(fmt.Sprintf("stats-key-%d", i))
		value := []byte(fmt.Sprintf("stats-value-%d", i))
		if err := e.Put(key, value); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	stats := e.Stats()

	if stats.NumWrites != 10 {
		t.Errorf("NumWrites is %d, want 10", stats.NumWrites)
	}

	// Read some keys
	for i := 0; i < 5; i++ {
		key := []byte(fmt.Sprintf("stats-key-%d", i))
		if _, err := e.Get(key); err != nil {
			t.Fatalf("Get failed: %v", err)
		}
	}

	stats = e.Stats()

	if stats.NumReads != 5 {
		t.Errorf("NumReads is %d, want 5", stats.NumReads)
	}
}

// ============= 持久化测试 =============

func TestEngine_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "persist.db")

	// 第一次：写入数据
	{
		cfg := engine.DefaultConfig(dbPath)
		e, err := New(cfg)
		if err != nil {
			t.Fatalf("New failed: %v", err)
		}

		if err := e.Put([]byte("persist-key"), []byte("persist-value")); err != nil {
			t.Fatalf("Put failed: %v", err)
		}

		if err := e.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}
	}

	// 第二次：重新打开，验证数据存在
	{
		cfg := engine.DefaultConfig(dbPath)
		e, err := New(cfg)
		if err != nil {
			t.Fatalf("New (reopen) failed: %v", err)
		}
		defer e.Close()

		val, err := e.Get([]byte("persist-key"))
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if string(val) != "persist-value" {
			t.Errorf("unexpected value after reopen: %s", val)
		}
	}
}

func TestEngine_Sync(t *testing.T) {
	e := testEngine(t)

	// 写入数据
	if err := e.Put([]byte("sync-key"), []byte("sync-value")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// 同步到磁盘
	if err := e.Sync(); err != nil {
		t.Fatalf("Sync failed: %v", err)
	}
}

func TestEngine_Compact(t *testing.T) {
	e := testEngine(t)

	// 写入一些数据然后删除，制造垃圾
	for i := 0; i < 100; i++ {
		key := []byte(fmt.Sprintf("compact-key-%d", i))
		if err := e.Put(key, []byte("value")); err != nil {
			t.Fatalf("Put failed: %v", err)
		}
	}

	for i := 0; i < 100; i++ {
		key := []byte(fmt.Sprintf("compact-key-%d", i))
		if err := e.Delete(key); err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
	}

	// 执行压缩（不应该出错）
	if err := e.Compact(); err != nil {
		t.Fatalf("Compact failed: %v", err)
	}
}
