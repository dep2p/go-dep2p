package ping

import (
	"bytes"
	"crypto/rand"
	"io"
	"testing"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPing_Handler_Echo 测试 Ping Handler 回显
func TestPing_Handler_Echo(t *testing.T) {
	service := NewService()
	require.NotNil(t, service)
	
	// 生成测试数据
	testData := make([]byte, PingSize)
	_, err := rand.Read(testData)
	require.NoError(t, err)
	
	// 创建 mock stream
	stream := NewMockPingStream(testData)
	
	// 在 goroutine 中运行 handler
	done := make(chan bool)
	go func() {
		service.Handler(stream)
		done <- true
	}()
	
	// 等待处理完成或超时
	select {
	case <-done:
		// Handler 完成
	}
	
	// 验证回显数据
	assert.Equal(t, testData, stream.writeData[:PingSize])
	
	t.Log("✅ Ping Handler 回显正确")
}

// TestPingService_New 测试创建服务
func TestPingService_New(t *testing.T) {
	service := NewService()
	require.NotNil(t, service)
	
	t.Log("✅ PingService 创建成功")
}

// TestPing_ProtocolID 测试协议 ID
func TestPing_ProtocolID(t *testing.T) {
	assert.Equal(t, "/dep2p/sys/ping/1.0.0", ProtocolID)
	assert.Equal(t, 32, PingSize)
	
	t.Log("✅ 协议常量正确")
}

// MockPingStream 模拟 Ping 流
type MockPingStream struct {
	readData  []byte
	writeData []byte
	readPos   int
	closed    bool
}

func NewMockPingStream(data []byte) *MockPingStream {
	return &MockPingStream{
		readData: data,
	}
}

func (s *MockPingStream) Read(p []byte) (n int, err error) {
	if s.readPos >= len(s.readData) {
		return 0, io.EOF
	}
	n = copy(p, s.readData[s.readPos:])
	s.readPos += n
	return n, nil
}

func (s *MockPingStream) Write(p []byte) (n int, err error) {
	s.writeData = append(s.writeData, p...)
	
	// 写满 32 字节后关闭流（模拟一次 ping）
	if len(s.writeData) >= PingSize {
		s.closed = true
	}
	
	return len(p), nil
}

func (s *MockPingStream) Close() error {
	s.closed = true
	return nil
}

func (s *MockPingStream) CloseRead() error {
	return nil
}

func (s *MockPingStream) CloseWrite() error {
	return nil
}

func (s *MockPingStream) IsClosed() bool {
	return s.closed
}

func (s *MockPingStream) Reset() error {
	s.closed = true
	return nil
}

func (s *MockPingStream) Protocol() string {
	return ProtocolID
}

func (s *MockPingStream) SetProtocol(protocol string) {
	// MockPingStream 不需要存储协议，忽略
}

func (s *MockPingStream) Conn() pkgif.Connection {
	return nil
}

func (s *MockPingStream) SetDeadline(t time.Time) error {
	return nil
}

func (s *MockPingStream) SetReadDeadline(t time.Time) error {
	return nil
}

func (s *MockPingStream) SetWriteDeadline(t time.Time) error {
	return nil
}

func (s *MockPingStream) Stat() types.StreamStat {
	return types.StreamStat{}
}

func (s *MockPingStream) State() types.StreamState {
	if s.closed {
		return types.StreamStateClosed
	}
	return types.StreamStateOpen
}

// TestPing_DataIntegrity 测试数据完整性
func TestPing_DataIntegrity(t *testing.T) {
	service := NewService()
	
	// 创建测试数据
	input := bytes.Repeat([]byte{0x42}, PingSize)
	stream := NewMockPingStream(input)
	
	// 运行 handler
	done := make(chan bool)
	go func() {
		service.Handler(stream)
		done <- true
	}()
	
	<-done
	
	// 验证数据完整性
	assert.Equal(t, input, stream.writeData[:PingSize])
	
	t.Log("✅ 数据完整性验证通过")
}
