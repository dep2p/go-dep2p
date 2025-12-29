// Package realm 提供 Realm 管理实现
//
// v1.1 变更:
//   - 采用严格单 Realm 模型（一个节点同时只能加入一个业务 Realm）
//   - JoinRealm: 已加入时返回 ErrAlreadyJoined
//   - LeaveRealm: 改为无参数
//   - IsMember: 改为无参数便捷方法
//   - 新增 IsMemberOf: 检查特定 Realm
package realm

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/config"
	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	livenessif "github.com/dep2p/go-dep2p/pkg/interfaces/liveness"
	messagingif "github.com/dep2p/go-dep2p/pkg/interfaces/messaging"
	realmif "github.com/dep2p/go-dep2p/pkg/interfaces/realm"
	relayif "github.com/dep2p/go-dep2p/pkg/interfaces/relay"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              realmState Realm 内部状态
// ============================================================================

// realmState Realm 内部状态
type realmState struct {
	metadata     *types.RealmMetadata
	realmKey     types.RealmKey // IMPL-1227: Realm 密钥（用于 PSK 验证）
	peers        map[types.NodeID]*peerInfo
	lastAnnounce time.Time
}

// peerInfo 节点信息
type peerInfo struct {
	nodeID   types.NodeID
	addrs    []endpoint.Address
	lastSeen time.Time
}

// ============================================================================
//                              Manager 实现
// ============================================================================

// Manager RealmManager 实现
type Manager struct {
	config   config.RealmConfig
	endpoint endpoint.Endpoint

	// v1.1: RealmAuth（可选）
	auth *Authenticator

	// 出站 RealmAuth 去抖（避免对同一连接重复并发认证）
	authMu       sync.Mutex
	authInFlight map[types.NodeID]struct{}

	// 发现服务（用于 DHT 注册）
	discovery discoveryif.DiscoveryService

	// 存活检测服务（用于发送 Goodbye）
	liveness livenessif.LivenessService

	// IMPL-1227 Phase 4: 消息服务（用于 Realm 服务适配）
	messaging messagingif.MessagingService

	// IMPL-1227 Phase 4: 中继服务（用于 Realm Relay 适配）
	relayServer relayif.RelayServer
	relayClient relayif.RelayClient

	// Realm 状态
	realms       map[string]*realmState
	primaryRealm string

	// IMPL-1227: 当前 Realm 对象（Layer 2 产物）
	currentRealmObj *realmImpl

	mu sync.RWMutex

	// 运行状态
	running int32
	closed  int32
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewManager 创建 Realm 管理器
func NewManager(cfg config.RealmConfig, endpoint endpoint.Endpoint) *Manager {
	return &Manager{
		config:   cfg,
		endpoint: endpoint,
		realms:   make(map[string]*realmState),
		authInFlight: make(map[types.NodeID]struct{}),
	}
}

// SetAuthenticator 设置 RealmAuth 认证器（可选）。
func (m *Manager) SetAuthenticator(auth *Authenticator) {
	m.auth = auth
}

// SetDiscovery 设置发现服务
func (m *Manager) SetDiscovery(discovery discoveryif.DiscoveryService) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.discovery = discovery
}

// SetLiveness 设置存活检测服务
func (m *Manager) SetLiveness(liveness livenessif.LivenessService) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.liveness = liveness
}

// SetMessaging 设置消息服务（IMPL-1227 Phase 4）
//
// 用于 Realm 服务适配层调用底层消息服务。
func (m *Manager) SetMessaging(messaging messagingif.MessagingService) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messaging = messaging
}

// SetRelayServices 设置中继服务（用于 Realm Relay 适配）
//
// IMPL-1227: 注入 RelayServer 和 RelayClient，以便 realmRelay 适配器可以配置它们。
func (m *Manager) SetRelayServices(server relayif.RelayServer, client relayif.RelayClient) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.relayServer = server
	m.relayClient = client
}

// RelayServer 返回 Relay Server（用于 realmRelay 适配器）
func (m *Manager) RelayServer() relayif.RelayServer {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.relayServer
}

// RelayClient 返回 Relay Client（用于 realmRelay 适配器）
func (m *Manager) RelayClient() relayif.RelayClient {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.relayClient
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动 Realm 管理服务
func (m *Manager) Start(_ context.Context) error {
	if !atomic.CompareAndSwapInt32(&m.running, 0, 1) {
		return nil
	}

	// 使用 context.Background() 而非 ctx，因为 Fx OnStart 的 ctx 在 OnStart 返回后会被取消。
	// 若使用该 ctx，会导致 RealmAuth/announceLoop 等后台协程提前退出。
	m.ctx, m.cancel = context.WithCancel(context.Background())

	log.Info("Realm 管理器启动中")

	// 启动出站 RealmAuth 循环（可选）
	if m.config.RealmAuthEnabled && m.auth != nil && m.endpoint != nil {
		go m.realmAuthLoop()
	}

	// IMPL-1227: 不再自动加入默认 Realm
	// 用户必须显式调用 JoinRealm 或 JoinRealmWithKey

	// 启动定期通告
	go m.announceLoop()

	log.Info("Realm 管理器已启动")
	return nil
}

// RegisterRealmAuthHandler 注册 RealmAuth 协议处理器（入站）。
//
// RealmAuth 是系统协议（/dep2p/sys/...），ProtocolRouter 不会要求 RealmContext。
func (m *Manager) RegisterRealmAuthHandler() {
	if m == nil || m.endpoint == nil || m.auth == nil {
		return
	}
	m.endpoint.SetProtocolHandler(RealmAuthProtocol, m.auth.HandleInbound)
}

// realmAuthLoop 周期性对已建立连接执行出站 RealmAuth（确保连接上有 RealmContext）。
func (m *Manager) realmAuthLoop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			// 仅在已加入 Realm 时进行出站认证
			if !m.IsMember() {
				continue
			}
			if m.endpoint == nil || m.auth == nil {
				continue
			}
			for _, conn := range m.endpoint.Connections() {
				if conn == nil || conn.IsClosed() {
					continue
				}
				rc := conn.RealmContext()
				if rc != nil && rc.IsValid() {
					continue
				}
				m.tryAuthenticateConn(conn)
			}
		}
	}
}

func (m *Manager) tryAuthenticateConn(conn endpoint.Connection) {
	remote := conn.RemoteID()

	m.authMu.Lock()
	if _, ok := m.authInFlight[remote]; ok {
		m.authMu.Unlock()
		return
	}
	m.authInFlight[remote] = struct{}{}
	m.authMu.Unlock()

	go func() {
		defer func() {
			m.authMu.Lock()
			delete(m.authInFlight, remote)
			m.authMu.Unlock()
		}()

		// 出站认证需要超时控制
		timeout := m.config.RealmAuthTimeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		ctx, cancel := context.WithTimeout(m.ctx, timeout)
		defer cancel()

		connCtx, err := m.auth.Authenticate(ctx, conn)
		if err != nil {
			log.Debug("出站 RealmAuth 失败",
				"remote", remote.ShortString(),
				"err", err)
			return
		}

		// 设置连接级 RealmContext（双保险：Authenticator 内部也会设置）
		conn.SetRealmContext(&endpoint.RealmContext{
			RealmID:   string(connCtx.RealmID),
			Verified:  true,
			ExpiresAt: connCtx.ExpiresAt,
		})
	}()
}

// Stop 停止 Realm 管理服务
func (m *Manager) Stop() error {
	if !atomic.CompareAndSwapInt32(&m.closed, 0, 1) {
		return nil
	}

	log.Info("Realm 管理器停止中")

	// 向所有 Realm 内的节点发送 Goodbye
	m.mu.RLock()
	for realmID := range m.realms {
		go m.sendRealmGoodbye(types.RealmID(realmID))
	}
	m.mu.RUnlock()

	if m.cancel != nil {
		m.cancel()
	}

	atomic.StoreInt32(&m.running, 0)
	log.Info("Realm 管理器已停止")
	return nil
}

// ============================================================================
//                              加入/离开 Realm
// ============================================================================

// CurrentRealm 返回当前 Realm 对象（IMPL-1227 新 API）
//
// 如果未加入任何 Realm，返回 nil。
func (m *Manager) CurrentRealm() realmif.Realm {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.currentRealmObj == nil {
		return nil
	}
	return m.currentRealmObj
}

// JoinRealm 加入指定 Realm，返回 Realm 对象（IMPL-1227 新 API）
//
// 必须通过 WithRealmKey 提供 realmKey，用于 PSK 成员认证。
// RealmID 由 realmKey 自动派生。
//
// 示例:
//
//	realm, err := manager.JoinRealm(ctx, "my-business", realm.WithRealmKey(key))
//	if err != nil { ... }
//	messaging := realm.Messaging()
func (m *Manager) JoinRealm(ctx context.Context, name string, opts ...realmif.RealmOption) (realmif.Realm, error) {
	// 应用选项
	realmOpts := realmif.ApplyRealmOptions(opts...)

	// 必须提供 realmKey
	if realmOpts.RealmKey.IsEmpty() {
		return nil, ErrRealmKeyRequired
	}

	// 从 realmKey 派生 RealmID
	realmID := types.DeriveRealmID(realmOpts.RealmKey)

	m.mu.Lock()
	defer m.mu.Unlock()

	realmStr := string(realmID)

	// v1.1: 严格单 Realm 模型 - 已加入任何 Realm 则拒绝
	if m.primaryRealm != "" {
		if m.primaryRealm == realmStr {
			log.Debug("已是该 Realm 成员",
				"realm", realmStr)
		} else {
			log.Debug("已加入其他 Realm，需先离开",
				"current", m.primaryRealm,
				"target", realmStr)
		}
		return nil, ErrAlreadyJoined
	}

	// REQ-BOOT-005: 检测是否为 Private Realm 自举
	isPrivateBootstrap := len(realmOpts.PrivateBootstrapPeers) > 0 || realmOpts.SkipDHTRegistration

	log.Info("加入 Realm",
		"name", name,
		"realm", realmStr,
		"private_bootstrap", isPrivateBootstrap,
		"bootstrap_peers", len(realmOpts.PrivateBootstrapPeers))

	// 创建 Realm 状态
	state := &realmState{
		metadata: &types.RealmMetadata{
			ID: realmID,
		},
		realmKey:     realmOpts.RealmKey,
		peers:        make(map[types.NodeID]*peerInfo),
		lastAnnounce: time.Time{},
	}
	m.realms[realmStr] = state

	// v1.1: 严格单 Realm，直接设为主 Realm
	m.primaryRealm = realmStr

	// IMPL-1227: 创建 Realm 对象
	realm := newRealmImpl(m, name, realmOpts.RealmKey, state)
	m.currentRealmObj = realm

	// REQ-BOOT-005: Private Realm 自举
	if isPrivateBootstrap {
		// 连接私有引导节点（不使用公共 DHT）
		go m.connectPrivateBootstrapPeers(ctx, realmID, realmOpts.PrivateBootstrapPeers)

		// 跳过 DHT 注册（Private Realm 不在公共 DHT 中注册）
		if !realmOpts.SkipDHTRegistration {
			// 如果未显式跳过但有私有引导节点，则仍然注册（可配置）
			go m.registerWithDHT(ctx, realmID)
		}
	} else {
		// 向公共 DHT 注册
		go m.registerWithDHT(ctx, realmID)
	}

	// 通告到 Realm
	go m.announceToRealm(ctx, realmID)

	return realm, nil
}

// JoinRealmWithKey 使用密钥加入 Realm（便捷方法）
//
// 等价于 JoinRealm(ctx, name, WithRealmKey(key), opts...)
func (m *Manager) JoinRealmWithKey(ctx context.Context, name string, key types.RealmKey, opts ...realmif.RealmOption) (realmif.Realm, error) {
	return m.JoinRealm(ctx, name, append(opts, realmif.WithRealmKey(key))...)
}

// connectPrivateBootstrapPeers 连接私有引导节点（REQ-BOOT-005）
//
// Private Realm 不在公共 DHT 中注册，通过已知节点地址直接连接。
func (m *Manager) connectPrivateBootstrapPeers(ctx context.Context, realmID types.RealmID, peers []string) {
	if len(peers) == 0 {
		return
	}

	log.Info("连接 Private Realm 引导节点",
		"realm", string(realmID),
		"count", len(peers))

	if m.endpoint == nil {
		log.Warn("无法连接私有引导节点：endpoint 未注入")
		return
	}

	successCount := 0
	for _, fullAddr := range peers {
		// 解析 Full Address 获取 NodeID 和 Dial Address
		nodeID, dialAddr, err := parseFullAddress(fullAddr)
		if err != nil {
			log.Warn("解析私有引导节点地址失败",
				"addr", fullAddr,
				"err", err)
			continue
		}

		// 创建地址对象
		addr := &stringAddr{s: dialAddr}

		// 使用 ConnectWithAddrs 连接
		connCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		_, err = m.endpoint.ConnectWithAddrs(connCtx, nodeID, []endpoint.Address{addr})
		cancel()
		if err != nil {
			log.Debug("连接私有引导节点失败",
				"addr", fullAddr,
				"err", err)
			continue
		}

		// 连接成功后执行 RealmAuth
		if m.auth != nil {
			if conn, ok := m.endpoint.Connection(nodeID); ok {
				authCtx, authCancel := context.WithTimeout(ctx, 10*time.Second)
				_, authErr := m.auth.Authenticate(authCtx, conn)
				authCancel()
				if authErr != nil {
					log.Warn("私有引导节点 RealmAuth 失败",
						"nodeID", nodeID.ShortString(),
						"err", authErr)
					continue
				}
			}
		}

		successCount++
		log.Info("已连接私有引导节点",
			"nodeID", nodeID.ShortString())

		// 从该节点获取其他 Realm 成员
		go m.syncPeersFrom(ctx, realmID, nodeID)
	}

	log.Info("Private Realm 引导完成",
		"realm", string(realmID),
		"connected", successCount,
		"total", len(peers))
}

// stringAddr 字符串地址辅助类型
type stringAddr struct {
	s string
}

func (a *stringAddr) Network() string   { return "unknown" }
func (a *stringAddr) String() string    { return a.s }
func (a *stringAddr) Bytes() []byte     { return []byte(a.s) }
func (a *stringAddr) IsPublic() bool    { return types.Multiaddr(a.s).IsPublic() }
func (a *stringAddr) IsPrivate() bool   { return types.Multiaddr(a.s).IsPrivate() }
func (a *stringAddr) IsLoopback() bool  { return types.Multiaddr(a.s).IsLoopback() }
func (a *stringAddr) Multiaddr() string { return a.s }
func (a *stringAddr) Equal(other endpoint.Address) bool {
	if other == nil {
		return false
	}
	return a.s == other.String()
}

// parseFullAddress 解析 Full Address 获取 NodeID
//
// 支持格式：/ip4/.../tcp/.../p2p/<NodeID> 或 /ip4/.../udp/.../quic-v1/p2p/<NodeID>
func parseFullAddress(fullAddr string) (types.NodeID, string, error) {
	// 查找 /p2p/ 前缀
	const p2pPrefix = "/p2p/"
	idx := strings.LastIndex(fullAddr, p2pPrefix)
	if idx == -1 {
		return types.NodeID{}, "", ErrInvalidFullAddress
	}

	// 提取 NodeID 字符串
	nodeIDStr := fullAddr[idx+len(p2pPrefix):]

	// 解析 NodeID
	nodeID, err := types.ParseNodeID(nodeIDStr)
	if err != nil {
		return types.NodeID{}, "", err
	}

	// 提取 Dial Address（不含 /p2p/<NodeID>）
	dialAddr := fullAddr[:idx]

	return nodeID, dialAddr, nil
}

// syncPeersFrom 从指定节点同步 Realm 成员
func (m *Manager) syncPeersFrom(_ context.Context, realmID types.RealmID, nodeID types.NodeID) {
	// 使用 Realm 成员同步协议获取其他成员
	// 此处可调用 sync.go 中的同步逻辑
	log.Debug("从节点同步 Realm 成员",
		"realm", string(realmID),
		"from", nodeID.ShortString())
}

// LeaveRealm 离开当前 Realm
//
// v1.1 变更: 无参数，离开当前唯一的 Realm
//   - 如果未加入任何 Realm，返回 ErrNotMember
func (m *Manager) LeaveRealm() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// v1.1: 检查是否加入了 Realm
	if m.primaryRealm == "" {
		return ErrNotMember
	}

	realmStr := m.primaryRealm
	realmID := types.RealmID(realmStr)

	state, exists := m.realms[realmStr]
	if !exists {
		// 不应该发生，但做防御处理
		m.primaryRealm = ""
		return nil
	}

	log.Info("离开 Realm",
		"realm", realmStr)

	// 向 Realm 内邻居发送 Goodbye
	go m.sendGoodbyeToPeers(state.peers)

	// 停止 DHT 通告
	go m.stopDHTAnnounce(realmID)

	delete(m.realms, realmStr)

	// v1.1: 清空主 Realm
	m.primaryRealm = ""

	// IMPL-1227: 清理 Realm 对象
	if m.currentRealmObj != nil {
		m.currentRealmObj.close()
		m.currentRealmObj = nil
	}

	return nil
}

// ============================================================================
//                              DHT 集成
// ============================================================================

// registerWithDHT 向 DHT 注册
func (m *Manager) registerWithDHT(ctx context.Context, realmID types.RealmID) {
	if m.discovery == nil {
		return
	}

	namespace := "realm/" + string(realmID)

	if err := m.discovery.Announce(ctx, namespace); err != nil {
		log.Debug("DHT 注册失败",
			"realm", string(realmID),
			"err", err)
	} else {
		log.Debug("DHT 注册成功",
			"realm", string(realmID))
	}
}

// stopDHTAnnounce 停止 DHT 通告
func (m *Manager) stopDHTAnnounce(realmID types.RealmID) {
	if m.discovery == nil {
		return
	}

	namespace := "realm/" + string(realmID)
	m.discovery.StopAnnounce(namespace)
}

// announceToRealm 通告到 Realm
func (m *Manager) announceToRealm(ctx context.Context, realmID types.RealmID) {
	if m.discovery == nil {
		return
	}

	namespace := "realm/" + string(realmID)

	// 发现同 Realm 的节点
	ch, err := m.discovery.DiscoverPeers(ctx, namespace)
	if err != nil {
		return
	}

	for info := range ch {
		m.addRealmPeer(realmID, info.ID, nil)
	}
}

// ============================================================================
//                              Goodbye 发送
// ============================================================================

// sendRealmGoodbye 发送 Realm Goodbye
func (m *Manager) sendRealmGoodbye(realmID types.RealmID) {
	m.mu.RLock()
	state, exists := m.realms[string(realmID)]
	if !exists {
		m.mu.RUnlock()
		return
	}

	// 复制节点列表
	peers := make(map[types.NodeID]*peerInfo, len(state.peers))
	for k, v := range state.peers {
		peers[k] = v
	}
	m.mu.RUnlock()

	m.sendGoodbyeToPeers(peers)
}

// sendGoodbyeToPeers 向节点发送 Goodbye
func (m *Manager) sendGoodbyeToPeers(peers map[types.NodeID]*peerInfo) {
	if m.liveness == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for nodeID := range peers {
		if err := m.liveness.SendGoodbyeTo(ctx, nodeID, "leaving"); err != nil {
			log.Debug("发送 Goodbye 失败",
				"peer", nodeID.ShortString(),
				"err", err)
		}
	}
}

// ============================================================================
//                              通告循环
// ============================================================================

// announceLoop 通告循环
func (m *Manager) announceLoop() {
	// 检查 ctx 是否为 nil（防止 Start() 未调用）
	if m.ctx == nil {
		return
	}

	// 默认 5 分钟通告一次
	announceInterval := 5 * time.Minute
	ticker := time.NewTicker(announceInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.runAnnounce()
		}
	}
}

// runAnnounce 执行通告
func (m *Manager) runAnnounce() {
	// 检查 ctx 是否为 nil
	if m.ctx == nil {
		return
	}

	m.mu.RLock()
	realms := make([]types.RealmID, 0, len(m.realms))
	for r := range m.realms {
		realms = append(realms, types.RealmID(r))
	}
	m.mu.RUnlock()

	ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	defer cancel()

	for _, realmID := range realms {
		m.registerWithDHT(ctx, realmID)
	}
}

// ============================================================================
//                              节点管理
// ============================================================================

// addRealmPeer 添加 Realm 节点
func (m *Manager) addRealmPeer(realmID types.RealmID, nodeID types.NodeID, addrs []endpoint.Address) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.realms[string(realmID)]
	if !exists {
		return
	}

	state.peers[nodeID] = &peerInfo{
		nodeID:   nodeID,
		addrs:    addrs,
		lastSeen: time.Now(),
	}

	log.Debug("添加 Realm 节点",
		"realm", string(realmID),
		"peer", nodeID.ShortString())
}

// removeRealmPeer 移除 Realm 节点
func (m *Manager) removeRealmPeer(realmID types.RealmID, nodeID types.NodeID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.realms[string(realmID)]
	if !exists {
		return
	}

	delete(state.peers, nodeID)
}

// ============================================================================
//                              查询方法
// ============================================================================

// IsMember 检查是否已加入任何 Realm
//
// v1.1 新增: 无参数便捷方法
//   - 返回 true 表示已加入某个 Realm
//   - 返回 false 表示未加入任何 Realm（业务 API 不可用）
func (m *Manager) IsMember() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.primaryRealm != ""
}

// IsMemberOf 检查是否是指定 Realm 的成员
//
// v1.1 变更: 从 IsMember(realmID) 重命名
func (m *Manager) IsMemberOf(realmID types.RealmID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.primaryRealm == string(realmID)
}

// RealmPeers 返回 Realm 内的节点列表
func (m *Manager) RealmPeers(realmID types.RealmID) []types.NodeID {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.realms[string(realmID)]
	if !exists {
		return nil
	}

	peers := make([]types.NodeID, 0, len(state.peers))
	for p := range state.peers {
		peers = append(peers, p)
	}
	return peers
}

// RealmPeerCount 返回 Realm 内的节点数量
func (m *Manager) RealmPeerCount(realmID types.RealmID) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.realms[string(realmID)]
	if !exists {
		return 0
	}
	return len(state.peers)
}

// RealmDHTKey 计算 Realm 感知的 DHT Key
func (m *Manager) RealmDHTKey(nodeID types.NodeID, realmID types.RealmID) []byte {
	return types.RealmDHTKey(realmID, nodeID)
}

// RealmMetadata 获取 Realm 元数据
func (m *Manager) RealmMetadata(realmID types.RealmID) (*types.RealmMetadata, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	state, exists := m.realms[string(realmID)]
	if !exists {
		return nil, ErrNotMember
	}
	return state.metadata, nil
}
