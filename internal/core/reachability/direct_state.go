// Package reachability 提供可达性管理功能
//
// DirectAddrUpdateState 实现直接地址更新状态机：
//   - 管理地址发现、验证、发布的完整流程
//   - 支持超时和重试
//   - 跟踪状态转换历史
package reachability

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var stateLogger = log.Logger("reachability/state")

// ============================================================================
//                              状态定义
// ============================================================================

// DirectAddrState 直接地址更新状态
type DirectAddrState int

const (
	// StateIdle 空闲状态
	StateIdle DirectAddrState = iota

	// StateDiscovering 发现地址中
	StateDiscovering

	// StateValidating 验证地址中
	StateValidating

	// StatePublishing 发布地址中
	StatePublishing

	// StateComplete 更新完成
	StateComplete

	// StateFailed 更新失败
	StateFailed
)

// String 返回状态字符串
func (s DirectAddrState) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateDiscovering:
		return "discovering"
	case StateValidating:
		return "validating"
	case StatePublishing:
		return "publishing"
	case StateComplete:
		return "complete"
	case StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// ============================================================================
//                              错误定义
// ============================================================================

var (
	// ErrStateMachineClosed 状态机已关闭
	ErrStateMachineClosed = errors.New("state machine closed")

	// ErrInvalidStateTransition 无效的状态转换
	ErrInvalidStateTransition = errors.New("invalid state transition")

	// ErrDiscoveryTimeout 发现超时
	ErrDiscoveryTimeout = errors.New("discovery timeout")

	// ErrValidationFailed 验证失败
	ErrValidationFailed = errors.New("validation failed")

	// ErrPublishFailed 发布失败
	ErrPublishFailed = errors.New("publish failed")
)

// ============================================================================
//                              状态机配置
// ============================================================================

// DirectAddrStateMachineConfig 状态机配置
type DirectAddrStateMachineConfig struct {
	// DiscoveryTimeout 发现超时
	DiscoveryTimeout time.Duration

	// ValidationTimeout 验证超时
	ValidationTimeout time.Duration

	// PublishTimeout 发布超时
	PublishTimeout time.Duration

	// MaxRetries 最大重试次数
	MaxRetries int
}

// DefaultDirectAddrStateMachineConfig 返回默认配置
func DefaultDirectAddrStateMachineConfig() DirectAddrStateMachineConfig {
	return DirectAddrStateMachineConfig{
		DiscoveryTimeout:  30 * time.Second,
		ValidationTimeout: 10 * time.Second,
		PublishTimeout:    15 * time.Second,
		MaxRetries:        3,
	}
}

// ============================================================================
//                              状态机结构
// ============================================================================

// DirectAddrUpdateStateMachine 直接地址更新状态机
//
// 管理地址更新的完整生命周期：
//   Idle -> Discovering -> Validating -> Publishing -> Complete
//                  \           \              \
//                   -> Failed   -> Failed      -> Failed
type DirectAddrUpdateStateMachine struct {
	// 配置
	config DirectAddrStateMachineConfig

	// 当前状态
	state   DirectAddrState
	stateMu sync.RWMutex

	// 发现的地址
	discoveredAddrs []string

	// 验证后的地址
	validatedAddrs []string

	// 错误信息
	lastError error

	// 重试计数
	retryCount int

	// 状态历史
	stateHistory []StateTransition

	// 回调函数
	onDiscover func(ctx context.Context) ([]string, error)
	onValidate func(ctx context.Context, addrs []string) ([]string, error)
	onPublish  func(ctx context.Context, addrs []string) error

	// 状态
	closed bool
}

// StateTransition 状态转换记录
type StateTransition struct {
	From      DirectAddrState
	To        DirectAddrState
	Timestamp time.Time
	Error     error
}

// ============================================================================
//                              构造函数
// ============================================================================

// NewDirectAddrUpdateStateMachine 创建状态机
func NewDirectAddrUpdateStateMachine(config DirectAddrStateMachineConfig) *DirectAddrUpdateStateMachine {
	return &DirectAddrUpdateStateMachine{
		config:       config,
		state:        StateIdle,
		stateHistory: make([]StateTransition, 0),
	}
}

// ============================================================================
//                              回调设置
// ============================================================================

// SetDiscoverCallback 设置发现回调
func (sm *DirectAddrUpdateStateMachine) SetDiscoverCallback(fn func(ctx context.Context) ([]string, error)) {
	sm.onDiscover = fn
}

// SetValidateCallback 设置验证回调
func (sm *DirectAddrUpdateStateMachine) SetValidateCallback(fn func(ctx context.Context, addrs []string) ([]string, error)) {
	sm.onValidate = fn
}

// SetPublishCallback 设置发布回调
func (sm *DirectAddrUpdateStateMachine) SetPublishCallback(fn func(ctx context.Context, addrs []string) error) {
	sm.onPublish = fn
}

// ============================================================================
//                              状态转换
// ============================================================================

// State 返回当前状态
func (sm *DirectAddrUpdateStateMachine) State() DirectAddrState {
	sm.stateMu.RLock()
	defer sm.stateMu.RUnlock()
	return sm.state
}

// transition 执行状态转换
func (sm *DirectAddrUpdateStateMachine) transition(to DirectAddrState, err error) error {
	sm.stateMu.Lock()
	defer sm.stateMu.Unlock()

	from := sm.state

	// 记录转换
	sm.stateHistory = append(sm.stateHistory, StateTransition{
		From:      from,
		To:        to,
		Timestamp: time.Now(),
		Error:     err,
	})

	sm.state = to
	sm.lastError = err

	stateLogger.Debug("状态转换",
		"from", from.String(),
		"to", to.String(),
		"error", err)

	return nil
}

// ============================================================================
//                              核心流程
// ============================================================================

// StartUpdate 开始更新流程
func (sm *DirectAddrUpdateStateMachine) StartUpdate(ctx context.Context) error {
	if sm.closed {
		return ErrStateMachineClosed
	}

	// 重置状态
	sm.retryCount = 0
	sm.discoveredAddrs = nil
	sm.validatedAddrs = nil

	return sm.runStateMachine(ctx)
}

// runStateMachine 运行状态机
func (sm *DirectAddrUpdateStateMachine) runStateMachine(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			sm.transition(StateFailed, ctx.Err())
			return ctx.Err()
		default:
		}

		currentState := sm.State()

		switch currentState {
		case StateIdle:
			sm.transition(StateDiscovering, nil)

		case StateDiscovering:
			if err := sm.doDiscover(ctx); err != nil {
				return sm.handleError(ctx, err)
			}
			sm.transition(StateValidating, nil)

		case StateValidating:
			if err := sm.doValidate(ctx); err != nil {
				return sm.handleError(ctx, err)
			}
			sm.transition(StatePublishing, nil)

		case StatePublishing:
			if err := sm.doPublish(ctx); err != nil {
				return sm.handleError(ctx, err)
			}
			sm.transition(StateComplete, nil)
			return nil

		case StateComplete:
			return nil

		case StateFailed:
			return sm.lastError

		default:
			return ErrInvalidStateTransition
		}
	}
}

// doDiscover 执行发现
func (sm *DirectAddrUpdateStateMachine) doDiscover(ctx context.Context) error {
	if sm.onDiscover == nil {
		return nil
	}

	discoverCtx, cancel := context.WithTimeout(ctx, sm.config.DiscoveryTimeout)
	defer cancel()

	addrs, err := sm.onDiscover(discoverCtx)
	if err != nil {
		return err
	}

	sm.discoveredAddrs = addrs
	return nil
}

// doValidate 执行验证
func (sm *DirectAddrUpdateStateMachine) doValidate(ctx context.Context) error {
	if sm.onValidate == nil {
		sm.validatedAddrs = sm.discoveredAddrs
		return nil
	}

	validateCtx, cancel := context.WithTimeout(ctx, sm.config.ValidationTimeout)
	defer cancel()

	addrs, err := sm.onValidate(validateCtx, sm.discoveredAddrs)
	if err != nil {
		return err
	}

	sm.validatedAddrs = addrs
	return nil
}

// doPublish 执行发布
func (sm *DirectAddrUpdateStateMachine) doPublish(ctx context.Context) error {
	if sm.onPublish == nil {
		return nil
	}

	publishCtx, cancel := context.WithTimeout(ctx, sm.config.PublishTimeout)
	defer cancel()

	return sm.onPublish(publishCtx, sm.validatedAddrs)
}

// handleError 处理错误
func (sm *DirectAddrUpdateStateMachine) handleError(ctx context.Context, err error) error {
	sm.retryCount++

	if sm.retryCount >= sm.config.MaxRetries {
		sm.transition(StateFailed, err)
		return err
	}

	stateLogger.Debug("重试更新", "retry", sm.retryCount, "maxRetries", sm.config.MaxRetries)

	// 重置到发现状态
	sm.transition(StateDiscovering, nil)
	return sm.runStateMachine(ctx)
}

// Reset 重置状态机
func (sm *DirectAddrUpdateStateMachine) Reset() {
	sm.stateMu.Lock()
	defer sm.stateMu.Unlock()

	sm.state = StateIdle
	sm.discoveredAddrs = nil
	sm.validatedAddrs = nil
	sm.lastError = nil
	sm.retryCount = 0
}

// Close 关闭状态机
func (sm *DirectAddrUpdateStateMachine) Close() {
	sm.closed = true
}

// ============================================================================
//                              状态查询
// ============================================================================

// LastError 返回最后的错误
func (sm *DirectAddrUpdateStateMachine) LastError() error {
	sm.stateMu.RLock()
	defer sm.stateMu.RUnlock()
	return sm.lastError
}

// DiscoveredAddrs 返回发现的地址
func (sm *DirectAddrUpdateStateMachine) DiscoveredAddrs() []string {
	sm.stateMu.RLock()
	defer sm.stateMu.RUnlock()
	return sm.discoveredAddrs
}

// ValidatedAddrs 返回验证后的地址
func (sm *DirectAddrUpdateStateMachine) ValidatedAddrs() []string {
	sm.stateMu.RLock()
	defer sm.stateMu.RUnlock()
	return sm.validatedAddrs
}

// StateHistory 返回状态历史
func (sm *DirectAddrUpdateStateMachine) StateHistory() []StateTransition {
	sm.stateMu.RLock()
	defer sm.stateMu.RUnlock()

	history := make([]StateTransition, len(sm.stateHistory))
	copy(history, sm.stateHistory)
	return history
}

// RetryCount 返回重试次数
func (sm *DirectAddrUpdateStateMachine) RetryCount() int {
	return sm.retryCount
}
