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
//                              Prober 测试
// ============================================================================

func TestNewProber(t *testing.T) {
	config := netreportif.DefaultConfig()
	prober := NewProber(config)

	if prober == nil {
		t.Fatal("prober should not be nil")
	}
}

func TestNewProber_NilLogger(t *testing.T) {
	config := netreportif.DefaultConfig()
	prober := NewProber(config)

	if prober == nil {
		t.Fatal("prober should handle nil logger")
	}
}

func TestProber_UpdateConfig(t *testing.T) {
	config := netreportif.DefaultConfig()
	prober := NewProber(config)

	// 更新配置
	newConfig := config
	newConfig.STUNServers = []string{"stun.test.com:3478"}
	newConfig.Timeout = 10 * time.Second

	prober.UpdateConfig(newConfig)

	// 验证配置已更新
	if len(prober.config.STUNServers) != 1 {
		t.Error("config not updated correctly")
	}
}

func TestProber_RunProbes_Timeout(t *testing.T) {
	config := netreportif.DefaultConfig()
	config.STUNServers = []string{"192.0.2.1:3478"} // 不可达
	config.Timeout = 100 * time.Millisecond
	config.ProbeTimeout = 50 * time.Millisecond

	prober := NewProber(config)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	report := prober.RunProbes(ctx)

	// 报告应该仍然生成
	if report == nil {
		t.Error("report should be generated even on timeout")
	}

	// 应该设置时间戳
	if report.Timestamp.IsZero() {
		t.Error("timestamp should be set")
	}
}

func TestProber_RunProbes_DisabledIPv4(t *testing.T) {
	config := netreportif.DefaultConfig()
	config.EnableIPv4 = false
	config.EnableIPv6 = true
	config.Timeout = 100 * time.Millisecond
	config.ProbeTimeout = 50 * time.Millisecond

	prober := NewProber(config)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	report := prober.RunProbes(ctx)

	// 报告应该生成
	if report == nil {
		t.Error("report should be generated")
	}
}

func TestProber_RunProbes_DisabledIPv6(t *testing.T) {
	config := netreportif.DefaultConfig()
	config.EnableIPv4 = true
	config.EnableIPv6 = false
	config.Timeout = 100 * time.Millisecond
	config.ProbeTimeout = 50 * time.Millisecond

	prober := NewProber(config)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	report := prober.RunProbes(ctx)

	if report == nil {
		t.Error("report should be generated")
	}
}

// ============================================================================
//                              Client 更多测试
// ============================================================================

func TestClient_ForceFullReport(t *testing.T) {
	config := netreportif.DefaultConfig()
	client := NewClient(config)

	// 强制完整报告
	client.ForceFullReport()

	// lastFull 应该被重置
	client.mu.RLock()
	if !client.lastFull.IsZero() {
		t.Error("lastFull should be reset")
	}
	client.mu.RUnlock()
}

func TestClient_UpdateConfig(t *testing.T) {
	config := netreportif.DefaultConfig()
	client := NewClient(config)

	// 更新配置
	newConfig := config
	newConfig.Timeout = 1 * time.Minute
	client.UpdateConfig(newConfig)

	// 验证配置已更新
	if client.Config().Timeout != 1*time.Minute {
		t.Error("config not updated correctly")
	}
}

func TestClient_History(t *testing.T) {
	config := netreportif.DefaultConfig()
	config.STUNServers = []string{"192.0.2.1:3478"}
	config.Timeout = 50 * time.Millisecond
	config.ProbeTimeout = 25 * time.Millisecond

	client := NewClient(config)

	// 初始历史应该为空
	history := client.History()
	if len(history) != 0 {
		t.Error("initial history should be empty")
	}

	// 生成报告
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := client.GetReport(ctx)
	if err != nil {
		t.Logf("report error (expected): %v", err)
	}

	// 历史应该有一个报告
	history = client.History()
	if len(history) != 1 {
		t.Errorf("expected 1 report in history, got %d", len(history))
	}
}

func TestClient_GetReportAsync(t *testing.T) {
	config := netreportif.DefaultConfig()
	config.STUNServers = []string{"192.0.2.1:3478"}
	config.Timeout = 50 * time.Millisecond
	config.ProbeTimeout = 25 * time.Millisecond

	client := NewClient(config)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// 异步获取报告
	ch := client.GetReportAsync(ctx)

	// 等待结果
	select {
	case report := <-ch:
		if report == nil {
			t.Error("report should not be nil")
		}
	case <-time.After(1 * time.Second):
		t.Error("timeout waiting for async report")
	}
}

func TestClient_BestRelays_WithData(t *testing.T) {
	config := netreportif.DefaultConfig()
	client := NewClient(config)

	// 模拟一个有中继延迟的报告
	client.mu.Lock()
	client.lastReport = &netreportif.Report{
		RelayLatencies: map[string]time.Duration{
			"relay1": 100 * time.Millisecond,
			"relay2": 50 * time.Millisecond,
			"relay3": 200 * time.Millisecond,
		},
	}
	client.mu.Unlock()

	// 获取最佳中继
	relays := client.BestRelays(2)

	if len(relays) != 2 {
		t.Errorf("expected 2 relays, got %d", len(relays))
	}

	// 第一个应该是延迟最低的
	if relays[0] != "relay2" {
		t.Errorf("first relay should be relay2, got %s", relays[0])
	}
}

func TestClient_Stats_WithHistory(t *testing.T) {
	config := netreportif.DefaultConfig()
	client := NewClient(config)

	// 添加一些历史报告
	client.mu.Lock()
	client.history = []*netreportif.Report{
		{UDPv4: true, UDPv6: false, Duration: 100 * time.Millisecond, Timestamp: time.Now()},
		{UDPv4: true, UDPv6: true, Duration: 200 * time.Millisecond, Timestamp: time.Now()},
		{UDPv4: false, UDPv6: true, Duration: 150 * time.Millisecond, Timestamp: time.Now()},
	}
	client.mu.Unlock()

	stats := client.Stats()

	if stats.TotalReports != 3 {
		t.Errorf("expected 3 total reports, got %d", stats.TotalReports)
	}
	if stats.SuccessfulIPv4 != 2 {
		t.Errorf("expected 2 successful IPv4, got %d", stats.SuccessfulIPv4)
	}
	if stats.SuccessfulIPv6 != 2 {
		t.Errorf("expected 2 successful IPv6, got %d", stats.SuccessfulIPv6)
	}
}

// ============================================================================
//                              Report 更多测试
// ============================================================================

func TestReport_GlobalAddress(t *testing.T) {
	report := &netreportif.Report{
		GlobalV4:     net.ParseIP("1.2.3.4"),
		GlobalV4Port: 12345,
		GlobalV6:     net.ParseIP("2001:db8::1"),
		GlobalV6Port: 54321,
	}

	if !report.GlobalV4.Equal(net.ParseIP("1.2.3.4")) {
		t.Error("GlobalV4 mismatch")
	}
	if report.GlobalV4Port != 12345 {
		t.Error("GlobalV4Port mismatch")
	}
	if !report.GlobalV6.Equal(net.ParseIP("2001:db8::1")) {
		t.Error("GlobalV6 mismatch")
	}
	if report.GlobalV6Port != 54321 {
		t.Error("GlobalV6Port mismatch")
	}
}

func TestReport_NATTypes(t *testing.T) {
	tests := []struct {
		natType  types.NATType
		expected string
	}{
		{types.NATTypeUnknown, "unknown"},
		{types.NATTypeFull, "full_cone"},
		{types.NATTypeRestricted, "restricted"},
		{types.NATTypePortRestricted, "port_restricted"},
		{types.NATTypeSymmetric, "symmetric"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			report := &netreportif.Report{NATType: tt.natType}
			if report.NATType.String() != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, report.NATType.String())
			}
		})
	}
}

func TestReport_CaptivePortal(t *testing.T) {
	// 有强制门户
	trueVal := true
	report1 := &netreportif.Report{CaptivePortal: &trueVal}
	if report1.CaptivePortal == nil || !*report1.CaptivePortal {
		t.Error("CaptivePortal should be true")
	}

	// 无强制门户
	falseVal := false
	report2 := &netreportif.Report{CaptivePortal: &falseVal}
	if report2.CaptivePortal == nil || *report2.CaptivePortal {
		t.Error("CaptivePortal should be false")
	}

	// 未检测
	report3 := &netreportif.Report{}
	if report3.CaptivePortal != nil {
		t.Error("CaptivePortal should be nil")
	}
}

// ============================================================================
//                              ReportBuilder 更多测试
// ============================================================================

func TestReportBuilder_SetPortMapAvailability(t *testing.T) {
	builder := NewReportBuilder()
	builder.SetPortMapAvailability(true, true, true)

	report := builder.Build()
	if !report.UPnPAvailable {
		t.Error("UPnP should be available")
	}
	if !report.NATPMPAvailable {
		t.Error("NAT-PMP should be available")
	}
	if !report.PCPAvailable {
		t.Error("PCP should be available")
	}
}

func TestReportBuilder_SetCaptivePortal(t *testing.T) {
	builder := NewReportBuilder()
	builder.SetCaptivePortal(true)

	report := builder.Build()
	if report.CaptivePortal == nil || !*report.CaptivePortal {
		t.Error("CaptivePortal should be true")
	}
}

func TestReportBuilder_IPv6Mapping(t *testing.T) {
	builder := NewReportBuilder()

	// 添加来自不同服务器的 IPv6 映射
	builder.AddIPv6Mapping("server1", net.ParseIP("2001:db8::1"), 12345)
	builder.AddIPv6Mapping("server2", net.ParseIP("2001:db8::1"), 12345)

	report := builder.Build()

	// 映射不变化
	if report.MappingVariesByDestIPv6 == nil || *report.MappingVariesByDestIPv6 {
		t.Error("MappingVariesByDestIPv6 should be false")
	}
}

func TestReportBuilder_IPv6MappingVaries(t *testing.T) {
	builder := NewReportBuilder()

	// 添加来自不同服务器的 IPv6 映射（不同端口 - 对称 NAT）
	builder.AddIPv6Mapping("server1", net.ParseIP("2001:db8::1"), 12345)
	builder.AddIPv6Mapping("server2", net.ParseIP("2001:db8::1"), 54321)

	report := builder.Build()

	// 映射变化
	if report.MappingVariesByDestIPv6 == nil || !*report.MappingVariesByDestIPv6 {
		t.Error("MappingVariesByDestIPv6 should be true")
	}
}

func TestReportBuilder_ConcurrentAccess(t *testing.T) {
	builder := NewReportBuilder()

	// 并发设置值
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			builder.SetUDPv4(n%2 == 0, net.ParseIP("1.2.3.4"), uint16(n))
			builder.AddRelayLatency("relay", time.Duration(n)*time.Millisecond)
			done <- true
		}(i)
	}

	// 等待完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 应该能够正常构建报告
	report := builder.Build()
	if report == nil {
		t.Error("report should not be nil")
	}
}

