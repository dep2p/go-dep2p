// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 NAT 服务接口。
package interfaces

import (
	"context"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// NATService 定义 NAT 穿透服务接口
//
// NAT 服务负责网络地址转换和可达性检测。
type NATService interface {
	// GetReachability 获取当前可达性状态
	GetReachability() string

	// ExternalAddrs 返回外部地址列表
	//
	// 返回经过验证的公网可达地址（通过 STUN/AutoNAT 检测）。
	ExternalAddrs() []string

	// GetNATType 返回当前检测到的 NAT 类型
	//
	// 返回最近一次 NAT 类型检测的结果。
	// 如果尚未执行检测，返回 NATTypeUnknown。
	GetNATType() types.NATType

	// DetectNATType 执行 NAT 类型检测
	//
	// 基于 RFC 3489 算法检测 NAT 类型：
	//   - NATTypeNone: 无 NAT（公网直连）
	//   - NATTypeFullCone: 完全锥形 NAT
	//   - NATTypeRestrictedCone: 受限锥形 NAT
	//   - NATTypePortRestricted: 端口受限锥形 NAT
	//   - NATTypeSymmetric: 对称型 NAT
	//
	// 检测过程需要与 STUN 服务器通信，可能耗时数秒。
	DetectNATType(ctx context.Context) (types.NATType, error)

	// Stop 停止服务
	Stop() error
}

// ════════════════════════════════════════════════════════════════════════════
// HolePuncher 接口（NAT 打洞服务）
// 实现位置：internal/core/nat/holepunch/
// ════════════════════════════════════════════════════════════════════════════

// HolePuncher 定义 NAT 打洞服务接口
//
// HolePuncher 实现 DCUtR (Direct Connection Upgrade through Relay) 协议：
//   - 当两个节点都在 NAT 后，无法直接连接
//   - 通过中继连接协商各自的公网地址
//   - 双方同时向对方发起连接，打通 NAT
//
// 
type HolePuncher interface {
	// DirectConnect 尝试通过打洞建立直连
	//
	// 参数：
	//   - ctx: 上下文（用于超时控制）
	//   - peerID: 目标节点 ID
	//   - addrs: 目标节点的观测地址（公网地址）
	//
	// 返回：
	//   - 成功返回 nil，表示已建立直连
	//   - 失败返回错误，调用方应回退到中继
	//
	// 流程：
	//   1. 通过已有的中继连接打开 HolePunch 协议流
	//   2. 交换双方的观测地址（CONNECT 消息）
	//   3. 同步时机（SYNC 消息）
	//   4. 双方同时向对方地址发起连接（打洞）
	//   5. 等待直连建立成功
	DirectConnect(ctx context.Context, peerID string, addrs []string) error

	// IsActive 检查是否正在对某节点进行打洞
	IsActive(peerID string) bool
}
