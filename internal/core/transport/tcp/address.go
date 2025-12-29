// Package tcp 提供基于 TCP 的传输层实现
package tcp

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
)

// ============================================================================
//                              地址格式
// ============================================================================

// 支持的地址格式：
// - /ip4/127.0.0.1/tcp/4001
// - /ip6/::1/tcp/4001
// - /dns4/example.com/tcp/4001
// - /dns6/example.com/tcp/4001

// ============================================================================
//                              Address 实现
// ============================================================================

// Address TCP 地址
type Address struct {
	network string // "tcp4", "tcp6", "tcp"
	host    string // IP 地址或域名
	port    int    // 端口号
}

// 确保实现接口
var _ endpoint.Address = (*Address)(nil)

// NewAddress 创建 TCP 地址
func NewAddress(network, host string, port int) *Address {
	return &Address{
		network: network,
		host:    host,
		port:    port,
	}
}

// NewAddressFromNetAddr 从 net.Addr 创建地址
func NewAddressFromNetAddr(addr net.Addr) (*Address, error) {
	tcpAddr, ok := addr.(*net.TCPAddr)
	if !ok {
		return nil, fmt.Errorf("不是 TCP 地址: %T", addr)
	}

	network := "tcp"
	if tcpAddr.IP.To4() != nil {
		network = "tcp4"
	} else if tcpAddr.IP.To16() != nil {
		network = "tcp6"
	}

	return &Address{
		network: network,
		host:    tcpAddr.IP.String(),
		port:    tcpAddr.Port,
	}, nil
}

// 地址解析正则
var (
	ip4TCPPattern  = regexp.MustCompile(`^/ip4/([^/]+)/tcp/(\d+)$`)
	ip6TCPPattern  = regexp.MustCompile(`^/ip6/([^/]+)/tcp/(\d+)$`)
	dns4TCPPattern = regexp.MustCompile(`^/dns4/([^/]+)/tcp/(\d+)$`)
	dns6TCPPattern = regexp.MustCompile(`^/dns6/([^/]+)/tcp/(\d+)$`)
)

// ParseAddress 解析地址字符串
func ParseAddress(addr string) (*Address, error) {
	// 尝试 IPv4
	if matches := ip4TCPPattern.FindStringSubmatch(addr); matches != nil {
		port, _ := strconv.Atoi(matches[2])
		return &Address{
			network: "tcp4",
			host:    matches[1],
			port:    port,
		}, nil
	}

	// 尝试 IPv6
	if matches := ip6TCPPattern.FindStringSubmatch(addr); matches != nil {
		port, _ := strconv.Atoi(matches[2])
		return &Address{
			network: "tcp6",
			host:    matches[1],
			port:    port,
		}, nil
	}

	// 尝试 DNS4
	if matches := dns4TCPPattern.FindStringSubmatch(addr); matches != nil {
		port, _ := strconv.Atoi(matches[2])
		return &Address{
			network: "tcp4",
			host:    matches[1],
			port:    port,
		}, nil
	}

	// 尝试 DNS6
	if matches := dns6TCPPattern.FindStringSubmatch(addr); matches != nil {
		port, _ := strconv.Atoi(matches[2])
		return &Address{
			network: "tcp6",
			host:    matches[1],
			port:    port,
		}, nil
	}

	return nil, fmt.Errorf("无效的 TCP 地址格式: %s", addr)
}

// MustParseAddress 解析地址，失败则 panic
//
// 注意：此函数通常用于初始化时已知不会失败的场景，或测试中。
// 在运行时关键路径中应避免使用，或确保错误被妥善处理。
func MustParseAddress(addr string) *Address {
	a, err := ParseAddress(addr)
	if err != nil {
		panic(fmt.Sprintf("解析 TCP 地址失败: %s, 错误: %v", addr, err))
	}
	return a
}

// ============================================================================
//                              endpoint.Address 接口实现
// ============================================================================

// Network 返回网络类型
func (a *Address) Network() string {
	return a.network
}

// String 返回多地址格式字符串
func (a *Address) String() string {
	if a.network == "tcp6" || strings.Contains(a.host, ":") {
		return fmt.Sprintf("/ip6/%s/tcp/%d", a.host, a.port)
	}
	return fmt.Sprintf("/ip4/%s/tcp/%d", a.host, a.port)
}

// Bytes 返回地址字节表示
func (a *Address) Bytes() []byte {
	return []byte(a.String())
}

// IsPublic 检查是否为公网地址
func (a *Address) IsPublic() bool {
	ip := net.ParseIP(a.host)
	if ip == nil {
		// 域名假设为公网
		return true
	}
	return !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsLinkLocalUnicast()
}

// IsPrivate 检查是否为私网地址
func (a *Address) IsPrivate() bool {
	ip := net.ParseIP(a.host)
	if ip == nil {
		return false
	}
	return ip.IsPrivate()
}

// IsLoopback 检查是否为回环地址
func (a *Address) IsLoopback() bool {
	ip := net.ParseIP(a.host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

// Multiaddr 返回 multiaddr 格式字符串
func (a *Address) Multiaddr() string {
	ipType := "ip4"
	if ip := net.ParseIP(a.host); ip != nil && ip.To4() == nil {
		ipType = "ip6"
	}
	return fmt.Sprintf("/%s/%s/tcp/%d", ipType, a.host, a.port)
}

// Equal 比较两个地址是否相等
func (a *Address) Equal(other endpoint.Address) bool {
	if other == nil {
		return false
	}
	otherTCP, ok := other.(*Address)
	if !ok {
		return a.String() == other.String()
	}
	return a.host == otherTCP.host && a.port == otherTCP.port
}

// ============================================================================
//                              辅助方法
// ============================================================================

// Host 返回主机地址
func (a *Address) Host() string {
	return a.host
}

// Port 返回端口号
func (a *Address) Port() int {
	return a.port
}

// ToNetAddr 转换为 net.Addr
func (a *Address) ToNetAddr() (*net.TCPAddr, error) {
	ip := net.ParseIP(a.host)
	if ip == nil {
		// 尝试解析域名
		addrs, err := net.LookupIP(a.host)
		if err != nil {
			return nil, fmt.Errorf("解析域名失败: %w", err)
		}
		if len(addrs) == 0 {
			return nil, fmt.Errorf("域名 %s 无法解析", a.host)
		}
		ip = addrs[0]
	}

	return &net.TCPAddr{
		IP:   ip,
		Port: a.port,
	}, nil
}

// NetDialString 返回 net.Dial 使用的地址字符串
func (a *Address) NetDialString() string {
	if strings.Contains(a.host, ":") {
		// IPv6 需要括号
		return fmt.Sprintf("[%s]:%d", a.host, a.port)
	}
	return fmt.Sprintf("%s:%d", a.host, a.port)
}

