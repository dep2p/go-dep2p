package storage

import (
	"time"

	"github.com/dep2p/go-dep2p/config"
	"github.com/dep2p/go-dep2p/internal/core/storage/engine"
)

// Config Storage 模块配置
//
// 从 v1.1.0 开始，DeP2P 统一使用 BadgerDB 持久化存储。
// 测试代码应使用 t.TempDir() 创建临时目录，确保测试与生产一致。
type Config struct {
	// Path 存储路径（BadgerDB 数据库目录，必需）
	Path string

	// SyncWrites 是否同步写入
	// 启用后每次写入都会同步到磁盘，更安全但性能较低
	SyncWrites bool

	// GCEnabled 是否启用垃圾回收
	GCEnabled bool

	// GCInterval 垃圾回收间隔
	GCInterval time.Duration

	// GCDiscardRatio 垃圾回收丢弃比例
	GCDiscardRatio float64

	// BlockCacheSize 块缓存大小（字节）
	BlockCacheSize int64

	// Compression 压缩级别（0 禁用）
	Compression int
}

// DefaultConfig 返回默认配置
//
// 从 v1.1.0 开始，DeP2P 统一使用 BadgerDB 持久化存储。
// Path 必须设置为有效的目录路径。
func DefaultConfig() Config {
	return Config{
		Path:           "./data/dep2p.db",
		SyncWrites:     false,
		GCEnabled:      true,
		GCInterval:     10 * time.Minute,
		GCDiscardRatio: 0.5,
		BlockCacheSize: 256 << 20, // 256MB
		Compression:    1,
	}
}

// ConfigFromUnified 从统一配置创建 Storage 配置
//
// 从 config.Config.Storage 读取数据目录配置。
func ConfigFromUnified(cfg *config.Config) Config {
	storageCfg := DefaultConfig()

	if cfg == nil {
		return storageCfg
	}

	// 从统一配置读取 DataDir
	if cfg.Storage.DataDir != "" {
		storageCfg.Path = cfg.Storage.DBPath()
	}

	return storageCfg
}

// ToEngineConfig 转换为引擎配置
func (c *Config) ToEngineConfig() *engine.Config {
	engineCfg := engine.DefaultConfig(c.Path)

	engineCfg.SyncWrites = c.SyncWrites
	engineCfg.Badger.GCInterval = c.GCInterval
	engineCfg.Badger.GCDiscardRatio = c.GCDiscardRatio
	engineCfg.Badger.BlockCacheSize = c.BlockCacheSize
	engineCfg.Badger.ZSTDCompressionLevel = c.Compression

	return engineCfg
}

// Validate 验证配置
func (c *Config) Validate() error {
	// Path 是必需的
	if c.Path == "" {
		return ErrInvalidConfig
	}

	if c.GCInterval < time.Minute {
		c.GCInterval = time.Minute
	}

	if c.GCDiscardRatio <= 0 || c.GCDiscardRatio > 1 {
		c.GCDiscardRatio = 0.5
	}

	return nil
}

// WithPath 设置存储路径
func (c Config) WithPath(path string) Config {
	c.Path = path
	return c
}

// WithSyncWrites 设置同步写入
func (c Config) WithSyncWrites(sync bool) Config {
	c.SyncWrites = sync
	return c
}

// WithGC 设置垃圾回收配置
func (c Config) WithGC(enabled bool, interval time.Duration) Config {
	c.GCEnabled = enabled
	c.GCInterval = interval
	return c
}

// WithBlockCache 设置块缓存大小
func (c Config) WithBlockCache(size int64) Config {
	c.BlockCacheSize = size
	return c
}

// WithCompression 设置压缩级别
func (c Config) WithCompression(level int) Config {
	c.Compression = level
	return c
}
