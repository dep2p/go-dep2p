// Package realm 提供 Realm 管理实现
package realm

import (
	"context"
	"fmt"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/relay"
	"github.com/dep2p/go-dep2p/internal/core/relay/server"
	endpointif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	messagingif "github.com/dep2p/go-dep2p/pkg/interfaces/messaging"
	realmif "github.com/dep2p/go-dep2p/pkg/interfaces/realm"
	relayif "github.com/dep2p/go-dep2p/pkg/interfaces/relay"
	"github.com/dep2p/go-dep2p/pkg/protocolids"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              服务适配器工厂函数
// ============================================================================

// newRealmMessaging 创建 Realm 消息服务适配器
func newRealmMessaging(r *realmImpl) realmif.Messaging {
	return &realmMessaging{
		realm:              r,
		defaultProtocol:    "messaging/1.0.0",
		registeredHandlers: make(map[string]types.ProtocolID),
	}
}

// newRealmPubSub 创建 Realm 发布订阅服务适配器
func newRealmPubSub(r *realmImpl) realmif.PubSub {
	return &realmPubSub{
		realm:  r,
		topics: make(map[string]*realmTopic),
	}
}

// newRealmDiscovery 创建 Realm 发现服务适配器
func newRealmDiscovery(r *realmImpl) realmif.RealmDiscoveryService {
	return &realmDiscovery{
		realm: r,
	}
}

// newRealmStreams 创建 Realm 流管理服务适配器
func newRealmStreams(r *realmImpl) realmif.StreamManager {
	return &realmStreams{
		realm:    r,
		handlers: make(map[string]types.ProtocolID),
	}
}

// newRealmRelay 创建 Realm 中继服务适配器
func newRealmRelay(r *realmImpl) realmif.RealmRelayService {
	return &realmRelay{
		realm: r,
	}
}

// ============================================================================
//                              Messaging 适配器（IMPL-1227 Phase 4）
// ============================================================================

// realmMessaging Realm 消息服务适配器
//
// 封装底层 MessagingService，自动添加 Realm 协议前缀。
type realmMessaging struct {
	realm              *realmImpl
	defaultProtocol    string
	registeredHandlers map[string]types.ProtocolID
}

// Send 发送消息（使用默认协议）
func (m *realmMessaging) Send(ctx context.Context, to types.NodeID, data []byte) error {
	return m.SendWithProtocol(ctx, to, m.defaultProtocol, data)
}

// SendWithProtocol 发送消息（指定协议，框架自动添加 Realm 前缀）
func (m *realmMessaging) SendWithProtocol(ctx context.Context, to types.NodeID, protocol string, data []byte) error {
	// 验证用户协议
	if err := protocolids.ValidateUserProtocol(protocol, m.realm.ID()); err != nil {
		return fmt.Errorf("invalid protocol: %w", err)
	}

	// 自动添加 Realm 前缀
	fullProto := protocolids.FullAppProtocol(m.realm.ID(), protocol)

	// 调用底层消息服务
	if m.realm.messagingSvc == nil {
		return fmt.Errorf("messaging service not available")
	}
	return m.realm.messagingSvc.Send(ctx, to, fullProto, data)
}

// Request 发送请求并等待响应（使用默认协议）
func (m *realmMessaging) Request(ctx context.Context, to types.NodeID, data []byte) ([]byte, error) {
	return m.RequestWithProtocol(ctx, to, m.defaultProtocol, data)
}

// RequestWithProtocol 发送请求并等待响应（指定协议，框架自动添加 Realm 前缀）
func (m *realmMessaging) RequestWithProtocol(ctx context.Context, to types.NodeID, protocol string, data []byte) ([]byte, error) {
	// 验证用户协议
	if err := protocolids.ValidateUserProtocol(protocol, m.realm.ID()); err != nil {
		return nil, fmt.Errorf("invalid protocol: %w", err)
	}

	// 自动添加 Realm 前缀
	fullProto := protocolids.FullAppProtocol(m.realm.ID(), protocol)

	// 调用底层消息服务
	if m.realm.messagingSvc == nil {
		return nil, fmt.Errorf("messaging service not available")
	}
	return m.realm.messagingSvc.Request(ctx, to, fullProto, data)
}

// OnMessage 注册默认消息处理器
func (m *realmMessaging) OnMessage(handler realmif.MessageHandler) {
	m.OnProtocol(m.defaultProtocol, func(from types.NodeID, protocol string, data []byte) ([]byte, error) {
		handler(from, data)
		return nil, nil
	})
}

// OnRequest 注册默认请求处理器
func (m *realmMessaging) OnRequest(handler realmif.RequestHandler) {
	m.OnProtocol(m.defaultProtocol, func(from types.NodeID, protocol string, data []byte) ([]byte, error) {
		return handler(from, data)
	})
}

// OnProtocol 注册自定义协议处理器
func (m *realmMessaging) OnProtocol(protocol string, handler realmif.ProtocolHandler) {
	// 验证用户协议
	if err := protocolids.ValidateUserProtocol(protocol, m.realm.ID()); err != nil {
		log.Warn("OnProtocol: 无效的协议名称",
			"realm", m.realm.ID(),
			"protocol", protocol,
			"err", err)
		return
	}

	// 自动添加 Realm 前缀
	fullProto := protocolids.FullAppProtocol(m.realm.ID(), protocol)
	m.registeredHandlers[protocol] = fullProto

	// 注册到底层服务
	if m.realm.messagingSvc == nil {
		log.Warn("OnProtocol: 消息服务不可用",
			"realm", m.realm.ID(),
			"protocol", protocol)
		return
	}

	// 包装处理器：底层回调时调用用户处理器
	// 注意：NotifyHandler 签名是 func(data []byte, from types.NodeID)
	m.realm.messagingSvc.SetNotifyHandler(fullProto, func(data []byte, from types.NodeID) {
		// 调用用户处理器（忽略返回值，因为是通知模式）
		_, _ = handler(from, protocol, data)
	})

	// 同时注册请求处理器（支持请求/响应模式）
	// 注意：RequestHandler 签名是 func(req *Request) *Response
	m.realm.messagingSvc.SetRequestHandler(fullProto, func(req *types.Request) *types.Response {
		resp, err := handler(req.From, protocol, req.Data)
		if err != nil {
			return &types.Response{
				Status: 1,
				Error:  err.Error(),
			}
		}
		return &types.Response{
			Status: 0,
			Data:   resp,
		}
	})
}

// ============================================================================
//                              PubSub 适配器（IMPL-1227 Phase 4）
// ============================================================================

// realmPubSub Realm 发布订阅服务适配器
//
// 封装底层 MessagingService 的 PubSub 功能，自动添加 Realm topic 前缀。
type realmPubSub struct {
	realm  *realmImpl
	topics map[string]*realmTopic
}

// Join 加入主题（自动添加 Realm 前缀）
func (p *realmPubSub) Join(ctx context.Context, topicName string) (realmif.Topic, error) {
	// 验证 topic 名称
	if err := protocolids.ValidateUserProtocol(topicName, p.realm.ID()); err != nil {
		return nil, fmt.Errorf("invalid topic: %w", err)
	}

	// 自动添加 Realm 前缀
	// 用户写 "blocks"，实际 topic: "/dep2p/app/<realmID>/blocks"
	fullTopic := string(protocolids.FullAppProtocol(p.realm.ID(), topicName))

	// 检查是否已加入
	if existing, ok := p.topics[topicName]; ok {
		return existing, nil
	}

	// 调用底层 PubSub 服务订阅
	if p.realm.messagingSvc == nil {
		return nil, fmt.Errorf("messaging service not available")
	}

	baseSub, err := p.realm.messagingSvc.Subscribe(ctx, fullTopic)
	if err != nil {
		return nil, fmt.Errorf("subscribe to topic: %w", err)
	}

	topic := &realmTopic{
		realm:    p.realm,
		name:     topicName,
		fullName: fullTopic,
		baseSub:  baseSub,
	}
	p.topics[topicName] = topic

	return topic, nil
}

// Topics 返回已加入的主题列表
func (p *realmPubSub) Topics() []realmif.Topic {
	topics := make([]realmif.Topic, 0, len(p.topics))
	for _, t := range p.topics {
		topics = append(topics, t)
	}
	return topics
}

// realmTopic Realm 主题实现
type realmTopic struct {
	realm    *realmImpl
	name     string
	fullName string
	baseSub  messagingif.Subscription // 底层订阅句柄
	left     bool
}

func (t *realmTopic) Name() string     { return t.name }
func (t *realmTopic) FullName() string { return t.fullName }

// Publish 发布消息到主题
func (t *realmTopic) Publish(ctx context.Context, data []byte) error {
	if t.left {
		return fmt.Errorf("topic already left")
	}
	if t.realm.messagingSvc == nil {
		return fmt.Errorf("messaging service not available")
	}
	return t.realm.messagingSvc.Publish(ctx, t.fullName, data)
}

// Subscribe 订阅主题消息
func (t *realmTopic) Subscribe() (realmif.Subscription, error) {
	if t.left {
		return nil, fmt.Errorf("topic already left")
	}
	if t.baseSub == nil {
		return nil, fmt.Errorf("not subscribed to topic")
	}

	// 包装底层订阅
	return &realmPubSubSubscription{
		baseSub:  t.baseSub,
		topicName: t.name,
		msgCh:    make(chan *realmif.PubSubMessage, 100),
	}, nil
}

// Peers 返回订阅此主题的节点列表
//
// 返回通过 GossipSub 协议发现的、订阅该 topic 的所有已知 peers。
// 这是本节点视角的已知订阅者，与 libp2p ListPeers 语义一致。
func (t *realmTopic) Peers() []types.NodeID {
	if t.left {
		return nil
	}
	if t.realm.messagingSvc == nil {
		log.Warn("Topic.Peers: 消息服务不可用", "topic", t.name)
		return nil
	}
	return t.realm.messagingSvc.TopicPeers(t.fullName)
}

// Leave 离开主题
func (t *realmTopic) Leave() error {
	if t.left {
		return nil
	}
	t.left = true
	if t.baseSub != nil {
		t.baseSub.Cancel()
	}
	return nil
}

// realmPubSubSubscription PubSub 订阅实现
type realmPubSubSubscription struct {
	baseSub   messagingif.Subscription
	topicName string
	msgCh     chan *realmif.PubSubMessage
	started   bool
}

// Messages 返回消息通道
func (s *realmPubSubSubscription) Messages() <-chan *realmif.PubSubMessage {
	// 懒启动消息转发协程
	if !s.started {
		s.started = true
		go s.forwardMessages()
	}
	return s.msgCh
}

// forwardMessages 将底层消息转发到 Realm 消息通道
func (s *realmPubSubSubscription) forwardMessages() {
	if s.baseSub == nil {
		return
	}

	baseCh := s.baseSub.Messages()
	for msg := range baseCh {
		// 转换消息格式
		pubsubMsg := &realmif.PubSubMessage{
			From:       msg.From,
			Data:       msg.Data,
			Topic:      s.topicName, // 返回用户看到的 topic 名称（不含前缀）
			ReceivedAt: time.Now(),
		}
		select {
		case s.msgCh <- pubsubMsg:
		default:
			// 通道满，丢弃消息
		}
	}
	close(s.msgCh)
}

// Cancel 取消订阅
func (s *realmPubSubSubscription) Cancel() {
	if s.baseSub != nil {
		s.baseSub.Cancel()
	}
}

// ============================================================================
//                              Discovery 适配器
// ============================================================================

// realmDiscovery Realm 发现服务适配器
//
// 封装底层 DiscoveryService，自动添加 Realm 命名空间前缀。
type realmDiscovery struct {
	realm *realmImpl
}

// realmNamespace 生成 Realm 级别的命名空间
func (d *realmDiscovery) realmNamespace(service string) string {
	if service == "" {
		return fmt.Sprintf("realm/%s", d.realm.ID())
	}
	return fmt.Sprintf("realm/%s/%s", d.realm.ID(), service)
}

func (d *realmDiscovery) FindPeers(ctx context.Context, opts ...realmif.FindOption) ([]types.NodeID, error) {
	// 应用选项
	options := &realmif.FindOptions{
		Limit:   100,
		Timeout: 30 * time.Second,
	}
	for _, opt := range opts {
		opt(options)
	}

	// 如果有超时，设置上下文
	if options.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, options.Timeout)
		defer cancel()
	}

	// 获取底层发现服务
	discovery := d.realm.manager.discovery
	if discovery == nil {
		// 回退到本地已知成员
		return d.realm.Members(), nil
	}

	// 通过 DHT 发现同 Realm 的节点
	namespace := d.realmNamespace("")
	ch, err := discovery.DiscoverPeers(ctx, namespace)
	if err != nil {
		// 发现失败时回退到本地已知成员
		return d.realm.Members(), nil
	}

	// 收集结果
	peers := make([]types.NodeID, 0, options.Limit)
	for info := range ch {
		peers = append(peers, info.ID)
		if options.Limit > 0 && len(peers) >= options.Limit {
			break
		}
	}

	// 合并本地已知成员（去重）
	known := make(map[types.NodeID]struct{}, len(peers))
	for _, p := range peers {
		known[p] = struct{}{}
	}
	for _, m := range d.realm.Members() {
		if _, ok := known[m]; !ok {
			peers = append(peers, m)
			if options.Limit > 0 && len(peers) >= options.Limit {
				break
			}
		}
	}

	return peers, nil
}

func (d *realmDiscovery) FindPeersWithService(ctx context.Context, service string) ([]types.NodeID, error) {
	discovery := d.realm.manager.discovery
	if discovery == nil {
		log.Debug("FindPeersWithService: 发现服务不可用，返回空结果",
			"realm", d.realm.ID(),
			"service", service)
		return []types.NodeID{}, nil // 返回空切片而非 nil
	}

	// 使用带服务名的 Realm 命名空间
	namespace := d.realmNamespace(service)
	ch, err := discovery.DiscoverPeers(ctx, namespace)
	if err != nil {
		return nil, err
	}

	// 收集结果
	var peers []types.NodeID
	for info := range ch {
		peers = append(peers, info.ID)
	}

	return peers, nil
}

func (d *realmDiscovery) Advertise(ctx context.Context, service string) error {
	discovery := d.realm.manager.discovery
	if discovery == nil {
		log.Warn("Advertise: 发现服务不可用",
			"realm", d.realm.ID(),
			"service", service)
		return ErrDiscoveryNotAvailable
	}

	// 使用带服务名的 Realm 命名空间注册
	namespace := d.realmNamespace(service)
	return discovery.Announce(ctx, namespace)
}

func (d *realmDiscovery) StopAdvertise(service string) error {
	discovery := d.realm.manager.discovery
	if discovery == nil {
		log.Debug("StopAdvertise: 发现服务不可用，跳过",
			"realm", d.realm.ID(),
			"service", service)
		return nil // 停止操作可以静默成功
	}

	// 停止注册
	namespace := d.realmNamespace(service)
	discovery.StopAnnounce(namespace)
	return nil
}

func (d *realmDiscovery) Watch(ctx context.Context) (<-chan realmif.MemberEvent, error) {
	discovery := d.realm.manager.discovery
	if discovery == nil {
		log.Warn("Watch: 发现服务不可用",
			"realm", d.realm.ID())
		return nil, ErrDiscoveryNotAvailable
	}

	ch := make(chan realmif.MemberEvent, 100)

	// 启动后台协程监听发现事件
	go func() {
		defer close(ch)

		namespace := d.realmNamespace("")
		peerCh, err := discovery.DiscoverPeers(ctx, namespace)
		if err != nil {
			log.Debug("Watch: 发现失败", "err", err)
			return
		}

		for {
			select {
			case <-ctx.Done():
				return
			case info, ok := <-peerCh:
				if !ok {
					return
				}
				// 转换为成员事件
				event := realmif.MemberEvent{
					Type:   realmif.MemberJoined,
					NodeID: info.ID,
				}
				select {
				case ch <- event:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ch, nil
}

// ============================================================================
//                              Streams 适配器
// ============================================================================

// realmStreams Realm 流管理服务适配器
//
// 封装底层 Endpoint 的流功能，自动添加 Realm 协议前缀。
type realmStreams struct {
	realm    *realmImpl
	handlers map[string]types.ProtocolID
}

func (s *realmStreams) Open(ctx context.Context, to types.NodeID, protocol string) (realmif.Stream, error) {
	if err := protocolids.ValidateUserProtocol(protocol, s.realm.ID()); err != nil {
		return nil, fmt.Errorf("invalid protocol: %w", err)
	}

	// 获取 Endpoint
	ep := s.realm.manager.endpoint
	if ep == nil {
		return nil, fmt.Errorf("endpoint not available")
	}

	// 获取或建立连接
	conn, ok := ep.Connection(to)
	if !ok {
		// 尝试建立连接
		var err error
		conn, err = ep.Connect(ctx, to)
		if err != nil {
			return nil, fmt.Errorf("connect to peer: %w", err)
		}
	}

	// 添加 Realm 前缀
	fullProto := protocolids.FullAppProtocol(s.realm.ID(), protocol)

	// 打开流
	stream, err := conn.OpenStream(ctx, fullProto)
	if err != nil {
		return nil, fmt.Errorf("open stream: %w", err)
	}

	// 返回包装后的流
	return &realmStreamWrapper{
		Stream:   stream,
		protocol: fullProto,
		remotePeer: to,
	}, nil
}

func (s *realmStreams) SetHandler(protocol string, handler realmif.StreamHandler) {
	if err := protocolids.ValidateUserProtocol(protocol, s.realm.ID()); err != nil {
		log.Warn("SetHandler: 无效的协议名称",
			"realm", s.realm.ID(),
			"protocol", protocol,
			"err", err)
		return
	}

	ep := s.realm.manager.endpoint
	if ep == nil {
		log.Warn("SetHandler: Endpoint 不可用",
			"realm", s.realm.ID(),
			"protocol", protocol)
		return
	}

	fullProto := protocolids.FullAppProtocol(s.realm.ID(), protocol)
	s.handlers[protocol] = fullProto

	// 注册到 Endpoint
	ep.SetProtocolHandler(fullProto, func(stream endpointif.Stream) {
		// 包装流并调用用户处理器
		wrapped := &realmStreamWrapper{
			Stream:     stream,
			protocol:   fullProto,
			remotePeer: stream.Connection().RemoteID(),
		}
		handler(wrapped)
	})

	log.Debug("SetHandler: 已注册协议处理器",
		"realm", s.realm.ID(),
		"protocol", protocol,
		"fullProto", fullProto)
}

func (s *realmStreams) RemoveHandler(protocol string) {
	fullProto, ok := s.handlers[protocol]
	if !ok {
		log.Debug("RemoveHandler: 处理器不存在",
			"realm", s.realm.ID(),
			"protocol", protocol)
		return
	}
	delete(s.handlers, protocol)

	ep := s.realm.manager.endpoint
	if ep == nil {
		log.Debug("RemoveHandler: Endpoint 不可用，跳过",
			"realm", s.realm.ID(),
			"protocol", protocol)
		return
	}

	ep.RemoveProtocolHandler(fullProto)
	log.Debug("RemoveHandler: 已移除协议处理器",
		"realm", s.realm.ID(),
		"protocol", protocol)
}

// realmStreamWrapper 包装 endpoint.Stream 为 realmif.Stream
type realmStreamWrapper struct {
	endpointif.Stream
	protocol   types.ProtocolID
	remotePeer types.NodeID
}

func (w *realmStreamWrapper) Protocol() types.ProtocolID {
	return w.protocol
}

func (w *realmStreamWrapper) RemotePeer() types.NodeID {
	return w.remotePeer
}

// ============================================================================
//                              Relay 适配器（IMPL-1227 Phase 5）
// ============================================================================

// realmRelay Realm 中继服务适配器
//
// 封装 Realm Relay 功能，自动配置 PSK 认证。
// 当启动 Relay Server 时，会自动设置 RealmID 和 PSK 认证器。
type realmRelay struct {
	realm    *realmImpl
	serving  bool
	servOpts *realmif.RelayServeOptions
}

// Serve 启动 Realm Relay 服务
//
// IMPL-1227: Realm Relay 会自动配置：
//   - RealmID: 从 Realm 获取
//   - PSK 认证器: 从 Realm 获取，用于验证成员资格
//
// 这确保只有持有相同 RealmKey 的节点才能使用此 Relay。
func (r *realmRelay) Serve(ctx context.Context, opts ...realmif.RelayServeOption) error {
	options := &realmif.RelayServeOptions{}
	for _, opt := range opts {
		opt(options)
	}
	r.servOpts = options

	// 获取底层 Relay Server
	relayServer := r.realm.manager.RelayServer()
	if relayServer == nil {
		return fmt.Errorf("relay server not available, enable with dep2p.WithRelayServer()")
	}

	// 配置为 Realm Relay 模式
	if srv, ok := relayServer.(*server.Server); ok {
		srv.SetRealmID(r.realm.ID())
		srv.SetPSKAuthenticator(r.realm.PSKAuth())

		log.Info("启动 Realm Relay 服务",
			"realmID", r.realm.ID(),
			"pskEnabled", r.realm.psk != nil)
	} else {
		return fmt.Errorf("relay server does not support Realm mode")
	}

	// 向 DHT 注册为 Realm Relay 提供者
	discovery := r.realm.manager.discovery
	if discovery != nil {
		namespace := fmt.Sprintf("realm/%s/relay", r.realm.ID())
		if err := discovery.Announce(ctx, namespace); err != nil {
			log.Warn("注册 Realm Relay 到 DHT 失败", "err", err)
		}
	}

	r.serving = true
	return nil
}

// StopServing 停止 Realm Relay 服务
func (r *realmRelay) StopServing() error {
	if !r.serving {
		return nil
	}
	r.serving = false

	// 取消 DHT 注册
	discovery := r.realm.manager.discovery
	if discovery != nil {
		namespace := fmt.Sprintf("realm/%s/relay", r.realm.ID())
		discovery.StopAnnounce(namespace)
	}

	// 重置底层 Relay Server 为 System Relay 模式
	relayServer := r.realm.manager.RelayServer()
	if relayServer != nil {
		if srv, ok := relayServer.(*server.Server); ok {
			srv.SetRealmID("")
			srv.SetPSKAuthenticator(nil)
		}
	}

	log.Info("停止 Realm Relay 服务", "realmID", r.realm.ID())
	return nil
}

// IsServing 检查是否正在提供 Relay 服务
func (r *realmRelay) IsServing() bool {
	return r.serving
}

// FindRelays 发现 Realm 内的中继服务器
//
// 只返回同一 Realm 内的中继节点。
// 实现策略：通过 DHT 发现 Realm 内注册为 Relay 服务的节点。
func (r *realmRelay) FindRelays(ctx context.Context) ([]types.NodeID, error) {
	discovery := r.realm.manager.discovery
	if discovery == nil {
		log.Debug("FindRelays: 发现服务不可用，返回空结果",
			"realm", r.realm.ID())
		return []types.NodeID{}, nil // 返回空切片而非 nil
	}

	// 发现 Realm 内的 Relay 提供者
	// 命名空间格式：realm/<realmID>/relay
	namespace := fmt.Sprintf("realm/%s/relay", r.realm.ID())
	ch, err := discovery.DiscoverPeers(ctx, namespace)
	if err != nil {
		return nil, fmt.Errorf("discover relays: %w", err)
	}

	// 收集结果
	var relays []types.NodeID
	for info := range ch {
		relays = append(relays, info.ID)
	}

	return relays, nil
}

// Reserve 在 Realm Relay 上预留资源
//
// IMPL-1227: 预留请求会包含 PSK 成员证明。
func (r *realmRelay) Reserve(ctx context.Context, relayNode types.NodeID) (realmif.Reservation, error) {
	// 获取底层 Relay Client
	relayClient := r.realm.manager.RelayClient()
	if relayClient == nil {
		return nil, fmt.Errorf("relay client not available")
	}

	// 设置 PSK 认证器以支持 Realm Relay 握手
	if client, ok := relayClient.(*relay.RelayClient); ok {
		client.SetPSKAuthenticator(r.realm.PSKAuth())
	}

	// 调用底层 Reserve
	res, err := relayClient.Reserve(ctx, relayNode)
	if err != nil {
		return nil, fmt.Errorf("reserve on realm relay: %w", err)
	}

	return &realmReservation{underlying: res}, nil
}

// Stats 返回 Relay 统计信息
func (r *realmRelay) Stats() realmif.RelayStats {
	relayServer := r.realm.manager.RelayServer()
	if relayServer == nil {
		return realmif.RelayStats{}
	}

	// 获取底层统计信息
	stats := relayServer.Stats()
	return realmif.RelayStats{
		RelayedConnections:       int64(stats.TotalConnections),
		RelayedBytes:             int64(stats.TotalBytesRelayed),
		ActiveRelayedConnections: stats.ActiveConnections,
	}
}

// RealmID 返回关联的 Realm ID（用于外部配置 Relay Server）
func (r *realmRelay) RealmID() types.RealmID {
	return r.realm.ID()
}

// PSKAuthenticator 返回 PSK 认证器（用于外部配置 Relay Server）
func (r *realmRelay) PSKAuthenticator() realmif.PSKAuthenticator {
	return r.realm.PSKAuth()
}

// ============================================================================
//                              Realm Reservation 适配器
// ============================================================================

// realmReservation 包装底层 relayif.Reservation
type realmReservation struct {
	underlying relayif.Reservation
}

func (r *realmReservation) Relay() types.NodeID {
	return r.underlying.Relay()
}

func (r *realmReservation) Expiry() time.Time {
	return r.underlying.Expiry()
}

func (r *realmReservation) Addrs() []string {
	// 转换 netaddr.Address 到 []string
	addrs := r.underlying.Addrs()
	result := make([]string, len(addrs))
	for i, addr := range addrs {
		result[i] = addr.String()
	}
	return result
}

func (r *realmReservation) Refresh(ctx context.Context) error {
	return r.underlying.Refresh(ctx)
}

func (r *realmReservation) Close() error {
	return r.underlying.Cancel()
}

