package dht

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
)

// ============================================================================
// rateLimiter 速率限制器测试
// ============================================================================

// TestRateLimiter_New 测试创建速率限制器
func TestRateLimiter_New(t *testing.T) {
	limiter := newRateLimiter(10, time.Minute)
	
	assert.NotNil(t, limiter)
	assert.Equal(t, 10, limiter.limit)
	assert.Equal(t, time.Minute, limiter.window)
	
	t.Log("✅ 速率限制器创建成功")
}

// TestRateLimiter_AllowFirstRequest 测试第一次请求
func TestRateLimiter_AllowFirstRequest(t *testing.T) {
	limiter := newRateLimiter(10, time.Minute)
	
	sender := types.NodeID("peer-1")
	allowed := limiter.Allow(sender)
	
	assert.True(t, allowed, "第一次请求应该被允许")
	
	t.Log("✅ 第一次请求允许通过")
}

// TestRateLimiter_AllowMultipleRequests 测试多次请求
func TestRateLimiter_AllowMultipleRequests(t *testing.T) {
	limiter := newRateLimiter(5, time.Minute)
	
	sender := types.NodeID("peer-1")
	
	// 前5次应该都允许
	for i := 0; i < 5; i++ {
		allowed := limiter.Allow(sender)
		assert.True(t, allowed, "第 %d 次请求应该被允许", i+1)
	}
	
	t.Log("✅ 多次请求在限制内正常通过")
}

// TestRateLimiter_ExceedLimit 测试超过速率限制
func TestRateLimiter_ExceedLimit(t *testing.T) {
	limiter := newRateLimiter(3, time.Minute)
	
	sender := types.NodeID("peer-1")
	
	// 前3次允许
	for i := 0; i < 3; i++ {
		allowed := limiter.Allow(sender)
		assert.True(t, allowed)
	}
	
	// 第4次应该被拒绝
	allowed := limiter.Allow(sender)
	assert.False(t, allowed, "超过限制的请求应该被拒绝")
	
	t.Log("✅ 超过速率限制正确拒绝")
}

// TestRateLimiter_DifferentSenders 测试不同发送者独立计数
func TestRateLimiter_DifferentSenders(t *testing.T) {
	limiter := newRateLimiter(2, time.Minute)
	
	sender1 := types.NodeID("peer-1")
	sender2 := types.NodeID("peer-2")
	
	// sender1 的2次请求
	assert.True(t, limiter.Allow(sender1))
	assert.True(t, limiter.Allow(sender1))
	
	// sender1 的第3次被拒绝
	assert.False(t, limiter.Allow(sender1))
	
	// sender2 的请求应该独立计数，不受sender1影响
	assert.True(t, limiter.Allow(sender2))
	assert.True(t, limiter.Allow(sender2))
	
	t.Log("✅ 不同发送者独立计数")
}

// TestRateLimiter_WindowExpiry 测试时间窗口过期
func TestRateLimiter_WindowExpiry(t *testing.T) {
	// 使用非常短的时间窗口以便测试
	limiter := newRateLimiter(2, 100*time.Millisecond)
	
	sender := types.NodeID("peer-1")
	
	// 前2次允许
	assert.True(t, limiter.Allow(sender))
	assert.True(t, limiter.Allow(sender))
	
	// 第3次被拒绝
	assert.False(t, limiter.Allow(sender))
	
	// 等待时间窗口过期
	time.Sleep(150 * time.Millisecond)
	
	// 窗口过期后应该重新允许
	assert.True(t, limiter.Allow(sender), "时间窗口过期后应该重新允许")
	
	t.Log("✅ 时间窗口过期后正确重置")
}

// ============================================================================
// extractIP IP 提取测试
// ============================================================================

// TestExtractIP_IPv4 测试提取 IPv4 地址
func TestExtractIP_IPv4(t *testing.T) {
	testCases := []struct {
		addr     string
		expected string
	}{
		{"/ip4/192.168.1.1/tcp/4001", "192.168.1.1"},
		{"/ip4/8.8.8.8/tcp/53", "8.8.8.8"},
		{"/ip4/127.0.0.1/tcp/8080", "127.0.0.1"},
		{"/ip4/10.0.0.1/tcp/9000", "10.0.0.1"},
	}
	
	for _, tc := range testCases {
		ip := extractIP(tc.addr)
		assert.NotNil(t, ip, "地址 %s 应该能提取IP", tc.addr)
		assert.Equal(t, tc.expected, ip.String(), "地址 %s 的IP不匹配", tc.addr)
	}
	
	t.Log("✅ IPv4 地址提取正确")
}

// TestExtractIP_IPv6 测试提取 IPv6 地址
func TestExtractIP_IPv6(t *testing.T) {
	testCases := []struct {
		addr     string
		expected string
	}{
		{"/ip6/::1/tcp/4001", "::1"},
		{"/ip6/2001:db8::1/tcp/8080", "2001:db8::1"},
		{"/ip6/fe80::1/tcp/9000", "fe80::1"},
	}
	
	for _, tc := range testCases {
		ip := extractIP(tc.addr)
		assert.NotNil(t, ip, "地址 %s 应该能提取IP", tc.addr)
		assert.Equal(t, tc.expected, ip.String(), "地址 %s 的IP不匹配", tc.addr)
	}
	
	t.Log("✅ IPv6 地址提取正确")
}

// TestExtractIP_Invalid 测试提取无效地址
func TestExtractIP_Invalid(t *testing.T) {
	invalidAddrs := []string{
		"",
		"invalid",
		"/tcp/4001",
		"/ip4/tcp/4001",
		"192.168.1.1",
		"/ip4/999.999.999.999/tcp/4001",
	}
	
	for _, addr := range invalidAddrs {
		ip := extractIP(addr)
		assert.Nil(t, ip, "无效地址 %s 应该返回nil", addr)
	}
	
	t.Log("✅ 无效地址正确处理")
}

// ============================================================================
// isPrivateIP 私网IP检测测试
// ============================================================================

// TestIsPrivateIP_PrivateIPv4 测试私网 IPv4
func TestIsPrivateIP_PrivateIPv4(t *testing.T) {
	privateIPs := []string{
		"192.168.1.1",
		"192.168.0.1",
		"10.0.0.1",
		"10.255.255.255",
		"172.16.0.1",
		"172.31.255.255",
	}
	
	for _, ipStr := range privateIPs {
		ip := net.ParseIP(ipStr)
		assert.True(t, isPrivateIP(ip), "%s 应该被识别为私网IP", ipStr)
	}
	
	t.Log("✅ 私网IPv4正确识别")
}

// TestIsPrivateIP_PublicIPv4 测试公网 IPv4
func TestIsPrivateIP_PublicIPv4(t *testing.T) {
	publicIPs := []string{
		"8.8.8.8",
		"1.1.1.1",
		"216.58.217.46", // google.com
		"93.184.216.34", // example.com
	}
	
	for _, ipStr := range publicIPs {
		ip := net.ParseIP(ipStr)
		assert.False(t, isPrivateIP(ip), "%s 应该被识别为公网IP", ipStr)
	}
	
	t.Log("✅ 公网IPv4正确识别")
}

// TestIsPrivateIP_IPv6 测试 IPv6
func TestIsPrivateIP_IPv6(t *testing.T) {
	// IPv6 私网地址（ULA: Unique Local Address）
	privateIPv6 := []string{
		"fc00::1",
		"fd00::1",
	}
	
	for _, ipStr := range privateIPv6 {
		ip := net.ParseIP(ipStr)
		assert.True(t, isPrivateIP(ip), "%s 应该被识别为私网IP", ipStr)
	}
	
	// IPv6 公网地址
	publicIPv6 := []string{
		"2001:db8::1",
		"2606:2800:220:1:248:1893:25c8:1946", // example.com
	}
	
	for _, ipStr := range publicIPv6 {
		ip := net.ParseIP(ipStr)
		assert.False(t, isPrivateIP(ip), "%s 应该被识别为公网IP", ipStr)
	}
	
	t.Log("✅ IPv6地址正确识别")
}

// ============================================================================
// isRoutableAddr 地址可路由性测试
// ============================================================================

// TestIsRoutableAddr_PublicIPv4 测试公网IPv4可路由
func TestIsRoutableAddr_PublicIPv4(t *testing.T) {
	publicAddrs := []string{
		"/ip4/8.8.8.8/tcp/4001",
		"/ip4/1.1.1.1/tcp/8080",
	}
	
	for _, addr := range publicAddrs {
		// 公网地址无论 allowPrivate 设置如何都应该可路由
		assert.True(t, isRoutableAddr(addr, false), "%s 应该可路由（不允许私网）", addr)
		assert.True(t, isRoutableAddr(addr, true), "%s 应该可路由（允许私网）", addr)
	}
	
	t.Log("✅ 公网IPv4地址可路由")
}

// TestIsRoutableAddr_PrivateIPv4 测试私网IPv4
func TestIsRoutableAddr_PrivateIPv4(t *testing.T) {
	privateAddrs := []string{
		"/ip4/192.168.1.1/tcp/4001",
		"/ip4/10.0.0.1/tcp/8080",
		"/ip4/172.16.0.1/tcp/9000",
	}
	
	for _, addr := range privateAddrs {
		// 不允许私网时应该不可路由
		assert.False(t, isRoutableAddr(addr, false), "%s 不应该可路由（不允许私网）", addr)
		// 允许私网时应该可路由
		assert.True(t, isRoutableAddr(addr, true), "%s 应该可路由（允许私网）", addr)
	}
	
	t.Log("✅ 私网IPv4地址根据配置正确判断")
}

// TestIsRoutableAddr_Loopback 测试回环地址
func TestIsRoutableAddr_Loopback(t *testing.T) {
	loopbackAddrs := []string{
		"/ip4/127.0.0.1/tcp/4001",
		"/ip6/::1/tcp/8080",
	}
	
	for _, addr := range loopbackAddrs {
		// 回环地址无论配置如何都不可路由
		assert.False(t, isRoutableAddr(addr, false), "%s 不应该可路由（回环地址）", addr)
		assert.False(t, isRoutableAddr(addr, true), "%s 不应该可路由（回环地址，即使允许私网）", addr)
	}
	
	t.Log("✅ 回环地址正确拒绝")
}

// TestIsRoutableAddr_LinkLocal 测试链路本地地址
func TestIsRoutableAddr_LinkLocal(t *testing.T) {
	linkLocalAddrs := []string{
		"/ip4/169.254.1.1/tcp/4001", // IPv4 link-local
		"/ip6/fe80::1/tcp/8080",     // IPv6 link-local
	}
	
	for _, addr := range linkLocalAddrs {
		// 链路本地地址无论配置如何都不可路由
		assert.False(t, isRoutableAddr(addr, false), "%s 不应该可路由（链路本地）", addr)
		assert.False(t, isRoutableAddr(addr, true), "%s 不应该可路由（链路本地，即使允许私网）", addr)
	}
	
	t.Log("✅ 链路本地地址正确拒绝")
}

// TestIsRoutableAddr_Invalid 测试无效地址
func TestIsRoutableAddr_Invalid(t *testing.T) {
	invalidAddrs := []string{
		"",
		"invalid-address",
		"/tcp/4001",
		"192.168.1.1", // 没有multiaddr格式
	}
	
	for _, addr := range invalidAddrs {
		assert.False(t, isRoutableAddr(addr, false), "无效地址 %s 不应该可路由", addr)
		assert.False(t, isRoutableAddr(addr, true), "无效地址 %s 不应该可路由", addr)
	}
	
	t.Log("✅ 无效地址正确拒绝")
}

// ============================================================================
// filterValidAddrs 地址过滤测试
// ============================================================================

// TestFilterValidAddrs 测试过滤有效地址
func TestFilterValidAddrs(t *testing.T) {
	h := &Handler{
		dht: &DHT{
			config: &Config{
				AllowPrivateAddrs: false,
			},
		},
	}
	
	addrs := []string{
		"/ip4/8.8.8.8/tcp/4001",        // 公网，应该保留
		"/ip4/192.168.1.1/tcp/4001",    // 私网，应该过滤
		"/ip4/127.0.0.1/tcp/4001",      // 回环，应该过滤
		"/ip4/1.1.1.1/tcp/8080",        // 公网，应该保留
		"invalid",                       // 无效，应该过滤
	}
	
	filtered := h.filterValidAddrs(addrs, false)
	
	assert.Equal(t, 2, len(filtered), "应该保留2个公网地址")
	assert.Contains(t, filtered, "/ip4/8.8.8.8/tcp/4001")
	assert.Contains(t, filtered, "/ip4/1.1.1.1/tcp/8080")
	
	t.Log("✅ 地址过滤功能正常")
}

// TestFilterValidAddrs_AllowPrivate 测试允许私网地址
func TestFilterValidAddrs_AllowPrivate(t *testing.T) {
	h := &Handler{
		dht: &DHT{
			config: &Config{
				AllowPrivateAddrs: true,
			},
		},
	}
	
	addrs := []string{
		"/ip4/8.8.8.8/tcp/4001",        // 公网
		"/ip4/192.168.1.1/tcp/4001",    // 私网（允许）
		"/ip4/127.0.0.1/tcp/4001",      // 回环（仍然过滤）
		"/ip4/10.0.0.1/tcp/8080",       // 私网（允许）
	}
	
	filtered := h.filterValidAddrs(addrs, true)
	
	assert.Equal(t, 3, len(filtered), "应该保留3个地址（2公网+1私网，回环被过滤）")
	assert.Contains(t, filtered, "/ip4/8.8.8.8/tcp/4001")
	assert.Contains(t, filtered, "/ip4/192.168.1.1/tcp/4001")
	assert.Contains(t, filtered, "/ip4/10.0.0.1/tcp/8080")
	
	t.Log("✅ 允许私网地址时过滤正确")
}

// ============================================================================
// 综合场景测试
// ============================================================================

// TestRateLimiter_ConcurrentAccess 测试并发访问
func TestRateLimiter_ConcurrentAccess(t *testing.T) {
	limiter := newRateLimiter(100, time.Minute)
	
	sender := types.NodeID("peer-1")
	
	// 并发发送50个请求
	done := make(chan bool, 50)
	for i := 0; i < 50; i++ {
		go func() {
			limiter.Allow(sender)
			done <- true
		}()
	}
	
	// 等待所有请求完成
	for i := 0; i < 50; i++ {
		<-done
	}
	
	// 验证速率限制器没有panic或数据竞争
	t.Log("✅ 并发访问安全")
}

// TestAddressValidation_RealWorldScenario 测试真实场景
func TestAddressValidation_RealWorldScenario(t *testing.T) {
	// 模拟从网络收到的各种地址
	receivedAddrs := []string{
		"/ip4/8.8.8.8/tcp/4001",           // Google DNS - 公网
		"/ip4/192.168.1.100/tcp/4001",     // 内网 - 私网
		"/ip4/127.0.0.1/tcp/4001",         // 本地 - 回环
		"/ip6/2001:4860:4860::8888/tcp/4001", // Google DNS IPv6 - 公网
		"/ip6/::1/tcp/4001",               // IPv6 loopback
		"/ip4/169.254.10.1/tcp/4001",      // Link-local
		"invalid-address",                  // 无效地址
	}
	
	// 场景1：公网环境（不允许私网）
	h1 := &Handler{
		dht: &DHT{
			config: &Config{
				AllowPrivateAddrs: false,
			},
		},
	}
	
	filtered1 := h1.filterValidAddrs(receivedAddrs, false)
	assert.LessOrEqual(t, len(filtered1), 2, "公网环境应该只保留公网地址")
	
	// 场景2：内网环境（允许私网）
	h2 := &Handler{
		dht: &DHT{
			config: &Config{
				AllowPrivateAddrs: true,
			},
		},
	}
	
	filtered2 := h2.filterValidAddrs(receivedAddrs, true)
	assert.Greater(t, len(filtered2), len(filtered1), "允许私网时应该保留更多地址")
	
	t.Log("✅ 真实场景地址验证正确")
}

// ============================================================================
// Handle* 函数测试 - 测试实际消息处理逻辑，发现BUG
// ============================================================================

// setupTestHandler 创建测试用的 Handler
func setupTestHandler(t *testing.T) *Handler {
	host := newMockHost("test-host-id")

	dht := &DHT{
		host:          host,
		routingTable:  NewRoutingTable(types.NodeID("test-host-id")),
		valueStore:    NewValueStore(),
		providerStore: NewProviderStore(),
		config: &Config{
			AllowPrivateAddrs: true, // 测试时允许私网地址
		},
	}

	return NewHandler(dht)
}

// TestHandleFindNode_Basic 测试基本的 FindNode 处理
func TestHandleFindNode_Basic(t *testing.T) {
	handler := setupTestHandler(t)
	ctx := context.Background()

	// 预先添加一些节点到路由表
	handler.dht.routingTable.Add(&RoutingNode{
		ID:       "peer-1",
		Addrs:    []string{"/ip4/1.1.1.1/tcp/4001"},
		LastSeen: time.Now(),
	})
	handler.dht.routingTable.Add(&RoutingNode{
		ID:       "peer-2",
		Addrs:    []string{"/ip4/2.2.2.2/tcp/4001"},
		LastSeen: time.Now(),
	})

	// 创建 FindNode 请求
	req := NewFindNodeRequest(12345, "requester", []string{"/ip4/10.0.0.1/tcp/4001"}, "target-peer")

	// 处理请求
	resp := handler.handleFindNode(ctx, req)

	// 验证响应
	assert.Equal(t, MessageTypeFindNodeResponse, resp.Type)
	assert.Equal(t, uint64(12345), resp.RequestID)
	assert.True(t, resp.Success)
	assert.GreaterOrEqual(t, len(resp.CloserPeers), 0) // 可能有节点返回

	t.Log("✅ FindNode 基本处理正确")
}

// TestHandleFindValue_Found 测试找到值的情况
func TestHandleFindValue_Found(t *testing.T) {
	handler := setupTestHandler(t)
	ctx := context.Background()

	// 预先存储一个值
	testValue := []byte("test-value-data")
	handler.dht.valueStore.Put("test-key", testValue, time.Hour)

	// 创建 FindValue 请求
	req := NewFindValueRequest(12345, "requester", []string{"/ip4/10.0.0.1/tcp/4001"}, "test-key")

	// 处理请求
	resp := handler.handleFindValue(ctx, req)

	// 验证响应
	assert.Equal(t, MessageTypeFindValueResponse, resp.Type)
	assert.True(t, resp.Success)
	assert.Equal(t, testValue, resp.Value)
	assert.Empty(t, resp.CloserPeers) // 找到值时不返回节点

	t.Log("✅ FindValue 找到值时处理正确")
}

// TestHandleFindValue_NotFound 测试未找到值的情况
func TestHandleFindValue_NotFound(t *testing.T) {
	handler := setupTestHandler(t)
	ctx := context.Background()

	// 添加一些节点
	handler.dht.routingTable.Add(&RoutingNode{
		ID:       "peer-closer",
		Addrs:    []string{"/ip4/3.3.3.3/tcp/4001"},
		LastSeen: time.Now(),
	})

	// 创建 FindValue 请求（键不存在）
	req := NewFindValueRequest(12345, "requester", []string{"/ip4/10.0.0.1/tcp/4001"}, "nonexistent-key")

	// 处理请求
	resp := handler.handleFindValue(ctx, req)

	// 验证响应
	assert.Equal(t, MessageTypeFindValueResponse, resp.Type)
	assert.True(t, resp.Success)
	assert.Empty(t, resp.Value) // 未找到值
	// 可能返回更近的节点

	t.Log("✅ FindValue 未找到值时处理正确")
}

// TestHandleStore_Success 测试成功存储
func TestHandleStore_Success(t *testing.T) {
	handler := setupTestHandler(t)
	ctx := context.Background()

	// 创建 Store 请求
	testValue := []byte("value-to-store")
	req := NewStoreRequest(12345, "requester", []string{"/ip4/10.0.0.1/tcp/4001"}, "store-key", testValue, 3600)

	// 处理请求
	resp := handler.handleStore(ctx, req)

	// 验证响应
	assert.Equal(t, MessageTypeStoreResponse, resp.Type)
	assert.True(t, resp.Success)
	assert.Empty(t, resp.Error)

	// 验证值已存储
	storedValue, exists := handler.dht.valueStore.Get("store-key")
	assert.True(t, exists)
	assert.Equal(t, testValue, storedValue)

	t.Log("✅ Store 存储成功")
}

// TestHandlePing_Success 测试 Ping 处理
func TestHandlePing_Success(t *testing.T) {
	handler := setupTestHandler(t)
	ctx := context.Background()

	// 创建 Ping 请求
	req := NewPingRequest(12345, "pinger", []string{"/ip4/10.0.0.1/tcp/4001"})

	// 处理请求
	resp := handler.handlePing(ctx, req)

	// 验证响应
	assert.Equal(t, MessageTypePingResponse, resp.Type)
	assert.True(t, resp.Success)
	assert.Equal(t, uint64(12345), resp.RequestID)

	t.Log("✅ Ping 处理正确")
}

// TestHandleAddProvider_Success 测试添加 Provider
func TestHandleAddProvider_Success(t *testing.T) {
	handler := setupTestHandler(t)
	ctx := context.Background()

	// 创建 AddProvider 请求
	req := NewAddProviderRequest(12345, "provider-id", []string{"/ip4/10.0.0.1/tcp/4001"}, "content-key", 7200)

	// 处理请求
	resp := handler.handleAddProvider(ctx, req)

	// 验证响应
	assert.Equal(t, MessageTypeAddProviderResponse, resp.Type)
	assert.True(t, resp.Success)

	// 验证 Provider 已添加
	providers := handler.dht.providerStore.GetProviders("content-key")
	assert.GreaterOrEqual(t, len(providers), 1)

	t.Log("✅ AddProvider 处理正确")
}

// TestHandleGetProviders_Success 测试获取 Providers
func TestHandleGetProviders_Success(t *testing.T) {
	handler := setupTestHandler(t)
	ctx := context.Background()

	// 预先添加 Provider（使用正确的 API 签名）
	handler.dht.providerStore.AddProvider(
		"content-key",
		types.NodeID("provider-1"),
		[]string{"/ip4/5.5.5.5/tcp/4001"},
		time.Hour,
	)

	// 创建 GetProviders 请求
	req := NewGetProvidersRequest(12345, "requester", []string{"/ip4/10.0.0.1/tcp/4001"}, "content-key")

	// 处理请求
	resp := handler.handleGetProviders(ctx, req)

	// 验证响应
	assert.Equal(t, MessageTypeGetProvidersResponse, resp.Type)
	assert.True(t, resp.Success)
	assert.GreaterOrEqual(t, len(resp.Providers), 1)

	t.Log("✅ GetProviders 处理正确")
}

// TestHandleRemoveProvider_Success 测试移除 Provider
func TestHandleRemoveProvider_Success(t *testing.T) {
	handler := setupTestHandler(t)
	ctx := context.Background()

	// 预先添加 Provider（使用正确的 API 签名）
	handler.dht.providerStore.AddProvider(
		"content-key",
		types.NodeID("requester"), // 同一个 sender
		[]string{"/ip4/5.5.5.5/tcp/4001"},
		time.Hour,
	)

	// 创建 RemoveProvider 请求
	req := NewRemoveProviderRequest(12345, "requester", []string{"/ip4/10.0.0.1/tcp/4001"}, "content-key")

	// 处理请求
	resp := handler.handleRemoveProvider(ctx, req)

	// 验证响应
	assert.Equal(t, MessageTypeRemoveProviderResponse, resp.Type)
	assert.True(t, resp.Success)

	t.Log("✅ RemoveProvider 处理正确")
}

// ============================================================================
// 边界条件和错误处理测试 - 重点发现BUG
// ============================================================================

// TestHandleFindNode_NoRoutableAddresses 测试无可路由地址
func TestHandleFindNode_NoRoutableAddresses(t *testing.T) {
	handler := setupTestHandler(t)
	handler.dht.config.AllowPrivateAddrs = false // 禁止私网地址
	ctx := context.Background()

	// 创建请求（只有回环地址）
	req := NewFindNodeRequest(12345, "requester", []string{"/ip4/127.0.0.1/tcp/4001"}, "target")

	// 处理请求
	resp := handler.handleFindNode(ctx, req)

	// 验证响应 - 应该返回错误
	assert.Equal(t, MessageTypeFindNodeResponse, resp.Type)
	assert.False(t, resp.Success)
	assert.Contains(t, resp.Error, "no routable addresses")

	t.Log("✅ 无可路由地址时正确返回错误")
}

// TestHandleStore_LargeValue 测试存储大值
func TestHandleStore_LargeValue(t *testing.T) {
	handler := setupTestHandler(t)
	ctx := context.Background()

	// 创建 100KB 的值
	largeValue := make([]byte, 100*1024)
	for i := range largeValue {
		largeValue[i] = byte(i % 256)
	}

	req := NewStoreRequest(12345, "requester", []string{"/ip4/10.0.0.1/tcp/4001"}, "large-key", largeValue, 3600)

	// 处理请求
	resp := handler.handleStore(ctx, req)

	// 验证响应
	assert.Equal(t, MessageTypeStoreResponse, resp.Type)
	assert.True(t, resp.Success)

	// 验证值已存储
	storedValue, exists := handler.dht.valueStore.Get("large-key")
	assert.True(t, exists)
	assert.Equal(t, largeValue, storedValue)

	t.Log("✅ 大值存储正确")
}

// TestHandleFindValue_EmptyKey 测试空键
func TestHandleFindValue_EmptyKey(t *testing.T) {
	handler := setupTestHandler(t)
	ctx := context.Background()

	// 创建请求（空键）
	req := NewFindValueRequest(12345, "requester", []string{"/ip4/10.0.0.1/tcp/4001"}, "")

	// 处理请求
	resp := handler.handleFindValue(ctx, req)

	// 验证响应 - 空键应该返回空值和可能的更近节点
	assert.Equal(t, MessageTypeFindValueResponse, resp.Type)
	assert.True(t, resp.Success) // 不是错误，只是没找到
	assert.Empty(t, resp.Value)

	t.Log("✅ 空键查询正确处理")
}
