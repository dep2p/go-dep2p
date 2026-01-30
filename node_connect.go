package dep2p

import (
	"context"
	"fmt"
	"strings"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ════════════════════════════════════════════════════════════════════════════
//                              连接管理
// ════════════════════════════════════════════════════════════════════════════

// Connect 连接到目标节点
//
// 支持多种输入格式（自动检测）：
//  1. Full Address: /ip4/1.2.3.4/udp/4001/quic-v1/p2p/12D3KooW...
//  2. ConnectionTicket: dep2p://base58...
//  3. 纯 NodeID: 12D3KooW...（通过 DHT 发现）
//
// 连接策略（自动执行）：
//  1. 检查 Peerstore 缓存
//  2. 解析提供的地址
//  3. 尝试直连
//  4. 尝试 NAT 打洞
//  5. 回退到中继
//
// 身份验证：
//   - TLS/Noise 握手时自动验证目标身份
//   - 身份不匹配返回 ErrIdentityMismatch
//
// 示例：
//
//	// 使用完整地址连接
//	err := node.Connect(ctx, "/ip4/1.2.3.4/tcp/4001/p2p/12D3KooW...")
//
//	// 使用票据连接
//	err := node.Connect(ctx, "dep2p://5Hx3fK...")
//
//	// 仅使用 NodeID 连接（通过 DHT 发现）
//	err := node.Connect(ctx, "12D3KooW...")
func (n *Node) Connect(ctx context.Context, target string) error {
	n.mu.RLock()
	if !n.started {
		n.mu.RUnlock()
		return ErrNotStarted
	}
	n.mu.RUnlock()

	// 检测输入格式并路由到对应的处理方法
	if strings.HasPrefix(target, "dep2p://") {
		// ConnectionTicket 格式
		return n.connectByTicket(ctx, target)
	} else if strings.HasPrefix(target, "/") {
		// Multiaddr 格式
		return n.connectByMultiaddr(ctx, target)
	} else {
		// 纯 NodeID 格式
		return n.connectByNodeID(ctx, target)
	}
}

// Disconnect 断开与指定节点的连接
//
// 主动关闭与指定节点的所有连接。适用于：
//   - 安全防护：检测到恶意节点后断开
//   - 资源管理：释放空闲连接
//   - 运维操作：手动断开指定节点
//
// 参数:
//   - peerID: 要断开连接的节点 ID
//
// 返回:
//   - error: 如果节点未启动或断开失败
//
// 示例：
//
//	err := node.Disconnect("12D3KooWxxxxxxxx")
//	if err != nil {
//	    log.Printf("断开连接失败: %v", err)
//	}
func (n *Node) Disconnect(peerID string) error {
	if n.host == nil {
		return fmt.Errorf("node not started")
	}

	// 通过 host.Network() 获取 Swarm
	swarm := n.getSwarm()
	if swarm == nil {
		return fmt.Errorf("network layer not available")
	}

	return swarm.ClosePeer(peerID)
}

// DisconnectAll 断开所有连接
//
// 关闭与所有节点的连接。适用于：
//   - 节点关闭前的优雅清理
//   - 网络重置场景
//
// 示例：
//
//	err := node.DisconnectAll()
func (n *Node) DisconnectAll() error {
	if n.host == nil {
		return fmt.Errorf("node not started")
	}

	swarm := n.getSwarm()
	if swarm == nil {
		return fmt.Errorf("network layer not available")
	}

	peers := swarm.Peers()
	var lastErr error
	for _, peerID := range peers {
		if err := swarm.ClosePeer(peerID); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// ConnectedPeers 返回所有已连接节点的 ID 列表
//
// 返回当前与本节点保持活跃连接的所有节点 ID。
//
// 注意：这是 Node 级别的 API，返回底层网络连接；
// 与 Realm.Members() 不同，后者返回的是 Realm 成员（可能未直连）。
//
// 示例：
//
//	peers := node.ConnectedPeers()
//	fmt.Printf("已连接 %d 个节点\n", len(peers))
//	for _, peer := range peers {
//	    fmt.Printf("  - %s\n", peer[:16])
//	}
func (n *Node) ConnectedPeers() []string {
	if n.host == nil {
		return nil
	}

	swarm := n.getSwarm()
	if swarm == nil {
		return nil
	}

	return swarm.Peers()
}

// IsConnected 检查是否与指定节点连接
//
// 参数:
//   - peerID: 要检查的节点 ID
//
// 返回:
//   - bool: 如果已连接返回 true
//
// 示例：
//
//	if node.IsConnected("12D3KooWxxxxxxxx") {
//	    // 节点已连接
//	}
func (n *Node) IsConnected(peerID string) bool {
	if n.host == nil {
		return false
	}

	swarm := n.getSwarm()
	if swarm == nil {
		return false
	}

	return swarm.Connectedness(peerID) == pkgif.Connected
}

// GetPeerInfo 获取指定节点的连接信息
//
// 返回与指定节点的详细连接信息。如果未连接，返回 nil。
//
// 参数:
//   - peerID: 目标节点 ID
//
// 返回:
//   - *PeerConnectionInfo: 连接信息，如果未连接返回 nil
//
// 示例：
//
//	info := node.GetPeerInfo("12D3KooWxxxxxxxx")
//	if info != nil {
//	    fmt.Printf("方向: %s, 流数量: %d\n", info.Direction, info.NumStreams)
//	}
func (n *Node) GetPeerInfo(peerID string) *PeerConnectionInfo {
	if n.host == nil {
		return nil
	}

	swarm := n.getSwarm()
	if swarm == nil {
		return nil
	}

	conns := swarm.ConnsToPeer(peerID)
	if len(conns) == 0 {
		return nil
	}

	// 聚合所有连接的信息
	info := &PeerConnectionInfo{
		PeerID:   peerID,
		NumConns: len(conns),
	}

	// 收集所有地址和流数量
	addrSet := make(map[string]struct{})
	for _, conn := range conns {
		// 获取远程地址（使用 RemoteMultiaddr）
		if addr := conn.RemoteMultiaddr(); addr != nil {
			addrSet[addr.String()] = struct{}{}
		}

		// 累加流数量（使用 Stat().NumStreams）
		stat := conn.Stat()
		info.NumStreams += stat.NumStreams

		// 使用第一个连接的方向和时间
		if info.Direction == "" {
			switch stat.Direction {
			case pkgif.DirInbound:
				info.Direction = "inbound"
			case pkgif.DirOutbound:
				info.Direction = "outbound"
			default:
				info.Direction = "unknown"
			}

			// 获取连接时间（Opened 是 int64 时间戳）
			if stat.Opened > 0 {
				info.ConnectedAt = time.Unix(0, stat.Opened)
			}
		}
	}

	// 转换地址集合为切片
	info.Addrs = make([]string, 0, len(addrSet))
	for addr := range addrSet {
		info.Addrs = append(info.Addrs, addr)
	}

	return info
}

// ════════════════════════════════════════════════════════════════════════════
//                              内部连接方法
// ════════════════════════════════════════════════════════════════════════════

// getSwarm 获取 Swarm 实例
//
// 内部方法，通过类型断言从 Host 获取 Swarm。
func (n *Node) getSwarm() pkgif.Swarm {
	if n.host == nil {
		return nil
	}

	// 通过 host.Network() 获取 Swarm
	if hostImpl, ok := n.host.(interface{ Network() pkgif.Swarm }); ok {
		return hostImpl.Network()
	}

	return nil
}

// connectByMultiaddr 通过 Multiaddr 连接
//
// 解析完整地址（包含 /p2p/<NodeID>），提取节点 ID 和传输地址。
//
// 支持 Relay 地址（/p2p-circuit/）
// 当地址包含 /p2p-circuit/ 时，需要先将 Relay 服务器地址添加到 Peerstore，
// 否则 Swarm 拨号时找不到 Relay 服务器的地址会失败。
func (n *Node) connectByMultiaddr(ctx context.Context, addr string) error {
	// 解析 Multiaddr
	ma, err := types.NewMultiaddr(addr)
	if err != nil {
		return fmt.Errorf("invalid multiaddr: %w", err)
	}

	// 检测并处理 Relay 地址
	if strings.Contains(addr, "/p2p-circuit/") {
		return n.connectViaRelayAddr(ctx, addr, ma)
	}

	// 提取 NodeID 和传输地址
	addrInfo, err := types.AddrInfoFromP2pAddr(ma)
	if err != nil {
		return fmt.Errorf("extract addrinfo: %w", err)
	}

	// 转换为字符串列表
	nodeID := string(addrInfo.ID)
	addrs := make([]string, len(addrInfo.Addrs))
	for i, a := range addrInfo.Addrs {
		addrs[i] = a.String()
	}

	// 使用 Host.Connect 连接
	if n.host == nil {
		return fmt.Errorf("host not initialized")
	}

	return n.host.Connect(ctx, nodeID, addrs)
}

// connectViaRelayAddr 通过 Relay 地址连接
//
// 处理 /p2p-circuit/ 格式的 Relay 地址
// 地址格式：/ip4/x.x.x.x/udp/port/quic-v1/p2p/<RelayID>/p2p-circuit/p2p/<TargetID>
//
// 简化后的步骤（Swarm 层已正确处理 Relay 地址）：
//  1. 提取目标节点 ID
//  2. 将完整 Relay 地址添加到目标节点的 Peerstore
//  3. 调用 Host.Connect，让 Swarm.dialPeer 自动处理 Relay 地址
func (n *Node) connectViaRelayAddr(ctx context.Context, addr string, ma types.Multiaddr) error {
	if n.host == nil {
		return fmt.Errorf("host not initialized")
	}

	// 1. 提取目标节点 ID
	// 分割 Relay 地址：/relay-addr/p2p-circuit/p2p/target
	parts := strings.Split(addr, "/p2p-circuit/")
	if len(parts) != 2 {
		return fmt.Errorf("invalid relay address format")
	}

	targetPart := parts[1] // p2p/<TargetID>

	// 提取目标节点 ID
	targetID := ""
	if strings.HasPrefix(targetPart, "p2p/") {
		targetID = strings.TrimPrefix(targetPart, "p2p/")
	} else {
		return fmt.Errorf("invalid target in relay address: %s", targetPart)
	}

	// 2. 将完整 Relay 地址添加到目标节点的 Peerstore
	// Swarm.dialPeer 会通过 filterRelayAddrs 提取并使用这个地址
	if n.host.Peerstore() != nil {
		relayMA, err := types.NewMultiaddr(addr)
		if err != nil {
			return fmt.Errorf("invalid relay address: %w", err)
		}
		n.host.Peerstore().AddAddrs(types.PeerID(targetID), []types.Multiaddr{relayMA}, time.Hour)
	}

	// 3. 调用 Host.Connect，让 Swarm.dialPeer 自动处理
	// Swarm.dialPeer 流程：
	//   - 直连失败（无直连地址或失败）
	//   - 通过 Peerstore 中的 Relay 地址连接
	return n.host.Connect(ctx, targetID, nil)
}

// connectByTicket 通过 ConnectionTicket 连接
//
// 解码票据，提取节点 ID 和地址提示。
func (n *Node) connectByTicket(ctx context.Context, ticket string) error {
	// 解码票据
	t, err := types.DecodeConnectionTicket(ticket)
	if err != nil {
		return fmt.Errorf("decode ticket: %w", err)
	}

	// 检查票据是否过期
	if t.IsExpired(24 * time.Hour) {
		return fmt.Errorf("ticket expired")
	}

	// 如果有地址提示，直接使用
	// 根据设计文档 Section 8.3，应该先尝试直连，再回退到 Relay
	if len(t.AddressHints) > 0 {
		if n.host == nil {
			return fmt.Errorf("host not initialized")
		}
		var relayHints []string
		var directHints []string
		for _, hint := range t.AddressHints {
			if strings.Contains(hint, "/p2p-circuit/") {
				relayHints = append(relayHints, hint)
			} else {
				directHints = append(directHints, hint)
			}
		}

		var lastErr error

		// 1. 先尝试直连地址提示（符合设计文档流程）
		if len(directHints) > 0 {
			if err := n.host.Connect(ctx, t.NodeID, directHints); err == nil {
				return nil
			} else {
				lastErr = err
				logger.Debug("票据直连地址连接失败，尝试 Relay 地址",
					"nodeID", t.NodeID[:8],
					"error", err)
			}
		}

		// 2. 直连失败或无直连地址，尝试 Relay 地址提示
		for _, hint := range relayHints {
			ma, err := types.NewMultiaddr(hint)
			if err != nil {
				lastErr = err
				continue
			}
			if err := n.connectViaRelayAddr(ctx, hint, ma); err == nil {
				return nil
			} else {
				lastErr = err
			}
		}

		if lastErr != nil {
			return lastErr
		}
	}

	// 否则回退到 NodeID 发现
	return n.connectByNodeID(ctx, t.NodeID)
}

// connectByNodeID 通过 NodeID 连接
//
// 使用 DHT 发现节点地址，然后连接。
func (n *Node) connectByNodeID(ctx context.Context, nodeID string) error {
	// 1. 检查 Peerstore 缓存
	if n.host != nil && n.host.Peerstore() != nil {
		peerID := types.PeerID(nodeID)
		addrs := n.host.Peerstore().Addrs(peerID)

		if len(addrs) > 0 {
			// 转换为字符串列表
			addrStrs := make([]string, len(addrs))
			for i, a := range addrs {
				addrStrs[i] = a.String()
			}

			// 尝试使用缓存的地址连接
			err := n.host.Connect(ctx, nodeID, addrStrs)
			if err == nil {
				return nil
			}
			// 缓存地址连接失败，继续尝试 DHT 发现
		}
	}

	// 2. 使用 DHT 发现节点
	if n.discovery == nil {
		return fmt.Errorf("discovery not available, cannot find node by ID")
	}

	// 类型断言为 DHTDiscovery
	dhtDiscovery, ok := n.discovery.(pkgif.DHTDiscovery)
	if !ok {
		return fmt.Errorf("discovery service does not support FindPeer (DHT required)")
	}

	// 通过 DHT 发现节点
	peerID := types.PeerID(nodeID)
	peerInfo, err := dhtDiscovery.FindPeer(ctx, peerID)
	if err != nil {
		return fmt.Errorf("dht find peer: %w", err)
	}

	if len(peerInfo.Addrs) == 0 {
		return fmt.Errorf("no addresses found for peer %s", nodeID)
	}

	// 转换为字符串列表
	addrStrs := peerInfo.AddrsToStrings()

	// 3. 连接到发现的地址
	if n.host == nil {
		return fmt.Errorf("host not initialized")
	}

	return n.host.Connect(ctx, nodeID, addrStrs)
}
