// Package liveness 实现存活检测服务
package liveness

import (
	"context"
	"testing"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

func TestService_New(t *testing.T) {
	tests := []struct {
		name     string
		host     interfaces.Host
		realmMgr interfaces.RealmManager
		wantErr  bool
	}{
		{
			name:     "成功创建",
			host:     newMockHost("peer1"),
			realmMgr: newMockRealmManager(),
			wantErr:  false,
		},
		{
			name:     "Host为nil",
			host:     nil,
			realmMgr: newMockRealmManager(),
			wantErr:  true,
		},
		{
			name:     "RealmManager为nil",
			host:     newMockHost("peer1"),
			realmMgr: nil,
			wantErr:  false, // RealmManager 现在是可选的
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, err := New(tt.host, tt.realmMgr)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && svc == nil {
				t.Error("New() returned nil service")
			}
		})
	}
}

func TestService_StartStop(t *testing.T) {
	host := newMockHost("peer1")
	realmMgr := newMockRealmManager()

	svc, err := New(host, realmMgr)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()

	// 启动
	if err := svc.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	// 重复启动应该失败
	if err := svc.Start(ctx); err == nil {
		t.Error("Start() should fail when already started")
	}

	// 停止
	if err := svc.Stop(ctx); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}

	// 重复停止应该失败
	if err := svc.Stop(ctx); err == nil {
		t.Error("Stop() should fail when not started")
	}
}

func TestService_Ping(t *testing.T) {
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

	// Ping测试
	rtt, err := svc.Ping(ctx, "peer2")
	if err != nil {
		t.Errorf("Ping() failed: %v", err)
	}
	if rtt < 0 {
		t.Errorf("Ping() returned negative RTT: %v", rtt)
	}
}

func TestService_Check(t *testing.T) {
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

	// Check测试
	alive, err := svc.Check(ctx, "peer2")
	if err != nil {
		t.Errorf("Check() failed: %v", err)
	}
	t.Logf("Check() returned alive=%v", alive)
}

func TestService_GetStatus(t *testing.T) {
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

	// GetStatus测试
	status := svc.GetStatus("peer2")
	t.Logf("GetStatus() = %+v", status)
}
