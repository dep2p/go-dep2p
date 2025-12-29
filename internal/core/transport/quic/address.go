// Package quic 提供基于 QUIC 的传输层实现
package quic

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
)

// Address QUIC 地址实现
type Address struct {
	host    string
	port    int
	network string // "ip4" 或 "ip6"
}

// 确保实现 endpoint.Address 接口
var _ endpoint.Address = (*Address)(nil)

// NewAddress 从主机和端口创建地址
func NewAddress(host string, port int) *Address {
	network := "ip4"
	if ip := net.ParseIP(host); ip != nil {
		if ip.To4() == nil {
			network = "ip6"
		}
	}
	return &Address{
		host:    host,
		port:    port,
		network: network,
	}
}

// ParseAddress 从字符串解析地址
// 支持格式: "host:port", "[ipv6]:port"
func ParseAddress(addr string) (*Address, error) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("解析地址失败: %w", err)
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		return nil, fmt.Errorf("解析端口失败: %w", err)
	}

	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("端口号超出范围: %d", port)
	}

	return NewAddress(host, port), nil
}

// MustParseAddress 解析地址，失败则 panic
//
// 注意：此函数仅应在程序初始化阶段用于硬编码地址，
// 不应在运行时处理用户输入时使用。对于运行时地址解析，
// 请使用 ParseAddress 并正确处理错误。
func MustParseAddress(addr string) *Address {
	a, err := ParseAddress(addr)
	if err != nil {
		panic(fmt.Sprintf("MustParseAddress: %v", err))
	}
	return a
}

// FromNetAddr 从 net.Addr 创建地址
func FromNetAddr(addr net.Addr) (*Address, error) {
	switch a := addr.(type) {
	case *net.UDPAddr:
		return NewAddress(a.IP.String(), a.Port), nil
	case *net.TCPAddr:
		return NewAddress(a.IP.String(), a.Port), nil
	default:
		// 尝试从字符串解析
		return ParseAddress(addr.String())
	}
}

// Network 返回网络类型
func (a *Address) Network() string {
	return a.network
}

// String 返回地址字符串表示
func (a *Address) String() string {
	if a.network == "ip6" {
		return fmt.Sprintf("[%s]:%d", a.host, a.port)
	}
	return fmt.Sprintf("%s:%d", a.host, a.port)
}

// Bytes 返回地址的字节表示
func (a *Address) Bytes() []byte {
	return []byte(a.String())
}

// Equal 比较两个地址是否相等
func (a *Address) Equal(other endpoint.Address) bool {
	if other == nil {
		return false
	}
	otherAddr, ok := other.(*Address)
	if !ok {
		// 如果不是同类型，比较字符串表示
		return a.String() == other.String()
	}
	return a.host == otherAddr.host && a.port == otherAddr.port
}

// IsPublic 是否是公网地址
func (a *Address) IsPublic() bool {
	ip := net.ParseIP(a.host)
	if ip == nil {
		// 如果不是 IP，假设是域名，域名通常是公网
		return true
	}
	// 0.0.0.0 和 :: 是未指定地址，不是公网地址
	if ip.IsUnspecified() {
		return false
	}
	return !a.IsPrivate() && !a.IsLoopback()
}

// IsPrivate 是否是私网地址
func (a *Address) IsPrivate() bool {
	ip := net.ParseIP(a.host)
	if ip == nil {
		return false
	}
	return ip.IsPrivate()
}

// IsLoopback 是否是回环地址
func (a *Address) IsLoopback() bool {
	ip := net.ParseIP(a.host)
	if ip == nil {
		return a.host == "localhost"
	}
	return ip.IsLoopback()
}

// Host 返回主机部分
func (a *Address) Host() string {
	return a.host
}

// Port 返回端口
func (a *Address) Port() int {
	return a.port
}

// ToUDPAddr 转换为 net.UDPAddr
func (a *Address) ToUDPAddr() (*net.UDPAddr, error) {
	ip := net.ParseIP(a.host)
	if ip == nil {
		// 尝试解析域名
		ips, err := net.LookupIP(a.host)
		if err != nil || len(ips) == 0 {
			return nil, fmt.Errorf("无法解析主机 %s: %w", a.host, err)
		}
		ip = ips[0]
	}
	return &net.UDPAddr{
		IP:   ip,
		Port: a.port,
	}, nil
}

// ToNetAddr 转换为 net.Addr
func (a *Address) ToNetAddr() net.Addr {
	udpAddr, err := a.ToUDPAddr()
	if err != nil {
		return nil
	}
	return udpAddr
}

// Multiaddr 返回多地址格式字符串
// 格式: /ip4/127.0.0.1/udp/8000/quic-v1 或 /ip6/::1/udp/8000/quic-v1
func (a *Address) Multiaddr() string {
	return fmt.Sprintf("/%s/%s/udp/%d/quic-v1", a.network, a.host, a.port)
}

// ParseMultiaddr 从多地址格式解析
func ParseMultiaddr(multiaddr string) (*Address, error) {
	parts := strings.Split(multiaddr, "/")
	if len(parts) < 6 {
		return nil, fmt.Errorf("无效的多地址格式: %s", multiaddr)
	}

	// 预期格式: /ip4/host/udp/port/quic-v1 或 /ip6/host/udp/port/quic-v1
	if parts[0] != "" {
		return nil, fmt.Errorf("多地址必须以 / 开头")
	}

	network := parts[1]
	if network != "ip4" && network != "ip6" {
		return nil, fmt.Errorf("不支持的网络类型: %s", network)
	}

	host := parts[2]

	if parts[3] != "udp" {
		return nil, fmt.Errorf("QUIC 只支持 UDP 传输")
	}

	port, err := strconv.Atoi(parts[4])
	if err != nil {
		return nil, fmt.Errorf("无效的端口: %s", parts[4])
	}

	return &Address{
		host:    host,
		port:    port,
		network: network,
	}, nil
}

// ResolveAddresses 解析地址列表
// 如果地址是 0.0.0.0 或 ::，则展开为所有本地接口地址
func ResolveAddresses(addr *Address) ([]*Address, error) {
	if addr.host == "0.0.0.0" || addr.host == "::" {
		return getAllLocalAddresses(addr.port, addr.host == "::")
	}
	return []*Address{addr}, nil
}

// getAllLocalAddresses 获取所有本地接口地址
func getAllLocalAddresses(port int, ipv6 bool) ([]*Address, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("获取网络接口失败: %w", err)
	}

	var addresses []*Address
	for _, iface := range interfaces {
		// 跳过非活动和回环接口
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			if ip == nil || ip.IsLoopback() || ip.IsMulticast() || ip.IsUnspecified() {
				continue
			}

			// 根据 IPv6 标志过滤
			isIPv6 := ip.To4() == nil
			if ipv6 != isIPv6 {
				continue
			}

			addresses = append(addresses, NewAddress(ip.String(), port))
		}
	}

	return addresses, nil
}

