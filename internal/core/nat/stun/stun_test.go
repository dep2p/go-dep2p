package stun

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
//                              Client 测试
// ============================================================================

func TestNewClient(t *testing.T) {
	t.Run("使用默认服务器", func(t *testing.T) {
		client := NewClient(nil)
		require.NotNil(t, client)
		assert.NotEmpty(t, client.servers)
	})

	t.Run("使用自定义服务器", func(t *testing.T) {
		servers := []string{"stun1.test.com:3478", "stun2.test.com:3478"}
		client := NewClient(servers)
		require.NotNil(t, client)
		assert.Equal(t, servers, client.servers)
	})

	t.Run("使用默认配置", func(t *testing.T) {
		client := NewClient(nil)
		require.NotNil(t, client)
	})
}

func TestDefaultServers(t *testing.T) {
	servers := DefaultServers()
	assert.NotEmpty(t, servers)
	// 验证服务器格式
	for _, server := range servers {
		_, _, err := net.SplitHostPort(server)
		assert.NoError(t, err, "服务器地址格式无效: %s", server)
	}
}

// ============================================================================
//                              常量测试
// ============================================================================

func TestConstants(t *testing.T) {
	// 验证常量值正确
	assert.Equal(t, uint16(0x0001), bindingRequest)
	assert.Equal(t, uint16(0x0101), bindingResponse)
	assert.Equal(t, uint32(0x2112A442), magicCookie)
	assert.Equal(t, 12, transactionIDLen)
}

// ============================================================================
//                              错误定义测试
// ============================================================================

func TestErrors(t *testing.T) {
	assert.NotNil(t, ErrNoResponse)
	assert.NotNil(t, ErrInvalidResponse)
	assert.NotNil(t, ErrAllServersFailed)
	assert.NotNil(t, ErrNoOtherAddress)
}

// ============================================================================
//                              NAT 类型检测测试
// ============================================================================

func TestClient_GetNATType_Cached(t *testing.T) {
	client := NewClient(nil)
	client.cacheDuration = time.Hour

	// 设置缓存
	client.cacheMu.Lock()
	client.cachedNATType = types.NATTypeFull
	client.cachedTime = time.Now()
	client.cacheMu.Unlock()

	// 应该返回缓存值
	natType, err := client.GetNATType(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, types.NATTypeFull, natType)
}

func TestClient_GetNATType_CacheExpired(t *testing.T) {
	client := NewClient([]string{"invalid.server:3478"})
	client.cacheDuration = time.Millisecond
	client.timeout = 100 * time.Millisecond

	// 设置过期缓存
	client.cacheMu.Lock()
	client.cachedNATType = types.NATTypeFull
	client.cachedTime = time.Now().Add(-time.Hour)
	client.cacheMu.Unlock()

	// 缓存过期，应该尝试重新检测（会失败）
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := client.GetNATType(ctx)
	assert.Error(t, err) // 无效服务器应该失败
}

// ============================================================================
//                              外部地址发现测试
// ============================================================================

func TestClient_GetMappedAddress_Cached(t *testing.T) {
	client := NewClient(nil)
	client.cacheDuration = time.Hour

	// 设置缓存
	client.cacheMu.Lock()
	client.cachedAddr = &stunAddress{ip: net.ParseIP("1.2.3.4"), port: 5678}
	client.cachedTime = time.Now()
	client.cacheMu.Unlock()

	// 应该返回缓存值
	addr, err := client.GetMappedAddress(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, addr)
	assert.Contains(t, addr.String(), "1.2.3.4")
}

func TestClient_GetMappedAddress_InvalidServer(t *testing.T) {
	client := NewClient([]string{"invalid.server.that.does.not.exist:3478"})
	client.timeout = 100 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := client.GetMappedAddress(ctx)
	assert.Error(t, err)
}

// ============================================================================
//                              错误处理测试
// ============================================================================

func TestClient_ContextCancellation(t *testing.T) {
	client := NewClient([]string{"stun.l.google.com:19302"})
	client.timeout = 10 * time.Second

	// 立即取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.GetMappedAddress(ctx)
	assert.Error(t, err)
}

func TestClient_EmptyServers(t *testing.T) {
	client := &Client{
		servers: []string{},
		timeout: defaultTimeout,
	}

	_, err := client.GetMappedAddress(context.Background())
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrAllServersFailed)
}

// ============================================================================
//                              SetTimeout/SetCacheDuration 测试
// ============================================================================

func TestClient_SetTimeout(t *testing.T) {
	client := NewClient(nil)
	client.SetTimeout(10 * time.Second)
	assert.Equal(t, 10*time.Second, client.timeout)
}

func TestClient_SetCacheDuration(t *testing.T) {
	client := NewClient(nil)
	client.SetCacheDuration(10 * time.Minute)
	assert.Equal(t, 10*time.Minute, client.cacheDuration)
}

func TestClient_Close(t *testing.T) {
	client := NewClient(nil)
	err := client.Close()
	assert.NoError(t, err)
}

// ============================================================================
//                              stunAddress 测试
// ============================================================================

func TestStunAddress(t *testing.T) {
	t.Run("IPv4 地址", func(t *testing.T) {
		addr := &stunAddress{ip: net.ParseIP("1.2.3.4"), port: 5678}

		assert.NotEmpty(t, addr.Network()) // Network() 返回协议类型
		assert.Contains(t, addr.String(), "1.2.3.4")
		assert.Contains(t, addr.String(), "5678")
		assert.NotEmpty(t, addr.Bytes())
	})

	t.Run("IPv6 地址", func(t *testing.T) {
		addr := &stunAddress{ip: net.ParseIP("2001:db8::1"), port: 8080}

		assert.NotEmpty(t, addr.Network()) // Network() 返回协议类型
		assert.Contains(t, addr.String(), "2001:db8::1")
	})

	t.Run("公网地址", func(t *testing.T) {
		addr := &stunAddress{ip: net.ParseIP("8.8.8.8"), port: 1234}

		assert.True(t, addr.IsPublic())
		assert.False(t, addr.IsPrivate())
		assert.False(t, addr.IsLoopback())
	})

	t.Run("私有地址", func(t *testing.T) {
		addr := &stunAddress{ip: net.ParseIP("192.168.1.1"), port: 1234}

		assert.False(t, addr.IsPublic())
		assert.True(t, addr.IsPrivate())
		assert.False(t, addr.IsLoopback())
	})

	t.Run("回环地址", func(t *testing.T) {
		addr := &stunAddress{ip: net.ParseIP("127.0.0.1"), port: 1234}

		assert.False(t, addr.IsPublic())
		assert.False(t, addr.IsPrivate())
		assert.True(t, addr.IsLoopback())
	})

	t.Run("Equal 比较", func(t *testing.T) {
		addr1 := &stunAddress{ip: net.ParseIP("1.2.3.4"), port: 5678}
		addr2 := &stunAddress{ip: net.ParseIP("1.2.3.4"), port: 5678}
		addr3 := &stunAddress{ip: net.ParseIP("5.6.7.8"), port: 5678}

		assert.True(t, addr1.Equal(addr2))
		assert.False(t, addr1.Equal(addr3))
		assert.False(t, addr1.Equal(nil))
	})
}

// ============================================================================
//                              集成测试（需要网络）
// ============================================================================

func TestClient_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试")
	}

	client := NewClient(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Run("获取映射地址", func(t *testing.T) {
		addr, err := client.GetMappedAddress(ctx)
		if err != nil {
			t.Skipf("无法连接 STUN 服务器: %v", err)
		}
		assert.NotNil(t, addr)
		t.Logf("映射地址: %s", addr.String())
	})

	t.Run("获取 NAT 类型", func(t *testing.T) {
		natType, err := client.GetNATType(ctx)
		if err != nil {
			t.Skipf("无法检测 NAT 类型: %v", err)
		}
		t.Logf("NAT 类型: %s", natType.String())
	})
}

// ============================================================================
//                              并发安全测试
// ============================================================================

func TestClient_SetTimeout_Concurrent(t *testing.T) {
	client := NewClient(nil)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			client.SetTimeout(time.Duration(i) * time.Second)
			_ = client.getTimeout()
		}(i)
	}
	wg.Wait()
	// 测试通过表示没有竞态条件
}

func TestClient_SetCacheDuration_Concurrent(t *testing.T) {
	client := NewClient(nil)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			client.SetCacheDuration(time.Duration(i) * time.Minute)
		}(i)
	}
	wg.Wait()
	// 测试通过表示没有竞态条件
}

func TestClient_Close_Idempotent(t *testing.T) {
	client := NewClient(nil)

	// 多次调用 Close 应该不会 panic
	for i := 0; i < 10; i++ {
		err := client.Close()
		assert.NoError(t, err)
	}
}

func TestClient_Close_Concurrent(t *testing.T) {
	client := NewClient(nil)

	// 设置一些缓存数据
	client.cacheMu.Lock()
	client.cachedAddr = &stunAddress{ip: net.ParseIP("1.2.3.4"), port: 5678}
	client.cacheMu.Unlock()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = client.Close()
		}()
	}
	wg.Wait()
	// 测试通过表示没有竞态条件
}

// ============================================================================
//                              parseMappedAddress 边界测试
// ============================================================================

func TestClient_ParseMappedAddress_EdgeCases(t *testing.T) {
	client := NewClient(nil)

	t.Run("数据过短", func(t *testing.T) {
		_, err := client.parseMappedAddress([]byte{0x00, 0x01, 0x00})
		assert.Error(t, err)
	})

	t.Run("未知地址族", func(t *testing.T) {
		// 4 字节头部 + 无效地址族
		data := []byte{0x00, 0x03, 0x00, 0x50, 0x01, 0x02, 0x03, 0x04}
		_, err := client.parseMappedAddress(data)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown address family")
	})

	t.Run("IPv4 数据不足", func(t *testing.T) {
		// 地址族 0x01 (IPv4) 但数据不够
		data := []byte{0x00, 0x01, 0x00, 0x50, 0x01, 0x02, 0x03}
		_, err := client.parseMappedAddress(data)
		assert.Error(t, err)
	})

	t.Run("IPv6 数据不足", func(t *testing.T) {
		// 地址族 0x02 (IPv6) 但数据不够
		data := []byte{0x00, 0x02, 0x00, 0x50, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}
		_, err := client.parseMappedAddress(data)
		assert.Error(t, err)
	})

	t.Run("有效 IPv4 地址", func(t *testing.T) {
		// 地址族 0x01 (IPv4), 端口 80 (0x0050), IP 1.2.3.4
		data := []byte{0x00, 0x01, 0x00, 0x50, 0x01, 0x02, 0x03, 0x04}
		addr, err := client.parseMappedAddress(data)
		assert.NoError(t, err)
		assert.NotNil(t, addr)
		assert.Contains(t, addr.String(), "1.2.3.4")
		assert.Contains(t, addr.String(), "80")
	})

	t.Run("有效 IPv6 地址", func(t *testing.T) {
		// 地址族 0x02 (IPv6), 端口 8080 (0x1F90), IP ::1
		data := []byte{0x00, 0x02, 0x1F, 0x90,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}
		addr, err := client.parseMappedAddress(data)
		assert.NoError(t, err)
		assert.NotNil(t, addr)
	})

	t.Run("IP 数据独立性", func(t *testing.T) {
		// 验证解析后的 IP 不共享原始 slice
		data := []byte{0x00, 0x01, 0x00, 0x50, 0x01, 0x02, 0x03, 0x04}
		addr, err := client.parseMappedAddress(data)
		require.NoError(t, err)

		// 修改原始数据
		data[4] = 0xFF
		data[5] = 0xFF

		// 地址应该不变
		assert.Contains(t, addr.String(), "1.2.3.4")
	})
}

// ============================================================================
//                              stunAddress 扩展测试
// ============================================================================

func TestStunAddress_ToUDPAddr(t *testing.T) {
	addr := &stunAddress{ip: net.ParseIP("192.168.1.1"), port: 8080}

	udpAddr, err := addr.ToUDPAddr()
	assert.NoError(t, err)
	assert.NotNil(t, udpAddr)
	assert.Equal(t, 8080, udpAddr.Port)
}

func TestStunAddress_Multiaddr(t *testing.T) {
	t.Run("IPv4", func(t *testing.T) {
		addr := &stunAddress{ip: net.ParseIP("192.168.1.1"), port: 8080}
		ma := addr.Multiaddr()
		assert.Contains(t, ma, "/ip4/192.168.1.1")
		assert.Contains(t, ma, "/udp/8080")
	})

	t.Run("IPv6", func(t *testing.T) {
		addr := &stunAddress{ip: net.ParseIP("::1"), port: 8080}
		ma := addr.Multiaddr()
		assert.Contains(t, ma, "/ip6/")
		assert.Contains(t, ma, "/udp/8080")
	})
}

func TestStunAddress_IPAndPort(t *testing.T) {
	addr := &stunAddress{ip: net.ParseIP("8.8.8.8"), port: 53}

	assert.Equal(t, net.ParseIP("8.8.8.8").To4(), addr.IP().To4())
	assert.Equal(t, 53, addr.Port())
}
