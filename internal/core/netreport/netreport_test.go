package netreport

import (
	"context"
	"net"
	"testing"
	"time"

	netreportif "github.com/dep2p/go-dep2p/pkg/interfaces/netreport"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              ReportBuilder 测试
// ============================================================================

func TestReportBuilder_Basic(t *testing.T) {
	builder := NewReportBuilder()

	// 设置 IPv4 状态
	builder.SetUDPv4(true, net.ParseIP("1.2.3.4"), 12345)

	// 设置 IPv6 状态
	builder.SetUDPv6(true, net.ParseIP("2001:db8::1"), 54321)

	// 设置 NAT 类型
	builder.SetNATType(types.NATTypeFull)

	// 添加中继延迟
	builder.AddRelayLatency("https://relay1.example.com", 50*time.Millisecond)
	builder.AddRelayLatency("https://relay2.example.com", 100*time.Millisecond)

	// 构建报告
	report := builder.Build()

	// 验证
	if !report.UDPv4 {
		t.Error("UDPv4 应该为 true")
	}
	if !report.UDPv6 {
		t.Error("UDPv6 应该为 true")
	}
	if !report.GlobalV4.Equal(net.ParseIP("1.2.3.4")) {
		t.Errorf("GlobalV4 不正确: %v", report.GlobalV4)
	}
	if report.GlobalV4Port != 12345 {
		t.Errorf("GlobalV4Port 不正确: %d", report.GlobalV4Port)
	}
	if report.NATType != types.NATTypeFull {
		t.Errorf("NATType 不正确: %s", report.NATType)
	}
	if report.PreferredRelay != "https://relay1.example.com" {
		t.Errorf("PreferredRelay 不正确: %s", report.PreferredRelay)
	}
}

func TestReportBuilder_MappingVaries(t *testing.T) {
	builder := NewReportBuilder()

	// 添加来自不同服务器的映射（相同地址）
	builder.AddIPv4Mapping("server1", net.ParseIP("1.2.3.4"), 12345)
	builder.AddIPv4Mapping("server2", net.ParseIP("1.2.3.4"), 12345)

	report := builder.Build()

	// 映射不变化
	if report.MappingVariesByDestIPv4 == nil || *report.MappingVariesByDestIPv4 {
		t.Error("MappingVariesByDestIPv4 应该为 false")
	}
}

func TestReportBuilder_MappingVariesSymmetric(t *testing.T) {
	builder := NewReportBuilder()

	// 添加来自不同服务器的映射（不同地址 - 对称 NAT）
	builder.AddIPv4Mapping("server1", net.ParseIP("1.2.3.4"), 12345)
	builder.AddIPv4Mapping("server2", net.ParseIP("1.2.3.4"), 54321) // 端口不同

	report := builder.Build()

	// 映射变化 = 对称 NAT
	if report.MappingVariesByDestIPv4 == nil || !*report.MappingVariesByDestIPv4 {
		t.Error("MappingVariesByDestIPv4 应该为 true")
	}
	if report.NATType != types.NATTypeSymmetric {
		t.Errorf("NATType 应该为 Symmetric: %s", report.NATType)
	}
}

func TestReportBuilder_RelayLatency(t *testing.T) {
	builder := NewReportBuilder()

	// 添加多个延迟，应该保留最低的
	builder.AddRelayLatency("https://relay.example.com", 100*time.Millisecond)
	builder.AddRelayLatency("https://relay.example.com", 50*time.Millisecond)
	builder.AddRelayLatency("https://relay.example.com", 200*time.Millisecond)

	report := builder.Build()

	if report.RelayLatencies["https://relay.example.com"] != 50*time.Millisecond {
		t.Errorf("延迟应该是最低的 50ms: %v", report.RelayLatencies["https://relay.example.com"])
	}
}

func TestReportBuilder_Snapshot(t *testing.T) {
	builder := NewReportBuilder()
	builder.SetUDPv4(true, net.ParseIP("1.2.3.4"), 12345)

	// 获取快照
	snapshot := builder.Snapshot()

	// 修改 builder
	builder.SetUDPv6(true, net.ParseIP("2001:db8::1"), 54321)

	// 快照不应该受影响
	if snapshot.UDPv6 {
		t.Error("快照不应该包含后续修改")
	}
}

// ============================================================================
//                              Report 测试
// ============================================================================

func TestReport_HasUDP(t *testing.T) {
	tests := []struct {
		name   string
		udpv4  bool
		udpv6  bool
		expect bool
	}{
		{"both true", true, true, true},
		{"only v4", true, false, true},
		{"only v6", false, true, true},
		{"both false", false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := &netreportif.Report{
				UDPv4: tt.udpv4,
				UDPv6: tt.udpv6,
			}
			if report.HasUDP() != tt.expect {
				t.Errorf("HasUDP() = %v, want %v", report.HasUDP(), tt.expect)
			}
		})
	}
}

func TestReport_IsSymmetricNAT(t *testing.T) {
	tests := []struct {
		name    string
		natType types.NATType
		expect  bool
	}{
		{"symmetric", types.NATTypeSymmetric, true},
		{"full", types.NATTypeFull, false},
		{"restricted", types.NATTypeRestricted, false},
		{"unknown", types.NATTypeUnknown, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := &netreportif.Report{
				NATType: tt.natType,
			}
			if report.IsSymmetricNAT() != tt.expect {
				t.Errorf("IsSymmetricNAT() = %v, want %v", report.IsSymmetricNAT(), tt.expect)
			}
		})
	}
}

func TestReport_HasPortMapping(t *testing.T) {
	tests := []struct {
		name   string
		upnp   bool
		natpmp bool
		pcp    bool
		expect bool
	}{
		{"all true", true, true, true, true},
		{"only upnp", true, false, false, true},
		{"only natpmp", false, true, false, true},
		{"only pcp", false, false, true, true},
		{"none", false, false, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := &netreportif.Report{
				UPnPAvailable:   tt.upnp,
				NATPMPAvailable: tt.natpmp,
				PCPAvailable:    tt.pcp,
			}
			if report.HasPortMapping() != tt.expect {
				t.Errorf("HasPortMapping() = %v, want %v", report.HasPortMapping(), tt.expect)
			}
		})
	}
}

// ============================================================================
//                              Client 测试
// ============================================================================

func TestClient_DefaultConfig(t *testing.T) {
	config := netreportif.DefaultConfig()

	if len(config.STUNServers) == 0 {
		t.Error("默认配置应该包含 STUN 服务器")
	}
	if config.Timeout == 0 {
		t.Error("默认配置应该有超时时间")
	}
	if config.ProbeTimeout == 0 {
		t.Error("默认配置应该有探测超时时间")
	}
}

func TestClient_Creation(t *testing.T) {
	config := netreportif.DefaultConfig()

	client := NewClient(config)

	if client == nil {
		t.Error("NewClient 不应该返回 nil")
	}
	if client.LastReport() != nil {
		t.Error("新客户端不应该有历史报告")
	}
}

func TestClient_SetServers(t *testing.T) {
	config := netreportif.DefaultConfig()
	client := NewClient(config)

	// 设置 STUN 服务器
	newSTUN := []string{"stun.test.com:3478"}
	client.SetSTUNServers(newSTUN)

	// 设置中继服务器
	newRelay := []string{"https://relay.test.com"}
	client.SetRelayServers(newRelay)

	// 验证配置已更新
	clientConfig := client.Config()
	if len(clientConfig.STUNServers) != 1 || clientConfig.STUNServers[0] != "stun.test.com:3478" {
		t.Error("STUN 服务器未正确更新")
	}
	if len(clientConfig.RelayServers) != 1 || clientConfig.RelayServers[0] != "https://relay.test.com" {
		t.Error("中继服务器未正确更新")
	}
}

func TestClient_Stats(t *testing.T) {
	config := netreportif.DefaultConfig()
	client := NewClient(config)

	stats := client.Stats()

	if stats.TotalReports != 0 {
		t.Error("新客户端应该没有报告")
	}
}

func TestClient_BestRelays(t *testing.T) {
	config := netreportif.DefaultConfig()
	client := NewClient(config)

	// 没有报告时应该返回 nil
	relays := client.BestRelays(3)
	if relays != nil {
		t.Error("没有报告时应该返回 nil")
	}
}

// ============================================================================
//                              Config 测试
// ============================================================================

func TestConfig_DefaultValues(t *testing.T) {
	config := netreportif.DefaultConfig()

	if config.Timeout != 30*time.Second {
		t.Errorf("默认超时不正确: %v", config.Timeout)
	}
	if config.ProbeTimeout != 5*time.Second {
		t.Errorf("默认探测超时不正确: %v", config.ProbeTimeout)
	}
	if !config.EnableIPv4 {
		t.Error("默认应该启用 IPv4")
	}
	if !config.EnableIPv6 {
		t.Error("默认应该启用 IPv6")
	}
	if config.MaxConcurrentProbes != 10 {
		t.Errorf("默认并发探测数不正确: %d", config.MaxConcurrentProbes)
	}
	if config.FullReportInterval != 5*time.Minute {
		t.Errorf("默认完整报告间隔不正确: %v", config.FullReportInterval)
	}
}

// ============================================================================
//                              ProbeType 测试
// ============================================================================

func TestProbeType_String(t *testing.T) {
	tests := []struct {
		probe  netreportif.ProbeType
		expect string
	}{
		{netreportif.ProbeTypeIPv4, "IPv4"},
		{netreportif.ProbeTypeIPv6, "IPv6"},
		{netreportif.ProbeTypeNAT, "NAT"},
		{netreportif.ProbeTypeRelay, "Relay"},
		{netreportif.ProbeTypePortMap, "PortMap"},
		{netreportif.ProbeTypeCaptivePortal, "CaptivePortal"},
		{netreportif.ProbeType(999), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expect, func(t *testing.T) {
			if tt.probe.String() != tt.expect {
				t.Errorf("String() = %s, want %s", tt.probe.String(), tt.expect)
			}
		})
	}
}

// ============================================================================
//                              集成测试（跳过需要网络的测试）
// ============================================================================

func TestClient_GetReport_Timeout(t *testing.T) {
	// 使用非常短的超时和不可达的服务器
	config := netreportif.DefaultConfig()
	config.STUNServers = []string{"192.0.2.1:3478"} // TEST-NET-1，不可达
	config.Timeout = 100 * time.Millisecond
	config.ProbeTimeout = 50 * time.Millisecond

	client := NewClient(config)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	report, err := client.GetReport(ctx)
	if err != nil {
		// 超时是正常的
		t.Logf("预期的超时错误: %v", err)
	}

	// 报告应该仍然生成（即使所有探测失败）
	if report == nil {
		t.Error("即使探测失败，报告也应该生成")
	}
}

// ============================================================================
//                              并发安全测试
// ============================================================================

func TestClient_UpdateConfig_Concurrent(t *testing.T) {
	config := netreportif.DefaultConfig()
	client := NewClient(config)

	done := make(chan struct{})

	// 并发更新配置
	go func() {
		for i := 0; i < 100; i++ {
			client.SetSTUNServers([]string{"stun1.example.com:3478"})
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < 100; i++ {
			client.SetRelayServers([]string{"https://relay.example.com"})
		}
		done <- struct{}{}
	}()

	go func() {
		for i := 0; i < 100; i++ {
			_ = client.Config()
		}
		done <- struct{}{}
	}()

	// 等待所有 goroutine 完成
	for i := 0; i < 3; i++ {
		<-done
	}
}

func TestProber_UpdateConfig_Concurrent(t *testing.T) {
	config := netreportif.DefaultConfig()
	prober := NewProber(config)

	done := make(chan struct{})

	// 并发更新配置
	go func() {
		for i := 0; i < 100; i++ {
			newConfig := netreportif.DefaultConfig()
			newConfig.Timeout = time.Duration(i) * time.Millisecond
			prober.UpdateConfig(newConfig)
		}
		done <- struct{}{}
	}()

	// 并发读取配置
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		for i := 0; i < 10; i++ {
			_ = prober.RunProbes(ctx)
		}
		done <- struct{}{}
	}()

	// 等待所有 goroutine 完成
	for i := 0; i < 2; i++ {
		<-done
	}
}

func TestClient_History_ThreadSafe(t *testing.T) {
	config := netreportif.DefaultConfig()
	config.STUNServers = []string{"192.0.2.1:3478"}
	config.Timeout = 10 * time.Millisecond
	config.ProbeTimeout = 5 * time.Millisecond
	client := NewClient(config)

	done := make(chan struct{})

	// 并发生成报告
	go func() {
		for i := 0; i < 5; i++ {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			_, _ = client.GetReport(ctx)
			cancel()
		}
		done <- struct{}{}
	}()

	// 并发读取历史
	go func() {
		for i := 0; i < 50; i++ {
			_ = client.History()
		}
		done <- struct{}{}
	}()

	// 等待所有 goroutine 完成
	for i := 0; i < 2; i++ {
		<-done
	}
}

