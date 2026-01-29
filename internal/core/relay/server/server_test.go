package server

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/dep2p/go-dep2p/tests/mocks"
)

// mockLimiter 测试用限制器
type mockLimiter struct {
	canReserve  bool
	canConnect  bool
	reserveFor  time.Duration
	maxCircuits int
}

func (m *mockLimiter) CanReserve(peer types.PeerID) bool {
	return m.canReserve
}

func (m *mockLimiter) CanConnect(src, dst types.PeerID) bool {
	return m.canConnect
}

func (m *mockLimiter) ReserveFor() time.Duration {
	return m.reserveFor
}

func (m *mockLimiter) MaxCircuitsPerPeer() int {
	return m.maxCircuits
}

// mockACL 测试用 ACL
type mockACL struct {
	allowReserve bool
	allowConnect bool
}

func (m *mockACL) AllowReserve(peer types.PeerID) bool {
	return m.allowReserve
}

func (m *mockACL) AllowConnect(src, dst types.PeerID) bool {
	return m.allowConnect
}

// setupTestServer 创建测试用 Server
func setupTestServer(t *testing.T) (*Server, *mocks.MockSwarm) {
	mockSwarm := mocks.NewMockSwarm("relay-server")
	limiter := &mockLimiter{
		canReserve:  true,
		canConnect:  true,
		reserveFor:  time.Hour,
		maxCircuits: 10,
	}

	server := NewServer(mockSwarm, limiter)
	require.NotNil(t, server)

	return server, mockSwarm
}

// TestRelayServer_Creation 测试服务端创建
func TestRelayServer_Creation(t *testing.T) {
	server, mockSwarm := setupTestServer(t)

	assert.NotNil(t, server)
	assert.NotNil(t, server.swarm)
	assert.Equal(t, mockSwarm, server.swarm)
	assert.NotNil(t, server.reservations)
	assert.NotNil(t, server.circuits)
}

// TestRelayServer_HandleReserve 测试处理预约请求
func TestRelayServer_HandleReserve(t *testing.T) {
	server, _ := setupTestServer(t)

	// 设置 ACL 允许预约
	server.acl = &mockACL{allowReserve: true, allowConnect: true}

	// 创建 mock stream
	stream := mocks.NewMockStream()
	stream.ProtocolID = HopProtocolID

	// 设置连接信息
	conn := mocks.NewMockConnection(types.PeerID("client-peer"), types.PeerID("relay-server"))
	stream.ConnValue = conn

	// 准备 RESERVE 消息
	// 格式: [type(1)][length(4)][data]
	msg := make([]byte, 5)
	msg[0] = MsgTypeReserve
	// length = 0 (RESERVE 不需要额外数据)
	stream.ReadData = msg

	// 处理请求
	server.HandleHop(stream)

	// 验证写入了响应 - 格式: [type(1)][length(4)][status(1)]
	assert.NotEmpty(t, stream.WriteData)
	assert.GreaterOrEqual(t, len(stream.WriteData), 6)
	assert.Equal(t, byte(MsgTypeStatus), stream.WriteData[0])
}

// TestRelayServer_HandleConnect 测试处理连接请求
func TestRelayServer_HandleConnect(t *testing.T) {
	server, mockSwarm := setupTestServer(t)

	// 设置 ACL 允许连接
	server.acl = &mockACL{allowReserve: true, allowConnect: true}

	// 先创建预约
	clientPeer := types.PeerID("client-peer")
	targetPeer := types.PeerID("target-peer")
	server.mu.Lock()
	server.reservations[clientPeer] = time.Now().Add(time.Hour)
	server.reservations[targetPeer] = time.Now().Add(time.Hour)
	server.mu.Unlock()

	// 模拟目标节点已连接
	targetConn := mocks.NewMockConnection(types.PeerID("relay-server"), targetPeer)
	mockSwarm.AddConnection(string(targetPeer), targetConn)

	// 创建 mock stream
	stream := mocks.NewMockStream()
	stream.ProtocolID = HopProtocolID

	// 设置连接信息
	conn := mocks.NewMockConnection(clientPeer, types.PeerID("relay-server"))
	stream.ConnValue = conn

	// 准备 CONNECT 消息
	// 格式: [type(1)][length(4)][target]
	targetBytes := []byte(targetPeer)
	msg := make([]byte, 5+len(targetBytes))
	msg[0] = MsgTypeConnect
	msg[1] = 0
	msg[2] = 0
	msg[3] = 0
	msg[4] = byte(len(targetBytes))
	copy(msg[5:], targetBytes)
	stream.ReadData = msg

	// 处理请求 - 由于需要完整的双向转发，这里只验证不 panic
	// 实际连接转发需要更复杂的 mock 设置
	server.HandleHop(stream)
}

// TestRelayServer_RealmAuth 测试 Realm 认证
func TestRelayServer_RealmAuth(t *testing.T) {
	server, _ := setupTestServer(t)

	// 设置 ACL 拒绝预约
	server.acl = &mockACL{allowReserve: false, allowConnect: false}

	// 创建 mock stream
	stream := mocks.NewMockStream()
	stream.ProtocolID = HopProtocolID

	// 设置连接信息
	conn := mocks.NewMockConnection(types.PeerID("unauthorized-peer"), types.PeerID("relay-server"))
	stream.ConnValue = conn

	// 准备 RESERVE 消息
	// 格式: [type(1)][length(4)][data]
	msg := make([]byte, 5)
	msg[0] = MsgTypeReserve
	stream.ReadData = msg

	// 处理请求
	server.HandleHop(stream)

	// 验证写入了拒绝响应（StatusPermissionDenied）
	// 响应格式: [type(1)][length(4)][status(1)]
	assert.NotEmpty(t, stream.WriteData)
	assert.GreaterOrEqual(t, len(stream.WriteData), 6)
	assert.Equal(t, byte(MsgTypeStatus), stream.WriteData[0])
	// 状态码在第 6 个字节 (index 5)
	assert.Equal(t, byte(StatusPermissionDenied), stream.WriteData[5])
}

// TestRelayServer_ReservationExpiry 测试预约过期
func TestRelayServer_ReservationExpiry(t *testing.T) {
	server, _ := setupTestServer(t)

	// 添加一个过期的预约
	expiredPeer := types.PeerID("expired-peer")
	server.mu.Lock()
	server.reservations[expiredPeer] = time.Now().Add(-time.Hour) // 1小时前过期
	server.mu.Unlock()

	// 添加一个有效的预约
	validPeer := types.PeerID("valid-peer")
	server.mu.Lock()
	server.reservations[validPeer] = time.Now().Add(time.Hour) // 1小时后过期
	server.mu.Unlock()

	// 运行 GC
	server.cleanExpiredReservations()

	// 验证过期预约被清理
	server.mu.RLock()
	_, hasExpired := server.reservations[expiredPeer]
	_, hasValid := server.reservations[validPeer]
	server.mu.RUnlock()

	assert.False(t, hasExpired, "过期预约应该被清理")
	assert.True(t, hasValid, "有效预约应该保留")
}

// TestRelayServer_Stats 测试统计功能
func TestRelayServer_Stats(t *testing.T) {
	server, _ := setupTestServer(t)

	// 添加一些预约和电路
	server.mu.Lock()
	server.reservations[types.PeerID("peer1")] = time.Now().Add(time.Hour)
	server.reservations[types.PeerID("peer2")] = time.Now().Add(time.Hour)
	server.circuits[types.PeerID("peer1")] = 2
	server.circuits[types.PeerID("peer2")] = 3
	server.mu.Unlock()

	// 获取统计
	stats := server.Stats()

	assert.Equal(t, 2, stats.ActiveReservations)
	assert.Equal(t, 5, stats.TotalCircuits)
	assert.Equal(t, 2, stats.UniqueRelayedPeers)
}

// TestRelayServer_HandleConnect_BUG33 测试
//
// 根据 Circuit Relay v2 协议：
// - 只有目标节点（被动方）需要预约
// - 发起方不需要预约
//
// 之前的实现错误地检查了发起方的预约，导致主动方无法连接到被动方。
func TestRelayServer_HandleConnect_BUG33(t *testing.T) {
	server, mockSwarm := setupTestServer(t)

	// 设置 ACL 允许连接
	server.acl = &mockACL{allowReserve: true, allowConnect: true}

	// ★
	clientPeer := types.PeerID("initiator-no-reservation")
	targetPeer := types.PeerID("target-with-reservation")
	server.mu.Lock()
	// 注意：clientPeer 没有预约！
	server.reservations[targetPeer] = time.Now().Add(time.Hour)
	server.mu.Unlock()

	// 模拟目标节点已连接
	targetConn := mocks.NewMockConnection(types.PeerID("relay-server"), targetPeer)
	mockSwarm.AddConnection(string(targetPeer), targetConn)

	// 创建 mock stream（发起方的请求）
	stream := mocks.NewMockStream()
	stream.ProtocolID = HopProtocolID

	// 设置连接信息（发起方连接到 Relay）
	conn := mocks.NewMockConnection(clientPeer, types.PeerID("relay-server"))
	stream.ConnValue = conn

	// 准备 CONNECT 消息
	// 格式: [type(1)][length(4)][target]
	targetBytes := []byte(targetPeer)
	msg := make([]byte, 5+len(targetBytes))
	msg[0] = MsgTypeConnect
	msg[1] = 0
	msg[2] = 0
	msg[3] = 0
	msg[4] = byte(len(targetBytes))
	copy(msg[5:], targetBytes)
	stream.ReadData = msg

	// 处理请求
	server.HandleHop(stream)

	// 验证：
	// 响应格式: [type(1)][length(4)][status(1)]
	assert.NotEmpty(t, stream.WriteData)
	assert.GreaterOrEqual(t, len(stream.WriteData), 6)
	assert.Equal(t, byte(MsgTypeStatus), stream.WriteData[0])

	// 验证状态码不是 StatusNoReservation
	// 注意：由于需要完整的双向转发，实际连接可能因为其他原因失败
	// 但关键是不应该因为发起方没有预约而被拒绝
	status := stream.WriteData[5]
	assert.NotEqual(t, byte(StatusNoReservation), status,
		"发起方没有预约时不应该返回 StatusNoReservation")

	t.Log("✅ 发起方不需要预约，CONNECT 请求不会因为发起方没有预约而被拒绝")
}

// TestRelayServer_LimiterDeny 测试限制器拒绝
func TestRelayServer_LimiterDeny(t *testing.T) {
	mockSwarm := mocks.NewMockSwarm("relay-server")
	limiter := &mockLimiter{
		canReserve:  false, // 拒绝预约
		canConnect:  false,
		reserveFor:  time.Hour,
		maxCircuits: 10,
	}

	server := NewServer(mockSwarm, limiter)
	server.acl = &mockACL{allowReserve: true, allowConnect: true}

	// 创建 mock stream
	stream := mocks.NewMockStream()
	stream.ProtocolID = HopProtocolID

	conn := mocks.NewMockConnection(types.PeerID("client"), types.PeerID("relay-server"))
	stream.ConnValue = conn

	// 准备 RESERVE 消息
	// 格式: [type(1)][length(4)][data]
	msg := make([]byte, 5)
	msg[0] = MsgTypeReserve
	stream.ReadData = msg

	// 处理请求
	server.HandleHop(stream)

	// 验证写入了资源限制拒绝响应
	// 响应格式: [type(1)][length(4)][status(1)]
	assert.NotEmpty(t, stream.WriteData)
	assert.GreaterOrEqual(t, len(stream.WriteData), 6)
	assert.Equal(t, byte(MsgTypeStatus), stream.WriteData[0])
	// 状态码在第 6 个字节 (index 5)
	assert.Equal(t, byte(StatusResourceLimitExceeded), stream.WriteData[5])
}
