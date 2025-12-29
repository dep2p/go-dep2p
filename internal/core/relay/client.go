// Package relay 提供中继服务模块的实现
package relay

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/address"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	realmif "github.com/dep2p/go-dep2p/pkg/interfaces/realm"
	relayif "github.com/dep2p/go-dep2p/pkg/interfaces/relay"
	transportif "github.com/dep2p/go-dep2p/pkg/interfaces/transport"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              RelayClient 实现
// ============================================================================

// RelayClient 中继客户端实现
type RelayClient struct {
	transport transportif.Transport
	dialer    relayif.Dialer // 使用最小 Dialer 接口，避免循环依赖
	config    relayif.Config

	// 已知中继
	relays   map[types.NodeID]*relayInfo
	relaysMu sync.RWMutex

	// 活跃预留
	reservations   map[types.NodeID]*reservation
	reservationsMu sync.RWMutex

	// IMPL-1227: PSK 认证器（用于 Realm Relay）
	pskAuth   realmif.PSKAuthenticator
	pskAuthMu sync.RWMutex

	// 状态
	closed int32
}

// relayInfo 中继信息
type relayInfo struct {
	nodeID    types.NodeID
	addrs     []endpoint.Address
	lastSeen  time.Time
	latency   time.Duration
	slots     int
	usedSlots int
}

// reservation 预留信息
type reservation struct {
	relay   types.NodeID
	expires time.Time
	slots   int
}

// NewRelayClient 创建中继客户端
//
// dialer: 提供连接能力的最小接口，通常由 Endpoint 实现
func NewRelayClient(transport transportif.Transport, dialer relayif.Dialer, config relayif.Config) *RelayClient {
	return &RelayClient{
		transport:    transport,
		dialer:       dialer,
		config:       config,
		relays:       make(map[types.NodeID]*relayInfo),
		reservations: make(map[types.NodeID]*reservation),
	}
}

// 确保实现接口
var _ relayif.RelayClient = (*RelayClient)(nil)

// ============================================================================
//                              预留功能
// ============================================================================

// Reserve 预留资源
//
// IMPL-1227: 支持 Realm Relay PSK 握手
// 如果 Relay Server 是 Realm Relay 模式，会发送 0x01 请求成员证明。
// Client 需要使用 PSK 认证器生成证明并发送。
func (c *RelayClient) Reserve(ctx context.Context, relay types.NodeID) (relayif.Reservation, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil, errors.New("client closed")
	}

	if c.dialer == nil {
		return nil, ErrRelayNotFound
	}

	log.Debug("预留中继资源",
		"relay", relay.ShortString())

	// 连接到中继
	conn, err := c.dialer.Connect(ctx, relay)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRelayNotFound, err)
	}

	// 打开预留流
	stream, err := conn.OpenStream(ctx, string(ProtocolRelay))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReservationFailed, err)
	}
	defer func() { _ = stream.Close() }()

	// 发送预留请求
	if err := c.writeReserveRequest(stream); err != nil {
		return nil, err
	}

	// IMPL-1227: 检查是否需要 PSK 认证
	// Realm Relay 会先发送 0x01 请求成员证明
	firstByte := make([]byte, 1)
	if _, err := io.ReadFull(stream, firstByte); err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if firstByte[0] == 0x01 {
		// Realm Relay 模式：需要发送成员证明
		log.Debug("收到 PSK 认证请求", "relay", relay.ShortString())

		c.pskAuthMu.RLock()
		pskAuth := c.pskAuth
		c.pskAuthMu.RUnlock()

		if pskAuth == nil {
			return nil, errors.New("realm relay requires PSK authenticator")
		}

		// 生成成员证明
		proof, err := pskAuth.Generate(ctx, relay)
		if err != nil {
			return nil, fmt.Errorf("generate membership proof: %w", err)
		}

		// 发送证明
		if err := c.writeMembershipProof(stream, proof); err != nil {
			return nil, fmt.Errorf("write membership proof: %w", err)
		}

		log.Debug("已发送 PSK 成员证明", "relay", relay.ShortString())

		// 继续读取响应的第一个字节
		if _, err := io.ReadFull(stream, firstByte); err != nil {
			return nil, fmt.Errorf("read response after proof: %w", err)
		}
	}

	// 读取响应（从 firstByte 继续）
	res, err := c.readReserveResponseWithFirstByte(stream, firstByte[0])
	if err != nil {
		return nil, err
	}

	// 保存预留信息
	c.reservationsMu.Lock()
	c.reservations[relay] = res
	c.reservationsMu.Unlock()

	log.Info("预留成功",
		"relay", relay.ShortString(),
		"expires", res.expires)

	return &reservationHandle{
		client: c,
		relay:  relay,
		res:    res,
	}, nil
}

// writeReserveRequest 发送预留请求
// 格式: type(1) + version(1) + TTL(4) = 6 字节
func (c *RelayClient) writeReserveRequest(w io.Writer) error {
	// 消息类型 + 版本
	if _, err := w.Write([]byte{MsgTypeReserve, 1}); err != nil {
		return err
	}
	// 请求的时长（秒），使用 ReservationTTL
	ttl := c.config.ReservationTTL
	if ttl == 0 {
		ttl = 1 * time.Hour // 默认 1 小时
	}
	return binary.Write(w, binary.BigEndian, uint32(ttl.Seconds()))
}

// readReserveResponse 读取预留响应
// 格式: type(1) + version(1) + [OK: TTL(4) + slots(2) + addrs] | [Error: code(2)]
func (c *RelayClient) readReserveResponse(r io.Reader) (*reservation, error) {
	// 读取消息头: type + version
	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}
	msgType := header[0]
	// version := header[1] // 版本号暂不使用

	if msgType == MsgTypeReserveError {
		// 读取错误码
		var errCode uint16
		if err := binary.Read(r, binary.BigEndian, &errCode); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%w: error code %d", ErrReservationFailed, errCode)
	}

	if msgType != MsgTypeReserveOK {
		return nil, ErrReservationFailed
	}

	// 读取过期时间 (4 字节)
	var expirySec uint32
	if err := binary.Read(r, binary.BigEndian, &expirySec); err != nil {
		return nil, err
	}

	// 读取槽位数 (2 字节)
	var slots uint16
	if err := binary.Read(r, binary.BigEndian, &slots); err != nil {
		return nil, err
	}

	return &reservation{
		expires: time.Now().Add(time.Duration(expirySec) * time.Second),
		slots:   int(slots),
	}, nil
}

// readReserveResponseWithFirstByte 读取预留响应（第一个字节已读取）
//
// IMPL-1227: 在 PSK 握手流程中，第一个字节可能是 0x01（proof request）或响应类型。
// 这个方法用于在已读取第一个字节后继续解析响应。
func (c *RelayClient) readReserveResponseWithFirstByte(r io.Reader, firstByte byte) (*reservation, error) {
	msgType := firstByte

	// 读取版本号
	versionByte := make([]byte, 1)
	if _, err := io.ReadFull(r, versionByte); err != nil {
		return nil, err
	}
	// version := versionByte[0] // 版本号暂不使用

	if msgType == MsgTypeReserveError {
		// 读取错误码
		var errCode uint16
		if err := binary.Read(r, binary.BigEndian, &errCode); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("%w: error code %d", ErrReservationFailed, errCode)
	}

	if msgType != MsgTypeReserveOK {
		return nil, fmt.Errorf("%w: unexpected message type %d", ErrReservationFailed, msgType)
	}

	// 读取过期时间 (4 字节)
	var expirySec uint32
	if err := binary.Read(r, binary.BigEndian, &expirySec); err != nil {
		return nil, err
	}

	// 读取槽位数 (2 字节)
	var slots uint16
	if err := binary.Read(r, binary.BigEndian, &slots); err != nil {
		return nil, err
	}

	return &reservation{
		expires: time.Now().Add(time.Duration(expirySec) * time.Second),
		slots:   int(slots),
	}, nil
}

// ============================================================================
//                              IMPL-1227: PSK 认证支持
// ============================================================================

// SetPSKAuthenticator 设置 PSK 认证器（用于 Realm Relay）
//
// 在调用 Reserve 前设置，以支持 Realm Relay 的 PSK 握手。
func (c *RelayClient) SetPSKAuthenticator(auth realmif.PSKAuthenticator) {
	c.pskAuthMu.Lock()
	defer c.pskAuthMu.Unlock()
	c.pskAuth = auth
}

// PSKAuthenticator 返回当前的 PSK 认证器
func (c *RelayClient) PSKAuthenticator() realmif.PSKAuthenticator {
	c.pskAuthMu.RLock()
	defer c.pskAuthMu.RUnlock()
	return c.pskAuth
}

// writeMembershipProof 发送成员证明
//
// 协议格式：
//   - 长度前缀: 2 bytes (big-endian)
//   - 证明数据: 变长（见 types.MembershipProof.Serialize）
func (c *RelayClient) writeMembershipProof(w io.Writer, proof *types.MembershipProof) error {
	// 序列化证明
	proofBytes := proof.Serialize()

	// 写入长度前缀
	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, uint16(len(proofBytes))) //nolint:gosec // G115: proof 大小由协议限制
	if _, err := w.Write(lenBuf); err != nil {
		return fmt.Errorf("write proof length: %w", err)
	}

	// 写入证明数据
	if _, err := w.Write(proofBytes); err != nil {
		return fmt.Errorf("write proof data: %w", err)
	}

	return nil
}

// ============================================================================
//                              连接功能
// ============================================================================

// Connect 通过中继连接到目标节点
//
// 设计说明（IMPL-1227 协议白名单）：
//   - Relay 建立的是透明双向隧道，具体协议协商在 stream 建立后才发生
//   - 因此 CONNECT 请求中的 Protocol 字段是可选的（用于预检查）
//   - 本方法不传入协议（ProtoLen=0），Server 端会跳过协议白名单检查
//   - 如需预检查协议合法性，请使用 ConnectWithProtocol
//
// 参数:
//   - relay: 中继服务器节点 ID
//   - dest: 目标节点 ID
//
// 返回: 到目标节点的连接（实际流量通过中继转发）
func (c *RelayClient) Connect(ctx context.Context, relay types.NodeID, dest types.NodeID) (transportif.Conn, error) {
	return c.ConnectWithProtocol(ctx, relay, dest, "")
}

// ConnectWithProtocol 通过中继连接到目标节点（支持协议预检查）
//
// 设计说明（IMPL-1227 协议白名单）：
//   - 如果 protocol 非空，Server 端会检查该协议是否在白名单中
//   - System Relay：只允许 /dep2p/sys/* 协议
//   - Realm Relay：只允许 /dep2p/app/<realmID>/* 和 /dep2p/realm/<realmID>/* 协议
//   - 如果 protocol 为空（""），Server 端跳过检查（透明隧道模式）
//
// 参数:
//   - relay: 中继服务器节点 ID
//   - dest: 目标节点 ID
//   - protocol: 目标协议（空字符串表示不检查）
//
// 返回: 到目标节点的连接（实际流量通过中继转发）
func (c *RelayClient) ConnectWithProtocol(ctx context.Context, relay types.NodeID, dest types.NodeID, protocol types.ProtocolID) (transportif.Conn, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil, errors.New("client closed")
	}

	if c.dialer == nil {
		return nil, ErrRelayNotFound
	}

	log.Debug("通过中继连接",
		"relay", relay.ShortString(),
		"dest", dest.ShortString(),
		"protocol", protocol)

	// 连接到中继
	conn, err := c.dialer.Connect(ctx, relay)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRelayNotFound, err)
	}

	// 打开连接流
	stream, err := conn.OpenStream(ctx, string(ProtocolRelayHop))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConnectFailed, err)
	}

	// 发送连接请求
	// protocol 为空时 ProtoLen=0，Server 跳过协议白名单检查
	if err := c.writeConnectRequest(stream, dest, protocol); err != nil {
		stream.Close()
		return nil, err
	}

	// 读取响应
	if err := c.readConnectResponse(stream); err != nil {
		stream.Close()
		return nil, err
	}

	log.Info("中继连接成功",
		"relay", relay.ShortString(),
		"dest", dest.ShortString())

	// 返回包装的连接
	return &relayedConn{
		stream:   stream,
		relay:    relay,
		dest:     dest,
		localID:  c.dialer.ID(),
		remoteID: dest,
	}, nil
}

// writeConnectRequest 发送连接请求
//
// 格式（与 server.readConnectRequest 对齐，IMPL-1227 扩展版）：
//   - MsgType:    1 byte
//   - Version:    1 byte
//   - DestPeerID: 32 bytes
//   - ProtoLen:   2 bytes (big-endian, 0 表示无协议)
//   - Protocol:   变长（可选）
func (c *RelayClient) writeConnectRequest(w io.Writer, dest types.NodeID, protocol types.ProtocolID) error {
	// 消息类型 + 版本
	if _, err := w.Write([]byte{MsgTypeConnect, 1}); err != nil {
		return err
	}
	// 目标节点 ID
	if _, err := w.Write(dest[:]); err != nil {
		return err
	}
	// 协议长度 + 协议内容（可选）
	protoBytes := []byte(string(protocol))
	lenBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenBuf, uint16(len(protoBytes))) //nolint:gosec // G115: 协议长度受限于上层约束
	if _, err := w.Write(lenBuf); err != nil {
		return err
	}
	if len(protoBytes) > 0 {
		if _, err := w.Write(protoBytes); err != nil {
			return err
		}
	}
	return nil
}

// readConnectResponse 读取连接响应
// 格式: type(1) + version(1) + [Error: code(2)]
func (c *RelayClient) readConnectResponse(r io.Reader) error {
	// 读取消息头: type + version
	header := make([]byte, 2)
	if _, err := io.ReadFull(r, header); err != nil {
		return err
	}
	msgType := header[0]
	// version := header[1] // 版本号暂不使用

	if msgType == MsgTypeConnectError {
		var errCode uint16
		if err := binary.Read(r, binary.BigEndian, &errCode); err != nil {
			return err
		}
		// 使用协议中定义的错误码进行判断
		ec := ErrorCode(errCode)
		switch ec {
		case ErrCodeMalformedMessage, ErrCodeUnexpectedMessage:
			return fmt.Errorf("%w: %s", ErrConnectFailed, ec.String())
		case ErrCodeResourceLimitHit:
			// 200: 资源限制（含：槽位已满/电路数已满等）
			return fmt.Errorf("%w: %s", ErrRelayBusy, ec.String())
		case ErrCodeRelayBusy:
			// 400: 兼容保留
			return ErrRelayBusy
		case ErrCodeNoReservation:
			// 201: 无预留
			return fmt.Errorf("%w: %s", ErrReservationFailed, ec.String())
		case ErrCodePermissionDenied:
			// 401: 权限错误（PSK 验证失败）
			return fmt.Errorf("%w: %s", ErrReservationFailed, ec.String())
		case ErrCodeProtocolNotAllowed:
			// 402: 协议不允许
			return fmt.Errorf("%w: %s", ErrConnectFailed, ec.String())
		case ErrCodeConnectionFailed:
			// 300: Relay Server 无法连接到目标节点
			return fmt.Errorf("%w: %s", ErrConnectFailed, ec.String())
		default:
			return fmt.Errorf("%w: %s (code %d)", ErrConnectFailed, ec.String(), errCode)
		}
	}

	if msgType != MsgTypeConnectOK {
		return ErrConnectFailed
	}

	return nil
}

// ============================================================================
//                              中继管理
// ============================================================================

// Relays 返回已知中继
func (c *RelayClient) Relays() []types.NodeID {
	c.relaysMu.RLock()
	defer c.relaysMu.RUnlock()

	relays := make([]types.NodeID, 0, len(c.relays))
	for id := range c.relays {
		relays = append(relays, id)
	}
	return relays
}

// AddRelay 添加中继
func (c *RelayClient) AddRelay(relay types.NodeID, addrs []endpoint.Address) {
	c.relaysMu.Lock()
	defer c.relaysMu.Unlock()

	c.relays[relay] = &relayInfo{
		nodeID:   relay,
		addrs:    addrs,
		lastSeen: time.Now(),
	}

	log.Debug("添加中继",
		"relay", relay.ShortString())
}

// RemoveRelay 移除中继
func (c *RelayClient) RemoveRelay(relay types.NodeID) {
	c.relaysMu.Lock()
	defer c.relaysMu.Unlock()

	delete(c.relays, relay)

	log.Debug("移除中继",
		"relay", relay.ShortString())
}

// FindRelays 发现中继
//
// 优先级：DHT providers > 本地缓存 > 静态配置
func (c *RelayClient) FindRelays(ctx context.Context) ([]types.NodeID, error) {
	if c.dialer == nil {
		return nil, ErrNoRelaysAvailable
	}

	var result []types.NodeID
	seen := make(map[types.NodeID]struct{})

	// 1. 首先返回已知中继（本地缓存）
	c.relaysMu.RLock()
	for id := range c.relays {
		result = append(result, id)
		seen[id] = struct{}{}
	}
	c.relaysMu.RUnlock()

	// 2. 尝试从发现服务获取更多（包括 DHT providers）
	if c.dialer != nil {
		discovery := c.dialer.Discovery()
		if discovery != nil {
			// 直接调用 DiscoverPeers（接口已扩展支持）
			ch, err := discovery.DiscoverPeers(ctx, RelayNamespace)
			if err == nil && ch != nil {
				// 设置一个短超时来收集结果
				collectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)

			collectLoop:
				for {
					select {
					case <-collectCtx.Done():
						break collectLoop
					case peer, ok := <-ch:
						if !ok {
							break collectLoop
						}
						if _, exists := seen[peer.ID]; !exists {
							result = append(result, peer.ID)
							seen[peer.ID] = struct{}{}

							// 同时添加到已知中继缓存
							addrs := make([]endpoint.Address, len(peer.Addrs))
							for i, addr := range peer.Addrs {
								addrs[i] = address.NewAddr(types.Multiaddr(addr))
							}
							c.AddRelay(peer.ID, addrs)
						}
					}
				}
				cancel()
			}
		}
	}

	if len(result) == 0 {
		return nil, ErrNoRelaysAvailable
	}

	log.Debug("发现中继",
		"count", len(result),
		"from_cache", len(c.relays))

	return result, nil
}

// Close 关闭客户端
func (c *RelayClient) Close() error {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return nil
	}

	log.Info("关闭中继客户端")
	return nil
}

// ============================================================================
//                              reservationHandle 实现
// ============================================================================

type reservationHandle struct {
	client *RelayClient
	relay  types.NodeID
	res    *reservation
	addrs  []endpoint.Address
}

func (h *reservationHandle) Relay() types.NodeID {
	return h.relay
}

func (h *reservationHandle) Addrs() []endpoint.Address {
	if h.addrs != nil {
		return h.addrs
	}
	// 返回中继地址
	return []endpoint.Address{&relayAddr{relay: h.relay, target: h.client.dialer.ID()}}
}

func (h *reservationHandle) Expiry() time.Time {
	return h.res.expires
}

func (h *reservationHandle) Refresh(ctx context.Context) error {
	// 重新预留
	newRes, err := h.client.Reserve(ctx, h.relay)
	if err != nil {
		return err
	}

	// 更新
	if nr, ok := newRes.(*reservationHandle); ok {
		h.res = nr.res
	}

	return nil
}

func (h *reservationHandle) Cancel() error {
	h.client.reservationsMu.Lock()
	delete(h.client.reservations, h.relay)
	h.client.reservationsMu.Unlock()
	return nil
}

// ============================================================================
//                              relayedConn 实现
// ============================================================================

type relayedConn struct {
	stream   relayif.Stream // 使用 relayif.Stream 最小接口
	relay    types.NodeID
	dest     types.NodeID
	localID  types.NodeID
	remoteID types.NodeID
}

func (c *relayedConn) Read(p []byte) (int, error) {
	return c.stream.Read(p)
}

func (c *relayedConn) Write(p []byte) (int, error) {
	return c.stream.Write(p)
}

func (c *relayedConn) Close() error {
	return c.stream.Close()
}

func (c *relayedConn) LocalAddr() endpoint.Address {
	return &relayAddr{relay: c.relay, target: c.localID}
}

func (c *relayedConn) RemoteAddr() endpoint.Address {
	return &relayAddr{relay: c.relay, target: c.remoteID}
}

func (c *relayedConn) LocalNetAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4zero, Port: 0}
}

func (c *relayedConn) RemoteNetAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4zero, Port: 0}
}

func (c *relayedConn) SetDeadline(t time.Time) error {
	return c.stream.SetDeadline(t)
}

func (c *relayedConn) SetReadDeadline(t time.Time) error {
	return c.stream.SetReadDeadline(t)
}

func (c *relayedConn) SetWriteDeadline(t time.Time) error {
	return c.stream.SetWriteDeadline(t)
}

func (c *relayedConn) IsClosed() bool {
	return c.stream.IsClosed()
}

func (c *relayedConn) Transport() string {
	return "relay"
}

// ============================================================================
//                              relayAddr 实现
// ============================================================================

type relayAddr struct {
	relay  types.NodeID
	target types.NodeID
}

func (a *relayAddr) Network() string { return "relay" }
func (a *relayAddr) String() string {
	// Layer1 修复：使用完整 NodeID，避免截断导致的碰撞和不可拨号问题
	return string(types.BuildSimpleRelayCircuit(a.relay, a.target))
}
func (a *relayAddr) Bytes() []byte { return []byte(a.String()) }
func (a *relayAddr) Equal(other endpoint.Address) bool {
	if other == nil {
		return false
	}
	return a.String() == other.String()
}

// relayAddr 不含 IP，无法可靠判断公网/私网/回环，保守处理为 unknown（全部 false）
func (a *relayAddr) IsPublic() bool    { return false }
func (a *relayAddr) IsPrivate() bool   { return false }
func (a *relayAddr) IsLoopback() bool  { return false }
func (a *relayAddr) Multiaddr() string { return a.String() } // relay 地址本身已是 multiaddr 格式
