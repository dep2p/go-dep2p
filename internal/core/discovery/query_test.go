// Package discovery 统一发现 API 测试
package discovery

import (
	"context"
	"testing"
	"time"

	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Scope 解析测试
// ============================================================================

func TestDiscoveryService_ResolveScope(t *testing.T) {
	config := discoveryif.DefaultConfig()
	config.EnableDHT = false
	config.EnableMDNS = false

	service := NewDiscoveryService(nil, nil, config)

	tests := []struct {
		name            string
		namespace       string
		scope           discoveryif.Scope
		currentRealm    types.RealmID
		explicitRealmID types.RealmID
		wantScope       discoveryif.Scope
		wantNamespace   string
	}{
		{
			name:          "sys: 前缀强制 ScopeSys",
			namespace:     "sys:relay",
			scope:         discoveryif.ScopeAuto,
			currentRealm:  "test-realm",
			wantScope:     discoveryif.ScopeSys,
			wantNamespace: "relay",
		},
		{
			name:          "显式 ScopeSys",
			namespace:     "relay",
			scope:         discoveryif.ScopeSys,
			currentRealm:  "test-realm",
			wantScope:     discoveryif.ScopeSys,
			wantNamespace: "relay",
		},
		{
			name:          "显式 ScopeRealm",
			namespace:     "myapp/chat",
			scope:         discoveryif.ScopeRealm,
			currentRealm:  "test-realm",
			wantScope:     discoveryif.ScopeRealm,
			wantNamespace: "myapp/chat",
		},
		{
			name:          "Auto + 有 Realm = ScopeRealm",
			namespace:     "myapp/chat",
			scope:         discoveryif.ScopeAuto,
			currentRealm:  "test-realm",
			wantScope:     discoveryif.ScopeRealm,
			wantNamespace: "myapp/chat",
		},
		{
			name:          "Auto + 无 Realm = ScopeSys",
			namespace:     "myapp/chat",
			scope:         discoveryif.ScopeAuto,
			currentRealm:  "",
			wantScope:     discoveryif.ScopeSys,
			wantNamespace: "myapp/chat",
		},
		{
			name:            "Auto + 显式 RealmID = ScopeRealm",
			namespace:       "myapp/chat",
			scope:           discoveryif.ScopeAuto,
			currentRealm:    "",
			explicitRealmID: "explicit-realm",
			wantScope:       discoveryif.ScopeRealm,
			wantNamespace:   "myapp/chat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 设置当前 Realm
			service.mu.Lock()
			service.currentRealm = tt.currentRealm
			service.mu.Unlock()

			gotScope, gotNS := service.resolveScope(tt.namespace, tt.scope, tt.explicitRealmID)

			if gotScope != tt.wantScope {
				t.Errorf("resolveScope() scope = %v, want %v", gotScope, tt.wantScope)
			}
			if gotNS != tt.wantNamespace {
				t.Errorf("resolveScope() namespace = %v, want %v", gotNS, tt.wantNamespace)
			}
		})
	}
}

// ============================================================================
//                              Key 构建测试
// ============================================================================

func TestDiscoveryService_BuildDiscoveryKey(t *testing.T) {
	config := discoveryif.DefaultConfig()
	service := NewDiscoveryService(nil, nil, config)
	service.currentRealm = "my-realm"

	tests := []struct {
		name            string
		scope           discoveryif.Scope
		namespace       string
		explicitRealmID types.RealmID
		wantKey         string
	}{
		{
			name:      "ScopeSys",
			scope:     discoveryif.ScopeSys,
			namespace: "relay",
			wantKey:   "dep2p/v1/sys/relay",
		},
		{
			name:      "ScopeRealm with current realm",
			scope:     discoveryif.ScopeRealm,
			namespace: "myapp/chat",
			wantKey:   "dep2p/v1/realm/my-realm/myapp/chat",
		},
		{
			name:            "ScopeRealm with explicit realm",
			scope:           discoveryif.ScopeRealm,
			namespace:       "myapp/chat",
			explicitRealmID: "other-realm",
			wantKey:         "dep2p/v1/realm/other-realm/myapp/chat",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotKey := service.buildDiscoveryKey(tt.scope, tt.namespace, tt.explicitRealmID)
			if gotKey != tt.wantKey {
				t.Errorf("buildDiscoveryKey() = %v, want %v", gotKey, tt.wantKey)
			}
		})
	}
}

// ============================================================================
//                              MergeResult 去重测试
// ============================================================================

func TestDiscoveryService_MergeResult_Dedup(t *testing.T) {
	config := discoveryif.DefaultConfig()
	service := NewDiscoveryService(nil, nil, config)

	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("test-node-id-12345678901234"))

	sourcePriority := map[discoveryif.Source]int{
		discoveryif.SourceProvider:   0,
		discoveryif.SourceRendezvous: 1,
		discoveryif.SourceLocal:      2,
	}

	results := make(map[types.NodeID]peerResult)

	// 先添加来自 Rendezvous 的结果（优先级 1）
	service.mergeResult(results, discoveryif.PeerInfo{
		ID:    nodeID,
		Addrs: types.StringsToMultiaddrs([]string{"/ip4/192.168.1.1/udp/8000/quic-v1"}),
	}, discoveryif.SourceRendezvous, sourcePriority)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[nodeID].source != discoveryif.SourceRendezvous {
		t.Errorf("expected source Rendezvous, got %v", results[nodeID].source)
	}

	// 再添加来自 Provider 的结果（优先级 0，更高）
	service.mergeResult(results, discoveryif.PeerInfo{
		ID:    nodeID,
		Addrs: types.StringsToMultiaddrs([]string{"/ip4/10.0.0.1/udp/9000/quic-v1"}),
	}, discoveryif.SourceProvider, sourcePriority)

	// 应该保留 Provider 的结果（优先级更高）
	if len(results) != 1 {
		t.Fatalf("expected 1 result after merge, got %d", len(results))
	}
	if results[nodeID].source != discoveryif.SourceProvider {
		t.Errorf("expected source Provider, got %v", results[nodeID].source)
	}

	// 再添加来自 Local 的结果（优先级 2，更低）
	service.mergeResult(results, discoveryif.PeerInfo{
		ID:    nodeID,
		Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/127.0.0.1/udp/7000/quic-v1")},
	}, discoveryif.SourceLocal, sourcePriority)

	// 应该仍然保留 Provider 的结果
	if results[nodeID].source != discoveryif.SourceProvider {
		t.Errorf("expected source still Provider, got %v", results[nodeID].source)
	}
}

// ============================================================================
//                              Discover 基本功能测试
// ============================================================================

func TestDiscoveryService_Discover_Basic(t *testing.T) {
	config := discoveryif.DefaultConfig()
	config.EnableDHT = false
	config.EnableMDNS = false

	service := NewDiscoveryService(nil, nil, config)

	// 添加一些本地节点
	nodeID1 := types.NodeID{}
	copy(nodeID1[:], []byte("test-node-id-11111111111111"))
	nodeID2 := types.NodeID{}
	copy(nodeID2[:], []byte("test-node-id-22222222222222"))

	service.AddKnownPeer(nodeID1, "", toAddressesFromStrings([]string{"192.168.1.1:8000"}))
	service.AddKnownPeer(nodeID2, "", toAddressesFromStrings([]string{"192.168.1.2:8000"}))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 启动服务
	if err := service.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer service.Stop()

	// 只使用 Local 来源查询
	query := discoveryif.DiscoveryQuery{
		Namespace:    "sys:test",
		Sources:      []discoveryif.Source{discoveryif.SourceLocal},
		IncludeLocal: true,
		Timeout:      2 * time.Second,
	}

	resultCh, err := service.Discover(ctx, query)
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}

	var results []discoveryif.DiscoveryResult
	for r := range resultCh {
		results = append(results, r)
	}

	// 应该返回 2 个本地节点
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}

	// 验证来源标记
	for _, r := range results {
		if r.Source != discoveryif.SourceLocal {
			t.Errorf("expected source Local, got %v", r.Source)
		}
	}
}

// ============================================================================
//                              Discover Limit 测试
// ============================================================================

func TestDiscoveryService_Discover_Limit(t *testing.T) {
	config := discoveryif.DefaultConfig()
	config.EnableDHT = false
	config.EnableMDNS = false

	service := NewDiscoveryService(nil, nil, config)

	// 添加 5 个本地节点
	for i := 0; i < 5; i++ {
		nodeID := types.NodeID{}
		copy(nodeID[:], []byte("test-node-id-"+string(rune('A'+i))+"1234567890123"))
		service.AddKnownPeer(nodeID, "", toAddressesFromStrings([]string{"192.168.1." + string(rune('1'+i)) + ":8000"}))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	service.Start(ctx)
	defer service.Stop()

	// 限制返回 2 个
	query := discoveryif.DiscoveryQuery{
		Namespace:    "sys:test",
		Sources:      []discoveryif.Source{discoveryif.SourceLocal},
		IncludeLocal: true,
		Limit:        2,
		Timeout:      2 * time.Second,
	}

	resultCh, err := service.Discover(ctx, query)
	if err != nil {
		t.Fatalf("Discover() error: %v", err)
	}

	var count int
	for range resultCh {
		count++
	}

	if count != 2 {
		t.Errorf("expected 2 results (limited), got %d", count)
	}
}

// ============================================================================
//                              RegisterService/UnregisterService 测试
// ============================================================================

func TestDiscoveryService_RegisterUnregisterService(t *testing.T) {
	config := discoveryif.DefaultConfig()
	config.EnableDHT = false
	config.EnableMDNS = false

	service := NewDiscoveryService(nil, nil, config)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	service.Start(ctx)
	defer service.Stop()

	reg := discoveryif.Registration{
		Namespace: "sys:test-service",
		Scope:     discoveryif.ScopeSys,
		TTL:       1 * time.Hour,
		Sources:   []discoveryif.Source{}, // 空，会被应用默认值
	}

	// 注册服务
	err := service.RegisterService(ctx, reg)
	if err != nil {
		// 没有 announcer，返回 nil 是正常的
		t.Logf("RegisterService returned: %v (expected without announcers)", err)
	}

	// 验证活跃注册被记录
	service.mu.RLock()
	hasReg := service.activeRegistrations != nil && len(service.activeRegistrations) > 0
	service.mu.RUnlock()

	if !hasReg {
		t.Error("expected active registration to be recorded")
	}

	// 注销服务
	err = service.UnregisterService(ctx, reg)
	if err != nil {
		t.Logf("UnregisterService returned: %v (expected without announcers)", err)
	}

	// 验证活跃注册被移除
	service.mu.RLock()
	isEmpty := service.activeRegistrations == nil || len(service.activeRegistrations) == 0
	service.mu.RUnlock()

	if !isEmpty {
		t.Error("expected active registration to be removed")
	}
}

// ============================================================================
//                              Default Query 测试
// ============================================================================

func TestDefaultQuery(t *testing.T) {
	query := discoveryif.DefaultQuery("my-namespace")

	if query.Namespace != "my-namespace" {
		t.Errorf("expected namespace 'my-namespace', got '%s'", query.Namespace)
	}
	if query.Scope != discoveryif.ScopeAuto {
		t.Errorf("expected ScopeAuto, got %v", query.Scope)
	}
	if len(query.Sources) != len(discoveryif.DefaultSources) {
		t.Errorf("expected default sources, got %v", query.Sources)
	}
	if !query.IncludeLocal {
		t.Error("expected IncludeLocal to be true")
	}
	if query.Timeout != 30*time.Second {
		t.Errorf("expected 30s timeout, got %v", query.Timeout)
	}
}

// ============================================================================
//                              Default Registration 测试
// ============================================================================

func TestDefaultRegistration(t *testing.T) {
	reg := discoveryif.DefaultRegistration("my-service")

	if reg.Namespace != "my-service" {
		t.Errorf("expected namespace 'my-service', got '%s'", reg.Namespace)
	}
	if reg.Scope != discoveryif.ScopeAuto {
		t.Errorf("expected ScopeAuto, got %v", reg.Scope)
	}
	if reg.TTL != 2*time.Hour {
		t.Errorf("expected 2h TTL, got %v", reg.TTL)
	}
	if len(reg.Sources) != 2 {
		t.Errorf("expected 2 sources (provider, rendezvous), got %d", len(reg.Sources))
	}
}

// ============================================================================
//                              Scope String 测试
// ============================================================================

func TestScope_String(t *testing.T) {
	tests := []struct {
		scope discoveryif.Scope
		want  string
	}{
		{discoveryif.ScopeAuto, "auto"},
		{discoveryif.ScopeSys, "sys"},
		{discoveryif.ScopeRealm, "realm"},
		{discoveryif.Scope(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.scope.String(); got != tt.want {
				t.Errorf("Scope.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

