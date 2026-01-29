package badger

import (
	"sync/atomic"

	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dgraph-io/badger/v4"
)

// WriteBatch BadgerDB 批量写入实现
type WriteBatch struct {
	db     *Engine
	batch  *badger.WriteBatch
	count  atomic.Int32
	closed atomic.Bool
}

// Put 添加一个写入操作到批量中
func (b *WriteBatch) Put(key, value []byte) {
	if b.closed.Load() {
		return
	}

	if len(key) == 0 {
		return
	}

	// BadgerDB WriteBatch.Set 不返回错误，错误在 Flush 时返回
	_ = b.batch.Set(key, value)
	b.count.Add(1)
}

// Delete 添加一个删除操作到批量中
func (b *WriteBatch) Delete(key []byte) {
	if b.closed.Load() {
		return
	}

	if len(key) == 0 {
		return
	}

	// BadgerDB WriteBatch.Delete 不返回错误，错误在 Flush 时返回
	_ = b.batch.Delete(key)
	b.count.Add(1)
}

// Write 执行批量写入
func (b *WriteBatch) Write() error {
	if b.closed.Load() {
		return engine.ErrBatchClosed
	}

	if b.db.closed.Load() {
		return engine.ErrClosed
	}

	if b.db.config.ReadOnly {
		return engine.ErrReadOnly
	}

	err := b.batch.Flush()
	if err != nil {
		return convertError(err)
	}

	// 更新统计
	count := int64(b.count.Load())
	b.db.stats.numWrites.Add(count)

	// 重置
	b.Reset()

	return nil
}

// Reset 重置批量对象
func (b *WriteBatch) Reset() {
	if b.closed.Load() {
		return
	}

	b.count.Store(0)
	// 注意：BadgerDB WriteBatch 在 Flush 后会自动重置
	// 但我们可能需要在不 Flush 的情况下重置
	// 由于 WriteBatch 没有 Reset 方法，我们需要重新创建
	b.batch = b.db.db.NewWriteBatch()
}

// Size 返回批量中的操作数量
func (b *WriteBatch) Size() int {
	return int(b.count.Load())
}

// Close 关闭批量对象
func (b *WriteBatch) Close() error {
	if b.closed.Swap(true) {
		return nil
	}

	b.batch.Cancel()
	return nil
}

// 编译时检查接口实现
var _ engine.Batch = (*WriteBatch)(nil)
