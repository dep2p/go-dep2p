// Package protocol 实现协议注册与路由
//
// # 核心功能
//
// 1. 协议注册表 (Registry)
//   - 管理协议 ID 与处理器的映射
//   - 支持精确匹配和模式匹配
//   - 线程安全的注册/注销
//
// 2. 协议路由器 (Router)
//   - 根据协议 ID 路由入站流
//   - 调用对应的协议处理器
//   - 支持通配符路由规则
//
// 3. 协议协商器 (Negotiator)
//   - 使用 multistream-select 协商协议
//   - 客户端模式：从列表中选择服务器支持的协议
//   - 服务器模式：等待客户端请求并响应
//
// 4. 系统协议
//   - Ping (/dep2p/sys/ping/1.0.0) - 存活检测和 RTT 测量
//   - Identify (/dep2p/sys/identify/1.0.0) - 节点身份信息交换
//
// # 快速开始
//
// 创建协议注册表：
//
//	registry := protocol.NewRegistry()
//
// 注册协议：
//
//	handler := func(stream pkgif.Stream) {
//	    // 处理协议逻辑
//	    defer stream.Close()
//	    // ...
//	}
//	registry.Register("/my/protocol/1.0.0", handler)
//
// 创建路由器：
//
//	negotiator := protocol.NewNegotiator(registry)
//	router := protocol.NewRouter(registry, negotiator)
//
// 路由入站流：
//
//	err := router.Route(stream)
//	if err != nil {
//	    log.Error("路由失败", err)
//	}
//
// 使用 Ping 协议：
//
//	import "github.com/dep2p/go-dep2p/internal/core/protocol/system/ping"
//
//	// 注册 Ping 协议
//	pingService := ping.NewService()
//	registry.Register(ping.ProtocolID, pingService.Handler)
//
//	// 主动 Ping 节点
//	rtt, err := ping.Ping(ctx, host, "peer-id")
//	fmt.Printf("RTT: %v\n", rtt)
//
// 使用 Identify 协议：
//
//	import "github.com/dep2p/go-dep2p/internal/core/protocol/system/identify"
//
//	// 注册 Identify 协议
//	idService := identify.NewService(host, registry)
//	registry.Register(identify.ProtocolID, idService.Handler)
//
//	// 主动识别节点
//	info, err := identify.Identify(ctx, host, "peer-id")
//	fmt.Printf("Peer: %s, Protocols: %v\n", info.PeerID, info.Protocols)
//
// # 协议分类
//
// 系统协议 (/dep2p/sys/*):
//   - /dep2p/sys/ping/1.0.0 - Ping
//   - /dep2p/sys/identify/1.0.0 - Identify
//   - /dep2p/sys/autonat/1.0.0 - AutoNAT (v1.1+)
//   - /dep2p/sys/holepunch/1.0.0 - HolePunch (v1.1+)
//   - /dep2p/sys/relay/2.0.0 - Relay (v1.1+)
//
// Realm 协议 (/dep2p/realm/<id>/*):
//   - 由 Realm 层定义
//
// 应用协议 (/dep2p/app/<id>/*):
//   - 由应用层定义
//
// # 协议协商流程
//
// 入站流处理：
//
//	1. 流到达
//	2. Router.Route(stream)
//	3. 从流获取协议 ID（stream.Protocol()）
//	4. Registry.GetHandler(protocolID)
//	5. 调用 handler(stream)
//
// multistream-select 协商（Negotiator）：
//
//	客户端模式（Negotiate）:
//	  1. 发送支持的协议列表
//	  2. 接收服务器选择的协议
//	  3. 返回协商的协议 ID
//
//	服务器模式（Handle）:
//	  1. 列出本地支持的协议
//	  2. 等待客户端选择
//	  3. 返回协商的协议 ID
//
// # 架构
//
// protocol 依赖：
//   - pkg/interfaces（接口定义）
//   - go-multistream（协议协商）
//
// protocol 被依赖：
//   - internal/core/host（集成协议路由）
//   - internal/core/swarm（处理入站流）
//
// # Ping 协议
//
// 协议 ID: /dep2p/sys/ping/1.0.0
// 消息格式: 32 字节随机数据
//
// 服务器端:
//   - 读取 32 字节
//   - 回显相同数据
//   - 循环处理
//
// 客户端:
//   - 生成 32 字节随机数据
//   - 发送并等待回显
//   - 计算 RTT
//
// # Identify 协议
//
// 协议 ID: /dep2p/sys/identify/1.0.0
// 消息格式: JSON
//
// 交换信息:
//   - PeerID（节点 ID）
//   - PublicKey（公钥）
//   - ListenAddrs（监听地址）
//   - ObservedAddr（观测地址）
//   - Protocols（支持的协议）
//   - AgentVersion（代理版本）
//   - ProtocolVersion（协议版本）
//
// # 注意事项
//
// 1. 线程安全: 所有 Registry 方法都是并发安全的
// 2. 处理器调用: handler(stream) 在新的 goroutine 中调用（由上层负责）
// 3. 流关闭: 处理器负责关闭流
// 4. 错误处理: 协商失败返回 ErrNegotiationFailed
//
// # 性能考虑
//
// 1. 注册操作: O(1) 时间复杂度
// 2. 获取处理器: O(1) 精确匹配，O(n) 模式匹配
// 3. 协商延迟: ~1-2 RTT（multistream-select）
//
// # 未来扩展
//
// 1. Protobuf 编码: Identify 消息使用 Protobuf（v1.1+）
// 2. Identify Push: 节点信息变更时主动推送（v1.1+）
// 3. AutoNAT: NAT 类型自动检测（v1.1+）
// 4. HolePunch: NAT 打洞协调（v1.1+）
// 5. Relay: Circuit Relay v2（v1.1+）
//
package protocol
