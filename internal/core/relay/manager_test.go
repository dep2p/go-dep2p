package relay

import (
	"context"
	"testing"
)

// TestManager_Creation 测试管理器创建
func TestManager_Creation(t *testing.T) {
	config := DefaultConfig()
	mgr := NewManager(config, nil, nil, nil, nil)

	if mgr == nil {
		t.Fatal("NewManager returned nil")
	}

	if mgr.config != config {
		t.Error("Manager config mismatch")
	}
}

// TestManager_StartStop 测试生命周期
func TestManager_StartStop(t *testing.T) {
	config := DefaultConfig()
	mgr := NewManager(config, nil, nil, nil, nil)

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if err := mgr.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

// TestManager_RelayMethods_V2 测试统一 Relay 方法（v2.0）
func TestManager_RelayMethods_V2(t *testing.T) {
	config := DefaultConfig()
	mgr := NewManager(config, nil, nil, nil, nil)

	// 测试初始状态
	if mgr.IsRelayEnabled() {
		t.Error("Relay should not be enabled initially")
	}

	// 测试 HasRelay
	if mgr.HasRelay() {
		t.Error("HasRelay should return false when no relay configured")
	}

	// 测试 RelayStats
	stats := mgr.RelayStats()
	if stats.Enabled {
		t.Error("RelayStats.Enabled should be false initially")
	}

	// 测试 RelayAddr
	addr, ok := mgr.RelayAddr()
	if ok || addr != nil {
		t.Error("RelayAddr should return (nil, false) initially")
	}
}

// TestManager_RelayMethods 测试 Relay 方法
func TestManager_RelayMethods(t *testing.T) {
	config := DefaultConfig()
	mgr := NewManager(config, nil, nil, nil, nil)

	// 测试初始状态
	if mgr.IsRelayEnabled() {
		t.Error("IsRelayEnabled should return false initially")
	}

	if mgr.HasRelay() {
		t.Error("HasRelay should return false initially")
	}

	stats := mgr.RelayStats()
	if stats.Enabled {
		t.Error("RelayStats.Enabled should be false initially")
	}

	addr, ok := mgr.RelayAddr()
	if ok || addr != nil {
		t.Error("RelayAddr should return (nil, false) initially")
	}
}

// TestManager_Close 测试关闭
func TestManager_Close(t *testing.T) {
	config := DefaultConfig()
	mgr := NewManager(config, nil, nil, nil, nil)

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if err := mgr.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// 再次关闭应该是安全的
	if err := mgr.Close(); err != nil {
		t.Fatalf("Second Close failed: %v", err)
	}
}
