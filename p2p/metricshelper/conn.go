package metricshelper

import ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"

// 支持的传输协议列表,包括中继、WebRTC、WebRTC直连、WebTransport、QUIC、QUIC v1、WSS、WS、TCP
var transports = [...]int{ma.P_CIRCUIT, ma.P_WEBRTC, ma.P_WEBRTC_DIRECT, ma.P_WEBTRANSPORT, ma.P_QUIC, ma.P_QUIC_V1, ma.P_WSS, ma.P_WS, ma.P_TCP}

// GetTransport 获取多地址使用的传输协议名称
// 参数:
//   - a: 多地址对象
//
// 返回值:
//   - string: 传输协议名称,如果无法识别则返回"other"
func GetTransport(a ma.Multiaddr) string {
	// 如果多地址为空,返回"other"
	if a == nil {
		return "other"
	}
	// 遍历支持的传输协议列表
	for _, t := range transports {
		// 尝试获取该协议的值,如果成功则返回协议名称
		if _, err := a.ValueForProtocol(t); err == nil {
			return ma.ProtocolWithCode(t).Name
		}
	}
	// 未找到匹配的协议,返回"other"
	return "other"
}

// GetIPVersion 获取多地址使用的IP版本
// 参数:
//   - addr: 多地址对象
//
// 返回值:
//   - string: IP版本("ip4"/"ip6"),如果无法识别则返回"unknown"
func GetIPVersion(addr ma.Multiaddr) string {
	// 初始化版本为未知
	version := "unknown"
	// 如果多地址为空,返回未知版本
	if addr == nil {
		return version
	}
	// 遍历多地址的每个组件
	ma.ForEach(addr, func(c ma.Component) bool {
		// 根据协议代码判断IP版本
		switch c.Protocol().Code {
		case ma.P_IP4, ma.P_DNS4:
			// IPv4或DNS4
			version = "ip4"
		case ma.P_IP6, ma.P_DNS6:
			// IPv6或DNS6
			version = "ip6"
		}
		// 返回false继续遍历
		return false
	})
	// 返回识别到的IP版本
	return version
}
