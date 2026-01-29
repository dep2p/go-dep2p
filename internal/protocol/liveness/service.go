// Package liveness 实现存活检测服务
package liveness

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/dep2p/go-dep2p/pkg/interfaces"
	"github.com/dep2p/go-dep2p/pkg/lib/log"
	"github.com/dep2p/go-dep2p/pkg/protocol"
)

var logger = log.Logger("protocol/liveness")

// Service 实现 Liveness 接口
type Service struct {
	host     interfaces.Host
	realmMgr interfaces.RealmManager // 可选，用于全局模式
	realm    interfaces.Realm        // 可选，用于 Realm 绑定模式
	realmID  string                  // 绑定的 RealmID（如果有）

	mu      sync.RWMutex
	started bool
	ctx     context.Context
	cancel  context.CancelFunc

	// 状态管理
	statuses map[string]*peerStatus

	// Watch 管理
	watches map[string][]chan interfaces.LivenessEvent

	// 配置
	config *Config
}

// 确保 Service 实现了 interfaces.Liveness 接口
var _ interfaces.Liveness = (*Service)(nil)

// New 创建 Liveness 服务（全局模式）
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
		statuses: make(map[string]*peerStatus),
		watches:  make(map[string][]chan interfaces.LivenessEvent),
		config:   config,
	}

	return s, nil
}

// NewForRealm 创建绑定到特定 Realm 的 Liveness 服务
//
// 与全局模式不同，此构造函数将服务绑定到特定的 Realm：
// - 协议 ID 包含 RealmID: /dep2p/app/<realmID>/liveness/1.0.0
// - 只监控该 Realm 的成员
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
		host:     host,
		realm:    realm,
		realmID:  realm.ID(),
		statuses: make(map[string]*peerStatus),
		watches:  make(map[string][]chan interfaces.LivenessEvent),
		config:   config,
	}

	return s, nil
}

// Start 启动服务
func (s *Service) Start(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return ErrAlreadyStarted
	}

	logger.Info("正在启动 Liveness 服务")

	// 使用 context.Background() 而不是传入的 ctx
	// 因为 Fx OnStart 的 ctx 在返回后会被取消，导致后台循环提前退出
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.started = true

	// 根据模式注册 Ping 处理器
	if s.realm != nil && s.realmID != "" {
		// Realm-bound 模式：只为绑定的 Realm 注册
		fullProtocol := buildProtocolID(s.realmID)
		s.registerPingHandler(fullProtocol)
		logger.Debug("注册 Realm-bound Ping 处理器", "realmID", s.realmID)
	} else if s.realmMgr != nil {
		// 全局模式：为所有 Realm 注册
		realms := s.realmMgr.ListRealms()
		for _, realm := range realms {
			fullProtocol := buildProtocolID(realm.ID())
			s.registerPingHandler(fullProtocol)
		}
		logger.Debug("注册全局 Ping 处理器", "realmCount", len(realms))
	}

	logger.Info("Liveness 服务启动成功")
	return nil
}

// Stop 停止服务
func (s *Service) Stop(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return ErrNotStarted
	}

	logger.Info("正在停止 Liveness 服务")

	if s.cancel != nil {
		s.cancel()
	}

	// 根据模式移除 Ping 处理器
	if s.realm != nil && s.realmID != "" {
		// Realm-bound 模式：只移除绑定的 Realm 的处理器
		fullProtocol := buildProtocolID(s.realmID)
		s.host.RemoveStreamHandler(fullProtocol)
		logger.Debug("移除 Realm-bound Ping 处理器", "realmID", s.realmID)
	} else if s.realmMgr != nil {
		// 全局模式：移除所有 Realm 的处理器
		realms := s.realmMgr.ListRealms()
		for _, realm := range realms {
			fullProtocol := buildProtocolID(realm.ID())
			s.host.RemoveStreamHandler(fullProtocol)
		}
	}

	// 关闭所有 watch 通道
	for _, channels := range s.watches {
		for _, ch := range channels {
			close(ch)
		}
	}
	s.watches = make(map[string][]chan interfaces.LivenessEvent)

	s.started = false
	logger.Info("Liveness 服务已停止")
	return nil
}

// Ping 发送 ping 并测量 RTT
func (s *Service) Ping(ctx context.Context, peerID string) (time.Duration, error) {
	s.mu.RLock()
	if !s.started {
		s.mu.RUnlock()
		return 0, ErrNotStarted
	}
	s.mu.RUnlock()

	// 验证节点ID
	if peerID == "" {
		return 0, ErrInvalidPeerID
	}

	// 确定使用的协议
	var fullProtocol string

	if s.realm != nil && s.realmID != "" {
		// Realm-bound 模式：直接使用绑定的 Realm
		fullProtocol = buildProtocolID(s.realmID)
	} else if s.realmMgr != nil {
		// 全局模式：查找包含该节点的 Realm
		var found bool
		realms := s.realmMgr.ListRealms()
		for _, realm := range realms {
			if realm.IsMember(peerID) {
				fullProtocol = buildProtocolID(realm.ID())
				found = true
				break
			}
		}

		// 如果有默认Realm，也尝试使用它
		if !found && s.config.DefaultRealmID != "" {
			realm, ok := s.realmMgr.GetRealm(s.config.DefaultRealmID)
			if ok {
				fullProtocol = buildProtocolID(realm.ID())
				found = true
			}
		}

		if !found {
			// 没有 Realm，使用默认协议（使用空 realmID 作为后备）
			fullProtocol = string(protocol.BuildAppProtocol("", protocol.AppProtocolLiveness+"/ping", protocol.Version10))
		}
	} else {
		// 无 Realm 模式：使用默认协议（使用空 realmID 作为后备）
		fullProtocol = string(protocol.BuildAppProtocol("", protocol.AppProtocolLiveness+"/ping", protocol.Version10))
	}

	// 设置超时
	pingCtx, cancel := context.WithTimeout(ctx, s.config.Timeout)
	defer cancel()

	logger.Debug("发送 Ping", "peerID", log.TruncateID(peerID, 8), "protocol", fullProtocol)
	
	// 打开流
	stream, err := s.host.NewStream(pingCtx, peerID, fullProtocol)
	if err != nil {
		logger.Debug("Ping 打开流失败", "peerID", log.TruncateID(peerID, 8), "error", err)
		s.updateStatus(peerID, 0, false)
		return 0, err
	}
	defer stream.Close()

	// 发送 Ping
	startTime := time.Now()
	ping := NewPingRequest()
	
	pingData, err := encodePing(ping)
	if err != nil {
		logger.Warn("Ping 编码失败", "peerID", log.TruncateID(peerID, 8), "error", err)
		s.updateStatus(peerID, 0, false)
		return 0, err
	}

	if err := sendMessage(stream, pingData); err != nil {
		logger.Warn("Ping 发送失败", "peerID", log.TruncateID(peerID, 8), "error", err)
		s.updateStatus(peerID, 0, false)
		return 0, err
	}

	// 接收 Pong
	pongData, err := receiveMessage(stream)
	if err != nil {
		logger.Debug("Ping 接收响应失败", "peerID", log.TruncateID(peerID, 8), "error", err)
		s.updateStatus(peerID, 0, false)
		return 0, err
	}

	pong, err := decodePong(pongData)
	if err != nil {
		logger.Warn("Ping 解码响应失败", "peerID", log.TruncateID(peerID, 8), "error", err)
		s.updateStatus(peerID, 0, false)
		return 0, ErrInvalidMessage
	}

	// 验证响应ID
	if pong.ID != ping.ID {
		logger.Warn("Ping 响应 ID 不匹配", "peerID", log.TruncateID(peerID, 8))
		s.updateStatus(peerID, 0, false)
		return 0, ErrInvalidMessage
	}

	// 计算 RTT
	rtt := time.Since(startTime)

	// 更新状态
	s.updateStatus(peerID, rtt, true)
	logger.Debug("Ping 成功", "peerID", log.TruncateID(peerID, 8), "rtt", rtt)

	return rtt, nil
}

// Check 检查节点是否存活
func (s *Service) Check(ctx context.Context, peerID string) (bool, error) {
	s.mu.RLock()
	if !s.started {
		s.mu.RUnlock()
		return false, ErrNotStarted
	}
	s.mu.RUnlock()

	// 发送 Ping 检查
	_, err := s.Ping(ctx, peerID)
	if err != nil {
		return false, nil
	}

	return true, nil
}

// Watch 监控节点状态变化
func (s *Service) Watch(peerID string) (<-chan interfaces.LivenessEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil, ErrNotStarted
	}

	// 创建事件通道
	ch := make(chan interfaces.LivenessEvent, 16)

	// 添加到 watches
	if s.watches[peerID] == nil {
		s.watches[peerID] = make([]chan interfaces.LivenessEvent, 0)
	}
	s.watches[peerID] = append(s.watches[peerID], ch)

	return ch, nil
}

// Unwatch 停止监控节点
func (s *Service) Unwatch(peerID string) error {
	s.mu.Lock()
	
	if !s.started {
		s.mu.Unlock()
		return ErrNotStarted
	}

	channels, ok := s.watches[peerID]
	if !ok || len(channels) == 0 {
		s.mu.Unlock()
		return ErrWatchNotFound
	}

	// 先从 map 中移除（持有锁）
	delete(s.watches, peerID)
	s.mu.Unlock()

	// 然后关闭所有通道（不持有锁，避免死锁）
	for _, ch := range channels {
		close(ch)
	}

	return nil
}

// GetStatus 获取节点存活状态
func (s *Service) GetStatus(peerID string) interfaces.LivenessStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status, ok := s.statuses[peerID]
	if !ok {
		// 未知节点,返回默认状态
		return interfaces.LivenessStatus{
			Alive: false,
		}
	}

	return status.getStatus()
}

// updateStatus 更新节点状态 (内部方法)
func (s *Service) updateStatus(peerID string, rtt time.Duration, success bool) {
	s.mu.Lock()
	
	// 获取或创建状态
	status, ok := s.statuses[peerID]
	if !ok {
		status = newPeerStatus(peerID)
		s.statuses[peerID] = status
	}
	
	// 记录之前的状态
	oldAlive := status.isAlive()
	
	s.mu.Unlock()

	// 更新状态（不持有Service锁）
	if success {
		status.recordSuccess(rtt)
	} else {
		status.recordFailure()
	}

	// 获取新状态
	newStatus := status.getStatus()
	newAlive := newStatus.Alive

	// 通知监听者
	var eventType interfaces.LivenessEventType
	if success {
		eventType = interfaces.LivenessEventPong
	} else {
		eventType = interfaces.LivenessEventTimeout
	}

	// 检查状态变化
	if oldAlive != newAlive {
		if newAlive {
			eventType = interfaces.LivenessEventUp
		} else {
			eventType = interfaces.LivenessEventDown
		}
	}

	event := interfaces.LivenessEvent{
		PeerID:    peerID,
		Type:      eventType,
		Status:    newStatus,
		Timestamp: time.Now(),
		RTT:       rtt,
	}

	s.notifyWatchers(peerID, event)
}

// notifyWatchers 通知监听者 (内部方法)
func (s *Service) notifyWatchers(peerID string, event interfaces.LivenessEvent) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	channels, ok := s.watches[peerID]
	if !ok || len(channels) == 0 {
		return
	}

	// 在持有锁的情况下同步发送（使用 select + default 避免阻塞）
	// 这样可以避免竞态条件
	for _, ch := range channels {
		select {
		case ch <- event:
			// 发送成功
		default:
			// 通道满了，丢弃事件
		}
	}
}

// registerPingHandler 注册 Ping 处理器 (内部方法)
func (s *Service) registerPingHandler(fullProtocol string) {
	s.host.SetStreamHandler(fullProtocol, func(stream interfaces.Stream) {
		defer stream.Close()

		// 接收 Ping
		pingData, err := receiveMessage(stream)
		if err != nil {
			return
		}

		ping, err := decodePing(pingData)
		if err != nil {
			return
		}

		// 发送 Pong
		pong := NewPongResponse(ping.ID)
		pongData, err := encodePong(pong)
		if err != nil {
			return
		}

		sendMessage(stream, pongData)
	})
}
