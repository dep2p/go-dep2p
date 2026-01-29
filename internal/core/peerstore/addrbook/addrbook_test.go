package addrbook

import (
	"context"
	"sync"
	"testing"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 测试辅助函数
func testPeerID(s string) types.PeerID {
	return types.PeerID(s)
}

func testMultiaddr(s string) types.Multiaddr {
	addr, _ := types.NewMultiaddr(s)
	return addr
}

func TestNew(t *testing.T) {
	ab := New()
	require.NotNil(t, ab)
}

func TestAddrBook_AddAddrs(t *testing.T) {
	ab := New()
	peerID := testPeerID("peer1")
	addrs := []types.Multiaddr{
		testMultiaddr("/ip4/127.0.0.1/tcp/4001"),
		testMultiaddr("/ip4/192.168.1.1/tcp/4001"),
	}

	ab.AddAddrs(peerID, addrs, 10*time.Minute)

	retrieved := ab.Addrs(peerID)
	assert.Len(t, retrieved, 2)
}

func TestAddrBook_SetAddrs(t *testing.T) {
	ab := New()
	peerID := testPeerID("peer1")

	// 先添加一些地址
	ab.AddAddrs(peerID, []types.Multiaddr{testMultiaddr("/ip4/127.0.0.1/tcp/4001")}, 10*time.Minute)

	// 使用 SetAddrs 覆盖
	newAddrs := []types.Multiaddr{testMultiaddr("/ip4/192.168.1.1/tcp/4001")}
	ab.SetAddrs(peerID, newAddrs, 10*time.Minute)

	retrieved := ab.Addrs(peerID)
	assert.Len(t, retrieved, 1)
	assert.Equal(t, newAddrs[0], retrieved[0])
}

func TestAddrBook_TTLExpiry(t *testing.T) {
	ab := New()
	peerID := testPeerID("peer1")
	addr := testMultiaddr("/ip4/127.0.0.1/tcp/4001")

	// 添加地址，TTL 为 100ms
	ab.AddAddrs(peerID, []types.Multiaddr{addr}, 100*time.Millisecond)

	// 立即查询应该能获取到
	addrs := ab.Addrs(peerID)
	assert.Len(t, addrs, 1)

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	// 再次查询应该为空
	addrs = ab.Addrs(peerID)
	assert.Empty(t, addrs)
}

func TestAddrBook_ClearAddrs(t *testing.T) {
	ab := New()
	peerID := testPeerID("peer1")

	ab.AddAddrs(peerID, []types.Multiaddr{testMultiaddr("/ip4/127.0.0.1/tcp/4001")}, 10*time.Minute)
	ab.ClearAddrs(peerID)

	addrs := ab.Addrs(peerID)
	assert.Empty(t, addrs)
}

func TestAddrBook_PeersWithAddrs(t *testing.T) {
	ab := New()

	peer1 := testPeerID("peer1")
	peer2 := testPeerID("peer2")

	ab.AddAddrs(peer1, []types.Multiaddr{testMultiaddr("/ip4/127.0.0.1/tcp/4001")}, 10*time.Minute)
	ab.AddAddrs(peer2, []types.Multiaddr{testMultiaddr("/ip4/192.168.1.1/tcp/4001")}, 10*time.Minute)

	peers := ab.PeersWithAddrs()
	assert.Len(t, peers, 2)
	assert.Contains(t, peers, peer1)
	assert.Contains(t, peers, peer2)
}

func TestAddrBook_AddrStream(t *testing.T) {
	ab := New()
	peerID := testPeerID("peer1")

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// 先添加一些地址
	addrs := []types.Multiaddr{
		testMultiaddr("/ip4/127.0.0.1/tcp/4001"),
		testMultiaddr("/ip4/192.168.1.1/tcp/4001"),
	}
	ab.AddAddrs(peerID, addrs, 10*time.Minute)

	// 获取地址流
	stream := ab.AddrStream(ctx, peerID)

	// 应该能收到现有地址
	received := []types.Multiaddr{}
	for addr := range stream {
		received = append(received, addr)
		if len(received) >= 2 {
			break
		}
	}

	assert.Len(t, received, 2)
}

func TestAddrBook_GC(t *testing.T) {
	ab := New()
	defer ab.Close()

	peerID := types.PeerID("gc-test-peer")
	addr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")

	// 添加一个非常短 TTL 的地址
	ab.AddAddr(peerID, addr, 1*time.Millisecond)

	// 验证地址存在
	addrs := ab.Addrs(peerID)
	assert.Len(t, addrs, 1)

	// 等待地址过期
	time.Sleep(10 * time.Millisecond)

	// 手动触发 GC
	ab.GCNow()

	// 验证过期地址已被清除
	addrs = ab.Addrs(peerID)
	assert.Len(t, addrs, 0, "过期地址应该被 GC 清除")
}

// ============================================================================
//                          缺失功能测试补充
// ============================================================================

// TestAddrBook_SetEventBus 测试设置事件总线
func TestAddrBook_SetEventBus(t *testing.T) {
	ab := New()
	defer ab.Close()

	// 创建模拟事件总线
	mockBus := &mockEventBus{events: make([]interface{}, 0)}
	ab.SetEventBus(mockBus)

	// 添加地址，应触发事件
	peerID := types.PeerID("test-peer")
	addr := testMultiaddr("/ip4/1.2.3.4/tcp/4001")
	ab.AddAddr(peerID, addr, time.Hour)

	// 验证事件已发布
	time.Sleep(50 * time.Millisecond)
	mockBus.mu.Lock()
	eventCount := len(mockBus.events)
	mockBus.mu.Unlock()
	assert.GreaterOrEqual(t, eventCount, 1, "应该发布地址添加事件")
}

// TestAddrBook_SetAddr 测试设置单个地址
func TestAddrBook_SetAddr(t *testing.T) {
	ab := New()
	defer ab.Close()

	peerID := types.PeerID("test-peer")
	addr1 := testMultiaddr("/ip4/1.2.3.4/tcp/4001")
	addr2 := testMultiaddr("/ip4/5.6.7.8/tcp/4002")

	// 设置第一个地址
	ab.SetAddr(peerID, addr1, time.Hour)
	addrs := ab.Addrs(peerID)
	assert.Len(t, addrs, 1)

	// 设置第二个地址（应该覆盖）
	ab.SetAddr(peerID, addr2, time.Hour)
	addrs = ab.Addrs(peerID)
	assert.Len(t, addrs, 1)
	assert.Equal(t, addr2.String(), addrs[0].String())
}

// TestAddrBook_UpdateAddrs 测试更新地址 TTL
func TestAddrBook_UpdateAddrs(t *testing.T) {
	ab := New()
	defer ab.Close()

	peerID := types.PeerID("test-peer")
	addr := testMultiaddr("/ip4/1.2.3.4/tcp/4001")

	// 添加短 TTL 地址
	ab.AddAddr(peerID, addr, 100*time.Millisecond)

	// 更新为长 TTL
	ab.UpdateAddrs(peerID, 100*time.Millisecond, time.Hour)

	// 等待原 TTL 过期时间
	time.Sleep(200 * time.Millisecond)
	ab.GCNow()

	// 地址应该仍然存在（已更新为长 TTL）
	addrs := ab.Addrs(peerID)
	assert.Len(t, addrs, 1, "更新 TTL 后地址不应过期")
}

// TestAddrBook_ClearAddrsBySource 测试按来源清除地址
func TestAddrBook_ClearAddrsBySource(t *testing.T) {
	ab := New()
	defer ab.Close()

	peerID := types.PeerID("test-peer")
	addr1 := testMultiaddr("/ip4/1.2.3.4/tcp/4001")
	addr2 := testMultiaddr("/ip4/5.6.7.8/tcp/4002")

	// 添加不同来源的地址
	ab.AddAddrsWithSource(peerID, []AddressWithSource{
		{Addr: addr1, Source: SourceDHT, TTL: time.Hour},
		{Addr: addr2, Source: SourceMemberList, TTL: time.Hour},
	})

	assert.Len(t, ab.Addrs(peerID), 2)

	// 清除 MemberList 来源的地址
	ab.ClearAddrsBySource(peerID, SourceMemberList)

	// 验证只清除了指定来源的地址
	addrs := ab.Addrs(peerID)
	assert.Len(t, addrs, 1)
	assert.Equal(t, addr1.String(), addrs[0].String())
}

// TestAddrBook_ResetTemporaryAddrs 测试重置临时地址
func TestAddrBook_ResetTemporaryAddrs(t *testing.T) {
	ab := New()
	defer ab.Close()

	peerID := types.PeerID("test-peer")
	longTermAddr := testMultiaddr("/ip4/1.2.3.4/tcp/4001")
	tempAddr := testMultiaddr("/ip4/5.6.7.8/tcp/4002")

	// 添加长期地址（20分钟，大于10分钟阈值）
	ab.AddAddr(peerID, longTermAddr, 20*time.Minute)
	// 添加临时地址（5分钟，小于10分钟阈值）
	ab.AddAddr(peerID, tempAddr, 5*time.Minute)

	assert.Len(t, ab.Addrs(peerID), 2)

	// 重置临时地址（清除 TTL < 10分钟的）
	ab.ResetTemporaryAddrs()

	// 验证只保留长期地址
	addrs := ab.Addrs(peerID)
	assert.Len(t, addrs, 1)
	assert.Equal(t, longTermAddr.String(), addrs[0].String())
}

// TestAddrBook_ConcurrentOperations 测试并发操作
func TestAddrBook_ConcurrentOperations(t *testing.T) {
	ab := New()
	defer ab.Close()

	const goroutines = 20
	const operationsPerGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			peerID := types.PeerID("peer-" + string(rune('A'+id)))
			addr := testMultiaddr("/ip4/1.2.3.4/tcp/4001")

			for j := 0; j < operationsPerGoroutine; j++ {
				// 随机操作
				switch j % 5 {
				case 0:
					ab.AddAddr(peerID, addr, time.Hour)
				case 1:
					_ = ab.Addrs(peerID)
				case 2:
					ab.ClearAddrs(peerID)
				case 3:
					ab.SetAddr(peerID, addr, time.Hour)
				case 4:
					ab.UpdateAddrs(peerID, time.Hour, 2*time.Hour)
				}
			}
		}(i)
	}

	wg.Wait()
	// 不应 panic 或死锁
}

// TestAddrBook_NilInputs 测试 nil 输入
func TestAddrBook_NilInputs(t *testing.T) {
	ab := New()
	defer ab.Close()

	peerID := types.PeerID("test-peer")

	// nil 地址列表
	ab.AddAddrs(peerID, nil, time.Hour)
	assert.Len(t, ab.Addrs(peerID), 0)

	// 空地址列表
	ab.AddAddrs(peerID, []types.Multiaddr{}, time.Hour)
	assert.Len(t, ab.Addrs(peerID), 0)

	// nil EventBus
	assert.NotPanics(t, func() {
		ab.SetEventBus(nil)
	})
}

// TestAddrBook_ZeroTTL 测试零 TTL（立即过期）
func TestAddrBook_ZeroTTL(t *testing.T) {
	ab := New()
	defer ab.Close()

	peerID := types.PeerID("test-peer")
	addr := testMultiaddr("/ip4/1.2.3.4/tcp/4001")

	// 添加零 TTL 地址（立即过期）
	ab.AddAddr(peerID, addr, 0)

	// TTL=0 的地址立即过期，触发 GC 后应被清除
	time.Sleep(10 * time.Millisecond)
	ab.GCNow()

	// 零 TTL 地址应该被清除
	addrs := ab.Addrs(peerID)
	assert.Len(t, addrs, 0, "TTL=0 的地址应立即过期")
}

// mockEventBus 模拟事件总线
type mockEventBus struct {
	mu     sync.Mutex
	events []interface{}
}

func (m *mockEventBus) Subscribe(eventType interface{}, opts ...pkgif.SubscriptionOpt) (pkgif.Subscription, error) {
	return nil, nil
}

func (m *mockEventBus) Emitter(eventType interface{}, opts ...pkgif.EmitterOpt) (pkgif.Emitter, error) {
	return &mockEmitter{bus: m}, nil
}

func (m *mockEventBus) GetAllEventTypes() []interface{} {
	return nil
}

// mockEmitter 模拟事件发射器
type mockEmitter struct {
	bus *mockEventBus
}

func (m *mockEmitter) Emit(event interface{}) error {
	m.bus.mu.Lock()
	defer m.bus.mu.Unlock()
	m.bus.events = append(m.bus.events, event)
	return nil
}

func (m *mockEmitter) Close() error {
	return nil
}
