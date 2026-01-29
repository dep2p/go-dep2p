// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 Bootstrap 接口，对应 internal/discovery/bootstrap/ 实现。
package interfaces

import (
	"context"
	"time"
)

// ════════════════════════════════════════════════════════════════════════════
// BootstrapService 接口
// ════════════════════════════════════════════════════════════════════════════

// BootstrapService 定义 Bootstrap 服务接口
//
// Bootstrap 服务用于新节点的冷启动，提供初始节点信息以加入 P2P 网络。
// 该服务采用极简设计，参数内置、无需配置。
//
// 注意：BootstrapService 不是标准的 Discovery 接口实现，
// 它是一个服务能力接口，用于将节点配置为引导节点。
// 实际的节点发现由 Coordinator 通过 DHT 的 Bootstrap() 方法完成。
//
// 架构位置：Discovery Layer
// 实现位置：internal/discovery/bootstrap/
// 详见：ADR-0009: Bootstrap 极简配置
//
// 使用示例:
//
//	bootstrap := bootstrap.NewService(host)
//
//	// 作为引导节点提供服务
//	bootstrap.Enable(ctx)
//	defer bootstrap.Close()
//
//	// 检查状态
//	if bootstrap.IsEnabled() {
//	    stats := bootstrap.Stats()
//	    log.Printf("在线节点数: %d", stats.OnlineNodes)
//	}
type BootstrapService interface {
	// Enable 启用 Bootstrap 服务能力
	//
	// 将当前节点设置为 Bootstrap 节点，为新节点提供入网引导。
	// 前置条件：节点必须有公网可达地址。
	// 无需参数，使用内置默认值。
	Enable(ctx context.Context) error

	// Disable 禁用 Bootstrap 服务能力
	//
	// 停止作为 Bootstrap 节点提供服务。
	Disable(ctx context.Context) error

	// IsEnabled 检查 Bootstrap 服务能力是否已启用
	IsEnabled() bool

	// Stats 返回 Bootstrap 服务统计信息
	Stats() BootstrapStats

	// Close 关闭 Bootstrap 服务
	Close() error
}

// BootstrapStats Bootstrap 服务统计信息
type BootstrapStats struct {
	// Enabled 是否已启用 Bootstrap 能力
	Enabled bool

	// TotalNodes 存储的节点总数
	TotalNodes int

	// OnlineNodes 在线节点数（最近探测成功）
	OnlineNodes int

	// LastProbe 最后一次存活探测时间
	LastProbe time.Time

	// LastDiscovery 最后一次主动发现时间
	LastDiscovery time.Time

	// ProbeSuccessRate 探测成功率（最近 1000 次）
	ProbeSuccessRate float64
}
