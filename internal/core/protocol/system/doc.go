// Package system 实现系统协议
//
// system 包含 DeP2P 内置的系统级协议，这些协议在节点启动时
// 自动注册，提供基础的网络功能。
//
// # 系统协议
//
//   - identify: 节点身份识别协议
//   - ping: 存活检测协议
//
// # 协议 ID
//
//   - /dep2p/identify/1.0.0
//   - /dep2p/sys/ping/1.0.0
package system
