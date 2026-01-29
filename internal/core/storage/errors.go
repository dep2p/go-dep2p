package storage

import (
	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
)

// 重导出 engine 包的错误，方便使用方直接使用
var (
	// ErrNotFound 键不存在
	ErrNotFound = engine.ErrNotFound

	// ErrKeyTooLarge 键太大
	ErrKeyTooLarge = engine.ErrKeyTooLarge

	// ErrValueTooLarge 值太大
	ErrValueTooLarge = engine.ErrValueTooLarge

	// ErrEmptyKey 空键
	ErrEmptyKey = engine.ErrEmptyKey

	// ErrClosed 引擎已关闭
	ErrClosed = engine.ErrClosed

	// ErrReadOnly 只读模式
	ErrReadOnly = engine.ErrReadOnly

	// ErrTransactionConflict 事务冲突
	ErrTransactionConflict = engine.ErrTransactionConflict

	// ErrTransactionTooLarge 事务太大
	ErrTransactionTooLarge = engine.ErrTransactionTooLarge

	// ErrTransactionDiscarded 事务已丢弃
	ErrTransactionDiscarded = engine.ErrTransactionDiscarded

	// ErrInvalidConfig 无效配置
	ErrInvalidConfig = engine.ErrInvalidConfig

	// ErrCorrupted 数据损坏
	ErrCorrupted = engine.ErrCorrupted
)

// 重导出错误检查函数
var (
	// IsNotFound 检查是否为 key not found 错误
	IsNotFound = engine.IsNotFound

	// IsClosed 检查是否为 engine closed 错误
	IsClosed = engine.IsClosed

	// IsConflict 检查是否为事务冲突错误
	IsConflict = engine.IsConflict

	// IsReadOnly 检查是否为只读模式错误
	IsReadOnly = engine.IsReadOnly

	// IsCorrupted 检查是否为数据损坏错误
	IsCorrupted = engine.IsCorrupted
)
