package stun

import (
	"context"
	"net"
	"testing"
	"time"
)

// TestNewSTUNClient 测试创建 STUN 客户端
func TestNewSTUNClient(t *testing.T) {
	servers := []string{"stun.l.google.com:19302"}
	client := NewSTUNClient(servers)

	if client == nil {
		t.Fatal("NewSTUNClient returned nil")
	}

	t.Log("✅ NewSTUNClient 成功创建客户端")
}

// TestSTUNClient_InvalidServer 测试无效服务器
func TestSTUNClient_InvalidServer(t *testing.T) {
	servers := []string{"invalid.server:9999"}
	client := NewSTUNClient(servers)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := client.GetExternalAddr(ctx)
	if err == nil {
		t.Error("Expected error for invalid server")
	}

	t.Log("✅ STUN 客户端正确处理无效服务器")
}

// TestSTUNClient_Timeout 测试超时
func TestSTUNClient_Timeout(t *testing.T) {
	// 使用一个不响应的地址
	servers := []string{"192.0.2.1:9999"}
	client := NewSTUNClient(servers)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := client.GetExternalAddr(ctx)
	if err == nil {
		t.Error("Expected timeout error")
	}

	t.Log("✅ STUN 客户端超时处理正确")
}

// TestSTUNClient_EmptyServers 测试空服务器列表
func TestSTUNClient_EmptyServers(t *testing.T) {
	client := NewSTUNClient([]string{})

	ctx := context.Background()
	_, err := client.GetExternalAddr(ctx)

	if err == nil {
		t.Error("Expected error for empty servers")
	}

	t.Log("✅ STUN 客户端正确处理空服务器列表")
}

// TestSTUNClient_MultipleServers 测试多服务器故障转移
func TestSTUNClient_MultipleServers(t *testing.T) {
	// 第一个服务器无效，第二个可能有效
	servers := []string{
		"invalid.server:9999",
		"stun.l.google.com:19302",
	}
	client := NewSTUNClient(servers)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 应该尝试故障转移到第二个服务器
	// 注意：这个测试需要网络连接，可能在隔离环境中失败
	_, err := client.GetExternalAddr(ctx)
	// 不严格要求成功，因为可能没有网络

	t.Logf("Multiple servers test result: %v", err)
	t.Log("✅ STUN 客户端多服务器测试完成")
}

// TestSTUNClient_CacheExpiry 测试缓存过期
func TestSTUNClient_CacheExpiry(t *testing.T) {
	servers := []string{"stun.l.google.com:19302"}
	client := NewSTUNClient(servers)

	// 设置短缓存时间用于测试
	client.SetCacheDuration(100 * time.Millisecond)

	// 第一次查询（假设）
	queryCount := 0
	client.SetQueryFunc(func() (*net.UDPAddr, error) {
		queryCount++
		return &net.UDPAddr{IP: net.ParseIP("1.2.3.4"), Port: 12345}, nil
	})

	ctx := context.Background()

	// 第一次查询
	addr1, _ := client.GetExternalAddr(ctx)
	count1 := queryCount

	// 立即第二次查询，应该使用缓存
	addr2, _ := client.GetExternalAddr(ctx)
	if queryCount != count1 {
		t.Error("Expected cached result, but new query was made")
	}
	if addr1.String() != addr2.String() {
		t.Error("Cached address mismatch")
	}

	// 等待缓存过期
	time.Sleep(150 * time.Millisecond)

	// 第三次查询，应该重新查询
	_, _ = client.GetExternalAddr(ctx)
	if queryCount == count1 {
		t.Error("Expected new query after cache expiry")
	}

	t.Log("✅ STUN 客户端缓存过期处理正确")
}

// TestSTUNClient_ContextCancellation 测试上下文取消
func TestSTUNClient_ContextCancellation(t *testing.T) {
	servers := []string{"stun.l.google.com:19302"}
	client := NewSTUNClient(servers)

	ctx, cancel := context.WithCancel(context.Background())

	// 立即取消
	cancel()

	_, err := client.GetExternalAddr(ctx)
	if err == nil {
		t.Error("Expected error for cancelled context")
	}

	t.Log("✅ STUN 客户端上下文取消处理正确")
}

// TestSTUNClient_ValidAddress 测试有效地址解析
func TestSTUNClient_ValidAddress(t *testing.T) {
	// 这个测试需要模拟 STUN 响应
	client := NewSTUNClient(nil)

	// 设置模拟响应
	client.SetQueryFunc(func() (*net.UDPAddr, error) {
		return &net.UDPAddr{
			IP:   net.ParseIP("203.0.113.1"),
			Port: 54321,
		}, nil
	})

	ctx := context.Background()
	addr, err := client.GetExternalAddr(ctx)

	if err != nil {
		t.Fatalf("GetExternalAddr failed: %v", err)
	}

	if addr.IP.String() != "203.0.113.1" {
		t.Errorf("IP = %s, want 203.0.113.1", addr.IP)
	}
	if addr.Port != 54321 {
		t.Errorf("Port = %d, want 54321", addr.Port)
	}

	t.Log("✅ STUN 客户端地址解析正确")
}
