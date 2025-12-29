// Package reachability 定义可达性验证相关接口
//
// 可达性验证模块负责验证节点的外部地址是否真正可达，
// 通过请求其他节点进行回拨（dial-back）来确认。
//
// 协议：/dep2p/sys/reachability/1.0.0
// 注：使用 sys 前缀表明这是系统级协议，无需 Realm 验证
//
// 协议流程：
//
//	┌──────────┐                    ┌──────────┐
//	│  Node A  │                    │  Node B  │
//	│(验证方)   │                    │(协助方)   │
//	└────┬─────┘                    └────┬─────┘
//	     │  1. 建立连接（已有）             │
//	     │────────────────────────────────>│
//	     │                                 │
//	     │  2. 发送 DialBackRequest        │
//	     │    { addrs: [候选地址列表] }     │
//	     │────────────────────────────────>│
//	     │                                 │
//	     │         3. B 尝试回拨 A 的候选地址
//	     │<────────────────────────────────│
//	     │                                 │
//	     │  4. 返回 DialBackResponse       │
//	     │    { reachable: [可达地址列表] } │
//	     │<────────────────────────────────│
package reachability

import (
	"context"
	"time"

	addressif "github.com/dep2p/go-dep2p/pkg/interfaces/address"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              协议常量
// ============================================================================

// 引用 pkg/protocolids 唯一真源
var (
	// ProtocolID 可达性验证协议标识（dial-back）
	// Layer1 修复：使用 sys 前缀与其他系统协议（relay/holepunch）保持一致
	ProtocolID = protocolids.SysReachability

	// WitnessProtocolID 入站见证协议标识（无外部依赖升级）
	// 当 peer 使用候选地址成功连入后，自动发送见证报告
	WitnessProtocolID = protocolids.SysReachabilityWitness
)

const (
	// DefaultDialBackTimeout 默认回拨超时时间
	DefaultDialBackTimeout = 10 * time.Second

	// DefaultRequestTimeout 默认请求超时时间
	DefaultRequestTimeout = 30 * time.Second

	// MaxAddrsPerRequest 单次请求最大地址数
	MaxAddrsPerRequest = 10

	// MaxConcurrentDialBacks 最大并发回拨数
	MaxConcurrentDialBacks = 3

	// DefaultMinWitnesses witness-threshold 默认最小见证数
	// 同一候选地址需被此数量的不同 IP 前缀 peer 见证后，才能升级为 VerifiedDirect
	DefaultMinWitnesses = 2

	// DefaultWitnessIPv4Prefix IPv4 见证去重前缀长度（/24）
	DefaultWitnessIPv4Prefix = 24

	// DefaultWitnessIPv6Prefix IPv6 见证去重前缀长度（/48）
	DefaultWitnessIPv6Prefix = 48

	// DefaultWitnessTTL 见证记录默认有效期
	DefaultWitnessTTL = 30 * time.Minute
)

// ============================================================================
//                              消息结构
// ============================================================================

// DialBackRequest 回拨验证请求
//
// 验证方发送此请求，请求协助方尝试回拨指定的候选地址。
type DialBackRequest struct {
	// Addrs 待验证的候选地址列表
	// 格式为 multiaddr 字符串，如 "/ip4/1.2.3.4/udp/4001/quic-v1"
	Addrs []string `json:"addrs"`

	// Nonce 随机数，用于防重放攻击
	// 协助方需在响应中原样返回
	Nonce []byte `json:"nonce"`

	// Timeout 期望的回拨超时时间（毫秒）
	// 协助方可能使用更短的超时
	TimeoutMs int64 `json:"timeout_ms,omitempty"`
}

// DialBackResponse 回拨验证响应
//
// 协助方返回此响应，报告哪些地址可达。
type DialBackResponse struct {
	// Reachable 可达的地址列表
	// 仅包含成功回拨的地址
	Reachable []string `json:"reachable"`

	// Nonce 原样返回请求中的随机数
	Nonce []byte `json:"nonce"`

	// Error 错误信息（可选）
	// 如果整体验证失败，此字段包含错误描述
	Error string `json:"error,omitempty"`

	// DialResults 每个地址的详细回拨结果（可选）
	DialResults []DialResult `json:"dial_results,omitempty"`
}

// DialResult 单个地址的回拨结果
type DialResult struct {
	// Addr 地址
	Addr string `json:"addr"`

	// Success 是否成功
	Success bool `json:"success"`

	// Latency 回拨延迟（毫秒），仅在成功时有效
	LatencyMs int64 `json:"latency_ms,omitempty"`

	// Error 失败原因，仅在失败时有效
	Error string `json:"error,omitempty"`
}

// ============================================================================
//                              Witness 协议消息
// ============================================================================

// WitnessReport 入站见证报告
//
// 当 peer 使用候选地址成功连入后，自动发送此报告。
// 用于无外部依赖的 VerifiedDirect 升级。
type WitnessReport struct {
	// DialedAddr 实际使用的 dial 地址
	// 格式为 multiaddr 字符串
	DialedAddr string `json:"dialed_addr"`

	// TargetID 目标节点 ID
	TargetID []byte `json:"target_id"`

	// Timestamp Unix 时间戳（秒）
	Timestamp int64 `json:"timestamp"`
}

// WitnessAck 入站见证确认
type WitnessAck struct {
	// Accepted 是否接受
	Accepted bool `json:"accepted"`

	// Reason 拒绝原因（如有）
	Reason string `json:"reason,omitempty"`

	// ObservedRemoteAddr 接收方观测到的“发送方”公网地址（旁路/弱证据）
	//
	// 语义：
	// - 仅用于降低用户门槛：向 NAT 客户端反馈其公网出口 (IP:port) 的观测值
	// - 不进入 DHT，不自动视为可达（并不等价于 VerifiedDirect）
	// - 可能是短暂映射/对端不可回拨，需谨慎展示
	ObservedRemoteAddr string `json:"observed_remote_addr,omitempty"`
}

// ============================================================================
//                              服务接口
// ============================================================================

// DialBackService 回拨验证服务接口
//
// 提供可达性验证的核心功能。
type DialBackService interface {
	// VerifyAddresses 验证候选地址的可达性
	//
	// 向指定的协助节点发送回拨请求，验证本地候选地址是否可达。
	// 返回经过验证的可达地址列表。
	//
	// 参数：
	//   - ctx: 上下文，用于超时和取消控制
	//   - helperID: 协助节点的 ID
	//   - candidateAddrs: 待验证的候选地址列表
	//
	// 返回：
	//   - reachable: 验证可达的地址列表
	//   - err: 错误信息（如无法连接协助节点）
	VerifyAddresses(ctx context.Context, helperID types.NodeID, candidateAddrs []endpoint.Address) (reachable []endpoint.Address, err error)

	// HandleDialBackRequest 处理来自其他节点的回拨请求
	//
	// 作为协助方，尝试回拨请求方指定的地址。
	// 此方法通常由协议处理器内部调用。
	HandleDialBackRequest(ctx context.Context, req *DialBackRequest) *DialBackResponse

	// Start 启动服务
	Start(ctx context.Context) error

	// Stop 停止服务
	Stop() error
}

// ============================================================================
//                              候选地址结构
// ============================================================================

// CandidateKind 候选地址类型
type CandidateKind string

const (
	// CandidateKindDirect 直连候选地址
	CandidateKindDirect CandidateKind = "direct"
	// CandidateKindRelay relay 候选地址
	CandidateKindRelay CandidateKind = "relay"
)

// ConfidenceLevel 置信度级别
type ConfidenceLevel string

const (
	// ConfidenceHigh 高置信度（如显式绑定公网 IP）
	ConfidenceHigh ConfidenceLevel = "high"
	// ConfidenceMedium 中置信度（如云元数据、STUN）
	ConfidenceMedium ConfidenceLevel = "medium"
	// ConfidenceLow 低置信度（如本机接口推断、用户配置）
	ConfidenceLow ConfidenceLevel = "low"
)

// BootstrapCandidate 候选地址结构
//
// 用于 BootstrapCandidates() 返回，支持人工分享/跨设备冷启动。
// MUST NOT 用于 DHT 发布，不等同于 ShareableAddrs。
type BootstrapCandidate struct {
	// FullAddr 完整地址（含 /p2p/<NodeID>）
	FullAddr string

	// Kind 地址类型：direct | relay
	Kind CandidateKind

	// Source 来源标识：listen-bound / interface / cloud-metadata / user-config / relay
	Source string

	// Confidence 置信度：high / medium / low
	Confidence ConfidenceLevel

	// Verified 是否已验证（VerifiedDirect）
	Verified bool

	// Notes 可读说明
	Notes string
}

// ============================================================================
//                              协调器接口
// ============================================================================

// Coordinator 可达性协调器接口（用于"可达性优先"策略）
//
// 设计目标：
// - Endpoint 等上层模块 **只能依赖接口**，禁止依赖 internal/core/reachability 的具体实现。
// - 该接口聚焦于"对外通告地址的裁剪/变更通知"以及"直连地址候选/验证事件"。
//
// 注：实现位于 internal/core/reachability/，并通过 fx 以 name:"reachability_coordinator" 注入。
type Coordinator interface {
	// AdvertisedAddrs 返回当前"可对外通告"的地址集合（通常是可拨号/可达的地址）。
	AdvertisedAddrs() []endpoint.Address

	// VerifiedDirectAddresses 返回已验证的直连地址列表（REQ-ADDR-002 真源）
	//
	// 这是 ShareableAddrs 的唯一真实数据源：
	// - 仅返回通过 dial-back 或 witness-threshold 验证的公网直连地址
	// - 不包含 Relay 地址
	// - 不包含 ListenAddrs 回退
	// - 无验证地址时返回 nil（而非空切片）
	//
	// 用于构建可分享的 Full Address（INV-005）。
	VerifiedDirectAddresses() []endpoint.Address

	// BootstrapCandidates 返回可用于冷启动尝试的候选地址列表（旁路/非严格）
	//
	// 用于人工分享/跨设备冷启动：
	// - 包含直连候选 + relay 候选
	// - 不保证可达，仅供跨设备试连
	// - MUST NOT 用于 DHT 发布
	// - MUST NOT 等同于 ShareableAddrs
	//
	// nodeID 用于构建完整地址（含 /p2p/<NodeID>）
	BootstrapCandidates(nodeID types.NodeID) []BootstrapCandidate

	// SetOnAddressChanged 注册回调：当对外通告地址集合变化时触发。
	SetOnAddressChanged(func([]endpoint.Address))

	// OnDirectAddressCandidate 上报"可能直连"的候选地址（例如来自 NAT/STUN/监听器）。
	OnDirectAddressCandidate(addr endpoint.Address, source string, priority addressif.AddressPriority)

	// OnDirectAddressVerified 上报"已验证直连"的地址（例如 dial-back 或 witness-threshold 成功）。
	OnDirectAddressVerified(addr endpoint.Address, source string, priority addressif.AddressPriority)

	// OnInboundWitness 上报入站见证（无外部依赖升级路径）
	//
	// 当 peer 使用候选地址成功连入后，调用此方法记录见证。
	// 达到阈值后自动升级为 VerifiedDirect。
	//
	// 参数：
	//   - dialedAddr: 对方实际使用的 dial 地址（multiaddr 字符串）
	//   - remotePeerID: 见证者 peer ID
	//   - remoteIP: 见证者 IP 地址（用于 IP 前缀去重）
	OnInboundWitness(dialedAddr string, remotePeerID types.NodeID, remoteIP string)

	// OnOutboundConnected 上报出站连接成功事件
	//
	// 当节点成功拨号到某个地址时调用，用于触发 witness 报告发送。
	// 实现应异步发送 WitnessReport 到对端。
	//
	// 参数：
	//   - conn: 成功建立的连接
	//   - dialedAddr: 实际使用的 dial 地址字符串
	OnOutboundConnected(conn endpoint.Connection, dialedAddr string)
}

// ============================================================================
//                              验证结果
// ============================================================================

// VerificationResult 地址验证结果
type VerificationResult struct {
	// Addr 被验证的地址
	Addr endpoint.Address

	// Reachable 是否可达
	Reachable bool

	// VerifiedBy 验证者节点 ID
	VerifiedBy types.NodeID

	// VerifiedAt 验证时间
	VerifiedAt time.Time

	// Latency 回拨延迟
	Latency time.Duration

	// Error 验证失败原因（如果有）
	Error error
}

// ============================================================================
//                              配置
// ============================================================================

// Config 可达性验证配置
type Config struct {
	// EnableDialBack 是否启用 dial-back 验证（Phase 3）
	EnableDialBack bool

	// TrustedHelpers 配置的“可信 helper”列表
	// 为空时会退化到使用当前已连接 peers 作为 helper（混合策略）。
	TrustedHelpers []types.NodeID

	// DialBackTimeout 回拨超时时间
	DialBackTimeout time.Duration

	// RequestTimeout 请求超时时间
	RequestTimeout time.Duration

	// MaxConcurrentDialBacks 最大并发回拨数
	MaxConcurrentDialBacks int

	// MinVerifications 最小验证次数
	// 一个地址需要被至少这么多个协助节点验证才算可达
	MinVerifications int

	// VerificationInterval 验证间隔
	// 定期重新验证已发布地址的可达性
	VerificationInterval time.Duration

	// EnableAsHelper 是否作为协助节点
	// 如果为 true，则响应其他节点的回拨请求
	EnableAsHelper bool
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		EnableDialBack:         true,
		TrustedHelpers:         nil,
		DialBackTimeout:        DefaultDialBackTimeout,
		RequestTimeout:         DefaultRequestTimeout,
		MaxConcurrentDialBacks: MaxConcurrentDialBacks,
		MinVerifications:       1,
		VerificationInterval:   5 * time.Minute,
		EnableAsHelper:         true,
	}
}
