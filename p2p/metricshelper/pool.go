package metricshelper

import (
	"fmt"
	"sync"
)

// 字符串切片池的容量大小
const capacity = 8

// 字符串切片对象池,用于复用字符串切片
var stringPool = sync.Pool{New: func() any {
	// 创建一个容量为capacity的空字符串切片
	s := make([]string, 0, capacity)
	return &s
}}

// GetStringSlice 从对象池获取一个字符串切片
// 返回值:
//   - *[]string: 字符串切片指针
func GetStringSlice() *[]string {
	// 从对象池获取字符串切片并转换类型
	s := stringPool.Get().(*[]string)
	// 重置切片长度为0,保留容量
	*s = (*s)[:0]
	// 返回切片指针
	return s
}

// PutStringSlice 将字符串切片放回对象池
// 参数:
//   - s: *[]string - 要放回的字符串切片指针
func PutStringSlice(s *[]string) {
	// 检查切片容量是否满足要求
	if c := cap(*s); c < capacity {
		// 如果容量小于预期,抛出错误
		panic(fmt.Sprintf("预期字符串切片容量不小于8,实际获得 %d", c))
	}
	// 将切片放回对象池
	stringPool.Put(s)
}
