// Package messaging 编解码测试
package messaging

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	messagingif "github.com/dep2p/go-dep2p/pkg/interfaces/messaging"
	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Request 编解码测试
// ============================================================================

func TestWriteReadRequest(t *testing.T) {
	t.Run("正常请求编解码", func(t *testing.T) {
		buf := &bytes.Buffer{}

		req := &types.Request{
			ID:       12345,
			Protocol: types.ProtocolID("/test/protocol/1.0.0"),
			Data:     []byte("request data"),
		}

		err := writeRequest(buf, req)
		require.NoError(t, err)

		// 解码
		decoded, err := readRequest(buf)
		require.NoError(t, err)

		assert.Equal(t, req.ID, decoded.ID)
		assert.Equal(t, req.Protocol, decoded.Protocol)
		assert.Equal(t, req.Data, decoded.Data)
	})

	t.Run("空数据请求", func(t *testing.T) {
		buf := &bytes.Buffer{}

		req := &types.Request{
			ID:       1,
			Protocol: types.ProtocolID("/empty"),
			Data:     nil,
		}

		err := writeRequest(buf, req)
		require.NoError(t, err)

		decoded, err := readRequest(buf)
		require.NoError(t, err)

		assert.Equal(t, req.ID, decoded.ID)
		assert.Nil(t, decoded.Data)
	})

	t.Run("大数据请求", func(t *testing.T) {
		buf := &bytes.Buffer{}

		largeData := make([]byte, 1024*1024) // 1MB
		for i := range largeData {
			largeData[i] = byte(i % 256)
		}

		req := &types.Request{
			ID:       999999,
			Protocol: types.ProtocolID("/large/data"),
			Data:     largeData,
		}

		err := writeRequest(buf, req)
		require.NoError(t, err)

		decoded, err := readRequest(buf)
		require.NoError(t, err)

		assert.Equal(t, req.ID, decoded.ID)
		assert.Equal(t, len(largeData), len(decoded.Data))
		assert.Equal(t, largeData, decoded.Data)
	})

	t.Run("读取不完整数据失败", func(t *testing.T) {
		// 只写入部分数据
		buf := &bytes.Buffer{}
		buf.Write([]byte{0, 0, 0, 0, 0, 0, 0, 1}) // 只写入 ID

		_, err := readRequest(buf)
		assert.Error(t, err)
	})
}

// ============================================================================
//                              Response 编解码测试
// ============================================================================

func TestWriteReadResponse(t *testing.T) {
	t.Run("成功响应编解码", func(t *testing.T) {
		buf := &bytes.Buffer{}

		resp := &types.Response{
			Status: messagingif.StatusOK,
			Data:   []byte("response data"),
			Error:  "",
		}

		err := writeResponse(buf, resp)
		require.NoError(t, err)

		decoded, err := readResponse(buf)
		require.NoError(t, err)

		assert.Equal(t, resp.Status, decoded.Status)
		assert.Equal(t, resp.Data, decoded.Data)
		assert.Equal(t, resp.Error, decoded.Error)
	})

	t.Run("错误响应编解码", func(t *testing.T) {
		buf := &bytes.Buffer{}

		resp := &types.Response{
			Status: messagingif.StatusInternalError,
			Data:   nil,
			Error:  "something went wrong",
		}

		err := writeResponse(buf, resp)
		require.NoError(t, err)

		decoded, err := readResponse(buf)
		require.NoError(t, err)

		assert.Equal(t, messagingif.StatusInternalError, decoded.Status)
		assert.Nil(t, decoded.Data)
		assert.Equal(t, "something went wrong", decoded.Error)
	})

	t.Run("未找到响应编解码", func(t *testing.T) {
		buf := &bytes.Buffer{}

		resp := &types.Response{
			Status: messagingif.StatusNotFound,
			Error:  "handler not found",
		}

		err := writeResponse(buf, resp)
		require.NoError(t, err)

		decoded, err := readResponse(buf)
		require.NoError(t, err)

		assert.Equal(t, messagingif.StatusNotFound, decoded.Status)
	})

	t.Run("读取空缓冲区失败", func(t *testing.T) {
		buf := &bytes.Buffer{}

		_, err := readResponse(buf)
		assert.Error(t, err)
	})
}

// ============================================================================
//                              Message 编解码测试
// ============================================================================

func TestWriteReadMessage(t *testing.T) {
	t.Run("正常消息编解码", func(t *testing.T) {
		buf := &bytes.Buffer{}

		msg := &types.Message{
			ID:    []byte{1, 2, 3, 4, 5, 6, 7, 8},
			Topic: "test-topic",
			From:  types.NodeID{9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38},
			Data:  []byte("message content"),
		}

		err := writeMessage(buf, msg)
		require.NoError(t, err)

		decoded, err := readMessage(buf)
		require.NoError(t, err)

		assert.Equal(t, msg.ID, decoded.ID)
		assert.Equal(t, msg.Topic, decoded.Topic)
		assert.Equal(t, msg.From, decoded.From)
		assert.Equal(t, msg.Data, decoded.Data)
	})

	t.Run("空主题消息", func(t *testing.T) {
		buf := &bytes.Buffer{}

		msg := &types.Message{
			ID:    []byte{1},
			Topic: "",
			Data:  []byte("data"),
		}

		err := writeMessage(buf, msg)
		require.NoError(t, err)

		decoded, err := readMessage(buf)
		require.NoError(t, err)

		assert.Equal(t, "", decoded.Topic)
	})

	t.Run("空数据消息", func(t *testing.T) {
		buf := &bytes.Buffer{}

		msg := &types.Message{
			ID:    []byte{1, 2},
			Topic: "topic",
			Data:  nil,
		}

		err := writeMessage(buf, msg)
		require.NoError(t, err)

		decoded, err := readMessage(buf)
		require.NoError(t, err)

		assert.Nil(t, decoded.Data)
	})
}

// ============================================================================
//                              基础编解码函数测试
// ============================================================================

func TestWriteReadBytes(t *testing.T) {
	t.Run("正常数据", func(t *testing.T) {
		buf := &bytes.Buffer{}

		data := []byte("hello world")
		err := writeBytes(buf, data)
		require.NoError(t, err)

		decoded, err := readBytes(buf)
		require.NoError(t, err)

		assert.Equal(t, data, decoded)
	})

	t.Run("空数据", func(t *testing.T) {
		buf := &bytes.Buffer{}

		err := writeBytes(buf, nil)
		require.NoError(t, err)

		decoded, err := readBytes(buf)
		require.NoError(t, err)

		assert.Nil(t, decoded)
	})

	t.Run("零长度切片", func(t *testing.T) {
		buf := &bytes.Buffer{}

		err := writeBytes(buf, []byte{})
		require.NoError(t, err)

		decoded, err := readBytes(buf)
		require.NoError(t, err)

		assert.Nil(t, decoded)
	})

	t.Run("读取不完整长度失败", func(t *testing.T) {
		buf := &bytes.Buffer{}
		buf.Write([]byte{0, 0}) // 只写入 2 字节，但需要 4 字节长度

		_, err := readBytes(buf)
		assert.Error(t, err)
	})

	t.Run("读取不完整数据失败", func(t *testing.T) {
		buf := &bytes.Buffer{}
		buf.Write([]byte{0, 0, 0, 10}) // 声明长度 10
		buf.Write([]byte{1, 2, 3})     // 但只有 3 字节数据

		_, err := readBytes(buf)
		assert.Error(t, err)
	})
}

func TestWriteReadString(t *testing.T) {
	t.Run("正常字符串", func(t *testing.T) {
		buf := &bytes.Buffer{}

		str := "hello world"
		err := writeString(buf, str)
		require.NoError(t, err)

		decoded, err := readString(buf)
		require.NoError(t, err)

		assert.Equal(t, str, decoded)
	})

	t.Run("空字符串", func(t *testing.T) {
		buf := &bytes.Buffer{}

		err := writeString(buf, "")
		require.NoError(t, err)

		decoded, err := readString(buf)
		require.NoError(t, err)

		assert.Equal(t, "", decoded)
	})

	t.Run("Unicode 字符串", func(t *testing.T) {
		buf := &bytes.Buffer{}

		str := "你好世界 🌍"
		err := writeString(buf, str)
		require.NoError(t, err)

		decoded, err := readString(buf)
		require.NoError(t, err)

		assert.Equal(t, str, decoded)
	})

	t.Run("长字符串", func(t *testing.T) {
		buf := &bytes.Buffer{}

		str := string(make([]byte, 10000))
		err := writeString(buf, str)
		require.NoError(t, err)

		decoded, err := readString(buf)
		require.NoError(t, err)

		assert.Equal(t, len(str), len(decoded))
	})
}

func TestWriteReadUint64(t *testing.T) {
	t.Run("正常值", func(t *testing.T) {
		buf := &bytes.Buffer{}

		val := uint64(1234567890)
		err := writeUint64(buf, val)
		require.NoError(t, err)

		decoded, err := readUint64(buf)
		require.NoError(t, err)

		assert.Equal(t, val, decoded)
	})

	t.Run("零值", func(t *testing.T) {
		buf := &bytes.Buffer{}

		err := writeUint64(buf, 0)
		require.NoError(t, err)

		decoded, err := readUint64(buf)
		require.NoError(t, err)

		assert.Equal(t, uint64(0), decoded)
	})

	t.Run("最大值", func(t *testing.T) {
		buf := &bytes.Buffer{}

		val := uint64(^uint64(0))
		err := writeUint64(buf, val)
		require.NoError(t, err)

		decoded, err := readUint64(buf)
		require.NoError(t, err)

		assert.Equal(t, val, decoded)
	})

	t.Run("读取空缓冲区失败", func(t *testing.T) {
		buf := &bytes.Buffer{}

		_, err := readUint64(buf)
		assert.Error(t, err)
	})

	t.Run("读取不完整数据失败", func(t *testing.T) {
		buf := &bytes.Buffer{}
		buf.Write([]byte{1, 2, 3, 4}) // 只有 4 字节，需要 8

		_, err := readUint64(buf)
		assert.Error(t, err)
	})
}

// ============================================================================
//                              辅助函数测试
// ============================================================================

func TestMsgIDToKey_Detailed(t *testing.T) {
	t.Run("固定输入固定输出", func(t *testing.T) {
		id := []byte{0x01, 0x02, 0x03, 0x04}
		expected := "01020304"

		key := msgIDToKey(id)
		assert.Equal(t, expected, key)
	})

	t.Run("空 ID", func(t *testing.T) {
		key := msgIDToKey(nil)
		assert.Equal(t, "", key)
	})

	t.Run("全零 ID", func(t *testing.T) {
		id := make([]byte, 8)
		key := msgIDToKey(id)
		assert.Equal(t, "0000000000000000", key)
	})

	t.Run("全 0xFF ID", func(t *testing.T) {
		id := []byte{0xFF, 0xFF, 0xFF, 0xFF}
		key := msgIDToKey(id)
		assert.Equal(t, "ffffffff", key)
	})
}

// ============================================================================
//                              错误写入测试
// ============================================================================

type errorWriter struct {
	failAfter int
	written   int
}

func (w *errorWriter) Write(p []byte) (n int, err error) {
	if w.written >= w.failAfter {
		return 0, io.ErrShortWrite
	}
	w.written += len(p)
	return len(p), nil
}

func TestWriteErrors(t *testing.T) {
	t.Run("writeBytes 写入失败", func(t *testing.T) {
		w := &errorWriter{failAfter: 0}
		err := writeBytes(w, []byte("test"))
		assert.Error(t, err)
	})

	t.Run("writeString 写入失败", func(t *testing.T) {
		w := &errorWriter{failAfter: 0}
		err := writeString(w, "test")
		assert.Error(t, err)
	})

	t.Run("writeUint64 写入失败", func(t *testing.T) {
		w := &errorWriter{failAfter: 0}
		err := writeUint64(w, 123)
		assert.Error(t, err)
	})
}

// ============================================================================
//                              多消息连续编解码测试
// ============================================================================

func TestMultipleMessagesEncoding(t *testing.T) {
	t.Run("连续写入读取多条消息", func(t *testing.T) {
		buf := &bytes.Buffer{}

		// 写入多条消息
		messages := []*types.Message{
			{ID: []byte{1}, Topic: "topic1", Data: []byte("data1")},
			{ID: []byte{2}, Topic: "topic2", Data: []byte("data2")},
			{ID: []byte{3}, Topic: "topic3", Data: []byte("data3")},
		}

		for _, msg := range messages {
			err := writeMessage(buf, msg)
			require.NoError(t, err)
		}

		// 读取多条消息
		for i, expected := range messages {
			decoded, err := readMessage(buf)
			require.NoError(t, err, "读取消息 %d 失败", i)

			assert.Equal(t, expected.ID, decoded.ID)
			assert.Equal(t, expected.Topic, decoded.Topic)
			assert.Equal(t, expected.Data, decoded.Data)
		}
	})

	t.Run("连续写入读取多个请求响应", func(t *testing.T) {
		buf := &bytes.Buffer{}

		// 写入请求
		req := &types.Request{ID: 1, Protocol: "/test", Data: []byte("req")}
		err := writeRequest(buf, req)
		require.NoError(t, err)

		// 写入响应
		resp := &types.Response{Status: messagingif.StatusOK, Data: []byte("resp")}
		err = writeResponse(buf, resp)
		require.NoError(t, err)

		// 读取请求
		decodedReq, err := readRequest(buf)
		require.NoError(t, err)
		assert.Equal(t, req.ID, decodedReq.ID)

		// 读取响应
		decodedResp, err := readResponse(buf)
		require.NoError(t, err)
		assert.Equal(t, resp.Status, decodedResp.Status)
	})
}

// ============================================================================
//                              安全性测试
// ============================================================================

func TestReadBytes_MaxLength(t *testing.T) {
	t.Run("超过最大长度应拒绝", func(t *testing.T) {
		buf := &bytes.Buffer{}
		// 写入一个声称长度超过 MaxMessageLength 的消息头
		// MaxMessageLength = 10 * 1024 * 1024 = 10485760
		// 我们声称长度为 20MB = 20971520
		length := uint32(20 * 1024 * 1024)
		err := writeUint64(buf, uint64(length)<<32) // 写入长度（前 4 字节）
		require.NoError(t, err)

		// 直接构造一个声称超大长度的缓冲区
		buf2 := &bytes.Buffer{}
		buf2.Write([]byte{0x01, 0x40, 0x00, 0x00}) // 20971520 in big endian

		_, err = readBytes(buf2)
		assert.Error(t, err)
		assert.ErrorIs(t, err, ErrMessageTooLarge)
	})

	t.Run("恰好等于最大长度应接受", func(t *testing.T) {
		// 注意：实际创建 10MB 的测试数据可能太慢
		// 这里我们只测试逻辑边界
		buf := &bytes.Buffer{}
		// 写入恰好等于 MaxMessageLength 的长度
		// 但不实际写入数据（会导致 io.ErrUnexpectedEOF）
		buf.Write([]byte{0x00, 0xA0, 0x00, 0x00}) // 10485760 in big endian

		_, err := readBytes(buf)
		// 应该是 EOF 错误（数据不足），不是 ErrMessageTooLarge
		assert.Error(t, err)
		assert.NotErrorIs(t, err, ErrMessageTooLarge)
	})

	t.Run("正常大小消息应接受", func(t *testing.T) {
		buf := &bytes.Buffer{}
		data := make([]byte, 1024) // 1KB
		err := writeBytes(buf, data)
		require.NoError(t, err)

		decoded, err := readBytes(buf)
		require.NoError(t, err)
		assert.Len(t, decoded, 1024)
	})
}
