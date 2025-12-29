// Package dht 提供分布式哈希表实现
package dht

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              生命周期
// ============================================================================

// Start 启动 DHT
func (d *DHT) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&d.running, 0, 1) {
		return nil
	}

	d.ctx, d.cancel = context.WithCancel(ctx)

	log.Info("DHT 启动",
		"mode", dhtModeString(d.Mode()),
		"realm", string(d.realmID))

	// 执行引导
	go d.bootstrap()

	// 启动引导重试循环（解决 seed 节点启动时无可用节点导致引导失败的问题）
	go d.bootstrapRetryLoop()

	// 启动刷新循环
	go d.refreshLoop()

	// 启动清理循环
	go d.cleanupLoop()

	// 启动 PeerRecord 重发布循环（确保记录在 TTL 过期前被续约）
	go d.republishLoop()

	return nil
}

// Stop 停止 DHT
func (d *DHT) Stop() error {
	if !atomic.CompareAndSwapInt32(&d.running, 1, 0) {
		return nil
	}

	if d.cancel != nil {
		d.cancel()
	}

	log.Info("DHT 已停止")
	return nil
}

// Close 关闭 DHT
func (d *DHT) Close() error {
	return d.Stop()
}

// ============================================================================
//                              引导流程
// ============================================================================

// Bootstrap 执行引导
func (d *DHT) Bootstrap(ctx context.Context) error {
	log.Info("执行 DHT 引导")

	// 添加引导节点到路由表
	for _, peer := range d.config.BootstrapPeers {
		node := &RoutingNode{
			ID:       peer.ID,
			Addrs:    types.MultiaddrsToStrings(peer.Addrs),
			LastSeen: time.Now(),
			RealmID:  d.realmID,
		}
		_ = d.routingTable.Update(node) // 忽略 bucket full 错误
	}

	// 查找自己（用于填充路由表）
	_, err := d.FindPeer(ctx, d.localID)
	if err != nil {
		log.Debug("引导查找自己失败", "err", err)
	}

	// 刷新所有桶
	d.refreshBuckets()

	log.Info("DHT 引导完成",
		"routing_table_size", d.routingTable.Size())

	return nil
}

// bootstrap 内部引导
func (d *DHT) bootstrap() {
	ctx, cancel := context.WithTimeout(d.ctx, 60*time.Second)
	defer cancel()

	_ = d.Bootstrap(ctx) // 引导失败在后台会重试
}

// ============================================================================
//                              刷新与清理循环
// ============================================================================

// refreshLoop 刷新循环
func (d *DHT) refreshLoop() {
	ticker := time.NewTicker(d.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			// 如果路由表为空，尝试重新引导（作为 bootstrapRetryLoop 的补充）
			if d.routingTable.Size() == 0 {
				log.Debug("刷新循环：路由表为空，尝试重新引导")
				ctx, cancel := context.WithTimeout(d.ctx, 30*time.Second)
				_ = d.Bootstrap(ctx) // 引导失败在后台会重试
				cancel()
				continue
			}
			d.refreshBuckets()
		}
	}
}

// refreshBuckets 刷新桶
func (d *DHT) refreshBuckets() {
	indices := d.routingTable.BucketsNeedingRefresh()

	for _, idx := range indices {
		// 生成桶范围内的随机 ID
		randomID := RandomIDInBucket(d.localID, idx)

		// 查找该 ID 附近的节点
		ctx, cancel := context.WithTimeout(d.ctx, d.config.QueryTimeout)
		_, _ = d.lookupPeers(ctx, randomID[:]) // 刷新时忽略查找结果
		cancel()

		d.routingTable.MarkBucketRefreshed(idx)
	}
}

// cleanupLoop 清理循环
func (d *DHT) cleanupLoop() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			// 清理过期节点
			d.routingTable.Cleanup()

			// 清理过期值
			d.cleanupStore()
		}
	}
}

// cleanupStore 清理过期存储
func (d *DHT) cleanupStore() {
	d.storeMu.Lock()
	defer d.storeMu.Unlock()

	for key, stored := range d.store {
		if time.Since(stored.Timestamp) > stored.TTL {
			delete(d.store, key)
		}
	}
}

// ============================================================================
//                              PeerRecord 重发布
// ============================================================================

// PeerRecordRepublishInterval PeerRecord 重发布间隔
//
// Layer1 设计要求（docs/05-iterations/2025-12-23-layer1-public-infrastructure.md 第 363, 712 行）：
// PeerRecord 每 20 分钟刷新，而非 TTL/2=30min
const PeerRecordRepublishInterval = 20 * time.Minute

// republishLoop 定期重发布 PeerRecord
//
// PeerRecord TTL=1h，重发布间隔=20min（设计文档要求），确保记录在过期前被续约。
// 这解决了"地址稳定时 PeerRecord 过期后无法被发现"的问题。
//
// 参考：docs/01-design/protocols/network/01-discovery.md
func (d *DHT) republishLoop() {
	// Layer1 修复：使用设计文档要求的 20 分钟间隔
	interval := PeerRecordRepublishInterval
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Debug("PeerRecord 重发布循环已启动",
		"interval", interval,
		"ttl", DefaultPeerRecordTTL)

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.republishLocalAddrs()
		}
	}
}

// ============================================================================
//                              引导重试机制
// ============================================================================

// BootstrapRetryInterval 引导重试间隔
const BootstrapRetryInterval = 30 * time.Second

// BootstrapRetryInitialDelay 引导重试初始延迟（等待初始引导完成）
const BootstrapRetryInitialDelay = 5 * time.Second

// bootstrapRetryLoop 引导重试循环
//
// 解决 seed 节点（或无 bootstrap peer 配置的节点）启动时引导失败的问题：
// - 当初始引导失败（路由表为空）时，定期重试直到成功
// - 一旦路由表有节点，停止重试循环
//
// 问题背景：DHT 启动时立即执行 bootstrap()，如果此时没有可用节点，
// lookupPeers() 返回 ErrNoNodes，引导失败且后续不会自动重试。
// 即使后面有节点连上来，DHT 路由表也一直是空的。
func (d *DHT) bootstrapRetryLoop() {
	// 等待初始引导完成
	select {
	case <-d.ctx.Done():
		return
	case <-time.After(BootstrapRetryInitialDelay):
	}

	ticker := time.NewTicker(BootstrapRetryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			// 如果路由表非空，停止重试
			if d.routingTable.Size() > 0 {
				log.Info("路由表已有节点，停止引导重试循环",
					"routing_table_size", d.routingTable.Size())
				return
			}

			log.Debug("路由表仍为空，尝试重新引导")
			ctx, cancel := context.WithTimeout(d.ctx, 30*time.Second)
			_ = d.Bootstrap(ctx) // 引导失败在后台会重试
			cancel()
		}
	}
}

