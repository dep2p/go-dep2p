package host

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/dep2p/go-dep2p/tests/mocks"
)

// TestAddrsManager_Creation 测试地址管理器创建
func TestAddrsManager_Creation(t *testing.T) {
	localPeerID := types.PeerID("test-peer-id")
	mockSwarm := mocks.NewMockSwarm(string(localPeerID))

	mgr := newAddrsManager(mockSwarm, localPeerID, nil)
	assert.NotNil(t, mgr)
	assert.NotNil(t, mgr.observedAddrs)
	assert.Equal(t, localPeerID, mgr.localPeerID)
}

// TestAddrsManager_Addrs 测试获取地址
func TestAddrsManager_Addrs(t *testing.T) {
	localPeerID := types.PeerID("test-peer-id")
	mockSwarm := mocks.NewMockSwarm(string(localPeerID))

	mgr := newAddrsManager(mockSwarm, localPeerID, nil)

	// 初始应该返回空地址列表
	addrs := mgr.Addrs()
	assert.NotNil(t, addrs)
	assert.Len(t, addrs, 0)

	// 更新监听地址
	listenAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	mgr.updateListenAddrs([]types.Multiaddr{listenAddr})

	// 应该返回带 /p2p/<peerID> 后缀的地址
	addrs = mgr.Addrs()
	assert.Len(t, addrs, 1)
	assert.Contains(t, addrs[0], "/p2p/test-peer-id")
}

// TestAddrsManager_DefaultFactory 测试默认地址工厂
func TestAddrsManager_DefaultFactory(t *testing.T) {
	localPeerID := types.PeerID("test-peer-id")
	mockSwarm := mocks.NewMockSwarm(string(localPeerID))

	// 使用默认工厂（nil）
	mgr := newAddrsManager(mockSwarm, localPeerID, nil)

	// 添加多个地址
	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	addr2, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	mgr.updateListenAddrs([]types.Multiaddr{addr1, addr2})

	// 默认工厂不过滤任何地址
	addrs := mgr.Addrs()
	assert.Len(t, addrs, 2)
}

// TestAddrsManager_CustomFactory 测试自定义地址工厂
func TestAddrsManager_CustomFactory(t *testing.T) {
	localPeerID := types.PeerID("test-peer-id")
	mockSwarm := mocks.NewMockSwarm(string(localPeerID))

	// 自定义工厂：过滤掉 localhost
	factory := func(addrs []types.Multiaddr) []types.Multiaddr {
		var filtered []types.Multiaddr
		for _, addr := range addrs {
			addrStr := addr.String()
			if !containsSubstring(addrStr, "127.0.0.1") {
				filtered = append(filtered, addr)
			}
		}
		return filtered
	}

	mgr := newAddrsManager(mockSwarm, localPeerID, factory)

	// 添加地址
	addr1, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	addr2, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	mgr.updateListenAddrs([]types.Multiaddr{addr1, addr2})

	// 应该只返回非 localhost 地址
	addrs := mgr.Addrs()
	assert.Len(t, addrs, 1)
	assert.Contains(t, addrs[0], "192.168.1.1")
}

// containsSubstring 检查字符串是否包含子串
func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestAddrsFactory_FilterLocalhost 测试过滤 localhost
func TestAddrsFactory_FilterLocalhost(t *testing.T) {
	// 创建过滤 localhost 的工厂
	filterLocalhost := func(addrs []types.Multiaddr) []types.Multiaddr {
		var filtered []types.Multiaddr
		for _, addr := range addrs {
			// 检查是否是 localhost
			ipStr, err := addr.ValueForProtocol(types.ProtocolIP4)
			if err != nil {
				filtered = append(filtered, addr)
				continue
			}
			ip := net.ParseIP(ipStr)
			if ip != nil && !ip.IsLoopback() {
				filtered = append(filtered, addr)
			}
		}
		return filtered
	}

	// 测试地址
	localAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	publicAddr, _ := types.NewMultiaddr("/ip4/8.8.8.8/tcp/4001")

	result := filterLocalhost([]types.Multiaddr{localAddr, publicAddr})
	assert.Len(t, result, 1)
	assert.Contains(t, result[0].String(), "8.8.8.8")
}

// TestAddrsFactory_FilterPrivate 测试过滤私有地址
func TestAddrsFactory_FilterPrivate(t *testing.T) {
	// 创建过滤私有地址的工厂
	filterPrivate := func(addrs []types.Multiaddr) []types.Multiaddr {
		var filtered []types.Multiaddr
		for _, addr := range addrs {
			ipStr, err := addr.ValueForProtocol(types.ProtocolIP4)
			if err != nil {
				filtered = append(filtered, addr)
				continue
			}
			ip := net.ParseIP(ipStr)
			if ip != nil && !ip.IsPrivate() && !ip.IsLoopback() {
				filtered = append(filtered, addr)
			}
		}
		return filtered
	}

	// 测试地址
	privateAddr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	publicAddr, _ := types.NewMultiaddr("/ip4/8.8.8.8/tcp/4001")

	result := filterPrivate([]types.Multiaddr{privateAddr, publicAddr})
	assert.Len(t, result, 1)
	assert.Contains(t, result[0].String(), "8.8.8.8")
}

// TestAddrsManager_ObservedAddrs 测试观测地址管理
func TestAddrsManager_ObservedAddrs(t *testing.T) {
	localPeerID := types.PeerID("test-peer-id")
	mockSwarm := mocks.NewMockSwarm(string(localPeerID))

	mgr := newAddrsManager(mockSwarm, localPeerID, nil)

	// 直接通过 observedAddrs 添加
	observedAddr, _ := types.NewMultiaddr("/ip4/8.8.8.8/tcp/4001")
	if mgr.observedAddrs != nil {
		mgr.observedAddrs.Add(observedAddr)
	}

	// 验证观测地址被添加
	require.NotNil(t, mgr.observedAddrs)
	topAddrs := mgr.observedAddrs.TopAddrs(5)
	assert.Len(t, topAddrs, 1)
}

// TestAddrsManager_AllAddrs 测试获取所有地址
func TestAddrsManager_AllAddrs(t *testing.T) {
	localPeerID := types.PeerID("test-peer-id")
	mockSwarm := mocks.NewMockSwarm(string(localPeerID))

	mgr := newAddrsManager(mockSwarm, localPeerID, nil)

	// 添加监听地址
	listenAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")
	mgr.updateListenAddrs([]types.Multiaddr{listenAddr})

	// 直接通过 observedAddrs 添加观测地址
	observedAddr, _ := types.NewMultiaddr("/ip4/8.8.8.8/tcp/4001")
	if mgr.observedAddrs != nil {
		mgr.observedAddrs.Add(observedAddr)
	}

	// AllAddrs 应该包含两种地址
	allAddrs := mgr.AllAddrs()
	assert.GreaterOrEqual(t, len(allAddrs), 1)
}

// TestObservedAddrManager_TopAddrs 测试观测地址排序
func TestObservedAddrManager_TopAddrs(t *testing.T) {
	oam := newObservedAddrManager()

	// 添加多个地址，不同观测次数
	addr1, _ := types.NewMultiaddr("/ip4/1.1.1.1/tcp/4001")
	addr2, _ := types.NewMultiaddr("/ip4/2.2.2.2/tcp/4001")

	// addr1 观测 3 次
	oam.Add(addr1)
	oam.Add(addr1)
	oam.Add(addr1)

	// addr2 观测 1 次
	oam.Add(addr2)

	// 获取 top 地址，应该 addr1 排前面（置信度更高）
	topAddrs := oam.TopAddrs(2)
	assert.Len(t, topAddrs, 2)
	assert.Contains(t, topAddrs[0].String(), "1.1.1.1")
}

// TestAddrsManager_AppendPeerID 测试添加 PeerID 后缀
func TestAddrsManager_AppendPeerID(t *testing.T) {
	localPeerID := types.PeerID("QmTest123")
	mockSwarm := mocks.NewMockSwarm(string(localPeerID))

	mgr := newAddrsManager(mockSwarm, localPeerID, nil)

	// 测试不带 /p2p 后缀的地址
	result := mgr.appendPeerID("/ip4/127.0.0.1/tcp/4001")
	assert.Equal(t, "/ip4/127.0.0.1/tcp/4001/p2p/QmTest123", result)

	// 测试已带 /p2p 后缀的地址
	result = mgr.appendPeerID("/ip4/127.0.0.1/tcp/4001/p2p/OtherPeer")
	assert.Equal(t, "/ip4/127.0.0.1/tcp/4001/p2p/OtherPeer", result)

	// 测试空地址
	result = mgr.appendPeerID("")
	assert.Equal(t, "", result)
}

// ============================================================================
// 新增测试（提升覆盖率）
// ============================================================================

// TestAddrsManager_SetReachabilityCoordinator 测试设置可达性协调器
func TestAddrsManager_SetReachabilityCoordinator(t *testing.T) {
	localPeerID := types.PeerID("test-peer-id")
	mockSwarm := mocks.NewMockSwarm(string(localPeerID))
	mgr := newAddrsManager(mockSwarm, localPeerID, nil)

	// 设置 nil 协调器应该不 panic
	mgr.SetReachabilityCoordinator(nil)
	assert.Nil(t, mgr.coordinator)
}

// TestAddrsManager_AdvertisedAddrs 测试广告地址（无 Coordinator）
func TestAddrsManager_AdvertisedAddrs(t *testing.T) {
	localPeerID := types.PeerID("test-peer-id")
	mockSwarm := mocks.NewMockSwarm(string(localPeerID))
	mgr := newAddrsManager(mockSwarm, localPeerID, nil)

	// 无 Coordinator，只返回监听地址
	listenAddr, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001")
	mgr.updateListenAddrs([]types.Multiaddr{listenAddr})

	addrs := mgr.AdvertisedAddrs()
	assert.NotEmpty(t, addrs)
	// 验证包含监听地址
	assert.True(t, len(addrs) > 0)
}

// TestAddrsManager_ShareableAddrs 测试可分享地址
func TestAddrsManager_ShareableAddrs(t *testing.T) {
	localPeerID := types.PeerID("test-peer-id")
	mockSwarm := mocks.NewMockSwarm(string(localPeerID))
	mgr := newAddrsManager(mockSwarm, localPeerID, nil)

	// 无 Coordinator，返回可连接的监听地址
	listenAddr1, _ := types.NewMultiaddr("/ip4/0.0.0.0/tcp/4001")     // 不可连接
	listenAddr2, _ := types.NewMultiaddr("/ip4/192.168.1.1/tcp/4001") // 可连接
	mgr.updateListenAddrs([]types.Multiaddr{listenAddr1, listenAddr2})

	addrs := mgr.ShareableAddrs()
	// 0.0.0.0 应该被过滤掉
	for _, addr := range addrs {
		assert.NotContains(t, addr, "0.0.0.0")
	}
}

// TestIsConnectableAddr 测试地址可连接性判断
func TestIsConnectableAddr(t *testing.T) {
	tests := []struct {
		addr     string
		expected bool
	}{
		{"/ip4/0.0.0.0/tcp/4001", false},        // 未指定地址
		{"/ip6/::/tcp/4001", false},             // IPv6 未指定地址
		{"/ip4/127.0.0.1/tcp/4001", false},      // 回环地址
		{"/ip4/127.0.0.2/tcp/4001", false},      // 回环地址段
		{"/ip6/::1/tcp/4001", false},            // IPv6 回环
		{"/ip4/192.168.1.1/tcp/4001", true},     // 私有地址（可连接）
		{"/ip4/203.0.113.1/tcp/4001", true},     // 公网地址
		{"", false},                             // 空地址
		// 
		// Relay 地址是合法的可连接地址，用于 ShareableAddrs 和 AdvertisedAddrs
		{"/ip4/101.37.245.124/udp/4005/quic-v1/p2p/9gMvzMGsyDFGRSUtzH6DXqyRjs1TDabPakAghZLHnrF6/p2p-circuit/p2p/HdqTYLnhvsgW4GNpUt7i3UyEsaA9HGU34U5DG8pwhunc", true}, // Relay 地址是可连接的
		{"/p2p-circuit/p2p/QmPeer", true},       // 纯 Relay 地址也是可连接的
	}

	for _, tc := range tests {
		t.Run(tc.addr, func(t *testing.T) {
			result := isConnectableAddr(tc.addr)
			assert.Equal(t, tc.expected, result, "isConnectableAddr(%s)", tc.addr)
		})
	}
}

// TestIsDirectConnectableAddr_BUG29_RelayFilter 测试 
func TestIsDirectConnectableAddr_BUG29_RelayFilter(t *testing.T) {
	// 典型的 Relay 地址场景 - 应该被 isDirectConnectableAddr 过滤
	relayAddrs := []string{
		// 完整的 Relay Circuit 地址
		"/ip4/101.37.245.124/udp/4005/quic-v1/p2p/9gMvzMGsyDFGRSUtzH6DXqyRjs1TDabPakAghZLHnrF6/p2p-circuit/p2p/HdqTYLnhvsgW4GNpUt7i3UyEsaA9HGU34U5DG8pwhunc",
		// 另一种 Relay 地址格式
		"/ip4/8.8.8.8/tcp/4001/p2p/QmRelay/p2p-circuit/p2p/QmTarget",
		// 纯 p2p-circuit
		"/p2p-circuit/p2p/QmPeer",
	}

	for _, addr := range relayAddrs {
		t.Run(addr[:20]+"...", func(t *testing.T) {
			// isConnectableAddr 应该通过（Relay 是合法连接）
			assert.True(t, isConnectableAddr(addr), "isConnectableAddr 应该通过 Relay 地址: %s", addr)
			// isDirectConnectableAddr 应该过滤（打洞不能用 Relay）
			assert.False(t, isDirectConnectableAddr(addr), "isDirectConnectableAddr 应该过滤 Relay 地址: %s", addr)
		})
	}

	// 非 Relay 地址应该通过两个函数
	directAddrs := []string{
		"/ip4/60.177.185.34/udp/54583/quic-v1",
		"/ip4/192.168.1.1/tcp/4001",
		"/ip4/8.8.8.8/tcp/4001/p2p/QmPeer",
	}

	for _, addr := range directAddrs {
		t.Run("direct:"+addr[:15]+"...", func(t *testing.T) {
			assert.True(t, isConnectableAddr(addr), "直连地址应该通过 isConnectableAddr: %s", addr)
			assert.True(t, isDirectConnectableAddr(addr), "直连地址应该通过 isDirectConnectableAddr: %s", addr)
		})
	}

	// 不可连接地址应该被两个函数过滤
	unconnectableAddrs := []string{
		"/ip4/0.0.0.0/tcp/4001",
		"/ip4/127.0.0.1/tcp/4001",
		"/ip6/::1/tcp/4001",
	}

	for _, addr := range unconnectableAddrs {
		t.Run("unconnectable:"+addr[:15]+"...", func(t *testing.T) {
			assert.False(t, isConnectableAddr(addr), "不可连接地址应该被 isConnectableAddr 过滤: %s", addr)
			assert.False(t, isDirectConnectableAddr(addr), "不可连接地址应该被 isDirectConnectableAddr 过滤: %s", addr)
		})
	}
}

// TestContainsP2PSuffix 测试 P2P 后缀检测
func TestContainsP2PSuffix(t *testing.T) {
	tests := []struct {
		addr     string
		expected bool
	}{
		{"/ip4/127.0.0.1/tcp/4001", false},
		{"/ip4/127.0.0.1/tcp/4001/p2p/QmPeer", true},
		{"/p2p/QmPeer", true},
		{"", false},
		{"/ip", false}, // 短于 4 字符
	}

	for _, tc := range tests {
		t.Run(tc.addr, func(t *testing.T) {
			result := containsP2PSuffix(tc.addr)
			assert.Equal(t, tc.expected, result)
		})
	}
}

// TestAddrsManager_StartStop 测试启动和停止
func TestAddrsManager_StartStop(t *testing.T) {
	localPeerID := types.PeerID("test-peer-id")
	mockSwarm := mocks.NewMockSwarm(string(localPeerID))
	mgr := newAddrsManager(mockSwarm, localPeerID, nil)

	// 启动
	err := mgr.Start()
	assert.NoError(t, err)

	// 停止
	err = mgr.Stop()
	assert.NoError(t, err)
}

// TestObservedAddrManager_Clear 测试清空观测地址
func TestObservedAddrManager_Clear(t *testing.T) {
	oam := newObservedAddrManager()

	// 添加一些地址
	addr1, _ := types.NewMultiaddr("/ip4/1.1.1.1/tcp/4001")
	addr2, _ := types.NewMultiaddr("/ip4/2.2.2.2/tcp/4001")
	oam.Add(addr1)
	oam.Add(addr2)

	// 验证已添加
	assert.Len(t, oam.TopAddrs(10), 2)

	// 清空
	oam.Clear()

	// 验证已清空
	assert.Len(t, oam.TopAddrs(10), 0)
}

// TestObservedAddrManager_CleanExpired 测试清理过期地址
func TestObservedAddrManager_CleanExpired(t *testing.T) {
	oam := newObservedAddrManager()

	// 添加一个地址
	addr, _ := types.NewMultiaddr("/ip4/1.1.1.1/tcp/4001")
	oam.Add(addr)

	// 验证已添加
	assert.Len(t, oam.TopAddrs(10), 1)

	// 清理（不应该清理任何地址，因为刚添加）
	oam.cleanExpired()
	assert.Len(t, oam.TopAddrs(10), 1)
}

// TestObservedAddrManager_TopAddrs_Empty 测试空观测地址
func TestObservedAddrManager_TopAddrs_Empty(t *testing.T) {
	oam := newObservedAddrManager()

	// 空时应该返回 nil
	result := oam.TopAddrs(5)
	assert.Nil(t, result)
}

// TestObservedAddrManager_TopAddrs_LessThanN 测试地址数少于请求数
func TestObservedAddrManager_TopAddrs_LessThanN(t *testing.T) {
	oam := newObservedAddrManager()

	// 只添加 2 个地址
	addr1, _ := types.NewMultiaddr("/ip4/1.1.1.1/tcp/4001")
	addr2, _ := types.NewMultiaddr("/ip4/2.2.2.2/tcp/4001")
	oam.Add(addr1)
	oam.Add(addr2)

	// 请求 10 个，但只有 2 个
	result := oam.TopAddrs(10)
	assert.Len(t, result, 2)
}

// TestObservedAddrManager_UpdateExisting 测试更新现有地址
func TestObservedAddrManager_UpdateExisting(t *testing.T) {
	oam := newObservedAddrManager()

	// 添加同一地址多次
	addr, _ := types.NewMultiaddr("/ip4/1.1.1.1/tcp/4001")
	oam.Add(addr)
	oam.Add(addr)
	oam.Add(addr)

	// 验证只有一个条目，但观测次数增加
	result := oam.TopAddrs(10)
	assert.Len(t, result, 1)

	// 检查置信度增加
	oam.mu.RLock()
	entry := oam.addrs[addr.String()]
	oam.mu.RUnlock()
	assert.NotNil(t, entry)
	assert.Equal(t, 3, entry.seenCount)
	assert.Greater(t, entry.confidence, 1)
}

// TestCalculateConfidence 测试置信度计算
func TestCalculateConfidence(t *testing.T) {
	now := time.Now()

	// 测试基本分数
	score1 := calculateConfidence(1, now)
	score3 := calculateConfidence(3, now)

	// 观测次数越多，分数越高
	assert.Greater(t, score3, score1)
}

// TestAddrsManager_GetListenAddrs_WithP2PSuffix 测试已有 P2P 后缀的地址
func TestAddrsManager_GetListenAddrs_WithP2PSuffix(t *testing.T) {
	localPeerID := types.PeerID("test-peer-id")
	mockSwarm := mocks.NewMockSwarm(string(localPeerID))
	mgr := newAddrsManager(mockSwarm, localPeerID, nil)

	// 添加已有 /p2p 后缀的地址
	addrWithP2P, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001/p2p/QmOtherPeer")
	mgr.updateListenAddrs([]types.Multiaddr{addrWithP2P})

	// 应该保留原有的 /p2p 后缀，不再添加
	addrs := mgr.Addrs()
	assert.Len(t, addrs, 1)
	assert.Contains(t, addrs[0], "QmOtherPeer")
	assert.NotContains(t, addrs[0], "test-peer-id")
}

