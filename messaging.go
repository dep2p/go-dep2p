package dep2p

import (
	"context"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// ════════════════════════════════════════════════════════════════════════════
//                              用户 API: Messaging
// ════════════════════════════════════════════════════════════════════════════

// Messaging 用户级消息服务 API
//
// Messaging 提供点对点消息传递（请求-响应模式），支持多协议。
//
// 使用示例：
//
//	messaging := realm.Messaging()
//	
//	// 注册多个协议处理器
//	messaging.RegisterHandler("chat", chatHandler)
//	messaging.RegisterHandler("rpc", rpcHandler)
//	
//	// 发送不同协议的消息
//	resp, _ := messaging.Send(ctx, peerID, "chat", []byte("hello"))
//	resp, _ = messaging.Send(ctx, peerID, "rpc", rpcPayload)
type Messaging struct {
	internal interfaces.Messaging
}

// ════════════════════════════════════════════════════════════════════════════
//                              发送消息
// ════════════════════════════════════════════════════════════════════════════

// Send 发送消息并等待响应
//
// 参数：
//   - ctx: 上下文（用于超时控制）
//   - peerID: 目标节点 ID
//   - protocol: 协议标识（如 "chat", "rpc", "file-meta"）
//   - data: 消息数据
//
// 返回：
//   - []byte: 响应数据
//   - error: 错误信息
//
// 协议 ID 组装：
//   用户传: "chat"
//   实际: /dep2p/app/<realmID>/messaging/chat/1.0.0
//
// 示例：
//
//	resp, err := messaging.Send(ctx, peerID, "chat", []byte("hello"))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	fmt.Println(string(resp))
func (m *Messaging) Send(ctx context.Context, peerID string, protocol string, data []byte) ([]byte, error) {
	return m.internal.Send(ctx, peerID, protocol, data)
}

// SendAsync 异步发送消息
//
// 返回一个 channel 用于接收响应，不阻塞调用者。
//
// 示例：
//
//	respCh, _ := messaging.SendAsync(ctx, peerID, "chat", []byte("hello"))
//	resp := <-respCh
//	if resp.Error != nil {
//	    log.Fatal(resp.Error)
//	}
//	fmt.Println(string(resp.Data))
func (m *Messaging) SendAsync(ctx context.Context, peerID string, protocol string, data []byte) (<-chan *Response, error) {
	return m.internal.SendAsync(ctx, peerID, protocol, data)
}

// ════════════════════════════════════════════════════════════════════════════
//                              注册处理器
// ════════════════════════════════════════════════════════════════════════════

// RegisterHandler 注册消息处理器
//
// 参数：
//   - protocol: 协议标识（如 "chat", "rpc"）
//   - handler: 消息处理函数
//
// 一个 Realm 可以注册多个协议处理器，互不干扰。
//
// 示例：
//
//	messaging.RegisterHandler("chat", func(ctx context.Context, req *Request) (*Response, error) {
//	    fmt.Printf("Received chat from %s: %s\n", req.From, req.Data)
//	    return &Response{Data: []byte("reply")}, nil
//	})
//	
//	messaging.RegisterHandler("rpc", func(ctx context.Context, req *Request) (*Response, error) {
//	    // 处理 RPC 调用
//	    result := processRPC(req.Data)
//	    return &Response{Data: result}, nil
//	})
func (m *Messaging) RegisterHandler(protocol string, handler MessageHandler) error {
	return m.internal.RegisterHandler(protocol, handler)
}

// UnregisterHandler 注销消息处理器
//
// 参数：
//   - protocol: 协议标识
//
// 示例：
//
//	messaging.UnregisterHandler("chat")
func (m *Messaging) UnregisterHandler(protocol string) error {
	return m.internal.UnregisterHandler(protocol)
}

// ════════════════════════════════════════════════════════════════════════════
//                              生命周期
// ════════════════════════════════════════════════════════════════════════════

// Close 关闭服务
func (m *Messaging) Close() error {
	return m.internal.Close()
}

// ════════════════════════════════════════════════════════════════════════════
//                              类型别名（方便用户使用）
// ════════════════════════════════════════════════════════════════════════════

// MessageHandler 消息处理函数类型
type MessageHandler = interfaces.MessageHandler

// Request 消息请求
type Request = interfaces.Request

// Response 消息响应
type Response = interfaces.Response
