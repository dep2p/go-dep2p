// Package keybook 实现密钥簿
//
// keybook 管理节点的公钥信息，用于身份验证和加密通信。
//
// # 功能
//
//   - 存储节点公钥
//   - 查询节点公钥
//   - 从 PeerID 派生公钥
//
// # 使用示例
//
//	book := keybook.NewKeyBook()
//	book.AddPubKey(peerID, pubKey)
//	pubKey := book.PubKey(peerID)
package keybook
