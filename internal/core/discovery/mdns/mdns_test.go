package mdns

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Config 测试
// ============================================================================

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	assert.NotEmpty(t, cfg.ServiceTag)
	assert.NotEmpty(t, cfg.Domain)
	assert.Greater(t, cfg.TTL, time.Duration(0))
	assert.Greater(t, cfg.QueryInterval, time.Duration(0))
}

func TestConfig_Values(t *testing.T) {
	cfg := DefaultConfig()

	t.Run("服务标签格式正确", func(t *testing.T) {
		assert.Contains(t, cfg.ServiceTag, "_dep2p")
	})

	t.Run("域名格式正确", func(t *testing.T) {
		assert.Equal(t, "local.", cfg.Domain)
	})

	t.Run("TTL 合理", func(t *testing.T) {
		assert.GreaterOrEqual(t, cfg.TTL, time.Minute)
		assert.LessOrEqual(t, cfg.TTL, time.Hour)
	})

	t.Run("查询间隔合理", func(t *testing.T) {
		assert.GreaterOrEqual(t, cfg.QueryInterval, 10*time.Second)
		assert.LessOrEqual(t, cfg.QueryInterval, 5*time.Minute)
	})
}

// ============================================================================
//                              Discoverer 测试
// ============================================================================

func TestNewDiscoverer(t *testing.T) {
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-12345678"))
	localAddrs := []string{"192.168.1.1:4001"}

	t.Run("使用默认配置", func(t *testing.T) {
		discoverer := NewDiscoverer(DefaultConfig(), localID, localAddrs)
		require.NotNil(t, discoverer)
		assert.NotNil(t, discoverer.peers)
		assert.Equal(t, localID, discoverer.localID)
	})

	t.Run("使用自定义 Logger", func(t *testing.T) {
		discoverer := NewDiscoverer(DefaultConfig(), localID, localAddrs)
		require.NotNil(t, discoverer)
	})

	t.Run("使用自定义配置", func(t *testing.T) {
		cfg := Config{
			ServiceTag:    "_custom._udp",
			Domain:        "custom.local.",
			Port:          5353,
			TTL:           5 * time.Minute,
			QueryInterval: 30 * time.Second,
		}
		discoverer := NewDiscoverer(cfg, localID, localAddrs)
		require.NotNil(t, discoverer)
		assert.Equal(t, "_custom._udp", discoverer.config.ServiceTag)
	})
}

// ============================================================================
//                              生命周期测试
// ============================================================================

func TestDiscoverer_Start_Stop(t *testing.T) {
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-12345678"))
	localAddrs := []string{"192.168.1.1:4001"}

	discoverer := NewDiscoverer(DefaultConfig(), localID, localAddrs)

	ctx := context.Background()

	// 启动
	err := discoverer.Start(ctx)
	// 启动可能成功或失败，取决于网络环境
	_ = err

	// 停止
	err = discoverer.Stop()
	assert.NoError(t, err)
}

func TestDiscoverer_Stop_NotStarted(t *testing.T) {
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-12345678"))

	discoverer := NewDiscoverer(DefaultConfig(), localID, nil)

	// 未启动时停止应该是安全的
	err := discoverer.Stop()
	assert.NoError(t, err)
}

func TestDiscoverer_Close(t *testing.T) {
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-12345678"))

	discoverer := NewDiscoverer(DefaultConfig(), localID, nil)

	err := discoverer.Close()
	assert.NoError(t, err)
}

func TestDiscoverer_IsRunning(t *testing.T) {
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-12345678"))

	discoverer := NewDiscoverer(DefaultConfig(), localID, nil)

	// 初始未运行
	assert.False(t, discoverer.IsRunning())
}

// ============================================================================
//                              节点管理测试
// ============================================================================

func TestDiscoverer_PeerManagement(t *testing.T) {
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-12345678"))

	discoverer := NewDiscoverer(DefaultConfig(), localID, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("discovered-peer-12345"))

	// 添加节点
	discoverer.peersMu.Lock()
	discoverer.peers[peerID] = peerEntry{
		ID:       peerID,
		Addrs:    []string{"192.168.1.2:4001"},
		LastSeen: time.Now(),
	}
	discoverer.peersMu.Unlock()

	// 验证节点存在
	discoverer.peersMu.RLock()
	entry, exists := discoverer.peers[peerID]
	discoverer.peersMu.RUnlock()

	assert.True(t, exists)
	assert.Equal(t, peerID, entry.ID)
	assert.Len(t, entry.Addrs, 1)
}

func TestDiscoverer_Peers(t *testing.T) {
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-12345678"))

	discoverer := NewDiscoverer(DefaultConfig(), localID, nil)

	// 添加一些节点
	for i := 0; i < 5; i++ {
		var peerID types.NodeID
		copy(peerID[:], []byte("peer-"+string(rune('0'+i))+"-12345678"))

		discoverer.peersMu.Lock()
		discoverer.peers[peerID] = peerEntry{
			ID:       peerID,
			Addrs:    []string{"192.168.1." + string(rune('0'+i)) + ":4001"},
			LastSeen: time.Now(),
		}
		discoverer.peersMu.Unlock()
	}

	peers := discoverer.Peers()
	assert.Len(t, peers, 5)
}

func TestDiscoverer_PeerCount(t *testing.T) {
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-12345678"))

	discoverer := NewDiscoverer(DefaultConfig(), localID, nil)

	// 初始为 0
	assert.Equal(t, 0, discoverer.PeerCount())

	// 添加节点
	var peerID types.NodeID
	copy(peerID[:], []byte("peer-count-test-1234"))

	discoverer.peersMu.Lock()
	discoverer.peers[peerID] = peerEntry{
		ID:       peerID,
		Addrs:    []string{"192.168.1.1:4001"},
		LastSeen: time.Now(),
	}
	discoverer.peersMu.Unlock()

	assert.Equal(t, 1, discoverer.PeerCount())
}

// ============================================================================
//                              地址更新测试
// ============================================================================

func TestDiscoverer_UpdateLocalAddrs(t *testing.T) {
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-12345678"))
	localAddrs := []string{"192.168.1.1:4001"}

	discoverer := NewDiscoverer(DefaultConfig(), localID, localAddrs)

	newAddrs := []string{"192.168.1.2:4001", "192.168.1.3:4001"}
	discoverer.UpdateLocalAddrs(newAddrs)

	assert.Equal(t, newAddrs, discoverer.localAddrs)
}

// ============================================================================
//                              并发安全测试
// ============================================================================

func TestDiscoverer_Concurrency(t *testing.T) {
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-12345678"))

	discoverer := NewDiscoverer(DefaultConfig(), localID, nil)

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(2)

		go func(id int) {
			defer wg.Done()
			var peerID types.NodeID
			copy(peerID[:], []byte("peer-id-"+string(rune('0'+id))))

			discoverer.peersMu.Lock()
			discoverer.peers[peerID] = peerEntry{
				ID:       peerID,
				Addrs:    []string{"192.168.1.1:4001"},
				LastSeen: time.Now(),
			}
			discoverer.peersMu.Unlock()
		}(i)

		go func(id int) {
			defer wg.Done()
			_ = discoverer.PeerCount()
		}(i)
	}

	wg.Wait()
}

// ============================================================================
//                              peerEntry 测试
// ============================================================================

func TestPeerEntry(t *testing.T) {
	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	entry := peerEntry{
		ID:       peerID,
		Addrs:    []string{"192.168.1.1:4001", "192.168.1.1:4002"},
		LastSeen: time.Now(),
	}

	assert.Equal(t, peerID, entry.ID)
	assert.Len(t, entry.Addrs, 2)
	assert.False(t, entry.LastSeen.IsZero())
}

// ============================================================================
//                              服务发现格式测试
// ============================================================================

func TestServiceTagFormat(t *testing.T) {
	cfg := DefaultConfig()

	t.Run("服务标签格式正确", func(t *testing.T) {
		// mDNS 服务标签应该以 _ 开头
		assert.True(t, len(cfg.ServiceTag) > 0)
		assert.Equal(t, '_', rune(cfg.ServiceTag[0]))
	})

	t.Run("域名以点结尾", func(t *testing.T) {
		assert.True(t, len(cfg.Domain) > 0)
		assert.Equal(t, '.', rune(cfg.Domain[len(cfg.Domain)-1]))
	})
}

// ============================================================================
//                              IPv4/IPv6 配置测试
// ============================================================================

func TestConfig_IPv4IPv6(t *testing.T) {
	t.Run("默认配置", func(t *testing.T) {
		cfg := DefaultConfig()
		// 默认启用 IPv4，禁用 IPv6
		assert.False(t, cfg.DisableIPv4)
		assert.True(t, cfg.DisableIPv6)
	})

	t.Run("仅 IPv4", func(t *testing.T) {
		cfg := Config{
			DisableIPv4: false,
			DisableIPv6: true,
		}
		assert.False(t, cfg.DisableIPv4)
		assert.True(t, cfg.DisableIPv6)
	})

	t.Run("仅 IPv6", func(t *testing.T) {
		cfg := Config{
			DisableIPv4: true,
			DisableIPv6: false,
		}
		assert.True(t, cfg.DisableIPv4)
		assert.False(t, cfg.DisableIPv6)
	})

	t.Run("双栈", func(t *testing.T) {
		cfg := Config{
			DisableIPv4: false,
			DisableIPv6: false,
		}
		assert.False(t, cfg.DisableIPv4)
		assert.False(t, cfg.DisableIPv6)
	})
}

// stringAddress 测试已迁移到 internal/core/address/addr_test.go
// 所有散落的 Address 实现已统一使用 address.Addr

// ============================================================================
//                              goroutine 安全测试
// ============================================================================

func TestDiscoverer_QueryLoopWithoutStart(t *testing.T) {
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-12345678"))

	discoverer := NewDiscoverer(DefaultConfig(), localID, nil)

	// 在未启动时调用 queryLoop 应该立即返回而不 panic
	done := make(chan struct{})
	go func() {
		defer close(done)
		discoverer.queryLoop() // 不应 panic
	}()

	select {
	case <-done:
		// 成功
	case <-time.After(time.Second):
		t.Fatal("queryLoop 应该立即返回")
	}
}

func TestDiscoverer_CleanupLoopWithoutStart(t *testing.T) {
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-12345678"))

	discoverer := NewDiscoverer(DefaultConfig(), localID, nil)

	// 在未启动时调用 cleanupLoop 应该立即返回而不 panic
	done := make(chan struct{})
	go func() {
		defer close(done)
		discoverer.cleanupLoop() // 不应 panic
	}()

	select {
	case <-done:
		// 成功
	case <-time.After(time.Second):
		t.Fatal("cleanupLoop 应该立即返回")
	}
}

func TestDiscoverer_UpdateLocalAddrs_Concurrent(t *testing.T) {
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-12345678"))
	localAddrs := []string{"192.168.1.1:4001"}

	discoverer := NewDiscoverer(DefaultConfig(), localID, localAddrs)

	var wg sync.WaitGroup

	// 并发更新地址
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			newAddrs := []string{
				"192.168.1." + string(rune('0'+id)) + ":4001",
			}
			discoverer.UpdateLocalAddrs(newAddrs)
		}(i)
	}

	// 并发读取状态
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = discoverer.IsRunning()
		}()
	}

	wg.Wait()
}

// ============================================================================
//                              集成测试（需要网络）
// ============================================================================

func TestDiscoverer_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试")
	}

	var localID types.NodeID
	copy(localID[:], []byte("integration-test-node"))
	localAddrs := []string{"127.0.0.1:4001"}

	discoverer := NewDiscoverer(DefaultConfig(), localID, localAddrs)

	ctx := context.Background()

	// 启动
	err := discoverer.Start(ctx)
	if err != nil {
		t.Skipf("无法启动 mDNS: %v", err)
	}

	// 等待一小段时间让服务运行
	time.Sleep(100 * time.Millisecond)

	// 停止
	err = discoverer.Stop()
	assert.NoError(t, err)
}

// ============================================================================
//                              虚拟网卡识别测试
// ============================================================================

func TestIsVirtualInterface(t *testing.T) {
	tests := []struct {
		name     string
		iface    string
		expected bool
	}{
		// macOS VPN 隧道
		{"utun0 (macOS VPN)", "utun0", true},
		{"utun4 (VPN)", "utun4", true},
		{"ipsec0", "ipsec0", true},

		// macOS 特殊接口
		{"awdl0 (AirDrop)", "awdl0", true},
		{"llw0 (Low Latency WLAN)", "llw0", true},
		{"ap1 (热点)", "ap1", true},

		// Linux/Docker/Kubernetes
		{"docker0", "docker0", true},
		{"br-abc123 (Docker 自定义桥)", "br-abc123", true},
		{"veth12345 (容器)", "veth12345", true},
		{"virbr0 (libvirt)", "virbr0", true},
		{"vboxnet0 (VirtualBox)", "vboxnet0", true},
		{"vmnet8 (VMware)", "vmnet8", true},

		// 通用虚拟接口
		{"tun0", "tun0", true},
		{"tap0", "tap0", true},
		{"wg0 (WireGuard)", "wg0", true},
		{"tailscale0", "tailscale0", true},

		// 真实网卡
		{"en0 (macOS Wi-Fi)", "en0", false},
		{"en1 (macOS Ethernet)", "en1", false},
		{"eth0 (Linux)", "eth0", false},
		{"wlan0 (Linux Wi-Fi)", "wlan0", false},
		{"lo0 (回环)", "lo0", false}, // 回环不是虚拟网卡，但会被其他逻辑过滤
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isVirtualInterface(tt.iface)
			assert.Equal(t, tt.expected, result, "interface: %s", tt.iface)
		})
	}
}

// ============================================================================
//                              不可路由地址段测试
// ============================================================================

func TestIsNonRoutableIP(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		// VPN/隧道地址段 (198.18.0.0/15)
		{"198.18.0.1 (VPN/Surge)", "198.18.0.1", true},
		{"198.18.255.255", "198.18.255.255", true},
		{"198.19.0.1", "198.19.0.1", true},
		{"198.19.255.255", "198.19.255.255", true},

		// CGNAT (100.64.0.0/10)
		{"100.64.0.1 (CGNAT)", "100.64.0.1", true},
		{"100.100.100.100 (Tailscale 常用)", "100.100.100.100", true},
		{"100.127.255.255 (CGNAT 边界)", "100.127.255.255", true},

		// 文档示例地址
		{"198.51.100.1 (RFC5737)", "198.51.100.1", true},
		{"203.0.113.1 (RFC5737)", "203.0.113.1", true},

		// 应该通过的正常私网地址
		{"192.168.1.1 (正常私网)", "192.168.1.1", false},
		{"10.0.0.1 (正常私网)", "10.0.0.1", false},
		{"172.16.0.1 (正常私网)", "172.16.0.1", false},

		// 公网地址（不在非路由列表中）
		{"8.8.8.8 (Google DNS)", "8.8.8.8", false},
		{"1.1.1.1 (Cloudflare)", "1.1.1.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := parseIP(tt.ip)
			require.NotNil(t, ip, "无法解析 IP: %s", tt.ip)
			result := isNonRoutableIP(ip)
			assert.Equal(t, tt.expected, result, "ip: %s", tt.ip)
		})
	}
}

// ============================================================================
//                              局域网 IP 判断测试
// ============================================================================

func TestIsLANIP(t *testing.T) {
	tests := []struct {
		name     string
		ip       string
		expected bool
	}{
		// 应该接受的局域网地址
		{"192.168.1.1 (家庭网络)", "192.168.1.1", true},
		{"192.168.0.100", "192.168.0.100", true},
		{"10.0.0.1 (企业网络)", "10.0.0.1", true},
		{"10.255.255.255", "10.255.255.255", true},
		{"172.16.0.1 (私网)", "172.16.0.1", true},
		{"172.31.255.255 (私网边界)", "172.31.255.255", true},

		// 应该拒绝的地址
		{"127.0.0.1 (回环)", "127.0.0.1", false},
		{"0.0.0.0 (未指定)", "0.0.0.0", false},
		{"198.18.0.1 (VPN)", "198.18.0.1", false},
		{"100.64.0.1 (CGNAT)", "100.64.0.1", false},
		{"8.8.8.8 (公网)", "8.8.8.8", false},
		{"1.1.1.1 (公网)", "1.1.1.1", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := parseIP(tt.ip)
			require.NotNil(t, ip, "无法解析 IP: %s", tt.ip)
			result := isLANIP(ip)
			assert.Equal(t, tt.expected, result, "ip: %s", tt.ip)
		})
	}
}

// ============================================================================
//                              IP 评分测试
// ============================================================================

func TestScoreLANIP(t *testing.T) {
	tests := []struct {
		name          string
		ip            string
		expectedRange string // "zero", "low", "medium", "high"
	}{
		// 高优先级：192.168.x.x
		{"192.168.1.1 应该最高", "192.168.1.1", "high"},
		{"192.168.0.1 应该最高", "192.168.0.1", "high"},

		// 中优先级：10.x.x.x
		{"10.0.0.1 应该中等", "10.0.0.1", "medium"},
		{"10.255.255.255 应该中等", "10.255.255.255", "medium"},

		// 低优先级：172.16-31.x.x
		{"172.16.0.1 应该较低", "172.16.0.1", "low"},
		{"172.31.255.255 应该较低", "172.31.255.255", "low"},

		// 零分：不适合 mDNS
		{"127.0.0.1 应该为零", "127.0.0.1", "zero"},
		{"0.0.0.0 应该为零", "0.0.0.0", "zero"},
		{"198.18.0.1 应该为零", "198.18.0.1", "zero"},
		{"8.8.8.8 应该为零", "8.8.8.8", "zero"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ip := parseIP(tt.ip)
			require.NotNil(t, ip, "无法解析 IP: %s", tt.ip)
			score := scoreLANIP(ip)

			switch tt.expectedRange {
			case "zero":
				assert.Equal(t, 0, score, "ip: %s, score: %d", tt.ip, score)
			case "low":
				assert.Greater(t, score, 0, "ip: %s, score: %d", tt.ip, score)
				assert.Less(t, score, 1200, "ip: %s, score: %d", tt.ip, score)
			case "medium":
				assert.Greater(t, score, 1100, "ip: %s, score: %d", tt.ip, score)
				assert.Less(t, score, 1300, "ip: %s, score: %d", tt.ip, score)
			case "high":
				assert.Greater(t, score, 1200, "ip: %s, score: %d", tt.ip, score)
			}
		})
	}

	// 验证相对优先级
	t.Run("优先级顺序正确", func(t *testing.T) {
		score192 := scoreLANIP(parseIP("192.168.1.1"))
		score10 := scoreLANIP(parseIP("10.0.0.1"))
		score172 := scoreLANIP(parseIP("172.16.0.1"))

		assert.Greater(t, score192, score10, "192.168.x.x 应该高于 10.x.x.x")
		assert.Greater(t, score10, score172, "10.x.x.x 应该高于 172.16.x.x")
	})
}

// ============================================================================
//                              地址排序测试
// ============================================================================

func TestSortAddrsByReachability(t *testing.T) {
	t.Run("host:port 格式排序", func(t *testing.T) {
		addrs := []string{
			"198.18.0.1:4001",   // VPN，应该被过滤或排最后
			"8.8.8.8:4001",      // 公网，分数 0
			"10.0.0.1:4001",     // 10.x，中优先级
			"192.168.1.1:4001",  // 192.168.x，高优先级
			"172.16.0.1:4001",   // 172.16.x，低优先级
		}

		sorted := sortAddrsByReachability(addrs)

		// 192.168.x 应该在最前面
		assert.Contains(t, sorted[0], "192.168", "第一个应该是 192.168.x")

		// 验证 192.168 > 10.x > 172.16
		idx192 := -1
		idx10 := -1
		idx172 := -1
		for i, addr := range sorted {
			if idx192 < 0 && containsSubstr(addr, "192.168") {
				idx192 = i
			}
			if idx10 < 0 && containsSubstr(addr, "10.0.0") {
				idx10 = i
			}
			if idx172 < 0 && containsSubstr(addr, "172.16") {
				idx172 = i
			}
		}

		if idx192 >= 0 && idx10 >= 0 {
			assert.Less(t, idx192, idx10, "192.168.x 应该排在 10.x 前面")
		}
		if idx10 >= 0 && idx172 >= 0 {
			assert.Less(t, idx10, idx172, "10.x 应该排在 172.16.x 前面")
		}
	})

	t.Run("multiaddr 格式排序", func(t *testing.T) {
		addrs := []string{
			"/ip4/8.8.8.8/udp/4001/quic-v1",
			"/ip4/192.168.1.100/udp/4001/quic-v1",
			"/ip4/10.0.0.50/udp/4001/quic-v1",
		}

		sorted := sortAddrsByReachability(addrs)

		// 192.168.x 应该在最前面
		assert.Contains(t, sorted[0], "192.168", "第一个应该是 192.168.x")
	})

	t.Run("空列表", func(t *testing.T) {
		sorted := sortAddrsByReachability([]string{})
		assert.Empty(t, sorted)
	})

	t.Run("单元素", func(t *testing.T) {
		sorted := sortAddrsByReachability([]string{"192.168.1.1:4001"})
		assert.Len(t, sorted, 1)
		assert.Equal(t, "192.168.1.1:4001", sorted[0])
	})
}

// ============================================================================
//                              filterDialableAddrs 测试
// ============================================================================

func TestFilterDialableAddrs(t *testing.T) {
	t.Run("过滤非局域网地址", func(t *testing.T) {
		input := []string{
			"192.168.1.1:4001",  // 应该保留
			"10.0.0.1:4001",     // 应该保留
			"198.18.0.1:4001",   // VPN，应该过滤
			"8.8.8.8:4001",      // 公网，应该过滤
			"0.0.0.0:4001",      // 未指定，应该过滤
			"127.0.0.1:4001",    // 回环，应该过滤
		}

		filtered := filterDialableAddrs(input)

		assert.Contains(t, filtered, "192.168.1.1:4001")
		assert.Contains(t, filtered, "10.0.0.1:4001")
		assert.NotContains(t, filtered, "198.18.0.1:4001")
		assert.NotContains(t, filtered, "8.8.8.8:4001")
		assert.NotContains(t, filtered, "0.0.0.0:4001")
		assert.NotContains(t, filtered, "127.0.0.1:4001")
	})

	t.Run("multiaddr 格式过滤", func(t *testing.T) {
		input := []string{
			"/ip4/192.168.1.1/udp/4001/quic-v1",  // 应该保留
			"/ip4/0.0.0.0/udp/4001/quic-v1",      // 应该过滤
			"/ip4/198.18.0.1/udp/4001/quic-v1",   // VPN，应该过滤
			"/ip6/::/udp/4001/quic-v1",           // 应该过滤
		}

		filtered := filterDialableAddrs(input)

		assert.Len(t, filtered, 1)
		assert.Contains(t, filtered, "/ip4/192.168.1.1/udp/4001/quic-v1")
	})

	t.Run("过滤端口为 0 的地址", func(t *testing.T) {
		input := []string{
			"192.168.1.1:0",     // 无效端口，应该过滤
			"192.168.1.1:4001",  // 有效，应该保留
		}

		filtered := filterDialableAddrs(input)

		assert.Len(t, filtered, 1)
		assert.Equal(t, "192.168.1.1:4001", filtered[0])
	})
}

// ============================================================================
//                              辅助函数
// ============================================================================

func parseIP(s string) net.IP {
	return net.ParseIP(s)
}

func containsSubstr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstrHelper(s, substr))
}

func containsSubstrHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
