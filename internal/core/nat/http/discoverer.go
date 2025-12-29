// Package http 提供基于 HTTP 的 IP 发现实现
//
// 通过公共 HTTP 服务获取外部 IP 地址，作为 STUN 的备用方案
package http

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	natif "github.com/dep2p/go-dep2p/pkg/interfaces/nat"
)

// 包级别日志实例
var log = logger.Logger("nat.http")

// 错误定义
var (
	ErrNoService     = errors.New("no HTTP IP service available")
	ErrInvalidIP     = errors.New("invalid IP address returned")
	ErrAllServicesFailed = errors.New("all HTTP IP services failed")
)

// DefaultServices 默认的 HTTP IP 发现服务
var DefaultServices = []string{
	"https://api.ipify.org",
	"https://ifconfig.me/ip",
	"https://icanhazip.com",
	"https://api.ip.sb/ip",
	"https://checkip.amazonaws.com",
}

// Discoverer HTTP IP 发现器实现
type Discoverer struct {
	services []string
	client   *http.Client

	// 缓存
	cachedIP      string
	cachedTime    time.Time
	cacheDuration time.Duration
	cacheMu       sync.RWMutex
}

// 确保实现接口
var _ natif.IPDiscoverer = (*Discoverer)(nil)

// NewDiscoverer 创建 HTTP IP 发现器
func NewDiscoverer(services []string) *Discoverer {
	if len(services) == 0 {
		services = DefaultServices
	}

	return &Discoverer{
		services: services,
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:       10,
				IdleConnTimeout:    30 * time.Second,
				DisableCompression: true,
			},
		},
		cacheDuration: 5 * time.Minute,
	}
}

// Name 返回发现器名称
func (d *Discoverer) Name() string {
	return "http"
}

// Discover 发现外部 IP
func (d *Discoverer) Discover(ctx context.Context) (endpoint.Address, error) {
	// 检查缓存（getCacheDuration 内部会获取锁，所以这里需要先获取 cacheDuration）
	cacheDuration := d.getCacheDuration()
	d.cacheMu.RLock()
	if d.cachedIP != "" && time.Since(d.cachedTime) < cacheDuration {
		ip := d.cachedIP
		d.cacheMu.RUnlock()
		return newIPAddr(net.ParseIP(ip), 0), nil
	}
	d.cacheMu.RUnlock()

	// 尝试每个服务
	var lastErr error
	for _, service := range d.services {
		ip, err := d.queryService(ctx, service)
		if err != nil {
			log.Debug("HTTP IP 服务查询失败",
				"service", service,
				"err", err)
			lastErr = err
			continue
		}

		// 验证 IP 地址
		parsedIP := net.ParseIP(ip)
		if parsedIP == nil {
			log.Debug("无效的 IP 地址",
				"service", service,
				"ip", ip)
			lastErr = ErrInvalidIP
			continue
		}

		// 更新缓存
		d.cacheMu.Lock()
		d.cachedIP = ip
		d.cachedTime = time.Now()
		d.cacheMu.Unlock()

		log.Info("通过 HTTP 服务获取到外部 IP",
			"service", service,
			"ip", ip)

		return newIPAddr(parsedIP, 0), nil
	}

	return nil, fmt.Errorf("%w: %v", ErrAllServicesFailed, lastErr)
}

// Priority 返回优先级
func (d *Discoverer) Priority() int {
	return 100 // 比 STUN (优先级 10) 低
}

// queryService 查询单个 HTTP 服务
func (d *Discoverer) queryService(ctx context.Context, service string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, service, nil)
	if err != nil {
		return "", fmt.Errorf("创建请求失败: %w", err)
	}

	req.Header.Set("User-Agent", "dep2p/1.0")

	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("HTTP 请求失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP 状态码: %d", resp.StatusCode)
	}

	// 限制读取大小
	body, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	ip := strings.TrimSpace(string(body))
	if ip == "" {
		return "", errors.New("empty response")
	}

	return ip, nil
}

// SetCacheDuration 设置缓存时间
func (d *Discoverer) SetCacheDuration(duration time.Duration) {
	d.cacheMu.Lock()
	d.cacheDuration = duration
	d.cacheMu.Unlock()
}

// getCacheDuration 获取缓存时间（线程安全）
func (d *Discoverer) getCacheDuration() time.Duration {
	d.cacheMu.RLock()
	defer d.cacheMu.RUnlock()
	return d.cacheDuration
}

// ClearCache 清除缓存
func (d *Discoverer) ClearCache() {
	d.cacheMu.Lock()
	d.cachedIP = ""
	d.cacheMu.Unlock()
}

// Close 关闭发现器
func (d *Discoverer) Close() error {
	d.client.CloseIdleConnections()
	return nil
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
	if a.port == 0 {
		return a.ip.String()
	}
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

