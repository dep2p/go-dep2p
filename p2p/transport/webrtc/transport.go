package dep2pwebrtc

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"time"

	mrand "golang.org/x/exp/rand"
	"google.golang.org/protobuf/proto"

	"github.com/dep2p/go-dep2p/core/connmgr"
	ic "github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	"github.com/dep2p/go-dep2p/core/pnet"
	"github.com/dep2p/go-dep2p/core/sec"
	tpt "github.com/dep2p/go-dep2p/core/transport"
	"github.com/dep2p/go-dep2p/p2p/security/noise"
	dep2pquic "github.com/dep2p/go-dep2p/p2p/transport/quic"
	"github.com/dep2p/go-dep2p/p2p/transport/webrtc/pb"
	"github.com/dep2p/go-dep2p/p2plib/msgio"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	manet "github.com/dep2p/go-dep2p/multiformats/multiaddr/net"
	"github.com/dep2p/go-dep2p/multiformats/multihash"

	"github.com/pion/datachannel"
	"github.com/pion/webrtc/v4"
)

// webrtcComponent 存储 WebRTC 多地址组件
var webrtcComponent *ma.Component

// init 初始化 WebRTC 多地址组件
func init() {
	var err error
	webrtcComponent, err = ma.NewComponent(ma.ProtocolWithCode(ma.P_WEBRTC_DIRECT).Name, "")
	if err != nil {
		log.Fatal(err)
	}
}

const (
	// handshakeChannelNegotiated 用于指定握手数据通道不需要通过 DCEP 进行协商
	// 使用常量是因为 DataChannelInit 结构体接受引用而不是值
	handshakeChannelNegotiated = true

	// handshakeChannelID 是握手数据通道的约定 ID
	// 使用常量是因为 DataChannelInit 结构体接受引用而不是值
	// 这里指定类型是因为该值只会被复制和通过引用传递
	handshakeChannelID = uint16(0)
)

// PeerConnection 的超时值
// https://github.com/pion/webrtc/blob/v3.1.50/settingengine.go#L102-L109
const (
	// DefaultDisconnectedTimeout 默认断开连接超时时间
	DefaultDisconnectedTimeout = 20 * time.Second
	// DefaultFailedTimeout 默认失败超时时间
	DefaultFailedTimeout = 30 * time.Second
	// DefaultKeepaliveTimeout 默认保活超时时间
	DefaultKeepaliveTimeout = 15 * time.Second

	// sctpReceiveBufferSize SCTP 接收缓冲区大小
	sctpReceiveBufferSize = 100_000
)

// WebRTCTransport WebRTC 传输层实现
type WebRTCTransport struct {
	webrtcConfig webrtc.Configuration    // WebRTC 配置
	rcmgr        network.ResourceManager // 资源管理器
	gater        connmgr.ConnectionGater // 连接过滤器
	privKey      ic.PrivKey              // 私钥
	noiseTpt     *noise.Transport        // Noise 传输层
	localPeerId  peer.ID                 // 本地节点 ID

	// listenUDP UDP 监听函数
	listenUDP func(network string, laddr *net.UDPAddr) (net.PacketConn, error)

	// 超时设置
	peerConnectionTimeouts iceTimeouts

	// 正在建立的连接数
	maxInFlightConnections uint32
}

// 确保 WebRTCTransport 实现了 tpt.Transport 接口
var _ tpt.Transport = &WebRTCTransport{}

// Option WebRTCTransport 配置选项函数类型
type Option func(*WebRTCTransport) error

// iceTimeouts ICE 超时配置
type iceTimeouts struct {
	Disconnect time.Duration // 断开连接超时
	Failed     time.Duration // 失败超时
	Keepalive  time.Duration // 保活超时
}

// ListenUDPFn UDP 监听函数类型
type ListenUDPFn func(network string, laddr *net.UDPAddr) (net.PacketConn, error)

// New 创建新的 WebRTC 传输层实例
// 参数:
//   - privKey: 私钥
//   - psk: 预共享密钥
//   - gater: 连接过滤器
//   - rcmgr: 资源管理器
//   - listenUDP: UDP 监听函数
//   - opts: 配置选项
//
// 返回值:
//   - *WebRTCTransport: WebRTC 传输层实例
//   - error: 错误信息
func New(privKey ic.PrivKey, psk pnet.PSK, gater connmgr.ConnectionGater, rcmgr network.ResourceManager, listenUDP ListenUDPFn, opts ...Option) (*WebRTCTransport, error) {
	if psk != nil {
		log.Debugf("WebRTC 暂不支持私有网络")
		return nil, fmt.Errorf("WebRTC 暂不支持私有网络")
	}
	if rcmgr == nil {
		rcmgr = &network.NullResourceManager{}
	}
	localPeerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		log.Debugf("获取本地节点 ID 失败: %s", err)
		return nil, err
	}
	// 使用椭圆曲线 P-256 因为它被浏览器广泛支持
	//
	// 实现说明: 通过浏览器测试发现，Chromium 只支持 ECDSA P-256 或 RSA 密钥签名的 WebRTC TLS 证书
	// 我们尝试使用 P-228 和 P-384 导致 DTLS 握手失败并报错 Illegal Parameter
	//
	// 请参考 WebCrypto API 建议的算法列表
	// RTCPeerConnection 证书生成算法必须遵循 WebCrypto API
	// 根据观察，RSA 和 ECDSA P-256 在几乎所有浏览器上都支持
	// Ed25519 不在列表中
	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		log.Debugf("生成证书密钥失败: %s", err)
		return nil, err
	}
	cert, err := webrtc.GenerateCertificate(pk)
	if err != nil {
		log.Debugf("生成证书失败: %s", err)
		return nil, err
	}
	config := webrtc.Configuration{
		Certificates: []webrtc.Certificate{*cert},
	}
	noiseTpt, err := noise.New(noise.ID, privKey, nil)
	if err != nil {
		log.Debugf("创建 Noise 传输层失败: %s", err)
		return nil, err
	}
	transport := &WebRTCTransport{
		rcmgr:        rcmgr,
		gater:        gater,
		webrtcConfig: config,
		privKey:      privKey,
		noiseTpt:     noiseTpt,
		localPeerId:  localPeerID,

		listenUDP: listenUDP,
		peerConnectionTimeouts: iceTimeouts{
			Disconnect: DefaultDisconnectedTimeout,
			Failed:     DefaultFailedTimeout,
			Keepalive:  DefaultKeepaliveTimeout,
		},

		maxInFlightConnections: DefaultMaxInFlightConnections,
	}
	for _, opt := range opts {
		if err := opt(transport); err != nil {
			log.Debugf("应用配置选项失败: %s", err)
			return nil, err
		}
	}
	return transport, nil
}

// ListenOrder 返回监听优先级
// 返回值:
//   - int: 监听优先级，比 QUIC 晚 1 以便可能复用相同端口
func (t *WebRTCTransport) ListenOrder() int {
	return dep2pquic.ListenOrder + 1 // 我们希望在 QUIC 监听之后监听，这样可以复用相同的端口
}

// Protocols 返回支持的协议列表
// 返回值:
//   - []int: 支持的协议代码列表
func (t *WebRTCTransport) Protocols() []int {
	return []int{ma.P_WEBRTC_DIRECT}
}

// Proxy 返回是否支持代理
// 返回值:
//   - bool: 是否支持代理
func (t *WebRTCTransport) Proxy() bool {
	return false
}

// CanDial 检查是否可以拨号到指定地址
// 参数:
//   - addr: 多地址
//
// 返回值:
//   - bool: 是否可以拨号
func (t *WebRTCTransport) CanDial(addr ma.Multiaddr) bool {
	isValid, n := IsWebRTCDirectMultiaddr(addr)
	return isValid && n > 0
}

// Listen 监听指定地址
// 参数:
//   - addr: 多地址
//
// 返回值:
//   - tpt.Listener: 监听器
//   - error: 错误信息
//
// 注意:
//   - addr 的 IP、端口组合必须是独占的，因为 WebRTC 监听器不能与其他基于 UDP 的传输层(如 QUIC 和 WebTransport)复用相同端口
//   - 详见 https://github.com/dep2p/issues/2446
func (t *WebRTCTransport) Listen(addr ma.Multiaddr) (tpt.Listener, error) {
	addr, wrtcComponent := ma.SplitLast(addr)
	isWebrtc := wrtcComponent.Equal(webrtcComponent)
	if !isWebrtc {
		log.Debugf("必须监听 WebRTC 多地址")
		return nil, fmt.Errorf("必须监听 WebRTC 多地址")
	}
	nw, host, err := manet.DialArgs(addr)
	if err != nil {
		log.Debugf("监听器无法获取拨号参数: %s", err)
		return nil, err
	}
	udpAddr, err := net.ResolveUDPAddr(nw, host)
	if err != nil {
		log.Debugf("监听器无法解析 UDP 地址: %s", err)
		return nil, err
	}

	socket, err := t.listenUDP(nw, udpAddr)
	if err != nil {
		log.Debugf("监听 UDP 失败: %s", err)
		return nil, err
	}

	listener, err := t.listenSocket(socket)
	if err != nil {
		log.Debugf("监听器无法创建: %s", err)
		socket.Close()
		return nil, err
	}
	return listener, nil
}

// listenSocket 在指定 socket 上创建监听器
// 参数:
//   - socket: UDP 套接字
//
// 返回值:
//   - tpt.Listener: 监听器
//   - error: 错误信息
func (t *WebRTCTransport) listenSocket(socket net.PacketConn) (tpt.Listener, error) {
	listenerMultiaddr, err := manet.FromNetAddr(socket.LocalAddr())
	if err != nil {
		log.Debugf("监听器无法从网络地址创建多地址: %s", err)
		return nil, err
	}

	listenerFingerprint, err := t.getCertificateFingerprint()
	if err != nil {
		log.Debugf("监听器无法获取证书指纹: %s", err)
		return nil, err
	}

	encodedLocalFingerprint, err := encodeDTLSFingerprint(listenerFingerprint)
	if err != nil {
		log.Debugf("监听器无法编码证书指纹: %s", err)
		return nil, err
	}

	certComp, err := ma.NewComponent(ma.ProtocolWithCode(ma.P_CERTHASH).Name, encodedLocalFingerprint)
	if err != nil {
		log.Debugf("监听器无法创建证书组件: %s", err)
		return nil, err
	}
	listenerMultiaddr = listenerMultiaddr.Encapsulate(webrtcComponent).Encapsulate(certComp)

	return newListener(
		t,
		listenerMultiaddr,
		socket,
		t.webrtcConfig,
	)
}

// Dial 拨号到远程节点
// 参数:
//   - ctx: 上下文
//   - remoteMultiaddr: 远程多地址
//   - p: 远程节点 ID
//
// 返回值:
//   - tpt.CapableConn: 连接
//   - error: 错误信息
func (t *WebRTCTransport) Dial(ctx context.Context, remoteMultiaddr ma.Multiaddr, p peer.ID) (tpt.CapableConn, error) {
	scope, err := t.rcmgr.OpenConnection(network.DirOutbound, false, remoteMultiaddr)
	if err != nil {
		log.Debugf("打开连接失败: %s", err)
		return nil, err
	}
	if err := scope.SetPeer(p); err != nil {
		scope.Done()
		log.Debugf("设置对等节点 ID 失败: %s", err)
		return nil, err
	}
	conn, err := t.dial(ctx, scope, remoteMultiaddr, p)
	if err != nil {
		scope.Done()
		log.Debugf("拨号失败: %s", err)
		return nil, err
	}
	return conn, nil
}

// dial 执行实际的拨号操作
// 参数:
//   - ctx: 上下文
//   - scope: 连接管理作用域
//   - remoteMultiaddr: 远程多地址
//   - p: 远程节点 ID
//
// 返回值:
//   - tpt.CapableConn: 连接
//   - error: 错误信息
func (t *WebRTCTransport) dial(ctx context.Context, scope network.ConnManagementScope, remoteMultiaddr ma.Multiaddr, p peer.ID) (tConn tpt.CapableConn, err error) {
	var w webRTCConnection
	defer func() {
		if err != nil {
			if w.PeerConnection != nil {
				_ = w.PeerConnection.Close()
			}
			if tConn != nil {
				_ = tConn.Close()
				tConn = nil
			}
		}
	}()

	remoteMultihash, err := decodeRemoteFingerprint(remoteMultiaddr)
	if err != nil {
		log.Debugf("解码指纹失败: %s", err)
		return nil, err
	}
	remoteHashFunction, ok := getSupportedSDPHash(remoteMultihash.Code)
	if !ok {
		log.Debugf("不支持的哈希函数")
		return nil, errors.New("不支持的哈希函数")
	}

	rnw, rhost, err := manet.DialArgs(remoteMultiaddr)
	if err != nil {
		log.Debugf("生成拨号参数失败: %s", err)
		return nil, err
	}

	raddr, err := net.ResolveUDPAddr(rnw, rhost)
	if err != nil {
		log.Debugf("解析 UDP 地址失败: %s", err)
		return nil, err
	}

	// 不是编码本地指纹，而是生成随机 UUID 作为连接 ufrag
	// 这里唯一的要求是 ufrag 和密码必须相等，这样服务器就可以使用 STUN 消息确定密码
	ufrag := genUfrag()

	settingEngine := webrtc.SettingEngine{
		LoggerFactory: pionLoggerFactory,
	}
	settingEngine.SetICECredentials(ufrag, ufrag)
	settingEngine.DetachDataChannels()
	// 使用第一个最佳地址候选
	settingEngine.SetPrflxAcceptanceMinWait(0)
	settingEngine.SetICETimeouts(
		t.peerConnectionTimeouts.Disconnect,
		t.peerConnectionTimeouts.Failed,
		t.peerConnectionTimeouts.Keepalive,
	)
	// 默认情况下，WebRTC 不会在回环地址上收集候选者
	// 这在 ICE 规范中是不允许的。但是，实现并不严格遵循这一点，例如 Chrome 收集 TCP 回环候选者
	// 如果你在一个只有回环接口 UP 的系统上运行 pion，它将无法连接到任何东西
	settingEngine.SetIncludeLoopbackCandidate(true)
	settingEngine.SetSCTPMaxReceiveBufferSize(sctpReceiveBufferSize)
	if err := scope.ReserveMemory(sctpReceiveBufferSize, network.ReservationPriorityMedium); err != nil {
		log.Debugf("预留内存失败: %s", err)
		return nil, err
	}

	w, err = newWebRTCConnection(settingEngine, t.webrtcConfig)
	if err != nil {
		log.Debugf("实例化对等连接失败: %s", err)
		return nil, err
	}

	errC := addOnConnectionStateChangeCallback(w.PeerConnection)

	// 执行提议-应答交换
	offer, err := w.PeerConnection.CreateOffer(nil)
	if err != nil {
		log.Debugf("创建提议失败: %s", err)
		return nil, err
	}

	err = w.PeerConnection.SetLocalDescription(offer)
	if err != nil {
		log.Debugf("设置本地描述失败: %s", err)
		return nil, err
	}

	answerSDPString, err := createServerSDP(raddr, ufrag, *remoteMultihash)
	if err != nil {
		log.Debugf("渲染服务器 SDP 失败: %s", err)
		return nil, err
	}

	answer := webrtc.SessionDescription{SDP: answerSDPString, Type: webrtc.SDPTypeAnswer}
	err = w.PeerConnection.SetRemoteDescription(answer)
	if err != nil {
		log.Debugf("设置远程描述失败: %s", err)
		return nil, err
	}

	// 等待对等连接打开
	select {
	case err := <-errC:
		if err != nil {
			log.Debugf("对等连接打开失败: %s", err)
			return nil, err
		}
	case <-ctx.Done():
		log.Debugf("对等连接打开超时")
		return nil, errors.New("对等连接打开超时")
	}

	// 已连接，运行 Noise 握手
	detached, err := detachHandshakeDataChannel(ctx, w.HandshakeDataChannel)
	if err != nil {
		log.Debugf("分离握手数据通道失败: %s", err)
		return nil, err
	}
	channel := newStream(w.HandshakeDataChannel, detached, func() {})

	remotePubKey, err := t.noiseHandshake(ctx, w.PeerConnection, channel, p, remoteHashFunction, false)
	if err != nil {
		log.Debugf("Noise 握手失败: %s", err)
		return nil, err
	}

	// 设置连接的本地和远程地址
	cp, err := w.HandshakeDataChannel.Transport().Transport().ICETransport().GetSelectedCandidatePair()
	if cp == nil {
		log.Debugf("ICE 连接没有选定的候选对: nil 结果")
		return nil, errors.New("ICE 连接没有选定的候选对: nil 结果")
	}
	if err != nil {
		log.Debugf("ICE 连接没有选定的候选对: 错误: %s", err)
		return nil, err
	}
	// 选定候选对的本地地址应该是连接的本地地址
	localAddr, err := manet.FromNetAddr(&net.UDPAddr{IP: net.ParseIP(cp.Local.Address), Port: int(cp.Local.Port)})
	if err != nil {
		log.Debugf("从网络地址创建多地址失败: %s", err)
		return nil, err
	}
	remoteMultiaddrWithoutCerthash, _ := ma.SplitFunc(remoteMultiaddr, func(c ma.Component) bool { return c.Protocol().Code == ma.P_CERTHASH })

	conn, err := newConnection(
		network.DirOutbound,
		w.PeerConnection,
		t,
		scope,
		t.localPeerId,
		localAddr,
		p,
		remotePubKey,
		remoteMultiaddrWithoutCerthash,
		w.IncomingDataChannels,
		w.PeerConnectionClosedCh,
	)
	if err != nil {
		log.Debugf("创建连接失败: %s", err)
		return nil, err
	}

	if t.gater != nil && !t.gater.InterceptSecured(network.DirOutbound, p, conn) {
		log.Debugf("安全连接被拦截")
		return nil, fmt.Errorf("安全连接被拦截")
	}
	return conn, nil
}

// genUfrag 生成 ICE 用户片段
// 返回值:
//   - string: 生成的用户片段
func genUfrag() string {
	const (
		uFragAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"
		uFragPrefix   = "dep2p+webrtc+v1/"
		uFragIdLength = 32
		uFragLength   = len(uFragPrefix) + uFragIdLength
	)

	seed := [8]byte{}
	rand.Read(seed[:])
	r := mrand.New(mrand.NewSource(binary.BigEndian.Uint64(seed[:])))
	b := make([]byte, uFragLength)
	for i := 0; i < len(uFragPrefix); i++ {
		b[i] = uFragPrefix[i]
	}
	for i := len(uFragPrefix); i < uFragLength; i++ {
		b[i] = uFragAlphabet[r.Intn(len(uFragAlphabet))]
	}
	return string(b)
}

// getCertificateFingerprint 获取证书指纹
// 返回值:
//   - webrtc.DTLSFingerprint: 证书指纹
//   - error: 错误信息
func (t *WebRTCTransport) getCertificateFingerprint() (webrtc.DTLSFingerprint, error) {
	fps, err := t.webrtcConfig.Certificates[0].GetFingerprints()
	if err != nil {
		return webrtc.DTLSFingerprint{}, err
	}
	return fps[0], nil
}

// generateNoisePrologue 生成 Noise 协议前言
// 参数:
//   - pc: WebRTC 对等连接
//   - hash: 哈希函数
//   - inbound: 是否为入站连接
//
// 返回值:
//   - []byte: 生成的前言
//   - error: 错误信息
func (t *WebRTCTransport) generateNoisePrologue(pc *webrtc.PeerConnection, hash crypto.Hash, inbound bool) ([]byte, error) {
	raw := pc.SCTP().Transport().GetRemoteCertificate()
	cert, err := x509.ParseCertificate(raw)
	if err != nil {
		log.Debugf("解析证书失败: %s", err)
		return nil, err
	}

	// 注意: 如果需要，我们可以分叉证书代码以避免由于不必要的字符串插入(hex)导致的额外分配
	localFp, err := t.getCertificateFingerprint()
	if err != nil {
		log.Debugf("获取证书指纹失败: %s", err)
		return nil, err
	}

	remoteFpBytes, err := parseFingerprint(cert, hash)
	if err != nil {
		log.Debugf("解析指纹失败: %s", err)
		return nil, err
	}

	localFpBytes, err := decodeInterspersedHexFromASCIIString(localFp.Value)
	if err != nil {
		log.Debugf("解码指纹失败: %s", err)
		return nil, err
	}

	localEncoded, err := multihash.Encode(localFpBytes, multihash.SHA2_256)
	if err != nil {
		log.Debugf("无法为本地指纹编码多重哈希")
		return nil, err
	}
	remoteEncoded, err := multihash.Encode(remoteFpBytes, multihash.SHA2_256)
	if err != nil {
		log.Debugf("无法为远程指纹编码多重哈希")
		return nil, err
	}

	result := []byte("dep2p-webrtc-noise:")
	if inbound {
		result = append(result, remoteEncoded...)
		result = append(result, localEncoded...)
	} else {
		result = append(result, localEncoded...)
		result = append(result, remoteEncoded...)
	}
	return result, nil
}

// noiseHandshake 执行 Noise 协议握手
// 参数:
//   - ctx: 上下文
//   - pc: WebRTC 对等连接
//   - s: 流
//   - peer: 对等节点 ID
//   - hash: 哈希函数
//   - inbound: 是否为入站连接
//
// 返回值:
//   - ic.PubKey: 远程公钥
//   - error: 错误信息
func (t *WebRTCTransport) noiseHandshake(ctx context.Context, pc *webrtc.PeerConnection, s *stream, peer peer.ID, hash crypto.Hash, inbound bool) (ic.PubKey, error) {
	prologue, err := t.generateNoisePrologue(pc, hash, inbound)
	if err != nil {
		log.Debugf("生成前言失败: %s", err)
		return nil, err
	}
	opts := make([]noise.SessionOption, 0, 2)
	opts = append(opts, noise.Prologue(prologue))
	if peer == "" {
		opts = append(opts, noise.DisablePeerIDCheck())
	}
	sessionTransport, err := t.noiseTpt.WithSessionOptions(opts...)
	if err != nil {
		log.Debugf("实例化 Noise 传输层失败: %s", err)
		return nil, err
	}
	var secureConn sec.SecureConn
	if inbound {
		secureConn, err = sessionTransport.SecureOutbound(ctx, netConnWrapper{s}, peer)
		if err != nil {
			log.Debugf("保护入站连接失败: %s", err)
			return nil, err
		}
	} else {
		secureConn, err = sessionTransport.SecureInbound(ctx, netConnWrapper{s}, peer)
		if err != nil {
			log.Debugf("保护出站连接失败: %s", err)
			return nil, err
		}
	}
	return secureConn.RemotePublicKey(), nil
}

// AddCertHashes 向多地址添加证书哈希
// 参数:
//   - addr: 多地址
//
// 返回值:
//   - ma.Multiaddr: 添加了证书哈希的多地址
//   - bool: 是否成功
func (t *WebRTCTransport) AddCertHashes(addr ma.Multiaddr) (ma.Multiaddr, bool) {
	listenerFingerprint, err := t.getCertificateFingerprint()
	if err != nil {
		log.Debugf("获取证书指纹失败: %s", err)
		return nil, false
	}

	encodedLocalFingerprint, err := encodeDTLSFingerprint(listenerFingerprint)
	if err != nil {
		log.Debugf("编码证书指纹失败: %s", err)
		return nil, false
	}

	certComp, err := ma.NewComponent(ma.ProtocolWithCode(ma.P_CERTHASH).Name, encodedLocalFingerprint)
	if err != nil {
		log.Debugf("创建证书组件失败: %s", err)
		return nil, false
	}
	return addr.Encapsulate(certComp), true
}

// netConnWrapper 网络连接包装器
type netConnWrapper struct {
	*stream
}

func (netConnWrapper) LocalAddr() net.Addr  { return nil }
func (netConnWrapper) RemoteAddr() net.Addr { return nil }
func (w netConnWrapper) Close() error {
	// 在运行安全握手时调用 Close 是一个错误，在这种情况下我们应该重置流而不是优雅关闭
	w.stream.Reset()
	return nil
}

// detachHandshakeDataChannel 分离握手数据通道
// 参数:
//   - ctx: 上下文
//   - dc: WebRTC 数据通道
//
// 返回值:
//   - datachannel.ReadWriteCloser: 读写关闭器
//   - error: 错误信息
func detachHandshakeDataChannel(ctx context.Context, dc *webrtc.DataChannel) (datachannel.ReadWriteCloser, error) {
	done := make(chan struct{})
	var rwc datachannel.ReadWriteCloser
	var err error
	dc.OnOpen(func() {
		defer close(done)
		rwc, err = dc.Detach()
	})
	// 这是安全的，因为对于分离的数据通道，如果 SCTP 传输也已连接，对等连接会立即运行 onOpen 回调
	select {
	case <-done:
		return rwc, err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// webRTCConnection 持有 webrtc.PeerConnection 和握手通道以及对等方创建的传入数据通道队列
//
// 创建 webrtc.PeerConnection 时，重要的是在与对等方连接之前设置 OnDataChannel 处理程序
// 如果在与对等方连接后设置处理程序，对等方创建的数据通道可能在短时间内不会显示给我们，并导致内存泄漏
type webRTCConnection struct {
	PeerConnection         *webrtc.PeerConnection // WebRTC 对等连接
	HandshakeDataChannel   *webrtc.DataChannel    // 握手数据通道
	IncomingDataChannels   chan dataChannel       // 传入数据通道队列
	PeerConnectionClosedCh chan struct{}          // 对等连接关闭通道
}

// newWebRTCConnection 创建新的 WebRTC 连接
// 参数:
//   - settings: webrtc.SettingEngine WebRTC 设置引擎
//   - config: webrtc.Configuration WebRTC 配置
//
// 返回值:
//   - webRTCConnection: WebRTC 连接对象
//   - error: 错误信息
func newWebRTCConnection(settings webrtc.SettingEngine, config webrtc.Configuration) (webRTCConnection, error) {
	// 使用设置引擎创建 WebRTC API
	api := webrtc.NewAPI(webrtc.WithSettingEngine(settings))
	// 创建对等连接
	pc, err := api.NewPeerConnection(config)
	if err != nil {
		log.Debugf("创建对等连接失败: %s", err)
		return webRTCConnection{}, err
	}

	// 创建握手数据通道
	negotiated, id := handshakeChannelNegotiated, handshakeChannelID
	handshakeDataChannel, err := pc.CreateDataChannel("", &webrtc.DataChannelInit{
		Negotiated: &negotiated,
		ID:         &id,
	})
	if err != nil {
		log.Debugf("创建握手通道失败: %s", err)
		pc.Close()
		return webRTCConnection{}, err
	}

	// 创建传入数据通道队列
	incomingDataChannels := make(chan dataChannel, maxAcceptQueueLen)
	// 设置数据通道处理函数
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		dc.OnOpen(func() {
			// 分离数据通道
			rwc, err := dc.Detach()
			if err != nil {
				log.Debugf("无法分离数据通道: id: %d", *dc.ID())
				return
			}
			// 将数据通道加入队列或拒绝连接
			select {
			case incomingDataChannels <- dataChannel{rwc, dc}:
			default:
				log.Debugf("连接繁忙，拒绝流")
				b, _ := proto.Marshal(&pb.Message{Flag: pb.Message_RESET.Enum()})
				w := msgio.NewWriter(rwc)
				w.WriteMsg(b)
				rwc.Close()
			}
		})
	})

	// 创建连接关闭通道
	connectionClosedCh := make(chan struct{}, 1)
	// 设置 SCTP 关闭处理函数
	pc.SCTP().OnClose(func(err error) {
		// 我们只需要一个消息。关闭连接是一个问题，因为 pion 可能多次调用回调。
		select {
		case connectionClosedCh <- struct{}{}:
		default:
		}
	})
	// 返回 WebRTC 连接对象
	return webRTCConnection{
		PeerConnection:         pc,
		HandshakeDataChannel:   handshakeDataChannel,
		IncomingDataChannels:   incomingDataChannels,
		PeerConnectionClosedCh: connectionClosedCh,
	}, nil
}

// IsWebRTCDirectMultiaddr 检查给定的多地址是否为 WebRTC 直连多地址，并返回其中的证书哈希数量
// 参数:
//   - addr: ma.Multiaddr 多地址对象
//
// 返回值:
//   - bool: 是否为 WebRTC 直连多地址
//   - int: 证书哈希数量
func IsWebRTCDirectMultiaddr(addr ma.Multiaddr) (bool, int) {
	var foundUDP, foundWebRTC bool
	certHashCount := 0
	// 遍历多地址的每个组件
	ma.ForEach(addr, func(c ma.Component) bool {
		if !foundUDP {
			if c.Protocol().Code == ma.P_UDP {
				foundUDP = true
			}
			return true
		}
		if !foundWebRTC && foundUDP {
			// UDP 之后的协议必须是 webrtc-direct
			if c.Protocol().Code != ma.P_WEBRTC_DIRECT {
				return false
			}
			foundWebRTC = true
			return true
		}
		if foundWebRTC {
			if c.Protocol().Code == ma.P_CERTHASH {
				certHashCount++
			} else {
				return false
			}
		}
		return true
	})
	return foundUDP && foundWebRTC, certHashCount
}
