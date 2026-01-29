package rendezvous

import (
	"errors"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// ============================================================================
//                              Discoverer 配置
// ============================================================================

// DiscovererConfig Rendezvous 发现器配置
type DiscovererConfig struct {
	// Points 已知的 Rendezvous 点
	Points []types.PeerID

	// DefaultTTL 默认注册 TTL
	DefaultTTL time.Duration

	// RenewalInterval 续约间隔（通常是 TTL/2）
	RenewalInterval time.Duration

	// DiscoverTimeout 发现超时
	DiscoverTimeout time.Duration

	// RegisterTimeout 注册超时
	RegisterTimeout time.Duration

	// MaxRetries 最大重试次数
	MaxRetries int

	// RetryInterval 重试间隔
	RetryInterval time.Duration
}

// DefaultDiscovererConfig 默认配置
func DefaultDiscovererConfig() DiscovererConfig {
	return DiscovererConfig{
		Points:          nil,
		DefaultTTL:      2 * time.Hour,
		RenewalInterval: 1 * time.Hour,
		DiscoverTimeout: 30 * time.Second,
		RegisterTimeout: 30 * time.Second,
		MaxRetries:      3,
		RetryInterval:   5 * time.Second,
	}
}

// Validate 验证配置
func (c *DiscovererConfig) Validate() error {
	if c.DefaultTTL <= 0 {
		return errors.New("default TTL must be positive")
	}
	if c.RenewalInterval <= 0 {
		return errors.New("renewal interval must be positive")
	}
	if c.RenewalInterval >= c.DefaultTTL {
		return errors.New("renewal interval must be less than default TTL")
	}
	if c.DiscoverTimeout <= 0 {
		return errors.New("discover timeout must be positive")
	}
	if c.RegisterTimeout <= 0 {
		return errors.New("register timeout must be positive")
	}
	if c.MaxRetries < 0 {
		return errors.New("max retries must be non-negative")
	}
	if c.RetryInterval <= 0 {
		return errors.New("retry interval must be positive")
	}
	return nil
}

// ============================================================================
//                              Point 配置
// ============================================================================

// PointConfig Rendezvous Point 配置
type PointConfig struct {
	// MaxRegistrations 最大注册数
	MaxRegistrations int

	// MaxNamespaces 最大命名空间数
	MaxNamespaces int

	// MaxTTL 最大 TTL
	MaxTTL time.Duration

	// DefaultTTL 默认 TTL
	DefaultTTL time.Duration

	// CleanupInterval 清理间隔
	CleanupInterval time.Duration

	// MaxRegistrationsPerNamespace 每个命名空间最大注册数
	MaxRegistrationsPerNamespace int

	// MaxRegistrationsPerPeer 每个节点最大注册数
	MaxRegistrationsPerPeer int

	// DefaultDiscoverLimit 默认发现限制
	DefaultDiscoverLimit int
}

// DefaultPointConfig 默认配置
func DefaultPointConfig() PointConfig {
	return PointConfig{
		MaxRegistrations:             10000,
		MaxNamespaces:                1000,
		MaxTTL:                       72 * time.Hour,
		DefaultTTL:                   2 * time.Hour,
		CleanupInterval:              5 * time.Minute,
		MaxRegistrationsPerNamespace: 1000,
		MaxRegistrationsPerPeer:      100,
		DefaultDiscoverLimit:         100,
	}
}

// Validate 验证配置
func (c *PointConfig) Validate() error {
	if c.MaxRegistrations <= 0 {
		return errors.New("max registrations must be positive")
	}
	if c.MaxNamespaces <= 0 {
		return errors.New("max namespaces must be positive")
	}
	if c.MaxTTL <= 0 {
		return errors.New("max TTL must be positive")
	}
	if c.DefaultTTL <= 0 {
		return errors.New("default TTL must be positive")
	}
	if c.DefaultTTL > c.MaxTTL {
		return errors.New("default TTL must not exceed max TTL")
	}
	if c.CleanupInterval <= 0 {
		return errors.New("cleanup interval must be positive")
	}
	if c.MaxRegistrationsPerNamespace <= 0 {
		return errors.New("max registrations per namespace must be positive")
	}
	if c.MaxRegistrationsPerPeer <= 0 {
		return errors.New("max registrations per peer must be positive")
	}
	if c.DefaultDiscoverLimit <= 0 {
		return errors.New("default discover limit must be positive")
	}
	return nil
}

// ============================================================================
//                              Store 配置
// ============================================================================

// StoreConfig 存储配置
type StoreConfig struct {
	// MaxRegistrations 最大注册总数
	MaxRegistrations int

	// MaxNamespaces 最大命名空间数
	MaxNamespaces int

	// MaxRegistrationsPerNamespace 每个命名空间最大注册数
	MaxRegistrationsPerNamespace int

	// MaxRegistrationsPerPeer 每个节点最大注册数
	MaxRegistrationsPerPeer int

	// MaxTTL 最大 TTL
	MaxTTL time.Duration

	// DefaultTTL 默认 TTL
	DefaultTTL time.Duration

	// CleanupInterval 清理间隔
	CleanupInterval time.Duration
}

// DefaultStoreConfig 默认存储配置
func DefaultStoreConfig() StoreConfig {
	return StoreConfig{
		MaxRegistrations:             10000,
		MaxNamespaces:                1000,
		MaxRegistrationsPerNamespace: 1000,
		MaxRegistrationsPerPeer:      100,
		MaxTTL:                       72 * time.Hour,
		DefaultTTL:                   2 * time.Hour,
		CleanupInterval:              5 * time.Minute,
	}
}

// Validate 验证配置
func (c *StoreConfig) Validate() error {
	if c.MaxRegistrations <= 0 {
		return errors.New("max registrations must be positive")
	}
	if c.MaxNamespaces <= 0 {
		return errors.New("max namespaces must be positive")
	}
	if c.MaxRegistrationsPerNamespace <= 0 {
		return errors.New("max registrations per namespace must be positive")
	}
	if c.MaxRegistrationsPerPeer <= 0 {
		return errors.New("max registrations per peer must be positive")
	}
	if c.MaxTTL <= 0 {
		return errors.New("max TTL must be positive")
	}
	if c.DefaultTTL <= 0 {
		return errors.New("default TTL must be positive")
	}
	if c.CleanupInterval <= 0 {
		return errors.New("cleanup interval must be positive")
	}
	return nil
}
