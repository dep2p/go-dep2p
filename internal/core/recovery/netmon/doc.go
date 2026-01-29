// Package netmon 提供网络状态监控功能
//
// # 概述
//
// netmon 包实现了网络状态监控，包括：
// - 错误驱动的网络状态检测
// - 状态机管理 (Healthy → Degraded → Down → Recovering)
// - 状态变更事件分发
// - 防抖机制避免状态频繁抖动
//
// # 状态机
//
//	NetworkHealthy    <---> NetworkDegraded
//	       ^                      |
//	       |                      v
//	NetworkRecovering <---- NetworkDown
//
// # 使用示例
//
//	// 创建监控器
//	config := netmon.DefaultConfig()
//	monitor := netmon.NewMonitor(config)
//
//	// 启动监控
//	monitor.Start(ctx)
//	defer monitor.Stop()
//
//	// 订阅状态变更
//	ch := monitor.Subscribe()
//	go func() {
//	    for change := range ch {
//	        log.Info("状态变更",
//	            "previous", change.PreviousState,
//	            "current", change.CurrentState,
//	            "reason", change.Reason)
//
//	        if change.CurrentState == interfaces.NetworkDown {
//	            // 触发恢复
//	            recoveryManager.TriggerRecovery(...)
//	        }
//	    }
//	}()
//
//	// 上报错误（在消息发送失败时调用）
//	monitor.OnSendError(peerID, err)
//
//	// 上报成功（在消息发送成功时调用）
//	monitor.OnSendSuccess(peerID)
//
// # 核心组件
//
// - Monitor: 网络监控器主体
// - ErrorCounter: 错误计数器，跟踪每个节点的错误
// - Config: 监控配置
//
// # 配置选项
//
// - ErrorThreshold: 错误阈值（默认 3）
// - ErrorWindow: 错误统计窗口（默认 1 分钟）
// - CriticalErrors: 关键错误类型列表
// - StateChangeDebounce: 状态变更防抖时间（默认 500ms）
// - MaxRecoveryAttempts: 最大恢复尝试次数（默认 5）
package netmon
