package config

import (
	"context"

	basichost "github.com/dep2p/go-dep2p/p2p/host/basic"
	routed "github.com/dep2p/go-dep2p/p2p/host/routed"

	"go.uber.org/fx"
)

// closableBasicHost 封装了基础主机和 fx 应用程序
type closableBasicHost struct {
	*fx.App              // fx 应用程序实例
	*basichost.BasicHost // 基础主机实例
}

// Close 关闭主机和应用程序
// 返回:
//   - error: 关闭过程中的错误,如果成功则返回 nil
func (h *closableBasicHost) Close() error {
	// 停止 fx 应用程序,忽略错误
	_ = h.App.Stop(context.Background())
	// 关闭基础主机并返回错误
	return h.BasicHost.Close()
}

// closableRoutedHost 封装了路由主机和 fx 应用程序
type closableRoutedHost struct {
	*fx.App            // fx 应用程序实例
	*routed.RoutedHost // 路由主机实例
}

// Close 关闭主机和应用程序
// 返回:
//   - error: 关闭过程中的错误,如果成功则返回 nil
func (h *closableRoutedHost) Close() error {
	// 停止 fx 应用程序,忽略错误
	_ = h.App.Stop(context.Background())
	// 关闭路由主机并返回错误
	return h.RoutedHost.Close()
}
