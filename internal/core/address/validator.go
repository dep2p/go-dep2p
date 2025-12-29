// Package address 提供地址管理模块的实现
package address

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
)

// ============================================================================
//                              错误定义
// ============================================================================

var (
	// ErrAddressUnreachable 地址不可达
	ErrAddressUnreachable = errors.New("address unreachable")

	// ErrInvalidFormat 地址格式无效
	ErrInvalidFormat = errors.New("invalid address format")

	// ErrInvalidPort 端口无效
	ErrInvalidPort = errors.New("invalid port")

	// ErrInvalidIP IP 地址无效
	ErrInvalidIP = errors.New("invalid IP address")
)

// ============================================================================
//                              Validator 实现
// ============================================================================

// Validator 地址验证器
//
// 提供地址格式验证和可达性检测。
type Validator struct {
	// 配置
	config ValidatorConfig

	// 可达性缓存
	reachabilityCache map[string]reachabilityEntry
	cacheMu           sync.RWMutex
}

// ValidatorConfig 验证器配置
type ValidatorConfig struct {
	// DialTimeout 连接超时
	DialTimeout time.Duration

	// ReachabilityTimeout 可达性检测超时
	ReachabilityTimeout time.Duration

	// CacheTTL 可达性缓存 TTL
	CacheTTL time.Duration

	// AllowPrivate 是否允许私有地址
	AllowPrivate bool

	// AllowLoopback 是否允许回环地址
	AllowLoopback bool

	// AllowReserved 是否允许保留地址
	AllowReserved bool
}

// reachabilityEntry 可达性缓存条目
type reachabilityEntry struct {
	reachable bool
	checkedAt time.Time
	rtt       time.Duration
}

// DefaultValidatorConfig 返回默认配置
func DefaultValidatorConfig() ValidatorConfig {
	return ValidatorConfig{
		DialTimeout:         5 * time.Second,
		ReachabilityTimeout: 10 * time.Second,
		CacheTTL:            5 * time.Minute,
		AllowPrivate:        true,
		AllowLoopback:       false,
		AllowReserved:       false,
	}
}

// NewValidator 创建验证器
func NewValidator(config ValidatorConfig) *Validator {
	return &Validator{
		config:            config,
		reachabilityCache: make(map[string]reachabilityEntry),
	}
}

// ============================================================================
//                              格式验证
// ============================================================================

// Validate 验证地址
//
// 检查地址格式是否有效。
func (v *Validator) Validate(addr endpoint.Address) error {
	if addr == nil {
		return ErrInvalidAddress
	}

	addrStr := addr.String()
	if addrStr == "" {
		return ErrInvalidAddress
	}

	// Multiaddr 格式
	if strings.HasPrefix(addrStr, "/") {
		return v.validateMultiaddr(addrStr)
	}

	// Host:Port 格式
	return v.validateHostPort(addrStr)
}

// validateMultiaddr 验证 Multiaddr 格式
func (v *Validator) validateMultiaddr(s string) error {
	parts := strings.Split(s, "/")
	if len(parts) < 3 || parts[0] != "" {
		return fmt.Errorf("%w: malformed multiaddr", ErrInvalidFormat)
	}

	hasIP := false
	hasPort := false

	for i := 1; i < len(parts); i += 2 {
		if i+1 > len(parts) {
			break
		}

		protocol := parts[i]
		value := ""
		if i+1 < len(parts) {
			value = parts[i+1]
		}

		switch protocol {
		case "ip4":
			ip := net.ParseIP(value)
			if ip == nil || ip.To4() == nil {
				return fmt.Errorf("%w: %s", ErrInvalidIP, value)
			}
			if err := v.validateIP(ip); err != nil {
				return err
			}
			hasIP = true

		case "ip6":
			ip := net.ParseIP(value)
			if ip == nil {
				return fmt.Errorf("%w: %s", ErrInvalidIP, value)
			}
			if err := v.validateIP(ip); err != nil {
				return err
			}
			hasIP = true

		case "tcp", "udp":
			if err := v.validatePort(value); err != nil {
				return err
			}
			hasPort = true

		case "quic", "quic-v1":
			i-- // 无值

		case "dns", "dns4", "dns6":
			if value == "" {
				return fmt.Errorf("%w: empty hostname", ErrInvalidFormat)
			}
			hasIP = true

		case "p2p", "p2p-circuit":
			// 跳过验证

		default:
			// 未知协议，跳过
		}
	}

	// 基本地址至少需要 IP/DNS
	if !hasIP && !strings.Contains(s, "p2p-circuit") {
		return fmt.Errorf("%w: no IP or hostname", ErrInvalidFormat)
	}

	_ = hasPort // 端口不是必须的

	return nil
}

// validateHostPort 验证 Host:Port 格式
func (v *Validator) validateHostPort(s string) error {
	host, portStr, err := net.SplitHostPort(s)
	if err != nil {
		// 可能只有主机名
		host = s
		portStr = ""
	}

	if host == "" {
		return fmt.Errorf("%w: empty host", ErrInvalidFormat)
	}

	// 验证 IP
	ip := net.ParseIP(host)
	if ip != nil {
		if err := v.validateIP(ip); err != nil {
			return err
		}
	}

	// 验证端口
	if portStr != "" {
		if err := v.validatePort(portStr); err != nil {
			return err
		}
	}

	return nil
}

// validateIP 验证 IP 地址
func (v *Validator) validateIP(ip net.IP) error {
	// 检查回环地址
	if !v.config.AllowLoopback && ip.IsLoopback() {
		return fmt.Errorf("%w: loopback address not allowed", ErrInvalidIP)
	}

	// 检查私有地址
	if !v.config.AllowPrivate && ip.IsPrivate() {
		return fmt.Errorf("%w: private address not allowed", ErrInvalidIP)
	}

	// 检查保留地址
	if !v.config.AllowReserved {
		if ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() {
			return fmt.Errorf("%w: reserved address not allowed", ErrInvalidIP)
		}
	}

	return nil
}

// validatePort 验证端口
func (v *Validator) validatePort(portStr string) error {
	if portStr == "" {
		return nil
	}

	var port int
	_, err := fmt.Sscanf(portStr, "%d", &port)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrInvalidPort, portStr)
	}

	if port < 0 || port > 65535 {
		return fmt.Errorf("%w: port out of range %d", ErrInvalidPort, port)
	}

	// 检查是否是保留端口
	if port == 0 {
		return fmt.Errorf("%w: port 0 not allowed", ErrInvalidPort)
	}

	return nil
}

// ============================================================================
//                              可达性检测
// ============================================================================

// IsReachable 检查地址是否可达
func (v *Validator) IsReachable(addr endpoint.Address) bool {
	ctx, cancel := context.WithTimeout(context.Background(), v.config.ReachabilityTimeout)
	defer cancel()

	return v.IsReachableCtx(ctx, addr)
}

// IsReachableCtx 带上下文检查可达性
func (v *Validator) IsReachableCtx(ctx context.Context, addr endpoint.Address) bool {
	if addr == nil {
		return false
	}

	addrStr := addr.String()

	// 检查缓存
	if entry, ok := v.getCachedReachability(addrStr); ok {
		return entry.reachable
	}

	// 执行可达性检测
	reachable, rtt := v.checkReachability(ctx, addr)

	// 更新缓存
	v.setCachedReachability(addrStr, reachable, rtt)

	return reachable
}

// checkReachability 执行可达性检测
func (v *Validator) checkReachability(ctx context.Context, addr endpoint.Address) (bool, time.Duration) {
	// 解析地址
	parser := NewParser()
	parsed, err := parser.Parse(addr.String())
	if err != nil {
		return false, 0
	}

	pa, ok := parsed.(*ParsedAddress)
	if !ok {
		return false, 0
	}

	// 获取目标地址
	var target string
	if pa.IP() != nil {
		if pa.Port() > 0 {
			target = net.JoinHostPort(pa.IP().String(), fmt.Sprintf("%d", pa.Port()))
		} else {
			return false, 0 // 没有端口无法检测
		}
	} else if pa.Host() != "" {
		if pa.Port() > 0 {
			target = net.JoinHostPort(pa.Host(), fmt.Sprintf("%d", pa.Port()))
		} else {
			return false, 0
		}
	} else {
		return false, 0
	}

	// 确定传输协议
	network := "tcp"
	if pa.Transport() == "udp" || pa.Transport() == "quic" {
		network = "udp"
	}

	// 执行连接测试
	start := time.Now()
	dialer := &net.Dialer{
		Timeout: v.config.DialTimeout,
	}

	conn, err := dialer.DialContext(ctx, network, target)
	if err != nil {
		log.Debug("可达性检测失败",
			"addr", addr.String(),
			"err", err)
		return false, 0
	}
	defer func() { _ = conn.Close() }()

	rtt := time.Since(start)
	log.Debug("可达性检测成功",
		"addr", addr.String(),
		"rtt", rtt)

	return true, rtt
}

// getCachedReachability 获取缓存的可达性
func (v *Validator) getCachedReachability(addrStr string) (reachabilityEntry, bool) {
	v.cacheMu.RLock()
	defer v.cacheMu.RUnlock()

	entry, ok := v.reachabilityCache[addrStr]
	if !ok {
		return reachabilityEntry{}, false
	}

	// 检查是否过期
	if time.Since(entry.checkedAt) > v.config.CacheTTL {
		return reachabilityEntry{}, false
	}

	return entry, true
}

// setCachedReachability 设置缓存的可达性
func (v *Validator) setCachedReachability(addrStr string, reachable bool, rtt time.Duration) {
	v.cacheMu.Lock()
	defer v.cacheMu.Unlock()

	v.reachabilityCache[addrStr] = reachabilityEntry{
		reachable: reachable,
		checkedAt: time.Now(),
		rtt:       rtt,
	}
}

// ClearCache 清空可达性缓存
func (v *Validator) ClearCache() {
	v.cacheMu.Lock()
	defer v.cacheMu.Unlock()
	v.reachabilityCache = make(map[string]reachabilityEntry)
}

// ============================================================================
//                              辅助方法
// ============================================================================

// IsValid 检查地址格式是否有效
func (v *Validator) IsValid(addr endpoint.Address) bool {
	return v.Validate(addr) == nil
}

// ValidateAndCheck 验证并检查可达性
func (v *Validator) ValidateAndCheck(ctx context.Context, addr endpoint.Address) error {
	// 先验证格式
	if err := v.Validate(addr); err != nil {
		return err
	}

	// 再检查可达性
	if !v.IsReachableCtx(ctx, addr) {
		return ErrAddressUnreachable
	}

	return nil
}

// GetRTT 获取地址的 RTT（从缓存）
func (v *Validator) GetRTT(addr endpoint.Address) (time.Duration, bool) {
	v.cacheMu.RLock()
	defer v.cacheMu.RUnlock()

	entry, ok := v.reachabilityCache[addr.String()]
	if !ok || !entry.reachable {
		return 0, false
	}

	return entry.rtt, true
}

