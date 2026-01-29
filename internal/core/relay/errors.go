package relay

import (
	"errors"
	"sync"
	"time"
	
	"github.com/dep2p/go-dep2p/internal/core/nat"
)

// Sentinel errors
var (
	// 通用错误
	ErrNoRelayAvailable      = errors.New("relay: no relay available")
	ErrInvalidConfig         = errors.New("relay: invalid config")
	ErrServiceClosed         = errors.New("relay: service closed")
	ErrAlreadyStarted        = errors.New("relay: already started")
	ErrNotPubliclyReachable  = errors.New("relay: node is not publicly reachable")

	// 配置错误
	ErrInvalidRelayAddress = errors.New("relay: invalid relay address")
	ErrCannotRelayToSelf   = errors.New("relay: cannot relay to self")
	
	// 资源限制错误
	ErrBandwidthExceeded     = errors.New("relay: bandwidth exceeded")
	ErrDurationExceeded      = errors.New("relay: duration exceeded")
	ErrResourceLimitExceeded = errors.New("relay: resource limit exceeded")
	ErrTooManyCircuits       = errors.New("relay: too many circuits")
	
	// 协议错误
	ErrProtocolNotAllowed = errors.New("relay: protocol not allowed")
	ErrMalformedMessage   = errors.New("relay: malformed message")
	
	// 认证错误
	ErrNotRealmMember    = errors.New("relay: not realm member")
	ErrPermissionDenied  = errors.New("relay: permission denied")
	
	// 预约错误
	ErrNoReservation       = errors.New("relay: no reservation")
	ErrReservationFailed   = errors.New("relay: reservation failed")
	ErrReservationRefused  = errors.New("relay: reservation refused")
	
	// 连接错误
	ErrConnectionFailed    = errors.New("relay: connection failed")
	ErrRelayTimeout        = errors.New("relay: timeout")
	ErrInvalidStateTransition = errors.New("relay: invalid state transition")
)

// RelayState 中继状态
type RelayState int

const (
	// RelayStateNone 未配置
	RelayStateNone RelayState = iota
	// RelayStateConfigured 已配置，未连接
	RelayStateConfigured
	// RelayStateConnecting 正在连接
	RelayStateConnecting
	// RelayStateConnected 已连接
	RelayStateConnected
	// RelayStateFailed 连接失败
	RelayStateFailed
)

func (s RelayState) String() string {
	switch s {
	case RelayStateNone:
		return "None"
	case RelayStateConfigured:
		return "Configured"
	case RelayStateConnecting:
		return "Connecting"
	case RelayStateConnected:
		return "Connected"
	case RelayStateFailed:
		return "Failed"
	default:
		return "Unknown"
	}
}

// RelayInfo 中继信息
type RelayInfo struct {
	ID          string   // 中继节点 ID
	Addrs       []string // 节点地址列表（用于区域感知选路）
	Latency     int64    // 延迟（毫秒）
	Capacity    float64  // 容量（0-1）
	Reliability float64  // 可靠性（0-1）
}

// RelayCandidate 中继候选
//
// 表示 Realm 内可作为中继的节点。
// 只有公网可达（ReachabilityPublic）的成员才能成为候选。
type RelayCandidate struct {
	PeerID       string            // 节点 ID
	Addrs        []string          // 地址列表
	Reachability nat.Reachability  // 可达性状态
	LastSeen     time.Time         // 最后活跃时间
	Score        int               // 评分（由 Selector 计算）
}

// RelayCandidatePool 中继候选池
//
// 管理 Realm 内的中继候选列表。
// 自动根据成员的可达性状态添加/移除候选。
type RelayCandidatePool struct {
	mu         sync.RWMutex
	realmID    string
	candidates map[string]*RelayCandidate // peerID -> candidate
	maxSize    int                        // 最大候选数量
	selector   *Selector                  // 中继选择器
	metrics    *CandidateMetrics          // 指标收集器（可选）
}
