package eventbus

import (
	"fmt"
	"runtime"
	"strings"
	"sync/atomic"
)

// subSettings 定义订阅者设置
type subSettings struct {
	buffer int    // 缓冲区大小
	name   string // 订阅者名称
}

// subCnt 订阅者计数器
var subCnt atomic.Int64

// subSettingsDefault 默认订阅者设置
var subSettingsDefault = subSettings{
	buffer: 16, // 默认缓冲区大小为16
}

// newSubSettings 创建新的订阅者设置
// 默认命名策略为 sub-<文件名>-L<行号>
// 返回:
//   - subSettings 订阅者设置
func newSubSettings() subSettings {
	// 使用默认设置
	settings := subSettingsDefault
	// 获取调用者信息,跳过2层调用栈(1层是eventbus.Subscriber)
	_, file, line, ok := runtime.Caller(2)
	if ok {
		// 移除github.com前缀
		file = strings.TrimPrefix(file, "github.com/")
		// 移除版本号,例如 go-dep2p-package@v0.x.y-some-hash-123/file.go
		// 将被缩短为 go-dep2p-package/file.go
		if idx1 := strings.Index(file, "@"); idx1 != -1 {
			if idx2 := strings.Index(file[idx1:], "/"); idx2 != -1 {
				file = file[:idx1] + file[idx1+idx2:]
			}
		}
		// 设置名称为 文件名-行号
		settings.name = fmt.Sprintf("%s-L%d", file, line)
	} else {
		// 如果获取调用信息失败,使用计数器生成名称
		settings.name = fmt.Sprintf("subscriber-%d", subCnt.Add(1))
	}
	return settings
}

// BufSize 设置缓冲区大小的选项函数
// 参数:
//   - n: int 缓冲区大小
//
// 返回:
//   - func(interface{}) error 选项函数
func BufSize(n int) func(interface{}) error {
	return func(s interface{}) error {
		// 设置缓冲区大小
		s.(*subSettings).buffer = n
		return nil
	}
}

// Name 设置订阅者名称的选项函数
// 参数:
//   - name: string 订阅者名称
//
// 返回:
//   - func(interface{}) error 选项函数
func Name(name string) func(interface{}) error {
	return func(s interface{}) error {
		// 设置订阅者名称
		s.(*subSettings).name = name
		return nil
	}
}

// emitterSettings 定义发射器设置
type emitterSettings struct {
	makeStateful bool // 是否保持状态
}

// Stateful 使事件总线通道"记住"最后发送的事件
// 当新订阅者加入总线时,记住的事件会立即发送到订阅通道
// 这允许为动态系统提供状态跟踪,并/或允许新订阅者验证通道上是否有发射器
// 参数:
//   - s: interface{} 设置对象
//
// 返回:
//   - error 设置错误
func Stateful(s interface{}) error {
	// 设置为有状态
	s.(*emitterSettings).makeStateful = true
	return nil
}

// Option 定义总线选项函数类型
type Option func(*basicBus)

// WithMetricsTracer 设置指标跟踪器的选项函数
// 参数:
//   - metricsTracer: MetricsTracer 指标跟踪器
//
// 返回:
//   - Option 选项函数
func WithMetricsTracer(metricsTracer MetricsTracer) Option {
	return func(bus *basicBus) {
		// 设置总线和通配符节点的指标跟踪器
		bus.metricsTracer = metricsTracer
		bus.wildcard.metricsTracer = metricsTracer
	}
}
