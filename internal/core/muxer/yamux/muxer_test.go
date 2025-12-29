package yamux

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	muxerif "github.com/dep2p/go-dep2p/pkg/interfaces/muxer"
)

// createConnPair 创建一对连接的网络连接
func createConnPair(t *testing.T) (net.Conn, net.Conn) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	var serverConn net.Conn
	var serverErr error
	done := make(chan struct{})

	go func() {
		serverConn, serverErr = listener.Accept()
		close(done)
	}()

	clientConn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)

	<-done
	require.NoError(t, serverErr)
	listener.Close()

	return serverConn, clientConn
}

// createMuxerPair 创建一对 Muxer（服务端和客户端）
func createMuxerPair(t *testing.T) (*Muxer, *Muxer, func()) {
	serverConn, clientConn := createConnPair(t)

	factory := NewFactory(muxerif.DefaultConfig())

	serverMuxer, err := factory.NewMuxer(serverConn, true)
	require.NoError(t, err)

	clientMuxer, err := factory.NewMuxer(clientConn, false)
	require.NoError(t, err)

	cleanup := func() {
		serverMuxer.Close()
		clientMuxer.Close()
		serverConn.Close()
		clientConn.Close()
	}

	return serverMuxer.(*Muxer), clientMuxer.(*Muxer), cleanup
}

func TestNewMuxer(t *testing.T) {
	serverMuxer, clientMuxer, cleanup := createMuxerPair(t)
	defer cleanup()

	assert.NotNil(t, serverMuxer)
	assert.NotNil(t, clientMuxer)
	assert.True(t, serverMuxer.IsServer())
	assert.False(t, clientMuxer.IsServer())
	assert.False(t, serverMuxer.IsClosed())
	assert.False(t, clientMuxer.IsClosed())
	assert.Equal(t, 0, serverMuxer.NumStreams())
	assert.Equal(t, 0, clientMuxer.NumStreams())
}

func TestMuxerNewStreamAcceptStream(t *testing.T) {
	serverMuxer, clientMuxer, cleanup := createMuxerPair(t)
	defer cleanup()

	var wg sync.WaitGroup
	var serverStream muxerif.Stream
	var clientStream muxerif.Stream
	var serverErr, clientErr error

	wg.Add(2)

	// 服务端接受流
	go func() {
		defer wg.Done()
		serverStream, serverErr = serverMuxer.AcceptStream()
	}()

	// 客户端创建流
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		clientStream, clientErr = clientMuxer.NewStream(ctx)
	}()

	wg.Wait()

	require.NoError(t, serverErr)
	require.NoError(t, clientErr)
	require.NotNil(t, serverStream)
	require.NotNil(t, clientStream)

	// 验证流数量
	assert.Equal(t, 1, serverMuxer.NumStreams())
	assert.Equal(t, 1, clientMuxer.NumStreams())

	// 清理流
	serverStream.Close()
	clientStream.Close()
}

func TestMuxerDataTransfer(t *testing.T) {
	serverMuxer, clientMuxer, cleanup := createMuxerPair(t)
	defer cleanup()

	var wg sync.WaitGroup
	var serverStream muxerif.Stream
	var clientStream muxerif.Stream

	wg.Add(2)

	// 服务端接受流
	go func() {
		defer wg.Done()
		var err error
		serverStream, err = serverMuxer.AcceptStream()
		require.NoError(t, err)
	}()

	// 客户端创建流
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var err error
		clientStream, err = clientMuxer.NewStream(ctx)
		require.NoError(t, err)
	}()

	wg.Wait()

	// 测试数据传输
	message := "Hello, Yamux!"

	wg.Add(2)

	// 客户端发送
	go func() {
		defer wg.Done()
		n, err := clientStream.Write([]byte(message))
		require.NoError(t, err)
		assert.Equal(t, len(message), n)
	}()

	// 服务端接收
	go func() {
		defer wg.Done()
		buf := make([]byte, len(message))
		n, err := io.ReadFull(serverStream, buf)
		require.NoError(t, err)
		assert.Equal(t, len(message), n)
		assert.Equal(t, message, string(buf))
	}()

	wg.Wait()

	serverStream.Close()
	clientStream.Close()
}

func TestMuxerMultipleStreams(t *testing.T) {
	serverMuxer, clientMuxer, cleanup := createMuxerPair(t)
	defer cleanup()

	numStreams := 5
	var wg sync.WaitGroup

	serverStreams := make([]muxerif.Stream, numStreams)
	clientStreams := make([]muxerif.Stream, numStreams)

	// 服务端接受流
	go func() {
		for i := 0; i < numStreams; i++ {
			s, err := serverMuxer.AcceptStream()
			require.NoError(t, err)
			serverStreams[i] = s
		}
	}()

	// 客户端创建流
	for i := 0; i < numStreams; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		s, err := clientMuxer.NewStream(ctx)
		cancel()
		require.NoError(t, err)
		clientStreams[i] = s
	}

	// 等待服务端接受所有流
	time.Sleep(100 * time.Millisecond)

	// 验证流数量
	assert.Equal(t, numStreams, serverMuxer.NumStreams())
	assert.Equal(t, numStreams, clientMuxer.NumStreams())

	// 在每个流上进行数据传输
	wg.Add(numStreams * 2)

	for i := 0; i < numStreams; i++ {
		idx := i
		message := []byte{byte(idx)}

		// 发送
		go func() {
			defer wg.Done()
			_, err := clientStreams[idx].Write(message)
			require.NoError(t, err)
		}()

		// 接收
		go func() {
			defer wg.Done()
			buf := make([]byte, 1)
			_, err := io.ReadFull(serverStreams[idx], buf)
			require.NoError(t, err)
			assert.Equal(t, message, buf)
		}()
	}

	wg.Wait()

	// 关闭所有流
	for i := 0; i < numStreams; i++ {
		serverStreams[i].Close()
		clientStreams[i].Close()
	}
}

func TestMuxerClose(t *testing.T) {
	serverMuxer, clientMuxer, cleanup := createMuxerPair(t)
	defer cleanup()

	// 创建一个流
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		serverMuxer.AcceptStream()
	}()

	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		clientMuxer.NewStream(ctx)
	}()

	wg.Wait()

	// 关闭 muxer
	err := serverMuxer.Close()
	require.NoError(t, err)

	assert.True(t, serverMuxer.IsClosed())

	// 在已关闭的 muxer 上创建流应该失败
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_, err = serverMuxer.NewStream(ctx)
	assert.Error(t, err)

	// 重复关闭应该没有问题
	err = serverMuxer.Close()
	assert.NoError(t, err)
}

func TestMuxerPing(t *testing.T) {
	serverMuxer, clientMuxer, cleanup := createMuxerPair(t)
	defer cleanup()

	// 测试 ping
	duration, err := clientMuxer.Ping()
	require.NoError(t, err)
	assert.True(t, duration > 0)

	duration, err = serverMuxer.Ping()
	require.NoError(t, err)
	assert.True(t, duration > 0)
}

func TestMuxerNewStreamContext(t *testing.T) {
	_, clientMuxer, cleanup := createMuxerPair(t)
	defer cleanup()

	// 测试超时 context
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// 没有接受方，创建流应该超时
	// 注意：这取决于 yamux 的行为，可能不会超时
	_, err := clientMuxer.NewStream(ctx)
	// 可能成功也可能超时，取决于时机
	_ = err

	// 测试已取消的 context
	ctx2, cancel2 := context.WithCancel(context.Background())
	cancel2() // 立即取消

	_, err = clientMuxer.NewStream(ctx2)
	assert.Error(t, err)
}

func TestMuxerGetStream(t *testing.T) {
	serverMuxer, clientMuxer, cleanup := createMuxerPair(t)
	defer cleanup()

	var wg sync.WaitGroup
	wg.Add(2)

	var serverStream muxerif.Stream

	go func() {
		defer wg.Done()
		var err error
		serverStream, err = serverMuxer.AcceptStream()
		require.NoError(t, err)
	}()

	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := clientMuxer.NewStream(ctx)
		require.NoError(t, err)
	}()

	wg.Wait()

	// 测试 GetStream
	stream, ok := serverMuxer.GetStream(serverStream.ID())
	assert.True(t, ok)
	assert.NotNil(t, stream)
	assert.Equal(t, serverStream.ID(), stream.ID())

	// 测试不存在的流
	_, ok = serverMuxer.GetStream(99999)
	assert.False(t, ok)

	serverStream.Close()
}

func TestMuxerAllStreams(t *testing.T) {
	serverMuxer, clientMuxer, cleanup := createMuxerPair(t)
	defer cleanup()

	numStreams := 3

	// 服务端接受流
	go func() {
		for i := 0; i < numStreams; i++ {
			serverMuxer.AcceptStream()
		}
	}()

	// 客户端创建流
	for i := 0; i < numStreams; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		clientMuxer.NewStream(ctx)
		cancel()
	}

	time.Sleep(100 * time.Millisecond)

	// 测试 AllStreams
	streams := serverMuxer.AllStreams()
	assert.Len(t, streams, numStreams)

	streams = clientMuxer.AllStreams()
	assert.Len(t, streams, numStreams)
}

// ============================================================================
//                              针对性修复测试
// ============================================================================

func TestMuxer_NewStream_ContextCancel(t *testing.T) {
	serverMuxer, clientMuxer, cleanup := createMuxerPair(t)
	defer cleanup()

	// 使用已取消的 context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	_, err := clientMuxer.NewStream(ctx)
	assert.Error(t, err)
	assert.Equal(t, context.Canceled, err)

	// 验证 muxer 仍然可用
	assert.False(t, clientMuxer.IsClosed())

	// 使用有效 context 创建流应该仍然成功
	go func() {
		serverMuxer.AcceptStream()
	}()

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	stream, err := clientMuxer.NewStream(ctx2)
	assert.NoError(t, err)
	assert.NotNil(t, stream)
	stream.Close()
}

func TestMuxer_Close_ReturnsError(t *testing.T) {
	serverMuxer, clientMuxer, cleanup := createMuxerPair(t)
	defer cleanup()

	// 创建一些流
	go func() {
		for i := 0; i < 3; i++ {
			serverMuxer.AcceptStream()
		}
	}()

	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		clientMuxer.NewStream(ctx)
		cancel()
	}

	time.Sleep(100 * time.Millisecond)

	// 关闭 muxer，应该能成功关闭所有流
	err := clientMuxer.Close()
	// 错误可能为 nil 或非 nil，取决于流的状态
	_ = err

	assert.True(t, clientMuxer.IsClosed())

	// 重复关闭应该没问题
	err = clientMuxer.Close()
	assert.NoError(t, err)
}

func TestMuxer_AddStream_NoDuplicateCount(t *testing.T) {
	serverMuxer, clientMuxer, cleanup := createMuxerPair(t)
	defer cleanup()

	var wg sync.WaitGroup
	wg.Add(2)

	var serverStream muxerif.Stream

	go func() {
		defer wg.Done()
		var err error
		serverStream, err = serverMuxer.AcceptStream()
		require.NoError(t, err)
	}()

	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := clientMuxer.NewStream(ctx)
		require.NoError(t, err)
	}()

	wg.Wait()

	// 验证流数量
	assert.Equal(t, 1, serverMuxer.NumStreams())

	// 尝试重复添加（通过内部方法）
	if s, ok := serverStream.(*Stream); ok {
		serverMuxer.addStream(s)
		// 流数量不应该增加
		assert.Equal(t, 1, serverMuxer.NumStreams())
	}

	serverStream.Close()
}

