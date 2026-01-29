package bootstrap

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("discovery/bootstrap")

// BootstrapAddrTTL Bootstrap 节点地址 TTL
//
// 10 年降低到 30 分钟
// 原因：过长的 TTL 导致已断开节点的过期地址持续被使用，
// 造成 "直连失败" dial timeout 反复出现。
// 30 分钟足够维持活跃连接，同时允许过期地址被 GC 清理。
const BootstrapAddrTTL = 30 * time.Minute

// Bootstrap 引导发现服务
type Bootstrap struct {
	ctx       context.Context
	ctxCancel context.CancelFunc
	
	host   pkgif.Host
	config *Config
	
	mu      sync.RWMutex
	peers   []types.PeerInfo
	started atomic.Bool
	closed  atomic.Bool
}

// New 创建 Bootstrap 服务
func New(host pkgif.Host, config *Config) (*Bootstrap, error) {
	if host == nil {
		return nil, fmt.Errorf("host cannot be nil")
	}
	
	if config == nil {
		return nil, fmt.Errorf("config cannot be nil, use ConfigFromUnified() to create config from unified config system")
	}
	
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	
	ctx, cancel := context.WithCancel(context.Background())
	
	return &Bootstrap{
		ctx:       ctx,
		ctxCancel: cancel,
		host:      host,
		config:    config,
		peers:     config.Peers,
	}, nil
}

// Bootstrap 执行引导流程
// 并发连接所有引导节点，要求至少 MinPeers 个成功连接
func (b *Bootstrap) Bootstrap(ctx context.Context) error {
	if b.closed.Load() {
		return ErrAlreadyClosed
	}
	
	b.mu.RLock()
	peers := b.peers
	minPeers := b.config.MinPeers
	timeout := b.config.Timeout
	b.mu.RUnlock()
	
	if len(peers) == 0 {
		logger.Warn("无引导节点配置")
		return ErrNoBootstrapPeers
	}
	
	logger.Info("开始引导流程", 
		"peerCount", len(peers), 
		"minPeers", minPeers,
		"timeout", timeout)
	
	// 打印引导节点列表
	for i, peer := range peers {
		addrs := convertAddrsToStrings(peer.Addrs)
		logger.Debug("引导节点配置", 
			"index", i,
			"peerID", log.TruncateID(string(peer.ID), 8), 
			"addrCount", len(addrs),
			"addrs", addrs)
	}
	
	// 并发连接所有引导节点
	type connResult struct {
		peerID  string
		success bool
		err     error
		duration time.Duration
	}
	results := make(chan connResult, len(peers))
	var wg sync.WaitGroup
	
	startTime := time.Now()
	
	for _, peer := range peers {
		wg.Add(1)
		go func(p types.PeerInfo) {
			defer wg.Done()
			
			peerIDShort := log.TruncateID(string(p.ID), 8)
			connStart := time.Now()
			
			// 为每个连接设置独立的超时
			connCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			
			// 添加地址到 Peerstore（使用永久 TTL）
			if err := b.addAddrsToPeerstore(p); err != nil {
				results <- connResult{
					peerID: peerIDShort,
					success: false,
					err: err,
					duration: time.Since(connStart),
				}
				return
			}
			
			// 连接节点
			addrs := convertAddrsToStrings(p.Addrs)
			logger.Debug("尝试连接引导节点", 
				"peerID", peerIDShort, 
				"addrCount", len(addrs),
				"firstAddr", func() string {
					if len(addrs) > 0 {
						return addrs[0]
					}
					return "<none>"
				}())
			
			if err := b.host.Connect(connCtx, string(p.ID), addrs); err != nil {
				logger.Warn("引导节点连接失败", 
					"peerID", peerIDShort, 
					"error", err,
					"duration", time.Since(connStart))
				results <- connResult{
					peerID: peerIDShort,
					success: false,
					err: err,
					duration: time.Since(connStart),
				}
				return
			}
			
			logger.Info("引导节点连接成功", 
				"peerID", peerIDShort,
				"duration", time.Since(connStart))
			results <- connResult{
				peerID: peerIDShort,
				success: true,
				duration: time.Since(connStart),
			}
		}(peer)
	}
	
	// 等待所有连接尝试完成
	wg.Wait()
	close(results)
	
	// 统计结果
	var successPeers []string
	var failedPeers []string
	var lastErr error
	
	for res := range results {
		if res.success {
			successPeers = append(successPeers, res.peerID)
		} else {
			failedPeers = append(failedPeers, res.peerID)
			lastErr = res.err
		}
	}
	
	successCount := len(successPeers)
	failCount := len(failedPeers)
	
	logger.Info("引导流程完成", 
		"success", successCount, 
		"failed", failCount, 
		"total", len(peers),
		"totalDuration", time.Since(startTime),
		"successPeers", successPeers,
		"failedPeers", failedPeers)
	
	// 检查：所有连接都失败
	if successCount == 0 {
		logger.Error("所有引导节点连接失败", "lastError", lastErr)
		if lastErr != nil {
			return fmt.Errorf("%w: %v", ErrAllConnectionsFailed, lastErr)
		}
		return ErrAllConnectionsFailed
	}
	
	// 检查：成功连接数不足（如果设置了最小连接数要求）
	if minPeers > 0 && successCount < minPeers {
		logger.Warn("引导节点连接数不足", 
			"got", successCount, 
			"want", minPeers,
			"successPeers", successPeers)
		return fmt.Errorf("%w: got %d, want %d", 
			ErrMinPeersNotMet, successCount, minPeers)
	}
	
	return nil
}

// FindPeers 返回引导节点列表（实现 Discovery 接口）
func (b *Bootstrap) FindPeers(ctx context.Context, ns string, _ ...pkgif.DiscoveryOption) (<-chan types.PeerInfo, error) {
	if b.closed.Load() {
		return nil, ErrAlreadyClosed
	}

	ns = pkgif.NormalizeNamespace(ns)
	
	out := make(chan types.PeerInfo)
	
	go func() {
		defer close(out)
		
		b.mu.RLock()
		peers := b.peers
		b.mu.RUnlock()
		
		for _, peer := range peers {
			select {
			case out <- peer:
			case <-ctx.Done():
				return
			case <-b.ctx.Done():
				return
			}
		}
	}()
	
	return out, nil
}

// Advertise 广播自身（Bootstrap 不支持广播）
func (b *Bootstrap) Advertise(_ context.Context, _ string, _ ...pkgif.DiscoveryOption) (time.Duration, error) {
	return 0, ErrNotSupported
}

// Start 启动服务（实现 Discovery 接口）
// 启动时会自动执行引导流程，连接到配置的引导节点
func (b *Bootstrap) Start(_ context.Context) error {
	if !b.started.CompareAndSwap(false, true) {
		return ErrAlreadyStarted
	}
	
	if b.closed.Load() {
		return ErrAlreadyClosed
	}
	
	// 检查是否配置了引导节点
	b.mu.RLock()
	hasPeers := len(b.peers) > 0
	enabled := b.config.Enabled
	b.mu.RUnlock()
	
	// 如果未启用或没有配置引导节点，跳过引导流程
	if !enabled {
		logger.Debug("Bootstrap 未启用，跳过引导流程")
		return nil
	}
	
	if !hasPeers {
		logger.Debug("未配置引导节点，跳过引导流程")
		return nil
	}
	
	// 执行引导流程（连接到引导节点）
	// 使用独立的 goroutine 以避免阻塞 Fx 启动流程
	go func() {
		// 等待一小段时间，确保 Host 完全就绪
		select {
		case <-time.After(100 * time.Millisecond):
		case <-b.ctx.Done():
			return
		}
		
		if err := b.Bootstrap(b.ctx); err != nil {
			logger.Warn("引导流程失败", "error", err)
			// 引导失败不是致命错误，继续运行
			// 节点仍可通过 mDNS 或其他方式发现对等方
		}
	}()
	
	return nil
}

// Stop 停止服务（实现 Discovery 接口）
func (b *Bootstrap) Stop(_ context.Context) error {
	if !b.closed.CompareAndSwap(false, true) {
		return nil // 幂等操作
	}
	
	if b.ctxCancel != nil {
		b.ctxCancel()
	}
	
	return nil
}

// AddPeer 添加引导节点
func (b *Bootstrap) AddPeer(peer types.PeerInfo) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.peers = append(b.peers, peer)
}

// Peers 返回所有引导节点
func (b *Bootstrap) Peers() []types.PeerInfo {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.peers
}

// Started 返回服务是否已启动
func (b *Bootstrap) Started() bool {
	return b.started.Load()
}

// Closed 返回服务是否已关闭
func (b *Bootstrap) Closed() bool {
	return b.closed.Load()
}

// addAddrsToPeerstore 添加地址到 Peerstore
func (b *Bootstrap) addAddrsToPeerstore(peer types.PeerInfo) error {
	peerstore := b.host.Peerstore()
	if peerstore == nil {
		// Peerstore 不可用，跳过（不影响连接）
		return nil
	}

	// 添加地址（使用 Bootstrap 专用 TTL）
	// 
	peerstore.AddAddrs(peer.ID, peer.Addrs, BootstrapAddrTTL)
	
	return nil
}

// convertAddrsToStrings 将 Multiaddr 列表转换为字符串列表
func convertAddrsToStrings(addrs []types.Multiaddr) []string {
	result := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		// 修复 B29: 跳过 nil 地址
		if addr == nil {
			continue
		}
		result = append(result, addr.String())
	}
	return result
}
