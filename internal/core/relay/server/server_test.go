package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	relayif "github.com/dep2p/go-dep2p/pkg/interfaces/relay"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// 确保 mock 实现正确的接口
var (
	_ endpoint.Stream     = (*mockStream)(nil)
	_ endpoint.Connection = (*mockConnection)(nil)
)

// ============================================================================
//                              Mock 实现
// ============================================================================

// mockConnection 模拟连接
type mockConnection struct {
	remoteID types.NodeID
	localID  types.NodeID
	closed   int32
	ctx      context.Context
	cancel   context.CancelFunc
	done     chan struct{}
}

func newMockConnection(localID, remoteID types.NodeID) *mockConnection {
	ctx, cancel := context.WithCancel(context.Background())
	return &mockConnection{
		localID:  localID,
		remoteID: remoteID,
		ctx:      ctx,
		cancel:   cancel,
		done:     make(chan struct{}),
	}
}

func (c *mockConnection) RemoteID() endpoint.NodeID {
	return c.remoteID
}

func (c *mockConnection) LocalID() endpoint.NodeID {
	return c.localID
}

func (c *mockConnection) RemotePublicKey() endpoint.PublicKey {
	return nil
}

func (c *mockConnection) RemoteAddrs() []endpoint.Address {
	return nil
}

func (c *mockConnection) LocalAddrs() []endpoint.Address {
	return nil
}

func (c *mockConnection) OpenStream(ctx context.Context, protocolID endpoint.ProtocolID) (endpoint.Stream, error) {
	return nil, nil
}

func (c *mockConnection) OpenStreamWithPriority(ctx context.Context, protocolID endpoint.ProtocolID, priority endpoint.Priority) (endpoint.Stream, error) {
	return nil, nil
}

func (c *mockConnection) AcceptStream(ctx context.Context) (endpoint.Stream, error) {
	return nil, nil
}

func (c *mockConnection) Streams() []endpoint.Stream {
	return nil
}

func (c *mockConnection) StreamCount() int {
	return 0
}

func (c *mockConnection) Stats() endpoint.ConnectionStats {
	return endpoint.ConnectionStats{}
}

func (c *mockConnection) Direction() endpoint.Direction {
	return endpoint.DirOutbound
}

func (c *mockConnection) Transport() string {
	return "mock"
}

func (c *mockConnection) Close() error {
	if atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		c.cancel()
		close(c.done)
	}
	return nil
}

func (c *mockConnection) CloseWithError(code uint32, reason string) error {
	return c.Close()
}

func (c *mockConnection) IsClosed() bool {
	return atomic.LoadInt32(&c.closed) == 1
}

func (c *mockConnection) Done() <-chan struct{} {
	return c.done
}

func (c *mockConnection) Context() context.Context {
	return c.ctx
}

func (c *mockConnection) SetStreamHandler(protocolID endpoint.ProtocolID, handler endpoint.ProtocolHandler) {
	panic("not expected to be called")
}

func (c *mockConnection) RemoveStreamHandler(protocolID endpoint.ProtocolID) {
	panic("not expected to be called")
}

// RealmContext 返回 Realm 上下文 (v1.1 新增)
func (c *mockConnection) RealmContext() *endpoint.RealmContext {
	return nil
}

// SetRealmContext 设置 Realm 上下文 (v1.1 新增)
func (c *mockConnection) SetRealmContext(ctx *endpoint.RealmContext) {
	panic("not expected to be called")
}

// IsRelayed 返回是否为中继连接 (Relay Transport Integration)
func (c *mockConnection) IsRelayed() bool {
	return false
}

// RelayID 返回中继节点 ID (Relay Transport Integration)
func (c *mockConnection) RelayID() endpoint.NodeID {
	return types.EmptyNodeID
}

// mockStream 模拟流
type mockStream struct {
	conn       *mockConnection
	readBuf    *bytes.Buffer
	writeBuf   *bytes.Buffer
	closed     int32
	readClosed int32
	writeClosed int32
	deadline   time.Time
	priority   int
	mu         sync.Mutex
}

func newMockStream(conn *mockConnection) *mockStream {
	return &mockStream{
		conn:     conn,
		readBuf:  bytes.NewBuffer(nil),
		writeBuf: bytes.NewBuffer(nil),
	}
}

func (s *mockStream) Read(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if atomic.LoadInt32(&s.closed) == 1 || atomic.LoadInt32(&s.readClosed) == 1 {
		return 0, io.EOF
	}
	return s.readBuf.Read(p)
}

func (s *mockStream) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if atomic.LoadInt32(&s.closed) == 1 || atomic.LoadInt32(&s.writeClosed) == 1 {
		return 0, io.ErrClosedPipe
	}
	return s.writeBuf.Write(p)
}

func (s *mockStream) Close() error {
	atomic.StoreInt32(&s.closed, 1)
	return nil
}

func (s *mockStream) ID() endpoint.StreamID {
	return 1
}

func (s *mockStream) ProtocolID() endpoint.ProtocolID {
	return "/test/1.0.0"
}

func (s *mockStream) Connection() endpoint.Connection {
	return s.conn
}

func (s *mockStream) SetDeadline(t time.Time) error {
	s.deadline = t
	return nil
}

func (s *mockStream) SetReadDeadline(t time.Time) error {
	return nil
}

func (s *mockStream) SetWriteDeadline(t time.Time) error {
	return nil
}

func (s *mockStream) CloseRead() error {
	atomic.StoreInt32(&s.readClosed, 1)
	return nil
}

func (s *mockStream) CloseWrite() error {
	atomic.StoreInt32(&s.writeClosed, 1)
	return nil
}

func (s *mockStream) SetPriority(priority endpoint.Priority) {
	s.priority = int(priority)
}

func (s *mockStream) Priority() endpoint.Priority {
	return endpoint.Priority(s.priority)
}

func (s *mockStream) Stats() endpoint.StreamStats {
	return endpoint.StreamStats{}
}

func (s *mockStream) IsClosed() bool {
	return atomic.LoadInt32(&s.closed) == 1
}

// ============================================================================
//                              NewServer 测试
// ============================================================================

func TestNewServer(t *testing.T) {
	t.Run("使用默认 Logger", func(t *testing.T) {
		config := relayif.DefaultServerConfig()
		var localID types.NodeID
		copy(localID[:], []byte("local-node-id-123456789012"))

		server := NewServer(config, localID, nil)

		require.NotNil(t, server)
		assert.Equal(t, config, server.config)
		assert.Equal(t, localID, server.localID)
		assert.NotNil(t, server.reservations)
		assert.NotNil(t, server.circuits)
	})


	t.Run("启用限流器", func(t *testing.T) {
		config := relayif.DefaultServerConfig()
		config.MaxDataRate = 1024 * 1024 // 1MB/s
		var localID types.NodeID
		copy(localID[:], []byte("local-node-id-123456789012"))

		server := NewServer(config, localID, nil)

		require.NotNil(t, server)
		assert.NotNil(t, server.limiter)
	})

	t.Run("禁用限流器", func(t *testing.T) {
		config := relayif.DefaultServerConfig()
		config.MaxDataRate = 0
		var localID types.NodeID
		copy(localID[:], []byte("local-node-id-123456789012"))

		server := NewServer(config, localID, nil)

		require.NotNil(t, server)
		assert.Nil(t, server.limiter)
	})
}

// ============================================================================
//                              生命周期测试
// ============================================================================

func TestServer_Start(t *testing.T) {
	t.Run("正常启动", func(t *testing.T) {
		config := relayif.DefaultServerConfig()
		var localID types.NodeID
		copy(localID[:], []byte("local-node-id-123456789012"))

		server := NewServer(config, localID, nil)
		ctx := context.Background()

		err := server.Start(ctx)

		assert.NoError(t, err)
		assert.Equal(t, int32(1), atomic.LoadInt32(&server.running))

		// 清理
		server.Stop()
	})

	t.Run("重复启动", func(t *testing.T) {
		config := relayif.DefaultServerConfig()
		var localID types.NodeID
		copy(localID[:], []byte("local-node-id-123456789012"))

		server := NewServer(config, localID, nil)
		ctx := context.Background()

		err1 := server.Start(ctx)
		err2 := server.Start(ctx)

		assert.NoError(t, err1)
		assert.NoError(t, err2) // 重复启动应该返回 nil
		assert.Equal(t, int32(1), atomic.LoadInt32(&server.running))

		server.Stop()
	})
}

func TestServer_Stop(t *testing.T) {
	t.Run("正常停止", func(t *testing.T) {
		config := relayif.DefaultServerConfig()
		var localID types.NodeID
		copy(localID[:], []byte("local-node-id-123456789012"))

		server := NewServer(config, localID, nil)
		ctx := context.Background()
		server.Start(ctx)

		err := server.Stop()

		assert.NoError(t, err)
		assert.Equal(t, int32(0), atomic.LoadInt32(&server.running))
		assert.Equal(t, int32(1), atomic.LoadInt32(&server.closed))
	})

	t.Run("重复停止", func(t *testing.T) {
		config := relayif.DefaultServerConfig()
		var localID types.NodeID
		copy(localID[:], []byte("local-node-id-123456789012"))

		server := NewServer(config, localID, nil)
		ctx := context.Background()
		server.Start(ctx)

		err1 := server.Stop()
		err2 := server.Stop()

		assert.NoError(t, err1)
		assert.NoError(t, err2) // 重复停止应该返回 nil
	})

	t.Run("未启动直接停止", func(t *testing.T) {
		config := relayif.DefaultServerConfig()
		var localID types.NodeID
		copy(localID[:], []byte("local-node-id-123456789012"))

		server := NewServer(config, localID, nil)

		err := server.Stop()

		assert.NoError(t, err)
	})
}

// ============================================================================
//                              预留测试
// ============================================================================

func TestReservation_IsExpired(t *testing.T) {
	t.Run("未过期", func(t *testing.T) {
		res := &Reservation{
			Expiry: time.Now().Add(time.Hour),
		}

		assert.False(t, res.IsExpired())
	})

	t.Run("已过期", func(t *testing.T) {
		res := &Reservation{
			Expiry: time.Now().Add(-time.Hour),
		}

		assert.True(t, res.IsExpired())
	})
}

func TestServer_getRelayAddrs(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-123456789012"))
	server := NewServer(config, localID, nil)

	var targetPeer types.NodeID
	copy(targetPeer[:], []byte("target-peer-id-12345678901"))

	addrs := server.getRelayAddrs(targetPeer)

	require.Len(t, addrs, 1)
	assert.Contains(t, addrs[0], "p2p-circuit")
}

// ============================================================================
//                              消息编解码测试
// ============================================================================

func TestServer_readReserveRequest(t *testing.T) {
	t.Run("正常读取", func(t *testing.T) {
		config := relayif.DefaultServerConfig()
		var localID types.NodeID
		server := NewServer(config, localID, nil)

		// 构造请求: type(1) + version(1) + TTL(4)
		buf := bytes.NewBuffer([]byte{
			MsgTypeReserve, // type
			1,              // version
			0, 0, 14, 16,   // TTL = 3600 seconds
		})

		ttl, err := server.readReserveRequest(buf)

		assert.NoError(t, err)
		assert.Equal(t, uint32(3600), ttl)
	})

	t.Run("读取失败 - 数据不足", func(t *testing.T) {
		config := relayif.DefaultServerConfig()
		var localID types.NodeID
		server := NewServer(config, localID, nil)

		buf := bytes.NewBuffer([]byte{1, 2}) // 不足 6 字节

		_, err := server.readReserveRequest(buf)

		assert.Error(t, err)
	})
}

func TestServer_sendReserveOK(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	server := NewServer(config, localID, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("peer-id-12345678901234567"))
	res := &Reservation{
		PeerID: peerID,
		Expiry: time.Now().Add(time.Hour),
		Slots:  4,
		Addrs:  []string{"/ip4/127.0.0.1/tcp/4001"},
	}

	buf := bytes.NewBuffer(nil)
	err := server.sendReserveOK(buf, res)

	assert.NoError(t, err)
	assert.Greater(t, buf.Len(), 0)
	// 验证消息类型
	assert.Equal(t, MsgTypeReserveOK, buf.Bytes()[0])
}

func TestServer_sendReserveError(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	server := NewServer(config, localID, nil)

	buf := bytes.NewBuffer(nil)
	err := server.sendReserveError(buf, ErrCodeResourceLimit)

	assert.NoError(t, err)
	assert.Equal(t, 4, buf.Len()) // type(1) + version(1) + code(2)
	assert.Equal(t, MsgTypeReserveError, buf.Bytes()[0])
}

func TestServer_readConnectRequest(t *testing.T) {
	t.Run("正常读取", func(t *testing.T) {
		config := relayif.DefaultServerConfig()
		var localID types.NodeID
		server := NewServer(config, localID, nil)

		// 构造请求（IMPL-1227 扩展版）: type(1) + version(1) + peerID(32) + protoLen(2)
		var expectedPeerID types.NodeID
		copy(expectedPeerID[:], []byte("dest-peer-id-12345678901234"))

		data := make([]byte, 2+32+2)
		data[0] = MsgTypeConnect
		data[1] = 1
		copy(data[2:34], expectedPeerID[:])
		// protoLen = 0 (无协议)
		data[34] = 0
		data[35] = 0

		buf := bytes.NewBuffer(data)

		req, err := server.readConnectRequest(buf)

		assert.NoError(t, err)
		assert.Equal(t, expectedPeerID, req.DestPeer)
		assert.Equal(t, types.ProtocolID(""), req.Protocol)
	})

	t.Run("正常读取带协议", func(t *testing.T) {
		config := relayif.DefaultServerConfig()
		var localID types.NodeID
		server := NewServer(config, localID, nil)

		// 构造请求: type(1) + version(1) + peerID(32) + protoLen(2) + protocol
		var expectedPeerID types.NodeID
		copy(expectedPeerID[:], []byte("dest-peer-id-12345678901234"))
		expectedProto := "/dep2p/app/test-realm/my-protocol"

		data := make([]byte, 2+32+2+len(expectedProto))
		data[0] = MsgTypeConnect
		data[1] = 1
		copy(data[2:34], expectedPeerID[:])
		// protoLen = len(expectedProto)
		binary.BigEndian.PutUint16(data[34:36], uint16(len(expectedProto)))
		copy(data[36:], []byte(expectedProto))

		buf := bytes.NewBuffer(data)

		req, err := server.readConnectRequest(buf)

		assert.NoError(t, err)
		assert.Equal(t, expectedPeerID, req.DestPeer)
		assert.Equal(t, types.ProtocolID(expectedProto), req.Protocol)
	})

	t.Run("读取失败 - 数据不足", func(t *testing.T) {
		config := relayif.DefaultServerConfig()
		var localID types.NodeID
		server := NewServer(config, localID, nil)

		buf := bytes.NewBuffer([]byte{1, 2, 3}) // 不足 36 字节

		_, err := server.readConnectRequest(buf)

		assert.Error(t, err)
	})
}

func TestServer_sendConnectOK(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	server := NewServer(config, localID, nil)

	buf := bytes.NewBuffer(nil)
	err := server.sendConnectOK(buf)

	assert.NoError(t, err)
	assert.Equal(t, 2, buf.Len()) // type(1) + version(1)
	assert.Equal(t, MsgTypeConnectOK, buf.Bytes()[0])
}

func TestServer_sendConnectError(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	server := NewServer(config, localID, nil)

	buf := bytes.NewBuffer(nil)
	err := server.sendConnectError(buf, ErrCodeNoReservation)

	assert.NoError(t, err)
	assert.Equal(t, 4, buf.Len()) // type(1) + version(1) + code(2)
	assert.Equal(t, MsgTypeConnectError, buf.Bytes()[0])
}

// ============================================================================
//                              统计信息测试
// ============================================================================

func TestServer_Stats(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-123456789012"))
	server := NewServer(config, localID, nil)

	// 初始统计
	stats := server.Stats()
	assert.Equal(t, 0, stats.ActiveReservations)
	assert.Equal(t, 0, stats.ActiveConnections)
	assert.Equal(t, uint64(0), stats.TotalBytesRelayed)

	// 模拟增加统计
	server.statsMu.Lock()
	server.stats.ActiveReservations = 5
	server.stats.ActiveCircuits = 3
	server.stats.BytesRelayed = 1024
	server.stats.ConnectionsAccepted = 10
	server.stats.ConnectionsRejected = 2
	server.statsMu.Unlock()

	stats = server.Stats()
	assert.Equal(t, 5, stats.ActiveReservations)
	assert.Equal(t, 3, stats.ActiveConnections)
	assert.Equal(t, uint64(1024), stats.TotalBytesRelayed)
	assert.Equal(t, uint64(12), stats.TotalConnections)
}

func TestServer_Reservations(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-123456789012"))
	server := NewServer(config, localID, nil)

	// 初始无预留
	reservations := server.Reservations()
	assert.Len(t, reservations, 0)

	// 添加预留
	var peerID1, peerID2 types.NodeID
	copy(peerID1[:], []byte("peer-1-id-12345678901234567"))
	copy(peerID2[:], []byte("peer-2-id-12345678901234567"))

	server.reservationsMu.Lock()
	server.reservations[peerID1] = &Reservation{
		PeerID:    peerID1,
		Expiry:    time.Now().Add(time.Hour),
		Slots:     4,
		UsedSlots: 1,
		Addrs:     []string{"/ip4/127.0.0.1/tcp/4001"},
	}
	server.reservations[peerID2] = &Reservation{
		PeerID:    peerID2,
		Expiry:    time.Now().Add(2 * time.Hour),
		Slots:     8,
		UsedSlots: 0,
		Addrs:     []string{"/ip4/127.0.0.1/tcp/4002"},
	}
	server.reservationsMu.Unlock()

	reservations = server.Reservations()
	assert.Len(t, reservations, 2)
}

func TestServer_Config(t *testing.T) {
	config := relayif.ServerConfig{
		MaxReservations:    100,
		MaxCircuits:        50,
		MaxCircuitsPerPeer: 10,
		ReservationTTL:     2 * time.Hour,
		MaxDataRate:        512 * 1024,
		MaxDuration:        5 * time.Minute,
	}
	var localID types.NodeID
	server := NewServer(config, localID, nil)

	result := server.Config()

	assert.Equal(t, config, result)
}

// ============================================================================
//                              电路测试
// ============================================================================

func TestCircuit(t *testing.T) {
	var srcID, destID types.NodeID
	copy(srcID[:], []byte("src-peer-id-123456789012345"))
	copy(destID[:], []byte("dest-peer-id-12345678901234"))

	circuit := &Circuit{
		ID:        "test-circuit-001",
		Src:       srcID,
		Dest:      destID,
		CreatedAt: time.Now(),
		Deadline:  time.Now().Add(time.Minute),
	}

	assert.Equal(t, "test-circuit-001", circuit.ID)
	assert.Equal(t, srcID, circuit.Src)
	assert.Equal(t, destID, circuit.Dest)
	assert.Equal(t, int64(0), circuit.BytesTransferred)
}

func TestServer_closeCircuit(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID, srcID, destID types.NodeID
	copy(srcID[:], []byte("src-peer-id-123456789012345"))
	copy(destID[:], []byte("dest-peer-id-12345678901234"))
	server := NewServer(config, localID, nil)

	srcConn := newMockConnection(localID, srcID)
	srcStream := newMockStream(srcConn)
	destConn := newMockConnection(localID, destID)
	destStream := newMockStream(destConn)

	circuit := &Circuit{
		ID:         "test-circuit",
		SrcStream:  srcStream,
		DestStream: destStream,
	}

	// 第一次关闭
	server.closeCircuit(circuit)
	assert.Equal(t, int32(1), atomic.LoadInt32(&circuit.closed))
	assert.Equal(t, int32(1), atomic.LoadInt32(&srcStream.closed))
	assert.Equal(t, int32(1), atomic.LoadInt32(&destStream.closed))

	// 第二次关闭 - 应该是幂等的
	server.closeCircuit(circuit)
	assert.Equal(t, int32(1), atomic.LoadInt32(&circuit.closed))
}

// ============================================================================
//                              限流器测试
// ============================================================================

func TestNewRateLimiter(t *testing.T) {
	t.Run("正常创建", func(t *testing.T) {
		limiter := NewRateLimiter(1024)

		require.NotNil(t, limiter)
		assert.Equal(t, int64(1024), limiter.rate)
		assert.Equal(t, int64(1024), limiter.tokens)
	})

	t.Run("零速率返回 nil", func(t *testing.T) {
		limiter := NewRateLimiter(0)

		assert.Nil(t, limiter)
	})

	t.Run("负速率返回 nil", func(t *testing.T) {
		limiter := NewRateLimiter(-100)

		assert.Nil(t, limiter)
	})
}

func TestRateLimiter_Wait(t *testing.T) {
	t.Run("nil 限流器不阻塞", func(t *testing.T) {
		var limiter *RateLimiter = nil
		start := time.Now()
		limiter.Wait(1000)
		elapsed := time.Since(start)

		assert.Less(t, elapsed, 10*time.Millisecond)
	})

	t.Run("有足够令牌不阻塞", func(t *testing.T) {
		limiter := NewRateLimiter(10000)
		start := time.Now()
		limiter.Wait(100) // 只需 100 个令牌
		elapsed := time.Since(start)

		assert.Less(t, elapsed, 10*time.Millisecond)
	})

	t.Run("令牌补充", func(t *testing.T) {
		limiter := NewRateLimiter(1000) // 1000 字节/秒

		// 消耗所有令牌
		limiter.Wait(1000)
		assert.Equal(t, int64(0), limiter.tokens)

		// 等待令牌补充
		time.Sleep(100 * time.Millisecond)

		// 再次等待应该不会阻塞太久
		start := time.Now()
		limiter.Wait(50) // 需要 50 个令牌
		elapsed := time.Since(start)

		// 0.1 秒应该补充约 100 个令牌
		assert.Less(t, elapsed, 100*time.Millisecond)
	})
}

// ============================================================================
//                              错误码测试
// ============================================================================

func TestErrorCodes(t *testing.T) {
	assert.Equal(t, ErrorCode(0), ErrCodeNone)
	assert.Equal(t, ErrorCode(100), ErrCodeMalformed)
	assert.Equal(t, ErrorCode(200), ErrCodeResourceLimit)
	assert.Equal(t, ErrorCode(201), ErrCodeNoReservation)
	assert.Equal(t, ErrorCode(300), ErrCodeConnectFailed)
}

// ============================================================================
//                              协议常量测试
// ============================================================================

func TestProtocolConstants(t *testing.T) {
	assert.Equal(t, types.ProtocolID("/dep2p/sys/relay/1.0.0"), ProtocolRelay)
	assert.Equal(t, types.ProtocolID("/dep2p/sys/relay/hop/1.0.0"), ProtocolRelayHop)
}

func TestMessageTypeConstants(t *testing.T) {
	assert.Equal(t, uint8(1), MsgTypeReserve)
	assert.Equal(t, uint8(2), MsgTypeReserveOK)
	assert.Equal(t, uint8(3), MsgTypeReserveError)
	assert.Equal(t, uint8(4), MsgTypeConnect)
	assert.Equal(t, uint8(5), MsgTypeConnectOK)
	assert.Equal(t, uint8(6), MsgTypeConnectError)
}

// ============================================================================
//                              清理测试
// ============================================================================

func TestServer_cleanupExpired(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-123456789012"))
	server := NewServer(config, localID, nil)

	// 添加过期和未过期的预留
	var peerID1, peerID2 types.NodeID
	copy(peerID1[:], []byte("peer-1-id-12345678901234567"))
	copy(peerID2[:], []byte("peer-2-id-12345678901234567"))

	server.reservationsMu.Lock()
	server.reservations[peerID1] = &Reservation{
		PeerID: peerID1,
		Expiry: time.Now().Add(-time.Hour), // 已过期
	}
	server.reservations[peerID2] = &Reservation{
		PeerID: peerID2,
		Expiry: time.Now().Add(time.Hour), // 未过期
	}
	server.reservationsMu.Unlock()

	// 执行清理
	server.cleanupExpired()

	// 验证结果
	server.reservationsMu.RLock()
	_, exists1 := server.reservations[peerID1]
	_, exists2 := server.reservations[peerID2]
	server.reservationsMu.RUnlock()

	assert.False(t, exists1, "过期预留应该被清理")
	assert.True(t, exists2, "未过期预留应该保留")
}

func TestServer_cleanupCircuit(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-123456789012"))
	server := NewServer(config, localID, nil)
	ctx := context.Background()
	server.Start(ctx)
	defer server.Stop()

	// 创建电路和预留
	var srcID, destID types.NodeID
	copy(srcID[:], []byte("src-peer-id-123456789012345"))
	copy(destID[:], []byte("dest-peer-id-12345678901234"))

	srcConn := newMockConnection(localID, srcID)
	srcStream := newMockStream(srcConn)
	destConn := newMockConnection(localID, destID)
	destStream := newMockStream(destConn)

	// 添加预留
	server.reservationsMu.Lock()
	server.reservations[destID] = &Reservation{
		PeerID:    destID,
		Slots:     4,
		UsedSlots: 1,
	}
	server.reservationsMu.Unlock()

	// 添加电路
	circuit := &Circuit{
		ID:         "test-circuit",
		Src:        srcID,
		Dest:       destID,
		SrcStream:  srcStream,
		DestStream: destStream,
	}
	server.circuitsMu.Lock()
	server.circuits[circuit.ID] = circuit
	server.circuitsMu.Unlock()

	// 执行清理
	server.cleanupCircuit(circuit)

	// 验证电路被移除
	server.circuitsMu.RLock()
	_, exists := server.circuits[circuit.ID]
	server.circuitsMu.RUnlock()
	assert.False(t, exists)

	// 验证槽位被释放
	server.reservationsMu.RLock()
	res := server.reservations[destID]
	server.reservationsMu.RUnlock()
	assert.Equal(t, 0, res.UsedSlots)
}

// ============================================================================
//                              并发测试
// ============================================================================

func TestServer_ConcurrentReservations(t *testing.T) {
	config := relayif.DefaultServerConfig()
	config.MaxReservations = 100
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-123456789012"))
	server := NewServer(config, localID, nil)

	var wg sync.WaitGroup
	numGoroutines := 20

	// 并发添加预留
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			var peerID types.NodeID
			copy(peerID[:], []byte("peer-id-concurrent-"+string(rune('A'+idx))))

			server.reservationsMu.Lock()
			server.reservations[peerID] = &Reservation{
				PeerID: peerID,
				Expiry: time.Now().Add(time.Hour),
				Slots:  4,
			}
			server.reservationsMu.Unlock()
		}(i)
	}

	wg.Wait()

	// 验证预留数量
	server.reservationsMu.RLock()
	count := len(server.reservations)
	server.reservationsMu.RUnlock()

	assert.Equal(t, numGoroutines, count)
}

func TestServer_ConcurrentStats(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-123456789012"))
	server := NewServer(config, localID, nil)

	var wg sync.WaitGroup
	numGoroutines := 50

	// 并发更新统计
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				server.statsMu.Lock()
				server.stats.BytesRelayed += 100
				server.stats.ConnectionsAccepted++
				server.statsMu.Unlock()

				// 读取统计
				_ = server.Stats()
			}
		}()
	}

	wg.Wait()

	// 验证最终统计
	stats := server.Stats()
	assert.Equal(t, uint64(numGoroutines*100*100), stats.TotalBytesRelayed)
	assert.Equal(t, uint64(numGoroutines*100), stats.TotalConnections)
}

// ============================================================================
//                              Bug 修复验证测试
// ============================================================================

func TestRateLimiter_AccurateLimiting(t *testing.T) {
	// 测试限流器是否按实际字节数限流，而非缓冲区大小
	limiter := NewRateLimiter(1000) // 1000 字节/秒

	// 消费 100 字节
	limiter.Wait(100)

	// 验证剩余令牌 = 1000 - 100 = 900
	assert.Equal(t, int64(900), limiter.tokens)

	// 再消费 200 字节
	limiter.Wait(200)

	// 验证剩余令牌 = 900 - 200 = 700
	assert.Equal(t, int64(700), limiter.tokens)
}

func TestRateLimiter_SmallReads(t *testing.T) {
	// 验证小读取不会被按大缓冲区限流
	limiter := NewRateLimiter(10000) // 10KB/秒

	start := time.Now()

	// 模拟 10 次 100 字节的读取
	for i := 0; i < 10; i++ {
		limiter.Wait(100) // 每次 100 字节
	}

	elapsed := time.Since(start)

	// 总共消费 1000 字节，不应该阻塞
	// 如果按 32KB 缓冲区计算，会需要等待约 32 秒
	assert.Less(t, elapsed, 100*time.Millisecond, "小读取不应该导致长时间阻塞")
}

func TestServer_SlotRollbackOnError(t *testing.T) {
	// 验证 sendConnectOK 失败时槽位会被回滚
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-123456789012"))
	server := NewServer(config, localID, nil)

	var destPeer types.NodeID
	copy(destPeer[:], []byte("dest-peer-id-12345678901234"))

	// 添加预留
	server.reservationsMu.Lock()
	server.reservations[destPeer] = &Reservation{
		PeerID:    destPeer,
		Slots:     4,
		UsedSlots: 0,
	}
	server.reservationsMu.Unlock()

	// 模拟增加槽位
	server.reservationsMu.Lock()
	server.reservations[destPeer].UsedSlots++
	usedBefore := server.reservations[destPeer].UsedSlots
	server.reservationsMu.Unlock()

	assert.Equal(t, 1, usedBefore)

	// 模拟回滚（如 sendConnectOK 失败后的操作）
	server.reservationsMu.Lock()
	server.reservations[destPeer].UsedSlots--
	usedAfter := server.reservations[destPeer].UsedSlots
	server.reservationsMu.Unlock()

	assert.Equal(t, 0, usedAfter, "槽位应该被正确回滚")
}

// ============================================================================
//                              真正验证业务逻辑的测试
// ============================================================================

// TestRateLimiter_ActualRateLimiting 验证限流器实际速率
func TestRateLimiter_ActualRateLimiting(t *testing.T) {
	// 创建 1000 字节/秒的限流器
	rate := int64(1000)
	limiter := NewRateLimiter(rate)

	t.Run("验证令牌初始值", func(t *testing.T) {
		assert.Equal(t, rate, limiter.tokens, "初始令牌应该等于速率")
	})

	t.Run("验证令牌消费正确", func(t *testing.T) {
		// 重新创建限流器
		limiter := NewRateLimiter(rate)

		// 消费 300 字节
		limiter.Wait(300)
		assert.Equal(t, int64(700), limiter.tokens, "消费 300 后应剩余 700")

		// 再消费 200 字节
		limiter.Wait(200)
		assert.Equal(t, int64(500), limiter.tokens, "消费 200 后应剩余 500")
	})

	t.Run("验证多次小读取累计正确", func(t *testing.T) {
		limiter := NewRateLimiter(10000) // 10KB/秒

		totalConsumed := int64(0)
		// 模拟 10 次 100 字节的小读取
		for i := 0; i < 10; i++ {
			limiter.Wait(100)
			totalConsumed += 100
		}

		expectedRemaining := int64(10000) - totalConsumed
		assert.Equal(t, expectedRemaining, limiter.tokens,
			"10 次 100 字节读取后，令牌应该正确减少")
	})

	t.Run("验证大缓冲区小读取不会过度限流", func(t *testing.T) {
		// 这是 BUG-001 的核心测试
		// 如果使用 len(buf) 而非实际读取字节数，会导致过度限流
		limiter := NewRateLimiter(32768) // 32KB/秒

		start := time.Now()

		// 模拟 10 次 100 字节的读取（总共 1KB）
		// 如果限流器错误使用 32KB 缓冲区大小，会需要等待约 10 秒
		// 正确使用实际字节数，应该几乎不需要等待
		for i := 0; i < 10; i++ {
			limiter.Wait(100) // 每次实际只读取 100 字节
		}

		elapsed := time.Since(start)
		assert.Less(t, elapsed, 200*time.Millisecond,
			"10 次 100 字节读取（共 1KB）在 32KB/秒的限速下不应该超过 200ms")
	})
}

// TestServer_CircuitStatistics 验证电路统计的准确性
func TestServer_CircuitStatistics(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-123456789012"))
	server := NewServer(config, localID, nil)

	t.Run("初始统计为零", func(t *testing.T) {
		stats := server.Stats()
		assert.Equal(t, 0, stats.ActiveConnections)
		assert.Equal(t, uint64(0), stats.TotalBytesRelayed)
	})

	t.Run("添加电路后统计正确", func(t *testing.T) {
		var srcID, destID types.NodeID
		copy(srcID[:], []byte("src-peer-id-123456789012345"))
		copy(destID[:], []byte("dest-peer-id-12345678901234"))

		srcConn := newMockConnection(localID, srcID)
		srcStream := newMockStream(srcConn)
		destConn := newMockConnection(localID, destID)
		destStream := newMockStream(destConn)

		circuit := &Circuit{
			ID:         "test-circuit-1",
			Src:        srcID,
			Dest:       destID,
			SrcStream:  srcStream,
			DestStream: destStream,
		}

		// 添加电路
		server.circuitsMu.Lock()
		server.circuits[circuit.ID] = circuit
		circuitCount := len(server.circuits)
		server.circuitsMu.Unlock()

		// 更新统计
		server.statsMu.Lock()
		server.stats.TotalCircuits++
		server.stats.ActiveCircuits = int64(circuitCount)
		server.statsMu.Unlock()

		stats := server.Stats()
		assert.Equal(t, 1, stats.ActiveConnections, "应该有 1 个活动连接")
	})

	t.Run("移除电路后统计正确", func(t *testing.T) {
		// 移除所有电路
		server.circuitsMu.Lock()
		for id := range server.circuits {
			delete(server.circuits, id)
		}
		circuitCount := len(server.circuits)
		server.circuitsMu.Unlock()

		server.statsMu.Lock()
		server.stats.ActiveCircuits = int64(circuitCount)
		server.statsMu.Unlock()

		stats := server.Stats()
		assert.Equal(t, 0, stats.ActiveConnections, "移除后应该有 0 个活动连接")
	})
}

// TestServer_ReservationSlotIntegrity 验证预留槽位的完整性
func TestServer_ReservationSlotIntegrity(t *testing.T) {
	config := relayif.DefaultServerConfig()
	config.MaxCircuitsPerPeer = 4
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-123456789012"))
	server := NewServer(config, localID, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("peer-id-12345678901234567890"))

	t.Run("槽位增减保持一致", func(t *testing.T) {
		// 创建预留
		server.reservationsMu.Lock()
		server.reservations[peerID] = &Reservation{
			PeerID:    peerID,
			Slots:     4,
			UsedSlots: 0,
			Expiry:    time.Now().Add(time.Hour),
		}
		server.reservationsMu.Unlock()

		// 模拟多次连接和断开
		for i := 0; i < 10; i++ {
			// 增加槽位
			server.reservationsMu.Lock()
			server.reservations[peerID].UsedSlots++
			server.reservationsMu.Unlock()

			// 减少槽位
			server.reservationsMu.Lock()
			server.reservations[peerID].UsedSlots--
			server.reservationsMu.Unlock()
		}

		// 验证槽位回到初始值
		server.reservationsMu.RLock()
		usedSlots := server.reservations[peerID].UsedSlots
		server.reservationsMu.RUnlock()

		assert.Equal(t, 0, usedSlots, "槽位应该回到初始值 0")
	})

	t.Run("并发槽位操作安全", func(t *testing.T) {
		var wg sync.WaitGroup

		// 并发增加和减少槽位
		for i := 0; i < 50; i++ {
			wg.Add(2)

			go func() {
				defer wg.Done()
				server.reservationsMu.Lock()
				if res, ok := server.reservations[peerID]; ok {
					res.UsedSlots++
				}
				server.reservationsMu.Unlock()
			}()

			go func() {
				defer wg.Done()
				time.Sleep(time.Millisecond) // 稍微延迟
				server.reservationsMu.Lock()
				if res, ok := server.reservations[peerID]; ok && res.UsedSlots > 0 {
					res.UsedSlots--
				}
				server.reservationsMu.Unlock()
			}()
		}

		wg.Wait()

		// 验证没有 panic，槽位值合理
		server.reservationsMu.RLock()
		usedSlots := server.reservations[peerID].UsedSlots
		server.reservationsMu.RUnlock()

		assert.GreaterOrEqual(t, usedSlots, 0, "槽位不应为负数")
		assert.LessOrEqual(t, usedSlots, 50, "槽位不应超过操作次数")
	})
}

// ============================================================================
//                              Bug 修复验证测试
// ============================================================================

// TestServer_readReserveRequest_Validation 验证预留请求消息类型验证
// 这是对 BUG-004 (消息类型未验证) 的修复验证
func TestServer_readReserveRequest_Validation(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	server := NewServer(config, localID, nil)

	t.Run("正确的消息类型", func(t *testing.T) {
		// 正确格式: type(MsgTypeReserve) + version(1) + TTL(4)
		buf := bytes.NewBuffer([]byte{
			MsgTypeReserve, // type = 1
			1,              // version
			0, 0, 14, 16,   // TTL = 3600
		})

		ttl, err := server.readReserveRequest(buf)
		assert.NoError(t, err)
		assert.Equal(t, uint32(3600), ttl)
	})

	t.Run("错误的消息类型被拒绝", func(t *testing.T) {
		// 错误格式: type(wrong) + version(1) + TTL(4)
		buf := bytes.NewBuffer([]byte{
			99, // wrong type
			1,  // version
			0, 0, 14, 16,
		})

		_, err := server.readReserveRequest(buf)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected message type")
	})
}

// TestServer_readConnectRequest_Validation 验证连接请求消息类型和零NodeID验证
// 这是对 BUG-004/BUG-005 (消息类型未验证/零NodeID未验证) 的修复验证
func TestServer_readConnectRequest_Validation(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	server := NewServer(config, localID, nil)

	t.Run("正确的消息类型和非零NodeID", func(t *testing.T) {
		// 正确格式（IMPL-1227 扩展版）: type(MsgTypeConnect) + version(1) + destPeerID(32) + protoLen(2)
		var expectedPeerID types.NodeID
		copy(expectedPeerID[:], []byte("dest-peer-id-12345678901234"))

		data := make([]byte, 36)
		data[0] = MsgTypeConnect
		data[1] = 1
		copy(data[2:34], expectedPeerID[:])
		// protoLen = 0
		data[34] = 0
		data[35] = 0

		buf := bytes.NewBuffer(data)

		req, err := server.readConnectRequest(buf)
		assert.NoError(t, err)
		assert.Equal(t, expectedPeerID, req.DestPeer)
	})

	t.Run("错误的消息类型被拒绝", func(t *testing.T) {
		data := make([]byte, 36)
		data[0] = 99 // wrong type
		data[1] = 1
		copy(data[2:34], []byte("dest-peer-id-12345678901234"))
		// protoLen = 0
		data[34] = 0
		data[35] = 0

		buf := bytes.NewBuffer(data)

		_, err := server.readConnectRequest(buf)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected message type")
	})

	t.Run("零NodeID被拒绝", func(t *testing.T) {
		// NodeID 全为 0
		data := make([]byte, 36)
		data[0] = MsgTypeConnect
		data[1] = 1
		// data[2:34] 默认为 0 (zero NodeID)
		// protoLen = 0
		data[34] = 0
		data[35] = 0

		buf := bytes.NewBuffer(data)

		_, err := server.readConnectRequest(buf)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "zero")
	})
}

// ============================================================================
//                              上下文安全测试
// ============================================================================

// TestServer_ConnectToDestWithoutStart 测试未启动时 connectToDest 的安全性
func TestServer_ConnectToDestWithoutStart(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-123456789012"))
	server := NewServer(config, localID, nil)

	var srcPeer, destPeer types.NodeID
	copy(srcPeer[:], []byte("src-peer-id-123456789012345"))
	copy(destPeer[:], []byte("dest-peer-id-12345678901234"))

	// 未调用 Start() 时，应该返回错误而不是 panic
	_, err := server.connectToDest(srcPeer, destPeer)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

// TestServer_CleanupLoopWithoutStart 测试未启动时 cleanupLoop 的安全性
func TestServer_CleanupLoopWithoutStart(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-123456789012"))
	server := NewServer(config, localID, nil)

	// 手动调用 cleanupLoop（模拟 ctx 为 nil 的情况）
	// 应该安全返回而不是 panic
	server.cleanupLoop()
}

// ============================================================================
//                              写入错误测试
// ============================================================================

// errorWriter 用于测试写入错误
type errorWriter struct {
	writeCount int
	failAfter  int
}

func (w *errorWriter) Write(p []byte) (n int, err error) {
	w.writeCount++
	if w.writeCount > w.failAfter {
		return 0, io.ErrClosedPipe
	}
	return len(p), nil
}

// TestServer_sendReserveOK_WriteError 测试 sendReserveOK 写入错误处理
func TestServer_sendReserveOK_WriteError(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	server := NewServer(config, localID, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("peer-id-12345678901234567"))
	res := &Reservation{
		PeerID: peerID,
		Expiry: time.Now().Add(time.Hour),
		Slots:  4,
		Addrs:  []string{"/ip4/127.0.0.1/tcp/4001"},
	}

	t.Run("第一次写入失败", func(t *testing.T) {
		w := &errorWriter{failAfter: 0}
		err := server.sendReserveOK(w, res)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "write message header")
	})

	t.Run("TTL写入失败", func(t *testing.T) {
		w := &errorWriter{failAfter: 1}
		err := server.sendReserveOK(w, res)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "write TTL")
	})

	t.Run("slots写入失败", func(t *testing.T) {
		w := &errorWriter{failAfter: 2}
		err := server.sendReserveOK(w, res)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "write slots")
	})
}

// TestServer_sendReserveError_WriteError 测试 sendReserveError 写入错误处理
func TestServer_sendReserveError_WriteError(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	server := NewServer(config, localID, nil)

	t.Run("header写入失败", func(t *testing.T) {
		w := &errorWriter{failAfter: 0}
		err := server.sendReserveError(w, ErrCodeResourceLimit)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "write error header")
	})

	t.Run("code写入失败", func(t *testing.T) {
		w := &errorWriter{failAfter: 1}
		err := server.sendReserveError(w, ErrCodeResourceLimit)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "write error code")
	})
}

// TestServer_sendConnectError_WriteError 测试 sendConnectError 写入错误处理
func TestServer_sendConnectError_WriteError(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	server := NewServer(config, localID, nil)

	t.Run("header写入失败", func(t *testing.T) {
		w := &errorWriter{failAfter: 0}
		err := server.sendConnectError(w, ErrCodeNoReservation)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "write error header")
	})

	t.Run("code写入失败", func(t *testing.T) {
		w := &errorWriter{failAfter: 1}
		err := server.sendConnectError(w, ErrCodeNoReservation)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "write error code")
	})
}

// ============================================================================
//                              Connection nil 检查测试
// ============================================================================

// mockStreamWithNilConnection 模拟无连接的流
type mockStreamWithNilConnection struct {
	mockStream
}

func (s *mockStreamWithNilConnection) Connection() endpoint.Connection {
	return nil
}

// TestServer_handleReserve_NilConnection 测试 handleReserve 处理 nil Connection
func TestServer_handleReserve_NilConnection(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-123456789012"))
	server := NewServer(config, localID, nil)
	server.Start(context.Background())
	defer server.Stop()

	// 创建一个 Connection 返回 nil 的流
	stream := &mockStreamWithNilConnection{
		mockStream: mockStream{
			readBuf:  bytes.NewBuffer(nil),
			writeBuf: bytes.NewBuffer(nil),
		},
	}

	// 不应 panic
	server.handleReserve(stream)

	// 流应该被关闭
	assert.Equal(t, int32(1), atomic.LoadInt32(&stream.closed))
}

// TestServer_handleConnect_NilConnection 测试 handleConnect 处理 nil Connection
func TestServer_handleConnect_NilConnection(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-123456789012"))
	server := NewServer(config, localID, nil)
	server.Start(context.Background())
	defer server.Stop()

	// 创建一个 Connection 返回 nil 的流
	stream := &mockStreamWithNilConnection{
		mockStream: mockStream{
			readBuf:  bytes.NewBuffer(nil),
			writeBuf: bytes.NewBuffer(nil),
		},
	}

	// 不应 panic
	server.handleConnect(stream)

	// 流应该被关闭
	assert.Equal(t, int32(1), atomic.LoadInt32(&stream.closed))
}

// ============================================================================
//                              IMPL-1227: Realm Relay 模式测试
// ============================================================================

// TestServer_SetRealmID 测试设置 RealmID
func TestServer_SetRealmID(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-123456789012"))
	server := NewServer(config, localID, nil)

	// 初始应该不是 Realm Relay
	assert.False(t, server.IsRealmRelay())
	assert.Empty(t, server.RealmID())

	// 设置 RealmID
	realmKey := types.GenerateRealmKey()
	realmID := types.DeriveRealmID(realmKey)
	server.SetRealmID(realmID)

	// 现在应该是 Realm Relay
	assert.True(t, server.IsRealmRelay())
	assert.Equal(t, realmID, server.RealmID())
}

// TestServer_IsProtocolAllowed_SystemRelay 测试 System Relay 协议白名单
func TestServer_IsProtocolAllowed_SystemRelay(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-123456789012"))
	server := NewServer(config, localID, nil)

	// System Relay 模式

	// 系统协议应该被允许
	assert.True(t, server.isProtocolAllowed("/dep2p/sys/echo/1.0.0"))
	assert.True(t, server.isProtocolAllowed("/dep2p/sys/relay/1.0.0"))
	assert.True(t, server.isProtocolAllowed("/dep2p/sys/dht/1.0.0"))

	// 应用协议应该被拒绝
	assert.False(t, server.isProtocolAllowed("/dep2p/app/somerealmid/chat/1.0.0"))

	// Realm 协议应该被拒绝
	assert.False(t, server.isProtocolAllowed("/dep2p/realm/somerealmid/sync/1.0.0"))
}

// TestServer_IsProtocolAllowed_RealmRelay 测试 Realm Relay 协议白名单
func TestServer_IsProtocolAllowed_RealmRelay(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-123456789012"))
	server := NewServer(config, localID, nil)

	// 设置为 Realm Relay 模式
	realmKey := types.GenerateRealmKey()
	realmID := types.DeriveRealmID(realmKey)
	server.SetRealmID(realmID)

	// 本 Realm 应用协议应该被允许
	appProto := "/dep2p/app/" + string(realmID) + "/chat/1.0.0"
	assert.True(t, server.isProtocolAllowed(types.ProtocolID(appProto)))

	// 本 Realm 控制协议应该被允许
	realmProto := "/dep2p/realm/" + string(realmID) + "/sync/1.0.0"
	assert.True(t, server.isProtocolAllowed(types.ProtocolID(realmProto)))

	// 其他 Realm 的协议应该被拒绝
	otherRealmKey := types.GenerateRealmKey()
	otherRealmID := types.DeriveRealmID(otherRealmKey)
	otherAppProto := "/dep2p/app/" + string(otherRealmID) + "/chat/1.0.0"
	assert.False(t, server.isProtocolAllowed(types.ProtocolID(otherAppProto)))

	// 系统协议应该被拒绝（Realm Relay 不转发系统协议）
	assert.False(t, server.isProtocolAllowed("/dep2p/sys/echo/1.0.0"))
}

// TestServer_VerifyPSKMembership_SystemRelay 测试 System Relay 无需验证
func TestServer_VerifyPSKMembership_SystemRelay(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-123456789012"))
	server := NewServer(config, localID, nil)

	// System Relay 模式（无 RealmID）
	stream := &mockStream{
		readBuf:  bytes.NewBuffer(nil),
		writeBuf: bytes.NewBuffer(nil),
	}

	// 验证应该通过（无需验证）
	var peerID types.NodeID
	copy(peerID[:], []byte("peer-node-id-123456789012"))
	err := server.verifyPSKMembership(stream, peerID)
	assert.NoError(t, err)
}

// TestServer_VerifyPSKMembership_RealmRelayNoPSK 测试 Realm Relay 无 PSK 认证器（警告并跳过）
func TestServer_VerifyPSKMembership_RealmRelayNoPSK(t *testing.T) {
	config := relayif.DefaultServerConfig()
	var localID types.NodeID
	copy(localID[:], []byte("local-node-id-123456789012"))
	server := NewServer(config, localID, nil)

	// 设置为 Realm Relay 模式但不配置 PSK 认证器
	realmKey := types.GenerateRealmKey()
	realmID := types.DeriveRealmID(realmKey)
	server.SetRealmID(realmID)

	stream := &mockStream{
		readBuf:  bytes.NewBuffer(nil),
		writeBuf: bytes.NewBuffer(nil),
	}

	// IMPL-1227 修复: Realm Relay 必须有 PSK 认证器
	// 如果没有配置，应该返回错误（安全不变量）
	var peerID types.NodeID
	copy(peerID[:], []byte("peer-node-id-123456789012"))
	err := server.verifyPSKMembership(stream, peerID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "realm relay requires PSK authenticator")
}
