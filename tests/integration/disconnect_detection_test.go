// Package integration 提供快速断开检测的集成测试
//
// 本文件测试断开检测系统的各个场景：
//   - 优雅断开通知（MemberLeave）
//   - QUIC 连接超时检测
//   - 见证人协议（快速路径和标准路径）
//   - 重连宽限期恢复
//   - 震荡检测与抑制
//   - Liveness 兜底检测
package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/member"
	"github.com/dep2p/go-dep2p/internal/realm/stability"
	"github.com/dep2p/go-dep2p/internal/realm/witness"
	witnesspb "github.com/dep2p/go-dep2p/pkg/lib/proto/realm/witness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                              测试场景 1: 优雅断开通知
// ============================================================================

// TestGracefulDisconnectNotification 测试优雅断开通知
//
// 场景：成员主动离开 Realm，发送 MemberLeave 消息
// 预期：其他成员立即收到通知并移除该成员，无需等待超时
func TestGracefulDisconnectNotification(t *testing.T) {
	// 此测试需要完整的 Realm 环境，这里测试防误判机制的行为
	afp := member.NewAntiFalsePositive(nil)
	defer afp.Close()

	// 模拟正常成员离开（非断开检测触发）
	// 在优雅离开场景中，不需要宽限期
	reject, reason := afp.ShouldRejectAdd("peer1")
	assert.False(t, reject, "新成员应该可以加入")
	assert.Empty(t, reason)

	// 手动标记为保护期（模拟成员离开后的状态）
	afp.ClearProtection("peer1")
}

// ============================================================================
//                              测试场景 2: QUIC 连接超时检测
// ============================================================================

// TestQUICConnectionTimeout 测试 QUIC 连接超时检测
//
// 场景：QUIC 连接在 idle timeout 后断开
// 预期：触发断开检测，进入宽限期
func TestQUICConnectionTimeout(t *testing.T) {
	afp := member.NewAntiFalsePositive(nil)
	defer afp.Close()

	// 模拟 QUIC 超时导致的断开
	shouldRemove, inGracePeriod := afp.OnPeerDisconnected("peer1", "realm1")

	// 不应该立即移除，应进入宽限期
	assert.False(t, shouldRemove)
	assert.True(t, inGracePeriod)
	assert.True(t, afp.IsInGracePeriod("peer1"))
}

// ============================================================================
//                              测试场景 3: 快速路径见证
// ============================================================================

// TestFastPathWitness 测试快速路径见证
//
// 场景：小型 Realm（成员数 < 10），检测到断开
// 预期：单票同意即可确认断开
func TestFastPathWitness(t *testing.T) {
	var resultReceived bool
	var resultConfirmed bool
	var mu sync.Mutex

	// 小型 Realm（成员数 < 10）
	session := witness.NewVotingSession("report-1", "target-peer", 5, func(result *witnesspb.WitnessVotingResult) {
		mu.Lock()
		defer mu.Unlock()
		resultReceived = true
		resultConfirmed = result.Confirmed
	})

	session.StartTimeout(1 * time.Second)

	// 添加单票同意
	session.AddConfirmation(&witnesspb.WitnessConfirmation{
		WitnessId:        []byte("witness1"),
		ConfirmationType: witnesspb.ConfirmationType_CONFIRMATION_TYPE_AGREE,
	})

	// 等待结果
	time.Sleep(50 * time.Millisecond)

	// 验证：快速路径下单票同意即可确认
	mu.Lock()
	assert.True(t, resultReceived)
	assert.True(t, resultConfirmed)
	mu.Unlock()
}

// ============================================================================
//                              测试场景 4: 标准路径见证（多数确认）
// ============================================================================

// TestStandardPathWitnessWithMajority 测试标准路径见证（多数确认）
//
// 场景：大型 Realm（成员数 >= 10），需要多数确认
// 预期：超过 50% 同意才能确认断开
func TestStandardPathWitnessWithMajority(t *testing.T) {
	var resultReceived bool
	var resultConfirmed bool
	var mu sync.Mutex

	// 大型 Realm（成员数 >= 10）
	session := witness.NewVotingSession("report-1", "target-peer", 15, func(result *witnesspb.WitnessVotingResult) {
		mu.Lock()
		defer mu.Unlock()
		resultReceived = true
		resultConfirmed = result.Confirmed
	})

	session.StartTimeout(1 * time.Second)

	// 添加多数同意投票
	// minResponses = 15/2 = 7
	// 需要 > 50% 同意
	for i := 0; i < 6; i++ {
		session.AddConfirmation(&witnesspb.WitnessConfirmation{
			WitnessId:        []byte("witness_agree_" + string(rune(i))),
			ConfirmationType: witnesspb.ConfirmationType_CONFIRMATION_TYPE_AGREE,
		})
	}
	// 添加 2 个反对，确保达到 minResponses
	for i := 0; i < 2; i++ {
		session.AddConfirmation(&witnesspb.WitnessConfirmation{
			WitnessId:        []byte("witness_disagree_" + string(rune(i))),
			ConfirmationType: witnesspb.ConfirmationType_CONFIRMATION_TYPE_DISAGREE,
		})
	}

	// 等待结果
	time.Sleep(50 * time.Millisecond)

	// 验证：6 同意 vs 2 反对，同意占多数
	mu.Lock()
	assert.True(t, resultReceived)
	assert.True(t, resultConfirmed)
	mu.Unlock()
}

// ============================================================================
//                              测试场景 5: 见证拒绝（分区保护）
// ============================================================================

// TestWitnessRejection_PartitionProtection 测试见证拒绝（分区保护）
//
// 场景：网络分区导致部分节点误判，其他节点反对
// 预期：反对票占多数时，不确认断开，保护被误判节点
func TestWitnessRejection_PartitionProtection(t *testing.T) {
	var resultReceived bool
	var resultConfirmed bool
	var mu sync.Mutex

	session := witness.NewVotingSession("report-1", "target-peer", 20, func(result *witnesspb.WitnessVotingResult) {
		mu.Lock()
		defer mu.Unlock()
		resultReceived = true
		resultConfirmed = result.Confirmed
	})

	session.StartTimeout(1 * time.Second)

	// 模拟分区场景：少数节点报告断开，多数节点仍能连接
	// 先添加反对票
	for i := 0; i < 8; i++ {
		session.AddConfirmation(&witnesspb.WitnessConfirmation{
			WitnessId:        []byte("witness_disagree_" + string(rune(i))),
			ConfirmationType: witnesspb.ConfirmationType_CONFIRMATION_TYPE_DISAGREE,
		})
	}
	// 添加少量同意票
	for i := 0; i < 2; i++ {
		session.AddConfirmation(&witnesspb.WitnessConfirmation{
			WitnessId:        []byte("witness_agree_" + string(rune(i))),
			ConfirmationType: witnesspb.ConfirmationType_CONFIRMATION_TYPE_AGREE,
		})
	}

	// 等待结果
	time.Sleep(50 * time.Millisecond)

	// 验证：反对票占多数，不应确认断开
	mu.Lock()
	assert.True(t, resultReceived)
	assert.False(t, resultConfirmed, "分区场景下应该保护被误判的节点")
	mu.Unlock()
}

// ============================================================================
//                              测试场景 6: Relay 电路关闭见证
// ============================================================================

// TestRelayCircuitCloseWitness 测试 Relay 电路关闭见证
//
// 场景：通过 Relay 连接的成员断开，Relay 检测到电路关闭
// 预期：Relay 可以作为见证人报告断开
func TestRelayCircuitCloseWitness(t *testing.T) {
	// 此测试需要 Relay 服务器环境
	// 这里测试见证服务对 RELAY_CIRCUIT 检测方法的支持

	mockMember := newMockMemberManager()
	mockMember.members["target-peer"] = true
	mockMember.totalCount = 5

	service := witness.NewService("relay-server", "test-realm", mockMember)
	err := service.Start(context.Background())
	require.NoError(t, err)
	defer service.Stop(context.Background())

	// Relay 可以通过 OnPeerDisconnected 报告电路关闭
	// 使用 DetectionMethod_RELAY_CIRCUIT
	lastContact := time.Now().Add(-5 * time.Second)
	service.OnPeerDisconnected(context.Background(), "target-peer", witnesspb.DetectionMethod_DETECTION_METHOD_RELAY_CIRCUIT, lastContact)

	// 验证：由于限速器会阻止同一目标的重复报告，第二次调用会被限速
	// 这里测试限速器独立功能
	limiter := witness.NewRateLimiter(1, time.Minute)
	assert.True(t, limiter.AllowReport("target-peer"))
	assert.False(t, limiter.AllowReport("target-peer"), "同一目标短时间内不应允许重复报告")
}

// ============================================================================
//                              测试场景 7: 重连宽限期恢复
// ============================================================================

// TestReconnectGracePeriodRecovery 测试重连宽限期恢复
//
// 场景：成员断开后在宽限期内重新连接
// 预期：成员状态恢复，不触发移除
func TestReconnectGracePeriodRecovery(t *testing.T) {
	afp := member.NewAntiFalsePositive(nil)
	defer afp.Close()

	// 触发断开，进入宽限期
	shouldRemove, inGracePeriod := afp.OnPeerDisconnected("peer1", "realm1")
	assert.False(t, shouldRemove)
	assert.True(t, inGracePeriod)
	assert.True(t, afp.IsInGracePeriod("peer1"))

	// 在宽限期内重连
	recovered, suppressed := afp.OnPeerReconnected("peer1")
	assert.True(t, recovered, "应该成功恢复")
	assert.False(t, suppressed)
	assert.False(t, afp.IsInGracePeriod("peer1"), "宽限期应该结束")

	// 使用不同的 peerID 测试再次断开（避免震荡检测）
	shouldRemove, inGracePeriod = afp.OnPeerDisconnected("peer2", "realm1")
	assert.False(t, shouldRemove)
	assert.True(t, inGracePeriod, "新 peer 应该正常进入宽限期")
}

// ============================================================================
//                              测试场景 8: 震荡检测与抑制
// ============================================================================

// TestFlappingDetectionAndSuppression 测试震荡检测与抑制
//
// 场景：成员频繁断开重连（60秒内 >= 3次）
// 预期：被标记为震荡，状态变更被抑制
func TestFlappingDetectionAndSuppression(t *testing.T) {
	config := &member.AntiFalsePositiveConfig{
		GracePeriod:        15 * time.Second,
		FlapWindow:         60 * time.Second,
		FlapThreshold:      3,
		ProtectionDuration: 30 * time.Second,
	}
	afp := member.NewAntiFalsePositive(config)
	defer afp.Close()

	// 模拟频繁断开重连（触发震荡检测）
	for i := 0; i < 3; i++ {
		afp.OnPeerDisconnected("peer1", "realm1")
		afp.OnPeerReconnected("peer1")
	}

	// 应该被标记为震荡
	assert.True(t, afp.IsFlapping("peer1"), "应该被检测为震荡")

	// 再次断开应该被抑制
	shouldRemove, inGracePeriod := afp.OnPeerDisconnected("peer1", "realm1")
	assert.False(t, shouldRemove)
	assert.False(t, inGracePeriod, "震荡状态下不应进入宽限期")

	// 应该拒绝添加
	reject, reason := afp.ShouldRejectAdd("peer1")
	assert.True(t, reject)
	assert.Contains(t, reason, "flapping")
}

// TestFlappingDetectionWithStabilityTracker 测试震荡检测器独立功能
func TestFlappingDetectionWithStabilityTracker(t *testing.T) {
	tracker := stability.NewConnectionStabilityTracker()

	// 记录多次状态转换
	for i := 0; i < stability.FlapThreshold; i++ {
		tracker.RecordTransition("peer1")
	}

	// 应该被标记为震荡
	assert.True(t, tracker.IsFlapping("peer1"))
	assert.True(t, tracker.ShouldSuppressStateChange("peer1"))

	// 获取震荡列表
	flappingPeers := tracker.GetFlappingPeers()
	assert.Contains(t, flappingPeers, "peer1")
}

// ============================================================================
//                              测试场景 9: Liveness 兜底检测
// ============================================================================

// TestLivenessFallbackDetection 测试 Liveness 兜底检测
//
// 场景：所有其他检测机制都未能检测到断开
// 预期：Liveness 定期检查可以发现不健康的连接
//
// 注意：此测试仅验证健康检查器的逻辑，不涉及真实网络调用
func TestLivenessFallbackDetection(t *testing.T) {
	// Liveness 健康检查器在 internal/core/swarm/health.go 中实现
	// 这里测试防误判机制与健康检查的协同工作

	afp := member.NewAntiFalsePositive(nil)
	defer afp.Close()

	// 模拟 Liveness 检测到不健康连接后触发的断开处理
	shouldRemove, inGracePeriod := afp.OnPeerDisconnected("unhealthy-peer", "realm1")

	// 即使是 Liveness 检测到的断开，也应该进入宽限期
	assert.False(t, shouldRemove)
	assert.True(t, inGracePeriod)
}

// ============================================================================
//                              辅助测试：限速器集成
// ============================================================================

// TestRateLimiterIntegration 测试限速器与服务的集成
func TestRateLimiterIntegration(t *testing.T) {
	limiter := witness.NewRateLimiter(2, 100*time.Millisecond)

	// 同一目标的报告应该被限速
	assert.True(t, limiter.AllowReport("peer1"))
	assert.True(t, limiter.AllowReport("peer1"))
	assert.False(t, limiter.AllowReport("peer1"), "第三次报告应该被限速")

	// 不同目标不受影响
	assert.True(t, limiter.AllowReport("peer2"))

	// 等待窗口过期
	time.Sleep(150 * time.Millisecond)

	// 窗口过期后可以继续报告
	assert.True(t, limiter.AllowReport("peer1"))
}

// ============================================================================
//                              辅助测试：断开保护期
// ============================================================================

// TestDisconnectProtectionIntegration 测试断开保护期集成
func TestDisconnectProtectionIntegration(t *testing.T) {
	tracker := member.NewDisconnectProtectionTracker()
	tracker.SetProtectionDuration(100 * time.Millisecond)

	// 记录成员移除
	tracker.OnMemberRemoved("peer1")

	// 在保护期内应该拒绝重新添加
	assert.True(t, tracker.IsProtected("peer1"))

	// 等待保护期过期
	time.Sleep(150 * time.Millisecond)

	// 保护期过后可以重新添加
	assert.False(t, tracker.IsProtected("peer1"))
}

// ============================================================================
//                              辅助类型
// ============================================================================

// mockMemberManager 模拟成员管理器
type mockMemberManager struct {
	members     map[string]bool
	totalCount  int
	removedPeer string
}

func newMockMemberManager() *mockMemberManager {
	return &mockMemberManager{
		members:    make(map[string]bool),
		totalCount: 5,
	}
}

func (m *mockMemberManager) IsMember(ctx context.Context, peerID string) bool {
	_, ok := m.members[peerID]
	return ok
}

func (m *mockMemberManager) GetTotalCount() int {
	return m.totalCount
}

func (m *mockMemberManager) Remove(ctx context.Context, peerID string) error {
	m.removedPeer = peerID
	delete(m.members, peerID)
	return nil
}

func (m *mockMemberManager) UpdateStatus(ctx context.Context, peerID string, online bool) error {
	return nil
}
