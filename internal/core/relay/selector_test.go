package relay

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSelector_SelectBest 测试选择最佳中继
func TestSelector_SelectBest(t *testing.T) {
	selector := NewSelector()
	require.NotNil(t, selector)

	relays := []RelayInfo{
		{ID: "relay-1", Latency: 100, Capacity: 0.5, Reliability: 0.8},
		{ID: "relay-2", Latency: 30, Capacity: 0.9, Reliability: 0.95},
		{ID: "relay-3", Latency: 50, Capacity: 0.7, Reliability: 0.9},
	}

	best := selector.SelectBest(relays, "target-peer")

	// relay-2 应该是最佳选择（低延迟、高容量、高可靠性）
	assert.Equal(t, "relay-2", best.ID)

	t.Log("✅ SelectBest 选择低延迟高可靠中继")
}

// TestSelector_SelectBest_Empty 测试空列表
func TestSelector_SelectBest_Empty(t *testing.T) {
	selector := NewSelector()

	best := selector.SelectBest(nil, "target")

	// 空列表应该返回空 RelayInfo
	assert.Empty(t, best.ID)

	t.Log("✅ SelectBest 处理空列表正确")
}

// TestSelector_SelectBest_Single 测试单个中继
func TestSelector_SelectBest_Single(t *testing.T) {
	selector := NewSelector()

	relays := []RelayInfo{
		{ID: "only-relay", Latency: 50, Capacity: 0.8, Reliability: 0.9},
	}

	best := selector.SelectBest(relays, "target")

	assert.Equal(t, "only-relay", best.ID)

	t.Log("✅ SelectBest 处理单个中继正确")
}

// TestSelector_ScoreCalculation 测试评分计算
func TestSelector_ScoreCalculation(t *testing.T) {
	selector := NewSelector()

	tests := []struct {
		name     string
		relay    RelayInfo
		minScore int
	}{
		{
			name: "低延迟中继",
			relay: RelayInfo{
				ID:          "fast-relay",
				Latency:     30, // 30ms
				Capacity:    0.8,
				Reliability: 0.95,
			},
			minScore: 100, // 基础分 100
		},
		{
			name: "高延迟中继",
			relay: RelayInfo{
				ID:          "slow-relay",
				Latency:     500, // 500ms
				Capacity:    0.8,
				Reliability: 0.95,
			},
			minScore: 50, // 高延迟会减分
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := selector.calculateScore(tt.relay, "target")
			assert.GreaterOrEqual(t, score, tt.minScore,
				"评分 %d 应该 >= %d", score, tt.minScore)
		})
	}
}

// TestSelector_Creation 测试选择器创建
func TestSelector_Creation(t *testing.T) {
	selector := NewSelector()
	require.NotNil(t, selector)

	t.Log("✅ Selector 创建成功")
}

// ============================================================================
//                 真正能发现 BUG 的测试
// ============================================================================

// TestSelector_SelectBest_EqualScores 测试评分相同时的选择行为
// 验证：当多个中继评分完全相同时，应该返回第一个（稳定性）
func TestSelector_SelectBest_EqualScores(t *testing.T) {
	selector := NewSelector()

	// 创建评分完全相同的中继
	relays := []RelayInfo{
		{ID: "relay-first", Latency: 50, Capacity: 0.5, Reliability: 0.5},
		{ID: "relay-second", Latency: 50, Capacity: 0.5, Reliability: 0.5},
		{ID: "relay-third", Latency: 50, Capacity: 0.5, Reliability: 0.5},
	}

	best := selector.SelectBest(relays, "target")

	// 验证选择了第一个（使用 > 而不是 >= 的结果）
	if best.ID != "relay-first" {
		t.Errorf("评分相同时应该选择第一个，但选择了: %s", best.ID)
	}
}

// TestSelector_SelectBest_NegativeCapacity 测试负数容量的处理
// 潜在BUG：负数 Capacity/Reliability 会导致评分异常
func TestSelector_SelectBest_NegativeCapacity(t *testing.T) {
	selector := NewSelector()

	relays := []RelayInfo{
		{ID: "normal", Latency: 50, Capacity: 0.5, Reliability: 0.5},
		{ID: "negative-capacity", Latency: 50, Capacity: -1.0, Reliability: 0.5},
	}

	best := selector.SelectBest(relays, "target")

	// 正常的中继应该被选中，而不是负数容量的
	if best.ID != "normal" {
		t.Errorf("BUG: 负数容量的中继不应该被选中，但选择了: %s", best.ID)
	}
}

// TestSelector_SelectBest_OverflowCapacity 测试超过 1.0 的容量
// 潜在BUG：超过 1.0 的值可能导致不公平的评分
func TestSelector_SelectBest_OverflowCapacity(t *testing.T) {
	selector := NewSelector()

	relays := []RelayInfo{
		{ID: "normal", Latency: 30, Capacity: 1.0, Reliability: 1.0},         // 最大正常值
		{ID: "overflow", Latency: 100, Capacity: 10.0, Reliability: 10.0},    // 异常高值
	}

	// 计算两者的评分
	normalScore := selector.calculateScore(relays[0], "target")
	overflowScore := selector.calculateScore(relays[1], "target")

	t.Logf("正常中继评分: %d, 溢出中继评分: %d", normalScore, overflowScore)

	// 低延迟+正常容量的中继应该获胜
	best := selector.SelectBest(relays, "target")
	if best.ID == "overflow" && overflowScore > normalScore {
		t.Logf("注意: 超过 1.0 的 Capacity/Reliability 值会获得更高评分，可能需要验证输入")
	}
}

// TestSelector_SelectMultiple_CountExceedsTotal 测试请求数量超过可用数量
func TestSelector_SelectMultiple_CountExceedsTotal(t *testing.T) {
	selector := NewSelector()

	relays := []RelayInfo{
		{ID: "relay-1", Latency: 50},
		{ID: "relay-2", Latency: 100},
	}

	// 请求 10 个，但只有 2 个可用
	result := selector.SelectMultiple(relays, "target", 10)

	if len(result) != 2 {
		t.Errorf("应该返回 2 个中继，但返回了 %d 个", len(result))
	}
}

// TestSelector_SelectMultiple_ZeroCount 测试请求 0 个
func TestSelector_SelectMultiple_ZeroCount(t *testing.T) {
	selector := NewSelector()

	relays := []RelayInfo{
		{ID: "relay-1", Latency: 50},
	}

	result := selector.SelectMultiple(relays, "target", 0)

	if result != nil {
		t.Errorf("请求 0 个应该返回 nil，但返回了 %v", result)
	}
}

// TestSelector_SelectMultiple_NegativeCount 测试负数请求
func TestSelector_SelectMultiple_NegativeCount(t *testing.T) {
	selector := NewSelector()

	relays := []RelayInfo{
		{ID: "relay-1", Latency: 50},
	}

	result := selector.SelectMultiple(relays, "target", -5)

	if result != nil {
		t.Errorf("负数请求应该返回 nil，但返回了 %v", result)
	}
}

// TestSelector_ExtractIP_MalformedAddresses 测试畸形地址的 IP 提取
func TestSelector_ExtractIP_MalformedAddresses(t *testing.T) {
	selector := NewSelector()

	tests := []struct {
		name    string
		addr    string
		wantNil bool
	}{
		{"empty", "", true},
		{"no ip component", "/tcp/4001", true},
		{"ip4 without value", "/ip4/", true},
		{"ip4 with invalid ip", "/ip4/not-an-ip/tcp/4001", true},
		{"valid ipv4", "/ip4/192.168.1.1/tcp/4001", false},
		{"valid ipv6", "/ip6/::1/tcp/4001", false},
		{"raw ipv4", "192.168.1.1", false},
		{"just slash", "/", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := selector.extractIP(tt.addr)
			if tt.wantNil && ip != nil {
				t.Errorf("地址 %q 应该提取不到 IP，但得到: %v", tt.addr, ip)
			}
			if !tt.wantNil && ip == nil {
				t.Errorf("地址 %q 应该能提取 IP，但得到 nil", tt.addr)
			}
		})
	}
}

// TestSelector_GetRegion_NilResolver 测试没有设置解析器时的区域获取
func TestSelector_GetRegion_NilResolver(t *testing.T) {
	selector := NewSelector()
	// 不设置 geoResolver

	region := selector.getRegion("/ip4/8.8.8.8/tcp/4001")

	if region != "" {
		t.Errorf("没有 geoResolver 时应该返回空字符串，但得到: %s", region)
	}
}
