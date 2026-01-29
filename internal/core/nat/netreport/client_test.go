// Package netreport 提供网络诊断功能
package netreport

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewClient(t *testing.T) {
	config := DefaultConfig()
	client := NewClient(config)

	assert.NotNil(t, client)
	assert.NotNil(t, client.stunClient)
}

func TestReportBuilder_Basic(t *testing.T) {
	builder := NewReportBuilder()

	// 设置 IPv4
	builder.SetUDPv4(true, []byte{1, 2, 3, 4}, 12345)

	report := builder.Build()
	assert.True(t, report.UDPv4)
	assert.Equal(t, uint16(12345), report.GlobalV4Port)
}

func TestReportBuilder_NATType(t *testing.T) {
	t.Run("Symmetric NAT", func(t *testing.T) {
		builder := NewReportBuilder()

		// 添加不同的映射（模拟对称 NAT）
		builder.AddIPv4Mapping("stun1", []byte{1, 2, 3, 4}, 10000)
		builder.AddIPv4Mapping("stun2", []byte{1, 2, 3, 4}, 10001) // 不同端口

		report := builder.Build()
		assert.Equal(t, NATTypeSymmetric, report.NATType)
	})

	t.Run("Full Cone NAT", func(t *testing.T) {
		builder := NewReportBuilder()

		// 添加相同的映射（模拟锥形 NAT）
		builder.AddIPv4Mapping("stun1", []byte{1, 2, 3, 4}, 10000)
		builder.AddIPv4Mapping("stun2", []byte{1, 2, 3, 4}, 10000) // 相同端口

		report := builder.Build()
		assert.Equal(t, NATTypeFull, report.NATType)
	})
}

func TestReportBuilder_RelayLatency(t *testing.T) {
	builder := NewReportBuilder()

	builder.AddRelayLatency("relay1", 100*time.Millisecond)
	builder.AddRelayLatency("relay2", 50*time.Millisecond)
	builder.AddRelayLatency("relay3", 150*time.Millisecond)

	report := builder.Build()

	// 首选中继应该是延迟最低的
	assert.Equal(t, "relay2", report.PreferredRelay)
	assert.Equal(t, 50*time.Millisecond, report.BestRelayLatency())
}

func TestReport_HasUDP(t *testing.T) {
	t.Run("No UDP", func(t *testing.T) {
		report := &Report{}
		assert.False(t, report.HasUDP())
	})

	t.Run("IPv4 UDP", func(t *testing.T) {
		report := &Report{UDPv4: true}
		assert.True(t, report.HasUDP())
	})

	t.Run("IPv6 UDP", func(t *testing.T) {
		report := &Report{UDPv6: true}
		assert.True(t, report.HasUDP())
	})
}

func TestReport_IsSymmetricNAT(t *testing.T) {
	t.Run("Not Symmetric", func(t *testing.T) {
		report := &Report{NATType: NATTypeFull}
		assert.False(t, report.IsSymmetricNAT())
	})

	t.Run("Symmetric", func(t *testing.T) {
		report := &Report{NATType: NATTypeSymmetric}
		assert.True(t, report.IsSymmetricNAT())
	})
}

func TestReport_HasPortMapping(t *testing.T) {
	t.Run("No PortMap", func(t *testing.T) {
		report := &Report{}
		assert.False(t, report.HasPortMapping())
	})

	t.Run("UPnP", func(t *testing.T) {
		report := &Report{UPnPAvailable: true}
		assert.True(t, report.HasPortMapping())
	})

	t.Run("NAT-PMP", func(t *testing.T) {
		report := &Report{NATPMPAvailable: true}
		assert.True(t, report.HasPortMapping())
	})
}

func TestNATType_String(t *testing.T) {
	tests := []struct {
		natType  NATType
		expected string
	}{
		{NATTypeUnknown, "unknown"},
		{NATTypeFull, "full_cone"},
		{NATTypeRestricted, "restricted"},
		{NATTypePortRestricted, "port_restricted"},
		{NATTypeSymmetric, "symmetric"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.natType.String())
		})
	}
}

func TestClient_LastReport(t *testing.T) {
	config := DefaultConfig()
	client := NewClient(config)

	// 初始应该为 nil
	assert.Nil(t, client.LastReport())
}

func TestClient_ForceFullReport(t *testing.T) {
	config := DefaultConfig()
	client := NewClient(config)

	// 模拟设置 lastReport
	client.lastReport = &Report{UDPv4: true}
	client.lastReportTime = time.Now()

	// 强制重置
	client.ForceFullReport()

	assert.Nil(t, client.lastReport)
	assert.True(t, client.lastReportTime.IsZero())
}

func TestClient_SetSTUNServers(t *testing.T) {
	config := DefaultConfig()
	client := NewClient(config)

	newServers := []string{"stun.example.com:3478"}
	client.SetSTUNServers(newServers)

	assert.Equal(t, newServers, client.config.STUNServers)
}

func TestClient_SetRelayServers(t *testing.T) {
	config := DefaultConfig()
	client := NewClient(config)

	newRelays := []string{"relay.example.com:443"}
	client.SetRelayServers(newRelays)

	assert.Equal(t, newRelays, client.config.RelayServers)
}

func TestClient_GetReportAsync(t *testing.T) {
	config := DefaultConfig()
	config.Timeout = 1 * time.Second
	config.EnableIPv4 = false
	config.EnableIPv6 = false
	config.EnableRelayProbe = false
	config.EnablePortMapProbe = false
	config.EnableCaptivePortalProbe = false

	client := NewClient(config)

	ctx := context.Background()
	ch := client.GetReportAsync(ctx)

	select {
	case report := <-ch:
		require.NotNil(t, report)
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for report")
	}
}

func TestSTUNClient_BuildRequest(t *testing.T) {
	request := buildSTUNBindingRequest()

	// 验证请求长度
	assert.Equal(t, 20, len(request))

	// 验证 Message Type
	assert.Equal(t, byte(0x00), request[0])
	assert.Equal(t, byte(0x01), request[1])

	// 验证 Magic Cookie
	assert.Equal(t, byte(0x21), request[4])
	assert.Equal(t, byte(0x12), request[5])
	assert.Equal(t, byte(0xA4), request[6])
	assert.Equal(t, byte(0x42), request[7])
}
