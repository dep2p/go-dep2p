// Package reachability 提供可达性协调模块的实现
package reachability

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

func TestNewDialBackService(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	service := NewDialBackService(config)

	assert.NotNil(t, service)
	assert.NotNil(t, service.config)
	assert.NotNil(t, service.results)
}

func TestDialBackService_StartStop(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	service := NewDialBackService(config)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err)
	assert.True(t, service.running)

	err = service.Stop()
	require.NoError(t, err)
	assert.False(t, service.running)
}

func TestDialBackService_VerifyAddresses_Empty(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	service := NewDialBackService(config)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err, "service should start successfully")
	defer service.Stop()

	// 空地址列表
	reachable, err := service.VerifyAddresses(ctx, "", nil)
	assert.NoError(t, err)
	assert.Nil(t, reachable)
}

func TestDialBackService_VerifyAddresses_MockVerify(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	service := NewDialBackService(config)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err, "service should start successfully")
	defer service.Stop()

	// 测试模拟验证（无 stream opener）
	candidates := []string{
		"/ip4/1.1.1.1/udp/4001/quic-v1",
		"/ip4/2.2.2.2/udp/4001/quic-v1",
	}

	reachable, err := service.VerifyAddresses(ctx, "", candidates)
	assert.NoError(t, err)
	assert.Equal(t, candidates, reachable)
}

func TestDialBackService_VerifyAddresses_MaxAddrs(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	service := NewDialBackService(config)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err, "service should start successfully")
	defer service.Stop()

	// 超过最大地址数
	candidates := make([]string, 20)
	for i := range candidates {
		candidates[i] = "/ip4/1.2.3.4/udp/4001/quic-v1"
	}

	reachable, err := service.VerifyAddresses(ctx, "", candidates)
	assert.NoError(t, err)
	assert.Equal(t, interfaces.MaxAddrsPerRequest, len(reachable))
}

func TestDialBackService_VerifyAddresses_ServiceStopped(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	service := NewDialBackService(config)

	// 不启动服务
	ctx := context.Background()
	_, err := service.VerifyAddresses(ctx, "", []string{"/ip4/1.1.1.1/udp/4001/quic-v1"})
	assert.ErrorIs(t, err, ErrServiceStopped)
}

func TestDialBackService_HandleDialBackRequest(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	service := NewDialBackService(config)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err, "service should start successfully")
	defer service.Stop()

	req := &interfaces.DialBackRequest{
		Addrs: []string{
			"/ip4/1.1.1.1/udp/4001/quic-v1",
			"/ip4/2.2.2.2/udp/4001/quic-v1",
		},
		Nonce:     []byte("test-nonce"),
		TimeoutMs: 5000,
	}

	resp := service.HandleDialBackRequest(ctx, req)

	assert.NotNil(t, resp)
	assert.Equal(t, req.Nonce, resp.Nonce)
	assert.Len(t, resp.DialResults, 2)
	assert.Len(t, resp.Reachable, 2)
}

func TestDialBackService_HandleDialBackRequest_Empty(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	service := NewDialBackService(config)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err, "service should start successfully")
	defer service.Stop()

	req := &interfaces.DialBackRequest{
		Addrs: []string{},
		Nonce: []byte("test-nonce"),
	}

	resp := service.HandleDialBackRequest(ctx, req)

	assert.NotNil(t, resp)
	assert.Equal(t, req.Nonce, resp.Nonce)
	assert.Len(t, resp.DialResults, 0)
}

func TestDialBackService_HandleDialBackRequest_MaxAddrs(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	service := NewDialBackService(config)

	ctx := context.Background()
	err := service.Start(ctx)
	require.NoError(t, err, "service should start successfully")
	defer service.Stop()

	// 超过最大地址数
	addrs := make([]string, 20)
	for i := range addrs {
		addrs[i] = "/ip4/1.2.3.4/udp/4001/quic-v1"
	}

	req := &interfaces.DialBackRequest{
		Addrs: addrs,
		Nonce: []byte("test-nonce"),
	}

	resp := service.HandleDialBackRequest(ctx, req)

	assert.NotNil(t, resp)
	assert.Equal(t, interfaces.MaxAddrsPerRequest, len(resp.DialResults))
}

func TestDialBackService_GetVerificationResult(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	service := NewDialBackService(config)

	ctx := context.Background()
	_ = service.Start(ctx)
	defer service.Stop()

	// 验证一些地址
	candidates := []string{"/ip4/1.1.1.1/udp/4001/quic-v1"}
	_, _ = service.VerifyAddresses(ctx, "", candidates)

	// 获取验证结果
	result, ok := service.GetVerificationResult(candidates[0])
	assert.True(t, ok)
	assert.NotNil(t, result)
	assert.True(t, result.Reachable)
}

func TestDialBackService_ClearResults(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	service := NewDialBackService(config)

	ctx := context.Background()
	_ = service.Start(ctx)
	defer service.Stop()

	// 验证一些地址
	candidates := []string{"/ip4/1.1.1.1/udp/4001/quic-v1"}
	_, _ = service.VerifyAddresses(ctx, "", candidates)

	// 清除结果
	service.ClearResults()

	// 验证结果已清除
	_, ok := service.GetVerificationResult(candidates[0])
	assert.False(t, ok)
}

func TestReadWriteFrame(t *testing.T) {
	// 使用内存缓冲区测试
	tests := []struct {
		name    string
		payload []byte
		wantErr bool
	}{
		{
			name:    "normal payload",
			payload: []byte("test data"),
			wantErr: false,
		},
		{
			name:    "larger payload",
			payload: make([]byte, 1000),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 使用 pipe 模拟读写
			r, w := pipeReaderWriter()

			// 写入
			go func() {
				err := writeFrame(w, tt.payload)
				assert.NoError(t, err)
				w.Close()
			}()

			// 读取
			data, err := readFrame(r)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, len(tt.payload), len(data))
			}
		})
	}
}

func TestWriteFrame_Errors(t *testing.T) {
	// 测试空 payload
	err := writeFrame(nil, []byte{})
	assert.Error(t, err)

	// 测试超大 payload
	err = writeFrame(nil, make([]byte, maxFrameSize+1))
	assert.Error(t, err)
}

// pipeReaderWriter 创建一对读写器
func pipeReaderWriter() (*pipeReader, *pipeWriter) {
	ch := make(chan []byte, 10)
	return &pipeReader{ch: ch}, &pipeWriter{ch: ch}
}

type pipeReader struct {
	ch     chan []byte
	buf    []byte
	closed bool
}

func (r *pipeReader) Read(p []byte) (int, error) {
	if len(r.buf) > 0 {
		n := copy(p, r.buf)
		r.buf = r.buf[n:]
		return n, nil
	}
	data, ok := <-r.ch
	if !ok {
		return 0, nil
	}
	n := copy(p, data)
	if n < len(data) {
		r.buf = data[n:]
	}
	return n, nil
}

type pipeWriter struct {
	ch     chan []byte
	closed bool
}

func (w *pipeWriter) Write(p []byte) (int, error) {
	data := make([]byte, len(p))
	copy(data, p)
	w.ch <- data
	return len(p), nil
}

func (w *pipeWriter) Close() error {
	if !w.closed {
		close(w.ch)
		w.closed = true
	}
	return nil
}

// mockStreamOpener 模拟 StreamOpener 实现
type mockStreamOpener struct {
	openCalled bool
	errOnOpen  error
}

func (m *mockStreamOpener) OpenStream(ctx context.Context, peerID string, protocolID string) (interfaces.StreamReadWriteCloser, error) {
	m.openCalled = true
	if m.errOnOpen != nil {
		return nil, m.errOnOpen
	}
	return &mockStream{}, nil
}

// mockStream 模拟流实现
type mockStream struct {
	closed bool
}

func (m *mockStream) Read(p []byte) (int, error)  { return 0, nil }
func (m *mockStream) Write(p []byte) (int, error) { return len(p), nil }
func (m *mockStream) Close() error                { m.closed = true; return nil }

func TestDialBackService_SetStreamOpener(t *testing.T) {
	config := interfaces.DefaultReachabilityConfig()
	service := NewDialBackService(config)

	// 验证初始状态没有 streamOpener
	assert.Nil(t, service.streamOpener)

	// 设置 StreamOpener
	opener := &mockStreamOpener{}
	service.SetStreamOpener(opener)

	// 验证 StreamOpener 已设置
	assert.NotNil(t, service.streamOpener)
	assert.Equal(t, opener, service.streamOpener)
}
