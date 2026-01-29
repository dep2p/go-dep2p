package protocol

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNewConfig 测试创建配置
func TestNewConfig(t *testing.T) {
	config := NewConfig()
	
	assert.Equal(t, 10*time.Second, config.NegotiationTimeout)
	
	t.Log("✅ NewConfig 创建成功")
}

// TestProvideRegistry 测试提供注册表
func TestProvideRegistry(t *testing.T) {
	registry := ProvideRegistry()
	
	require.NotNil(t, registry)
	assert.Empty(t, registry.Protocols())
	
	t.Log("✅ ProvideRegistry 提供成功")
}

// TestProvideNegotiator 测试提供协商器
func TestProvideNegotiator(t *testing.T) {
	registry := ProvideRegistry()
	negotiator := ProvideNegotiator(registry)
	
	require.NotNil(t, negotiator)
	
	// 类型断言验证
	neg, ok := negotiator.(*Negotiator)
	require.True(t, ok)
	assert.NotNil(t, neg.registry)
	
	t.Log("✅ ProvideNegotiator 提供成功")
}

// TestProvideRouter 测试提供路由器
func TestProvideRouter(t *testing.T) {
	registry := ProvideRegistry()
	negotiator := ProvideNegotiator(registry)
	router := ProvideRouter(registry, negotiator)
	
	require.NotNil(t, router)
	
	// 验证内部字段（ProvideRouter 现在返回 *Router 具体类型）
	assert.NotNil(t, router.registry)
	assert.NotNil(t, router.negotiator)
	
	t.Log("✅ ProvideRouter 提供成功")
}

// TestModule_Exists 测试模块函数存在
func TestModule_Exists(t *testing.T) {
	module := Module()
	assert.NotNil(t, module)
	t.Log("✅ Module 函数存在")
}

