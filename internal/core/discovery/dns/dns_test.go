package dns

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	discoveryif "github.com/dep2p/go-dep2p/pkg/interfaces/discovery"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              解析器测试
// ============================================================================

func TestParseDNSAddr_ValidIPv4(t *testing.T) {
	// 使用有效的 Base58 编码 NodeID（需要能被 types.ParseNodeID 解析）
	// 使用 types.NodeID 生成有效的 Base58 字符串
	var nodeID types.NodeID
	for i := 0; i < 32; i++ {
		nodeID[i] = byte(i + 1)
	}
	nodeIDBase58 := nodeID.String()
	record := "dnsaddr=/ip4/1.2.3.4/tcp/4001/p2p/" + nodeIDBase58

	peer, nestedDomain, err := ParseDNSAddr(record)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if nestedDomain != "" {
		t.Errorf("期望无嵌套域名，得到: %s", nestedDomain)
	}

	if peer == nil {
		t.Fatal("期望返回 peer，得到 nil")
	}

	if !strings.HasPrefix(peer.Addrs[0].String(), "/ip4/1.2.3.4/tcp/4001") {
		t.Errorf("地址不正确: %v", peer.Addrs)
	}

	if peer.Source != "dns" {
		t.Errorf("Source 不正确: %s", peer.Source)
	}

	// 验证 NodeID 正确解析
	if !peer.ID.Equal(nodeID) {
		t.Errorf("NodeID 不正确: got %s, want %s", peer.ID.String(), nodeIDBase58)
	}
}

func TestParseDNSAddr_ValidIPv6(t *testing.T) {
	// 使用有效的 Base58 编码 NodeID
	var nodeID types.NodeID
	for i := 0; i < 32; i++ {
		nodeID[i] = byte(i + 1)
	}
	nodeIDBase58 := nodeID.String()
	record := "dnsaddr=/ip6/::1/tcp/4001/p2p/" + nodeIDBase58

	peer, nestedDomain, err := ParseDNSAddr(record)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if nestedDomain != "" {
		t.Errorf("期望无嵌套域名，得到: %s", nestedDomain)
	}

	if peer == nil {
		t.Fatal("期望返回 peer，得到 nil")
	}

	if !strings.HasPrefix(peer.Addrs[0].String(), "/ip6/::1/tcp/4001") {
		t.Errorf("地址不正确: %v", peer.Addrs)
	}

	// 验证 NodeID 正确解析
	if !peer.ID.Equal(nodeID) {
		t.Errorf("NodeID 不正确: got %s, want %s", peer.ID.String(), nodeIDBase58)
	}
}

func TestParseDNSAddr_NestedDNSAddr(t *testing.T) {
	record := "dnsaddr=/dnsaddr/us-east.bootstrap.dep2p.network"

	peer, nestedDomain, err := ParseDNSAddr(record)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	if peer != nil {
		t.Errorf("期望 peer 为 nil，得到: %+v", peer)
	}

	if nestedDomain != "us-east.bootstrap.dep2p.network" {
		t.Errorf("嵌套域名不正确: %s", nestedDomain)
	}
}

func TestParseDNSAddr_InvalidPrefix(t *testing.T) {
	record := "invalid=/ip4/1.2.3.4/tcp/4001/p2p/5Q2STWvBDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN"

	_, _, err := ParseDNSAddr(record)
	if err == nil {
		t.Fatal("期望返回错误")
	}
	if err != ErrInvalidDNSAddr {
		t.Errorf("期望 ErrInvalidDNSAddr，得到: %v", err)
	}
}

func TestParseDNSAddr_EmptyRecord(t *testing.T) {
	record := "dnsaddr="

	_, _, err := ParseDNSAddr(record)
	if err == nil {
		t.Fatal("期望返回错误")
	}
}

func TestParseDNSAddr_MissingNodeID(t *testing.T) {
	record := "dnsaddr=/ip4/1.2.3.4/tcp/4001"

	_, _, err := ParseDNSAddr(record)
	if err == nil {
		t.Fatal("期望返回错误")
	}
	if !strings.Contains(err.Error(), "missing /p2p/") {
		t.Errorf("错误信息不正确: %v", err)
	}
}

// ============================================================================
//                              域名验证测试
// ============================================================================

func TestValidateDomain_Valid(t *testing.T) {
	validDomains := []string{
		"example.com",
		"bootstrap.dep2p.network",
		"us-east-1.example.com",
		"_dnsaddr.example.com",
	}

	for _, domain := range validDomains {
		if err := ValidateDomain(domain); err != nil {
			t.Errorf("域名 %s 应该有效，得到错误: %v", domain, err)
		}
	}
}

func TestValidateDomain_Invalid(t *testing.T) {
	invalidDomains := []string{
		"",
		"-example.com",
		"exa mple.com",
	}

	for _, domain := range invalidDomains {
		if err := ValidateDomain(domain); err == nil {
			t.Errorf("域名 %s 应该无效", domain)
		}
	}
}

func TestValidateDomain_TooLong(t *testing.T) {
	// 创建超长域名
	longDomain := strings.Repeat("a", 64) + ".com"

	if err := ValidateDomain(longDomain); err == nil {
		t.Error("应该拒绝标签超过 63 字符的域名")
	}
}

// ============================================================================
//                              解析器缓存测试
// ============================================================================

func TestResolver_Cache(t *testing.T) {
	config := DefaultResolverConfig()
	config.CacheTTL = 1 * time.Second

	resolver := NewResolver(config)

	// 模拟缓存设置
	domain := "_dnsaddr.test.com"
	peers := []discoveryif.PeerInfo{
		{Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/1.2.3.4/tcp/4001")}, Source: "dns"},
	}
	resolver.setCache(domain, peers)

	// 验证缓存命中
	cached, ok := resolver.getFromCache(domain)
	if !ok {
		t.Fatal("期望缓存命中")
	}
	if len(cached) != 1 {
		t.Errorf("缓存的节点数不正确: %d", len(cached))
	}

	// 等待缓存过期
	time.Sleep(1500 * time.Millisecond)

	// 验证缓存过期
	_, ok = resolver.getFromCache(domain)
	if ok {
		t.Error("期望缓存过期")
	}
}

func TestResolver_ClearCache(t *testing.T) {
	config := DefaultResolverConfig()
	resolver := NewResolver(config)

	// 设置缓存
	resolver.setCache("domain1", []discoveryif.PeerInfo{{Source: "dns"}})
	resolver.setCache("domain2", []discoveryif.PeerInfo{{Source: "dns"}})

	// 清除缓存
	resolver.ClearCache()

	// 验证缓存已清空
	if _, ok := resolver.getFromCache("domain1"); ok {
		t.Error("domain1 应该被清除")
	}
	if _, ok := resolver.getFromCache("domain2"); ok {
		t.Error("domain2 应该被清除")
	}
}

func TestResolver_ClearExpiredCache(t *testing.T) {
	config := DefaultResolverConfig()
	config.CacheTTL = 100 * time.Millisecond
	resolver := NewResolver(config)

	// 设置缓存
	resolver.setCache("domain1", []discoveryif.PeerInfo{{Source: "dns"}})

	// 等待过期
	time.Sleep(150 * time.Millisecond)

	// 添加新缓存
	resolver.setCache("domain2", []discoveryif.PeerInfo{{Source: "dns"}})

	// 清除过期缓存
	resolver.ClearExpiredCache()

	// domain1 应该被清除，domain2 应该保留
	if _, ok := resolver.getFromCache("domain1"); ok {
		t.Error("domain1 应该被清除")
	}
	if _, ok := resolver.getFromCache("domain2"); !ok {
		t.Error("domain2 应该保留")
	}
}

// ============================================================================
//                              发现器测试
// ============================================================================

func TestDiscoverer_Config(t *testing.T) {
	config := DefaultDiscovererConfig()

	if config.Timeout != DefaultTimeout {
		t.Errorf("默认超时不正确: %v", config.Timeout)
	}
	if config.MaxDepth != DefaultMaxDepth {
		t.Errorf("默认深度不正确: %d", config.MaxDepth)
	}
	if config.CacheTTL != DefaultCacheTTL {
		t.Errorf("默认缓存 TTL 不正确: %v", config.CacheTTL)
	}
}

func TestDiscoverer_Domains(t *testing.T) {
	config := DefaultDiscovererConfig()
	config.Domains = []string{"example.com", "test.com"}

	discoverer := NewDiscoverer(config)

	domains := discoverer.Domains()
	if len(domains) != 2 {
		t.Errorf("域名数量不正确: %d", len(domains))
	}
	if domains[0] != "example.com" {
		t.Errorf("第一个域名不正确: %s", domains[0])
	}
}

func TestDiscoverer_AddRemoveDomain(t *testing.T) {
	config := DefaultDiscovererConfig()
	discoverer := NewDiscoverer(config)

	// 添加域名
	err := discoverer.AddDomain("example.com")
	if err != nil {
		t.Fatalf("添加域名失败: %v", err)
	}

	if len(discoverer.Domains()) != 1 {
		t.Error("域名应该被添加")
	}

	// 添加无效域名
	err = discoverer.AddDomain("")
	if err == nil {
		t.Error("应该拒绝无效域名")
	}

	// 移除域名
	discoverer.RemoveDomain("example.com")
	if len(discoverer.Domains()) != 0 {
		t.Error("域名应该被移除")
	}
}

func TestDiscoverer_StartStop(t *testing.T) {
	config := DefaultDiscovererConfig()
	config.RefreshInterval = 100 * time.Millisecond
	discoverer := NewDiscoverer(config)

	ctx := context.Background()
	err := discoverer.Start(ctx)
	if err != nil {
		t.Fatalf("启动失败: %v", err)
	}

	// 等待一个刷新周期
	time.Sleep(150 * time.Millisecond)

	err = discoverer.Stop()
	if err != nil {
		t.Fatalf("停止失败: %v", err)
	}
}

func TestDiscoverer_AllPeers(t *testing.T) {
	config := DefaultDiscovererConfig()
	discoverer := NewDiscoverer(config)

	// 模拟添加节点 - 使用不同的 NodeID
	var nodeID1, nodeID2 [32]byte
	nodeID1[0] = 1
	nodeID2[0] = 2

	discoverer.peersMu.Lock()
	discoverer.peers["domain1"] = []discoveryif.PeerInfo{
		{ID: nodeID1, Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/1.2.3.4/tcp/4001")}, Source: "dns"},
	}
	discoverer.peers["domain2"] = []discoveryif.PeerInfo{
		{ID: nodeID2, Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/5.6.7.8/tcp/4001")}, Source: "dns"},
	}
	discoverer.peersMu.Unlock()

	peers := discoverer.AllPeers()
	if len(peers) != 2 {
		t.Errorf("节点数量不正确: %d", len(peers))
	}
}

func TestDiscoverer_PeersForDomain(t *testing.T) {
	config := DefaultDiscovererConfig()
	discoverer := NewDiscoverer(config)

	// 模拟添加节点
	discoverer.peersMu.Lock()
	discoverer.peers["domain1"] = []discoveryif.PeerInfo{
		{Addrs: []types.Multiaddr{types.MustParseMultiaddr("/ip4/1.2.3.4/tcp/4001")}, Source: "dns"},
	}
	discoverer.peersMu.Unlock()

	peers := discoverer.PeersForDomain("domain1")
	if len(peers) != 1 {
		t.Errorf("节点数量不正确: %d", len(peers))
	}

	peers = discoverer.PeersForDomain("nonexistent")
	if len(peers) != 0 {
		t.Error("不存在的域名应该返回空列表")
	}
}

func TestDiscoverer_Stats(t *testing.T) {
	config := DefaultDiscovererConfig()
	config.Domains = []string{"example.com", "test.com"}
	discoverer := NewDiscoverer(config)

	// 模拟添加节点
	discoverer.peersMu.Lock()
	discoverer.peers["example.com"] = []discoveryif.PeerInfo{
		{Source: "dns"},
		{Source: "dns"},
	}
	discoverer.peers["test.com"] = []discoveryif.PeerInfo{
		{Source: "dns"},
	}
	discoverer.peersMu.Unlock()

	stats := discoverer.Stats()
	if stats.TotalDomains != 2 {
		t.Errorf("域名总数不正确: %d", stats.TotalDomains)
	}
	if stats.TotalPeers != 3 {
		t.Errorf("节点总数不正确: %d", stats.TotalPeers)
	}
	if stats.DomainStats["example.com"] != 2 {
		t.Errorf("example.com 节点数不正确: %d", stats.DomainStats["example.com"])
	}
}

// ============================================================================
//                              域名规范化测试
// ============================================================================

func TestResolver_NormalizeDomain(t *testing.T) {
	resolver := NewResolver(DefaultResolverConfig())

	tests := []struct {
		input    string
		expected string
	}{
		{"example.com", "_dnsaddr.example.com"},
		{"_dnsaddr.example.com", "_dnsaddr.example.com"},
		{"example.com.", "_dnsaddr.example.com"},
		{"_dnsaddr.example.com.", "_dnsaddr.example.com"},
	}

	for _, tt := range tests {
		result := resolver.normalizeDomain(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeDomain(%s) = %s, expected %s", tt.input, result, tt.expected)
		}
	}
}

// ============================================================================
//                              DiscoverPeers 测试
// ============================================================================

func TestDiscoverer_DiscoverPeers(t *testing.T) {
	config := DefaultDiscovererConfig()
	config.Domains = []string{"example.com"}
	discoverer := NewDiscoverer(config)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// DiscoverPeers 会尝试 DNS 解析，可能失败，但不应该 panic
	ch, err := discoverer.DiscoverPeers(ctx, "dns")
	if err != nil {
		t.Fatalf("DiscoverPeers 返回错误: %v", err)
	}

	// 消费通道（可能为空或有错误）
	count := 0
	for range ch {
		count++
	}
	// 不检查 count，因为 DNS 解析可能失败
}

// ============================================================================
//                              goroutine 安全测试
// ============================================================================

func TestDiscoverer_RefreshLoopWithoutStart(t *testing.T) {
	config := DefaultDiscovererConfig()
	discoverer := NewDiscoverer(config)

	// 在未启动时调用 refreshLoop 应该立即返回而不 panic
	discoverer.wg.Add(1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		discoverer.refreshLoop()
	}()

	select {
	case <-done:
		// 成功
	case <-time.After(time.Second):
		t.Fatal("refreshLoop 应该立即返回")
	}
}

func TestDiscoverer_RefreshDomainWithoutStart(t *testing.T) {
	config := DefaultDiscovererConfig()
	discoverer := NewDiscoverer(config)

	// 在未启动时调用 refreshDomain 应该立即返回而不 panic
	done := make(chan struct{})
	go func() {
		defer close(done)
		discoverer.refreshDomain("example.com")
	}()

	select {
	case <-done:
		// 成功
	case <-time.After(time.Second):
		t.Fatal("refreshDomain 应该立即返回")
	}
}

func TestDiscoverer_StartMultiple(t *testing.T) {
	config := DefaultDiscovererConfig()
	config.RefreshInterval = 1 * time.Hour // 避免后台刷新
	discoverer := NewDiscoverer(config)

	ctx := context.Background()

	// 第一次启动
	err := discoverer.Start(ctx)
	if err != nil {
		t.Fatalf("第一次启动失败: %v", err)
	}

	// 第二次启动应该是幂等的
	err = discoverer.Start(ctx)
	if err != nil {
		t.Fatalf("第二次启动失败: %v", err)
	}

	// 停止
	err = discoverer.Stop()
	if err != nil {
		t.Fatalf("停止失败: %v", err)
	}
}

func TestDiscoverer_StopWithoutStart(t *testing.T) {
	config := DefaultDiscovererConfig()
	discoverer := NewDiscoverer(config)

	// 未启动时停止应该是安全的
	err := discoverer.Stop()
	if err != nil {
		t.Fatalf("未启动时停止应该不报错: %v", err)
	}
}

func TestDiscoverer_StopMultiple(t *testing.T) {
	config := DefaultDiscovererConfig()
	config.RefreshInterval = 1 * time.Hour
	discoverer := NewDiscoverer(config)

	ctx := context.Background()
	_ = discoverer.Start(ctx)

	// 多次停止应该是幂等的
	err := discoverer.Stop()
	if err != nil {
		t.Fatalf("第一次停止失败: %v", err)
	}

	err = discoverer.Stop()
	if err != nil {
		t.Fatalf("第二次停止失败: %v", err)
	}
}

func TestDiscoverer_AddRemoveDomain_Concurrent(t *testing.T) {
	config := DefaultDiscovererConfig()
	discoverer := NewDiscoverer(config)

	var wg sync.WaitGroup

	// 并发添加域名
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			domain := "test" + string(rune('0'+id)) + ".example.com"
			_ = discoverer.AddDomain(domain)
		}(i)
	}

	// 并发读取域名
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = discoverer.Domains()
		}()
	}

	// 并发移除域名
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			domain := "test" + string(rune('0'+id)) + ".example.com"
			discoverer.RemoveDomain(domain)
		}(i)
	}

	wg.Wait()
}

func TestDiscoverer_StatsConcurrent(t *testing.T) {
	config := DefaultDiscovererConfig()
	config.Domains = []string{"example.com", "test.com"}
	discoverer := NewDiscoverer(config)

	var wg sync.WaitGroup

	// 并发读取统计
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = discoverer.Stats()
		}()
	}

	// 并发添加节点
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			discoverer.peersMu.Lock()
			discoverer.peers["domain"+string(rune('0'+id))] = []discoveryif.PeerInfo{
				{Source: "dns"},
			}
			discoverer.peersMu.Unlock()
		}(i)
	}

	wg.Wait()
}

