package client

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/dep2p/go-dep2p/tests/mocks"
)

// TestRelayClient_Creation 测试客户端创建
func TestRelayClient_Creation(t *testing.T) {
	swarm := mocks.NewMockSwarm("local-peer")
	relayPeer := types.PeerID("relay-peer")
	relayAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")

	client := NewClient(swarm, relayPeer, relayAddr)
	require.NotNil(t, client)

	assert.Equal(t, relayPeer, client.relayPeer)
	assert.Equal(t, relayAddr, client.relayAddr)
	assert.False(t, client.closed)

	t.Log("✅ RelayClient 创建成功")
}

// TestRelayClient_Close 测试关闭客户端
func TestRelayClient_Close(t *testing.T) {
	swarm := mocks.NewMockSwarm("local-peer")
	relayPeer := types.PeerID("relay-peer")
	relayAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")

	client := NewClient(swarm, relayPeer, relayAddr)
	require.NotNil(t, client)

	// 关闭客户端
	err := client.Close()
	assert.NoError(t, err)

	// 验证已关闭
	assert.True(t, client.closed)

	t.Log("✅ RelayClient 关闭成功")
}

// TestRelayClient_Reserve_Closed 测试已关闭时预约
func TestRelayClient_Reserve_Closed(t *testing.T) {
	swarm := mocks.NewMockSwarm("local-peer")
	relayPeer := types.PeerID("relay-peer")
	relayAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")

	client := NewClient(swarm, relayPeer, relayAddr)
	_ = client.Close()

	ctx := context.Background()
	_, err := client.Reserve(ctx)
	assert.Error(t, err)
	assert.Equal(t, ErrClientClosed, err)

	t.Log("✅ 已关闭时预约返回错误")
}

// TestRelayClient_Connect_Closed 测试已关闭时连接
func TestRelayClient_Connect_Closed(t *testing.T) {
	swarm := mocks.NewMockSwarm("local-peer")
	relayPeer := types.PeerID("relay-peer")
	relayAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")

	client := NewClient(swarm, relayPeer, relayAddr)
	_ = client.Close()

	ctx := context.Background()
	_, err := client.Connect(ctx, types.PeerID("target-peer"))
	assert.Error(t, err)
	assert.Equal(t, ErrClientClosed, err)

	t.Log("✅ 已关闭时连接返回错误")
}

// TestRelayClient_StateTransition 测试状态转换
func TestRelayClient_StateTransition(t *testing.T) {
	swarm := mocks.NewMockSwarm("local-peer")
	relayPeer := types.PeerID("relay-peer")
	relayAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")

	client := NewClient(swarm, relayPeer, relayAddr)
	require.NotNil(t, client)

	// 初始状态：无连接、无预约
	assert.Nil(t, client.conn)
	assert.Nil(t, client.reservation)
	assert.False(t, client.closed)

	// 关闭后状态
	_ = client.Close()
	assert.True(t, client.closed)

	t.Log("✅ 状态转换正确")
}

// TestRelayClient_MessageTypes 测试消息类型常量
func TestRelayClient_MessageTypes(t *testing.T) {
	assert.Equal(t, byte(0), byte(MsgTypeReserve))
	assert.Equal(t, byte(1), byte(MsgTypeConnect))
	assert.Equal(t, byte(2), byte(MsgTypeStatus))

	t.Log("✅ 消息类型常量正确")
}

// TestRelayClient_StatusCodes 测试状态码常量
func TestRelayClient_StatusCodes(t *testing.T) {
	assert.Equal(t, byte(0), byte(StatusOK))
	assert.Equal(t, byte(1), byte(StatusPermissionDenied))
	assert.Equal(t, byte(2), byte(StatusNoReservation))
	assert.Equal(t, byte(3), byte(StatusResourceLimitExceeded))
	assert.Equal(t, byte(4), byte(StatusMalformedMessage))
	assert.Equal(t, byte(5), byte(StatusUnexpectedMessage))

	t.Log("✅ 状态码常量正确")
}

// TestReservation 测试预约信息结构
func TestReservation(t *testing.T) {
	relayPeer := types.PeerID("relay-peer")
	expireTime := time.Now().Add(time.Hour)

	reservation := &Reservation{
		RelayPeer:  relayPeer,
		ExpireTime: expireTime,
	}

	assert.Equal(t, relayPeer, reservation.RelayPeer)
	assert.Equal(t, expireTime, reservation.ExpireTime)

	t.Log("✅ Reservation 结构正确")
}

// TestProtocolIDs 测试协议 ID 常量
func TestProtocolIDs(t *testing.T) {
	assert.Equal(t, "/dep2p/relay/1.0.0/hop", HopProtocolID)
	assert.Equal(t, "/dep2p/relay/1.0.0/stop", StopProtocolID)

	t.Log("✅ 协议 ID 正确")
}

// ============================================================================
//
// ============================================================================

// TestRelayClient_ConnectAsInitiator_Closed 测试已关闭时的 ConnectAsInitiator
func TestRelayClient_ConnectAsInitiator_Closed(t *testing.T) {
	swarm := mocks.NewMockSwarm("local-peer")
	relayPeer := types.PeerID("relay-peer")
	relayAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")

	client := NewClient(swarm, relayPeer, relayAddr)
	_ = client.Close()

	ctx := context.Background()
	_, err := client.ConnectAsInitiator(ctx, types.PeerID("target-peer"))
	assert.Error(t, err)
	assert.Equal(t, ErrClientClosed, err)

	t.Log("✅ 已关闭时 ConnectAsInitiator 返回错误")
}

// TestRelayClient_ConnectAsInitiator_NoReservationRequired 测试不需要 reservation
//
// 根据 Circuit Relay v2 协议，主动方不需要 reservation。
// ConnectAsInitiator 不应该检查 reservation。
func TestRelayClient_ConnectAsInitiator_NoReservationRequired(t *testing.T) {
	swarm := mocks.NewMockSwarm("local-peer")
	relayPeer := types.PeerID("relay-peer")
	relayAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")

	client := NewClient(swarm, relayPeer, relayAddr)
	require.NotNil(t, client)

	// 验证没有 reservation
	assert.Nil(t, client.reservation)

	// ConnectAsInitiator 不应该因为没有 reservation 而立即失败
	// 它应该尝试连接到 Relay 服务器
	// 这里我们只验证不会因 ErrNoReservation 失败
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.ConnectAsInitiator(ctx, types.PeerID("target-peer"))
	// 可能会因为其他原因失败（如连接超时），但不应该是 ErrNoReservation
	if err != nil {
		assert.NotEqual(t, ErrNoReservation, err, "ConnectAsInitiator 不应该检查 reservation")
	}

	t.Log("✅ ConnectAsInitiator 不检查 reservation")
}

// TestRelayClient_Connect_RequiresReservation 测试 Connect 仍然需要 reservation
func TestRelayClient_Connect_RequiresReservation(t *testing.T) {
	swarm := mocks.NewMockSwarm("local-peer")
	relayPeer := types.PeerID("relay-peer")
	relayAddr, _ := types.NewMultiaddr("/ip4/127.0.0.1/tcp/4001")

	client := NewClient(swarm, relayPeer, relayAddr)
	require.NotNil(t, client)

	// 没有 reservation
	assert.Nil(t, client.reservation)

	ctx := context.Background()
	_, err := client.Connect(ctx, types.PeerID("target-peer"))

	// Connect 应该因为没有 reservation 而失败
	assert.Error(t, err)
	assert.Equal(t, ErrNoReservation, err, "Connect 应该检查 reservation")

	t.Log("✅ Connect 仍然检查 reservation（被动方语义）")
}
