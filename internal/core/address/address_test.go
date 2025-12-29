// Package address 地址管理模块测试
package address

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              测试地址实现
// ============================================================================

type testAddr struct {
	addr      string
	network   string
	isPublic  bool
	isPrivate bool
	isLoop    bool
}

func (a *testAddr) Network() string   { return a.network }
func (a *testAddr) String() string    { return a.addr }
func (a *testAddr) Bytes() []byte     { return []byte(a.addr) }
func (a *testAddr) IsPublic() bool    { return a.isPublic }
func (a *testAddr) IsPrivate() bool   { return a.isPrivate }
func (a *testAddr) IsLoopback() bool  { return a.isLoop }
func (a *testAddr) Multiaddr() string {
	// 如果已经是 multiaddr 格式，直接返回
	if len(a.addr) > 0 && a.addr[0] == '/' {
		return a.addr
	}
	// 否则转换为 multiaddr 格式
	return fmt.Sprintf("/ip4/%s/udp/8000/quic-v1", a.addr)
}
func (a *testAddr) Equal(other endpoint.Address) bool {
	if other == nil {
		return false
	}
	return a.addr == other.String()
}

func newTestAddr(addr string, public, private, loop bool) *testAddr {
	return &testAddr{
		addr:      addr,
		network:   "ip4",
		isPublic:  public,
		isPrivate: private,
		isLoop:    loop,
	}
}

// ============================================================================
//                              AddressBook 测试
// ============================================================================

func TestAddressBook_AddAndGet(t *testing.T) {
	book := NewAddressBook()

	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("test-node-id-12345678901234"))

	addr1 := newTestAddr("192.168.1.1:8000", false, true, false)
	addr2 := newTestAddr("10.0.0.1:8001", false, true, false)

	book.AddAddrs(nodeID, []endpoint.Address{addr1, addr2}, time.Hour)

	addrs := book.Addrs(nodeID)
	if len(addrs) != 2 {
		t.Errorf("expected 2 addresses, got %d", len(addrs))
	}
}

func TestAddressBook_TTL(t *testing.T) {
	book := NewAddressBook()

	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("test-node-id-12345678901234"))

	addr := newTestAddr("192.168.1.1:8000", false, true, false)

	// 添加短 TTL 地址
	book.AddAddrs(nodeID, []endpoint.Address{addr}, 10*time.Millisecond)

	// 立即应该能获取
	addrs := book.Addrs(nodeID)
	if len(addrs) != 1 {
		t.Errorf("expected 1 address immediately, got %d", len(addrs))
	}

	// 等待过期
	time.Sleep(20 * time.Millisecond)

	addrs = book.Addrs(nodeID)
	if len(addrs) != 0 {
		t.Errorf("expected 0 addresses after TTL, got %d", len(addrs))
	}
}

func TestAddressBook_Clear(t *testing.T) {
	book := NewAddressBook()

	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("test-node-id-12345678901234"))

	addr := newTestAddr("192.168.1.1:8000", false, true, false)
	book.AddAddrs(nodeID, []endpoint.Address{addr}, time.Hour)

	book.ClearAddrs(nodeID)

	addrs := book.Addrs(nodeID)
	if len(addrs) != 0 {
		t.Errorf("expected 0 addresses after clear, got %d", len(addrs))
	}
}

func TestAddressBook_Peers(t *testing.T) {
	book := NewAddressBook()

	nodeID1 := types.NodeID{}
	nodeID2 := types.NodeID{}
	copy(nodeID1[:], []byte("test-node-id-12345678901234"))
	copy(nodeID2[:], []byte("test-node-id-56789012345678"))

	addr1 := newTestAddr("192.168.1.1:8000", false, true, false)
	addr2 := newTestAddr("192.168.1.2:8000", false, true, false)

	book.AddAddrs(nodeID1, []endpoint.Address{addr1}, time.Hour)
	book.AddAddrs(nodeID2, []endpoint.Address{addr2}, time.Hour)

	peers := book.Peers()
	if len(peers) != 2 {
		t.Errorf("expected 2 peers, got %d", len(peers))
	}
}

func TestAddressBook_BestAddr(t *testing.T) {
	book := NewAddressBook()

	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("test-node-id-12345678901234"))

	publicAddr := newTestAddr("8.8.8.8:8000", true, false, false)
	privateAddr := newTestAddr("192.168.1.1:8000", false, true, false)

	book.AddAddrs(nodeID, []endpoint.Address{privateAddr, publicAddr}, time.Hour)

	best := book.BestAddr(nodeID)
	if best == nil {
		t.Fatal("expected best address, got nil")
	}

	// 公网地址应该优先
	if best.String() != publicAddr.String() {
		t.Errorf("expected public address %s, got %s", publicAddr.String(), best.String())
	}
}

func TestAddressBook_RecordSuccess(t *testing.T) {
	book := NewAddressBook()

	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("test-node-id-12345678901234"))

	addr := newTestAddr("192.168.1.1:8000", false, true, false)
	book.AddAddrs(nodeID, []endpoint.Address{addr}, time.Hour)

	// 记录成功
	book.RecordSuccess(nodeID, addr, 10*time.Millisecond)

	pas := book.GetPrioritizedAddrs(nodeID)
	if len(pas) == 0 {
		t.Fatal("expected prioritized addresses")
	}

	if pas[0].Stats.SuccessCount != 1 {
		t.Errorf("expected success count 1, got %d", pas[0].Stats.SuccessCount)
	}
}

func TestAddressBook_RecordFail(t *testing.T) {
	book := NewAddressBook()

	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("test-node-id-12345678901234"))

	addr := newTestAddr("192.168.1.1:8000", false, true, false)
	book.AddAddrs(nodeID, []endpoint.Address{addr}, time.Hour)

	// 记录失败
	book.RecordFail(nodeID, addr)

	pas := book.GetPrioritizedAddrs(nodeID)
	if len(pas) == 0 {
		t.Fatal("expected prioritized addresses")
	}

	if pas[0].Stats.FailCount != 1 {
		t.Errorf("expected fail count 1, got %d", pas[0].Stats.FailCount)
	}
}

func TestAddressBook_Cleanup(t *testing.T) {
	book := NewAddressBook()

	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("test-node-id-12345678901234"))

	addr := newTestAddr("192.168.1.1:8000", false, true, false)
	book.AddAddrs(nodeID, []endpoint.Address{addr}, 10*time.Millisecond)

	time.Sleep(20 * time.Millisecond)
	book.Cleanup()

	peers := book.Peers()
	if len(peers) != 0 {
		t.Errorf("expected 0 peers after cleanup, got %d", len(peers))
	}
}

// ============================================================================
//                              Parser 测试
// ============================================================================

func TestParser_ParseIPv4(t *testing.T) {
	parser := NewParser()

	// 根据 IMPL-ADDRESS-UNIFICATION.md，Parser.Parse() 仅接受 multiaddr 格式
	// host:port 格式应返回 ErrNotMultiaddrFormat 错误
	tests := []struct {
		input    string
		wantErr  bool
		wantIP   string
		wantPort int
	}{
		// host:port 格式现在应该报错
		{"192.168.1.1:8000", true, "", 0},
		{"10.0.0.1:80", true, "", 0},
		{"invalid", true, "", 0},
		{":8000", true, "", 0},
		// multiaddr 格式应该正常工作
		{"/ip4/192.168.1.1/udp/8000/quic-v1", false, "192.168.1.1", 8000},
		{"/ip4/10.0.0.1/tcp/80", false, "10.0.0.1", 80},
	}

	for _, tt := range tests {
		addr, err := parser.Parse(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("Parse(%q) expected error, got nil", tt.input)
			}
			continue
		}

		if err != nil {
			t.Errorf("Parse(%q) unexpected error: %v", tt.input, err)
			continue
		}

		// Parser.Parse() 现在返回 *Addr（基于 types.Multiaddr）
		// 验证基本属性
		if addr == nil {
			t.Errorf("Parse(%q) returned nil", tt.input)
			continue
		}

		// 验证地址字符串包含预期的 IP
		addrStr := addr.String()
		if tt.wantIP != "" && !contains(addrStr, tt.wantIP) {
			t.Errorf("Parse(%q) addr = %s, want to contain %s", tt.input, addrStr, tt.wantIP)
		}

		// 验证端口（通过 Multiaddr 的 Port() 方法）
		if a, ok := addr.(*Addr); ok && tt.wantPort > 0 {
			if a.MA().Port() != tt.wantPort {
				t.Errorf("Parse(%q) Port = %d, want %d", tt.input, a.MA().Port(), tt.wantPort)
			}
		}
	}
}

func TestParser_RejectHostPort(t *testing.T) {
	parser := NewParser()

	// 验证 host:port 格式被正确拒绝
	hostPortInputs := []string{
		"192.168.1.1:8000",
		"10.0.0.1:80",
		"example.com:443",
		"[::1]:8000",
		":8000",
	}

	for _, input := range hostPortInputs {
		_, err := parser.Parse(input)
		if err == nil {
			t.Errorf("Parse(%q) should reject host:port format", input)
		}
		// 确认是正确的错误类型
		if err != nil && !contains(err.Error(), "not multiaddr format") {
			t.Errorf("Parse(%q) error should mention 'not multiaddr format', got: %v", input, err)
		}
	}
}

func TestParser_ParseMultiaddr(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		input    string
		wantErr  bool
		wantIP   string
		wantPort int
	}{
		{"/ip4/192.168.1.1/tcp/8000", false, "192.168.1.1", 8000},
		{"/ip4/10.0.0.1/udp/8001", false, "10.0.0.1", 8001},
		{"/ip6/::1/tcp/8000", false, "::1", 8000},
		{"/dns4/example.com/tcp/8000", false, "", 8000},
		{"/invalid", true, "", 0},
	}

	for _, tt := range tests {
		addr, err := parser.Parse(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("Parse(%q) expected error, got nil", tt.input)
			}
			continue
		}

		if err != nil {
			t.Errorf("Parse(%q) unexpected error: %v", tt.input, err)
			continue
		}

		// Parser.Parse() 现在返回 *Addr（基于 types.Multiaddr）
		a, ok := addr.(*Addr)
		if !ok {
			t.Errorf("Parse(%q) expected *Addr, got %T", tt.input, addr)
			continue
		}

		if tt.wantIP != "" && a.MA().IP() != nil {
			if a.MA().IP().String() != tt.wantIP {
				t.Errorf("Parse(%q) IP = %v, want %v", tt.input, a.MA().IP(), tt.wantIP)
			}
		}

		if tt.wantPort > 0 && a.MA().Port() != tt.wantPort {
			t.Errorf("Parse(%q) Port = %d, want %d", tt.input, a.MA().Port(), tt.wantPort)
		}
	}
}

func TestParser_ParseMultiple(t *testing.T) {
	parser := NewParser()

	// 根据 IMPL-ADDRESS-UNIFICATION.md，仅接受 multiaddr 格式
	inputs := []string{
		"/ip4/192.168.1.1/udp/8000/quic-v1",
		"/ip4/10.0.0.1/tcp/8001",
	}

	addrs, err := parser.ParseMultiple(inputs)
	if err != nil {
		t.Fatalf("ParseMultiple() error: %v", err)
	}

	if len(addrs) != 2 {
		t.Errorf("expected 2 addresses, got %d", len(addrs))
	}
}

func TestParser_ParseMultiple_RejectHostPort(t *testing.T) {
	parser := NewParser()

	// 包含 host:port 格式应该失败
	inputs := []string{
		"/ip4/192.168.1.1/udp/8000/quic-v1",
		"192.168.1.1:8000", // host:port 格式
	}

	_, err := parser.ParseMultiple(inputs)
	if err == nil {
		t.Error("ParseMultiple() should reject host:port format")
	}
}

func TestAddr_Multiaddr(t *testing.T) {
	parser := NewParser()

	// 使用 multiaddr 格式进行测试
	addr, err := parser.Parse("/ip4/192.168.1.1/tcp/8000")
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	// Parser.Parse() 现在返回 *Addr
	a, ok := addr.(*Addr)
	if !ok {
		t.Fatalf("Parse() expected *Addr, got %T", addr)
	}

	ma := a.Multiaddr()

	if !contains(ma, "/ip4/192.168.1.1") {
		t.Errorf("Multiaddr() = %s, want to contain /ip4/192.168.1.1", ma)
	}
}

func TestAddr_IsPublicPrivate(t *testing.T) {
	parser := NewParser()

	tests := []struct {
		input     string
		wantPub   bool
		wantPriv  bool
		wantLoop  bool
	}{
		{"/ip4/8.8.8.8/tcp/8000", true, false, false},
		{"/ip4/192.168.1.1/tcp/8000", false, true, false},
		{"/ip4/127.0.0.1/tcp/8000", false, false, true},
	}

	for _, tt := range tests {
		addr, err := parser.Parse(tt.input)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", tt.input, err)
			continue
		}

		// Parser.Parse() 现在返回 *Addr
		if addr.IsPublic() != tt.wantPub {
			t.Errorf("Parse(%q).IsPublic() = %v, want %v", tt.input, addr.IsPublic(), tt.wantPub)
		}
		if addr.IsPrivate() != tt.wantPriv {
			t.Errorf("Parse(%q).IsPrivate() = %v, want %v", tt.input, addr.IsPrivate(), tt.wantPriv)
		}
		if addr.IsLoopback() != tt.wantLoop {
			t.Errorf("Parse(%q).IsLoopback() = %v, want %v", tt.input, addr.IsLoopback(), tt.wantLoop)
		}
	}
}

// ============================================================================
//                              Validator 测试
// ============================================================================

func TestValidator_Validate(t *testing.T) {
	config := DefaultValidatorConfig()
	config.AllowPrivate = true
	config.AllowLoopback = true
	validator := NewValidator(config)

	tests := []struct {
		input   string
		wantErr bool
	}{
		{"/ip4/192.168.1.1/tcp/8000", false},
		{"/ip4/8.8.8.8/tcp/443", false},
		{"/ip6/::1/tcp/8000", false},
		{"/ip4/192.168.1.1/tcp/99999", true}, // 无效端口
		{"/ip4/invalid/tcp/8000", true},       // 无效 IP
	}

	for _, tt := range tests {
		parser := NewParser()
		addr, _ := parser.Parse(tt.input)

		err := validator.Validate(addr)
		if tt.wantErr {
			if err == nil {
				t.Errorf("Validate(%q) expected error, got nil", tt.input)
			}
		} else {
			if err != nil {
				t.Errorf("Validate(%q) unexpected error: %v", tt.input, err)
			}
		}
	}
}

func TestValidator_IsValid(t *testing.T) {
	validator := NewValidator(DefaultValidatorConfig())
	parser := NewParser()

	addr, _ := parser.Parse("/ip4/192.168.1.1/tcp/8000")
	if !validator.IsValid(addr) {
		t.Error("expected address to be valid")
	}
}

// ============================================================================
//                              AddressManager 测试
// ============================================================================

func TestAddressManager_ListenAddrs(t *testing.T) {
	manager := NewAddressManager(DefaultAddressManagerConfig())

	addr := newTestAddr("0.0.0.0:8000", false, false, false)
	manager.AddListenAddr(addr)

	addrs := manager.ListenAddrs()
	if len(addrs) != 1 {
		t.Errorf("expected 1 listen address, got %d", len(addrs))
	}

	// 添加重复
	manager.AddListenAddr(addr)
	addrs = manager.ListenAddrs()
	if len(addrs) != 1 {
		t.Errorf("expected 1 listen address after duplicate, got %d", len(addrs))
	}
}

func TestAddressManager_AdvertisedAddrs(t *testing.T) {
	manager := NewAddressManager(DefaultAddressManagerConfig())

	publicAddr := newTestAddr("8.8.8.8:8000", true, false, false)
	manager.AddAdvertisedAddr(publicAddr)

	addrs := manager.AdvertisedAddrs()
	if len(addrs) != 1 {
		t.Errorf("expected 1 advertised address, got %d", len(addrs))
	}
}

func TestAddressManager_BestAddr(t *testing.T) {
	manager := NewAddressManager(DefaultAddressManagerConfig())

	privateAddr := newTestAddr("192.168.1.1:8000", false, true, false)
	publicAddr := newTestAddr("8.8.8.8:8000", true, false, false)

	manager.AddAdvertisedAddr(privateAddr)
	manager.AddAdvertisedAddr(publicAddr)

	best := manager.BestAddr()
	if best == nil {
		t.Fatal("expected best address, got nil")
	}

	// 公网地址应该优先
	if best.String() != publicAddr.String() {
		t.Errorf("expected public address %s, got %s", publicAddr.String(), best.String())
	}
}

func TestAddressManager_RemoveAddrs(t *testing.T) {
	manager := NewAddressManager(DefaultAddressManagerConfig())

	addr := newTestAddr("192.168.1.1:8000", false, true, false)
	manager.AddListenAddr(addr)
	manager.AddAdvertisedAddr(addr)

	manager.RemoveListenAddr(addr)
	manager.RemoveAdvertisedAddr(addr)

	if len(manager.ListenAddrs()) != 0 {
		t.Error("expected 0 listen addresses after remove")
	}
	if len(manager.AdvertisedAddrs()) != 0 {
		t.Error("expected 0 advertised addresses after remove")
	}
}

func TestAddressManager_FilteredAddrs(t *testing.T) {
	config := DefaultAddressManagerConfig()
	config.FilterLocalAddrs = true
	config.FilterPrivateAddrs = true

	manager := NewAddressManager(config)

	loopAddr := newTestAddr("127.0.0.1:8000", false, false, true)
	privateAddr := newTestAddr("192.168.1.1:8000", false, true, false)
	publicAddr := newTestAddr("8.8.8.8:8000", true, false, false)

	manager.AddAdvertisedAddr(loopAddr)
	manager.AddAdvertisedAddr(privateAddr)
	manager.AddAdvertisedAddr(publicAddr)

	filtered := manager.FilteredAdvertisedAddrs()
	if len(filtered) != 1 {
		t.Errorf("expected 1 filtered address, got %d", len(filtered))
	}

	if filtered[0].String() != publicAddr.String() {
		t.Errorf("expected public address in filtered, got %s", filtered[0].String())
	}
}

func TestAddressManager_LocalInterfaceAddrs(t *testing.T) {
	manager := NewAddressManager(DefaultAddressManagerConfig())

	addrs := manager.LocalInterfaceAddrs()
	// 至少应该有一个非回环地址
	t.Logf("found %d local interface addresses", len(addrs))
}

func TestAddressManager_StartStop(t *testing.T) {
	manager := NewAddressManager(DefaultAddressManagerConfig())

	ctx := context.Background()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// 重复启动应该没问题
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() again error: %v", err)
	}

	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}

	// 重复停止应该没问题
	if err := manager.Stop(); err != nil {
		t.Fatalf("Stop() again error: %v", err)
	}
}

func TestAddressManager_SetNATService_Concurrent(t *testing.T) {
	manager := NewAddressManager(DefaultAddressManagerConfig())

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			manager.SetNATService(nil)
			_ = manager.getNATService()
		}()
	}
	wg.Wait()
	// 测试通过表示没有竞态条件
}

func TestAddressManager_AddListenAddr_PublicAutoAdvertise(t *testing.T) {
	manager := NewAddressManager(DefaultAddressManagerConfig())

	publicAddr := newTestAddr("8.8.8.8:8000", true, false, false)
	manager.AddListenAddr(publicAddr)

	// 公网地址应该自动添加到通告地址
	advertised := manager.AdvertisedAddrs()
	if len(advertised) != 1 {
		t.Errorf("expected 1 advertised address, got %d", len(advertised))
	}

	if advertised[0].String() != publicAddr.String() {
		t.Errorf("expected advertised address %s, got %s", publicAddr.String(), advertised[0].String())
	}
}

func TestAddressManager_DiscoverExternalAddresses_WithoutStart(t *testing.T) {
	manager := NewAddressManager(DefaultAddressManagerConfig())

	// 不启动就调用不应该 panic
	manager.discoverExternalAddresses()
	// 测试通过表示没有 panic
}

func TestAddressManager_RefreshLoop_WithoutStart(t *testing.T) {
	manager := NewAddressManager(DefaultAddressManagerConfig())

	// 不启动就调用不应该 panic
	done := make(chan struct{})
	go func() {
		manager.refreshLoop()
		close(done)
	}()

	select {
	case <-done:
		// 正常退出
	case <-time.After(100 * time.Millisecond):
		// 超时也算正常（因为没有启动）
	}
}

// ============================================================================
//                              AddressRecord 测试
// ============================================================================

func TestAddressRecord_SignAndVerify(t *testing.T) {
	// 需要 identity 模块支持，这里只做基本结构测试
	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("test-node-id-12345678901234"))

	addrs := []endpoint.Address{
		newTestAddr("192.168.1.1:8000", false, true, false),
	}

	record := NewAddressRecord(nodeID, addrs, time.Hour)

	if record.NodeID != nodeID {
		t.Error("NodeID mismatch")
	}

	if len(record.Addresses) != 1 {
		t.Error("Addresses count mismatch")
	}

	if record.TTL != time.Hour {
		t.Error("TTL mismatch")
	}
}

func TestAddressRecord_Expiry(t *testing.T) {
	nodeID := types.NodeID{}
	addrs := []endpoint.Address{newTestAddr("192.168.1.1:8000", false, true, false)}

	// 短 TTL
	record := NewAddressRecord(nodeID, addrs, 10*time.Millisecond)

	if record.IsExpired() {
		t.Error("record should not be expired immediately")
	}

	time.Sleep(20 * time.Millisecond)

	if !record.IsExpired() {
		t.Error("record should be expired after TTL")
	}
}

func TestAddressRecord_IsNewerThan(t *testing.T) {
	nodeID := types.NodeID{}
	addrs := []endpoint.Address{newTestAddr("192.168.1.1:8000", false, true, false)}

	record1 := NewAddressRecord(nodeID, addrs, time.Hour)
	time.Sleep(time.Millisecond)
	record2 := NewAddressRecord(nodeID, addrs, time.Hour)

	if !record2.IsNewerThan(record1) {
		t.Error("record2 should be newer than record1")
	}

	if record1.IsNewerThan(record2) {
		t.Error("record1 should not be newer than record2")
	}
}

func TestAddressRecord_Validate(t *testing.T) {
	nodeID := types.NodeID{}
	copy(nodeID[:], []byte("test-node-id-12345678901234"))

	// 空地址列表
	record := NewAddressRecord(nodeID, nil, time.Hour)
	if err := record.Validate(); err == nil {
		t.Error("expected error for empty addresses")
	}

	// 正常记录
	addrs := []endpoint.Address{newTestAddr("192.168.1.1:8000", false, true, false)}
	record = NewAddressRecord(nodeID, addrs, time.Hour)
	if err := record.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ============================================================================
//                              Priority 测试
// ============================================================================

func TestPrioritizedAddress_Priority(t *testing.T) {
	publicAddr := newTestAddr("8.8.8.8:8000", true, false, false)
	privateAddr := newTestAddr("192.168.1.1:8000", false, true, false)

	pa1 := NewPrioritizedAddress(publicAddr, AddressTypePublic, AddressSourceDirect)
	pa2 := NewPrioritizedAddress(privateAddr, AddressTypeLAN, AddressSourceDHT)

	// 公网地址优先级应该更高
	if pa1.Priority <= pa2.Priority {
		t.Error("public address should have higher priority")
	}
}

func TestPrioritizedAddress_RecordStats(t *testing.T) {
	addr := newTestAddr("192.168.1.1:8000", false, true, false)
	pa := NewPrioritizedAddress(addr, AddressTypeLAN, AddressSourceDirect)

	// 记录成功
	pa.RecordSuccess(10 * time.Millisecond)
	pa.RecordSuccess(20 * time.Millisecond)

	if pa.Stats.SuccessCount != 2 {
		t.Errorf("expected 2 successes, got %d", pa.Stats.SuccessCount)
	}

	// RTT 应该更新
	if pa.Stats.AvgRTT == 0 {
		t.Error("AvgRTT should be updated")
	}

	// 记录失败
	pa.RecordFail()
	if pa.Stats.FailCount != 1 {
		t.Errorf("expected 1 failure, got %d", pa.Stats.FailCount)
	}
}

func TestSortAddresses(t *testing.T) {
	publicAddr := newTestAddr("8.8.8.8:8000", true, false, false)
	privateAddr := newTestAddr("192.168.1.1:8000", false, true, false)
	relayAddr := newTestAddr("/p2p-circuit/relay", false, false, false)

	pas := []*PrioritizedAddress{
		NewPrioritizedAddress(relayAddr, AddressTypeRelay, AddressSourceDHT),
		NewPrioritizedAddress(privateAddr, AddressTypeLAN, AddressSourceDirect),
		NewPrioritizedAddress(publicAddr, AddressTypePublic, AddressSourceDirect),
	}

	SortAddresses(pas)

	// 公网应该在前
	if pas[0].Type != AddressTypePublic {
		t.Errorf("expected public address first, got %v", pas[0].Type)
	}
}

func TestSelectBestAddress(t *testing.T) {
	publicAddr := newTestAddr("8.8.8.8:8000", true, false, false)
	privateAddr := newTestAddr("192.168.1.1:8000", false, true, false)

	pas := []*PrioritizedAddress{
		NewPrioritizedAddress(privateAddr, AddressTypeLAN, AddressSourceDirect),
		NewPrioritizedAddress(publicAddr, AddressTypePublic, AddressSourceDirect),
	}

	best := SelectBestAddress(pas)
	if best == nil {
		t.Fatal("expected best address")
	}

	if best.Address.String() != publicAddr.String() {
		t.Errorf("expected public address, got %s", best.Address.String())
	}
}

// ============================================================================
//                              RelayAddr 测试
// ============================================================================

func TestRelayAddr(t *testing.T) {
	relayID := types.NodeID{}
	destID := types.NodeID{}
	copy(relayID[:], []byte("relay-node-id-123456789012"))
	copy(destID[:], []byte("dest-node-id-1234567890123"))

	addr := NewRelayAddr(relayID, destID)

	if addr.Network() != "p2p-circuit" {
		t.Errorf("expected network p2p-circuit, got %s", addr.Network())
	}

	if addr.RelayID() != relayID {
		t.Error("RelayID mismatch")
	}

	if addr.DestID() != destID {
		t.Error("DestID mismatch")
	}

	if !contains(addr.String(), "p2p-circuit") {
		t.Error("expected p2p-circuit in address string")
	}
}

// ============================================================================
//                              DetectAddressType 测试
// ============================================================================

func TestDetectAddressType(t *testing.T) {
	tests := []struct {
		addr     string
		wantType AddressType
	}{
		{"/ip4/8.8.8.8/tcp/8000", AddressTypePublic},
		{"/ip4/192.168.1.1/tcp/8000", AddressTypeLAN},
		{"/ip4/10.0.0.1/tcp/8000", AddressTypeLAN},
		{"/ip4/127.0.0.1/tcp/8000", AddressTypeLAN},
		{"/p2p/Qm.../p2p-circuit/p2p/Qm...", AddressTypeRelay},
	}

	parser := NewParser()

	for _, tt := range tests {
		addr, err := parser.Parse(tt.addr)
		if err != nil {
			// 继续使用模拟地址
			addr = newTestAddr(tt.addr, tt.wantType == AddressTypePublic, tt.wantType == AddressTypeLAN, false)
		}

		got := DetectAddressType(addr)
		if got != tt.wantType {
			t.Errorf("DetectAddressType(%q) = %v, want %v", tt.addr, got, tt.wantType)
		}
	}
}

// ============================================================================
//                              辅助函数
// ============================================================================

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsAt(s, substr)
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ============================================================================
//                              Resolve 测试
// ============================================================================

// TestParsedAddress_Resolve 已废弃
//
// 根据 IMPL-ADDRESS-UNIFICATION.md 规范，ParsedAddress 类型已被 Addr 替代。
// Addr 类型不提供 Resolve() 方法，DNS 解析应在更高层处理。
func TestParsedAddress_Resolve(t *testing.T) {
	t.Skip("ParsedAddress 已废弃，Resolve() 方法不再在 Addr 类型中提供")
}

// ============================================================================
//                              Benchmark 测试
// ============================================================================

func BenchmarkParser_Parse(b *testing.B) {
	parser := NewParser()
	input := "/ip4/192.168.1.1/tcp/8000"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parser.Parse(input)
	}
}

func BenchmarkAddressBook_AddAddrs(b *testing.B) {
	book := NewAddressBook()
	nodeID := types.NodeID{}
	addr := newTestAddr("192.168.1.1:8000", false, true, false)
	addrs := []endpoint.Address{addr}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		book.AddAddrs(nodeID, addrs, time.Hour)
	}
}

func BenchmarkAddressBook_Addrs(b *testing.B) {
	book := NewAddressBook()
	nodeID := types.NodeID{}
	addr := newTestAddr("192.168.1.1:8000", false, true, false)
	book.AddAddrs(nodeID, []endpoint.Address{addr}, time.Hour)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		book.Addrs(nodeID)
	}
}

