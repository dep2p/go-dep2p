package dep2p

import (
	"github.com/dep2p/go-dep2p/config"
	"github.com/dep2p/go-dep2p/core/host"
	logging "github.com/dep2p/log"
)

var log = logging.Logger("dep2p")

// Config 描述了 dep2p 节点的一组设置
type Config = config.Config

// Option 是一个 dep2p 配置选项，可以传递给 dep2p 构造函数 (`dep2p.New`)
type Option = config.Option

// ChainOptions 将多个选项链接成单个选项
// 参数:
//   - opts: ...Option 要链接的选项列表
//
// 返回:
//   - Option: 链接后的单个选项函数
func ChainOptions(opts ...Option) Option {
	// 返回一个新的选项函数,该函数将按顺序应用所有选项
	return func(cfg *Config) error {
		// 遍历所有选项
		for _, opt := range opts {
			// 跳过空选项
			if opt == nil {
				continue
			}
			// 应用选项,如果出错则返回错误
			if err := opt(cfg); err != nil {
				log.Debugf("应用选项失败: %s", err)
				return err
			}
		}
		return nil
	}
}

// New 使用给定选项构造一个新的 dep2p 节点,如果没有提供某些选项则使用合理的默认值
// 默认值包括:
// - 如果未提供传输和监听地址,节点将监听 "/ip4/0.0.0.0/tcp/0" 和 "/ip6/::/tcp/0"
// - 如果未提供传输选项,节点使用 TCP、websocket 和 QUIC 传输协议
// - 如果未提供多路复用器配置,节点默认使用 yamux
// - 如果未提供安全传输,主机使用 go-dep2p 的 noise 和/或 tls 加密传输来加密所有流量
// - 如果未提供对等身份,它会生成一个随机的 Ed25519 密钥对并从中派生新身份
// - 如果未提供对等存储,主机将使用空的对等存储进行初始化
//
// 参数:
//   - opts: ...Option 配置选项列表
//
// 返回:
//   - host.Host: 新创建的 dep2p 主机
//   - error: 如果发生错误则返回错误信息
func New(opts ...Option) (host.Host, error) {
	// 调用 NewWithoutDefaults 并添加默认值选项
	return NewWithoutDefaults(append(opts, FallbackDefaults)...)
}

// NewWithoutDefaults 使用给定选项构造新的 dep2p 节点,但不使用默认值
// 警告: 此函数不应被视为稳定接口
// 我们可能随时选择添加必需的服务,使用此函数即表示您选择不使用我们可能提供的任何默认值
//
// 参数:
//   - opts: ...Option 配置选项列表
//
// 返回:
//   - host.Host: 新创建的 dep2p 主机
//   - error: 如果发生错误则返回错误信息
func NewWithoutDefaults(opts ...Option) (host.Host, error) {
	// 创建新的配置对象
	var cfg Config
	// 应用所有选项
	if err := cfg.Apply(opts...); err != nil {
		log.Debugf("应用选项失败: %s", err)
		return nil, err
	}
	// 创建并返回新节点
	return cfg.NewNode()
}
