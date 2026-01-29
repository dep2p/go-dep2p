// Package liveness 实现存活检测服务
package liveness

import (
	"context"
	"sync"
	"testing"
)

func TestConcurrent_Ping(t *testing.T) {
	host := newMockHost("peer1")
	realmMgr := newMockRealmManager()

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

	// 并发Ping
	var wg sync.WaitGroup
	count := 50

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.Ping(ctx, "peer2")
		}()
	}

	wg.Wait()
}

func TestConcurrent_Watch(t *testing.T) {
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

	// 并发Watch
	var wg sync.WaitGroup
	peers := []string{"peer2", "peer3", "peer4", "peer5"}

	for _, peer := range peers {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			_, err := svc.Watch(p)
			if err != nil {
				t.Errorf("Watch(%s) failed: %v", p, err)
			}
		}(peer)
	}

	wg.Wait()

	// 并发Unwatch
	for _, peer := range peers {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			svc.Unwatch(p)
		}(peer)
	}

	wg.Wait()
}

func TestConcurrent_GetStatus(t *testing.T) {
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

	// 并发获取状态
	var wg sync.WaitGroup
	count := 100

	for i := 0; i < count; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.GetStatus("peer2")
		}()
	}

	wg.Wait()
}

func TestConcurrent_Mixed(t *testing.T) {
	host := newMockHost("peer1")
	realmMgr := newMockRealmManager()

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

	// Ping
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.Ping(ctx, "peer2")
		}()
	}

	// Watch
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.Watch("peer2")
		}()
	}

	// GetStatus
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.GetStatus("peer2")
		}()
	}

	// Unwatch
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc.Unwatch("peer2")
		}()
	}

	wg.Wait()
}
