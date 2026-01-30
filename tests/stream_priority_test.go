// Package tests 流优先级 API 测试
package tests

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ════════════════════════════════════════════════════════════════════════════
//                         流优先级类型测试
// ════════════════════════════════════════════════════════════════════════════

func TestStreamPriorityValues(t *testing.T) {
	// 验证优先级数值
	// 优先级越小，优先级越高
	if interfaces.StreamPriorityCritical != 0 {
		t.Errorf("StreamPriorityCritical should be 0, got %d", interfaces.StreamPriorityCritical)
	}
	if interfaces.StreamPriorityHigh != 1 {
		t.Errorf("StreamPriorityHigh should be 1, got %d", interfaces.StreamPriorityHigh)
	}
	if interfaces.StreamPriorityNormal != 2 {
		t.Errorf("StreamPriorityNormal should be 2, got %d", interfaces.StreamPriorityNormal)
	}
	if interfaces.StreamPriorityLow != 3 {
		t.Errorf("StreamPriorityLow should be 3, got %d", interfaces.StreamPriorityLow)
	}

	// 验证优先级顺序
	if interfaces.StreamPriorityCritical >= interfaces.StreamPriorityHigh {
		t.Error("Critical should have lower numeric value than High")
	}
	if interfaces.StreamPriorityHigh >= interfaces.StreamPriorityNormal {
		t.Error("High should have lower numeric value than Normal")
	}
	if interfaces.StreamPriorityNormal >= interfaces.StreamPriorityLow {
		t.Error("Normal should have lower numeric value than Low")
	}
}

func TestStreamPriorityString(t *testing.T) {
	tests := []struct {
		priority interfaces.StreamPriority
		expected string
	}{
		{interfaces.StreamPriorityCritical, "critical"},
		{interfaces.StreamPriorityHigh, "high"},
		{interfaces.StreamPriorityNormal, "normal"},
		{interfaces.StreamPriorityLow, "low"},
		{interfaces.StreamPriority(99), "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			got := tc.priority.String()
			if got != tc.expected {
				t.Errorf("Priority(%d).String() = %q, want %q", tc.priority, got, tc.expected)
			}
		})
	}
}

// ════════════════════════════════════════════════════════════════════════════
//                         流选项测试
// ════════════════════════════════════════════════════════════════════════════

func TestDefaultStreamOptions(t *testing.T) {
	opts := interfaces.DefaultStreamOptions()

	if opts.Priority != interfaces.StreamPriorityNormal {
		t.Errorf("DefaultStreamOptions().Priority = %d, want %d (Normal)",
			opts.Priority, interfaces.StreamPriorityNormal)
	}
}

func TestStreamOptionsWithPriority(t *testing.T) {
	// 创建不同优先级的选项
	criticalOpts := interfaces.StreamOptions{Priority: interfaces.StreamPriorityCritical}
	highOpts := interfaces.StreamOptions{Priority: interfaces.StreamPriorityHigh}
	normalOpts := interfaces.StreamOptions{Priority: interfaces.StreamPriorityNormal}
	lowOpts := interfaces.StreamOptions{Priority: interfaces.StreamPriorityLow}

	if criticalOpts.Priority != interfaces.StreamPriorityCritical {
		t.Errorf("criticalOpts.Priority = %d, want %d", criticalOpts.Priority, interfaces.StreamPriorityCritical)
	}
	if highOpts.Priority != interfaces.StreamPriorityHigh {
		t.Errorf("highOpts.Priority = %d, want %d", highOpts.Priority, interfaces.StreamPriorityHigh)
	}
	if normalOpts.Priority != interfaces.StreamPriorityNormal {
		t.Errorf("normalOpts.Priority = %d, want %d", normalOpts.Priority, interfaces.StreamPriorityNormal)
	}
	if lowOpts.Priority != interfaces.StreamPriorityLow {
		t.Errorf("lowOpts.Priority = %d, want %d", lowOpts.Priority, interfaces.StreamPriorityLow)
	}
}

// ════════════════════════════════════════════════════════════════════════════
//                         优先级使用场景测试
// ════════════════════════════════════════════════════════════════════════════

func TestStreamPriorityScenarios(t *testing.T) {
	// 模拟 WES 的使用场景
	scenarios := []struct {
		name     string
		protocol string
		priority interfaces.StreamPriority
	}{
		{"Consensus Proposal", "/consensus/proposal/1.0.0", interfaces.StreamPriorityCritical},
		{"Consensus Vote", "/consensus/vote/1.0.0", interfaces.StreamPriorityCritical},
		{"Consensus Commit", "/consensus/commit/1.0.0", interfaces.StreamPriorityCritical},
		{"Transaction Broadcast", "/tx/broadcast/1.0.0", interfaces.StreamPriorityHigh},
		{"Block Header Sync", "/sync/headers/1.0.0", interfaces.StreamPriorityNormal},
		{"Block Body Sync", "/sync/bodies/1.0.0", interfaces.StreamPriorityLow},
		{"Historical Data", "/sync/history/1.0.0", interfaces.StreamPriorityLow},
	}

	for _, s := range scenarios {
		t.Run(s.name, func(t *testing.T) {
			opts := interfaces.StreamOptions{Priority: s.priority}

			// 验证优先级映射正确
			switch {
			case s.priority == interfaces.StreamPriorityCritical:
				if opts.Priority.String() != "critical" {
					t.Errorf("%s should use critical priority", s.name)
				}
			case s.priority == interfaces.StreamPriorityHigh:
				if opts.Priority.String() != "high" {
					t.Errorf("%s should use high priority", s.name)
				}
			case s.priority == interfaces.StreamPriorityNormal:
				if opts.Priority.String() != "normal" {
					t.Errorf("%s should use normal priority", s.name)
				}
			case s.priority == interfaces.StreamPriorityLow:
				if opts.Priority.String() != "low" {
					t.Errorf("%s should use low priority", s.name)
				}
			}
		})
	}
}
