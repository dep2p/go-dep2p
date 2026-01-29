// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 Rendezvous 接口，对应 internal/discovery/rendezvous/ 实现。
package interfaces

import (
	"context"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ════════════════════════════════════════════════════════════════════════════
// RendezvousService 接口（客户端）
// ════════════════════════════════════════════════════════════════════════════

// RendezvousService 定义 Rendezvous 客户端接口
//
// Rendezvous 通过命名空间实现轻量级节点发现，与 DHT 不同，
// 它通过中心化的 Rendezvous Point 来协调节点发现。
// 适用于 Realm 内成员发现和服务发现场景。
//
// 架构位置：Discovery Layer
// 实现位置：internal/discovery/rendezvous/
//
// 使用场景:
//   - Realm 内节点发现（命名空间 = RealmID）
//   - 应用级节点分组
//   - 服务发现
//
// 使用示例:
//
//	discoverer := rendezvous.NewDiscoverer(host, config)
//	discoverer.Start(ctx)
//	defer discoverer.Stop(ctx)
//
//	// 注册到命名空间
//	discoverer.Register(ctx, "my-app/chat", 2*time.Hour)
//
//	// 发现节点
//	peers, _ := discoverer.Discover(ctx, "my-app/chat", 10)
type RendezvousService interface {
	Discovery

	// Register 在命名空间注册本节点
	//
	// 参数:
	//   - ns: 命名空间
	//   - ttl: 注册有效期
	Register(ctx context.Context, ns string, ttl time.Duration) error

	// Unregister 取消命名空间注册
	Unregister(ctx context.Context, ns string) error

	// Discover 同步发现节点
	//
	// 参数:
	//   - ns: 命名空间
	//   - limit: 返回数量限制
	Discover(ctx context.Context, ns string, limit int) ([]types.PeerInfo, error)

	// Stats 返回 Rendezvous 服务统计信息
	Stats() RendezvousStats
}

// RendezvousStats Rendezvous 服务统计信息
type RendezvousStats struct {
	// RegisteredNamespaces 已注册的命名空间数
	RegisteredNamespaces int

	// TotalRegisters 总注册次数
	TotalRegisters int64

	// TotalDiscovers 总发现请求数
	TotalDiscovers int64

	// PeersDiscovered 发现的节点总数
	PeersDiscovered int64

	// LastRegister 最后一次注册时间
	LastRegister time.Time

	// LastDiscover 最后一次发现时间
	LastDiscover time.Time
}

// ════════════════════════════════════════════════════════════════════════════
// RendezvousPoint 接口（服务端）
// ════════════════════════════════════════════════════════════════════════════

// RendezvousPoint 定义 Rendezvous Point 服务端接口
//
// Rendezvous Point 是 Rendezvous 协议的服务端，负责：
// - 存储节点注册信息
// - 处理发现请求
// - 过期清理
//
// 架构位置：Discovery Layer
// 实现位置：internal/discovery/rendezvous/
//
// 使用示例:
//
//	point := rendezvous.NewPoint(host, config)
//	point.Start(ctx)
//	defer point.Stop()
//
//	// 获取统计信息
//	stats := point.Stats()
//	log.Printf("注册数: %d", stats.TotalRegistrations)
type RendezvousPoint interface {
	// Start 启动 Rendezvous Point 服务
	Start(ctx context.Context) error

	// Stop 停止 Rendezvous Point 服务
	Stop() error

	// Stats 返回 Point 统计信息
	Stats() RendezvousPointStats
}

// RendezvousPointStats Rendezvous Point 统计信息
type RendezvousPointStats struct {
	// TotalRegistrations 总注册数
	TotalRegistrations int

	// TotalNamespaces 命名空间数
	TotalNamespaces int

	// RegistersReceived 收到的注册请求数
	RegistersReceived int64

	// DiscoversReceived 收到的发现请求数
	DiscoversReceived int64

	// LastCleanup 最后一次清理时间
	LastCleanup time.Time
}
