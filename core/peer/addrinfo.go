package peer

import (
	"fmt"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// AddrInfo 是一个小型结构体，用于传递包含一组地址和对等节点ID的信息
type AddrInfo struct {
	// ID 表示对等节点的唯一标识符
	ID ID
	// Addrs 存储对等节点的多地址列表
	Addrs []ma.Multiaddr
}

var _ fmt.Stringer = AddrInfo{}

// String 实现 fmt.Stringer 接口，返回 AddrInfo 的字符串表示
// 返回值：
//   - string: 包含节点ID和地址列表的格式化字符串
func (pi AddrInfo) String() string {
	return fmt.Sprintf("{%v: %v}", pi.ID, pi.Addrs)
}

// ErrInvalidAddr 表示无效的 p2p 多地址错误
var ErrInvalidAddr = fmt.Errorf("无效的 p2p 多地址")

// AddrInfosFromP2pAddrs 将一组多地址转换为 AddrInfo 结构体集合
// 参数：
//   - maddrs: 可变参数，表示要转换的多地址列表
//
// 返回值：
//   - []AddrInfo: 转换后的 AddrInfo 结构体切片
//   - error: 如果转换过程中发生错误，返回相应的错误信息
func AddrInfosFromP2pAddrs(maddrs ...ma.Multiaddr) ([]AddrInfo, error) {
	// 创建映射以存储ID到地址的对应关系
	m := make(map[ID][]ma.Multiaddr)
	// 遍历所有多地址
	for _, maddr := range maddrs {
		transport, id := SplitAddr(maddr)
		if id == "" {
			log.Debugf("无效的p2p多地址: %v", maddr)
			return nil, ErrInvalidAddr
		}
		if transport == nil {
			if _, ok := m[id]; !ok {
				m[id] = nil
			}
		} else {
			m[id] = append(m[id], transport)
		}
	}
	// 构建结果切片
	ais := make([]AddrInfo, 0, len(m))
	for id, maddrs := range m {
		ais = append(ais, AddrInfo{ID: id, Addrs: maddrs})
	}
	return ais, nil
}

// SplitAddr 将 p2p 多地址分割为传输地址和对等节点ID
// 参数：
//   - m: ma.Multiaddr 要分割的多地址
//
// 返回值：
//   - transport: ma.Multiaddr 传输地址部分，如果地址仅包含 p2p 部分则返回 nil
//   - id: ID 对等节点ID，如果地址不包含 p2p 部分则返回空字符串
func SplitAddr(m ma.Multiaddr) (transport ma.Multiaddr, id ID) {
	if m == nil {
		return nil, ""
	}

	transport, p2ppart := ma.SplitLast(m)
	if p2ppart == nil || p2ppart.Protocol().Code != ma.P_P2P {
		return m, ""
	}
	id = ID(p2ppart.RawValue())
	return transport, id
}

// IDFromP2PAddr 从 p2p 多地址中提取对等节点ID
// 参数：
//   - m: ma.Multiaddr p2p 多地址
//
// 返回值：
//   - ID: 提取的对等节点ID
//   - error: 如果地址无效则返回错误
func IDFromP2PAddr(m ma.Multiaddr) (ID, error) {
	if m == nil {
		log.Debugf("无效的p2p多地址: %v", m)
		return "", ErrInvalidAddr
	}
	var lastComponent ma.Component
	ma.ForEach(m, func(c ma.Component) bool {
		lastComponent = c
		return true
	})
	if lastComponent.Protocol().Code != ma.P_P2P {
		log.Debugf("无效的p2p多地址: %v", m)
		return "", ErrInvalidAddr
	}

	id := ID(lastComponent.RawValue())
	return id, nil
}

// AddrInfoFromString 从多地址的字符串表示构建 AddrInfo
// 参数：
//   - s: string 多地址的字符串表示
//
// 返回值：
//   - *AddrInfo: 构建的 AddrInfo 指针
//   - error: 如果转换过程中发生错误，返回相应的错误信息
func AddrInfoFromString(s string) (*AddrInfo, error) {
	a, err := ma.NewMultiaddr(s)
	if err != nil {
		log.Debugf("创建多地址失败: %v", err)
		return nil, err
	}

	return AddrInfoFromP2pAddr(a)
}

// AddrInfoFromP2pAddr 将多地址转换为 AddrInfo
// 参数：
//   - m: ma.Multiaddr 要转换的多地址
//
// 返回值：
//   - *AddrInfo: 转换后的 AddrInfo 指针
//   - error: 如果转换过程中发生错误，返回相应的错误信息
func AddrInfoFromP2pAddr(m ma.Multiaddr) (*AddrInfo, error) {
	transport, id := SplitAddr(m)
	if id == "" {
		log.Debugf("地址 %s 不是p2p地址: %s", m, ErrInvalidAddr)
		return nil, ErrInvalidAddr
	}
	info := &AddrInfo{ID: id}
	if transport != nil {
		info.Addrs = []ma.Multiaddr{transport}
	}
	return info, nil
}

// AddrInfoToP2pAddrs 将 AddrInfo 转换为多地址列表
// 参数：
//   - pi: *AddrInfo 要转换的 AddrInfo 指针
//
// 返回值：
//   - []ma.Multiaddr: 转换后的多地址切片
//   - error: 如果转换过程中发生错误，返回相应的错误信息
func AddrInfoToP2pAddrs(pi *AddrInfo) ([]ma.Multiaddr, error) {
	p2ppart, err := ma.NewComponent("p2p", pi.ID.String())
	if err != nil {
		log.Debugf("创建p2p多地址失败: %v", err)
		return nil, err
	}
	if len(pi.Addrs) == 0 {
		return []ma.Multiaddr{p2ppart}, nil
	}
	addrs := make([]ma.Multiaddr, 0, len(pi.Addrs))
	for _, addr := range pi.Addrs {
		addrs = append(addrs, addr.Encapsulate(p2ppart))
	}
	return addrs, nil
}

// Loggable 实现日志接口，返回可记录的键值对映射
// 返回值：
//   - map[string]interface{}: 包含节点ID和地址信息的映射
func (pi *AddrInfo) Loggable() map[string]interface{} {
	return map[string]interface{}{
		"peerID": pi.ID.String(),
		"addrs":  pi.Addrs,
	}
}

// AddrInfosToIDs 从 AddrInfo 切片中提取对等节点ID列表
// 参数：
//   - pis: []AddrInfo AddrInfo 结构体切片
//
// 返回值：
//   - []ID: 按顺序提取的对等节点ID切片
func AddrInfosToIDs(pis []AddrInfo) []ID {
	ps := make([]ID, len(pis))
	for i, pi := range pis {
		ps[i] = pi.ID
	}
	return ps
}
