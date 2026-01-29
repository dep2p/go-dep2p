package gateway

import (
	"context"
	"sync"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
)

// ============================================================================
//                              Routing 协作适配器
// ============================================================================

// RouterAdapter Routing 协作适配器
//
// RouterAdapter 负责 Gateway 与 Router 之间的协作：
// - 向 Router 报告 Gateway 容量
// - 维护可达节点列表
// - 处理来自 Router 的中继请求
type RouterAdapter struct {
	mu sync.RWMutex

	// 关联的 Gateway
	gateway interfaces.Gateway

	// Router 接口（可选）
	router interfaces.Router

	// 可达节点列表
	reachablePeers map[string]bool
}

// NewRouterAdapter 创建 RouterAdapter
func NewRouterAdapter(gateway interfaces.Gateway) *RouterAdapter {
	return &RouterAdapter{
		gateway:        gateway,
		reachablePeers: make(map[string]bool),
	}
}

// ============================================================================
//                              注册与报告
// ============================================================================

// RegisterWithRouter 注册到 Router
func (ra *RouterAdapter) RegisterWithRouter(gateway interfaces.Gateway) error {
	ra.mu.Lock()
	defer ra.mu.Unlock()

	ra.gateway = gateway

	return nil
}

// ReportCapacity 定期报告容量
func (ra *RouterAdapter) ReportCapacity(ctx context.Context) error {
	if ra.gateway == nil {
		return ErrGatewayClosed
	}

	// 获取网关状态
	state, err := ra.gateway.ReportState(ctx)
	if err != nil {
		return err
	}

	// 如果有 router，同步状态
	if ra.router != nil {
		// 调用 router 的状态同步接口
		// ra.router.SyncGatewayState(ctx, state)
		_ = state
	}

	return nil
}

// UpdateReachable 更新可达节点列表
//
// 该方法由 Router 调用，用于通知 Gateway 哪些节点可达。
// Gateway 使用此信息来决定中继请求是否可以被服务。
func (ra *RouterAdapter) UpdateReachable(peers []string) {
	ra.mu.Lock()
	defer ra.mu.Unlock()

	// 清空现有列表并重建
	ra.reachablePeers = make(map[string]bool, len(peers))
	for _, peer := range peers {
		ra.reachablePeers[peer] = true
	}
}

// IsReachable 检查节点是否可达
func (ra *RouterAdapter) IsReachable(peerID string) bool {
	ra.mu.RLock()
	defer ra.mu.RUnlock()

	return ra.reachablePeers[peerID]
}

// GetReachablePeers 获取所有可达节点
func (ra *RouterAdapter) GetReachablePeers() []string {
	ra.mu.RLock()
	defer ra.mu.RUnlock()

	peers := make([]string, 0, len(ra.reachablePeers))
	for peer := range ra.reachablePeers {
		peers = append(peers, peer)
	}
	return peers
}

// ============================================================================
//                              处理中继请求
// ============================================================================

// OnRelayRequest 处理来自 Router 的请求
func (ra *RouterAdapter) OnRelayRequest(ctx context.Context, req *interfaces.RelayRequest) error {
	if ra.gateway == nil {
		return ErrGatewayClosed
	}

	return ra.gateway.Relay(ctx, req)
}

// 确保实现接口
var _ interfaces.RouterAdapter = (*RouterAdapter)(nil)
