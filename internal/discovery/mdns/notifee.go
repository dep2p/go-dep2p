package mdns

import (
	"context"
	"strings"
	"sync"

	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/dep2p/go-dep2p/pkg/lib/zeroconf"
)

// logger 在 mdns.go 中定义

// truncateID 安全截断 ID 字符串，避免切片越界 panic
func truncateID(id string, maxLen int) string {
	if len(id) <= maxLen {
		return id
	}
	return id[:maxLen]
}

// peerNotifee 内部 Notifee 实现
type peerNotifee struct {
	ctx    context.Context
	selfID string
	peerCh chan<- types.PeerInfo
	mu     sync.Mutex
	seen   map[string]bool // 防止重复通知
	closed bool            // 标记 channel 是否已关闭
}

// newPeerNotifee 创建 peerNotifee
func newPeerNotifee(ctx context.Context, selfID string, peerCh chan<- types.PeerInfo) *peerNotifee {
	return &peerNotifee{
		ctx:    ctx,
		selfID: selfID,
		peerCh: peerCh,
		seen:   make(map[string]bool),
		closed: false,
	}
}

// Close 标记 notifee 为关闭状态，防止向已关闭 channel 发送
func (n *peerNotifee) Close() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.closed = true
}

// safeSend 安全地向 channel 发送数据
// 返回 true 表示发送成功，false 表示 channel 已关闭或 context 已取消
func (n *peerNotifee) safeSend(peerInfo types.PeerInfo) (ok bool) {
	// 使用 defer recover 捕获向已关闭 channel 发送的 panic
	defer func() {
		if r := recover(); r != nil {
			logger.Debug("发送到已关闭的 channel", "peer", truncateID(string(peerInfo.ID), 8))
			ok = false
		}
	}()

	// 先检查是否已标记关闭
	n.mu.Lock()
	if n.closed {
		n.mu.Unlock()
		return false
	}
	n.mu.Unlock()

	// 尝试发送
	select {
	case <-n.ctx.Done():
		return false
	case n.peerCh <- peerInfo:
		return true
	}
}

// handleEntry 处理 zeroconf ServiceEntry
func (n *peerNotifee) handleEntry(entry *zeroconf.ServiceEntry) error {
	if entry == nil {
		return nil
	}

	logger.Debug("发现服务",
		"instance", entry.Instance,
		"service", entry.Service,
		"domain", entry.Domain,
		"txt", entry.Text)

	// 解析 TXT 记录，提取 multiaddr
	var addrs []types.Multiaddr
	for _, txt := range entry.Text {
		if !strings.HasPrefix(txt, DNSAddrPrefix) {
			continue
		}

		addrStr := txt[len(DNSAddrPrefix):]
		addr, err := types.NewMultiaddr(addrStr)
		if err != nil {
			logger.Debug("无效地址", "addr", addrStr, "error", err)
			continue
		}
		addrs = append(addrs, addr)
	}

	if len(addrs) == 0 {
		logger.Debug("未找到有效地址")
		return nil
	}

	logger.Debug("解析地址", "count", len(addrs))

	// 从 multiaddr 中提取 AddrInfo
	peerMap := make(map[types.PeerID]*types.AddrInfo)
	for _, addr := range addrs {
		info, err := types.AddrInfoFromP2pAddr(addr)
		if err != nil {
			// 使用更宽松的解析方式，避免校验过严导致丢失节点
			peerID, idErr := types.GetPeerID(addr)
			if idErr != nil {
				logger.Debug("AddrInfo 解析失败", "addr", addr.String(), "error", err, "peerIDError", idErr)
				continue
			}
			transport := types.WithoutPeerID(addr)
			info = &types.AddrInfo{ID: peerID}
			if !types.IsEmpty(transport) {
				info.Addrs = []types.Multiaddr{transport}
			}
		}

		if existing, ok := peerMap[info.ID]; ok {
			existing.Addrs = append(existing.Addrs, info.Addrs...)
		} else {
			peerMap[info.ID] = info
		}
	}

	infos := make([]types.AddrInfo, 0, len(peerMap))
	for _, info := range peerMap {
		infos = append(infos, *info)
	}

	if len(infos) == 0 {
		logger.Debug("未解析到有效的节点信息")
		return nil
	}

	logger.Debug("解析到节点", "count", len(infos))

	// 转换为 PeerInfo
	for _, ai := range infos {
		peerInfo := ai.ToPeerInfo()

		// 跳过自己
		if string(peerInfo.ID) == n.selfID {
			logger.Debug("跳过自己", "peer", truncateID(string(peerInfo.ID), 8))
			continue
		}

		// 防止重复通知
		n.mu.Lock()
		if n.seen[string(peerInfo.ID)] {
			n.mu.Unlock()
			logger.Debug("已见过", "peer", truncateID(string(peerInfo.ID), 8))
			continue
		}
		n.seen[string(peerInfo.ID)] = true
		n.mu.Unlock()

		logger.Info("发现新节点", "peer", truncateID(string(peerInfo.ID), 8), "addrs", peerInfo.Addrs)

		// 推送节点信息（安全发送，防止向已关闭 channel 发送）
		if !n.safeSend(peerInfo) {
			return nil // channel 已关闭，安全退出
		}
	}

	return nil
}

// isSuitableForMDNS 检查地址是否适合 mDNS 广播
//
// 适合的地址必须满足：
//  1. 以 IP4/IP6 或 .local DNS 开头
//  2. 不包含不适合的协议（circuit, websocket, webrtc）
//
// 参考：go-libp2p/p2p/discovery/mdns/mdns.go
func isSuitableForMDNS(addr types.Multiaddr) bool {
	if addr == nil {
		return false
	}

	first, rest := types.SplitFirst(addr)
	if first.Protocol().Code == 0 {
		return false
	}

	// 检查地址类型
	switch first.Protocol().Code {
	case types.P_IP4, types.P_IP6:
		// ✅ 直接 IP 地址适合 LAN 发现
	case types.P_DNS, types.P_DNS4, types.P_DNS6, types.P_DNSADDR:
		// ✅ 只有 .local TLD 适合（mDNS 域）
		if !strings.HasSuffix(strings.ToLower(first.Value()), ".local") {
			return false
		}
	default:
		return false
	}

	// 检查是否包含不适合的协议
	return !containsUnsuitableProtocol(rest)
}

// containsUnsuitableProtocol 检查是否包含不适合 mDNS 的协议
//
// 不适合的协议：
//   - Circuit relay（需要中继，不是直连）
//   - Browser transports: WebTransport, WebRTC, WebSocket
//
// 参考：go-libp2p/p2p/discovery/mdns/mdns.go
func containsUnsuitableProtocol(addr types.Multiaddr) bool {
	if addr == nil {
		return false
	}

	found := false
	types.ForEach(addr, func(c types.Component) bool {
		switch c.Protocol().Code {
		case types.P_CIRCUIT,
			types.P_WEBTRANSPORT,
			types.P_WEBRTC,
			types.P_WEBRTC_DIRECT,
			types.P_WS,
			types.P_WSS:
			found = true
			return false // 停止遍历
		}
		return true // 继续遍历
	})
	return found
}
