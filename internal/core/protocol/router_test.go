package protocol

import (
	"testing"
	"time"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/types"
	"github.com/stretchr/testify/assert"
)

// mockStream 模拟流
type mockStream struct {
	protocol string
	conn     pkgif.Connection
}

func (m *mockStream) Read(p []byte) (n int, err error)  { return 0, nil }
func (m *mockStream) Write(p []byte) (n int, err error) { return len(p), nil }
func (m *mockStream) Close() error                      { return nil }
func (m *mockStream) Reset() error                      { return nil }
func (m *mockStream) CloseWrite() error                 { return nil }
func (m *mockStream) CloseRead() error                  { return nil }
func (m *mockStream) SetDeadline(t time.Time) error     { return nil }
func (m *mockStream) SetReadDeadline(t time.Time) error { return nil }
func (m *mockStream) SetWriteDeadline(t time.Time) error { return nil }
func (m *mockStream) Protocol() string                  { return m.protocol }
func (m *mockStream) SetProtocol(protocol string)       { m.protocol = protocol }
func (m *mockStream) Conn() pkgif.Connection            { return m.conn }
func (m *mockStream) IsClosed() bool                     { return false }
func (m *mockStream) Stat() types.StreamStat {
	return types.StreamStat{
		Direction:    types.DirUnknown,
		Opened:       time.Now(),
		Protocol:     types.ProtocolID(m.protocol),
		BytesRead:    0,
		BytesWritten: 0,
	}
}

func (m *mockStream) State() types.StreamState {
	return types.StreamStateOpen
}

var _ pkgif.Stream = (*mockStream)(nil)

// TestRouter_New 测试创建路由器
func TestRouter_New(t *testing.T) {
	registry := NewRegistry()
	negotiator := NewNegotiator(registry)
	router := NewRouter(registry, negotiator)
	
	assert.NotNil(t, router)
	
	t.Log("✅ Router 创建成功")
}

// TestRouter_Route 测试路由流
func TestRouter_Route(t *testing.T) {
	registry := NewRegistry()
	router := NewRouter(registry, nil)
	
	// 记录处理器是否被调用
	handlerCalled := false
	handler := func(stream pkgif.Stream) {
		handlerCalled = true
	}
	
	// 注册处理器
	err := registry.Register("/test/proto/1.0.0", handler)
	assert.NoError(t, err)
	
	// 创建 mock stream
	stream := &mockStream{protocol: "/test/proto/1.0.0"}
	
	// 路由流
	err = router.Route(stream)
	assert.NoError(t, err)
	assert.True(t, handlerCalled)
	
	t.Log("✅ Route 路由成功")
}

// TestRouter_Route_NoHandler 测试路由没有处理器
func TestRouter_Route_NoHandler(t *testing.T) {
	registry := NewRegistry()
	router := NewRouter(registry, nil)
	
	// 创建 mock stream，协议未注册
	stream := &mockStream{protocol: "/unknown/proto/1.0.0"}
	
	// 路由流应该返回错误
	err := router.Route(stream)
	assert.Error(t, err)
	assert.Equal(t, ErrNoHandler, err)
	
	t.Log("✅ Route 无处理器测试通过")
}

// TestRouter_AddRoute 测试添加路由规则
func TestRouter_AddRoute(t *testing.T) {
	registry := NewRegistry()
	router := NewRouter(registry, nil)
	
	handler := func(stream pkgif.Stream) {}
	
	// 测试精确匹配
	err := router.AddRoute("/test/exact/1.0.0", handler)
	assert.NoError(t, err)
	
	_, ok := registry.GetHandler("/test/exact/1.0.0")
	assert.True(t, ok)
	
	// 测试模式匹配
	err = router.AddRoute("/test/*", handler)
	assert.NoError(t, err)
	
	_, ok = registry.GetHandler("/test/v1")
	assert.True(t, ok)
	
	t.Log("✅ AddRoute 添加成功")
}

// TestRouter_RemoveRoute 测试移除路由规则
func TestRouter_RemoveRoute(t *testing.T) {
	registry := NewRegistry()
	router := NewRouter(registry, nil)
	
	handler := func(stream pkgif.Stream) {}
	
	// 测试移除精确匹配
	err := router.AddRoute("/test/exact/1.0.0", handler)
	assert.NoError(t, err)
	
	err = router.RemoveRoute("/test/exact/1.0.0")
	assert.NoError(t, err)
	
	// 验证已移除
	_, ok := registry.GetHandler("/test/exact/1.0.0")
	assert.False(t, ok)
	
	// 测试移除模式匹配
	err = router.AddRoute("/test/*", handler)
	assert.NoError(t, err)
	
	err = router.RemoveRoute("/test/*")
	assert.NoError(t, err)
	
	t.Log("✅ RemoveRoute 移除成功")
}

// TestRouter_PatternMatch 测试模式匹配
func TestRouter_PatternMatch(t *testing.T) {
	registry := NewRegistry()
	router := NewRouter(registry, nil)
	
	handler := func(stream pkgif.Stream) {}
	
	// 添加模式路由
	err := router.AddRoute("/test/*", handler)
	assert.NoError(t, err)
	
	// 测试匹配
	tests := []struct {
		protocol string
		match    bool
	}{
		{"/test/v1", true},
		{"/test/v2", true},
		{"/test/hello", true},
		{"/other/v1", false},
		{"/test", false}, // 前缀不完全匹配
	}
	
	for _, tt := range tests {
		t.Run(tt.protocol, func(t *testing.T) {
			_, ok := registry.GetHandler(pkgif.ProtocolID(tt.protocol))
			assert.Equal(t, tt.match, ok, "协议 %s 匹配结果应该是 %v", tt.protocol, tt.match)
		})
	}
	
	t.Log("✅ PatternMatch 模式匹配成功")
}

