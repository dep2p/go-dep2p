// Package keybook 实现密钥簿
package keybook

import (
	"sync"

	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dep2p/go-dep2p/internal/core/storage/kv"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 存储前缀
const (
	pubKeyPrefix  = "pub/"  // 公钥前缀
	privKeyPrefix = "priv/" // 私钥前缀
)

// KeyDeserializer 密钥反序列化器接口
//
// 由于 keybook 不应该依赖 crypto 包，
// 需要外部提供密钥反序列化方法。
type KeyDeserializer interface {
	// DeserializePublicKey 反序列化公钥
	DeserializePublicKey(data []byte) (pkgif.PublicKey, error)
	// DeserializePrivateKey 反序列化私钥
	DeserializePrivateKey(data []byte) (pkgif.PrivateKey, error)
}

// PersistentKeyBook 持久化密钥簿
//
// 使用 BadgerDB 存储密钥数据，同时保留内存缓存以支持快速查询。
type PersistentKeyBook struct {
	mu sync.RWMutex

	// store KV 存储（前缀 p/k/）
	store *kv.Store

	// deserializer 密钥反序列化器
	deserializer KeyDeserializer

	// 内存缓存
	pubKeys  map[types.PeerID]pkgif.PublicKey
	privKeys map[types.PeerID]pkgif.PrivateKey
}

// NewPersistent 创建持久化密钥簿
//
// 参数:
//   - store: KV 存储实例（已带前缀 p/k/）
//   - deserializer: 密钥反序列化器（可选，为 nil 时不会从存储加载）
func NewPersistent(store *kv.Store, deserializer KeyDeserializer) (*PersistentKeyBook, error) {
	kb := &PersistentKeyBook{
		store:        store,
		deserializer: deserializer,
		pubKeys:      make(map[types.PeerID]pkgif.PublicKey),
		privKeys:     make(map[types.PeerID]pkgif.PrivateKey),
	}

	// 如果有反序列化器，从存储加载已有数据
	if deserializer != nil {
		if err := kb.loadFromStore(); err != nil {
			return nil, err
		}
	}

	return kb, nil
}

// loadFromStore 从存储加载密钥数据
func (kb *PersistentKeyBook) loadFromStore() error {
	// 加载公钥
	err := kb.store.PrefixScan([]byte(pubKeyPrefix), func(key, value []byte) bool {
		peerID := types.PeerID(key[len(pubKeyPrefix):])

		pubKey, err := kb.deserializer.DeserializePublicKey(value)
		if err != nil {
			// 跳过无法反序列化的数据
			return true
		}

		kb.pubKeys[peerID] = pubKey
		return true
	})
	if err != nil {
		return err
	}

	// 加载私钥
	err = kb.store.PrefixScan([]byte(privKeyPrefix), func(key, value []byte) bool {
		peerID := types.PeerID(key[len(privKeyPrefix):])

		privKey, err := kb.deserializer.DeserializePrivateKey(value)
		if err != nil {
			// 跳过无法反序列化的数据
			return true
		}

		kb.privKeys[peerID] = privKey
		// 同时添加公钥到缓存
		kb.pubKeys[peerID] = privKey.PublicKey()
		return true
	})

	return err
}

// PubKey 获取公钥
func (kb *PersistentKeyBook) PubKey(peerID types.PeerID) (pkgif.PublicKey, error) {
	kb.mu.RLock()
	pubKey, ok := kb.pubKeys[peerID]
	kb.mu.RUnlock()

	if ok {
		return pubKey, nil
	}

	// 尝试从存储加载
	data, err := kb.store.Get([]byte(pubKeyPrefix + string(peerID)))
	if err != nil {
		if engine.IsNotFound(err) {
			// 尝试从 PeerID 中提取内嵌公钥（仅 identity multihash 格式支持）
			// DeP2P 原生格式不包含内嵌公钥，会返回 ErrPeerIDNoEmbeddedKey
			_, err := peerID.ExtractPublicKey()
			if err == nil {
				return nil, ErrNotFound
			}
			return nil, ErrNotFound
		}
		return nil, err
	}

	// 反序列化并缓存
	if kb.deserializer != nil {
		pubKey, err = kb.deserializer.DeserializePublicKey(data)
		if err != nil {
			return nil, err
		}

		kb.mu.Lock()
		kb.pubKeys[peerID] = pubKey
		kb.mu.Unlock()

		return pubKey, nil
	}

	return nil, ErrNotFound
}

// AddPubKey 添加公钥
func (kb *PersistentKeyBook) AddPubKey(peerID types.PeerID, pubKey pkgif.PublicKey) error {
	if pubKey == nil {
		return ErrInvalidPublicKey
	}

	// 获取公钥字节表示
	pubKeyBytes, err := pubKey.Raw()
	if err != nil {
		return err
	}

	// 仅对有效的 PeerID 格式进行验证
	if peerID.Validate() == nil {
		if !peerID.MatchesPublicKey(pubKeyBytes) {
			return ErrInvalidPublicKey
		}
	}

	kb.mu.Lock()
	kb.pubKeys[peerID] = pubKey
	kb.mu.Unlock()

	// 持久化
	return kb.store.Put([]byte(pubKeyPrefix+string(peerID)), pubKeyBytes)
}

// PrivKey 获取私钥
func (kb *PersistentKeyBook) PrivKey(peerID types.PeerID) (pkgif.PrivateKey, error) {
	kb.mu.RLock()
	privKey, ok := kb.privKeys[peerID]
	kb.mu.RUnlock()

	if ok {
		return privKey, nil
	}

	// 尝试从存储加载
	data, err := kb.store.Get([]byte(privKeyPrefix + string(peerID)))
	if err != nil {
		if engine.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	// 反序列化并缓存
	if kb.deserializer != nil {
		privKey, err = kb.deserializer.DeserializePrivateKey(data)
		if err != nil {
			return nil, err
		}

		kb.mu.Lock()
		kb.privKeys[peerID] = privKey
		kb.pubKeys[peerID] = privKey.PublicKey()
		kb.mu.Unlock()

		return privKey, nil
	}

	return nil, ErrNotFound
}

// AddPrivKey 添加私钥
func (kb *PersistentKeyBook) AddPrivKey(peerID types.PeerID, privKey pkgif.PrivateKey) error {
	if privKey == nil {
		return ErrNotFound
	}

	// 获取私钥字节表示
	privKeyBytes, err := privKey.Raw()
	if err != nil {
		return err
	}

	// BUG FIX #B5: 检查公钥是否为 nil，避免 nil pointer dereference
	pubKey := privKey.PublicKey()
	if pubKey == nil {
		return ErrInvalidPublicKey
	}

	kb.mu.Lock()
	kb.privKeys[peerID] = privKey
	kb.pubKeys[peerID] = pubKey
	kb.mu.Unlock()

	// 持久化私钥
	if err := kb.store.Put([]byte(privKeyPrefix+string(peerID)), privKeyBytes); err != nil {
		return err
	}

	// 持久化公钥 (已安全检查过 pubKey 不为 nil)
	pubKeyBytes, err := pubKey.Raw()
	if err != nil {
		return err
	}

	return kb.store.Put([]byte(pubKeyPrefix+string(peerID)), pubKeyBytes)
}

// PeersWithKeys 返回拥有密钥的节点列表
func (kb *PersistentKeyBook) PeersWithKeys() []types.PeerID {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	peerSet := make(map[types.PeerID]struct{})

	for peerID := range kb.pubKeys {
		peerSet[peerID] = struct{}{}
	}
	for peerID := range kb.privKeys {
		peerSet[peerID] = struct{}{}
	}

	peers := make([]types.PeerID, 0, len(peerSet))
	for peerID := range peerSet {
		peers = append(peers, peerID)
	}

	return peers
}

// RemovePeer 移除节点密钥
func (kb *PersistentKeyBook) RemovePeer(peerID types.PeerID) {
	kb.mu.Lock()
	delete(kb.pubKeys, peerID)
	delete(kb.privKeys, peerID)
	kb.mu.Unlock()

	// 从存储中删除
	kb.store.Delete([]byte(pubKeyPrefix + string(peerID)))
	kb.store.Delete([]byte(privKeyPrefix + string(peerID)))
}

// Ensure PersistentKeyBook implements the same interface as KeyBook
var _ interface {
	PubKey(types.PeerID) (pkgif.PublicKey, error)
	AddPubKey(types.PeerID, pkgif.PublicKey) error
	PrivKey(types.PeerID) (pkgif.PrivateKey, error)
	AddPrivKey(types.PeerID, pkgif.PrivateKey) error
	PeersWithKeys() []types.PeerID
	RemovePeer(types.PeerID)
} = (*PersistentKeyBook)(nil)
