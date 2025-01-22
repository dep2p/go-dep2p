package routing

// Option 是一个函数类型，用于修改 Options 结构体的选项
// 参数：
//   - opts: *Options 需要修改的选项对象
//
// 返回值：
//   - error: 如果发生错误，返回错误信息
type Option func(opts *Options) error

// Options 定义了路由系统的配置选项集合
type Options struct {
	// Expired 允许返回过期的值
	Expired bool
	// Offline 表示是否离线运行
	Offline bool
	// Other 存储其他特定于 ValueStore 实现的选项
	Other map[interface{}]interface{}
}

// Apply 将给定的选项应用到当前 Options 实例
// 参数：
//   - options: ...Option 要应用的选项列表
//
// 返回值：
//   - error: 如果应用选项时发生错误，返回错误信息
func (opts *Options) Apply(options ...Option) error {
	// 遍历所有选项并依次应用
	for _, o := range options {
		if err := o(opts); err != nil {
			log.Debugf("应用选项失败: %v", err)
			return err
		}
	}
	return nil
}

// ToOption 将当前 Options 实例转换为单个 Option 函数
// 返回值：
//   - Option: 返回一个可以复制当前选项的 Option 函数
func (opts *Options) ToOption() Option {
	return func(nopts *Options) error {
		// 复制基本字段
		*nopts = *opts
		// 深度复制 Other 字段
		if opts.Other != nil {
			nopts.Other = make(map[interface{}]interface{}, len(opts.Other))
			for k, v := range opts.Other {
				nopts.Other[k] = v
			}
		}
		return nil
	}
}

// Expired 是一个选项，用于告诉路由系统在没有更新记录时返回过期记录
var Expired Option = func(opts *Options) error {
	opts.Expired = true
	return nil
}

// Offline 是一个选项，用于告诉路由系统以离线模式运行(即仅依赖缓存/本地数据)
var Offline Option = func(opts *Options) error {
	opts.Offline = true
	return nil
}
