// Package addrmgmt 提供地址管理协议的实现
//
// 协议 ID: /dep2p/sys/addr-mgmt/1.0.0
//
// 本包实现以下协议消息：
//   - AddressRefreshNotify: 地址刷新通知
//   - AddressQueryRequest/Response: 地址查询请求/响应
//
// # 协议功能
//
// 地址管理协议负责：
//   - 地址记录签名验证
//   - 地址刷新通知广播
//   - 邻居地址查询
//   - 过期地址清理
//
// # 消息格式
//
// AddressRefreshNotify:
//
//	[NodeID: 32 bytes]
//	[RealmID length: 2 bytes]
//	[RealmID: variable]
//	[Sequence: 8 bytes]
//	[Timestamp: 8 bytes]
//	[AddressCount: 2 bytes]
//	[Addresses: variable (length-prefixed strings)]
//	[KeyType: 1 byte]
//	[PublicKey length: 2 bytes (optional)]
//	[PublicKey: variable (optional)]
//	[Signature: variable]
//
// # 使用示例
//
//	handler := addrmgmt.NewHandler(localID, addressBook, keyFactory)
//	scheduler := addrmgmt.NewScheduler(config, identity, addressBook, handler)
//	scheduler.SetNeighborFuncs(getNeighbors, openStream)
//	scheduler.Start(ctx)
//
// # 安全性
//
// 所有地址记录必须包含有效签名。Handler 会验证：
//   - 签名与 NodeID 对应的公钥匹配
//   - 序列号递增（防止重放攻击）
//   - 时间戳合理（防止过期记录）
package addrmgmt
