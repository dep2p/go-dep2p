package relay

import (
	"testing"
	"time"
)

// ============================================================================
// RelayLimiter 测试（v2.0 统一限流器）
// ============================================================================

// TestRelayLimiter_Creation 测试限流器创建
func TestRelayLimiter_Creation(t *testing.T) {
	limiter := NewRelayLimiter(DefaultRelayLimiterConfig())

	if limiter == nil {
		t.Fatal("Expected non-nil limiter")
	}

	t.Log("✅ RelayLimiter 创建成功")
}

// TestRelayLimiter_AllowReservation 测试预约
func TestRelayLimiter_AllowReservation(t *testing.T) {
	limiter := NewRelayLimiter(DefaultRelayLimiterConfig())

	// 默认配置不限制，应该成功
	err := limiter.AllowReservation("peer1")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	t.Log("✅ RelayLimiter AllowReservation 成功")
}

// TestRelayLimiter_CircuitLimit 测试电路数限制
func TestRelayLimiter_CircuitLimit(t *testing.T) {
	config := RelayLimiterConfig{
		MaxConnections:        10,
		MaxConnectionsPerPeer: 2,
	}
	limiter := NewRelayLimiter(config)

	// 允许 2 个电路
	if err := limiter.AllowCircuit("peer1"); err != nil {
		t.Errorf("Unexpected error for circuit 1: %v", err)
	}
	if err := limiter.AllowCircuit("peer1"); err != nil {
		t.Errorf("Unexpected error for circuit 2: %v", err)
	}

	// 第 3 个应该失败
	if err := limiter.AllowCircuit("peer1"); err != ErrTooManyCircuits {
		t.Errorf("Expected ErrTooManyCircuits, got %v", err)
	}

	// 释放一个电路后应该成功
	limiter.ReleaseCircuit("peer1")
	if err := limiter.AllowCircuit("peer1"); err != nil {
		t.Errorf("Unexpected error after release: %v", err)
	}

	t.Log("✅ RelayLimiter CircuitLimit 成功")
}

// TestRelayLimiter_NoLimitByDefault 测试默认不限制
func TestRelayLimiter_NoLimitByDefault(t *testing.T) {
	limiter := NewRelayLimiter(DefaultRelayLimiterConfig())

	// 默认配置应该允许任意数量的预约
	for i := 0; i < 100; i++ {
		err := limiter.AllowReservation("peer")
		if err != nil {
			t.Errorf("Unexpected error at %d: %v", i, err)
		}
	}

	t.Log("✅ RelayLimiter 默认不限制")
}

// TestRelayLimiter_StrictConfig 测试严格配置
func TestRelayLimiter_StrictConfig(t *testing.T) {
	config := StrictRelayLimiterConfig()
	limiter := NewRelayLimiter(config)

	// 严格配置应该有限制
	if config.MaxConnections != 128 {
		t.Errorf("Expected MaxConnections=128, got %d", config.MaxConnections)
	}
	if config.MaxConnectionsPerPeer != 2 {
		t.Errorf("Expected MaxConnectionsPerPeer=2, got %d", config.MaxConnectionsPerPeer)
	}

	// 分配电路
	if err := limiter.AllowCircuit("peer1"); err != nil {
		t.Errorf("First circuit failed: %v", err)
	}

	t.Log("✅ RelayLimiter 严格配置成功")
}

// TestRelayLimiter_ReleaseCircuit 测试释放电路
func TestRelayLimiter_ReleaseCircuit(t *testing.T) {
	config := RelayLimiterConfig{
		MaxConnections: 2,
	}
	limiter := NewRelayLimiter(config)

	// 占用 2 个
	limiter.AllowCircuit("peer1")
	limiter.AllowCircuit("peer2")

	// 第 3 个应该失败
	if err := limiter.AllowCircuit("peer3"); err == nil {
		t.Error("Expected error")
	}

	// 释放一个
	limiter.ReleaseCircuit("peer1")

	// 现在应该成功
	if err := limiter.AllowCircuit("peer3"); err != nil {
		t.Errorf("Unexpected error after release: %v", err)
	}

	t.Log("✅ RelayLimiter ReleaseCircuit 成功")
}

// TestRelayLimiter_Stats 测试统计信息
func TestRelayLimiter_Stats(t *testing.T) {
	config := RelayLimiterConfig{
		MaxConnections:        10,
		MaxConnectionsPerPeer: 3,
	}
	limiter := NewRelayLimiter(config)

	// 分配一些电路
	limiter.AllowCircuit("peer1")
	limiter.AllowCircuit("peer1")
	limiter.AllowCircuit("peer2")

	stats := limiter.Stats()

	if stats.TotalCircuits != 3 {
		t.Errorf("Expected TotalCircuits=3, got %d", stats.TotalCircuits)
	}
	if stats.UniquePeers != 2 {
		t.Errorf("Expected UniquePeers=2, got %d", stats.UniquePeers)
	}

	t.Log("✅ RelayLimiter Stats 成功")
}

// ============================================================================
// 边界情况和 BUG 检测测试
// ============================================================================

// TestRelayLimiter_CleanupExpiredRequests 测试清理过期记录
func TestRelayLimiter_CleanupExpiredRequests(t *testing.T) {
	limiter := NewRelayLimiter(DefaultRelayLimiterConfig())

	// 直接操作内部状态来模拟过期
	limiter.mu.Lock()
	// 添加一个过期的记录（6分钟前，超过 RequestExpiry=5分钟）
	limiter.requests["expired-peer"] = time.Now().Add(-6 * time.Minute)
	// 添加一个未过期的记录（1分钟前）
	limiter.requests["active-peer"] = time.Now().Add(-1 * time.Minute)
	limiter.mu.Unlock()

	// 执行清理
	limiter.CleanupExpiredRequests()

	// 验证：过期记录应该被删除
	limiter.mu.Lock()
	_, expiredExists := limiter.requests["expired-peer"]
	_, activeExists := limiter.requests["active-peer"]
	limiter.mu.Unlock()

	if expiredExists {
		t.Error("BUG: 过期记录 'expired-peer' 应该被清理但仍然存在")
	}
	if !activeExists {
		t.Error("BUG: 未过期记录 'active-peer' 被错误清理")
	}
}

// TestRelayLimiter_ReleaseCircuit_Underflow 测试重复释放是否会导致计数器下溢
func TestRelayLimiter_ReleaseCircuit_Underflow(t *testing.T) {
	config := RelayLimiterConfig{
		MaxConnections:        2,
		MaxConnectionsPerPeer: 2,
	}
	limiter := NewRelayLimiter(config)

	// 只分配 1 个电路
	err := limiter.AllowCircuit("peer1")
	if err != nil {
		t.Fatalf("AllowCircuit 失败: %v", err)
	}

	// 释放 1 次（正常）
	limiter.ReleaseCircuit("peer1")

	// 再释放 2 次（异常调用）
	limiter.ReleaseCircuit("peer1")
	limiter.ReleaseCircuit("peer1")

	// 验证：现在应该能分配 2 个电路
	err = limiter.AllowCircuit("peer1")
	if err != nil {
		t.Errorf("BUG: 重复 ReleaseCircuit 后应该能分配电路，但得到错误: %v", err)
	}
	err = limiter.AllowCircuit("peer1")
	if err != nil {
		t.Errorf("BUG: 应该能分配第 2 个电路，但得到错误: %v", err)
	}

	// 第 3 个应该失败（验证计数器没有变成负数）
	// 可能是 ErrTooManyCircuits（per-peer 限制）或 ErrResourceLimitExceeded（总数限制）
	err = limiter.AllowCircuit("peer1")
	if err == nil {
		t.Error("BUG: 计数器可能下溢，第 3 个电路应该失败但成功了")
	}
}

// TestRelayLimiter_ReleaseCircuit_NonExistent 测试释放不存在的电路
func TestRelayLimiter_ReleaseCircuit_NonExistent(t *testing.T) {
	config := RelayLimiterConfig{
		MaxConnections: 2,
	}
	limiter := NewRelayLimiter(config)

	// 释放从未分配过的 peer
	limiter.ReleaseCircuit("non-existent-peer")

	// 验证 total 计数器没有变成负数
	// 应该仍然能分配 2 个电路
	for i := 0; i < 2; i++ {
		err := limiter.AllowCircuit("peer")
		if err != nil {
			t.Errorf("BUG: 释放不存在的 peer 后，第 %d 次 AllowCircuit 失败: %v", i+1, err)
		}
	}

	// 第 3 个应该失败
	err := limiter.AllowCircuit("peer")
	if err == nil {
		t.Error("BUG: total 计数器可能变成负数")
	}
}

// TestRelayLimiter_PerPeerLimit 测试单节点连接限制
func TestRelayLimiter_PerPeerLimit(t *testing.T) {
	config := RelayLimiterConfig{
		MaxConnections:        100, // 总连接数足够
		MaxConnectionsPerPeer: 3,   // 单节点限制 3
	}
	limiter := NewRelayLimiter(config)

	// peer1 可以分配 3 个
	for i := 0; i < 3; i++ {
		err := limiter.AllowCircuit("peer1")
		if err != nil {
			t.Errorf("peer1 第 %d 次 AllowCircuit 失败: %v", i+1, err)
		}
	}

	// peer1 第 4 个应该失败
	err := limiter.AllowCircuit("peer1")
	if err != ErrTooManyCircuits {
		t.Errorf("peer1 第 4 次应该失败，但得到: %v", err)
	}

	// peer2 应该仍然可以分配
	err = limiter.AllowCircuit("peer2")
	if err != nil {
		t.Errorf("peer2 应该可以分配，但得到: %v", err)
	}
}

// TestRelayLimiter_EmptyPeerID 测试空 peer ID 的处理
func TestRelayLimiter_EmptyPeerID(t *testing.T) {
	limiter := NewRelayLimiter(DefaultRelayLimiterConfig())

	// 空 peer ID 应该被正常处理（可能是边界情况）
	err := limiter.AllowReservation("")
	if err != nil {
		t.Logf("空 peer ID 预约: %v", err)
	}

	err = limiter.AllowCircuit("")
	if err != nil {
		t.Logf("空 peer ID 分配: %v", err)
	}

	// 验证不会影响其他 peer
	err = limiter.AllowCircuit("normal-peer")
	if err != nil {
		t.Errorf("空 peer ID 不应该影响其他 peer: %v", err)
	}
}
