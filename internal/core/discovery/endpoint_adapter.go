package discovery

import (
	"context"

	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	endpointif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// endpointDiscoveryAdapter 将 pkg/interfaces/discovery.DiscoveryService 适配为
// pkg/interfaces/endpoint.DiscoveryService，供 Endpoint 注入使用。
//
// 关键点：此前 Fx 中 name:"discovery" 输出的是 discoveryif.DiscoveryService，
// 但 Endpoint 依赖的是 endpointif.DiscoveryService，类型不匹配导致注入为 nil，
// 进而 RefreshAnnounce/OnPeerDiscovered 都不会生效。
type endpointDiscoveryAdapter struct {
	// 这里用 *DiscoveryService（内部实现）以便调用 RefreshAnnounce 等非接口方法。
	// ProvideServices() 确保传入的是该实现。
	svc *DiscoveryService
}

func (a *endpointDiscoveryAdapter) FindPeer(ctx context.Context, id endpointif.NodeID) ([]endpointif.Address, error) {
	return a.svc.FindPeer(ctx, id)
}

func (a *endpointDiscoveryAdapter) Announce(ctx context.Context, namespace string) error {
	return a.svc.Announce(ctx, namespace)
}

func (a *endpointDiscoveryAdapter) RefreshAnnounce(addrs []endpointif.Address) {
	a.svc.RefreshAnnounce(addrs)
}

func (a *endpointDiscoveryAdapter) OnPeerDiscovered(callback func(endpointif.PeerInfo)) {
	if callback == nil {
		return
	}
	a.svc.OnPeerDiscovered(func(peer discoveryif.PeerInfo) {
		callback(endpointif.PeerInfo{
			ID:    peer.ID,
			Addrs: types.MultiaddrsToStrings(peer.Addrs),
		})
	})
}

// DiscoverPeers 发现指定命名空间中的节点
func (a *endpointDiscoveryAdapter) DiscoverPeers(ctx context.Context, namespace string) (<-chan endpointif.PeerInfo, error) {
	// 调用内部 DiscoveryService 的 DiscoverPeers
	internalCh, err := a.svc.DiscoverPeers(ctx, namespace)
	if err != nil {
		return nil, err
	}

	// 转换 channel 类型
	ch := make(chan endpointif.PeerInfo, 20)
	go func() {
		defer close(ch)
		for peer := range internalCh {
			select {
			case <-ctx.Done():
				return
			case ch <- endpointif.PeerInfo{
				ID:    peer.ID,
				Addrs: types.MultiaddrsToStrings(peer.Addrs),
			}:
			}
		}
	}()

	return ch, nil
}

// GetBootstrapPeers 获取当前配置的引导节点列表
func (a *endpointDiscoveryAdapter) GetBootstrapPeers(ctx context.Context) ([]endpointif.PeerInfo, error) {
	peers, err := a.svc.GetBootstrapPeers(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]endpointif.PeerInfo, len(peers))
	for i, peer := range peers {
		result[i] = endpointif.PeerInfo{
			ID:    peer.ID,
			Addrs: types.MultiaddrsToStrings(peer.Addrs),
		}
	}
	return result, nil
}

// AddBootstrapPeer 运行时添加引导节点
func (a *endpointDiscoveryAdapter) AddBootstrapPeer(peer endpointif.PeerInfo) {
	a.svc.AddBootstrapPeer(discoveryif.PeerInfo{
		ID:    peer.ID,
		Addrs: types.StringsToMultiaddrs(peer.Addrs),
	})
}

// RemoveBootstrapPeer 运行时移除引导节点
func (a *endpointDiscoveryAdapter) RemoveBootstrapPeer(id endpointif.NodeID) {
	a.svc.RemoveBootstrapPeer(id)
}

func (a *endpointDiscoveryAdapter) Start(ctx context.Context) error { return a.svc.Start(ctx) }
func (a *endpointDiscoveryAdapter) Stop() error                    { return a.svc.Stop() }

// State 返回当前入网状态
func (a *endpointDiscoveryAdapter) State() endpointif.DiscoveryState {
	state := a.svc.State()
	// 转换 discoveryif.DiscoveryState 到 endpointif.DiscoveryState
	// 两者定义相同，直接类型转换
	return endpointif.DiscoveryState(state)
}

// SetOnStateChanged 注册状态变更回调
func (a *endpointDiscoveryAdapter) SetOnStateChanged(callback func(endpointif.StateChangeEvent)) {
	if callback == nil {
		return
	}
	a.svc.SetOnStateChanged(func(event discoveryif.StateChangeEvent) {
		callback(endpointif.StateChangeEvent{
			OldState: endpointif.DiscoveryState(event.OldState),
			NewState: endpointif.DiscoveryState(event.NewState),
			Reason:   event.Reason,
		})
	})
}

// WaitReady 等待服务就绪
func (a *endpointDiscoveryAdapter) WaitReady(ctx context.Context) error {
	return a.svc.WaitReady(ctx)
}

// NotifyPeerConnected 通知新连接建立
func (a *endpointDiscoveryAdapter) NotifyPeerConnected(nodeID endpointif.NodeID, addrs []string) {
	a.svc.NotifyPeerConnected(nodeID, addrs)
}

// NotifyPeerDisconnected 通知连接断开
func (a *endpointDiscoveryAdapter) NotifyPeerDisconnected(nodeID endpointif.NodeID) {
	a.svc.NotifyPeerDisconnected(nodeID)
}

