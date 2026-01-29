// Package liveness 实现存活检测服务
package liveness

import (
	"context"
	"testing"
)

func BenchmarkService_Ping(b *testing.B) {
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
		b.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		b.Fatalf("Start() failed: %v", err)
	}
	defer svc.Stop(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.Ping(ctx, "peer2")
	}
}

func BenchmarkService_Check(b *testing.B) {
	host := newMockHost("peer1")
	realmMgr := newMockRealmManager()

	svc, err := New(host, realmMgr)
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		b.Fatalf("Start() failed: %v", err)
	}
	defer svc.Stop(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.Check(ctx, "peer2")
	}
}

func BenchmarkService_GetStatus(b *testing.B) {
	host := newMockHost("peer1")
	realmMgr := newMockRealmManager()

	svc, err := New(host, realmMgr)
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		b.Fatalf("Start() failed: %v", err)
	}
	defer svc.Stop(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		svc.GetStatus("peer2")
	}
}

func BenchmarkService_Watch(b *testing.B) {
	host := newMockHost("peer1")
	realmMgr := newMockRealmManager()

	svc, err := New(host, realmMgr)
	if err != nil {
		b.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		b.Fatalf("Start() failed: %v", err)
	}
	defer svc.Stop(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		peer := "peer-" + string(rune('0'+i%1000))
		svc.Watch(peer)
	}
}

func BenchmarkPeerStatus_RecordSuccess(b *testing.B) {
	ps := newPeerStatus("peer1")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ps.recordSuccess(100)
	}
}

func BenchmarkPeerStatus_GetStatus(b *testing.B) {
	ps := newPeerStatus("peer1")
	ps.recordSuccess(100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ps.getStatus()
	}
}
