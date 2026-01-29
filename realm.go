package dep2p

import (
	"context"
	"fmt"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ════════════════════════════════════════════════════════════════════════════
//                              用户 API: Realm
// ════════════════════════════════════════════════════════════════════════════

// Realm 用户级 Realm API
//
// Realm 是对 interfaces.Realm 的包装，只暴露用户需要的方法。
// 内部方法（Router, Gateway, PSK, Authenticate 等）被隐藏。
//
// 使用示例：
//
//	node, _ := dep2p.Start(ctx, dep2p.Desktop())
//	realm, _ := node.JoinRealm(ctx, []byte("secret"))
//
//	// 基本信息
//	fmt.Println(realm.ID())
//	fmt.Println(realm.Name())
//
//	// 成员管理
//	members := realm.Members()
//	count := realm.MemberCount()
//
//	// 通信服务
//	messaging := realm.Messaging()
//	pubsub := realm.PubSub()
type Realm struct {
	internal interfaces.Realm
}

// ════════════════════════════════════════════════════════════════════════════
//                              域信息
// ════════════════════════════════════════════════════════════════════════════

// ID 返回域唯一标识
func (r *Realm) ID() string {
	return r.internal.ID()
}

// Name 返回域名称
func (r *Realm) Name() string {
	return r.internal.Name()
}

// ════════════════════════════════════════════════════════════════════════════
//                              成员管理
// ════════════════════════════════════════════════════════════════════════════

// Members 返回成员列表
func (r *Realm) Members() []string {
	return r.internal.Members()
}

// MemberCount 返回成员数量
func (r *Realm) MemberCount() int {
	return len(r.internal.Members())
}

// IsMember 检查是否为成员
func (r *Realm) IsMember(peerID string) bool {
	return r.internal.IsMember(peerID)
}

// OnMemberJoin 注册成员加入回调
//
// 当有新成员加入 Realm 时调用回调函数。
// 回调在后台 goroutine 中执行，不会阻塞事件处理。
//
// 注意：回调函数应该快速返回，避免长时间阻塞。
// 如需执行耗时操作，请在回调中启动新的 goroutine。
//
// 示例：
//
//	realm.OnMemberJoin(func(peerID string) {
//	    fmt.Println("新成员加入:", peerID[:8])
//	})
func (r *Realm) OnMemberJoin(handler func(peerID string)) error {
	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}

	eventBus := r.internal.EventBus()
	if eventBus == nil {
		return fmt.Errorf("event bus not available")
	}

	sub, err := eventBus.Subscribe(new(types.EvtRealmMemberJoined))
	if err != nil {
		return fmt.Errorf("subscribe member joined event: %w", err)
	}

	// 启动后台 goroutine 处理事件
	go func() {
		defer sub.Close()
		for evt := range sub.Out() {
			if e, ok := evt.(*types.EvtRealmMemberJoined); ok {
				// 检查是否是当前 Realm 的事件
				if string(e.RealmID) == r.ID() {
					handler(string(e.MemberID))
				}
			}
		}
	}()

	return nil
}

// OnMemberLeave 注册成员离开回调
//
// 当成员离开 Realm 时调用回调函数。
// 回调在后台 goroutine 中执行，不会阻塞事件处理。
//
// 注意：回调函数应该快速返回，避免长时间阻塞。
// 如需执行耗时操作，请在回调中启动新的 goroutine。
//
// 示例：
//
//	realm.OnMemberLeave(func(peerID string) {
//	    fmt.Println("成员离开:", peerID[:8])
//	})
func (r *Realm) OnMemberLeave(handler func(peerID string)) error {
	if handler == nil {
		return fmt.Errorf("handler cannot be nil")
	}

	eventBus := r.internal.EventBus()
	if eventBus == nil {
		return fmt.Errorf("event bus not available")
	}

	sub, err := eventBus.Subscribe(new(types.EvtRealmMemberLeft))
	if err != nil {
		return fmt.Errorf("subscribe member left event: %w", err)
	}

	// 启动后台 goroutine 处理事件
	go func() {
		defer sub.Close()
		for evt := range sub.Out() {
			if e, ok := evt.(*types.EvtRealmMemberLeft); ok {
				// 检查是否是当前 Realm 的事件
				if string(e.RealmID) == r.ID() {
					handler(string(e.MemberID))
				}
			}
		}
	}()

	return nil
}

// ════════════════════════════════════════════════════════════════════════════
//                              健康状态
// ════════════════════════════════════════════════════════════════════════════

// Health 返回域健康状态
//
// 返回当前 Realm 的基本健康指标。
// 更详细的监控指标请使用 Metrics API。
func (r *Realm) Health() *RealmHealth {
	memberCount := r.MemberCount()

	// 根据成员数量判断状态
	var status string
	switch memberCount {
	case 0:
		status = "isolated" // 没有其他成员
	case 1:
		status = "minimal" // 只有自己
	default:
		status = "healthy"
	}

	return &RealmHealth{
		Status:      status,
		MemberCount: memberCount,
		// ActivePeers 和 MessagesPerSec 需要更复杂的统计，
		// 当前版本暂不提供，留待后续通过 Metrics API 实现
	}
}

// RealmHealth 域健康状态
type RealmHealth struct {
	// Status 状态：healthy, minimal, isolated
	Status string

	// MemberCount 成员数量
	MemberCount int

	// ActivePeers 活跃连接数（暂未实现）
	ActivePeers int

	// MessagesPerSec 消息吞吐率（暂未实现）
	MessagesPerSec float64
}

// ════════════════════════════════════════════════════════════════════════════
//                              通信服务（多协议入口）
// ════════════════════════════════════════════════════════════════════════════

// Messaging 返回消息服务
//
// 用于点对点消息传递（请求-响应模式）。
//
// 示例：
//
//	messaging := realm.Messaging()
//	messaging.RegisterHandler("chat", chatHandler)
//	resp, _ := messaging.Send(ctx, peerID, "chat", []byte("hello"))
func (r *Realm) Messaging() *Messaging {
	return &Messaging{internal: r.internal.Messaging()}
}

// PubSub 返回发布订阅服务
//
// 用于主题发布订阅（一对多广播）。
//
// 示例：
//
//	pubsub := realm.PubSub()
//	topic, _ := pubsub.Join("room/general")
//	topic.Publish(ctx, []byte("hello everyone"))
func (r *Realm) PubSub() *PubSub {
	return &PubSub{internal: r.internal.PubSub()}
}

// Streams 返回流服务
//
// 用于双向流通信（长连接、原始字节流）。
//
// 示例：
//
//	streams := realm.Streams()
//	stream, _ := streams.Open(ctx, peerID, "file-transfer")
//	stream.Write(fileData)
func (r *Realm) Streams() *Streams {
	return &Streams{internal: r.internal.Streams()}
}

// Liveness 返回存活检测服务
//
// 用于节点健康检查和连接质量监控。
//
// 示例：
//
//	liveness := realm.Liveness()
//	rtt, _ := liveness.Ping(ctx, peerID)
//	status := liveness.Status(peerID)
func (r *Realm) Liveness() *Liveness {
	return &Liveness{internal: r.internal.Liveness()}
}

// ════════════════════════════════════════════════════════════════════════════
//                              生命周期
// ════════════════════════════════════════════════════════════════════════════

// Leave 离开域
//
// 离开后无法继续使用通信服务。
func (r *Realm) Leave() error {
	ctx := context.Background()
	return r.internal.Leave(ctx)
}

// Close 关闭域（Leave 的别名）
func (r *Realm) Close() error {
	return r.Leave()
}

// ════════════════════════════════════════════════════════════════════════════
//                              连接管理
// ════════════════════════════════════════════════════════════════════════════

// Connect 连接 Realm 成员或潜在成员
//
// 支持多种输入格式（自动检测）：
//   - ConnectionTicket: dep2p://base64...（便于分享的票据）
//   - Full Address: /ip4/x.x.x.x/udp/port/quic-v1/p2p/12D3KooW...
//   - 纯 NodeID: 12D3KooW...（通过 DHT 自动发现地址）
//
// 连接流程（自动处理）：
//  1. 解析 target，提取 NodeID 和地址提示
//  2. 如果目标已是成员，直接连接
//  3. 如果目标不是成员，先建立底层连接，等待 PSK 认证完成
//  4. 认证完成后返回已认证的连接
//
// 连接优先级：直连 → 打洞 → Relay 保底。
//
// ★ 返回成功时保证可通信（传输层 + PSK 认证完成）
//
// 示例：
//
//	// 使用票据连接（推荐，便于分享）
//	conn, err := realm.Connect(ctx, "dep2p://eyJ...")
//
//	// 使用完整地址连接
//	conn, err := realm.Connect(ctx, "/ip4/1.2.3.4/udp/4001/quic-v1/p2p/12D3KooW...")
//
//	// 使用纯 NodeID 连接（需要 DHT 发现）
//	conn, err := realm.Connect(ctx, "12D3KooWA...")
func (r *Realm) Connect(ctx context.Context, target string) (interfaces.Connection, error) {
	return r.internal.Connect(ctx, target)
}

// ConnectWithHint 使用地址提示连接 Realm 成员
//
// 与 Connect 类似，但允许用户提供地址提示来加速连接。
// 提示地址会被优先尝试，如果失败则回退到自动发现流程。
//
// 示例：
//
//	hints := []string{"/ip4/192.168.1.100/udp/4001/quic-v1"}
//	conn, err := realm.ConnectWithHint(ctx, "12D3KooWA...", hints)
func (r *Realm) ConnectWithHint(ctx context.Context, target string, hints []string) (interfaces.Connection, error) {
	return r.internal.ConnectWithHint(ctx, target, hints)
}

// ════════════════════════════════════════════════════════════════════════════
//                              内部方法（不暴露给用户）
// ════════════════════════════════════════════════════════════════════════════

// 以下方法在 interfaces.Realm 中存在，但在用户 API 中不暴露：
//
// - Join(ctx) error                  - 由 Node.JoinRealm() 内部调用
// - Router() Router                  - 内部路由，用户不需要
// - Gateway() Gateway                - 内部网关，用户不需要
// - PSK() []byte                     - 安全敏感，不暴露
// - Authenticate(...)                - 内部认证，用户不需要
// - GenerateProof(...)               - 内部认证，用户不需要
