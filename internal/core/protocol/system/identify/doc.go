// Package identify 实现节点身份识别协议
//
// identify 协议用于在连接建立后交换节点信息，包括：
//   - 节点 ID 和公钥
//   - 支持的协议列表
//   - 监听地址
//   - 代理版本
//
// # 协议 ID
//
//   /dep2p/identify/1.0.0
//
// # 流程
//
//  1. 连接建立后自动触发
//  2. 双方交换 Identify 消息
//  3. 更新 Peerstore 中的节点信息
//
// # 使用
//
// identify 协议由 Host 自动注册和处理，用户无需手动调用。
package identify
