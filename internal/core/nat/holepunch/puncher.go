// Package holepunch 提供 NAT 打洞实现
//
// 打洞器用于穿透 NAT 建立直接连接：
// - UDP/QUIC 同时发送
// - 多地址尝试
// - 超时重试
package holepunch

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	natif "github.com/dep2p/go-dep2p/pkg/interfaces/nat"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 包级别日志实例
var log = logger.Logger("nat.holepunch")

// ============================================================================
//                              错误定义
// ============================================================================

// 打洞相关错误
var (
	// ErrPunchFailed 打洞失败
	ErrPunchFailed = errors.New("hole punch failed")
	ErrNoAddresses    = errors.New("no addresses to punch")
	ErrTimeout        = errors.New("hole punch timeout")
	ErrNoPeerResponse = errors.New("no response from peer")
)

// ============================================================================
//                              配置
// ============================================================================

// Config 打洞器配置
type Config struct {
	// MaxAttempts 最大尝试次数
	MaxAttempts int

	// AttemptInterval 尝试间隔
	AttemptInterval time.Duration

	// Timeout 总超时时间
	Timeout time.Duration

	// PacketSize 打洞包大小
	PacketSize int
}

// DefaultConfig 返回默认配置
func DefaultConfig() Config {
	return Config{
		MaxAttempts:     5,
		AttemptInterval: 200 * time.Millisecond,
		Timeout:         10 * time.Second,
		PacketSize:      64,
	}
}

// MinPacketSize 打洞包最小大小（4字节 magic + 16字节 nonce）
const MinPacketSize = 20

// Validate 验证配置
func (c *Config) Validate() {
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 5
	}
	if c.AttemptInterval <= 0 {
		c.AttemptInterval = 200 * time.Millisecond
	}
	if c.Timeout <= 0 {
		c.Timeout = 10 * time.Second
	}
	// 确保包大小足够容纳 magic (4) + nonce (16)
	if c.PacketSize < MinPacketSize {
		c.PacketSize = 64
	}
}

// ============================================================================
//                              Puncher 结构
// ============================================================================

// Puncher 打洞器实现
type Puncher struct {
	config Config

	// 本地地址
	localAddrs []endpoint.Address

	// 活跃打洞会话
	sessions   map[string]*punchSession
	sessionsMu sync.RWMutex
}

// punchSession 打洞会话
type punchSession struct {
	remoteID    types.NodeID
	remoteAddrs []endpoint.Address
	nonce       []byte
	startTime   time.Time
	successAddr endpoint.Address
	done        chan struct{}
	err         error
	completed   bool // 标记会话是否已完成，防止重复关闭 done channel
}

// 确保实现接口
var _ natif.HolePuncher = (*Puncher)(nil)

// NewPuncher 创建打洞器
func NewPuncher(config Config) *Puncher {
	// 验证并修正配置
	config.Validate()

	return &Puncher{
		config:   config,
		sessions: make(map[string]*punchSession),
	}
}

// ============================================================================
//                              HolePuncher 接口实现
// ============================================================================

// Punch 尝试打洞连接
func (p *Puncher) Punch(ctx context.Context, remoteID types.NodeID, remoteAddrs []endpoint.Address) (endpoint.Address, error) {
	if len(remoteAddrs) == 0 {
		return nil, ErrNoAddresses
	}

	log.Info("开始打洞",
		"remoteID", remoteID.ShortString(),
		"addresses", len(remoteAddrs))

	// 创建会话
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	session := &punchSession{
		remoteID:    remoteID,
		remoteAddrs: remoteAddrs,
		nonce:       nonce,
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
	resultCh := make(chan endpoint.Address, len(remoteAddrs))
	errCh := make(chan error, len(remoteAddrs))

	var wg sync.WaitGroup
	for _, addr := range remoteAddrs {
		wg.Add(1)
		go func(addr endpoint.Address) {
			defer wg.Done()
			if successAddr, err := p.punchAddress(punchCtx, addr, nonce); err == nil {
				resultCh <- successAddr
			} else {
				errCh <- err
			}
		}(addr)
	}

	// 等待结果
	go func() {
		wg.Wait()
		close(resultCh)
		close(errCh)
	}()

	// 返回第一个成功的地址
	select {
	case <-punchCtx.Done():
		return nil, fmt.Errorf("%w: %v", ErrTimeout, punchCtx.Err())
	case addr := <-resultCh:
		if addr != nil {
			log.Info("打洞成功",
				"remoteID", remoteID.ShortString(),
				"address", addr.String(),
				"duration", time.Since(session.startTime))
			return addr, nil
		}
	}

	// 收集所有错误
	var lastErr error
	for err := range errCh {
		lastErr = err
	}

	if lastErr != nil {
		return nil, fmt.Errorf("%w: %v", ErrPunchFailed, lastErr)
	}

	return nil, ErrNoPeerResponse
}

// StartRendezvous 启动打洞协调
func (p *Puncher) StartRendezvous(ctx context.Context, remoteID types.NodeID) error {
	log.Debug("启动打洞协调",
		"remoteID", remoteID.ShortString())

	// 创建会话
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("failed to generate nonce: %w", err)
	}

	session := &punchSession{
		remoteID:  remoteID,
		nonce:     nonce,
		startTime: time.Now(),
		done:      make(chan struct{}),
	}

	sessionKey := remoteID.String()
	p.sessionsMu.Lock()
	p.sessions[sessionKey] = session
	p.sessionsMu.Unlock()

	// 等待打洞完成或超时
	select {
	case <-ctx.Done():
		p.sessionsMu.Lock()
		delete(p.sessions, sessionKey)
		p.sessionsMu.Unlock()
		return ctx.Err()
	case <-session.done:
		p.sessionsMu.Lock()
		delete(p.sessions, sessionKey)
		p.sessionsMu.Unlock()
		return session.err
	}
}

// ============================================================================
//                              内部方法
// ============================================================================

// punchAddress 对单个地址进行打洞
func (p *Puncher) punchAddress(ctx context.Context, addr endpoint.Address, nonce []byte) (endpoint.Address, error) {
	// 解析地址
	udpAddr, err := parseUDPAddr(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid address: %w", err)
	}

	// 创建 UDP 连接
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return nil, fmt.Errorf("dial failed: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// 构建打洞包
	packet := p.buildPunchPacket(nonce)

	// 多轮尝试
	for attempt := 0; attempt < p.config.MaxAttempts; attempt++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// 发送打洞包
		if _, err := conn.Write(packet); err != nil {
			log.Debug("发送打洞包失败",
				"addr", addr.String(),
				"attempt", attempt+1,
				"err", err)
			continue
		}

		log.Debug("发送打洞包",
			"addr", addr.String(),
			"attempt", attempt+1)

		// 设置读取超时
		_ = conn.SetReadDeadline(time.Now().Add(p.config.AttemptInterval))

		// 尝试接收响应
		buf := make([]byte, 128)
		n, remoteAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			// 超时继续下一轮
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			continue
		}

		// 验证响应
		if n > 0 && p.validatePunchResponse(buf[:n], nonce) {
			// 打洞成功
			return newIPAddr(remoteAddr.IP, remoteAddr.Port), nil
		}
	}

	return nil, ErrNoPeerResponse
}

// buildPunchPacket 构建打洞包
func (p *Puncher) buildPunchPacket(nonce []byte) []byte {
	// 简单的打洞包格式：
	// [4 bytes: magic] [16 bytes: nonce] [padding]
	magic := []byte("P2PH") // P2P Hole punch

	packet := make([]byte, p.config.PacketSize)
	copy(packet[0:4], magic)
	copy(packet[4:20], nonce)

	return packet
}

// validatePunchResponse 验证打洞响应
func (p *Puncher) validatePunchResponse(data, expectedNonce []byte) bool {
	if len(data) < 20 {
		return false
	}

	// 检查 magic
	magic := string(data[0:4])
	if magic != "P2PH" && magic != "P2PR" { // P2P Response
		return false
	}

	// 检查 nonce
	for i := 0; i < 16 && i < len(expectedNonce); i++ {
		if data[4+i] != expectedNonce[i] {
			return false
		}
	}

	return true
}

// ============================================================================
//                              辅助方法
// ============================================================================

// SetLocalAddrs 设置本地地址
func (p *Puncher) SetLocalAddrs(addrs []endpoint.Address) {
	p.localAddrs = addrs
}

// GetLocalAddrs 获取本地地址
func (p *Puncher) GetLocalAddrs() []endpoint.Address {
	return p.localAddrs
}

// GetSession 获取会话
func (p *Puncher) GetSession(remoteID types.NodeID) *punchSession {
	p.sessionsMu.RLock()
	defer p.sessionsMu.RUnlock()
	return p.sessions[remoteID.String()]
}

// CompleteSession 完成会话
func (p *Puncher) CompleteSession(remoteID types.NodeID, addr endpoint.Address, err error) {
	p.sessionsMu.Lock()
	session, ok := p.sessions[remoteID.String()]
	if ok && !session.completed {
		// 在持有锁的情况下设置结果并关闭 done channel
		// 使用 completed 标志防止重复关闭
		session.successAddr = addr
		session.err = err
		session.completed = true
		close(session.done)
	}
	p.sessionsMu.Unlock()
}

// parseUDPAddr 解析 UDP 地址
func parseUDPAddr(addr endpoint.Address) (*net.UDPAddr, error) {
	// 尝试类型断言
	if ia, ok := addr.(*ipAddr); ok {
		return &net.UDPAddr{IP: ia.ip, Port: ia.port}, nil
	}

	// 尝试解析字符串
	return net.ResolveUDPAddr("udp", addr.String())
}

// ============================================================================
//                              ipAddr 实现（统一地址类型）
// ============================================================================

// ipAddr 统一的 IP 地址实现
// 此类型实现 endpoint.Address 接口，替代所有散落的 Address 实现
type ipAddr struct {
	ip   net.IP
	port int
}

// newIPAddr 创建新的 IP 地址
func newIPAddr(ip net.IP, port int) *ipAddr {
	return &ipAddr{ip: ip, port: port}
}

func (a *ipAddr) Network() string {
	if a.ip.To4() != nil {
		return "ip4"
	}
	return "ip6"
}

func (a *ipAddr) String() string {
	if a.ip.To4() != nil {
		return fmt.Sprintf("%s:%d", a.ip.String(), a.port)
	}
	return fmt.Sprintf("[%s]:%d", a.ip.String(), a.port)
}

func (a *ipAddr) Bytes() []byte {
	return []byte(a.String())
}

func (a *ipAddr) Equal(other endpoint.Address) bool {
	if other == nil {
		return false
	}
	return a.String() == other.String()
}

func (a *ipAddr) IsPublic() bool {
	// 0.0.0.0 和 :: 是未指定地址，不是公网地址
	return !a.IsPrivate() && !a.IsLoopback() && !a.ip.IsUnspecified()
}

func (a *ipAddr) IsPrivate() bool {
	return a.ip.IsPrivate()
}

func (a *ipAddr) IsLoopback() bool {
	return a.ip.IsLoopback()
}

func (a *ipAddr) Multiaddr() string {
	ipType := "ip4"
	if a.ip.To4() == nil {
		ipType = "ip6"
	}
	return fmt.Sprintf("/%s/%s/udp/%d/quic-v1", ipType, a.ip.String(), a.port)
}

// ToUDPAddr 转换为 net.UDPAddr
func (a *ipAddr) ToUDPAddr() (*net.UDPAddr, error) {
	return &net.UDPAddr{IP: a.ip, Port: a.port}, nil
}

