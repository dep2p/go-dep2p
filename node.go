package dep2p

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/dep2p/go-dep2p/internal/app"
	addressif "github.com/dep2p/go-dep2p/pkg/interfaces/address"
	connmgrif "github.com/dep2p/go-dep2p/pkg/interfaces/connmgr"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	livenessif "github.com/dep2p/go-dep2p/pkg/interfaces/liveness"
	messagingif "github.com/dep2p/go-dep2p/pkg/interfaces/messaging"
	reachabilityif "github.com/dep2p/go-dep2p/pkg/interfaces/reachability"
	realmif "github.com/dep2p/go-dep2p/pkg/interfaces/realm"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// Node 是面向用户的一把梭 Facade：
// - 对外提供更友好的高层 API（Send/Request/Publish/Subscribe 等）
// - 内部仍保持 endpoint.Endpoint 的最小稳定接口，避免把 core 接口绑死
//
// Node 同时持有 fx Runtime 的 Stop 句柄，Close 时会正确 Stop fx，避免资源泄露。
type Node struct {
	rt *app.Runtime

	// goodbyeWait 优雅下线等待时间
	// Close 时先发送 Goodbye，等待此时间让消息传播，再断开连接
	goodbyeWait time.Duration
}

// Endpoint 返回底层 endpoint.Endpoint（最小稳定接口）。
func (n *Node) Endpoint() endpoint.Endpoint {
	if n == nil || n.rt == nil {
		return nil
	}
	return n.rt.Endpoint
}

// Messaging 返回消息子系统（可为 nil，取决于配置/模块）。
func (n *Node) Messaging() messagingif.MessagingService {
	if n == nil || n.rt == nil {
		return nil
	}
	return n.rt.Messaging
}

// ConnectionManager 返回连接管理子系统（可为 nil，取决于配置/模块）。
func (n *Node) ConnectionManager() connmgrif.ConnectionManager {
	if n == nil || n.rt == nil {
		return nil
	}
	return n.rt.ConnectionManager
}

// Liveness 返回存活检测服务（可为 nil，取决于配置/模块）。
func (n *Node) Liveness() livenessif.LivenessService {
	if n == nil || n.rt == nil {
		return nil
	}
	return n.rt.Liveness
}

// Realm 返回 Realm 管理器（可为 nil，取决于配置/模块）。
func (n *Node) Realm() realmif.RealmManager {
	if n == nil || n.rt == nil {
		return nil
	}
	return n.rt.Realm
}

// AddressParser 返回地址解析器（通过 Fx 注入）。
func (n *Node) AddressParser() addressif.AddressParser {
	if n == nil || n.rt == nil {
		return nil
	}
	return n.rt.AddressParser
}

// ===========================
// Facade: endpoint.Endpoint 透传
// ===========================

// ID 返回节点 ID（透传 Endpoint.ID）。
func (n *Node) ID() types.NodeID {
	if n.Endpoint() == nil {
		return types.EmptyNodeID
	}
	return types.NodeID(n.Endpoint().ID())
}

// Discovery 返回发现服务（透传 Endpoint.Discovery）。
func (n *Node) Discovery() endpoint.DiscoveryService {
	if n.Endpoint() == nil {
		return nil
	}
	return n.Endpoint().Discovery()
}

// NAT 返回 NAT 服务（透传 Endpoint.NAT）。
func (n *Node) NAT() endpoint.NATService {
	if n.Endpoint() == nil {
		return nil
	}
	return n.Endpoint().NAT()
}

// Relay 返回中继客户端（透传 Endpoint.Relay）。
func (n *Node) Relay() endpoint.RelayClient {
	if n.Endpoint() == nil {
		return nil
	}
	return n.Endpoint().Relay()
}

// AddressBook 返回地址簿（透传 Endpoint.AddressBook）。
func (n *Node) AddressBook() endpoint.AddressBook {
	if n.Endpoint() == nil {
		return nil
	}
	return n.Endpoint().AddressBook()
}

// 注意：EventBus 已于 2025-12-20 删除
// 原因：从未实现，当前系统使用回调函数模式（如 OnUpgraded callback）
// 如需事件系统，建议使用回调注册或 channel 机制

// ===========================
// Facade: 连接便捷方法
// ===========================

// Connect 通过 NodeID 连接到节点
//
// 自动从 AddressBook/Discovery 查找地址并连接。
// 这是最推荐的连接方式，用户只需提供 NodeID。
//
// 如果已有到该节点的连接，返回现有连接。
//
// 身份验证（SPEC-CONNECTION-001）：
//
//	身份验证在 Endpoint 层执行。TLS/Noise 握手完成后，
//	Endpoint 验证 RemoteIdentity() == expectedNodeID，
//	验证失败返回 ErrIdentityMismatch 并关闭连接。
//
// 示例:
//
//	conn, err := node.Connect(ctx, peerID)
//
// 参见：SPEC-CONNECTION-001（连接身份验证规范）
func (n *Node) Connect(ctx context.Context, nodeID types.NodeID) (endpoint.Connection, error) {
	if n.Endpoint() == nil {
		return nil, fmt.Errorf("Endpoint 未初始化")
	}
	return n.Endpoint().Connect(ctx, endpoint.NodeID(nodeID))
}

// ConnectWithAddrs 使用指定地址字符串连接到节点
//
// 内部自动解析地址字符串，无需用户手动转换。
// 跳过发现服务，直接使用提供的地址尝试连接。
//
// 注意：地址参数使用 Dial Address 格式（不含 /p2p/<NodeID>），
// NodeID 需要单独提供。对于 Full Address，请使用 ConnectToAddr。
//
// 支持多种地址格式：
//   - "192.168.1.1:8000" (IP:Port 格式)
//   - "/ip4/192.168.1.1/udp/8000/quic-v1" (Multiaddr 格式)
//
// 身份验证（SPEC-CONNECTION-001）：
//
//	身份验证在 Endpoint 层执行。TLS/Noise 握手完成后，
//	Endpoint 验证 RemoteIdentity() == nodeID，
//	验证失败返回 ErrIdentityMismatch 并关闭连接。
//
// 示例:
//
//	conn, err := node.ConnectWithAddrs(ctx, peerID, []string{"192.168.1.1:8000"})
//
// 参见：
//   - SPEC-ADDRESS-001（Dial Address 定义）
//   - SPEC-CONNECTION-001（连接身份验证规范）
func (n *Node) ConnectWithAddrs(ctx context.Context, nodeID types.NodeID, addrs []string) (endpoint.Connection, error) {
	if n.Endpoint() == nil {
		return nil, fmt.Errorf("Endpoint 未初始化")
	}
	if n.AddressParser() == nil {
		return nil, fmt.Errorf("AddressParser 未初始化")
	}

	// INV-004：ConnectWithAddrs 的输入必须是 Dial Address（不含 /p2p/<NodeID>）。
	// Full Address（含 /p2p/）必须使用 ConnectToAddr，以避免语义混用。
	for _, a := range addrs {
		if strings.Contains(a, "/p2p/") {
			return nil, fmt.Errorf("ConnectWithAddrs 仅接受 Dial Address（不含 /p2p/<NodeID>），请使用 ConnectToAddr: %s", a)
		}
	}

	// 使用注入的 AddressParser 解析地址字符串
	parsedAddrs, err := n.AddressParser().ParseMultiple(addrs)
	if err != nil {
		return nil, fmt.Errorf("解析地址失败: %w", err)
	}

	return n.Endpoint().ConnectWithAddrs(ctx, endpoint.NodeID(nodeID), parsedAddrs)
}

// ParseAddress 解析单个地址字符串
//
// 根据 IMPL-ADDRESS-UNIFICATION.md 规范，仅支持 multiaddr 格式：
//   - "/ip4/192.168.1.1/udp/8000/quic-v1"
//   - "/ip6/::1/udp/8000/quic-v1"
//   - "/dns4/example.com/udp/8000/quic-v1"
//   - "/p2p/QmPeer/p2p-circuit/p2p/QmDest"
//
// host:port 格式（如 "192.168.1.1:8000"）不再支持。
// 如需从 host:port 创建地址，请使用 types.FromHostPort：
//
//	ma, _ := types.FromHostPort("192.168.1.1", 8000, "udp/quic-v1")
//	addr, _ := node.ParseAddress(ma.String())
func (n *Node) ParseAddress(s string) (endpoint.Address, error) {
	if n.AddressParser() == nil {
		return nil, fmt.Errorf("AddressParser 未初始化")
	}
	return n.AddressParser().Parse(s)
}

// ParseAddresses 解析多个地址字符串
//
// 根据 IMPL-ADDRESS-UNIFICATION.md 规范，仅支持 multiaddr 格式。
// 详见 ParseAddress 文档。
func (n *Node) ParseAddresses(ss []string) ([]endpoint.Address, error) {
	if n.AddressParser() == nil {
		return nil, fmt.Errorf("AddressParser 未初始化")
	}
	return n.AddressParser().ParseMultiple(ss)
}

// ListenAddrs 返回监听地址列表（透传 Endpoint.ListenAddrs）。
func (n *Node) ListenAddrs() []endpoint.Address {
	if n.Endpoint() == nil {
		return nil
	}
	return n.Endpoint().ListenAddrs()
}

// AdvertisedAddrs 返回通告地址列表（透传 Endpoint.AdvertisedAddrs）。
func (n *Node) AdvertisedAddrs() []endpoint.Address {
	if n.Endpoint() == nil {
		return nil
	}
	return n.Endpoint().AdvertisedAddrs()
}

// ===========================
// Facade: 完整地址 API（v1.2 新增）
// ===========================

// ShareableAddrs 返回可分享的完整地址列表
//
// 每个地址都包含 /p2p/<NodeID> 后缀，可直接分享给其他用户/节点。
//
// 严格语义（INV-005 + REQ-ADDR-002）：
//   - 仅返回"已验证的公网直连地址"（VerifiedDirect）的 Full Address
//   - 数据源为 VerifiedDirectAddrs()（唯一真源），非 AdvertisedAddrs()
//   - 过滤掉非公网地址（私网/回环/link-local）
//   - 在无 VerifiedDirect 时返回 nil（节点不可对外引导）
//
// 注意：本方法不再回退到监听地址（ListenAddrs）。监听地址可能是 0.0.0.0、内网地址或回环地址，
// 直接分享给其他节点通常无效，且会造成引导/入网语义混乱。
//
// 返回格式示例：
//
//	/ip4/1.2.3.4/udp/4001/quic-v1/p2p/5Q2STWvBFn...
//	/dns4/mynode.example.com/udp/4001/quic-v1/p2p/5Q2STWvBFn...
//
// 用法：
//
//	addrs := node.ShareableAddrs()
//	if len(addrs) > 0 {
//	    fmt.Println("分享此地址给其他人:", addrs[0])
//	}
//
// 参见：系统不变量 INV-005（ShareableAddrs = VerifiedDirect Full Address）
// 参见：REQ-ADDR-002（ShareableAddrs 只能返回 VerifiedDirect）
func (n *Node) ShareableAddrs() []string {
	if n.Endpoint() == nil {
		return nil
	}

	selfID := n.ID()
	if selfID.IsEmpty() {
		return nil
	}

	// REQ-ADDR-002: 严格使用 VerifiedDirectAddrs 作为唯一真源
	// 这些地址已经是：
	// 1. 通过 dial-back 验证的直连地址
	// 2. 不包含 Relay 地址
	// 3. 不包含 ListenAddrs 回退
	addrs := n.Endpoint().VerifiedDirectAddrs()
	if len(addrs) == 0 {
		// INV-005: 无 VerifiedDirect 时返回 nil
		return nil
	}

	result := make([]string, 0, len(addrs))
	seen := make(map[string]bool)

	for _, addr := range addrs {
		if addr == nil {
			continue
		}

		addrStr := addr.String()

		// INV-005 过滤：排除非公网地址（私网/回环/link-local）
		// VerifiedDirectAddrs 理论上不应包含这些，但做防御性检查
		if !addr.IsPublic() {
			continue
		}

		// 构建完整地址
		fullAddr := string(types.Multiaddr(addrStr).WithPeerID(selfID))
		if fullAddr == "" || fullAddr == addrStr {
			// 跳过无法构建的地址
			continue
		}

		// 去重
		if seen[fullAddr] {
			continue
		}
		seen[fullAddr] = true
		result = append(result, fullAddr)
	}

	// INV-005：无 VerifiedDirect 时返回 nil（而非空切片），语义更明确
	if len(result) == 0 {
		return nil
	}

	return result
}

// BootstrapCandidate 候选地址结构
//
// 用于 BootstrapCandidates() 返回，支持人工分享/跨设备冷启动。
// MUST NOT 用于 DHT 发布，不等同于 ShareableAddrs。
// BootstrapCandidates 返回可用于冷启动尝试的候选地址列表（旁路/非严格）
//
// 与 ShareableAddrs() 正交分离：
//   - ShareableAddrs(): 严格，只返回 VerifiedDirect，可入 DHT
//   - BootstrapCandidates(): 旁路，返回所有候选（直连+relay），不入 DHT
//
// 典型用途：
//   - 人工分享给其他设备试连
//   - 创世节点启动后立即可用（无需等待验证）
//
// 返回的候选地址：
//   - 直连候选：来自本机接口/云元数据/用户配置等
//   - relay 候选：来自 AutoRelay/RelayClient
//   - 每个候选都标注 Kind/Source/Confidence/Verified
//
// 参见：系统不变量 INV-005（BootstrapCandidates 与 ShareableAddrs 正交）
func (n *Node) BootstrapCandidates() []reachabilityif.BootstrapCandidate {
	if n == nil || n.rt == nil {
		return nil
	}

	selfID := n.ID()
	if selfID.IsEmpty() {
		return nil
	}

	// 优先：使用 reachability coordinator 的候选快照（包含直连候选 + relay 候选）
	if n.rt.Reachability != nil {
		out := n.rt.Reachability.BootstrapCandidates(selfID)
		if len(out) > 0 {
			return out
		}
	}

	// 回退：如果没有 reachability coordinator，则用 AdvertisedAddrs 构建候选
	ep := n.Endpoint()
	if ep == nil {
		return nil
	}

	var result []reachabilityif.BootstrapCandidate
	for _, addr := range ep.AdvertisedAddrs() {
		if addr == nil {
			continue
		}
		addrStr := addr.String()

		kind := reachabilityif.CandidateKindDirect
		if strings.Contains(addrStr, "/p2p-circuit") {
			kind = reachabilityif.CandidateKindRelay
		}

		fullAddr := string(types.Multiaddr(addrStr).WithPeerID(selfID))
		if fullAddr == "" || fullAddr == addrStr {
			continue
		}

		verified := false
		for _, va := range ep.VerifiedDirectAddrs() {
			if va != nil && va.String() == addrStr {
				verified = true
				break
			}
		}

		result = append(result, reachabilityif.BootstrapCandidate{
			FullAddr:   fullAddr,
			Kind:       kind,
			Source:     "advertised",
			Confidence: reachabilityif.ConfidenceMedium,
			Verified:   verified,
			Notes:      "",
		})
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// WaitShareableAddrs 等待节点产生至少一个可分享的完整地址（Full Address）。
//
// 典型用途：创世节点/入口节点启动后，等待 Reachability/NAT 等机制生成 VerifiedDirect 地址，
// 然后将其作为 Bootstrap seed 分享给后续节点。
//
// 注意：Relay 电路地址不计入可分享地址（INV-005），因此纯 NAT 节点若无直连验证通过，
// 本方法会持续等待直至 ctx 超时。
//
// 返回：当 ShareableAddrs() 非空时返回该列表；若 ctx 取消/超时则返回错误。
//
// 参见：系统不变量 INV-005（ShareableAddrs = VerifiedDirect Full Address）
func (n *Node) WaitShareableAddrs(ctx context.Context) ([]string, error) {
	// 先快速检查一次
	if addrs := n.ShareableAddrs(); len(addrs) > 0 {
		return addrs, nil
	}

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			if addrs := n.ShareableAddrs(); len(addrs) > 0 {
				return addrs, nil
			}
		}
	}
}

// ConnectToAddr 使用完整地址（Full Address）连接到节点
//
// 完整地址必须包含 /p2p/<NodeID> 后缀（符合 SPEC-ADDRESS-001）。
// 自动解析 NodeID 和可拨号地址，然后建立连接。
//
// 身份验证（SPEC-CONNECTION-001）：
//
//	身份验证在 Endpoint 层（dialAddr）执行，不在 Node 层重复验证。
//	TLS/Noise 握手完成后，Endpoint 会验证 RemoteIdentity() == ExpectedNodeID，
//	验证失败会返回 ErrIdentityMismatch 并关闭连接。
//
// 示例：
//
//	// 使用从其他用户获取的完整地址
//	conn, err := node.ConnectToAddr(ctx, "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/5Q2STW...")
//
//	// 支持 Relay 电路地址
//	conn, err := node.ConnectToAddr(ctx, "/ip4/.../p2p/RelayID/p2p-circuit/p2p/TargetID")
//
// 与 ConnectWithAddrs 的区别：
//   - ConnectToAddr: 输入完整地址（含 /p2p/<NodeID>），自动解析
//   - ConnectWithAddrs: 输入 NodeID + 地址列表，需要分别提供
//
// 参见：
//   - SPEC-ADDRESS-001（地址格式术语规范）
//   - SPEC-CONNECTION-001（连接身份验证规范）
func (n *Node) ConnectToAddr(ctx context.Context, fullAddr string) (endpoint.Connection, error) {
	if n.Endpoint() == nil {
		return nil, fmt.Errorf("Endpoint 未初始化")
	}

	// 解析完整地址
	ma := types.Multiaddr(fullAddr)
	peerID := ma.PeerID()
	if peerID.IsEmpty() {
		return nil, fmt.Errorf("解析地址失败: 缺少 /p2p/<NodeID>")
	}
	dialAddr := string(ma.WithoutPeerID())

	// 使用解析出的地址连接
	return n.ConnectWithAddrs(ctx, peerID, []string{dialAddr})
}

// Close 关闭 Node（优雅下线）：
//
// 关闭顺序：
//  1. 发送 Goodbye 消息（如果 Liveness 启用）
//  2. 等待 Goodbye 传播（可通过 WithGoodbyeWait 配置）
//  3. 停止 fx（触发各模块 OnStop）
//  4. 补偿性关闭 Endpoint
func (n *Node) Close() error {
	if n == nil || n.rt == nil {
		return nil
	}

	// 1. 发送 Goodbye（如果 Liveness 启用）
	if n.Liveness() != nil {
		goodbyeCtx, goodbyeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = n.Liveness().SendGoodbye(goodbyeCtx, types.GoodbyeReasonShutdown)
		goodbyeCancel()
	}

	// 2. 等待 Goodbye 传播
	if n.goodbyeWait > 0 {
		time.Sleep(n.goodbyeWait)
	}

	// 3. 停止 fx
	stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stopErr := n.rt.Stop(stopCtx)

	// 4. 兜底关闭 Endpoint
	var closeErr error
	if n.rt.Endpoint != nil {
		closeErr = n.rt.Endpoint.Close()
	}

	if stopErr != nil && closeErr != nil {
		return fmt.Errorf("停止运行时失败: %v; 关闭 Endpoint 失败: %v", stopErr, closeErr)
	}
	if stopErr != nil {
		return stopErr
	}
	return closeErr
}

// ===========================
// Facade: Messaging 快捷方法
// ===========================

// Send 发送单向消息
//
// v1.1 变更: 强制隔离检查点 #1
//   - 调用前必须已 JoinRealm
//   - 未加入 Realm 返回 ErrNotMember
func (n *Node) Send(ctx context.Context, nodeID types.NodeID, data []byte) error {
	// IMPL-1227: 从当前 Realm 获取 Messaging 服务
	realm := n.CurrentRealm()
	if realm == nil {
		return endpoint.ErrNotMember
	}
	return realm.Messaging().Send(ctx, nodeID, data)
}

// Request 发送请求-响应消息
//
// IMPL-1227: 从当前 Realm 的 Messaging 服务发送请求
func (n *Node) Request(ctx context.Context, nodeID types.NodeID, data []byte) ([]byte, error) {
	// IMPL-1227: 从当前 Realm 获取 Messaging 服务
	realm := n.CurrentRealm()
	if realm == nil {
		return nil, endpoint.ErrNotMember
	}
	return realm.Messaging().Request(ctx, nodeID, data)
}

// Publish 发布消息到主题
//
// v1.1 变更: 强制隔离检查点 #1
//   - 调用前必须已 JoinRealm
//   - 未加入 Realm 返回 ErrNotMember
func (n *Node) Publish(ctx context.Context, topic string, data []byte) error {
	// 🔒 强制隔离检查点 #1: Node Facade
	if !n.IsMember() {
		return endpoint.ErrNotMember
	}

	// IMPL-1227: 使用 Realm PubSub 服务（自动添加 Realm 前缀）
	realm := n.CurrentRealm()
	if realm == nil {
		return endpoint.ErrNotMember
	}

	pubsub := realm.PubSub()
	if pubsub == nil {
		// 回退到旧的 Messaging.Publish（如果 PubSub 未配置）
		if n.Messaging() == nil {
			return fmt.Errorf("PubSub/Messaging 未启用")
		}
		return n.Messaging().Publish(ctx, topic, data)
	}

	// 加入主题并发布
	t, err := pubsub.Join(ctx, topic)
	if err != nil {
		return fmt.Errorf("join topic: %w", err)
	}
	return t.Publish(ctx, data)
}

// Subscribe 订阅主题
//
// v1.1 变更: 强制隔离检查点 #1
//   - 调用前必须已 JoinRealm
//   - 未加入 Realm 返回 ErrNotMember
//
// v1.2 变更（IMPL-1227）:
//   - 使用 Realm PubSub 服务，自动添加 Realm 前缀
func (n *Node) Subscribe(ctx context.Context, topic string) (messagingif.Subscription, error) {
	// 🔒 强制隔离检查点 #1: Node Facade
	if !n.IsMember() {
		return nil, endpoint.ErrNotMember
	}

	// IMPL-1227: 使用 Realm PubSub 服务（自动添加 Realm 前缀）
	realm := n.CurrentRealm()
	if realm == nil {
		return nil, endpoint.ErrNotMember
	}

	pubsub := realm.PubSub()
	if pubsub == nil {
		// 回退到旧的 Messaging.Subscribe（如果 PubSub 未配置）
		if n.Messaging() == nil {
			return nil, fmt.Errorf("PubSub/Messaging 未启用")
		}
		return n.Messaging().Subscribe(ctx, topic)
	}

	// 加入主题并订阅
	t, err := pubsub.Join(ctx, topic)
	if err != nil {
		return nil, fmt.Errorf("join topic: %w", err)
	}
	sub, err := t.Subscribe()
	if err != nil {
		return nil, fmt.Errorf("subscribe: %w", err)
	}

	// 包装 realmif.Subscription 为 messagingif.Subscription
	return newRealmSubscriptionAdapter(sub, topic), nil
}

// Query 发送查询
//
// v1.1 变更: 强制隔离检查点 #1
//   - 调用前必须已 JoinRealm
//   - 未加入 Realm 返回 ErrNotMember
func (n *Node) Query(ctx context.Context, topic string, query []byte) ([]byte, error) {
	// 🔒 强制隔离检查点 #1: Node Facade
	if !n.IsMember() {
		return nil, endpoint.ErrNotMember
	}
	if n.Messaging() == nil {
		return nil, fmt.Errorf("Messaging 未启用")
	}
	return n.Messaging().Query(ctx, topic, query)
}

// QueryAll 发送查询到所有响应者
//
// v1.1 变更: 强制隔离检查点 #1
//   - 调用前必须已 JoinRealm
//   - 未加入 Realm 返回 ErrNotMember
func (n *Node) QueryAll(ctx context.Context, topic string, query []byte, opts messagingif.QueryOptions) ([]messagingif.QueryResponse, error) {
	// 🔒 强制隔离检查点 #1: Node Facade
	if !n.IsMember() {
		return nil, endpoint.ErrNotMember
	}
	if n.Messaging() == nil {
		return nil, fmt.Errorf("Messaging 未启用")
	}
	return n.Messaging().QueryAll(ctx, topic, query, opts)
}

// SetRequestHandler 设置请求处理器
func (n *Node) SetRequestHandler(protocol types.ProtocolID, handler messagingif.RequestHandler) {
	if n.Messaging() == nil {
		return
	}
	n.Messaging().SetRequestHandler(protocol, handler)
}

// SetNotifyHandler 设置通知处理器
func (n *Node) SetNotifyHandler(protocol types.ProtocolID, handler messagingif.NotifyHandler) {
	if n.Messaging() == nil {
		return
	}
	n.Messaging().SetNotifyHandler(protocol, handler)
}

// SetQueryHandler 设置查询处理器
func (n *Node) SetQueryHandler(topic string, handler messagingif.QueryHandler) {
	if n.Messaging() == nil {
		return
	}
	n.Messaging().SetQueryHandler(topic, handler)
}

// ===========================
// Facade: Realm 快捷方法
// ===========================

// JoinRealm 加入指定 Realm，返回 Realm 对象（IMPL-1227 新 API）
//
// 必须通过 WithRealmKey 提供 realmKey，用于 PSK 成员认证。
// RealmID 由 realmKey 自动派生。
//
// 示例:
//
//	realm, err := node.JoinRealm(ctx, "my-business", realmif.WithRealmKey(key))
//	if err != nil { ... }
//	messaging := realm.Messaging()
func (n *Node) JoinRealm(ctx context.Context, name string, opts ...realmif.RealmOption) (realmif.Realm, error) {
	if n.Realm() == nil {
		return nil, fmt.Errorf("Realm 未启用")
	}
	return n.Realm().JoinRealm(ctx, name, opts...)
}

// JoinRealmWithKey 使用密钥加入 Realm（便捷方法）
//
// 等价于 JoinRealm(ctx, name, WithRealmKey(key), opts...)
func (n *Node) JoinRealmWithKey(ctx context.Context, name string, key types.RealmKey, opts ...realmif.RealmOption) (realmif.Realm, error) {
	if n.Realm() == nil {
		return nil, fmt.Errorf("Realm 未启用")
	}
	return n.Realm().JoinRealmWithKey(ctx, name, key, opts...)
}

// LeaveRealm 离开当前 Realm（快捷方法）
//
// v1.1 变更: 无参数，离开当前唯一的 Realm
//   - 如果未加入任何 Realm，返回 ErrNotMember
//
// 等价于 node.Realm().LeaveRealm()
func (n *Node) LeaveRealm() error {
	if n.Realm() == nil {
		return fmt.Errorf("Realm 未启用")
	}
	return n.Realm().LeaveRealm()
}

// CurrentRealm 返回当前 Realm 对象（IMPL-1227 新 API）
//
// 如果未加入任何 Realm，返回 nil。
func (n *Node) CurrentRealm() realmif.Realm {
	if n.Realm() == nil {
		return nil
	}
	return n.Realm().CurrentRealm()
}

// IsMember 检查是否已加入任何 Realm（快捷方法）
//
// v1.1 新增: 无参数便捷方法
//   - 返回 true 表示已加入某个 Realm（业务 API 可用）
//   - 返回 false 表示未加入任何 Realm（业务 API 不可用）
//
// 等价于 node.Realm().IsMember()
func (n *Node) IsMember() bool {
	if n.Realm() == nil {
		return false
	}
	return n.Realm().IsMember()
}

// RealmPeers 返回当前 Realm 内的节点列表（快捷方法）
//
// IMPL-1227: 使用 CurrentRealm().Members() 替代
func (n *Node) RealmPeers() []types.NodeID {
	realm := n.CurrentRealm()
	if realm == nil {
		return nil
	}
	return realm.Members()
}

// ===========================
// Facade: Liveness 快捷方法
// ===========================

// Ping 对指定节点进行 Ping 检测（快捷方法）
//
// 返回 RTT（往返时间），如果超时返回错误。
// 等价于 node.Liveness().Ping(ctx, nodeID)
func (n *Node) Ping(ctx context.Context, nodeID types.NodeID) (time.Duration, error) {
	if n.Liveness() == nil {
		return 0, fmt.Errorf("Liveness 未启用")
	}
	return n.Liveness().Ping(ctx, nodeID)
}

// PeerStatus 获取节点状态（快捷方法）
//
// 返回节点当前状态：Online/Degraded/Offline/Unknown
// 等价于 node.Liveness().PeerStatus(nodeID)
func (n *Node) PeerStatus(nodeID types.NodeID) types.PeerStatus {
	if n.Liveness() == nil {
		return types.PeerStatusUnknown
	}
	return n.Liveness().PeerStatus(nodeID)
}

// SendGoodbye 发送优雅下线消息（快捷方法）
//
// 向所有已连接的邻居节点发送 Goodbye 消息。
// 等价于 node.Liveness().SendGoodbye(ctx, reason)
func (n *Node) SendGoodbye(ctx context.Context, reason types.GoodbyeReason) error {
	if n.Liveness() == nil {
		return fmt.Errorf("Liveness 未启用")
	}
	return n.Liveness().SendGoodbye(ctx, reason)
}

// OnlinePeers 获取所有在线节点（快捷方法）
//
// 等价于 node.Liveness().OnlinePeers()
func (n *Node) OnlinePeers() []types.NodeID {
	if n.Liveness() == nil {
		return nil
	}
	return n.Liveness().OnlinePeers()
}

// HealthScore 获取节点健康评分（快捷方法）
//
// 返回 0-100 的健康评分。
// 等价于 node.Liveness().HealthScore(nodeID)
func (n *Node) HealthScore(nodeID types.NodeID) int {
	if n.Liveness() == nil {
		return 0
	}
	return n.Liveness().HealthScore(nodeID)
}

// ===========================
// 运维审计（REQ-OPS-002）
// ===========================

// RequirementStatus 需求状态
type RequirementStatus string

const (
	// RequirementImplemented 已实现
	RequirementImplemented RequirementStatus = "implemented"
	// RequirementPartial 部分实现
	RequirementPartial RequirementStatus = "partial"
	// RequirementNotImplemented 未实现
	RequirementNotImplemented RequirementStatus = "not_implemented"
)

// RequirementAuditItem 单条需求审计项
type RequirementAuditItem struct {
	// ID 需求 ID（如 REQ-CONN-001）
	ID string
	// Title 需求标题
	Title string
	// Category 需求分类
	Category string
	// Status 实现状态
	Status RequirementStatus
	// Evidence 证据（实现了哪些组件/方法）
	Evidence []string
	// Gaps 缺口（缺少什么）
	Gaps []string
}

// RequirementAuditReport 需求审计报告
type RequirementAuditReport struct {
	// Timestamp 审计时间
	Timestamp time.Time
	// NodeID 被审计的节点 ID
	NodeID types.NodeID
	// Summary 摘要
	Summary AuditSummary
	// Items 详细审计项
	Items []RequirementAuditItem
}

// AuditSummary 审计摘要
type AuditSummary struct {
	// TotalRequirements 总需求数
	TotalRequirements int
	// ImplementedCount 已实现数
	ImplementedCount int
	// PartialCount 部分实现数
	PartialCount int
	// NotImplementedCount 未实现数
	NotImplementedCount int
	// ImplementationRate 实现率（0-100）
	ImplementationRate float64
}

// AuditRequirements 一键审计需求实现状态（REQ-OPS-002）
//
// 返回当前节点的需求实现审计报告，包括：
// - 各需求的实现状态（implemented/partial/not_implemented）
// - 实现证据（对应的组件/方法）
// - 缺口说明（缺少什么）
//
// 示例：
//
//	report := node.AuditRequirements()
//	fmt.Printf("实现率: %.1f%%\n", report.Summary.ImplementationRate)
//	for _, item := range report.Items {
//	    if item.Status != dep2p.RequirementImplemented {
//	        fmt.Printf("缺口: %s - %v\n", item.ID, item.Gaps)
//	    }
//	}
func (n *Node) AuditRequirements() *RequirementAuditReport {
	report := &RequirementAuditReport{
		Timestamp: time.Now(),
		NodeID:    n.ID(),
		Items:     make([]RequirementAuditItem, 0),
	}

	// 审计各子系统
	n.auditConnectionRequirements(report)
	n.auditAddressRequirements(report)
	n.auditDiscoveryRequirements(report)
	n.auditRealmRequirements(report)
	n.auditSecurityRequirements(report)
	n.auditObservabilityRequirements(report)

	// 计算摘要
	for _, item := range report.Items {
		report.Summary.TotalRequirements++
		switch item.Status {
		case RequirementImplemented:
			report.Summary.ImplementedCount++
		case RequirementPartial:
			report.Summary.PartialCount++
		case RequirementNotImplemented:
			report.Summary.NotImplementedCount++
		}
	}

	if report.Summary.TotalRequirements > 0 {
		report.Summary.ImplementationRate = float64(report.Summary.ImplementedCount) /
			float64(report.Summary.TotalRequirements) * 100
	}

	return report
}

// auditConnectionRequirements 审计连接相关需求
func (n *Node) auditConnectionRequirements(report *RequirementAuditReport) {
	ep := n.Endpoint()

	// REQ-CONN-001: 用户可预测的连接语义
	item := RequirementAuditItem{
		ID:       "REQ-CONN-001",
		Title:    "用户可预测的连接语义（按 NodeID/FullAddr 分流）",
		Category: "conn",
	}
	if ep != nil {
		item.Status = RequirementImplemented
		item.Evidence = []string{
			"Connect(nodeID) - DialByNodeID",
			"ConnectWithAddrs(nodeID, addrs) - DialByNodeIDWithDialAddrs",
		}
	} else {
		item.Status = RequirementNotImplemented
		item.Gaps = []string{"Endpoint 未初始化"}
	}
	report.Items = append(report.Items, item)

	// REQ-CONN-005: 连接幂等性与并发去重
	item = RequirementAuditItem{
		ID:       "REQ-CONN-005",
		Title:    "连接幂等性与并发行为可预测",
		Category: "conn",
	}
	if ep != nil {
		item.Status = RequirementImplemented
		item.Evidence = []string{
			"dialInflight sync.Map - 并发去重",
			"dialFuture - 复用进行中的拨号",
		}
	} else {
		item.Status = RequirementNotImplemented
		item.Gaps = []string{"Endpoint 未初始化"}
	}
	report.Items = append(report.Items, item)
}

// auditAddressRequirements 审计地址相关需求
func (n *Node) auditAddressRequirements(report *RequirementAuditReport) {
	ep := n.Endpoint()

	// REQ-ADDR-002: ShareableAddrs=VerifiedDirect
	item := RequirementAuditItem{
		ID:       "REQ-ADDR-002",
		Title:    "ShareableAddrs 只能返回 VerifiedDirect",
		Category: "address",
	}
	if ep != nil {
		item.Status = RequirementImplemented
		item.Evidence = []string{
			"VerifiedDirectAddrs() - 已验证直连地址",
			"ShareableAddrs() - 基于 VerifiedDirect 构建",
		}
	} else {
		item.Status = RequirementNotImplemented
		item.Gaps = []string{"Endpoint 未初始化"}
	}
	report.Items = append(report.Items, item)

	// REQ-ADDR-003: 地址变化订阅
	item = RequirementAuditItem{
		ID:       "REQ-ADDR-003",
		Title:    "地址变化可被订阅",
		Category: "address",
		Status:   RequirementImplemented,
		Evidence: []string{"SetOnAddressChanged(callback)"},
	}
	report.Items = append(report.Items, item)
}

// auditDiscoveryRequirements 审计发现相关需求
func (n *Node) auditDiscoveryRequirements(report *RequirementAuditReport) {
	disc := n.Discovery()

	// REQ-DISC-002: 入网状态机
	item := RequirementAuditItem{
		ID:       "REQ-DISC-002",
		Title:    "入网应存在可解释的状态机",
		Category: "discovery",
	}
	if disc != nil {
		item.Status = RequirementImplemented
		item.Evidence = []string{
			"DiscoveryState 枚举（NotStarted/Bootstrapping/Connected/Discoverable/Failed）",
			"State() - 获取当前状态",
			"SetOnStateChanged() - 订阅状态变化",
		}
	} else {
		item.Status = RequirementNotImplemented
		item.Gaps = []string{"Discovery 服务未初始化"}
	}
	report.Items = append(report.Items, item)

	// REQ-DISC-006: 禁止递归发现
	item = RequirementAuditItem{
		ID:       "REQ-DISC-006",
		Title:    "禁止递归发现（避免自递归闭环）",
		Category: "discovery",
		Status:   RequirementImplemented,
		Evidence: []string{
			"inFlightDiscoveries sync.Map - 追踪进行中的发现",
			"recursionDepth - 递归深度检测",
			"enterDiscoveryContext/leaveDiscoveryContext - 递归防护",
		},
	}
	report.Items = append(report.Items, item)
}

// auditRealmRequirements 审计 Realm 相关需求
func (n *Node) auditRealmRequirements(report *RequirementAuditReport) {
	realm := n.Realm()

	// REQ-REALM-001: Realm 强制隔离
	item := RequirementAuditItem{
		ID:       "REQ-REALM-001",
		Title:    "Realm 强制隔离：未 JoinRealm 必须拒绝",
		Category: "protocol_stream",
	}
	if realm != nil {
		item.Status = RequirementImplemented
		item.Evidence = []string{
			"RealmAccessController.CheckAccess()",
			"入站流 Realm 校验",
		}
	} else {
		item.Status = RequirementNotImplemented
		item.Gaps = []string{"Realm 模块未启用"}
	}
	report.Items = append(report.Items, item)

	// REQ-BOOT-005: Private Realm 自举策略
	item = RequirementAuditItem{
		ID:       "REQ-BOOT-005",
		Title:    "Private Realm 自举策略可落地",
		Category: "bootstrap",
		Status:   RequirementImplemented,
		Evidence: []string{
			"WithPrivateBootstrapPeers() - JoinOption",
			"WithInviteData() - JoinOption",
			"WithSkipDHTRegistration() - JoinOption",
			"connectPrivateBootstrapPeers() - 私有引导连接",
		},
	}
	report.Items = append(report.Items, item)
}

// auditSecurityRequirements 审计安全相关需求
func (n *Node) auditSecurityRequirements(report *RequirementAuditReport) {
	// REQ-SEC-001: 所有连接必须加密
	item := RequirementAuditItem{
		ID:       "REQ-SEC-001",
		Title:    "所有连接必须加密且身份可验证",
		Category: "security",
		Status:   RequirementImplemented,
		Evidence: []string{
			"TLS/Noise 安全传输",
			"SecureInbound/SecureOutbound",
			"身份验证（RemoteIdentity == expected）",
		},
	}
	report.Items = append(report.Items, item)

	// REQ-SEC-002: 安全事件可观测
	item = RequirementAuditItem{
		ID:       "REQ-SEC-002",
		Title:    "关键安全事件必须可观测",
		Category: "security",
		Status:   RequirementImplemented,
		Evidence: []string{
			"SecurityEventType 枚举",
			"SecurityEvent 结构",
			"SecurityEventCallback 回调机制",
			"OnSecurityEvent() 订阅接口",
		},
	}
	report.Items = append(report.Items, item)
}

// auditObservabilityRequirements 审计可观测性相关需求
func (n *Node) auditObservabilityRequirements(report *RequirementAuditReport) {
	ep := n.Endpoint()

	// REQ-OPS-001: 统一诊断入口
	item := RequirementAuditItem{
		ID:       "REQ-OPS-001",
		Title:    "关键状态可观测且有统一诊断入口",
		Category: "observability_ops",
	}
	if ep != nil {
		item.Status = RequirementImplemented
		item.Evidence = []string{
			"DiagnosticReport() - 统一诊断报告",
			"包含 NodeID/Uptime/Connections/Discovery/NAT/Relay/Realm 诊断",
		}
	} else {
		item.Status = RequirementNotImplemented
		item.Gaps = []string{"Endpoint 未初始化"}
	}
	report.Items = append(report.Items, item)

	// REQ-OPS-002: 一键审计
	item = RequirementAuditItem{
		ID:       "REQ-OPS-002",
		Title:    "一键审计输出缺口列表",
		Category: "observability_ops",
		Status:   RequirementImplemented,
		Evidence: []string{
			"AuditRequirements() - 一键审计方法",
			"RequirementAuditReport - 审计报告结构",
		},
	}
	report.Items = append(report.Items, item)

	// REQ-OPS-004: 结构化日志
	item = RequirementAuditItem{
		ID:       "REQ-OPS-004",
		Title:    "运维日志应结构化",
		Category: "observability_ops",
		Status:   RequirementImplemented,
		Evidence: []string{
			"logger 包支持 JSON 格式",
			"结构化字段（nodeID, addr, err 等）",
		},
	}
	report.Items = append(report.Items, item)

	// REQ-OPS-006: 存活诊断
	liveness := n.Liveness()
	item = RequirementAuditItem{
		ID:       "REQ-OPS-006",
		Title:    "存活与状态可诊断（Ping/状态转移）",
		Category: "observability_ops",
	}
	if liveness != nil {
		item.Status = RequirementImplemented
		item.Evidence = []string{
			"Ping(nodeID) - RTT 检测",
			"PeerStatus(nodeID) - 状态查询",
			"HealthScore(nodeID) - 健康评分",
		}
	} else {
		item.Status = RequirementPartial
		item.Gaps = []string{"Liveness 服务未启用"}
	}
	report.Items = append(report.Items, item)
}

// String 返回审计报告的字符串表示
func (r *RequirementAuditReport) String() string {
	var sb strings.Builder

	sb.WriteString("=== 需求审计报告 ===\n")
	sb.WriteString(fmt.Sprintf("时间: %s\n", r.Timestamp.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("节点: %s\n\n", r.NodeID.ShortString()))

	sb.WriteString("--- 摘要 ---\n")
	sb.WriteString(fmt.Sprintf("总需求数: %d\n", r.Summary.TotalRequirements))
	sb.WriteString(fmt.Sprintf("已实现: %d\n", r.Summary.ImplementedCount))
	sb.WriteString(fmt.Sprintf("部分实现: %d\n", r.Summary.PartialCount))
	sb.WriteString(fmt.Sprintf("未实现: %d\n", r.Summary.NotImplementedCount))
	sb.WriteString(fmt.Sprintf("实现率: %.1f%%\n\n", r.Summary.ImplementationRate))

	// 列出缺口
	sb.WriteString("--- 缺口列表 ---\n")
	hasGaps := false
	for _, item := range r.Items {
		if item.Status != RequirementImplemented {
			hasGaps = true
			sb.WriteString(fmt.Sprintf("[%s] %s - %s\n", item.Status, item.ID, item.Title))
			for _, gap := range item.Gaps {
				sb.WriteString(fmt.Sprintf("  - %s\n", gap))
			}
		}
	}
	if !hasGaps {
		sb.WriteString("无缺口，所有需求已实现！\n")
	}

	return sb.String()
}

// ============================================================================
//                              IMPL-1227: 订阅适配器
// ============================================================================

// realmSubscriptionAdapter 将 realmif.Subscription 适配为 messagingif.Subscription
//
// 用于 Node.Subscribe 返回值的兼容性。
type realmSubscriptionAdapter struct {
	sub    realmif.Subscription
	topic  string
	active bool
	msgCh  chan *types.Message
}

// newRealmSubscriptionAdapter 创建订阅适配器
func newRealmSubscriptionAdapter(sub realmif.Subscription, topic string) *realmSubscriptionAdapter {
	adapter := &realmSubscriptionAdapter{
		sub:    sub,
		topic:  topic,
		active: true,
		msgCh:  make(chan *types.Message),
	}

	// 启动消息转发协程
	go adapter.forwardMessages()

	return adapter
}

// forwardMessages 转发消息（realmif.PubSubMessage -> types.Message）
func (a *realmSubscriptionAdapter) forwardMessages() {
	defer close(a.msgCh)

	for msg := range a.sub.Messages() {
		if msg == nil {
			continue
		}
		a.msgCh <- &types.Message{
			ID:        nil, // PubSubMessage 没有 ID 字段
			Topic:     a.topic,
			From:      msg.From,
			Data:      msg.Data,
			Timestamp: msg.ReceivedAt,
		}
	}

	a.active = false
}

// Topic 返回订阅的主题
func (a *realmSubscriptionAdapter) Topic() string {
	return a.topic
}

// Messages 返回消息通道
func (a *realmSubscriptionAdapter) Messages() <-chan *types.Message {
	return a.msgCh
}

// Cancel 取消订阅
func (a *realmSubscriptionAdapter) Cancel() {
	a.sub.Cancel()
	a.active = false
}

// IsActive 是否仍然活跃
func (a *realmSubscriptionAdapter) IsActive() bool {
	return a.active
}

// 确保实现 messagingif.Subscription 接口
var _ messagingif.Subscription = (*realmSubscriptionAdapter)(nil)
