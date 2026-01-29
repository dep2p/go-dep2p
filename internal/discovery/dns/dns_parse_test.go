// Package dns - DNS 解析逻辑测试
package dns

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                         ParseDNSAddr 测试
// ============================================================================

// TestParseDNSAddr_ValidFormats 测试有效的 dnsaddr 格式
func TestParseDNSAddr_ValidFormats(t *testing.T) {
	tests := []struct {
		name           string
		record         string
		wantPeerID     string
		wantNestedDomain string
		wantErr        bool
	}{
		{
			name:       "valid ip4 tcp p2p",
			record:     "dnsaddr=/ip4/192.168.1.1/tcp/4001/p2p/QmPeerID123",
			wantPeerID: "QmPeerID123",
			wantErr:    false,
		},
		{
			name:       "valid ip6 tcp p2p",
			record:     "dnsaddr=/ip6/2001:db8::1/tcp/4001/p2p/QmPeerID456",
			wantPeerID: "QmPeerID456",
			wantErr:    false,
		},
		{
			name:             "nested dnsaddr",
			record:           "dnsaddr=/dnsaddr/bootstrap.example.com",
			wantNestedDomain: "bootstrap.example.com",
			wantErr:          false,
		},
		{
			name:       "complex multiaddr",
			record:     "dnsaddr=/ip4/10.0.0.1/tcp/9000/ws/p2p/QmComplexPeer",
			wantPeerID: "QmComplexPeer",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			peer, nestedDomain, err := ParseDNSAddr(tt.record)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.wantNestedDomain != "" {
				assert.Nil(t, peer)
				assert.Equal(t, tt.wantNestedDomain, nestedDomain)
			} else {
				require.NotNil(t, peer)
				assert.Equal(t, tt.wantPeerID, string(peer.ID))
				assert.Equal(t, types.SourceDNS, peer.Source)
				assert.NotEmpty(t, peer.Addrs)
				assert.Empty(t, nestedDomain)
			}
		})
	}
}

// TestParseDNSAddr_InvalidFormats 测试无效格式
func TestParseDNSAddr_InvalidFormats(t *testing.T) {
	invalidRecords := []struct {
		name   string
		record string
		reason string
	}{
		{
			name:   "missing dnsaddr prefix",
			record: "/ip4/192.168.1.1/tcp/4001/p2p/QmPeerID",
			reason: "no dnsaddr= prefix",
		},
		{
			name:   "empty after prefix",
			record: "dnsaddr=",
			reason: "empty address",
		},
		{
			name:   "missing p2p component",
			record: "dnsaddr=/ip4/192.168.1.1/tcp/4001",
			reason: "no peer ID",
		},
		{
			name:   "incomplete nested path",
			record: "dnsaddr=/dnsaddr",
			reason: "incomplete dnsaddr path",
		},
		{
			name:   "invalid multiaddr",
			record: "dnsaddr=/invalid/format/p2p/QmPeer",
			reason: "invalid protocol",
		},
	}

	for _, tt := range invalidRecords {
		t.Run(tt.name, func(t *testing.T) {
			peer, nestedDomain, err := ParseDNSAddr(tt.record)
			
			assert.Error(t, err, "Should error for: %s", tt.reason)
			assert.Nil(t, peer)
			assert.Empty(t, nestedDomain)
		})
	}
}

// TestParseDNSAddr_EdgeCases 测试边界条件
func TestParseDNSAddr_EdgeCases(t *testing.T) {
	t.Run("empty record", func(t *testing.T) {
		peer, nestedDomain, err := ParseDNSAddr("")
		assert.Error(t, err)
		assert.Nil(t, peer)
		assert.Empty(t, nestedDomain)
	})

	t.Run("only prefix", func(t *testing.T) {
		peer, nestedDomain, err := ParseDNSAddr("dnsaddr=")
		assert.Error(t, err)
		assert.Nil(t, peer)
		assert.Empty(t, nestedDomain)
	})

	t.Run("nested with trailing slash", func(t *testing.T) {
		_, nestedDomain, err := ParseDNSAddr("dnsaddr=/dnsaddr/example.com/")
		// 应该能容错处理尾随斜杠
		if err == nil {
			assert.Equal(t, "example.com", nestedDomain)
		}
	})
}

// ============================================================================
//                         parseMultiaddr 测试
// ============================================================================

// TestParseMultiaddr_ValidFormats 测试有效 multiaddr 解析
func TestParseMultiaddr_ValidFormats(t *testing.T) {
	tests := []struct {
		name       string
		addrStr    string
		wantPeerID string
		wantErr    bool
	}{
		{
			name:       "ip4 tcp p2p",
			addrStr:    "/ip4/192.168.1.1/tcp/4001/p2p/QmPeerID",
			wantPeerID: "QmPeerID",
			wantErr:    false,
		},
		{
			name:       "ip6 tcp p2p",
			addrStr:    "/ip6/::1/tcp/4001/p2p/QmPeerID",
			wantPeerID: "QmPeerID",
			wantErr:    false,
		},
		{
			name:       "dns4 tcp p2p",
			addrStr:    "/dns4/example.com/tcp/4001/p2p/QmPeerID",
			wantPeerID: "QmPeerID",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			peer, nestedDomain, err := parseMultiaddr(tt.addrStr)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, peer)
			assert.Equal(t, tt.wantPeerID, string(peer.ID))
			assert.Empty(t, nestedDomain)
			assert.NotEmpty(t, peer.Addrs)
		})
	}
}

// TestParseMultiaddr_InvalidFormats 测试无效格式
func TestParseMultiaddr_InvalidFormats(t *testing.T) {
	invalidAddrs := []struct {
		name    string
		addrStr string
		reason  string
	}{
		{
			name:    "missing p2p component",
			addrStr: "/ip4/192.168.1.1/tcp/4001",
			reason:  "no peer ID",
		},
		{
			name:    "empty string",
			addrStr: "",
			reason:  "empty address",
		},
		{
			name:    "only slash",
			addrStr: "/",
			reason:  "incomplete",
		},
		{
			name:    "p2p at end without ID",
			addrStr: "/ip4/192.168.1.1/tcp/4001/p2p",
			reason:  "missing peer ID value",
		},
	}

	for _, tt := range invalidAddrs {
		t.Run(tt.name, func(t *testing.T) {
			peer, nestedDomain, err := parseMultiaddr(tt.addrStr)
			
			assert.Error(t, err, "Should error for: %s", tt.reason)
			assert.Nil(t, peer)
			assert.Empty(t, nestedDomain)
		})
	}
}

// ============================================================================
//                         Resolve 和 ResolveWithDepth 测试
// ============================================================================

// TestDiscoverer_Resolve 测试 Resolve 方法
func TestDiscoverer_Resolve(t *testing.T) {
	config := DefaultConfig()
	config.Timeout = 500 * time.Millisecond
	discoverer := NewDiscoverer(config)

	ctx := context.Background()
	err := discoverer.Start(ctx)
	require.NoError(t, err)
	defer discoverer.Stop(ctx)

	t.Run("not started error", func(t *testing.T) {
		d := NewDiscoverer(config)
		_, err := d.Resolve(ctx, "example.com")
		assert.ErrorIs(t, err, ErrNotStarted)
	})

	t.Run("nonexistent domain", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(ctx, 1*time.Second)
		defer cancel()

		_, err := discoverer.Resolve(ctx, "nonexistent-domain-12345.invalid")
		// 应该返回错误（域名不存在）
		assert.Error(t, err)
	})
}

// TestResolver_ResolveWithDepth_MaxDepth 测试递归深度限制
func TestResolver_ResolveWithDepth_MaxDepth(t *testing.T) {
	config := ResolverConfig{
		Timeout:  5 * time.Second,
		MaxDepth: 3,
		CacheTTL: 1 * time.Minute,
	}
	resolver := NewResolver(config)

	ctx := context.Background()

	t.Run("negative depth", func(t *testing.T) {
		_, err := resolver.ResolveWithDepth(ctx, "example.com", -1)
		assert.ErrorIs(t, err, ErrMaxDepthExceeded)
	})

	t.Run("zero depth", func(t *testing.T) {
		// 深度为0应该仍然尝试解析（不递归）
		// 但域名不存在会失败
		_, err := resolver.ResolveWithDepth(ctx, "nonexistent.invalid", 0)
		assert.Error(t, err)
	})
}

// TestResolver_ResolveWithDepth_Normalization 测试域名规范化
func TestResolver_ResolveWithDepth_Normalization(t *testing.T) {
	config := ResolverConfig{
		Timeout:  5 * time.Second,
		MaxDepth: 3,
		CacheTTL: 1 * time.Minute,
	}
	resolver := NewResolver(config)

	tests := []struct {
		name   string
		domain string
		want   string
	}{
		{
			name:   "no prefix",
			domain: "example.com",
			want:   "_dnsaddr.example.com",
		},
		{
			name:   "with prefix",
			domain: "_dnsaddr.example.com",
			want:   "_dnsaddr.example.com",
		},
		{
			name:   "trailing dot",
			domain: "example.com.",
			want:   "_dnsaddr.example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized := resolver.normalizeDomain(tt.domain)
			assert.Equal(t, tt.want, normalized)
		})
	}
}

// ============================================================================
//                         AllPeers 和 PeersForDomain 测试
// ============================================================================

// TestDiscoverer_AllPeers 测试获取所有节点
func TestDiscoverer_AllPeers(t *testing.T) {
	config := DefaultConfig()
	discoverer := NewDiscoverer(config)

	t.Run("empty peers", func(t *testing.T) {
		peers := discoverer.AllPeers()
		assert.Empty(t, peers)
	})

	t.Run("with cached peers", func(t *testing.T) {
		// 手动添加缓存的节点
		discoverer.peersMu.Lock()
		discoverer.peers["domain1.com"] = []types.PeerInfo{
			{ID: "peer1"},
			{ID: "peer2"},
		}
		discoverer.peers["domain2.com"] = []types.PeerInfo{
			{ID: "peer2"}, // 重复
			{ID: "peer3"},
		}
		discoverer.peersMu.Unlock()

		peers := discoverer.AllPeers()
		assert.Len(t, peers, 3, "Should deduplicate peers")

		// 验证去重
		ids := make(map[string]bool)
		for _, peer := range peers {
			ids[string(peer.ID)] = true
		}
		assert.Len(t, ids, 3)
	})
}

// TestDiscoverer_PeersForDomain 测试获取指定域名的节点
func TestDiscoverer_PeersForDomain(t *testing.T) {
	config := DefaultConfig()
	discoverer := NewDiscoverer(config)

	t.Run("nonexistent domain", func(t *testing.T) {
		peers := discoverer.PeersForDomain("nonexistent.com")
		assert.Nil(t, peers)
	})

	t.Run("existing domain", func(t *testing.T) {
		// 添加节点
		discoverer.peersMu.Lock()
		discoverer.peers["test.com"] = []types.PeerInfo{
			{ID: "peer1"},
			{ID: "peer2"},
		}
		discoverer.peersMu.Unlock()

		peers := discoverer.PeersForDomain("test.com")
		assert.Len(t, peers, 2)

		// 验证返回的是副本（修改不影响原始数据）
		peers[0].ID = "modified"
		originalPeers := discoverer.PeersForDomain("test.com")
		assert.NotEqual(t, "modified", string(originalPeers[0].ID))
	})
}

// ============================================================================
//                         Reset 测试
// ============================================================================

// TestDiscoverer_Reset 测试重置功能
func TestDiscoverer_Reset(t *testing.T) {
	config := DefaultConfig()
	discoverer := NewDiscoverer(config)

	ctx := context.Background()
	err := discoverer.Start(ctx)
	require.NoError(t, err)
	defer discoverer.Stop(ctx)

	// 添加一些缓存数据
	discoverer.peersMu.Lock()
	discoverer.peers["domain1.com"] = []types.PeerInfo{{ID: "peer1"}}
	discoverer.peersMu.Unlock()

	discoverer.resolver.setCache("test.com", []types.PeerInfo{{ID: "peer2"}})

	// 验证数据存在
	assert.NotEmpty(t, discoverer.AllPeers())
	cached, ok := discoverer.resolver.getFromCache("test.com")
	assert.True(t, ok)
	assert.NotEmpty(t, cached)

	// 执行重置
	err = discoverer.Reset(ctx)
	require.NoError(t, err)

	// 验证缓存已清除
	assert.Empty(t, discoverer.AllPeers())
	_, ok = discoverer.resolver.getFromCache("test.com")
	assert.False(t, ok)
}

// TestDiscoverer_Reset_NotStarted 测试未启动时重置
func TestDiscoverer_Reset_NotStarted(t *testing.T) {
	config := DefaultConfig()
	discoverer := NewDiscoverer(config)

	// 添加缓存
	discoverer.peersMu.Lock()
	discoverer.peers["domain1.com"] = []types.PeerInfo{{ID: "peer1"}}
	discoverer.peersMu.Unlock()

	ctx := context.Background()
	err := discoverer.Reset(ctx)
	require.NoError(t, err)

	// 验证缓存已清除
	assert.Empty(t, discoverer.AllPeers())
}

// ============================================================================
//                         ClearCache 测试
// ============================================================================

// TestResolver_ClearCache 测试清除缓存
func TestResolver_ClearCache(t *testing.T) {
	config := ResolverConfig{
		Timeout:  5 * time.Second,
		MaxDepth: 3,
		CacheTTL: 1 * time.Minute,
	}
	resolver := NewResolver(config)

	// 设置多个缓存条目
	resolver.setCache("domain1.com", []types.PeerInfo{{ID: "peer1"}})
	resolver.setCache("domain2.com", []types.PeerInfo{{ID: "peer2"}})
	resolver.setCache("domain3.com", []types.PeerInfo{{ID: "peer3"}})

	// 验证缓存存在
	_, ok := resolver.getFromCache("domain1.com")
	assert.True(t, ok)

	// 清除缓存
	resolver.ClearCache()

	// 验证所有缓存都被清除
	_, ok = resolver.getFromCache("domain1.com")
	assert.False(t, ok)
	_, ok = resolver.getFromCache("domain2.com")
	assert.False(t, ok)
	_, ok = resolver.getFromCache("domain3.com")
	assert.False(t, ok)
}

// TestResolver_ClearExpiredCache 测试清除过期缓存
func TestResolver_ClearExpiredCache(t *testing.T) {
	config := ResolverConfig{
		Timeout:  5 * time.Second,
		MaxDepth: 3,
		CacheTTL: 100 * time.Millisecond, // 短 TTL
	}
	resolver := NewResolver(config)

	// 设置缓存
	resolver.setCache("fresh.com", []types.PeerInfo{{ID: "peer1"}})
	resolver.setCache("stale.com", []types.PeerInfo{{ID: "peer2"}})

	// 等待一个过期
	time.Sleep(150 * time.Millisecond)

	// fresh.com 已过期，再添加一个新的
	resolver.setCache("new.com", []types.PeerInfo{{ID: "peer3"}})

	// 清除过期缓存
	resolver.ClearExpiredCache()

	// 新缓存应该还在
	_, ok := resolver.getFromCache("new.com")
	assert.True(t, ok, "new.com should still be cached")

	// 旧缓存应该被清除
	_, ok = resolver.getFromCache("fresh.com")
	assert.False(t, ok, "fresh.com should be expired and cleared")
	_, ok = resolver.getFromCache("stale.com")
	assert.False(t, ok, "stale.com should be expired and cleared")
}

// ============================================================================
//                         Stats 测试
// ============================================================================

// TestDiscoverer_Stats 测试统计信息
func TestDiscoverer_Stats(t *testing.T) {
	config := DefaultConfig()
	config.Domains = []string{"domain1.com", "domain2.com"}
	discoverer := NewDiscoverer(config)

	t.Run("empty stats", func(t *testing.T) {
		stats := discoverer.Stats()
		assert.Equal(t, 2, stats.TotalDomains)
		assert.Equal(t, 0, stats.TotalPeers)
		assert.Empty(t, stats.DomainStats)
	})

	t.Run("with peers", func(t *testing.T) {
		discoverer.peersMu.Lock()
		discoverer.peers["domain1.com"] = []types.PeerInfo{
			{ID: "peer1"},
			{ID: "peer2"},
		}
		discoverer.peers["domain2.com"] = []types.PeerInfo{
			{ID: "peer3"},
		}
		discoverer.peersMu.Unlock()

		stats := discoverer.Stats()
		assert.Equal(t, 2, stats.TotalDomains)
		assert.Equal(t, 3, stats.TotalPeers)
		assert.Equal(t, 2, stats.DomainStats["domain1.com"])
		assert.Equal(t, 1, stats.DomainStats["domain2.com"])
	})
}

// ============================================================================
//                         边界条件和错误处理测试
// ============================================================================

// TestDiscoverer_StopBeforeStart 测试未启动就停止
func TestDiscoverer_StopBeforeStart(t *testing.T) {
	config := DefaultConfig()
	discoverer := NewDiscoverer(config)

	ctx := context.Background()
	err := discoverer.Stop(ctx)
	assert.ErrorIs(t, err, ErrNotStarted)
}

// TestDiscoverer_FindPeersNotStarted 测试未启动就查找
func TestDiscoverer_FindPeersNotStarted(t *testing.T) {
	config := DefaultConfig()
	discoverer := NewDiscoverer(config)

	ctx := context.Background()
	_, err := discoverer.FindPeers(ctx, "test")
	assert.ErrorIs(t, err, ErrNotStarted)
}

// TestDiscoverer_ConcurrentReset 测试并发重置
func TestDiscoverer_ConcurrentReset(t *testing.T) {
	config := DefaultConfig()
	config.Domains = []string{"example.com"}
	discoverer := NewDiscoverer(config)

	ctx := context.Background()
	err := discoverer.Start(ctx)
	require.NoError(t, err)
	defer discoverer.Stop(ctx)

	// 添加一些数据
	discoverer.peersMu.Lock()
	discoverer.peers["test.com"] = []types.PeerInfo{{ID: "peer1"}}
	discoverer.peersMu.Unlock()

	// 并发操作
	done := make(chan bool)
	for i := 0; i < 20; i++ {
		go func(id int) {
			defer func() { done <- true }()

			switch id % 4 {
			case 0:
				_ = discoverer.Reset(ctx)
			case 1:
				_ = discoverer.AllPeers()
			case 2:
				_ = discoverer.PeersForDomain("test.com")
			case 3:
				_ = discoverer.Stats()
			}
		}(i)
	}

	// 等待完成
	for i := 0; i < 20; i++ {
		<-done
	}
}

// TestDefaultResolverConfig 测试默认解析器配置
func TestDefaultResolverConfig(t *testing.T) {
	config := DefaultResolverConfig()

	assert.Equal(t, 10*time.Second, config.Timeout)
	assert.Equal(t, 3, config.MaxDepth)
	assert.Equal(t, "", config.CustomResolver)
	assert.Equal(t, 5*time.Minute, config.CacheTTL)
}

// TestValidateDomain_EdgeCases 测试域名验证边界条件
func TestValidateDomain_EdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		domain  string
		wantErr bool
		reason  string
	}{
		{
			name:    "very long domain",
			domain:  strings.Repeat("a", 254),
			wantErr: true,
			reason:  "exceeds 253 chars",
		},
		{
			name:    "max valid length",
			domain:  strings.Repeat("a", 63) + "." + strings.Repeat("b", 63) + "." + strings.Repeat("c", 63) + "." + strings.Repeat("d", 60),
			wantErr: false,
			reason:  "253 chars with valid labels",
		},
		{
			name:    "label too long",
			domain:  strings.Repeat("a", 64) + ".com",
			wantErr: true,
			reason:  "label exceeds 63 chars",
		},
		{
			name:    "max valid label",
			domain:  strings.Repeat("a", 63) + ".com",
			wantErr: false,
			reason:  "label exactly 63 chars",
		},
		{
			name:    "double dot",
			domain:  "example..com",
			wantErr: true,
			reason:  "empty label",
		},
		{
			name:    "starts with digit",
			domain:  "1example.com",
			wantErr: false,
			reason:  "digit is valid start",
		},
		{
			name:    "underscore in label",
			domain:  "ex_ample.com",
			wantErr: false,
			reason:  "underscore is allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDomain(tt.domain)
			if tt.wantErr {
				assert.Error(t, err, "Should error: %s", tt.reason)
			} else {
				assert.NoError(t, err, "Should not error: %s", tt.reason)
			}
		})
	}
}
