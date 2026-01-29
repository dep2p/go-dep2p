// Package connector 实现 Realm 内"仅 ID 连接"能力
//
// 本包提供地址解析和连接建立功能，支持用户仅通过 NodeID 连接 Realm 成员，
// 系统自动完成地址解析和连接建立（直连 → 打洞 → Relay 保底）。
//
// # 核心组件
//
//   - AddressResolver: 多源地址解析器，按优先级从 Peerstore 和 AddressBook 解析地址
//   - Connector: 连接器，协调直连、打洞和 Relay 三级连接策略
//
// # 地址解析优先级
//
//  1. Peerstore 本地缓存（最快）
//  2. Relay 地址簿查询（网络请求）
//  3. 返回空（后续走 Relay 保底）
//
// # 连接优先级
//
//  1. 直连：有地址时直接连接
//  2. 打洞：直连失败时尝试 NAT 穿透
//  3. Relay：保底方案，确保总是可达
//
// # 使用示例
//
//	// 通过 Realm 接口使用
//	conn, err := realm.Connect(ctx, targetNodeID)
//	if err != nil {
//	    // 处理错误
//	}
//	defer conn.Close()
//
//	// 使用连接
//	stream, err := conn.NewStream(ctx)
package connector
