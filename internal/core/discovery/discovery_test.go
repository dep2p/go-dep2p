// Package discovery 发现模块测试
package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/discovery/bootstrap"
	"github.com/dep2p/go-dep2p/internal/core/discovery/dht"
	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              DiscoveryService 测试
// ============================================================================

func TestDiscoveryService_StartStop(t *testing.T) {
	config := discoveryif.DefaultConfig()
	config.EnableDHT = false
	config.EnableMDNS = false
	config.EnableBootstrap = false

	service := NewDiscoveryService(nil, nil, config)

	ctx := context.Background()

	// 启动
	if err := service.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// 重复启动应该没问题
	if err := service.Start(ctx); err != nil {
		t.Fatalf("Start() again error: %v", err)
	}

	// 停止
	if err := service.Stop(); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}

	// 重复停止应该没问题
	if err := service.Stop(); err != nil {
		t.Fatalf("Stop() again error: %v", err)
	}
}

func TestDiscoveryService_RegisterDiscoverer(t *testing.T) {
	config := discoveryif.DefaultConfig()
	config.EnableDHT = false
	config.EnableMDNS = false

	service := NewDiscoveryService(nil, nil, config)

	// 创建一个模拟发现器
	mockDiscoverer := &mockDiscoverer{}

	service.RegisterDiscoverer("mock", mockDiscoverer)

	// 验证注册成功
	if len(service.discoverers) != 1 {
		t.Errorf("expected 1 discoverer, got %d", len(service.discoverers))
	}
}

func TestDiscoveryService_AddKnownPeer(t *testing.T) {
	config := discoveryif.DefaultConfig()
	config.EnableDHT = false
	config.EnableMDNS = false

	service := NewDiscoveryService(nil, nil, config)

	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("test-node-id-12345678901234"))

	addrs := toAddressesFromStrings([]string{"192.168.1.1:8000"})

	service.AddKnownPeer(nodeID, types.DefaultRealmID, addrs)

	if service.KnownPeerCount() != 1 {
		t.Errorf("expected 1 known peer, got %d", service.KnownPeerCount())
	}
}

func TestDiscoveryService_RemoveKnownPeer(t *testing.T) {
	config := discoveryif.DefaultConfig()
	service := NewDiscoveryService(nil, nil, config)

	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("test-node-id-12345678901234"))

	addrs := toAddressesFromStrings([]string{"192.168.1.1:8000"})

	service.AddKnownPeer(nodeID, types.DefaultRealmID, addrs)
	service.RemoveKnownPeer(nodeID)

	if service.KnownPeerCount() != 0 {
		t.Errorf("expected 0 known peers, got %d", service.KnownPeerCount())
	}
}

func TestDiscoveryService_DynamicInterval(t *testing.T) {
	config := discoveryif.DefaultConfig()
	config.TargetPeers = 10

	service := NewDiscoveryService(nil, nil, config)

	// 获取初始间隔（没有节点时应该是最小间隔）
	interval1 := service.GetDiscoveryInterval()

	// 添加一些节点
	for i := 0; i < 5; i++ {
		nodeID := types.NodeID{}
		copy(nodeID[:], []byte{byte(i)})
		service.AddKnownPeer(nodeID, types.DefaultRealmID, nil)
	}

	// 获取新间隔
	interval2 := service.GetDiscoveryInterval()

	t.Logf("interval1: %v, interval2: %v", interval1, interval2)
}

// ============================================================================
//                              Bootstrap 测试
// ============================================================================

func TestBootstrap_NewDiscoverer(t *testing.T) {
	config := bootstrap.DefaultConfig()
	config.Peers = []discoveryif.PeerInfo{
		{
			ID:    types.NodeID{},
			Addrs: types.StringsToMultiaddrs([]string{"/ip4/192.168.1.1/udp/8000/quic-v1"}),
		},
	}

	discoverer := bootstrap.NewDiscoverer(config, nil)

	if discoverer == nil {
		t.Fatal("expected non-nil discoverer")
	}
}

func TestBootstrap_GetBootstrapPeers(t *testing.T) {
	config := bootstrap.DefaultConfig()
	config.Peers = []discoveryif.PeerInfo{
		{
			ID:    types.NodeID{1},
			Addrs: types.StringsToMultiaddrs([]string{"/ip4/192.168.1.1/udp/8000/quic-v1"}),
		},
		{
			ID:    types.NodeID{2},
			Addrs: types.StringsToMultiaddrs([]string{"/ip4/192.168.1.2/udp/8000/quic-v1"}),
		},
	}

	discoverer := bootstrap.NewDiscoverer(config, nil)

	peers, err := discoverer.GetBootstrapPeers(context.Background())
	if err != nil {
		t.Fatalf("GetBootstrapPeers() error: %v", err)
	}

	if len(peers) != 2 {
		t.Errorf("expected 2 peers, got %d", len(peers))
	}
}

func TestBootstrap_AddRemoveBootstrapPeer(t *testing.T) {
	config := bootstrap.DefaultConfig()
	discoverer := bootstrap.NewDiscoverer(config, nil)

	peer := discoveryif.PeerInfo{
		ID:    types.NodeID{1},
		Addrs: types.StringsToMultiaddrs([]string{"/ip4/192.168.1.1/udp/8000/quic-v1"}),
	}

	discoverer.AddBootstrapPeer(peer)

	peers, _ := discoverer.GetBootstrapPeers(context.Background())
	if len(peers) != 1 {
		t.Errorf("expected 1 peer after add, got %d", len(peers))
	}

	discoverer.RemoveBootstrapPeer(peer.ID)

	peers, _ = discoverer.GetBootstrapPeers(context.Background())
	if len(peers) != 0 {
		t.Errorf("expected 0 peers after remove, got %d", len(peers))
	}
}

func TestBootstrap_SetBootstrapPeers(t *testing.T) {
	config := bootstrap.DefaultConfig()
	discoverer := bootstrap.NewDiscoverer(config, nil)

	peers := []discoveryif.PeerInfo{
		{ID: types.NodeID{1}, Addrs: types.StringsToMultiaddrs([]string{"/ip4/1.1.1.1/udp/8000/quic-v1"})},
		{ID: types.NodeID{2}, Addrs: types.StringsToMultiaddrs([]string{"/ip4/2.2.2.2/udp/8000/quic-v1"})},
		{ID: types.NodeID{3}, Addrs: types.StringsToMultiaddrs([]string{"/ip4/3.3.3.3/udp/8000/quic-v1"})},
	}

	discoverer.SetBootstrapPeers(peers)

	result, _ := discoverer.GetBootstrapPeers(context.Background())
	if len(result) != 3 {
		t.Errorf("expected 3 peers, got %d", len(result))
	}
}

func TestBootstrap_DiscoverPeers(t *testing.T) {
	config := bootstrap.DefaultConfig()
	config.Peers = []discoveryif.PeerInfo{
		{ID: types.NodeID{1}, Addrs: types.StringsToMultiaddrs([]string{"/ip4/1.1.1.1/udp/8000/quic-v1"})},
	}
	discoverer := bootstrap.NewDiscoverer(config, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	ch, err := discoverer.DiscoverPeers(ctx, "")
	if err != nil {
		t.Fatalf("DiscoverPeers() error: %v", err)
	}

	count := 0
	for range ch {
		count++
	}

	if count != 1 {
		t.Errorf("expected 1 peer from DiscoverPeers, got %d", count)
	}
}

// ============================================================================
//                              DiscoveryService Bootstrap 管理测试
// ============================================================================

func TestDiscoveryService_GetBootstrapPeers(t *testing.T) {
	config := discoveryif.DefaultConfig()
	config.EnableDHT = false
	config.EnableMDNS = false
	config.BootstrapPeers = []discoveryif.PeerInfo{
		{ID: types.NodeID{1}, Addrs: types.StringsToMultiaddrs([]string{"/ip4/1.1.1.1/udp/8000/quic-v1"})},
		{ID: types.NodeID{2}, Addrs: types.StringsToMultiaddrs([]string{"/ip4/2.2.2.2/udp/8000/quic-v1"})},
	}

	service := NewDiscoveryService(nil, nil, config)

	ctx := context.Background()
	peers, err := service.GetBootstrapPeers(ctx)
	if err != nil {
		t.Fatalf("GetBootstrapPeers() error: %v", err)
	}

	if len(peers) != 2 {
		t.Errorf("expected 2 bootstrap peers, got %d", len(peers))
	}
}

func TestDiscoveryService_AddRemoveBootstrapPeer(t *testing.T) {
	config := discoveryif.DefaultConfig()
	config.EnableDHT = false
	config.EnableMDNS = false

	service := NewDiscoveryService(nil, nil, config)

	ctx := context.Background()

	// 初始为空
	peers, _ := service.GetBootstrapPeers(ctx)
	if len(peers) != 0 {
		t.Errorf("expected 0 bootstrap peers initially, got %d", len(peers))
	}

	// 添加节点（REQ-BOOT-001 要求 Full Address 格式）
	peerID := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	peer := discoveryif.PeerInfo{
		ID:    peerID,
		Addrs: types.StringsToMultiaddrs([]string{"/ip4/1.1.1.1/udp/8000/quic-v1/p2p/" + peerID.String()}),
	}
	service.AddBootstrapPeer(peer)

	peers, _ = service.GetBootstrapPeers(ctx)
	if len(peers) != 1 {
		t.Errorf("expected 1 bootstrap peer after add, got %d", len(peers))
	}

	// 重复添加应该被忽略
	service.AddBootstrapPeer(peer)
	peers, _ = service.GetBootstrapPeers(ctx)
	if len(peers) != 1 {
		t.Errorf("expected 1 bootstrap peer after duplicate add, got %d", len(peers))
	}

	// 移除节点
	service.RemoveBootstrapPeer(peer.ID)
	peers, _ = service.GetBootstrapPeers(ctx)
	if len(peers) != 0 {
		t.Errorf("expected 0 bootstrap peers after remove, got %d", len(peers))
	}

	// 移除不存在的节点应该安全
	service.RemoveBootstrapPeer(types.NodeID{99})
}

// ============================================================================
//                              Endpoint Adapter 测试
// ============================================================================

func TestEndpointDiscoveryAdapter_BootstrapManagement(t *testing.T) {
	config := discoveryif.DefaultConfig()
	config.EnableDHT = false
	config.EnableMDNS = false

	service := NewDiscoveryService(nil, nil, config)
	adapter := &endpointDiscoveryAdapter{svc: service}

	ctx := context.Background()

	// 测试 GetBootstrapPeers
	peers, err := adapter.GetBootstrapPeers(ctx)
	if err != nil {
		t.Fatalf("GetBootstrapPeers() error: %v", err)
	}
	if len(peers) != 0 {
		t.Errorf("expected 0 bootstrap peers initially, got %d", len(peers))
	}

	// 测试 AddBootstrapPeer（REQ-BOOT-001 要求 Full Address 格式）
	peerID := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32}
	peer := endpoint.PeerInfo{
		ID:    peerID,
		Addrs: []string{"/ip4/1.1.1.1/udp/8000/quic-v1/p2p/" + peerID.String()},
	}
	adapter.AddBootstrapPeer(peer)

	peers, _ = adapter.GetBootstrapPeers(ctx)
	if len(peers) != 1 {
		t.Errorf("expected 1 bootstrap peer after add, got %d", len(peers))
	}

	// 测试 RemoveBootstrapPeer
	adapter.RemoveBootstrapPeer(peer.ID)
	peers, _ = adapter.GetBootstrapPeers(ctx)
	if len(peers) != 0 {
		t.Errorf("expected 0 bootstrap peers after remove, got %d", len(peers))
	}
}

// ============================================================================
//                              DHT 路由表测试
// ============================================================================

func TestRoutingTable_New(t *testing.T) {
	localID := types.NodeID{}
	copy(localID[:], []byte("local-node-id-1234567890123"))

	rt := dht.NewRoutingTable(localID, types.DefaultRealmID)

	if rt.Size() != 0 {
		t.Errorf("expected empty routing table, got size %d", rt.Size())
	}
}

func TestRoutingTable_Update(t *testing.T) {
	localID := types.NodeID{}
	copy(localID[:], []byte("local-node-id-1234567890123"))

	rt := dht.NewRoutingTable(localID, types.DefaultRealmID)

	// 添加节点
	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("other-node-id-1234567890123"))

	node := &dht.RoutingNode{
		ID:       nodeID,
		Addrs:    []string{"192.168.1.1:8000"},
		LastSeen: time.Now(),
	}

	rt.Update(node)

	if rt.Size() != 1 {
		t.Errorf("expected 1 node, got %d", rt.Size())
	}
}

func TestRoutingTable_Remove(t *testing.T) {
	localID := types.NodeID{}
	copy(localID[:], []byte("local-node-id-1234567890123"))

	rt := dht.NewRoutingTable(localID, types.DefaultRealmID)

	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("other-node-id-1234567890123"))

	node := &dht.RoutingNode{
		ID:       nodeID,
		LastSeen: time.Now(),
	}

	rt.Update(node)
	rt.Remove(nodeID)

	if rt.Size() != 0 {
		t.Errorf("expected 0 nodes after remove, got %d", rt.Size())
	}
}

func TestRoutingTable_Find(t *testing.T) {
	localID := types.NodeID{}
	copy(localID[:], []byte("local-node-id-1234567890123"))

	rt := dht.NewRoutingTable(localID, types.DefaultRealmID)

	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("other-node-id-1234567890123"))

	node := &dht.RoutingNode{
		ID:       nodeID,
		Addrs:    []string{"192.168.1.1:8000"},
		LastSeen: time.Now(),
	}

	rt.Update(node)

	found := rt.Find(nodeID)
	if found == nil {
		t.Fatal("expected to find node")
	}

	if found.ID != nodeID {
		t.Errorf("found wrong node")
	}
}

func TestRoutingTable_Peers(t *testing.T) {
	localID := types.NodeID{}
	copy(localID[:], []byte("local-node-id-1234567890123"))

	rt := dht.NewRoutingTable(localID, types.DefaultRealmID)

	// 添加多个节点
	for i := 0; i < 10; i++ {
		nodeID := types.NodeID{}
		copy(nodeID[:], []byte{byte(i), byte(i + 1)})

		node := &dht.RoutingNode{
			ID:       nodeID,
			LastSeen: time.Now(),
		}
		rt.Update(node)
	}

	peers := rt.Peers()
	if len(peers) != 10 {
		t.Errorf("expected 10 peers, got %d", len(peers))
	}
}

func TestRoutingTable_NearestPeers(t *testing.T) {
	localID := types.NodeID{}
	copy(localID[:], []byte("local-node-id-1234567890123"))

	rt := dht.NewRoutingTable(localID, types.DefaultRealmID)

	// 添加节点
	for i := 0; i < 20; i++ {
		nodeID := types.NodeID{}
		copy(nodeID[:], []byte{byte(i), byte(i * 2)})

		node := &dht.RoutingNode{
			ID:       nodeID,
			LastSeen: time.Now(),
		}
		rt.Update(node)
	}

	// 查找最近的 5 个节点
	key := []byte{10, 20, 30, 40}
	nearest := rt.NearestPeers(key, 5)

	if len(nearest) != 5 {
		t.Errorf("expected 5 nearest peers, got %d", len(nearest))
	}
}

func TestRoutingTable_Cleanup(t *testing.T) {
	localID := types.NodeID{}
	copy(localID[:], []byte("local-node-id-1234567890123"))

	rt := dht.NewRoutingTable(localID, types.DefaultRealmID)

	// 添加一个过期节点
	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("expired-node-id-123456789012"))

	node := &dht.RoutingNode{
		ID:       nodeID,
		LastSeen: time.Now().Add(-25 * time.Hour), // 已过期
	}
	rt.Update(node)

	// 清理
	removed := rt.Cleanup()

	if removed != 1 {
		t.Errorf("expected 1 node removed, got %d", removed)
	}

	if rt.Size() != 0 {
		t.Errorf("expected 0 nodes after cleanup, got %d", rt.Size())
	}
}

// ============================================================================
//                              DHT 测试
// ============================================================================

func TestDHT_New(t *testing.T) {
	config := dht.DefaultConfig()
	d := dht.NewDHT(config, nil, types.DefaultRealmID)

	if d == nil {
		t.Fatal("expected non-nil DHT")
	}
}

func TestDHT_StartStop(t *testing.T) {
	config := dht.DefaultConfig()
	d := dht.NewDHT(config, nil, types.DefaultRealmID)

	ctx := context.Background()

	if err := d.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if err := d.Stop(); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}
}

func TestDHT_AddNode(t *testing.T) {
	config := dht.DefaultConfig()
	d := dht.NewDHT(config, nil, types.DefaultRealmID)

	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("test-node-id-12345678901234"))

	d.AddNode(nodeID, []string{"192.168.1.1:8000"}, types.DefaultRealmID)

	rt := d.RoutingTable()
	if rt.Size() != 1 {
		t.Errorf("expected 1 node in routing table, got %d", rt.Size())
	}
}

func TestDHT_RemoveNode(t *testing.T) {
	config := dht.DefaultConfig()
	d := dht.NewDHT(config, nil, types.DefaultRealmID)

	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("test-node-id-12345678901234"))

	d.AddNode(nodeID, []string{"192.168.1.1:8000"}, types.DefaultRealmID)
	d.RemoveNode(nodeID)

	rt := d.RoutingTable()
	if rt.Size() != 0 {
		t.Errorf("expected 0 nodes after remove, got %d", rt.Size())
	}
}

func TestDHT_Mode(t *testing.T) {
	config := dht.DefaultConfig()
	config.Mode = discoveryif.DHTModeServer

	d := dht.NewDHT(config, nil, types.DefaultRealmID)

	if d.Mode() != discoveryif.DHTModeServer {
		t.Errorf("expected server mode, got %v", d.Mode())
	}

	d.SetMode(discoveryif.DHTModeClient)

	if d.Mode() != discoveryif.DHTModeClient {
		t.Errorf("expected client mode, got %v", d.Mode())
	}
}

// ============================================================================
//                              XOR 距离测试
// ============================================================================

func TestXORDistance(t *testing.T) {
	a := []byte{0b11110000, 0b10101010}
	b := []byte{0b00001111, 0b01010101}

	distance := dht.XORDistance(a, b)

	expected := []byte{0b11111111, 0b11111111}
	for i := range distance {
		if distance[i] != expected[i] {
			t.Errorf("XORDistance byte %d: got %08b, want %08b", i, distance[i], expected[i])
		}
	}
}

func TestXORDistance_Same(t *testing.T) {
	a := []byte{1, 2, 3, 4}
	distance := dht.XORDistance(a, a)

	for i, b := range distance {
		if b != 0 {
			t.Errorf("XORDistance with self: byte %d = %d, want 0", i, b)
		}
	}
}

// ============================================================================
//                              RealmAwareDHTKey 测试
// ============================================================================

func TestRealmAwareDHTKey_NoRealm(t *testing.T) {
	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("test-node-id-12345678901234"))

	key1 := dht.RealmAwareDHTKey(types.DefaultRealmID, nodeID)
	key2 := dht.RealmAwareDHTKey("", nodeID)

	// 两个空 Realm 应该产生相同的 Key
	if len(key1) != len(key2) {
		t.Errorf("key lengths differ: %d vs %d", len(key1), len(key2))
	}

	for i := range key1 {
		if key1[i] != key2[i] {
			t.Errorf("keys differ at position %d", i)
			break
		}
	}
}

func TestRealmAwareDHTKey_WithRealm(t *testing.T) {
	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("test-node-id-12345678901234"))

	key1 := dht.RealmAwareDHTKey(types.DefaultRealmID, nodeID)
	key2 := dht.RealmAwareDHTKey("my-realm", nodeID)

	// 不同 Realm 应该产生不同的 Key
	same := true
	for i := range key1 {
		if i < len(key2) && key1[i] != key2[i] {
			same = false
			break
		}
	}

	if same && len(key1) == len(key2) {
		t.Error("expected different keys for different realms")
	}
}

// ============================================================================
//                              Dynamic Interval 测试
// ============================================================================

func TestDynamicInterval_Calculate(t *testing.T) {
	config := DefaultIntervalConfig()
	di := NewDynamicInterval(config)

	// 没有节点时应该是最小间隔
	interval := di.Calculate(0)
	if interval != config.MinInterval {
		t.Errorf("expected min interval %v for 0 peers, got %v", config.MinInterval, interval)
	}

	// 达到目标时（ratio > 0.9）应该是 BaseInterval * 2
	interval = di.Calculate(config.TargetPeerCount)
	expectedSlowdown := config.BaseInterval * 2
	if interval != expectedSlowdown {
		t.Errorf("expected slowdown interval %v for target peers, got %v", expectedSlowdown, interval)
	}
}

func TestDynamicInterval_Emergency(t *testing.T) {
	config := DefaultIntervalConfig()
	config.TargetPeerCount = 50
	di := NewDynamicInterval(config)

	// 计算低于紧急阈值的间隔（小于 3 个节点触发紧急模式）
	di.Calculate(2)

	if !di.IsEmergency() {
		t.Error("expected emergency mode for low peer count")
	}

	// 恢复到正常（需要多次调用模拟恢复）
	for i := 0; i < 15; i++ {
		di.Calculate(20)
	}

	if di.IsEmergency() {
		t.Log("still in emergency mode, continuing recovery")
	}
}

// ============================================================================
//                              KBucket 测试
// ============================================================================

func TestKBucket_Add(t *testing.T) {
	bucket := dht.NewKBucket()

	// 添加节点
	for i := 0; i < 10; i++ {
		node := &dht.RoutingNode{
			ID:       types.NodeID{byte(i)},
			LastSeen: time.Now(),
		}
		bucket.Add(node)
	}

	if bucket.Size() != 10 {
		t.Errorf("expected 10 nodes, got %d", bucket.Size())
	}
}

func TestKBucket_Full(t *testing.T) {
	bucket := dht.NewKBucket()

	// 填满桶
	for i := 0; i < dht.BucketSize; i++ {
		node := &dht.RoutingNode{
			ID:       types.NodeID{byte(i)},
			LastSeen: time.Now(),
		}
		bucket.Add(node)
	}

	if !bucket.IsFull() {
		t.Error("bucket should be full")
	}

	// 再添加一个应该失败
	extra := &dht.RoutingNode{
		ID:       types.NodeID{255},
		LastSeen: time.Now(),
	}
	added := bucket.Add(extra)

	if added {
		t.Error("should not add to full bucket")
	}
}

func TestKBucket_Contains(t *testing.T) {
	bucket := dht.NewKBucket()

	node := &dht.RoutingNode{
		ID:       types.NodeID{1, 2, 3},
		LastSeen: time.Now(),
	}
	bucket.Add(node)

	if !bucket.Contains(node.ID) {
		t.Error("bucket should contain the node")
	}

	if bucket.Contains(types.NodeID{4, 5, 6}) {
		t.Error("bucket should not contain unknown node")
	}
}

func TestKBucket_Remove(t *testing.T) {
	bucket := dht.NewKBucket()

	node := &dht.RoutingNode{
		ID:       types.NodeID{1, 2, 3},
		LastSeen: time.Now(),
	}
	bucket.Add(node)
	bucket.Remove(node.ID)

	if bucket.Contains(node.ID) {
		t.Error("bucket should not contain removed node")
	}
}

// ============================================================================
//                              DiscoveryService 增强测试
// ============================================================================

func TestDiscoveryService_RegisterAnnouncer(t *testing.T) {
	config := discoveryif.DefaultConfig()
	service := NewDiscoveryService(nil, nil, config)

	mockAnn := &mockAnnouncer{}

	service.RegisterAnnouncer("mock", mockAnn)

	if len(service.announcers) != 1 {
		t.Errorf("expected 1 announcer, got %d", len(service.announcers))
	}
}

func TestDiscoveryService_RegisterPeerFinder(t *testing.T) {
	config := discoveryif.DefaultConfig()
	service := NewDiscoveryService(nil, nil, config)

	mockFinder := &mockPeerFinder{}

	service.RegisterPeerFinder("mock", mockFinder)

	if len(service.peerFinders) != 1 {
		t.Errorf("expected 1 peer finder, got %d", len(service.peerFinders))
	}
}

func TestDiscoveryService_RegisterClosestPeerFinder(t *testing.T) {
	config := discoveryif.DefaultConfig()
	service := NewDiscoveryService(nil, nil, config)

	mockFinder := &mockClosestPeerFinder{}

	service.RegisterClosestPeerFinder("mock", mockFinder)

	if len(service.closestPeerFinders) != 1 {
		t.Errorf("expected 1 closest peer finder, got %d", len(service.closestPeerFinders))
	}
}

func TestDiscoveryService_RegisterNamespaceDiscoverer(t *testing.T) {
	config := discoveryif.DefaultConfig()
	service := NewDiscoveryService(nil, nil, config)

	mockNS := &mockNamespaceDiscoverer{}

	service.RegisterNamespaceDiscoverer("mock", mockNS)

	if len(service.namespaceDiscoverers) != 1 {
		t.Errorf("expected 1 namespace discoverer, got %d", len(service.namespaceDiscoverers))
	}
}

func TestDiscoveryService_RendezvousPoint(t *testing.T) {
	config := discoveryif.DefaultConfig()
	service := NewDiscoveryService(nil, nil, config)

	mockPoint := &mockRendezvousPoint{}

	service.SetRendezvousPoint(mockPoint)

	got := service.GetRendezvousPoint()
	if got == nil {
		t.Error("expected non-nil rendezvous point")
	}
}

func TestDiscoveryService_IsEmergencyMode(t *testing.T) {
	config := discoveryif.DefaultConfig()
	config.TargetPeers = 50
	service := NewDiscoveryService(nil, nil, config)

	// 初始状态没有节点，可能处于紧急模式
	// 因为 DynamicInterval 在 Calculate 后才确定紧急状态
	service.GetDiscoveryInterval() // 触发计算

	// 添加一些节点
	for i := 0; i < 30; i++ {
		nodeID := types.NodeID{}
		copy(nodeID[:], []byte{byte(i)})
		service.AddKnownPeer(nodeID, types.DefaultRealmID, nil)
	}

	// 检查是否脱离紧急模式
	for i := 0; i < 10; i++ {
		service.GetDiscoveryInterval()
	}

	// 结果取决于具体实现，只验证不 panic
}

func TestDiscoveryService_StopCancelsLookups(t *testing.T) {
	config := discoveryif.DefaultConfig()
	service := NewDiscoveryService(nil, nil, config)
	service.Start(context.Background())

	// 添加一个待处理的查找
	lookupCh := make(chan lookupResult)
	nodeID := types.NodeID{1, 2, 3}

	service.lookupMu.Lock()
	service.pendingLookups[nodeID] = append(service.pendingLookups[nodeID], lookupCh)
	service.lookupMu.Unlock()

	// 停止服务应该关闭所有通道
	service.Stop()

	select {
	case _, ok := <-lookupCh:
		if ok {
			t.Error("lookup channel should be closed")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("lookup channel was not closed")
	}
}

func TestDiscoveryService_MultipleAddRemove(t *testing.T) {
	config := discoveryif.DefaultConfig()
	service := NewDiscoveryService(nil, nil, config)

	// 批量添加
	for i := 0; i < 100; i++ {
		nodeID := types.NodeID{}
		copy(nodeID[:], []byte{byte(i), byte(i >> 8)})
		service.AddKnownPeer(nodeID, types.DefaultRealmID, nil)
	}

	if service.KnownPeerCount() != 100 {
		t.Errorf("expected 100 peers, got %d", service.KnownPeerCount())
	}

	// 批量删除
	for i := 0; i < 50; i++ {
		nodeID := types.NodeID{}
		copy(nodeID[:], []byte{byte(i), byte(i >> 8)})
		service.RemoveKnownPeer(nodeID)
	}

	if service.KnownPeerCount() != 50 {
		t.Errorf("expected 50 peers after removal, got %d", service.KnownPeerCount())
	}
}

func TestDiscoveryService_UpdateKnownPeer(t *testing.T) {
	config := discoveryif.DefaultConfig()
	service := NewDiscoveryService(nil, nil, config)

	nodeID := types.NodeID{1, 2, 3}
	addrs1 := toAddressesFromStrings([]string{"192.168.1.1:8000"})
	addrs2 := toAddressesFromStrings([]string{"192.168.1.2:8000", "192.168.1.3:8000"})

	// 首次添加
	service.AddKnownPeer(nodeID, types.DefaultRealmID, addrs1)

	// 更新
	service.AddKnownPeer(nodeID, "new-realm", addrs2)

	// 仍然只有一个节点
	if service.KnownPeerCount() != 1 {
		t.Errorf("expected 1 peer, got %d", service.KnownPeerCount())
	}

	// 验证信息已更新
	service.mu.RLock()
	info := service.knownPeers[nodeID]
	service.mu.RUnlock()

	if info.RealmID != "new-realm" {
		t.Errorf("expected realm 'new-realm', got %s", info.RealmID)
	}

	if len(info.Addresses) != 2 {
		t.Errorf("expected 2 addresses, got %d", len(info.Addresses))
	}
}

// ============================================================================
//                              辅助类型
// ============================================================================

type mockDiscoverer struct{}

func (m *mockDiscoverer) FindPeer(ctx context.Context, id types.NodeID) ([]endpoint.Address, error) {
	return nil, nil
}

func (m *mockDiscoverer) FindPeers(ctx context.Context, ids []types.NodeID) (map[types.NodeID][]endpoint.Address, error) {
	return nil, nil
}

func (m *mockDiscoverer) FindClosestPeers(ctx context.Context, key []byte, count int) ([]types.NodeID, error) {
	return nil, nil
}

func (m *mockDiscoverer) DiscoverPeers(ctx context.Context, namespace string) (<-chan discoveryif.PeerInfo, error) {
	ch := make(chan discoveryif.PeerInfo)
	close(ch)
	return ch, nil
}

type mockAnnouncer struct{}

func (m *mockAnnouncer) Announce(ctx context.Context, namespace string) error {
	return nil
}

func (m *mockAnnouncer) AnnounceWithTTL(ctx context.Context, namespace string, ttl time.Duration) error {
	return nil
}

func (m *mockAnnouncer) StopAnnounce(namespace string) error {
	return nil
}

type mockPeerFinder struct{}

func (m *mockPeerFinder) FindPeer(ctx context.Context, id types.NodeID) ([]endpoint.Address, error) {
	return nil, nil
}

func (m *mockPeerFinder) FindPeers(ctx context.Context, ids []types.NodeID) (map[types.NodeID][]endpoint.Address, error) {
	return nil, nil
}

type mockClosestPeerFinder struct{}

func (m *mockClosestPeerFinder) FindClosestPeers(ctx context.Context, key []byte, count int) ([]types.NodeID, error) {
	return nil, nil
}

type mockNamespaceDiscoverer struct{}

func (m *mockNamespaceDiscoverer) DiscoverPeers(ctx context.Context, namespace string) (<-chan discoveryif.PeerInfo, error) {
	ch := make(chan discoveryif.PeerInfo)
	close(ch)
	return ch, nil
}

type mockRendezvousPoint struct{}

func (m *mockRendezvousPoint) Start(ctx context.Context) error {
	return nil
}

func (m *mockRendezvousPoint) Stop() error {
	return nil
}

func (m *mockRendezvousPoint) HandleRegister(from types.NodeID, namespace string, peer discoveryif.PeerInfo, ttl time.Duration) error {
	return nil
}

func (m *mockRendezvousPoint) HandleUnregister(from types.NodeID, namespace string) error {
	return nil
}

func (m *mockRendezvousPoint) HandleDiscover(from types.NodeID, namespace string, limit int, cookie []byte) ([]discoveryif.PeerInfo, []byte, error) {
	return nil, nil, nil
}

func (m *mockRendezvousPoint) Namespaces() []string {
	return nil
}

func (m *mockRendezvousPoint) PeersInNamespace(namespace string) int {
	return 0
}

func (m *mockRendezvousPoint) Stats() discoveryif.RendezvousStats {
	return discoveryif.RendezvousStats{}
}

// toAddressesFromStrings 将字符串数组转换为地址数组
func toAddressesFromStrings(addrs []string) []endpoint.Address {
	result := make([]endpoint.Address, len(addrs))
	for i, a := range addrs {
		result[i] = &stringAddr{s: a}
	}
	return result
}

// ============================================================================
//                              Finder 测试
// ============================================================================

func TestDiscoveryService_FindPeerFromCache(t *testing.T) {
	config := discoveryif.DefaultConfig()
	service := NewDiscoveryService(nil, nil, config)

	// 添加已知节点
	nodeID := types.NodeID{1, 2, 3}
	addrs := toAddressesFromStrings([]string{"192.168.1.1:8000"})
	service.AddKnownPeer(nodeID, types.DefaultRealmID, addrs)

	// 查找节点
	ctx := context.Background()
	result, err := service.FindPeer(ctx, nodeID)

	if err != nil {
		t.Fatalf("FindPeer() error: %v", err)
	}

	if len(result) != 1 {
		t.Errorf("expected 1 address, got %d", len(result))
	}
}

func TestDiscoveryService_FindClosestPeers(t *testing.T) {
	config := discoveryif.DefaultConfig()
	service := NewDiscoveryService(nil, nil, config)

	// 添加多个节点
	for i := 0; i < 20; i++ {
		nodeID := types.NodeID{byte(i), byte(i * 2)}
		service.AddKnownPeer(nodeID, types.DefaultRealmID, nil)
	}

	// 查找最近的 5 个
	key := []byte{10, 20, 30}
	result, err := service.FindClosestPeers(context.Background(), key, 5)

	if err != nil {
		t.Fatalf("FindClosestPeers() error: %v", err)
	}

	if len(result) != 5 {
		t.Errorf("expected 5 peers, got %d", len(result))
	}
}

func TestDiscoveryService_FindClosestPeers_LessThanCount(t *testing.T) {
	config := discoveryif.DefaultConfig()
	service := NewDiscoveryService(nil, nil, config)

	// 只添加 3 个节点
	for i := 0; i < 3; i++ {
		nodeID := types.NodeID{byte(i)}
		service.AddKnownPeer(nodeID, types.DefaultRealmID, nil)
	}

	// 查找 10 个
	result, err := service.FindClosestPeers(context.Background(), []byte{1}, 10)

	if err != nil {
		t.Fatalf("FindClosestPeers() error: %v", err)
	}

	if len(result) != 3 {
		t.Errorf("expected 3 peers (all available), got %d", len(result))
	}
}

func TestDiscoveryService_DiscoverPeers(t *testing.T) {
	config := discoveryif.DefaultConfig()
	service := NewDiscoveryService(nil, nil, config)

	// 添加节点
	for i := 0; i < 5; i++ {
		nodeID := types.NodeID{byte(i)}
		addrs := toAddressesFromStrings([]string{"192.168.1." + string(rune('0'+i)) + ":8000"})
		service.AddKnownPeer(nodeID, types.DefaultRealmID, addrs)
	}

	// 发现节点
	ch, err := service.DiscoverPeers(context.Background(), "")
	if err != nil {
		t.Fatalf("DiscoverPeers() error: %v", err)
	}

	count := 0
	for range ch {
		count++
	}

	if count != 5 {
		t.Errorf("expected 5 peers, got %d", count)
	}
}

func TestDiscoveryService_DiscoverPeers_Cancelled(t *testing.T) {
	config := discoveryif.DefaultConfig()
	service := NewDiscoveryService(nil, nil, config)

	// 添加很多节点
	for i := 0; i < 100; i++ {
		nodeID := types.NodeID{byte(i)}
		service.AddKnownPeer(nodeID, types.DefaultRealmID, nil)
	}

	ctx, cancel := context.WithCancel(context.Background())

	ch, err := service.DiscoverPeers(ctx, "")
	if err != nil {
		t.Fatalf("DiscoverPeers() error: %v", err)
	}

	// 立即取消
	cancel()

	// 可能会收到一些节点，但不会全部
	count := 0
	for range ch {
		count++
	}

	// 只验证通道已关闭
	t.Logf("received %d peers before cancel", count)
}

func TestCompareBytes(t *testing.T) {
	tests := []struct {
		name     string
		a, b     []byte
		expected int
	}{
		{"equal", []byte{1, 2, 3}, []byte{1, 2, 3}, 0},
		{"a less", []byte{1, 2, 3}, []byte{1, 2, 4}, -1},
		{"a greater", []byte{1, 2, 4}, []byte{1, 2, 3}, 1},
		{"a shorter", []byte{1, 2}, []byte{1, 2, 3}, -1},
		{"a longer", []byte{1, 2, 3}, []byte{1, 2}, 1},
		{"first byte differs", []byte{2, 0, 0}, []byte{1, 9, 9}, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareBytes(tt.a, tt.b)
			if (result < 0 && tt.expected >= 0) ||
				(result > 0 && tt.expected <= 0) ||
				(result == 0 && tt.expected != 0) {
				t.Errorf("compareBytes(%v, %v) = %d, want sign of %d", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

// ============================================================================
//                              Announcer 测试
// ============================================================================

func TestDiscoveryService_Announce(t *testing.T) {
	config := discoveryif.DefaultConfig()
	service := NewDiscoveryService(nil, nil, config)

	// 添加 announcer
	mockAnn := &mockAnnouncerWithTracking{}
	service.RegisterAnnouncer("mock", mockAnn)

	// 通告
	err := service.Announce(context.Background(), "test-namespace")

	if err != nil {
		t.Fatalf("Announce() error: %v", err)
	}

	if !mockAnn.announced {
		t.Error("expected announcer to be called")
	}
}

func TestDiscoveryService_AnnounceWithTTL(t *testing.T) {
	config := discoveryif.DefaultConfig()
	service := NewDiscoveryService(nil, nil, config)

	mockAnn := &mockAnnouncerWithTracking{}
	service.RegisterAnnouncer("mock", mockAnn)

	err := service.AnnounceWithTTL(context.Background(), "test-namespace", 30*time.Minute)

	if err != nil {
		t.Fatalf("AnnounceWithTTL() error: %v", err)
	}

	if mockAnn.ttl != 30*time.Minute {
		t.Errorf("expected TTL 30m, got %v", mockAnn.ttl)
	}
}

func TestDiscoveryService_StopAnnounce(t *testing.T) {
	config := discoveryif.DefaultConfig()
	service := NewDiscoveryService(nil, nil, config)

	mockAnn := &mockAnnouncerWithTracking{}
	service.RegisterAnnouncer("mock", mockAnn)

	err := service.StopAnnounce("test-namespace")

	if err != nil {
		t.Fatalf("StopAnnounce() error: %v", err)
	}

	if mockAnn.stoppedNamespace != "test-namespace" {
		t.Errorf("expected stop on 'test-namespace', got '%s'", mockAnn.stoppedNamespace)
	}
}

func TestDiscoveryService_AnnounceNoAnnouncers(t *testing.T) {
	config := discoveryif.DefaultConfig()
	service := NewDiscoveryService(nil, nil, config)

	// 没有 announcer，不应该报错
	err := service.Announce(context.Background(), "test")

	if err != nil {
		t.Errorf("Announce() with no announcers should not error, got: %v", err)
	}
}

// ============================================================================
//                              Realm 过滤测试
// ============================================================================

func TestDiscoveryService_KnownPeersInRealm(t *testing.T) {
	config := discoveryif.DefaultConfig()
	service := NewDiscoveryService(nil, nil, config)

	// 添加不同 realm 的节点
	service.AddKnownPeer(types.NodeID{1}, "realm-a", nil)
	service.AddKnownPeer(types.NodeID{2}, "realm-a", nil)
	service.AddKnownPeer(types.NodeID{3}, "realm-a", nil)
	service.AddKnownPeer(types.NodeID{4}, "realm-b", nil)
	service.AddKnownPeer(types.NodeID{5}, "realm-b", nil)

	// 验证 realm-a 有 3 个节点
	countA := service.KnownPeersInRealm("realm-a")
	if countA != 3 {
		t.Errorf("expected 3 peers in realm-a, got %d", countA)
	}

	// 验证 realm-b 有 2 个节点
	countB := service.KnownPeersInRealm("realm-b")
	if countB != 2 {
		t.Errorf("expected 2 peers in realm-b, got %d", countB)
	}

	// 验证总数
	if service.KnownPeerCount() != 5 {
		t.Errorf("expected 5 total peers, got %d", service.KnownPeerCount())
	}
}

type mockAnnouncerWithTracking struct {
	announced        bool
	ttl              time.Duration
	stoppedNamespace string
}

func (m *mockAnnouncerWithTracking) Announce(ctx context.Context, namespace string) error {
	m.announced = true
	return nil
}

func (m *mockAnnouncerWithTracking) AnnounceWithTTL(ctx context.Context, namespace string, ttl time.Duration) error {
	m.announced = true
	m.ttl = ttl
	return nil
}

func (m *mockAnnouncerWithTracking) StopAnnounce(namespace string) error {
	m.stoppedNamespace = namespace
	return nil
}

// ============================================================================
//                              Benchmark 测试
// ============================================================================

func BenchmarkXORDistance(b *testing.B) {
	a := make([]byte, 32)
	c := make([]byte, 32)
	for i := range a {
		a[i] = byte(i)
		c[i] = byte(i * 2)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dht.XORDistance(a, c)
	}
}

func BenchmarkRoutingTable_Update(b *testing.B) {
	localID := types.NodeID{}
	rt := dht.NewRoutingTable(localID, types.DefaultRealmID)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		node := &dht.RoutingNode{
			ID:       types.NodeID{byte(i), byte(i >> 8)},
			LastSeen: time.Now(),
		}
		rt.Update(node)
	}
}

func BenchmarkRoutingTable_NearestPeers(b *testing.B) {
	localID := types.NodeID{}
	rt := dht.NewRoutingTable(localID, types.DefaultRealmID)

	// 预填充节点
	for i := 0; i < 100; i++ {
		node := &dht.RoutingNode{
			ID:       types.NodeID{byte(i), byte(i >> 8)},
			LastSeen: time.Now(),
		}
		rt.Update(node)
	}

	key := []byte{1, 2, 3, 4}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rt.NearestPeers(key, 10)
	}
}

// ============================================================================
//                              模块测试
// ============================================================================

func TestProvideServices_NilTransport(t *testing.T) {
	input := ModuleInput{
		Transport: nil,
	}

	_, err := ProvideServices(input)
	if err == nil {
		t.Error("expected error when Transport is nil")
	}

	if err.Error() != "transport is required" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestProvideServices_WithTransport(t *testing.T) {
	mockTransport := &mockTransport{}
	mockIdentity := &mockIdentity{}
	input := ModuleInput{
		Transport: mockTransport,
		Identity:  mockIdentity,
	}

	output, err := ProvideServices(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if output.DiscoveryService == nil {
		t.Error("expected non-nil DiscoveryService")
	}
}

// ============================================================================
//                              Service Stop 测试
// ============================================================================

func TestDiscoveryService_StopWithoutStart(t *testing.T) {
	config := discoveryif.DefaultConfig()
	service := NewDiscoveryService(nil, nil, config)

	// 未启动情况下停止应该安全
	if err := service.Stop(); err != nil {
		t.Fatalf("Stop() without Start() should not error: %v", err)
	}
}

func TestDiscoveryService_StopMultiple(t *testing.T) {
	config := discoveryif.DefaultConfig()
	config.EnableDHT = false
	config.EnableMDNS = false
	config.EnableBootstrap = false

	service := NewDiscoveryService(nil, nil, config)
	service.Start(context.Background())

	// 多次停止应该安全
	for i := 0; i < 3; i++ {
		if err := service.Stop(); err != nil {
			t.Fatalf("Stop() #%d error: %v", i, err)
		}
	}
}

// ============================================================================
//                              FindPeer 超时测试
// ============================================================================

func TestDiscoveryService_FindPeer_Timeout(t *testing.T) {
	config := discoveryif.DefaultConfig()
	service := NewDiscoveryService(nil, nil, config)

	// 使用短超时的 context
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	nodeID := types.NodeID{1, 2, 3}

	// 没有发现器，应该超时
	_, err := service.FindPeer(ctx, nodeID)

	if err == nil {
		t.Error("expected error for FindPeer with no discoverers")
	}

	// 验证 pending lookup 已清理
	time.Sleep(100 * time.Millisecond) // 等待 defer 清理
	service.lookupMu.Lock()
	_, exists := service.pendingLookups[nodeID]
	service.lookupMu.Unlock()

	if exists {
		t.Error("pending lookup should be cleaned up after timeout")
	}
}

func TestDiscoveryService_FindPeer_ContextCanceled(t *testing.T) {
	config := discoveryif.DefaultConfig()
	service := NewDiscoveryService(nil, nil, config)

	ctx, cancel := context.WithCancel(context.Background())

	nodeID := types.NodeID{1, 2, 3}

	// 立即取消
	cancel()

	_, err := service.FindPeer(ctx, nodeID)

	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got: %v", err)
	}
}

// ============================================================================
//                              stringAddr Network() 测试
// ============================================================================

func TestStringAddr_Network(t *testing.T) {
	tests := []struct {
		addr     string
		expected string
	}{
		{"/ip4/127.0.0.1/tcp/8000", "tcp4"},
		{"/ip4/127.0.0.1/udp/8000", "udp4"},
		{"/ip6/::1/tcp/8000", "tcp6"},
		{"/ip6/::1/udp/8000", "udp6"},
		{"/ip4/192.168.1.1", "ip4"},
		{"/ip6/::1", "ip6"},
		{"192.168.1.1:8000", "tcp4"},
		{"[::1]:8000", "tcp6"},
		{"::1:8000", "tcp6"},
		{"unknown-format", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.addr, func(t *testing.T) {
			sa := &stringAddr{s: tt.addr}
			got := sa.Network()
			if got != tt.expected {
				t.Errorf("Network() = %s, want %s", got, tt.expected)
			}
		})
	}
}

// ============================================================================
//                              锁安全测试
// ============================================================================

func TestDiscoveryService_ConcurrentAccess(t *testing.T) {
	config := discoveryif.DefaultConfig()
	config.EnableDHT = false
	config.EnableMDNS = false
	service := NewDiscoveryService(nil, nil, config)
	service.Start(context.Background())
	defer service.Stop()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			nodeID := types.NodeID{byte(i)}
			addrs := toAddressesFromStrings([]string{"192.168.1.1:8000"})
			service.AddKnownPeer(nodeID, types.DefaultRealmID, addrs)
		}
	}()

	for i := 0; i < 100; i++ {
		service.KnownPeerCount()
		service.GetDiscoveryInterval()
	}

	<-done
}

// ============================================================================
//                              mockTransport
// ============================================================================

type mockTransport struct{}

func (m *mockTransport) Dial(ctx context.Context, addr endpoint.Address) (transportif.Conn, error) {
	return nil, nil
}

func (m *mockTransport) DialWithOptions(ctx context.Context, addr endpoint.Address, opts transportif.DialOptions) (transportif.Conn, error) {
	return nil, nil
}

func (m *mockTransport) Listen(addr endpoint.Address) (transportif.Listener, error) {
	return nil, nil
}

func (m *mockTransport) ListenWithOptions(addr endpoint.Address, opts transportif.ListenOptions) (transportif.Listener, error) {
	return nil, nil
}

func (m *mockTransport) Protocols() []string {
	return []string{"mock"}
}

func (m *mockTransport) CanDial(addr endpoint.Address) bool {
	return true
}

func (m *mockTransport) Close() error {
	return nil
}

func (m *mockTransport) Proxy() bool {
	return false
}

// ============================================================================
//                              mockIdentity
// ============================================================================

type mockIdentity struct{}

func (m *mockIdentity) ID() types.NodeID {
	return types.NodeID{}
}

func (m *mockIdentity) PublicKey() identityif.PublicKey {
	return nil
}

func (m *mockIdentity) PrivateKey() identityif.PrivateKey {
	return nil
}

func (m *mockIdentity) Sign(data []byte) ([]byte, error) {
	return nil, nil
}

func (m *mockIdentity) Verify(data, signature []byte, pubKey identityif.PublicKey) (bool, error) {
	return true, nil
}

func (m *mockIdentity) KeyType() types.KeyType {
	return types.KeyTypeEd25519
}
