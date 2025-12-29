package components

import (
	"time"

	"github.com/dep2p/go-dep2p/internal/config"
)

// MessagingOptions 消息服务选项
type MessagingOptions struct {
	// RequestTimeout 请求超时
	RequestTimeout time.Duration

	// MaxMessageSize 最大消息大小
	MaxMessageSize int

	// PubSub 发布订阅配置
	PubSub PubSubOptions
}

// PubSubOptions 发布订阅选项
type PubSubOptions struct {
	// Enable 启用发布订阅
	Enable bool

	// MessageCacheSize 消息缓存大小（用于去重）
	MessageCacheSize int

	// MessageCacheTTL 消息缓存 TTL
	MessageCacheTTL time.Duration
}

// NewMessagingOptions 从配置创建消息服务选项
func NewMessagingOptions(cfg *config.MessagingConfig) *MessagingOptions {
	return &MessagingOptions{
		RequestTimeout: cfg.RequestTimeout,
		MaxMessageSize: cfg.MaxMessageSize,
		PubSub: PubSubOptions{
			Enable:           cfg.PubSub.Enable,
			MessageCacheSize: cfg.PubSub.MessageCacheSize,
			MessageCacheTTL:  cfg.PubSub.MessageCacheTTL,
		},
	}
}

// DefaultMessagingOptions 默认消息服务选项
func DefaultMessagingOptions() *MessagingOptions {
	return &MessagingOptions{
		RequestTimeout: 30 * time.Second,
		MaxMessageSize: 4 * 1024 * 1024, // 4 MB
		PubSub: PubSubOptions{
			Enable:           true,
			MessageCacheSize: 1000,
			MessageCacheTTL:  2 * time.Minute,
		},
	}
}

