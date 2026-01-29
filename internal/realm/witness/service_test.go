package witness

import (
	"context"
	"sync"
	"testing"
	"time"

	witnesspb "github.com/dep2p/go-dep2p/pkg/lib/proto/realm/witness"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

// TestWitnessService_New 测试创建见证服务
func TestWitnessService_New(t *testing.T) {
	member := newMockMemberManager()
	service := NewService("self-peer", "test-realm", member)

	assert.NotNil(t, service)
	assert.Equal(t, "test-realm", service.realmID)
	assert.Equal(t, "self-peer", service.localID)
}

// TestWitnessService_RateLimiter 测试限速器
func TestWitnessService_RateLimiter(t *testing.T) {
	member := newMockMemberManager()
	service := NewService("self-peer", "test-realm", member)

	err := service.Start(context.Background())
	require.NoError(t, err)
	defer service.Stop(context.Background())

	// 第一次报告应该被允许
	allowed := service.rateLimiter.AllowReport("target-peer")
	assert.True(t, allowed)
}

// TestWitnessService_StartStop 测试启动和停止
func TestWitnessService_StartStop(t *testing.T) {
	member := newMockMemberManager()
	service := NewService("self-peer", "test-realm", member)

	// 启动
	err := service.Start(context.Background())
	assert.NoError(t, err)

	// 验证已启动
	assert.NotNil(t, service.ctx)

	// 停止
	err = service.Stop(context.Background())
	assert.NoError(t, err)

	// 验证已停止
	select {
	case <-service.ctx.Done():
		// 正确
	default:
		t.Error("context 应该已取消")
	}
}

// TestRateLimiter_AllowReport 测试限速器允许报告
func TestRateLimiter_AllowReport(t *testing.T) {
	limiter := NewRateLimiter(2, 100*time.Millisecond)

	// 第一次：允许
	assert.True(t, limiter.AllowReport("peer1"))

	// 第二次：允许
	assert.True(t, limiter.AllowReport("peer1"))

	// 第三次：超过限制
	assert.False(t, limiter.AllowReport("peer1"))

	// 不同 peer：允许
	assert.True(t, limiter.AllowReport("peer2"))

	// 等待窗口过期
	time.Sleep(150 * time.Millisecond)

	// 窗口过期后：允许
	assert.True(t, limiter.AllowReport("peer1"))
}

// TestRateLimiter_Cleanup 测试限速器清理
func TestRateLimiter_Cleanup(t *testing.T) {
	limiter := NewRateLimiter(1, 50*time.Millisecond)

	// 记录报告
	limiter.AllowReport("peer1")
	limiter.AllowReport("peer2")

	// 等待过期
	time.Sleep(100 * time.Millisecond)

	// 清理
	limiter.Cleanup()

	// 验证已清理
	limiter.mu.Lock()
	assert.Empty(t, limiter.reports)
	limiter.mu.Unlock()
}

// TestVotingSession_New 测试创建投票会话
func TestVotingSession_New(t *testing.T) {
	session := NewVotingSession("report-1", "target-peer", 5, nil)

	assert.NotNil(t, session)
	assert.Equal(t, "report-1", session.reportID)
	assert.Equal(t, "target-peer", session.targetID)
	assert.Equal(t, 5, session.memberCount)
}

// TestVotingSession_AddConfirmation 测试添加确认
func TestVotingSession_AddConfirmation(t *testing.T) {
	var resultReceived bool
	var resultConfirmed bool
	var mu sync.Mutex

	session := NewVotingSession("report-1", "target-peer", 3, func(result *witnesspb.WitnessVotingResult) {
		mu.Lock()
		defer mu.Unlock()
		resultReceived = true
		resultConfirmed = result.Confirmed
	})

	session.StartTimeout(1 * time.Second)

	// 添加同意确认
	session.AddConfirmation(&witnesspb.WitnessConfirmation{
		WitnessId:        []byte("witness1"),
		ConfirmationType: witnesspb.ConfirmationType_CONFIRMATION_TYPE_AGREE,
	})
	session.AddConfirmation(&witnesspb.WitnessConfirmation{
		WitnessId:        []byte("witness2"),
		ConfirmationType: witnesspb.ConfirmationType_CONFIRMATION_TYPE_AGREE,
	})

	// 等待结果
	time.Sleep(50 * time.Millisecond)

	// 验证（2/3 同意 = 多数）
	mu.Lock()
	assert.True(t, resultReceived)
	assert.True(t, resultConfirmed)
	mu.Unlock()
}

// TestVotingSession_Reject 测试拒绝确认
func TestVotingSession_Reject(t *testing.T) {
	var resultReceived bool
	var resultConfirmed bool
	var mu sync.Mutex

	// 使用大于 10 的 memberCount 来走标准路径
	// 标准路径需要简单多数（>50%）才能确认
	session := NewVotingSession("report-1", "target-peer", 20, func(result *witnesspb.WitnessVotingResult) {
		mu.Lock()
		defer mu.Unlock()
		resultReceived = true
		resultConfirmed = result.Confirmed
	})

	session.StartTimeout(1 * time.Second)

	// 先添加反对票，确保在达到 minResponses 时反对票占多数
	// minResponses = 20/2 = 10
	// 先添加 7 个反对
	for i := 0; i < 7; i++ {
		session.AddConfirmation(&witnesspb.WitnessConfirmation{
			WitnessId:        []byte("witness_disagree_" + string(rune(i))),
			ConfirmationType: witnesspb.ConfirmationType_CONFIRMATION_TYPE_DISAGREE,
		})
	}
	// 再添加 3 个同意（10 票时：3 同意 vs 7 反对）
	for i := 0; i < 3; i++ {
		session.AddConfirmation(&witnesspb.WitnessConfirmation{
			WitnessId:        []byte("witness_agree_" + string(rune(i))),
			ConfirmationType: witnesspb.ConfirmationType_CONFIRMATION_TYPE_AGREE,
		})
	}

	// 等待结果
	time.Sleep(50 * time.Millisecond)

	// 验证（3 同意 vs 7 反对，反对占多数，不应确认）
	// effectiveVotes = 10, agreeCount(3) > 5 = false
	// disagreeCount(7) > 5 = true，所以会 finalize 且 confirmed = false
	mu.Lock()
	assert.True(t, resultReceived)
	assert.False(t, resultConfirmed)
	mu.Unlock()
}

// TestVotingSession_Timeout 测试超时
func TestVotingSession_Timeout(t *testing.T) {
	var resultReceived bool
	var mu sync.Mutex

	session := NewVotingSession("report-1", "target-peer", 5, func(result *witnesspb.WitnessVotingResult) {
		mu.Lock()
		defer mu.Unlock()
		resultReceived = true
	})

	session.StartTimeout(50 * time.Millisecond)

	// 不添加任何确认，等待超时
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.True(t, resultReceived, "超时应该触发回调")
	mu.Unlock()
}

// TestVotingSession_DuplicateVote 测试重复投票
func TestVotingSession_DuplicateVote(t *testing.T) {
	session := NewVotingSession("report-1", "target-peer", 5, nil)

	// 第一次投票
	session.AddConfirmation(&witnesspb.WitnessConfirmation{
		WitnessId:        []byte("witness1"),
		ConfirmationType: witnesspb.ConfirmationType_CONFIRMATION_TYPE_AGREE,
	})

	// 重复投票（应该被忽略）
	session.AddConfirmation(&witnesspb.WitnessConfirmation{
		WitnessId:        []byte("witness1"),
		ConfirmationType: witnesspb.ConfirmationType_CONFIRMATION_TYPE_DISAGREE,
	})

	// 验证只计算了一次
	session.mu.RLock()
	assert.Equal(t, 1, session.agreeCount)
	assert.Equal(t, 0, session.disagreeCount)
	session.mu.RUnlock()
}

// TestVotingSession_Concurrent 测试并发安全
func TestVotingSession_Concurrent(t *testing.T) {
	session := NewVotingSession("report-1", "target-peer", 100, func(result *witnesspb.WitnessVotingResult) {
		// 只需要不 panic
	})

	session.StartTimeout(1 * time.Second)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			session.AddConfirmation(&witnesspb.WitnessConfirmation{
				WitnessId:        []byte("witness" + string(rune(i))),
				ConfirmationType: witnesspb.ConfirmationType_CONFIRMATION_TYPE_AGREE,
			})
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 成功
	case <-time.After(5 * time.Second):
		require.Fail(t, "并发测试超时")
	}
}

// TestRateLimiter_Concurrent 测试限速器并发安全
func TestRateLimiter_Concurrent(t *testing.T) {
	limiter := NewRateLimiter(100, 1*time.Second)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				_ = limiter.AllowReport("peer" + string(rune(i)))
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// 成功
	case <-time.After(5 * time.Second):
		require.Fail(t, "并发测试超时")
	}
}
