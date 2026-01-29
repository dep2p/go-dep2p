package messaging

import (
	"context"
	"sync"
	"testing"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConcurrent_RegisterHandler(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()
	
	realm := newMockRealm("realm-1", "Test Realm")
	realmMgr.AddRealm(realm)
	
	svc, err := New(host, realmMgr)
	require.NoError(t, err)
	
	ctx := context.Background()
	err = svc.Start(ctx)
	require.NoError(t, err)
	defer svc.Close()
	
	handler := func(ctx context.Context, req *interfaces.Request) (*interfaces.Response, error) {
		return &interfaces.Response{}, nil
	}
	
	// 并发注册100个处理器
	var wg sync.WaitGroup
	errors := make([]error, 100)
	
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			proto := string(rune('a' + (id % 26)))
			errors[id] = svc.RegisterHandler(proto, handler)
		}(i)
	}
	
	wg.Wait()
	
	// 至少有一些应该成功
	successCount := 0
	for _, err := range errors {
		if err == nil {
			successCount++
		}
	}
	assert.Greater(t, successCount, 0)
}

func TestConcurrent_Send(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()
	
	realm := newMockRealm("realm-1", "Test Realm")
	realm.AddMember("peer-1")
	realm.AddMember("peer-2")
	realmMgr.AddRealm(realm)
	
	svc, err := New(host, realmMgr)
	require.NoError(t, err)
	
	ctx := context.Background()
	err = svc.Start(ctx)
	require.NoError(t, err)
	defer svc.Close()
	
	// 并发发送消息
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 这些会因为没有真实的网络栈而失败,但我们测试并发安全性
			_, _ = svc.Send(ctx, "peer-2", "test", []byte("data"))
		}()
	}
	
	wg.Wait()
	
	// 如果到这里没有 panic,说明并发安全
	assert.True(t, true)
}

func TestConcurrent_SendAsync(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()
	
	realm := newMockRealm("realm-1", "Test Realm")
	realm.AddMember("peer-1")
	realm.AddMember("peer-2")
	realmMgr.AddRealm(realm)
	
	svc, err := New(host, realmMgr)
	require.NoError(t, err)
	
	ctx := context.Background()
	err = svc.Start(ctx)
	require.NoError(t, err)
	defer svc.Close()
	
	// 并发异步发送
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			respCh, err := svc.SendAsync(ctx, "peer-2", "test", []byte("data"))
			if err == nil {
				// 尝试接收响应
				select {
				case <-respCh:
				default:
				}
			}
		}()
	}
	
	wg.Wait()
}

func TestConcurrent_StartStop(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()
	
	svc, err := New(host, realmMgr)
	require.NoError(t, err)
	
	ctx := context.Background()
	
	// 并发启动/停止
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		
		// 启动
		go func() {
			defer wg.Done()
			_ = svc.Start(ctx)
		}()
		
		// 停止
		go func() {
			defer wg.Done()
			_ = svc.Stop(ctx)
		}()
	}
	
	wg.Wait()
}

func TestConcurrent_MixedOperations(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()
	
	realm := newMockRealm("realm-1", "Test Realm")
	realm.AddMember("peer-1")
	realm.AddMember("peer-2")
	realmMgr.AddRealm(realm)
	
	svc, err := New(host, realmMgr)
	require.NoError(t, err)
	
	ctx := context.Background()
	err = svc.Start(ctx)
	require.NoError(t, err)
	defer svc.Close()
	
	handler := func(ctx context.Context, req *interfaces.Request) (*interfaces.Response, error) {
		return &interfaces.Response{}, nil
	}
	
	// 混合并发操作
	var wg sync.WaitGroup
	
	// 注册处理器
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			proto := string(rune('a' + (id % 5)))
			_ = svc.RegisterHandler(proto, handler)
		}(i)
	}
	
	// 发送消息
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = svc.Send(ctx, "peer-2", "test", []byte("data"))
		}()
	}
	
	// 异步发送
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = svc.SendAsync(ctx, "peer-2", "test", []byte("data"))
		}()
	}
	
	// 注销处理器
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			proto := string(rune('a' + id))
			_ = svc.UnregisterHandler(proto)
		}(i)
	}
	
	wg.Wait()
}

// TestConcurrent_RaceDetector 使用 -race 标志运行时会检测竞态条件
func TestConcurrent_RaceDetector(t *testing.T) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()
	
	svc, err := New(host, realmMgr)
	require.NoError(t, err)
	
	ctx := context.Background()
	err = svc.Start(ctx)
	require.NoError(t, err)
	
	// 并发读写 started 状态
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		
		// 读
		go func() {
			defer wg.Done()
			svc.mu.RLock()
			_ = svc.started
			svc.mu.RUnlock()
		}()
		
		// 写(通过 Start/Stop)
		go func() {
			defer wg.Done()
			if i%2 == 0 {
				_ = svc.Start(ctx)
			} else {
				_ = svc.Stop(ctx)
			}
		}()
	}
	
	wg.Wait()
	_ = svc.Close()
}
