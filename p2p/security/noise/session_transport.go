package noise

import (
	"context"
	"net"

	"github.com/dep2p/go-dep2p/core/canonicallog"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
	"github.com/dep2p/go-dep2p/core/sec"
	"github.com/dep2p/go-dep2p/p2p/security/noise/pb"

	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

// SessionOption 定义了一个函数类型,用于配置 SessionTransport
type SessionOption = func(*SessionTransport) error

// Prologue 为 Noise 会话设置序言
// 只有当双方设置相同的序言时,握手才能成功完成
// 参考 https://noiseprotocol.org/noise.html#prologue 获取详情
//
// 参数:
//   - prologue: []byte 序言内容
//
// 返回值:
//   - SessionOption 返回一个配置函数
func Prologue(prologue []byte) SessionOption {
	return func(s *SessionTransport) error {
		s.prologue = prologue
		return nil
	}
}

// EarlyDataHandler 定义了应用层负载的处理接口
// 用于处理第二条(如果是响应方)或第三条(如果是发起方)握手消息
// 并定义了处理对方早期数据的逻辑
// 注意:第二条握手消息中的早期数据是加密的,但此时对端身份尚未验证
type EarlyDataHandler interface {
	// Send 对于发起方,在发送第三条握手消息前调用,定义第三条消息的应用层负载
	// 对于响应方,在发送第二条握手消息前调用
	//
	// 参数:
	//   - ctx: context.Context 上下文
	//   - conn: net.Conn 网络连接
	//   - p: peer.ID 对端ID
	//
	// 返回值:
	//   - *pb.NoiseExtensions Noise扩展数据
	Send(context.Context, net.Conn, peer.ID) *pb.NoiseExtensions

	// Received 对于发起方,在收到响应方的第二条握手消息时调用
	// 对于响应方,在收到发起方的第三条握手消息时调用
	//
	// 参数:
	//   - ctx: context.Context 上下文
	//   - conn: net.Conn 网络连接
	//   - ext: *pb.NoiseExtensions Noise扩展数据
	//
	// 返回值:
	//   - error 处理过程中的错误
	Received(context.Context, net.Conn, *pb.NoiseExtensions) error
}

// EarlyData 为发起方和响应方角色设置 EarlyDataHandler
// 参考 EarlyDataHandler 获取更多详情
//
// 参数:
//   - initiator: EarlyDataHandler 发起方的处理器
//   - responder: EarlyDataHandler 响应方的处理器
//
// 返回值:
//   - SessionOption 返回一个配置函数
func EarlyData(initiator, responder EarlyDataHandler) SessionOption {
	return func(s *SessionTransport) error {
		s.initiatorEarlyDataHandler = initiator
		s.responderEarlyDataHandler = responder
		return nil
	}
}

// DisablePeerIDCheck 禁用对 noise 连接的远程对等点 ID 检查
// 对于出站连接,这相当于使用空的对等点 ID 调用 SecureInbound
// 这容易受到中间人攻击,因为我们不验证远程对等点的身份
//
// 返回值:
//   - SessionOption 返回一个配置函数
func DisablePeerIDCheck() SessionOption {
	return func(s *SessionTransport) error {
		s.disablePeerIDCheck = true
		return nil
	}
}

var _ sec.SecureTransport = &SessionTransport{}

// SessionTransport 可用于提供每个连接的选项
type SessionTransport struct {
	t *Transport
	// 选项
	prologue           []byte
	disablePeerIDCheck bool

	protocolID protocol.ID

	initiatorEarlyDataHandler, responderEarlyDataHandler EarlyDataHandler
}

// SecureInbound 作为响应方运行 Noise 握手
// 如果 p 为空,则接受来自任何对等点的连接
//
// 参数:
//   - ctx: context.Context 上下文
//   - insecure: net.Conn 不安全的网络连接
//   - p: peer.ID 对等点ID
//
// 返回值:
//   - sec.SecureConn 安全连接
//   - error 握手过程中的错误
func (i *SessionTransport) SecureInbound(ctx context.Context, insecure net.Conn, p peer.ID) (sec.SecureConn, error) {
	checkPeerID := !i.disablePeerIDCheck && p != ""
	c, err := newSecureSession(i.t, ctx, insecure, p, i.prologue, i.initiatorEarlyDataHandler, i.responderEarlyDataHandler, false, checkPeerID)
	if err != nil {
		addr, maErr := manet.FromNetAddr(insecure.RemoteAddr())
		if maErr == nil {
			canonicallog.LogPeerStatus(100, p, addr, "handshake_failure", "noise", "err", err.Error())
		}
	}
	return c, err
}

// SecureOutbound 作为发起方运行 Noise 握手
//
// 参数:
//   - ctx: context.Context 上下文
//   - insecure: net.Conn 不安全的网络连接
//   - p: peer.ID 对等点ID
//
// 返回值:
//   - sec.SecureConn 安全连接
//   - error 握手过程中的错误
func (i *SessionTransport) SecureOutbound(ctx context.Context, insecure net.Conn, p peer.ID) (sec.SecureConn, error) {
	return newSecureSession(i.t, ctx, insecure, p, i.prologue, i.initiatorEarlyDataHandler, i.responderEarlyDataHandler, true, !i.disablePeerIDCheck)
}

// ID 返回协议ID
//
// 返回值:
//   - protocol.ID 协议标识符
func (i *SessionTransport) ID() protocol.ID {
	return i.protocolID
}
