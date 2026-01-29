package storage

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/config"
	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"go.uber.org/fx"
	"go.uber.org/fx/fxtest"
)

// ============= Fx 模块测试 =============

func TestModule_Basic(t *testing.T) {
	// 使用 t.TempDir() 创建临时目录，确保测试与生产一致
	tmpDir := t.TempDir()

	var eng engine.InternalEngine
	var cfg Config

	// 提供统一配置
	unifiedCfg := config.NewConfig()
	unifiedCfg.Storage.DataDir = tmpDir

	app := fxtest.New(t,
		fx.Supply(unifiedCfg),
		Module(),
		fx.Populate(&eng, &cfg),
	)

	app.RequireStart()
	defer app.RequireStop()

	// 验证引擎已创建
	if eng == nil {
		t.Fatal("engine is nil")
	}

	// 验证配置 - 路径应该设置正确
	if cfg.Path == "" {
		t.Error("expected Path to be set")
	}

	// 测试基本操作
	key := []byte("test-key")
	value := []byte("test-value")

	if err := eng.Put(key, value); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	got, err := eng.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if !bytes.Equal(got, value) {
		t.Errorf("Get returned %q, want %q", got, value)
	}
}

func TestModule_Lifecycle(t *testing.T) {
	tmpDir := t.TempDir()

	var eng engine.InternalEngine

	startCalled := false
	stopCalled := false

	unifiedCfg := config.NewConfig()
	unifiedCfg.Storage.DataDir = tmpDir

	app := fxtest.New(t,
		fx.Supply(unifiedCfg),
		Module(),
		fx.Populate(&eng),
		fx.Invoke(func(lc fx.Lifecycle, e engine.InternalEngine) {
			lc.Append(fx.Hook{
				OnStart: func(ctx context.Context) error {
					startCalled = true
					// 验证引擎已启动
					return e.Put([]byte("lifecycle-key"), []byte("started"))
				},
				OnStop: func(ctx context.Context) error {
					stopCalled = true
					return nil
				},
			})
		}),
	)

	app.RequireStart()

	if !startCalled {
		t.Error("OnStart hook not called")
	}

	// 验证数据存在
	val, err := eng.Get([]byte("lifecycle-key"))
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(val) != "started" {
		t.Errorf("unexpected value: %s", val)
	}

	app.RequireStop()

	if !stopCalled {
		t.Error("OnStop hook not called")
	}
}

// ============= 配置测试 =============

func TestConfig_Default(t *testing.T) {
	cfg := DefaultConfig()

	// 默认配置应该有路径
	if cfg.Path == "" {
		t.Error("default config should have a path")
	}

	if cfg.GCInterval != 10*time.Minute {
		t.Errorf("unexpected GCInterval: %v", cfg.GCInterval)
	}
}

func TestConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "valid default",
			cfg:     DefaultConfig(),
			wantErr: false,
		},
		{
			name: "valid with custom path",
			cfg: Config{
				Path: "/data/test",
			},
			wantErr: false,
		},
		{
			name: "invalid - empty path",
			cfg: Config{
				Path: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestConfig_WithMethods(t *testing.T) {
	cfg := DefaultConfig().
		WithPath("/data/storage").
		WithSyncWrites(true).
		WithGC(true, 5*time.Minute).
		WithBlockCache(512 << 20).
		WithCompression(3)

	if cfg.Path != "/data/storage" {
		t.Errorf("unexpected path: %s", cfg.Path)
	}

	if !cfg.SyncWrites {
		t.Error("SyncWrites should be true")
	}

	if cfg.GCInterval != 5*time.Minute {
		t.Errorf("unexpected GCInterval: %v", cfg.GCInterval)
	}

	if cfg.BlockCacheSize != 512<<20 {
		t.Errorf("unexpected BlockCacheSize: %d", cfg.BlockCacheSize)
	}

	if cfg.Compression != 3 {
		t.Errorf("unexpected Compression: %d", cfg.Compression)
	}
}

// ============= 便捷函数测试 =============

func TestNew(t *testing.T) {
	// 使用 t.TempDir() 创建临时目录
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	eng, err := New(dbPath)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer eng.Close()

	// 测试基本操作
	if err := eng.Put([]byte("key"), []byte("value")); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	val, err := eng.Get([]byte("key"))
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if string(val) != "value" {
		t.Errorf("unexpected value: %s", val)
	}
}

func TestNewKVStore(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	eng, err := New(dbPath)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer eng.Close()

	// 创建两个隔离的 KVStore
	store1 := NewKVStore(eng, []byte("s1/"))
	store2 := NewKVStore(eng, []byte("s2/"))

	// 在 store1 写入
	if err := store1.Put([]byte("key"), []byte("value1")); err != nil {
		t.Fatalf("store1.Put failed: %v", err)
	}

	// 在 store2 写入同名键
	if err := store2.Put([]byte("key"), []byte("value2")); err != nil {
		t.Fatalf("store2.Put failed: %v", err)
	}

	// 验证隔离
	val1, err := store1.Get([]byte("key"))
	if err != nil {
		t.Fatalf("store1.Get failed: %v", err)
	}
	if string(val1) != "value1" {
		t.Errorf("store1 value = %s, want value1", val1)
	}

	val2, err := store2.Get([]byte("key"))
	if err != nil {
		t.Fatalf("store2.Get failed: %v", err)
	}
	if string(val2) != "value2" {
		t.Errorf("store2 value = %s, want value2", val2)
	}
}

// ============= 错误重导出测试 =============

func TestErrors(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	eng, err := New(dbPath)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer eng.Close()

	// 测试 ErrNotFound
	_, err = eng.Get([]byte("nonexistent"))
	if !IsNotFound(err) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}

	// 测试 ErrEmptyKey
	err = eng.Put([]byte{}, []byte("value"))
	if err != ErrEmptyKey {
		t.Errorf("expected ErrEmptyKey, got %v", err)
	}

	// 关闭后测试 ErrClosed
	eng.Close()
	err = eng.Put([]byte("key"), []byte("value"))
	if !IsClosed(err) {
		t.Errorf("expected ErrClosed, got %v", err)
	}
}

// ============= 多模块集成测试 =============

func TestModule_MultipleKVStores(t *testing.T) {
	tmpDir := t.TempDir()

	var eng engine.InternalEngine

	unifiedCfg := config.NewConfig()
	unifiedCfg.Storage.DataDir = tmpDir

	app := fxtest.New(t,
		fx.Supply(unifiedCfg),
		Module(),
		fx.Populate(&eng),
	)

	app.RequireStart()
	defer app.RequireStop()

	// 模拟多个组件使用不同前缀
	peerstore := NewKVStore(eng, []byte("p/"))
	dht := NewKVStore(eng, []byte("d/"))
	addressbook := NewKVStore(eng, []byte("a/"))

	// 各自写入数据
	if err := peerstore.Put([]byte("addr/peer1"), []byte("addr1")); err != nil {
		t.Fatalf("peerstore.Put failed: %v", err)
	}

	if err := dht.Put([]byte("v/key1"), []byte("value1")); err != nil {
		t.Fatalf("dht.Put failed: %v", err)
	}

	if err := addressbook.Put([]byte("realm1/node1"), []byte("addrs")); err != nil {
		t.Fatalf("addressbook.Put failed: %v", err)
	}

	// 验证各自独立
	if _, err := peerstore.Get([]byte("v/key1")); !IsNotFound(err) {
		t.Error("peerstore should not see dht data")
	}

	if _, err := dht.Get([]byte("addr/peer1")); !IsNotFound(err) {
		t.Error("dht should not see peerstore data")
	}

	// 验证正确读取
	val, err := peerstore.Get([]byte("addr/peer1"))
	if err != nil {
		t.Fatalf("peerstore.Get failed: %v", err)
	}
	if string(val) != "addr1" {
		t.Errorf("unexpected value: %s", val)
	}
}

// ============= 数据持久化测试 =============

func TestPersistence(t *testing.T) {
	// 使用 t.TempDir() - 目录在测试结束后自动清理
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "persist.db")

	// 第一次：写入数据
	{
		eng, err := New(dbPath)
		if err != nil {
			t.Fatalf("New failed: %v", err)
		}

		if err := eng.Put([]byte("persist-key"), []byte("persist-value")); err != nil {
			t.Fatalf("Put failed: %v", err)
		}

		if err := eng.Close(); err != nil {
			t.Fatalf("Close failed: %v", err)
		}
	}

	// 第二次：重新打开，验证数据存在
	{
		eng, err := New(dbPath)
		if err != nil {
			t.Fatalf("New (reopen) failed: %v", err)
		}
		defer eng.Close()

		val, err := eng.Get([]byte("persist-key"))
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}

		if string(val) != "persist-value" {
			t.Errorf("unexpected value after reopen: %s", val)
		}
	}
}
