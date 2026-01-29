// Package badger 实现 BadgerDB 存储引擎
//
// badger 使用 BadgerDB 作为底层存储，提供高性能的键值存储。
//
// # 特性
//
//   - LSM-tree 存储引擎
//   - 支持事务
//   - 自动 GC
//   - 压缩支持
//
// # 配置
//
//	config := badger.Config{
//	    Path:       "/path/to/data",
//	    InMemory:   false,
//	    SyncWrites: true,
//	}
//
// # 使用示例
//
//	engine, err := badger.NewEngine(config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer engine.Close()
//
//	err = engine.Put([]byte("key"), []byte("value"))
//	value, err := engine.Get([]byte("key"))
package badger
