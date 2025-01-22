package routing

import (
	"context"
	"time"

	"github.com/dep2p/go-dep2p/core/discovery"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/routing"
	logging "github.com/dep2p/log"

	"github.com/dep2p/go-dep2p/cid"
	mh "github.com/dep2p/go-dep2p/multiformats/multihash"
)

var log = logging.Logger("discovery-routing")

// RoutingDiscovery 是一个使用ContentRouting实现的发现服务
// 命名空间使用SHA256哈希转换为Cid
type RoutingDiscovery struct {
	routing.ContentRouting // 内容路由接口
}

// NewRoutingDiscovery 创建一个新的路由发现服务实例
// 参数:
//   - router: routing.ContentRouting 内容路由实例
//
// 返回值:
//   - *RoutingDiscovery: 返回创建的路由发现服务实例
func NewRoutingDiscovery(router routing.ContentRouting) *RoutingDiscovery {
	return &RoutingDiscovery{router}
}

// Advertise 在指定命名空间发布节点信息
// 参数:
//   - ctx: context.Context 上下文
//   - ns: string 命名空间
//   - opts: ...discovery.Option 可选的配置选项
//
// 返回值:
//   - time.Duration: 返回发布信息的有效期
//   - error: 如果发生错误则返回错误信息
func (d *RoutingDiscovery) Advertise(ctx context.Context, ns string, opts ...discovery.Option) (time.Duration, error) {
	// 应用配置选项
	var options discovery.Options
	err := options.Apply(opts...)
	if err != nil {
		log.Debugf("应用配置选项失败: %v", err)
		return 0, err
	}

	// 设置TTL(存活时间)
	ttl := options.Ttl
	if ttl == 0 || ttl > 3*time.Hour {
		// DHT提供者记录有效期为24小时,但建议至少每6小时重新发布一次
		// 这里更进一步,每3小时重新发布
		ttl = 3 * time.Hour
	}

	// 将命名空间转换为CID
	cid, err := nsToCid(ns)
	if err != nil {
		log.Debugf("命名空间转换为CID失败: %v", err)
		return 0, err
	}

	// 创建一个带超时的上下文,用于限制DHT查找最近节点的时间
	// 不设置超时会导致DHT无限期地查找
	pctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	// 提供记录
	err = d.Provide(pctx, cid, true)
	if err != nil {
		log.Debugf("提供记录失败: %v", err)
		return 0, err
	}

	return ttl, nil
}

// FindPeers 在指定命名空间查找节点
// 参数:
//   - ctx: context.Context 上下文
//   - ns: string 命名空间
//   - opts: ...discovery.Option 可选的配置选项
//
// 返回值:
//   - <-chan peer.AddrInfo: 返回节点信息通道
//   - error: 如果发生错误则返回错误信息
func (d *RoutingDiscovery) FindPeers(ctx context.Context, ns string, opts ...discovery.Option) (<-chan peer.AddrInfo, error) {
	// 设置默认选项
	options := discovery.Options{
		Limit: 100, // 如果未指定则使用默认限制
	}
	// 应用配置选项
	err := options.Apply(opts...)
	if err != nil {
		log.Debugf("应用配置选项失败: %v", err)
		return nil, err
	}

	// 将命名空间转换为CID
	cid, err := nsToCid(ns)
	if err != nil {
		log.Debugf("命名空间转换为CID失败: %v", err)
		return nil, err
	}

	return d.FindProvidersAsync(ctx, cid, options.Limit), nil
}

// nsToCid 将命名空间字符串转换为CID
// 参数:
//   - ns: string 命名空间
//
// 返回值:
//   - cid.Cid: 返回转换后的CID
//   - error: 如果发生错误则返回错误信息
func nsToCid(ns string) (cid.Cid, error) {
	// 计算命名空间的SHA256哈希
	h, err := mh.Sum([]byte(ns), mh.SHA2_256, -1)
	if err != nil {
		log.Debugf("计算命名空间的SHA256哈希失败: %v", err)
		return cid.Undef, err
	}

	// 创建新的CIDv1
	return cid.NewCidV1(cid.Raw, h), nil
}

// NewDiscoveryRouting 创建一个新的发现路由服务实例
// 参数:
//   - disc: discovery.Discovery 发现服务实例
//   - opts: ...discovery.Option 可选的配置选项
//
// 返回值:
//   - *DiscoveryRouting: 返回创建的发现路由服务实例
func NewDiscoveryRouting(disc discovery.Discovery, opts ...discovery.Option) *DiscoveryRouting {
	return &DiscoveryRouting{disc, opts}
}

// DiscoveryRouting 实现了一个基于发现服务的路由
type DiscoveryRouting struct {
	discovery.Discovery                    // 发现服务接口
	opts                []discovery.Option // 配置选项
}

// Provide 提供指定CID的内容
// 参数:
//   - ctx: context.Context 上下文
//   - c: cid.Cid 内容标识符
//   - bcast: bool 是否广播
//
// 返回值:
//   - error: 如果发生错误则返回错误信息
func (r *DiscoveryRouting) Provide(ctx context.Context, c cid.Cid, bcast bool) error {
	// 如果不广播则直接返回
	if !bcast {
		return nil
	}

	// 发布CID对应的命名空间
	_, err := r.Advertise(ctx, cidToNs(c), r.opts...)
	if err != nil {
		log.Debugf("发布CID对应的命名空间失败: %v", err)
	}
	return err
}

// FindProvidersAsync 异步查找提供指定CID内容的节点
// 参数:
//   - ctx: context.Context 上下文
//   - c: cid.Cid 内容标识符
//   - limit: int 返回结果数量限制
//
// 返回值:
//   - <-chan peer.AddrInfo: 返回节点信息通道
func (r *DiscoveryRouting) FindProvidersAsync(ctx context.Context, c cid.Cid, limit int) <-chan peer.AddrInfo {
	// 查找CID对应命名空间的节点
	ch, err := r.FindPeers(ctx, cidToNs(c), append([]discovery.Option{discovery.Limit(limit)}, r.opts...)...)
	if err != nil {
		log.Debugf("查找CID对应的命名空间节点失败: %v", err)
	}
	return ch
}

// cidToNs 将CID转换为命名空间
// 参数:
//   - c: cid.Cid 内容标识符
//
// 返回值:
//   - string: 返回转换后的命名空间
func cidToNs(c cid.Cid) string {
	return "/provider/" + c.String()
}
