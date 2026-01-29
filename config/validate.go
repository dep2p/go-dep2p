package config

import (
	"errors"
	"fmt"
)

// ValidateAll 验证整个配置的有效性
//
// 这是 Config.Validate() 的别名，提供更明确的语义。
// 它会递归验证所有子配置。
func ValidateAll(c *Config) error {
	if c == nil {
		return errors.New("config is nil")
	}
	return c.Validate()
}

// ValidateAndFix 验证配置并尝试自动修复常见问题
//
// 该函数会：
//   1. 检查配置有效性
//   2. 对于某些可修复的问题，自动应用修复
//   3. 返回修复后的配置或错误
//
// 可修复的问题示例：
//   - 低水位大于高水位 -> 交换值
//   - 超时时间为负 -> 使用默认值
//   - 空的列表 -> 使用默认值
func ValidateAndFix(c *Config) (*Config, error) {
	if c == nil {
		return NewConfig(), nil
	}

	// 连接管理：修复水位问题
	if c.ConnMgr.LowWater > c.ConnMgr.HighWater {
		c.ConnMgr.LowWater, c.ConnMgr.HighWater = c.ConnMgr.HighWater, c.ConnMgr.LowWater
	}

	// 发现：如果未指定引导节点且禁用了其他发现机制，启用 mDNS
	if !c.Discovery.EnableDHT && !c.Discovery.EnableMDNS &&
		!c.Discovery.EnableBootstrap && !c.Discovery.EnableRendezvous && !c.Discovery.EnableDNS {
		c.Discovery.EnableMDNS = true
	}

	// NAT：如果启用 AutoNAT 但没有 STUN 服务器，使用默认值
	if c.NAT.EnableAutoNAT && len(c.NAT.STUNServers) == 0 {
		c.NAT.STUNServers = DefaultNATConfig().STUNServers
	}

	// 传输：确保至少启用一种传输协议
	if !c.Transport.EnableQUIC && !c.Transport.EnableTCP && !c.Transport.EnableWebSocket {
		c.Transport.EnableQUIC = true
	}

	// 安全：确保至少启用一种安全协议
	if !c.Security.EnableTLS && !c.Security.EnableNoise {
		c.Security.EnableTLS = true
	}

	// 验证修复后的配置
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed after fixes: %w", err)
	}

	return c, nil
}

// ValidateSubConfig 验证特定子配置
//
// 用于单独验证某个子配置而不验证整个配置树。
type ValidateSubConfig interface {
	Validate() error
}

// MustValidate 验证配置，如果失败则 panic
//
// 仅用于初始化阶段或测试代码。
// 生产代码应使用 Validate() 并处理错误。
func MustValidate(c *Config) {
	if err := c.Validate(); err != nil {
		panic(fmt.Sprintf("config validation failed: %v", err))
	}
}

// ValidateCompatibility 验证配置之间的兼容性
//
// 检查配置的各个部分是否相互兼容。
// 例如：
//   - 如果启用 Relay 服务端，应该有足够的资源限制
//   - 如果启用 DHT，应该有足够的连接限制
func ValidateCompatibility(c *Config) error {
	if c == nil {
		return errors.New("config is nil")
	}

	// 检查：Relay 服务端需要足够的资源
	if c.Relay.EnableServer {
		minConnections := c.Relay.Server.MaxReservations * 2
		if c.Resource.System.MaxConnections < minConnections {
			return fmt.Errorf("relay server enabled but insufficient connections (need %d, have %d)",
				minConnections, c.Resource.System.MaxConnections)
		}
	}

	// 检查：DHT 需要足够的连接
	if c.Discovery.EnableDHT {
		minConnections := c.Discovery.DHT.BucketSize * 10
		if c.ConnMgr.HighWater < minConnections {
			return fmt.Errorf("DHT enabled but insufficient connection limit (need %d, have %d)",
				minConnections, c.ConnMgr.HighWater)
		}
	}

	// 注：AutoNAT 服务端需要公网地址，此检查在运行时进行

	// 检查：资源限制的层次关系
	if c.Resource.EnableResourceManager {
		// 对等节点限制应该小于系统限制
		if c.Resource.Peer.MaxStreamsPerPeer*c.ConnMgr.HighWater > c.Resource.System.MaxStreams {
			return fmt.Errorf("peer stream limit * max connections (%d) exceeds system stream limit (%d)",
				c.Resource.Peer.MaxStreamsPerPeer*c.ConnMgr.HighWater,
				c.Resource.System.MaxStreams)
		}
	}

	return nil
}

// ValidateForEnvironment 验证配置是否适用于特定环境
//
// 环境类型：
//   - "mobile": 移动设备
//   - "desktop": 桌面应用
//   - "server": 服务器
//   - "embedded": 嵌入式设备
func ValidateForEnvironment(c *Config, env string) error {
	if c == nil {
		return errors.New("config is nil")
	}

	// 先进行基本验证
	if err := c.Validate(); err != nil {
		return err
	}

	switch env {
	case "mobile":
		// 移动设备：检查资源限制
		if c.Resource.System.MaxMemory > 512<<20 { // 512 MB
			return errors.New("mobile: memory limit too high (max 512MB recommended)")
		}
		if c.ConnMgr.HighWater > 100 {
			return errors.New("mobile: connection limit too high (max 100 recommended)")
		}
		if c.Relay.EnableServer {
			return errors.New("mobile: relay server should be disabled")
		}

	case "desktop":
		// 桌面应用：中等限制
		if c.Resource.System.MaxMemory > 2<<30 { // 2 GB
			return errors.New("desktop: memory limit too high (max 2GB recommended)")
		}

	case "server":
		// 服务器：需要更高的限制
		if c.ConnMgr.HighWater < 500 {
			return errors.New("server: connection limit too low (min 500 recommended)")
		}
		if !c.NAT.AutoNAT.EnableServer && !c.Relay.EnableServer {
			return errors.New("server: should enable AutoNAT or Relay server")
		}

	case "embedded":
		// 嵌入式设备：极低限制
		if c.Resource.System.MaxMemory > 256<<20 { // 256 MB
			return errors.New("embedded: memory limit too high (max 256MB recommended)")
		}
		if c.ConnMgr.HighWater > 50 {
			return errors.New("embedded: connection limit too high (max 50 recommended)")
		}

	default:
		return fmt.Errorf("unknown environment: %s", env)
	}

	return nil
}
