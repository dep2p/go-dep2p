// Package introspect 提供本地自省 HTTP 服务
//
// 该服务运行在本地端口，提供 JSON 格式的诊断信息，用于调试和监控。
// 默认绑定到 127.0.0.1，不暴露到网络。
//
// # 端点
//
//	GET /debug/introspect      - 完整诊断报告 (JSON)
//	GET /debug/introspect/node - 节点信息
//	GET /debug/introspect/connections - 连接信息
//	GET /debug/introspect/peers - 节点列表
//	GET /debug/introspect/bandwidth - 带宽统计
//	GET /debug/pprof/*         - Go pprof 端点
//	GET /health                - 健康检查
//
// # 使用示例
//
//	server := introspect.New(introspect.Config{
//	    Addr: "127.0.0.1:6060",
//	    Host: myHost,
//	})
//	server.Start(ctx)
//	defer server.Stop()
//
//	// 访问 http://127.0.0.1:6060/debug/introspect
//
// # 安全
//
// 默认只监听本地地址，不暴露到网络。
// 如果需要远程访问，请确保配置适当的访问控制。
//
// # 架构归属
//
// 本模块属于 Core Layer，提供节点级别的诊断能力。
// 通过 config.Diagnostics.EnableIntrospect 配置启用。
package introspect
