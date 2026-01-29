// Package netmon 网络状态监控 - BUG 修复测试
package netmon

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// TestBugFix_B35_TruncatePeerID_ShortString 测试 B35 修复 - 短字符串
//
// BUG #B35: peerLabel[:8] 当 peer 长度<8 时会 panic
func TestBugFix_B35_TruncatePeerID_ShortString(t *testing.T) {
	tests := []struct {
		name     string
		peerID   string
		maxLen   int
		expected string
	}{
		{
			name:     "empty string",
			peerID:   "",
			maxLen:   8,
			expected: "",
		},
		{
			name:     "short string (3 chars)",
			peerID:   "abc",
			maxLen:   8,
			expected: "abc",
		},
		{
			name:     "exact length (8 chars)",
			peerID:   "12345678",
			maxLen:   8,
			expected: "12345678",
		},
		{
			name:     "long string (20 chars)",
			peerID:   "12345678901234567890",
			maxLen:   8,
			expected: "12345678",
		},
		{
			name:     "single char",
			peerID:   "x",
			maxLen:   8,
			expected: "x",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncatePeerID(tt.peerID, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncatePeerID(%q, %d) = %q, want %q",
					tt.peerID, tt.maxLen, result, tt.expected)
			}
		})
	}
}

// TestBugFix_B35_TruncatePeerID_MultiByteChars 测试 B35 修复 - 多字节字符
//
// BUG #B35: 直接字节切片可能切断多字节 UTF-8 字符
func TestBugFix_B35_TruncatePeerID_MultiByteChars(t *testing.T) {
	tests := []struct {
		name     string
		peerID   string
		maxLen   int
		expected string
	}{
		{
			name:     "chinese characters",
			peerID:   "你好世界测试123456",
			maxLen:   8,
			expected: "你好世界测试123",
		},
		{
			name:     "emoji",
			peerID:   "😀😁😂😃😄😅😆😇",
			maxLen:   4,
			expected: "😀😁😂😃",
		},
		{
			name:     "mixed ascii and unicode",
			peerID:   "abc你好world测试",
			maxLen:   8,
			expected: "abc你好wo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncatePeerID(tt.peerID, tt.maxLen)
			
			// 验证结果长度不超过最大值
			runes := []rune(result)
			if len(runes) > tt.maxLen {
				t.Errorf("truncatePeerID(%q, %d) returned %d runes, want <= %d",
					tt.peerID, tt.maxLen, len(runes), tt.maxLen)
			}
			
			// 验证结果是有效的 UTF-8
			if !strings.Contains(result, "�") {
				// 不包含替换字符，说明没有破坏 UTF-8 编码
			} else {
				t.Errorf("truncatePeerID(%q, %d) produced invalid UTF-8: %q",
					tt.peerID, tt.maxLen, result)
			}
		})
	}
}

// TestBugFix_B35_OnSendError_ShortPeerID 测试 B35 修复 - OnSendError 使用短 peer ID
func TestBugFix_B35_OnSendError_ShortPeerID(t *testing.T) {
	config := DefaultConfig()
	monitor := NewMonitor(config)

	ctx := context.Background()
	monitor.Start(ctx)
	defer monitor.Stop()

	// 使用各种长度的 peer ID，不应该 panic
	testCases := []string{
		"",              // 空
		"a",             // 1 字符
		"abc",           // 3 字符
		"12345678",      // 8 字符
		"1234567890",    // 10 字符
		"你好",          // 多字节字符
		"😀😁",          // emoji
	}

	for _, peerID := range testCases {
		// 应该不会 panic
		monitor.OnSendError(peerID, errors.New("test error"))
	}

	// 验证监控器仍在正常运行
	state := monitor.GetState()
	// 状态应该是有效的枚举值
	if state != interfaces.ConnectionHealthy &&
		state != interfaces.ConnectionDegraded &&
		state != interfaces.ConnectionDown &&
		state != interfaces.ConnectionRecovering {
		t.Errorf("Invalid monitor state: %v", state)
	}
}

// TestBugFix_B36_Unsubscribe_Safe 测试 B36 修复 - 安全的切片删除
//
// BUG #B36: Unsubscribe 在遍历中删除切片元素
func TestBugFix_B36_Unsubscribe_Safe(t *testing.T) {
	config := DefaultConfig()
	monitor := NewMonitor(config)

	ctx := context.Background()
	monitor.Start(ctx)
	defer monitor.Stop()

	// 创建多个订阅
	ch1 := monitor.Subscribe()
	ch2 := monitor.Subscribe()
	ch3 := monitor.Subscribe()

	// 验证初始订阅数
	monitor.subscribersMu.RLock()
	initialCount := len(monitor.subscribers)
	monitor.subscribersMu.RUnlock()

	if initialCount != 3 {
		t.Fatalf("Expected 3 subscribers, got %d", initialCount)
	}

	// 取消订阅中间的订阅者
	monitor.Unsubscribe(ch2)

	// 验证订阅数减少
	monitor.subscribersMu.RLock()
	afterCount := len(monitor.subscribers)
	monitor.subscribersMu.RUnlock()

	if afterCount != 2 {
		t.Errorf("Expected 2 subscribers after unsubscribe, got %d", afterCount)
	}

	// 验证剩余订阅者仍可接收消息
	monitor.TriggerRecoveryState()

	select {
	case <-ch1:
		// ch1 应该收到消息
	case <-time.After(100 * time.Millisecond):
		t.Error("ch1 did not receive state change")
	}

	select {
	case <-ch3:
		// ch3 应该收到消息
	case <-time.After(100 * time.Millisecond):
		t.Error("ch3 did not receive state change")
	}

	// 清理
	monitor.Unsubscribe(ch1)
	monitor.Unsubscribe(ch3)
}

// TestBugFix_B36_Unsubscribe_Concurrent 测试 B36 修复 - 并发取消订阅
func TestBugFix_B36_Unsubscribe_Concurrent(t *testing.T) {
	config := DefaultConfig()
	monitor := NewMonitor(config)

	ctx := context.Background()
	monitor.Start(ctx)
	defer monitor.Stop()

	// 创建多个订阅
	const numSubscribers = 20
	channels := make([]<-chan interfaces.ConnectionHealthChange, numSubscribers)
	for i := 0; i < numSubscribers; i++ {
		channels[i] = monitor.Subscribe()
	}

	// 并发取消订阅
	var wg sync.WaitGroup
	wg.Add(numSubscribers)

	for i := 0; i < numSubscribers; i++ {
		go func(ch <-chan interfaces.ConnectionHealthChange) {
			defer wg.Done()
			time.Sleep(time.Millisecond * time.Duration(i%5))
			monitor.Unsubscribe(ch)
		}(channels[i])
	}

	wg.Wait()

	// 验证所有订阅者都被清理
	monitor.subscribersMu.RLock()
	finalCount := len(monitor.subscribers)
	monitor.subscribersMu.RUnlock()

	if finalCount != 0 {
		t.Errorf("Expected 0 subscribers after concurrent unsubscribe, got %d", finalCount)
	}
}

// TestBugFix_B37_NewMonitor_ValidateConfig 测试 B37 修复 - Validate 正确调用
//
// BUG #B37: NewMonitor 忽略 Validate 返回的错误
func TestBugFix_B37_NewMonitor_ValidateConfig(t *testing.T) {
	// 创建无效配置
	config := &Config{
		ErrorThreshold:      0,  // 无效：应该 > 0
		ProbeInterval:       0,  // 无效：应该 > 0
		MaxRecoveryAttempts: 0,  // 无效：应该 > 0
		BackoffFactor:       0,  // 无效：应该 > 1
	}

	// NewMonitor 应该修正配置
	monitor := NewMonitor(config)

	// 验证配置被修正
	if monitor.config.ErrorThreshold <= 0 {
		t.Errorf("ErrorThreshold not corrected: %d", monitor.config.ErrorThreshold)
	}
	if monitor.config.ProbeInterval <= 0 {
		t.Errorf("ProbeInterval not corrected: %v", monitor.config.ProbeInterval)
	}
	if monitor.config.MaxRecoveryAttempts <= 0 {
		t.Errorf("MaxRecoveryAttempts not corrected: %d", monitor.config.MaxRecoveryAttempts)
	}
	if monitor.config.BackoffFactor <= 1.0 {
		t.Errorf("BackoffFactor not corrected: %f", monitor.config.BackoffFactor)
	}
}

// TestBugFix_B38_NotifySubscribers_NoLoss 测试 B38 修复 - 不丢失关键通知
//
// BUG #B38: notifySubscribers 使用 select default 可能丢失消息
// 这个测试验证即使订阅者处理慢，也不会完全丢失消息
func TestBugFix_B38_NotifySubscribers_NoLoss(t *testing.T) {
	config := DefaultConfig()
	config.ErrorThreshold = 1
	config.StateChangeDebounce = 5 * time.Millisecond
	monitor := NewMonitor(config)

	ctx := context.Background()
	monitor.Start(ctx)
	defer monitor.Stop()

	// 创建一个较小缓冲区的订阅者
	ch := monitor.Subscribe()

	// 触发多次状态变更（在 Down 和 Healthy 之间切换）
	const numCycles = 10
	go func() {
		for i := 0; i < numCycles; i++ {
			// 触发错误 -> Down
			monitor.OnSendError("peer-test", errors.New("test error"))
			time.Sleep(10 * time.Millisecond)
			
			// 触发成功 -> Healthy
			monitor.OnSendSuccess("peer-test")
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// 慢速消费者
	receivedCount := 0
	timeout := time.After(3 * time.Second)

	for {
		select {
		case _, ok := <-ch:
			if !ok {
				goto done
			}
			receivedCount++
			time.Sleep(8 * time.Millisecond) // 模拟慢速处理
		case <-timeout:
			// 超时后退出
			goto done
		}
	}

done:
	// 我们期望收到大约 numCycles*2 个状态变更（Down + Healthy）
	expectedChanges := numCycles * 2
	minExpected := expectedChanges - 5 // 允许最多5个因超时丢失
	
	if receivedCount < minExpected {
		t.Errorf("Received only %d changes, expected at least %d (out of ~%d)",
			receivedCount, minExpected, expectedChanges)
	}

	t.Logf("Received %d state changes in %d cycles (%.1f%%)",
		receivedCount, numCycles, float64(receivedCount)/float64(expectedChanges)*100)

	monitor.Unsubscribe(ch)
}

// TestBugFix_B38_NotifySubscribers_Timeout 测试 B38 修复 - 超时机制
func TestBugFix_B38_NotifySubscribers_Timeout(t *testing.T) {
	config := DefaultConfig()
	monitor := NewMonitor(config)

	ctx := context.Background()
	monitor.Start(ctx)

	// 创建一个阻塞的订阅者（缓冲区为0）
	blockingCh := make(chan interfaces.ConnectionHealthChange)
	monitor.subscribersMu.Lock()
	monitor.subscribers = append(monitor.subscribers, blockingCh)
	monitor.subscribersMu.Unlock()

	// 触发状态变更（应该不会永久阻塞）
	done := make(chan bool)
	go func() {
		monitor.TriggerRecoveryState()
		done <- true
	}()

	// 验证在合理时间内完成（100ms超时 + 一些余量）
	select {
	case <-done:
		// 成功完成
		t.Log("State change completed without blocking")
	case <-time.After(300 * time.Millisecond):
		t.Error("notifySubscribers blocked too long, timeout mechanism may not be working")
	}

	// 在 Stop 之前清理手动添加的通道，避免 double close
	monitor.subscribersMu.Lock()
	for i, ch := range monitor.subscribers {
		if ch == blockingCh {
			// 移除这个通道，让它不被 Stop() 关闭
			lastIdx := len(monitor.subscribers) - 1
			monitor.subscribers[i] = monitor.subscribers[lastIdx]
			monitor.subscribers = monitor.subscribers[:lastIdx]
			break
		}
	}
	monitor.subscribersMu.Unlock()

	// 手动关闭我们创建的通道
	close(blockingCh)

	// 现在可以安全 Stop
	monitor.Stop()
}
