// Package messaging 提供消息服务模块的实现
package messaging

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/messaging/gossipsub"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	messagingif "github.com/dep2p/go-dep2p/pkg/interfaces/messaging"
	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              协议 ID
// ============================================================================

// 引用 pkg/protocolids 唯一真源
var (
	// ProtocolRequest 请求响应协议 (v1.1 scope: app)
	ProtocolRequest = protocolids.AppMessagingRequest

	// ProtocolNotify 通知协议 (v1.1 scope: app)
	ProtocolNotify = protocolids.AppMessagingNotify

	// ProtocolPubsub 发布订阅协议 (v1.1 scope: app)
	ProtocolPubsub = protocolids.AppMessagingPubsub

	// ProtocolQuery 查询协议 (v1.1 scope: app)
	ProtocolQuery = protocolids.AppMessagingQuery
)

// ============================================================================
//                              待处理请求
// ============================================================================

// pendingRequest 待处理的请求
type pendingRequest struct {
	id       uint64
	respCh   chan *types.Response
	deadline time.Time
}

// ============================================================================
//                              MessagingService 实现
// ============================================================================

// MessagingService 消息服务实现
type MessagingService struct {
	config   messagingif.Config
	endpoint endpoint.Endpoint
	identity identityif.Identity

	// GossipSub 路由器
	gossipRouter *gossipsub.Router

	// 请求 ID 计数器
	requestID uint64

	// 待处理请求
	pendingRequests map[uint64]*pendingRequest
	pendingMu       sync.RWMutex

	// 处理器
	requestHandlers map[types.ProtocolID]messagingif.RequestHandler
	notifyHandlers  map[types.ProtocolID]messagingif.NotifyHandler
	queryHandlers   map[string]messagingif.QueryHandler
	handlerMu       sync.RWMutex

	// 查询响应收集器（用于 PublishQuery）
	queryResponseHandlers   map[string]*queryResponseCollector
	queryResponseHandlersMu sync.RWMutex

	// 订阅管理
	subscriptions map[string][]*subscription
	subMu         sync.RWMutex

	// 消息去重缓存
	seenMessages map[string]time.Time
	seenMu       sync.RWMutex

	// 状态
	running int32
	closed  int32
	stopCh  chan struct{}
}

// subscription 订阅
type subscription struct {
	id       string
	topic    string
	messages chan *types.Message
	active   int32
}

// NewMessagingService 创建消息服务
func NewMessagingService(config messagingif.Config, endpoint endpoint.Endpoint, identity identityif.Identity) *MessagingService {
	svc := &MessagingService{
		config:                config,
		endpoint:              endpoint,
		identity:              identity,
		pendingRequests:       make(map[uint64]*pendingRequest),
		requestHandlers:       make(map[types.ProtocolID]messagingif.RequestHandler),
		notifyHandlers:        make(map[types.ProtocolID]messagingif.NotifyHandler),
		queryHandlers:         make(map[string]messagingif.QueryHandler),
		queryResponseHandlers: make(map[string]*queryResponseCollector),
		subscriptions:         make(map[string][]*subscription),
		seenMessages:          make(map[string]time.Time),
		stopCh:                make(chan struct{}),
	}

	// 创建 GossipSub 路由器（如果不是洪泛模式）
	if !config.FloodPublish && endpoint != nil {
		gossipConfig := gossipsub.DefaultConfig()
		gossipConfig.FloodPublish = config.FloodPublish
		gossipConfig.HeartbeatInterval = config.HeartbeatInterval
		gossipConfig.HistoryLength = config.HistoryLength
		gossipConfig.HistoryGossip = config.HistoryGossip

		svc.gossipRouter = gossipsub.NewRouter(
			gossipConfig,
			endpoint.ID(),
			identity,
			endpoint,
		)
	}

	return svc
}

// 确保实现接口
var _ messagingif.MessagingService = (*MessagingService)(nil)

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动服务
func (s *MessagingService) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		return nil // 已经运行
	}

	log.Info("消息服务启动中")

	// 启动 GossipSub 路由器
	if s.gossipRouter != nil {
		if err := s.gossipRouter.Start(ctx); err != nil {
			log.Error("GossipSub 路由器启动失败", "err", err)
			return err
		}
		log.Info("GossipSub 路由器已启动")
	}

	// 注册协议处理器
	if s.endpoint != nil {
		s.endpoint.SetProtocolHandler(ProtocolRequest, s.handleRequestStream)
		s.endpoint.SetProtocolHandler(ProtocolNotify, s.handleNotifyStream)
		// 只有在洪泛模式下才注册简单的 pubsub 处理器
		if s.gossipRouter == nil {
			s.endpoint.SetProtocolHandler(ProtocolPubsub, s.handlePubsubStream)
		}
		s.endpoint.SetProtocolHandler(ProtocolQuery, s.handleQueryStream)
		// 注册查询响应协议处理器（用于 PublishQuery）
		s.endpoint.SetProtocolHandler(ProtocolQueryResponse, s.handleQueryResponseStream)

		// 注册连接回调，通知 GossipSub 新 peer 连接
		// 这是确保 GossipSub 能建立 mesh 的关键
		if s.gossipRouter != nil {
			s.endpoint.RegisterConnectionCallback(func(nodeID endpoint.NodeID, outbound bool) {
				s.gossipRouter.AddPeer(nodeID, outbound)
			})

			// 同步当前已有的连接（可能在 messaging service 启动前已建立）
			for _, conn := range s.endpoint.Connections() {
				s.gossipRouter.AddPeer(conn.RemoteID(), true) // 假设是出站连接
			}
		}
	}

	// 启动清理协程
	go s.cleanupLoop()

	log.Info("消息服务已启动")
	return nil
}

// Stop 停止服务
func (s *MessagingService) Stop() error {
	if !atomic.CompareAndSwapInt32(&s.closed, 0, 1) {
		return nil // 已经关闭
	}

	log.Info("消息服务停止中")

	// 发送停止信号
	close(s.stopCh)

	// 停止 GossipSub 路由器
	if s.gossipRouter != nil {
		_ = s.gossipRouter.Stop() // 停止时错误可忽略
	}

	// 移除协议处理器
	if s.endpoint != nil {
		s.endpoint.RemoveProtocolHandler(ProtocolRequest)
		s.endpoint.RemoveProtocolHandler(ProtocolNotify)
		if s.gossipRouter == nil {
			s.endpoint.RemoveProtocolHandler(ProtocolPubsub)
		}
		s.endpoint.RemoveProtocolHandler(ProtocolQuery)
		s.endpoint.RemoveProtocolHandler(ProtocolQueryResponse)
	}

	// 取消所有待处理请求
	s.pendingMu.Lock()
	for _, req := range s.pendingRequests {
		close(req.respCh)
	}
	s.pendingRequests = make(map[uint64]*pendingRequest)
	s.pendingMu.Unlock()

	// 取消所有订阅
	s.subMu.Lock()
	for _, subs := range s.subscriptions {
		for _, sub := range subs {
			if atomic.CompareAndSwapInt32(&sub.active, 1, 0) {
				close(sub.messages)
			}
		}
	}
	s.subscriptions = make(map[string][]*subscription)
	s.subMu.Unlock()

	atomic.StoreInt32(&s.running, 0)
	log.Info("消息服务已停止")
	return nil
}

// ============================================================================
//                              去重
// ============================================================================

// hasSeen 检查消息是否已见
func (s *MessagingService) hasSeen(msgID []byte) bool {
	key := msgIDToKey(msgID)
	s.seenMu.RLock()
	_, seen := s.seenMessages[key]
	s.seenMu.RUnlock()
	return seen
}

// markSeen 标记消息为已见
func (s *MessagingService) markSeen(msgID []byte) {
	key := msgIDToKey(msgID)
	s.seenMu.Lock()
	s.seenMessages[key] = time.Now()
	s.seenMu.Unlock()
}

// MinCleanupInterval 最小清理间隔
const MinCleanupInterval = time.Second

// cleanupLoop 清理协程
func (s *MessagingService) cleanupLoop() {
	// 计算清理间隔，确保不会因为 TTL 过小而导致问题
	interval := s.config.DeDuplicateTTL / 2
	if interval < MinCleanupInterval {
		interval = MinCleanupInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.cleanupSeenMessages()
		}
	}
}

// cleanupSeenMessages 清理过期的已见消息
func (s *MessagingService) cleanupSeenMessages() {
	cutoff := time.Now().Add(-s.config.DeDuplicateTTL)

	s.seenMu.Lock()
	for key, seen := range s.seenMessages {
		if seen.Before(cutoff) {
			delete(s.seenMessages, key)
		}
	}
	s.seenMu.Unlock()
}

// ============================================================================
//                              Topic 查询
// ============================================================================

// TopicPeers 获取订阅指定 topic 的所有 peers
func (s *MessagingService) TopicPeers(topic string) []types.NodeID {
	if s.gossipRouter == nil {
		return nil
	}
	return s.gossipRouter.PeersInTopic(topic)
}

// MeshPeers 获取指定 topic 的 mesh peers
func (s *MessagingService) MeshPeers(topic string) []types.NodeID {
	if s.gossipRouter == nil {
		return nil
	}
	return s.gossipRouter.MeshPeers(topic)
}

