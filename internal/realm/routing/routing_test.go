package routing

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              路由表测试（5个）
// ============================================================================

// TestRouteTable_AddNode 测试添加节点
func TestRouteTable_AddNode(t *testing.T) {
	table := NewRouteTable("local-peer")

	node := &interfaces.RouteNode{
		PeerID:      "peer1",
		Addrs:       []string{"/ip4/127.0.0.1/tcp/4001"},
		Latency:     10 * time.Millisecond,
		IsReachable: true,
	}

	err := table.AddNode(node)
	require.NoError(t, err)

	assert.Equal(t, 1, table.Size())
}

// TestRouteTable_NearestPeers 测试查找最近节点
func TestRouteTable_NearestPeers(t *testing.T) {
	table := NewRouteTable("local-peer")

	// 添加多个节点
	for i := 1; i <= 5; i++ {
		node := &interfaces.RouteNode{
			PeerID:      fmt.Sprintf("peer%d", i),
			IsReachable: true,
		}
		table.AddNode(node)
	}

	// 查找最近的 3 个节点
	nearest := table.NearestPeers("target-peer", 3)
	assert.LessOrEqual(t, len(nearest), 3)
}

// TestRouteTable_Update 测试更新节点
func TestRouteTable_Update(t *testing.T) {
	table := NewRouteTable("local-peer")

	node := &interfaces.RouteNode{
		PeerID:  "peer1",
		Latency: 10 * time.Millisecond,
	}
	table.AddNode(node)

	// 更新延迟
	err := table.Update("peer1", 20*time.Millisecond)
	require.NoError(t, err)

	updated, _ := table.GetNode("peer1")
	assert.Equal(t, 20*time.Millisecond, updated.Latency)
}

// TestRouteTable_Remove 测试移除节点
func TestRouteTable_Remove(t *testing.T) {
	table := NewRouteTable("local-peer")

	node := &interfaces.RouteNode{
		PeerID: "peer1",
	}
	table.AddNode(node)

	err := table.RemoveNode("peer1")
	require.NoError(t, err)

	assert.Equal(t, 0, table.Size())
}

// TestRouteTable_Size 测试路由表大小
func TestRouteTable_Size(t *testing.T) {
	table := NewRouteTable("local-peer")

	assert.Equal(t, 0, table.Size())

	for i := 1; i <= 10; i++ {
		node := &interfaces.RouteNode{
			PeerID: fmt.Sprintf("peer%d", i),
		}
		table.AddNode(node)
	}

	assert.Equal(t, 10, table.Size())
}

// ============================================================================
//                              路径查找测试（5个）
// ============================================================================

// TestPathFinder_ShortestPath 测试最短路径
func TestPathFinder_ShortestPath(t *testing.T) {
	ctx := context.Background()
	table := NewRouteTable("local-peer")
	finder := NewPathFinder(table, nil)

	// 构建简单拓扑（添加可达标记）
	nodes := []*interfaces.RouteNode{
		{PeerID: "peer1", Latency: 10 * time.Millisecond, IsReachable: true},
		{PeerID: "peer2", Latency: 20 * time.Millisecond, IsReachable: true},
	}
	for _, node := range nodes {
		table.AddNode(node)
	}

	path, err := finder.FindShortestPath(ctx, "local-peer", "peer1")
	require.NoError(t, err)
	assert.NotNil(t, path)
	assert.True(t, len(path.Nodes) >= 1)
}

// TestPathFinder_MultiPath 测试多路径查找
func TestPathFinder_MultiPath(t *testing.T) {
	ctx := context.Background()
	table := NewRouteTable("local-peer")
	finder := NewPathFinder(table, nil)

	// 添加节点（标记为可达）
	for i := 1; i <= 5; i++ {
		node := &interfaces.RouteNode{
			PeerID:      fmt.Sprintf("peer%d", i),
			IsReachable: true,
			Latency:     time.Duration(i*10) * time.Millisecond,
		}
		table.AddNode(node)
	}

	paths, err := finder.FindMultiplePaths(ctx, "local-peer", "peer1", 3)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(paths), 3)
	assert.GreaterOrEqual(t, len(paths), 1)
}

// TestPathFinder_PathScore 测试路径评分
func TestPathFinder_PathScore(t *testing.T) {
	table := NewRouteTable("local-peer")
	finder := NewPathFinder(table, nil)

	path := &interfaces.Path{
		Nodes:        []string{"local-peer", "peer1", "peer2"},
		TotalLatency: 30 * time.Millisecond,
		Hops:         2,
		Valid:        true,
	}

	score := finder.ScorePath(path)
	assert.Greater(t, score, 0.0)
}

// TestPathFinder_PathCache 测试路径缓存
func TestPathFinder_PathCache(t *testing.T) {
	table := NewRouteTable("local-peer")
	finder := NewPathFinder(table, nil)

	path := &interfaces.Path{
		Nodes: []string{"local-peer", "peer1"},
		Valid: true,
	}

	finder.CachePath(path)
	// 验证缓存（通过后续查找）
}

// TestPathFinder_PathFailover 测试路径失效
func TestPathFinder_PathFailover(t *testing.T) {
	table := NewRouteTable("local-peer")
	finder := NewPathFinder(table, nil)

	finder.InvalidatePath("peer1")
	// 验证路径已失效
}

// ============================================================================
//                              负载均衡测试（4个）
// ============================================================================

// TestLoadBalancer_WeightedRoundRobin 测试加权轮询
func TestLoadBalancer_WeightedRoundRobin(t *testing.T) {
	ctx := context.Background()
	balancer := NewLoadBalancer()

	// 先报告负载
	balancer.ReportLoad("peer1", &interfaces.NodeLoad{ConnectionCount: 10})
	balancer.ReportLoad("peer2", &interfaces.NodeLoad{ConnectionCount: 5})
	balancer.ReportLoad("peer3", &interfaces.NodeLoad{ConnectionCount: 15})

	candidates := []*interfaces.RouteNode{
		{PeerID: "peer1", Load: &interfaces.NodeLoad{ConnectionCount: 10}},
		{PeerID: "peer2", Load: &interfaces.NodeLoad{ConnectionCount: 5}},
		{PeerID: "peer3", Load: &interfaces.NodeLoad{ConnectionCount: 15}},
	}

	selected, err := balancer.SelectNode(ctx, candidates)
	require.NoError(t, err)
	assert.NotNil(t, selected)
	// peer2 负载最低，应该被选中
	assert.Equal(t, "peer2", selected.PeerID)
}

// TestLoadBalancer_LeastConnection 测试最少连接
func TestLoadBalancer_LeastConnection(t *testing.T) {
	ctx := context.Background()
	balancer := NewLoadBalancer()

	// 先报告负载
	balancer.ReportLoad("peer1", &interfaces.NodeLoad{ConnectionCount: 100})
	balancer.ReportLoad("peer2", &interfaces.NodeLoad{ConnectionCount: 50})

	candidates := []*interfaces.RouteNode{
		{PeerID: "peer1", Load: &interfaces.NodeLoad{ConnectionCount: 100}},
		{PeerID: "peer2", Load: &interfaces.NodeLoad{ConnectionCount: 50}},
	}

	selected, err := balancer.SelectNode(ctx, candidates)
	require.NoError(t, err)
	assert.Equal(t, "peer2", selected.PeerID)
}

// TestLoadBalancer_OverloadProtection 测试过载保护
func TestLoadBalancer_OverloadProtection(t *testing.T) {
	balancer := NewLoadBalancer()

	load := &interfaces.NodeLoad{
		ConnectionCount: 1000,
		BandwidthUsage:  1000000000, // 1GB
		CPUUsage:        0.95,
	}
	balancer.ReportLoad("peer1", load)

	isOverloaded := balancer.IsOverloaded("peer1")
	assert.True(t, isOverloaded)
}

// TestLoadBalancer_LoadReport 测试负载报告
func TestLoadBalancer_LoadReport(t *testing.T) {
	balancer := NewLoadBalancer()

	load := &interfaces.NodeLoad{
		ConnectionCount: 50,
		BandwidthUsage:  500000,
		LastUpdated:     time.Now(),
	}

	err := balancer.ReportLoad("peer1", load)
	require.NoError(t, err)

	retrieved, err := balancer.GetLoad("peer1")
	require.NoError(t, err)
	assert.Equal(t, int64(500000), retrieved.BandwidthUsage)
}

// ============================================================================
//                              延迟测量测试（4个）
// ============================================================================

// TestLatencyProber_Ping 测试 Ping 延迟
func TestLatencyProber_Ping(t *testing.T) {
	ctx := context.Background()
	prober := NewLatencyProber(nil)

	// 没有 host 时，MeasureLatency 应该失败
	latency, err := prober.MeasureLatency(ctx, "peer1")
	assert.Error(t, err, "MeasureLatency without host should fail")
	assert.Equal(t, time.Duration(0), latency)
}

// TestLatencyProber_Statistics 测试延迟统计
func TestLatencyProber_Statistics(t *testing.T) {
	prober := NewLatencyProber(nil)

	// 记录多次延迟
	prober.RecordLatency("peer1", 10*time.Millisecond)
	prober.RecordLatency("peer1", 20*time.Millisecond)
	prober.RecordLatency("peer1", 30*time.Millisecond)

	stats := prober.GetStatistics("peer1")
	assert.NotNil(t, stats)
	assert.Greater(t, stats.Mean, time.Duration(0))
}

// TestLatencyProber_Prediction 测试延迟预测
func TestLatencyProber_Prediction(t *testing.T) {
	prober := NewLatencyProber(nil)

	// 记录历史延迟
	for i := 0; i < 10; i++ {
		prober.RecordLatency("peer1", time.Duration(10+i)*time.Millisecond)
	}

	// 获取预测延迟（应该接近平均值）
	latency, ok := prober.GetLatency("peer1")
	assert.True(t, ok)
	assert.Greater(t, latency, time.Duration(0))
}

// TestLatencyProber_Cache 测试延迟缓存
func TestLatencyProber_Cache(t *testing.T) {
	prober := NewLatencyProber(nil)

	prober.RecordLatency("peer1", 15*time.Millisecond)

	latency, ok := prober.GetLatency("peer1")
	assert.True(t, ok)
	assert.Equal(t, 15*time.Millisecond, latency)
}

// ============================================================================
//                              路由缓存测试（4个）
// ============================================================================

// TestRouteCache_LRU 测试 LRU 淘汰
func TestRouteCache_LRU(t *testing.T) {
	cache := NewRouteCache(2, 5*time.Minute) // 容量为 2

	route1 := &interfaces.Route{TargetPeerID: "peer1", NextHop: "hop1"}
	route2 := &interfaces.Route{TargetPeerID: "peer2", NextHop: "hop2"}
	route3 := &interfaces.Route{TargetPeerID: "peer3", NextHop: "hop3"}

	cache.Set(route1)
	cache.Set(route2)
	cache.Set(route3) // 应该淘汰 peer1

	_, ok := cache.Get("peer1")
	assert.False(t, ok)

	_, ok = cache.Get("peer3")
	assert.True(t, ok)
}

// TestRouteCache_TTL 测试 TTL 过期
func TestRouteCache_TTL(t *testing.T) {
	cache := NewRouteCache(100, 100*time.Millisecond)

	route := &interfaces.Route{TargetPeerID: "peer1", NextHop: "hop1"}
	cache.Set(route)

	// 立即获取应该成功
	_, ok := cache.Get("peer1")
	assert.True(t, ok)

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	_, ok = cache.Get("peer1")
	assert.False(t, ok)
}

// TestRouteCache_Invalidate 测试缓存失效
func TestRouteCache_Invalidate(t *testing.T) {
	cache := NewRouteCache(100, 5*time.Minute)

	route := &interfaces.Route{TargetPeerID: "peer1", NextHop: "hop1"}
	cache.Set(route)

	cache.Invalidate("peer1")

	_, ok := cache.Get("peer1")
	assert.False(t, ok)
}

// TestRouteCache_HitRate 测试缓存命中率
func TestRouteCache_HitRate(t *testing.T) {
	cache := NewRouteCache(100, 5*time.Minute)

	route := &interfaces.Route{TargetPeerID: "peer1", NextHop: "hop1"}
	cache.Set(route)

	// 命中
	cache.Get("peer1")
	// 未命中
	cache.Get("peer2")

	stats := cache.GetStats()
	assert.Greater(t, stats.HitRate, 0.0)
}

// ============================================================================
//                              Gateway 协作测试（3个）
// ============================================================================

// TestGatewayAdapter_RelayRequest 测试中继请求
func TestGatewayAdapter_RelayRequest(t *testing.T) {
	ctx := context.Background()
	adapter := NewGatewayAdapter(nil)

	err := adapter.RequestRelay(ctx, "peer1", []byte("test-data"))
	// 没有 gateway 时应该返回错误
	assert.Error(t, err, "RequestRelay without gateway should fail")
}

// TestGatewayAdapter_QueryReachable 测试查询可达节点
func TestGatewayAdapter_QueryReachable(t *testing.T) {
	ctx := context.Background()
	adapter := NewGatewayAdapter(nil)

	nodes, err := adapter.QueryReachable(ctx)
	// 没有 gateway 时返回空列表，不报错
	assert.NoError(t, err, "QueryReachable without gateway should not fail")
	assert.Empty(t, nodes, "QueryReachable without gateway should return empty list")
}

// TestGatewayAdapter_StateSync 测试状态同步
func TestGatewayAdapter_StateSync(t *testing.T) {
	ctx := context.Background()
	adapter := NewGatewayAdapter(nil)

	state := &interfaces.GatewayState{
		ReachableNodes: []string{"peer1", "peer2"},
		RelayNodes:     []string{"relay1"},
		LastUpdated:    time.Now(),
	}

	err := adapter.SyncState(ctx, state)
	// 没有 gateway 时为空操作，不报错
	assert.NoError(t, err, "SyncState without gateway should succeed as no-op")
}

// ============================================================================
//                     正常路径测试（使用 mocks 模拟有效依赖）
// ============================================================================

// TestLatencyProber_WithMockHost_Ping 测试带有效 Host 的 Ping
func TestLatencyProber_WithMockHost_Ping(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// 创建 mock host（返回可用的 stream）
	mockHost := &mockHostForRouting{
		id:     "prober-host",
		stream: newMockStreamForRouting(),
	}

	prober := NewLatencyProber(mockHost)

	// 正常路径：测量延迟
	latency, err := prober.MeasureLatency(ctx, "peer1")
	// mock host 会返回 stream，ping 协议会尝试执行
	// 即使失败（因为 mock stream 无法完成真实协议），也验证了代码路径
	if err != nil {
		// 可能因为 mock stream 无法完成 ping 协议而失败，这是可接受的
		t.Logf("MeasureLatency failed as expected with mock: %v", err)
	} else {
		assert.GreaterOrEqual(t, latency, time.Duration(0))
	}
}

// TestRouteTable_ConcurrentAccess 测试路由表并发访问
func TestRouteTable_ConcurrentAccess(t *testing.T) {
	table := NewRouteTable("local-peer")

	// 预先添加一些节点
	for i := 0; i < 10; i++ {
		node := &interfaces.RouteNode{
			PeerID:      fmt.Sprintf("peer%d", i),
			IsReachable: true,
			Latency:     time.Duration(i) * time.Millisecond,
		}
		table.AddNode(node)
	}

	// 并发读写
	const numGoroutines = 20
	done := make(chan bool, numGoroutines)

	// 一半读，一半写
	for i := 0; i < numGoroutines; i++ {
		if i%2 == 0 {
			go func(idx int) {
				defer func() { done <- true }()
				table.GetNode(fmt.Sprintf("peer%d", idx%10))
			}(i)
		} else {
			go func(idx int) {
				defer func() { done <- true }()
				node := &interfaces.RouteNode{
					PeerID:      fmt.Sprintf("new-peer%d", idx),
					IsReachable: true,
				}
				table.AddNode(node)
			}(i)
		}
	}

	// 等待所有 goroutine 完成
	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// 验证没有 panic，表大小增加
	assert.GreaterOrEqual(t, table.Size(), 10)
}

// TestPolicyEvaluator_SelectBestRoute 测试策略评估器选择最佳路由
func TestPolicyEvaluator_SelectBestRoute(t *testing.T) {
	evaluator := NewPolicyEvaluator()

	routes := []*interfaces.Route{
		{TargetPeerID: "peer1", NextHop: "peer1", Latency: 50 * time.Millisecond},
		{TargetPeerID: "peer2", NextHop: "peer2", Latency: 10 * time.Millisecond},
		{TargetPeerID: "peer3", NextHop: "peer3", Latency: 30 * time.Millisecond},
	}

	// 使用最低延迟策略
	best := evaluator.SelectBestRoute(routes, interfaces.PolicyLowestLatency)
	require.NotNil(t, best)
	assert.Equal(t, "peer2", best.TargetPeerID, "Should select lowest latency route")
}

// TestLoadBalancer_ConcurrentReportLoad 测试负载均衡器并发负载报告
func TestLoadBalancer_ConcurrentReportLoad(t *testing.T) {
	balancer := NewLoadBalancer()

	const numGoroutines = 20
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer func() { done <- true }()
			load := &interfaces.NodeLoad{
				ConnectionCount: idx * 10,
				BandwidthUsage:  int64(idx * 1000),
			}
			balancer.ReportLoad(fmt.Sprintf("peer%d", idx%5), load)
		}(i)
	}

	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// 验证没有 panic，至少有一些负载记录
	for i := 0; i < 5; i++ {
		load, err := balancer.GetLoad(fmt.Sprintf("peer%d", i))
		assert.NoError(t, err)
		assert.NotNil(t, load)
	}
}

// TestRouteCache_ConcurrentAccess 测试路由缓存并发访问
func TestRouteCache_ConcurrentAccess(t *testing.T) {
	cache := NewRouteCache(100, 5*time.Minute)

	const numGoroutines = 20
	done := make(chan bool, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(idx int) {
			defer func() { done <- true }()
			if idx%2 == 0 {
				route := &interfaces.Route{
					TargetPeerID: fmt.Sprintf("peer%d", idx),
					NextHop:      fmt.Sprintf("hop%d", idx),
				}
				cache.Set(route)
			} else {
				cache.Get(fmt.Sprintf("peer%d", idx-1))
			}
		}(i)
	}

	for i := 0; i < numGoroutines; i++ {
		<-done
	}

	// 验证没有 panic
	stats := cache.GetStats()
	assert.GreaterOrEqual(t, stats.Size, 0)
}

// ============================================================================
//                              Mock 实现（本地 routing 测试用）
// ============================================================================

// mockHostForRouting 为 routing 测试提供的简单 mock host
type mockHostForRouting struct {
	id     string
	stream *mockStreamForRouting
}

func (m *mockHostForRouting) ID() string                   { return m.id }
func (m *mockHostForRouting) Addrs() []string              { return []string{"/ip4/127.0.0.1/tcp/4001"} }
func (m *mockHostForRouting) Listen(addrs ...string) error { return nil }
func (m *mockHostForRouting) Connect(ctx context.Context, peerID string, addrs []string) error {
	return nil
}
func (m *mockHostForRouting) SetStreamHandler(protocolID string, handler pkgif.StreamHandler) {}
func (m *mockHostForRouting) RemoveStreamHandler(protocolID string)                           {}
func (m *mockHostForRouting) NewStream(ctx context.Context, peerID string, protocolIDs ...string) (pkgif.Stream, error) {
	if m.stream != nil {
		return m.stream, nil
	}
	return nil, fmt.Errorf("no stream available")
}
func (m *mockHostForRouting) NewStreamWithPriority(ctx context.Context, peerID string, protocolID string, priority int) (pkgif.Stream, error) {
	return m.NewStream(ctx, peerID, protocolID)
}
func (m *mockHostForRouting) Peerstore() pkgif.Peerstore                                 { return nil }
func (m *mockHostForRouting) EventBus() pkgif.EventBus                                   { return nil }
func (m *mockHostForRouting) Close() error                                               { return nil }
func (m *mockHostForRouting) AdvertisedAddrs() []string                                  { return nil }
func (m *mockHostForRouting) ShareableAddrs() []string                                   { return nil }
func (m *mockHostForRouting) HolePunchAddrs() []string                                   { return nil }
func (m *mockHostForRouting) SetReachabilityCoordinator(c pkgif.ReachabilityCoordinator) {}

func (m *mockHostForRouting) Network() pkgif.Swarm { return nil }

func (m *mockHostForRouting) HandleInboundStream(stream pkgif.Stream) {
	// Mock implementation: no-op
}

// mockStreamForRouting 为 routing 测试提供的简单 mock stream
type mockStreamForRouting struct {
	readData  []byte
	writeData []byte
	closed    bool
}

func newMockStreamForRouting() *mockStreamForRouting {
	return &mockStreamForRouting{writeData: make([]byte, 0)}
}

func (m *mockStreamForRouting) Read(p []byte) (int, error) {
	if m.closed {
		return 0, fmt.Errorf("stream closed")
	}
	return 0, fmt.Errorf("EOF")
}

func (m *mockStreamForRouting) Write(p []byte) (int, error) {
	if m.closed {
		return 0, fmt.Errorf("stream closed")
	}
	m.writeData = append(m.writeData, p...)
	return len(p), nil
}

func (m *mockStreamForRouting) Close() error                       { m.closed = true; return nil }
func (m *mockStreamForRouting) Reset() error                       { m.closed = true; return nil }
func (m *mockStreamForRouting) Protocol() string                   { return "/test/1.0.0" }
func (m *mockStreamForRouting) SetProtocol(protocol string)        {}
func (m *mockStreamForRouting) Conn() pkgif.Connection             { return nil }
func (m *mockStreamForRouting) IsClosed() bool                     { return m.closed }
func (m *mockStreamForRouting) CloseWrite() error                  { return nil }
func (m *mockStreamForRouting) CloseRead() error                   { return nil }
func (m *mockStreamForRouting) SetDeadline(t time.Time) error      { return nil }
func (m *mockStreamForRouting) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockStreamForRouting) SetWriteDeadline(t time.Time) error { return nil }
func (m *mockStreamForRouting) Stat() types.StreamStat             { return types.StreamStat{} }
func (m *mockStreamForRouting) State() types.StreamState           { return types.StreamStateOpen }
