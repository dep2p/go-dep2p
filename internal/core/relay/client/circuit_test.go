package client

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/muxer"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// circuitMockStream 实现 pkgif.Stream 接口的简化 mock
type circuitMockStream struct {
	conn     pkgif.Connection
	protocol string
	closed   bool
	mu       sync.Mutex
}

func (s *circuitMockStream) Read(p []byte) (n int, err error)  { return 0, nil }
func (s *circuitMockStream) Write(p []byte) (n int, err error) { return len(p), nil }
func (s *circuitMockStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}
func (s *circuitMockStream) CloseWrite() error                  { return nil }
func (s *circuitMockStream) CloseRead() error                   { return nil }
func (s *circuitMockStream) Reset() error                       { return s.Close() }
func (s *circuitMockStream) SetDeadline(t time.Time) error      { return nil }
func (s *circuitMockStream) SetReadDeadline(t time.Time) error  { return nil }
func (s *circuitMockStream) SetWriteDeadline(t time.Time) error { return nil }
func (s *circuitMockStream) Protocol() string                   { return s.protocol }
func (s *circuitMockStream) SetProtocol(protocol string)        { s.protocol = protocol }
func (s *circuitMockStream) Conn() pkgif.Connection             { return s.conn }
func (s *circuitMockStream) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}
func (s *circuitMockStream) Stat() types.StreamStat {
	return types.StreamStat{}
}
func (s *circuitMockStream) State() types.StreamState {
	if s.IsClosed() {
		return types.StreamStateClosed
	}
	return types.StreamStateOpen
}

// createMuxerPair 创建一对 yamux muxer 用于测试
func createMuxerPair(t *testing.T) (pkgif.MuxedConn, pkgif.MuxedConn, func()) {
	clientConn, serverConn := net.Pipe()
	transport := muxer.NewTransport()

	var clientMuxer, serverMuxer pkgif.MuxedConn
	var clientErr, serverErr error

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		clientMuxer, clientErr = transport.NewConn(clientConn, false, nil)
	}()

	go func() {
		defer wg.Done()
		serverMuxer, serverErr = transport.NewConn(serverConn, true, nil)
	}()

	wg.Wait()

	require.NoError(t, clientErr)
	require.NoError(t, serverErr)

	cleanup := func() {
		clientMuxer.Close()
		serverMuxer.Close()
		clientConn.Close()
		serverConn.Close()
	}

	return clientMuxer, serverMuxer, cleanup
}

// startControlResponder 接收控制流并响应 ping/pong
func startControlResponder(t *testing.T, serverMuxer pkgif.MuxedConn) {
	t.Helper()
	stream, err := serverMuxer.AcceptStream()
	if err != nil {
		return
	}
	go func() {
		defer stream.Close()
		buf := make([]byte, 1)
		for {
			n, err := io.ReadFull(stream, buf)
			if err != nil || n != 1 {
				return
			}
			switch buf[0] {
			case controlMsgPing:
				_, _ = stream.Write([]byte{controlMsgPong})
			case controlMsgPong:
				// ignore
			}
		}
	}()
}

// TestRelayCircuitState 测试电路状态机
func TestRelayCircuitState(t *testing.T) {
	clientMuxer, serverMuxer, cleanup := createMuxerPair(t)
	defer cleanup()

	// 创建 mock 基础流
	mockBase := &circuitMockStream{}

	// 创建 RelayCircuit
	circuit := NewRelayCircuit(
		mockBase,
		clientMuxer,
		types.PeerID("local-peer"),
		types.PeerID("remote-peer"),
		types.PeerID("relay-peer"),
		nil, // relayTransportAddr - 测试中不需要
	)
	startControlResponder(t, serverMuxer)

	// 测试初始状态
	assert.Equal(t, CircuitStateActive, circuit.State())
	assert.False(t, circuit.IsClosed())

	// 测试状态转换
	circuit.SetState(CircuitStateStale)
	assert.Equal(t, CircuitStateStale, circuit.State())

	// 测试关闭
	err := circuit.Close()
	assert.NoError(t, err)
	assert.Equal(t, CircuitStateClosed, circuit.State())
	assert.True(t, circuit.IsClosed())

	// 关闭服务端
	_ = serverMuxer
}

// TestRelayCircuitIdentity 测试电路身份信息
func TestRelayCircuitIdentity(t *testing.T) {
	clientMuxer, serverMuxer, cleanup := createMuxerPair(t)
	defer cleanup()

	// 创建 mock 基础流
	mockBase := &circuitMockStream{}

	// 创建 RelayCircuit
	localPeer := types.PeerID("local-peer-12345")
	remotePeer := types.PeerID("remote-peer-67890")
	relayPeer := types.PeerID("relay-peer-abcde")

	circuit := NewRelayCircuit(
		mockBase,
		clientMuxer,
		localPeer,
		remotePeer,
		relayPeer,
		nil, // relayTransportAddr - 测试中不需要
	)
	startControlResponder(t, serverMuxer)
	defer circuit.Close()

	// 验证身份信息
	assert.Equal(t, localPeer, circuit.LocalPeer())
	assert.Equal(t, remotePeer, circuit.RemotePeer())
	assert.Contains(t, circuit.LocalMultiaddr().String(), string(localPeer))
	assert.Contains(t, circuit.RemoteMultiaddr().String(), string(relayPeer))
	assert.Contains(t, circuit.RemoteMultiaddr().String(), "p2p-circuit")
	assert.Contains(t, circuit.RemoteMultiaddr().String(), string(remotePeer))

	// 验证连接类型
	assert.Equal(t, pkgif.ConnectionTypeRelay, circuit.ConnType())

	// 验证统计
	stat := circuit.Stat()
	assert.True(t, stat.Transient)
	assert.Equal(t, 0, stat.NumStreams)
}

// TestRelayCircuitMultiStream 测试多流创建
func TestRelayCircuitMultiStream(t *testing.T) {
	clientMuxer, serverMuxer, cleanup := createMuxerPair(t)
	defer cleanup()

	// 创建 mock 基础流
	mockBase := &circuitMockStream{}

	// 创建 RelayCircuit
	circuit := NewRelayCircuit(
		mockBase,
		clientMuxer,
		types.PeerID("local"),
		types.PeerID("remote"),
		types.PeerID("relay"),
		nil, // relayTransportAddr - 测试中不需要
	)
	defer circuit.Close()
	startControlResponder(t, serverMuxer)

	// 测试创建多个流
	ctx := context.Background()
	const numStreams = 3

	var streams []pkgif.Stream

	for i := 0; i < numStreams; i++ {
		// 在后台接受流
		go func() {
			s, _ := serverMuxer.AcceptStream()
			if s != nil {
				defer s.Close()
			}
		}()

		// 客户端创建流
		stream, err := circuit.NewStream(ctx)
		require.NoError(t, err)
		streams = append(streams, stream)
	}

	// 等待一下让流跟踪更新
	time.Sleep(10 * time.Millisecond)

	// 验证流数量
	assert.Len(t, circuit.GetStreams(), numStreams)

	// 关闭一个流
	if len(streams) > 0 {
		err := streams[0].Close()
		assert.NoError(t, err)

		// 等待流跟踪更新
		time.Sleep(10 * time.Millisecond)

		// 其他流应该仍然活跃
		activeStreams := circuit.GetStreams()
		assert.Len(t, activeStreams, numStreams-1)
	}
}

// TestRelayCircuitStreamIsolation 测试流关闭隔离
func TestRelayCircuitStreamIsolation(t *testing.T) {
	clientMuxer, serverMuxer, cleanup := createMuxerPair(t)
	defer cleanup()

	// 创建 mock 基础流
	mockBase := &circuitMockStream{}

	// 创建 RelayCircuit
	circuit := NewRelayCircuit(
		mockBase,
		clientMuxer,
		types.PeerID("local"),
		types.PeerID("remote"),
		types.PeerID("relay"),
		nil, // relayTransportAddr - 测试中不需要
	)
	defer circuit.Close()
	startControlResponder(t, serverMuxer)

	ctx := context.Background()

	// 后台接受流
	go func() {
		for i := 0; i < 2; i++ {
			s, _ := serverMuxer.AcceptStream()
			if s != nil {
				defer s.Close()
			}
		}
	}()

	// 创建两个流
	stream1, err := circuit.NewStream(ctx)
	require.NoError(t, err)

	stream2, err := circuit.NewStream(ctx)
	require.NoError(t, err)

	// 等待一下
	time.Sleep(10 * time.Millisecond)

	// 验证电路有两个流
	assert.Len(t, circuit.GetStreams(), 2)

	// 关闭第一个流
	err = stream1.Close()
	assert.NoError(t, err)

	// 等待流跟踪更新
	time.Sleep(10 * time.Millisecond)

	// 电路应该仍然活跃，第二个流应该仍然可用
	assert.Equal(t, CircuitStateActive, circuit.State())
	assert.Len(t, circuit.GetStreams(), 1)
	assert.False(t, stream2.IsClosed())

	// 清理
	stream2.Close()
}

// TestRelayCircuitClose 测试电路关闭
func TestRelayCircuitClose(t *testing.T) {
	clientMuxer, serverMuxer, cleanup := createMuxerPair(t)
	defer cleanup()

	// 创建 mock 基础流
	mockBase := &circuitMockStream{}

	// 创建 RelayCircuit
	circuit := NewRelayCircuit(
		mockBase,
		clientMuxer,
		types.PeerID("local"),
		types.PeerID("remote"),
		types.PeerID("relay"),
		nil, // relayTransportAddr - 测试中不需要
	)
	startControlResponder(t, serverMuxer)

	ctx := context.Background()

	// 后台接受流
	go func() {
		for i := 0; i < 2; i++ {
			s, _ := serverMuxer.AcceptStream()
			if s != nil {
				defer s.Close()
			}
		}
	}()

	// 创建一些流
	stream1, err := circuit.NewStream(ctx)
	require.NoError(t, err)

	stream2, err := circuit.NewStream(ctx)
	require.NoError(t, err)

	// 等待一下
	time.Sleep(10 * time.Millisecond)

	// 验证流存在
	assert.Len(t, circuit.GetStreams(), 2)

	// 关闭电路
	err = circuit.Close()
	assert.NoError(t, err)

	// 验证电路已关闭
	assert.True(t, circuit.IsClosed())
	assert.Equal(t, CircuitStateClosed, circuit.State())

	// 验证所有流都被关闭
	assert.Len(t, circuit.GetStreams(), 0)
	assert.True(t, stream1.IsClosed())
	assert.True(t, stream2.IsClosed())

	// 尝试创建新流应该失败
	_, err = circuit.NewStream(ctx)
	assert.Error(t, err)
	assert.Equal(t, ErrCircuitNotActive, err)

	// 多次关闭应该是幂等的
	err = circuit.Close()
	assert.NoError(t, err)
}

// TestRelayCircuitActivity 测试活动时间跟踪
func TestRelayCircuitActivity(t *testing.T) {
	clientMuxer, serverMuxer, cleanup := createMuxerPair(t)
	defer cleanup()

	// 创建 mock 基础流
	mockBase := &circuitMockStream{}

	// 创建 RelayCircuit
	circuit := NewRelayCircuit(
		mockBase,
		clientMuxer,
		types.PeerID("local"),
		types.PeerID("remote"),
		types.PeerID("relay"),
		nil, // relayTransportAddr - 测试中不需要
	)
	defer circuit.Close()
	startControlResponder(t, serverMuxer)

	// 记录初始活动时间
	initialActivity := circuit.LastActivity()

	// 等待一小段时间
	time.Sleep(20 * time.Millisecond)

	// 后台接受流
	go func() {
		s, _ := serverMuxer.AcceptStream()
		if s != nil {
			defer s.Close()
		}
	}()

	// 创建新流应该更新活动时间
	ctx := context.Background()
	stream, err := circuit.NewStream(ctx)
	require.NoError(t, err)
	defer stream.Close()

	// 验证活动时间已更新
	newActivity := circuit.LastActivity()
	assert.True(t, newActivity.After(initialActivity), "Activity time should be updated after NewStream")
}

// TestCreateRelayCircuitFromStream 测试从流创建电路
func TestCreateRelayCircuitFromStream(t *testing.T) {
	// 使用 net.Pipe 创建连接对
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	// 将 net.Conn 包装为 mock stream
	clientStream := &netConnStream{conn: clientConn}
	serverStream := &netConnStream{conn: serverConn}

	var clientCircuit, serverCircuit *RelayCircuit
	var clientErr, serverErr error

	var wg sync.WaitGroup
	wg.Add(2)

	// 客户端创建电路
	go func() {
		defer wg.Done()
		clientCircuit, clientErr = CreateRelayCircuitFromStream(
			clientStream,
			false, // isClient
			types.PeerID("client"),
			types.PeerID("server"),
			types.PeerID("relay"),
			nil, // relayTransportAddr - 测试中不需要
		)
	}()

	// 服务端创建电路
	go func() {
		defer wg.Done()
		serverCircuit, serverErr = CreateRelayCircuitFromStream(
			serverStream,
			true, // isServer
			types.PeerID("server"),
			types.PeerID("client"),
			types.PeerID("relay"),
			nil, // relayTransportAddr - 测试中不需要
		)
	}()

	wg.Wait()

	require.NoError(t, clientErr)
	require.NoError(t, serverErr)
	defer clientCircuit.Close()
	defer serverCircuit.Close()

	// 验证两端都是活跃状态
	assert.Equal(t, CircuitStateActive, clientCircuit.State())
	assert.Equal(t, CircuitStateActive, serverCircuit.State())

	// 验证可以创建流
	ctx := context.Background()

	// 客户端创建流，服务端接受
	go func() {
		s, _ := serverCircuit.AcceptStream()
		if s != nil {
			s.Close()
		}
	}()

	stream, err := clientCircuit.NewStream(ctx)
	require.NoError(t, err)
	stream.Close()
}

// netConnStream 将 net.Conn 包装为 pkgif.Stream
type netConnStream struct {
	conn     net.Conn
	protocol string
	closed   bool
	mu       sync.Mutex
}

func (s *netConnStream) Read(p []byte) (n int, err error)  { return s.conn.Read(p) }
func (s *netConnStream) Write(p []byte) (n int, err error) { return s.conn.Write(p) }
func (s *netConnStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return s.conn.Close()
}
func (s *netConnStream) CloseWrite() error                  { return nil }
func (s *netConnStream) CloseRead() error                   { return nil }
func (s *netConnStream) Reset() error                       { return s.Close() }
func (s *netConnStream) SetDeadline(t time.Time) error      { return s.conn.SetDeadline(t) }
func (s *netConnStream) SetReadDeadline(t time.Time) error  { return s.conn.SetReadDeadline(t) }
func (s *netConnStream) SetWriteDeadline(t time.Time) error { return s.conn.SetWriteDeadline(t) }
func (s *netConnStream) Protocol() string                   { return s.protocol }
func (s *netConnStream) SetProtocol(protocol string)        { s.protocol = protocol }
func (s *netConnStream) Conn() pkgif.Connection             { return nil }
func (s *netConnStream) IsClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}
func (s *netConnStream) Stat() types.StreamStat {
	return types.StreamStat{}
}
func (s *netConnStream) State() types.StreamState {
	if s.IsClosed() {
		return types.StreamStateClosed
	}
	return types.StreamStateOpen
}
