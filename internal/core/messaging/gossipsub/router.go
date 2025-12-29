// Package gossipsub 实现 GossipSub v1.1 协议
package gossipsub

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/sha512"
	"math/big"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 包级别日志实例
var log = logger.Logger("messaging.gossipsub")

// ============================================================================
//                              GossipSub 路由器
// ============================================================================

// GossipRouter GossipSub 路由器接口
type GossipRouter interface {
	// Join 加入主题（开始接收消息）
	Join(topic string) error

	// Leave 离开主题
	Leave(topic string) error

	// Publish 发布消息
	Publish(ctx context.Context, topic string, data []byte) error

	// Subscribe 订阅主题，返回消息通道
	Subscribe(topic string) (<-chan *Message, func(), error)

	// AddPeer 添加 peer
	AddPeer(peer types.NodeID, outbound bool)

	// RemovePeer 移除 peer
	RemovePeer(peer types.NodeID)

	// HandleRPC 处理收到的 RPC
	HandleRPC(from types.NodeID, rpc *RPC) error

	// PeersInTopic 返回订阅指定 topic 的所有已知 peers
	//
	// 返回通过 GossipSub 协议发现的订阅者（本节点视角）。
	// 与 libp2p pubsub.ListPeers(topic) 语义一致。
	PeersInTopic(topic string) []types.NodeID

	// MeshPeers 返回指定 topic 的 mesh peers
	//
	// 返回 mesh 网络中的 peers（约 D=6 个），用于第一跳消息传播。
	MeshPeers(topic string) []types.NodeID

	// Start 启动路由器
	Start(ctx context.Context) error

	// Stop 停止路由器
	Stop() error
}

// Router GossipSub 路由器实现
type Router struct {
	mu sync.RWMutex

	// config 配置
	config *Config

	// localID 本地节点 ID
	localID types.NodeID

	// identity 本地身份（用于签名）
	identity identityif.Identity

	// endpoint 网络端点
	endpoint endpoint.Endpoint

	// mesh mesh 管理器
	mesh *MeshManager

	// cache 消息缓存
	cache *MessageCache

	// seenCache 已见消息缓存
	seenCache *SeenCache

	// scorer 评分器
	scorer *PeerScorer

	// heartbeat 心跳管理器
	heartbeat *Heartbeat

	// codec 协议编解码器
	codec *RPCCodec

	// subscriptions 本地订阅
	subscriptions map[string][]*localSubscription

	// seqNo 消息序列号
	seqNo uint64

	// running 运行状态
	running int32

	// stopCh 停止通道
	stopCh chan struct{}

	// ctx 上下文
	ctx    context.Context
	cancel context.CancelFunc

	// sentInitialSubs 记录已发送初始订阅快照的 peers
	// 确保每个 peer 只收到一次快照，避免重复/抖动
	sentInitialSubs map[types.NodeID]bool
}

// localSubscription 本地订阅
type localSubscription struct {
	topic    string
	messages chan *Message
	active   int32
}

// NewRouter 创建新的 GossipSub 路由器
func NewRouter(
	config *Config,
	localID types.NodeID,
	identity identityif.Identity,
	endpoint endpoint.Endpoint,
) *Router {
	if config == nil {
		config = DefaultConfig()
	}
	_ = config.Validate()

	// 创建组件
	scorer := NewPeerScorer(DefaultScoreParams())
	mesh := NewMeshManager(config, scorer)
	cache := NewMessageCache(config.HistoryLength, config.HistoryGossip)
	seenCache := NewSeenCache(config.SeenTTL, 100000)
	heartbeat := NewHeartbeat(config, mesh, cache, scorer)
	codec := NewRPCCodec()

	router := &Router{
		config:          config,
		localID:         localID,
		identity:        identity,
		endpoint:        endpoint,
		mesh:            mesh,
		cache:           cache,
		seenCache:       seenCache,
		scorer:          scorer,
		heartbeat:       heartbeat,
		codec:           codec,
		subscriptions:   make(map[string][]*localSubscription),
		stopCh:          make(chan struct{}),
		sentInitialSubs: make(map[types.NodeID]bool),
	}

	// 设置心跳的 RPC 发送回调
	heartbeat.SetSendRPC(router.sendRPC)

	return router
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动路由器
func (r *Router) Start(_ context.Context) error {
	if !atomic.CompareAndSwapInt32(&r.running, 0, 1) {
		return nil // 已经运行
	}

	// 使用 context.Background() 而非 ctx，因为 Fx OnStart 的 ctx 在 OnStart 返回后会被取消
	// 这会导致 Heartbeat 提前退出
	r.ctx, r.cancel = context.WithCancel(context.Background())
	r.stopCh = make(chan struct{})

	log.Info("GossipSub 路由器启动中")

	// 注册协议处理器
	if r.endpoint != nil {
		r.endpoint.SetProtocolHandler(ProtocolGossipSub, r.handleStream)
	}

	// 启动心跳
	if err := r.heartbeat.Start(r.ctx); err != nil {
		return err
	}

	log.Info("GossipSub 路由器已启动")
	return nil
}

// Stop 停止路由器
func (r *Router) Stop() error {
	if !atomic.CompareAndSwapInt32(&r.running, 1, 0) {
		return nil // 已经停止
	}

	log.Info("GossipSub 路由器停止中")

	// 取消上下文
	if r.cancel != nil {
		r.cancel()
	}

	// 停止心跳
	_ = r.heartbeat.Stop() // 心跳停止错误可忽略

	// 移除协议处理器
	if r.endpoint != nil {
		r.endpoint.RemoveProtocolHandler(ProtocolGossipSub)
	}

	// 关闭所有订阅
	r.mu.Lock()
	for _, subs := range r.subscriptions {
		for _, sub := range subs {
			if atomic.CompareAndSwapInt32(&sub.active, 1, 0) {
				close(sub.messages)
			}
		}
	}
	r.subscriptions = make(map[string][]*localSubscription)
	r.mu.Unlock()

	close(r.stopCh)

	log.Info("GossipSub 路由器已停止")
	return nil
}

// ============================================================================
//                              主题操作
// ============================================================================

// Join 加入主题
func (r *Router) Join(topic string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 加入 mesh
	toGraft := r.mesh.Join(topic)

	// 发送 GRAFT 消息
	for _, peer := range toGraft {
		r.sendGraftAsync(peer, topic)
	}

	// 广播订阅变更
	r.broadcastSubscription(topic, true)

	log.Info("加入主题",
		"topic", topic,
		"grafted", len(toGraft))

	return nil
}

// Leave 离开主题
func (r *Router) Leave(topic string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// 离开 mesh
	toPrune := r.mesh.Leave(topic)

	// 发送 PRUNE 消息
	for _, peer := range toPrune {
		pxPeers := r.mesh.GetPXPeers(topic, peer, 10)
		r.sendPruneAsync(peer, topic, pxPeers)
	}

	// 广播订阅变更
	r.broadcastSubscription(topic, false)

	log.Info("离开主题",
		"topic", topic,
		"pruned", len(toPrune))

	return nil
}

// Subscribe 订阅主题
func (r *Router) Subscribe(topic string) (<-chan *Message, func(), error) {
	// 先检查是否需要加入主题（不持有锁）
	r.mu.RLock()
	needJoin := !r.mesh.IsSubscribed(topic)
	r.mu.RUnlock()

	// 如果需要加入主题，在不持有锁的情况下调用 Join
	if needJoin {
		if err := r.Join(topic); err != nil {
			return nil, nil, err
		}
	}

	// 创建订阅
	r.mu.Lock()
	defer r.mu.Unlock()

	sub := &localSubscription{
		topic:    topic,
		messages: make(chan *Message, 100),
		active:   1,
	}

	r.subscriptions[topic] = append(r.subscriptions[topic], sub)

	cancel := func() {
		if atomic.CompareAndSwapInt32(&sub.active, 1, 0) {
			close(sub.messages)

			r.mu.Lock()
			subs := r.subscriptions[topic]
			for i, s := range subs {
				if s == sub {
					r.subscriptions[topic] = append(subs[:i], subs[i+1:]...)
					break
				}
			}
			r.mu.Unlock()
		}
	}

	return sub.messages, cancel, nil
}

// ============================================================================
//                              消息发布
// ============================================================================

// Publish 发布消息
func (r *Router) Publish(_ context.Context, topic string, data []byte) error {
	// 生成消息 ID
	msgID := r.generateMsgID(data)

	// 检查消息大小
	if len(data) > r.config.MaxMessageSize {
		return ErrMessageTooLarge
	}

	// 创建消息
	seqNo := atomic.AddUint64(&r.seqNo, 1)
	msg := &Message{
		ID:        msgID,
		Topic:     topic,
		From:      r.localID,
		Data:      data,
		Timestamp: time.Now(),
		Sequence:  seqNo,
	}

	// 如果配置要求签名消息，且有身份
	if r.config.SignMessages && r.identity != nil {
		pubKey := r.identity.PublicKey()
		msg.Key = pubKey.Bytes()
		msg.KeyType = pubKey.Type()

		// 构建待签名数据
		signData := r.buildSignData(msg)

		// 签名
		sig, err := r.identity.Sign(signData)
		if err != nil {
			log.Warn("消息签名失败", "err", err)
		} else {
			msg.Signature = sig
		}
	}

	// 标记为已见
	r.seenCache.Add(msgID)

	// 添加到缓存
	r.cache.Put(&CacheEntry{
		Message:      msg,
		ReceivedFrom: r.localID,
		ReceivedAt:   time.Now(),
		Validated:    true,
		Valid:        true,
	})

	// 本地分发
	r.deliverLocal(msg)

	// 获取转发目标
	var peers []types.NodeID
	if r.mesh.IsSubscribed(topic) {
		// 已订阅：发送给 mesh peers
		peers = r.mesh.MeshPeers(topic)
	} else {
		// 未订阅：发送给 fanout peers
		peers = r.mesh.FanoutPeers(topic)
	}

	// 洪泛发布模式
	if r.config.FloodPublish {
		peers = r.mesh.PeersInTopic(topic)
	}

	// 发送消息
	for _, peer := range peers {
		r.sendMessageAsync(peer, msg)
	}

	log.Debug("消息已发布",
		"topic", topic,
		"size", len(data),
		"peers", len(peers))

	return nil
}

// ============================================================================
//                              Peer 管理
// ============================================================================

// AddPeer 添加 peer
func (r *Router) AddPeer(peer types.NodeID, outbound bool) {
	r.mesh.AddPeer(peer, outbound)
	if r.scorer != nil {
		r.scorer.AddPeer(peer, "")
	}

	log.Debug("添加 peer",
		"peer", peer.String(),
		"outbound", outbound)

	// 新连接建立时主动发送订阅快照
	// 不依赖入站 RPC，确保对方能立即知道我们订阅的 topic
	// 这解决了 Seed 早于 Peer 加入 topic 时，Peer 永远不知道 Seed 订阅状态的问题
	r.maybeSendInitialSubscriptions(peer)
}

// RemovePeer 移除 peer
func (r *Router) RemovePeer(peer types.NodeID) {
	r.mesh.RemovePeer(peer)
	if r.scorer != nil {
		r.scorer.RemovePeer(peer)
	}

	// 清理初始订阅记录，允许重连后重新同步
	r.mu.Lock()
	delete(r.sentInitialSubs, peer)
	r.mu.Unlock()

	log.Debug("移除 peer",
		"peer", peer.String())
}

// PeersInTopic 返回订阅指定 topic 的所有已知 peers
func (r *Router) PeersInTopic(topic string) []types.NodeID {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mesh.PeersInTopic(topic)
}

// MeshPeers 返回指定 topic 的 mesh peers
func (r *Router) MeshPeers(topic string) []types.NodeID {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mesh.MeshPeers(topic)
}

// ============================================================================
//                              RPC 处理
// ============================================================================

// HandleRPC 处理收到的 RPC
func (r *Router) HandleRPC(from types.NodeID, rpc *RPC) error {
	// 处理订阅变更
	for _, sub := range rpc.Subscriptions {
		r.handleSubscription(from, sub)
	}

	// 处理数据消息
	for _, msg := range rpc.Messages {
		r.handleMessage(from, msg)
	}

	// 处理控制消息
	if rpc.Control != nil {
		r.handleControl(from, rpc.Control)
	}

	return nil
}

// handleSubscription 处理订阅变更
func (r *Router) handleSubscription(from types.NodeID, sub SubOpt) {
	if sub.Subscribe {
		r.mesh.AddPeerToTopic(from, sub.Topic)
	} else {
		r.mesh.RemovePeerFromTopic(from, sub.Topic)
	}

	log.Debug("收到订阅变更",
		"from", from.String(),
		"topic", sub.Topic,
		"subscribe", sub.Subscribe)
}

// parseRealmIDFromTopic 从 topic 解析 realmID。
//
// 当前 Realm topic 的格式由 Realm wrapper 生成：realm/{realmID}/{topic}
// 这里仅解析 realmID（即 realm/ 后到下一个 / 之间的部分）。
func parseRealmIDFromTopic(topic string) (string, bool) {
	const prefix = "realm/"
	if !strings.HasPrefix(topic, prefix) {
		return "", false
	}
	rest := topic[len(prefix):]
	if rest == "" {
		return "", false
	}
	for i := 0; i < len(rest); i++ {
		if rest[i] == '/' {
			// realm/{realmID}/...
			if i == 0 {
				return "", false
			}
			return rest[:i], true
		}
	}
	// realm/{realmID}
	return rest, true
}

// validateRealmTopicPeer 校验 realm topic 的消息是否来自该 realm 的已验证连接。
//
// design_as_truth：PubSub 必须在 GossipSub 管线层做成员验证，避免仅在 wrapper 层兜底。
// 规则：
// - 若 topic 不是 realm/ 前缀：不做校验（保持兼容）。
// - 若是 realm/ 前缀：要求 connection.RealmContext 有效且 realmID 匹配，否则丢弃。
func (r *Router) validateRealmTopicPeer(from types.NodeID, topic string) bool {
	realmID, ok := parseRealmIDFromTopic(topic)
	if !ok {
		return true
	}

	// 没有 endpoint 时无法获取连接上下文（通常仅发生在纯单元测试/未接入网络时）
	if r.endpoint == nil {
		return true
	}

	conn, exists := r.endpoint.Connection(from)
	if !exists || conn == nil {
		log.Warn("丢弃 realm topic 消息：无连接上下文",
			"from", from.String(),
			"topic", topic)
		return false
	}

	rc := conn.RealmContext()
	if rc == nil || !rc.IsValid() || rc.RealmID != realmID {
		log.Debug("丢弃非成员 realm topic 消息",
			"from", from.String(),
			"topic", topic,
			"expectedRealm", realmID,
			"verified", rc != nil && rc.IsValid(),
			"connRealm", func() string {
				if rc == nil {
					return ""
				}
				return rc.RealmID
			}())
		return false
	}

	return true
}

// handleMessage 处理数据消息
func (r *Router) handleMessage(from types.NodeID, msg *Message) {
	// 检查是否已见
	if r.seenCache.Has(msg.ID) {
		// 记录重复消息
		// 检查此 peer 是否是原始投递者
		wasFirst := false
		if entry, exists := r.cache.Get(msg.ID); exists {
			wasFirst = (entry.ReceivedFrom == from)
		}
		if r.scorer != nil {
			r.scorer.DuplicateMessage(from, msg.Topic, wasFirst)
		}
		return
	}

	// 🔒 强制隔离检查点（PubSub / GossipSub 管线）：
	// 对 realm/{realmID}/... 的消息进行连接级 RealmContext 校验。
	// 不通过则直接丢弃，且不进入 seenCache（防止非成员投毒 seenCache）。
	if !r.validateRealmTopicPeer(from, msg.Topic) {
		return
	}

	// 标记为已见
	r.seenCache.Add(msg.ID)

	// 验证消息
	isValid := r.validateMessage(msg)
	isFirst := true

	// 更新评分
	if r.scorer != nil {
		r.scorer.ValidateMessage(from, msg.Topic, isFirst, isValid)
	}

	if !isValid {
		log.Debug("消息验证失败",
			"from", from.String(),
			"topic", msg.Topic)
		return
	}

	// 添加到缓存
	r.cache.Put(&CacheEntry{
		Message:      msg,
		ReceivedFrom: from,
		ReceivedAt:   time.Now(),
		Validated:    true,
		Valid:        true,
	})

	// 履行 IWANT
	r.heartbeat.FulfillIWant(msg.ID)

	// 本地分发
	r.deliverLocal(msg)

	// 转发给 mesh peers（排除来源）
	if r.mesh.IsSubscribed(msg.Topic) {
		for _, peer := range r.mesh.MeshPeers(msg.Topic) {
			if peer != from {
				r.sendMessageAsync(peer, msg)
			}
		}
	}

	log.Debug("收到消息",
		"from", from.String(),
		"topic", msg.Topic,
		"size", len(msg.Data))
}

// handleControl 处理控制消息
func (r *Router) handleControl(from types.NodeID, ctrl *ControlMessage) {
	// 处理 IHAVE
	for _, ihave := range ctrl.IHave {
		r.handleIHave(from, &ihave)
	}

	// 处理 IWANT
	for _, iwant := range ctrl.IWant {
		r.handleIWant(from, &iwant)
	}

	// 处理 GRAFT
	for _, graft := range ctrl.Graft {
		r.handleGraft(from, &graft)
	}

	// 处理 PRUNE
	for _, prune := range ctrl.Prune {
		r.handlePrune(from, &prune)
	}
}

// handleIHave 处理 IHAVE 消息
func (r *Router) handleIHave(from types.NodeID, ihave *ControlIHaveMessage) {
	// 检查评分
	if r.scorer != nil && r.scorer.IsBelowGossipThreshold(from) {
		return
	}

	// 找出缺失的消息
	missing := make([][]byte, 0)
	for _, msgID := range ihave.MessageIDs {
		if !r.cache.Has(msgID) && !r.seenCache.Has(msgID) {
			missing = append(missing, msgID)
			// 追踪 IWANT
			r.heartbeat.TrackIWant(msgID, from)
		}
	}

	if len(missing) == 0 {
		return
	}

	// 限制 IWANT 大小
	if len(missing) > r.config.MaxIWantLength {
		missing = missing[:r.config.MaxIWantLength]
	}

	// 发送 IWANT
	r.sendIWantAsync(from, missing)

	log.Debug("处理 IHAVE",
		"from", from.String(),
		"topic", ihave.Topic,
		"ihave", len(ihave.MessageIDs),
		"iwant", len(missing))
}

// handleIWant 处理 IWANT 消息
func (r *Router) handleIWant(from types.NodeID, iwant *ControlIWantMessage) {
	// 从缓存中查找消息并发送
	for _, msgID := range iwant.MessageIDs {
		msg, exists := r.cache.GetMessage(msgID)
		if exists {
			r.sendMessageAsync(from, msg)
		}
	}

	log.Debug("处理 IWANT",
		"from", from.String(),
		"requested", len(iwant.MessageIDs))
}

// handleGraft 处理 GRAFT 消息
func (r *Router) handleGraft(from types.NodeID, graft *ControlGraftMessage) {
	topic := graft.Topic

	// 检查是否订阅该主题
	if !r.mesh.IsSubscribed(topic) {
		// 发送 PRUNE
		r.sendPruneAsync(from, topic, nil)
		return
	}

	// 检查评分
	if r.scorer != nil && r.scorer.IsBelowGraylistThreshold(from) {
		// 发送 PRUNE
		pxPeers := r.mesh.GetPXPeers(topic, from, 10)
		r.sendPruneAsync(from, topic, pxPeers)
		return
	}

	// 添加到 mesh
	if r.mesh.Graft(from, topic) {
		log.Debug("GRAFT 成功",
			"from", from.String(),
			"topic", topic)
	}
}

// handlePrune 处理 PRUNE 消息
func (r *Router) handlePrune(from types.NodeID, prune *ControlPruneMessage) {
	topic := prune.Topic

	// 从 mesh 移除
	backoff := time.Duration(prune.Backoff) * time.Second
	r.mesh.Prune(from, topic, backoff)

	// 处理 PX peers
	if len(prune.Peers) > 0 {
		toConnect := r.mesh.HandlePX(from, topic, prune.Peers)
		// 可以尝试连接这些 peers
		_ = toConnect
	}

	log.Debug("PRUNE",
		"from", from.String(),
		"topic", topic,
		"px", len(prune.Peers))
}

// ============================================================================
//                              流处理
// ============================================================================

// handleStream 处理入站流
func (r *Router) handleStream(stream endpoint.Stream) {
	defer func() { _ = stream.Close() }()

	conn := stream.Connection()
	if conn == nil {
		return
	}
	from := conn.RemoteID()

	// 设置读取超时，防止无限阻塞
	if err := stream.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
		log.Debug("设置读取超时失败", "from", from.String(), "err", err)
	}

	// 读取 RPC
	rpc, err := ReadRPC(stream)
	if err != nil {
		log.Debug("读取 RPC 失败",
			"from", from.String(),
			"err", err)
		return
	}

	// 处理 RPC
	if err := r.HandleRPC(from, rpc); err != nil {
		log.Debug("处理 RPC 失败",
			"from", from.String(),
			"err", err)
	}

	// 首次收到该 peer 的 RPC 时，发送本地订阅快照
	// 确保对方知道我们已订阅的 topic，从而能正确建立 mesh
	r.maybeSendInitialSubscriptions(from)
}

// ============================================================================
//                              消息发送
// ============================================================================

// sendRPC 发送 RPC 到 peer
func (r *Router) sendRPC(peer types.NodeID, rpc *RPC) error {
	if r.endpoint == nil {
		return nil
	}

	// 获取连接
	conn, exists := r.endpoint.Connection(peer)
	if !exists {
		return nil
	}

	// 打开流
	ctx, cancel := context.WithTimeout(r.ctx, 10*time.Second)
	defer cancel()

	stream, err := conn.OpenStream(ctx, ProtocolGossipSub)
	if err != nil {
		return err
	}

	// 设置写入超时，防止写阻塞导致 goroutine 泄漏
	if err := stream.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		log.Debug("设置写入超时失败", "peer", peer.String(), "err", err)
	}

	// 写入 RPC
	if err := WriteRPC(stream, rpc); err != nil {
		_ = stream.Close()
		return err
	}

	// 使用 CloseWrite 确保数据发送完成后发送 FIN
	// 这比 Close() 更安全，不会在数据发送完成前关闭流
	return stream.CloseWrite()
}

// sendMessageAsync 异步发送消息
func (r *Router) sendMessageAsync(peer types.NodeID, msg *Message) {
	go func() {
		rpc := &RPC{
			Messages: []*Message{msg},
		}
		if err := r.sendRPC(peer, rpc); err != nil {
			log.Debug("发送消息失败",
				"peer", peer.String(),
				"err", err)
		}
	}()
}

// sendGraftAsync 异步发送 GRAFT
func (r *Router) sendGraftAsync(peer types.NodeID, topic string) {
	go func() {
		rpc := &RPC{
			Control: &ControlMessage{
				Graft: []ControlGraftMessage{{Topic: topic}},
			},
		}
		if err := r.sendRPC(peer, rpc); err != nil {
			log.Debug("发送 GRAFT 失败", "peer", peer.String(), "topic", topic, "err", err)
		}
	}()
}

// sendPruneAsync 异步发送 PRUNE
func (r *Router) sendPruneAsync(peer types.NodeID, topic string, pxPeers []PeerInfo) {
	go func() {
		rpc := &RPC{
			Control: &ControlMessage{
				Prune: []ControlPruneMessage{{
					Topic:   topic,
					Peers:   pxPeers,
					Backoff: uint64(r.config.PruneBackoff.Seconds()),
				}},
			},
		}
		if err := r.sendRPC(peer, rpc); err != nil {
			log.Debug("发送 PRUNE 失败", "peer", peer.String(), "topic", topic, "err", err)
		}
	}()
}

// sendIWantAsync 异步发送 IWANT
func (r *Router) sendIWantAsync(peer types.NodeID, msgIDs [][]byte) {
	go func() {
		rpc := &RPC{
			Control: &ControlMessage{
				IWant: []ControlIWantMessage{{MessageIDs: msgIDs}},
			},
		}
		if err := r.sendRPC(peer, rpc); err != nil {
			log.Debug("发送 IWANT 失败", "peer", peer.String(), "err", err)
		}
	}()
}

// broadcastSubscription 广播订阅变更
func (r *Router) broadcastSubscription(topic string, subscribe bool) {
	if r.endpoint == nil {
		return
	}

	rpc := &RPC{
		Subscriptions: []SubOpt{{
			Subscribe: subscribe,
			Topic:     topic,
		}},
	}

	for _, conn := range r.endpoint.Connections() {
		peer := conn.RemoteID()
		go func(p types.NodeID) {
			if err := r.sendRPC(p, rpc); err != nil {
				log.Debug("广播订阅失败", "peer", p.String(), "topic", topic, "subscribe", subscribe, "err", err)
			}
		}(peer)
	}
}

// maybeSendInitialSubscriptions 首次与 peer 交互时发送本地订阅快照
// 确保每个 peer 只收到一次，避免重复/抖动
func (r *Router) maybeSendInitialSubscriptions(peer types.NodeID) {
	r.mu.Lock()
	if r.sentInitialSubs[peer] {
		r.mu.Unlock()
		return
	}
	r.sentInitialSubs[peer] = true
	r.mu.Unlock()

	// 获取本地已订阅的 topic 列表（使用 mesh.Topics() 作为数据源）
	topics := r.mesh.Topics()
	if len(topics) == 0 {
		return
	}

	// 构造订阅 RPC
	subs := make([]SubOpt, 0, len(topics))
	for _, topic := range topics {
		subs = append(subs, SubOpt{Subscribe: true, Topic: topic})
	}

	rpc := &RPC{Subscriptions: subs}

	// 异步发送
	go func() {
		if err := r.sendRPC(peer, rpc); err != nil {
			log.Debug("发送初始订阅快照失败", "peer", peer.String(), "err", err)
		} else {
			log.Debug("已发送初始订阅快照", "peer", peer.String(), "topics", len(topics))
		}
	}()
}

// ============================================================================
//                              本地分发
// ============================================================================

// deliverLocal 本地分发消息
func (r *Router) deliverLocal(msg *Message) {
	r.mu.RLock()
	subs := r.subscriptions[msg.Topic]
	r.mu.RUnlock()

	for _, sub := range subs {
		if atomic.LoadInt32(&sub.active) == 1 {
			select {
			case sub.messages <- msg:
			default:
				// 通道满，丢弃消息
				log.Warn("订阅通道已满，丢弃消息",
					"topic", msg.Topic)
			}
		}
	}
}

// ============================================================================
//                              辅助方法
// ============================================================================

// generateMsgID 生成消息 ID
func (r *Router) generateMsgID(data []byte) []byte {
	h := sha256.New()
	h.Write(r.localID[:])
	h.Write(data)

	seqNo := atomic.LoadUint64(&r.seqNo)
	seqBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		seqBytes[i] = byte(seqNo >> (8 * (7 - i)))
	}
	h.Write(seqBytes)

	return h.Sum(nil)[:20] // 20 字节的消息 ID
}

// validateMessage 验证消息
func (r *Router) validateMessage(msg *Message) bool {
	// 基本验证
	if len(msg.ID) == 0 || len(msg.Topic) == 0 {
		return false
	}

	// 验证 From 字段不为空（全零）
	if msg.From == (types.NodeID{}) {
		log.Debug("消息 From 字段无效（全零）",
			"topic", msg.Topic)
		return false
	}

	// 大小限制
	if len(msg.Data) > r.config.MaxMessageSize {
		return false
	}

	// 签名验证（当启用时）
	if r.config.ValidateMessages {
		if !r.verifyMessageSignature(msg) {
			log.Debug("消息签名验证失败",
				"topic", msg.Topic,
				"from", msg.From.String(),
			)
			return false
		}
	}

	return true
}

// verifyMessageSignature 验证消息签名
//
// 签名验证流程：
// 1. 检查消息是否携带公钥和签名
// 2. 推断或使用消息携带的密钥类型
// 3. 构建待签名数据：Topic + From + Sequence + Data
// 4. 根据密钥类型验证签名（支持 Ed25519、ECDSA-P256、ECDSA-P384）
// 5. 可选：验证公钥哈希是否与 From NodeID 匹配
func (r *Router) verifyMessageSignature(msg *Message) bool {
	// 检查是否有公钥
	if len(msg.Key) == 0 {
		log.Debug("消息缺少公钥，拒绝消息")
		return false
	}

	// 检查是否有签名
	if len(msg.Signature) == 0 {
		log.Debug("消息缺少签名，拒绝消息")
		return false
	}

	// 推断密钥类型（如果未显式指定）
	keyType := msg.KeyType
	if keyType == types.KeyTypeUnknown {
		keyType = r.inferKeyTypeFromPublicKey(msg.Key)
	}

	// 构建待签名数据
	signData := r.buildSignData(msg)

	// 根据密钥类型验证签名
	var valid bool
	switch keyType {
	case types.KeyTypeEd25519:
		valid = r.verifyEd25519Signature(msg.Key, signData, msg.Signature)
	case types.KeyTypeECDSAP256:
		valid = r.verifyECDSASignature(msg.Key, signData, msg.Signature, elliptic.P256())
	case types.KeyTypeECDSAP384:
		valid = r.verifyECDSASignature(msg.Key, signData, msg.Signature, elliptic.P384())
	default:
		log.Debug("不支持的密钥类型",
			"type", keyType.String(),
			"keyLen", len(msg.Key))
		return false
	}

	if !valid {
		log.Debug("签名验证失败",
			"keyType", keyType.String())
		return false
	}

	// 可选：验证公钥哈希与 From NodeID 匹配
	if r.config.StrictSignatureValidation {
		expectedNodeID := r.computeNodeIDFromKey(msg.Key)
		if expectedNodeID != msg.From {
			log.Debug("公钥与 From NodeID 不匹配",
				"expected", expectedNodeID.String(),
				"actual", msg.From.String())
			return false
		}
	}

	return true
}

// inferKeyTypeFromPublicKey 根据公钥长度推断密钥类型
func (r *Router) inferKeyTypeFromPublicKey(key []byte) types.KeyType {
	switch len(key) {
	case ed25519.PublicKeySize: // 32 字节
		return types.KeyTypeEd25519
	case 65: // P-256 未压缩格式
		return types.KeyTypeECDSAP256
	case 97: // P-384 未压缩格式
		return types.KeyTypeECDSAP384
	default:
		return types.KeyTypeUnknown
	}
}

// verifyEd25519Signature 验证 Ed25519 签名
func (r *Router) verifyEd25519Signature(pubKeyBytes, data, sig []byte) bool {
	if len(pubKeyBytes) != ed25519.PublicKeySize {
		return false
	}
	if len(sig) != ed25519.SignatureSize {
		return false
	}
	pubKey := ed25519.PublicKey(pubKeyBytes)
	return ed25519.Verify(pubKey, data, sig)
}

// verifyECDSASignature 验证 ECDSA 签名
func (r *Router) verifyECDSASignature(pubKeyBytes, data, sig []byte, curve elliptic.Curve) bool {
	// 解析公钥
	x, y := elliptic.Unmarshal(curve, pubKeyBytes)
	if x == nil {
		log.Debug("无法解析 ECDSA 公钥",
			"keyLen", len(pubKeyBytes))
		return false
	}

	pubKey := &ecdsa.PublicKey{
		Curve: curve,
		X:     x,
		Y:     y,
	}

	// 根据曲线选择哈希算法
	var hash []byte
	switch curve {
	case elliptic.P384():
		h := sha512.Sum384(data)
		hash = h[:]
	default: // P-256
		h := sha256.Sum256(data)
		hash = h[:]
	}

	// 解析签名（支持 r||s 格式和 ASN.1 DER 格式）
	rVal, sVal, err := parseECDSASignatureForVerify(sig, curve)
	if err != nil {
		log.Debug("无法解析 ECDSA 签名",
			"sigLen", len(sig),
			"err", err)
		return false
	}

	return ecdsa.Verify(pubKey, hash, rVal, sVal)
}

// parseECDSASignatureForVerify 解析 ECDSA 签名
// 支持两种格式：
// 1. r||s 格式（固定长度，r 和 s 各占一半）
// 2. ASN.1 DER 格式
func parseECDSASignatureForVerify(sig []byte, curve elliptic.Curve) (*big.Int, *big.Int, error) {
	byteLen := (curve.Params().BitSize + 7) / 8

	// 尝试 r||s 格式
	if len(sig) == byteLen*2 {
		r := new(big.Int).SetBytes(sig[:byteLen])
		s := new(big.Int).SetBytes(sig[byteLen:])
		return r, s, nil
	}

	// 尝试 ASN.1 DER 格式
	if len(sig) > 2 && sig[0] == 0x30 {
		// 简化的 ASN.1 DER 解析
		// 格式: 0x30 len 0x02 rLen r 0x02 sLen s
		if len(sig) < 8 {
			return nil, nil, errInvalidSignature
		}

		idx := 2 // 跳过 SEQUENCE tag 和长度
		if sig[idx] != 0x02 {
			return nil, nil, errInvalidSignature
		}
		idx++
		rLen := int(sig[idx])
		idx++
		if idx+rLen >= len(sig) {
			return nil, nil, errInvalidSignature
		}
		rBytes := sig[idx : idx+rLen]
		idx += rLen

		if sig[idx] != 0x02 {
			return nil, nil, errInvalidSignature
		}
		idx++
		sLen := int(sig[idx])
		idx++
		if idx+sLen > len(sig) {
			return nil, nil, errInvalidSignature
		}
		sBytes := sig[idx : idx+sLen]

		r := new(big.Int).SetBytes(rBytes)
		s := new(big.Int).SetBytes(sBytes)
		return r, s, nil
	}

	return nil, nil, errInvalidSignature
}

// errInvalidSignature 无效签名错误
var errInvalidSignature = &signatureError{"invalid ECDSA signature format"}

type signatureError struct {
	msg string
}

func (e *signatureError) Error() string {
	return e.msg
}

// buildSignData 构建待签名数据
func (r *Router) buildSignData(msg *Message) []byte {
	// 计算所需长度
	topicBytes := []byte(msg.Topic)
	fromBytes := msg.From[:]
	seqBytes := make([]byte, 8)
	for i := 0; i < 8; i++ {
		seqBytes[i] = byte(msg.Sequence >> (8 * (7 - i)))
	}

	// 拼接数据
	result := make([]byte, 0, len(topicBytes)+len(fromBytes)+8+len(msg.Data))
	result = append(result, topicBytes...)
	result = append(result, fromBytes...)
	result = append(result, seqBytes...)
	result = append(result, msg.Data...)

	return result
}

// computeNodeIDFromKey 从公钥计算 NodeID
func (r *Router) computeNodeIDFromKey(key []byte) types.NodeID {
	hash := sha256.Sum256(key)
	var nodeID types.NodeID
	copy(nodeID[:], hash[:])
	return nodeID
}

// GetStats 获取统计信息
func (r *Router) GetStats() *Stats {
	stats := r.mesh.GetStats()

	r.mu.RLock()
	for topic, subs := range r.subscriptions {
		if ts, exists := stats.TopicStats[topic]; exists {
			ts.MessagesPublished = uint64(len(subs))
		}
	}
	r.mu.RUnlock()

	stats.TotalMessagesReceived = uint64(r.cache.Size())
	stats.TotalDuplicates = uint64(r.seenCache.Size())

	return stats
}

