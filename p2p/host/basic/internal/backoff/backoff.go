package backoff

import (
	"time"
)

// since 是 time.Since 的别名,用于获取时间间隔
var since = time.Since

// 默认延迟时间为100毫秒
const defaultDelay = 100 * time.Millisecond

// 默认最大延迟时间为1分钟
const defaultMaxDelay = 1 * time.Minute

// ExpBackoff 指数退避算法结构体
type ExpBackoff struct {
	Delay    time.Duration // 延迟时间
	MaxDelay time.Duration // 最大延迟时间

	failures int       // 失败次数
	lastRun  time.Time // 上次运行时间
}

// init 初始化ExpBackoff结构体
// 如果未设置延迟时间和最大延迟时间,则使用默认值
func (b *ExpBackoff) init() {
	if b.Delay == 0 {
		b.Delay = defaultDelay
	}
	if b.MaxDelay == 0 {
		b.MaxDelay = defaultMaxDelay
	}
}

// calcDelay 计算当前应该延迟的时间
// 返回值: time.Duration 计算得到的延迟时间
func (b *ExpBackoff) calcDelay() time.Duration {
	delay := b.Delay * time.Duration(1<<(b.failures-1)) // 根据失败次数计算延迟时间
	delay = min(delay, b.MaxDelay)                      // 确保延迟时间不超过最大延迟时间
	return delay
}

// Run 执行给定的函数,并根据执行结果更新退避状态
// 参数:
//   - f func() error: 需要执行的函数
//
// 返回值:
//   - err error: 执行函数返回的错误
//   - ran bool: 是否实际执行了函数
func (b *ExpBackoff) Run(f func() error) (err error, ran bool) {
	b.init() // 初始化参数

	if b.failures != 0 { // 如果之前有失败
		if since(b.lastRun) < b.calcDelay() { // 检查是否达到延迟时间
			return nil, false // 未达到延迟时间,不执行
		}
	}

	b.lastRun = time.Now() // 更新最后执行时间
	err = f()              // 执行函数
	if err == nil {        // 执行成功
		b.failures = 0 // 重置失败次数
	} else { // 执行失败
		b.failures++ // 增加失败次数
	}
	return err, true // 返回执行结果
}
