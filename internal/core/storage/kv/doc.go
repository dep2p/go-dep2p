// Package kv 提供简化的键值存储接口
//
// kv 在 Engine 基础上提供更简单的键值操作接口，
// 适合不需要完整事务功能的场景。
//
// # 接口
//
//   - Store: 简化的 KV 存储接口
//   - Get/Put/Delete/Has
//
// # 使用示例
//
//	store := kv.NewStore(engine)
//	store.Put("key", []byte("value"))
//	value, err := store.Get("key")
//	exists := store.Has("key")
//	store.Delete("key")
package kv
