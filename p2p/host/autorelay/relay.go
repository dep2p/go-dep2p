package autorelay

import (
	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// Filter 过滤掉所有中继地址
//
// 方法名称: Filter
// 参数:
//   - addrs []ma.Multiaddr: 多地址切片
//
// 返回值:
//   - []ma.Multiaddr: 过滤后的多地址切片
//
// 已弃用: 如果用户需要此功能,可以很容易地自行实现
func Filter(addrs []ma.Multiaddr) []ma.Multiaddr {
	// 创建一个新的切片用于存储非中继地址,容量为输入切片长度
	raddrs := make([]ma.Multiaddr, 0, len(addrs))
	// 遍历输入的地址切片
	for _, addr := range addrs {
		// 如果是中继地址则跳过
		if isRelayAddr(addr) {
			continue
		}
		// 将非中继地址添加到结果切片中
		raddrs = append(raddrs, addr)
	}
	// 返回过滤后的地址切片
	return raddrs
}
