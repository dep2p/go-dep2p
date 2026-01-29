// Package streams 实现流协议
package streams

import (
	"context"
	"sync"
	"testing"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

func TestConcurrent_RegisterHandler(t *testing.T) {
	host := newMockHost("peer1")
	realmMgr := newMockRealmManager()

	svc, err := New(host, realmMgr)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer svc.Stop(ctx)

	// 并发注册处理器
	var wg sync.WaitGroup
	count := 100

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			protocol := "proto-" + string(rune('0'+idx%10))
			handler := func(stream interfaces.BiStream) {}
			// 忽略重复注册错误（并发环境下可能发生）
			svc.RegisterHandler(protocol, handler)
		}(i)
	}

	wg.Wait()

	// 验证处理器数量
	svc.mu.RLock()
	handlerCount := len(svc.handlers)
	svc.mu.RUnlock()

	if handlerCount == 0 {
		t.Error("Expected some handlers to be registered")
	}
}

func TestConcurrent_UnregisterHandler(t *testing.T) {
	host := newMockHost("peer1")
	realmMgr := newMockRealmManager()

	svc, err := New(host, realmMgr)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer svc.Stop(ctx)

	// 先注册处理器
	protocols := []string{"p1", "p2", "p3", "p4", "p5"}
	for _, proto := range protocols {
		if err := svc.RegisterHandler(proto, func(s interfaces.BiStream) {}); err != nil {
			t.Fatalf("RegisterHandler() failed: %v", err)
		}
	}

	// 并发注销处理器
	var wg sync.WaitGroup
	for _, proto := range protocols {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			if err := svc.UnregisterHandler(p); err != nil {
				t.Errorf("UnregisterHandler() failed: %v", err)
			}
		}(proto)
	}

	wg.Wait()

	// 验证所有处理器已注销
	svc.mu.RLock()
	count := len(svc.handlers)
	svc.mu.RUnlock()

	if count != 0 {
		t.Errorf("Expected 0 handlers, got %d", count)
	}
}

func TestConcurrent_Open(t *testing.T) {
	host := newMockHost("peer1")
	realmMgr := newMockRealmManager()

	// 创建测试Realm
	realm := &mockRealm{
		id:      "test-realm",
		name:    "Test Realm",
		members: make(map[string]bool),
	}
	realm.AddMember("peer2")
	realmMgr.realms["test-realm"] = realm

	svc, err := New(host, realmMgr)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer svc.Stop(ctx)

	// 并发打开流
	var wg sync.WaitGroup
	count := 50

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stream, err := svc.Open(ctx, "peer2", "test")
			if err != nil {
				t.Errorf("Open() failed: %v", err)
				return
			}
			if stream != nil {
				stream.Close()
			}
		}()
	}

	wg.Wait()
}

func TestConcurrent_Mixed(t *testing.T) {
	host := newMockHost("peer1")
	realmMgr := newMockRealmManager()

	// 创建测试Realm
	realm := &mockRealm{
		id:      "test-realm",
		name:    "Test Realm",
		members: make(map[string]bool),
	}
	realm.AddMember("peer2")
	realmMgr.realms["test-realm"] = realm

	svc, err := New(host, realmMgr)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}
	defer svc.Stop(ctx)

	// 混合并发操作
	var wg sync.WaitGroup

	// 注册处理器
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			protocol := "proto-" + string(rune('0'+idx%5))
			if err := svc.RegisterHandler(protocol, func(s interfaces.BiStream) {}); err != nil {
				// 可能重复注册，忽略错误
			}
		}(i)
	}

	// 打开流
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stream, err := svc.Open(ctx, "peer2", "test")
			if err != nil {
				return
			}
			if stream != nil {
				stream.Close()
			}
		}()
	}

	// 注销处理器
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			protocol := "proto-" + string(rune('0'+idx%5))
			svc.UnregisterHandler(protocol)
		}(i)
	}

	wg.Wait()
}
