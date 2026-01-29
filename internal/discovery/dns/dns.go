package dns

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("discovery/dns")

// ============================================================================
//                              Discoverer 实现
// ============================================================================

// Discoverer DNS 发现器
type Discoverer struct {
	resolver *Resolver
	config   Config

	// 域名列表
	domains   []string
	domainsMu sync.RWMutex

	// 缓存的节点信息
	peersMu sync.RWMutex
	peers   map[string][]types.PeerInfo // domain -> peers

	// 生命周期
	ctx       context.Context
	ctxCancel context.CancelFunc
	started   atomic.Bool
	wg        sync.WaitGroup

	mu sync.RWMutex
}

// NewDiscoverer 创建 DNS 发现器
func NewDiscoverer(config Config) *Discoverer {
	resolverConfig := ResolverConfig{
		Timeout:        config.Timeout,
		MaxDepth:       config.MaxDepth,
		CustomResolver: config.CustomResolver,
		CacheTTL:       config.CacheTTL,
	}

	d := &Discoverer{
		resolver: NewResolver(resolverConfig),
		config:   config,
		domains:  config.Domains,
		peers:    make(map[string][]types.PeerInfo),
	}

	return d
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动 DNS 发现器
func (d *Discoverer) Start(_ context.Context) error {
	if d.started.Load() {
		return ErrAlreadyStarted
	}

	logger.Info("正在启动 DNS 发现器", "domains", len(d.domains))

	// 使用 context.Background() 而不是传入的 ctx
	// 因为 Fx OnStart 的 ctx 在返回后会被取消，导致后台循环提前退出
	d.ctx, d.ctxCancel = context.WithCancel(context.Background())
	d.started.Store(true)

	// 立即执行一次发现
	d.refreshAll()

	// 启动后台刷新
	d.wg.Add(1)
	go d.refreshLoop()

	logger.Info("DNS 发现器启动成功")
	return nil
}

// Stop 停止 DNS 发现器
func (d *Discoverer) Stop(_ context.Context) error {
	if !d.started.Load() {
		return ErrNotStarted
	}

	logger.Info("正在停止 DNS 发现器")

	d.started.Store(false)
	if d.ctxCancel != nil {
		d.ctxCancel()
	}

	d.wg.Wait()
	logger.Info("DNS 发现器已停止")
	return nil
}

// ============================================================================
//                              Discovery 接口实现
// ============================================================================

// FindPeers 发现节点（实现 Discovery 接口）
func (d *Discoverer) FindPeers(ctx context.Context, ns string, opts ...pkgif.DiscoveryOption) (<-chan types.PeerInfo, error) {
	if !d.started.Load() {
		return nil, ErrNotStarted
	}

	ns = pkgif.NormalizeNamespace(ns)

	logger.Debug("DNS 查找节点", "namespace", ns)

	// 解析选项
	options := &pkgif.DiscoveryOptions{Limit: 100}
	for _, opt := range opts {
		opt(options)
	}

	ch := make(chan types.PeerInfo, options.Limit)

	go func() {
		defer close(ch)

		var domains []string
		if ns == "" || ns == "dns" || ValidateDomain(ns) != nil {
			// 使用所有配置的域名
			domains = d.Domains()
		} else {
			// 将 namespace 作为域名
			domains = []string{ns}
		}

		seen := make(map[string]bool)
		count := 0

		for _, domain := range domains {
			select {
			case <-ctx.Done():
				return
			default:
			}

			peers, err := d.resolver.Resolve(ctx, domain)
			if err != nil {
				continue
			}

			for _, peer := range peers {
				key := string(peer.ID)
				if seen[key] {
					continue
				}
				seen[key] = true

				select {
				case ch <- peer:
					count++
					if options.Limit > 0 && count >= options.Limit {
						return
					}
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return ch, nil
}

// Advertise 广播（实现 Discovery 接口，DNS 不支持动态广播）
func (d *Discoverer) Advertise(_ context.Context, _ string, _ ...pkgif.DiscoveryOption) (time.Duration, error) {
	// DNS 是只读的，不支持动态广播
	return 0, nil
}

// ============================================================================
//                              DNS 方法
// ============================================================================

// Resolve 解析单个域名
func (d *Discoverer) Resolve(ctx context.Context, domain string) ([]types.PeerInfo, error) {
	if !d.started.Load() {
		return nil, ErrNotStarted
	}

	return d.resolver.Resolve(ctx, domain)
}

// ResolveWithDepth 递归解析域名
func (d *Discoverer) ResolveWithDepth(ctx context.Context, domain string, maxDepth int) ([]types.PeerInfo, error) {
	if !d.started.Load() {
		return nil, ErrNotStarted
	}

	return d.resolver.ResolveWithDepth(ctx, domain, maxDepth)
}

// ============================================================================
//                              域名管理
// ============================================================================

// Domains 返回配置的域名列表
func (d *Discoverer) Domains() []string {
	d.domainsMu.RLock()
	defer d.domainsMu.RUnlock()

	result := make([]string, len(d.domains))
	copy(result, d.domains)
	return result
}

// AddDomain 动态添加域名
func (d *Discoverer) AddDomain(domain string) error {
	if err := ValidateDomain(domain); err != nil {
		return err
	}

	d.domainsMu.Lock()
	// 检查是否已存在
	for _, existing := range d.domains {
		if existing == domain {
			d.domainsMu.Unlock()
			return nil
		}
	}

	d.domains = append(d.domains, domain)
	d.domainsMu.Unlock()

	// 立即解析新域名
	if d.started.Load() && d.ctx != nil {
		go d.refreshDomain(domain)
	}

	return nil
}

// RemoveDomain 移除域名
func (d *Discoverer) RemoveDomain(domain string) {
	d.domainsMu.Lock()
	var newDomains []string
	for _, existing := range d.domains {
		if existing != domain {
			newDomains = append(newDomains, existing)
		}
	}
	d.domains = newDomains
	d.domainsMu.Unlock()

	// 清除缓存
	d.peersMu.Lock()
	delete(d.peers, domain)
	d.peersMu.Unlock()
}

// ============================================================================
//                              节点缓存
// ============================================================================

// AllPeers 返回所有已发现的节点
func (d *Discoverer) AllPeers() []types.PeerInfo {
	d.peersMu.RLock()
	defer d.peersMu.RUnlock()

	seen := make(map[string]bool)
	var result []types.PeerInfo

	for _, peers := range d.peers {
		for _, peer := range peers {
			key := string(peer.ID)
			if !seen[key] {
				seen[key] = true
				result = append(result, peer)
			}
		}
	}

	return result
}

// PeersForDomain 返回指定域名的节点
func (d *Discoverer) PeersForDomain(domain string) []types.PeerInfo {
	d.peersMu.RLock()
	defer d.peersMu.RUnlock()

	peers, ok := d.peers[domain]
	if !ok {
		return nil
	}

	result := make([]types.PeerInfo, len(peers))
	copy(result, peers)
	return result
}

// ============================================================================
//                              后台循环
// ============================================================================

// refreshLoop 后台刷新循环
func (d *Discoverer) refreshLoop() {
	defer d.wg.Done()

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
	domains := d.Domains()

	for _, domain := range domains {
		d.refreshDomain(domain)
	}
}

// refreshDomain 刷新单个域名
func (d *Discoverer) refreshDomain(domain string) {
	if d.ctx == nil {
		return
	}

	ctx, cancel := context.WithTimeout(d.ctx, d.config.Timeout)
	defer cancel()

	peers, err := d.resolver.Resolve(ctx, domain)
	if err != nil {
		return
	}

	d.peersMu.Lock()
	d.peers[domain] = peers
	d.peersMu.Unlock()
}

// ============================================================================
//                              缓存重置
// ============================================================================

// Reset 重置 DNS 解析器
//
// 清除所有缓存，在网络变化时调用。
// 新网络可能使用不同的 DNS 服务器，旧缓存可能无效。
func (d *Discoverer) Reset(_ context.Context) error {
	// 清除解析器缓存
	d.resolver.ClearCache()
	
	// 清除节点缓存
	d.peersMu.Lock()
	d.peers = make(map[string][]types.PeerInfo)
	d.peersMu.Unlock()
	
	// 如果正在运行，立即刷新所有域名
	if d.started.Load() {
		go d.refreshAll()
	}
	
	return nil
}

// ============================================================================
//                              统计
// ============================================================================

// Stats 统计信息
type Stats struct {
	TotalDomains int
	TotalPeers   int
	DomainStats  map[string]int
}

// Stats 返回统计信息
func (d *Discoverer) Stats() Stats {
	d.domainsMu.RLock()
	totalDomains := len(d.domains)
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

	return Stats{
		TotalDomains: totalDomains,
		TotalPeers:   totalPeers,
		DomainStats:  domainStats,
	}
}
