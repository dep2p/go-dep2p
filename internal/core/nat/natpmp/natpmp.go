package natpmp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"
	
	natpmp "github.com/jackpal/go-nat-pmp"
	"github.com/jackpal/gateway"
	
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("nat/natpmp")

// NATPMPMapper NAT-PMP 端口映射器
type NATPMPMapper struct {
	client   *natpmp.Client
	gateway  net.IP
	mappings map[int]*Mapping
	mu       sync.RWMutex
	
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	
	// 21: 操作超时配置
	timeout time.Duration
	
	// 地址发现集成
	coordinator   pkgif.ReachabilityCoordinator // 可达性协调器（用于上报候选地址）
	coordinatorMu sync.RWMutex
}

// Mapping 端口映射记录
type Mapping struct {
	Protocol     string
	InternalPort int
	ExternalPort int
	Duration     int
	CreatedAt    time.Time
}

// DefaultNATPMPTimeout 默认 NAT-PMP 超时
const DefaultNATPMPTimeout = 5 * time.Second

// NewNATPMPMapper 创建 NAT-PMP 映射器
//
// 21: 使用默认超时，兼容旧代码
func NewNATPMPMapper() (*NATPMPMapper, error) {
	return NewNATPMPMapperWithTimeout(DefaultNATPMPTimeout)
}

// NewNATPMPMapperWithTimeout 创建带超时配置的 NAT-PMP 映射器
//
// 21: 添加可配置的超时参数，避免长时间阻塞
//
// 参数:
//   - timeout: 操作超时时间（网关发现、外部地址获取、端口映射）
func NewNATPMPMapperWithTimeout(timeout time.Duration) (*NATPMPMapper, error) {
	if timeout <= 0 {
		timeout = DefaultNATPMPTimeout
	}

	// 1. 发现默认网关（带超时）
	gatewayIPCh := make(chan net.IP, 1)
	errCh := make(chan error, 1)
	
	go func() {
		ip, err := gateway.DiscoverGateway()
		if err != nil {
			errCh <- err
			return
		}
		gatewayIPCh <- ip
	}()
	
	var gatewayIP net.IP
	select {
	case gatewayIP = <-gatewayIPCh:
		// 成功
	case err := <-errCh:
		return nil, &NATPMPError{
			Message: "discover gateway",
			Cause:   err,
		}
	case <-time.After(timeout):
		return nil, &NATPMPError{
			Message: "discover gateway",
			Cause:   fmt.Errorf("timeout after %v", timeout),
		}
	}
	
	// 2. 创建 NAT-PMP 客户端
	client := natpmp.NewClientWithTimeout(gatewayIP, timeout)
	
	// 3. 测试连接（获取外部地址）
	// 使用 goroutine + channel 实现超时控制
	resultCh := make(chan *natpmp.GetExternalAddressResult, 1)
	testErrCh := make(chan error, 1)
	
	go func() {
		result, err := client.GetExternalAddress()
		if err != nil {
			testErrCh <- err
			return
		}
		resultCh <- result
	}()
	
	select {
	case <-resultCh:
		// 连接测试成功
	case err := <-testErrCh:
		return nil, &NATPMPError{
			Message: "test connection",
			Cause:   err,
		}
	case <-time.After(timeout):
		return nil, &NATPMPError{
			Message: "test connection",
			Cause:   fmt.Errorf("timeout after %v", timeout),
		}
	}
	
	logger.Info("NAT-PMP 映射器已创建",
		"gateway", gatewayIP.String(),
		"timeout", timeout)
	
	return &NATPMPMapper{
		client:   client,
		gateway:  gatewayIP,
		mappings: make(map[int]*Mapping),
		timeout:  timeout,
	}, nil
}

// MapPort 映射端口
func (n *NATPMPMapper) MapPort(_ context.Context, proto string, internalPort int) (int, error) {
	externalPort := internalPort
	
	// NAT-PMP 使用小写协议名
	protocol := proto
	switch proto {
	case "UDP":
		protocol = "udp"
	case "TCP":
		protocol = "tcp"
	}
	
	// 添加端口映射（租期 1 小时）
	result, err := n.client.AddPortMapping(protocol, internalPort, externalPort, 3600)
	if err != nil {
		return 0, &MappingError{
			Protocol: proto,
			Port:     internalPort,
			Cause:    err,
		}
	}
	
	mappedPort := int(result.MappedExternalPort)
	
	// 记录映射
	n.mu.Lock()
	n.mappings[internalPort] = &Mapping{
		Protocol:     proto,
		InternalPort: internalPort,
		ExternalPort: mappedPort,
		Duration:     3600,
		CreatedAt:    time.Now(),
	}
	n.mu.Unlock()
	
	// 上报到 Reachability Coordinator
	n.reportMappedAddressToCoordinator(proto, mappedPort)
	
	logger.Debug("NAT-PMP 端口映射成功",
		"proto", proto,
		"internalPort", internalPort,
		"externalPort", mappedPort)
	
	return mappedPort, nil
}

// reportMappedAddressToCoordinator 将映射地址上报到 Coordinator
func (n *NATPMPMapper) reportMappedAddressToCoordinator(proto string, externalPort int) {
	n.coordinatorMu.RLock()
	coordinator := n.coordinator
	n.coordinatorMu.RUnlock()
	
	if coordinator == nil {
		return
	}
	
	// 获取外部地址
	extIP, err := n.GetExternalAddress()
	if err != nil {
		logger.Debug("NAT-PMP 获取外部地址失败，跳过上报", "err", err)
		return
	}
	
	// 构建 multiaddr
	var maddr string
	transportProto := "udp"
	if proto == "TCP" || proto == "tcp" {
		transportProto = "tcp"
	}
	
	if extIP.To4() != nil {
		maddr = fmt.Sprintf("/ip4/%s/%s/%d/quic-v1", extIP.String(), transportProto, externalPort)
	} else {
		maddr = fmt.Sprintf("/ip6/%s/%s/%d/quic-v1", extIP.String(), transportProto, externalPort)
	}
	
	coordinator.OnDirectAddressCandidate(maddr, "natpmp", pkgif.PriorityUnverified)
	logger.Debug("NAT-PMP 地址已上报到 Coordinator", "addr", maddr)
}

// SetReachabilityCoordinator 设置可达性协调器
func (n *NATPMPMapper) SetReachabilityCoordinator(coordinator pkgif.ReachabilityCoordinator) {
	n.coordinatorMu.Lock()
	n.coordinator = coordinator
	n.coordinatorMu.Unlock()
}

// UnmapPort 取消端口映射
func (n *NATPMPMapper) UnmapPort(proto string, externalPort int) error {
	protocol := proto
	switch proto {
	case "UDP":
		protocol = "udp"
	case "TCP":
		protocol = "tcp"
	}
	
	// 设置租期为 0 表示删除映射
	_, err := n.client.AddPortMapping(protocol, externalPort, 0, 0)
	if err != nil {
		return &MappingError{
			Protocol: proto,
			Port:     externalPort,
			Cause:    err,
		}
	}
	
	// 从记录中移除
	n.mu.Lock()
	delete(n.mappings, externalPort)
	n.mu.Unlock()
	
	return nil
}

// Start 启动续期循环
func (n *NATPMPMapper) Start(_ context.Context) {
	// 使用 context.Background() 保证后台循环不受上层 ctx 取消的影响
	n.ctx, n.cancel = context.WithCancel(context.Background())
	n.wg.Add(1)
	go func() {
		defer n.wg.Done()
		n.renewLoop(n.ctx)
	}()
}

// Stop 停止续期循环
func (n *NATPMPMapper) Stop() {
	if n.cancel != nil {
		n.cancel()
	}
	n.wg.Wait()
}

// renewLoop 续期循环
func (n *NATPMPMapper) renewLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n.renewMappings(ctx)
		}
	}
}

// renewMappings 续期所有映射
func (n *NATPMPMapper) renewMappings(ctx context.Context) {
	n.mu.RLock()
	mappings := make([]*Mapping, 0, len(n.mappings))
	for _, m := range n.mappings {
		mappings = append(mappings, m)
	}
	n.mu.RUnlock()
	
	for _, m := range mappings {
		// 在租期 2/3 时续期
		elapsed := time.Since(m.CreatedAt)
		threshold := time.Duration(m.Duration) * 2 / 3 * time.Second
		
		if elapsed > threshold {
			// 重新映射
			_, _ = n.MapPort(ctx, m.Protocol, m.InternalPort)
		}
	}
}

// GetExternalAddress 获取外部地址
func (n *NATPMPMapper) GetExternalAddress() (net.IP, error) {
	result, err := n.client.GetExternalAddress()
	if err != nil {
		return nil, &NATPMPError{
			Message: "get external address",
			Cause:   err,
		}
	}
	
	return result.ExternalIPAddress[:], nil
}

// Errors
var (
	ErrNoGateway = &NATPMPError{Message: "no gateway found"}
)

// NATPMPError NAT-PMP 错误
type NATPMPError struct {
	Message string
	Cause   error
}

func (e *NATPMPError) Error() string {
	if e.Cause != nil {
		return "natpmp: " + e.Message + ": " + e.Cause.Error()
	}
	return "natpmp: " + e.Message
}

func (e *NATPMPError) Unwrap() error {
	return e.Cause
}

// MappingError 端口映射错误
type MappingError struct {
	Protocol string
	Port     int
	Cause    error
}

func (e *MappingError) Error() string {
	return fmt.Sprintf("natpmp: mapping %s port %d failed: %v", e.Protocol, e.Port, e.Cause)
}

func (e *MappingError) Unwrap() error {
	return e.Cause
}
