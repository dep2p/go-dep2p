package engine

import (
	"os"
	"path/filepath"
	"time"
)

// Config 存储引擎配置
//
// 从 v1.1.0 开始，DeP2P 统一使用 BadgerDB 持久化存储。
// 测试代码应使用 t.TempDir() 创建临时目录，确保测试与生产一致。
type Config struct {
	// Path 数据目录路径（必需）
	Path string

	// SyncWrites 是否同步写入
	// 启用后每次写入都会同步到磁盘，更安全但性能较低
	SyncWrites bool

	// NumVersionsToKeep 保留的版本数
	// 用于 MVCC，默认为 1（仅保留最新版本）
	NumVersionsToKeep int

	// ReadOnly 是否只读模式
	// 只读模式下不能进行写入操作
	ReadOnly bool

	// Logger 日志记录器
	// 如果为 nil，将禁用日志
	Logger Logger

	// Badger 特定选项
	Badger BadgerOptions
}

// BadgerOptions BadgerDB 特定选项
type BadgerOptions struct {
	// MemTableSize 内存表大小（字节）
	// 默认 64MB
	MemTableSize int64

	// ValueLogFileSize 值日志文件大小（字节）
	// 默认 1GB
	ValueLogFileSize int64

	// NumMemtables 内存表数量
	// 默认 5
	NumMemtables int

	// NumLevelZeroTables Level 0 表数量阈值
	// 超过此值会触发压缩
	// 默认 5
	NumLevelZeroTables int

	// NumLevelZeroTablesStall Level 0 表数量停滞阈值
	// 超过此值会暂停写入
	// 默认 15
	NumLevelZeroTablesStall int

	// ValueLogMaxEntries 值日志最大条目数
	// 默认 1000000
	ValueLogMaxEntries uint32

	// ValueThreshold 值大小阈值
	// 大于此值的值会存储在值日志中
	// 默认 1KB
	ValueThreshold int64

	// BlockCacheSize 块缓存大小（字节）
	// 默认 256MB
	BlockCacheSize int64

	// IndexCacheSize 索引缓存大小（字节）
	// 默认 0（禁用）
	IndexCacheSize int64

	// NumCompactors 压缩器数量
	// 默认 4
	NumCompactors int

	// CompactL0OnClose 关闭时是否压缩 L0
	// 默认 false
	CompactL0OnClose bool

	// ZSTDCompressionLevel ZSTD 压缩级别
	// 0 表示禁用压缩
	// 默认 1
	ZSTDCompressionLevel int

	// GCInterval 垃圾回收间隔
	// 默认 10 分钟
	GCInterval time.Duration

	// GCDiscardRatio 垃圾回收丢弃比例
	// 默认 0.5
	GCDiscardRatio float64
}

// Logger 日志接口
type Logger interface {
	Errorf(format string, args ...interface{})
	Warningf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Debugf(format string, args ...interface{})
}

// DefaultConfig 返回默认配置
func DefaultConfig(path string) *Config {
	return &Config{
		Path:              path,
		SyncWrites:        false,
		NumVersionsToKeep: 1,
		ReadOnly:          false,
		Logger:            nil,
		Badger:            DefaultBadgerOptions(),
	}
}

// DefaultBadgerOptions 返回默认 BadgerDB 选项
func DefaultBadgerOptions() BadgerOptions {
	return BadgerOptions{
		MemTableSize:            64 << 20, // 64MB
		ValueLogFileSize:        1 << 30,  // 1GB
		NumMemtables:            5,
		NumLevelZeroTables:      5,
		NumLevelZeroTablesStall: 15,
		ValueLogMaxEntries:      1000000,
		ValueThreshold:          1 << 10,   // 1KB
		BlockCacheSize:          256 << 20, // 256MB
		IndexCacheSize:          0,
		NumCompactors:           4,
		CompactL0OnClose:        false,
		ZSTDCompressionLevel:    1,
		GCInterval:              10 * time.Minute,
		GCDiscardRatio:          0.5,
	}
}

// Validate 验证配置
func (c *Config) Validate() error {
	// Path 是必需的
	if c.Path == "" {
		return ErrInvalidConfig
	}

	if c.NumVersionsToKeep < 1 {
		return ErrInvalidConfig
	}

	if c.Badger.MemTableSize < 1<<20 { // 最小 1MB
		return ErrInvalidConfig
	}

	if c.Badger.ValueLogFileSize < 1<<20 { // 最小 1MB
		return ErrInvalidConfig
	}

	return nil
}

// EnsureDir 确保数据目录存在
func (c *Config) EnsureDir() error {
	// 获取绝对路径
	absPath, err := filepath.Abs(c.Path)
	if err != nil {
		return err
	}
	c.Path = absPath

	// 创建目录
	return os.MkdirAll(c.Path, 0755)
}

// Clone 克隆配置
func (c *Config) Clone() *Config {
	clone := *c
	clone.Badger = c.Badger
	return &clone
}

// WithPath 设置数据路径
func (c *Config) WithPath(path string) *Config {
	c.Path = path
	return c
}

// WithSyncWrites 设置同步写入
func (c *Config) WithSyncWrites(sync bool) *Config {
	c.SyncWrites = sync
	return c
}

// WithReadOnly 设置只读模式
func (c *Config) WithReadOnly(readOnly bool) *Config {
	c.ReadOnly = readOnly
	return c
}

// WithLogger 设置日志记录器
func (c *Config) WithLogger(logger Logger) *Config {
	c.Logger = logger
	return c
}

// WithBlockCacheSize 设置块缓存大小
func (c *Config) WithBlockCacheSize(size int64) *Config {
	c.Badger.BlockCacheSize = size
	return c
}

// WithCompression 设置压缩级别
func (c *Config) WithCompression(level int) *Config {
	c.Badger.ZSTDCompressionLevel = level
	return c
}
