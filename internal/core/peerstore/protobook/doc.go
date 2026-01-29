// Package protobook 实现协议簿
//
// protobook 管理节点支持的协议列表，用于协议协商。
//
// # 功能
//
//   - 存储节点支持的协议
//   - 查询节点支持的协议
//   - 检查协议支持
//
// # 使用示例
//
//	book := protobook.NewProtoBook()
//	book.AddProtocols(peerID, []string{"/chat/1.0.0", "/file/1.0.0"})
//	protocols := book.GetProtocols(peerID)
//	supported := book.SupportsProtocols(peerID, "/chat/1.0.0")
package protobook
