package rendezvous

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	pb "github.com/dep2p/go-dep2p/pkg/proto/rendezvous"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              配置
// ============================================================================

// PointConfig Rendezvous Point 配置
type PointConfig struct {
	// MaxRegistrations 最大注册数
	MaxRegistrations int

	// MaxNamespaces 最大命名空间数
	MaxNamespaces int

	// MaxTTL 最大 TTL
	MaxTTL time.Duration

	// DefaultTTL 默认 TTL
	DefaultTTL time.Duration

	// CleanupInterval 清理间隔
	CleanupInterval time.Duration

	// MaxRegistrationsPerNamespace 每个命名空间最大注册数
	MaxRegistrationsPerNamespace int

	// MaxRegistrationsPerPeer 每个节点最大注册数
	MaxRegistrationsPerPeer int

	// DefaultDiscoverLimit 默认发现限制
	DefaultDiscoverLimit int
}

// DefaultPointConfig 默认配置
func DefaultPointConfig() PointConfig {
	return PointConfig{
		MaxRegistrations:             10000,
		MaxNamespaces:                1000,
		MaxTTL:                       72 * time.Hour,
		DefaultTTL:                   2 * time.Hour,
		CleanupInterval:              5 * time.Minute,
		MaxRegistrationsPerNamespace: 1000,
		MaxRegistrationsPerPeer:      100,
		DefaultDiscoverLimit:         100,
	}
}

// ============================================================================
//                              Point 实现
// ============================================================================

// Point Rendezvous Point 服务端
type Point struct {
	config   PointConfig
	store    *Store
	endpoint endpoint.Endpoint

	// 统计
	registersReceived uint64
	discoversReceived uint64

	// 生命周期
	running int32
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewPoint 创建 Rendezvous Point
func NewPoint(endpoint endpoint.Endpoint, config PointConfig) *Point {
	storeConfig := StoreConfig{
		MaxRegistrations:             config.MaxRegistrations,
		MaxNamespaces:                config.MaxNamespaces,
		MaxRegistrationsPerNamespace: config.MaxRegistrationsPerNamespace,
		MaxRegistrationsPerPeer:      config.MaxRegistrationsPerPeer,
		MaxTTL:                       config.MaxTTL,
		DefaultTTL:                   config.DefaultTTL,
		CleanupInterval:              config.CleanupInterval,
	}

	return &Point{
		config:   config,
		store:    NewStore(storeConfig),
		endpoint: endpoint,
	}
}

// 确保实现接口
var _ discoveryif.RendezvousPoint = (*Point)(nil)

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动 Rendezvous Point
func (p *Point) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&p.running, 0, 1) {
		return errors.New("rendezvous point already running")
	}

	p.ctx, p.cancel = context.WithCancel(ctx)

	// 注册协议处理器
	if p.endpoint != nil {
		p.endpoint.SetProtocolHandler(ProtocolID, p.handleStream)
	}

	log.Info("rendezvous point started")
	return nil
}

// Stop 停止 Rendezvous Point
func (p *Point) Stop() error {
	if !atomic.CompareAndSwapInt32(&p.running, 1, 0) {
		return nil
	}

	p.cancel()
	p.wg.Wait()

	// 移除协议处理器
	if p.endpoint != nil {
		p.endpoint.RemoveProtocolHandler(ProtocolID)
	}

	// 关闭存储
	p.store.Close()

	log.Info("rendezvous point stopped")
	return nil
}

// ============================================================================
//                              协议处理
// ============================================================================

// handleStream 处理入站流
func (p *Point) handleStream(stream endpoint.Stream) {
	p.wg.Add(1)
	defer p.wg.Done()
	defer func() { _ = stream.Close() }()

	conn := stream.Connection()
	if conn == nil {
		log.Debug("stream has no connection")
		return
	}
	remotePeer := conn.RemoteID()

	for {
		// 读取消息
		msg, err := ReadMessage(stream)
		if err != nil {
			if err != io.EOF {
				log.Debug("failed to read message",
					"err", err,
					"peer", remotePeer.String(),
				)
			}
			return
		}

		// 处理消息
		var response *pb.Message
		switch msg.Type {
		case pb.MessageType_MESSAGE_TYPE_REGISTER:
			response = p.handleRegisterMessage(remotePeer, msg.Register)
		case pb.MessageType_MESSAGE_TYPE_UNREGISTER:
			response = p.handleUnregisterMessage(remotePeer, msg.Unregister)
		case pb.MessageType_MESSAGE_TYPE_DISCOVER:
			response = p.handleDiscoverMessage(remotePeer, msg.Discover)
		default:
			log.Debug("unknown message type",
				"type", int32(msg.Type),
				"peer", remotePeer.String(),
			)
			continue
		}

		// 发送响应
		if response != nil {
			if err := WriteMessage(stream, response); err != nil {
				log.Debug("failed to write response",
					"err", err,
					"peer", remotePeer.String(),
				)
				return
			}
		}
	}
}

// handleRegisterMessage 处理注册消息
func (p *Point) handleRegisterMessage(_ types.NodeID, req *pb.Register) *pb.Message {
	atomic.AddUint64(&p.registersReceived, 1)

	// 验证请求
	if err := ValidateRegisterRequest(req, p.config.MaxTTL); err != nil {
		return NewRegisterResponse(pb.ResponseStatus_RESPONSE_STATUS_E_INVALID_NAMESPACE, err.Error(), 0)
	}

	// 验证地址
	if err := ValidateAddresses(req.Addrs); err != nil {
		return NewRegisterResponse(pb.ResponseStatus_RESPONSE_STATUS_E_INVALID_NAMESPACE, err.Error(), 0)
	}

	// 获取 TTL
	ttl := time.Duration(req.Ttl) * time.Second
	if ttl <= 0 {
		ttl = p.config.DefaultTTL
	}
	if ttl > p.config.MaxTTL {
		ttl = p.config.MaxTTL
	}

	// 解析 peer ID
	var peerID types.NodeID
	copy(peerID[:], req.PeerId)

	// 创建注册记录
	reg := &Registration{
		Namespace: req.Namespace,
		PeerInfo: discoveryif.PeerInfo{
			ID:    peerID,
			Addrs: types.StringsToMultiaddrs(req.Addrs),
		},
		TTL:          ttl,
		RegisteredAt: time.Now(),
		SignedRecord: req.SignedRecord,
	}

	// 存储注册
	if err := p.store.Register(reg); err != nil {
		var storeErr *StoreError
		if errors.As(err, &storeErr) {
			return NewRegisterResponse(pb.ResponseStatus_RESPONSE_STATUS_E_UNAVAILABLE, err.Error(), 0)
		}
		return NewRegisterResponse(pb.ResponseStatus_RESPONSE_STATUS_E_INTERNAL_ERROR, err.Error(), 0)
	}

	log.Debug("peer registered",
		"namespace", req.Namespace,
		"peer", peerID.String(),
		"ttl", ttl,
	)

	return NewRegisterResponse(pb.ResponseStatus_RESPONSE_STATUS_OK, "", ttl)
}

// handleUnregisterMessage 处理取消注册消息
func (p *Point) handleUnregisterMessage(_ types.NodeID, req *pb.Unregister) *pb.Message {
	// 验证命名空间
	if err := ValidateNamespace(req.Namespace); err != nil {
		return NewRegisterResponse(pb.ResponseStatus_RESPONSE_STATUS_E_INVALID_NAMESPACE, err.Error(), 0)
	}

	// 解析 peer ID
	var peerID types.NodeID
	copy(peerID[:], req.PeerId)

	// 取消注册
	if err := p.store.Unregister(req.Namespace, peerID); err != nil {
		return NewRegisterResponse(pb.ResponseStatus_RESPONSE_STATUS_E_INTERNAL_ERROR, err.Error(), 0)
	}

	log.Debug("peer unregistered",
		"namespace", req.Namespace,
		"peer", peerID.String(),
	)

	return NewRegisterResponse(pb.ResponseStatus_RESPONSE_STATUS_OK, "", 0)
}

// handleDiscoverMessage 处理发现消息
func (p *Point) handleDiscoverMessage(from types.NodeID, req *pb.Discover) *pb.Message {
	atomic.AddUint64(&p.discoversReceived, 1)

	// 验证请求
	if err := ValidateDiscoverRequest(req); err != nil {
		return NewDiscoverResponse(pb.ResponseStatus_RESPONSE_STATUS_E_INVALID_NAMESPACE, err.Error(), nil, nil)
	}

	// 获取限制
	limit := int(req.Limit)
	if limit <= 0 {
		limit = p.config.DefaultDiscoverLimit
	}

	// 查询注册
	regs, cookie, err := p.store.Discover(req.Namespace, limit, req.Cookie)
	if err != nil {
		return NewDiscoverResponse(pb.ResponseStatus_RESPONSE_STATUS_E_INTERNAL_ERROR, err.Error(), nil, nil)
	}

	// 转换为 protobuf
	pbRegs := make([]*pb.Registration, 0, len(regs))
	for _, reg := range regs {
		pbRegs = append(pbRegs, reg.ToProto())
	}

	log.Debug("discover request",
		"namespace", req.Namespace,
		"peer", from.String(),
		"results", len(pbRegs),
	)

	return NewDiscoverResponse(pb.ResponseStatus_RESPONSE_STATUS_OK, "", pbRegs, cookie)
}

// ============================================================================
//                              接口实现
// ============================================================================

// HandleRegister 处理注册请求
func (p *Point) HandleRegister(_ types.NodeID, namespace string, info discoveryif.PeerInfo, ttl time.Duration) error {
	// 验证命名空间
	if err := ValidateNamespace(namespace); err != nil {
		return err
	}

	// 验证 TTL
	if ttl <= 0 {
		ttl = p.config.DefaultTTL
	}
	if ttl > p.config.MaxTTL {
		ttl = p.config.MaxTTL
	}

	// 创建注册记录
	reg := &Registration{
		Namespace:    namespace,
		PeerInfo:     info,
		TTL:          ttl,
		RegisteredAt: time.Now(),
	}

	return p.store.Register(reg)
}

// HandleDiscover 处理发现请求
func (p *Point) HandleDiscover(_ types.NodeID, namespace string, limit int, cookie []byte) ([]discoveryif.PeerInfo, []byte, error) {
	// 验证命名空间
	if err := ValidateNamespace(namespace); err != nil {
		return nil, nil, err
	}

	if limit <= 0 {
		limit = p.config.DefaultDiscoverLimit
	}

	regs, nextCookie, err := p.store.Discover(namespace, limit, cookie)
	if err != nil {
		return nil, nil, err
	}

	peers := make([]discoveryif.PeerInfo, 0, len(regs))
	for _, reg := range regs {
		peers = append(peers, reg.PeerInfo)
	}

	return peers, nextCookie, nil
}

// HandleUnregister 处理取消注册请求
func (p *Point) HandleUnregister(from types.NodeID, namespace string) error {
	return p.store.Unregister(namespace, from)
}

// Stats 返回统计信息
func (p *Point) Stats() discoveryif.RendezvousStats {
	storeStats := p.store.Stats()
	return discoveryif.RendezvousStats{
		TotalRegistrations:   storeStats.TotalRegistrations,
		TotalNamespaces:      storeStats.TotalNamespaces,
		RegistersReceived:    atomic.LoadUint64(&p.registersReceived),
		DiscoversReceived:    atomic.LoadUint64(&p.discoversReceived),
		RegistrationsExpired: storeStats.RegistrationsExpired,
	}
}

// Namespaces 返回所有命名空间
func (p *Point) Namespaces() []string {
	return p.store.Namespaces()
}

// PeersInNamespace 返回命名空间中的节点数
func (p *Point) PeersInNamespace(namespace string) int {
	return p.store.PeersInNamespace(namespace)
}

