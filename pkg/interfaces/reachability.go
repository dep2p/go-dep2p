// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义可达性验证相关接口
//
// IMPL-NETWORK-RESILIENCE Phase 6.4: Reachability Coordinator
package interfaces

import (
	"context"
	"time"
)

// ============================================================================
//                              协议常量
// ============================================================================

const (
	// ReachabilityProtocolID 可达性验证协议标识（dial-back）
	ReachabilityProtocolID = "/dep2p/sys/reachability/1.0.0"

	// WitnessProtocolID 入站见证协议标识
	WitnessProtocolID = "/dep2p/sys/reachability/witness/1.0.0"

	// DefaultDialBackTimeout 默认回拨超时时间
	DefaultDialBackTimeout = 10 * time.Second

	// DefaultRequestTimeout 默认请求超时时间
	DefaultRequestTimeout = 30 * time.Second

	// MaxAddrsPerRequest 单次请求最大地址数
	MaxAddrsPerRequest = 10

	// MaxConcurrentDialBacks 最大并发回拨数
	MaxConcurrentDialBacks = 3

	// DefaultMinWitnesses 默认最小见证数
	DefaultMinWitnesses = 2

	// DefaultWitnessIPv4Prefix IPv4 见证去重前缀长度
	DefaultWitnessIPv4Prefix = 24

	// DefaultWitnessIPv6Prefix IPv6 见证去重前缀长度
	DefaultWitnessIPv6Prefix = 48
)

// ============================================================================
//                              消息结构
// ============================================================================

// DialBackRequest 回拨验证请求
type DialBackRequest struct {
	// Addrs 待验证的候选地址列表
	Addrs []string `json:"addrs"`

	// Nonce 随机数，用于防重放攻击
	Nonce []byte `json:"nonce"`

	// TimeoutMs 期望的回拨超时时间（毫秒）
	TimeoutMs int64 `json:"timeout_ms,omitempty"`
}

// DialBackResponse 回拨验证响应
type DialBackResponse struct {
	// Reachable 可达的地址列表
	Reachable []string `json:"reachable"`

	// Nonce 原样返回请求中的随机数
	Nonce []byte `json:"nonce"`

	// Error 错误信息
	Error string `json:"error,omitempty"`

	// DialResults 每个地址的详细回拨结果
	DialResults []DialResult `json:"dial_results,omitempty"`
}

// DialResult 单个地址的回拨结果
type DialResult struct {
	Addr      string `json:"addr"`
	Success   bool   `json:"success"`
	LatencyMs int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// WitnessReport 入站见证报告
type WitnessReport struct {
	DialedAddr string `json:"dialed_addr"`
	TargetID   []byte `json:"target_id"`
	Timestamp  int64  `json:"timestamp"`
}

// WitnessAck 入站见证确认
type WitnessAck struct {
	Accepted           bool   `json:"accepted"`
	Reason             string `json:"reason,omitempty"`
	ObservedRemoteAddr string `json:"observed_remote_addr,omitempty"`
}

// ============================================================================
//                              地址类型
// ============================================================================

// AddressPriority 地址优先级
type AddressPriority int

const (
	// PriorityUnverified 未验证地址（最低优先级）
	PriorityUnverified AddressPriority = 0

	// PriorityLocalListen 本地监听地址
	PriorityLocalListen AddressPriority = 10

	// PriorityRelayGuarantee Relay 地址（保证可达）
	PriorityRelayGuarantee AddressPriority = 50

	// PrioritySTUNDiscovered STUN 发现的公网地址
	// STUN 协议本身验证了外部地址的存在，无需额外 dial-back
	PrioritySTUNDiscovered AddressPriority = 75

	// PriorityVerifiedDirect 已验证直连地址（dial-back 验证）
	PriorityVerifiedDirect AddressPriority = 100

	// PriorityConfiguredAdvertise 用户配置的公网地址（最高优先级）
	// 用户通过 WithPublicAddr() 明确声明的地址，不需要验证
	PriorityConfiguredAdvertise AddressPriority = 150
)

// String 返回优先级字符串
func (p AddressPriority) String() string {
	switch {
	case p >= PriorityConfiguredAdvertise:
		return "configured_advertise"
	case p >= PriorityVerifiedDirect:
		return "verified_direct"
	case p >= PrioritySTUNDiscovered:
		return "stun_discovered"
	case p >= PriorityRelayGuarantee:
		return "relay_guarantee"
	case p >= PriorityLocalListen:
		return "local_listen"
	default:
		return "unverified"
	}
}

// CandidateKind 候选地址类型
type CandidateKind string

const (
	CandidateKindDirect CandidateKind = "direct"
	CandidateKindRelay  CandidateKind = "relay"
)

// ConfidenceLevel 置信度级别
type ConfidenceLevel string

const (
	ConfidenceHigh   ConfidenceLevel = "high"
	ConfidenceMedium ConfidenceLevel = "medium"
	ConfidenceLow    ConfidenceLevel = "low"
)

// BootstrapCandidate 候选地址结构
type BootstrapCandidate struct {
	FullAddr   string          // 完整地址（含 /p2p/<NodeID>）
	Kind       CandidateKind   // 地址类型
	Source     string          // 来源标识
	Confidence ConfidenceLevel // 置信度
	Verified   bool            // 是否已验证
	Notes      string          // 可读说明
}

// CandidateUpdate 候选地址更新结构
type CandidateUpdate struct {
	Addr     string          // 地址字符串
	Priority AddressPriority // 优先级
}

// ============================================================================
//                              协调器接口
// ============================================================================

// ReachabilityCoordinator 可达性协调器接口
//
// 统一管理地址发布，实现"可达性优先"策略：
// - 先保证连得上（Relay 兜底）
// - 再争取直连更优路径
type ReachabilityCoordinator interface {
	// AdvertisedAddrs 返回当前可对外通告的地址集合
	AdvertisedAddrs() []string

	// VerifiedDirectAddresses 返回已验证的直连地址列表
	VerifiedDirectAddresses() []string

	// CandidateDirectAddresses 返回候选直连地址列表（未验证的 STUN/UPnP/NAT-PMP 地址）
	//
	// 
	// 对于 NAT 节点，dial-back 验证无法成功，但 STUN 候选地址是真实的外部地址，
	// 可用于打洞协商。
	CandidateDirectAddresses() []string

	// RelayAddresses 返回所有 Relay 地址
	RelayAddresses() []string

	// BootstrapCandidates 返回可用于冷启动尝试的候选地址列表
	BootstrapCandidates(nodeID string) []BootstrapCandidate

	// SetOnAddressChanged 设置地址变更回调
	SetOnAddressChanged(callback func([]string))

	// OnDirectAddressCandidate 上报候选直连地址
	OnDirectAddressCandidate(addr string, source string, priority AddressPriority)

	// UpdateDirectCandidates 批量更新直连候选地址
	UpdateDirectCandidates(source string, candidates []CandidateUpdate)

	// OnDirectAddressVerified 上报已验证的直连地址
	OnDirectAddressVerified(addr string, source string, priority AddressPriority)

	// OnDirectAddressExpired 上报过期的直连地址
	OnDirectAddressExpired(addr string)

	// OnRelayReserved Relay 预留成功回调
	OnRelayReserved(addrs []string)

	// OnInboundWitness 上报入站见证
	OnInboundWitness(dialedAddr string, remotePeerID string, remoteIP string)

	// HasRelayAddress 是否有 Relay 地址
	HasRelayAddress() bool

	// HasVerifiedDirectAddress 是否有已验证的直连地址
	HasVerifiedDirectAddress() bool

	// Start 启动协调器
	Start(ctx context.Context) error

	// Stop 停止协调器
	Stop() error
}

// ============================================================================
//                              Dial-Back 服务接口
// ============================================================================

// StreamOpener 流打开接口，用于回拨验证
type StreamOpener interface {
	OpenStream(ctx context.Context, peerID string, protocolID string) (StreamReadWriteCloser, error)
}

// StreamReadWriteCloser 流读写关闭接口
type StreamReadWriteCloser interface {
	Read(p []byte) (n int, err error)
	Write(p []byte) (n int, err error)
	Close() error
}

// DialBackService 回拨验证服务接口
type DialBackService interface {
	// VerifyAddresses 验证候选地址的可达性
	VerifyAddresses(ctx context.Context, helperID string, candidateAddrs []string) (reachable []string, err error)

	// HandleDialBackRequest 处理来自其他节点的回拨请求
	HandleDialBackRequest(ctx context.Context, req *DialBackRequest) *DialBackResponse

	// SetStreamOpener 设置流打开器
	// 必须在 Start 之前调用，否则将使用模拟验证
	SetStreamOpener(opener StreamOpener)

	// Start 启动服务
	Start(ctx context.Context) error

	// Stop 停止服务
	Stop() error
}

// ============================================================================
//                              验证结果
// ============================================================================

// VerificationResult 地址验证结果
type VerificationResult struct {
	Addr       string        // 被验证的地址
	Reachable  bool          // 是否可达
	VerifiedBy string        // 验证者节点 ID
	VerifiedAt time.Time     // 验证时间
	Latency    time.Duration // 回拨延迟
	Error      error         // 验证失败原因
}

// ============================================================================
//                              配置
// ============================================================================

// ReachabilityConfig 可达性验证配置
type ReachabilityConfig struct {
	// EnableDialBack 是否启用 dial-back 验证
	EnableDialBack bool

	// TrustedHelpers 可信 helper 列表
	TrustedHelpers []string

	// DialBackTimeout 回拨超时时间
	DialBackTimeout time.Duration

	// RequestTimeout 请求超时时间
	RequestTimeout time.Duration

	// MaxConcurrentDialBacks 最大并发回拨数
	MaxConcurrentDialBacks int

	// MinVerifications 最小验证次数
	MinVerifications int

	// VerificationInterval 验证间隔
	VerificationInterval time.Duration

	// EnableAsHelper 是否作为协助节点
	EnableAsHelper bool

	// MinWitnesses 最小见证数
	MinWitnesses int

	// WitnessIPv4Prefix IPv4 见证去重前缀长度
	WitnessIPv4Prefix int

	// WitnessIPv6Prefix IPv6 见证去重前缀长度
	WitnessIPv6Prefix int

	// DirectAddrStorePath 直连地址存储文件路径
	DirectAddrStorePath string

	// CandidateTTL 候选地址 TTL
	CandidateTTL time.Duration

	// VerifiedTTL 已验证地址 TTL
	VerifiedTTL time.Duration

	// MaxVerifiedDirectAddrs 对外通告的直连地址上限
	// 0 表示使用默认值（避免对用户展示过多历史端口）。
	MaxVerifiedDirectAddrs int

	// TrustSTUNAddresses 信任 STUN 发现的地址
	//
	// 启用后，STUN 发现的地址将直接标记为已验证（verified），
	// 无需通过 dial-back 或 witness 验证。
	//
	// 适用场景：
	//   - 云服务器部署（VPC 环境，公网 IP 由 NAT Gateway 提供）
	//   - 已知公网可达的环境
	//
	// 风险提示：
	//   - 仅在受控环境中启用
	//   - 如果 STUN 服务器被劫持，可能导致地址欺骗
	//
	// 默认值：false（需要显式启用）
	TrustSTUNAddresses bool
}

// DefaultReachabilityConfig 返回默认配置
func DefaultReachabilityConfig() *ReachabilityConfig {
	return &ReachabilityConfig{
		EnableDialBack:         true,
		TrustedHelpers:         nil,
		DialBackTimeout:        DefaultDialBackTimeout,
		RequestTimeout:         DefaultRequestTimeout,
		MaxConcurrentDialBacks: MaxConcurrentDialBacks,
		MinVerifications:       1,
		VerificationInterval:   5 * time.Minute,
		EnableAsHelper:         true,
		MinWitnesses:           DefaultMinWitnesses,
		WitnessIPv4Prefix:      DefaultWitnessIPv4Prefix,
		WitnessIPv6Prefix:      DefaultWitnessIPv6Prefix,
		DirectAddrStorePath:    "",
		CandidateTTL:           2 * time.Hour,
		VerifiedTTL:            24 * time.Hour,
		MaxVerifiedDirectAddrs: 3,
		TrustSTUNAddresses:     false, // 默认不信任，需要显式启用
	}
}
