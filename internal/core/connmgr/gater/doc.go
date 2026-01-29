// Package gater 实现连接门控器
//
// gater 提供多阶段的连接拦截和过滤功能，在连接建立的各个阶段
// 进行检查，决定是否允许连接继续。
//
// # 拦截阶段
//
//   - InterceptPeerDial: 拨号前检查目标节点
//   - InterceptAddrDial: 拨号前检查目标地址
//   - InterceptAccept: 接受连接前检查
//   - InterceptSecured: 安全握手后检查
//   - InterceptUpgraded: 连接升级后检查
//
// # 使用示例
//
//	gater := gater.NewGater()
//	gater.BlockPeer("bad-peer")
//
//	if !gater.InterceptPeerDial("bad-peer") {
//	    // 拨号被拒绝
//	}
package gater
