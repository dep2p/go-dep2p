package nat

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// TestAutoNAT_Creation 测试创建
func TestAutoNAT_Creation(t *testing.T) {
	cfg := DefaultConfig()
	an := newAutoNAT(cfg)

	if an == nil {
		t.Fatal("newAutoNAT returned nil")
	}

	// 初始状态应为 Unknown
	if an.currentStatus != ReachabilityUnknown {
		t.Errorf("Initial status = %v, want Unknown", an.currentStatus)
	}

	t.Log("✅ AutoNAT 创建正确")
}

// TestAutoNAT_RecordSuccess 测试记录成功探测
func TestAutoNAT_RecordSuccess(t *testing.T) {
	cfg := DefaultConfig()
	an := newAutoNAT(cfg)

	// 记录多次成功
	for i := 0; i < 3; i++ {
		an.recordSuccess()
	}

	// 检查置信度
	if an.confidence < 3 {
		t.Errorf("Confidence = %d, want >= 3", an.confidence)
	}

	// 状态应该变为 Public
	an.mu.RLock()
	status := an.currentStatus
	an.mu.RUnlock()

	if status != ReachabilityPublic {
		t.Errorf("Status = %v, want Public", status)
	}

	t.Log("✅ AutoNAT 成功探测记录正确")
}

// TestAutoNAT_RecordFailure 测试记录失败探测
func TestAutoNAT_RecordFailure(t *testing.T) {
	cfg := DefaultConfig()
	an := newAutoNAT(cfg)

	// 记录多次失败
	for i := 0; i < 3; i++ {
		an.recordFailure()
	}

	// 检查置信度
	if an.confidence < 3 {
		t.Errorf("Confidence = %d, want >= 3", an.confidence)
	}

	// 状态应该变为 Private
	an.mu.RLock()
	status := an.currentStatus
	an.mu.RUnlock()

	if status != ReachabilityPrivate {
		t.Errorf("Status = %v, want Private", status)
	}

	t.Log("✅ AutoNAT 失败探测记录正确")
}

// TestAutoNAT_StatusChange 测试状态切换
func TestAutoNAT_StatusChange(t *testing.T) {
	cfg := DefaultConfig()
	an := newAutoNAT(cfg)

	// 先变为 Public
	for i := 0; i < 3; i++ {
		an.recordSuccess()
	}

	an.mu.RLock()
	status := an.currentStatus
	an.mu.RUnlock()

	if status != ReachabilityPublic {
		t.Fatalf("Expected Public status")
	}

	// 再变回 Private
	an.mu.Lock()
	an.confidence = 0
	an.mu.Unlock()

	for i := 0; i < 3; i++ {
		an.recordFailure()
	}

	an.mu.RLock()
	status = an.currentStatus
	an.mu.RUnlock()

	if status != ReachabilityPrivate {
		t.Errorf("Status = %v, want Private", status)
	}

	t.Log("✅ AutoNAT 状态切换正确")
}

// TestAutoNAT_ProbeInterval 测试探测间隔
func TestAutoNAT_ProbeInterval(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ProbeInterval = 100 * time.Millisecond
	cfg.EnableAutoNAT = true

	an := newAutoNAT(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	var probeCount int32 // 使用原子变量避免数据竞争
	an.probeFunc = func() error {
		atomic.AddInt32(&probeCount, 1)
		return nil
	}

	// 启动探测循环
	go an.runProbeLoop(ctx)

	// 等待多次探测
	time.Sleep(350 * time.Millisecond)
	cancel()
	time.Sleep(50 * time.Millisecond) // 等待 goroutine 退出

	// 应该至少探测 2-3 次
	count := atomic.LoadInt32(&probeCount)
	if count < 2 {
		t.Errorf("Probe count = %d, want >= 2", count)
	}

	t.Log("✅ AutoNAT 探测间隔正确")
}

// TestAutoNAT_Confidence 测试置信度机制
func TestAutoNAT_Confidence(t *testing.T) {
	cfg := DefaultConfig()
	an := newAutoNAT(cfg)

	// 1次成功，不足以改变状态
	an.recordSuccess()
	an.mu.RLock()
	status := an.currentStatus
	an.mu.RUnlock()

	if status != ReachabilityUnknown {
		t.Errorf("Status = %v, want Unknown after 1 success", status)
	}

	// 2次成功，仍然不足
	an.recordSuccess()
	an.mu.RLock()
	status = an.currentStatus
	an.mu.RUnlock()

	if status != ReachabilityUnknown {
		t.Errorf("Status = %v, want Unknown after 2 success", status)
	}

	// 3次成功，达到置信度
	an.recordSuccess()
	an.mu.RLock()
	status = an.currentStatus
	an.mu.RUnlock()

	if status != ReachabilityPublic {
		t.Errorf("Status = %v, want Public after 3 success", status)
	}

	t.Log("✅ AutoNAT 置信度机制正确")
}

// TestAutoNAT_RecentProbes 测试最近探测记录
func TestAutoNAT_RecentProbes(t *testing.T) {
	cfg := DefaultConfig()
	an := newAutoNAT(cfg)

	// 验证 recentProbes 已初始化
	if an.recentProbes == nil {
		t.Error("recentProbes not initialized")
	}

	t.Log("✅ AutoNAT 最近探测记录初始化正确")
}
