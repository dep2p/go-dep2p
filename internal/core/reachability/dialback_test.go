// Package reachability dial-back 测试
package reachability

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	endpointif "github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	reachabilityif "github.com/dep2p/go-dep2p/pkg/interfaces/reachability"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              消息结构测试
// ============================================================================

func TestDialBackRequest(t *testing.T) {
	req := &reachabilityif.DialBackRequest{
		Addrs:     []string{"/ip4/1.2.3.4/udp/4001/quic-v1", "/ip4/5.6.7.8/udp/4002/quic-v1"},
		Nonce:     []byte("test-nonce-12345"),
		TimeoutMs: 10000,
	}

	assert.Equal(t, 2, len(req.Addrs))
	assert.Equal(t, "/ip4/1.2.3.4/udp/4001/quic-v1", req.Addrs[0])
	assert.Equal(t, int64(10000), req.TimeoutMs)
	assert.Equal(t, "test-nonce-12345", string(req.Nonce))
}

func TestDialBackResponse(t *testing.T) {
	resp := &reachabilityif.DialBackResponse{
		Reachable: []string{"/ip4/1.2.3.4/udp/4001/quic-v1"},
		Nonce:     []byte("test-nonce-12345"),
		DialResults: []reachabilityif.DialResult{
			{Addr: "/ip4/1.2.3.4/udp/4001/quic-v1", Success: true, LatencyMs: 50},
			{Addr: "/ip4/5.6.7.8/udp/4002/quic-v1", Success: false, Error: "connection refused"},
		},
	}

	assert.Equal(t, 1, len(resp.Reachable))
	assert.Equal(t, 2, len(resp.DialResults))
	assert.True(t, resp.DialResults[0].Success)
	assert.False(t, resp.DialResults[1].Success)
}

// ============================================================================
//                              配置测试
// ============================================================================

func TestDefaultConfig(t *testing.T) {
	config := reachabilityif.DefaultConfig()

	assert.Equal(t, reachabilityif.DefaultDialBackTimeout, config.DialBackTimeout)
	assert.Equal(t, reachabilityif.DefaultRequestTimeout, config.RequestTimeout)
	assert.Equal(t, reachabilityif.MaxConcurrentDialBacks, config.MaxConcurrentDialBacks)
	assert.Equal(t, 1, config.MinVerifications)
	assert.True(t, config.EnableAsHelper)
}

// ============================================================================
//                              multiaddr 解析测试（已移除）
// ============================================================================
//
// 说明：
// Phase 3 采用“Handshake 判定”，dial-back 不再做简化的 multiaddr->host:port 解析与 UDP 探测；
// 候选地址以字符串形式透传给 endpoint 的 VerifyOutboundHandshake，由 transport/security 层处理解析与握手。

// ============================================================================
//                              DialBackService 测试
// ============================================================================

func TestNewDialBackService(t *testing.T) {
	t.Run("默认配置", func(t *testing.T) {
		svc := NewDialBackService(nil, nil)
		require.NotNil(t, svc)
		assert.NotNil(t, svc.config)
		assert.NotNil(t, svc.results)
	})

	t.Run("自定义配置", func(t *testing.T) {
		config := &reachabilityif.Config{
			DialBackTimeout: 5 * time.Second,
			RequestTimeout:  15 * time.Second,
		}
		svc := NewDialBackService(nil, config)
		require.NotNil(t, svc)
		assert.Equal(t, 5*time.Second, svc.config.DialBackTimeout)
	})
}

func TestDialBackService_StartStop(t *testing.T) {
	svc := NewDialBackService(nil, nil)

	// 启动
	err := svc.Start(context.Background())
	require.NoError(t, err)

	// 重复启动应该无影响
	err = svc.Start(context.Background())
	require.NoError(t, err)

	// 停止
	err = svc.Stop()
	require.NoError(t, err)

	// 重复停止应该无影响
	err = svc.Stop()
	require.NoError(t, err)
}

func TestDialBackService_HandleDialBackRequest(t *testing.T) {
	svc := NewDialBackService(nil, nil)
	svc.Start(context.Background())
	defer svc.Stop()

	t.Run("空地址列表", func(t *testing.T) {
		req := &reachabilityif.DialBackRequest{
			Addrs: []string{},
			Nonce: []byte("test"),
		}
		resp := svc.HandleDialBackRequest(context.Background(), req)
		assert.Equal(t, []byte("test"), resp.Nonce)
		assert.Empty(t, resp.Reachable)
	})

	t.Run("无效地址", func(t *testing.T) {
		req := &reachabilityif.DialBackRequest{
			Addrs: []string{"/invalid/address"},
			Nonce: []byte("test"),
		}
		resp := svc.HandleDialBackRequest(context.Background(), req)
		assert.Empty(t, resp.Reachable)
		assert.Equal(t, 1, len(resp.DialResults))
		assert.False(t, resp.DialResults[0].Success)
	})

	t.Run("不可达地址", func(t *testing.T) {
		// 使用一个不存在的地址
		req := &reachabilityif.DialBackRequest{
			Addrs:     []string{"/ip4/192.0.2.1/udp/12345/quic-v1"}, // TEST-NET-1，不可路由
			Nonce:     []byte("test"),
			TimeoutMs: 100, // 短超时
		}
		resp := svc.HandleDialBackRequest(context.Background(), req)
		// UDP "连接" 通常不会立即失败，但探测包不会有回应
		// 这里主要测试流程是否正确执行
		assert.NotNil(t, resp)
		assert.Equal(t, []byte("test"), resp.Nonce)
	})
}

func TestDialBackService_VerifyAddresses_Stopped(t *testing.T) {
	svc := NewDialBackService(nil, nil)
	// 不启动服务

	_, err := svc.VerifyAddresses(context.Background(), [32]byte{}, nil)
	assert.ErrorIs(t, err, ErrServiceStopped)
}

func TestDialBackService_VerifyAddresses_NeverClosesConn(t *testing.T) {
	// 覆盖 ISSUE-2025-12-26-001 的根因：dial-back requester 侧不应关闭任何连接，
	// 无论是复用的还是新建的。连接生命周期由 Endpoint 统一管理。
	//
	// 修复策略：
	// 1. 复用的连接：承载其他业务流，关闭会导致 muxer closed 级联断连
	// 2. 新建的连接：由 Endpoint 的 connManager 通过空闲超时/水位线统一管理
	// 3. 避免 TOCTOU 竞态：不再检查"调用前是否存在连接"

	t.Run("已存在连接时不关闭", func(t *testing.T) {
	ep := &fakeDialBackEndpoint{}
	sharedConn := &fakeDialBackConn{}
	ep.existing = sharedConn
	ep.connectReturns = sharedConn

	svc := NewDialBackService(ep, reachabilityif.DefaultConfig())
	require.NoError(t, svc.Start(context.Background()))
	defer svc.Stop()

	var helperID types.NodeID
	helperID[0] = 0x01
	candidate := &stringAddress{raw: "/ip4/127.0.0.1/udp/4001/quic-v1/p2p/" + helperID.String()}

	_, err := svc.VerifyAddresses(context.Background(), helperID, []endpointif.Address{candidate})
	require.NoError(t, err)

	assert.Equal(t, int32(0), sharedConn.closeCount.Load(), "不应关闭已存在的共享连接")
	})

	t.Run("新建连接时也不关闭", func(t *testing.T) {
		ep := &fakeDialBackEndpoint{}
		newConn := &fakeDialBackConn{}
		ep.existing = nil // 无预先存在的连接
		ep.connectReturns = newConn

		svc := NewDialBackService(ep, reachabilityif.DefaultConfig())
		require.NoError(t, svc.Start(context.Background()))
		defer svc.Stop()

		var helperID types.NodeID
		helperID[0] = 0x02
		candidate := &stringAddress{raw: "/ip4/127.0.0.1/udp/4001/quic-v1/p2p/" + helperID.String()}

		_, err := svc.VerifyAddresses(context.Background(), helperID, []endpointif.Address{candidate})
		require.NoError(t, err)

		// 关键断言：即使是新建的连接，dial-back 也不应关闭它
		// 连接生命周期由 Endpoint 统一管理
		assert.Equal(t, int32(0), newConn.closeCount.Load(), "不应关闭新建的连接（由 Endpoint 管理生命周期）")
	})
}

// ============================================================================
//                              结果缓存测试
// ============================================================================

// ============================================================================
//                              dial-back fake (避免 import cycle)
// ============================================================================

type fakeDialBackEndpoint struct {
	existing       endpointif.Connection
	connectReturns endpointif.Connection
}

func (e *fakeDialBackEndpoint) Connect(ctx context.Context, nodeID endpointif.NodeID) (endpointif.Connection, error) {
	return e.connectReturns, nil
}

func (e *fakeDialBackEndpoint) Connection(nodeID endpointif.NodeID) (endpointif.Connection, bool) {
	if e.existing != nil {
		return e.existing, true
	}
	return nil, false
}

func (e *fakeDialBackEndpoint) SetProtocolHandler(protocolID endpointif.ProtocolID, handler endpointif.ProtocolHandler) {
	// 预期会被 DialBackService.Start() 调用，mock 中忽略
}

func (e *fakeDialBackEndpoint) RemoveProtocolHandler(protocolID endpointif.ProtocolID) {
	// 预期会被 DialBackService.Stop() 调用，mock 中忽略
}

type fakeDialBackConn struct {
	closeCount atomic.Int32
}

// ---- endpointif.Connection: 对端信息 ----
func (c *fakeDialBackConn) RemoteID() endpointif.NodeID                 { return types.EmptyNodeID }
func (c *fakeDialBackConn) RemotePublicKey() endpointif.PublicKey       { return nil }
func (c *fakeDialBackConn) RemoteAddrs() []endpointif.Address           { return nil }
func (c *fakeDialBackConn) LocalID() endpointif.NodeID                  { return types.EmptyNodeID }
func (c *fakeDialBackConn) LocalAddrs() []endpointif.Address            { return nil }

// ---- endpointif.Connection: 流管理 ----
func (c *fakeDialBackConn) OpenStream(ctx context.Context, protocolID endpointif.ProtocolID) (endpointif.Stream, error) {
	return &fakeDialBackStream{}, nil
}

func (c *fakeDialBackConn) OpenStreamWithPriority(ctx context.Context, protocolID endpointif.ProtocolID, priority endpointif.Priority) (endpointif.Stream, error) {
	return c.OpenStream(ctx, protocolID)
}

func (c *fakeDialBackConn) AcceptStream(ctx context.Context) (endpointif.Stream, error) { return nil, io.EOF }
func (c *fakeDialBackConn) Streams() []endpointif.Stream                                { return nil }
func (c *fakeDialBackConn) StreamCount() int                                            { return 0 }

// ---- endpointif.Connection: 连接信息 ----
func (c *fakeDialBackConn) Stats() endpointif.ConnectionStats     { return endpointif.ConnectionStats{} }
func (c *fakeDialBackConn) Direction() endpointif.Direction       { return 0 }
func (c *fakeDialBackConn) Transport() string                     { return "fake" }

// ---- endpointif.Connection: 生命周期 ----
func (c *fakeDialBackConn) Close() error {
	c.closeCount.Add(1)
	return nil
}

func (c *fakeDialBackConn) CloseWithError(code uint32, reason string) error { return c.Close() }
func (c *fakeDialBackConn) IsClosed() bool                                  { return false }
func (c *fakeDialBackConn) Done() <-chan struct{}                           { ch := make(chan struct{}); return ch }
func (c *fakeDialBackConn) Context() context.Context                        { return context.Background() }

// ---- endpointif.Connection: 中继信息 ----
func (c *fakeDialBackConn) IsRelayed() bool              { return false }
func (c *fakeDialBackConn) RelayID() endpointif.NodeID   { return types.EmptyNodeID }

// ---- endpointif.Connection: 扩展 ----
func (c *fakeDialBackConn) SetStreamHandler(protocolID endpointif.ProtocolID, handler endpointif.ProtocolHandler) {
}
func (c *fakeDialBackConn) RemoveStreamHandler(protocolID endpointif.ProtocolID) {}
func (c *fakeDialBackConn) RealmContext() *endpointif.RealmContext               { return nil }
func (c *fakeDialBackConn) SetRealmContext(ctx *endpointif.RealmContext)         {}

// fakeDialBackStream 会回显 DialBackResponse（Nonce 与请求一致），以便 VerifyAddresses 通过 nonce 校验。
type fakeDialBackStream struct {
	wbuf []byte
	rbuf []byte
}

func (s *fakeDialBackStream) Read(p []byte) (n int, err error) {
	if len(s.rbuf) == 0 {
		s.prepareResponse()
	}
	if len(s.rbuf) == 0 {
		return 0, io.EOF
	}
	n = copy(p, s.rbuf)
	s.rbuf = s.rbuf[n:]
	return n, nil
}

func (s *fakeDialBackStream) Write(p []byte) (n int, err error) {
	s.wbuf = append(s.wbuf, p...)
	return len(p), nil
}

func (s *fakeDialBackStream) Close() error      { return nil }
func (s *fakeDialBackStream) CloseRead() error  { return nil }
func (s *fakeDialBackStream) CloseWrite() error { return nil }

func (s *fakeDialBackStream) ID() endpointif.StreamID                 { return endpointif.StreamID(1) }
func (s *fakeDialBackStream) ProtocolID() endpointif.ProtocolID       { return reachabilityif.ProtocolID }
func (s *fakeDialBackStream) Connection() endpointif.Connection       { return nil }
func (s *fakeDialBackStream) SetDeadline(t time.Time) error           { return nil }
func (s *fakeDialBackStream) SetReadDeadline(t time.Time) error       { return nil }
func (s *fakeDialBackStream) SetWriteDeadline(t time.Time) error      { return nil }
func (s *fakeDialBackStream) SetPriority(priority endpointif.Priority) {}
func (s *fakeDialBackStream) Priority() endpointif.Priority           { return 0 }
func (s *fakeDialBackStream) Stats() endpointif.StreamStats           { return endpointif.StreamStats{} }
func (s *fakeDialBackStream) IsClosed() bool                          { return false }

func (s *fakeDialBackStream) prepareResponse() {
	// 等待写入完整 frame，再解析
	if len(s.wbuf) < 4 {
		return
	}
	n := int(binary.BigEndian.Uint32(s.wbuf[:4]))
	if len(s.wbuf) < 4+n {
		return
	}
	payload := s.wbuf[4 : 4+n]

	var req reachabilityif.DialBackRequest
	_ = json.Unmarshal(payload, &req)

	resp := reachabilityif.DialBackResponse{
		Nonce:       req.Nonce,
		Reachable:   nil,
		DialResults: nil,
	}
	respBytes, _ := json.Marshal(resp)

	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(respBytes)))
	s.rbuf = append(s.rbuf, lenBuf[:]...)
	s.rbuf = append(s.rbuf, respBytes...)
}


func TestDialBackService_ResultCache(t *testing.T) {
	svc := NewDialBackService(nil, nil)

	// 初始状态无结果
	_, ok := svc.GetVerificationResult(&mockAddress{addr: "test"})
	assert.False(t, ok)

	// 清除空缓存不应报错
	svc.ClearResults()
}

// ============================================================================
//                              辅助 mock
// ============================================================================

type mockAddress struct {
	addr string
}

func (a *mockAddress) Network() string                    { return "ip4" }
func (a *mockAddress) String() string                     { return a.addr }
func (a *mockAddress) Bytes() []byte                      { return []byte(a.addr) }
func (a *mockAddress) Equal(other endpointif.Address) bool { return a.addr == other.String() }
func (a *mockAddress) IsPublic() bool                     { return false }
func (a *mockAddress) IsPrivate() bool                    { return true }
func (a *mockAddress) IsLoopback() bool                   { return false }
func (a *mockAddress) Multiaddr() string {
	// 如果已经是 multiaddr 格式，直接返回
	if len(a.addr) > 0 && a.addr[0] == '/' {
		return a.addr
	}
	// 否则转换为 multiaddr
	return fmt.Sprintf("/ip4/%s/udp/4001/quic-v1", a.addr)
}

// stringAddress 字符串地址（测试用）
type stringAddress struct {
	raw string
}

func (a *stringAddress) Network() string                     { return "ip4" }
func (a *stringAddress) String() string                      { return a.raw }
func (a *stringAddress) Bytes() []byte                       { return []byte(a.raw) }
func (a *stringAddress) Equal(other endpointif.Address) bool { return a.raw == other.String() }
func (a *stringAddress) IsPublic() bool                      { return types.Multiaddr(a.raw).IsPublic() }
func (a *stringAddress) IsPrivate() bool                     { return types.Multiaddr(a.raw).IsPrivate() }
func (a *stringAddress) IsLoopback() bool                    { return types.Multiaddr(a.raw).IsLoopback() }
func (a *stringAddress) Multiaddr() string                   { return a.raw }

// ============================================================================
//                              并发测试
// ============================================================================

// TestDialBackService_VerifyAddresses_ConcurrentSafe 验证并发调用 VerifyAddresses 的安全性
// 覆盖 TOCTOU 竞态场景：多个 goroutine 同时对同一 helperID 调用 VerifyAddresses
func TestDialBackService_VerifyAddresses_ConcurrentSafe(t *testing.T) {
	// 创建一个共享连接，模拟并发场景下的连接复用
	sharedConn := &fakeDialBackConn{}

	ep := &fakeDialBackEndpoint{
		existing:       sharedConn,
		connectReturns: sharedConn,
	}

	svc := NewDialBackService(ep, reachabilityif.DefaultConfig())
	require.NoError(t, svc.Start(context.Background()))
	defer svc.Stop()

	var helperID types.NodeID
	helperID[0] = 0x01

	candidate := &stringAddress{raw: "/ip4/127.0.0.1/udp/4001/quic-v1/p2p/" + helperID.String()}

	const numGoroutines = 20
	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// 并发调用 VerifyAddresses
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			_, _ = svc.VerifyAddresses(context.Background(), helperID, []endpointif.Address{candidate})
		}()
	}

	wg.Wait()

	// 关键断言：无论多少并发调用，连接都不应被关闭
	// 这验证了修复后不存在 TOCTOU 竞态条件
	assert.Equal(t, int32(0), sharedConn.closeCount.Load(),
		"并发调用 VerifyAddresses 不应关闭任何连接（避免 TOCTOU 竞态）")
}

func TestDialBackService_ConcurrentRequests(t *testing.T) {
	svc := NewDialBackService(nil, nil)
	svc.Start(context.Background())
	defer svc.Stop()

	// 并发发送多个请求
	const numRequests = 10
	done := make(chan struct{}, numRequests)

	for i := 0; i < numRequests; i++ {
		go func(idx int) {
			req := &reachabilityif.DialBackRequest{
				Addrs:     []string{"/ip4/192.0.2.1/udp/12345/quic-v1"},
				Nonce:     []byte{byte(idx)},
				TimeoutMs: 50,
			}
			resp := svc.HandleDialBackRequest(context.Background(), req)
			assert.NotNil(t, resp)
			done <- struct{}{}
		}(i)
	}

	// 等待所有请求完成
	for i := 0; i < numRequests; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("请求超时")
		}
	}
}

// ============================================================================
//                              边界条件测试
// ============================================================================

func TestDialBackService_MaxAddrsLimit(t *testing.T) {
	svc := NewDialBackService(nil, nil)
	svc.Start(context.Background())
	defer svc.Stop()

	// 创建超过限制的地址列表
	addrs := make([]string, reachabilityif.MaxAddrsPerRequest+5)
	for i := range addrs {
		addrs[i] = "/ip4/192.0.2.1/udp/12345/quic-v1"
	}

	req := &reachabilityif.DialBackRequest{
		Addrs:     addrs,
		Nonce:     []byte("test"),
		TimeoutMs: 50,
	}
	resp := svc.HandleDialBackRequest(context.Background(), req)

	// 应该只处理 MaxAddrsPerRequest 个地址
	assert.LessOrEqual(t, len(resp.DialResults), reachabilityif.MaxAddrsPerRequest)
}

// ============================================================================
//                              本地回拨测试（需要实际网络）
// ============================================================================

func TestDialBackService_LocalLoopback(t *testing.T) {
	// 启动一个本地 UDP 服务器
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		t.Skip("无法创建本地 UDP 服务器:", err)
	}
	defer func() { _ = conn.Close() }()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	// 测试回拨
	svc := NewDialBackService(nil, nil)
	svc.Start(context.Background())
	defer svc.Stop()

	multiaddr := "/ip4/127.0.0.1/udp/" + strconv.Itoa(localAddr.Port) + "/quic-v1"

	req := &reachabilityif.DialBackRequest{
		Addrs:     []string{multiaddr},
		Nonce:     []byte("test"),
		TimeoutMs: 1000,
	}

	resp := svc.HandleDialBackRequest(context.Background(), req)

	// UDP 回拨应该"成功"（发送探测包不会失败）
	assert.NotEmpty(t, resp.DialResults)
	// 注意：由于 UDP 无连接特性，发送探测包通常会成功
	// 即使目标没有响应，Success 也可能为 true
}

