package transport

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
)

func TestNewConfig(t *testing.T) {
	cfg := NewConfig()

	// 验证默认值
	assert.True(t, cfg.EnableQUIC, "QUIC 应该默认启用")
	assert.True(t, cfg.EnableTCP, "TCP 应该默认启用")
	assert.False(t, cfg.EnableWebSocket, "WebSocket 应该默认禁用")

	assert.Equal(t, 2*time.Minute, cfg.QUICMaxIdleTimeout)
	assert.Equal(t, 1024, cfg.QUICMaxStreams)
	assert.Equal(t, 10*time.Second, cfg.TCPTimeout)
	assert.Equal(t, 30*time.Second, cfg.DialTimeout)

	t.Log("✅ NewConfig 返回正确的默认值")
}

func TestModule(t *testing.T) {
	// Module 返回 fx.Option，验证不为 nil
	module := Module()
	require.NotNil(t, module)

	t.Log("✅ Module 返回 fx.Option")
}

func TestConfigFromUnified_Nil(t *testing.T) {
	// 传入 nil 应该返回默认配置
	cfg := ConfigFromUnified(nil)

	assert.True(t, cfg.EnableQUIC)
	assert.True(t, cfg.EnableTCP)

	t.Log("✅ ConfigFromUnified(nil) 返回默认配置")
}

func TestTransportManager_WithNilDependencies(t *testing.T) {
	cfg := NewConfig()

	// 不传 identity 和 upgrader
	tm := NewTransportManager(cfg, nil, nil)
	require.NotNil(t, tm)

	assert.Equal(t, cfg, tm.config)

	t.Log("✅ TransportManager 可以在没有依赖时创建")
}

// ============================================================================
//                       Rebind 测试（网络切换关键功能）
// ============================================================================

// mockRebindableTransport 模拟支持 Rebind 的 Transport
type mockRebindableTransport struct {
	rebindCalled bool
	rebindError  error
}

func (m *mockRebindableTransport) Rebind(ctx context.Context) error {
	m.rebindCalled = true
	return m.rebindError
}

// 实现 pkgif.Transport 接口
func (m *mockRebindableTransport) Dial(ctx context.Context, raddr types.Multiaddr, peerID types.PeerID) (pkgif.Connection, error) { return nil, nil }
func (m *mockRebindableTransport) CanDial(addr types.Multiaddr) bool { return true }
func (m *mockRebindableTransport) Listen(laddr types.Multiaddr) (pkgif.Listener, error) { return nil, nil }
func (m *mockRebindableTransport) Protocols() []int { return []int{0} }
func (m *mockRebindableTransport) Close() error { return nil }

// mockNonRebindableTransport 模拟不支持 Rebind 的 Transport
type mockNonRebindableTransport struct{}

func (m *mockNonRebindableTransport) Dial(ctx context.Context, raddr types.Multiaddr, peerID types.PeerID) (pkgif.Connection, error) { return nil, nil }
func (m *mockNonRebindableTransport) CanDial(addr types.Multiaddr) bool { return true }
func (m *mockNonRebindableTransport) Listen(laddr types.Multiaddr) (pkgif.Listener, error) { return nil, nil }
func (m *mockNonRebindableTransport) Protocols() []int { return []int{0} }
func (m *mockNonRebindableTransport) Close() error { return nil }

// TestTransportManager_Rebind_WithRebindableTransport 测试有可重绑定传输时
func TestTransportManager_Rebind_WithRebindableTransport(t *testing.T) {
	cfg := NewConfig()
	tm := NewTransportManager(cfg, nil, nil)
	require.NotNil(t, tm)

	// 添加可重绑定的传输
	rebindable := &mockRebindableTransport{}
	tm.transports = append(tm.transports, rebindable)

	// 执行重绑定
	ctx := context.Background()
	err := tm.Rebind(ctx)
	require.NoError(t, err, "Rebind 应该成功")

	// 验证 Rebind 被调用
	assert.True(t, rebindable.rebindCalled, "Rebind 应该被调用")
	t.Log("✅ TransportManager.Rebind 成功调用可重绑定传输")
}

// TestTransportManager_Rebind_WithNonRebindableTransport 测试无可重绑定传输时
func TestTransportManager_Rebind_WithNonRebindableTransport(t *testing.T) {
	cfg := NewConfig()
	tm := NewTransportManager(cfg, nil, nil)
	require.NotNil(t, tm)

	// 添加不支持重绑定的传输
	nonRebindable := &mockNonRebindableTransport{}
	tm.transports = append(tm.transports, nonRebindable)

	// 执行重绑定 - 应该返回 nil（没有传输失败，但也没有传输成功 rebind）
	ctx := context.Background()
	err := tm.Rebind(ctx)
	// 当没有传输支持 Rebind 时，rebindCount=0，返回 lastErr（nil）
	assert.Nil(t, err, "无可重绑定传输时应返回 nil")
	t.Log("✅ TransportManager.Rebind 正确处理不支持重绑定的传输")
}

// TestTransportManager_Rebind_MixedTransports 测试混合传输
func TestTransportManager_Rebind_MixedTransports(t *testing.T) {
	cfg := NewConfig()
	tm := NewTransportManager(cfg, nil, nil)
	require.NotNil(t, tm)

	// 添加混合传输
	rebindable := &mockRebindableTransport{}
	nonRebindable := &mockNonRebindableTransport{}
	tm.transports = append(tm.transports, rebindable, nonRebindable)

	// 执行重绑定
	ctx := context.Background()
	err := tm.Rebind(ctx)
	require.NoError(t, err)

	// 验证可重绑定的传输被调用
	assert.True(t, rebindable.rebindCalled)
	t.Log("✅ TransportManager.Rebind 正确处理混合传输")
}

// TestTransportManager_Rebind_AllFailed 测试所有传输重绑定都失败
func TestTransportManager_Rebind_AllFailed(t *testing.T) {
	cfg := NewConfig()
	cfg.EnableQUIC = false  // 禁用内置传输
	cfg.EnableTCP = false
	
	// 直接创建空的 TransportManager
	tm := &TransportManager{
		config:     cfg,
		transports: []pkgif.Transport{},
	}

	// 只添加会失败的可重绑定传输
	rebindable := &mockRebindableTransport{
		rebindError: assert.AnError,
	}
	tm.transports = append(tm.transports, rebindable)

	// 执行重绑定
	ctx := context.Background()
	err := tm.Rebind(ctx)
	
	// 验证 Rebind 被调用，且返回错误（因为唯一的传输失败了）
	assert.True(t, rebindable.rebindCalled)
	assert.Error(t, err, "所有 Rebind 都失败时应返回错误")
	t.Log("✅ TransportManager.Rebind 正确处理所有传输重绑定失败")
}
