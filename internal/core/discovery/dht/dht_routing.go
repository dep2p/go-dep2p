// Package dht 提供分布式哈希表实现
package dht

import (
	"context"
	"time"

	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              节点更新
// ============================================================================

// AddNode 添加节点到路由表
func (d *DHT) AddNode(id types.NodeID, addrs []string, realmID types.RealmID) {
	node := &RoutingNode{
		ID:       id,
		Addrs:    addrs,
		LastSeen: time.Now(),
		RealmID:  realmID,
	}
	_ = d.routingTable.Update(node) // 忽略 bucket full 错误
}

// RemoveNode 从路由表移除节点
func (d *DHT) RemoveNode(id types.NodeID) {
	d.routingTable.Remove(id)
}

// UpdateNode 更新节点信息
func (d *DHT) UpdateNode(id types.NodeID, addrs []string, rtt time.Duration) {
	node := d.routingTable.Find(id)
	if node != nil {
		node.Addrs = addrs
		node.RTT = rtt
		node.LastSeen = time.Now()
		node.FailCount = 0
		_ = d.routingTable.Update(node) // 忽略 bucket full 错误
	}
}

// RecordFailure 记录节点失败
func (d *DHT) RecordFailure(id types.NodeID) {
	node := d.routingTable.Find(id)
	if node != nil {
		node.FailCount++
		if node.FailCount > 5 {
			d.routingTable.Remove(id)
		}
	}
}

// ============================================================================
//                              连接通知
// ============================================================================

// NotifyPeerConnected 通知新连接建立
//
// 将连接的节点加入路由表，并在路由表从空变为非空时触发引导。
// 这解决了 seed 节点被动等待连接时 DHT 路由表一直为空的问题。
//
// 调用时机：Endpoint 或 DiscoveryService 在新连接建立时调用此方法。
func (d *DHT) NotifyPeerConnected(nodeID types.NodeID, addrs []string) {
	// 不能添加自己
	if nodeID == d.localID {
		return
	}

	// 检查路由表是否为空
	wasEmpty := d.routingTable.Size() == 0

	// 将节点加入路由表
	d.AddNode(nodeID, addrs, d.realmID)

	log.Debug("连接节点已加入 DHT 路由表",
		"nodeID", nodeID.ShortString(),
		"addrs", addrs,
		"wasEmpty", wasEmpty,
		"routing_table_size", d.routingTable.Size())

	// 如果路由表从空变为非空，立即触发引导
	// 这让 seed 节点在第一个连接到来时能快速填充路由表
	if wasEmpty && d.routingTable.Size() > 0 {
		log.Info("路由表从空变为非空，立即触发 DHT 引导")
		go func() {
			ctx, cancel := context.WithTimeout(d.ctx, 30*time.Second)
			defer cancel()
			_ = d.Bootstrap(ctx) // 引导失败在后台会重试
		}()
	}
}

// NotifyPeerDisconnected 通知连接断开
//
// 可选：在连接断开时从路由表移除节点。
// 注意：不建议立即移除，因为节点可能只是暂时断开。
// 路由表有自己的过期清理机制（cleanupLoop）。
func (d *DHT) NotifyPeerDisconnected(nodeID types.NodeID) {
	// 记录失败，让路由表的清理机制处理
	d.RecordFailure(nodeID)

	log.Debug("连接断开，已记录节点失败",
		"nodeID", nodeID.ShortString())
}

// ============================================================================
//                              路由表接口
// ============================================================================

// RoutingTable 返回路由表
func (d *DHT) RoutingTable() discoveryif.RoutingTable {
	return &routingTableWrapper{rt: d.routingTable}
}

// InternalRoutingTable 返回内部路由表（仅供模块内部使用）
//
// 用于 NetworkAdapter 直接从路由表获取节点地址，避免 DHT RPC 拨号时触发 discovery 递归。
func (d *DHT) InternalRoutingTable() *RoutingTable {
	return d.routingTable
}

// routingTableWrapper 路由表包装器
type routingTableWrapper struct {
	rt *RoutingTable
}

func (w *routingTableWrapper) Size() int {
	return w.rt.Size()
}

func (w *routingTableWrapper) Peers() []types.NodeID {
	return w.rt.Peers()
}

func (w *routingTableWrapper) NearestPeers(key []byte, count int) []types.NodeID {
	return w.rt.NearestPeers(key, count)
}

func (w *routingTableWrapper) Update(id types.NodeID) error {
	node := &RoutingNode{
		ID:       id,
		LastSeen: time.Now(),
	}
	return w.rt.Update(node)
}

func (w *routingTableWrapper) Remove(id types.NodeID) {
	w.rt.Remove(id)
}

func (w *routingTableWrapper) Find(id types.NodeID) *RoutingNode {
	return w.rt.Find(id)
}

