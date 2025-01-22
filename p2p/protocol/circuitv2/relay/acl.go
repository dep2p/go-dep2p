package relay

import (
	"github.com/dep2p/go-dep2p/core/peer"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
)

// ACLFilter 是中继连接的访问控制机制接口
type ACLFilter interface {
	// AllowReserve 检查是否允许指定对等节点和地址的预留请求
	// 参数:
	//   - p: peer.ID 请求预留的对等节点ID
	//   - a: ma.Multiaddr 请求预留的多地址
	//
	// 返回值:
	//   - bool 如果允许预留则返回true,否则返回false
	AllowReserve(p peer.ID, a ma.Multiaddr) bool

	// AllowConnect 检查是否允许源节点连接到目标节点
	// 参数:
	//   - src: peer.ID 源节点ID
	//   - srcAddr: ma.Multiaddr 源节点的多地址
	//   - dest: peer.ID 目标节点ID
	//
	// 返回值:
	//   - bool 如果允许连接则返回true,否则返回false
	AllowConnect(src peer.ID, srcAddr ma.Multiaddr, dest peer.ID) bool
}
