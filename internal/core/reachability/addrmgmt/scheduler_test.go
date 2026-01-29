// Package addrmgmt 提供地址管理协议的实现
package addrmgmt

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultSchedulerConfig(t *testing.T) {
	config := DefaultSchedulerConfig()

	assert.Equal(t, 30*time.Minute, config.RefreshInterval)
	assert.Equal(t, 10*time.Minute, config.CleanupInterval)
	assert.Equal(t, 10*time.Second, config.NotifyTimeout)
	assert.Equal(t, 50, config.MaxNeighbors)
}

func TestNewScheduler(t *testing.T) {
	config := DefaultSchedulerConfig()
	handler := NewHandler("local-peer")

	scheduler := NewScheduler(config, "local-peer", handler)

	assert.NotNil(t, scheduler)
	assert.Equal(t, "local-peer", scheduler.localID)
	assert.Equal(t, handler, scheduler.handler)
}

func TestScheduler_StartStop(t *testing.T) {
	config := DefaultSchedulerConfig()
	config.RefreshInterval = 100 * time.Millisecond
	config.CleanupInterval = 100 * time.Millisecond

	handler := NewHandler("local-peer")
	scheduler := NewScheduler(config, "local-peer", handler)

	ctx := context.Background()
	err := scheduler.Start(ctx)
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&scheduler.running))

	// 重复启动应该无效
	err = scheduler.Start(ctx)
	require.NoError(t, err)

	err = scheduler.Stop()
	require.NoError(t, err)
	assert.Equal(t, int32(0), atomic.LoadInt32(&scheduler.running))

	// 重复停止应该无效
	err = scheduler.Stop()
	require.NoError(t, err)
}

func TestScheduler_UpdateLocalAddrs(t *testing.T) {
	config := DefaultSchedulerConfig()
	handler := NewHandler("local-peer")
	scheduler := NewScheduler(config, "local-peer", handler)

	addrs := []string{"/ip4/1.1.1.1/tcp/4001", "/ip4/2.2.2.2/tcp/4002"}
	scheduler.UpdateLocalAddrs(addrs)

	record := scheduler.GetLocalRecord()
	require.NotNil(t, record)
	assert.Equal(t, "local-peer", record.NodeID)
	assert.Len(t, record.Addresses, 2)
	assert.Equal(t, uint64(1), record.Sequence)

	// 更新地址
	newAddrs := []string{"/ip4/3.3.3.3/tcp/4003"}
	scheduler.UpdateLocalAddrs(newAddrs)

	record = scheduler.GetLocalRecord()
	assert.Len(t, record.Addresses, 1)
	assert.Equal(t, uint64(2), record.Sequence)
}

func TestScheduler_SetSignFunction(t *testing.T) {
	config := DefaultSchedulerConfig()
	handler := NewHandler("local-peer")
	scheduler := NewScheduler(config, "local-peer", handler)

	signCount := 0
	scheduler.SetSignFunction(func(record *AddressRecord) error {
		signCount++
		record.Signature = []byte("signed")
		return nil
	})

	scheduler.UpdateLocalAddrs([]string{"/ip4/1.1.1.1/tcp/4001"})

	// 等待异步操作
	time.Sleep(10 * time.Millisecond)

	assert.Equal(t, 1, signCount)

	record := scheduler.GetLocalRecord()
	assert.Equal(t, []byte("signed"), record.Signature)
}

func TestScheduler_SetNeighborFuncs(t *testing.T) {
	config := DefaultSchedulerConfig()
	handler := NewHandler("local-peer")
	scheduler := NewScheduler(config, "local-peer", handler)

	// 只验证函数被正确设置
	scheduler.SetNeighborFuncs(
		func() []string {
			return []string{"peer1", "peer2"}
		},
		nil, // openStream 为 nil，会跳过实际通知
	)

	assert.NotNil(t, scheduler.getNeighbors)

	// 验证 getNeighbors 返回正确的值
	neighbors := scheduler.getNeighbors()
	assert.Len(t, neighbors, 2)
}

func TestScheduler_QueryPeerAddrs_FromCache(t *testing.T) {
	config := DefaultSchedulerConfig()
	handler := NewHandler("local-peer")
	scheduler := NewScheduler(config, "local-peer", handler)

	// 预置缓存
	handler.records["target-peer"] = &AddressRecord{
		NodeID:    "target-peer",
		Addresses: []string{"/ip4/1.1.1.1/tcp/4001"},
		Timestamp: time.Now(),
		TTL:       time.Hour,
	}

	ctx := context.Background()
	addrs, err := scheduler.QueryPeerAddrs(ctx, "target-peer")
	require.NoError(t, err)
	assert.Len(t, addrs, 1)
}

func TestScheduler_QueryPeerAddrs_CacheExpired(t *testing.T) {
	config := DefaultSchedulerConfig()
	handler := NewHandler("local-peer")
	scheduler := NewScheduler(config, "local-peer", handler)

	// 预置过期缓存
	handler.records["target-peer"] = &AddressRecord{
		NodeID:    "target-peer",
		Addresses: []string{"/ip4/1.1.1.1/tcp/4001"},
		Timestamp: time.Now().Add(-2 * time.Hour),
		TTL:       time.Hour,
	}

	ctx := context.Background()
	addrs, err := scheduler.QueryPeerAddrs(ctx, "target-peer")
	require.NoError(t, err)
	// 没有配置邻居函数，返回空
	assert.Nil(t, addrs)
}

func TestScheduler_GetLocalRecord_Nil(t *testing.T) {
	config := DefaultSchedulerConfig()
	handler := NewHandler("local-peer")
	scheduler := NewScheduler(config, "local-peer", handler)

	record := scheduler.GetLocalRecord()
	assert.Nil(t, record)
}

func TestScheduler_RefreshAndCleanup(t *testing.T) {
	config := DefaultSchedulerConfig()
	config.RefreshInterval = 50 * time.Millisecond
	config.CleanupInterval = 50 * time.Millisecond

	handler := NewHandler("local-peer")
	scheduler := NewScheduler(config, "local-peer", handler)

	// 添加初始地址
	scheduler.UpdateLocalAddrs([]string{"/ip4/1.1.1.1/tcp/4001"})

	// 添加过期记录
	handler.records["expired-peer"] = &AddressRecord{
		NodeID:    "expired-peer",
		Timestamp: time.Now().Add(-2 * time.Hour),
		TTL:       time.Hour,
	}

	ctx := context.Background()
	err := scheduler.Start(ctx)
	require.NoError(t, err)

	// 等待刷新和清理
	time.Sleep(100 * time.Millisecond)

	// 检查刷新
	record := scheduler.GetLocalRecord()
	assert.GreaterOrEqual(t, record.Sequence, uint64(1))

	// 检查清理
	assert.Nil(t, handler.GetRecord("expired-peer"))

	scheduler.Stop()
}
