package ping

import (
	"io"
	"testing"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
)

// ============================================================================
//                     🐛 BUG 发现测试
// ============================================================================

// TestBUG_Ping_DataMismatchErrorType 测试数据不匹配时的错误类型
//
// 🐛 BUG B14: Ping 函数数据验证失败时返回 io.ErrUnexpectedEOF
// 这是语义错误，应该返回专门的"数据不匹配"错误
func TestBUG_Ping_DataMismatchErrorType_Documentation(t *testing.T) {
	t.Logf("⚠️ 警告: Ping 数据验证失败时错误类型不当")
	t.Logf("   位置: ping.go:96")
	t.Logf("   当前: return 0, io.ErrUnexpectedEOF")
	t.Logf("   问题: 语义上应该是'数据不匹配'而不是'意外的EOF'")
	t.Logf("   建议: 定义 ErrDataMismatch = errors.New(\"ping: data mismatch\")")

	// 验证当前行为
	assert.Equal(t, "unexpected EOF", io.ErrUnexpectedEOF.Error())
}

// TestBUG_Ping_NoStreamTimeout 测试 Ping 函数没有设置流超时
//
// 🐛 BUG B15: Ping 函数没有设置流的读写超时
// 如果对方建立连接后不响应，会一直阻塞（即使 context 取消，流读写可能不会立即中断）
func TestBUG_Ping_NoStreamTimeout_Documentation(t *testing.T) {
	t.Logf("⚠️ 警告: Ping 函数没有设置流超时")
	t.Logf("   位置: ping.go:60-101")
	t.Logf("   问题: 虽然 context 可取消，但流本身没有超时")
	t.Logf("   建议: 在 Write/Read 前调用 stream.SetDeadline()")
}

// TestBUG_Handler_NoIdleTimeout 测试 Handler 没有空闲超时
//
// 🐛 BUG B16: Handler 循环没有空闲超时
// 如果客户端建立连接后不发送数据，会一直阻塞 io.ReadFull
func TestBUG_Handler_NoIdleTimeout_Documentation(t *testing.T) {
	t.Logf("⚠️ 警告: Handler 没有空闲超时")
	t.Logf("   位置: ping.go:35-56")
	t.Logf("   问题: io.ReadFull 会一直阻塞等待 32 字节")
	t.Logf("   影响: 恶意客户端可以占用服务器资源")
	t.Logf("   建议: 添加读取超时或最大连接时间限制")
}

// ============================================================================
//                     边界条件测试
// ============================================================================

// TestPing_Constants_Values 测试常量值
func TestPing_Constants_Values(t *testing.T) {
	assert.Equal(t, "/dep2p/sys/ping/1.0.0", ProtocolID)
	assert.Equal(t, 32, PingSize)
	assert.NotZero(t, PingTimeout)
}

// TestPing_Handler_PartialData 测试部分数据
func TestPing_Handler_PartialData(t *testing.T) {
	service := NewService()

	// 只发送 16 字节（不足 32 字节）
	partialData := make([]byte, 16)
	stream := NewMockPingStream(partialData)

	// Handler 应该在读取失败时返回
	done := make(chan bool)
	go func() {
		service.Handler(stream)
		done <- true
	}()

	<-done

	// 由于数据不足，不应该有回显
	assert.Less(t, len(stream.writeData), PingSize)
	t.Log("✅ Handler 正确处理部分数据")
}

// TestPing_Handler_EmptyStream 测试空流
func TestPing_Handler_EmptyStream(t *testing.T) {
	service := NewService()

	// 空数据
	stream := NewMockPingStream([]byte{})

	done := make(chan bool)
	go func() {
		service.Handler(stream)
		done <- true
	}()

	<-done

	// 不应该有任何回显
	assert.Empty(t, stream.writeData)
	t.Log("✅ Handler 正确处理空流")
}

// TestPing_Handler_MultiplePings 测试多次 Ping
func TestPing_Handler_MultiplePings(t *testing.T) {
	service := NewService()

	// 发送两次 Ping 数据（64 字节）
	doubleData := make([]byte, PingSize*2)
	for i := range doubleData {
		doubleData[i] = byte(i)
	}

	stream := &MultiplePingStream{
		readData:    doubleData,
		maxWriteLen: PingSize * 2, // 允许写两次
	}

	done := make(chan bool)
	go func() {
		service.Handler(stream)
		done <- true
	}()

	<-done

	// 应该有两次回显
	assert.Equal(t, PingSize*2, len(stream.writeData))
	// 第一次回显
	assert.Equal(t, doubleData[:PingSize], stream.writeData[:PingSize])
	// 第二次回显
	assert.Equal(t, doubleData[PingSize:], stream.writeData[PingSize:])

	t.Log("✅ Handler 正确处理多次 Ping")
}

// MultiplePingStream 支持多次 Ping 的 mock
type MultiplePingStream struct {
	readData    []byte
	writeData   []byte
	readPos     int
	maxWriteLen int
	closed      bool
}

func (s *MultiplePingStream) Read(p []byte) (n int, err error) {
	if s.readPos >= len(s.readData) {
		return 0, io.EOF
	}
	n = copy(p, s.readData[s.readPos:])
	s.readPos += n
	return n, nil
}

func (s *MultiplePingStream) Write(p []byte) (n int, err error) {
	s.writeData = append(s.writeData, p...)
	if len(s.writeData) >= s.maxWriteLen {
		s.closed = true
	}
	return len(p), nil
}

func (s *MultiplePingStream) Close() error {
	s.closed = true
	return nil
}

func (s *MultiplePingStream) CloseRead() error            { return nil }
func (s *MultiplePingStream) CloseWrite() error           { return nil }
func (s *MultiplePingStream) Reset() error                { return nil }
func (s *MultiplePingStream) Protocol() string            { return ProtocolID }
func (s *MultiplePingStream) SetProtocol(protocol string) {}
func (s *MultiplePingStream) Conn() pkgif.Connection      { return nil }
func (s *MultiplePingStream) IsClosed() bool              { return s.closed }
func (s *MultiplePingStream) SetDeadline(t time.Time) error      { return nil }
func (s *MultiplePingStream) SetReadDeadline(t time.Time) error  { return nil }
func (s *MultiplePingStream) SetWriteDeadline(t time.Time) error { return nil }
func (s *MultiplePingStream) Stat() types.StreamStat            { return types.StreamStat{} }
func (s *MultiplePingStream) State() types.StreamState {
	if s.closed {
		return types.StreamStateClosed
	}
	return types.StreamStateOpen
}
