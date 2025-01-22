package noise

import (
	"context"
	"net"

	"github.com/dep2p/go-dep2p/core/canonicallog"
	"github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
	"github.com/dep2p/go-dep2p/core/sec"
	tptu "github.com/dep2p/go-dep2p/p2p/net/upgrader"
	"github.com/dep2p/go-dep2p/p2p/security/noise/pb"
	logging "github.com/dep2p/log"

	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

var log = logging.Logger("p2p-security-noise")

// ID 是 noise 协议的标识符
const ID = "/noise"

// maxProtoNum 是最大协议数量限制
const maxProtoNum = 100

// Transport 实现了 noise 安全传输
// 字段:
//   - protocolID: 协议标识符
//   - localID: 本地节点 ID
//   - privateKey: 本地私钥
//   - muxers: 支持的多路复用器列表
type Transport struct {
	protocolID protocol.ID
	localID    peer.ID
	privateKey crypto.PrivKey
	muxers     []protocol.ID
}

var _ sec.SecureTransport = &Transport{}

// New 使用给定的私钥创建一个新的 Noise 传输实例
// 参数:
//   - id: 协议标识符
//   - privkey: 用作 dep2p 身份的私钥
//   - muxers: 支持的多路复用器列表
//
// 返回值:
//   - *Transport: 新创建的传输实例
//   - error: 创建过程中的错误
func New(id protocol.ID, privkey crypto.PrivKey, muxers []tptu.StreamMuxer) (*Transport, error) {
	// 从私钥生成节点 ID
	localID, err := peer.IDFromPrivateKey(privkey)
	if err != nil {
		log.Debugf("从私钥生成节点 ID 时出错: %s", err)
		return nil, err
	}

	// 提取多路复用器的协议 ID
	muxerIDs := make([]protocol.ID, 0, len(muxers))
	for _, m := range muxers {
		muxerIDs = append(muxerIDs, m.ID)
	}

	// 创建并返回传输实例
	return &Transport{
		protocolID: id,
		localID:    localID,
		privateKey: privkey,
		muxers:     muxerIDs,
	}, nil
}

// SecureInbound 作为响应方执行 Noise 握手
// 参数:
//   - ctx: 上下文
//   - insecure: 未加密的网络连接
//   - p: 对端节点 ID，如果为空则接受任何节点的连接
//
// 返回值:
//   - sec.SecureConn: 安全连接
//   - error: 握手过程中的错误
func (t *Transport) SecureInbound(ctx context.Context, insecure net.Conn, p peer.ID) (sec.SecureConn, error) {
	// 创建响应方早期数据处理器
	responderEDH := newTransportEDH(t)
	// 创建安全会话
	c, err := newSecureSession(t, ctx, insecure, p, nil, nil, responderEDH, false, p != "")
	if err != nil {
		// 如果握手失败，记录日志
		addr, maErr := manet.FromNetAddr(insecure.RemoteAddr())
		if maErr == nil {
			canonicallog.LogPeerStatus(100, p, addr, "handshake_failure", "noise", "err", err.Error())
		}
	}
	return SessionWithConnState(c, responderEDH.MatchMuxers(false)), err
}

// SecureOutbound 作为发起方执行 Noise 握手
// 参数:
//   - ctx: 上下文
//   - insecure: 未加密的网络连接
//   - p: 对端节点 ID
//
// 返回值:
//   - sec.SecureConn: 安全连接
//   - error: 握手过程中的错误
func (t *Transport) SecureOutbound(ctx context.Context, insecure net.Conn, p peer.ID) (sec.SecureConn, error) {
	// 创建发起方早期数据处理器
	initiatorEDH := newTransportEDH(t)
	// 创建安全会话
	c, err := newSecureSession(t, ctx, insecure, p, nil, initiatorEDH, nil, true, true)
	if err != nil {
		log.Debugf("创建安全会话时出错: %s", err)
		return nil, err
	}
	return SessionWithConnState(c, initiatorEDH.MatchMuxers(true)), err
}

// WithSessionOptions 使用给定的会话选项创建新的会话传输
// 参数:
//   - opts: 会话选项列表
//
// 返回值:
//   - *SessionTransport: 新的会话传输实例
//   - error: 创建过程中的错误
func (t *Transport) WithSessionOptions(opts ...SessionOption) (*SessionTransport, error) {
	st := &SessionTransport{t: t, protocolID: t.protocolID}
	for _, opt := range opts {
		if err := opt(st); err != nil {
			log.Debugf("应用会话选项时出错: %s", err)
			return nil, err
		}
	}
	return st, nil
}

// ID 返回传输的协议标识符
// 返回值:
//   - protocol.ID: 协议标识符
func (t *Transport) ID() protocol.ID {
	return t.protocolID
}

// matchMuxers 匹配发起方和响应方支持的多路复用器
// 参数:
//   - initiatorMuxers: 发起方支持的多路复用器列表
//   - responderMuxers: 响应方支持的多路复用器列表
//
// 返回值:
//   - protocol.ID: 匹配的多路复用器，如果没有匹配则返回空字符串
func matchMuxers(initiatorMuxers, responderMuxers []protocol.ID) protocol.ID {
	for _, initMuxer := range initiatorMuxers {
		for _, respMuxer := range responderMuxers {
			if initMuxer == respMuxer {
				return initMuxer
			}
		}
	}
	return ""
}

// transportEarlyDataHandler 处理传输早期数据
// 字段:
//   - transport: 传输实例
//   - receivedMuxers: 接收到的多路复用器列表
type transportEarlyDataHandler struct {
	transport      *Transport
	receivedMuxers []protocol.ID
}

var _ EarlyDataHandler = &transportEarlyDataHandler{}

// newTransportEDH 创建新的传输早期数据处理器
// 参数:
//   - t: 传输实例
//
// 返回值:
//   - *transportEarlyDataHandler: 新的早期数据处理器
func newTransportEDH(t *Transport) *transportEarlyDataHandler {
	return &transportEarlyDataHandler{transport: t}
}

// Send 发送早期数据
// 参数:
//   - context.Context: 上下文
//   - net.Conn: 网络连接
//   - peer.ID: 对端节点 ID
//
// 返回值:
//   - *pb.NoiseExtensions: 包含多路复用器信息的扩展数据
func (i *transportEarlyDataHandler) Send(context.Context, net.Conn, peer.ID) *pb.NoiseExtensions {
	return &pb.NoiseExtensions{
		StreamMuxers: protocol.ConvertToStrings(i.transport.muxers),
	}
}

// Received 处理接收到的早期数据
// 参数:
//   - context.Context: 上下文
//   - net.Conn: 网络连接
//   - extension: 接收到的扩展数据
//
// 返回值:
//   - error: 处理过程中的错误
func (i *transportEarlyDataHandler) Received(_ context.Context, _ net.Conn, extension *pb.NoiseExtensions) error {
	// 出于安全考虑，丢弃超出扩展限制的消息
	if extension != nil && len(extension.StreamMuxers) <= maxProtoNum {
		i.receivedMuxers = protocol.ConvertFromStrings(extension.GetStreamMuxers())
	}
	return nil
}

// MatchMuxers 匹配多路复用器
// 参数:
//   - isInitiator: 是否为发起方
//
// 返回值:
//   - protocol.ID: 匹配的多路复用器
func (i *transportEarlyDataHandler) MatchMuxers(isInitiator bool) protocol.ID {
	if isInitiator {
		return matchMuxers(i.transport.muxers, i.receivedMuxers)
	}
	return matchMuxers(i.receivedMuxers, i.transport.muxers)
}
