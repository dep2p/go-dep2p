package config

import (
	"errors"
	"time"
)

// MessagingConfig 消息传递配置
//
// 配置协议层的消息传递服务：
//   - PubSub: 发布订阅
//   - Streams: 流式传输
//   - Liveness: 心跳存活检测
type MessagingConfig struct {
	// EnablePubSub 启用 PubSub
	EnablePubSub bool

	// EnableStreams 启用 Streams
	EnableStreams bool

	// EnableLiveness 启用 Liveness
	EnableLiveness bool

	// PubSub PubSub 配置
	PubSub PubSubConfig

	// Streams Streams 配置
	Streams StreamsConfig

	// Liveness Liveness 配置
	Liveness LivenessConfig
}

// PubSubConfig PubSub 配置
type PubSubConfig struct {
	// MaxMessageSize 最大消息大小（字节）
	MaxMessageSize int

	// ValidateQueueSize 验证队列大小
	ValidateQueueSize int

	// OutboundQueueSize 出站队列大小
	OutboundQueueSize int

	// ValidationTimeout 验证超时
	ValidationTimeout time.Duration

	// SignMessages 是否对消息签名
	SignMessages bool

	// StrictSignatureVerification 严格签名验证
	StrictSignatureVerification bool

	// FloodPublish 是否使用洪泛发布
	FloodPublish bool
}

// StreamsConfig Streams 配置
type StreamsConfig struct {
	// MaxConcurrentStreams 最大并发流数
	MaxConcurrentStreams int

	// StreamTimeout 流超时
	StreamTimeout time.Duration

	// BufferSize 流缓冲区大小
	BufferSize int

	// MaxMessageSize 最大消息大小
	MaxMessageSize int
}

// LivenessConfig Liveness 配置
type LivenessConfig struct {
	// HeartbeatInterval 心跳间隔
	HeartbeatInterval time.Duration

	// HeartbeatTimeout 心跳超时
	HeartbeatTimeout time.Duration

	// MaxMissedHeartbeats 最大丢失心跳数
	MaxMissedHeartbeats int

	// EnableAutoRemove 是否自动移除失活节点
	EnableAutoRemove bool
}

// DefaultMessagingConfig 返回默认消息配置
func DefaultMessagingConfig() MessagingConfig {
	return MessagingConfig{
		// ════════════════════════════════════════════════════════════════════
		// 消息服务启用配置
		// ════════════════════════════════════════════════════════════════════
		// 注意：这些功能需要 RealmManager，默认禁用
		// 创建 Realm 后可以按需启用
		EnablePubSub:   false, // 禁用 PubSub：需要 Realm 支持
		EnableStreams:  false, // 禁用 Streams：需要 Realm 支持
		EnableLiveness: false, // 禁用 Liveness：需要 Realm 支持

		// ════════════════════════════════════════════════════════════════════
		// PubSub 配置（GossipSub 协议）
		// ════════════════════════════════════════════════════════════════════
		PubSub: PubSubConfig{
			MaxMessageSize:              1 << 20,           // 最大消息大小：1 MB
			ValidateQueueSize:           32,                // 验证队列大小：32 条消息
			OutboundQueueSize:           128,               // 出站队列大小：128 条消息
			ValidationTimeout:           10 * time.Second,  // 验证超时：10 秒
			SignMessages:                true,              // 消息签名：启用，防止篡改
			StrictSignatureVerification: true,              // 严格签名验证：启用，拒绝无签名消息
			FloodPublish:                false,             // 洪泛发布：禁用，使用 GossipSub 优化
		},

		// ════════════════════════════════════════════════════════════════════
		// Streams 配置（点对点流式通信）
		// ════════════════════════════════════════════════════════════════════
		Streams: StreamsConfig{
			MaxConcurrentStreams: 1024,             // 最大并发流：1024 个
			StreamTimeout:        30 * time.Second, // 流超时：30 秒无活动则关闭
			BufferSize:           4096,             // 缓冲区大小：4 KB
			MaxMessageSize:       1 << 20,          // 最大消息大小：1 MB
		},

		// ════════════════════════════════════════════════════════════════════
		// Liveness 配置（心跳存活检测）
		// ════════════════════════════════════════════════════════════════════
		Liveness: LivenessConfig{
			HeartbeatInterval:   30 * time.Second, // 心跳间隔：30 秒
			HeartbeatTimeout:    90 * time.Second, // 心跳超时：90 秒（3 倍间隔）
			MaxMissedHeartbeats: 3,                // 最大丢失心跳：3 次后判定失活
			EnableAutoRemove:    true,             // 自动移除：启用，自动清理失活节点
		},
	}
}

// Validate 验证消息配置
func (c MessagingConfig) Validate() error {
	// 验证 PubSub 配置
	if c.EnablePubSub {
		if c.PubSub.MaxMessageSize <= 0 {
			return errors.New("PubSub max message size must be positive")
		}
		if c.PubSub.ValidateQueueSize <= 0 {
			return errors.New("PubSub validate queue size must be positive")
		}
		if c.PubSub.OutboundQueueSize <= 0 {
			return errors.New("PubSub outbound queue size must be positive")
		}
		if c.PubSub.ValidationTimeout <= 0 {
			return errors.New("PubSub validation timeout must be positive")
		}
	}

	// 验证 Streams 配置
	if c.EnableStreams {
		if c.Streams.MaxConcurrentStreams <= 0 {
			return errors.New("streams max concurrent streams must be positive")
		}
		if c.Streams.StreamTimeout <= 0 {
			return errors.New("streams stream timeout must be positive")
		}
		if c.Streams.BufferSize <= 0 {
			return errors.New("streams buffer size must be positive")
		}
		if c.Streams.MaxMessageSize <= 0 {
			return errors.New("streams max message size must be positive")
		}
	}

	// 验证 Liveness 配置
	if c.EnableLiveness {
		if c.Liveness.HeartbeatInterval <= 0 {
			return errors.New("liveness heartbeat interval must be positive")
		}
		if c.Liveness.HeartbeatTimeout <= 0 {
			return errors.New("liveness heartbeat timeout must be positive")
		}
		if c.Liveness.HeartbeatTimeout <= c.Liveness.HeartbeatInterval {
			return errors.New("liveness heartbeat timeout must be greater than interval")
		}
		if c.Liveness.MaxMissedHeartbeats <= 0 {
			return errors.New("liveness max missed heartbeats must be positive")
		}
	}

	return nil
}

// WithPubSub 设置是否启用 PubSub
func (c MessagingConfig) WithPubSub(enabled bool) MessagingConfig {
	c.EnablePubSub = enabled
	return c
}

// WithStreams 设置是否启用 Streams
func (c MessagingConfig) WithStreams(enabled bool) MessagingConfig {
	c.EnableStreams = enabled
	return c
}

// WithLiveness 设置是否启用 Liveness
func (c MessagingConfig) WithLiveness(enabled bool) MessagingConfig {
	c.EnableLiveness = enabled
	return c
}
