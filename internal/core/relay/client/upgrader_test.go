// Package client 提供中继客户端实现
package client

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// mockHolePuncher 模拟打洞器
type mockHolePuncher struct {
	punchResult string
	punchErr    error
	punchCount  int32
}

func (m *mockHolePuncher) Punch(_ context.Context, _ types.NodeID, _ []string) (string, error) {
	atomic.AddInt32(&m.punchCount, 1)
	return m.punchResult, m.punchErr
}

func TestNewConnectionUpgrader(t *testing.T) {
	config := DefaultUpgraderConfig()
	puncher := &mockHolePuncher{}

	upgrader := NewConnectionUpgrader(config, puncher, nil)

	assert.NotNil(t, upgrader)
	assert.NotNil(t, upgrader.sessions)
}

func TestConnectionUpgrader_StartStop(t *testing.T) {
	config := DefaultUpgraderConfig()
	upgrader := NewConnectionUpgrader(config, nil, nil)

	ctx := context.Background()
	err := upgrader.Start(ctx)
	require.NoError(t, err)
	assert.Equal(t, int32(1), atomic.LoadInt32(&upgrader.running))

	err = upgrader.Stop()
	require.NoError(t, err)
	assert.Equal(t, int32(0), atomic.LoadInt32(&upgrader.running))
}

func TestConnectionUpgrader_DoubleStart(t *testing.T) {
	config := DefaultUpgraderConfig()
	upgrader := NewConnectionUpgrader(config, nil, nil)

	ctx := context.Background()
	err := upgrader.Start(ctx)
	require.NoError(t, err)

	// 第二次启动应该无效果
	err = upgrader.Start(ctx)
	require.NoError(t, err)

	upgrader.Stop()
}

func TestConnectionUpgrader_OnUpgraded(t *testing.T) {
	config := DefaultUpgraderConfig()
	upgrader := NewConnectionUpgrader(config, nil, nil)

	var callbackCalled bool
	var callbackNodeID types.NodeID
	var callbackAddr string

	upgrader.OnUpgraded(func(nodeID types.NodeID, addr string) {
		callbackCalled = true
		callbackNodeID = nodeID
		callbackAddr = addr
	})

	// 模拟调用
	testNodeID := types.NodeID("12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN")
	upgrader.onUpgraded(testNodeID, "/ip4/1.2.3.4/udp/4001/quic-v1")

	assert.True(t, callbackCalled)
	assert.Equal(t, testNodeID, callbackNodeID)
	assert.Equal(t, "/ip4/1.2.3.4/udp/4001/quic-v1", callbackAddr)
}

func TestConnectionUpgrader_SetPuncher(t *testing.T) {
	config := DefaultUpgraderConfig()
	upgrader := NewConnectionUpgrader(config, nil, nil)

	assert.Nil(t, upgrader.puncher)

	puncher := &mockHolePuncher{}
	upgrader.SetPuncher(puncher)

	assert.NotNil(t, upgrader.puncher)
}

func TestDefaultUpgraderConfig(t *testing.T) {
	config := DefaultUpgraderConfig()

	assert.Equal(t, 10*time.Second, config.HolePunchTimeout)
	assert.Equal(t, 5*time.Second, config.AddrExchangeTimeout)
	assert.Equal(t, 5*time.Minute, config.RetryInterval)
	assert.Equal(t, 3, config.MaxRetries)
	assert.True(t, config.EnableAutoUpgrade)
}

func TestUpgradeSession_States(t *testing.T) {
	session := &upgradeSession{
		state: UpgradeStatePending,
		done:  make(chan struct{}),
	}

	// 测试状态转换
	assert.Equal(t, UpgradeStatePending, atomic.LoadInt32(&session.state))

	atomic.StoreInt32(&session.state, UpgradeStateExchanged)
	assert.Equal(t, UpgradeStateExchanged, atomic.LoadInt32(&session.state))

	atomic.StoreInt32(&session.state, UpgradeStatePunching)
	assert.Equal(t, UpgradeStatePunching, atomic.LoadInt32(&session.state))

	atomic.StoreInt32(&session.state, UpgradeStateSuccess)
	assert.Equal(t, UpgradeStateSuccess, atomic.LoadInt32(&session.state))
}

func TestConnectionUpgrader_HolePunch_NoPuncher(t *testing.T) {
	config := DefaultUpgraderConfig()
	upgrader := NewConnectionUpgrader(config, nil, nil)

	session := &upgradeSession{
		remoteAddrs: []string{"/ip4/1.2.3.4/udp/4001/quic-v1"},
	}

	ctx := context.Background()
	_, err := upgrader.holePunch(ctx, session)

	assert.ErrorIs(t, err, ErrNoPuncher)
}

func TestConnectionUpgrader_HolePunch_NoAddresses(t *testing.T) {
	config := DefaultUpgraderConfig()
	puncher := &mockHolePuncher{}
	upgrader := NewConnectionUpgrader(config, puncher, nil)

	session := &upgradeSession{
		remoteAddrs: []string{},
	}

	ctx := context.Background()
	_, err := upgrader.holePunch(ctx, session)

	assert.ErrorIs(t, err, ErrNoAddresses)
}

func TestConnectionUpgrader_HolePunch_Success(t *testing.T) {
	config := DefaultUpgraderConfig()
	puncher := &mockHolePuncher{
		punchResult: "/ip4/1.2.3.4/udp/4001/quic-v1",
	}
	upgrader := NewConnectionUpgrader(config, puncher, nil)

	session := &upgradeSession{
		remoteAddrs: []string{"/ip4/1.2.3.4/udp/4001/quic-v1"},
	}

	ctx := context.Background()
	addr, err := upgrader.holePunch(ctx, session)

	require.NoError(t, err)
	assert.Equal(t, "/ip4/1.2.3.4/udp/4001/quic-v1", addr)
	assert.Equal(t, UpgradeStateSuccess, atomic.LoadInt32(&session.state))
	assert.Equal(t, int32(1), atomic.LoadInt32(&puncher.punchCount))
}

func TestConnectionUpgrader_ExchangeAddresses_NoLocalAddrs(t *testing.T) {
	config := DefaultUpgraderConfig()
	upgrader := NewConnectionUpgrader(config, nil, func() []string {
		return []string{}
	})

	session := &upgradeSession{}

	ctx := context.Background()
	err := upgrader.exchangeAddresses(ctx, session)

	assert.ErrorIs(t, err, ErrNoAddresses)
}

func TestConnectionUpgrader_SendReceiveAddresses(t *testing.T) {
	// 使用 pipe 测试地址发送和接收
	r, w := testPipe()

	config := DefaultUpgraderConfig()
	upgrader := NewConnectionUpgrader(config, nil, nil)

	addrs := []string{
		"/ip4/1.1.1.1/udp/4001/quic-v1",
		"/ip4/2.2.2.2/udp/4002/quic-v1",
	}

	// 并发发送和接收
	errCh := make(chan error, 1)
	go func() {
		errCh <- upgrader.sendAddresses(w, addrs)
		w.Close()
	}()

	received, err := upgrader.receiveAddresses(r)
	require.NoError(t, err)
	assert.Len(t, received, 2)
	assert.Equal(t, addrs[0], received[0])
	assert.Equal(t, addrs[1], received[1])

	require.NoError(t, <-errCh)
}

// testPipe 创建测试用的读写管道
func testPipe() (*mockStream, *mockStream) {
	ch := make(chan []byte, 100)
	return &mockStream{ch: ch, reader: true}, &mockStream{ch: ch, reader: false}
}

// mockStream 实现 interfaces.Stream 接口
type mockStream struct {
	ch     chan []byte
	buf    []byte
	reader bool
	closed bool
}

func (m *mockStream) Read(b []byte) (int, error) {
	if len(m.buf) > 0 {
		n := copy(b, m.buf)
		m.buf = m.buf[n:]
		return n, nil
	}
	data, ok := <-m.ch
	if !ok {
		return 0, io.EOF
	}
	n := copy(b, data)
	if n < len(data) {
		m.buf = data[n:]
	}
	return n, nil
}

func (m *mockStream) Write(b []byte) (int, error) {
	data := make([]byte, len(b))
	copy(data, b)
	m.ch <- data
	return len(b), nil
}

func (m *mockStream) Close() error {
	if !m.closed && !m.reader {
		close(m.ch)
		m.closed = true
	}
	return nil
}

func (m *mockStream) CloseWrite() error {
	return nil
}

func (m *mockStream) CloseRead() error {
	return nil
}

func (m *mockStream) Reset() error {
	return nil
}

func (m *mockStream) SetDeadline(_ time.Time) error {
	return nil
}

func (m *mockStream) SetReadDeadline(_ time.Time) error {
	return nil
}

func (m *mockStream) SetWriteDeadline(_ time.Time) error {
	return nil
}
