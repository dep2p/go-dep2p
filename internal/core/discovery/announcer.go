// Package discovery 提供节点发现模块的实现
package discovery

import (
	"context"
	"sync/atomic"
	"time"

	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
)

// ============================================================================
//                              通告功能
// ============================================================================

// Announce 通告本节点
//
// 系统域通告（无需 JoinRealm）：
//
//	Announce 操作在系统域（`dep2p/v1/sys/<namespace>`）注册服务，
//	不检查 Realm 成员身份。这允许节点在未加入任何业务 Realm 的情况下
//	注册基础设施服务（如 Relay、Bootstrap）。
//
// 参考：docs/04-usage/examples/advanced.md#系统域服务发现
func (s *DiscoveryService) Announce(ctx context.Context, namespace string) error {
	return s.AnnounceWithTTL(ctx, namespace, time.Hour)
}

// AnnounceWithTTL 带 TTL 的通告
func (s *DiscoveryService) AnnounceWithTTL(ctx context.Context, namespace string, ttl time.Duration) error {
	s.mu.RLock()
	announcers := make(map[string]discoveryif.Announcer, len(s.announcers))
	for k, v := range s.announcers {
		announcers[k] = v
	}
	s.mu.RUnlock()

	var lastErr error
	for name, announcer := range announcers {
		if err := announcer.AnnounceWithTTL(ctx, namespace, ttl); err != nil {
			log.Debug("通告失败",
				"announcer", name,
				"err", err)
			lastErr = err
		}
	}

	return lastErr
}

// StopAnnounce 停止通告
func (s *DiscoveryService) StopAnnounce(namespace string) error {
	s.mu.RLock()
	announcers := make(map[string]discoveryif.Announcer, len(s.announcers))
	for k, v := range s.announcers {
		announcers[k] = v
	}
	s.mu.RUnlock()

	for _, announcer := range announcers {
		_ = announcer.StopAnnounce(namespace) // 停止通告错误可忽略
	}

	return nil
}

// RefreshAnnounce 刷新地址发布
// 当节点地址变化时调用，更新发现器的本地地址并重新发布
func (s *DiscoveryService) RefreshAnnounce(addrs []endpoint.Address) {
	if atomic.LoadInt32(&s.running) == 0 {
		return
	}

	log.Debug("刷新地址发布", "addrs", len(addrs))

	// 转换“对外通告地址”为字符串列表（通常用于 DHT/PeerRecord 等公共通告）
	advertisedStrs := make([]string, len(addrs))
	for i, addr := range addrs {
		advertisedStrs[i] = addr.String()
	}

	// mDNS 需要“监听端口 + 本机 LAN IP”，而 Reachability/AdvertisedAddrs 可能为空：
	// - 可达性优先会过滤 0.0.0.0/:: 等监听地址，导致 addrs=[]；
	// - 但 mDNS 仍需要从监听地址推断端口，再用真实网卡 IP 生成可拨号地址。
	//
	// 因此：对 mdns 发现器，优先使用 endpoint.ListenAddrs() 的原始监听地址（包含端口）。
	var listenStrs []string
	s.epMu.RLock()
	ep := s.ep
	s.epMu.RUnlock()
	if ep != nil {
		las := ep.ListenAddrs()
		listenStrs = make([]string, 0, len(las))
		for _, a := range las {
			if a == nil {
				continue
			}
			listenStrs = append(listenStrs, a.String())
		}
	}

	// 更新各发现器的本地地址
	s.mu.RLock()
	discoverers := make(map[string]discoveryif.Discoverer, len(s.discoverers))
	for k, v := range s.discoverers {
		discoverers[k] = v
	}
	s.mu.RUnlock()

	for name, discoverer := range discoverers {
		// 检查是否实现了 AddressUpdater 接口
		if updater, ok := discoverer.(discoveryif.AddressUpdater); ok {
			// mdns 特殊处理：用 ListenAddrs 提供端口推断输入
			if name == "mdns" && len(listenStrs) > 0 {
				updater.UpdateLocalAddrs(listenStrs)
				log.Debug("更新发现器地址（mDNS 使用 ListenAddrs）",
					"discoverer", name,
					"addrs", listenStrs)
				continue
			}
			updater.UpdateLocalAddrs(advertisedStrs)
			log.Debug("更新发现器地址",
				"discoverer", name,
				"addrs", advertisedStrs)
		}
	}

	// 重新执行通告
	s.runAnnounce()
}
