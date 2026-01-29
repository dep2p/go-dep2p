// Package eventbus 实现事件总线
package eventbus

import pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"

// ============================================================================
// 本地选项函数
// ============================================================================

// BufSize 设置订阅缓冲区大小
//
// 这是一个便利函数，与 pkg/interfaces.BufSize 等效
func BufSize(size int) pkgif.SubscriptionOpt {
	return pkgif.BufSize(size)
}

// Stateful 设置发射器为有状态模式
//
// 这是一个便利函数，与 pkg/interfaces.Stateful 等效
func Stateful() pkgif.EmitterOpt {
	return pkgif.Stateful()
}
