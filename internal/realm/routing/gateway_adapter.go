package routing

import (
	"context"
	"fmt"
	"sync"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
)

// ============================================================================
//                              Gateway 协作适配器
// ============================================================================

// GatewayAdapter Gateway 协作适配器
//
// Phase 11 修复：使用正确的 Gateway 接口类型
type GatewayAdapter struct {
	mu sync.RWMutex

	// Gateway 接口
	gateway interfaces.Gateway

	// 缓存的状态
	state *interfaces.GatewayState
}

// NewGatewayAdapter 创建 Gateway 适配器
func NewGatewayAdapter(gateway interfaces.Gateway) *GatewayAdapter {
	return &GatewayAdapter{
		gateway: gateway,
		state: &interfaces.GatewayState{
			ReachableNodes: make([]string, 0),
			RelayNodes:     make([]string, 0),
		},
	}
}

// ============================================================================
//                              中继请求
// ============================================================================

// RequestRelay 请求中继
//
// Phase 11 修复：实现真实中继逻辑
func (ga *GatewayAdapter) RequestRelay(ctx context.Context, targetPeerID string, data []byte) error {
	if ga.gateway == nil {
		return fmt.Errorf("gateway not available")
	}

	// 构造中继请求
	req := &interfaces.RelayRequest{
		TargetPeerID: targetPeerID,
		Data:         data,
	}

	// 调用 Gateway 的中继接口
	return ga.gateway.Relay(ctx, req)
}

// ============================================================================
//                              可达性查询
// ============================================================================

// QueryReachable 查询可达节点
func (ga *GatewayAdapter) QueryReachable(_ context.Context) ([]string, error) {
	ga.mu.RLock()
	defer ga.mu.RUnlock()

	if ga.state == nil {
		return nil, fmt.Errorf("gateway state not initialized")
	}

	return ga.state.ReachableNodes, nil
}

// ============================================================================
//                              状态同步
// ============================================================================

// SyncState 同步状态
func (ga *GatewayAdapter) SyncState(_ context.Context, state *interfaces.GatewayState) error {
	if state == nil {
		return fmt.Errorf("invalid gateway state")
	}

	ga.mu.Lock()
	defer ga.mu.Unlock()

	ga.state = state

	return nil
}

// GetState 获取状态
func (ga *GatewayAdapter) GetState() *interfaces.GatewayState {
	ga.mu.RLock()
	defer ga.mu.RUnlock()

	return ga.state
}

// 确保实现接口
var _ interfaces.GatewayAdapter = (*GatewayAdapter)(nil)
