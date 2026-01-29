// Package gater 实现连接门控
package gater

import (
	"net"
	"sync"
)

// Filter 地址过滤器
type Filter struct {
	mu sync.RWMutex

	// allowedCIDRs 允许的 CIDR 列表
	allowedCIDRs []*net.IPNet

	// blockedCIDRs 阻止的 CIDR 列表
	blockedCIDRs []*net.IPNet

	// defaultAllow 默认是否允许
	defaultAllow bool
}

// NewFilter 创建过滤器
func NewFilter(defaultAllow bool) *Filter {
	return &Filter{
		defaultAllow: defaultAllow,
	}
}

// AllowCIDR 允许 CIDR
func (f *Filter) AllowCIDR(cidr string) error {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.allowedCIDRs = append(f.allowedCIDRs, ipnet)
	return nil
}

// BlockCIDR 阻止 CIDR
func (f *Filter) BlockCIDR(cidr string) error {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return err
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.blockedCIDRs = append(f.blockedCIDRs, ipnet)
	return nil
}

// AllowIP 检查 IP 是否允许
func (f *Filter) AllowIP(ip net.IP) bool {
	f.mu.RLock()
	defer f.mu.RUnlock()

	// 检查阻止列表
	for _, ipnet := range f.blockedCIDRs {
		if ipnet.Contains(ip) {
			return false
		}
	}

	// 检查允许列表
	if len(f.allowedCIDRs) > 0 {
		for _, ipnet := range f.allowedCIDRs {
			if ipnet.Contains(ip) {
				return true
			}
		}
		return false
	}

	return f.defaultAllow
}

// AllowAddr 检查地址是否允许
func (f *Filter) AllowAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}

	ip := net.ParseIP(host)
	if ip == nil {
		// 无法解析为 IP，使用默认值
		return f.defaultAllow
	}

	return f.AllowIP(ip)
}

// Reset 重置过滤器
func (f *Filter) Reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.allowedCIDRs = nil
	f.blockedCIDRs = nil
}
