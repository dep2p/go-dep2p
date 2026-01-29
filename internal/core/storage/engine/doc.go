// Package engine 定义存储引擎接口
//
// engine 提供存储引擎的抽象接口，允许使用不同的底层存储实现。
//
// # 接口
//
//   - Engine: 存储引擎主接口
//   - Txn: 事务接口
//   - Iterator: 迭代器接口
//
// # 实现
//
//   - badger: BadgerDB 实现（默认）
//
// # 使用示例
//
//	engine, err := badger.NewEngine(config)
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer engine.Close()
package engine
