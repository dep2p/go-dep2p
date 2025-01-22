package swarm

import "time"

// InstantTimer 定义一个在特定时刻触发的定时器接口,而不是基于持续时间
type InstantTimer interface {
	// Reset 重置定时器到指定时刻
	// 参数:
	//   - d: time.Time 目标时刻
	// 返回值:
	//   - bool 是否成功重置
	Reset(d time.Time) bool

	// Stop 停止定时器
	// 返回值:
	//   - bool 是否成功停止
	Stop() bool

	// Ch 返回定时器的通道
	// 返回值:
	//   - <-chan time.Time 定时器通道
	Ch() <-chan time.Time
}

// Clock 定义一个可以创建基于时刻触发定时器的时钟接口
type Clock interface {
	// Now 返回当前时间
	// 返回值:
	//   - time.Time 当前时间
	Now() time.Time

	// Since 返回从指定时刻到现在的时间间隔
	// 参数:
	//   - t: time.Time 起始时刻
	// 返回值:
	//   - time.Duration 时间间隔
	Since(t time.Time) time.Duration

	// InstantTimer 创建一个在指定时刻触发的定时器
	// 参数:
	//   - when: time.Time 触发时刻
	// 返回值:
	//   - InstantTimer 定时器接口
	InstantTimer(when time.Time) InstantTimer
}

// RealTimer 实现了 InstantTimer 接口的真实定时器
type RealTimer struct {
	t *time.Timer // 内部定时器
}

// 确保 RealTimer 实现了 InstantTimer 接口
var _ InstantTimer = (*RealTimer)(nil)

// Ch 返回定时器的通道
// 返回值:
//   - <-chan time.Time 定时器通道
func (t RealTimer) Ch() <-chan time.Time {
	return t.t.C
}

// Reset 重置定时器到指定时刻
// 参数:
//   - d: time.Time 目标时刻
//
// 返回值:
//   - bool 是否成功重置
func (t RealTimer) Reset(d time.Time) bool {
	return t.t.Reset(time.Until(d))
}

// Stop 停止定时器
// 返回值:
//   - bool 是否成功停止
func (t RealTimer) Stop() bool {
	return t.t.Stop()
}

// RealClock 实现了 Clock 接口的真实时钟
type RealClock struct{}

// 确保 RealClock 实现了 Clock 接口
var _ Clock = RealClock{}

// Now 返回当前时间
// 返回值:
//   - time.Time 当前时间
func (RealClock) Now() time.Time {
	return time.Now()
}

// Since 返回从指定时刻到现在的时间间隔
// 参数:
//   - t: time.Time 起始时刻
//
// 返回值:
//   - time.Duration 时间间隔
func (RealClock) Since(t time.Time) time.Duration {
	return time.Since(t)
}

// InstantTimer 创建一个在指定时刻触发的定时器
// 参数:
//   - when: time.Time 触发时刻
//
// 返回值:
//   - InstantTimer 定时器接口
func (RealClock) InstantTimer(when time.Time) InstantTimer {
	t := time.NewTimer(time.Until(when))
	return &RealTimer{t}
}
