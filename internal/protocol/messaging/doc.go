// Package messaging 实现点对点消息传递协议
//
// 协议标识: /dep2p/app/<realmID>/messaging/1.0.0
//
// # 架构定位
//
// - 架构层: Protocol Layer (L4)
// - 公共接口: pkg/interfaces/messaging.go
// - Protobuf 定义: pkg/proto/messaging/
// - 依赖: internal/core/host, internal/realm
//
// # 核心功能
//
// messaging 提供以下核心功能:
//
//  1. 请求-响应模式 (Send) - 同步发送消息并等待响应
//  2. 异步发送 (SendAsync) - 异步发送消息,通过 channel 接收响应
//  3. 处理器注册 (RegisterHandler) - 注册协议处理器
//  4. Realm 集成 - 自动验证节点成员资格
//  5. 协议管理 - 自动构造协议 ID
//  6. 消息编解码 - 使用 Protobuf 编解码
//  7. 重试机制 - 自动重试失败的请求
//  8. 超时控制 - 支持请求超时
//
// # 使用示例
//
// ## 发送消息
//
//	// 创建服务
//	svc, err := messaging.New(host, realmMgr)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 启动服务
//	if err := svc.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//
//	// 同步发送消息
//	resp, err := svc.Send(ctx, peerID, "myprotocol", []byte("hello"))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("Response: %s\n", resp)
//
//	// 异步发送消息
//	respCh, err := svc.SendAsync(ctx, peerID, "myprotocol", []byte("hello"))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	resp := <-respCh
//	if resp.Error != nil {
//	    log.Fatal(resp.Error)
//	}
//
// ## 注册处理器
//
//	// 注册消息处理器
//	err = svc.RegisterHandler("myprotocol", func(ctx context.Context, req *interfaces.Request) (*interfaces.Response, error) {
//	    // 处理请求
//	    fmt.Printf("Received: %s from %s\n", req.Data, req.From)
//
//	    // 返回响应
//	    return &interfaces.Response{
//	        ID:   req.ID,
//	        From: host.ID(),
//	        Data: []byte("world"),
//	    }, nil
//	})
//
// # 配置选项
//
// 服务支持以下配置选项:
//
//	svc, err := messaging.New(
//	    host,
//	    realmMgr,
//	    messaging.WithTimeout(10*time.Second),  // 设置超时
//	    messaging.WithMaxRetries(5),            // 设置最大重试次数
//	    messaging.WithRetryDelay(time.Second),  // 设置重试延迟
//	)
//
// # 协议格式
//
// 协议 ID 格式: /dep2p/app/<realmID>/<protocol>/1.0.0
//
// 消息格式使用 Protobuf (pkg/proto/messaging/messaging.proto):
//   - Request: ID, From, Protocol, Data, Timestamp, Metadata
//   - Response: ID, From, Data, Error, Timestamp, Latency, Metadata
//
// # Fx 集成
//
// 使用 Fx 模块:
//
//	app := fx.New(
//	    host.Module,
//	    realm.Module,
//	    messaging.Module,  // 自动注册服务
//	)
//
// # 错误处理
//
// 服务定义了以下错误:
//   - ErrNotStarted: 服务未启动
//   - ErrAlreadyStarted: 服务已启动
//   - ErrInvalidProtocol: 无效的协议格式
//   - ErrNotRealmMember: 节点不是 Realm 成员
//   - ErrHandlerNotFound: 处理器未找到
//   - ErrTimeout: 请求超时
//   - ErrStreamClosed: 流已关闭
//   - ErrInvalidMessage: 无效的消息格式
//
// # 性能特性
//
//   - 消息延迟: < 100ms (局域网)
//   - 吞吐量: > 1000 msg/s
//   - 并发安全: 所有方法均为并发安全
//   - 自动重试: 网络错误自动重试(最多3次)
//   - 流复用: 复用 Host 提供的流多路复用
//
// # 相关文档
//
//   - 设计文档: design/03_architecture/L6_domains/protocol_messaging/
//   - 接口定义: pkg/interfaces/messaging.go
//   - Protobuf 定义: pkg/proto/messaging/messaging.proto
//
package messaging
