// Package address 提供地址管理模块的实现
package address

import (
	"github.com/dep2p/go-dep2p/pkg/interfaces/netaddr"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Addr - 唯一的 netaddr.Address 实现
// ============================================================================

// Addr 是 netaddr.Address 接口的唯一实现
//
// Addr 封装 types.Multiaddr，提供统一的地址操作接口。
// 所有散落在各模块的 Address 实现都应迁移到此类型。
//
// 设计原则：
//   - 内部存储 canonical multiaddr（types.Multiaddr）
//   - 实现 netaddr.Address 接口
//   - 不复制 types.Multiaddr 的方法，仅封装
type Addr struct {
	ma types.Multiaddr
}

// 确保 Addr 实现 netaddr.Address 接口
var _ netaddr.Address = (*Addr)(nil)

// ============================================================================
//                              构造函数
// ============================================================================

// NewAddr 从 types.Multiaddr 创建 Addr
//
// 这是创建 Addr 的首选方式。
func NewAddr(ma types.Multiaddr) *Addr {
	return &Addr{ma: ma}
}

// Parse 从字符串解析 Addr
//
// 仅接受 multiaddr 格式（以 "/" 开头）。
// host:port 格式应在 CLI/UI 边界层使用 types.FromHostPort 转换后再进入 core。
func Parse(s string) (*Addr, error) {
	ma, err := types.ParseMultiaddr(s)
	if err != nil {
		return nil, err
	}
	return &Addr{ma: ma}, nil
}

// MustParse 从字符串解析 Addr，失败时 panic
//
// 仅用于常量初始化或测试代码。
func MustParse(s string) *Addr {
	ma := types.MustParseMultiaddr(s)
	return &Addr{ma: ma}
}

// ============================================================================
//                              netaddr.Address 接口实现
// ============================================================================

// Network 返回网络类型
//
// 返回值：
//   - "ip4", "ip6": 直连地址
//   - "p2p-circuit": 中继地址
//   - "quic-v1", "tcp", "udp": 传输协议
func (a *Addr) Network() string {
	return a.ma.Network()
}

// String 返回 canonical multiaddr 字符串
func (a *Addr) String() string {
	return a.ma.String()
}

// Bytes 返回地址的字节表示
func (a *Addr) Bytes() []byte {
	return a.ma.Bytes()
}

// Equal 比较两个地址是否相等
func (a *Addr) Equal(other netaddr.Address) bool {
	if other == nil {
		return false
	}
	return a.ma.String() == other.String()
}

// IsPublic 是否是公网地址
func (a *Addr) IsPublic() bool {
	return a.ma.IsPublic()
}

// IsPrivate 是否是私网地址
func (a *Addr) IsPrivate() bool {
	return a.ma.IsPrivate()
}

// IsLoopback 是否是回环地址
func (a *Addr) IsLoopback() bool {
	return a.ma.IsLoopback()
}

// Multiaddr 返回标准 multiaddr 格式的地址字符串
func (a *Addr) Multiaddr() string {
	return a.ma.String()
}

// ============================================================================
//                              扩展方法（不在接口中）
// ============================================================================

// MA 返回内部的 types.Multiaddr
//
// 用于需要直接访问 Multiaddr 方法的场景。
func (a *Addr) MA() types.Multiaddr {
	return a.ma
}

// IsRelay 是否是中继地址
func (a *Addr) IsRelay() bool {
	return a.ma.IsRelay()
}

// PeerID 返回嵌入的 NodeID（如果有）
func (a *Addr) PeerID() types.NodeID {
	return a.ma.PeerID()
}

// WithPeerID 附加 /p2p/<nodeID> 组件
func (a *Addr) WithPeerID(nodeID types.NodeID) *Addr {
	return &Addr{ma: a.ma.WithPeerID(nodeID)}
}

// WithoutPeerID 移除 /p2p/<nodeID> 组件
func (a *Addr) WithoutPeerID() *Addr {
	return &Addr{ma: a.ma.WithoutPeerID()}
}

// IsEmpty 是否为空
func (a *Addr) IsEmpty() bool {
	return a == nil || a.ma.IsEmpty()
}

// ============================================================================
//                              Relay 地址操作
// ============================================================================

// BuildRelayAddr 构建中继地址
func (a *Addr) BuildRelayAddr(destID types.NodeID) (*Addr, error) {
	relayMA, err := a.ma.BuildRelayAddr(destID)
	if err != nil {
		return nil, err
	}
	return &Addr{ma: relayMA}, nil
}

// RelayID 返回中继节点 ID
func (a *Addr) RelayID() types.NodeID {
	return a.ma.RelayID()
}

// DestID 返回目标节点 ID
func (a *Addr) DestID() types.NodeID {
	return a.ma.DestID()
}

// RelayBaseAddr 返回中继节点的基础地址
func (a *Addr) RelayBaseAddr() *Addr {
	return &Addr{ma: a.ma.RelayBaseAddr()}
}

// IsDialableRelayAddr 检查是否是可拨号的 relay 地址
func (a *Addr) IsDialableRelayAddr() bool {
	return a.ma.IsDialableRelayAddr()
}

// ============================================================================
//                              辅助函数
// ============================================================================

// ParseAddrs 从字符串切片解析 Addr 切片
func ParseAddrs(ss []string) ([]*Addr, error) {
	addrs := make([]*Addr, 0, len(ss))
	for _, s := range ss {
		addr, err := Parse(s)
		if err != nil {
			return nil, err
		}
		addrs = append(addrs, addr)
	}
	return addrs, nil
}

// ParseMultiaddrs 从 Multiaddr 切片创建 Addr 切片
func ParseMultiaddrs(mas []types.Multiaddr) []*Addr {
	addrs := make([]*Addr, 0, len(mas))
	for _, ma := range mas {
		addrs = append(addrs, NewAddr(ma))
	}
	return addrs
}

// AddrsToStrings 将 Addr 切片转换为字符串切片
func AddrsToStrings(addrs []*Addr) []string {
	ss := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		ss = append(ss, addr.String())
	}
	return ss
}

// AddrsToMultiaddrs 将 Addr 切片转换为 Multiaddr 切片
func AddrsToMultiaddrs(addrs []*Addr) []types.Multiaddr {
	mas := make([]types.Multiaddr, 0, len(addrs))
	for _, addr := range addrs {
		mas = append(mas, addr.MA())
	}
	return mas
}

// AddrsToNetaddrs 将 Addr 切片转换为 netaddr.Address 切片
func AddrsToNetaddrs(addrs []*Addr) []netaddr.Address {
	result := make([]netaddr.Address, 0, len(addrs))
	for _, addr := range addrs {
		result = append(result, addr)
	}
	return result
}

