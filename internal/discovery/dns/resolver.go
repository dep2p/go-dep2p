package dns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              常量定义
// ============================================================================

const (
	// DNSAddrPrefix dnsaddr 前缀
	DNSAddrPrefix = "dnsaddr="

	// DNSAddrDomainPrefix DNS 地址域名前缀
	DNSAddrDomainPrefix = "_dnsaddr."
)

// ============================================================================
//                              解析器配置
// ============================================================================

// ResolverConfig 解析器配置
type ResolverConfig struct {
	// Timeout DNS 查询超时
	Timeout time.Duration

	// MaxDepth 最大递归深度
	MaxDepth int

	// CustomResolver 自定义 DNS 解析器地址（格式: "ip:port"）
	CustomResolver string

	// CacheTTL 缓存 TTL
	CacheTTL time.Duration
}

// DefaultResolverConfig 默认配置
func DefaultResolverConfig() ResolverConfig {
	return ResolverConfig{
		Timeout:        10 * time.Second,
		MaxDepth:       3,
		CustomResolver: "",
		CacheTTL:       5 * time.Minute,
	}
}

// ============================================================================
//                              缓存条目
// ============================================================================

// cacheEntry 缓存条目
type cacheEntry struct {
	peers     []types.PeerInfo
	expiresAt time.Time
}

// ============================================================================
//                              Resolver 实现
// ============================================================================

// Resolver DNS TXT 记录解析器
type Resolver struct {
	resolver *net.Resolver
	config   ResolverConfig

	// 缓存
	cacheMu sync.RWMutex
	cache   map[string]cacheEntry
}

// NewResolver 创建 DNS 解析器
func NewResolver(config ResolverConfig) *Resolver {
	r := &Resolver{
		config: config,
		cache:  make(map[string]cacheEntry),
	}

	// 配置 DNS 解析器
	if config.CustomResolver != "" {
		r.resolver = &net.Resolver{
			PreferGo: true,
			Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
				d := net.Dialer{
					Timeout: config.Timeout,
				}
				return d.DialContext(ctx, network, config.CustomResolver)
			},
		}
	} else {
		r.resolver = net.DefaultResolver
	}

	return r
}

// ============================================================================
//                              解析方法
// ============================================================================

// Resolve 解析 DNS 域名获取节点信息
func (r *Resolver) Resolve(ctx context.Context, domain string) ([]types.PeerInfo, error) {
	return r.ResolveWithDepth(ctx, domain, r.config.MaxDepth)
}

// ResolveWithDepth 递归解析 DNS 域名
func (r *Resolver) ResolveWithDepth(ctx context.Context, domain string, maxDepth int) ([]types.PeerInfo, error) {
	if maxDepth < 0 {
		return nil, ErrMaxDepthExceeded
	}

	// 规范化域名
	domain = r.normalizeDomain(domain)

	// 检查缓存
	if peers, ok := r.getFromCache(domain); ok {
		return peers, nil
	}

	// 查询 TXT 记录
	records, err := r.resolveTXT(ctx, domain)
	if err != nil {
		return nil, fmt.Errorf("resolve TXT records for %s: %w", domain, err)
	}

	if len(records) == 0 {
		return nil, ErrNoRecordsFound
	}

	// 解析记录
	var peers []types.PeerInfo
	seen := make(map[string]bool)

	for _, record := range records {
		peer, nestedDomain, err := ParseDNSAddr(record)
		if err != nil {
			continue
		}

		if nestedDomain != "" {
			// 递归解析嵌套域名
			if maxDepth > 0 {
				nestedPeers, err := r.ResolveWithDepth(ctx, nestedDomain, maxDepth-1)
				if err != nil {
					continue
				}
				for _, p := range nestedPeers {
					key := string(p.ID)
					if !seen[key] {
						seen[key] = true
						peers = append(peers, p)
					}
				}
			}
		} else if peer != nil {
			key := string(peer.ID)
			if !seen[key] {
				seen[key] = true
				peers = append(peers, *peer)
			}
		}
	}

	// 缓存结果
	r.setCache(domain, peers)

	return peers, nil
}

// resolveTXT 查询 DNS TXT 记录
func (r *Resolver) resolveTXT(ctx context.Context, domain string) ([]string, error) {
	// 设置超时
	ctx, cancel := context.WithTimeout(ctx, r.config.Timeout)
	defer cancel()

	records, err := r.resolver.LookupTXT(ctx, domain)
	if err != nil {
		// 检查是否为 "no such host" 错误
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			return nil, ErrNoRecordsFound
		}
		return nil, err
	}

	return records, nil
}

// ============================================================================
//                              域名处理
// ============================================================================

// normalizeDomain 规范化域名
func (r *Resolver) normalizeDomain(domain string) string {
	// 移除可能的尾随点
	domain = strings.TrimSuffix(domain, ".")

	// 如果没有 _dnsaddr. 前缀，添加它
	if !strings.HasPrefix(domain, DNSAddrDomainPrefix) {
		domain = DNSAddrDomainPrefix + domain
	}

	return domain
}

// ============================================================================
//                              缓存管理
// ============================================================================

// getFromCache 从缓存获取结果
func (r *Resolver) getFromCache(domain string) ([]types.PeerInfo, bool) {
	r.cacheMu.RLock()
	defer r.cacheMu.RUnlock()

	entry, ok := r.cache[domain]
	if !ok {
		return nil, false
	}

	if time.Now().After(entry.expiresAt) {
		return nil, false
	}

	// 返回副本
	peers := make([]types.PeerInfo, len(entry.peers))
	copy(peers, entry.peers)
	return peers, true
}

// setCache 设置缓存
func (r *Resolver) setCache(domain string, peers []types.PeerInfo) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	// 存储副本
	peersCopy := make([]types.PeerInfo, len(peers))
	copy(peersCopy, peers)

	r.cache[domain] = cacheEntry{
		peers:     peersCopy,
		expiresAt: time.Now().Add(r.config.CacheTTL),
	}
}

// ClearCache 清除缓存
func (r *Resolver) ClearCache() {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()
	r.cache = make(map[string]cacheEntry)
}

// ClearExpiredCache 清除过期缓存
func (r *Resolver) ClearExpiredCache() {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	now := time.Now()
	for domain, entry := range r.cache {
		if now.After(entry.expiresAt) {
			delete(r.cache, domain)
		}
	}
}

// ============================================================================
//                              dnsaddr 解析
// ============================================================================

// ParseDNSAddr 解析 dnsaddr 格式
//
// 支持的格式：
// - dnsaddr=/ip4/<ip>/tcp/<port>/p2p/<peerID>
// - dnsaddr=/ip6/<ip>/tcp/<port>/p2p/<peerID>
// - dnsaddr=/dnsaddr/<domain> (嵌套域名)
//
// 返回值：
// - peer: 解析出的节点信息（如果是直接地址）
// - nestedDomain: 嵌套的域名（如果是 dnsaddr 嵌套）
// - error: 错误
func ParseDNSAddr(record string) (*types.PeerInfo, string, error) {
	// 检查 dnsaddr= 前缀
	if !strings.HasPrefix(record, DNSAddrPrefix) {
		return nil, "", ErrInvalidDNSAddr
	}

	// 提取 multiaddr 部分
	addrStr := strings.TrimPrefix(record, DNSAddrPrefix)
	if addrStr == "" {
		return nil, "", ErrInvalidDNSAddr
	}

	// 检查是否为嵌套 dnsaddr
	if strings.HasPrefix(addrStr, "/dnsaddr/") {
		// 提取嵌套域名
		parts := strings.SplitN(addrStr, "/", 4)
		if len(parts) < 3 {
			return nil, "", ErrInvalidDNSAddr
		}
		nestedDomain := parts[2]
		
		// FIX #B39: 验证嵌套域名不为空
		// "dnsaddr=/dnsaddr/" 会导致 nestedDomain = ""，应该拒绝
		if nestedDomain == "" {
			return nil, "", fmt.Errorf("%w: empty nested domain", ErrInvalidDNSAddr)
		}
		
		return nil, nestedDomain, nil
	}

	// 解析为 multiaddr
	return parseMultiaddr(addrStr)
}

// parseMultiaddr 解析 multiaddr 字符串
func parseMultiaddr(addrStr string) (*types.PeerInfo, string, error) {
	// 分割 multiaddr 组件
	parts := strings.Split(addrStr, "/")
	if len(parts) < 2 {
		return nil, "", ErrInvalidMultiaddr
	}

	// 查找 /p2p/ 部分
	var peerIDStr string
	var addrParts []string

	for i := 0; i < len(parts); i++ {
		if parts[i] == "p2p" && i+1 < len(parts) {
			peerIDStr = parts[i+1]
			addrParts = parts[:i]
			break
		}
	}

	if peerIDStr == "" {
		return nil, "", fmt.Errorf("%w: missing /p2p/<peerID>", ErrInvalidMultiaddr)
	}

	// 构建地址字符串并转换为 Multiaddr
	addrString := strings.Join(addrParts, "/")
	ma, err := types.NewMultiaddr(addrString)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrInvalidMultiaddr, err)
	}

	peer := &types.PeerInfo{
		ID:     types.PeerID(peerIDStr),
		Addrs:  []types.Multiaddr{ma},
		Source: types.SourceDNS,
	}

	return peer, "", nil
}

// ============================================================================
//                              域名验证
// ============================================================================

// ValidateDomain 验证域名格式
func ValidateDomain(domain string) error {
	if domain == "" {
		return ErrInvalidDomain
	}

	// 移除可能的 _dnsaddr. 前缀
	domain = strings.TrimPrefix(domain, DNSAddrDomainPrefix)

	// 基本域名格式检查
	if len(domain) > 253 {
		return fmt.Errorf("%w: domain too long", ErrInvalidDomain)
	}

	labels := strings.Split(domain, ".")
	for _, label := range labels {
		if len(label) == 0 {
			return fmt.Errorf("%w: empty label", ErrInvalidDomain)
		}
		if len(label) > 63 {
			return fmt.Errorf("%w: label too long", ErrInvalidDomain)
		}
		// 检查首字符
		if !isAlphaNum(label[0]) {
			return fmt.Errorf("%w: label must start with alphanumeric", ErrInvalidDomain)
		}
		// 检查尾字符（不能以 - 结尾）
		if label[len(label)-1] == '-' {
			return fmt.Errorf("%w: label must not end with hyphen", ErrInvalidDomain)
		}
		// 检查所有字符
		for _, c := range label {
			if !isAlphaNum(byte(c)) && c != '-' && c != '_' {
				return fmt.Errorf("%w: invalid character in label", ErrInvalidDomain)
			}
		}
	}

	return nil
}

func isAlphaNum(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}
