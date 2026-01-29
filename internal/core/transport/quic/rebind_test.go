package quic

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                 RebindSupport 测试 - 覆盖 0% 函数
// ============================================================================

// TestRebindSupport_UpdateAddr 测试更新地址
func TestRebindSupport_UpdateAddr(t *testing.T) {
	rebind := NewRebindSupport()

	// 初始应该为 nil
	assert.Nil(t, rebind.GetCurrentAddr())

	// 更新地址
	addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080}
	rebind.UpdateAddr(addr)

	// 验证地址
	currentAddr := rebind.GetCurrentAddr()
	require.NotNil(t, currentAddr)
	assert.Equal(t, "127.0.0.1:8080", currentAddr.String())

	t.Log("✅ UpdateAddr 测试通过")
}

// TestRebindSupport_GetCurrentAddr 测试获取当前地址
func TestRebindSupport_GetCurrentAddr(t *testing.T) {
	rebind := NewRebindSupport()

	// 初始为 nil
	assert.Nil(t, rebind.GetCurrentAddr())

	// 设置地址
	addr1 := &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 4001}
	rebind.UpdateAddr(addr1)
	assert.Equal(t, addr1, rebind.GetCurrentAddr())

	// 更新地址
	addr2 := &net.UDPAddr{IP: net.ParseIP("192.168.1.2"), Port: 4002}
	rebind.UpdateAddr(addr2)
	assert.Equal(t, addr2, rebind.GetCurrentAddr())

	t.Log("✅ GetCurrentAddr 测试通过")
}

// TestRebindSupport_Rebind_NoFunc 测试没有设置函数时的 Rebind
func TestRebindSupport_Rebind_NoFunc(t *testing.T) {
	rebind := NewRebindSupport()

	ctx := context.Background()

	// 没有设置 rebindFunc 时应该直接返回 nil
	err := rebind.Rebind(ctx)
	assert.NoError(t, err)

	t.Log("✅ Rebind 无函数测试通过")
}

// TestRebindSupport_Rebind_Success 测试成功的 Rebind
func TestRebindSupport_Rebind_Success(t *testing.T) {
	rebind := NewRebindSupport()

	// 设置初始地址
	oldAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.1"), Port: 4001}
	rebind.UpdateAddr(oldAddr)

	// 设置 rebind 函数
	called := false
	newAddr := &net.UDPAddr{IP: net.ParseIP("192.168.1.2"), Port: 4002}
	rebind.SetRebindFunc(func(ctx context.Context) error {
		called = true
		rebind.UpdateAddr(newAddr)
		return nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 执行 rebind
	err := rebind.Rebind(ctx)
	assert.NoError(t, err)
	assert.True(t, called)

	// 验证地址已更新
	assert.Equal(t, newAddr, rebind.GetCurrentAddr())

	t.Log("✅ Rebind 成功测试通过")
}

// TestRebindSupport_Rebind_Failure 测试失败的 Rebind
func TestRebindSupport_Rebind_Failure(t *testing.T) {
	rebind := NewRebindSupport()

	// 设置会失败的 rebind 函数
	expectedErr := errors.New("rebind failed")
	rebind.SetRebindFunc(func(ctx context.Context) error {
		return expectedErr
	})

	ctx := context.Background()

	// 执行 rebind
	err := rebind.Rebind(ctx)
	assert.Error(t, err)
	assert.Equal(t, expectedErr, err)

	t.Log("✅ Rebind 失败测试通过")
}

// TestRebindSupport_Concurrent 测试并发安全
func TestRebindSupport_Concurrent(t *testing.T) {
	rebind := NewRebindSupport()

	// 设置 rebind 函数
	rebind.SetRebindFunc(func(ctx context.Context) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})

	// 并发执行
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func(idx int) {
			addr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 4000 + idx}
			rebind.UpdateAddr(addr)
			_ = rebind.GetCurrentAddr()
			rebind.Rebind(context.Background())
			done <- struct{}{}
		}(i)
	}

	// 等待所有 goroutine 完成
	for i := 0; i < 10; i++ {
		<-done
	}

	t.Log("✅ 并发安全测试通过")
}
