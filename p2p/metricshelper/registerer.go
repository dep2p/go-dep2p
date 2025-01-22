package metricshelper

import (
	"errors"

	"github.com/prometheus/client_golang/prometheus"
)

// RegisterCollectors 注册收集器到注册器中
// 将收集器注册到指定的注册器中，忽略重复注册错误，其他错误则触发panic
// 参数:
//   - reg: prometheus.Registerer - 注册器实例
//   - collectors: ...prometheus.Collector - 一个或多个收集器
func RegisterCollectors(reg prometheus.Registerer, collectors ...prometheus.Collector) {
	// 遍历所有收集器
	for _, c := range collectors {
		// 尝试注册当前收集器
		err := reg.Register(c)
		// 检查是否发生错误
		if err != nil {
			// 判断是否为重复注册错误，如果不是则触发panic
			if ok := errors.As(err, &prometheus.AlreadyRegisteredError{}); !ok {
				panic(err) // 非重复注册错误，触发panic
			}
		}
	}
}
