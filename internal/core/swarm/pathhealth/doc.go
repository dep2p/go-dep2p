// Package pathhealth 提供路径健康管理功能
//
// 本包实现了 PathHealthManager 接口，用于跟踪和管理网络路径的健康状态。
//
// # 概述
//
// PathHealthManager 负责：
//   - 跟踪到每个 Peer 的多条路径（直连、中继等）
//   - 使用 EWMA 计算平滑 RTT
//   - 维护路径状态机（Unknown → Healthy → Suspect → Dead）
//   - 基于 RTT、成功率和路径类型计算综合评分
//   - 提供路径排序和切换决策支持
//
// # 路径状态机
//
//	┌─────────┐
//	│ Unknown │ ← 初始状态
//	└────┬────┘
//	     │ 首次成功探测
//	     ▼
//	┌─────────┐
//	│ Healthy │ ← RTT 正常
//	└────┬────┘
//	     │ RTT 升高 / 偶发失败
//	     ▼
//	┌─────────┐
//	│ Suspect │ ← 可疑状态
//	└────┬────┘
//	     │ 连续失败达到阈值
//	     ▼
//	┌─────────┐
//	│  Dead   │ ← 死亡状态
//	└─────────┘
//
// 状态可以在成功探测后恢复：Dead/Suspect → Healthy
//
// # 路径评分
//
// 评分公式：
//
//	Score = BaseScore * PathTypeMultiplier
//
// 其中：
//   - BaseScore = EWMA_RTT_ms + (1 - SuccessRate) * 1000
//   - PathTypeMultiplier: 直连路径 = 0.8, 中继路径 = 1.0
//
// 评分越低越好。
//
// # 切换决策
//
// 切换条件：
//  1. 当前路径死亡 → 立即切换
//  2. 存在更好路径 → 评分差超过滞后阈值且稳定
//
// # 使用示例
//
//	manager := pathhealth.NewManager(config)
//	manager.Start(ctx)
//	defer manager.Stop()
//
//	// 报告探测结果
//	manager.ReportProbe("peer-1", "/ip4/1.2.3.4/udp/4001", 50*time.Millisecond, nil)
//
//	// 获取路径排序
//	ranked := manager.RankAddrs("peer-1", addrs)
//
//	// 检查切换决策
//	decision := manager.ShouldSwitch("peer-1", currentPath)
//	if decision.ShouldSwitch {
//	    // 执行切换
//	}
package pathhealth
