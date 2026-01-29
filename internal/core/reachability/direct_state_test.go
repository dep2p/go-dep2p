package reachability

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
//                              配置测试
// ============================================================================

// TestDefaultDirectAddrStateMachineConfig 测试默认配置
func TestDefaultDirectAddrStateMachineConfig(t *testing.T) {
	config := DefaultDirectAddrStateMachineConfig()

	assert.Equal(t, 30*time.Second, config.DiscoveryTimeout)
	assert.Equal(t, 10*time.Second, config.ValidationTimeout)
	assert.Equal(t, 15*time.Second, config.PublishTimeout)
	assert.Equal(t, 3, config.MaxRetries)
}

// TestNewDirectAddrUpdateStateMachine 测试创建状态机
func TestNewDirectAddrUpdateStateMachine(t *testing.T) {
	sm := NewDirectAddrUpdateStateMachine(DefaultDirectAddrStateMachineConfig())
	require.NotNil(t, sm)

	assert.Equal(t, StateIdle, sm.State())
}

// ============================================================================
//                              状态测试
// ============================================================================

// TestDirectAddrState_String 测试状态字符串
func TestDirectAddrState_String(t *testing.T) {
	tests := []struct {
		state DirectAddrState
		want  string
	}{
		{StateIdle, "idle"},
		{StateDiscovering, "discovering"},
		{StateValidating, "validating"},
		{StatePublishing, "publishing"},
		{StateComplete, "complete"},
		{StateFailed, "failed"},
		{DirectAddrState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.state.String())
		})
	}
}

// ============================================================================
//                              状态转换测试
// ============================================================================

// TestDirectAddrUpdateStateMachine_SuccessfulUpdate 测试成功更新流程
func TestDirectAddrUpdateStateMachine_SuccessfulUpdate(t *testing.T) {
	sm := NewDirectAddrUpdateStateMachine(DefaultDirectAddrStateMachineConfig())

	discoveredAddrs := []string{"/ip4/1.2.3.4/tcp/4001"}
	validatedAddrs := []string{"/ip4/1.2.3.4/tcp/4001"}

	// 设置回调
	sm.SetDiscoverCallback(func(ctx context.Context) ([]string, error) {
		return discoveredAddrs, nil
	})

	sm.SetValidateCallback(func(ctx context.Context, addrs []string) ([]string, error) {
		return validatedAddrs, nil
	})

	sm.SetPublishCallback(func(ctx context.Context, addrs []string) error {
		return nil
	})

	// 启动更新
	err := sm.StartUpdate(context.Background())
	assert.NoError(t, err)

	// 验证最终状态
	assert.Equal(t, StateComplete, sm.State())
	assert.Equal(t, discoveredAddrs, sm.DiscoveredAddrs())
	assert.Equal(t, validatedAddrs, sm.ValidatedAddrs())
	assert.Nil(t, sm.LastError())
}

// TestDirectAddrUpdateStateMachine_DiscoveryFailure 测试发现失败
func TestDirectAddrUpdateStateMachine_DiscoveryFailure(t *testing.T) {
	config := DefaultDirectAddrStateMachineConfig()
	config.MaxRetries = 1 // 只重试一次
	sm := NewDirectAddrUpdateStateMachine(config)

	discoverErr := errors.New("discovery failed")
	callCount := 0

	sm.SetDiscoverCallback(func(ctx context.Context) ([]string, error) {
		callCount++
		return nil, discoverErr
	})

	// 启动更新
	err := sm.StartUpdate(context.Background())
	assert.Error(t, err)

	// 验证最终状态
	assert.Equal(t, StateFailed, sm.State())
	assert.Equal(t, 1, callCount) // 初始 + 重试 = 2 次，但 MaxRetries=1 表示只重试一次
}

// TestDirectAddrUpdateStateMachine_ValidationFailure 测试验证失败
func TestDirectAddrUpdateStateMachine_ValidationFailure(t *testing.T) {
	config := DefaultDirectAddrStateMachineConfig()
	config.MaxRetries = 0 // 不重试
	sm := NewDirectAddrUpdateStateMachine(config)

	validateErr := errors.New("validation failed")

	sm.SetDiscoverCallback(func(ctx context.Context) ([]string, error) {
		return []string{"/ip4/1.2.3.4/tcp/4001"}, nil
	})

	sm.SetValidateCallback(func(ctx context.Context, addrs []string) ([]string, error) {
		return nil, validateErr
	})

	// 启动更新
	err := sm.StartUpdate(context.Background())
	assert.Error(t, err)

	// 验证最终状态
	assert.Equal(t, StateFailed, sm.State())
}

// TestDirectAddrUpdateStateMachine_PublishFailure 测试发布失败
func TestDirectAddrUpdateStateMachine_PublishFailure(t *testing.T) {
	config := DefaultDirectAddrStateMachineConfig()
	config.MaxRetries = 0
	sm := NewDirectAddrUpdateStateMachine(config)

	publishErr := errors.New("publish failed")

	sm.SetDiscoverCallback(func(ctx context.Context) ([]string, error) {
		return []string{"/ip4/1.2.3.4/tcp/4001"}, nil
	})

	sm.SetValidateCallback(func(ctx context.Context, addrs []string) ([]string, error) {
		return addrs, nil
	})

	sm.SetPublishCallback(func(ctx context.Context, addrs []string) error {
		return publishErr
	})

	// 启动更新
	err := sm.StartUpdate(context.Background())
	assert.Error(t, err)

	// 验证最终状态
	assert.Equal(t, StateFailed, sm.State())
}

// ============================================================================
//                              重试测试
// ============================================================================

// TestDirectAddrUpdateStateMachine_RetryOnFailure 测试失败后重试
func TestDirectAddrUpdateStateMachine_RetryOnFailure(t *testing.T) {
	config := DefaultDirectAddrStateMachineConfig()
	config.MaxRetries = 3
	sm := NewDirectAddrUpdateStateMachine(config)

	callCount := 0
	sm.SetDiscoverCallback(func(ctx context.Context) ([]string, error) {
		callCount++
		if callCount < 3 {
			return nil, errors.New("temporary failure")
		}
		return []string{"/ip4/1.2.3.4/tcp/4001"}, nil
	})

	sm.SetValidateCallback(func(ctx context.Context, addrs []string) ([]string, error) {
		return addrs, nil
	})

	sm.SetPublishCallback(func(ctx context.Context, addrs []string) error {
		return nil
	})

	// 启动更新
	err := sm.StartUpdate(context.Background())
	assert.NoError(t, err)

	// 验证成功
	assert.Equal(t, StateComplete, sm.State())
	assert.Equal(t, 3, callCount)
}

// ============================================================================
//                              上下文取消测试
// ============================================================================

// TestDirectAddrUpdateStateMachine_ContextCancel 测试上下文取消
func TestDirectAddrUpdateStateMachine_ContextCancel(t *testing.T) {
	sm := NewDirectAddrUpdateStateMachine(DefaultDirectAddrStateMachineConfig())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消

	sm.SetDiscoverCallback(func(ctx context.Context) ([]string, error) {
		return []string{"/ip4/1.2.3.4/tcp/4001"}, nil
	})

	// 启动更新（应该因为上下文取消而失败）
	err := sm.StartUpdate(ctx)
	assert.Error(t, err)
	assert.Equal(t, StateFailed, sm.State())
}

// ============================================================================
//                              重置测试
// ============================================================================

// TestDirectAddrUpdateStateMachine_Reset 测试重置
func TestDirectAddrUpdateStateMachine_Reset(t *testing.T) {
	sm := NewDirectAddrUpdateStateMachine(DefaultDirectAddrStateMachineConfig())

	sm.SetDiscoverCallback(func(ctx context.Context) ([]string, error) {
		return []string{"/ip4/1.2.3.4/tcp/4001"}, nil
	})

	// 完成一次更新
	err := sm.StartUpdate(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, StateComplete, sm.State())

	// 重置
	sm.Reset()

	assert.Equal(t, StateIdle, sm.State())
	assert.Empty(t, sm.DiscoveredAddrs())
	assert.Empty(t, sm.ValidatedAddrs())
	assert.Nil(t, sm.LastError())
}

// ============================================================================
//                              状态历史测试
// ============================================================================

// TestDirectAddrUpdateStateMachine_StateHistory 测试状态历史
func TestDirectAddrUpdateStateMachine_StateHistory(t *testing.T) {
	sm := NewDirectAddrUpdateStateMachine(DefaultDirectAddrStateMachineConfig())

	sm.SetDiscoverCallback(func(ctx context.Context) ([]string, error) {
		return []string{"/ip4/1.2.3.4/tcp/4001"}, nil
	})

	// 完成更新
	err := sm.StartUpdate(context.Background())
	assert.NoError(t, err)

	// 检查历史
	history := sm.StateHistory()
	assert.NotEmpty(t, history)

	// 验证状态转换顺序
	expectedStates := []DirectAddrState{
		StateDiscovering,
		StateValidating,
		StatePublishing,
		StateComplete,
	}

	for i, expected := range expectedStates {
		if i < len(history) {
			assert.Equal(t, expected, history[i].To)
		}
	}
}

// ============================================================================
//                              关闭测试
// ============================================================================

// TestDirectAddrUpdateStateMachine_Close 测试关闭
func TestDirectAddrUpdateStateMachine_Close(t *testing.T) {
	sm := NewDirectAddrUpdateStateMachine(DefaultDirectAddrStateMachineConfig())
	sm.Close()

	err := sm.StartUpdate(context.Background())
	assert.ErrorIs(t, err, ErrStateMachineClosed)
}

// ============================================================================
//                              无回调测试
// ============================================================================

// TestDirectAddrUpdateStateMachine_NoCallbacks 测试无回调
func TestDirectAddrUpdateStateMachine_NoCallbacks(t *testing.T) {
	sm := NewDirectAddrUpdateStateMachine(DefaultDirectAddrStateMachineConfig())

	// 不设置任何回调，应该能正常完成
	err := sm.StartUpdate(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, StateComplete, sm.State())
}
