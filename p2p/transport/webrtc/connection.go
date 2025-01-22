//go:build !js

package dep2pwebrtc

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"sync"
	"sync/atomic"

	ic "github.com/dep2p/go-dep2p/core/crypto"
	"github.com/dep2p/go-dep2p/core/network"
	"github.com/dep2p/go-dep2p/core/peer"
	tpt "github.com/dep2p/go-dep2p/core/transport"

	ma "github.com/dep2p/go-dep2p/multiformats/multiaddr"
	"github.com/pion/datachannel"
	"github.com/pion/sctp"
	"github.com/pion/webrtc/v4"
)

// 确保 connection 实现了 tpt.CapableConn 接口
var _ tpt.CapableConn = &connection{}

// 最大接受队列长度
const maxAcceptQueueLen = 256

// 连接超时错误
type errConnectionTimeout struct{}

// 确保 errConnectionTimeout 实现了 net.Error 接口
var _ net.Error = &errConnectionTimeout{}

// Error 返回错误信息
// 返回值:
//   - string 错误信息
func (errConnectionTimeout) Error() string { return "连接超时" }

// Timeout 返回是否为超时错误
// 返回值:
//   - bool 是否为超时错误
func (errConnectionTimeout) Timeout() bool { return true }

// Temporary 返回是否为临时错误
// 返回值:
//   - bool 是否为临时错误
func (errConnectionTimeout) Temporary() bool { return false }

// 连接关闭错误
var errConnClosed = errors.New("连接已关闭")

// dataChannel 表示一个数据通道
// 字段:
//   - stream: datachannel.ReadWriteCloser 数据流
//   - channel: *webrtc.DataChannel WebRTC 数据通道
type dataChannel struct {
	stream  datachannel.ReadWriteCloser
	channel *webrtc.DataChannel
}

// connection 表示一个 WebRTC 连接
// 字段:
//   - pc: *webrtc.PeerConnection 对等连接
//   - transport: *WebRTCTransport 传输层
//   - scope: network.ConnManagementScope 连接管理范围
//   - closeOnce: sync.Once 确保只关闭一次
//   - closeErr: error 关闭错误
//   - localPeer: peer.ID 本地节点 ID
//   - localMultiaddr: ma.Multiaddr 本地多地址
//   - remotePeer: peer.ID 远程节点 ID
//   - remoteKey: ic.PubKey 远程公钥
//   - remoteMultiaddr: ma.Multiaddr 远程多地址
//   - m: sync.Mutex 互斥锁
//   - streams: map[uint16]*stream 流映射
//   - nextStreamID: atomic.Int32 下一个流 ID
//   - acceptQueue: chan dataChannel 接受队列
//   - ctx: context.Context 上下文
//   - cancel: context.CancelFunc 取消函数
type connection struct {
	pc        *webrtc.PeerConnection
	transport *WebRTCTransport
	scope     network.ConnManagementScope

	closeOnce sync.Once
	closeErr  error

	localPeer      peer.ID
	localMultiaddr ma.Multiaddr

	remotePeer      peer.ID
	remoteKey       ic.PubKey
	remoteMultiaddr ma.Multiaddr

	m            sync.Mutex
	streams      map[uint16]*stream
	nextStreamID atomic.Int32

	acceptQueue chan dataChannel

	ctx    context.Context
	cancel context.CancelFunc
}

// newConnection 创建一个新的连接
// 参数:
//   - direction: network.Direction 连接方向
//   - pc: *webrtc.PeerConnection 对等连接
//   - transport: *WebRTCTransport 传输层
//   - scope: network.ConnManagementScope 连接管理范围
//   - localPeer: peer.ID 本地节点 ID
//   - localMultiaddr: ma.Multiaddr 本地多地址
//   - remotePeer: peer.ID 远程节点 ID
//   - remoteKey: ic.PubKey 远程公钥
//   - remoteMultiaddr: ma.Multiaddr 远程多地址
//   - incomingDataChannels: chan dataChannel 传入数据通道队列
//   - peerConnectionClosedCh: chan struct{} 对等连接关闭通道
//
// 返回值:
//   - *connection 新创建的连接
//   - error 错误信息
func newConnection(
	direction network.Direction,
	pc *webrtc.PeerConnection,
	transport *WebRTCTransport,
	scope network.ConnManagementScope,

	localPeer peer.ID,
	localMultiaddr ma.Multiaddr,

	remotePeer peer.ID,
	remoteKey ic.PubKey,
	remoteMultiaddr ma.Multiaddr,
	incomingDataChannels chan dataChannel,
	peerConnectionClosedCh chan struct{},
) (*connection, error) {
	// 创建上下文和取消函数
	ctx, cancel := context.WithCancel(context.Background())

	// 初始化连接对象
	c := &connection{
		pc:        pc,
		transport: transport,
		scope:     scope,

		localPeer:      localPeer,
		localMultiaddr: localMultiaddr,

		remotePeer:      remotePeer,
		remoteKey:       remoteKey,
		remoteMultiaddr: remoteMultiaddr,
		ctx:             ctx,
		cancel:          cancel,
		streams:         make(map[uint16]*stream),

		acceptQueue: incomingDataChannels,
	}

	// 根据连接方向设置初始流 ID
	switch direction {
	case network.DirInbound:
		c.nextStreamID.Store(1)
	case network.DirOutbound:
		// 流 ID 0 用于 Noise 握手流
		c.nextStreamID.Store(2)
	}

	// 设置连接状态变化回调
	pc.OnConnectionStateChange(c.onConnectionStateChange)

	// 设置 SCTP 关闭回调
	if sctp := pc.SCTP(); sctp != nil {
		sctp.OnClose(func(err error) {
			if err != nil {
				c.closeWithError(fmt.Errorf("%w: %w", errConnClosed, err))
			}
			c.closeWithError(errConnClosed)
		})
	}

	// 检查对等连接是否已关闭
	select {
	case <-peerConnectionClosedCh:
		c.Close()
		return nil, errConnClosed
	default:
	}
	return c, nil
}

// ConnState 实现 transport.CapableConn 接口
// 返回值:
//   - network.ConnectionState 连接状态
func (c *connection) ConnState() network.ConnectionState {
	return network.ConnectionState{Transport: "webrtc-direct"}
}

// Close 关闭底层对等连接
// 返回值:
//   - error 错误信息
func (c *connection) Close() error {
	c.closeWithError(errConnClosed)
	return nil
}

// closeWithError 在底层 DTLS 连接失败时用于关闭连接
// 参数:
//   - err: error 错误信息
func (c *connection) closeWithError(err error) {
	c.closeOnce.Do(func() {
		// 设置关闭错误
		c.closeErr = err
		// 在设置 closeErr 后调用 cancel。这确保等待 ctx.Done 的 goroutine 可以在不持有连接锁的情况下读取 closeErr
		c.cancel()
		// 关闭对等连接将关闭与流关联的数据通道
		c.pc.Close()

		// 关闭所有流
		c.m.Lock()
		streams := c.streams
		c.streams = nil
		c.m.Unlock()
		for _, s := range streams {
			s.closeForShutdown(err)
		}
		c.scope.Done()
	})
}

// IsClosed 检查连接是否已关闭
// 返回值:
//   - bool 是否已关闭
func (c *connection) IsClosed() bool {
	return c.ctx.Err() != nil
}

// OpenStream 打开一个新的流
// 参数:
//   - ctx: context.Context 上下文
//
// 返回值:
//   - network.MuxedStream 多路复用流
//   - error 错误信息
func (c *connection) OpenStream(ctx context.Context) (network.MuxedStream, error) {
	// 检查连接是否已关闭
	if c.IsClosed() {
		log.Debugf("连接已关闭: %s", c.closeErr)
		return nil, c.closeErr
	}

	// 获取新的流 ID
	id := c.nextStreamID.Add(2) - 2
	if id > math.MaxUint16 {
		log.Debugf("流 ID 空间已耗尽")
		return nil, errors.New("流 ID 空间已耗尽")
	}
	streamID := uint16(id)

	// 创建数据通道
	dc, err := c.pc.CreateDataChannel("", &webrtc.DataChannelInit{ID: &streamID})
	if err != nil {
		log.Debugf("创建数据通道时出错: %s", err)
		return nil, err
	}

	// 分离数据通道
	rwc, err := c.detachChannel(ctx, dc)
	if err != nil {
		// 在 webrtc.SCTP.OnClose 回调和底层关联关闭之间存在竞争。在这里关闭连接更好
		if errors.Is(err, sctp.ErrStreamClosed) {
			log.Debugf("SCTP 流已关闭: %s", err)
			c.closeWithError(errConnClosed)
			return nil, c.closeErr
		}
		dc.Close()
		log.Debugf("分离流(%d)的通道失败: %s", streamID, err)
		return nil, fmt.Errorf("分离流(%d)的通道失败: %w", streamID, err)
	}

	// 创建新流
	str := newStream(dc, rwc, func() { c.removeStream(streamID) })
	if err := c.addStream(str); err != nil {
		log.Debugf("将流(%d)添加到连接失败: %s", streamID, err)
		str.Reset()
		return nil, fmt.Errorf("将流(%d)添加到连接失败: %w", streamID, err)
	}
	return str, nil
}

// AcceptStream 接受一个新的流
// 返回值:
//   - network.MuxedStream 多路复用流
//   - error 错误信息
func (c *connection) AcceptStream() (network.MuxedStream, error) {
	select {
	case <-c.ctx.Done():
		log.Debugf("连接已关闭: %s", c.closeErr)
		return nil, c.closeErr
	case dc := <-c.acceptQueue:
		str := newStream(dc.channel, dc.stream, func() { c.removeStream(*dc.channel.ID()) })
		if err := c.addStream(str); err != nil {
			log.Debugf("将流(%d)添加到连接失败: %s", *dc.channel.ID(), err)
			str.Reset()
			return nil, fmt.Errorf("将流(%d)添加到连接失败: %w", *dc.channel.ID(), err)
		}
		return str, nil
	}
}

// LocalPeer 返回本地节点 ID
// 返回值:
//   - peer.ID 本地节点 ID
func (c *connection) LocalPeer() peer.ID { return c.localPeer }

// RemotePeer 返回远程节点 ID
// 返回值:
//   - peer.ID 远程节点 ID
func (c *connection) RemotePeer() peer.ID { return c.remotePeer }

// RemotePublicKey 返回远程公钥
// 返回值:
//   - ic.PubKey 远程公钥
func (c *connection) RemotePublicKey() ic.PubKey { return c.remoteKey }

// LocalMultiaddr 返回本地多地址
// 返回值:
//   - ma.Multiaddr 本地多地址
func (c *connection) LocalMultiaddr() ma.Multiaddr { return c.localMultiaddr }

// RemoteMultiaddr 返回远程多地址
// 返回值:
//   - ma.Multiaddr 远程多地址
func (c *connection) RemoteMultiaddr() ma.Multiaddr { return c.remoteMultiaddr }

// Scope 返回连接管理范围
// 返回值:
//   - network.ConnScope 连接管理范围
func (c *connection) Scope() network.ConnScope { return c.scope }

// Transport 返回传输层
// 返回值:
//   - tpt.Transport 传输层
func (c *connection) Transport() tpt.Transport { return c.transport }

// addStream 添加一个流
// 参数:
//   - str: *stream 要添加的流
//
// 返回值:
//   - error 错误信息
func (c *connection) addStream(str *stream) error {
	c.m.Lock()
	defer c.m.Unlock()
	if c.streams == nil {
		return c.closeErr
	}
	if _, ok := c.streams[str.id]; ok {
		log.Debugf("流 ID 已存在")
		return errors.New("流 ID 已存在")
	}
	c.streams[str.id] = str
	return nil
}

// removeStream 移除一个流
// 参数:
//   - id: uint16 要移除的流 ID
func (c *connection) removeStream(id uint16) {
	c.m.Lock()
	defer c.m.Unlock()
	delete(c.streams, id)
}

// onConnectionStateChange 处理连接状态变化
// 参数:
//   - state: webrtc.PeerConnectionState 连接状态
func (c *connection) onConnectionStateChange(state webrtc.PeerConnectionState) {
	if state == webrtc.PeerConnectionStateFailed || state == webrtc.PeerConnectionStateClosed {
		c.closeWithError(errConnectionTimeout{})
	}
}

// detachChannel 分离传出通道,同时考虑传递给 OpenStream 的上下文和底层对等连接的关闭
//
// 数据通道的底层 SCTP 流实现了 net.Conn 接口。
// 但是,数据通道创建了一个持续从 SCTP 流读取并使用 OnMessage 回调传递数据的 goroutine。
//
// 实际的抽象如下: webrtc.DataChannel 包装 pion.DataChannel,后者包装 sctp.Stream。
//
// 读取 goroutine、Detach 方法和 OnMessage 回调存在于 webrtc.DataChannel 层。
// Detach 提供了对底层 pion.DataChannel 的抽象访问,允许我们对数据通道发出 Read 调用。
// 这是必要的,因为使用 OnMessage 回调无法引入背压。
// 代价是 OnOpen 回调语义的改变,以及必须在本地强制关闭 Read。
//
// 参数:
//   - ctx: context.Context 上下文
//   - dc: *webrtc.DataChannel 数据通道
//
// 返回值:
//   - datachannel.ReadWriteCloser 读写关闭器
//   - error 错误信息
func (c *connection) detachChannel(ctx context.Context, dc *webrtc.DataChannel) (datachannel.ReadWriteCloser, error) {
	done := make(chan struct{})

	var rwc datachannel.ReadWriteCloser
	var err error
	// 对于已分离的数据通道,OnOpen 将立即返回
	// 参考: https://github.com/pion/webrtc/blob/7ab3174640b3ce15abebc2516a2ca3939b5f105f/datachannel.go#L278-L282
	dc.OnOpen(func() {
		rwc, err = dc.Detach()
		// 这是安全的,因为如果对等连接关闭,函数应该立即返回
		close(done)
	})
	select {
	case <-c.ctx.Done():
		return nil, c.closeErr
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-done:
		return rwc, err
	}
}
