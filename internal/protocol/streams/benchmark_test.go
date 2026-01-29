// Package streams 实现流协议
package streams

import (
	"context"
	"testing"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

func BenchmarkService_RegisterHandler(b *testing.B) {
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

	handler := func(stream interfaces.BiStream) {}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		protocol := "proto-" + string(rune('0'+i%1000))
		svc.RegisterHandler(protocol, handler)
	}
}

func BenchmarkService_UnregisterHandler(b *testing.B) {
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

	// 预先注册处理器
	handler := func(stream interfaces.BiStream) {}
	for i := 0; i < 1000; i++ {
		protocol := "proto-" + string(rune('0'+i))
		svc.RegisterHandler(protocol, handler)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		protocol := "proto-" + string(rune('0'+i%1000))
		svc.UnregisterHandler(protocol)
	}
}

func BenchmarkService_Open(b *testing.B) {
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
		b.Fatalf("New() failed: %v", err)
	}

	ctx := context.Background()
	if err := svc.Start(ctx); err != nil {
		b.Fatalf("Start() failed: %v", err)
	}
	defer svc.Stop(ctx)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream, err := svc.Open(ctx, "peer2", "test")
		if err != nil {
			b.Fatalf("Open() failed: %v", err)
		}
		if stream != nil {
			stream.Close()
		}
	}
}

func BenchmarkStreamWrapper_Write(b *testing.B) {
	mockS := newMockStream("test-protocol", "peer2", "peer1")
	wrapper := newStreamWrapper(mockS, "test-protocol")

	data := []byte("benchmark test data")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wrapper.Write(data)
	}
}

func BenchmarkStreamWrapper_Read(b *testing.B) {
	mockS := newMockStream("test-protocol", "peer2", "peer1")
	wrapper := newStreamWrapper(mockS, "test-protocol")

	// 预先写入数据
	data := []byte("benchmark test data")
	for i := 0; i < 10000; i++ {
		wrapper.Write(data)
	}

	buf := make([]byte, len(data))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wrapper.Read(buf)
	}
}
