package protocol

import (
	"bytes"
	"context"
	"io"
	"testing"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/multiaddr"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockConnection 模拟连接（实现完整的 pkgif.Connection 接口）
type mockConnection struct {
	reader io.Reader
	writer io.Writer
	closed bool
}

func (m *mockConnection) LocalPeer() types.PeerID { return "local-peer" }
func (m *mockConnection) LocalMultiaddr() types.Multiaddr {
	ma, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/1234")
	return ma
}
func (m *mockConnection) RemotePeer() types.PeerID { return "remote-peer" }
func (m *mockConnection) RemoteMultiaddr() types.Multiaddr {
	ma, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/5678")
	return ma
}
func (m *mockConnection) NewStream(ctx context.Context) (pkgif.Stream, error) { return nil, nil }
func (m *mockConnection) NewStreamWithPriority(ctx context.Context, priority int) (pkgif.Stream, error) {
	return m.NewStream(ctx)
}
func (m *mockConnection) SupportsStreamPriority() bool        { return false }
func (m *mockConnection) AcceptStream() (pkgif.Stream, error) { return nil, nil }
func (m *mockConnection) GetStreams() []pkgif.Stream          { return nil }
func (m *mockConnection) Stat() pkgif.ConnectionStat          { return pkgif.ConnectionStat{} }
func (m *mockConnection) Close() error                        { m.closed = true; return nil }
func (m *mockConnection) IsClosed() bool                      { return m.closed }
func (m *mockConnection) ConnType() pkgif.ConnectionType      { return pkgif.ConnectionTypeDirect }
func (m *mockConnection) Read(p []byte) (n int, err error) {
	if m.reader == nil {
		return 0, io.EOF
	}
	return m.reader.Read(p)
}
func (m *mockConnection) Write(p []byte) (n int, err error) {
	if m.writer == nil {
		return 0, io.ErrClosedPipe
	}
	return m.writer.Write(p)
}

// 验证 mockConnection 实现了 Connection 接口
var _ pkgif.Connection = (*mockConnection)(nil)

// TestNegotiator_New 测试创建协商器
func TestNegotiator_New(t *testing.T) {
	registry := NewRegistry()
	negotiator := NewNegotiator(registry)

	require.NotNil(t, negotiator)
	assert.Equal(t, registry, negotiator.registry)

	t.Log("✅ Negotiator 创建成功")
}

// TestNegotiator_Interface 验证接口实现
func TestNegotiator_Interface(t *testing.T) {
	var _ pkgif.ProtocolNegotiator = (*Negotiator)(nil)
	t.Log("✅ Negotiator 实现 ProtocolNegotiator 接口")
}

// TestConnAdapter_Read 测试连接适配器读取
func TestConnAdapter_Read(t *testing.T) {
	data := []byte("hello world")
	conn := &mockConnection{reader: bytes.NewReader(data)}

	adapter := connToReadWriteCloser(conn)

	buf := make([]byte, len(data))
	n, err := adapter.Read(buf)

	assert.NoError(t, err)
	assert.Equal(t, len(data), n)
	assert.Equal(t, data, buf)

	t.Log("✅ connAdapter Read 成功")
}

// TestConnAdapter_Write 测试连接适配器写入
func TestConnAdapter_Write(t *testing.T) {
	buf := &bytes.Buffer{}
	conn := &mockConnection{writer: buf}

	adapter := connToReadWriteCloser(conn)

	data := []byte("hello world")
	n, err := adapter.Write(data)

	assert.NoError(t, err)
	assert.Equal(t, len(data), n)
	assert.Equal(t, data, buf.Bytes())

	t.Log("✅ connAdapter Write 成功")
}

// TestConnAdapter_Close 测试连接适配器关闭
func TestConnAdapter_Close(t *testing.T) {
	conn := &mockConnection{}

	adapter := connToReadWriteCloser(conn)

	err := adapter.Close()
	assert.NoError(t, err)
	assert.True(t, conn.closed)

	t.Log("✅ connAdapter Close 成功")
}

// TestConnAdapter_ReadNoReader 测试无读取器
func TestConnAdapter_ReadNoReader(t *testing.T) {
	conn := &mockConnection{reader: nil}

	adapter := connToReadWriteCloser(conn)

	buf := make([]byte, 10)
	_, err := adapter.Read(buf)
	assert.Equal(t, io.EOF, err)

	t.Log("✅ connAdapter Read 无读取器测试通过")
}

// TestConnAdapter_WriteNoWriter 测试无写入器
func TestConnAdapter_WriteNoWriter(t *testing.T) {
	conn := &mockConnection{writer: nil}

	adapter := connToReadWriteCloser(conn)

	_, err := adapter.Write([]byte("test"))
	assert.Equal(t, io.ErrClosedPipe, err)

	t.Log("✅ connAdapter Write 无写入器测试通过")
}
