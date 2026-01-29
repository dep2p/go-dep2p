package metadata

import (
	"path/filepath"
	"testing"

	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dep2p/go-dep2p/internal/core/storage/engine/badger"
	"github.com/dep2p/go-dep2p/internal/core/storage/kv"
	"github.com/dep2p/go-dep2p/pkg/types"
)

func newTestPersistentMetadataStore(t *testing.T) (*PersistentMetadataStore, func()) {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := engine.DefaultConfig(dbPath)
	eng, err := badger.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	store := kv.New(eng, []byte("p/m/"))
	ms, err := NewPersistent(store)
	if err != nil {
		eng.Close()
		t.Fatalf("failed to create persistent metadata store: %v", err)
	}

	cleanup := func() {
		eng.Close()
	}

	return ms, cleanup
}

func TestPersistentMetadataStore_PutAndGet(t *testing.T) {
	ms, cleanup := newTestPersistentMetadataStore(t)
	defer cleanup()

	peerID := types.PeerID("test-peer-1")

	// 存储字符串
	if err := ms.Put(peerID, "name", "test-node"); err != nil {
		t.Fatalf("failed to put metadata: %v", err)
	}

	// 获取
	val, err := ms.Get(peerID, "name")
	if err != nil {
		t.Fatalf("failed to get metadata: %v", err)
	}

	if val != "test-node" {
		t.Errorf("expected 'test-node', got %v", val)
	}
}

func TestPersistentMetadataStore_PutAndGetNumber(t *testing.T) {
	ms, cleanup := newTestPersistentMetadataStore(t)
	defer cleanup()

	peerID := types.PeerID("test-peer-1")

	// 存储数字
	if err := ms.Put(peerID, "port", float64(4001)); err != nil {
		t.Fatalf("failed to put metadata: %v", err)
	}

	// 获取
	val, err := ms.Get(peerID, "port")
	if err != nil {
		t.Fatalf("failed to get metadata: %v", err)
	}

	// JSON 解析数字为 float64
	if val != float64(4001) {
		t.Errorf("expected 4001, got %v", val)
	}
}

func TestPersistentMetadataStore_PutAndGetStruct(t *testing.T) {
	ms, cleanup := newTestPersistentMetadataStore(t)
	defer cleanup()

	peerID := types.PeerID("test-peer-1")

	// 存储结构体（map）
	data := map[string]interface{}{
		"version": "1.0.0",
		"count":   float64(100),
	}

	if err := ms.Put(peerID, "info", data); err != nil {
		t.Fatalf("failed to put metadata: %v", err)
	}

	// 获取
	val, err := ms.Get(peerID, "info")
	if err != nil {
		t.Fatalf("failed to get metadata: %v", err)
	}

	valMap, ok := val.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", val)
	}

	if valMap["version"] != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %v", valMap["version"])
	}
}

func TestPersistentMetadataStore_Delete(t *testing.T) {
	ms, cleanup := newTestPersistentMetadataStore(t)
	defer cleanup()

	peerID := types.PeerID("test-peer-1")

	// 存储
	ms.Put(peerID, "key1", "value1")

	// 删除
	if err := ms.Delete(peerID, "key1"); err != nil {
		t.Fatalf("failed to delete metadata: %v", err)
	}

	// 验证已删除
	val, err := ms.Get(peerID, "key1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != nil {
		t.Errorf("expected nil, got %v", val)
	}
}

func TestPersistentMetadataStore_RemovePeer(t *testing.T) {
	ms, cleanup := newTestPersistentMetadataStore(t)
	defer cleanup()

	peerID := types.PeerID("test-peer-1")

	// 存储多个键
	ms.Put(peerID, "key1", "value1")
	ms.Put(peerID, "key2", "value2")

	// 移除节点
	ms.RemovePeer(peerID)

	// 验证所有键已删除
	val1, _ := ms.Get(peerID, "key1")
	val2, _ := ms.Get(peerID, "key2")

	if val1 != nil || val2 != nil {
		t.Errorf("expected all values nil, got key1=%v, key2=%v", val1, val2)
	}
}

func TestPersistentMetadataStore_GetAll(t *testing.T) {
	ms, cleanup := newTestPersistentMetadataStore(t)
	defer cleanup()

	peerID := types.PeerID("test-peer-1")

	// 存储多个键
	ms.Put(peerID, "key1", "value1")
	ms.Put(peerID, "key2", "value2")

	// 获取所有
	all := ms.GetAll(peerID)
	if len(all) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(all))
	}

	if all["key1"] != "value1" {
		t.Errorf("expected 'value1', got %v", all["key1"])
	}

	if all["key2"] != "value2" {
		t.Errorf("expected 'value2', got %v", all["key2"])
	}
}

func TestPersistentMetadataStore_Persistence(t *testing.T) {
	// 创建引擎
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "persist.db")
	cfg := engine.DefaultConfig(dbPath)
	eng, err := badger.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer eng.Close()

	store := kv.New(eng, []byte("p/m/"))
	peerID := types.PeerID("test-peer-1")

	// 创建第一个 MetadataStore 并写入数据
	ms1, err := NewPersistent(store)
	if err != nil {
		t.Fatalf("failed to create persistent metadata store: %v", err)
	}

	if err := ms1.Put(peerID, "key", "value"); err != nil {
		t.Fatalf("failed to put metadata: %v", err)
	}

	// 创建第二个 MetadataStore，验证数据持久化
	ms2, err := NewPersistent(store)
	if err != nil {
		t.Fatalf("failed to create persistent metadata store: %v", err)
	}

	val, _ := ms2.Get(peerID, "key")
	if val != "value" {
		t.Errorf("expected 'value', got %v", val)
	}
}

func TestPersistentMetadataStore_PeersWithMetadata(t *testing.T) {
	ms, cleanup := newTestPersistentMetadataStore(t)
	defer cleanup()

	peer1 := types.PeerID("test-peer-1")
	peer2 := types.PeerID("test-peer-2")

	// 存储数据
	ms.Put(peer1, "key", "value1")
	ms.Put(peer2, "key", "value2")

	// 获取有元数据的节点
	peers := ms.PeersWithMetadata()
	if len(peers) != 2 {
		t.Fatalf("expected 2 peers, got %d", len(peers))
	}
}
