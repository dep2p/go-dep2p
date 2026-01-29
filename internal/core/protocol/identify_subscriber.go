// Package protocol 实现协议注册与路由
//
// identify_subscriber.go: P0 修复 - Identify 结果入库
// 订阅连接事件，主动调用 Identify 客户端获取远端地址并写入 Peerstore
package protocol

import (
	"context"
	"strings"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/peerstore"
	"github.com/dep2p/go-dep2p/internal/core/protocol/system/identify"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/types"
)

var identifyLogger = log.Logger("protocol/identify-subscriber")

// IdentifySubscriber 订阅连接事件并主动识别远端节点
//
// P0 修复：打通 Identify → Peerstore 地址传播链路
type IdentifySubscriber struct {
	host        pkgif.Host
	coordinator pkgif.ReachabilityCoordinator //
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewIdentifySubscriber 创建 Identify 订阅器
//
// coordinator 可以为 nil（测试场景），此时不会同步观测地址到候选地址
func NewIdentifySubscriber(host pkgif.Host, coordinator pkgif.ReachabilityCoordinator) *IdentifySubscriber {
	return &IdentifySubscriber{
		host:        host,
		coordinator: coordinator,
	}
}

// Start 启动订阅器
func (s *IdentifySubscriber) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)
	go s.subscribeConnectionEvents()
	identifyLogger.Info("Identify 订阅器已启动")
	return nil
}

// Stop 停止订阅器
func (s *IdentifySubscriber) Stop() error {
	if s.cancel != nil {
		s.cancel()
	}
	identifyLogger.Info("Identify 订阅器已停止")
	return nil
}

// subscribeConnectionEvents 订阅连接事件
func (s *IdentifySubscriber) subscribeConnectionEvents() {
	eventBus := s.host.EventBus()
	if eventBus == nil {
		identifyLogger.Warn("EventBus 不可用，无法订阅连接事件")
		return
	}

	sub, err := eventBus.Subscribe(new(types.EvtPeerConnected))
	if err != nil {
		identifyLogger.Warn("订阅连接事件失败", "error", err)
		return
	}
	if sub == nil {
		identifyLogger.Warn("订阅返回 nil，无法监听连接事件")
		return
	}
	defer sub.Close()

	for {
		select {
		case <-s.ctx.Done():
			return
		case evt := <-sub.Out():
			connected, ok := evt.(*types.EvtPeerConnected)
			if !ok {
				continue
			}
			// 异步处理，避免阻塞事件循环
			go s.handlePeerConnected(string(connected.PeerID))
		}
	}
}

// handlePeerConnected 处理节点连接事件
//
// 主动调用 Identify 客户端，将远端地址写入 Peerstore
func (s *IdentifySubscriber) handlePeerConnected(peerID string) {
	peerIDShort := peerID
	if len(peerIDShort) > 8 {
		peerIDShort = peerIDShort[:8]
	}

	// 如果 Peerstore 已知远端协议且不支持 Identify，则跳过
	if ps := s.host.Peerstore(); ps != nil {
		supported, err := ps.SupportsProtocols(types.PeerID(peerID), types.ProtocolID(identify.ProtocolID))
		if err == nil && len(supported) == 0 {
			allProtos, _ := ps.GetProtocols(types.PeerID(peerID))
			if len(allProtos) > 0 {
				identifyLogger.Debug("跳过 Identify：远端不支持 Identify 协议",
					"peer", peerIDShort,
					"knownProtocols", len(allProtos))
				return
			}
		}
	}

	// 使用超时上下文
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()

	// 调用 Identify 客户端获取远端信息
	info, err := identify.Identify(ctx, s.host, peerID)
	if err != nil {
		//
		// 某些节点（如 Relay）可能不支持 Identify 协议
		// 这不应该影响连接的正常使用
		identifyLogger.Debug("Identify 获取远端信息失败（降级：使用已知地址）",
			"peer", peerIDShort,
			"error", err)
		// 不返回，尝试使用已有的 Peerstore 信息
		return
	}

	// 将 ListenAddrs 写入 Peerstore
	if len(info.ListenAddrs) > 0 {
		ps := s.host.Peerstore()
		if ps == nil {
			identifyLogger.Warn("Peerstore 不可用，无法写入地址", "peer", peerIDShort)
			return
		}

		var maddrs []types.Multiaddr
		for _, addrStr := range info.ListenAddrs {
			ma, err := types.NewMultiaddr(addrStr)
			if err != nil {
				identifyLogger.Debug("解析地址失败",
					"peer", peerIDShort,
					"addr", addrStr,
					"error", err)
				continue
			}
			maddrs = append(maddrs, ma)
		}

		if len(maddrs) > 0 {
			ps.AddAddrs(types.PeerID(peerID), maddrs, peerstore.ConnectedAddrTTL)
			identifyLogger.Debug("Identify 地址已写入 Peerstore",
				"peer", peerIDShort,
				"addrs", len(maddrs))
		}
	}

	// P0 修复：将 ObservedAddr 写入本机的 ObservedAddrManager
	// 这是远端看到的我方地址（可能是公网地址）
	if info.ObservedAddr != "" {
		s.addObservedAddr(info.ObservedAddr, peerID)
	}
}

// addObservedAddr 添加观测地址到本机 ObservedAddrManager
//
// ObservedAddr 是远端看到的我方地址，可能是公网地址。
// 多个不同节点观测到的相同地址具有更高可信度。
//
//   - 过滤 Relay 地址（/p2p-circuit/）
//   - 只信任直连连接的观测结果
//   - 同步到 ReachabilityCoordinator 作为打洞候选地址
func (s *IdentifySubscriber) addObservedAddr(addrStr string, observerID string) {
	observerShort := observerID
	if len(observerShort) > 8 {
		observerShort = observerShort[:8]
	}

	// 1: 过滤 Relay 地址（/p2p-circuit/）
	// Relay 地址不能用于打洞，且 Relay 连接观测到的地址可能是 Relay 服务器的地址
	if strings.Contains(addrStr, "/p2p-circuit/") {
		identifyLogger.Debug("跳过 Relay 地址",
			"addr", addrStr,
			"observer", observerShort)
		return
	}

	// 2: 检查是否存在直连连接
	// 只有来自直连连接的观测地址才可信
	// Relay 连接的观测结果可能是 Relay 服务器的地址，不可用于打洞
	if !s.hasDirectConnection(observerID) {
		identifyLogger.Debug("跳过非直连连接的观测地址",
			"addr", addrStr,
			"observer", observerShort)
		return
	}

	// 解析地址
	ma, err := types.NewMultiaddr(addrStr)
	if err != nil {
		identifyLogger.Debug("解析 ObservedAddr 失败",
			"addr", addrStr,
			"observer", observerShort,
			"error", err)
		return
	}

	// 尝试获取 Host 的 AddObservedAddr 方法
	// 由于 pkgif.Host 接口可能没有直接暴露此方法，使用类型断言
	type observedAddrAdder interface {
		AddObservedAddr(addr types.Multiaddr) error
	}

	if adder, ok := s.host.(observedAddrAdder); ok {
		if err := adder.AddObservedAddr(ma); err != nil {
			identifyLogger.Debug("添加 ObservedAddr 失败",
				"addr", addrStr,
				"error", err)
		} else {
			identifyLogger.Debug("已添加 ObservedAddr",
				"addr", addrStr,
				"observer", observerShort)
		}
	}

	// 3: 同步到 ReachabilityCoordinator 作为候选地址
	// 这是打洞协议获取本地外部地址的关键通道
	if s.coordinator != nil {
		source := "observed:" + observerShort
		s.coordinator.OnDirectAddressCandidate(addrStr, source, pkgif.PriorityUnverified)
		identifyLogger.Debug("添加观测地址候选",
			"addr", addrStr,
			"source", source)
	}
}

// hasDirectConnection 检查是否存在到指定节点的直连连接
//
// 只有来自直连连接的观测地址才应该被用于打洞候选
func (s *IdentifySubscriber) hasDirectConnection(peerID string) bool {
	swarm := s.host.Network()
	if swarm == nil {
		return false
	}

	conns := swarm.ConnsToPeer(peerID)
	for _, conn := range conns {
		if conn.ConnType().IsDirect() {
			return true
		}
	}
	return false
}
