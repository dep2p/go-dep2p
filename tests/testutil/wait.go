package testutil

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p"
)

// WaitForCondition 等待条件满足或超时
//
// 参数：
//   - t: 测试对象
//   - timeout: 超时时间
//   - interval: 检查间隔
//   - condition: 条件函数，返回 true 表示条件满足
//
// 返回：条件是否满足（超时返回 false）
func WaitForCondition(t *testing.T, timeout time.Duration, interval time.Duration, condition func() bool) bool {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// 立即检查一次
	if condition() {
		return true
	}

	for {
		select {
		case <-ctx.Done():
			return false
		case <-ticker.C:
			if condition() {
				return true
			}
		}
	}
}

// WaitForConditionOrFail 等待条件满足，超时则 fail 测试
func WaitForConditionOrFail(t *testing.T, timeout time.Duration, interval time.Duration, condition func() bool, msg string) {
	t.Helper()

	if !WaitForCondition(t, timeout, interval, condition) {
		t.Fatalf("等待超时: %s", msg)
	}
}

// Eventually 在指定时间内重试条件检查
//
// 使用默认间隔 100ms。
//
// 示例:
//
//	testutil.Eventually(t, 10*time.Second, func() bool {
//	    return node.ConnectionCount() > 0
//	}, "应该建立连接")
func Eventually(t *testing.T, timeout time.Duration, condition func() bool, msg string) {
	t.Helper()
	WaitForConditionOrFail(t, timeout, 100*time.Millisecond, condition, msg)
}

// EventuallyOrFail 在指定时间内重试条件检查，失败则 fail 测试（默认 1 秒超时）
//
// Deprecated: 使用 Eventually(t, timeout, condition, msg) 替代
func EventuallyOrFail(t *testing.T, condition func() bool, msg string) {
	t.Helper()
	WaitForConditionOrFail(t, time.Second, 100*time.Millisecond, condition, msg)
}

// WaitForMembers 等待 Realm 成员数达到预期值
//
// 用于等待 PSK 认证完成和成员发现。
//
// 示例:
//
//	testutil.WaitForMembers(t, realm, 3, 30*time.Second)
func WaitForMembers(t *testing.T, realm *dep2p.Realm, count int, timeout time.Duration) {
	t.Helper()

	Eventually(t, timeout, func() bool {
		return len(realm.Members()) >= count
	}, "等待成员数达到预期")
}

// WaitForMembersNoFail 等待 Realm 成员数达到预期值（不会失败）
//
// 与 WaitForMembers 类似，但超时不会使测试失败，而是返回错误。
// 用于环境依赖的测试（如 mDNS）。
//
// 示例:
//
//	err := testutil.WaitForMembersNoFail(t, realm, 3, 30*time.Second)
//	if err != nil {
//	    t.Logf("成员发现不完整: %v", err)
//	}
func WaitForMembersNoFail(t *testing.T, realm *dep2p.Realm, count int, timeout time.Duration) error {
	t.Helper()

	success := WaitForCondition(t, timeout, 100*time.Millisecond, func() bool {
		return len(realm.Members()) >= count
	})

	if !success {
		return fmt.Errorf("成员数未达到预期: 期望 %d, 实际 %d", count, len(realm.Members()))
	}
	return nil
}

// WaitForMessage 等待 PubSub 消息
//
// 从订阅中读取下一条消息，超时则失败。
//
// 示例:
//
//	msg := testutil.WaitForMessage(t, sub, 10*time.Second)
//	assert.Equal(t, "Hello", string(msg.Data))
func WaitForMessage(t *testing.T, sub *dep2p.Subscription, timeout time.Duration) *dep2p.Message {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	msg, err := sub.Next(ctx)
	if err != nil {
		t.Fatalf("等待消息超时: %v", err)
	}

	return msg
}

// Sleep 等待指定时间（用于测试中的简单延迟）
func Sleep(d time.Duration) {
	time.Sleep(d)
}
