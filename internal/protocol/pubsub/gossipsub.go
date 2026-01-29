// Package pubsub 实现发布订阅协议
package pubsub

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	pb "github.com/dep2p/go-dep2p/pkg/lib/proto/gossipsub"
	"github.com/dep2p/go-dep2p/pkg/types"
	"google.golang.org/protobuf/proto"
)

// gossipSub GossipSub 协议实现
type gossipSub struct {
	host     interfaces.Host
	realmMgr interfaces.RealmManager // 可选，用于全局模式
	realm    interfaces.Realm        // 可选，用于 Realm 绑定模式
	realmID  string                  // 绑定的 RealmID（如果有）
	config   *Config

	mu            sync.RWMutex
	topics        map[string]*topic   // topic name -> topic
	mesh          *meshPeers          // Mesh 节点管理
	messageCache  *messageCache       // 消息缓存
	seenMessages  *seenMessages       // 已见消息
	heartbeat     *heartbeat          // 心跳管理器
	fanout        map[string][]string // topic -> peers (fanout)
	lastPublished map[string]int64    // topic -> timestamp

	// Phase 5.1: 网络健康监控器（可选）
	// 用于上报发送错误，支持网络状态检测和自动恢复
	healthMonitor interfaces.ConnectionHealthMonitor

	// Phase 9 修复：消息验证器
	// 用于验证接收到的消息（成员资格、大小限制、自定义验证）
	validator *messageValidator

	// P1 修复完成：节点评分器
	scorer *PeerScorer

	// ★ 连接退避跟踪器
	// 记录连接失败的节点和下次允许重试的时间
	// 防止对无法连接的节点进行无限重试
	connectBackoffMu sync.RWMutex
	connectBackoff   map[string]*connectBackoffEntry

	ctx    context.Context
	cancel context.CancelFunc
}

// connectBackoffEntry 连接退避条目
type connectBackoffEntry struct {
	failures    int       // 连续失败次数
	nextRetry   time.Time // 下次允许重试时间
	lastFailure time.Time // 最后一次失败时间
}

// newGossipSub 创建 GossipSub 实例（全局模式）
func newGossipSub(host interfaces.Host, realmMgr interfaces.RealmManager, config *Config) *gossipSub {
	gs := &gossipSub{
		host:           host,
		realmMgr:       realmMgr,
		config:         config,
		topics:         make(map[string]*topic),
		mesh:           newMeshPeers(config.D, config.Dlo, config.Dhi),
		messageCache:   newMessageCache(config.MessageCacheSize),
		seenMessages:   newSeenMessages(config.SeenMessagesTTL),
		fanout:         make(map[string][]string),
		lastPublished:  make(map[string]int64),
		connectBackoff: make(map[string]*connectBackoffEntry),
	}

	gs.heartbeat = newHeartbeat(config.HeartbeatInterval, gs)

	// Phase 9 修复：初始化消息验证器（全局模式）
	gs.validator = newMessageValidator(realmMgr, config.MaxMessageSize)

	// P1 修复完成：初始化节点评分器
	if config.PeerScoring.Enabled {
		gs.scorer = NewPeerScorerWithThresholds(
			config.PeerScoring.ScoreParams,
			config.PeerScoring.GossipThreshold,
			config.PeerScoring.PublishThreshold,
			config.PeerScoring.GraylistThreshold,
			config.PeerScoring.AcceptPXThreshold,
		)
		logger.Info("已启用 GossipSub 节点评分")
	}

	return gs
}

// newGossipSubForRealm 创建绑定到特定 Realm 的 GossipSub 实例
func newGossipSubForRealm(host interfaces.Host, realm interfaces.Realm, config *Config) *gossipSub {
	gs := &gossipSub{
		host:           host,
		realm:          realm,
		realmID:        realm.ID(),
		config:         config,
		topics:         make(map[string]*topic),
		mesh:           newMeshPeers(config.D, config.Dlo, config.Dhi),
		messageCache:   newMessageCache(config.MessageCacheSize),
		seenMessages:   newSeenMessages(config.SeenMessagesTTL),
		fanout:         make(map[string][]string),
		lastPublished:  make(map[string]int64),
		connectBackoff: make(map[string]*connectBackoffEntry),
	}

	gs.heartbeat = newHeartbeat(config.HeartbeatInterval, gs)

	// Phase 9 修复：初始化消息验证器（Realm 绑定模式）
	gs.validator = newMessageValidatorForRealm(realm, config.MaxMessageSize)

	// P1 修复完成：初始化节点评分器
	if config.PeerScoring.Enabled {
		gs.scorer = NewPeerScorerWithThresholds(
			config.PeerScoring.ScoreParams,
			config.PeerScoring.GossipThreshold,
			config.PeerScoring.PublishThreshold,
			config.PeerScoring.GraylistThreshold,
			config.PeerScoring.AcceptPXThreshold,
		)
		logger.Info("已启用 GossipSub 节点评分", "realm", realm.ID())
	}

	return gs
}

// Start 启动 GossipSub
func (gs *gossipSub) Start(ctx context.Context) error {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	// 如果 ctx 是从 Fx OnStart 传递下来的，使用 Background 更安全
	// 但这里的 ctx 通常是从 Service.Start 传递的已处理 ctx
	gs.ctx, gs.cancel = context.WithCancel(ctx)

	// 根据模式注册 GossipSub 协议处理器
	if gs.realm != nil && gs.realmID != "" {
		// Realm-bound 模式：只为绑定的 Realm 注册
		protocolID := buildProtocolID(gs.realmID)
		gs.host.SetStreamHandler(string(protocolID), gs.handleStream)
	} else if gs.realmMgr != nil {
		// 全局模式：为所有 Realm 注册
		realms := gs.realmMgr.ListRealms()
		for _, realm := range realms {
			protocolID := buildProtocolID(realm.ID())
			gs.host.SetStreamHandler(string(protocolID), gs.handleStream)
		}
	}

	// P2-L1 修复：订阅断开事件，及时清理 Mesh
	go gs.subscribeDisconnectEvents()

	// 启动心跳(除非禁用)
	if !gs.config.DisableHeartbeat {
		gs.heartbeat.Start(gs.ctx)
	}

	return nil
}

// Stop 停止 GossipSub
func (gs *gossipSub) Stop() error {
	gs.mu.Lock()

	if gs.cancel != nil {
		gs.cancel()
	}

	// 停止心跳(如果启动了)
	if !gs.config.DisableHeartbeat {
		gs.heartbeat.Stop()
	}

	// 获取所有主题(不持有锁)
	topics := make([]*topic, 0, len(gs.topics))
	for _, t := range gs.topics {
		topics = append(topics, t)
	}
	gs.mu.Unlock()

	// 在不持有锁的情况下关闭主题
	for _, topic := range topics {
		topic.Close()
	}

	return nil
}

// Join 加入主题
func (gs *gossipSub) Join(name string) (*topic, error) {
	// 先检查是否已存在(使用读锁)
	gs.mu.RLock()
	_, exists := gs.topics[name]
	gs.mu.RUnlock()

	if exists {
		return nil, ErrTopicAlreadyJoined
	}

	// 创建主题(使用写锁)
	gs.mu.Lock()
	// 再次检查(double-check pattern)
	if _, exists := gs.topics[name]; exists {
		gs.mu.Unlock()
		return nil, ErrTopicAlreadyJoined
	}

	t := newTopic(name, nil, gs) // ps 会在 Service.Join 中设置
	gs.topics[name] = t
	gs.mu.Unlock()

	// 立即为新 topic 建立 Mesh，不等待心跳周期
	gs.graftPeers(name)

	return t, nil
}

// Leave 离开主题
func (gs *gossipSub) Leave(name string) error {
	gs.mu.Lock()
	defer gs.mu.Unlock()

	// 清理 Mesh
	gs.mesh.Clear(name)

	// 删除主题
	delete(gs.topics, name)

	return nil
}

// Publish 发布消息到主题
func (gs *gossipSub) Publish(_ context.Context, topicName string, msg *pb.Message) error {
	// 生成消息 ID
	msgID := messageID(string(msg.From), msg.Seqno)

	// 检查是否重复
	if gs.seenMessages.Has(msgID) {
		return ErrDuplicateMessage
	}

	// 标记为已见
	gs.seenMessages.Add(msgID)

	// 添加到缓存
	gs.messageCache.Put(msg)

	// 投递给本地订阅者
	gs.mu.RLock()
	topic, exists := gs.topics[topicName]
	gs.mu.RUnlock()

	if exists {
		interfaceMsg := protoToInterface(msg)
		topic.deliverMessage(interfaceMsg)
	}

	// 转发给 Mesh 节点
	return gs.forwardToMesh(topicName, msg)
}

// forwardToMesh 转发消息给 Mesh 节点
//
// Phase 5.1 语义修复: 无连接时返回错误，不静默成功
// - 如果 Mesh 中没有节点，返回 ErrNoConnectedPeers
// - 如果所有发送都失败，返回 ErrAllSendsFailed
func (gs *gossipSub) forwardToMesh(topicName string, msg *pb.Message) error {
	peers := gs.mesh.List(topicName)

	logger.Debug("forwardToMesh 转发消息",
		"topic", topicName,
		"meshPeers", len(peers))

	// Phase 5.1: 无 Mesh 节点时返回错误，不静默成功
	if len(peers) == 0 {
		logger.Warn("forwardToMesh: 没有可用的 Mesh 节点",
			"topic", topicName)
		return ErrNoConnectedPeers
	}

	// 序列化消息
	data, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Phase 5.1: 追踪发送结果
	type sendResult struct {
		peer string
		err  error
	}

	resultCh := make(chan sendResult, len(peers))

	// 发送给所有 Mesh 节点
	for _, peerID := range peers {
		go func(pid string) {
			err := gs.sendMessage(pid, topicName, data)
			resultCh <- sendResult{peer: pid, err: err}
		}(peerID)
	}

	// 收集结果
	var successCount, failCount int
	var lastErr error

	for i := 0; i < len(peers); i++ {
		result := <-resultCh
		if result.err != nil {
			failCount++
			lastErr = result.err

			// P2-L4 修复：记录发送失败到评分器
			if gs.scorer != nil {
				gs.scorer.DeliveryFailed(result.peer, topicName)
			}

			logger.Debug("sendMessage 失败",
				"peer", result.peer,
				"error", result.err)
		} else {
			successCount++
		}
	}

	// Phase 5.1: 所有发送都失败时返回错误
	if successCount == 0 && failCount > 0 {
		logger.Warn("forwardToMesh: 所有发送都失败",
			"topic", topicName,
			"failCount", failCount,
			"lastError", lastErr)
		return fmt.Errorf("%w: last error: %v", ErrAllSendsFailed, lastErr)
	}

	logger.Debug("forwardToMesh 完成",
		"topic", topicName,
		"success", successCount,
		"failed", failCount)

	return nil
}

// sendMessage 发送消息给节点
//
// Phase 5.1: 发送结果会上报到 NetworkMonitor（如果设置了）
func (gs *gossipSub) sendMessage(peerID, _ string, data []byte) error {
	// 查找所属 Realm
	realm := gs.findRealmForPeer(peerID)
	if realm == nil {
		err := fmt.Errorf("%w: peer=%s", ErrNotRealmMember, peerID)
		gs.reportSendError(peerID, err)
		return err
	}

	// 确保连接已建立，避免协议协商超时
	if network := gs.host.Network(); network != nil && network.Connectedness(peerID) != interfaces.Connected {
		ctx := gs.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		connCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		_, err := realm.Connect(connCtx, peerID)
		cancel()
		if err != nil {
			gs.reportSendError(peerID, err)
			return fmt.Errorf("ensure connected: %w", err)
		}
	}

	// 构造协议 ID
	protocolID := buildProtocolID(realm.ID())

	// 打开流
	stream, err := gs.host.NewStream(gs.ctx, peerID, string(protocolID))
	if err != nil {
		gs.reportSendError(peerID, err)
		return err
	}
	defer stream.Close()

	// 写入消息
	_, err = stream.Write(data)
	if err != nil {
		gs.reportSendError(peerID, err)
		return err
	}

	// Phase 5.1: 发送成功上报
	gs.reportSendSuccess(peerID)
	return nil
}

// SetHealthMonitor 设置网络健康监控器
//
// Phase 5.1: 用于支持发送错误上报和网络状态检测
func (gs *gossipSub) SetHealthMonitor(monitor interfaces.ConnectionHealthMonitor) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	gs.healthMonitor = monitor
}

// reportSendError 上报发送错误到 NetworkMonitor
func (gs *gossipSub) reportSendError(peerID string, err error) {
	gs.mu.RLock()
	monitor := gs.healthMonitor
	gs.mu.RUnlock()

	if monitor != nil {
		monitor.OnSendError(peerID, err)
	}
}

// reportSendSuccess 上报发送成功到 NetworkMonitor
func (gs *gossipSub) reportSendSuccess(peerID string) {
	gs.mu.RLock()
	monitor := gs.healthMonitor
	gs.mu.RUnlock()

	if monitor != nil {
		monitor.OnSendSuccess(peerID)
	}
}

// handleStream 处理incoming流
func (gs *gossipSub) handleStream(stream interfaces.Stream) {
	defer stream.Close()

	// 读取消息
	data, err := io.ReadAll(stream)
	if err != nil {
		return
	}

	// 解析消息
	msg := &pb.Message{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return
	}

	// 处理消息
	gs.handleMessage(string(stream.Conn().RemotePeer()), msg)
}

// handleMessage 处理接收到的消息
func (gs *gossipSub) handleMessage(peerID string, msg *pb.Message) {
	msgID := messageID(string(msg.From), msg.Seqno)

	// 检查是否已见
	isFirst := !gs.seenMessages.Has(msgID)
	if !isFirst {
		// P1 修复完成：记录重复消息到评分器
		if gs.scorer != nil {
			gs.scorer.DuplicateMessage(peerID, msg.Topic, true)
		}
		return
	}

	// 标记为已见
	gs.seenMessages.Add(msgID)

	// Phase 9 修复：使用 messageValidator 进行消息验证
	// 验证包括：消息大小、必要字段、Realm 成员资格、自定义验证器
	isValid := true
	if gs.validator != nil {
		if err := gs.validator.Validate(gs.ctx, peerID, msg); err != nil {
			isValid = false
			logger.Debug("消息验证失败",
				"peerID", peerID,
				"topic", msg.Topic,
				"error", err,
			)
		}
	}

	// P1 修复完成：记录消息验证结果到评分器
	if gs.scorer != nil {
		gs.scorer.ValidateMessage(peerID, msg.Topic, isFirst, isValid)
	}

	if !isValid {
		return
	}

	// 添加到缓存
	gs.messageCache.Put(msg)

	// 投递给本地订阅者
	gs.mu.RLock()
	topic, exists := gs.topics[msg.Topic]
	gs.mu.RUnlock()

	if exists {
		interfaceMsg := protoToInterface(msg)
		interfaceMsg.ReceivedFrom = peerID
		topic.deliverMessage(interfaceMsg)
	}

	// 转发给其他 Mesh 节点(除了发送者)
	gs.forwardToMeshExcept(msg.Topic, msg, peerID)
}

// forwardToMeshExcept 转发给 Mesh 节点(排除指定节点)
func (gs *gossipSub) forwardToMeshExcept(topicName string, msg *pb.Message, exceptPeer string) {
	peers := gs.mesh.List(topicName)

	data, err := proto.Marshal(msg)
	if err != nil {
		return
	}

	for _, peerID := range peers {
		if peerID == exceptPeer {
			continue
		}

		go func(pid string) {
			gs.sendMessage(pid, topicName, data)
		}(peerID)
	}
}

// maintainMesh 维护 Mesh
//
// P2-L2 修复：先清理断开的节点，再检查数量
func (gs *gossipSub) maintainMesh() {
	gs.mu.RLock()
	topics := make([]string, 0, len(gs.topics))
	for name := range gs.topics {
		topics = append(topics, name)
	}
	gs.mu.RUnlock()

	for _, topicName := range topics {
		// P2-L2 修复：先清理断开的节点
		gs.cleanupDisconnectedPeers(topicName)

		// 检查是否需要更多节点
		if gs.mesh.NeedMorePeers(topicName) {
			gs.graftPeers(topicName)
		}

		// 检查是否节点过多
		if gs.mesh.TooManyPeers(topicName) {
			gs.prunePeers(topicName)
		}
	}
}

// graftPeers 添加节点到 Mesh
//
// 日志优化：只在实际 graft 节点时输出日志，减少 heartbeat 噪音。
// P2-L3 修复：添加前验证节点连接状态。
// 20: 当 Mesh 为空且无已连接节点时，主动尝试连接候选节点
func (gs *gossipSub) graftPeers(topicName string) {
	// 获取候选节点(从 Realm 成员中选择)
	candidates := gs.getCandidatePeers(topicName)

	// P1 修复完成：过滤低评分节点
	if gs.scorer != nil {
		filtered := make([]string, 0, len(candidates))
		for _, peerID := range candidates {
			// 不选择低于 gossip 阈值的节点
			if !gs.scorer.IsBelowGossipThreshold(peerID) {
				filtered = append(filtered, peerID)
			}
		}
		candidates = filtered
	}

	// P2-L3 修复：过滤未连接的节点
	connectedCandidates := gs.filterConnectedPeers(candidates)

	// 计算需要的节点数
	needed := gs.config.D - gs.mesh.Count(topicName)
	if needed <= 0 {
		return
	}

	// 20: 如果已连接的候选节点不足，尝试主动连接其他候选节点
	if len(connectedCandidates) < needed && len(candidates) > len(connectedCandidates) {
		gs.tryConnectCandidates(candidates, connectedCandidates, needed-len(connectedCandidates))
		// 重新获取已连接的候选节点
		connectedCandidates = gs.filterConnectedPeers(candidates)
	}

	// 选择节点
	toGraft := gs.mesh.SelectPeersToGraft(topicName, connectedCandidates, needed)

	// 没有可 graft 的节点时静默返回
	if len(toGraft) == 0 {
		return
	}

	// 添加到 Mesh
	for _, peerID := range toGraft {
		gs.mesh.Add(topicName, peerID)

		// P1 修复完成：通知评分器 GRAFT
		if gs.scorer != nil {
			gs.scorer.Graft(peerID, topicName)
		}

		// 通知 Topic
		gs.mu.RLock()
		topic, exists := gs.topics[topicName]
		gs.mu.RUnlock()

		if exists {
			topic.notifyPeerJoin(peerID)
		}
	}

	//
	logger.Debug("graftPeers 添加节点到 Mesh",
		"topic", topicName,
		"grafted", len(toGraft),
		"meshCount", gs.mesh.Count(topicName))
}

// prunePeers 从 Mesh 移除节点
func (gs *gossipSub) prunePeers(topicName string) {
	// 计算需要移除的节点数
	toRemove := gs.mesh.Count(topicName) - gs.config.D
	if toRemove <= 0 {
		return
	}

	// P1 修复完成：优先移除低评分节点
	var toPrune []string
	if gs.scorer != nil {
		meshPeers := gs.mesh.List(topicName)
		// 按评分排序，移除评分最低的节点
		type peerScore struct {
			id    string
			score float64
		}
		scores := make([]peerScore, 0, len(meshPeers))
		for _, pid := range meshPeers {
			scores = append(scores, peerScore{id: pid, score: gs.scorer.Score(pid)})
		}
		// 简单排序：选择评分最低的
		for i := 0; i < toRemove && i < len(scores); i++ {
			minIdx := i
			for j := i + 1; j < len(scores); j++ {
				if scores[j].score < scores[minIdx].score {
					minIdx = j
				}
			}
			scores[i], scores[minIdx] = scores[minIdx], scores[i]
			toPrune = append(toPrune, scores[i].id)
		}
	} else {
		// 无评分器时使用原有逻辑
		toPrune = gs.mesh.SelectPeersToPrune(topicName, toRemove)
	}

	// 从 Mesh 移除
	for _, peerID := range toPrune {
		gs.mesh.Remove(topicName, peerID)

		// P1 修复完成：通知评分器 PRUNE
		if gs.scorer != nil {
			gs.scorer.Prune(peerID, topicName)
		}

		// 通知 Topic
		gs.mu.RLock()
		topic, exists := gs.topics[topicName]
		gs.mu.RUnlock()

		if exists {
			topic.notifyPeerLeave(peerID)
		}
	}
}

// getCandidatePeers 获取候选节点
//
// 候选节点来源：
// 1. Realm 成员列表
// 2. 已建立连接的节点（确保消息可以通过已连接节点转发）
func (gs *gossipSub) getCandidatePeers(_ string) []string {
	candidateSet := make(map[string]bool)

	// Realm-bound 模式：从绑定的 Realm 获取成员
	if gs.realm != nil {
		members := gs.realm.Members()
		for _, member := range members {
			// 排除自己
			if member != gs.host.ID() {
				candidateSet[member] = true
			}
		}
	} else if gs.realmMgr != nil {
		// 全局模式：从所有 Realm 成员中选择
		realms := gs.realmMgr.ListRealms()
		for _, realm := range realms {
			members := realm.Members()
			for _, member := range members {
				// 排除自己
				if member != gs.host.ID() {
					candidateSet[member] = true
				}
			}
		}
	}

	// 同时添加已知地址的节点（确保消息可以通过已知节点转发）
	// 这解决了"鸡和蛋"问题：mesh 需要成员列表，成员列表需要 mesh 传播
	// P2: 仅纳入已是 Realm 成员的节点，避免把 Bootstrap/Relay 误加进 Mesh。
	if peerstore := gs.host.Peerstore(); peerstore != nil {
		knownPeers := peerstore.PeersWithAddrs()
		for _, peerID := range knownPeers {
			if string(peerID) != gs.host.ID() {
				if gs.findRealmForPeer(string(peerID)) != nil {
					candidateSet[string(peerID)] = true
				}
			}
		}
	}

	// 转换为切片
	candidates := make([]string, 0, len(candidateSet))
	for peer := range candidateSet {
		candidates = append(candidates, peer)
	}

	return candidates
}

// findRealmForPeer 查找节点所属 Realm
func (gs *gossipSub) findRealmForPeer(peerID string) interfaces.Realm {
	// Realm-bound 模式：直接返回绑定的 Realm（如果 peer 是成员）
	if gs.realm != nil {
		if gs.realm.IsMember(peerID) {
			return gs.realm
		}
		return nil
	}

	// 全局模式：从 RealmManager 查找
	if gs.realmMgr == nil {
		return nil
	}

	realms := gs.realmMgr.ListRealms()
	for _, realm := range realms {
		if realm.IsMember(peerID) {
			return realm
		}
	}
	return nil
}

// cleanupSeenMessages 清理过期的已见消息
func (gs *gossipSub) cleanupSeenMessages() {
	gs.seenMessages.Cleanup()
}

// cleanupMessageCache 清理过期的消息缓存
func (gs *gossipSub) cleanupMessageCache() {
	gs.messageCache.CleanupOld(gs.config.SeenMessagesTTL)
}

// decayScores 执行评分衰减（由心跳调用）
func (gs *gossipSub) decayScores() {
	if gs.scorer != nil {
		gs.scorer.Decay()
	}
}

// GetScorer 获取评分器（供外部访问）
func (gs *gossipSub) GetScorer() *PeerScorer {
	return gs.scorer
}

// GetPeerScore 获取节点评分
func (gs *gossipSub) GetPeerScore(peerID string) float64 {
	if gs.scorer == nil {
		return 0
	}
	return gs.scorer.Score(peerID)
}

// SetAppScore 设置节点的应用层评分
func (gs *gossipSub) SetAppScore(peerID string, score float64) {
	if gs.scorer != nil {
		gs.scorer.SetAppScore(peerID, score)
	}
}

// AddPeer 添加节点到评分器
func (gs *gossipSub) AddPeer(peerID string, ip string) {
	if gs.scorer != nil {
		gs.scorer.AddPeer(peerID, ip)
	}
}

// RemovePeer 从评分器移除节点
func (gs *gossipSub) RemovePeer(peerID string) {
	if gs.scorer != nil {
		gs.scorer.RemovePeer(peerID)
	}
}

// ============================================================================
// P2 修复：Mesh 清理机制
// ============================================================================

// subscribeDisconnectEvents 订阅断开事件并处理
//
// P2-L1 修复：在节点断开的第一时间从 Mesh 中移除，响应及时。
func (gs *gossipSub) subscribeDisconnectEvents() {
	eventBus := gs.host.EventBus()
	if eventBus == nil {
		logger.Warn("EventBus 不可用，无法订阅断开事件")
		return
	}

	sub, err := eventBus.Subscribe(new(types.EvtPeerDisconnected))
	if err != nil {
		logger.Warn("订阅断开事件失败", "error", err)
		return
	}
	// 处理订阅返回 nil 的情况（例如 mock 实现）
	if sub == nil {
		logger.Warn("订阅返回 nil，无法监听断开事件")
		return
	}
	defer sub.Close()

	for {
		select {
		case <-gs.ctx.Done():
			return
		case evt := <-sub.Out():
			disconnected, ok := evt.(*types.EvtPeerDisconnected)
			if !ok {
				continue
			}
			gs.handlePeerDisconnected(string(disconnected.PeerID))
		}
	}
}

// handlePeerDisconnected 处理节点断开
//
// 从所有 Topic 的 Mesh 中移除该节点，通知评分器。
func (gs *gossipSub) handlePeerDisconnected(peerID string) {
	gs.mu.RLock()
	topics := make([]string, 0, len(gs.topics))
	for name := range gs.topics {
		topics = append(topics, name)
	}
	gs.mu.RUnlock()

	peerIDShort := peerID
	if len(peerIDShort) > 8 {
		peerIDShort = peerIDShort[:8]
	}

	removed := false
	for _, topicName := range topics {
		if gs.mesh.Has(topicName, peerID) {
			gs.mesh.Remove(topicName, peerID)
			removed = true

			// 通知评分器 PRUNE
			if gs.scorer != nil {
				gs.scorer.Prune(peerID, topicName)
			}

			// 通知 Topic
			gs.mu.RLock()
			topic, exists := gs.topics[topicName]
			gs.mu.RUnlock()
			if exists {
				topic.notifyPeerLeave(peerID)
			}
		}
	}

	if removed {
		logger.Debug("节点断开，已从 Mesh 移除",
			"peer", peerIDShort)
	}

	// 同时从评分器移除
	if gs.scorer != nil {
		gs.scorer.RemovePeer(peerID)
	}
}

// cleanupDisconnectedPeers 清理断开的 Mesh 节点
//
// P2-L2 修复：作为兜底机制，在心跳维护时检查 Mesh 节点的连接状态。
func (gs *gossipSub) cleanupDisconnectedPeers(topicName string) {
	peers := gs.mesh.List(topicName)
	if len(peers) == 0 {
		return
	}

	// 获取 Network 接口检查连接状态
	network := gs.host.Network()
	if network == nil {
		return
	}

	var removed []string
	for _, peerID := range peers {
		if network.Connectedness(peerID) != interfaces.Connected {
			gs.mesh.Remove(topicName, peerID)
			removed = append(removed, peerID)

			// 通知评分器
			if gs.scorer != nil {
				gs.scorer.Prune(peerID, topicName)
			}

			// 通知 Topic
			gs.mu.RLock()
			topic, exists := gs.topics[topicName]
			gs.mu.RUnlock()
			if exists {
				topic.notifyPeerLeave(peerID)
			}
		}
	}

	if len(removed) > 0 {
		logger.Debug("心跳清理断开的 Mesh 节点",
			"topic", topicName,
			"removed", len(removed),
			"remaining", gs.mesh.Count(topicName))
	}
}

// filterConnectedPeers 过滤已连接的节点
//
// P2-L3 修复：在将节点添加到 Mesh 前，验证其确实已连接。
func (gs *gossipSub) filterConnectedPeers(candidates []string) []string {
	network := gs.host.Network()
	if network == nil {
		return candidates // 无法检查，返回原列表
	}

	connected := make([]string, 0, len(candidates))
	for _, peerID := range candidates {
		if network.Connectedness(peerID) == interfaces.Connected {
			connected = append(connected, peerID)
		}
	}
	return connected
}

// ============================================================================
//                              连接退避机制
// ============================================================================

// 退避参数常量
const (
	// 基础退避时间
	connectBackoffBase = 5 * time.Second
	// 最大退避时间
	connectBackoffMax = 5 * time.Minute
	// 退避记录过期时间（超过此时间的成功重试清除记录）
	connectBackoffExpiry = 10 * time.Minute
)

// tryConnectCandidates 尝试连接候选节点
//
// 20: 当 Mesh 需要更多节点但已连接节点不足时，
// 主动尝试连接其他候选节点以建立 Mesh。
//
// 添加指数退避机制，防止对无法连接的节点无限重试
//
// 参数:
//   - allCandidates: 所有候选节点
//   - alreadyConnected: 已连接的候选节点
//   - needed: 还需要连接的节点数
func (gs *gossipSub) tryConnectCandidates(allCandidates, alreadyConnected []string, needed int) {
	if needed <= 0 {
		return
	}

	// 构建已连接节点的集合
	connectedSet := make(map[string]bool)
	for _, pid := range alreadyConnected {
		connectedSet[pid] = true
	}

	// 获取 Peerstore 以查找地址
	peerstore := gs.host.Peerstore()
	if peerstore == nil {
		logger.Debug("tryConnectCandidates: Peerstore 不可用，跳过主动连接")
		return
	}

	now := time.Now()

	// 尝试连接未连接的候选节点
	connectCount := 0
	skippedBackoff := 0
	for _, peerID := range allCandidates {
		if connectCount >= needed {
			break
		}
		if connectedSet[peerID] {
			continue // 已连接，跳过
		}

		// ★ 检查退避状态
		if gs.isInConnectBackoff(peerID, now) {
			skippedBackoff++
			continue // 在退避期内，跳过
		}

		// 从 Peerstore 获取地址
		addrs := peerstore.Addrs(types.PeerID(peerID))
		if len(addrs) == 0 {
			// 无地址也记录失败，避免频繁检查
			gs.recordConnectFailure(peerID, now)
			continue
		}

		// 转换地址为字符串切片
		addrStrs := make([]string, len(addrs))
		for i, addr := range addrs {
			addrStrs[i] = addr.String()
		}

		// 尝试连接（使用短超时避免阻塞）
		ctx, cancel := context.WithTimeout(context.Background(), gs.config.HeartbeatInterval)
		err := gs.host.Connect(ctx, peerID, addrStrs)
		cancel()

		if err != nil {
			// ★ 记录失败并更新退避
			gs.recordConnectFailure(peerID, now)
			logger.Debug("tryConnectCandidates: 连接候选节点失败",
				"peer", peerID[:min(8, len(peerID))],
				"error", err)
		} else {
			// ★ 成功则清除退避记录
			gs.clearConnectBackoff(peerID)
			logger.Info("tryConnectCandidates: 成功连接候选节点",
				"peer", peerID[:min(8, len(peerID))])
			connectCount++
		}
	}

	if connectCount > 0 || skippedBackoff > 0 {
		logger.Debug("tryConnectCandidates: 完成主动连接尝试",
			"connected", connectCount,
			"needed", needed,
			"skippedBackoff", skippedBackoff)
	}
}

// isInConnectBackoff 检查节点是否在退避期内
func (gs *gossipSub) isInConnectBackoff(peerID string, now time.Time) bool {
	gs.connectBackoffMu.RLock()
	defer gs.connectBackoffMu.RUnlock()

	entry, ok := gs.connectBackoff[peerID]
	if !ok {
		return false
	}
	return now.Before(entry.nextRetry)
}

// recordConnectFailure 记录连接失败
func (gs *gossipSub) recordConnectFailure(peerID string, now time.Time) {
	gs.connectBackoffMu.Lock()
	defer gs.connectBackoffMu.Unlock()

	entry, ok := gs.connectBackoff[peerID]
	if !ok {
		entry = &connectBackoffEntry{}
		gs.connectBackoff[peerID] = entry
	}

	entry.failures++
	entry.lastFailure = now

	// 计算退避时间：base * 2^(failures-1)，最大不超过 max
	backoff := connectBackoffBase * time.Duration(1<<min(entry.failures-1, 6))
	if backoff > connectBackoffMax {
		backoff = connectBackoffMax
	}
	entry.nextRetry = now.Add(backoff)

	// 只在首次失败或达到特定阈值时输出日志
	if entry.failures == 1 || entry.failures == 5 || entry.failures == 10 || entry.failures%50 == 0 {
		logger.Debug("连接退避已更新",
			"peerID", peerID[:min(8, len(peerID))],
			"failures", entry.failures,
			"backoff", backoff)
	}
}

// clearConnectBackoff 清除退避记录（连接成功时调用）
func (gs *gossipSub) clearConnectBackoff(peerID string) {
	gs.connectBackoffMu.Lock()
	defer gs.connectBackoffMu.Unlock()

	if entry, ok := gs.connectBackoff[peerID]; ok {
		if entry.failures > 0 {
			logger.Debug("清除连接退避（连接成功）",
				"peerID", peerID[:min(8, len(peerID))],
				"previousFailures", entry.failures)
		}
		delete(gs.connectBackoff, peerID)
	}
}

// cleanupConnectBackoff 清理过期的退避记录
// 由心跳定期调用
func (gs *gossipSub) cleanupConnectBackoff() {
	gs.connectBackoffMu.Lock()
	defer gs.connectBackoffMu.Unlock()

	now := time.Now()
	expiry := connectBackoffExpiry

	for peerID, entry := range gs.connectBackoff {
		// 如果最后一次失败已经超过过期时间，清除记录
		if now.Sub(entry.lastFailure) > expiry {
			delete(gs.connectBackoff, peerID)
		}
	}
}
