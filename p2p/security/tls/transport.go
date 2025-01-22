package dep2ptls

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"runtime/debug"

	"github.com/dep2p/go-dep2p/core/canonicallog"
	ci "github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/protocol"
	"github.com/dep2p/go-dep2p/core/sec"
	tptu "github.com/dep2p/go-dep2p/p2p/net/upgrader"

	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
)

// ID 是协议标识符(用于多流协议协商)
const ID = "/tls/1.0.0"

// Transport 为对等节点构建安全通信会话
type Transport struct {
	identity *Identity // 身份信息

	localPeer  peer.ID       // 本地节点ID
	privKey    ci.PrivKey    // 本地私钥
	muxers     []protocol.ID // 支持的多路复用器列表
	protocolID protocol.ID   // 协议标识符
}

var _ sec.SecureTransport = &Transport{}

// New 创建一个TLS加密传输层
// 参数:
//   - id: protocol.ID 协议标识符
//   - key: ci.PrivKey 私钥
//   - muxers: []tptu.StreamMuxer 多路复用器列表
//
// 返回值:
//   - *Transport 传输层实例
//   - error 错误信息
func New(id protocol.ID, key ci.PrivKey, muxers []tptu.StreamMuxer) (*Transport, error) {
	// 从私钥生成节点ID
	localPeer, err := peer.IDFromPrivateKey(key)
	if err != nil {
		log.Debugf("从私钥生成节点ID时出错: %s", err)
		return nil, err
	}
	// 提取多路复用器ID
	muxerIDs := make([]protocol.ID, 0, len(muxers))
	for _, m := range muxers {
		muxerIDs = append(muxerIDs, m.ID)
	}
	// 创建传输层实例
	t := &Transport{
		protocolID: id,
		localPeer:  localPeer,
		privKey:    key,
		muxers:     muxerIDs,
	}

	// 创建身份信息
	identity, err := NewIdentity(key)
	if err != nil {
		log.Debugf("创建TLS身份信息时出错: %s", err)
		return nil, err
	}
	t.identity = identity
	return t, nil
}

// SecureInbound 作为服务端执行TLS握手
// 参数:
//   - ctx: context.Context 上下文
//   - insecure: net.Conn 未加密连接
//   - p: peer.ID 对端ID(为空时接受任意对端)
//
// 返回值:
//   - sec.SecureConn 安全连接
//   - error 错误信息
func (t *Transport) SecureInbound(ctx context.Context, insecure net.Conn, p peer.ID) (sec.SecureConn, error) {
	// 获取TLS配置和密钥通道
	config, keyCh := t.identity.ConfigForPeer(p)
	// 转换多路复用器ID为字符串
	muxers := make([]string, 0, len(t.muxers))
	for _, muxer := range t.muxers {
		muxers = append(muxers, string(muxer))
	}
	// TLS的ALPN选择让服务器选择协议,优先考虑服务器的偏好
	// 我们想要优先考虑客户端的偏好
	getConfigForClient := config.GetConfigForClient
	config.GetConfigForClient = func(info *tls.ClientHelloInfo) (*tls.Config, error) {
	alpnLoop:
		for _, proto := range info.SupportedProtos {
			for _, m := range muxers {
				if m == proto {
					// 找到匹配项,选择此多路复用器,因为这是客户端的偏好
					// 这里不需要添加"dep2p"条目
					config.NextProtos = []string{proto}
					break alpnLoop
				}
			}
		}
		if getConfigForClient != nil {
			return getConfigForClient(info)
		}
		return config, nil
	}
	config.NextProtos = append(muxers, config.NextProtos...)
	cs, err := t.handshake(ctx, tls.Server(insecure, config), keyCh)
	if err != nil {
		addr, maErr := manet.FromNetAddr(insecure.RemoteAddr())
		if maErr == nil {
			canonicallog.LogPeerStatus(100, p, addr, "handshake_failure", "tls", "err", err.Error())
		}
		insecure.Close()
	}
	return cs, err
}

// SecureOutbound 作为客户端执行TLS握手
// 参数:
//   - ctx: context.Context 上下文
//   - insecure: net.Conn 未加密连接
//   - p: peer.ID 对端ID
//
// 返回值:
//   - sec.SecureConn 安全连接
//   - error 错误信息
//
// 注意:
//   - 如果服务器不接受证书,SecureOutbound不会返回错误
//   - 这是因为在TLS 1.3中,客户端在同一个飞行中发送其证书和ClientFinished,并且可以立即发送应用数据
//   - 如果握手失败,服务器将关闭连接。客户端在调用Read时会在1个RTT后注意到这一点
func (t *Transport) SecureOutbound(ctx context.Context, insecure net.Conn, p peer.ID) (sec.SecureConn, error) {
	config, keyCh := t.identity.ConfigForPeer(p)
	muxers := make([]string, 0, len(t.muxers))
	for _, muxer := range t.muxers {
		muxers = append(muxers, (string)(muxer))
	}
	// 将首选多路复用器列表添加到TLS配置中
	config.NextProtos = append(muxers, config.NextProtos...)
	cs, err := t.handshake(ctx, tls.Client(insecure, config), keyCh)
	if err != nil {
		insecure.Close()
	}
	return cs, err
}

// handshake 执行TLS握手
// 参数:
//   - ctx: context.Context 上下文
//   - tlsConn: *tls.Conn TLS连接
//   - keyCh: <-chan ci.PubKey 公钥通道
//
// 返回值:
//   - sec.SecureConn 安全连接
//   - error 错误信息
func (t *Transport) handshake(ctx context.Context, tlsConn *tls.Conn, keyCh <-chan ci.PubKey) (_sconn sec.SecureConn, err error) {
	defer func() {
		if rerr := recover(); rerr != nil {
			fmt.Fprintf(os.Stderr, "TLS握手中发生panic: %s\n%s\n", rerr, debug.Stack())
			err = fmt.Errorf("TLS握手中发生panic: %s", rerr)
		}
	}()

	// 执行握手
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		log.Debugf("TLS握手失败: %s", err)
		return nil, err
	}

	// 此时应该准备就绪,不要阻塞
	var remotePubKey ci.PubKey
	select {
	case remotePubKey = <-keyCh:
	default:
	}
	if remotePubKey == nil {
		log.Debugf("go-dep2p tls错误: 预期远程公钥已设置")
		return nil, fmt.Errorf("go-dep2p tls错误: 预期远程公钥已设置")
	}

	return t.setupConn(tlsConn, remotePubKey)
}

// setupConn 设置安全连接
// 参数:
//   - tlsConn: *tls.Conn TLS连接
//   - remotePubKey: ci.PubKey 远程公钥
//
// 返回值:
//   - sec.SecureConn 安全连接
//   - error 错误信息
func (t *Transport) setupConn(tlsConn *tls.Conn, remotePubKey ci.PubKey) (sec.SecureConn, error) {
	remotePeerID, err := peer.IDFromPublicKey(remotePubKey)
	if err != nil {
		log.Debugf("从公钥生成节点ID时出错: %s", err)
		return nil, err
	}

	nextProto := tlsConn.ConnectionState().NegotiatedProtocol
	// 特殊的ALPN扩展值"dep2p"用于不支持早期多路复用器协商的dep2p版本
	// 如果我们看到选择了这个特殊值,这意味着我们正在与不支持早期多路复用器协商的版本进行握手
	// 在这种情况下返回空nextProto以表示没有选择多路复用器
	if nextProto == "dep2p" {
		nextProto = ""
	}

	return &conn{
		Conn:         tlsConn,
		localPeer:    t.localPeer,
		remotePeer:   remotePeerID,
		remotePubKey: remotePubKey,
		connectionState: network.ConnectionState{
			StreamMultiplexer:         protocol.ID(nextProto),
			UsedEarlyMuxerNegotiation: nextProto != "",
		},
	}, nil
}

// ID 返回协议标识符
// 返回值:
//   - protocol.ID 协议标识符
func (t *Transport) ID() protocol.ID {
	return t.protocolID
}
