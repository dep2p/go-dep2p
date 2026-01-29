package coordinator

import "errors"

var (
	// ErrNotStarted 协调器未启动
	ErrNotStarted = errors.New("coordinator: not started")

	// ErrAlreadyStarted 协调器已启动
	ErrAlreadyStarted = errors.New("coordinator: already started")

	// ErrNoDiscoveries 没有注册任何发现器
	ErrNoDiscoveries = errors.New("coordinator: no discoveries registered")

	// ErrInvalidConfig 无效的配置
	ErrInvalidConfig = errors.New("coordinator: invalid config")

	// ErrTimeout 操作超时
	ErrTimeout = errors.New("coordinator: operation timeout")

	// ErrContextCanceled 上下文已取消
	ErrContextCanceled = errors.New("coordinator: context canceled")

	// ErrDiscoveryNotFound 发现器不存在
	ErrDiscoveryNotFound = errors.New("coordinator: discovery not found")

	// ErrDuplicateDiscovery 重复的发现器名称
	ErrDuplicateDiscovery = errors.New("coordinator: duplicate discovery name")
)
