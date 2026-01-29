package addressbook_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/relay/addressbook"
	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dep2p/go-dep2p/internal/core/storage/engine/badger"
	realmif "github.com/dep2p/go-dep2p/internal/realm/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// testEngine 创建测试用的存储引擎
func testEngine(t *testing.T) engine.InternalEngine {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	cfg := engine.DefaultConfig(dbPath)
	eng, err := badger.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	t.Cleanup(func() {
		eng.Close()
	})

	return eng
}

// testStore 创建测试用的 BadgerStore
func testStore(t *testing.T) realmif.AddressBookStore {
	t.Helper()

	eng := testEngine(t)
	store, err := addressbook.NewBadgerStoreWithEngine(eng)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	return store
}

// testBook 创建测试用的 MemberAddressBook
func testBook(t *testing.T) *addressbook.MemberAddressBook {
	t.Helper()

	eng := testEngine(t)
	book, err := addressbook.NewWithEngine("test-realm", eng)
	if err != nil {
		t.Fatalf("failed to create addressbook: %v", err)
	}

	t.Cleanup(func() {
		book.Close()
	})

	return book
}

// 辅助函数：创建测试用的 MemberEntry
func createTestEntry(nodeID string, online bool) realmif.MemberEntry {
	addr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	return realmif.MemberEntry{
		NodeID:       types.PeerID(nodeID),
		DirectAddrs:  []types.Multiaddr{addr},
		NATType:      types.NATTypeFullCone,
		Capabilities: []string{"relay", "dht"},
		Online:       online,
		LastSeen:     time.Now(),
		LastUpdate:   time.Now(),
	}
}

// ============================================================================
//                              BadgerStore 测试
// ============================================================================

func TestBadgerStore_PutGet(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	ctx := context.Background()
	entry := createTestEntry("test-node-1", true)

	// Put
	err := store.Put(ctx, entry)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Get
	got, found, err := store.Get(ctx, entry.NodeID)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !found {
		t.Fatal("Entry not found")
	}
	if got.NodeID != entry.NodeID {
		t.Errorf("NodeID = %v, want %v", got.NodeID, entry.NodeID)
	}
}

func TestBadgerStore_Delete(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	ctx := context.Background()
	entry := createTestEntry("test-node-1", true)

	// Put
	store.Put(ctx, entry)

	// Delete
	err := store.Delete(ctx, entry.NodeID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	_, found, _ := store.Get(ctx, entry.NodeID)
	if found {
		t.Error("Entry should be deleted")
	}
}

func TestBadgerStore_List(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	ctx := context.Background()

	// Put multiple entries
	for i := 0; i < 5; i++ {
		entry := createTestEntry("test-node-"+string(rune('a'+i)), true)
		store.Put(ctx, entry)
	}

	// List
	entries, err := store.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(entries) != 5 {
		t.Errorf("List returned %d entries, want 5", len(entries))
	}
}

func TestBadgerStore_Close(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "close-test.db")
	cfg := engine.DefaultConfig(dbPath)
	eng, err := badger.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	store, err := addressbook.NewBadgerStoreWithEngine(eng)
	if err != nil {
		eng.Close()
		t.Fatalf("failed to create store: %v", err)
	}

	// Close
	err = store.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Close engine
	eng.Close()

	// Operations should fail after close
	ctx := context.Background()
	entry := createTestEntry("test-node-1", true)

	err = store.Put(ctx, entry)
	if err != addressbook.ErrStoreClosed {
		t.Errorf("Put after close should return ErrStoreClosed, got %v", err)
	}
}

func TestBadgerStore_InvalidNodeID(t *testing.T) {
	store := testStore(t)
	defer store.Close()

	ctx := context.Background()

	// Empty NodeID
	entry := realmif.MemberEntry{
		NodeID: types.EmptyPeerID,
	}

	err := store.Put(ctx, entry)
	if err != addressbook.ErrInvalidNodeID {
		t.Errorf("Put with empty NodeID should return ErrInvalidNodeID, got %v", err)
	}
}

// ============================================================================
//                              MemberAddressBook 测试
// ============================================================================

func TestAddressBook_Register(t *testing.T) {
	book := testBook(t)

	ctx := context.Background()
	entry := createTestEntry("test-node-1", false)

	// Register
	err := book.Register(ctx, entry)
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Verify registered
	got, err := book.Query(ctx, entry.NodeID)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if got.NodeID != entry.NodeID {
		t.Errorf("NodeID = %v, want %v", got.NodeID, entry.NodeID)
	}
	if !got.Online {
		t.Error("Registered entry should be online")
	}
}

func TestAddressBook_Query(t *testing.T) {
	book := testBook(t)

	ctx := context.Background()
	entry := createTestEntry("test-node-1", true)

	// Register
	book.Register(ctx, entry)

	// Query existing
	got, err := book.Query(ctx, entry.NodeID)
	if err != nil {
		t.Fatalf("Query failed: %v", err)
	}
	if got.NodeID != entry.NodeID {
		t.Errorf("NodeID = %v, want %v", got.NodeID, entry.NodeID)
	}

	// Query non-existing
	_, err = book.Query(ctx, "non-existing")
	if err != addressbook.ErrMemberNotFound {
		t.Errorf("Query non-existing should return ErrMemberNotFound, got %v", err)
	}
}

func TestAddressBook_Update(t *testing.T) {
	book := testBook(t)

	ctx := context.Background()
	entry := createTestEntry("test-node-1", true)

	// Register
	book.Register(ctx, entry)

	// Update
	newAddr, _ := types.NewMultiaddr("/ip4/10.0.0.1/tcp/4001")
	entry.DirectAddrs = []types.Multiaddr{newAddr}
	entry.NATType = types.NATTypeSymmetric

	err := book.Update(ctx, entry)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Verify updated
	got, _ := book.Query(ctx, entry.NodeID)
	if got.NATType != types.NATTypeSymmetric {
		t.Errorf("NATType = %v, want Symmetric", got.NATType)
	}
}

func TestAddressBook_UpdateNonExisting(t *testing.T) {
	book := testBook(t)

	ctx := context.Background()
	entry := createTestEntry("test-node-1", true)

	// Update without register
	err := book.Update(ctx, entry)
	if err != addressbook.ErrMemberNotFound {
		t.Errorf("Update non-existing should return ErrMemberNotFound, got %v", err)
	}
}

func TestAddressBook_Remove(t *testing.T) {
	book := testBook(t)

	ctx := context.Background()
	entry := createTestEntry("test-node-1", true)

	// Register
	book.Register(ctx, entry)

	// Remove
	err := book.Remove(ctx, entry.NodeID)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	// Verify removed
	_, err = book.Query(ctx, entry.NodeID)
	if err != addressbook.ErrMemberNotFound {
		t.Errorf("Query after remove should return ErrMemberNotFound, got %v", err)
	}
}

func TestAddressBook_Members(t *testing.T) {
	book := testBook(t)

	ctx := context.Background()

	// Register multiple
	for i := 0; i < 5; i++ {
		entry := createTestEntry("test-node-"+string(rune('a'+i)), true)
		book.Register(ctx, entry)
	}

	// Get all members
	members, err := book.Members(ctx)
	if err != nil {
		t.Fatalf("Members failed: %v", err)
	}
	if len(members) != 5 {
		t.Errorf("Members returned %d, want 5", len(members))
	}
}

func TestAddressBook_OnlineMembers(t *testing.T) {
	book := testBook(t)

	ctx := context.Background()

	// Register mixed online/offline
	for i := 0; i < 5; i++ {
		entry := createTestEntry("test-node-"+string(rune('a'+i)), true)
		book.Register(ctx, entry)
	}

	// Set some offline
	book.SetOnline(ctx, "test-node-a", false)
	book.SetOnline(ctx, "test-node-c", false)

	// Get online members
	online, err := book.OnlineMembers(ctx)
	if err != nil {
		t.Fatalf("OnlineMembers failed: %v", err)
	}
	if len(online) != 3 {
		t.Errorf("OnlineMembers returned %d, want 3", len(online))
	}
}

func TestAddressBook_SetOnline(t *testing.T) {
	book := testBook(t)

	ctx := context.Background()
	entry := createTestEntry("test-node-1", true)

	// Register
	book.Register(ctx, entry)

	// Set offline
	err := book.SetOnline(ctx, entry.NodeID, false)
	if err != nil {
		t.Fatalf("SetOnline failed: %v", err)
	}

	// Verify offline
	got, _ := book.Query(ctx, entry.NodeID)
	if got.Online {
		t.Error("Entry should be offline")
	}

	// Set back online
	book.SetOnline(ctx, entry.NodeID, true)
	got, _ = book.Query(ctx, entry.NodeID)
	if !got.Online {
		t.Error("Entry should be online")
	}
}

func TestAddressBook_Close(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "close-test.db")
	cfg := engine.DefaultConfig(dbPath)
	eng, err := badger.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	book, err := addressbook.NewWithEngine("test-realm", eng)
	if err != nil {
		eng.Close()
		t.Fatalf("failed to create addressbook: %v", err)
	}

	// Close
	err = book.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Close engine
	eng.Close()

	// Operations should fail after close
	ctx := context.Background()
	entry := createTestEntry("test-node-1", true)

	err = book.Register(ctx, entry)
	if err != addressbook.ErrBookClosed {
		t.Errorf("Register after close should return ErrBookClosed, got %v", err)
	}
}

// ============================================================================
//                              Entry 转换测试
// ============================================================================

func TestEntryToProto(t *testing.T) {
	entry := createTestEntry("test-node-1", true)

	pb := addressbook.EntryToProto(entry)
	if pb == nil {
		t.Fatal("EntryToProto returned nil")
	}
	if string(pb.NodeId) != "test-node-1" {
		t.Errorf("NodeId = %s, want test-node-1", pb.NodeId)
	}
	if !pb.Online {
		t.Error("Online should be true")
	}
}

func TestEntryFromProto(t *testing.T) {
	entry := createTestEntry("test-node-1", true)
	pb := addressbook.EntryToProto(entry)

	// Round trip
	got, err := addressbook.EntryFromProto(pb)
	if err != nil {
		t.Fatalf("EntryFromProto failed: %v", err)
	}
	if got.NodeID != entry.NodeID {
		t.Errorf("NodeID = %v, want %v", got.NodeID, entry.NodeID)
	}
	if got.NATType != entry.NATType {
		t.Errorf("NATType = %v, want %v", got.NATType, entry.NATType)
	}
}

func TestEntryFromProto_Nil(t *testing.T) {
	_, err := addressbook.EntryFromProto(nil)
	if err != addressbook.ErrInvalidEntry {
		t.Errorf("EntryFromProto(nil) should return ErrInvalidEntry, got %v", err)
	}
}

// ============================================================================
//                              配置验证测试
// ============================================================================

func TestConfig_Validate(t *testing.T) {
	// 没有 Engine 或 Store 应该失败
	cfg := addressbook.Config{
		RealmID: "test-realm",
	}

	err := cfg.Validate()
	if err == nil {
		t.Error("Validate should fail without Engine or Store")
	}
}

func TestNewWithStore_NilStore(t *testing.T) {
	// nil store 应该失败
	_, err := addressbook.NewWithStore("test-realm", nil)
	if err == nil {
		t.Error("NewWithStore should fail with nil store")
	}
}

func TestNewWithEngine_NilEngine(t *testing.T) {
	// nil engine 应该失败
	_, err := addressbook.NewWithEngine("test-realm", nil)
	if err == nil {
		t.Error("NewWithEngine should fail with nil engine")
	}
}
