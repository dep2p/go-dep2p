package autonat

import (
	"context"
	"io"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// AutoNAT 是NAT自动发现的接口
type AutoNAT interface {
	// Status 返回当前NAT状态
	// 返回值:
	//   - network.Reachability: 返回当前NAT的可达性状态
	Status() network.Reachability
	io.Closer // 继承io.Closer接口,用于关闭资源
}

// Client 是与AutoNAT对等节点通信的无状态客户端接口
type Client interface {
	// DialBack 请求提供AutoNAT服务的对等节点进行回拨测试,
	// 并在连接成功时报告地址
	// 参数:
	//   - ctx: context.Context 上下文,用于控制请求的生命周期
	//   - p: peer.ID 对等节点ID,指定要请求回拨的节点
	//
	// 返回值:
	//   - error: 如果发生错误则返回错误信息,nil表示回拨成功
	DialBack(ctx context.Context, p peer.ID) error
}

// AddrFunc 是返回本地主机候选地址的函数类型
// 返回值:
//   - []ma.Multiaddr: 返回本地主机的多地址列表
type AddrFunc func() []ma.Multiaddr

// Option 是用于配置AutoNAT的选项函数类型
// 参数:
//   - *config: 配置对象指针
//
// 返回值:
//   - error: 如果发生错误则返回错误信息,nil表示配置成功
type Option func(*config) error
