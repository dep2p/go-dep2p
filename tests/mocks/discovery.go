package mocks

import (
	"context"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// MockDiscovery 模拟 Discovery 接口实现
type MockDiscovery struct {
	// 存储发现的节点
	Peers map[string]types.PeerInfo

	// 可覆盖的方法
	FindPeersFunc  func(ctx context.Context, namespace string, opts ...interfaces.DiscoveryOption) (<-chan types.PeerInfo, error)
	AdvertiseFunc  func(ctx context.Context, namespace string, opts ...interfaces.DiscoveryOption) (time.Duration, error)

	// 调用记录
	FindPeersCalls  []FindPeersCall
	AdvertiseCalls  []AdvertiseCall
}

// FindPeersCall 记录 FindPeers 调用
type FindPeersCall struct {
	Namespace string
}

// AdvertiseCall 记录 Advertise 调用
type AdvertiseCall struct {
	Namespace string
}

// NewMockDiscovery 创建带有默认值的 MockDiscovery
func NewMockDiscovery() *MockDiscovery {
	return &MockDiscovery{
		Peers: make(map[string]types.PeerInfo),
	}
}

// FindPeers 发现节点
func (m *MockDiscovery) FindPeers(ctx context.Context, namespace string, opts ...interfaces.DiscoveryOption) (<-chan types.PeerInfo, error) {
	m.FindPeersCalls = append(m.FindPeersCalls, FindPeersCall{Namespace: namespace})
	
	if m.FindPeersFunc != nil {
		return m.FindPeersFunc(ctx, namespace, opts...)
	}

	ch := make(chan types.PeerInfo, len(m.Peers))
	go func() {
		defer close(ch)
		for _, peer := range m.Peers {
			select {
			case <-ctx.Done():
				return
			case ch <- peer:
			}
		}
	}()
	return ch, nil
}

// Advertise 广播服务
func (m *MockDiscovery) Advertise(ctx context.Context, namespace string, opts ...interfaces.DiscoveryOption) (time.Duration, error) {
	m.AdvertiseCalls = append(m.AdvertiseCalls, AdvertiseCall{Namespace: namespace})
	
	if m.AdvertiseFunc != nil {
		return m.AdvertiseFunc(ctx, namespace, opts...)
	}
	return time.Hour, nil
}

// AddPeer 添加测试用的节点
func (m *MockDiscovery) AddPeer(peer types.PeerInfo) {
	m.Peers[string(peer.ID)] = peer
}

// Start 启动发现服务
func (m *MockDiscovery) Start(_ context.Context) error {
	return nil
}

// Stop 停止发现服务
func (m *MockDiscovery) Stop(_ context.Context) error {
	return nil
}

// 确保实现接口
var _ interfaces.Discovery = (*MockDiscovery)(nil)
