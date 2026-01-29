// Package interfaces - Storage 存储引擎接口
//
// 本文件定义 DeP2P 存储引擎的公共接口，属于 Core Layer。
// 允许用户提供自定义存储后端（可选）。
//
// # 设计原则
//
// 1. 最小化接口：仅暴露必要的基础操作
// 2. 可替换性：用户可以实现自定义存储后端
// 3. 无状态方法：所有方法都是幂等的
package interfaces

// Engine 存储引擎基础接口
//
// 提供键值存储的基本操作。DeP2P 内部使用 BadgerDB 实现，
// 但用户可以提供自定义实现来替换默认存储后端。
//
// 线程安全：实现必须保证所有方法的线程安全性。
//
// 示例:
//
//	engine, err := badger.New(badger.DefaultConfig("/data/storage"))
//	if err != nil {
//	    return err
//	}
//	defer engine.Close()
//
//	// 写入
//	if err := engine.Put([]byte("key"), []byte("value")); err != nil {
//	    return err
//	}
//
//	// 读取
//	value, err := engine.Get([]byte("key"))
//	if err != nil {
//	    return err
//	}
type Engine interface {
	// Get 获取指定键的值
	//
	// 参数:
	//   - key: 键（不能为空）
	//
	// 返回:
	//   - []byte: 值的副本（调用者可以安全修改）
	//   - error: ErrNotFound 如果键不存在，其他错误表示存储故障
	Get(key []byte) ([]byte, error)

	// Put 设置键值对
	//
	// 如果键已存在，则覆盖旧值。
	//
	// 参数:
	//   - key: 键（不能为空）
	//   - value: 值（可以为空，表示存储空值）
	//
	// 返回:
	//   - error: ErrKeyTooLarge/ErrValueTooLarge 如果超过大小限制
	Put(key, value []byte) error

	// Delete 删除指定键
	//
	// 如果键不存在，不返回错误（幂等操作）。
	//
	// 参数:
	//   - key: 键（不能为空）
	//
	// 返回:
	//   - error: 存储故障时返回错误
	Delete(key []byte) error

	// Has 检查键是否存在
	//
	// 参数:
	//   - key: 键（不能为空）
	//
	// 返回:
	//   - bool: 键是否存在
	//   - error: 存储故障时返回错误
	Has(key []byte) (bool, error)

	// Close 关闭存储引擎
	//
	// 关闭后不能再进行任何操作。
	// 多次调用 Close 是安全的。
	//
	// 返回:
	//   - error: 关闭过程中的错误
	Close() error
}

// EngineStats 引擎统计信息
//
// 提供存储引擎的运行时统计数据，用于监控和调优。
type EngineStats struct {
	// KeyCount 当前存储的键数量
	KeyCount int64 `json:"key_count"`

	// DiskSize 磁盘占用大小（字节）
	DiskSize int64 `json:"disk_size"`

	// CacheHits 缓存命中次数
	CacheHits int64 `json:"cache_hits"`

	// CacheMisses 缓存未命中次数
	CacheMisses int64 `json:"cache_misses"`
}

// CacheHitRate 计算缓存命中率
//
// 返回:
//   - float64: 命中率（0.0 - 1.0），如果没有访问则返回 0
func (s *EngineStats) CacheHitRate() float64 {
	total := s.CacheHits + s.CacheMisses
	if total == 0 {
		return 0
	}
	return float64(s.CacheHits) / float64(total)
}
