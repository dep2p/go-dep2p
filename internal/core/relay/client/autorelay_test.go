package client

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Mock 实现
// ============================================================================

// mockRelayClient 模拟 RelayClient
type mockRelayClient struct {
	reserveFunc    func(ctx context.Context, relayID string) (pkgif.Reservation, error)
	findRelaysFunc func(ctx context.Context) ([]string, error)
	reserveCalls   int32
	findCalls      int32
}

func (m *mockRelayClient) Reserve(ctx context.Context, relayID string) (pkgif.Reservation, error) {
	atomic.AddInt32(&m.reserveCalls, 1)
	if m.reserveFunc != nil {
		return m.reserveFunc(ctx, relayID)
	}
	return &mockReservation{
		relayID: relayID,
		addrs:   []string{"/ip4/1.2.3.4/tcp/1234/p2p/" + relayID},
		expiry:  time.Now().Add(time.Hour).Unix(),
	}, nil
}

func (m *mockRelayClient) FindRelays(ctx context.Context) ([]string, error) {
	atomic.AddInt32(&m.findCalls, 1)
	if m.findRelaysFunc != nil {
		return m.findRelaysFunc(ctx)
	}
	return []string{"relay1", "relay2", "relay3"}, nil
}

// mockReservation 模拟预留
type mockReservation struct {
	relayID      string
	addrs        []string
	expiry       int64
	refreshCount int32
	cancelled    bool
	refreshErr   error
}

func (r *mockReservation) Expiry() int64 {
	return r.expiry
}

func (r *mockReservation) Addrs() []string {
	return r.addrs
}

func (r *mockReservation) Refresh(ctx context.Context) error {
	atomic.AddInt32(&r.refreshCount, 1)
	if r.refreshErr != nil {
		return r.refreshErr
	}
	r.expiry = time.Now().Add(time.Hour).Unix()
	return nil
}

func (r *mockReservation) Cancel() {
	r.cancelled = true
}

// ============================================================================
//                              测试用例 - 配置
// ============================================================================

// TestAutoRelayConfig_Default 测试默认配置
func TestAutoRelayConfig_Default(t *testing.T) {
	config := DefaultAutoRelayConfig()

	if config.MinRelays != DefaultMinRelays {
		t.Errorf("MinRelays = %d, want %d", config.MinRelays, DefaultMinRelays)
	}
	if config.MaxRelays != DefaultMaxRelays {
		t.Errorf("MaxRelays = %d, want %d", config.MaxRelays, DefaultMaxRelays)
	}
	if config.RefreshInterval != DefaultRefreshInterval {
		t.Errorf("RefreshInterval = %v, want %v", config.RefreshInterval, DefaultRefreshInterval)
	}
	if config.DiscoveryInterval != DefaultDiscoveryInterval {
		t.Errorf("DiscoveryInterval = %v, want %v", config.DiscoveryInterval, DefaultDiscoveryInterval)
	}

	t.Log("✅ 默认配置正确")
}

// ============================================================================
//                              测试用例 - AutoRelay 核心功能
// ============================================================================

// TestAutoRelay_New 测试创建 AutoRelay（不需要完整 Host/Peerstore）
func TestAutoRelay_New(t *testing.T) {
	config := DefaultAutoRelayConfig()
	client := &mockRelayClient{}

	// 使用 nil 作为 Host 和 Peerstore（测试不需要它们）
	ar := NewAutoRelay(config, client, nil, nil)

	if ar == nil {
		t.Fatal("NewAutoRelay returned nil")
	}
	if ar.config.MinRelays != config.MinRelays {
		t.Error("config not set correctly")
	}
	if ar.activeRelays == nil {
		t.Error("activeRelays not initialized")
	}
	if ar.candidates == nil {
		t.Error("candidates not initialized")
	}
	if ar.blacklist == nil {
		t.Error("blacklist not initialized")
	}

	t.Log("✅ AutoRelay 创建成功")
}

// TestAutoRelay_StartStop 测试启动停止
func TestAutoRelay_StartStop(t *testing.T) {
	config := DefaultAutoRelayConfig()
	config.RefreshInterval = 100 * time.Millisecond
	config.DiscoveryInterval = 100 * time.Millisecond

	client := &mockRelayClient{}
	ar := NewAutoRelay(config, client, nil, nil)

	// 启动
	ctx := context.Background()
	err := ar.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 验证运行状态
	if atomic.LoadInt32(&ar.running) != 1 {
		t.Error("AutoRelay should be running")
	}

	// 重复启动应该无影响
	err = ar.Start(ctx)
	if err != nil {
		t.Errorf("Second Start should not error: %v", err)
	}

	// 停止
	err = ar.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// 验证停止状态
	if atomic.LoadInt32(&ar.running) != 0 {
		t.Error("AutoRelay should be stopped")
	}

	// 重复停止应该无影响
	err = ar.Stop()
	if err != nil {
		t.Errorf("Second Stop should not error: %v", err)
	}

	t.Log("✅ 启动停止正常")
}

// TestAutoRelay_EnableDisable 测试启用禁用
func TestAutoRelay_EnableDisable(t *testing.T) {
	config := DefaultAutoRelayConfig()
	client := &mockRelayClient{}
	ar := NewAutoRelay(config, client, nil, nil)

	// 初始状态应该是禁用
	if ar.IsEnabled() {
		t.Error("AutoRelay should be disabled initially")
	}

	// 启用
	ar.Enable()
	if !ar.IsEnabled() {
		t.Error("AutoRelay should be enabled")
	}

	// 禁用
	ar.Disable()
	if ar.IsEnabled() {
		t.Error("AutoRelay should be disabled")
	}

	t.Log("✅ 启用禁用正常")
}

// TestAutoRelay_AddRemoveCandidate 测试候选管理
func TestAutoRelay_AddRemoveCandidate(t *testing.T) {
	config := DefaultAutoRelayConfig()
	client := &mockRelayClient{}
	ar := NewAutoRelay(config, client, nil, nil)

	// 添加候选
	ar.AddCandidate("relay1", []string{"/ip4/1.2.3.4/tcp/1234"}, 10)
	ar.AddCandidate("relay2", []string{"/ip4/5.6.7.8/tcp/5678"}, 5)

	ar.candidatesMu.RLock()
	if len(ar.candidates) != 2 {
		t.Errorf("candidates count = %d, want 2", len(ar.candidates))
	}
	ar.candidatesMu.RUnlock()

	// 移除候选
	ar.RemoveCandidate("relay1")

	ar.candidatesMu.RLock()
	if len(ar.candidates) != 1 {
		t.Errorf("candidates count = %d, want 1", len(ar.candidates))
	}
	if _, exists := ar.candidates["relay2"]; !exists {
		t.Error("relay2 should still exist")
	}
	ar.candidatesMu.RUnlock()

	t.Log("✅ 候选管理正常")
}

// TestAutoRelay_Blacklist 测试黑名单
func TestAutoRelay_Blacklist(t *testing.T) {
	config := DefaultAutoRelayConfig()
	client := &mockRelayClient{}
	ar := NewAutoRelay(config, client, nil, nil)

	// 初始不在黑名单
	if ar.isBlacklisted("relay1") {
		t.Error("relay1 should not be blacklisted initially")
	}

	// 添加到黑名单
	ar.addToBlacklist("relay1")

	// 检查黑名单
	if !ar.isBlacklisted("relay1") {
		t.Error("relay1 should be blacklisted")
	}

	// 清理过期黑名单（手动设置过期）
	ar.blacklistMu.Lock()
	ar.blacklist["relay1"] = time.Now().Add(-time.Second) // 已过期
	ar.blacklistMu.Unlock()

	ar.cleanupBlacklist()

	if ar.isBlacklisted("relay1") {
		t.Error("relay1 should be removed from blacklist after cleanup")
	}

	t.Log("✅ 黑名单管理正常")
}

// TestAutoRelay_PreferredRelays 测试首选中继
func TestAutoRelay_PreferredRelays(t *testing.T) {
	config := DefaultAutoRelayConfig()
	client := &mockRelayClient{}
	ar := NewAutoRelay(config, client, nil, nil)

	// 初始无首选
	if ar.isPreferredRelay("relay1") {
		t.Error("relay1 should not be preferred initially")
	}

	// 设置首选
	ar.SetPreferredRelays([]string{"relay1", "relay2"})

	if !ar.isPreferredRelay("relay1") {
		t.Error("relay1 should be preferred")
	}
	if !ar.isPreferredRelay("relay2") {
		t.Error("relay2 should be preferred")
	}
	if ar.isPreferredRelay("relay3") {
		t.Error("relay3 should not be preferred")
	}

	t.Log("✅ 首选中继设置正常")
}

// TestAutoRelay_Status 测试状态查询
func TestAutoRelay_Status(t *testing.T) {
	config := DefaultAutoRelayConfig()
	client := &mockRelayClient{}
	ar := NewAutoRelay(config, client, nil, nil)

	// 添加一些测试数据
	ar.activeRelaysMu.Lock()
	ar.activeRelays["relay1"] = &activeRelay{
		relayID: "relay1",
		addrs:   []string{"/ip4/1.2.3.4/tcp/1234"},
	}
	ar.activeRelaysMu.Unlock()

	ar.AddCandidate("relay2", []string{}, 0)
	ar.addToBlacklist("relay3")

	// 获取状态
	status := ar.Status()

	if status.NumRelays != 1 {
		t.Errorf("NumRelays = %d, want 1", status.NumRelays)
	}
	if status.NumCandidates != 1 {
		t.Errorf("NumCandidates = %d, want 1", status.NumCandidates)
	}
	if status.NumBlacklisted != 1 {
		t.Errorf("NumBlacklisted = %d, want 1", status.NumBlacklisted)
	}
	if len(status.RelayAddrs) != 1 {
		t.Errorf("RelayAddrs count = %d, want 1", len(status.RelayAddrs))
	}

	t.Log("✅ 状态查询正常")
}

// TestAutoRelay_Relays 测试中继列表
func TestAutoRelay_Relays(t *testing.T) {
	config := DefaultAutoRelayConfig()
	client := &mockRelayClient{}
	ar := NewAutoRelay(config, client, nil, nil)

	// 添加活跃中继
	ar.activeRelaysMu.Lock()
	ar.activeRelays["relay1"] = &activeRelay{relayID: "relay1"}
	ar.activeRelays["relay2"] = &activeRelay{relayID: "relay2"}
	ar.activeRelaysMu.Unlock()

	relays := ar.Relays()

	if len(relays) != 2 {
		t.Errorf("Relays count = %d, want 2", len(relays))
	}

	t.Log("✅ 中继列表查询正常")
}

// TestAutoRelay_RelayAddrs 测试中继地址
func TestAutoRelay_RelayAddrs(t *testing.T) {
	config := DefaultAutoRelayConfig()
	client := &mockRelayClient{}
	ar := NewAutoRelay(config, client, nil, nil)

	// 添加活跃中继
	ar.activeRelaysMu.Lock()
	ar.activeRelays["relay1"] = &activeRelay{
		relayID: "relay1",
		addrs:   []string{"/ip4/1.2.3.4/tcp/1234", "/ip4/1.2.3.4/udp/1234/quic-v1"},
	}
	ar.activeRelays["relay2"] = &activeRelay{
		relayID: "relay2",
		addrs:   []string{"/ip4/5.6.7.8/tcp/5678"},
	}
	ar.activeRelaysMu.Unlock()

	addrs := ar.RelayAddrs()

	if len(addrs) != 3 {
		t.Errorf("RelayAddrs count = %d, want 3", len(addrs))
	}

	t.Log("✅ 中继地址查询正常")
}

// TestAutoRelay_OnAddrsChanged 测试地址变更回调
func TestAutoRelay_OnAddrsChanged(t *testing.T) {
	config := DefaultAutoRelayConfig()
	client := &mockRelayClient{}
	ar := NewAutoRelay(config, client, nil, nil)

	// 设置回调
	callbackCalled := make(chan []string, 1)
	ar.SetOnAddrsChanged(func(addrs []string) {
		callbackCalled <- addrs
	})

	// 添加活跃中继
	ar.activeRelaysMu.Lock()
	ar.activeRelays["relay1"] = &activeRelay{
		relayID: "relay1",
		addrs:   []string{"/ip4/1.2.3.4/tcp/1234"},
	}
	ar.activeRelaysMu.Unlock()

	// 触发通知
	ar.notifyAddrsChanged()

	// 等待回调
	select {
	case addrs := <-callbackCalled:
		if len(addrs) != 1 {
			t.Errorf("callback addrs count = %d, want 1", len(addrs))
		}
	case <-time.After(time.Second):
		t.Error("callback not called within timeout")
	}

	t.Log("✅ 地址变更回调正常")
}

// TestAutoRelay_StaticRelays 测试静态中继
func TestAutoRelay_StaticRelays(t *testing.T) {
	config := DefaultAutoRelayConfig()
	config.StaticRelays = []string{"static-relay-1", "static-relay-2"}

	client := &mockRelayClient{}
	ar := NewAutoRelay(config, client, nil, nil)

	// 启动
	ctx := context.Background()
	err := ar.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer ar.Stop()

	// 验证静态中继已添加到候选
	ar.candidatesMu.RLock()
	if len(ar.candidates) != 2 {
		t.Errorf("candidates count = %d, want 2", len(ar.candidates))
	}
	if _, exists := ar.candidates["static-relay-1"]; !exists {
		t.Error("static-relay-1 should be in candidates")
	}
	if _, exists := ar.candidates["static-relay-2"]; !exists {
		t.Error("static-relay-2 should be in candidates")
	}
	ar.candidatesMu.RUnlock()

	t.Log("✅ 静态中继配置正常")
}

// TestAutoRelay_GetCandidates 测试获取候选列表
func TestAutoRelay_GetCandidates(t *testing.T) {
	config := DefaultAutoRelayConfig()
	client := &mockRelayClient{}
	ar := NewAutoRelay(config, client, nil, nil)

	// 添加候选
	ar.AddCandidate("relay1", []string{}, 10)
	ar.AddCandidate("relay2", []string{}, 5)
	ar.AddCandidate("relay3", []string{}, 20)

	// 获取候选（按优先级排序）
	candidates := ar.getCandidates(10)

	if len(candidates) != 3 {
		t.Errorf("candidates count = %d, want 3", len(candidates))
	}

	// 验证排序（按优先级降序）
	if candidates[0].relayID != "relay3" {
		t.Errorf("first candidate = %s, want relay3", candidates[0].relayID)
	}
	if candidates[1].relayID != "relay1" {
		t.Errorf("second candidate = %s, want relay1", candidates[1].relayID)
	}

	t.Log("✅ 获取候选列表正常")
}

// TestAutoRelay_GetCandidates_WithPreferred 测试首选中继优先
func TestAutoRelay_GetCandidates_WithPreferred(t *testing.T) {
	config := DefaultAutoRelayConfig()
	client := &mockRelayClient{}
	ar := NewAutoRelay(config, client, nil, nil)

	// 添加候选
	ar.AddCandidate("relay1", []string{}, 10)
	ar.AddCandidate("relay2", []string{}, 5)
	ar.AddCandidate("relay3", []string{}, 20)

	// 设置 relay2 为首选（即使优先级低）
	ar.SetPreferredRelays([]string{"relay2"})

	// 获取候选
	candidates := ar.getCandidates(10)

	// relay2 应该排在第一位（因为是首选）
	if candidates[0].relayID != "relay2" {
		t.Errorf("first candidate = %s, want relay2 (preferred)", candidates[0].relayID)
	}

	t.Log("✅ 首选中继优先排序正常")
}

// TestBuildCircuitAddr 测试构建 circuit 地址
func TestBuildCircuitAddr(t *testing.T) {
	addr := buildCircuitAddr("/ip4/1.2.3.4/tcp/1234/p2p/relay-id", "local-id")
	expected := "/ip4/1.2.3.4/tcp/1234/p2p/relay-id/p2p-circuit/p2p/local-id"

	if addr != expected {
		t.Errorf("buildCircuitAddr = %s, want %s", addr, expected)
	}

	t.Log("✅ circuit 地址构建正常")
}

// TestAutoRelay_Concurrent 测试并发安全
func TestAutoRelay_Concurrent(t *testing.T) {
	config := DefaultAutoRelayConfig()
	config.RefreshInterval = 50 * time.Millisecond
	config.DiscoveryInterval = 50 * time.Millisecond

	client := &mockRelayClient{}
	ar := NewAutoRelay(config, client, nil, nil)

	ctx := context.Background()
	if err := ar.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// 并发操作
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			relayID := "relay-" + string(rune('A'+id))
			for j := 0; j < 100; j++ {
				ar.AddCandidate(relayID, []string{}, id)
				ar.RemoveCandidate(relayID)
				ar.IsEnabled()
				ar.Relays()
				ar.RelayAddrs()
				ar.Status()
			}
		}(i)
	}

	wg.Wait()

	if err := ar.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	t.Log("✅ 并发安全测试通过")
}

// TestAutoRelay_RemoveRelay 测试移除中继
func TestAutoRelay_RemoveRelay(t *testing.T) {
	config := DefaultAutoRelayConfig()
	client := &mockRelayClient{}
	ar := NewAutoRelay(config, client, nil, nil)

	// 添加活跃中继
	mockRes := &mockReservation{
		relayID: "relay1",
		addrs:   []string{"/ip4/1.2.3.4/tcp/1234"},
	}

	ar.activeRelaysMu.Lock()
	ar.activeRelays["relay1"] = &activeRelay{
		relayID:     "relay1",
		reservation: mockRes,
		addrs:       []string{"/ip4/1.2.3.4/tcp/1234"},
	}
	ar.activeRelaysMu.Unlock()

	// 移除
	ar.removeRelay("relay1")

	// 验证已移除
	ar.activeRelaysMu.RLock()
	_, exists := ar.activeRelays["relay1"]
	ar.activeRelaysMu.RUnlock()

	if exists {
		t.Error("relay1 should be removed")
	}

	// 验证 reservation 被取消
	if !mockRes.cancelled {
		t.Error("reservation should be cancelled")
	}

	t.Log("✅ 移除中继正常")
}

// ============================================================================
//                              测试用例 - reservationWrapper
// ============================================================================

// TestReservationWrapper 测试预留包装器
func TestReservationWrapper(t *testing.T) {
	expireTime := time.Now().Add(time.Hour)
	wrapper := &reservationWrapper{
		relayPeer:  types.PeerID("relay1"),
		expireTime: expireTime,
		addrs:      []string{"/ip4/1.2.3.4/tcp/1234"},
		client:     nil,
	}

	// 测试 Expiry
	if wrapper.Expiry() != expireTime.Unix() {
		t.Errorf("Expiry = %d, want %d", wrapper.Expiry(), expireTime.Unix())
	}

	// 测试 Addrs
	addrs := wrapper.Addrs()
	if len(addrs) != 1 || addrs[0] != "/ip4/1.2.3.4/tcp/1234" {
		t.Error("Addrs not correct")
	}

	// 测试 Refresh（无 client 应该返回错误）
	err := wrapper.Refresh(context.Background())
	if err != ErrClientClosed {
		t.Errorf("Refresh should return ErrClientClosed, got %v", err)
	}

	// 测试 Cancel（不应该 panic）
	wrapper.Cancel()

	t.Log("✅ reservationWrapper 正常")
}

// ============================================================================
//                 真正能发现 BUG 的测试
// ============================================================================

// TestAutoRelay_GetCandidates_ZeroOrNegativeCount 测试请求 0 或负数个候选
// 潜在BUG：getCandidates(0) 或 getCandidates(-1) 的行为
func TestAutoRelay_GetCandidates_ZeroOrNegativeCount(t *testing.T) {
	config := DefaultAutoRelayConfig()
	client := &mockRelayClient{}
	ar := NewAutoRelay(config, client, nil, nil)

	// 添加候选
	ar.AddCandidate("relay1", []string{}, 10)
	ar.AddCandidate("relay2", []string{}, 5)

	// 请求 0 个
	candidates := ar.getCandidates(0)
	if len(candidates) != 0 {
		t.Errorf("请求 0 个候选应该返回空列表，但返回了 %d 个", len(candidates))
	}

	// 请求负数个 - 这可能会导致问题
	candidates = ar.getCandidates(-1)
	// 由于 slice[:negative] 会 panic，这个测试验证代码是否正确处理
	// 如果代码没有崩溃，说明处理正确
	t.Logf("请求 -1 个候选返回 %d 个", len(candidates))
}

// mockPeerstore 模拟 Peerstore（用于 HOP 协议检查测试）
type mockPeerstore struct {
	protocols map[types.PeerID][]types.ProtocolID
}

func newMockPeerstore() *mockPeerstore {
	return &mockPeerstore{
		protocols: make(map[types.PeerID][]types.ProtocolID),
	}
}

// SetTestProtocols 设置测试用协议列表（不是接口方法）
func (m *mockPeerstore) SetTestProtocols(peerID types.PeerID, protos []types.ProtocolID) {
	m.protocols[peerID] = protos
}

// SetProtocols 实现 ProtoBook 接口
func (m *mockPeerstore) SetProtocols(peerID types.PeerID, protos ...types.ProtocolID) error {
	m.protocols[peerID] = protos
	return nil
}

func (m *mockPeerstore) SupportsProtocols(peerID types.PeerID, protos ...types.ProtocolID) ([]types.ProtocolID, error) {
	peerProtos := m.protocols[peerID]
	var supported []types.ProtocolID
	for _, p := range protos {
		for _, pp := range peerProtos {
			if p == pp {
				supported = append(supported, p)
				break
			}
		}
	}
	return supported, nil
}

func (m *mockPeerstore) GetProtocols(peerID types.PeerID) ([]types.ProtocolID, error) {
	return m.protocols[peerID], nil
}

// 其他 Peerstore 接口方法的空实现
func (m *mockPeerstore) AddAddr(peerID types.PeerID, addr types.Multiaddr, ttl time.Duration)     {}
func (m *mockPeerstore) AddAddrs(peerID types.PeerID, addrs []types.Multiaddr, ttl time.Duration) {}
func (m *mockPeerstore) SetAddr(peerID types.PeerID, addr types.Multiaddr, ttl time.Duration)     {}
func (m *mockPeerstore) SetAddrs(peerID types.PeerID, addrs []types.Multiaddr, ttl time.Duration) {}
func (m *mockPeerstore) Addrs(peerID types.PeerID) []types.Multiaddr                              { return nil }
func (m *mockPeerstore) ClearAddrs(peerID types.PeerID)                                           {}
func (m *mockPeerstore) UpdateAddrs(peerID types.PeerID, oldTTL, newTTL time.Duration)            {}
func (m *mockPeerstore) PeersWithAddrs() []types.PeerID                                           { return nil }
func (m *mockPeerstore) AddrStream(ctx context.Context, peerID types.PeerID) <-chan types.Multiaddr {
	return nil
}
func (m *mockPeerstore) AddProtocols(peerID types.PeerID, protos ...types.ProtocolID) error {
	return nil
}
func (m *mockPeerstore) RemoveProtocols(peerID types.PeerID, protos ...types.ProtocolID) error {
	return nil
}
func (m *mockPeerstore) FirstSupportedProtocol(peerID types.PeerID, protos ...types.ProtocolID) (types.ProtocolID, error) {
	return "", nil
}
func (m *mockPeerstore) Get(peerID types.PeerID, key string) (interface{}, error) { return nil, nil }
func (m *mockPeerstore) Put(peerID types.PeerID, key string, val interface{}) error {
	return nil
}
func (m *mockPeerstore) Peers() []types.PeerID                { return nil }
func (m *mockPeerstore) Close() error                         { return nil }
func (m *mockPeerstore) PeerInfo(peerID types.PeerID) types.PeerInfo { return types.PeerInfo{} }
func (m *mockPeerstore) RemovePeer(peerID types.PeerID) {}
func (m *mockPeerstore) QuerySeeds(count int, maxAge time.Duration) []*pkgif.NodeRecord { return nil }
func (m *mockPeerstore) UpdateDialAttempt(peerID types.PeerID, success bool) error { return nil }
func (m *mockPeerstore) NodeDBSize() int { return 0 }
func (m *mockPeerstore) PubKey(peerID types.PeerID) (pkgif.PublicKey, error) { return nil, nil }
func (m *mockPeerstore) AddPubKey(peerID types.PeerID, pubKey pkgif.PublicKey) error { return nil }
func (m *mockPeerstore) PrivKey(peerID types.PeerID) (pkgif.PrivateKey, error) { return nil, nil }
func (m *mockPeerstore) AddPrivKey(peerID types.PeerID, privKey pkgif.PrivateKey) error { return nil }
func (m *mockPeerstore) PeersWithKeys() []types.PeerID { return nil }


// TestAutoRelay_TryRelay_NotRunning 测试未运行时尝试中继
func TestAutoRelay_TryRelay_NotRunning(t *testing.T) {
	config := DefaultAutoRelayConfig()
	client := &mockRelayClient{}
	ar := NewAutoRelay(config, client, nil, nil)
	// 不调用 Start，直接尝试

	result := ar.tryRelay("relay1")
	if result {
		t.Error("BUG: 未运行时 tryRelay 应该返回 false")
	}
}

// TestAutoRelay_TryRelay_SkipsNonHopPeer 测试跳过不支持 HOP 协议的节点
// 这是 BUG #B18 的回归测试
func TestAutoRelay_TryRelay_SkipsNonHopPeer(t *testing.T) {
	config := DefaultAutoRelayConfig()
	client := &mockRelayClient{}
	peerstore := newMockPeerstore()
	ar := NewAutoRelay(config, client, nil, peerstore)

	// 启动
	ar.Start(context.Background())
	defer ar.Stop()

	// 设置一个节点有协议信息但不支持 HOP 协议
	peerstore.SetTestProtocols(types.PeerID("non-relay-peer"), []types.ProtocolID{
		"/dep2p/sys/identify/1.0.0",
		"/dep2p/sys/ping/1.0.0",
	})

	// 尝试预留，应该被跳过（不应调用 Reserve）
	initialCalls := atomic.LoadInt32(&client.reserveCalls)
	result := ar.tryRelay("non-relay-peer")

	if result {
		t.Error("BUG: 不支持 HOP 协议的节点不应该预留成功")
	}
	if atomic.LoadInt32(&client.reserveCalls) != initialCalls {
		t.Error("BUG: 不支持 HOP 协议的节点不应该触发 Reserve 调用")
	}

	// 检查是否被加入黑名单
	if !ar.isBlacklisted("non-relay-peer") {
		t.Error("BUG: 不支持 HOP 协议的节点应该被加入黑名单")
	}
}

// TestAutoRelay_TryRelay_AllowsUnknownPeer 测试允许协议信息未知的节点尝试预留
func TestAutoRelay_TryRelay_AllowsUnknownPeer(t *testing.T) {
	config := DefaultAutoRelayConfig()
	client := &mockRelayClient{}
	peerstore := newMockPeerstore()
	ar := NewAutoRelay(config, client, nil, peerstore)

	// 启动
	ar.Start(context.Background())
	defer ar.Stop()

	// 不设置任何协议信息（模拟新发现的节点）

	// 尝试预留，应该允许（因为没有协议信息来判断）
	initialCalls := atomic.LoadInt32(&client.reserveCalls)
	_ = ar.tryRelay("unknown-peer")

	if atomic.LoadInt32(&client.reserveCalls) == initialCalls {
		t.Error("BUG: 协议信息未知的节点应该允许尝试预留")
	}
}

// TestAutoRelay_TryRelay_AllowsHopPeer 测试允许支持 HOP 协议的节点预留
func TestAutoRelay_TryRelay_AllowsHopPeer(t *testing.T) {
	config := DefaultAutoRelayConfig()
	client := &mockRelayClient{}
	peerstore := newMockPeerstore()
	ar := NewAutoRelay(config, client, nil, peerstore)

	// 启动
	ar.Start(context.Background())
	defer ar.Stop()

	// 设置一个支持 HOP 协议的节点
	peerstore.SetTestProtocols(types.PeerID("relay-peer"), []types.ProtocolID{
		"/dep2p/sys/identify/1.0.0",
		types.ProtocolID(HopProtocolID),
	})

	// 尝试预留，应该允许
	initialCalls := atomic.LoadInt32(&client.reserveCalls)
	_ = ar.tryRelay("relay-peer")

	if atomic.LoadInt32(&client.reserveCalls) == initialCalls {
		t.Error("BUG: 支持 HOP 协议的节点应该允许预留")
	}
}

// TestAutoRelay_TryRelay_AlreadyActive 测试已激活的中继
func TestAutoRelay_TryRelay_AlreadyActive(t *testing.T) {
	config := DefaultAutoRelayConfig()
	client := &mockRelayClient{}
	ar := NewAutoRelay(config, client, nil, nil)

	// 启动
	ar.Start(context.Background())
	defer ar.Stop()

	// 手动添加活跃中继
	ar.activeRelaysMu.Lock()
	ar.activeRelays["relay1"] = &activeRelay{relayID: "relay1"}
	ar.activeRelaysMu.Unlock()

	// 尝试同一个中继
	result := ar.tryRelay("relay1")
	if result {
		t.Error("BUG: 已激活的中继不应该重复尝试")
	}
}

// TestAutoRelay_EnsureRelays_AlreadyHaveEnough 测试已有足够中继时的行为
func TestAutoRelay_EnsureRelays_AlreadyHaveEnough(t *testing.T) {
	config := DefaultAutoRelayConfig()
	config.MinRelays = 2
	client := &mockRelayClient{}
	ar := NewAutoRelay(config, client, nil, nil)

	// 手动添加足够的活跃中继
	ar.activeRelaysMu.Lock()
	ar.activeRelays["relay1"] = &activeRelay{relayID: "relay1"}
	ar.activeRelays["relay2"] = &activeRelay{relayID: "relay2"}
	ar.activeRelaysMu.Unlock()

	// 调用 ensureRelays，不应该触发新的预留
	initialCalls := atomic.LoadInt32(&client.reserveCalls)
	ar.ensureRelays()
	
	if atomic.LoadInt32(&client.reserveCalls) != initialCalls {
		t.Error("BUG: 已有足够中继时不应该触发新的预留")
	}
}

// TestAutoRelay_RefreshReservations_ExpiryZero 测试 Expiry 返回 0 的情况
// 这是 BUG #B12 的回归测试
func TestAutoRelay_RefreshReservations_ExpiryZero(t *testing.T) {
	config := DefaultAutoRelayConfig()
	config.RefreshInterval = 50 * time.Millisecond
	client := &mockRelayClient{}
	ar := NewAutoRelay(config, client, nil, nil)

	// 启动
	ar.Start(context.Background())
	defer ar.Stop()

	// 创建一个 expiry=0 的预留
	mockRes := &mockReservation{
		relayID: "relay1",
		expiry:  0, // 关键：expiry 为 0
	}

	ar.activeRelaysMu.Lock()
	ar.activeRelays["relay1"] = &activeRelay{
		relayID:     "relay1",
		reservation: mockRes,
	}
	ar.activeRelaysMu.Unlock()

	// 调用刷新
	ar.refreshReservations()

	// expiry=0 时应该尝试刷新（不应该跳过）
	if atomic.LoadInt32(&mockRes.refreshCount) == 0 {
		t.Error("BUG #B12: expiry=0 时应该尝试刷新，但被跳过了")
	}
}

// TestAutoRelay_RefreshReservations_ConcurrentModification 测试刷新时的并发修改
// 验证 BUG #B13 的修复
func TestAutoRelay_RefreshReservations_ConcurrentModification(t *testing.T) {
	config := DefaultAutoRelayConfig()
	client := &mockRelayClient{}
	ar := NewAutoRelay(config, client, nil, nil)

	// 启动
	ar.Start(context.Background())
	defer ar.Stop()

	// 创建一个会失败的预留
	mockRes := &mockReservation{
		relayID:    "relay1",
		expiry:     0,
		refreshErr: ErrClientClosed,
	}

	ar.activeRelaysMu.Lock()
	ar.activeRelays["relay1"] = &activeRelay{
		relayID:     "relay1",
		reservation: mockRes,
	}
	ar.activeRelaysMu.Unlock()

	// 并发刷新和修改
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ar.refreshReservations()
		}()
	}

	wg.Wait()
	// 如果没有 panic，说明并发修改被正确处理
}

// TestAutoRelay_Blacklist_PriorityBasedTTL 测试基于优先级的黑名单 TTL
func TestAutoRelay_Blacklist_PriorityBasedTTL(t *testing.T) {
	config := DefaultAutoRelayConfig()
	client := &mockRelayClient{}
	ar := NewAutoRelay(config, client, nil, nil)

	// 添加一个低优先级的候选（priority < 0）
	ar.candidatesMu.Lock()
	ar.candidates["bad-relay"] = &relayCandidate{
		relayID:  "bad-relay",
		priority: -1,
	}
	ar.candidatesMu.Unlock()

	// 添加到黑名单
	ar.addToBlacklist("bad-relay")

	// 验证使用了长期黑名单
	ar.blacklistMu.RLock()
	expiry := ar.blacklist["bad-relay"]
	ar.blacklistMu.RUnlock()

	// 长期黑名单应该是 1 小时后
	if time.Until(expiry) < 55*time.Minute {
		t.Error("BUG: 低优先级候选应该使用长期黑名单 (1小时)")
	}
}

// TestAutoRelay_DiscoverRelays_NilClient 测试 client 为 nil 时的发现
func TestAutoRelay_DiscoverRelays_NilClient(t *testing.T) {
	config := DefaultAutoRelayConfig()
	// 故意不设置 client
	ar := NewAutoRelay(config, nil, nil, nil)

	// 启动
	ar.Start(context.Background())
	defer ar.Stop()

	// 调用发现，不应该 panic
	ar.discoverRelays()
}

// TestAutoRelay_Stop_CancelsAllReservations 测试停止时取消所有预留
func TestAutoRelay_Stop_CancelsAllReservations(t *testing.T) {
	config := DefaultAutoRelayConfig()
	client := &mockRelayClient{}
	ar := NewAutoRelay(config, client, nil, nil)

	// 启动
	ar.Start(context.Background())

	// 添加多个活跃中继
	mockRes1 := &mockReservation{relayID: "relay1"}
	mockRes2 := &mockReservation{relayID: "relay2"}

	ar.activeRelaysMu.Lock()
	ar.activeRelays["relay1"] = &activeRelay{relayID: "relay1", reservation: mockRes1}
	ar.activeRelays["relay2"] = &activeRelay{relayID: "relay2", reservation: mockRes2}
	ar.activeRelaysMu.Unlock()

	// 停止
	ar.Stop()

	// 验证所有预留都被取消
	if !mockRes1.cancelled {
		t.Error("BUG: relay1 的预留应该被取消")
	}
	if !mockRes2.cancelled {
		t.Error("BUG: relay2 的预留应该被取消")
	}

	// 验证 activeRelays 被清空
	ar.activeRelaysMu.RLock()
	count := len(ar.activeRelays)
	ar.activeRelaysMu.RUnlock()
	if count != 0 {
		t.Errorf("BUG: 停止后 activeRelays 应该为空，但有 %d 个", count)
	}
}

// TestAutoRelay_RemoveRelay_NonExistent 测试移除不存在的中继
func TestAutoRelay_RemoveRelay_NonExistent(t *testing.T) {
	config := DefaultAutoRelayConfig()
	client := &mockRelayClient{}
	ar := NewAutoRelay(config, client, nil, nil)

	// 移除不存在的中继，不应该 panic
	ar.removeRelay("non-existent")
}
