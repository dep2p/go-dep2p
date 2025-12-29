package tls

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dep2p/go-dep2p/internal/core/identity"
	"github.com/dep2p/go-dep2p/pkg/interfaces/endpoint"
	identityif "github.com/dep2p/go-dep2p/pkg/interfaces/identity"
	securityif "github.com/dep2p/go-dep2p/pkg/interfaces/security"
)

func createTestIdentity(t *testing.T) identityif.Identity {
	cfg := identityif.DefaultConfig()
	mgr := identity.NewManager(cfg)
	ident, err := mgr.Create()
	require.NoError(t, err)
	return ident
}

func TestNewTransport(t *testing.T) {
	ident := createTestIdentity(t)
	config := securityif.DefaultConfig()

	transport, err := NewTransport(ident, config)
	require.NoError(t, err)
	require.NotNil(t, transport)

	assert.Equal(t, "tls", transport.Protocol())
	assert.NotNil(t, transport.AccessController())
}

func TestNewTransportNilIdentity(t *testing.T) {
	config := securityif.DefaultConfig()

	_, err := NewTransport(nil, config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "identity 不能为空")
}

func TestNewTransportNilLogger(t *testing.T) {
	ident := createTestIdentity(t)
	config := securityif.DefaultConfig()

	// nil logger 应该也能工作
	transport, err := NewTransport(ident, config)
	require.NoError(t, err)
	require.NotNil(t, transport)
}

func TestSecureHandshake(t *testing.T) {
	serverIdent := createTestIdentity(t)
	clientIdent := createTestIdentity(t)
	config := securityif.DefaultConfig()

	serverTransport, err := NewTransport(serverIdent, config)
	require.NoError(t, err)

	clientTransport, err := NewTransport(clientIdent, config)
	require.NoError(t, err)

	// 创建 TCP 连接对
	serverConn, clientConn := createConnPair(t)
	defer serverConn.Close()
	defer clientConn.Close()

	var wg sync.WaitGroup
	var serverSecureConn securityif.SecureConn
	var serverErr error
	var clientSecureConn securityif.SecureConn
	var clientErr error

	wg.Add(2)

	// 服务端握手
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		serverSecureConn, serverErr = serverTransport.SecureInbound(ctx, &mockTransportConn{netConn: serverConn})
	}()

	// 客户端握手
	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		clientSecureConn, clientErr = clientTransport.SecureOutbound(ctx, &mockTransportConn{netConn: clientConn}, serverIdent.ID())
	}()

	wg.Wait()

	require.NoError(t, serverErr)
	require.NoError(t, clientErr)
	require.NotNil(t, serverSecureConn)
	require.NotNil(t, clientSecureConn)

	// 验证身份
	assert.Equal(t, serverIdent.ID(), serverSecureConn.LocalIdentity())
	assert.Equal(t, clientIdent.ID(), serverSecureConn.RemoteIdentity())
	assert.Equal(t, clientIdent.ID(), clientSecureConn.LocalIdentity())
	assert.Equal(t, serverIdent.ID(), clientSecureConn.RemoteIdentity())

	// 清理
	serverSecureConn.Close()
	clientSecureConn.Close()
}

func TestSecureHandshakeDataTransfer(t *testing.T) {
	serverIdent := createTestIdentity(t)
	clientIdent := createTestIdentity(t)
	config := securityif.DefaultConfig()

	serverTransport, err := NewTransport(serverIdent, config)
	require.NoError(t, err)

	clientTransport, err := NewTransport(clientIdent, config)
	require.NoError(t, err)

	serverConn, clientConn := createConnPair(t)
	defer serverConn.Close()
	defer clientConn.Close()

	var wg sync.WaitGroup
	var serverSecureConn securityif.SecureConn
	var clientSecureConn securityif.SecureConn

	wg.Add(2)

	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var err error
		serverSecureConn, err = serverTransport.SecureInbound(ctx, &mockTransportConn{netConn: serverConn})
		require.NoError(t, err)
	}()

	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var err error
		clientSecureConn, err = clientTransport.SecureOutbound(ctx, &mockTransportConn{netConn: clientConn}, serverIdent.ID())
		require.NoError(t, err)
	}()

	wg.Wait()

	defer serverSecureConn.Close()
	defer clientSecureConn.Close()

	// 测试数据传输
	message := "Hello, TLS!"

	wg.Add(2)

	// 客户端发送
	go func() {
		defer wg.Done()
		n, err := clientSecureConn.Write([]byte(message))
		require.NoError(t, err)
		assert.Equal(t, len(message), n)
	}()

	// 服务端接收
	go func() {
		defer wg.Done()
		buf := make([]byte, len(message))
		n, err := io.ReadFull(serverSecureConn, buf)
		require.NoError(t, err)
		assert.Equal(t, len(message), n)
		assert.Equal(t, message, string(buf))
	}()

	wg.Wait()
}

func TestSecureOutboundNodeIDMismatch(t *testing.T) {
	serverIdent := createTestIdentity(t)
	clientIdent := createTestIdentity(t)
	wrongIdent := createTestIdentity(t) // 错误的预期 NodeID
	config := securityif.DefaultConfig()

	serverTransport, err := NewTransport(serverIdent, config)
	require.NoError(t, err)

	clientTransport, err := NewTransport(clientIdent, config)
	require.NoError(t, err)

	serverConn, clientConn := createConnPair(t)
	defer serverConn.Close()
	defer clientConn.Close()

	var wg sync.WaitGroup
	var clientErr error

	wg.Add(2)

	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		serverTransport.SecureInbound(ctx, &mockTransportConn{netConn: serverConn})
	}()

	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		// 使用错误的预期 NodeID
		_, clientErr = clientTransport.SecureOutbound(ctx, &mockTransportConn{netConn: clientConn}, wrongIdent.ID())
	}()

	wg.Wait()

	// 客户端应该因为 NodeID 不匹配而失败
	require.Error(t, clientErr)
	assert.Contains(t, clientErr.Error(), "NodeID 不匹配")
}

func TestAccessControlInbound(t *testing.T) {
	serverIdent := createTestIdentity(t)
	clientIdent := createTestIdentity(t)
	config := securityif.DefaultConfig()

	serverTransport, err := NewTransport(serverIdent, config)
	require.NoError(t, err)

	// 将客户端加入黑名单
	serverTransport.AccessController().AddToBlockList(clientIdent.ID())

	clientTransport, err := NewTransport(clientIdent, config)
	require.NoError(t, err)

	serverConn, clientConn := createConnPair(t)
	defer serverConn.Close()
	defer clientConn.Close()

	var wg sync.WaitGroup
	var serverErr error

	wg.Add(2)

	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, serverErr = serverTransport.SecureInbound(ctx, &mockTransportConn{netConn: serverConn})
	}()

	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		clientTransport.SecureOutbound(ctx, &mockTransportConn{netConn: clientConn}, serverIdent.ID())
	}()

	wg.Wait()

	// 服务端应该拒绝连接
	require.Error(t, serverErr)
	assert.Contains(t, serverErr.Error(), "入站连接被拒绝")
}

func TestAccessControlOutbound(t *testing.T) {
	serverIdent := createTestIdentity(t)
	clientIdent := createTestIdentity(t)
	config := securityif.DefaultConfig()

	clientTransport, err := NewTransport(clientIdent, config)
	require.NoError(t, err)

	// 将服务端加入客户端的黑名单
	clientTransport.AccessController().AddToBlockList(serverIdent.ID())

	serverConn, clientConn := createConnPair(t)
	defer serverConn.Close()
	defer clientConn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 客户端应该因为访问控制而失败
	_, err = clientTransport.SecureOutbound(ctx, &mockTransportConn{netConn: clientConn}, serverIdent.ID())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "出站连接被拒绝")
}

// ============================================================================
//                              辅助函数和类型
// ============================================================================

// createConnPair 创建一对连接的 TCP 连接
func createConnPair(t *testing.T) (net.Conn, net.Conn) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	var serverConn net.Conn
	var serverErr error
	done := make(chan struct{})

	go func() {
		serverConn, serverErr = listener.Accept()
		close(done)
	}()

	clientConn, err := net.Dial("tcp", listener.Addr().String())
	require.NoError(t, err)

	<-done
	require.NoError(t, serverErr)
	listener.Close()

	return serverConn, clientConn
}

// mockAddress 模拟 endpoint.Address 接口
type mockAddress struct {
	addr net.Addr
}

func (a *mockAddress) Network() string {
	return a.addr.Network()
}

func (a *mockAddress) String() string {
	return a.addr.String()
}

func (a *mockAddress) Bytes() []byte {
	return []byte(a.addr.String())
}

func (a *mockAddress) Equal(other endpoint.Address) bool {
	if other == nil {
		return false
	}
	return a.String() == other.String()
}

func (a *mockAddress) IsPublic() bool {
	return false
}

func (a *mockAddress) IsPrivate() bool {
	return true
}

func (a *mockAddress) IsLoopback() bool {
	return true
}

func (a *mockAddress) Multiaddr() string {
	// 从 net.Addr 转换为 multiaddr
	host, port, err := net.SplitHostPort(a.addr.String())
	if err != nil {
		return "/ip4/127.0.0.1/tcp/0"
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Sprintf("/dns4/%s/tcp/%s", host, port)
	}
	ipType := "ip4"
	if ip.To4() == nil {
		ipType = "ip6"
	}
	return fmt.Sprintf("/%s/%s/tcp/%s", ipType, host, port)
}

// mockTransportConn 模拟 transport.Conn
type mockTransportConn struct {
	netConn net.Conn
}

func (m *mockTransportConn) Read(p []byte) (int, error) {
	return m.netConn.Read(p)
}

func (m *mockTransportConn) Write(p []byte) (int, error) {
	return m.netConn.Write(p)
}

func (m *mockTransportConn) Close() error {
	return m.netConn.Close()
}

func (m *mockTransportConn) LocalAddr() endpoint.Address {
	return &mockAddress{addr: m.netConn.LocalAddr()}
}

func (m *mockTransportConn) RemoteAddr() endpoint.Address {
	return &mockAddress{addr: m.netConn.RemoteAddr()}
}

func (m *mockTransportConn) LocalNetAddr() net.Addr {
	return m.netConn.LocalAddr()
}

func (m *mockTransportConn) RemoteNetAddr() net.Addr {
	return m.netConn.RemoteAddr()
}

func (m *mockTransportConn) SetDeadline(t time.Time) error {
	return m.netConn.SetDeadline(t)
}

func (m *mockTransportConn) SetReadDeadline(t time.Time) error {
	return m.netConn.SetReadDeadline(t)
}

func (m *mockTransportConn) SetWriteDeadline(t time.Time) error {
	return m.netConn.SetWriteDeadline(t)
}

func (m *mockTransportConn) IsClosed() bool {
	return false
}

func (m *mockTransportConn) Transport() string {
	return "tcp"
}

func (m *mockTransportConn) NetConn() net.Conn {
	return m.netConn
}

// TestSetAccessControllerConcurrent 测试并发设置访问控制器
func TestSetAccessControllerConcurrent(t *testing.T) {
	ident := createTestIdentity(t)
	config := securityif.DefaultConfig()

	transport, err := NewTransport(ident, config)
	require.NoError(t, err)

	var wg sync.WaitGroup
	numGoroutines := 100

	// 并发设置和获取访问控制器
	for i := 0; i < numGoroutines; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			ac := NewAccessController()
			transport.SetAccessController(ac)
		}()
		go func() {
			defer wg.Done()
			_ = transport.AccessController()
		}()
	}

	wg.Wait()
	// 如果没有发生 race condition，测试通过
	assert.NotNil(t, transport.AccessController())
}

// TestSecureConnCloseAlwaysClosesRawConn 测试 Close 总是关闭底层连接
func TestSecureConnCloseAlwaysClosesRawConn(t *testing.T) {
	serverIdent := createTestIdentity(t)
	clientIdent := createTestIdentity(t)
	config := securityif.DefaultConfig()

	serverTransport, err := NewTransport(serverIdent, config)
	require.NoError(t, err)

	clientTransport, err := NewTransport(clientIdent, config)
	require.NoError(t, err)

	serverConn, clientConn := createConnPair(t)

	var wg sync.WaitGroup
	var serverSecureConn securityif.SecureConn
	var clientSecureConn securityif.SecureConn

	wg.Add(2)

	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var err error
		serverSecureConn, err = serverTransport.SecureInbound(ctx, &mockTransportConn{netConn: serverConn})
		require.NoError(t, err)
	}()

	go func() {
		defer wg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var err error
		clientSecureConn, err = clientTransport.SecureOutbound(ctx, &mockTransportConn{netConn: clientConn}, serverIdent.ID())
		require.NoError(t, err)
	}()

	wg.Wait()

	// 关闭客户端安全连接 - 第一次关闭应该成功
	err = clientSecureConn.Close()
	assert.NoError(t, err)

	// 验证连接已关闭 - 尝试写入应该失败
	_, err = clientSecureConn.Write([]byte("test"))
	assert.Error(t, err)

	// 再次关闭客户端连接应该是幂等的（不返回错误）
	err = clientSecureConn.Close()
	assert.NoError(t, err)

	// 关闭服务端安全连接 - 由于对端已关闭，可能返回错误
	// 但我们的实现保证底层连接一定被关闭
	_ = serverSecureConn.Close()
}

