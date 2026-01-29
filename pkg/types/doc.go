// Package types 定义 DeP2P 的公共数据结构
//
// 这是整个系统的最底层包，不依赖任何其他 dep2p 内部包。
// 所有类型都是纯值类型，用于在各模块间传递数据。
//
// # 职能
//
// pkg/types 的职能是定义 **Go 内部数据结构**：
//   - 模块间数据传递
//   - API 参数/返回值
//   - 配置结构、选项模式、事件类型
//
// # 与 pkg/proto 的区别
//
// pkg/types 定义 Go 内部数据结构（内存结构），
// pkg/proto 定义网络协议消息（wire format）。
//
// # 文件组织
//
// 基础类型:
//   - ids.go        - PeerID, RealmID, RealmKey, PSK, ProtocolID, StreamID
//   - enums.go      - KeyType, Direction, NATType, Connectedness, Reachability
//   - base58.go     - Base58 编解码
//   - multiaddr.go  - Multiaddr 多地址类型
//   - errors.go     - 公共错误定义
//
// 网络类型:
//   - connection.go - ConnInfo, ConnStat, ConnState, ConnScope
//   - stream.go     - StreamInfo, StreamStat, StreamState, StreamScope
//   - discovery.go  - PeerInfo, AddrInfo
//
// 业务类型:
//   - realm.go      - RealmInfo, RealmConfig, RealmMember, RealmStats
//   - protocol.go   - ProtocolID 辅助函数, 协议常量
//
// 事件类型:
//   - events.go     - 所有事件类型（连接、发现、协议、Realm、NAT、存活）
//
// # 类型分类
//
// ID 类型:
//   - PeerID     - 节点唯一标识（公钥派生，Base58 编码）
//   - RealmID    - 业务域标识（PSK 派生）
//   - RealmKey   - Realm 密钥（32字节）
//   - PSK        - 预共享密钥
//   - ProtocolID - 协议标识（如 /dep2p/sys/ping/1.0.0）
//   - StreamID   - 流标识
//
// 枚举类型:
//   - Direction     - 连接/流方向（Inbound/Outbound）
//   - Connectedness - 连接状态（Connected/NotConnected/...）
//   - Reachability  - 可达性（Public/Private/Unknown）
//   - NATType       - NAT 类型（None/FullCone/Symmetric/...）
//   - KeyType       - 密钥类型（Ed25519/ECDSA/RSA/...）
//   - Priority      - 优先级（Low/Normal/High/Critical）
//
// 事件类型 (EvtXXX):
//   - EvtPeerConnected    - 节点连接事件
//   - EvtPeerDiscovered   - 节点发现事件
//   - EvtRealmJoined      - 加入 Realm 事件
//   - EvtNATTypeDetected  - NAT 类型检测事件
//
// # 设计原则
//
//  1. 不可变性：类型创建后尽量不可修改，使用值类型
//  2. 可比较性：实现 Equal 方法，支持作为 map key
//  3. 可序列化：实现 TextMarshaler/Unmarshaler，支持 JSON
//  4. 安全性：敏感类型（如 PSK）不实现 String，避免意外泄露
//  5. 零依赖：不依赖任何其他 dep2p 内部包（最底层）
//
// # 使用示例
//
//	import "github.com/dep2p/dep2p/pkg/types"
//
//	// 解析 PeerID
//	peerID, err := types.ParsePeerID("12D3KooW...")
//
//	// 创建 PeerInfo
//	peer := types.NewPeerInfo(peerID, addrs)
//
//	// 生成 PSK
//	psk := types.GeneratePSK()
//	realmID := psk.DeriveRealmID()
//
//	// 创建 Realm 配置
//	config := types.RealmConfig{
//	    PSK:      psk,
//	    AuthMode: types.AuthModePSK,
//	}
package types
