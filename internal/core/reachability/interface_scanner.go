package reachability

import (
	"net"
)

// InterfaceScanner 网络接口扫描器
//
// 扫描本机所有网络接口，找出绑定在网卡上的公网 IP 地址。
// 这适用于云服务器直接绑定公网 IP 的场景（如 AWS EC2、阿里云 ECS 等）。
type InterfaceScanner struct{}

// NewInterfaceScanner 创建网络接口扫描器
func NewInterfaceScanner() *InterfaceScanner {
	return &InterfaceScanner{}
}

// DiscoverPublicIPs 发现本机公网接口 IP
//
// 遍历所有网络接口，筛选出公网 IP 地址。
// 返回的 IP 列表不包含私网地址、回环地址、链路本地地址等。
func (s *InterfaceScanner) DiscoverPublicIPs() []net.IP {
	var publicIPs []net.IP

	ifaces, err := net.Interfaces()
	if err != nil {
		logger.Debug("获取网络接口失败", "err", err)
		return publicIPs
	}

	for _, iface := range ifaces {
		// 跳过回环接口
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		// 跳过未启用的接口
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			logger.Debug("获取接口地址失败", "iface", iface.Name, "err", err)
			continue
		}

		for _, addr := range addrs {
			ip := extractIPFromNetAddr(addr)
			if ip == nil {
				continue
			}

			// 检查是否是公网 IP
			if IsPublicIP(ip) {
				publicIPs = append(publicIPs, ip)
				logger.Debug("发现本机公网 IP", "ip", ip.String(), "iface", iface.Name)
			}
		}
	}

	return publicIPs
}

// DiscoverPrivateIPs 发现本机私网接口 IP
//
// 遍历所有网络接口，筛选出私网 IP 地址（RFC1918）。
// 这些地址可用于局域网内的节点发现。
func (s *InterfaceScanner) DiscoverPrivateIPs() []net.IP {
	var privateIPs []net.IP

	ifaces, err := net.Interfaces()
	if err != nil {
		logger.Debug("获取网络接口失败", "err", err)
		return privateIPs
	}

	for _, iface := range ifaces {
		// 跳过回环接口
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		// 跳过未启用的接口
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ip := extractIPFromNetAddr(addr)
			if ip == nil {
				continue
			}

			// 检查是否是私网 IP
			if ip.IsPrivate() && ip.IsGlobalUnicast() {
				privateIPs = append(privateIPs, ip)
				logger.Debug("发现本机私网 IP", "ip", ip.String(), "iface", iface.Name)
			}
		}
	}

	return privateIPs
}

// DiscoverAllUsableIPs 发现所有可用 IP（公网 + 私网）
//
// 返回所有可用于监听的 IP 地址，排除回环地址和链路本地地址。
func (s *InterfaceScanner) DiscoverAllUsableIPs() []net.IP {
	var usableIPs []net.IP

	ifaces, err := net.Interfaces()
	if err != nil {
		return usableIPs
	}

	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ip := extractIPFromNetAddr(addr)
			if ip == nil {
				continue
			}

			// 排除不可用的地址
			if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
				continue
			}

			// 只保留全局单播地址
			if ip.IsGlobalUnicast() {
				usableIPs = append(usableIPs, ip)
			}
		}
	}

	return usableIPs
}

// extractIPFromNetAddr 从网络地址中提取 IP
func extractIPFromNetAddr(addr net.Addr) net.IP {
	switch v := addr.(type) {
	case *net.IPNet:
		return v.IP
	case *net.IPAddr:
		return v.IP
	default:
		return nil
	}
}

// IsPublicIP 判断 IP 是否是公网地址
//
// 公网地址需要满足：
//   - 是全局单播地址
//   - 不是回环地址
//   - 不是链路本地地址
//   - 不是私网地址（RFC1918）
//   - 不是保留地址
func IsPublicIP(ip net.IP) bool {
	if ip == nil {
		return false
	}

	// 必须是全局单播地址
	if !ip.IsGlobalUnicast() {
		return false
	}

	// 排除回环地址
	if ip.IsLoopback() {
		return false
	}

	// 排除链路本地地址
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}

	// 排除私网地址
	if ip.IsPrivate() {
		return false
	}

	// IPv4 额外检查
	if ip4 := ip.To4(); ip4 != nil {
		// 排除 0.0.0.0/8 (当前网络)
		if ip4[0] == 0 {
			return false
		}

		// 排除 127.0.0.0/8 (回环)
		if ip4[0] == 127 {
			return false
		}

		// 排除 169.254.0.0/16 (链路本地)
		if ip4[0] == 169 && ip4[1] == 254 {
			return false
		}

		// 排除 100.64.0.0/10 (运营商级 NAT，CGNAT)
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return false
		}

		// 排除 192.0.0.0/24 (IETF 协议分配)
		if ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 0 {
			return false
		}

		// 排除 192.0.2.0/24 (文档用途，TEST-NET-1)
		if ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 2 {
			return false
		}

		// 排除 198.51.100.0/24 (文档用途，TEST-NET-2)
		if ip4[0] == 198 && ip4[1] == 51 && ip4[2] == 100 {
			return false
		}

		// 排除 203.0.113.0/24 (文档用途，TEST-NET-3)
		if ip4[0] == 203 && ip4[1] == 0 && ip4[2] == 113 {
			return false
		}

		// 排除 198.18.0.0/15 (基准测试)
		if ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19) {
			return false
		}

		// 排除 224.0.0.0/4 (多播)
		if ip4[0] >= 224 && ip4[0] <= 239 {
			return false
		}

		// 排除 240.0.0.0/4 (保留)
		if ip4[0] >= 240 {
			return false
		}

		// 排除广播地址
		if ip4[0] == 255 && ip4[1] == 255 && ip4[2] == 255 && ip4[3] == 255 {
			return false
		}
	}

	// IPv6 额外检查
	if ip6 := ip.To16(); ip6 != nil && ip.To4() == nil {
		// 排除 ::1 (回环)
		if ip.IsLoopback() {
			return false
		}

		// 排除 fe80::/10 (链路本地)
		if ip6[0] == 0xfe && (ip6[1]&0xc0) == 0x80 {
			return false
		}

		// 排除 fc00::/7 (唯一本地地址，ULA)
		if ip6[0] == 0xfc || ip6[0] == 0xfd {
			return false
		}

		// 排除 ff00::/8 (多播)
		if ip6[0] == 0xff {
			return false
		}
	}

	return true
}

// IsPrivateIP 判断是否是私网 IP（RFC1918）
func IsPrivateIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return ip.IsPrivate()
}
