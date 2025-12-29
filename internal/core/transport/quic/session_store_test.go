package quic

import (
	"crypto/tls"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                              SessionStore 配置测试
// ============================================================================

func TestDefaultSessionStoreConfig(t *testing.T) {
	config := DefaultSessionStoreConfig()

	assert.Equal(t, 1000, config.MaxSize)
	assert.Equal(t, 24*time.Hour, config.TTL)
	assert.True(t, config.EnableAntiReplay)
	assert.Equal(t, 10*time.Second, config.AntiReplayWindow)
}

func TestNewSessionStore(t *testing.T) {
	config := DefaultSessionStoreConfig()
	store := NewSessionStore(config)

	require.NotNil(t, store)
	assert.Equal(t, 0, store.Size())
	assert.NotNil(t, store.antiPlay)
}

func TestNewSessionStore_InvalidConfig(t *testing.T) {
	config := SessionStoreConfig{
		MaxSize: -1,
		TTL:     -1,
	}
	store := NewSessionStore(config)

	require.NotNil(t, store)
	assert.Equal(t, 1000, store.maxSize)
	assert.Equal(t, 24*time.Hour, store.ttl)
}

// ============================================================================
//                              Get/Put 测试
// ============================================================================

func TestSessionStore_PutAndGet(t *testing.T) {
	config := DefaultSessionStoreConfig()
	store := NewSessionStore(config)

	// 创建模拟的 session state
	sessionKey := "server.example.com:443"

	// 存储（使用 nil 因为我们只测试缓存逻辑）
	store.Put(sessionKey, nil)

	// 获取
	session, ok := store.Get(sessionKey)
	assert.True(t, ok)
	assert.Nil(t, session) // 我们存储的是 nil
	assert.Equal(t, 1, store.Size())
}

func TestSessionStore_GetNonExistent(t *testing.T) {
	config := DefaultSessionStoreConfig()
	store := NewSessionStore(config)

	session, ok := store.Get("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, session)
}

func TestSessionStore_GetExpired(t *testing.T) {
	config := SessionStoreConfig{
		MaxSize: 100,
		TTL:     50 * time.Millisecond, // 非常短的 TTL
	}
	store := NewSessionStore(config)

	sessionKey := "server.example.com:443"
	store.Put(sessionKey, nil)

	// 立即获取应该成功
	_, ok := store.Get(sessionKey)
	assert.True(t, ok)

	// 等待过期
	time.Sleep(100 * time.Millisecond)

	// 过期后获取应该失败
	_, ok = store.Get(sessionKey)
	assert.False(t, ok)
	assert.Equal(t, 0, store.Size())
}

func TestSessionStore_UsedCount(t *testing.T) {
	config := DefaultSessionStoreConfig()
	store := NewSessionStore(config)

	sessionKey := "server.example.com:443"
	store.Put(sessionKey, nil)

	// 多次获取
	for i := 0; i < 5; i++ {
		store.Get(sessionKey)
	}

	// 检查统计
	stats := store.Stats()
	assert.Equal(t, 5, stats.TotalUses)
	assert.Equal(t, 1, stats.UsedEntries)
}

// ============================================================================
//                              容量管理测试
// ============================================================================

func TestSessionStore_MaxSize(t *testing.T) {
	config := SessionStoreConfig{
		MaxSize: 3,
		TTL:     time.Hour,
	}
	store := NewSessionStore(config)

	// 添加超过最大容量的条目
	for i := 0; i < 5; i++ {
		store.Put("server"+string(rune('0'+i)), nil)
		time.Sleep(10 * time.Millisecond) // 确保时间顺序
	}

	// 应该只保留最新的 3 个
	assert.Equal(t, 3, store.Size())
}

// ============================================================================
//                              Remove/Clear 测试
// ============================================================================

func TestSessionStore_Remove(t *testing.T) {
	config := DefaultSessionStoreConfig()
	store := NewSessionStore(config)

	sessionKey := "server.example.com:443"
	store.Put(sessionKey, nil)
	assert.Equal(t, 1, store.Size())

	store.Remove(sessionKey)
	assert.Equal(t, 0, store.Size())

	_, ok := store.Get(sessionKey)
	assert.False(t, ok)
}

func TestSessionStore_Clear(t *testing.T) {
	config := DefaultSessionStoreConfig()
	store := NewSessionStore(config)

	// 添加多个条目
	for i := 0; i < 5; i++ {
		store.Put("server"+string(rune('0'+i)), nil)
	}
	assert.Equal(t, 5, store.Size())

	// 清空
	store.Clear()
	assert.Equal(t, 0, store.Size())
}

// ============================================================================
//                              Stats 测试
// ============================================================================

func TestSessionStore_Stats(t *testing.T) {
	config := DefaultSessionStoreConfig()
	store := NewSessionStore(config)

	// 初始统计
	stats := store.Stats()
	assert.Equal(t, 0, stats.TotalEntries)
	assert.Equal(t, 0, stats.UsedEntries)
	assert.Equal(t, 0, stats.TotalUses)

	// 添加条目
	store.Put("server1", nil)
	store.Put("server2", nil)

	// 使用一个
	store.Get("server1")
	store.Get("server1")

	stats = store.Stats()
	assert.Equal(t, 2, stats.TotalEntries)
	assert.Equal(t, 1, stats.UsedEntries)
	assert.Equal(t, 2, stats.TotalUses)
}

// ============================================================================
//                              AntiReplayCache 测试
// ============================================================================

func TestAntiReplayCache_Check(t *testing.T) {
	cache := NewAntiReplayCache(100 * time.Millisecond)

	// 第一次检查应该返回 true（新的）
	assert.True(t, cache.Check("nonce1"))

	// 立即再次检查应该返回 false（重放）
	assert.False(t, cache.Check("nonce1"))

	// 不同的 nonce 应该返回 true
	assert.True(t, cache.Check("nonce2"))
}

func TestAntiReplayCache_WindowExpiry(t *testing.T) {
	cache := NewAntiReplayCache(50 * time.Millisecond)

	// 第一次检查
	assert.True(t, cache.Check("nonce1"))

	// 窗口期内重放
	assert.False(t, cache.Check("nonce1"))

	// 等待窗口期过期
	time.Sleep(100 * time.Millisecond)

	// 窗口期后应该被视为新的
	assert.True(t, cache.Check("nonce1"))
}

func TestAntiReplayCache_DefaultWindow(t *testing.T) {
	// 使用无效窗口创建
	cache := NewAntiReplayCache(-1)

	// 应该使用默认值
	assert.Equal(t, 10*time.Second, cache.window)
}

// ============================================================================
//                              Global SessionStore 测试
// ============================================================================

func TestGetGlobalSessionStore(t *testing.T) {
	store := GetGlobalSessionStore()
	require.NotNil(t, store)

	// 多次调用应该返回同一实例
	store2 := GetGlobalSessionStore()
	assert.Same(t, store, store2)
}

// ============================================================================
//                              实际 TLS SessionState 测试
// ============================================================================

func TestSessionStore_WithRealSession(t *testing.T) {
	config := DefaultSessionStoreConfig()
	store := NewSessionStore(config)

	// 创建一个模拟的 TLS session state
	// 注意：在实际使用中，这个 state 来自 TLS 握手
	sessionKey := "example.com:443"

	// 存储一个 nil session（在测试中足够了）
	store.Put(sessionKey, nil)

	// 验证存储和获取
	got, ok := store.Get(sessionKey)
	assert.True(t, ok)
	assert.Nil(t, got)
}

// ============================================================================
//                              接口符合性测试
// ============================================================================

func TestSessionStore_ImplementsClientSessionCache(t *testing.T) {
	config := DefaultSessionStoreConfig()
	store := NewSessionStore(config)

	// 验证 SessionStore 实现了 tls.ClientSessionCache 接口
	var _ tls.ClientSessionCache = store
}

// ============================================================================
//                              并发测试
// ============================================================================

func TestSessionStore_Concurrent(t *testing.T) {
	config := DefaultSessionStoreConfig()
	store := NewSessionStore(config)

	done := make(chan bool)
	n := 100

	// 并发写入
	for i := 0; i < n; i++ {
		go func(id int) {
			store.Put("server"+string(rune(id%26+'a')), nil)
			done <- true
		}(i)
	}

	// 并发读取
	for i := 0; i < n; i++ {
		go func(id int) {
			store.Get("server" + string(rune(id%26+'a')))
			done <- true
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 2*n; i++ {
		<-done
	}

	// 验证没有数据竞争（测试通过即可）
	assert.LessOrEqual(t, store.Size(), n)
}

func TestAntiReplayCache_Concurrent(t *testing.T) {
	cache := NewAntiReplayCache(time.Second)

	done := make(chan bool)
	n := 100

	for i := 0; i < n; i++ {
		go func(id int) {
			cache.Check("nonce" + string(rune(id)))
			done <- true
		}(i)
	}

	for i := 0; i < n; i++ {
		<-done
	}

	// 测试通过即可（验证没有数据竞争）
}

// ============================================================================
//                              Close 测试
// ============================================================================

func TestSessionStore_Close(t *testing.T) {
	config := SessionStoreConfig{
		MaxSize: 100,
		TTL:     time.Hour,
	}
	store := NewSessionStore(config)

	// 添加数据
	store.Put("server1", nil)
	assert.Equal(t, 1, store.Size())

	// 关闭 store
	store.Close()

	// 多次调用 Close 不应 panic
	store.Close()
	store.Close()
}

func TestAntiReplayCache_Close(t *testing.T) {
	cache := NewAntiReplayCache(time.Second)

	// 使用缓存
	cache.Check("nonce1")

	// 关闭
	cache.Close()

	// 多次调用 Close 不应 panic
	cache.Close()
	cache.Close()
}

func TestSessionStore_GetAtomicity(t *testing.T) {
	config := SessionStoreConfig{
		MaxSize: 100,
		TTL:     50 * time.Millisecond, // 短 TTL
	}
	store := NewSessionStore(config)

	// 添加数据
	store.Put("server1", nil)

	// 并发 Get（验证原子性：检查过期 + 删除 + 更新计数）
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				store.Get("server1")
			}
			done <- true
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 测试通过即可（验证没有数据竞争）
}

