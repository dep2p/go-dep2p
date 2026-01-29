package relay

import (
	"testing"
	"time"
	
	"github.com/dep2p/go-dep2p/internal/core/nat"
)

// TestRelayCandidatePool_Add 测试添加候选
func TestRelayCandidatePool_Add(t *testing.T) {
	pool := NewRelayCandidatePool("test-realm")
	
	// 添加公网可达候选
	candidate := &RelayCandidate{
		PeerID:       "peer1",
		Addrs:        []string{"/ip4/1.2.3.4/tcp/4001"},
		Reachability: nat.ReachabilityPublic,
		LastSeen:     time.Now(),
	}
	
	pool.Add(candidate)
	
	// 验证添加成功
	if pool.Count() != 1 {
		t.Errorf("expected count 1, got %d", pool.Count())
	}
	
	// 添加非公网可达候选（应该被拒绝）
	candidate2 := &RelayCandidate{
		PeerID:       "peer2",
		Addrs:        []string{"/ip4/192.168.1.1/tcp/4001"},
		Reachability: nat.ReachabilityPrivate,
	}
	
	pool.Add(candidate2)
	
	// 验证未添加
	if pool.Count() != 1 {
		t.Errorf("expected count 1, got %d", pool.Count())
	}
}

// TestRelayCandidatePool_Remove 测试移除候选
func TestRelayCandidatePool_Remove(t *testing.T) {
	pool := NewRelayCandidatePool("test-realm")
	
	// 添加候选
	candidate := &RelayCandidate{
		PeerID:       "peer1",
		Addrs:        []string{"/ip4/1.2.3.4/tcp/4001"},
		Reachability: nat.ReachabilityPublic,
		LastSeen:     time.Now(),
	}
	
	pool.Add(candidate)
	
	// 验证添加成功
	if pool.Count() != 1 {
		t.Fatalf("expected count 1, got %d", pool.Count())
	}
	
	// 移除候选
	pool.Remove("peer1")
	
	// 验证移除成功
	if pool.Count() != 0 {
		t.Errorf("expected count 0, got %d", pool.Count())
	}
}

// TestRelayCandidatePool_UpdateReachability 测试更新可达性
func TestRelayCandidatePool_UpdateReachability(t *testing.T) {
	pool := NewRelayCandidatePool("test-realm")
	
	// 添加公网可达候选
	candidate := &RelayCandidate{
		PeerID:       "peer1",
		Addrs:        []string{"/ip4/1.2.3.4/tcp/4001"},
		Reachability: nat.ReachabilityPublic,
		LastSeen:     time.Now(),
	}
	
	pool.Add(candidate)
	
	// 更新为非公网可达（应自动移除）
	pool.UpdateReachability("peer1", nat.ReachabilityPrivate)
	
	// 验证已移除
	if pool.Count() != 0 {
		t.Errorf("expected count 0 after reachability change, got %d", pool.Count())
	}
}

// TestRelayCandidatePool_SelectBest 测试选择最优候选
func TestRelayCandidatePool_SelectBest(t *testing.T) {
	pool := NewRelayCandidatePool("test-realm")
	
	// 空池返回 nil
	best := pool.SelectBest()
	if best != nil {
		t.Errorf("expected nil for empty pool, got %v", best)
	}
	
	// 添加候选
	now := time.Now()
	
	candidate1 := &RelayCandidate{
		PeerID:       "peer1",
		Addrs:        []string{"/ip4/1.2.3.4/tcp/4001"},
		Reachability: nat.ReachabilityPublic,
		LastSeen:     now.Add(-1 * time.Hour), // 1小时前
	}
	
	candidate2 := &RelayCandidate{
		PeerID:       "peer2",
		Addrs:        []string{"/ip4/5.6.7.8/tcp/4001"},
		Reachability: nat.ReachabilityPublic,
		LastSeen:     now, // 最新
	}
	
	pool.Add(candidate1)
	pool.Add(candidate2)
	
	// 选择最优（应该选择最近活跃的）
	best = pool.SelectBest()
	if best == nil {
		t.Fatal("expected non-nil best candidate")
	}
	
	// 验证选择了最近活跃的
	if best.PeerID != "peer2" {
		t.Errorf("expected peer2, got %s", best.PeerID)
	}
}

// TestRelayCandidatePool_MaxSize 测试最大容量限制
func TestRelayCandidatePool_MaxSize(t *testing.T) {
	pool := NewRelayCandidatePool("test-realm")
	
	// 添加超过最大容量的候选
	for i := 0; i < 60; i++ {
		candidate := &RelayCandidate{
			PeerID:       string(rune('a' + i)),
			Addrs:        []string{"/ip4/1.2.3.4/tcp/4001"},
			Reachability: nat.ReachabilityPublic,
			LastSeen:     time.Now().Add(time.Duration(i) * time.Second),
		}
		pool.Add(candidate)
	}
	
	// 验证不超过最大容量
	if pool.Count() > 50 {
		t.Errorf("expected count <= 50, got %d", pool.Count())
	}
}

// TestRelayCandidatePool_Clear 测试清空候选池
func TestRelayCandidatePool_Clear(t *testing.T) {
	pool := NewRelayCandidatePool("test-realm")
	
	// 添加候选
	for i := 0; i < 10; i++ {
		candidate := &RelayCandidate{
			PeerID:       string(rune('a' + i)),
			Addrs:        []string{"/ip4/1.2.3.4/tcp/4001"},
			Reachability: nat.ReachabilityPublic,
			LastSeen:     time.Now(),
		}
		pool.Add(candidate)
	}
	
	// 验证添加成功
	if pool.Count() != 10 {
		t.Fatalf("expected count 10, got %d", pool.Count())
	}
	
	// 清空
	pool.Clear()
	
	// 验证清空成功
	if pool.Count() != 0 {
		t.Errorf("expected count 0 after clear, got %d", pool.Count())
	}
}

// TestRelayCandidatePool_GetAll 测试获取所有候选
func TestRelayCandidatePool_GetAll(t *testing.T) {
	pool := NewRelayCandidatePool("test-realm")
	
	// 添加候选
	for i := 0; i < 5; i++ {
		candidate := &RelayCandidate{
			PeerID:       string(rune('a' + i)),
			Addrs:        []string{"/ip4/1.2.3.4/tcp/4001"},
			Reachability: nat.ReachabilityPublic,
			LastSeen:     time.Now(),
		}
		pool.Add(candidate)
	}
	
	// 获取所有候选
	all := pool.GetAll()
	
	// 验证数量
	if len(all) != 5 {
		t.Errorf("expected 5 candidates, got %d", len(all))
	}
	
	// 验证是拷贝（修改不影响原池）
	all[0].PeerID = "modified"
	
	// 再次获取，验证未被修改
	all2 := pool.GetAll()
	if all2[0].PeerID == "modified" {
		t.Error("expected independent copy, but pool was modified")
	}
}

// ============================================================================
// 覆盖率提升测试
// ============================================================================

// TestConfig_Validate 测试配置验证
func TestConfig_Validate(t *testing.T) {
	// 有效配置
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("valid config failed: %v", err)
	}

	// 无效 MaxReservations
	cfg.MaxReservations = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid MaxReservations")
	}
	cfg.MaxReservations = 128

	// MaxCircuits = 0 表示不限制，是合法值
	cfg.MaxCircuits = 0
	if err := cfg.Validate(); err != nil {
		t.Errorf("MaxCircuits = 0 should be valid (means unlimited), got error: %v", err)
	}
	cfg.MaxCircuits = 16

	// 无效 MaxCircuits（负数）
	cfg.MaxCircuits = -1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative MaxCircuits")
	}
	cfg.MaxCircuits = 16

	// 无效 ReservationTTL
	cfg.ReservationTTL = 30 * time.Second
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid ReservationTTL")
	}
	cfg.ReservationTTL = 1 * time.Hour

	// 无效 BufferSize
	cfg.BufferSize = 512
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid BufferSize")
	}
}

// TestRelayCandidatePool_SetMetrics 测试设置度量
// 注意：SetMetrics 需要 *CandidateMetrics 服务，这里跳过复杂的集成测试
func TestRelayCandidatePool_SetMetrics(t *testing.T) {
	pool := NewRelayCandidatePool("test-realm")
	
	// 添加候选
	candidate := &RelayCandidate{
		PeerID:       "peer1",
		Addrs:        []string{"/ip4/1.2.3.4/tcp/4001"},
		Reachability: nat.ReachabilityPublic,
		LastSeen:     time.Now(),
	}
	pool.Add(candidate)
	
	// SetMetrics(nil) 应该安全处理
	pool.SetMetrics(nil)
	
	// 验证候选仍存在
	all := pool.GetAll()
	if len(all) == 0 {
		t.Fatal("expected 1 candidate")
	}
}
