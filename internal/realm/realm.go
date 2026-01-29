package realm

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/lifecycle"
	"github.com/dep2p/go-dep2p/internal/core/relay/addressbook"
	"github.com/dep2p/go-dep2p/internal/realm/connector"
	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	"github.com/dep2p/go-dep2p/internal/realm/member"
	realmprotocol "github.com/dep2p/go-dep2p/internal/realm/protocol"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	memberleavepb "github.com/dep2p/go-dep2p/pkg/lib/proto/realm/memberleave"
	"github.com/dep2p/go-dep2p/pkg/protocol"
	"github.com/dep2p/go-dep2p/pkg/types"
	"google.golang.org/protobuf/proto"
)

// ============================================================================
//                              Realm 实现
// ============================================================================

// pendingAuth 待重试认证的节点（认证重试机制）
type pendingAuth struct {
	peerID    string
	attempts  int
	nextRetry time.Time
}

// realmImpl Realm 实现
type realmImpl struct {
	mu sync.RWMutex

	// 基础信息
	id   string
	name string
	psk  []byte

	// 子模块引用
	auth    interfaces.Authenticator
	member  interfaces.MemberManager
	routing interfaces.Router
	gateway interfaces.Gateway

	// Manager 引用
	manager *Manager

	// CapabilityManager（用于能力广播）
	capabilityManager CapabilityBroadcaster

	// Protocol 层服务（全局单例，通过 Manager 注入）
	messaging pkgif.Messaging
	pubsub    pkgif.PubSub
	streams   pkgif.Streams
	liveness  pkgif.Liveness

	// 连接器（"仅 ID 连接"支持）
	connector *connector.Connector

	// AuthHandler（协议层认证处理器，用于自动认证新连接）
	authHandler *realmprotocol.AuthHandler

	// EventBus 引用（用于订阅连接事件）
	eventBus pkgif.EventBus

	// 连接事件订阅（用于取消订阅）
	connSub    pkgif.Subscription
	disconnSub pkgif.Subscription

	// 成员同步 Topic 和订阅
	memberSyncTopic pkgif.Topic
	memberSyncSub   pkgif.TopicSubscription

	// 成员同步重试标志（防止多个重试 goroutine）
	syncRetrying atomic.Bool

	// 认证去重：正在认证中的 peer 集合
	// 防止同一 peer 的多个连接事件触发重复认证
	authenticatingPeers   map[string]struct{}
	authenticatingPeersMu sync.Mutex

	// 待重试认证的节点（认证重试机制）
	// 当认证因"协议不支持"失败时（对方可能还没就绪），安排重试
	pendingAuths   map[string]*pendingAuth
	pendingAuthsMu sync.Mutex

	// MemberLeave 防重放缓存
	// key: peer_id + realm_id + timestamp，用于防止重复处理同一消息
	memberLeaveProcessed   map[string]time.Time
	memberLeaveProcessedMu sync.Mutex

	// 基础设施节点集合（Bootstrap、Relay 等）
	// 这些节点不是 Realm 成员，跳过对它们的认证尝试
	infrastructurePeers map[string]struct{}

	// 地址簿服务（用于网络变化时通知 Relay）
	addressBookService AddressBookServiceInterface

	// Step B2 对齐：基于 Discovery 的成员同步
	synchronizer *member.Synchronizer // 成员同步器（基于 Discovery/Rendezvous）
	discovery    pkgif.Discovery      // 发现服务引用

	// 生命周期协调器引用（对齐 20260125-node-lifecycle-cross-cutting.md）
	lifecycleCoordinator *lifecycle.Coordinator

	// DHT 引用（用于权威解析入口节点）
	dht pkgif.DHT

	// 状态
	active atomic.Bool
	ctx    context.Context
	cancel context.CancelFunc
}

// AddressBookServiceInterface 地址簿服务接口
//
// 用于网络变化时重新注册地址到 Relay。
type AddressBookServiceInterface interface {
	// RegisterSelf 向 Relay 注册自己的地址
	RegisterSelf(ctx context.Context, relayPeerID string) error
	// GetRelayPeerID 获取当前注册的 Relay 节点 ID
	GetRelayPeerID() string
}

// memberSyncTopicName 生成 Realm 作用域的成员同步 topic 名称
//
// Step B2 对齐：将 Topic 改为 Realm 作用域，避免跨 Realm 污染
// 格式：/dep2p/realm/<realmID>/members
func memberSyncTopicName(realmID string) string {
	return fmt.Sprintf("%s/%s/members", protocol.PrefixRealm, realmID)
}

const (
	memberSyncJoinV2Prefix = "join2:"
	memberSyncSyncV2Prefix = "sync2:"
)

type memberSyncJoinV2 struct {
	PeerID string   `json:"peer_id"`
	Addrs  []string `json:"addrs,omitempty"`
}

type memberSyncListV2 struct {
	Members []memberSyncJoinV2 `json:"members"`
}

// CapabilityBroadcaster 能力广播接口
type CapabilityBroadcaster interface {
	// ReBroadcast 重新广播能力
	ReBroadcast(ctx context.Context, newAddrs []string) error

	// Start 启动能力管理器（P1 修复）
	Start() error

	// Stop 停止能力管理器（P1 修复）
	Stop() error

	// P0 修复：SendToPeer 向指定节点发送能力公告（单播）
	SendToPeer(ctx context.Context, peerID string) error
}

// ============================================================================
//                              基础信息
// ============================================================================

// ID 返回 Realm ID
func (r *realmImpl) ID() string {
	return r.id
}

// Name 返回 Realm 名称
func (r *realmImpl) Name() string {
	return r.name
}

// PSK 返回 PSK（仅用于内部）
func (r *realmImpl) PSK() []byte {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// 返回副本以避免修改
	psk := make([]byte, len(r.psk))
	copy(psk, r.psk)
	return psk
}

// ============================================================================
//                              成员管理
// ============================================================================

// Members 返回成员列表
func (r *realmImpl) Members() []string {
	if r.member == nil {
		return []string{}
	}

	ctx := context.Background()
	members, err := r.member.List(ctx, nil)
	if err != nil {
		return []string{}
	}

	peerIDs := make([]string, 0, len(members))
	for _, m := range members {
		peerIDs = append(peerIDs, m.PeerID)
	}

	return peerIDs
}

// IsMember 检查是否为成员
func (r *realmImpl) IsMember(peerID string) bool {
	if r.member == nil {
		return false
	}

	ctx := context.Background()
	return r.member.IsMember(ctx, peerID)
}

// Connect 连接 Realm 成员或潜在成员
//
// 支持多种输入格式（自动检测）：
//   - ConnectionTicket: dep2p://base64...
//   - Full Address: /ip4/x.x.x.x/udp/port/quic-v1/p2p/12D3KooW...
//   - 纯 NodeID: 12D3KooW...
//
// 连接流程：
//  1. 解析 target，提取 NodeID 和地址提示
//  2. 如果目标已是成员，直接使用 Connector 连接
//  3. 如果目标不是成员，先建立底层连接，等待 PSK 认证完成
//  4. 认证完成后返回连接
//
// 连接优先级：直连 → 打洞 → Relay 保底。
func (r *realmImpl) Connect(ctx context.Context, target string) (pkgif.Connection, error) {
	// 1. 解析 target，提取 NodeID 和地址提示
	nodeID, hints := r.parseConnectTarget(target)
	if nodeID == "" {
		return nil, fmt.Errorf("无法解析连接目标: %s", target)
	}

	// 2. 如果目标已是成员，直接使用 Connector 连接
	if r.IsMember(nodeID) {
		if r.connector == nil {
			return nil, ErrRealmInactive
		}
		if len(hints) > 0 {
			result, err := r.connector.ConnectWithHint(ctx, nodeID, hints)
			if err != nil {
				return nil, err
			}
			return result.Conn, nil
		}
		result, err := r.connector.Connect(ctx, nodeID)
		if err != nil {
			return nil, err
		}
		return result.Conn, nil
	}

	// 3. 目标不是成员，先建立底层连接，触发 PSK 认证
	logger.Debug("目标不是 Realm 成员，建立底层连接并等待认证",
		"realmID", r.id,
		"target", truncateID(nodeID))

	if r.manager == nil || r.manager.host == nil {
		return nil, ErrRealmInactive
	}

	// 建立底层连接
	if err := r.manager.host.Connect(ctx, nodeID, hints); err != nil {
		return nil, fmt.Errorf("底层连接失败: %w", err)
	}

	// 4. 等待 PSK 认证完成（认证成功后目标会自动加入成员列表）
	authTimeout := 10 * time.Second
	authCtx, authCancel := context.WithTimeout(ctx, authTimeout)
	defer authCancel()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-authCtx.Done():
			return nil, fmt.Errorf("PSK 认证超时（%v），目标可能不在同一 Realm", authTimeout)
		case <-ticker.C:
			if r.IsMember(nodeID) {
				// 认证完成，使用 Connector 获取连接
				logger.Debug("PSK 认证完成，目标已加入 Realm",
					"realmID", r.id,
					"target", truncateID(nodeID))
				if r.connector == nil {
					return nil, ErrRealmInactive
				}
				result, err := r.connector.Connect(ctx, nodeID)
				if err != nil {
					return nil, err
				}
				return result.Conn, nil
			}
		}
	}
}

// parseConnectTarget 解析连接目标，提取 NodeID 和地址提示
//
// 支持格式：
//   - dep2p://base64... → 解码票据，提取 NodeID 和 AddressHints
//   - /ip4/.../p2p/NodeID → 提取 NodeID 和地址
//   - 纯 NodeID → 直接返回
//
// 安全检查：
//   - 空字符串/空格处理
//   - NodeID 格式验证（Base58）
//   - 票据过期检查（24小时）
//   - 地址格式基本验证
func (r *realmImpl) parseConnectTarget(target string) (nodeID string, hints []string) {
	// 0. 预处理：去除前后空格
	target = strings.TrimSpace(target)
	if target == "" {
		return "", nil
	}

	// 1. ConnectionTicket 格式
	if strings.HasPrefix(target, "dep2p://") {
		ticket, err := types.DecodeConnectionTicket(target)
		if err != nil {
			logger.Debug("票据解码失败", "error", err)
			return "", nil
		}

		// 检查票据是否过期（24小时）
		if ticket.IsExpired(24 * time.Hour) {
			logger.Debug("票据已过期", "timestamp", ticket.Timestamp)
			return "", nil
		}

		// 验证 NodeID 格式
		if !r.isValidNodeID(ticket.NodeID) {
			logger.Debug("票据中的 NodeID 格式无效", "nodeID", truncateID(ticket.NodeID))
			return "", nil
		}

		// 过滤无效地址
		validHints := r.filterValidAddresses(ticket.AddressHints)

		return ticket.NodeID, validHints
	}

	// 2. Multiaddr 格式
	if strings.HasPrefix(target, "/") && strings.Contains(target, "/p2p/") {
		// 提取最后一个 /p2p/ 后面的 NodeID
		parts := strings.Split(target, "/p2p/")
		if len(parts) >= 2 {
			lastPart := parts[len(parts)-1]
			// 去掉可能的后续路径
			if idx := strings.Index(lastPart, "/"); idx > 0 {
				lastPart = lastPart[:idx]
			}

			// 验证 NodeID 格式
			if !r.isValidNodeID(lastPart) {
				logger.Debug("地址中的 NodeID 格式无效", "nodeID", truncateID(lastPart))
				return "", nil
			}

			// 验证地址基本格式
			if !r.isValidMultiaddr(target) {
				logger.Debug("地址格式无效", "addr", target)
				return "", nil
			}

			return lastPart, []string{target}
		}
		return "", nil
	}

	// 3. 纯 NodeID 格式
	if !strings.Contains(target, "/") && !strings.Contains(target, ":") {
		// 验证 NodeID 格式
		if !r.isValidNodeID(target) {
			logger.Debug("NodeID 格式无效", "nodeID", truncateID(target))
			return "", nil
		}
		return target, nil
	}

	return "", nil
}

// isValidNodeID 验证 NodeID 格式
//
// 使用 types.PeerID.Validate() 进行 Base58 格式验证
func (r *realmImpl) isValidNodeID(nodeID string) bool {
	if nodeID == "" {
		return false
	}

	// 长度检查（Base58 编码的 32 字节哈希约 43-44 字符）
	if len(nodeID) < 20 || len(nodeID) > 100 {
		return false
	}

	// 使用 types.PeerID 进行格式验证
	peerID := types.PeerID(nodeID)
	return peerID.Validate() == nil
}

// isValidMultiaddr 验证 Multiaddr 格式
//
// 基本格式检查，防止注入攻击
func (r *realmImpl) isValidMultiaddr(addr string) bool {
	if addr == "" {
		return false
	}

	// 长度检查（防止超长地址攻击）
	if len(addr) > 500 {
		return false
	}

	// 必须以 / 开头
	if !strings.HasPrefix(addr, "/") {
		return false
	}

	// 检查是否包含常见的传输协议
	hasTransport := strings.Contains(addr, "/ip4/") ||
		strings.Contains(addr, "/ip6/") ||
		strings.Contains(addr, "/dns4/") ||
		strings.Contains(addr, "/dns6/")

	if !hasTransport {
		return false
	}

	// 检查是否包含 /p2p/（节点标识）
	if !strings.Contains(addr, "/p2p/") {
		return false
	}

	// 禁止危险字符（防止命令注入）
	dangerousChars := []string{";", "|", "&", "$", "`", "\n", "\r", "\\"}
	for _, c := range dangerousChars {
		if strings.Contains(addr, c) {
			return false
		}
	}

	return true
}

// filterValidAddresses 过滤无效地址
//
// 移除格式无效或潜在危险的地址
func (r *realmImpl) filterValidAddresses(addrs []string) []string {
	if len(addrs) == 0 {
		return nil
	}

	valid := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		if r.isValidMultiaddr(addr) {
			valid = append(valid, addr)
		}
	}

	return valid
}

// ConnectWithHint 使用 NodeID 和地址提示连接 Realm 成员
//
// 与 Connect 类似，但允许用户提供地址提示来加速连接。
// 提示地址会被优先尝试，如果失败则回退到自动发现流程。
func (r *realmImpl) ConnectWithHint(ctx context.Context, target string, hints []string) (pkgif.Connection, error) {
	// 1. 验证目标是 Realm 成员
	if !r.IsMember(target) {
		return nil, connector.ErrNotMember
	}

	// 2. 检查连接器是否可用
	if r.connector == nil {
		return nil, ErrRealmInactive
	}

	// 3. 使用 Connector 带提示连接
	result, err := r.connector.ConnectWithHint(ctx, target, hints)
	if err != nil {
		return nil, err
	}

	return result.Conn, nil
}

// SetConnector 设置连接器
func (r *realmImpl) SetConnector(c *connector.Connector) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.connector = c
}

// MemberCount 返回成员数量
func (r *realmImpl) MemberCount() int {
	if r.member == nil {
		return 0
	}

	return r.member.GetTotalCount()
}

// ============================================================================
//                              认证
// ============================================================================

// Authenticate 验证对方身份
func (r *realmImpl) Authenticate(ctx context.Context, peerID string, proof []byte) (bool, error) {
	if r.auth == nil {
		return false, ErrAuthFailed
	}

	return r.auth.Authenticate(ctx, peerID, proof)
}

// GenerateProof 生成认证证明
func (r *realmImpl) GenerateProof(ctx context.Context) ([]byte, error) {
	if r.auth == nil {
		return nil, ErrAuthFailed
	}

	return r.auth.GenerateProof(ctx)
}

// ============================================================================
//                              路由
// ============================================================================

// FindRoute 查找路由
func (r *realmImpl) FindRoute(ctx context.Context, targetPeerID string) (*interfaces.Route, error) {
	if r.routing == nil {
		return nil, ErrRoutingFailed
	}

	return r.routing.FindRoute(ctx, targetPeerID)
}

// SelectBestRoute 选择最佳路由
func (r *realmImpl) SelectBestRoute(ctx context.Context, routes []*interfaces.Route, policy interfaces.RoutingPolicy) (*interfaces.Route, error) {
	if r.routing == nil {
		return nil, ErrRoutingFailed
	}

	return r.routing.SelectBestRoute(ctx, routes, policy)
}

// InvalidateRoute 使路由失效
func (r *realmImpl) InvalidateRoute(peerID string) {
	if r.routing != nil {
		r.routing.InvalidateRoute(peerID)
	}
}

// GetRouteTable 获取路由表
func (r *realmImpl) GetRouteTable() interfaces.RouteTable {
	if r.routing == nil {
		return nil
	}

	return r.routing.GetRouteTable()
}

// ============================================================================
//                              网关
// ============================================================================

// RelayRequest 中继转发请求
func (r *realmImpl) RelayRequest(ctx context.Context, req *interfaces.RelayRequest) error {
	if r.gateway == nil {
		return ErrGatewayFailed
	}

	return r.gateway.Relay(ctx, req)
}

// GetReachableNodes 获取可达节点
func (r *realmImpl) GetReachableNodes() []string {
	if r.gateway == nil {
		return []string{}
	}

	return r.gateway.GetReachableNodes()
}

// ReportGatewayState 报告网关状态
func (r *realmImpl) ReportGatewayState(ctx context.Context) (*interfaces.GatewayState, error) {
	if r.gateway == nil {
		return nil, ErrGatewayFailed
	}

	return r.gateway.ReportState(ctx)
}

// UpdateReachableNodes 更新可达节点
//
// 该方法将可达节点列表同步到 Routing 层的 Gateway 适配器。
// 用于优化中继路径选择。
func (r *realmImpl) UpdateReachableNodes(nodes []string) {
	// 1. 更新路由表（如果存在）
	if r.routing != nil {
		table := r.routing.GetRouteTable()
		if table != nil {
			// 标记节点可达性
			for _, nodeID := range nodes {
				node, err := table.GetNode(nodeID)
				if err == nil && node != nil {
					node.IsReachable = true
				}
			}
		}
	}

	logger.Debug("更新可达节点", "count", len(nodes), "realmID", r.id)
}

// ============================================================================
//                              服务门面（返回全局 Protocol 层服务）
// ============================================================================

// Messaging 返回 Messaging 服务
func (r *realmImpl) Messaging() pkgif.Messaging {
	return r.messaging
}

// PubSub 返回 PubSub 服务
func (r *realmImpl) PubSub() pkgif.PubSub {
	return r.pubsub
}

// Streams 返回 Streams 服务
func (r *realmImpl) Streams() pkgif.Streams {
	return r.streams
}

// Liveness 返回 Liveness 服务
func (r *realmImpl) Liveness() pkgif.Liveness {
	return r.liveness
}

// newTicker 创建 Ticker
func newTicker(d time.Duration) *time.Ticker {
	return time.NewTicker(d)
}

// ============================================================================
//                              状态同步
// ============================================================================

// syncAuthToMember 同步 Auth → Member
//
// 注：Auth 到 Member 的同步已通过事件驱动方式实现：
// - EvtPeerConnected 事件触发 authenticateAndAddMember
// - PSK 认证成功后通过 onPeerAuthenticated 回调添加成员

// syncMemberToRouting 同步 Member → Routing
func (r *realmImpl) syncMemberToRouting(ctx context.Context) error {
	if r.member == nil || r.routing == nil {
		return nil
	}

	// 获取所有成员
	members, err := r.member.List(ctx, nil)
	if err != nil {
		return err
	}

	// 更新路由表
	table := r.routing.GetRouteTable()
	if table == nil {
		return nil
	}

	for _, m := range members {
		if m.Online {
			// 在线成员添加到路由表
			node := &interfaces.RouteNode{
				PeerID:      m.PeerID,
				IsReachable: true,
				LastSeen:    m.LastSeen,
			}
			table.AddNode(node)
		} else {
			// 离线成员从路由表移除
			table.RemoveNode(m.PeerID)
		}
	}

	return nil
}

// syncRoutingToGateway 同步 Routing → Gateway
//
// 从路由表获取可达节点，同步到 Gateway 以支持中继服务。
func (r *realmImpl) syncRoutingToGateway(_ context.Context) error {
	if r.routing == nil || r.gateway == nil {
		return nil
	}

	// 获取路由表
	table := r.routing.GetRouteTable()
	if table == nil {
		return nil
	}

	// 获取所有可达节点
	allNodes := table.GetAllNodes()
	reachableNodes := make([]string, 0, len(allNodes))

	for _, node := range allNodes {
		if node.IsReachable {
			reachableNodes = append(reachableNodes, node.PeerID)
		}
	}

	// 更新到本地可达节点列表
	r.UpdateReachableNodes(reachableNodes)

	logger.Debug("同步路由到网关",
		"reachableCount", len(reachableNodes),
		"totalNodes", len(allNodes),
		"realmID", r.id)

	return nil
}

// ============================================================================
//                              生命周期
// ============================================================================

// start 启动 Realm
func (r *realmImpl) start(ctx context.Context) error {
	if r.active.Load() {
		return nil
	}

	// 使用 context.Background() 而不是传入的 ctx
	// 保证子模块的生命周期不受上层 ctx 取消的影响
	r.ctx, r.cancel = context.WithCancel(context.Background())

	// 依次启动子模块（Auth → Member → Routing → Gateway）
	// 注：Auth 模块不需要显式 Start，它在认证时按需工作

	if r.member != nil {
		// 注入 Host 引用到 MemberManager（用于获取本地 PeerID）
		if r.manager != nil && r.manager.host != nil {
			if hostSetter, ok := r.member.(interface{ SetHost(pkgif.Host) }); ok {
				hostSetter.SetHost(r.manager.host)
				logger.Debug("Host 已注入到 MemberManager", "realmID", r.id)
			}
		}

		if err := r.member.Start(ctx); err != nil {
			return err
		}

		// 将自己添加为 Realm 成员（重要：否则 PubSub 等服务无法发送消息）
		if r.manager != nil && r.manager.host != nil {
			localPeerID := string(r.manager.host.ID())
			now := time.Now()
			memberInfo := &interfaces.MemberInfo{
				PeerID:   localPeerID,
				RealmID:  r.id,
				Role:     interfaces.RoleMember,
				Online:   true,
				JoinedAt: now,
				LastSeen: now,
			}
			// P0 修复：写入本地可分享地址，方便地址传播
			memberInfo.Addrs = r.getPeerAddrs(localPeerID)
			if err := r.member.Add(ctx, memberInfo); err != nil {
				// 如果是重复添加错误，忽略（例如从存储恢复的情况）
				logger.Debug("添加自己为成员", "err", err, "peerID", localPeerID)
			}
		}
	}

	if r.routing != nil {
		if err := r.routing.Start(ctx); err != nil {
			return err
		}
	}

	if r.gateway != nil {
		if err := r.gateway.Start(ctx); err != nil {
			return err
		}
	}

	// 启动 AuthHandler（用于自动认证新连接）
	if r.authHandler != nil {
		// 设置认证成功回调（入站认证成功时调用）
		r.authHandler.SetOnAuthSuccess(func(peerID string) {
			r.onPeerAuthenticated(r.ctx, peerID)
		})

		// 设置成员交换回调（认证成功后即时同步成员）
		r.authHandler.SetMemberExchangeCallbacks(
			r.getMemberListForExchange,
			r.mergeMemberListFromExchange,
		)

		if err := r.authHandler.Start(ctx); err != nil {
			logger.Warn("启动 AuthHandler 失败", "err", err)
			// 非致命错误，继续
		}
	}

	// 生命周期阶段 B1: PSK 认证完成
	// AuthHandler 启动成功意味着 PSK 已派生 RealmID 和 AuthKey，认证服务就绪
	if r.lifecycleCoordinator != nil {
		r.lifecycleCoordinator.Complete(lifecycle.PhaseB1PSKAuth)
	}

	// 订阅连接事件（用于自动触发认证）
	if err := r.subscribeConnectionEvents(r.ctx); err != nil {
		logger.Warn("订阅连接事件失败", "err", err)
		// 非致命错误，继续
	}

	// 启动 Protocol 服务（Messaging、PubSub、Streams、Liveness）
	if err := startProtocolServices(ctx, r); err != nil {
		return err
	}

	// 订阅成员同步消息（在 PubSub 启动后）
	if err := r.subscribeMemberSync(r.ctx); err != nil {
		logger.Warn("订阅成员同步失败", "err", err)
		// 非致命错误，继续
	} else {
		// 启动延迟的全量同步请求
		// 等待 Mesh 形成后再请求（约 1-2 秒后）
		go func() {
			select {
			case <-time.After(2 * time.Second):
				if r.active.Load() {
					r.requestFullMemberSync(r.ctx)
				}
			case <-r.ctx.Done():
			}
		}()
	}

	// Step B2 对齐：启动基于 Discovery 的成员同步器
	if r.synchronizer != nil {
		if err := r.synchronizer.Start(ctx); err != nil {
			logger.Warn("启动成员同步器失败", "realmID", r.id, "err", err)
			// 非致命错误，Gossip 同步仍可用
		} else {
			logger.Info("成员同步器已启动", "realmID", r.id)
		}
	}

	// 成员发现通过 DHT Provider Record 完成（见 discoveryLoop + ProvideRealmMembership）
	// 不再使用 Discovery.Advertise，避免 PubSub Topic 与 DHT Key 格式混用

	// 启动 CapabilityManager（能力广播）
	if r.capabilityManager != nil {
		if err := r.capabilityManager.Start(); err != nil {
			logger.Warn("启动 CapabilityManager 失败", "err", err)
			// 非致命错误，继续
		} else {
			logger.Debug("CapabilityManager 已启动", "realmID", r.id)
		}
	}

	r.active.Store(true)

	// 启动认证重试循环（处理认证失败后的重试）
	go r.authRetryLoop(r.ctx)

	// 延迟检查待认证节点（处理连接时机早于 Realm 就绪的情况）
	go r.checkPendingAuthentications(r.ctx)

	// 执行"先发布后发现"的 Join 流程
	go r.joinV2(r.ctx)

	// 注意：B4 阶段（地址簿注册）已移至 A5（Node 级别）
	// 由 Relay Manager 在连接时自动执行
	// 参见 internal/core/relay/manager.go ConnectAndRegister()

	// 启动同步协程
	go r.syncWorker(r.ctx)

	return nil
}

// joinV2 执行"先发布后发现"的 Join 流程
//
// 时序对齐（20260125-node-lifecycle-cross-cutting.md Phase B）：
//  1. 等待地址就绪（A5 gate）
//  2. Step B2/B3: 发布 Provider Record（声明自己是 Realm 成员）- 带重试
//  3. Step B2/B3: 发布 PeerRecord（发布自己的地址）- 带重试
//  4. 标记 B3 完成（DHT 发布完成）
//  5. 启动后台发现循环（B2 成员发现持续进行）
//  6. 标记 B2 完成（发现机制就绪）
//  7. 启动 Provider Record 续期循环
func (r *realmImpl) joinV2(ctx context.Context) {
	maxWaitForAddress := 60 * time.Second
	const maxRetries = 3
	const retryInterval = 2 * time.Second

	// Step 1: 等待地址就绪（A5 gate）
	if r.lifecycleCoordinator != nil {
		logger.Info("v2.0 Join 等待地址就绪", "realmID", r.id)
		waitCtx, waitCancel := context.WithTimeout(ctx, maxWaitForAddress)
		err := r.lifecycleCoordinator.WaitAddressReady(waitCtx)
		waitCancel()
		if err != nil {
			logger.Warn("等待地址就绪超时，继续发布",
				"realmID", r.id,
				"err", err)
		} else {
			logger.Info("地址就绪，开始 v2.0 Join 流程", "realmID", r.id)
		}
	}

	// Step 2: 发布 Provider Record（声明自己是 Realm 成员）- 带重试
	providerPublished := false
	if r.dht != nil {
		for i := 0; i < maxRetries; i++ {
			if err := r.dht.ProvideRealmMembership(ctx, types.RealmID(r.id)); err != nil {
				if i < maxRetries-1 {
					logger.Warn("发布 Provider Record 失败，重试中",
						"realmID", r.id,
						"attempt", i+1,
						"err", err)
					select {
					case <-ctx.Done():
						return
					case <-time.After(retryInterval):
						continue
					}
				} else {
					logger.Error("发布 Provider Record 失败，已达最大重试次数",
						"realmID", r.id,
						"err", err)
				}
			} else {
				logger.Info("Provider Record 发布成功", "realmID", r.id)
				providerPublished = true
				break
			}
		}
	}

	// Step 3: 发布 PeerRecord（发布自己的地址）- 带重试
	if r.dht != nil && r.manager != nil && r.manager.host != nil {
		record := r.buildSignedPeerRecord()
		if record != nil {
			for i := 0; i < maxRetries; i++ {
				if err := r.dht.PublishRealmPeerRecord(ctx, types.RealmID(r.id), record); err != nil {
					if i < maxRetries-1 {
						logger.Warn("发布 PeerRecord 失败，重试中",
							"realmID", r.id,
							"attempt", i+1,
							"err", err)
						select {
						case <-ctx.Done():
							return
						case <-time.After(retryInterval):
							continue
						}
					} else {
						logger.Error("发布 PeerRecord 失败，已达最大重试次数",
							"realmID", r.id,
							"err", err)
					}
				} else {
					logger.Info("PeerRecord 发布成功", "realmID", r.id)
					break
				}
			}
		}
	}

	// Step 4: 标记 B3 阶段完成（DHT 发布完成）
	// 注意：按照"先发布后发现"模式，B3 在 B2 之前完成
	if r.lifecycleCoordinator != nil {
		r.lifecycleCoordinator.Complete(lifecycle.PhaseB3DHTPublish)
	}

	// Step 5: 启动后台发现循环（B2 成员发现持续进行）
	go r.discoveryLoop(ctx)

	// Step 6: 标记 B2 阶段完成（发现循环已启动）
	// B2 表示成员发现机制已就绪，后台循环将持续运行
	if r.lifecycleCoordinator != nil {
		r.lifecycleCoordinator.Complete(lifecycle.PhaseB2MemberDiscovery)
	}

	// Step 7: 启动 Provider Record 续期循环
	if providerPublished {
		go r.providerRepublishLoop(ctx)
	}

	logger.Info("v2.0 Join 流程完成", "realmID", r.id)
}

// providerRepublishLoop Provider Record 续期循环
//
// Provider Record 默认 TTL 是 24 小时，在 TTL/2（12 小时）时自动续期。
// 确保节点持续可被其他 Realm 成员发现。
func (r *realmImpl) providerRepublishLoop(ctx context.Context) {
	// Provider TTL 通常是 24 小时，在 TTL/2 时续期
	const providerTTL = 24 * time.Hour
	interval := providerTTL / 2

	logger.Info("启动 Provider Record 续期循环",
		"realmID", r.id,
		"interval", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Debug("Provider Record 续期循环退出", "realmID", r.id)
			return
		case <-ticker.C:
			if r.dht != nil {
				if err := r.dht.ProvideRealmMembership(ctx, types.RealmID(r.id)); err != nil {
					logger.Warn("Provider Record 续期失败",
						"realmID", r.id,
						"err", err)
				} else {
					logger.Debug("Provider Record 续期成功", "realmID", r.id)
				}
			}
		}
	}
}

// buildSignedPeerRecord 构建签名的 PeerRecord
func (r *realmImpl) buildSignedPeerRecord() []byte {
	if r.manager == nil || r.manager.host == nil {
		return nil
	}

	// 获取本地可分享地址
	addrs := r.manager.host.ShareableAddrs()
	if len(addrs) == 0 {
		addrs = r.manager.host.Addrs()
	}

	if len(addrs) == 0 {
		logger.Debug("无可用地址，跳过 PeerRecord 构建", "realmID", r.id)
		return nil
	}

	// 简单序列化：JSON 格式
	record := map[string]interface{}{
		"peer_id": r.manager.host.ID(),
		"realm":   r.id,
		"addrs":   addrs,
		"ts":      time.Now().Unix(),
	}

	data, err := json.Marshal(record)
	if err != nil {
		logger.Warn("序列化 PeerRecord 失败", "err", err)
		return nil
	}

	return data
}

// discoveryLoop 后台发现循环（指数退避）
//
// 通过 DHT Provider 机制发现 Realm 成员，连接并认证。
func (r *realmImpl) discoveryLoop(ctx context.Context) {
	interval := 2 * time.Second
	maxInterval := 60 * time.Second

	logger.Info("启动 v2.0 发现循环", "realmID", r.id)

	for {
		select {
		case <-ctx.Done():
			logger.Debug("发现循环退出", "realmID", r.id)
			return
		case <-time.After(interval):
			found := r.discoverAndAuthenticate(ctx)
			if found > 0 {
				interval = 2 * time.Second // 发现新成员，重置间隔
				logger.Debug("发现新成员，重置间隔",
					"realmID", r.id,
					"found", found)
			} else {
				// 指数退避
				interval = min(interval*2, maxInterval)
			}
		}
	}
}

// discoverAndAuthenticate 通过 DHT Provider 发现并认证成员
//
// v2.0 增强：添加并发控制，限制同时进行的认证数量，防止资源耗尽。
func (r *realmImpl) discoverAndAuthenticate(ctx context.Context) int {
	if r.dht == nil || r.manager == nil || r.manager.host == nil {
		return 0
	}

	localID := r.manager.host.ID()

	// 通过 DHT 查找 Realm 成员
	memberCh, err := r.dht.FindRealmMembers(ctx, types.RealmID(r.id))
	if err != nil {
		logger.Debug("查找 Realm 成员失败",
			"realmID", r.id,
			"err", err)
		return 0
	}

	// v2.0 并发控制：限制同时进行的认证数量
	const maxConcurrentAuth = 5
	authSemaphore := make(chan struct{}, maxConcurrentAuth)
	var wg sync.WaitGroup

	found := 0
	for {
		select {
		case <-ctx.Done():
			wg.Wait() // 等待所有认证完成
			return found
		case peerID, ok := <-memberCh:
			if !ok {
				wg.Wait() // 等待所有认证完成
				return found
			}

			peerIDStr := string(peerID)
			if peerIDStr == "" || peerIDStr == localID {
				continue
			}

			// 跳过基础设施节点
			if r.isInfrastructurePeer(peerIDStr) {
				continue
			}

			// 跳过已是成员的节点
			if r.member != nil && r.member.IsMember(ctx, peerIDStr) {
				continue
			}

			// 获取成员地址并尝试连接
			record, err := r.dht.FindRealmPeerRecord(ctx, types.RealmID(r.id), types.NodeID(peerID))
			if err != nil {
				logger.Debug("获取成员 PeerRecord 失败",
					"peerID", truncateID(peerIDStr),
					"err", err)
				continue
			}

			// 解析地址并连接
			addrs := r.parseAddrsFromRecord(record)
			if len(addrs) > 0 {
				if err := r.manager.host.Connect(ctx, peerIDStr, addrs); err != nil {
					logger.Debug("连接成员失败",
						"peerID", truncateID(peerIDStr),
						"err", err)
					continue
				}

				// v2.0 并发控制：获取信号量后启动认证
				select {
				case authSemaphore <- struct{}{}:
					wg.Add(1)
					go func(pid string) {
						defer func() {
							<-authSemaphore // 释放信号量
							wg.Done()
						}()
						r.authenticateAndAddMember(ctx, pid)
					}(peerIDStr)
					found++
				case <-ctx.Done():
					wg.Wait()
					return found
				}
			}
		}
	}
}

// parseAddrsFromRecord 从 PeerRecord 解析地址
func (r *realmImpl) parseAddrsFromRecord(record []byte) []string {
	if len(record) == 0 {
		return nil
	}

	var data map[string]interface{}
	if err := json.Unmarshal(record, &data); err != nil {
		return nil
	}

	addrsRaw, ok := data["addrs"]
	if !ok {
		return nil
	}

	addrsSlice, ok := addrsRaw.([]interface{})
	if !ok {
		return nil
	}

	addrs := make([]string, 0, len(addrsSlice))
	for _, addr := range addrsSlice {
		if addrStr, ok := addr.(string); ok {
			addrs = append(addrs, addrStr)
		}
	}

	return addrs
}

// registerToRelay Step B4: 向 Relay 地址簿注册
//
// 将本地节点的地址信息注册到 Relay 地址簿，使 Relay 能够转发请求。
// 这是一个异步操作，注册失败不影响 Realm 的正常运行。
//
// 生命周期重构对齐（Step B4）：
// - 首先探测 Relay 是否支持 addressbook 协议
// - 如果不支持，降级到 DHT 权威发现模式
// - DHT 作为发现的权威来源，addressbook 是可选的优化
//
// 调用时机：Realm 启动后（Protocol 服务启动后）。
// 后续更新：网络变化时通过 OnNetworkChange() 自动触发。
func (r *realmImpl) registerToRelay(ctx context.Context) {
	if r.addressBookService == nil {
		return
	}

	relayPeerID := r.addressBookService.GetRelayPeerID()
	if relayPeerID == "" {
		logger.Debug("跳过 Relay 注册（未配置 Relay）", "realmID", r.id)
		return
	}

	// 首先尝试 addressbook 协议
	if err := r.addressBookService.RegisterSelf(ctx, relayPeerID); err != nil {
		if errors.Is(err, addressbook.ErrProtocolNotSupported) {
			// 协议不支持，降级到 DHT 权威发现
			logger.Warn("Relay 不支持地址簿协议，降级到 DHT 权威发现",
				"realmID", r.id,
				"relay", truncateID(relayPeerID))
			r.SetAddressBookService(nil)
			r.fallbackToDHTAuthoritative(ctx)
			return
		}
		logger.Warn("初始 Relay 注册失败", "realmID", r.id, "relay", relayPeerID, "err", err)
		return
	}

	logger.Info("Relay 地址簿注册成功", "realmID", r.id, "relay", truncateID(relayPeerID))
}

// fallbackToDHTAuthoritative 降级到 DHT 权威发现模式
//
// 当 Relay 不支持 addressbook 协议时调用。
// 确保 DHT 中的 PeerRecord 是最新的，作为发现的唯一权威来源。
//
// 设计对齐（20260123-nat-relay-concept-clarification.md）：
// - DHT 是发现的权威来源
// - addressbook 是优化手段，不是必需的
// - 降级时增加 DHT 发布频率以补偿
func (r *realmImpl) fallbackToDHTAuthoritative(_ context.Context) {
	logger.Info("启用 DHT 权威发现模式（addressbook 降级）", "realmID", r.id)

	// 立即触发一次 DHT 发布
	if r.dht != nil {
		go func() {
			pubCtx, cancel := context.WithTimeout(r.ctx, 30*time.Second)
			defer cancel()
			record := r.buildSignedPeerRecord()
			if record == nil {
				logger.Warn("DHT 权威发布跳过（无可用地址）", "realmID", r.id)
				return
			}
			if err := r.dht.PublishRealmPeerRecord(pubCtx, types.RealmID(r.id), record); err != nil {
				logger.Warn("DHT 权威发布失败", "realmID", r.id, "err", err)
			} else {
				logger.Info("DHT 权威发布成功（addressbook 降级后）", "realmID", r.id)
			}
		}()
	}
}

// ============================================================================
//                              MemberLeave 通知
// ============================================================================

// memberLeaveWaitBeforeClose MemberLeave 广播后等待时间
const memberLeaveWaitBeforeClose = 50 * time.Millisecond

// BroadcastMemberLeave 广播成员离开消息
//
// 当成员优雅离开 Realm 时，通过 PubSub 广播 MemberLeave 消息，
// 通知其他成员立即更新成员列表，实现 < 100ms 的检测延迟。
//
// 协议：/dep2p/app/{realmID}/memberleave/1.0.0
// 安全：消息包含签名和时间戳，防止伪造和重放攻击。
func (r *realmImpl) BroadcastMemberLeave(ctx context.Context, reason memberleavepb.LeaveReason) error {
	if r.memberSyncTopic == nil {
		return fmt.Errorf("member sync topic not initialized")
	}

	if r.manager == nil || r.manager.host == nil {
		return fmt.Errorf("host not available")
	}

	localID := r.manager.host.ID()
	timestamp := time.Now().UnixNano()

	// 构建 MemberLeave 消息
	msg := &memberleavepb.MemberLeave{
		PeerId:    []byte(localID),
		RealmId:   []byte(r.id),
		Reason:    reason,
		Timestamp: timestamp,
	}

	// 签名消息（签名内容：peer_id || realm_id || reason || timestamp）
	sig, err := r.signMemberLeave(localID, r.id, reason, timestamp)
	if err != nil {
		logger.Warn("MemberLeave 签名失败", "err", err)
		// 继续广播，签名验证是可选的安全增强
	} else {
		msg.Signature = sig
	}

	// 序列化消息
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("序列化 MemberLeave 失败: %w", err)
	}

	// 添加消息类型前缀：leave:<proto bytes>
	payload := append([]byte("leave:"), data...)

	// 发布到成员同步 topic
	if err := r.memberSyncTopic.Publish(ctx, payload); err != nil {
		return fmt.Errorf("广播 MemberLeave 失败: %w", err)
	}

	logger.Info("广播 MemberLeave 消息",
		"peerID", truncateID(localID),
		"realmID", r.id,
		"reason", reason.String())

	return nil
}

// signMemberLeave 签名 MemberLeave 消息
//
// 签名内容：peer_id || realm_id || reason || timestamp
func (r *realmImpl) signMemberLeave(peerID, realmID string, reason memberleavepb.LeaveReason, timestamp int64) ([]byte, error) {
	if r.manager == nil || r.manager.host == nil {
		return nil, fmt.Errorf("host not available")
	}

	// 获取私钥
	ps := r.manager.host.Peerstore()
	if ps == nil {
		return nil, fmt.Errorf("peerstore not available")
	}

	privKey, err := ps.PrivKey(types.PeerID(peerID))
	if err != nil {
		return nil, fmt.Errorf("获取私钥失败: %w", err)
	}
	if privKey == nil {
		return nil, fmt.Errorf("私钥为空")
	}

	// 构建签名内容
	signData := r.buildMemberLeaveSignData(peerID, realmID, reason, timestamp)

	// 签名
	return privKey.Sign(signData)
}

// buildMemberLeaveSignData 构建 MemberLeave 签名数据
//
// 格式：peer_id || realm_id || reason (4 bytes, big-endian) || timestamp (8 bytes, big-endian)
func (r *realmImpl) buildMemberLeaveSignData(peerID, realmID string, reason memberleavepb.LeaveReason, timestamp int64) []byte {
	// 计算总长度
	peerIDBytes := []byte(peerID)
	realmIDBytes := []byte(realmID)
	totalLen := len(peerIDBytes) + len(realmIDBytes) + 4 + 8

	data := make([]byte, totalLen)
	offset := 0

	// peer_id
	copy(data[offset:], peerIDBytes)
	offset += len(peerIDBytes)

	// realm_id
	copy(data[offset:], realmIDBytes)
	offset += len(realmIDBytes)

	// reason (4 bytes, big-endian)
	binary.BigEndian.PutUint32(data[offset:], uint32(reason))
	offset += 4

	// timestamp (8 bytes, big-endian)
	binary.BigEndian.PutUint64(data[offset:], uint64(timestamp))

	return data
}

// stop 停止 Realm
func (r *realmImpl) stop(ctx context.Context) error {
	if !r.active.Load() {
		return nil
	}

	// Phase 2.2: 在关闭前广播 MemberLeave 消息
	// 通知其他成员本节点正在优雅离开
	if r.memberSyncTopic != nil {
		broadcastCtx, broadcastCancel := context.WithTimeout(ctx, 100*time.Millisecond)
		if err := r.BroadcastMemberLeave(broadcastCtx, memberleavepb.LeaveReason_LEAVE_REASON_GRACEFUL); err != nil {
			logger.Debug("广播 MemberLeave 失败（非致命）", "err", err)
		}
		broadcastCancel()

		// 等待 50ms，确保消息被传播
		time.Sleep(memberLeaveWaitBeforeClose)
	}

	r.active.Store(false)

	// 取消同步协程
	if r.cancel != nil {
		r.cancel()
	}

	// 1. 取消订阅连接事件和成员同步
	r.unsubscribeConnectionEvents()
	r.unsubscribeMemberSync()

	// P1 修复：停止 CapabilityManager
	if r.capabilityManager != nil {
		if err := r.capabilityManager.Stop(); err != nil {
			logger.Debug("停止 CapabilityManager 失败", "err", err)
		}
	}

	// Step B2 对齐：停止成员同步器
	if r.synchronizer != nil {
		if err := r.synchronizer.Stop(ctx); err != nil {
			logger.Debug("停止成员同步器失败", "realmID", r.id, "err", err)
		} else {
			logger.Debug("成员同步器已停止", "realmID", r.id)
		}
	}

	// 2. 停止 AuthHandler
	if r.authHandler != nil {
		if err := r.authHandler.Stop(ctx); err != nil {
			logger.Debug("停止 AuthHandler 失败", "err", err)
		}
	}

	// 3. 先停止 Protocol 服务（Liveness → Streams → PubSub → Messaging）
	if err := stopProtocolServices(ctx, r); err != nil {
		logger.Warn("停止 Protocol 服务失败", "err", err)
	}

	// 4. 依次停止子模块（逆序：Gateway → Routing → Member → Auth）
	if r.gateway != nil {
		if err := r.gateway.Stop(ctx); err != nil {
			_ = err
		}
	}

	if r.routing != nil {
		if err := r.routing.Stop(ctx); err != nil {
			_ = err
		}
	}

	if r.member != nil {
		if err := r.member.Stop(ctx); err != nil {
			_ = err
		}
	}

	return nil
}

// syncWorker 同步协程
func (r *realmImpl) syncWorker(ctx context.Context) {
	if r.manager == nil || r.manager.config == nil {
		return
	}

	ticker := newTicker(r.manager.config.SyncInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// 定期同步
			r.syncMemberToRouting(ctx)
			r.syncRoutingToGateway(ctx)

		case <-ctx.Done():
			return
		}
	}
}

// Join 加入 Realm（满足 interfaces.Realm 接口）
//
// 采用"先发布后发现"模式，无需入口节点。
// 实际的 Join 逻辑在 joinV2() 中异步执行。
func (r *realmImpl) Join(_ context.Context) error {
	// v2.0: Join 立即返回成功，实际流程在 start() 中异步执行
	// 这符合"先发布后发现"的设计：
	// 1. 发布自己到 DHT
	// 2. 后台循环发现其他成员
	logger.Info("Join 调用（异步模式）", "realmID", r.id)
	return nil
}

// Leave 离开 Realm
func (r *realmImpl) Leave(ctx context.Context) error {
	if r.manager == nil {
		return ErrNotInRealm
	}

	return r.manager.Leave(ctx)
}

// OnNetworkChange 处理网络变化事件
//
// 当网络变化时（如 4G→WiFi），需要：
// 1. 更新成员地址通知
// 2. 触发 Capability 重新广播
// 3. 通知 Relay 地址簿更新
func (r *realmImpl) OnNetworkChange(ctx context.Context, event pkgif.NetworkChangeEvent) error {
	if !r.active.Load() {
		return ErrRealmInactive
	}

	logger.Info("Realm 处理网络变化",
		"realmID", r.id,
		"type", event.Type)

	// 1. 如果有 CapabilityManager，触发重新广播
	if r.capabilityManager != nil {
		if err := r.capabilityManager.ReBroadcast(ctx, event.NewAddrs); err != nil {
			logger.Warn("重新广播能力失败", "err", err)
		} else {
			logger.Debug("能力重新广播完成")
		}
	}

	// 2. 通知成员管理器刷新成员状态
	if r.member != nil {
		// 通知成员刷新本地地址
		// 成员管理器会更新 Peerstore 中本地节点的地址
		logger.Debug("刷新 Realm 成员状态")

		// 触发成员状态刷新（如果 member 支持此操作）
		if refresher, ok := r.member.(interface{ RefreshLocalAddrs([]string) }); ok {
			refresher.RefreshLocalAddrs(event.NewAddrs)
		}
	}

	// 3. 通知 Relay 地址簿更新
	if r.addressBookService != nil {
		relayPeerID := r.addressBookService.GetRelayPeerID()
		if relayPeerID != "" {
			logger.Debug("通知 Relay 地址簿更新", "relay", relayPeerID)
			if err := r.addressBookService.RegisterSelf(ctx, relayPeerID); err != nil {
				logger.Warn("更新 Relay 地址簿失败", "relay", relayPeerID, "err", err)
			} else {
				logger.Debug("Relay 地址簿更新完成")
			}
		}
	}

	logger.Debug("Realm 网络变化处理完成", "realmID", r.id)
	return nil
}

// SetCapabilityManager 设置能力管理器
func (r *realmImpl) SetCapabilityManager(cm CapabilityBroadcaster) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.capabilityManager = cm
}

// SetAddressBookService 设置地址簿服务
//
// 用于网络变化时通知 Relay 地址簿更新。
func (r *realmImpl) SetAddressBookService(abs AddressBookServiceInterface) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.addressBookService = abs
}

// SetLifecycleCoordinator 设置生命周期协调器
//
// 生命周期重构：用于协调 Join 等操作与 A5/B3 gate 的依赖关系。
func (r *realmImpl) SetLifecycleCoordinator(lc *lifecycle.Coordinator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lifecycleCoordinator = lc
}

// SetDHT 设置 DHT 引用
//
// 生命周期重构：用于 DHT 权威解析入口节点。
func (r *realmImpl) SetDHT(dht pkgif.DHT) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dht = dht
}

// ============================================================================
//                              成员自动管理
// ============================================================================

// subscribeConnectionEvents 订阅连接事件
//
// 当新连接建立时，自动触发 PSK 认证流程。
// 认证成功后，自动将对方添加为 Realm 成员。
func (r *realmImpl) subscribeConnectionEvents(ctx context.Context) error {
	if r.eventBus == nil {
		logger.Debug("EventBus 未配置，跳过连接事件订阅")
		return nil
	}

	// 订阅连接建立事件
	connSub, err := r.eventBus.Subscribe(new(types.EvtPeerConnected))
	if err != nil {
		logger.Debug("订阅连接事件失败", "err", err)
		return nil // 非致命错误，继续
	}
	r.connSub = connSub

	// 订阅连接断开事件
	disconnSub, err := r.eventBus.Subscribe(new(types.EvtPeerDisconnected))
	if err != nil {
		logger.Debug("订阅断开事件失败", "err", err)
		// 继续，不影响主流程
	} else {
		r.disconnSub = disconnSub
	}

	// 启动连接事件处理协程
	go r.handleConnectionEvents(ctx)

	logger.Info("已订阅连接事件", "realmID", r.id)
	return nil
}

// handleConnectionEvents 处理连接事件
//
// 即使 disconnSub 为 nil，也应继续处理 connSub 事件
// 原问题：如果 disconnSub 订阅失败，整个事件循环会直接退出，导致连接事件无法触发认证
func (r *realmImpl) handleConnectionEvents(ctx context.Context) {
	// 获取订阅引用（加锁保护）
	r.mu.RLock()
	connSub := r.connSub
	disconnSub := r.disconnSub
	r.mu.RUnlock()

	//只要 connSub 有效就继续处理
	// 原代码：if connSub == nil || disconnSub == nil { return }
	// 这会导致：如果 disconnSub 订阅失败，连接事件也无法处理
	if connSub == nil {
		logger.Warn("connSub 为 nil，无法处理连接事件")
		return
	}

	connCh := connSub.Out()

	// disconnSub 可能为 nil，需要安全处理
	var disconnCh <-chan interface{}
	if disconnSub != nil {
		disconnCh = disconnSub.Out()
	} else {
		logger.Debug("disconnSub 为 nil，跳过断开事件处理（不影响连接事件）")
		// 创建一个永远不会收到数据的 channel
		disconnCh = make(chan interface{})
	}

	logger.Info("连接事件处理循环已启动", "realmID", r.id)

	for {
		select {
		case <-ctx.Done():
			return

		case evt, ok := <-connCh:
			if !ok {
				logger.Warn("connCh 已关闭，退出事件循环")
				return
			}

			e, ok := evt.(*types.EvtPeerConnected)
			if !ok {
				logger.Debug("收到非 EvtPeerConnected 事件，跳过")
				continue
			}

			peerID := string(e.PeerID)
			peerShort := peerID
			if len(peerShort) > 8 {
				peerShort = peerShort[:8]
			}

			// 调试日志：确认收到连接事件
			logger.Info("收到 EvtPeerConnected 事件",
				"peerID", peerShort,
				"direction", e.Direction,
				"realmID", r.id)

			// 跳过自己
			if r.manager != nil && r.manager.host != nil {
				if peerID == string(r.manager.host.ID()) {
					logger.Debug("跳过自己", "peerID", peerShort)
					continue
				}
			}

			// 优化：跳过基础设施节点（Bootstrap、Relay）
			// 这些节点不是 Realm 成员，不支持 Realm 认证协议
			// 避免产生不必要的网络请求和错误日志
			if r.isInfrastructurePeer(peerID) {
				logger.Debug("跳过基础设施节点认证", "peerID", peerShort, "realmID", r.id)
				continue
			}

			// 如果已是成员，只更新最后活跃时间
			if r.member != nil && r.member.IsMember(ctx, peerID) {
				logger.Debug("已是成员，更新 LastSeen", "peerID", peerShort)
				r.member.UpdateLastSeen(ctx, peerID)
				continue
			}

			// 触发 Auth 认证
			logger.Info("触发认证流程", "peerID", peerShort, "realmID", r.id)
			go r.authenticateAndAddMember(ctx, peerID)

		case evt, ok := <-disconnCh:
			if !ok {
				return
			}

			e, ok := evt.(*types.EvtPeerDisconnected)
			if !ok {
				continue
			}

			peerID := string(e.PeerID)

			// 更新成员状态为离线
			if r.member != nil && r.member.IsMember(ctx, peerID) {
				if err := r.member.UpdateStatus(ctx, peerID, false); err != nil {
					logger.Debug("更新成员离线状态失败", "peerID", peerID, "err", err)
				}
			}
		}
	}
}

// authenticateAndAddMember 认证并添加成员
//
// 实现角色协商，NodeID 字节序大的做发起方，
// 避免两个节点同时发起认证导致的竞争条件。
//
// 认证重试机制增强：
//   - 被动方等待 5 秒后超时主动发起认证，打破死锁
//   - 协议协商失败时安排重试，而非静默放弃
//
// 认证去重：同一 peer 的并发连接事件只触发一次认证。
func (r *realmImpl) authenticateAndAddMember(ctx context.Context, peerID string) {
	if r.authHandler == nil {
		logger.Debug("AuthHandler 未配置，跳过认证", "peerID", peerID)
		return
	}

	// v2.0 角色协商：比较 NodeID 字节序，大的发起认证
	// 这样可以避免两方同时发起认证请求
	localID := ""
	if r.manager != nil && r.manager.host != nil {
		localID = r.manager.host.ID()
	}

	// 被动方等待超时后主动发起认证（认证重试机制）
	if localID != "" && localID <= peerID {
		logger.Debug("角色协商：被动方，等待对方发起或超时后主动",
			"localID", truncateID(localID),
			"peerID", truncateID(peerID),
			"reason", "localID <= peerID")

		// 等待 5 秒，看对方是否发起认证
		// 如果 5 秒内成为成员，说明对方已完成认证
		waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()

		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-waitCtx.Done():
				// 超时，检查是否已是成员
				if r.member != nil && r.member.IsMember(ctx, peerID) {
					logger.Debug("被动方等待期间对方已完成认证",
						"peerID", truncateID(peerID))
					return
				}
				// 超时后主动发起认证
				logger.Debug("被动方等待超时，主动发起认证",
					"peerID", truncateID(peerID))
				// 继续执行下面的认证逻辑
				goto doAuth
			case <-ticker.C:
				if r.member != nil && r.member.IsMember(ctx, peerID) {
					logger.Debug("被动方等待期间对方已完成认证",
						"peerID", truncateID(peerID))
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}

	// 本地 ID 字节序大于对端 ID，主动发起认证
	logger.Debug("角色协商：主动发起认证",
		"localID", truncateID(localID),
		"peerID", truncateID(peerID),
		"reason", "localID > peerID")

doAuth:
	// 认证去重检查
	r.authenticatingPeersMu.Lock()
	if _, authenticating := r.authenticatingPeers[peerID]; authenticating {
		r.authenticatingPeersMu.Unlock()
		logger.Debug("节点正在认证中，跳过重复请求", "peerID", truncateID(peerID))
		return
	}
	r.authenticatingPeers[peerID] = struct{}{}
	r.authenticatingPeersMu.Unlock()

	// 认证完成后清理状态
	defer func() {
		r.authenticatingPeersMu.Lock()
		delete(r.authenticatingPeers, peerID)
		r.authenticatingPeersMu.Unlock()
	}()

	logger.Debug("开始认证节点", "peerID", truncateID(peerID), "realmID", r.id)

	// 发起 PSK 认证
	if err := r.authHandler.Authenticate(ctx, peerID); err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "protocols not supported") ||
			strings.Contains(errStr, "protocol negotiation failed") {
			// 对方可能还没加入 Realm，安排重试（认证重试机制）
			// 不再静默放弃，改为安排重试
			if !r.isInfrastructurePeer(peerID) {
				r.scheduleAuthRetry(peerID)
			}
			return
		}
		// 其他错误才记录日志
		logger.Debug("认证失败", "peerID", truncateID(peerID), "err", err)
		return
	}

	// 认证成功，移除待重试列表
	r.removePendingAuth(peerID)

	// 认证成功，添加为成员
	logger.Info("PSK 认证成功", "peerID", truncateID(peerID), "realmID", r.id)
	r.onPeerAuthenticated(ctx, peerID)
}

// isInfrastructurePeer 检查是否为基础设施节点
//
// 基础设施节点包括 Bootstrap 和 Relay 节点，它们：
//   - 不是任何 Realm 的成员
//   - 不支持 Realm 认证协议
//   - 认证尝试总是会失败
//
// 跳过对它们的认证可以避免不必要的网络请求和错误日志。
func (r *realmImpl) isInfrastructurePeer(peerID string) bool {
	if r.infrastructurePeers == nil {
		return false
	}
	_, isInfra := r.infrastructurePeers[peerID]
	return isInfra
}

// SetInfrastructurePeers 设置基础设施节点列表
//
// 由 Manager 在创建 Realm 时调用，传入配置的 Bootstrap 和 Relay PeerID。
func (r *realmImpl) SetInfrastructurePeers(peerIDs []string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.infrastructurePeers = make(map[string]struct{}, len(peerIDs))
	for _, id := range peerIDs {
		if id != "" {
			r.infrastructurePeers[id] = struct{}{}
		}
	}

	if len(r.infrastructurePeers) > 0 {
		logger.Info("已设置基础设施节点列表", "realmID", r.id, "count", len(r.infrastructurePeers))
	}
}

// ============================================================================
//                         认证重试机制
// ============================================================================

// scheduleAuthRetry 安排认证重试
//
// 当认证因"协议不支持"失败时调用，安排指数退避重试（2s, 5s, 10s, 30s）。
// 最多重试 4 次，避免无限重试。
func (r *realmImpl) scheduleAuthRetry(peerID string) {
	r.pendingAuthsMu.Lock()
	defer r.pendingAuthsMu.Unlock()

	if r.pendingAuths == nil {
		r.pendingAuths = make(map[string]*pendingAuth)
	}

	// 检查是否已是成员，无需重试
	if r.member != nil && r.member.IsMember(context.Background(), peerID) {
		return
	}

	// 检查是否为基础设施节点
	if r.isInfrastructurePeer(peerID) {
		return
	}

	delays := []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second, 30 * time.Second}

	if existing, ok := r.pendingAuths[peerID]; ok {
		existing.attempts++
		if existing.attempts > len(delays) {
			delete(r.pendingAuths, peerID)
			logger.Debug("认证重试次数耗尽，放弃",
				"peerID", truncateID(peerID),
				"attempts", existing.attempts)
			return
		}
		existing.nextRetry = time.Now().Add(delays[existing.attempts-1])
		logger.Debug("安排认证重试",
			"peerID", truncateID(peerID),
			"attempt", existing.attempts,
			"delay", delays[existing.attempts-1])
	} else {
		r.pendingAuths[peerID] = &pendingAuth{
			peerID:    peerID,
			attempts:  1,
			nextRetry: time.Now().Add(delays[0]),
		}
		logger.Debug("首次安排认证重试",
			"peerID", truncateID(peerID),
			"delay", delays[0])
	}
}

// removePendingAuth 从待重试列表移除节点
func (r *realmImpl) removePendingAuth(peerID string) {
	r.pendingAuthsMu.Lock()
	defer r.pendingAuthsMu.Unlock()
	delete(r.pendingAuths, peerID)
}

// authRetryLoop 认证重试循环
//
// 每秒检查待重试列表，对到期的节点重新发起认证。
func (r *realmImpl) authRetryLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.processPendingAuths(ctx)
		}
	}
}

// processPendingAuths 处理待重试的认证
func (r *realmImpl) processPendingAuths(ctx context.Context) {
	r.pendingAuthsMu.Lock()
	now := time.Now()
	var toRetry []string

	for peerID, pa := range r.pendingAuths {
		if now.After(pa.nextRetry) {
			toRetry = append(toRetry, peerID)
		}
	}
	r.pendingAuthsMu.Unlock()

	for _, peerID := range toRetry {
		// 检查是否已是成员
		if r.member != nil && r.member.IsMember(ctx, peerID) {
			r.removePendingAuth(peerID)
			continue
		}

		// 检查是否仍然连接
		if r.manager == nil || r.manager.host == nil {
			continue
		}
		conns := r.manager.host.Network().ConnsToPeer(peerID)
		if len(conns) == 0 {
			r.removePendingAuth(peerID)
			logger.Debug("节点已断开，移除重试",
				"peerID", truncateID(peerID))
			continue
		}

		logger.Debug("重试认证",
			"peerID", truncateID(peerID),
			"realmID", r.id)
		go r.authenticateAndAddMember(ctx, peerID)
	}
}

// checkPendingAuthentications 检查待认证的已连接节点
//
// Realm 就绪后扫描所有已连接但未认证的节点，触发认证。
// 解决"连接时机早于 Realm 就绪"的问题。
func (r *realmImpl) checkPendingAuthentications(ctx context.Context) {
	// 延迟 2 秒执行，确保 Realm 完全就绪
	select {
	case <-time.After(2 * time.Second):
	case <-ctx.Done():
		return
	}

	if r.manager == nil || r.manager.host == nil {
		return
	}

	// 获取所有已连接节点
	connectedPeers := r.manager.host.Network().Peers()
	localID := r.manager.host.ID()

	var pendingCount int
	for _, peerID := range connectedPeers {
		peerIDStr := string(peerID)

		// 跳过自己
		if peerIDStr == localID {
			continue
		}

		// 跳过已是成员
		if r.member != nil && r.member.IsMember(ctx, peerIDStr) {
			continue
		}

		// 跳过基础设施节点（Bootstrap/Relay）
		if r.isInfrastructurePeer(peerIDStr) {
			continue
		}

		logger.Debug("检测到已连接但未认证的节点",
			"peerID", truncateID(peerIDStr),
			"realmID", r.id)
		go r.authenticateAndAddMember(ctx, peerIDStr)
		pendingCount++
	}

	if pendingCount > 0 {
		logger.Info("Realm 就绪后触发待认证节点检查",
			"realmID", r.id,
			"pendingCount", pendingCount)
	}
}

// ============================================================================
//                     PSK 认证后成员交换（即时同步优化）
// ============================================================================

// getMemberListForExchange 获取本地成员列表用于交换（即时同步优化）
//
// 返回当前 Realm 的所有成员信息，用于 PSK 认证成功后的即时成员同步。
func (r *realmImpl) getMemberListForExchange() []realmprotocol.MemberExchangeInfo {
	if r.member == nil {
		return nil
	}

	ctx := context.Background()
	members, err := r.member.List(ctx, nil)
	if err != nil {
		logger.Debug("获取成员列表失败", "err", err)
		return nil
	}

	result := make([]realmprotocol.MemberExchangeInfo, 0, len(members))
	for _, m := range members {
		info := realmprotocol.MemberExchangeInfo{
			PeerID:   m.PeerID,
			Addrs:    m.Addrs,
			LastSeen: m.LastSeen.Unix(),
		}
		result = append(result, info)
	}

	return result
}

// mergeMemberListFromExchange 合并远程成员列表（即时同步优化）
//
// 将从对方收到的成员列表合并到本地，实现即时成员同步。
// 跳过自己、已存在的成员、和主动离开的成员。
func (r *realmImpl) mergeMemberListFromExchange(members []realmprotocol.MemberExchangeInfo) {
	if r.member == nil || len(members) == 0 {
		return
	}

	ctx := context.Background()
	selfID := ""
	if r.manager != nil && r.manager.host != nil {
		selfID = r.manager.host.ID()
	}

	addedCount := 0
	for _, m := range members {
		// 跳过自己
		if m.PeerID == selfID {
			continue
		}

		// 跳过已存在的成员
		if r.member.IsMember(ctx, m.PeerID) {
			continue
		}

		// 跳过主动离开的成员
		if mgr, ok := r.member.(*member.Manager); ok {
			if mgr.IsGracefullyLeft(m.PeerID) {
				logger.Debug("跳过主动离开的成员", "peerID", truncateID(m.PeerID))
				continue
			}
		}

		// 添加新成员
		memberInfo := &interfaces.MemberInfo{
			PeerID:   m.PeerID,
			RealmID:  r.id,
			Role:     interfaces.RoleMember,
			Online:   true,
			JoinedAt: time.Now(),
			LastSeen: time.Unix(m.LastSeen, 0),
			Addrs:    m.Addrs,
		}

		if err := r.member.Add(ctx, memberInfo); err != nil {
			logger.Debug("添加交换成员失败", "peerID", truncateID(m.PeerID), "err", err)
		} else {
			addedCount++
		}
	}

	if addedCount > 0 {
		logger.Info("成员交换完成", "added", addedCount, "total", len(members))
	}
}

// onPeerAuthenticated 认证成功回调
//
// 当 PSK 认证成功后调用，将对方添加到成员列表，并广播给其他成员。
//
// 去重机制：
//   - 使用 authenticatingPeersMu 锁保护整个"检查+添加"流程
//   - 如果节点已经是成员，只更新 LastSeen，不重复广播
//   - 防止入站 auth handler 和连接事件处理同时触发导致重复
func (r *realmImpl) onPeerAuthenticated(ctx context.Context, peerID string) {
	if r.member == nil {
		logger.Debug("MemberManager 未配置，跳过成员添加", "peerID", peerID)
		return
	}

	// ★ 清除主动离开标记
	// 认证成功意味着节点真的重新连接了，可以清除之前的离开标记
	if mgr, ok := r.member.(*member.Manager); ok {
		mgr.ClearGracefullyLeft(peerID)
	}

	//使用锁保护"检查+添加"流程，防止并发添加
	r.authenticatingPeersMu.Lock()
	defer r.authenticatingPeersMu.Unlock()

	// 去重检查：如果已经是成员，只更新 LastSeen
	if r.member.IsMember(ctx, peerID) {
		r.member.UpdateLastSeen(ctx, peerID)
		logger.Debug("成员已存在，更新 LastSeen", "peerID", truncateID(peerID))
		return
	}

	memberInfo := &interfaces.MemberInfo{
		PeerID:   peerID,
		RealmID:  r.id,
		Role:     interfaces.RoleMember,
		Online:   true,
		JoinedAt: time.Now(),
		LastSeen: time.Now(),
	}

	// P0 修复：填充已知地址，供 AddrSyncer 入库
	memberInfo.Addrs = r.getPeerAddrs(peerID)

	// 幂等添加：如果已存在则更新 LastSeen
	if err := r.member.Add(ctx, memberInfo); err != nil {
		logger.Debug("添加认证通过的成员", "peerID", peerID, "err", err)
	} else {
		logger.Info("成员加入 Realm", "peerID", peerID, "realmID", r.id)

		// 广播成员加入消息给其他成员（Gossip 传播）
		r.broadcastMemberJoined(ctx, peerID)

		// P0 修复：向新成员发送本节点的能力公告（单播）
		// 这确保新成员立即知道本节点的地址，无需等待周期广播
		if r.capabilityManager != nil {
			go func() {
				// 稍微延迟，确保对方的处理器已就绪
				time.Sleep(200 * time.Millisecond)
				if err := r.capabilityManager.SendToPeer(r.ctx, peerID); err != nil {
					logger.Debug("向新成员发送能力公告失败",
						"peerID", truncateID(peerID),
						"err", err)
				}
			}()
		}

		// 同时广播完整成员列表，确保新加入的节点能获取所有成员信息
		// 延迟广播，等待新成员的 Mesh 形成
		go func() {
			select {
			case <-time.After(1500 * time.Millisecond):
				if r.active.Load() {
					r.broadcastFullMemberList(r.ctx)
				}
			case <-r.ctx.Done():
			}
		}()
	}
}

// broadcastMemberJoined 广播成员加入消息
//
// 通过 PubSub 向 Realm 内所有成员广播新成员信息，
// 使得非直连的节点也能发现新成员（Gossip 传播）。
//
// 注意：GossipSub Mesh 形成需要时间（约 1 秒心跳周期），
// 如果首次发送失败，会在后台重试以确保消息最终送达。
func (r *realmImpl) broadcastMemberJoined(ctx context.Context, peerID string) {
	if r.memberSyncTopic == nil {
		logger.Debug("成员同步 Topic 未初始化，跳过广播",
			"peerID", truncateID(peerID),
			"realmID", r.id)
		return
	}

	// 构建广播消息：优先使用带地址的 V2 格式，兼容旧格式
	joinV2 := memberSyncJoinV2{
		PeerID: peerID,
		Addrs:  r.truncateAddrs(r.getPeerAddrs(peerID), 10),
	}
	msgV2 := r.encodeMemberSyncJoinV2(joinV2)
	msgV1 := []byte("join:" + peerID)

	// 发布到成员同步 topic
	if err := r.memberSyncTopic.Publish(ctx, msgV2); err != nil {
		logger.Debug("广播成员加入消息失败，将在后台重试",
			"peerID", truncateID(peerID),
			"err", err)

		// 启动后台重试：GossipSub Mesh 形成需要时间
		go r.retryBroadcastMemberJoined(peerID, [][]byte{msgV2, msgV1})
		return
	}

	// 兼容旧格式（join:<peerID>）
	if err := r.memberSyncTopic.Publish(ctx, msgV1); err != nil {
		logger.Debug("广播成员加入消息(兼容)失败",
			"peerID", truncateID(peerID),
			"err", err)
	}

	logger.Debug("广播成员加入消息",
		"peerID", truncateID(peerID),
		"topic", memberSyncTopicName(r.id),
		"realmID", r.id)
}

// retryBroadcastMemberJoined 重试广播成员加入消息
//
// 当首次广播失败时（通常是因为 GossipSub Mesh 尚未形成），
// 等待一段时间后重试，最多重试 3 次。
func (r *realmImpl) retryBroadcastMemberJoined(peerID string, msgs [][]byte) {
	// 重试参数
	const maxRetries = 3
	retryDelays := []time.Duration{
		500 * time.Millisecond, // 第 1 次重试：等待 500ms（让 Mesh 形成）
		1 * time.Second,        // 第 2 次重试：等待 1s
		2 * time.Second,        // 第 3 次重试：等待 2s
	}

	for i := 0; i < maxRetries; i++ {
		// 检查 Realm 是否仍然活跃
		if !r.active.Load() {
			logger.Debug("Realm 已停止，取消重试广播",
				"peerID", truncateID(peerID))
			return
		}

		// 等待
		select {
		case <-r.ctx.Done():
			return
		case <-time.After(retryDelays[i]):
		}

		// 检查 Topic 是否可用
		if r.memberSyncTopic == nil {
			logger.Debug("成员同步 Topic 不可用，取消重试",
				"peerID", truncateID(peerID))
			return
		}

		// 重试发送（包含 V2 + 兼容格式）
		if err := r.publishMemberSyncMessages(r.ctx, msgs); err != nil {
			logger.Debug("重试广播成员加入消息失败",
				"peerID", truncateID(peerID),
				"attempt", i+1,
				"maxRetries", maxRetries,
				"err", err)
			continue
		}

		logger.Info("重试广播成员加入消息成功",
			"peerID", truncateID(peerID),
			"attempt", i+1,
			"topic", memberSyncTopicName(r.id))
		return
	}

	logger.Warn("广播成员加入消息最终失败，可能导致成员同步不完整",
		"peerID", truncateID(peerID),
		"maxRetries", maxRetries)
}

// subscribeMemberSync 订阅成员同步消息
//
// 在 Realm 启动时调用，用于接收其他节点广播的成员信息。
func (r *realmImpl) subscribeMemberSync(ctx context.Context) error {
	if r.pubsub == nil {
		logger.Debug("PubSub 未配置，跳过成员同步订阅")
		return nil
	}

	// 加入成员同步 topic（Step B2 对齐：使用 Realm 作用域 Topic）
	topicName := memberSyncTopicName(r.id)
	topic, err := r.pubsub.Join(topicName)
	if err != nil {
		logger.Debug("加入成员同步 topic 失败", "topic", topicName, "err", err)
		return err
	}
	r.memberSyncTopic = topic

	// 订阅该 topic
	sub, err := topic.Subscribe()
	if err != nil {
		topic.Close()
		r.memberSyncTopic = nil
		logger.Debug("订阅成员同步 topic 失败", "topic", topicName, "err", err)
		return err
	}

	// 使用锁保护写入（修复 B25 数据竞争）
	r.mu.Lock()
	r.memberSyncSub = sub
	r.mu.Unlock()

	// 启动消息处理协程
	go r.handleMemberSyncMessages(ctx)

	logger.Debug("已订阅成员同步 topic", "topic", topicName, "realmID", r.id)
	return nil
}

// handleMemberSyncMessages 处理成员同步消息
func (r *realmImpl) handleMemberSyncMessages(ctx context.Context) {
	// 获取订阅的本地引用（修复 B25 数据竞争）
	r.mu.RLock()
	sub := r.memberSyncSub
	r.mu.RUnlock()

	if sub == nil {
		return
	}

	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			return
		}

		r.processMemberSyncMessage(ctx, msg)
	}
}

// processMemberSyncMessage 处理单条成员同步消息
//
// 支持的消息格式：
//   - join:<peerID> - 成员加入通知
//   - sync:<peerID1>,<peerID2>,... - 全量成员列表同步
//   - req:sync - 请求全量成员列表
//   - leave:<proto bytes> - 成员离开通知（快速断开检测 Phase 2）
func (r *realmImpl) processMemberSyncMessage(ctx context.Context, msg *pkgif.Message) {
	if r.member == nil || msg == nil {
		return
	}

	data := msg.Data
	if len(data) < 5 {
		return
	}

	// 获取消息发送者
	from := msg.From

	// 跳过自己发送的消息
	localID := ""
	if r.manager != nil && r.manager.host != nil {
		localID = string(r.manager.host.ID())
		if from == localID {
			return
		}
	}

	// 解析消息类型
	dataStr := string(data)

	switch {
	case len(dataStr) >= len(memberSyncJoinV2Prefix) && strings.HasPrefix(dataStr, memberSyncJoinV2Prefix):
		// 处理带地址的成员加入消息
		r.handleMemberJoinV2(ctx, dataStr[len(memberSyncJoinV2Prefix):], from)

	case len(dataStr) >= len(memberSyncSyncV2Prefix) && strings.HasPrefix(dataStr, memberSyncSyncV2Prefix):
		// 处理带地址的全量成员列表
		r.handleMemberSyncV2(ctx, dataStr[len(memberSyncSyncV2Prefix):], from)

	case len(data) >= 6 && string(data[:6]) == "leave:":
		// 处理成员离开消息（快速断开检测 Phase 2）
		r.handleMemberLeave(ctx, data[6:], from)

	case len(dataStr) >= 5 && dataStr[:5] == "join:":
		// 处理单个成员加入
		r.handleMemberJoin(ctx, dataStr[5:], from)

	case len(dataStr) >= 5 && dataStr[:5] == "sync:":
		// 处理全量成员列表
		r.handleMemberSync(ctx, dataStr[5:], from)

	case dataStr == "req:sync":
		// 响应全量同步请求
		r.handleSyncRequest(ctx, from)
	}
}

// ============================================================================
//                              MemberLeave 接收处理
// ============================================================================

// memberLeaveTimestampValidity MemberLeave 时间戳有效期
const memberLeaveTimestampValidity = 30 * time.Second

// memberLeaveProcessedCleanupInterval 防重放缓存清理间隔
const memberLeaveProcessedCleanupInterval = 60 * time.Second

// handleMemberLeave 处理成员离开消息
//
// 快速断开检测 Phase 2.3：接收并验证 MemberLeave 消息。
// 验证通过后，立即将成员标记为离线/移除，实现 < 100ms 的检测延迟。
//
// 安全验证：
//   - 签名验证：使用发送者公钥验证消息签名
//   - 时间戳检查：消息时间戳必须在 30s 内
//   - 防重放：每个 peer_id + realm_id + timestamp 组合只处理一次
func (r *realmImpl) handleMemberLeave(ctx context.Context, payload []byte, _ string) {
	// 1. 解析 protobuf 消息
	msg := &memberleavepb.MemberLeave{}
	if err := proto.Unmarshal(payload, msg); err != nil {
		logger.Debug("解析 MemberLeave 消息失败", "err", err)
		return
	}

	peerID := string(msg.PeerId)
	realmID := string(msg.RealmId)
	timestamp := msg.Timestamp
	reason := msg.Reason

	// 2. 基本验证
	if peerID == "" || realmID == "" {
		logger.Debug("MemberLeave 消息字段为空")
		return
	}

	// 验证 RealmID 匹配
	if realmID != r.id {
		logger.Debug("MemberLeave RealmID 不匹配",
			"expected", r.id,
			"got", realmID)
		return
	}

	// 3. 时间戳检查（30s 有效期）
	msgTime := time.Unix(0, timestamp)
	now := time.Now()
	if now.Sub(msgTime) > memberLeaveTimestampValidity || msgTime.After(now.Add(time.Second*5)) {
		logger.Debug("MemberLeave 时间戳过期或超前",
			"peerID", truncateID(peerID),
			"msgTime", msgTime,
			"now", now)
		return
	}

	// 4. 防重放检查
	cacheKey := fmt.Sprintf("%s:%s:%d", peerID, realmID, timestamp)
	r.memberLeaveProcessedMu.Lock()
	if r.memberLeaveProcessed == nil {
		r.memberLeaveProcessed = make(map[string]time.Time)
	}
	if _, exists := r.memberLeaveProcessed[cacheKey]; exists {
		r.memberLeaveProcessedMu.Unlock()
		logger.Debug("MemberLeave 消息已处理（防重放）",
			"peerID", truncateID(peerID))
		return
	}
	r.memberLeaveProcessed[cacheKey] = now
	r.memberLeaveProcessedMu.Unlock()

	// 5. 签名验证（可选，增强安全性）
	if len(msg.Signature) > 0 {
		valid, err := r.verifyMemberLeaveSignature(peerID, realmID, reason, timestamp, msg.Signature)
		if err != nil {
			logger.Debug("MemberLeave 签名验证失败",
				"peerID", truncateID(peerID),
				"err", err)
			// 签名验证失败，但为了向后兼容，继续处理
			// 在生产环境可能需要更严格的处理
		} else if !valid {
			logger.Warn("MemberLeave 签名无效",
				"peerID", truncateID(peerID))
			return
		}
	}

	// 6. 检查是否为已知成员
	if r.member == nil {
		return
	}

	if !r.member.IsMember(ctx, peerID) {
		logger.Debug("MemberLeave 发送者不是成员",
			"peerID", truncateID(peerID))
		return
	}

	// 7. 处理成员离开
	logger.Info("收到 MemberLeave 消息",
		"peerID", truncateID(peerID),
		"reason", reason.String(),
		"realmID", r.id)

	// 根据离开原因决定处理方式
	switch reason {
	case memberleavepb.LeaveReason_LEAVE_REASON_GRACEFUL:
		// 优雅离开：立即移除成员
		// ★ 标记为主动离开，防止成员同步消息重新添加
		if mgr, ok := r.member.(*member.Manager); ok {
			mgr.MarkGracefullyLeft(peerID)
		}
		if err := r.member.Remove(ctx, peerID); err != nil {
			logger.Debug("移除优雅离开的成员失败",
				"peerID", truncateID(peerID),
				"err", err)
		} else {
			logger.Info("成员优雅离开",
				"peerID", truncateID(peerID),
				"realmID", r.id)
		}

	case memberleavepb.LeaveReason_LEAVE_REASON_KICKED:
		// 被踢出：立即移除成员
		// ★ 标记为主动离开，防止成员同步消息重新添加
		if mgr, ok := r.member.(*member.Manager); ok {
			mgr.MarkGracefullyLeft(peerID)
		}
		if err := r.member.Remove(ctx, peerID); err != nil {
			logger.Debug("移除被踢出的成员失败",
				"peerID", truncateID(peerID),
				"err", err)
		} else {
			logger.Info("成员被踢出",
				"peerID", truncateID(peerID),
				"realmID", r.id)
		}

	case memberleavepb.LeaveReason_LEAVE_REASON_WITNESS:
		// 见证人报告：先标记离线，等待确认后再移除
		if err := r.member.UpdateStatus(ctx, peerID, false); err != nil {
			logger.Debug("更新见证报告成员状态失败",
				"peerID", truncateID(peerID),
				"err", err)
		} else {
			logger.Info("成员被见证人报告离线",
				"peerID", truncateID(peerID),
				"realmID", r.id)
		}

	default:
		// 未知原因：标记离线
		if err := r.member.UpdateStatus(ctx, peerID, false); err != nil {
			logger.Debug("更新成员状态失败",
				"peerID", truncateID(peerID),
				"err", err)
		}
	}

	// 8. 清理过期的防重放缓存（异步）
	go r.cleanupMemberLeaveCache()
}

// verifyMemberLeaveSignature 验证 MemberLeave 签名
func (r *realmImpl) verifyMemberLeaveSignature(peerID, realmID string, reason memberleavepb.LeaveReason, timestamp int64, signature []byte) (bool, error) {
	if r.manager == nil || r.manager.host == nil {
		return false, fmt.Errorf("host not available")
	}

	// 获取发送者公钥
	ps := r.manager.host.Peerstore()
	if ps == nil {
		return false, fmt.Errorf("peerstore not available")
	}

	pubKey, err := ps.PubKey(types.PeerID(peerID))
	if err != nil {
		return false, fmt.Errorf("获取公钥失败: %w", err)
	}
	if pubKey == nil {
		return false, fmt.Errorf("公钥不可用")
	}

	// 构建签名数据
	signData := r.buildMemberLeaveSignData(peerID, realmID, reason, timestamp)

	// 验证签名
	return pubKey.Verify(signData, signature)
}

// cleanupMemberLeaveCache 清理过期的防重放缓存
func (r *realmImpl) cleanupMemberLeaveCache() {
	r.memberLeaveProcessedMu.Lock()
	defer r.memberLeaveProcessedMu.Unlock()

	if r.memberLeaveProcessed == nil {
		return
	}

	now := time.Now()
	expiry := memberLeaveTimestampValidity * 2 // 保留 2 倍有效期

	for key, processedAt := range r.memberLeaveProcessed {
		if now.Sub(processedAt) > expiry {
			delete(r.memberLeaveProcessed, key)
		}
	}
}

// handleMemberJoin 处理成员加入消息
func (r *realmImpl) handleMemberJoin(ctx context.Context, peerID string, from string) {
	r.handleMemberJoinWithAddrs(ctx, peerID, nil, from)
}

func (r *realmImpl) handleMemberJoinV2(ctx context.Context, payload string, from string) {
	var join memberSyncJoinV2
	if err := json.Unmarshal([]byte(payload), &join); err != nil {
		logger.Debug("解析 join2 消息失败", "err", err)
		return
	}
	r.handleMemberJoinWithAddrs(ctx, join.PeerID, join.Addrs, from)
}

func (r *realmImpl) handleMemberJoinWithAddrs(ctx context.Context, peerID string, addrs []string, from string) {
	if peerID == "" {
		return
	}

	// 检查是否已经是成员
	if r.member.IsMember(ctx, peerID) {
		return
	}

	if len(addrs) == 0 {
		addrs = r.getPeerAddrs(peerID)
	}

	// 添加为成员（通过 Gossip 发现的成员）
	memberInfo := &interfaces.MemberInfo{
		PeerID:   peerID,
		RealmID:  r.id,
		Role:     interfaces.RoleMember,
		Online:   true,
		JoinedAt: time.Now(),
		LastSeen: time.Now(),
		Addrs:    addrs,
	}

	if err := r.member.Add(ctx, memberInfo); err != nil {
		logger.Debug("添加 Gossip 发现的成员失败", "peerID", peerID, "err", err)
	} else {
		logger.Debug("通过 Gossip 发现新成员",
			"peerID", truncateID(peerID),
			"from", truncateID(from),
			"addrs", len(addrs),
			"realmID", r.id)

		// 继续广播给其他成员（Gossip 传播）
		r.broadcastMemberJoined(ctx, peerID)

		// 尝试自动连接 Gossip 发现的成员
		// 使用现有的 Connector 组件，避免重复造轮子
		if len(addrs) > 0 {
			go r.tryConnectToGossipMember(peerID, addrs)
		}
	}
}

// handleMemberSync 处理全量成员列表同步
func (r *realmImpl) handleMemberSync(ctx context.Context, membersStr string, from string) {
	if membersStr == "" {
		return
	}

	// 解析成员列表（逗号分隔）
	members := strings.Split(membersStr, ",")
	entries := make([]memberSyncJoinV2, 0, len(members))
	for _, peerID := range members {
		peerID = strings.TrimSpace(peerID)
		if peerID == "" {
			continue
		}
		entries = append(entries, memberSyncJoinV2{PeerID: peerID})
	}

	r.handleMemberSyncEntries(ctx, entries, from)
}

func (r *realmImpl) handleMemberSyncV2(ctx context.Context, payload string, from string) {
	var list memberSyncListV2
	if err := json.Unmarshal([]byte(payload), &list); err != nil {
		logger.Debug("解析 sync2 消息失败", "err", err)
		return
	}
	r.handleMemberSyncEntries(ctx, list.Members, from)
}

func (r *realmImpl) handleMemberSyncEntries(ctx context.Context, entries []memberSyncJoinV2, from string) {
	if len(entries) == 0 {
		return
	}

	addedCount := 0
	// 收集需要连接的成员
	var membersToConnect []memberSyncJoinV2

	for _, entry := range entries {
		peerID := strings.TrimSpace(entry.PeerID)
		if peerID == "" {
			continue
		}

		// 检查是否已经是成员
		if r.member.IsMember(ctx, peerID) {
			continue
		}

		addrs := entry.Addrs
		if len(addrs) == 0 {
			addrs = r.getPeerAddrs(peerID)
		}

		// 添加为成员
		memberInfo := &interfaces.MemberInfo{
			PeerID:   peerID,
			RealmID:  r.id,
			Role:     interfaces.RoleMember,
			Online:   true,
			JoinedAt: time.Now(),
			LastSeen: time.Now(),
			Addrs:    addrs,
		}

		if err := r.member.Add(ctx, memberInfo); err != nil {
			logger.Debug("添加同步成员失败", "peerID", truncateID(peerID), "err", err)
		} else {
			addedCount++
			// 记录需要连接的成员
			if len(addrs) > 0 {
				membersToConnect = append(membersToConnect, memberSyncJoinV2{
					PeerID: peerID,
					Addrs:  addrs,
				})
			}
		}
	}

	if addedCount > 0 {
		logger.Info("通过全量同步添加成员",
			"from", truncateID(from),
			"added", addedCount,
			"total", len(entries),
			"realmID", r.id)

		// 异步尝试连接新发现的成员
		if len(membersToConnect) > 0 {
			go r.tryConnectToGossipMembers(membersToConnect)
		}
	}
}

// tryConnectToGossipMember 尝试连接 Gossip 发现的单个成员
//
// Gossip 发现成员后应自动建立连接
// 复用现有的 Connector 组件，避免重复造轮子
func (r *realmImpl) tryConnectToGossipMember(peerID string, addrs []string) {
	if !r.active.Load() {
		return
	}

	// 检查是否已有连接
	if r.manager != nil && r.manager.host != nil {
		network := r.manager.host.Network()
		if network != nil && network.Connectedness(peerID) == pkgif.Connected {
			logger.Debug("跳过已连接的成员",
				"peerID", truncateID(peerID))
			return
		}
	}

	// 使用 Connector 连接（支持直连、打洞、Relay 自动降级）
	r.mu.RLock()
	conn := r.connector
	r.mu.RUnlock()

	if conn == nil {
		logger.Debug("Connector 未初始化，跳过自动连接",
			"peerID", truncateID(peerID))
		return
	}

	ctx, cancel := context.WithTimeout(r.ctx, 15*time.Second)
	defer cancel()

	logger.Debug("尝试自动连接 Gossip 发现的成员",
		"peerID", truncateID(peerID),
		"addrs", len(addrs))

	result, err := conn.ConnectWithHint(ctx, peerID, addrs)
	if err != nil {
		logger.Debug("自动连接失败",
			"peerID", truncateID(peerID),
			"err", err)
		return
	}

	logger.Info("自动连接 Gossip 成员成功",
		"peerID", truncateID(peerID),
		"method", result.Method,
		"duration", result.Duration)
}

// tryConnectToGossipMembers 批量尝试连接 Gossip 发现的成员
//
// 全量同步后批量连接新发现的成员
// 使用有限并发避免资源耗尽
func (r *realmImpl) tryConnectToGossipMembers(members []memberSyncJoinV2) {
	if !r.active.Load() || len(members) == 0 {
		return
	}

	// 限制并发连接数
	const maxConcurrent = 3
	sem := make(chan struct{}, maxConcurrent)

	var wg sync.WaitGroup
	for _, m := range members {
		if !r.active.Load() {
			break
		}

		wg.Add(1)
		go func(peerID string, addrs []string) {
			defer wg.Done()

			// 获取信号量
			sem <- struct{}{}
			defer func() { <-sem }()

			r.tryConnectToGossipMember(peerID, addrs)
		}(m.PeerID, m.Addrs)
	}

	wg.Wait()
}

// handleSyncRequest 处理全量同步请求
func (r *realmImpl) handleSyncRequest(_ context.Context, _ string) {
	// 延迟响应，避免多个节点同时响应
	go func() {
		// 随机延迟 100-500ms
		time.Sleep(time.Duration(100+time.Now().UnixNano()%400) * time.Millisecond)

		if !r.active.Load() {
			return
		}

		r.broadcastFullMemberList(context.Background())
	}()
}

// broadcastFullMemberList 广播完整成员列表
func (r *realmImpl) broadcastFullMemberList(ctx context.Context) {
	if r.memberSyncTopic == nil || r.member == nil {
		return
	}

	// 获取所有成员
	members, err := r.member.List(ctx, nil)
	if err != nil || len(members) == 0 {
		return
	}

	// 构建成员列表（带地址）与旧格式列表
	entries := make([]memberSyncJoinV2, 0, len(members))
	memberIDs := make([]string, 0, len(members))
	for _, m := range members {
		if m == nil || m.PeerID == "" {
			continue
		}
		memberIDs = append(memberIDs, m.PeerID)

		addrs := m.Addrs
		if len(addrs) == 0 {
			addrs = r.getPeerAddrs(m.PeerID)
		}
		entries = append(entries, memberSyncJoinV2{
			PeerID: m.PeerID,
			Addrs:  r.truncateAddrs(addrs, 10),
		})
	}

	// 构建同步消息：sync2:<json>
	msgV2 := r.encodeMemberSyncListV2(entries)
	msgV1 := []byte("sync:" + strings.Join(memberIDs, ","))

	// 发布（优先 V2，兼容旧格式）
	if err := r.publishMemberSyncMessages(ctx, [][]byte{msgV2, msgV1}); err != nil {
		logger.Debug("广播成员列表失败", "err", err, "memberCount", len(memberIDs))
		return
	}

	logger.Debug("广播完整成员列表",
		"memberCount", len(memberIDs),
		"topic", memberSyncTopicName(r.id),
		"realmID", r.id)
}

// requestFullMemberSync 请求全量成员同步
//
// 当新节点加入 Realm 后，向网络请求完整成员列表。
// 收到请求的节点会响应自己的成员列表。
//
// 即时同步优化：不再等待有成员后才发送请求，直接发送并重试。
// 原设计会导致双方互相等待的死锁：
//   - A 等待 B 成为成员后才发送同步请求
//   - B 等待 A 成为成员后才发送同步请求
//   - 结果：双方都等待 120 秒超时
//
// 新设计：立即发送同步请求，Mesh 未就绪时启动后台重试。
func (r *realmImpl) requestFullMemberSync(ctx context.Context) {
	if r.memberSyncTopic == nil {
		return
	}

	// 直接发送同步请求，不检查成员数
	msg := []byte("req:sync")
	if err := r.memberSyncTopic.Publish(ctx, msg); err != nil {
		// Mesh 未就绪，启动后台重试
		logger.Debug("发送成员同步请求失败，启动后台重试", "err", err, "realmID", r.id)
		go r.retryMemberSyncRequest(ctx)
		return
	}

	logger.Debug("发送成员同步请求", "topic", memberSyncTopicName(r.id), "realmID", r.id)
}

// retryMemberSyncRequest 后台重试成员同步请求（即时同步优化）
//
// 当首次发送失败时（通常是 Mesh 未就绪），使用递增延迟重试。
// 重试策略：1s, 2s, 4s, 8s, 15s（共 5 次，总计 30 秒）
//
// 使用 syncRetrying 原子标志防止多个重试 goroutine 并发执行。
func (r *realmImpl) retryMemberSyncRequest(ctx context.Context) {
	// 防止多个重试 goroutine 并发
	if !r.syncRetrying.CompareAndSwap(false, true) {
		logger.Debug("成员同步重试已在进行中，跳过", "realmID", r.id)
		return
	}
	defer r.syncRetrying.Store(false)

	retryDelays := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		15 * time.Second,
	}

	for i, delay := range retryDelays {
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}

		// 检查 Topic 是否仍可用
		if r.memberSyncTopic == nil || !r.active.Load() {
			logger.Debug("成员同步 Topic 不可用，停止重试", "realmID", r.id)
			return
		}

		msg := []byte("req:sync")
		if err := r.memberSyncTopic.Publish(ctx, msg); err != nil {
			logger.Debug("重试成员同步请求失败",
				"attempt", i+1,
				"maxAttempts", len(retryDelays),
				"err", err,
				"realmID", r.id)
			continue
		}

		logger.Debug("重试成员同步请求成功",
			"attempt", i+1,
			"topic", memberSyncTopicName(r.id),
			"realmID", r.id)
		return
	}

	logger.Debug("成员同步请求最终失败，将依赖其他同步机制",
		"maxAttempts", len(retryDelays),
		"realmID", r.id)
}

// waitForMembersAndSync 等待有 Realm 成员后再发送同步请求
//
// Deprecated: 即时同步优化后不再使用此函数。
// 原设计会导致双方互相等待的死锁，已改为使用 retryMemberSyncRequest 直接重试。
// 保留此函数仅用于兼容性，未来版本将移除。
func (r *realmImpl) waitForMembersAndSync(_ context.Context) {
	// 此函数已弃用，不再执行任何操作
	// 成员同步请求现在由 retryMemberSyncRequest 处理
	logger.Debug("waitForMembersAndSync 已弃用，使用 retryMemberSyncRequest 替代", "realmID", r.id)
}

// encodeMemberSyncJoinV2 构建 join2 消息
func (r *realmImpl) encodeMemberSyncJoinV2(join memberSyncJoinV2) []byte {
	payload, err := json.Marshal(join)
	if err != nil {
		logger.Debug("编码 join2 消息失败", "err", err)
		return nil
	}
	return []byte(memberSyncJoinV2Prefix + string(payload))
}

// encodeMemberSyncListV2 构建 sync2 消息
func (r *realmImpl) encodeMemberSyncListV2(entries []memberSyncJoinV2) []byte {
	payload, err := json.Marshal(memberSyncListV2{Members: entries})
	if err != nil {
		logger.Debug("编码 sync2 消息失败", "err", err)
		return nil
	}
	return []byte(memberSyncSyncV2Prefix + string(payload))
}

// publishMemberSyncMessages 发布成员同步消息（V2 + 兼容格式）
func (r *realmImpl) publishMemberSyncMessages(ctx context.Context, msgs [][]byte) error {
	if r.memberSyncTopic == nil {
		return fmt.Errorf("member sync topic not initialized")
	}

	var lastErr error
	published := false
	for _, msg := range msgs {
		if len(msg) == 0 {
			continue
		}
		if err := r.memberSyncTopic.Publish(ctx, msg); err != nil {
			lastErr = err
			continue
		}
		published = true
	}

	if published {
		return nil
	}
	return lastErr
}

// getPeerAddrs 获取已知地址（优先使用 Peerstore）
func (r *realmImpl) getPeerAddrs(peerID string) []string {
	if peerID == "" || r.manager == nil || r.manager.host == nil {
		return nil
	}

	host := r.manager.host
	if peerID == host.ID() {
		addrs := host.ShareableAddrs()
		if len(addrs) == 0 {
			addrs = host.Addrs()
		}
		return r.truncateAddrs(addrs, 10)
	}

	ps := host.Peerstore()
	if ps == nil {
		return nil
	}

	maddrs := ps.Addrs(types.PeerID(peerID))
	if len(maddrs) == 0 {
		return nil
	}

	out := make([]string, 0, len(maddrs))
	for _, addr := range maddrs {
		if addr == nil {
			continue
		}
		out = append(out, addr.String())
	}
	return r.truncateAddrs(out, 10)
}

// truncateAddrs 限制地址数量，避免消息过大
func (r *realmImpl) truncateAddrs(addrs []string, max int) []string {
	if max <= 0 || len(addrs) <= max {
		return addrs
	}
	return addrs[:max]
}

// truncateID 安全截断 ID 用于日志
func truncateID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// unsubscribeConnectionEvents 取消订阅连接事件
func (r *realmImpl) unsubscribeConnectionEvents() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.connSub != nil {
		r.connSub.Close()
		r.connSub = nil
	}
	if r.disconnSub != nil {
		r.disconnSub.Close()
		r.disconnSub = nil
	}
}

// unsubscribeMemberSync 取消订阅成员同步
func (r *realmImpl) unsubscribeMemberSync() {
	// 使用锁保护读写（修复 B25 数据竞争）
	r.mu.Lock()
	sub := r.memberSyncSub
	r.memberSyncSub = nil
	topic := r.memberSyncTopic
	r.memberSyncTopic = nil
	r.mu.Unlock()

	// 在锁外执行关闭操作（避免持锁时阻塞）
	if sub != nil {
		sub.Cancel()
	}
	if topic != nil {
		topic.Close()
	}
}

// EventBus 返回事件总线
//
// 用于订阅 Realm 成员事件：
//   - types.EvtRealmMemberJoined: 成员加入
//   - types.EvtRealmMemberLeft: 成员离开
func (r *realmImpl) EventBus() pkgif.EventBus {
	return r.eventBus
}

// Close 关闭 Realm
func (r *realmImpl) Close() error {
	if r.active.Load() {
		ctx := context.Background()
		r.stop(ctx)
	}

	// 关闭所有子模块
	if r.gateway != nil {
		r.gateway.Close()
	}

	if r.routing != nil {
		r.routing.Close()
	}

	if r.member != nil {
		r.member.Close()
	}

	if r.auth != nil {
		r.auth.Close()
	}

	// 关闭 AuthHandler
	if r.authHandler != nil {
		r.authHandler.Close()
	}

	return nil
}

// 确保实现接口
var _ interfaces.Realm = (*realmImpl)(nil)
var _ pkgif.Realm = (*realmImpl)(nil)
