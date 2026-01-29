package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/dep2p/go-dep2p/tests/mocks"
)

// TestHost_SetStreamHandler_Integration 测试协议处理器注册
func TestHost_SetStreamHandler_Integration(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	handlerCalled := false
	handler := func(s pkgif.Stream) {
		handlerCalled = true
	}

	// 注册协议处理器
	host.SetStreamHandler("/test/1.0.0", handler)

	// 验证 mux 中有该协议
	protocols := host.mux.Protocols()
	assert.Contains(t, protocols, "/test/1.0.0")

	// handlerCalled 只在实际有入站流时才为 true
	// 此测试只验证注册功能，不触发入站流
	assert.False(t, handlerCalled, "Handler should not be called without inbound stream")
}

// TestHost_RemoveStreamHandler_Integration 测试移除协议处理器
func TestHost_RemoveStreamHandler_Integration(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	handler := func(s pkgif.Stream) {}

	// 注册协议处理器
	host.SetStreamHandler("/test/1.0.0", handler)
	protocols := host.mux.Protocols()
	assert.Contains(t, protocols, "/test/1.0.0")

	// 移除协议处理器
	host.RemoveStreamHandler("/test/1.0.0")

	// 验证 mux 中不再有该协议
	protocols = host.mux.Protocols()
	assert.NotContains(t, protocols, "/test/1.0.0")
}

// TestHost_MultipleProtocols 测试多个协议
func TestHost_MultipleProtocols(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	handler1 := func(s pkgif.Stream) {}
	handler2 := func(s pkgif.Stream) {}
	handler3 := func(s pkgif.Stream) {}

	// 注册多个协议
	host.SetStreamHandler("/proto1/1.0.0", handler1)
	host.SetStreamHandler("/proto2/1.0.0", handler2)
	host.SetStreamHandler("/proto3/1.0.0", handler3)

	// 验证所有协议都已注册
	protocols := host.mux.Protocols()
	assert.Contains(t, protocols, "/proto1/1.0.0")
	assert.Contains(t, protocols, "/proto2/1.0.0")
	assert.Contains(t, protocols, "/proto3/1.0.0")
	assert.Len(t, protocols, 3)
}

// TestHost_ProtocolNegotiation 测试协议协商
func TestHost_ProtocolNegotiation(t *testing.T) {
	host, mockSwarm, _, _ := setupTestHost(t)
	defer host.Close()

	// 注册协议
	handler := func(s pkgif.Stream) {}
	host.SetStreamHandler("/test/1.0.0", handler)

	// 配置 mock swarm 返回连接
	targetPeer := "peer-123"
	localPeer := host.ID()
	mockConn := mocks.NewMockConnection(types.PeerID(localPeer), types.PeerID(targetPeer))
	mockSwarm.AddConnection(targetPeer, mockConn)

	// 测试协议是否在 mux 中注册
	protocols := host.mux.Protocols()
	assert.Contains(t, protocols, "/test/1.0.0")

	// 注意：完整的协议协商需要真实的流和 multistream-select
	// 这里验证设置正确
}

// TestHost_UnsupportedProtocol 测试不支持的协议
func TestHost_UnsupportedProtocol(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	// 只注册一个协议
	handler := func(s pkgif.Stream) {}
	host.SetStreamHandler("/supported/1.0.0", handler)

	// 验证未注册的协议不在列表中
	protocols := host.mux.Protocols()
	assert.Contains(t, protocols, "/supported/1.0.0")
	assert.NotContains(t, protocols, "/unsupported/1.0.0")
}

// TestHost_ProtocolOverwrite 测试协议处理器覆盖
func TestHost_ProtocolOverwrite(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	handler1Called := false
	handler2Called := false

	handler1 := func(s pkgif.Stream) { handler1Called = true }
	handler2 := func(s pkgif.Stream) { handler2Called = true }

	// 注册第一个处理器
	host.SetStreamHandler("/test/1.0.0", handler1)

	// 用第二个处理器覆盖
	host.SetStreamHandler("/test/1.0.0", handler2)

	// 验证协议仍然存在
	protocols := host.mux.Protocols()
	assert.Contains(t, protocols, "/test/1.0.0")

	// 验证只有一个协议条目（覆盖而非累加）
	count := 0
	for _, p := range protocols {
		if p == "/test/1.0.0" {
			count++
		}
	}
	assert.Equal(t, 1, count, "Protocol should be registered once (overwrite)")

	// 验证 handler 变量未被调用（无入站流）
	assert.False(t, handler1Called, "Handler1 should not be called without inbound stream")
	assert.False(t, handler2Called, "Handler2 should not be called without inbound stream")
}

// TestHost_ProtocolVersioning 测试协议版本
func TestHost_ProtocolVersioning(t *testing.T) {
	host, _, _, _ := setupTestHost(t)
	defer host.Close()

	handler := func(s pkgif.Stream) {}

	// 注册多个版本
	host.SetStreamHandler("/test/1.0.0", handler)
	host.SetStreamHandler("/test/1.1.0", handler)
	host.SetStreamHandler("/test/2.0.0", handler)

	protocols := host.mux.Protocols()
	assert.Contains(t, protocols, "/test/1.0.0")
	assert.Contains(t, protocols, "/test/1.1.0")
	assert.Contains(t, protocols, "/test/2.0.0")
}

// TestHost_MuxNotNil 测试 mux 初始化
func TestHost_MuxNotNil(t *testing.T) {
	mockSwarm := mocks.NewMockSwarm("test-peer-id")
	mockPeerstore := mocks.NewMockPeerstore()
	mockEventBus := mocks.NewMockEventBus()

	host, err := New(
		WithSwarm(mockSwarm),
		WithPeerstore(mockPeerstore),
		WithEventBus(mockEventBus),
	)
	require.NoError(t, err)
	defer host.Close()

	// 验证 mux 已初始化
	assert.NotNil(t, host.mux)
}
