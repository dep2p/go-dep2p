package ping

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/dep2p/go-dep2p/tests/mocks"
)

// ============================================================================
//                     Ping 客户端函数测试
// ============================================================================

// TestPing_Success 测试 Ping 成功
func TestPing_Success(t *testing.T) {
	// 创建一个模拟回显的 stream
	stream := &echoPingStream{
		data: make([]byte, 0),
	}

	host := mocks.NewMockHost("local-peer")
	host.NewStreamFunc = func(ctx context.Context, peerID string, protocolIDs ...string) (pkgif.Stream, error) {
		require.Contains(t, protocolIDs, ProtocolID)
		return stream, nil
	}

	ctx := context.Background()
	rtt, err := Ping(ctx, host, "remote-peer")

	require.NoError(t, err)
	assert.Greater(t, rtt, time.Duration(0), "RTT should be positive")
	assert.True(t, stream.closed, "stream should be closed")

	t.Logf("✅ Ping 成功, RTT: %v", rtt)
}

// TestPing_NewStreamError 测试创建流失败
func TestPing_NewStreamError(t *testing.T) {
	host := mocks.NewMockHost("local-peer")
	host.NewStreamFunc = func(ctx context.Context, peerID string, protocolIDs ...string) (pkgif.Stream, error) {
		return nil, errors.New("connection refused")
	}

	ctx := context.Background()
	rtt, err := Ping(ctx, host, "remote-peer")

	require.Error(t, err)
	assert.Zero(t, rtt)
	assert.Contains(t, err.Error(), "connection refused")
}

// TestPing_WriteError 测试写入失败
func TestPing_WriteError(t *testing.T) {
	stream := mocks.NewMockStream()
	stream.WriteFunc = func(p []byte) (n int, err error) {
		return 0, errors.New("write failed")
	}

	host := mocks.NewMockHost("local-peer")
	host.NewStreamFunc = func(ctx context.Context, peerID string, protocolIDs ...string) (pkgif.Stream, error) {
		return stream, nil
	}

	ctx := context.Background()
	rtt, err := Ping(ctx, host, "remote-peer")

	require.Error(t, err)
	assert.Zero(t, rtt)
	assert.Contains(t, err.Error(), "write failed")
}

// TestPing_ReadError 测试读取失败
func TestPing_ReadError(t *testing.T) {
	stream := mocks.NewMockStream()
	stream.WriteFunc = func(p []byte) (n int, err error) {
		return len(p), nil
	}
	stream.ReadFunc = func(p []byte) (n int, err error) {
		return 0, errors.New("read failed")
	}

	host := mocks.NewMockHost("local-peer")
	host.NewStreamFunc = func(ctx context.Context, peerID string, protocolIDs ...string) (pkgif.Stream, error) {
		return stream, nil
	}

	ctx := context.Background()
	rtt, err := Ping(ctx, host, "remote-peer")

	require.Error(t, err)
	assert.Zero(t, rtt)
	assert.Contains(t, err.Error(), "read failed")
}

// TestPing_DataMismatch 测试数据不匹配（BUG #B14 验证）
func TestPing_DataMismatch(t *testing.T) {
	// 创建一个返回错误数据的 stream
	stream := &mismatchPingStream{}

	host := mocks.NewMockHost("local-peer")
	host.NewStreamFunc = func(ctx context.Context, peerID string, protocolIDs ...string) (pkgif.Stream, error) {
		return stream, nil
	}

	ctx := context.Background()
	rtt, err := Ping(ctx, host, "remote-peer")

	require.Error(t, err)
	assert.Zero(t, rtt)
	// 验证使用专门的错误类型（BUG #B14 修复后）
	assert.ErrorIs(t, err, ErrDataMismatch)
}

// TestPing_PartialRead 测试部分读取（EOF）
func TestPing_PartialRead(t *testing.T) {
	stream := mocks.NewMockStream()
	stream.WriteFunc = func(p []byte) (n int, err error) {
		return len(p), nil
	}
	// 只返回 16 字节而不是 32 字节
	stream.ReadData = make([]byte, 16)
	rand.Read(stream.ReadData)

	host := mocks.NewMockHost("local-peer")
	host.NewStreamFunc = func(ctx context.Context, peerID string, protocolIDs ...string) (pkgif.Stream, error) {
		return stream, nil
	}

	ctx := context.Background()
	rtt, err := Ping(ctx, host, "remote-peer")

	require.Error(t, err)
	assert.Zero(t, rtt)
	// io.ReadFull 应该返回 EOF 或 ErrUnexpectedEOF
	assert.True(t, errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF))
}

// TestPing_ContextCancellation 测试上下文取消
func TestPing_ContextCancellation(t *testing.T) {
	host := mocks.NewMockHost("local-peer")
	host.NewStreamFunc = func(ctx context.Context, peerID string, protocolIDs ...string) (pkgif.Stream, error) {
		// 检查上下文是否已取消
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New("should not reach here")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	rtt, err := Ping(ctx, host, "remote-peer")

	require.Error(t, err)
	assert.Zero(t, rtt)
}

// ============================================================================
//                     Handler 写入失败测试
// ============================================================================

// TestHandler_WriteError 测试 Handler 写入失败
func TestHandler_WriteError(t *testing.T) {
	service := NewService()

	stream := &writeErrorStream{
		readData: make([]byte, PingSize),
	}
	rand.Read(stream.readData)

	// Handler 应该在写入失败时返回（不 panic）
	service.Handler(stream)

	assert.True(t, stream.closed, "stream should be closed")
	assert.Empty(t, stream.writeData, "no data should be written")
}

// ============================================================================
//                     辅助 Mock
// ============================================================================

// echoPingStream 回显 stream（模拟正确的服务器响应）
type echoPingStream struct {
	data      []byte
	sentData  []byte
	closed    bool
	readPos   int
}

func (s *echoPingStream) Read(p []byte) (n int, err error) {
	// 返回之前写入的数据（回显）
	if s.readPos >= len(s.sentData) {
		return 0, io.EOF
	}
	n = copy(p, s.sentData[s.readPos:])
	s.readPos += n
	return n, nil
}

func (s *echoPingStream) Write(p []byte) (n int, err error) {
	// 保存发送的数据，用于回显
	s.sentData = append(s.sentData, p...)
	return len(p), nil
}

func (s *echoPingStream) Close() error {
	s.closed = true
	return nil
}

func (s *echoPingStream) CloseRead() error            { return nil }
func (s *echoPingStream) CloseWrite() error           { return nil }
func (s *echoPingStream) Reset() error                { s.closed = true; return nil }
func (s *echoPingStream) Protocol() string            { return ProtocolID }
func (s *echoPingStream) SetProtocol(protocol string) {}
func (s *echoPingStream) Conn() pkgif.Connection      { return nil }
func (s *echoPingStream) IsClosed() bool              { return s.closed }
func (s *echoPingStream) SetDeadline(t time.Time) error      { return nil }
func (s *echoPingStream) SetReadDeadline(t time.Time) error  { return nil }
func (s *echoPingStream) SetWriteDeadline(t time.Time) error { return nil }
func (s *echoPingStream) Stat() types.StreamStat             { return types.StreamStat{} }
func (s *echoPingStream) State() types.StreamState {
	if s.closed {
		return types.StreamStateClosed
	}
	return types.StreamStateOpen
}

// mismatchPingStream 返回错误数据的 stream
type mismatchPingStream struct {
	sentData []byte
	readPos  int
	closed   bool
}

func (s *mismatchPingStream) Read(p []byte) (n int, err error) {
	if s.readPos >= PingSize {
		return 0, io.EOF
	}
	// 返回与发送数据不同的数据
	remaining := PingSize - s.readPos
	toRead := len(p)
	if toRead > remaining {
		toRead = remaining
	}
	for i := 0; i < toRead; i++ {
		p[i] = byte(0xFF) // 全部填充 0xFF（与原数据不同）
	}
	s.readPos += toRead
	return toRead, nil
}

func (s *mismatchPingStream) Write(p []byte) (n int, err error) {
	s.sentData = append(s.sentData, p...)
	return len(p), nil
}

func (s *mismatchPingStream) Close() error                      { s.closed = true; return nil }
func (s *mismatchPingStream) CloseRead() error                  { return nil }
func (s *mismatchPingStream) CloseWrite() error                 { return nil }
func (s *mismatchPingStream) Reset() error                      { return nil }
func (s *mismatchPingStream) Protocol() string                  { return ProtocolID }
func (s *mismatchPingStream) SetProtocol(protocol string)       {}
func (s *mismatchPingStream) Conn() pkgif.Connection            { return nil }
func (s *mismatchPingStream) IsClosed() bool                    { return s.closed }
func (s *mismatchPingStream) SetDeadline(t time.Time) error     { return nil }
func (s *mismatchPingStream) SetReadDeadline(t time.Time) error { return nil }
func (s *mismatchPingStream) SetWriteDeadline(t time.Time) error{ return nil }
func (s *mismatchPingStream) Stat() types.StreamStat            { return types.StreamStat{} }
func (s *mismatchPingStream) State() types.StreamState {
	if s.closed {
		return types.StreamStateClosed
	}
	return types.StreamStateOpen
}

// writeErrorStream 写入失败的 stream
type writeErrorStream struct {
	readData  []byte
	writeData []byte
	readPos   int
	closed    bool
}

func (s *writeErrorStream) Read(p []byte) (n int, err error) {
	if s.readPos >= len(s.readData) {
		return 0, io.EOF
	}
	n = copy(p, s.readData[s.readPos:])
	s.readPos += n
	return n, nil
}

func (s *writeErrorStream) Write(p []byte) (n int, err error) {
	return 0, errors.New("write error")
}

func (s *writeErrorStream) Close() error                      { s.closed = true; return nil }
func (s *writeErrorStream) CloseRead() error                  { return nil }
func (s *writeErrorStream) CloseWrite() error                 { return nil }
func (s *writeErrorStream) Reset() error                      { return nil }
func (s *writeErrorStream) Protocol() string                  { return ProtocolID }
func (s *writeErrorStream) SetProtocol(protocol string)       {}
func (s *writeErrorStream) Conn() pkgif.Connection            { return nil }
func (s *writeErrorStream) IsClosed() bool                    { return s.closed }
func (s *writeErrorStream) SetDeadline(t time.Time) error     { return nil }
func (s *writeErrorStream) SetReadDeadline(t time.Time) error { return nil }
func (s *writeErrorStream) SetWriteDeadline(t time.Time) error{ return nil }
func (s *writeErrorStream) Stat() types.StreamStat            { return types.StreamStat{} }
func (s *writeErrorStream) State() types.StreamState {
	if s.closed {
		return types.StreamStateClosed
	}
	return types.StreamStateOpen
}
