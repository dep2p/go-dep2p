package realm

import (
	"fmt"
	"time"
)

// ============================================================================
//                              Manager 配置
// ============================================================================

// ManagerConfig Manager 配置
type ManagerConfig struct {
	// Realm 配置
	DefaultRealmName string // 默认 Realm 名称
	MaxRealms        int    // 最大 Realm 数（暂不使用，仅支持单 Realm）

	// 子模块超时
	AuthTimeout  time.Duration // Auth 超时
	LeaveTimeout time.Duration // Leave 超时
	SyncInterval time.Duration // 状态同步间隔

	// InfrastructurePeers 基础设施节点 ID 列表
	// 包括 Bootstrap 和 Relay 节点的 PeerID
	// 这些节点不是 Realm 成员，连接时跳过认证尝试
	// 避免产生不必要的网络请求和错误日志
	InfrastructurePeers []string

	// RelayPeers Relay 节点 ID 列表（仅 Relay）
	// 用于 Relay 地址簿回退查询
	RelayPeers []string

	// 子模块配置（简化实现：使用接口类型避免循环依赖）
	// AuthConfig    interface{}
	// MemberConfig  interface{}
	// RoutingConfig interface{}
	// GatewayConfig interface{}
}

// DefaultManagerConfig 返回默认配置
func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		DefaultRealmName: "default",
		MaxRealms:        1, // 当前仅支持单 Realm

		// 超时配置
		AuthTimeout:  30 * time.Second,
		LeaveTimeout: 30 * time.Second,
		SyncInterval: 30 * time.Second,

		// 子模块配置（简化实现：已注释）
	}
}

// Validate 验证配置
func (c *ManagerConfig) Validate() error {
	if c.DefaultRealmName == "" {
		return fmt.Errorf("%w: DefaultRealmName is required", ErrInvalidConfig)
	}

	if c.MaxRealms < 1 {
		return fmt.Errorf("%w: MaxRealms must be at least 1", ErrInvalidConfig)
	}

	if c.AuthTimeout <= 0 {
		return fmt.Errorf("%w: AuthTimeout must be positive", ErrInvalidConfig)
	}

	if c.LeaveTimeout <= 0 {
		return fmt.Errorf("%w: LeaveTimeout must be positive", ErrInvalidConfig)
	}

	if c.SyncInterval <= 0 {
		return fmt.Errorf("%w: SyncInterval must be positive", ErrInvalidConfig)
	}

	return nil
}

// Clone 克隆配置
func (c *ManagerConfig) Clone() *ManagerConfig {
	cloned := &ManagerConfig{
		DefaultRealmName: c.DefaultRealmName,
		MaxRealms:        c.MaxRealms,
		AuthTimeout:      c.AuthTimeout,
		LeaveTimeout:     c.LeaveTimeout,
		SyncInterval:     c.SyncInterval,
	}

	// 克隆基础设施节点列表
	if len(c.InfrastructurePeers) > 0 {
		cloned.InfrastructurePeers = make([]string, len(c.InfrastructurePeers))
		copy(cloned.InfrastructurePeers, c.InfrastructurePeers)
	}

	// 克隆 Relay 节点列表
	if len(c.RelayPeers) > 0 {
		cloned.RelayPeers = make([]string, len(c.RelayPeers))
		copy(cloned.RelayPeers, c.RelayPeers)
	}

	// 简化实现：子模块配置已注释

	return cloned
}
