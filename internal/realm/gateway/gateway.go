package gateway

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/protocol"
)

var logger = log.Logger("realm/gateway")

// ============================================================================
//                              Gateway 实现
// ============================================================================

// Gateway 域网关
type Gateway struct {
	mu sync.RWMutex

	// 配置
	realmID string
	config  *Config

	// 组件
	host         pkgif.Host
	auth         interfaces.Authenticator
	relayService interfaces.RelayService
	connPool     interfaces.ConnectionPool
	limiter      interfaces.BandwidthLimiter
	validator    interfaces.ProtocolValidator
	metrics      *Metrics

	// 可达节点列表
	reachableNodes []string

	// 状态
	started atomic.Bool
	closed  atomic.Bool

	// 控制
	ctx    context.Context
	cancel context.CancelFunc
}

// NewGateway 创建 Gateway
func NewGateway(realmID string, host pkgif.Host, auth interfaces.Authenticator, config *Config) *Gateway {
	if config == nil {
		config = DefaultConfig()
	}

	validator := NewProtocolValidator()
	limiter := NewBandwidthLimiter(config.MaxBandwidth, config.BurstSize)
	connPool := NewConnectionPool(host, config.MaxConnPerPeer, config.MaxConcurrent)
	metrics := NewMetrics()

	gw := &Gateway{
		realmID:        realmID,
		config:         config,
		host:           host,
		auth:           auth,
		validator:      validator,
		limiter:        limiter,
		connPool:       connPool,
		metrics:        metrics,
		reachableNodes: make([]string, 0),
	}

	// 创建 RelayService
	gw.relayService = NewRelayService(gw)

	return gw
}

// ============================================================================
//                              中继转发
// ============================================================================

// Relay 中继转发
func (g *Gateway) Relay(ctx context.Context, req *interfaces.RelayRequest) error {
	if !g.started.Load() {
		return ErrNotStarted
	}

	if g.closed.Load() {
		return ErrGatewayClosed
	}

	logger.Debug("中继转发请求", "source", log.TruncateID(req.SourcePeerID, 8), "target", log.TruncateID(req.TargetPeerID, 8), "protocol", req.Protocol)
	g.metrics.RecordRelay()

	// 1. 验证请求
	if err := g.validateRequest(req); err != nil {
		logger.Warn("中继请求验证失败", "error", err)
		g.metrics.RecordFailure()
		return err
	}

	// 2. PSK 认证验证
	if g.auth != nil {
		_, err := g.auth.Authenticate(ctx, req.SourcePeerID, req.AuthProof)
		if err != nil {
			logger.Warn("中继认证失败", "source", log.TruncateID(req.SourcePeerID, 8), "error", err)
			g.metrics.RecordFailure()
			return ErrAuthFailed
		}
		logger.Debug("中继认证成功", "source", log.TruncateID(req.SourcePeerID, 8))
	}

	// 3. 协议前缀验证
	if err := g.validator.ValidateProtocol(req.Protocol, g.realmID); err != nil {
		g.metrics.RecordFailure()
		return err
	}

	// 4. 带宽限流
	if g.config.EnableRateLimit {
		token, err := g.limiter.Acquire(ctx, int64(len(req.Data)))
		if err != nil {
			logger.Warn("带宽限流触发", "source", log.TruncateID(req.SourcePeerID, 8))
			g.metrics.RecordFailure()
			return ErrBandwidthLimit
		}
		defer g.limiter.Release(token)
	}

	// 5. 获取连接
	conn, err := g.connPool.Acquire(ctx, req.TargetPeerID)
	if err != nil {
		logger.Warn("获取目标连接失败", "target", log.TruncateID(req.TargetPeerID, 8), "error", err)
		g.metrics.RecordFailure()
		return err
	}
	defer g.connPool.Release(req.TargetPeerID, conn)

	// 6. 创建会话并转发
	session := g.relayService.NewSession(req)
	defer session.Close()

	if err := session.Transfer(ctx, conn); err != nil {
		logger.Warn("中继转发失败", "source", log.TruncateID(req.SourcePeerID, 8), "target", log.TruncateID(req.TargetPeerID, 8), "error", err)
		g.metrics.RecordFailure()
		return err
	}

	// 7. 记录成功
	g.metrics.RecordSuccess()
	stats := session.GetStats()
	g.metrics.RecordBytes(stats.BytesSent, stats.BytesRecv)
	logger.Debug("中继转发成功", "source", log.TruncateID(req.SourcePeerID, 8), "target", log.TruncateID(req.TargetPeerID, 8), "bytesSent", stats.BytesSent, "bytesRecv", stats.BytesRecv)

	return nil
}

// validateRequest 验证请求
func (g *Gateway) validateRequest(req *interfaces.RelayRequest) error {
	if req == nil {
		return ErrInvalidRequest
	}

	if req.SourcePeerID == "" || req.TargetPeerID == "" {
		return ErrInvalidRequest
	}

	if req.Protocol == "" {
		return ErrInvalidRequest
	}

	if req.RealmID != g.realmID {
		return ErrRealmMismatch
	}

	return nil
}

// ============================================================================
//                              服务监听
// ============================================================================

// ServeRelay 启动中继服务监听
func (g *Gateway) ServeRelay(ctx context.Context) error {
	if !g.started.Load() {
		return ErrNotStarted
	}

	if g.host == nil {
		return ErrNoHost
	}

	// 设置中继协议处理器
	protoID := protocol.BuildRealmProtocol(g.realmID, "relay", protocol.Version10)
	g.host.SetStreamHandler(string(protoID), g.handleStream)

	// 等待取消
	<-ctx.Done()
	return ctx.Err()
}

// handleStream 处理流
func (g *Gateway) handleStream(stream pkgif.Stream) {
	defer stream.Close()

	ctx := context.Background()
	g.relayService.HandleRelayRequest(ctx, stream)
}

// ============================================================================
//                              查询接口
// ============================================================================

// GetReachableNodes 查询可达节点
func (g *Gateway) GetReachableNodes() []string {
	g.mu.RLock()
	defer g.mu.RUnlock()

	nodes := make([]string, len(g.reachableNodes))
	copy(nodes, g.reachableNodes)

	return nodes
}

// ReportState 报告网关状态
func (g *Gateway) ReportState(_ context.Context) (*interfaces.GatewayState, error) {
	poolStats := g.connPool.GetStats()
	limiterStats := g.limiter.GetStats()

	state := &interfaces.GatewayState{
		ReachableNodes: g.GetReachableNodes(),
		RelayNodes:     []string{g.realmID}, // 本网关
		LastUpdated:    time.Now(),
	}

	// 添加容量信息
	_ = poolStats
	_ = limiterStats

	return state, nil
}

// UpdateReachableNodes 更新可达节点
func (g *Gateway) UpdateReachableNodes(nodes []string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.reachableNodes = make([]string, len(nodes))
	copy(g.reachableNodes, nodes)
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动 Gateway
func (g *Gateway) Start(_ context.Context) error {
	if g.started.Load() {
		return ErrAlreadyStarted
	}

	if g.closed.Load() {
		return ErrGatewayClosed
	}

	// 使用 context.Background() 保证后台循环不受上层 ctx 取消的影响
	g.ctx, g.cancel = context.WithCancel(context.Background())
	g.started.Store(true)

	return nil
}

// Stop 停止 Gateway
func (g *Gateway) Stop(_ context.Context) error {
	if !g.started.Load() {
		return ErrNotStarted
	}

	g.started.Store(false)

	if g.cancel != nil {
		g.cancel()
	}

	return nil
}

// Close 关闭 Gateway
func (g *Gateway) Close() error {
	if g.closed.Load() {
		return nil
	}

	g.closed.Store(true)

	if g.started.Load() {
		ctx := context.Background()
		g.Stop(ctx)
	}

	// 关闭组件
	if g.connPool != nil {
		g.connPool.Close()
	}

	if g.limiter != nil {
		g.limiter.Close()
	}

	return nil
}

// 确保实现接口
var _ interfaces.Gateway = (*Gateway)(nil)
