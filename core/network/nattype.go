package network

// NATDeviceType 表示 NAT 设备的类型
type NATDeviceType int

const (
	// NATDeviceTypeUnknown 表示 NAT 设备类型未知
	NATDeviceTypeUnknown NATDeviceType = iota

	// NATDeviceTypeCone 表示 NAT 设备是一个锥形 NAT
	// 锥形 NAT 会将来自相同源 IP 地址和端口的所有出站连接映射到相同的 IP 地址和端口,而不考虑目标地址
	// 根据 RFC 3489,这可能是完全锥形 NAT、受限锥形 NAT 或端口受限锥形 NAT
	// 但是,我们在这里不区分它们,而是简单地将所有此类 NAT 归类为锥形 NAT
	// 只有当远程对等点也在锥形 NAT 后面时,才可以通过打洞实现 NAT 穿透
	// 如果远程对等点在对称 NAT 后面,打洞将失败
	NATDeviceTypeCone

	// NATDeviceTypeSymmetric 表示 NAT 设备是一个对称 NAT
	// 对称 NAT 会将具有不同目标地址的出站连接映射到不同的 IP 地址和端口,即使它们来自相同的源 IP 地址和端口
	// 在 dep2p 中,无论远程对等点的 NAT 类型如何,目前都无法通过打洞实现对称 NAT 的穿透
	NATDeviceTypeSymmetric
)

// String 返回 NATDeviceType 的字符串表示
// 返回值:
//   - string: NAT 设备类型的字符串描述
func (r NATDeviceType) String() string {
	switch r {
	case 0:
		return "Unknown"
	case 1:
		return "Cone"
	case 2:
		return "Symmetric"
	default:
		return "unrecognized"
	}
}

// NATTransportProtocol 表示确定 NAT 设备类型的传输协议
type NATTransportProtocol int

const (
	// NATTransportUDP 表示已为 UDP 协议确定 NAT 设备类型
	NATTransportUDP NATTransportProtocol = iota
	// NATTransportTCP 表示已为 TCP 协议确定 NAT 设备类型
	NATTransportTCP
)

// String 返回 NATTransportProtocol 的字符串表示
// 返回值:
//   - string: NAT 传输协议的字符串描述
func (n NATTransportProtocol) String() string {
	switch n {
	case 0:
		return "UDP"
	case 1:
		return "TCP"
	default:
		return "unrecognized"
	}
}
