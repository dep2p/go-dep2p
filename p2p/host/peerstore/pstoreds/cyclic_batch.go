package pstoreds

import (
	"context"
	"errors"

	ds "github.com/dep2p/go-dep2p/datastore"
)

// 在循环批处理中队列中的操作数达到多少时进行刷新
var defaultOpsPerCyclicBatch = 20

// cyclicBatch 缓冲数据存储写操作,并在队列中累积了 defaultOpsPerCyclicBatch (20)个操作后自动刷新。
// 显式调用 `Commit()` 会关闭这个循环批处理,之后的所有操作都会返回错误。
//
// 它类似于 go-ds 自动批处理,但它由数据存储提供的实际 Batch 功能驱动。
type cyclicBatch struct {
	threshold int         // 触发自动刷新的阈值
	ds.Batch              // 内嵌的批处理接口
	ds        ds.Batching // 数据存储接口
	pending   int         // 当前等待处理的操作数
}

// newCyclicBatch 创建一个新的循环批处理实例
// 参数:
//   - ds: 实现了批处理接口的数据存储
//   - threshold: 自动刷新的阈值
//
// 返回:
//   - ds.Batch: 批处理接口
//   - error: 错误信息
func newCyclicBatch(ds ds.Batching, threshold int) (ds.Batch, error) {
	// 从数据存储创建一个新的批处理
	batch, err := ds.Batch(context.TODO())
	if err != nil {
		log.Debugf("创建批处理失败: %v", err)
		return nil, err
	}
	return &cyclicBatch{Batch: batch, ds: ds}, nil
}

// cycle 检查并在必要时执行批处理循环
// 返回:
//   - error: 错误信息
func (cb *cyclicBatch) cycle() (err error) {
	// 检查批处理是否已关闭
	if cb.Batch == nil {
		log.Debugf("批处理已关闭")
		return errors.New("批处理已关闭")
	}
	// 检查是否达到阈值
	if cb.pending < cb.threshold {
		return nil
	}
	// 提交当前批处理
	if err = cb.Batch.Commit(context.TODO()); err != nil {
		log.Debugf("提交批处理失败: %v", err)
		return err
	}
	// 创建新的批处理
	if cb.Batch, err = cb.ds.Batch(context.TODO()); err != nil {
		log.Debugf("创建新批处理失败: %v", err)
		return err
	}
	return nil
}

// Put 将键值对添加到批处理中
// 参数:
//   - ctx: 上下文
//   - key: 键
//   - val: 值
//
// 返回:
//   - error: 错误信息
func (cb *cyclicBatch) Put(ctx context.Context, key ds.Key, val []byte) error {
	// 检查是否需要执行批处理循环
	if err := cb.cycle(); err != nil {
		log.Debugf("批处理循环失败: %v", err)
		return err
	}
	// 增加待处理操作计数
	cb.pending++
	return cb.Batch.Put(ctx, key, val)
}

// Delete 从批处理中删除指定键的数据
// 参数:
//   - ctx: 上下文
//   - key: 要删除的键
//
// 返回:
//   - error: 错误信息
func (cb *cyclicBatch) Delete(ctx context.Context, key ds.Key) error {
	// 检查是否需要执行批处理循环
	if err := cb.cycle(); err != nil {
		log.Debugf("批处理循环失败: %v", err)
		return err
	}
	// 增加待处理操作计数
	cb.pending++
	return cb.Batch.Delete(ctx, key)
}

// Commit 提交并关闭批处理
// 参数:
//   - ctx: 上下文
//
// 返回:
//   - error: 错误信息
func (cb *cyclicBatch) Commit(ctx context.Context) error {
	// 检查批处理是否已关闭
	if cb.Batch == nil {
		log.Debugf("批处理已关闭")
		return errors.New("批处理已关闭")
	}
	// 提交批处理
	if err := cb.Batch.Commit(ctx); err != nil {
		log.Debugf("提交批处理失败: %v", err)
		return err
	}
	// 重置状态并关闭批处理
	cb.pending = 0
	cb.Batch = nil
	return nil
}
