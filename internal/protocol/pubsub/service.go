// Package pubsub 实现发布订阅协议
package pubsub

import (
	"context"
	"fmt"
	"sync"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
)

// pubsub 模块 logger
var logger = log.Logger("protocol/pubsub")

// Service 实现 PubSub 接口
type Service struct {
	host      interfaces.Host
	realmMgr  interfaces.RealmManager
	gossip    *gossipSub
	validator *messageValidator

	mu      sync.RWMutex
	started bool
	ctx     context.Context
	cancel  context.CancelFunc

	config *Config

	// P1 修复：可靠投递管理器
	reliableMgr *ReliableTopicManager
}

// 确保 Service 实现了 interfaces.PubSub 接口
var _ interfaces.PubSub = (*Service)(nil)

// New 创建 PubSub 服务（全局模式）
func New(host interfaces.Host, realmMgr interfaces.RealmManager, opts ...Option) (*Service, error) {
	if host == nil {
		return nil, ErrNilHost
	}
	// RealmManager 现在是可选的
	// 如果 realmMgr == nil，某些 Realm 相关功能将被禁用

	// 应用配置
	config := DefaultConfig()
	for _, opt := range opts {
		opt(config)
	}

	s := &Service{
		host:     host,
		realmMgr: realmMgr,
		config:   config,
	}

	// 创建 GossipSub
	s.gossip = newGossipSub(host, realmMgr, config)

	// 创建验证器
	s.validator = newMessageValidator(realmMgr, config.MaxMessageSize)

	// 彻底重构：初始化可靠投递管理器（使用内嵌配置）
	if config.ReliableDelivery.Enabled {
		s.reliableMgr = NewReliableTopicManager(config.ReliableDelivery.PublisherConfig)
		logger.Info("已启用可靠投递功能")
	}

	return s, nil
}

// NewForRealm 创建绑定到特定 Realm 的 PubSub 服务
//
// 与全局模式不同，此构造函数将服务绑定到特定的 Realm：
// - 协议 ID 包含 RealmID: /dep2p/app/<realmID>/pubsub/1.0.0
// - 只处理该 Realm 的消息
// - 成员验证基于绑定的 Realm
func NewForRealm(host interfaces.Host, realm interfaces.Realm, opts ...Option) (*Service, error) {
	if host == nil {
		return nil, ErrNilHost
	}
	if realm == nil {
		return nil, fmt.Errorf("realm is required for NewForRealm")
	}

	// 应用配置
	config := DefaultConfig()
	for _, opt := range opts {
		opt(config)
	}

	s := &Service{
		host:   host,
		config: config,
	}

	// 创建绑定到 Realm 的 GossipSub
	s.gossip = newGossipSubForRealm(host, realm, config)

	// 创建绑定到 Realm 的验证器
	s.validator = newMessageValidatorForRealm(realm, config.MaxMessageSize)

	// 彻底重构：初始化可靠投递管理器（使用内嵌配置）
	if config.ReliableDelivery.Enabled {
		s.reliableMgr = NewReliableTopicManager(config.ReliableDelivery.PublisherConfig)
		logger.Info("已启用可靠投递功能", "realm", realm.ID())
	}

	return s, nil
}

// Start 启动服务
func (s *Service) Start(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		logger.Debug("PubSub 服务已启动，跳过")
		return ErrAlreadyStarted
	}

	logger.Info("正在启动 PubSub 服务")

	// 使用 context.Background() 而不是传入的 ctx
	// 因为 Fx OnStart 的 ctx 在返回后会被取消，导致后台循环提前退出
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.started = true

	// 启动 GossipSub
	if err := s.gossip.Start(s.ctx); err != nil {
		logger.Error("GossipSub 启动失败", "error", err)
		return err
	}

	logger.Info("PubSub 服务启动成功")
	return nil
}

// Stop 停止服务
func (s *Service) Stop(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		logger.Debug("PubSub 服务未启动，跳过停止")
		return ErrNotStarted
	}

	logger.Info("正在停止 PubSub 服务")

	if s.cancel != nil {
		s.cancel()
	}

	// P1 修复：停止可靠投递管理器
	if s.reliableMgr != nil {
		if err := s.reliableMgr.Close(); err != nil {
			logger.Warn("关闭可靠投递管理器失败", "error", err)
		}
	}

	// 停止 GossipSub
	if err := s.gossip.Stop(); err != nil {
		logger.Error("GossipSub 停止失败", "error", err)
		return err
	}

	s.started = false
	logger.Info("PubSub 服务已停止")
	return nil
}

// Join 加入主题
func (s *Service) Join(topicName string, _ ...interfaces.TopicOption) (interfaces.Topic, error) {
	logger.Debug("加入主题", "topic", topicName)

	s.mu.RLock()
	if !s.started {
		s.mu.RUnlock()
		logger.Warn("PubSub 服务未启动，无法加入主题", "topic", topicName)
		return nil, ErrNotStarted
	}
	s.mu.RUnlock()

	// 加入主题(内部会检查重复)
	t, err := s.gossip.Join(topicName)
	if err != nil {
		logger.Error("加入主题失败", "topic", topicName, "error", err)
		return nil, err
	}

	// 设置 topic 的 ps 引用
	t.ps = s

	// P1 修复：如果启用可靠投递，包装 Topic
	if s.reliableMgr != nil {
		rt, err := s.reliableMgr.WrapTopic(t)
		if err != nil {
			logger.Warn("包装可靠 Topic 失败，回退到普通模式", "topic", topicName, "error", err)
			// 回退到普通模式
		} else {
			logger.Info("已加入主题（可靠模式）", "topic", topicName)
			return rt, nil
		}
	}

	logger.Info("已加入主题", "topic", topicName)
	return t, nil
}

// GetTopics 获取所有已加入的主题
func (s *Service) GetTopics() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	s.gossip.mu.RLock()
	defer s.gossip.mu.RUnlock()

	topics := make([]string, 0, len(s.gossip.topics))
	for name := range s.gossip.topics {
		topics = append(topics, name)
	}
	return topics
}

// ListPeers 列出指定主题的所有节点
func (s *Service) ListPeers(topicName string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.gossip.mesh.List(topicName)
}

// Close 关闭服务
func (s *Service) Close() error {
	return s.Stop(context.Background())
}

// RegisterTopicValidator 注册主题验证器
func (s *Service) RegisterTopicValidator(topic string, validator ValidatorFunc) {
	s.validator.RegisterValidator(topic, validator)
}

// UnregisterTopicValidator 注销主题验证器
func (s *Service) UnregisterTopicValidator(topic string) {
	s.validator.UnregisterValidator(topic)
}

// SetHealthMonitor 设置网络健康监控器
//
// Phase 5.1: 用于支持发送错误上报和网络状态检测
// 当消息发送失败时，会通过此监控器上报错误，支持网络弹性恢复
func (s *Service) SetHealthMonitor(monitor interfaces.ConnectionHealthMonitor) {
	s.gossip.SetHealthMonitor(monitor)
}

// ════════════════════════════════════════════════════════════════════════════
//                       节点评分 API（P1 修复完成）
// ════════════════════════════════════════════════════════════════════════════

// GetPeerScore 获取节点评分
func (s *Service) GetPeerScore(peerID string) float64 {
	return s.gossip.GetPeerScore(peerID)
}

// SetAppScore 设置节点的应用层评分
//
// 应用层可以根据业务逻辑设置额外的评分，例如：
// - 提供有价值数据的节点加分
// - 恶意行为的节点减分
func (s *Service) SetAppScore(peerID string, score float64) {
	s.gossip.SetAppScore(peerID, score)
}

// GetScorer 获取评分器（供高级用户访问）
func (s *Service) GetScorer() *PeerScorer {
	return s.gossip.GetScorer()
}

// IsScoringEnabled 检查是否启用评分
func (s *Service) IsScoringEnabled() bool {
	return s.gossip.GetScorer() != nil
}

// SetTopicScoreParams 设置主题评分参数
func (s *Service) SetTopicScoreParams(topic string, params *TopicScoreParams) {
	if scorer := s.gossip.GetScorer(); scorer != nil {
		scorer.SetTopicParams(topic, params)
	}
}
