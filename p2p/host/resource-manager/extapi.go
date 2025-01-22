package rcmgr

import (
	"bytes"
	"sort"
	"strings"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
)

// ResourceScopeLimiter 是一个允许访问作用域限制的接口
type ResourceScopeLimiter interface {
	// Limit 获取当前限制
	// 返回值:
	//   - Limit: 当前限制
	Limit() Limit

	// SetLimit 设置新的限制
	// 参数:
	//   - Limit: 要设置的新限制
	SetLimit(Limit)
}

// 确保 resourceScope 实现了 ResourceScopeLimiter 接口
var _ ResourceScopeLimiter = (*resourceScope)(nil)

// ResourceManagerState 是一个允许访问资源管理器状态的接口
type ResourceManagerState interface {
	// ListServices 列出所有服务名称
	// 返回值:
	//   - []string: 服务名称列表
	ListServices() []string

	// ListProtocols 列出所有协议ID
	// 返回值:
	//   - []protocol.ID: 协议ID列表
	ListProtocols() []protocol.ID

	// ListPeers 列出所有对等节点ID
	// 返回值:
	//   - []peer.ID: 对等节点ID列表
	ListPeers() []peer.ID

	// Stat 获取资源管理器统计信息
	// 返回值:
	//   - ResourceManagerStat: 资源管理器统计信息
	Stat() ResourceManagerStat
}

// ResourceManagerStat 定义了资源管理器的统计信息结构
type ResourceManagerStat struct {
	System    network.ScopeStat                 // 系统级别统计
	Transient network.ScopeStat                 // 临时作用域统计
	Services  map[string]network.ScopeStat      // 服务级别统计
	Protocols map[protocol.ID]network.ScopeStat // 协议级别统计
	Peers     map[peer.ID]network.ScopeStat     // 对等节点级别统计
}

// 确保 resourceManager 实现了 ResourceManagerState 接口
var _ ResourceManagerState = (*resourceManager)(nil)

// Limit 获取资源作用域的当前限制
// 返回值:
//   - Limit: 当前限制
func (s *resourceScope) Limit() Limit {
	s.Lock()         // 加锁
	defer s.Unlock() // 解锁

	return s.rc.limit // 返回当前限制
}

// SetLimit 设置资源作用域的新限制
// 参数:
//   - limit: 要设置的新限制
func (s *resourceScope) SetLimit(limit Limit) {
	s.Lock()         // 加锁
	defer s.Unlock() // 解锁

	s.rc.limit = limit // 设置新限制
}

// SetLimit 设置协议作用域的新限制
// 参数:
//   - limit: 要设置的新限制
func (s *protocolScope) SetLimit(limit Limit) {
	s.rcmgr.setStickyProtocol(s.proto) // 设置协议为固定协议
	s.resourceScope.SetLimit(limit)    // 设置新限制
}

// SetLimit 设置对等节点作用域的新限制
// 参数:
//   - limit: 要设置的新限制
func (s *peerScope) SetLimit(limit Limit) {
	s.rcmgr.setStickyPeer(s.peer)   // 设置对等节点为固定节点
	s.resourceScope.SetLimit(limit) // 设置新限制
}

// ListServices 列出所有服务名称
// 返回值:
//   - []string: 服务名称列表
func (r *resourceManager) ListServices() []string {
	r.mx.Lock()         // 加锁
	defer r.mx.Unlock() // 解锁

	result := make([]string, 0, len(r.svc)) // 创建结果切片
	for svc := range r.svc {                // 遍历所有服务
		result = append(result, svc) // 添加服务名称
	}

	sort.Slice(result, func(i, j int) bool { // 对结果进行排序
		return strings.Compare(result[i], result[j]) < 0
	})

	return result // 返回排序后的结果
}

// ListProtocols 列出所有协议ID
// 返回值:
//   - []protocol.ID: 协议ID列表
func (r *resourceManager) ListProtocols() []protocol.ID {
	r.mx.Lock()         // 加锁
	defer r.mx.Unlock() // 解锁

	result := make([]protocol.ID, 0, len(r.proto)) // 创建结果切片
	for p := range r.proto {                       // 遍历所有协议
		result = append(result, p) // 添加协议ID
	}

	sort.Slice(result, func(i, j int) bool { // 对结果进行排序
		return result[i] < result[j]
	})

	return result // 返回排序后的结果
}

// ListPeers 列出所有对等节点ID
// 返回值:
//   - []peer.ID: 对等节点ID列表
func (r *resourceManager) ListPeers() []peer.ID {
	r.mx.Lock()         // 加锁
	defer r.mx.Unlock() // 解锁

	result := make([]peer.ID, 0, len(r.peer)) // 创建结果切片
	for p := range r.peer {                   // 遍历所有对等节点
		result = append(result, p) // 添加对等节点ID
	}

	sort.Slice(result, func(i, j int) bool { // 对结果进行排序
		return bytes.Compare([]byte(result[i]), []byte(result[j])) < 0
	})

	return result // 返回排序后的结果
}

// Stat 获取资源管理器的统计信息
// 返回值:
//   - ResourceManagerStat: 资源管理器统计信息
func (r *resourceManager) Stat() (result ResourceManagerStat) {
	r.mx.Lock() // 加锁
	// 创建临时切片存储作用域信息
	svcs := make([]*serviceScope, 0, len(r.svc))
	for _, svc := range r.svc {
		svcs = append(svcs, svc)
	}
	protos := make([]*protocolScope, 0, len(r.proto))
	for _, proto := range r.proto {
		protos = append(protos, proto)
	}
	peers := make([]*peerScope, 0, len(r.peer))
	for _, peer := range r.peer {
		peers = append(peers, peer)
	}
	r.mx.Unlock() // 解锁

	// 注意：没有全局锁，所以在我们导出状态时系统仍在更新...
	// 因此统计数据可能与系统级别不完全匹配；我们最后获取系统统计信息
	// 以便获得最新的快照
	result.Peers = make(map[peer.ID]network.ScopeStat, len(peers))
	for _, peer := range peers {
		result.Peers[peer.peer] = peer.Stat()
	}
	result.Protocols = make(map[protocol.ID]network.ScopeStat, len(protos))
	for _, proto := range protos {
		result.Protocols[proto.proto] = proto.Stat()
	}
	result.Services = make(map[string]network.ScopeStat, len(svcs))
	for _, svc := range svcs {
		result.Services[svc.service] = svc.Stat()
	}
	result.Transient = r.transient.Stat() // 获取临时作用域统计
	result.System = r.system.Stat()       // 获取系统作用域统计

	return result // 返回统计结果
}

// GetConnLimit 获取连接数限制
// 返回值:
//   - int: 连接数限制
func (r *resourceManager) GetConnLimit() int {
	return r.limits.GetSystemLimits().GetConnTotalLimit() // 返回系统级别的总连接数限制
}
