// Package streams 实现流协议
package streams

import (
	"context"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

func TestService_New(t *testing.T) {
	tests := []struct {
		name    string
		host    interfaces.Host
		realmMgr interfaces.RealmManager
		wantErr bool
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

func TestService_RegisterHandler(t *testing.T) {
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

	// 注册处理器
	handler := func(stream interfaces.BiStream) {
		// 处理流
	}

	if err := svc.RegisterHandler("test", handler); err != nil {
		t.Errorf("RegisterHandler() failed: %v", err)
	}

	// 注销处理器
	if err := svc.UnregisterHandler("test"); err != nil {
		t.Errorf("UnregisterHandler() failed: %v", err)
	}
}

func TestService_Open(t *testing.T) {
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

	// 打开流
	stream, err := svc.Open(ctx, "peer2", "test-protocol")
	if err != nil {
		t.Errorf("Open() failed: %v", err)
	}
	if stream != nil {
		defer stream.Close()
	}
}

func TestService_Close(t *testing.T) {
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

	// 关闭服务
	if err := svc.Close(); err != nil {
		t.Errorf("Close() failed: %v", err)
	}
}

// ============================================================================
//                       Config Options 测试（覆盖 0% 函数）
// ============================================================================

func TestConfig_WithReadTimeout(t *testing.T) {
	host := newMockHost("peer1")
	realmMgr := newMockRealmManager()

	svc, err := New(host, realmMgr, WithReadTimeout(60*time.Second))
	if err != nil {
		t.Fatalf("New() with WithReadTimeout failed: %v", err)
	}

	if svc.config.ReadTimeout != 60*time.Second {
		t.Errorf("WithReadTimeout() = %v, want %v", svc.config.ReadTimeout, 60*time.Second)
	}
	t.Log("✅ WithReadTimeout 测试通过")
}

func TestConfig_WithWriteTimeout(t *testing.T) {
	host := newMockHost("peer1")
	realmMgr := newMockRealmManager()

	svc, err := New(host, realmMgr, WithWriteTimeout(45*time.Second))
	if err != nil {
		t.Fatalf("New() with WithWriteTimeout failed: %v", err)
	}

	if svc.config.WriteTimeout != 45*time.Second {
		t.Errorf("WithWriteTimeout() = %v, want %v", svc.config.WriteTimeout, 45*time.Second)
	}
	t.Log("✅ WithWriteTimeout 测试通过")
}

func TestConfig_WithMaxStreamBuffer(t *testing.T) {
	host := newMockHost("peer1")
	realmMgr := newMockRealmManager()

	svc, err := New(host, realmMgr, WithMaxStreamBuffer(8192))
	if err != nil {
		t.Fatalf("New() with WithMaxStreamBuffer failed: %v", err)
	}

	if svc.config.MaxStreamBuffer != 8192 {
		t.Errorf("WithMaxStreamBuffer() = %v, want %v", svc.config.MaxStreamBuffer, 8192)
	}
	t.Log("✅ WithMaxStreamBuffer 测试通过")
}

func TestConfig_WithDefaultRealmID(t *testing.T) {
	host := newMockHost("peer1")
	realmMgr := newMockRealmManager()

	svc, err := New(host, realmMgr, WithDefaultRealmID("my-realm"))
	if err != nil {
		t.Fatalf("New() with WithDefaultRealmID failed: %v", err)
	}

	if svc.config.DefaultRealmID != "my-realm" {
		t.Errorf("WithDefaultRealmID() = %v, want %v", svc.config.DefaultRealmID, "my-realm")
	}
	t.Log("✅ WithDefaultRealmID 测试通过")
}

func TestConfig_MultipleOptions(t *testing.T) {
	host := newMockHost("peer1")
	realmMgr := newMockRealmManager()

	svc, err := New(host, realmMgr,
		WithReadTimeout(10*time.Second),
		WithWriteTimeout(20*time.Second),
		WithMaxStreamBuffer(1024),
		WithDefaultRealmID("combined-test"),
	)
	if err != nil {
		t.Fatalf("New() with multiple options failed: %v", err)
	}

	if svc.config.ReadTimeout != 10*time.Second {
		t.Errorf("ReadTimeout = %v, want %v", svc.config.ReadTimeout, 10*time.Second)
	}
	if svc.config.WriteTimeout != 20*time.Second {
		t.Errorf("WriteTimeout = %v, want %v", svc.config.WriteTimeout, 20*time.Second)
	}
	if svc.config.MaxStreamBuffer != 1024 {
		t.Errorf("MaxStreamBuffer = %v, want %v", svc.config.MaxStreamBuffer, 1024)
	}
	if svc.config.DefaultRealmID != "combined-test" {
		t.Errorf("DefaultRealmID = %v, want %v", svc.config.DefaultRealmID, "combined-test")
	}
	t.Log("✅ 多选项组合测试通过")
}

func TestConfig_DefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.ReadTimeout != 30*time.Second {
		t.Errorf("DefaultConfig ReadTimeout = %v, want %v", cfg.ReadTimeout, 30*time.Second)
	}
	if cfg.WriteTimeout != 30*time.Second {
		t.Errorf("DefaultConfig WriteTimeout = %v, want %v", cfg.WriteTimeout, 30*time.Second)
	}
	if cfg.MaxStreamBuffer != 4096 {
		t.Errorf("DefaultConfig MaxStreamBuffer = %v, want %v", cfg.MaxStreamBuffer, 4096)
	}
	if cfg.DefaultRealmID != "" {
		t.Errorf("DefaultConfig DefaultRealmID = %v, want empty", cfg.DefaultRealmID)
	}
	t.Log("✅ DefaultConfig 测试通过")
}
