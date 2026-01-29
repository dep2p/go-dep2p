package dep2p

import (
	"context"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ════════════════════════════════════════════════════════════════════════════
//                              用户 API: Liveness
// ════════════════════════════════════════════════════════════════════════════

// Liveness 用户级存活检测服务 API
//
// Liveness 提供节点存活检测和连接质量监控。
//
// 使用示例：
//
//	liveness := realm.Liveness()
//	
//	// 单次 Ping
//	rtt, _ := liveness.Ping(ctx, peerID)
//	fmt.Printf("RTT: %v\n", rtt)
//	
//	// 查询状态
//	status, _ := liveness.Status(peerID)
//	if status.Alive {
//	    fmt.Printf("Peer is alive, RTT: %v\n", status.LastRTT)
//	}
//	
//	// 持续监控
//	statusCh, _ := liveness.Watch(peerID)
//	for status := range statusCh {
//	    fmt.Printf("Peer status changed: %v\n", status)
//	}
type Liveness struct {
	internal interfaces.Liveness
}

// ════════════════════════════════════════════════════════════════════════════
//                              存活检测
// ════════════════════════════════════════════════════════════════════════════

// Ping 发送 ping 并测量 RTT
//
// 参数：
//   - ctx: 上下文（用于超时控制）
//   - peerID: 目标节点 ID
//
// 返回：
//   - time.Duration: 往返时间（RTT）
//   - error: 错误信息（如节点不可达、超时）
//
// 示例：
//
//	rtt, err := liveness.Ping(ctx, peerID)
//	if err != nil {
//	    log.Printf("Ping failed: %v", err)
//	} else {
//	    fmt.Printf("RTT: %v\n", rtt)
//	}
func (l *Liveness) Ping(ctx context.Context, peerID string) (time.Duration, error) {
	return l.internal.Ping(ctx, peerID)
}

// Check 检查节点是否存活
//
// 参数：
//   - ctx: 上下文
//   - peerID: 目标节点 ID
//
// 返回：
//   - bool: 是否存活
//   - error: 错误信息
//
// 示例：
//
//	alive, err := liveness.Check(ctx, peerID)
//	if err != nil {
//	    log.Printf("Check failed: %v", err)
//	} else if alive {
//	    fmt.Println("Peer is alive")
//	}
func (l *Liveness) Check(ctx context.Context, peerID string) (bool, error) {
	return l.internal.Check(ctx, peerID)
}

// ════════════════════════════════════════════════════════════════════════════
//                              状态查询
// ════════════════════════════════════════════════════════════════════════════

// Status 获取节点存活状态
//
// 参数：
//   - peerID: 目标节点 ID
//
// 返回：
//   - *LivenessStatus: 状态对象
//   - error: 错误信息
//
// 示例：
//
//	status, err := liveness.Status(peerID)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	
//	if status.Alive {
//	    fmt.Printf("Peer is alive\n")
//	    fmt.Printf("Last seen: %v\n", status.LastSeen)
//	    fmt.Printf("Last RTT: %v\n", status.LastRTT)
//	    fmt.Printf("Avg RTT: %v\n", status.AvgRTT)
//	} else {
//	    fmt.Printf("Peer is down (fail count: %d)\n", status.FailCount)
//	}
func (l *Liveness) Status(peerID string) (*LivenessStatus, error) {
	status := l.internal.GetStatus(peerID)
	return &LivenessStatus{
		Alive:        status.Alive,
		LastSeen:     status.LastSeen,
		LastRTT:      status.LastRTT,
		AvgRTT:       status.AvgRTT,
		MinRTT:       status.MinRTT,
		MaxRTT:       status.MaxRTT,
		FailCount:    status.FailCount,
		TotalPings:   status.TotalPings,
		SuccessCount: status.SuccessCount,
		SuccessRate:  status.SuccessRate,
	}, nil
}

// ════════════════════════════════════════════════════════════════════════════
//                              持续监控
// ════════════════════════════════════════════════════════════════════════════

// Watch 监控节点状态变化
//
// 参数：
//   - peerID: 目标节点 ID
//
// 返回：
//   - <-chan *LivenessStatus: 状态变化通道
//   - error: 错误信息
//
// 示例：
//
//	statusCh, err := liveness.Watch(peerID)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	
//	for status := range statusCh {
//	    if status.Alive {
//	        fmt.Printf("Peer is UP, RTT: %v\n", status.LastRTT)
//	    } else {
//	        fmt.Printf("Peer is DOWN\n")
//	    }
//	}
func (l *Liveness) Watch(peerID string) (<-chan *LivenessStatus, error) {
	eventCh, err := l.internal.Watch(peerID)
	if err != nil {
		return nil, err
	}
	
	// 转换通道类型
	statusCh := make(chan *LivenessStatus, 10)
	go func() {
		defer close(statusCh)
		for event := range eventCh {
			statusCh <- &LivenessStatus{
				Alive:        event.Status.Alive,
				LastSeen:     event.Status.LastSeen,
				LastRTT:      event.Status.LastRTT,
				AvgRTT:       event.Status.AvgRTT,
				MinRTT:       event.Status.MinRTT,
				MaxRTT:       event.Status.MaxRTT,
				FailCount:    event.Status.FailCount,
				TotalPings:   event.Status.TotalPings,
				SuccessCount: event.Status.SuccessCount,
				SuccessRate:  event.Status.SuccessRate,
			}
		}
	}()
	
	return statusCh, nil
}

// Unwatch 停止监控节点
//
// 参数：
//   - peerID: 目标节点 ID
//
// 示例：
//
//	liveness.Unwatch(peerID)
func (l *Liveness) Unwatch(peerID string) error {
	return l.internal.Unwatch(peerID)
}

// ════════════════════════════════════════════════════════════════════════════
//                              类型定义
// ════════════════════════════════════════════════════════════════════════════

// LivenessStatus 存活状态
type LivenessStatus struct {
	// Alive 是否存活
	Alive bool
	
	// LastSeen 最后一次确认存活的时间
	LastSeen time.Time
	
	// LastRTT 最后一次 RTT
	LastRTT time.Duration
	
	// AvgRTT 平均 RTT（基于滑动窗口）
	AvgRTT time.Duration
	
	// MinRTT 最小 RTT（历史最优）
	MinRTT time.Duration
	
	// MaxRTT 最大 RTT（历史最差）
	MaxRTT time.Duration
	
	// FailCount 连续失败次数
	FailCount int
	
	// TotalPings 总 Ping 次数
	TotalPings int
	
	// SuccessCount 成功次数
	SuccessCount int
	
	// SuccessRate 成功率 (0.0 - 1.0)
	SuccessRate float64
}
