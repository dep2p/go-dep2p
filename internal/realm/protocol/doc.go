// Package protocol 实现 Realm 协议处理器
//
// 本包提供 Realm 级别的网络协议实现，包括：
//   - 认证协议（auth）：成员身份验证
//   - 加入协议（join）：新成员加入 Realm
//   - 同步协议（sync）：成员列表同步
//
// # 架构设计
//
// 协议层复用 auth/member 等包的核心业务逻辑，只负责协议封装：
//   - 注册协议到 Host
//   - 处理流的读写
//   - 转发到业务层处理
//
// # 协议 ID 格式
//
// 所有协议 ID 包含 RealmID，确保协议级别隔离：
//
//	/dep2p/realm/<realmID>/auth/1.0.0    - 认证协议
//	/dep2p/realm/<realmID>/join/1.0.0    - 加入协议
//	/dep2p/realm/<realmID>/sync/1.0.0    - 同步协议
//
// # 使用示例
//
//	// 创建认证处理器
//	authHandler := protocol.NewAuthHandler(host, realmID, authKey, authenticator, nil)
//
//	// 启动处理器（注册协议）
//	err := authHandler.Start(ctx)
//
//	// 向远程节点发起认证
//	err = authHandler.Authenticate(ctx, peerID)
//
//	// 停止处理器
//	err = authHandler.Stop(ctx)
//
// # 设计原则
//
//  1. 复用而非重复：复用 auth/challenge.go 的认证逻辑
//  2. 单一职责：协议层只负责网络通信封装
//  3. 接口驱动：依赖 interfaces.Host 等抽象接口
//
// # 相关文档
//
//   - 设计文档：design/02_constraints/protocol/L4_application/realm.md
//   - 认证逻辑：internal/realm/auth/
//   - 成员管理：internal/realm/member/
package protocol
