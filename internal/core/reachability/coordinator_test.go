// Package reachability 提供可达性协调模块的实现
package reachability

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

func TestNewCoordinator(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	coordinator := NewCoordinator(config)

	assert.NotNil(t, coordinator)
	assert.NotNil(t, coordinator.verifiedAddrs)
	assert.NotNil(t, coordinator.candidateAddrs)
}

func TestCoordinator_StartStop(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	config.EnableDialBack = false // 禁用以简化测试
	coordinator := NewCoordinator(config)

	ctx := context.Background()
	err := coordinator.Start(ctx)
	require.NoError(t, err)

	err = coordinator.Stop()
	require.NoError(t, err)
}

func TestCoordinator_OnRelayReserved(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	config.EnableDialBack = false
	coordinator := NewCoordinator(config)

	ctx := context.Background()
	_ = coordinator.Start(ctx)
	defer coordinator.Stop()

	// 注册回调
	var callbackAddrs []string
	done := make(chan struct{})
	coordinator.SetOnAddressChanged(func(addrs []string) {
		callbackAddrs = addrs
		close(done)
	})

	// 添加 Relay 地址
	relayAddrs := []string{"/ip4/1.2.3.4/tcp/9000/p2p-circuit"}
	coordinator.OnRelayReserved(relayAddrs)

	// 等待回调
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for callback")
	}

	// 验证
	assert.True(t, coordinator.HasRelayAddress())
	assert.Contains(t, callbackAddrs, relayAddrs[0])
}

func TestCoordinator_OnDirectAddressVerified(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	config.EnableDialBack = false
	coordinator := NewCoordinator(config)

	ctx := context.Background()
	_ = coordinator.Start(ctx)
	defer coordinator.Stop()

	// 验证直连地址
	addr := "/ip4/1.2.3.4/udp/4001/quic-v1"
	coordinator.OnDirectAddressVerified(addr, "test", interfaces.PriorityVerifiedDirect)

	// 验证
	assert.True(t, coordinator.HasVerifiedDirectAddress())
	verified := coordinator.VerifiedDirectAddresses()
	assert.Contains(t, verified, addr)
}

func TestCoordinator_OnDirectAddressCandidate(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	config.EnableDialBack = false
	coordinator := NewCoordinator(config)

	ctx := context.Background()
	_ = coordinator.Start(ctx)
	defer coordinator.Stop()

	// 添加候选地址
	addr := "/ip4/1.2.3.4/udp/4001/quic-v1"
	coordinator.OnDirectAddressCandidate(addr, "test", interfaces.PriorityUnverified)

	// 验证候选地址已记录
	coordinator.candidateAddrsMu.RLock()
	_, exists := coordinator.candidateAddrs[addr]
	coordinator.candidateAddrsMu.RUnlock()

	assert.True(t, exists)
}

func TestCoordinator_AdvertisedAddrs_Priority(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	config.EnableDialBack = false
	coordinator := NewCoordinator(config)

	ctx := context.Background()
	_ = coordinator.Start(ctx)
	defer coordinator.Stop()

	// 添加不同优先级的地址
	directAddr := "/ip4/1.1.1.1/udp/4001/quic-v1"
	relayAddr := "/ip4/2.2.2.2/tcp/9000/p2p-circuit"

	coordinator.OnDirectAddressVerified(directAddr, "test", interfaces.PriorityVerifiedDirect)
	coordinator.OnRelayReserved([]string{relayAddr})

	// 获取通告地址
	addrs := coordinator.AdvertisedAddrs()

	// 验证优先级排序：直连地址应该在前面
	require.Len(t, addrs, 2)
	assert.Equal(t, directAddr, addrs[0])
	assert.Equal(t, relayAddr, addrs[1])
}

func TestCoordinator_BootstrapCandidates(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	config.EnableDialBack = false
	coordinator := NewCoordinator(config)

	ctx := context.Background()
	_ = coordinator.Start(ctx)
	defer coordinator.Stop()

	nodeID := "QmTestNode123"

	// 添加不同类型的地址
	verifiedAddr := "/ip4/1.1.1.1/udp/4001/quic-v1"
	candidateAddr := "/ip4/2.2.2.2/udp/4001/quic-v1"
	relayAddr := "/ip4/3.3.3.3/tcp/9000/p2p-circuit"

	coordinator.OnDirectAddressVerified(verifiedAddr, "test", interfaces.PriorityVerifiedDirect)
	coordinator.OnDirectAddressCandidate(candidateAddr, "stun", interfaces.PriorityUnverified)
	coordinator.OnRelayReserved([]string{relayAddr})

	// 获取候选
	candidates := coordinator.BootstrapCandidates(nodeID)

	// 验证
	assert.Len(t, candidates, 3)

	// 检查已验证地址
	var foundVerified bool
	for _, c := range candidates {
		if c.Verified && c.Kind == interfaces.CandidateKindDirect {
			foundVerified = true
			assert.Equal(t, interfaces.ConfidenceHigh, c.Confidence)
		}
	}
	assert.True(t, foundVerified)
}

func TestCoordinator_OnInboundWitness(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	config.EnableDialBack = false
	config.MinWitnesses = 2
	coordinator := NewCoordinator(config)

	ctx := context.Background()
	_ = coordinator.Start(ctx)
	defer coordinator.Stop()

	// 添加候选地址
	addr := "/ip4/1.2.3.4/udp/4001/quic-v1"
	coordinator.OnDirectAddressCandidate(addr, "test", interfaces.PriorityUnverified)

	// 上报第一个见证（不应升级）
	coordinator.OnInboundWitness(addr, "peer1", "192.168.1.1")
	assert.False(t, coordinator.HasVerifiedDirectAddress())

	// 上报第二个见证（来自不同 IP 前缀，应升级）
	coordinator.OnInboundWitness(addr, "peer2", "10.0.0.1")
	assert.True(t, coordinator.HasVerifiedDirectAddress())
}

func TestCoordinator_OnInboundWitness_SamePrefix(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	config.EnableDialBack = false
	config.MinWitnesses = 2
	coordinator := NewCoordinator(config)

	ctx := context.Background()
	_ = coordinator.Start(ctx)
	defer coordinator.Stop()

	// 添加候选地址
	addr := "/ip4/1.2.3.4/udp/4001/quic-v1"
	coordinator.OnDirectAddressCandidate(addr, "test", interfaces.PriorityUnverified)

	// 上报两个见证（来自相同 /24 前缀，不应升级）
	coordinator.OnInboundWitness(addr, "peer1", "192.168.1.1")
	coordinator.OnInboundWitness(addr, "peer2", "192.168.1.100")
	assert.False(t, coordinator.HasVerifiedDirectAddress())
}

func TestCoordinator_OnDirectAddressExpired(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	config.EnableDialBack = false
	coordinator := NewCoordinator(config)

	ctx := context.Background()
	_ = coordinator.Start(ctx)
	defer coordinator.Stop()

	// 添加验证地址
	addr := "/ip4/1.2.3.4/udp/4001/quic-v1"
	coordinator.OnDirectAddressVerified(addr, "test", interfaces.PriorityVerifiedDirect)
	assert.True(t, coordinator.HasVerifiedDirectAddress())

	// 过期地址
	coordinator.OnDirectAddressExpired(addr)
	assert.False(t, coordinator.HasVerifiedDirectAddress())
}

func TestCoordinator_UpdateDirectCandidates(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	config.EnableDialBack = false
	coordinator := NewCoordinator(config)

	ctx := context.Background()
	_ = coordinator.Start(ctx)
	defer coordinator.Stop()

	// 批量添加候选地址
	candidates := []interfaces.CandidateUpdate{
		{Addr: "/ip4/1.1.1.1/udp/4001/quic-v1", Priority: interfaces.PriorityUnverified},
		{Addr: "/ip4/2.2.2.2/udp/4001/quic-v1", Priority: interfaces.PriorityUnverified},
	}
	coordinator.UpdateDirectCandidates("stun", candidates)

	// 验证候选地址已添加
	coordinator.candidateAddrsMu.RLock()
	count := len(coordinator.candidateAddrs)
	coordinator.candidateAddrsMu.RUnlock()
	assert.Equal(t, 2, count)

	// 更新候选地址（按来源替换）
	newCandidates := []interfaces.CandidateUpdate{
		{Addr: "/ip4/3.3.3.3/udp/4001/quic-v1", Priority: interfaces.PriorityUnverified},
	}
	coordinator.UpdateDirectCandidates("stun", newCandidates)

	// 验证旧地址已移除，新地址已添加
	coordinator.candidateAddrsMu.RLock()
	count = len(coordinator.candidateAddrs)
	_, hasNew := coordinator.candidateAddrs["/ip4/3.3.3.3/udp/4001/quic-v1"]
	_, hasOld := coordinator.candidateAddrs["/ip4/1.1.1.1/udp/4001/quic-v1"]
	coordinator.candidateAddrsMu.RUnlock()

	assert.Equal(t, 1, count)
	assert.True(t, hasNew)
	assert.False(t, hasOld)
}

// ============================================================================
// DirectAddrUpdateStateMachine 集成测试
// ============================================================================

func TestCoordinator_SetStateMachine(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	config.EnableDialBack = false
	coordinator := NewCoordinator(config)

	// 创建状态机
	smConfig := DefaultDirectAddrStateMachineConfig()
	sm := NewDirectAddrUpdateStateMachine(smConfig)

	// 设置状态机
	coordinator.SetStateMachine(sm)

	// 验证状态机已设置
	assert.Equal(t, sm, coordinator.GetStateMachine())
}

func TestCoordinator_SetStateMachine_Nil(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	config.EnableDialBack = false
	coordinator := NewCoordinator(config)

	// 设置 nil 状态机不应报错
	coordinator.SetStateMachine(nil)

	// 验证状态机为 nil
	assert.Nil(t, coordinator.GetStateMachine())
}

func TestCoordinator_TriggerAddressUpdate_NoStateMachine(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	config.EnableDialBack = false
	coordinator := NewCoordinator(config)

	ctx := context.Background()
	_ = coordinator.Start(ctx)
	defer coordinator.Stop()

	// 没有状态机时，TriggerAddressUpdate 应该正常工作
	err := coordinator.TriggerAddressUpdate(ctx)
	assert.NoError(t, err)
}

func TestCoordinator_TriggerAddressUpdate_WithStateMachine(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	config.EnableDialBack = false
	coordinator := NewCoordinator(config)

	ctx := context.Background()
	_ = coordinator.Start(ctx)
	defer coordinator.Stop()

	// 添加一些候选地址
	coordinator.OnDirectAddressCandidate("/ip4/1.1.1.1/udp/4001/quic-v1", "test", interfaces.PriorityUnverified)

	// 创建并设置状态机
	smConfig := DefaultDirectAddrStateMachineConfig()
	sm := NewDirectAddrUpdateStateMachine(smConfig)
	coordinator.SetStateMachine(sm)

	// 触发地址更新
	err := coordinator.TriggerAddressUpdate(ctx)
	assert.NoError(t, err)

	// 等待状态机完成
	time.Sleep(100 * time.Millisecond)

	// 验证状态机状态
	state := sm.State()
	// 状态机应该已经运行过（状态不再是 Idle）或者已完成
	assert.NotEqual(t, StateIdle, state, "状态机应该已被触发")
}
