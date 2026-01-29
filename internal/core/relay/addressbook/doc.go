// Package addressbook 实现 Realm 成员地址簿
//
// 地址簿是"仅 ID 连接"能力的核心组件，用于存储和管理 Realm 成员的地址信息。
// 地址簿为 Realm 成员提供地址发现服务。
//
// # 重要更新 (v1.1.0+)
//
// 从 v1.1.0 开始，AddressBook 统一使用 BadgerDB 持久化存储，不再提供内存模式。
// 创建 AddressBook 时必须提供存储引擎。
//
// # 核心组件
//
//   - MemberAddressBook: 聚合根，封装业务逻辑
//   - BadgerStore: BadgerDB 持久化存储实现
//   - EntryToProto/EntryFromProto: 协议转换工具
//
// # 使用示例
//
//	// 创建地址簿（使用存储引擎）
//	book, err := addressbook.NewWithEngine(realmID, storageEngine)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 注册成员（使用 internal/realm/interfaces.MemberEntry）
//	entry := realmif.MemberEntry{
//	    NodeID:      nodeID,
//	    DirectAddrs: addrs,
//	    NATType:     types.NATTypeFullCone,
//	}
//	book.Register(ctx, entry)
//
//	// 查询成员
//	entry, err := book.Query(ctx, targetNodeID)
package addressbook
