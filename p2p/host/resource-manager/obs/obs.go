// Package obs 实现资源管理器的指标跟踪
//
// 已弃用: obs包已弃用,所有导出的类型和方法
// 已移至rcmgr包。请使用rcmgr包中对应的标识符,例如:
// obs.NewStatsTraceReporter => rcmgr.NewStatsTraceReporter
package obs

import (
	rcmgr "github.com/dep2p/go-dep2p/p2p/host/resource-manager"
)

// MustRegisterWith 注册资源管理器的指标收集器
// 参数:
//   - 继承自rcmgr.MustRegisterWith
//
// 返回:
//   - 继承自rcmgr.MustRegisterWith
var MustRegisterWith = rcmgr.MustRegisterWith

// StatsTraceReporter 使用跟踪信息报告资源管理器的统计数据
// 继承自rcmgr.StatsTraceReporter类型
type StatsTraceReporter = rcmgr.StatsTraceReporter

// NewStatsTraceReporter 创建新的统计跟踪报告器
// 参数:
//   - 继承自rcmgr.NewStatsTraceReporter
//
// 返回:
//   - 继承自rcmgr.NewStatsTraceReporter
var NewStatsTraceReporter = rcmgr.NewStatsTraceReporter
