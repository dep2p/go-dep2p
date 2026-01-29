package protocol

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/tests/mocks"
)

// TestProtocol_FullNegotiation 测试完整协议协商
func TestProtocol_FullNegotiation(t *testing.T) {
	// 1. 创建 Registry
	registry := NewRegistry()
	require.NotNil(t, registry)

	// 2. 注册协议
	handlerCalled := false
	handler := func(s pkgif.Stream) {
		handlerCalled = true
	}
	err := registry.Register("/test/1.0.0", handler)
	require.NoError(t, err)

	// 3. 创建 Negotiator
	negotiator := NewNegotiator(registry)
	require.NotNil(t, negotiator)

	// 4. 创建 Router
	router := NewRouter(registry, negotiator)
	require.NotNil(t, router)

	// 5. 模拟流协商 - 创建 mock stream 并设置协议
	stream := mocks.NewMockStream()
	stream.ProtocolID = "/test/1.0.0"

	// 6. 路由并验证协议选择正确
	err = router.Route(stream)
	assert.NoError(t, err)
	assert.True(t, handlerCalled, "handler should be called")
}

// TestProtocol_PingProtocol 测试 Ping 协议
func TestProtocol_PingProtocol(t *testing.T) {
	// 1. 创建 Registry
	registry := NewRegistry()

	// 2. 注册 Ping 协议
	pingReceived := false
	pingHandler := func(s pkgif.Stream) {
		pingReceived = true
		// 读取 ping 数据
		buf := make([]byte, 32)
		n, _ := s.Read(buf)
		// 回显
		if n > 0 {
			s.Write(buf[:n])
		}
	}
	err := registry.Register("/dep2p/sys/ping/1.0.0", pingHandler)
	require.NoError(t, err)

	// 3. 创建 Router
	router := NewRouter(registry, NewNegotiator(registry))

	// 4. 创建模拟流并发送 Ping
	pingData := []byte("PING-12345678")
	stream := mocks.NewMockStreamWithData(pingData)
	stream.ProtocolID = "/dep2p/sys/ping/1.0.0"

	// 5. 路由流
	start := time.Now()
	err = router.Route(stream)
	rtt := time.Since(start)

	// 6. 验证
	assert.NoError(t, err)
	assert.True(t, pingReceived, "ping should be received")
	assert.Less(t, rtt, 100*time.Millisecond, "RTT should be fast")

	// 验证回显数据
	assert.Equal(t, pingData, stream.WriteData, "echo should match")
}

// TestProtocol_IdentifyProtocol 测试 Identify 协议
func TestProtocol_IdentifyProtocol(t *testing.T) {
	// 1. 创建 Registry
	registry := NewRegistry()

	// 2. 注册 Identify 协议
	identifyData := []byte(`{"peer_id":"test-peer-123","agent_version":"test/1.0.0"}`)
	identifyHandler := func(s pkgif.Stream) {
		// 发送节点信息
		s.Write(identifyData)
	}
	err := registry.Register("/dep2p/sys/identify/1.0.0", identifyHandler)
	require.NoError(t, err)

	// 3. 创建 Router
	router := NewRouter(registry, NewNegotiator(registry))

	// 4. 创建模拟流
	stream := mocks.NewMockStream()
	stream.ProtocolID = "/dep2p/sys/identify/1.0.0"

	// 5. 路由流
	err = router.Route(stream)
	assert.NoError(t, err)

	// 6. 验证信息正确
	assert.Equal(t, identifyData, stream.WriteData)
}

// TestProtocol_MultipleProtocols 测试多协议协商
func TestProtocol_MultipleProtocols(t *testing.T) {
	registry := NewRegistry()

	// 注册多个协议
	protocols := []string{
		"/protocol/v1",
		"/protocol/v2",
		"/protocol/v3",
	}

	calledProtocol := ""
	for _, p := range protocols {
		proto := p // 捕获变量
		err := registry.Register(pkgif.ProtocolID(proto), func(s pkgif.Stream) {
			calledProtocol = proto
		})
		require.NoError(t, err)
	}

	router := NewRouter(registry, NewNegotiator(registry))

	// 测试每个协议
	for _, proto := range protocols {
		stream := mocks.NewMockStream()
		stream.ProtocolID = proto

		err := router.Route(stream)
		assert.NoError(t, err)
		assert.Equal(t, proto, calledProtocol)
	}
}

// TestProtocol_NegotiationTimeout 测试协商超时
func TestProtocol_NegotiationTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	// 创建一个模拟超时场景
	registry := NewRegistry()
	slowHandler := func(s pkgif.Stream) {
		// 模拟慢处理
		select {
		case <-ctx.Done():
			return
		case <-time.After(100 * time.Millisecond):
			s.Write([]byte("done"))
		}
	}
	err := registry.Register("/slow/1.0.0", slowHandler)
	require.NoError(t, err)

	router := NewRouter(registry, NewNegotiator(registry))

	stream := mocks.NewMockStream()
	stream.ProtocolID = "/slow/1.0.0"

	// 路由流（处理器会因为 context 超时而提前返回）
	err = router.Route(stream)
	assert.NoError(t, err)

	// 等待 context 超时
	<-ctx.Done()
	assert.Equal(t, context.DeadlineExceeded, ctx.Err())
}

// TestProtocol_UnknownProtocol 测试未知协议
func TestProtocol_UnknownProtocol(t *testing.T) {
	registry := NewRegistry()
	router := NewRouter(registry, NewNegotiator(registry))

	// 创建流请求未注册的协议
	stream := mocks.NewMockStream()
	stream.ProtocolID = "/unknown/protocol"

	// 应该返回错误
	err := router.Route(stream)
	assert.Error(t, err)
	assert.Equal(t, ErrNoHandler, err)
}

// TestProtocol_PatternMatching 测试模式匹配
func TestProtocol_PatternMatching(t *testing.T) {
	registry := NewRegistry()
	router := NewRouter(registry, NewNegotiator(registry))

	// 使用模式注册
	matchedProtocol := ""
	err := router.AddRoute("/api/*", func(s pkgif.Stream) {
		matchedProtocol = s.Protocol()
	})
	require.NoError(t, err)

	// 测试匹配
	testCases := []struct {
		protocol    string
		shouldMatch bool
	}{
		{"/api/v1", true},
		{"/api/v2", true},
		{"/api/users", true},
		{"/other/v1", false},
	}

	for _, tc := range testCases {
		stream := mocks.NewMockStream()
		stream.ProtocolID = tc.protocol
		matchedProtocol = ""

		err := router.Route(stream)
		if tc.shouldMatch {
			assert.NoError(t, err, "protocol %s should match", tc.protocol)
			assert.Equal(t, tc.protocol, matchedProtocol)
		} else {
			assert.Error(t, err, "protocol %s should not match", tc.protocol)
		}
	}
}

// TestProtocol_RegisterUnregister 测试注册和注销
func TestProtocol_RegisterUnregister(t *testing.T) {
	registry := NewRegistry()

	handler := func(s pkgif.Stream) {}

	// 注册
	err := registry.Register("/test/1.0.0", handler)
	assert.NoError(t, err)

	// 重复注册应该失败
	err = registry.Register("/test/1.0.0", handler)
	assert.Error(t, err)
	assert.Equal(t, ErrDuplicateProtocol, err)

	// 注销
	err = registry.Unregister("/test/1.0.0")
	assert.NoError(t, err)

	// 注销不存在的协议应该失败
	err = registry.Unregister("/test/1.0.0")
	assert.Error(t, err)
	assert.Equal(t, ErrProtocolNotRegistered, err)

	// 现在可以重新注册
	err = registry.Register("/test/1.0.0", handler)
	assert.NoError(t, err)
}

// TestProtocol_ListProtocols 测试列出协议
func TestProtocol_ListProtocols(t *testing.T) {
	registry := NewRegistry()

	protocols := []string{
		"/protocol/a",
		"/protocol/b",
		"/protocol/c",
	}

	for _, p := range protocols {
		err := registry.Register(pkgif.ProtocolID(p), func(s pkgif.Stream) {})
		require.NoError(t, err)
	}

	listed := registry.Protocols()
	assert.Len(t, listed, len(protocols))

	// 验证所有协议都在列表中
	listedMap := make(map[pkgif.ProtocolID]bool)
	for _, p := range listed {
		listedMap[p] = true
	}
	for _, p := range protocols {
		assert.True(t, listedMap[pkgif.ProtocolID(p)], "protocol %s should be listed", p)
	}
}
