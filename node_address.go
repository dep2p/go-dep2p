package dep2p

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ════════════════════════════════════════════════════════════════════════════
//                              地址管理
// ════════════════════════════════════════════════════════════════════════════

// ListenAddrs 返回监听地址
//
// 返回节点正在监听的所有本地地址。
// 注意：不包含外部地址和中继地址，如需对外公告请使用 AdvertisedAddrs()。
func (n *Node) ListenAddrs() []string {
	if n.host == nil {
		return nil
	}
	return n.host.Addrs()
}

// AdvertisedAddrs 返回对外公告地址
//
// 返回应该对外公告的地址列表，包括：
//  1. 用户配置的公网地址（WithPublicAddr）
//  2. 已验证的公网直连地址（ShareableAddrs）
//  3. 如果配置了 Relay，包含中继电路地址
//
// 中继地址格式：
//
//	{relay-addr}/p2p-circuit/p2p/{local-id}
//	例如：/ip4/relay.example.com/tcp/4001/p2p/QmRelay/p2p-circuit/p2p/QmLocal
//
// 其他节点可以使用中继地址通过 Relay 连接到本节点。
//
// 示例：
//
//	addrs := node.AdvertisedAddrs()
//	for _, addr := range addrs {
//	    fmt.Println("公告地址:", addr)
//	}
func (n *Node) AdvertisedAddrs() []string {
	nodeID := n.ID()
	if nodeID == "" {
		return nil
	}

	var result []string
	seen := make(map[string]struct{})

	// 1. 添加用户配置的公网地址（WithPublicAddr 设置的地址）
	//    这些地址用于云服务器场景：节点监听 0.0.0.0，但对外公告公网 IP
	//    优先级最高，因为是用户明确配置的
	if n.config != nil && len(n.config.advertiseAddrs) > 0 {
		for _, addr := range n.config.advertiseAddrs {
			// 如果地址不包含 /p2p/ 后缀，添加本节点 ID
			if !strings.Contains(addr, "/p2p/") {
				addr = addr + "/p2p/" + nodeID
			}
			if _, ok := seen[addr]; !ok {
				seen[addr] = struct{}{}
				result = append(result, addr)
			}
		}
	}

	// 2. 从 Host/Reachability Coordinator 获取地址
	//    包括：已验证的直连地址 + Relay 地址 + 监听地址
	if n.host != nil {
		// 优先使用 Host.AdvertisedAddrs()，它整合了 Coordinator 的地址
		if hostAddrs := n.host.AdvertisedAddrs(); len(hostAddrs) > 0 {
			for _, addr := range hostAddrs {
				if _, ok := seen[addr]; !ok {
					seen[addr] = struct{}{}
					result = append(result, addr)
				}
			}
		}
	}

	// 3. 从 NAT Service 获取外部地址（兼容旧逻辑）
	if n.natService != nil {
		for _, addr := range n.natService.ExternalAddrs() {
			fullAddr := addr
			if !strings.Contains(addr, "/p2p/") {
				fullAddr = addr + "/p2p/" + nodeID
			}
			if _, ok := seen[fullAddr]; !ok {
				seen[fullAddr] = struct{}{}
				result = append(result, fullAddr)
			}
		}
	}

	// 4. 如果配置了 Relay，添加中继电路地址
	if n.relayManager != nil {
		relayAddr, hasRelay := n.relayManager.RelayAddr()
		if hasRelay && relayAddr != nil {
			// 生成中继电路地址：{relay-addr}/p2p-circuit/p2p/{local-id}
			circuitAddr := buildCircuitAddr(relayAddr.String(), nodeID)
			if circuitAddr != "" {
				if _, ok := seen[circuitAddr]; !ok {
					seen[circuitAddr] = struct{}{}
					result = append(result, circuitAddr)
				}
			}
		}
	}

	if len(result) == 0 {
		return nil
	}

	return result
}

// ShareableAddrs 返回可分享的完整地址
//
// 严格语义（继承自 go-dep2p-main）：
//   - 仅返回已验证的公网直连地址（VerifiedDirect）
//   - 返回 Full Address 格式（包含 /p2p/<NodeID>）
//   - 过滤掉私网/回环/link-local 地址
//   - 无验证地址时返回 nil
//
// 用途：
//   - DHT 发布
//   - 分享给其他用户
//   - 作为引导节点地址
//
// 示例：
//
//	addrs := node.ShareableAddrs()
//	for _, addr := range addrs {
//	    fmt.Println("可分享地址:", addr)
//	}
func (n *Node) ShareableAddrs() []string {
	// 获取本地节点 ID
	nodeID := n.ID()
	if nodeID == "" {
		return nil
	}

	var result []string
	seen := make(map[string]struct{})

	// 1. 优先从 Host/Reachability Coordinator 获取已验证的公网地址
	if n.host != nil {
		if shareableAddrs := n.host.ShareableAddrs(); len(shareableAddrs) > 0 {
			for _, addr := range shareableAddrs {
				if _, ok := seen[addr]; !ok {
					seen[addr] = struct{}{}
					result = append(result, addr)
				}
			}
		}
	}

	// 2. 从 NAT Service 获取外部地址（兼容旧逻辑）
	if n.natService != nil {
		for _, addr := range n.natService.ExternalAddrs() {
			// 过滤非公网地址
			if !isPublicAddr(addr) {
				continue
			}

			// 添加 /p2p/<NodeID> 后缀
			fullAddr := addr
			if !strings.Contains(addr, "/p2p/") {
				fullAddr = addr + "/p2p/" + nodeID
			}

			if _, ok := seen[fullAddr]; !ok {
				seen[fullAddr] = struct{}{}
				result = append(result, fullAddr)
			}
		}
	}

	// 3. 用户配置的公网地址也可作为可分享地址
	if n.config != nil && len(n.config.advertiseAddrs) > 0 {
		for _, addr := range n.config.advertiseAddrs {
			// 过滤非公网地址
			if !isPublicAddr(addr) {
				continue
			}

			fullAddr := addr
			if !strings.Contains(addr, "/p2p/") {
				fullAddr = addr + "/p2p/" + nodeID
			}

			if _, ok := seen[fullAddr]; !ok {
				seen[fullAddr] = struct{}{}
				result = append(result, fullAddr)
			}
		}
	}

	if len(result) == 0 {
		return nil
	}

	return result
}

// WaitShareableAddrs 等待可分享地址就绪
//
// 典型用途：创世节点/引导节点启动后等待地址验证完成。
//
// 示例：
//
//	// 启动引导节点
//	node.Start(ctx)
//
//	// 等待地址就绪
//	addrs, err := node.WaitShareableAddrs(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 将地址添加到配置文件
//	saveBootstrapAddrs(addrs)
func (n *Node) WaitShareableAddrs(ctx context.Context) ([]string, error) {
	// 设置超时
	const (
		maxWait       = 30 * time.Second
		checkInterval = 500 * time.Millisecond
	)

	deadline := time.After(maxWait)
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()

		case <-deadline:
			return nil, fmt.Errorf("timeout waiting for shareable addresses")

		case <-ticker.C:
			addrs := n.ShareableAddrs()
			if len(addrs) > 0 {
				return addrs, nil
			}
		}
	}
}

// BootstrapCandidates 返回候选地址（旁路，不入 DHT）
//
// 与 ShareableAddrs 正交：
//   - ShareableAddrs: 严格验证，可入 DHT
//   - BootstrapCandidates: 旁路，包含所有候选（直连+监听+中继）
//
// 用途：
//   - 人工分享/跨设备冷启动
//   - 提供备选地址（即使未验证）
//
// 返回类型：
//   - BootstrapCandidateDirect: 直连地址
//   - BootstrapCandidateRelay: 中继电路地址
func (n *Node) BootstrapCandidates() []types.BootstrapCandidate {
	var candidates []types.BootstrapCandidate

	nodeID := n.ID()
	if nodeID == "" {
		return nil
	}

	// 1. 添加已验证的公网地址（Direct）
	shareableAddrs := n.ShareableAddrs()
	if len(shareableAddrs) > 0 {
		candidates = append(candidates, types.BootstrapCandidate{
			NodeID: nodeID,
			Addrs:  shareableAddrs,
			Type:   types.BootstrapCandidateDirect,
		})
	}

	// 2. 添加所有监听地址（作为候选）
	listenAddrs := n.ListenAddrs()
	if len(listenAddrs) > 0 {
		// 过滤掉已包含在 shareable 中的地址
		var uniqueAddrs []string
		for _, addr := range listenAddrs {
			if !containsAddr(shareableAddrs, addr) {
				uniqueAddrs = append(uniqueAddrs, addr)
			}
		}

		if len(uniqueAddrs) > 0 {
			candidates = append(candidates, types.BootstrapCandidate{
				NodeID: nodeID,
				Addrs:  uniqueAddrs,
				Type:   types.BootstrapCandidateDirect,
			})
		}
	}

	// 3. 添加中继电路地址（Relay）
	if n.relayManager != nil {
		relayAddr, hasRelay := n.relayManager.RelayAddr()
		if hasRelay && relayAddr != nil {
			circuitAddr := buildCircuitAddr(relayAddr.String(), nodeID)
			if circuitAddr != "" {
				candidates = append(candidates, types.BootstrapCandidate{
					NodeID: nodeID,
					Addrs:  []string{circuitAddr},
					Type:   types.BootstrapCandidateRelay,
				})
			}
		}
	}

	return candidates
}

// ConnectionTicket 返回用户友好的连接票据
//
// 格式：dep2p://base64url(...)
// 便于通过聊天/二维码分享。
//
// 票据包含的地址优先级：
//  1. ShareableAddrs（已验证的外部地址，不包含 0.0.0.0）
//  2. AdvertisedAddrs（直连+中继地址）
//
// 注意：如果没有可分享的地址，票据中将只包含 NodeID，
// 连接时需要依赖 DHT 或其他发现机制。
//
// 示例：
//
//	ticket := node.ConnectionTicket()
//	fmt.Println("分享此票据给其他用户:", ticket)
//
//	// 其他节点使用票据连接
//	err := otherNode.Connect(ctx, ticket)
func (n *Node) ConnectionTicket() string {
	nodeID := n.ID()
	if nodeID == "" {
		return ""
	}

	// 获取地址提示（优先使用 ShareableAddrs，已过滤 0.0.0.0 等不可连接地址）
	var addressHints []string
	if shareableAddrs := n.ShareableAddrs(); len(shareableAddrs) > 0 {
		addressHints = shareableAddrs
	} else if advertisedAddrs := n.AdvertisedAddrs(); len(advertisedAddrs) > 0 {
		// 回退到 AdvertisedAddrs（可能包含 Relay 地址）
		// 但需要过滤掉无效地址
		for _, addr := range advertisedAddrs {
			if !strings.Contains(addr, "/0.0.0.0/") &&
				!strings.Contains(addr, "/::/") &&
				!strings.Contains(addr, "/127.0.0.1/") {
				addressHints = append(addressHints, addr)
			}
		}
	}

	// 如果没有有效地址，返回空字符串
	// 票据没有可连接地址是没有意义的
	if len(addressHints) == 0 {
		return ""
	}

	// 创建票据
	ticket := types.NewConnectionTicket(nodeID, addressHints)

	// 编码
	encoded, err := ticket.Encode()
	if err != nil {
		return "" // 编码失败，返回空字符串
	}

	return encoded
}
