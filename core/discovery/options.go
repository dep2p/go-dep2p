package discovery

import "time"

// Option 定义了一个发现选项的函数类型
// 参数:
//   - opts: 选项配置对象指针
//
// 返回值:
//   - error: 错误信息
type Option func(opts *Options) error

// Options 定义了发现服务的配置选项
type Options struct {
	// Ttl 广告的存活时间
	Ttl time.Duration
	// Limit 对等节点数量上限
	Limit int

	// Other 其他特定实现的选项
	// key和value类型为interface{}的map
	Other map[interface{}]interface{}
}

// Apply 将给定的选项应用到当前Options对象
// 参数:
//   - options: 可变参数,接收多个Option函数
//
// 返回值:
//   - error: 错误信息
func (opts *Options) Apply(options ...Option) error {
	// 遍历所有选项函数
	for _, o := range options {
		// 执行选项函数,如果返回错误则直接返回
		if err := o(opts); err != nil {
			log.Debugf("应用选项失败: %v", err)
			return err
		}
	}
	return nil
}

// TTL 设置广告的存活时间选项
// 参数:
//   - ttl: 存活时间
//
// 返回值:
//   - Option: 返回一个Option函数
func TTL(ttl time.Duration) Option {
	return func(opts *Options) error {
		// 设置ttl字段
		opts.Ttl = ttl
		return nil
	}
}

// Limit 设置对等节点数量上限选项
// 参数:
//   - limit: 数量上限
//
// 返回值:
//   - Option: 返回一个Option函数
func Limit(limit int) Option {
	return func(opts *Options) error {
		// 设置limit字段
		opts.Limit = limit
		return nil
	}
}
