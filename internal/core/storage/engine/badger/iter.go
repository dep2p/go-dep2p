package badger

import (
	"bytes"
	"sync/atomic"

	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dgraph-io/badger/v4"
)

// Iterator BadgerDB 迭代器实现
type Iterator struct {
	txn      *badger.Txn
	iter     *badger.Iterator
	prefix   []byte
	startKey []byte
	endKey   []byte
	started  bool
	closed   atomic.Bool
	err      error
}

// First 移动到第一个键值对
func (it *Iterator) First() bool {
	if it.closed.Load() {
		return false
	}

	it.started = true

	// 如果有起始键，定位到起始键
	if len(it.startKey) > 0 {
		it.iter.Seek(it.startKey)
	} else if len(it.prefix) > 0 {
		it.iter.Seek(it.prefix)
	} else {
		it.iter.Rewind()
	}

	return it.checkValid()
}

// Next 移动到下一个键值对
func (it *Iterator) Next() bool {
	if it.closed.Load() {
		return false
	}

	if !it.started {
		return it.First()
	}

	it.iter.Next()
	return it.checkValid()
}

// checkValid 检查当前位置是否有效
func (it *Iterator) checkValid() bool {
	if !it.iter.Valid() {
		return false
	}

	key := it.iter.Item().Key()

	// 检查前缀
	if len(it.prefix) > 0 && !bytes.HasPrefix(key, it.prefix) {
		return false
	}

	// 检查结束键
	if len(it.endKey) > 0 && bytes.Compare(key, it.endKey) >= 0 {
		return false
	}

	return true
}

// Valid 检查迭代器是否指向有效位置
func (it *Iterator) Valid() bool {
	if it.closed.Load() {
		return false
	}

	return it.iter.Valid() && it.checkValid()
}

// Key 返回当前键
func (it *Iterator) Key() []byte {
	if it.closed.Load() || !it.iter.Valid() {
		return nil
	}

	// 返回键的副本
	return copyBytes(it.iter.Item().Key())
}

// Value 返回当前值
func (it *Iterator) Value() []byte {
	if it.closed.Load() || !it.iter.Valid() {
		return nil
	}

	value, err := it.iter.Item().ValueCopy(nil)
	if err != nil {
		it.err = err
		return nil
	}

	return value
}

// Close 关闭迭代器
func (it *Iterator) Close() {
	if it.closed.Swap(true) {
		return
	}

	it.iter.Close()
	it.txn.Discard()
}

// Error 返回迭代过程中的错误
func (it *Iterator) Error() error {
	return it.err
}

// Seek 定位到指定键
func (it *Iterator) Seek(key []byte) bool {
	if it.closed.Load() {
		return false
	}

	it.started = true
	it.iter.Seek(key)
	return it.checkValid()
}

// copyBytes 复制字节切片
func copyBytes(src []byte) []byte {
	if src == nil {
		return nil
	}
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}

// 编译时检查接口实现
var _ engine.Iterator = (*Iterator)(nil)
