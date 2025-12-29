package yamux

import (
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	muxerif "github.com/dep2p/go-dep2p/pkg/interfaces/muxer"
)

func TestStreamReadWrite(t *testing.T) {
	serverMuxer, clientMuxer, cleanup := createMuxerPair(t)
	defer cleanup()

	var wg sync.WaitGroup
	var serverStream muxerif.Stream
	var clientStream muxerif.Stream

	wg.Add(2)

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
		var err error
		clientStream, err = clientMuxer.NewStream(ctx)
		require.NoError(t, err)
	}()

	wg.Wait()

	// 测试双向读写
	wg.Add(4)

	// 客户端 -> 服务端
	go func() {
		defer wg.Done()
		_, err := clientStream.Write([]byte("client->server"))
		require.NoError(t, err)
	}()

	go func() {
		defer wg.Done()
		buf := make([]byte, 14)
		_, err := io.ReadFull(serverStream, buf)
		require.NoError(t, err)
		assert.Equal(t, "client->server", string(buf))
	}()

	// 服务端 -> 客户端
	go func() {
		defer wg.Done()
		_, err := serverStream.Write([]byte("server->client"))
		require.NoError(t, err)
	}()

	go func() {
		defer wg.Done()
		buf := make([]byte, 14)
		_, err := io.ReadFull(clientStream, buf)
		require.NoError(t, err)
		assert.Equal(t, "server->client", string(buf))
	}()

	wg.Wait()

	serverStream.Close()
	clientStream.Close()
}

func TestStreamID(t *testing.T) {
	serverMuxer, clientMuxer, cleanup := createMuxerPair(t)
	defer cleanup()

	var wg sync.WaitGroup
	var serverStream muxerif.Stream
	var clientStream muxerif.Stream

	wg.Add(2)

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
		var err error
		clientStream, err = clientMuxer.NewStream(ctx)
		require.NoError(t, err)
	}()

	wg.Wait()

	// 流 ID 应该是非零的
	assert.NotEqual(t, uint32(0), clientStream.ID())
	assert.NotEqual(t, uint32(0), serverStream.ID())

	serverStream.Close()
	clientStream.Close()
}

func TestStreamClose(t *testing.T) {
	serverMuxer, clientMuxer, cleanup := createMuxerPair(t)
	defer cleanup()

	var wg sync.WaitGroup
	var serverStream muxerif.Stream
	var clientStream muxerif.Stream

	wg.Add(2)

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
		var err error
		clientStream, err = clientMuxer.NewStream(ctx)
		require.NoError(t, err)
	}()

	wg.Wait()

	// 关闭客户端流
	err := clientStream.Close()
	require.NoError(t, err)

	// 重复关闭应该没问题
	err = clientStream.Close()
	assert.NoError(t, err)

	// 服务端读取应该收到 EOF
	buf := make([]byte, 10)
	_, err = serverStream.Read(buf)
	assert.Error(t, err) // 应该是 EOF 或类似错误

	serverStream.Close()
}

func TestStreamSetDeadline(t *testing.T) {
	serverMuxer, clientMuxer, cleanup := createMuxerPair(t)
	defer cleanup()

	var wg sync.WaitGroup
	var serverStream muxerif.Stream
	var clientStream muxerif.Stream

	wg.Add(2)

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
		var err error
		clientStream, err = clientMuxer.NewStream(ctx)
		require.NoError(t, err)
	}()

	wg.Wait()

	// 设置读取超时
	err := serverStream.SetReadDeadline(time.Now().Add(10 * time.Millisecond))
	require.NoError(t, err)

	// 尝试读取（应该超时）
	buf := make([]byte, 10)
	_, err = serverStream.Read(buf)
	assert.Error(t, err) // 应该超时

	// 设置写入超时
	err = clientStream.SetWriteDeadline(time.Now().Add(1 * time.Hour))
	require.NoError(t, err)

	// 设置整体超时
	err = serverStream.SetDeadline(time.Now().Add(1 * time.Hour))
	require.NoError(t, err)

	serverStream.Close()
	clientStream.Close()
}

func TestStreamReset(t *testing.T) {
	serverMuxer, clientMuxer, cleanup := createMuxerPair(t)
	defer cleanup()

	var wg sync.WaitGroup
	var serverStream muxerif.Stream
	var clientStream muxerif.Stream

	wg.Add(2)

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
		var err error
		clientStream, err = clientMuxer.NewStream(ctx)
		require.NoError(t, err)
	}()

	wg.Wait()

	// 重置流
	err := clientStream.Reset()
	require.NoError(t, err)

	// 获取内部 Stream 对象检查状态
	if s, ok := clientStream.(*Stream); ok {
		assert.True(t, s.IsClosed())
	}

	// 重复重置应该没问题
	err = clientStream.Reset()
	assert.NoError(t, err)

	serverStream.Close()
}

func TestStreamCloseReadWrite(t *testing.T) {
	serverMuxer, clientMuxer, cleanup := createMuxerPair(t)
	defer cleanup()

	var wg sync.WaitGroup
	var serverStream muxerif.Stream
	var clientStream muxerif.Stream

	wg.Add(2)

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
		var err error
		clientStream, err = clientMuxer.NewStream(ctx)
		require.NoError(t, err)
	}()

	wg.Wait()

	// 关闭读端
	err := clientStream.CloseRead()
	require.NoError(t, err)

	// 关闭写端
	err = clientStream.CloseWrite()
	require.NoError(t, err)

	serverStream.Close()
}

