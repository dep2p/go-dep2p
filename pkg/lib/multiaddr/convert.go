package multiaddr

import (
	"fmt"
	"net"
)

// ToTCPAddr 将多地址转换为 *net.TCPAddr
func (m *multiaddr) ToTCPAddr() (*net.TCPAddr, error) {
	// 获取 IP 地址
	var ipStr string
	var err error

	// 尝试 IPv4
	ipStr, err = m.ValueForProtocol(P_IP4)
	if err != nil {
		// 尝试 IPv6
		ipStr, err = m.ValueForProtocol(P_IP6)
		if err != nil {
			return nil, fmt.Errorf("no IP address in multiaddr")
		}
	}

	// 解析 IP
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address: %s", ipStr)
	}

	// 解析端口
	port, err := m.ValueForProtocol(P_TCP)
	if err != nil {
		return nil, err
	}

	// port 是字符串，需要转换为整数
	var portNum int
	_, err = fmt.Sscanf(port, "%d", &portNum)
	if err != nil {
		return nil, fmt.Errorf("invalid port: %s", port)
	}

	return &net.TCPAddr{
		IP:   ip,
		Port: portNum,
	}, nil
}

// ToUDPAddr 将多地址转换为 *net.UDPAddr
func (m *multiaddr) ToUDPAddr() (*net.UDPAddr, error) {
	// 获取 IP 地址
	var ipStr string
	var err error

	// 尝试 IPv4
	ipStr, err = m.ValueForProtocol(P_IP4)
	if err != nil {
		// 尝试 IPv6
		ipStr, err = m.ValueForProtocol(P_IP6)
		if err != nil {
			return nil, fmt.Errorf("no IP address in multiaddr")
		}
	}

	// 获取 UDP 端口
	portStr, err := m.ValueForProtocol(P_UDP)
	if err != nil {
		return nil, fmt.Errorf("no UDP port in multiaddr")
	}

	// 解析 IP
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address: %s", ipStr)
	}

	// 解析端口
	var portNum int
	_, err = fmt.Sscanf(portStr, "%d", &portNum)
	if err != nil {
		return nil, fmt.Errorf("invalid port: %s", portStr)
	}

	return &net.UDPAddr{
		IP:   ip,
		Port: portNum,
	}, nil
}

// FromTCPAddr 从 *net.TCPAddr 创建多地址
func FromTCPAddr(addr *net.TCPAddr) (Multiaddr, error) {
	if addr == nil {
		return nil, fmt.Errorf("nil TCP address")
	}

	// 确定 IP 版本
	ip := addr.IP
	var proto string
	if ip4 := ip.To4(); ip4 != nil {
		proto = "ip4"
		ip = ip4
	} else {
		proto = "ip6"
	}

	// 构造多地址字符串
	s := fmt.Sprintf("/%s/%s/tcp/%d", proto, ip.String(), addr.Port)
	return NewMultiaddr(s)
}

// FromUDPAddr 从 *net.UDPAddr 创建多地址
func FromUDPAddr(addr *net.UDPAddr) (Multiaddr, error) {
	if addr == nil {
		return nil, fmt.Errorf("nil UDP address")
	}

	// 确定 IP 版本
	ip := addr.IP
	var proto string
	if ip4 := ip.To4(); ip4 != nil {
		proto = "ip4"
		ip = ip4
	} else {
		proto = "ip6"
	}

	// 构造多地址字符串
	s := fmt.Sprintf("/%s/%s/udp/%d", proto, ip.String(), addr.Port)
	return NewMultiaddr(s)
}

// FromNetAddr 从 net.Addr 创建多地址
func FromNetAddr(addr net.Addr) (Multiaddr, error) {
	if addr == nil {
		return nil, fmt.Errorf("nil address")
	}

	switch a := addr.(type) {
	case *net.TCPAddr:
		return FromTCPAddr(a)
	case *net.UDPAddr:
		return FromUDPAddr(a)
	default:
		return nil, fmt.Errorf("unsupported address type: %T", addr)
	}
}
