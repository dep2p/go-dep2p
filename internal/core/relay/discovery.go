// Package relay 提供中继发现功能
package relay

import (
	"context"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/address"
	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	relayif "github.com/dep2p/go-dep2p/pkg/interfaces/relay"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              常量
// ============================================================================

const (
	// RelayNamespace 中继发现命名空间
	// 与 DHT Provider key 格式对齐：dep2p/v1/sys/relay
	RelayNamespace = "relay"

	// RelayNamespaceKey DHT Provider key
	// 完整格式：dep2p/v1/sys/relay
	RelayNamespaceKey = "dep2p/v1/sys/relay"

	// DefaultAdvertiseInterval 默认通告间隔
	DefaultAdvertiseInterval = 10 * time.Minute

	// DefaultDiscoveryLimit 默认发现数量限制
	DefaultDiscoveryLimit = 10
)

// ============================================================================
//                              RelayDiscovery 实现
// ============================================================================

// RelayDiscovery 中继发现服务
type RelayDiscovery struct {
	discovery discoveryif.DiscoveryService
	endpoint  endpoint.Endpoint
	localID   types.NodeID

	// 已发现的中继
	relays   map[types.NodeID]*discoveredRelay
	relaysMu sync.RWMutex

	// 是否是中继服务器
	isServer bool

	// 通告间隔
	advertiseInterval time.Duration

	// 状态
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.RWMutex
}

// discoveredRelay 发现的中继
type discoveredRelay struct {
	ID       types.NodeID
	Addrs    []endpoint.Address
	LastSeen time.Time
	Latency  time.Duration
}

// NewRelayDiscovery 创建中继发现服务
func NewRelayDiscovery(discovery discoveryif.DiscoveryService, endpoint endpoint.Endpoint, localID types.NodeID, isServer bool) *RelayDiscovery {
	return &RelayDiscovery{
		discovery:         discovery,
		endpoint:          endpoint,
		localID:           localID,
		relays:            make(map[types.NodeID]*discoveredRelay),
		isServer:          isServer,
		advertiseInterval: DefaultAdvertiseInterval,
	}
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动发现服务
func (rd *RelayDiscovery) Start(ctx context.Context) error {
	rd.mu.Lock()
	defer rd.mu.Unlock()

	if rd.running {
		return nil
	}

	rd.ctx, rd.cancel = context.WithCancel(ctx)

	// 如果是服务器，启动通告
	if rd.isServer {
		go rd.advertiseLoop()
	}

	// 启动发现循环
	go rd.discoveryLoop()

	rd.running = true
	log.Info("中继发现服务已启动",
		"is_server", rd.isServer)

	return nil
}

// Stop 停止发现服务
func (rd *RelayDiscovery) Stop() error {
	rd.mu.Lock()
	defer rd.mu.Unlock()

	if !rd.running {
		return nil
	}

	if rd.cancel != nil {
		rd.cancel()
	}

	rd.running = false
	log.Info("中继发现服务已停止")
	return nil
}

// ============================================================================
//                              发现接口
// ============================================================================

// FindRelays 发现中继服务器
func (rd *RelayDiscovery) FindRelays(ctx context.Context, count int) ([]types.NodeID, error) {
	if count <= 0 {
		count = DefaultDiscoveryLimit
	}

	// 首先返回缓存的中继
	rd.relaysMu.RLock()
	cached := make([]types.NodeID, 0, len(rd.relays))
	for id := range rd.relays {
		cached = append(cached, id)
		if len(cached) >= count {
			break
		}
	}
	rd.relaysMu.RUnlock()

	if len(cached) >= count {
		return cached[:count], nil
	}

	// 通过发现服务查找更多
	if rd.discovery != nil {
		ch, err := rd.discovery.DiscoverPeers(ctx, RelayNamespace)
		if err != nil {
			log.Debug("发现中继失败", "err", err)
		} else {
			for peer := range ch {
				if len(cached) >= count {
					break
				}

				// 转换地址
				addrs := make([]endpoint.Address, len(peer.Addrs))
				for i, addr := range peer.Addrs {
					addrs[i] = address.NewAddr(addr)
				}

				rd.relaysMu.Lock()
				rd.relays[peer.ID] = &discoveredRelay{
					ID:       peer.ID,
					Addrs:    addrs,
					LastSeen: time.Now(),
				}
				rd.relaysMu.Unlock()

				cached = append(cached, peer.ID)
			}
		}
	}

	return cached, nil
}

// Advertise 通告本节点为中继服务器
func (rd *RelayDiscovery) Advertise(ctx context.Context) error {
	if rd.discovery == nil {
		return nil
	}

	return rd.discovery.Announce(ctx, RelayNamespace)
}

// ============================================================================
//                              后台任务
// ============================================================================

// advertiseLoop 通告循环
func (rd *RelayDiscovery) advertiseLoop() {
	// 立即通告一次
	rd.Advertise(rd.ctx)

	ticker := time.NewTicker(rd.advertiseInterval)
	defer ticker.Stop()

	for {
		select {
		case <-rd.ctx.Done():
			return
		case <-ticker.C:
			if err := rd.Advertise(rd.ctx); err != nil {
				log.Debug("通告失败", "err", err)
			}
		}
	}
}

// discoveryLoop 发现循环
func (rd *RelayDiscovery) discoveryLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-rd.ctx.Done():
			return
		case <-ticker.C:
			rd.refreshRelays()
			rd.cleanupStale()
		}
	}
}

// refreshRelays 刷新中继列表
func (rd *RelayDiscovery) refreshRelays() {
	ctx, cancel := context.WithTimeout(rd.ctx, 30*time.Second)
	defer cancel()

	_, err := rd.FindRelays(ctx, DefaultDiscoveryLimit)
	if err != nil {
		log.Debug("刷新中继列表失败", "err", err)
	}
}

// cleanupStale 清理过期中继
func (rd *RelayDiscovery) cleanupStale() {
	rd.relaysMu.Lock()
	defer rd.relaysMu.Unlock()

	cutoff := time.Now().Add(-30 * time.Minute)
	for id, relay := range rd.relays {
		if relay.LastSeen.Before(cutoff) {
			delete(rd.relays, id)
			log.Debug("移除过期中继",
				"relay", id.ShortString())
		}
	}
}

// ============================================================================
//                              查询接口
// ============================================================================

// GetRelays 获取所有已知中继
func (rd *RelayDiscovery) GetRelays() []types.NodeID {
	rd.relaysMu.RLock()
	defer rd.relaysMu.RUnlock()

	relays := make([]types.NodeID, 0, len(rd.relays))
	for id := range rd.relays {
		relays = append(relays, id)
	}
	return relays
}

// GetRelayAddrs 获取中继地址
func (rd *RelayDiscovery) GetRelayAddrs(relayID types.NodeID) []endpoint.Address {
	rd.relaysMu.RLock()
	defer rd.relaysMu.RUnlock()

	if relay, ok := rd.relays[relayID]; ok {
		return relay.Addrs
	}
	return nil
}

// AddRelay 手动添加中继
func (rd *RelayDiscovery) AddRelay(relayID types.NodeID, addrs []endpoint.Address) {
	rd.relaysMu.Lock()
	defer rd.relaysMu.Unlock()

	rd.relays[relayID] = &discoveredRelay{
		ID:       relayID,
		Addrs:    addrs,
		LastSeen: time.Now(),
	}
}

// RemoveRelay 移除中继
func (rd *RelayDiscovery) RemoveRelay(relayID types.NodeID) {
	rd.relaysMu.Lock()
	defer rd.relaysMu.Unlock()

	delete(rd.relays, relayID)
}

// stringAddress 已删除，统一使用 address.Addr

// ============================================================================
//                              接口断言
// ============================================================================

var _ relayif.RelayDiscovery = (*RelayDiscovery)(nil)

