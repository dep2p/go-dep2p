// Package introspect 定义本地自省服务接口
//
// 自省服务提供 HTTP 端点用于获取节点诊断信息，支持：
//   - 节点状态查询
//   - 连接信息
//   - Realm 信息
//   - Relay 信息
//   - pprof 性能分析
//
// 默认监听地址为 127.0.0.1:6060，仅限本地访问。
package introspect

import (
	"context"
)

// Server 自省服务接口
//
// Server 提供本地 HTTP 服务用于诊断和监控。
// 默认绑定到 127.0.0.1，不暴露到网络。
//
// 端点：
//   - GET /debug/introspect           - 完整诊断报告
//   - GET /debug/introspect/node      - 节点信息
//   - GET /debug/introspect/connections - 连接信息
//   - GET /debug/introspect/realm     - Realm 信息
//   - GET /debug/introspect/relay     - Relay 信息
//   - GET /debug/pprof/*              - Go pprof 端点
//   - GET /health                     - 健康检查
type Server interface {
	// Start 启动服务
	//
	// 在配置的地址上启动 HTTP 服务器。
	// 如果服务已在运行，返回 nil。
	Start(ctx context.Context) error

	// Stop 停止服务
	//
	// 优雅关闭 HTTP 服务器，等待现有请求完成。
	Stop() error

	// Addr 返回实际监听地址
	//
	// 如果服务正在运行，返回实际绑定的地址。
	// 否则返回配置的地址。
	Addr() string
}

// DefaultAddr 默认监听地址
const DefaultAddr = "127.0.0.1:6060"

