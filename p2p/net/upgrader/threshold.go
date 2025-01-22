package upgrader

import (
	"sync"
)

// newThreshold 创建一个新的阈值控制器
// 参数:
//   - cutoff: int 阈值大小
//
// 返回值:
//   - *threshold 阈值控制器实例
func newThreshold(cutoff int) *threshold {
	// 创建阈值控制器实例并设置阈值
	t := &threshold{
		threshold: cutoff,
	}
	// 初始化条件变量的锁
	t.cond.L = &t.mu
	return t
}

// threshold 阈值控制器结构体
type threshold struct {
	mu   sync.Mutex // 互斥锁
	cond sync.Cond  // 条件变量

	count     int // 当前计数
	threshold int // 阈值
}

// Acquire 增加计数器，该操作不会阻塞
// 注意:
//   - 该方法是线程安全的
func (t *threshold) Acquire() {
	// 加锁保护共享资源
	t.mu.Lock()
	// 计数加1
	t.count++
	// 释放锁
	t.mu.Unlock()
}

// Release 减少计数器
// 注意:
//   - 当计数为0时会触发panic
//   - 当计数等于阈值时会广播通知等待的goroutine
func (t *threshold) Release() {
	// 加锁保护共享资源
	t.mu.Lock()
	// 检查计数是否为0
	if t.count == 0 {
		panic("计数不能为负数")
	}
	// 如果当前计数等于阈值，广播通知等待的goroutine
	if t.threshold == t.count {
		t.cond.Broadcast()
	}
	// 计数减1
	t.count--
	// 释放锁
	t.mu.Unlock()
}

// Wait 等待计数降低到阈值以下
// 注意:
//   - 该方法会阻塞直到条件满足
func (t *threshold) Wait() {
	// 加锁保护共享资源
	t.mu.Lock()
	// 当计数大于等于阈值时循环等待
	for t.count >= t.threshold {
		t.cond.Wait()
	}
	// 释放锁
	t.mu.Unlock()
}
