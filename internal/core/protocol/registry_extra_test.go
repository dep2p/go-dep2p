package protocol

import (
	"testing"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/stretchr/testify/assert"
)

// TestRegistry_AddMatcher 测试添加模式匹配器
func TestRegistry_AddMatcher(t *testing.T) {
	registry := NewRegistry()
	
	handler := func(stream pkgif.Stream) {}
	
	// 添加模式匹配器
	matchFunc := func(proto pkgif.ProtocolID) bool {
		return string(proto)[:6] == "/test/"
	}
	
	registry.AddMatcher("/test/*", matchFunc, handler)
	
	// 验证模式匹配
	h, ok := registry.GetHandler("/test/v1")
	assert.True(t, ok)
	assert.NotNil(t, h)
	
	h, ok = registry.GetHandler("/test/v2")
	assert.True(t, ok)
	assert.NotNil(t, h)
	
	// 不匹配的协议
	_, ok = registry.GetHandler("/other/v1")
	assert.False(t, ok)
	
	t.Log("✅ AddMatcher 模式匹配正确")
}

// TestRegistry_RemoveMatcher 测试移除模式匹配器
func TestRegistry_RemoveMatcher(t *testing.T) {
	registry := NewRegistry()
	
	handler := func(stream pkgif.Stream) {}
	matchFunc := func(proto pkgif.ProtocolID) bool {
		return true
	}
	
	registry.AddMatcher("/test/*", matchFunc, handler)
	
	// 移除前可以匹配
	_, ok := registry.GetHandler("/test/v1")
	assert.True(t, ok)
	
	// 移除
	registry.RemoveMatcher("/test/*")
	
	// 移除后不能匹配
	_, ok = registry.GetHandler("/test/v1")
	assert.False(t, ok)
	
	t.Log("✅ RemoveMatcher 移除正确")
}

// TestRegistry_Clear 测试清空
func TestRegistry_Clear(t *testing.T) {
	registry := NewRegistry()
	
	handler := func(stream pkgif.Stream) {}
	
	registry.Register("/proto1/1.0.0", handler)
	registry.Register("/proto2/1.0.0", handler)
	
	assert.Len(t, registry.Protocols(), 2)
	
	registry.Clear()
	
	assert.Len(t, registry.Protocols(), 0)
	
	t.Log("✅ Clear 清空正确")
}

// TestRegistry_UnregisterNotExist 测试注销不存在的协议
func TestRegistry_UnregisterNotExist(t *testing.T) {
	registry := NewRegistry()
	
	err := registry.Unregister("/not/exist")
	assert.Error(t, err)
	assert.Equal(t, ErrProtocolNotRegistered, err)
	
	t.Log("✅ 注销不存在的协议返回错误")
}
