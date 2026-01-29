package storage

import (
	"context"

	"github.com/dep2p/go-dep2p/config"
	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
	"github.com/dep2p/go-dep2p/internal/core/storage/engine/badger"
	"github.com/dep2p/go-dep2p/internal/core/storage/kv"
	"go.uber.org/fx"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

var logger = log.Logger("core/storage")

// Params Storage 模块依赖参数
type Params struct {
	fx.In

	UnifiedCfg *config.Config `optional:"true"`
}

// Result Storage 模块提供的结果
type Result struct {
	fx.Out

	Engine engine.InternalEngine
	Config Config
}

// Module 返回 Storage Fx 模块
//
// 提供:
//   - engine.InternalEngine: 存储引擎实例
//   - Config: 存储配置
//
// 生命周期:
//   - OnStart: 启动引擎（GC 等后台任务）
//   - OnStop: 关闭引擎
func Module() fx.Option {
	return fx.Module("storage",
		fx.Provide(
			ProvideStorage,
		),
		fx.Invoke(registerLifecycle),
	)
}

// ProvideStorage 提供存储引擎和配置
func ProvideStorage(p Params) (Result, error) {
	cfg := ConfigFromUnified(p.UnifiedCfg)

	if err := cfg.Validate(); err != nil {
		return Result{}, err
	}

	eng, err := NewEngine(cfg)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Engine: eng,
		Config: cfg,
	}, nil
}

// registerLifecycle 注册生命周期钩子
func registerLifecycle(lc fx.Lifecycle, eng engine.InternalEngine) {
	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			logger.Info("正在启动存储引擎")
			if err := eng.Start(); err != nil {
				logger.Error("存储引擎启动失败", "error", err)
				return err
			}
			logger.Info("存储引擎启动成功")
			return nil
		},
		OnStop: func(_ context.Context) error {
			logger.Info("正在关闭存储引擎")
			if err := eng.Close(); err != nil {
				logger.Warn("存储引擎关闭失败", "error", err)
				return err
			}
			logger.Info("存储引擎已关闭")
			return nil
		},
	})
}

// NewEngine 根据配置创建存储引擎
func NewEngine(cfg Config) (engine.InternalEngine, error) {
	logger.Debug("创建存储引擎", "path", cfg.Path)
	engineCfg := cfg.ToEngineConfig()
	eng, err := badger.New(engineCfg)
	if err != nil {
		logger.Error("创建存储引擎失败", "error", err)
		return nil, err
	}
	logger.Debug("存储引擎创建成功")
	return eng, nil
}

// NewKVStore 创建带前缀的 KVStore
//
// 参数:
//   - eng: 存储引擎
//   - prefix: 键前缀
//
// 返回:
//   - *kv.Store: KVStore 实例
func NewKVStore(eng engine.InternalEngine, prefix []byte) *kv.Store {
	return kv.New(eng, prefix)
}

// ============= 便捷函数 =============

// New 创建持久化存储引擎
//
// 参数:
//   - path: 存储路径
func New(path string) (engine.InternalEngine, error) {
	cfg := DefaultConfig()
	cfg.Path = path
	return NewEngine(cfg)
}

// ============= 类型别名（便于外部使用） =============

// InternalEngine 是 engine.InternalEngine 的类型别名
type InternalEngine = engine.InternalEngine

// KVStore 是 kv.Store 的类型别名
type KVStore = kv.Store

// Batch 是 kv.Batch 的类型别名
type Batch = kv.Batch

// Transaction 是 kv.Transaction 的类型别名
type Transaction = kv.Transaction
