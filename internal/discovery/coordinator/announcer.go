// Package coordinator 提供发现协调功能
//
// AddressAnnouncer 实现地址刷新和公告功能：
//   - 定期刷新地址公告
//   - 支持 DHT 和 Realm 发布
//   - 可配置刷新间隔
package coordinator

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var announcerLogger = log.Logger("discovery/announcer")

// ============================================================================
//                              配置
// ============================================================================

// AddressAnnouncerConfig 地址公告器配置
type AddressAnnouncerConfig struct {
	// RefreshInterval 刷新间隔
	RefreshInterval time.Duration

	// AnnounceTimeout 公告超时
	AnnounceTimeout time.Duration

	// Namespaces 要公告的命名空间
	Namespaces []string
}

// DefaultAddressAnnouncerConfig 返回默认配置
func DefaultAddressAnnouncerConfig() AddressAnnouncerConfig {
	return AddressAnnouncerConfig{
		RefreshInterval: 10 * time.Minute,
		AnnounceTimeout: 30 * time.Second,
		Namespaces:      []string{"_dep2p._tcp"},
	}
}

// ============================================================================
//                              AddressAnnouncer 结构
// ============================================================================

// AddressAnnouncer 地址公告器
//
// 定期将本节点地址公告到发现网络，确保其他节点可以找到本节点。
type AddressAnnouncer struct {
	// 配置
	config AddressAnnouncerConfig

	// 依赖组件
	host pkgif.Host // 本地主机

	// 发现源
	discoveries   map[string]pkgif.Discovery
	discoveriesMu sync.RWMutex

	// 状态
	running int32
	closed  int32

	// 同步
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// ============================================================================
//                              构造函数
// ============================================================================

// NewAddressAnnouncer 创建地址公告器
func NewAddressAnnouncer(config AddressAnnouncerConfig) *AddressAnnouncer {
	if config.RefreshInterval <= 0 {
		config.RefreshInterval = DefaultAddressAnnouncerConfig().RefreshInterval
	}
	if config.AnnounceTimeout <= 0 {
		config.AnnounceTimeout = DefaultAddressAnnouncerConfig().AnnounceTimeout
	}
	if len(config.Namespaces) == 0 {
		config.Namespaces = DefaultAddressAnnouncerConfig().Namespaces
	}

	ctx, cancel := context.WithCancel(context.Background())

	aa := &AddressAnnouncer{
		config:      config,
		discoveries: make(map[string]pkgif.Discovery),
		ctx:         ctx,
		cancel:      cancel,
	}

	announcerLogger.Info("地址公告器已创建",
		"refreshInterval", config.RefreshInterval,
		"namespaces", len(config.Namespaces))

	return aa
}

// SetHost 设置 Host
func (aa *AddressAnnouncer) SetHost(h pkgif.Host) {
	aa.host = h
}

// RegisterDiscovery 注册发现源
func (aa *AddressAnnouncer) RegisterDiscovery(name string, d pkgif.Discovery) {
	if d == nil {
		return
	}

	aa.discoveriesMu.Lock()
	defer aa.discoveriesMu.Unlock()

	aa.discoveries[name] = d
}

// UnregisterDiscovery 注销发现源
func (aa *AddressAnnouncer) UnregisterDiscovery(name string) {
	aa.discoveriesMu.Lock()
	defer aa.discoveriesMu.Unlock()

	delete(aa.discoveries, name)
}

// ============================================================================
//                              公告功能
// ============================================================================

// Announce 立即公告地址
//
// 向所有注册的发现源公告本节点地址。
func (aa *AddressAnnouncer) Announce(ctx context.Context) error {
	if atomic.LoadInt32(&aa.closed) == 1 {
		return ErrFinderClosed
	}

	aa.discoveriesMu.RLock()
	discoveries := make(map[string]pkgif.Discovery)
	for k, v := range aa.discoveries {
		discoveries[k] = v
	}
	aa.discoveriesMu.RUnlock()

	if len(discoveries) == 0 {
		announcerLogger.Debug("无可用发现源，跳过公告")
		return nil
	}

	announceCtx, cancel := context.WithTimeout(ctx, aa.config.AnnounceTimeout)
	defer cancel()

	var wg sync.WaitGroup
	var successCount, failCount int32

	for _, ns := range aa.config.Namespaces {
		for name, d := range discoveries {
			// 
			if name == "bootstrap" {
				continue
			}

			wg.Add(1)
			go func(discName string, disc pkgif.Discovery, namespace string) {
				defer wg.Done()

				_, err := disc.Advertise(announceCtx, namespace)
				if err != nil {
					announcerLogger.Debug("公告失败",
						"discovery", discName,
						"namespace", namespace,
						"error", err)
					atomic.AddInt32(&failCount, 1)
				} else {
					announcerLogger.Debug("公告成功",
						"discovery", discName,
						"namespace", namespace)
					atomic.AddInt32(&successCount, 1)
				}
			}(name, d, ns)
		}
	}

	wg.Wait()

	announcerLogger.Info("地址公告完成",
		"success", successCount,
		"failed", failCount)

	return nil
}

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动公告器
func (aa *AddressAnnouncer) Start(_ context.Context) error {
	if !atomic.CompareAndSwapInt32(&aa.running, 0, 1) {
		return nil
	}

	// 延迟首次公告，给 DHT 等组件启动时间
	// FX 的 OnStart 钩子虽然按依赖顺序执行，但 DHT 启动可能需要一些时间
	go func() {
		// 等待 5 秒后再进行首次公告
		select {
		case <-aa.ctx.Done():
			return
		case <-time.After(5 * time.Second):
		}
		_ = aa.Announce(aa.ctx)
	}()

	// 启动定期公告
	aa.wg.Add(1)
	go aa.refreshLoop()

	announcerLogger.Info("地址公告器已启动")
	return nil
}

// refreshLoop 刷新循环
func (aa *AddressAnnouncer) refreshLoop() {
	defer aa.wg.Done()

	ticker := time.NewTicker(aa.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-aa.ctx.Done():
			return
		case <-ticker.C:
			_ = aa.Announce(aa.ctx)
		}
	}
}

// Close 关闭公告器
func (aa *AddressAnnouncer) Close() error {
	if !atomic.CompareAndSwapInt32(&aa.closed, 0, 1) {
		return nil
	}

	aa.cancel()
	aa.wg.Wait()

	announcerLogger.Info("地址公告器已关闭")
	return nil
}

// ============================================================================
//                              统计信息
// ============================================================================

// AddressAnnouncerStats 公告器统计
type AddressAnnouncerStats struct {
	// DiscoverySourceCount 发现源数量
	DiscoverySourceCount int

	// NamespaceCount 命名空间数量
	NamespaceCount int

	// Running 是否运行中
	Running bool
}

// Stats 返回统计信息
func (aa *AddressAnnouncer) Stats() AddressAnnouncerStats {
	aa.discoveriesMu.RLock()
	discCount := len(aa.discoveries)
	aa.discoveriesMu.RUnlock()

	return AddressAnnouncerStats{
		DiscoverySourceCount: discCount,
		NamespaceCount:       len(aa.config.Namespaces),
		Running:              atomic.LoadInt32(&aa.running) == 1,
	}
}
