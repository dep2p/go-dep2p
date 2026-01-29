// Package client 提供中继客户端实现
package client

import (
	"context"
	"strings"
	"sync"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/protocol"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// RelayNamespace 中继发现命名空间
// 使用 pkg/protocol 中的统一定义
var RelayNamespace = protocol.RelayNamespace

var relayClientLogger = log.Logger("relay/client")

// RelayClientService 中继客户端服务
//
// 实现 pkgif.RelayClient 接口，用于与中继服务器通信。
type RelayClientService struct {
	swarm     pkgif.Swarm
	peerstore pkgif.Peerstore
	discovery pkgif.Discovery

	// 客户端缓存
	clients   map[string]*Client
	clientsMu sync.RWMutex
}

// NewRelayClientService 创建中继客户端服务
func NewRelayClientService(swarm pkgif.Swarm, peerstore pkgif.Peerstore, discovery pkgif.Discovery) *RelayClientService {
	return &RelayClientService{
		swarm:     swarm,
		peerstore: peerstore,
		discovery: discovery,
		clients:   make(map[string]*Client),
	}
}

// Reserve 在中继节点上预留资源
func (s *RelayClientService) Reserve(ctx context.Context, relayID string) (pkgif.Reservation, error) {
	// 获取或创建客户端
	client, err := s.getOrCreateClient(relayID)
	if err != nil {
		return nil, err
	}

	// 预留
	reservation, err := client.Reserve(ctx)
	if err != nil {
		return nil, err
	}

	// 构建中继地址
	localID := s.swarm.LocalPeer()
	var circuitAddrs []string

	// 从 peerstore 获取中继节点地址
	if s.peerstore != nil {
		relayAddrs := s.peerstore.Addrs(types.PeerID(relayID))
		for _, addr := range relayAddrs {
			addrStr := addr.String()
			// 过滤不可连接的地址（如 0.0.0.0、::、127.0.0.1 等）
			if !isConnectableRelayAddr(addrStr) {
				continue
			}
			circuitAddr := buildCircuitAddr(addrStr+"/p2p/"+relayID, localID)
			circuitAddrs = append(circuitAddrs, circuitAddr)
		}
	}

	// 包装为 Reservation 接口
	return &reservationWrapper{
		relayPeer:  types.PeerID(relayID),
		expireTime: reservation.ExpireTime,
		addrs:      circuitAddrs,
		client:     client,
	}, nil
}

// FindRelays 发现可用的中继服务器
func (s *RelayClientService) FindRelays(ctx context.Context) ([]string, error) {
	if s.discovery == nil {
		return nil, nil
	}

	// 通过 DHT 查找中继服务
	// v2.0 统一命名空间：使用 RelayNamespace 常量
	peers, err := s.discovery.FindPeers(ctx, RelayNamespace)
	if err != nil {
		relayClientLogger.Debug("发现中继失败", "err", err)
		return nil, err
	}

	var relayIDs []string
	for peer := range peers {
		relayIDs = append(relayIDs, string(peer.ID))
	}

	return relayIDs, nil
}

// getOrCreateClient 获取或创建客户端
func (s *RelayClientService) getOrCreateClient(relayID string) (*Client, error) {
	s.clientsMu.RLock()
	client, exists := s.clients[relayID]
	s.clientsMu.RUnlock()

	if exists {
		return client, nil
	}

	// 获取中继地址
	var relayAddr types.Multiaddr
	if s.peerstore != nil {
		addrs := s.peerstore.Addrs(types.PeerID(relayID))
		if len(addrs) > 0 {
			relayAddr = addrs[0]
		}
	}

	// 创建客户端
	client = NewClient(s.swarm, types.PeerID(relayID), relayAddr)

	s.clientsMu.Lock()
	s.clients[relayID] = client
	s.clientsMu.Unlock()

	return client, nil
}

// Close 关闭所有客户端
func (s *RelayClientService) Close() error {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()

	for _, client := range s.clients {
		client.Close()
	}
	s.clients = make(map[string]*Client)

	return nil
}

// ============================================================================
//                              Health Check
// ============================================================================

// HealthChecker 中继健康检查器
type HealthChecker struct {
	client       pkgif.RelayClient
	checkTimeout time.Duration
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(client pkgif.RelayClient) *HealthChecker {
	return &HealthChecker{
		client:       client,
		checkTimeout: 10 * time.Second,
	}
}

// Check 检查中继健康状态
func (h *HealthChecker) Check(ctx context.Context, relayID string) (bool, error) {
	checkCtx, cancel := context.WithTimeout(ctx, h.checkTimeout)
	defer cancel()

	// 尝试预留来检查健康
	reservation, err := h.client.Reserve(checkCtx, relayID)
	if err != nil {
		return false, err
	}

	// 检查预留是否有效
	if reservation.Expiry() <= time.Now().Unix() {
		return false, nil
	}

	return true, nil
}

// ============================================================================
//                              辅助函数
// ============================================================================

// isConnectableRelayAddr 判断 Relay 地址是否是可连接的
//
// 过滤掉以下不可连接的地址：
//   - 0.0.0.0 (IPv4 未指定地址，用于监听但不可连接)
//   - :: (IPv6 未指定地址)
//   - 127.0.0.1 / ::1 (回环地址，只能本机访问)
func isConnectableRelayAddr(addr string) bool {
	if addr == "" {
		return false
	}

	// 不可连接的地址模式
	unconnectablePatterns := []string{
		"/ip4/0.0.0.0/",
		"/ip6/::/",
		"/ip4/127.0.0.1/",
		"/ip4/127.",
		"/ip6/::1/",
	}

	for _, pattern := range unconnectablePatterns {
		if strings.Contains(addr, pattern) {
			return false
		}
	}

	return true
}

// ============================================================================
//                              接口断言
// ============================================================================

var _ pkgif.RelayClient = (*RelayClientService)(nil)
