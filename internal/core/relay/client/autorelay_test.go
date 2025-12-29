// Package client AutoRelay 测试
package client

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	endpointif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	relayif "github.com/dep2p/go-dep2p/pkg/interfaces/relay"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Config 测试
// ============================================================================

func TestDefaultAutoRelayConfig(t *testing.T) {
	config := DefaultAutoRelayConfig()

	assert.Equal(t, DefaultMinRelays, config.MinRelays)
	assert.Equal(t, DefaultMaxRelays, config.MaxRelays)
	assert.Equal(t, DefaultRefreshInterval, config.RefreshInterval)
	assert.Equal(t, DefaultDiscoveryInterval, config.DiscoveryInterval)
	assert.True(t, config.EnableBackoff)
}

// ============================================================================
//                              AutoRelay 创建测试
// ============================================================================

func TestNewAutoRelay(t *testing.T) {
	t.Run("默认创建", func(t *testing.T) {
		config := DefaultAutoRelayConfig()
		ar := NewAutoRelay(config, nil, nil)

		require.NotNil(t, ar)
		assert.NotNil(t, ar.activeRelays)
		assert.NotNil(t, ar.candidates)
		assert.NotNil(t, ar.blacklist)
	})

	t.Run("带静态中继", func(t *testing.T) {
		config := DefaultAutoRelayConfig()
		config.StaticRelays = []types.NodeID{{1}, {2}, {3}}
		ar := NewAutoRelay(config, nil, nil)

		require.NotNil(t, ar)
		assert.Equal(t, 3, len(config.StaticRelays))
	})
}

// ============================================================================
//                              生命周期测试
// ============================================================================

func TestAutoRelay_StartStop(t *testing.T) {
	t.Run("正常启动停止", func(t *testing.T) {
		config := DefaultAutoRelayConfig()
		ar := NewAutoRelay(config, nil, nil)

		ctx := context.Background()
		err := ar.Start(ctx)
		require.NoError(t, err)
		assert.Equal(t, int32(1), atomic.LoadInt32(&ar.running))

		err = ar.Stop()
		require.NoError(t, err)
		assert.Equal(t, int32(0), atomic.LoadInt32(&ar.running))
	})

	t.Run("重复启动", func(t *testing.T) {
		config := DefaultAutoRelayConfig()
		ar := NewAutoRelay(config, nil, nil)

		ctx := context.Background()
		ar.Start(ctx)
		err := ar.Start(ctx) // 重复启动
		require.NoError(t, err)

		ar.Stop()
	})

	t.Run("重复停止", func(t *testing.T) {
		config := DefaultAutoRelayConfig()
		ar := NewAutoRelay(config, nil, nil)

		ctx := context.Background()
		ar.Start(ctx)
		ar.Stop()
		err := ar.Stop() // 重复停止
		require.NoError(t, err)
	})

	t.Run("静态中继初始化", func(t *testing.T) {
		config := DefaultAutoRelayConfig()
		config.StaticRelays = []types.NodeID{{1}, {2}}
		ar := NewAutoRelay(config, nil, nil)

		ctx := context.Background()
		ar.Start(ctx)
		defer ar.Stop()

		// 验证静态中继被添加为候选
		ar.candidatesMu.RLock()
		candidateCount := len(ar.candidates)
		ar.candidatesMu.RUnlock()

		assert.Equal(t, 2, candidateCount)
	})
}

// ============================================================================
//                              启用/禁用测试
// ============================================================================

func TestAutoRelay_EnableDisable(t *testing.T) {
	t.Run("启用禁用切换", func(t *testing.T) {
		config := DefaultAutoRelayConfig()
		ar := NewAutoRelay(config, nil, nil)
		ar.Start(context.Background())
		defer ar.Stop()

		assert.False(t, ar.IsEnabled())

		ar.Enable()
		assert.True(t, ar.IsEnabled())

		ar.Disable()
		assert.False(t, ar.IsEnabled())
	})

	t.Run("多次启用", func(t *testing.T) {
		config := DefaultAutoRelayConfig()
		ar := NewAutoRelay(config, nil, nil)
		ar.Start(context.Background())
		defer ar.Stop()

		ar.Enable()
		ar.Enable()
		ar.Enable()

		assert.True(t, ar.IsEnabled())
	})
}

// ============================================================================
//                              查询接口测试
// ============================================================================

func TestAutoRelay_Relays(t *testing.T) {
	config := DefaultAutoRelayConfig()
	ar := NewAutoRelay(config, nil, nil)

	// 初始状态无中继
	relays := ar.Relays()
	assert.Empty(t, relays)

	// 手动添加活跃中继（模拟）
	ar.activeRelaysMu.Lock()
	ar.activeRelays[types.NodeID{1}] = &activeRelay{nodeID: types.NodeID{1}}
	ar.activeRelays[types.NodeID{2}] = &activeRelay{nodeID: types.NodeID{2}}
	ar.activeRelaysMu.Unlock()

	relays = ar.Relays()
	assert.Len(t, relays, 2)
}

func TestAutoRelay_Status(t *testing.T) {
	config := DefaultAutoRelayConfig()
	ar := NewAutoRelay(config, nil, nil)

	// 初始状态
	status := ar.Status()
	assert.False(t, status.Enabled)
	assert.Equal(t, 0, status.NumRelays)
	assert.Empty(t, status.RelayAddrs)

	// 启用后
	ar.Enable()
	status = ar.Status()
	assert.True(t, status.Enabled)
}

func TestAutoRelay_RelayAddrs(t *testing.T) {
	config := DefaultAutoRelayConfig()
	ar := NewAutoRelay(config, nil, nil)

	// 初始状态无地址
	addrs := ar.RelayAddrs()
	assert.Empty(t, addrs)
}

func TestAutoRelay_BlacklistTTL_InferredVsFound(t *testing.T) {
	ar := NewAutoRelay(DefaultAutoRelayConfig(), nil, nil)

	inferred := types.NodeID{1}
	found := types.NodeID{2}

	ar.candidatesMu.Lock()
	ar.candidates[inferred] = &relayCandidate{nodeID: inferred, priority: inferredRelayPriority}
	ar.candidates[found] = &relayCandidate{nodeID: found, priority: 0}
	ar.candidatesMu.Unlock()

	before := time.Now()
	ar.addToBlacklist(inferred)
	ar.addToBlacklist(found)

	ar.blacklistMu.RLock()
	inferredExp := ar.blacklist[inferred]
	foundExp := ar.blacklist[found]
	ar.blacklistMu.RUnlock()

	// 推断候选失败：应更长时间黑名单（约 1h）
	assert.GreaterOrEqual(t, inferredExp.Sub(before), 50*time.Minute)
	// 明确候选失败：短黑名单（约 5min）
	assert.GreaterOrEqual(t, foundExp.Sub(before), 4*time.Minute)
	assert.LessOrEqual(t, foundExp.Sub(before), 10*time.Minute)
}

// ============================================================================
//                              候选管理测试
// ============================================================================

func TestAutoRelay_AddRemoveCandidate(t *testing.T) {
	config := DefaultAutoRelayConfig()
	ar := NewAutoRelay(config, nil, nil)

	relayID := types.NodeID{1, 2, 3}

	// 添加候选
	ar.AddCandidate(relayID, nil, 50)

	ar.candidatesMu.RLock()
	candidate, exists := ar.candidates[relayID]
	ar.candidatesMu.RUnlock()

	assert.True(t, exists)
	assert.Equal(t, 50, candidate.priority)

	// 移除候选
	ar.RemoveCandidate(relayID)

	ar.candidatesMu.RLock()
	_, exists = ar.candidates[relayID]
	ar.candidatesMu.RUnlock()

	assert.False(t, exists)
}

func TestAutoRelay_GetCandidates(t *testing.T) {
	config := DefaultAutoRelayConfig()
	ar := NewAutoRelay(config, nil, nil)

	// 添加多个候选
	for i := 0; i < 10; i++ {
		relayID := types.NodeID{byte(i)}
		ar.AddCandidate(relayID, nil, i*10)
	}

	// 获取前 5 个
	candidates := ar.getCandidates(5)
	assert.Len(t, candidates, 5)

	// 验证排序（高优先级在前）
	for i := 0; i < len(candidates)-1; i++ {
		assert.GreaterOrEqual(t, candidates[i].priority, candidates[i+1].priority)
	}
}

func TestAutoRelay_GetCandidates_LessThanCount(t *testing.T) {
	config := DefaultAutoRelayConfig()
	ar := NewAutoRelay(config, nil, nil)

	// 只添加 3 个候选
	for i := 0; i < 3; i++ {
		ar.AddCandidate(types.NodeID{byte(i)}, nil, 0)
	}

	// 请求 10 个
	candidates := ar.getCandidates(10)
	assert.Len(t, candidates, 3)
}

// ============================================================================
//                              黑名单测试
// ============================================================================

func TestAutoRelay_Blacklist(t *testing.T) {
	config := DefaultAutoRelayConfig()
	ar := NewAutoRelay(config, nil, nil)

	relayID := types.NodeID{1, 2, 3}

	// 初始不在黑名单
	assert.False(t, ar.isBlacklisted(relayID))

	// 添加到黑名单
	ar.addToBlacklist(relayID)
	assert.True(t, ar.isBlacklisted(relayID))

	// 验证黑名单记录存在
	ar.blacklistMu.RLock()
	_, exists := ar.blacklist[relayID]
	ar.blacklistMu.RUnlock()
	assert.True(t, exists)
}

func TestAutoRelay_CleanupBlacklist(t *testing.T) {
	config := DefaultAutoRelayConfig()
	ar := NewAutoRelay(config, nil, nil)

	relayID := types.NodeID{1}

	// 添加已过期的黑名单条目
	ar.blacklistMu.Lock()
	ar.blacklist[relayID] = time.Now().Add(-time.Hour) // 已过期
	ar.blacklistMu.Unlock()

	// 清理
	ar.cleanupBlacklist()

	// 验证已清理
	ar.blacklistMu.RLock()
	_, exists := ar.blacklist[relayID]
	ar.blacklistMu.RUnlock()
	assert.False(t, exists)
}

// ============================================================================
//                              升级器测试
// ============================================================================

func TestAutoRelay_SetUpgrader(t *testing.T) {
	config := DefaultAutoRelayConfig()
	ar := NewAutoRelay(config, nil, nil)

	upgraderConfig := DefaultUpgraderConfig()
	upgrader := NewConnectionUpgrader(upgraderConfig, nil, nil, nil)

	ar.SetUpgrader(upgrader)

	assert.Equal(t, upgrader, ar.upgrader)
}

func TestAutoRelay_TryUpgradeConnection_NoUpgrader(t *testing.T) {
	config := DefaultAutoRelayConfig()
	ar := NewAutoRelay(config, nil, nil)

	ctx := context.Background()
	_, err := ar.TryUpgradeConnection(ctx, types.NodeID{1}, nil)

	assert.ErrorIs(t, err, ErrNoPuncher)
}

// ============================================================================
//                              并发测试
// ============================================================================

func TestAutoRelay_ConcurrentCandidateOps(t *testing.T) {
	config := DefaultAutoRelayConfig()
	ar := NewAutoRelay(config, nil, nil)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			ar.AddCandidate(types.NodeID{byte(i)}, nil, i)
		}
		close(done)
	}()

	go func() {
		for {
			select {
			case <-done:
				return
			default:
				ar.getCandidates(10)
			}
		}
	}()

	<-done
	// 验证无 panic
}

func TestAutoRelay_ConcurrentBlacklistOps(t *testing.T) {
	config := DefaultAutoRelayConfig()
	ar := NewAutoRelay(config, nil, nil)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			ar.addToBlacklist(types.NodeID{byte(i)})
		}
		close(done)
	}()

	go func() {
		for {
			select {
			case <-done:
				return
			default:
				ar.isBlacklisted(types.NodeID{50})
				ar.cleanupBlacklist()
			}
		}
	}()

	<-done
	// 验证无 panic
}

// ============================================================================
//                              Mock 实现
// ============================================================================

type mockRelayClient struct {
	reserveErr       error
	findRelaysResult []types.NodeID
}

func (m *mockRelayClient) Connect(ctx context.Context, relayID, targetID types.NodeID) (transportif.Conn, error) {
	return nil, nil
}

func (m *mockRelayClient) ConnectWithProtocol(ctx context.Context, relayID, targetID types.NodeID, protocol types.ProtocolID) (transportif.Conn, error) {
	return nil, nil
}

func (m *mockRelayClient) Reserve(ctx context.Context, relayID types.NodeID) (relayif.Reservation, error) {
	if m.reserveErr != nil {
		return nil, m.reserveErr
	}
	return &mockReservation{}, nil
}

func (m *mockRelayClient) FindRelays(ctx context.Context) ([]types.NodeID, error) {
	return m.findRelaysResult, nil
}

func (m *mockRelayClient) AddRelay(relayID types.NodeID, addrs []endpointif.Address) {}
func (m *mockRelayClient) RemoveRelay(relayID types.NodeID)                          {}
func (m *mockRelayClient) HasRelay(relayID types.NodeID) bool                        { return false }
func (m *mockRelayClient) Relays() []types.NodeID                                    { return nil }
func (m *mockRelayClient) Close() error                                              { return nil }

type mockReservation struct{}

func (m *mockReservation) Relay() types.NodeID               { return types.NodeID{} }
func (m *mockReservation) Expiry() time.Time                 { return time.Now().Add(time.Hour) }
func (m *mockReservation) Addrs() []endpointif.Address       { return nil }
func (m *mockReservation) Refresh(ctx context.Context) error { return nil }
func (m *mockReservation) Cancel() error                     { return nil }

// 确保实现接口
var _ relayif.RelayClient = (*mockRelayClient)(nil)
var _ relayif.Reservation = (*mockReservation)(nil)

// ============================================================================
//                              上下文安全测试
// ============================================================================

// TestAutoRelay_MethodsWithoutStart 测试在未启动时调用方法的安全性
func TestAutoRelay_MethodsWithoutStart(t *testing.T) {
	t.Run("tryRelay_without_start", func(t *testing.T) {
		config := DefaultAutoRelayConfig()
		ar := NewAutoRelay(config, nil, nil)

		// 未调用 Start() 时，ctx 为 nil
		// tryRelay 应该安全返回 false
		result := ar.tryRelay(types.NodeID{1})
		assert.False(t, result)
	})

	t.Run("refreshReservations_without_start", func(t *testing.T) {
		config := DefaultAutoRelayConfig()
		ar := NewAutoRelay(config, nil, nil)

		// 不应 panic
		ar.refreshReservations()
	})

	t.Run("discoverRelays_without_start", func(t *testing.T) {
		config := DefaultAutoRelayConfig()
		client := &mockRelayClient{findRelaysResult: []types.NodeID{{1}, {2}}}
		ar := NewAutoRelay(config, client, nil)

		// 不应 panic，应提前返回
		ar.discoverRelays()
	})

	t.Run("StartAutoUpgrade_without_start", func(t *testing.T) {
		config := DefaultAutoRelayConfig()
		ar := NewAutoRelay(config, nil, nil)

		upgraderConfig := DefaultUpgraderConfig()
		upgrader := NewConnectionUpgrader(upgraderConfig, nil, nil, nil)
		ar.SetUpgrader(upgrader)

		// 不应 panic
		ar.StartAutoUpgrade(types.NodeID{1}, nil)
	})
}

// TestAutoRelay_StaticRelayConcurrency 测试 Start() 中静态中继初始化的并发安全性
func TestAutoRelay_StaticRelayConcurrency(t *testing.T) {
	config := DefaultAutoRelayConfig()
	config.StaticRelays = []types.NodeID{{1}, {2}, {3}}
	ar := NewAutoRelay(config, nil, nil)

	// 并发启动和访问候选列表
	done := make(chan struct{})

	go func() {
		ar.Start(context.Background())
		close(done)
	}()

	// 同时尝试访问候选列表
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				ar.getCandidates(10)
			}
		}
	}()

	<-done
	ar.Stop()
	// 无 panic 表示测试通过
}

// TestAutoRelay_StopWithoutStart 测试未启动时调用 Stop 的安全性
func TestAutoRelay_StopWithoutStart(t *testing.T) {
	config := DefaultAutoRelayConfig()
	ar := NewAutoRelay(config, nil, nil)

	// 不应 panic
	err := ar.Stop()
	assert.NoError(t, err)
}
