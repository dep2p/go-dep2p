package badger

import (
	"sync/atomic"

	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dgraph-io/badger/v4"
)

// Transaction BadgerDB 事务实现
type Transaction struct {
	txn       *badger.Txn
	writable  bool
	committed atomic.Bool
	discarded atomic.Bool
}

// Get 在事务中读取值
func (t *Transaction) Get(key []byte) ([]byte, error) {
	if t.discarded.Load() {
		return nil, engine.ErrTransactionDiscarded
	}

	if len(key) == 0 {
		return nil, engine.ErrEmptyKey
	}

	item, err := t.txn.Get(key)
	if err != nil {
		return nil, convertError(err)
	}

	return item.ValueCopy(nil)
}

// Set 在事务中设置值
func (t *Transaction) Set(key, value []byte) error {
	if t.discarded.Load() {
		return engine.ErrTransactionDiscarded
	}

	if !t.writable {
		return engine.ErrReadOnly
	}

	if len(key) == 0 {
		return engine.ErrEmptyKey
	}

	err := t.txn.Set(key, value)
	return convertError(err)
}

// Delete 在事务中删除键
func (t *Transaction) Delete(key []byte) error {
	if t.discarded.Load() {
		return engine.ErrTransactionDiscarded
	}

	if !t.writable {
		return engine.ErrReadOnly
	}

	if len(key) == 0 {
		return engine.ErrEmptyKey
	}

	err := t.txn.Delete(key)
	return convertError(err)
}

// Commit 提交事务
func (t *Transaction) Commit() error {
	if t.discarded.Load() {
		return engine.ErrTransactionDiscarded
	}

	if t.committed.Swap(true) {
		return nil // 已经提交
	}

	err := t.txn.Commit()
	if err != nil {
		return convertError(err)
	}

	return nil
}

// Discard 丢弃事务
func (t *Transaction) Discard() {
	if t.discarded.Swap(true) {
		return // 已经丢弃
	}

	if t.committed.Load() {
		return // 已经提交
	}

	t.txn.Discard()
}

// NewIterator 在事务中创建迭代器
func (t *Transaction) NewIterator(opts *engine.IteratorOptions) engine.Iterator {
	if t.discarded.Load() {
		return &Iterator{closed: atomic.Bool{}}
	}

	if opts == nil {
		opts = engine.DefaultIteratorOptions()
	}

	badgerOpts := badger.DefaultIteratorOptions
	badgerOpts.Reverse = opts.Reverse
	badgerOpts.PrefetchSize = opts.PrefetchSize
	badgerOpts.PrefetchValues = opts.PrefetchValues

	if len(opts.Prefix) > 0 {
		badgerOpts.Prefix = opts.Prefix
	}

	return &Iterator{
		txn:      t.txn,
		iter:     t.txn.NewIterator(badgerOpts),
		prefix:   opts.Prefix,
		startKey: opts.StartKey,
		endKey:   opts.EndKey,
	}
}

// IsWritable 返回事务是否可写
func (t *Transaction) IsWritable() bool {
	return t.writable
}

// IsCommitted 返回事务是否已提交
func (t *Transaction) IsCommitted() bool {
	return t.committed.Load()
}

// IsDiscarded 返回事务是否已丢弃
func (t *Transaction) IsDiscarded() bool {
	return t.discarded.Load()
}

// 编译时检查接口实现
var _ engine.Transaction = (*Transaction)(nil)
