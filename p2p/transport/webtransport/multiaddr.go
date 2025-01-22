package dep2pwebtransport

import (
	"errors"
	"net"
	"strconv"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	"github.com/dep2p/go-dep2p/multiformats/multibase"
	"github.com/dep2p/go-dep2p/multiformats/multihash"
)

// webtransportMA 是 WebTransport 协议的多地址表示
var webtransportMA = ma.StringCast("/quic-v1/webtransport")

// toWebtransportMultiaddr 将网络地址转换为 WebTransport 多地址
// 参数:
//   - na: net.Addr 网络地址对象
//
// 返回值:
//   - ma.Multiaddr WebTransport 多地址
//   - error 转换过程中的错误
func toWebtransportMultiaddr(na net.Addr) (ma.Multiaddr, error) {
	// 将网络地址转换为多地址格式
	addr, err := manet.FromNetAddr(na)
	if err != nil {
		log.Debugf("将网络地址转换为多地址格式失败: %s", err)
		return nil, err
	}
	// 验证是否为 UDP 地址
	if _, err := addr.ValueForProtocol(ma.P_UDP); err != nil {
		log.Debugf("不是一个 UDP 地址: %s", err)
		return nil, errors.New("不是一个 UDP 地址")
	}
	// 封装为 WebTransport 多地址
	return addr.Encapsulate(webtransportMA), nil
}

// stringToWebtransportMultiaddr 将字符串地址转换为 WebTransport 多地址
// 参数:
//   - str: string 地址字符串，格式为 "host:port"
//
// 返回值:
//   - ma.Multiaddr WebTransport 多地址
//   - error 转换过程中的错误
func stringToWebtransportMultiaddr(str string) (ma.Multiaddr, error) {
	// 分割主机和端口
	host, portStr, err := net.SplitHostPort(str)
	if err != nil {
		log.Debugf("分割主机和端口失败: %s", err)
		return nil, err
	}
	// 解析端口号
	port, err := strconv.ParseInt(portStr, 10, 32)
	if err != nil {
		log.Debugf("解析端口号失败: %s", err)
		return nil, err
	}
	// 解析 IP 地址
	ip := net.ParseIP(host)
	if ip == nil {
		log.Debugf("IP 地址解析失败")
		return nil, errors.New("IP 地址解析失败")
	}
	// 转换为 WebTransport 多地址
	return toWebtransportMultiaddr(&net.UDPAddr{IP: ip, Port: int(port)})
}

// extractCertHashes 从多地址中提取证书哈希值
// 参数:
//   - addr: ma.Multiaddr 多地址对象
//
// 返回值:
//   - []multihash.DecodedMultihash 解码后的证书哈希值列表
//   - error 提取过程中的错误
func extractCertHashes(addr ma.Multiaddr) ([]multihash.DecodedMultihash, error) {
	// 存储证书哈希字符串
	certHashesStr := make([]string, 0, 2)
	// 遍历多地址的每个组件
	ma.ForEach(addr, func(c ma.Component) bool {
		if c.Protocol().Code == ma.P_CERTHASH {
			certHashesStr = append(certHashesStr, c.Value())
		}
		return true
	})
	// 解码证书哈希值
	certHashes := make([]multihash.DecodedMultihash, 0, len(certHashesStr))
	for _, s := range certHashesStr {
		_, ch, err := multibase.Decode(s)
		if err != nil {
			log.Debugf("证书哈希的 multibase 解码失败: %s", err)
			return nil, err
		}
		dh, err := multihash.Decode(ch)
		if err != nil {
			log.Debugf("证书哈希的 multihash 解码失败: %s", err)
			return nil, err
		}
		certHashes = append(certHashes, *dh)
	}
	return certHashes, nil
}

// addrComponentForCert 为证书创建多地址组件
// 参数:
//   - hash: []byte 证书哈希值
//
// 返回值:
//   - ma.Multiaddr 包含证书哈希的多地址组件
//   - error 创建过程中的错误
func addrComponentForCert(hash []byte) (ma.Multiaddr, error) {
	// 对哈希值进行 multihash 编码
	mh, err := multihash.Encode(hash, multihash.SHA2_256)
	if err != nil {
		log.Debugf("证书哈希的 multihash 编码失败: %s", err)
		return nil, err
	}
	// 对编码后的哈希值进行 multibase 编码
	certStr, err := multibase.Encode(multibase.Base58BTC, mh)
	if err != nil {
		log.Debugf("证书哈希的 multibase 编码失败: %s", err)
		return nil, err
	}
	// 创建多地址组件
	return ma.NewComponent(ma.ProtocolWithCode(ma.P_CERTHASH).Name, certStr)
}

// IsWebtransportMultiaddr 检查给定的多地址是否为格式正确的 WebTransport 多地址
// 参数:
//   - multiaddr: ma.Multiaddr 待检查的多地址
//
// 返回值:
//   - bool 是否为有效的 WebTransport 多地址
//   - int 找到的证书哈希数量
func IsWebtransportMultiaddr(multiaddr ma.Multiaddr) (bool, int) {
	const (
		init              = iota // 初始状态
		foundUDP                 // 找到 UDP 协议
		foundQuicV1              // 找到 QUIC-V1 协议
		foundWebTransport        // 找到 WebTransport 协议
	)
	state := init
	certhashCount := 0

	// 遍历多地址的每个组件
	ma.ForEach(multiaddr, func(c ma.Component) bool {
		switch c.Protocol().Code {
		case ma.P_UDP:
			if state == init {
				state = foundUDP
			}
		case ma.P_QUIC_V1:
			if state == foundUDP {
				state = foundQuicV1
			}
		case ma.P_WEBTRANSPORT:
			if state == foundQuicV1 {
				state = foundWebTransport
			}
		case ma.P_CERTHASH:
			if state == foundWebTransport {
				certhashCount++
			}
		}
		return true
	})
	return state == foundWebTransport, certhashCount
}
