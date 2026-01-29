// Package peerstore 实现节点信息存储
package peerstore

import (
	"context"
	"fmt"
	"time"

	"github.com/dep2p/go-dep2p/config"
	"github.com/dep2p/go-dep2p/internal/core/peerstore/addrbook"
	"github.com/dep2p/go-dep2p/internal/core/peerstore/keybook"
	"github.com/dep2p/go-dep2p/internal/core/peerstore/metadata"
	"github.com/dep2p/go-dep2p/internal/core/peerstore/protobook"
	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dep2p/go-dep2p/internal/core/storage/kv"
	pkgif "github.com/dep2p/go-dep2p/pkg/interfaces"
	"go.uber.org/fx"
)

// 存储前缀
const (
	addrBookPrefix  = "a/" // 地址簿前缀
	keyBookPrefix   = "k/" // 密钥簿前缀
	protoBookPrefix = "p/" // 协议簿前缀
	metadataPrefix  = "m/" // 元数据前缀
)

// Config Peerstore 配置
//
// 从 v1.1.0 开始，Peerstore 统一使用 BadgerDB 持久化存储，
// 不再提供内存模式选项。
type Config struct {
	// DataDir 数据目录（从 config.Storage.DataDir 继承）
	// 如果为空，使用默认路径 "./data"
	DataDir string

	// GC 配置
	EnableGC    bool          // 是否启用 GC
	GCInterval  time.Duration // GC 间隔
	GCLookahead time.Duration // GC 提前量
}

// NewConfig 创建默认配置
func NewConfig() Config {
	return Config{
		DataDir:     "./data", // 默认数据目录
		EnableGC:    true,
		GCInterval:  1 * time.Minute,
		GCLookahead: 10 * time.Second,
	}
}

// ConfigFromUnified 从统一配置创建 Peerstore 配置
//
// 从 config.Storage.DataDir 读取数据目录。
func ConfigFromUnified(cfg *config.Config) Config {
	c := NewConfig()
	if cfg != nil && cfg.Storage.DataDir != "" {
		c.DataDir = cfg.Storage.DataDir
	}
	return c
}

// WithDataDir 设置数据目录
func (c Config) WithDataDir(dir string) Config {
	c.DataDir = dir
	return c
}

// Params Peerstore 依赖参数
type Params struct {
	fx.In

	UnifiedCfg *config.Config        `optional:"true"`
	Engine     engine.InternalEngine `optional:"true"` // 可选的存储引擎
}

// Output Peerstore 模块输出
type Output struct {
	fx.Out

	Peerstore pkgif.Peerstore // 移除 name 标签
}

// Module 返回 Fx 模块
func Module() fx.Option {
	return fx.Module("peerstore",
		fx.Provide(
			ProvideConfig,
			providePeerstoreNamed,
		),
		fx.Invoke(registerLifecycle),
	)
}

// providePeerstoreNamed 提供带名称标签的 Peerstore
func providePeerstoreNamed(cfg Config, p Params) (Output, error) {
	ps, err := ProvidePeerstore(cfg, p)
	if err != nil {
		return Output{}, err
	}
	return Output{Peerstore: ps}, nil
}

// ProvideConfig 从统一配置提供 Peerstore 配置
func ProvideConfig(p Params) Config {
	return ConfigFromUnified(p.UnifiedCfg)
}

// ProvidePeerstore 提供 Peerstore 实例
//
// 从 v1.1.0 开始，始终使用持久化存储。
// 如果没有存储引擎注入，将返回错误。
func ProvidePeerstore(_ Config, p Params) (*Peerstore, error) {
	// 必须有存储引擎
	if p.Engine == nil {
		return nil, fmt.Errorf("peerstore: storage engine is required (v1.1.0+ requires persistent storage)")
	}

	// 创建持久化 Peerstore
	return NewPersistentPeerstore(p.Engine, nil)
}

// NewPersistentPeerstore 创建持久化 Peerstore
//
// 参数:
//   - eng: 存储引擎
//   - keyDeserializer: 密钥反序列化器（可选）
func NewPersistentPeerstore(eng engine.InternalEngine, keyDeserializer keybook.KeyDeserializer) (*Peerstore, error) {
	// 创建带前缀的 KV 存储
	peerstoreKV := kv.New(eng, []byte("p/"))

	// 创建各子组件的持久化实现
	addrBook, err := addrbook.NewPersistent(peerstoreKV.SubStore([]byte(addrBookPrefix)))
	if err != nil {
		return nil, err
	}

	keyBook, err := keybook.NewPersistent(peerstoreKV.SubStore([]byte(keyBookPrefix)), keyDeserializer)
	if err != nil {
		addrBook.Close()
		return nil, err
	}

	protoBook, err := protobook.NewPersistent(peerstoreKV.SubStore([]byte(protoBookPrefix)))
	if err != nil {
		addrBook.Close()
		return nil, err
	}

	metadataStore, err := metadata.NewPersistent(peerstoreKV.SubStore([]byte(metadataPrefix)))
	if err != nil {
		addrBook.Close()
		return nil, err
	}

	return &Peerstore{
		addrBook:           addrBook,
		keyBook:            keyBook,
		protoBook:          protoBook,
		metadata:           metadataStore,
		persistentAddrBook: addrBook,
		persistentKeyBook:  keyBook,
		persistentProto:    protoBook,
		persistentMeta:     metadataStore,
		closed:             false,
	}, nil
}

// lifecycleInput 生命周期注册输入
type lifecycleInput struct {
	fx.In

	LC        fx.Lifecycle
	Peerstore pkgif.Peerstore // 移除 name 标签
}

// registerLifecycle 注册生命周期钩子
func registerLifecycle(input lifecycleInput) {
	input.LC.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			// GC 已在 AddrBook.New() 中自动启动
			return nil
		},
		OnStop: func(_ context.Context) error {
			// 尝试关闭 Peerstore（如果支持）
			if closer, ok := input.Peerstore.(interface{ Close() error }); ok {
				return closer.Close()
			}
			return nil
		},
	})
}
