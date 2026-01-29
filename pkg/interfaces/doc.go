// Package interfaces 定义 DeP2P 的公共接口
//
// 本包采用五层架构组织接口定义，采用扁平命名（无层级前缀）：
//
// # API Layer 接口
//
// 节点入口和配置：
//   - node.go           - Node 门面接口（用户入口）
//
// # Protocol Layer 接口
//
// 面向应用开发者的业务通信服务：
//   - messaging.go      - 点对点消息服务
//   - pubsub.go         - 发布订阅服务
//   - streams.go        - 流管理服务
//   - liveness.go       - 存活检测服务
//
// # Realm Layer 接口
//
// 业务隔离单元：
//   - realm.go          - Realm 业务隔离
//   - addressbook.go    - 成员地址簿（"仅 ID 连接"支持）
//
// # Core Layer 接口
//
// P2P 网络核心能力（一个接口文件 = 一个实现目录）：
//   - host.go           - 网络主机（核心门面）
//   - identity.go       - 身份管理
//   - transport.go      - 传输层
//   - security.go       - 安全层
//   - muxer.go          - 多路复用
//   - connmgr.go        - 连接管理（含 JitterTolerance）
//   - relay.go          - 中继服务
//   - peerstore.go      - 节点信息存储
//   - swarm.go          - 连接群管理（含 BandwidthCounter, PathHealthManager）
//   - protocol.go       - 协议路由
//   - upgrader.go       - 连接升级
//   - resource.go       - 资源管理
//   - eventbus.go       - 事件总线
//   - metrics.go        - 指标接口
//   - storage.go        - 存储引擎（BadgerDB）
//   - nat.go            - NAT 穿透
//   - reachability.go   - 可达性协调
//   - recovery.go       - 网络恢复（含 ConnectionHealthMonitor, NetworkMonitor）
//
// # Discovery Layer 接口
//
// 节点发现与广播，与 Core Layer 双向协作：
//   - coordinator.go    - 发现协调器（含 Discovery 契约接口）
//   - dht.go            - DHT 分布式哈希表接口
//   - bootstrap.go      - Bootstrap 引导服务接口
//   - mdns.go           - mDNS 局域网发现接口
//   - dns.go            - DNS 发现接口
//   - rendezvous.go     - Rendezvous 命名空间发现接口
//
// # 依赖方向
//
//	API → Protocol → Realm → Core ↔ Discovery
//
// 禁止反向依赖。
//
// # 设计原则
//
// 本包仅包含纯接口定义，数据结构定义在 pkg/types 包中，
// 协议消息定义在 pkg/proto 包中。
//
// 采用扁平命名结构：
//   - 简化导入：一次性导入所有接口
//   - 避免循环依赖：清晰的依赖关系
//   - 降低包层级：提高可维护性
package interfaces
