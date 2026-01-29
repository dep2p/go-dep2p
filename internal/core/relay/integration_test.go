package relay

import (
	"testing"
)

// TestConfig_Validation 测试配置验证
func TestConfig_Validation(t *testing.T) {
	// 默认配置应该有效
	config := DefaultConfig()
	if err := config.Validate(); err != nil {
		t.Errorf("Default config invalid: %v", err)
	}

	// 无效配置
	invalidConfig := &Config{
		MaxReservations: 0, // 应该 >= 1
	}
	if err := invalidConfig.Validate(); err == nil {
		t.Error("Expected validation error for MaxReservations = 0")
	}
}

// TestRelayLimiter_Integration 测试统一限流器集成
func TestRelayLimiter_Integration(t *testing.T) {
	limiter := NewRelayLimiter(DefaultRelayLimiterConfig())

	// 允许预约
	if err := limiter.AllowReservation("peer1"); err != nil {
		t.Errorf("AllowReservation failed: %v", err)
	}

	// 允许电路
	if err := limiter.AllowCircuit("peer1"); err != nil {
		t.Errorf("AllowCircuit failed: %v", err)
	}

	// 释放电路
	limiter.ReleaseCircuit("peer1")
}

// TestRelayLimiter_StrictConfig_Integration 测试严格配置集成
func TestRelayLimiter_StrictConfig_Integration(t *testing.T) {
	config := StrictRelayLimiterConfig()
	limiter := NewRelayLimiter(config)

	// 严格配置应该有合理的限制
	// MaxConnectionsPerPeer=2，所以每个 peer 最多 2 个
	// 使用不同的 peer 来分配电路
	for i := 0; i < 10; i++ {
		peerID := string(rune('a' + i)) // 使用不同的 peer ID
		if err := limiter.AllowCircuit(peerID); err != nil {
			t.Errorf("Unexpected error for peer %s: %v", peerID, err)
		}
	}

	// 获取统计信息
	stats := limiter.Stats()
	if stats.TotalCircuits != 10 {
		t.Errorf("Expected 10 circuits, got %d", stats.TotalCircuits)
	}
}

// TestRelayLimiter_DefaultNoLimit_Integration 测试默认不限制
func TestRelayLimiter_DefaultNoLimit_Integration(t *testing.T) {
	limiter := NewRelayLimiter(DefaultRelayLimiterConfig())

	// 默认不限制，应该允许大量预约
	for i := 0; i < 100; i++ {
		if err := limiter.AllowReservation("peer"); err != nil {
			t.Errorf("Unexpected error at %d: %v", i, err)
		}
	}
}
