package dht

import (
	"context"
	"sync"
	"testing"
	"time"

	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Mock Network
// ============================================================================

// mockNetwork 模拟网络
type mockNetwork struct {
	localID    types.NodeID
	localAddrs []string
	stores     map[string]mockStoredValue

	mu            sync.Mutex
	addProviderTTL []time.Duration
	removeKeys     []string
	removeCh       chan string
}

type mockStoredValue struct {
	value []byte
	ttl   time.Duration
}

func newMockNetwork() *mockNetwork {
	id := types.NodeID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
	return &mockNetwork{
		localID:    id,
		localAddrs: []string{"/ip4/127.0.0.1/tcp/8000"},
		stores:     make(map[string]mockStoredValue),
		removeCh:   make(chan string, 10),
	}
}

func (m *mockNetwork) SendFindNode(ctx context.Context, to types.NodeID, target types.NodeID) ([]discoveryif.PeerInfo, error) {
	return nil, nil
}

func (m *mockNetwork) SendFindValue(ctx context.Context, to types.NodeID, key string) ([]byte, []discoveryif.PeerInfo, error) {
	return nil, nil, nil
}

func (m *mockNetwork) SendStore(ctx context.Context, to types.NodeID, key string, value []byte, ttl time.Duration) error {
	m.stores[key] = mockStoredValue{value: value, ttl: ttl}
	return nil
}

func (m *mockNetwork) SendPing(ctx context.Context, to types.NodeID) (time.Duration, error) {
	return 10 * time.Millisecond, nil
}

func (m *mockNetwork) SendAddProvider(ctx context.Context, to types.NodeID, key string, ttl time.Duration) error {
	m.mu.Lock()
	m.addProviderTTL = append(m.addProviderTTL, ttl)
	m.mu.Unlock()
	return nil
}

func (m *mockNetwork) SendGetProviders(ctx context.Context, to types.NodeID, key string) ([]ProviderInfo, []types.NodeID, error) {
	return nil, nil, nil
}

func (m *mockNetwork) SendRemoveProvider(ctx context.Context, to types.NodeID, key string) error {
	m.mu.Lock()
	m.removeKeys = append(m.removeKeys, key)
	m.mu.Unlock()
	select {
	case m.removeCh <- key:
	default:
	}
	return nil
}

func (m *mockNetwork) LocalID() types.NodeID {
	return m.localID
}

func (m *mockNetwork) LocalAddrs() []string {
	return m.localAddrs
}

func (m *mockNetwork) UpdateLocalAddrs(addrs []string) {
	m.localAddrs = addrs
}

// ============================================================================
//                              DHT 测试
// ============================================================================

func TestDHT_PutValue(t *testing.T) {
	network := newMockNetwork()
	config := DefaultConfig()
	dht := NewDHT(config, network, types.RealmID("test"))

	ctx := context.Background()
	dht.Start(ctx)
	defer dht.Stop()

	// 存储值
	key := "test-key"
	value := []byte("test-value")

	err := dht.PutValue(ctx, key, value)
	if err != nil {
		t.Fatalf("PutValue failed: %v", err)
	}

	// 获取值
	got, err := dht.GetValue(ctx, key)
	if err != nil {
		t.Fatalf("GetValue failed: %v", err)
	}

	if string(got) != string(value) {
		t.Errorf("GetValue = %s, want %s", string(got), string(value))
	}
}

func TestDHT_PutValueWithTTL(t *testing.T) {
	network := newMockNetwork()
	config := DefaultConfig()
	dht := NewDHT(config, network, types.RealmID("test"))

	ctx := context.Background()
	dht.Start(ctx)
	defer dht.Stop()

	// 使用自定义 TTL 存储值
	key := "ttl-key"
	value := []byte("ttl-value")
	ttl := 1 * time.Hour

	err := dht.PutValueWithTTL(ctx, key, value, ttl)
	if err != nil {
		t.Fatalf("PutValueWithTTL failed: %v", err)
	}

	// 检查本地存储的 TTL（T2: 使用哈希后的 key）
	hashedKey := HashKeyString(key)
	dht.storeMu.RLock()
	stored, ok := dht.store[hashedKey]
	dht.storeMu.RUnlock()

	if !ok {
		t.Fatal("Value not stored")
	}

	if stored.TTL != ttl {
		t.Errorf("TTL = %v, want %v", stored.TTL, ttl)
	}
}

func TestDHT_PutValueWithTTL_Expiry(t *testing.T) {
	network := newMockNetwork()
	config := DefaultConfig()
	dht := NewDHT(config, network, types.RealmID("test"))

	ctx := context.Background()
	dht.Start(ctx)
	defer dht.Stop()

	// 存储一个很短 TTL 的值
	key := "expiry-key"
	value := []byte("expiry-value")
	ttl := 100 * time.Millisecond

	err := dht.PutValueWithTTL(ctx, key, value, ttl)
	if err != nil {
		t.Fatalf("PutValueWithTTL failed: %v", err)
	}

	// 立即获取应该成功
	got, err := dht.GetValue(ctx, key)
	if err != nil {
		t.Fatalf("GetValue failed immediately: %v", err)
	}
	if string(got) != string(value) {
		t.Errorf("GetValue = %s, want %s", string(got), string(value))
	}

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	// 过期后获取应该失败
	got, err = dht.GetValue(ctx, key)
	if err != ErrKeyNotFound {
		t.Errorf("GetValue after expiry: err = %v, want ErrKeyNotFound", err)
	}
	if got != nil {
		t.Errorf("GetValue after expiry: got = %v, want nil", got)
	}
}

func TestDHT_PutValueWithTTL_ZeroTTL(t *testing.T) {
	network := newMockNetwork()
	config := DefaultConfig()
	dht := NewDHT(config, network, types.RealmID("test"))

	ctx := context.Background()
	dht.Start(ctx)
	defer dht.Stop()

	// 使用 0 TTL 应该使用默认值
	key := "zero-ttl-key"
	value := []byte("zero-ttl-value")

	err := dht.PutValueWithTTL(ctx, key, value, 0)
	if err != nil {
		t.Fatalf("PutValueWithTTL failed: %v", err)
	}

	// 检查使用了默认 TTL（T2: 使用哈希后的 key）
	hashedKey := HashKeyString(key)
	dht.storeMu.RLock()
	stored := dht.store[hashedKey]
	dht.storeMu.RUnlock()

	if stored.TTL != config.MaxRecordAge {
		t.Errorf("TTL = %v, want default %v", stored.TTL, config.MaxRecordAge)
	}
}

func TestDHT_CleanupStore(t *testing.T) {
	network := newMockNetwork()
	config := DefaultConfig()
	dht := NewDHT(config, network, types.RealmID("test"))

	ctx := context.Background()
	dht.Start(ctx)
	defer dht.Stop()

	// 存储一个已过期的值
	dht.storeMu.Lock()
	dht.store["expired-key"] = storedValue{
		Value:     []byte("expired"),
		Provider:  network.localID,
		Timestamp: time.Now().Add(-2 * time.Hour), // 2 小时前
		TTL:       1 * time.Hour,                  // 1 小时 TTL
	}
	dht.store["valid-key"] = storedValue{
		Value:     []byte("valid"),
		Provider:  network.localID,
		Timestamp: time.Now(),
		TTL:       24 * time.Hour,
	}
	dht.storeMu.Unlock()

	// 执行清理
	dht.cleanupStore()

	// 检查过期的被删除
	dht.storeMu.RLock()
	_, expiredExists := dht.store["expired-key"]
	_, validExists := dht.store["valid-key"]
	dht.storeMu.RUnlock()

	if expiredExists {
		t.Error("Expired key should have been cleaned up")
	}
	if !validExists {
		t.Error("Valid key should still exist")
	}
}

func TestProviderTTL_LocalExpire(t *testing.T) {
	network := newMockNetwork()
	config := DefaultConfig()
	dht := NewDHT(config, network, types.RealmID("test"))

	ctx := context.Background()
	_ = dht.Start(ctx)
	defer dht.Stop()

	ttl := 50 * time.Millisecond
	if err := dht.AnnounceWithTTL(ctx, "relay", ttl); err != nil {
		t.Fatalf("AnnounceWithTTL failed: %v", err)
	}

	key := ProviderKeyPrefix + "relay"
	hashedKey := HashKeyString(key)

	// 未过期前应存在
	if got := dht.getProvidersLocal(hashedKey); len(got) == 0 {
		t.Fatalf("expected provider to be present before expiry")
	}

	time.Sleep(80 * time.Millisecond)
	dht.cleanupProviders()

	if got := dht.getProvidersLocal(hashedKey); len(got) != 0 {
		t.Fatalf("expected provider to be expired and removed, got=%d", len(got))
	}
}

func TestStopAnnounce_BestEffortRevoke(t *testing.T) {
	network := newMockNetwork()
	config := DefaultConfig()
	dht := NewDHT(config, network, types.RealmID("test"))

	ctx := context.Background()
	_ = dht.Start(ctx)
	defer dht.Stop()

	// 填充一个可选的“最近节点”，确保会尝试网络发送撤销
	peer := types.NodeID{9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9, 9}
	dht.AddNode(peer, []string{"/ip4/127.0.0.1/tcp/9000"}, types.RealmID("test"))

	// 先通告
	if err := dht.AnnounceWithTTL(ctx, "relay", 10*time.Second); err != nil {
		t.Fatalf("AnnounceWithTTL failed: %v", err)
	}

	// 再停止通告（应触发 best-effort REMOVE_PROVIDER）
	if err := dht.StopAnnounce("relay"); err != nil {
		t.Fatalf("StopAnnounce failed: %v", err)
	}

	select {
	case k := <-network.removeCh:
		if k != ProviderKeyPrefix+"relay" {
			t.Fatalf("unexpected remove key: %s", k)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("expected SendRemoveProvider to be called")
	}
}

func TestProtocol_AddProviderRequest_TTL(t *testing.T) {
	sender := types.NodeID{1}
	req := NewAddProviderRequest(1, sender, []string{"/ip4/127.0.0.1/tcp/1"}, "dep2p/v1/sys/relay", 7)
	if req.TTL != 7 {
		t.Fatalf("TTL=%d, want 7", req.TTL)
	}
}

func TestHandler_GetProvidersResponse_IncludesTTLAndTimestamp(t *testing.T) {
	network := newMockNetwork()
	config := DefaultConfig()
	dht := NewDHT(config, network, types.RealmID("test"))
	_ = dht.Start(context.Background())
	defer dht.Stop()

	key := ProviderKeyPrefix + "relay"
	hashedKey := HashKeyString(key)

	providerID := types.NodeID{7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7, 7}
	ts := time.Now().Add(-2 * time.Second).Truncate(time.Nanosecond)
	ttl := 9 * time.Second
	dht.addProviderLocalWithMeta(hashedKey, providerID, []string{"/ip4/1.2.3.4/tcp/1234"}, ts, ttl)

	h := NewHandler(dht)
	resp := h.handleGetProviders(&Message{
		Type:      MessageTypeGetProviders,
		RequestID: 1,
		Sender:    network.localID,
		Key:       key,
	})

	if !resp.Success {
		t.Fatalf("expected success, err=%s", resp.Error)
	}
	if len(resp.Providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(resp.Providers))
	}
	if resp.Providers[0].TTL != uint32(ttl.Seconds()) {
		t.Fatalf("provider TTL=%d, want %d", resp.Providers[0].TTL, uint32(ttl.Seconds()))
	}
	if resp.Providers[0].Timestamp != ts.UnixNano() {
		t.Fatalf("provider Timestamp=%d, want %d", resp.Providers[0].Timestamp, ts.UnixNano())
	}
}

func TestDHT_Mode(t *testing.T) {
	network := newMockNetwork()
	config := DefaultConfig()
	config.Mode = discoveryif.DHTModeServer
	dht := NewDHT(config, network, types.RealmID("test"))

	if dht.Mode() != discoveryif.DHTModeServer {
		t.Errorf("Mode = %v, want DHTModeServer", dht.Mode())
	}

	dht.SetMode(discoveryif.DHTModeClient)

	if dht.Mode() != discoveryif.DHTModeClient {
		t.Errorf("Mode = %v, want DHTModeClient", dht.Mode())
	}
}

func TestDHT_RoutingTable(t *testing.T) {
	network := newMockNetwork()
	config := DefaultConfig()
	dht := NewDHT(config, network, types.RealmID("test"))

	rt := dht.RoutingTable()
	if rt == nil {
		t.Fatal("RoutingTable should not be nil")
	}

	if rt.Size() != 0 {
		t.Errorf("Initial routing table size = %d, want 0", rt.Size())
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.BucketSize != 20 {
		t.Errorf("BucketSize = %d, want 20", config.BucketSize)
	}

	if config.Alpha != 3 {
		t.Errorf("Alpha = %d, want 3", config.Alpha)
	}

	if config.MaxRecordAge != 24*time.Hour {
		t.Errorf("MaxRecordAge = %v, want 24h", config.MaxRecordAge)
	}

	if !config.EnableValueStore {
		t.Error("EnableValueStore should be true by default")
	}
}

// ============================================================================
//                              DiscoverPeers 测试
// ============================================================================

func TestDHT_DiscoverPeers_WithNamespace(t *testing.T) {
	network := newMockNetwork()
	config := DefaultConfig()
	dht := NewDHT(config, network, types.RealmID("test"))

	ctx := context.Background()
	dht.Start(ctx)
	defer dht.Stop()

	// 添加一些节点到路由表
	for i := 0; i < 5; i++ {
		id := types.NodeID{}
		id[0] = byte(i + 1)
		dht.AddNode(id, []string{"/ip4/127.0.0.1/tcp/8000"}, types.RealmID("test"))
	}

	// 使用 namespace 发现节点
	ch, err := dht.DiscoverPeers(ctx, "test-namespace")
	if err != nil {
		t.Fatalf("DiscoverPeers failed: %v", err)
	}

	// 收集结果
	var peers []discoveryif.PeerInfo
	for peer := range ch {
		peers = append(peers, peer)
	}

	// 应该返回一些节点（基于 namespace 的最近节点）
	if len(peers) == 0 {
		t.Error("DiscoverPeers should return some peers")
	}

	t.Logf("DiscoverPeers with namespace returned %d peers", len(peers))
}

func TestDHT_DiscoverPeers_WithoutNamespace(t *testing.T) {
	network := newMockNetwork()
	config := DefaultConfig()
	dht := NewDHT(config, network, types.RealmID("test"))

	ctx := context.Background()
	dht.Start(ctx)
	defer dht.Stop()

	// 添加一些节点到路由表
	for i := 0; i < 3; i++ {
		id := types.NodeID{}
		id[0] = byte(i + 1)
		dht.AddNode(id, []string{"/ip4/127.0.0.1/tcp/8000"}, types.RealmID("test"))
	}

	// 不使用 namespace 发现节点
	ch, err := dht.DiscoverPeers(ctx, "")
	if err != nil {
		t.Fatalf("DiscoverPeers failed: %v", err)
	}

	// 收集结果
	var peers []discoveryif.PeerInfo
	for peer := range ch {
		peers = append(peers, peer)
	}

	// 应该返回所有节点
	if len(peers) != 3 {
		t.Errorf("DiscoverPeers without namespace: got %d peers, want 3", len(peers))
	}
}

func TestDHT_DiscoverPeers_Closed(t *testing.T) {
	network := newMockNetwork()
	config := DefaultConfig()
	dht := NewDHT(config, network, types.RealmID("test"))

	ctx := context.Background()
	dht.Start(ctx)
	dht.Stop() // 立即停止

	// 尝试发现节点应该失败
	_, err := dht.DiscoverPeers(ctx, "test")
	if err != ErrDHTClosed {
		t.Errorf("DiscoverPeers on closed DHT: got %v, want ErrDHTClosed", err)
	}
}

// ============================================================================
//                              ProviderRecord 解码测试
// ============================================================================

func TestDecodeProviderRecord_Valid(t *testing.T) {
	original := &ProviderRecord{
		Provider:  types.NodeID{1, 2, 3, 4, 5, 6, 7, 8},
		Addrs:     []string{"/ip4/127.0.0.1/tcp/8000", "/ip4/192.168.1.1/tcp/8001"},
		Timestamp: time.Now().Truncate(time.Nanosecond),
		TTL:       1 * time.Hour,
	}

	data := original.Encode()
	decoded, err := DecodeProviderRecord(data)
	if err != nil {
		t.Fatalf("DecodeProviderRecord failed: %v", err)
	}

	if decoded.Provider != original.Provider {
		t.Errorf("Provider mismatch: got %v, want %v", decoded.Provider, original.Provider)
	}

	if len(decoded.Addrs) != len(original.Addrs) {
		t.Errorf("Addrs count mismatch: got %d, want %d", len(decoded.Addrs), len(original.Addrs))
	}

	if decoded.TTL != original.TTL {
		t.Errorf("TTL mismatch: got %v, want %v", decoded.TTL, original.TTL)
	}
}

func TestDecodeProviderRecord_TooShort(t *testing.T) {
	data := make([]byte, 10) // 太短
	_, err := DecodeProviderRecord(data)
	if err == nil {
		t.Error("DecodeProviderRecord should fail on short data")
	}
}

func TestDecodeProviderRecord_TooManyAddrs(t *testing.T) {
	// 构造一个声称有 200 个地址的无效数据
	data := make([]byte, 60)
	// Provider ID
	copy(data[0:32], make([]byte, 32))
	// Addr count = 200 (超过限制)
	data[32] = 0
	data[33] = 200

	_, err := DecodeProviderRecord(data)
	if err == nil {
		t.Error("DecodeProviderRecord should fail on too many addresses")
	}
}

func TestDecodeProviderRecord_TruncatedData(t *testing.T) {
	// 构造一个声称有地址但数据被截断的记录
	data := make([]byte, 60)
	// Provider ID
	copy(data[0:32], make([]byte, 32))
	// Addr count = 1
	data[32] = 0
	data[33] = 1
	// Addr length = 100 (但实际数据不够)
	data[34] = 0
	data[35] = 100

	_, err := DecodeProviderRecord(data)
	if err == nil {
		t.Error("DecodeProviderRecord should fail on truncated data")
	}
}

// stringAddress 测试已迁移到 internal/core/address/addr_test.go
// 所有散落的 Address 实现已统一使用 address.Addr

// ============================================================================
//                              FindPeer 错误处理测试
// ============================================================================

func TestDHT_FindPeer_NotFound(t *testing.T) {
	network := newMockNetwork()
	config := DefaultConfig()
	dht := NewDHT(config, network, types.RealmID("test"))

	ctx := context.Background()
	dht.Start(ctx)
	defer dht.Stop()

	// 查找一个不存在的节点
	unknownID := types.NodeID{99, 99, 99, 99}
	_, err := dht.FindPeer(ctx, unknownID)

	// 应该返回 ErrNoNodes 或 ErrKeyNotFound
	if err == nil {
		t.Error("FindPeer should return error for unknown peer")
	}
}

