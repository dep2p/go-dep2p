// Package pubsub 实现发布订阅协议
//
// 协议标识: /dep2p/app/<realmID>/pubsub/1.0.0
//
// # 架构定位
//
// - 架构层: Protocol Layer (L4)
// - 公共接口: pkg/interfaces/pubsub.go
// - Protobuf 定义: pkg/proto/gossipsub/
// - 依赖: internal/core/host, internal/realm
//
// # 核心功能
//
// pubsub 提供以下核心功能:
//
//  1. 主题管理 (Join) - 加入主题
//  2. 消息发布 (Publish) - 发布消息到主题
//  3. 消息订阅 (Subscribe) - 订阅主题消息
//  4. 事件处理 (EventHandler) - 监听节点加入/离开事件
//  5. GossipSub 协议 - 基于 Mesh 的消息传播
//  6. Mesh 管理 - 自动维护 D-regular graph
//  7. 消息验证 - Realm 成员验证,消息去重
//  8. 心跳机制 - 周期性维护 Mesh
//
// # 使用示例
//
// ## 发布订阅
//
//	// 创建服务
//	svc, err := pubsub.New(host, realmMgr)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 启动服务
//	if err := svc.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//
//	// 加入主题
//	topic, err := svc.Join("my-topic")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 订阅主题
//	sub, err := topic.Subscribe()
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 发布消息
//	err = topic.Publish(ctx, []byte("hello"))
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 接收消息
//	msg, err := sub.Next(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Received: %s from %s\n", msg.Data, msg.From)
//
// ## 事件处理
//
//	// 监听节点事件
//	handler, err := topic.EventHandler()
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	for {
//	    event, err := handler.NextPeerEvent(ctx)
//	    if err != nil {
//	        break
//	    }
//	    
//	    switch event.Type {
//	    case interfaces.PeerJoin:
//	        fmt.Printf("Peer joined: %s\n", event.Peer)
//	    case interfaces.PeerLeave:
//	        fmt.Printf("Peer left: %s\n", event.Peer)
//	    }
//	}
//
// # 配置选项
//
// 服务支持以下配置选项:
//
//	svc, err := pubsub.New(
//	    host,
//	    realmMgr,
//	    pubsub.WithHeartbeatInterval(time.Second),  // 心跳间隔
//	    pubsub.WithMeshDegree(6, 4, 12),           // Mesh 度数
//	    pubsub.WithMaxMessageSize(1<<20),          // 最大消息 1MB
//	)
//
// # GossipSub 协议
//
// 本实现遵循 GossipSub v1.1 规范:
//
//   - Mesh: D-regular graph (默认 D=6)
//   - GRAFT: 加入 Mesh
//   - PRUNE: 离开 Mesh
//   - IHAVE/IWANT: 消息传播优化
//   - 心跳: 1秒间隔,维护 Mesh 健康
//
// # Fx 集成
//
// 使用 Fx 模块:
//
//	app := fx.New(
//	    host.Module,
//	    realm.Module,
//	    pubsub.Module,  // 自动注册服务
//	)
//
// # 错误处理
//
// 服务定义了以下错误:
//   - ErrNotStarted: 服务未启动
//   - ErrAlreadyStarted: 服务已启动
//   - ErrTopicNotFound: 主题未找到
//   - ErrTopicAlreadyJoined: 主题已加入
//   - ErrTopicClosed: 主题已关闭
//   - ErrSubscriptionCancelled: 订阅已取消
//   - ErrInvalidMessage: 无效消息
//   - ErrMessageTooLarge: 消息过大
//   - ErrNotRealmMember: 非 Realm 成员
//   - ErrDuplicateMessage: 重复消息
//
// # 性能特性
//
//   - 消息延迟: < 200ms (局域网)
//   - 吞吐量: > 1000 msg/s
//   - Mesh 度数: 6 (可配置)
//   - 消息去重: LRU 缓存 + TTL
//   - 并发安全: 所有方法并发安全
//
// # 相关文档
//
//   - 设计文档: design/03_architecture/L6_domains/protocol_pubsub/
//   - 接口定义: pkg/interfaces/pubsub.go
//   - Protobuf 定义: pkg/proto/gossipsub/gossipsub.proto
//
package pubsub
