package rcmgr

import (
	"bytes"
	"errors"
	"net"
	"sync"

	"github.com/dep2p/go-dep2p/core/peer"

	"github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

// Allowlist 白名单结构体
type Allowlist struct {
	mu sync.RWMutex
	// 一个简单的网络列表结构。虽然可能有更快的方法来检查IP地址是否在这个网络中,
	// 但对于小规模网络(<1_000)来说,这种方式已经足够好了。
	// 在尝试优化之前先分析基准测试。

	// 具有这些IP的任何对等节点都被允许
	allowedNetworks []*net.IPNet

	// 只有指定的对等节点可以使用这些IP
	allowedPeerByNetwork map[peer.ID][]*net.IPNet
}

// WithAllowlistedMultiaddrs 设置要加入白名单的多地址
// 参数:
//   - mas: 多地址切片
//
// 返回:
//   - Option: 配置选项函数
func WithAllowlistedMultiaddrs(mas []multiaddr.Multiaddr) Option {
	return func(rm *resourceManager) error {
		for _, ma := range mas { // 遍历多地址
			err := rm.allowlist.Add(ma) // 添加到白名单
			if err != nil {
				log.Debugf("添加到白名单失败: %v", err)
				return err
			}
		}
		return nil
	}
}

// newAllowlist 创建新的白名单实例
// 返回:
//   - Allowlist: 白名单实例
func newAllowlist() Allowlist {
	return Allowlist{
		allowedPeerByNetwork: make(map[peer.ID][]*net.IPNet), // 初始化对等节点网络映射
	}
}

// toIPNet 将多地址转换为IP网络
// 参数:
//   - ma: 多地址
//
// 返回:
//   - *net.IPNet: IP网络
//   - peer.ID: 对等节点ID
//   - error: 错误信息
func toIPNet(ma multiaddr.Multiaddr) (*net.IPNet, peer.ID, error) {
	var ipString string
	var mask string
	var allowedPeerStr string
	var allowedPeer peer.ID
	var isIPV4 bool

	// 遍历多地址组件
	multiaddr.ForEach(ma, func(c multiaddr.Component) bool {
		if c.Protocol().Code == multiaddr.P_IP4 || c.Protocol().Code == multiaddr.P_IP6 {
			isIPV4 = c.Protocol().Code == multiaddr.P_IP4 // 判断是否为IPv4
			ipString = c.Value()                          // 获取IP字符串
		}
		if c.Protocol().Code == multiaddr.P_IPCIDR {
			mask = c.Value() // 获取掩码
		}
		if c.Protocol().Code == multiaddr.P_P2P {
			allowedPeerStr = c.Value() // 获取对等节点字符串
		}
		return ipString == "" || mask == "" || allowedPeerStr == ""
	})

	if ipString == "" {
		log.Debugf("缺少IP地址")
		return nil, allowedPeer, errors.New("缺少IP地址")
	}

	if allowedPeerStr != "" {
		var err error
		allowedPeer, err = peer.Decode(allowedPeerStr) // 解码对等节点ID
		if err != nil {
			log.Debugf("解码允许的对等节点失败: %v", err)
			return nil, allowedPeer, err
		}
	}

	if mask == "" {
		ip := net.ParseIP(ipString) // 解析IP地址
		if ip == nil {
			log.Debugf("无效的IP地址")
			return nil, allowedPeer, errors.New("无效的IP地址")
		}
		var mask net.IPMask
		if isIPV4 {
			mask = net.CIDRMask(32, 32) // IPv4掩码
		} else {
			mask = net.CIDRMask(128, 128) // IPv6掩码
		}

		net := &net.IPNet{IP: ip, Mask: mask} // 创建IP网络
		return net, allowedPeer, nil
	}

	_, ipnet, err := net.ParseCIDR(ipString + "/" + mask) // 解析CIDR
	return ipnet, allowedPeer, err
}

// Add 将多地址添加到白名单中
// 参数:
//   - ma: 多地址
//
// 返回:
//   - error: 错误信息
func (al *Allowlist) Add(ma multiaddr.Multiaddr) error {
	ipnet, allowedPeer, err := toIPNet(ma) // 转换为IP网络
	if err != nil {
		log.Debugf("转换为IP网络失败: %v", err)
		return err
	}
	al.mu.Lock()         // 加锁
	defer al.mu.Unlock() // 解锁

	if allowedPeer != peer.ID("") {
		// 我们有对等节点ID约束
		if al.allowedPeerByNetwork == nil {
			al.allowedPeerByNetwork = make(map[peer.ID][]*net.IPNet) // 初始化映射
		}
		al.allowedPeerByNetwork[allowedPeer] = append(al.allowedPeerByNetwork[allowedPeer], ipnet) // 添加到对等节点网络列表
	} else {
		al.allowedNetworks = append(al.allowedNetworks, ipnet) // 添加到允许网络列表
	}
	return nil
}

// Remove 从白名单中移除多地址
// 参数:
//   - ma: 多地址
//
// 返回:
//   - error: 错误信息
func (al *Allowlist) Remove(ma multiaddr.Multiaddr) error {
	ipnet, allowedPeer, err := toIPNet(ma) // 转换为IP网络
	if err != nil {
		log.Debugf("转换为IP网络失败: %v", err)
		return err
	}
	al.mu.Lock()         // 加锁
	defer al.mu.Unlock() // 解锁

	ipNetList := al.allowedNetworks // 获取网络列表

	if allowedPeer != "" {
		// 我们有对等节点ID约束
		ipNetList = al.allowedPeerByNetwork[allowedPeer] // 获取对等节点的网络列表
	}

	if ipNetList == nil {
		return nil
	}

	i := len(ipNetList)
	for i > 0 {
		i--
		if ipNetList[i].IP.Equal(ipnet.IP) && bytes.Equal(ipNetList[i].Mask, ipnet.Mask) {
			// 交换移除
			ipNetList[i] = ipNetList[len(ipNetList)-1] // 将最后一个元素移到当前位置
			ipNetList = ipNetList[:len(ipNetList)-1]   // 截断列表
			break                                      // 只移除一个
		}
	}

	if allowedPeer != "" {
		al.allowedPeerByNetwork[allowedPeer] = ipNetList // 更新对等节点网络列表
	} else {
		al.allowedNetworks = ipNetList // 更新允许网络列表
	}

	return nil
}

// Allowed 检查多地址是否在白名单中
// 参数:
//   - ma: 多地址
//
// 返回:
//   - bool: 是否允许
func (al *Allowlist) Allowed(ma multiaddr.Multiaddr) bool {
	ip, err := manet.ToIP(ma) // 转换为IP
	if err != nil {
		log.Debugf("转换为IP失败: %v", err)
		return false
	}
	al.mu.RLock()         // 读锁
	defer al.mu.RUnlock() // 解锁

	for _, network := range al.allowedNetworks { // 检查允许网络
		if network.Contains(ip) {
			return true
		}
	}

	for _, allowedNetworks := range al.allowedPeerByNetwork { // 检查对等节点网络
		for _, network := range allowedNetworks {
			if network.Contains(ip) {
				return true
			}
		}
	}

	return false
}

// AllowedPeerAndMultiaddr 检查对等节点和多地址是否在白名单中
// 参数:
//   - peerID: 对等节点ID
//   - ma: 多地址
//
// 返回:
//   - bool: 是否允许
func (al *Allowlist) AllowedPeerAndMultiaddr(peerID peer.ID, ma multiaddr.Multiaddr) bool {
	ip, err := manet.ToIP(ma) // 转换为IP
	if err != nil {
		log.Debugf("转换为IP失败: %v", err)
		return false
	}
	al.mu.RLock()         // 读锁
	defer al.mu.RUnlock() // 解锁

	for _, network := range al.allowedNetworks { // 检查允许网络
		if network.Contains(ip) {
			// 找到不受对等节点ID约束的匹配项
			return true
		}
	}

	if expectedNetworks, ok := al.allowedPeerByNetwork[peerID]; ok { // 检查特定对等节点的网络
		for _, expectedNetwork := range expectedNetworks {
			if expectedNetwork.Contains(ip) {
				return true
			}
		}
	}

	return false
}
