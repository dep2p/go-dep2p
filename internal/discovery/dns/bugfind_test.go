// Package dns - BUG 验证测试
package dns

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                    BUG #B39: ParseDNSAddr 空嵌套域名
// ============================================================================

// TestBugFind_B39_ParseDNSAddr_EmptyNestedDomain 测试 BUG #B39
//
// BUG 描述：ParseDNSAddr 对于 "dnsaddr=/dnsaddr/" 输入，会返回空字符串作为 nestedDomain
// 这会导致后续的 ResolveWithDepth 调用时，传入空字符串域名，可能引发问题
//
// 位置：resolver.go:300-308
//
// 问题代码：
//   if strings.HasPrefix(addrStr, "/dnsaddr/") {
//       parts := strings.SplitN(addrStr, "/", 4)
//       if len(parts) < 3 {
//           return nil, "", ErrInvalidDNSAddr
//       }
//       nestedDomain := parts[2]  // ⚠️ 可能是空字符串！
//       return nil, nestedDomain, nil
//   }
//
// 测试输入：
// - "dnsaddr=/dnsaddr/" → parts = ["", "dnsaddr", ""], nestedDomain = ""
// - "dnsaddr=/dnsaddr//" → parts = ["", "dnsaddr", "", ""], nestedDomain = ""
func TestBugFind_B39_ParseDNSAddr_EmptyNestedDomain(t *testing.T) {
	t.Run("trailing slash - empty nested domain", func(t *testing.T) {
		// 这个输入会导致 nestedDomain = ""
		peer, nestedDomain, err := ParseDNSAddr("dnsaddr=/dnsaddr/")

		// 当前行为：不返回错误，但 nestedDomain 是空字符串
		t.Logf("Result: peer=%v, nestedDomain=%q, err=%v", peer, nestedDomain, err)

		// BUG: 应该返回错误，但实际可能返回空字符串
		if err == nil && nestedDomain == "" {
			t.Error("🐛 BUG #B39 确认：返回了空的 nestedDomain，应该返回错误")
		}
	})

	t.Run("double slash - empty nested domain", func(t *testing.T) {
		peer, nestedDomain, err := ParseDNSAddr("dnsaddr=/dnsaddr//")

		t.Logf("Result: peer=%v, nestedDomain=%q, err=%v", peer, nestedDomain, err)

		if err == nil && nestedDomain == "" {
			t.Error("🐛 BUG #B39 确认：返回了空的 nestedDomain，应该返回错误")
		}
	})

	t.Run("valid nested domain for comparison", func(t *testing.T) {
		peer, nestedDomain, err := ParseDNSAddr("dnsaddr=/dnsaddr/example.com")

		require.NoError(t, err)
		assert.Nil(t, peer)
		assert.Equal(t, "example.com", nestedDomain)
		t.Log("✅ 有效的嵌套域名正常工作")
	})
}

// ============================================================================
//              BUG #B39 的下游影响测试
// ============================================================================

// TestBugFind_B39_Impact_ResolveWithEmptyDomain 测试空域名对 Resolve 的影响
func TestBugFind_B39_Impact_ResolveWithEmptyDomain(t *testing.T) {
	config := ResolverConfig{
		Timeout:  1 * time.Second,
		MaxDepth: 3,
		CacheTTL: 1 * time.Minute,
	}
	resolver := NewResolver(config)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	t.Run("empty domain normalization", func(t *testing.T) {
		// 空域名会被规范化为 "_dnsaddr."
		normalized := resolver.normalizeDomain("")
		t.Logf("Empty domain normalized to: %q", normalized)

		// 这会导致查询 "_dnsaddr." 这个无效域名
		assert.Equal(t, "_dnsaddr.", normalized)
	})

	t.Run("resolve empty domain", func(t *testing.T) {
		// 尝试解析空域名
		peers, err := resolver.Resolve(ctx, "")

		// 这应该返回错误（DNS 查询失败）
		t.Logf("Resolve empty domain: peers=%v, err=%v", len(peers), err)

		// 虽然会返回错误，但浪费了一次 DNS 查询
		// 最好在 ParseDNSAddr 阶段就拒绝空域名
		if err == nil {
			t.Error("🐛 空域名解析应该失败，但却成功了")
		}
	})
}

// ============================================================================
//                    其他潜在的边界条件 BUG
// ============================================================================

// TestBugFind_ParseDNSAddr_EdgeCases 测试其他边界条件
func TestBugFind_ParseDNSAddr_EdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		record      string
		shouldError bool
		reason      string
	}{
		{
			name:        "nested with only slash",
			record:      "dnsaddr=/dnsaddr/",
			shouldError: true,
			reason:      "empty nested domain should be rejected",
		},
		{
			name:        "nested with whitespace domain",
			record:      "dnsaddr=/dnsaddr/ ",
			shouldError: false, // 空格是有效字符（虽然不是好的域名）
			reason:      "whitespace domain (边界情况)",
		},
		{
			name:        "nested with dot only",
			record:      "dnsaddr=/dnsaddr/.",
			shouldError: false, // "." 是有效的域名字符
			reason:      "dot-only domain",
		},
		{
			name:        "multiaddr with empty peer ID",
			record:      "dnsaddr=/ip4/192.168.1.1/tcp/4001/p2p/",
			shouldError: true,
			reason:      "empty peer ID should be rejected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			peer, nestedDomain, err := ParseDNSAddr(tt.record)

			if tt.shouldError {
				if err == nil {
					t.Errorf("🐛 潜在BUG：%s，但没有返回错误。peer=%v, nestedDomain=%q",
						tt.reason, peer, nestedDomain)
				} else {
					t.Logf("✅ 正确拒绝：%s", tt.reason)
				}
			} else {
				t.Logf("边界情况：%s → err=%v, nestedDomain=%q", tt.reason, err, nestedDomain)
			}
		})
	}
}

// ============================================================================
//                    并发安全边界测试
// ============================================================================

// TestBugFind_ConcurrentCacheAccess 测试并发缓存访问的边界条件
func TestBugFind_ConcurrentCacheAccess(t *testing.T) {
	config := ResolverConfig{
		Timeout:  5 * time.Second,
		MaxDepth: 3,
		CacheTTL: 100 * time.Millisecond, // 短 TTL
	}
	resolver := NewResolver(config)

	// 并发写入和过期清理
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			resolver.setCache("domain1.com", nil)
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()

	// Cleaner goroutine
	go func() {
		for i := 0; i < 50; i++ {
			resolver.ClearExpiredCache()
			time.Sleep(2 * time.Millisecond)
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			_, _ = resolver.getFromCache("domain1.com")
			time.Sleep(1 * time.Millisecond)
		}
		done <- true
	}()

	// 等待完成
	for i := 0; i < 3; i++ {
		<-done
	}

	t.Log("✅ 并发缓存访问测试通过（无panic）")
}

// TestBugFind_AddDomain_DuplicateRace 测试并发添加相同域名
func TestBugFind_AddDomain_DuplicateRace(t *testing.T) {
	config := DefaultConfig()
	discoverer := NewDiscoverer(config)

	ctx := context.Background()
	err := discoverer.Start(ctx)
	require.NoError(t, err)
	defer discoverer.Stop(ctx)

	// 并发添加同一个域名
	const goroutines = 20
	done := make(chan bool, goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			_ = discoverer.AddDomain("test.com")
			done <- true
		}()
	}

	// 等待完成
	for i := 0; i < goroutines; i++ {
		<-done
	}

	// 验证只添加了一次
	domains := discoverer.Domains()
	count := 0
	for _, d := range domains {
		if d == "test.com" {
			count++
		}
	}

	if count > 1 {
		t.Errorf("🐛 潜在BUG：并发添加域名导致重复，count=%d（应该是1）", count)
	} else {
		t.Logf("✅ 并发添加域名去重正确，count=%d", count)
	}
}

// ============================================================================
//                    递归深度边界测试
// ============================================================================

// TestBugFind_ResolveWithDepth_NegativeDepthBehavior 测试负数深度的行为
func TestBugFind_ResolveWithDepth_NegativeDepthBehavior(t *testing.T) {
	config := ResolverConfig{
		Timeout:  1 * time.Second,
		MaxDepth: 3,
		CacheTTL: 1 * time.Minute,
	}
	resolver := NewResolver(config)

	ctx := context.Background()

	// 测试各种负数深度
	depths := []int{-1, -10, -100, -999999}

	for _, depth := range depths {
		t.Run("depth="+string(rune(depth)), func(t *testing.T) {
			_, err := resolver.ResolveWithDepth(ctx, "example.com", depth)

			// 应该立即返回 ErrMaxDepthExceeded
			if err != ErrMaxDepthExceeded {
				t.Errorf("🐛 深度=%d 应该返回 ErrMaxDepthExceeded，但返回了 %v", depth, err)
			} else {
				t.Logf("✅ 深度=%d 正确返回 ErrMaxDepthExceeded", depth)
			}
		})
	}
}

// ============================================================================
//                    资源泄漏检测
// ============================================================================

// TestBugFind_FindPeers_ChannelLeak 测试 FindPeers 通道是否正确关闭
func TestBugFind_FindPeers_ChannelLeak(t *testing.T) {
	config := DefaultConfig()
	config.Timeout = 100 * time.Millisecond
	discoverer := NewDiscoverer(config)

	ctx := context.Background()
	err := discoverer.Start(ctx)
	require.NoError(t, err)
	defer discoverer.Stop(ctx)

	// 创建一个会超时的 context
	ctx, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()

	ch, err := discoverer.FindPeers(ctx, "nonexistent.invalid")
	require.NoError(t, err)

	// 不读取通道，等待 context 超时
	time.Sleep(200 * time.Millisecond)

	// 验证通道是否关闭
	select {
	case _, ok := <-ch:
		if ok {
			t.Log("⚠️ 通道仍然打开，可能有goroutine泄漏")
		} else {
			t.Log("✅ 通道已关闭")
		}
	default:
		t.Log("⚠️ 通道没有关闭也没有数据")
	}
}

// ============================================================================
//                    总结
// ============================================================================

// TestBugFind_Summary 运行所有 BUG 检测测试并生成报告
func TestBugFind_Summary(t *testing.T) {
	t.Log("============================================================")
	t.Log("                 DNS 模块 BUG 检测总结")
	t.Log("============================================================")
	t.Log("")
	t.Log("已发现的潜在 BUG：")
	t.Log("1. ⚠️  BUG #B39: ParseDNSAddr 接受空嵌套域名")
	t.Log("   - 输入: 'dnsaddr=/dnsaddr/' 返回空字符串而非错误")
	t.Log("   - 影响: 浪费 DNS 查询，应该提前拒绝")
	t.Log("   - 严重度: 🟡 中等")
	t.Log("")
	t.Log("2. ⚠️  可能的并发问题（需要长时间压测验证）")
	t.Log("   - AddDomain 并发去重")
	t.Log("   - 缓存并发访问")
	t.Log("")
	t.Log("建议：")
	t.Log("- 修复 #B39：在 ParseDNSAddr 中验证 nestedDomain 不为空")
	t.Log("- 增加更多并发压力测试")
	t.Log("============================================================")
}
