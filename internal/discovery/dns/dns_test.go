package dns

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

var _ pkgif.Discovery = (*Discoverer)(nil) // 确保实现接口

// TestDiscoverer_Creation 测试创建
func TestDiscoverer_Creation(t *testing.T) {
	config := DefaultConfig()
	config.Domains = []string{"example.com"}

	discoverer := NewDiscoverer(config)
	require.NotNil(t, discoverer)
	assert.NotNil(t, discoverer.resolver)
}

// TestDiscoverer_Start 测试启动
func TestDiscoverer_Start(t *testing.T) {
	config := DefaultConfig()
	discoverer := NewDiscoverer(config)

	ctx := context.Background()
	err := discoverer.Start(ctx)
	require.NoError(t, err)
	assert.True(t, discoverer.started.Load())

	err = discoverer.Stop(ctx)
	require.NoError(t, err)
}

// TestDiscoverer_FindPeers 测试发现
func TestDiscoverer_FindPeers(t *testing.T) {
	config := DefaultConfig()
	config.Domains = []string{"nonexistent.example.invalid"}
	config.Timeout = 500 * time.Millisecond
	discoverer := NewDiscoverer(config)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := discoverer.Start(ctx)
	require.NoError(t, err)
	defer discoverer.Stop(ctx)

	// FindPeers 会尝试 DNS 查询（但域名不存在，会快速返回空）
	ch, err := discoverer.FindPeers(ctx, "dns")
	require.NoError(t, err)
	require.NotNil(t, ch)

	// 消费通道（应该为空或快速完成）
	count := 0
	done := make(chan struct{})
	go func() {
		for range ch {
			count++
			if count > 10 {
				break
			}
		}
		close(done)
	}()

	select {
	case <-done:
		// 正常完成
	case <-time.After(2 * time.Second):
		t.Fatal("FindPeers 超时")
	}
}

// TestDiscoverer_Advertise 测试广播
func TestDiscoverer_Advertise(t *testing.T) {
	config := DefaultConfig()
	discoverer := NewDiscoverer(config)

	ctx := context.Background()
	err := discoverer.Start(ctx)
	require.NoError(t, err)
	defer discoverer.Stop(ctx)

	// DNS 不支持动态广播，返回 0
	ttl, err := discoverer.Advertise(ctx, "test-ns")
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), ttl)
}

// TestDiscoverer_AddRemoveDomain 测试域名管理
func TestDiscoverer_AddRemoveDomain(t *testing.T) {
	config := DefaultConfig()
	discoverer := NewDiscoverer(config)

	// 添加域名
	err := discoverer.AddDomain("example.com")
	require.NoError(t, err)
	assert.Equal(t, 1, len(discoverer.Domains()))

	// 添加无效域名
	err = discoverer.AddDomain("")
	assert.Error(t, err)

	// 移除域名
	discoverer.RemoveDomain("example.com")
	assert.Equal(t, 0, len(discoverer.Domains()))
}

// TestDiscoverer_Lifecycle 测试生命周期
func TestDiscoverer_Lifecycle(t *testing.T) {
	config := DefaultConfig()
	discoverer := NewDiscoverer(config)

	ctx := context.Background()

	// 启动
	err := discoverer.Start(ctx)
	require.NoError(t, err)
	assert.True(t, discoverer.started.Load())

	// 重复启动应该失败
	err = discoverer.Start(ctx)
	assert.Error(t, err)

	// 停止
	err = discoverer.Stop(ctx)
	require.NoError(t, err)
	assert.False(t, discoverer.started.Load())
}

// TestResolver_Cache 测试缓存
func TestResolver_Cache(t *testing.T) {
	config := ResolverConfig{
		Timeout:  5 * time.Second,
		MaxDepth: 3,
		CacheTTL: 1 * time.Second,
	}
	resolver := NewResolver(config)

	domain := "_dnsaddr.test.com"
	peers := []types.PeerInfo{
		{ID: "peer-1", Addrs: []types.Multiaddr{}},
	}

	// 设置缓存
	resolver.setCache(domain, peers)

	// 验证缓存命中
	cached, ok := resolver.getFromCache(domain)
	assert.True(t, ok)
	assert.Equal(t, 1, len(cached))

	// 等待过期
	time.Sleep(1200 * time.Millisecond)

	// 验证过期
	_, ok = resolver.getFromCache(domain)
	assert.False(t, ok)
}

// TestValidateDomain 测试域名验证
func TestValidateDomain(t *testing.T) {
	validDomains := []string{
		"example.com",
		"bootstrap.dep2p.io",
		"us-east.example.com",
	}

	for _, domain := range validDomains {
		err := ValidateDomain(domain)
		assert.NoError(t, err, "domain: %s", domain)
	}

	invalidDomains := []string{
		"",
		"-invalid.com",
		"invalid-.com",
	}

	for _, domain := range invalidDomains {
		err := ValidateDomain(domain)
		assert.Error(t, err, "domain: %s", domain)
	}
}

// TestConfig_Validate 测试配置验证
func TestConfig_Validate(t *testing.T) {
	config := DefaultConfig()
	err := config.Validate()
	require.NoError(t, err)

	// 无效配置
	config.Timeout = -1
	err = config.Validate()
	assert.Error(t, err)

	config = DefaultConfig()
	config.MaxDepth = 20
	err = config.Validate()
	assert.Error(t, err)
}

// TestDiscoverer_Concurrent 测试并发安全
func TestDiscoverer_Concurrent(t *testing.T) {
	config := DefaultConfig()
	config.Domains = []string{"example.com"}
	discoverer := NewDiscoverer(config)

	ctx := context.Background()
	err := discoverer.Start(ctx)
	require.NoError(t, err)
	defer discoverer.Stop(ctx)

	// 并发添加/移除域名
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			_ = discoverer.AddDomain("test.com")
			discoverer.RemoveDomain("test.com")
			_ = discoverer.Domains()
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}
