package swarm

import (
	"time"

	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// ListenAddresses 返回此 swarm 监听的地址列表
// 返回值:
//   - []ma.Multiaddr 监听地址列表
func (s *Swarm) ListenAddresses() []ma.Multiaddr {
	s.listeners.RLock()
	defer s.listeners.RUnlock()
	return s.listenAddressesNoLock()
}

// listenAddressesNoLock 返回此 swarm 监听的地址列表(无锁版本)
// 返回值:
//   - []ma.Multiaddr 监听地址列表
func (s *Swarm) listenAddressesNoLock() []ma.Multiaddr {
	// 预分配一个稍大的切片以避免在循环中重新分配
	addrs := make([]ma.Multiaddr, 0, len(s.listeners.m)+10)
	// 遍历所有监听器,获取其多地址
	for l := range s.listeners.m {
		addrs = append(addrs, l.Multiaddr())
	}
	return addrs
}

// 接口地址缓存的有效期
const ifaceAddrsCacheDuration = 1 * time.Minute

// InterfaceListenAddresses 返回此 swarm 监听的地址列表
// 它会将"任意接口"地址(/ip4/0.0.0.0, /ip6/::)展开为已知的本地接口地址
// 返回值:
//   - []ma.Multiaddr 监听地址列表
//   - error 可能的错误
func (s *Swarm) InterfaceListenAddresses() ([]ma.Multiaddr, error) {
	s.listeners.RLock() // 读锁开始

	ifaceListenAddres := s.listeners.ifaceListenAddres
	isEOL := time.Now().After(s.listeners.cacheEOL)
	s.listeners.RUnlock() // 读锁结束

	if !isEOL {
		// 缓存有效,克隆切片返回
		return append(ifaceListenAddres[:0:0], ifaceListenAddres...), nil
	}

	// 缓存无效
	// 执行双重检查锁定

	s.listeners.Lock() // 写锁开始

	ifaceListenAddres = s.listeners.ifaceListenAddres
	isEOL = time.Now().After(s.listeners.cacheEOL)
	if isEOL {
		// 缓存仍然无效
		listenAddres := s.listenAddressesNoLock()
		if len(listenAddres) > 0 {
			// 实际正在监听地址
			var err error
			// 解析未指定的地址为具体接口地址
			ifaceListenAddres, err = manet.ResolveUnspecifiedAddresses(listenAddres, nil)
			if err != nil {
				s.listeners.Unlock() // 错误时提前解锁
				log.Debugf("解析未指定的地址失败: %v", err)
				return nil, err
			}
		} else {
			ifaceListenAddres = nil
		}

		// 更新缓存和过期时间
		s.listeners.ifaceListenAddres = ifaceListenAddres
		s.listeners.cacheEOL = time.Now().Add(ifaceAddrsCacheDuration)
	}

	s.listeners.Unlock() // 写锁结束

	// 返回缓存地址的克隆
	return append(ifaceListenAddres[:0:0], ifaceListenAddres...), nil
}
