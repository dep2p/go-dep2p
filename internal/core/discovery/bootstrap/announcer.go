// Package bootstrap 提供引导节点发现和通告功能
package bootstrap

import (
	"context"
	"sync"
	"time"

	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              常量
// ============================================================================

const (
	// BootstrapNamespace Bootstrap 发现命名空间
	// 与 DHT Provider key 格式对齐：dep2p/v1/sys/bootstrap
	BootstrapNamespace = "bootstrap"

	// BootstrapNamespaceKey DHT Provider key
	// 完整格式：dep2p/v1/sys/bootstrap
	BootstrapNamespaceKey = "dep2p/v1/sys/bootstrap"

	// DefaultAdvertiseInterval 默认通告间隔
	DefaultAdvertiseInterval = 20 * time.Minute

	// DefaultDiscoveryLimit 默认发现数量限制
	DefaultDiscoveryLimit = 20
)

// ============================================================================
//                              BootstrapAnnouncer 实现
// ============================================================================

// Announcer Bootstrap 节点通告器
//
// Layer1 修复：使 Bootstrap 节点注册到 DHT，可被动态发现。
// 类似 RelayDiscovery，但专门用于 Bootstrap 节点。
type Announcer struct {
	discovery discoveryif.DiscoveryService
	endpoint  endpoint.Endpoint
	localID   types.NodeID

	// 已发现的 Bootstrap 节点
	bootstraps   map[types.NodeID]*discoveredBootstrap
	bootstrapsMu sync.RWMutex

	// 是否是 Bootstrap 服务器
	isServer bool

	// 通告间隔
	advertiseInterval time.Duration

	// 状态
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.RWMutex
}

// discoveredBootstrap 发现的 Bootstrap 节点
type discoveredBootstrap struct {
	ID       types.NodeID
	Addrs    []endpoint.Address
	LastSeen time.Time
}

// NewAnnouncer 创建 Bootstrap 节点通告器
//
// 参数：
//   - discovery: 发现服务（用于注册到 DHT）
//   - endpoint: 端点（用于获取本地地址）
//   - localID: 本地节点 ID
//   - isServer: 是否是 Bootstrap 服务器（如果是，会注册到 DHT）
func NewAnnouncer(discovery discoveryif.DiscoveryService, endpoint endpoint.Endpoint, localID types.NodeID, isServer bool) *Announcer {
	return &Announcer{
		discovery:         discovery,
		endpoint:          endpoint,
		localID:           localID,
		bootstraps:        make(map[types.NodeID]*discoveredBootstrap),
		isServer:          isServer,
		advertiseInterval: DefaultAdvertiseInterval,
	}
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动通告服务
func (a *Announcer) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.running {
		return nil
	}

	a.ctx, a.cancel = context.WithCancel(ctx)

	// 如果是 Bootstrap 服务器，启动通告
	if a.isServer {
		go a.advertiseLoop()
	}

	// 启动发现循环（即使不是服务器，也可以发现其他 Bootstrap 节点）
	go a.discoveryLoop()

	a.running = true
	log.Info("Bootstrap 通告器已启动",
		"is_server", a.isServer)

	return nil
}

// Stop 停止通告服务
func (a *Announcer) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.running {
		return nil
	}

	if a.cancel != nil {
		a.cancel()
	}

	a.running = false
	log.Info("Bootstrap 通告器已停止")
	return nil
}

// ============================================================================
//                              发现接口
// ============================================================================

// FindBootstrapNodes 发现 Bootstrap 节点
func (a *Announcer) FindBootstrapNodes(ctx context.Context, count int) ([]types.NodeID, error) {
	if count <= 0 {
		count = DefaultDiscoveryLimit
	}

	// 首先返回缓存的 Bootstrap 节点
	a.bootstrapsMu.RLock()
	cached := make([]types.NodeID, 0, len(a.bootstraps))
	for id := range a.bootstraps {
		cached = append(cached, id)
		if len(cached) >= count {
			break
		}
	}
	a.bootstrapsMu.RUnlock()

	if len(cached) >= count {
		return cached[:count], nil
	}

	// 通过发现服务查找更多
	if a.discovery != nil {
		ch, err := a.discovery.DiscoverPeers(ctx, BootstrapNamespace)
		if err != nil {
			log.Debug("发现 Bootstrap 节点失败", "err", err)
		} else {
			for peer := range ch {
				if len(cached) >= count {
					break
				}

				// 转换地址
				addrs := make([]endpoint.Address, len(peer.Addrs))
				for i, addr := range peer.Addrs {
					addrs[i] = &announcerStringAddress{s: addr.String()}
				}

				a.bootstrapsMu.Lock()
				a.bootstraps[peer.ID] = &discoveredBootstrap{
					ID:       peer.ID,
					Addrs:    addrs,
					LastSeen: time.Now(),
				}
				a.bootstrapsMu.Unlock()

				cached = append(cached, peer.ID)
			}
		}
	}

	return cached, nil
}

// Advertise 通告本节点为 Bootstrap 服务器
func (a *Announcer) Advertise(ctx context.Context) error {
	if a.discovery == nil {
		return nil
	}

	return a.discovery.Announce(ctx, BootstrapNamespace)
}

// ============================================================================
//                              后台任务
// ============================================================================

// advertiseLoop 通告循环
func (a *Announcer) advertiseLoop() {
	// 立即通告一次
	if err := a.Advertise(a.ctx); err != nil {
		log.Debug("Bootstrap 通告失败", "err", err)
	} else {
		log.Info("Bootstrap 节点已注册到 DHT")
	}

	ticker := time.NewTicker(a.advertiseInterval)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			if err := a.Advertise(a.ctx); err != nil {
				log.Debug("Bootstrap 通告失败", "err", err)
			}
		}
	}
}

// discoveryLoop 发现循环
func (a *Announcer) discoveryLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
			a.refreshBootstraps()
			a.cleanupStale()
		}
	}
}

// refreshBootstraps 刷新 Bootstrap 列表
func (a *Announcer) refreshBootstraps() {
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()

	_, err := a.FindBootstrapNodes(ctx, DefaultDiscoveryLimit)
	if err != nil {
		log.Debug("刷新 Bootstrap 列表失败", "err", err)
	}
}

// cleanupStale 清理过期 Bootstrap 节点
func (a *Announcer) cleanupStale() {
	a.bootstrapsMu.Lock()
	defer a.bootstrapsMu.Unlock()

	cutoff := time.Now().Add(-30 * time.Minute)
	for id, bootstrap := range a.bootstraps {
		if bootstrap.LastSeen.Before(cutoff) {
			delete(a.bootstraps, id)
			log.Debug("移除过期 Bootstrap 节点",
				"node", id.ShortString())
		}
	}
}

// ============================================================================
//                              查询接口
// ============================================================================

// GetBootstrapNodes 获取所有已知 Bootstrap 节点
func (a *Announcer) GetBootstrapNodes() []types.NodeID {
	a.bootstrapsMu.RLock()
	defer a.bootstrapsMu.RUnlock()

	nodes := make([]types.NodeID, 0, len(a.bootstraps))
	for id := range a.bootstraps {
		nodes = append(nodes, id)
	}
	return nodes
}

// GetBootstrapAddrs 获取 Bootstrap 节点地址
func (a *Announcer) GetBootstrapAddrs(nodeID types.NodeID) []endpoint.Address {
	a.bootstrapsMu.RLock()
	defer a.bootstrapsMu.RUnlock()

	if bootstrap, ok := a.bootstraps[nodeID]; ok {
		return bootstrap.Addrs
	}
	return nil
}

// AddBootstrap 手动添加 Bootstrap 节点
func (a *Announcer) AddBootstrap(nodeID types.NodeID, addrs []endpoint.Address) {
	a.bootstrapsMu.Lock()
	defer a.bootstrapsMu.Unlock()

	a.bootstraps[nodeID] = &discoveredBootstrap{
		ID:       nodeID,
		Addrs:    addrs,
		LastSeen: time.Now(),
	}
}

// RemoveBootstrap 移除 Bootstrap 节点
func (a *Announcer) RemoveBootstrap(nodeID types.NodeID) {
	a.bootstrapsMu.Lock()
	defer a.bootstrapsMu.Unlock()

	delete(a.bootstraps, nodeID)
}

// ============================================================================
//                              辅助类型
// ============================================================================

// announcerStringAddress 字符串地址
type announcerStringAddress struct {
	s string
}

func (a *announcerStringAddress) Network() string   { return "bootstrap" }
func (a *announcerStringAddress) String() string    { return a.s }
func (a *announcerStringAddress) Bytes() []byte     { return []byte(a.s) }
func (a *announcerStringAddress) IsPublic() bool    { return types.Multiaddr(a.s).IsPublic() }
func (a *announcerStringAddress) IsPrivate() bool   { return types.Multiaddr(a.s).IsPrivate() }
func (a *announcerStringAddress) IsLoopback() bool  { return types.Multiaddr(a.s).IsLoopback() }
func (a *announcerStringAddress) Multiaddr() string { return a.s } // 已经是 multiaddr 格式
func (a *announcerStringAddress) Equal(other endpoint.Address) bool {
	if other == nil {
		return false
	}
	return a.s == other.String()
}

