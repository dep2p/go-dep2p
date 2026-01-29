// Package pubsub 实现发布订阅协议
package pubsub

import "time"

// Config PubSub 服务配置
type Config struct {
	// HeartbeatInterval 心跳间隔
	HeartbeatInterval time.Duration

	// DisableHeartbeat 禁用心跳(仅用于测试)
	DisableHeartbeat bool

	// D 目标 Mesh 度数
	D int

	// Dlo Mesh 度数下限
	Dlo int

	// Dhi Mesh 度数上限
	Dhi int

	// Dlazy lazy 推送的节点数
	Dlazy int

	// MaxMessageSize 最大消息大小
	MaxMessageSize int

	// MessageCacheSize 消息缓存大小
	MessageCacheSize int

	// HistoryLength 历史消息保留时长
	HistoryLength int

	// HistoryGossip gossip 历史消息数
	HistoryGossip int

	// SeenMessagesTTL 已见消息 TTL
	SeenMessagesTTL time.Duration

	// ReliableDelivery 可靠投递配置（P1 修复完成）
	ReliableDelivery ReliableConfig

	// PeerScoring 节点评分配置（P1 修复完成）
	PeerScoring PeerScoringConfig
}

// PeerScoringConfig 节点评分配置
type PeerScoringConfig struct {
	// Enabled 是否启用评分
	Enabled bool

	// ScoreParams 评分参数
	ScoreParams *ScoreParams

	// GossipThreshold Gossip 阈值（低于此值不参与 gossip）
	GossipThreshold float64

	// PublishThreshold 发布阈值（低于此值不接受发布）
	PublishThreshold float64

	// GraylistThreshold 灰名单阈值（低于此值加入灰名单）
	GraylistThreshold float64

	// AcceptPXThreshold PX 接受阈值（高于此值接受 PX）
	AcceptPXThreshold float64

	// DecayInterval 衰减执行间隔
	DecayInterval time.Duration
}

// DefaultPeerScoringConfig 默认评分配置
func DefaultPeerScoringConfig() PeerScoringConfig {
	return PeerScoringConfig{
		Enabled:           false, // 默认禁用
		ScoreParams:       nil,   // 使用默认参数
		GossipThreshold:   -500,
		PublishThreshold:  -1000,
		GraylistThreshold: -2500,
		AcceptPXThreshold: 10,
		DecayInterval:     time.Second,
	}
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		HeartbeatInterval: time.Second,
		D:                 6,          // 目标度数
		Dlo:               4,          // 下限
		Dhi:               12,         // 上限
		Dlazy:             6,          // lazy 推送节点数
		MaxMessageSize:    1 << 20,    // 1MB
		MessageCacheSize:  128,
		HistoryLength:     5,
		HistoryGossip:     3,
		SeenMessagesTTL:   2 * time.Minute,
		ReliableDelivery:  DefaultReliableConfig(), // P1 修复完成
		PeerScoring:       DefaultPeerScoringConfig(), // P1 修复完成
	}
}

// Option 配置选项函数
type Option func(*Config)

// WithHeartbeatInterval 设置心跳间隔
func WithHeartbeatInterval(interval time.Duration) Option {
	return func(c *Config) {
		c.HeartbeatInterval = interval
	}
}

// WithMeshDegree 设置 Mesh 度数
func WithMeshDegree(d, dlo, dhi int) Option {
	return func(c *Config) {
		c.D = d
		c.Dlo = dlo
		c.Dhi = dhi
	}
}

// WithMaxMessageSize 设置最大消息大小
func WithMaxMessageSize(size int) Option {
	return func(c *Config) {
		c.MaxMessageSize = size
	}
}

// WithMessageCacheSize 设置消息缓存大小
func WithMessageCacheSize(size int) Option {
	return func(c *Config) {
		c.MessageCacheSize = size
	}
}

// WithDisableHeartbeat 禁用心跳(仅用于测试)
func WithDisableHeartbeat(disable bool) Option {
	return func(c *Config) {
		c.DisableHeartbeat = disable
	}
}

// WithPeerScoring 启用节点评分
func WithPeerScoring(enabled bool) Option {
	return func(c *Config) {
		c.PeerScoring.Enabled = enabled
	}
}

// WithPeerScoringParams 设置节点评分参数
func WithPeerScoringParams(params *ScoreParams) Option {
	return func(c *Config) {
		c.PeerScoring.ScoreParams = params
	}
}

// WithPeerScoringThresholds 设置节点评分阈值
func WithPeerScoringThresholds(gossip, publish, graylist, acceptPX float64) Option {
	return func(c *Config) {
		c.PeerScoring.GossipThreshold = gossip
		c.PeerScoring.PublishThreshold = publish
		c.PeerScoring.GraylistThreshold = graylist
		c.PeerScoring.AcceptPXThreshold = acceptPX
	}
}
