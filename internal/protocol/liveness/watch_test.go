// Package liveness 实现存活检测服务
package liveness

import (
	"context"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

func TestService_Watch(t *testing.T) {
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

	// Watch节点
	ch, err := svc.Watch("peer2")
	if err != nil {
		t.Fatalf("Watch() failed: %v", err)
	}

	if ch == nil {
		t.Fatal("Watch() returned nil channel")
	}

	// 应该能够从通道接收
	select {
	case <-ch:
		// 收到事件
	case <-time.After(100 * time.Millisecond):
		// 超时是预期的，因为还没有触发事件
	}
}

func TestService_Unwatch(t *testing.T) {
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

	// Watch然后Unwatch
	ch, err := svc.Watch("peer2")
	if err != nil {
		t.Fatalf("Watch() failed: %v", err)
	}

	if err := svc.Unwatch("peer2"); err != nil {
		t.Errorf("Unwatch() failed: %v", err)
	}

	// 通道应该被关闭
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("Channel should be closed after Unwatch()")
		}
	case <-time.After(100 * time.Millisecond):
		// 如果通道未关闭，会超时
	}
}

func TestService_MultipleWatches(t *testing.T) {
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

	// 多个Watch
	peers := []string{"peer2", "peer3", "peer4"}
	channels := make(map[string]<-chan interfaces.LivenessEvent)

	for _, peer := range peers {
		ch, err := svc.Watch(peer)
		if err != nil {
			t.Errorf("Watch(%s) failed: %v", peer, err)
			continue
		}
		channels[peer] = ch
	}

	// 验证所有通道都已创建
	if len(channels) != len(peers) {
		t.Errorf("Expected %d channels, got %d", len(peers), len(channels))
	}

	// Unwatch所有
	for _, peer := range peers {
		if err := svc.Unwatch(peer); err != nil {
			t.Errorf("Unwatch(%s) failed: %v", peer, err)
		}
	}
}
