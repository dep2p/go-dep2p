package pstoremem

import (
	"fmt"
	"io"

	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/peerstore"
	pstore "github.com/dep2p/go-dep2p/p2p/host/peerstore"
)

// pstoremem 内存对等节点存储实现
type pstoremem struct {
	peerstore.Metrics // 对等节点存储指标

	*memoryKeyBook      // 内存密钥簿
	*memoryAddrBook     // 内存地址簿
	*memoryProtoBook    // 内存协议簿
	*memoryPeerMetadata // 内存元数据
}

// 确保 pstoremem 实现了 Peerstore 接口
var _ peerstore.Peerstore = &pstoremem{}

// Option 定义对等节点存储选项接口
type Option interface{}

// NewPeerstore 创建一个内存中的线程安全的对等节点集合
// NewPeerstore 创建一个内存中的线程安全的对等节点集合。
// 调用者需要负责调用 RemovePeer 以确保对等节点存储的内存消耗不会无限增长。
// 参数:
//   - opts: 可选的配置选项
//
// 返回:
//   - *pstoremem: 内存对等节点存储实例
//   - error: 错误信息
func NewPeerstore(opts ...Option) (ps *pstoremem, err error) {
	var protoBookOpts []ProtoBookOption // 协议簿选项列表
	var addrBookOpts []AddrBookOption   // 地址簿选项列表
	for _, opt := range opts {          // 遍历选项
		switch o := opt.(type) { // 判断选项类型
		case ProtoBookOption: // 如果是协议簿选项
			protoBookOpts = append(protoBookOpts, o) // 添加到协议簿选项列表
		case AddrBookOption: // 如果是地址簿选项
			addrBookOpts = append(addrBookOpts, o) // 添加到地址簿选项列表
		default: // 其他类型
			log.Debugf("意外的对等节点存储选项: %v", o)
			return nil, fmt.Errorf("意外的对等节点存储选项: %v", o) // 返回错误
		}
	}
	ab := NewAddrBook(addrBookOpts...) // 创建地址簿

	pb, err := NewProtoBook(protoBookOpts...) // 创建协议簿
	if err != nil {                           // 如果出错
		ab.Close() // 关闭地址簿
		log.Debugf("创建协议簿失败: %v", err)
		return nil, err // 返回错误
	}

	return &pstoremem{ // 返回内存对等节点存储实例
		Metrics:            pstore.NewMetrics(), // 创建指标
		memoryKeyBook:      NewKeyBook(),        // 创建密钥簿
		memoryAddrBook:     ab,                  // 设置地址簿
		memoryProtoBook:    pb,                  // 设置协议簿
		memoryPeerMetadata: NewPeerMetadata(),   // 创建元数据
	}, nil
}

// Close 关闭对等节点存储
// 返回:
//   - error: 错误信息
func (ps *pstoremem) Close() (err error) {
	var errs []error                                // 错误列表
	weakClose := func(name string, c interface{}) { // 弱关闭函数
		if cl, ok := c.(io.Closer); ok { // 如果实现了 Closer 接口
			if err = cl.Close(); err != nil { // 关闭并检查错误
				errs = append(errs, fmt.Errorf("%s 错误: %s", name, err)) // 添加错误信息
			}
		}
	}
	weakClose("keybook", ps.memoryKeyBook)           // 关闭密钥簿
	weakClose("addressbook", ps.memoryAddrBook)      // 关闭地址簿
	weakClose("protobook", ps.memoryProtoBook)       // 关闭协议簿
	weakClose("peermetadata", ps.memoryPeerMetadata) // 关闭元数据

	if len(errs) > 0 { // 如果有错误
		log.Debugf("关闭对等节点存储失败: %v", errs)            // 返回错误信息
		return fmt.Errorf("关闭对等节点存储失败; 错误: %q", errs) // 返回错误信息
	}
	return nil
}

// Peers 返回所有已知的对等节点ID
// 返回:
//   - peer.IDSlice: 对等节点ID列表
func (ps *pstoremem) Peers() peer.IDSlice {
	set := map[peer.ID]struct{}{}          // 创建集合
	for _, p := range ps.PeersWithKeys() { // 遍历有密钥的节点
		set[p] = struct{}{} // 添加到集合
	}
	for _, p := range ps.PeersWithAddrs() { // 遍历有地址的节点
		set[p] = struct{}{} // 添加到集合
	}

	pps := make(peer.IDSlice, 0, len(set)) // 创建切片
	for p := range set {                   // 遍历集合
		pps = append(pps, p) // 添加到切片
	}
	return pps
}

// PeerInfo 返回指定对等节点的信息
// 参数:
//   - p: 对等节点ID
//
// 返回:
//   - peer.AddrInfo: 对等节点信息
func (ps *pstoremem) PeerInfo(p peer.ID) peer.AddrInfo {
	return peer.AddrInfo{ // 返回节点信息
		ID:    p,                          // 节点ID
		Addrs: ps.memoryAddrBook.Addrs(p), // 节点地址列表
	}
}

// RemovePeer 移除与对等节点关联的以下条目:
// * KeyBook 中的条目
// * ProtoBook 中的条目
// * PeerMetadata 中的条目
// * Metrics 中的条目
// 注意:这不会从 AddrBook 中移除对等节点
// 参数:
//   - p: 对等节点ID
func (ps *pstoremem) RemovePeer(p peer.ID) {
	ps.memoryKeyBook.RemovePeer(p)      // 从密钥簿移除
	ps.memoryProtoBook.RemovePeer(p)    // 从协议簿移除
	ps.memoryPeerMetadata.RemovePeer(p) // 从元数据移除
	ps.Metrics.RemovePeer(p)            // 从指标移除
}
