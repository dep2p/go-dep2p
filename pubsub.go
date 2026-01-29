package dep2p

import (
	"context"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ════════════════════════════════════════════════════════════════════════════
//                              用户 API: PubSub
// ════════════════════════════════════════════════════════════════════════════

// PubSub 用户级发布订阅服务 API
//
// PubSub 提供基于 GossipSub 的发布订阅功能，支持多主题并行。
//
// 使用示例：
//
//	pubsub := realm.PubSub()
//	
//	// 加入多个主题
//	chatTopic, _ := pubsub.Join("room/general")
//	alertTopic, _ := pubsub.Join("alerts")
//	
//	// 发布消息
//	chatTopic.Publish(ctx, []byte("hello everyone"))
//	
//	// 订阅消息
//	sub, _ := chatTopic.Subscribe()
//	for {
//	    msg, _ := sub.Next(ctx)
//	    fmt.Printf("From %s: %s\n", msg.From, msg.Data)
//	}
type PubSub struct {
	internal interfaces.PubSub
}

// ════════════════════════════════════════════════════════════════════════════
//                              主题管理
// ════════════════════════════════════════════════════════════════════════════

// Join 加入主题
//
// 返回 Topic 对象，用于发布和订阅消息。
// 一个 Realm 可以同时加入多个主题。
//
// 参数：
//   - topic: 主题名称（如 "room/general", "alerts", "state/sync"）
//
// 为什么需要 Join？
//   1. 资源管理：Topic 是有状态对象，需要显式创建和销毁
//   2. Mesh 维护：GossipSub 需要维护 D-regular graph
//   3. 生命周期：不同 Topic 可以独立关闭
//   4. 语义清晰：明确表达"我要参与这个主题"的意图
//
// 示例：
//
//	chatTopic, err := pubsub.Join("room/general")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer chatTopic.Close()
func (p *PubSub) Join(topic string) (*Topic, error) {
	internal, err := p.internal.Join(topic)
	if err != nil {
		return nil, err
	}
	return &Topic{internal: internal}, nil
}

// GetTopics 获取所有已加入的主题
//
// 返回主题名称列表。
func (p *PubSub) GetTopics() []string {
	return p.internal.GetTopics()
}

// ListPeers 列出指定主题的所有节点
//
// 参数：
//   - topic: 主题名称
//
// 返回：该主题中的所有节点 ID 列表
func (p *PubSub) ListPeers(topic string) []string {
	return p.internal.ListPeers(topic)
}

// ════════════════════════════════════════════════════════════════════════════
//                              生命周期
// ════════════════════════════════════════════════════════════════════════════

// Close 关闭服务
func (p *PubSub) Close() error {
	return p.internal.Close()
}

// ════════════════════════════════════════════════════════════════════════════
//                              用户 API: Topic
// ════════════════════════════════════════════════════════════════════════════

// Topic 用户级主题 API
//
// Topic 代表一个已加入的发布订阅主题。
// 通过 Topic 对象进行发布和订阅操作。
//
// 使用示例：
//
//	topic, _ := pubsub.Join("room/general")
//	defer topic.Close()
//	
//	// 发布消息
//	topic.Publish(ctx, []byte("hello"))
//	
//	// 订阅消息
//	sub, _ := topic.Subscribe()
//	msg, _ := sub.Next(ctx)
type Topic struct {
	internal interfaces.Topic
}

// ════════════════════════════════════════════════════════════════════════════
//                              主题信息
// ════════════════════════════════════════════════════════════════════════════

// String 返回主题名称
func (t *Topic) String() string {
	return t.internal.String()
}

// ListPeers 列出此主题的所有节点
//
// 返回当前在此主题中的所有节点 ID。
func (t *Topic) ListPeers() []string {
	return t.internal.ListPeers()
}

// ════════════════════════════════════════════════════════════════════════════
//                              发布消息
// ════════════════════════════════════════════════════════════════════════════

// Publish 发布消息到此主题
//
// 参数：
//   - ctx: 上下文（用于超时控制）
//   - data: 消息数据
//
// 消息会通过 GossipSub 协议广播到所有订阅此主题的节点。
//
// 示例：
//
//	err := topic.Publish(ctx, []byte("hello everyone"))
//	if err != nil {
//	    log.Fatal(err)
//	}
func (t *Topic) Publish(ctx context.Context, data []byte) error {
	return t.internal.Publish(ctx, data)
}

// ════════════════════════════════════════════════════════════════════════════
//                              订阅消息
// ════════════════════════════════════════════════════════════════════════════

// Subscribe 订阅此主题
//
// 返回 Subscription 对象，用于接收消息。
// 一个 Topic 可以创建多个 Subscription。
//
// 示例：
//
//	sub, err := topic.Subscribe()
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer sub.Cancel()
//	
//	for {
//	    msg, err := sub.Next(ctx)
//	    if err != nil {
//	        break
//	    }
//	    fmt.Printf("From %s: %s\n", msg.From, msg.Data)
//	}
func (t *Topic) Subscribe() (*Subscription, error) {
	internal, err := t.internal.Subscribe()
	if err != nil {
		return nil, err
	}
	return &Subscription{internal: internal}, nil
}

// ════════════════════════════════════════════════════════════════════════════
//                              生命周期
// ════════════════════════════════════════════════════════════════════════════

// Close 关闭主题
//
// 离开主题，释放资源。
// 关闭后不再收到该主题的消息，也无法继续发布。
func (t *Topic) Close() error {
	return t.internal.Close()
}

// ════════════════════════════════════════════════════════════════════════════
//                              用户 API: Subscription
// ════════════════════════════════════════════════════════════════════════════

// Subscription 用户级订阅 API
//
// Subscription 代表对一个主题的订阅。
//
// 使用示例：
//
//	sub, _ := topic.Subscribe()
//	defer sub.Cancel()
//	
//	for {
//	    msg, err := sub.Next(ctx)
//	    if err != nil {
//	        break
//	    }
//	    // 处理消息
//	}
type Subscription struct {
	internal interfaces.TopicSubscription
}

// ════════════════════════════════════════════════════════════════════════════
//                              接收消息
// ════════════════════════════════════════════════════════════════════════════

// Next 获取下一条消息
//
// 阻塞等待直到收到消息或上下文取消。
//
// 参数：
//   - ctx: 上下文（用于超时控制和取消）
//
// 返回：
//   - *Message: 消息对象
//   - error: 错误信息（如订阅已取消、上下文已取消）
//
// 示例：
//
//	msg, err := sub.Next(ctx)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Printf("From %s: %s\n", msg.From, msg.Data)
func (s *Subscription) Next(ctx context.Context) (*Message, error) {
	return s.internal.Next(ctx)
}

// ════════════════════════════════════════════════════════════════════════════
//                              生命周期
// ════════════════════════════════════════════════════════════════════════════

// Cancel 取消订阅
//
// 取消后，Next() 会立即返回错误。
func (s *Subscription) Cancel() {
	s.internal.Cancel()
}

// ════════════════════════════════════════════════════════════════════════════
//                              类型别名（方便用户使用）
// ════════════════════════════════════════════════════════════════════════════

// Message 消息结构
type Message = interfaces.Message
