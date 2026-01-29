package keybook

import (
	"fmt"
	"testing"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 测试辅助
func testPeerID(s string) types.PeerID {
	return types.PeerID(s)
}

func testPubKey(s string) pkgif.PublicKey {
	return &testPublicKey{data: []byte(s)}
}

type testPublicKey struct {
	data []byte
}

func (k *testPublicKey) Raw() ([]byte, error) { return k.data, nil }
func (k *testPublicKey) Type() pkgif.KeyType  { return pkgif.KeyTypeEd25519 }
func (k *testPublicKey) Equals(other pkgif.PublicKey) bool {
	if other == nil {
		return false
	}
	otherData, _ := other.Raw()
	return string(k.data) == string(otherData)
}
func (k *testPublicKey) Verify(data []byte, sig []byte) (bool, error) { return true, nil }
func (k *testPublicKey) String() string                               { return fmt.Sprintf("TestPubKey(%s)", string(k.data)) }

func TestNew(t *testing.T) {
	kb := New()
	require.NotNil(t, kb)
}

func TestKeyBook_AddPubKey(t *testing.T) {
	kb := New()
	peerID := testPeerID("peer1")
	pubKey := testPubKey("key1")

	err := kb.AddPubKey(peerID, pubKey)
	require.NoError(t, err)

	retrieved, err := kb.PubKey(peerID)
	require.NoError(t, err)
	assert.True(t, pubKey.Equals(retrieved))
}

func TestKeyBook_PubKey_NotFound(t *testing.T) {
	kb := New()
	peerID := testPeerID("nonexistent")

	_, err := kb.PubKey(peerID)
	assert.Error(t, err)
}

func TestKeyBook_PeersWithKeys(t *testing.T) {
	kb := New()

	peer1 := testPeerID("peer1")
	peer2 := testPeerID("peer2")

	kb.AddPubKey(peer1, testPubKey("key1"))
	kb.AddPubKey(peer2, testPubKey("key2"))

	peers := kb.PeersWithKeys()
	assert.Len(t, peers, 2)
	assert.Contains(t, peers, peer1)
	assert.Contains(t, peers, peer2)
}

func TestKeyBook_RemovePeer(t *testing.T) {
	kb := New()
	peerID := testPeerID("peer1")

	kb.AddPubKey(peerID, testPubKey("key1"))
	kb.RemovePeer(peerID)

	_, err := kb.PubKey(peerID)
	assert.Error(t, err)
}

func TestKeyBook_PeerIDValidation(t *testing.T) {
	kb := New()

	// 使用一个简单的测试 PeerID 和公钥
	// 注意：真正的验证需要 PeerID 是有效的 Multihash 格式
	// 对于简单字符串 PeerID，验证会被跳过

	// 测试简单 PeerID（非 Multihash 格式）
	simplePeerID := testPeerID("simple-peer")
	simpleKey := testPubKey("test-key-data")

	// 简单 PeerID 不会触发严格验证
	err := kb.AddPubKey(simplePeerID, simpleKey)
	assert.NoError(t, err, "简单 PeerID 应该允许添加任何公钥")

	// 验证公钥已存储
	retrievedKey, err := kb.PubKey(simplePeerID)
	assert.NoError(t, err)
	assert.NotNil(t, retrievedKey)
}

// ============================================================================
// 补充测试（提升覆盖率）
// ============================================================================

// testPrivateKey 测试用私钥
type testPrivateKey struct {
	data   []byte
	pubKey pkgif.PublicKey
}

func (k *testPrivateKey) Raw() ([]byte, error)          { return k.data, nil }
func (k *testPrivateKey) Type() pkgif.KeyType           { return pkgif.KeyTypeEd25519 }
func (k *testPrivateKey) PublicKey() pkgif.PublicKey    { return k.pubKey }
func (k *testPrivateKey) Sign(data []byte) ([]byte, error) { return []byte("signature"), nil }
func (k *testPrivateKey) Equals(other pkgif.PrivateKey) bool {
	if other == nil {
		return false
	}
	otherData, _ := other.Raw()
	return string(k.data) == string(otherData)
}
func (k *testPrivateKey) String() string { return fmt.Sprintf("TestPrivKey(%s)", string(k.data)) }

func testPrivKey(s string) pkgif.PrivateKey {
	pubKey := testPubKey(s + "-pub")
	return &testPrivateKey{data: []byte(s), pubKey: pubKey}
}

// errorPublicKey 返回错误的公钥
type errorPublicKey struct{}

func (k *errorPublicKey) Raw() ([]byte, error)                    { return nil, fmt.Errorf("raw error") }
func (k *errorPublicKey) Type() pkgif.KeyType                     { return pkgif.KeyTypeEd25519 }
func (k *errorPublicKey) Equals(other pkgif.PublicKey) bool       { return false }
func (k *errorPublicKey) Verify(data []byte, sig []byte) (bool, error) { return false, nil }
func (k *errorPublicKey) String() string                          { return "ErrorPubKey" }

// TestKeyBook_AddPubKey_NilKey 测试添加 nil 公钥
func TestKeyBook_AddPubKey_NilKey(t *testing.T) {
	kb := New()
	peerID := testPeerID("peer1")

	err := kb.AddPubKey(peerID, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

// TestKeyBook_AddPubKey_RawError 测试公钥 Raw() 返回错误
func TestKeyBook_AddPubKey_RawError(t *testing.T) {
	kb := New()
	peerID := testPeerID("peer1")

	err := kb.AddPubKey(peerID, &errorPublicKey{})
	assert.Error(t, err)
}

// TestKeyBook_PrivKey 测试私钥操作
func TestKeyBook_PrivKey(t *testing.T) {
	kb := New()
	peerID := testPeerID("peer1")
	privKey := testPrivKey("priv1")

	// 添加私钥
	err := kb.AddPrivKey(peerID, privKey)
	assert.NoError(t, err)

	// 获取私钥
	retrieved, err := kb.PrivKey(peerID)
	assert.NoError(t, err)
	assert.True(t, privKey.Equals(retrieved))

	// 验证公钥也被添加
	pubKey, err := kb.PubKey(peerID)
	assert.NoError(t, err)
	assert.NotNil(t, pubKey)
}

// TestKeyBook_PrivKey_NotFound 测试获取不存在的私钥
func TestKeyBook_PrivKey_NotFound(t *testing.T) {
	kb := New()
	peerID := testPeerID("nonexistent")

	_, err := kb.PrivKey(peerID)
	assert.Error(t, err)
	assert.Equal(t, ErrNotFound, err)
}

// TestKeyBook_PeersWithKeys_Mixed 测试混合密钥的节点列表
func TestKeyBook_PeersWithKeys_Mixed(t *testing.T) {
	kb := New()

	peer1 := testPeerID("peer1")
	peer2 := testPeerID("peer2")
	peer3 := testPeerID("peer3")

	// peer1 只有公钥
	kb.AddPubKey(peer1, testPubKey("key1"))

	// peer2 只有私钥（会自动添加公钥）
	kb.AddPrivKey(peer2, testPrivKey("priv2"))

	// peer3 有公钥和私钥
	kb.AddPubKey(peer3, testPubKey("key3"))
	kb.AddPrivKey(peer3, testPrivKey("priv3"))

	peers := kb.PeersWithKeys()
	assert.Len(t, peers, 3)
	assert.Contains(t, peers, peer1)
	assert.Contains(t, peers, peer2)
	assert.Contains(t, peers, peer3)
}

// TestKeyBook_RemovePeer_WithPrivKey 测试移除有私钥的节点
func TestKeyBook_RemovePeer_WithPrivKey(t *testing.T) {
	kb := New()
	peerID := testPeerID("peer1")

	// 添加公钥和私钥
	kb.AddPubKey(peerID, testPubKey("key1"))
	kb.AddPrivKey(peerID, testPrivKey("priv1"))

	// 验证都存在
	_, err := kb.PubKey(peerID)
	assert.NoError(t, err)
	_, err = kb.PrivKey(peerID)
	assert.NoError(t, err)

	// 移除节点
	kb.RemovePeer(peerID)

	// 验证都已删除
	_, err = kb.PubKey(peerID)
	assert.Error(t, err)
	_, err = kb.PrivKey(peerID)
	assert.Error(t, err)
}

// TestKeyBook_UpdatePubKey 测试更新公钥
func TestKeyBook_UpdatePubKey(t *testing.T) {
	kb := New()
	peerID := testPeerID("peer1")

	// 添加第一个公钥
	key1 := testPubKey("key1")
	err := kb.AddPubKey(peerID, key1)
	assert.NoError(t, err)

	// 更新为新公钥
	key2 := testPubKey("key2")
	err = kb.AddPubKey(peerID, key2)
	assert.NoError(t, err)

	// 验证是新公钥
	retrieved, err := kb.PubKey(peerID)
	assert.NoError(t, err)
	assert.True(t, key2.Equals(retrieved))
}

// TestKeyBook_Concurrent 测试并发访问
func TestKeyBook_Concurrent(t *testing.T) {
	kb := New()
	done := make(chan bool)

	// 并发写入
	for i := 0; i < 10; i++ {
		go func(idx int) {
			peerID := testPeerID(fmt.Sprintf("peer%d", idx))
			kb.AddPubKey(peerID, testPubKey(fmt.Sprintf("key%d", idx)))
			kb.AddPrivKey(peerID, testPrivKey(fmt.Sprintf("priv%d", idx)))
			done <- true
		}(i)
	}

	// 并发读取
	for i := 0; i < 10; i++ {
		go func(idx int) {
			peerID := testPeerID(fmt.Sprintf("peer%d", idx))
			kb.PubKey(peerID)
			kb.PrivKey(peerID)
			kb.PeersWithKeys()
			done <- true
		}(i)
	}

	// 等待完成
	for i := 0; i < 20; i++ {
		<-done
	}

	// 验证节点数
	peers := kb.PeersWithKeys()
	assert.Len(t, peers, 10)
}

// TestKeyBook_Empty 测试空密钥簿
func TestKeyBook_Empty(t *testing.T) {
	kb := New()

	peers := kb.PeersWithKeys()
	assert.Empty(t, peers)
}

// TestKeyBook_RemovePeer_NonExistent 测试移除不存在的节点
func TestKeyBook_RemovePeer_NonExistent(t *testing.T) {
	kb := New()
	peerID := testPeerID("nonexistent")

	// 移除不存在的节点应该不报错
	kb.RemovePeer(peerID)

	// 验证没有影响
	peers := kb.PeersWithKeys()
	assert.Empty(t, peers)
}
