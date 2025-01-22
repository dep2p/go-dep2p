package websocket

import (
	"fmt"
	"net"
	"net/url"
	"strconv"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

// Addr 是 WebSocket 的 net.Addr 实现
type Addr struct {
	*url.URL
}

var _ net.Addr = (*Addr)(nil)

// Network 返回 WebSocket 的网络类型 "websocket"
// 返回值:
//   - string: 固定返回 "websocket" 字符串
func (addr *Addr) Network() string {
	return "websocket"
}

// NewAddr 创建一个使用 `ws` 协议(不安全)的 Addr
//
// 已弃用。请使用 NewAddrWithScheme。
// 参数:
//   - host: string 主机地址
//
// 返回值:
//   - *Addr: 新创建的 WebSocket 地址对象
func NewAddr(host string) *Addr {
	// 旧版本的传输层只支持不安全连接(即 WS 而不是 WSS)。这里假设使用不安全连接。
	return NewAddrWithScheme(host, false)
}

// NewAddrWithScheme 使用给定的主机字符串创建新的 Addr
// isSecure 参数在 WSS 连接时应为 true，WS 连接时为 false
// 参数:
//   - host: string 主机地址
//   - isSecure: bool 是否使用安全连接
//
// 返回值:
//   - *Addr: 新创建的 WebSocket 地址对象
func NewAddrWithScheme(host string, isSecure bool) *Addr {
	scheme := "ws"
	if isSecure {
		scheme = "wss"
	}
	return &Addr{
		URL: &url.URL{
			Scheme: scheme,
			Host:   host,
		},
	}
}

// ConvertWebsocketMultiaddrToNetAddr 将 WebSocket 多地址转换为网络地址
// 参数:
//   - maddr: ma.Multiaddr WebSocket 多地址
//
// 返回值:
//   - net.Addr: 转换后的网络地址
//   - error: 转换过程中的错误
func ConvertWebsocketMultiaddrToNetAddr(maddr ma.Multiaddr) (net.Addr, error) {
	url, err := parseMultiaddr(maddr)
	if err != nil {
		log.Debugf("解析多地址失败: %s", err)
		return nil, err
	}
	return &Addr{URL: url}, nil
}

// ParseWebsocketNetAddr 将网络地址解析为 WebSocket 多地址
// 参数:
//   - a: net.Addr 网络地址
//
// 返回值:
//   - ma.Multiaddr: 解析后的多地址
//   - error: 解析过程中的错误
func ParseWebsocketNetAddr(a net.Addr) (ma.Multiaddr, error) {
	wsa, ok := a.(*Addr)
	if !ok {
		return nil, fmt.Errorf("不是一个 websocket 地址")
	}

	var (
		tcpma ma.Multiaddr
		err   error
		port  int
		host  = wsa.Hostname()
	)

	// 获取端口
	if portStr := wsa.Port(); portStr != "" {
		port, err = strconv.Atoi(portStr)
		if err != nil {
			return nil, fmt.Errorf("解析端口 '%q' 失败: %s", portStr, err)
		}
	} else {
		return nil, fmt.Errorf("URL 中的端口无效: '%q'", wsa.URL)
	}

	// 注意：忽略 IPv6 区域...
	// 检测主机是 IP 地址还是 DNS
	if ip := net.ParseIP(host); ip != nil {
		// 假设是 IP 地址
		tcpma, err = manet.FromNetAddr(&net.TCPAddr{
			IP:   ip,
			Port: port,
		})
		if err != nil {
			return nil, err
		}
	} else {
		// 假设是 DNS 名称
		tcpma, err = ma.NewMultiaddr(fmt.Sprintf("/dns/%s/tcp/%d", host, port))
		if err != nil {
			return nil, err
		}
	}

	wsma, err := ma.NewMultiaddr("/" + wsa.Scheme)
	if err != nil {
		return nil, err
	}

	return tcpma.Encapsulate(wsma), nil
}

// parseMultiaddr 解析多地址为 URL
// 参数:
//   - maddr: ma.Multiaddr 待解析的多地址
//
// 返回值:
//   - *url.URL: 解析后的 URL
//   - error: 解析过程中的错误
func parseMultiaddr(maddr ma.Multiaddr) (*url.URL, error) {
	parsed, err := parseWebsocketMultiaddr(maddr)
	if err != nil {
		return nil, err
	}

	scheme := "ws"
	if parsed.isWSS {
		scheme = "wss"
	}

	network, host, err := manet.DialArgs(parsed.restMultiaddr)
	if err != nil {
		return nil, err
	}
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return nil, fmt.Errorf("不支持的 websocket 网络类型 %s", network)
	}
	return &url.URL{
		Scheme: scheme,
		Host:   host,
	}, nil
}

// parsedWebsocketMultiaddr 表示解析后的 WebSocket 多地址结构
type parsedWebsocketMultiaddr struct {
	isWSS         bool          // 是否是 WSS 连接
	sni           *ma.Component // TLS 握手的 SNI 值，用于设置 HTTP Host 头
	restMultiaddr ma.Multiaddr  // 在 /tls/sni/example.com/ws 或 /ws 或 /wss 之前的多地址部分
}

// parseWebsocketMultiaddr 解析 WebSocket 多地址
// 参数:
//   - a: ma.Multiaddr 待解析的多地址
//
// 返回值:
//   - parsedWebsocketMultiaddr: 解析后的 WebSocket 多地址结构
//   - error: 解析过程中的错误
func parseWebsocketMultiaddr(a ma.Multiaddr) (parsedWebsocketMultiaddr, error) {
	out := parsedWebsocketMultiaddr{}
	// 首先检查是否有 WSS 组件。如果有，我们将其规范化为 /tls/ws
	withoutWss := a.Decapsulate(wssComponent)
	if !withoutWss.Equal(a) {
		a = withoutWss.Encapsulate(tlsWsComponent)
	}

	// 移除 ws 组件
	withoutWs := a.Decapsulate(wsComponent)
	if withoutWs.Equal(a) {
		return out, fmt.Errorf("不是一个 websocket 多地址")
	}

	rest := withoutWs
	// 如果这不是 wss，则 withoutWs 是多地址的其余部分
	out.restMultiaddr = withoutWs
	for {
		var head *ma.Component
		rest, head = ma.SplitLast(rest)
		if head == nil || rest == nil {
			break
		}

		if head.Protocol().Code == ma.P_SNI {
			out.sni = head
		} else if head.Protocol().Code == ma.P_TLS {
			out.isWSS = true
			out.restMultiaddr = rest
			break
		}
	}

	return out, nil
}
