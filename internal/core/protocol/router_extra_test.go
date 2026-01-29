package protocol

import (
	"testing"

	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/stretchr/testify/assert"
)

// TestCreateMatcher_Prefix 测试前缀匹配
func TestCreateMatcher_Prefix(t *testing.T) {
	matcher := createMatcher("/test/*")
	
	tests := []struct {
		protocol string
		match    bool
	}{
		{"/test/v1", true},
		{"/test/v2", true},
		{"/test/hello/world", true},
		{"/other/v1", false},
		{"/test", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.protocol, func(t *testing.T) {
			result := matcher(pkgif.ProtocolID(tt.protocol))
			assert.Equal(t, tt.match, result)
		})
	}
	
	t.Log("✅ createMatcher 前缀匹配测试通过")
}

// TestCreateMatcher_Suffix 测试后缀匹配
func TestCreateMatcher_Suffix(t *testing.T) {
	matcher := createMatcher("*/v1.0.0")
	
	tests := []struct {
		protocol string
		match    bool
	}{
		{"/test/v1.0.0", true},
		{"/other/v1.0.0", true},
		{"/test/v2.0.0", false},
		{"/test/", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.protocol, func(t *testing.T) {
			result := matcher(pkgif.ProtocolID(tt.protocol))
			assert.Equal(t, tt.match, result)
		})
	}
	
	t.Log("✅ createMatcher 后缀匹配测试通过")
}

// TestCreateMatcher_Middle 测试中间匹配
func TestCreateMatcher_Middle(t *testing.T) {
	matcher := createMatcher("/test/*/v1.0.0")
	
	tests := []struct {
		protocol string
		match    bool
	}{
		{"/test/hello/v1.0.0", true},
		{"/test/world/v1.0.0", true},
		{"/test/hello/v2.0.0", false},
		{"/other/hello/v1.0.0", false},
	}
	
	for _, tt := range tests {
		t.Run(tt.protocol, func(t *testing.T) {
			result := matcher(pkgif.ProtocolID(tt.protocol))
			assert.Equal(t, tt.match, result)
		})
	}
	
	t.Log("✅ createMatcher 中间匹配测试通过")
}

// TestCreateMatcher_Empty 测试空模式
func TestCreateMatcher_Empty(t *testing.T) {
	matcher := createMatcher("")
	
	// 空模式会匹配所有（因为 parts[0] 是空字符串，HasPrefix 为真）
	result := matcher(pkgif.ProtocolID("/test/v1"))
	assert.True(t, result)
	
	t.Log("✅ createMatcher 空模式测试通过")
}

// TestRouter_RemoveRoute_NotExist 测试移除不存在的路由
func TestRouter_RemoveRoute_NotExist(t *testing.T) {
	registry := NewRegistry()
	router := NewRouter(registry, nil)
	
	// 移除不存在的精确匹配
	err := router.RemoveRoute("/not/exist/1.0.0")
	assert.Error(t, err)
	assert.Equal(t, ErrProtocolNotRegistered, err)
	
	// 移除不存在的模式匹配（不报错）
	err = router.RemoveRoute("/not/exist/*")
	assert.NoError(t, err)
	
	t.Log("✅ RemoveRoute 不存在路由测试通过")
}

// TestRouter_AddRoute_Duplicate 测试重复添加路由
func TestRouter_AddRoute_Duplicate(t *testing.T) {
	registry := NewRegistry()
	router := NewRouter(registry, nil)
	
	handler := func(stream pkgif.Stream) {}
	
	// 添加路由
	err := router.AddRoute("/test/exact/1.0.0", handler)
	assert.NoError(t, err)
	
	// 重复添加
	err = router.AddRoute("/test/exact/1.0.0", handler)
	assert.Error(t, err)
	assert.Equal(t, ErrDuplicateProtocol, err)
	
	t.Log("✅ AddRoute 重复添加测试通过")
}
