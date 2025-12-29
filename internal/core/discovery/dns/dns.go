package dns

import (
	"context"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/util/logger"
	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
)

// 包级别日志实例
var log = logger.Logger("discovery.dns")

// DiscovererConfig DNS 发现器配置
type DiscovererConfig struct {
	// Domains DNS 域名列表
	Domains []string

	// Timeout DNS 查询超时
	Timeout time.Duration

	// MaxDepth 最大递归深度
	MaxDepth int

	// CacheTTL 缓存 TTL
	CacheTTL time.Duration

	// CustomResolver 自定义 DNS 解析器
	CustomResolver string

	// RefreshInterval 后台刷新间隔
	RefreshInterval time.Duration
}

// DefaultDiscovererConfig 返回默认配置
func DefaultDiscovererConfig() DiscovererConfig {
	return DiscovererConfig{
		Domains:         nil,
		Timeout:         DefaultTimeout,
		MaxDepth:        DefaultMaxDepth,
		CacheTTL:        DefaultCacheTTL,
		CustomResolver:  "",
		RefreshInterval: 5 * time.Minute,
	}
}

// Discoverer DNS 发现器
//
// 通过 DNS TXT 记录发现节点，实现 DNSDiscoverer 和 NamespaceDiscoverer 接口。
type Discoverer struct {
	resolver *Resolver
	config   DiscovererConfig

	// 后台刷新
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// 运行状态
	running bool
	mu      sync.RWMutex

	// 域名列表保护
	domainsMu sync.RWMutex

	// 缓存的节点信息
	peersMu sync.RWMutex
	peers   map[string][]discoveryif.PeerInfo // domain -> peers
}

// NewDiscoverer 创建 DNS 发现器
func NewDiscoverer(config DiscovererConfig) *Discoverer {
	resolverConfig := ResolverConfig{
		Timeout:        config.Timeout,
		MaxDepth:       config.MaxDepth,
		CustomResolver: config.CustomResolver,
		CacheTTL:       config.CacheTTL,
	}

	d := &Discoverer{
		resolver: NewResolver(resolverConfig),
		config:   config,
		peers:    make(map[string][]discoveryif.PeerInfo),
	}

	return d
}

// Start 启动 DNS 发现器
func (d *Discoverer) Start(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.running {
		return nil
	}

	d.ctx, d.cancel = context.WithCancel(ctx)
	d.running = true

	// 立即执行一次发现
	d.refreshAll()

	// 启动后台刷新
	d.wg.Add(1)
	go d.refreshLoop()

	log.Info("DNS 发现器已启动",
		"domains", d.config.Domains,
		"refresh_interval", d.config.RefreshInterval)

	return nil
}

// Stop 停止 DNS 发现器
func (d *Discoverer) Stop() error {
	d.mu.Lock()
	if !d.running {
		d.mu.Unlock()
		return nil
	}
	d.running = false
	if d.cancel != nil {
		d.cancel()
	}
	d.mu.Unlock()

	d.wg.Wait()
	log.Info("DNS 发现器已停止")
	return nil
}

// Resolve 解析单个域名
func (d *Discoverer) Resolve(ctx context.Context, domain string) ([]discoveryif.PeerInfo, error) {
	return d.resolver.Resolve(ctx, domain)
}

// ResolveWithDepth 递归解析域名
func (d *Discoverer) ResolveWithDepth(ctx context.Context, domain string, maxDepth int) ([]discoveryif.PeerInfo, error) {
	return d.resolver.ResolveWithDepth(ctx, domain, maxDepth)
}

// Domains 返回配置的域名列表
func (d *Discoverer) Domains() []string {
	d.domainsMu.RLock()
	defer d.domainsMu.RUnlock()

	result := make([]string, len(d.config.Domains))
	copy(result, d.config.Domains)
	return result
}

// DiscoverPeers 实现 NamespaceDiscoverer 接口
//
// 对于 DNS 发现，namespace 参数被解释为域名。
// 如果 namespace 为空或为 "dns"，则从所有配置的域名发现节点。
func (d *Discoverer) DiscoverPeers(ctx context.Context, namespace string) (<-chan discoveryif.PeerInfo, error) {
	ch := make(chan discoveryif.PeerInfo, 100)

	go func() {
		defer close(ch)

		var domains []string
		if namespace == "" || namespace == "dns" {
			// 使用所有配置的域名
			d.domainsMu.RLock()
			domains = make([]string, len(d.config.Domains))
			copy(domains, d.config.Domains)
			d.domainsMu.RUnlock()
		} else {
			// 将 namespace 作为域名
			domains = []string{namespace}
		}

		seen := make(map[string]bool)

		for _, domain := range domains {
			select {
			case <-ctx.Done():
				return
			default:
			}

			peers, err := d.resolver.Resolve(ctx, domain)
			if err != nil {
				log.Debug("DNS 发现失败",
					"domain", domain,
					"err", err)
				continue
			}

			for _, peer := range peers {
				key := peer.ID.String()
				if seen[key] {
					continue
				}
				seen[key] = true

				select {
				case ch <- peer:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ch, nil
}

// AllPeers 返回所有已发现的节点
func (d *Discoverer) AllPeers() []discoveryif.PeerInfo {
	d.peersMu.RLock()
	defer d.peersMu.RUnlock()

	seen := make(map[string]bool)
	var result []discoveryif.PeerInfo

	for _, peers := range d.peers {
		for _, peer := range peers {
			key := peer.ID.String()
			if !seen[key] {
				seen[key] = true
				result = append(result, peer)
			}
		}
	}

	return result
}

// PeersForDomain 返回指定域名的节点
func (d *Discoverer) PeersForDomain(domain string) []discoveryif.PeerInfo {
	d.peersMu.RLock()
	defer d.peersMu.RUnlock()

	peers, ok := d.peers[domain]
	if !ok {
		return nil
	}

	result := make([]discoveryif.PeerInfo, len(peers))
	copy(result, peers)
	return result
}

// AddDomain 动态添加域名
func (d *Discoverer) AddDomain(domain string) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}

	d.domainsMu.Lock()
	// 检查是否已存在
	for _, existing := range d.config.Domains {
		if existing == domain {
			d.domainsMu.Unlock()
			return nil
		}
	}

	d.config.Domains = append(d.config.Domains, domain)
	d.domainsMu.Unlock()

	// 立即解析新域名
	if d.ctx != nil {
		go d.refreshDomain(domain)
	}

	return nil
}

// RemoveDomain 移除域名
func (d *Discoverer) RemoveDomain(domain string) {
	d.domainsMu.Lock()
	var newDomains []string
	for _, existing := range d.config.Domains {
		if existing != domain {
			newDomains = append(newDomains, existing)
		}
	}
	d.config.Domains = newDomains
	d.domainsMu.Unlock()

	// 清除缓存
	d.peersMu.Lock()
	delete(d.peers, domain)
	d.peersMu.Unlock()
}

// refreshLoop 后台刷新循环
func (d *Discoverer) refreshLoop() {
	defer d.wg.Done()

	// 检查 ctx 是否为 nil（防止 Start() 未调用）
	if d.ctx == nil {
		return
	}

	ticker := time.NewTicker(d.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.refreshAll()
		}
	}
}

// refreshAll 刷新所有域名
func (d *Discoverer) refreshAll() {
	d.domainsMu.RLock()
	domains := make([]string, len(d.config.Domains))
	copy(domains, d.config.Domains)
	d.domainsMu.RUnlock()

	for _, domain := range domains {
		d.refreshDomain(domain)
	}
}

// refreshDomain 刷新单个域名
func (d *Discoverer) refreshDomain(domain string) {
	// 检查 ctx 是否为 nil（防止 Start() 未调用）
	if d.ctx == nil {
		return
	}

	ctx, cancel := context.WithTimeout(d.ctx, d.config.Timeout)
	defer cancel()

	peers, err := d.resolver.Resolve(ctx, domain)
	if err != nil {
		log.Debug("刷新域名失败",
			"domain", domain,
			"err", err)
		return
	}

	d.peersMu.Lock()
	d.peers[domain] = peers
	d.peersMu.Unlock()

	log.Debug("刷新域名成功",
		"domain", domain,
		"peers", len(peers))
}

// Stats 返回统计信息
func (d *Discoverer) Stats() DNSDiscovererStats {
	d.domainsMu.RLock()
	totalDomains := len(d.config.Domains)
	d.domainsMu.RUnlock()

	d.peersMu.RLock()
	defer d.peersMu.RUnlock()

	totalPeers := 0
	domainStats := make(map[string]int)

	for domain, peers := range d.peers {
		count := len(peers)
		domainStats[domain] = count
		totalPeers += count
	}

	return DNSDiscovererStats{
		TotalDomains: totalDomains,
		TotalPeers:   totalPeers,
		DomainStats:  domainStats,
	}
}

// DNSDiscovererStats DNS 发现器统计
type DNSDiscovererStats struct {
	// TotalDomains 配置的域名总数
	TotalDomains int

	// TotalPeers 发现的节点总数
	TotalPeers int

	// DomainStats 每个域名的节点数
	DomainStats map[string]int
}

