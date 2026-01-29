package connector

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/nat/holepunch"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("realm/connector")

// ConnectStrategy 连接策略
type ConnectStrategy int

const (
	// StrategyAuto 自动选择最优策略
	StrategyAuto ConnectStrategy = iota
	// StrategyDirectOnly 仅直连
	StrategyDirectOnly
	// StrategyRelayOnly 仅 Relay
	StrategyRelayOnly
)

// ConnectResult 连接结果
type ConnectResult struct {
	// Conn 建立的连接
	Conn pkgif.Connection

	// Method 使用的连接方法
	Method ConnectMethod

	// Duration 连接耗时
	Duration time.Duration

	// Attempts 尝试次数
	Attempts int
}

// ConnectMethod 连接方法
type ConnectMethod string

const (
	MethodDirect    ConnectMethod = "direct"
	MethodHolePunch ConnectMethod = "holepunch"
	MethodRelay     ConnectMethod = "relay"
)

// Connector Realm 内连接器
//
// 实现"仅 ID 连接"能力：用户只需提供目标 NodeID，
// 系统自动完成地址解析和连接建立。
//
// v2.0 连接优先级：
//  1. 直连：有地址时直接连接
//  2. 打洞：直连失败时尝试 NAT 穿透
//
// 注意：Relay 功能已移至节点级别（统一 Relay 架构 v2.0）
// Realm Connector 不再直接管理 Relay 连接
type Connector struct {
	resolver    *AddressResolver
	host        pkgif.Host
	holePuncher *holepunch.HolePuncher

	// 配置
	config ConnectorConfig

	// 状态
	closed atomic.Bool
}

// ConnectorConfig 连接器配置
type ConnectorConfig struct {
	// DirectTimeout 直连超时
	DirectTimeout time.Duration

	// HolePunchTimeout 打洞超时
	HolePunchTimeout time.Duration

	// RelayTimeout Relay 连接超时
	RelayTimeout time.Duration

	// TotalTimeout 总超时
	TotalTimeout time.Duration

	// Strategy 连接策略
	Strategy ConnectStrategy

	// EnableHolePunch 是否启用打洞
	EnableHolePunch bool
}

// DefaultConnectorConfig 返回默认配置
func DefaultConnectorConfig() ConnectorConfig {
	return ConnectorConfig{
		DirectTimeout:    5 * time.Second,
		HolePunchTimeout: 10 * time.Second,
		RelayTimeout:     10 * time.Second,
		TotalTimeout:     30 * time.Second,
		Strategy:         StrategyAuto,
		EnableHolePunch:  true,
	}
}

// NewConnector 创建连接器
//
// v2.0 统一 Relay 架构：Connector 不再接受 RelayService 参数
// Relay 功能由节点级别的 RelayService 统一管理
func NewConnector(
	resolver *AddressResolver,
	host pkgif.Host,
	holePuncher *holepunch.HolePuncher,
	config ConnectorConfig,
) *Connector {
	if config.DirectTimeout <= 0 {
		config.DirectTimeout = 5 * time.Second
	}
	if config.HolePunchTimeout <= 0 {
		config.HolePunchTimeout = 10 * time.Second
	}
	if config.RelayTimeout <= 0 {
		config.RelayTimeout = 10 * time.Second
	}
	if config.TotalTimeout <= 0 {
		config.TotalTimeout = 30 * time.Second
	}

	return &Connector{
		resolver:    resolver,
		host:        host,
		holePuncher: holePuncher,
		config:      config,
	}
}

// Connect 使用纯 NodeID 连接目标节点
//
// 连接优先级：
//  1. 直连：有地址时尝试直接连接
//  2. 打洞：直连失败且启用打洞时尝试 NAT 穿透
//  3. Relay：保底方案，通过 Relay 中继连接
func (c *Connector) Connect(ctx context.Context, target string) (*ConnectResult, error) {
	if c.closed.Load() {
		return nil, ErrConnectorClosed
	}
	
	logger.Debug("连接节点", "target", log.TruncateID(target, 8))

	if target == "" {
		return nil, ErrInvalidTarget
	}

	startTime := time.Now()
	attempts := 0

	// 应用总超时
	ctx, cancel := context.WithTimeout(ctx, c.config.TotalTimeout)
	defer cancel()

	// 根据策略选择连接方式
	var result *ConnectResult
	var err error
	switch c.config.Strategy {
	case StrategyDirectOnly:
		logger.Debug("使用直连策略", "target", log.TruncateID(target, 8))
		result, err = c.connectDirect(ctx, target, startTime, &attempts)
	case StrategyRelayOnly:
		logger.Debug("使用 Relay 策略", "target", log.TruncateID(target, 8))
		result, err = c.connectViaRelay(ctx, target, startTime, &attempts)
	default:
		logger.Debug("使用自动策略", "target", log.TruncateID(target, 8))
		result, err = c.connectAuto(ctx, target, startTime, &attempts)
	}
	
	if err != nil {
		logger.Warn("连接失败", "target", log.TruncateID(target, 8), "strategy", c.config.Strategy, "attempts", attempts, "error", err)
	} else {
		logger.Info("连接成功", "target", log.TruncateID(target, 8), "method", result.Method, "duration", result.Duration, "attempts", result.Attempts)
	}
	return result, err
}

// ConnectWithHint 使用地址提示连接目标节点
//
// 优先使用提示地址尝试连接，失败后回退到自动发现流程。
func (c *Connector) ConnectWithHint(ctx context.Context, target string, hints []string) (*ConnectResult, error) {
	if c.closed.Load() {
		return nil, ErrConnectorClosed
	}

	if target == "" {
		return nil, ErrInvalidTarget
	}

	startTime := time.Now()
	attempts := 0

	// 应用总超时
	ctx, cancel := context.WithTimeout(ctx, c.config.TotalTimeout)
	defer cancel()

	// 解析提示地址
	var hintAddrs []types.Multiaddr
	for _, hint := range hints {
		addr, err := types.NewMultiaddr(hint)
		if err == nil && addr != nil {
			hintAddrs = append(hintAddrs, addr)
		}
	}

	// 如果有提示地址，优先尝试直连
	if len(hintAddrs) > 0 {
		attempts++
		conn, err := c.dialDirect(ctx, target, hintAddrs)
		if err == nil {
			return &ConnectResult{
				Conn:     conn,
				Method:   MethodDirect,
				Duration: time.Since(startTime),
				Attempts: attempts,
			}, nil
		}

		// 直连失败，尝试打洞
		if c.config.EnableHolePunch && c.holePuncher != nil {
			attempts++
			conn, err := c.dialHolePunch(ctx, target, hintAddrs)
			if err == nil {
				return &ConnectResult{
					Conn:     conn,
					Method:   MethodHolePunch,
					Duration: time.Since(startTime),
					Attempts: attempts,
				}, nil
			}
		}
	}

	// 提示地址失败或无提示，回退到自动发现流程
	return c.connectAuto(ctx, target, startTime, &attempts)
}

// connectAuto 自动选择最优连接方式
//
// v2.0 统一 Relay 架构：
//
//	1. 地址解析：Peerstore → MemberList → DHT → Relay 地址簿
//	2. 直连尝试：有地址时优先直连
//	3. 打洞尝试：直连失败时尝试 NAT 穿透
//	4. Relay 兜底：直连和打洞失败时，通过 RelayDialer 回退
func (c *Connector) connectAuto(ctx context.Context, target string, startTime time.Time, attempts *int) (*ConnectResult, error) {
	// 1. 解析地址
	var addrs []types.Multiaddr
	if c.resolver != nil {
		result, err := c.resolver.Resolve(ctx, target)
		if err != nil {
			return nil, fmt.Errorf("resolve address: %w", err)
		}
		if result.HasAddrs() {
			addrs = result.Addrs
		}
	}

	// 2. 如果有地址，尝试直连
	if len(addrs) > 0 {
		*attempts++
		conn, err := c.dialDirect(ctx, target, addrs)
		if err == nil {
			return &ConnectResult{
				Conn:     conn,
				Method:   MethodDirect,
				Duration: time.Since(startTime),
				Attempts: *attempts,
			}, nil
		}

		// 3. 直连失败，尝试打洞
		if c.config.EnableHolePunch && c.holePuncher != nil {
			*attempts++
			conn, err := c.dialHolePunch(ctx, target, addrs)
			if err == nil {
				return &ConnectResult{
					Conn:     conn,
					Method:   MethodHolePunch,
					Duration: time.Since(startTime),
					Attempts: *attempts,
				}, nil
			}
		}
	}

	// 4. 直连和打洞都失败，尝试 Relay 兜底
	return c.connectViaRelay(ctx, target, startTime, attempts)
}

// connectDirect 仅直连
func (c *Connector) connectDirect(ctx context.Context, target string, startTime time.Time, attempts *int) (*ConnectResult, error) {
	// 解析地址
	if c.resolver == nil {
		return nil, ErrNoAddress
	}

	result, err := c.resolver.Resolve(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("resolve address: %w", err)
	}

	if !result.HasAddrs() {
		return nil, ErrNoAddress
	}

	*attempts++
	conn, err := c.dialDirect(ctx, target, result.Addrs)
	if err != nil {
		return nil, fmt.Errorf("direct connect failed: %w", err)
	}

	return &ConnectResult{
		Conn:     conn,
		Method:   MethodDirect,
		Duration: time.Since(startTime),
		Attempts: *attempts,
	}, nil
}

// connectViaRelay 仅 Relay
//
// v2.0 统一 Relay 架构：通过节点级别 RelayDialer 连接
func (c *Connector) connectViaRelay(ctx context.Context, target string, startTime time.Time, attempts *int) (*ConnectResult, error) {
	*attempts++
	dialer := c.getRelayDialer()
	if dialer == nil || !dialer.HasRelay() {
		return nil, ErrNoRelayAvailable
	}

	dialCtx, cancel := context.WithTimeout(ctx, c.config.RelayTimeout)
	defer cancel()

	conn, err := dialer.DialViaRelay(dialCtx, target)
	if err != nil {
		return nil, fmt.Errorf("relay connect failed: %w", err)
	}

	return &ConnectResult{
		Conn:     conn,
		Method:   MethodRelay,
		Duration: time.Since(startTime),
		Attempts: *attempts,
	}, nil
}

func (c *Connector) getRelayDialer() pkgif.RelayDialer {
	if c.host == nil {
		return nil
	}

	network := c.host.Network()
	if network == nil {
		return nil
	}

	type relayDialerGetter interface {
		RelayDialer() pkgif.RelayDialer
	}

	if getter, ok := network.(relayDialerGetter); ok {
		return getter.RelayDialer()
	}

	return nil
}

// dialDirect 直接连接
func (c *Connector) dialDirect(ctx context.Context, target string, addrs []types.Multiaddr) (pkgif.Connection, error) {
	if c.host == nil {
		return nil, ErrDirectConnectFailed
	}

	// 应用直连超时
	dialCtx, cancel := context.WithTimeout(ctx, c.config.DirectTimeout)
	defer cancel()

	// 转换地址为字符串
	addrStrs := make([]string, len(addrs))
	for i, addr := range addrs {
		addrStrs[i] = addr.String()
	}

	// 尝试连接
	err := c.host.Connect(dialCtx, target, addrStrs)
	if err != nil {
		return nil, fmt.Errorf("host connect: %w", err)
	}

	// Host.Connect 成功后，获取连接
	// 注意：Host.Connect 可能不返回 Connection，需要从 Swarm 获取
	// 这里简化处理，返回一个包装的连接信息
	return &connWrapper{
		remotePeer: types.PeerID(target),
		method:     MethodDirect,
	}, nil
}

// dialHolePunch 打洞连接
func (c *Connector) dialHolePunch(ctx context.Context, target string, addrs []types.Multiaddr) (pkgif.Connection, error) {
	if c.holePuncher == nil {
		return nil, ErrHolePunchFailed
	}

	// 应用打洞超时
	punchCtx, cancel := context.WithTimeout(ctx, c.config.HolePunchTimeout)
	defer cancel()

	// 转换地址为字符串
	addrStrs := make([]string, len(addrs))
	for i, addr := range addrs {
		addrStrs[i] = addr.String()
	}

	// 尝试打洞
	err := c.holePuncher.DirectConnect(punchCtx, target, addrStrs)
	if err != nil {
		return nil, fmt.Errorf("hole punch: %w", err)
	}

	return &connWrapper{
		remotePeer: types.PeerID(target),
		method:     MethodHolePunch,
	}, nil
}

// Close 关闭连接器
func (c *Connector) Close() error {
	c.closed.Store(true)
	return nil
}

// SetHost 设置 Host
func (c *Connector) SetHost(host pkgif.Host) {
	c.host = host
}

// SetHolePuncher 设置 HolePuncher
func (c *Connector) SetHolePuncher(puncher *holepunch.HolePuncher) {
	c.holePuncher = puncher
}

// connWrapper 连接包装器
//
// 用于直连和打洞成功后返回连接信息。
// 实际使用时应该从 Swarm 获取真实的 Connection。
type connWrapper struct {
	remotePeer types.PeerID
	method     ConnectMethod
}

func (w *connWrapper) LocalPeer() types.PeerID {
	return ""
}

func (w *connWrapper) LocalMultiaddr() types.Multiaddr {
	return nil
}

func (w *connWrapper) RemotePeer() types.PeerID {
	return w.remotePeer
}

func (w *connWrapper) RemoteMultiaddr() types.Multiaddr {
	return nil
}

func (w *connWrapper) NewStream(_ context.Context) (pkgif.Stream, error) {
	return nil, fmt.Errorf("not implemented: use host.NewStream instead")
}

func (w *connWrapper) AcceptStream() (pkgif.Stream, error) {
	return nil, fmt.Errorf("not implemented")
}

func (w *connWrapper) GetStreams() []pkgif.Stream {
	return nil
}

func (w *connWrapper) Stat() pkgif.ConnectionStat {
	return pkgif.ConnectionStat{}
}

func (w *connWrapper) Close() error {
	return nil
}

func (w *connWrapper) IsClosed() bool {
	return false
}

func (w *connWrapper) ConnType() pkgif.ConnectionType {
	if w.method == MethodRelay {
		return pkgif.ConnectionTypeRelay
	}
	return pkgif.ConnectionTypeDirect
}
