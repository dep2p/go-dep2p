// Package server 实现中继服务端
//
// server 提供中继服务，为其他节点转发流量。
//
// # 功能
//
//   - 接受预约请求
//   - 转发连接
//   - 资源限制
//
// # 资源限制
//
//   - 最大预约数
//   - 最大电路数
//   - 单电路带宽限制
//   - 电路持续时间限制
//
// # 使用示例
//
//	server := server.NewServer(host, server.Config{
//	    MaxReservations: 128,
//	    MaxCircuits:     16,
//	    MaxDuration:     2 * time.Minute,
//	})
//	server.Start(ctx)
package server
