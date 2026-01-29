// Package addrbook 实现地址簿
//
// addrbook 管理节点的多地址信息，包括地址的添加、查询、
// 过期清理等功能。
//
// # 功能
//
//   - 添加/查询节点地址
//   - 地址 TTL 管理
//   - 地址过期清理
//   - 已认证地址标记
//
// # 地址 TTL
//
//   - PermanentAddrTTL: 永久地址
//   - ConnectedAddrTTL: 已连接地址（30 分钟）
//   - RecentlyConnectedAddrTTL: 最近连接（10 分钟）
//   - TempAddrTTL: 临时地址（2 分钟）
//
// # 使用示例
//
//	book := addrbook.NewAddrBook()
//	book.AddAddrs(peerID, addrs, ttl)
//	addrs := book.Addrs(peerID)
package addrbook
