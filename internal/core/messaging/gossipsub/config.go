// Package gossipsub 实现 GossipSub v1.1 协议
package gossipsub

import (
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces/messaging"
)

// ============================================================================
//                              GossipSub 配置
// ============================================================================

// Config GossipSub 配置
//
// 本结构体整合了 messaging.GossipSubConfig，提供统一的配置接口。
type Config struct {
	// ==================== Mesh 参数 ====================

	// D 是目标 mesh 大小（每个主题的目标 peer 数）
	D int

	// Dlo 是最小 mesh 大小，低于此值会触发 GRAFT
	Dlo int

	// Dhi 是最大 mesh 大小，超过此值会触发 PRUNE
	Dhi int

	// Dlazy 是 gossip 目标数量（IHAVE 发送的 peer 数）
	Dlazy int

	// Dscore 是基于评分选择 mesh peer 的数量
	Dscore int

	// Dout 是出站连接在 mesh 中的最小数量
	Dout int

	// ==================== 时间参数 ====================

	// HeartbeatInterval 心跳间隔
	HeartbeatInterval time.Duration

	// HeartbeatInitialDelay 首次心跳延迟
	HeartbeatInitialDelay time.Duration

	// FanoutTTL fanout 过期时间
	FanoutTTL time.Duration

	// SeenTTL 已见消息缓存时间
	SeenTTL time.Duration

	// HistoryLength 消息历史长度（心跳周期数）
	HistoryLength int

	// HistoryGossip gossip 的历史消息窗口（心跳周期数）
	HistoryGossip int

	// ==================== GRAFT/PRUNE 参数 ====================

	// GraftFloodThreshold GRAFT 洪泛阈值
	GraftFloodThreshold time.Duration

	// PruneBackoff PRUNE 后的退避时间
	PruneBackoff time.Duration

	// UnsubscribeBackoff 取消订阅后的退避时间
	UnsubscribeBackoff time.Duration

	// BackoffSlackTime 退避宽限时间
	BackoffSlackTime time.Duration

	// ConnectorQueueSize 连接器队列大小
	ConnectorQueueSize int

	// ==================== 消息参数 ====================

	// MaxMessageSize 最大消息大小
	MaxMessageSize int

	// MaxIHaveLength 单个 IHAVE 消息的最大消息 ID 数
	MaxIHaveLength int

	// MaxIHaveMessages 单个 RPC 中的最大 IHAVE 消息数
	MaxIHaveMessages int

	// MaxIWantLength 单个 IWANT 请求的最大消息 ID 数
	MaxIWantLength int

	// IWantFollowupTime IWANT 跟踪时间
	IWantFollowupTime time.Duration

	// GossipFactor gossip 发送比例
	GossipFactor float64

	// ==================== 评分参数 ====================

	// GossipThreshold gossip 阈值
	GossipThreshold float64

	// PublishThreshold 发布阈值
	PublishThreshold float64

	// GraylistThreshold 灰名单阈值
	GraylistThreshold float64

	// AcceptPXThreshold 接受 PX 阈值
	AcceptPXThreshold float64

	// OpportunisticGraftThreshold 机会性 GRAFT 阈值
	OpportunisticGraftThreshold float64

	// OpportunisticGraftTicks 机会性 GRAFT 间隔（心跳周期数）
	OpportunisticGraftTicks int

	// OpportunisticGraftPeers 机会性 GRAFT 的 peer 数
	OpportunisticGraftPeers int

	// ==================== 扩展功能 ====================

	// FloodPublish 是否使用洪泛发布
	FloodPublish bool

	// DirectPeers 直连 peer 列表
	DirectPeers []DirectPeer

	// SignMessages 是否对消息签名
	SignMessages bool

	// ValidateMessages 是否验证消息签名
	ValidateMessages bool

	// StrictSignatureValidation 严格签名验证
	// 如果启用，将验证签名公钥与消息 From 字段的 NodeID 是否匹配
	StrictSignatureValidation bool

	// SlowHeartbeatWarning 慢心跳警告阈值
	SlowHeartbeatWarning time.Duration
}

// DirectPeer 直连 peer 配置
type DirectPeer struct {
	// ID 节点 ID
	ID string
	// Addrs 地址列表
	Addrs []string
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		// Mesh 参数
		D:      6,
		Dlo:    4,
		Dhi:    12,
		Dlazy:  6,
		Dscore: 4,
		Dout:   2,

		// 时间参数
		HeartbeatInterval:     time.Second,
		HeartbeatInitialDelay: 100 * time.Millisecond,
		FanoutTTL:             60 * time.Second,
		SeenTTL:               120 * time.Second,
		HistoryLength:         5,
		HistoryGossip:         3,

		// GRAFT/PRUNE 参数
		GraftFloodThreshold: 10 * time.Millisecond,
		PruneBackoff:        60 * time.Second,
		UnsubscribeBackoff:  10 * time.Second,
		BackoffSlackTime:    time.Second,
		ConnectorQueueSize:  32,

		// 消息参数
		MaxMessageSize:    1 << 20, // 1 MB
		MaxIHaveLength:    5000,
		MaxIHaveMessages:  10,
		MaxIWantLength:    5000,
		IWantFollowupTime: 3 * time.Second,
		GossipFactor:      0.25,

		// 评分参数
		GossipThreshold:             -500,
		PublishThreshold:            -1000,
		GraylistThreshold:           -2500,
		AcceptPXThreshold:           10,
		OpportunisticGraftThreshold: 5,
		OpportunisticGraftTicks:     60,
		OpportunisticGraftPeers:     2,

		// 扩展功能
		FloodPublish:              false,
		SignMessages:              true,
		ValidateMessages:          true,
		StrictSignatureValidation: false, // 默认不启用严格验证
		SlowHeartbeatWarning:      100 * time.Millisecond,
	}
}

// FromMessagingConfig 从 messaging.Config 创建 GossipSub 配置
func FromMessagingConfig(cfg messaging.Config) *Config {
	gsConfig := cfg.GossipSub
	return &Config{
		// Mesh 参数
		D:      gsConfig.D,
		Dlo:    gsConfig.Dlo,
		Dhi:    gsConfig.Dhi,
		Dlazy:  gsConfig.Dlazy,
		Dscore: gsConfig.Dscore,
		Dout:   gsConfig.Dout,

		// 时间参数
		HeartbeatInterval:     cfg.HeartbeatInterval,
		HeartbeatInitialDelay: 100 * time.Millisecond,
		FanoutTTL:             gsConfig.FanoutTTL,
		SeenTTL:               gsConfig.SeenMessagesTTL,
		HistoryLength:         cfg.HistoryLength,
		HistoryGossip:         cfg.HistoryGossip,

		// GRAFT/PRUNE 参数
		GraftFloodThreshold: gsConfig.GraftFloodThreshold,
		PruneBackoff:        gsConfig.PruneBackoff,
		UnsubscribeBackoff:  gsConfig.UnsubscribeBackoff,
		BackoffSlackTime:    time.Second,
		ConnectorQueueSize:  gsConfig.ConnectorQueueSize,

		// 消息参数
		MaxMessageSize:    cfg.MaxMessageSize,
		MaxIHaveLength:    gsConfig.MaxIHaveLength,
		MaxIHaveMessages:  gsConfig.MaxIHaveMessages,
		MaxIWantLength:    5000,
		IWantFollowupTime: gsConfig.IWantFollowupTime,
		GossipFactor:      gsConfig.GossipFactor,

		// 评分参数
		GossipThreshold:             gsConfig.Scoring.GossipThreshold,
		PublishThreshold:            gsConfig.Scoring.PublishThreshold,
		GraylistThreshold:           gsConfig.Scoring.GraylistThreshold,
		AcceptPXThreshold:           gsConfig.Scoring.AcceptPXThreshold,
		OpportunisticGraftThreshold: gsConfig.OpportunisticGraftThreshold,
		OpportunisticGraftTicks:     gsConfig.OpportunisticGraftTicks,
		OpportunisticGraftPeers:     gsConfig.OpportunisticGraftPeers,

		// 扩展功能
		FloodPublish:         cfg.FloodPublish,
		SignMessages:         true,
		ValidateMessages:     true,
		SlowHeartbeatWarning: time.Duration(gsConfig.SlowHeartbeatWarning * float64(time.Second)),
	}
}

// ToMessagingConfig 转换为 messaging.GossipSubConfig
func (c *Config) ToMessagingConfig() messaging.GossipSubConfig {
	return messaging.GossipSubConfig{
		D:                           c.D,
		Dlo:                         c.Dlo,
		Dhi:                         c.Dhi,
		Dlazy:                       c.Dlazy,
		Dscore:                      c.Dscore,
		Dout:                        c.Dout,
		FanoutTTL:                   c.FanoutTTL,
		GossipFactor:                c.GossipFactor,
		OpportunisticGraftThreshold: c.OpportunisticGraftThreshold,
		OpportunisticGraftTicks:     c.OpportunisticGraftTicks,
		OpportunisticGraftPeers:     c.OpportunisticGraftPeers,
		PruneBackoff:                c.PruneBackoff,
		UnsubscribeBackoff:          c.UnsubscribeBackoff,
		ConnectorQueueSize:          c.ConnectorQueueSize,
		GraftFloodThreshold:         c.GraftFloodThreshold,
		MaxIHaveLength:              c.MaxIHaveLength,
		MaxIHaveMessages:            c.MaxIHaveMessages,
		IWantFollowupTime:           c.IWantFollowupTime,
		SeenMessagesTTL:             c.SeenTTL,
	}
}

// Validate 验证配置
func (c *Config) Validate() error {
	if c.D < 0 {
		c.D = 6
	}
	if c.Dlo < 0 {
		c.Dlo = 4
	}
	if c.Dhi < c.D {
		c.Dhi = c.D * 2
	}
	if c.Dlazy < 0 {
		c.Dlazy = c.D
	}
	if c.HeartbeatInterval <= 0 {
		c.HeartbeatInterval = time.Second
	}
	if c.HistoryLength <= 0 {
		c.HistoryLength = 5
	}
	if c.HistoryGossip <= 0 || c.HistoryGossip > c.HistoryLength {
		c.HistoryGossip = 3
	}
	return nil
}

// ============================================================================
//                              评分参数（类型别名）
// ============================================================================

// ScoreParams 评分参数（类型别名）
type ScoreParams = messaging.GossipScoreConfig

// TopicScoreParams 主题评分参数（类型别名）
type TopicScoreParams = messaging.TopicScoreConfig

// DefaultScoreParams 返回默认评分参数
func DefaultScoreParams() *ScoreParams {
	return messaging.DefaultGossipScoreConfig()
}

// DefaultTopicScoreParams 返回默认主题评分参数
func DefaultTopicScoreParams() *TopicScoreParams {
	return messaging.DefaultTopicScoreConfig()
}
