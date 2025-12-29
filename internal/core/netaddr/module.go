// Package netaddr 提供网络地址类型的核心实现
//
// 本模块对应 pkg/interfaces/netaddr 接口包，提供 Address 接口的具体实现。
// netaddr 位于依赖层次的底层，不依赖其他 core 模块。
package netaddr

import (
	"go.uber.org/fx"
)

// Module 返回 netaddr fx 模块
//
// 本模块目前主要提供类型定义，不需要运行时服务。
// 如果将来需要地址解析服务，可在此添加 provider。
func Module() fx.Option {
	return fx.Module("netaddr",
	// 本模块目前不提供运行时服务
	// Address 接口的实现分布在 address、endpoint 等模块中
	)
}

