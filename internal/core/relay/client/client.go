// Package client 实现中继客户端
package client

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/muxer"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/lib/multiaddr"
	"github.com/dep2p/go-dep2p/pkg/protocol"
	"github.com/dep2p/go-dep2p/pkg/types"
	mss "github.com/multiformats/go-multistream"
)

var clientLogger = log.Logger("relay/client")

var (
	// ErrMessageTooLarge 消息过大
	ErrMessageTooLarge = errors.New("relay: message too large")
)

// MaxMessageSize 最大消息大小 (64KB)
const MaxMessageSize = 64 * 1024

// HOP 协议 ID（使用统一定义）
var (
	HopProtocolID  = string(protocol.RelayHop)
	StopProtocolID = string(protocol.RelayStop)
)

// 消息类型
const (
	MsgTypeReserve = 0
	MsgTypeConnect = 1
	MsgTypeStatus  = 2
)

// 状态码
const (
	StatusOK                    = 0
	StatusPermissionDenied      = 1
	StatusNoReservation         = 2
	StatusResourceLimitExceeded = 3
	StatusMalformedMessage      = 4
	StatusUnexpectedMessage     = 5
	// 细分状态码
	StatusTargetUnreachable = 6 // 目标节点不可达
	StatusProtocolError     = 7 // 协议协商失败
	StatusInternalError     = 8 // 内部错误
)

// Client 中继客户端
type Client struct {
	swarm     pkgif.Swarm
	relayPeer types.PeerID
	relayAddr types.Multiaddr

	conn   pkgif.Connection
	stream pkgif.Stream

	reservation *Reservation

	mu     sync.RWMutex
	closed bool
}

// Reservation 预约信息
type Reservation struct {
	RelayPeer  types.PeerID
	ExpireTime time.Time
}

// NewClient 创建中继客户端
func NewClient(swarm pkgif.Swarm, relayPeer types.PeerID, relayAddr types.Multiaddr) *Client {
	return &Client{
		swarm:     swarm,
		relayPeer: relayPeer,
		relayAddr: relayAddr,
	}
}

// Reserve 预约中继资源
func (c *Client) Reserve(ctx context.Context) (*Reservation, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, ErrClientClosed
	}

	// 1. 连接到中继节点
	if c.conn == nil {
		conn, err := c.swarm.DialPeer(ctx, string(c.relayPeer))
		if err != nil {
			return nil, err
		}
		c.conn = conn
	}

	// 2. 打开流
	stream, err := c.conn.NewStream(ctx)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	// 进行 HOP 协议协商
	selectedProto, err := mss.SelectOneOf([]string{HopProtocolID}, stream)
	if err != nil {
		return nil, fmt.Errorf("HOP 协议协商失败: %w", err)
	}
	stream.SetProtocol(selectedProto)

	// 3. 发送 RESERVE 消息
	if err := c.writeMessage(stream, MsgTypeReserve, nil); err != nil {
		return nil, err
	}

	// 4. 读取响应
	msgType, data, err := c.readMessage(stream)
	if err != nil {
		return nil, err
	}

	if msgType != MsgTypeStatus {
		return nil, ErrUnexpectedMessage
	}

	status := decodeStatus(data)
	if status != StatusOK {
		return nil, statusToError(status)
	}

	// 5. 解析预约信息
	reservation := &Reservation{
		RelayPeer:  c.relayPeer,
		ExpireTime: time.Now().Add(1 * time.Hour), // 默认 1 小时
	}

	c.reservation = reservation
	return reservation, nil
}

// Connect 通过中继连接目标节点
func (c *Client) Connect(ctx context.Context, target types.PeerID) (pkgif.Connection, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, ErrClientClosed
	}

	// 检查预约
	if c.reservation == nil {
		return nil, ErrNoReservation
	}

	if time.Now().After(c.reservation.ExpireTime) {
		c.reservation = nil
		return nil, ErrReservationExpired
	}

	// 1. 打开流
	if c.conn == nil {
		return nil, ErrNotConnected
	}

	stream, err := c.conn.NewStream(ctx)
	if err != nil {
		return nil, err
	}

	// 进行 HOP 协议协商
	selectedProto, err := mss.SelectOneOf([]string{HopProtocolID}, stream)
	if err != nil {
		stream.Close()
		return nil, fmt.Errorf("HOP 协议协商失败: %w", err)
	}
	stream.SetProtocol(selectedProto)

	// 2. 发送 CONNECT 消息 + 目标节点 ID
	if err := c.writeMessage(stream, MsgTypeConnect, []byte(target)); err != nil {
		stream.Close()
		return nil, err
	}

	// 3. 读取响应
	msgType, data, err := c.readMessage(stream)
	if err != nil {
		stream.Close()
		return nil, err
	}

	if msgType != MsgTypeStatus {
		stream.Close()
		return nil, ErrUnexpectedMessage
	}

	status := decodeStatus(data)
	if status != StatusOK {
		stream.Close()
		return nil, statusToError(status)
	}

	// 4. 创建 RelayCircuit（支持多路复用）
	//
	relayTransportAddr := extractTransportAddr(c.conn.RemoteMultiaddr())

	circuit, err := createRelayCircuit(
		stream,
		false, // isClient
		types.PeerID(c.swarm.LocalPeer()),
		target,
		c.relayPeer,
		relayTransportAddr,
	)
	if err != nil {
		stream.Close()
		return nil, fmt.Errorf("创建中继电路失败: %w", err)
	}

	clientLogger.Info("中继电路已建立",
		"target", safePeerIDPrefix(target),
		"relay", safePeerIDPrefix(c.relayPeer),
		"relayTransportAddr", relayTransportAddr)

	return circuit, nil
}

// ConnectAsInitiator 作为主动方通过中继连接目标节点
//
// 符合 Circuit Relay v2 协议：主动方不需要 reservation。
// 与 Connect 方法的区别：
//   - 不检查 reservation（主动方不需要在 Relay 上预约）
//   - 如果没有连接，会自动连接到 Relay 服务器
//
// 使用场景：
//   - 云服务器（主动方）通过用户提供的 Relay 地址连接 NAT 后的节点（被动方）
//   - 被动方需要已在 Relay 上做过 Reserve
//
// 参数：
//   - ctx: 上下文
//   - target: 目标节点 ID（被动方，需要已在 Relay 上 Reserve）
//
// 返回：
//   - pkgif.Connection: 中继电路连接
//   - error: 错误信息
func (c *Client) ConnectAsInitiator(ctx context.Context, target types.PeerID) (pkgif.Connection, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil, ErrClientClosed
	}

	// 1. 确保已连接到 Relay 服务器
	if c.conn == nil {
		clientLogger.Debug("ConnectAsInitiator: 连接到 Relay 服务器",
			"relay", safePeerIDPrefix(c.relayPeer))
		conn, err := c.swarm.DialPeer(ctx, string(c.relayPeer))
		if err != nil {
			return nil, fmt.Errorf("connect to relay server: %w", err)
		}
		c.conn = conn
	}

	// 2. 打开流
	stream, err := c.conn.NewStream(ctx)
	if err != nil {
		return nil, fmt.Errorf("open stream to relay: %w", err)
	}

	// 进行 HOP 协议协商
	selectedProto, err := mss.SelectOneOf([]string{HopProtocolID}, stream)
	if err != nil {
		stream.Close()
		return nil, fmt.Errorf("HOP 协议协商失败: %w", err)
	}
	stream.SetProtocol(selectedProto)

	// 3. 发送 CONNECT 消息 + 目标节点 ID
	// 注意：主动方不需要 reservation，直接发送 CONNECT 请求
	clientLogger.Debug("ConnectAsInitiator: 发送 CONNECT 请求",
		"target", safePeerIDPrefix(target),
		"relay", safePeerIDPrefix(c.relayPeer))

	if err := c.writeMessage(stream, MsgTypeConnect, []byte(target)); err != nil {
		stream.Close()
		return nil, fmt.Errorf("send CONNECT message: %w", err)
	}

	// 4. 读取响应
	msgType, data, err := c.readMessage(stream)
	if err != nil {
		stream.Close()
		return nil, fmt.Errorf("read CONNECT response: %w", err)
	}

	if msgType != MsgTypeStatus {
		stream.Close()
		return nil, ErrUnexpectedMessage
	}

	status := decodeStatus(data)
	if status != StatusOK {
		stream.Close()
		clientLogger.Warn("ConnectAsInitiator: Relay 拒绝连接",
			"status", status,
			"target", safePeerIDPrefix(target),
			"relay", safePeerIDPrefix(c.relayPeer))
		return nil, statusToError(status)
	}

	// 5. 创建 RelayCircuit（支持多路复用）
	//
	relayTransportAddr := extractTransportAddr(c.conn.RemoteMultiaddr())

	circuit, err := createRelayCircuit(
		stream,
		false, // isClient
		types.PeerID(c.swarm.LocalPeer()),
		target,
		c.relayPeer,
		relayTransportAddr,
	)
	if err != nil {
		stream.Close()
		return nil, fmt.Errorf("创建中继电路失败: %w", err)
	}

	clientLogger.Info("ConnectAsInitiator: 中继电路已建立",
		"target", safePeerIDPrefix(target),
		"relay", safePeerIDPrefix(c.relayPeer),
		"relayTransportAddr", relayTransportAddr)

	return circuit, nil
}

// createRelayCircuit 创建中继电路
//
// 在 STOP 流上叠加 yamux muxer，实现多路复用。
func createRelayCircuit(
	stream pkgif.Stream,
	isServer bool,
	localPeer, remotePeer, relayPeer types.PeerID,
	relayTransportAddr types.Multiaddr,
) (*RelayCircuit, error) {
	// 创建流到 net.Conn 的适配器
	netConn := newStreamNetConn(stream)

	// 创建 yamux muxer
	transport := muxer.NewTransport()
	muxedConn, err := transport.NewConn(netConn, isServer, nil)
	if err != nil {
		clientLogger.Error("创建 muxer 失败", "error", err)
		return nil, err
	}

	opts := DefaultCircuitOptions()
	if isServer {
		opts.Direction = pkgif.DirInbound
		opts.Initiator = false
	} else {
		opts.Direction = pkgif.DirOutbound
		opts.Initiator = true
	}

	circuit := NewRelayCircuitWithOptions(stream, muxedConn, localPeer, remotePeer, relayPeer, relayTransportAddr, opts)
	if err := circuit.initControlAsInitiator(); err != nil {
		_ = circuit.Close()
		return nil, err
	}

	return circuit, nil
}

// extractTransportAddr 从完整地址中提取传输层地址（不含 /p2p/<ID>）
//
// 输入: /ip4/1.2.3.4/udp/4001/quic-v1/p2p/QmRelay...
// 输出: /ip4/1.2.3.4/udp/4001/quic-v1
func extractTransportAddr(addr types.Multiaddr) types.Multiaddr {
	if addr == nil {
		return nil
	}
	addrStr := addr.String()
	// 找到 /p2p/ 的位置，截取之前的部分
	idx := strings.LastIndex(addrStr, "/p2p/")
	if idx == -1 {
		return addr // 没有 /p2p/ 后缀，直接返回
	}
	transportStr := addrStr[:idx]
	if transportStr == "" {
		return nil
	}
	transportAddr, err := multiaddr.NewMultiaddr(transportStr)
	if err != nil {
		clientLogger.Warn("提取传输地址失败", "addr", addrStr, "error", err)
		return nil
	}
	return transportAddr
}

// safePeerIDPrefix 安全获取 PeerID 前缀
func safePeerIDPrefix(peerID types.PeerID) string {
	s := string(peerID)
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

// Close 关闭客户端
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	if c.stream != nil {
		c.stream.Close()
	}

	if c.conn != nil {
		c.conn.Close()
	}

	return nil
}

// writeMessage 写入消息
func (c *Client) writeMessage(w io.Writer, msgType byte, data []byte) error {
	// 简化的消息格式: [1 byte type][4 bytes length][data]
	header := make([]byte, 5)
	header[0] = msgType
	binary.BigEndian.PutUint32(header[1:], uint32(len(data)))

	if _, err := w.Write(header); err != nil {
		return err
	}

	if len(data) > 0 {
		if _, err := w.Write(data); err != nil {
			return err
		}
	}

	return nil
}

// readMessage 读取消息
func (c *Client) readMessage(r io.Reader) (byte, []byte, error) {
	// 读取消息头
	header := make([]byte, 5)
	if _, err := io.ReadFull(r, header); err != nil {
		return 0, nil, err
	}

	msgType := header[0]
	length := binary.BigEndian.Uint32(header[1:])

	// 检查消息大小，防止内存攻击
	if length > MaxMessageSize {
		return 0, nil, fmt.Errorf("%w: %d > %d", ErrMessageTooLarge, length, MaxMessageSize)
	}

	// 读取消息体
	if length > 0 {
		data := make([]byte, length)
		if _, err := io.ReadFull(r, data); err != nil {
			return 0, nil, err
		}
		return msgType, data, nil
	}

	return msgType, nil, nil
}

// decodeStatus 解码状态
func decodeStatus(data []byte) int {
	if len(data) == 0 {
		return StatusOK
	}
	return int(data[0])
}

// statusToError 状态码转错误
func statusToError(status int) error {
	switch status {
	case StatusPermissionDenied:
		return ErrPermissionDenied
	case StatusNoReservation:
		return ErrNoReservation
	case StatusResourceLimitExceeded:
		return ErrResourceLimitExceeded
	case StatusMalformedMessage:
		return ErrMalformedMessage
	case StatusTargetUnreachable:
		return ErrTargetUnreachable
	case StatusProtocolError:
		return ErrProtocolError
	case StatusInternalError:
		return ErrInternalError
	default:
		return ErrUnknownStatus
	}
}
