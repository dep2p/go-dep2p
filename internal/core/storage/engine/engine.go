// Package engine 定义存储引擎的内部接口
//
// 本包扩展 pkg/interfaces 中的公共 Engine 接口，
// 提供批量操作、迭代器、事务等高级功能。
//
// # 接口层次
//
//	pkg/interfaces.Engine     - 公共基础接口（用户可实现）
//	    ↓
//	engine.InternalEngine     - 内部扩展接口（DeP2P 内部使用）
//
// # 线程安全
//
// 所有接口实现必须保证线程安全。批量操作和事务在提交前
// 是独立的，不影响其他并发操作。
package engine

import (
	"github.com/dep2p/go-dep2p/pkg/interfaces"
)

// InternalEngine 内部扩展接口
//
// 扩展公共 Engine 接口，提供批量操作、迭代器、事务等高级功能。
// 仅供 DeP2P 内部使用，不对外暴露。
type InternalEngine interface {
	interfaces.Engine // 嵌入公共接口

	// --- 批量操作 ---

	// NewBatch 创建新的批量写入对象
	//
	// 批量写入可以提高写入性能，将多个操作合并为一次磁盘写入。
	// 调用者负责在使用后调用 Reset() 或让其被垃圾回收。
	NewBatch() Batch

	// Write 执行批量写入
	//
	// 将批量操作原子性地写入存储。
	// 写入完成后，Batch 对象会被自动重置。
	//
	// 参数:
	//   - batch: 要写入的批量操作
	//
	// 返回:
	//   - error: 写入失败时返回错误
	Write(batch Batch) error

	// --- 迭代器 ---

	// NewIterator 创建新的迭代器
	//
	// 迭代器用于遍历存储中的键值对。
	// 调用者负责在使用后调用 Close()。
	//
	// 参数:
	//   - opts: 迭代器选项（可为 nil 使用默认选项）
	//
	// 返回:
	//   - Iterator: 新创建的迭代器
	NewIterator(opts *IteratorOptions) Iterator

	// NewPrefixIterator 创建前缀迭代器
	//
	// 仅遍历具有指定前缀的键。
	// 这是 NewIterator 的便捷方法。
	//
	// 参数:
	//   - prefix: 键前缀
	//
	// 返回:
	//   - Iterator: 新创建的前缀迭代器
	NewPrefixIterator(prefix []byte) Iterator

	// --- 事务 ---

	// NewTransaction 创建新的事务
	//
	// 事务提供读写隔离，支持原子性提交或回滚。
	// 调用者负责在使用后调用 Commit() 或 Discard()。
	//
	// 参数:
	//   - writable: true 表示读写事务，false 表示只读事务
	//
	// 返回:
	//   - Transaction: 新创建的事务
	NewTransaction(writable bool) Transaction

	// --- 维护操作 ---

	// Start 启动存储引擎
	//
	// 执行必要的初始化操作，如打开数据库文件、恢复日志等。
	// 必须在使用引擎前调用。
	//
	// 返回:
	//   - error: 启动失败时返回错误
	Start() error

	// Compact 压缩存储
	//
	// 回收已删除数据占用的空间，优化存储布局。
	// 这是一个耗时操作，建议在低负载时执行。
	//
	// 返回:
	//   - error: 压缩失败时返回错误
	Compact() error

	// Sync 同步数据到磁盘
	//
	// 确保所有已写入的数据持久化到磁盘。
	// 用于需要强一致性保证的场景。
	//
	// 返回:
	//   - error: 同步失败时返回错误
	Sync() error

	// Stats 获取引擎统计信息
	//
	// 返回:
	//   - *Stats: 当前统计信息的快照
	Stats() *Stats
}

// Batch 批量写入接口
//
// 用于将多个写入操作合并为一次原子写入，提高写入性能。
// Batch 不是线程安全的，不应在多个 goroutine 中并发使用。
type Batch interface {
	// Put 添加一个写入操作到批量中
	//
	// 参数:
	//   - key: 键
	//   - value: 值
	Put(key, value []byte)

	// Delete 添加一个删除操作到批量中
	//
	// 参数:
	//   - key: 要删除的键
	Delete(key []byte)

	// Write 执行批量写入
	//
	// 将所有操作原子性地写入存储。
	// 写入后批量对象会被自动重置。
	//
	// 返回:
	//   - error: 写入失败时返回错误
	Write() error

	// Reset 重置批量对象
	//
	// 清空所有待写入的操作，使批量对象可以重用。
	Reset()

	// Size 返回批量中的操作数量
	//
	// 返回:
	//   - int: 待写入的操作数量
	Size() int
}

// Iterator 迭代器接口
//
// 用于遍历存储中的键值对。迭代器保持创建时的快照视图，
// 不受后续写入操作影响。
//
// 使用模式:
//
//	iter := engine.NewIterator(nil)
//	defer iter.Close()
//
//	for iter.First(); iter.Valid(); iter.Next() {
//	    key := iter.Key()
//	    value := iter.Value()
//	    // 处理 key/value
//	}
//
//	if err := iter.Error(); err != nil {
//	    return err
//	}
type Iterator interface {
	// First 移动到第一个键值对
	//
	// 返回:
	//   - bool: 是否存在第一个元素
	First() bool

	// Next 移动到下一个键值对
	//
	// 返回:
	//   - bool: 是否存在下一个元素
	Next() bool

	// Valid 检查迭代器是否指向有效位置
	//
	// 返回:
	//   - bool: 当前位置是否有效
	Valid() bool

	// Key 返回当前键
	//
	// 返回的切片仅在下次迭代器操作前有效。
	// 如需保留，请复制。
	//
	// 返回:
	//   - []byte: 当前键（仅在 Valid() 为 true 时有效）
	Key() []byte

	// Value 返回当前值
	//
	// 返回的切片仅在下次迭代器操作前有效。
	// 如需保留，请复制。
	//
	// 返回:
	//   - []byte: 当前值（仅在 Valid() 为 true 时有效）
	Value() []byte

	// Close 关闭迭代器
	//
	// 释放迭代器占用的资源。
	// 关闭后不能再使用迭代器。
	Close()

	// Error 返回迭代过程中的错误
	//
	// 应在迭代完成后检查。
	//
	// 返回:
	//   - error: 迭代过程中的错误，无错误返回 nil
	Error() error
}

// IteratorOptions 迭代器选项
type IteratorOptions struct {
	// Prefix 仅迭代具有此前缀的键
	Prefix []byte

	// Reverse 是否反向迭代
	Reverse bool

	// StartKey 起始键（包含）
	StartKey []byte

	// EndKey 结束键（不包含）
	EndKey []byte

	// PrefetchSize 预取数量（0 表示使用默认值）
	PrefetchSize int

	// PrefetchValues 是否预取值（默认 true）
	PrefetchValues bool
}

// DefaultIteratorOptions 返回默认迭代器选项
func DefaultIteratorOptions() *IteratorOptions {
	return &IteratorOptions{
		PrefetchSize:   100,
		PrefetchValues: true,
	}
}

// Transaction 事务接口
//
// 提供 ACID 事务支持。读写事务可以读取和修改数据，
// 只读事务只能读取数据但开销更小。
//
// 使用模式:
//
//	txn := engine.NewTransaction(true)
//	defer txn.Discard() // 确保事务被清理
//
//	if err := txn.Set(key, value); err != nil {
//	    return err
//	}
//
//	return txn.Commit()
type Transaction interface {
	// Get 在事务中读取值
	//
	// 参数:
	//   - key: 键
	//
	// 返回:
	//   - []byte: 值
	//   - error: ErrNotFound 如果键不存在
	Get(key []byte) ([]byte, error)

	// Set 在事务中设置值
	//
	// 仅对读写事务有效。
	//
	// 参数:
	//   - key: 键
	//   - value: 值
	//
	// 返回:
	//   - error: 设置失败时返回错误
	Set(key, value []byte) error

	// Delete 在事务中删除键
	//
	// 仅对读写事务有效。
	//
	// 参数:
	//   - key: 键
	//
	// 返回:
	//   - error: 删除失败时返回错误
	Delete(key []byte) error

	// Commit 提交事务
	//
	// 将所有修改原子性地持久化。
	// 提交后事务不能再使用。
	//
	// 返回:
	//   - error: ErrTransactionConflict 如果发生写冲突
	Commit() error

	// Discard 丢弃事务
	//
	// 回滚所有未提交的修改。
	// 丢弃后事务不能再使用。
	// 多次调用是安全的。
	Discard()
}

// Stats 引擎统计信息（扩展）
//
// 比公共 EngineStats 包含更多内部统计信息。
type Stats struct {
	// 基础统计（与 interfaces.EngineStats 对应）
	KeyCount    int64 `json:"key_count"`
	DiskSize    int64 `json:"disk_size"`
	CacheHits   int64 `json:"cache_hits"`
	CacheMisses int64 `json:"cache_misses"`

	// 扩展统计
	LSMSize      int64 `json:"lsm_size"`       // LSM 树大小
	VlogSize     int64 `json:"vlog_size"`      // 值日志大小
	NumTables    int   `json:"num_tables"`     // SST 表数量
	NumLevels    int   `json:"num_levels"`     // LSM 层数
	NumCompacts  int64 `json:"num_compacts"`   // 压缩次数
	NumWrites    int64 `json:"num_writes"`     // 写入次数
	NumReads     int64 `json:"num_reads"`      // 读取次数
	NumDeletes   int64 `json:"num_deletes"`    // 删除次数
	NumBytesRead int64 `json:"num_bytes_read"` // 读取字节数
	NumBytesWritten int64 `json:"num_bytes_written"` // 写入字节数
}

// ToPublicStats 转换为公共统计信息
func (s *Stats) ToPublicStats() *interfaces.EngineStats {
	return &interfaces.EngineStats{
		KeyCount:    s.KeyCount,
		DiskSize:    s.DiskSize,
		CacheHits:   s.CacheHits,
		CacheMisses: s.CacheMisses,
	}
}
