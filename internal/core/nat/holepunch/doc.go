// Package holepunch 实现 NAT 打洞协议
//
// holepunch 提供 UDP 和 TCP 打洞功能，帮助处于 NAT 后的节点
// 建立直接连接，避免通过中继转发流量。
//
// # 打洞流程
//
//  1. 通过中继或信令服务器交换地址信息
//  2. 双方同时向对方发送连接请求
//  3. NAT 设备记录出站连接，允许入站回复
//  4. 建立直接连接
//
// # 支持的 NAT 类型
//
//   - Full Cone NAT: 容易打洞
//   - Restricted Cone NAT: 需要先发包
//   - Port Restricted Cone NAT: 端口限制
//   - Symmetric NAT: 难以打洞，通常需要中继
//
// # 使用示例
//
//	puncher := holepunch.NewPuncher(config)
//	conn, err := puncher.DirectConnect(ctx, peerID, addrs)
package holepunch
