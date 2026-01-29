package swarm

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/types"
)

var logger = log.Logger("core/swarm")

// Swarm 连接群管理
type Swarm struct {
	mu sync.RWMutex

	// 本地节点 ID
	localPeer string

	// 连接池：peerID -> []Connection
	conns map[string][]pkgif.Connection

	// 传输层：protocol -> Transport
	transports map[string]pkgif.Transport

	// 升级器
	upgrader pkgif.Upgrader

	// 监听器
	listeners []pkgif.Listener

	// 通知器
	notifiers []pkgif.SwarmNotifier

	// 入站流处理器（由 Host 设置）
	inboundStreamHandler pkgif.InboundStreamHandler

	// 依赖（可选）
	peerstore         pkgif.Peerstore
	connmgr           pkgif.ConnManager
	eventbus          pkgif.EventBus
	bandwidth         pkgif.BandwidthCounter
	pathHealthManager pkgif.PathHealthManager // Phase 0 修复：路径健康管理

	// Relay 惰性回退支持（v2.0 统一接口）
	// 当直连失败时，通过此接口尝试 Relay 连接
	relayDialer pkgif.RelayDialer

	// HolePuncher 打洞服务
	// 直连失败后，如果有中继连接，先尝试通过打洞建立直连
	// 打洞失败才回退到中继
	holePuncher pkgif.HolePuncher

	// Liveness 服务（用于连接健康检测）
	// 通过定期 Ping 检测 QUIC 直连的健康状态
	liveness pkgif.Liveness

	// 连接健康检测控制
	connHealthCtx    context.Context
	connHealthCancel context.CancelFunc

	// 缓存不支持 Liveness 协议的节点
	// 避免对基础设施节点（Bootstrap/Relay）重复尝试 Ping
	noLivenessPeers sync.Map // map[peerID string]bool

	// OPT-2 优化：跟踪连续拨号失败次数
	// 连续失败超过阈值后降低日志级别，减少日志噪音
	dialFailures sync.Map // map[peerID string]int

	// Relay 地址退避跟踪器
	// 当通过 Relay 地址连接失败时（如 no reservation），
	// 记录失败信息并实施退避，避免频繁重试无效的 Relay 地址
	relayAddrBackoff sync.Map // map[relayAddrKey]*relayBackoffEntry

	// 配置
	config *Config

	// 状态
	closed atomic.Bool
}

// NewSwarm 创建 Swarm
func NewSwarm(localPeer string, opts ...Option) (*Swarm, error) {
	if localPeer == "" {
		return nil, fmt.Errorf("localPeer cannot be empty")
	}

	s := &Swarm{
		localPeer:  localPeer,
		conns:      make(map[string][]pkgif.Connection),
		transports: make(map[string]pkgif.Transport),
		listeners:  make([]pkgif.Listener, 0),
		notifiers:  make([]pkgif.SwarmNotifier, 0),
		config:     DefaultConfig(),
	}

	// 应用选项
	for _, opt := range opts {
		if err := opt(s); err != nil {
			return nil, err
		}
	}

	return s, nil
}

// LocalPeer 返回本地节点 ID
func (s *Swarm) LocalPeer() string {
	return s.localPeer
}

// Peers 返回所有已连接的节点 ID
func (s *Swarm) Peers() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed.Load() {
		return nil
	}

	peers := make([]string, 0, len(s.conns))
	for peerID := range s.conns {
		peers = append(peers, peerID)
	}
	return peers
}

// Conns 返回所有活跃连接
func (s *Swarm) Conns() []pkgif.Connection {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed.Load() {
		return nil
	}

	var conns []pkgif.Connection
	for _, peerConns := range s.conns {
		conns = append(conns, peerConns...)
	}
	return conns
}

// ConnsToPeer 返回到指定节点的所有连接
func (s *Swarm) ConnsToPeer(peerID string) []pkgif.Connection {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed.Load() {
		return nil
	}

	conns := s.conns[peerID]
	if len(conns) == 0 {
		return nil
	}

	// 返回副本
	result := make([]pkgif.Connection, len(conns))
	copy(result, conns)
	return result
}

// Connectedness 返回与指定节点的连接状态
func (s *Swarm) Connectedness(peerID string) pkgif.Connectedness {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.closed.Load() {
		return pkgif.NotConnected
	}

	// 检查是否已连接
	if len(s.conns[peerID]) > 0 {
		return pkgif.Connected
	}

	// 检查 PeerStore 是否有地址，返回 CanConnect
	if s.peerstore != nil {
		addrs := s.peerstore.Addrs(types.PeerID(peerID))
		if len(addrs) > 0 {
			return pkgif.CanConnect
		}
	}

	return pkgif.NotConnected
}

// DialPeer 拨号连接到指定节点
func (s *Swarm) DialPeer(ctx context.Context, peerID string) (pkgif.Connection, error) {
	if s.closed.Load() {
		return nil, ErrSwarmClosed
	}

	// 检查是否拨号自己
	if peerID == s.localPeer {
		return nil, ErrDialToSelf
	}

	// 调用完整的拨号逻辑（在 dial.go 中实现）
	// dialPeer 会：
	//  1. 检查已有连接复用
	//  2. 从 PeerStore 获取地址
	//  3. 地址排序和优先级
	//  4. 并发拨号
	//  5. 选择传输层
	//  6. 触发连接事件
	conn, err := s.dialPeer(ctx, peerID)
	if err != nil {
		logger.Debug("拨号失败", "peerID", truncateID(peerID, 8), "error", err)
	}
	return conn, err
}

// ClosePeer 关闭与指定节点的所有连接
func (s *Swarm) ClosePeer(peerID string) error {
	s.mu.Lock()
	conns := s.conns[peerID]
	delete(s.conns, peerID)
	s.mu.Unlock()

	// 清除 Liveness 协议缓存（节点重连后可能支持了）
	s.noLivenessPeers.Delete(peerID)

	var errs []error
	for _, conn := range conns {
		if err := conn.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("close peer %s: %v", peerID, errs)
	}
	return nil
}

// NewStream 创建到指定节点的新流
func (s *Swarm) NewStream(ctx context.Context, peerID string) (pkgif.Stream, error) {
	if s.closed.Load() {
		return nil, ErrSwarmClosed
	}

	// 获取连接
	conns := s.ConnsToPeer(peerID)
	if len(conns) == 0 {
		return nil, ErrNoConnection
	}

	// 尝试可用连接
	for _, conn := range conns {
		if conn == nil {
			continue
		}
		if conn.IsClosed() {
			s.removeConn(conn)
			continue
		}
		stream, err := conn.NewStream(ctx)
		if err == nil {
			return stream, nil
		}
		// 连接创建流失败，若已关闭则移除
		if conn.IsClosed() {
			s.removeConn(conn)
		}
	}

	return nil, ErrNoConnection
}

// Notify 注册连接事件通知
func (s *Swarm) Notify(notifier pkgif.SwarmNotifier) {
	if notifier == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.notifiers = append(s.notifiers, notifier)
}

// Close 关闭 Swarm
func (s *Swarm) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return ErrSwarmClosed
	}

	logger.Info("正在关闭 Swarm")

	// 停止连接健康检测循环
	if s.connHealthCancel != nil {
		s.connHealthCancel()
	}

	// 收集需要关闭的资源（避免持锁调用 Close 导致死锁）
	s.mu.Lock()
	listeners := s.listeners
	s.listeners = nil

	// 收集所有连接
	var allConns []pkgif.Connection
	for _, conns := range s.conns {
		allConns = append(allConns, conns...)
	}
	s.conns = make(map[string][]pkgif.Connection) // 清空但保留 map
	s.mu.Unlock()

	var errs []error

	// 关闭所有监听器
	for _, listener := range listeners {
		if err := listener.Close(); err != nil {
			logger.Warn("关闭监听器失败", "error", err)
			errs = append(errs, fmt.Errorf("close listener: %w", err))
		}
	}

	// 关闭所有连接（此时已释放锁，conn.Close 可以安全调用 removeConn）
	connCount := len(allConns)
	for _, conn := range allConns {
		if gc, ok := conn.(interface{ GracefulClose(time.Duration) error }); ok {
			if err := gc.GracefulClose(s.config.NewStreamTimeout); err != nil {
				logger.Warn("优雅关闭连接失败，降级为强制关闭", "error", err)
				_ = conn.Close()
			}
			continue
		}
		if err := conn.Close(); err != nil {
			peerID := string(conn.RemotePeer())
			peerLabel := peerID
			if len(peerLabel) > 8 {
				peerLabel = peerLabel[:8]
			}
			logger.Warn("关闭连接失败", "peerID", peerLabel, "error", err)
			errs = append(errs, fmt.Errorf("close conn: %w", err))
		}
	}

	if len(errs) > 0 {
		logger.Error("关闭 Swarm 时发生错误", "errorCount", len(errs))
		return fmt.Errorf("close swarm: %v", errs)
	}

	logger.Info("Swarm 已关闭", "closedConnections", connCount)
	return nil
}

// AddInboundConnection 添加入站连接到连接池
//
// 用于添加外部创建的连接（如中继电路）到 Swarm 的连接池。
// 这允许其他代码通过 ConnsToPeer/NewStream 使用这些连接。
//
// v2.0 新增：支持 RelayCircuit 集成
// v2.0 修复：Done() 监听已移至 addConn，避免重复
func (s *Swarm) AddInboundConnection(conn pkgif.Connection) {
	if conn == nil {
		return
	}

	peerShort := string(conn.RemotePeer())
	if len(peerShort) > 8 {
		peerShort = peerShort[:8]
	}

	logger.Info("添加入站连接",
		"remotePeer", peerShort,
		"connType", conn.ConnType())

	s.addConn(conn)
	s.notifyConnected(conn)
	// Done() 监听已在 addConn 中统一处理
}

// relayCircuitWithStreamLoop 用于检测是否为支持流接受循环的 RelayCircuit
//
// RelayCircuit 实现了此接口，允许 Swarm 为出站中继连接启动入站流处理。
type relayCircuitWithStreamLoop interface {
	AcceptStreamLoopWithHandler(handler func(pkgif.Stream))
}

// addConn 添加连接到池（内部方法）
//
// v2.0 修复：统一处理 Done() 监听，确保连接关闭时自动从池中移除。
// 这对于 RelayCircuit 尤其重要，因为电路可能因心跳超时等原因异步关闭。
//
// v2.1 修复：为 RelayCircuit 启动流接受循环。
// 出站 RelayCircuit（通过 DialViaRelay 创建）需要能够接收对端发来的流，
// 例如 PSK 认证流。之前只有入站 RelayCircuit 启动了流接受循环，
// 导致出站方无法接收入站流，认证流程无法完成。
func (s *Swarm) addConn(conn pkgif.Connection) {
	if conn == nil {
		return
	}

	s.mu.Lock()
	if s.closed.Load() {
		s.mu.Unlock()
		conn.Close()
		return
	}

	peerID := string(conn.RemotePeer())
	peerLabel := peerID
	if len(peerLabel) > 8 {
		peerLabel = peerLabel[:8]
	}
	s.conns[peerID] = append(s.conns[peerID], conn)
	s.mu.Unlock()

	// v2.0 修复：如果连接提供 Done() 信号，监听关闭并自动移除
	// 这确保了出站 RelayCircuit 关闭时能主动从 Swarm 移除
	if doneConn, ok := conn.(interface{ Done() <-chan struct{} }); ok {
		go func() {
			// 连接关闭信号
			<-doneConn.Done()
			// 检查 Swarm 是否已关闭，避免对已关闭的 Swarm 操作
			if !s.closed.Load() {
				s.removeConn(conn)
				s.notifyDisconnected(conn)
			}
		}()
	}

	// v2.1 修复：为 RelayCircuit 启动流接受循环
	// 这使得出站 RelayCircuit 也能接收对端发来的流
	// 注意：AcceptStreamLoopWithHandler 内部使用 sync.Once + goroutine，
	// 确保循环只启动一次且不阻塞调用者
	if circuit, ok := conn.(relayCircuitWithStreamLoop); ok {
		handler := s.getInboundStreamHandler()
		if handler != nil {
			logger.Debug("为 RelayCircuit 启动流接受循环", "peerID", peerLabel)
			circuit.AcceptStreamLoopWithHandler(handler)
		} else {
			logger.Warn("无法为 RelayCircuit 启动流接受循环：入站流处理器未设置", "peerID", peerLabel)
		}
	}
}

// removeConn 从池中移除连接（内部方法）
func (s *Swarm) removeConn(conn pkgif.Connection) {
	if conn == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	peerID := string(conn.RemotePeer())
	conns := s.conns[peerID]

	// 移除指定连接
	for i, c := range conns {
		if c == conn {
			s.conns[peerID] = append(conns[:i], conns[i+1:]...)
			break
		}
	}

	// 如果没有连接了，删除节点
	if len(s.conns[peerID]) == 0 {
		delete(s.conns, peerID)
	}
}

// notifyConnected 触发连接建立事件（内部方法）
func (s *Swarm) notifyConnected(conn pkgif.Connection) {
	if conn == nil {
		return
	}

	s.mu.RLock()
	notifiers := make([]pkgif.SwarmNotifier, len(s.notifiers))
	copy(notifiers, s.notifiers)
	s.mu.RUnlock()

	// 异步通知，避免阻塞
	for _, n := range notifiers {
		go n.Connected(conn)
	}
}

// notifyDisconnected 触发连接断开事件（内部方法）
func (s *Swarm) notifyDisconnected(conn pkgif.Connection) {
	if conn == nil {
		return
	}

	// 连接断开时降低地址 TTL，加速过期清理
	s.reduceDisconnectedPeerTTL(conn)

	s.mu.RLock()
	notifiers := make([]pkgif.SwarmNotifier, len(s.notifiers))
	copy(notifiers, s.notifiers)
	s.mu.RUnlock()

	// 异步通知，避免阻塞
	for _, n := range notifiers {
		go n.Disconnected(conn)
	}
}

// DisconnectedAddrTTL 断开连接后地址的 TTL
//
// 连接断开后，将地址 TTL 降低到此值，
// 允许地址在短时间内过期，避免反复尝试失效地址。
const DisconnectedAddrTTL = 5 * time.Minute

// reduceDisconnectedPeerTTL 降低断开连接的 Peer 地址 TTL
//
// 当连接断开时，降低该 Peer 地址的 TTL，
// 使其在较短时间内过期，避免反复尝试失效地址。
func (s *Swarm) reduceDisconnectedPeerTTL(conn pkgif.Connection) {
	peerID := conn.RemotePeer()
	if peerID == "" {
		return
	}

	s.mu.RLock()
	peerstore := s.peerstore
	s.mu.RUnlock()

	if peerstore == nil {
		return
	}

	// 尝试调用 ReduceAddrsTTL（如果 Peerstore 实现支持）
	type ttlReducer interface {
		ReduceAddrsTTL(peerID types.PeerID, maxTTL time.Duration) int
	}

	if reducer, ok := peerstore.(ttlReducer); ok {
		reduced := reducer.ReduceAddrsTTL(peerID, DisconnectedAddrTTL)
		if reduced > 0 {
			logger.Debug("连接断开，已降低地址 TTL",
				"peerID", truncateID(string(peerID), 8),
				"reducedCount", reduced,
				"newTTL", DisconnectedAddrTTL)
		}
	}
}

// SetInboundStreamHandler 设置入站流处理器
//
// 由 Host 在启动时调用，设置入站流的协议协商和路由处理逻辑。
// 当 Swarm 接受新连接后，会为每个入站流调用此处理器。
func (s *Swarm) SetInboundStreamHandler(handler pkgif.InboundStreamHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inboundStreamHandler = handler
}

// getInboundStreamHandler 获取入站流处理器（内部方法）
func (s *Swarm) getInboundStreamHandler() pkgif.InboundStreamHandler {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.inboundStreamHandler
}

// ============================================================================
//                          连接健康检测
// ============================================================================

// SetLiveness 设置 Liveness 服务并启动连接健康检测
//
// 通过定期 Ping 检测 QUIC 直连的健康状态
// 加速离线检测，缩短从 ~2 分钟到 ~30 秒
func (s *Swarm) SetLiveness(liveness pkgif.Liveness) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.liveness = liveness

	// 如果已有健康检测循环在运行，先停止
	if s.connHealthCancel != nil {
		s.connHealthCancel()
	}

	// 启动新的健康检测循环
	if liveness != nil && s.config.ConnHealthInterval > 0 {
		s.connHealthCtx, s.connHealthCancel = context.WithCancel(context.Background())
		go s.connHealthCheckLoop(s.connHealthCtx)
		logger.Info("连接健康检测已启动",
			"interval", s.config.ConnHealthInterval,
			"timeout", s.config.ConnHealthTimeout)
	}
}

// connHealthCheckLoop 连接健康检测循环
//
// 定期检查所有连接的健康状态
// 对于不健康的连接，主动关闭并触发 EvtPeerDisconnected
func (s *Swarm) connHealthCheckLoop(ctx context.Context) {
	if s.config.ConnHealthInterval <= 0 {
		logger.Debug("连接健康检测已禁用")
		return
	}

	ticker := time.NewTicker(s.config.ConnHealthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Debug("连接健康检测循环已停止")
			return
		case <-ticker.C:
			s.checkAllConnections(ctx)
		}
	}
}

// checkAllConnections 检查所有连接的健康状态
//
// 并发检查所有连接，对于检测失败的连接主动关闭
// 跳过基础设施节点（Relay/Bootstrap），它们不支持 Realm 的 Liveness 协议
func (s *Swarm) checkAllConnections(ctx context.Context) {
	s.mu.RLock()
	liveness := s.liveness
	if liveness == nil {
		s.mu.RUnlock()
		return
	}

	// 收集所有已连接的 peer
	peers := make([]string, 0, len(s.conns))
	for peerID := range s.conns {
		peers = append(peers, peerID)
	}
	s.mu.RUnlock()

	if len(peers) == 0 {
		return
	}

	logger.Debug("开始连接健康检测", "peerCount", len(peers))

	// 并发检查每个 peer
	var wg sync.WaitGroup
	for _, peerID := range peers {
		wg.Add(1)
		go func(pid string) {
			defer wg.Done()

			peerLabel := pid
			if len(peerLabel) > 8 {
				peerLabel = peerLabel[:8]
			}

			// 跳过基础设施节点（Relay/Bootstrap）
			// 基础设施节点不属于任何 Realm，不支持 Realm 特定的 Liveness 协议
			// 对它们执行 Ping 会导致 "protocols not supported" 错误
			if s.isInfrastructurePeer(pid) {
				logger.Debug("跳过基础设施节点的健康检测", "peerID", peerLabel)
				return
			}

			// 检查缓存，跳过已知不支持 Liveness 协议的节点
			if _, noLiveness := s.noLivenessPeers.Load(pid); noLiveness {
				return
			}

			// 使用配置的超时
			pingCtx, cancel := context.WithTimeout(ctx, s.config.ConnHealthTimeout)
			defer cancel()

			_, err := liveness.Ping(pingCtx, pid)
			if err != nil {
				// 区分协议不支持错误和真正的连接问题
				// 如果对方不支持 Liveness 协议，说明它可能是非 Realm 成员（基础设施节点）
				// 不应该关闭这种连接
				errStr := err.Error()
				if strings.Contains(errStr, "protocols not supported") ||
					strings.Contains(errStr, "protocol not supported") {
					// 缓存此节点，后续不再尝试 Ping
					s.noLivenessPeers.Store(pid, true)
					logger.Debug("节点不支持 Liveness 协议，已缓存跳过",
						"peerID", peerLabel)
					return
				}

				logger.Info("连接健康检测失败，关闭连接",
					"peerID", peerLabel,
					"error", err)

				// 关闭该 peer 的所有连接，这会触发 EvtPeerDisconnected
				s.ClosePeer(pid)
			}
		}(peerID)
	}

	wg.Wait()
}

// isInfrastructurePeer 检查是否为基础设施节点
//
// 基础设施节点包括：
// - 预配置的 Relay 节点（带 "relay" 标签）
// - 预配置的 Bootstrap 节点（带 "bootstrap" 标签）
// - 其他基础设施节点（带 "infrastructure" 标签）
//
// 这些节点不属于任何 Realm，不应该对它们执行 Realm 特定的 Liveness Ping。
func (s *Swarm) isInfrastructurePeer(peerID string) bool {
	if s.connmgr == nil {
		return false
	}

	tagInfo := s.connmgr.GetTagInfo(peerID)
	if tagInfo == nil || tagInfo.Tags == nil {
		return false
	}

	// 检查是否有基础设施相关的标签
	infrastructureTags := []string{"relay", "bootstrap", "infrastructure", "dht-server"}
	for _, tag := range infrastructureTags {
		if _, exists := tagInfo.Tags[tag]; exists {
			return true
		}
	}

	return false
}
