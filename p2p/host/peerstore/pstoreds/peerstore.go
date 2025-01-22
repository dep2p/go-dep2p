package pstoreds

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/peerstore"
	pstore "github.com/dep2p/go-dep2p/p2p/host/peerstore"

	ds "github.com/dep2p/go-dep2p/datastore"
	"github.com/dep2p/go-dep2p/datastore/query"
	"github.com/dep2p/go-dep2p/multiformats/base32"
)

// Options 是 peerstore 的配置对象
type Options struct {
	// 内存缓存的大小。值为 0 或更小时禁用缓存
	CacheSize uint

	// MaxProtocols 是每个对等节点存储的最大协议数
	MaxProtocols int

	// 从数据存储中清除过期地址的扫描间隔。如果为零值，GC 不会自动运行，但可以通过显式调用触发
	GCPurgeInterval time.Duration

	// 更新 GC 预读窗口的间隔。如果为零值，预读将被禁用，每个清除周期都会遍历整个数据存储
	GCLookaheadInterval time.Duration

	// GC 进程启动前的初始延迟。目的是在启动 GC 前给系统充分的启动时间
	GCInitialDelay time.Duration

	// 时钟接口
	Clock clock
}

// DefaultOpts 返回持久化 peerstore 的默认选项，使用完整清除 GC 算法:
//
// * 缓存大小: 1024
// * 最大协议数: 1024
// * GC 清除间隔: 2小时
// * GC 预读间隔: 禁用
// * GC 初始延迟: 60秒
func DefaultOpts() Options {
	return Options{
		CacheSize:           1024,
		MaxProtocols:        1024,
		GCPurgeInterval:     2 * time.Hour,
		GCLookaheadInterval: 0,
		GCInitialDelay:      60 * time.Second,
		Clock:               realclock{},
	}
}

// pstoreds 实现了基于数据存储的 peerstore
type pstoreds struct {
	// 内嵌 peerstore.Metrics 接口
	peerstore.Metrics

	// 密钥簿
	*dsKeyBook
	// 地址簿
	*dsAddrBook
	// 协议簿
	*dsProtoBook
	// 对等节点元数据
	*dsPeerMetadata
}

// 确保 pstoreds 实现了 peerstore.Peerstore 接口
var _ peerstore.Peerstore = &pstoreds{}

// NewPeerstore 创建一个由持久化数据存储支持的 peerstore。
// 调用者需要负责调用 RemovePeer 以确保 peerstore 的内存消耗不会无限增长。
//
// 参数:
//   - ctx: 上下文对象
//   - store: 支持批处理的数据存储
//   - opts: 配置选项
//
// 返回值:
//   - *pstoreds: peerstore 实例
//   - error: 错误信息
func NewPeerstore(ctx context.Context, store ds.Batching, opts Options) (*pstoreds, error) {
	// 创建地址簿
	addrBook, err := NewAddrBook(ctx, store, opts)
	if err != nil {
		log.Debugf("创建地址簿失败: %v", err)
		return nil, err
	}

	// 创建密钥簿
	keyBook, err := NewKeyBook(ctx, store, opts)
	if err != nil {
		log.Debugf("创建密钥簿失败: %v", err)
		return nil, err
	}

	// 创建对等节点元数据
	peerMetadata, err := NewPeerMetadata(ctx, store, opts)
	if err != nil {
		log.Debugf("创建对等节点元数据失败: %v", err)
		return nil, err
	}

	// 创建协议簿
	protoBook, err := NewProtoBook(peerMetadata, WithMaxProtocols(opts.MaxProtocols))
	if err != nil {
		log.Debugf("创建协议簿失败: %v", err)
		return nil, err
	}

	// 返回完整的 peerstore 实例
	return &pstoreds{
		Metrics:        pstore.NewMetrics(),
		dsKeyBook:      keyBook,
		dsAddrBook:     addrBook,
		dsPeerMetadata: peerMetadata,
		dsProtoBook:    protoBook,
	}, nil
}

// uniquePeerIds 从数据库键中提取并返回唯一的对等节点 ID。
//
// 参数:
//   - ds: 数据存储接口
//   - prefix: 键前缀
//   - extractor: 从查询结果提取 ID 的函数
//
// 返回值:
//   - peer.IDSlice: 对等节点 ID 切片
//   - error: 错误信息
func uniquePeerIds(ds ds.Datastore, prefix ds.Key, extractor func(result query.Result) string) (peer.IDSlice, error) {
	var (
		// 构造只查询键的查询对象
		q       = query.Query{Prefix: prefix.String(), KeysOnly: true}
		results query.Results
		err     error
	)

	// 执行查询
	if results, err = ds.Query(context.TODO(), q); err != nil {
		log.Debugf("查询数据存储失败: %v", err)
		return nil, err
	}

	// 确保关闭查询结果
	defer results.Close()

	// 使用 map 去重
	idset := make(map[string]struct{})
	for result := range results.Next() {
		k := extractor(result)
		idset[k] = struct{}{}
	}

	// 如果没有结果返回空切片
	if len(idset) == 0 {
		return peer.IDSlice{}, nil
	}

	// 转换为 peer.ID 切片
	ids := make(peer.IDSlice, 0, len(idset))
	for id := range idset {
		pid, _ := base32.RawStdEncoding.DecodeString(id)
		id, _ := peer.IDFromBytes(pid)
		ids = append(ids, id)
	}
	return ids, nil
}

// Close 关闭 peerstore 及其所有组件。
//
// 返回值:
//   - error: 错误信息
func (ps *pstoreds) Close() (err error) {
	var errs []error
	// 定义弱关闭函数
	weakClose := func(name string, c interface{}) {
		if cl, ok := c.(io.Closer); ok {
			if err = cl.Close(); err != nil {
				errs = append(errs, fmt.Errorf("%s error: %s", name, err))
			}
		}
	}
	// 关闭各个组件
	weakClose("keybook", ps.dsKeyBook)
	weakClose("addressbook", ps.dsAddrBook)
	weakClose("protobook", ps.dsProtoBook)
	weakClose("peermetadata", ps.dsPeerMetadata)

	// 如果有错误则返回组合的错误信息
	if len(errs) > 0 {
		log.Debugf("关闭peerstore失败: %v", errs)
		return fmt.Errorf("关闭peerstore失败: %v", errs)
	}
	return nil
}

// Peers 返回所有已知的对等节点 ID。
//
// 返回值:
//   - peer.IDSlice: 对等节点 ID 切片
func (ps *pstoreds) Peers() peer.IDSlice {
	// 使用 map 去重
	set := map[peer.ID]struct{}{}
	// 添加有密钥的对等节点
	for _, p := range ps.PeersWithKeys() {
		set[p] = struct{}{}
	}
	// 添加有地址的对等节点
	for _, p := range ps.PeersWithAddrs() {
		set[p] = struct{}{}
	}

	// 转换为切片
	pps := make(peer.IDSlice, 0, len(set))
	for p := range set {
		pps = append(pps, p)
	}
	return pps
}

// PeerInfo 返回指定对等节点的信息。
//
// 参数:
//   - p: 对等节点 ID
//
// 返回值:
//   - peer.AddrInfo: 对等节点信息
func (ps *pstoreds) PeerInfo(p peer.ID) peer.AddrInfo {
	return peer.AddrInfo{
		ID:    p,
		Addrs: ps.dsAddrBook.Addrs(p),
	}
}

// RemovePeer 从以下组件中移除与对等节点相关的条目:
// * 密钥簿
// * 协议簿
// * 对等节点元数据
// * 指标
// 注意: 不会从地址簿中移除对等节点。
//
// 参数:
//   - p: 要移除的对等节点 ID
func (ps *pstoreds) RemovePeer(p peer.ID) {
	ps.dsKeyBook.RemovePeer(p)
	ps.dsProtoBook.RemovePeer(p)
	ps.dsPeerMetadata.RemovePeer(p)
	ps.Metrics.RemovePeer(p)
}
