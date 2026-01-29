// Package badger 提供基于 BadgerDB 的存储引擎实现
//
// BadgerDB 是一个高性能的嵌入式键值存储引擎，支持 ACID 事务、
// MVCC、压缩、垃圾回收等特性。
//
// # 特性
//
//   - 高性能：基于 LSM 树，优化写入性能
//   - ACID 事务：支持完整的事务语义
//   - MVCC：支持多版本并发控制
//   - 压缩：支持 ZSTD 压缩，减少磁盘占用
//   - 垃圾回收：自动回收已删除数据的空间
//
// # 使用示例
//
//	cfg := engine.DefaultConfig("/data/storage")
//	db, err := badger.New(cfg)
//	if err != nil {
//	    return err
//	}
//	defer db.Close()
//
//	// 写入
//	if err := db.Put([]byte("key"), []byte("value")); err != nil {
//	    return err
//	}
//
//	// 读取
//	value, err := db.Get([]byte("key"))
package badger

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dgraph-io/badger/v4"
)

// logger 是 badger 存储引擎的日志记录器
var logger = log.Logger("storage/badger")

// Engine BadgerDB 存储引擎
type Engine struct {
	db     *badger.DB
	config *engine.Config
	closed atomic.Bool
	mu     sync.RWMutex

	// 统计信息
	stats struct {
		numReads    atomic.Int64
		numWrites   atomic.Int64
		numDeletes  atomic.Int64
		cacheHits   atomic.Int64
		cacheMisses atomic.Int64
	}

	// 后台任务
	gcCtx    context.Context
	gcCancel context.CancelFunc
	gcWg     sync.WaitGroup
}

// New 创建新的 BadgerDB 存储引擎
func New(cfg *engine.Config) (*Engine, error) {
	if cfg == nil {
		return nil, engine.ErrInvalidConfig
	}

	// 验证配置
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// 确保目录存在
	if err := cfg.EnsureDir(); err != nil {
		return nil, err
	}

	// 构建 BadgerDB 选项
	opts := buildBadgerOptions(cfg)

	// 打开数据库
	db, err := badger.Open(opts)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	e := &Engine{
		db:       db,
		config:   cfg,
		gcCtx:    ctx,
		gcCancel: cancel,
	}

	return e, nil
}

// buildBadgerOptions 根据配置构建 BadgerDB 选项
func buildBadgerOptions(cfg *engine.Config) badger.Options {
	// 始终使用持久化模式
	opts := badger.DefaultOptions(cfg.Path)

	// 基础选项
	opts = opts.
		WithSyncWrites(cfg.SyncWrites).
		WithNumVersionsToKeep(cfg.NumVersionsToKeep).
		WithReadOnly(cfg.ReadOnly)

	// BadgerDB 特定选项
	b := cfg.Badger
	opts = opts.
		WithMemTableSize(b.MemTableSize).
		WithValueLogFileSize(b.ValueLogFileSize).
		WithNumMemtables(b.NumMemtables).
		WithNumLevelZeroTables(b.NumLevelZeroTables).
		WithNumLevelZeroTablesStall(b.NumLevelZeroTablesStall).
		WithValueLogMaxEntries(b.ValueLogMaxEntries).
		WithValueThreshold(b.ValueThreshold).
		WithBlockCacheSize(b.BlockCacheSize).
		WithIndexCacheSize(b.IndexCacheSize).
		WithNumCompactors(b.NumCompactors).
		WithCompactL0OnClose(b.CompactL0OnClose).
		WithZSTDCompressionLevel(b.ZSTDCompressionLevel)

	// 日志设置
	if cfg.Logger != nil {
		opts = opts.WithLogger(&badgerLogger{cfg.Logger})
	} else {
		opts = opts.WithLogger(nil)
	}

	return opts
}

// badgerLogger 适配器：将 engine.Logger 适配到 badger.Logger
type badgerLogger struct {
	logger engine.Logger
}

func (l *badgerLogger) Errorf(format string, args ...interface{}) {
	l.logger.Errorf(format, args...)
}

func (l *badgerLogger) Warningf(format string, args ...interface{}) {
	l.logger.Warningf(format, args...)
}

func (l *badgerLogger) Infof(format string, args ...interface{}) {
	l.logger.Infof(format, args...)
}

func (l *badgerLogger) Debugf(format string, args ...interface{}) {
	l.logger.Debugf(format, args...)
}

// Start 启动存储引擎
func (e *Engine) Start() error {
	if e.closed.Load() {
		return engine.ErrClosed
	}

	// 启动垃圾回收
	if e.config.Badger.GCInterval > 0 {
		e.startGC()
	}

	return nil
}

// startGC 启动垃圾回收后台任务
func (e *Engine) startGC() {
	e.gcWg.Add(1)
	go func() {
		defer e.gcWg.Done()

		ticker := time.NewTicker(e.config.Badger.GCInterval)
		defer ticker.Stop()

		for {
			select {
			case <-e.gcCtx.Done():
				return
			case <-ticker.C:
				e.runGC()
			}
		}
	}()
}

// runGC 执行一次垃圾回收
func (e *Engine) runGC() {
	if e.closed.Load() {
		return
	}

	// 运行 GC 直到返回 nil（没有更多可回收的空间）
	for {
		err := e.db.RunValueLogGC(e.config.Badger.GCDiscardRatio)
		if err != nil {
			break
		}
	}
}

// --- 公共接口实现 (interfaces.Engine) ---

// Get 获取指定键的值
func (e *Engine) Get(key []byte) ([]byte, error) {
	if e.closed.Load() {
		return nil, engine.ErrClosed
	}

	if len(key) == 0 {
		return nil, engine.ErrEmptyKey
	}

	var value []byte
	err := e.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			if err == badger.ErrKeyNotFound {
				e.stats.cacheMisses.Add(1)
				return engine.ErrNotFound
			}
			return err
		}

		e.stats.cacheHits.Add(1)

		value, err = item.ValueCopy(nil)
		return err
	})

	e.stats.numReads.Add(1)

	if err != nil {
		return nil, err
	}

	return value, nil
}

// Put 设置键值对
func (e *Engine) Put(key, value []byte) error {
	if e.closed.Load() {
		return engine.ErrClosed
	}

	if e.config.ReadOnly {
		return engine.ErrReadOnly
	}

	if len(key) == 0 {
		return engine.ErrEmptyKey
	}

	err := e.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, value)
	})

	if err == nil {
		e.stats.numWrites.Add(1)
	}

	return convertError(err)
}

// Delete 删除指定键
func (e *Engine) Delete(key []byte) error {
	if e.closed.Load() {
		return engine.ErrClosed
	}

	if e.config.ReadOnly {
		return engine.ErrReadOnly
	}

	if len(key) == 0 {
		return engine.ErrEmptyKey
	}

	err := e.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})

	if err == nil {
		e.stats.numDeletes.Add(1)
	}

	return convertError(err)
}

// Has 检查键是否存在
func (e *Engine) Has(key []byte) (bool, error) {
	if e.closed.Load() {
		return false, engine.ErrClosed
	}

	if len(key) == 0 {
		return false, engine.ErrEmptyKey
	}

	var exists bool
	err := e.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		if err == nil {
			exists = true
			return nil
		}
		if err == badger.ErrKeyNotFound {
			exists = false
			return nil
		}
		return err
	})

	return exists, err
}

// Close 关闭存储引擎
func (e *Engine) Close() error {
	if e.closed.Swap(true) {
		return nil // 已经关闭
	}

	// 停止 GC
	e.gcCancel()
	e.gcWg.Wait()

	// 关闭数据库
	return e.db.Close()
}

// --- 内部扩展接口实现 (engine.InternalEngine) ---

// NewBatch 创建新的批量写入对象
func (e *Engine) NewBatch() engine.Batch {
	return &WriteBatch{
		db:    e,
		batch: e.db.NewWriteBatch(),
	}
}

// Write 执行批量写入
func (e *Engine) Write(batch engine.Batch) error {
	if e.closed.Load() {
		return engine.ErrClosed
	}

	if e.config.ReadOnly {
		return engine.ErrReadOnly
	}

	wb, ok := batch.(*WriteBatch)
	if !ok {
		return engine.ErrInvalidConfig
	}

	return wb.batch.Flush()
}

// NewIterator 创建新的迭代器
func (e *Engine) NewIterator(opts *engine.IteratorOptions) engine.Iterator {
	if opts == nil {
		opts = engine.DefaultIteratorOptions()
	}

	txn := e.db.NewTransaction(false)

	badgerOpts := badger.DefaultIteratorOptions
	badgerOpts.Reverse = opts.Reverse
	badgerOpts.PrefetchSize = opts.PrefetchSize
	badgerOpts.PrefetchValues = opts.PrefetchValues

	if len(opts.Prefix) > 0 {
		badgerOpts.Prefix = opts.Prefix
	}

	return &Iterator{
		txn:      txn,
		iter:     txn.NewIterator(badgerOpts),
		prefix:   opts.Prefix,
		startKey: opts.StartKey,
		endKey:   opts.EndKey,
	}
}

// NewPrefixIterator 创建前缀迭代器
func (e *Engine) NewPrefixIterator(prefix []byte) engine.Iterator {
	return e.NewIterator(&engine.IteratorOptions{
		Prefix:         prefix,
		PrefetchSize:   100,
		PrefetchValues: true,
	})
}

// NewTransaction 创建新的事务
func (e *Engine) NewTransaction(writable bool) engine.Transaction {
	return &Transaction{
		txn:      e.db.NewTransaction(writable),
		writable: writable,
	}
}

// Compact 压缩存储
func (e *Engine) Compact() error {
	if e.closed.Load() {
		return engine.ErrClosed
	}

	return e.db.Flatten(4) // 4 个并发 worker
}

// Sync 同步数据到磁盘
func (e *Engine) Sync() error {
	if e.closed.Load() {
		return engine.ErrClosed
	}

	return e.db.Sync()
}

// Stats 获取引擎统计信息
func (e *Engine) Stats() *engine.Stats {
	lsm, vlog := e.db.Size()

	// 计算表和层数
	tables := e.db.Tables()
	levels := e.db.Levels()

	return &engine.Stats{
		KeyCount:        e.estimateKeyCount(),
		DiskSize:        lsm + vlog,
		CacheHits:       e.stats.cacheHits.Load(),
		CacheMisses:     e.stats.cacheMisses.Load(),
		LSMSize:         lsm,
		VlogSize:        vlog,
		NumTables:       len(tables),
		NumLevels:       len(levels),
		NumWrites:       e.stats.numWrites.Load(),
		NumReads:        e.stats.numReads.Load(),
		NumDeletes:      e.stats.numDeletes.Load(),
		NumBytesRead:    0, // BadgerDB 不直接提供
		NumBytesWritten: 0, // BadgerDB 不直接提供
	}
}

// estimateKeyCount 估算键数量
func (e *Engine) estimateKeyCount() int64 {
	var count int64
	err := e.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			count++
			if count >= 10000 {
				// 采样估算
				break
			}
		}
		return nil
	})
	if err != nil {
		// 估算失败时返回 0，不影响功能
		logger.Debug("估算键数量失败", "error", err)
		return 0
	}

	return count
}

// DB 返回底层 BadgerDB 实例（仅供内部使用）
func (e *Engine) DB() *badger.DB {
	return e.db
}

// convertError 转换 BadgerDB 错误到引擎错误
func convertError(err error) error {
	if err == nil {
		return nil
	}

	switch err {
	case badger.ErrKeyNotFound:
		return engine.ErrNotFound
	case badger.ErrEmptyKey:
		return engine.ErrEmptyKey
	case badger.ErrTxnTooBig:
		return engine.ErrTransactionTooLarge
	case badger.ErrConflict:
		return engine.ErrTransactionConflict
	case badger.ErrDiscardedTxn:
		return engine.ErrTransactionDiscarded
	case badger.ErrReadOnlyTxn:
		return engine.ErrReadOnly
	default:
		return err
	}
}

// 编译时检查接口实现
var _ engine.InternalEngine = (*Engine)(nil)
