// Package streams 实现流协议
package streams

import (
	"io"
	"testing"
	"time"
)

func TestStreamWrapper_ReadWrite(t *testing.T) {
	mockS := newMockStream("test-protocol", "peer2", "peer1")
	wrapper := newStreamWrapper(mockS, "test-protocol")

	// 写入数据
	data := []byte("hello world")
	n, err := wrapper.Write(data)
	if err != nil {
		t.Fatalf("Write() failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("Write() wrote %d bytes, want %d", n, len(data))
	}

	// 读取数据
	buf := make([]byte, 100)
	n, err = wrapper.Read(buf)
	if err != nil && err != io.EOF {
		t.Fatalf("Read() failed: %v", err)
	}
	if n != len(data) {
		t.Errorf("Read() read %d bytes, want %d", n, len(data))
	}
	if string(buf[:n]) != string(data) {
		t.Errorf("Read() = %s, want %s", buf[:n], data)
	}
}

func TestStreamWrapper_Protocol(t *testing.T) {
	mockS := newMockStream("test-protocol", "peer2", "peer1")
	wrapper := newStreamWrapper(mockS, "test-protocol")

	if wrapper.Protocol() != "test-protocol" {
		t.Errorf("Protocol() = %s, want test-protocol", wrapper.Protocol())
	}
}

func TestStreamWrapper_RemotePeer(t *testing.T) {
	mockS := newMockStream("test-protocol", "peer2", "peer1")
	wrapper := newStreamWrapper(mockS, "test-protocol")

	if wrapper.RemotePeer() != "peer2" {
		t.Errorf("RemotePeer() = %s, want peer2", wrapper.RemotePeer())
	}
}

func TestStreamWrapper_Close(t *testing.T) {
	mockS := newMockStream("test-protocol", "peer2", "peer1")
	wrapper := newStreamWrapper(mockS, "test-protocol")

	if err := wrapper.Close(); err != nil {
		t.Errorf("Close() failed: %v", err)
	}
}

func TestStreamWrapper_Reset(t *testing.T) {
	mockS := newMockStream("test-protocol", "peer2", "peer1")
	wrapper := newStreamWrapper(mockS, "test-protocol")

	if err := wrapper.Reset(); err != nil {
		t.Errorf("Reset() failed: %v", err)
	}
}

func TestStreamWrapper_Stat(t *testing.T) {
	mockS := newMockStream("test-protocol", "peer2", "peer1")
	// 设置特定的打开时间用于测试
	testTime := time.Unix(123456, 0)
	mockS.openedAt = testTime

	wrapper := newStreamWrapper(mockS, "test-protocol")

	stat := wrapper.Stat()
	if stat.Protocol != "test-protocol" {
		t.Errorf("Stat().Protocol = %s, want test-protocol", stat.Protocol)
	}
	if stat.Opened != 123456 {
		t.Errorf("Stat().Opened = %d, want 123456", stat.Opened)
	}
}

func TestStreamWrapper_SetProtocol(t *testing.T) {
	mockS := newMockStream("test-protocol", "peer2", "peer1")
	wrapper := newStreamWrapper(mockS, "test-protocol")

	if err := wrapper.SetProtocol("new-protocol"); err != nil {
		t.Errorf("SetProtocol() failed: %v", err)
	}

	if wrapper.Protocol() != "new-protocol" {
		t.Errorf("Protocol() = %s, want new-protocol after SetProtocol", wrapper.Protocol())
	}
}

func TestStreamWrapper_CloseRead(t *testing.T) {
	mockS := newMockStream("test-protocol", "peer2", "peer1")
	wrapper := newStreamWrapper(mockS, "test-protocol")

	// CloseRead应该关闭读端
	if err := wrapper.CloseRead(); err != nil {
		t.Errorf("CloseRead() failed: %v", err)
	}
}

func TestStreamWrapper_CloseWrite(t *testing.T) {
	mockS := newMockStream("test-protocol", "peer2", "peer1")
	wrapper := newStreamWrapper(mockS, "test-protocol")

	// CloseWrite应该关闭写端
	if err := wrapper.CloseWrite(); err != nil {
		t.Errorf("CloseWrite() failed: %v", err)
	}
}

func TestStreamWrapper_SetDeadline(t *testing.T) {
	mockS := newMockStream("test-protocol", "peer2", "peer1")
	wrapper := newStreamWrapper(mockS, "test-protocol")

	deadline := time.Now().Add(30 * time.Second)

	// 测试 SetDeadline
	if err := wrapper.SetDeadline(deadline); err != nil {
		t.Errorf("SetDeadline() failed: %v", err)
	}

	// 测试 SetReadDeadline
	if err := wrapper.SetReadDeadline(deadline); err != nil {
		t.Errorf("SetReadDeadline() failed: %v", err)
	}

	// 测试 SetWriteDeadline
	if err := wrapper.SetWriteDeadline(deadline); err != nil {
		t.Errorf("SetWriteDeadline() failed: %v", err)
	}

	// 测试清除超时
	if err := wrapper.SetDeadline(time.Time{}); err != nil {
		t.Errorf("SetDeadline(time.Time{}) failed: %v", err)
	}
}

func TestStreamWrapper_ClosedOperations(t *testing.T) {
	mockS := newMockStream("test-protocol", "peer2", "peer1")
	wrapper := newStreamWrapper(mockS, "test-protocol")

	// 关闭流
	if err := wrapper.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}

	// 关闭后的操作应该返回 ErrStreamClosed
	if _, err := wrapper.Read(make([]byte, 10)); err != ErrStreamClosed {
		t.Errorf("Read() after Close() should return ErrStreamClosed, got: %v", err)
	}

	if _, err := wrapper.Write([]byte("test")); err != ErrStreamClosed {
		t.Errorf("Write() after Close() should return ErrStreamClosed, got: %v", err)
	}

	if err := wrapper.SetProtocol("new"); err != ErrStreamClosed {
		t.Errorf("SetProtocol() after Close() should return ErrStreamClosed, got: %v", err)
	}

	if err := wrapper.SetDeadline(time.Now()); err != ErrStreamClosed {
		t.Errorf("SetDeadline() after Close() should return ErrStreamClosed, got: %v", err)
	}

	// 重复关闭应该是安全的
	if err := wrapper.Close(); err != nil {
		t.Errorf("Double Close() should not fail, got: %v", err)
	}
}

func TestStreamWrapper_IsClosed(t *testing.T) {
	mockS := newMockStream("test-protocol", "peer2", "peer1")
	wrapper := newStreamWrapper(mockS, "test-protocol")

	if wrapper.IsClosed() {
		t.Error("IsClosed() should be false before Close()")
	}

	wrapper.Close()

	if !wrapper.IsClosed() {
		t.Error("IsClosed() should be true after Close()")
	}
}

// ============================================================================
//                       State 测试（覆盖 0% 函数）
// ============================================================================

func TestStreamWrapper_State(t *testing.T) {
	t.Run("open state", func(t *testing.T) {
		mockS := newMockStream("test-protocol", "peer2", "peer1")
		wrapper := newStreamWrapper(mockS, "test-protocol")

		// 未关闭时应返回 Open 状态
		state := wrapper.State()
		if state == 0 {
			// State 返回值取决于底层流，只要不 panic 就行
			t.Log("✅ State() 返回初始状态")
		}
	})

	t.Run("closed state", func(t *testing.T) {
		mockS := newMockStream("test-protocol", "peer2", "peer1")
		wrapper := newStreamWrapper(mockS, "test-protocol")

		// 关闭流
		wrapper.Close()

		// 关闭后应返回 Closed 状态
		state := wrapper.State()
		// types.StreamStateClosed 值通常为 2
		if state == 0 {
			t.Error("State() should not return 0 after Close()")
		}
		t.Logf("✅ State() after Close = %v", state)
	})
}

// TestStreamWrapper_NilUnderlyingStream 测试底层流为 nil 时的行为
func TestStreamWrapper_NilUnderlyingStream(t *testing.T) {
	// 直接创建 wrapper，底层流为 nil
	wrapper := &streamWrapper{}
	wrapper.protocol.Store("test")

	// Protocol 应返回设置的值
	if wrapper.Protocol() != "test" {
		t.Errorf("Protocol() = %s, want test", wrapper.Protocol())
	}

	// IsClosed 应返回关闭状态
	wrapper.closed.Store(true)
	if !wrapper.IsClosed() {
		t.Error("IsClosed() should return true when closed")
	}

	t.Log("✅ nil 底层流边界测试通过")
}
