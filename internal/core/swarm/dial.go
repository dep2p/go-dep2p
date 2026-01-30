package swarm

import (
	"context"
	"fmt"
	"strings"
	"time"

	relayclient "github.com/dep2p/go-dep2p/internal/core/relay/client"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/multiaddr"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// OPT-2 优化：连续拨号失败阈值
// 超过此阈值后，后续失败日志降为 DEBUG 级别，减少日志噪音
const dialFailureLogThreshold = 3

// ============================================================================
//                              Relay 地址退避机制
// ============================================================================

// Relay 地址退避参数
const (
	// 基础退避时间
	relayBackoffBase = 10 * time.Second
	// 最大退避时间
	relayBackoffMax = 10 * time.Minute
	// 退避记录过期时间
	relayBackoffExpiry = 30 * time.Minute
)

// relayBackoffEntry Relay 地址退避条目
type relayBackoffEntry struct {
	failures    int       // 连续失败次数
	nextRetry   time.Time // 下次允许重试时间
	lastFailure time.Time // 最后一次失败时间
	lastError   string    // 最后一次错误信息
}

// makeRelayAddrKey 生成 Relay 地址退避键
// 格式: peerID:relayServerID
func makeRelayAddrKey(peerID, relayAddr string) string {
	// 从 Relay 地址中提取 Relay 服务器 ID
	// 地址格式: /ip4/.../p2p/<relayID>/p2p-circuit/p2p/<targetID>
	parts := strings.Split(relayAddr, "/p2p-circuit/")
	if len(parts) > 0 {
		// 提取 Relay 服务器部分
		relayPart := parts[0]
		// 从中提取 peerID
		if idx := strings.LastIndex(relayPart, "/p2p/"); idx >= 0 {
			relayID := relayPart[idx+5:]
			if len(relayID) > 8 {
				relayID = relayID[:8]
			}
			return peerID + ":" + relayID
		}
	}
	return peerID + ":" + relayAddr
}

// isRelayAddrInBackoff 检查 Relay 地址是否在退避期内
func (s *Swarm) isRelayAddrInBackoff(peerID, relayAddr string) bool {
	key := makeRelayAddrKey(peerID, relayAddr)
	v, ok := s.relayAddrBackoff.Load(key)
	if !ok {
		return false
	}
	entry := v.(*relayBackoffEntry)
	return time.Now().Before(entry.nextRetry)
}

// recordRelayAddrFailure 记录 Relay 地址连接失败
func (s *Swarm) recordRelayAddrFailure(peerID, relayAddr, errStr string) {
	key := makeRelayAddrKey(peerID, relayAddr)
	now := time.Now()

	var entry *relayBackoffEntry
	if v, ok := s.relayAddrBackoff.Load(key); ok {
		entry = v.(*relayBackoffEntry)
	} else {
		entry = &relayBackoffEntry{}
	}

	entry.failures++
	entry.lastFailure = now
	entry.lastError = errStr

	// 计算退避时间：base * 2^(failures-1)，最大不超过 max
	backoff := relayBackoffBase * time.Duration(1<<min(entry.failures-1, 6))
	if backoff > relayBackoffMax {
		backoff = relayBackoffMax
	}
	entry.nextRetry = now.Add(backoff)

	s.relayAddrBackoff.Store(key, entry)

	// 只在首次失败或达到特定阈值时输出日志
	if entry.failures == 1 || entry.failures == 5 || entry.failures%20 == 0 {
		logger.Debug("Relay 地址退避已更新",
			"peerID", truncateID(peerID, 8),
			"failures", entry.failures,
			"backoff", backoff,
			"error", errStr)
	}
}

// clearRelayAddrBackoff 清除 Relay 地址退避（连接成功时调用）
func (s *Swarm) clearRelayAddrBackoff(peerID, relayAddr string) {
	key := makeRelayAddrKey(peerID, relayAddr)
	if v, ok := s.relayAddrBackoff.LoadAndDelete(key); ok {
		entry := v.(*relayBackoffEntry)
		if entry.failures > 0 {
			logger.Debug("清除 Relay 地址退避（连接成功）",
				"peerID", truncateID(peerID, 8),
				"previousFailures", entry.failures)
		}
	}
}

// cleanupRelayAddrBackoff 清理过期的 Relay 地址退避记录
func (s *Swarm) cleanupRelayAddrBackoff() {
	now := time.Now()
	s.relayAddrBackoff.Range(func(key, value interface{}) bool {
		entry := value.(*relayBackoffEntry)
		if now.Sub(entry.lastFailure) > relayBackoffExpiry {
			s.relayAddrBackoff.Delete(key)
		}
		return true
	})
}

// truncateID 安全截取 ID 用于日志显示
func truncateID(id string, maxLen int) string {
	if len(id) <= maxLen {
		return id
	}
	return id[:maxLen]
}

// OPT-2: getDialFailures 获取指定节点的连续拨号失败次数
func (s *Swarm) getDialFailures(peerID string) int {
	if v, ok := s.dialFailures.Load(peerID); ok {
		return v.(int)
	}
	return 0
}

// OPT-2: incrementDialFailures 增加拨号失败次数并返回当前值
func (s *Swarm) incrementDialFailures(peerID string) int {
	// 最多尝试 10 次 CAS，避免高并发下无限循环
	const maxRetries = 10
	for i := 0; i < maxRetries; i++ {
		oldVal := s.getDialFailures(peerID)
		newVal := oldVal + 1
		if s.dialFailures.CompareAndSwap(peerID, oldVal, newVal) {
			return newVal
		}
		// 如果是第一次失败，需要用 Store
		if oldVal == 0 {
			s.dialFailures.Store(peerID, 1)
			return 1
		}
	}
	// 降级：直接存储，接受可能的计数不精确
	s.dialFailures.Store(peerID, 1)
	return 1
}

// OPT-2: resetDialFailures 重置拨号失败次数（连接成功时调用）
func (s *Swarm) resetDialFailures(peerID string) {
	s.dialFailures.Delete(peerID)
}

// dialResult 拨号结果
type dialResult struct {
	conn pkgif.Connection
	err  error
}

// dialPeer 拨号连接到指定节点（完整实现）
//
// 实现惰性中继策略 + HolePunch 打洞（符合设计文档 Section 8.3）：
//  1. 检查已有连接（复用）
//  2. 尝试直连（从 Peerstore 获取 directAddrs）
//  3. ★ 直连失败 → 尝试通过 Peerstore 中的 Relay 地址连接
//     - 成功建立 Relay 连接后，如果配置了 HolePuncher，尝试打洞升级
//     - 打洞成功返回直连，失败返回 Relay 连接
//  4. 尝试通过预配置的 relayDialer 进行 HolePunch 打洞
//     - 先建立 Relay 连接（作为信令通道）
//     - 在 Relay 连接上尝试打洞
//     - 成功返回直连，失败返回 Relay 连接
//  5. 回退到预配置的 relayDialer（纯中继）
//  6. 返回连接或失败
//
// 中继对用户完全透明，用户无需关心底层使用直连还是中继。
func (s *Swarm) dialPeer(ctx context.Context, peerID string) (pkgif.Connection, error) {
	if s.closed.Load() {
		return nil, ErrSwarmClosed
	}

	peerShort := truncateID(peerID, 8)

	// 1. 检查是否已有连接（复用）
	if conns := s.ConnsToPeer(peerID); len(conns) > 0 {
		return conns[0], nil
	}

	// 检查是否拨号自己
	if peerID == s.localPeer {
		logger.Warn("尝试拨号自己", "peerID", peerShort)
		return nil, ErrDialToSelf
	}

	// 2. 尝试直连
	var directErr error

	// 从 PeerStore 获取地址
	var addrs []string
	if s.peerstore != nil {
		maddrs := s.peerstore.Addrs(types.PeerID(peerID))
		for _, ma := range maddrs {
			addrs = append(addrs, ma.String())
		}
	} else {
		logger.Debug("PeerStore 不可用，无法获取地址", "peerID", peerShort)
	}

	// 过滤出非中继地址用于直连
	directAddrs := filterDirectAddrs(addrs)

	if len(directAddrs) > 0 {
		// 记录原始地址（用于调试）
		logger.Debug("原始地址列表", "peerID", peerShort, "addrs", directAddrs)

		// 地址排序（优先级）
		directAddrs = rankAddrs(directAddrs)
		logger.Debug("地址排序完成", "peerID", peerShort, "order", directAddrs)

		// Phase 0 修复：使用路径健康管理器进一步优化地址排序
		directAddrs = s.rankAddrsWithHealth(peerID, directAddrs)

		logger.Info("尝试直连", "peerID", peerShort, "addrCount", len(directAddrs), "firstAddr", directAddrs[0])

		// 并发拨号
		conn, err := s.dialWorker(ctx, peerID, directAddrs)
		if err == nil {
			logger.Info("直连成功", "peerID", peerShort, "remoteAddr", conn.RemoteMultiaddr())
			s.resetDialFailures(peerID) // OPT-2: 成功时重置失败计数
			return conn, nil
		}
		// 直连失败是 NAT 环境下的预期行为，后续会尝试 Relay，降级到 DEBUG
		logger.Debug("直连失败（将尝试 Relay 回退）", "peerID", peerShort, "error", err, "triedAddrs", len(directAddrs))
		directErr = err
	} else {
		// 无直连地址在 NAT 环境下是预期的，后续会尝试 Relay
		logger.Debug("无可用直连地址（将尝试 Relay 回退）", "peerID", peerShort)
		directErr = ErrNoAddresses
	}

	// 获取 HolePuncher 和 RelayDialer（在多个地方使用）
	s.mu.RLock()
	holePuncher := s.holePuncher
	relayDialer := s.relayDialer
	s.mu.RUnlock()

	// 3. ★ 尝试通过 Peerstore 中的 Relay 地址连接
	//
	// 当用户提供 /p2p-circuit/ 地址时，这些地址会被添加到 Peerstore。
	// 直连失败后，优先使用这些显式提供的 Relay 地址，
	// 而不是依赖预配置的 relayDialer。
	//
	// 根据设计文档 Section 8.3，Relay 连接建立后应尝试打洞升级。
	// 排除以自己为中继的地址
	relayAddrs := s.filterRelayAddrsExcludeSelf(addrs)
	if len(relayAddrs) > 0 {
		logger.Info("尝试通过 Peerstore 中的 Relay 地址连接",
			"peerID", peerShort,
			"relayAddrCount", len(relayAddrs))

		skippedBackoff := 0
		for _, relayAddr := range relayAddrs {
			// 检查 Relay 地址是否在退避期内
			if s.isRelayAddrInBackoff(peerID, relayAddr) {
				skippedBackoff++
				continue
			}

			relayConn, err := s.dialViaRelayAddr(ctx, peerID, relayAddr)
			if err == nil {
				// 成功时清除退避
				s.clearRelayAddrBackoff(peerID, relayAddr)
				logger.Info("通过 Peerstore Relay 地址连接成功",
					"peerID", peerShort,
					"relayAddr", relayAddr)
				s.addConn(relayConn)
				s.notifyConnected(relayConn)

				// ★ 根据设计文档 Section 8.3：Relay 连接建立后应尝试打洞升级
				// 如果配置了 HolePuncher，尝试在 Relay 连接上进行打洞
				if holePuncher != nil && !holePuncher.IsActive(peerID) {
					logger.Info("Relay 连接已建立，尝试打洞升级",
						"peerID", peerShort)

					holePunchErr := holePuncher.DirectConnect(ctx, peerID, nil)
					if holePunchErr == nil {
						// 打洞成功，检查是否建立了直连
						if directConns := s.getDirectConns(peerID); len(directConns) > 0 {
							logger.Info("打洞成功，已升级为直连",
								"peerID", peerShort,
								"directAddr", directConns[0].RemoteMultiaddr())
							s.resetDialFailures(peerID) // OPT-2: 成功时重置失败计数
							return directConns[0], nil
						}
					}
					logger.Debug("打洞失败或无直连，使用 Relay 连接",
						"peerID", peerShort,
						"error", holePunchErr)
				}

				// 打洞失败或未配置 HolePuncher，返回 Relay 连接
				s.resetDialFailures(peerID) // OPT-2: 成功时重置失败计数
				return relayConn, nil
			}
			// 记录失败并更新退避
			s.recordRelayAddrFailure(peerID, relayAddr, err.Error())
			logger.Debug("通过 Relay 地址连接失败，尝试下一个",
				"peerID", peerShort,
				"relayAddr", relayAddr,
				"error", err)
		}

		// 统计实际尝试和跳过的数量
		triedCount := len(relayAddrs) - skippedBackoff
		if triedCount > 0 || skippedBackoff == 0 {
			logger.Warn("所有 Peerstore Relay 地址都失败",
				"peerID", peerShort,
				"triedCount", triedCount,
				"skippedBackoff", skippedBackoff)
		} else if skippedBackoff > 0 {
			// 所有地址都在退避期内
			logger.Debug("所有 Relay 地址在退避期内，跳过",
				"peerID", peerShort,
				"skippedCount", skippedBackoff)
		}
	}

	// 4. 尝试 HolePunch 打洞（通过预配置的 relayDialer）
	// 直连失败后，如果配置了 HolePuncher 且有预配置的 Relay，
	// 先尝试通过打洞协议建立直连。
	//
	// 打洞不依赖 Peerstore 的 directAddrs，
	// 而是通过 CONNECT 消息交换双方的 ShareableAddrs（观测地址）

	// 检查是否有可用的预配置 Relay
	hasRelay := relayDialer != nil && relayDialer.HasRelay()

	if holePuncher != nil && hasRelay {
		// 检查是否正在打洞（防止递归）
		if !holePuncher.IsActive(peerID) {
			logger.Info("直连失败，尝试通过预配置 Relay 进行 HolePunch 打洞",
				"peerID", peerShort,
				"directAddrsCount", len(directAddrs))

			// 先建立中继连接（用于协商打洞）
			relayConn, err := relayDialer.DialViaRelay(ctx, peerID)
			if err == nil && relayConn != nil {
				logger.Info("中继连接已建立，开始打洞协商",
					"peerID", peerShort,
					"relayAddr", relayConn.RemoteMultiaddr())
				s.addConn(relayConn)
				s.notifyConnected(relayConn)

				// 打洞不依赖 directAddrs，传 nil
				// 打洞协议会通过 CONNECT 消息交换双方的 ShareableAddrs
				holePunchErr := holePuncher.DirectConnect(ctx, peerID, nil)
				if holePunchErr == nil {
					// 打洞成功，检查是否建立了直连
					if directConns := s.getDirectConns(peerID); len(directConns) > 0 {
						logger.Info("HolePunch 打洞成功，已升级为直连",
							"peerID", peerShort,
							"directAddr", directConns[0].RemoteMultiaddr())
						s.resetDialFailures(peerID) // OPT-2: 成功时重置失败计数
						return directConns[0], nil
					}
				}
				logger.Info("HolePunch 打洞失败，使用中继连接",
					"peerID", peerShort,
					"error", holePunchErr)

				// 打洞失败，但中继连接已建立，直接返回中继连接
				s.resetDialFailures(peerID) // OPT-2: 成功时重置失败计数
				return relayConn, nil
			}
			logger.Warn("中继连接也失败", "peerID", peerShort, "error", err)
		} else {
			logger.Debug("已在打洞中，跳过重复尝试", "peerID", peerShort)
		}
	}

	// 5. Relay 惰性回退（预配置的 relayDialer）
	//
	// 如果没有配置 HolePuncher，或打洞流程跳过，
	// 直接尝试中继回退。
	if hasRelay {
		// 检查是否已有中继连接（可能在打洞流程中已建立）
		if conns := s.ConnsToPeer(peerID); len(conns) > 0 {
			logger.Debug("复用已有连接（可能是中继）", "peerID", peerShort)
			return conns[0], nil
		}

		logger.Info("直连失败，尝试预配置 Relay 回退", "peerID", peerShort)
		conn, err := relayDialer.DialViaRelay(ctx, peerID)
		if err == nil && conn != nil {
			// Relay 连接成功，添加到连接池
			logger.Info("预配置 Relay 连接成功", "peerID", peerShort, "relayed", true)
			s.addConn(conn)
			s.notifyConnected(conn)
			s.resetDialFailures(peerID) // OPT-2: 成功时重置失败计数
			return conn, nil
		}
		// OPT-2 优化：根据连续失败次数决定日志级别
		failures := s.incrementDialFailures(peerID)
		if failures > dialFailureLogThreshold {
			logger.Debug("预配置 Relay 回退失败（连续失败中）",
				"peerID", peerShort,
				"failures", failures,
				"error", err)
		} else {
			logger.Warn("预配置 Relay 回退也失败", "peerID", peerShort, "error", err)
		}
		// Relay 也失败，返回综合错误
		return nil, &DialError{
			Peer:   peerID,
			Errors: []error{directErr, fmt.Errorf("relay fallback failed: %w", err)},
		}
	} else {
		logger.Debug("未配置 Relay，无法回退", "peerID", peerShort)
	}

	// 6. 没有配置 Relay，返回直连错误
	// OPT-2 优化：根据连续失败次数决定日志级别
	failures := s.incrementDialFailures(peerID)
	if failures > dialFailureLogThreshold {
		logger.Debug("拨号失败（连续失败中，节点可能离线）",
			"peerID", peerShort,
			"failures", failures)
	}
	// 这是正确的行为：中继是可选的兜底方案
	return nil, directErr
}

// filterDirectAddrs 过滤出非中继地址
//
// 中继地址格式包含 /p2p-circuit/
// 同时过滤无效的 0.0.0.0 地址，避免无意义的拨号尝试
func filterDirectAddrs(addrs []string) []string {
	var direct []string
	for _, addr := range addrs {
		// 跳过中继地址
		if strings.Contains(addr, "/p2p-circuit/") {
			continue
		}

		// 跳过 0.0.0.0 地址
		// 0.0.0.0 是无效的拨号目标（表示"所有接口"），不可能成功连接
		// 这些地址通常是服务器错误上报的本地监听地址
		if strings.Contains(addr, "/ip4/0.0.0.0/") {
			logger.Debug("过滤无效的 0.0.0.0 地址", "addr", addr)
			continue
		}

		direct = append(direct, addr)
	}
	return direct
}

// filterRelayAddrs 过滤出中继地址
//
// 中继地址格式包含 /p2p-circuit/
// 用于在 dialPeer 中尝试通过 Peerstore 中的 Relay 地址连接
func filterRelayAddrs(addrs []string) []string {
	var relay []string
	for _, addr := range addrs {
		if strings.Contains(addr, "/p2p-circuit/") {
			relay = append(relay, addr)
		}
	}
	return relay
}

// filterRelayAddrsExcludeSelf 过滤出中继地址，排除以自己为中继的地址
//
// Relay 节点不应尝试通过自己中继流量
func (s *Swarm) filterRelayAddrsExcludeSelf(addrs []string) []string {
	var relay []string
	for _, addr := range addrs {
		if !strings.Contains(addr, "/p2p-circuit/") {
			continue
		}
		// 检查 Relay ID 是否是自己
		// 地址格式：.../p2p/<RelayID>/p2p-circuit/...
		parts := strings.Split(addr, "/p2p-circuit/")
		if len(parts) >= 1 {
			// 从前半部分提取 RelayID
			relayPart := parts[0]
			if strings.Contains(relayPart, "/p2p/"+s.localPeer) {
				logger.Debug("过滤以自己为中继的地址",
					"addr", addr,
					"localPeer", truncateID(s.localPeer, 8))
				continue
			}
		}
		relay = append(relay, addr)
	}
	return relay
}

// dialViaRelayAddr 通过指定的 Relay 地址连接目标节点
//
// 实现 Circuit Relay v2 协议的主动方连接流程：
//  1. 解析 Relay 地址，提取 Relay 服务器 ID 和传输地址
//  2. 确保已连接到 Relay 服务器
//  3. 创建临时 Relay Client，发送 CONNECT 请求
//
// 地址格式：/ip4/x.x.x.x/udp/port/quic-v1/p2p/<RelayID>/p2p-circuit/p2p/<TargetID>
//
// 参数：
//   - ctx: 上下文
//   - targetPeerID: 目标节点 ID
//   - relayAddr: 完整的 Relay 地址（包含 /p2p-circuit/）
//
// 返回：
//   - pkgif.Connection: 中继电路连接
//   - error: 错误信息
func (s *Swarm) dialViaRelayAddr(ctx context.Context, targetPeerID, relayAddr string) (pkgif.Connection, error) {
	peerShort := truncateID(targetPeerID, 8)
	logger.Debug("dialViaRelayAddr: 开始通过 Relay 地址连接",
		"target", peerShort,
		"relayAddr", relayAddr)

	// 1. 解析 Relay 地址
	relayPeerID, relayServerAddr, err := parseRelayAddrForSwarm(relayAddr)
	if err != nil {
		logger.Debug("dialViaRelayAddr: 解析 Relay 地址失败",
			"target", peerShort,
			"error", err)
		return nil, fmt.Errorf("parse relay addr: %w", err)
	}

	// 跳过以自己为中继的地址
	// Relay 节点不可能通过自己来中继流量到目标节点
	if relayPeerID == s.localPeer {
		logger.Debug("dialViaRelayAddr: 跳过自己作为中继的地址",
			"target", peerShort,
			"relay", truncateID(relayPeerID, 8))
		return nil, ErrDialToSelf
	}

	relayShort := truncateID(relayPeerID, 8)
	logger.Debug("dialViaRelayAddr: 解析 Relay 地址成功",
		"target", peerShort,
		"relay", relayShort,
		"relayServerAddr", relayServerAddr)

	// 2. 确保已连接到 Relay 服务器
	if conns := s.ConnsToPeer(relayPeerID); len(conns) == 0 {
		logger.Debug("dialViaRelayAddr: 未连接 Relay 服务器，尝试直连",
			"relay", relayShort)

		// 使用 dialWorker 连接 Relay 服务器
		_, err := s.dialWorker(ctx, relayPeerID, []string{relayServerAddr})
		if err != nil {
			logger.Warn("dialViaRelayAddr: 连接 Relay 服务器失败",
				"relay", relayShort,
				"error", err)
			return nil, fmt.Errorf("connect to relay server: %w", err)
		}
		logger.Debug("dialViaRelayAddr: 已连接到 Relay 服务器",
			"relay", relayShort)
	}

	// 3. 创建 Relay Client，使用 ConnectAsInitiator 连接目标
	// 注意：不要调用 client.Close()，因为：
	//   - RelayCircuit 依赖于 client.conn 上的 stream
	//   - client.conn 是 Swarm 管理的连接，不应由 Client 关闭
	//   - Client 对象本身是轻量级的，会被 GC 回收
	relayMA, err := types.NewMultiaddr(relayServerAddr + "/p2p/" + relayPeerID)
	if err != nil {
		return nil, fmt.Errorf("create relay multiaddr: %w", err)
	}

	client := relayclient.NewClient(s, types.PeerID(relayPeerID), relayMA)

	// 4. 作为主动方连接目标节点（不需要 reservation）
	conn, err := client.ConnectAsInitiator(ctx, types.PeerID(targetPeerID))
	if err != nil {
		logger.Warn("dialViaRelayAddr: 通过 Relay 连接目标失败",
			"target", peerShort,
			"relay", relayShort,
			"error", err)
		return nil, fmt.Errorf("connect via relay: %w", err)
	}

	logger.Info("dialViaRelayAddr: 中继电路建立成功",
		"target", peerShort,
		"relay", relayShort)

	return conn, nil
}

// parseRelayAddrForSwarm 解析 Relay 地址（Swarm 内部使用）
//
// 地址格式：/ip4/x.x.x.x/udp/port/quic-v1/p2p/<RelayID>/p2p-circuit/p2p/<TargetID>
//
// 返回：
//   - relayPeerID: Relay 服务器的 PeerID
//   - relayServerAddr: Relay 服务器的传输地址（不含 /p2p/<ID>）
//   - error: 错误信息
func parseRelayAddrForSwarm(addr string) (relayPeerID, relayServerAddr string, err error) {
	// 查找 p2p-circuit 分隔符
	circuitIdx := strings.Index(addr, "/p2p-circuit")
	if circuitIdx == -1 {
		return "", "", fmt.Errorf("missing /p2p-circuit in address: %s", addr)
	}

	// 解析中继部分：/ip4/.../p2p/<RelayID>
	relayPart := addr[:circuitIdx]
	p2pIdx := strings.LastIndex(relayPart, "/p2p/")
	if p2pIdx == -1 {
		return "", "", fmt.Errorf("missing relay peer ID in address: %s", addr)
	}

	relayPeerID = relayPart[p2pIdx+5:]
	relayServerAddr = relayPart[:p2pIdx]

	if relayPeerID == "" || relayServerAddr == "" {
		return "", "", fmt.Errorf("invalid relay address format: %s", addr)
	}

	return relayPeerID, relayServerAddr, nil
}

// getDirectConns 获取到指定节点的直连（非中继）连接
func (s *Swarm) getDirectConns(peerID string) []pkgif.Connection {
	conns := s.ConnsToPeer(peerID)
	var direct []pkgif.Connection
	for _, conn := range conns {
		addr := conn.RemoteMultiaddr()
		if addr != nil && !strings.Contains(addr.String(), "/p2p-circuit/") {
			direct = append(direct, conn)
		}
	}
	return direct
}

// DialDirect 直接拨号到指定地址（用于 HolePunch）
//
// 提供直接拨号能力，不经过完整的 dialPeer 流程，
// 用于 HolePuncher 的同时拨号，避免递归
//
// 实现 holepunch.DirectDialer 接口
func (s *Swarm) DialDirect(ctx context.Context, peerID string, addr string) (pkgif.Connection, error) {
	peerShort := truncateID(peerID, 8)
	logger.Debug("DialDirect 开始", "peerID", peerShort, "addr", addr)

	// 直接拨号单个地址
	conn, err := s.dialAddr(ctx, peerID, addr)
	if err != nil {
		logger.Debug("DialDirect 失败", "peerID", peerShort, "addr", addr, "error", err)
		return nil, err
	}

	logger.Info("DialDirect 成功", "peerID", peerShort, "addr", addr)
	return conn, nil
}

// rankAddrs 对地址进行排序
// 优先级：本地网络 > QUIC > TCP
func rankAddrs(addrs []string) []string {
	var local, quic, tcp, other []string

	for _, addr := range addrs {
		if isPrivateAddr(addr) {
			local = append(local, addr)
		} else if strings.Contains(addr, "/quic") {
			quic = append(quic, addr)
		} else if strings.Contains(addr, "/tcp") {
			tcp = append(tcp, addr)
		} else {
			other = append(other, addr)
		}
	}

	// 合并：本地 > QUIC > TCP > 其他
	result := make([]string, 0, len(addrs))
	result = append(result, local...)
	result = append(result, quic...)
	result = append(result, tcp...)
	result = append(result, other...)

	return result
}

// rankAddrsWithHealth 使用路径健康管理器优化地址排序
//
// Phase 0 修复：集成 pathhealth 模块
// 如果设置了 pathHealthManager，会：
//   - 使用路径健康度重新排序地址
//   - 过滤掉死亡路径
func (s *Swarm) rankAddrsWithHealth(peerID string, addrs []string) []string {
	s.mu.RLock()
	manager := s.pathHealthManager
	s.mu.RUnlock()

	if manager == nil {
		return addrs
	}

	// 使用路径健康管理器排序
	rankedAddrs := manager.RankAddrs(peerID, addrs)

	// 过滤掉死亡路径
	var healthyAddrs []string
	for _, addr := range rankedAddrs {
		stats := manager.GetPathStats(peerID, addr)
		if stats == nil {
			// 未知路径，允许尝试
			healthyAddrs = append(healthyAddrs, addr)
		} else if stats.State != pkgif.PathStateDead {
			// 非死亡路径，允许尝试
			healthyAddrs = append(healthyAddrs, addr)
		} else {
			logger.Debug("跳过死亡路径", "peerID", truncateID(peerID, 8), "addr", addr)
		}
	}

	// 如果所有路径都死亡，仍然尝试（可能网络恢复了）
	if len(healthyAddrs) == 0 && len(addrs) > 0 {
		logger.Debug("所有路径死亡，仍尝试拨号", "peerID", truncateID(peerID, 8))
		return addrs
	}

	return healthyAddrs
}

// isPrivateAddr 检查是否为私有网络地址
func isPrivateAddr(addr string) bool {
	// 检查 192.168.x.x, 10.x.x.x, 172.16-31.x.x, 127.x.x.x
	return strings.Contains(addr, "192.168") ||
		strings.Contains(addr, "/ip4/10.") ||
		strings.Contains(addr, "172.16") ||
		strings.Contains(addr, "172.17") ||
		strings.Contains(addr, "172.18") ||
		strings.Contains(addr, "172.19") ||
		strings.Contains(addr, "172.2") || // 172.20-29
		strings.Contains(addr, "172.30") ||
		strings.Contains(addr, "172.31") ||
		strings.Contains(addr, "127.0.0.1") ||
		strings.Contains(addr, "localhost")
}

// dialWorker 并发拨号工作器
func (s *Swarm) dialWorker(ctx context.Context, peerID string, addrs []string) (pkgif.Connection, error) {
	// 确定超时时间
	timeout := s.config.DialTimeout
	hasLocal := false
	for _, addr := range addrs {
		if isPrivateAddr(addr) {
			hasLocal = true
			break
		}
	}
	if hasLocal {
		timeout = s.config.DialTimeoutLocal
	}

	// 创建超时 context
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// 结果通道
	results := make(chan dialResult, len(addrs))

	// 并发拨号所有地址
	for _, addr := range addrs {
		go func(addr string) {
			conn, err := s.dialAddr(ctx, peerID, addr)
			results <- dialResult{conn, err}
		}(addr)
	}

	// 等待第一个成功或全部失败
	var errs []error
	for i := 0; i < len(addrs); i++ {
		select {
		case res := <-results:
			if res.err == nil {
				// 第一个成功的连接胜出，取消其他拨号
				cancel()
				return res.conn, nil
			}
			errs = append(errs, res.err)
		case <-ctx.Done():
			// 超时或取消
			if ctx.Err() == context.DeadlineExceeded {
				// 超时后检查死亡路径
				s.cleanupDeadPaths(peerID, addrs)
				return nil, ErrDialTimeout
			}
			return nil, ctx.Err()
		}
	}

	// 所有拨号失败后，检查并清理死亡路径
	s.cleanupDeadPaths(peerID, addrs)

	// 所有拨号都失败
	return nil, &DialError{
		Peer:   peerID,
		Errors: errs,
	}
}

// cleanupDeadPaths 清理死亡路径
//
// 当所有拨号失败后，检查 PathHealthManager 中标记为 Dead 的路径，
// 并从 Peerstore 中清除这些无效地址，避免后续重复尝试。
func (s *Swarm) cleanupDeadPaths(peerID string, addrs []string) {
	s.mu.RLock()
	manager := s.pathHealthManager
	peerstore := s.peerstore
	s.mu.RUnlock()

	if manager == nil || peerstore == nil {
		return
	}

	// 检查每个地址的状态
	var deadAddrs []string
	for _, addr := range addrs {
		stats := manager.GetPathStats(peerID, addr)
		if stats != nil && stats.State == pkgif.PathStateDead {
			deadAddrs = append(deadAddrs, addr)
		}
	}

	if len(deadAddrs) == 0 {
		return
	}

	// 如果所有地址都死亡，清除该 Peer 的所有地址
	// 这样下次连接时会重新从其他来源获取地址
	if len(deadAddrs) == len(addrs) {
		logger.Info("清除 Peer 所有死亡地址",
			"peerID", truncateID(peerID, 8),
			"deadCount", len(deadAddrs))
		peerstore.ClearAddrs(types.PeerID(peerID))
		return
	}

	// 部分地址死亡，记录日志（目前接口不支持单个地址清理）
	// 这些地址会在 PathHealthManager 的排序中被降级
	logger.Debug("发现部分死亡地址",
		"peerID", truncateID(peerID, 8),
		"deadCount", len(deadAddrs),
		"totalCount", len(addrs),
		"deadAddrs", deadAddrs)
}

// dialAddr 拨号单个地址
func (s *Swarm) dialAddr(ctx context.Context, peerID string, addr string) (pkgif.Connection, error) {
	logger.Debug("开始拨号地址",
		"peerID", truncateID(peerID, 8),
		"addr", addr)

	startTime := time.Now()

	// 解析地址为 Multiaddr
	maddr, err := multiaddr.NewMultiaddr(addr)
	if err != nil {
		logger.Debug("地址解析失败", "addr", addr, "error", err)
		// 报告解析失败给 PathHealthManager
		s.reportDialResult(peerID, addr, 0, err)
		return nil, fmt.Errorf("parse addr %s: %w", addr, err)
	}

	// 选择传输层
	transport := s.selectTransportForDial(addr)
	if transport == nil {
		logger.Debug("无匹配传输层", "addr", addr)
		// 报告传输层选择失败
		s.reportDialResult(peerID, addr, 0, ErrNoTransport)
		return nil, fmt.Errorf("%w: %s", ErrNoTransport, addr)
	}

	logger.Debug("开始传输层拨号",
		"peerID", truncateID(peerID, 8),
		"addr", addr,
		"transport", fmt.Sprintf("%T", transport))

	// 拨号获取 transport 连接
	transportConn, err := transport.Dial(ctx, maddr, types.PeerID(peerID))
	if err != nil {
		rtt := time.Since(startTime)
		logger.Debug("传输层拨号失败",
			"peerID", truncateID(peerID, 8),
			"addr", addr,
			"error", err,
			"duration", rtt)
		// 报告拨号失败给 PathHealthManager
		s.reportDialResult(peerID, addr, rtt, err)
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}

	// Transport 返回的已经是 Connection，直接返回封装
	// 注意：在真实实现中，transport.Dial 会调用 upgrader 完成升级
	// 这里 transportConn 已经是升级后的连接了

	rtt := time.Since(startTime)

	// 验证节点 ID
	if string(transportConn.RemotePeer()) != peerID {
		transportConn.Close()
		// 报告 PeerID 不匹配错误
		mismatchErr := fmt.Errorf("peer ID mismatch: expected %s, got %s", peerID, transportConn.RemotePeer())
		s.reportDialResult(peerID, addr, rtt, mismatchErr)
		return nil, mismatchErr
	}

	// 报告拨号成功给 PathHealthManager
	s.reportDialResult(peerID, addr, rtt, nil)

	// 封装为 Swarm 连接
	conn := newSwarmConn(s, transportConn)

	// 添加到连接池
	s.addConn(conn)

	// 触发事件
	s.notifyConnected(conn)

	// P0-2: 记录直连成功日志，包含连接类型和 RTT
	logger.Debug("直连拨号成功",
		"peerID", truncateID(peerID, 8),
		"connType", conn.ConnType().String(),
		"addr", addr,
		"rtt", rtt)

	// 启动入站流处理循环（关键：出站连接也需要能接收对方发来的流）
	go s.handleInboundStreams(conn)

	// 注：ConnMgr 通过事件总线自动处理连接事件

	return conn, nil
}

// reportDialResult 报告拨号结果给 PathHealthManager
//
// 将拨号结果（成功/失败）报告给 PathHealthManager，
// 使其能够学习路径质量，用于后续地址排序和死亡路径清理。
func (s *Swarm) reportDialResult(peerID string, addr string, rtt time.Duration, err error) {
	s.mu.RLock()
	manager := s.pathHealthManager
	s.mu.RUnlock()

	if manager == nil {
		return
	}

	// 报告探测结果
	manager.ReportProbe(peerID, addr, rtt, err)

	if err != nil {
		logger.Debug("已报告拨号失败给 PathHealthManager",
			"peerID", truncateID(peerID, 8),
			"addr", addr,
			"error", err)
	}
}

// selectTransportForDial 根据地址选择传输层（拨号用）
func (s *Swarm) selectTransportForDial(addr string) pkgif.Transport {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 解析地址，选择对应的传输层
	if strings.Contains(addr, "/quic") {
		if t, ok := s.transports["quic"]; ok {
			return t
		}
	}

	if strings.Contains(addr, "/tcp") {
		if t, ok := s.transports["tcp"]; ok {
			return t
		}
	}

	// 默认返回第一个可用传输层
	for _, t := range s.transports {
		return t
	}

	return nil
}

// AddTransport 添加传输层
func (s *Swarm) AddTransport(protocol string, transport pkgif.Transport) error {
	if transport == nil {
		return fmt.Errorf("transport cannot be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed.Load() {
		return ErrSwarmClosed
	}

	s.transports[protocol] = transport
	return nil
}

// SetUpgrader 设置升级器
func (s *Swarm) SetUpgrader(upgrader pkgif.Upgrader) error {
	if upgrader == nil {
		return fmt.Errorf("upgrader cannot be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed.Load() {
		return ErrSwarmClosed
	}

	s.upgrader = upgrader
	return nil
}

// SetPeerstore 设置 Peerstore
func (s *Swarm) SetPeerstore(peerstore pkgif.Peerstore) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.peerstore = peerstore
}

// SetConnMgr 设置 ConnMgr
func (s *Swarm) SetConnMgr(connmgr pkgif.ConnManager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.connmgr = connmgr
}

// SetEventBus 设置 EventBus
func (s *Swarm) SetEventBus(eventbus pkgif.EventBus) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.eventbus = eventbus
}

// SetBandwidthCounter 设置带宽计数器
func (s *Swarm) SetBandwidthCounter(bandwidth pkgif.BandwidthCounter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.bandwidth = bandwidth
}

// SetPathHealthManager 设置路径健康管理器
//
// Phase 0 修复：集成 pathhealth 模块
// 设置后，拨号时会：
//   - 按路径健康度排序地址（优先选择健康路径）
//   - 跳过死亡路径
func (s *Swarm) SetPathHealthManager(manager pkgif.PathHealthManager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pathHealthManager = manager
}

// getBandwidthCounter 获取带宽计数器（内部方法）
func (s *Swarm) getBandwidthCounter() pkgif.BandwidthCounter {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bandwidth
}

// BandwidthCounter 返回带宽计数器（v1.1 新增公开方法）
//
// 用于 Node API 层通过类型断言访问带宽统计功能。
// 如果带宽统计未启用，返回 nil。
func (s *Swarm) BandwidthCounter() pkgif.BandwidthCounter {
	return s.getBandwidthCounter()
}

// SetRelayDialer 设置 Relay 拨号器（v2.0 统一接口）
//
// 用于支持惰性中继策略：
//   - 当直连失败时，Swarm 通过此接口尝试 Relay 连接
func (s *Swarm) SetRelayDialer(dialer pkgif.RelayDialer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.relayDialer = dialer
}

// RelayDialer 获取 Relay 拨号器
func (s *Swarm) RelayDialer() pkgif.RelayDialer {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.relayDialer
}

// SetHolePuncher 设置打洞服务
//
// 集成 HolePuncher 到拨号流程：
// 直连失败后，如果有中继连接，先尝试通过打洞建立直连，打洞失败才回退到中继
func (s *Swarm) SetHolePuncher(puncher pkgif.HolePuncher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.holePuncher = puncher
}

// HolePuncher 获取打洞服务
func (s *Swarm) HolePuncher() pkgif.HolePuncher {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.holePuncher
}
