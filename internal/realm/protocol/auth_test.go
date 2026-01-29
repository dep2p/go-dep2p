package protocol

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/realm/auth"
	"github.com/dep2p/go-dep2p/internal/realm/interfaces"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                              Mock 实现
// ============================================================================

// mockHost 模拟 Host
type mockHost struct {
	mu       sync.RWMutex
	id       string
	handlers map[string]pkgif.StreamHandler
	streams  chan *mockStream
}

func newMockHost(id string) *mockHost {
	return &mockHost{
		id:       id,
		handlers: make(map[string]pkgif.StreamHandler),
		streams:  make(chan *mockStream, 10),
	}
}

func (h *mockHost) ID() string {
	return h.id
}

func (h *mockHost) Addrs() []string {
	return []string{"/ip4/127.0.0.1/tcp/0"}
}

func (h *mockHost) Listen(addrs ...string) error {
	return nil
}

func (h *mockHost) Connect(ctx context.Context, peerID string, addrs []string) error {
	return nil
}

func (h *mockHost) SetStreamHandler(protocolID string, handler pkgif.StreamHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.handlers[protocolID] = handler
}

func (h *mockHost) RemoveStreamHandler(protocolID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.handlers, protocolID)
}

func (h *mockHost) NewStream(ctx context.Context, peerID string, protocolIDs ...string) (pkgif.Stream, error) {
	// 创建一对连接的流
	local, remote := newMockStreamPair(h.id, peerID, protocolIDs[0])
	
	// 将远程流发送给对方处理
	h.streams <- remote
	
	return local, nil
}

func (h *mockHost) Peerstore() pkgif.Peerstore {
	return nil
}

func (h *mockHost) EventBus() pkgif.EventBus {
	return nil
}

func (h *mockHost) Close() error {
	return nil
}

func (h *mockHost) AdvertisedAddrs() []string {
	return h.Addrs()
}

func (h *mockHost) ShareableAddrs() []string {
	return nil
}

func (h *mockHost) HolePunchAddrs() []string {
	return nil
}

func (h *mockHost) SetReachabilityCoordinator(coordinator pkgif.ReachabilityCoordinator) {
	// no-op for mock
}

func (h *mockHost) Network() pkgif.Swarm {
	return nil
}

func (h *mockHost) HandleInboundStream(stream pkgif.Stream) {
	// Mock implementation: no-op
}

// getHandler 获取协议处理器（测试用）
func (h *mockHost) getHandler(protocolID string) pkgif.StreamHandler {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.handlers[protocolID]
}

// mockStream 模拟 Stream
type mockStream struct {
	localPeer  string
	remotePeer string
	protocol   string
	readBuf    *pipe
	writeBuf   *pipe
	conn       *mockConnection
	closed     bool
	mu         sync.Mutex
}

func newMockStreamPair(localPeer, remotePeer, protocol string) (*mockStream, *mockStream) {
	pipe1 := newPipe()
	pipe2 := newPipe()

	conn1 := &mockConnection{localPeer: localPeer, remotePeer: remotePeer}
	conn2 := &mockConnection{localPeer: remotePeer, remotePeer: localPeer}

	local := &mockStream{
		localPeer:  localPeer,
		remotePeer: remotePeer,
		protocol:   protocol,
		readBuf:    pipe2,
		writeBuf:   pipe1,
		conn:       conn1,
	}

	remote := &mockStream{
		localPeer:  remotePeer,
		remotePeer: localPeer,
		protocol:   protocol,
		readBuf:    pipe1,
		writeBuf:   pipe2,
		conn:       conn2,
	}

	return local, remote
}

func (s *mockStream) Read(p []byte) (n int, err error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return 0, io.EOF
	}
	s.mu.Unlock()
	return s.readBuf.Read(p)
}

func (s *mockStream) Write(p []byte) (n int, err error) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return 0, io.ErrClosedPipe
	}
	s.mu.Unlock()
	return s.writeBuf.Write(p)
}

func (s *mockStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		s.readBuf.Close()
		s.writeBuf.Close()
	}
	return nil
}

func (s *mockStream) Reset() error {
	return s.Close()
}

func (s *mockStream) Protocol() string {
	return s.protocol
}

func (s *mockStream) SetProtocol(protocol string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.protocol = protocol
}

func (s *mockStream) Conn() pkgif.Connection {
	return s.conn
}

func (s *mockStream) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *mockStream) CloseWrite() error {
	return nil
}

func (s *mockStream) CloseRead() error {
	return nil
}

func (s *mockStream) SetDeadline(t time.Time) error {
	return nil
}

func (s *mockStream) SetReadDeadline(t time.Time) error {
	return nil
}

func (s *mockStream) SetWriteDeadline(t time.Time) error {
	return nil
}

func (s *mockStream) Stat() types.StreamStat {
	s.mu.Lock()
	defer s.mu.Unlock()
	return types.StreamStat{
		Direction:    types.DirUnknown,
		Opened:       time.Now(),
		Protocol:     types.ProtocolID(s.protocol),
		BytesRead:    0,
		BytesWritten: 0,
	}
}

func (s *mockStream) State() types.StreamState {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return types.StreamStateClosed
	}
	return types.StreamStateOpen
}

// mockConnection 模拟 Connection
type mockConnection struct {
	localPeer  string
	remotePeer string
}

func (c *mockConnection) LocalPeer() types.PeerID {
	return types.PeerID(c.localPeer)
}

func (c *mockConnection) LocalMultiaddr() types.Multiaddr {
	addr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	return addr
}

func (c *mockConnection) RemotePeer() types.PeerID {
	return types.PeerID(c.remotePeer)
}

func (c *mockConnection) RemoteMultiaddr() types.Multiaddr {
	addr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
	return addr
}

func (c *mockConnection) NewStream(ctx context.Context) (pkgif.Stream, error) {
	return nil, nil
}

func (c *mockConnection) AcceptStream() (pkgif.Stream, error) {
	return nil, nil
}

func (c *mockConnection) GetStreams() []pkgif.Stream {
	return nil
}

func (c *mockConnection) Stat() pkgif.ConnectionStat {
	return pkgif.ConnectionStat{}
}

func (c *mockConnection) Close() error {
	return nil
}

func (c *mockConnection) IsClosed() bool {
	return false
}

func (c *mockConnection) ConnType() pkgif.ConnectionType {
	return pkgif.ConnectionTypeDirect
}

// pipe 简单的内存管道
type pipe struct {
	mu     sync.Mutex
	buf    []byte
	cond   *sync.Cond
	closed bool
}

func newPipe() *pipe {
	p := &pipe{}
	p.cond = sync.NewCond(&p.mu)
	return p
}

func (p *pipe) Read(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for len(p.buf) == 0 && !p.closed {
		p.cond.Wait()
	}

	if p.closed && len(p.buf) == 0 {
		return 0, io.EOF
	}

	n := copy(data, p.buf)
	p.buf = p.buf[n:]
	return n, nil
}

func (p *pipe) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return 0, io.ErrClosedPipe
	}

	p.buf = append(p.buf, data...)
	p.cond.Signal()
	return len(data), nil
}

func (p *pipe) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	p.cond.Broadcast()
	return nil
}

// mockAuthenticator 模拟认证器
type mockAuthenticator struct {
	realmID string
	mode    interfaces.AuthMode
}

func (a *mockAuthenticator) Authenticate(ctx context.Context, peerID string, proof []byte) (bool, error) {
	return true, nil
}

func (a *mockAuthenticator) GenerateProof(ctx context.Context) ([]byte, error) {
	return []byte("mock-proof"), nil
}

func (a *mockAuthenticator) Mode() interfaces.AuthMode {
	return a.mode
}

func (a *mockAuthenticator) RealmID() string {
	return a.realmID
}

func (a *mockAuthenticator) Close() error {
	return nil
}

// ============================================================================
//                              单元测试
// ============================================================================

func TestAuthHandler_Creation(t *testing.T) {
	host := newMockHost("peer1")
	realmID := "test-realm"
	authKey := []byte("test-auth-key-32-bytes-long!!")
	authenticator := &mockAuthenticator{realmID: realmID, mode: interfaces.AuthModePSK}

	handler := NewAuthHandler(host, realmID, authKey, authenticator, nil)

	require.NotNil(t, handler)
	require.Equal(t, realmID, handler.realmID)
	require.Equal(t, authKey, handler.authKey)
	require.NotNil(t, handler.challengeHandler)
}

func TestAuthHandler_StartStop(t *testing.T) {
	host := newMockHost("peer1")
	realmID := "test-realm"
	authKey := []byte("test-auth-key-32-bytes-long!!")
	authenticator := &mockAuthenticator{realmID: realmID, mode: interfaces.AuthModePSK}

	handler := NewAuthHandler(host, realmID, authKey, authenticator, nil)

	ctx := context.Background()

	// 测试启动
	err := handler.Start(ctx)
	require.NoError(t, err)
	require.True(t, handler.started)

	// 验证协议已注册
	protocolID := "/dep2p/realm/test-realm/auth/1.0.0"
	require.NotNil(t, host.getHandler(protocolID))

	// 测试重复启动
	err = handler.Start(ctx)
	require.Error(t, err)

	// 测试停止
	err = handler.Stop(ctx)
	require.NoError(t, err)
	require.False(t, handler.started)

	// 验证协议已注销
	require.Nil(t, host.getHandler(protocolID))
}

func TestAuthHandler_MessageReadWrite(t *testing.T) {
	local, remote := newMockStreamPair("peer1", "peer2", "/test/1.0.0")

	// 测试写入和读取
	testData := []byte("hello, world!")

	// 写入消息
	err := writeMessage(local, testData)
	require.NoError(t, err)

	// 读取消息
	received, err := readMessage(remote)
	require.NoError(t, err)
	require.Equal(t, testData, received)
}

func TestAuthHandler_MessageReadWrite_LargeMessage(t *testing.T) {
	local, remote := newMockStreamPair("peer1", "peer2", "/test/1.0.0")

	// 测试大消息
	testData := make([]byte, 10000)
	for i := range testData {
		testData[i] = byte(i % 256)
	}

	// 写入消息
	err := writeMessage(local, testData)
	require.NoError(t, err)

	// 读取消息
	received, err := readMessage(remote)
	require.NoError(t, err)
	require.Equal(t, testData, received)
}

func TestAuthHandler_Callbacks(t *testing.T) {
	host := newMockHost("peer1")
	realmID := "test-realm"
	authKey := []byte("test-auth-key-32-bytes-long!!")
	authenticator := &mockAuthenticator{realmID: realmID, mode: interfaces.AuthModePSK}

	handler := NewAuthHandler(host, realmID, authKey, authenticator, nil)

	// 设置成功回调
	handler.SetOnAuthSuccess(func(peerID string) {
		require.Equal(t, "peer2", peerID)
	})

	// 设置失败回调
	handler.SetOnAuthFailed(func(peerID string, err error) {
		// This will be called on failure
	})

	require.NotNil(t, handler.onAuthSuccess)
	require.NotNil(t, handler.onAuthFailed)
}

func TestAuthHandler_Close(t *testing.T) {
	host := newMockHost("peer1")
	realmID := "test-realm"
	authKey := []byte("test-auth-key-32-bytes-long!!")
	authenticator := &mockAuthenticator{realmID: realmID, mode: interfaces.AuthModePSK}

	handler := NewAuthHandler(host, realmID, authKey, authenticator, nil)

	ctx := context.Background()

	// 启动
	err := handler.Start(ctx)
	require.NoError(t, err)

	// 关闭
	err = handler.Close()
	require.NoError(t, err)
	require.True(t, handler.closed)
	require.False(t, handler.started)

	// 重复关闭应该无错误
	err = handler.Close()
	require.NoError(t, err)
}

// ============================================================================
//                              集成测试
// ============================================================================

func TestAuthHandler_Integration(t *testing.T) {
	// 创建两个节点
	host1 := newMockHost("peer1")
	host2 := newMockHost("peer2")

	realmID := "test-realm"
	psk := []byte("shared-secret-key-32-bytes!!")

	// 派生 authKey
	authKey := auth.DeriveAuthKey(psk, realmID)
	require.NotNil(t, authKey)

	// 创建认证器
	auth1, err := auth.NewPSKAuthenticator(psk, "peer1")
	require.NoError(t, err)
	defer auth1.Close()

	auth2, err := auth.NewPSKAuthenticator(psk, "peer2")
	require.NoError(t, err)
	defer auth2.Close()

	// 创建处理器
	handler1 := NewAuthHandler(host1, realmID, authKey, auth1, nil)
	handler2 := NewAuthHandler(host2, realmID, authKey, auth2, nil)

	ctx := context.Background()

	// 启动处理器
	err = handler1.Start(ctx)
	require.NoError(t, err)
	defer handler1.Stop(ctx)

	err = handler2.Start(ctx)
	require.NoError(t, err)
	defer handler2.Stop(ctx)

	// 设置回调
	var success2 bool
	handler2.SetOnAuthSuccess(func(peerID string) {
		success2 = true
	})

	// peer1 向 peer2 发起认证
	go func() {
		// 从 host1 获取流
		stream := <-host1.streams
		// 使用 host2 的处理器处理
		protocolID := "/dep2p/realm/test-realm/auth/1.0.0"
		handler := host2.getHandler(protocolID)
		if handler != nil {
			handler(stream)
		}
	}()

	// 执行认证
	authCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err = handler1.Authenticate(authCtx, "peer2")
	require.NoError(t, err)

	// 等待回调
	time.Sleep(100 * time.Millisecond)
	require.True(t, success2, "peer2 should have authenticated peer1")
}

func TestAuthHandler_AuthenticateFailed_InvalidKey(t *testing.T) {
	// 创建两个节点，使用不同的 PSK
	host1 := newMockHost("peer1")
	host2 := newMockHost("peer2")

	realmID := "test-realm"
	psk1 := []byte("key1-32-bytes-long-secret-!!")
	psk2 := []byte("key2-32-bytes-long-secret-!!")

	// 派生不同的 authKey
	authKey1 := auth.DeriveAuthKey(psk1, realmID)
	authKey2 := auth.DeriveAuthKey(psk2, realmID)

	// 创建认证器
	auth1, err := auth.NewPSKAuthenticator(psk1, "peer1")
	require.NoError(t, err)
	defer auth1.Close()

	auth2, err := auth.NewPSKAuthenticator(psk2, "peer2")
	require.NoError(t, err)
	defer auth2.Close()

	// 创建处理器
	handler1 := NewAuthHandler(host1, realmID, authKey1, auth1, nil)
	handler2 := NewAuthHandler(host2, realmID, authKey2, auth2, nil)

	ctx := context.Background()

	// 启动处理器
	err = handler1.Start(ctx)
	require.NoError(t, err)
	defer handler1.Stop(ctx)

	err = handler2.Start(ctx)
	require.NoError(t, err)
	defer handler2.Stop(ctx)

	// 设置失败回调
	var failed2 bool
	handler2.SetOnAuthFailed(func(peerID string, err error) {
		failed2 = true
	})

	// peer1 向 peer2 发起认证（应该失败）
	go func() {
		stream := <-host1.streams
		protocolID := "/dep2p/realm/test-realm/auth/1.0.0"
		handler := host2.getHandler(protocolID)
		if handler != nil {
			handler(stream)
		}
	}()

	// 执行认证
	authCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err = handler1.Authenticate(authCtx, "peer2")
	require.Error(t, err, "authentication should fail with different keys")

	// 等待回调
	time.Sleep(100 * time.Millisecond)
	require.True(t, failed2, "peer2 should have failed to authenticate peer1")
}
