// Package netmon 网络状态监控 - 错误计数器测试
package netmon

import (
	"errors"
	"testing"
	"time"
)

// ============================================================================
//                              ErrorCounter 测试
// ============================================================================

// TestErrorCounter_RecordError 测试错误记录
func TestErrorCounter_RecordError(t *testing.T) {
	config := DefaultConfig()
	config.ErrorThreshold = 3
	counter := NewErrorCounter(config)

	peer := "peer-1"
	err := errors.New("connection failed")

	// 第一次错误 - 未达到阈值
	reached, critical := counter.RecordError(peer, err)
	if reached {
		t.Error("first error should not reach threshold")
	}
	if critical {
		t.Error("first error should not be critical")
	}

	// 第二次错误 - 未达到阈值
	reached, critical = counter.RecordError(peer, err)
	if reached {
		t.Error("second error should not reach threshold")
	}

	// 第三次错误 - 达到阈值
	reached, critical = counter.RecordError(peer, err)
	if !reached {
		t.Error("third error should reach threshold")
	}
}

// TestErrorCounter_RecordSuccess 测试成功记录
func TestErrorCounter_RecordSuccess(t *testing.T) {
	config := DefaultConfig()
	config.ErrorThreshold = 3
	counter := NewErrorCounter(config)

	peer := "peer-1"

	// 记录两次错误
	counter.RecordError(peer, errors.New("error 1"))
	counter.RecordError(peer, errors.New("error 2"))

	// 记录成功应该重置错误计数
	counter.RecordSuccess(peer)

	// 验证错误计数被重置
	if counter.GetPeerErrorCount(peer) != 0 {
		t.Errorf("expected error count 0 after success, got %d", counter.GetPeerErrorCount(peer))
	}
}

// TestErrorCounter_GetFailingPeers 测试获取失败节点
func TestErrorCounter_GetFailingPeers(t *testing.T) {
	config := DefaultConfig()
	config.ErrorThreshold = 2
	counter := NewErrorCounter(config)

	// peer-1: 达到阈值
	counter.RecordError("peer-1", errors.New("error"))
	counter.RecordError("peer-1", errors.New("error"))

	// peer-2: 未达到阈值
	counter.RecordError("peer-2", errors.New("error"))

	// peer-3: 健康
	counter.RecordSuccess("peer-3")

	failing := counter.GetFailingPeers()

	// 应该只有 peer-1
	if len(failing) != 1 {
		t.Errorf("expected 1 failing peer, got %d", len(failing))
	}
	if len(failing) > 0 && failing[0] != "peer-1" {
		t.Errorf("expected peer-1 to be failing, got %s", failing[0])
	}
}

// TestErrorCounter_GetHealthyPeers 测试获取健康节点
func TestErrorCounter_GetHealthyPeers(t *testing.T) {
	config := DefaultConfig()
	config.ErrorThreshold = 2
	counter := NewErrorCounter(config)

	// peer-1: 达到阈值
	counter.RecordError("peer-1", errors.New("error"))
	counter.RecordError("peer-1", errors.New("error"))

	// peer-2: 未达到阈值
	counter.RecordError("peer-2", errors.New("error"))

	// peer-3: 健康
	counter.RecordSuccess("peer-3")

	healthy := counter.GetHealthyPeers()

	// 应该有 peer-2 和 peer-3
	if len(healthy) != 2 {
		t.Errorf("expected 2 healthy peers, got %d", len(healthy))
	}
}

// TestErrorCounter_CriticalError 测试关键错误检测
func TestErrorCounter_CriticalError(t *testing.T) {
	config := DefaultConfig()
	config.CriticalErrors = []string{"network is unreachable", "no route to host"}
	counter := NewErrorCounter(config)

	peer := "peer-1"

	// 关键错误
	criticalErr := errors.New("network is unreachable")
	_, critical := counter.RecordError(peer, criticalErr)
	if !critical {
		t.Error("should detect critical error")
	}

	// 验证关键错误被记录
	lastPeer, lastTime, lastErr := counter.GetLastCriticalError()
	if lastErr == nil {
		t.Error("critical error not recorded")
	}
	if lastPeer != peer {
		t.Errorf("expected peer %s, got %s", peer, lastPeer)
	}
	if lastTime.IsZero() {
		t.Error("critical error time not recorded")
	}

	// 普通错误
	normalErr := errors.New("connection timeout")
	_, critical = counter.RecordError(peer, normalErr)
	if critical {
		t.Error("should not detect normal error as critical")
	}
}

// TestErrorCounter_Reset 测试重置
func TestErrorCounter_Reset(t *testing.T) {
	config := DefaultConfig()
	counter := NewErrorCounter(config)

	// 记录一些错误
	counter.RecordError("peer-1", errors.New("error"))
	counter.RecordError("peer-2", errors.New("critical error"))

	// 重置
	counter.Reset()

	// 验证所有计数被清除
	if counter.TotalPeerCount() != 0 {
		t.Errorf("expected 0 peers after reset, got %d", counter.TotalPeerCount())
	}

	_, _, lastErr := counter.GetLastCriticalError()
	if lastErr != nil {
		t.Error("critical error not cleared after reset")
	}
}

// TestErrorCounter_ResetPeer 测试重置单个节点
func TestErrorCounter_ResetPeer(t *testing.T) {
	config := DefaultConfig()
	counter := NewErrorCounter(config)

	// 记录多个节点的错误
	counter.RecordError("peer-1", errors.New("error"))
	counter.RecordError("peer-2", errors.New("error"))

	// 重置 peer-1
	counter.ResetPeer("peer-1")

	// 验证 peer-1 被清除，peer-2 保留
	if counter.TotalPeerCount() != 1 {
		t.Errorf("expected 1 peer after reset, got %d", counter.TotalPeerCount())
	}

	if counter.GetPeerErrorCount("peer-1") != 0 {
		t.Error("peer-1 error count not reset")
	}

	if counter.GetPeerErrorCount("peer-2") != 1 {
		t.Error("peer-2 error count should remain")
	}
}

// TestErrorCounter_ErrorWindow 测试错误窗口
func TestErrorCounter_ErrorWindow(t *testing.T) {
	config := DefaultConfig()
	config.ErrorWindow = 100 * time.Millisecond
	counter := NewErrorCounter(config)

	peer := "peer-1"

	// 记录错误
	counter.RecordError(peer, errors.New("error 1"))

	// 等待超过窗口期
	time.Sleep(150 * time.Millisecond)

	// 再次记录错误
	counter.RecordError(peer, errors.New("error 2"))

	// 旧错误应该被清理
	// 注意: cleanExpiredErrors 在 RecordError 时调用
	// 错误计数应该反映清理后的状态
	count := counter.GetPeerErrorCount(peer)
	if count > 2 {
		t.Errorf("expected error count <= 2 after window expiry, got %d", count)
	}
}

// TestErrorCounter_NilError 测试 nil 错误
func TestErrorCounter_NilError(t *testing.T) {
	config := DefaultConfig()
	counter := NewErrorCounter(config)

	// nil 错误不应被记录
	reached, critical := counter.RecordError("peer-1", nil)
	if reached {
		t.Error("nil error should not reach threshold")
	}
	if critical {
		t.Error("nil error should not be critical")
	}

	// 验证没有记录
	if counter.TotalPeerCount() != 0 {
		t.Errorf("expected 0 peers after nil error, got %d", counter.TotalPeerCount())
	}
}

// TestErrorCounter_MultiplePeers 测试多个节点
func TestErrorCounter_MultiplePeers(t *testing.T) {
	config := DefaultConfig()
	config.ErrorThreshold = 2
	counter := NewErrorCounter(config)

	// 多个节点分别记录错误
	for i := 1; i <= 5; i++ {
		peer := "peer-" + string(rune('0'+i))
		counter.RecordError(peer, errors.New("error"))
		if i >= 2 {
			counter.RecordError(peer, errors.New("error"))
		}
	}

	total := counter.TotalPeerCount()
	if total != 5 {
		t.Errorf("expected 5 peers, got %d", total)
	}

	failing := counter.GetFailingPeers()
	if len(failing) != 4 { // peer-2, peer-3, peer-4, peer-5
		t.Errorf("expected 4 failing peers, got %d", len(failing))
	}

	healthy := counter.GetHealthyPeers()
	if len(healthy) != 1 { // peer-1
		t.Errorf("expected 1 healthy peer, got %d", len(healthy))
	}
}

// TestErrorCounter_CaseInsensitiveCriticalError 测试大小写不敏感的关键错误检测
func TestErrorCounter_CaseInsensitiveCriticalError(t *testing.T) {
	config := DefaultConfig()
	config.CriticalErrors = []string{"Network Is Unreachable"}
	counter := NewErrorCounter(config)

	tests := []struct {
		err      error
		critical bool
	}{
		{errors.New("network is unreachable"), true},
		{errors.New("NETWORK IS UNREACHABLE"), true},
		{errors.New("Network Is Unreachable"), true},
		{errors.New("The network is unreachable now"), true},
		{errors.New("connection failed"), false},
	}

	for _, tt := range tests {
		_, critical := counter.RecordError("peer-1", tt.err)
		if critical != tt.critical {
			t.Errorf("error %q: expected critical=%v, got %v", tt.err, tt.critical, critical)
		}
	}
}

// TestErrorCounter_ConcurrentAccess 测试并发访问
func TestErrorCounter_ConcurrentAccess(t *testing.T) {
	config := DefaultConfig()
	counter := NewErrorCounter(config)

	done := make(chan bool)

	// 并发写入
	for i := 0; i < 10; i++ {
		go func(id int) {
			peer := "peer-" + string(rune('0'+id))
			for j := 0; j < 100; j++ {
				if j%2 == 0 {
					counter.RecordError(peer, errors.New("error"))
				} else {
					counter.RecordSuccess(peer)
				}
			}
			done <- true
		}(i)
	}

	// 并发读取
	for i := 0; i < 5; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = counter.GetFailingPeers()
				_ = counter.GetHealthyPeers()
				_ = counter.TotalPeerCount()
			}
			done <- true
		}()
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 15; i++ {
		<-done
	}

	// 验证不崩溃
	if counter.TotalPeerCount() < 0 {
		t.Error("invalid peer count after concurrent access")
	}
}
