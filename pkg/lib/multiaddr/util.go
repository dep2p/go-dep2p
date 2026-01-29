package multiaddr

import (
	"fmt"
	"strings"
)

// Split 分离传输地址和 P2P 组件
// 输入：/ip4/1.2.3.4/tcp/4001/p2p/12D3KooW...
// 输出：/ip4/1.2.3.4/tcp/4001, 12D3KooW...
func Split(m Multiaddr) (transport Multiaddr, peerID string) {
	if m == nil {
		return nil, ""
	}

	s := m.String()

	// 查找 /p2p/ 组件
	idx := strings.Index(s, "/p2p/")
	if idx < 0 {
		// 没有 P2P 组件
		return m, ""
	}

	// 分离传输地址
	transportStr := s[:idx]
	if transportStr == "" {
		transport = nil
	} else {
		transport, _ = NewMultiaddr(transportStr)
	}

	// 提取 PeerID
	peerID = s[idx+5:] // 跳过 "/p2p/"

	// 如果有更多组件，只取到下一个 /
	if nextSlash := strings.Index(peerID, "/"); nextSlash > 0 {
		peerID = peerID[:nextSlash]
	}

	return transport, peerID
}

// Join 合并传输地址和 P2P 组件
func Join(transport Multiaddr, peerID string) Multiaddr {
	if peerID == "" {
		return transport
	}

	p2pAddr, err := NewMultiaddr(fmt.Sprintf("/p2p/%s", peerID))
	if err != nil {
		// 如果无法创建 P2P 地址，只返回传输地址
		return transport
	}

	if transport == nil {
		return p2pAddr
	}

	result := transport.Encapsulate(p2pAddr)
	return result
}

// FilterAddrs 过滤多地址列表
func FilterAddrs(addrs []Multiaddr, filter func(Multiaddr) bool) []Multiaddr {
	result := make([]Multiaddr, 0, len(addrs))
	for _, addr := range addrs {
		if filter(addr) {
			result = append(result, addr)
		}
	}
	return result
}

// UniqueAddrs 去重多地址列表（保持顺序）
func UniqueAddrs(addrs []Multiaddr) []Multiaddr {
	seen := make(map[string]bool)
	result := make([]Multiaddr, 0, len(addrs))

	for _, addr := range addrs {
		s := addr.String()
		if !seen[s] {
			seen[s] = true
			result = append(result, addr)
		}
	}

	return result
}

// HasProtocol 检查多地址是否包含指定协议
func HasProtocol(m Multiaddr, code int) bool {
	if m == nil {
		return false
	}

	protocols := m.Protocols()
	for _, p := range protocols {
		if p.Code == code {
			return true
		}
	}
	return false
}

// IsTCPMultiaddr 检查是否为 TCP 多地址
func IsTCPMultiaddr(m Multiaddr) bool {
	return HasProtocol(m, P_TCP)
}

// IsUDPMultiaddr 检查是否为 UDP 多地址
func IsUDPMultiaddr(m Multiaddr) bool {
	return HasProtocol(m, P_UDP)
}

// IsIP4Multiaddr 检查是否包含 IPv4
func IsIP4Multiaddr(m Multiaddr) bool {
	return HasProtocol(m, P_IP4)
}

// IsIP6Multiaddr 检查是否包含 IPv6
func IsIP6Multiaddr(m Multiaddr) bool {
	return HasProtocol(m, P_IP6)
}

// IsIPMultiaddr 检查是否包含 IP（IPv4 或 IPv6）
func IsIPMultiaddr(m Multiaddr) bool {
	return IsIP4Multiaddr(m) || IsIP6Multiaddr(m)
}

// GetPeerID 从多地址中提取 PeerID（如果有）
func GetPeerID(m Multiaddr) (string, error) {
	if m == nil {
		return "", fmt.Errorf("nil multiaddr")
	}

	_, peerID := Split(m)
	if peerID == "" {
		return "", fmt.Errorf("no peer ID in multiaddr")
	}

	return peerID, nil
}

// WithPeerID 为多地址添加或替换 PeerID
func WithPeerID(m Multiaddr, peerID string) (Multiaddr, error) {
	if m == nil {
		return nil, fmt.Errorf("nil multiaddr")
	}

	// 移除现有的 P2P 组件（如果有）
	transport, _ := Split(m)

	// 添加新的 PeerID
	return Join(transport, peerID), nil
}

// WithoutPeerID 移除多地址中的 PeerID
func WithoutPeerID(m Multiaddr) Multiaddr {
	if m == nil {
		return nil
	}

	transport, _ := Split(m)
	return transport
}

// Component 表示多地址组件
type Component struct {
	protocol Protocol
	value    string
}

// Protocol 返回组件的协议
func (c Component) Protocol() Protocol {
	return c.protocol
}

// Value 返回组件的值
func (c Component) Value() string {
	return c.value
}

// SplitFirst 分离多地址的第一个组件和剩余部分
func SplitFirst(m Multiaddr) (Component, Multiaddr) {
	if m == nil {
		return Component{}, nil
	}

	bytes := m.Bytes()
	if len(bytes) == 0 {
		return Component{}, nil
	}

	// 读取协议代码
	code, n, err := readVarintCode(bytes)
	if err != nil || n == 0 {
		return Component{}, nil
	}

	proto := ProtocolWithCode(code)
	if proto.Code == 0 {
		return Component{}, nil
	}

	// 读取值
	offset := n
	var value string
	if proto.Size > 0 {
		// 固定大小
		size := (proto.Size + 7) / 8
		if len(bytes) < offset+size {
			return Component{}, nil
		}
		if proto.Transcoder != nil {
			value, _ = proto.Transcoder.BytesToString(bytes[offset : offset+size])
		}
		offset += size
	} else if proto.Size == LengthPrefixedVarSize {
		// 变长
		lengthU64, n2, err := uvarintDecode(bytes[offset:])
		if err != nil || n2 == 0 {
			return Component{}, nil
		}
		length := int(lengthU64)
		offset += n2
		if len(bytes) < offset+length {
			return Component{}, nil
		}
		if proto.Transcoder != nil {
			value, _ = proto.Transcoder.BytesToString(bytes[offset : offset+length])
		}
		offset += length
	}

	// 构建第一个组件
	comp := Component{
		protocol: proto,
		value:    value,
	}

	// 剩余部分
	var rest Multiaddr
	if offset < len(bytes) {
		rest, _ = NewMultiaddrBytes(bytes[offset:])
	}

	return comp, rest
}

// ForEach 遍历多地址中的每个组件
// 如果回调函数返回 false，则停止遍历
func ForEach(m Multiaddr, fn func(Component) bool) {
	if m == nil {
		return
	}

	current := m
	for current != nil {
		comp, rest := SplitFirst(current)
		if comp.protocol.Code == 0 {
			break
		}

		if !fn(comp) {
			break
		}

		current = rest
	}
}
