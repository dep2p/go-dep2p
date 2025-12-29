// Package relay 提供中继服务模块的实现
package relay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/address"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              RelayTransport 实现
// ============================================================================

// RelayTransport 中继传输层实现
//
// 实现 Transport 接口，使 Relay 成为一等公民传输层。
// 这是 libp2p 架构的核心设计：Relay 不是独立 API，而是透明的 Transport。
//
// 地址格式：
//   - /ip4/<relay-ip>/udp/<port>/quic-v1/p2p/<relay-id>/p2p-circuit
//   - /ip4/<relay-ip>/udp/<port>/quic-v1/p2p/<relay-id>/p2p-circuit/p2p/<dest-id>
type RelayTransport struct {
	client   *RelayClient
	endpoint endpoint.Endpoint

	// 入站连接通道（用于 Listen）
	incoming chan transportif.Conn
	listener *relayListener

	mu     sync.RWMutex
	closed bool
}

// NewRelayTransport 创建中继传输层
func NewRelayTransport(client *RelayClient, ep endpoint.Endpoint) *RelayTransport {
	return &RelayTransport{
		client:   client,
		endpoint: ep,
		incoming: make(chan transportif.Conn, 32),
	}
}

// 确保实现接口
var _ transportif.Transport = (*RelayTransport)(nil)

// ============================================================================
//                              Transport 接口实现
// ============================================================================

// Dial 通过中继建立出站连接
//
// 地址格式：
//   - /...relay.../p2p-circuit/p2p/<dest-id>
func (t *RelayTransport) Dial(ctx context.Context, addr endpoint.Address) (transportif.Conn, error) {
	return t.DialWithOptions(ctx, addr, transportif.DefaultDialOptions())
}

// DialWithOptions 使用选项通过中继建立连接
func (t *RelayTransport) DialWithOptions(ctx context.Context, addr endpoint.Address, opts transportif.DialOptions) (transportif.Conn, error) {
	t.mu.RLock()
	if t.closed {
		t.mu.RUnlock()
		return nil, errors.New("transport closed")
	}
	t.mu.RUnlock()

	// 解析中继地址
	addrStr := addr.String()
	relayInfo, err := types.ParseRelayAddr(addrStr)
	if err != nil {
		return nil, fmt.Errorf("invalid relay address: %w", err)
	}

	// 确保有目标节点
	if relayInfo.DestID.IsEmpty() {
		return nil, errors.New("relay address missing destination node ID")
	}

	log.Debug("RelayTransport 拨号",
		"relay", relayInfo.RelayID.ShortString(),
		"dest", relayInfo.DestID.ShortString(),
	)

	// 设置超时
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	// 首先确保中继节点有地址
	if t.endpoint != nil {
		// 解析中继地址的基础部分并添加到地址簿
		t.ensureRelayInAddressBook(relayInfo)
	}

	// 通过 RelayClient 建立连接
	conn, err := t.client.Connect(ctx, relayInfo.RelayID, relayInfo.DestID)
	if err != nil {
		return nil, fmt.Errorf("relay connect failed: %w", err)
	}

	log.Info("RelayTransport 连接成功",
		"relay", relayInfo.RelayID.ShortString(),
		"dest", relayInfo.DestID.ShortString(),
	)

	return conn, nil
}

// Listen 监听中继入站连接
//
// 对于 RelayTransport，Listen 实际上是注册为中继目标。
// 这需要先在中继节点上进行 Reserve。
func (t *RelayTransport) Listen(addr endpoint.Address) (transportif.Listener, error) {
	return t.ListenWithOptions(addr, transportif.DefaultListenOptions())
}

// ListenWithOptions 使用选项监听中继入站连接
func (t *RelayTransport) ListenWithOptions(addr endpoint.Address, opts transportif.ListenOptions) (transportif.Listener, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil, errors.New("transport closed")
	}

	if t.listener != nil {
		return nil, errors.New("listener already exists")
	}

	// 解析地址获取中继节点
	relayInfo, err := types.ParseRelayAddr(addr.String())
	if err != nil {
		return nil, fmt.Errorf("invalid relay listen address: %w", err)
	}

	t.listener = &relayListener{
		transport: t,
		relayID:   relayInfo.RelayID,
		addr:      addr,
		incoming:  t.incoming,
		closeCh:   make(chan struct{}),
	}

	log.Info("RelayTransport 开始监听",
		"relay", relayInfo.RelayID.ShortString(),
	)

	return t.listener, nil
}

// Protocols 返回支持的协议
func (t *RelayTransport) Protocols() []string {
	return []string{types.RelayAddrProtocol}
}

// CanDial 检查是否可以拨号到指定地址
func (t *RelayTransport) CanDial(addr endpoint.Address) bool {
	return types.IsRelayAddr(addr.String())
}

// Proxy 返回是否为代理传输
//
// RelayTransport 是代理传输，流量通过中继节点转发
func (t *RelayTransport) Proxy() bool {
	return true
}

// Close 关闭传输层
func (t *RelayTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}
	t.closed = true

	// 关闭监听器
	if t.listener != nil {
		t.listener.Close()
		t.listener = nil
	}

	// 关闭入站通道
	close(t.incoming)

	log.Info("RelayTransport 已关闭")
	return nil
}

// ============================================================================
//                              辅助方法
// ============================================================================

// ensureRelayInAddressBook 确保中继节点在地址簿中
func (t *RelayTransport) ensureRelayInAddressBook(info *types.RelayAddrInfo) {
	if t.endpoint == nil {
		return
	}

	if info == nil || info.RelayID.IsEmpty() || info.RelayAddr == "" {
		return
	}

	ab := t.endpoint.AddressBook()
	if ab == nil {
		return
	}

	// RelayAddrInfo.RelayAddr 的格式为：/ip4/.../udp/.../quic-v1/p2p/<relayID>
	// 这是"中继节点本身"的可拨号 base addr。它必须存在于 AddressBook 中，
	// 否则 AutoRelay 在 announceRelayAddresses() 阶段无法构造完整的 relay-circuit 地址，
	// 进而导致其他节点只能拿到不可拨号的 /p2p/<relayID>/p2p-circuit/... 片段，最终退化为直连失败。
	addr := address.NewAddr(types.Multiaddr(info.RelayAddr))

	// 去重：避免反复追加相同地址
	for _, existing := range ab.Get(info.RelayID) {
		if existing != nil && existing.String() == addr.String() {
			return
		}
	}

	ab.Add(info.RelayID, addr)
	log.Debug("已添加中继节点到地址簿",
		"relay", info.RelayID.ShortString(),
		"addr", info.RelayAddr,
	)
}

// relayBaseAddress 已删除，统一使用 address.Addr

// DeliverInbound 投递入站连接
//
// 由 HandleStopConnect 调用，将通过中继建立的入站连接投递到监听器
func (t *RelayTransport) DeliverInbound(conn transportif.Conn) error {
	t.mu.RLock()
	closed := t.closed
	t.mu.RUnlock()

	if closed {
		return errors.New("transport closed")
	}

	select {
	case t.incoming <- conn:
		log.Debug("投递中继入站连接")
		return nil
	default:
		log.Warn("中继入站连接通道已满，丢弃连接")
		return errors.New("incoming channel full")
	}
}

// HandleStopConnect 处理来自 Relay Server 的 STOP 连接请求
//
// 当 Relay Server 需要将连接转发到本节点时，它会打开一个 ProtocolRelayHop 流
// 并发送 STOP 连接请求。本方法处理该请求并将连接投递到监听器。
//
// 协议流程:
//  1. Relay Server 发送: MsgStopConnect(10) + version(1) + relayID(32) + srcID(32)
//  2. 本节点响应: MsgStopConnectOK(11) + version(1) + padding(2)
//  3. 流转变为数据通道，投递到 incoming channel
func (t *RelayTransport) HandleStopConnect(stream endpoint.Stream) {
	conn := stream.Connection()
	if conn == nil {
		log.Debug("收到无连接的 STOP 流")
		_ = stream.Close()
		return
	}

	// 读取 STOP 请求: type(1) + version(1) + relayID(32) + srcID(32) = 66 bytes
	// 使用 io.ReadFull 确保读取完整的消息，避免部分读取导致解析错误
	buf := make([]byte, 66)
	if _, err := io.ReadFull(stream, buf); err != nil {
		log.Debug("读取 STOP 请求失败", "err", err)
		_ = stream.Close()
		return
	}

	// 验证消息类型
	msgType := buf[0]
	if msgType != 10 { // MsgStopConnect
		log.Debug("非 STOP 连接请求", "msgType", msgType)
		_ = stream.Close()
		return
	}

	// 解析 relayID 和 srcID
	var relayID, srcID types.NodeID
	copy(relayID[:], buf[2:34])
	copy(srcID[:], buf[34:66])

	log.Debug("收到 STOP 连接请求",
		"relay", relayID.ShortString(),
		"src", srcID.ShortString(),
	)

	// 发送成功响应: type(1) + version(1) + padding(2) = 4 bytes
	resp := []byte{11, 1, 0, 0} // MsgStopConnectOK
	if _, err := stream.Write(resp); err != nil {
		log.Debug("发送 STOP 响应失败", "err", err)
		_ = stream.Close()
		return
	}

	// 创建中继连接包装器并投递
	relayConn := &relayInboundConn{
		stream:  stream,
		relayID: relayID,
		srcID:   srcID,
		localID: t.endpoint.ID(),
	}

	if err := t.DeliverInbound(relayConn); err != nil {
		log.Warn("投递入站中继连接失败", "err", err)
		_ = stream.Close()
		return
	}

	log.Info("中继入站连接已建立",
		"relay", relayID.ShortString(),
		"src", srcID.ShortString(),
	)
}

// ============================================================================
//                              relayListener 实现
// ============================================================================

// relayListener 中继监听器
type relayListener struct {
	transport *RelayTransport
	relayID   types.NodeID
	addr      endpoint.Address
	incoming  chan transportif.Conn
	closeCh   chan struct{}
	closed    bool
	mu        sync.Mutex
}

// Accept 接受入站连接
func (l *relayListener) Accept() (transportif.Conn, error) {
	select {
	case <-l.closeCh:
		return nil, errors.New("listener closed")
	case conn, ok := <-l.incoming:
		if !ok {
			return nil, errors.New("listener closed")
		}
		return conn, nil
	}
}

// Addr 返回监听地址
func (l *relayListener) Addr() endpoint.Address {
	return l.addr
}

// Close 关闭监听器
func (l *relayListener) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return nil
	}
	l.closed = true
	close(l.closeCh)

	log.Debug("RelayListener 已关闭",
		"relay", l.relayID.ShortString(),
	)
	return nil
}

// Multiaddr 返回多地址格式的监听地址
func (l *relayListener) Multiaddr() string {
	return l.addr.String()
}

// ============================================================================
//                              relayInboundConn 实现
// ============================================================================

// relayInboundConn 中继入站连接包装器
//
// 将 endpoint.Stream 包装为 transportif.Conn，用于通过 RelayTransport.DeliverInbound()
// 投递到监听器。
type relayInboundConn struct {
	stream  endpoint.Stream
	relayID types.NodeID
	srcID   types.NodeID
	localID types.NodeID
}

// 确保实现接口
var _ transportif.Conn = (*relayInboundConn)(nil)

// Read 从连接读取数据
func (c *relayInboundConn) Read(p []byte) (n int, err error) {
	return c.stream.Read(p)
}

// Write 向连接写入数据
func (c *relayInboundConn) Write(p []byte) (n int, err error) {
	return c.stream.Write(p)
}

// Close 关闭连接
func (c *relayInboundConn) Close() error {
	return c.stream.Close()
}

// LocalAddr 返回本地地址
func (c *relayInboundConn) LocalAddr() endpoint.Address {
	// 构建中继地址格式
	return address.NewAddr(types.BuildSimpleRelayCircuit(c.relayID, c.localID))
}

// RemoteAddr 返回远程地址
func (c *relayInboundConn) RemoteAddr() endpoint.Address {
	// 构建中继地址格式
	return address.NewAddr(types.BuildSimpleRelayCircuit(c.relayID, c.srcID))
}

// LocalNetAddr 返回底层 net.Addr
func (c *relayInboundConn) LocalNetAddr() net.Addr {
	return &relayNetAddr{nodeID: c.localID, relayID: c.relayID}
}

// RemoteNetAddr 返回底层远程 net.Addr
func (c *relayInboundConn) RemoteNetAddr() net.Addr {
	return &relayNetAddr{nodeID: c.srcID, relayID: c.relayID}
}

// SetDeadline 设置读写超时
func (c *relayInboundConn) SetDeadline(t time.Time) error {
	return c.stream.SetDeadline(t)
}

// SetReadDeadline 设置读超时
func (c *relayInboundConn) SetReadDeadline(t time.Time) error {
	return c.stream.SetReadDeadline(t)
}

// SetWriteDeadline 设置写超时
func (c *relayInboundConn) SetWriteDeadline(t time.Time) error {
	return c.stream.SetWriteDeadline(t)
}

// IsClosed 检查连接是否已关闭
func (c *relayInboundConn) IsClosed() bool {
	// Stream 没有直接的 IsClosed 方法，尝试通过写入空数据检测
	return false
}

// Transport 返回传输协议名称
func (c *relayInboundConn) Transport() string {
	return "relay"
}

// simpleRelayAddress 已删除，统一使用 address.Addr

// relayNetAddr net.Addr 实现
type relayNetAddr struct {
	nodeID  types.NodeID
	relayID types.NodeID
}

func (a *relayNetAddr) Network() string { return "relay" }
func (a *relayNetAddr) String() string {
	return string(types.BuildSimpleRelayCircuit(a.relayID, a.nodeID))
}

