package pstoreds

import (
	"bytes"
	"context"
	"encoding/gob"

	"github.com/dep2p/go-dep2p/core/peer"
	pstore "github.com/dep2p/go-dep2p/core/peerstore"
	"github.com/dep2p/go-dep2p/core/protocol"
	pool "github.com/dep2p/go-dep2p/p2plib/buffer/pool"

	ds "github.com/dep2p/go-dep2p/datastore"
	"github.com/dep2p/go-dep2p/datastore/query"
	"github.com/dep2p/go-dep2p/multiformats/base32"
)

// 元数据存储在以下数据库键模式下:
// /peers/metadata/<b32 peer id no padding>/<key>
var pmBase = ds.NewKey("/peers/metadata")

// dsPeerMetadata 结构体用于存储基于数据存储的对等节点元数据
type dsPeerMetadata struct {
	ds ds.Datastore // 数据存储实例
}

// 确保 dsPeerMetadata 实现了 pstore.PeerMetadata 接口
var _ pstore.PeerMetadata = (*dsPeerMetadata)(nil)

func init() {
	// Gob 默认注册基本类型
	//
	// 注册 peerstore 本身使用的复杂类型
	gob.Register(make(map[protocol.ID]struct{}))
}

// NewPeerMetadata 创建一个由持久化数据库支持的元数据存储。它使用 gob 进行序列化。
//
// 参数:
//   - ctx: 上下文对象
//   - store: 数据存储实例
//   - opts: 配置选项
//
// 返回值:
//   - *dsPeerMetadata: 元数据存储实例
//   - error: 错误信息
//
// 查看 `init()` 函数了解默认注册的类型。
// 想要存储其他类型值的模块需要显式调用 `gob.Register()`， 否则调用者将收到运行时错误。
func NewPeerMetadata(_ context.Context, store ds.Datastore, _ Options) (*dsPeerMetadata, error) {
	return &dsPeerMetadata{store}, nil
}

// Get 获取指定对等节点的指定键的元数据值
//
// 参数:
//   - p: 对等节点ID
//   - key: 元数据键名
//
// 返回值:
//   - interface{}: 元数据值
//   - error: 错误信息
func (pm *dsPeerMetadata) Get(p peer.ID, key string) (interface{}, error) {
	// 构造完整的数据存储键
	k := pmBase.ChildString(base32.RawStdEncoding.EncodeToString([]byte(p))).ChildString(key)
	// 从数据存储中获取值
	value, err := pm.ds.Get(context.TODO(), k)
	if err != nil {
		if err == ds.ErrNotFound {
			err = pstore.ErrNotFound
		}
		log.Debugf("获取元数据失败: %v", err)
		return nil, err
	}

	// 解码存储的值
	var res interface{}
	if err := gob.NewDecoder(bytes.NewReader(value)).Decode(&res); err != nil {
		log.Debugf("解码元数据失败: %v", err)
		return nil, err
	}
	return res, nil
}

// Put 为指定对等节点设置指定键的元数据值
//
// 参数:
//   - p: 对等节点ID
//   - key: 元数据键名
//   - val: 要存储的值
//
// 返回值:
//   - error: 错误信息
func (pm *dsPeerMetadata) Put(p peer.ID, key string, val interface{}) error {
	// 构造完整的数据存储键
	k := pmBase.ChildString(base32.RawStdEncoding.EncodeToString([]byte(p))).ChildString(key)
	// 使用缓冲池获取缓冲区
	var buf pool.Buffer
	// 编码值
	if err := gob.NewEncoder(&buf).Encode(&val); err != nil {
		log.Debugf("编码元数据失败: %v", err)
		return err
	}
	// 存储到数据存储中
	return pm.ds.Put(context.TODO(), k, buf.Bytes())
}

// RemovePeer 移除指定对等节点的所有元数据
//
// 参数:
//   - p: 要移除的对等节点ID
func (pm *dsPeerMetadata) RemovePeer(p peer.ID) {
	// 查询该对等节点的所有元数据键
	result, err := pm.ds.Query(context.TODO(), query.Query{
		Prefix:   pmBase.ChildString(base32.RawStdEncoding.EncodeToString([]byte(p))).String(),
		KeysOnly: true,
	})
	if err != nil {
		log.Debugf("查询数据存储以移除对等节点时失败: %v", err)
		return
	}
	// 删除所有找到的键
	for entry := range result.Next() {
		pm.ds.Delete(context.TODO(), ds.NewKey(entry.Key))
	}
}
