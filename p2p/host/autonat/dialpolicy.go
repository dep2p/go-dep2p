package autonat

import (
	"net"

	"github.com/dep2p/go-dep2p/core/host"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

// dialPolicy 定义了拨号策略
type dialPolicy struct {
	allowSelfDials bool      // 是否允许自拨号
	host           host.Host // dep2p主机实例
}

// skipDial 判断是否应该跳过对某个多地址的拨号
// 此逻辑在以下两种场景使用:
// 1. autonat客户端判断远程节点是否适合作为服务器
// 2. 服务器判断是否应该回拨请求的客户端
// 参数:
//   - addr: ma.Multiaddr 要判断的多地址
//
// 返回值:
//   - bool: 如果应该跳过拨号则返回true
func (d *dialPolicy) skipDial(addr ma.Multiaddr) bool {
	// 跳过中继地址
	_, err := addr.ValueForProtocol(ma.P_CIRCUIT)
	if err == nil {
		log.Debugf("跳过中继地址: %s", addr.String())
		return true
	}

	// 如果允许自拨号则不跳过
	if d.allowSelfDials {
		log.Debugf("允许自拨号: %s", addr.String())
		return false
	}

	// 跳过私有网络(不可路由)地址
	if !manet.IsPublicAddr(addr) {
		log.Debugf("跳过私有网络地址: %s", addr.String())
		return true
	}
	// 获取候选IP地址
	candidateIP, err := manet.ToIP(addr)
	if err != nil {
		log.Debugf("获取候选IP地址失败: %s", err.Error())
		return true
	}

	// 跳过我们认为是本地节点的地址
	for _, localAddr := range d.host.Addrs() {
		localIP, err := manet.ToIP(localAddr)
		if err != nil {
			continue
		}
		if localIP.Equal(candidateIP) {
			log.Debugf("跳过本地节点地址: %s", addr.String())
			return true
		}
	}

	return false
}

// skipPeer 判断是否应该跳过对某个节点的拨号
// 如果节点的地址列表中有一个地址与我们的地址匹配,
// 即使列表中还有其他有效的公共地址,也会排除该节点
// 参数:
//   - addrs: []ma.Multiaddr 要判断的节点的地址列表
//
// 返回值:
//   - bool: 如果应该跳过该节点则返回true
func (d *dialPolicy) skipPeer(addrs []ma.Multiaddr) bool {
	// 获取本地地址列表
	localAddrs := d.host.Addrs()
	localHosts := make([]net.IP, 0)
	// 遍历本地地址,提取公共IP
	for _, lAddr := range localAddrs {
		if _, err := lAddr.ValueForProtocol(ma.P_CIRCUIT); err != nil && manet.IsPublicAddr(lAddr) {
			lIP, err := manet.ToIP(lAddr)
			if err != nil {
				continue
			}
			localHosts = append(localHosts, lIP)
		}
	}

	// 如果节点的公共IP是我们的IP之一,则跳过该节点
	goodPublic := false
	for _, addr := range addrs {
		if _, err := addr.ValueForProtocol(ma.P_CIRCUIT); err != nil && manet.IsPublicAddr(addr) {
			aIP, err := manet.ToIP(addr)
			if err != nil {
				continue
			}

			for _, lIP := range localHosts {
				if lIP.Equal(aIP) {
					log.Debugf("跳过本地节点地址: %s", addr.String())
					return true
				}
			}
			goodPublic = true
		}
	}

	// 如果允许自拨号则不跳过
	if d.allowSelfDials {
		return false
	}

	// 如果没有有效的公共地址则跳过
	return !goodPublic
}
