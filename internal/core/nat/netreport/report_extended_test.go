package netreport

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                     NATType.String 测试
// ============================================================================

func TestNATType_String_Extended(t *testing.T) {
	tests := []struct {
		natType  NATType
		expected string
	}{
		{NATTypeUnknown, "unknown"},
		{NATTypeFull, "full_cone"},
		{NATTypeRestricted, "restricted"},
		{NATTypePortRestricted, "port_restricted"},
		{NATTypeSymmetric, "symmetric"},
		{NATType(99), "unknown"}, // 未知值
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.natType.String())
		})
	}
}

// ============================================================================
//                     Report 方法测试
// ============================================================================

func TestReport_HasUDP_Extended(t *testing.T) {
	// 无 UDP
	r := &Report{}
	assert.False(t, r.HasUDP())

	// 只有 IPv4
	r.UDPv4 = true
	assert.True(t, r.HasUDP())

	// 只有 IPv6
	r = &Report{UDPv6: true}
	assert.True(t, r.HasUDP())

	// 两者都有
	r = &Report{UDPv4: true, UDPv6: true}
	assert.True(t, r.HasUDP())
}

func TestReport_IsSymmetricNAT_AllTypes(t *testing.T) {
	r := &Report{}
	assert.False(t, r.IsSymmetricNAT())

	r.NATType = NATTypeSymmetric
	assert.True(t, r.IsSymmetricNAT())

	r.NATType = NATTypeFull
	assert.False(t, r.IsSymmetricNAT())
}

func TestReport_HasPortMapping_Extended(t *testing.T) {
	r := &Report{}
	assert.False(t, r.HasPortMapping())

	r.UPnPAvailable = true
	assert.True(t, r.HasPortMapping())

	r = &Report{NATPMPAvailable: true}
	assert.True(t, r.HasPortMapping())

	r = &Report{PCPAvailable: true}
	assert.True(t, r.HasPortMapping())
}

func TestReport_BestRelayLatency(t *testing.T) {
	r := &Report{
		RelayLatencies: make(map[string]time.Duration),
	}

	// 无首选中继
	assert.Equal(t, time.Duration(0), r.BestRelayLatency())

	// 有首选中继
	r.RelayLatencies["relay1"] = 100 * time.Millisecond
	r.RelayLatencies["relay2"] = 50 * time.Millisecond
	r.PreferredRelay = "relay2"
	assert.Equal(t, 50*time.Millisecond, r.BestRelayLatency())
}

// ============================================================================
//                     ReportBuilder 测试
// ============================================================================

func TestReportBuilder_New(t *testing.T) {
	b := NewReportBuilder()
	require.NotNil(t, b)
	require.NotNil(t, b.report)
	require.NotNil(t, b.report.RelayLatencies)
	assert.False(t, b.report.Timestamp.IsZero())
}

func TestReportBuilder_SetUDPv4(t *testing.T) {
	b := NewReportBuilder()

	ip := net.ParseIP("203.0.113.1")
	b.SetUDPv4(true, ip, 4001)

	assert.True(t, b.report.UDPv4)
	assert.Equal(t, ip, b.report.GlobalV4)
	assert.Equal(t, uint16(4001), b.report.GlobalV4Port)
}

func TestReportBuilder_SetUDPv4_NotAvailable(t *testing.T) {
	b := NewReportBuilder()

	b.SetUDPv4(false, nil, 0)

	assert.False(t, b.report.UDPv4)
	assert.Nil(t, b.report.GlobalV4)
}

func TestReportBuilder_SetUDPv6(t *testing.T) {
	b := NewReportBuilder()

	ip := net.ParseIP("2001:db8::1")
	b.SetUDPv6(true, ip, 4001)

	assert.True(t, b.report.UDPv6)
	assert.Equal(t, ip, b.report.GlobalV6)
	assert.Equal(t, uint16(4001), b.report.GlobalV6Port)
}

func TestReportBuilder_AddIPv4Mapping(t *testing.T) {
	b := NewReportBuilder()

	ip1 := net.ParseIP("203.0.113.1")
	b.AddIPv4Mapping("stun1", ip1, 4001)

	assert.True(t, b.report.UDPv4)
	assert.Equal(t, ip1, b.report.GlobalV4)
	assert.Equal(t, uint16(4001), b.report.GlobalV4Port)
	assert.Len(t, b.ipv4Mappings, 1)
}

func TestReportBuilder_AddIPv4Mapping_Multiple(t *testing.T) {
	b := NewReportBuilder()

	ip1 := net.ParseIP("203.0.113.1")
	ip2 := net.ParseIP("203.0.113.1") // 相同 IP

	b.AddIPv4Mapping("stun1", ip1, 4001)
	b.AddIPv4Mapping("stun2", ip2, 4001) // 相同端口

	// 映射不变化
	require.NotNil(t, b.report.MappingVariesByDestIPv4)
	assert.False(t, *b.report.MappingVariesByDestIPv4)
}

func TestReportBuilder_AddIPv4Mapping_Varies(t *testing.T) {
	b := NewReportBuilder()

	ip1 := net.ParseIP("203.0.113.1")
	ip2 := net.ParseIP("203.0.113.2") // 不同 IP

	b.AddIPv4Mapping("stun1", ip1, 4001)
	b.AddIPv4Mapping("stun2", ip2, 4002) // 不同端口

	// 映射变化
	require.NotNil(t, b.report.MappingVariesByDestIPv4)
	assert.True(t, *b.report.MappingVariesByDestIPv4)
}

func TestReportBuilder_AddIPv6Mapping(t *testing.T) {
	b := NewReportBuilder()

	ip1 := net.ParseIP("2001:db8::1")
	b.AddIPv6Mapping("stun1", ip1, 4001)

	assert.True(t, b.report.UDPv6)
	assert.Equal(t, ip1, b.report.GlobalV6)
	assert.Len(t, b.ipv6Mappings, 1)
}

func TestReportBuilder_SetNATType(t *testing.T) {
	b := NewReportBuilder()

	b.SetNATType(NATTypeSymmetric)
	assert.Equal(t, NATTypeSymmetric, b.report.NATType)
}

func TestReportBuilder_AddRelayLatency(t *testing.T) {
	b := NewReportBuilder()

	b.AddRelayLatency("relay1", 100*time.Millisecond)
	b.AddRelayLatency("relay2", 50*time.Millisecond)

	assert.Equal(t, 100*time.Millisecond, b.report.RelayLatencies["relay1"])
	assert.Equal(t, 50*time.Millisecond, b.report.RelayLatencies["relay2"])
	assert.Equal(t, "relay2", b.report.PreferredRelay) // 最低延迟
}

func TestReportBuilder_AddRelayLatency_Update(t *testing.T) {
	b := NewReportBuilder()

	b.AddRelayLatency("relay1", 100*time.Millisecond)
	b.AddRelayLatency("relay1", 50*time.Millisecond) // 更低延迟

	// 应该更新为更低延迟
	assert.Equal(t, 50*time.Millisecond, b.report.RelayLatencies["relay1"])
}

func TestReportBuilder_SetPortMapAvailability(t *testing.T) {
	b := NewReportBuilder()

	b.SetPortMapAvailability(true, true, false)

	assert.True(t, b.report.UPnPAvailable)
	assert.True(t, b.report.NATPMPAvailable)
	assert.False(t, b.report.PCPAvailable)
}

func TestReportBuilder_SetCaptivePortal(t *testing.T) {
	b := NewReportBuilder()

	b.SetCaptivePortal(true)

	require.NotNil(t, b.report.CaptivePortal)
	assert.True(t, *b.report.CaptivePortal)
}

func TestReportBuilder_SetDuration(t *testing.T) {
	b := NewReportBuilder()

	b.SetDuration(5 * time.Second)
	assert.Equal(t, 5*time.Second, b.report.Duration)
}

func TestReportBuilder_Build(t *testing.T) {
	b := NewReportBuilder()

	ip := net.ParseIP("203.0.113.1")
	b.SetUDPv4(true, ip, 4001)
	b.AddRelayLatency("relay1", 100*time.Millisecond)
	b.SetPortMapAvailability(true, false, false)
	b.SetDuration(2 * time.Second)

	report := b.Build()

	require.NotNil(t, report)
	assert.True(t, report.UDPv4)
	assert.Equal(t, "relay1", report.PreferredRelay)
	assert.True(t, report.UPnPAvailable)
	assert.Equal(t, 2*time.Second, report.Duration)
}

func TestReportBuilder_Build_InferNATType(t *testing.T) {
	b := NewReportBuilder()

	// 添加两个不同的映射结果
	ip1 := net.ParseIP("203.0.113.1")
	ip2 := net.ParseIP("203.0.113.2")
	b.AddIPv4Mapping("stun1", ip1, 4001)
	b.AddIPv4Mapping("stun2", ip2, 4002)

	report := b.Build()

	// 应该推断为对称 NAT
	assert.Equal(t, NATTypeSymmetric, report.NATType)
}

func TestReportBuilder_Build_InferFullCone(t *testing.T) {
	b := NewReportBuilder()

	// 添加相同的映射结果
	ip := net.ParseIP("203.0.113.1")
	b.AddIPv4Mapping("stun1", ip, 4001)
	b.AddIPv4Mapping("stun2", ip, 4001)

	report := b.Build()

	// 应该推断为完全锥形 NAT
	assert.Equal(t, NATTypeFull, report.NATType)
}

func TestReportBuilder_Build_NoUDP(t *testing.T) {
	b := NewReportBuilder()

	report := b.Build()

	// 无 UDP 连通性，NAT 类型未知
	assert.Equal(t, NATTypeUnknown, report.NATType)
}

// ============================================================================
//                     checkMappingVaries 测试
// ============================================================================

func TestReportBuilder_CheckMappingVaries(t *testing.T) {
	b := NewReportBuilder()

	// 少于 2 个映射
	result := b.checkMappingVaries([]mappingResult{})
	assert.False(t, result)

	result = b.checkMappingVaries([]mappingResult{
		{server: "s1", ip: net.ParseIP("1.1.1.1"), port: 4001},
	})
	assert.False(t, result)

	// 相同映射
	result = b.checkMappingVaries([]mappingResult{
		{server: "s1", ip: net.ParseIP("1.1.1.1"), port: 4001},
		{server: "s2", ip: net.ParseIP("1.1.1.1"), port: 4001},
	})
	assert.False(t, result)

	// 不同 IP
	result = b.checkMappingVaries([]mappingResult{
		{server: "s1", ip: net.ParseIP("1.1.1.1"), port: 4001},
		{server: "s2", ip: net.ParseIP("1.1.1.2"), port: 4001},
	})
	assert.True(t, result)

	// 不同端口
	result = b.checkMappingVaries([]mappingResult{
		{server: "s1", ip: net.ParseIP("1.1.1.1"), port: 4001},
		{server: "s2", ip: net.ParseIP("1.1.1.1"), port: 4002},
	})
	assert.True(t, result)
}

// ============================================================================
//                     Config 测试
// ============================================================================

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.NotEmpty(t, cfg.STUNServers)
	assert.Equal(t, 30*time.Second, cfg.Timeout)
	assert.Equal(t, 5*time.Second, cfg.ProbeTimeout)
	assert.True(t, cfg.EnableIPv4)
	assert.True(t, cfg.EnableIPv6)
	assert.True(t, cfg.EnableRelayProbe)
	assert.True(t, cfg.EnablePortMapProbe)
	assert.True(t, cfg.EnableCaptivePortalProbe)
	assert.Equal(t, 10, cfg.MaxConcurrentProbes)
	assert.Equal(t, 5*time.Minute, cfg.FullReportInterval)
}
