// Package metadata 实现元数据存储
//
// metadata 管理节点的自定义元数据，支持任意键值对存储。
//
// # 功能
//
//   - 存储节点元数据
//   - 查询节点元数据
//   - 删除元数据
//
// # 使用示例
//
//	store := metadata.NewStore()
//	store.Put(peerID, "agent", "dep2p/1.0.0")
//	agent, err := store.Get(peerID, "agent")
package metadata
