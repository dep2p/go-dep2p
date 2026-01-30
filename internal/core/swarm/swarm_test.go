package swarm

import (
	"context"
	"sync"
	"testing"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// TestSwarm_New 测试创建 Swarm
func TestSwarm_New(t *testing.T) {
	localPeer := "test-peer"

	s, err := NewSwarm(localPeer)
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	if s.LocalPeer() != localPeer {
		t.Errorf("LocalPeer() = %v, want %v", s.LocalPeer(), localPeer)
	}

	t.Log("✅ Swarm 创建成功")
}

// TestSwarm_LocalPeer 测试获取本地节点 ID
func TestSwarm_LocalPeer(t *testing.T) {
	localPeer := "test-local-peer"
	s, err := NewSwarm(localPeer)
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	got := s.LocalPeer()
	if got != localPeer {
		t.Errorf("LocalPeer() = %v, want %v", got, localPeer)
	}

	t.Log("✅ LocalPeer 正确")
}

// TestSwarm_Peers 测试获取所有已连接节点
func TestSwarm_Peers(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	peers := s.Peers()
	if len(peers) != 0 {
		t.Errorf("Peers() returned %d peers, want 0", len(peers))
	}

	t.Log("✅ Peers 返回空列表")
}

// TestSwarm_Conns 测试获取所有连接
func TestSwarm_Conns(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	conns := s.Conns()
	if len(conns) != 0 {
		t.Errorf("Conns() returned %d connections, want 0", len(conns))
	}

	t.Log("✅ Conns 返回空列表")
}

// TestSwarm_ConnsToPeer 测试查询特定节点连接
func TestSwarm_ConnsToPeer(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	peerID := "remote-peer"
	conns := s.ConnsToPeer(peerID)
	if len(conns) != 0 {
		t.Errorf("ConnsToPeer() returned %d connections, want 0", len(conns))
	}

	t.Log("✅ ConnsToPeer 返回空列表")
}

// TestSwarm_Connectedness 测试连接状态
func TestSwarm_Connectedness(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	peerID := "remote-peer"
	state := s.Connectedness(peerID)
	if state != pkgif.NotConnected {
		t.Errorf("Connectedness() = %v, want NotConnected", state)
	}

	t.Log("✅ Connectedness 返回 NotConnected")
}

// 注意：TestSwarm_Notify 已在 swarm_test.go 前面定义为 TestSwarm_NotifyRegister

// TestSwarm_Close 测试关闭 Swarm
func TestSwarm_Close(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}

	if err := s.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// 关闭后的操作应该失败
	peers := s.Peers()
	if len(peers) != 0 {
		t.Logf("Warning: Peers() after Close() returned %d peers", len(peers))
	}

	t.Log("✅ Close 成功")
}

// TestSwarm_Concurrent 测试并发访问
func TestSwarm_Concurrent(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	done := make(chan bool)

	// 并发读取
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 100; j++ {
				_ = s.LocalPeer()
				_ = s.Peers()
				_ = s.Conns()
			}
			done <- true
		}()
	}

	// 等待完成
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("并发测试超时")
		}
	}

	t.Log("✅ 并发访问安全")
}

// TestNotifier 测试用的通知器（线程安全）
type TestNotifier struct {
	mu           sync.Mutex
	connected    []pkgif.Connection
	disconnected []pkgif.Connection
}

func (n *TestNotifier) Connected(conn pkgif.Connection) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.connected = append(n.connected, conn)
}

func (n *TestNotifier) Disconnected(conn pkgif.Connection) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.disconnected = append(n.disconnected, conn)
}

// ConnectedCount 返回已连接数量（线程安全）
func (n *TestNotifier) ConnectedCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.connected)
}

// DisconnectedCount 返回已断开数量（线程安全）
func (n *TestNotifier) DisconnectedCount() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.disconnected)
}

// ============================================================================
// 补充测试
// ============================================================================

// TestSwarm_New_EmptyPeer 测试空 PeerID 创建
func TestSwarm_New_EmptyPeer(t *testing.T) {
	_, err := NewSwarm("")
	if err == nil {
		t.Error("NewSwarm() should fail for empty peer ID")
	}
	t.Log("✅ 空 PeerID 创建正确失败")
}

// TestSwarm_DialPeer_Self 测试拨号自己
func TestSwarm_DialPeer_Self(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	_, err = s.DialPeer(ctx, "test-peer")
	if err == nil {
		t.Error("DialPeer() should fail when dialing self")
	}
	if err != ErrDialToSelf {
		t.Logf("Expected ErrDialToSelf, got: %v", err)
	}
	t.Log("✅ 拨号自己正确失败")
}

// TestSwarm_NewStream_NoConnection 测试无连接时创建流
func TestSwarm_NewStream_NoConnection(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	_, err = s.NewStream(ctx, "remote-peer")
	if err == nil {
		t.Error("NewStream() should fail without connection")
	}
	if err != ErrNoConnection {
		t.Logf("Expected ErrNoConnection, got: %v", err)
	}
	t.Log("✅ 无连接创建流正确失败")
}

// TestSwarm_ClosePeer 测试关闭特定节点
func TestSwarm_ClosePeer(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	// 关闭一个不存在的节点应该不返回错误
	err = s.ClosePeer("non-existent-peer")
	if err != nil {
		t.Errorf("ClosePeer() error = %v, want nil", err)
	}
	t.Log("✅ 关闭不存在节点返回 nil")
}

// TestSwarm_Notify 测试注册通知器
func TestSwarm_Notify(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	notifier := &TestNotifier{}
	s.Notify(notifier)

	// nil notifier 应该被忽略
	s.Notify(nil)

	t.Log("✅ 注册通知器成功")
}

// TestSwarm_DoubleClose 测试重复关闭
func TestSwarm_DoubleClose(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}

	// 第一次关闭
	err = s.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// 第二次关闭应该返回错误
	err = s.Close()
	if err == nil {
		t.Error("Close() should return error on double close")
	}
	t.Log("✅ 重复关闭正确返回错误")
}

// TestSwarm_DialPeer_Closed 测试关闭后拨号
func TestSwarm_DialPeer_Closed(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	s.Close()

	ctx := context.Background()
	_, err = s.DialPeer(ctx, "remote-peer")
	if err == nil {
		t.Error("DialPeer() should fail after close")
	}
	if err != ErrSwarmClosed {
		t.Logf("Expected ErrSwarmClosed, got: %v", err)
	}
	t.Log("✅ 关闭后拨号正确失败")
}

// TestSwarm_NewStream_Closed 测试关闭后创建流
func TestSwarm_NewStream_Closed(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	s.Close()

	ctx := context.Background()
	_, err = s.NewStream(ctx, "remote-peer")
	if err == nil {
		t.Error("NewStream() should fail after close")
	}
	if err != ErrSwarmClosed {
		t.Logf("Expected ErrSwarmClosed, got: %v", err)
	}
	t.Log("✅ 关闭后创建流正确失败")
}

// TestSwarm_Peers_Closed 测试关闭后获取节点列表
func TestSwarm_Peers_Closed(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	s.Close()

	peers := s.Peers()
	if peers != nil {
		t.Errorf("Peers() should return nil after close, got %v", peers)
	}
	t.Log("✅ 关闭后获取节点列表返回 nil")
}

// TestSwarm_Conns_Closed 测试关闭后获取连接列表
func TestSwarm_Conns_Closed(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	s.Close()

	conns := s.Conns()
	if conns != nil {
		t.Errorf("Conns() should return nil after close, got %v", conns)
	}
	t.Log("✅ 关闭后获取连接列表返回 nil")
}

// TestSwarm_ConnsToPeer_Closed 测试关闭后获取到节点的连接
func TestSwarm_ConnsToPeer_Closed(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	s.Close()

	conns := s.ConnsToPeer("remote-peer")
	if conns != nil {
		t.Errorf("ConnsToPeer() should return nil after close, got %v", conns)
	}
	t.Log("✅ 关闭后获取到节点的连接返回 nil")
}

// TestSwarm_Connectedness_Closed 测试关闭后获取连接状态
func TestSwarm_Connectedness_Closed(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	s.Close()

	state := s.Connectedness("remote-peer")
	if state != pkgif.NotConnected {
		t.Errorf("Connectedness() should return NotConnected after close, got %v", state)
	}
	t.Log("✅ 关闭后获取连接状态返回 NotConnected")
}

// TestSwarm_SetInboundStreamHandler 测试设置入站流处理器
func TestSwarm_SetInboundStreamHandler(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	handler := func(stream pkgif.Stream) {
		// 空处理器
	}

	s.SetInboundStreamHandler(handler)

	// 验证处理器已设置
	got := s.getInboundStreamHandler()
	if got == nil {
		t.Error("InboundStreamHandler should not be nil")
	}
	t.Log("✅ 设置入站流处理器成功")
}

// ============================================================================
// 内部方法测试（提升覆盖率）
// ============================================================================

// testConn 测试用模拟连接（扩展版，支持 closed 状态）
type testConn struct {
	remotePeer types.PeerID
	closed     bool
}

func (m *testConn) LocalPeer() types.PeerID                             { return types.PeerID("local-peer") }
func (m *testConn) RemotePeer() types.PeerID                            { return m.remotePeer }
func (m *testConn) LocalMultiaddr() types.Multiaddr                     { return nil }
func (m *testConn) RemoteMultiaddr() types.Multiaddr                    { return nil }
func (m *testConn) NewStream(ctx context.Context) (pkgif.Stream, error) { return nil, nil }
func (m *testConn) NewStreamWithPriority(ctx context.Context, priority int) (pkgif.Stream, error) {
	return m.NewStream(ctx)
}
func (m *testConn) SupportsStreamPriority() bool        { return false }
func (m *testConn) AcceptStream() (pkgif.Stream, error) { return nil, nil }
func (m *testConn) GetStreams() []pkgif.Stream          { return nil }
func (m *testConn) IsClosed() bool                      { return m.closed }
func (m *testConn) Stat() pkgif.ConnectionStat          { return pkgif.ConnectionStat{} }
func (m *testConn) Close() error {
	m.closed = true
	return nil
}

func (m *testConn) ConnType() pkgif.ConnectionType {
	return pkgif.ConnectionTypeDirect
}

// TestSwarm_addConn 测试添加连接
func TestSwarm_addConn(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	// 测试添加 nil 连接（应该被忽略）
	s.addConn(nil)
	if len(s.Conns()) != 0 {
		t.Error("addConn(nil) should not add connection")
	}

	// 测试添加正常连接
	conn := &testConn{
		remotePeer: "remote-peer-1",
	}
	s.addConn(conn)

	if len(s.Peers()) != 1 {
		t.Errorf("Peers() = %d, want 1", len(s.Peers()))
	}
	if len(s.Conns()) != 1 {
		t.Errorf("Conns() = %d, want 1", len(s.Conns()))
	}

	t.Log("✅ addConn 正常工作")
}

// TestSwarm_addConn_Closed 测试关闭后添加连接
func TestSwarm_addConn_Closed(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	s.Close()

	conn := &testConn{
		remotePeer: "remote-peer-1",
	}
	s.addConn(conn)

	// 连接应该被关闭
	if !conn.closed {
		t.Error("Connection should be closed when added to closed swarm")
	}

	t.Log("✅ addConn 关闭后正确关闭连接")
}

// TestSwarm_removeConn 测试移除连接
func TestSwarm_removeConn(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	// 测试移除 nil 连接（应该被忽略）
	s.removeConn(nil)

	// 添加连接
	conn1 := &testConn{remotePeer: "remote-peer-1"}
	conn2 := &testConn{remotePeer: "remote-peer-1"}
	conn3 := &testConn{remotePeer: "remote-peer-2"}

	s.addConn(conn1)
	s.addConn(conn2)
	s.addConn(conn3)

	if len(s.Peers()) != 2 {
		t.Errorf("Peers() = %d, want 2", len(s.Peers()))
	}

	// 移除第一个连接
	s.removeConn(conn1)

	// 应该还剩 2 个连接（peer1 有 1 个，peer2 有 1 个）
	if len(s.Conns()) != 2 {
		t.Errorf("Conns() = %d, want 2", len(s.Conns()))
	}

	// 移除 peer1 的最后一个连接
	s.removeConn(conn2)

	// 应该还剩 1 个 peer
	if len(s.Peers()) != 1 {
		t.Errorf("Peers() = %d, want 1", len(s.Peers()))
	}

	t.Log("✅ removeConn 正常工作")
}

// TestSwarm_notifyConnected 测试连接通知
func TestSwarm_notifyConnected(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	// 测试通知 nil 连接（应该被忽略）
	s.notifyConnected(nil)

	// 注册通知器
	notifier := &TestNotifier{}
	s.Notify(notifier)

	// 触发连接通知
	conn := &testConn{remotePeer: "remote-peer-1"}
	s.notifyConnected(conn)

	// 等待异步通知完成
	time.Sleep(10 * time.Millisecond)

	if notifier.ConnectedCount() != 1 {
		t.Errorf("connected count = %d, want 1", notifier.ConnectedCount())
	}

	t.Log("✅ notifyConnected 正常工作")
}

// TestSwarm_notifyDisconnected 测试断开通知
func TestSwarm_notifyDisconnected(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	// 测试通知 nil 连接（应该被忽略）
	s.notifyDisconnected(nil)

	// 注册通知器
	notifier := &TestNotifier{}
	s.Notify(notifier)

	// 触发断开通知
	conn := &testConn{remotePeer: "remote-peer-1"}
	s.notifyDisconnected(conn)

	// 等待异步通知完成
	time.Sleep(10 * time.Millisecond)

	if notifier.DisconnectedCount() != 1 {
		t.Errorf("disconnected count = %d, want 1", notifier.DisconnectedCount())
	}

	t.Log("✅ notifyDisconnected 正常工作")
}

// TestSwarm_MultipleConnections 测试多个连接
func TestSwarm_MultipleConnections(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	// 添加多个连接到同一个节点
	peer1Conn1 := &testConn{remotePeer: "peer-1"}
	peer1Conn2 := &testConn{remotePeer: "peer-1"}
	peer2Conn1 := &testConn{remotePeer: "peer-2"}

	s.addConn(peer1Conn1)
	s.addConn(peer1Conn2)
	s.addConn(peer2Conn1)

	// 验证连接数
	if len(s.Conns()) != 3 {
		t.Errorf("Conns() = %d, want 3", len(s.Conns()))
	}

	// 验证到 peer-1 的连接数
	peer1Conns := s.ConnsToPeer("peer-1")
	if len(peer1Conns) != 2 {
		t.Errorf("ConnsToPeer(peer-1) = %d, want 2", len(peer1Conns))
	}

	// 验证连接状态
	if s.Connectedness("peer-1") != pkgif.Connected {
		t.Error("Connectedness(peer-1) should be Connected")
	}

	t.Log("✅ 多连接管理正常工作")
}

// TestSwarm_ClosePeer_WithConnections 测试关闭有连接的节点
func TestSwarm_ClosePeer_WithConnections(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	// 添加连接
	conn1 := &testConn{remotePeer: "remote-peer"}
	conn2 := &testConn{remotePeer: "remote-peer"}
	s.addConn(conn1)
	s.addConn(conn2)

	// 关闭节点
	err = s.ClosePeer("remote-peer")
	if err != nil {
		t.Errorf("ClosePeer() error = %v", err)
	}

	// 验证连接已关闭
	if !conn1.closed || !conn2.closed {
		t.Error("Connections should be closed")
	}

	// 验证节点已移除
	if len(s.Peers()) != 0 {
		t.Errorf("Peers() = %d, want 0", len(s.Peers()))
	}

	t.Log("✅ ClosePeer 关闭有连接的节点成功")
}

// 注意：Listen 相关测试已移至 listen_test.go

// TestSwarm_Close_WithConnections 测试关闭带连接的 Swarm
func TestSwarm_Close_WithConnections(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}

	// 添加连接
	conn1 := &testConn{remotePeer: "peer-1"}
	conn2 := &testConn{remotePeer: "peer-2"}
	s.addConn(conn1)
	s.addConn(conn2)

	// 关闭 Swarm
	err = s.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// 验证连接已关闭
	if !conn1.closed || !conn2.closed {
		t.Error("All connections should be closed")
	}

	t.Log("✅ 关闭带连接的 Swarm 成功")
}

// TestSwarm_Options 测试配置选项
func TestSwarm_Options(t *testing.T) {
	// 测试有效选项
	s, err := NewSwarm("test-peer", WithConfig(DefaultConfig()))
	if err != nil {
		t.Fatalf("NewSwarm with options failed: %v", err)
	}
	defer s.Close()

	t.Log("✅ 配置选项应用成功")
}

// TestSwarm_getBandwidthCounter 测试获取带宽计数器
func TestSwarm_getBandwidthCounter(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	// 默认没有带宽计数器
	counter := s.getBandwidthCounter()
	if counter != nil {
		t.Error("getBandwidthCounter should return nil by default")
	}

	t.Log("✅ getBandwidthCounter 默认返回 nil")
}

// ============================================================================
// SwarmConn 测试
// ============================================================================

// fullMockConn 完整的模拟连接
type fullMockConn struct {
	localPeer    types.PeerID
	remotePeer   types.PeerID
	localAddr    types.Multiaddr
	remoteAddr   types.Multiaddr
	closed       bool
	newStreamErr error
	acceptErr    error
	streams      []pkgif.Stream
}

func (m *fullMockConn) LocalPeer() types.PeerID          { return m.localPeer }
func (m *fullMockConn) RemotePeer() types.PeerID         { return m.remotePeer }
func (m *fullMockConn) LocalMultiaddr() types.Multiaddr  { return m.localAddr }
func (m *fullMockConn) RemoteMultiaddr() types.Multiaddr { return m.remoteAddr }
func (m *fullMockConn) NewStream(ctx context.Context) (pkgif.Stream, error) {
	if m.newStreamErr != nil {
		return nil, m.newStreamErr
	}
	return &fullMockStream{}, nil
}
func (m *fullMockConn) NewStreamWithPriority(ctx context.Context, priority int) (pkgif.Stream, error) {
	return m.NewStream(ctx)
}
func (m *fullMockConn) SupportsStreamPriority() bool {
	return false
}
func (m *fullMockConn) AcceptStream() (pkgif.Stream, error) {
	if m.acceptErr != nil {
		return nil, m.acceptErr
	}
	return &fullMockStream{}, nil
}
func (m *fullMockConn) GetStreams() []pkgif.Stream { return m.streams }
func (m *fullMockConn) IsClosed() bool             { return m.closed }
func (m *fullMockConn) Stat() pkgif.ConnectionStat { return pkgif.ConnectionStat{} }
func (m *fullMockConn) Close() error {
	m.closed = true
	return nil
}

func (m *fullMockConn) ConnType() pkgif.ConnectionType {
	return pkgif.ConnectionTypeDirect
}

// fullMockStream 完整的模拟流
type fullMockStream struct {
	closed   bool
	protocol string
	conn     pkgif.Connection
}

func (m *fullMockStream) Read(p []byte) (int, error)         { return 0, nil }
func (m *fullMockStream) Write(p []byte) (int, error)        { return len(p), nil }
func (m *fullMockStream) Close() error                       { m.closed = true; return nil }
func (m *fullMockStream) Reset() error                       { return nil }
func (m *fullMockStream) CloseRead() error                   { return nil }
func (m *fullMockStream) CloseWrite() error                  { return nil }
func (m *fullMockStream) Conn() pkgif.Connection             { return m.conn }
func (m *fullMockStream) Protocol() string                   { return m.protocol }
func (m *fullMockStream) SetProtocol(p string)               { m.protocol = p }
func (m *fullMockStream) IsClosed() bool                     { return m.closed }
func (m *fullMockStream) Stat() types.StreamStat             { return types.StreamStat{} }
func (m *fullMockStream) State() types.StreamState           { return types.StreamStateOpen }
func (m *fullMockStream) SetDeadline(t time.Time) error      { return nil }
func (m *fullMockStream) SetReadDeadline(t time.Time) error  { return nil }
func (m *fullMockStream) SetWriteDeadline(t time.Time) error { return nil }

// TestSwarmConn_Basic 测试 SwarmConn 基本功能
func TestSwarmConn_Basic(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	mockConn := &fullMockConn{
		localPeer:  "local-peer",
		remotePeer: "remote-peer",
	}

	swarmConn := newSwarmConn(s, mockConn)

	// 测试 LocalPeer
	if swarmConn.LocalPeer() != "local-peer" {
		t.Errorf("LocalPeer() = %v, want local-peer", swarmConn.LocalPeer())
	}

	// 测试 RemotePeer
	if swarmConn.RemotePeer() != "remote-peer" {
		t.Errorf("RemotePeer() = %v, want remote-peer", swarmConn.RemotePeer())
	}

	// 测试 IsClosed
	if swarmConn.IsClosed() {
		t.Error("IsClosed() should return false")
	}

	// 测试 GetStreams
	streams := swarmConn.GetStreams()
	if len(streams) != 0 {
		t.Errorf("GetStreams() = %d, want 0", len(streams))
	}

	// 测试 Stat
	stat := swarmConn.Stat()
	if stat.Direction != 0 {
		t.Log("Stat() returned non-zero direction (OK)")
	}

	t.Log("✅ SwarmConn 基本功能正常")
}

// TestSwarmConn_NewStream 测试 SwarmConn 创建流
func TestSwarmConn_NewStream(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	mockConn := &fullMockConn{
		localPeer:  "local-peer",
		remotePeer: "remote-peer",
	}

	swarmConn := newSwarmConn(s, mockConn)

	ctx := context.Background()
	stream, err := swarmConn.NewStream(ctx)
	if err != nil {
		t.Errorf("NewStream() error = %v", err)
	}
	if stream == nil {
		t.Error("NewStream() returned nil stream")
	}

	// 验证流已记录
	streams := swarmConn.GetStreams()
	if len(streams) != 1 {
		t.Errorf("GetStreams() = %d, want 1", len(streams))
	}

	t.Log("✅ SwarmConn 创建流成功")
}

// TestSwarmConn_NewStream_Closed 测试关闭后创建流
func TestSwarmConn_NewStream_Closed(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	mockConn := &fullMockConn{
		localPeer:  "local-peer",
		remotePeer: "remote-peer",
	}

	swarmConn := newSwarmConn(s, mockConn)
	swarmConn.Close()

	ctx := context.Background()
	_, err = swarmConn.NewStream(ctx)
	if err == nil {
		t.Error("NewStream() should fail after close")
	}

	t.Log("✅ SwarmConn 关闭后创建流正确失败")
}

// TestSwarmConn_AcceptStream 测试 SwarmConn 接受流
func TestSwarmConn_AcceptStream(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	mockConn := &fullMockConn{
		localPeer:  "local-peer",
		remotePeer: "remote-peer",
	}

	swarmConn := newSwarmConn(s, mockConn)

	stream, err := swarmConn.AcceptStream()
	if err != nil {
		t.Errorf("AcceptStream() error = %v", err)
	}
	if stream == nil {
		t.Error("AcceptStream() returned nil stream")
	}

	// 验证流已记录
	streams := swarmConn.GetStreams()
	if len(streams) != 1 {
		t.Errorf("GetStreams() = %d, want 1", len(streams))
	}

	t.Log("✅ SwarmConn 接受流成功")
}

// TestSwarmConn_AcceptStream_Closed 测试关闭后接受流
func TestSwarmConn_AcceptStream_Closed(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	mockConn := &fullMockConn{
		localPeer:  "local-peer",
		remotePeer: "remote-peer",
	}

	swarmConn := newSwarmConn(s, mockConn)
	swarmConn.Close()

	_, err = swarmConn.AcceptStream()
	if err == nil {
		t.Error("AcceptStream() should fail after close")
	}

	t.Log("✅ SwarmConn 关闭后接受流正确失败")
}

// TestSwarmConn_Close 测试 SwarmConn 关闭
func TestSwarmConn_Close(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	mockConn := &fullMockConn{
		localPeer:  "local-peer",
		remotePeer: "remote-peer",
	}

	swarmConn := newSwarmConn(s, mockConn)
	s.addConn(swarmConn)

	// 创建一些流
	ctx := context.Background()
	swarmConn.NewStream(ctx)
	swarmConn.NewStream(ctx)

	// 关闭连接
	err = swarmConn.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// 验证已关闭
	if !swarmConn.IsClosed() {
		t.Error("IsClosed() should return true after close")
	}

	// 重复关闭应该不返回错误
	err = swarmConn.Close()
	if err != nil {
		t.Errorf("Close() should return nil on double close, got %v", err)
	}

	t.Log("✅ SwarmConn 关闭成功")
}

// TestSwarmConn_removeStream 测试移除流
func TestSwarmConn_removeStream(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	mockConn := &fullMockConn{
		localPeer:  "local-peer",
		remotePeer: "remote-peer",
	}

	swarmConn := newSwarmConn(s, mockConn)

	// 创建流
	ctx := context.Background()
	stream1, _ := swarmConn.NewStream(ctx)
	stream2, _ := swarmConn.NewStream(ctx)

	if len(swarmConn.GetStreams()) != 2 {
		t.Errorf("GetStreams() = %d, want 2", len(swarmConn.GetStreams()))
	}

	// 移除第一个流
	swarmConn.removeStream(stream1)

	if len(swarmConn.GetStreams()) != 1 {
		t.Errorf("GetStreams() after remove = %d, want 1", len(swarmConn.GetStreams()))
	}

	// 移除第二个流
	swarmConn.removeStream(stream2)

	if len(swarmConn.GetStreams()) != 0 {
		t.Errorf("GetStreams() after remove all = %d, want 0", len(swarmConn.GetStreams()))
	}

	t.Log("✅ SwarmConn 移除流成功")
}

// TestSwarmConn_Concurrent 测试并发操作
func TestSwarmConn_Concurrent(t *testing.T) {
	s, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer s.Close()

	mockConn := &fullMockConn{
		localPeer:  "local-peer",
		remotePeer: "remote-peer",
	}

	swarmConn := newSwarmConn(s, mockConn)

	done := make(chan bool)
	ctx := context.Background()

	// 并发创建流
	for i := 0; i < 10; i++ {
		go func() {
			for j := 0; j < 10; j++ {
				swarmConn.NewStream(ctx)
				swarmConn.GetStreams()
				swarmConn.IsClosed()
			}
			done <- true
		}()
	}

	// 等待完成
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("并发测试超时")
		}
	}

	t.Log("✅ SwarmConn 并发访问安全")
}
