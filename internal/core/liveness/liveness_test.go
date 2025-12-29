package liveness

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/internal/config"
	endpointif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	livenessif "github.com/dep2p/go-dep2p/pkg/interfaces/liveness"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Mock 实现
// ============================================================================

// mockEndpoint 模拟 Endpoint
type mockEndpoint struct {
	handlers      map[types.ProtocolID]endpointif.ProtocolHandler
	connections   map[types.NodeID]*mockConnection
	mu            sync.RWMutex
	onConnect     func(ctx context.Context, id types.NodeID) (endpointif.Connection, error)
	connectCalled int32
}

func newMockEndpoint() *mockEndpoint {
	return &mockEndpoint{
		handlers:    make(map[types.ProtocolID]endpointif.ProtocolHandler),
		connections: make(map[types.NodeID]*mockConnection),
	}
}

func (m *mockEndpoint) SetProtocolHandler(proto types.ProtocolID, handler endpointif.ProtocolHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[proto] = handler
}

func (m *mockEndpoint) RemoveProtocolHandler(proto types.ProtocolID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.handlers, proto)
}

func (m *mockEndpoint) Connection(id types.NodeID) (endpointif.Connection, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	conn, ok := m.connections[id]
	if !ok {
		return nil, false
	}
	return conn, true
}

func (m *mockEndpoint) Connections() []endpointif.Connection {
	m.mu.RLock()
	defer m.mu.RUnlock()
	conns := make([]endpointif.Connection, 0, len(m.connections))
	for _, c := range m.connections {
		conns = append(conns, c)
	}
	return conns
}

func (m *mockEndpoint) Connect(ctx context.Context, id types.NodeID) (endpointif.Connection, error) {
	atomic.AddInt32(&m.connectCalled, 1)
	if m.onConnect != nil {
		return m.onConnect(ctx, id)
	}
	return nil, errors.New("no connection")
}

func (m *mockEndpoint) addConnection(id types.NodeID, conn *mockConnection) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.connections[id] = conn
}

func (m *mockEndpoint) RegisterConnectionCallback(callback func(types.NodeID, bool)) {
	// Mock implementation - do nothing
}

func (m *mockEndpoint) RegisterConnectionEventCallback(callback func(event interface{})) {
	// Mock implementation - do nothing
}

func (m *mockEndpoint) ConnectWithRetry(ctx context.Context, nodeID types.NodeID, config *endpointif.RetryConfig) (endpointif.Connection, error) {
	return m.Connect(ctx, nodeID)
}

// 其他 Endpoint 方法（不需要完整实现）
func (m *mockEndpoint) ID() types.NodeID                 { return types.NodeID{} }
func (m *mockEndpoint) PublicKey() endpointif.PublicKey  { return nil }
func (m *mockEndpoint) Listen(ctx context.Context) error { return nil }
func (m *mockEndpoint) Accept(ctx context.Context) (endpointif.Connection, error) {
	return nil, nil
}
func (m *mockEndpoint) Disconnect(id types.NodeID) error { return nil }
func (m *mockEndpoint) ConnectWithAddrs(ctx context.Context, id types.NodeID, addrs []endpointif.Address) (endpointif.Connection, error) {
	return nil, nil
}
func (m *mockEndpoint) ConnectionCount() int                      { return len(m.connections) }
func (m *mockEndpoint) Protocols() []types.ProtocolID             { return nil }
func (m *mockEndpoint) ListenAddrs() []endpointif.Address         { return nil }
func (m *mockEndpoint) AdvertisedAddrs() []endpointif.Address     { return nil }
func (m *mockEndpoint) VerifiedDirectAddrs() []endpointif.Address { return nil }
func (m *mockEndpoint) AddAdvertisedAddr(addr endpointif.Address) {}
func (m *mockEndpoint) Discovery() endpointif.DiscoveryService    { return nil }
func (m *mockEndpoint) NAT() endpointif.NATService                { return nil }
func (m *mockEndpoint) Relay() endpointif.RelayClient             { return nil }
func (m *mockEndpoint) AddressBook() endpointif.AddressBook       { return nil }
func (m *mockEndpoint) DiagnosticReport() endpointif.DiagnosticReport {
	return endpointif.DiagnosticReport{}
}
func (m *mockEndpoint) Close() error { return nil }

// mockConnection 模拟 Connection
type mockConnection struct {
	remoteID     types.NodeID
	streams      []*mockStream
	openStreamFn func(ctx context.Context, proto types.ProtocolID) (endpointif.Stream, error)
	mu           sync.Mutex
	doneCh       chan struct{}
}

func newMockConnection(id types.NodeID) *mockConnection {
	return &mockConnection{
		remoteID: id,
		doneCh:   make(chan struct{}),
	}
}

func (m *mockConnection) RemoteID() types.NodeID {
	return m.remoteID
}

func (m *mockConnection) OpenStream(ctx context.Context, proto types.ProtocolID) (endpointif.Stream, error) {
	if m.openStreamFn != nil {
		return m.openStreamFn(ctx, proto)
	}
	return nil, errors.New("no stream")
}

func (m *mockConnection) OpenStreamWithPriority(ctx context.Context, proto types.ProtocolID, priority endpointif.Priority) (endpointif.Stream, error) {
	return m.OpenStream(ctx, proto)
}

func (m *mockConnection) AcceptStream(ctx context.Context) (endpointif.Stream, error) {
	return nil, nil
}

func (m *mockConnection) Close() error                                    { return nil }
func (m *mockConnection) CloseWithError(code uint32, reason string) error { return nil }
func (m *mockConnection) IsClosed() bool                                  { return false }
func (m *mockConnection) LocalID() types.NodeID                           { return types.NodeID{} }
func (m *mockConnection) RemotePublicKey() endpointif.PublicKey           { return nil }
func (m *mockConnection) LocalAddrs() []endpointif.Address                { return nil }
func (m *mockConnection) RemoteAddrs() []endpointif.Address               { return nil }
func (m *mockConnection) Streams() []endpointif.Stream                    { return nil }
func (m *mockConnection) StreamCount() int                                { return 0 }
func (m *mockConnection) Stats() endpointif.ConnectionStats               { return endpointif.ConnectionStats{} }
func (m *mockConnection) Direction() endpointif.Direction                 { return endpointif.DirOutbound }
func (m *mockConnection) Transport() string                               { return "mock" }
func (m *mockConnection) Context() context.Context                        { return context.Background() }
func (m *mockConnection) Done() <-chan struct{}                           { return m.doneCh }
func (m *mockConnection) SetStreamHandler(proto types.ProtocolID, handler endpointif.ProtocolHandler) {
}
func (m *mockConnection) RemoveStreamHandler(proto types.ProtocolID) {}

// RealmContext 返回 Realm 上下文 (v1.1 新增)
func (m *mockConnection) RealmContext() *endpointif.RealmContext { return nil }

// SetRealmContext 设置 Realm 上下文 (v1.1 新增)
func (m *mockConnection) SetRealmContext(ctx *endpointif.RealmContext) {}

// IsRelayed 返回是否为中继连接 (Relay Transport Integration)
func (m *mockConnection) IsRelayed() bool { return false }

// RelayID 返回中继节点 ID (Relay Transport Integration)
func (m *mockConnection) RelayID() endpointif.NodeID { return types.EmptyNodeID }

// mockStream 模拟 Stream
type mockStream struct {
	conn         *mockConnection
	readBuf      *bytes.Buffer
	writeBuf     *bytes.Buffer
	closed       bool
	readErr      error
	writeErr     error
	mu           sync.Mutex
	onClose      func()
	readDeadline time.Time
}

func newMockStream(conn *mockConnection) *mockStream {
	return &mockStream{
		conn:     conn,
		readBuf:  bytes.NewBuffer(nil),
		writeBuf: bytes.NewBuffer(nil),
	}
}

func (m *mockStream) Read(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.readErr != nil {
		return 0, m.readErr
	}
	if m.closed {
		return 0, io.EOF
	}
	return m.readBuf.Read(p)
}

func (m *mockStream) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return 0, m.writeErr
	}
	if m.closed {
		return 0, errors.New("stream closed")
	}
	return m.writeBuf.Write(p)
}

func (m *mockStream) Close() error {
	m.mu.Lock()
	m.closed = true
	if m.onClose != nil {
		m.onClose()
	}
	m.mu.Unlock()
	return nil
}

func (m *mockStream) Connection() endpointif.Connection {
	if m.conn == nil {
		return nil
	}
	return m.conn
}

func (m *mockStream) ID() endpointif.StreamID       { return 0 }
func (m *mockStream) ProtocolID() types.ProtocolID  { return "" }
func (m *mockStream) SetDeadline(t time.Time) error { return nil }
func (m *mockStream) SetReadDeadline(t time.Time) error {
	m.mu.Lock()
	m.readDeadline = t
	m.mu.Unlock()
	return nil
}
func (m *mockStream) SetWriteDeadline(t time.Time) error { return nil }
func (m *mockStream) CloseRead() error                   { return nil }
func (m *mockStream) CloseWrite() error                  { return nil }
func (m *mockStream) SetPriority(p endpointif.Priority)  {}
func (m *mockStream) Priority() endpointif.Priority      { return 0 }
func (m *mockStream) Stats() endpointif.StreamStats      { return endpointif.StreamStats{} }
func (m *mockStream) IsClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

// ============================================================================
//                              Service 测试
// ============================================================================

func TestNewService(t *testing.T) {
	t.Run("使用默认配置", func(t *testing.T) {
		cfg := config.DefaultLivenessConfig()
		service := NewService(cfg, nil)
		require.NotNil(t, service)
		assert.NotNil(t, service.peers)
		assert.NotNil(t, service.decay)
	})

	t.Run("使用默认 Logger", func(t *testing.T) {
		cfg := config.DefaultLivenessConfig()
		service := NewService(cfg, nil)
		require.NotNil(t, service)
	})
}

// ============================================================================
//                              生命周期测试
// ============================================================================

func TestService_Start_Stop(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	ctx := context.Background()

	// 启动
	err := service.Start(ctx)
	assert.NoError(t, err)

	// 停止
	err = service.Stop()
	assert.NoError(t, err)
}

func TestService_Start_MultipleTimes(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	ctx := context.Background()

	// 多次启动应该是安全的
	err := service.Start(ctx)
	assert.NoError(t, err)

	err = service.Start(ctx)
	assert.NoError(t, err) // 第二次启动应该返回 nil

	err = service.Stop()
	assert.NoError(t, err)
}

func TestService_Stop_NotStarted(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	// 未启动时停止应该是安全的
	err := service.Stop()
	assert.NoError(t, err)
}

// ============================================================================
//                              节点状态测试
// ============================================================================

func TestService_PeerStatus(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	// 未知节点
	status := service.PeerStatus(peerID)
	assert.Equal(t, types.PeerStatusUnknown, status)

	// 添加节点状态
	service.mu.Lock()
	service.peers[peerID.String()] = &peerState{
		status:   types.PeerStatusOnline,
		lastSeen: time.Now(),
	}
	service.mu.Unlock()

	// 获取状态
	status = service.PeerStatus(peerID)
	assert.Equal(t, types.PeerStatusOnline, status)
}

func TestService_PeerHealth(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	// 未知节点
	health := service.PeerHealth(peerID)
	assert.Nil(t, health)

	// 添加节点状态
	now := time.Now()
	service.mu.Lock()
	service.peers[peerID.String()] = &peerState{
		status:      types.PeerStatusOnline,
		lastSeen:    now,
		avgRTT:      50 * time.Millisecond,
		healthScore: 80,
	}
	service.mu.Unlock()

	// 获取健康信息
	health = service.PeerHealth(peerID)
	require.NotNil(t, health)
	assert.Equal(t, types.PeerStatusOnline, health.Status)
}

func TestService_AllPeerStatuses(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	// 初始为空
	statuses := service.AllPeerStatuses()
	assert.Empty(t, statuses)

	// 添加多个节点
	for i := 0; i < 5; i++ {
		var peerID types.NodeID
		copy(peerID[:], []byte("peer-"+string(rune('0'+i))+"-12345678"))

		service.mu.Lock()
		service.peers[peerID.String()] = &peerState{
			status:   types.PeerStatusOnline,
			lastSeen: time.Now(),
		}
		service.mu.Unlock()
	}

	statuses = service.AllPeerStatuses()
	assert.Len(t, statuses, 5)
}

func TestService_OnlinePeers(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	// 添加不同状态的节点
	var peerID1, peerID2, peerID3 types.NodeID
	copy(peerID1[:], []byte("peer-online-12345678"))
	copy(peerID2[:], []byte("peer-offline-1234567"))
	copy(peerID3[:], []byte("peer-online2-1234567"))

	service.mu.Lock()
	service.peers[peerID1.String()] = &peerState{status: types.PeerStatusOnline}
	service.peers[peerID2.String()] = &peerState{status: types.PeerStatusOffline}
	service.peers[peerID3.String()] = &peerState{status: types.PeerStatusOnline}
	service.mu.Unlock()

	// 获取在线节点
	onlinePeers := service.OnlinePeers()
	assert.Len(t, onlinePeers, 2)
}

// ============================================================================
//                              健康分数测试
// ============================================================================

func TestService_HealthScore(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	// 未知节点
	score := service.HealthScore(peerID)
	assert.Equal(t, 0, score)

	// 添加节点状态
	service.mu.Lock()
	service.peers[peerID.String()] = &peerState{
		status:      types.PeerStatusOnline,
		healthScore: 80,
	}
	service.mu.Unlock()

	// 获取健康分数
	score = service.HealthScore(peerID)
	assert.Equal(t, 80, score)
}

// ============================================================================
//                              回调测试
// ============================================================================

func TestService_OnStatusChange(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	callback := livenessif.StatusChangeCallback(func(event types.PeerStatusChangeEvent) {
		// 回调处理
	})

	// 注册回调
	service.OnStatusChange(callback)

	service.mu.RLock()
	assert.Len(t, service.callbacks, 1)
	service.mu.RUnlock()
}

// ============================================================================
//                              衰减函数测试
// ============================================================================

func TestService_SetHealthScoreDecay(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	customDecay := livenessif.HealthScoreDecay{
		DecayInterval:  time.Minute,
		DecayAmount:    5,
		MinScore:       0,
		RecoveryOnPing: 10,
	}

	service.SetHealthScoreDecay(customDecay)

	// 验证设置成功
	assert.NotNil(t, service.decay)
}

// ============================================================================
//                              阈值测试
// ============================================================================

func TestService_Thresholds(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	thresholds := service.Thresholds()
	assert.Greater(t, thresholds.HeartbeatInterval, time.Duration(0))
}

func TestService_SetThresholds(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	newThresholds := types.LivenessThresholds{
		DegradedRTT:       500 * time.Millisecond,
		HeartbeatInterval: 30 * time.Second,
		HeartbeatTimeout:  90 * time.Second,
		StatusExpiry:      10 * time.Minute,
	}

	service.SetThresholds(newThresholds)

	// 验证设置成功
	thresholds := service.Thresholds()
	assert.Equal(t, 30*time.Second, thresholds.HeartbeatInterval)
}

// ============================================================================
//                              心跳测试
// ============================================================================

func TestService_StartStopHeartbeat(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err)

	var peerID types.NodeID
	copy(peerID[:], []byte("heartbeat-peer-12345"))

	// 添加节点状态
	service.mu.Lock()
	service.peers[peerID.String()] = &peerState{
		status:   types.PeerStatusOnline,
		lastSeen: time.Now(),
	}
	service.mu.Unlock()

	// 启动心跳
	service.StartHeartbeat(peerID)

	// 停止心跳
	service.StopHeartbeat(peerID)

	err = service.Stop()
	assert.NoError(t, err)
}

// ============================================================================
//                              并发安全测试
// ============================================================================

func TestService_Concurrency(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(2)

		go func(id int) {
			defer wg.Done()
			var peerID types.NodeID
			copy(peerID[:], []byte("peer-id-"+string(rune('0'+id))))

			service.mu.Lock()
			service.peers[peerID.String()] = &peerState{
				status:   types.PeerStatusOnline,
				lastSeen: time.Now(),
			}
			service.mu.Unlock()
		}(i)

		go func(id int) {
			defer wg.Done()
			var peerID types.NodeID
			copy(peerID[:], []byte("peer-id-"+string(rune('0'+id))))
			_ = service.PeerStatus(peerID)
			_ = service.HealthScore(peerID)
		}(i)
	}

	wg.Wait()
}

// ============================================================================
//                              协议 ID 测试
// ============================================================================

func TestProtocolIDs(t *testing.T) {
	assert.NotEmpty(t, string(ProtocolPing))
	assert.NotEmpty(t, string(ProtocolGoodbye))

	// 验证协议 ID 格式
	assert.Contains(t, string(ProtocolPing), "/dep2p/")
	assert.Contains(t, string(ProtocolGoodbye), "/dep2p/")
}

// ============================================================================
//                              常量测试
// ============================================================================

func TestConstants(t *testing.T) {
	assert.Equal(t, 32, PingPayloadSize)
}

// ============================================================================
//                              错误定义测试
// ============================================================================

func TestErrors(t *testing.T) {
	assert.NotNil(t, ErrServiceClosed)
	assert.NotNil(t, ErrPingTimeout)
	assert.NotNil(t, ErrPingFailed)
	assert.NotNil(t, ErrNoConnection)
}

// ============================================================================
//                              peerState 测试
// ============================================================================

func TestPeerState(t *testing.T) {
	state := &peerState{
		status:      types.PeerStatusOnline,
		lastSeen:    time.Now(),
		lastPing:    time.Now(),
		lastPingRTT: 20 * time.Millisecond,
		avgRTT:      25 * time.Millisecond,
		failedPings: 0,
		healthScore: 100,
	}

	assert.Equal(t, types.PeerStatusOnline, state.status)
	assert.False(t, state.lastSeen.IsZero())
	assert.False(t, state.lastPing.IsZero())
	assert.Equal(t, 20*time.Millisecond, state.lastPingRTT)
	assert.Equal(t, 25*time.Millisecond, state.avgRTT)
	assert.Equal(t, 0, state.failedPings)
	assert.Equal(t, 100, state.healthScore)
}

// ============================================================================
//                              DefaultHealthScoreDecay 测试
// ============================================================================

func TestDefaultHealthScoreDecay(t *testing.T) {
	decay := livenessif.DefaultHealthScoreDecay()
	require.NotNil(t, decay)

	// 验证默认值合理
	assert.Greater(t, decay.DecayInterval, time.Duration(0))
	assert.Greater(t, decay.DecayAmount, 0)
}

// ============================================================================
//                              Ping 功能测试
// ============================================================================

func TestService_Ping_ServiceClosed(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	// 标记服务已关闭
	atomic.StoreInt32(&service.closed, 1)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	_, err := service.Ping(context.Background(), peerID)
	assert.Equal(t, ErrServiceClosed, err)
}

func TestService_Ping_NoEndpoint(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil) // endpoint is nil

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	_, err := service.Ping(context.Background(), peerID)
	assert.Equal(t, ErrNoConnection, err)
}

func TestService_Ping_NoConnection_ConnectFails(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	endpoint := newMockEndpoint()
	endpoint.onConnect = func(ctx context.Context, id types.NodeID) (endpointif.Connection, error) {
		return nil, errors.New("connect failed")
	}
	service := NewService(cfg, endpoint)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	_, err := service.Ping(context.Background(), peerID)
	assert.Equal(t, ErrNoConnection, err)
}

func TestService_Ping_OpenStreamFails(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	endpoint := newMockEndpoint()
	service := NewService(cfg, endpoint)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	// 添加一个打开流会失败的连接
	conn := &mockConnection{
		remoteID: peerID,
		openStreamFn: func(ctx context.Context, proto types.ProtocolID) (endpointif.Stream, error) {
			return nil, errors.New("stream open failed")
		},
	}
	endpoint.addConnection(peerID, conn)

	_, err := service.Ping(context.Background(), peerID)
	assert.Equal(t, ErrPingFailed, err)

	// 验证 Ping 失败被记录
	service.mu.RLock()
	state := service.peers[peerID.String()]
	service.mu.RUnlock()
	assert.NotNil(t, state)
	assert.Equal(t, 1, state.failedPings)
}

func TestService_Ping_WriteFails(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	endpoint := newMockEndpoint()
	service := NewService(cfg, endpoint)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	stream := newMockStream(nil)
	stream.writeErr = errors.New("write failed")

	conn := &mockConnection{
		remoteID: peerID,
		openStreamFn: func(ctx context.Context, proto types.ProtocolID) (endpointif.Stream, error) {
			return stream, nil
		},
	}
	endpoint.addConnection(peerID, conn)

	_, err := service.Ping(context.Background(), peerID)
	assert.Equal(t, ErrPingFailed, err)
}

func TestService_Ping_ReadFails(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	endpoint := newMockEndpoint()
	service := NewService(cfg, endpoint)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	stream := newMockStream(nil)
	stream.readErr = io.EOF

	conn := &mockConnection{
		remoteID: peerID,
		openStreamFn: func(ctx context.Context, proto types.ProtocolID) (endpointif.Stream, error) {
			return stream, nil
		},
	}
	endpoint.addConnection(peerID, conn)

	_, err := service.Ping(context.Background(), peerID)
	assert.Equal(t, ErrPingFailed, err)
}

func TestService_Ping_Success(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	cfg.DegradedRTTThreshold = 100 * time.Millisecond
	endpoint := newMockEndpoint()
	service := NewService(cfg, endpoint)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	stream := newMockStream(nil)
	// 预填充响应数据（模拟 echo）
	stream.readBuf.Write(make([]byte, PingPayloadSize))

	conn := &mockConnection{
		remoteID: peerID,
		openStreamFn: func(ctx context.Context, proto types.ProtocolID) (endpointif.Stream, error) {
			return stream, nil
		},
	}
	endpoint.addConnection(peerID, conn)

	rtt, err := service.Ping(context.Background(), peerID)
	assert.NoError(t, err)
	assert.Greater(t, rtt, time.Duration(0))

	// 验证状态更新
	service.mu.RLock()
	state := service.peers[peerID.String()]
	service.mu.RUnlock()
	assert.NotNil(t, state)
	assert.Equal(t, 0, state.failedPings)
	assert.Equal(t, types.PeerStatusOnline, state.status)
}

func TestService_Ping_DegradedStatus(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	cfg.DegradedRTTThreshold = 1 * time.Nanosecond // 设置非常低的阈值
	endpoint := newMockEndpoint()
	service := NewService(cfg, endpoint)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	stream := newMockStream(nil)
	stream.readBuf.Write(make([]byte, PingPayloadSize))

	conn := &mockConnection{
		remoteID: peerID,
		openStreamFn: func(ctx context.Context, proto types.ProtocolID) (endpointif.Stream, error) {
			// 模拟一些延迟
			time.Sleep(10 * time.Millisecond)
			return stream, nil
		},
	}
	endpoint.addConnection(peerID, conn)

	_, err := service.Ping(context.Background(), peerID)
	assert.NoError(t, err)

	// 验证状态为 Degraded
	service.mu.RLock()
	state := service.peers[peerID.String()]
	service.mu.RUnlock()
	assert.Equal(t, types.PeerStatusDegraded, state.status)
}

// ============================================================================
//                              updatePingSuccess 测试
// ============================================================================

func TestService_updatePingSuccess_NewPeer(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	cfg.DegradedRTTThreshold = 100 * time.Millisecond
	service := NewService(cfg, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	// 更新一个不存在的节点
	service.updatePingSuccess(peerID, 10*time.Millisecond)

	service.mu.RLock()
	state := service.peers[peerID.String()]
	service.mu.RUnlock()

	assert.NotNil(t, state)
	assert.Equal(t, types.PeerStatusOnline, state.status)
	assert.Equal(t, 10*time.Millisecond, state.avgRTT)
	assert.Equal(t, 0, state.failedPings)
}

func TestService_updatePingSuccess_ExistingPeer_RTTAverage(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	cfg.DegradedRTTThreshold = 100 * time.Millisecond
	service := NewService(cfg, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	// 初始 RTT
	service.updatePingSuccess(peerID, 80*time.Millisecond)

	// 第二次 RTT，验证指数移动平均
	service.updatePingSuccess(peerID, 40*time.Millisecond)

	service.mu.RLock()
	state := service.peers[peerID.String()]
	service.mu.RUnlock()

	// avgRTT = (80*7 + 40) / 8 = 75ms
	assert.Equal(t, 75*time.Millisecond, state.avgRTT)
}

func TestService_updatePingSuccess_HealthScoreRecovery(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	// 设置初始健康分较低
	service.mu.Lock()
	service.peers[peerID.String()] = &peerState{
		status:      types.PeerStatusDegraded,
		healthScore: 30,
	}
	service.mu.Unlock()

	// Ping 成功，健康分应该恢复
	service.updatePingSuccess(peerID, 10*time.Millisecond)

	service.mu.RLock()
	state := service.peers[peerID.String()]
	service.mu.RUnlock()

	// 应该恢复 RecoveryOnPing 分
	assert.Equal(t, 30+service.decay.RecoveryOnPing, state.healthScore)
}

func TestService_updatePingSuccess_HealthScoreCap(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	// 设置健康分接近满分
	service.mu.Lock()
	service.peers[peerID.String()] = &peerState{
		status:      types.PeerStatusOnline,
		healthScore: 95,
	}
	service.mu.Unlock()

	// Ping 成功，健康分不应超过 100
	service.updatePingSuccess(peerID, 10*time.Millisecond)

	service.mu.RLock()
	state := service.peers[peerID.String()]
	service.mu.RUnlock()

	assert.Equal(t, 100, state.healthScore)
}

// ============================================================================
//                              handlePingFailure 测试
// ============================================================================

func TestService_handlePingFailure_NewPeer(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	service.handlePingFailure(peerID)

	service.mu.RLock()
	state := service.peers[peerID.String()]
	service.mu.RUnlock()

	assert.NotNil(t, state)
	assert.Equal(t, 1, state.failedPings)
	assert.Equal(t, types.PeerStatusDegraded, state.status)
}

func TestService_handlePingFailure_ThreeFailures_Offline(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	// 三次连续失败
	service.handlePingFailure(peerID)
	service.handlePingFailure(peerID)
	service.handlePingFailure(peerID)

	service.mu.RLock()
	state := service.peers[peerID.String()]
	service.mu.RUnlock()

	assert.Equal(t, 3, state.failedPings)
	assert.Equal(t, types.PeerStatusOffline, state.status)
}

func TestService_handlePingFailure_HealthScoreDecay(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	// 设置初始状态
	service.mu.Lock()
	service.peers[peerID.String()] = &peerState{
		status:      types.PeerStatusOnline,
		healthScore: 80,
	}
	service.mu.Unlock()

	service.handlePingFailure(peerID)

	service.mu.RLock()
	state := service.peers[peerID.String()]
	service.mu.RUnlock()

	// 健康分应该衰减
	assert.Equal(t, 80-service.decay.DecayAmount, state.healthScore)
}

func TestService_handlePingFailure_MinScore(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	// 设置 MinScore
	service.decay.MinScore = 10

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	// 设置初始健康分很低
	service.mu.Lock()
	service.peers[peerID.String()] = &peerState{
		status:      types.PeerStatusOnline,
		healthScore: 5,
	}
	service.mu.Unlock()

	service.handlePingFailure(peerID)

	service.mu.RLock()
	state := service.peers[peerID.String()]
	service.mu.RUnlock()

	// 健康分不应低于 MinScore
	assert.Equal(t, 10, state.healthScore)
}

// ============================================================================
//                              handlePingStream 测试
// ============================================================================

func TestService_handlePingStream(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	// 模拟流
	stream := newMockStream(nil)

	// 写入 ping payload
	payload := make([]byte, PingPayloadSize)
	for i := range payload {
		payload[i] = byte(i)
	}
	stream.readBuf.Write(payload)

	// 处理流
	service.handlePingStream(stream)

	// 验证 echo 响应
	assert.Equal(t, PingPayloadSize, stream.writeBuf.Len())
	assert.Equal(t, payload, stream.writeBuf.Bytes())
}

func TestService_handlePingStream_ReadError(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	stream := newMockStream(nil)
	stream.readErr = io.EOF

	// 不应 panic
	service.handlePingStream(stream)
	assert.True(t, stream.closed)
}

// ============================================================================
//                              Goodbye 功能测试
// ============================================================================

func TestService_SendGoodbye_ServiceClosed(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)
	atomic.StoreInt32(&service.closed, 1)

	err := service.SendGoodbye(context.Background(), types.GoodbyeReasonShutdown)
	assert.Equal(t, ErrServiceClosed, err)
}

func TestService_SendGoodbye_NoEndpoint(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	err := service.SendGoodbye(context.Background(), types.GoodbyeReasonShutdown)
	assert.Equal(t, ErrNoConnection, err)
}

func TestService_SendGoodbyeTo_ServiceClosed(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)
	atomic.StoreInt32(&service.closed, 1)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	err := service.SendGoodbyeTo(context.Background(), peerID, types.GoodbyeReasonShutdown)
	assert.Equal(t, ErrServiceClosed, err)
}

func TestService_SendGoodbyeTo_NoConnection(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	endpoint := newMockEndpoint()
	service := NewService(cfg, endpoint)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	err := service.SendGoodbyeTo(context.Background(), peerID, types.GoodbyeReasonShutdown)
	assert.Equal(t, ErrNoConnection, err)
}

func TestService_SendGoodbyeTo_OpenStreamFails(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	endpoint := newMockEndpoint()
	service := NewService(cfg, endpoint)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	conn := &mockConnection{
		remoteID: peerID,
		openStreamFn: func(ctx context.Context, proto types.ProtocolID) (endpointif.Stream, error) {
			return nil, errors.New("stream failed")
		},
	}
	endpoint.addConnection(peerID, conn)

	err := service.SendGoodbyeTo(context.Background(), peerID, types.GoodbyeReasonShutdown)
	assert.Error(t, err)
}

func TestService_SendGoodbyeTo_Success(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	endpoint := newMockEndpoint()
	service := NewService(cfg, endpoint)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	// 设置初始状态为在线
	service.mu.Lock()
	service.peers[peerID.String()] = &peerState{
		status: types.PeerStatusOnline,
	}
	service.mu.Unlock()

	stream := newMockStream(nil)
	conn := &mockConnection{
		remoteID: peerID,
		openStreamFn: func(ctx context.Context, proto types.ProtocolID) (endpointif.Stream, error) {
			return stream, nil
		},
	}
	endpoint.addConnection(peerID, conn)

	err := service.SendGoodbyeTo(context.Background(), peerID, types.GoodbyeReasonShutdown)
	assert.NoError(t, err)

	// 验证状态变为离线
	service.mu.RLock()
	state := service.peers[peerID.String()]
	service.mu.RUnlock()
	assert.Equal(t, types.PeerStatusOffline, state.status)
}

func TestService_handleGoodbyeStream(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	// 设置初始状态
	service.mu.Lock()
	service.peers[peerID.String()] = &peerState{
		status: types.PeerStatusOnline,
	}
	service.mu.Unlock()

	conn := &mockConnection{remoteID: peerID}
	stream := newMockStream(conn)

	// 写入 goodbye reason（新格式：长度 + 内容）
	reason := "shutdown"
	_ = binary.Write(stream.readBuf, binary.BigEndian, uint16(len(reason)))
	stream.readBuf.WriteString(reason)

	service.handleGoodbyeStream(stream)

	// 验证状态变为离线
	service.mu.RLock()
	state := service.peers[peerID.String()]
	service.mu.RUnlock()
	assert.Equal(t, types.PeerStatusOffline, state.status)
}

func TestService_handleGoodbyeStream_NoConnection(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	stream := newMockStream(nil) // conn is nil

	// 不应 panic
	service.handleGoodbyeStream(stream)
}

// ============================================================================
//                              状态变更回调测试
// ============================================================================

func TestService_notifyStatusChange(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	// 注册回调
	callbackCh := make(chan types.PeerStatusChangeEvent, 1)
	service.OnStatusChange(func(event types.PeerStatusChangeEvent) {
		callbackCh <- event
	})

	// 触发状态变更
	service.notifyStatusChange(peerID, types.PeerStatusOnline, types.PeerStatusOffline, "test")

	// 等待回调
	select {
	case event := <-callbackCh:
		assert.Equal(t, peerID, event.NodeID)
		assert.Equal(t, types.PeerStatusOnline, event.OldStatus)
		assert.Equal(t, types.PeerStatusOffline, event.NewStatus)
		assert.Equal(t, "test", event.Reason)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("callback not received")
	}
}

func TestService_notifyStatusChange_MultipleCallbacks(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	// 注册多个回调
	var wg sync.WaitGroup
	count := int32(0)
	for i := 0; i < 3; i++ {
		service.OnStatusChange(func(event types.PeerStatusChangeEvent) {
			atomic.AddInt32(&count, 1)
			wg.Done()
		})
	}
	wg.Add(3)

	service.notifyStatusChange(peerID, types.PeerStatusOnline, types.PeerStatusOffline, "test")

	wg.Wait()
	assert.Equal(t, int32(3), count)
}

// ============================================================================
//                              心跳循环测试
// ============================================================================

func TestService_heartbeatLoop_Stops(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	cfg.HeartbeatInterval = 10 * time.Millisecond
	service := NewService(cfg, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	ctx, cancel := context.WithCancel(context.Background())
	state := &peerState{
		heartbeatCtx:    ctx,
		heartbeatCancel: cancel,
	}

	done := make(chan struct{})
	go func() {
		service.heartbeatLoop(peerID, state)
		close(done)
	}()

	// 取消心跳
	time.Sleep(30 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// 正常退出
	case <-time.After(100 * time.Millisecond):
		t.Fatal("heartbeat loop did not stop")
	}
}

func TestService_StartHeartbeat_RestartsExisting(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	ctx := context.Background()
	_ = service.Start(ctx)
	defer service.Stop()

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	// 启动第一次心跳
	service.StartHeartbeat(peerID)

	service.mu.RLock()
	cancel1 := service.peers[peerID.String()].heartbeatCancel
	service.mu.RUnlock()
	assert.NotNil(t, cancel1)

	// 启动第二次心跳（应该取消第一次）
	time.Sleep(10 * time.Millisecond)
	service.StartHeartbeat(peerID)

	service.mu.RLock()
	cancel2 := service.peers[peerID.String()].heartbeatCancel
	service.mu.RUnlock()
	assert.NotNil(t, cancel2)
}

// ============================================================================
//                              expiryLoop 测试
// ============================================================================

func TestService_cleanupExpiredStates(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	cfg.StatusExpiry = 10 * time.Millisecond
	service := NewService(cfg, nil)

	var peerID1, peerID2 types.NodeID
	copy(peerID1[:], []byte("peer-expired-12345678"))
	copy(peerID2[:], []byte("peer-active-123456789"))

	service.mu.Lock()
	// 过期的离线节点
	service.peers[peerID1.String()] = &peerState{
		status:   types.PeerStatusOffline,
		lastSeen: time.Now().Add(-1 * time.Minute),
	}
	// 活跃的在线节点
	service.peers[peerID2.String()] = &peerState{
		status:   types.PeerStatusOnline,
		lastSeen: time.Now(),
	}
	service.mu.Unlock()

	// 运行清理
	service.cleanupExpiredStates()

	service.mu.RLock()
	_, expired := service.peers[peerID1.String()]
	_, active := service.peers[peerID2.String()]
	service.mu.RUnlock()

	assert.False(t, expired, "过期节点应被清理")
	assert.True(t, active, "活跃节点应保留")
}

func TestService_cleanupExpiredStates_OfflineNotExpired(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	cfg.StatusExpiry = 10 * time.Minute
	service := NewService(cfg, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("peer-offline-12345678"))

	service.mu.Lock()
	service.peers[peerID.String()] = &peerState{
		status:   types.PeerStatusOffline,
		lastSeen: time.Now().Add(-1 * time.Minute), // 未过期
	}
	service.mu.Unlock()

	service.cleanupExpiredStates()

	service.mu.RLock()
	_, exists := service.peers[peerID.String()]
	service.mu.RUnlock()

	assert.True(t, exists, "未过期的离线节点不应被清理")
}

// ============================================================================
//                              协议处理器注册测试
// ============================================================================

func TestService_Start_RegistersHandlers(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	endpoint := newMockEndpoint()
	service := NewService(cfg, endpoint)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err)

	endpoint.mu.RLock()
	_, hasPing := endpoint.handlers[ProtocolPing]
	_, hasGoodbye := endpoint.handlers[ProtocolGoodbye]
	endpoint.mu.RUnlock()

	assert.True(t, hasPing, "Ping 协议处理器应被注册")
	assert.True(t, hasGoodbye, "Goodbye 协议处理器应被注册")

	err = service.Stop()
	assert.NoError(t, err)

	endpoint.mu.RLock()
	_, hasPing = endpoint.handlers[ProtocolPing]
	_, hasGoodbye = endpoint.handlers[ProtocolGoodbye]
	endpoint.mu.RUnlock()

	assert.False(t, hasPing, "Ping 协议处理器应被移除")
	assert.False(t, hasGoodbye, "Goodbye 协议处理器应被移除")
}

// ============================================================================
//                              集成测试
// ============================================================================

func TestService_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("跳过集成测试")
	}

	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	ctx := context.Background()

	// 启动服务
	err := service.Start(ctx)
	require.NoError(t, err)

	// 等待一小段时间
	time.Sleep(100 * time.Millisecond)

	// 添加一些节点
	for i := 0; i < 3; i++ {
		var peerID types.NodeID
		copy(peerID[:], []byte("integ-peer-"+string(rune('0'+i))))

		service.mu.Lock()
		service.peers[peerID.String()] = &peerState{
			status:      types.PeerStatusOnline,
			lastSeen:    time.Now(),
			healthScore: 100,
		}
		service.mu.Unlock()
	}

	// 验证节点
	statuses := service.AllPeerStatuses()
	assert.Len(t, statuses, 3)

	// 停止服务
	err = service.Stop()
	assert.NoError(t, err)
}

// ============================================================================
//                              边界条件测试
// ============================================================================

func TestService_Stop_StopsAllHeartbeats(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	ctx := context.Background()
	_ = service.Start(ctx)

	var peerID1, peerID2 types.NodeID
	copy(peerID1[:], []byte("peer-heartbeat-123456"))
	copy(peerID2[:], []byte("peer-heartbeat-789012"))

	// 启动心跳
	service.StartHeartbeat(peerID1)
	service.StartHeartbeat(peerID2)

	// 停止服务应该停止所有心跳
	err := service.Stop()
	assert.NoError(t, err)
}

func TestService_Stop_MultipleTimes(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	ctx := context.Background()
	_ = service.Start(ctx)

	// 多次停止应该是安全的
	err := service.Stop()
	assert.NoError(t, err)

	err = service.Stop()
	assert.NoError(t, err) // 第二次应该返回 nil
}

func TestService_PeerHealth_AllFields(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("test-peer-id-12345678"))

	now := time.Now()
	service.mu.Lock()
	service.peers[peerID.String()] = &peerState{
		status:      types.PeerStatusOnline,
		lastSeen:    now,
		lastPing:    now.Add(-1 * time.Second),
		lastPingRTT: 20 * time.Millisecond,
		avgRTT:      25 * time.Millisecond,
		failedPings: 1,
		healthScore: 85,
	}
	service.mu.Unlock()

	health := service.PeerHealth(peerID)
	require.NotNil(t, health)

	assert.Equal(t, peerID, health.NodeID)
	assert.Equal(t, types.PeerStatusOnline, health.Status)
	assert.Equal(t, now, health.LastSeen)
	assert.Equal(t, now.Add(-1*time.Second), health.LastPing)
	assert.Equal(t, 20*time.Millisecond, health.LastPingRTT)
	assert.Equal(t, 25*time.Millisecond, health.AvgRTT)
	assert.Equal(t, 1, health.FailedPings)
	assert.Equal(t, 85, health.HealthScore)
}

// ============================================================================
//                              修复验证测试
// ============================================================================

// TestService_GoodbyeProtocolConsistency 验证 Goodbye 协议一致性
func TestService_GoodbyeProtocolConsistency(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	endpoint := newMockEndpoint()
	service := NewService(cfg, endpoint)

	var peerID types.NodeID
	copy(peerID[:], []byte("goodbye-protocol-test"))

	// 创建一个模拟双向通信的 stream
	sendStream := newMockStream(nil)
	conn := &mockConnection{
		remoteID: peerID,
		openStreamFn: func(ctx context.Context, proto types.ProtocolID) (endpointif.Stream, error) {
			return sendStream, nil
		},
	}
	endpoint.addConnection(peerID, conn)

	// 发送 Goodbye
	reason := types.GoodbyeReasonShutdown
	err := service.SendGoodbyeTo(context.Background(), peerID, reason)
	require.NoError(t, err)

	// 验证发送的格式：length (uint16) + content
	writtenData := sendStream.writeBuf.Bytes()
	assert.GreaterOrEqual(t, len(writtenData), 2, "至少应该有 2 字节的长度字段")

	// 读取长度
	var reasonLen uint16
	err = binary.Read(bytes.NewReader(writtenData), binary.BigEndian, &reasonLen)
	require.NoError(t, err)

	// 验证长度匹配
	expectedLen := len([]byte(string(reason)))
	assert.Equal(t, uint16(expectedLen), reasonLen, "长度字段应匹配 reason 内容长度")

	// 验证内容
	content := writtenData[2:]
	assert.Equal(t, string(reason), string(content), "内容应匹配 reason")
}

// TestService_HandleGoodbyeStream_ProtocolFormat 验证接收端正确解析协议格式
func TestService_HandleGoodbyeStream_ProtocolFormat(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("goodbye-receive-test-1"))

	// 设置初始状态
	service.mu.Lock()
	service.peers[peerID.String()] = &peerState{
		status: types.PeerStatusOnline,
	}
	service.mu.Unlock()

	conn := &mockConnection{remoteID: peerID}
	stream := newMockStream(conn)

	// 写入正确格式的 goodbye（length + content）
	reason := "shutdown"
	_ = binary.Write(stream.readBuf, binary.BigEndian, uint16(len(reason)))
	stream.readBuf.WriteString(reason)

	service.handleGoodbyeStream(stream)

	// 验证状态变为离线
	service.mu.RLock()
	state := service.peers[peerID.String()]
	service.mu.RUnlock()
	assert.Equal(t, types.PeerStatusOffline, state.status)
}

// TestService_HandleGoodbyeStream_TooLongReason 验证过长的 reason 被拒绝
func TestService_HandleGoodbyeStream_TooLongReason(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("goodbye-long-reason-te"))

	conn := &mockConnection{remoteID: peerID}
	stream := newMockStream(conn)

	// 写入过长的 reason 长度
	_ = binary.Write(stream.readBuf, binary.BigEndian, uint16(500)) // > 256 限制

	service.handleGoodbyeStream(stream)

	// 应该没有创建状态（因为被拒绝）
	service.mu.RLock()
	_, exists := service.peers[peerID.String()]
	service.mu.RUnlock()
	assert.False(t, exists, "过长的 reason 应被拒绝")
}

// TestService_StartHeartbeat_WithoutStart 验证未启动服务时的心跳处理
func TestService_StartHeartbeat_WithoutStart(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("heartbeat-no-start-tes"))

	// 不调用 Start()，直接调用 StartHeartbeat
	// 应该不会 panic，而是静默返回
	service.StartHeartbeat(peerID)

	// 验证没有创建心跳（因为服务未启动）
	service.mu.RLock()
	state, exists := service.peers[peerID.String()]
	service.mu.RUnlock()

	// 状态可能被创建，但心跳不应该启动
	if exists {
		assert.Nil(t, state.heartbeatCancel, "服务未启动时不应创建心跳")
	}
}

// TestService_CallbackConcurrentSafety 验证回调并发安全
func TestService_CallbackConcurrentSafety(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	service := NewService(cfg, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("callback-concurrent-te"))

	var wg sync.WaitGroup
	callbackCount := int32(0)

	// 并发注册回调
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			service.OnStatusChange(func(event types.PeerStatusChangeEvent) {
				atomic.AddInt32(&callbackCount, 1)
			})
		}()
	}

	// 并发触发通知
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			service.notifyStatusChange(peerID, types.PeerStatusOnline, types.PeerStatusOffline, "test")
		}()
	}

	wg.Wait()

	// 等待所有回调完成
	time.Sleep(100 * time.Millisecond)

	// 验证没有 panic，且回调被调用
	assert.Greater(t, atomic.LoadInt32(&callbackCount), int32(0))
}

// TestService_CleanupExpiredStates_NotifiesAfterUnlock 验证清理过期状态时通知在释放锁后发送
func TestService_CleanupExpiredStates_NotifiesAfterUnlock(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	cfg.StatusExpiry = 10 * time.Millisecond
	service := NewService(cfg, nil)

	var peerID types.NodeID
	copy(peerID[:], []byte("cleanup-notify-test-12"))

	// 添加过期的离线节点
	service.mu.Lock()
	service.peers[peerID.String()] = &peerState{
		status:   types.PeerStatusOffline,
		lastSeen: time.Now().Add(-1 * time.Minute),
	}
	service.mu.Unlock()

	// 注册回调
	notified := make(chan bool, 1)
	service.OnStatusChange(func(event types.PeerStatusChangeEvent) {
		notified <- true
	})

	// 运行清理
	service.cleanupExpiredStates()

	// 验证节点被删除
	service.mu.RLock()
	_, exists := service.peers[peerID.String()]
	service.mu.RUnlock()
	assert.False(t, exists, "过期节点应被删除")

	// 验证回调被触发
	select {
	case <-notified:
		// 成功
	case <-time.After(100 * time.Millisecond):
		t.Fatal("回调未被触发")
	}
}

// TestService_PingPayload_Random 验证 Ping payload 是随机的
func TestService_PingPayload_Random(t *testing.T) {
	cfg := config.DefaultLivenessConfig()
	endpoint := newMockEndpoint()
	service := NewService(cfg, endpoint)

	var peerID types.NodeID
	copy(peerID[:], []byte("ping-random-payload-12"))

	var payloads [][]byte

	// 创建多个 stream 来捕获多次 Ping 的 payload
	for i := 0; i < 3; i++ {
		stream := newMockStream(nil)
		// 预填充响应
		stream.readBuf.Write(make([]byte, PingPayloadSize))

		conn := &mockConnection{
			remoteID: peerID,
			openStreamFn: func(ctx context.Context, proto types.ProtocolID) (endpointif.Stream, error) {
				return stream, nil
			},
		}
		endpoint.addConnection(peerID, conn)

		_, err := service.Ping(context.Background(), peerID)
		require.NoError(t, err)

		// 获取发送的 payload
		payload := make([]byte, stream.writeBuf.Len())
		copy(payload, stream.writeBuf.Bytes())
		payloads = append(payloads, payload)
	}

	// 验证 payloads 不全相同（随机性）
	allSame := true
	for i := 1; i < len(payloads); i++ {
		if !bytes.Equal(payloads[0], payloads[i]) {
			allSame = false
			break
		}
	}
	assert.False(t, allSame, "Ping payloads 应该是随机的，不应完全相同")
}
