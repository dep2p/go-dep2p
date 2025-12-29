// Package dns 实现通过 DNS TXT 记录发现节点
//
// DNS 发现遵循 libp2p dnsaddr 规范，通过解析 DNS TXT 记录获取节点信息。
//
// TXT 记录格式：
//
//	_dnsaddr.<domain> TXT "dnsaddr=/ip4/<ip>/tcp/<port>/p2p/<nodeID>"
//
// 示例：
//
//	_dnsaddr.bootstrap.dep2p.network TXT "dnsaddr=/ip4/1.2.3.4/tcp/4001/p2p/5Q2STWvB..."
package dns

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 常量定义
const (
	// DNSAddrPrefix dnsaddr 前缀
	DNSAddrPrefix = "dnsaddr="

	// DNSAddrDomainPrefix DNS 地址域名前缀
	DNSAddrDomainPrefix = "_dnsaddr."

	// DefaultTimeout 默认 DNS 查询超时
	DefaultTimeout = 10 * time.Second

	// DefaultMaxDepth 默认最大递归深度
	DefaultMaxDepth = 3

	// DefaultCacheTTL 默认缓存 TTL
	DefaultCacheTTL = 5 * time.Minute
)

// 错误定义
var (
	// ErrInvalidDomain 无效的域名
	ErrInvalidDomain = errors.New("invalid domain")

	// ErrInvalidDNSAddr 无效的 dnsaddr 记录
	ErrInvalidDNSAddr = errors.New("invalid dnsaddr record")

	// ErrMaxDepthExceeded 超过最大递归深度
	ErrMaxDepthExceeded = errors.New("max recursion depth exceeded")

	// ErrNoRecordsFound 未找到记录
	ErrNoRecordsFound = errors.New("no DNS records found")

	// ErrInvalidMultiaddr 无效的 multiaddr
	ErrInvalidMultiaddr = errors.New("invalid multiaddr in dnsaddr")
)

// ResolverConfig 解析器配置
type ResolverConfig struct {
	// Timeout DNS 查询超时
	Timeout time.Duration

	// MaxDepth 最大递归深度
	MaxDepth int

	// CustomResolver 自定义 DNS 解析器地址
	// 格式: <ip>:<port>，例如 "8.8.8.8:53"
	CustomResolver string

	// CacheTTL 缓存 TTL
	CacheTTL time.Duration
}

// DefaultResolverConfig 返回默认解析器配置
func DefaultResolverConfig() ResolverConfig {
	return ResolverConfig{
		Timeout:        DefaultTimeout,
		MaxDepth:       DefaultMaxDepth,
		CustomResolver: "",
		CacheTTL:       DefaultCacheTTL,
	}
}

// cacheEntry 缓存条目
type cacheEntry struct {
	peers     []discoveryif.PeerInfo
	expiresAt time.Time
}

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

// Resolve 解析 DNS 域名获取节点信息
func (r *Resolver) Resolve(ctx context.Context, domain string) ([]discoveryif.PeerInfo, error) {
	return r.ResolveWithDepth(ctx, domain, r.config.MaxDepth)
}

// ResolveWithDepth 递归解析 DNS 域名
func (r *Resolver) ResolveWithDepth(ctx context.Context, domain string, maxDepth int) ([]discoveryif.PeerInfo, error) {
	if maxDepth < 0 {
		return nil, ErrMaxDepthExceeded
	}

	// 规范化域名
	domain = r.normalizeDomain(domain)

		// 检查缓存
	if peers, ok := r.getFromCache(domain); ok {
		log.Debug("使用缓存的 DNS 结果",
			"domain", domain,
			"peers", len(peers))
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

	log.Debug("获取到 DNS TXT 记录",
		"domain", domain,
		"records", len(records))

	// 解析记录
	var peers []discoveryif.PeerInfo
	seen := make(map[string]bool)

	for _, record := range records {
		peer, nestedDomain, err := ParseDNSAddr(record)
		if err != nil {
			log.Debug("跳过无效的 dnsaddr 记录",
				"record", record,
				"err", err)
			continue
		}

		if nestedDomain != "" {
			// 递归解析嵌套域名
			if maxDepth > 0 {
				nestedPeers, err := r.ResolveWithDepth(ctx, nestedDomain, maxDepth-1)
				if err != nil {
					log.Debug("递归解析失败",
						"domain", nestedDomain,
						"err", err)
					continue
				}
				for _, p := range nestedPeers {
					key := p.ID.String()
					if !seen[key] {
						seen[key] = true
						peers = append(peers, p)
					}
				}
			}
		} else if peer != nil {
			key := peer.ID.String()
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

// getFromCache 从缓存获取结果
func (r *Resolver) getFromCache(domain string) ([]discoveryif.PeerInfo, bool) {
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
	peers := make([]discoveryif.PeerInfo, len(entry.peers))
	copy(peers, entry.peers)
	return peers, true
}

// setCache 设置缓存
func (r *Resolver) setCache(domain string, peers []discoveryif.PeerInfo) {
	r.cacheMu.Lock()
	defer r.cacheMu.Unlock()

	// 存储副本
	peersCopy := make([]discoveryif.PeerInfo, len(peers))
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

// ParseDNSAddr 解析 dnsaddr 格式
//
// 支持的格式：
// - dnsaddr=/ip4/<ip>/tcp/<port>/p2p/<nodeID>
// - dnsaddr=/ip6/<ip>/tcp/<port>/p2p/<nodeID>
// - dnsaddr=/dns4/<domain>/tcp/<port>/p2p/<nodeID> (嵌套)
// - dnsaddr=/dnsaddr/<domain> (嵌套域名)
//
// 返回值：
// - peer: 解析出的节点信息（如果是直接地址）
// - nestedDomain: 嵌套的域名（如果是 dnsaddr 嵌套）
// - error: 错误
func ParseDNSAddr(record string) (*discoveryif.PeerInfo, string, error) {
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
		return nil, nestedDomain, nil
	}

	// 解析为 multiaddr
	return parseMultiaddr(addrStr)
}

// parseMultiaddr 解析 multiaddr 字符串
func parseMultiaddr(addrStr string) (*discoveryif.PeerInfo, string, error) {
	// 分割 multiaddr 组件
	parts := strings.Split(addrStr, "/")
	if len(parts) < 2 {
		return nil, "", ErrInvalidMultiaddr
	}

	// 查找 /p2p/ 部分
	var nodeIDStr string
	var addrParts []string

	for i := 0; i < len(parts); i++ {
		if parts[i] == "p2p" && i+1 < len(parts) {
			nodeIDStr = parts[i+1]
			addrParts = parts[:i]
			break
		}
	}

	if nodeIDStr == "" {
		return nil, "", fmt.Errorf("%w: missing /p2p/<nodeID>", ErrInvalidMultiaddr)
	}

	// 解析 NodeID
	nodeID, err := types.ParseNodeID(nodeIDStr)
	if err != nil {
		return nil, "", fmt.Errorf("%w: invalid node ID: %v", ErrInvalidMultiaddr, err)
	}

	// 构建地址字符串并转换为 Multiaddr
	addrString := strings.Join(addrParts, "/")

	peer := &discoveryif.PeerInfo{
		ID:     nodeID,
		Addrs:  types.StringsToMultiaddrs([]string{addrString}),
		Source: "dns",
	}

	return peer, "", nil
}

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
		// 检查所有字符
		for _, c := range label {
			if !isAlphaNum(byte(c)) && c != '-' {
				return fmt.Errorf("%w: invalid character in label", ErrInvalidDomain)
			}
		}
	}

	return nil
}

func isAlphaNum(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

