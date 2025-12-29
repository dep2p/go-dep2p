package http

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                              Discoverer 测试
// ============================================================================

func TestNewDiscoverer(t *testing.T) {
	t.Run("使用默认服务", func(t *testing.T) {
		d := NewDiscoverer(nil)
		require.NotNil(t, d)
		assert.NotEmpty(t, d.services)
		assert.Equal(t, DefaultServices, d.services)
	})

	t.Run("使用自定义服务", func(t *testing.T) {
		services := []string{"https://test.com/ip"}
		d := NewDiscoverer(services)
		require.NotNil(t, d)
		assert.Equal(t, services, d.services)
	})

	t.Run("使用默认服务", func(t *testing.T) {
		d := NewDiscoverer(nil)
		require.NotNil(t, d)
	})
}

func TestDefaultServices(t *testing.T) {
	assert.NotEmpty(t, DefaultServices)
	// 验证所有服务都是 HTTPS
	for _, service := range DefaultServices {
		assert.Contains(t, service, "https://")
	}
}

func TestDiscoverer_Name(t *testing.T) {
	d := NewDiscoverer(nil)
	assert.Equal(t, "http", d.Name())
}

func TestDiscoverer_Priority(t *testing.T) {
	d := NewDiscoverer(nil)
	// HTTP 优先级应该比 STUN 低
	assert.Greater(t, d.Priority(), 0)
}

// ============================================================================
//                              缓存测试
// ============================================================================

func TestDiscoverer_Cache(t *testing.T) {
	d := NewDiscoverer(nil)
	d.cacheDuration = time.Hour

	// 设置缓存
	d.cacheMu.Lock()
	d.cachedIP = "1.2.3.4"
	d.cachedTime = time.Now()
	d.cacheMu.Unlock()

	// 应该返回缓存值
	ctx := context.Background()
	addr, err := d.Discover(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, addr)
	assert.Equal(t, "1.2.3.4", addr.String())
}

func TestDiscoverer_CacheExpired(t *testing.T) {
	// 创建测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("5.6.7.8"))
	}))
	defer server.Close()

	d := NewDiscoverer([]string{server.URL})
	d.cacheDuration = time.Millisecond

	// 设置过期缓存
	d.cacheMu.Lock()
	d.cachedIP = "1.2.3.4"
	d.cachedTime = time.Now().Add(-time.Hour)
	d.cacheMu.Unlock()

	// 等待缓存过期
	time.Sleep(10 * time.Millisecond)

	// 应该查询新值
	ctx := context.Background()
	addr, err := d.Discover(ctx)
	assert.NoError(t, err)
	assert.NotNil(t, addr)
	assert.Equal(t, "5.6.7.8", addr.String())
}

func TestDiscoverer_SetCacheDuration(t *testing.T) {
	d := NewDiscoverer(nil)
	d.SetCacheDuration(10 * time.Minute)
	assert.Equal(t, 10*time.Minute, d.cacheDuration)
}

func TestDiscoverer_ClearCache(t *testing.T) {
	d := NewDiscoverer(nil)

	// 设置缓存
	d.cacheMu.Lock()
	d.cachedIP = "1.2.3.4"
	d.cachedTime = time.Now()
	d.cacheMu.Unlock()

	// 清除缓存
	d.ClearCache()

	// 验证缓存已清除
	d.cacheMu.RLock()
	assert.Empty(t, d.cachedIP)
	d.cacheMu.RUnlock()
}

// ============================================================================
//                              发现测试（使用 Mock 服务器）
// ============================================================================

func TestDiscoverer_Discover_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("203.0.113.1\n"))
	}))
	defer server.Close()

	d := NewDiscoverer([]string{server.URL})

	ctx := context.Background()
	addr, err := d.Discover(ctx)
	assert.NoError(t, err)
	require.NotNil(t, addr)
	assert.Equal(t, "203.0.113.1", addr.String())
}

func TestDiscoverer_Discover_IPv6(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("2001:db8::1"))
	}))
	defer server.Close()

	d := NewDiscoverer([]string{server.URL})

	ctx := context.Background()
	addr, err := d.Discover(ctx)
	assert.NoError(t, err)
	require.NotNil(t, addr)
	assert.Contains(t, addr.String(), "2001:db8::1")
}

func TestDiscoverer_Discover_Fallback(t *testing.T) {
	// 第一个服务器失败
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server1.Close()

	// 第二个服务器成功
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("198.51.100.1"))
	}))
	defer server2.Close()

		d := NewDiscoverer([]string{server1.URL, server2.URL})

	ctx := context.Background()
	addr, err := d.Discover(ctx)
	assert.NoError(t, err)
	require.NotNil(t, addr)
	assert.Equal(t, "198.51.100.1", addr.String())
}

func TestDiscoverer_Discover_AllFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	d := NewDiscoverer([]string{server.URL})

	ctx := context.Background()
	_, err := d.Discover(ctx)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrAllServicesFailed)
}

func TestDiscoverer_Discover_InvalidIP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-an-ip"))
	}))
	defer server.Close()

	d := NewDiscoverer([]string{server.URL})

	ctx := context.Background()
	_, err := d.Discover(ctx)
	assert.Error(t, err)
}

func TestDiscoverer_Discover_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(""))
	}))
	defer server.Close()

	d := NewDiscoverer([]string{server.URL})

	ctx := context.Background()
	_, err := d.Discover(ctx)
	assert.Error(t, err)
}

func TestDiscoverer_Discover_ContextCancelled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(time.Second)
		_, _ = w.Write([]byte("1.2.3.4"))
	}))
	defer server.Close()

	d := NewDiscoverer([]string{server.URL})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := d.Discover(ctx)
	assert.Error(t, err)
}

// ============================================================================
//                              Close 测试
// ============================================================================

func TestDiscoverer_Close(t *testing.T) {
	d := NewDiscoverer(nil)
	err := d.Close()
	assert.NoError(t, err)
}

// ============================================================================
//                              并发测试
// ============================================================================

func TestDiscoverer_Concurrency(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("1.2.3.4"))
	}))
	defer server.Close()

	d := NewDiscoverer([]string{server.URL})
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()
			_, _ = d.Discover(ctx)
		}()
	}

	wg.Wait()
}

func TestDiscoverer_SetCacheDuration_Concurrent(t *testing.T) {
	d := NewDiscoverer(nil)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			d.SetCacheDuration(time.Duration(i) * time.Minute)
			_ = d.getCacheDuration()
		}(i)
	}
	wg.Wait()
	// 测试通过表示没有竞态条件
}

func TestDiscoverer_Close_Concurrent(t *testing.T) {
	d := NewDiscoverer(nil)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = d.Close()
		}()
	}
	wg.Wait()
	// 测试通过表示没有竞态条件
}

func TestDiscoverer_ClearCache_Concurrent(t *testing.T) {
	d := NewDiscoverer(nil)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			d.ClearCache()
		}()
		go func() {
			defer wg.Done()
			d.cacheMu.Lock()
			d.cachedIP = "1.2.3.4"
			d.cachedTime = time.Now()
			d.cacheMu.Unlock()
		}()
	}
	wg.Wait()
	// 测试通过表示没有竞态条件
}

// ============================================================================
//                              ipAddr 测试
// ============================================================================

func TestIPAddr(t *testing.T) {
	t.Run("IPv4 地址", func(t *testing.T) {
		addr := newIPAddr(net.ParseIP("1.2.3.4"), 0)

		assert.Equal(t, "ip4", addr.Network())
		assert.Equal(t, "1.2.3.4", addr.String())
		assert.True(t, addr.IsPublic())
		assert.False(t, addr.IsPrivate())
		assert.False(t, addr.IsLoopback())
	})

	t.Run("IPv4 带端口", func(t *testing.T) {
		addr := newIPAddr(net.ParseIP("1.2.3.4"), 8080)

		assert.Equal(t, "1.2.3.4:8080", addr.String())
	})

	t.Run("IPv6 地址", func(t *testing.T) {
		addr := newIPAddr(net.ParseIP("2001:db8::1"), 0)

		assert.Equal(t, "ip6", addr.Network())
		assert.Equal(t, "2001:db8::1", addr.String())
	})

	t.Run("IPv6 带端口", func(t *testing.T) {
		addr := newIPAddr(net.ParseIP("2001:db8::1"), 8080)

		assert.Equal(t, "[2001:db8::1]:8080", addr.String())
	})

	t.Run("私有地址", func(t *testing.T) {
		addr := newIPAddr(net.ParseIP("192.168.1.1"), 0)

		assert.False(t, addr.IsPublic())
		assert.True(t, addr.IsPrivate())
		assert.False(t, addr.IsLoopback())
	})

	t.Run("回环地址", func(t *testing.T) {
		addr := newIPAddr(net.ParseIP("127.0.0.1"), 0)

		assert.False(t, addr.IsPublic())
		assert.False(t, addr.IsPrivate())
		assert.True(t, addr.IsLoopback())
	})

	t.Run("Equal 比较", func(t *testing.T) {
		addr1 := newIPAddr(net.ParseIP("1.2.3.4"), 0)
		addr2 := newIPAddr(net.ParseIP("1.2.3.4"), 0)
		addr3 := newIPAddr(net.ParseIP("5.6.7.8"), 0)

		assert.True(t, addr1.Equal(addr2))
		assert.False(t, addr1.Equal(addr3))
		assert.False(t, addr1.Equal(nil))
	})

	t.Run("Bytes", func(t *testing.T) {
		addr := newIPAddr(net.ParseIP("1.2.3.4"), 0)
		assert.NotEmpty(t, addr.Bytes())
	})
}

// ============================================================================
//                              集成测试（需要网络）
// ============================================================================

func TestDiscoverer_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试")
	}

	d := NewDiscoverer(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	addr, err := d.Discover(ctx)
	if err != nil {
		t.Skipf("无法通过 HTTP 发现外部 IP: %v", err)
	}

	require.NotNil(t, addr)
	t.Logf("外部 IP: %s", addr.String())

	// 验证是公网 IP
	assert.True(t, addr.IsPublic())
}

