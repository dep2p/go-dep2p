package messaging

import (
	"context"
	"testing"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandlerRegistry_Register(t *testing.T) {
	registry := NewHandlerRegistry()

	handler := func(ctx context.Context, req *interfaces.Request) (*interfaces.Response, error) {
		return &interfaces.Response{}, nil
	}

	// 注册成功
	err := registry.Register("test-protocol", handler)
	require.NoError(t, err)

	// 验证已注册
	_, exists := registry.Get("test-protocol")
	assert.True(t, exists)
}

func TestHandlerRegistry_Register_Duplicate(t *testing.T) {
	registry := NewHandlerRegistry()

	handler := func(ctx context.Context, req *interfaces.Request) (*interfaces.Response, error) {
		return &interfaces.Response{}, nil
	}

	// 首次注册
	err := registry.Register("test-protocol", handler)
	require.NoError(t, err)

	// 重复注册应失败
	err = registry.Register("test-protocol", handler)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrHandlerAlreadyRegistered)
}

func TestHandlerRegistry_Unregister(t *testing.T) {
	registry := NewHandlerRegistry()

	handler := func(ctx context.Context, req *interfaces.Request) (*interfaces.Response, error) {
		return &interfaces.Response{}, nil
	}

	// 注册
	err := registry.Register("test-protocol", handler)
	require.NoError(t, err)

	// 注销
	err = registry.Unregister("test-protocol")
	require.NoError(t, err)

	// 验证已注销
	_, exists := registry.Get("test-protocol")
	assert.False(t, exists)
}

func TestHandlerRegistry_Unregister_NotFound(t *testing.T) {
	registry := NewHandlerRegistry()

	// 注销不存在的处理器
	err := registry.Unregister("non-existent")
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrHandlerNotFound)
}

func TestHandlerRegistry_Get(t *testing.T) {
	registry := NewHandlerRegistry()

	handler := func(ctx context.Context, req *interfaces.Request) (*interfaces.Response, error) {
		return &interfaces.Response{Data: []byte("test")}, nil
	}

	// 注册
	err := registry.Register("test-protocol", handler)
	require.NoError(t, err)

	// 获取
	retrieved, exists := registry.Get("test-protocol")
	assert.True(t, exists)
	assert.NotNil(t, retrieved)

	// 调用处理器验证
	resp, err := retrieved(context.Background(), &interfaces.Request{})
	require.NoError(t, err)
	assert.Equal(t, []byte("test"), resp.Data)
}

func TestHandlerRegistry_Get_NotFound(t *testing.T) {
	registry := NewHandlerRegistry()

	// 获取不存在的处理器
	handler, exists := registry.Get("non-existent")
	assert.False(t, exists)
	assert.Nil(t, handler)
}

func TestHandlerRegistry_List(t *testing.T) {
	registry := NewHandlerRegistry()

	handler := func(ctx context.Context, req *interfaces.Request) (*interfaces.Response, error) {
		return &interfaces.Response{}, nil
	}

	// 注册多个处理器
	protocols := []string{"proto1", "proto2", "proto3"}
	for _, proto := range protocols {
		err := registry.Register(proto, handler)
		require.NoError(t, err)
	}

	// 列出所有协议
	list := registry.List()
	assert.Len(t, list, len(protocols))

	// 验证所有协议都在列表中
	for _, proto := range protocols {
		assert.Contains(t, list, proto)
	}
}

func TestHandlerRegistry_Clear(t *testing.T) {
	registry := NewHandlerRegistry()

	handler := func(ctx context.Context, req *interfaces.Request) (*interfaces.Response, error) {
		return &interfaces.Response{}, nil
	}

	// 注册多个处理器
	for i := 0; i < 5; i++ {
		err := registry.Register(string(rune('a'+i)), handler)
		require.NoError(t, err)
	}

	// 清空
	registry.Clear()

	// 验证已清空
	list := registry.List()
	assert.Empty(t, list)
}

func TestHandlerRegistry_Concurrent(t *testing.T) {
	registry := NewHandlerRegistry()

	handler := func(ctx context.Context, req *interfaces.Request) (*interfaces.Response, error) {
		return &interfaces.Response{}, nil
	}

	// 并发注册
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			defer func() { done <- true }()
			proto := string(rune('a' + id))
			err := registry.Register(proto, handler)
			if err != nil && err != ErrHandlerAlreadyRegistered {
				t.Errorf("unexpected error: %v", err)
			}
		}(i)
	}

	// 等待完成
	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证注册成功
	list := registry.List()
	assert.NotEmpty(t, list)
}
