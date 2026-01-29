// Package relay 提供中继服务实现
//
// RelayTransport 将中继连接作为一等公民传输层：
//   - 实现 Transport 接口
//   - 支持通过中继建立双向连接
//   - 自动升级到直连（配合 holepunch）
//   - 管理中继预留和连接生命周期
package relay

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/types"
)

var transportLogger = log.Logger("relay/transport")

// ============================================================================
//                              错误定义
// ============================================================================

var (
	// ErrTransportClosed 传输已关闭
	ErrTransportClosed = errors.New("relay transport closed")

	// ErrInvalidRelayAddr 无效的中继地址
	ErrInvalidRelayAddr = errors.New("invalid relay address")

	// ErrDialFailed 拨号失败
	ErrDialFailed = errors.New("relay dial failed")

	// ErrListenNotSupported 不支持监听
	ErrListenNotSupported = errors.New("relay transport does not support listen")
)

// ============================================================================
//                              常量定义
// ============================================================================

const (
	// CircuitProtocol 中继电路协议前缀
	CircuitProtocol = "/p2p-circuit"

	// DefaultDialTimeout 默认拨号超时
	DefaultDialTimeout = 30 * time.Second

	// DefaultReservationTimeout 默认预留超时
	DefaultReservationTimeout = 10 * time.Second

	// MaxConcurrentDials 最大并发拨号数
	MaxConcurrentDials = 8
)

// ============================================================================
//                              RelayTransport 结构
// ============================================================================

// RelayTransport 中继传输层
//
// 将中继连接作为一等公民传输层，实现通过中继服务器
// 建立到其他节点的连接。
type RelayTransport struct {
	// 依赖组件
	manager   *Manager          // 中继管理器
	host      pkgif.Host        // 本地主机
	peerstore pkgif.Peerstore   // 节点存储
	upgrader  pkgif.Upgrader    // 连接升级器（可选）

	// 配置
	dialTimeout        time.Duration
	reservationTimeout time.Duration
	maxConcurrentDials int

	// 活跃连接跟踪
	activeConns   map[string]*relayConn
	activeConnsMu sync.RWMutex

	// 拨号限制
	dialSem chan struct{}

	// 状态
	running int32
	closed  int32

	// 同步
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// relayConn 中继连接
type relayConn struct {
	conn       net.Conn
	relayPeer  types.PeerID
	remotePeer types.PeerID
	createdAt  time.Time
	upgraded   bool
}

// RelayTransportConfig 中继传输配置
type RelayTransportConfig struct {
	// DialTimeout 拨号超时
	DialTimeout time.Duration

	// ReservationTimeout 预留超时
	ReservationTimeout time.Duration

	// MaxConcurrentDials 最大并发拨号数
	MaxConcurrentDials int
}

// DefaultRelayTransportConfig 返回默认配置
func DefaultRelayTransportConfig() RelayTransportConfig {
	return RelayTransportConfig{
		DialTimeout:        DefaultDialTimeout,
		ReservationTimeout: DefaultReservationTimeout,
		MaxConcurrentDials: MaxConcurrentDials,
	}
}

// ============================================================================
//                              构造函数
// ============================================================================

// NewRelayTransport 创建中继传输层
func NewRelayTransport(manager *Manager, host pkgif.Host, peerstore pkgif.Peerstore, config RelayTransportConfig) *RelayTransport {
	ctx, cancel := context.WithCancel(context.Background())

	// 应用默认值
	if config.DialTimeout <= 0 {
		config.DialTimeout = DefaultDialTimeout
	}
	if config.ReservationTimeout <= 0 {
		config.ReservationTimeout = DefaultReservationTimeout
	}
	if config.MaxConcurrentDials <= 0 {
		config.MaxConcurrentDials = MaxConcurrentDials
	}

	rt := &RelayTransport{
		manager:            manager,
		host:               host,
		peerstore:          peerstore,
		dialTimeout:        config.DialTimeout,
		reservationTimeout: config.ReservationTimeout,
		maxConcurrentDials: config.MaxConcurrentDials,
		activeConns:        make(map[string]*relayConn),
		dialSem:            make(chan struct{}, config.MaxConcurrentDials),
		ctx:                ctx,
		cancel:             cancel,
	}

	transportLogger.Info("中继传输层已创建",
		"dialTimeout", config.DialTimeout,
		"maxConcurrent", config.MaxConcurrentDials)

	return rt
}

// SetUpgrader 设置连接升级器
func (rt *RelayTransport) SetUpgrader(upgrader pkgif.Upgrader) {
	rt.upgrader = upgrader
}

// ============================================================================
//                              Transport 接口实现
// ============================================================================

// Dial 通过中继拨号到远程节点
//
// 参数：
//   - ctx: 上下文
//   - raddr: 中继地址（格式：/p2p/<relay-id>/p2p-circuit/p2p/<target-id>）
//   - p: 目标节点 ID
//
// 返回：
//   - net.Conn: 中继连接
//   - error: 错误
func (rt *RelayTransport) Dial(ctx context.Context, raddr string, p types.PeerID) (net.Conn, error) {
	if atomic.LoadInt32(&rt.closed) == 1 {
		return nil, ErrTransportClosed
	}

	// 解析中继地址
	relayPeer, targetPeer, err := parseCircuitAddr(raddr)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidRelayAddr, err)
	}

	// 验证目标节点
	if targetPeer != "" && targetPeer != p {
		return nil, fmt.Errorf("peer ID mismatch: addr=%s, expected=%s", targetPeer, p)
	}

	transportLogger.Debug("通过中继拨号",
		"relay", safePeerIDPrefix(relayPeer),
		"target", safePeerIDPrefix(p))

	// 获取拨号信号量
	select {
	case rt.dialSem <- struct{}{}:
		defer func() { <-rt.dialSem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// 设置超时
	dialCtx, cancel := context.WithTimeout(ctx, rt.dialTimeout)
	defer cancel()

	// 通过中继管理器拨号
	conn, err := rt.dialThroughRelay(dialCtx, relayPeer, p)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDialFailed, err)
	}

	// 跟踪连接
	rt.trackConnection(relayPeer, p, conn)

	transportLogger.Info("中继连接已建立",
		"relay", safePeerIDPrefix(relayPeer),
		"target", safePeerIDPrefix(p))

	return conn, nil
}

// DialPeer 通过中继拨号到节点（自动选择中继）
//
// 参数：
//   - ctx: 上下文
//   - p: 目标节点 ID
//
// 返回：
//   - net.Conn: 中继连接
//   - error: 错误
func (rt *RelayTransport) DialPeer(ctx context.Context, p types.PeerID) (net.Conn, error) {
	if atomic.LoadInt32(&rt.closed) == 1 {
		return nil, ErrTransportClosed
	}

	transportLogger.Debug("通过中继拨号（自动选择）", "target", safePeerIDPrefix(p))

	// 获取可用的中继
	relays := rt.getAvailableRelays()
	if len(relays) == 0 {
		return nil, ErrNoRelayAvailable
	}

	// 尝试每个中继
	var lastErr error
	for _, relay := range relays {
		// 获取拨号信号量
		select {
		case rt.dialSem <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}

		// 设置超时
		dialCtx, cancel := context.WithTimeout(ctx, rt.dialTimeout)
		conn, err := rt.dialThroughRelay(dialCtx, relay, p)
		cancel()
		<-rt.dialSem

		if err == nil {
			rt.trackConnection(relay, p, conn)
			transportLogger.Info("中继连接已建立",
				"relay", safePeerIDPrefix(relay),
				"target", safePeerIDPrefix(p))
			return conn, nil
		}

		lastErr = err
		transportLogger.Debug("中继拨号失败，尝试下一个",
			"relay", safePeerIDPrefix(relay),
			"error", err)
	}

	return nil, fmt.Errorf("%w: all relays failed, last error: %v", ErrNoRelayAvailable, lastErr)
}

// Listen 监听中继连接（不支持）
//
// 注意：中继传输不支持主动监听，入站连接由中继管理器处理。
func (rt *RelayTransport) Listen(_ string) (net.Listener, error) {
	return nil, ErrListenNotSupported
}

// Protocols 返回支持的协议列表
func (rt *RelayTransport) Protocols() []string {
	return []string{CircuitProtocol}
}

// CanDial 检查是否可以拨号到指定地址
func (rt *RelayTransport) CanDial(addr string) bool {
	return strings.Contains(addr, CircuitProtocol)
}

// ============================================================================
//                              内部方法
// ============================================================================

// dialThroughRelay 通过指定中继拨号
func (rt *RelayTransport) dialThroughRelay(ctx context.Context, _, targetPeer types.PeerID) (net.Conn, error) {
	if rt.manager == nil {
		return nil, errors.New("relay manager not available")
	}

	// 使用统一 Relay 接口
	if rt.manager.HasRelay() {
		conn, err := rt.manager.DialViaRelay(ctx, string(targetPeer))
		if err == nil {
			return connToNetConn(conn), nil
		}
		transportLogger.Debug("Relay 拨号失败", "error", err)
	}

	return nil, ErrNoRelayAvailable
}

// connToNetConn 将 pkgif.Connection 转换为 net.Conn
func connToNetConn(conn pkgif.Connection) net.Conn {
	if netConn, ok := conn.(net.Conn); ok {
		return netConn
	}
	// Connection 接口应该包含 net.Conn 的方法
	return &connectionWrapper{conn: conn}
}

// connectionWrapper 包装 pkgif.Connection 为 net.Conn
type connectionWrapper struct {
	conn pkgif.Connection
}

func (w *connectionWrapper) Read(_ []byte) (n int, err error) {
	// Connection 不直接支持 Read，需要通过 Stream
	return 0, errors.New("use streams for data transfer")
}

func (w *connectionWrapper) Write(_ []byte) (n int, err error) {
	return 0, errors.New("use streams for data transfer")
}

func (w *connectionWrapper) Close() error {
	return w.conn.Close()
}

func (w *connectionWrapper) LocalAddr() net.Addr {
	return &relayAddr{addr: "local"}
}

func (w *connectionWrapper) RemoteAddr() net.Addr {
	return &relayAddr{addr: string(w.conn.RemotePeer())}
}

func (w *connectionWrapper) SetDeadline(_ time.Time) error {
	return nil
}

func (w *connectionWrapper) SetReadDeadline(_ time.Time) error {
	return nil
}

func (w *connectionWrapper) SetWriteDeadline(_ time.Time) error {
	return nil
}

// relayAddr 实现 net.Addr
type relayAddr struct {
	addr string
}

func (a *relayAddr) Network() string {
	return "p2p-circuit"
}

func (a *relayAddr) String() string {
	return a.addr
}

// getAvailableRelays 获取可用的中继列表
//
// v2.0 改进：支持多候选中继
// 优先级：AutoRelay 活跃中继 > 静态配置的中继地址
func (rt *RelayTransport) getAvailableRelays() []types.PeerID {
	if rt.manager == nil {
		return nil
	}

	var result []types.PeerID
	seen := make(map[string]bool)

	// 1. v2.0 新增：从 AutoRelay 获取多个活跃中继
	autoRelayAddrs := rt.manager.GetAutoRelayAddrs()
	for _, addr := range autoRelayAddrs {
		// 从 circuit 地址中提取 relay peer ID
		// 格式: /ip4/.../p2p/QmRelay.../p2p-circuit/p2p/QmLocal
		peerID := extractPeerIDFromCircuitAddr(addr)
		if peerID != "" && !seen[string(peerID)] {
			result = append(result, peerID)
			seen[string(peerID)] = true
		}
	}

	// 2. 检查静态配置的 Relay 地址
	if rt.manager.HasRelay() {
		if addr, ok := rt.manager.RelayAddr(); ok {
			if peerID := extractPeerIDFromAddr(addr); peerID != "" && !seen[string(peerID)] {
				result = append(result, peerID)
				seen[string(peerID)] = true
			}
		}
	}

	return result
}

// extractPeerIDFromCircuitAddr 从 circuit 地址中提取 Relay PeerID
//
// 格式: /ip4/.../p2p/QmRelay.../p2p-circuit/p2p/QmLocal
// 返回 QmRelay（第一个 /p2p/ 后的 ID）
func extractPeerIDFromCircuitAddr(addr string) types.PeerID {
	if addr == "" {
		return ""
	}

	// 查找 /p2p-circuit/
	circuitIdx := strings.Index(addr, "/p2p-circuit")
	if circuitIdx == -1 {
		// 不是 circuit 地址，尝试普通提取
		return extractPeerIDFromAddrString(addr)
	}

	// 从开头到 /p2p-circuit 之间提取
	beforeCircuit := addr[:circuitIdx]

	// 查找最后一个 /p2p/
	const p2pPrefix = "/p2p/"
	idx := strings.LastIndex(beforeCircuit, p2pPrefix)
	if idx == -1 {
		return ""
	}

	peerIDStr := beforeCircuit[idx+len(p2pPrefix):]
	if peerIDStr == "" {
		return ""
	}

	return types.PeerID(peerIDStr)
}

// extractPeerIDFromAddrString 从地址字符串中提取 PeerID
func extractPeerIDFromAddrString(addr string) types.PeerID {
	const p2pPrefix = "/p2p/"
	idx := strings.LastIndex(addr, p2pPrefix)
	if idx == -1 {
		return ""
	}

	peerIDStr := addr[idx+len(p2pPrefix):]

	// 如果后面还有其他组件，截取到下一个 /
	if nextSlash := strings.Index(peerIDStr, "/"); nextSlash != -1 {
		peerIDStr = peerIDStr[:nextSlash]
	}

	if peerIDStr == "" {
		return ""
	}

	return types.PeerID(peerIDStr)
}

// extractPeerIDFromAddr 从 Multiaddr 中提取 PeerID
//
// 支持的地址格式：
//   - /ip4/1.2.3.4/tcp/4001/p2p/QmXXX
//   - /dns4/relay.example.com/tcp/4001/p2p/QmXXX
func extractPeerIDFromAddr(addr types.Multiaddr) types.PeerID {
	if addr == nil {
		return ""
	}
	
	addrStr := addr.String()
	
	// 查找 /p2p/ 组件
	const p2pPrefix = "/p2p/"
	idx := strings.LastIndex(addrStr, p2pPrefix)
	if idx == -1 {
		return ""
	}
	
	// 提取 PeerID（从 /p2p/ 后面到字符串末尾或下一个 / ）
	peerIDStr := addrStr[idx+len(p2pPrefix):]
	
	// 如果后面还有其他组件（如 /p2p-circuit），截取到下一个 /
	if nextSlash := strings.Index(peerIDStr, "/"); nextSlash != -1 {
		peerIDStr = peerIDStr[:nextSlash]
	}
	
	if peerIDStr == "" {
		return ""
	}
	
	return types.PeerID(peerIDStr)
}

// trackConnection 跟踪连接
func (rt *RelayTransport) trackConnection(relayPeer, remotePeer types.PeerID, conn net.Conn) {
	rt.activeConnsMu.Lock()
	defer rt.activeConnsMu.Unlock()

	key := fmt.Sprintf("%s-%s", relayPeer, remotePeer)
	rt.activeConns[key] = &relayConn{
		conn:       conn,
		relayPeer:  relayPeer,
		remotePeer: remotePeer,
		createdAt:  time.Now(),
	}
}

// ============================================================================
//                              生命周期管理
// ============================================================================

// Start 启动传输层
func (rt *RelayTransport) Start(_ context.Context) error {
	if !atomic.CompareAndSwapInt32(&rt.running, 0, 1) {
		return nil
	}

	transportLogger.Info("中继传输层已启动")
	return nil
}

// Close 关闭传输层
func (rt *RelayTransport) Close() error {
	if !atomic.CompareAndSwapInt32(&rt.closed, 0, 1) {
		return nil
	}

	rt.cancel()

	// 关闭所有活跃连接
	rt.activeConnsMu.Lock()
	for _, rc := range rt.activeConns {
		if rc.conn != nil {
			rc.conn.Close()
		}
	}
	rt.activeConns = make(map[string]*relayConn)
	rt.activeConnsMu.Unlock()

	rt.wg.Wait()

	transportLogger.Info("中继传输层已关闭")
	return nil
}

// ============================================================================
//                              统计信息
// ============================================================================

// RelayTransportStats 传输统计
type RelayTransportStats struct {
	// ActiveConnections 活跃连接数
	ActiveConnections int

	// TotalDialed 总拨号次数
	TotalDialed uint64

	// FailedDials 失败次数
	FailedDials uint64
}

// Stats 返回统计信息
func (rt *RelayTransport) Stats() RelayTransportStats {
	rt.activeConnsMu.RLock()
	activeCount := len(rt.activeConns)
	rt.activeConnsMu.RUnlock()

	return RelayTransportStats{
		ActiveConnections: activeCount,
	}
}

// GetActiveConnections 获取活跃连接信息
func (rt *RelayTransport) GetActiveConnections() []RelayConnectionInfo {
	rt.activeConnsMu.RLock()
	defer rt.activeConnsMu.RUnlock()

	result := make([]RelayConnectionInfo, 0, len(rt.activeConns))
	for _, rc := range rt.activeConns {
		result = append(result, RelayConnectionInfo{
			RelayPeer:  rc.relayPeer,
			RemotePeer: rc.remotePeer,
			CreatedAt:  rc.createdAt,
			Upgraded:   rc.upgraded,
		})
	}

	return result
}

// RelayConnectionInfo 中继连接信息
type RelayConnectionInfo struct {
	RelayPeer  types.PeerID
	RemotePeer types.PeerID
	CreatedAt  time.Time
	Upgraded   bool
}

// ============================================================================
//                              辅助函数
// ============================================================================

// parseCircuitAddr 解析中继电路地址
//
// 格式：/ip4/.../p2p/<relay-id>/p2p-circuit/p2p/<target-id>
// 或：/p2p/<relay-id>/p2p-circuit/p2p/<target-id>
func parseCircuitAddr(addr string) (relayPeer, targetPeer types.PeerID, err error) {
	// 查找 p2p-circuit 分隔符
	circuitIdx := strings.Index(addr, "/p2p-circuit")
	if circuitIdx == -1 {
		return "", "", fmt.Errorf("missing /p2p-circuit in address")
	}

	// 解析中继部分
	relayPart := addr[:circuitIdx]
	p2pIdx := strings.LastIndex(relayPart, "/p2p/")
	if p2pIdx == -1 {
		return "", "", fmt.Errorf("missing relay peer ID")
	}
	relayPeer = types.PeerID(relayPart[p2pIdx+5:])

	// 解析目标部分
	targetPart := addr[circuitIdx+len("/p2p-circuit"):]
	if strings.HasPrefix(targetPart, "/p2p/") {
		targetPeer = types.PeerID(targetPart[5:])
	}

	return relayPeer, targetPeer, nil
}

// BuildCircuitAddr 构建中继电路地址
func BuildCircuitAddr(relayAddr string, relayPeer, targetPeer types.PeerID) string {
	return fmt.Sprintf("%s/p2p/%s/p2p-circuit/p2p/%s", relayAddr, relayPeer, targetPeer)
}

// IsCircuitAddr 检查是否为中继电路地址
func IsCircuitAddr(addr string) bool {
	return strings.Contains(addr, "/p2p-circuit")
}
