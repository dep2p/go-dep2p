package quicreuse

import (
	"fmt"
	"net"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	"github.com/quic-go/quic-go"
)

var (
	// QUIC v1版本的多地址组件
	quicV1MA = ma.StringCast("/quic-v1")
)

// ToQuicMultiaddr 将网络地址和QUIC版本转换为多地址
// 参数:
//   - na: net.Addr 网络地址
//   - version: quic.Version QUIC协议版本
//
// 返回值:
//   - ma.Multiaddr 转换后的多地址
//   - error 错误信息
func ToQuicMultiaddr(na net.Addr, version quic.Version) (ma.Multiaddr, error) {
	// 将网络地址转换为UDP多地址
	udpMA, err := manet.FromNetAddr(na)
	if err != nil {
		log.Debugf("将网络地址转换为UDP多地址时出错: %s", err)
		return nil, err
	}
	// 根据QUIC版本进行处理
	switch version {
	case quic.Version1:
		// 封装QUIC v1组件
		return udpMA.Encapsulate(quicV1MA), nil
	default:
		log.Debugf("未知的QUIC版本")
		return nil, fmt.Errorf("未知的QUIC版本")
	}
}

// FromQuicMultiaddr 从多地址中解析出UDP地址和QUIC版本
// 参数:
//   - addr: ma.Multiaddr 多地址
//
// 返回值:
//   - *net.UDPAddr UDP地址
//   - quic.Version QUIC协议版本
//   - error 错误信息
func FromQuicMultiaddr(addr ma.Multiaddr) (*net.UDPAddr, quic.Version, error) {
	// 初始化QUIC版本和QUIC组件之前的地址部分
	var version quic.Version
	var partsBeforeQUIC []ma.Multiaddr
	// 遍历多地址的各个组件
	ma.ForEach(addr, func(c ma.Component) bool {
		switch c.Protocol().Code {
		case ma.P_QUIC_V1:
			// 找到QUIC v1组件
			version = quic.Version1
			return false
		default:
			// 收集QUIC组件之前的地址部分
			partsBeforeQUIC = append(partsBeforeQUIC, &c)
			return true
		}
	})
	if len(partsBeforeQUIC) == 0 {
		log.Debugf("QUIC组件之前没有地址部分")
		return nil, version, fmt.Errorf("QUIC组件之前没有地址部分")
	}
	if version == 0 {
		// 未找到QUIC版本
		log.Debugf("未知的QUIC版本")
		return nil, version, fmt.Errorf("未知的QUIC版本")
	}
	// 将地址部分转换为网络地址
	netAddr, err := manet.ToNetAddr(ma.Join(partsBeforeQUIC...))
	if err != nil {
		log.Debugf("将地址部分转换为网络地址时出错: %s", err)
		return nil, version, err
	}
	// 转换为UDP地址
	udpAddr, ok := netAddr.(*net.UDPAddr)
	if !ok {
		log.Debugf("不是UDP地址")
		return nil, 0, fmt.Errorf("不是UDP地址")
	}
	return udpAddr, version, nil
}
