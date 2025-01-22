// Package discovery 提供了dep2p的服务广告和节点发现接口
package discovery

import (
	"context"
	"time"

	"github.com/dep2p/go-dep2p/core/peer"
	logging "github.com/dep2p/log"
)

var log = logging.Logger("core-discovery")

// Advertiser 定义了服务广告的接口
type Advertiser interface {
	// Advertise 广播一个服务
	// 参数:
	//   - ctx: 上下文对象,用于控制操作的生命周期
	//   - ns: 命名空间字符串,用于标识服务
	//   - opts: 可选的配置选项
	// 返回:
	//   - time.Duration: 广告的有效时长
	//   - error: 操作过程中的错误信息
	Advertise(ctx context.Context, ns string, opts ...Option) (time.Duration, error)
}

// Discoverer 定义了节点发现的接口
type Discoverer interface {
	// FindPeers 发现提供特定服务的节点
	// 参数:
	//   - ctx: 上下文对象,用于控制操作的生命周期
	//   - ns: 命名空间字符串,用于标识要查找的服务
	//   - opts: 可选的配置选项
	// 返回:
	//   - <-chan peer.AddrInfo: 返回包含发现节点信息的通道
	//   - error: 操作过程中的错误信息
	FindPeers(ctx context.Context, ns string, opts ...Option) (<-chan peer.AddrInfo, error)
}

// Discovery 是一个组合了服务广告和节点发现功能的接口
type Discovery interface {
	// 嵌入 Advertiser 接口
	Advertiser
	// 嵌入 Discoverer 接口
	Discoverer
}
