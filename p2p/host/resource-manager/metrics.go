package rcmgr

import (
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
)

// MetricsReporter 是一个用于收集资源管理器操作指标的接口
type MetricsReporter interface {
	// AllowConn 当允许打开连接时调用
	// 参数:
	//   - dir: network.Direction - 连接方向(入站/出站)
	//   - usefd: bool - 是否使用文件描述符
	AllowConn(dir network.Direction, usefd bool)

	// BlockConn 当阻止打开连接时调用
	// 参数:
	//   - dir: network.Direction - 连接方向(入站/出站)
	//   - usefd: bool - 是否使用文件描述符
	BlockConn(dir network.Direction, usefd bool)

	// AllowStream 当允许打开流时调用
	// 参数:
	//   - p: peer.ID - 对等节点ID
	//   - dir: network.Direction - 流方向(入站/出站)
	AllowStream(p peer.ID, dir network.Direction)

	// BlockStream 当阻止打开流时调用
	// 参数:
	//   - p: peer.ID - 对等节点ID
	//   - dir: network.Direction - 流方向(入站/出站)
	BlockStream(p peer.ID, dir network.Direction)

	// AllowPeer 当允许将连接附加到对等节点时调用
	// 参数:
	//   - p: peer.ID - 对等节点ID
	AllowPeer(p peer.ID)

	// BlockPeer 当阻止将连接附加到对等节点时调用
	// 参数:
	//   - p: peer.ID - 对等节点ID
	BlockPeer(p peer.ID)

	// AllowProtocol 当允许为流设置协议时调用
	// 参数:
	//   - proto: protocol.ID - 协议ID
	AllowProtocol(proto protocol.ID)

	// BlockProtocol 当阻止为流设置协议时调用
	// 参数:
	//   - proto: protocol.ID - 协议ID
	BlockProtocol(proto protocol.ID)

	// BlockProtocolPeer 当在每个协议对等节点范围内阻止为流设置协议时调用
	// 参数:
	//   - proto: protocol.ID - 协议ID
	//   - p: peer.ID - 对等节点ID
	BlockProtocolPeer(proto protocol.ID, p peer.ID)

	// AllowService 当允许为流设置服务时调用
	// 参数:
	//   - svc: string - 服务名称
	AllowService(svc string)

	// BlockService 当阻止为流设置服务时调用
	// 参数:
	//   - svc: string - 服务名称
	BlockService(svc string)

	// BlockServicePeer 当在每个服务对等节点范围内阻止为流设置服务时调用
	// 参数:
	//   - svc: string - 服务名称
	//   - p: peer.ID - 对等节点ID
	BlockServicePeer(svc string, p peer.ID)

	// AllowMemory 当允许内存预留时调用
	// 参数:
	//   - size: int - 内存大小
	AllowMemory(size int)

	// BlockMemory 当阻止内存预留时调用
	// 参数:
	//   - size: int - 内存大小
	BlockMemory(size int)
}

// metrics 是指标收集器的实现
type metrics struct {
	reporter MetricsReporter // 指标报告器
}

// WithMetrics 是一个资源管理器选项,用于启用指标收集
// 参数:
//   - reporter: MetricsReporter - 指标报告器
//
// 返回值:
//   - Option - 资源管理器选项
func WithMetrics(reporter MetricsReporter) Option {
	return func(r *resourceManager) error {
		r.metrics = &metrics{reporter: reporter} // 设置指标收集器
		return nil
	}
}

// AllowConn 记录允许连接的指标
// 参数:
//   - dir: network.Direction - 连接方向
//   - usefd: bool - 是否使用文件描述符
func (m *metrics) AllowConn(dir network.Direction, usefd bool) {
	if m == nil { // 如果指标收集器为空
		return
	}

	m.reporter.AllowConn(dir, usefd) // 报告允许连接
}

// BlockConn 记录阻止连接的指标
// 参数:
//   - dir: network.Direction - 连接方向
//   - usefd: bool - 是否使用文件描述符
func (m *metrics) BlockConn(dir network.Direction, usefd bool) {
	if m == nil { // 如果指标收集器为空
		return
	}

	m.reporter.BlockConn(dir, usefd) // 报告阻止连接
}

// AllowStream 记录允许流的指标
// 参数:
//   - p: peer.ID - 对等节点ID
//   - dir: network.Direction - 流方向
func (m *metrics) AllowStream(p peer.ID, dir network.Direction) {
	if m == nil { // 如果指标收集器为空
		return
	}

	m.reporter.AllowStream(p, dir) // 报告允许流
}

// BlockStream 记录阻止流的指标
// 参数:
//   - p: peer.ID - 对等节点ID
//   - dir: network.Direction - 流方向
func (m *metrics) BlockStream(p peer.ID, dir network.Direction) {
	if m == nil { // 如果指标收集器为空
		return
	}

	m.reporter.BlockStream(p, dir) // 报告阻止流
}

// AllowPeer 记录允许对等节点的指标
// 参数:
//   - p: peer.ID - 对等节点ID
func (m *metrics) AllowPeer(p peer.ID) {
	if m == nil { // 如果指标收集器为空
		return
	}

	m.reporter.AllowPeer(p) // 报告允许对等节点
}

// BlockPeer 记录阻止对等节点的指标
// 参数:
//   - p: peer.ID - 对等节点ID
func (m *metrics) BlockPeer(p peer.ID) {
	if m == nil { // 如果指标收集器为空
		return
	}

	m.reporter.BlockPeer(p) // 报告阻止对等节点
}

// AllowProtocol 记录允许协议的指标
// 参数:
//   - proto: protocol.ID - 协议ID
func (m *metrics) AllowProtocol(proto protocol.ID) {
	if m == nil { // 如果指标收集器为空
		return
	}

	m.reporter.AllowProtocol(proto) // 报告允许协议
}

// BlockProtocol 记录阻止协议的指标
// 参数:
//   - proto: protocol.ID - 协议ID
func (m *metrics) BlockProtocol(proto protocol.ID) {
	if m == nil { // 如果指标收集器为空
		return
	}

	m.reporter.BlockProtocol(proto) // 报告阻止协议
}

// BlockProtocolPeer 记录阻止协议对等节点的指标
// 参数:
//   - proto: protocol.ID - 协议ID
//   - p: peer.ID - 对等节点ID
func (m *metrics) BlockProtocolPeer(proto protocol.ID, p peer.ID) {
	if m == nil { // 如果指标收集器为空
		return
	}

	m.reporter.BlockProtocolPeer(proto, p) // 报告阻止协议对等节点
}

// AllowService 记录允许服务的指标
// 参数:
//   - svc: string - 服务名称
func (m *metrics) AllowService(svc string) {
	if m == nil { // 如果指标收集器为空
		return
	}

	m.reporter.AllowService(svc) // 报告允许服务
}

// BlockService 记录阻止服务的指标
// 参数:
//   - svc: string - 服务名称
func (m *metrics) BlockService(svc string) {
	if m == nil { // 如果指标收集器为空
		return
	}

	m.reporter.BlockService(svc) // 报告阻止服务
}

// BlockServicePeer 记录阻止服务对等节点的指标
// 参数:
//   - svc: string - 服务名称
//   - p: peer.ID - 对等节点ID
func (m *metrics) BlockServicePeer(svc string, p peer.ID) {
	if m == nil { // 如果指标收集器为空
		return
	}

	m.reporter.BlockServicePeer(svc, p) // 报告阻止服务对等节点
}

// AllowMemory 记录允许内存的指标
// 参数:
//   - size: int - 内存大小
func (m *metrics) AllowMemory(size int) {
	if m == nil { // 如果指标收集器为空
		return
	}

	m.reporter.AllowMemory(size) // 报告允许内存
}

// BlockMemory 记录阻止内存的指标
// 参数:
//   - size: int - 内存大小
func (m *metrics) BlockMemory(size int) {
	if m == nil { // 如果指标收集器为空
		return
	}

	m.reporter.BlockMemory(size) // 报告阻止内存
}
