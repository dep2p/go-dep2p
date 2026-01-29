package rendezvous

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	pb "github.com/dep2p/go-dep2p/pkg/lib/proto/rendezvous"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// Point Rendezvous Point 服务端
type Point struct {
	config PointConfig
	store  *Store
	host   pkgif.Host

	// 统计
	registersReceived uint64
	discoversReceived uint64

	// 生命周期
	ctx       context.Context
	ctxCancel context.CancelFunc
	started   atomic.Bool
	wg        sync.WaitGroup

	mu sync.RWMutex
}

// NewPoint 创建 Rendezvous Point
func NewPoint(host pkgif.Host, config PointConfig) *Point {
	storeConfig := StoreConfig{
		MaxRegistrations:             config.MaxRegistrations,
		MaxNamespaces:                config.MaxNamespaces,
		MaxRegistrationsPerNamespace: config.MaxRegistrationsPerNamespace,
		MaxRegistrationsPerPeer:      config.MaxRegistrationsPerPeer,
		MaxTTL:                       config.MaxTTL,
		DefaultTTL:                   config.DefaultTTL,
		CleanupInterval:              config.CleanupInterval,
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Point{
		config:    config,
		store:     NewStore(storeConfig),
		host:      host,
		ctx:       ctx,
		ctxCancel: cancel,
	}
}

// Start 启动 Point
func (p *Point) Start(_ context.Context) error {
	if p.started.Load() {
		return ErrAlreadyStarted
	}

	// 注册协议处理器
	p.host.SetStreamHandler(ProtocolID, func(stream pkgif.Stream) {
		p.handleStream(stream)
	})

	p.started.Store(true)

	// 启动清理循环
	p.wg.Add(1)
	go p.cleanupLoop()

	return nil
}

// Stop 停止 Point
func (p *Point) Stop() error {
	if !p.started.Load() {
		return ErrNotStarted
	}

	p.started.Store(false)
	p.ctxCancel()

	// 等待后台循环结束
	p.wg.Wait()

	// 移除协议处理器
	p.host.RemoveStreamHandler(ProtocolID)

	return nil
}

// ============================================================================
//                              协议处理
// ============================================================================

// handleStream 处理协议流
func (p *Point) handleStream(stream pkgif.Stream) {
	defer stream.Close()

	// 读取请求
	req, err := ReadMessage(stream)
	if err != nil {
		return
	}

	// 路由到对应处理函数
	var resp *pb.Message
	switch req.Type {
	case pb.Message_REGISTER:
		resp = p.handleRegister(req)
	case pb.Message_UNREGISTER:
		resp = p.handleUnregister(req)
	case pb.Message_DISCOVER:
		resp = p.handleDiscover(req)
	default:
		resp = NewRegisterResponse(pb.Message_E_INTERNAL_ERROR, "unknown message type", 0)
	}

	// 发送响应
	_ = WriteMessage(stream, resp)
}

// handleRegister 处理注册请求
func (p *Point) handleRegister(req *pb.Message) *pb.Message {
	atomic.AddUint64(&p.registersReceived, 1)

	if req.Register == nil {
		return NewRegisterResponse(pb.Message_E_INTERNAL_ERROR, "missing register field", 0)
	}

	// 验证请求
	if err := ValidateRegisterRequest(req.Register, p.config.MaxTTL); err != nil {
		return NewRegisterResponse(pb.Message_E_INVALID_NAMESPACE, err.Error(), 0)
	}

	// 提取 PeerInfo（简化版：从 SignedPeerRecord 解析）
	peerID := types.PeerID(req.Register.SignedPeerRecord)
	peerInfo := types.PeerInfo{
		ID:    peerID,
		Addrs: []types.Multiaddr{},
	}

	// 添加到存储
	ttl := time.Duration(req.Register.Ttl) * time.Second
	if err := p.store.Add(req.Register.Ns, peerInfo, ttl); err != nil {
		return NewRegisterResponse(pb.Message_E_INTERNAL_ERROR, err.Error(), 0)
	}

	return NewRegisterResponse(pb.Message_OK, "", ttl)
}

// handleUnregister 处理取消注册请求
func (p *Point) handleUnregister(req *pb.Message) *pb.Message {
	if req.Unregister == nil {
		return NewRegisterResponse(pb.Message_E_INTERNAL_ERROR, "missing unregister field", 0)
	}

	// 验证命名空间
	if err := ValidateNamespace(req.Unregister.Ns); err != nil {
		return NewRegisterResponse(pb.Message_E_INVALID_NAMESPACE, err.Error(), 0)
	}

	// 移除注册
	peerID := types.PeerID(req.Unregister.Id)
	p.store.Remove(req.Unregister.Ns, peerID)

	return NewRegisterResponse(pb.Message_OK, "", 0)
}

// handleDiscover 处理发现请求
func (p *Point) handleDiscover(req *pb.Message) *pb.Message {
	atomic.AddUint64(&p.discoversReceived, 1)

	if req.Discover == nil {
		return NewDiscoverResponse(pb.Message_E_INTERNAL_ERROR, "missing discover field", nil, nil)
	}

	// 验证请求
	if err := ValidateDiscoverRequest(req.Discover); err != nil {
		return NewDiscoverResponse(pb.Message_E_INVALID_NAMESPACE, err.Error(), nil, nil)
	}

	// 查询存储
	limit := int(req.Discover.Limit)
	if limit <= 0 {
		limit = p.config.DefaultDiscoverLimit
	}

	regs, nextCookie, err := p.store.Get(req.Discover.Ns, limit, req.Discover.Cookie)
	if err != nil {
		return NewDiscoverResponse(pb.Message_E_INTERNAL_ERROR, err.Error(), nil, nil)
	}

	// 转换为 protobuf
	var pbRegs []*pb.Message_Registration
	for _, reg := range regs {
		pbRegs = append(pbRegs, PeerInfoToRegistration(reg.PeerInfo, reg.Namespace, reg.RemainingTTL()))
	}

	return NewDiscoverResponse(pb.Message_OK, "", pbRegs, nextCookie)
}

// ============================================================================
//                              后台循环
// ============================================================================

// cleanupLoop 清理循环
func (p *Point) cleanupLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.store.CleanupExpired()

		case <-p.ctx.Done():
			return
		}
	}
}

// ============================================================================
//                              统计
// ============================================================================

// Stats 返回统计信息
func (p *Point) Stats() Stats {
	return p.store.Stats()
}
