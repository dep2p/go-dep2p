package messaging

import (
	"context"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

func BenchmarkCodec_EncodeRequest(b *testing.B) {
	codec := NewCodec()
	req := &interfaces.Request{
		ID:       "bench-request",
		From:     "peer-1",
		Protocol: "test",
		Data:     make([]byte, 1024),
		Timestamp: time.Now(),
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = codec.EncodeRequest(req)
	}
}

func BenchmarkCodec_DecodeRequest(b *testing.B) {
	codec := NewCodec()
	req := &interfaces.Request{
		ID:       "bench-request",
		From:     "peer-1",
		Data:     make([]byte, 1024),
		Timestamp: time.Now(),
	}
	
	data, _ := codec.EncodeRequest(req)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = codec.DecodeRequest(data)
	}
}

func BenchmarkCodec_EncodeResponse(b *testing.B) {
	codec := NewCodec()
	resp := &interfaces.Response{
		ID:   "bench-response",
		From: "peer-1",
		Data: make([]byte, 1024),
		Timestamp: time.Now(),
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = codec.EncodeResponse(resp)
	}
}

func BenchmarkCodec_DecodeResponse(b *testing.B) {
	codec := NewCodec()
	resp := &interfaces.Response{
		ID:   "bench-response",
		From: "peer-1",
		Data: make([]byte, 1024),
		Timestamp: time.Now(),
	}
	
	data, _ := codec.EncodeResponse(resp)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = codec.DecodeResponse(data)
	}
}

func BenchmarkHandlerRegistry_Register(b *testing.B) {
	handler := func(ctx context.Context, req *interfaces.Request) (*interfaces.Response, error) {
		return &interfaces.Response{}, nil
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		registry := NewHandlerRegistry()
		_ = registry.Register("test", handler)
	}
}

func BenchmarkHandlerRegistry_Get(b *testing.B) {
	registry := NewHandlerRegistry()
	handler := func(ctx context.Context, req *interfaces.Request) (*interfaces.Response, error) {
		return &interfaces.Response{}, nil
	}
	_ = registry.Register("test", handler)
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = registry.Get("test")
	}
}

func BenchmarkService_RegisterHandler(b *testing.B) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()
	
	realm := newMockRealm("realm-1", "Test Realm")
	realmMgr.AddRealm(realm)
	
	svc, _ := New(host, realmMgr)
	ctx := context.Background()
	_ = svc.Start(ctx)
	defer svc.Close()
	
	handler := func(ctx context.Context, req *interfaces.Request) (*interfaces.Response, error) {
		return &interfaces.Response{}, nil
	}
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		proto := string(rune('a' + (i % 26)))
		_ = svc.RegisterHandler(proto, handler)
		_ = svc.UnregisterHandler(proto)
	}
}

func BenchmarkBuildProtocolID(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildProtocolID("realm-1", "test")
	}
}

func BenchmarkValidateProtocol(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = validateProtocol("test-protocol")
	}
}

func BenchmarkCodec_LargePayload(b *testing.B) {
	codec := NewCodec()
	
	// 1MB payload
	largeData := make([]byte, 1024*1024)
	req := &interfaces.Request{
		ID:   "large",
		From: "peer-1",
		Data: largeData,
		Timestamp: time.Now(),
	}
	
	b.ResetTimer()
	b.Run("Encode", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = codec.EncodeRequest(req)
		}
	})
	
	data, _ := codec.EncodeRequest(req)
	b.Run("Decode", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_, _ = codec.DecodeRequest(data)
		}
	})
}

func BenchmarkParallel_RegisterHandler(b *testing.B) {
	host := newMockHost("peer-1")
	realmMgr := newMockRealmManager()
	
	realm := newMockRealm("realm-1", "Test Realm")
	realmMgr.AddRealm(realm)
	
	svc, _ := New(host, realmMgr)
	ctx := context.Background()
	_ = svc.Start(ctx)
	defer svc.Close()
	
	handler := func(ctx context.Context, req *interfaces.Request) (*interfaces.Response, error) {
		return &interfaces.Response{}, nil
	}
	
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			proto := string(rune('a' + (i % 26)))
			_ = svc.RegisterHandler(proto, handler)
			_ = svc.UnregisterHandler(proto)
			i++
		}
	})
}
