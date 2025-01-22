package pstoremem

import (
	"errors"
	"sync"

	ic "github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/peer"
	pstore "github.com/dep2p/go-dep2p/core/peerstore"
)

// memoryKeyBook 内存密钥簿实现
type memoryKeyBook struct {
	sync.RWMutex                        // 读写锁,用于并发控制
	pks          map[peer.ID]ic.PubKey  // 存储公钥映射
	sks          map[peer.ID]ic.PrivKey // 存储私钥映射
}

// 确保 memoryKeyBook 实现了 KeyBook 接口
var _ pstore.KeyBook = (*memoryKeyBook)(nil)

// NewKeyBook 创建新的内存密钥簿
// 返回:
//   - *memoryKeyBook: 内存密钥簿实例
func NewKeyBook() *memoryKeyBook {
	return &memoryKeyBook{
		pks: map[peer.ID]ic.PubKey{},  // 初始化公钥映射
		sks: map[peer.ID]ic.PrivKey{}, // 初始化私钥映射
	}
}

// PeersWithKeys 获取所有有密钥的对等节点
// 返回:
//   - peer.IDSlice: 对等节点ID列表
func (mkb *memoryKeyBook) PeersWithKeys() peer.IDSlice {
	mkb.RLock()                                            // 加读锁
	ps := make(peer.IDSlice, 0, len(mkb.pks)+len(mkb.sks)) // 创建切片
	for p := range mkb.pks {                               // 遍历所有公钥
		ps = append(ps, p) // 添加对等节点ID
	}
	for p := range mkb.sks { // 遍历所有私钥
		if _, found := mkb.pks[p]; !found { // 如果没有对应的公钥
			ps = append(ps, p) // 添加对等节点ID
		}
	}
	mkb.RUnlock() // 解读锁
	return ps
}

// PubKey 获取对等节点的公钥
// 参数:
//   - p: 对等节点ID
//
// 返回:
//   - ic.PubKey: 公钥
func (mkb *memoryKeyBook) PubKey(p peer.ID) ic.PubKey {
	mkb.RLock()      // 加读锁
	pk := mkb.pks[p] // 获取公钥
	mkb.RUnlock()    // 解读锁
	if pk != nil {   // 如果找到公钥
		return pk // 返回公钥
	}
	pk, err := p.ExtractPublicKey() // 从ID提取公钥
	if err == nil {                 // 如果提取成功
		mkb.Lock()      // 加写锁
		mkb.pks[p] = pk // 保存公钥
		mkb.Unlock()    // 解写锁
	}
	return pk // 返回公钥
}

// AddPubKey 添加对等节点的公钥
// 参数:
//   - p: 对等节点ID
//   - pk: 公钥
//
// 返回:
//   - error: 错误信息
func (mkb *memoryKeyBook) AddPubKey(p peer.ID, pk ic.PubKey) error {
	if !p.MatchesPublicKey(pk) { // 验证ID与公钥是否匹配
		log.Debugf("ID与公钥不匹配: %v", p)
		return errors.New("ID与公钥不匹配")
	}

	mkb.Lock()      // 加写锁
	mkb.pks[p] = pk // 保存公钥
	mkb.Unlock()    // 解写锁
	return nil
}

// PrivKey 获取对等节点的私钥
// 参数:
//   - p: 对等节点ID
//
// 返回:
//   - ic.PrivKey: 私钥
func (mkb *memoryKeyBook) PrivKey(p peer.ID) ic.PrivKey {
	mkb.RLock()         // 加读锁
	defer mkb.RUnlock() // 延迟解读锁
	return mkb.sks[p]   // 返回私钥
}

// AddPrivKey 添加对等节点的私钥
// 参数:
//   - p: 对等节点ID
//   - sk: 私钥
//
// 返回:
//   - error: 错误信息
func (mkb *memoryKeyBook) AddPrivKey(p peer.ID, sk ic.PrivKey) error {
	if sk == nil { // 检查私钥是否为空
		log.Debugf("私钥为空: %v", p)
		return errors.New("私钥为空")
	}

	if !p.MatchesPrivateKey(sk) { // 验证ID与私钥是否匹配
		log.Debugf("ID与私钥不匹配: %v", p)
		return errors.New("ID与私钥不匹配")
	}

	mkb.Lock()      // 加写锁
	mkb.sks[p] = sk // 保存私钥
	mkb.Unlock()    // 解写锁
	return nil
}

// RemovePeer 移除对等节点的所有密钥
// 参数:
//   - p: 对等节点ID
func (mkb *memoryKeyBook) RemovePeer(p peer.ID) {
	mkb.Lock()         // 加写锁
	delete(mkb.sks, p) // 删除私钥
	delete(mkb.pks, p) // 删除公钥
	mkb.Unlock()       // 解写锁
}
