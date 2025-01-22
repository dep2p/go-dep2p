package backoff

import (
	"math"
	"math/rand"
	"sync"
	"time"

	logging "github.com/dep2p/log"
)

// log 用于记录discovery-backoff相关的日志
var log = logging.Logger("discovery-backoff")

// BackoffFactory 是一个函数类型，用于创建BackoffStrategy实例
// 返回值:
//   - BackoffStrategy: 返回一个新的退避策略实例
type BackoffFactory func() BackoffStrategy

// BackoffStrategy 描述了如何实现退避策略的接口。BackoffStrategy是有状态的。
type BackoffStrategy interface {
	// Delay 根据之前的调用计算下一次退避的持续时间
	// 返回值:
	//   - time.Duration: 返回下一次退避的时间间隔
	Delay() time.Duration
	// Reset 清除BackoffStrategy的内部状态
	Reset()
}

// Jitter 实现参考自 https://aws.amazon.com/blogs/architecture/exponential-backoff-and-jitter/

// Jitter 是一个函数类型，必须返回一个介于min和max之间的持续时间。min必须小于或等于max。
// 参数:
//   - duration: 原始持续时间
//   - min: 最小持续时间
//   - max: 最大持续时间
//   - rng: 随机数生成器
//
// 返回值:
//   - time.Duration: 返回添加抖动后的持续时间
type Jitter func(duration, min, max time.Duration, rng *rand.Rand) time.Duration

// FullJitter 从[min, boundedDur]范围内均匀随机选择一个数字返回。
// boundedDur是在min和max之间的持续时间。
// 参数:
//   - duration: 原始持续时间
//   - min: 最小持续时间
//   - max: 最大持续时间
//   - rng: 随机数生成器
//
// 返回值:
//   - time.Duration: 返回添加完全抖动后的持续时间
func FullJitter(duration, min, max time.Duration, rng *rand.Rand) time.Duration {
	// 如果持续时间小于等于最小值，直接返回最小值
	if duration <= min {
		return min
	}

	// 计算归一化后的持续时间
	normalizedDur := boundedDuration(duration, min, max) - min

	// 在归一化范围内生成随机值并加上最小值
	return boundedDuration(time.Duration(rng.Int63n(int64(normalizedDur)))+min, min, max)
}

// NoJitter 返回在min和max之间的持续时间
// 参数:
//   - duration: 原始持续时间
//   - min: 最小持续时间
//   - max: 最大持续时间
//   - rng: 随机数生成器
//
// 返回值:
//   - time.Duration: 返回不添加抖动的持续时间
func NoJitter(duration, min, max time.Duration, rng *rand.Rand) time.Duration {
	return boundedDuration(duration, min, max)
}

// randomizedBackoff 实现了基本的随机退避功能
type randomizedBackoff struct {
	min time.Duration // 最小退避时间
	max time.Duration // 最大退避时间
	rng *rand.Rand    // 随机数生成器
}

// BoundedDelay 返回一个在最小值和最大值之间的持续时间
// 参数:
//   - duration: 原始持续时间
//
// 返回值:
//   - time.Duration: 返回限定在最小最大值之间的持续时间
func (b *randomizedBackoff) BoundedDelay(duration time.Duration) time.Duration {
	return boundedDuration(duration, b.min, b.max)
}

// boundedDuration 确保持续时间在min和max之间
// 参数:
//   - d: 原始持续时间
//   - min: 最小持续时间
//   - max: 最大持续时间
//
// 返回值:
//   - time.Duration: 返回限定在最小最大值之间的持续时间
func boundedDuration(d, min, max time.Duration) time.Duration {
	if d < min {
		return min
	}
	if d > max {
		return max
	}
	return d
}

// attemptBackoff 实现了基于尝试次数的退避策略
type attemptBackoff struct {
	attempt           int    // 当前尝试次数
	jitter            Jitter // 抖动函数
	randomizedBackoff        // 嵌入随机退避功能
}

// Reset 重置尝试次数为0
func (b *attemptBackoff) Reset() {
	b.attempt = 0
}

// NewFixedBackoff 创建一个具有固定退避时间的BackoffFactory
// 参数:
//   - delay: 固定的退避时间
//
// 返回值:
//   - BackoffFactory: 返回创建固定退避策略的工厂函数
func NewFixedBackoff(delay time.Duration) BackoffFactory {
	return func() BackoffStrategy {
		return &fixedBackoff{delay: delay}
	}
}

// fixedBackoff 实现了固定时间的退避策略
type fixedBackoff struct {
	delay time.Duration // 固定的退避时间
}

// Delay 返回固定的退避时间
// 返回值:
//   - time.Duration: 返回固定的退避时间
func (b *fixedBackoff) Delay() time.Duration {
	return b.delay
}

// Reset 对于固定退避策略无需重置
func (b *fixedBackoff) Reset() {}

// NewPolynomialBackoff 创建一个多项式退避策略的BackoffFactory
// 退避时间计算公式: c0*x^0 + c1*x^1 + ... + cn*x^n，其中x是尝试次数
// 参数:
//   - min: 最小退避时间
//   - max: 最大退避时间
//   - jitter: 添加随机性的函数
//   - timeUnits: 多项式计算的时间单位
//   - polyCoefs: 多项式系数数组[c0, c1, ..., cn]
//   - rngSrc: 随机数源
//
// 返回值:
//   - BackoffFactory: 返回创建多项式退避策略的工厂函数
func NewPolynomialBackoff(min, max time.Duration, jitter Jitter,
	timeUnits time.Duration, polyCoefs []float64, rngSrc rand.Source) BackoffFactory {
	// 创建线程安全的随机数生成器
	rng := rand.New(&lockedSource{src: rngSrc})
	return func() BackoffStrategy {
		return &polynomialBackoff{
			attemptBackoff: attemptBackoff{
				randomizedBackoff: randomizedBackoff{
					min: min,
					max: max,
					rng: rng,
				},
				jitter: jitter,
			},
			timeUnits: timeUnits,
			poly:      polyCoefs,
		}
	}
}

// polynomialBackoff 实现了多项式退避策略
type polynomialBackoff struct {
	attemptBackoff               // 嵌入基本退避功能
	timeUnits      time.Duration // 时间单位
	poly           []float64     // 多项式系数
}

// Delay 计算并返回下一次退避时间
// 返回值:
//   - time.Duration: 返回计算得到的下一次退避时间
func (b *polynomialBackoff) Delay() time.Duration {
	var polySum float64
	switch len(b.poly) {
	case 0: // 如果没有系数，返回0
		return 0
	case 1: // 如果只有一个系数，直接使用该系数
		polySum = b.poly[0]
	default: // 计算多项式和
		polySum = b.poly[0]
		exp := 1
		attempt := b.attempt
		b.attempt++

		for _, c := range b.poly[1:] {
			exp *= attempt
			polySum += float64(exp) * c
		}
	}
	return b.jitter(time.Duration(float64(b.timeUnits)*polySum), b.min, b.max, b.rng)
}

// NewExponentialBackoff 创建一个指数退避策略的BackoffFactory
// 退避时间计算公式: base^x + offset，其中x是尝试次数
// 参数:
//   - min: 最小退避时间
//   - max: 最大退避时间
//   - jitter: 添加随机性的函数
//   - timeUnits: 指数计算的时间单位
//   - base: 指数的底数
//   - offset: 固定偏移时间
//   - rngSrc: 随机数源
//
// 返回值:
//   - BackoffFactory: 返回创建指数退避策略的工厂函数
func NewExponentialBackoff(min, max time.Duration, jitter Jitter,
	timeUnits time.Duration, base float64, offset time.Duration, rngSrc rand.Source) BackoffFactory {
	// 创建线程安全的随机数生成器
	rng := rand.New(&lockedSource{src: rngSrc})
	return func() BackoffStrategy {
		return &exponentialBackoff{
			attemptBackoff: attemptBackoff{
				randomizedBackoff: randomizedBackoff{
					min: min,
					max: max,
					rng: rng,
				},
				jitter: jitter,
			},
			timeUnits: timeUnits,
			base:      base,
			offset:    offset,
		}
	}
}

// exponentialBackoff 实现了指数退避策略
type exponentialBackoff struct {
	attemptBackoff               // 嵌入基本退避功能
	timeUnits      time.Duration // 时间单位
	base           float64       // 指数的底数
	offset         time.Duration // 固定偏移时间
}

// Delay 计算并返回下一次退避时间
// 返回值:
//   - time.Duration: 返回计算得到的下一次退避时间
func (b *exponentialBackoff) Delay() time.Duration {
	attempt := b.attempt
	b.attempt++
	return b.jitter(
		time.Duration(math.Pow(b.base, float64(attempt))*float64(b.timeUnits))+b.offset, b.min, b.max, b.rng)
}

// NewExponentialDecorrelatedJitter 创建一个去相关指数抖动退避策略的BackoffFactory
// 退避从最小持续时间开始，每次尝试后delay = rand(min, delay * base)，受max限制
// 参数:
//   - min: 最小退避时间
//   - max: 最大退避时间
//   - base: 指数的底数
//   - rngSrc: 随机数源
//
// 返回值:
//   - BackoffFactory: 返回创建去相关指数抖动退避策略的工厂函数
func NewExponentialDecorrelatedJitter(min, max time.Duration, base float64, rngSrc rand.Source) BackoffFactory {
	// 创建线程安全的随机数生成器
	rng := rand.New(&lockedSource{src: rngSrc})
	return func() BackoffStrategy {
		return &exponentialDecorrelatedJitter{
			randomizedBackoff: randomizedBackoff{
				min: min,
				max: max,
				rng: rng,
			},
			base: base,
		}
	}
}

// exponentialDecorrelatedJitter 实现了去相关指数抖动退避策略
type exponentialDecorrelatedJitter struct {
	randomizedBackoff               // 嵌入随机退避功能
	base              float64       // 指数的底数
	lastDelay         time.Duration // 上一次的退避时间
}

// Delay 计算并返回下一次退避时间
// 返回值:
//   - time.Duration: 返回计算得到的下一次退避时间
func (b *exponentialDecorrelatedJitter) Delay() time.Duration {
	// 如果上次延迟小于最小值，返回最小值
	if b.lastDelay < b.min {
		b.lastDelay = b.min
		return b.lastDelay
	}

	// 计算下一个最大值
	nextMax := int64(float64(b.lastDelay) * b.base)
	// 在新范围内生成随机延迟时间
	b.lastDelay = boundedDuration(time.Duration(b.rng.Int63n(nextMax-int64(b.min)))+b.min, b.min, b.max)
	return b.lastDelay
}

// Reset 重置上一次的退避时间为0
func (b *exponentialDecorrelatedJitter) Reset() { b.lastDelay = 0 }

// lockedSource 实现了线程安全的随机数源
type lockedSource struct {
	lk  sync.Mutex  // 互斥锁
	src rand.Source // 随机数源
}

// Int63 返回一个线程安全的63位整数随机数
// 返回值:
//   - int64: 返回生成的随机数
func (r *lockedSource) Int63() (n int64) {
	r.lk.Lock()
	n = r.src.Int63()
	r.lk.Unlock()
	return
}

// Seed 以线程安全的方式设置随机数种子
// 参数:
//   - seed: 随机数种子
func (r *lockedSource) Seed(seed int64) {
	r.lk.Lock()
	r.src.Seed(seed)
	r.lk.Unlock()
}
