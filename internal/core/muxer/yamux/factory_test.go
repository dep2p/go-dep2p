package yamux

import (
	"net"
	"testing"

	"github.com/hashicorp/yamux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	muxerif "github.com/dep2p/go-dep2p/pkg/interfaces/muxer"
)

func TestNewFactory(t *testing.T) {
	config := muxerif.DefaultConfig()
	factory := NewFactory(config)

	assert.NotNil(t, factory)
	assert.Equal(t, "yamux", factory.Protocol())
	assert.Equal(t, config.MaxStreams, factory.Config().MaxStreams)
}

func TestNewFactoryWithYamuxConfig(t *testing.T) {
	yamuxCfg := DefaultYamuxConfig()
	yamuxCfg.AcceptBacklog = 512

	factory := NewFactoryWithYamuxConfig(yamuxCfg)

	assert.NotNil(t, factory)
	assert.Equal(t, "yamux", factory.Protocol())
	assert.Equal(t, 512, factory.Config().MaxStreams)
	assert.Equal(t, yamuxCfg, factory.YamuxConfig())
}

func TestFactoryNewMuxerServer(t *testing.T) {
	serverConn, clientConn := createConnPairForFactory(t)
	defer serverConn.Close()
	defer clientConn.Close()

	factory := NewFactory(muxerif.DefaultConfig())

	// 创建服务端 Muxer
	muxer, err := factory.NewMuxer(serverConn, true)
	require.NoError(t, err)
	require.NotNil(t, muxer)
	defer muxer.Close()

	// 验证是服务端
	if m, ok := muxer.(*Muxer); ok {
		assert.True(t, m.IsServer())
	}
}

func TestFactoryNewMuxerClient(t *testing.T) {
	serverConn, clientConn := createConnPairForFactory(t)
	defer serverConn.Close()
	defer clientConn.Close()

	factory := NewFactory(muxerif.DefaultConfig())

	// 创建客户端 Muxer
	muxer, err := factory.NewMuxer(clientConn, false)
	require.NoError(t, err)
	require.NotNil(t, muxer)
	defer muxer.Close()

	// 验证是客户端
	if m, ok := muxer.(*Muxer); ok {
		assert.False(t, m.IsServer())
	}
}

func TestFactoryProtocol(t *testing.T) {
	factory := NewFactory(muxerif.DefaultConfig())
	assert.Equal(t, "yamux", factory.Protocol())
}

func TestFactoryConfig(t *testing.T) {
	config := muxerif.Config{
		MaxStreams:          100,
		MaxStreamWindowSize: 512 * 1024,
	}

	factory := NewFactory(config)

	returnedConfig := factory.Config()
	assert.Equal(t, config.MaxStreams, returnedConfig.MaxStreams)
	assert.Equal(t, config.MaxStreamWindowSize, returnedConfig.MaxStreamWindowSize)
}

func TestFactoryYamuxConfig(t *testing.T) {
	config := muxerif.DefaultConfig()
	factory := NewFactory(config)

	yamuxCfg := factory.YamuxConfig()
	assert.NotNil(t, yamuxCfg)
	assert.Equal(t, config.MaxStreams, yamuxCfg.AcceptBacklog)
}

func TestFactoryNewMuxerPair(t *testing.T) {
	serverConn, clientConn := createConnPairForFactory(t)
	defer serverConn.Close()
	defer clientConn.Close()

	factory := NewFactory(muxerif.DefaultConfig())

	// 创建服务端 Muxer
	serverMuxer, err := factory.NewMuxer(serverConn, true)
	require.NoError(t, err)
	defer serverMuxer.Close()

	// 创建客户端 Muxer
	clientMuxer, err := factory.NewMuxer(clientConn, false)
	require.NoError(t, err)
	defer clientMuxer.Close()

	// 验证两边都能工作
	assert.False(t, serverMuxer.IsClosed())
	assert.False(t, clientMuxer.IsClosed())
}

func TestFactoryWithCustomYamuxConfig(t *testing.T) {
	yamuxCfg := &yamux.Config{
		AcceptBacklog:       64,
		EnableKeepAlive:     false,
		MaxStreamWindowSize: 128 * 1024,
	}

	factory := NewFactoryWithYamuxConfig(yamuxCfg)

	assert.Equal(t, 64, factory.Config().MaxStreams)
	assert.Equal(t, uint32(128*1024), factory.Config().MaxStreamWindowSize)
	assert.False(t, factory.Config().EnableKeepAlive)
}

// createConnPairForFactory 创建一对连接（用于 factory 测试）
func createConnPairForFactory(t *testing.T) (net.Conn, net.Conn) {
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

// ============================================================================
//                              针对性修复测试
// ============================================================================

func TestFactory_NewMuxer_NilConn(t *testing.T) {
	factory := NewFactory(muxerif.DefaultConfig())

	// 测试 nil 连接
	muxer, err := factory.NewMuxer(nil, true)
	assert.Error(t, err)
	assert.Nil(t, muxer)
	assert.Contains(t, err.Error(), "nil")

	muxer, err = factory.NewMuxer(nil, false)
	assert.Error(t, err)
	assert.Nil(t, muxer)
}

