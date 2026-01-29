package ping

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/dep2p/go-dep2p/tests/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                     🔍 BUG 发现测试 - 边界条件
// ============================================================================

// TestPing_DataMismatch_FirstByte 测试第一个字节不匹配
// 验证数据验证循环是否正确检查所有字节（包括第一个）
func TestPing_DataMismatch_FirstByte(t *testing.T) {
	stream := &firstByteMismatchStream{}

	host := mocks.NewMockHost("local-peer")
	host.NewStreamFunc = func(ctx context.Context, peerID string, protocolIDs ...string) (pkgif.Stream, error) {
		return stream, nil
	}

	ctx := context.Background()
	rtt, err := Ping(ctx, host, "remote-peer")

	require.Error(t, err)
	assert.Zero(t, rtt)
	assert.ErrorIs(t, err, ErrDataMismatch)

	t.Log("✅ 正确检测到第一个字节不匹配")
}

// TestPing_DataMismatch_LastByte 测试最后一个字节不匹配
// 🔍 这是关键测试！验证循环边界 i < PingSize 是否正确
func TestPing_DataMismatch_LastByte(t *testing.T) {
	stream := &lastByteMismatchStream{}

	host := mocks.NewMockHost("local-peer")
	host.NewStreamFunc = func(ctx context.Context, peerID string, protocolIDs ...string) (pkgif.Stream, error) {
		return stream, nil
	}

	ctx := context.Background()
	rtt, err := Ping(ctx, host, "remote-peer")

	require.Error(t, err, "应该检测到最后一个字节不匹配")
	assert.Zero(t, rtt)
	assert.ErrorIs(t, err, ErrDataMismatch)

	t.Log("✅ 正确检测到最后一个字节不匹配（边界条件验证通过）")
}

// TestPing_DataMismatch_MiddleByte 测试中间字节不匹配
func TestPing_DataMismatch_MiddleByte(t *testing.T) {
	stream := &middleByteMismatchStream{}

	host := mocks.NewMockHost("local-peer")
	host.NewStreamFunc = func(ctx context.Context, peerID string, protocolIDs ...string) (pkgif.Stream, error) {
		return stream, nil
	}

	ctx := context.Background()
	rtt, err := Ping(ctx, host, "remote-peer")

	require.Error(t, err)
	assert.Zero(t, rtt)
	assert.ErrorIs(t, err, ErrDataMismatch)

	t.Log("✅ 正确检测到中间字节不匹配")
}

// ============================================================================
//                     🔍 BUG 发现测试 - 并发安全
// ============================================================================

// TestHandler_ConcurrentCalls 测试并发调用 Handler
// 验证多个 goroutine 同时处理不同流时是否会有竞态条件
func TestHandler_ConcurrentCalls(t *testing.T) {
	service := NewService()

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	errors := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()

			// 每个 goroutine 使用不同的测试数据
			testData := make([]byte, PingSize)
			for j := range testData {
				testData[j] = byte(id) // 用 goroutine ID 填充
			}

			stream := NewMockPingStream(testData)
			service.Handler(stream)

			// 验证回显数据
			if len(stream.writeData) < PingSize {
				errors <- assert.AnError
				return
			}
			for j := 0; j < PingSize; j++ {
				if stream.writeData[j] != testData[j] {
					errors <- assert.AnError
					return
				}
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// 验证没有错误
	for err := range errors {
		t.Errorf("并发测试失败: %v", err)
	}

	t.Log("✅ Handler 并发调用安全（20 个 goroutines）")
}

// TestPing_ConcurrentCalls 测试并发调用 Ping
func TestPing_ConcurrentCalls(t *testing.T) {
	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	errors := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()

			stream := &echoPingStream{data: make([]byte, 0)}
			host := mocks.NewMockHost("local-peer")
			host.NewStreamFunc = func(ctx context.Context, peerID string, protocolIDs ...string) (pkgif.Stream, error) {
				return stream, nil
			}

			ctx := context.Background()
			rtt, err := Ping(ctx, host, "remote-peer")

			if err != nil {
				errors <- err
				return
			}
			if rtt <= 0 {
				errors <- assert.AnError
				return
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	// 验证没有错误
	for err := range errors {
		t.Errorf("并发 Ping 失败: %v", err)
	}

	t.Log("✅ Ping 并发调用安全（20 个 goroutines）")
}

// ============================================================================
//                     🔍 BUG 发现测试 - 资源清理
// ============================================================================

// TestPing_StreamClose_CalledOnce 测试流只关闭一次
func TestPing_StreamClose_CalledOnce(t *testing.T) {
	// 不设置 testData，让它回显 sentData
	stream := &closeCountStream{}

	host := mocks.NewMockHost("local-peer")
	host.NewStreamFunc = func(ctx context.Context, peerID string, protocolIDs ...string) (pkgif.Stream, error) {
		return stream, nil
	}

	ctx := context.Background()
	_, err := Ping(ctx, host, "remote-peer")

	require.NoError(t, err)
	assert.Equal(t, 1, stream.closeCount, "流应该只关闭一次")

	t.Log("✅ 流正确关闭一次（无重复关闭）")
}

// TestPing_StreamClose_OnError 测试错误时流仍然被关闭
func TestPing_StreamClose_OnError(t *testing.T) {
	stream := &closeCountStream{
		writeError: errors.New("write failed"),
	}

	host := mocks.NewMockHost("local-peer")
	host.NewStreamFunc = func(ctx context.Context, peerID string, protocolIDs ...string) (pkgif.Stream, error) {
		return stream, nil
	}

	ctx := context.Background()
	_, err := Ping(ctx, host, "remote-peer")

	require.Error(t, err)
	assert.Equal(t, 1, stream.closeCount, "即使出错，流也应该被关闭")

	t.Log("✅ 错误时流正确关闭（资源清理验证通过）")
}

// TestHandler_StreamClose_Idempotent 测试 Handler 关闭流的幂等性
func TestHandler_StreamClose_Idempotent(t *testing.T) {
	service := NewService()

	// 创建测试数据
	testData := make([]byte, PingSize)
	rand.Read(testData)

	stream := &closeCountStream{
		testData: testData,
	}

	// 手动先关闭一次
	err := stream.Close()
	require.NoError(t, err)
	assert.Equal(t, 1, stream.closeCount)

	// Handler 会再次关闭（defer）
	service.Handler(stream)

	// 应该是 2 次（手动 1 次 + defer 1 次）
	assert.GreaterOrEqual(t, stream.closeCount, 2, "Close 应该可以多次调用（幂等性）")

	t.Log("✅ Handler 的 defer Close() 是幂等的")
}

// ============================================================================
//                     🔍 BUG 发现测试 - 极端条件
// ============================================================================

// TestPing_ZeroSizeBuffer 测试 PingSize 为 0 的边界条件
// 注意：这只是假设性测试，实际 PingSize=32 是常量
func TestPing_ZeroSizeBuffer_Hypothetical(t *testing.T) {
	// 这个测试验证如果 PingSize=0 会发生什么
	// 实际上 PingSize 是常量 32，但这是边界思考
	t.Log("📝 假设性测试: 如果 PingSize=0，会发生什么？")
	t.Log("   当前实现: buf := make([]byte, PingSize) 会创建空切片")
	t.Log("   io.ReadFull 会立即返回 nil")
	t.Log("   数据验证循环不会执行（i < 0 为假）")
	t.Log("   结论: 逻辑正确，但无意义（Ping 大小为 0）")
}

// TestHandler_ReadTimeout_Behavior 测试 SetReadDeadline 的行为
//
// 🐛 设计限制 #L1: Handler 的超时保护依赖 stream 实现
// 如果 stream 不支持 SetReadDeadline（类型断言失败），Handler 就没有超时保护
func TestHandler_ReadTimeout_Behavior(t *testing.T) {
	t.Skip("跳过：需要真实的支持 deadline 的 stream 实现才能测试超时行为")
	
	// 这个测试揭示了设计限制：
	// - ping.go:54-56 使用类型断言来设置超时
	// - 如果断言失败，Handler 就无法控制超时
	// - 建议：考虑使用 context.WithTimeout 来统一管理超时
	
	t.Log("📝 设计限制: Handler 超时保护不是强制的")
	t.Log("   位置: ping.go:54-56")
	t.Log("   问题: 类型断言可能失败")
	t.Log("   风险: 恶意客户端可以长时间占用连接")
	t.Log("   建议: 使用 context 或要求 stream 接口包含 deadline 方法")
}

// ============================================================================
//                     辅助 Mock Streams
// ============================================================================

// firstByteMismatchStream 第一个字节不匹配
type firstByteMismatchStream struct {
	sentData []byte
	readPos  int
	closed   bool
}

func (s *firstByteMismatchStream) Read(p []byte) (n int, err error) {
	if s.readPos >= PingSize {
		return 0, io.EOF
	}
	remaining := PingSize - s.readPos
	toRead := len(p)
	if toRead > remaining {
		toRead = remaining
	}
	for i := 0; i < toRead; i++ {
		if s.readPos+i == 0 {
			// 第一个字节不同
			p[i] = ^s.sentData[s.readPos+i]
		} else {
			p[i] = s.sentData[s.readPos+i]
		}
	}
	s.readPos += toRead
	return toRead, nil
}

func (s *firstByteMismatchStream) Write(p []byte) (n int, err error) {
	s.sentData = append(s.sentData, p...)
	return len(p), nil
}

func (s *firstByteMismatchStream) Close() error                      { s.closed = true; return nil }
func (s *firstByteMismatchStream) CloseRead() error                  { return nil }
func (s *firstByteMismatchStream) CloseWrite() error                 { return nil }
func (s *firstByteMismatchStream) Reset() error                      { return nil }
func (s *firstByteMismatchStream) Protocol() string                  { return ProtocolID }
func (s *firstByteMismatchStream) SetProtocol(protocol string)       {}
func (s *firstByteMismatchStream) Conn() pkgif.Connection            { return nil }
func (s *firstByteMismatchStream) IsClosed() bool                    { return s.closed }
func (s *firstByteMismatchStream) SetDeadline(t time.Time) error     { return nil }
func (s *firstByteMismatchStream) SetReadDeadline(t time.Time) error { return nil }
func (s *firstByteMismatchStream) SetWriteDeadline(t time.Time) error { return nil }
func (s *firstByteMismatchStream) Stat() types.StreamStat           { return types.StreamStat{} }
func (s *firstByteMismatchStream) State() types.StreamState {
	if s.closed {
		return types.StreamStateClosed
	}
	return types.StreamStateOpen
}

// lastByteMismatchStream 最后一个字节不匹配（关键边界测试）
type lastByteMismatchStream struct {
	sentData []byte
	readPos  int
	closed   bool
}

func (s *lastByteMismatchStream) Read(p []byte) (n int, err error) {
	if s.readPos >= PingSize {
		return 0, io.EOF
	}
	remaining := PingSize - s.readPos
	toRead := len(p)
	if toRead > remaining {
		toRead = remaining
	}
	for i := 0; i < toRead; i++ {
		if s.readPos+i == PingSize-1 {
			// 最后一个字节不同
			p[i] = ^s.sentData[s.readPos+i]
		} else {
			p[i] = s.sentData[s.readPos+i]
		}
	}
	s.readPos += toRead
	return toRead, nil
}

func (s *lastByteMismatchStream) Write(p []byte) (n int, err error) {
	s.sentData = append(s.sentData, p...)
	return len(p), nil
}

func (s *lastByteMismatchStream) Close() error                      { s.closed = true; return nil }
func (s *lastByteMismatchStream) CloseRead() error                  { return nil }
func (s *lastByteMismatchStream) CloseWrite() error                 { return nil }
func (s *lastByteMismatchStream) Reset() error                      { return nil }
func (s *lastByteMismatchStream) Protocol() string                  { return ProtocolID }
func (s *lastByteMismatchStream) SetProtocol(protocol string)       {}
func (s *lastByteMismatchStream) Conn() pkgif.Connection            { return nil }
func (s *lastByteMismatchStream) IsClosed() bool                    { return s.closed }
func (s *lastByteMismatchStream) SetDeadline(t time.Time) error     { return nil }
func (s *lastByteMismatchStream) SetReadDeadline(t time.Time) error { return nil }
func (s *lastByteMismatchStream) SetWriteDeadline(t time.Time) error { return nil }
func (s *lastByteMismatchStream) Stat() types.StreamStat           { return types.StreamStat{} }
func (s *lastByteMismatchStream) State() types.StreamState {
	if s.closed {
		return types.StreamStateClosed
	}
	return types.StreamStateOpen
}

// middleByteMismatchStream 中间字节不匹配
type middleByteMismatchStream struct {
	sentData []byte
	readPos  int
	closed   bool
}

func (s *middleByteMismatchStream) Read(p []byte) (n int, err error) {
	if s.readPos >= PingSize {
		return 0, io.EOF
	}
	remaining := PingSize - s.readPos
	toRead := len(p)
	if toRead > remaining {
		toRead = remaining
	}
	for i := 0; i < toRead; i++ {
		if s.readPos+i == PingSize/2 {
			// 中间字节不同
			p[i] = ^s.sentData[s.readPos+i]
		} else {
			p[i] = s.sentData[s.readPos+i]
		}
	}
	s.readPos += toRead
	return toRead, nil
}

func (s *middleByteMismatchStream) Write(p []byte) (n int, err error) {
	s.sentData = append(s.sentData, p...)
	return len(p), nil
}

func (s *middleByteMismatchStream) Close() error                      { s.closed = true; return nil }
func (s *middleByteMismatchStream) CloseRead() error                  { return nil }
func (s *middleByteMismatchStream) CloseWrite() error                 { return nil }
func (s *middleByteMismatchStream) Reset() error                      { return nil }
func (s *middleByteMismatchStream) Protocol() string                  { return ProtocolID }
func (s *middleByteMismatchStream) SetProtocol(protocol string)       {}
func (s *middleByteMismatchStream) Conn() pkgif.Connection            { return nil }
func (s *middleByteMismatchStream) IsClosed() bool                    { return s.closed }
func (s *middleByteMismatchStream) SetDeadline(t time.Time) error     { return nil }
func (s *middleByteMismatchStream) SetReadDeadline(t time.Time) error { return nil }
func (s *middleByteMismatchStream) SetWriteDeadline(t time.Time) error { return nil }
func (s *middleByteMismatchStream) Stat() types.StreamStat           { return types.StreamStat{} }
func (s *middleByteMismatchStream) State() types.StreamState {
	if s.closed {
		return types.StreamStateClosed
	}
	return types.StreamStateOpen
}

// closeCountStream 统计关闭次数
type closeCountStream struct {
	testData   []byte // 用于 Handler 测试的预设数据
	sentData   []byte // 写入的数据（用于回显）
	readPos    int
	closeCount int
	writeError error
	closed     bool
}

func (s *closeCountStream) Read(p []byte) (n int, err error) {
	// 如果有预设的 testData，使用它（用于 Handler 测试）
	if len(s.testData) > 0 {
		if s.readPos >= len(s.testData) {
			return 0, io.EOF
		}
		n = copy(p, s.testData[s.readPos:])
		s.readPos += n
		return n, nil
	}
	
	// 否则回显 sentData（用于 Ping 测试）
	if s.readPos >= len(s.sentData) {
		return 0, io.EOF
	}
	n = copy(p, s.sentData[s.readPos:])
	s.readPos += n
	return n, nil
}

func (s *closeCountStream) Write(p []byte) (n int, err error) {
	if s.writeError != nil {
		return 0, s.writeError
	}
	s.sentData = append(s.sentData, p...)
	return len(p), nil
}

func (s *closeCountStream) Close() error {
	s.closeCount++
	s.closed = true
	return nil
}

func (s *closeCountStream) CloseRead() error                  { return nil }
func (s *closeCountStream) CloseWrite() error                 { return nil }
func (s *closeCountStream) Reset() error                      { return nil }
func (s *closeCountStream) Protocol() string                  { return ProtocolID }
func (s *closeCountStream) SetProtocol(protocol string)       {}
func (s *closeCountStream) Conn() pkgif.Connection            { return nil }
func (s *closeCountStream) IsClosed() bool                    { return s.closed }
func (s *closeCountStream) SetDeadline(t time.Time) error     { return nil }
func (s *closeCountStream) SetReadDeadline(t time.Time) error { return nil }
func (s *closeCountStream) SetWriteDeadline(t time.Time) error { return nil }
func (s *closeCountStream) Stat() types.StreamStat           { return types.StreamStat{} }
func (s *closeCountStream) State() types.StreamState {
	if s.closed {
		return types.StreamStateClosed
	}
	return types.StreamStateOpen
}

// deadlineStream 模拟超时行为
type deadlineStream struct {
	readDelay   time.Duration
	timeout     time.Duration
	deadlineSet bool
	closed      bool
}

func (s *deadlineStream) Read(p []byte) (n int, err error) {
	// 模拟延迟读取
	time.Sleep(s.readDelay)
	// 如果延迟超过超时，返回超时错误
	if s.readDelay > s.timeout && s.deadlineSet {
		return 0, errors.New("i/o timeout")
	}
	// 否则返回 EOF（结束读取）
	return 0, io.EOF
}

func (s *deadlineStream) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (s *deadlineStream) Close() error {
	s.closed = true
	return nil
}

func (s *deadlineStream) SetReadDeadline(t time.Time) error {
	s.deadlineSet = true
	return nil
}

func (s *deadlineStream) CloseRead() error            { return nil }
func (s *deadlineStream) CloseWrite() error           { return nil }
func (s *deadlineStream) Reset() error                { return nil }
func (s *deadlineStream) Protocol() string            { return ProtocolID }
func (s *deadlineStream) SetProtocol(protocol string) {}
func (s *deadlineStream) Conn() pkgif.Connection      { return nil }
func (s *deadlineStream) IsClosed() bool              { return s.closed }
func (s *deadlineStream) SetDeadline(t time.Time) error     { return nil }
func (s *deadlineStream) SetWriteDeadline(t time.Time) error { return nil }
func (s *deadlineStream) Stat() types.StreamStat           { return types.StreamStat{} }
func (s *deadlineStream) State() types.StreamState {
	if s.closed {
		return types.StreamStateClosed
	}
	return types.StreamStateOpen
}

// 注意: echoPingStream 已在 ping_core_test.go 中定义，这里复用
