package dht

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                 Config 配置选项测试 - 覆盖 0% 函数
// ============================================================================

// TestConfig_WithBucketSize 测试设置桶大小
func TestConfig_WithBucketSize(t *testing.T) {
	config := DefaultConfig()
	WithBucketSize(30)(config)
	assert.Equal(t, 30, config.BucketSize)

	t.Log("✅ WithBucketSize 测试通过")
}

// TestConfig_WithAlpha 测试设置并发参数
func TestConfig_WithAlpha(t *testing.T) {
	config := DefaultConfig()
	WithAlpha(5)(config)
	assert.Equal(t, 5, config.Alpha)

	t.Log("✅ WithAlpha 测试通过")
}

// TestConfig_WithQueryTimeout 测试设置查询超时
func TestConfig_WithQueryTimeout(t *testing.T) {
	config := DefaultConfig()
	WithQueryTimeout(60 * time.Second)(config)
	assert.Equal(t, 60*time.Second, config.QueryTimeout)

	t.Log("✅ WithQueryTimeout 测试通过")
}

// TestConfig_WithRefreshInterval 测试设置刷新间隔
func TestConfig_WithRefreshInterval(t *testing.T) {
	config := DefaultConfig()
	WithRefreshInterval(2 * time.Hour)(config)
	assert.Equal(t, 2*time.Hour, config.RefreshInterval)

	t.Log("✅ WithRefreshInterval 测试通过")
}

// TestConfig_WithValueStore 测试设置值存储
func TestConfig_WithValueStore(t *testing.T) {
	config := DefaultConfig()
	WithValueStore(false)(config)
	assert.False(t, config.EnableValueStore)

	t.Log("✅ WithValueStore 测试通过")
}

// TestConfig_WithDataDir 测试设置数据目录
func TestConfig_WithDataDir(t *testing.T) {
	config := DefaultConfig()
	WithDataDir("/tmp/dht-test")(config)
	assert.Equal(t, "/tmp/dht-test", config.DataDir)

	t.Log("✅ WithDataDir 测试通过")
}

// TestConfig_WithAllowPrivateAddrs 测试设置私网地址
func TestConfig_WithAllowPrivateAddrs(t *testing.T) {
	config := DefaultConfig()
	WithAllowPrivateAddrs(false)(config)
	assert.False(t, config.AllowPrivateAddrs)

	t.Log("✅ WithAllowPrivateAddrs 测试通过")
}

// ============================================================================
//                 DHTError 错误测试 - 覆盖 0% 函数
// ============================================================================

// TestDHTError_Error 测试错误消息
func TestDHTError_Error(t *testing.T) {
	// 带消息的错误
	err := NewDHTError("FindPeer", ErrPeerNotFound, "peer xyz not in routing table")
	expected := "dht FindPeer: peer xyz not in routing table: dht: peer not found"
	assert.Equal(t, expected, err.Error())

	// 不带消息的错误
	err2 := NewDHTError("Store", ErrKeyNotFound, "")
	expected2 := "dht Store: dht: key not found"
	assert.Equal(t, expected2, err2.Error())

	t.Log("✅ DHTError.Error 测试通过")
}

// TestDHTError_Unwrap 测试错误解包
func TestDHTError_Unwrap(t *testing.T) {
	err := NewDHTError("Get", ErrKeyNotFound, "key abc")
	
	unwrapped := err.Unwrap()
	assert.Equal(t, ErrKeyNotFound, unwrapped)
	
	// 使用 errors.Is 测试
	assert.True(t, errors.Is(err, ErrKeyNotFound))

	t.Log("✅ DHTError.Unwrap 测试通过")
}

// TestNewDHTError 测试创建错误
func TestNewDHTError(t *testing.T) {
	err := NewDHTError("Put", ErrInvalidValue, "value too large")
	
	assert.Equal(t, "Put", err.Op)
	assert.Equal(t, ErrInvalidValue, err.Err)
	assert.Equal(t, "value too large", err.Message)

	t.Log("✅ NewDHTError 测试通过")
}

// ============================================================================
//                 ProviderStore 测试 - 覆盖 0% 函数
// ============================================================================

// TestProviderStore_CleanupExpired 测试清理过期 Provider
func TestProviderStore_CleanupExpired(t *testing.T) {
	ps := NewProviderStore()

	// 添加一个过期的 Provider
	ps.AddProvider("key1", "peer1", []string{"/ip4/127.0.0.1/tcp/4001"}, -1*time.Hour) // 已过期
	// 添加一个未过期的 Provider
	ps.AddProvider("key2", "peer2", []string{"/ip4/127.0.0.1/tcp/4002"}, 1*time.Hour)

	// 清理过期
	count := ps.CleanupExpired()
	assert.Equal(t, 1, count)

	// 验证只剩未过期的
	assert.Equal(t, 1, ps.Size())
	providers := ps.GetProviders("key2")
	assert.Len(t, providers, 1)

	t.Log("✅ ProviderStore CleanupExpired 测试通过")
}

// TestProviderStore_Size 测试 Provider 数量
func TestProviderStore_Size(t *testing.T) {
	ps := NewProviderStore()

	assert.Equal(t, 0, ps.Size())

	ps.AddProvider("key1", "peer1", []string{"/ip4/127.0.0.1/tcp/4001"}, 1*time.Hour)
	assert.Equal(t, 1, ps.Size())

	ps.AddProvider("key1", "peer2", []string{"/ip4/127.0.0.1/tcp/4002"}, 1*time.Hour)
	assert.Equal(t, 2, ps.Size())

	ps.AddProvider("key2", "peer3", []string{"/ip4/127.0.0.1/tcp/4003"}, 1*time.Hour)
	assert.Equal(t, 3, ps.Size())

	t.Log("✅ ProviderStore Size 测试通过")
}

// TestProviderStore_Clear 测试清空
func TestProviderStore_Clear(t *testing.T) {
	ps := NewProviderStore()

	ps.AddProvider("key1", "peer1", []string{"/ip4/127.0.0.1/tcp/4001"}, 1*time.Hour)
	ps.AddProvider("key2", "peer2", []string{"/ip4/127.0.0.1/tcp/4002"}, 1*time.Hour)
	assert.Equal(t, 2, ps.Size())

	ps.Clear()
	assert.Equal(t, 0, ps.Size())

	t.Log("✅ ProviderStore Clear 测试通过")
}

// ============================================================================
//                 ValueStore 测试 - 覆盖 0% 函数
// ============================================================================

// TestValueStore_Delete 测试删除值
func TestValueStore_Delete(t *testing.T) {
	vs := NewValueStore()

	vs.Put("key1", []byte("value1"), 1*time.Hour)
	
	// 验证存在
	val, ok := vs.Get("key1")
	require.True(t, ok)
	assert.Equal(t, []byte("value1"), val)

	// 删除
	vs.Delete("key1")

	// 验证已删除
	_, ok = vs.Get("key1")
	assert.False(t, ok)

	t.Log("✅ ValueStore Delete 测试通过")
}

// TestValueStore_Size 测试值数量
func TestValueStore_Size(t *testing.T) {
	vs := NewValueStore()

	assert.Equal(t, 0, vs.Size())

	vs.Put("key1", []byte("value1"), 1*time.Hour)
	assert.Equal(t, 1, vs.Size())

	vs.Put("key2", []byte("value2"), 1*time.Hour)
	assert.Equal(t, 2, vs.Size())

	t.Log("✅ ValueStore Size 测试通过")
}

// TestValueStore_CleanupExpired 测试清理过期值
func TestValueStore_CleanupExpired(t *testing.T) {
	vs := NewValueStore()

	// 添加过期值
	vs.Put("expired", []byte("old"), -1*time.Hour)
	// 添加未过期值
	vs.Put("valid", []byte("new"), 1*time.Hour)

	count := vs.CleanupExpired()
	assert.Equal(t, 1, count)
	assert.Equal(t, 1, vs.Size())

	// 验证只剩未过期的
	_, ok := vs.Get("valid")
	assert.True(t, ok)

	t.Log("✅ ValueStore CleanupExpired 测试通过")
}

// TestValueStore_Clear 测试清空
func TestValueStore_Clear(t *testing.T) {
	vs := NewValueStore()

	vs.Put("key1", []byte("value1"), 1*time.Hour)
	vs.Put("key2", []byte("value2"), 1*time.Hour)
	assert.Equal(t, 2, vs.Size())

	vs.Clear()
	assert.Equal(t, 0, vs.Size())

	t.Log("✅ ValueStore Clear 测试通过")
}

// ============================================================================
//                 RealmKey 测试 - 覆盖 0% 函数
// ============================================================================

// TestHashNodeID_Extended 测试节点 ID 哈希
func TestHashNodeID_Extended(t *testing.T) {
	hash := HashNodeID(types.NodeID("test-peer"))
	assert.NotEmpty(t, hash)
	assert.Len(t, hash, 32) // SHA256 produces 32 bytes

	// 相同输入应产生相同输出
	hash2 := HashNodeID(types.NodeID("test-peer"))
	assert.Equal(t, hash, hash2)

	// 不同输入应产生不同输出
	hash3 := HashNodeID(types.NodeID("another-peer"))
	assert.NotEqual(t, hash, hash3)

	t.Log("✅ HashNodeID 测试通过")
}

// ============================================================================
//                 并发安全测试
// ============================================================================

// TestProviderStore_Concurrent 测试 Provider 并发安全
func TestProviderStore_Concurrent(t *testing.T) {
	ps := NewProviderStore()
	done := make(chan struct{})

	for i := 0; i < 10; i++ {
		go func(idx int) {
			for j := 0; j < 100; j++ {
				ps.AddProvider("key", types.NodeID("peer"+string(rune(idx))), []string{"/ip4/127.0.0.1/tcp/4001"}, 1*time.Hour)
				_ = ps.GetProviders("key")
				_ = ps.Size()
			}
			done <- struct{}{}
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	t.Log("✅ ProviderStore 并发安全测试通过")
}

// TestValueStore_Concurrent 测试 Value 并发安全
func TestValueStore_Concurrent(t *testing.T) {
	vs := NewValueStore()
	done := make(chan struct{})

	for i := 0; i < 10; i++ {
		go func(idx int) {
			for j := 0; j < 100; j++ {
				vs.Put("key", []byte("value"), 1*time.Hour)
				_, _ = vs.Get("key")
				_ = vs.Size()
			}
			done <- struct{}{}
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	t.Log("✅ ValueStore 并发安全测试通过")
}

// ============================================================================
//                 routingTableWrapper 测试 - 覆盖 0% 函数
// ============================================================================

// TestRoutingTableWrapper_NearestPeers 测试获取最近节点
func TestRoutingTableWrapper_NearestPeers(t *testing.T) {
	host := newMockHost("local-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	// 添加一些节点到路由表
	rt := dht.routingTable
	rt.Add(&RoutingNode{
		ID:       types.NodeID("peer1"),
		Addrs:    []string{"/ip4/127.0.0.1/tcp/4001"},
		LastSeen: time.Now(),
	})
	rt.Add(&RoutingNode{
		ID:       types.NodeID("peer2"),
		Addrs:    []string{"/ip4/127.0.0.1/tcp/4002"},
		LastSeen: time.Now(),
	})

	// 通过 wrapper 获取最近节点
	rtw := dht.RoutingTable()
	require.NotNil(t, rtw)

	peers := rtw.NearestPeers("target-id", 5)
	assert.NotEmpty(t, peers)
	assert.LessOrEqual(t, len(peers), 5)
	t.Logf("✅ NearestPeers 返回 %d 个节点", len(peers))
}

// TestRoutingTableWrapper_Update 测试更新路由表节点
func TestRoutingTableWrapper_Update(t *testing.T) {
	host := newMockHost("local-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	// 添加节点
	node := &RoutingNode{
		ID:       types.NodeID("peer-to-update"),
		Addrs:    []string{"/ip4/127.0.0.1/tcp/4001"},
		LastSeen: time.Now().Add(-1 * time.Hour), // 1小时前
	}
	dht.routingTable.Add(node)

	// 获取原始 LastSeen
	original := dht.routingTable.Get(types.NodeID("peer-to-update"))
	require.NotNil(t, original)
	oldLastSeen := original.LastSeen

	// 通过 wrapper 更新
	rtw := dht.RoutingTable()
	err = rtw.Update("peer-to-update")
	require.NoError(t, err)

	// 验证 LastSeen 已更新
	updated := dht.routingTable.Get(types.NodeID("peer-to-update"))
	require.NotNil(t, updated)
	assert.True(t, updated.LastSeen.After(oldLastSeen), "LastSeen 应该被更新")

	t.Log("✅ routingTableWrapper.Update 测试通过")
}

// TestRoutingTableWrapper_Update_NonExistent 测试更新不存在的节点
func TestRoutingTableWrapper_Update_NonExistent(t *testing.T) {
	host := newMockHost("local-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	// 更新不存在的节点应该不报错
	rtw := dht.RoutingTable()
	err = rtw.Update("non-existent-peer")
	require.NoError(t, err)

	t.Log("✅ routingTableWrapper.Update 不存在节点测试通过")
}

// TestRoutingTableWrapper_Remove 测试删除路由表节点
func TestRoutingTableWrapper_Remove(t *testing.T) {
	host := newMockHost("local-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	// 添加节点
	dht.routingTable.Add(&RoutingNode{
		ID:       types.NodeID("peer-to-remove"),
		Addrs:    []string{"/ip4/127.0.0.1/tcp/4001"},
		LastSeen: time.Now(),
	})

	// 验证节点存在
	node := dht.routingTable.Get(types.NodeID("peer-to-remove"))
	require.NotNil(t, node)

	// 通过 wrapper 删除
	rtw := dht.RoutingTable()
	rtw.Remove("peer-to-remove")

	// 验证节点已删除
	node = dht.routingTable.Get(types.NodeID("peer-to-remove"))
	assert.Nil(t, node)

	t.Log("✅ routingTableWrapper.Remove 测试通过")
}

// TestRoutingTableWrapper_Size 测试路由表大小
func TestRoutingTableWrapper_Size(t *testing.T) {
	host := newMockHost("local-peer")
	dht, err := New(host, nil)
	require.NoError(t, err)

	rtw := dht.RoutingTable()
	
	// 初始为空
	assert.Equal(t, 0, rtw.Size())

	// 添加节点
	dht.routingTable.Add(&RoutingNode{
		ID:       types.NodeID("peer1"),
		Addrs:    []string{"/ip4/127.0.0.1/tcp/4001"},
		LastSeen: time.Now(),
	})
	assert.Equal(t, 1, rtw.Size())

	t.Log("✅ routingTableWrapper.Size 测试通过")
}

// ============================================================================
//                 RealmKey 测试 - 覆盖 0% 函数
// ============================================================================

// TestRealmKey 测试 Realm 键生成
func TestRealmKey(t *testing.T) {
	key := RealmKey(types.RealmID("test-realm"), "test-type", []byte("test-payload"))
	assert.NotEmpty(t, key)

	// 相同输入应产生相同输出
	key2 := RealmKey(types.RealmID("test-realm"), "test-type", []byte("test-payload"))
	assert.Equal(t, key, key2)

	// 不同 realm 应产生不同输出
	key3 := RealmKey(types.RealmID("other-realm"), "test-type", []byte("test-payload"))
	assert.NotEqual(t, key, key3)

	// 不同 payload 应产生不同输出
	key4 := RealmKey(types.RealmID("test-realm"), "test-type", []byte("other-payload"))
	assert.NotEqual(t, key, key4)

	t.Log("✅ RealmKey 测试通过")
}

// ============================================================================
//                 Config.Validate 测试 - 补充边界条件
// ============================================================================

// TestConfig_Validate_AllErrors 测试所有验证错误
func TestConfig_Validate_AllErrors(t *testing.T) {
	t.Run("BucketSize zero", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.BucketSize = 0
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "bucket size")
	})

	t.Run("Alpha zero", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Alpha = 0
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "alpha")
	})

	t.Run("QueryTimeout zero", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.QueryTimeout = 0
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "query timeout")
	})

	t.Run("RefreshInterval zero", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.RefreshInterval = 0
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "refresh interval")
	})

	t.Run("MaxRecordAge zero", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.MaxRecordAge = 0
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "max record age")
	})

	t.Run("ProviderTTL zero", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.ProviderTTL = 0
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider TTL")
	})

	t.Run("CleanupInterval zero", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.CleanupInterval = 0
		err := cfg.Validate()
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "cleanup interval")
	})

	t.Log("✅ Config.Validate 所有边界条件测试通过")
}
