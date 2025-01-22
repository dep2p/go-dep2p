package pstoreds

import (
	"context"
	"errors"

	ic "github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/peer"
	pstore "github.com/dep2p/go-dep2p/core/peerstore"

	ds "github.com/dep2p/go-dep2p/datastore"
	"github.com/dep2p/go-dep2p/datastore/query"
	"github.com/dep2p/go-dep2p/multiformats/base32"
)

// 公钥和私钥存储在以下数据库键模式下:
// /peers/keys/<b32 peer id 无填充>/{pub, priv}
var (
	kbBase     = ds.NewKey("/peers/keys") // 密钥存储的基础路径
	pubSuffix  = ds.NewKey("/pub")        // 公钥后缀
	privSuffix = ds.NewKey("/priv")       // 私钥后缀
)

// dsKeyBook 实现基于数据存储的密钥簿
type dsKeyBook struct {
	ds ds.Datastore // 数据存储接口
}

// 确保 dsKeyBook 实现了 pstore.KeyBook 接口
var _ pstore.KeyBook = (*dsKeyBook)(nil)

// NewKeyBook 创建一个新的基于数据存储的密钥簿
//
// 参数:
//   - ctx: context.Context 上下文
//   - store: ds.Datastore 数据存储接口
//   - opts: Options 配置选项
//
// 返回:
//   - *dsKeyBook 密钥簿实例
//   - error 错误信息
func NewKeyBook(_ context.Context, store ds.Datastore, _ Options) (*dsKeyBook, error) {
	return &dsKeyBook{store}, nil
}

// PubKey 获取peer的公钥
//
// 参数:
//   - p: peer.ID peer标识
//
// 返回:
//   - ic.PubKey 公钥
func (kb *dsKeyBook) PubKey(p peer.ID) ic.PubKey {
	// 构造存储键
	key := peerToKey(p, pubSuffix)

	var pk ic.PubKey
	// 尝试从数据存储获取
	if value, err := kb.ds.Get(context.TODO(), key); err == nil {
		// 反序列化公钥
		pk, err = ic.UnmarshalPublicKey(value)
		if err != nil {
			log.Errorf("从数据存储反序列化peer %s的公钥时出错: %s\n", p, err)
		}
	} else if err == ds.ErrNotFound {
		// 如果不存在,尝试从peer ID提取
		pk, err = p.ExtractPublicKey()
		switch err {
		case nil:
		case peer.ErrNoPublicKey:
			return nil
		default:
			log.Errorf("从peer ID提取peer %s的公钥时出错: %s\n", p, err)
			return nil
		}
		// 序列化公钥
		pkb, err := ic.MarshalPublicKey(pk)
		if err != nil {
			log.Errorf("序列化peer %s的提取公钥时出错: %s\n", p, err)
			return nil
		}
		// 存储到数据存储
		if err := kb.ds.Put(context.TODO(), key, pkb); err != nil {
			log.Errorf("将peer %s的提取公钥添加到peerstore时出错: %s\n", p, err)
			return nil
		}
	} else {
		log.Errorf("从数据存储获取peer %s的公钥时出错: %s\n", p, err)
	}

	return pk
}

// AddPubKey 添加peer的公钥
//
// 参数:
//   - p: peer.ID peer标识
//   - pk: ic.PubKey 公钥
//
// 返回:
//   - error 错误信息
func (kb *dsKeyBook) AddPubKey(p peer.ID, pk ic.PubKey) error {
	// 检查公钥是否匹配peer ID
	if !p.MatchesPublicKey(pk) {
		log.Debugf("peer ID与公钥不匹配: %v", p)
		return errors.New("peer ID与公钥不匹配")
	}

	// 序列化公钥
	val, err := ic.MarshalPublicKey(pk)
	if err != nil {
		log.Errorf("序列化peer %s的公钥时出错: %s\n", p, err)
		return err
	}
	// 存储到数据存储
	if err := kb.ds.Put(context.TODO(), peerToKey(p, pubSuffix), val); err != nil {
		log.Errorf("更新peer %s的公钥到数据存储时出错: %s\n", p, err)
		return err
	}
	return nil
}

// PrivKey 获取peer的私钥
//
// 参数:
//   - p: peer.ID peer标识
//
// 返回:
//   - ic.PrivKey 私钥
func (kb *dsKeyBook) PrivKey(p peer.ID) ic.PrivKey {
	// 从数据存储获取
	value, err := kb.ds.Get(context.TODO(), peerToKey(p, privSuffix))
	if err != nil {
		return nil
	}
	// 反序列化私钥
	sk, err := ic.UnmarshalPrivateKey(value)
	if err != nil {
		return nil
	}
	return sk
}

// AddPrivKey 添加peer的私钥
//
// 参数:
//   - p: peer.ID peer标识
//   - sk: ic.PrivKey 私钥
//
// 返回:
//   - error 错误信息
func (kb *dsKeyBook) AddPrivKey(p peer.ID, sk ic.PrivKey) error {
	// 检查私钥是否为空
	if sk == nil {
		log.Debugf("私钥为空: %v", p)
		return errors.New("私钥为空")
	}
	// 检查私钥是否匹配peer ID
	if !p.MatchesPrivateKey(sk) {
		log.Debugf("peer ID与私钥不匹配: %v", p)
		return errors.New("peer ID与私钥不匹配")
	}

	// 序列化私钥
	val, err := ic.MarshalPrivateKey(sk)
	if err != nil {
		log.Errorf("序列化peer %s的私钥时出错: %s\n", p, err)
		return err
	}
	// 存储到数据存储
	if err := kb.ds.Put(context.TODO(), peerToKey(p, privSuffix), val); err != nil {
		log.Errorf("更新peer %s的私钥到数据存储时出错: %s\n", p, err)
	}
	return err
}

// PeersWithKeys 返回所有有密钥的peer ID列表
//
// 返回:
//   - peer.IDSlice peer ID切片
func (kb *dsKeyBook) PeersWithKeys() peer.IDSlice {
	// 从数据存储获取唯一的peer ID
	ids, err := uniquePeerIds(kb.ds, kbBase, func(result query.Result) string {
		log.Debugf("获取有密钥的peer: %v", result.Key)
		return ds.RawKey(result.Key).Parent().Name()
	})
	if err != nil {
		log.Errorf("获取有密钥的peer时出错: %v", err)
	}
	return ids
}

// RemovePeer 移除peer的所有密钥
//
// 参数:
//   - p: peer.ID peer标识
func (kb *dsKeyBook) RemovePeer(p peer.ID) {
	// 删除私钥和公钥
	kb.ds.Delete(context.TODO(), peerToKey(p, privSuffix))
	kb.ds.Delete(context.TODO(), peerToKey(p, pubSuffix))
}

// peerToKey 将peer ID转换为存储键
//
// 参数:
//   - p: peer.ID peer标识
//   - suffix: ds.Key 后缀
//
// 返回:
//   - ds.Key 存储键
func peerToKey(p peer.ID, suffix ds.Key) ds.Key {
	return kbBase.ChildString(base32.RawStdEncoding.EncodeToString([]byte(p))).Child(suffix)
}
