package engine

import "errors"

// 存储引擎错误定义
var (
	// ErrNotFound 键不存在
	ErrNotFound = errors.New("storage: key not found")

	// ErrKeyTooLarge 键太大
	ErrKeyTooLarge = errors.New("storage: key too large")

	// ErrValueTooLarge 值太大
	ErrValueTooLarge = errors.New("storage: value too large")

	// ErrEmptyKey 空键
	ErrEmptyKey = errors.New("storage: empty key")

	// ErrClosed 引擎已关闭
	ErrClosed = errors.New("storage: engine closed")

	// ErrReadOnly 只读模式
	ErrReadOnly = errors.New("storage: read-only mode")

	// ErrTransactionConflict 事务冲突
	ErrTransactionConflict = errors.New("storage: transaction conflict")

	// ErrTransactionTooLarge 事务太大
	ErrTransactionTooLarge = errors.New("storage: transaction too large")

	// ErrTransactionDiscarded 事务已丢弃
	ErrTransactionDiscarded = errors.New("storage: transaction discarded")

	// ErrInvalidConfig 无效配置
	ErrInvalidConfig = errors.New("storage: invalid configuration")

	// ErrCorrupted 数据损坏
	ErrCorrupted = errors.New("storage: data corrupted")

	// ErrIteratorClosed 迭代器已关闭
	ErrIteratorClosed = errors.New("storage: iterator closed")

	// ErrBatchClosed 批量操作已关闭
	ErrBatchClosed = errors.New("storage: batch closed")
)

// IsNotFound 检查是否为 key not found 错误
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsClosed 检查是否为 engine closed 错误
func IsClosed(err error) bool {
	return errors.Is(err, ErrClosed)
}

// IsConflict 检查是否为事务冲突错误
func IsConflict(err error) bool {
	return errors.Is(err, ErrTransactionConflict)
}

// IsReadOnly 检查是否为只读模式错误
func IsReadOnly(err error) bool {
	return errors.Is(err, ErrReadOnly)
}

// IsCorrupted 检查是否为数据损坏错误
func IsCorrupted(err error) bool {
	return errors.Is(err, ErrCorrupted)
}
