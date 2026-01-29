package keybook

import (
	"path/filepath"
	"testing"

	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dep2p/go-dep2p/internal/core/storage/engine/badger"
	"github.com/dep2p/go-dep2p/internal/core/storage/kv"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testKeyDeserializer 测试用密钥反序列化器
type testKeyDeserializer struct{}

func (d *testKeyDeserializer) DeserializePublicKey(data []byte) (pkgif.PublicKey, error) {
	return &testPublicKey{data: data}, nil
}

func (d *testKeyDeserializer) DeserializePrivateKey(data []byte) (pkgif.PrivateKey, error) {
	return &testPrivateKey{
		data:   data,
		pubKey: &testPublicKey{data: data}, // 使用相同数据创建公钥
	}, nil
}

// testPersistentStore 创建测试用持久化存储
func testPersistentStore(t *testing.T) *kv.Store {
	t.Helper()

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := engine.DefaultConfig(dbPath)
	eng, err := badger.New(cfg)
	require.NoError(t, err, "创建存储引擎失败")

	t.Cleanup(func() {
		assert.NoError(t, eng.Close(), "关闭存储引擎失败")
	})

	return kv.New(eng, []byte("p/k/"))
}

// TestNewPersistent_WithoutDeserializer 测试不带反序列化器创建
func TestNewPersistent_WithoutDeserializer(t *testing.T) {
	store := testPersistentStore(t)

	kb, err := NewPersistent(store, nil)
	require.NoError(t, err)
	require.NotNil(t, kb)
	assert.NotNil(t, kb.pubKeys)
	assert.NotNil(t, kb.privKeys)
}

// TestNewPersistent_WithDeserializer 测试带反序列化器创建
func TestNewPersistent_WithDeserializer(t *testing.T) {
	store := testPersistentStore(t)
	deserializer := &testKeyDeserializer{}

	kb, err := NewPersistent(store, deserializer)
	require.NoError(t, err)
	require.NotNil(t, kb)
}

// TestPersistentKeyBook_AddPubKey 测试添加公钥
func TestPersistentKeyBook_AddPubKey(t *testing.T) {
	store := testPersistentStore(t)
	kb, err := NewPersistent(store, &testKeyDeserializer{})
	require.NoError(t, err)

	peerID := testPeerID("peer1")
	pubKey := testPubKey("key1")

	// 添加公钥
	err = kb.AddPubKey(peerID, pubKey)
	require.NoError(t, err)

	// 验证内存缓存
	cached, err := kb.PubKey(peerID)
	require.NoError(t, err)
	require.NotNil(t, cached)
	assert.True(t, cached.Equals(pubKey))

	// 验证持久化存储
	keyData, err := pubKey.Raw()
	require.NoError(t, err)
	stored, err := store.Get(append([]byte(pubKeyPrefix), []byte(peerID)...))
	require.NoError(t, err)
	assert.Equal(t, keyData, stored)
}

// TestPersistentKeyBook_AddPubKey_NilKey 测试添加 nil 公钥（应返回错误或忽略）
func TestPersistentKeyBook_AddPubKey_NilKey(t *testing.T) {
	store := testPersistentStore(t)
	kb, err := NewPersistent(store, &testKeyDeserializer{})
	require.NoError(t, err)

	peerID := testPeerID("peer1")

	// BUG 检测：添加 nil 公钥
	err = kb.AddPubKey(peerID, nil)
	
	// 根据代码逻辑，应该返回错误或者忽略
	// 这里我们检测是否会 panic 或产生不一致状态
	if err == nil {
		// 如果没有返回错误，验证状态一致性
		retrieved, err := kb.PubKey(peerID)
		// 应该返回 nil，而不是崩溃
		assert.Nil(t, retrieved, "添加 nil 公钥后应返回 nil")
		_ = err
	}
}

// TestPersistentKeyBook_PubKey 测试获取公钥
func TestPersistentKeyBook_PubKey(t *testing.T) {
	store := testPersistentStore(t)
	kb, err := NewPersistent(store, &testKeyDeserializer{})
	require.NoError(t, err)

	peerID := testPeerID("peer1")
	pubKey := testPubKey("key1")

	// 添加公钥
	err = kb.AddPubKey(peerID, pubKey)
	require.NoError(t, err)

	// 获取公钥
	retrieved, err := kb.PubKey(peerID)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.True(t, retrieved.Equals(pubKey))
}

// TestPersistentKeyBook_PubKey_NotFound 测试获取不存在的公钥
func TestPersistentKeyBook_PubKey_NotFound(t *testing.T) {
	store := testPersistentStore(t)
	kb, err := NewPersistent(store, &testKeyDeserializer{})
	require.NoError(t, err)

	peerID := testPeerID("peer1")

	// 获取不存在的公钥
	retrieved, err := kb.PubKey(peerID)
	assert.Error(t, err, "不存在的公钥应返回错误")
	assert.Nil(t, retrieved)
}

// TestPersistentKeyBook_AddPrivKey 测试添加私钥
func TestPersistentKeyBook_AddPrivKey(t *testing.T) {
	store := testPersistentStore(t)
	kb, err := NewPersistent(store, &testKeyDeserializer{})
	require.NoError(t, err)

	peerID := testPeerID("peer1")
	privKey := &testPrivateKey{
		data:   []byte("privkey1"),
		pubKey: testPubKey("pubkey1"),
	}

	// 添加私钥
	err = kb.AddPrivKey(peerID, privKey)
	require.NoError(t, err)

	// 验证内存缓存
	cached, err := kb.PrivKey(peerID)
	require.NoError(t, err)
	require.NotNil(t, cached)
	assert.True(t, cached.Equals(privKey))

	// 验证持久化存储
	keyData, err := privKey.Raw()
	require.NoError(t, err)
	stored, err := store.Get(append([]byte(privKeyPrefix), []byte(peerID)...))
	require.NoError(t, err)
	assert.Equal(t, keyData, stored)
}

// TestPersistentKeyBook_PrivKey 测试获取私钥
func TestPersistentKeyBook_PrivKey(t *testing.T) {
	store := testPersistentStore(t)
	kb, err := NewPersistent(store, &testKeyDeserializer{})
	require.NoError(t, err)

	peerID := testPeerID("peer1")
	privKey := &testPrivateKey{
		data:   []byte("privkey1"),
		pubKey: testPubKey("pubkey1"),
	}

	// 添加私钥
	err = kb.AddPrivKey(peerID, privKey)
	require.NoError(t, err)

	// 获取私钥
	retrieved, err := kb.PrivKey(peerID)
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	assert.True(t, retrieved.Equals(privKey))
}

// TestPersistentKeyBook_PeersWithKeys 测试获取有密钥的节点列表
func TestPersistentKeyBook_PeersWithKeys(t *testing.T) {
	store := testPersistentStore(t)
	kb, err := NewPersistent(store, &testKeyDeserializer{})
	require.NoError(t, err)

	peer1 := testPeerID("peer1")
	peer2 := testPeerID("peer2")
	peer3 := testPeerID("peer3")

	// 添加公钥
	require.NoError(t, kb.AddPubKey(peer1, testPubKey("key1")))
	require.NoError(t, kb.AddPubKey(peer2, testPubKey("key2")))

	// 添加私钥
	require.NoError(t, kb.AddPrivKey(peer3, &testPrivateKey{
		data:   []byte("privkey3"),
		pubKey: testPubKey("pubkey3"),
	}))

	// 获取有密钥的节点
	peers := kb.PeersWithKeys()
	assert.Len(t, peers, 3)
	assert.Contains(t, peers, peer1)
	assert.Contains(t, peers, peer2)
	assert.Contains(t, peers, peer3)
}

// TestPersistentKeyBook_RemovePeer 测试删除节点
func TestPersistentKeyBook_RemovePeer(t *testing.T) {
	store := testPersistentStore(t)
	kb, err := NewPersistent(store, &testKeyDeserializer{})
	require.NoError(t, err)

	peerID := testPeerID("peer1")
	pubKey := testPubKey("key1")
	privKey := &testPrivateKey{
		data:   []byte("privkey1"),
		pubKey: pubKey,
	}

	// 添加密钥
	require.NoError(t, kb.AddPubKey(peerID, pubKey))
	require.NoError(t, kb.AddPrivKey(peerID, privKey))

	// 删除节点
	kb.RemovePeer(peerID)

	// 验证内存缓存已删除
	pubKeyAfter, err := kb.PubKey(peerID)
	assert.Error(t, err)
	assert.Nil(t, pubKeyAfter)
	
	privKeyAfter, err := kb.PrivKey(peerID)
	assert.Error(t, err)
	assert.Nil(t, privKeyAfter)

	// 验证持久化存储已删除
	_, err = store.Get(append([]byte(pubKeyPrefix), []byte(peerID)...))
	assert.Error(t, err, "公钥应该已从存储中删除")

	_, err = store.Get(append([]byte(privKeyPrefix), []byte(peerID)...))
	assert.Error(t, err, "私钥应该已从存储中删除")
}

// TestPersistentKeyBook_Persistence 测试数据持久化
func TestPersistentKeyBook_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// 第一次：创建并添加数据
	{
		cfg := engine.DefaultConfig(dbPath)
		eng, err := badger.New(cfg)
		require.NoError(t, err)

		store := kv.New(eng, []byte("p/k/"))
		kb, err := NewPersistent(store, &testKeyDeserializer{})
		require.NoError(t, err)

		peerID := testPeerID("peer1")
		pubKey := testPubKey("key1")
		require.NoError(t, kb.AddPubKey(peerID, pubKey))

		require.NoError(t, eng.Close())
	}

	// 第二次：重新打开，验证数据已持久化
	{
		cfg := engine.DefaultConfig(dbPath)
		eng, err := badger.New(cfg)
		require.NoError(t, err)
		defer eng.Close()

		store := kv.New(eng, []byte("p/k/"))
		kb, err := NewPersistent(store, &testKeyDeserializer{})
		require.NoError(t, err)

		peerID := testPeerID("peer1")
		
		// BUG 检测：数据应该从存储中加载
		retrieved, err := kb.PubKey(peerID)
		require.NoError(t, err, "从存储加载应该成功")
		require.NotNil(t, retrieved, "持久化的公钥应该能够加载")
		
		expectedData := []byte("key1")
		actualData, err := retrieved.Raw()
		require.NoError(t, err)
		assert.Equal(t, expectedData, actualData, "加载的公钥数据应该匹配")
	}
}

// TestPersistentKeyBook_Concurrent 测试并发访问
func TestPersistentKeyBook_Concurrent(t *testing.T) {
	store := testPersistentStore(t)
	kb, err := NewPersistent(store, &testKeyDeserializer{})
	require.NoError(t, err)

	const goroutines = 10
	const operationsPerGoroutine = 100

	done := make(chan bool, goroutines)

	// 并发写入
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer func() { done <- true }()
			
			for j := 0; j < operationsPerGoroutine; j++ {
				peerID := testPeerID(string(rune(id*operationsPerGoroutine + j)))
				pubKey := testPubKey(string(rune(id*operationsPerGoroutine + j)))
				
				// BUG 检测：并发添加应该不会 panic 或数据竞争
				if err := kb.AddPubKey(peerID, pubKey); err != nil {
					t.Errorf("并发添加公钥失败: %v", err)
				}
				
				// BUG 检测：并发读取应该不会 panic
				_, _ = kb.PubKey(peerID)
			}
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < goroutines; i++ {
		<-done
	}

	// 验证所有密钥都已添加
	peers := kb.PeersWithKeys()
	assert.Equal(t, goroutines*operationsPerGoroutine, len(peers), "并发添加的所有密钥应该都存在")
}

// TestPersistentKeyBook_LoadFromStore 测试从存储加载数据
func TestPersistentKeyBook_LoadFromStore(t *testing.T) {
	store := testPersistentStore(t)
	
	// 直接向存储写入数据（模拟已存在的数据）
	peerID := testPeerID("peer1")
	keyData := []byte("key1")
	
	pubKeyKey := append([]byte(pubKeyPrefix), []byte(peerID)...)
	require.NoError(t, store.Put(pubKeyKey, keyData))

	// 创建 PersistentKeyBook，应该自动加载
	kb, err := NewPersistent(store, &testKeyDeserializer{})
	require.NoError(t, err)

	// 验证已加载
	retrieved, err := kb.PubKey(peerID)
	require.NoError(t, err, "从存储加载应该成功")
	require.NotNil(t, retrieved, "应该从存储加载已有数据")
	
	actualData, err := retrieved.Raw()
	require.NoError(t, err)
	assert.Equal(t, keyData, actualData)
}

// TestPersistentKeyBook_AddPrivKey_NilPublicKey 测试私钥的公钥为 nil 的情况 (BUG #B5 修复验证)
func TestPersistentKeyBook_AddPrivKey_NilPublicKey(t *testing.T) {
	store := testPersistentStore(t)
	kb, err := NewPersistent(store, &testKeyDeserializer{})
	require.NoError(t, err)

	peerID := testPeerID("peer1")
	
	// 创建一个公钥为 nil 的私钥
	privKeyWithNilPubKey := &testPrivateKey{
		data:   []byte("privkey1"),
		pubKey: nil, // ← 故意设置为 nil
	}

	// BUG #B5 修复验证：应该返回错误而不是 panic
	err = kb.AddPrivKey(peerID, privKeyWithNilPubKey)
	
	// 验证：应该返回 ErrInvalidPublicKey 错误
	require.Error(t, err, "公钥为 nil 的私钥应该返回错误")
	assert.ErrorIs(t, err, ErrInvalidPublicKey, "应该返回 ErrInvalidPublicKey")
	
	// 验证：不应该有任何数据被存储
	retrievedPrivKey, err := kb.PrivKey(peerID)
	assert.Error(t, err, "不应该存储无效的私钥")
	assert.Nil(t, retrievedPrivKey)
	
	retrievedPubKey, err := kb.PubKey(peerID)
	assert.Error(t, err, "不应该存储 nil 公钥")
	assert.Nil(t, retrievedPubKey)
}

// TestPersistentKeyBook_EmptyPeerID 测试空 PeerID
func TestPersistentKeyBook_EmptyPeerID(t *testing.T) {
	store := testPersistentStore(t)
	kb, err := NewPersistent(store, &testKeyDeserializer{})
	require.NoError(t, err)

	emptyPeerID := testPeerID("")
	pubKey := testPubKey("key1")

	// BUG 检测：空 PeerID 应该如何处理？
	err = kb.AddPubKey(emptyPeerID, pubKey)
	
	// 可能的行为：
	// 1. 返回错误（推荐）
	// 2. 忽略（可接受）
	// 3. panic（BUG）
	
	// 这里我们只是确保不 panic
	// 如果返回错误，这是期望的行为
	if err == nil {
		// 如果没有错误，确保状态一致
		retrieved, err := kb.PubKey(emptyPeerID)
		// 至少不应该 panic
		_ = retrieved
		_ = err
	}
}
