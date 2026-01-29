package dht

import (
	"errors"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// Config DHT 配置
type Config struct {
	// BucketSize K-桶大小
	BucketSize int

	// Alpha 并发查询参数
	Alpha int

	// QueryTimeout 查询超时
	QueryTimeout time.Duration

	// RefreshInterval 路由表刷新间隔
	RefreshInterval time.Duration

	// ReplicationFactor 值复制因子
	ReplicationFactor int

	// EnableValueStore 启用值存储
	EnableValueStore bool

	// MaxRecordAge 记录最大存活时间
	MaxRecordAge time.Duration

	// BootstrapPeers 引导节点
	BootstrapPeers []types.PeerInfo

	// ProviderTTL Provider 记录 TTL
	ProviderTTL time.Duration

	// PeerRecordTTL PeerRecord TTL
	PeerRecordTTL time.Duration

	// CleanupInterval 清理间隔
	CleanupInterval time.Duration

	// RepublishInterval PeerRecord 重新发布间隔
	RepublishInterval time.Duration

	// ============= 存储配置 =============

	// DataDir 数据目录（从 config.Storage.DataDir 继承）
	// DHT 使用 BadgerDB 持久化存储
	DataDir string

	// ============= 地址验证配置 =============

	// AllowPrivateAddrs 是否允许私网地址
	// 设置为 true 时，DHT 会接受私网地址（如 192.168.x.x、10.x.x.x 等）
	// 在局域网测试或私有网络部署时很有用
	// 默认: false（仅接受公网地址）
	AllowPrivateAddrs bool

	// ============= v2.0 新增：地址发布策略 =============

	// AddressPublishStrategy 地址发布策略
	// - PublishAll: 发布所有地址（直连 + 中继）
	// - PublishDirectOnly: 仅发布直连地址
	// - PublishRelayOnly: 仅发布中继地址（用于 Private 节点）
	// - PublishAuto: 自动根据可达性决定（默认，推荐）
	//   - Public 节点: 发布所有地址
	//   - Private 节点: 仅发布中继地址
	AddressPublishStrategy AddressPublishStrategy

	// ReachabilityProvider 可达性提供器（用于 PublishAuto 策略）
	// 如果为 nil，则视为 Public 节点
	ReachabilityProvider func() types.NATType
}

// AddressPublishStrategy 地址发布策略
type AddressPublishStrategy int

const (
	// PublishAuto 自动策略（根据可达性决定）
	PublishAuto AddressPublishStrategy = iota
	// PublishAll 发布所有地址
	PublishAll
	// PublishDirectOnly 仅发布直连地址
	PublishDirectOnly
	// PublishRelayOnly 仅发布中继地址
	PublishRelayOnly
)

// DefaultConfig 返回默认配置
//
// 从 v1.1.0 开始，DHT 统一使用 BadgerDB 持久化存储。
func DefaultConfig() *Config {
	return &Config{
		BucketSize:          20,
		Alpha:               5,                    // v2.0.1: 从 3 增加到 5，提升查询并发度
		QueryTimeout:        60 * time.Second,    // v2.0.1: 从 30s 增加到 60s，减少超时风险
		RefreshInterval:     1 * time.Hour,
		ReplicationFactor:   3,
		EnableValueStore:    true,
		MaxRecordAge:        24 * time.Hour,
		BootstrapPeers:      nil,
		ProviderTTL:         24 * time.Hour,
		PeerRecordTTL:       1 * time.Hour,
		CleanupInterval:     10 * time.Minute,
		RepublishInterval:   20 * time.Minute,
		DataDir:             "./data", // 默认数据目录
		AllowPrivateAddrs:   true,     // 默认允许私网地址，便于测试和私有网络部署
	}
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c.BucketSize <= 0 {
		return errors.New("bucket size must be positive")
	}

	if c.Alpha <= 0 {
		return errors.New("alpha must be positive")
	}

	if c.QueryTimeout <= 0 {
		return errors.New("query timeout must be positive")
	}

	if c.RefreshInterval <= 0 {
		return errors.New("refresh interval must be positive")
	}

	if c.ReplicationFactor <= 0 {
		return errors.New("replication factor must be positive")
	}

	if c.MaxRecordAge <= 0 {
		return errors.New("max record age must be positive")
	}

	if c.ProviderTTL <= 0 {
		return errors.New("provider TTL must be positive")
	}

	if c.PeerRecordTTL <= 0 {
		return errors.New("peer record TTL must be positive")
	}

	if c.CleanupInterval <= 0 {
		return errors.New("cleanup interval must be positive")
	}

	if c.RepublishInterval <= 0 {
		return errors.New("republish interval must be positive")
	}

	return nil
}

// ConfigOption 配置选项函数
type ConfigOption func(*Config)

// WithBucketSize 设置K-桶大小
func WithBucketSize(size int) ConfigOption {
	return func(c *Config) {
		c.BucketSize = size
	}
}

// WithAlpha 设置并发查询参数
func WithAlpha(alpha int) ConfigOption {
	return func(c *Config) {
		c.Alpha = alpha
	}
}

// WithQueryTimeout 设置查询超时
func WithQueryTimeout(timeout time.Duration) ConfigOption {
	return func(c *Config) {
		c.QueryTimeout = timeout
	}
}

// WithRefreshInterval 设置刷新间隔
func WithRefreshInterval(interval time.Duration) ConfigOption {
	return func(c *Config) {
		c.RefreshInterval = interval
	}
}

// WithBootstrapPeers 设置引导节点
func WithBootstrapPeers(peers []types.PeerInfo) ConfigOption {
	return func(c *Config) {
		c.BootstrapPeers = peers
	}
}

// WithValueStore 设置是否启用值存储
func WithValueStore(enabled bool) ConfigOption {
	return func(c *Config) {
		c.EnableValueStore = enabled
	}
}

// WithDataDir 设置数据目录
func WithDataDir(dir string) ConfigOption {
	return func(c *Config) {
		c.DataDir = dir
	}
}

// WithAllowPrivateAddrs 设置是否允许私网地址
func WithAllowPrivateAddrs(allow bool) ConfigOption {
	return func(c *Config) {
		c.AllowPrivateAddrs = allow
	}
}

// WithAddressPublishStrategy 设置地址发布策略
func WithAddressPublishStrategy(strategy AddressPublishStrategy) ConfigOption {
	return func(c *Config) {
		c.AddressPublishStrategy = strategy
	}
}

// WithReachabilityProvider 设置可达性提供器
func WithReachabilityProvider(provider func() types.NATType) ConfigOption {
	return func(c *Config) {
		c.ReachabilityProvider = provider
	}
}
