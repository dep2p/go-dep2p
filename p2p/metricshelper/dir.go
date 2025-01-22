package metricshelper

import "github.com/dep2p/go-dep2p/core/network"

// GetDirection 获取连接的方向
// 参数:
//   - dir: network.Direction - 连接方向枚举值
//
// 返回值:
//   - string: 连接方向的字符串描述:"outbound"(出站)/"inbound"(入站)/"unknown"(未知)
func GetDirection(dir network.Direction) string {
	// 根据连接方向枚举值进行判断
	switch dir {
	case network.DirOutbound:
		// 出站连接
		return "outbound"
	case network.DirInbound:
		// 入站连接
		return "inbound"
	default:
		// 未知连接方向
		return "unknown"
	}
}
