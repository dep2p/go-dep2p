// Package recovery 提供网络恢复功能
//
// # 概述
//
// recovery 包实现了网络恢复管理，当网络出现故障时自动执行恢复操作：
// - Rebind 底层传输（重建 UDP socket）
// - 重新发现地址（STUN）
// - 重建关键连接
//
// # 恢复流程
//
//	1. 检测到网络故障（通过 NetworkMonitor）
//	2. 触发恢复流程
//	3. 如果需要，执行 rebind
//	4. 重新发现外部地址
//	5. 重建关键节点连接
//	6. 通知恢复结果
//
// # 使用示例
//
//	// 创建恢复管理器
//	config := recovery.DefaultConfig()
//	manager := recovery.NewManager(config)
//
//	// 设置依赖组件
//	manager.SetConnector(connector)
//	manager.SetRebinder(rebinder)
//	manager.SetAddressDiscoverer(discoverer)
//
//	// 设置关键节点
//	manager.SetCriticalPeers([]string{"peer1", "peer2"})
//
//	// 启动管理器
//	manager.Start(ctx)
//	defer manager.Stop()
//
//	// 注册回调
//	manager.OnRecoveryComplete(func(result interfaces.RecoveryResult) {
//	    if result.Success {
//	        log.Info("恢复成功")
//	    }
//	})
//
//	// 触发恢复（通常由 NetworkMonitor 状态变更触发）
//	result := manager.TriggerRecovery(ctx, interfaces.RecoveryReasonNetworkUnreachable)
//
// # 与 NetworkMonitor 集成
//
//	// 订阅网络状态变更
//	ch := monitor.Subscribe()
//	go func() {
//	    for change := range ch {
//	        if change.CurrentState == interfaces.NetworkDown {
//	            // 将网络状态变更原因映射为恢复原因
//	            reason := mapToRecoveryReason(change.Reason)
//	            recoveryManager.TriggerRecovery(ctx, reason)
//	        }
//	    }
//	}()
//
// # 配置选项
//
// - MaxAttempts: 最大恢复尝试次数（默认 5）
// - RecoveryTimeout: 单次恢复超时（默认 30s）
// - RebindOnCriticalError: 关键错误时是否 rebind（默认 true）
// - RediscoverAddresses: 恢复时是否重新发现地址（默认 true）
package recovery
