// Package keybook 实现密钥簿
package keybook

import (
	"errors"
	"sync"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

var (
	// ErrNotFound 未找到密钥
	ErrNotFound = errors.New("key not found")

	// ErrInvalidPublicKey 公钥无效
	ErrInvalidPublicKey = errors.New("invalid public key for peer")
)

// KeyBook 密钥簿
type KeyBook struct {
	mu sync.RWMutex

	// pubKeys 公钥映射
	pubKeys map[types.PeerID]pkgif.PublicKey

	// privKeys 私钥映射（仅本地节点）
	privKeys map[types.PeerID]pkgif.PrivateKey
}

// New 创建密钥簿
func New() *KeyBook {
	return &KeyBook{
		pubKeys:  make(map[types.PeerID]pkgif.PublicKey),
		privKeys: make(map[types.PeerID]pkgif.PrivateKey),
	}
}

// PubKey 获取公钥
//
// 获取流程：
//  1. 从缓存查找
//  2. 如果未找到，尝试从 PeerID 提取内嵌公钥（identity multihash）
//
// 注意：对于从 PeerID 提取的原始公钥字节，上层调用者需要自行反序列化
// 为 pkgif.PublicKey 接口类型后，再调用 AddPubKey 添加到缓存。
func (kb *KeyBook) PubKey(peerID types.PeerID) (pkgif.PublicKey, error) {
	kb.mu.RLock()
	pubKey, ok := kb.pubKeys[peerID]
	kb.mu.RUnlock()

	if ok {
		return pubKey, nil
	}

	// 尝试从 PeerID 中提取内嵌公钥（仅适用于 identity multihash 格式）
	// DeP2P 原生格式（Base58(SHA256(pubKey))）不包含内嵌公钥，会返回 ErrPeerIDNoEmbeddedKey
	// 注意：这里仅检查是否可以提取，但不进行反序列化
	// 因为 keybook 不依赖 crypto 包，反序列化应该由上层完成
	_, err := peerID.ExtractPublicKey()
	if err == nil {
		// PeerID 包含内嵌公钥（identity multihash 格式），但尚未添加到缓存
		// 返回 ErrNotFound，提示调用者可以提取
		return nil, ErrNotFound
	}

	return nil, ErrNotFound
}

// AddPubKey 添加公钥
//
// 验证流程：
//  1. 获取公钥字节表示
//  2. 验证 PeerID 格式是否有效
//  3. 如果 PeerID 格式有效，使用 MatchesPublicKey() 验证公钥与 PeerID 匹配
//  4. 存储公钥
//
// 支持的 PeerID 格式：
//   - DeP2P 原生格式：Base58(SHA256(pubKey)) - 通过重新计算哈希验证
//   - Multihash identity 格式：直接比较内嵌公钥
//
// 注意：对于非标准 PeerID 格式（如测试时使用的简单字符串），
// 会跳过验证以保持向后兼容。
func (kb *KeyBook) AddPubKey(peerID types.PeerID, pubKey pkgif.PublicKey) error {
	if pubKey == nil {
		return errors.New("public key is nil")
	}

	// 获取公钥字节表示
	pubKeyBytes, err := pubKey.Raw()
	if err != nil {
		return err
	}

	// 仅对有效的 PeerID 格式进行验证
	// 如果 PeerID 不是有效的 Multihash 格式，跳过验证（向后兼容）
	if peerID.Validate() == nil {
		// PeerID 是有效格式，验证公钥匹配
		if !peerID.MatchesPublicKey(pubKeyBytes) {
			return ErrInvalidPublicKey
		}
	}

	kb.mu.Lock()
	defer kb.mu.Unlock()

	kb.pubKeys[peerID] = pubKey
	return nil
}

// PrivKey 获取私钥
func (kb *KeyBook) PrivKey(peerID types.PeerID) (pkgif.PrivateKey, error) {
	kb.mu.RLock()
	defer kb.mu.RUnlock()

	privKey, ok := kb.privKeys[peerID]
	if !ok {
		return nil, ErrNotFound
	}

	return privKey, nil
}

// AddPrivKey 添加私钥
func (kb *KeyBook) AddPrivKey(peerID types.PeerID, privKey pkgif.PrivateKey) error {
	kb.mu.Lock()
	defer kb.mu.Unlock()

	kb.privKeys[peerID] = privKey

	// 同时存储对应的公钥
	pubKey := privKey.PublicKey()
	kb.pubKeys[peerID] = pubKey

	return nil
}

// PeersWithKeys 返回拥有密钥的节点列表
func (kb *KeyBook) PeersWithKeys() []types.PeerID {
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
func (kb *KeyBook) RemovePeer(peerID types.PeerID) {
	kb.mu.Lock()
	defer kb.mu.Unlock()

	delete(kb.pubKeys, peerID)
	delete(kb.privKeys, peerID)
}
