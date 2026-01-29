package bootstrap

import (
	"errors"
	"time"

	"github.com/dep2p/go-dep2p/pkg/types"
)

// Config Bootstrap 配置
type Config struct {
	// Peers 引导节点列表
	Peers []types.PeerInfo

	// Timeout 单个节点连接超时
	Timeout time.Duration

	// MinPeers 最少成功连接数
	MinPeers int

	// MaxRetries 最大重试次数（v1.1 实现）
	MaxRetries int

	// Interval 引导间隔
	Interval time.Duration

	// Enabled 是否启用
	Enabled bool
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config cannot be nil")
	}

	// 如果未启用，跳过验证
	if !c.Enabled {
		return nil
	}

	// 如果启用了 Bootstrap 但 Peers 为空，自动禁用并返回成功
	// 这允许开发/测试环境在没有有效引导节点时正常启动
	if len(c.Peers) == 0 {
		c.Enabled = false
		return nil
	}

	if c.Timeout <= 0 {
		return errors.New("timeout must be positive")
	}

	if c.MinPeers < 0 {
		return errors.New("MinPeers cannot be negative")
	}

	// 智能调整 MinPeers：当提供的节点数少于 MinPeers 时，自动降低到实际节点数
	// 这允许用户只配置 1 个引导节点而不需要手动设置 MinPeers
	if len(c.Peers) > 0 && c.MinPeers > len(c.Peers) {
		logger.Info("自动调整 MinPeers", "原值", c.MinPeers, "新值", len(c.Peers), "实际节点数", len(c.Peers))
		c.MinPeers = len(c.Peers)
	}

	if c.MaxRetries < 0 {
		return errors.New("MaxRetries cannot be negative")
	}

	// 验证每个节点信息
	for i, peer := range c.Peers {
		if peer.ID == "" {
			return errors.New("peer ID cannot be empty at index " + string(rune(i)))
		}

		if len(peer.Addrs) == 0 {
			return errors.New("peer must have at least one address at index " + string(rune(i)))
		}
	}

	return nil
}
