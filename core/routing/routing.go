// Package routing 提供了 dep2p 中的对等节点路由和内容路由接口

package routing

import (
	"context"
	"errors"

	cid "github.com/dep2p/go-dep2p/cid"
	ci "github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/peer"
	logging "github.com/dep2p/log"
)

var log = logging.Logger("core-routing")

// ErrNotFound 当路由器找不到请求的记录时返回此错误
var ErrNotFound = errors.New("路由: 未找到")

// ErrNotSupported 当路由器不支持给定的记录类型或操作时返回此错误
var ErrNotSupported = errors.New("路由: 不支持的操作或密钥")

// ContentProviding 定义了在路由系统中宣告内容位置的能力
type ContentProviding interface {
	// Provide 将给定的 CID 添加到内容路由系统中
	// 参数：
	//   - ctx: context.Context 用于控制操作的上下文
	//   - c: cid.Cid 要提供的内容标识符
	//   - announce: bool 如果为 true 则同时宣告内容，否则仅在本地记录
	//
	// 返回值：
	//   - error: 如果发生错误，返回错误信息
	Provide(context.Context, cid.Cid, bool) error
}

// ContentDiscovery 定义了使用路由系统检索内容提供者的能力
type ContentDiscovery interface {
	// FindProvidersAsync 异步搜索能够提供给定密钥的对等节点
	// 参数：
	//   - ctx: context.Context 用于控制操作的上下文
	//   - c: cid.Cid 要查找的内容标识符
	//   - count: int 要返回的最大结果数，0 表示不限制
	//
	// 返回值：
	//   - <-chan peer.AddrInfo: 返回提供者地址信息的通道
	FindProvidersAsync(context.Context, cid.Cid, int) <-chan peer.AddrInfo
}

// ContentRouting 是一个内容提供者的间接层，用于查找谁拥有什么内容
// 内容通过 CID(内容标识符)标识，它以面向未来的方式对标识内容的哈希进行编码
type ContentRouting interface {
	ContentProviding
	ContentDiscovery
}

// PeerRouting 定义了查找特定对等节点地址信息的方法
// 可以通过简单的查找表、跟踪服务器甚至 DHT 来实现
type PeerRouting interface {
	// FindPeer 搜索具有给定 ID 的对等节点
	// 参数：
	//   - ctx: context.Context 用于控制操作的上下文
	//   - id: peer.ID 要查找的对等节点 ID
	//
	// 返回值：
	//   - peer.AddrInfo: 包含相关地址的对等节点信息
	//   - error: 如果发生错误，返回错误信息
	FindPeer(context.Context, peer.ID) (peer.AddrInfo, error)
}

// ValueStore 定义了基本的 Put/Get 接口
type ValueStore interface {
	// PutValue 添加与给定键对应的值
	// 参数：
	//   - ctx: context.Context 用于控制操作的上下文
	//   - key: string 存储值的键
	//   - value: []byte 要存储的值
	//   - opts: ...Option 可选的存储选项
	//
	// 返回值：
	//   - error: 如果发生错误，返回错误信息
	PutValue(context.Context, string, []byte, ...Option) error

	// GetValue 搜索与给定键对应的值
	// 参数：
	//   - ctx: context.Context 用于控制操作的上下文
	//   - key: string 要查找的键
	//   - opts: ...Option 可选的查找选项
	//
	// 返回值：
	//   - []byte: 找到的值
	//   - error: 如果发生错误，返回错误信息
	GetValue(context.Context, string, ...Option) ([]byte, error)

	// SearchValue 持续搜索与给定键对应的更好值
	// 参数：
	//   - ctx: context.Context 用于控制操作的上下文
	//   - key: string 要搜索的键
	//   - opts: ...Option 可选的搜索选项
	//
	// 返回值：
	//   - <-chan []byte: 返回找到的值的通道
	//   - error: 如果发生错误，返回错误信息
	SearchValue(context.Context, string, ...Option) (<-chan []byte, error)
}

// Routing 组合了 dep2p 支持的不同路由类型
// 可以由单个组件(如 DHT)或多个针对每个任务优化的不同组件来满足
type Routing interface {
	ContentRouting
	PeerRouting
	ValueStore

	// Bootstrap 允许调用者提示路由系统进入并保持引导状态
	// 这不是同步调用
	// 参数：
	//   - ctx: context.Context 用于控制操作的上下文
	//
	// 返回值：
	//   - error: 如果发生错误，返回错误信息
	Bootstrap(context.Context) error
}

// PubKeyFetcher 是一个应该由可以优化公钥检索的值存储实现的接口
type PubKeyFetcher interface {
	// GetPublicKey 返回给定对等节点的公钥
	// 参数：
	//   - ctx: context.Context 用于控制操作的上下文
	//   - id: peer.ID 要获取公钥的对等节点 ID
	//
	// 返回值：
	//   - ci.PubKey: 获取到的公钥
	//   - error: 如果发生错误，返回错误信息
	GetPublicKey(context.Context, peer.ID) (ci.PubKey, error)
}

// KeyForPublicKey 返回用于从值存储中检索公钥的键
// 参数：
//   - id: peer.ID 对等节点 ID
//
// 返回值：
//   - string: 用于检索公钥的键
func KeyForPublicKey(id peer.ID) string {
	return "/pk/" + string(id)
}

// GetPublicKey 从值存储中检索与给定对等节点 ID 关联的公钥
// 参数：
//   - r: ValueStore 值存储接口
//   - ctx: context.Context 用于控制操作的上下文
//   - p: peer.ID 要获取公钥的对等节点 ID
//
// 返回值：
//   - ci.PubKey: 获取到的公钥
//   - error: 如果发生错误，返回错误信息
func GetPublicKey(r ValueStore, ctx context.Context, p peer.ID) (ci.PubKey, error) {
	// 尝试从对等节点 ID 中提取公钥
	switch k, err := p.ExtractPublicKey(); err {
	case peer.ErrNoPublicKey:
		// 如果无法提取，继续检查数据存储
	case nil:
		return k, nil
	default:
		log.Debugf("获取公钥失败: %v", err)
		return nil, err
	}

	// 如果值存储实现了 PubKeyFetcher 接口，使用优化的获取器
	if dht, ok := r.(PubKeyFetcher); ok {
		return dht.GetPublicKey(ctx, p)
	}

	// 否则从值存储中获取公钥
	key := KeyForPublicKey(p)
	pkval, err := r.GetValue(ctx, key)
	if err != nil {
		log.Debugf("从值存储中获取公钥失败: %v", err)
		return nil, err
	}

	// 从节点数据中解析公钥
	return ci.UnmarshalPublicKey(pkval)
}
