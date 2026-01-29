// Package nodedb 提供节点数据库实现
//
// 节点数据库模块负责：
// - 持久化节点信息（ID、地址、最后活跃时间）
// - 查询种子节点（按最后活跃时间）
// - 记录 Pong 时间（用于节点健康评估）
//
// # 使用示例
//
//	config := nodedb.DefaultConfig()
//	db := nodedb.NewMemoryDB(config)
//	defer db.Close()
//
//	// 更新节点信息
//	db.UpdateNode(&nodedb.NodeRecord{
//	    ID:       "peer1",
//	    IP:       net.ParseIP("192.168.1.1"),
//	    TCP:      4001,
//	    LastSeen: time.Now(),
//	})
//
//	// 查询种子节点
//	seeds := db.QuerySeeds(10, 24*time.Hour)
//
// # 实现
//
// 目前提供内存实现，适用于测试和开发。
// 生产环境可使用 Storage 模块提供的持久化存储。
//
// # 架构归属
//
// 本模块属于 Core Layer，提供节点数据持久化能力。
// 可被 Discovery 层和 Peerstore 模块使用。
//
// # 参考
//
// 设计参考 go-ethereum p2p/enode.DB
package nodedb
