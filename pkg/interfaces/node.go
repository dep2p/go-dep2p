// Package interfaces 定义 DeP2P 公共接口
//
// 本文件定义 Node 接口，是 DeP2P 的顶层 API 入口。
package interfaces

import (
	"context"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ════════════════════════════════════════════════════════════════════════════
// ReadyLevel 就绪级别（讨论稿 Section 7.4 对齐）
// ════════════════════════════════════════════════════════════════════════════

// ReadyLevel 节点就绪级别
//
// 表示节点在启动过程中的就绪程度，用于用户感知启动进度。
// 级别递增表示更高的就绪程度。
type ReadyLevel int

const (
	// ReadyLevelCreated Node 对象已创建，未启动
	ReadyLevelCreated ReadyLevel = iota

	// ReadyLevelNetwork 传输层就绪，能发起出站连接
	//
	// 此级别表示：
	//   - Transport 已绑定端口
	//   - 能发起出站连接
	//   - 但可能还未加入 DHT 网络
	ReadyLevelNetwork

	// ReadyLevelDiscovered 全局 DHT 入网成功，能发现其他节点
	//
	// 此级别表示：
	//   - Bootstrap 完成
	//   - 已加入全局 DHT 网络
	//   - 能通过 DHT 发现其他节点
	ReadyLevelDiscovered

	// ReadyLevelReachable 可达性验证完成，能被其他节点发现
	//
	// 此级别表示：
	//   - 已发布 PeerRecord 到全局 DHT
	//   - 其他节点可通过 DHT 找到本节点
	//   - 这是 Node 级别的最高就绪状态（未加入 Realm）
	ReadyLevelReachable

	// ReadyLevelRealmReady Realm 加入完成，Realm 成员可达
	//
	// 对齐设计文档 Section 7.4 Level 4。
	// 此级别表示：
	//   - 已成功加入至少一个 Realm
	//   - 已发布 RealmPeerRecord 到 DHT
	//   - Realm 成员可通过 DHT 找到本节点
	//   - 这是 Node + Realm 的最高就绪状态
	ReadyLevelRealmReady
)

// String 返回就绪级别的字符串表示
func (l ReadyLevel) String() string {
	switch l {
	case ReadyLevelCreated:
		return "created"
	case ReadyLevelNetwork:
		return "network"
	case ReadyLevelDiscovered:
		return "discovered"
	case ReadyLevelReachable:
		return "reachable"
	case ReadyLevelRealmReady:
		return "realm_ready"
	default:
		return "unknown"
	}
}

// Node 定义 DeP2P 节点的顶层接口
//
// Node 是用户与 DeP2P 交互的主要入口点，聚合了所有核心功能。
type Node interface {
	// ID 返回节点的唯一标识符
	ID() string

	// Host 返回底层 Host 实例
	Host() Host

	// Realm 获取或创建指定 ID 的隔离域
	Realm(id string) (Realm, error)

	// Discovery 返回发现服务
	Discovery() Discovery

	// Metrics 返回监控指标服务
	Metrics() Metrics

	// NetworkChange 通知节点网络可能已变化
	//
	// 在某些平台（如 Android）上，系统无法自动检测网络变化，
	// 应用需要在收到系统网络变化回调时调用此方法。
	//
	// 即使网络实际未变化，调用此方法也不会有副作用。
	NetworkChange()

	// OnNetworkChange 注册网络变化回调
	//
	// 当检测到网络变化时，会调用注册的回调函数。
	// 可以用于应用层做相应处理，如重新获取配置等。
	OnNetworkChange(callback func(event NetworkChangeEvent))

	// Start 启动节点
	Start(ctx context.Context) error

	// Stop 停止节点
	Stop(ctx context.Context) error

	// Close 关闭节点并释放资源
	Close() error

	// ════════════════════════════════════════════════════════════════════════
	// ReadyLevel API（讨论稿 Section 7.4 对齐）
	// ════════════════════════════════════════════════════════════════════════

	// ReadyLevel 返回当前就绪级别
	//
	// 就绪级别表示节点启动过程中的当前阶段：
	//   - ReadyLevelCreated: 已创建，未启动
	//   - ReadyLevelNetwork: 传输层就绪
	//   - ReadyLevelDiscovered: DHT 入网成功
	//   - ReadyLevelReachable: 可达性验证完成
	//
	// 示例：
	//
	//	level := node.ReadyLevel()
	//	if level >= interfaces.ReadyLevelDiscovered {
	//	    // 可以使用 DHT 发现功能
	//	}
	ReadyLevel() ReadyLevel

	// WaitReady 等待到达指定就绪级别
	//
	// 阻塞直到节点达到指定的就绪级别或 context 被取消。
	// 如果当前级别已经 >= 目标级别，立即返回 nil。
	//
	// 参数：
	//   - ctx: 上下文，可用于设置超时或取消等待
	//   - level: 目标就绪级别
	//
	// 返回：
	//   - err: 如果 context 被取消或超时，返回对应错误
	//
	// 示例：
	//
	//	// 等待 DHT 入网完成，最多等 30 秒
	//	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	//	defer cancel()
	//	if err := node.WaitReady(ctx, interfaces.ReadyLevelDiscovered); err != nil {
	//	    log.Printf("等待超时: %v", err)
	//	}
	WaitReady(ctx context.Context, level ReadyLevel) error

	// OnReadyLevelChange 注册就绪级别变化回调
	//
	// 当就绪级别发生变化时，会调用注册的回调函数。
	// 回调在级别变化时同步调用，请勿在回调中执行耗时操作。
	//
	// 示例：
	//
	//	node.OnReadyLevelChange(func(level interfaces.ReadyLevel) {
	//	    log.Printf("Ready level changed: %v", level)
	//	})
	OnReadyLevelChange(callback func(level ReadyLevel))

	// ════════════════════════════════════════════════════════════════════════
	// Bootstrap 能力开关（ADR-0009）
	// ════════════════════════════════════════════════════════════════════════

	// EnableBootstrap 启用 Bootstrap 能力
	//
	// 将当前节点设置为引导节点，为网络中的新节点提供初始对等方发现服务。
	// 启用后，节点将：
	//   - 维护扩展的节点存储（最多 50,000 个节点）
	//   - 定期探测存储节点的存活状态
	//   - 主动通过 Random Walk 发现新节点
	//   - 响应 FIND_NODE 请求，返回最近 K 个节点
	//
	// 前置条件：
	//   - 节点必须有公网可达地址（非 NAT 后）
	//
	// 所有运营参数使用内置默认值，用户无需也无法配置。
	EnableBootstrap(ctx context.Context) error

	// DisableBootstrap 禁用 Bootstrap 能力
	//
	// 停止作为引导节点服务，但保留已存储的节点信息（下次启用时可快速恢复）。
	DisableBootstrap(ctx context.Context) error

	// IsBootstrapEnabled 查询 Bootstrap 能力是否已启用
	IsBootstrapEnabled() bool

	// BootstrapStats 获取 Bootstrap 统计信息
	//
	// 返回当前 Bootstrap 状态和统计数据。
	// 如果 Bootstrap 未启用，Enabled 字段为 false。
	BootstrapStats() BootstrapStats

	// ════════════════════════════════════════════════════════════════════════
	// Relay 能力开关（v2.0 统一接口）
	// ════════════════════════════════════════════════════════════════════════

	// EnableRelay 启用 Relay 能力
	//
	// 将当前节点设置为中继服务器，为 NAT 后的节点提供中继服务。
	//
	// 前置条件：
	//   - 节点必须有公网可达地址（非 NAT 后）
	//
	// 所有资源限制参数使用内置默认值，用户无需也无法配置。
	EnableRelay(ctx context.Context) error

	// DisableRelay 禁用 Relay 能力
	//
	// 停止作为中继服务。已建立的中继电路会被优雅关闭。
	DisableRelay(ctx context.Context) error

	// IsRelayEnabled 查询 Relay 能力是否已启用
	IsRelayEnabled() bool

	// SetRelayAddr 设置要使用的 Relay 地址（客户端使用）
	//
	// 指定节点应使用的 Relay 地址。
	//
	// 参数：
	//   - addr: Relay 的完整 multiaddr 地址
	SetRelayAddr(addr types.Multiaddr) error

	// RemoveRelayAddr 移除 Relay 地址配置
	RemoveRelayAddr() error

	// RelayAddr 获取当前配置的 Relay 地址
	//
	// 返回当前配置的 Relay 地址。
	// 如果未配置，返回 (nil, false)。
	RelayAddr() (types.Multiaddr, bool)

	// RelayStats 获取 Relay 统计信息
	//
	// 返回当前 Relay 状态和统计数据。
	// 如果 Relay 未启用，Enabled 字段为 false。
	RelayStats() RelayStats
}

// 注意: BootstrapStats 定义在 discovery.go 中
// 注意: RelayStats 定义在 relay.go 中

// ════════════════════════════════════════════════════════════════════════════
// Health Check 相关定义（被 Node.Health 方法使用）
// ════════════════════════════════════════════════════════════════════════════

// HealthState 健康状态枚举
type HealthState int

const (
	HealthStateHealthy   HealthState = iota // 健康
	HealthStateDegraded                     // 降级
	HealthStateUnhealthy                    // 不健康
)

// String 返回状态字符串
func (s HealthState) String() string {
	switch s {
	case HealthStateHealthy:
		return "healthy"
	case HealthStateDegraded:
		return "degraded"
	case HealthStateUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

// HealthStatus 健康状态
type HealthStatus struct {
	Status  HealthState            // 状态
	Message string                 // 描述信息
	Details map[string]interface{} // 详细信息
}

// NewHealthStatusWithDetails 创建带详细信息的健康状态
func NewHealthStatusWithDetails(state HealthState, message string, details map[string]interface{}) HealthStatus {
	return HealthStatus{
		Status:  state,
		Message: message,
		Details: details,
	}
}

// HealthyStatus 创建健康状态
func HealthyStatus(message string) HealthStatus {
	return HealthStatus{Status: HealthStateHealthy, Message: message}
}

// UnhealthyStatus 创建不健康状态
func UnhealthyStatus(message string) HealthStatus {
	return HealthStatus{Status: HealthStateUnhealthy, Message: message}
}

// HealthChecker 健康检查接口
type HealthChecker interface {
	// Check 执行健康检查
	Check(ctx context.Context) HealthStatus
}
