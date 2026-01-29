package stun

import (
	"context"
	"net"
	"sync"
	"time"

	"github.com/pion/stun"
)

// STUNClient STUN 客户端
type STUNClient struct {
	servers []string
	timeout time.Duration
	retries int

	mu            sync.RWMutex
	cachedAddr    *net.UDPAddr
	cachedTime    time.Time
	cacheDuration time.Duration

	// 用于测试的钩子函数
	queryFunc func() (*net.UDPAddr, error)
}

// NewSTUNClient 创建 STUN 客户端
func NewSTUNClient(servers []string) *STUNClient {
	return &STUNClient{
		servers:       servers,
		timeout:       5 * time.Second,
		retries:       3,
		cacheDuration: 5 * time.Minute,
	}
}

// GetExternalAddr 获取外部地址
func (s *STUNClient) GetExternalAddr(ctx context.Context) (*net.UDPAddr, error) {
	// 检查缓存
	if addr := s.getCachedAddr(); addr != nil {
		return addr, nil
	}

	// 使用测试钩子
	if s.queryFunc != nil {
		addr, err := s.queryFunc()
		if err == nil {
			s.setCachedAddr(addr)
		}
		return addr, err
	}

	// 检查是否有服务器
	if len(s.servers) == 0 {
		return nil, ErrNoServers
	}

	// 检查上下文
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	// 尝试查询服务器
	for _, server := range s.servers {
		for retry := 0; retry < s.retries; retry++ {
			addr, err := s.queryServer(ctx, server)
			if err == nil {
				s.setCachedAddr(addr)
				return addr, nil
			}

			// 指数退避
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(1<<retry) * time.Second):
			}
		}
	}

	return nil, ErrTimeout
}

// queryServer 查询单个 STUN 服务器
func (s *STUNClient) queryServer(ctx context.Context, server string) (*net.UDPAddr, error) {
	// 1. 解析服务器地址
	addr, err := net.ResolveUDPAddr("udp", server)
	if err != nil {
		return nil, &STUNError{Message: "resolve server address", Cause: err}
	}

	// 2. 创建 UDP 连接
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, &STUNError{Message: "dial server", Cause: err}
	}
	defer conn.Close()

	// 
	// 这确保在服务关闭时能够快速退出，避免 Fx 超时
	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	// 3. 设置超时
	deadline := time.Now().Add(s.timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	conn.SetDeadline(deadline)

	// 4. 构造 STUN Binding Request
	msg, err := stun.Build(stun.TransactionID, stun.BindingRequest)
	if err != nil {
		return nil, &STUNError{Message: "build request", Cause: err}
	}

	// 5. 发送请求
	if _, err := msg.WriteTo(conn); err != nil {
		return nil, &STUNError{Message: "send request", Cause: err}
	}

	// 6. 读取响应
	buf := make([]byte, 1500)
	n, err := conn.Read(buf)
	if err != nil {
		// 
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, &STUNError{Message: "read response", Cause: err}
	}

	// 7. 解析响应
	res := new(stun.Message)
	res.Raw = buf[:n]
	if err := res.Decode(); err != nil {
		return nil, &STUNError{Message: "decode response", Cause: err}
	}

	// 8. 提取 XOR-MAPPED-ADDRESS
	var xorAddr stun.XORMappedAddress
	if err := xorAddr.GetFrom(res); err != nil {
		// 尝试 MAPPED-ADDRESS（旧版 STUN）
		var mappedAddr stun.MappedAddress
		if err := mappedAddr.GetFrom(res); err != nil {
			return nil, &STUNError{Message: "no mapped address in response", Cause: err}
		}
		return &net.UDPAddr{
			IP:   mappedAddr.IP,
			Port: mappedAddr.Port,
		}, nil
	}

	return &net.UDPAddr{
		IP:   xorAddr.IP,
		Port: xorAddr.Port,
	}, nil
}

// getCachedAddr 获取缓存的地址
func (s *STUNClient) getCachedAddr() *net.UDPAddr {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.cachedAddr != nil && time.Since(s.cachedTime) < s.cacheDuration {
		return s.cachedAddr
	}

	return nil
}

// setCachedAddr 设置缓存的地址
func (s *STUNClient) setCachedAddr(addr *net.UDPAddr) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cachedAddr = addr
	s.cachedTime = time.Now()
}

// SetCacheDuration 设置缓存时间（用于测试）
func (s *STUNClient) SetCacheDuration(d time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cacheDuration = d
}

// SetQueryFunc 设置查询函数（用于测试）
func (s *STUNClient) SetQueryFunc(f func() (*net.UDPAddr, error)) {
	s.queryFunc = f
}

// Errors
var (
	ErrNoServers = &STUNError{Message: "no STUN servers"}
	ErrTimeout   = &STUNError{Message: "STUN request timeout"}
	ErrInvalid   = &STUNError{Message: "invalid STUN response"}
)

// STUNError STUN 错误
type STUNError struct {
	Message string
	Cause   error
}

func (e *STUNError) Error() string {
	if e.Cause != nil {
		return "stun: " + e.Message + ": " + e.Cause.Error()
	}
	return "stun: " + e.Message
}

func (e *STUNError) Unwrap() error {
	return e.Cause
}
