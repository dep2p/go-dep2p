package messaging

import (
	"bytes"
	"testing"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCodec_EncodeDecodeRequest(t *testing.T) {
	codec := NewCodec()

	req := &interfaces.Request{
		ID:       "test-request-id",
		From:     "peer-123",
		Protocol: "myprotocol",
		Data:     []byte("hello world"),
		Timestamp: time.Now(),
		Metadata: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
	}

	// 编码
	data, err := codec.EncodeRequest(req)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// 解码
	decoded, err := codec.DecodeRequest(data)
	require.NoError(t, err)
	require.NotNil(t, decoded)

	// 验证
	assert.Equal(t, req.ID, decoded.ID)
	assert.Equal(t, req.From, decoded.From)
	assert.Equal(t, req.Data, decoded.Data)
	assert.Equal(t, req.Metadata["key1"], decoded.Metadata["key1"])
	assert.Equal(t, req.Metadata["key2"], decoded.Metadata["key2"])
}

func TestCodec_EncodeDecodeResponse(t *testing.T) {
	codec := NewCodec()

	resp := &interfaces.Response{
		ID:       "test-response-id",
		From:     "peer-456",
		Data:     []byte("response data"),
		Timestamp: time.Now(),
		Latency:  100 * time.Millisecond,
		Metadata: map[string]string{
			"status": "ok",
		},
	}

	// 编码
	data, err := codec.EncodeResponse(resp)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	// 解码
	decoded, err := codec.DecodeResponse(data)
	require.NoError(t, err)
	require.NotNil(t, decoded)

	// 验证
	assert.Equal(t, resp.ID, decoded.ID)
	assert.Equal(t, resp.From, decoded.From)
	assert.Equal(t, resp.Data, decoded.Data)
	assert.Equal(t, resp.Metadata["status"], decoded.Metadata["status"])
	assert.InDelta(t, resp.Latency.Nanoseconds(), decoded.Latency.Nanoseconds(), float64(time.Millisecond))
}

func TestCodec_EncodeDecodeResponse_WithError(t *testing.T) {
	codec := NewCodec()

	resp := &interfaces.Response{
		ID:    "test-error-response",
		From:  "peer-789",
		Error: ErrTimeout,
		Timestamp: time.Now(),
	}

	// 编码
	data, err := codec.EncodeResponse(resp)
	require.NoError(t, err)

	// 解码
	decoded, err := codec.DecodeResponse(data)
	require.NoError(t, err)
	require.NotNil(t, decoded)

	// 验证错误信息
	assert.NotNil(t, decoded.Error)
	assert.Contains(t, decoded.Error.Error(), "timeout")
}

func TestCodec_WriteReadRequest(t *testing.T) {
	codec := NewCodec()

	req := &interfaces.Request{
		ID:       "stream-test-request",
		From:     "peer-111",
		Data:     []byte("test data"),
		Timestamp: time.Now(),
	}

	// 使用 buffer 模拟流
	buf := &bytes.Buffer{}

	// 写入
	err := codec.WriteRequest(buf, req)
	require.NoError(t, err)

	// 读取
	decoded, err := codec.ReadRequest(buf)
	require.NoError(t, err)
	require.NotNil(t, decoded)

	// 验证
	assert.Equal(t, req.ID, decoded.ID)
	assert.Equal(t, req.From, decoded.From)
	assert.Equal(t, req.Data, decoded.Data)
}

func TestCodec_WriteReadResponse(t *testing.T) {
	codec := NewCodec()

	resp := &interfaces.Response{
		ID:   "stream-test-response",
		From: "peer-222",
		Data: []byte("response"),
		Timestamp: time.Now(),
	}

	// 使用 buffer 模拟流
	buf := &bytes.Buffer{}

	// 写入
	err := codec.WriteResponse(buf, resp)
	require.NoError(t, err)

	// 读取
	decoded, err := codec.ReadResponse(buf)
	require.NoError(t, err)
	require.NotNil(t, decoded)

	// 验证
	assert.Equal(t, resp.ID, decoded.ID)
	assert.Equal(t, resp.From, decoded.From)
	assert.Equal(t, resp.Data, decoded.Data)
}

func TestCodec_EncodeRequest_Nil(t *testing.T) {
	codec := NewCodec()

	data, err := codec.EncodeRequest(nil)
	assert.Error(t, err)
	assert.Nil(t, data)
	assert.ErrorIs(t, err, ErrInvalidMessage)
}

func TestCodec_DecodeRequest_Empty(t *testing.T) {
	codec := NewCodec()

	decoded, err := codec.DecodeRequest([]byte{})
	assert.Error(t, err)
	assert.Nil(t, decoded)
	assert.ErrorIs(t, err, ErrInvalidMessage)
}

func TestCodec_EncodeResponse_Nil(t *testing.T) {
	codec := NewCodec()

	data, err := codec.EncodeResponse(nil)
	assert.Error(t, err)
	assert.Nil(t, data)
	assert.ErrorIs(t, err, ErrInvalidMessage)
}

func TestCodec_DecodeResponse_Empty(t *testing.T) {
	codec := NewCodec()

	decoded, err := codec.DecodeResponse([]byte{})
	assert.Error(t, err)
	assert.Nil(t, decoded)
	assert.ErrorIs(t, err, ErrInvalidMessage)
}

func TestCodec_LargePayload(t *testing.T) {
	codec := NewCodec()

	// 创建大payload (1MB)
	largeData := make([]byte, 1024*1024)
	for i := range largeData {
		largeData[i] = byte(i % 256)
	}

	req := &interfaces.Request{
		ID:   "large-payload-test",
		From: "peer-large",
		Data: largeData,
		Timestamp: time.Now(),
	}

	// 编码
	encoded, err := codec.EncodeRequest(req)
	require.NoError(t, err)

	// 解码
	decoded, err := codec.DecodeRequest(encoded)
	require.NoError(t, err)
	require.NotNil(t, decoded)

	// 验证
	assert.Equal(t, req.ID, decoded.ID)
	assert.Equal(t, len(req.Data), len(decoded.Data))
	assert.Equal(t, req.Data, decoded.Data)
}
