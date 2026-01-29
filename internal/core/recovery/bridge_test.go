// Package recovery 网络恢复管理
package recovery

import (
	"testing"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// TestMapToRecoveryReason 测试原因映射
func TestMapToRecoveryReason(t *testing.T) {
	tests := []struct {
		input    interfaces.StateChangeReason
		expected interfaces.RecoveryReason
	}{
		{interfaces.ReasonCriticalError, interfaces.RecoveryReasonNetworkUnreachable},
		{interfaces.ReasonAllConnectionsLost, interfaces.RecoveryReasonAllConnectionsLost},
		{interfaces.ReasonErrorThreshold, interfaces.RecoveryReasonErrorThreshold},
		{interfaces.ReasonManualTrigger, interfaces.RecoveryReasonManualTrigger},
		{interfaces.ReasonConnectionRestored, interfaces.RecoveryReasonUnknown}, // 默认映射
	}

	for _, tt := range tests {
		result := MapToRecoveryReason(tt.input)
		if result != tt.expected {
			t.Errorf("MapToRecoveryReason(%v) = %v, expected %v", tt.input, result, tt.expected)
		}
	}
}
