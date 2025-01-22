package dep2pwebrtc

import (
	"context"
	"crypto"
	"encoding/hex"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	tpt "github.com/dep2p/go-dep2p/core/transport"
	"github.com/dep2p/go-dep2p/p2p/transport/webrtc/udpmux"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	"github.com/dep2p/go-dep2p/multiformats/multibase"
	"github.com/dep2p/go-dep2p/multiformats/multihash"
	"github.com/pion/webrtc/v4"
)

// connMultiaddrs 实现了 network.ConnMultiaddrs 接口,用于存储本地和远程的多地址
type connMultiaddrs struct {
	// local 本地多地址
	local ma.Multiaddr
	// remote 远程多地址
	remote ma.Multiaddr
}

// 确保 connMultiaddrs 实现了 network.ConnMultiaddrs 接口
var _ network.ConnMultiaddrs = &connMultiaddrs{}

// LocalMultiaddr 返回本地多地址
// 返回值:
//   - ma.Multiaddr 本地多地址
func (c *connMultiaddrs) LocalMultiaddr() ma.Multiaddr { return c.local }

// RemoteMultiaddr 返回远程多地址
// 返回值:
//   - ma.Multiaddr 远程多地址
func (c *connMultiaddrs) RemoteMultiaddr() ma.Multiaddr { return c.remote }

const (
	// candidateSetupTimeout 候选者设置超时时间为10秒
	candidateSetupTimeout = 10 * time.Second
	// DefaultMaxInFlightConnections 默认最大并发连接数为128
	// 这个值比其他传输协议(64)要高,因为在发送初始连接请求消息(STUN Binding request)后
	// 无法检测到对端是否已经离开。这些对端会占用一个goroutine直到连接超时。
	// 由于并行握手的数量仍然受资源管理器的限制,这个较高的数字是可以接受的。
	DefaultMaxInFlightConnections = 128
)

// listener 实现了 tpt.Listener 接口,用于监听 WebRTC 连接
type listener struct {
	// transport WebRTC 传输层对象
	transport *WebRTCTransport

	// mux UDP 多路复用器
	mux *udpmux.UDPMux

	// config WebRTC 配置
	config webrtc.Configuration
	// localFingerprint 本地 DTLS 指纹
	localFingerprint webrtc.DTLSFingerprint
	// localFingerprintMultibase 本地指纹的 multibase 编码
	localFingerprintMultibase string

	// localAddr 本地网络地址
	localAddr net.Addr
	// localMultiaddr 本地多地址
	localMultiaddr ma.Multiaddr

	// acceptQueue 缓冲的入站连接队列
	acceptQueue chan tpt.CapableConn

	// 用于控制监听器的生命周期
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// 确保 listener 实现了 tpt.Listener 接口
var _ tpt.Listener = &listener{}

// newListener 创建一个新的 WebRTC 监听器
// 参数:
//   - transport: WebRTCTransport WebRTC 传输层对象
//   - laddr: ma.Multiaddr 本地多地址
//   - socket: net.PacketConn UDP socket 连接
//   - config: webrtc.Configuration WebRTC 配置
//
// 返回值:
//   - *listener 新创建的监听器
//   - error 错误信息
func newListener(transport *WebRTCTransport, laddr ma.Multiaddr, socket net.PacketConn, config webrtc.Configuration) (*listener, error) {
	localFingerprints, err := config.Certificates[0].GetFingerprints()
	if err != nil {
		log.Debugf("获取证书指纹时出错: %s", err)
		return nil, err
	}

	localMh, err := hex.DecodeString(strings.ReplaceAll(localFingerprints[0].Value, ":", ""))
	if err != nil {
		log.Debugf("解码证书指纹时出错: %s", err)
		return nil, err
	}
	localMhBuf, err := multihash.Encode(localMh, multihash.SHA2_256)
	if err != nil {
		log.Debugf("编码证书指纹时出错: %s", err)
		return nil, err
	}
	localFpMultibase, err := multibase.Encode(multibase.Base64url, localMhBuf)
	if err != nil {
		log.Debugf("编码证书指纹时出错: %s", err)
		return nil, err
	}

	l := &listener{
		transport:                 transport,
		config:                    config,
		localFingerprint:          localFingerprints[0],
		localFingerprintMultibase: localFpMultibase,
		localMultiaddr:            laddr,
		localAddr:                 socket.LocalAddr(),
		acceptQueue:               make(chan tpt.CapableConn),
	}

	l.ctx, l.cancel = context.WithCancel(context.Background())
	l.mux = udpmux.NewUDPMux(socket)
	l.mux.Start()

	l.wg.Add(1)
	go func() {
		defer l.wg.Done()
		l.listen()
	}()

	return l, err
}

// listen 监听并处理入站连接
func (l *listener) listen() {
	// 接受连接需要实例化一个对等连接和一个噪声连接,这是昂贵的操作。
	// 因此我们限制正在处理的连接请求数量。从处理连接的那一刻起,
	// 直到它被 Accept 调用出队或以某种方式出错为止,连接被认为是正在处理中。
	inFlightSemaphore := make(chan struct{}, l.transport.maxInFlightConnections)
	for {
		select {
		case inFlightSemaphore <- struct{}{}:
		case <-l.ctx.Done():
			return
		}

		candidate, err := l.mux.Accept(l.ctx)
		if err != nil {
			if l.ctx.Err() == nil {
				log.Debugf("接受候选者失败: %s", err)
			}
			return
		}

		go func() {
			defer func() { <-inFlightSemaphore }()

			ctx, cancel := context.WithTimeout(l.ctx, candidateSetupTimeout)
			defer cancel()

			conn, err := l.handleCandidate(ctx, candidate)
			if err != nil {
				l.mux.RemoveConnByUfrag(candidate.Ufrag)
				log.Debugf("无法接受连接: %s: %v", candidate.Ufrag, err)
				return
			}

			select {
			case <-l.ctx.Done():
				log.Debug("监听器已关闭,丢弃连接")
				conn.Close()
			case l.acceptQueue <- conn:
				// acceptQueue 是一个无缓冲通道,所以这里会阻塞直到连接被接受
			}
		}()
	}
}

// handleCandidate 处理 ICE 候选者
// 参数:
//   - ctx: context.Context 上下文
//   - candidate: udpmux.Candidate ICE 候选者
//
// 返回值:
//   - tpt.CapableConn 建立的连接
//   - error 错误信息
func (l *listener) handleCandidate(ctx context.Context, candidate udpmux.Candidate) (tpt.CapableConn, error) {
	remoteMultiaddr, err := manet.FromNetAddr(candidate.Addr)
	if err != nil {
		log.Debugf("从网络地址创建多地址时出错: %s", err)
		return nil, err
	}
	if l.transport.gater != nil {
		localAddr, _ := ma.SplitFunc(l.localMultiaddr, func(c ma.Component) bool { return c.Protocol().Code == ma.P_CERTHASH })
		if !l.transport.gater.InterceptAccept(&connMultiaddrs{local: localAddr, remote: remoteMultiaddr}) {
			log.Debugf("连接被拦截")
			// 在我们能够向客户端发送错误之前,连接尝试被拒绝。
			// 这意味着连接尝试将超时。
			return nil, errors.New("连接被拦截")
		}
	}
	scope, err := l.transport.rcmgr.OpenConnection(network.DirInbound, false, remoteMultiaddr)
	if err != nil {
		log.Debugf("打开连接时出错: %s", err)
		return nil, err
	}
	conn, err := l.setupConnection(ctx, scope, remoteMultiaddr, candidate)
	if err != nil {
		scope.Done()
		log.Debugf("设置连接时出错: %s", err)
		return nil, err
	}
	if l.transport.gater != nil && !l.transport.gater.InterceptSecured(network.DirInbound, conn.RemotePeer(), conn) {
		conn.Close()
		log.Debugf("连接被拦截")
		return nil, errors.New("连接被拦截")
	}
	return conn, nil
}

// setupConnection 设置 WebRTC 连接
// 参数:
//   - ctx: context.Context 上下文
//   - scope: network.ConnManagementScope 连接管理作用域
//   - remoteMultiaddr: ma.Multiaddr 远程多地址
//   - candidate: udpmux.Candidate ICE 候选者
//
// 返回值:
//   - tpt.CapableConn 建立的连接
//   - error 错误信息
func (l *listener) setupConnection(
	ctx context.Context, scope network.ConnManagementScope,
	remoteMultiaddr ma.Multiaddr, candidate udpmux.Candidate,
) (tConn tpt.CapableConn, err error) {
	var w webRTCConnection
	defer func() {
		if err != nil {
			log.Debugf("设置连接时出错: %s", err)
			if w.PeerConnection != nil {
				_ = w.PeerConnection.Close()
			}
			if tConn != nil {
				_ = tConn.Close()
			}
		}
	}()

	settingEngine := webrtc.SettingEngine{LoggerFactory: pionLoggerFactory}
	settingEngine.SetAnsweringDTLSRole(webrtc.DTLSRoleServer)
	settingEngine.SetICECredentials(candidate.Ufrag, candidate.Ufrag)
	settingEngine.SetLite(true)
	settingEngine.SetICEUDPMux(l.mux)
	settingEngine.SetIncludeLoopbackCandidate(true)
	settingEngine.DisableCertificateFingerprintVerification(true)
	settingEngine.SetICETimeouts(
		l.transport.peerConnectionTimeouts.Disconnect,
		l.transport.peerConnectionTimeouts.Failed,
		l.transport.peerConnectionTimeouts.Keepalive,
	)
	// 这个值比路径 MTU 要高,是由于 sctp 分块逻辑中的一个 bug。
	// 在 https://github.com/pion/sctp/pull/301 合并发布后可以移除这个设置。
	settingEngine.SetReceiveMTU(udpmux.ReceiveBufSize)
	settingEngine.DetachDataChannels()
	settingEngine.SetSCTPMaxReceiveBufferSize(sctpReceiveBufferSize)
	if err := scope.ReserveMemory(sctpReceiveBufferSize, network.ReservationPriorityMedium); err != nil {
		log.Debugf("预留内存时出错: %s", err)
		return nil, err
	}

	w, err = newWebRTCConnection(settingEngine, l.config)
	if err != nil {
		log.Debugf("实例化对等连接失败: %s", err)
		return nil, err
	}

	errC := addOnConnectionStateChangeCallback(w.PeerConnection)
	// 通过设置 ice-ufrag 从传入的 STUN 消息推断客户端 SDP
	if err := w.PeerConnection.SetRemoteDescription(webrtc.SessionDescription{
		SDP:  createClientSDP(candidate.Addr, candidate.Ufrag),
		Type: webrtc.SDPTypeOffer,
	}); err != nil {
		log.Debugf("设置远程描述时出错: %s", err)
		return nil, err
	}
	answer, err := w.PeerConnection.CreateAnswer(nil)
	if err != nil {
		log.Debugf("创建答案时出错: %s", err)
		return nil, err
	}
	if err := w.PeerConnection.SetLocalDescription(answer); err != nil {
		log.Debugf("设置本地描述时出错: %s", err)
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-errC:
		if err != nil {
			log.Debugf("ufrag 为 %s 的对等连接失败: %s", candidate.Ufrag, err)
			return nil, err
		}
	}

	// 运行噪声握手
	rwc, err := detachHandshakeDataChannel(ctx, w.HandshakeDataChannel)
	if err != nil {
		log.Debugf("分离握手数据通道时出错: %s", err)
		return nil, err
	}
	handshakeChannel := newStream(w.HandshakeDataChannel, rwc, func() {})
	// 我们还不知道 A 的对等 ID,所以接受任何入站连接
	remotePubKey, err := l.transport.noiseHandshake(ctx, w.PeerConnection, handshakeChannel, "", crypto.SHA256, true)
	if err != nil {
		log.Debugf("噪声握手时出错: %s", err)
		return nil, err
	}
	remotePeer, err := peer.IDFromPublicKey(remotePubKey)
	if err != nil {
		log.Debugf("从公钥创建对等 ID 时出错: %s", err)
		return nil, err
	}
	// 最早知道远程对等 ID 的时间点
	if err := scope.SetPeer(remotePeer); err != nil {
		log.Debugf("设置对等 ID 时出错: %s", err)
		return nil, err
	}

	localMultiaddrWithoutCerthash, _ := ma.SplitFunc(l.localMultiaddr, func(c ma.Component) bool { return c.Protocol().Code == ma.P_CERTHASH })
	conn, err := newConnection(
		network.DirInbound,
		w.PeerConnection,
		l.transport,
		scope,
		l.transport.localPeerId,
		localMultiaddrWithoutCerthash,
		remotePeer,
		remotePubKey,
		remoteMultiaddr,
		w.IncomingDataChannels,
		w.PeerConnectionClosedCh,
	)
	if err != nil {
		log.Debugf("创建连接时出错: %s", err)
		return nil, err
	}

	return conn, err
}

// Accept 接受一个入站连接
// 返回值:
//   - tpt.CapableConn 接受的连接
//   - error 错误信息
func (l *listener) Accept() (tpt.CapableConn, error) {
	select {
	case <-l.ctx.Done():
		log.Debugf("监听器已关闭")
		return nil, tpt.ErrListenerClosed
	case conn := <-l.acceptQueue:
		return conn, nil
	}
}

// Close 关闭监听器
// 返回值:
//   - error 错误信息
func (l *listener) Close() error {
	select {
	case <-l.ctx.Done():
	default:
	}
	l.cancel()
	l.mux.Close()
	l.wg.Wait()
loop:
	for {
		select {
		case conn := <-l.acceptQueue:
			conn.Close()
		default:
			break loop
		}
	}
	return nil
}

// Addr 返回监听器的网络地址
// 返回值:
//   - net.Addr 网络地址
func (l *listener) Addr() net.Addr {
	return l.localAddr
}

// Multiaddr 返回监听器的多地址
// 返回值:
//   - ma.Multiaddr 多地址
func (l *listener) Multiaddr() ma.Multiaddr {
	return l.localMultiaddr
}

// addOnConnectionStateChangeCallback 为对等连接添加连接状态变化回调
// 参数:
//   - pc: *webrtc.PeerConnection 对等连接
//
// 返回值:
//   - <-chan error 错误通道
//   - 当状态变为 Connected 时关闭
//   - 当状态变为 Failed、Closed 或 Disconnected 时接收错误
func addOnConnectionStateChangeCallback(pc *webrtc.PeerConnection) <-chan error {
	errC := make(chan error, 1)
	var once sync.Once
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		switch pc.ConnectionState() {
		case webrtc.PeerConnectionStateConnected:
			once.Do(func() { close(errC) })
		// PeerConnectionStateFailed 发生在连接协商失败时
		// PeerConnectionStateDisconnected 发生在连接后立即断开时
		// PeerConnectionStateClosed 发生在本地关闭对等连接时,而不是远程关闭时。
		// 在这种情况下我们不需要报错,但这是一个空操作,所以没有害处。
		case webrtc.PeerConnectionStateFailed, webrtc.PeerConnectionStateClosed, webrtc.PeerConnectionStateDisconnected:
			once.Do(func() {
				errC <- errors.New("对等连接失败")
				close(errC)
			})
		}
	})
	return errC
}
