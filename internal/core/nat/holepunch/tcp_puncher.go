// Package holepunch 提供 NAT 打洞实现
//
// TCP 打洞使用 Simultaneous Open 技术：
//   - 双方同时发起 TCP 连接
//   - 使用 SO_REUSEADDR/SO_REUSEPORT 允许端口复用
//   - 多轮尝试直到连接建立
package holepunch

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

// TCP 打洞日志
var tcpLogger = log.Logger("nat/holepunch/tcp")

// ============================================================================
//                              TCP 打洞错误
// ============================================================================

var (
	// ErrTCPPunchFailed TCP 打洞失败
	ErrTCPPunchFailed = errors.New("TCP hole punch failed")

	// ErrTCPNoAddresses 没有 TCP 地址
	ErrTCPNoAddresses = errors.New("no TCP addresses to punch")

	// ErrTCPTimeout TCP 打洞超时
	ErrTCPTimeout = errors.New("TCP hole punch timeout")

	// ErrTCPNoPeerResponse 没有收到对端响应
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

	// 活跃打洞会话
	sessions   map[string]*tcpPunchSession
	sessionsMu sync.RWMutex
}

// tcpPunchSession TCP 打洞会话
type tcpPunchSession struct {
	remoteID    string
	remoteAddrs []string
	startTime   time.Time
	done        chan struct{}
}

// NewTCPPuncher 创建 TCP 打洞器
func NewTCPPuncher(config TCPConfig) *TCPPuncher {
	config.Validate()

	return &TCPPuncher{
		config:   config,
		sessions: make(map[string]*tcpPunchSession),
	}
}

// ============================================================================
//                              打洞核心方法
// ============================================================================

// Punch 尝试 TCP 打洞连接
//
// 参数:
//   - ctx: 上下文，支持超时和取消
//   - remoteID: 远程节点 ID
//   - remoteAddrs: 远程节点 TCP 地址列表
//
// 返回:
//   - net.Conn: 成功建立的 TCP 连接
//   - string: 成功连接的地址
//   - error: 错误信息
func (p *TCPPuncher) Punch(ctx context.Context, remoteID string, remoteAddrs []string) (net.Conn, string, error) {
	if len(remoteAddrs) == 0 {
		return nil, "", ErrTCPNoAddresses
	}

	tcpLogger.Info("开始 TCP 打洞",
		"remoteID", remoteID,
		"addresses", len(remoteAddrs))

	// 创建会话
	session := &tcpPunchSession{
		remoteID:    remoteID,
		remoteAddrs: remoteAddrs,
		startTime:   time.Now(),
		done:        make(chan struct{}),
	}

	p.sessionsMu.Lock()
	p.sessions[remoteID] = session
	p.sessionsMu.Unlock()

	defer func() {
		p.sessionsMu.Lock()
		delete(p.sessions, remoteID)
		p.sessionsMu.Unlock()
	}()

	// 创建超时上下文
	punchCtx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	// 并行尝试所有地址
	type result struct {
		conn net.Conn
		addr string
		err  error
	}

	resultCh := make(chan result, len(remoteAddrs))

	var wg sync.WaitGroup
	for _, addr := range remoteAddrs {
		wg.Add(1)
		go func(addr string) {
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
			return nil, "", fmt.Errorf("%w: %v", ErrTCPTimeout, punchCtx.Err())
		case res, ok := <-resultCh:
			if !ok {
				return nil, "", ErrTCPNoPeerResponse
			}
			if res.err == nil && res.conn != nil {
				tcpLogger.Info("TCP 打洞成功",
					"remoteID", remoteID,
					"address", res.addr,
					"duration", time.Since(session.startTime))
				return res.conn, res.addr, nil
			}
		}
	}
}

// PunchWithLocalPort 使用指定本地端口尝试 TCP 打洞
func (p *TCPPuncher) PunchWithLocalPort(ctx context.Context, remoteID string, remoteAddrs []string, localPort int) (net.Conn, string, error) {
	if len(remoteAddrs) == 0 {
		return nil, "", ErrTCPNoAddresses
	}

	tcpLogger.Info("开始 TCP 打洞（指定本地端口）",
		"remoteID", remoteID,
		"localPort", localPort,
		"addresses", len(remoteAddrs))

	// 创建超时上下文
	punchCtx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	// 并行尝试所有地址
	type result struct {
		conn net.Conn
		addr string
		err  error
	}

	resultCh := make(chan result, len(remoteAddrs))

	var wg sync.WaitGroup
	for _, addr := range remoteAddrs {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			conn, successAddr, err := p.punchTCPAddressWithLocalPort(punchCtx, addr, localPort)
			resultCh <- result{conn: conn, addr: successAddr, err: err}
		}(addr)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	for {
		select {
		case <-punchCtx.Done():
			return nil, "", fmt.Errorf("%w: %v", ErrTCPTimeout, punchCtx.Err())
		case res, ok := <-resultCh:
			if !ok {
				return nil, "", ErrTCPNoPeerResponse
			}
			if res.err == nil && res.conn != nil {
				tcpLogger.Info("TCP 打洞成功",
					"remoteID", remoteID,
					"address", res.addr)
				return res.conn, res.addr, nil
			}
		}
	}
}

// ============================================================================
//                              内部方法
// ============================================================================

// punchTCPAddress 对单个地址进行 TCP 打洞
func (p *TCPPuncher) punchTCPAddress(ctx context.Context, addr string) (net.Conn, string, error) {
	return p.punchTCPAddressWithLocalPort(ctx, addr, p.config.LocalPort)
}

// punchTCPAddressWithLocalPort 使用指定本地端口对单个地址进行 TCP 打洞
func (p *TCPPuncher) punchTCPAddressWithLocalPort(ctx context.Context, addr string, localPort int) (net.Conn, string, error) {
	// 解析远程地址
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return nil, "", fmt.Errorf("invalid TCP address: %w", err)
	}

	// 多轮尝试（Simultaneous Open 需要多次尝试）
	for attempt := 0; attempt < p.config.MaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return nil, "", ctx.Err()
		default:
		}

		tcpLogger.Debug("TCP 打洞尝试",
			"addr", addr,
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
			tcpLogger.Debug("TCP 打洞尝试失败",
				"addr", addr,
				"attempt", attempt+1,
				"err", err)

			select {
			case <-ctx.Done():
				return nil, "", ctx.Err()
			case <-time.After(p.config.AttemptInterval):
				continue
			}
		}

		// 连接成功
		return conn, addr, nil
	}

	return nil, "", ErrTCPNoPeerResponse
}

// dialTCPWithReuse 使用 SO_REUSEADDR/SO_REUSEPORT 进行 TCP 拨号
func (p *TCPPuncher) dialTCPWithReuse(ctx context.Context, localAddr, remoteAddr *net.TCPAddr) (net.Conn, error) {
	dialer := &net.Dialer{
		Timeout:   p.config.ConnectTimeout,
		LocalAddr: localAddr,
		Control:   p.reuseControl,
	}

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
		if err := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
			opErr = fmt.Errorf("set SO_REUSEADDR: %w", err)
			return
		}

		// 设置 SO_REUSEPORT（如果启用）
		// 注意：某些系统可能不支持
		if p.config.EnableReusePort {
			// SO_REUSEPORT 在 syscall 包中可能不存在，使用常量值
			const SO_REUSEPORT = 0xf // unix.SO_REUSEPORT
			if err := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, SO_REUSEPORT, 1); err != nil {
				tcpLogger.Debug("设置 SO_REUSEPORT 失败（某些系统不支持）", "err", err)
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

// IsActive 检查是否有活跃的打洞会话
func (p *TCPPuncher) IsActive(remoteID string) bool {
	p.sessionsMu.RLock()
	defer p.sessionsMu.RUnlock()
	_, exists := p.sessions[remoteID]
	return exists
}

// ActiveCount 返回活跃打洞会话数量
func (p *TCPPuncher) ActiveCount() int {
	p.sessionsMu.RLock()
	defer p.sessionsMu.RUnlock()
	return len(p.sessions)
}
