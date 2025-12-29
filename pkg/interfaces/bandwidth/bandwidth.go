// Package bandwidth 定义带宽统计接口
//
// 带宽统计模块负责：
// - 总流量统计（入/出）
// - 按 Peer 分类的流量统计
// - 按 Protocol 分类的流量统计
// - 实时速率计算（bytes/sec）
//
// 参考: libp2p 的 BandwidthCounter 设计
package bandwidth

import (
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
)

// ============================================================================
//                              统计数据
// ============================================================================

// Stats 带宽统计快照
//
// Stats 包含某个维度（总量、Peer、Protocol）的流量统计信息
type Stats struct {
	// TotalIn 总入站字节数
	TotalIn int64

	// TotalOut 总出站字节数
	TotalOut int64

	// RateIn 入站速率 (bytes/sec)
	// 使用指数加权移动平均 (EWMA) 计算
	RateIn float64

	// RateOut 出站速率 (bytes/sec)
	// 使用指数加权移动平均 (EWMA) 计算
	RateOut float64
}

// TotalBytes 返回总字节数（入+出）
func (s Stats) TotalBytes() int64 {
	return s.TotalIn + s.TotalOut
}

// TotalRate 返回总速率（入+出）
func (s Stats) TotalRate() float64 {
	return s.RateIn + s.RateOut
}

// ============================================================================
//                              计数器接口
// ============================================================================

// Counter 带宽计数器接口
//
// Counter 跟踪本地节点的入站和出站数据传输。
// 提供总带宽、按 Peer 和按 Protocol 的统计维度。
//
// 使用示例:
//
//	counter := bandwidth.NewCounter()
//
//	// 在 Stream 中记录流量
//	counter.LogSentMessageStream(1024, "/chat/1.0", peerID)
//
//	// 获取统计
//	total := counter.GetBandwidthTotals()
//	fmt.Printf("总流量: 入 %d, 出 %d\n", total.TotalIn, total.TotalOut)
type Counter interface {
	// ==================== 记录流量 ====================

	// LogSentMessage 记录发送的消息大小
	//
	// 记录出站消息，不关联具体的 Peer 或 Protocol
	LogSentMessage(size int64)

	// LogRecvMessage 记录接收的消息大小
	//
	// 记录入站消息，不关联具体的 Peer 或 Protocol
	LogRecvMessage(size int64)

	// LogSentMessageStream 记录流上发送的消息
	//
	// 记录出站消息，关联到具体的 Protocol 和 Peer
	//
	// 参数：
	//   - size: 消息字节数
	//   - proto: 协议 ID
	//   - peer: 远程节点 ID
	LogSentMessageStream(size int64, proto endpoint.ProtocolID, peer endpoint.NodeID)

	// LogRecvMessageStream 记录流上接收的消息
	//
	// 记录入站消息，关联到具体的 Protocol 和 Peer
	//
	// 参数：
	//   - size: 消息字节数
	//   - proto: 协议 ID
	//   - peer: 远程节点 ID
	LogRecvMessageStream(size int64, proto endpoint.ProtocolID, peer endpoint.NodeID)

	// ==================== 获取统计 ====================

	// GetBandwidthTotals 获取总带宽统计
	//
	// 返回所有数据的汇总统计，不区分 Peer 或 Protocol
	GetBandwidthTotals() Stats

	// GetBandwidthForPeer 获取指定 Peer 的带宽统计
	//
	// 返回与指定节点通信的统计，包括所有协议
	GetBandwidthForPeer(peer endpoint.NodeID) Stats

	// GetBandwidthForProtocol 获取指定协议的带宽统计
	//
	// 返回指定协议的统计，包括所有 Peer
	GetBandwidthForProtocol(proto endpoint.ProtocolID) Stats

	// GetBandwidthByPeer 获取所有 Peer 的带宽统计
	//
	// 返回按 Peer 分组的统计 map
	// 注意：此方法可能比较耗时
	GetBandwidthByPeer() map[endpoint.NodeID]Stats

	// GetBandwidthByProtocol 获取所有协议的带宽统计
	//
	// 返回按协议分组的统计 map
	GetBandwidthByProtocol() map[endpoint.ProtocolID]Stats

	// ==================== 管理 ====================

	// Reset 重置所有统计
	Reset()

	// TrimIdle 清理空闲条目
	//
	// 移除自指定时间以来没有活动的 Peer 和 Protocol 记录
	// 用于内存优化
	TrimIdle(since time.Time)
}

// ============================================================================
//                              报告器接口（v1.1 已删除）
// ============================================================================

// 注意：Reporter 接口已删除（v1.1 清理）。
// 原因：未集成 Fx，无外部使用。
// 内部实现保留在 internal/core/bandwidth/reporter.go。

// Report 带宽报告
type Report struct {
	// Timestamp 报告生成时间
	Timestamp time.Time

	// Duration 报告周期
	Duration time.Duration

	// Total 总带宽统计
	Total Stats

	// ByPeer 按 Peer 的统计（可选，可能为空）
	ByPeer map[endpoint.NodeID]Stats

	// ByProtocol 按协议的统计（可选，可能为空）
	ByProtocol map[endpoint.ProtocolID]Stats

	// TopPeers 流量最大的 N 个 Peer
	TopPeers []PeerStats

	// TopProtocols 流量最大的 N 个协议
	TopProtocols []ProtocolStats
}

// PeerStats Peer 统计条目
type PeerStats struct {
	Peer  endpoint.NodeID
	Stats Stats
}

// ProtocolStats 协议统计条目
type ProtocolStats struct {
	Protocol endpoint.ProtocolID
	Stats    Stats
}

// ============================================================================
//                              配置
// ============================================================================

// Config 带宽统计配置
type Config struct {
	// Enabled 是否启用带宽统计
	// 默认 true
	Enabled bool

	// TrackByPeer 是否按 Peer 统计
	// 默认 true
	TrackByPeer bool

	// TrackByProtocol 是否按协议统计
	// 默认 true
	TrackByProtocol bool

	// IdleTimeout 空闲超时
	// 超过此时间没有活动的条目会被清理
	// 默认 1 小时
	IdleTimeout time.Duration

	// TrimInterval 清理间隔
	// 默认 10 分钟
	TrimInterval time.Duration

	// ReportInterval 报告间隔（如果启用自动报告）
	// 默认 1 分钟
	ReportInterval time.Duration
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		Enabled:         true,
		TrackByPeer:     true,
		TrackByProtocol: true,
		IdleTimeout:     time.Hour,
		TrimInterval:    10 * time.Minute,
		ReportInterval:  time.Minute,
	}
}

