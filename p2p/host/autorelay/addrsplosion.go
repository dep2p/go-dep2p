package autorelay

import (
	"encoding/binary"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

// cleanupAddressSet 清理中继节点的地址集合,移除私有地址并限制地址爆炸
// 参数:
//   - addrs: []ma.Multiaddr 待清理的地址列表
//
// 返回值:
//   - []ma.Multiaddr: 返回清理后的地址列表
func cleanupAddressSet(addrs []ma.Multiaddr) []ma.Multiaddr {
	var public, private []ma.Multiaddr // 定义公网和私网地址切片

	for _, a := range addrs { // 遍历所有地址
		if isRelayAddr(a) { // 如果是中继地址则跳过
			continue
		}

		if manet.IsPublicAddr(a) || isDNSAddr(a) { // 如果是公网地址或DNS地址
			public = append(public, a) // 添加到公网地址列表
			continue
		}

		// 丢弃不可路由的地址
		if manet.IsPrivateAddr(a) { // 如果是私网地址
			private = append(private, a) // 添加到私网地址列表
		}
	}

	if !hasAddrsplosion(public) { // 如果没有地址爆炸问题
		return public // 直接返回公网地址
	}

	return sanitizeAddrsplodedSet(public, private) // 清理地址爆炸集合
}

// isRelayAddr 检查给定地址是否为中继地址
// 参数:
//   - a: ma.Multiaddr 待检查的地址
//
// 返回值:
//   - bool: 如果是中继地址返回true,否则返回false
func isRelayAddr(a ma.Multiaddr) bool {
	isRelay := false // 初始化中继标志

	ma.ForEach(a, func(c ma.Component) bool { // 遍历地址的每个组件
		switch c.Protocol().Code {
		case ma.P_CIRCUIT: // 如果是中继协议
			isRelay = true // 设置中继标志
			return false
		default:
			return true
		}
	})

	return isRelay
}

// isDNSAddr 检查给定地址是否为DNS地址
// 参数:
//   - a: ma.Multiaddr 待检查的地址
//
// 返回值:
//   - bool: 如果是DNS地址返回true,否则返回false
func isDNSAddr(a ma.Multiaddr) bool {
	if first, _ := ma.SplitFirst(a); first != nil { // 获取地址的第一个组件
		switch first.Protocol().Code {
		case ma.P_DNS, ma.P_DNS4, ma.P_DNS6, ma.P_DNSADDR: // 如果是DNS相关协议
			return true
		}
	}
	return false
}

// hasAddrsplosion 检查地址列表是否存在地址爆炸问题
// 当在同一基础地址上广播多个端口时,就存在地址爆炸
// 参数:
//   - addrs: []ma.Multiaddr 待检查的地址列表
//
// 返回值:
//   - bool: 如果存在地址爆炸返回true,否则返回false
func hasAddrsplosion(addrs []ma.Multiaddr) bool {
	aset := make(map[string]int) // 创建地址-端口映射

	for _, a := range addrs { // 遍历所有地址
		key, port := addrKeyAndPort(a) // 获取地址键和端口
		xport, ok := aset[key]         // 检查是否已存在该地址键
		if ok && port != xport {       // 如果存在且端口不同
			return true // 存在地址爆炸
		}
		aset[key] = port // 记录地址键和端口
	}

	return false
}

// addrKeyAndPort 从地址中提取键和端口
// 参数:
//   - a: ma.Multiaddr 待处理的地址
//
// 返回值:
//   - string: 地址键
//   - int: 端口号
func addrKeyAndPort(a ma.Multiaddr) (string, int) {
	var (
		key  string // 地址键
		port int    // 端口号
	)

	ma.ForEach(a, func(c ma.Component) bool { // 遍历地址的每个组件
		switch c.Protocol().Code {
		case ma.P_TCP, ma.P_UDP: // 如果是TCP或UDP协议
			port = int(binary.BigEndian.Uint16(c.RawValue())) // 解析端口号
			key += "/" + c.Protocol().Name                    // 添加协议名到键
		default:
			val := c.Value() // 获取组件值
			if val == "" {
				val = c.Protocol().Name // 如果值为空则使用协议名
			}
			key += "/" + val // 添加值到键
		}
		return true
	})

	return key, port
}

// sanitizeAddrsplodedSet 清理地址爆炸集合
// 使用以下启发式方法:
//   - 对于每个基础地址/协议组合,如果广播了多个端口,则优先使用默认端口
//   - 如果没有默认端口,则通过跟踪私有端口绑定来检查非标准端口
//   - 如果既没有默认端口也没有私有端口绑定,则无法推断正确的端口,返回所有地址
//
// 参数:
//   - public: []ma.Multiaddr 公网地址列表
//   - private: []ma.Multiaddr 私网地址列表
//
// 返回值:
//   - []ma.Multiaddr: 返回清理后的地址列表
func sanitizeAddrsplodedSet(public, private []ma.Multiaddr) []ma.Multiaddr {
	type portAndAddr struct { // 定义端口和地址的结构体
		addr ma.Multiaddr
		port int
	}

	privports := make(map[int]struct{})        // 创建私有端口集合
	pubaddrs := make(map[string][]portAndAddr) // 创建公网地址映射

	for _, a := range private { // 遍历私网地址
		_, port := addrKeyAndPort(a) // 获取端口
		privports[port] = struct{}{} // 记录私有端口
	}

	for _, a := range public { // 遍历公网地址
		key, port := addrKeyAndPort(a)                                          // 获取地址键和端口
		pubaddrs[key] = append(pubaddrs[key], portAndAddr{addr: a, port: port}) // 添加到公网地址映射
	}

	var result []ma.Multiaddr      // 定义结果切片
	for _, pas := range pubaddrs { // 遍历公网地址映射
		if len(pas) == 1 { // 如果只有一个地址
			result = append(result, pas[0].addr) // 添加到结果
			continue
		}

		haveAddr := false        // 是否已找到合适的地址
		for _, pa := range pas { // 遍历地址端口对
			if _, ok := privports[pa.port]; ok { // 如果匹配私有端口
				result = append(result, pa.addr) // 添加到结果
				haveAddr = true
				continue
			}

			if pa.port == 4001 || pa.port == 4002 { // 如果是默认端口
				result = append(result, pa.addr) // 添加到结果
				haveAddr = true
			}
		}

		if !haveAddr { // 如果没有找到合适的地址
			for _, pa := range pas { // 添加所有地址
				result = append(result, pa.addr)
			}
		}
	}

	return result
}
