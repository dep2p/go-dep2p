// Package mdns 实现局域网多播 DNS 节点发现
//
// mdns 使用 mDNS (Multicast DNS) 协议进行局域网内的节点自动发现，
// 无需中心服务器或互联网连接。
//
// # 核心功能
//
// 1. 服务广播 (Advertise)
//   - 使用 zeroconf 库注册 mDNS 服务
//   - TXT 记录存储节点的 multiaddr
//   - 定期广播节点信息
//
// 2. 服务发现 (FindPeers)
//   - 监听局域网内的 mDNS 广播
//   - 解析 TXT 记录提取 multiaddr
//   - 通过 channel 返回发现的节点
//
// 3. 地址过滤
//   - 只广播适合 LAN 的地址（IP4/IP6, .local DNS）
//   - 排除不适合的协议（circuit relay, websocket, webrtc）
//   - 符合 RFC 6762 数据包大小限制（1500 字节）
//
// # 使用示例
//
//	// 创建 MDNS 服务
//	config := mdns.DefaultConfig()
//	mdns, err := mdns.New(host, config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// 启动服务（Server + Resolver）
//	ctx := context.Background()
//	if err := mdns.Start(ctx); err != nil {
//	    log.Fatal(err)
//	}
//	defer mdns.Stop(ctx)
//
//	// 发现节点
//	peerCh, err := mdns.FindPeers(ctx, "my-namespace")
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	for peer := range peerCh {
//	    fmt.Printf("发现节点: %s, 地址: %v\n", peer.ID, peer.Addrs)
//	}
//
//	// 广播自身
//	ttl, err := mdns.Advertise(ctx, "my-namespace")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("广播 TTL: %v\n", ttl)
//
// # 架构说明
//
// mdns 采用 Server + Resolver 双组件架构：
//
//   - Server: 使用 zeroconf.RegisterProxy() 注册服务，广播节点信息
//   - Resolver: 使用 zeroconf.Browse() 监听网络，发现其他节点
//   - Notifee: 内部回调，将发现的节点推送到 channel
//   - 地址过滤: isSuitableForMDNS() 确保只广播适合 LAN 的地址
//
// # 配置参数
//
//   - ServiceTag: mDNS 服务标签，默认 "_dep2p._udp"
//   - Interval: 广播间隔，默认 10s
//   - Enabled: 是否启用，默认 true
//
// # v1.0 范围
//
// ✅ Server 广播（Advertise）
// ✅ Resolver 发现（FindPeers）
// ✅ 地址过滤（isSuitableForMDNS）
// ✅ Discovery 接口实现
// ✅ 并发安全（atomic.Bool + sync.RWMutex）
//
// # 技术债
//
// v1.0 完整实现，无技术债项。
//
// 如需高级功能，可在 v1.1 添加：
//   - TD-MDNS-001: TTL 控制（自定义广播 TTL）
//   - TD-MDNS-002: 查询间隔（Interval 参数实际使用）
//   - TD-MDNS-003: 多播分组（支持多个 ServiceTag）
//
// # 并发安全
//
// mdns 是并发安全的：
//   - atomic.Bool 保护 started/closed 状态
//   - sync.RWMutex 保护 server 实例
//   - sync.WaitGroup 同步 goroutine
//   - context.Context 控制生命周期
//
// # Discovery 接口
//
// mdns 实现 pkg/interfaces/discovery.go 的 Discovery 接口：
//
//	type Discovery interface {
//	    FindPeers(ctx, ns string, opts ...) (<-chan types.PeerInfo, error)
//	    Advertise(ctx, ns string, opts ...) (time.Duration, error)
//	    Start(ctx context.Context) error
//	    Stop(ctx context.Context) error
//	}
//
// # Fx 模块
//
// 通过 Fx 依赖注入使用：
//
//	app := fx.New(
//	    host.Module,
//	    mdns.Module,
//	)
//	app.Run()
//
// # 适用场景
//
//   - 开发和测试环境
//   - 无互联网连接的私有网络
//   - 快速节点发现
//   - 局域网 P2P 应用
//
// # 限制
//
//   - 仅限局域网（LAN）发现
//   - 不适合大规模网络（推荐使用 DHT）
//   - 依赖网络支持多播（某些网络可能禁用）
package mdns
