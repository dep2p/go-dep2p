package config

import (
	"errors"
)

// ResourceConfig 资源管理配置
//
// 配置节点的资源限制和配额：
//   - 系统级资源限制
//   - 对等节点资源限制
//   - 协议资源限制
//   - 服务资源限制
type ResourceConfig struct {
	// EnableResourceManager 启用资源管理
	EnableResourceManager bool `json:"enable_resource_manager"`

	// System 系统级限制
	System SystemLimits `json:"system,omitempty"`

	// Peer 对等节点级限制
	Peer PeerLimits `json:"peer,omitempty"`

	// Protocol 协议级限制
	Protocol ProtocolLimits `json:"protocol,omitempty"`

	// Service 服务级限制
	Service ServiceLimits `json:"service,omitempty"`
}

// SystemLimits 系统级资源限制
type SystemLimits struct {
	// MaxConnections 最大连接数
	MaxConnections int `json:"max_connections"`

	// MaxStreams 最大流数量
	MaxStreams int `json:"max_streams"`

	// MaxMemory 最大内存使用（字节）
	// 0 表示不限制
	MaxMemory int64 `json:"max_memory,omitempty"`

	// MaxFD 最大文件描述符数
	// 0 表示不限制
	MaxFD int `json:"max_fd,omitempty"`
}

// PeerLimits 对等节点级资源限制
type PeerLimits struct {
	// MaxStreamsPerPeer 每个对等节点最大流数量
	MaxStreamsPerPeer int

	// MaxConnectionsPerPeer 每个对等节点最大连接数
	MaxConnectionsPerPeer int

	// MaxMemoryPerPeer 每个对等节点最大内存（字节）
	// 0 表示不限制
	MaxMemoryPerPeer int64
}

// ProtocolLimits 协议级资源限制
type ProtocolLimits struct {
	// MaxStreamsPerProtocol 每个协议最大流数量
	MaxStreamsPerProtocol int

	// MaxMemoryPerProtocol 每个协议最大内存（字节）
	// 0 表示不限制
	MaxMemoryPerProtocol int64
}

// ServiceLimits 服务级资源限制
type ServiceLimits struct {
	// MaxStreamsPerService 每个服务最大流数量
	MaxStreamsPerService int

	// MaxMemoryPerService 每个服务最大内存（字节）
	// 0 表示不限制
	MaxMemoryPerService int64
}

// DefaultResourceConfig 返回默认资源配置
func DefaultResourceConfig() ResourceConfig {
	return ResourceConfig{
		// ════════════════════════════════════════════════════════════════════
		// 资源管理启用
		// ════════════════════════════════════════════════════════════════════
		EnableResourceManager: true, // 启用资源管理：防止资源耗尽

		// ════════════════════════════════════════════════════════════════════
		// 系统级资源限制（全局上限）
		// ════════════════════════════════════════════════════════════════════
		System: SystemLimits{
			MaxConnections: 1000,     // 最大连接数：1000 个
			MaxStreams:     10000,    // 最大流数：10000 个
			MaxMemory:      1 << 30,  // 最大内存：1 GB（0 表示不限制）
			MaxFD:          4096,     // 最大文件描述符：4096 个（0 表示不限制）
		},

		// ════════════════════════════════════════════════════════════════════
		// 对等节点级资源限制（单个节点上限）
		// ════════════════════════════════════════════════════════════════════
		Peer: PeerLimits{
			MaxStreamsPerPeer:     100,       // 每节点最大流数：100 个
			MaxConnectionsPerPeer: 8,         // 每节点最大连接数：8 个
			MaxMemoryPerPeer:      64 << 20,  // 每节点最大内存：64 MB
		},

		// ════════════════════════════════════════════════════════════════════
		// 协议级资源限制（单个协议上限）
		// ════════════════════════════════════════════════════════════════════
		Protocol: ProtocolLimits{
			MaxStreamsPerProtocol: 500,       // 每协议最大流数：500 个
			MaxMemoryPerProtocol:  128 << 20, // 每协议最大内存：128 MB
		},

		// ════════════════════════════════════════════════════════════════════
		// 服务级资源限制（单个服务上限）
		// ════════════════════════════════════════════════════════════════════
		Service: ServiceLimits{
			MaxStreamsPerService: 200,       // 每服务最大流数：200 个
			MaxMemoryPerService:  256 << 20, // 每服务最大内存：256 MB
		},
	}
}

// Validate 验证资源配置
func (c ResourceConfig) Validate() error {
	if !c.EnableResourceManager {
		return nil // 如果未启用资源管理，跳过验证
	}

	// 验证系统级限制
	if c.System.MaxConnections <= 0 {
		return errors.New("system max connections must be positive")
	}
	if c.System.MaxStreams <= 0 {
		return errors.New("system max streams must be positive")
	}
	if c.System.MaxMemory < 0 {
		return errors.New("system max memory must be non-negative")
	}
	if c.System.MaxFD < 0 {
		return errors.New("system max FD must be non-negative")
	}

	// 验证对等节点级限制
	if c.Peer.MaxStreamsPerPeer <= 0 {
		return errors.New("peer max streams must be positive")
	}
	if c.Peer.MaxConnectionsPerPeer <= 0 {
		return errors.New("peer max connections must be positive")
	}
	if c.Peer.MaxMemoryPerPeer < 0 {
		return errors.New("peer max memory must be non-negative")
	}

	// 验证协议级限制
	if c.Protocol.MaxStreamsPerProtocol <= 0 {
		return errors.New("protocol max streams must be positive")
	}
	if c.Protocol.MaxMemoryPerProtocol < 0 {
		return errors.New("protocol max memory must be non-negative")
	}

	// 验证服务级限制
	if c.Service.MaxStreamsPerService <= 0 {
		return errors.New("service max streams must be positive")
	}
	if c.Service.MaxMemoryPerService < 0 {
		return errors.New("service max memory must be non-negative")
	}

	// 交叉验证：对等节点限制应该小于系统限制
	if c.Peer.MaxStreamsPerPeer > c.System.MaxStreams {
		return errors.New("peer max streams cannot exceed system max streams")
	}
	if c.Peer.MaxConnectionsPerPeer > c.System.MaxConnections {
		return errors.New("peer max connections cannot exceed system max connections")
	}

	return nil
}

// WithResourceManager 设置是否启用资源管理
func (c ResourceConfig) WithResourceManager(enabled bool) ResourceConfig {
	c.EnableResourceManager = enabled
	return c
}

// WithSystemMaxConnections 设置系统最大连接数
func (c ResourceConfig) WithSystemMaxConnections(max int) ResourceConfig {
	c.System.MaxConnections = max
	return c
}

// WithSystemMaxStreams 设置系统最大流数量
func (c ResourceConfig) WithSystemMaxStreams(max int) ResourceConfig {
	c.System.MaxStreams = max
	return c
}

// WithSystemMaxMemory 设置系统最大内存
func (c ResourceConfig) WithSystemMaxMemory(max int64) ResourceConfig {
	c.System.MaxMemory = max
	return c
}
