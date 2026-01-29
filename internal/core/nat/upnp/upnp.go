package upnp

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/huin/goupnp/dcps/internetgateway1"
	"github.com/huin/goupnp/dcps/internetgateway2"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("nat/upnp")

// UPnPMapper UPnP 端口映射器
type UPnPMapper struct {
	client   IGDClient
	mappings map[int]*Mapping
	mu       sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// 地址发现集成
	coordinator   pkgif.ReachabilityCoordinator // 可达性协调器（用于上报候选地址）
	coordinatorMu sync.RWMutex
}

// IGDClient UPnP IGD 客户端接口
type IGDClient interface {
	AddPortMapping(
		NewRemoteHost string,
		NewExternalPort uint16,
		NewProtocol string,
		NewInternalPort uint16,
		NewInternalClient string,
		NewEnabled bool,
		NewPortMappingDescription string,
		NewLeaseDuration uint32,
	) error

	DeletePortMapping(
		NewRemoteHost string,
		NewExternalPort uint16,
		NewProtocol string,
	) error

	// GetExternalIPAddress 获取路由器的外部 IP 地址
	// 所有 goupnp 的 WANIPConnection1 和 WANPPPConnection1 都实现了此方法
	GetExternalIPAddress() (string, error)
}

// Mapping 端口映射记录
type Mapping struct {
	Protocol     string
	InternalPort int
	ExternalPort int
	Duration     uint32
	CreatedAt    time.Time
}

// DefaultUPnPTimeout 默认 UPnP 发现超时
// NAT-001 优化：缩短默认超时时间，加快启动速度
const DefaultUPnPTimeout = 2 * time.Second

// NewUPnPMapper 创建 UPnP 映射器（使用默认超时）
func NewUPnPMapper() (*UPnPMapper, error) {
	return NewUPnPMapperWithTimeout(DefaultUPnPTimeout)
}

// NewUPnPMapperWithTimeout 创建带超时的 UPnP 映射器
//
// NAT-001 优化：添加超时控制，避免 UPnP 发现阻塞启动流程
// goupnp 库的默认发现可能需要 8+ 秒，通过超时可以快速失败
func NewUPnPMapperWithTimeout(timeout time.Duration) (*UPnPMapper, error) {
	if timeout <= 0 {
		timeout = DefaultUPnPTimeout
	}

	type result struct {
		mapper *UPnPMapper
		err    error
	}

	resultCh := make(chan result, 1)

	go func() {
		mapper, err := discoverUPnPDevice()
		resultCh <- result{mapper: mapper, err: err}
	}()

	select {
	case res := <-resultCh:
		return res.mapper, res.err
	case <-time.After(timeout):
		return nil, fmt.Errorf("upnp: discovery timeout after %v", timeout)
	}
}

// discoverUPnPDevice 发现 UPnP 设备（内部实现）
func discoverUPnPDevice() (*UPnPMapper, error) {
	// 1. 尝试 IGDv2 (WANIPConnection1)
	clients2, _, err := internetgateway2.NewWANIPConnection1Clients()
	if err == nil && len(clients2) > 0 {
		return &UPnPMapper{
			client:   clients2[0],
			mappings: make(map[int]*Mapping),
		}, nil
	}

	// 2. 尝试 IGDv2 (WANPPPConnection1)
	pppClients2, _, err := internetgateway2.NewWANPPPConnection1Clients()
	if err == nil && len(pppClients2) > 0 {
		return &UPnPMapper{
			client:   pppClients2[0],
			mappings: make(map[int]*Mapping),
		}, nil
	}

	// 3. 回退到 IGDv1 (WANIPConnection1)
	clients1, _, err := internetgateway1.NewWANIPConnection1Clients()
	if err == nil && len(clients1) > 0 {
		return &UPnPMapper{
			client:   clients1[0],
			mappings: make(map[int]*Mapping),
		}, nil
	}

	// 4. 回退到 IGDv1 (WANPPPConnection1)
	pppClients1, _, err := internetgateway1.NewWANPPPConnection1Clients()
	if err == nil && len(pppClients1) > 0 {
		return &UPnPMapper{
			client:   pppClients1[0],
			mappings: make(map[int]*Mapping),
		}, nil
	}

	return nil, ErrNoUPnPDevice
}

// MapPort 映射端口
func (u *UPnPMapper) MapPort(_ context.Context, proto string, internalPort int) (int, error) {
	// 获取本地 IP
	localIP, err := getLocalIP()
	if err != nil {
		return 0, &MappingError{
			Protocol: proto,
			Port:     internalPort,
			Cause:    fmt.Errorf("get local IP: %w", err),
		}
	}

	externalPort := internalPort // 尝试映射到相同端口

	// 调用 UPnP AddPortMapping
	err = u.client.AddPortMapping(
		"",                   // NewRemoteHost (空=任意)
		uint16(externalPort), // NewExternalPort
		proto,                // "UDP" 或 "TCP"
		uint16(internalPort), // NewInternalPort
		localIP.String(),     // NewInternalClient
		true,                 // NewEnabled
		"dep2p",              // NewPortMappingDescription
		3600,                 // NewLeaseDuration (1小时)
	)

	if err != nil {
		return 0, &MappingError{
			Protocol: proto,
			Port:     internalPort,
			Cause:    err,
		}
	}

	// 记录映射
	u.mu.Lock()
	u.mappings[internalPort] = &Mapping{
		Protocol:     proto,
		InternalPort: internalPort,
		ExternalPort: externalPort,
		Duration:     3600,
		CreatedAt:    time.Now(),
	}
	u.mu.Unlock()

	// 获取外部 IP 并上报到 Reachability Coordinator
	// 注意：从 UPnP 网关动态获取外部 IP，而不是依赖外部设置
	extIP, err := u.client.GetExternalIPAddress()
	if err != nil {
		logger.Warn("UPnP 获取外部 IP 失败", "err", err)
	} else if extIP != "" {
		u.reportMappedAddressToCoordinator(proto, externalPort, extIP)
	}

	logger.Debug("UPnP 端口映射成功",
		"proto", proto,
		"internalPort", internalPort,
		"externalPort", externalPort,
		"externalIP", extIP)

	return externalPort, nil
}

// reportMappedAddressToCoordinator 将映射地址上报到 Coordinator
//
// extIP: 从 UPnP 网关动态获取的外部 IP 地址
func (u *UPnPMapper) reportMappedAddressToCoordinator(proto string, externalPort int, extIP string) {
	u.coordinatorMu.RLock()
	coordinator := u.coordinator
	u.coordinatorMu.RUnlock()

	if coordinator == nil {
		logger.Debug("UPnP Coordinator 未设置，跳过上报")
		return
	}

	if extIP == "" {
		logger.Debug("UPnP 外部 IP 为空，跳过上报")
		return
	}

	// 构建 multiaddr
	var maddr string
	transportProto := "udp"
	if proto == "TCP" || proto == "tcp" {
		transportProto = "tcp"
	}

	ip := net.ParseIP(extIP)
	if ip == nil {
		logger.Debug("UPnP 外部 IP 格式无效", "ip", extIP)
		return
	}

	if ip.To4() != nil {
		maddr = fmt.Sprintf("/ip4/%s/%s/%d/quic-v1", extIP, transportProto, externalPort)
	} else {
		maddr = fmt.Sprintf("/ip6/%s/%s/%d/quic-v1", extIP, transportProto, externalPort)
	}

	coordinator.OnDirectAddressCandidate(maddr, "upnp", pkgif.PriorityUnverified)
	logger.Info("UPnP 地址已上报到 Coordinator", "addr", maddr)
}

// SetReachabilityCoordinator 设置可达性协调器
func (u *UPnPMapper) SetReachabilityCoordinator(coordinator pkgif.ReachabilityCoordinator) {
	u.coordinatorMu.Lock()
	u.coordinator = coordinator
	u.coordinatorMu.Unlock()
}

// UnmapPort 取消端口映射
func (u *UPnPMapper) UnmapPort(proto string, externalPort int) error {
	err := u.client.DeletePortMapping("", uint16(externalPort), proto)
	if err != nil {
		return &MappingError{
			Protocol: proto,
			Port:     externalPort,
			Cause:    err,
		}
	}

	// 从记录中移除
	u.mu.Lock()
	delete(u.mappings, externalPort)
	u.mu.Unlock()

	return nil
}

// Start 启动续期循环
func (u *UPnPMapper) Start(_ context.Context) {
	// 使用 context.Background() 保证后台循环不受上层 ctx 取消的影响
	u.ctx, u.cancel = context.WithCancel(context.Background())
	u.wg.Add(1)
	go func() {
		defer u.wg.Done()
		u.renewLoop(u.ctx)
	}()
}

// Stop 停止续期循环
func (u *UPnPMapper) Stop() {
	if u.cancel != nil {
		u.cancel()
	}
	u.wg.Wait()
}

// renewLoop 续期循环
func (u *UPnPMapper) renewLoop(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			u.renewMappings(ctx)
		}
	}
}

// renewMappings 续期所有映射
func (u *UPnPMapper) renewMappings(ctx context.Context) {
	u.mu.RLock()
	mappings := make([]*Mapping, 0, len(u.mappings))
	for _, m := range u.mappings {
		mappings = append(mappings, m)
	}
	u.mu.RUnlock()

	for _, m := range mappings {
		// 在租期 2/3 时续期
		elapsed := time.Since(m.CreatedAt)
		threshold := time.Duration(m.Duration) * 2 / 3 * time.Second

		if elapsed > threshold {
			// 重新映射（会更新 CreatedAt）
			_, _ = u.MapPort(ctx, m.Protocol, m.InternalPort)
		}
	}
}

// getLocalIP 获取本地 IP 地址
func getLocalIP() (net.IP, error) {
	// 连接到外部地址以获取本地 IP（不会实际发送数据）
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP, nil
}

// Errors
var (
	ErrNoUPnPDevice = &UPnPError{Message: "no UPnP device found"}
)

// UPnPError UPnP 错误
type UPnPError struct {
	Message string
	Cause   error
}

func (e *UPnPError) Error() string {
	if e.Cause != nil {
		return "upnp: " + e.Message + ": " + e.Cause.Error()
	}
	return "upnp: " + e.Message
}

func (e *UPnPError) Unwrap() error {
	return e.Cause
}

// MappingError 端口映射错误
type MappingError struct {
	Protocol string
	Port     int
	Cause    error
}

func (e *MappingError) Error() string {
	return fmt.Sprintf("upnp: mapping %s port %d failed: %v", e.Protocol, e.Port, e.Cause)
}

func (e *MappingError) Unwrap() error {
	return e.Cause
}
