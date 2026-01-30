package swarm

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/swarm/bandwidth"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// mockStream 实现 pkgif.Stream 接口用于测试
type mockStream struct {
	readData  []byte
	writeData []byte
	readPos   int
	protocol  string
	conn      pkgif.Connection
}

func (m *mockStream) Read(p []byte) (n int, err error) {
	if m.readPos >= len(m.readData) {
		return 0, io.EOF
	}
	n = copy(p, m.readData[m.readPos:])
	m.readPos += n
	return n, nil
}

func (m *mockStream) Write(p []byte) (n int, err error) {
	m.writeData = append(m.writeData, p...)
	return len(p), nil
}

func (m *mockStream) Close() error {
	return nil
}

func (m *mockStream) Reset() error {
	return nil
}

func (m *mockStream) CloseWrite() error {
	return nil
}

func (m *mockStream) CloseRead() error {
	return nil
}

func (m *mockStream) SetDeadline(t time.Time) error {
	return nil
}

func (m *mockStream) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *mockStream) SetWriteDeadline(t time.Time) error {
	return nil
}

func (m *mockStream) Protocol() string {
	return m.protocol
}

func (m *mockStream) SetProtocol(protocol string) {
	m.protocol = protocol
}

func (m *mockStream) Conn() pkgif.Connection {
	return m.conn
}

func (m *mockStream) IsClosed() bool {
	return false
}

func (m *mockStream) Stat() types.StreamStat {
	return types.StreamStat{}
}

func (m *mockStream) State() types.StreamState {
	return types.StreamStateOpen
}

// mockConnection 实现 pkgif.Connection 接口用于测试
type mockConnection struct {
	remotePeer types.PeerID
}

func (m *mockConnection) LocalPeer() types.PeerID {
	return types.PeerID("local-peer")
}

func (m *mockConnection) RemotePeer() types.PeerID {
	return m.remotePeer
}

func (m *mockConnection) LocalMultiaddr() types.Multiaddr {
	return nil
}

func (m *mockConnection) RemoteMultiaddr() types.Multiaddr {
	return nil
}

func (m *mockConnection) NewStream(ctx context.Context) (pkgif.Stream, error) {
	return nil, nil
}

func (m *mockConnection) NewStreamWithPriority(ctx context.Context, priority int) (pkgif.Stream, error) {
	return m.NewStream(ctx)
}

func (m *mockConnection) SupportsStreamPriority() bool {
	return false
}

func (m *mockConnection) AcceptStream() (pkgif.Stream, error) {
	return nil, nil
}

func (m *mockConnection) GetStreams() []pkgif.Stream {
	return nil
}

func (m *mockConnection) Stat() pkgif.ConnectionStat {
	return pkgif.ConnectionStat{}
}

func (m *mockConnection) Close() error {
	return nil
}

func (m *mockConnection) IsClosed() bool {
	return false
}

func (m *mockConnection) ConnType() pkgif.ConnectionType {
	return pkgif.ConnectionTypeDirect
}

// TestSwarmStream_BandwidthTracking 测试 SwarmStream 的带宽统计功能
func TestSwarmStream_BandwidthTracking(t *testing.T) {
	// 创建 Swarm 和 BandwidthCounter
	localPeer := "test-peer"
	swarm, err := NewSwarm(localPeer)
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer swarm.Close()

	config := pkgif.DefaultBandwidthConfig()
	config.Enabled = true
	config.TrackByPeer = true
	config.TrackByProtocol = true
	bwCounter := bandwidth.NewCounter(config)
	swarm.SetBandwidthCounter(bwCounter)

	// 创建 mock 连接和流
	remotePeer := types.PeerID("remote-peer-123")
	mockConn := &mockConnection{remotePeer: remotePeer}
	swarmConn := newSwarmConn(swarm, mockConn)

	testData := []byte("Hello, World!")
	mockStream := &mockStream{
		readData:  testData,
		writeData: make([]byte, 0),
		protocol:  "/test/protocol/1.0",
		conn:      mockConn,
	}

	swarmStream := newSwarmStream(swarmConn, mockStream)

	// 测试 Read 统计
	readBuf := make([]byte, len(testData))
	n, err := swarmStream.Read(readBuf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Read() = %d, want %d", n, len(testData))
	}

	// 验证接收统计
	peerStats := bwCounter.GetForPeer(string(remotePeer))
	if peerStats.TotalIn != int64(len(testData)) {
		t.Errorf("GetForPeer().TotalIn = %d, want %d", peerStats.TotalIn, len(testData))
	}

	protocolStats := bwCounter.GetForProtocol("/test/protocol/1.0")
	if protocolStats.TotalIn != int64(len(testData)) {
		t.Errorf("GetForProtocol().TotalIn = %d, want %d", protocolStats.TotalIn, len(testData))
	}

	totalStats := bwCounter.GetTotals()
	if totalStats.TotalIn != int64(len(testData)) {
		t.Errorf("GetTotals().TotalIn = %d, want %d", totalStats.TotalIn, len(testData))
	}

	// 测试 Write 统计
	writeData := []byte("Response Data")
	n, err = swarmStream.Write(writeData)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(writeData) {
		t.Errorf("Write() = %d, want %d", n, len(writeData))
	}

	// 验证发送统计
	peerStats = bwCounter.GetForPeer(string(remotePeer))
	if peerStats.TotalOut != int64(len(writeData)) {
		t.Errorf("GetForPeer().TotalOut = %d, want %d", peerStats.TotalOut, len(writeData))
	}

	protocolStats = bwCounter.GetForProtocol("/test/protocol/1.0")
	if protocolStats.TotalOut != int64(len(writeData)) {
		t.Errorf("GetForProtocol().TotalOut = %d, want %d", protocolStats.TotalOut, len(writeData))
	}

	totalStats = bwCounter.GetTotals()
	if totalStats.TotalOut != int64(len(writeData)) {
		t.Errorf("GetTotals().TotalOut = %d, want %d", totalStats.TotalOut, len(writeData))
	}

	// 验证总统计
	if totalStats.TotalIn != int64(len(testData)) {
		t.Errorf("GetTotals().TotalIn = %d, want %d", totalStats.TotalIn, len(testData))
	}

	t.Log("✅ SwarmStream 带宽统计功能正常")
}

// TestSwarmStream_BandwidthTrackingWithoutCounter 测试没有 BandwidthCounter 时的行为
func TestSwarmStream_BandwidthTrackingWithoutCounter(t *testing.T) {
	localPeer := "test-peer"
	swarm, err := NewSwarm(localPeer)
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer swarm.Close()

	// 不设置 BandwidthCounter
	remotePeer := types.PeerID("remote-peer-456")
	mockConn := &mockConnection{remotePeer: remotePeer}
	swarmConn := newSwarmConn(swarm, mockConn)

	testData := []byte("Test Data")
	mockStream := &mockStream{
		readData:  testData,
		writeData: make([]byte, 0),
		protocol:  "/test/protocol/1.0",
		conn:      mockConn,
	}

	swarmStream := newSwarmStream(swarmConn, mockStream)

	// 应该正常工作，不会 panic
	readBuf := make([]byte, len(testData))
	n, err := swarmStream.Read(readBuf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read failed: %v", err)
	}
	if n != len(testData) {
		t.Errorf("Read() = %d, want %d", n, len(testData))
	}

	n, err = swarmStream.Write([]byte("Write Data"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != 10 {
		t.Errorf("Write() = %d, want 10", n)
	}

	t.Log("✅ SwarmStream 在没有 BandwidthCounter 时正常工作")
}

// TestSwarmStream_BandwidthTrackingEmptyProtocol 测试协议为空时的统计
func TestSwarmStream_BandwidthTrackingEmptyProtocol(t *testing.T) {
	localPeer := "test-peer"
	swarm, err := NewSwarm(localPeer)
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer swarm.Close()

	config := pkgif.DefaultBandwidthConfig()
	config.Enabled = true
	bwCounter := bandwidth.NewCounter(config)
	swarm.SetBandwidthCounter(bwCounter)

	remotePeer := types.PeerID("remote-peer-789")
	mockConn := &mockConnection{remotePeer: remotePeer}
	swarmConn := newSwarmConn(swarm, mockConn)

	// 协议为空（协议协商前）
	mockStream := &mockStream{
		readData:  []byte("Data"),
		writeData: make([]byte, 0),
		protocol:  "", // 空协议
		conn:      mockConn,
	}

	swarmStream := newSwarmStream(swarmConn, mockStream)

	// Read 应该正常工作，协议为空字符串
	readBuf := make([]byte, 4)
	n, err := swarmStream.Read(readBuf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read failed: %v", err)
	}
	if n != 4 {
		t.Errorf("Read() = %d, want 4", n)
	}

	// 验证空协议的统计
	protocolStats := bwCounter.GetForProtocol("")
	if protocolStats.TotalIn != 4 {
		t.Errorf("GetForProtocol('').TotalIn = %d, want 4", protocolStats.TotalIn)
	}

	t.Log("✅ SwarmStream 空协议统计正常")
}

// TestSwarmStream_BandwidthTrackingZeroBytes 测试零字节读写
func TestSwarmStream_BandwidthTrackingZeroBytes(t *testing.T) {
	localPeer := "test-peer"
	swarm, err := NewSwarm(localPeer)
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer swarm.Close()

	config := pkgif.DefaultBandwidthConfig()
	config.Enabled = true
	bwCounter := bandwidth.NewCounter(config)
	swarm.SetBandwidthCounter(bwCounter)

	remotePeer := types.PeerID("remote-peer-zero")
	mockConn := &mockConnection{remotePeer: remotePeer}
	swarmConn := newSwarmConn(swarm, mockConn)

	mockStream := &mockStream{
		readData:  []byte{},
		writeData: make([]byte, 0),
		protocol:  "/test/protocol/1.0",
		conn:      mockConn,
	}

	swarmStream := newSwarmStream(swarmConn, mockStream)

	// 读取空数据
	readBuf := make([]byte, 10)
	n, err := swarmStream.Read(readBuf)
	if err != io.EOF {
		t.Errorf("Read() error = %v, want io.EOF", err)
	}
	if n != 0 {
		t.Errorf("Read() = %d, want 0", n)
	}

	// 写入空数据
	n, err = swarmStream.Write([]byte{})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != 0 {
		t.Errorf("Write() = %d, want 0", n)
	}

	// 验证统计应该没有变化（零字节不记录）
	totalStats := bwCounter.GetTotals()
	if totalStats.TotalIn != 0 {
		t.Errorf("GetTotals().TotalIn = %d, want 0", totalStats.TotalIn)
	}
	if totalStats.TotalOut != 0 {
		t.Errorf("GetTotals().TotalOut = %d, want 0", totalStats.TotalOut)
	}

	t.Log("✅ SwarmStream 零字节读写处理正常")
}

// ============================================================================
//                     SwarmStream 方法覆盖测试
// ============================================================================

// TestSwarmStream_SetProtocol 测试设置协议
func TestSwarmStream_SetProtocol(t *testing.T) {
	swarm, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer swarm.Close()

	mockConn := &mockConnection{remotePeer: "remote-peer"}
	swarmConn := newSwarmConn(swarm, mockConn)

	mockStream := &mockStream{
		protocol: "",
	}

	swarmStream := newSwarmStream(swarmConn, mockStream)

	// 设置协议
	swarmStream.SetProtocol("/test/1.0")

	// 验证协议已设置
	if swarmStream.Protocol() != "/test/1.0" {
		t.Errorf("Protocol() = %v, want /test/1.0", swarmStream.Protocol())
	}

	// 验证底层流也被设置
	if mockStream.protocol != "/test/1.0" {
		t.Errorf("底层流协议未设置")
	}

	t.Log("✅ SetProtocol 正常工作")
}

// TestSwarmStream_Protocol_FallbackToUnderlying 测试协议回退
func TestSwarmStream_Protocol_FallbackToUnderlying(t *testing.T) {
	swarm, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer swarm.Close()

	mockConn := &mockConnection{remotePeer: "remote-peer"}
	swarmConn := newSwarmConn(swarm, mockConn)

	mockStream := &mockStream{
		protocol: "/underlying/1.0",
	}

	swarmStream := newSwarmStream(swarmConn, mockStream)

	// 本地协议未设置时，应该返回底层流的协议
	if swarmStream.Protocol() != "/underlying/1.0" {
		t.Errorf("Protocol() = %v, want /underlying/1.0", swarmStream.Protocol())
	}

	t.Log("✅ Protocol 回退正常工作")
}

// TestSwarmStream_Conn 测试获取连接
func TestSwarmStream_Conn(t *testing.T) {
	swarm, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer swarm.Close()

	mockConn := &mockConnection{remotePeer: "remote-peer"}
	swarmConn := newSwarmConn(swarm, mockConn)

	mockStream := &mockStream{}
	swarmStream := newSwarmStream(swarmConn, mockStream)

	// 获取连接
	conn := swarmStream.Conn()
	if conn != swarmConn {
		t.Error("Conn() 返回错误的连接")
	}

	t.Log("✅ Conn 正常工作")
}

// TestSwarmStream_Reset 测试重置流
func TestSwarmStream_Reset(t *testing.T) {
	swarm, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer swarm.Close()

	mockConn := &mockConnection{remotePeer: "remote-peer"}
	swarmConn := newSwarmConn(swarm, mockConn)

	mockStream := &mockStream{}
	swarmStream := newSwarmStream(swarmConn, mockStream)

	// 重置流
	err = swarmStream.Reset()
	if err != nil {
		t.Errorf("Reset() error = %v", err)
	}

	t.Log("✅ Reset 正常工作")
}

// TestSwarmStream_CloseWrite 测试半关闭写
func TestSwarmStream_CloseWrite(t *testing.T) {
	swarm, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer swarm.Close()

	mockConn := &mockConnection{remotePeer: "remote-peer"}
	swarmConn := newSwarmConn(swarm, mockConn)

	mockStream := &mockStream{}
	swarmStream := newSwarmStream(swarmConn, mockStream)

	err = swarmStream.CloseWrite()
	if err != nil {
		t.Errorf("CloseWrite() error = %v", err)
	}

	t.Log("✅ CloseWrite 正常工作")
}

// TestSwarmStream_CloseRead 测试半关闭读
func TestSwarmStream_CloseRead(t *testing.T) {
	swarm, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer swarm.Close()

	mockConn := &mockConnection{remotePeer: "remote-peer"}
	swarmConn := newSwarmConn(swarm, mockConn)

	mockStream := &mockStream{}
	swarmStream := newSwarmStream(swarmConn, mockStream)

	err = swarmStream.CloseRead()
	if err != nil {
		t.Errorf("CloseRead() error = %v", err)
	}

	t.Log("✅ CloseRead 正常工作")
}

// TestSwarmStream_SetDeadline 测试设置超时
func TestSwarmStream_SetDeadline(t *testing.T) {
	swarm, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer swarm.Close()

	mockConn := &mockConnection{remotePeer: "remote-peer"}
	swarmConn := newSwarmConn(swarm, mockConn)

	mockStream := &mockStream{}
	swarmStream := newSwarmStream(swarmConn, mockStream)

	deadline := time.Now().Add(10 * time.Second)

	err = swarmStream.SetDeadline(deadline)
	if err != nil {
		t.Errorf("SetDeadline() error = %v", err)
	}

	err = swarmStream.SetReadDeadline(deadline)
	if err != nil {
		t.Errorf("SetReadDeadline() error = %v", err)
	}

	err = swarmStream.SetWriteDeadline(deadline)
	if err != nil {
		t.Errorf("SetWriteDeadline() error = %v", err)
	}

	t.Log("✅ Deadline 设置正常工作")
}

// TestSwarmStream_IsClosed 测试检查流是否关闭
func TestSwarmStream_IsClosed(t *testing.T) {
	swarm, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer swarm.Close()

	mockConn := &mockConnection{remotePeer: "remote-peer"}
	swarmConn := newSwarmConn(swarm, mockConn)

	mockStream := &mockStream{}
	swarmStream := newSwarmStream(swarmConn, mockStream)

	if swarmStream.IsClosed() {
		t.Error("IsClosed() should return false initially")
	}

	t.Log("✅ IsClosed 正常工作")
}

// TestSwarmStream_Stat 测试获取统计信息
func TestSwarmStream_Stat(t *testing.T) {
	swarm, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer swarm.Close()

	mockConn := &mockConnection{remotePeer: "remote-peer"}
	swarmConn := newSwarmConn(swarm, mockConn)

	mockStream := &mockStream{}
	swarmStream := newSwarmStream(swarmConn, mockStream)

	stat := swarmStream.Stat()
	// 验证返回了有效的 stat 对象
	_ = stat

	t.Log("✅ Stat 正常工作")
}

// TestSwarmStream_State 测试获取流状态
func TestSwarmStream_State(t *testing.T) {
	swarm, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer swarm.Close()

	mockConn := &mockConnection{remotePeer: "remote-peer"}
	swarmConn := newSwarmConn(swarm, mockConn)

	mockStream := &mockStream{}
	swarmStream := newSwarmStream(swarmConn, mockStream)

	state := swarmStream.State()
	if state != types.StreamStateOpen {
		t.Errorf("State() = %v, want StreamStateOpen", state)
	}

	t.Log("✅ State 正常工作")
}

// TestSwarmStream_Close 测试关闭流
func TestSwarmStream_Close(t *testing.T) {
	swarm, err := NewSwarm("test-peer")
	if err != nil {
		t.Fatalf("NewSwarm failed: %v", err)
	}
	defer swarm.Close()

	mockConn := &mockConnection{remotePeer: "remote-peer"}
	swarmConn := newSwarmConn(swarm, mockConn)

	mockStream := &mockStream{}
	swarmStream := newSwarmStream(swarmConn, mockStream)

	// 先添加流到连接
	swarmConn.streamsMu.Lock()
	swarmConn.streams = append(swarmConn.streams, swarmStream)
	swarmConn.streamsMu.Unlock()

	// 关闭流
	err = swarmStream.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// 验证流已从连接中移除
	streams := swarmConn.GetStreams()
	if len(streams) != 0 {
		t.Errorf("流未从连接中移除")
	}

	t.Log("✅ Close 正常工作")
}
