// Package watcher 提供网络监控和变化处理功能
//
// 核心功能：
// - 网络接口监控（NetworkMonitor）
// - 网络变化检测（Major/Minor）
// - 平台特定实现（Unix/Windows/Stub）
// - 网络变化处理（NetworkChangeHandler）
//
// 使用示例：
//
//	monitor, err := watcher.NewMonitor()
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 订阅网络变化事件
//	events := monitor.Subscribe()
//
//	// 启动监控
//	ctx := context.Background()
//	monitor.Start(ctx)
//
//	// 处理事件
//	for event := range events {
//	    log.Printf("网络变化: %s", event.Type)
//	}
//
// 平台支持：
// - Linux: 轮询监控（可扩展为 netlink）
// - macOS/BSD: 轮询监控（可扩展为 routing socket）
// - Windows: 轮询监控（可扩展为 IP Helper API）
// - 其他平台: Stub 实现，需要手动调用 NotifyChange()
package watcher

import (
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("core/watcher")
