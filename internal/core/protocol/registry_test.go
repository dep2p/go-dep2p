package protocol

import (
	"testing"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegistry_New 测试创建注册表
func TestRegistry_New(t *testing.T) {
	registry := NewRegistry()
	require.NotNil(t, registry)
	
	// 初始应该没有协议
	protocols := registry.Protocols()
	assert.Len(t, protocols, 0)
	
	t.Log("✅ Registry 创建成功")
}

// TestRegistry_Register 测试注册协议
func TestRegistry_Register(t *testing.T) {
	registry := NewRegistry()
	
	handler := func(stream pkgif.Stream) {
		// 测试处理器
	}
	
	err := registry.Register("/test/protocol/1.0.0", handler)
	assert.NoError(t, err)
	
	// 验证注册成功
	protocols := registry.Protocols()
	assert.Contains(t, protocols, pkgif.ProtocolID("/test/protocol/1.0.0"))
	
	t.Log("✅ Register 注册成功")
}

// TestRegistry_Unregister 测试注销协议
func TestRegistry_Unregister(t *testing.T) {
	registry := NewRegistry()
	
	handler := func(stream pkgif.Stream) {}
	registry.Register("/test/protocol/1.0.0", handler)
	
	// 注销
	err := registry.Unregister("/test/protocol/1.0.0")
	assert.NoError(t, err)
	
	// 验证已注销
	protocols := registry.Protocols()
	assert.NotContains(t, protocols, pkgif.ProtocolID("/test/protocol/1.0.0"))
	
	t.Log("✅ Unregister 注销成功")
}

// TestRegistry_GetHandler 测试获取处理器
func TestRegistry_GetHandler(t *testing.T) {
	registry := NewRegistry()
	
	handler := func(stream pkgif.Stream) {}
	registry.Register("/test/protocol/1.0.0", handler)
	
	// 获取处理器
	h, ok := registry.GetHandler("/test/protocol/1.0.0")
	assert.True(t, ok)
	assert.NotNil(t, h)
	
	// 不存在的协议
	_, ok = registry.GetHandler("/not/exist/1.0.0")
	assert.False(t, ok)
	
	t.Log("✅ GetHandler 获取成功")
}

// TestRegistry_DuplicateRegister 测试重复注册
func TestRegistry_DuplicateRegister(t *testing.T) {
	registry := NewRegistry()
	
	handler1 := func(stream pkgif.Stream) {}
	handler2 := func(stream pkgif.Stream) {}
	
	err := registry.Register("/test/protocol/1.0.0", handler1)
	assert.NoError(t, err)
	
	// 重复注册应该返回错误
	err = registry.Register("/test/protocol/1.0.0", handler2)
	assert.Error(t, err)
	assert.Equal(t, ErrDuplicateProtocol, err)
	
	t.Log("✅ 重复注册正确拒绝")
}

// TestRegistry_Protocols 测试列出所有协议
func TestRegistry_Protocols(t *testing.T) {
	registry := NewRegistry()
	
	handler := func(stream pkgif.Stream) {}
	
	registry.Register("/proto1/1.0.0", handler)
	registry.Register("/proto2/1.0.0", handler)
	registry.Register("/proto3/1.0.0", handler)
	
	protocols := registry.Protocols()
	assert.Len(t, protocols, 3)
	assert.Contains(t, protocols, pkgif.ProtocolID("/proto1/1.0.0"))
	assert.Contains(t, protocols, pkgif.ProtocolID("/proto2/1.0.0"))
	assert.Contains(t, protocols, pkgif.ProtocolID("/proto3/1.0.0"))
	
	t.Log("✅ Protocols 列出正确")
}

// TestRegistry_Concurrent 测试并发安全
func TestRegistry_Concurrent(t *testing.T) {
	registry := NewRegistry()
	
	done := make(chan bool, 10)
	
	// 并发注册和注销
	for i := 0; i < 10; i++ {
		go func(n int) {
			handler := func(stream pkgif.Stream) {}
			protocolID := pkgif.ProtocolID("/test/proto/" + string(rune('0'+n)))
			
			for j := 0; j < 100; j++ {
				registry.Register(protocolID, handler)
				registry.GetHandler(protocolID)
				registry.Unregister(protocolID)
			}
			done <- true
		}(i)
	}
	
	// 等待完成
	for i := 0; i < 10; i++ {
		<-done
	}
	
	t.Log("✅ 并发操作安全")
}

// TestRegistry_Interface 验证接口实现
func TestRegistry_Interface(t *testing.T) {
	var _ pkgif.ProtocolRegistry = (*Registry)(nil)
	t.Log("✅ Registry 实现 ProtocolRegistry 接口")
}
