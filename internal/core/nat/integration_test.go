//go:build integration
// +build integration

package nat

import (
	"context"
	"testing"
	"time"
	
	"github.com/dep2p/go-dep2p/internal/core/nat/stun"
	"github.com/dep2p/go-dep2p/internal/core/nat/upnp"
	"github.com/dep2p/go-dep2p/internal/core/nat/natpmp"
)

// TestSTUN_RealServer 测试真实 STUN 服务器
func TestSTUN_RealServer(t *testing.T) {
	client := stun.NewSTUNClient([]string{
		"stun.l.google.com:19302",
		"stun1.l.google.com:19302",
	})
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	addr, err := client.GetExternalAddr(ctx)
	if err != nil {
		t.Fatalf("STUN query failed: %v", err)
	}
	
	if addr.IP == nil {
		t.Error("No external IP")
	}
	
	t.Logf("✅ External address: %s:%d", addr.IP, addr.Port)
}

// TestUPnP_Discovery 测试 UPnP 设备发现
func TestUPnP_Discovery(t *testing.T) {
	mapper, err := upnp.NewUPnPMapper()
	if err != nil {
		t.Skipf("No UPnP device found (expected in many environments): %v", err)
		return
	}
	
	t.Log("✅ UPnP device found")
	
	// 测试端口映射
	ctx := context.Background()
	port, err := mapper.MapPort(ctx, "UDP", 54321)
	if err != nil {
		t.Fatalf("MapPort failed: %v", err)
	}
	defer mapper.UnmapPort("UDP", port)
	
	t.Logf("✅ Mapped port: %d", port)
}

// TestNATPMP_Discovery 测试 NAT-PMP 网关发现
func TestNATPMP_Discovery(t *testing.T) {
	mapper, err := natpmp.NewNATPMPMapper()
	if err != nil {
		t.Skipf("No NAT-PMP gateway found (expected in many environments): %v", err)
		return
	}
	
	t.Log("✅ NAT-PMP gateway found")
	
	// 测试端口映射
	ctx := context.Background()
	port, err := mapper.MapPort(ctx, "UDP", 54322)
	if err != nil {
		t.Fatalf("MapPort failed: %v", err)
	}
	defer mapper.UnmapPort("UDP", port)
	
	t.Logf("✅ Mapped port: %d", port)
}

// TestService_FullIntegration 测试完整 NAT 服务集成
func TestService_FullIntegration(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ProbeInterval = 5 * time.Second // 加快测试速度
	cfg.STUNCacheDuration = 10 * time.Second
	
	service, err := NewService(cfg, nil, nil)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// 启动服务
	if err := service.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer service.Stop()
	
	// 等待 STUN 查询和 AutoNAT 探测
	time.Sleep(15 * time.Second)
	
	// 检查外部地址
	addrs := service.ExternalAddrs()
	t.Logf("External addresses: %v", addrs)
	
	// 检查可达性
	reachability := service.Reachability()
	t.Logf("Reachability: %s", reachability)
	
	t.Log("✅ Full integration test completed")
}

// TestService_PortMapping 测试端口映射服务
func TestService_PortMapping(t *testing.T) {
	cfg := DefaultConfig()
	service, err := NewService(cfg, nil, nil)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	
	ctx := context.Background()
	if err := service.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer service.Stop()
	
	// 尝试映射端口
	port, err := service.MapPort(ctx, "UDP", 54323)
	if err != nil {
		t.Logf("Port mapping failed (expected if no UPnP/NAT-PMP): %v", err)
		return
	}
	defer service.UnmapPort("UDP", port)
	
	t.Logf("✅ Mapped port: %d", port)
}
