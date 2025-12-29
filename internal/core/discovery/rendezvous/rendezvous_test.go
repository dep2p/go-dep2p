package rendezvous

import (
	"context"
	"sync"
	"testing"
	"time"


	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	pb "github.com/dep2p/go-dep2p/pkg/proto/rendezvous"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Protocol 测试
// ============================================================================

func TestValidateNamespace(t *testing.T) {
	tests := []struct {
		name      string
		namespace string
		wantErr   bool
	}{
		{"valid", "blockchain/mainnet/peers", false},
		{"valid_realm", "realm-123/topic/events", false},
		{"valid_service", "service/storage", false},
		{"empty", "", true},
		{"too_long", string(make([]byte, MaxNamespaceLength+1)), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNamespace(tt.namespace)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateNamespace() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateTTL(t *testing.T) {
	maxTTL := 72 * time.Hour

	tests := []struct {
		name     string
		ttl      time.Duration
		expected time.Duration
	}{
		{"zero_uses_default", 0, DefaultTTL},
		{"negative_uses_default", -1 * time.Hour, DefaultTTL},
		{"within_max", 1 * time.Hour, 1 * time.Hour},
		{"exceeds_max", 100 * time.Hour, maxTTL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _ := ValidateTTL(tt.ttl, maxTTL)
			if result != tt.expected {
				t.Errorf("ValidateTTL() = %v, expected %v", result, tt.expected)
			}
		})
	}
}

func TestValidateAddresses(t *testing.T) {
	tests := []struct {
		name    string
		addrs   []string
		wantErr bool
	}{
		{"valid", []string{"/ip4/127.0.0.1/tcp/8000"}, false},
		{"multiple", []string{"/ip4/127.0.0.1/tcp/8000", "/ip4/192.168.1.1/tcp/8000"}, false},
		{"empty", []string{}, true},
		{"too_many", make([]string, MaxAddresses+1), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAddresses(tt.addrs)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAddresses() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewRegisterRequest(t *testing.T) {
	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("12345678901234567890123456789012"))
	addrs := []string{"/ip4/127.0.0.1/tcp/8000"}
	ttl := 2 * time.Hour

	msg := NewRegisterRequest("test/namespace", nodeID, addrs, ttl)

	if msg.Type != pb.MessageType_MESSAGE_TYPE_REGISTER {
		t.Errorf("expected REGISTER type, got %v", msg.Type)
	}
	if msg.Register.Namespace != "test/namespace" {
		t.Errorf("expected namespace 'test/namespace', got %v", msg.Register.Namespace)
	}
	if len(msg.Register.Addrs) != 1 {
		t.Errorf("expected 1 address, got %v", len(msg.Register.Addrs))
	}
	if msg.Register.Ttl != uint64(ttl.Seconds()) {
		t.Errorf("expected TTL %v, got %v", ttl.Seconds(), msg.Register.Ttl)
	}
}

func TestNewDiscoverRequest(t *testing.T) {
	msg := NewDiscoverRequest("test/namespace", 50, nil)

	if msg.Type != pb.MessageType_MESSAGE_TYPE_DISCOVER {
		t.Errorf("expected DISCOVER type, got %v", msg.Type)
	}
	if msg.Discover.Namespace != "test/namespace" {
		t.Errorf("expected namespace 'test/namespace', got %v", msg.Discover.Namespace)
	}
	if msg.Discover.Limit != 50 {
		t.Errorf("expected limit 50, got %v", msg.Discover.Limit)
	}
}

func TestStatusToError(t *testing.T) {
	tests := []struct {
		status  pb.ResponseStatus
		wantNil bool
	}{
		{pb.ResponseStatus_RESPONSE_STATUS_OK, true},
		{pb.ResponseStatus_RESPONSE_STATUS_E_INVALID_NAMESPACE, false},
		{pb.ResponseStatus_RESPONSE_STATUS_E_INVALID_TTL, false},
		{pb.ResponseStatus_RESPONSE_STATUS_E_NOT_AUTHORIZED, false},
		{pb.ResponseStatus_RESPONSE_STATUS_E_INTERNAL_ERROR, false},
	}

	for _, tt := range tests {
		t.Run(tt.status.String(), func(t *testing.T) {
			err := StatusToError(tt.status, "test message")
			if (err == nil) != tt.wantNil {
				t.Errorf("StatusToError() = %v, wantNil %v", err, tt.wantNil)
			}
		})
	}
}

// ============================================================================
//                              Store 测试
// ============================================================================

func TestStoreRegister(t *testing.T) {
	store := NewStore(DefaultStoreConfig())
	defer store.Close()

	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("12345678901234567890123456789012"))

	reg := &Registration{
		Namespace: "test/namespace",
		PeerInfo: discoveryif.PeerInfo{
			ID:    nodeID,
			Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/127.0.0.1/tcp/8000")},
		},
		TTL:          2 * time.Hour,
		RegisteredAt: time.Now(),
	}

	err := store.Register(reg)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// 验证存储
	got := store.GetRegistration("test/namespace", nodeID)
	if got == nil {
		t.Fatal("GetRegistration() returned nil")
	}
	if got.Namespace != reg.Namespace {
		t.Errorf("namespace mismatch: got %v, want %v", got.Namespace, reg.Namespace)
	}
}

func TestStoreUnregister(t *testing.T) {
	store := NewStore(DefaultStoreConfig())
	defer store.Close()

	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("12345678901234567890123456789012"))

	reg := &Registration{
		Namespace: "test/namespace",
		PeerInfo: discoveryif.PeerInfo{
			ID:    nodeID,
			Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/127.0.0.1/tcp/8000")},
		},
		TTL:          2 * time.Hour,
		RegisteredAt: time.Now(),
	}

	store.Register(reg)
	err := store.Unregister("test/namespace", nodeID)
	if err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}

	got := store.GetRegistration("test/namespace", nodeID)
	if got != nil {
		t.Error("GetRegistration() should return nil after unregister")
	}
}

func TestStoreDiscover(t *testing.T) {
	store := NewStore(DefaultStoreConfig())
	defer store.Close()

	// 注册多个节点
	for i := 0; i < 5; i++ {
		nodeID := types.NodeID{}
		copy(nodeID[:], []byte("1234567890123456789012345678901"+string(rune('0'+i))))

		reg := &Registration{
			Namespace: "test/namespace",
			PeerInfo: discoveryif.PeerInfo{
				ID:    nodeID,
				Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/127.0.0.1/tcp/800" + string(rune('0'+i)))},
			},
			TTL:          2 * time.Hour,
			RegisteredAt: time.Now(),
		}
		store.Register(reg)
	}

	// 发现
	regs, cookie, err := store.Discover("test/namespace", 3, nil)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(regs) != 3 {
		t.Errorf("expected 3 results, got %v", len(regs))
	}
	if cookie == nil {
		t.Error("expected non-nil cookie for pagination")
	}

	// 获取下一页
	regs2, cookie2, err := store.Discover("test/namespace", 3, cookie)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if len(regs2) != 2 {
		t.Errorf("expected 2 results on second page, got %v", len(regs2))
	}
	if cookie2 != nil {
		t.Error("expected nil cookie on last page")
	}
}

func TestStoreLimits(t *testing.T) {
	config := DefaultStoreConfig()
	config.MaxNamespaces = 2
	config.MaxRegistrationsPerNamespace = 2

	store := NewStore(config)
	defer store.Close()

	// 注册到两个命名空间
	for ns := 0; ns < 2; ns++ {
		for i := 0; i < 2; i++ {
			nodeID := types.NodeID{}
			copy(nodeID[:], []byte("123456789012345678901234567890"+string(rune('0'+ns))+string(rune('0'+i))))

			reg := &Registration{
				Namespace: "namespace" + string(rune('0'+ns)),
				PeerInfo: discoveryif.PeerInfo{
					ID:    nodeID,
					Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/127.0.0.1/tcp/8000")},
				},
				TTL:          2 * time.Hour,
				RegisteredAt: time.Now(),
			}
			store.Register(reg)
		}
	}

	// 尝试注册到第三个命名空间应该失败
	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("12345678901234567890123456789099"))

	reg := &Registration{
		Namespace: "namespace3",
		PeerInfo: discoveryif.PeerInfo{
			ID:    nodeID,
			Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/127.0.0.1/tcp/8000")},
		},
		TTL:          2 * time.Hour,
		RegisteredAt: time.Now(),
	}

	err := store.Register(reg)
	if err == nil {
		t.Error("expected error when exceeding namespace limit")
	}
}

func TestStoreStats(t *testing.T) {
	store := NewStore(DefaultStoreConfig())
	defer store.Close()

	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("12345678901234567890123456789012"))

	reg := &Registration{
		Namespace: "test/namespace",
		PeerInfo: discoveryif.PeerInfo{
			ID:    nodeID,
			Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/127.0.0.1/tcp/8000")},
		},
		TTL:          2 * time.Hour,
		RegisteredAt: time.Now(),
	}
	store.Register(reg)

	stats := store.Stats()
	if stats.TotalRegistrations != 1 {
		t.Errorf("expected 1 registration, got %v", stats.TotalRegistrations)
	}
	if stats.TotalNamespaces != 1 {
		t.Errorf("expected 1 namespace, got %v", stats.TotalNamespaces)
	}
}

// ============================================================================
//                              Point 测试
// ============================================================================

func TestPointHandleRegister(t *testing.T) {
	point := NewPoint(nil, DefaultPointConfig())

	from := types.NodeID{}
	copy(from[:], []byte("12345678901234567890123456789012"))

	info := discoveryif.PeerInfo{
		ID:    from,
		Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/127.0.0.1/tcp/8000")},
	}

	err := point.HandleRegister(from, "test/namespace", info, 2*time.Hour)
	if err != nil {
		t.Fatalf("HandleRegister() error = %v", err)
	}

	// 验证统计
	stats := point.Stats()
	if stats.TotalRegistrations != 1 {
		t.Errorf("expected 1 registration, got %v", stats.TotalRegistrations)
	}
}

func TestPointHandleDiscover(t *testing.T) {
	point := NewPoint(nil, DefaultPointConfig())

	// 注册一些节点
	for i := 0; i < 3; i++ {
		nodeID := types.NodeID{}
		copy(nodeID[:], []byte("1234567890123456789012345678901"+string(rune('0'+i))))

		info := discoveryif.PeerInfo{
			ID:    nodeID,
			Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/127.0.0.1/tcp/800" + string(rune('0'+i)))},
		}

		point.HandleRegister(nodeID, "test/namespace", info, 2*time.Hour)
	}

	// 发现
	from := types.NodeID{}
	peers, _, err := point.HandleDiscover(from, "test/namespace", 10, nil)
	if err != nil {
		t.Fatalf("HandleDiscover() error = %v", err)
	}
	if len(peers) != 3 {
		t.Errorf("expected 3 peers, got %v", len(peers))
	}
}

func TestPointNamespaces(t *testing.T) {
	point := NewPoint(nil, DefaultPointConfig())

	from := types.NodeID{}
	copy(from[:], []byte("12345678901234567890123456789012"))

	info := discoveryif.PeerInfo{
		ID:    from,
		Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/127.0.0.1/tcp/8000")},
	}

	point.HandleRegister(from, "namespace1", info, 2*time.Hour)
	point.HandleRegister(from, "namespace2", info, 2*time.Hour)

	namespaces := point.Namespaces()
	if len(namespaces) != 2 {
		t.Errorf("expected 2 namespaces, got %v", len(namespaces))
	}

	count := point.PeersInNamespace("namespace1")
	if count != 1 {
		t.Errorf("expected 1 peer in namespace1, got %v", count)
	}
}

// ============================================================================
//                              Discoverer 测试
// ============================================================================

func TestDiscovererConfig(t *testing.T) {
	config := DefaultDiscovererConfig()

	if config.DefaultTTL != 2*time.Hour {
		t.Errorf("expected DefaultTTL 2h, got %v", config.DefaultTTL)
	}
	if config.RenewalInterval != 1*time.Hour {
		t.Errorf("expected RenewalInterval 1h, got %v", config.RenewalInterval)
	}
}

func TestDiscovererPoints(t *testing.T) {
	config := DefaultDiscovererConfig()

	point1 := types.NodeID{}
	copy(point1[:], []byte("12345678901234567890123456789012"))

	point2 := types.NodeID{}
	copy(point2[:], []byte("12345678901234567890123456789013"))

	discoverer := NewDiscoverer(nil, types.NodeID{}, config)

	// 添加点
	discoverer.AddPoint(point1)
	discoverer.AddPoint(point2)

	points := discoverer.Points()
	if len(points) != 2 {
		t.Errorf("expected 2 points, got %v", len(points))
	}

	// 移除点
	discoverer.RemovePoint(point1)
	points = discoverer.Points()
	if len(points) != 1 {
		t.Errorf("expected 1 point after removal, got %v", len(points))
	}
}

func TestDiscovererValidation(t *testing.T) {
	config := DefaultDiscovererConfig()
	discoverer := NewDiscoverer(nil, types.NodeID{}, config)

	ctx := context.Background()

	// 无效命名空间应该返回错误
	err := discoverer.Register(ctx, "", 1*time.Hour)
	if err == nil {
		t.Error("expected error for empty namespace")
	}

	err = discoverer.Unregister(ctx, "")
	if err == nil {
		t.Error("expected error for empty namespace")
	}

	_, err = discoverer.Discover(ctx, "", 10)
	if err == nil {
		t.Error("expected error for empty namespace")
	}
}

func TestDiscovererNoPoints(t *testing.T) {
	config := DefaultDiscovererConfig()
	config.Points = nil
	discoverer := NewDiscoverer(nil, types.NodeID{}, config)

	ctx := context.Background()

	// 没有 Rendezvous 点应该返回错误
	err := discoverer.Register(ctx, "test/namespace", 1*time.Hour)
	if err == nil {
		t.Error("expected error when no rendezvous points available")
	}
}

// ============================================================================
//                              Registration 测试
// ============================================================================

func TestRegistrationExpiry(t *testing.T) {
	reg := &Registration{
		Namespace:    "test",
		TTL:          1 * time.Second,
		RegisteredAt: time.Now(),
		ExpiresAt:    time.Now().Add(-1 * time.Second), // 已过期
	}

	if !reg.IsExpired() {
		t.Error("expected registration to be expired")
	}

	reg2 := &Registration{
		Namespace:    "test",
		TTL:          1 * time.Hour,
		RegisteredAt: time.Now(),
		ExpiresAt:    time.Now().Add(1 * time.Hour), // 未过期
	}

	if reg2.IsExpired() {
		t.Error("expected registration to not be expired")
	}
}

func TestRegistrationRemainingTTL(t *testing.T) {
	// 已过期的注册
	reg := &Registration{
		ExpiresAt: time.Now().Add(-1 * time.Second),
	}

	if reg.RemainingTTL() != 0 {
		t.Errorf("expected 0 remaining TTL for expired registration, got %v", reg.RemainingTTL())
	}

	// 未过期的注册
	reg2 := &Registration{
		ExpiresAt: time.Now().Add(1 * time.Hour),
	}

	remaining := reg2.RemainingTTL()
	if remaining < 59*time.Minute || remaining > 61*time.Minute {
		t.Errorf("expected ~1h remaining TTL, got %v", remaining)
	}
}

// ============================================================================
//                              安全性测试
// ============================================================================

func TestDiscoverer_StopWithoutStart(t *testing.T) {
	config := DefaultDiscovererConfig()
	discoverer := NewDiscoverer(nil, types.NodeID{}, config)

	// 未启动就调用 Stop 不应该 panic
	err := discoverer.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func TestDiscoverer_StartStopMultiple(t *testing.T) {
	config := DefaultDiscovererConfig()
	discoverer := NewDiscoverer(nil, types.NodeID{}, config)

	ctx := context.Background()

	// 启动
	err := discoverer.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// 重复启动应该返回错误
	err = discoverer.Start(ctx)
	if err == nil {
		t.Error("expected error on second Start()")
	}

	// 停止
	err = discoverer.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	// 重复停止应该没问题
	err = discoverer.Stop()
	if err != nil {
		t.Errorf("second Stop() error = %v", err)
	}
}

func TestStore_Close_Idempotent(t *testing.T) {
	store := NewStore(DefaultStoreConfig())

	// 多次调用 Close 不应该 panic
	store.Close()
	store.Close()
	store.Close()
	// 测试通过表示没有 panic
}

func TestStore_Close_Concurrent(t *testing.T) {
	store := NewStore(DefaultStoreConfig())

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store.Close()
		}()
	}
	wg.Wait()
	// 测试通过表示没有竞态条件
}

func TestPoint_StartStopMultiple(t *testing.T) {
	point := NewPoint(nil, DefaultPointConfig())

	ctx := context.Background()

	// 启动
	err := point.Start(ctx)
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// 重复启动应该返回错误
	err = point.Start(ctx)
	if err == nil {
		t.Error("expected error on second Start()")
	}

	// 停止
	err = point.Stop()
	if err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	// 重复停止应该没问题
	err = point.Stop()
	if err != nil {
		t.Errorf("second Stop() error = %v", err)
	}
}

