package rendezvous

import (
	"context"
	"sync"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Point 发现配置
// ============================================================================

// PointDiscoveryConfig Point 发现配置
type PointDiscoveryConfig struct {
	// PointNamespace 用于发现 Rendezvous Points 的命名空间
	PointNamespace string

	// RefreshInterval 刷新间隔
	RefreshInterval time.Duration

	// MinPoints 最小 Point 数量
	MinPoints int

	// MaxPoints 最大 Point 数量
	MaxPoints int

	// DiscoverTimeout 发现超时
	DiscoverTimeout time.Duration
}

// DefaultPointDiscoveryConfig 默认配置
func DefaultPointDiscoveryConfig() PointDiscoveryConfig {
	return PointDiscoveryConfig{
		PointNamespace:  "/dep2p/rendezvous-point/1.0.0",
		RefreshInterval: 10 * time.Minute,
		MinPoints:       3,
		MaxPoints:       10,
		DiscoverTimeout: 30 * time.Second,
	}
}

// ============================================================================
//                              PointDiscovery 实现
// ============================================================================

// PointDiscovery 通过 DHT 发现 Rendezvous Points
type PointDiscovery struct {
	config PointDiscoveryConfig
	dht    pkgif.DHT
	host   pkgif.Host

	// 已发现的 Points
	points   []types.PeerID
	pointsMu sync.RWMutex

	// 生命周期
	ctx       context.Context
	ctxCancel context.CancelFunc
	wg        sync.WaitGroup
	started   bool
}

// NewPointDiscovery 创建 Point 发现器
func NewPointDiscovery(dht pkgif.DHT, host pkgif.Host, config PointDiscoveryConfig) *PointDiscovery {
	ctx, cancel := context.WithCancel(context.Background())

	return &PointDiscovery{
		config:    config,
		dht:       dht,
		host:      host,
		points:    make([]types.PeerID, 0),
		ctx:       ctx,
		ctxCancel: cancel,
	}
}

// Start 启动 Point 发现
func (pd *PointDiscovery) Start(_ context.Context) error {
	if pd.started {
		return ErrAlreadyStarted
	}

	pd.started = true

	// 立即执行一次发现
	pd.discoverPoints()

	// 启动后台刷新
	pd.wg.Add(1)
	go pd.refreshLoop()

	return nil
}

// Stop 停止 Point 发现
func (pd *PointDiscovery) Stop() error {
	if !pd.started {
		return ErrNotStarted
	}

	pd.started = false
	pd.ctxCancel()
	pd.wg.Wait()

	return nil
}

// GetPoints 获取已发现的 Points
func (pd *PointDiscovery) GetPoints() []types.PeerID {
	pd.pointsMu.RLock()
	defer pd.pointsMu.RUnlock()

	result := make([]types.PeerID, len(pd.points))
	copy(result, pd.points)
	return result
}

// AnnounceAsPoint 宣告自己为 Rendezvous Point
func (pd *PointDiscovery) AnnounceAsPoint(ctx context.Context) error {
	if pd.dht == nil {
		return ErrNilHost
	}

	// 通过 DHT 广播
	_, err := pd.dht.Advertise(ctx, pd.config.PointNamespace)
	return err
}

// ============================================================================
//                              内部方法
// ============================================================================

// refreshLoop 刷新循环
func (pd *PointDiscovery) refreshLoop() {
	defer pd.wg.Done()

	ticker := time.NewTicker(pd.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pd.discoverPoints()

		case <-pd.ctx.Done():
			return
		}
	}
}

// discoverPoints 发现 Points
func (pd *PointDiscovery) discoverPoints() {
	if pd.dht == nil {
		return
	}

	ctx, cancel := context.WithTimeout(pd.ctx, pd.config.DiscoverTimeout)
	defer cancel()

	// 通过 DHT 发现 Points
	peerCh, err := pd.dht.FindPeers(ctx, pd.config.PointNamespace)
	if err != nil {
		return
	}

	// 收集发现的 Points
	var newPoints []types.PeerID
	for peer := range peerCh {
		// 跳过自己
		if string(peer.ID) == pd.host.ID() {
			continue
		}

		newPoints = append(newPoints, peer.ID)

		// 达到最大数量
		if len(newPoints) >= pd.config.MaxPoints {
			break
		}
	}

	// 更新 Points 列表
	if len(newPoints) > 0 {
		pd.pointsMu.Lock()
		pd.points = newPoints
		pd.pointsMu.Unlock()
	}
}

// ============================================================================
//                              Discoverer 集成
// ============================================================================

// DiscovererWithDHT 带 DHT 支持的 Discoverer
type DiscovererWithDHT struct {
	*Discoverer
	pointDiscovery *PointDiscovery
}

// NewDiscovererWithDHT 创建带 DHT 支持的 Discoverer
func NewDiscovererWithDHT(host pkgif.Host, dht pkgif.DHT, config DiscovererConfig) *DiscovererWithDHT {
	discoverer := NewDiscoverer(host, config)

	pointConfig := DefaultPointDiscoveryConfig()
	pointDiscovery := NewPointDiscovery(dht, host, pointConfig)

	return &DiscovererWithDHT{
		Discoverer:     discoverer,
		pointDiscovery: pointDiscovery,
	}
}

// Start 启动
func (d *DiscovererWithDHT) Start(ctx context.Context) error {
	// 启动 Point 发现
	if err := d.pointDiscovery.Start(ctx); err != nil {
		return err
	}

	// 启动 Discoverer
	if err := d.Discoverer.Start(ctx); err != nil {
		d.pointDiscovery.Stop()
		return err
	}

	return nil
}

// Stop 停止
func (d *DiscovererWithDHT) Stop(ctx context.Context) error {
	// 停止 Discoverer
	if err := d.Discoverer.Stop(ctx); err != nil {
		return err
	}

	// 停止 Point 发现
	return d.pointDiscovery.Stop()
}
