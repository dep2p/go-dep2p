package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/nat"
	"github.com/dep2p/go-dep2p/internal/core/protocol"
	"github.com/dep2p/go-dep2p/internal/core/relay"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/types"
	mss "github.com/multiformats/go-multistream"
)

var logger = log.Logger("core/host")

// Host P2P 主机实现
// 采用门面（Facade）模式，聚合所有核心组件
type Host struct {
	ctx       context.Context
	ctxCancel context.CancelFunc

	// 核心组件
	swarm       pkgif.Swarm
	protocol    *protocol.Router
	peerstore   pkgif.Peerstore
	eventbus    pkgif.EventBus
	connmgr     pkgif.ConnManager
	resourcemgr pkgif.ResourceManager

	// multistream-select muxer 用于入站协议协商
	mux *mss.MultistreamMuxer[string]

	// 服务
	nat   *nat.Service
	relay *relay.Manager

	// 配置
	config *Config

	// 地址管理
	addrsManager *addrsManager

	// 连接去重：跟踪每个 peer 的连接数量
	// 只在首次连接时发布 Connected 事件，
	// 只在最后一个连接断开时发布 Disconnected 事件
	peerConnCount   map[string]int
	peerConnCountMu sync.Mutex

	// 生命周期
	mu       sync.RWMutex
	started  atomic.Bool
	closed   atomic.Bool
	refCount sync.WaitGroup
}

// New 创建新的 Host
func New(opts ...Option) (*Host, error) {
	ctx, cancel := context.WithCancel(context.Background())

	h := &Host{
		ctx:           ctx,
		ctxCancel:     cancel,
		config:        DefaultConfig(),
		mux:           mss.NewMultistreamMuxer[string](),
		peerConnCount: make(map[string]int),
	}

	// 应用选项
	for _, opt := range opts {
		if err := opt(h); err != nil {
			cancel()
			return nil, fmt.Errorf("failed to apply option: %w", err)
		}
	}

	// 验证必需依赖
	if h.swarm == nil {
		cancel()
		return nil, errors.New("swarm is required")
	}

	// 创建地址管理器
	localPeerID := types.PeerID(h.swarm.LocalPeer())
	h.addrsManager = newAddrsManager(h.swarm, localPeerID, h.config.AddrsFactory)

	// 设置 Swarm 事件通知
	if h.swarm != nil {
		h.swarm.Notify(h)
	}

	// 设置入站流处理器
	h.swarm.SetInboundStreamHandler(h.handleInboundStream)

	return h, nil
}

// ID 返回节点 ID
func (h *Host) ID() string {
	if h.swarm == nil {
		return ""
	}
	return h.swarm.LocalPeer()
}

// Addrs 返回监听地址列表
func (h *Host) Addrs() []string {
	if h.addrsManager == nil {
		return nil
	}
	return h.addrsManager.Addrs()
}

// AdvertisedAddrs 返回对外公告地址
//
// 整合多个地址来源，按优先级排序：
//  1. 已验证的直连地址（来自 Reachability Coordinator）
//  2. Relay 地址（来自 Reachability Coordinator）
//  3. 监听地址（作为兜底）
func (h *Host) AdvertisedAddrs() []string {
	if h.addrsManager == nil {
		return nil
	}
	return h.addrsManager.AdvertisedAddrs()
}

// ShareableAddrs 返回可分享的公网地址
//
// 只返回已验证的公网地址（不包含私网地址和 0.0.0.0）。
func (h *Host) ShareableAddrs() []string {
	if h.addrsManager == nil {
		return nil
	}
	return h.addrsManager.ShareableAddrs()
}

// HolePunchAddrs 返回用于打洞协商的地址列表
//
// 而不仅仅是已验证的地址。对于 NAT 节点，dial-back 验证无法成功，
// 但 STUN 候选地址是真实的外部地址，是打洞必需的。
func (h *Host) HolePunchAddrs() []string {
	if h.addrsManager == nil {
		return nil
	}
	return h.addrsManager.HolePunchAddrs()
}

// SetReachabilityCoordinator 设置可达性协调器
//
// 用于获取已验证的外部地址和 Relay 地址。
func (h *Host) SetReachabilityCoordinator(coordinator pkgif.ReachabilityCoordinator) {
	if h.addrsManager != nil {
		h.addrsManager.SetReachabilityCoordinator(coordinator)
	}
}

// Listen 监听指定地址
//
// 委托给 Swarm 处理实际的监听逻辑，并更新地址管理器。
// 完成后发布 EvtLocalAddrsUpdated 事件通知依赖地址的服务（如 mDNS）。
func (h *Host) Listen(addrs ...string) error {
	if h.closed.Load() {
		return errors.New("host is closed")
	}
	if h.swarm == nil {
		return errors.New("swarm not initialized")
	}

	logger.Info("开始监听", "addrs", len(addrs))

	// 委托给 Swarm 监听
	if err := h.swarm.Listen(addrs...); err != nil {
		logger.Error("监听失败", "error", err)
		return err
	}

	// 获取新的监听地址
	newAddrs := h.swarm.ListenAddrs()
	logger.Info("监听成功", "addrs", newAddrs)

	// 从 Swarm 获取实际监听地址并更新 addrsManager
	if h.addrsManager != nil {
		multiaddrs := make([]types.Multiaddr, 0, len(newAddrs))
		for _, addrStr := range newAddrs {
			ma, err := types.NewMultiaddr(addrStr)
			if err == nil {
				multiaddrs = append(multiaddrs, ma)
			}
		}
		h.addrsManager.updateListenAddrs(multiaddrs)
	}

	// 发布地址更新事件，通知 mDNS 等依赖服务
	h.emitLocalAddrsUpdated(newAddrs, addrs)

	return nil
}

// emitLocalAddrsUpdated 发布本地地址更新事件
func (h *Host) emitLocalAddrsUpdated(current []string, added []string) {
	if h.eventbus == nil {
		return
	}

	emitter, err := h.eventbus.Emitter(&types.EvtLocalAddrsUpdated{})
	if err != nil {
		logger.Debug("创建地址更新事件发射器失败", "error", err)
		return
	}
	defer emitter.Close()

	emitter.Emit(&types.EvtLocalAddrsUpdated{
		BaseEvent: types.NewBaseEvent(types.EventTypeLocalAddrsUpdated),
		Current:   current,
		Added:     added,
		Removed:   nil,
	})

	logger.Debug("已发布地址更新事件", "current", len(current), "added", len(added))
}

// Connect 连接到指定节点
// 该方法会先将地址添加到 Peerstore，然后委托给 Swarm.DialPeer
func (h *Host) Connect(ctx context.Context, peerID string, addrs []string) error {
	if h.closed.Load() {
		return errors.New("host is closed")
	}

	// 2. 委托给 Swarm 拨号
	if h.swarm == nil {
		logger.Error("Swarm 不可用")
		return errors.New("swarm not available")
	}

	//
	// 这可以大幅减少日志量（实测 27 分钟内减少 700+ 条重复日志）
	alreadyConnected := h.swarm.Connectedness(peerID) == pkgif.Connected

	// 1. 添加地址到 Peerstore
	if h.peerstore != nil && len(addrs) > 0 {
		// 类型转换 string -> types.Multiaddr
		multiaddrs := make([]types.Multiaddr, 0, len(addrs))
		for _, addrStr := range addrs {
			ma, err := types.NewMultiaddr(addrStr)
			if err != nil {
				// 跳过无效地址
				continue
			}
			multiaddrs = append(multiaddrs, ma)
		}

		// 添加到 peerstore（TTL = 1 小时）
		if len(multiaddrs) > 0 {
			h.peerstore.AddAddrs(types.PeerID(peerID), multiaddrs, time.Hour)
		}
	}

	conn, err := h.swarm.DialPeer(ctx, peerID)
	if err != nil {
		logger.Warn("连接节点失败", "peerID", peerID[:8], "error", err)
	} else if !alreadyConnected {
		//
		// P0-2: 添加连接类型标签，便于 NAT 穿透效果分析
		connType := "direct"
		if conn != nil && conn.ConnType().IsRelay() {
			connType = "relay"
		}
		logger.Info("连接节点成功", "peerID", peerID[:8], "connType", connType)
	}
	// 如果是复用连接，不打印日志（避免日志膨胀）
	return err
}

// KnownPeer 已知节点配置（与 config.KnownPeer 对应）
type KnownPeer struct {
	PeerID string
	Addrs  []string
}

// ConnectKnownPeers 连接已知节点列表
//
// 启动时调用，直接连接配置的已知节点，不依赖引导节点或 DHT 发现。
// 适用于云服务器部署、私有网络等已知节点地址的场景。
//
// 特点：
//   - 并发连接所有已知节点
//   - 连接失败不阻塞启动，仅记录警告日志
//   - 连接成功后节点会被加入 Peerstore，后续 DHT 可从中学习
func (h *Host) ConnectKnownPeers(ctx context.Context, peers []KnownPeer) error {
	if len(peers) == 0 {
		return nil
	}

	if h.closed.Load() {
		return errors.New("host is closed")
	}

	logger.Info("开始连接已知节点", "count", len(peers))

	var wg sync.WaitGroup
	var successCount, failCount int32

	for _, peer := range peers {
		wg.Add(1)
		go func(p KnownPeer) {
			defer wg.Done()

			// 跳过自己
			if p.PeerID == h.ID() {
				return
			}

			// 使用子上下文，设置单个连接超时
			connCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			err := h.Connect(connCtx, p.PeerID, p.Addrs)
			if err != nil {
				atomic.AddInt32(&failCount, 1)
				logger.Warn("连接已知节点失败",
					"peerID", truncatePeerID(p.PeerID, 8),
					"addrs", p.Addrs,
					"error", err)
			} else {
				atomic.AddInt32(&successCount, 1)
				logger.Info("连接已知节点成功",
					"peerID", truncatePeerID(p.PeerID, 8))
			}
		}(peer)
	}

	// 等待所有连接尝试完成
	wg.Wait()

	logger.Info("已知节点连接完成",
		"total", len(peers),
		"success", atomic.LoadInt32(&successCount),
		"failed", atomic.LoadInt32(&failCount))

	return nil
}

// truncatePeerID 截断 PeerID 用于日志显示
func truncatePeerID(peerID string, maxLen int) string {
	if len(peerID) <= maxLen {
		return peerID
	}
	return peerID[:maxLen]
}

// NewStream 创建到指定节点的新流（默认优先级）
// 该方法会确保连接存在，然后创建流并进行协议协商
func (h *Host) NewStream(ctx context.Context, peerID string, protocolIDs ...string) (pkgif.Stream, error) {
	// 默认使用普通优先级
	if len(protocolIDs) == 0 {
		return h.NewStreamWithPriority(ctx, peerID, "", int(pkgif.StreamPriorityNormal))
	}
	return h.NewStreamWithPriority(ctx, peerID, protocolIDs[0], int(pkgif.StreamPriorityNormal))
}

// NewStreamWithPriority 创建到指定节点的新流（指定优先级）(v1.2 新增)
//
// 允许指定流优先级。在 QUIC 连接上，优先级会传递给底层传输层。
// 在 TCP 连接上，优先级会被忽略（优雅降级）。
func (h *Host) NewStreamWithPriority(ctx context.Context, peerID string, protocolID string, priority int) (pkgif.Stream, error) {
	if h.closed.Load() {
		return nil, errors.New("host is closed")
	}

	if h.swarm == nil {
		return nil, errors.New("swarm not available")
	}

	// 1. 委托给 Swarm 创建流（带优先级）
	stream, err := h.swarm.NewStreamWithPriority(ctx, peerID, priority)
	if err != nil {
		return nil, fmt.Errorf("failed to create stream: %w", err)
	}

	// 2. 协议协商（如果提供了协议 ID）
	if protocolID != "" {
		// 使用 multistream-select 进行协议协商（客户端侧）
		selectedProto, err := mss.SelectOneOf([]string{protocolID}, stream)
		if err != nil {
			stream.Close()
			return nil, fmt.Errorf("protocol negotiation failed: %w", err)
		}

		// 设置协商后的协议 ID 到流
		stream.SetProtocol(selectedProto)
	}

	return stream, nil
}

// SetStreamHandler 为指定协议设置流处理器
// 同时注册到 multistream-select muxer（入站协商）和 Protocol Router（路由）
func (h *Host) SetStreamHandler(protocolID string, handler pkgif.StreamHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 1. 注册到 multistream-select muxer
	// 这使得入站流协议协商时能识别此协议
	if h.mux != nil {
		if handler != nil {
			// 将 StreamHandler 适配为 multistream-select 的 handler
			h.mux.AddHandler(protocolID, func(proto string, rwc io.ReadWriteCloser) error {
				stream, ok := rwc.(pkgif.Stream)
				if !ok {
					return fmt.Errorf("unexpected stream type for protocol %s", proto)
				}
				handler(stream)
				return nil
			})
		} else {
			h.mux.AddHandler(protocolID, nil)
		}
	}

	// 2. 注册到 Protocol Router
	if h.protocol != nil {
		h.protocol.AddRoute(protocolID, handler)
	}

	logger.Debug("注册协议处理器", "protocolID", protocolID)
}

// RemoveStreamHandler 移除指定协议的流处理器
// 同时从 multistream-select muxer 和 Protocol Router 移除
func (h *Host) RemoveStreamHandler(protocolID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// 1. 从 multistream-select muxer 移除
	if h.mux != nil {
		h.mux.RemoveHandler(protocolID)
	}

	// 2. 从 Protocol Router 移除
	if h.protocol != nil {
		h.protocol.RemoveRoute(protocolID)
	}

	logger.Debug("移除协议处理器", "protocolID", protocolID)
}

// Peerstore 返回节点存储
func (h *Host) Peerstore() pkgif.Peerstore {
	return h.peerstore
}

// EventBus 返回事件总线
func (h *Host) EventBus() pkgif.EventBus {
	return h.eventbus
}

// Network 返回 Swarm（用于内部访问）
func (h *Host) Network() pkgif.Swarm {
	return h.swarm
}

// AddObservedAddr 添加观测地址
//
// P0 修复：暴露 ObservedAddrManager.Add 供 Identify 订阅器使用。
// 观测地址是远端节点看到的我方地址，可能是公网地址。
// 多个节点观测到的相同地址具有更高可信度。
func (h *Host) AddObservedAddr(addr types.Multiaddr) error {
	if h.addrsManager == nil {
		return errors.New("addrsManager not initialized")
	}
	if h.addrsManager.observedAddrs == nil {
		return errors.New("observedAddrs not initialized")
	}
	h.addrsManager.observedAddrs.Add(addr)
	logger.Debug("已添加观测地址", "addr", addr.String())
	return nil
}

// Close 关闭 Host
// 该方法会关闭所有子系统并等待所有后台任务完成
func (h *Host) Close() error {
	if !h.closed.CompareAndSwap(false, true) {
		return nil // 已关闭，幂等操作
	}

	logger.Info("正在关闭 Host")

	// 取消上下文
	if h.ctxCancel != nil {
		h.ctxCancel()
	}

	// 关闭 NAT 服务
	if h.nat != nil {
		if err := h.nat.Stop(); err != nil {
			// 记录错误但继续关闭
			logger.Warn("停止 NAT 服务失败", "error", err)
		}
	}

	// 关闭 Relay 管理器
	if h.relay != nil {
		if err := h.relay.Stop(); err != nil {
			// 记录错误但继续关闭
			logger.Warn("停止 Relay 管理器失败", "error", err)
		}
	}

	// 关闭 Swarm
	if h.swarm != nil {
		if err := h.swarm.Close(); err != nil {
			// 记录错误但继续关闭
			logger.Warn("关闭 Swarm 失败", "error", err)
		}
	}

	logger.Info("Host 已关闭")

	// 等待所有后台任务完成
	h.refCount.Wait()

	return nil
}

// Closed 返回 Host 是否已关闭
func (h *Host) Closed() bool {
	return h.closed.Load()
}

// handleInboundStream 处理入站流
//
// 该方法由 Swarm 在接受到新的入站流时调用。
// 执行以下步骤：
// 1. 使用 multistream-select 进行服务端侧协议协商
// 2. 设置协商后的协议 ID 到流
// 3. 将流路由到对应的协议处理器
//
// 中继连接的 STOP 流需要调用此方法来处理后续的协议协商
func (h *Host) handleInboundStream(stream pkgif.Stream) {
	if h.closed.Load() {
		stream.Reset()
		return
	}

	remotePeer := string(stream.Conn().RemotePeer())
	remotePeerLabel := remotePeer
	if len(remotePeerLabel) > 8 {
		remotePeerLabel = remotePeerLabel[:8]
	}

	// 1. 使用 multistream-select 进行服务端侧协议协商
	h.mu.RLock()
	mux := h.mux
	h.mu.RUnlock()

	if mux == nil {
		logger.Error("multistream muxer 未初始化")
		stream.Reset()
		return
	}

	// 服务端协商：等待客户端发送协议请求，选择匹配的协议
	selectedProto, handler, err := mux.Negotiate(stream)
	if err != nil {
		if err != io.EOF {
			logger.Debug("协议协商失败", "remotePeer", remotePeerLabel, "error", err)
		}
		stream.Reset()
		return
	}

	// 2. 设置协商后的协议 ID 到流
	stream.SetProtocol(selectedProto)

	// 3. 将流路由到对应的协议处理器
	// 如果 mux 的 handler 不为 nil，优先使用它
	if handler != nil {
		if err := handler(selectedProto, stream); err != nil {
			logger.Debug("mux handler 处理失败", "remotePeer", remotePeerLabel, "protocol", selectedProto, "error", err)
			stream.Reset()
		}
		return
	}

	// 使用 Protocol Router 路由
	if h.protocol != nil {
		err := h.protocol.Route(stream)
		if err != nil {
			logger.Debug("流路由失败", "remotePeer", remotePeerLabel, "protocol", selectedProto, "error", err)
			stream.Reset()
			return
		}
	} else {
		logger.Warn("Protocol Router 未设置，关闭流", "protocol", selectedProto)
		stream.Reset()
	}
}

// HandleInboundStream 处理入站流（公开接口）
//
// 对流进行协议协商并路由到相应处理器。
// 用于处理非标准来源的入站流（如中继连接的 STOP 流）。
//
// WiFi→Relay→4G 的数据通过 STOP 流传输，需要进行协议协商和路由。
func (h *Host) HandleInboundStream(stream pkgif.Stream) {
	h.handleInboundStream(stream)
}
