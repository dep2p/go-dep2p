package util

import (
	"context"
	"time"

	"github.com/dep2p/go-dep2p/core/discovery"
	"github.com/dep2p/go-dep2p/core/peer"

	logging "github.com/dep2p/log"
)

// log 用于记录发现工具相关的日志
var log = logging.Logger("discovery-util")

// FindPeers 同步从发现者收集对等节点信息
// 参数:
//   - ctx: context.Context 上下文
//   - d: discovery.Discoverer 发现者接口
//   - ns: string 命名空间
//   - opts: ...discovery.Option 可选的配置选项
//
// 返回值:
//   - []peer.AddrInfo: 返回收集到的节点信息列表
//   - error: 如果发生错误则返回错误信息
func FindPeers(ctx context.Context, d discovery.Discoverer, ns string, opts ...discovery.Option) ([]peer.AddrInfo, error) {
	// 调用发现者接口查找节点
	ch, err := d.FindPeers(ctx, ns, opts...)
	if err != nil {
		log.Debugf("查找节点失败: %v", err)
		return nil, err
	}

	// 收集通道中的节点信息
	res := make([]peer.AddrInfo, 0, len(ch))
	for pi := range ch {
		res = append(res, pi)
	}

	return res, nil
}

// Advertise 持续通过广告者发布服务
// 参数:
//   - ctx: context.Context 上下文
//   - a: discovery.Advertiser 广告者接口
//   - ns: string 命名空间
//   - opts: ...discovery.Option 可选的配置选项
func Advertise(ctx context.Context, a discovery.Advertiser, ns string, opts ...discovery.Option) {
	go func() {
		for {
			// 发布服务
			ttl, err := a.Advertise(ctx, ns, opts...)
			if err != nil {
				// 发生错误时记录日志
				log.Debugf("发布服务 %s 时出错: %s", ns, err.Error())
				if ctx.Err() != nil {
					return
				}

				// 等待2分钟后重试或上下文取消
				select {
				case <-time.After(2 * time.Minute):
					continue
				case <-ctx.Done():
					return
				}
			}

			// 在TTL过期前重新发布
			wait := 7 * ttl / 8
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return
			}
		}
	}()
}
