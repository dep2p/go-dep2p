// Package holepunch 提供 NAT 打洞实现
//
// TCP 打洞使用 Simultaneous Open 技术：
// - 双方同时发起 TCP 连接
// - 使用 SO_REUSEADDR/SO_REUSEPORT 允许端口复用
// - 多轮尝试直到连接建立
package holepunch

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	natif "github.com/dep2p/go-dep2p/pkg/interfaces/nat"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              TCP 打洞错误
// ============================================================================

// TCP 打洞相关错误
var (
	// ErrTCPPunchFailed TCP 打洞失败
	ErrTCPPunchFailed = errors.New("TCP hole punch failed")
	ErrTCPNoAddresses    = errors.New("no TCP addresses to punch")
	ErrTCPTimeout        = errors.New("TCP hole punch timeout")
	ErrTCPNoPeerResponse = errors.New("no TCP response from peer")
)

// ============================================================================
//                              TCP 打洞配置
// ============================================================================

// TCPConfig TCP 打洞器配置
type TCPConfig struct {
	// MaxAttempts 最大尝试次数
	MaxAttempts int

	// AttemptInterval 尝试间隔
	AttemptInterval time.Duration

	// Timeout 总超时时间
	Timeout time.Duration

	// ConnectTimeout 单次连接超时
	ConnectTimeout time.Duration

	// LocalPort 本地端口（0 表示自动分配）
	LocalPort int

	// EnableReusePort 启用端口复用
	EnableReusePort bool
}

// DefaultTCPConfig 返回默认 TCP 配置
func DefaultTCPConfig() TCPConfig {
	return TCPConfig{
		MaxAttempts:     10,
		AttemptInterval: 100 * time.Millisecond,
		Timeout:         15 * time.Second,
		ConnectTimeout:  2 * time.Second,
		LocalPort:       0,
		EnableReusePort: true,
	}
}

// Validate 验证配置
func (c *TCPConfig) Validate() {
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 10
	}
	if c.AttemptInterval <= 0 {
		c.AttemptInterval = 100 * time.Millisecond
	}
	if c.Timeout <= 0 {
		c.Timeout = 15 * time.Second
	}
	if c.ConnectTimeout <= 0 {
		c.ConnectTimeout = 2 * time.Second
	}
}

// ============================================================================
//                              TCPPuncher 结构
// ============================================================================

// TCPPuncher TCP 打洞器实现
type TCPPuncher struct {
	config TCPConfig

	// 本地地址
	localAddrs []endpoint.Address

	// 活跃打洞会话
	sessions   map[string]*tcpPunchSession
	sessionsMu sync.RWMutex
}

// tcpPunchSession TCP 打洞会话
type tcpPunchSession struct {
	remoteID    types.NodeID
	remoteAddrs []endpoint.Address
	startTime   time.Time
	successConn net.Conn
	successAddr endpoint.Address
	done        chan struct{}
	err         error
}

// NewTCPPuncher 创建 TCP 打洞器
func NewTCPPuncher(config TCPConfig) *TCPPuncher {
	// 验证并修正配置
	config.Validate()

	return &TCPPuncher{
		config:   config,
		sessions: make(map[string]*tcpPunchSession),
	}
}

// 确保实现 TCPHolePuncher 接口
var _ natif.TCPHolePuncher = (*TCPPuncher)(nil)

// ============================================================================
//                              TCPHolePuncher 接口实现
// ============================================================================

// PunchTCP 实现 TCPHolePuncher 接口
func (p *TCPPuncher) PunchTCP(ctx context.Context, remoteID types.NodeID, remoteAddrs []endpoint.Address) (conn interface{}, addr endpoint.Address, err error) {
	return p.Punch(ctx, remoteID, remoteAddrs)
}

// PunchTCPWithLocalPort 实现 TCPHolePuncher 接口
func (p *TCPPuncher) PunchTCPWithLocalPort(ctx context.Context, remoteID types.NodeID, remoteAddrs []endpoint.Address, localPort int) (conn interface{}, addr endpoint.Address, err error) {
	return p.PunchWithLocalPort(ctx, remoteID, remoteAddrs, localPort)
}

// ============================================================================
//                              打洞核心方法
// ============================================================================

// Punch 尝试 TCP 打洞连接
func (p *TCPPuncher) Punch(ctx context.Context, remoteID types.NodeID, remoteAddrs []endpoint.Address) (net.Conn, endpoint.Address, error) {
	if len(remoteAddrs) == 0 {
		return nil, nil, ErrTCPNoAddresses
	}

	log.Info("开始 TCP 打洞",
		"remoteID", remoteID.ShortString(),
		"addresses", len(remoteAddrs))

	// 创建会话
	session := &tcpPunchSession{
		remoteID:    remoteID,
		remoteAddrs: remoteAddrs,
		startTime:   time.Now(),
		done:        make(chan struct{}),
	}

	sessionKey := remoteID.String()
	p.sessionsMu.Lock()
	p.sessions[sessionKey] = session
	p.sessionsMu.Unlock()

	defer func() {
		p.sessionsMu.Lock()
		delete(p.sessions, sessionKey)
		p.sessionsMu.Unlock()
	}()

	// 创建超时上下文
	punchCtx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	// 并行尝试所有地址
	type result struct {
		conn net.Conn
		addr endpoint.Address
		err  error
	}

	resultCh := make(chan result, len(remoteAddrs))

	var wg sync.WaitGroup
	for _, addr := range remoteAddrs {
		wg.Add(1)
		go func(addr endpoint.Address) {
			defer wg.Done()
			conn, successAddr, err := p.punchTCPAddress(punchCtx, addr)
			resultCh <- result{conn: conn, addr: successAddr, err: err}
		}(addr)
	}

	// 等待结果
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// 返回第一个成功的连接
	for {
		select {
		case <-punchCtx.Done():
			return nil, nil, fmt.Errorf("%w: %v", ErrTCPTimeout, punchCtx.Err())
		case res, ok := <-resultCh:
			if !ok {
				// 所有尝试都失败
				return nil, nil, ErrTCPNoPeerResponse
			}
			if res.err == nil && res.conn != nil {
				log.Info("TCP 打洞成功",
					"remoteID", remoteID.ShortString(),
					"address", res.addr.String(),
					"duration", time.Since(session.startTime))
				return res.conn, res.addr, nil
			}
		}
	}
}

// PunchWithLocalPort 使用指定本地端口尝试 TCP 打洞
func (p *TCPPuncher) PunchWithLocalPort(ctx context.Context, remoteID types.NodeID, remoteAddrs []endpoint.Address, localPort int) (net.Conn, endpoint.Address, error) {
	if len(remoteAddrs) == 0 {
		return nil, nil, ErrTCPNoAddresses
	}

	log.Info("开始 TCP 打洞（指定本地端口）",
		"remoteID", remoteID.ShortString(),
		"localPort", localPort,
		"addresses", len(remoteAddrs))

	// 创建超时上下文
	punchCtx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	// 并行尝试所有地址
	type result struct {
		conn net.Conn
		addr endpoint.Address
		err  error
	}

	resultCh := make(chan result, len(remoteAddrs))

	var wg sync.WaitGroup
	for _, addr := range remoteAddrs {
		wg.Add(1)
		go func(addr endpoint.Address) {
			defer wg.Done()
			conn, successAddr, err := p.punchTCPAddressWithLocalPort(punchCtx, addr, localPort)
			resultCh <- result{conn: conn, addr: successAddr, err: err}
		}(addr)
	}

	// 等待结果
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// 返回第一个成功的连接
	for {
		select {
		case <-punchCtx.Done():
			return nil, nil, fmt.Errorf("%w: %v", ErrTCPTimeout, punchCtx.Err())
		case res, ok := <-resultCh:
			if !ok {
				return nil, nil, ErrTCPNoPeerResponse
			}
			if res.err == nil && res.conn != nil {
				log.Info("TCP 打洞成功",
					"remoteID", remoteID.ShortString(),
					"address", res.addr.String())
				return res.conn, res.addr, nil
			}
		}
	}
}

// ============================================================================
//                              内部方法
// ============================================================================

// punchTCPAddress 对单个地址进行 TCP 打洞
func (p *TCPPuncher) punchTCPAddress(ctx context.Context, addr endpoint.Address) (net.Conn, endpoint.Address, error) {
	return p.punchTCPAddressWithLocalPort(ctx, addr, p.config.LocalPort)
}

// punchTCPAddressWithLocalPort 使用指定本地端口对单个地址进行 TCP 打洞
func (p *TCPPuncher) punchTCPAddressWithLocalPort(ctx context.Context, addr endpoint.Address, localPort int) (net.Conn, endpoint.Address, error) {
	// 解析远程地址
	tcpAddr, err := parseTCPAddr(addr)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid TCP address: %w", err)
	}

	// 多轮尝试（Simultaneous Open 需要多次尝试）
	for attempt := 0; attempt < p.config.MaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		default:
		}

		log.Debug("TCP 打洞尝试",
			"addr", addr.String(),
			"attempt", attempt+1,
			"localPort", localPort)

		// 创建可复用的本地地址
		var localAddr *net.TCPAddr
		if localPort > 0 {
			localAddr = &net.TCPAddr{Port: localPort}
		}

		// 创建带超时的连接
		conn, err := p.dialTCPWithReuse(ctx, localAddr, tcpAddr)
		if err != nil {
			log.Debug("TCP 打洞尝试失败",
				"addr", addr.String(),
				"attempt", attempt+1,
				"err", err)

			// 等待一段时间再重试
			select {
			case <-ctx.Done():
				return nil, nil, ctx.Err()
			case <-time.After(p.config.AttemptInterval):
				continue
			}
		}

		// 连接成功
		return conn, newTCPAddr(tcpAddr.IP, tcpAddr.Port), nil
	}

	return nil, nil, ErrTCPNoPeerResponse
}

// dialTCPWithReuse 使用 SO_REUSEADDR/SO_REUSEPORT 进行 TCP 拨号
func (p *TCPPuncher) dialTCPWithReuse(ctx context.Context, localAddr, remoteAddr *net.TCPAddr) (net.Conn, error) {
	// 创建 dialer 配置
	dialer := &net.Dialer{
		Timeout:   p.config.ConnectTimeout,
		LocalAddr: localAddr,
		Control:   p.reuseControl,
	}

	// 使用带上下文的拨号
	conn, err := dialer.DialContext(ctx, "tcp", remoteAddr.String())
	if err != nil {
		return nil, err
	}

	return conn, nil
}

// reuseControl 设置 SO_REUSEADDR 和 SO_REUSEPORT
func (p *TCPPuncher) reuseControl(_, _ string, c syscall.RawConn) error {
	var opErr error
	err := c.Control(func(fd uintptr) {
		// 设置 SO_REUSEADDR
		if err := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
			opErr = fmt.Errorf("set SO_REUSEADDR: %w", err)
			return
		}

		// 设置 SO_REUSEPORT（如果启用）
		if p.config.EnableReusePort {
			if err := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1); err != nil {
				log.Warn("设置 SO_REUSEPORT 失败（某些系统不支持）", "err", err)
				// 不返回错误，继续使用 SO_REUSEADDR
			}
		}
	})

	if err != nil {
		return err
	}
	return opErr
}

// ============================================================================
//                              辅助方法
// ============================================================================

// SetLocalAddrs 设置本地地址
func (p *TCPPuncher) SetLocalAddrs(addrs []endpoint.Address) {
	p.localAddrs = addrs
}

// GetLocalAddrs 获取本地地址
func (p *TCPPuncher) GetLocalAddrs() []endpoint.Address {
	return p.localAddrs
}

// parseTCPAddr 解析 TCP 地址
func parseTCPAddr(addr endpoint.Address) (*net.TCPAddr, error) {
	// 尝试类型断言
	if ta, ok := addr.(*tcpAddr); ok {
		return &net.TCPAddr{IP: ta.ip, Port: ta.port}, nil
	}

	// 尝试解析字符串
	return net.ResolveTCPAddr("tcp", addr.String())
}

// ============================================================================
//                              tcpAddr 实现（TCP 专用地址类型）
// ============================================================================

// tcpAddr TCP 地址实现
// 此类型实现 endpoint.Address 接口，专用于 TCP 打洞
type tcpAddr struct {
	ip   net.IP
	port int
}

// newTCPAddr 创建新的 TCP 地址
func newTCPAddr(ip net.IP, port int) *tcpAddr {
	return &tcpAddr{ip: ip, port: port}
}

func (a *tcpAddr) Network() string {
	return "tcp"
}

func (a *tcpAddr) String() string {
	if a.ip.To4() != nil {
		return fmt.Sprintf("%s:%d", a.ip.String(), a.port)
	}
	return fmt.Sprintf("[%s]:%d", a.ip.String(), a.port)
}

func (a *tcpAddr) Bytes() []byte {
	return []byte(a.String())
}

func (a *tcpAddr) Equal(other endpoint.Address) bool {
	if other == nil {
		return false
	}
	return a.String() == other.String()
}

func (a *tcpAddr) IsPublic() bool {
	// 0.0.0.0 和 :: 是未指定地址，不是公网地址
	return !a.IsPrivate() && !a.IsLoopback() && !a.ip.IsUnspecified()
}

func (a *tcpAddr) IsPrivate() bool {
	return a.ip.IsPrivate()
}

func (a *tcpAddr) IsLoopback() bool {
	return a.ip.IsLoopback()
}

func (a *tcpAddr) Multiaddr() string {
	ipType := "ip4"
	if a.ip.To4() == nil {
		ipType = "ip6"
	}
	return fmt.Sprintf("/%s/%s/tcp/%d", ipType, a.ip.String(), a.port)
}

// ToTCPAddr 转换为 net.TCPAddr
func (a *tcpAddr) ToTCPAddr() (*net.TCPAddr, error) {
	return &net.TCPAddr{IP: a.ip, Port: a.port}, nil
}

