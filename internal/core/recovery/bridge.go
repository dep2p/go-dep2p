// Package recovery 提供网络恢复功能
package recovery

import (
	"context"
	"sync"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ============================================================================
//                              监控与恢复桥接
// ============================================================================

// MonitorBridge 网络监控与恢复管理的桥接器
//
// 订阅 ConnectionHealthMonitor 的状态变更，在检测到网络故障时自动触发恢复。
type MonitorBridge struct {
	monitor         interfaces.ConnectionHealthMonitor
	recoveryManager interfaces.RecoveryManager

	mu     sync.RWMutex // 保护 ctx 和 cancel
	ctx    context.Context
	cancel context.CancelFunc
}

// NewMonitorBridge 创建监控桥接器
func NewMonitorBridge(monitor interfaces.ConnectionHealthMonitor, recoveryManager interfaces.RecoveryManager) *MonitorBridge {
	return &MonitorBridge{
		monitor:         monitor,
		recoveryManager: recoveryManager,
	}
}

// Start 启动桥接器
func (b *MonitorBridge) Start(ctx context.Context) {
	b.mu.Lock()
	// 防御性检查：nil context 使用 Background
	if ctx == nil {
		ctx = context.Background()
	}
	b.ctx, b.cancel = context.WithCancel(ctx)
	localCtx := b.ctx // 创建局部副本供 goroutine 使用
	b.mu.Unlock()

	// 订阅状态变更
	ch := b.monitor.Subscribe()

	go func() {
		for {
			select {
			case <-localCtx.Done():
				b.monitor.Unsubscribe(ch)
				return
			case change, ok := <-ch:
				if !ok {
					return
				}
				b.handleStateChange(change)
			}
		}
	}()
}

// Stop 停止桥接器
func (b *MonitorBridge) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	
	if b.cancel != nil {
		b.cancel()
	}
}

// handleStateChange 处理状态变更
func (b *MonitorBridge) handleStateChange(change interfaces.ConnectionHealthChange) {
	// 只在状态变为 Down 时触发恢复
	if change.CurrentState != interfaces.ConnectionDown {
		return
	}

	// 如果已在恢复中，跳过
	if b.recoveryManager.IsRecovering() {
		return
	}

	// 将状态变更原因映射为恢复原因
	reason := MapToRecoveryReason(change.Reason)

	// 触发恢复
	go func() {
		result := b.recoveryManager.TriggerRecovery(b.ctx, reason)

		// 通知监控器恢复结果
		if result.Success {
			b.monitor.NotifyRecoverySuccess()
		} else {
			b.monitor.NotifyRecoveryFailed(result.Error)
		}
	}()
}

// MapToRecoveryReason 将状态变更原因映射为恢复原因
func MapToRecoveryReason(reason interfaces.StateChangeReason) interfaces.RecoveryReason {
	switch reason {
	case interfaces.ReasonCriticalError:
		return interfaces.RecoveryReasonNetworkUnreachable
	case interfaces.ReasonAllConnectionsLost:
		return interfaces.RecoveryReasonAllConnectionsLost
	case interfaces.ReasonErrorThreshold:
		return interfaces.RecoveryReasonErrorThreshold
	case interfaces.ReasonNetworkChanged:
		return interfaces.RecoveryReasonNetworkChange
	case interfaces.ReasonProbeFailed:
		return interfaces.RecoveryReasonNetworkUnreachable
	case interfaces.ReasonManualTrigger:
		return interfaces.RecoveryReasonManualTrigger
	default:
		return interfaces.RecoveryReasonUnknown
	}
}
