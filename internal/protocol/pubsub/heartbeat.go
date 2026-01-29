// Package pubsub 实现发布订阅协议
package pubsub

import (
	"context"
	"sync"
	"time"
)

// heartbeat 心跳管理器
type heartbeat struct {
	interval  time.Duration
	gossip    *gossipSub
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup // 等待 goroutine 退出
	tickCount int            // 心跳计数器，用于低频任务
}

// newHeartbeat 创建心跳管理器
func newHeartbeat(interval time.Duration, gossip *gossipSub) *heartbeat {
	return &heartbeat{
		interval: interval,
		gossip:   gossip,
	}
}

// Start 启动心跳
func (hb *heartbeat) Start(ctx context.Context) {
	hb.ctx, hb.cancel = context.WithCancel(ctx)

	hb.wg.Add(1)
	go hb.run()
}

// Stop 停止心跳
func (hb *heartbeat) Stop() {
	if hb.cancel != nil {
		hb.cancel()
		hb.wg.Wait() // 等待 goroutine 完全退出
	}
}

// run 运行心跳循环
func (hb *heartbeat) run() {
	defer hb.wg.Done() // 确保退出时调用 Done

	ticker := time.NewTicker(hb.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hb.tick()
		case <-hb.ctx.Done():
			return
		}
	}
}

// tick 执行一次心跳
func (hb *heartbeat) tick() {
	hb.tickCount++

	// 维护 Mesh
	hb.gossip.maintainMesh()

	// 清理过期的已见消息
	hb.gossip.cleanupSeenMessages()

	// 清理过期的消息缓存
	hb.gossip.cleanupMessageCache()

	// P1 修复完成：执行评分衰减
	hb.gossip.decayScores()

	// 每 60 次心跳（约 1 分钟）清理过期退避记录
	if hb.tickCount%60 == 0 {
		hb.gossip.cleanupConnectBackoff()
	}
}
