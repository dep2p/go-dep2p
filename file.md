# dep2p 项目目录结构

📁 config        // 配置相关的包
├── 📄 config.go        // 基础配置定义，包含所有dep2p节点的核心配置选项
├── 📄 host.go          // 主机配置，定义了dep2p主机的配置参数
├── 📄 log.go           // 日志配置，定义日志系统的配置选项
└── 📄 quic.go          // QUIC 传输协议配置，包含QUIC协议特定的配置参数

📁 core             // 核心功能包
├── 📁 canonicallog           // 规范化日志包
│   └── 📄 canonicallog.go   // 规范化日志实现，提供标准日志格式
├── 📁 connmgr         // 连接管理器包
│   ├── 📄 decay.go         // 连接衰减算法实现
│   ├── 📄 gater.go         // 连接控制器实现
│   ├── 📄 manager.go       // 连接管理器核心实现
│   ├── 📄 null.go          // 空连接管理器实现
│   └── 📄 presets.go       // 预设连接管理配置
├── 📁 control        // 控制功能包
│   └── 📄 disconnect.go    // 处理节点断开连接的逻辑
├── 📁 crypto         // 加密相关功能包
│   ├── 📄 ecdsa.go         // ECDSA加密算法实现
│   ├── 📄 ed25519.go       // Ed25519加密算法实现
│   ├── 📄 key.go           // 密钥管理核心实现
│   ├── 📄 key_to_stdlib.go // 密钥转换为标准库格式
│   ├── 📁 pb               // Protocol Buffers定义
│   │   ├── 📄 crypto.pb.go   // 加密相关的PB生成代码
│   │   ├── 📄 crypto.proto   // 加密相关的Protocol Buffers定义
│   ├── 📄 rsa_common.go    // RSA通用功能实现
│   ├── 📄 rsa_go.go        // RSA Go语言特定实现
│   └── 📄 secp256k1.go     // Secp256k1椭圆曲线加密实现
├── 📁 discovery      // 节点发现功能包
│   ├── 📄 discovery.go    // 节点发现核心实现
│   ├── 📄 options.go      // 发现功能的配置选项
├── 📁 event         // 事件系统包
│   ├── 📄 addrs.go        // 地址相关事件处理
│   ├── 📄 bus.go          // 事件总线实现
│   ├── 📄 dht.go          // DHT相关事件处理
│   ├── 📄 doc.go          // 包文档说明
│   ├── 📄 identify.go     // 身份识别事件处理
│   ├── 📄 nattype.go      // NAT类型事件处理
│   ├── 📄 network.go      // 网络事件处理
│   ├── 📄 protocol.go     // 协议相关事件处理
│   └── 📄 reachability.go // 可达性事件处理
├── 📁 host          // 主机功能包
│   ├── 📄 helpers.go      // 主机辅助函数
│   ├── 📄 host.go         // 主机核心实现
├── 📁 internal      // 内部功能包
│   ├── 📁 catch          // 错误捕获功能
│   │   └── 📄 catch.go    // 错误捕获实现
├── 📁 metrics       // 指标统计包
│   ├── 📄 bandwidth.go    // 带宽统计实现
│   └── 📄 reporter.go     // 指标报告器实现
├── 📁 network       // 网络功能包
│   ├── 📄 conn.go         // 连接实现
│   ├── 📄 context.go      // 上下文管理
│   ├── 📄 errors.go       // 错误定义
│   ├── 📁 mocks           // 模拟测试实现
│   │   ├── 📄 mock_conn_management_scope.go    // 连接管理作用域模拟
│   │   ├── 📄 mock_peer_scope.go               // 对等节点作用域模拟
│   │   ├── 📄 mock_protocol_scope.go           // 协议作用域模拟
│   │   ├── 📄 mock_resource_manager.go         // 资源管理器模拟
│   │   ├── 📄 mock_resource_scope_span.go      // 资源作用域跨度模拟
│   │   ├── 📄 mock_stream_management_scope.go   // 流管理作用域模拟
│   │   └── 📄 network.go                       // 网络模拟
│   ├── 📄 mux.go          // 多路复用实现
│   ├── 📄 nattype.go      // NAT类型处理
│   ├── 📄 network.go      // 网络核心实现
│   ├── 📄 notifee.go      // 网络事件通知
│   ├── 📄 rcmgr.go        // 资源管理器
│   └── 📄 stream.go       // 流处理实现
├── 📁 peer          // 对等节点管理包
│   ├── 📄 addrinfo.go      // 节点地址信息处理
│   ├── 📄 addrinfo_serde.go // 节点地址信息序列化/反序列化
│   ├── 📁 pb               // Protocol Buffers定义
│   │   ├── 📄 peer_record.pb.go  // 节点记录PB生成代码
│   │   └── 📄 peer_record.proto  // 节点记录协议定义
│   ├── 📄 peer.go          // 对等节点核心实现
│   ├── 📄 peer_serde.go    // 节点信息序列化处理
│   └── 📄 record.go        // 节点记录实现
├── 📁 peerstore     // 对等节点存储包
│   ├── 📄 helpers.go       // 存储辅助函数
│   └── 📄 peerstore.go     // 节点存储核心实现
├── 📁 pnet          // 私有网络功能包
│   ├── 📄 codec.go         // 私有网络编解码器
│   ├── 📄 env.go           // 环境变量处理
│   ├── 📄 error.go         // 错误定义
│   └── 📄 protector.go     // 网络保护器实现
├── 📁 protocol      // 协议定义包
│   ├── 📄 id.go            // 协议ID定义和管理
│   ├── 📄 switch.go        // 协议切换实现
├── 📁 record        // 记录管理包
│   ├── 📄 envelope.go      // 记录封装实现
│   ├── 📁 pb              // Protocol Buffers定义
│   │   ├── 📄 envelope.pb.go  // 记录封装PB生成代码
│   │   └── 📄 envelope.proto  // 记录封装协议定义
│   └── 📄 record.go        // 记录核心实现
├── 📁 routing       // 路由功能包
│   ├── 📄 options.go       // 路由选项配置
│   ├── 📄 query.go         // 路由查询实现
│   ├── 📄 query_serde.go   // 查询序列化处理
│   └── 📄 routing.go       // 路由核心实现
├── 📁 sec           // 安全功能包
│   ├── 📁 insecure        // 不安全传输实现
│   │   ├── 📄 insecure.go    // 不安全传输核心实现
│   │   └── 📁 pb             // Protocol Buffers定义
│   │       ├── 📄 plaintext.pb.go  // 明文传输PB生成代码
│   │       └── 📄 plaintext.proto  // 明文传输协议定义
│   └── 📄 security.go      // 安全功能核心实现
├── 📁 transport     // 传输层包
│   └── 📄 transport.go     // 传输层核心实现
└── 📄 alias.go            // 类型别名定义

📁 p2p           // P2P网络实现包
├── 📁 discovery     // 节点发现实现包
│   ├── 📁 backoff       // 退避算法实现
│   │   ├── 📄 backoff.go       // 退避算法核心实现
│   │   ├── 📄 backoffcache.go  // 退避缓存管理
│   │   └── 📄 backoffconnector.go // 带退避机制的连接器
│   ├── 📁 mdns          // mDNS服务发现
│   │   └── 📄 mdns.go         // mDNS发现实现
│   ├── 📁 mocks         // 模拟测试
│   │   └── 📄 mocks.go        // 发现功能模拟实现
│   ├── 📁 routing       // 路由发现
│   │   └── 📄 routing.go      // 基于路由的发现实现
│   └── 📁 util          // 工具函数
│       └── 📄 util.go         // 发现相关工具函数
├── 📁 host          // 主机实现包
│   ├── 📁 autonat       // NAT自动检测
│   │   ├── 📄 autonat.go      // NAT自动检测核心实现
│   │   ├── 📄 client.go       // NAT客户端实现
│   │   ├── 📄 dialpolicy.go   // 拨号策略实现
│   │   ├── 📄 interface.go    // 接口定义
│   │   ├── 📄 metrics.go      // 指标统计
│   │   ├── 📄 notify.go       // 事件通知
│   │   ├── 📄 options.go      // 配置选项
│   │   ├── 📁 pb              // Protocol Buffers定义
│   │   │   ├── 📄 autonat.pb.go  // NAT自动检测PB生成代码
│   │   │   └── 📄 autonat.proto  // NAT自动检测协议定义
│   │   ├── 📄 proto.go        // 协议实现
│   │   └── 📄 svc.go          // 服务实现
│   ├── 📁 autorelay     // 自动中继
│   │   ├── 📄 addrsplosion.go // 地址扩展处理
│   │   ├── 📄 autorelay.go    // 自动中继核心实现
│   │   ├── 📄 metrics.go      // 指标统计
│   │   ├── 📄 options.go      // 配置选项
│   │   ├── 📄 relay.go        // 中继功能实现
│   │   └── 📄 relay_finder.go // 中继节点发现
│   ├── 📁 basic         // 基础主机实现
│   │   ├── 📄 basic_host.go   // 基础主机核心实现
│   │   ├── 📁 internal       // 内部功能
│   │   │   └── 📁 backoff    // 退避算法
│   │   │       └── 📄 backoff.go // 主机退避实现
│   │   ├── 📄 mocks.go       // 模拟测试
│   │   └── 📄 natmgr.go      // NAT管理器
│   ├── 📁 blank         // 空白主机实现
│   │   └── 📄 blank.go       // 用于测试的空白主机
│   ├── 📁 eventbus      // 事件总线
│   │   ├── 📄 basic.go       // 基础事件总线实现
│   │   ├── 📄 basic_metrics.go // 事件指标统计
│   │   └── 📄 opts.go        // 事件总线选项
│   ├── 📁 peerstore     // 节点存储实现
│   │   ├── 📄 metrics.go      // 存储指标统计
│   │   ├── 📄 peerstore.go    // 节点存储实现
│   │   ├── 📁 pstoreds       // 持久化存储实现
│   │   │   ├── 📄 addr_book.go    // 地址簿实现
│   │   │   ├── 📄 addr_book_gc.go // 地址簿垃圾回收
│   │   │   ├── 📄 cache.go        // 缓存实现
│   │   │   ├── 📄 cyclic_batch.go // 循环批处理
│   │   │   ├── 📄 deprecate.go    // 废弃功能处理
│   │   │   ├── 📄 keybook.go      // 密钥簿实现
│   │   │   ├── 📄 metadata.go     // 元数据管理
│   │   │   ├── 📁 pb             // Protocol Buffers定义
│   │   │   │   ├── 📄 pstore.pb.go  // 存储PB生成代码
│   │   │   │   └── 📄 pstore.proto  // 存储协议定义
│   │   │   ├── 📄 peerstore.go    // 持久化存储核心实现
│   │   │   └── 📄 protobook.go    // 协议簿实现
│   │   ├── 📁 pstoremem      // 内存存储实现
│   │   │   ├── 📄 addr_book.go    // 内存地址簿实现
│   │   │   ├── 📄 keybook.go      // 内存密钥簿实现
│   │   │   ├── 📄 metadata.go     // 内存元数据管理
│   │   │   ├── 📄 peerstore.go    // 内存存储核心实现
│   │   │   ├── 📄 protobook.go    // 内存协议簿实现
│   │   │   └── 📄 sorting.go      // 排序功能实现
│   │   ├── 📁 pstoremanager  // 存储管理器
│   │   │   └── 📄 pstoremanager.go // 存储管理器实现
│   │   ├── 📁 relaysvc      // 中继服务
│   │   │   └── 📄 relay.go        // 中继服务实现
│   │   ├── 📁 resource-manager  // 资源管理器
│   │   │   ├── 📄 allowlist.go     // 白名单实现
│   │   │   ├── 📄 conn_limiter.go  // 连接限制器
│   │   │   ├── 📄 error.go         // 错误定义
│   │   │   ├── 📄 extapi.go        // 扩展API
│   │   │   ├── 📄 limit.go         // 限制实现
│   │   │   ├── 📄 limit_defaults.go // 默认限制配置
│   │   │   ├── 📄 metrics.go       // 资源指标统计
│   │   │   ├── 📁 obs             // 观察器
│   │   │   │   └── 📄 obs.go       // 观察器实现
│   │   │   ├── 📄 rcmgr.go        // 资源管理器核心实现
│   │   │   ├── 📄 scope.go        // 资源作用域
│   │   │   ├── 📄 stats.go        // 统计功能
│   │   │   ├── 📄 sys_not_unix.go // 非Unix系统实现
│   │   │   ├── 📄 sys_unix.go     // Unix系统实现
│   │   │   ├── 📄 sys_windows.go  // Windows系统实现
│   │   │   └── 📄 trace.go        // 跟踪功能
│   │   ├── 📁 routed        // 路由主机
│   │   │   └── 📄 routed.go       // 路由主机实现
├── 📁 http          // HTTP功能包
│   ├── 📁 auth          // HTTP认证
│   │   ├── 📄 auth.go        // 认证核心实现
│   │   ├── 📄 client.go      // 客户端认证
│   │   ├── 📁 internal       // 内部实现
│   │   │   ├── 📁 handshake  // 握手实现
│   │   │   │   ├── 📄 client.go  // 客户端握手
│   │   │   │   ├── 📄 handshake.go // 握手核心实现
│   │   │   │   └── 📄 server.go  // 服务端握手
│   │   └── 📄 server.go      // 服务端认证
│   ├── 📄 dep2phttp.go  // dep2p HTTP实现
│   ├── 📄 options.go     // HTTP选项配置
│   └── 📁 ping          // HTTP ping实现
│       └── 📄 ping.go        // ping功能实现
├── 📁 metricshelper  // 指标助手包
│   ├── 📄 conn.go         // 连接指标
│   ├── 📄 dir.go          // 目录指标
│   ├── 📄 pool.go         // 池化指标
│   └── 📄 registerer.go   // 指标注册器
├── 📁 muxer         // 多路复用器包
│   └── 📁 yamux         // yamux多路复用实现
│       ├── 📄 conn.go        // yamux连接
│       ├── 📄 stream.go      // yamux流
│       └── 📄 transport.go   // yamux传输
├── 📁 net           // 网络功能包
│   ├── 📁 gostream      // Go流实现
│   │   ├── 📄 addr.go        // 地址实现
│   │   ├── 📄 conn.go        // 连接实现
│   │   ├── 📄 gostream.go    // Go流核心实现
│   │   └── 📄 listener.go    // 监听器实现
│   ├── 📁 mock          // 模拟网络实现
│   │   ├── 📄 complement.go   // 补充功能
│   │   ├── 📄 interface.go    // 接口定义
│   │   ├── 📄 mock.go         // 模拟核心实现
│   │   ├── 📄 mock_conn.go    // 模拟连接
│   │   ├── 📄 mock_link.go    // 模拟链路
│   │   ├── 📄 mock_net.go     // 模拟网络
│   │   ├── 📄 mock_peernet.go // 模拟对等网络
│   │   ├── 📄 mock_printer.go // 模拟打印器
│   │   └── 📄 ratelimiter.go  // 速率限制器
│   ├── 📁 nat           // NAT功能实现
│   │   └── 📄 nat.go         // NAT核心实现
│   ├── 📁 pnet          // 私有网络实现
│   │   ├── 📄 protector.go   // 网络保护器
│   │   └── 📄 psk_conn.go    // PSK连接实现
│   ├── 📁 reuseport     // 端口重用
│   │   ├── 📄 dial.go        // 拨号实现
│   │   ├── 📄 dialer.go      // 拨号器实现
│   │   ├── 📄 listen.go      // 监听实现
│   │   ├── 📄 reuseport.go   // 端口重用核心实现
│   │   ├── 📄 reuseport_plan9.go // Plan9系统实现
│   │   ├── 📄 reuseport_posix.go // POSIX系统实现
│   │   └── 📄 transport.go   // 传输实现
│   ├── 📁 swarm         // Swarm网络实现
│   │   ├── 📄 black_hole_detector.go // 黑洞检测器
│   │   ├── 📄 clock.go       // 时钟实现
│   │   ├── 📄 connectedness_event_emitter.go // 连接事件发射器
│   │   ├── 📄 dial_error.go  // 拨号错误处理
│   │   ├── 📄 dial_ranker.go // 拨号排序器
│   │   ├── 📄 dial_sync.go   // 拨号同步
│   │   ├── 📄 dial_worker.go // 拨号工作器
│   │   ├── 📄 limiter.go     // 限制器实现
│   │   ├── 📄 swarm.go       // Swarm核心实现
│   │   ├── 📄 swarm_addr.go  // Swarm地址处理
│   │   ├── 📄 swarm_conn.go  // Swarm连接处理
│   │   ├── 📄 swarm_dial.go  // Swarm拨号处理
│   │   ├── 📄 swarm_listen.go // Swarm监听处理
│   │   ├── 📄 swarm_metrics.go // Swarm指标统计
│   │   ├── 📄 swarm_stream.go // Swarm流处理
│   │   └── 📄 swarm_transport.go // Swarm传输处理
│   └── 📁 upgrader      // 连接升级器
│       ├── 📄 conn.go        // 连接升级
│       ├── 📄 listener.go    // 监听器升级
│       ├── 📄 threshold.go   // 阈值处理
│       └── 📄 upgrader.go    // 升级器核心实现
├── 📁 protocol      // 协议实现包
│   ├── 📁 autonatv2     // 自动NAT v2实现
│   │   ├── 📄 autonat.go      // NAT自动检测核心实现
│   │   ├── 📄 client.go       // 客户端实现
│   │   ├── 📄 metrics.go      // 指标统计
│   │   ├── 📄 msg_reader.go   // 消息读取器
│   │   ├── 📄 options.go      // 配置选项
│   │   ├── 📁 pb             // Protocol Buffers定义
│   │   │   ├── 📄 autonatv2.pb.go  // NAT v2 PB生成代码
│   │   │   └── 📄 autonatv2.proto  // NAT v2协议定义
│   │   └── 📄 server.go      // 服务端实现
│   ├── 📁 circuitv2     // 电路v2实现
│   │   ├── 📁 pb            // Protocol Buffers定义
│   │   │   ├── 📄 circuit.pb.go   // 电路PB生成代码
│   │   │   ├── 📄 circuit.proto   // 电路协议定义
│   │   │   ├── 📄 voucher.pb.go   // 凭证PB生成代码
│   │   │   └── 📄 voucher.proto   // 凭证协议定义
│   │   ├── 📁 proto         // 协议实现
│   │   │   ├── 📄 protocol.go    // 协议核心实现
│   │   │   └── 📄 voucher.go     // 凭证处理
│   │   ├── 📁 relay         // 中继实现
│   │   │   ├── 📄 acl.go         // 访问控制列表
│   │   │   ├── 📄 constraints.go  // 约束条件
│   │   │   ├── 📄 metrics.go      // 指标统计
│   │   │   ├── 📄 options.go      // 配置选项
│   │   │   ├── 📄 relay.go        // 中继核心实现
│   │   │   └── 📄 resources.go    // 资源管理
│   │   └── 📁 util          // 工具函数
│   │       ├── 📄 io.go          // IO操作
│   │       └── 📄 pbconv.go      // PB转换工具
│   ├── 📁 holepunch     // 打洞协议实现
│   │   ├── 📄 filter.go       // 过滤器实现
│   │   ├── 📄 holepuncher.go  // 打洞器实现
│   │   ├── 📄 metrics.go      // 指标统计
│   │   ├── 📁 pb             // Protocol Buffers定义
│   │   │   ├── 📄 holepunch.pb.go  // 打洞PB生成代码
│   │   │   └── 📄 holepunch.proto  // 打洞协议定义
│   │   ├── 📄 svc.go          // 服务实现
│   │   ├── 📄 tracer.go       // 跟踪器实现
│   │   └── 📄 util.go         // 工具函数
│   ├── 📁 identify      // 身份识别协议
│   │   ├── 📄 id.go           // 身份识别核心实现
│   │   ├── 📄 metrics.go      // 指标统计
│   │   ├── 📄 nat_emitter.go  // NAT事件发射器
│   │   ├── 📄 obsaddr.go      // 观察地址处理
│   │   ├── 📄 opts.go         // 配置选项
│   │   ├── 📁 pb             // Protocol Buffers定义
│   │   │   ├── 📄 identify.pb.go  // 身份识别PB生成代码
│   │   │   └── 📄 identify.proto  // 身份识别协议定义
│   │   └── 📄 user_agent.go   // 用户代理处理
│   └── 📁 ping          // Ping协议实现
│       └── 📄 ping.go         // Ping功能实现
├── 📁 security      // 安全实现包
│   ├── 📁 noise         // Noise协议实现
│   │   ├── 📄 crypto.go       // 加密功能
│   │   ├── 📄 handshake.go    // 握手协议
│   │   ├── 📁 pb             // Protocol Buffers定义
│   │   │   ├── 📄 payload.pb.go  // 负载PB生成代码
│   │   │   └── 📄 payload.proto  // 负载协议定义
│   │   ├── 📄 rw.go           // 读写实现
│   │   ├── 📄 session.go      // 会话管理
│   │   ├── 📄 session_transport.go // 会话传输
│   │   └── 📄 transport.go    // 传输实现
│   └── 📁 tls           // TLS实现
│       ├── 📄 conn.go         // TLS连接
│       ├── 📄 crypto.go       // 加密功能
│       ├── 📄 extension.go    // 扩展功能
│       └── 📄 transport.go    // 传输实现
├── 📁 transport     // 传输协议实现包
│   ├── 📁 quic          // QUIC传输实现
│   │   ├── 📄 conn.go         // QUIC连接
│   │   ├── 📄 listener.go     // QUIC监听器
│   │   ├── 📄 stream.go       // QUIC流
│   │   ├── 📄 transport.go    // QUIC传输
│   │   └── 📄 virtuallistener.go // 虚拟监听器
│   ├── 📁 quicreuse     // QUIC端口重用
│   │   ├── 📄 config.go       // 配置实现
│   │   ├── 📄 connmgr.go      // 连接管理
│   │   ├── 📄 listener.go     // 监听器实现
│   │   ├── 📄 nonquic_packetconn.go // 非QUIC包连接
│   │   ├── 📄 options.go      // 配置选项
│   │   ├── 📄 quic_multiaddr.go // QUIC多地址
│   │   ├── 📄 reuse.go        // 重用实现
│   │   └── 📄 tracer.go       // 跟踪器
│   ├── 📁 tcp           // TCP传输实现
│   │   ├── 📄 metrics.go       // TCP指标统计
│   │   ├── 📄 metrics_darwin.go // Darwin系统指标
│   │   ├── 📄 metrics_general.go // 通用指标
│   │   ├── 📄 metrics_linux.go  // Linux系统指标
│   │   ├── 📄 metrics_none.go   // 无指标实现
│   │   └── 📄 tcp.go           // TCP核心实现
│   ├── 📁 websocket     // WebSocket传输实现
│   │   ├── 📄 addrs.go        // 地址处理
│   │   ├── 📄 conn.go         // WebSocket连接
│   │   ├── 📄 listener.go     // WebSocket监听器
│   │   └── 📄 websocket.go    // WebSocket核心实现
│   └── 📁 webtransport  // WebTransport传输实现
│       ├── 📄 cert_manager.go  // 证书管理
│       ├── 📄 conn.go         // WebTransport连接
│       ├── 📄 crypto.go       // 加密功能
│       ├── 📄 listener.go     // WebTransport监听器
│       ├── 📄 multiaddr.go    // 多地址支持
│       ├── 📄 noise_early_data.go // Noise早期数据
│       ├── 📄 stream.go       // WebTransport流
│       └── 📄 transport.go    // WebTransport核心实现

// 根目录文件
📄 defaults.go       // 默认配置定义
📄 dep2p.go        // dep2p主入口文件
📄 limits.go        // 资源限制定义
📄 options.go       // 全局选项配置
📄 options_filter.go // 选项过滤器实现
📄 tools.go         // 工具函数集合