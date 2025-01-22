package connmgr

import (
	"math"
	"time"
)

// DecayNone 返回一个不执行任何衰减的衰减函数。
// 返回值:
//   - DecayFn: 衰减函数,保持标签值不变
func DecayNone() DecayFn {
	// 返回一个简单的衰减函数,该函数:
	// - 保持原始值不变
	// - 永不删除标签(返回 false)
	return func(value DecayingValue) (_ int, rm bool) {
		return value.Value, false
	}
}

// DecayFixed 返回一个固定衰减函数,每次从当前值中减去指定的数值。
// 当值首次达到0或负数时删除标签。
// 参数:
//   - minuend: 每次衰减要减去的固定值
//
// 返回值:
//   - DecayFn: 执行固定衰减的函数
func DecayFixed(minuend int) DecayFn {
	// 返回一个衰减函数,该函数:
	// - 从当前值减去固定值 minuend
	// - 当结果小于等于0时删除标签
	return func(value DecayingValue) (_ int, rm bool) {
		v := value.Value - minuend
		return v, v <= 0
	}
}

// DecayLinear 返回一个线性衰减函数,对标签值应用一个分数系数。
// 使用 math.Floor 向下取整,当结果为0时删除标签。
// 参数:
//   - coef: 衰减系数(0-1之间的小数)
//
// 返回值:
//   - DecayFn: 执行线性衰减的函数
func DecayLinear(coef float64) DecayFn {
	// 返回一个衰减函数,该函数:
	// - 将当前值乘以系数 coef
	// - 向下取整
	// - 当结果小于等于0时删除标签
	return func(value DecayingValue) (after int, rm bool) {
		v := math.Floor(float64(value.Value) * coef)
		return int(v), v <= 0
	}
}

// DecayExpireWhenInactive 返回一个衰减函数,在指定时间内没有提升时使标签过期。
// 参数:
//   - after: 无活动后过期的时间间隔
//
// 返回值:
//   - DecayFn: 基于不活动时间的衰减函数
func DecayExpireWhenInactive(after time.Duration) DecayFn {
	// 返回一个衰减函数,该函数:
	// - 检查自上次提升以来的时间
	// - 如果超过指定时间则删除标签
	return func(value DecayingValue) (_ int, rm bool) {
		rm = time.Until(value.LastVisit) >= after
		return 0, rm
	}
}

// BumpSumUnbounded 返回一个提升函数,简单地将增量加到当前分数上,不设上限。
// 返回值:
//   - BumpFn: 无界累加的提升函数
func BumpSumUnbounded() BumpFn {
	// 返回一个提升函数,该函数:
	// - 将增量直接加到当前值上
	return func(value DecayingValue, delta int) (after int) {
		return value.Value + delta
	}
}

// BumpSumBounded 返回一个提升函数,在指定范围内累加分数。
// 参数:
//   - min: 允许的最小值
//   - max: 允许的最大值
//
// 返回值:
//   - BumpFn: 有界累加的提升函数
func BumpSumBounded(min, max int) BumpFn {
	// 返回一个提升函数,该函数:
	// - 将增量加到当前值
	// - 确保结果在 [min, max] 范围内
	return func(value DecayingValue, delta int) (after int) {
		v := value.Value + delta
		if v >= max {
			return max
		} else if v <= min {
			return min
		}
		return v
	}
}

// BumpOverwrite 返回一个提升函数,用新值直接覆盖当前值。
// 返回值:
//   - BumpFn: 覆盖式的提升函数
func BumpOverwrite() BumpFn {
	// 返回一个提升函数,该函数:
	// - 忽略当前值
	// - 直接返回新的增量值
	return func(value DecayingValue, delta int) (after int) {
		return delta
	}
}
